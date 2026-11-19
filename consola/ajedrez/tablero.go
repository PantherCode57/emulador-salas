package main

import (
	"image"
	"image/color"
	"strings"
)

// El tablero en pantalla: 8 × 48 px a la izquierda. La cruceta mueve el cursor (al revés si juegas
// con negras, que se ven abajo), A coge una pieza tuya y A otra vez la deja donde puede ir.
const (
	lado       = 48
	tabX, tabY = 16, 48
)

var (
	colClara  = color.RGBA{0xf0, 0xd9, 0xb5, 0xff} // las casillas de madera de siempre
	colOscura = color.RGBA{0xb5, 0x88, 0x63, 0xff}
	colUltima = color.NRGBA{0x9b, 0xc7, 0x00, 0x70} // sin premultiplicar: se mezcla con la casilla
	colCogida = color.NRGBA{0x14, 0x55, 0x1e, 0x80}
	colJaque  = color.NRGBA{0xff, 0x30, 0x30, 0xa0}
	colPista  = color.NRGBA{0x1c, 0xb0, 0xf6, 0x90}
	colPunto  = color.RGBA{0x14, 0x1e, 0x14, 0x58}
)

type vistaTablero struct {
	tab        tablero
	conBlancas bool // tú llevas las blancas (se ven abajo)
	cur, sel   [2]int
	ultima     [2][2]int
	hayUltima  bool
	pista      [2]int // casilla marcada en azul (-1: ninguna)
	// coronación: el peón llega al final y se elige pieza
	coronando bool
	corona    jugada
	opCorona  int
}

func nuevaVista(t tablero, conBlancas bool) *vistaTablero {
	v := &vistaTablero{tab: t, conBlancas: conBlancas, sel: [2]int{-1, -1}, pista: [2]int{-1, -1}}
	v.cur = [2]int{6, 4}
	if !conBlancas {
		v.cur = [2]int{1, 4}
	}
	return v
}

func (v *vistaTablero) marcarUltima(j jugada) {
	v.ultima, v.hayUltima = [2][2]int{{j.df, j.dc}, {j.af, j.ac}}, true
}

func (v *vistaTablero) aPantalla(f, c int) (int, int) {
	if v.conBlancas {
		return f, c
	}
	return 7 - f, 7 - c
}

func (v *vistaTablero) mia(p byte) bool { return p != 0 && blanca(p) == v.conBlancas }

// pulsar: un botón en el tablero. Devuelve la jugada si se ha hecho una (y si se puede mover ahora).
func (v *vistaTablero) pulsar(b uint16, puedeMover bool) (jugada, bool) {
	if v.coronando {
		switch b {
		case bIzquierda:
			v.opCorona = (v.opCorona + 3) % 4
		case bDerecha:
			v.opCorona = (v.opCorona + 1) % 4
		case bA:
			v.coronando = false
			j := v.corona
			j.corona = "qrbn"[v.opCorona]
			return j, true
		case bB:
			v.coronando = false
		}
		return jugada{}, false
	}
	dir := 1
	if !v.conBlancas {
		dir = -1
	}
	switch b {
	case bArriba:
		v.cur[0] = min(max(v.cur[0]-dir, 0), 7)
	case bAbajo:
		v.cur[0] = min(max(v.cur[0]+dir, 0), 7)
	case bIzquierda:
		v.cur[1] = min(max(v.cur[1]-dir, 0), 7)
	case bDerecha:
		v.cur[1] = min(max(v.cur[1]+dir, 0), 7)
	case bA:
		f, c := v.cur[0], v.cur[1]
		q := v.tab.c[f][c]
		if v.sel[0] < 0 || (v.mia(q) && [2]int{f, c} != v.sel) {
			if v.mia(q) {
				v.sel = [2]int{f, c}
			}
			return jugada{}, false
		}
		if [2]int{f, c} == v.sel {
			v.sel = [2]int{-1, -1}
			return jugada{}, false
		}
		if !puedeMover || !v.tab.puedeIr(v.sel[0], v.sel[1], f, c) {
			return jugada{}, false
		}
		j := jugada{df: v.sel[0], dc: v.sel[1], af: f, ac: c}
		v.sel = [2]int{-1, -1}
		if pz := v.tab.c[j.df][j.dc]; (pz == 'P' && f == 0) || (pz == 'p' && f == 7) {
			v.coronando, v.corona, v.opCorona = true, j, 0
			return jugada{}, false
		}
		return j, true
	}
	return jugada{}, false
}

func (a *app) dibujarTablero(v *vistaTablero, activo bool) {
	l := a.l
	l.redondeado(image.Rect(tabX-4, tabY-4, tabX+8*lado+4, tabY+8*lado+4), 8, color.RGBA{0x26, 0x24, 0x21, 0xff})
	var destinos [][2]int
	if v.sel[0] >= 0 {
		destinos = v.tab.destinos(v.sel[0], v.sel[1])
	}
	jf, jc, jaque := -1, -1, false
	if v.tab.enJaque() {
		jf, jc, jaque = func() (int, int, bool) { return v.tab.rey(v.tab.blancas) }()
	}
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			sf, sc := v.aPantalla(f, c)
			r := image.Rect(tabX+sc*lado, tabY+sf*lado, tabX+(sc+1)*lado, tabY+(sf+1)*lado)
			col := colClara
			if (f+c)%2 == 1 {
				col = colOscura
			}
			l.rect(r, col)
			if v.hayUltima && ([2]int{f, c} == v.ultima[0] || [2]int{f, c} == v.ultima[1]) {
				l.rect(r, colUltima)
			}
			if [2]int{f, c} == v.sel {
				l.rect(r, colCogida)
			}
			if [2]int{f, c} == v.pista {
				l.rect(r, colPista)
			}
			if jaque && f == jf && c == jc {
				l.rect(r, colJaque)
			}
			otro := colOscura
			if col == colOscura {
				otro = colClara
			}
			if sc == 0 {
				l.texto(r.Min.X+3, r.Min.Y+13, string(rune('8'-f)), l.f.mini, otro)
			}
			if sf == 7 {
				l.texto(r.Max.X-10, r.Max.Y-4, string(rune('a'+c)), l.f.mini, otro)
			}
			a.pieza(r, v.tab.c[f][c])
			for _, d := range destinos {
				if d[0] == f && d[1] == c {
					if v.tab.c[f][c] != 0 {
						l.marcoRedondo(r.Inset(2), lado/2, 4, colPunto)
					} else {
						l.circuloSuave((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, 7, colPunto)
					}
				}
			}
			if activo && [2]int{f, c} == v.cur {
				l.marcoRedondo(r, 4, 3, colTexto)
				l.marcoRedondo(r.Inset(3), 3, 1, colFondo)
			}
		}
	}
	if v.coronando {
		a.dibujarCoronacion(v)
	}
}

// dibujarCoronacion: las cuatro piezas para elegir, encima del tablero.
func (a *app) dibujarCoronacion(v *vistaTablero) {
	l := a.l
	caja := image.Rect(tabX+30, tabY+120, tabX+8*lado-30, tabY+262)
	l.redondeado(caja, 16, color.RGBA{0x14, 0x14, 0x14, 0xf2})
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+6, caja.Max.X, caja.Min.Y+30), "¿En qué se convierte el peón?", l.f.pequena, colTexto)
	ps := "qrbn"
	if v.conBlancas {
		ps = "QRBN"
	}
	for i := 0; i < 4; i++ {
		r := image.Rect(caja.Min.X+26+i*68, caja.Min.Y+38, caja.Min.X+26+i*68+60, caja.Min.Y+98)
		fondo := colClara
		if i == v.opCorona {
			fondo = colOro
		}
		l.redondeado(r, 10, fondo)
		a.pieza(r, ps[i])
	}
	l.centrado(image.Rect(caja.Min.X, caja.Max.Y-30, caja.Max.X, caja.Max.Y-8), "IZQ DER  Elegir    A  Vale    B  Otra jugada", l.f.mini, colSuave)
}

// pieza: con la letra de ajedrez (DejaVu Sans): la silueta rellena y el contorno encima.
func (a *app) pieza(r image.Rectangle, p byte) {
	if p == 0 {
		return
	}
	l := a.l
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
	rr := r.Add(image.Pt(0, -1))
	if blanca(p) {
		l.centrado(rr, string(llenas[q]), a.f.piezas, colTexto)
		l.centrado(rr, string(huecas[q]), a.f.piezas, color.RGBA{0x20, 0x20, 0x20, 0xff})
	} else {
		l.centrado(rr, string(llenas[q]), a.f.piezas, color.RGBA{0x1a, 0x1a, 0x1a, 0xff})
	}
}
