package main

import "testing"

// perft: cuántas partidas distintas hay a esa profundidad (los números de siempre de las posiciones
// de prueba del ajedrez por ordenador). Cuenta cada coronación como cuatro jugadas.
func perft(t tablero, prof int) int {
	if prof == 0 {
		return 1
	}
	n := 0
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			for _, d := range t.destinos(f, c) {
				p := t.c[f][c]
				coronas := []byte{0}
				if (p == 'P' && d[0] == 0) || (p == 'p' && d[0] == 7) {
					coronas = []byte{'q', 'r', 'b', 'n'}
				}
				for _, cor := range coronas {
					s := t
					s.mover(jugada{df: f, dc: c, af: d[0], ac: d[1], corona: cor})
					n += perft(s, prof-1)
				}
			}
		}
	}
	return n
}

func TestPerft(t *testing.T) {
	casos := []struct {
		fen  string
		prof int
		n    int
	}{
		{fenInicial, 1, 20},
		{fenInicial, 3, 8902},
		{fenInicial, 4, 197281},
		{"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 1, 48},
		{"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 2, 2039},
		{"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 3, 97862},
		{"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 4, 43238},
		{"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 3, 9467},
		{"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", 3, 62379},
	}
	for _, c := range casos {
		tab, ok := leerFEN(c.fen)
		if !ok {
			t.Fatalf("no lee %s", c.fen)
		}
		if n := perft(tab, c.prof); n != c.n {
			t.Errorf("perft(%s, %d) = %d, debería ser %d", c.fen, c.prof, n, c.n)
		}
	}
}

func TestLegible(t *testing.T) {
	tab, _ := posicionDe("startpos", "e2e4 e7e5 g1f3 b8c6 f1c4 g8f6 f3g5 d7d5")
	j, _ := leerUCI("e4d5")
	if s := tab.legible(j); s != "exd5" {
		t.Errorf("exd5: %s", s)
	}
	tab, _ = posicionDe("startpos", "f2f3 e7e5 g2g4")
	j, _ = leerUCI("d8h4")
	if s := tab.legible(j); s != "Dh4#" {
		t.Errorf("Dh4#: %s", s)
	}
	tab, _ = posicionDe("startpos", "e2e4 e7e5 g1f3 b8c6 f1c4 g8f6")
	j, _ = leerUCI("e1g1")
	if s := tab.legible(j); s != "0-0" {
		t.Errorf("0-0: %s", s)
	}
}
