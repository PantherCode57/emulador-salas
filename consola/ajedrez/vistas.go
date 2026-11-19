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
)

// Pantallas con el estilo de Panther OS (fondo de las listas, letra Selawik) y colores de tablero
// de madera; el verde de los botones es el de Lichess.
var (
	colVerdeLi  = color.RGBA{0x62, 0x99, 0x24, 0xff}
	colVerdeOsc = color.RGBA{0x4a, 0x76, 0x1a, 0xff}
	colOro      = color.RGBA{0xf2, 0xc0, 0x37, 0xff}
	colRojoLi   = color.RGBA{0xe0, 0x4b, 0x4b, 0xff}
	colPanel    = color.RGBA{0x12, 0x14, 0x18, 0xdc}
	colGris     = color.RGBA{0x3a, 0x40, 0x48, 0xff}
)

const (
	icAmigos    = ""
	icOrdenador = ""
	icPuzzle    = ""
	icMando     = ""
	icReloj     = ""
	icRayoLi    = ""
)

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x50})
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pCuenta:
		a.dibujarCuenta()
	case pMenu:
		a.dibujarMenu()
	case pRival:
		a.dibujarRival()
	case pEscribir:
		a.dibujarEscribir()
	case pOpciones:
		a.dibujarOpciones()
	case pEsperando:
		a.dibujarEsperando()
	case pEmpezadas:
		a.dibujarEmpezadas()
	case pPartida:
		a.dibujarPartida()
	case pProblema:
		a.dibujarProblema()
	}
	if !hayRed() {
		a.chip(image.Rect(ancho/2-80, alto-74, ancho/2+80, alto-46), "Sin internet", colRojoLi)
	}
	if len(a.retos) > 0 {
		a.dibujarRetoEntrante()
	}
	if a.confirmSi != nil {
		a.dibujarConfirmar()
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

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

func (l *lienzo) circuloSuave(cx, cy int, r float64, c color.RGBA) {
	R := int(r) + 2
	for y := cy - R; y <= cy+R; y++ {
		for x := cx - R; x <= cx+R; x++ {
			d := math.Hypot(float64(x-cx)+0.5, float64(y-cy)+0.5)
			mezclar(l.img, x, y, c, r-d+0.5)
		}
	}
}

func (l *lienzo) boton(r image.Rectangle, c, borde color.RGBA) {
	l.redondeado(r, 12, borde)
	l.redondeado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-4), 12, c)
}

func (l *lienzo) taparFondo(r image.Rectangle) {
	if l.fondo != nil {
		draw.Draw(l.img, r, l.fondo, r.Min, draw.Src)
	} else {
		draw.Draw(l.img, r, &image.Uniform{colFondo}, image.Point{}, draw.Src)
	}
}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

// logo: un caballo blanco en un cuadrado verde.
func (a *app) logo(x, y, tam int) {
	r := image.Rect(x, y, x+tam, y+tam)
	a.l.redondeado(r, tam/4, colVerdeLi)
	if a.f.piezas != nil {
		cara := a.f.piezas
		if tam < 60 {
			cara = a.f.piezasPeq
		}
		a.l.centrado(r.Add(image.Pt(0, -tam/40)), "♞", cara, colTexto)
	}
}

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Ajedrez", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "con Lichess · Panther OS", l.f.mini, colSuave)
}

func (a *app) chip(r image.Rectangle, s string, c color.RGBA) {
	a.l.redondeado(r, r.Dy()/2, color.RGBA{0, 0, 0, 0xd0})
	a.l.centrado(r, s, a.l.f.pequena, c)
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

func puntos() string { return strings.Repeat("·", int(time.Now().UnixMilli()/400%3)+1) }

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

// dibujarRetoEntrante: «Dylan te reta» (arriba en las partidas; en medio en lo demás).
func (a *app) dibujarRetoEntrante() {
	l := a.l
	r := a.retos[0]
	quien := r.De
	if r.Elo > 0 {
		quien += fmt.Sprintf(" (%d)", r.Elo)
	}
	puntua := "sin puntos"
	if r.Puntua {
		puntua = "con puntos"
	}
	if a.pant == pPartida {
		caja := image.Rect(tabX+8*lado+16, 4, ancho-12, 42)
		l.redondeado(caja, 12, colOro)
		l.izquierda(image.Rect(caja.Min.X+10, caja.Min.Y, caja.Max.X-6, caja.Max.Y), recortar(l.f.mini, quien+" te reta · Y rechazar", caja.Dx()-16), l.f.mini, colFondo)
		return
	}
	l.rect(l.img.Bounds(), color.RGBA{0, 0, 0, 0x90})
	caja := image.Rect(80, 120, ancho-80, 350)
	l.redondeado(caja, 20, color.RGBA{0x16, 0x1c, 0x12, 0xf6})
	l.marcoRedondo(caja, 20, 2, colVerdeLi)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+18, caja.Max.X, caja.Min.Y+56), icAmigos, a.f.iconoGrande, colVerdeLi)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+60, caja.Max.X, caja.Min.Y+94), recortar(l.f.grande, quien, caja.Dx()-30), l.f.grande, colTexto)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+96, caja.Max.X, caja.Min.Y+122), "te reta a una partida", l.f.texto, colSuave)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+128, caja.Max.X, caja.Min.Y+152), r.Ritmo+" · "+puntua, l.f.normal, colOro)
	si := image.Rect(caja.Min.X+40, caja.Max.Y-62, caja.Min.X+40+150, caja.Max.Y-18)
	no := image.Rect(caja.Max.X-40-150, caja.Max.Y-62, caja.Max.X-40, caja.Max.Y-18)
	l.boton(si, colVerdeLi, colVerdeOsc)
	l.centrado(image.Rect(si.Min.X, si.Min.Y, si.Max.X, si.Max.Y-4), "A  ACEPTAR", l.f.nombre, colTexto)
	l.boton(no, colGris, color.RGBA{0x28, 0x2c, 0x32, 0xff})
	l.centrado(image.Rect(no.Min.X, no.Min.Y, no.Max.X, no.Max.Y-4), "B  AHORA NO", l.f.nombre, colTexto)
}

// ---- Cuenta --------------------------------------------------------------------------------------

func (a *app) dibujarCuenta() {
	l := a.l
	a.logo(22, 14, 44)
	l.izquierda(image.Rect(78, 10, 460, 42), "Ajedrez", l.f.grande, colTexto)
	l.izquierda(image.Rect(80, 44, 460, 64), "Pasa tu cuenta de Lichess a la consola", l.f.mini, colSuave)
	ip := miIP()
	x, y := 28, 88
	for _, ln := range ajustar(l.f.texto, "Para jugar con tus amigos desde aquí, abre esta página en el PC o en el móvil, en la misma red que la consola, y sigue los pasos (es un minuto):", 330) {
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
	l.centrado(r, dir, l.f.nombre, colOro)
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
	if a.li.cuenta.tiene() {
		a.ayudaAbajo("B  Dejar la cuenta de ahora")
	} else {
		a.ayudaAbajo("B  Salir")
	}
}

// ---- Menú ----------------------------------------------------------------------------------------

func (a *app) cabecera(titulo, sub string) {
	l := a.l
	a.logo(20, 12, 42)
	l.izquierda(image.Rect(72, 10, 440, 38), recortar(l.f.titulo2, titulo, 360), l.f.titulo2, colTexto)
	if sub != "" {
		l.izquierda(image.Rect(73, 36, 440, 56), recortar(l.f.mini, sub, 360), l.f.mini, colSuave)
	}
}

func (a *app) dibujarMenu() {
	l := a.l
	sub := "Conectando con Lichess" + puntos()
	if a.hayYo {
		var rs []string
		for _, k := range []string{"rapid", "blitz", "puzzle"} {
			if r, ok := a.yo.Ratings[k]; ok {
				rs = append(rs, map[string]string{"rapid": "Rápidas", "blitz": "Blitz", "puzzle": "Problemas"}[k]+" "+itoa(r))
			}
		}
		sub = a.yo.Usuario
		if len(rs) > 0 {
			sub += " · " + strings.Join(rs, " · ")
		}
	} else if a.errCuenta != "" {
		sub = a.errCuenta
	}
	a.cabecera("Ajedrez", sub)
	iconos := []string{icAmigos, icOrdenador, icPuzzle, icMando}
	subs := []string{"Reta a quien quieras por su nombre", "Del nivel 1 al 8, con Stockfish", "Los de Lichess, a tu nivel", ""}
	n := 0
	for _, p := range a.empezadas {
		if p.TeToca {
			n++
		}
	}
	subs[3] = fmt.Sprintf("%d en marcha", len(a.empezadas))
	if n > 0 {
		subs[3] += fmt.Sprintf(" · te toca en %d", n)
	}
	w, h := (ancho-48-16)/2, 150
	for i, t := range opcionesMenu {
		x := 24 + (i%2)*(w+16)
		y := 78 + (i/2)*(h+16)
		r := image.Rect(x, y, x+w, y+h)
		l.redondeado(r, 18, colPanel)
		if i == a.menuSel {
			l.marcoRedondo(r.Inset(-3), 21, 3, colTexto)
		}
		l.circuloSuave(r.Min.X+44, r.Min.Y+46, 26, colVerdeLi)
		l.centrado(image.Rect(r.Min.X+18, r.Min.Y+20, r.Min.X+70, r.Min.Y+72), iconos[i], a.f.iconoGrande, colTexto)
		l.izquierda(image.Rect(r.Min.X+18, r.Min.Y+84, r.Max.X-12, r.Min.Y+110), recortar(l.f.titulo2, t, w-30), l.f.titulo2, colTexto)
		for k, ln := range ajustar(l.f.pequena, subs[i], w-30) {
			if k < 2 {
				l.izquierda(image.Rect(r.Min.X+18, r.Min.Y+112+k*19, r.Max.X-12, r.Min.Y+130+k*19), ln, l.f.pequena, colSuave)
			}
		}
	}
	a.ayudaAbajo("A  Entrar      SELECT  Tu cuenta de Lichess      B  Salir")
}

func (a *app) dibujarRival() {
	l := a.l
	a.cabecera("¿A quién retas?", "Los últimos a los que has retado y a quien sigues en Lichess")
	items := append([]string{"Escribir un nombre…"}, a.lista...)
	ini := max(0, min(a.listaSel-4, len(items)-8))
	for i := ini; i < len(items) && i < ini+8; i++ {
		y := 76 + (i-ini)*44
		r := image.Rect(24, y, ancho-24, y+38)
		if i == a.listaSel {
			l.redondeado(r, 12, colTexto)
		} else {
			l.redondeado(r, 12, colPanel)
		}
		c := color.RGBA(colTexto)
		if i == a.listaSel {
			c = color.RGBA{0x12, 0x14, 0x18, 0xff}
		}
		ic := icUsuario
		if i == 0 {
			ic = ""
		}
		l.texto(r.Min.X+16, baseCentrada(r, a.f.icono), ic, a.f.icono, c)
		l.izquierda(image.Rect(r.Min.X+48, r.Min.Y, r.Max.X-16, r.Max.Y), items[i], l.f.nombre, c)
	}
	if a.cargando {
		l.centrado(image.Rect(0, alto-74, ancho, alto-46), "Buscando a quién sigues"+puntos(), l.f.pequena, colSuave)
	}
	a.ayudaAbajo("A  Elegir      B  Volver")
}

func (a *app) dibujarEscribir() {
	l := a.l
	a.cabecera("Nombre de tu amigo en Lichess", "START cuando esté · SELECT vuelve")
	caja := image.Rect(22, 74, ancho-22, 128)
	a.translucido(caja, 14, 50)
	t := a.tec
	txt := string(t.borrador[:t.cursor]) + "|" + string(t.borrador[t.cursor:])
	col := color.Color(colTexto)
	if len(t.borrador) == 0 {
		txt, col = "|  Su nombre de usuario", colApagado
	}
	l.izquierda(caja.Inset(14), txt, l.f.texto, col)
	y0 := 146
	for f := 0; f < 4; f++ {
		for c, r := range []rune(capas[t.capa][f]) {
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
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Listo"}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*56, x+w-6, y0+4*56+48)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 12, colVerdeLi)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 12, 38)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Letra    B  Borrar    X  Letras    START  Listo")
}

func (a *app) dibujarOpciones() {
	l := a.l
	if a.contraMaquina {
		a.cabecera("Contra la máquina", "La de Lichess: Stockfish, del nivel 1 (empezando) al 8 (muy fuerte)")
	} else {
		a.cabecera("Retar a "+a.rival, "Le llega el reto en Lichess (en el móvil o en la web)")
	}
	filas := a.filasOpciones()
	y := 90
	for i, f := range filas {
		r := image.Rect(60, y, ancho-60, y+56)
		if f == "ir" {
			r = image.Rect(ancho/2-130, y+16, ancho/2+130, y+70)
			l.boton(r, colVerdeLi, colVerdeOsc)
			txt := "RETAR"
			if a.contraMaquina {
				txt = "EMPEZAR"
			}
			l.centrado(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Max.Y-4), txt, l.f.titulo2, colTexto)
			if i == a.opSel {
				l.marcoRedondo(r.Inset(-4), 16, 3, colTexto)
			}
			break
		}
		l.redondeado(r, 14, colPanel)
		if i == a.opSel {
			l.marcoRedondo(r.Inset(-2), 16, 3, colTexto)
		}
		nombre, valor := "", ""
		switch f {
		case "nivel":
			nombre, valor = "Nivel", itoa(a.nivel)+" de 8"
		case "ritmo":
			nombre, valor = "Tiempo", ritmos[a.ritmoSel].Nombre
			if ritmos[a.ritmoSel].Dias == 0 {
				valor += " minutos"
			}
		case "color":
			nombre, valor = "Juegas con", nombresColor[a.colorSel]
		case "puntua":
			nombre, valor = "Cuenta para tu puntuación", map[bool]string{true: "Sí", false: "No"}[a.puntua]
		}
		l.izquierda(image.Rect(r.Min.X+20, r.Min.Y, r.Max.X, r.Max.Y), nombre, l.f.nombre, colSuave)
		l.derecha(image.Rect(r.Min.X, r.Min.Y, r.Max.X-50, r.Max.Y), valor, l.f.titulo2, colTexto)
		l.texto(r.Max.X-38, baseCentrada(r, a.f.icono), "", a.f.iconoPeq, colApagado)
		l.texto(r.Max.X-22, baseCentrada(r, a.f.icono), "", a.f.iconoPeq, colApagado)
		y += 68
	}
	a.ayudaAbajo("ARRIBA ABAJO  Elegir    IZQ DER  Cambiar    START  " + map[bool]string{true: "Empezar", false: "Retar"}[a.contraMaquina] + "    B  Volver")
}

func (a *app) dibujarEsperando() {
	l := a.l
	a.logo(ancho/2-45, 120, 90)
	msg := "Empezando la partida" + puntos()
	if a.esperandoA != "" {
		msg = "Esperando a que " + a.esperandoA + " acepte" + puntos()
	}
	l.centrado(image.Rect(0, 236, ancho, 270), msg, l.f.titulo2, colTexto)
	if a.esperandoA != "" {
		for i, ln := range ajustar(l.f.pequena, "Le ha llegado el reto en Lichess. En cuanto acepte, empieza la partida aquí.", ancho-140) {
			l.centrado(image.Rect(70, 280+i*22, ancho-70, 302+i*22), ln, l.f.pequena, colSuave)
		}
	}
	a.ayudaAbajo("B  Cancelar")
}

func (a *app) dibujarEmpezadas() {
	l := a.l
	a.cabecera("Partidas empezadas", "Las que tienes en marcha en Lichess")
	if len(a.empezadas) == 0 {
		msg := "No tienes ninguna partida en marcha."
		if a.cargando {
			msg = "Mirando" + puntos()
		}
		l.centrado(image.Rect(0, 220, ancho, 250), msg, l.f.texto, colSuave)
	}
	ini := max(0, min(a.empSel-3, len(a.empezadas)-6))
	for i := ini; i < len(a.empezadas) && i < ini+6; i++ {
		p := a.empezadas[i]
		y := 76 + (i-ini)*62
		r := image.Rect(24, y, ancho-24, y+54)
		l.redondeado(r, 14, colPanel)
		if i == a.empSel {
			l.marcoRedondo(r.Inset(-2), 16, 3, colTexto)
		}
		quien := p.Rival
		if p.RivalElo > 0 && p.Maquina == 0 {
			quien += fmt.Sprintf(" (%d)", p.RivalElo)
		}
		l.izquierda(image.Rect(r.Min.X+18, r.Min.Y+4, r.Max.X-150, r.Min.Y+30), recortar(l.f.nombre, quien, r.Dx()-180), l.f.nombre, colTexto)
		color := "con negras"
		if p.Blancas {
			color = "con blancas"
		}
		l.izquierda(image.Rect(r.Min.X+18, r.Min.Y+28, r.Max.X-150, r.Max.Y-4), velocidad(p.Velocidad)+" · "+color, l.f.mini, colSuave)
		if p.TeToca {
			l.derecha(image.Rect(r.Min.X, r.Min.Y, r.Max.X-18, r.Max.Y), "Te toca", l.f.nombre, colOro)
		} else {
			l.derecha(image.Rect(r.Min.X, r.Min.Y, r.Max.X-18, r.Max.Y), "Le toca", l.f.normal, colApagado)
		}
	}
	a.ayudaAbajo("A  Seguir la partida      Y  Actualizar      B  Volver")
}

func velocidad(s string) string {
	switch s {
	case "bullet":
		return "Bala"
	case "blitz":
		return "Blitz"
	case "rapid":
		return "Rápida"
	case "classical":
		return "Clásica"
	case "correspondence":
		return "Por días"
	}
	return s
}

// ---- Partida -------------------------------------------------------------------------------------

func reloj(ms int) string {
	if ms >= 3600*1000 {
		return fmt.Sprintf("%d:%02d:%02d", ms/3600000, ms/60000%60, ms/1000%60)
	}
	if ms < 10000 {
		return fmt.Sprintf("%d:%02d.%d", ms/60000, ms/1000%60, ms/100%10)
	}
	return fmt.Sprintf("%d:%02d", ms/60000, ms/1000%60)
}

func (a *app) cajaJugador(r image.Rectangle, j jugador, ms int, conReloj, leToca bool) {
	l := a.l
	l.redondeado(r, 14, colPanel)
	if leToca {
		l.marcoRedondo(r, 14, 2, colVerdeLi)
	}
	nombre := j.Nombre
	if j.Elo > 0 && j.Maquina == 0 {
		nombre += " " + itoa(j.Elo)
	}
	l.izquierda(image.Rect(r.Min.X+12, r.Min.Y+4, r.Max.X-8, r.Min.Y+26), recortar(l.f.pequena, nombre, r.Dx()-20), l.f.pequena, colSuave)
	if conReloj {
		rc := image.Rect(r.Min.X+8, r.Min.Y+26, r.Max.X-8, r.Max.Y-6)
		fondo := color.RGBA{0x26, 0x2a, 0x30, 0xff}
		if leToca {
			fondo = colTexto
		}
		l.redondeado(rc, 8, fondo)
		c := color.RGBA(colTexto)
		if leToca {
			c = color.RGBA{0x12, 0x14, 0x18, 0xff}
			if ms < 20000 {
				c = colRojoLi
			}
		}
		l.centrado(rc, reloj(ms), l.f.grande, c)
	} else if leToca {
		l.izquierda(image.Rect(r.Min.X+12, r.Min.Y+28, r.Max.X-8, r.Max.Y-6), "Le toca", l.f.nombre, colVerdeLi)
	}
}

func (a *app) dibujarPartida() {
	l := a.l
	p := a.pa
	if p == nil || p.v == nil {
		l.centrado(image.Rect(0, 220, ancho, 250), "Abriendo la partida"+puntos(), l.f.texto, colTexto)
		a.ayudaAbajo("B  Volver")
		return
	}
	a.dibujarTablero(p.v, !p.terminada() && !a.menuPartida)
	x0 := tabX + 8*lado + 16
	wb, wn := p.relojes()
	yo, rival := p.blanco, p.negro
	msYo, msRival := wb, wn
	if !p.v.conBlancas {
		yo, rival, msYo, msRival = p.negro, p.blanco, wn, wb
	}
	tocaBlancas := p.tabLichess.blancas
	leTocaRival := !p.terminada() && tocaBlancas != p.v.conBlancas
	a.cajaJugador(image.Rect(x0, 46, ancho-12, 46+80), rival, msRival, p.conReloj, leTocaRival)
	a.cajaJugador(image.Rect(x0, alto-48-80, ancho-12, alto-48), yo, msYo, p.conReloj, !p.terminada() && !leTocaRival)

	// en medio: las jugadas (las últimas) y cómo va
	medio := image.Rect(x0, 134, ancho-12, alto-56-80)
	l.redondeado(medio, 14, colPanel)
	var lineas []string
	for i := 0; i < len(p.san); i += 2 {
		s := itoa(i/2+1) + ". " + p.san[i]
		if i+1 < len(p.san) {
			s += "  " + p.san[i+1]
		}
		lineas = append(lineas, s)
	}
	estado := ""
	switch {
	case p.terminada():
	case p.tablasRival:
		estado = "Te ofrece tablas: START"
	case p.enviando:
		estado = "Mandando" + puntos()
	case !leTocaRival:
		estado = "Te toca"
		if p.v.tab.enJaque() {
			estado = "¡Jaque! Te toca"
		}
	default:
		estado = "Piensa el rival" + puntos()
	}
	cab := 4
	if estado != "" {
		c := colSuave
		if strings.HasPrefix(estado, "Te toca") || strings.HasPrefix(estado, "¡Jaque") {
			c = colVerdeLi
		}
		if p.tablasRival {
			c = colOro
		}
		l.izquierda(image.Rect(medio.Min.X+12, medio.Min.Y+6, medio.Max.X-8, medio.Min.Y+30), estado, l.f.nombre, c)
		cab = 32
	}
	if p.rivalFuera && !p.terminada() {
		l.izquierda(image.Rect(medio.Min.X+12, medio.Min.Y+cab, medio.Max.X-8, medio.Min.Y+cab+20), "El rival se ha ido", l.f.mini, colOro)
		cab += 20
	}
	caben := (medio.Dy() - cab - 8) / 20
	ini := max(0, len(lineas)-caben)
	for i := ini; i < len(lineas); i++ {
		l.izquierda(image.Rect(medio.Min.X+12, medio.Min.Y+cab+(i-ini)*20, medio.Max.X-8, medio.Min.Y+cab+(i-ini)*20+20), lineas[i], l.f.pequena, colTexto)
	}
	if p.terminada() {
		a.dibujarFinPartida(p)
		return
	}
	if a.menuPartida {
		a.dibujarMenuPartida()
		return
	}
	a.ayudaAbajo("A  Coger y dejar      B  Soltar      START  Tablas, rendirse, salir")
}

func (a *app) dibujarFinPartida(p *partidaViva) {
	l := a.l
	titulo, sub := p.textoFinal()
	caja := image.Rect(tabX+30, tabY+120, tabX+8*lado-30, tabY+260)
	c := colOro
	if titulo == "Has perdido" {
		c = colRojoLi
	} else if titulo == "Tablas" || titulo == "Partida anulada" {
		c = colSuave
	}
	l.redondeado(caja, 18, color.RGBA{0x10, 0x12, 0x14, 0xf0})
	l.marcoRedondo(caja, 18, 2, c)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+20, caja.Max.X, caja.Min.Y+60), titulo, l.f.grande, c)
	l.centrado(image.Rect(caja.Min.X, caja.Min.Y+66, caja.Max.X, caja.Min.Y+92), sub, l.f.texto, colTexto)
	l.centrado(image.Rect(caja.Min.X, caja.Max.Y-36, caja.Max.X, caja.Max.Y-12), "A  Volver al menú", l.f.pequena, colSuave)
	a.ayudaAbajo("A  Volver al menú")
}

func (a *app) dibujarMenuPartida() {
	l := a.l
	ops := a.opcionesPartida()
	caja := image.Rect(tabX+40, tabY+70, tabX+8*lado-40, tabY+90+len(ops)*48)
	l.redondeado(caja, 16, color.RGBA{0x10, 0x12, 0x14, 0xf2})
	for i, o := range ops {
		r := image.Rect(caja.Min.X+12, caja.Min.Y+12+i*48, caja.Max.X-12, caja.Min.Y+52+i*48)
		if i == a.mpSel {
			l.redondeado(r, 10, colTexto)
			l.centrado(r, o, l.f.nombre, colFondo)
		} else {
			l.centrado(r, o, l.f.nombre, colTexto)
		}
	}
	a.ayudaAbajo("A  Elegir      B  Seguir jugando")
}

// ---- Problemas -----------------------------------------------------------------------------------

func (a *app) dibujarProblema() {
	l := a.l
	pz := a.pz
	if pz == nil || pz.v == nil {
		msg := "Buscando un problema" + puntos()
		if pz != nil && pz.estado == "error" {
			msg = pz.err
		}
		y := 220
		for _, ln := range ajustar(l.f.texto, msg, ancho-100) {
			l.centrado(image.Rect(40, y, ancho-40, y+28), ln, l.f.texto, colTexto)
			y += 28
		}
		a.ayudaAbajo("A  Otro      B  Volver")
		return
	}
	a.dibujarTablero(pz.v, pz.estado == "jugando")
	x0 := tabX + 8*lado + 16
	r := image.Rect(x0, 46, ancho-12, 250)
	l.redondeado(r, 14, colPanel)
	l.izquierda(image.Rect(r.Min.X+12, r.Min.Y+8, r.Max.X-8, r.Min.Y+34), "Problema", l.f.titulo2, colTexto)
	quien := "Juegas con blancas"
	if !pz.v.conBlancas {
		quien = "Juegas con negras"
	}
	l.izquierda(image.Rect(r.Min.X+12, r.Min.Y+38, r.Max.X-8, r.Min.Y+60), quien, l.f.pequena, colSuave)
	l.izquierda(image.Rect(r.Min.X+12, r.Min.Y+60, r.Max.X-8, r.Min.Y+82), "Dificultad: "+itoa(pz.pr.Elo), l.f.pequena, colSuave)
	if tema := temaProblema(pz.pr.Temas); tema != "" {
		for i, ln := range ajustar(l.f.pequena, tema, r.Dx()-24) {
			if i < 3 {
				l.izquierda(image.Rect(r.Min.X+12, r.Min.Y+90+i*20, r.Max.X-8, r.Min.Y+110+i*20), ln, l.f.pequena, colOro)
			}
		}
	}
	l.izquierda(image.Rect(r.Min.X+12, r.Max.Y-30, r.Max.X-8, r.Max.Y-8), fmt.Sprintf("Hoy: %d bien, %d mal", pz.resueltos, pz.fallados), l.f.mini, colApagado)
	est := image.Rect(x0, 262, ancho-12, alto-48)
	l.redondeado(est, 14, colPanel)
	switch pz.estado {
	case "jugando":
		msg := "Encuentra la mejor jugada"
		if !pz.rivalEn.IsZero() {
			msg = "Contesta el rival" + puntos()
		} else if pz.paso > 0 {
			msg = "¡Bien! Sigue"
		}
		for i, ln := range ajustar(l.f.nombre, msg, est.Dx()-24) {
			l.izquierda(image.Rect(est.Min.X+12, est.Min.Y+12+i*24, est.Max.X-8, est.Min.Y+36+i*24), ln, l.f.nombre, colVerdeLi)
		}
		a.ayudaAbajo("A  Coger y dejar      Y  Pista      B  Volver")
	case "bien":
		l.izquierda(image.Rect(est.Min.X+12, est.Min.Y+12, est.Max.X-8, est.Min.Y+44), "¡Resuelto!", l.f.grande, colVerdeLi)
		a.ayudaAbajo("A  Otro problema      B  Volver")
	case "mal":
		l.izquierda(image.Rect(est.Min.X+12, est.Min.Y+12, est.Max.X-8, est.Min.Y+40), "No era esa", l.f.titulo2, colRojoLi)
		for i, ln := range ajustar(l.f.pequena, "La buena: "+pz.buena, est.Dx()-24) {
			l.izquierda(image.Rect(est.Min.X+12, est.Min.Y+46+i*20, est.Max.X-8, est.Min.Y+66+i*20), ln, l.f.pequena, colTexto)
		}
		a.ayudaAbajo("A  Otro problema      Y  Otra vez      B  Volver")
	}
}

// temaProblema: los temas de Lichess, en español (los que más salen).
func temaProblema(ts []string) string {
	nombres := map[string]string{"mateIn1": "Mate en 1", "mateIn2": "Mate en 2", "mateIn3": "Mate en 3", "fork": "Ataque doble",
		"pin": "Clavada", "skewer": "Pincho", "hangingPiece": "Pieza colgada", "sacrifice": "Sacrificio",
		"discoveredAttack": "Ataque a la descubierta", "endgame": "Final", "middlegame": "Medio juego", "opening": "Apertura",
		"backRankMate": "Mate en la última fila", "deflection": "Desviación", "attraction": "Atracción", "promotion": "Coronar",
		"short": "", "long": "", "veryLong": "", "oneMove": "", "crushing": "", "advantage": "", "equality": "", "master": ""}
	var out []string
	for _, t := range ts {
		if n, ok := nombres[t]; ok {
			if n != "" {
				out = append(out, n)
			}
		}
	}
	return strings.Join(out, " · ")
}
