package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"
)

type pantalla int

const (
	pTitulo pantalla = iota
	pJuego
	pPausa
	pFin
	pTienda
)

type app struct {
	f    *fuentes
	l    *lienzo
	srv  *servidorPantalla
	luz  *luz
	m    *mundo
	son  *sonido
	pr   *progreso
	pant pantalla

	c      *carrera // la partida
	demo   *carrera // la del fondo del título y de la tienda
	frames map[string][]*image.RGBA

	salir       chan struct{}
	pulsados    uint16
	selTienda   int
	nuevoRecord bool
	avisoTexto  string
	avisoHasta  time.Time
	destello    float64 // el golpe en rojo al chocar (o en azul con el escudo)
	colorDest   color.RGBA
}

func nuevaApp(f *fuentes, pr *progreso) *app {
	a := &app{f: f, l: nuevoLienzo(f), luz: nuevaLuz(), m: nuevoMundo(f), son: nuevoSonido(), pr: pr,
		frames: map[string][]*image.RGBA{}, salir: make(chan struct{})}
	a.son.conMusica(!pr.SinMusica)
	a.demo = nuevaCarrera(1)
	a.demo.soloAdorno = true
	a.zarpaEn(pr.Traje, postura{}) // los dibujos de su traje, ya hechos (los demás, al verlos en la tienda)
	for i, t := range trajes {
		if t.ID == pr.Traje {
			a.selTienda = i
		}
	}
	return a
}

func (a *app) terminar() {
	select {
	case <-a.salir:
	default:
		close(a.salir)
	}
}

func (a *app) aviso(s string) { a.avisoTexto, a.avisoHasta = s, time.Now().Add(2500*time.Millisecond) }

// zarpaEn: el dibujo de Zarpa con ese traje y esa postura (se hacen una vez por traje).
func (a *app) zarpaEn(id string, ps postura) *image.RGBA {
	fs, ok := a.frames[id]
	if !ok {
		t := trajeDe(id)
		for i := 0; i < 8; i++ {
			fs = append(fs, dibujarZarpa(t, postura{fase: float64(i) / 8 * 2 * math.Pi}))
		}
		fs = append(fs, dibujarZarpa(t, postura{salto: true}), dibujarZarpa(t, postura{agachado: true}),
			dibujarZarpa(t, postura{choque: true}))
		a.frames[id] = fs
	}
	switch {
	case ps.choque:
		return fs[10]
	case ps.agachado:
		return fs[9]
	case ps.salto:
		return fs[8]
	}
	return fs[int(ps.fase/(2*math.Pi)*8)%8]
}

// ---- Botones ---------------------------------------------------------------------------------------

func (a *app) botones(m uint16) {
	nuevos := m &^ a.pulsados
	a.pulsados = m
	if a.luz.apagada {
		if nuevos != 0 {
			a.luz.encender()
		}
		return
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

func (a *app) pulsar(b uint16) {
	switch a.pant {
	case pTitulo:
		switch b {
		case bA, bStart:
			a.empezar()
		case bY:
			a.pant = pTienda
		case bX:
			a.pr.SinMusica = !a.pr.SinMusica
			a.son.conMusica(!a.pr.SinMusica)
			a.pr.guardar()
			if a.pr.SinMusica {
				a.aviso("Música quitada")
			} else {
				a.aviso("Música puesta")
			}
		case bB:
			a.terminar()
		}
	case pJuego:
		c := a.c
		switch b {
		case bIzquierda:
			c.mover(-1)
		case bDerecha:
			c.mover(1)
		case bArriba, bA, bX:
			c.saltar()
		case bAbajo, bB:
			c.agacharse()
		case bStart, bSelect:
			if c.viva {
				a.pant = pPausa
			}
		}
	case pPausa:
		switch b {
		case bA, bStart:
			a.pant = pJuego
		case bB, bSelect:
			// las estrellas cogidas no se pierden, pero no cuenta para el récord
			a.pr.Estrellas += a.c.estrellas
			a.pr.Total += a.c.estrellas
			a.pr.guardar()
			a.pant = pTitulo
		}
	case pFin:
		switch b {
		case bA, bStart:
			a.empezar()
		case bY:
			a.pant = pTienda
		case bB:
			a.pant = pTitulo
		}
	case pTienda:
		switch b {
		case bIzquierda:
			a.selTienda = (a.selTienda + len(trajes) - 1) % len(trajes)
		case bDerecha:
			a.selTienda = (a.selTienda + 1) % len(trajes)
		case bA:
			a.comprar(trajes[a.selTienda])
		case bB, bY:
			a.pant = pTitulo
		}
	}
}

func (a *app) comprar(t traje) {
	switch {
	case a.pr.tiene(t.ID):
		a.pr.Traje = t.ID
		a.pr.guardar()
		a.aviso("Puesto: " + t.Nombre)
	case a.pr.Estrellas < t.Precio:
		a.aviso(fmt.Sprintf("Te faltan %d estrellas", t.Precio-a.pr.Estrellas))
	default:
		a.pr.Estrellas -= t.Precio
		a.pr.Trajes = append(a.pr.Trajes, t.ID)
		a.pr.Traje = t.ID
		a.pr.guardar()
		a.son.efecto("poder")
		a.aviso("¡Comprado! Ya llevas " + t.Nombre)
	}
}

func (a *app) empezar() {
	a.c = nuevaCarrera(time.Now().UnixNano())
	a.pant, a.nuevoRecord = pJuego, false
}

// ---- Cada imagen -------------------------------------------------------------------------------------

func (a *app) paso(dt float64) {
	if !a.avisoHasta.IsZero() && time.Now().After(a.avisoHasta) {
		a.avisoTexto, a.avisoHasta = "", time.Time{}
	}
	a.destello = math.Max(0, a.destello-dt*2.5)
	switch a.pant {
	case pJuego:
		c := a.c
		viva := c.viva
		c.paso(dt)
		for _, s := range c.avisos {
			a.son.efecto(s)
			switch s {
			case "choque":
				a.destello, a.colorDest = 1, color.RGBA{255, 40, 40, 255}
			case "escudo":
				a.destello, a.colorDest = 0.8, color.RGBA{60, 200, 255, 255}
			}
		}
		if viva && !c.viva {
			a.nuevoRecord = a.pr.acabarPartida(c)
		}
		if !c.viva && c.muerta > 1.3 {
			a.pant = pFin
			if a.nuevoRecord {
				a.son.efecto("record")
			}
		}
	case pTitulo, pTienda, pFin:
		a.demo.paso(dt)
	}
}

func (a *app) dibujar() {
	l := a.l
	switch a.pant {
	case pTitulo:
		a.pintarDemo()
		a.dibujarTitulo()
	case pTienda:
		a.pintarDemo()
		a.dibujarTienda()
	case pJuego, pPausa:
		a.pintarCarrera()
		if a.pant == pPausa {
			a.dibujarPausa()
		}
	case pFin:
		a.pintarCarrera()
		a.dibujarFin()
	}
	if a.destello > 0 {
		c := a.colorDest
		k := uint8(a.destello * 110)
		l.rect(l.img.Bounds(), color.RGBA{uint8(int(c.R) * int(k) / 255), uint8(int(c.G) * int(k) / 255), uint8(int(c.B) * int(k) / 255), k})
	}
	if a.avisoTexto != "" {
		s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
		w := medir(l.f.pequena, s) + 36
		r := image.Rect((ancho-w)/2, alto-110, (ancho+w)/2, alto-78)
		l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xe0})
		l.centrado(r, s, l.f.pequena, colTexto)
	}
}

func (a *app) pintarDemo() {
	d := a.demo
	img := a.zarpaEn(a.pr.Traje, postura{fase: d.fase})
	if a.pant == pTienda {
		img = a.zarpaEn(trajes[a.selTienda].ID, postura{fase: d.fase})
	}
	a.m.pintar(a.l.img, d.recorrido, d.cosas(), img, 0, 0, false, 0)
}

func (a *app) pintarCarrera() {
	c := a.c
	temblor := 0
	if c.temblor > 0 {
		temblor = int(math.Sin(c.tiempo*90+c.muerta*70) * c.temblor * 6)
	}
	img := a.zarpaEn(a.pr.Traje, c.postura())
	a.m.pintar(a.l.img, c.recorrido, c.cosas(), img, c.carrilX, c.y, c.escudo > 0, temblor)
	a.dibujarMarcador()
}

// ---- Lo de encima --------------------------------------------------------------------------------------

// marcador: arriba a la izquierda (arriba a la derecha va el recuadro de la batería y la hora).
func (a *app) dibujarMarcador() {
	l, c := a.l, a.c
	r := image.Rect(14, 10, 220, 96)
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0x90})
	l.izquierdaRapido(image.Rect(r.Min.X+16, r.Min.Y+4, r.Max.X-8, r.Min.Y+44), puntosConPunto(c.puntos()), l.f.marcador, colTexto)
	est := a.m.spr["estrella"].de(0.38)
	ponerSobre(l.img, est, r.Min.X+30, r.Min.Y+80)
	l.izquierdaRapido(image.Rect(r.Min.X+48, r.Min.Y+50, r.Max.X-8, r.Min.Y+80), itoa(c.estrellas), l.f.titulo2, color.RGBA{0xff, 0xd8, 0x40, 0xff})
	l.derechaRapido(image.Rect(r.Min.X, r.Min.Y+50, r.Max.X-16, r.Min.Y+80), itoa(c.metros())+" m", l.f.pequena, colSuave)
	// los poderes que llevas y lo que les queda
	y := r.Max.Y + 8
	for _, p := range []struct {
		spr  string
		t, d float64
	}{{"iman", c.iman, tIman}, {"escudo", c.escudo, tEscudo}} {
		if p.t <= 0 {
			continue
		}
		b := image.Rect(14, y, 150, y+30)
		l.redondeado(b, 15, color.RGBA{0, 0, 0, 0x90})
		ponerSobre(l.img, a.m.spr[p.spr].de(0.34), b.Min.X+18, b.Max.Y-2)
		barra := image.Rect(b.Min.X+38, b.Min.Y+12, b.Max.X-12, b.Min.Y+18)
		l.redondeado(barra, 3, color.RGBA{0x50, 0x50, 0x60, 0xff})
		l.redondeado(image.Rect(barra.Min.X, barra.Min.Y, barra.Min.X+int(float64(barra.Dx())*p.t/p.d), barra.Max.Y), 3, color.RGBA{0x40, 0xd8, 0xff, 0xff})
		y += 36
	}
}

func puntosConPunto(n int) string {
	s := itoa(n)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return b.String()
}

var imgLogo *image.RGBA

// logo: «ZARPA» con brillo de neón (se pinta una vez y luego se pega).
func (a *app) logo(y int) {
	if imgLogo == nil {
		l := &lienzo{img: image.NewRGBA(image.Rect(0, 0, ancho, 100)), f: a.f}
		s := "ZARPA"
		x := (ancho - medir(l.f.enorme, s)) / 2
		base := 10 + l.f.enorme.Metrics().Ascent.Round()
		for _, d := range [][2]int{{-3, 0}, {3, 0}, {0, -3}, {0, 3}, {-2, -2}, {2, 2}, {2, -2}, {-2, 2}} {
			l.texto(x+d[0], base+d[1], s, l.f.enorme, color.RGBA{0xff, 0x3a, 0x9a, 0x70})
		}
		l.texto(x+3, base+4, s, l.f.enorme, color.RGBA{0x40, 0x10, 0x50, 0xff})
		l.texto(x, base, s, l.f.enorme, color.RGBA{0xff, 0xf4, 0xf8, 0xff})
		imgLogo = l.img
	}
	draw.Draw(a.l.img, imgLogo.Bounds().Add(image.Pt(0, y-10)), imgLogo, image.Point{}, draw.Over)
}

func (a *app) dibujarTitulo() {
	l := a.l
	l.rect(image.Rect(0, 0, ancho, 200), color.RGBA{0, 0, 0, 0x30})
	a.logo(40)
	l.centrado(image.Rect(0, 118, ancho, 146), "Carrera por Los Ángeles", l.f.titulo2, color.RGBA{0xff, 0xd8, 0x80, 0xff})
	info := image.Rect(ancho/2-170, 158, ancho/2+170, 192)
	l.redondeado(info, 17, color.RGBA{0, 0, 0, 0xa0})
	l.centrado(info, fmt.Sprintf("Récord %s     Estrellas %s", puntosConPunto(a.pr.Record), puntosConPunto(a.pr.Estrellas)), l.f.normal, colTexto)
	musica := "Música: sí"
	if a.pr.SinMusica {
		musica = "Música: no"
	}
	a.barraAbajo("A  Jugar      Y  Trajes      X  " + musica + "      B  Salir")
}

func (a *app) barraAbajo(s string) {
	l := a.l
	r := image.Rect(0, alto-40, ancho, alto)
	l.rect(r, color.RGBA{0, 0, 0, 0xb0})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) dibujarPausa() {
	l := a.l
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x90})
	l.centrado(image.Rect(0, 170, ancho, 230), "Pausa", l.f.enorme, colTexto)
	l.centrado(image.Rect(0, 240, ancho, 270), "Las estrellas que llevas no se pierden si sales", l.f.normal, colSuave)
	a.barraAbajo("A  Seguir      B  Salir al menú")
}

func (a *app) dibujarFin() {
	l, c := a.l, a.c
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x70})
	caja := image.Rect(110, 80, ancho-110, 380)
	l.redondeado(caja, 22, color.RGBA{0x18, 0x0e, 0x2c, 0xf0})
	l.marcoRedondo(caja, 22, 3, color.RGBA{0xff, 0x5a, 0xb0, 0xff})
	titulo, ct := "¡Chocaste!", color.RGBA{0xff, 0x8a, 0x8a, 0xff}
	if a.nuevoRecord {
		titulo, ct = "¡Nuevo récord!", color.RGBA{0xff, 0xd8, 0x40, 0xff}
	}
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+18, caja.Max.X, caja.Min.Y+62), titulo, l.f.titulo, ct)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+70, caja.Max.X, caja.Min.Y+130), puntosConPunto(c.puntos()), l.f.enorme, colTexto)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+130, caja.Max.X, caja.Min.Y+152), "puntos", l.f.pequena, colSuave)
	filas := [][2]string{{"Metros", puntosConPunto(c.metros())}, {"Estrellas cogidas", itoa(c.estrellas)},
		{"Récord", puntosConPunto(a.pr.Record)}, {"Estrellas para la tienda", puntosConPunto(a.pr.Estrellas)}}
	for i, f := range filas {
		y := caja.Min.Y + 166 + i*28
		l.izquierda(image.Rect(caja.Min.X+40, y, caja.Max.X-40, y+26), f[0], l.f.normal, colSuave)
		l.derecha(image.Rect(caja.Min.X+40, y, caja.Max.X-40, y+26), f[1], l.f.nombre, colTexto)
	}
	a.barraAbajo("A  Otra vez      Y  Trajes      B  Menú")
}

func (a *app) dibujarTienda() {
	l := a.l
	t := trajes[a.selTienda]
	l.rect(image.Rect(0, 0, ancho, 150), color.RGBA{0, 0, 0, 0x50})
	l.centrado(image.Rect(0, 14, ancho, 54), "Trajes de Zarpa", l.f.titulo, colTexto)
	l.centrado(image.Rect(0, 52, ancho, 76), fmt.Sprintf("Tienes %s estrellas", puntosConPunto(a.pr.Estrellas)), l.f.normal, color.RGBA{0xff, 0xd8, 0x40, 0xff})
	// los trajes en fila: el elegido en grande
	for i, tr := range trajes {
		x := ancho/2 - (len(trajes)*44)/2 + i*44 + 22
		c := color.RGBA{tr.Pelaje.R, tr.Pelaje.G, tr.Pelaje.B, 255}
		if i == a.selTienda {
			a.l.circulo(x, 100, 18, colTexto)
		}
		a.l.circulo(x, 100, 15, c)
		a.l.circulo(x+7, 107, 6, color.RGBA{tr.Bufanda.R, tr.Bufanda.G, tr.Bufanda.B, 255})
	}
	caja := image.Rect(150, 132, ancho-150, 224) // arriba: Zarpa se ve abajo con el traje
	l.redondeado(caja, 18, color.RGBA{0x18, 0x0e, 0x2c, 0xe8})
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+10, caja.Max.X, caja.Min.Y+44), t.Nombre, l.f.titulo, colTexto)
	estado, ce := "", colSuave
	switch {
	case t.ID == a.pr.Traje:
		estado, ce = "Lo llevas puesto", color.RGBA{0x6a, 0xff, 0x8a, 0xff}
	case a.pr.tiene(t.ID):
		estado = "Es tuyo: A para ponértelo"
	case a.pr.Estrellas >= t.Precio:
		estado, ce = fmt.Sprintf("%d estrellas: A para comprarlo", t.Precio), color.RGBA{0xff, 0xd8, 0x40, 0xff}
	default:
		estado = fmt.Sprintf("Cuesta %d estrellas (te faltan %d)", t.Precio, t.Precio-a.pr.Estrellas)
	}
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+50, caja.Max.X, caja.Min.Y+80), estado, l.f.normal, ce)
	a.barraAbajo("IZQ DER  Elegir      A  Comprar o ponértelo      B  Volver")
}
