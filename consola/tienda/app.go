package main

import (
	"context"
	"errors"
	"time"
)

type pantalla int

const (
	pLista pantalla = iota // las dos pestañas: apps y juegos
	pFicha                 // lo de uno, en grande
)

const filasVisibles = 4

// operacion: poner o quitar algo. Las hace de una en una el trabajador, por orden.
type operacion struct {
	e            *elemento
	quitar       bool
	fase         string // «En cola», «Bajando», «Descomprimiendo», «Poniendo», «Quitando»
	hecho, total int64
	empezada     bool
	ctx          context.Context
	cancel       context.CancelFunc
}

func (op *operacion) parte() float64 {
	if op.total <= 0 {
		return 0
	}
	return min(1, float64(op.hecho)/float64(op.total))
}

// dialogo: una pregunta con A (sí) y B (no).
type dialogo struct {
	titulo, texto string
	si            string
	peligro       bool
	hacer         func()
}

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	luz    *luz
	t      *tienda
	tareas chan func()
	salir  chan struct{}
	cola   chan func()

	pant pantalla
	pest int    // 0: apps, 1: juegos
	sel  [2]int // el elegido de cada pestaña
	prim [2]int // la primera fila que se ve
	dlg  *dialogo
	ops  int // en marcha o en cola

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time
	saliendo   bool

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
}

func nuevaApp(f *fuentes, t *tienda) *app {
	a := &app{f: f, l: nuevoLienzo(f), t: t, tareas: make(chan func(), 256), salir: make(chan struct{}),
		cola: make(chan func(), 64), cambios: true, luz: nuevaLuz()}
	a.cargaHasta = time.Now().Add(700 * time.Millisecond)
	t.mirar()
	for _, e := range t.apps {
		e.icono, e.imagen = t.iconoApp(e)
	}
	return a
}

func (a *app) lista() []*elemento {
	if a.pest == 1 {
		return a.t.juegos
	}
	return a.t.apps
}

func (a *app) elegido() *elemento {
	l := a.lista()
	if len(l) == 0 {
		return nil
	}
	return l[min(a.sel[a.pest], len(l)-1)]
}

// enBucle: desde otra gorrutina, que lo haga el bucle (que es quien toca los datos).
func (a *app) enBucle(fn func()) {
	select {
	case a.tareas <- fn:
	case <-a.salir:
	}
}

// empezar: el trabajador y lo que se pide al abrir (la fuente, PortMaster y las capturas).
func (a *app) empezar() {
	a.arrancarTrabajador()
	t := a.t
	go func() {
		ind, err := leerIndice(t.ctx, t.fuente)
		a.enBucle(func() {
			if ind != nil {
				t.indice = ind
			}
			t.errIndice = err
			t.mirar()
		})
	}()
	// Copias de lo que hace falta: el bucle puede cambiar los juegos mientras tanto.
	zips := make([]string, len(t.juegos))
	copias := make([]juegoPM, len(t.juegos))
	for i, e := range t.juegos {
		zips[i], copias[i] = e.juego.zip, *e.juego
	}
	go func() {
		cat, err := leerCatalogoPM(t.ctx, zips)
		a.enBucle(func() {
			t.errPM = err
			if cat != nil {
				aplicarCatalogoPM(t.juegos, cat)
				t.catalogoPM = time.Now()
			}
			t.mirar()
		})
		// Las capturas, después (son pequeñas; de una en una).
		for i, e := range t.juegos {
			g := copias[i]
			if c, ok := cat[g.zip]; ok && c.Attr.Image.Screenshot != "" {
				g.captura = c.Attr.Image.Screenshot
			}
			im, err := capturaJuego(t.ctx, g)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					logf("captura de %s: %v", g.zip, err)
				}
				continue
			}
			ic, img := iconosDe(im)
			e := e
			a.enBucle(func() { e.icono, e.imagen = ic, img })
		}
	}()
}

func (a *app) arrancarTrabajador() {
	go func() {
		for {
			select {
			case <-a.salir:
				return
			case fn := <-a.cola:
				fn()
			}
		}
	}()
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

func (a *app) repetir() {
	if a.pulsados&(bArriba|bAbajo) == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada || a.dlg != nil {
		return
	}
	a.repetirEn = time.Now().Add(110 * time.Millisecond)
	for _, b := range []uint16{bArriba, bAbajo} {
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
	if !a.cargaHasta.IsZero() {
		return
	}
	if a.dlg != nil {
		switch b {
		case bA:
			d := a.dlg
			a.dlg = nil
			d.hacer()
		case bB:
			a.dlg = nil
		}
		return
	}
	switch a.pant {
	case pLista:
		a.pulsarLista(b)
	case pFicha:
		a.pulsarFicha(b)
	}
}

func (a *app) mover(d int) {
	n := len(a.lista())
	if n == 0 {
		return
	}
	s := a.sel[a.pest] + d
	if s < 0 || s >= n {
		return
	}
	a.sel[a.pest] = s
	if s < a.prim[a.pest] {
		a.prim[a.pest] = s
	}
	if s >= a.prim[a.pest]+filasVisibles {
		a.prim[a.pest] = s - filasVisibles + 1
	}
}

func (a *app) pulsarLista(b uint16) {
	switch b {
	case bArriba:
		a.mover(-1)
	case bAbajo:
		a.mover(1)
	case bIzquierda, bL:
		a.pest = 0
	case bDerecha, bR:
		a.pest = 1
	case bA:
		if e := a.elegido(); e != nil {
			a.accion(e)
		}
	case bX, bY, bStart:
		if a.elegido() != nil {
			a.pant = pFicha
		}
	case bB:
		a.pedirSalir()
	case bSelect:
		a.luz.apagar()
	}
}

func (a *app) pulsarFicha(b uint16) {
	switch b {
	case bArriba, bIzquierda:
		a.mover(-1)
	case bAbajo, bDerecha:
		a.mover(1)
	case bA:
		if e := a.elegido(); e != nil {
			a.accion(e)
		}
	case bB, bX, bY:
		a.pant = pLista
	case bSelect:
		a.luz.apagar()
	}
}

func (a *app) pedirSalir() {
	if a.ops == 0 {
		a.terminar()
		return
	}
	a.dlg = &dialogo{
		titulo: "¿Salir de la tienda?",
		texto:  "Todavía se está poniendo algo. Si sales ahora, se para; la próxima vez sigue donde se quedó.",
		si:     "Salir", peligro: true,
		hacer: a.terminar,
	}
}

// accion: lo que hace A con uno: ponerlo, quitarlo (preguntando) o parar lo que se está haciendo.
func (a *app) accion(e *elemento) {
	switch {
	case e.op != nil:
		op := e.op
		texto := "Todavía no ha empezado: se quita de la cola."
		if op.empezada {
			texto = "Se para a medias. Si lo vuelves a poner, sigue donde se quedó."
		}
		a.dlg = &dialogo{titulo: "¿Parar " + e.nombre + "?", texto: texto, si: "Parar", peligro: true,
			hacer: func() { op.cancel() }}
	case e.puesto:
		texto := "Se quita la app y su tarjeta de la pantalla principal. Se guarda tal cual, con tu sesión, por si la vuelves a poner."
		switch {
		case e.tipo == tJuego:
			texto = "Se borra el juego con sus partidas guardadas y sus ajustes, y quedan libres " + tamTexto(a.t.ocupaJuego(e.juego)) + "."
		case e.app.servicio:
			texto = "Se quita su tarjeta de la pantalla principal. El servicio de Panther (la hora, la batería, el volumen y los avisos de mensajes) sigue en marcha."
		}
		a.dlg = &dialogo{titulo: "¿Quitar " + e.nombre + "?", texto: texto, si: "Quitar", peligro: true,
			hacer: func() { a.encolar(e, true) }}
	case !e.disponible:
		a.aviso(e.motivo)
	default:
		a.encolar(e, false)
	}
}

func (a *app) encolar(e *elemento, quitar bool) {
	t := a.t
	ctx, cancel := context.WithCancel(t.ctx)
	op := &operacion{e: e, quitar: quitar, fase: "En cola", ctx: ctx, cancel: cancel}
	e.op, e.fallo = op, ""
	var trabajo func(context.Context, avance) error
	if e.tipo == tApp {
		tr := trabajoApp{e: e, indice: t.indice}
		for _, o := range t.apps {
			if o.id == "youtube" && o.puesto {
				tr.otraYT = true
			}
		}
		trabajo = func(c context.Context, inf avance) error { return t.ponerApp(c, tr, inf) }
		if quitar {
			trabajo = func(c context.Context, inf avance) error { return t.quitarApp(c, tr, inf) }
		}
	} else {
		tr := trabajoJuego{g: *e.juego, nombre: e.nombre, larga: e.larga, genero: e.genero, imagen: e.imagen}
		trabajo = func(c context.Context, inf avance) error { return t.ponerJuego(c, tr, inf) }
		if quitar {
			trabajo = func(c context.Context, inf avance) error { return t.quitarJuego(c, tr, inf) }
		}
	}
	a.ops++
	ejecutar := func() {
		if op.ctx.Err() != nil {
			a.enBucle(func() { a.terminado(op, op.ctx.Err()) })
			return
		}
		a.enBucle(func() { op.empezada, op.fase = true, "Empezando" })
		err := trabajo(op.ctx, func(fase string, h, tt int64) {
			a.enBucle(func() { op.fase, op.hecho, op.total = fase, h, tt })
		})
		a.enBucle(func() { a.terminado(op, err) })
	}
	select {
	case a.cola <- ejecutar:
	default:
		a.ops--
		e.op = nil
		a.aviso("Hay demasiadas cosas en la cola: espera un poco")
	}
}

func (a *app) terminado(op *operacion, err error) {
	e := op.e
	if e.op == op {
		e.op = nil
	}
	a.ops--
	op.cancel()
	antes := e.puesto
	a.t.mirar()
	if e.puesto != antes {
		a.t.cambios = true // la tarjeta (o el juego de la lista) cambia
	}
	switch {
	case err == nil:
		a.t.cambios = true
		if op.quitar {
			a.aviso(e.nombre + " se ha quitado")
		} else {
			a.aviso(e.nombre + " ya está. Su tarjeta sale al salir de la tienda")
		}
	case errors.Is(err, context.Canceled):
		a.aviso("Parado: " + e.nombre)
	default:
		logf("%s %s: %v", map[bool]string{true: "quitar", false: "poner"}[op.quitar], e.id, err)
		e.fallo = mensaje(err)
		if op.quitar {
			a.aviso("No se ha podido quitar " + e.nombre + ": " + e.fallo)
		} else {
			a.aviso("No se ha podido poner " + e.nombre + ": " + e.fallo)
		}
	}
}
