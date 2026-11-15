package main

import (
	"image"
	"image/color"
	"strconv"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

func itoa(n int) string { return strconv.Itoa(n) }

func (a *app) dibujar() {
	l := a.l
	l.rect(l.img.Bounds(), colFondo)
	sinSesion := a.estado == waArrancando || a.estado == waQR || a.estado == waQRCaducado || a.estado == waDesvinculado
	switch {
	case a.pant == pMenu:
		a.dibujarMenu()
	case sinSesion:
		a.dibujarQR()
	case a.pant == pChats:
		a.dibujarChats()
	case a.pant == pChat:
		a.dibujarChat(40)
	case a.pant == pTeclado:
		a.dibujarTeclado()
	case a.pant == pRapidas:
		a.dibujarRapidas()
	case a.pant == pVisor:
		a.dibujarVisor()
	}
	if a.avisoTexto != "" {
		r := image.Rect(16, 8, ancho-16, 50)
		l.redondeado(r, 12, colAviso)
		l.izquierda(r.Inset(12), a.avisoTexto, l.f.pequena, colTexto)
	}
}

// cabecera: barra de arriba con título y el punto del estado de la conexión.
func (a *app) cabecera(titulo, sub string) {
	l := a.l
	l.rect(image.Rect(0, 0, ancho, 56), colBarra)
	l.izquierda(image.Rect(18, 4, ancho-150, 32), titulo, l.f.titulo, colTexto)
	if sub != "" {
		l.izquierda(image.Rect(18, 32, ancho-150, 52), sub, l.f.mini, colSuave)
	}
	c, t := colVerde, "Conectado"
	switch a.estado {
	case waConectando, waArrancando:
		c, t = colAmarillo, "Conectando…"
	case waSinRed:
		c, t = colRojo, "Sin conexión"
	case waQR, waQRCaducado, waDesvinculado:
		c, t = colSuave, "Sin vincular"
	}
	l.circulo(ancho-128, 28, 5, c)
	l.izquierda(image.Rect(ancho-116, 16, ancho-10, 40), t, l.f.mini, colSuave)
}

func (a *app) dibujarQR() {
	l := a.l
	a.cabecera("WhatsApp", "Panther OS")
	tam := 300
	caja := image.Rect(24, 78, 24+tam, 78+tam)
	textoX := caja.Max.X + 24
	anchoTexto := ancho - textoX - 20
	switch a.estado {
	case waQR:
		l.redondeado(caja.Inset(-6), 10, colBlanco)
		a.pintarQR(caja, a.qr)
		y := caja.Min.Y
		y = a.parrafo(textoX, y, anchoTexto, "Vincula la consola", l.f.negrita, colTexto)
		y = a.parrafo(textoX, y+8, anchoTexto, "1. Abre WhatsApp en el móvil.", l.f.pequena, colSuave)
		y = a.parrafo(textoX, y+6, anchoTexto, "2. Ajustes → Dispositivos vinculados.", l.f.pequena, colSuave)
		y = a.parrafo(textoX, y+6, anchoTexto, "3. Vincular un dispositivo y apunta a este código.", l.f.pequena, colSuave)
		a.parrafo(textoX, y+14, anchoTexto, "El código cambia solo cada poco.", l.f.mini, colSuave)
		l.pistas([2]string{"B", "Menú"})
	case waQRCaducado:
		l.redondeado(caja, 10, colBarra)
		l.centrado(caja, "Código caducado", l.f.negrita, colSuave)
		a.parrafo(textoX, caja.Min.Y, anchoTexto, "Han pasado varios códigos sin escanear. Pulsa A para sacar otro.", l.f.normal, colTexto)
		l.pistas([2]string{"A", "Otro código"}, [2]string{"B", "Menú"})
	case waDesvinculado:
		l.redondeado(caja, 10, colBarra)
		l.centrado(caja, "Sin vincular", l.f.negrita, colSuave)
		a.parrafo(textoX, caja.Min.Y, anchoTexto, "La consola ya no está vinculada a tu WhatsApp. Pulsa A para vincularla otra vez.", l.f.normal, colTexto)
		l.pistas([2]string{"A", "Vincular"}, [2]string{"B", "Menú"})
	default:
		l.redondeado(caja, 10, colBarra)
		l.centrado(caja, "Preparando el código…", l.f.normal, colSuave)
		l.pistas([2]string{"B", "Menú"})
	}
}

func (a *app) parrafo(x, y, anchoMax int, s string, cara fontFace, c color.Color) int {
	al := altoLinea(cara)
	for _, linea := range ajustar(cara, s, anchoMax) {
		y += al
		a.l.texto(x, y, linea, cara, c)
	}
	return y
}

// pintarQR dibuja el código con módulos de píxeles enteros, centrado en la caja.
func (a *app) pintarQR(caja image.Rectangle, codigo string) {
	q, err := qrcode.New(codigo, qrcode.Low)
	if err != nil {
		return
	}
	q.DisableBorder = true
	m := q.Bitmap()
	n := len(m)
	if n == 0 {
		return
	}
	mod := caja.Dx() / n
	x0 := caja.Min.X + (caja.Dx()-mod*n)/2
	y0 := caja.Min.Y + (caja.Dy()-mod*n)/2
	for y, fila := range m {
		for x, negro := range fila {
			if negro {
				a.l.rect(image.Rect(x0+x*mod, y0+y*mod, x0+(x+1)*mod, y0+(y+1)*mod), colNegro)
			}
		}
	}
}

func hora(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	ahora := time.Now()
	switch {
	case t.YearDay() == ahora.YearDay() && t.Year() == ahora.Year():
		return t.Format("15:04")
	case ahora.Sub(t) < 6*24*time.Hour:
		return [...]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}[t.Weekday()]
	}
	return t.Format("02/01/06")
}

func (a *app) dibujarChats() {
	l := a.l
	lista := a.w.ordenados()
	sub := itoa(len(lista)) + " chats"
	if len(lista) == 1 {
		sub = "1 chat"
	}
	a.cabecera("WhatsApp", sub)
	if len(lista) == 0 {
		l.centrado(image.Rect(0, 60, ancho, alto-40), "Todavía no hay chats. Llegarán en un momento.", l.f.normal, colSuave)
		l.pistas([2]string{"START", "Menú"})
		return
	}
	a.selChat = min(a.selChat, len(lista)-1)
	if a.selChat < a.primera {
		a.primera = a.selChat
	}
	if a.selChat >= a.primera+filasVisibles {
		a.primera = a.selChat - filasVisibles + 1
	}
	const altoFila = 62
	for i := 0; i < filasVisibles && a.primera+i < len(lista); i++ {
		c := lista[a.primera+i]
		r := image.Rect(0, 58+i*altoFila, ancho, 58+(i+1)*altoFila)
		if a.primera+i == a.selChat {
			l.rect(r, colFila)
			l.rect(image.Rect(0, r.Min.Y, 4, r.Max.Y), colVerde)
		}
		// Inicial del nombre en un círculo.
		ini := "?"
		if rs := []rune(c.Nombre); len(rs) > 0 {
			ini = string(rs[0])
		}
		l.circulo(38, r.Min.Y+31, 21, colorDe(c.JID))
		l.centrado(image.Rect(17, r.Min.Y+10, 59, r.Min.Y+52), ini, l.f.negrita, colBlanco)
		nombre := c.Nombre
		if nombre == "" {
			nombre = "…"
		}
		cH := colSuave
		if c.NoLeidos > 0 {
			cH = colVerde
		}
		l.derecha(image.Rect(ancho-110, r.Min.Y+6, ancho-16, r.Min.Y+30), hora(c.Ultimo), l.f.mini, cH)
		l.izquierda(image.Rect(72, r.Min.Y+6, ancho-116, r.Min.Y+32), nombre, l.f.negrita, colTexto)
		finTexto := ancho - 16
		if c.NoLeidos > 0 {
			n := itoa(c.NoLeidos)
			w := max(24, medir(l.f.mini, n)+12)
			b := image.Rect(ancho-16-w, r.Min.Y+34, ancho-16, r.Min.Y+56)
			l.redondeado(b, 11, colVerde)
			l.centrado(b, n, l.f.mini, colFondo)
			finTexto = b.Min.X - 8
		}
		l.izquierda(image.Rect(72, r.Min.Y+33, finTexto, r.Min.Y+57), c.vistaPrevia(), l.f.pequena, colSuave)
	}
	l.pistas([2]string{"A", "Abrir"}, [2]string{"L/R", "Página"}, [2]string{"START", "Menú"})
}

// colorDe: un color fijo para cada chat (para el círculo de la inicial).
func colorDe(s string) color.RGBA {
	paleta := []color.RGBA{{0x2f, 0x80, 0xed, 0xff}, {0x9b, 0x51, 0xe0, 0xff}, {0xeb, 0x57, 0x57, 0xff},
		{0xf2, 0x99, 0x4a, 0xff}, {0x27, 0xae, 0x60, 0xff}, {0x56, 0xcc, 0xf2, 0xff}, {0xbb, 0x6b, 0xd9, 0xff}}
	h := 0
	for _, r := range s {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return paleta[h%len(paleta)]
}

// burbuja: lo que ocupa cada mensaje ya partido en líneas.
type burbuja struct {
	m      *Mensaje
	de     string
	lineas []string
	alto   int
	// Fotos y stickers
	img        image.Image
	imgW, imgH int
	hueco      string // texto del hueco mientras no está la imagen
	sticker    bool
}

func (a *app) dibujarChat(margenAbajo int) {
	l := a.l
	c := a.abierto
	if c == nil {
		a.pant = pChats
		a.dibujarChats()
		return
	}
	sub := "Chat"
	if c.Grupo {
		sub = "Grupo"
	}
	zona := image.Rect(0, 56, ancho, alto-margenAbajo)
	anchoMax := 430
	al := altoLinea(l.f.pequena)
	var bs []burbuja
	for _, m := range c.Mensajes {
		b := burbuja{m: m}
		if c.Grupo && !m.Mio {
			b.de = m.De
		}
		texto := m.Texto
		if m.Tipo != "" && a.media != nil {
			// Foto o sticker: la imagen (si ya está) y debajo el pie de foto.
			texto = m.Pie
			b.sticker = m.Tipo == tipoSticker
			im, cargando, falta := a.media.imagenes(m, false)
			if im != nil && im.vista != nil {
				b.img = im.vista
				b.imgW, b.imgH = b.img.Bounds().Dx(), b.img.Bounds().Dy()
			} else {
				b.imgW, b.imgH = tamVista(m)
				b.imgH = b.imgH * 2 / 3
				b.hueco = "Cargando…"
				if falta && !cargando {
					b.hueco = "No disponible"
				}
			}
		}
		if texto != "" {
			b.lineas = ajustar(l.f.pequena, texto, anchoMax-24)
		}
		b.alto = len(b.lineas)*al + 26
		if b.imgH > 0 {
			b.alto += b.imgH + 6
		}
		if b.de != "" {
			b.alto += al
		}
		bs = append(bs, b)
	}
	// Se dibuja de abajo arriba; 'desplazar' sube el chat de 20 en 20 píxeles.
	y := zona.Max.Y - 8 + a.desplazar*20
	tope := y
	for i := len(bs) - 1; i >= 0; i-- {
		tope -= bs[i].alto + 8
	}
	if tope > zona.Min.Y+8 && a.desplazar > 0 { // no subir más de lo que hay
		a.desplazar = max(a.desplazar-1, 0)
		y = zona.Max.Y - 8 + a.desplazar*20
	}
	for i := len(bs) - 1; i >= 0 && y > zona.Min.Y; i-- {
		b := bs[i]
		y -= b.alto
		if y+b.alto < zona.Min.Y || y > zona.Max.Y {
			y -= 8
			continue
		}
		w := 0
		for _, ln := range b.lineas {
			w = max(w, medir(l.f.pequena, ln))
		}
		w = max(w, medir(l.f.mini, "00:00 !")+8)
		if b.de != "" {
			w = max(w, min(medir(l.f.mini, b.de), anchoMax-24))
		}
		w = max(w, b.imgW)
		w += 24
		x, fondo := 14, colSuyo
		if b.m.Mio {
			x, fondo = ancho-14-w, colMio
		}
		r := image.Rect(x, y, x+w, y+b.alto)
		if !b.sticker { // los stickers van sueltos, sin burbuja, como en el móvil
			a.recortado(zona, func() { a.l.redondeado(r, 10, fondo) })
		}
		ty := y + 4
		if b.de != "" {
			ty += al
			if ty > zona.Min.Y && ty < zona.Max.Y {
				l.texto(x+12, ty, recortar(l.f.mini, b.de, w-24), l.f.mini, colorDe(b.m.Remite))
			}
		}
		if b.imgH > 0 {
			iy := ty + 6
			if b.de == "" {
				iy = y + 8
			}
			ix := x + 12
			if b.m.Mio && b.sticker {
				ix = r.Max.X - 12 - b.imgW
			}
			hueco := image.Rect(ix, iy, ix+b.imgW, iy+b.imgH)
			a.recortado(zona, func() {
				if b.img != nil {
					a.l.pegar(b.img, ix, iy)
				} else {
					a.l.huecoFoto(hueco, b.hueco)
				}
			})
			if b.img == nil || b.hueco != "" || (a.media != nil && a.media.esMini(b.m)) {
				a.media.pedir(b.m) // se ve en pantalla: que se descargue
			}
			ty = iy + b.imgH - 2
		}
		for _, ln := range b.lineas {
			ty += al
			if ty-al > zona.Min.Y && ty < zona.Max.Y {
				l.texto(x+12, ty, ln, l.f.pequena, colTexto)
			}
		}
		pie := b.m.Hora.Format("15:04")
		cPie := colSuave
		if b.m.Error {
			pie, cPie = "no enviado", colRojo
		}
		if y+b.alto-6 < zona.Max.Y && y+b.alto-20 > zona.Min.Y {
			l.texto(r.Max.X-10-medir(l.f.mini, pie), r.Max.Y-7, pie, l.f.mini, cPie)
		}
		y -= 8
	}
	if len(bs) == 0 {
		l.centrado(zona, "Aún no hay mensajes aquí.", l.f.normal, colSuave)
	}
	// La cabecera va encima de lo que se salga por arriba.
	a.cabecera(c.Nombre, sub)
	if margenAbajo == 40 {
		if len(mediosDe(c)) > 0 {
			l.pistas([2]string{"A", "Escribir"}, [2]string{"X", "Rápidas"}, [2]string{"Y", "Fotos"}, [2]string{"B", "Volver"})
		} else {
			l.pistas([2]string{"A", "Escribir"}, [2]string{"X", "Rápidas"}, [2]string{"↑↓", "Mover"}, [2]string{"B", "Volver"})
		}
	}
}

// mediosDe: las fotos y stickers de un chat, de la más antigua a la más nueva.
func mediosDe(c *Chat) []*Mensaje {
	var r []*Mensaje
	for _, m := range c.Mensajes {
		if m.Tipo != "" {
			r = append(r, m)
		}
	}
	return r
}

// dibujarVisor: una foto o un sticker a pantalla completa, con quién lo manda y su pie.
func (a *app) dibujarVisor() {
	l := a.l
	l.rect(l.img.Bounds(), colNegro)
	if len(a.visor) == 0 || a.media == nil {
		a.pant = pChat
		return
	}
	a.visorI = max(0, min(a.visorI, len(a.visor)-1))
	m := a.visor[a.visorI]
	zona := image.Rect(0, 0, ancho, alto-64)
	im, cargando, falta := a.media.imagenes(m, true)
	if im != nil && im.completa != nil {
		img := im.completa
		if b := img.Bounds(); b.Dx() > zona.Dx() || b.Dy() > zona.Dy() || im.mini {
			img = ajustarA(img, zona.Dx(), zona.Dy())
		}
		b := img.Bounds()
		l.pegar(img, (zona.Dx()-b.Dx())/2, (zona.Dy()-b.Dy())/2)
		if im.mini && cargando {
			l.centrado(image.Rect(0, zona.Max.Y-30, ancho, zona.Max.Y), "Descargando…", l.f.mini, colSuave)
		}
	} else {
		t := "Descargando…"
		if falta && !cargando {
			t = "Esta foto ya no está disponible"
		}
		l.centrado(zona, t, l.f.normal, colSuave)
	}
	// Barra de abajo: qué es, de quién, cuándo y el pie.
	barra := image.Rect(0, alto-64, ancho, alto)
	l.rect(barra, colBarra)
	que := "Foto"
	if m.Tipo == tipoSticker {
		que = "Sticker"
	}
	quien := m.De
	if m.Mio {
		quien = "Tú"
	}
	linea := que + " " + itoa(a.visorI+1) + " de " + itoa(len(a.visor))
	for _, p := range []string{quien, hora(m.Hora)} {
		if p != "" {
			linea += "  ·  " + p
		}
	}
	l.izquierda(image.Rect(14, barra.Min.Y+4, ancho-14, barra.Min.Y+26), linea, l.f.pequena, colTexto)
	if m.Pie != "" {
		l.izquierda(image.Rect(14, barra.Min.Y+24, ancho-200, barra.Min.Y+46), m.Pie, l.f.mini, colSuave)
	}
	x := ancho - 196
	x = l.boton(x, barra.Min.Y+48, "←→", "Otra")
	l.boton(x, barra.Min.Y+48, "B", "Volver")
}

// recortado dibuja solo dentro de la zona del chat (para que las burbujas no pisen las barras).
func (a *app) recortado(zona image.Rectangle, dibujo func()) {
	completa := a.l.img
	sub, ok := completa.SubImage(zona).(*image.RGBA)
	if !ok {
		dibujo()
		return
	}
	a.l.img = sub
	dibujo()
	a.l.img = completa
}

func (a *app) dibujarTeclado() {
	l := a.l
	// Arriba, el chat en pequeño; abajo, la caja del texto y el teclado.
	altoTeclado := 5*40 + 14
	cajaTexto := image.Rect(10, alto-altoTeclado-74, ancho-10, alto-altoTeclado-8)
	a.dibujarChat(alto - cajaTexto.Min.Y + 6)
	l.rect(image.Rect(0, cajaTexto.Min.Y-6, ancho, alto), colFondo)
	l.redondeado(cajaTexto, 12, colBarra)
	// Texto con el cursor (las dos últimas líneas si no cabe).
	t := a.tec
	antes := string(t.borrador[:t.cursor])
	despues := string(t.borrador[t.cursor:])
	lineas := ajustar(l.f.pequena, antes+"|"+despues, cajaTexto.Dx()-24)
	if len(t.borrador) == 0 {
		lineas = []string{"|  Escribe un mensaje"}
	}
	if len(lineas) > 2 {
		lineas = lineas[len(lineas)-2:]
	}
	al := altoLinea(l.f.pequena)
	y := cajaTexto.Min.Y + 8
	for _, ln := range lineas {
		y += al
		c := colTexto
		if len(t.borrador) == 0 {
			c = colSuave
		}
		l.texto(cajaTexto.Min.X+12, y, ln, l.f.pequena, c)
	}
	// Teclas.
	y0 := alto - altoTeclado
	for f := 0; f < 4; f++ {
		fila := []rune(capas[t.capa][f])
		for col, r := range fila {
			k := image.Rect(6+col*63, y0+f*40, 6+col*63+59, y0+f*40+36)
			fondo, c := colTecla, colTexto
			if t.fila == f && t.col == col {
				fondo, c = colVerde, colFondo
			}
			l.redondeado(k, 8, fondo)
			l.centrado(k, string(r), l.f.negrita, c)
		}
	}
	anchos := []int{110, 254, 120, 134}
	x := 6
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Enviar"}
	for i, w := range anchos {
		k := image.Rect(x, y0+4*40, x+w-4, y0+4*40+36)
		fondo, c := colTecla, colTexto
		if i == 3 {
			fondo = colMio
		}
		if t.fila == 4 && t.col == i {
			fondo, c = colVerde, colFondo
		}
		l.redondeado(k, 8, fondo)
		l.centrado(k, nombres[i], l.f.negrita, c)
		x += w
	}
	_ = especiales
}

func (a *app) dibujarRapidas() {
	l := a.l
	nombre := ""
	if a.abierto != nil {
		nombre = a.abierto.Nombre
	}
	a.cabecera("Respuestas rápidas", "Para "+nombre)
	const altoFila = 46
	primera := max(0, a.selRapida-7)
	for i := 0; i < 8 && primera+i < len(a.rapidas); i++ {
		r := image.Rect(14, 66+i*altoFila, ancho-14, 66+i*altoFila+40)
		fondo, c := colBarra, colTexto
		if primera+i == a.selRapida {
			fondo = colMio
		}
		l.redondeado(r, 10, fondo)
		l.izquierda(r.Inset(14), a.rapidas[primera+i], l.f.normal, c)
	}
	l.pistas([2]string{"A", "Enviar"}, [2]string{"B", "Volver"})
}

func (a *app) dibujarMenu() {
	l := a.l
	a.cabecera("WhatsApp", "Menú")
	for i, o := range opcionesMenu {
		r := image.Rect(60, 110+i*66, ancho-60, 110+i*66+52)
		fondo, c := colBarra, colTexto
		if i == a.selMenu {
			fondo = colMio
		}
		texto := o
		if i == 1 && a.selMenu == 1 && a.confirmar {
			texto, fondo = "Pulsa A otra vez para desvincular", colRojo
		}
		l.redondeado(r, 12, fondo)
		l.centrado(r, texto, l.f.negrita, c)
	}
	a.parrafo(60, 330, ancho-120, "Desvincular cierra la sesión en la consola: tendrás que volver a escanear el QR.", l.f.mini, colSuave)
	l.pistas([2]string{"A", "Elegir"}, [2]string{"B", "Volver"})
}
