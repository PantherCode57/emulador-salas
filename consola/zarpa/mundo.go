package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"math/rand"
	"sort"
)

// El mundo en pantalla: una autopista de Los Ángeles al atardecer, vista desde detrás de Zarpa.
//
// Proyección: un punto a z unidades por delante de Zarpa, a la altura y, se ve con escala
// e(z) = focal / (z + distCamara) píxeles por unidad; su suelo, en horizonte + altCamara·e(z), y de lado
// en 320 + x·e(z). A la altura de Zarpa (z = 0), escala0 ≈ 187 px por unidad (un carril).
const (
	horizonte  = 150
	focal      = 1120.0
	distCamara = 6.0
	altCamara  = 1.5
	escala0    = focal / distCamara
	zLejos     = 82.0
	largoBanda = 1.6 // las rayas del suelo (dan la sensación de velocidad)
)

func escalaEn(z float64) float64 { return focal / (z + distCamara) }

func sueloEn(z float64) float64 { return horizonte + altCamara*escalaEn(z) }

type mundo struct {
	cielo   *image.RGBA    // lo de arriba del horizonte (fijo)
	filas   [480][2][]byte // cada fila del suelo, con y sin rayas (se elige según la distancia)
	zFila   [480]float64   // a qué distancia está cada fila
	spr     map[string]*escalado
	sombras [8]*image.RGBA // la sombra de Zarpa, de grande (en el suelo) a pequeña (en lo alto del salto)
	burb    *image.RGBA
}

func nuevoMundo(f *fuentes) *mundo {
	m := &mundo{cielo: dibujarCielo(), spr: map[string]*escalado{}}
	m.prepararSuelo()
	e := float32(escala0)
	m.spr["valla"] = escalas(valla(e))
	m.spr["cartel"] = escalas(cartel(e, f))
	m.spr["camion"] = escalas(camion(e, f))
	m.spr["estrella"] = escalas(dibujarEstrella(e))
	m.spr["iman"] = escalas(iman(e))
	m.spr["escudo"] = escalas(escudo(e))
	m.spr["palmeraI"] = escalas(palmera(e, false))
	m.spr["palmeraD"] = escalas(palmera(e, true))
	s := sombraZarpa()
	for i := range m.sombras {
		k := 1 - float64(i)/14
		w, h := int(float64(s.Bounds().Dx())*k), s.Bounds().Dy()
		m.sombras[i] = image.NewRGBA(image.Rect(0, 0, w, h))
		escalarSobre(m.sombras[i], m.sombras[i].Bounds(), s)
	}
	m.burb = burbuja()
	return m
}

func mezcla(a, b color.NRGBA, t float64) color.NRGBA {
	t = math.Min(1, math.Max(0, t))
	return color.NRGBA{uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t), uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t), 255}
}

// dibujarCielo: el atardecer (de azul noche a naranja), el sol con rayas, las colinas y los
// rascacielos del centro con alguna ventana encendida, y palmeras en el horizonte.
func dibujarCielo() *image.RGBA {
	p := nuevoPincel(640, horizonte+1)
	img := p.img
	paradas := []struct {
		y float64
		c color.NRGBA
	}{{0, rgba(0x14, 0x0e, 0x3a, 255)}, {0.38, rgba(0x4a, 0x22, 0x7a, 255)}, {0.68, rgba(0xd8, 0x44, 0x7e, 255)},
		{0.88, rgba(0xff, 0x8a, 0x3c, 255)}, {1, rgba(0xff, 0xc8, 0x5c, 255)}}
	for y := 0; y <= horizonte; y++ {
		t := float64(y) / horizonte
		var c color.NRGBA
		for i := 1; i < len(paradas); i++ {
			if t <= paradas[i].y {
				c = mezcla(paradas[i-1].c, paradas[i].c, (t-paradas[i-1].y)/(paradas[i].y-paradas[i-1].y))
				break
			}
		}
		for x := 0; x < 640; x++ {
			img.Set(x, y, c)
		}
	}
	// unas estrellas arriba
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 40; i++ {
		x, y := r.Intn(640), r.Intn(horizonte/3)
		p.elipse(rgba(255, 255, 255, uint8(80+r.Intn(120))), float32(x), float32(y), 0.9, 0.9)
	}
	// el sol, bajando, con rayas que se van abriendo
	sx, sy, sr := float32(400), float32(horizonte-6), float32(66)
	p.elipse(rgba(0xff, 0xb8, 0x5a, 70), sx, sy, sr+14, sr+14)
	p.elipse(rgba(0xff, 0xd8, 0x6a, 255), sx, sy, sr, sr)
	p.elipse(rgba(0xff, 0x9a, 0x6a, 120), sx, sy+sr*0.4, sr, sr*0.7)
	for i := 0; i < 6; i++ {
		y := sy - sr*0.2 + float32(i)*sr*0.16
		p.rect(rgba(0xff, 0x8a, 0x3c, 255), sx-sr-2, y, sx+sr+2, y+float32(2+i))
	}
	// las colinas detrás
	var col []float32
	for x := 0; x <= 640; x += 16 {
		y := float32(horizonte) - 26 - 14*float32(math.Sin(float64(x)/90)) - 8*float32(math.Sin(float64(x)/37+1))
		col = append(col, float32(x), y)
	}
	p.poli(rgba(0x3a, 0x1a, 0x52, 255), append(col, 640, horizonte+1, 0, horizonte+1)...)
	// los rascacielos del centro, a la izquierda
	edif := []struct{ x, w, h int }{{40, 26, 52}, {68, 18, 70}, {88, 30, 96}, {120, 22, 78}, {144, 34, 112}, {180, 20, 64},
		{202, 28, 86}, {232, 16, 58}, {250, 24, 72}}
	for _, b := range edif {
		x0, x1, y0 := float32(b.x), float32(b.x+b.w), float32(horizonte-b.h)
		p.rect(rgba(0x22, 0x14, 0x3c, 255), x0, y0, x1, horizonte+1)
		if b.h > 90 { // la antena de los más altos
			p.rect(rgba(0x22, 0x14, 0x3c, 255), x0+float32(b.w)/2-1, y0-12, x0+float32(b.w)/2+1, y0)
			p.elipse(rgba(0xff, 0x40, 0x40, 255), x0+float32(b.w)/2, y0-12, 1.6, 1.6)
		}
		for wy := int(y0) + 5; wy < horizonte-4; wy += 6 {
			for wx := b.x + 3; wx < b.x+b.w-3; wx += 5 {
				if r.Intn(5) == 0 {
					p.rect(rgba(0xff, 0xd8, 0x7a, 200), float32(wx), float32(wy), float32(wx+2), float32(wy+2))
				}
			}
		}
	}
	// palmeras en el horizonte (siluetas)
	for _, x := range []float32{300, 318, 520, 548, 600} {
		p.curva(rgba(0x1e, 0x0e, 0x2c, 255), x, horizonte, x+3, horizonte-20, x+6, horizonte-40, 2.6, 1.6)
		for i := 0; i < 6; i++ {
			a := float64(i)/6*2*math.Pi + 0.3
			p.curva(rgba(0x1e, 0x0e, 0x2c, 255), x+6, horizonte-40, x+6+float32(math.Cos(a))*7, horizonte-44+float32(math.Sin(a))*3,
				x+6+float32(math.Cos(a))*14, horizonte-36+float32(math.Sin(a))*4, 2, 0.6)
		}
	}
	return img
}

// prepararSuelo: cada fila del suelo, ya pintada (con y sin raya): la autopista con sus líneas y,
// a los lados, la arena de la playa al anochecer.
func (m *mundo) prepararSuelo() {
	asfalto := [2]color.NRGBA{rgba(0x3a, 0x34, 0x4e, 255), rgba(0x34, 0x2e, 0x48, 255)}
	arcen := rgba(0x6a, 0x5e, 0x72, 255)
	arena := [2]color.NRGBA{rgba(0x8a, 0x5a, 0x6a, 255), rgba(0x80, 0x52, 0x62, 255)}
	amarillo, blanco := rgba(0xff, 0xc8, 0x3a, 255), rgba(0xf0, 0xec, 0xf4, 255)
	niebla := rgba(0xe0, 0x7a, 0x7a, 255)
	for y := horizonte + 1; y < 480; y++ {
		e := (float64(y) - horizonte) / altCamara
		z := focal/e - distCamara
		m.zFila[y] = z
		lejos := math.Max(0, math.Min(1, (z-8)/70)) * 0.75 // de lejos, hacia el color del horizonte
		for v := 0; v < 2; v++ {
			fila := make([]byte, 640*4)
			for x := 0; x < 640; x++ {
				dx := (float64(x) - 320) / e // en carriles
				ad := math.Abs(dx)
				var c color.NRGBA
				switch {
				case ad < 1.5:
					c = asfalto[v]
					// las líneas entre carriles (a trozos: solo en las bandas «0»)
					if v == 0 && math.Abs(ad-0.5) < 0.035 {
						c = blanco
					}
					if ad > 1.42 {
						c = amarillo
					}
				case ad < 1.68:
					c = arcen
				default:
					c = arena[v]
				}
				c = mezcla(c, niebla, lejos)
				fila[x*4], fila[x*4+1], fila[x*4+2], fila[x*4+3] = c.R, c.G, c.B, 255
			}
			m.filas[y][v] = fila
		}
	}
}

// cosa: algo que se pinta en el mundo.
type cosa struct {
	z    float64
	x    float64 // en carriles
	alto float64 // a qué altura (las estrellas en el aire)
	spr  string
}

// pintar: el mundo con lo que haya (de lejos a cerca) y Zarpa delante.
func (m *mundo) pintar(dst *image.RGBA, recorrido float64, cosas []cosa, zarpa *image.RGBA, zx, zy float64, conEscudo bool, temblor int) {
	copy(dst.Pix[:len(m.cielo.Pix)], m.cielo.Pix)
	for y := horizonte + 1; y < 480; y++ {
		banda := int(math.Floor((m.zFila[y]+recorrido)/largoBanda)) & 1
		yy := y + temblor
		if yy < horizonte+1 || yy >= 480 {
			yy = y
		}
		copy(dst.Pix[y*dst.Stride:y*dst.Stride+640*4], m.filas[yy][banda])
	}
	sort.Slice(cosas, func(i, j int) bool { return cosas[i].z > cosas[j].z })
	delante := 0 // las que están por detrás de la cámara se pintan después de Zarpa
	for _, c := range cosas {
		if c.z < -0.45 {
			break
		}
		delante++
		m.pintarCosa(dst, c)
	}
	// Zarpa: su sombra en el suelo y ella (con su salto)
	sx := int(320 + zx*escala0)
	sy := int(sueloEn(0))
	sombra := m.sombras[min(int(zy*7), len(m.sombras)-1)]
	sb := sombra.Bounds()
	draw.Draw(dst, sb.Add(image.Pt(sx-sb.Dx()/2, sy-sb.Dy()/2)), sombra, image.Point{}, draw.Over)
	py := sy - int(zy*escala0) + 4
	ponerSobre(dst, zarpa, sx, py)
	if conEscudo {
		ponerSobre(dst, m.burb, sx, py+10)
	}
	for _, c := range cosas[delante:] {
		m.pintarCosa(dst, c)
	}
}

func (m *mundo) pintarCosa(dst *image.RGBA, c cosa) {
	if c.z > zLejos {
		return
	}
	e := escalaEn(c.z)
	sp := m.spr[c.spr]
	if sp == nil {
		return
	}
	img := sp.de(e / escala0)
	x := int(320 + c.x*e)
	y := int(horizonte + altCamara*e - c.alto*e)
	ponerSobre(dst, img, x, y)
}
