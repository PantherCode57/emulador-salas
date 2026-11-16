package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"
)

// Pantallas con el estilo de Panther OS (el de Spotin y YouTube): el fondo de las listas, letra
// Selawik y el borde azul en lo elegido. Las páginas, sobre azul oscuro para que se lean bien.
var (
	colAzulNav = color.RGBA{0x0e, 0x74, 0xd6, 0xff}
	colPagina  = color.RGBA{0x0d, 0x14, 0x22, 0xff}
	colElegido = color.RGBA{0x2a, 0x5c, 0x9a, 0xff}
	colEstrella = color.RGBA{0xff, 0xd0, 0x4a, 0xff}
)

func (a *app) dibujar() {
	l := a.l
	if a.pant == pPagina {
		l.rect(l.img.Bounds(), colPagina)
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
	case pPagina:
		a.dibujarPagina()
	}
	if !hayRed() {
		a.etiqueta(image.Pt(ancho/2, 14), "Sin internet")
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

func (a *app) globo(x, y, tam int) {
	l := a.l
	l.redondeado(image.Rect(x, y, x+tam, y+tam), tam/4, colAzulNav)
	l.centrado(image.Rect(x, y, x+tam, y+tam), icGlobo, a.f.iconoDe(float64(tam)*0.62), colTexto)
}

func (a *app) cabecera(titulo string) {
	a.globo(26, 24, 38)
	a.l.izquierda(image.Rect(76, 22, 440, 62), titulo, a.l.f.grande, colTexto)
	a.hora()
}

func (a *app) hora() {
	t := time.Now()
	if t.Year() < 2001 {
		return
	}
	a.l.derecha(image.Rect(ancho-150, 22, ancho-24, 58), t.Format("15:04"), a.l.f.hora, colTexto)
}

func (a *app) dibujarCarga() {
	l := a.l
	a.globo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Navegador", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

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

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-84, (ancho+w)/2, alto-52)
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xe0})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	s = recortar(l.f.pequena, s, ancho-16)
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-22+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

// ---- Inicio -------------------------------------------------------------------------------------

func (a *app) dibujarInicio() {
	l := a.l
	a.cabecera("Navegador")
	caja := image.Rect(24, 72, ancho-24, 112)
	alfa := uint8(40)
	if a.selInicio == 0 {
		alfa = 64
	}
	a.translucido(caja, 20, alfa)
	if a.selInicio == 0 {
		l.marcoRedondo(caja.Inset(-3), 23, 3, colMarco)
	}
	l.centrado(image.Rect(caja.Min.X+12, caja.Min.Y, caja.Min.X+44, caja.Max.Y), icBuscar, l.f.icono, colSuave)
	l.izquierda(image.Rect(caja.Min.X+50, caja.Min.Y, caja.Max.X-16, caja.Max.Y), "Buscar o escribir una dirección", l.f.texto, colApagado)

	e := a.entradasInicio()
	filas := 5
	sel := a.selInicio - 1
	primera := 0
	if sel >= filas {
		primera = sel - filas + 1
	}
	y := 130
	seccion := ""
	for i := primera; i < len(e) && i < primera+filas; i++ {
		x := e[i]
		if x.seccion != seccion {
			seccion = x.seccion
			if i > primera || i == 0 || true {
				l.izquierda(image.Rect(28, y, 400, y+20), seccion, l.f.pequena, colAzul)
				y += 22
			}
		}
		r := image.Rect(24, y, ancho-24, y+50)
		elegida := i == sel
		alfa := uint8(26)
		if elegida {
			alfa = 56
		}
		a.translucido(r, 14, alfa)
		if elegida {
			l.marcoRedondo(r.Inset(-3), 16, 3, colMarco)
		}
		ic, ci := icHistorial, color.Color(colSuave)
		switch x.seccion {
		case "Guardadas":
			ic, ci = icEstrella, colEstrella
		case "Para empezar":
			ic, ci = icGlobo, colAzul
		}
		l.centrado(image.Rect(r.Min.X+8, r.Min.Y, r.Min.X+40, r.Max.Y), ic, l.f.iconoPeq, ci)
		titulo := l.f.limpiar(x.s.Titulo)
		if titulo == "" {
			titulo = dominio(x.s.URL)
		}
		l.izquierda(image.Rect(r.Min.X+46, r.Min.Y+4, r.Max.X-14, r.Min.Y+28), titulo, l.f.nombre, colTexto)
		l.izquierda(image.Rect(r.Min.X+46, r.Min.Y+28, r.Max.X-14, r.Max.Y-4), dominio(x.s.URL), l.f.mini, colSuave)
		y += 56
	}
	ayuda := "A  Buscar      B  Salir      SELECT  Apagar la pantalla"
	if sel >= 0 && sel < len(e) {
		switch e[sel].seccion {
		case "Guardadas":
			ayuda = "A  Abrir      X  Quitar      START  Buscar      B  Salir"
		case "Últimas":
			ayuda = "A  Abrir      Y  Guardar      X  Quitar      B  Salir"
		default:
			ayuda = "A  Abrir      Y  Guardar      START  Buscar      B  Salir"
		}
	}
	a.ayudaAbajo(ayuda)
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
		texto, col = "|  Qué buscar, o una dirección (wikipedia.org)", colApagado
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
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Ir"}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*58, x+w-6, y0+4*58+50)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 12, colAzulNav)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 12, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir      B  Borrar      START  Ir      SELECT  Volver")
}

// ---- Resultados ---------------------------------------------------------------------------------

func (a *app) dibujarResultados() {
	l := a.l
	a.globo(26, 24, 38)
	l.izquierda(image.Rect(76, 18, ancho-120, 46), recortar(l.f.nombre, "«"+a.consulta+"»", ancho-200), l.f.nombre, colTexto)
	sub := fmt.Sprintf("%d resultados", len(a.res))
	switch {
	case a.buscando:
		sub = "Buscando…"
	case a.errBusca != "":
		sub = a.errBusca
	}
	l.izquierda(image.Rect(76, 44, ancho-120, 64), sub, l.f.mini, colAzul)
	a.hora()
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
		r := a.res[i]
		y := 78 + k*90
		caja := image.Rect(24, y, ancho-24, y+82)
		elegida := i == a.selRes
		alfa := uint8(26)
		if elegida {
			alfa = 56
		}
		a.translucido(caja, 14, alfa)
		if elegida {
			l.marcoRedondo(caja.Inset(-3), 16, 3, colMarco)
		}
		w := caja.Dx() - 28
		l.izquierda(image.Rect(caja.Min.X+14, y+6, caja.Max.X-14, y+28), l.f.limpiar(r.Titulo), l.f.nombre, colTexto)
		l.izquierda(image.Rect(caja.Min.X+14, y+28, caja.Max.X-14, y+44), dominio(r.URL), l.f.mini, colEnlace)
		ty := y + 44
		lineas := ajustar(l.f.mini, l.f.limpiar(r.Resumen), w)
		for j, ln := range lineas {
			if j == 2 {
				break
			}
			if j == 1 && len(lineas) > 2 {
				ln = recortar(l.f.mini, ln+"…", w)
			}
			l.izquierda(image.Rect(caja.Min.X+14, ty, caja.Max.X-14, ty+17), ln, l.f.mini, colSuave)
			ty += 17
		}
	}
	if n == 0 && !a.buscando {
		msg := "No ha salido nada"
		if a.errBusca != "" {
			msg = a.errBusca
		}
		l.centrado(image.Rect(24, 200, ancho-24, 240), msg, l.f.normal, colApagado)
	}
	a.ayudaAbajo("A  Abrir      X  Buscar otra cosa      B  Volver")
}

// ---- Página -------------------------------------------------------------------------------------

func (a *app) dibujarPagina() {
	l := a.l
	vista := image.Rect(0, topePag, ancho, topePag+altoVista())
	switch {
	case a.cargando:
		s := "Abriendo " + dominio(a.cargandoURL) + "…"
		if seg := int(time.Since(a.cargaDesde).Seconds()); seg >= 2 {
			s += fmt.Sprintf("  %d s", seg)
		}
		r := image.Rect(ancho/2-200, alto/2-24, ancho/2+200, alto/2+24)
		l.redondeado(r, 24, color.RGBA{0xff, 0xff, 0xff, 0x14})
		l.centrado(r, recortar(l.f.normal, s, 380), l.f.normal, colTexto)
	case a.errPag != "":
		y := alto/2 - 30
		for _, ln := range ajustar(l.f.normal, a.errPag, ancho-80) {
			l.centrado(image.Rect(40, y, ancho-40, y+24), ln, l.f.normal, colRojo)
			y += 24
		}
	case a.maq != nil:
		a.dibujarContenido(vista)
	}
	// Barra de arriba: título, dirección, guardada y hora (encima de lo que asome).
	l.rect(image.Rect(0, 0, ancho, topePag), color.RGBA{0x07, 0x0c, 0x16, 0xff})
	l.rect(image.Rect(0, topePag-1, ancho, topePag), color.RGBA{0xff, 0xff, 0xff, 0x22})
	titulo, dir := "", ""
	if a.pag != nil && !a.cargando {
		titulo, dir = l.f.limpiar(a.pag.titulo), dominio(a.pag.url)
	} else if a.cargando {
		dir = dominio(a.cargandoURL)
	}
	x := 20
	if a.pag != nil && !a.cargando && a.alm.guardada(a.pag.url) {
		l.centrado(image.Rect(14, 6, 40, 30), icEstrella, l.f.iconoPeq, colEstrella)
		x = 44
	}
	l.izquierda(image.Rect(x, 6, ancho-110, 30), titulo, l.f.nombre, colTexto)
	l.izquierda(image.Rect(20, 30, ancho-110, 48), dir, l.f.mini, colEnlace)
	if t := time.Now(); t.Year() >= 2001 {
		l.derecha(image.Rect(ancho-110, 6, ancho-20, 46), t.Format("15:04"), l.f.hora, colTexto)
	}
	// Abajo: los botones.
	l.rect(image.Rect(0, topePag+altoVista(), ancho, alto), color.RGBA{0x07, 0x0c, 0x16, 0xff})
	ayuda := "B  Volver      START  Buscar"
	if a.maq != nil && !a.cargando && a.errPag == "" {
		ayuda = "ARRIBA ABAJO  Leer      IZQ DER  Enlaces      A  Abrir      Y  Guardar      B  Atrás"
		if a.selOc >= 0 && a.ocVisible(a.selOc) {
			ayuda = "A  Abrir " + dominio(a.pag.enlaces[a.maq.ocurrencias[a.selOc].enlace]) + "      IZQ DER  Enlaces      B  Atrás"
		}
	}
	a.ayudaAbajo(ayuda)
}

// dibujarContenido: las líneas de la página que caen en la vista.
func (a *app) dibujarContenido(vista image.Rectangle) {
	l := a.l
	m := a.maq
	for i := range m.lineas {
		ln := &m.lineas[i]
		y := vista.Min.Y + ln.y - a.scroll
		if y+ln.alto < vista.Min.Y-30 || y > vista.Max.Y+30 {
			if y > vista.Max.Y+altoVista() {
				break
			}
			// Las imágenes de cerca (una pantalla más abajo) se van pidiendo.
			if ln.img && y > vista.Max.Y && y < vista.Max.Y+altoVista() {
				a.img.pedir(a.pag.bloques[ln.bloque].img)
			}
			continue
		}
		x0 := margen + ln.sangria
		if ln.img {
			b := a.pag.bloques[ln.bloque]
			xi := x0 + (anchoTexto()-ln.sangria-ln.imgW)/2
			r := image.Rect(xi, y, xi+ln.imgW, y+ln.imgH)
			if im, _ := a.img.pedir(b.img); im != nil {
				l.pegar(im, nil, r.Min.X, r.Min.Y)
			} else {
				a.translucido(r, 10, 20)
				l.centrado(r, icImagen, l.f.icono, color.RGBA{0xff, 0xff, 0xff, 0x40})
			}
			continue
		}
		if ln.cita {
			l.rect(image.Rect(margen+4, y, margen+7, y+ln.alto), colApagado)
		}
		if ln.vineta {
			l.texto(margen+ln.sangria-14, y+ln.alto-7, "•", l.f.normal, colSuave)
		}
		base := y + ln.alto - 6
		for _, p := range ln.piezas {
			if p.ocurrencia >= 0 && p.ocurrencia == a.selOc {
				w := medir(p.cara, p.texto)
				l.redondeado(image.Rect(x0+p.x-3, y, x0+p.x+w+3, y+ln.alto), 5, colElegido)
			}
			txt := p.texto
			if strings.TrimSpace(txt) == "" {
				continue
			}
			l.texto(x0+p.x, base, txt, p.cara, p.col)
		}
	}
	// La barrita de la derecha: por dónde vas.
	if m.alto > altoVista() {
		h := max(24, altoVista()*altoVista()/m.alto)
		y := vista.Min.Y + (altoVista()-h)*a.scroll/max(1, m.alto-altoVista())
		l.redondeado(image.Rect(ancho-7, y, ancho-3, y+h), 2, color.RGBA{0xff, 0xff, 0xff, 0x50})
	}
}
