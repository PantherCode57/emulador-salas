package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Tamaño de la pantalla de salas (RetroArch la escala a la pantalla real). Por defecto 960×540
// (16:9); en consolas con pantalla 4:3 se pone POKEMULADOR_ANCHO y POKEMULADOR_ALTO (640×480).
var ancho, alto = tamañoPantalla()

// compacto: pantallas pequeñas (menos de 800 de ancho): letras algo menores y una sola columna.
var compacto = ancho < 800

func tamañoPantalla() (int, int) {
	w, h := 960, 540
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 320 && v <= 1920 {
		w = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 240 && v <= 1080 {
		h = v
	}
	return w, h
}

// elegir: un valor para pantallas pequeñas y otro para las normales.
func elegir(pequena, normal int) int {
	if compacto {
		return pequena
	}
	return normal
}

// Colores de Pokemulador (estilo Pokédex, como en el móvil).
var (
	colFondo   = color.RGBA{0x9a, 0x33, 0x30, 0xff}
	colPanel   = color.RGBA{0xfb, 0xed, 0xec, 0xff}
	colTarjeta = color.RGBA{0xf3, 0xcf, 0xcd, 0xff}
	colBoton   = color.RGBA{0xd2, 0x50, 0x4a, 0xff}
	colTexto   = color.RGBA{0x4a, 0x16, 0x16, 0xff}
	colSuave   = color.RGBA{0x84, 0x4a, 0x47, 0xff}
	colBlanco  = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colVerde   = color.RGBA{0x2e, 0x7d, 0x32, 0xff}
	colAzul    = color.RGBA{0x3b, 0x82, 0xe0, 0xff}
	colAmarillo = color.RGBA{0xf4, 0xc4, 0x30, 0xff}
)

type fuentes struct {
	grande, titulo, normal, negrita, pequena font.Face
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
		grande:  cara(neg, float64(elegir(32, 44))),
		titulo:  cara(neg, float64(elegir(24, 30))),
		normal:  cara(reg, float64(elegir(20, 22))),
		negrita: cara(neg, float64(elegir(20, 22))),
		pequena: cara(reg, float64(elegir(16, 18))),
	}
}

type lienzo struct {
	img *image.RGBA
	f   *fuentes
}

func nuevoLienzo(f *fuentes) *lienzo {
	return &lienzo{img: image.NewRGBA(image.Rect(0, 0, ancho, alto)), f: f}
}

func (l *lienzo) rect(r image.Rectangle, c color.Color) {
	draw.Draw(l.img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

// redondeado: rectángulo con esquinas redondas (borde > 0 deja solo el contorno de ese grosor).
func (l *lienzo) redondeado(r image.Rectangle, radio int, c color.Color, borde int, dentro color.Color) {
	l.relleno(r, radio, c)
	if borde > 0 {
		l.relleno(r.Inset(borde), max(radio-borde, 0), dentro)
	}
}

func (l *lienzo) relleno(r image.Rectangle, radio int, c color.Color) {
	if radio*2 > r.Dy() {
		radio = r.Dy() / 2
	}
	u := &image.Uniform{c}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		dentro := 0
		t := min(y-r.Min.Y, r.Max.Y-1-y)
		if t < radio {
			v := float64(radio) - float64(t) - 0.5
			dentro = int(math.Round(float64(radio) - math.Sqrt(math.Max(0, float64(radio*radio)-v*v))))
		}
		draw.Draw(l.img, image.Rect(r.Min.X+dentro, y, r.Max.X-dentro, y+1), u, image.Point{}, draw.Src)
	}
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
	s = recortar(cara, s, r.Dx()-8)
	l.texto(r.Min.X+(r.Dx()-medir(cara, s))/2, baseCentrada(r, cara), s, cara, c)
}

func (l *lienzo) izquierda(r image.Rectangle, s string, cara font.Face, c color.Color) {
	l.texto(r.Min.X, baseCentrada(r, cara), recortar(cara, s, r.Dx()), cara, c)
}

// parrafo escribe con saltos de línea automáticos; devuelve la y de debajo.
func (l *lienzo) parrafo(x, y, anchoMax int, s string, cara font.Face, c color.Color) int {
	alturaLinea := cara.Metrics().Height.Round() + 4
	for _, linea := range ajustar(cara, s, anchoMax) {
		y += alturaLinea
		l.texto(x, y, linea, cara, c)
	}
	return y
}

func ajustar(cara font.Face, s string, anchoMax int) []string {
	var lineas []string
	for _, parrafo := range strings.Split(s, "\n") {
		actual := ""
		for _, p := range strings.Fields(parrafo) {
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

// imagen dibuja una imagen ajustada dentro de r, sin deformarla.
func (l *lienzo) imagen(r image.Rectangle, img image.Image) {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return
	}
	w, h := r.Dx(), r.Dx()*b.Dy()/b.Dx()
	if h > r.Dy() {
		w, h = r.Dy()*b.Dx()/b.Dy(), r.Dy()
	}
	x, y := r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-h)/2
	xdraw.ApproxBiLinear.Scale(l.img, image.Rect(x, y, x+w, y+h), img, b, xdraw.Src, nil)
}

// boton: píldora rellena si está elegida; si no, solo el contorno.
func (l *lienzo) boton(r image.Rectangle, s string, elegido bool) {
	if elegido {
		l.redondeado(r, r.Dy()/2, colBoton, 0, nil)
		l.centrado(r, s, l.f.negrita, colBlanco)
	} else {
		l.redondeado(r, r.Dy()/2, colBoton, 2, colPanel)
		l.centrado(r, s, l.f.negrita, colBoton)
	}
}

// fila de lista: resaltada si está elegida.
func (l *lienzo) fila(r image.Rectangle, s, etiqueta string, elegido bool) {
	if elegido {
		l.redondeado(r, 14, colBoton, 0, nil)
	}
	c := colTexto
	if elegido {
		c = colBlanco
	}
	x := r.Min.X + 16
	if etiqueta != "" {
		e := image.Rect(x, r.Min.Y+8, x+medir(l.f.pequena, etiqueta)+18, r.Max.Y-8)
		fondoEtiqueta := colTarjeta
		if elegido {
			fondoEtiqueta = colBlanco
		}
		l.redondeado(e, e.Dy()/2, fondoEtiqueta, 0, nil)
		l.centrado(e, etiqueta, l.f.pequena, colBoton)
		x = e.Max.X + 12
	}
	l.izquierda(image.Rect(x, r.Min.Y, r.Max.X-12, r.Max.Y), s, l.f.normal, c)
}

// marco pasa la imagen al formato que espera el núcleo (XRGB8888 en little endian: B, G, R, X).
func (l *lienzo) marco() []byte {
	px := l.img.Pix
	out := make([]byte, len(px))
	for i := 0; i < len(px); i += 4 {
		out[i], out[i+1], out[i+2], out[i+3] = px[i+2], px[i+1], px[i], 0xff
	}
	return out
}
