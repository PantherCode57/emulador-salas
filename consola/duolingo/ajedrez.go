package main

import (
	"strconv"
	"strings"
)

// Ajedrez: el tablero de los ejercicios del curso de ajedrez de Duolingo. Cada ejercicio llega con
// la posición en FEN y lo que hay que jugar (chessPuzzleInfo: correctMoves, enemyMoves, validPaths,
// moveEvaluationsForPositions, maxMoves), con las jugadas en UCI («e2e4», «e7e8q»). En los de
// estrellas, los peones negros son las estrellas que hay que comerse y no hay reyes.
//
// Las casillas: fila 0 es la 8.ª (la de arriba con las blancas abajo) y columna 0 la «a».
type tablero struct {
	c       [8][8]byte // 'K','Q','R','B','N','P' blancas; minúsculas, negras; 0 vacía
	blancas bool       // les toca a las blancas
}

func leerFEN(fen string) (tablero, bool) {
	var t tablero
	partes := strings.Fields(fen)
	if len(partes) == 0 {
		return t, false
	}
	filas := strings.Split(partes[0], "/")
	if len(filas) != 8 {
		return t, false
	}
	for f, fila := range filas {
		col := 0
		for _, r := range fila {
			switch {
			case r >= '1' && r <= '8':
				col += int(r - '0')
			case strings.ContainsRune("KQRBNPkqrbnp", r):
				if col > 7 {
					return t, false
				}
				t.c[f][col] = byte(r)
				col++
			default:
				return t, false
			}
		}
		if col != 8 {
			return t, false
		}
	}
	t.blancas = len(partes) < 2 || partes[1] != "b"
	return t, true
}

// fen: la posición (solo las piezas y a quién le toca), para compararla con las de Duolingo.
func (t tablero) fen() string {
	var b strings.Builder
	for f := 0; f < 8; f++ {
		vacias := 0
		for col := 0; col < 8; col++ {
			p := t.c[f][col]
			if p == 0 {
				vacias++
				continue
			}
			if vacias > 0 {
				b.WriteString(strconv.Itoa(vacias))
				vacias = 0
			}
			b.WriteByte(p)
		}
		if vacias > 0 {
			b.WriteString(strconv.Itoa(vacias))
		}
		if f < 7 {
			b.WriteByte('/')
		}
	}
	if t.blancas {
		b.WriteString(" w")
	} else {
		b.WriteString(" b")
	}
	return b.String()
}

// mismaPosicion: si dos FEN son la misma posición (las piezas y a quién le toca).
func mismaPosicion(a, b string) bool {
	pa, pb := strings.Fields(a), strings.Fields(b)
	if len(pa) == 0 || len(pb) == 0 || pa[0] != pb[0] {
		return false
	}
	return len(pa) < 2 || len(pb) < 2 || pa[1] == pb[1]
}

func blanca(p byte) bool { return p >= 'A' && p <= 'Z' }

// casilla: «e4» → fila, columna.
func casilla(s string) (int, int, bool) {
	if len(s) < 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return 0, 0, false
	}
	return int('8' - s[1]), int(s[0] - 'a'), true
}

func nombreCasilla(f, c int) string { return string(rune('a'+c)) + string(rune('8'-f)) }

type jugada struct {
	df, dc, af, ac int
	corona         byte // 'q', 'r', 'b', 'n' o 0
}

func leerUCI(s string) (jugada, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 4 {
		return jugada{}, false
	}
	df, dc, ok1 := casilla(s[:2])
	af, ac, ok2 := casilla(s[2:4])
	if !ok1 || !ok2 {
		return jugada{}, false
	}
	j := jugada{df: df, dc: dc, af: af, ac: ac}
	if len(s) >= 5 && strings.ContainsRune("qrbn", rune(s[4])) {
		j.corona = s[4]
	}
	return j, true
}

func (j jugada) uci() string {
	s := nombreCasilla(j.df, j.dc) + nombreCasilla(j.af, j.ac)
	if j.corona != 0 {
		s += string(j.corona)
	}
	return s
}

// jugadasUCI: las jugadas que haya en un texto o en una lista («e2e4 e7e5», ["e2e4", …]).
func jugadasUCI(v any) []string {
	var out []string
	var mirar func(x any)
	mirar = func(x any) {
		switch y := x.(type) {
		case string:
			for _, p := range strings.Fields(y) {
				if _, ok := leerUCI(p); ok && len(p) <= 5 {
					out = append(out, strings.ToLower(p))
				}
			}
		case []any:
			for _, z := range y {
				mirar(z)
			}
		case map[string]any:
			for _, k := range []string{"move", "uci", "san"} {
				if s, ok := y[k].(string); ok {
					mirar(s)
					return
				}
			}
		}
	}
	mirar(v)
	return out
}

// mover hace la jugada (sin mirar si se puede) y cambia el turno. Devuelve lo que se ha comido.
func (t *tablero) mover(j jugada) byte {
	p := t.c[j.df][j.dc]
	comida := t.c[j.af][j.ac]
	t.c[j.af][j.ac], t.c[j.df][j.dc] = p, 0
	switch {
	case j.corona != 0 && blanca(p):
		t.c[j.af][j.ac] = j.corona - 'a' + 'A'
	case j.corona != 0:
		t.c[j.af][j.ac] = j.corona
	case (p == 'P' && j.af == 0) || (p == 'p' && j.af == 7): // el peón que llega, a dama
		t.c[j.af][j.ac] = p - 'P' + 'Q'
	}
	// enroque: si el rey anda dos casillas, la torre salta
	if (p == 'K' || p == 'k') && j.df == j.af && abs(j.ac-j.dc) == 2 {
		if j.ac > j.dc {
			t.c[j.af][5], t.c[j.af][7] = t.c[j.af][7], 0
		} else {
			t.c[j.af][3], t.c[j.af][0] = t.c[j.af][0], 0
		}
	}
	t.blancas = !t.blancas
	return comida
}

// destinos: adónde puede ir la pieza de (f, c), sin mirar los jaques (en los ejercicios lo que vale
// lo dice Duolingo; esto es para pintar los puntitos y no dejar mover las piezas como no van).
func (t tablero) destinos(f, c int) [][2]int {
	p := t.c[f][c]
	if p == 0 {
		return nil
	}
	mia := blanca(p)
	var out [][2]int
	dentro := func(f, c int) bool { return f >= 0 && f < 8 && c >= 0 && c < 8 }
	libreORival := func(f, c int) bool {
		q := t.c[f][c]
		return q == 0 || blanca(q) != mia
	}
	deslizar := func(dirs [][2]int) {
		for _, d := range dirs {
			for k := 1; k < 8; k++ {
				nf, nc := f+d[0]*k, c+d[1]*k
				if !dentro(nf, nc) {
					break
				}
				if t.c[nf][nc] == 0 {
					out = append(out, [2]int{nf, nc})
					continue
				}
				if blanca(t.c[nf][nc]) != mia {
					out = append(out, [2]int{nf, nc})
				}
				break
			}
		}
	}
	saltos := func(ds [][2]int) {
		for _, d := range ds {
			if nf, nc := f+d[0], c+d[1]; dentro(nf, nc) && libreORival(nf, nc) {
				out = append(out, [2]int{nf, nc})
			}
		}
	}
	rectas := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	diagonales := [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	switch p | 0x20 { // en minúscula
	case 'r':
		deslizar(rectas)
	case 'b':
		deslizar(diagonales)
	case 'q':
		deslizar(append(rectas, diagonales...))
	case 'n':
		saltos([][2]int{{1, 2}, {2, 1}, {-1, 2}, {-2, 1}, {1, -2}, {2, -1}, {-1, -2}, {-2, -1}})
	case 'k':
		saltos(append(rectas, diagonales...))
		// enroques (si rey y torre están en su sitio y no hay nada en medio)
		fila := 7
		if !mia {
			fila = 0
		}
		torre := byte('R')
		if !mia {
			torre = 'r'
		}
		if f == fila && c == 4 {
			if t.c[fila][7] == torre && t.c[fila][5] == 0 && t.c[fila][6] == 0 {
				out = append(out, [2]int{fila, 6})
			}
			if t.c[fila][0] == torre && t.c[fila][1] == 0 && t.c[fila][2] == 0 && t.c[fila][3] == 0 {
				out = append(out, [2]int{fila, 2})
			}
		}
	case 'p':
		paso, salida := -1, 6
		if !mia {
			paso, salida = 1, 1
		}
		if dentro(f+paso, c) && t.c[f+paso][c] == 0 {
			out = append(out, [2]int{f + paso, c})
			if f == salida && t.c[f+2*paso][c] == 0 {
				out = append(out, [2]int{f + 2*paso, c})
			}
		}
		for _, dc := range []int{-1, 1} {
			if nf, nc := f+paso, c+dc; dentro(nf, nc) && t.c[nf][nc] != 0 && blanca(t.c[nf][nc]) != mia {
				out = append(out, [2]int{nf, nc})
			}
		}
	}
	return out
}

func (t tablero) puedeIr(f, c, af, ac int) bool {
	for _, d := range t.destinos(f, c) {
		if d[0] == af && d[1] == ac {
			return true
		}
	}
	return false
}

// quedan: cuántas piezas de esa letra hay (las estrellas son los peones negros).
func (t tablero) quedan(p byte) int {
	n := 0
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			if t.c[f][c] == p {
				n++
			}
		}
	}
	return n
}

// legible: «g1f3» → «Caballo de g1 a f3».
func (t tablero) legible(uci string) string {
	j, ok := leerUCI(uci)
	if !ok {
		return uci
	}
	nombre := map[byte]string{'k': "Rey", 'q': "Dama", 'r': "Torre", 'b': "Alfil", 'n': "Caballo", 'p': "Peón"}[t.c[j.df][j.dc]|0x20]
	s := "de " + nombreCasilla(j.df, j.dc) + " a " + nombreCasilla(j.af, j.ac)
	if nombre != "" {
		s = nombre + " " + s
	}
	return s
}
