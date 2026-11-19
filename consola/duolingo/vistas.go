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

// Pantallas con el estilo de Panther OS (fondo de las listas, letra Selawik) y los colores de
// Duolingo: verde para lo bueno y para avanzar, rojo para los fallos, dorado para lo ya hecho.
var (
	colVerdeDuo = color.RGBA{0x58, 0xcc, 0x02, 0xff}
	colVerdeOsc = color.RGBA{0x46, 0xa3, 0x02, 0xff}
	colAzulDuo  = color.RGBA{0x1c, 0xb0, 0xf6, 0xff}
	colAzulOsc  = color.RGBA{0x18, 0x99, 0xd6, 0xff}
	colRojoDuo  = color.RGBA{0xff, 0x4b, 0x4b, 0xff}
	colRojoOsc  = color.RGBA{0xea, 0x2b, 0x2b, 0xff}
	colOro      = color.RGBA{0xff, 0xc8, 0x00, 0xff}
	colOroOsc   = color.RGBA{0xe5, 0xa4, 0x00, 0xff}
	colFuego    = color.RGBA{0xff, 0x96, 0x00, 0xff}
	colGris     = color.RGBA{0x37, 0x46, 0x4f, 0xff}
	colGrisOsc  = color.RGBA{0x2a, 0x35, 0x3c, 0xff}
	colPanel    = color.RGBA{0x10, 0x18, 0x28, 0xd8}
	colBienFdo  = color.RGBA{0x14, 0x3a, 0x12, 0xf4}
	colMalFdo   = color.RGBA{0x45, 0x14, 0x18, 0xf4}
)

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x40})
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pCuenta:
		a.dibujarCuenta()
	case pInicio:
		a.dibujarInicio()
	case pPerfil:
		a.dibujarPerfil()
	case pCargando:
		a.dibujarCargando()
	case pLeccion:
		a.dibujarLeccion()
	case pFin:
		a.dibujarFin()
	case pSinVidas:
		a.dibujarSinVidas()
	}
	if !hayRed() && a.pant != pLeccion {
		a.chip(image.Rect(ancho/2-80, alto-74, ancho/2+80, alto-46), icWifiNo+"  Sin internet", colRojoDuo)
	}
	if a.confirmSi != nil {
		a.dibujarConfirmar()
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

// circuloSuave: círculo con el borde suavizado.
func (l *lienzo) circuloSuave(cx, cy int, r float64, c color.RGBA) {
	R := int(r) + 2
	for y := cy - R; y <= cy+R; y++ {
		for x := cx - R; x <= cx+R; x++ {
			d := math.Hypot(float64(x-cx)+0.5, float64(y-cy)+0.5)
			mezclar(l.img, x, y, c, r-d+0.5)
		}
	}
}

// boton: rectángulo redondeado con un borde de abajo más oscuro (el «botón gordo» de Duolingo).
func (l *lienzo) boton(r image.Rectangle, c, borde color.RGBA) {
	l.redondeado(r, 14, borde)
	l.redondeado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-4), 14, c)
}

// logo: cuadrado verde redondeado con un bocadillo blanco y «Aa».
func (a *app) logo(x, y, tam int) {
	l := a.l
	l.redondeado(image.Rect(x, y, x+tam, y+tam), tam/4, colVerdeDuo)
	b := image.Rect(x+tam*18/100, y+tam*20/100, x+tam*82/100, y+tam*68/100)
	l.redondeado(b, tam/9, colTexto)
	// la colita del bocadillo
	for k := 0; k < tam*14/100; k++ {
		l.rect(image.Rect(b.Min.X+tam*16/100, b.Max.Y+k, b.Min.X+tam*30/100-k, b.Max.Y+k+1), colTexto)
	}
	var cara font.Face
	switch {
	case tam >= 90:
		cara = a.f.grande
	case tam >= 40:
		cara = a.f.nombre
	default:
		cara = a.f.mini
	}
	l.centrado(b, "Aa", cara, colVerdeOsc)
}

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Duolingo", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

func (a *app) chip(r image.Rectangle, s string, c color.RGBA) {
	a.l.redondeado(r, r.Dy()/2, color.RGBA{0, 0, 0, 0xd0})
	a.textoConIconos(r, s, a.f.pequena, c, true)
}

// textoConIconos: escribe un texto que puede llevar iconos de Font Awesome (los de icXxx).
func (a *app) textoConIconos(r image.Rectangle, s string, f font.Face, c color.Color, centrado bool) {
	l := a.l
	type trozo struct {
		s  string
		ic bool
	}
	var ts []trozo
	for _, ru := range s {
		ic := ru >= 0xf000 && ru <= 0xf2ff
		if len(ts) == 0 || ts[len(ts)-1].ic != ic {
			ts = append(ts, trozo{ic: ic})
		}
		ts[len(ts)-1].s += string(ru)
	}
	icono := a.f.iconoPeq
	if f == a.f.nombre || f == a.f.titulo2 || f == a.f.texto {
		icono = a.f.icono
	}
	if f == a.f.grande || f == a.f.titulo {
		icono = a.f.iconoGrande
	}
	w := 0
	for _, t := range ts {
		if t.ic {
			w += medir(icono, t.s)
		} else {
			w += medir(f, t.s)
		}
	}
	x := r.Min.X
	if centrado {
		x = r.Min.X + (r.Dx()-w)/2
	}
	base := baseCentrada(r, f)
	for _, t := range ts {
		if t.ic {
			x = l.texto(x, base, t.s, icono, c)
		} else {
			x = l.texto(x, base, t.s, f, c)
		}
	}
}

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-118, (ancho+w)/2, alto-86)
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xe8})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	s = recortar(l.f.pequena, s, ancho-16)
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-18+(l.f.pequena.Metrics().Ascent.Round()-l.f.pequena.Metrics().Descent.Round())/2, s, l.f.pequena, colAyuda)
}

func puntos() string {
	return strings.Repeat("·", int(time.Now().UnixMilli()/400%3)+1)
}

func (a *app) dibujarConfirmar() {
	l := a.l
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x90})
	r := image.Rect(70, 150, ancho-70, 330)
	l.redondeado(r, 18, color.RGBA{0x14, 0x1a, 0x2a, 0xf6})
	y := 180
	for _, ln := range ajustar(l.f.texto, a.confirmTexto, r.Dx()-60) {
		l.centrado(image.Rect(r.Min.X+20, y, r.Max.X-20, y+26), ln, l.f.texto, colTexto)
		y += 28
	}
	l.centrado(image.Rect(r.Min.X, r.Max.Y-48, r.Max.X, r.Max.Y-16), "A  Sí      B  No", l.f.pequena, colAyuda)
}

// ---- Cuenta --------------------------------------------------------------------------------------

func (a *app) dibujarCuenta() {
	l := a.l
	a.logo(22, 14, 44)
	l.izquierda(image.Rect(78, 10, 460, 42), "Duolingo", l.f.grande, colTexto)
	l.izquierda(image.Rect(80, 44, 460, 64), "Pasa tu cuenta a la consola", l.f.mini, colSuave)
	ip := miIP()
	x, y := 28, 88
	texto := "Para hacer tus lecciones aquí (y que cuenten para tu racha), abre esta página en el PC o en el móvil, en la misma red que la consola, y sigue los pasos:"
	if a.cuentaMal {
		texto = "Tu sesión ha caducado. " + texto
	}
	for _, ln := range ajustar(l.f.texto, texto, 330) {
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
	l.centrado(r, dir, l.f.nombre, colAzulDuo)
	y += 70
	for _, ln := range ajustar(l.f.pequena, a.sesionTexto, 330) {
		l.arriba(x, y, ln, l.f.pequena, colSuave)
		y += 22
	}
	if ip != "" {
		if q, err := qrcode.New(dir, qrcode.Medium); err == nil {
			q.DisableBorder = true
			img := q.Image(220)
			fondo := image.Rect(390, 88, 390+240, 88+240)
			l.redondeado(fondo, 16, colTexto)
			b := img.Bounds()
			for yy := 0; yy < b.Dy(); yy++ {
				for xx := 0; xx < b.Dx(); xx++ {
					if rr, _, _, _ := img.At(xx, yy).RGBA(); rr < 0x8000 {
						l.img.Set(fondo.Min.X+10+xx, fondo.Min.Y+10+yy, color.Black)
					}
				}
			}
			l.centrado(image.Rect(390, 334, 630, 356), "Con el móvil, escanéalo", l.f.mini, colSuave)
		}
	}
	if a.duo.cuenta.tiene() {
		a.ayudaAbajo("B  Dejar la cuenta de ahora")
	} else {
		a.ayudaAbajo("B  Salir")
	}
}

// ---- El camino -----------------------------------------------------------------------------------

// cabeceraInicio: logo y curso a la izquierda; racha, gemas y vidas (sin tapar el recuadro de la
// batería de arriba a la derecha).
func (a *app) cabeceraInicio() {
	l := a.l
	p := a.perfil
	a.logo(20, 12, 40)
	curso := p.Curso
	if curso == "" {
		curso = "Duolingo"
	}
	l.izquierda(image.Rect(70, 10, 300, 36), recortar(l.f.titulo2, curso, 220), l.f.titulo2, colTexto)
	if p.Usuario != "" {
		l.izquierda(image.Rect(71, 36, 300, 54), recortar(l.f.mini, p.Usuario, 220), l.f.mini, colSuave)
	}
	if !a.perfilHecho {
		return
	}
	fuego := colApagado
	if p.RachaHoy {
		fuego = colFuego
	}
	x := 250
	pastilla := func(ic string, valor string, c color.RGBA) {
		w := medir(a.f.icono, ic) + medir(a.f.nombre, valor) + 34
		r := image.Rect(x, 30, x+w, 58)
		a.translucido(r, 14, 34)
		a.l.texto(r.Min.X+12, baseCentrada(r, a.f.icono), ic, a.f.icono, c)
		a.l.texto(r.Min.X+20+medir(a.f.icono, ic), baseCentrada(r, a.f.nombre), valor, a.f.nombre, colTexto)
		x += w + 8
	}
	pastilla(icFuego, itoa(p.Racha), fuego)
	pastilla(icGema, itoa(p.Gemas), colAzulDuo)
	if p.Vidas < 0 {
		pastilla(icCorazon, "Sin límite", colRojoDuo)
	} else {
		pastilla(icCorazon, itoa(p.Vidas), colRojoDuo)
	}
}

func iconoNivel(n nivel) string {
	switch n.Tipo {
	case "story":
		return icLibro
	case "chest":
		return icCofre
	case "unit_review", "practice", "review":
		return icRepaso
	case "unit_test", "legendary", "test":
		return icTrofeo
	}
	if n.Estado == estHecho {
		return icHecho
	}
	if n.Estado == estBloqueado {
		return icCandado
	}
	return icEstrella
}

func (a *app) dibujarInicio() {
	l := a.l
	if len(a.camino) == 0 {
		a.cabeceraInicio()
		msg := "Cargando tu curso" + puntos()
		if a.errCarga != "" && !a.cargando {
			msg = a.errCarga
		}
		y := 200
		for _, ln := range ajustar(l.f.texto, msg, ancho-100) {
			l.centrado(image.Rect(40, y, ancho-40, y+28), ln, l.f.texto, colTexto)
			y += 28
		}
		if a.errCarga != "" {
			a.ayudaAbajo("Y  Probar otra vez      R  Perfil      B  Salir")
		} else {
			a.ayudaAbajo("B  Salir")
		}
		return
	}
	u := a.unidadVista()
	// los niveles: en zigzag, como un camino, y se mueve para que el elegido se vea
	vista := image.Rect(0, 136, 400, alto-46)
	paso := 70
	n := len(u.Niveles)
	centro := vista.Min.Y + vista.Dy()/2
	y0 := centro - a.nivelSel*paso
	minY := vista.Min.Y + 36
	if y0 > minY {
		y0 = minY
	}
	if fin := y0 + (n-1)*paso; fin < vista.Max.Y-40 && n > 1 {
		y0 += min(vista.Max.Y-40-fin, minY-y0)
	}
	zig := []int{0, -55, -80, -55, 0, 55, 80, 55}
	cx := vista.Min.X + vista.Dx()/2
	for i, nv := range u.Niveles {
		y := y0 + i*paso
		if y < vista.Min.Y-30 || y > vista.Max.Y+30 {
			continue
		}
		x := cx + zig[i%len(zig)]
		c, cb := colGris, colGrisOsc
		ic := colApagado
		switch nv.Estado {
		case estHecho:
			c, cb, ic = colOro, colOroOsc, colTexto
		case estActivo:
			c, cb, ic = colVerdeDuo, colVerdeOsc, colTexto
		}
		if i == a.nivelSel {
			l.circuloSuave(x, y, 39, colTexto)
			l.circuloSuave(x, y, 35, color.RGBA{0x0b, 0x12, 0x20, 0xff})
		}
		l.circuloSuave(x, y+4, 29, cb)
		l.circuloSuave(x, y, 29, c)
		l.centrado(image.Rect(x-29, y-29, x+29, y+29), iconoNivel(nv), a.f.iconoGrande, ic)
		if nv.Estado == estActivo && nv.Total > 1 {
			// cuántas lecciones del nivel llevas, en un arco alrededor
			k := float64(nv.Hechas) / float64(nv.Total)
			for ang := 0.0; ang < 2*math.Pi*k; ang += 0.02 {
				px := float64(x) + 35*math.Sin(ang)
				py := float64(y) - 35*math.Cos(ang)
				l.circuloSuave(int(px), int(py), 2.2, colVerdeDuo)
			}
		}
	}
	l.taparFondo(image.Rect(0, vista.Max.Y, ancho, alto))
	l.rect(image.Rect(0, vista.Max.Y, ancho, alto), color.RGBA{0, 0, 0, 0x40})
	l.taparFondo(image.Rect(0, 0, ancho, vista.Min.Y))
	l.rect(image.Rect(0, 0, ancho, vista.Min.Y), color.RGBA{0, 0, 0, 0x40})
	a.cabeceraInicio()
	// la tira de la unidad
	tira := image.Rect(20, 68, ancho-20, 126)
	l.boton(tira, colVerdeDuo, colVerdeOsc)
	sup := strings.ToUpper(u.Titulo)
	if u.Seccion != "" {
		sup = strings.ToUpper(u.Seccion + " · " + u.Titulo)
	}
	l.izquierda(image.Rect(tira.Min.X+18, tira.Min.Y+6, tira.Max.X-80, tira.Min.Y+26), recortar(l.f.mini, sup, tira.Dx()-110), l.f.mini, color.RGBA{0xe8, 0xff, 0xd6, 0xff})
	tema := u.Tema
	if tema == "" {
		tema = u.Titulo
	}
	l.izquierda(image.Rect(tira.Min.X+18, tira.Min.Y+24, tira.Max.X-80, tira.Max.Y-6), recortar(l.f.titulo2, l.f.limpiar(tema), tira.Dx()-110), l.f.titulo2, colTexto)
	cuenta := fmt.Sprintf("%d/%d", a.unidad+1, len(a.camino))
	zc := image.Rect(tira.Max.X-120, tira.Min.Y, tira.Max.X-34, tira.Max.Y-4)
	l.derecha(zc, cuenta, l.f.pequena, colTexto)
	if a.unidad > 0 {
		l.derecha(image.Rect(zc.Min.X, zc.Min.Y, zc.Max.X-medir(l.f.pequena, cuenta)-10, zc.Max.Y), icIzquierda, a.f.iconoPeq, colTexto)
	}
	if a.unidad < len(a.camino)-1 {
		l.izquierda(image.Rect(zc.Max.X+10, zc.Min.Y, tira.Max.X-8, zc.Max.Y), icDerecha, a.f.iconoPeq, colTexto)
	}

	// a la derecha: el nivel elegido y la meta del día
	panel := image.Rect(400, 150, ancho-20, 300)
	l.redondeado(panel, 16, colPanel)
	if a.nivelSel < n {
		nv := u.Niveles[a.nivelSel]
		y := panel.Min.Y + 14
		for _, ln := range ajustar(l.f.nombre, l.f.limpiar(nv.Nombre), panel.Dx()-32) {
			l.arriba(panel.Min.X+16, y, ln, l.f.nombre, colTexto)
			y += 24
		}
		estado := ""
		switch {
		case nv.Tipo == "story":
			estado = "Historia (en el móvil)"
		case nv.Tipo == "chest":
			estado = "Cofre (en el móvil)"
		case nv.Estado == estHecho:
			estado = "Hecho. A para repasarlo"
		case nv.Estado == estActivo && nv.Total > 0:
			estado = fmt.Sprintf("Lección %d de %d", min(nv.Hechas+1, nv.Total), nv.Total)
		case nv.Estado == estActivo:
			estado = "Te toca"
		default:
			estado = "Bloqueado"
		}
		for _, ln := range ajustar(l.f.pequena, estado, panel.Dx()-32) {
			l.arriba(panel.Min.X+16, y+2, ln, l.f.pequena, colSuave)
			y += 21
		}
		if nv.Estado != estBloqueado && nv.Tipo != "story" && nv.Tipo != "chest" {
			bt := image.Rect(panel.Min.X+16, panel.Max.Y-54, panel.Max.X-16, panel.Max.Y-12)
			if nv.Estado == estHecho {
				l.boton(bt, colOro, colOroOsc)
				l.centrado(image.Rect(bt.Min.X, bt.Min.Y, bt.Max.X, bt.Max.Y-4), "REPASAR", l.f.nombre, colGrisOsc)
			} else {
				l.boton(bt, colVerdeDuo, colVerdeOsc)
				l.centrado(image.Rect(bt.Min.X, bt.Min.Y, bt.Max.X, bt.Max.Y-4), "EMPEZAR", l.f.nombre, colTexto)
			}
		}
	}
	meta := image.Rect(400, 312, ancho-20, 400)
	l.redondeado(meta, 16, colPanel)
	p := a.perfil
	l.izquierda(image.Rect(meta.Min.X+16, meta.Min.Y+10, meta.Max.X-16, meta.Min.Y+32), "Meta del día", l.f.negrita, colTexto)
	if p.Meta > 0 {
		k := math.Min(1, float64(p.XPHoy)/float64(p.Meta))
		barra := image.Rect(meta.Min.X+16, meta.Min.Y+44, meta.Max.X-16, meta.Min.Y+60)
		l.redondeado(barra, 8, colGris)
		if k > 0 {
			l.redondeado(image.Rect(barra.Min.X, barra.Min.Y, barra.Min.X+max(16, int(float64(barra.Dx())*k)), barra.Max.Y), 8, colOro)
		}
		l.izquierda(image.Rect(meta.Min.X+16, meta.Min.Y+64, meta.Max.X-16, meta.Max.Y-4), fmt.Sprintf("%d de %d XP", p.XPHoy, p.Meta), l.f.pequena, colSuave)
	} else {
		l.izquierda(image.Rect(meta.Min.X+16, meta.Min.Y+40, meta.Max.X-16, meta.Max.Y-8), fmt.Sprintf("Hoy: %d XP", p.XPHoy), l.f.pequena, colSuave)
	}
	a.ayudaAbajo("A  Empezar    X  Practicar    IZQ DER  Unidades    R  Perfil    B  Salir")
}

// ---- Perfil --------------------------------------------------------------------------------------

func (a *app) dibujarPerfil() {
	l := a.l
	a.cabeceraInicio()
	p := a.perfil
	r := image.Rect(20, 72, ancho-20, 176)
	l.redondeado(r, 18, colPanel)
	inicial := "?"
	if rs := []rune(strings.TrimSpace(p.Nombre + p.Usuario)); len(rs) > 0 {
		inicial = strings.ToUpper(string(rs[0]))
	}
	l.circuloSuave(r.Min.X+56, r.Min.Y+52, 36, colAzulDuo)
	l.centrado(image.Rect(r.Min.X+20, r.Min.Y+16, r.Min.X+92, r.Min.Y+88), inicial, l.f.grande, colTexto)
	nombre := p.Nombre
	if nombre == "" {
		nombre = p.Usuario
	}
	l.izquierda(image.Rect(r.Min.X+112, r.Min.Y+18, r.Max.X-20, r.Min.Y+50), recortar(l.f.grande, l.f.limpiar(nombre), r.Dx()-140), l.f.grande, colTexto)
	sub := p.Usuario
	if p.Curso != "" {
		sub += " · aprende " + strings.ToLower(p.Curso)
	}
	l.izquierda(image.Rect(r.Min.X+112, r.Min.Y+54, r.Max.X-20, r.Min.Y+76), recortar(l.f.pequena, sub, r.Dx()-140), l.f.pequena, colSuave)

	casillas := []struct {
		ic, valor, nombre string
		c                 color.RGBA
	}{
		{icFuego, itoa(p.Racha), "días de racha", colFuego},
		{icRayo, itoa(p.XPTotal), "XP en total", colOro},
		{icGema, itoa(p.Gemas), "gemas", colAzulDuo},
		{icTrofeo, siVacio(p.Liga, "—"), "liga", colOro},
	}
	w := (ancho - 40 - 3*12) / 4
	for i, c := range casillas {
		x := 20 + i*(w+12)
		k := image.Rect(x, 192, x+w, 290)
		l.redondeado(k, 16, colPanel)
		l.centrado(image.Rect(k.Min.X, k.Min.Y+10, k.Max.X, k.Min.Y+40), c.ic, a.f.iconoGrande, c.c)
		l.centrado(image.Rect(k.Min.X, k.Min.Y+42, k.Max.X, k.Min.Y+68), recortar(l.f.titulo2, c.valor, w-12), l.f.titulo2, colTexto)
		l.centrado(image.Rect(k.Min.X, k.Min.Y+68, k.Max.X, k.Max.Y-6), c.nombre, l.f.mini, colSuave)
	}
	y := 312
	for _, ln := range ajustar(l.f.pequena, "Tu sesión se queda solo en la consola y solo se usa para tus lecciones. Lo que haces aquí cuenta en tu cuenta de Duolingo como en el móvil.", ancho-60) {
		l.izquierda(image.Rect(30, y, ancho-30, y+22), ln, l.f.pequena, colApagado)
		y += 21
	}
	if a.cuentaMal {
		l.izquierda(image.Rect(30, y+8, ancho-30, y+30), "La sesión ha caducado: pulsa A y pásala otra vez.", l.f.normal, colRojoDuo)
	}
	a.ayudaAbajo("A  Cambiar de cuenta      X  Cerrar sesión      B  Volver")
}

func siVacio(s, otro string) string {
	if s == "" {
		return otro
	}
	return s
}

// ---- Preparando la lección -----------------------------------------------------------------------

func (a *app) dibujarCargando() {
	l := a.l
	a.logo(ancho/2-45, 130, 90)
	if a.errLec != "" {
		y := 250
		for _, ln := range ajustar(l.f.texto, a.errLec, ancho-100) {
			l.centrado(image.Rect(40, y, ancho-40, y+28), ln, l.f.texto, colTexto)
			y += 28
		}
		a.ayudaAbajo("B  Volver")
		return
	}
	l.centrado(image.Rect(0, 250, ancho, 280), "Preparando la lección"+puntos(), l.f.texto, colTexto)
	a.ayudaAbajo("B  Cancelar")
}

// ---- La lección ----------------------------------------------------------------------------------

func (a *app) dibujarLeccion() {
	l := a.l
	lec, t := a.lec, a.t
	if lec == nil || t == nil {
		return
	}
	e := t.ej
	// arriba: el nombre, la barra de lo que llevas y tus vidas
	l.izquierda(image.Rect(24, 4, 440, 26), recortar(l.f.mini, lec.nombre, 400), l.f.mini, colSuave)
	barra := image.Rect(24, 34, ancho-118, 52)
	l.redondeado(barra, 9, colGris)
	if k := lec.progreso(); k > 0 {
		l.redondeado(image.Rect(barra.Min.X, barra.Min.Y, barra.Min.X+max(18, int(float64(barra.Dx())*k)), barra.Max.Y), 9, colVerdeDuo)
		l.redondeado(image.Rect(barra.Min.X+8, barra.Min.Y+4, barra.Min.X+max(18, int(float64(barra.Dx())*k))-8, barra.Min.Y+8), 2, color.RGBA{0x9e, 0xe5, 0x6a, 0xff})
	}
	vr := image.Rect(ancho-104, 28, ancho-20, 58)
	if lec.vidas >= 0 { // sin límite (practicando o con Super) no se pintan
		l.texto(vr.Min.X+10, baseCentrada(vr, a.f.icono), icCorazon, a.f.icono, colRojoDuo)
		l.texto(vr.Min.X+38, baseCentrada(vr, a.f.titulo2), itoa(lec.vidas), a.f.titulo2, colRojoDuo)
	}

	if e.Tipo != ejAjedrez {
		l.izquierda(image.Rect(24, 62, ancho-24, 92), recortar(l.f.titulo2, e.Instruccion, ancho-48), l.f.titulo2, colTexto)
	}

	switch e.Tipo {
	case ejElegir:
		a.dibujarElegir(t)
	case ejBanco:
		a.dibujarBanco(t)
	case ejHuecos:
		a.dibujarHuecos(t)
	case ejParejas:
		a.dibujarParejas(t)
	case ejEscribir:
		a.dibujarEscribirEj(t)
	case ejAjedrez:
		a.dibujarAjedrez(t)
	}

	if t.comprobado {
		a.dibujarVeredicto(t)
		return
	}
	oir := ""
	if e.Audio != "" {
		oir = "    X  Oír"
	}
	switch e.Tipo {
	case ejElegir:
		a.ayudaAbajo("A  Elegir (otra vez: comprobar)" + oir + "    START  Comprobar    B  Salir")
	case ejBanco, ejHuecos:
		a.ayudaAbajo("A  Poner    B  Quitar" + oir + "    L  Quitar todas    START  Comprobar")
	case ejParejas:
		a.ayudaAbajo("A  Elegir    IZQ DER  Columna    B  Salir")
	case ejEscribir:
		a.ayudaAbajo("A  Letra    B  Borrar    Y  Espacio    X  Letras    START  Comprobar")
	case ejAjedrez:
		a.ayudaAbajo("A  Coger y dejar    B  Soltar    Y  Pista")
	}
}

// enunciado: el bocadillo con la frase (o el altavoz grande si solo hay que oírla). Devuelve dónde acaba.
func (a *app) enunciado(e *ejercicio, y int) int {
	l := a.l
	if e.SoloOir {
		c := image.Rect(ancho/2-46, y+6, ancho/2+46, y+92)
		l.boton(c, colAzulDuo, colAzulOsc)
		l.centrado(image.Rect(c.Min.X, c.Min.Y, c.Max.X, c.Max.Y-4), icAltavoz, a.f.iconoDe(40), colTexto)
		if e.AudioLento != "" {
			l.centrado(image.Rect(0, c.Max.Y+4, ancho, c.Max.Y+24), "X  Oír otra vez      Y  Más despacio", l.f.mini, colSuave)
		} else {
			l.centrado(image.Rect(0, c.Max.Y+4, ancho, c.Max.Y+24), "X  Oír otra vez", l.f.mini, colSuave)
		}
		return c.Max.Y + 30
	}
	if e.Frase == "" && len(e.Piezas) == 0 {
		return y
	}
	frase := e.Frase
	if len(e.Piezas) > 0 && e.Tipo == ejElegir {
		var b strings.Builder
		for _, p := range e.Piezas {
			if p.Hueco {
				b.WriteString(" ____ ")
			} else {
				b.WriteString(p.Texto)
			}
		}
		frase = b.String()
	}
	x0 := 24
	if e.Audio != "" {
		c := image.Rect(24, y+4, 72, y+52)
		l.boton(c, colAzulDuo, colAzulOsc)
		l.centrado(image.Rect(c.Min.X, c.Min.Y, c.Max.X, c.Max.Y-4), icAltavoz, a.f.icono, colTexto)
		x0 = 86
	}
	lineas := ajustar(l.f.texto, l.f.limpiar(frase), ancho-x0-50)
	if len(lineas) > 3 {
		lineas = lineas[:3]
	}
	h := len(lineas)*26 + 20
	b := image.Rect(x0, y, x0+40+maxAncho(l.f.texto, lineas), y+max(h, 56))
	b.Max.X = min(b.Max.X, ancho-24)
	l.redondeado(b, 14, colPanel)
	l.marcoRedondo(b, 14, 2, colGris)
	for i, ln := range lineas {
		l.arriba(b.Min.X+20, b.Min.Y+10+i*26+(b.Dy()-h)/2, ln, l.f.texto, colTexto)
	}
	return b.Max.Y + 12
}

func maxAncho(cara font.Face, ls []string) int {
	w := 0
	for _, s := range ls {
		w = max(w, medir(cara, s))
	}
	return w
}

func (a *app) dibujarElegir(t *turno) {
	l := a.l
	e := t.ej
	y := a.enunciado(e, 100)
	cols := a.columnasOpciones(e)
	n := len(e.Opciones)
	filas := (n + cols - 1) / cols
	hueco := alto - 120 - y
	h := min(56, (hueco-(filas-1)*10)/max(filas, 1))
	w := (ancho - 48 - (cols-1)*12) / cols
	for i, o := range e.Opciones {
		x := 24 + (i%cols)*(w+12)
		yy := y + (i/cols)*(h+10)
		r := image.Rect(x, yy, x+w, yy+h)
		borde, fondo, txt := colGris, colPanel, color.RGBA(colTexto)
		switch {
		case t.comprobado && i == e.Correcta:
			borde, fondo, txt = colVerdeDuo, colBienFdo, colVerdeDuo
		case t.comprobado && i == t.elegida:
			borde, fondo, txt = colRojoDuo, colMalFdo, colRojoDuo
		case i == t.elegida:
			borde, fondo, txt = colAzulDuo, color.RGBA{0x10, 0x2c, 0x44, 0xf0}, colAzulDuo
		}
		l.redondeado(r, 14, fondo)
		g := 2
		if i == t.foco && !t.comprobado {
			borde, g = colTexto, 3
		}
		l.marcoRedondo(r, 14, g, borde)
		num := image.Rect(r.Min.X+10, r.Min.Y+(h-28)/2, r.Min.X+38, r.Min.Y+(h-28)/2+28)
		l.marcoRedondo(num, 7, 2, colGris)
		l.centrado(num, itoa(i+1), l.f.mini, colSuave)
		l.izquierda(image.Rect(r.Min.X+50, r.Min.Y, r.Max.X-10, r.Max.Y), recortar(l.f.nombre, l.f.limpiar(o.Texto), r.Dx()-60), l.f.nombre, txt)
	}
	if !t.comprobado {
		a.botonComprobar(t.elegida >= 0, t.foco < 0)
	}
}

// botonComprobar: abajo a la derecha, verde cuando ya se puede.
func (a *app) botonComprobar(listo, foco bool) {
	l := a.l
	r := image.Rect(ancho-200, alto-92, ancho-24, alto-46)
	if listo {
		l.boton(r, colVerdeDuo, colVerdeOsc)
		l.centrado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-4), "COMPROBAR", l.f.nombre, colTexto)
	} else {
		l.boton(r, colGris, colGrisOsc)
		l.centrado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-4), "COMPROBAR", l.f.nombre, colApagado)
	}
	if foco {
		l.marcoRedondo(r.Inset(-4), 17, 3, colTexto)
	}
}

func (a *app) ficha(r image.Rectangle, texto string, puesta, foco bool) {
	l := a.l
	if puesta { // su hueco en el banco
		l.redondeado(r, 12, colGrisOsc)
	} else {
		l.redondeado(r, 12, colGris)
		l.redondeado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-3), 12, color.RGBA{0x1c, 0x26, 0x33, 0xff})
		l.marcoRedondo(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-3), 12, 2, colGris)
		l.centrado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-3), l.f.limpiar(texto), l.f.nombre, colTexto)
	}
	if foco {
		l.marcoRedondo(r.Inset(-3), 14, 3, colTexto)
	}
}

func (a *app) dibujarBanco(t *turno) {
	l := a.l
	e := t.ej
	a.enunciado(e, 100)
	// la respuesta: dos rayas y las fichas puestas encima
	zonaR := image.Rect(30, 190, ancho-30, 252)
	if e.SoloOir {
		zonaR = image.Rect(30, 200, ancho-30, 252)
	}
	for _, yy := range []int{zonaR.Min.Y + 24, zonaR.Max.Y - 2} {
		l.rect(image.Rect(zonaR.Min.X, yy, zonaR.Max.X, yy+2), colGris)
	}
	x, y := zonaR.Min.X, zonaR.Min.Y-14
	for _, i := range t.fichas {
		w := medir(l.f.nombre, l.f.limpiar(e.Opciones[i].Texto)) + 26
		if x+w > zonaR.Max.X {
			x, y = zonaR.Min.X, y+36
		}
		r := image.Rect(x, y, x+w, y+34)
		l.redondeado(r, 10, color.RGBA{0x1c, 0x26, 0x33, 0xff})
		l.marcoRedondo(r, 10, 2, colGris)
		l.centrado(r, l.f.limpiar(e.Opciones[i].Texto), l.f.nombre, colTexto)
		x += w + 8
	}
	for i, r := range a.colocarFichas(e, a.zonaFichas()) {
		a.ficha(r, e.Opciones[i].Texto, t.puesta(i), i == t.foco && !t.comprobado)
	}
}

func (a *app) dibujarHuecos(t *turno) {
	l := a.l
	e := t.ej
	y0 := 104
	if e.Audio != "" {
		c := image.Rect(24, y0, 72, y0+48)
		l.boton(c, colAzulDuo, colAzulOsc)
		l.centrado(image.Rect(c.Min.X, c.Min.Y, c.Max.X, c.Max.Y-4), icAltavoz, a.f.icono, colTexto)
	}
	// la frase, palabra a palabra, con los huecos (rellenos con lo que has puesto)
	x, y := 90, y0+8
	k := 0
	for _, p := range e.Piezas {
		if p.Hueco {
			txt := "      "
			if k < len(t.fichas) {
				txt = e.Opciones[t.fichas[k]].Texto
			}
			w := max(70, medir(l.f.nombre, txt)+24)
			if x+w > ancho-24 {
				x, y = 90, y+44
			}
			r := image.Rect(x, y, x+w, y+36)
			if k < len(t.fichas) {
				l.redondeado(r, 10, color.RGBA{0x1c, 0x26, 0x33, 0xff})
				l.marcoRedondo(r, 10, 2, colGris)
				l.centrado(r, l.f.limpiar(txt), l.f.nombre, colTexto)
			} else {
				l.rect(image.Rect(r.Min.X, r.Max.Y-2, r.Max.X, r.Max.Y), colSuave)
			}
			x += w + 6
			k++
			continue
		}
		for _, pal := range strings.Fields(p.Texto) {
			w := medir(l.f.texto, pal)
			if x+w > ancho-24 {
				x, y = 90, y+44
			}
			l.texto(x, y+26, pal, l.f.texto, colTexto)
			x += w + medir(l.f.texto, " ")
		}
	}
	for i, r := range a.colocarFichas(e, a.zonaFichas()) {
		a.ficha(r, e.Opciones[i].Texto, t.puesta(i), i == t.foco && !t.comprobado)
	}
}

func (a *app) dibujarParejas(t *turno) {
	l := a.l
	e := t.ej
	n := len(e.Izquierda)
	if n == 0 {
		return
	}
	top := 104
	h := min(58, (alto-130-top-(n-1)*10)/n)
	w := (ancho - 48 - 20) / 2
	for col := 0; col < 2; col++ {
		orden, lista, sel, mal := t.ordIzq, e.Izquierda, t.selIzq, t.malIzq
		if col == 1 {
			orden, lista, sel, mal = t.ordDer, e.Derecha, t.selDer, t.malDer
		}
		for fila, id := range orden {
			x := 24 + col*(w+20)
			y := top + fila*(h+10)
			r := image.Rect(x, y, x+w, y+h)
			o := lista[id]
			switch {
			case t.hechas[id]:
				l.redondeado(r, 14, color.RGBA{0x14, 0x1c, 0x26, 0xc0})
				l.centrado(r, recortar(l.f.nombre, l.f.limpiar(o.Texto), w-20), l.f.nombre, colGris)
			case id == mal && !t.malHasta.IsZero():
				l.redondeado(r, 14, colMalFdo)
				l.marcoRedondo(r, 14, 2, colRojoDuo)
				l.centrado(r, recortar(l.f.nombre, l.f.limpiar(o.Texto), w-20), l.f.nombre, colRojoDuo)
			case id == sel:
				l.redondeado(r, 14, color.RGBA{0x10, 0x2c, 0x44, 0xf0})
				l.marcoRedondo(r, 14, 2, colAzulDuo)
				l.centrado(r, recortar(l.f.nombre, l.f.limpiar(o.Texto), w-20), l.f.nombre, colAzulDuo)
			default:
				l.redondeado(r, 14, colPanel)
				l.marcoRedondo(r, 14, 2, colGris)
				l.centrado(r, recortar(l.f.nombre, l.f.limpiar(o.Texto), w-20), l.f.nombre, colTexto)
			}
			if t.col == col && t.foco%n == fila && !t.comprobado {
				l.marcoRedondo(r.Inset(-3), 17, 3, colTexto)
			}
		}
	}
}

func (a *app) dibujarEscribirEj(t *turno) {
	l := a.l
	e := t.ej
	y := a.enunciado(e, 96)
	if e.SoloOir {
		y -= 8
	}
	caja := image.Rect(24, y, ancho-24, y+42)
	a.translucido(caja, 12, 46)
	tx := a.tec
	texto := string(tx.borrador[:tx.cursor]) + "|" + string(tx.borrador[tx.cursor:])
	col := color.Color(colTexto)
	if len(tx.borrador) == 0 {
		texto, col = "|  Escribe en "+nombreIdioma(e.Idioma), colApagado
	}
	if t.comprobado {
		texto = tx.texto()
	}
	ls := ajustar(l.f.normal, texto, caja.Dx()-28)
	l.izquierda(image.Rect(caja.Min.X+14, caja.Min.Y, caja.Max.X-14, caja.Max.Y), ls[len(ls)-1], l.f.normal, col)
	if !t.comprobado {
		a.dibujarTeclado(caja.Max.Y+8, alto-36-(caja.Max.Y+8))
	}
}

// dibujarTeclado: el teclado en pantalla (el de la app de Claude), en el alto que quede.
func (a *app) dibujarTeclado(y0, hueco int) {
	l := a.l
	t := a.tec
	paso := min(52, hueco/5)
	h := paso - 6
	for f := 0; f < 4; f++ {
		fila := []rune(capas[t.capa][f])
		for c, r := range fila {
			k := image.Rect(14+c*61, y0+f*paso, 14+c*61+55, y0+f*paso+h)
			if t.fila == f && t.col == c {
				l.redondeado(k, 10, colTexto)
				l.centrado(k, string(r), l.f.nombre, colFondo)
			} else {
				a.translucido(k, 10, 38)
				l.centrado(k, string(r), l.f.nombre, colTexto)
			}
		}
	}
	anchos := []int{110, 254, 120, 126}
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Comprobar"}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*paso, x+w-6, y0+4*paso+h)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 10, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 10, colVerdeDuo)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 10, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
}

func (a *app) dibujarVeredicto(t *turno) {
	l := a.l
	e := t.ej
	r := image.Rect(0, alto-128, ancho, alto)
	fondo, c, titulo := colMalFdo, colRojoDuo, "Solución correcta:"
	ic := icMal
	if t.ver.Bien {
		fondo, c, titulo, ic = colBienFdo, colVerdeDuo, frasesBien[len(a.lec.acertados)%len(frasesBien)], icHecho
	}
	l.rect(r, fondo)
	l.circuloSuave(44, r.Min.Y+34, 18, c)
	l.centrado(image.Rect(26, r.Min.Y+16, 62, r.Min.Y+52), ic, a.f.icono, colFondo)
	l.izquierda(image.Rect(74, r.Min.Y+16, ancho-24, r.Min.Y+50), titulo, l.f.titulo2, c)
	texto := ""
	switch {
	case !t.ver.Bien:
		texto = e.Mostrar
		if texto == "" && e.Tipo == ejElegir && e.Correcta >= 0 && e.Correcta < len(e.Opciones) {
			texto = e.Opciones[e.Correcta].Texto
		}
		if texto == "" && e.Tipo == ejHuecos {
			var ps []string
			for _, i := range e.Correctas {
				ps = append(ps, e.Opciones[i].Texto)
			}
			texto = strings.Join(ps, " · ")
		}
	case t.ver.Nota != "":
		texto = t.ver.Nota
	}
	y := r.Min.Y + 54
	for i, ln := range ajustar(l.f.normal, l.f.limpiar(texto), ancho-200) {
		if i >= 2 {
			break
		}
		l.izquierda(image.Rect(74, y, ancho-200, y+22), ln, l.f.normal, c)
		y += 22
	}
	bt := image.Rect(ancho-180, r.Min.Y+58, ancho-24, r.Min.Y+104)
	if t.ver.Bien {
		l.boton(bt, colVerdeDuo, colVerdeOsc)
	} else {
		l.boton(bt, colRojoDuo, colRojoOsc)
	}
	l.centrado(image.Rect(bt.Min.X, bt.Min.Y, bt.Max.X, bt.Max.Y-4), "CONTINUAR", l.f.nombre, colTexto)
	l.centrado(image.Rect(bt.Min.X, bt.Max.Y+2, bt.Max.X, bt.Max.Y+20), "A", l.f.mini, colSuave)
}

var frasesBien = []string{"¡Muy bien!", "¡Genial!", "¡Perfecto!", "¡Eso es!", "¡Bien hecho!", "¡Increíble!"}

// ---- Fin -----------------------------------------------------------------------------------------

func (a *app) dibujarFin() {
	l := a.l
	lec := a.lec
	l.circuloSuave(ancho/2, 116, 56, colOro)
	l.centrado(image.Rect(ancho/2-56, 60, ancho/2+56, 172), icTrofeo, a.f.iconoDe(54), colTexto)
	l.centrado(image.Rect(0, 186, ancho, 222), "¡Lección completada!", l.f.grande, colOro)
	xp := "…"
	if a.enviado && a.fin.Error == "" {
		xp = itoa(a.fin.XP)
	}
	precision := 100
	if lec != nil && len(lec.ejs) > 0 {
		precision = len(lec.ejs) * 100 / (len(lec.ejs) + lec.fallos)
	}
	dur := time.Duration(0)
	if lec != nil {
		dur = time.Since(lec.inicio)
		if !a.fin.Hora.IsZero() {
			dur = a.fin.Hora.Sub(lec.inicio)
		}
	}
	casillas := []struct {
		titulo, valor string
		c             color.RGBA
	}{
		{"XP GANADOS", xp, colOro},
		{"ACIERTOS", fmt.Sprintf("%d %%", precision), colVerdeDuo},
		{"TIEMPO", fmt.Sprintf("%d:%02d", int(dur.Minutes()), int(dur.Seconds())%60), colAzulDuo},
	}
	w := 170
	x0 := (ancho - 3*w - 2*16) / 2
	for i, c := range casillas {
		x := x0 + i*(w+16)
		r := image.Rect(x, 240, x+w, 330)
		l.redondeado(r, 16, c.c)
		l.centrado(image.Rect(r.Min.X, r.Min.Y+4, r.Max.X, r.Min.Y+30), c.titulo, l.f.mini, colFondo)
		in := image.Rect(r.Min.X+3, r.Min.Y+30, r.Max.X-3, r.Max.Y-3)
		l.redondeado(in, 13, colFondo)
		l.centrado(in, c.valor, l.f.titulo2, c.c)
	}
	msg := ""
	switch {
	case !a.enviado:
		msg = "Mandándola a Duolingo" + puntos()
	case a.fin.Error != "":
		msg = "No se ha podido mandar a Duolingo: " + a.fin.Error
	case a.fin.Racha > 0:
		msg = fmt.Sprintf("Racha: %d días. ¡Sigue así!", a.fin.Racha)
	default:
		msg = "Guardada en tu cuenta de Duolingo."
	}
	y := 352
	for _, ln := range ajustar(l.f.texto, msg, ancho-80) {
		l.centrado(image.Rect(40, y, ancho-40, y+26), ln, l.f.texto, colTexto)
		y += 26
	}
	if a.enviado {
		a.ayudaAbajo("A  Continuar")
	}
}

func (a *app) dibujarSinVidas() {
	l := a.l
	l.circuloSuave(ancho/2, 140, 60, colRojoDuo)
	l.centrado(image.Rect(ancho/2-60, 80, ancho/2+60, 200), icCorazon, a.f.iconoDe(56), colTexto)
	l.centrado(image.Rect(0, 226, ancho, 262), "Te has quedado sin vidas", l.f.grande, colTexto)
	y := 280
	for _, ln := range ajustar(l.f.texto, "Se recargan solas con el tiempo. También puedes practicar (X en el camino) para recuperarlas.", ancho-120) {
		l.centrado(image.Rect(60, y, ancho-60, y+26), ln, l.f.texto, colSuave)
		y += 26
	}
	a.ayudaAbajo("A  Volver al camino")
}

func nombreIdioma(c string) string {
	if n, ok := idiomas[strings.ToLower(c)]; ok {
		return strings.ToLower(n)
	}
	return "el idioma que aprendes"
}

var idiomas = map[string]string{
	"en": "Inglés", "es": "Español", "fr": "Francés", "de": "Alemán", "it": "Italiano", "pt": "Portugués",
	"ja": "Japonés", "ko": "Coreano", "zh": "Chino", "zs": "Chino", "ru": "Ruso", "ar": "Árabe", "nl-NL": "Neerlandés",
	"nl": "Neerlandés", "sv": "Sueco", "no-BO": "Noruego", "nb": "Noruego", "pl": "Polaco", "tr": "Turco",
	"el": "Griego", "ca": "Catalán", "eu": "Euskera", "gl": "Gallego", "la": "Latín", "hi": "Hindi",
	"ga": "Irlandés", "cy": "Galés", "da": "Danés", "fi": "Finés", "uk": "Ucraniano", "he": "Hebreo",
	"hv": "Alto valyrio", "eo": "Esperanto", "math": "Matemáticas", "music": "Música", "chess": "Ajedrez",
}

// borrar lo que asoma: vuelve a poner el fondo (oscurecido como el resto) en «r».
func (l *lienzo) taparFondo(r image.Rectangle) {
	if l.fondo != nil {
		draw.Draw(l.img, r, l.fondo, r.Min, draw.Src)
	} else {
		draw.Draw(l.img, r, &image.Uniform{colFondo}, image.Point{}, draw.Src)
	}
}
