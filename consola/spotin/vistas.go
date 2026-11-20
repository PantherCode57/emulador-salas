package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

// Las pantallas copian las vistas previas aprobadas (vista_spotin.py): fondo de las listas del
// menú, letra Selawik, el borde azul en lo elegido y el verde para lo que suena.

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pSesion:
		a.dibujarSesion()
	case pBiblioteca:
		a.dibujarBiblioteca()
	case pLista:
		a.dibujarLista()
	case pSonando:
		a.dibujarSonando()
	case pLetra:
		a.dibujarLetra()
	case pBuscar:
		a.dibujarBuscar()
	}
	if !hayRed() && a.pant != pSesion {
		a.etiqueta(image.Pt(ancho/2, 14), "Sin internet · solo lo descargado")
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
	if !a.volHasta.IsZero() && a.volumen >= 0 {
		a.dibujarVolumen()
	}
}

// dibujarVolumen: la barra que sale al subir o bajar el volumen.
func (a *app) dibujarVolumen() {
	l := a.l
	r := image.Rect(ancho/2-150, 22, ancho/2+150, 66)
	l.redondeado(r, 22, color.RGBA{0, 0, 0, 0xd8})
	l.centrado(image.Rect(r.Min.X+10, r.Min.Y, r.Min.X+46, r.Max.Y), icAltavoz, l.f.icono, colTexto)
	x0, x1 := r.Min.X+52, r.Max.X-62
	l.redondeado(image.Rect(x0, 41, x1, 47), 3, color.RGBA{70, 70, 70, 70})
	l.redondeado(image.Rect(x0, 41, x0+(x1-x0)*a.volumen/100, 47), 3, colVerde)
	l.derecha(image.Rect(x1+6, r.Min.Y, r.Max.X-14, r.Max.Y), fmt.Sprintf("%d %%", a.volumen), l.f.pequena, colTexto)
}

func (a *app) dibujarCarga() {
	l := a.l
	p := portadaDibujada(l.f, "logo", icMusica, 0.38, 150, true)
	l.pegar(p.img, p.sb, (ancho-150)/2, alto/2-110)
	l.centrado(image.Rect(0, alto/2+56, ancho, alto/2+92), "Spotin", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

// hora: ya no se pinta: la hora sale en el recuadro negro de arriba a la derecha (el del servicio
// de WhatsApp, con la batería), y aquí estaba repetida.
func (a *app) hora(grande bool) {}

// translucido: rectángulo blanco transparente (las filas de las listas).
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
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xd0})
	l.centrado(r, s, l.f.pequena, colTexto)
}

// portadaDe: la portada de una lista (dibujada para «Tus me gusta» y «Descargadas»).
func (a *app) portadaLista(ls Lista, lado int, conSombra bool) *portada {
	switch ls.Tipo {
	case tipoMeGusta:
		return portadaDibujada(a.l.f, "megusta", icCorazon, 0.72, lado, conSombra)
	case tipoDescargadas:
		return portadaDibujada(a.l.f, "descargadas", icDescarga, 0.38, lado, conSombra)
	case tipoBusqueda:
		return portadaDibujada(a.l.f, "busqueda", icBuscar, 0.55, lado, conSombra)
	}
	if p := a.port.pedir(ls.Portada, lado, conSombra); p != nil {
		return p
	}
	return portadaDibujada(a.l.f, "lista", icMusica, 0.6, lado, conSombra)
}

func (a *app) portadaCancion(c *Cancion, lado int, conSombra bool) *portada {
	if p := a.port.pedir(c.Portada, lado, conSombra); p != nil {
		return p
	}
	return portadaDibujada(a.l.f, "cancion", icMusica, 0.6, lado, conSombra)
}

// ---- Sesión ---------------------------------------------------------------------------------------

func (a *app) dibujarSesion() {
	l := a.l
	l.texto(28, 40+(l.f.iconoGrande.Metrics().Ascent.Round()-l.f.iconoGrande.Metrics().Descent.Round())/2, icMusica, l.f.iconoGrande, colVerde)
	l.izquierda(image.Rect(62, 22, 400, 58), "Spotin", l.f.grande, colTexto)
	a.hora(false)
	ip := miIP()
	x := 28
	y := 96
	for _, ln := range ajustar(l.f.texto, "Para entrar en tu cuenta de Spotify, abre esta página en el PC (o en el móvil) y sigue los pasos:", 330) {
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
			fondo := image.Rect(390, 100, 390+240, 100+240)
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
	if a.hayGuardado() {
		l.ayuda("X  Ver lo guardado      B  Salir      SELECT  Apagar la pantalla", -1)
	} else {
		l.ayuda("B  Salir      SELECT  Apagar la pantalla", -1)
	}
}

// ---- Biblioteca -----------------------------------------------------------------------------------

func (a *app) dibujarBiblioteca() {
	l := a.l
	ls := a.listasBiblioteca()
	a.selLista = min(max(a.selLista, 0), len(ls)-1)
	const lado, ladoSel, paso, cx, cy = 118, 132, 164, 24 + 105, 246
	for rel := -2; rel <= 2; rel++ {
		i := a.selLista + rel
		if i < 0 || i >= len(ls) {
			continue
		}
		sel := rel == 0
		t := lado
		if sel {
			t = ladoSel
		}
		y := cy + rel*paso
		p := a.portadaLista(ls[i], t, true)
		l.pegar(p.img, p.sb, cx-t/2, y-t/2)
		if sel {
			l.marcoRedondo(image.Rect(cx-t/2-4, y-t/2-4, cx+t/2+4, y+t/2+4), 17, 3, colMarco)
		}
		cara, col := l.f.mini, color.Color(color.RGBA{205, 205, 215, 255})
		if sel {
			cara, col = l.f.nombre, colTexto
		}
		l.centrado(image.Rect(cx-115, y+t/2+2, cx+115, y+t/2+24), ls[i].Nombre, cara, col)
	}
	a.hora(true)
	if len(ls) == 0 {
		return
	}
	el := ls[a.selLista]
	const px0 = 290
	ancho2 := ancho - 24 - px0
	l.arriba(px0, 112, recortar(l.f.titulo, el.Nombre, ancho2), l.f.titulo, colTexto)
	cs := a.alm.cancionesDe(el.ID)
	if el.Tipo == tipoDescargadas {
		cs = a.alm.descargadas()
	}
	total := el.Total
	if total == 0 && cs != nil {
		total = len(cs)
	}
	sub := []string{map[string]string{tipoLista: "Lista", tipoAlbum: "Álbum", tipoMeGusta: "Canciones que te gustan",
		tipoDescargadas: "En la consola"}[el.Tipo]}
	if total > 0 {
		sub = append(sub, fmt.Sprintf("%d canciones", total))
	}
	if cs != nil && el.Tipo != tipoDescargadas {
		if d := a.alm.cuantasDescargadas(cs); d > 0 {
			sub = append(sub, fmt.Sprintf("%d descargadas", d))
		}
	} else if el.Dueno != "" {
		sub = append(sub, el.Dueno)
	}
	l.arriba(px0, 146, recortar(l.f.pequena, strings.Join(sub, "  ·  "), ancho2), l.f.pequena, colAzul)
	l.rect(image.Rect(px0, 172, ancho-24, 173), color.RGBA{70, 70, 70, 70})
	if cs == nil {
		msg := "Pulsa A para ver las canciones"
		if !hayRed() {
			msg = "Sin internet: esta lista aún no se ha abierto en la consola"
		}
		for k, ln := range ajustar(l.f.normal, msg, ancho2) {
			l.arriba(px0, 186+k*24, ln, l.f.normal, colApagado)
		}
	} else {
		for k := 0; k < 4 && k < len(cs); k++ {
			y := 186 + k*30
			art := recortar(l.f.mini, cs[k].artistas(), 130)
			wArt := medir(l.f.mini, art)
			l.arriba(px0, y, recortar(l.f.normal, l.f.limpiar(cs[k].Nombre), ancho2-wArt-14), l.f.normal, color.RGBA{230, 230, 238, 255})
			l.arriba(ancho-24-wArt, y+2, art, l.f.mini, color.RGBA{170, 175, 190, 255})
		}
		if len(cs) > 4 {
			l.arriba(px0, 312, fmt.Sprintf("y %d más", len(cs)-4), l.f.mini, color.RGBA{150, 150, 165, 255})
		}
	}
	if a.descPend > 0 {
		a.lineaDescarga(px0, 340)
	}
	ayuda := "A  Abrir      Y  Buscar      B  Salir"
	if a.rep.sonando() || a.buscando {
		ayuda = "A  Abrir      Y  Buscar      X  Sonando      B  Salir"
	}
	l.ayuda(ayuda, px0)
}

func (a *app) lineaDescarga(x, y int) {
	l := a.l
	s := fmt.Sprintf("%s  Descargando: quedan %d", icDescarga, a.descPend)
	_ = s
	l.texto(x, y+l.f.iconoPeq.Metrics().Ascent.Round(), icDescarga, l.f.iconoPeq, colVerde)
	txt := fmt.Sprintf("Descargando · quedan %d", a.descPend)
	if a.descActual != "" {
		txt += fmt.Sprintf(" · %s %d %%", a.descActual, int(a.descPct*100))
	}
	l.arriba(x+22, y, recortar(l.f.mini, txt, ancho-24-x-22), l.f.mini, colSuave)
}

// ---- Lista ----------------------------------------------------------------------------------------

func fmtDur(d time.Duration) string {
	s := int(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (a *app) dibujarLista() {
	l := a.l
	p := a.portadaLista(a.lista, 84, true)
	if a.lista.Tipo == tipoAlbum || a.lista.Tipo == tipoLista {
		if q := a.port.pedir(a.lista.Portada, 84, true); q != nil {
			p = q
		}
	}
	l.pegar(p.img, p.sb, 24, 20)
	l.arriba(124, 30, recortar(l.f.titulo, a.lista.Nombre, ancho-124-130), l.f.titulo, colTexto)
	var total time.Duration
	for _, c := range a.canciones {
		total += c.duracion()
	}
	sub := fmt.Sprintf("%d canciones", len(a.canciones))
	if total > 0 {
		sub += "  ·  " + strings.Replace(fmtHoras(total), ":", " h ", 1)
	}
	if d := a.alm.cuantasDescargadas(a.canciones); d > 0 && a.lista.Tipo != tipoDescargadas {
		sub += fmt.Sprintf("  ·  %d descargadas", d)
	}
	if a.cargando && len(a.canciones) == 0 {
		sub = "Cargando…"
	}
	l.arriba(124, 66, recortar(l.f.pequena, sub, ancho-124-24), l.f.pequena, colAzul)
	a.hora(false)
	if a.descPend > 0 {
		a.lineaDescarga(124, 90)
	}

	mini := a.rep.sonando() || a.buscando
	filas := 6
	if mini {
		filas = 5
	}
	n := len(a.canciones)
	if a.selCancion < a.primera {
		a.primera = a.selCancion
	}
	if a.selCancion >= a.primera+filas {
		a.primera = a.selCancion - filas + 1
	}
	a.primera = max(0, min(a.primera, n-filas))
	actual := a.actual()
	for k := 0; k < filas && a.primera+k < n; k++ {
		i := a.primera + k
		c := &a.canciones[i]
		y := 122 + k*54
		r := image.Rect(24, y, ancho-24, y+46)
		sel := i == a.selCancion
		alfa := uint8(30)
		if sel {
			alfa = 60
		}
		a.translucido(r, 14, alfa)
		if sel {
			l.marcoRedondo(r.Inset(-3), 16, 3, colMarco)
		}
		suena := actual != nil && actual.ID == c.ID && (a.rep.sonando() || a.buscando)
		colNombre := color.Color(colTexto)
		if suena {
			colNombre = colVerde
			l.centrado(image.Rect(32, y, 64, y+46), icAltavoz, l.f.iconoPeq, colVerde)
		} else {
			l.centrado(image.Rect(32, y, 64, y+46), fmt.Sprint(i+1), l.f.pequena, colSuave)
		}
		dur := fmtDur(c.duracion())
		wDur := medir(l.f.pequena, dur)
		_, desc := a.alm.descargada(c.ID)
		margen := wDur + 24
		if desc {
			margen += 22
			l.centrado(image.Rect(ancho-40-wDur-24, y, ancho-40-wDur-4, y+46), icHecho, l.f.iconoPeq, colVerde)
		}
		l.izquierda(image.Rect(72, y+4, ancho-40-margen, y+26), l.f.limpiar(c.Nombre), l.f.nombre, colNombre)
		l.izquierda(image.Rect(72, y+25, ancho-40-margen, y+42), l.f.limpiar(c.artistas()), l.f.mini, colSuave)
		l.derecha(image.Rect(ancho-40-wDur, y, ancho-40, y+46), dur, l.f.pequena, colSuave)
	}
	if n == 0 && !a.cargando {
		msg := "No hay canciones"
		if a.lista.Tipo == tipoDescargadas {
			msg = "Aún no has descargado nada: abre una lista y pulsa Y"
		}
		l.centrado(image.Rect(24, 200, ancho-24, 240), msg, l.f.normal, colApagado)
	}
	if mini {
		a.miniReproductor(396)
	}
	ayuda := "A  Reproducir      Y  Descargar todas      B  Volver"
	if a.lista.Tipo == tipoBusqueda {
		ayuda = "A  Reproducir      Y  Descargar esta      B  Volver"
	}
	if a.lista.Tipo == tipoDescargadas {
		ayuda = "A  Reproducir      B  Volver"
	}
	if mini {
		ayuda = strings.Replace(ayuda, "      B", "      X  Sonando      B", 1)
	}
	a.ayudaAbajo(ayuda)
}

func fmtHoras(d time.Duration) string {
	m := int(d.Minutes())
	if m >= 60 {
		return fmt.Sprintf("%d:%d min", m/60, m%60)
	}
	return fmt.Sprintf("%d min", m)
}

// ayudaAbajo: la línea de botones centrada, un poco más abajo (como en la vista previa de la lista).
func (a *app) ayudaAbajo(s string) {
	l := a.l
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-22+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

func (a *app) miniReproductor(y int) {
	l := a.l
	c := a.actual()
	if c == nil {
		return
	}
	l.redondeado(image.Rect(16, y, ancho-16, y+52), 14, colOscuro)
	p := a.portadaCancion(c, 40, false)
	l.pegar(p.img, p.sb, 24, y+6)
	l.izquierda(image.Rect(74, y+6, ancho-80, y+28), l.f.limpiar(c.Nombre), l.f.nombre, colTexto)
	sub := c.artistas()
	if a.buscando {
		sub = "Buscando la canción…"
	}
	l.izquierda(image.Rect(74, y+26, ancho-80, y+44), l.f.limpiar(sub), l.f.mini, colSuave)
	ic := icPausa
	if a.rep.enPausa() || !a.rep.sonando() {
		ic = icPlay
	}
	l.centrado(image.Rect(ancho-60, y, ancho-20, y+52), ic, l.f.icono, colTexto)
	x0, x1 := 74, ancho-30
	l.redondeado(image.Rect(x0, y+46, x1, y+48), 1, color.RGBA{60, 60, 60, 60})
	if d := c.duracion(); d > 0 {
		f := float64(a.rep.posicion()) / float64(d)
		l.redondeado(image.Rect(x0, y+46, x0+int(float64(x1-x0)*min(f, 1)), y+48), 1, colVerde)
	}
}

// ---- Reproductor ----------------------------------------------------------------------------------

func (a *app) dibujarSonando() {
	l := a.l
	c := a.actual()
	a.hora(false)
	if c == nil {
		l.centrado(image.Rect(0, 200, ancho, 240), "No suena nada", l.f.texto, colApagado)
		a.ayudaAbajo("B  Volver")
		return
	}
	de := "Sonando"
	if a.colaDe != "" {
		de += " · " + a.colaDe
	}
	l.izquierda(image.Rect(28, 20, 400, 44), recortar(l.f.pequena, de, 360), l.f.pequena, colSuave)
	p := a.portadaCancion(c, 232, true)
	l.pegar(p.img, p.sb, 28, 64)
	const x0 = 290
	w := ancho - 24 - x0
	l.arriba(x0, 82, recortar(l.f.titulo, l.f.limpiar(c.Nombre), w), l.f.titulo, colTexto)
	sub := c.artistas()
	if c.Album != "" {
		sub += "  ·  " + c.Album
	}
	l.arriba(x0, 116, recortar(l.f.pequena, l.f.limpiar(sub), w), l.f.pequena, colAzul)
	// Barra de progreso.
	x1 := ancho - 28
	l.redondeado(image.Rect(x0, 154, x1, 158), 2, color.RGBA{60, 60, 60, 60})
	pos, dur := a.rep.posicion(), c.duracion()
	f := 0.0
	if dur > 0 {
		f = min(float64(pos)/float64(dur), 1)
	}
	xp := x0 + int(float64(x1-x0)*f)
	l.redondeado(image.Rect(x0, 154, xp, 158), 2, colVerde)
	l.circulo(xp, 156, 6, colTexto)
	izq := fmtDur(pos)
	if a.buscando {
		izq = "Buscando…"
	}
	l.arriba(x0, 168, izq, l.f.mini, colSuave)
	dd := fmtDur(dur)
	l.arriba(x1-medir(l.f.mini, dd), 168, dd, l.f.mini, colSuave)
	// Botones.
	l.centrado(image.Rect(x0+20, 200, x0+60, 244), icAnterior, l.f.icono, colTexto)
	l.circulo(x0+120, 222, 26, colTexto)
	ic := icPausa
	if a.rep.enPausa() || !a.rep.sonando() {
		ic = icPlay
	}
	l.centrado(image.Rect(x0+94, 196, x0+146, 248), ic, l.f.iconoGrande, color.RGBA{10, 14, 24, 255})
	l.centrado(image.Rect(x0+180, 200, x0+220, 244), icSiguiente, l.f.icono, colTexto)
	if _, ok := a.alm.descargada(c.ID); ok {
		l.centrado(image.Rect(x0+236, 200, x0+262, 244), icHecho, l.f.iconoPeq, colVerde)
	}
	// El volumen, con cómo cambiarlo.
	if a.volumen < 0 {
		a.volumen = leerVolumen()
	}
	if a.volumen >= 0 {
		l.centrado(image.Rect(x0+262, 196, x0+300, 224), icAltavoz, l.f.iconoPeq, colSuave)
		l.centrado(image.Rect(x0+250, 220, x0+312, 240), fmt.Sprintf("%d %%", a.volumen), l.f.mini, colSuave)
		l.centrado(image.Rect(x0+236, 238, x0+326, 256), "arriba / abajo", l.f.mini, colApagado)
	}
	// Letra: la línea de ahora y las de alrededor.
	y := 278
	k := a.lineaActual()
	switch {
	case len(a.letra) > 0:
		desde := max(0, k-1)
		for i := desde; i < len(a.letra) && i < desde+4; i++ {
			cara, col := l.f.normal, color.Color(color.RGBA{110, 110, 110, 110})
			if i == k {
				cara, col = l.f.nombre, colTexto
			}
			l.arriba(x0, y, recortar(cara, l.f.limpiar(a.letra[i].Texto), w), cara, col)
			y += 30
		}
	case a.letraDe == c.ID:
		l.arriba(x0, y, "Esta canción no tiene letra", l.f.normal, colApagado)
	}
	a.ayudaAbajo("A  Pausa      L  R  Canción      Y  Letra      X  Lista      B  Volver")
}

func (a *app) dibujarLetra() {
	l := a.l
	c := a.actual()
	a.hora(false)
	if c == nil {
		return
	}
	l.izquierda(image.Rect(28, 20, 420, 44), recortar(l.f.pequena, l.f.limpiar(c.Nombre+" · "+c.artistas()), 380), l.f.pequena, colSuave)
	if len(a.letra) == 0 {
		l.centrado(image.Rect(0, 200, ancho, 240), "Esta canción no tiene letra", l.f.texto, colApagado)
		a.ayudaAbajo("B  Volver")
		return
	}
	k := a.lineaActual()
	desde := max(0, k-3)
	y := 70
	for i := desde; i < len(a.letra) && y < alto-70; i++ {
		cara, col := l.f.texto, color.Color(color.RGBA{120, 120, 120, 120})
		if i == k {
			cara, col = l.f.grande, colTexto
		}
		for _, ln := range ajustar(cara, l.f.limpiar(a.letra[i].Texto), ancho-80) {
			l.centrado(image.Rect(40, y, ancho-40, y+altoLinea(cara)), ln, cara, col)
			y += altoLinea(cara) + 4
		}
		y += 6
	}
	a.ayudaAbajo("A  Pausa      L  R  Canción      B  Volver")
}

// ---- Buscar ---------------------------------------------------------------------------------------

func (a *app) dibujarBuscar() {
	l := a.l
	l.texto(28, 40+(l.f.icono.Metrics().Ascent.Round()-l.f.icono.Metrics().Descent.Round())/2, icBuscar, l.f.icono, colTexto)
	l.izquierda(image.Rect(60, 22, 400, 58), "Buscar", l.f.grande, colTexto)
	a.hora(false)
	caja := image.Rect(24, 74, ancho-24, 122)
	a.translucido(caja, 14, 50)
	t := a.tec
	texto := string(t.borrador[:t.cursor]) + "|" + string(t.borrador[t.cursor:])
	col := color.Color(colTexto)
	if len(t.borrador) == 0 {
		texto, col = "|  Canción, artista o álbum", colApagado
	}
	l.izquierda(caja.Inset(14), texto, l.f.texto, col)
	// Teclas.
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
		if t.fila == 4 && t.col == i {
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		} else if i == 3 {
			l.redondeado(k, 12, colVerde)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		} else {
			a.translucido(k, 12, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir      B  Borrar      START  Buscar      SELECT  Volver")
}
