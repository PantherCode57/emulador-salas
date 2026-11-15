// Partida compartida (GBA-PK) por la sala.
//
// GBA-PK es un script de mGBA en el que un jugador abre una "partida" (escucha en TCP, puerto
// 4096) y los demás se conectan a ella. Así os veis andando por el mismo mapa.
//
// El que tiene la sala abierta también abre la partida en su emulador. Los amigos se conectan a
// 127.0.0.1:4096 en su propio dispositivo: el enlace recoge esa conexión y la lleva por Tailscale
// al puerto de la sala del anfitrión, presentándose con saludoJuego para distinguirla del canal
// de progreso. El anfitrión la entrega a la partida de su emulador (127.0.0.1:4096).
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

const saludoJuego = "PKMJUEGO1\n"

// conLector es una conexión de la que ya se han leído (o espiado) bytes con un bufio.Reader.
type conLector struct {
	net.Conn
	r *bufio.Reader
}

func (c conLector) Read(b []byte) (int, error) { return c.r.Read(b) }

// clasificarEntrada mira qué es una conexión que llega a la sala: la partida compartida de un
// amigo o el canal de progreso de siempre.
func clasificarEntrada(ctx context.Context, c net.Conn, puertoJuego int, progreso func(net.Conn)) {
	lector := bufio.NewReaderSize(c, 64<<10)
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	inicio, err := lector.Peek(len(saludoJuego))
	_ = c.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return
	}
	if string(inicio) == saludoJuego {
		_, _ = lector.Discard(len(saludoJuego))
		entregarJuego(ctx, conLector{c, lector}, puertoJuego)
		return
	}
	progreso(conLector{c, lector})
}

// entregarJuego lleva la conexión de un amigo a la partida de nuestro emulador.
func entregarJuego(ctx context.Context, remoto net.Conn, puertoJuego int) {
	defer remoto.Close()
	if puertoJuego <= 0 {
		return
	}
	d := net.Dialer{Timeout: 3 * time.Second}
	local, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", puertoJuego))
	if err != nil {
		return // nuestro juego aún no está abierto: el amigo lo volverá a intentar
	}
	unir(ctx, remoto, local)
}

// servirJuegoLocal deja que el emulador de este dispositivo se conecte a la partida del
// anfitrión como si estuviera en 127.0.0.1. Dura mientras estemos conectados a la sala.
func servirJuegoLocal(ctx context.Context, srv *tsnet.Server, anfitrion netip.Addr, puertoSala, puertoJuego int) {
	if puertoJuego <= 0 {
		return
	}
	var ln net.Listener
	for intento := 0; ; intento++ {
		var err error
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", puertoJuego))
		if err == nil {
			break
		}
		if intento >= 10 {
			avisar("aviso", map[string]any{"mensaje": fmt.Sprintf("El puerto %d está ocupado: no se podrá ver al amigo en el juego.", puertoJuego)})
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
			if _, err := io.WriteString(remoto, saludoJuego); err != nil {
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
