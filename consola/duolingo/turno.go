package main

import (
	"image"
	"strings"
	"time"
)

// turno: lo que estás haciendo en el ejercicio de ahora.
type turno struct {
	ej       *ejercicio
	foco     int   // la opción o ficha donde está la cruceta (-1: el botón de comprobar)
	elegida  int   // ejElegir: la marcada (-1: ninguna)
	fichas   []int // ejBanco y ejHuecos: las puestas, en orden
	escribir bool  // ejEscribir: el teclado está abierto

	// ejParejas
	ordIzq, ordDer []int // en qué orden salen
	col            int   // 0 izquierda, 1 derecha
	selIzq, selDer int   // la elegida en cada columna (-1)
	hechas         map[int]bool
	malIzq, malDer int // la pareja que no iba (se pinta en rojo un momento)
	malHasta       time.Time

	comprobado bool
	ver        veredicto
	desde      time.Time
}

func (a *app) nuevoTurno() {
	e := a.lec.actual()
	if e == nil {
		a.acabar()
		return
	}
	a.t = &turno{ej: e, elegida: -1, selIzq: -1, selDer: -1, malIzq: -1, malDer: -1, desde: time.Now()}
	a.tec.borrarTodo()
	if e.Tipo == ejParejas {
		a.t.ordIzq, a.t.ordDer, a.t.hechas = barajar(len(e.Izquierda)), barajar(len(e.Derecha)), map[int]bool{}
	}
	if e.Tipo == ejEscribir {
		a.t.escribir = true
		a.tec.abrir()
	}
	a.son.parar()
	a.son.decir(e.Audio)
}

func (a *app) salirDeLeccion() {
	a.confirmar("¿Salir de la lección? Lo que llevas no contará.", func() {
		a.son.parar()
		a.son.olvidar()
		a.lec, a.t = nil, nil
		a.pant = pInicio
	})
}

func (a *app) pulsarLeccion(b uint16) {
	t := a.t
	if t == nil {
		return
	}
	e := t.ej
	if t.comprobado {
		switch b {
		case bA, bStart:
			a.continuar()
		case bX:
			a.son.decir(e.Audio)
		case bB:
			a.salirDeLeccion()
		}
		return
	}
	if b == bX && e.Tipo != ejEscribir {
		a.son.decir(e.Audio)
		return
	}
	if b == bY && e.AudioLento != "" && e.Tipo != ejEscribir {
		a.son.decir(e.AudioLento)
		return
	}
	switch e.Tipo {
	case ejElegir:
		a.pulsarElegir(b)
	case ejBanco, ejHuecos:
		a.pulsarFichas(b)
	case ejParejas:
		a.pulsarParejas(b)
	case ejEscribir:
		a.pulsarEscribirEj(b)
	}
}

// columnasOpciones: las opciones van en dos columnas si son cortas y pocas; si no, una debajo de otra.
func (a *app) columnasOpciones(e *ejercicio) int {
	if len(e.Opciones) > 6 {
		return 2
	}
	for _, o := range e.Opciones {
		if medir(a.f.nombre, o.Texto) > 230 {
			return 1
		}
	}
	if len(e.Opciones) >= 4 {
		return 2
	}
	return 1
}

func (a *app) pulsarElegir(b uint16) {
	t, e := a.t, a.t.ej
	n := len(e.Opciones)
	cols := a.columnasOpciones(e)
	switch b {
	case bArriba:
		if t.foco < 0 {
			t.foco = max(0, n-1)
		} else if t.foco-cols >= 0 {
			t.foco -= cols
		}
	case bAbajo:
		if t.foco >= 0 {
			if t.foco+cols < n {
				t.foco += cols
			} else if t.elegida >= 0 {
				t.foco = -1
			}
		}
	case bIzquierda:
		if t.foco > 0 && cols == 2 && t.foco%2 == 1 {
			t.foco--
		}
	case bDerecha:
		if t.foco >= 0 && cols == 2 && t.foco%2 == 0 && t.foco+1 < n {
			t.foco++
		}
	case bA:
		if t.foco < 0 || t.foco == t.elegida {
			a.comprobar()
			return
		}
		t.elegida = t.foco
		a.son.decir(e.Opciones[t.foco].Audio)
	case bStart:
		a.comprobar()
	case bB:
		if t.elegida >= 0 {
			t.elegida = -1
		} else {
			a.salirDeLeccion()
		}
	}
}

// Las fichas: se colocan en filas de izquierda a derecha (la geometría la usan el dibujo y la cruceta).
const (
	altoFicha = 42
	sepFicha  = 10
)

func (a *app) colocarFichas(e *ejercicio, zona image.Rectangle) []image.Rectangle {
	var rs []image.Rectangle
	x, y := zona.Min.X, zona.Min.Y
	filas := [][]int{{}}
	for i, o := range e.Opciones {
		w := medir(a.f.nombre, a.f.limpiar(o.Texto)) + 30
		w = min(max(w, 52), zona.Dx())
		if x+w > zona.Max.X && x > zona.Min.X {
			x, y = zona.Min.X, y+altoFicha+sepFicha
			filas = append(filas, []int{})
		}
		rs = append(rs, image.Rect(x, y, x+w, y+altoFicha))
		filas[len(filas)-1] = append(filas[len(filas)-1], i)
		x += w + sepFicha
	}
	// cada fila, centrada
	for _, f := range filas {
		if len(f) == 0 {
			continue
		}
		sobra := zona.Max.X - rs[f[len(f)-1]].Max.X
		for _, i := range f {
			rs[i] = rs[i].Add(image.Pt(sobra/2, 0))
		}
	}
	return rs
}

func (a *app) zonaFichas() image.Rectangle { return image.Rect(30, 262, ancho-30, alto-60) }

// vecina: la ficha más cercana hacia arriba (dy < 0) o hacia abajo (dy > 0).
func vecina(rs []image.Rectangle, i, dy int) int {
	if i < 0 || i >= len(rs) {
		return i
	}
	c := rs[i]
	mejor, md := -1, 1<<30
	for j, r := range rs {
		if (dy < 0 && r.Min.Y >= c.Min.Y) || (dy > 0 && r.Min.Y <= c.Min.Y) {
			continue
		}
		d := abs(r.Min.Y-c.Min.Y)*4 + abs((r.Min.X+r.Max.X)/2-(c.Min.X+c.Max.X)/2)
		if d < md {
			mejor, md = j, d
		}
	}
	if mejor < 0 {
		return i
	}
	return mejor
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (t *turno) puesta(i int) bool {
	for _, f := range t.fichas {
		if f == i {
			return true
		}
	}
	return false
}

func (a *app) pulsarFichas(b uint16) {
	t, e := a.t, a.t.ej
	rs := a.colocarFichas(e, a.zonaFichas())
	switch b {
	case bIzquierda:
		if t.foco > 0 {
			t.foco--
		}
	case bDerecha:
		if t.foco < len(rs)-1 {
			t.foco++
		}
	case bArriba:
		t.foco = vecina(rs, t.foco, -1)
	case bAbajo:
		t.foco = vecina(rs, t.foco, 1)
	case bA:
		if t.foco < 0 || t.foco >= len(e.Opciones) || t.puesta(t.foco) {
			return
		}
		if e.Tipo == ejHuecos && len(t.fichas) >= len(e.Correctas) {
			a.aviso("Ya están todos los huecos: START para comprobar")
			return
		}
		t.fichas = append(t.fichas, t.foco)
		a.son.decir(e.Opciones[t.foco].Audio)
		// la cruceta pasa a la siguiente que quede libre
		for k := 1; k < len(e.Opciones); k++ {
			j := (t.foco + k) % len(e.Opciones)
			if !t.puesta(j) {
				t.foco = j
				break
			}
		}
	case bB:
		if len(t.fichas) > 0 {
			t.foco = t.fichas[len(t.fichas)-1]
			t.fichas = t.fichas[:len(t.fichas)-1]
		} else {
			a.salirDeLeccion()
		}
	case bL:
		t.fichas = nil
	case bStart:
		a.comprobar()
	}
}

func (a *app) pulsarParejas(b uint16) {
	t, e := a.t, a.t.ej
	n := len(e.Izquierda)
	fila := func(col int) int { // en qué fila está la cruceta
		if col == 0 {
			return t.foco % max(n, 1)
		}
		return t.foco % max(n, 1)
	}
	f := fila(t.col)
	switch b {
	case bArriba:
		if f > 0 {
			t.foco = f - 1
		}
	case bAbajo:
		if f < n-1 {
			t.foco = f + 1
		}
	case bIzquierda:
		t.col = 0
	case bDerecha:
		t.col = 1
	case bA:
		if t.col == 0 {
			id := t.ordIzq[f]
			if t.hechas[id] {
				return
			}
			t.selIzq = id
			a.son.decir(e.Izquierda[id].Audio)
			if t.selDer < 0 {
				t.col = 1 // ahora la de la derecha
			}
		} else {
			id := t.ordDer[f]
			if t.hechas[id] {
				return
			}
			t.selDer = id
			if t.selIzq < 0 {
				t.col = 0
			}
		}
		if t.selIzq >= 0 && t.selDer >= 0 {
			if t.selIzq == t.selDer || normalizar(e.Derecha[t.selIzq].Texto) == normalizar(e.Derecha[t.selDer].Texto) {
				t.hechas[t.selIzq] = true
				if t.selIzq != t.selDer {
					t.hechas[t.selDer] = true
				}
				a.son.aviso(true)
			} else {
				t.malIzq, t.malDer, t.malHasta = t.selIzq, t.selDer, time.Now().Add(600*time.Millisecond)
				a.son.aviso(false)
			}
			t.selIzq, t.selDer = -1, -1
			if len(t.hechas) >= n {
				t.comprobado, t.ver = true, veredicto{Bien: true}
				a.lec.responder(true)
				a.lec.apuntar(e, nil, true, int(time.Since(t.desde).Milliseconds()))
			}
		}
	case bB:
		if t.selIzq >= 0 || t.selDer >= 0 {
			t.selIzq, t.selDer = -1, -1
		} else {
			a.salirDeLeccion()
		}
	}
}

func (a *app) pulsarEscribirEj(b uint16) {
	switch a.tec.pulsar(b) {
	case tecEnviar:
		a.comprobar()
	case tecCancelar:
		a.salirDeLeccion()
	}
}

// respuestaDe: lo que has contestado, en texto (para comprobarlo y para mandarlo a Duolingo).
func (a *app) respuestaDe(t *turno) string {
	e := t.ej
	switch e.Tipo {
	case ejElegir:
		if t.elegida >= 0 {
			return e.Opciones[t.elegida].Texto
		}
	case ejBanco, ejHuecos:
		var ps []string
		for _, i := range t.fichas {
			ps = append(ps, e.Opciones[i].Texto)
		}
		return unirFichas(ps)
	case ejEscribir:
		return a.tec.texto()
	}
	return ""
}

// unirFichas: las palabras con un espacio, menos antes de los signos («Hola , Ana» → «Hola, Ana»).
func unirFichas(ps []string) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 && !strings.ContainsAny(p[:1], ".,!?;:") && !strings.HasSuffix(ps[i-1], "'") {
			b.WriteByte(' ')
		}
		b.WriteString(p)
	}
	return b.String()
}

func (a *app) comprobar() {
	t, e := a.t, a.t.ej
	var v veredicto
	switch e.Tipo {
	case ejElegir:
		if t.elegida < 0 {
			a.aviso("Elige una con A")
			return
		}
		v.Bien = t.elegida == e.Correcta ||
			(e.Correcta >= 0 && e.Correcta < len(e.Opciones) && normalizar(e.Opciones[t.elegida].Texto) == normalizar(e.Opciones[e.Correcta].Texto))
	case ejBanco:
		if len(t.fichas) == 0 {
			a.aviso("Pon las fichas con A")
			return
		}
		v = comprobarFrase(a.respuestaDe(t), e.Soluciones, false)
	case ejHuecos:
		if len(t.fichas) < len(e.Correctas) {
			a.aviso("Faltan huecos por rellenar")
			return
		}
		v.Bien = true
		for k, i := range t.fichas {
			if i != e.Correctas[k] && normalizar(e.Opciones[i].Texto) != normalizar(e.Opciones[e.Correctas[k]].Texto) {
				v.Bien = false
			}
		}
	case ejEscribir:
		if strings.TrimSpace(a.tec.texto()) == "" {
			a.aviso("Escribe la respuesta primero")
			return
		}
		v = comprobarFrase(a.tec.texto(), e.Soluciones, true)
	default:
		return
	}
	t.comprobado, t.ver = true, v
	t.escribir = false
	a.son.aviso(v.Bien)
	a.lec.responder(v.Bien)
	var guess any = a.respuestaDe(t)
	if e.Tipo == ejElegir {
		guess = t.elegida
	}
	a.lec.apuntar(e, guess, v.Bien, int(time.Since(t.desde).Milliseconds()))
}

func (a *app) continuar() {
	switch {
	case a.lec.sinVidas():
		a.pant = pSinVidas
		a.son.parar()
	case a.lec.acabada():
		a.acabar()
	default:
		a.nuevoTurno()
	}
}
