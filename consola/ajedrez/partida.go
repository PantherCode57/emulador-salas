package main

import (
	"context"
	"strings"
	"time"
)

// Una partida que se está jugando: lo que manda Lichess por el stream de la partida (gameFull al
// principio y gameState con cada jugada) y lo que haces tú.
type partidaViva struct {
	id                      string
	v                       *vistaTablero
	blanco                  jugador
	negro                   jugador
	fenInicio               string
	jugadas                 string   // las que ha dicho Lichess (UCI separadas por espacios)
	san                     []string // las mismas, como se escriben («e4», «Cf3»…)
	tabLichess              tablero  // la posición según Lichess (la de v puede ir una jugada por delante)
	conReloj                bool
	wtime                   int // milisegundos
	btime                   int
	recibido                time.Time
	estado                  string // started, mate, resign, outoftime, draw, stalemate, aborted…
	ganador                 string // white, black o ""
	tablasRival, tablasMias bool
	rivalFuera              bool
	enviando                bool
	cancelar                context.CancelFunc
	cargada                 bool
}

type jugador struct {
	Nombre  string
	Elo     int
	Maquina int
}

func (a *app) abrirPartida(id string) {
	if a.pa != nil && a.pa.cancelar != nil {
		a.pa.cancelar()
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &partidaViva{id: id, cancelar: cancel, estado: "started"}
	a.pa, a.pant, a.menuPartida = p, pPartida, false
	go func() {
		for intento := 0; intento < 3; intento++ {
			err := a.li.stream(ctx, "GET", "/api/board/game/stream/"+id, nil, func(m map[string]any) {
				a.hacer(func() {
					if a.pa == p {
						a.alEstadoPartida(m)
					}
				})
			})
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				a.hacer(func() { a.aviso(textoError(err)) })
			}
			acabada := make(chan bool, 1)
			a.hacer(func() { acabada <- p.terminada() })
			select {
			case fin := <-acabada:
				if fin {
					return
				}
			case <-a.salir:
				return
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

func jugadorDe(m map[string]any) jugador {
	j := jugador{Nombre: texto(m, "name"), Elo: entero(m, "rating"), Maquina: entero(m, "aiLevel")}
	if j.Maquina > 0 {
		j.Nombre = "Máquina de Lichess, nivel " + itoa(j.Maquina)
	}
	if j.Nombre == "" {
		j.Nombre = "Anónimo"
	}
	return j
}

func (a *app) alEstadoPartida(m map[string]any) {
	p := a.pa
	switch texto(m, "type") {
	case "gameFull":
		p.blanco, p.negro = jugadorDe(obj(m, "white")), jugadorDe(obj(m, "black"))
		p.fenInicio = texto(m, "initialFen")
		p.conReloj = obj(m, "clock") != nil
		soyBlancas := strings.EqualFold(texto(obj(m, "white"), "id"), a.yo.ID)
		if !soyBlancas && !strings.EqualFold(texto(obj(m, "black"), "id"), a.yo.ID) && a.yo.ID == "" {
			soyBlancas = true // sin saber quién eres (no debería pasar)
		}
		t, _ := posicionDe(p.fenInicio, "")
		p.v = nuevaVista(t, soyBlancas)
		p.jugadas = "\x00" // para que el primer estado se aplique siempre
		p.cargada = true
		a.alEstadoPartida(obj(m, "state"))
	case "gameState":
		if p.v == nil {
			return
		}
		jugadas := texto(m, "moves")
		if jugadas != p.jugadas {
			antes := len(strings.Fields(p.jugadas))
			p.jugadas = jugadas
			t, hechas := posicionDe(p.fenInicio, jugadas)
			p.tabLichess = t
			// lo que se escribe de cada jugada
			p.san = p.san[:0]
			tt, _ := posicionDe(p.fenInicio, "")
			for _, j := range hechas {
				p.san = append(p.san, tt.legible(j))
				tt.mover(j)
			}
			nueva := len(hechas) > antes && p.jugadas != "\x00"
			p.v.tab = t
			p.v.sel = [2]int{-1, -1}
			if len(hechas) > 0 {
				p.v.marcarUltima(hechas[len(hechas)-1])
				if nueva && antes >= 0 {
					ult := p.san[len(p.san)-1]
					if t.enJaque() {
						a.son.jaque()
					} else {
						a.son.jugada(strings.Contains(ult, "x"))
					}
				}
			}
			p.enviando = false
			p.tablasMias = false
		}
		p.wtime, p.btime, p.recibido = entero(m, "wtime"), entero(m, "btime"), time.Now()
		antes := p.estado
		p.estado = texto(m, "status")
		p.ganador = texto(m, "winner")
		if p.v.conBlancas {
			p.tablasRival = verdad(m, "bdraw")
		} else {
			p.tablasRival = verdad(m, "wdraw")
		}
		if antes == "started" && p.terminada() {
			a.son.fin(p.ganador != "" && (p.ganador == "white") == p.v.conBlancas)
			if a.luz.apagada {
				a.luz.encender()
			}
		}
	case "opponentGone":
		p.rivalFuera = verdad(m, "gone")
	}
}

func (p *partidaViva) terminada() bool {
	return p.estado != "" && p.estado != "started" && p.estado != "created"
}

func (p *partidaViva) meToca() bool {
	return p.v != nil && !p.terminada() && p.tabLichess.blancas == p.v.conBlancas && !p.enviando
}

// relojes: lo que le queda a cada uno ahora (el del que mueve va bajando desde el último aviso).
func (p *partidaViva) relojes() (blancas, negras int) {
	blancas, negras = p.wtime, p.btime
	if !p.conReloj || p.terminada() || len(p.san) < 2 {
		return
	}
	pasado := int(time.Since(p.recibido).Milliseconds())
	if p.tabLichess.blancas {
		blancas = max(0, blancas-pasado)
	} else {
		negras = max(0, negras-pasado)
	}
	return
}

func (a *app) pulsarPartida(b uint16) {
	p := a.pa
	if p == nil || p.v == nil {
		if b == bB {
			a.salirDePartida()
		}
		return
	}
	if a.menuPartida {
		a.pulsarMenuPartida(b)
		return
	}
	if len(a.retos) > 0 && b == bY { // en mitad de una partida, Y rechaza el reto que ha llegado
		r := a.retos[0]
		a.retos = a.retos[1:]
		go func() { _ = a.li.Rechazar(r.ID) }()
		return
	}
	if p.terminada() {
		if b == bA || b == bB || b == bStart {
			a.salirDePartida()
		}
		return
	}
	switch b {
	case bStart:
		a.menuPartida, a.mpSel = true, 0
		return
	case bB:
		if p.v.sel[0] < 0 && !p.v.coronando {
			a.menuPartida, a.mpSel = true, 0
			return
		}
		if !p.v.coronando {
			p.v.sel = [2]int{-1, -1}
			return
		}
	}
	j, hecha := p.v.pulsar(b, p.meToca())
	if b == bA && !hecha && p.v.sel[0] < 0 && !p.v.coronando && !p.meToca() {
		if q := p.v.tab.c[p.v.cur[0]][p.v.cur[1]]; p.v.mia(q) {
			a.aviso("Espera: le toca al rival")
		}
	}
	if !hecha {
		return
	}
	// la jugada se ve ya; si Lichess dice que no, se vuelve a poner como estaba
	uci := j.uci()
	come := p.v.tab.c[j.af][j.ac] != 0
	p.v.tab.mover(j)
	p.v.marcarUltima(j)
	p.enviando = true
	if p.v.tab.enJaque() {
		a.son.jaque()
	} else {
		a.son.jugada(come)
	}
	id := p.id
	go func() {
		if err := a.li.Mover(id, uci); err != nil {
			a.hacer(func() {
				if a.pa != p {
					return
				}
				p.enviando = false
				p.v.tab = p.tabLichess
				a.aviso("No se ha podido mandar la jugada: " + textoError(err))
			})
		}
	}()
}

// las opciones de START en la partida
func (a *app) opcionesPartida() []string {
	p := a.pa
	var ops []string
	if p.tablasRival {
		ops = append(ops, "Aceptar las tablas", "Rechazar las tablas")
	} else if !p.tablasMias {
		ops = append(ops, "Ofrecer tablas")
	}
	if len(p.san) < 2 {
		ops = append(ops, "Anular la partida")
	} else {
		ops = append(ops, "Rendirse")
	}
	return append(ops, "Salir al menú (la partida sigue)", "Seguir jugando")
}

func (a *app) pulsarMenuPartida(b uint16) {
	ops := a.opcionesPartida()
	p := a.pa
	switch b {
	case bArriba:
		a.mpSel = (a.mpSel + len(ops) - 1) % len(ops)
	case bAbajo:
		a.mpSel = (a.mpSel + 1) % len(ops)
	case bB, bStart:
		a.menuPartida = false
	case bA:
		a.menuPartida = false
		id := p.id
		hacer := func(fn func() error) {
			go func() {
				if err := fn(); err != nil {
					a.hacer(func() { a.aviso(textoError(err)) })
				}
			}()
		}
		switch ops[a.mpSel] {
		case "Aceptar las tablas":
			hacer(func() error { return a.li.Tablas(id, true) })
		case "Rechazar las tablas":
			p.tablasRival = false
			hacer(func() error { return a.li.Tablas(id, false) })
		case "Ofrecer tablas":
			p.tablasMias = true
			a.aviso("Tablas ofrecidas")
			hacer(func() error { return a.li.Tablas(id, true) })
		case "Anular la partida":
			hacer(func() error { return a.li.Abortar(id) })
		case "Rendirse":
			a.confirmar("¿Seguro que te rindes? Pierdes la partida.", func() {
				hacer(func() error { return a.li.Rendirse(id) })
			})
		case "Salir al menú (la partida sigue)":
			a.salirDePartida()
		}
	}
}

func (a *app) salirDePartida() {
	if a.pa != nil && a.pa.cancelar != nil {
		a.pa.cancelar()
	}
	a.pa, a.menuPartida = nil, false
	a.pant = pMenu
	a.cargarCuenta()
}

// textoFinal: cómo ha acabado («Has ganado por jaque mate»).
func (p *partidaViva) textoFinal() (string, string) {
	por := map[string]string{"mate": "por jaque mate", "resign": "por abandono", "outoftime": "por tiempo",
		"timeout": "porque se fue", "stalemate": "por rey ahogado", "draw": "", "cheat": "por trampas",
		"noStart": "porque no empezó", "variantEnd": ""}[p.estado]
	switch {
	case p.estado == "aborted":
		return "Partida anulada", "Nadie gana ni pierde."
	case p.ganador == "":
		if p.estado == "stalemate" {
			return "Tablas", "Por rey ahogado."
		}
		return "Tablas", "Nadie gana."
	case (p.ganador == "white") == p.v.conBlancas:
		return "¡Has ganado!", strings.TrimSpace("Ganas " + por + ".")
	default:
		return "Has perdido", strings.TrimSpace("Pierdes " + por + ".")
	}
}
