package main

import (
	"strings"
	"time"
)

// Los problemas de Lichess: la posición (después de la jugada del rival) y la solución en UCI, en la
// que tus jugadas y las del rival van alternas. Vale también cualquier jaque mate, como en Lichess.
type problemaVivo struct {
	pr                  problema
	v                   *vistaTablero
	paso                int // por dónde va la solución
	rivalEn             time.Time
	estado              string // "cargando", "jugando", "bien", "mal", "error"
	err                 string
	buena               string // la jugada que era, si fallas
	resueltos, fallados int
}

func (a *app) nuevoProblema() {
	if !hayRed() {
		a.aviso("Sin internet")
		return
	}
	viejo := a.pz
	a.pz = &problemaVivo{estado: "cargando"}
	if viejo != nil {
		a.pz.resueltos, a.pz.fallados = viejo.resueltos, viejo.fallados
	}
	pz := a.pz
	a.pant = pProblema
	go func() {
		pr, err := a.li.Problema("")
		a.hacer(func() {
			if a.pz != pz {
				return
			}
			if err != nil {
				pz.estado, pz.err = "error", textoError(err)
				return
			}
			t, ok := posicionProblema(pr)
			if !ok {
				pz.estado, pz.err = "error", "Este problema no se ha podido leer. Pulsa A para otro."
				return
			}
			pz.pr, pz.estado = pr, "jugando"
			pz.v = nuevaVista(t, t.blancas)
			if j, ok := leerUCI(pr.Ultima); ok {
				pz.v.marcarUltima(j)
			}
			// el cursor, en la pieza que hay que mover no: en el centro (que no sea una pista)
			if t.blancas {
				pz.v.cur = [2]int{6, 4}
			} else {
				pz.v.cur = [2]int{1, 3}
			}
		})
	}()
}

// posicionProblema: con el FEN si lo trae; si no, repitiendo la partida (en PGN) hasta el problema.
// Se mira que la primera jugada de la solución se pueda hacer.
func posicionProblema(pr problema) (tablero, bool) {
	vale := func(t tablero) bool {
		j, ok := leerUCI(pr.Solucion[0])
		return ok && t.puedeIr(j.df, j.dc, j.af, j.ac)
	}
	if pr.FEN != "" {
		if t, ok := leerFEN(pr.FEN); ok && vale(t) {
			return t, true
		}
	}
	if pr.Jugadas != "" {
		if t, ok := jugarPGN(pr.Jugadas); ok && vale(t) {
			return t, true
		}
	}
	return tablero{}, false
}

// jugarPGN: la posición después de las jugadas escritas en inglés («e4 e5 Nf3 Nc6 O-O …»).
func jugarPGN(pgn string) (tablero, bool) {
	t, _ := leerFEN(fenInicial)
	for _, tok := range strings.Fields(pgn) {
		if strings.HasSuffix(tok, ".") || strings.Contains(tok, "...") || tok == "1-0" || tok == "0-1" || tok == "1/2-1/2" || tok == "*" {
			continue
		}
		if i := strings.Index(tok, "."); i >= 0 { // «12.e4»
			tok = tok[i+1:]
		}
		tok = strings.TrimRight(tok, "+#!?")
		j, ok := t.deSAN(tok)
		if !ok {
			return t, false
		}
		t.mover(j)
	}
	return t, true
}

// deSAN: qué jugada es «Nf3» (en inglés: K Q R B N) en esta posición.
func (t tablero) deSAN(s string) (jugada, bool) {
	for f := 0; f < 8; f++ {
		for c := 0; c < 8; c++ {
			q := t.c[f][c]
			if q == 0 || blanca(q) != t.blancas {
				continue
			}
			for _, d := range t.destinos(f, c) {
				j := jugada{df: f, dc: c, af: d[0], ac: d[1]}
				if (q == 'P' && d[0] == 0) || (q == 'p' && d[0] == 7) {
					for _, cor := range []byte("qrbn") {
						j.corona = cor
						if sanIngles(t, j) == s {
							return j, true
						}
					}
					continue
				}
				if sanIngles(t, j) == s {
					return j, true
				}
			}
		}
	}
	return jugada{}, false
}

// sanIngles: la jugada como en un PGN (sin + ni #).
func sanIngles(t tablero, j jugada) string {
	es := t.legible(j)
	es = strings.TrimRight(es, "+#")
	if es == "0-0" {
		return "O-O"
	}
	if es == "0-0-0" {
		return "O-O-O"
	}
	cambio := strings.NewReplacer("R", "K", "D", "Q", "T", "R", "A", "B", "C", "N")
	if i := strings.Index(es, "="); i >= 0 {
		return cambio.Replace(es[:i]) + "=" + cambio.Replace(es[i+1:])
	}
	return cambio.Replace(es)
}

func (a *app) pulsarProblema(b uint16) {
	pz := a.pz
	if pz == nil {
		a.pant = pMenu
		return
	}
	switch pz.estado {
	case "cargando":
		if b == bB {
			a.pant = pMenu
		}
		return
	case "error", "bien", "mal":
		switch b {
		case bA, bStart:
			a.nuevoProblema()
		case bY:
			if pz.estado == "mal" { // otra vez el mismo
				t, _ := posicionProblema(pz.pr)
				pz.v = nuevaVista(t, t.blancas)
				pz.paso, pz.estado, pz.buena = 0, "jugando", ""
			}
		case bB:
			a.pant = pMenu
		}
		return
	}
	if !pz.rivalEn.IsZero() {
		return
	}
	switch b {
	case bB:
		if pz.v.sel[0] < 0 && !pz.v.coronando {
			a.pant = pMenu
			return
		}
	case bY:
		if j, ok := leerUCI(pz.pr.Solucion[pz.paso]); ok {
			pz.v.pista = [2]int{j.df, j.dc}
		}
		return
	}
	j, hecha := pz.v.pulsar(b, true)
	if !hecha {
		return
	}
	esperada := pz.pr.Solucion[pz.paso]
	m := j.uci()
	despues := pz.v.tab
	despues.mover(j)
	mate := despues.enJaque() && !despues.hayJugadas()
	come := pz.v.tab.c[j.af][j.ac] != 0
	if m != esperada && !(len(esperada) == 4 && strings.HasPrefix(m, esperada)) && !mate {
		ej, _ := leerUCI(esperada)
		pz.buena = pz.v.tab.legible(ej)
		pz.v.tab = despues
		pz.v.marcarUltima(j)
		pz.estado = "mal"
		pz.fallados++
		a.son.mal()
		go a.li.MandarProblema(pz.pr.ID, false)
		return
	}
	pz.v.tab = despues
	pz.v.marcarUltima(j)
	pz.v.pista = [2]int{-1, -1}
	pz.paso++
	if despues.enJaque() {
		a.son.jaque()
	} else {
		a.son.jugada(come)
	}
	if mate || pz.paso >= len(pz.pr.Solucion) {
		pz.estado = "bien"
		pz.resueltos++
		a.son.bien()
		go a.li.MandarProblema(pz.pr.ID, true)
		return
	}
	pz.rivalEn = time.Now().Add(600 * time.Millisecond) // ahora contesta el rival
}

func (a *app) moverRivalProblema() {
	pz := a.pz
	if pz.rivalEn.IsZero() || time.Now().Before(pz.rivalEn) {
		return
	}
	pz.rivalEn = time.Time{}
	a.cambios = true
	j, ok := leerUCI(pz.pr.Solucion[pz.paso])
	if !ok {
		pz.estado = "bien"
		return
	}
	come := pz.v.tab.c[j.af][j.ac] != 0
	pz.v.tab.mover(j)
	pz.v.marcarUltima(j)
	a.son.jugada(come)
	pz.paso++
	if pz.paso >= len(pz.pr.Solucion) {
		pz.estado = "bien"
		pz.resueltos++
		a.son.bien()
		go a.li.MandarProblema(pz.pr.ID, true)
	}
}
