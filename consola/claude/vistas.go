package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"
)

// Pantallas con el estilo de Panther OS (fondo de las listas, letra Selawik) y los colores de Claude:
// el naranja tostado del logo, burbujas azules las tuyas y oscuras las suyas.
var (
	colClaude   = color.RGBA{0xd9, 0x77, 0x57, 0xff}
	colClaudeOs = color.RGBA{0xb8, 0x5c, 0x3e, 0xff}
	colMia      = color.RGBA{0x0e, 0x74, 0xd6, 0xff}
	colSuya     = color.RGBA{0x1c, 0x22, 0x30, 0xea}
	colCrema    = color.RGBA{0xf5, 0xef, 0xe6, 0xff}
)

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pChat:
		a.dibujarChat()
	case pEscribir:
		a.dibujarEscribir()
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

// mezclar pinta un punto con «c» a una cobertura de 0 a 1 (la imagen es RGBA premultiplicada).
func mezclar(im *image.RGBA, x, y int, c color.RGBA, k float64) {
	if k <= 0 || !(image.Point{x, y}.In(im.Rect)) {
		return
	}
	if k > 1 {
		k = 1
	}
	k *= float64(c.A) / 255
	i := im.PixOffset(x, y)
	p := im.Pix[i : i+4 : i+4]
	p[0] = uint8(float64(p[0])*(1-k) + float64(c.R)*k + 0.5)
	p[1] = uint8(float64(p[1])*(1-k) + float64(c.G)*k + 0.5)
	p[2] = uint8(float64(p[2])*(1-k) + float64(c.B)*k + 0.5)
	p[3] = uint8(float64(p[3])*(1-k) + 255*k + 0.5)
}

// chispa: el dibujo de Claude, rayos que salen del centro (de largos un poco distintos).
func (l *lienzo) chispa(cx, cy, r int, c color.RGBA) {
	largos := [...]float64{1, 0.84, 0.96, 0.8, 0.99, 0.88, 0.82, 0.97, 0.86, 0.93, 0.8, 0.95}
	n := len(largos)
	paso := 2 * math.Pi / float64(n)
	R := float64(r)
	for y := cy - r - 1; y <= cy+r+1; y++ {
		for x := cx - r - 1; x <= cx+r+1; x++ {
			dx, dy := float64(x-cx)+0.5, float64(y-cy)+0.5
			d := math.Hypot(dx, dy)
			ang := math.Atan2(dy, dx) - math.Pi/2
			for ang < 0 {
				ang += 2 * math.Pi
			}
			idx := int(math.Floor(ang/paso+0.5)) % n
			delta := math.Abs(ang - float64(idx)*paso)
			if delta > math.Pi {
				delta = 2*math.Pi - delta
			}
			largo := R * largos[idx]
			medio := R*0.1*(1-0.7*d/largo) + 0.45 // el rayo se afina hacia la punta
			k := medio - delta*d + 0.5
			if d > largo-medio { // punta redondeada
				k = math.Min(k, medio-(d-(largo-medio))+0.5)
			}
			if d < R*0.17 {
				k = 1
			}
			mezclar(l.img, x, y, c, k)
		}
	}
}

// taparFondo vuelve a poner el fondo en «r» (para tapar lo que se sale de la vista).
func (l *lienzo) taparFondo(r image.Rectangle) {
	if l.fondo != nil {
		draw.Draw(l.img, r, l.fondo, r.Min, draw.Src)
	} else {
		draw.Draw(l.img, r, &image.Uniform{colFondo}, image.Point{}, draw.Src)
	}
}

// logo: cuadrado redondeado naranja con la chispa en crema.
func (a *app) logo(x, y, tam int) {
	a.l.redondeado(image.Rect(x, y, x+tam, y+tam), tam/4, colClaude)
	a.l.chispa(x+tam/2, y+tam/2, tam*36/100, colCrema)
}

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Claude", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-118, (ancho+w)/2, alto-86)
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xe0})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	s = recortar(l.f.pequena, s, ancho-16)
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-18+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

// puntos: «...» que se van llenando, para cuando Claude escribe.
func puntos() string {
	return strings.Repeat("·", int(time.Now().UnixMilli()/400%3)+1)
}

// cabecera: logo, «Claude» y cómo está (deja libre arriba a la derecha, donde va el recuadro de la
// batería y la hora).
func (a *app) cabecera() {
	l := a.l
	a.logo(22, 14, 44)
	l.izquierda(image.Rect(78, 10, 420, 42), "Claude", l.f.grande, colTexto)
	var punto color.RGBA
	var texto string
	switch a.estado {
	case estListo:
		punto, texto = colVerde, "Conectado al PC"
	case estEnviado:
		punto, texto = colVerde, "Enviando"+puntos()
	case estEscribiendo:
		punto, texto = colClaude, "Escribiendo"+puntos()
	default:
		punto, texto = colApagado, "No escucha ahora (el PC o la conversación están cerrados)"
	}
	l.circulo(84, 54, 4, punto)
	l.izquierda(image.Rect(94, 44, ancho-20, 64), recortar(l.f.mini, texto, ancho-120), l.f.mini, colSuave)
}

// lineasDe: el texto en líneas de como mucho «w» píxeles (respetando los saltos de línea).
func (a *app) lineasDe(s string, w int) []string {
	var out []string
	for _, p := range strings.Split(s, "\n") {
		p = strings.TrimRight(p, " ")
		if p == "" {
			out = append(out, "")
			continue
		}
		out = append(out, ajustar(a.l.f.texto, a.f.limpiar(p), w)...)
	}
	return out
}

func (a *app) dibujarChat() {
	l := a.l
	vista := image.Rect(0, 74, ancho, alto-80)
	cara := l.f.texto
	alt := altoLinea(cara)
	maxW := ancho * 70 / 100

	// de abajo arriba: cada burbuja con su alto
	type burbuja struct {
		m      mensaje
		lineas []string
		w, h   int
	}
	var bs []burbuja
	total := 0
	for i := len(a.ch.msgs) - 1; i >= 0; i-- {
		m := a.ch.msgs[i]
		ls := a.lineasDe(m.Texto, maxW-28)
		w := 0
		for _, s := range ls {
			w = max(w, medir(cara, s))
		}
		b := burbuja{m: m, lineas: ls, w: w + 28, h: len(ls)*alt + 18}
		bs = append(bs, b)
		total += b.h + 10
		if total > vista.Dy()+a.scroll+600 {
			break
		}
	}
	a.maxScr = max(0, total-vista.Dy())
	if len(bs) == len(a.ch.msgs) {
		a.maxScr = max(0, total-vista.Dy()+6)
	}
	a.scroll = min(a.scroll, a.maxScr)

	if len(a.ch.msgs) == 0 {
		a.logo(ancho/2-34, 130, 68)
		l.centrado(image.Rect(0, 214, ancho, 244), "Habla con Claude desde la consola", l.f.nombre, colTexto)
		for i, s := range []string{
			"Lo que escribas le llega a la conversación de Claude que hay",
			"abierta en el PC de casa, y su respuesta sale aquí.",
			"Pulsa A para escribir.",
		} {
			l.centrado(image.Rect(20, 254+i*26, ancho-20, 278+i*26), s, l.f.pequena, colSuave)
		}
	}

	// recorte: solo se pinta lo que cae en la vista
	y := vista.Max.Y + a.scroll
	for _, b := range bs {
		y -= b.h + 10
		if y > vista.Max.Y {
			continue
		}
		if y+b.h < vista.Min.Y {
			break
		}
		x := 22
		fondo := colSuya
		if b.m.Mio {
			x = ancho - 22 - b.w
			fondo = colMia
		}
		r := image.Rect(x, y, x+b.w, y+b.h)
		l.redondeado(r, 16, fondo)
		for i, s := range b.lineas {
			ly := y + 9 + i*alt
			if ly < vista.Min.Y-alt || ly > vista.Max.Y {
				continue
			}
			l.arriba(x+14, ly, s, cara, colTexto)
		}
	}
	// lo que asoma por encima de la vista se tapa con el fondo de la cabecera
	l.taparFondo(image.Rect(0, 0, ancho, vista.Min.Y))
	l.taparFondo(image.Rect(0, vista.Max.Y, ancho, alto))
	a.cabecera()

	// caja de escribir
	caja := image.Rect(22, alto-72, ancho-22, alto-34)
	a.translucido(caja, 19, 46)
	borrador := a.tec.texto()
	if borrador == "" {
		l.izquierda(caja.Inset(16), "Pulsa A para escribir a Claude", l.f.pequena, colApagado)
	} else {
		l.izquierda(caja.Inset(16), recortar(l.f.pequena, borrador, caja.Dx()-32), l.f.pequena, colTexto)
	}
	if a.maxScr > 0 && a.scroll < a.maxScr {
		a.ayudaAbajo("A  Escribir      ARRIBA ABAJO  Leer      B  Salir")
	} else {
		a.ayudaAbajo("A  Escribir      B  Salir      SELECT  Apagar la pantalla")
	}
}

func (a *app) dibujarEscribir() {
	l := a.l
	a.logo(22, 14, 44)
	l.izquierda(image.Rect(78, 10, 420, 42), "Escribir a Claude", l.f.grande, colTexto)
	l.izquierda(image.Rect(80, 44, ancho-20, 64), "START envía · SELECT vuelve sin enviar", l.f.mini, colSuave)

	caja := image.Rect(22, 74, ancho-22, 132)
	a.translucido(caja, 16, 50)
	t := a.tec
	texto := string(t.borrador[:t.cursor]) + "|" + string(t.borrador[t.cursor:])
	col := color.Color(colTexto)
	if len(t.borrador) == 0 {
		texto, col = "|  Escribe tu mensaje", colApagado
	}
	// las dos últimas líneas, con el cursor
	ls := ajustar(l.f.pequena, texto, caja.Dx()-28)
	if len(ls) > 2 {
		ls = ls[len(ls)-2:]
	}
	for i, s := range ls {
		l.izquierda(image.Rect(caja.Min.X+14, caja.Min.Y+4+i*25, caja.Max.X-14, caja.Min.Y+30+i*25), s, l.f.pequena, col)
	}

	y0 := 146
	for f := 0; f < 4; f++ {
		fila := []rune(capas[t.capa][f])
		for c, r := range fila {
			k := image.Rect(14+c*61, y0+f*56, 14+c*61+55, y0+f*56+48)
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
		k := image.Rect(x, y0+4*56, x+w-6, y0+4*56+48)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 12, colClaude)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 12, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir    B  Borrar    Y  Espacio    X  Letras    START  Enviar")
}
