// Enlace de Pokemulador: conecta dos dispositivos por un código de sala usando Tailscale
// (tsnet) dentro del propio programa, sin instalar Tailscale ni activar una VPN.
//
//	enlace host   --codigo K7P2QX   el que crea la sala: expone su sala de Azahar (UDP 24872)
//	enlace unirse --codigo K7P2QX   el amigo: abre 127.0.0.1:24872 y lo lleva hasta el anfitrión
//	enlace sala   --codigo K7P2QX   sala guardada: entra el que llegue y se reparten los papeles
//	                                (ver sala.go), se pasan el progreso de cada uno y llevan la
//	                                partida compartida de GBA y el cable link de Game Boy (juego.go)
//
// La clave (secreto OAuth "tskey-client-…") llega en la variable POKEMULADOR_TS_KEY.
// Cada línea que escribe en la salida es un JSON con el estado, para que lo lea Pokemulador.
//
// Versión de la consola portátil (Panther OS): sin clave. La primera vez la consola entra en tu red
// de Tailscale con tu cuenta: el enlace avisa {"estado":"login","url":"https://login.tailscale.com/…"}
// y la app lo enseña como QR. La sesión se queda guardada en --estado (no hay que repetirlo) y la
// consola se apunta con la etiqueta tag:pokemulador, como los móviles y los PC de la app.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/ipn"
	"tailscale.com/ipn/store/mem"
	"tailscale.com/tsnet"
)

const (
	etiqueta       = "tag:pokemulador"
	alfabetoCodigo = "abcdefghijklmnopqrstuvwxyz0123456789"
	inactividad    = 2 * time.Minute
)

func avisar(estado string, extra map[string]any) {
	m := map[string]any{"estado": estado}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	fmt.Println(string(b))
}

func fallar(mensaje string) {
	avisar("error", map[string]any{"mensaje": mensaje})
	os.Exit(1)
}

func codigoValido(c string) bool {
	if len(c) != 6 {
		return false
	}
	for _, r := range c {
		if !strings.ContainsRune(alfabetoCodigo, r) {
			return false
		}
	}
	return true
}

func sufijo() string {
	b := make([]byte, 4)
	for i := range b {
		b[i] = alfabetoCodigo[rand.IntN(len(alfabetoCodigo))]
	}
	return string(b)
}

func main() {
	if len(os.Args) < 2 {
		fallar("Uso: enlace host|unirse --codigo XXXXXX")
	}
	modo := os.Args[1]
	opciones := flag.NewFlagSet(modo, flag.ExitOnError)
	codigo := opciones.String("codigo", "", "código de sala de 6 caracteres")
	puerto := opciones.Int("puerto", 24872, "puerto UDP de la sala de Azahar")
	puertoLocal := opciones.Int("local", 0, "al unirse: puerto local para Azahar (por defecto, el mismo que --puerto)")
	progresoPropio := opciones.String("progreso", "", "modo sala: archivo JSON con nuestro progreso")
	carpetaAmigos := opciones.String("amigos", "", "modo sala: carpeta donde guardar el progreso de los amigos")
	idPropio := opciones.String("id", "", "modo sala: nuestro identificador en la sala")
	puertosJuego := opciones.String("juego", "4096,56400", "modo sala: puertos de juego que van por la sala (GBA-PK, cable link de Game Boy); 0 ninguno")
	carpetaEstado := opciones.String("estado", "", "sin clave: carpeta donde se guarda la sesión de Tailscale de este dispositivo")
	_ = opciones.Parse(os.Args[2:])
	if *puertoLocal == 0 {
		*puertoLocal = *puerto
	}

	c := strings.ToLower(strings.TrimSpace(*codigo))
	if !codigoValido(c) {
		fallar("El código de sala no es válido.")
	}
	if modo != "host" && modo != "unirse" && modo != "sala" {
		fallar("Modo desconocido: " + modo)
	}

	clave := strings.TrimSpace(os.Getenv("POKEMULADOR_TS_KEY"))
	interactivo := clave == ""
	if interactivo && *carpetaEstado == "" {
		fallar("Falta la clave de Tailscale (o --estado para entrar con tu cuenta).")
	}
	if strings.HasPrefix(clave, "tskey-client-") && !strings.Contains(clave, "?") {
		clave += "?ephemeral=true&preauthorized=true"
	}

	var dir string
	if interactivo {
		dir = *carpetaEstado
		if err := os.MkdirAll(dir, 0o700); err != nil {
			fallar("No se pudo crear la carpeta de la sesión: " + err.Error())
		}
	} else {
		d, err := os.MkdirTemp("", "pokemulador-enlace-*")
		if err != nil {
			fallar("No se pudo crear la carpeta temporal: " + err.Error())
		}
		defer os.RemoveAll(d)
		dir = d
	}

	nombre := "pkm-" + c
	if modo != "host" {
		nombre += "-" + sufijo()
	}
	srv := &tsnet.Server{
		Dir:           dir,
		Hostname:      nombre,
		AuthKey:       clave,
		Ephemeral:     !interactivo,
		AdvertiseTags: []string{etiqueta},
		Logf:          func(string, ...any) {},
		UserLogf:      func(string, ...any) {},
	}
	if !interactivo {
		srv.Store = new(mem.Store)
	}
	if os.Getenv("POKEMULADOR_DEBUG") != "" {
		srv.Logf = log.Printf
		srv.UserLogf = log.Printf
	}
	defer srv.Close()

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancelar()
	// Si Pokemulador se cierra, se cierra su lado de la tubería y salimos.
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		cancelar()
	}()

	avisar("conectando", nil)
	espera := 60 * time.Second
	if interactivo {
		// Hay que esperar a que la persona entre con su cuenta (con el QR): hasta 10 minutos.
		espera = 10 * time.Minute
		if err := srv.Start(); err != nil {
			fallar("No se pudo arrancar Tailscale: " + err.Error())
		}
		go avisarLogin(ctx, srv)
	}
	ctxUp, cancelarUp := context.WithTimeout(ctx, espera)
	if _, err := srv.Up(ctxUp); err != nil {
		fallar("No se pudo conectar con Tailscale: " + err.Error())
	}
	cancelarUp()

	switch modo {
	case "host":
		anfitrion(ctx, srv, *puerto)
	case "sala":
		sala(ctx, srv, c, *puerto, *puertoLocal, puertosDeJuego(*puertosJuego), nuevosProgresos(*progresoPropio, *carpetaAmigos, *idPropio))
	default:
		unirse(ctx, srv, "pkm-"+c, *puerto, *puertoLocal)
	}
	// Un momento para que lleguen los cierres de conexión a los amigos antes de desconectarnos.
	time.Sleep(500 * time.Millisecond)
}

// anfitrion recibe a los amigos por Tailscale y reenvía su tráfico a la sala local de Azahar.
func anfitrion(ctx context.Context, srv *tsnet.Server, puerto int) {
	ip4, _ := srv.TailscaleIPs()
	pc, err := srv.ListenPacket("udp", netip.AddrPortFrom(ip4, uint16(puerto)).String())
	if err != nil {
		fallar("No se pudo abrir la sala en Tailscale: " + err.Error())
	}
	go func() {
		<-ctx.Done()
		pc.Close()
	}()
	avisar("listo", map[string]any{"ip": ip4.String()})
	reenviarSala(ctx, pc, puerto, func(n int) { avisar("amigos", map[string]any{"n": n}) })
}

// unirse busca al anfitrión por su nombre en Tailscale y abre 127.0.0.1:puerto para Azahar.
func unirse(ctx context.Context, srv *tsnet.Server, nombreAnfitrion string, puerto, puertoLocal int) {
	lc, err := srv.LocalClient()
	if err != nil {
		fallar("Tailscale no responde: " + err.Error())
	}

	avisar("buscando", nil)
	var ip netip.Addr
	limite := time.Now().Add(45 * time.Second)
	for !ip.IsValid() {
		if estado, err := lc.Status(ctx); err == nil {
			for _, p := range estado.Peer {
				if !strings.EqualFold(p.HostName, nombreAnfitrion) || !p.Online {
					continue
				}
				for _, a := range p.TailscaleIPs {
					if a.Is4() {
						ip = a
					}
				}
			}
		}
		if ip.IsValid() {
			break
		}
		if time.Now().After(limite) {
			fallar("No hay ninguna sala abierta con ese código. Comprueba que tu amigo la ha creado.")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	remoto, err := srv.Dial(ctx, "udp", netip.AddrPortFrom(ip, uint16(puerto)).String())
	if err != nil {
		fallar("No se pudo llegar a la sala: " + err.Error())
	}
	defer remoto.Close()
	local, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: puertoLocal})
	if err != nil {
		fallar(fmt.Sprintf("El puerto %d de este equipo está ocupado. Cierra otras salas de Azahar.", puertoLocal))
	}
	defer local.Close()
	go func() {
		<-ctx.Done()
		local.Close()
		remoto.Close()
	}()
	avisar("conectado", map[string]any{"ip": ip.String()})
	go informarRuta(ctx, lc, ip)

	var cliente atomic.Pointer[net.UDPAddr]
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

	buf := make([]byte, 65535)
	for {
		n, origen, err := local.ReadFromUDP(buf)
		if err != nil {
			return
		}
		cliente.Store(origen)
		_, _ = remoto.Write(buf[:n])
	}
}

// informarRuta avisa de si la conexión va directa o pasando por un repetidor de Tailscale.
func informarRuta(ctx context.Context, lc *local.Client, ip netip.Addr) {
	anterior := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
		estado, err := lc.Status(ctx)
		if err != nil {
			continue
		}
		for _, p := range estado.Peer {
			for _, a := range p.TailscaleIPs {
				if a != ip {
					continue
				}
				ruta := "repetidor"
				if p.CurAddr != "" {
					ruta = "directa"
				}
				if ruta != anterior {
					avisar("ruta", map[string]any{"tipo": ruta})
					anterior = ruta
				}
			}
		}
	}
}

// avisarLogin: sin clave, avisa de la dirección para entrar con la cuenta de Tailscale (cada vez que
// Tailscale da una nueva) hasta que la consola está dentro.
func avisarLogin(ctx context.Context, srv *tsnet.Server) {
	lc, err := srv.LocalClient()
	if err != nil {
		return
	}
	vigia, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState)
	if err != nil {
		return
	}
	defer vigia.Close()
	pedido := false
	for {
		n, err := vigia.Next()
		if err != nil {
			return
		}
		if n.BrowseToURL != nil && *n.BrowseToURL != "" {
			avisar("login", map[string]any{"url": *n.BrowseToURL})
		}
		if n.State != nil {
			switch *n.State {
			case ipn.NeedsLogin:
				if !pedido { // que Tailscale prepare la dirección para entrar
					pedido = true
					_ = lc.StartLoginInteractive(ctx)
				}
			case ipn.Running:
				return
			}
		}
	}
}
