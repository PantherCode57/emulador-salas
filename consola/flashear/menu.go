package main

import (
	"fmt"
	"strings"
	"time"
)

// Los menús de «Flashear tarjeta»: se mueve con la cruceta, A elige (o marca), B vuelve y, en las
// listas para marcar, START sigue. Si no se toca nada en 3 minutos, se vuelve atrás.

type tecla int

const (
	tNada tecla = iota
	tArriba
	tAbajo
	tIzq
	tDer
	tA
	tB
	tX
	tY
	tStart
	tSelect
	tOtra
)

type opcion struct {
	titulo, detalle string
	marcada         bool
	apagada         bool // se ve, pero no se puede elegir
}

type menu struct {
	titulo, explica, ayuda string
	ops                    []opcion
	sel                    int
	multiple               bool
}

func (m *menu) conDetalle() bool {
	for _, o := range m.ops {
		if o.detalle != "" {
			return true
		}
	}
	return false
}

const (
	volver  = -1 // B, o nada en 3 minutos
	aceptar = -2 // START en una lista para marcar
)

var esperaMenu = 3 * time.Minute

// paso: lo que hace una tecla en el menú. Devuelve la opción elegida, volver, aceptar o -3 (nada).
func (m *menu) paso(t tecla) int {
	n := len(m.ops)
	switch t {
	case tArriba:
		if n > 0 {
			m.sel = (m.sel + n - 1) % n
		}
	case tAbajo:
		if n > 0 {
			m.sel = (m.sel + 1) % n
		}
	case tB:
		return volver
	case tA:
		if n == 0 || m.ops[m.sel].apagada {
			return -3
		}
		if m.multiple {
			m.ops[m.sel].marcada = !m.ops[m.sel].marcada
			return -3
		}
		return m.sel
	case tStart:
		if m.multiple {
			return aceptar
		}
		if n > 0 && !m.ops[m.sel].apagada {
			return m.sel
		}
	case tX, tY:
		if m.multiple {
			for i := range m.ops {
				if !m.ops[i].apagada {
					m.ops[i].marcada = t == tX
				}
			}
		}
	}
	return -3
}

// elegir: enseña el menú hasta que se elige algo.
func elegir(m *menu, botones <-chan tecla) int {
	for {
		pintarMenu(m)
		select {
		case t := <-botones:
			if r := m.paso(t); r != -3 {
				return r
			}
		case <-time.After(esperaMenu):
			return volver
		}
	}
}

func marcadas(m *menu) []int {
	var r []int
	for i, o := range m.ops {
		if o.marcada {
			r = append(r, i)
		}
	}
	return r
}

// --- Los menús ---

func menuConsolas() *menu {
	m := &menu{titulo: "¿Para qué consola es la tarjeta?",
		explica: "Se copia lo que elijas a la tarjeta de otra consola, cambiándola por la de Panther OS en esta misma ranura. " +
			"No se pasan juegos comerciales: solo Panther OS, sus apps y juegos libres y, si es tuya, tus cuentas y partidas.",
		ayuda: "A elegir     B salir"}
	for _, d := range destinos {
		m.ops = append(m.ops, opcion{titulo: d.nombre, detalle: d.detalle})
	}
	m.ops = append(m.ops, opcion{titulo: "Otras (PSP, 3DS, Wii…)", detalle: "Cuando haya Panther OS para ellas.", apagada: true})
	return m
}

func menuTieneDarkOS() *menu {
	return &menu{titulo: "¿La tarjeta de esa consola ya tiene dArkOS?",
		explica: "Las consolas chinas como la tuya llevan el sistema en la tarjeta. Si la tarjeta ya lo tiene (aunque sea el de fábrica), se le añade Panther OS.",
		ops: []opcion{
			{titulo: "Sí, ya tiene dArkOS", detalle: "Se le añaden la Panther Store y lo que elijas."},
			{titulo: "No, está vacía", detalle: "Primero hay que ponerle el sistema."},
		}}
}

func menuSinDarkOS() *menu {
	return &menu{titulo: "Primero hace falta el sistema",
		explica: "Una tarjeta vacía necesita dArkOS (unos 2-3 GB). Desde aquí todavía no se puede: hay que ponerlo con un PC " +
			"(con Rufus o balenaEtcher, la imagen de dArkOS de esa consola). Luego vuelve aquí y elige «Sí, ya tiene dArkOS».",
		ops:   []opcion{{titulo: "Vale"}},
		ayuda: "A o B volver"}
}

func menuParaQuien() *menu {
	return &menu{titulo: "¿Para quién es?",
		ops: []opcion{
			{titulo: "Para mí", detalle: "Con mis cuentas, mis partidas y mi wifi."},
			{titulo: "Para otra persona", detalle: "Limpia: sin nada tuyo. Esa persona pone sus cuentas."},
		}}
}

func menuApps(apps []appTienda) *menu {
	m := &menu{titulo: "¿Qué le pasas?", multiple: true,
		explica: "Vienen marcadas las que tienes puestas. Se ponen solas en esa consola cuando tenga internet (con la Panther Store)."}
	for _, a := range apps {
		d := "App de Panther OS"
		if a.Tipo == "juego" {
			d = "Juego libre"
		}
		m.ops = append(m.ops, opcion{titulo: a.Nombre, detalle: d, marcada: a.Puesto})
	}
	return m
}

func menuPartidas(ps []partida) *menu {
	m := &menu{titulo: "¿Qué partidas le pasas?", multiple: true,
		explica: "Las de tus juegos de GBA de esta consola. En la flashcard tiene que estar el mismo juego con el mismo nombre."}
	for _, p := range ps {
		m.ops = append(m.ops, opcion{titulo: p.juego, detalle: fmt.Sprintf("%d KB", (p.tam+1023)/1024), marcada: true})
	}
	return m
}

func menuConfirmar(d destino, c carga) *menu {
	return &menu{titulo: "Se va a pasar a: " + d.nombre,
		explica: "· " + strings.Join(c.resumen, "\n· "),
		ops: []opcion{
			{titulo: "Empezar", detalle: "Luego te dirá cuándo sacar la tarjeta de Panther OS."},
			{titulo: "Volver", detalle: "Elegir otra cosa."},
		}}
}

// textoLuego: lo que hay que hacer en la otra consola.
func textoLuego(destino string) string {
	switch destino {
	case "ds":
		return "En la DS: mete la tarjeta en la flashcard y abre «Panther OS» (panther.nds) en su menú."
	case "gba":
		return "En la GBA: mete la tarjeta en la flashcard y abre el juego: tendrá tu partida."
	case "darkos":
		return "En la otra consola: enciéndela y conéctala a un wifi. En cuanto tenga internet se pone todo solo (tarda unos minutos) y se reinicia el menú."
	}
	return ""
}
