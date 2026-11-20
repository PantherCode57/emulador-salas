package main

import (
	"image"
	"image/color"
	"image/draw"
	"unsafe"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Lo que se hace en cada imagen tiene que ir rápido (30 por segundo en un procesador pequeño).

// marcoRapido: de RGBA a lo que espera el núcleo (B, G, R, X), de cuatro en cuatro bytes a la vez.
func marcoRapido(px, out []byte) {
	n := min(len(px), len(out)) / 4
	if n == 0 {
		return
	}
	src := unsafe.Slice((*uint32)(unsafe.Pointer(&px[0])), n)
	dst := unsafe.Slice((*uint32)(unsafe.Pointer(&out[0])), n)
	for i, v := range src { // v = R | G<<8 | B<<16 | A<<24
		dst[i] = v&0x0000ff00 | (v&0xff)<<16 | (v>>16)&0xff | 0xff000000
	}
}

// letras: cada letra, ya pintada con su color, para no rasterizarla en cada imagen (los números del
// marcador cambian a cada momento).
type claveLetra struct {
	cara font.Face
	col  color.RGBA
	r    rune
}

type letraHecha struct {
	img *image.RGBA
	dr  image.Rectangle // respecto al punto de la línea base
	adv int
}

var letras = map[claveLetra]*letraHecha{}

func letraDe(cara font.Face, c color.RGBA, r rune) *letraHecha {
	k := claveLetra{cara, c, r}
	if l, ok := letras[k]; ok {
		return l
	}
	dr, mask, mp, adv, ok := cara.Glyph(fixed.P(0, 0), r)
	l := &letraHecha{dr: dr, adv: adv.Round()}
	if ok && !dr.Empty() {
		l.img = image.NewRGBA(image.Rect(0, 0, dr.Dx(), dr.Dy()))
		draw.DrawMask(l.img, l.img.Bounds(), image.NewUniform(c), image.Point{}, mask, mp, draw.Src)
	}
	letras[k] = l
	return l
}

// textoRapido: como lienzo.texto (con la línea base en y), con las letras guardadas. Devuelve dónde acaba.
func (l *lienzo) textoRapido(x, y int, s string, cara font.Face, c color.RGBA) int {
	for _, r := range s {
		g := letraDe(cara, c, r)
		if g.img != nil {
			draw.Draw(l.img, g.dr.Add(image.Pt(x, y)), g.img, image.Point{}, draw.Over)
		}
		x += g.adv
	}
	return x
}

func anchoRapido(s string, cara font.Face) int {
	w := 0
	for _, r := range s {
		w += letraDe(cara, color.RGBA{255, 255, 255, 255}, r).adv
	}
	return w
}

// izquierdaRapido y derechaRapido: centrado en alto dentro de r, a la izquierda o a la derecha.
func (l *lienzo) izquierdaRapido(r image.Rectangle, s string, cara font.Face, c color.RGBA) {
	l.textoRapido(r.Min.X, baseCentrada(r, cara), s, cara, c)
}

func (l *lienzo) derechaRapido(r image.Rectangle, s string, cara font.Face, c color.RGBA) {
	l.textoRapido(r.Max.X-anchoRapido(s, cara), baseCentrada(r, cara), s, cara, c)
}

func (l *lienzo) centradoRapido(r image.Rectangle, s string, cara font.Face, c color.RGBA) {
	l.textoRapido(r.Min.X+(r.Dx()-anchoRapido(s, cara))/2, baseCentrada(r, cara), s, cara, c)
}

// ---- Pegar dibujos ---------------------------------------------------------------------------------

// tramos: en cada fila de un dibujo, de qué columna a qué columna hay algo (lo de los lados es
// transparente y no se mira). Se calcula la primera vez que se pega cada dibujo.
var tramos = map[*image.RGBA][][2]int32{}

func tramosDe(img *image.RGBA) [][2]int32 {
	if t, ok := tramos[img]; ok {
		return t
	}
	b := img.Bounds()
	t := make([][2]int32, b.Dy())
	for y := 0; y < b.Dy(); y++ {
		fila := img.Pix[y*img.Stride : y*img.Stride+b.Dx()*4]
		a0, a1 := -1, -1
		for x := 0; x < b.Dx(); x++ {
			if fila[x*4+3] != 0 {
				if a0 < 0 {
					a0 = x
				}
				a1 = x + 1
			}
		}
		if a0 >= 0 {
			t[y] = [2]int32{int32(a0), int32(a1)}
		}
	}
	tramos[img] = t
	return t
}

// ponerSobre: pega el dibujo con su punto de abajo en el centro en (x, y). Lo opaco se copia, lo
// transparente se salta y solo lo de medio se mezcla (más rápido que image/draw para esto).
func ponerSobre(dst *image.RGBA, img *image.RGBA, x, y int) {
	b := img.Bounds()
	x0, y0 := x-b.Dx()/2, y-b.Dy()
	db := dst.Bounds()
	t := tramosDe(img)
	for fy := 0; fy < b.Dy(); fy++ {
		dy := y0 + fy
		if dy < db.Min.Y || dy >= db.Max.Y {
			continue
		}
		a0, a1 := int(t[fy][0]), int(t[fy][1])
		if x0+a0 < db.Min.X {
			a0 = db.Min.X - x0
		}
		if x0+a1 > db.Max.X {
			a1 = db.Max.X - x0
		}
		if a0 >= a1 {
			continue
		}
		so := fy*img.Stride + a0*4
		do := (dy-db.Min.Y)*dst.Stride + (x0+a0-db.Min.X)*4
		mezclarFila(dst.Pix[do:do+(a1-a0)*4], img.Pix[so:so+(a1-a0)*4])
	}
}

// mezclarFila: d = s + d·(1 − alfa de s), con los colores ya multiplicados por su alfa (image.RGBA).
func mezclarFila(d, s []byte) {
	n := len(s) / 4
	if n == 0 {
		return
	}
	su := unsafe.Slice((*uint32)(unsafe.Pointer(&s[0])), n)
	du := unsafe.Slice((*uint32)(unsafe.Pointer(&d[0])), n)
	for i, v := range su {
		a := v >> 24
		if a == 0 {
			continue
		}
		if a == 255 {
			du[i] = v
			continue
		}
		inv := 255 - a
		dv := du[i]
		var out uint32
		for k := uint32(0); k < 32; k += 8 {
			x := (dv>>k&0xff)*inv + 128
			c := v>>k&0xff + (x+(x>>8))>>8
			if c > 255 {
				c = 255
			}
			out |= c << k
		}
		du[i] = out
	}
}
