package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

const (
	altoCabecera = 72
	margen       = 24
)

var panel = image.Rect(margen, altoCabecera+8, ancho-margen, alto-40)

func (a *app) dibujar() {
	a.sucio = false
	l := nuevoLienzo(a.f)
	l.rect(l.img.Bounds(), colFondo)
	a.cabecera(l)
	l.redondeado(panel, 24, colPanel, 0, nil)
	switch a.pant {
	case pInicio:
		a.dibujarInicio(l)
	case pSala:
		a.dibujarSala(l)
	case pJuegos:
		a.dibujarJuegos(l)
	case pPartida:
		a.dibujarPartida(l)
	case pTeclado:
		a.dibujarTeclado(l)
	case pMensaje:
		a.dibujarMensaje(l)
	case pConfirmar:
		a.dibujarConfirmar(l)
	}
	ayuda := "A o Start: aceptar     B: volver     F1 o mantener Start: menú de Lakka"
	if a.pant == pTeclado {
		ayuda = "A: escribir     B o Y: borrar     Start: aceptar"
	}
	l.centrado(image.Rect(0, alto-40, ancho, alto), ayuda, a.f.pequena, colBlanco)
	a.srv.enseñar(l.marco())
}

// dibujarAviso: una pantalla de espera (al abrir un juego).
func (a *app) dibujarAviso(texto string) {
	l := nuevoLienzo(a.f)
	l.rect(l.img.Bounds(), colFondo)
	a.cabecera(l)
	l.redondeado(panel, 24, colPanel, 0, nil)
	l.centrado(panel, texto, a.f.titulo, colTexto)
	a.srv.enseñar(l.marco())
}

func (a *app) cabecera(l *lienzo) {
	// Las luces de la Pokédex.
	l.redondeado(image.Rect(margen, 14, margen+44, 58), 22, colBlanco, 0, nil)
	l.redondeado(image.Rect(margen+5, 19, margen+39, 53), 17, colAzul, 0, nil)
	for i, c := range []color.Color{color.RGBA{0xe8, 0x8a, 0x8a, 0xff}, colAmarillo, color.RGBA{0x8b, 0xc3, 0x4a, 0xff}} {
		x := margen + 56 + i*20
		l.redondeado(image.Rect(x, 30, x+12, 42), 6, c, 0, nil)
	}
	l.texto(margen+124, 50, "Pokemulador", a.f.titulo, colBlanco)
	estado := "Sin Internet"
	if hayInternet() {
		estado = "Con Internet"
	}
	derecha := estado + "   ·   " + a.jug.Nombre
	l.texto(ancho-margen-medir(a.f.pequena, derecha), 46, derecha, a.f.pequena, colBlanco)
}

// lista con desplazamiento; devuelve el rectángulo de cada fila visible.
func (a *app) lista(l *lienzo, r image.Rectangle, filas []opcion) {
	const alturaFila, hueco = 50, 6
	visibles := max(1, (r.Dy()+hueco)/(alturaFila+hueco))
	if a.sel < a.desp {
		a.desp = a.sel
	}
	if a.sel >= a.desp+visibles {
		a.desp = a.sel - visibles + 1
	}
	for i := a.desp; i < len(filas) && i < a.desp+visibles; i++ {
		y := r.Min.Y + (i-a.desp)*(alturaFila+hueco)
		l.fila(image.Rect(r.Min.X, y, r.Max.X, y+alturaFila), filas[i].texto, filas[i].etiqueta, i == a.sel)
	}
	if a.desp > 0 {
		l.centrado(image.Rect(r.Max.X-40, r.Min.Y-22, r.Max.X, r.Min.Y), "▲", a.f.pequena, colSuave)
	}
	if a.desp+visibles < len(filas) {
		l.centrado(image.Rect(r.Max.X-40, r.Max.Y, r.Max.X, r.Max.Y+22), "▼", a.f.pequena, colSuave)
	}
}

func (a *app) dibujarInicio(l *lienzo) {
	ops := a.opcionesInicio()
	if a.sel >= len(ops) {
		a.sel = len(ops) - 1
	}
	a.lista(l, image.Rect(panel.Min.X+20, panel.Min.Y+22, 560, panel.Max.Y-20), ops)
	x := 590
	info := image.Rect(x, panel.Min.Y+22, panel.Max.X-22, panel.Max.Y-20)
	l.redondeado(info, 18, colTarjeta, 0, nil)
	o := ops[a.sel]
	y := l.parrafo(info.Min.X+18, info.Min.Y+4, info.Dx()-36, o.texto, a.f.negrita, colTexto)
	l.parrafo(info.Min.X+18, y+6, info.Dx()-36, o.ayuda, a.f.pequena, colSuave)
	abajo := fmt.Sprintf("Juegos en la memoria: %d", len(a.juegos))
	if len(a.juegos) == 0 {
		abajo = "No hay juegos: copia los tuyos a la parte JUEGOS de la memoria desde un ordenador."
	}
	lineas := ajustar(a.f.pequena, abajo, info.Dx()-36)
	yb := info.Max.Y - 18 - (len(lineas)-1)*(a.f.pequena.Metrics().Height.Round()+4)
	for _, ln := range lineas {
		l.texto(info.Min.X+18, yb, ln, a.f.pequena, colTexto)
		yb += a.f.pequena.Metrics().Height.Round() + 4
	}
}

func (a *app) textoEstadoSala() (string, color.Color) {
	if a.activa == nil {
		return "", colSuave
	}
	e := a.activa.Estado()
	switch {
	case e.Fase == "error":
		t := e.Error
		if !hayInternet() {
			t = "Sin Internet. Conéctate en el menú de Lakka (Ajustes › Wi-Fi)."
		}
		return t, colBoton
	case !e.RolDecidido && e.Fase == "conectando":
		return "Conectando…", colSuave
	case !e.RolDecidido:
		return "Buscando la sala…", colSuave
	case e.Anfitrion && e.Amigos > 0:
		if e.Amigos == 1 {
			return "Tienes la sala abierta · 1 amigo dentro", colVerde
		}
		return fmt.Sprintf("Tienes la sala abierta · %d amigos dentro", e.Amigos), colVerde
	case e.Anfitrion:
		return "Tienes la sala abierta · esperando a tus amigos", colTexto
	case e.Fase == "conectado":
		return "Estás en la sala de tu amigo", colVerde
	default:
		return "Buscando a tu amigo…", colSuave
	}
}

func ayudaSistema(sistema string) string {
	switch sistema {
	case "gba":
		return "En Rojo Fuego, Verde Hoja, Rubí, Zafiro y Esmeralda os veréis andando por el mapa (Rojo Fuego con Verde Hoja; Rubí, Zafiro y Esmeralda entre ellos). Se conecta solo al abrir el juego desde aquí."
	case "gb", "gbc":
		return "El cable link va por la sala: id los dos al Centro Pokémon, subid a la planta de arriba y hablad con la recepcionista para intercambiar o combatir."
	case "nds":
		return "El Wi-Fi del juego va por Internet con servidores de la comunidad: Nintendo WFC › Ajustes y conectad al punto de acceso «melonAP»."
	case "3ds":
		return "La sala hace de red local entre vosotros: intercambiad y combatid desde el propio juego."
	}
	return ""
}

func (a *app) dibujarSala(l *lienzo) {
	if a.sala == nil {
		a.irA(pInicio)
		a.dibujarInicio(l)
		return
	}
	x0 := panel.Min.X + 24
	l.texto(x0, panel.Min.Y+48, "Sala "+codigoVisible(a.sala.Codigo), a.f.grande, colTexto)
	codigoAncho := medir(a.f.grande, "Sala "+codigoVisible(a.sala.Codigo))
	l.texto(x0+codigoAncho+20, panel.Min.Y+46, recortar(a.f.negrita, a.sala.JuegoTitulo, panel.Max.X-x0-codigoAncho-48), a.f.negrita, colSuave)
	estado, c := a.textoEstadoSala()
	l.texto(x0, panel.Min.Y+80, recortar(a.f.normal, estado, panel.Dx()-48), a.f.normal, c)
	ayuda := ajustar(a.f.pequena, ayudaSistema(a.sala.Sistema), panel.Dx()-48)
	y := panel.Min.Y + 84
	for i, ln := range ayuda {
		if i == 2 {
			break
		}
		y += 24
		l.texto(x0, y, ln, a.f.pequena, colSuave)
	}

	// Botones
	bot := a.botonesSala()
	if a.sel >= len(bot) {
		a.sel = 0
	}
	bx, by := x0, panel.Min.Y+146
	for i, b := range bot {
		w := medir(a.f.negrita, b.texto) + 48
		l.boton(image.Rect(bx, by, bx+w, by+44), b.texto, i == a.sel)
		bx += w + 14
	}

	// Tarjetas: tú y tus amigos (los más recientes).
	var tarjetas []*Progreso
	if p := leerProgreso(archivoPropio(a.sala.Codigo)); p != nil {
		p.Nombre = "Tú (" + a.jug.Nombre + ")"
		tarjetas = append(tarjetas, p)
	}
	amigos := progresosAmigos(a.sala.Codigo, a.jug.ID)
	tarjetas = append(tarjetas, amigos...)
	ty := by + 62
	if len(amigos) == 0 {
		l.texto(x0, panel.Max.Y-16, "Cuando tus amigos entren, aquí verás cómo van, aunque no estéis a la vez.", a.f.pequena, colSuave)
	}
	tw := (panel.Dx() - 48 - 2*16) / 3
	for i, p := range tarjetas {
		if i == 3 {
			break
		}
		tx := x0 + i*(tw+16)
		r := image.Rect(tx, ty, tx+tw, panel.Max.Y-36)
		if len(amigos) > 0 {
			r.Max.Y = panel.Max.Y - 16
		}
		a.tarjeta(l, r, p)
	}
}

func (a *app) tarjeta(l *lienzo, r image.Rectangle, p *Progreso) {
	l.redondeado(r, 16, colTarjeta, 0, nil)
	foto := image.Rect(r.Min.X+10, r.Min.Y+10, r.Max.X-10, r.Min.Y+10+(r.Dx()-20)*2/3)
	if foto.Max.Y > r.Max.Y-100 {
		foto.Max.Y = r.Max.Y - 100
	}
	l.redondeado(foto, 10, colTexto, 0, nil)
	if img := a.imagenCaptura(p); img != nil {
		l.imagen(foto.Inset(3), img)
	} else {
		l.centrado(foto, "Sin captura todavía", a.f.pequena, colBlanco)
	}
	x, y := r.Min.X+12, foto.Max.Y
	ancho := r.Dx() - 24
	partida := "partida de siempre"
	if p.Partida == "sala" {
		partida = "partida de la sala"
	}
	lineas := []struct {
		t    string
		cara int
	}{
		{p.Nombre, 1}, {p.Juego, 0}, {partida + " · " + duracionTexto(p.JugadoMs), 0}, {haceTexto(p.UltimaVez), 0},
	}
	for _, ln := range lineas {
		cara, col := a.f.pequena, colSuave
		if ln.cara == 1 {
			cara, col = a.f.negrita, colTexto
		}
		y += cara.Metrics().Height.Round() + 2
		if y > r.Max.Y-6 {
			break
		}
		l.texto(x, y, recortar(cara, ln.t, ancho), cara, col)
	}
}

func (a *app) titulo(l *lienzo, t string) {
	l.texto(panel.Min.X+24, panel.Min.Y+44, t, a.f.titulo, colTexto)
}

func (a *app) dibujarJuegos(l *lienzo) {
	a.titulo(l, "Elige el juego")
	if len(a.juegos) == 0 {
		l.parrafo(panel.Min.X+24, panel.Min.Y+70, panel.Dx()-48,
			"No hay juegos en la memoria USB. Conéctala a un ordenador con Windows, abre la parte JUEGOS y copia ahí tus juegos (.gb, .gbc, .gba, .nds, .3ds). Lee LEEME.txt para más detalles.",
			a.f.normal, colTexto)
		return
	}
	filas := make([]opcion, len(a.juegos))
	for i, j := range a.juegos {
		filas[i] = opcion{texto: j.Titulo, etiqueta: sistemas[j.Sistema].nombre}
	}
	a.lista(l, image.Rect(panel.Min.X+20, panel.Min.Y+64, panel.Max.X-20, panel.Max.Y-14), filas)
}

func (a *app) dibujarPartida(l *lienzo) {
	a.titulo(l, "¿Con qué partida vas a jugar en esta sala?")
	ops := []opcion{
		{"Partida nueva solo para esta sala", "", "Empiezas de cero y tu partida de siempre no se toca.", nil},
		{"Mi partida de siempre", "", "La misma que usas al jugar sin sala.", nil},
	}
	for i, o := range ops {
		y := panel.Min.Y + 80 + i*120
		r := image.Rect(panel.Min.X+24, y, panel.Max.X-24, y+100)
		fondo, texto, suave := colTarjeta, colTexto, colSuave
		if i == a.sel {
			fondo, texto, suave = colBoton, colBlanco, colBlanco
		}
		l.redondeado(r, 18, fondo, 0, nil)
		l.texto(r.Min.X+22, r.Min.Y+40, o.texto, a.f.negrita, texto)
		l.texto(r.Min.X+22, r.Min.Y+74, o.ayuda, a.f.normal, suave)
	}
	if a.elegido != nil {
		l.texto(panel.Min.X+24, panel.Max.Y-20, "Juego: "+a.elegido.Titulo, a.f.pequena, colSuave)
	}
}

func (a *app) dibujarTeclado(l *lienzo) {
	titulo := "Escribe el código de la sala"
	if a.fl == fNombre {
		titulo = "Escribe tu nombre"
	}
	a.titulo(l, titulo)
	campo := image.Rect(panel.Min.X+24, panel.Min.Y+64, panel.Max.X-24, panel.Min.Y+116)
	l.redondeado(campo, 14, colBlanco, 0, nil)
	mostrado := a.texto
	if a.fl == fUnirse {
		mostrado = strings.ToUpper(mostrado)
	}
	l.izquierda(campo.Inset(16), mostrado+"_", a.f.titulo, colTexto)

	filas := a.filasTeclado()
	y := campo.Max.Y + 16
	for fi, fila := range filas {
		especial := fi == len(filas)-1
		w, h, hueco := 76, 46, 8
		x := panel.Min.X + 24
		if especial {
			w = (panel.Dx() - 48 - (len(fila)-1)*hueco) / len(fila)
		}
		for ci, t := range fila {
			r := image.Rect(x, y, x+w, y+h)
			elegido := fi == a.filaTecla && ci == a.colTecla
			if elegido {
				l.redondeado(r, 12, colBoton, 0, nil)
				l.centrado(r, t, a.f.negrita, colBlanco)
			} else {
				l.redondeado(r, 12, colTarjeta, 0, nil)
				l.centrado(r, t, a.f.negrita, colTexto)
			}
			x += w + hueco
		}
		y += h + 8
	}
}

func (a *app) dibujarMensaje(l *lienzo) {
	y := l.parrafo(panel.Min.X+40, panel.Min.Y+50, panel.Dx()-80, a.mensaje, a.f.normal, colTexto)
	l.boton(image.Rect(panel.Min.X+40, y+30, panel.Min.X+220, y+76), "Aceptar", true)
}

func (a *app) dibujarConfirmar(l *lienzo) {
	y := l.parrafo(panel.Min.X+40, panel.Min.Y+50, panel.Dx()-80, a.confirmarTexto, a.f.normal, colTexto)
	l.boton(image.Rect(panel.Min.X+40, y+30, panel.Min.X+200, y+76), "Sí", a.sel == 0)
	l.boton(image.Rect(panel.Min.X+220, y+30, panel.Min.X+380, y+76), "No", a.sel == 1)
}
