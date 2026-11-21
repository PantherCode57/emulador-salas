package main

import (
	"path/filepath"
	"strings"
	"time"
)

type pantalla int

const (
	pInicio pantalla = iota // los dos botones: Cartas y Juegos
	pCartas                 // la lista de todos los Pokémon
	pJuegos                 // está por llegar
	pBuscar                 // el teclado
)

// Lo que se ve de la lista: filas de alto filaAlto desde listaArriba.
const (
	listaArriba = 78
	filaAlto    = 44
	filasLista  = 8
)

type app struct {
	f   *fuentes
	l   *lienzo
	srv *servidorPantalla
	luz *luz
	col *coleccion
	sp  *sprites

	salir chan struct{}

	pant pantalla
	menu int // en el inicio: 0 Cartas, 1 Juegos

	lista    []int  // los números que se ven en Cartas (todos o lo buscado)
	sel      int    // la fila elegida (en lista)
	primera  int    // la primera fila que se ve
	busqueda string // lo buscado (vacío: todos)
	tec      teclado
	previa   []int // lo que va saliendo mientras se escribe

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios      bool
	pulsados     uint16
	repetirEn    time.Time
	pulsadoDesde time.Time
}

func nuevaApp(f *fuentes, col *coleccion) *app {
	a := &app{f: f, l: nuevoLienzo(f), col: col, salir: make(chan struct{}), cambios: true, luz: nuevaLuz(),
		sp: nuevosSprites(filepath.Join(carpetaDatos, "sprites"))}
	a.cargaHasta = time.Now().Add(700 * time.Millisecond)
	a.lista = todos()
	return a
}

func todos() []int {
	out := make([]int, total)
	for i := range out {
		out[i] = i + 1
	}
	return out
}

func (a *app) aviso(s string) {
	a.avisoTexto = s
	a.avisoHasta = time.Now().Add(3 * time.Second)
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
	reloj := time.NewTicker(30 * time.Millisecond)
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
		case m := <-botones:
			a.botones(m)
		case r := <-a.sp.listo:
			if a.sp.recibir(r) && a.pant == pCartas {
				a.cambios = true
			}
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
	if nuevos&flechas != 0 {
		a.repetirEn = time.Now().Add(300 * time.Millisecond)
		a.pulsadoDesde = time.Now()
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

// repetir: con una flecha mantenida, se repite; en la lista, cada vez más deprisa.
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
		return
	}
	paso := 90 * time.Millisecond
	if a.pant == pCartas && time.Since(a.pulsadoDesde) > 1500*time.Millisecond {
		paso = 30 * time.Millisecond
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
	if !a.cargaHasta.IsZero() {
		a.cargaHasta = time.Time{}
	}
	switch a.pant {
	case pInicio:
		a.pulsarInicio(b)
	case pCartas:
		a.pulsarCartas(b)
	case pJuegos:
		if b == bB || b == bA {
			a.pant = pInicio
		}
	case pBuscar:
		a.pulsarBuscar(b)
	}
}

func (a *app) pulsarInicio(b uint16) {
	switch b {
	case bArriba, bAbajo:
		a.menu = 1 - a.menu
	case bA, bStart:
		if a.menu == 0 {
			a.pant = pCartas
		} else {
			a.pant = pJuegos
		}
	case bB:
		a.terminar()
	case bSelect:
		a.luz.apagar()
	}
}

func (a *app) pulsarCartas(b uint16) {
	switch b {
	case bArriba:
		a.mover(-1)
	case bAbajo:
		a.mover(1)
	case bIzquierda:
		a.mover(-filasLista)
	case bDerecha:
		a.mover(filasLista)
	case bL:
		a.saltarGeneracion(-1)
	case bR:
		a.saltarGeneracion(1)
	case bA:
		if len(a.lista) > 0 {
			a.col.cambiar(a.lista[a.sel])
		}
	case bY, bX:
		a.abrirBuscador()
	case bB:
		if a.busqueda != "" { // primero se quita lo buscado (quedándose en el mismo Pokémon)
			a.verTodos()
			return
		}
		a.pant = pInicio
	case bSelect:
		a.luz.apagar()
	}
}

// mover la fila elegida (sin dar la vuelta, salvo con un solo paso desde un extremo).
func (a *app) mover(d int) {
	n := len(a.lista)
	if n == 0 {
		return
	}
	switch {
	case d == -1 && a.sel == 0 && time.Since(a.pulsadoDesde) < 300*time.Millisecond:
		a.sel = n - 1
	case d == 1 && a.sel == n-1 && time.Since(a.pulsadoDesde) < 300*time.Millisecond:
		a.sel = 0
	default:
		a.sel = min(max(a.sel+d, 0), n-1)
	}
	a.ajustarVista()
}

func (a *app) ajustarVista() {
	if a.sel < a.primera {
		a.primera = a.sel
	}
	if a.sel >= a.primera+filasLista {
		a.primera = a.sel - filasLista + 1
	}
	a.primera = min(a.primera, max(len(a.lista)-filasLista, 0))
	a.primera = max(a.primera, 0)
}

// saltarGeneracion: R va al primero de la generación siguiente; L, al primero de esta (y si ya se
// está ahí, al de la anterior).
func (a *app) saltarGeneracion(d int) {
	if len(a.lista) == 0 {
		return
	}
	g := generacion(a.lista[a.sel])
	if d > 0 {
		for i := a.sel + 1; i < len(a.lista); i++ {
			if generacion(a.lista[i]) > g {
				a.ponerSel(i)
				return
			}
		}
		return
	}
	inicio := a.sel
	for inicio > 0 && generacion(a.lista[inicio-1]) == g {
		inicio--
	}
	if inicio < a.sel {
		a.ponerSel(inicio)
		return
	}
	if inicio == 0 {
		return
	}
	g = generacion(a.lista[inicio-1])
	i := inicio - 1
	for i > 0 && generacion(a.lista[i-1]) == g {
		i--
	}
	a.ponerSel(i)
}

// ponerSel elige la fila i y la deja arriba del todo (como al saltar de generación).
func (a *app) ponerSel(i int) {
	a.sel = i
	a.primera = i
	a.ajustarVista()
}

func (a *app) verTodos() {
	n := 0
	if len(a.lista) > 0 {
		n = a.lista[a.sel]
	}
	a.busqueda, a.lista = "", todos()
	if n > 0 {
		a.sel = n - 1
		a.primera = a.sel - filasLista/2
	}
	a.ajustarVista()
}

func (a *app) abrirBuscador() {
	a.tec.abrir(a.busqueda)
	a.previa = buscar(a.busqueda)
	a.pant = pBuscar
}

func (a *app) pulsarBuscar(b uint16) {
	switch a.tec.pulsar(b) {
	case tecCambia:
		a.previa = buscar(a.tec.texto())
	case tecBuscar:
		q := strings.TrimSpace(a.tec.texto())
		if normalizar(q) == "" {
			if a.busqueda != "" {
				a.verTodos()
			}
			a.pant = pCartas
			return
		}
		res := buscar(q)
		if len(res) == 0 {
			a.aviso("No hay ningún Pokémon con «" + q + "»")
			return
		}
		a.busqueda, a.lista, a.sel, a.primera = q, res, 0, 0
		a.pant = pCartas
	case tecCancelar:
		a.pant = pCartas
	}
}
