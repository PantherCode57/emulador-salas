package main

import (
	"strings"
	"time"
)

type pantalla int

const (
	pChat     pantalla = iota // la conversación
	pEscribir                 // el teclado
)

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	luz    *luz
	ch     *chat
	tareas chan func()
	salir  chan struct{}

	pant   pantalla
	tec    teclado
	scroll int // píxeles por encima del final (0 = lo último)
	maxScr int // lo que se puede subir (lo calcula el dibujo)
	estado estadoChat

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
}

func nuevaApp(f *fuentes, ch *chat) *app {
	a := &app{f: f, l: nuevoLienzo(f), ch: ch, tareas: make(chan func(), 64), salir: make(chan struct{}), cambios: true, luz: nuevaLuz()}
	a.cargaHasta = time.Now().Add(700 * time.Millisecond)
	a.estado = ch.estado()
	return a
}

func (a *app) aviso(s string) {
	a.avisoTexto = s
	a.avisoHasta = time.Now().Add(3500 * time.Millisecond)
	a.cambios = true
}

func (a *app) terminar() {
	select {
	case <-a.salir:
	default:
		close(a.salir)
	}
}

func (a *app) bucle(botones <-chan uint16, ra *retroarch) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	mirar := time.NewTicker(500 * time.Millisecond)
	defer mirar.Stop()
	var raHecho <-chan error
	if ra != nil {
		raHecho = ra.hecho
	}
	for {
		select {
		case <-a.salir:
			return
		case err := <-raHecho:
			logf("la pantalla se ha cerrado (%v): salgo", err)
			a.terminar()
			return
		case fn := <-a.tareas:
			fn()
			a.cambios = true
		case m := <-botones:
			a.botones(m)
		case <-reloj.C:
			a.repetir()
			if !a.avisoHasta.IsZero() && time.Now().After(a.avisoHasta) {
				a.avisoHasta, a.avisoTexto, a.cambios = time.Time{}, "", true
			}
			if !a.cargaHasta.IsZero() && time.Now().After(a.cargaHasta) {
				a.cargaHasta, a.cambios = time.Time{}, true
			}
		case <-mirar.C:
			if a.ch.recoger() {
				a.scroll = 0 // a lo nuevo
				a.cambios = true
				if a.luz.apagada {
					a.luz.encender()
				}
			}
			if e := a.ch.estado(); e != a.estado {
				a.estado, a.cambios = e, true
			}
			if a.estado == estEscribiendo || a.estado == estEnviado {
				a.cambios = true // los puntitos
			}
		}
		if a.cambios && !a.luz.apagada {
			a.cambios = false
			a.dibujar()
			a.srv.enviarCon(a.l.marcoEn)
		}
	}
}

const flechas = bArriba | bAbajo | bIzquierda | bDerecha

func (a *app) botones(m uint16) {
	nuevos := m &^ a.pulsados
	a.pulsados = m
	if m&flechas != 0 {
		a.repetirEn = time.Now().Add(300 * time.Millisecond)
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

// repetir: con una flecha mantenida, se repite (en el chat, más rápido: se desliza).
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
		return
	}
	paso := 90 * time.Millisecond
	if a.pant == pChat {
		paso = 40 * time.Millisecond
	}
	a.repetirEn = time.Now().Add(paso)
	for _, b := range []uint16{bArriba, bAbajo, bIzquierda, bDerecha} {
		if a.pulsados&b != 0 {
			a.pulsar(b)
		}
	}
}

func (a *app) pulsar(b uint16) {
	a.cambios = true
	if a.luz.apagada {
		a.luz.encender()
		return
	}
	switch a.pant {
	case pChat:
		a.pulsarChat(b)
	case pEscribir:
		a.pulsarEscribir(b)
	}
}

func (a *app) pulsarChat(b uint16) {
	switch b {
	case bArriba:
		a.scroll = min(a.scroll+36, a.maxScr)
	case bAbajo:
		a.scroll = max(a.scroll-36, 0)
	case bIzquierda, bL:
		a.scroll = min(a.scroll+300, a.maxScr)
	case bDerecha, bR:
		a.scroll = max(a.scroll-300, 0)
	case bA, bStart:
		a.pant = pEscribir
		a.tec.abrir()
	case bB:
		a.terminar()
	case bSelect:
		a.luz.apagar()
	}
}

func (a *app) pulsarEscribir(b uint16) {
	switch a.tec.pulsar(b) {
	case tecEnviar:
		texto := strings.TrimSpace(a.tec.texto())
		if texto == "" {
			a.aviso("Escribe algo primero")
			return
		}
		a.ch.enviar(texto)
		a.tec.borrarTodo()
		a.pant, a.scroll = pChat, 0
		a.estado = a.ch.estado()
		if a.estado == estDormido {
			a.aviso("Claude no escucha ahora: le llegará cuando vuelva")
		}
	case tecCancelar:
		a.pant = pChat
	}
}
