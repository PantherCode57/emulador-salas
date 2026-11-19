package main

import (
	"strconv"
	"strings"
)

// Las reglas del ajedrez, para pintar la partida y no dejar mandar jugadas que no valen (Lichess las
// mira igual, pero así no hay que esperar a que diga que no). La posición sale de repetir desde el
// principio las jugadas que manda Lichess (en UCI: «e2e4», «e7e8q», «e1g1» para enrocar).
//
// Las casillas: fila 0 es la 8.ª (arriba con las blancas abajo) y columna 0 la «a».
type tablero struct {
	c        [8][8]byte // 'K','Q','R','B','N','P' blancas; minúsculas, negras; 0 vacía
	blancas  bool       // les toca a las blancas
	enroques string     // «KQkq» (los que quedan)
	alPaso   [2]int     // la casilla donde se puede comer al paso (-1 si no hay)
}

const fenInicial = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

func leerFEN(fen string) (tablero, bool) {
	t := tablero{alPaso: [2]int{-1, -1}}
	if fen == "" || fen == "startpos" {
		fen = fenInicial
	}
	partes := strings.Fields(fen)
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
	if len(partes) >= 3 && partes[2] != "-" {
		t.enroques = partes[2]
	}
	if len(partes) >= 4 {
		if f, c, ok := casilla(partes[3]); ok {
			t.alPaso = [2]int{f, c}
		}
	}
	return t, true
}

// posicionDe: la posición después de esas jugadas (desde la inicial o desde otra en FEN).
func posicionDe(fenInicio, jugadas string) (tablero, []jugada) {
	t, ok := leerFEN(fenInicio)
	if !ok {
		t, _ = leerFEN(fenInicial)
	}
	var hechas []jugada
	for _, m := range strings.Fields(jugadas) {
		j, ok := leerUCI(m)
		if !ok {
			break
		}
		t.mover(j)
		hechas = append(hechas, j)
	}
	return t, hechas
}

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
	return b.String()
}

func blanca(p byte) bool { return p >= 'A' && p <= 'Z' }

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

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// mover hace la jugada (sin mirar si vale), con el enroque, la captura al paso y la coronación, y
// cambia el turno. Devuelve lo que se ha comido.
func (t *tablero) mover(j jugada) byte {
	p := t.c[j.df][j.dc]
	comida := t.c[j.af][j.ac]
	// al paso: el peón va en diagonal a una casilla vacía
	if (p == 'P' || p == 'p') && j.dc != j.ac && comida == 0 {
		comida = t.c[j.df][j.ac]
		t.c[j.df][j.ac] = 0
	}
	t.c[j.af][j.ac], t.c[j.df][j.dc] = p, 0
	switch {
	case j.corona != 0 && blanca(p):
		t.c[j.af][j.ac] = j.corona - 'a' + 'A'
	case j.corona != 0:
		t.c[j.af][j.ac] = j.corona
	case (p == 'P' && j.af == 0) || (p == 'p' && j.af == 7):
		t.c[j.af][j.ac] = p - 'P' + 'Q'
	}
	if (p == 'K' || p == 'k') && j.df == j.af && abs(j.ac-j.dc) == 2 {
		if j.ac > j.dc {
			t.c[j.af][5], t.c[j.af][7] = t.c[j.af][7], 0
		} else {
			t.c[j.af][3], t.c[j.af][0] = t.c[j.af][0], 0
		}
	}
	// los enroques que se pierden
	quitar := func(s string) { t.enroques = strings.NewReplacer(string(s[0]), "").Replace(t.enroques) }
	switch p {
	case 'K':
		quitar("K")
		quitar("Q")
	case 'k':
		quitar("k")
		quitar("q")
	}
	for _, x := range [][3]int{{7, 7, 'K'}, {7, 0, 'Q'}, {0, 7, 'k'}, {0, 0, 'q'}} {
		if (j.df == x[0] && j.dc == x[1]) || (j.af == x[0] && j.ac == x[1]) {
			quitar(string(rune(x[2])))
		}
	}
	t.alPaso = [2]int{-1, -1}
	if (p == 'P' || p == 'p') && abs(j.af-j.df) == 2 {
		t.alPaso = [2]int{(j.af + j.df) / 2, j.dc}
	}
	t.blancas = !t.blancas
	return comida
}

func dentro(f, c int) bool { return f >= 0 && f < 8 && c >= 0 && c < 8 }

var (
	rectas     = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	diagonales = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	saltosCab  = [][2]int{{1, 2}, {2, 1}, {-1, 2}, {-2, 1}, {1, -2}, {2, -1}, {-1, -2}, {-2, -1}}
)

// atacada: si las piezas de ese color atacan (f, c).
func (t tablero) atacada(f, c int, porBlancas bool) bool {
	de := func(p byte) bool { return p != 0 && blanca(p) == porBlancas }
	tipo := func(p byte) byte { return p | 0x20 }
	for _, d := range saltosCab {
		if nf, nc := f+d[0], c+d[1]; dentro(nf, nc) && de(t.c[nf][nc]) && tipo(t.c[nf][nc]) == 'n' {
			return true
		}
	}
	for _, d := range append(append([][2]int{}, rectas...), diagonales...) {
		if nf, nc := f+d[0], c+d[1]; dentro(nf, nc) && de(t.c[nf][nc]) && tipo(t.c[nf][nc]) == 'k' {
			return true
		}
	}
	// peones: atacan en diagonal hacia delante
	paso := 1 // un peón blanco que ataca (f, c) está en la fila de abajo
	if !porBlancas {
		paso = -1
	}
	for _, dc := range []int{-1, 1} {
		if nf, nc := f+paso, c+dc; dentro(nf, nc) && de(t.c[nf][nc]) && tipo(t.c[nf][nc]) == 'p' {
			return true
		}
	}
	rayos := func(dirs [][2]int, piezas string) bool {
		for _, d := range dirs {
			for k := 1; k < 8; k++ {
				nf, nc := f+d[0]*k, c+d[1]*k
				if !dentro(nf, nc) {
					break
				}
				if q := t.c[nf][nc]; q != 0 {
					if de(q) && strings.IndexByte(piezas, tipo(q)) >= 0 {
						return true
					}
					break
				}
			}
		}
		return false
	}
	return rayos(rectas, "rq") || rayos(diagonales, "bq")
}

func (t tablero) rey(deBlancas bool) (int, int, bool) {
	r := byte('k')
	if deBlancas {
		r = 'K'
	}
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			if t.c[f][c] == r {
				return f, c, true
			}
		}
	}
	return 0, 0, false
}

// enJaque: si al que le toca le dan jaque.
func (t tablero) enJaque() bool {
	f, c, ok := t.rey(t.blancas)
	return ok && t.atacada(f, c, !t.blancas)
}

// destinos: adónde puede ir de verdad la pieza de (f, c) (sin dejar a su rey en jaque).
func (t tablero) destinos(f, c int) [][2]int {
	p := t.c[f][c]
	if p == 0 || blanca(p) != t.blancas {
		return nil
	}
	var out [][2]int
	for _, d := range t.posibles(f, c) {
		prueba := t
		prueba.mover(jugada{df: f, dc: c, af: d[0], ac: d[1]})
		if rf, rc, ok := prueba.rey(t.blancas); ok && prueba.atacada(rf, rc, !t.blancas) {
			continue
		}
		out = append(out, d)
	}
	return out
}

// posibles: adónde va la pieza según cómo se mueve (sin mirar los jaques, salvo en el enroque).
func (t tablero) posibles(f, c int) [][2]int {
	p := t.c[f][c]
	mia := blanca(p)
	var out [][2]int
	libreORival := func(f, c int) bool { q := t.c[f][c]; return q == 0 || blanca(q) != mia }
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
	switch p | 0x20 {
	case 'r':
		deslizar(rectas)
	case 'b':
		deslizar(diagonales)
	case 'q':
		deslizar(append(append([][2]int{}, rectas...), diagonales...))
	case 'n':
		saltos(saltosCab)
	case 'k':
		saltos(append(append([][2]int{}, rectas...), diagonales...))
		fila, corto, largo := 7, byte('K'), byte('Q')
		if !mia {
			fila, corto, largo = 0, 'k', 'q'
		}
		torre := byte('R')
		if !mia {
			torre = 'r'
		}
		if f == fila && c == 4 && !t.atacada(fila, 4, !mia) {
			if strings.IndexByte(t.enroques, corto) >= 0 && t.c[fila][7] == torre && t.c[fila][5] == 0 && t.c[fila][6] == 0 &&
				!t.atacada(fila, 5, !mia) && !t.atacada(fila, 6, !mia) {
				out = append(out, [2]int{fila, 6})
			}
			if strings.IndexByte(t.enroques, largo) >= 0 && t.c[fila][0] == torre && t.c[fila][1] == 0 && t.c[fila][2] == 0 &&
				t.c[fila][3] == 0 && !t.atacada(fila, 3, !mia) && !t.atacada(fila, 2, !mia) {
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
			nf, nc := f+paso, c+dc
			if !dentro(nf, nc) {
				continue
			}
			if (t.c[nf][nc] != 0 && blanca(t.c[nf][nc]) != mia) || (t.alPaso == [2]int{nf, nc}) {
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

// hayJugadas: si al que le toca le queda alguna jugada (si no: mate o ahogado).
func (t tablero) hayJugadas() bool {
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			if q := t.c[f][c]; q != 0 && blanca(q) == t.blancas && len(t.destinos(f, c)) > 0 {
				return true
			}
		}
	}
	return false
}

// nombrePieza: «Caballo», «Dama»…
func nombrePieza(p byte) string {
	return map[byte]string{'k': "Rey", 'q': "Dama", 'r': "Torre", 'b': "Alfil", 'n': "Caballo", 'p': "Peón"}[p|0x20]
}

// legible: «g1f3» en la posición de antes → «Cf3» (la notación de siempre, en español: R D T A C).
func (t tablero) legible(j jugada) string {
	p := t.c[j.df][j.dc]
	if p == 0 {
		return j.uci()
	}
	if (p == 'K' || p == 'k') && abs(j.ac-j.dc) == 2 {
		if j.ac > j.dc {
			return "0-0"
		}
		return "0-0-0"
	}
	letra := map[byte]string{'k': "R", 'q': "D", 'r': "T", 'b': "A", 'n': "C", 'p': ""}[p|0x20]
	come := t.c[j.af][j.ac] != 0 || ((p|0x20) == 'p' && j.dc != j.ac)
	s := letra
	if (p | 0x20) == 'p' {
		if come {
			s = string(rune('a' + j.dc))
		}
	} else {
		// si otra igual puede ir a la misma casilla, se dice de cuál
		for f := 0; f < 8; f++ {
			for c := 0; c < 8; c++ {
				if (f != j.df || c != j.dc) && t.c[f][c] == p && t.puedeIr(f, c, j.af, j.ac) {
					if c != j.dc {
						s += string(rune('a' + j.dc))
					} else {
						s += string(rune('8' - j.df))
					}
				}
			}
		}
	}
	if come {
		s += "x"
	}
	s += nombreCasilla(j.af, j.ac)
	if j.corona != 0 || ((p|0x20) == 'p' && (j.af == 0 || j.af == 7)) {
		cor := j.corona
		if cor == 0 {
			cor = 'q'
		}
		s += "=" + map[byte]string{'q': "D", 'r': "T", 'b': "A", 'n': "C"}[cor]
	}
	despues := t
	despues.mover(j)
	if despues.enJaque() {
		if despues.hayJugadas() {
			s += "+"
		} else {
			s += "#"
		}
	}
	return s
}
