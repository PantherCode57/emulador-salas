// Modo "sala" del enlace: salas guardadas en las que cualquiera puede entrar primero.
//
//	enlace sala --codigo K7P2QX --id Jugador-AB12 --progreso mi.json --amigos carpeta/
//
// Al entrar se busca si alguien de la sala ya la tiene abierta (escucha en TCP en el puerto de
// la sala). Si lo hay, entramos como amigo; si no, la abrimos nosotros. Si dos entran a la vez,
// el que tenga la IP de Tailscale más alta pasa a ser amigo del otro.
//
// Por esa misma conexión TCP se mandan el progreso de cada uno (un JSON con nombre, tiempo
// jugado, captura...). El anfitrión reenvía el de cada amigo a los demás. Lo que llega se guarda
// en la carpeta de amigos como <id>.json, así se puede ver aunque el amigo ya se haya ido.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/tsnet"
)

const (
	maxLinea      = 4 << 20 // un progreso con captura ocupa unas decenas de KB
	esperaLectura = 15 * time.Second
	pulso         = 5 * time.Second
)

// ---------------------------------------------------------------------------------------------
// Progreso: lo que se cuentan los jugadores de la sala.

type conexion struct {
	c  net.Conn
	mu sync.Mutex
	w  *bufio.Writer
}

func (cx *conexion) enviar(linea []byte) error {
	cx.mu.Lock()
	defer cx.mu.Unlock()
	_ = cx.c.SetWriteDeadline(time.Now().Add(30 * time.Second))
	if _, err := cx.w.Write(linea); err != nil {
		return err
	}
	if err := cx.w.WriteByte('\n'); err != nil {
		return err
	}
	return cx.w.Flush()
}

type progresos struct {
	propio  string // archivo con nuestro progreso (lo escribe Pokemulador)
	carpeta string // carpeta donde guardamos el de los amigos
	id      string // nuestro identificador, para no guardarnos a nosotros mismos

	mu        sync.Mutex
	conns     map[*conexion]struct{}
	conocidos map[string][]byte // id → última línea recibida (el anfitrión se la da a los nuevos)
}

func nuevosProgresos(propio, carpeta, id string) *progresos {
	if carpeta != "" {
		_ = os.MkdirAll(carpeta, 0o755)
	}
	return &progresos{
		propio:    propio,
		carpeta:   carpeta,
		id:        id,
		conns:     map[*conexion]struct{}{},
		conocidos: map[string][]byte{},
	}
}

func lineaProgreso(datos json.RawMessage) []byte {
	b, _ := json.Marshal(map[string]any{"tipo": "progreso", "datos": datos})
	return b
}

// lineaPropia lee nuestro progreso del disco. Devuelve nil si aún no hay.
func (p *progresos) lineaPropia() []byte {
	if p.propio == "" {
		return nil
	}
	b, err := os.ReadFile(p.propio)
	if err != nil || len(bytes.TrimSpace(b)) == 0 || !json.Valid(b) {
		return nil
	}
	return lineaProgreso(json.RawMessage(bytes.TrimSpace(b)))
}

func (p *progresos) cuantos() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}

func (p *progresos) difundir(linea []byte, excepto *conexion) {
	p.mu.Lock()
	destinos := make([]*conexion, 0, len(p.conns))
	for cx := range p.conns {
		if cx != excepto {
			destinos = append(destinos, cx)
		}
	}
	p.mu.Unlock()
	for _, cx := range destinos {
		go func(cx *conexion) {
			if cx.enviar(linea) != nil {
				cx.c.Close()
			}
		}(cx)
	}
}

// vigilarPropio manda nuestro progreso a todos cada vez que Pokemulador lo actualiza.
func (p *progresos) vigilarPropio(ctx context.Context) {
	if p.propio == "" {
		return
	}
	var ultimo time.Time
	var tam int64 = -1
	if fi, err := os.Stat(p.propio); err == nil {
		ultimo, tam = fi.ModTime(), fi.Size()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
		fi, err := os.Stat(p.propio)
		if err != nil || (fi.ModTime().Equal(ultimo) && fi.Size() == tam) {
			continue
		}
		ultimo, tam = fi.ModTime(), fi.Size()
		if linea := p.lineaPropia(); linea != nil {
			p.difundir(linea, nil)
		}
	}
}

func nombreArchivo(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return b.String() + ".json"
}

func (p *progresos) guardarAmigo(id string, datos json.RawMessage) {
	if p.carpeta == "" {
		return
	}
	nombre := nombreArchivo(id)
	if nombre == "" {
		return
	}
	destino := filepath.Join(p.carpeta, nombre)
	tmp := destino + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, destino) != nil {
		_ = os.Remove(tmp)
	}
}

// atender lleva una conexión TCP con otro jugador hasta que se corta.
func (p *progresos) atender(ctx context.Context, c net.Conn, anfitrion bool, alCambiar func()) {
	cx := &conexion{c: c, w: bufio.NewWriter(c)}
	p.mu.Lock()
	p.conns[cx] = struct{}{}
	var previos [][]byte
	if anfitrion {
		for _, l := range p.conocidos {
			previos = append(previos, l)
		}
	}
	p.mu.Unlock()
	alCambiar()

	ctxConn, cancelar := context.WithCancel(ctx)
	defer func() {
		cancelar()
		c.Close()
		p.mu.Lock()
		delete(p.conns, cx)
		p.mu.Unlock()
		alCambiar()
	}()
	go func() {
		<-ctxConn.Done()
		c.Close()
	}()

	// Lo primero, un saludo, nuestro progreso y (si somos el anfitrión) el de los demás amigos.
	go func() {
		hola, _ := json.Marshal(map[string]any{"tipo": "hola"})
		if cx.enviar(hola) != nil {
			c.Close()
			return
		}
		if linea := p.lineaPropia(); linea != nil {
			if cx.enviar(linea) != nil {
				c.Close()
				return
			}
		}
		for _, l := range previos {
			if cx.enviar(l) != nil {
				c.Close()
				return
			}
		}
		for {
			select {
			case <-ctxConn.Done():
				return
			case <-time.After(pulso):
			}
			if cx.enviar(hola) != nil {
				c.Close()
				return
			}
		}
	}()

	lector := bufio.NewReaderSize(c, 64<<10)
	for {
		_ = c.SetReadDeadline(time.Now().Add(esperaLectura))
		linea, err := leerLinea(lector)
		if err != nil {
			return
		}
		var m struct {
			Tipo  string          `json:"tipo"`
			Datos json.RawMessage `json:"datos"`
		}
		if json.Unmarshal(linea, &m) != nil || m.Tipo != "progreso" || len(m.Datos) == 0 {
			continue
		}
		var cabecera struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(m.Datos, &cabecera) != nil || cabecera.ID == "" || cabecera.ID == p.id {
			continue
		}
		copia := append([]byte(nil), linea...)
		p.mu.Lock()
		p.conocidos[cabecera.ID] = copia
		p.mu.Unlock()
		p.guardarAmigo(cabecera.ID, m.Datos)
		avisar("progreso", map[string]any{"id": cabecera.ID})
		if anfitrion {
			p.difundir(copia, cx)
		}
	}
}

func leerLinea(r *bufio.Reader) ([]byte, error) {
	var total []byte
	for {
		trozo, esPrefijo, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		total = append(total, trozo...)
		if len(total) > maxLinea {
			return nil, fmt.Errorf("línea demasiado larga")
		}
		if !esPrefijo {
			return total, nil
		}
	}
}

// ---------------------------------------------------------------------------------------------
// Quién tiene la sala abierta.

// buscarAnfitrion prueba a conectar por TCP con cada dispositivo de esta sala. El que responde
// es el que la tiene abierta.
func buscarAnfitrion(ctx context.Context, srv *tsnet.Server, lc *local.Client, codigo string, puerto int, propia netip.Addr) (netip.Addr, net.Conn) {
	ip, c, _ := buscarAnfitrionContando(ctx, srv, lc, codigo, puerto, propia)
	return ip, c
}

// buscarAnfitrionContando además dice a cuántos dispositivos de la sala ha preguntado.
func buscarAnfitrionContando(ctx context.Context, srv *tsnet.Server, lc *local.Client, codigo string, puerto int, propia netip.Addr) (netip.Addr, net.Conn, int) {
	estado, err := lc.Status(ctx)
	if err != nil {
		return netip.Addr{}, nil, 0
	}
	probados := 0
	base := "pkm-" + codigo
	for _, p := range estado.Peer {
		nombre := strings.ToLower(p.HostName)
		if !p.Online || (nombre != base && !strings.HasPrefix(nombre, base+"-")) {
			continue
		}
		var ip netip.Addr
		for _, a := range p.TailscaleIPs {
			if a.Is4() {
				ip = a
			}
		}
		if !ip.IsValid() || ip == propia {
			continue
		}
		probados++
		ctxDial, cancelar := context.WithTimeout(ctx, 3*time.Second)
		c, err := srv.Dial(ctxDial, "tcp", netip.AddrPortFrom(ip, uint16(puerto)).String())
		cancelar()
		if err == nil {
			return ip, c, probados
		}
	}
	return netip.Addr{}, nil, probados
}

func sala(ctx context.Context, srv *tsnet.Server, codigo string, puerto, puertoLocal, puertoJuego int, prog *progresos) {
	lc, err := srv.LocalClient()
	if err != nil {
		fallar("Tailscale no responde: " + err.Error())
	}
	propia, _ := srv.TailscaleIPs()
	go prog.vigilarPropio(ctx)

	avisar("buscando", nil)
	var ip netip.Addr
	var conn net.Conn
	// La lista de dispositivos puede tardar un momento en llegar: si no vemos a nadie de la sala,
	// miramos otra vez. Si vemos a alguien y nadie responde, es que no hay nadie con ella abierta.
	for intento := 0; intento < 3; intento++ {
		if intento > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
		var probados int
		ip, conn, probados = buscarAnfitrionContando(ctx, srv, lc, codigo, puerto, propia)
		if conn != nil || probados > 0 {
			break
		}
	}

	if conn == nil {
		avisar("rol", map[string]any{"rol": "host"})
		ip, conn = salaAnfitrion(ctx, srv, lc, codigo, puerto, puertoJuego, propia, prog)
		if conn == nil {
			return
		}
	}
	avisar("rol", map[string]any{"rol": "unirse"})
	salaAmigo(ctx, srv, lc, codigo, ip, conn, puerto, puertoLocal, puertoJuego, propia, prog)
}

// salaAnfitrion abre la sala. Devuelve la conexión con otro anfitrión si resulta que hay dos y
// nos toca pasar a ser su amigo.
func salaAnfitrion(ctx context.Context, srv *tsnet.Server, lc *local.Client, codigo string, puerto, puertoJuego int, propia netip.Addr, prog *progresos) (netip.Addr, net.Conn) {
	ctxSala, cerrar := context.WithCancel(ctx)
	defer cerrar()

	ln, err := srv.Listen("tcp", fmt.Sprintf(":%d", puerto))
	if err != nil {
		fallar("No se pudo abrir la sala en Tailscale: " + err.Error())
	}
	pc, err := srv.ListenPacket("udp", netip.AddrPortFrom(propia, uint16(puerto)).String())
	if err != nil {
		ln.Close()
		fallar("No se pudo abrir la sala en Tailscale: " + err.Error())
	}
	go func() {
		<-ctxSala.Done()
		ln.Close()
		pc.Close()
	}()

	contar := func() { avisar("amigos", map[string]any{"n": prog.cuantos()}) }
	go reenviarSala(ctxSala, pc, puerto, nil)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go clasificarEntrada(ctxSala, c, puertoJuego, func(c net.Conn) { prog.atender(ctxSala, c, true, contar) })
		}
	}()

	// ¿Ha entrado alguien más a la vez? El de IP más baja se queda la sala.
	comprobar := func() (netip.Addr, net.Conn) {
		otra, c := buscarAnfitrion(ctxSala, srv, lc, codigo, puerto, propia)
		if c == nil {
			return netip.Addr{}, nil
		}
		if otra.Less(propia) {
			return otra, c
		}
		c.Close() // somos nosotros los que nos quedamos: el otro se pasará a amigo
		return netip.Addr{}, nil
	}
	esperas := []time.Duration{3 * time.Second, 5 * time.Second}
	for i := 0; ; i++ {
		espera := 20 * time.Second
		if i < len(esperas) {
			espera = esperas[i]
		}
		select {
		case <-ctxSala.Done():
			return netip.Addr{}, nil
		case <-time.After(espera):
		}
		if otra, c := comprobar(); c != nil {
			cerrar()
			return otra, c
		}
		if i == 0 {
			avisar("listo", map[string]any{"ip": propia.String()})
		}
	}
}

// reenviarSala lleva lo que llega por Tailscale a la sala local de Azahar. Cada amigo usa su
// propio socket local, así la sala lo ve como un jugador distinto.
func reenviarSala(ctx context.Context, pc net.PacketConn, puerto int, contar func(int)) {
	defer pc.Close()
	salaLocal := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: puerto}
	var mu sync.Mutex
	sesiones := map[string]*net.UDPConn{}
	defer func() {
		mu.Lock()
		for _, s := range sesiones {
			s.Close()
		}
		mu.Unlock()
	}()
	buf := make([]byte, 65535)
	for {
		n, origen, err := pc.ReadFrom(buf)
		if err != nil {
			return
		}
		clave := origen.String()
		mu.Lock()
		s, existe := sesiones[clave]
		if !existe {
			s, err = net.DialUDP("udp", nil, salaLocal)
			if err != nil {
				mu.Unlock()
				continue
			}
			sesiones[clave] = s
			if contar != nil {
				contar(len(sesiones))
			}
			go func(s *net.UDPConn, origen net.Addr, clave string) {
				b := make([]byte, 65535)
				for {
					_ = s.SetReadDeadline(time.Now().Add(inactividad))
					m, err := s.Read(b)
					if err != nil {
						break
					}
					_, _ = pc.WriteTo(b[:m], origen)
				}
				s.Close()
				mu.Lock()
				delete(sesiones, clave)
				quedan := len(sesiones)
				mu.Unlock()
				if contar != nil {
					contar(quedan)
				}
			}(s, origen, clave)
		}
		mu.Unlock()
		_, _ = s.Write(buf[:n])
	}
}

// destinoUDP es la conexión con la sala del anfitrión; se cambia si el anfitrión sale y vuelve.
type destinoUDP struct {
	actual atomic.Pointer[net.Conn]
}

func (d *destinoUDP) cambiar(c net.Conn) {
	if viejo := d.actual.Swap(&c); viejo != nil {
		(*viejo).Close()
	}
}

func (d *destinoUDP) escribir(b []byte) {
	if c := d.actual.Load(); c != nil {
		_, _ = (*c).Write(b)
	}
}

// salaAmigo entra en la sala de otro. Si el anfitrión se va, seguimos buscando por si vuelve
// (o entra otro de la sala y la abre él).
func salaAmigo(ctx context.Context, srv *tsnet.Server, lc *local.Client, codigo string, ip netip.Addr, conn net.Conn, puerto, puertoLocal, puertoJuego int, propia netip.Addr, prog *progresos) {
	// El puerto local puede seguir ocupado un momento si veníamos de tener la sala abierta.
	var local *net.UDPConn
	for intento := 0; ; intento++ {
		var err error
		local, err = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: puertoLocal})
		if err == nil {
			break
		}
		if intento >= 10 {
			fallar(fmt.Sprintf("El puerto %d de este equipo está ocupado. Cierra otras salas de Azahar.", puertoLocal))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	defer local.Close()
	go func() {
		<-ctx.Done()
		local.Close()
	}()

	var destino destinoUDP
	var cliente atomic.Pointer[net.UDPAddr]
	conectarUDP := func(ip netip.Addr) bool {
		remoto, err := srv.Dial(ctx, "udp", netip.AddrPortFrom(ip, uint16(puerto)).String())
		if err != nil {
			return false
		}
		destino.cambiar(remoto)
		go func() {
			b := make([]byte, 65535)
			for {
				m, err := remoto.Read(b)
				if err != nil {
					return
				}
				if a := cliente.Load(); a != nil {
					_, _ = local.WriteToUDP(b[:m], a)
				}
			}
		}()
		return true
	}
	go func() {
		buf := make([]byte, 65535)
		for {
			n, origen, err := local.ReadFromUDP(buf)
			if err != nil {
				return
			}
			cliente.Store(origen)
			destino.escribir(buf[:n])
		}
	}()

	ctxRuta, pararRuta := context.WithCancel(ctx)
	for ctx.Err() == nil {
		if conn != nil && conectarUDP(ip) {
			avisar("conectado", map[string]any{"ip": ip.String()})
			avisar("amigos", map[string]any{"n": 1})
			go informarRuta(ctxRuta, lc, ip)
			// Mientras estemos conectados, nuestro emulador puede entrar en la partida compartida.
			ctxJuego, pararJuego := context.WithCancel(ctx)
			go servirJuegoLocal(ctxJuego, srv, ip, puerto, puertoJuego)
			prog.atender(ctx, conn, false, func() {})
			pararJuego()
			pararRuta()
			ctxRuta, pararRuta = context.WithCancel(ctx)
			if ctx.Err() != nil {
				break
			}
			avisar("amigos", map[string]any{"n": 0})
			avisar("buscando", nil)
		} else if conn != nil {
			conn.Close()
		}
		conn = nil
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			ip, conn = buscarAnfitrion(ctx, srv, lc, codigo, puerto, propia)
		}
	}
	pararRuta()
	if c := destino.actual.Load(); c != nil {
		(*c).Close()
	}
}
