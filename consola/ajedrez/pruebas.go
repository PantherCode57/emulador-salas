package main

import (
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas: las pantallas con datos inventados, en PNG (para verlas sin consola).
func dibujarPruebas(f *fuentes, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	a := nuevaApp(f, &lichess{cuenta: &cuenta{}})
	a.son.callado = true
	guardar := func(nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}
	a.cargaHasta = time.Now().Add(time.Hour)
	if err := guardar("01-carga"); err != nil {
		return err
	}
	a.cargaHasta = time.Time{}
	a.pant, a.sesionTexto = pCuenta, "Esperando tu token…"
	if err := guardar("02-cuenta"); err != nil {
		return err
	}
	a.yo, a.hayYo = miCuenta{Usuario: "alex_juega", ID: "alex_juega", Ratings: map[string]int{"rapid": 1184, "blitz": 1102, "puzzle": 1460}}, true
	a.empezadas = []partidaEnCurso{
		{ID: "a1", Rival: "dylan_ajedrez", RivalElo: 1230, Blancas: true, TeToca: true, Velocidad: "correspondence"},
		{ID: "a2", Rival: "Máquina, nivel 3", Maquina: 3, Blancas: false, Velocidad: "rapid"},
	}
	a.pant = pMenu
	if err := guardar("03-menu"); err != nil {
		return err
	}
	a.pant, a.lista, a.listaSel = pRival, []string{"dylan_ajedrez", "lucia_14"}, 1
	if err := guardar("04-rival"); err != nil {
		return err
	}
	a.rival = "dylan_ajedrez"
	a.irAOpciones(false)
	a.opSel = 0
	if err := guardar("05-opciones"); err != nil {
		return err
	}
	a.irAOpciones(true)
	if err := guardar("06-maquina"); err != nil {
		return err
	}
	a.pant, a.esperandoA = pEsperando, "dylan_ajedrez"
	if err := guardar("07-esperando"); err != nil {
		return err
	}
	a.pant = pEmpezadas
	if err := guardar("08-empezadas"); err != nil {
		return err
	}
	a.pant = pMenu
	a.retos = []retoEntrante{{ID: "r1", De: "dylan_ajedrez", Elo: 1230, Ritmo: "10+0"}}
	if err := guardar("09-reto"); err != nil {
		return err
	}
	a.retos = nil

	// una partida a medias: blancas tú, contra Dylan, con el reloj
	p := &partidaViva{id: "x", estado: "started", conReloj: true}
	a.pa, a.pant = p, pPartida
	a.yo.ID = "alex_juega"
	a.alEstadoPartida(map[string]any{"type": "gameFull", "initialFen": "startpos", "clock": map[string]any{"initial": 600000.0},
		"white": map[string]any{"id": "alex_juega", "name": "alex_juega", "rating": 1184.0},
		"black": map[string]any{"id": "dylan_ajedrez", "name": "dylan_ajedrez", "rating": 1230.0},
		"state": map[string]any{"type": "gameState", "moves": "e2e4 e7e5 g1f3 b8c6 f1c4 g8f6 d2d3 f8c5", "wtime": 512000.0, "btime": 498000.0, "status": "started"}})
	p.recibido = time.Now()
	p.v.cur, p.v.sel = [2]int{5, 5}, [2]int{5, 5}
	p.v.cur = [2]int{3, 6}
	if err := guardar("10-partida"); err != nil {
		return err
	}
	a.menuPartida = true
	if err := guardar("11-menu-partida"); err != nil {
		return err
	}
	a.menuPartida = false
	a.alEstadoPartida(map[string]any{"type": "gameState", "moves": "e2e4 e7e5 g1f3 b8c6 f1c4 g8f6 f3g5 d7d5 e4d5 f6d5 g5f7 e8f7 d1f3 f7e6 b1c3 c6e7 c3d5 e7d5 f3f7",
		"wtime": 300000.0, "btime": 240000.0, "status": "started"})
	if err := guardar("12-jaque"); err != nil {
		return err
	}
	a.alEstadoPartida(map[string]any{"type": "gameState", "moves": "f2f3 e7e5 g2g4 d8h4", "wtime": 590000.0, "btime": 595000.0, "status": "mate", "winner": "black"})
	if err := guardar("13-fin"); err != nil {
		return err
	}

	// coronar
	t, _ := leerFEN("8/4P1k1/8/8/8/8/5K2/8 w - - 0 1")
	p.estado = "started"
	p.v = nuevaVista(t, true)
	p.tabLichess = t
	p.v.coronando, p.v.corona, p.v.opCorona = true, jugada{df: 1, dc: 4, af: 0, ac: 4}, 0
	if err := guardar("14-coronar"); err != nil {
		return err
	}

	// un problema: mate en dos
	a.pa = nil
	pr := problema{ID: "p1", Elo: 1450, FEN: "r1bqkb1r/pppp1ppp/2n2n2/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR w KQkq - 4 4",
		Ultima: "g8f6", Solucion: []string{"h5f7"}, Temas: []string{"mateIn1", "opening", "short"}}
	tp, _ := posicionProblema(pr)
	a.pz = &problemaVivo{pr: pr, v: nuevaVista(tp, true), estado: "jugando", resueltos: 3, fallados: 1}
	a.pz.v.marcarUltima(jugada{df: 0, dc: 6, af: 2, ac: 5})
	a.pant = pProblema
	a.pz.v.sel, a.pz.v.cur = [2]int{3, 7}, [2]int{1, 5}
	if err := guardar("15-problema"); err != nil {
		return err
	}
	a.pz.v.sel = [2]int{-1, -1}
	a.pz.v.cur = [2]int{3, 7}
	a.pulsarProblema(bA)
	a.pz.v.cur = [2]int{1, 5}
	a.pulsarProblema(bA)
	return guardar("16-problema-bien")
}
