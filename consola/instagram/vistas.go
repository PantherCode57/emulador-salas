package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
	"golang.org/x/image/font"
)

// Las pantallas siguen el estilo del resto de Panther OS (el fondo de las listas, letra Selawik, el
// borde azul en lo elegido) con los colores de Instagram. Arriba a la derecha no se pinta nada: ahí
// va el recuadro de la batería y la hora del servicio de Panther.

var nombrePestana = map[pantalla]string{pInicio: "Inicio", pReels: "Reels", pHistorias: "Historias", pMensajes: "Mensajes", pCuenta: "Tú"}

// colorIG: el degradado de Instagram (t de 0 a 1: amarillo, naranja, rosa, morado y azul).
func colorIG(t float64) color.RGBA {
	paradas := []struct {
		t float64
		c color.RGBA
	}{{0, color.RGBA{0xfe, 0xda, 0x75, 0xff}}, {0.28, color.RGBA{0xfa, 0x7e, 0x1e, 0xff}}, {0.55, color.RGBA{0xd6, 0x29, 0x76, 0xff}},
		{0.8, color.RGBA{0x96, 0x2f, 0xbf, 0xff}}, {1, color.RGBA{0x4f, 0x5b, 0xd5, 0xff}}}
	t = math.Max(0, math.Min(1, t))
	for i := 1; i < len(paradas); i++ {
		if t <= paradas[i].t {
			a, b := paradas[i-1], paradas[i]
			k := (t - a.t) / (b.t - a.t)
			m := func(x, y uint8) uint8 { return uint8(float64(x)*(1-k) + float64(y)*k) }
			return color.RGBA{m(a.c.R, b.c.R), m(a.c.G, b.c.G), m(a.c.B, b.c.B), 0xff}
		}
	}
	return paradas[len(paradas)-1].c
}

// degradado pinta en r el degradado de Instagram (de abajo a la izquierda a arriba a la derecha)
// donde cobertura(x, y) > 0.
func (a *app) degradado(r image.Rectangle, cobertura func(fx, fy float64) float64) {
	img := a.l.img
	r = r.Intersect(img.Bounds())
	w, h := float64(max(r.Dx()-1, 1)), float64(max(r.Dy()-1, 1))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := cobertura(float64(x)+0.5, float64(y)+0.5)
			if c <= 0 {
				continue
			}
			c = math.Min(c, 1)
			t := (float64(x-r.Min.X)/w + float64(r.Max.Y-1-y)/h) / 2
			col := colorIG(t)
			i := img.PixOffset(x, y)
			img.Pix[i] = uint8(float64(img.Pix[i])*(1-c) + float64(col.R)*c)
			img.Pix[i+1] = uint8(float64(img.Pix[i+1])*(1-c) + float64(col.G)*c)
			img.Pix[i+2] = uint8(float64(img.Pix[i+2])*(1-c) + float64(col.B)*c)
			img.Pix[i+3] = 0xff
		}
	}
}

// logo: el cuadrado de colores con la cámara.
func (a *app) logo(x, y, s int) {
	l := a.l
	radio := s * 28 / 100
	a.degradado(image.Rect(x, y, x+s, y+s), func(fx, fy float64) float64 {
		return distanciaBorde(int(fx)-x, int(fy)-y, s, s, radio) + 0.5
	})
	g := max(1, s*8/100)
	in := s * 21 / 100
	l.marcoRedondo(image.Rect(x+in, y+in, x+s-in, y+s-in), s*17/100, g, colTexto)
	rc := s * 15 / 100
	cx, cy := x+s/2, y+s/2
	l.marcoRedondo(image.Rect(cx-rc, cy-rc, cx+rc, cy+rc), rc, g, colTexto)
	if p := max(1, s*5/100); p > 0 {
		l.circulo(x+s*68/100, y+s*32/100, p, colTexto)
	}
}

// anilloIG: el aro de colores de las historias sin ver.
func (a *app) anilloIG(cx, cy, r, g int) {
	fr, fg := float64(r), float64(g)
	a.degradado(image.Rect(cx-r, cy-r, cx+r, cy+r), func(fx, fy float64) float64 {
		d := math.Hypot(fx-float64(cx), fy-float64(cy))
		return math.Min(fr-d, d-(fr-fg)) + 0.5
	})
}

func (l *lienzo) aro(cx, cy, r, g int, c color.Color) {
	l.marcoRedondo(image.Rect(cx-r, cy-r, cx+r, cy+r), r, g, c)
}

// fondoEn vuelve a poner el fondo en r (para tapar lo que se salga de una lista).
func (l *lienzo) fondoEn(r image.Rectangle) {
	r = r.Intersect(l.img.Bounds())
	if l.fondo == nil {
		draw.Draw(l.img, r, &image.Uniform{colFondo}, image.Point{}, draw.Src)
		return
	}
	draw.Draw(l.img, r, l.fondo, r.Min, draw.Src)
}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

func (a *app) dibujar() {
	l := a.l
	switch a.pant {
	case pReels, pVideo, pHistoria:
		l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0xff})
	default:
		l.limpiarFondo()
	}
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pInicio:
		a.dibujarInicio()
	case pReels:
		a.dibujarReels()
	case pHistorias:
		a.dibujarHistorias()
	case pMensajes:
		a.dibujarMensajes()
	case pCuenta:
		a.dibujarCuenta()
	case pHistoria:
		a.dibujarHistoria()
	case pVideo:
		a.dibujarVideo()
	case pChat:
		a.dibujarChat()
	case pEscribir:
		a.dibujarEscribir()
	case pConfirmar:
		switch a.confirmVolver {
		case pCuenta:
			a.dibujarCuenta()
		}
		a.dibujarConfirmar()
	}
	if !hayRed() {
		a.etiqueta(image.Pt(ancho/2, alto-58), "Sin internet")
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Instagram", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

// cabecera: el logo y las pestañas (la elegida en blanco y subrayada con los colores de Instagram).
func (a *app) cabecera() {
	l := a.l
	a.logo(16, 11, 30)
	x := 62
	for _, p := range pestanas {
		nombre := nombrePestana[p]
		w := medir(l.f.nombre, nombre)
		col := colApagado
		if p == a.pant || (a.pant == pConfirmar && p == a.confirmVolver) {
			col = colTexto
			a.degradado(image.Rect(x, 42, x+w, 46), func(fx, fy float64) float64 { return 1 })
		}
		l.izquierda(image.Rect(x, 10, x+w+4, 40), nombre, l.f.nombre, col)
		if p == pMensajes && a.sinLeer() > 0 {
			l.circulo(x+w+7, 17, 4, colRosa)
		}
		x += w + 24
	}
}

func (a *app) etiqueta(centro image.Point, s string) {
	l := a.l
	w := medir(l.f.mini, s) + 24
	r := image.Rect(centro.X-w/2, centro.Y-11, centro.X+w/2, centro.Y+13)
	l.redondeado(r, 12, color.RGBA{0x3a, 0x17, 0x20, 0xe6})
	l.centrado(r, s, l.f.mini, color.RGBA{0xff, 0xc2, 0xcb, 0xff})
}

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-88, (ancho+w)/2, alto-54)
	l.redondeado(r, 17, color.RGBA{0, 0, 0, 0xe6})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	s = recortar(l.f.pequena, s, ancho-24)
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-22+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

// mensajeCentrado: lo que sale en vez de una lista (cargando, vacía o con error).
func (a *app) mensajeCentrado(r image.Rectangle, texto string, col color.Color) {
	l := a.l
	lineas := ajustar(l.f.texto, texto, r.Dx()-40)
	y := r.Min.Y + (r.Dy()-len(lineas)*26)/2
	for _, ln := range lineas {
		l.centrado(image.Rect(r.Min.X, y, r.Max.X, y+26), ln, l.f.texto, col)
		y += 26
	}
}

func (a *app) estadoTira(t *tira, cargando, vacio string) (string, color.Color) {
	switch {
	case t.err != "":
		return t.err + " (A para reintentar)", colRojo
	case t.cargando || !t.cargada:
		return cargando, colSuave
	}
	return vacio, colSuave
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (a *app) barraProgreso(x0, x1, y int, pos, dur time.Duration) {
	l := a.l
	l.redondeado(image.Rect(x0, y, x1, y+4), 2, color.RGBA{0x60, 0x60, 0x60, 0xff})
	if dur > 0 {
		xp := x0 + int(float64(x1-x0)*math.Min(1, pos.Seconds()/dur.Seconds()))
		l.redondeado(image.Rect(x0, y, xp, y+4), 2, colTexto)
	}
}

// pegarEn pone im centrada en r.
func (a *app) pegarEn(im *image.RGBA, r image.Rectangle) {
	b := im.Bounds()
	a.l.pegar(im, nil, r.Min.X+(r.Dx()-b.Dx())/2, r.Min.Y+(r.Dy()-b.Dy())/2)
}

// medioEn: la foto de un medio dentro de r (o un hueco mientras llega).
func (a *app) medioEn(m Medio, r image.Rectangle, radio int) {
	if im := a.img.pedir(m.Foto, r.Dx(), r.Dy(), false, radio); im != nil {
		a.pegarEn(im, r)
		return
	}
	a.l.centrado(r, icFoto, a.l.f.iconoGrande, color.RGBA{0xff, 0xff, 0xff, 0x40})
}

// lineasTexto escribe un texto partido en líneas hasta yMax (la última, con «…» si no cabe).
func (a *app) lineasTexto(x, y, w, yMax int, s string, cara font.Face, col color.Color, alto int) int {
	l := a.l
	lineas := ajustar(cara, l.f.limpiar(s), w)
	for i, ln := range lineas {
		if y+alto > yMax {
			break
		}
		if y+2*alto > yMax && i < len(lineas)-1 {
			ln = recortar(cara, ln+"…", w)
		}
		l.izquierda(image.Rect(x, y, x+w, y+alto), ln, cara, col)
		y += alto
	}
	return y
}

// ---- Inicio --------------------------------------------------------------------------------------

var cajaInicio = image.Rect(16, 54, 16+384, 54+384)

func (a *app) dibujarInicio() {
	l := a.l
	a.cabecera()
	p := a.postInicio()
	if p == nil {
		s, c := a.estadoTira(&a.inicio, "Cargando tu inicio…", "No hay nada nuevo. Pulsa X para actualizar.")
		a.mensajeCentrado(image.Rect(0, 60, ancho, alto-40), s, c)
		a.ayudaAbajo("X  Actualizar      L R  Pestañas      B  Salir")
		return
	}
	caja := cajaInicio
	a.translucido(caja, 14, 26)
	m, i := a.medioDe(p)
	a.medioEn(m, caja, 0)
	if m.esVideo() {
		cx, cy := caja.Min.X+caja.Dx()/2, caja.Min.Y+caja.Dy()/2
		l.circulo(cx, cy, 32, color.RGBA{0, 0, 0, 0xa8})
		l.centrado(image.Rect(cx-30, cy-30, cx+34, cy+30), icPlay, l.f.iconoGrande, colTexto)
		if m.Duracion > 0 {
			s := fmtDur(time.Duration(m.Duracion * float64(time.Second)))
			w := medir(l.f.mini, s) + 12
			e := image.Rect(caja.Max.X-w-8, caja.Max.Y-26, caja.Max.X-8, caja.Max.Y-8)
			l.redondeado(e, 5, color.RGBA{0, 0, 0, 0xc0})
			l.centrado(e, s, l.f.mini, colTexto)
		}
	}
	if n := len(p.Medios); n > 1 {
		s := fmt.Sprintf("%d/%d", i+1, n)
		w := medir(l.f.mini, s) + 16
		e := image.Rect(caja.Max.X-w-8, caja.Min.Y+8, caja.Max.X-8, caja.Min.Y+30)
		l.redondeado(e, 11, color.RGBA{0, 0, 0, 0xb0})
		l.centrado(e, s, l.f.mini, colTexto)
		x0 := caja.Min.X + caja.Dx()/2 - (n*12)/2
		for k := 0; k < n && k < 20; k++ {
			c := color.Color(color.RGBA{0xff, 0xff, 0xff, 0x70})
			if k == i {
				c = colBurbuja
			}
			l.circulo(x0+k*12+6, caja.Max.Y-12, 3, c)
		}
	}
	// A la derecha: quién, cuándo, los «me gusta» y el texto.
	x0, x1 := caja.Max.X+16, ancho-14
	a.cara(x0+18, 76, 18, p.FotoUsuario, p.Usuario)
	l.izquierda(image.Rect(x0+44, 58, x1, 78), recortar(l.f.nombre, p.Usuario, x1-x0-44), l.f.nombre, colTexto)
	sub := hace(p.Cuando)
	if p.Sugerido {
		sub = "Sugerencia para ti"
	}
	l.izquierda(image.Rect(x0+44, 78, x1, 96), sub, l.f.mini, colApagado)
	y := 112
	corazon, cc := icCorazonVacio, color.Color(colTexto)
	if p.MeGusta {
		corazon, cc = icCorazon, colRosa
	}
	l.izquierda(image.Rect(x0, y, x0+24, y+24), corazon, l.f.icono, cc)
	likes := "Me gusta"
	if p.Likes > 0 {
		likes = cifra(p.Likes) + " Me gusta"
	}
	l.izquierda(image.Rect(x0+28, y, x1, y+24), likes, l.f.normal, colTexto)
	y += 28
	if p.Comentarios > 0 {
		l.izquierda(image.Rect(x0, y, x0+24, y+24), icComentario, l.f.icono, colSuave)
		l.izquierda(image.Rect(x0+28, y, x1, y+24), cifra(p.Comentarios)+" comentarios", l.f.pequena, colSuave)
		y += 28
	}
	y += 8
	if p.Texto != "" {
		a.lineasTexto(x0, y, x1-x0, caja.Max.Y-24, p.Texto, l.f.pequena, colSuave, 20)
	}
	l.izquierda(image.Rect(x0, caja.Max.Y-20, x1, caja.Max.Y), fmt.Sprintf("%d de %d", a.selInicio+1, len(a.inicio.posts)), l.f.mini, colApagado)
	ayuda := "ARRIBA ABAJO  Mover      Y  Me gusta      X  Actualizar      L R  Pestañas"
	switch {
	case m.esVideo():
		ayuda = "A  Ver vídeo      ARRIBA ABAJO  Mover      Y  Me gusta      L R  Pestañas"
	case len(p.Medios) > 1:
		ayuda = "IZQ DER  Fotos      ARRIBA ABAJO  Mover      Y  Me gusta      L R  Pestañas"
	}
	a.ayudaAbajo(ayuda)
}

// ---- Vídeo del inicio ----------------------------------------------------------------------------

func (a *app) dibujarVideo() {
	l := a.l
	p, m := a.videoPost, a.videoMedio
	area := a.areaVideo()
	if !a.rep.hayImagen() {
		a.medioEn(m, area, 0)
	}
	y := 416
	a.cara(30, y+14, 14, p.FotoUsuario, p.Usuario)
	l.izquierda(image.Rect(52, y, 300, y+28), recortar(l.f.nombre, p.Usuario, 240), l.f.nombre, colTexto)
	corazon, cc := icCorazonVacio, color.Color(colTexto)
	if p.MeGusta {
		corazon, cc = icCorazon, colRosa
	}
	l.izquierda(image.Rect(310, y, 334, y+28), corazon, l.f.icono, cc)
	l.izquierda(image.Rect(336, y, 450, y+28), cifra(p.Likes), l.f.pequena, colTexto)
	pos, dur := a.rep.posicion(), a.rep.duracion()
	ic := icPausa
	if a.rep.enPausa() {
		ic = icPlay
	}
	l.izquierda(image.Rect(470, y, 494, y+28), ic, l.f.iconoPeq, colTexto)
	l.derecha(image.Rect(494, y, ancho-16, y+28), fmtDur(pos)+" / "+fmtDur(dur), l.f.pequena, colTexto)
	a.barraProgreso(16, ancho-16, y+34, pos, dur)
	ayuda := "A  Pausa      IZQ DER  5 s      Y  Me gusta      B  Volver"
	if a.videoErr != "" {
		ayuda = a.videoErr + "      B  Volver"
	}
	l.centrado(image.Rect(0, y+42, ancho, alto), ayuda, l.f.mini, colAyuda)
}

// ---- Reels ---------------------------------------------------------------------------------------

// panelBotones: la lista de botones de la derecha en los reels y las historias.
func (a *app) panelBotones(x0, x1 int, botones [][2]string) {
	l := a.l
	y := alto - 22 - len(botones)*24
	for _, b := range botones {
		l.izquierda(image.Rect(x0, y, x0+66, y+22), b[0], l.f.mini, colTexto)
		l.izquierda(image.Rect(x0+68, y, x1, y+22), b[1], l.f.mini, colAyuda)
		y += 24
	}
}

func (a *app) dibujarReels() {
	l := a.l
	area := image.Rect((ancho-anchoVert)/2, 0, (ancho+anchoVert)/2, altoVert)
	izq := image.Rect(14, 0, area.Min.X-14, alto)
	der := image.Rect(area.Max.X+14, 0, ancho-14, alto)
	l.izquierda(image.Rect(izq.Min.X, 18, izq.Min.X+26, 46), icReels, l.f.icono, colTexto)
	l.izquierda(image.Rect(izq.Min.X+28, 18, izq.Max.X, 46), "Reels", l.f.nombre, colTexto)
	p := a.reelActual()
	if p == nil {
		a.translucido(area, 0, 16)
		s, c := a.estadoTira(&a.reels, "Buscando reels…", "No hay reels. Pulsa X para buscar más.")
		a.mensajeCentrado(area, s, c)
		a.panelBotones(der.Min.X, der.Max.X, [][2]string{{"X", "Buscar más"}, {"L R", "Pestañas"}, {"B", "Inicio"}})
		return
	}
	if !a.rep.hayImagen() {
		a.medioEn(p.Medios[0], area, 0)
		if a.videoErr == "" {
			l.centrado(image.Rect(area.Min.X, area.Max.Y-60, area.Max.X, area.Max.Y-30), "Cargando…", l.f.pequena, colTexto)
		}
	}
	// Izquierda: quién, el texto y los «me gusta».
	y := 70
	a.cara(izq.Min.X+16, y+16, 16, p.FotoUsuario, p.Usuario)
	l.izquierda(image.Rect(izq.Min.X+40, y, izq.Max.X, y+32), recortar(l.f.nombre, p.Usuario, izq.Dx()-40), l.f.nombre, colTexto)
	y += 46
	corazon, cc := icCorazonVacio, color.Color(colTexto)
	if p.MeGusta {
		corazon, cc = icCorazon, colRosa
	}
	l.izquierda(image.Rect(izq.Min.X, y, izq.Min.X+24, y+24), corazon, l.f.icono, cc)
	l.izquierda(image.Rect(izq.Min.X+28, y, izq.Max.X, y+24), cifra(p.Likes), l.f.pequena, colTexto)
	y += 26
	if p.Comentarios > 0 {
		l.izquierda(image.Rect(izq.Min.X, y, izq.Min.X+24, y+24), icComentario, l.f.icono, colSuave)
		l.izquierda(image.Rect(izq.Min.X+28, y, izq.Max.X, y+24), cifra(p.Comentarios), l.f.pequena, colSuave)
		y += 26
	}
	y += 10
	a.lineasTexto(izq.Min.X, y, izq.Dx(), alto-50, p.Texto, l.f.mini, colSuave, 18)
	l.izquierda(image.Rect(izq.Min.X, alto-42, izq.Max.X, alto-20), fmt.Sprintf("%d de %d", a.selReel+1, len(a.reels.posts)), l.f.pequena, colApagado)
	// Derecha: cómo va y los botones (empieza más abajo: arriba va el recuadro de la hora).
	pos, dur := a.rep.posicion(), a.rep.duracion()
	ic := icPausa
	if a.rep.enPausa() || !a.rep.hayImagen() {
		ic = icPlay
	}
	l.izquierda(image.Rect(der.Min.X, 50, der.Min.X+24, 76), ic, l.f.iconoPeq, colTexto)
	l.izquierda(image.Rect(der.Min.X+26, 50, der.Max.X, 76), fmtDur(pos)+" / "+fmtDur(dur), l.f.pequena, colTexto)
	a.barraProgreso(der.Min.X, der.Max.X, 84, pos, dur)
	if a.videoErr != "" {
		a.lineasTexto(der.Min.X, 100, der.Dx(), 180, a.videoErr+". Pasa a otro.", l.f.mini, colRojo, 18)
	}
	a.panelBotones(der.Min.X, der.Max.X, [][2]string{{"ARRIBA", "Anterior"}, {"ABAJO", "Siguiente"}, {"A", "Pausa"},
		{"IZQ DER", "5 s"}, {"Y", "Me gusta"}, {"L R", "Pestañas"}, {"B", "Inicio"}})
}

// ---- Historias -----------------------------------------------------------------------------------

func (a *app) dibujarHistorias() {
	l := a.l
	a.cabecera()
	if len(a.bandejas) == 0 {
		s, c := "Cargando las historias…", color.Color(colSuave)
		switch {
		case a.histErr != "":
			s, c = a.histErr+" (A para reintentar)", colRojo
		case a.histHecha && !a.histCarga:
			s = "Nadie tiene historias ahora. Pulsa X para actualizar."
		}
		a.mensajeCentrado(image.Rect(0, 60, ancho, alto-40), s, c)
		a.ayudaAbajo("X  Actualizar      L R  Pestañas      B  Inicio")
		return
	}
	const celda, fila, y0 = 120, 128, 60
	filaSel := a.selHist / colsHist
	primera := max(0, filaSel-2)
	for k := primera * colsHist; k < len(a.bandejas) && k < (primera+3)*colsHist; k++ {
		b := a.bandejas[k]
		col, f := k%colsHist, k/colsHist-primera
		cx, cy := 20+col*celda+celda/2, y0+f*fila+50
		if k == a.selHist {
			l.aro(cx, cy, 52, 3, colMarco)
		}
		if b.Vista {
			l.aro(cx, cy, 46, 2, color.RGBA{0x80, 0x80, 0x8c, 0xff})
		} else {
			a.anilloIG(cx, cy, 47, 4)
		}
		a.cara(cx, cy, 40, b.Foto, b.Usuario)
		nombre := recortar(l.f.mini, b.Usuario, celda-8)
		c := color.Color(colSuave)
		if k == a.selHist {
			c = colTexto
		}
		l.centrado(image.Rect(cx-celda/2, cy+52, cx+celda/2, cy+72), nombre, l.f.mini, c)
	}
	a.ayudaAbajo("A  Ver      Cruceta  Elegir      X  Actualizar      L R  Pestañas      B  Inicio")
}

func (a *app) dibujarHistoria() {
	l := a.l
	area := image.Rect((ancho-anchoVert)/2, 0, (ancho+anchoVert)/2, altoVert)
	izq := image.Rect(14, 0, area.Min.X-14, alto)
	der := image.Rect(area.Max.X+14, 0, ancho-14, alto)
	if a.hb >= len(a.bandejas) {
		return
	}
	b := a.bandejas[a.hb]
	p := a.historiaActual()
	a.cara(izq.Min.X+22, 74, 22, b.Foto, b.Usuario)
	l.izquierda(image.Rect(izq.Min.X, 104, izq.Max.X, 128), recortar(l.f.nombre, b.Usuario, izq.Dx()), l.f.nombre, colTexto)
	if p == nil {
		a.translucido(area, 0, 16)
		a.mensajeCentrado(area, "Cargando…", colSuave)
		return
	}
	l.izquierda(image.Rect(izq.Min.X, 128, izq.Max.X, 148), hace(p.Cuando), l.f.mini, colApagado)
	l.izquierda(image.Rect(izq.Min.X, 156, izq.Max.X, 176), fmt.Sprintf("%d de %d", a.hi+1, len(b.Items)), l.f.mini, colApagado)
	m := p.Medios[0]
	if !m.esVideo() || !a.rep.hayImagen() {
		a.medioEn(m, area, 0)
	}
	// Las rayas de arriba: una por historia; la de ahora se va llenando.
	n := len(b.Items)
	hueco := 3
	w := (area.Dx() - 12 - (n-1)*hueco) / max(n, 1)
	avance := 0.0
	switch {
	case m.esVideo():
		if d := a.rep.duracion(); d > 0 {
			avance = a.rep.posicion().Seconds() / d.Seconds()
		}
	case a.histPausa > 0:
		avance = a.histPausa.Seconds() / duraFoto.Seconds()
	case !a.histDesde.IsZero():
		avance = time.Since(a.histDesde).Seconds() / duraFoto.Seconds()
	}
	if w >= 2 && !m.esVideo() {
		for k := 0; k < n; k++ {
			x := area.Min.X + 6 + k*(w+hueco)
			r := image.Rect(x, 8, x+w, 11)
			l.redondeado(r, 1, color.RGBA{0xff, 0xff, 0xff, 0x60})
			lleno := 0.0
			if k < a.hi {
				lleno = 1
			} else if k == a.hi {
				lleno = math.Min(1, avance)
			}
			if lleno > 0 {
				l.redondeado(image.Rect(x, 8, x+int(float64(w)*lleno), 11), 1, colTexto)
			}
		}
	}
	if m.esVideo() {
		a.barraProgreso(izq.Min.X, izq.Max.X, 190, a.rep.posicion(), a.rep.duracion())
	}
	if a.histPausa > 0 || (m.esVideo() && a.rep.enPausa()) {
		l.izquierda(image.Rect(izq.Min.X, 204, izq.Max.X, 226), "En pausa", l.f.pequena, colTexto)
	}
	a.panelBotones(der.Min.X, der.Max.X, [][2]string{{"A  DER", "Siguiente"}, {"IZQ", "Anterior"}, {"R  ABAJO", "Otra persona"},
		{"L  ARRIBA", "La de antes"}, {"Y", "Pausa"}, {"B", "Salir"}})
}

// ---- Mensajes ------------------------------------------------------------------------------------

func (a *app) dibujarMensajes() {
	l := a.l
	a.cabecera()
	if len(a.chats) == 0 {
		s, c := "Cargando tus mensajes…", color.Color(colSuave)
		switch {
		case a.chatsErr != "":
			s, c = a.chatsErr+" (A para reintentar)", colRojo
		case a.chatsHecho && !a.chatsCarga:
			s = "No tienes mensajes."
		}
		a.mensajeCentrado(image.Rect(0, 60, ancho, alto-40), s, c)
		a.ayudaAbajo("X  Actualizar      L R  Pestañas      B  Inicio")
		return
	}
	const y0, h, visibles = 56, 62, 6
	primera := max(0, a.selChat-visibles+1)
	for k := primera; k < len(a.chats) && k < primera+visibles; k++ {
		c := a.chats[k]
		y := y0 + (k-primera)*h
		r := image.Rect(12, y+2, ancho-12, y+h-2)
		if k == a.selChat {
			a.translucido(r, 14, 40)
			l.marcoRedondo(r, 14, 2, colMarco)
		}
		a.cara(46, y+h/2, 22, c.Foto, c.Titulo)
		titulo := recortar(l.f.nombre, l.f.limpiar(c.Titulo), 420)
		l.izquierda(image.Rect(80, y+8, 520, y+32), titulo, l.f.nombre, colTexto)
		col := color.Color(colApagado)
		if c.SinLeer {
			col = colTexto
		}
		l.izquierda(image.Rect(80, y+32, 560, y+54), recortar(l.f.pequena, l.f.limpiar(c.Ultimo), 470), l.f.pequena, col)
		l.derecha(image.Rect(520, y+10, ancho-26, y+30), horaCorta(c.Cuando), l.f.mini, colApagado)
		if c.SinLeer {
			l.circulo(ancho-32, y+43, 5, colBurbuja)
		}
	}
	a.ayudaAbajo("A  Abrir      ARRIBA ABAJO  Elegir      X  Actualizar      L R  Pestañas      B  Inicio")
}

// burbuja: lo que mide y cómo va un mensaje de la conversación.
type burbuja struct {
	m      Mensaje
	lineas []string
	w, h   int
	nombre bool
}

const anchoBurbuja = 380

func (a *app) burbujas() ([]burbuja, int) {
	l := a.l
	var bs []burbuja
	total := 0
	if a.chat == nil {
		return nil, 0
	}
	for _, m := range a.chat.Mensajes {
		b := burbuja{m: m, lineas: ajustar(l.f.pequena, l.f.limpiar(m.Texto), anchoBurbuja-24)}
		for _, ln := range b.lineas {
			b.w = max(b.w, medir(l.f.pequena, ln))
		}
		b.w += 24
		b.h = len(b.lineas)*20 + 14
		b.nombre = !m.Mio && m.Quien != "" && !strings.EqualFold(m.Quien, a.chat.Titulo)
		if b.nombre {
			b.h += 16
		}
		total += b.h + 6
		bs = append(bs, b)
	}
	return bs, total
}

func (a *app) dibujarChat() {
	l := a.l
	c := a.chat
	if c == nil {
		return
	}
	const arriba, abajo = 60, 432
	bs, total := a.burbujas()
	if a.subir > total-(abajo-arriba)+20 { // arriba del todo: no se sube más (y se piden los de antes)
		a.subir = max(total-(abajo-arriba)+20, 0)
		if c.Cursor != "" {
			a.cargarChat(true)
		}
	}
	y := abajo + a.subir
	for i := len(bs) - 1; i >= 0 && y > arriba-200; i-- {
		b := bs[i]
		y -= b.h + 6
		if y > abajo {
			continue
		}
		x := 20
		fondo := color.Color(color.RGBA{0x2a, 0x2c, 0x38, 0xf0})
		if b.m.Mio {
			x = ancho - 20 - b.w
			fondo = colBurbuja
		}
		yb := y
		if b.nombre {
			l.izquierda(image.Rect(x+6, y, x+300, y+16), recortar(l.f.mini, b.m.Quien, 290), l.f.mini, colApagado)
			yb += 16
		}
		l.redondeado(image.Rect(x, yb, x+b.w, y+b.h), 16, fondo)
		for k, ln := range b.lineas {
			l.izquierda(image.Rect(x+12, yb+7+k*20, x+b.w-8, yb+7+k*20+20), ln, l.f.pequena, colTexto)
		}
	}
	// Lo que se salga, tapado por la cabecera y la ayuda.
	l.fondoEn(image.Rect(0, 0, ancho, arriba-4))
	l.fondoEn(image.Rect(0, abajo+4, ancho, alto))
	a.cara(34, 30, 18, c.Foto, c.Titulo)
	l.izquierda(image.Rect(62, 14, 470, 46), recortar(l.f.nombre, l.f.limpiar(c.Titulo), 400), l.f.nombre, colTexto)
	l.rect(image.Rect(12, arriba-4, ancho-12, arriba-3), color.RGBA{0xff, 0xff, 0xff, 0x30})
	switch {
	case a.chatErr != "" && len(bs) == 0:
		a.mensajeCentrado(image.Rect(0, arriba, ancho, abajo), a.chatErr, colRojo)
	case len(bs) == 0 && a.chatCarga:
		a.mensajeCentrado(image.Rect(0, arriba, ancho, abajo), "Cargando…", colSuave)
	case len(bs) == 0:
		a.mensajeCentrado(image.Rect(0, arriba, ancho, abajo), "Aún no hay mensajes.", colSuave)
	}
	if n := len(bs); n > 0 && a.subir == 0 {
		l.derecha(image.Rect(ancho-200, abajo+6, ancho-22, abajo+22), horaCorta(bs[n-1].m.Cuando), l.f.mini, colApagado)
	}
	a.ayudaAbajo("A  Escribir      ARRIBA ABAJO  Mover      X  Actualizar      B  Volver")
}

func (a *app) dibujarEscribir() {
	l := a.l
	titulo := ""
	if a.chat != nil {
		titulo = l.f.limpiar(a.chat.Titulo)
	}
	l.izquierda(image.Rect(24, 14, 470, 46), recortar(l.f.nombre, "Para "+titulo, 440), l.f.nombre, colTexto)
	caja := image.Rect(24, 64, ancho-24, 116)
	a.translucido(caja, 14, 50)
	t := a.tec
	texto := string(t.borrador[:t.cursor]) + "|" + string(t.borrador[t.cursor:])
	col := color.Color(colTexto)
	switch {
	case a.enviando:
		texto, col = "Enviando…", colSuave
	case len(t.borrador) == 0:
		texto, col = "|  Escribe un mensaje", colApagado
	}
	if medir(l.f.texto, texto) > caja.Dx()-28 { // se ve el final (donde se escribe)
		r := []rune(texto)
		for len(r) > 1 && medir(l.f.texto, "…"+string(r)) > caja.Dx()-28 {
			r = r[1:]
		}
		texto = "…" + string(r)
	}
	l.izquierda(caja.Inset(14), texto, l.f.texto, col)
	y0 := 134
	for f := 0; f < 4; f++ {
		fila := []rune(capas[t.capa][f])
		for c, r := range fila {
			k := image.Rect(14+c*61, y0+f*58, 14+c*61+55, y0+f*58+50)
			if t.fila == f && t.col == c {
				l.redondeado(k, 12, colTexto)
				l.centrado(k, string(r), l.f.nombre, colFondo)
			} else {
				a.translucido(k, 12, 38)
				l.centrado(k, string(r), l.f.nombre, colTexto)
			}
		}
	}
	anchos := []int{110, 254, 120, 126}
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Enviar"}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*58, x+w-6, y0+4*58+50)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 12, colBurbuja)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 12, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir      B  Borrar      Y  Espacio      START  Enviar      SELECT  Volver")
}

// ---- Tu cuenta -----------------------------------------------------------------------------------

func (a *app) dibujarCuenta() {
	l := a.l
	a.cabecera()
	c := a.ig.cuenta
	if c.tiene() && !a.cambiandoCta {
		c.mu.Lock()
		usuario, nombre, foto := c.Usuario, c.Nombre, c.Foto
		c.mu.Unlock()
		r := image.Rect(24, 74, ancho-24, 214)
		a.translucido(r, 18, 40)
		a.anilloIG(r.Min.X+70, r.Min.Y+70, 51, 4)
		a.cara(r.Min.X+70, r.Min.Y+70, 44, foto, usuario)
		if nombre == "" {
			nombre = usuario
		}
		if nombre == "" {
			nombre = "Tu cuenta"
		}
		arroba := "@" + usuario
		if usuario == "" {
			arroba = "Instagram"
		}
		l.izquierda(image.Rect(r.Min.X+136, r.Min.Y+34, r.Max.X-20, r.Min.Y+66), recortar(l.f.grande, l.f.limpiar(nombre), r.Dx()-160), l.f.grande, colTexto)
		l.izquierda(image.Rect(r.Min.X+136, r.Min.Y+70, r.Max.X-20, r.Min.Y+94), arroba, l.f.pequena, colSuave)
		estado, ce := "Conectada: tu inicio, tus historias, los reels y tus mensajes salen en las pestañas.", color.Color(colVerde)
		if a.cuentaMal {
			estado, ce = "La sesión ha caducado: pulsa A y pásala otra vez desde el PC.", colRojo
		}
		y := 240
		for _, ln := range ajustar(l.f.normal, estado, ancho-60) {
			l.izquierda(image.Rect(30, y, ancho-30, y+24), ln, l.f.normal, ce)
			y += 24
		}
		y += 10
		for _, ln := range ajustar(l.f.pequena, "Tu sesión se queda solo en la consola. Se usa para ver lo tuyo, para tus «me gusta» y para contestar "+
			"mensajes. Al ver historias o abrir chats aquí, Instagram no lo marca como visto.", ancho-60) {
			l.izquierda(image.Rect(30, y, ancho-30, y+22), ln, l.f.pequena, colApagado)
			y += 21
		}
		a.ayudaAbajo("A  Cambiar de cuenta      X  Cerrar sesión      L R  Pestañas      B  Inicio")
		return
	}
	ip := miIP()
	x, y := 28, 72
	for _, ln := range ajustar(l.f.texto, "Para ver tu Instagram, abre esta página en el PC (en el Wi-Fi de casa) y sigue los pasos:", 330) {
		l.arriba(x, y, ln, l.f.texto, colTexto)
		y += 26
	}
	dir := "Conecta el Wi-Fi"
	if ip != "" {
		dir = fmt.Sprintf("http://%s:%d", ip, puertoSesion)
	}
	y += 14
	r := image.Rect(x-6, y, x+340, y+50)
	a.translucido(r, 14, 40)
	l.centrado(r, dir, l.f.nombre, colAzul)
	y += 74
	for _, ln := range ajustar(l.f.pequena, a.sesionEstado, 330) {
		l.arriba(x, y, ln, l.f.pequena, colSuave)
		y += 22
	}
	if ip != "" {
		if q, err := qrcode.New(dir, qrcode.Medium); err == nil {
			q.DisableBorder = true
			img := q.Image(220)
			fondo := image.Rect(390, 80, 390+240, 80+240)
			l.redondeado(fondo, 16, colTexto)
			b := img.Bounds()
			for yy := 0; yy < b.Dy(); yy++ {
				for xx := 0; xx < b.Dx(); xx++ {
					if rr, _, _, _ := img.At(xx, yy).RGBA(); rr < 0x8000 {
						l.img.Set(fondo.Min.X+10+xx, fondo.Min.Y+10+yy, color.Black)
					}
				}
			}
		}
	}
	if c.tiene() {
		a.ayudaAbajo("B  Dejar la cuenta de ahora      L R  Pestañas")
	} else {
		a.ayudaAbajo("B  Salir")
	}
}

func (a *app) dibujarConfirmar() {
	l := a.l
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x90})
	r := image.Rect(70, 150, ancho-70, 330)
	l.redondeado(r, 18, color.RGBA{0x14, 0x1a, 0x2a, 0xf4})
	y := 180
	for _, ln := range ajustar(l.f.texto, a.confirmTexto, r.Dx()-60) {
		l.centrado(image.Rect(r.Min.X+20, y, r.Max.X-20, y+26), ln, l.f.texto, colTexto)
		y += 28
	}
	l.centrado(image.Rect(r.Min.X, r.Max.Y-48, r.Max.X, r.Max.Y-16), "A  Sí      B  No", l.f.pequena, colAyuda)
}
