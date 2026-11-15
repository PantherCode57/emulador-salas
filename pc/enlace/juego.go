// Conexiones de juego por la sala: la partida compartida de GBA (GBA-PK, puerto 4096) y el cable
// link de Game Boy (Gambatte, puerto 56400).
//
// En los dos casos un emulador hace de "servidor" (escucha en TCP) y los demás se conectan a él.
// El que tiene la sala abierta es el servidor. Los amigos se conectan a 127.0.0.1:<puerto> en su
// propio dispositivo: el enlace recoge esa conexión y la lleva por Tailscale al puerto de la sala
// del anfitrión, presentándose con un saludo que dice a qué puerto va (así se distingue del canal
// de progreso). El anfitrión la entrega a su emulador (127.0.0.1:<puerto>).
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

// Saludo: "PKMJ" + puerto con 5 cifras + salto de línea. El de la primera versión (solo GBA-PK)
// era "PKMJUEGO1\n", que mide lo mismo.
const (
	largoSaludo   = 10
	saludoAntiguo = "PKMJUEGO1\n"
)

func saludoPara(puerto int) string { return fmt.Sprintf("PKMJ%05d\n", puerto) }

// conLector es una conexión de la que ya se han leído (o espiado) bytes con un bufio.Reader.
type conLector struct {
	net.Conn
	r *bufio.Reader
}

func (c conLector) Read(b []byte) (int, error) { return c.r.Read(b) }

// puertoJuego: en qué puerto de este dispositivo escucha el amigo (local) y a qué puerto del
// anfitrión va (remoto). Normalmente son el mismo; distintos solo para probar en un mismo equipo.
type puertoJuego struct{ local, remoto int }

// puertosDeJuego lee la lista de --juego ("4096,56400", o "14096:4096" para probar). 0: ninguno.
func puertosDeJuego(texto string) []puertoJuego {
	var puertos []puertoJuego
	for _, trozo := range strings.Split(texto, ",") {
		partes := strings.SplitN(strings.TrimSpace(trozo), ":", 2)
		local, err := strconv.Atoi(partes[0])
		if err != nil || local <= 0 || local > 65535 {
			continue
		}
		remoto := local
		if len(partes) == 2 {
			if remoto, err = strconv.Atoi(partes[1]); err != nil || remoto <= 0 || remoto > 65535 {
				continue
			}
		}
		puertos = append(puertos, puertoJuego{local, remoto})
	}
	return puertos
}

func admite(lista []puertoJuego, puerto int) bool {
	for _, p := range lista {
		if p.remoto == puerto {
			return true
		}
	}
	return false
}

// clasificarEntrada mira qué es una conexión que llega a la sala: un juego de un amigo o el canal
// de progreso de siempre.
func clasificarEntrada(ctx context.Context, c net.Conn, puertos []puertoJuego, progreso func(net.Conn)) {
	lector := bufio.NewReaderSize(c, 64<<10)
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	inicio, err := lector.Peek(largoSaludo)
	_ = c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return
	}
	saludo := string(inicio)
	if strings.HasPrefix(saludo, "PKMJ") {
		_, _ = lector.Discard(largoSaludo)
		puerto := 4096
		if saludo != saludoAntiguo {
			puerto, err = strconv.Atoi(strings.TrimSpace(saludo[4:]))
		}
		if err != nil || !admite(puertos, puerto) {
			c.Close()
			return
		}
		entregarJuego(ctx, conLector{c, lector}, puerto)
		return
	}
	progreso(conLector{c, lector})
}

// entregarJuego lleva la conexión de un amigo al emulador de este dispositivo.
func entregarJuego(ctx context.Context, remoto net.Conn, puerto int) {
	defer remoto.Close()
	d := net.Dialer{Timeout: 3 * time.Second}
	local, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", puerto))
	if err != nil {
		return // nuestro juego aún no está abierto: el amigo lo volverá a intentar
	}
	unir(ctx, remoto, local)
}

// servirJuegosLocales abre en este dispositivo los puertos de juego que llevan al anfitrión.
// Duran mientras estemos conectados a la sala.
func servirJuegosLocales(ctx context.Context, srv *tsnet.Server, anfitrion netip.Addr, puertoSala int, puertos []puertoJuego) {
	for _, p := range puertos {
		go servirJuegoLocal(ctx, srv, anfitrion, puertoSala, p)
	}
}

func servirJuegoLocal(ctx context.Context, srv *tsnet.Server, anfitrion netip.Addr, puertoSala int, puerto puertoJuego) {
	var ln net.Listener
	for intento := 0; ; intento++ {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", puerto.local))
		if err == nil {
			break
		}
		if intento >= 10 {
			avisar("aviso", map[string]any{"mensaje": fmt.Sprintf("El puerto %d está ocupado: ese juego no podrá conectar con tu amigo.", puerto.local)})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		local, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer local.Close()
			ctxDial, cancelar := context.WithTimeout(ctx, 8*time.Second)
			remoto, err := srv.Dial(ctxDial, "tcp", netip.AddrPortFrom(anfitrion, uint16(puertoSala)).String())
			cancelar()
			if err != nil {
				return
			}
			if _, err := io.WriteString(remoto, saludoPara(puerto.remoto)); err != nil {
				remoto.Close()
				return
			}
			unir(ctx, local, remoto)
		}()
	}
}

// unir copia en los dos sentidos hasta que uno de los lados se cierra.
func unir(ctx context.Context, a, b net.Conn) {
	ctxUnion, cancelar := context.WithCancel(ctx)
	defer cancelar()
	var una sync.Once
	cerrar := func() {
		una.Do(func() {
			a.Close()
			b.Close()
		})
	}
	go func() {
		<-ctxUnion.Done()
		cerrar()
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(a, b)
		cerrar()
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(b, a)
		cerrar()
	}()
	wg.Wait()
}
