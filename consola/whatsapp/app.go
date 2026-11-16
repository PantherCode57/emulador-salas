package main

import (
	"net"
	"time"
)

// Pantallas de la app.
type pantalla int

const (
	pChats pantalla = iota
	pChat
	pTeclado
	pRapidas
	pMenu
	pVisor // fotos y stickers a pantalla completa
)

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	w      *wa
	media  *medios
	av     *avatares
	tareas chan func()
	salir  chan struct{}

	estado estadoWA
	qr     string

	pant      pantalla
	selChat   int // fila elegida en la lista de chats
	primera   int // primera fila visible
	abierto   *Chat
	desplazar int // líneas subidas en el chat (0 = abajo del todo)
	selMenu   int
	confirmar bool // segundo paso para desvincular
	selRapida int
	rapidas   []string
	visor     []*Mensaje // fotos y stickers del chat abierto
	visorI    int

	tec teclado

	avisoTexto string
	avisoHasta time.Time

	// Pantalla de carga al abrir (ver enCarga).
	cargaMin, cargaMax time.Time
	enCargaAntes       bool

	cambios bool

	// Botones: los que estaban pulsados y cuándo se repiten (para mantener pulsadas las flechas).
	pulsados  uint16
	repetirEn time.Time

	// Como servicio (ver servicio.go).
	modo            modoPantalla
	capa, panel     *capaCompartida
	mando           *mando
	mandoAntes      uint32
	esperaSoltar    bool // el chat de encima espera a que sueltes los botones para abrirse o cerrarse
	cerrando        bool
	botonesCore     chan uint16
	ra              *retroarch
	cliente         net.Conn
	panelSucio      bool
	panelHecho      time.Time
	ultimoAviso     *Chat
	ultimoAvisoHora time.Time
	// El recuadro de arriba, el volumen y las grabaciones (barra.go, captura.go).
	barra, volumen *capaCompartida
	barraVista     estadoBarra
	grab           *grabacion
	teclasVol      <-chan int
}

func nuevaApp(f *fuentes) *app {
	a := &app{f: f, l: nuevoLienzo(f), tareas: make(chan func(), 256), salir: make(chan struct{}), cambios: true}
	a.av = nuevosAvatares(a)
	a.cargaMin, a.cargaMax = time.Now().Add(time.Second), time.Now().Add(6*time.Second)
	return a
}

// hacer pide que algo se haga en el bucle de la app (desde otro hilo).
func (a *app) hacer(fn func()) {
	select {
	case a.tareas <- fn:
	case <-a.salir:
	}
}

func (a *app) redibujar() { a.cambios = true }

func (a *app) aviso(s string) {
	a.avisoTexto = s
	a.avisoHasta = time.Now().Add(4 * time.Second)
	a.cambios = true
}

func (a *app) chatAbierto() string {
	if a.modo == modoFondo || a.esperaSoltar && a.cerrando {
		return "" // no se está viendo: lo que llegue queda sin leer
	}
	if a.abierto != nil && (a.pant == pChat || a.pant == pTeclado || a.pant == pRapidas || a.pant == pVisor) {
		return a.abierto.JID
	}
	return ""
}

func (a *app) terminar() {
	select {
	case <-a.salir:
	default:
		close(a.salir)
	}
}

// bucle: todo lo que cambia la app pasa por aquí, en un solo hilo.
func (a *app) bucle(botones <-chan uint16, ra *retroarch) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	guardado := time.NewTicker(5 * time.Second)
	defer guardado.Stop()
	var raHecho <-chan error
	if ra != nil {
		raHecho = ra.hecho
	}
	for {
		select {
		case <-a.salir:
			return
		case err := <-raHecho:
			logf("RetroArch se ha cerrado (%v): salgo", err)
			a.terminar()
			return
		case fn := <-a.tareas:
			fn()
			a.cambios = true // cualquier novedad (QR, mensajes, estado) se enseña enseguida
		case m := <-botones:
			a.botones(m)
		case <-reloj.C:
			a.repetir()
			if c := a.enCarga(); c != a.enCargaAntes { // se acaba la pantalla de carga
				a.enCargaAntes, a.cambios = c, true
			}
			if !a.avisoHasta.IsZero() && time.Now().After(a.avisoHasta) {
				a.avisoHasta = time.Time{}
				a.avisoTexto = ""
				a.cambios = true
			}
		case <-guardado.C:
			if a.w != nil {
				a.w.guardar()
			}
		}
		if a.cambios {
			a.cambios = false
			a.dibujar()
			if a.srv != nil {
				a.srv.enviar(a.l.marco())
			}
		}
	}
}

const flechas = bArriba | bAbajo | bIzquierda | bDerecha

func (a *app) botones(m uint16) {
	nuevos := m &^ a.pulsados
	a.pulsados = m
	if m&flechas != 0 {
		a.repetirEn = time.Now().Add(350 * time.Millisecond)
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

// repetir: con una flecha mantenida, se repite cada 70 ms.
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) {
		return
	}
	a.repetirEn = time.Now().Add(70 * time.Millisecond)
	for _, b := range []uint16{bArriba, bAbajo, bIzquierda, bDerecha} {
		if a.pulsados&b != 0 {
			a.pulsar(b)
		}
	}
}

func (a *app) pulsar(b uint16) {
	a.cambios = true
	if a.modo == modoCapa && (a.esperaSoltar || b == bB && (a.pant == pChat || a.pant == pChats)) {
		// Encima del juego, B vuelve a él (desde el chat o desde la lista).
		if !a.esperaSoltar {
			a.cerrarCapa()
		}
		return
	}
	if a.estado != waConectado && a.estado != waSinRed && a.estado != waConectando && a.pant != pMenu {
		// Sin sesión: solo la pantalla del QR.
		switch b {
		case bA:
			if a.estado == waQRCaducado || a.estado == waDesvinculado {
				a.w.otraVez()
			}
		case bB, bStart:
			a.pant, a.selMenu, a.confirmar = pMenu, 0, false
		}
		return
	}
	switch a.pant {
	case pChats:
		a.pulsarChats(b)
	case pChat:
		a.pulsarChat(b)
	case pTeclado:
		a.pulsarTeclado(b)
	case pRapidas:
		a.pulsarRapidas(b)
	case pMenu:
		a.pulsarMenu(b)
	case pVisor:
		a.pulsarVisor(b)
	}
}

func (a *app) pulsarVisor(b uint16) {
	switch b {
	case bIzquierda, bArriba, bL:
		a.visorI = max(a.visorI-1, 0)
	case bDerecha, bAbajo, bR:
		a.visorI = min(a.visorI+1, len(a.visor)-1)
	case bB, bY, bStart:
		a.pant = pChat
	}
}

const filasVisibles = 6

func (a *app) pulsarChats(b uint16) {
	lista := a.w.visibles()
	switch b {
	case bArriba:
		a.selChat = max(a.selChat-1, 0)
	case bAbajo:
		a.selChat = min(a.selChat+1, max(len(lista)-1, 0))
	case bIzquierda, bL:
		a.selChat = max(a.selChat-filasVisibles, 0)
	case bDerecha, bR:
		a.selChat = min(a.selChat+filasVisibles, max(len(lista)-1, 0))
	case bA:
		if a.selChat < len(lista) {
			a.abrir(lista[a.selChat])
		}
	case bStart, bB:
		a.pant, a.selMenu, a.confirmar = pMenu, 0, false
	}
}

func (a *app) abrir(c *Chat) {
	a.abierto, a.pant, a.desplazar = c, pChat, 0
	a.w.pedirAntiguas(c) // si tiene fotos de antes de que se pudieran ver, se piden otra vez
	if c.NoLeidos > 0 {
		n := min(c.NoLeidos, len(c.Mensajes))
		a.w.leido(c, c.Mensajes[len(c.Mensajes)-n:])
		c.NoLeidos = 0
		a.w.sucio = true
	}
}

func (a *app) pulsarChat(b uint16) {
	switch b {
	case bArriba:
		a.desplazar += 2
	case bAbajo:
		a.desplazar = max(a.desplazar-2, 0)
	case bL, bIzquierda:
		a.desplazar += 10
	case bR, bDerecha:
		a.desplazar = max(a.desplazar-10, 0)
	case bA:
		a.tec.abrir()
		a.pant = pTeclado
	case bX:
		a.rapidas = cargarRapidas()
		a.selRapida = 0
		a.pant = pRapidas
	case bY: // fotos y stickers del chat, empezando por el último
		if a.abierto != nil && a.media != nil {
			if a.visor = mediosDe(a.abierto); len(a.visor) > 0 {
				a.visorI = len(a.visor) - 1
				a.pant = pVisor
			}
		}
	case bB:
		a.pant = pChats
	case bStart:
		a.pant, a.selMenu, a.confirmar = pMenu, 0, false
	}
}

func (a *app) pulsarTeclado(b uint16) {
	switch a.tec.pulsar(b) {
	case tecEnviar:
		if a.abierto != nil {
			a.w.enviar(a.abierto, a.tec.texto())
		}
		a.tec.borrarTodo()
		a.pant, a.desplazar = pChat, 0
	case tecCancelar:
		a.pant = pChat // el borrador se queda
	}
}

func (a *app) pulsarRapidas(b uint16) {
	switch b {
	case bArriba:
		a.selRapida = max(a.selRapida-1, 0)
	case bAbajo:
		a.selRapida = min(a.selRapida+1, len(a.rapidas)-1)
	case bA:
		if a.abierto != nil && a.selRapida < len(a.rapidas) {
			a.w.enviar(a.abierto, a.rapidas[a.selRapida])
		}
		a.pant, a.desplazar = pChat, 0
	case bB, bX:
		a.pant = pChat
	}
}

// Opciones del menú.
var opcionesMenu = []string{"Seguir", "Desvincular este dispositivo", "Salir de WhatsApp"}

func (a *app) pulsarMenu(b uint16) {
	switch b {
	case bArriba:
		a.selMenu, a.confirmar = max(a.selMenu-1, 0), false
	case bAbajo:
		a.selMenu, a.confirmar = min(a.selMenu+1, len(opcionesMenu)-1), false
	case bB, bStart:
		a.pant = pChats
	case bA:
		switch a.selMenu {
		case 0:
			a.pant = pChats
		case 1:
			if !a.confirmar {
				a.confirmar = true
				return
			}
			a.w.desvincular()
			a.pant, a.selChat, a.abierto = pChats, 0, nil
		case 2:
			a.salirPantalla()
		}
	}
}
