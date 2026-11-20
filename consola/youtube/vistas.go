package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

// Las pantallas siguen el estilo de Spotin y de las listas del menú: el fondo de las listas, letra
// Selawik, el borde azul en lo elegido. El rojo es el de los vídeos.
var (
	colRojoYT   = color.RGBA{0xff, 0x33, 0x33, 0xff}
	colBarraVid = color.RGBA{0xff, 0x40, 0x40, 0xff}
)

func (a *app) dibujar() {
	l := a.l
	if a.pant == pViendo || (a.pant == pConfirmar && a.confirmVolver == pViendo) {
		l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0xff})
	} else {
		l.limpiarFondo()
	}
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pInicio:
		a.dibujarInicio()
	case pBuscar:
		a.dibujarBuscar()
	case pResultados:
		a.dibujarResultados()
	case pViendo:
		a.dibujarViendo()
	case pCuenta:
		a.dibujarCuenta()
	case pConfirmar:
		switch a.confirmVolver {
		case pInicio:
			a.dibujarInicio()
		case pResultados:
			a.dibujarResultados()
		case pCuenta:
			a.dibujarCuenta()
		}
		a.dibujarConfirmar()
	}
	if !hayRed() && a.pant != pViendo {
		a.etiqueta(image.Pt(ancho/2, 14), "Sin internet · solo lo descargado")
	}
	corto := a.pant == pViendo && a.enCorto()
	if a.avisoTexto != "" {
		a.dibujarAviso(corto)
	}
	if !a.volHasta.IsZero() && a.volumen >= 0 && !corto {
		a.dibujarVolumen()
	}
}

// logo: el botón rojo con el triángulo de «reproducir».
func (a *app) logo(x, y, w int) {
	l := a.l
	h := w * 7 / 10
	l.redondeado(image.Rect(x, y, x+w, y+h), h/4, colRojoYT)
	cx, cy, t := x+w/2+w/40, y+h/2, h*3/10
	for dy := -t; dy <= t; dy++ { // triángulo blanco
		largo := (t - abs(dy)) * 17 / 10
		l.rect(image.Rect(cx-t*8/10, cy+dy, cx-t*8/10+largo, cy+dy+1), colTexto)
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (a *app) cabecera(titulo string) {
	l := a.l
	a.logo(26, 28, 38)
	l.izquierda(image.Rect(76, 22, 440, 60), titulo, l.f.grande, colTexto)
	a.hora(false)
}

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-60, alto/2-110, 120)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "YouTube", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

func (a *app) dibujarVolumen() {
	l := a.l
	y := 22
	if a.pant == pViendo {
		y = 8
	}
	r := image.Rect(ancho/2-150, y, ancho/2+150, y+44)
	l.redondeado(r, 22, color.RGBA{0, 0, 0, 0xd8})
	l.centrado(image.Rect(r.Min.X+10, r.Min.Y, r.Min.X+46, r.Max.Y), icAltavoz, l.f.icono, colTexto)
	x0, x1 := r.Min.X+52, r.Max.X-62
	l.redondeado(image.Rect(x0, y+19, x1, y+25), 3, color.RGBA{70, 70, 70, 70})
	l.redondeado(image.Rect(x0, y+19, x0+(x1-x0)*a.volumen/100, y+25), 3, colBarraVid)
	l.derecha(image.Rect(x1+6, r.Min.Y, r.Max.X-14, r.Max.Y), fmt.Sprintf("%d %%", a.volumen), l.f.pequena, colTexto)
}

// hora: ya no se pinta: la hora sale en el recuadro negro de arriba a la derecha (el del servicio
// de WhatsApp, con la batería), y aquí estaba repetida.
func (a *app) hora(grande bool) {}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

func (a *app) etiqueta(centro image.Point, s string) {
	l := a.l
	w := medir(l.f.mini, s) + 24
	r := image.Rect(centro.X-w/2, centro.Y-11, centro.X+w/2, centro.Y+13)
	l.redondeado(r, 12, color.RGBA{0x3a, 0x17, 0x20, 0xe6})
	l.centrado(r, s, l.f.mini, color.RGBA{0xff, 0xc2, 0xcb, 0xff})
}

func (a *app) dibujarAviso(corto bool) {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-84, (ancho+w)/2, alto-52)
	switch {
	case corto: // en los Shorts, abajo a la izquierda (al lado del vídeo)
		izq := areaVideo(anchoVert, altoVert).Min.X - 20
		lineas := ajustar(l.f.mini, a.avisoTexto, izq-34)
		r = image.Rect(10, alto-80-len(lineas)*18, izq, alto-64)
		l.redondeado(r, 12, color.RGBA{0, 0, 0, 0xe0})
		y := r.Min.Y + 8
		for _, ln := range lineas {
			l.izquierda(image.Rect(r.Min.X+12, y, r.Max.X-8, y+18), ln, l.f.mini, colTexto)
			y += 18
		}
		return
	case a.pant == pViendo:
		r = image.Rect((ancho-w)/2, 14, (ancho+w)/2, 46)
	}
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xe0})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-22+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// miniatura: la del vídeo en r (o un hueco gris mientras llega), con la duración encima.
func (a *app) miniatura(v Video, r image.Rectangle, conDuracion bool) {
	l := a.l
	if im := a.mini.pedir(v, r.Dx(), r.Dy()); im != nil {
		l.pegar(im, nil, r.Min.X, r.Min.Y)
	} else {
		a.translucido(r, max(6, r.Dy()/9), 34)
		l.centrado(r, icPelicula, l.f.icono, color.RGBA{0xff, 0xff, 0xff, 0x50})
	}
	if !conDuracion {
		return
	}
	s := fmtDur(v.duracion())
	fondo := color.Color(color.RGBA{0, 0, 0, 0xcc})
	switch {
	case v.Corto:
		s, fondo = "SHORT", colRojoYT
	case v.Directo:
		s, fondo = "EN DIRECTO", colRojoYT
	}
	w := medir(l.f.mini, s) + 10
	e := image.Rect(r.Max.X-w-5, r.Max.Y-22, r.Max.X-5, r.Max.Y-5)
	l.redondeado(e, 4, fondo)
	l.centrado(e, s, l.f.mini, colTexto)
}

// barraVista: lo que ya has visto de un vídeo (debajo de su miniatura).
func (a *app) barraVista(r image.Rectangle, pos, total int) {
	if pos <= 0 || total <= 0 {
		return
	}
	y := r.Max.Y - 4
	a.l.rect(image.Rect(r.Min.X+6, y, r.Max.X-6, y+3), color.RGBA{0x80, 0x80, 0x80, 0xc0})
	a.l.rect(image.Rect(r.Min.X+6, y, r.Min.X+6+(r.Dx()-12)*min(pos, total)/total, y+3), colBarraVid)
}

// ---- Inicio -------------------------------------------------------------------------------------

const (
	anchoTarjeta = 156
	altoTarjeta  = 88
	pasoTarjeta  = 172
	altoFila     = 152
)

func (a *app) dibujarInicio() {
	l := a.l
	a.cabecera("YouTube")
	filas := a.filasInicio()
	k := a.focoActual(filas)
	enBuscar := k < 0 && a.pant == pInicio
	// Buscar y, a la derecha, tu cuenta.
	caja := image.Rect(24, 72, ancho-24-124, 112)
	alfa := uint8(40)
	if enBuscar && !a.focoCuenta {
		alfa = 64
	}
	a.translucido(caja, 20, alfa)
	if enBuscar && !a.focoCuenta {
		l.marcoRedondo(caja.Inset(-3), 23, 3, colMarco)
	}
	l.centrado(image.Rect(caja.Min.X+12, caja.Min.Y, caja.Min.X+44, caja.Max.Y), icBuscar, l.f.icono, colSuave)
	l.izquierda(image.Rect(caja.Min.X+50, caja.Min.Y, caja.Max.X-16, caja.Max.Y), "Buscar en YouTube", l.f.texto, colApagado)
	bc := image.Rect(ancho-24-112, 72, ancho-24, 112)
	alfa = 40
	if enBuscar && a.focoCuenta {
		alfa = 64
	}
	a.translucido(bc, 20, alfa)
	if enBuscar && a.focoCuenta {
		l.marcoRedondo(bc.Inset(-3), 23, 3, colMarco)
	}
	nombre, colIc := "Cuenta", color.Color(colSuave)
	if a.yt.cuenta.tiene() {
		if n := strings.Fields(a.f.limpiar(a.yt.cuenta.nombre())); len(n) > 0 {
			nombre = n[0]
		}
		colIc = colVerde
		if a.cuentaMal {
			colIc = colRojo
		}
	}
	l.centrado(image.Rect(bc.Min.X+8, bc.Min.Y, bc.Min.X+36, bc.Max.Y), icUsuario, l.f.icono, colIc)
	l.izquierda(image.Rect(bc.Min.X+38, bc.Min.Y, bc.Max.X-10, bc.Max.Y), recortar(l.f.pequena, nombre, bc.Dx()-48), l.f.pequena, colTexto)
	if a.descPend > 0 {
		a.lineaDescarga(204, 46)
	}

	if len(filas) == 0 {
		a.logo(ancho/2-40, 190, 80)
		l.centrado(image.Rect(0, 266, ancho, 296), "Busca un vídeo para empezar", l.f.texto, colSuave)
		l.centrado(image.Rect(40, 298, ancho-40, 322), "Con tu cuenta («Cuenta», arriba a la derecha) salen tus recomendaciones y tus Shorts.",
			l.f.pequena, colApagado)
		a.ayudaAbajo("A  Abrir      B  Salir      SELECT  Apagar la pantalla")
		return
	}
	// Dos filas a la vista: la elegida y la de antes (o las dos primeras).
	primera := max(0, k-1)
	if primera > 0 && primera == len(filas)-1 {
		primera--
	}
	y := 140
	for i := primera; i < len(filas) && i < primera+2; i++ {
		a.filaTarjetas(y, filas[i], i == k && a.pant == pInicio)
		y += altoFila
	}
	ayuda := "A  Abrir      B  Salir      SELECT  Apagar la pantalla"
	if k >= 0 {
		switch f := filas[k]; {
		case f.clave == "historial":
			ayuda = "A  Ver      Y  Descargar      X  Quitar      B  Salir"
		case f.clave == "descargados":
			ayuda = "A  Ver      X  Borrar      B  Salir"
		default:
			ayuda = "A  Ver      Y  Descargar      X  Ver toda la fila      B  Salir"
		}
	}
	a.ayudaAbajo(ayuda)
}

// filaTarjetas: una fila de miniaturas con su título («Para ti», «Seguir viendo»…).
func (a *app) filaTarjetas(y int, f filaInicio, activa bool) {
	l := a.l
	col := color.Color(colSuave)
	if activa {
		col = colTexto
	}
	l.texto(28, y+l.f.iconoPeq.Metrics().Ascent.Round()-2, f.icono, l.f.iconoPeq, col)
	l.izquierda(image.Rect(52, y-6, 400, y+18), f.nombre, l.f.nombre, col)
	cuenta := fmt.Sprint(len(f.videos))
	if f.feed != nil && f.feed.Cargando {
		cuenta = "cargando…"
	} else if f.feed != nil && f.feed.Token != "" {
		cuenta += "+"
	}
	l.derecha(image.Rect(400, y-6, ancho-28, y+18), cuenta, l.f.pequena, colApagado)
	sel := min(a.sel[f.clave], len(f.videos)-1)
	// La elegida, siempre a la vista: se corre la fila.
	visibles := (ancho - 24) / pasoTarjeta
	primera := 0
	if sel >= visibles {
		primera = sel - visibles + 1
	}
	y0 := y + 24
	if len(f.videos) == 0 {
		for i := 0; i <= visibles; i++ { // huecos mientras carga
			x := 24 + i*pasoTarjeta
			a.translucido(image.Rect(x, y0, x+anchoTarjeta, y0+altoTarjeta), max(6, altoTarjeta/9), 24)
		}
		return
	}
	for i := primera; i < len(f.videos) && i <= primera+visibles; i++ {
		x := 24 + (i-primera)*pasoTarjeta
		r := image.Rect(x, y0, x+anchoTarjeta, y0+altoTarjeta)
		a.miniatura(f.videos[i], r, true)
		if i < len(f.pos) {
			a.barraVista(r, f.pos[i], f.videos[i].Segundos)
		}
		if activa && i == sel {
			l.marcoRedondo(r.Inset(-3), max(6, altoTarjeta/9)+3, 3, colMarco)
		}
		tc := color.Color(colSuave)
		if activa && i == sel {
			tc = colTexto
		}
		titulo := l.f.limpiar(f.videos[i].Titulo)
		if titulo == "" {
			titulo = "Short"
		}
		l.izquierda(image.Rect(x+2, y0+altoTarjeta+4, x+anchoTarjeta, y0+altoTarjeta+24),
			recortar(l.f.pequena, titulo, anchoTarjeta-4), l.f.pequena, tc)
	}
}

func (a *app) lineaDescarga(x, y int) {
	l := a.l
	s := fmt.Sprintf("Descargando %d", a.descPend)
	if a.descActual != "" {
		s = fmt.Sprintf("Descargando (%d)  ·  %.0f %%  ·  %s", a.descPend, a.descPct, l.f.limpiar(a.descActual))
	}
	l.texto(x, y+l.f.iconoPeq.Metrics().Ascent.Round()-6, icDescarga, l.f.iconoPeq, colVerde)
	fin := ancho - 28
	if a.pant == pInicio {
		fin = ancho - 120 // al lado del título, antes de la hora
	}
	l.izquierda(image.Rect(x+22, y-10, fin, y+8), recortar(l.f.mini, s, fin-x-22), l.f.mini, colVerde)
}

// ---- Tu cuenta ----------------------------------------------------------------------------------

func (a *app) dibujarCuenta() {
	l := a.l
	a.cabecera("Tu cuenta")
	if a.yt.cuenta.tiene() && !a.cambiandoCta {
		c := a.yt.cuenta
		c.mu.Lock()
		nombre, usuario := c.Nombre, c.Usuario
		c.mu.Unlock()
		r := image.Rect(24, 90, ancho-24, 230)
		a.translucido(r, 18, 40)
		l.circulo(r.Min.X+70, r.Min.Y+70, 44, colRojoYT)
		inicial := "?"
		if rs := []rune(l.f.limpiar(nombre)); len(rs) > 0 {
			inicial = strings.ToUpper(string(rs[0]))
		}
		l.centrado(image.Rect(r.Min.X+26, r.Min.Y+26, r.Min.X+114, r.Min.Y+114), inicial, l.f.titulo, colTexto)
		l.izquierda(image.Rect(r.Min.X+136, r.Min.Y+34, r.Max.X-20, r.Min.Y+66), recortar(l.f.grande, l.f.limpiar(nombre), r.Dx()-160), l.f.grande, colTexto)
		l.izquierda(image.Rect(r.Min.X+136, r.Min.Y+70, r.Max.X-20, r.Min.Y+94), l.f.limpiar(usuario), l.f.pequena, colSuave)
		estado, ce := "Conectada: tus listas salen en la pantalla de inicio.", color.Color(colVerde)
		if a.cuentaMal {
			estado, ce = "La sesión ha caducado: pulsa A y pásala otra vez desde el PC.", colRojo
		}
		y := 262
		for _, ln := range ajustar(l.f.normal, estado, ancho-60) {
			l.izquierda(image.Rect(30, y, ancho-30, y+24), ln, l.f.normal, ce)
			y += 24
		}
		y += 10
		for _, ln := range ajustar(l.f.pequena, "Tu cuenta solo se usa para tus listas (recomendaciones, suscripciones, Shorts, «Ver más tarde» y «Me gusta»). "+
			"Los vídeos se ven y se descargan sin ella.", ancho-60) {
			l.izquierda(image.Rect(30, y, ancho-30, y+22), ln, l.f.pequena, colApagado)
			y += 21
		}
		a.ayudaAbajo("A  Cambiar de cuenta      Y  Actualizar      X  Cerrar sesión      B  Volver")
		return
	}
	ip := miIP()
	x, y := 28, 90
	for _, ln := range ajustar(l.f.texto, "Para ver tus recomendaciones, suscripciones y Shorts, abre esta página en el PC y sigue los pasos:", 330) {
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
			fondo := image.Rect(390, 96, 390+240, 96+240)
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
	a.ayudaAbajo("B  Volver      SELECT  Apagar la pantalla")
}

// ---- Buscar -------------------------------------------------------------------------------------

func (a *app) dibujarBuscar() {
	l := a.l
	a.cabecera("Buscar")
	caja := image.Rect(24, 74, ancho-24, 122)
	a.translucido(caja, 14, 50)
	t := a.tec
	texto := string(t.borrador[:t.cursor]) + "|" + string(t.borrador[t.cursor:])
	col := color.Color(colTexto)
	if len(t.borrador) == 0 {
		texto, col = "|  Vídeo, canal o canción", colApagado
	}
	l.izquierda(caja.Inset(14), texto, l.f.texto, col)
	y0 := 140
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
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Buscar"}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*58, x+w-6, y0+4*58+50)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 12, colRojoYT)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 12, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir      B  Borrar      START  Buscar      SELECT  Volver")
}

// ---- Resultados ---------------------------------------------------------------------------------

func (a *app) dibujarResultados() {
	l := a.l
	a.logo(26, 28, 38)
	titulo := "«" + a.consulta + "»"
	if a.resFeed != nil {
		titulo = a.resFeed.Nombre
		a.res = a.resFeed.Videos
	}
	l.izquierda(image.Rect(76, 18, ancho-120, 46), recortar(l.f.nombre, titulo, ancho-200), l.f.nombre, colTexto)
	sub := fmt.Sprintf("%d vídeos", len(a.res))
	switch {
	case a.buscando:
		sub = "Buscando…"
	case a.errBusca != "":
		sub = a.errBusca
	case a.cargandoMas || (a.resFeed != nil && a.resFeed.Cargando):
		sub += "  ·  cargando más…"
	}
	l.izquierda(image.Rect(76, 44, ancho-120, 64), sub, l.f.mini, colAzul)
	a.hora(false)
	y0 := 78
	if a.descPend > 0 {
		a.lineaDescarga(28, 80)
		y0 = 94
	}
	filas := 4
	n := len(a.res)
	if a.selRes < a.primeraRes {
		a.primeraRes = a.selRes
	}
	if a.selRes >= a.primeraRes+filas {
		a.primeraRes = a.selRes - filas + 1
	}
	a.primeraRes = max(0, min(a.primeraRes, n-filas))
	for k := 0; k < filas && a.primeraRes+k < n; k++ {
		i := a.primeraRes + k
		v := a.res[i]
		y := y0 + k*90
		r := image.Rect(24, y, ancho-24, y+82)
		sel := i == a.selRes
		alfa := uint8(26)
		if sel {
			alfa = 56
		}
		a.translucido(r, 14, alfa)
		if sel {
			l.marcoRedondo(r.Inset(-3), 16, 3, colMarco)
		}
		m := image.Rect(r.Min.X+6, r.Min.Y+6, r.Min.X+6+124, r.Min.Y+6+70)
		a.miniatura(v, m, true)
		a.barraVista(m, a.alm.posicion(v.ID), v.Segundos)
		x := m.Max.X + 14
		w := r.Max.X - 14 - x
		lineas := ajustar(l.f.nombre, l.f.limpiar(v.Titulo), w)
		if len(lineas) > 2 {
			lineas = lineas[:2]
			lineas[1] = recortar(l.f.nombre, lineas[1]+"…", w)
		}
		ty := r.Min.Y + 8
		for _, ln := range lineas {
			l.izquierda(image.Rect(x, ty, x+w, ty+22), ln, l.f.nombre, colTexto)
			ty += 21
		}
		var datos []string
		for _, d := range []string{v.Canal, v.Vistas, v.Fecha} {
			if d = l.f.limpiar(d); d != "" {
				datos = append(datos, d)
			}
		}
		ic := ""
		if a.alm.descargado(v.ID) != nil {
			ic = icHecho
		} else if a.descEnCola[v.ID] {
			ic = icDescarga
		}
		wDatos := w
		if ic != "" {
			wDatos -= 24
			l.centrado(image.Rect(r.Max.X-34, r.Max.Y-28, r.Max.X-10, r.Max.Y-6), ic, l.f.iconoPeq, colVerde)
		}
		l.izquierda(image.Rect(x, r.Max.Y-26, x+wDatos, r.Max.Y-8), recortar(l.f.mini, strings.Join(datos, "  ·  "), wDatos), l.f.mini, colSuave)
	}
	if n == 0 && !a.buscando {
		msg := "No ha salido nada"
		if a.errBusca != "" {
			msg = a.errBusca
		}
		l.centrado(image.Rect(24, 200, ancho-24, 240), msg, l.f.normal, colApagado)
	}
	a.ayudaAbajo("A  Ver      Y  Descargar      X  Buscar      B  Volver")
}

// ---- Viendo -------------------------------------------------------------------------------------

// dibujarViendo: lo de alrededor del vídeo. Los normales van a lo ancho, con el título arriba y la
// barra abajo; los Shorts, de pie en medio, con los datos a la izquierda y los botones a la derecha.
// Mientras carga, su miniatura en grande.
func (a *app) dibujarViendo() {
	if a.viendo == nil {
		return
	}
	v := *a.viendo
	corto := a.enCorto()
	area := areaVideo(anchoVideo, altoVideo)
	if corto {
		area = areaVideo(anchoVert, altoVert)
	}
	if !a.rep.hayImagen() {
		a.dibujarAbriendo(v, area)
	}
	if corto {
		a.alrededorCorto(v, area)
		return
	}
	// Arriba: título y canal (si no los tapa el volumen o un aviso).
	tapado := a.avisoTexto != "" || (!a.volHasta.IsZero() && a.volumen >= 0)
	if !tapado {
		a.tituloViendo(v)
	}
	a.barraViendo(v, area.Max.Y)
}

func (a *app) dibujarAbriendo(v Video, area image.Rectangle) {
	l := a.l
	if im := a.mini.grande(v, area.Dx(), area.Dy()); im != nil {
		draw.Draw(l.img, area, im, image.Point{}, draw.Src)
		l.rect(area, color.RGBA{0, 0, 0, 0x90})
	}
	s := "Abriendo el vídeo…"
	if a.intento > 0 {
		s = "Probando de otra forma…"
	}
	if seg := int(time.Since(a.cargaDesde).Seconds()); seg >= 3 {
		s += fmt.Sprintf("  %d s", seg)
	}
	w := min(300, area.Dx()-20)
	c := image.Pt((area.Min.X+area.Max.X)/2, (area.Min.Y+area.Max.Y)/2)
	r := image.Rect(c.X-w/2, c.Y-22, c.X+w/2, c.Y+22)
	l.redondeado(r, 22, color.RGBA{0, 0, 0, 0xc0})
	l.centrado(r, recortar(l.f.normal, s, w-16), l.f.normal, colTexto)
}

func (a *app) tituloViendo(v Video) {
	l := a.l
	l.izquierda(image.Rect(24, 6, ancho-110, 32), recortar(l.f.nombre, l.f.limpiar(v.Titulo), ancho-134), l.f.nombre, colTexto)
	l.izquierda(image.Rect(24, 32, ancho-110, 52), recortar(l.f.mini, a.subtitulo(v), ancho-134), l.f.mini, colSuave)
}

func (a *app) subtitulo(v Video) string {
	sub := a.l.f.limpiar(v.Canal)
	if a.alm.descargado(v.ID) != nil {
		sub += "  ·  descargado"
	} else if a.descEnCola[v.ID] {
		sub += fmt.Sprintf("  ·  descargando %.0f %%", a.descPct)
	}
	return strings.TrimPrefix(sub, "  ·  ")
}

// tiempos: por dónde va y cuánto dura.
func (a *app) tiempos(v Video) (time.Duration, time.Duration) {
	pos, dur := a.rep.posicion(), a.rep.duracion()
	if dur == 0 {
		dur = v.duracion()
	}
	if a.cargando || !a.rep.abierto() {
		pos = a.desdeVideo
	}
	return pos, dur
}

// barraViendo: abajo, el botón, los tiempos y la barra.
func (a *app) barraViendo(v Video, yb int) {
	l := a.l
	pos, dur := a.tiempos(v)
	ic := icPausa
	if a.rep.enPausa() || a.cargando {
		ic = icPlay
	}
	l.centrado(image.Rect(18, yb+4, 50, yb+32), ic, l.f.icono, colTexto)
	ts := fmtDur(pos)
	td := fmtDur(dur)
	if v.Directo || dur == 0 {
		td = "directo"
	}
	l.izquierda(image.Rect(56, yb+4, 120, yb+32), ts, l.f.pequena, colTexto)
	l.derecha(image.Rect(ancho-110, yb+4, ancho-22, yb+32), td, l.f.pequena, colSuave)
	x0, x1 := 56+medir(l.f.pequena, ts)+12, ancho-22-medir(l.f.pequena, td)-12
	a.barraProgreso(x0, x1, yb+17, pos, dur)
	ayuda := "A  Pausa      IZQ DER  10 s      L R  1 min      ARRIBA ABAJO  Volumen      Y  Descargar      B  Salir"
	l.centrado(image.Rect(0, yb+34, ancho, yb+56), recortar(l.f.mini, ayuda, ancho-20), l.f.mini, colAyuda)
}

func (a *app) barraProgreso(x0, x1, y int, pos, dur time.Duration) {
	l := a.l
	l.redondeado(image.Rect(x0, y, x1, y+5), 2, color.RGBA{0x60, 0x60, 0x60, 0xff})
	if dur > 0 {
		xp := x0 + int(float64(x1-x0)*min(1, pos.Seconds()/dur.Seconds()))
		l.redondeado(image.Rect(x0, y, xp, y+5), 2, colBarraVid)
		l.circulo(xp, y+2, 6, colBarraVid)
	}
}

// alrededorCorto: los datos del Short a la izquierda y los botones a la derecha.
func (a *app) alrededorCorto(v Video, area image.Rectangle) {
	l := a.l
	izq := image.Rect(16, 0, area.Min.X-14, alto)
	der := image.Rect(area.Max.X+14, 0, ancho-16, alto)
	// Izquierda: «Shorts», el título, el canal y las vistas; abajo, cuál es de cuántos.
	l.texto(izq.Min.X, 40, icRayo, l.f.icono, colRojoYT)
	l.izquierda(image.Rect(izq.Min.X+24, 20, izq.Max.X, 48), "Shorts", l.f.nombre, colTexto)
	y := 72
	titulo := l.f.limpiar(v.Titulo)
	if titulo == "" {
		titulo = "…"
	}
	lineas := ajustar(l.f.nombre, titulo, izq.Dx())
	if len(lineas) > 7 {
		lineas = append(lineas[:6], recortar(l.f.nombre, lineas[6]+"…", izq.Dx()))
	}
	for _, ln := range lineas {
		l.izquierda(image.Rect(izq.Min.X, y, izq.Max.X, y+22), ln, l.f.nombre, colTexto)
		y += 22
	}
	y += 8
	for _, d := range []string{a.subtitulo(v), l.f.limpiar(v.Vistas)} {
		for _, ln := range ajustar(l.f.mini, d, izq.Dx()) {
			l.izquierda(image.Rect(izq.Min.X, y, izq.Max.X, y+18), ln, l.f.mini, colSuave)
			y += 18
		}
	}
	if n := len(a.cola); n > 1 {
		l.izquierda(image.Rect(izq.Min.X, alto-44, izq.Max.X, alto-20), fmt.Sprintf("%d de %d", a.colaI+1, n), l.f.pequena, colApagado)
	}
	// Derecha: el tiempo y los botones (la hora ya sale en el recuadro de arriba).
	pos, dur := a.tiempos(v)
	ic := icPausa
	if a.rep.enPausa() || a.cargando {
		ic = icPlay
	}
	l.centrado(image.Rect(der.Min.X, 70, der.Min.X+26, 98), ic, l.f.icono, colTexto)
	l.izquierda(image.Rect(der.Min.X+32, 70, der.Max.X, 98), fmtDur(pos)+" / "+fmtDur(dur), l.f.pequena, colTexto)
	a.barraProgreso(der.Min.X+2, der.Max.X-4, 108, pos, dur)
	botones := [][2]string{{"ARRIBA", "Anterior"}, {"ABAJO", "Siguiente"}, {"A", "Pausa"}, {"IZQ DER", "5 s"},
		{"L R", "Volumen"}, {"Y", "Descargar"}, {"B", "Salir"}}
	y = alto - 26 - len(botones)*24
	for _, b := range botones {
		l.izquierda(image.Rect(der.Min.X, y, der.Min.X+70, y+22), b[0], l.f.mini, colTexto)
		l.izquierda(image.Rect(der.Min.X+72, y, der.Max.X, y+22), b[1], l.f.mini, colAyuda)
		y += 24
	}
	if !a.volHasta.IsZero() && a.volumen >= 0 { // el volumen, encima de los botones
		r := image.Rect(der.Min.X-4, 138, der.Max.X+4, 170)
		l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xd8})
		l.centrado(image.Rect(r.Min.X+4, r.Min.Y, r.Min.X+28, r.Max.Y), icAltavoz, l.f.iconoPeq, colTexto)
		l.redondeado(image.Rect(r.Min.X+32, 152, r.Max.X-44, 157), 2, color.RGBA{70, 70, 70, 255})
		l.redondeado(image.Rect(r.Min.X+32, 152, r.Min.X+32+(r.Dx()-76)*a.volumen/100, 157), 2, colBarraVid)
		l.derecha(image.Rect(r.Max.X-44, r.Min.Y, r.Max.X-8, r.Max.Y), fmt.Sprint(a.volumen), l.f.mini, colTexto)
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
