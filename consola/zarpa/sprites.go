package main

import (
	"image"
	"image/color"
	"math"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Todo lo que se ve está dibujado aquí con trazos (golang.org/x/image/vector, con los bordes
// suavizados): Zarpa, los obstáculos, las estrellas, las palmeras y el cielo de Los Ángeles. Nada
// sale de imágenes de fuera.

// pincel: cada figura se apunta (con lo que ocupa) y al pintarla solo se rasteriza ese trozo. Antes
// cada trazo recorría la imagen entera, y una palmera (cientos de trazos) tardaba segundos.
type pincel struct {
	img            *image.RGBA
	r              *vector.Rasterizer
	ops            []opTrazo
	x0, y0, x1, y1 float32
}

type opTrazo struct {
	k byte // M, L, Q, C, Z
	p [6]float32
}

func nuevoPincel(w, h int) *pincel {
	return &pincel{img: image.NewRGBA(image.Rect(0, 0, w, h)), r: vector.NewRasterizer(1, 1), x0: 1e9, y0: 1e9, x1: -1e9, y1: -1e9}
}

func (p *pincel) ver(pts ...float32) {
	for i := 0; i+1 < len(pts); i += 2 {
		p.x0, p.x1 = min(p.x0, pts[i]), max(p.x1, pts[i])
		p.y0, p.y1 = min(p.y0, pts[i+1]), max(p.y1, pts[i+1])
	}
}

func (p *pincel) MoveTo(x, y float32) {
	p.ver(x, y)
	p.ops = append(p.ops, opTrazo{k: 'M', p: [6]float32{x, y}})
}

func (p *pincel) LineTo(x, y float32) {
	p.ver(x, y)
	p.ops = append(p.ops, opTrazo{k: 'L', p: [6]float32{x, y}})
}

func (p *pincel) QuadTo(x1, y1, x, y float32) {
	p.ver(x1, y1, x, y)
	p.ops = append(p.ops, opTrazo{k: 'Q', p: [6]float32{x1, y1, x, y}})
}

func (p *pincel) CubeTo(x1, y1, x2, y2, x, y float32) {
	p.ver(x1, y1, x2, y2, x, y)
	p.ops = append(p.ops, opTrazo{k: 'C', p: [6]float32{x1, y1, x2, y2, x, y}})
}

func (p *pincel) ClosePath() { p.ops = append(p.ops, opTrazo{k: 'Z'}) }

func (p *pincel) pintar(c color.Color) {
	b := p.img.Bounds()
	rx0, ry0 := max(int(math.Floor(float64(p.x0)))-1, 0), max(int(math.Floor(float64(p.y0)))-1, 0)
	rx1, ry1 := min(int(math.Ceil(float64(p.x1)))+1, b.Dx()), min(int(math.Ceil(float64(p.y1)))+1, b.Dy())
	if rx1 > rx0 && ry1 > ry0 {
		w, h := rx1-rx0, ry1-ry0
		p.r.Reset(w, h)
		ox, oy := float32(rx0), float32(ry0)
		for _, o := range p.ops {
			q := o.p
			switch o.k {
			case 'M':
				p.r.MoveTo(q[0]-ox, q[1]-oy)
			case 'L':
				p.r.LineTo(q[0]-ox, q[1]-oy)
			case 'Q':
				p.r.QuadTo(q[0]-ox, q[1]-oy, q[2]-ox, q[3]-oy)
			case 'C':
				p.r.CubeTo(q[0]-ox, q[1]-oy, q[2]-ox, q[3]-oy, q[4]-ox, q[5]-oy)
			case 'Z':
				p.r.ClosePath()
			}
		}
		p.r.Draw(p.img, image.Rect(rx0, ry0, rx1, ry1), image.NewUniform(c), image.Point{})
	}
	p.ops = p.ops[:0]
	p.x0, p.y0, p.x1, p.y1 = 1e9, 1e9, -1e9, -1e9
}

// poli: un polígono (x0, y0, x1, y1…).
func (p *pincel) poli(c color.Color, pts ...float32) {
	p.MoveTo(pts[0], pts[1])
	for i := 2; i+1 < len(pts); i += 2 {
		p.LineTo(pts[i], pts[i+1])
	}
	p.ClosePath()
	p.pintar(c)
}

func (p *pincel) elipse(c color.Color, cx, cy, rx, ry float32) {
	const k = 0.5523
	r := p
	r.MoveTo(cx+rx, cy)
	r.CubeTo(cx+rx, cy+ry*k, cx+rx*k, cy+ry, cx, cy+ry)
	r.CubeTo(cx-rx*k, cy+ry, cx-rx, cy+ry*k, cx-rx, cy)
	r.CubeTo(cx-rx, cy-ry*k, cx-rx*k, cy-ry, cx, cy-ry)
	r.CubeTo(cx+rx*k, cy-ry, cx+rx, cy-ry*k, cx+rx, cy)
	r.ClosePath()
	p.pintar(c)
}

func (p *pincel) rect(c color.Color, x0, y0, x1, y1 float32) {
	p.poli(c, x0, y0, x1, y0, x1, y1, x0, y1)
}

func (p *pincel) rectRedondo(c color.Color, x0, y0, x1, y1, rad float32) {
	r := p
	r.MoveTo(x0+rad, y0)
	r.LineTo(x1-rad, y0)
	r.QuadTo(x1, y0, x1, y0+rad)
	r.LineTo(x1, y1-rad)
	r.QuadTo(x1, y1, x1-rad, y1)
	r.LineTo(x0+rad, y1)
	r.QuadTo(x0, y1, x0, y1-rad)
	r.LineTo(x0, y0+rad)
	r.QuadTo(x0, y0, x0+rad, y0)
	r.ClosePath()
	p.pintar(c)
}

// trazo: una línea gruesa con las puntas redondas.
func (p *pincel) trazo(c color.Color, x0, y0, x1, y1, g float32) {
	dx, dy := x1-x0, y1-y0
	l := float32(math.Hypot(float64(dx), float64(dy)))
	if l > 0 {
		nx, ny := -dy/l*g/2, dx/l*g/2
		p.poli(c, x0+nx, y0+ny, x1+nx, y1+ny, x1-nx, y1-ny, x0-nx, y0-ny)
	}
	p.elipse(c, x0, y0, g/2, g/2)
	p.elipse(c, x1, y1, g/2, g/2)
}

// curva: una línea gruesa que sigue una curva (cuadrática), más fina hacia el final.
func (p *pincel) curva(c color.Color, x0, y0, cx, cy, x1, y1, g0, g1 float32) {
	const n = 14
	px, py := x0, y0
	for i := 1; i <= n; i++ {
		t := float32(i) / n
		u := 1 - t
		x := u*u*x0 + 2*u*t*cx + t*t*x1
		y := u*u*y0 + 2*u*t*cy + t*t*y1
		p.trazo(c, px, py, x, y, g0+(g1-g0)*t)
		px, py = x, y
	}
}

// estrella: de cinco puntas.
func (p *pincel) estrella(c color.Color, cx, cy, r float32) {
	var pts []float32
	for i := 0; i < 10; i++ {
		rr := r
		if i%2 == 1 {
			rr = r * 0.45
		}
		a := -math.Pi/2 + float64(i)*math.Pi/5
		pts = append(pts, cx+rr*float32(math.Cos(a)), cy+rr*float32(math.Sin(a)))
	}
	p.poli(c, pts...)
}

func (p *pincel) texto(cara font.Face, s string, cx, base int, c color.Color) {
	w := font.MeasureString(cara, s).Round()
	d := &font.Drawer{Dst: p.img, Src: image.NewUniform(c), Face: cara, Dot: fixed.P(cx-w/2, base)}
	d.DrawString(s)
}

func rgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{r, g, b, a} }

// ---- Zarpa -------------------------------------------------------------------------------------------

// traje: los colores de Zarpa (se cambian en la tienda).
type traje struct {
	ID, Nombre      string
	Precio          int
	Pelaje, Brillo  color.NRGBA // el pelo y su brillo (la luz del atardecer en el borde)
	Bufanda, BufOsc color.NRGBA
	Mochila         color.NRGBA
}

var trajes = []traje{
	{ID: "noche", Nombre: "Noche", Pelaje: rgba(0x1c, 0x1a, 0x26, 255), Brillo: rgba(0x6a, 0x5c, 0x9c, 255),
		Bufanda: rgba(0x22, 0xe0, 0xf0, 255), BufOsc: rgba(0x0e, 0x9a, 0xb0, 255), Mochila: rgba(0xff, 0x6b, 0x3d, 255)},
	{ID: "playa", Nombre: "Playa", Precio: 250, Pelaje: rgba(0xe0, 0xa8, 0x5a, 255), Brillo: rgba(0xff, 0xe0, 0xa0, 255),
		Bufanda: rgba(0x2a, 0x8c, 0xff, 255), BufOsc: rgba(0x14, 0x56, 0xb8, 255), Mochila: rgba(0xff, 0xd8, 0x3a, 255)},
	{ID: "nieve", Nombre: "Nieve", Precio: 500, Pelaje: rgba(0xee, 0xee, 0xf4, 255), Brillo: rgba(0xff, 0xff, 0xff, 255),
		Bufanda: rgba(0xff, 0x3a, 0x5c, 255), BufOsc: rgba(0xb0, 0x1c, 0x38, 255), Mochila: rgba(0x3a, 0x3a, 0x4a, 255)},
	{ID: "oro", Nombre: "Oro", Precio: 900, Pelaje: rgba(0xd8, 0xa0, 0x18, 255), Brillo: rgba(0xff, 0xec, 0x80, 255),
		Bufanda: rgba(0x18, 0x18, 0x20, 255), BufOsc: rgba(0x08, 0x08, 0x0c, 255), Mochila: rgba(0x8a, 0x2a, 0xe0, 255)},
	{ID: "neon", Nombre: "Neón", Precio: 1400, Pelaje: rgba(0x30, 0x10, 0x48, 255), Brillo: rgba(0xff, 0x4a, 0xd8, 255),
		Bufanda: rgba(0x6a, 0xff, 0x6a, 255), BufOsc: rgba(0x2a, 0xb8, 0x2a, 255), Mochila: rgba(0x22, 0xe0, 0xf0, 255)},
	{ID: "lava", Nombre: "Lava", Precio: 2000, Pelaje: rgba(0x3a, 0x30, 0x30, 255), Brillo: rgba(0xff, 0x8a, 0x2a, 255),
		Bufanda: rgba(0xff, 0x5a, 0x10, 255), BufOsc: rgba(0xb8, 0x30, 0x00, 255), Mochila: rgba(0xff, 0xd0, 0x30, 255)},
}

func trajeDe(id string) traje {
	for _, t := range trajes {
		if t.ID == id {
			return t
		}
	}
	return trajes[0]
}

// postura de Zarpa en un fotograma.
type postura struct {
	fase     float64 // la carrera (0..2π)
	salto    bool
	agachado bool
	choque   bool
}

const (
	anchoZarpa, altoZarpa = 124, 148
)

// dibujarZarpa: Zarpa de espaldas (como se ve corriendo), con su mochila y la bufanda al viento.
func dibujarZarpa(t traje, ps postura) *image.RGBA {
	p := nuevoPincel(anchoZarpa, altoZarpa)
	cx := float32(anchoZarpa / 2)
	suelo := float32(altoZarpa - 6)
	s := float32(math.Sin(ps.fase))
	bote := float32(math.Abs(math.Sin(ps.fase))) * 3
	if ps.salto || ps.choque {
		bote = 0
	}
	if ps.agachado {
		// agachada: el cuerpo bajo y estirado, las orejas hacia atrás
		y := suelo - 28
		p.curva(t.Pelaje, cx+20, y+6, cx+52, y-6, cx+58, y-22, 9, 5) // la cola
		p.elipse(t.Brillo, cx, y+2, 40, 22)
		p.elipse(t.Pelaje, cx, y+4, 38, 20)
		p.rectRedondo(t.Mochila, cx-17, y-14, cx+17, y+8, 6)
		p.elipse(t.Brillo, cx, y-18, 21, 17)
		p.elipse(t.Pelaje, cx, y-16, 20, 16)
		p.poli(t.Pelaje, cx-18, y-24, cx-30, y-30, cx-10, y-30)
		p.poli(t.Pelaje, cx+18, y-24, cx+30, y-30, cx+10, y-30)
		p.rectRedondo(t.Bufanda, cx-19, y-6, cx+19, y+2, 4)
		p.poli(t.BufOsc, cx+8, y-2, cx+40, y-10+s*4, cx+38, y-2+s*4, cx+10, y+2)
		return p.img
	}
	yb := suelo - 46 - bote // centro del cuerpo
	// la cola: sale de abajo y sube hacia un lado, con un poco de vaivén
	p.curva(t.Pelaje, cx+8, yb+18, cx+44, yb+8+s*6, cx+36+s*8, yb-34, 10, 5)
	p.curva(t.Brillo, cx+36+s*8, yb-34, cx+37+s*8, yb-37, cx+38+s*8, yb-40, 5, 3)
	// las piernas: una arriba y otra abajo (corriendo); en el salto, las dos dobladas
	lev := [2]float32{float32(math.Max(0, float64(s))) * 16, float32(math.Max(0, float64(-s))) * 16}
	if ps.salto || ps.choque {
		lev = [2]float32{14, 10}
	}
	for i, lado := range []float32{-1, 1} {
		xc := cx + lado*13
		p.trazo(t.Pelaje, xc, yb+14, xc+lado*2, suelo-10-lev[i], 15)
		p.elipse(t.Brillo, xc+lado*2, suelo-6-lev[i], 10, 6)
		p.elipse(t.Pelaje, xc+lado*2, suelo-7-lev[i], 9, 6)
	}
	// el cuerpo
	p.elipse(t.Brillo, cx, yb, 29, 33)
	p.elipse(t.Pelaje, cx, yb+1, 27, 31)
	// los brazos, que se balancean al correr (arriba en el salto)
	for i, lado := range []float32{-1, 1} {
		bal := s * 10 * lado
		ax, ay := cx+lado*30, yb+16+bal
		if ps.salto {
			ax, ay = cx+lado*36, yb-26
		}
		if ps.choque {
			ax, ay = cx+lado*40, yb-34+float32(i)*6
		}
		p.trazo(t.Pelaje, cx+lado*21, yb-14, ax, ay, 12)
		p.elipse(t.Brillo, ax, ay, 7, 7)
		p.elipse(t.Pelaje, ax, ay+1, 6, 6)
	}
	// la mochila, con sus tirantes
	p.rectRedondo(t.Mochila, cx-17, yb-18, cx+17, yb+14, 8)
	p.rectRedondo(rgba(0, 0, 0, 60), cx-13, yb-2, cx+13, yb+10, 5)
	p.trazo(rgba(0, 0, 0, 90), cx-15, yb-20, cx-21, yb-6, 4)
	p.trazo(rgba(0, 0, 0, 90), cx+15, yb-20, cx+21, yb-6, 4)
	// la cabeza y las orejas
	yc := yb - 46
	p.elipse(t.Brillo, cx, yc, 24, 22)
	p.elipse(t.Pelaje, cx, yc+1, 23, 21)
	for _, lado := range []float32{-1, 1} {
		p.poli(t.Brillo, cx+lado*10, yc-15, cx+lado*24, yc-36, cx+lado*25, yc-10)
		p.poli(t.Pelaje, cx+lado*11, yc-13, cx+lado*23, yc-32, cx+lado*23, yc-10)
		p.poli(rgba(0xff, 0x8a, 0xa8, 140), cx+lado*15, yc-15, cx+lado*21, yc-27, cx+lado*21, yc-13)
	}
	// un poco de brillo arriba (la luz del atardecer)
	p.elipse(rgba(0xff, 0xff, 0xff, 30), cx-8, yc-10, 9, 5)
	// la bufanda y sus puntas al viento
	p.rectRedondo(t.Bufanda, cx-21, yb-32, cx+21, yb-22, 5)
	p.poli(t.Bufanda, cx+6, yb-27, cx+42, yb-38+s*6, cx+44, yb-30+s*6, cx+10, yb-21)
	p.poli(t.BufOsc, cx+8, yb-23, cx+36, yb-24+s*5, cx+38, yb-18+s*5, cx+10, yb-19)
	return p.img
}

// sombraZarpa: la sombra en el suelo (aparte: no sube con el salto).
func sombraZarpa() *image.RGBA {
	p := nuevoPincel(100, 26)
	p.elipse(rgba(0, 0, 0, 110), 50, 13, 46, 11)
	return p.img
}

// ---- Obstáculos y cosas ------------------------------------------------------------------------------

// Medidas en unidades del mundo (un carril = 1). Con escala0 px por unidad, a la altura de Zarpa.
const (
	altoValla    = 0.55
	bajoCartel   = 0.5 // la parte de abajo del cartel: hay que agacharse
	altoCartel   = 1.0
	altoCamion   = 1.6
	largoCamion  = 2.6
	altoEstrella = 0.45 // a qué altura flotan (las del salto, más arriba)
)

func valla(e float32) *image.RGBA {
	w, h := int(0.95*e), int(altoValla*e)
	p := nuevoPincel(w+4, h+4)
	fw, fh := float32(w), float32(h)
	naranja, blanco := rgba(0xff, 0x6a, 0x1a, 255), rgba(0xf4, 0xf0, 0xe8, 255)
	// las patas
	for _, x := range []float32{fw * 0.14, fw * 0.86} {
		p.trazo(rgba(0x3a, 0x38, 0x44, 255), x, fh*0.25, x-fw*0.06, fh, fw*0.04)
		p.trazo(rgba(0x3a, 0x38, 0x44, 255), x, fh*0.25, x+fw*0.06, fh, fw*0.04)
	}
	// dos tablas con rayas
	for _, y := range []float32{fh * 0.12, fh * 0.5} {
		p.rect(blanco, 2, y, fw+2, y+fh*0.24)
		for x := float32(-fw * 0.2); x < fw; x += fw * 0.2 {
			p.poli(naranja, x, y+fh*0.24, x+fw*0.1, y, x+fw*0.2, y, x+fw*0.1, y+fh*0.24)
		}
	}
	// las luces de arriba
	for _, x := range []float32{fw * 0.18, fw * 0.82} {
		p.elipse(rgba(0xff, 0xe0, 0x40, 90), x, fh*0.06, fw*0.06, fw*0.06)
		p.elipse(rgba(0xff, 0xe8, 0x60, 255), x, fh*0.06, fw*0.03, fw*0.03)
	}
	return p.img
}

func cartel(e float32, f *fuentes) *image.RGBA {
	w, h := int(1.0*e), int(altoCartel*e)
	p := nuevoPincel(w+4, h+4)
	fw, fh := float32(w), float32(h)
	gris := rgba(0x8a, 0x8e, 0x98, 255)
	// los postes
	p.rect(gris, fw*0.04, 0, fw*0.1, fh)
	p.rect(gris, fw*0.9, 0, fw*0.96, fh)
	// el cartel verde de autopista (arriba de bajoCartel)
	y0, y1 := float32(0), fh*(1-bajoCartel/altoCartel)
	p.rectRedondo(rgba(0x0c, 0x6a, 0x3a, 255), 0, y0+2, fw+2, y1, fw*0.04)
	p.rectRedondo(rgba(0xf4, 0xf4, 0xf4, 255), fw*0.03, y0+fh*0.04, fw*0.99, y1-fh*0.04, fw*0.03)
	p.rectRedondo(rgba(0x0c, 0x6a, 0x3a, 255), fw*0.05, y0+fh*0.06, fw*0.97, y1-fh*0.06, fw*0.025)
	p.texto(f.grandeNegrita, "DOWNTOWN", w/2, int(y0+fh*0.27), rgba(255, 255, 255, 255))
	p.texto(f.medianaNegrita, "LOS ANGELES", w/2, int(y0+fh*0.41), rgba(255, 255, 255, 255))
	// la flecha hacia abajo (agáchate)
	ax := fw * 0.5
	p.poli(rgba(0xff, 0xd0, 0x30, 255), ax-fw*0.07, y1+fh*0.04, ax+fw*0.07, y1+fh*0.04, ax, y1+fh*0.13)
	return p.img
}

func camion(e float32, f *fuentes) *image.RGBA {
	w, h := int(0.96*e), int(altoCamion*e)
	p := nuevoPincel(w+4, h+4)
	fw, fh := float32(w), float32(h)
	// las ruedas
	p.rectRedondo(rgba(0x18, 0x18, 0x1c, 255), fw*0.06, fh*0.86, fw*0.3, fh, fw*0.05)
	p.rectRedondo(rgba(0x18, 0x18, 0x1c, 255), fw*0.7, fh*0.86, fw*0.94, fh, fw*0.05)
	// la caja (de espaldas): blanca con el toldo de rayas y el letrero
	p.rectRedondo(rgba(0xf2, 0xee, 0xe4, 255), 0, fh*0.16, fw+2, fh*0.9, fw*0.06)
	p.rectRedondo(rgba(0xe0, 0x3a, 0x4a, 255), 0, fh*0.06, fw+2, fh*0.2, fw*0.05)
	p.texto(f.grandeNegrita, "TACOS", w/2, int(fh*0.17), rgba(0xff, 0xe8, 0x50, 255))
	for i := 0; i < 6; i++ {
		x := fw * float32(i) / 6
		col := rgba(0x2a, 0xa8, 0x5a, 255)
		if i%2 == 1 {
			col = rgba(0xff, 0xff, 0xff, 255)
		}
		p.poli(col, x, fh*0.2, x+fw/6, fh*0.2, x+fw/6, fh*0.27, x, fh*0.27)
	}
	// las puertas de atrás y sus asas
	p.rect(rgba(0xd6, 0xd0, 0xc4, 255), fw*0.08, fh*0.32, fw*0.49, fh*0.82)
	p.rect(rgba(0xd6, 0xd0, 0xc4, 255), fw*0.51, fh*0.32, fw*0.92, fh*0.82)
	p.rect(rgba(0x9a, 0x94, 0x8a, 255), fw*0.42, fh*0.52, fw*0.46, fh*0.6)
	p.rect(rgba(0x9a, 0x94, 0x8a, 255), fw*0.54, fh*0.52, fw*0.58, fh*0.6)
	// un taco dibujado en la puerta
	p.elipse(rgba(0xf0, 0xb8, 0x3a, 255), fw*0.28, fh*0.47, fw*0.12, fh*0.06)
	p.elipse(rgba(0x4a, 0xb0, 0x3a, 255), fw*0.28, fh*0.44, fw*0.09, fh*0.025)
	// las luces de atrás y la matrícula
	p.rectRedondo(rgba(0xff, 0x2a, 0x2a, 255), fw*0.02, fh*0.7, fw*0.08, fh*0.82, fw*0.02)
	p.rectRedondo(rgba(0xff, 0x2a, 0x2a, 255), fw*0.92, fh*0.7, fw*0.98, fh*0.82, fw*0.02)
	p.rectRedondo(rgba(0xf8, 0xf8, 0xf0, 255), fw*0.36, fh*0.84, fw*0.64, fh*0.9, fw*0.01)
	p.texto(f.mini, "LA 2026", w/2, int(fh*0.895), rgba(0x20, 0x3a, 0x8a, 255))
	return p.img
}

func dibujarEstrella(e float32) *image.RGBA {
	t := int(0.34 * e)
	p := nuevoPincel(t, t)
	c := float32(t) / 2
	p.elipse(rgba(0xff, 0xd8, 0x40, 60), c, c, c, c)
	p.estrella(rgba(0xc8, 0x8a, 0x10, 255), c, c+1, c*0.86)
	p.estrella(rgba(0xff, 0xd4, 0x2a, 255), c, c, c*0.82)
	p.estrella(rgba(0xff, 0xf2, 0x9a, 255), c-c*0.1, c-c*0.1, c*0.36)
	return p.img
}

func iman(e float32) *image.RGBA {
	t := int(0.42 * e)
	p := nuevoPincel(t, t)
	ft := float32(t)
	p.elipse(rgba(0xff, 0x40, 0x60, 50), ft/2, ft/2, ft/2, ft/2)
	rojo := rgba(0xe8, 0x24, 0x3a, 255)
	p.curva(rojo, ft*0.25, ft*0.3, ft*0.25, ft*0.85, ft*0.5, ft*0.82, ft*0.16, ft*0.16)
	p.curva(rojo, ft*0.75, ft*0.3, ft*0.75, ft*0.85, ft*0.5, ft*0.82, ft*0.16, ft*0.16)
	p.rect(rgba(0xd8, 0xdc, 0xe4, 255), ft*0.17, ft*0.14, ft*0.33, ft*0.3)
	p.rect(rgba(0xd8, 0xdc, 0xe4, 255), ft*0.67, ft*0.14, ft*0.83, ft*0.3)
	return p.img
}

func escudo(e float32) *image.RGBA {
	t := int(0.46 * e)
	p := nuevoPincel(t, t)
	c := float32(t) / 2
	p.elipse(rgba(0x40, 0xd8, 0xff, 70), c, c, c, c)
	p.elipse(rgba(0x40, 0xd8, 0xff, 110), c, c, c*0.82, c*0.82)
	p.poli(rgba(0xf0, 0xfa, 0xff, 255), c, c*0.45, c*1.45, c*0.62, c*1.35, c*1.15, c, c*1.55, c*0.65, c*1.15, c*0.55, c*0.62)
	p.elipse(rgba(255, 255, 255, 120), c*0.7, c*0.6, c*0.18, c*0.12)
	return p.img
}

// burbuja: el escudo puesto, alrededor de Zarpa.
func burbuja() *image.RGBA {
	p := nuevoPincel(anchoZarpa+30, altoZarpa+20)
	cx, cy := float32(anchoZarpa+30)/2, float32(altoZarpa+20)/2
	p.elipse(rgba(0x40, 0xd8, 0xff, 50), cx, cy, cx-2, cy-2)
	p.elipse(rgba(0x40, 0xd8, 0xff, 20), cx, cy, cx-10, cy-10)
	p.elipse(rgba(255, 255, 255, 70), cx-cx*0.4, cy-cy*0.5, cx*0.18, cy*0.08)
	return p.img
}

func palmera(e float32, espejo bool) *image.RGBA {
	w, h := int(1.6*e), int(3.2*e)
	p := nuevoPincel(w, h)
	fw, fh := float32(w), float32(h)
	lado := float32(1)
	if espejo {
		lado = -1
	}
	bx := fw/2 - lado*fw*0.05
	tx, ty := fw/2+lado*fw*0.14, fh*0.16
	tronco := rgba(0x4a, 0x2e, 0x3a, 255)
	p.curva(tronco, bx, fh, bx+lado*fw*0.02, fh*0.5, tx, ty, fw*0.07, fw*0.045)
	// las hojas: arcos que caen desde arriba
	hoja := rgba(0x1e, 0x4a, 0x3a, 255)
	for i := 0; i < 8; i++ {
		a := float64(i)/8*2*math.Pi + 0.2
		dx, dy := float32(math.Cos(a)), float32(math.Sin(a))*0.4-0.2
		ex, ey := tx+dx*fw*0.46, ty+dy*fh*0.12+fh*0.08
		p.curva(hoja, tx, ty, tx+dx*fw*0.3, ty+dy*fh*0.1-fh*0.05, ex, ey, fw*0.06, fw*0.012)
	}
	p.elipse(rgba(0x3a, 0x22, 0x2a, 255), tx, ty, fw*0.04, fw*0.035)
	return p.img
}

// ---- Escalas -----------------------------------------------------------------------------------------

// Cada dibujo se guarda ya escalado a varios tamaños (como se ve de lejos a de cerca), y de lejos un
// poco transparente (la neblina del atardecer).
const nEscalas = 44

func factorEscala(i int) float64 { return 0.055 * math.Pow(1.2/0.055, float64(i)/float64(nEscalas-1)) }

type escalado struct {
	imgs [nEscalas]*image.RGBA
}

func escalas(base *image.RGBA) *escalado {
	e := &escalado{}
	b := base.Bounds()
	for i := 0; i < nEscalas; i++ {
		f := factorEscala(i)
		w, h := max(1, int(float64(b.Dx())*f+0.5)), max(1, int(float64(b.Dy())*f+0.5))
		d := image.NewRGBA(image.Rect(0, 0, w, h))
		xdraw.ApproxBiLinear.Scale(d, d.Bounds(), base, b, xdraw.Src, nil)
		if niebla := math.Min(1, 0.35+f*2.2); niebla < 1 {
			for k := range d.Pix {
				d.Pix[k] = uint8(float64(d.Pix[k]) * niebla)
			}
		}
		e.imgs[i] = d
	}
	return e
}

// de: la imagen para ese factor de tamaño (1 = a la altura de Zarpa).
func (e *escalado) de(f float64) *image.RGBA {
	i := int(math.Round(math.Log(f/0.055) / math.Log(1.2/0.055) * float64(nEscalas-1)))
	return e.imgs[min(max(i, 0), nEscalas-1)]
}
