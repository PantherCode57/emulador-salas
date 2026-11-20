package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"
)

// Pantallas con el estilo de Panther OS (fondo de las listas, letra Selawik) y el azul de la tienda.
var (
	colBoton   = color.RGBA{0x2f, 0x6f, 0xe8, 0xff} // «Poner»
	colPuesto  = color.RGBA{0x4a, 0xde, 0x80, 0xff}
	colPeligro = color.RGBA{0xdc, 0x3b, 0x4b, 0xff}
	colCaja    = color.RGBA{0x14, 0x1b, 0x2a, 0xf4}
)

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pLista:
		a.dibujarLista()
	case pFicha:
		a.dibujarFicha()
	}
	if a.dlg != nil {
		a.dibujarDialogo()
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

func (a *app) pegar(im *image.RGBA, x, y int) {
	if im == nil {
		return
	}
	draw.Draw(a.l.img, image.Rect(x, y, x+im.Rect.Dx(), y+im.Rect.Dy()), im, image.Point{}, draw.Over)
}

func (a *app) logo(x, y, tam int) { a.pegar(a.f.logoTienda(tam), x, y) }

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Panther Store", l.f.grande, colTexto)
	if a.saliendo {
		l.centrado(image.Rect(0, alto/2+48, ancho, alto/2+72), "Poniendo al día la pantalla principal…", l.f.pequena, colSuave)
	}
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-84, (ancho+w)/2, alto-52)
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xe8})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	s = recortar(l.f.pequena, s, ancho-16)
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-18+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

// cabecera: logo, título y una línea de estado (deja libre arriba a la derecha, donde va el
// recuadro de la batería y la hora).
func (a *app) cabecera(titulo, sub string) {
	l := a.l
	a.logo(22, 14, 44)
	l.izquierda(image.Rect(78, 10, 420, 42), titulo, l.f.grande, colTexto)
	l.izquierda(image.Rect(80, 44, ancho-20, 64), recortar(l.f.mini, sub, ancho-110), l.f.mini, colSuave)
}

func (a *app) estadoGeneral() string {
	t := a.t
	switch {
	case a.ops > 0:
		for _, l := range [][]*elemento{t.apps, t.juegos} {
			for _, e := range l {
				if e.op != nil && e.op.empezada {
					if e.op.quitar {
						return "Quitando " + e.nombre + "…"
					}
					return "Poniendo " + e.nombre + "…"
				}
			}
		}
		return "Empezando…"
	case t.cambios:
		return "Al salir, la pantalla principal se pone al día con lo que has cambiado"
	}
	return "Apps de Panther OS y juegos libres, con un botón"
}

// pestañas: «Apps» y «Juegos», con cuántos hay puestos.
func (a *app) pestanas(y int) {
	l := a.l
	x := 22
	for i, nombre := range []string{"Apps", "Juegos"} {
		lista := a.t.apps
		ic := icApps
		if i == 1 {
			lista, ic = a.t.juegos, icMando
		}
		puestos := 0
		for _, e := range lista {
			if e.puesto {
				puestos++
			}
		}
		texto := fmt.Sprintf("%s  %d/%d", nombre, puestos, len(lista))
		w := medir(l.f.negrita, texto) + 54
		r := image.Rect(x, y, x+w, y+32)
		fg := color.Color(colTexto)
		if a.pest == i {
			l.redondeado(r, 16, colTexto)
			fg = colFondo
		} else {
			a.translucido(r, 16, 40)
		}
		l.texto(x+16, baseCentrada(r, l.f.iconoPeq), ic, l.f.iconoPeq, fg)
		l.texto(x+40, baseCentrada(r, l.f.negrita), texto, l.f.negrita, fg)
		x += w + 10
	}
	if libre := espacioLibre(a.t.roms); libre > 0 {
		l.derecha(image.Rect(x, y, ancho-24, y+32), "Libre en la tarjeta: "+tamTexto(libre), l.f.mini, colApagado)
	}
}

func (a *app) dibujarLista() {
	l := a.l
	a.cabecera("Panther Store", a.estadoGeneral())
	a.pestanas(76)
	lista := a.lista()
	y0 := 118
	alto := 76
	paso := alto + 4
	prim := a.prim[a.pest]
	for i := prim; i < len(lista) && i < prim+filasVisibles; i++ {
		e := lista[i]
		y := y0 + (i-prim)*paso
		a.fila(e, image.Rect(22, y, ancho-26, y+alto), i == a.sel[a.pest])
	}
	if len(lista) > filasVisibles { // barra de la derecha
		total := y0 + filasVisibles*paso - 4 - y0
		h := max(24, total*filasVisibles/len(lista))
		yy := y0 + (total-h)*prim/max(1, len(lista)-filasVisibles)
		a.translucido(image.Rect(ancho-16, y0, ancho-12, y0+total), 2, 30)
		l.redondeado(image.Rect(ancho-16, yy, ancho-12, yy+h), 2, colSuave)
	}
	ayuda := "A  Poner"
	if e := a.elegido(); e != nil {
		switch {
		case e.op != nil:
			ayuda = "A  Parar"
		case e.puesto:
			ayuda = "A  Quitar"
		}
	}
	a.ayudaAbajo(ayuda + "      X  Ver más      L R  Apps / Juegos      B  Salir")
}

// fila: un elemento de la lista: icono, nombre, descripción y, a la derecha, cómo está.
func (a *app) fila(e *elemento, r image.Rectangle, elegida bool) {
	l := a.l
	if elegida {
		a.translucido(r, 14, 58)
		l.marcoRedondo(r, 14, 2, colMarco)
	} else {
		a.translucido(r, 14, 26)
	}
	ic := e.icono
	if ic == nil {
		ic = a.f.iconoDibujado(e, iconoAncho, iconoAlto)
		e.icono = ic
	}
	a.pegar(ic, r.Min.X+9, r.Min.Y+(r.Dy()-iconoAlto)/2)

	tx := r.Min.X + 9 + iconoAncho + 14
	col := r.Max.X - 120 // columna del estado
	l.izquierda(image.Rect(tx, r.Min.Y+7, col-8, r.Min.Y+29), e.nombre, l.f.nombre, colTexto)
	lineas := ajustar(l.f.pequena, e.desc, col-8-tx)
	nota, colNota := "", colApagado
	switch {
	case e.op != nil && e.op.empezada:
		nota = a.textoAvance(e.op)
		colNota = colSuave
	case e.fallo != "":
		nota, colNota = "No se pudo: "+e.fallo, colRojo
	case !e.puesto && !e.disponible:
		nota = e.motivo
	}
	if nota != "" {
		lineas = append(lineas[:1], recortar(l.f.mini, nota, col-8-tx))
	}
	for i, s := range lineas {
		if i > 1 {
			break
		}
		cara, c := l.f.pequena, color.Color(colSuave)
		if i == 1 && nota != "" {
			cara, c = l.f.mini, colNota
		} else if i == 1 && len(lineas) > 2 {
			s = recortar(cara, s+"…", col-8-tx)
		}
		l.izquierda(image.Rect(tx, r.Min.Y+31+i*20, col-8, r.Min.Y+50+i*20), s, cara, c)
	}

	// Columna del estado.
	b := image.Rect(col, r.Min.Y+12, r.Max.X-12, r.Min.Y+42)
	debajo := image.Rect(col, r.Min.Y+46, r.Max.X-12, r.Min.Y+66)
	switch {
	case e.op != nil && !e.op.empezada:
		a.translucido(b, 15, 34)
		a.iconoYTexto(b, icReloj, "En cola", colSuave)
	case e.op != nil:
		pct := fmt.Sprintf("%d %%", int(e.op.parte()*100+0.5))
		l.centrado(image.Rect(col, r.Min.Y+8, r.Max.X-12, r.Min.Y+36), pct, l.f.titulo2, colTexto)
		barra := image.Rect(col+6, r.Min.Y+44, r.Max.X-18, r.Min.Y+50)
		a.translucido(barra, 3, 50)
		lleno := barra
		lleno.Max.X = barra.Min.X + int(float64(barra.Dx())*e.op.parte())
		if lleno.Dx() >= 6 {
			c := colBoton
			if e.op.quitar {
				c = colPeligro
			}
			l.redondeado(lleno, 3, c)
		}
		fase := e.op.fase
		if fase == "Descomprimiendo" || fase == "Empezando" {
			fase = "Poniendo"
		}
		l.centrado(image.Rect(col, r.Min.Y+52, r.Max.X-12, r.Min.Y+70), fase, l.f.mini, colApagado)
		return
	case e.puesto:
		s := "Puesta"
		if e.tipo == tJuego {
			s = "Puesto"
		}
		a.iconoYTexto(b, icHecho, s, colPuesto)
	case e.disponible:
		var c color.Color = colBoton
		if !elegida {
			c = color.NRGBA{0x2f, 0x6f, 0xe8, 0xb0}
		}
		l.redondeado(b, 15, c)
		a.iconoYTexto(b, icDescarga, "Poner", colTexto)
	default:
		a.translucido(b, 15, 22)
		a.iconoYTexto(b, icAviso, "No está", colApagado)
	}
	if e.tam > 0 && (e.puesto || e.disponible) {
		l.centrado(debajo, tamTexto(e.tam), l.f.mini, colApagado)
	}
}

// iconoYTexto: un icono pequeño y un texto, centrados juntos en r.
func (a *app) iconoYTexto(r image.Rectangle, ic, s string, c color.Color) {
	l := a.l
	wi := medir(l.f.iconoPeq, ic)
	ws := medir(l.f.negrita, s)
	x := r.Min.X + (r.Dx()-wi-8-ws)/2
	l.texto(x, baseCentrada(r, l.f.iconoPeq), ic, l.f.iconoPeq, c)
	l.texto(x+wi+8, baseCentrada(r, l.f.negrita), s, l.f.negrita, c)
}

func (a *app) textoAvance(op *operacion) string {
	switch {
	case op.fase == "Bajando" && op.total > 0:
		return fmt.Sprintf("Bajando: %s de %s", tamTexto(op.hecho), tamTexto(op.total))
	case op.fase == "Descomprimiendo":
		return "Descomprimiendo…"
	case op.quitar:
		return "Quitando…"
	}
	return op.fase + "…"
}

func (a *app) dibujarFicha() {
	l := a.l
	e := a.elegido()
	if e == nil {
		return
	}
	sub := "App de Panther OS"
	if e.tipo == tJuego {
		sub = "Juego libre de PortMaster · " + e.genero
	}
	a.cabecera(e.nombre, sub)
	img := e.imagen
	if img == nil {
		img = a.f.iconoDibujado(e, imagenAncho, imagenAlto)
		e.imagen = img
	}
	a.pegar(img, 22, 78)

	// A la derecha: lo que ocupa, cómo está y el botón.
	x := 22 + imagenAncho + 22
	y := 82
	dato := func(clave, valor string) {
		if valor == "" {
			return
		}
		l.izquierda(image.Rect(x, y, x+90, y+22), clave, l.f.mini, colApagado)
		l.izquierda(image.Rect(x+92, y, ancho-24, y+22), recortar(l.f.normal, valor, ancho-24-x-92), l.f.normal, colTexto)
		y += 26
	}
	estado := "Sin poner"
	if e.puesto {
		estado = "Puesto"
		if e.tipo == tApp {
			estado = "Puesta"
		}
	}
	if e.tipo == tApp {
		dato("Tamaño", tamTexto(e.tam))
		dato("Estado", estado)
		dato("Viene de", "Panther OS")
	} else {
		dato("Tamaño", tamTexto(e.tam))
		dato("Estado", estado)
		g := e.juego
		de := g.autor
		if g.anio > 0 {
			de += fmt.Sprintf(" · %d", g.anio)
		}
		dato("Hecho por", de)
		dato("Viene de", "PortMaster")
	}
	b := image.Rect(x, 78+imagenAlto-38, ancho-24, 78+imagenAlto)
	switch {
	case e.op != nil && e.op.empezada:
		a.translucido(b, 19, 40)
		lleno := b
		lleno.Max.X = b.Min.X + int(float64(b.Dx())*e.op.parte())
		if lleno.Dx() >= 38 {
			c := colBoton
			if e.op.quitar {
				c = colPeligro
			}
			l.redondeado(lleno, 19, c)
		}
		l.centrado(b, fmt.Sprintf("%s  %d %%", strings.TrimSuffix(a.textoAvance(e.op), "…"), int(e.op.parte()*100+0.5)), l.f.negrita, colTexto)
	case e.op != nil:
		a.translucido(b, 19, 40)
		a.iconoYTexto(b, icReloj, "En cola", colSuave)
	case e.puesto:
		a.translucido(b, 19, 40)
		a.iconoYTexto(b, icPapelera, "A  Quitar", colTexto)
	case e.disponible:
		l.redondeado(b, 19, colBoton)
		a.iconoYTexto(b, icDescarga, "A  Poner", colTexto)
	default:
		a.translucido(b, 19, 26)
		a.iconoYTexto(b, icAviso, "No se puede poner", colApagado)
	}

	// Debajo, de qué va (y lo que haya salido mal).
	texto := e.larga
	lineas := ajustar(l.f.texto, texto, ancho-44)
	yy := 78 + imagenAlto + 16
	alt := altoLinea(l.f.texto) + 2
	maxLin := (alto - 44 - yy) / alt
	if e.fallo != "" || (!e.puesto && !e.disponible && e.op == nil) {
		maxLin--
	}
	for i, s := range lineas {
		if i >= maxLin {
			break
		}
		if i == maxLin-1 && len(lineas) > maxLin {
			s = recortar(l.f.texto, s+"…", ancho-44)
		}
		l.arriba(22, yy+i*alt, s, l.f.texto, colSuave)
	}
	switch {
	case e.fallo != "":
		l.arriba(22, yy+min(len(lineas), maxLin)*alt+4, recortar(l.f.pequena, "No se pudo: "+e.fallo, ancho-44), l.f.pequena, colRojo)
	case !e.puesto && !e.disponible && e.op == nil:
		l.arriba(22, yy+min(len(lineas), maxLin)*alt+4, recortar(l.f.pequena, e.motivo, ancho-44), l.f.pequena, colApagado)
	}
	accion := "A  Poner"
	switch {
	case e.op != nil:
		accion = "A  Parar"
	case e.puesto:
		accion = "A  Quitar"
	}
	a.ayudaAbajo(accion + "      ARRIBA ABAJO  Otro      B  Volver")
}

func (a *app) dibujarDialogo() {
	l := a.l
	d := a.dlg
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0xa8})
	w := 520
	lineas := ajustar(l.f.pequena, d.texto, w-56)
	h := 104 + len(lineas)*22 + 50
	r := image.Rect((ancho-w)/2, (alto-h)/2, (ancho+w)/2, (alto+h)/2)
	l.redondeado(r, 20, colCaja)
	l.marcoRedondo(r, 20, 1, color.RGBA{0x40, 0x48, 0x5c, 0xff})
	l.izquierda(image.Rect(r.Min.X+28, r.Min.Y+22, r.Max.X-28, r.Min.Y+50), d.titulo, l.f.titulo2, colTexto)
	for i, s := range lineas {
		l.arriba(r.Min.X+28, r.Min.Y+64+i*22, s, l.f.pequena, colSuave)
	}
	by := r.Max.Y - 62
	si := image.Rect(r.Max.X-28-150, by, r.Max.X-28, by+38)
	no := image.Rect(si.Min.X-12-150, by, si.Min.X-12, by+38)
	c := colBoton
	if d.peligro {
		c = colPeligro
	}
	l.redondeado(si, 19, c)
	l.centrado(si, "A  "+d.si, l.f.negrita, colTexto)
	a.translucido(no, 19, 44)
	l.centrado(no, "B  Cancelar", l.f.negrita, colTexto)
}
