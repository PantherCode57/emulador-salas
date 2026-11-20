package main

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
)

// Las pantallas copian las vistas previas aprobadas (scratchpad/vista_pokemon.py): la columna de
// tarjetas de las listas de juegos del menú, filas blancas transparentes, el borde azul en lo
// elegido y el verde para lo que está abierto o conectado.

type portada struct {
	img *image.RGBA
	sb  *image.Alpha
}

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga("")
		return
	}
	switch a.pant {
	case pJuegos:
		a.dibujarJuegos()
	case pSalas:
		a.dibujarSalas()
	case pPartida:
		a.dibujarPartida()
	case pTeclado:
		a.dibujarTeclado()
	case pSala:
		if a.activa != nil && a.activa.Estado().Login != "" {
			a.dibujarLogin(a.activa.Estado().Login)
		} else {
			a.dibujarSala()
		}
	case pConfirmar:
		a.pant = a.confirmVolver
		a.dibujar()
		a.pant = pConfirmar
		a.dibujarConfirmar()
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

func (a *app) dibujarCarga(texto string) {
	l := a.l
	l.limpiarFondo()
	r := image.Rect(ancho/2-75, alto/2-120, ancho/2+75, alto/2+30)
	l.redondeado(r, 22, colRojo)
	l.centrado(r, icUsuarios, l.f.iconoGrande, colTexto)
	l.centrado(image.Rect(0, alto/2+46, ancho, alto/2+82), "Pokémon", l.f.grande, colTexto)
	if texto != "" {
		l.centrado(image.Rect(0, alto/2+86, ancho, alto/2+112), texto, l.f.pequena, colSuave)
	}
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Pokemulador · Panther OS", l.f.mini, colSuave)
}

// hora: ya no se pinta; la hora ya sale en el recuadro de arriba de la consola.
func (a *app) hora(grande bool) {}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	l.centrado(image.Rect(0, alto-36, ancho, alto-8), s, l.f.pequena, colAyuda)
}

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	r := image.Rect((ancho-w)/2, alto-84, (ancho+w)/2, alto-52)
	l.redondeado(r, 16, color.RGBA{0, 0, 0, 0xd0})
	l.centrado(r, s, l.f.pequena, colTexto)
}

// cabecera: el icono rojo de las salas y el título.
func (a *app) cabecera(titulo string) {
	l := a.l
	l.medio(28, 40, icUsuarios, l.f.iconoGrande, colRojo)
	l.medio(66, 40, recortar(l.f.grande, titulo, ancho-66-28), l.f.grande, colTexto)
	a.hora(false)
}

// tarjetaDe: la tarjeta de la lista del juego (o una de su color con el nombre), al tamaño pedido.
func (a *app) tarjetaDe(j *Juego, w, h int) *portada {
	clave := fmt.Sprintf("%s|%d", j.Relativa, w)
	if p, ok := a.tarjetas[clave]; ok {
		return p
	}
	var p *portada
	if im := abrirImagen(j.Tarjeta); im != nil {
		p = &portada{img: escalar(im, w, h, false)}
	} else {
		c := sistemas[j.Sistema].color
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		tmp := &lienzo{img: img, f: a.l.f}
		margen := w * 16 / 267
		tmp.redondeado(image.Rect(margen, margen*3/4, w-margen, h-margen*11/8), w*14/267, color.RGBA{c[0], c[1], c[2], 255})
		tmp.centrado(image.Rect(margen, margen*3/4, w-margen, h-margen*11/8), j.Titulo, a.l.f.nombre, colTexto)
		p = &portada{img: img}
	}
	if len(a.tarjetas) > 60 {
		a.tarjetas = map[string]*portada{}
	}
	a.tarjetas[clave] = p
	return p
}

// ---- Juegos -------------------------------------------------------------------------------------

func (a *app) dibujarJuegos() {
	l := a.l
	a.hora(true)
	if len(a.juegos) == 0 {
		l.centrado(image.Rect(0, 200, ancho, 240), "No hay juegos de Game Boy, Game Boy Color ni GBA en la consola", l.f.normal, colApagado)
		a.ayudaAbajo("B  Salir")
		return
	}
	a.selJuego = min(max(a.selJuego, 0), len(a.juegos)-1)
	for rel := -2; rel <= 2; rel++ {
		i := a.selJuego + rel
		if i < 0 || i >= len(a.juegos) {
			continue
		}
		sel := rel == 0
		w, h := 238, 148
		if sel {
			w, h = 267, 166
		}
		cx, cy := 129, 251+rel*164
		p := a.tarjetaDe(&a.juegos[i], w, h)
		l.pegar(p.img, nil, cx-w/2, cy-h/2)
		if sel {
			// el borde, 4 px por fuera de la tarjeta (que va con 16/12 px de margen para la sombra)
			l.marcoRedondo(image.Rect(cx-w/2+12, cy-h/2+8, cx+w/2-12, cy+h/2-18), 17, 3, colMarco)
		}
		cara, col := l.f.mini, color.Color(colEtiq)
		dy := 141
		if sel {
			cara, col, dy = l.f.etiqueta, colTexto, 156
		}
		l.centrado(image.Rect(cx-120, cy-h/2+dy-11, cx+120, cy-h/2+dy+11), a.juegos[i].Titulo, cara, col)
	}
	j := a.juegos[a.selJuego]
	const x0 = 290
	w := ancho - 24 - x0
	l.arriba(x0, 70, recortar(l.f.titulo, j.Titulo, w), l.f.titulo, colTexto)
	l.arriba(x0, 104, recortar(l.f.pequena, j.genero(), w), l.f.pequena, colAzul)
	l.rect(image.Rect(x0, 130, ancho-24, 131), color.RGBA{70, 70, 70, 70})
	y := 142
	if esRojoFuego(j) {
		estado, col := "no", color.Color(colApagado)
		if a.caramelos[j.Relativa] {
			estado, col = "sí", colVerde
		}
		l.arriba(x0, y, "Caramelos Raros infinitos: "+estado, l.f.chica, col)
		y += 24
	}
	l.arriba(x0, y, "TUS SALAS DE ESTE JUEGO", l.f.mini, colApagado)
	y += 22
	ss := a.salasDe(&j)
	for k := 0; k < len(ss) && k < 3; k++ {
		r := image.Rect(x0, y, ancho-24, y+50)
		l.translucido(r, 12, 34, false)
		l.medio(x0+14, y+17, recortar(l.f.fila, ss[k].Nombre, w-40), l.f.fila, colTexto)
		sub := codigoVisible(ss[k].Codigo)
		if ss[k].PartidaPropia {
			sub += "  ·  partida propia"
		}
		if n := len(progresosAmigos(ss[k].Codigo, a.jug.ID)); n > 0 {
			sub += fmt.Sprintf("  ·  %d amigos", n)
		}
		l.medio(x0+14, y+36, sub, l.f.mini, colSuave)
		if a.sala != nil && a.sala.Codigo == ss[k].Codigo {
			l.disco(float64(ancho-42), float64(y+25), 4, colVerde)
		}
		y += 58
	}
	if len(ss) == 0 {
		for _, ln := range ajustar(l.f.chica, "Pulsa Y para crear una sala o unirte con un código.", w) {
			l.arriba(x0, y, ln, l.f.chica, colApagado)
			y += 20
		}
	}
	ayuda := "A  Jugar solo      Y  Salas      B  Salir"
	if esRojoFuego(j) {
		ayuda = "A  Jugar solo      Y  Salas      X  Caramelos      B  Salir"
	}
	l.ayuda(ayuda, x0)
}

// ---- Salas de un juego --------------------------------------------------------------------------

func (a *app) dibujarSalas() {
	l := a.l
	j := a.juegoElegido()
	if j == nil {
		return
	}
	a.cabecera("Salas")
	l.medio(28, 78, recortar(l.f.pequena, j.Titulo, ancho-56), l.f.pequena, colAzul)
	type fila struct{ icono, nombre, sub string }
	filas := []fila{
		{icMas, "Crear una sala nueva", "Te da un código de 6 letras para que tus amigos entren"},
		{icEntrar, "Unirme con un código", "Escribe el código de la sala de tu amigo"},
	}
	for _, s := range a.salasDe(j) {
		sub := codigoVisible(s.Codigo)
		if s.PartidaPropia {
			sub += "  ·  partida propia de la sala"
		} else {
			sub += "  ·  tu partida de siempre"
		}
		filas = append(filas, fila{icUsuarios, s.Nombre, sub})
	}
	a.selOpcion = min(max(a.selOpcion, 0), len(filas)-1)
	primera := max(0, a.selOpcion-4)
	y := 104
	for k := primera; k < len(filas) && y < alto-80; k++ {
		r := image.Rect(24, y, ancho-24, y+56)
		sel := k == a.selOpcion
		alfa := uint8(30)
		if sel {
			alfa = 60
		}
		l.translucido(r, 14, alfa, sel)
		l.centrado(image.Rect(34, y, 80, y+56), filas[k].icono, l.f.icono, colTexto)
		l.medio(90, y+19, recortar(l.f.nombre, filas[k].nombre, ancho-140), l.f.nombre, colTexto)
		l.medio(90, y+39, recortar(l.f.mini, filas[k].sub, ancho-140), l.f.mini, colSuave)
		y += 64
	}
	ayuda := "A  Elegir      B  Volver"
	if a.selOpcion >= 2 {
		ayuda = "A  Entrar      X  Borrar      B  Volver"
	}
	a.ayudaAbajo(ayuda)
}

func (a *app) dibujarPartida() {
	l := a.l
	j := a.juegoElegido()
	if j == nil {
		return
	}
	titulo := "Sala nueva"
	if !a.crear {
		titulo = "Sala " + codigoVisible(a.codigo)
	}
	a.cabecera(titulo)
	l.medio(28, 78, "¿Con qué partida de "+recortar(l.f.pequena, j.Titulo, 300)+" vas a jugar?", l.f.pequena, colAzul)
	ops := [][2]string{
		{"Tu partida de siempre", "La misma que al jugar solo y desde el menú: lo que avances en la sala se queda ahí."},
		{"Una partida nueva para la sala", "Empiezas de cero y solo se usa en esta sala. Tu partida de siempre no se toca."},
	}
	y := 110
	for k, op := range ops {
		r := image.Rect(24, y, ancho-24, y+86)
		sel := k == a.selPartida
		alfa := uint8(30)
		if sel {
			alfa = 60
		}
		l.translucido(r, 14, alfa, sel)
		l.medio(44, y+24, op[0], l.f.nombre, colTexto)
		yy := y + 42
		for _, ln := range ajustar(l.f.chica, op[1], ancho-110) {
			l.arriba(44, yy, ln, l.f.chica, colSuave)
			yy += 19
		}
		y += 100
	}
	a.ayudaAbajo("A  Elegir      B  Volver")
}

// ---- Teclado ------------------------------------------------------------------------------------

func (a *app) dibujarTeclado() {
	l := a.l
	titulo, ayudaVacia := "Tu nombre en las salas", "Cómo te verán tus amigos"
	if a.tecPara == "codigo" {
		titulo, ayudaVacia = "Código de la sala", "Las 6 letras de la sala de tu amigo"
	}
	a.cabecera(titulo)
	caja := image.Rect(24, 74, ancho-24, 122)
	l.translucido(caja, 14, 50, false)
	t := a.tec
	texto := string(t.borrador[:t.cursor]) + "|" + string(t.borrador[t.cursor:])
	col := color.Color(colTexto)
	if len(t.borrador) == 0 {
		texto, col = "|  "+ayudaVacia, colApagado
	}
	if a.tecPara == "codigo" {
		texto = strings.ToUpper(texto)
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
				l.translucido(k, 12, 38, false)
				l.centrado(k, string(r), l.f.nombre, colTexto)
			}
		}
	}
	anchos := []int{110, 254, 120, 126}
	nombres := []string{[...]string{"ABC", "?123", "abc"}[t.capa], "espacio", "borrar", "Aceptar"}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*58, x+w-6, y0+4*58+50)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		case i == 3:
			l.redondeado(k, 12, colVerde)
			l.centrado(k, nombres[i], l.f.nombre, colFondo)
		default:
			l.translucido(k, 12, 38, false)
			l.centrado(k, nombres[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir      B  Borrar      START  Aceptar      SELECT  Volver")
}

// ---- Sala ---------------------------------------------------------------------------------------

func colorDe(s string) color.RGBA {
	paleta := []color.RGBA{{91, 79, 207, 255}, {20, 160, 150, 255}, {230, 110, 40, 255}, {214, 51, 132, 255},
		{59, 130, 246, 255}, {34, 160, 90, 255}, {124, 58, 237, 255}}
	h := fnv.New32a()
	h.Write([]byte(s))
	return paleta[h.Sum32()%uint32(len(paleta))]
}

func horasTexto(ms int64) string {
	m := ms / 60000
	if m < 60 {
		return fmt.Sprintf("%d min jugados", m)
	}
	return fmt.Sprintf("%d h %d min jugados", m/60, m%60)
}

func haceTexto(ms int64) string {
	if ms <= 0 {
		return "Aún no ha jugado"
	}
	d := time.Since(time.UnixMilli(ms))
	switch {
	case d < 3*time.Minute:
		return "Jugando"
	case d < time.Hour:
		return fmt.Sprintf("Hace %d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("Hace %d h", int(d.Hours()))
	default:
		return fmt.Sprintf("Hace %d días", int(d.Hours()/24))
	}
}

func (a *app) dibujarSala() {
	l := a.l
	if a.sala == nil || a.activa == nil {
		return
	}
	s := a.sala
	e := a.activa.Estado()
	a.cabecera(s.Nombre)
	pre := s.JuegoTitulo + "  ·  código "
	x := l.medio(28, 78, recortar(l.f.pequena, pre, 330), l.f.pequena, colAzul)
	l.medio(x, 78, codigoVisible(s.Codigo), l.f.semi15, colTexto)
	// Cómo va la conexión.
	estado, col := "Conectando…", color.Color(colAmbar)
	switch {
	case e.Fase == "error":
		estado, col = recortar(l.f.chica, e.Error, 240), colAviso
	case e.RolDecidido && e.Anfitrion:
		estado, col = fmt.Sprintf("Sala abierta  ·  %d amigos", e.Amigos), colVerde
	case e.RolDecidido && e.Ruta != "":
		estado, col = "Tailscale  ·  "+e.Ruta, colVerde
	case e.RolDecidido:
		estado, col = "Conectado", colVerde
	case e.Fase == "buscando":
		estado = "Buscando la sala…"
	}
	xr := l.medioDerecha(ancho-24, 78, estado, l.f.chica, col)
	l.medioDerecha(xr-8, 78, icWifi, l.f.iconoPeq, col)

	// Tú y tus amigos.
	type miembro struct {
		nombre, sub, estado string
		jugando             bool
	}
	yo := leerProgreso(archivoPropio(s.Codigo))
	var ms []miembro
	sub := "En la sala"
	if yo != nil {
		sub = horasTexto(yo.JugadoMs)
	}
	ms = append(ms, miembro{a.jug.Nombre + " (tú)", sub, "En la sala", true})
	for _, p := range progresosAmigos(s.Codigo, a.jug.ID) {
		h := haceTexto(p.UltimaVez)
		juego := p.Juego
		if p.Partida == "sala" {
			juego += " · partida de la sala"
		}
		ms = append(ms, miembro{p.Nombre, horasTexto(p.JugadoMs) + "  ·  " + juego, h, h == "Jugando"})
	}
	y := 104
	for k, m := range ms {
		if y > 104+3*64 {
			l.arriba(40, y, fmt.Sprintf("y %d más", len(ms)-k), l.f.mini, colApagado)
			break
		}
		r := image.Rect(24, y, ancho-24, y+56)
		l.translucido(r, 14, 30, false)
		c := colorDe(m.nombre)
		l.disco(58, float64(y+28), 18, c)
		ini := strings.ToUpper(string([]rune(m.nombre)[:1]))
		l.centrado(image.Rect(40, y+10, 76, y+46), ini, l.f.inicial, colTexto)
		wEst := medir(l.f.chica, m.estado)
		l.medio(90, y+19, recortar(l.f.nombre, m.nombre, ancho-150-wEst), l.f.nombre, colTexto)
		l.medio(90, y+39, recortar(l.f.mini, m.sub, ancho-150-wEst), l.f.mini, colSuave)
		ce := color.Color(colApagado)
		if m.jugando {
			ce = colVerde
		}
		l.medioDerecha(ancho-40, y+28, m.estado, l.f.chica, ce)
		y += 64
	}
	if len(ms) == 1 {
		l.arriba(40, y+4, "Aún no ha entrado nadie. Pasa el código "+codigoVisible(s.Codigo)+" a tus amigos.", l.f.chica, colApagado)
		y += 28
	}

	// Qué se puede hacer juntos.
	titulo, texto := "Combates e intercambios por cable link",
		"Entrad los dos al juego desde la sala y id al Centro Pokémon, como con el cable de verdad."
	if s.Sistema == "gba" {
		titulo = "Os veréis por el mapa"
		texto = "Rojo Fuego con Verde Hoja; Rubí, Zafiro y Esmeralda entre ellos. Hablad con el otro jugador para combatir o intercambiar."
		if _, gbapk := nucleoDe("gba"); !gbapk {
			texto = "Falta el mGBA de Pokemulador en la consola: sin él no os veréis por el mapa."
		}
	}
	caja := image.Rect(24, max(y+6, alto-140), ancho-24, alto-48)
	l.translucido(caja, 14, 26, false)
	l.centrado(image.Rect(34, caja.Min.Y+6, 66, caja.Min.Y+34), icIntercamb, l.f.icono, colTexto)
	l.medio(74, caja.Min.Y+20, titulo, l.f.semi15, colTexto)
	yy := caja.Min.Y + 36
	for _, ln := range ajustar(l.f.mini, texto, caja.Dx()-70) {
		l.arriba(74, yy, ln, l.f.mini, colSuave)
		yy += 17
	}
	a.ayudaAbajo("A  Jugar en la sala      X  Borrar sala      B  Salir de la sala")
}

// ---- Entrar en Tailscale (la primera vez) -------------------------------------------------------

func (a *app) dibujarLogin(url string) {
	l := a.l
	a.cabecera("Pokémon · salas")
	y := 100
	for _, ln := range []string{"Para jugar con tus amigos, la consola", "tiene que entrar en tu red de Tailscale.", "",
		"Escanea el código con el móvil y entra", "con tu cuenta. Solo hace falta la", "primera vez."} {
		l.arriba(28, y, ln, l.f.texto, colTexto)
		y += 26
	}
	l.arriba(28, y+18, "Esperando a que entres…", l.f.pequena, colSuave)
	if q, err := qrcode.New(url, qrcode.Medium); err == nil {
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
	a.ayudaAbajo("B  Volver      SELECT  Apagar la pantalla")
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
