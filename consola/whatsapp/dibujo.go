package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Tamaño de la pantalla (el de la XF35H). Se puede cambiar con POKEMULADOR_ANCHO/ALTO.
var ancho, alto = 640, 480

// Colores (tema oscuro, como WhatsApp de noche).
var (
	colFondo    = color.RGBA{0x0b, 0x14, 0x1a, 0xff}
	colBarra    = color.RGBA{0x20, 0x2c, 0x33, 0xff}
	colFila     = color.RGBA{0x2a, 0x39, 0x42, 0xff}
	colMio      = color.RGBA{0x00, 0x5c, 0x4b, 0xff}
	colSuyo     = color.RGBA{0x20, 0x2c, 0x33, 0xff}
	colTexto    = color.RGBA{0xe9, 0xed, 0xef, 0xff}
	colSuave    = color.RGBA{0x86, 0x96, 0xa0, 0xff}
	colVerde    = color.RGBA{0x00, 0xa8, 0x84, 0xff}
	colRojo     = color.RGBA{0xf1, 0x5c, 0x6d, 0xff}
	colAmarillo = color.RGBA{0xff, 0xd2, 0x79, 0xff}
	colBlanco   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colNegro    = color.RGBA{0x00, 0x00, 0x00, 0xff}
	colTecla    = color.RGBA{0x2a, 0x39, 0x42, 0xff}
	colAviso    = color.RGBA{0x10, 0x3a, 0x33, 0xf0}
)

type fontFace = font.Face

type fuentes struct {
	titulo, normal, negrita, pequena, mini font.Face
	reg                                    *sfnt.Font
}

func cargarFuentes() *fuentes {
	reg, err1 := opentype.Parse(goregular.TTF)
	neg, err2 := opentype.Parse(gobold.TTF)
	if err1 != nil || err2 != nil {
		panic("no se pudieron cargar las letras")
	}
	cara := func(f *opentype.Font, tam float64) font.Face {
		c, err := opentype.NewFace(f, &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			panic(err)
		}
		return c
	}
	return &fuentes{
		titulo:  cara(neg, 24),
		normal:  cara(reg, 19),
		negrita: cara(neg, 19),
		pequena: cara(reg, 16),
		mini:    cara(reg, 14),
		reg:     reg,
	}
}

// limpiar quita lo que las letras no saben dibujar (emojis sobre todo), para que no salgan cuadros.
func (f *fuentes) limpiar(s string) string {
	var b strings.Builder
	var buf sfnt.Buffer
	quitados := 0
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if unicode.IsControl(r) || r == 0xfe0f || r == 0x200d {
			continue
		}
		if i, err := f.reg.GlyphIndex(&buf, r); err != nil || i == 0 {
			quitados++
			continue
		}
		b.WriteRune(r)
	}
	t := strings.Join(strings.Fields(b.String()), " ")
	if t == "" && quitados > 0 {
		return "(emoji)"
	}
	return t
}

type lienzo struct {
	img *image.RGBA
	f   *fuentes
}

func nuevoLienzo(f *fuentes) *lienzo {
	return &lienzo{img: image.NewRGBA(image.Rect(0, 0, ancho, alto)), f: f}
}

func (l *lienzo) rect(r image.Rectangle, c color.Color) {
	draw.Draw(l.img, r, &image.Uniform{c}, image.Point{}, draw.Over)
}

// redondeado: rectángulo con las esquinas redondas.
func (l *lienzo) redondeado(r image.Rectangle, radio int, c color.Color) {
	if radio*2 > r.Dy() {
		radio = r.Dy() / 2
	}
	if radio*2 > r.Dx() {
		radio = r.Dx() / 2
	}
	u := &image.Uniform{c}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		dentro := 0
		t := min(y-r.Min.Y, r.Max.Y-1-y)
		if t < radio {
			v := float64(radio) - float64(t) - 0.5
			dentro = int(math.Round(float64(radio) - math.Sqrt(math.Max(0, float64(radio*radio)-v*v))))
		}
		draw.Draw(l.img, image.Rect(r.Min.X+dentro, y, r.Max.X-dentro, y+1), u, image.Point{}, draw.Over)
	}
}

func (l *lienzo) circulo(cx, cy, radio int, c color.Color) {
	l.redondeado(image.Rect(cx-radio, cy-radio, cx+radio, cy+radio), radio, c)
}

func medir(cara font.Face, s string) int { return font.MeasureString(cara, s).Round() }

// texto escribe con la línea base en y; devuelve dónde acaba.
func (l *lienzo) texto(x, y int, s string, cara font.Face, c color.Color) int {
	d := &font.Drawer{Dst: l.img, Src: image.NewUniform(c), Face: cara, Dot: fixed.P(x, y)}
	d.DrawString(s)
	return d.Dot.X.Round()
}

func baseCentrada(r image.Rectangle, cara font.Face) int {
	m := cara.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Round()-m.Descent.Round())/2
}

func (l *lienzo) centrado(r image.Rectangle, s string, cara font.Face, c color.Color) {
	s = recortar(cara, s, r.Dx()-6)
	l.texto(r.Min.X+(r.Dx()-medir(cara, s))/2, baseCentrada(r, cara), s, cara, c)
}

func (l *lienzo) izquierda(r image.Rectangle, s string, cara font.Face, c color.Color) {
	l.texto(r.Min.X, baseCentrada(r, cara), recortar(cara, s, r.Dx()), cara, c)
}

func (l *lienzo) derecha(r image.Rectangle, s string, cara font.Face, c color.Color) {
	s = recortar(cara, s, r.Dx())
	l.texto(r.Max.X-medir(cara, s), baseCentrada(r, cara), s, cara, c)
}

func altoLinea(cara font.Face) int { return cara.Metrics().Height.Round() + 3 }

// ajustar parte el texto en líneas que caben en anchoMax (y corta las palabras demasiado largas).
func ajustar(cara font.Face, s string, anchoMax int) []string {
	var lineas []string
	actual := ""
	for _, p := range strings.Fields(s) {
		for medir(cara, p) > anchoMax {
			r := []rune(p)
			n := len(r)
			for n > 1 && medir(cara, string(r[:n])) > anchoMax {
				n--
			}
			if actual != "" {
				lineas = append(lineas, actual)
				actual = ""
			}
			lineas = append(lineas, string(r[:n]))
			p = string(r[n:])
		}
		prueba := p
		if actual != "" {
			prueba = actual + " " + p
		}
		if medir(cara, prueba) > anchoMax && actual != "" {
			lineas = append(lineas, actual)
			actual = p
		} else {
			actual = prueba
		}
	}
	if actual != "" || len(lineas) == 0 {
		lineas = append(lineas, actual)
	}
	return lineas
}

func recortar(cara font.Face, s string, anchoMax int) string {
	if medir(cara, s) <= anchoMax {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && medir(cara, string(r)+"…") > anchoMax {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// boton dibuja una pista de mando: un círculo con la letra y lo que hace. Devuelve dónde acaba.
func (l *lienzo) boton(x, cy int, letra, que string) int {
	ancho := max(22, medir(l.f.mini, letra)+12)
	l.redondeado(image.Rect(x, cy-11, x+ancho, cy+11), 11, colTexto)
	l.centrado(image.Rect(x, cy-11, x+ancho, cy+11), letra, l.f.mini, colBarra)
	return l.texto(x+ancho+6, baseCentrada(image.Rect(0, cy-11, 0, cy+11), l.f.pequena), que, l.f.pequena, colSuave) + 18
}

// pistas: la barra de abajo con los botones de cada pantalla.
func (l *lienzo) pistas(p ...[2]string) {
	r := image.Rect(0, alto-40, ancho, alto)
	l.rect(r, colBarra)
	x := 14
	for _, b := range p {
		x = l.boton(x, r.Min.Y+20, b[0], b[1])
	}
}

// marco pasa la imagen al formato del núcleo (XRGB8888 en little endian: B, G, R, X).
func (l *lienzo) marco() []byte {
	out := make([]byte, len(l.img.Pix))
	l.marcoEn(out)
	return out
}

// marcoEn hace lo mismo en una memoria que ya existe (la del servidor de la pantalla).
func (l *lienzo) marcoEn(out []byte) {
	px := l.img.Pix
	for i := 0; i+3 < len(px) && i+3 < len(out); i += 4 {
		out[i], out[i+1], out[i+2], out[i+3] = px[i+2], px[i+1], px[i], 0xff
	}
}
