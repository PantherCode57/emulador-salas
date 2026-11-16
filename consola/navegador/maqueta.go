package main

import (
	"image/color"
	"strings"
	"unicode"

	"golang.org/x/image/font"
)

// Maqueta: la página puesta en líneas del ancho de la pantalla, con el sitio de cada enlace.
const (
	margen    = 24
	topePag   = 52 // debajo de la barra de arriba
	maxAltImg = 300
)

// altoVista: lo que se ve de la página (entre la barra de arriba y la línea de botones).
func altoVista() int { return alto - topePag - 34 }

var colEnlace = color.RGBA{0x8c, 0xc8, 0xff, 0xff}

type pieza struct {
	x          int
	texto      string
	cara       font.Face
	col        color.Color
	enlace     int
	ocurrencia int // cuál de los enlaces de la página es (en orden), -1 si no es un enlace
}

type linea struct {
	y, alto int
	piezas  []pieza
	bloque  int
	sangria int
	vineta  bool
	cita    bool
	// Las imágenes ocupan una línea entera.
	img        bool
	imgW, imgH int
}

type ocurrencia struct {
	enlace int
	linea  int
}

type maqueta struct {
	lineas      []linea
	alto        int
	ocurrencias []ocurrencia
}

func anchoTexto() int { return ancho - 2*margen }

// maquetar pone la página en líneas. tam dice lo que mide cada imagen ya bajada (0, 0 si aún no).
func maquetar(p *pagina, f *fuentes, tam func(src string) (int, int)) *maqueta {
	m := &maqueta{}
	y := 6
	ultimaOc := -1
	for bi, b := range p.bloques {
		sangria := 0
		var cara font.Face = f.normal
		col := color.Color(colTexto)
		antes := 10
		switch b.tipo {
		case bTitulo1:
			cara, antes = f.titulo, 18
		case bTitulo2:
			cara, antes = f.titulo2, 16
		case bTitulo3:
			cara, antes = f.nombre, 12
		case bItem:
			sangria, antes = 18+16*max(b.nivel-1, 0), 4
		case bCita:
			sangria, col = 16, colSuave
		case bCodigo:
			cara, col = f.pequena, colSuave
		}
		if bi == 0 {
			antes = 0
		}
		y += antes
		if b.tipo == bImagen {
			w, h := tam(b.img)
			if w < 0 {
				continue
			}
			if w == 0 || h == 0 {
				w, h = b.ancho, b.alto
			}
			dw, dh := anchoTexto()-sangria, 150 // hueco mientras llega
			if w > 0 && h > 0 {
				dw = min(anchoTexto()-sangria, w)
				dh = h * dw / w
				if dh > maxAltImg {
					dh, dw = maxAltImg, w*maxAltImg/h
				}
			}
			m.lineas = append(m.lineas, linea{y: y, alto: dh, bloque: bi, sangria: sangria, img: true, imgW: dw, imgH: dh})
			y += dh
			if b.alt != "" && len(b.alt) < 300 {
				m.lineas = append(m.lineas, m.partir(b, bi, []trozo{{texto: b.alt, enlace: -1}}, f.mini, colApagado, sangria, &y, &ultimaOc)...)
			}
			continue
		}
		primera := len(m.lineas)
		m.lineas = append(m.lineas, m.partir(b, bi, b.trozos, cara, col, sangria, &y, &ultimaOc)...)
		if b.tipo == bItem && len(m.lineas) > primera {
			m.lineas[primera].vineta = true
		}
		for i := primera; i < len(m.lineas); i++ {
			m.lineas[i].cita = b.tipo == bCita
		}
		if b.tipo != bParrafo || len(b.trozos) > 0 {
			ultimaOc = -1 // un enlace no sigue de un bloque al siguiente
		}
	}
	m.alto = y + 20
	return m
}

// partir reparte los trozos de un bloque en líneas del ancho que hay.
func (m *maqueta) partir(b bloque, bi int, trozos []trozo, cara font.Face, col color.Color, sangria int, y *int, ultimaOc *int) []linea {
	alturaLinea := cara.Metrics().Height.Round() + 5
	maxW := anchoTexto() - sangria
	var lineas []linea
	act := linea{y: *y, alto: alturaLinea, bloque: bi, sangria: sangria}
	x := 0
	nueva := func() {
		lineas = append(lineas, act)
		*y += alturaLinea
		act = linea{y: *y, alto: alturaLinea, bloque: bi, sangria: sangria}
		x = 0
	}
	for _, t := range trozos {
		c, colT := cara, col
		if t.negrita && cara == fuentesActuales.normal {
			c = fuentesActuales.negrita
		}
		if t.enlace >= 0 {
			colT = colEnlace
		}
		oc := -1
		if t.enlace >= 0 {
			if *ultimaOc >= 0 && m.ocurrencias[*ultimaOc].enlace == t.enlace {
				oc = *ultimaOc
			} else {
				m.ocurrencias = append(m.ocurrencias, ocurrencia{enlace: t.enlace, linea: len(m.lineas) + len(lineas)})
				oc = len(m.ocurrencias) - 1
			}
		}
		*ultimaOc = oc
		for _, pal := range palabras(fuentesActuales.limpiarSuave(t.texto)) {
			if pal == "\n" {
				if x > 0 { // los saltos seguidos no dejan líneas vacías
					nueva()
				}
				continue
			}
			w := medir(c, pal)
			if x+w > maxW && x > 0 {
				nueva()
				pal = strings.TrimLeft(pal, " ")
				w = medir(c, pal)
			}
			for w > maxW { // una palabra más larga que la línea (direcciones): se corta
				r := []rune(pal)
				n := len(r)
				for n > 1 && medir(c, string(r[:n])) > maxW-x {
					n--
				}
				act.piezas = append(act.piezas, pieza{x: x, texto: string(r[:n]), cara: c, col: colT, enlace: t.enlace, ocurrencia: oc})
				nueva()
				pal = string(r[n:])
				w = medir(c, pal)
			}
			if x == 0 {
				pal = strings.TrimLeft(pal, " ")
				w = medir(c, pal)
			}
			if pal == "" {
				continue
			}
			act.piezas = append(act.piezas, pieza{x: x, texto: pal, cara: c, col: colT, enlace: t.enlace, ocurrencia: oc})
			x += w
		}
	}
	if len(act.piezas) > 0 {
		lineas = append(lineas, act)
		*y += alturaLinea
	}
	return lineas
}

// palabras: el texto en trozos que no se parten («palabra » con su espacio detrás) y los saltos.
func palabras(s string) []string {
	var out []string
	var b strings.Builder
	enEspacio := false
	for _, r := range s {
		switch {
		case r == '\n':
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			out = append(out, "\n")
			enEspacio = false
		case unicode.IsSpace(r):
			b.WriteRune(' ')
			enEspacio = true
		default:
			if enEspacio {
				out = append(out, b.String())
				b.Reset()
				enEspacio = false
			}
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// fuentesActuales: las letras de la app (para la negrita al partir líneas).
var fuentesActuales *fuentes
