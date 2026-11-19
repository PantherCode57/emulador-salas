package main

import (
	"image"
	"image/color"
	"strings"
	"time"
)

// Los ejercicios de ajedrez: el tablero a la izquierda (la cruceta mueve el cursor, A coge una pieza y
// A otra vez la deja) y a la derecha qué hay que hacer. Las respuestas del rival las pone la consola
// sola, como en Duolingo.

type evalJugada struct {
	Jugada    string
	Respuesta string
	Buena     bool
}

// datos de un ejercicio de ajedrez (dentro de ejercicio)
type datosAjedrez struct {
	FEN          string
	Jugadas      []string   // las tuyas (correctMoves)
	Rival        []string   // las respuestas (enemyMoves)
	Caminos      [][]string // otras formas buenas (validPaths)
	Evaluaciones map[string][]evalJugada
	MaxJugadas   int
	Estrellas    bool // cómete las estrellas (los peones negros)
}

// lo que pasa en el tablero mientras juegas
type partida struct {
	tab        tablero
	conBlancas bool
	cur        [2]int
	sel        [2]int // la pieza cogida (-1: ninguna)
	hechas     []string
	usadas     int // jugadas tuyas (en los de estrellas)
	ultima     [2][2]int
	hayUltima  bool
	rivalEn    time.Time
	rival      string
	acabar     bool // al poner la del rival, se acaba bien
	pista      bool
}

var (
	colCasillaClara  = color.RGBA{0xeb, 0xec, 0xd0, 0xff}
	colCasillaOscura = color.RGBA{0x77, 0x95, 0x56, 0xff}
	colUltima        = color.NRGBA{0xf6, 0xf6, 0x69, 0x90} // sin premultiplicar: se mezcla con la casilla
	colCogida        = color.NRGBA{0xff, 0xd4, 0x3b, 0xb0}
	colPista         = color.NRGBA{0x1c, 0xb0, 0xf6, 0x90}
	colPunto         = color.RGBA{0x10, 0x18, 0x10, 0x60}
)

const (
	ladoCasilla = 48
	tabX, tabY  = 20, 58
)

func convertirAjedrez(c map[string]any, e *ejercicio) bool {
	fen := texto(c, "fen")
	if fen == "" {
		fen = texto(c, "boardFen")
	}
	if fen == "" {
		return false
	}
	if _, ok := leerFEN(fen); !ok {
		return false
	}
	info := obj(c, "chessPuzzleInfo")
	d := &datosAjedrez{FEN: fen, Jugadas: jugadasUCI(info["correctMoves"]), Rival: jugadasUCI(info["enemyMoves"]),
		MaxJugadas: entero(info, "maxMoves")}
	for _, v := range lista(info, "validPaths") {
		if p := jugadasUCI(v); len(p) > 0 {
			d.Caminos = append(d.Caminos, p)
		}
	}
	if ev := obj(info, "moveEvaluationsForPositions"); ev != nil {
		d.Evaluaciones = map[string][]evalJugada{}
		for pos, l := range ev {
			ls, _ := l.([]any)
			for _, x := range ls {
				d.Evaluaciones[pos] = append(d.Evaluaciones[pos], evalJugada{Jugada: strings.ToLower(texto(x, "move")),
					Respuesta: strings.ToLower(texto(x, "enemyResponse")), Buena: texto(x, "moveCorrectness") == "correct"})
			}
		}
	}
	t, _ := leerFEN(fen)
	d.Estrellas = e.Clase == "star" || (t.quedan('k') == 0 && t.quedan('p') > 0 && len(d.Jugadas) == 0)
	if !d.Estrellas && len(d.Jugadas) == 0 && len(d.Caminos) == 0 && len(d.Evaluaciones) == 0 {
		return false // no se sabe qué vale
	}
	e.Tipo, e.Ajedrez = ejAjedrez, d
	if p := texto(c, "prompt"); p != "" && e.Instruccion == "Contesta" {
		e.Instruccion = p
	}
	if e.Instruccion == "Contesta" || e.Instruccion == "" {
		e.Instruccion = "Encuentra la mejor jugada"
		if d.Estrellas {
			e.Instruccion = "Cómete todas las estrellas"
		}
	}
	if len(d.Jugadas) > 0 {
		e.Mostrar = t.legible(d.Jugadas[0])
	}
	return true
}

func (a *app) empezarAjedrez() {
	d := a.t.ej.Ajedrez
	t, _ := leerFEN(d.FEN)
	p := &partida{tab: t, conBlancas: t.blancas, sel: [2]int{-1, -1}}
	// el cursor, en la primera pieza tuya que se pueda mover (de abajo arriba)
	p.cur = [2]int{7, 4}
	if !p.conBlancas {
		p.cur = [2]int{0, 4}
	}
	for k := 0; k < 64; k++ {
		f, c := 7-k/8, k%8
		if !p.conBlancas {
			f = k / 8
		}
		if q := t.c[f][c]; q != 0 && blanca(q) == p.conBlancas && len(t.destinos(f, c)) > 0 {
			p.cur = [2]int{f, c}
			break
		}
	}
	a.t.ajedrez = p
}

// en pantalla: con las negras, el tablero se ve al revés
func (p *partida) aPantalla(f, c int) (int, int) {
	if p.conBlancas {
		return f, c
	}
	return 7 - f, 7 - c
}

func (a *app) pulsarAjedrez(b uint16) {
	t := a.t
	p := t.ajedrez
	if p == nil || !p.rivalEn.IsZero() {
		return // mientras juega el rival
	}
	dir := 1
	if !p.conBlancas {
		dir = -1
	}
	switch b {
	case bArriba:
		p.cur[0] = min(max(p.cur[0]-dir, 0), 7)
	case bAbajo:
		p.cur[0] = min(max(p.cur[0]+dir, 0), 7)
	case bIzquierda:
		p.cur[1] = min(max(p.cur[1]-dir, 0), 7)
	case bDerecha:
		p.cur[1] = min(max(p.cur[1]+dir, 0), 7)
	case bY:
		p.pista = true
	case bB:
		if p.sel[0] >= 0 {
			p.sel = [2]int{-1, -1}
		} else {
			a.salirDeLeccion()
		}
	case bA:
		f, c := p.cur[0], p.cur[1]
		q := p.tab.c[f][c]
		if p.sel[0] < 0 || (q != 0 && blanca(q) == p.conBlancas && [2]int{f, c} != p.sel) {
			if q != 0 && blanca(q) == p.conBlancas {
				p.sel = [2]int{f, c}
			}
			return
		}
		if [2]int{f, c} == p.sel {
			p.sel = [2]int{-1, -1}
			return
		}
		if !p.tab.puedeIr(p.sel[0], p.sel[1], f, c) {
			a.aviso("Esa pieza no se mueve así")
			return
		}
		j := jugada{df: p.sel[0], dc: p.sel[1], af: f, ac: c}
		if pz := p.tab.c[j.df][j.dc]; (pz == 'P' && f == 0) || (pz == 'p' && f == 7) {
			j.corona = 'q'
		}
		p.sel = [2]int{-1, -1}
		a.jugar(j)
	}
}

// jugar: tu jugada. Se mira si es la buena; si lo es, el rival contesta (si le toca).
func (a *app) jugar(j jugada) {
	t := a.t
	p, d := t.ajedrez, t.ej.Ajedrez
	m := j.uci()
	if d.Estrellas {
		p.tab.mover(j)
		p.tab.blancas = p.conBlancas // en los de estrellas siempre mueves tú
		p.ultima, p.hayUltima = [2][2]int{{j.df, j.dc}, {j.af, j.ac}}, true
		p.usadas++
		p.hechas = append(p.hechas, m)
		switch {
		case p.tab.quedan('p') == 0:
			a.acabarAjedrez(true)
		case d.MaxJugadas > 0 && p.usadas >= d.MaxJugadas:
			a.acabarAjedrez(false)
		}
		return
	}
	k := len(p.hechas)
	ok, respuesta, fin := false, "", false
	coincide := func(esperada string) bool {
		return esperada == m || (len(esperada) == 4 && len(m) == 5 && strings.HasPrefix(m, esperada))
	}
	if k < len(d.Jugadas) && coincide(d.Jugadas[k]) {
		ok, fin = true, k+1 >= len(d.Jugadas)
		if k < len(d.Rival) && !fin {
			respuesta = d.Rival[k]
		}
	}
	for _, cam := range d.Caminos {
		if ok || k >= len(cam) || !coincide(cam[k]) || !prefijo(cam, p.hechas) {
			continue
		}
		ok, fin = true, k+1 >= len(cam)
		if !fin && k+1 < len(cam) {
			respuesta = cam[k+1] // en los caminos van las dos (tuya, suya, tuya…)
		}
	}
	if !ok && len(d.Evaluaciones) > 0 {
		pos := p.tab.fen()
		for clave, evs := range d.Evaluaciones {
			if !mismaPosicion(clave, pos) {
				continue
			}
			for _, ev := range evs {
				if ev.Buena && coincide(ev.Jugada) {
					ok, respuesta = true, ev.Respuesta
				}
			}
		}
		if ok && respuesta == "" {
			fin = true
		}
	}
	if !ok {
		t.ver.Nota = ""
		if k < len(d.Jugadas) {
			t.ej.Mostrar = p.tab.legible(d.Jugadas[k])
		}
		p.tab.mover(j)
		p.ultima, p.hayUltima = [2][2]int{{j.df, j.dc}, {j.af, j.ac}}, true
		a.acabarAjedrez(false)
		return
	}
	p.tab.mover(j)
	p.ultima, p.hayUltima = [2][2]int{{j.df, j.dc}, {j.af, j.ac}}, true
	p.hechas = append(p.hechas, m)
	p.pista = false
	if fin || respuesta == "" {
		a.acabarAjedrez(true)
		return
	}
	a.son.notas([]float64{523.25}, 0.06)
	p.rival, p.rivalEn = respuesta, time.Now().Add(650*time.Millisecond)
}

func prefijo(cam, hechas []string) bool {
	if len(hechas) > len(cam) {
		return false
	}
	for i, h := range hechas {
		if cam[i] != h {
			return false
		}
	}
	return true
}

// moverRival: la respuesta del rival, cuando ha pasado un momento.
func (a *app) moverRival() {
	t := a.t
	if t == nil || t.ajedrez == nil || t.ajedrez.rivalEn.IsZero() || time.Now().Before(t.ajedrez.rivalEn) {
		return
	}
	p := t.ajedrez
	p.rivalEn = time.Time{}
	j, ok := leerUCI(p.rival)
	if !ok {
		a.acabarAjedrez(true)
		return
	}
	p.tab.mover(j)
	p.ultima, p.hayUltima = [2][2]int{{j.df, j.dc}, {j.af, j.ac}}, true
	p.hechas = append(p.hechas, p.rival)
	a.cambios = true
	// con las evaluaciones: si la posición nueva ya no está, se ha acabado
	d := t.ej.Ajedrez
	if len(d.Jugadas) == 0 && len(d.Caminos) == 0 && len(d.Evaluaciones) > 0 {
		pos := p.tab.fen()
		for clave := range d.Evaluaciones {
			if mismaPosicion(clave, pos) {
				return
			}
		}
		a.acabarAjedrez(true)
	}
}

func (a *app) acabarAjedrez(bien bool) {
	t := a.t
	t.comprobado, t.ver = true, veredicto{Bien: bien}
	if !bien && t.ej.Ajedrez.Estrellas {
		t.ej.Mostrar = "Te has quedado sin jugadas"
	}
	a.son.aviso(bien)
	a.lec.responder(bien)
	a.lec.apuntar(t.ej, strings.Join(t.ajedrez.hechas, " "), bien, int(time.Since(t.desde).Milliseconds()))
}

// esperada: la casilla de la pieza que hay que mover ahora (para la pista).
func (p *partida) esperada(d *datosAjedrez) (int, int, bool) {
	k := len(p.hechas)
	var m string
	switch {
	case k < len(d.Jugadas):
		m = d.Jugadas[k]
	default:
		for _, cam := range d.Caminos {
			if k < len(cam) && prefijo(cam, p.hechas) {
				m = cam[k]
				break
			}
		}
	}
	if j, ok := leerUCI(m); ok {
		return j.df, j.dc, true
	}
	return 0, 0, false
}

func (a *app) dibujarAjedrez(t *turno) {
	l := a.l
	p, d := t.ajedrez, t.ej.Ajedrez
	if p == nil {
		return
	}
	borde := image.Rect(tabX-4, tabY-4, tabX+8*ladoCasilla+4, tabY+8*ladoCasilla+4)
	l.redondeado(borde, 8, colGrisOsc)
	var destinos [][2]int
	if p.sel[0] >= 0 {
		destinos = p.tab.destinos(p.sel[0], p.sel[1])
	}
	pf, pc, hayPista := 0, 0, false
	if p.pista {
		pf, pc, hayPista = p.esperada(d)
	}
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			sf, sc := p.aPantalla(f, c)
			r := image.Rect(tabX+sc*ladoCasilla, tabY+sf*ladoCasilla, tabX+(sc+1)*ladoCasilla, tabY+(sf+1)*ladoCasilla)
			col := colCasillaClara
			if (f+c)%2 == 1 {
				col = colCasillaOscura
			}
			l.rect(r, col)
			if p.hayUltima && ([2]int{f, c} == p.ultima[0] || [2]int{f, c} == p.ultima[1]) {
				l.rect(r, colUltima)
			}
			if [2]int{f, c} == p.sel {
				l.rect(r, colCogida)
			}
			if hayPista && f == pf && c == pc {
				l.rect(r, colPista)
			}
			// las letras y números de los bordes
			if sc == 0 {
				l.texto(r.Min.X+3, r.Min.Y+13, string(rune('8'-f)), l.f.mini, contrario(col))
			}
			if sf == 7 {
				l.texto(r.Max.X-10, r.Max.Y-4, string(rune('a'+c)), l.f.mini, contrario(col))
			}
			a.pieza(r, p.tab.c[f][c], d.Estrellas)
			for _, ds := range destinos {
				if ds[0] == f && ds[1] == c {
					if p.tab.c[f][c] != 0 {
						l.marcoRedondo(r.Inset(2), ladoCasilla/2, 4, colPunto)
					} else {
						l.circuloSuave((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, 7, colPunto)
					}
				}
			}
			if [2]int{f, c} == p.cur && !t.comprobado {
				l.marcoRedondo(r, 4, 3, colTexto)
				l.marcoRedondo(r.Inset(3), 3, 1, colFondo)
			}
		}
	}
	// a la derecha: qué hay que hacer
	x0 := tabX + 8*ladoCasilla + 20
	y := 60
	for i, ln := range ajustar(l.f.titulo2, t.ej.Instruccion, ancho-18-x0) {
		if i >= 3 {
			break
		}
		l.arriba(x0, y, ln, l.f.titulo2, colTexto)
		y += 26
	}
	panel := image.Rect(x0, y+10, ancho-18, y+200)
	l.redondeado(panel, 16, colPanel)
	y = panel.Min.Y + 14
	turno := "Juegas con blancas"
	if !p.conBlancas {
		turno = "Juegas con negras"
	}
	l.circuloSuave(panel.Min.X+24, y+11, 9, colTexto)
	if !p.conBlancas {
		l.circuloSuave(panel.Min.X+24, y+11, 7, colFondo)
	}
	l.izquierda(image.Rect(panel.Min.X+40, y, panel.Max.X-10, y+22), turno, l.f.pequena, colTexto)
	y += 36
	switch {
	case d.Estrellas:
		l.izquierda(image.Rect(panel.Min.X+16, y, panel.Max.X-10, y+24), "Estrellas: "+itoa(p.tab.quedan('p')), l.f.nombre, colOro)
		y += 28
		if d.MaxJugadas > 0 {
			l.izquierda(image.Rect(panel.Min.X+16, y, panel.Max.X-10, y+24), "Jugadas: "+itoa(p.usadas)+" de "+itoa(d.MaxJugadas), l.f.normal, colSuave)
			y += 28
		}
	case !p.rivalEn.IsZero():
		l.izquierda(image.Rect(panel.Min.X+16, y, panel.Max.X-10, y+24), "Piensa el rival"+puntos(), l.f.normal, colSuave)
		y += 28
	case len(d.Jugadas) > 1:
		hechasTuyas := (len(p.hechas) + 1) / 2
		l.izquierda(image.Rect(panel.Min.X+16, y, panel.Max.X-10, y+24), "Jugada "+itoa(min(hechasTuyas+1, len(d.Jugadas)))+" de "+itoa(len(d.Jugadas)), l.f.normal, colSuave)
		y += 28
	}
	for _, ln := range ajustar(l.f.pequena, "A coge una pieza y la deja. Y da una pista.", panel.Dx()-32) {
		l.izquierda(image.Rect(panel.Min.X+16, y, panel.Max.X-10, y+20), ln, l.f.pequena, colApagado)
		y += 20
	}
}

func contrario(c color.RGBA) color.RGBA {
	if c == colCasillaClara {
		return colCasillaOscura
	}
	return colCasillaClara
}

// pieza: con la letra de ajedrez (DejaVu Sans): la silueta rellena y el contorno encima.
func (a *app) pieza(r image.Rectangle, p byte, estrellas bool) {
	if p == 0 {
		return
	}
	l := a.l
	if estrellas && p == 'p' {
		l.centrado(r, icEstrella, a.f.iconoDe(30), colOro)
		return
	}
	if a.f.piezas == nil {
		c := color.RGBA{0x10, 0x10, 0x10, 0xff}
		if blanca(p) {
			c = colTexto
		}
		l.centrado(r, strings.ToUpper(string(p)), l.f.grande, c)
		return
	}
	llenas := map[byte]rune{'k': '♚', 'q': '♛', 'r': '♜', 'b': '♝', 'n': '♞', 'p': '♟'}
	huecas := map[byte]rune{'k': '♔', 'q': '♕', 'r': '♖', 'b': '♗', 'n': '♘', 'p': '♙'}
	q := p | 0x20
	if blanca(p) {
		l.centrado(r.Add(image.Pt(0, -1)), string(llenas[q]), a.f.piezas, colTexto)
		l.centrado(r.Add(image.Pt(0, -1)), string(huecas[q]), a.f.piezas, color.RGBA{0x20, 0x20, 0x20, 0xff})
	} else {
		l.centrado(r.Add(image.Pt(0, -1)), string(llenas[q]), a.f.piezas, color.RGBA{0x22, 0x22, 0x22, 0xff})
	}
}
