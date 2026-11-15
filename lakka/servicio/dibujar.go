package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
)

// Medidas de la pantalla: en "compacto" (pantallas de menos de 800, como 640×480) todo es algo
// más pequeño y va en una sola columna.
var (
	altoCabecera = elegir(54, 72)
	margen       = elegir(10, 24)
	altoPie      = elegir(30, 40)
	altoFila     = elegir(42, 50)
	panel        = image.Rect(margen, altoCabecera+elegir(4, 8), ancho-margen, alto-altoPie)
	dentro       = elegir(14, 24) // margen dentro del panel
)

func (a *app) dibujar() {
	a.sucio = false
	l := nuevoLienzo(a.f)
	l.rect(l.img.Bounds(), colFondo)
	a.cabecera(l)
	l.redondeado(panel, elegir(16, 24), colPanel, 0, nil)
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
	switch {
	case a.pant == pTeclado:
		ayuda = "A: escribir     B o Y: borrar     Start: aceptar"
	case modoPort || compacto:
		ayuda = "A o Start: aceptar     B: volver"
	}
	l.centrado(image.Rect(0, alto-altoPie, ancho, alto), ayuda, a.f.pequena, colBlanco)
	a.srv.enseñar(l.marco())
}

// dibujarAviso: una pantalla de espera (al abrir un juego).
func (a *app) dibujarAviso(texto string) {
	l := nuevoLienzo(a.f)
	l.rect(l.img.Bounds(), colFondo)
	a.cabecera(l)
	l.redondeado(panel, elegir(16, 24), colPanel, 0, nil)
	l.centrado(panel, texto, a.f.titulo, colTexto)
	a.srv.enseñar(l.marco())
}

func (a *app) cabecera(l *lienzo) {
	// Las luces de la Pokédex.
	d := altoCabecera * 44 / 72
	y0 := (altoCabecera - d) / 2
	l.redondeado(image.Rect(margen, y0, margen+d, y0+d), d/2, colBlanco, 0, nil)
	l.redondeado(image.Rect(margen+d/9, y0+d/9, margen+d-d/9, y0+d-d/9), d/2-d/9, colAzul, 0, nil)
	luz := d * 12 / 44
	for i, c := range []color.Color{color.RGBA{0xe8, 0x8a, 0x8a, 0xff}, colAmarillo, color.RGBA{0x8b, 0xc3, 0x4a, 0xff}} {
		x := margen + d + 12 + i*(luz+8)
		l.redondeado(image.Rect(x, altoCabecera/2-luz/2, x+luz, altoCabecera/2+luz/2), luz/2, c, 0, nil)
	}
	xTitulo := margen + d + 12 + 3*(luz+8) + 8
	l.texto(xTitulo, baseCentrada(image.Rect(0, 0, ancho, altoCabecera), a.f.titulo), "Pokemulador", a.f.titulo, colBlanco)
	estado := "Sin Internet"
	if hayInternet() {
		estado = "Con Internet"
	}
	derecha := estado + "   ·   " + a.jug.Nombre
	finTitulo := xTitulo + medir(a.f.titulo, "Pokemulador") + 16
	derecha = recortar(a.f.pequena, derecha, ancho-margen-finTitulo)
	l.texto(ancho-margen-medir(a.f.pequena, derecha), baseCentrada(image.Rect(0, 0, ancho, altoCabecera), a.f.pequena), derecha, a.f.pequena, colBlanco)
}

// lista con desplazamiento.
func (a *app) lista(l *lienzo, r image.Rectangle, filas []opcion) {
	hueco := elegir(5, 6)
	visibles := max(1, (r.Dy()+hueco)/(altoFila+hueco))
	if a.sel < a.desp {
		a.desp = a.sel
	}
	if a.sel >= a.desp+visibles {
		a.desp = a.sel - visibles + 1
	}
	for i := a.desp; i < len(filas) && i < a.desp+visibles; i++ {
		y := r.Min.Y + (i-a.desp)*(altoFila+hueco)
		l.fila(image.Rect(r.Min.X, y, r.Max.X, y+altoFila), filas[i].texto, filas[i].etiqueta, i == a.sel)
	}
	if a.desp > 0 {
		l.centrado(image.Rect(r.Max.X-40, r.Min.Y-20, r.Max.X, r.Min.Y), "▲", a.f.pequena, colSuave)
	}
	if a.desp+visibles < len(filas) {
		l.centrado(image.Rect(r.Max.X-40, r.Max.Y-4, r.Max.X, r.Max.Y+16), "▼", a.f.pequena, colSuave)
	}
}

func (a *app) dibujarInicio(l *lienzo) {
	ops := a.opcionesInicio()
	if a.sel >= len(ops) {
		a.sel = len(ops) - 1
	}
	o := ops[a.sel]
	resumen := fmt.Sprintf("Juegos en la memoria: %d", len(a.juegos))
	if len(a.juegos) == 0 {
		resumen = "No hay juegos: copia los tuyos a la parte JUEGOS de la memoria desde un ordenador."
		if os.Getenv("POKEMULADOR_JUEGOS") != "" {
			resumen = "No hay juegos en " + a.raiz + "."
		}
	}
	if compacto {
		// Una columna: la lista y, debajo, la explicación de lo elegido.
		altoAyuda := 3*(a.f.pequena.Metrics().Height.Round()+4) + 10
		a.lista(l, image.Rect(panel.Min.X+dentro, panel.Min.Y+12, panel.Max.X-dentro, panel.Max.Y-altoAyuda), ops)
		texto := o.ayuda
		if a.sel == 0 || len(a.juegos) == 0 {
			texto = resumen + " " + texto
		}
		lineas := ajustar(a.f.pequena, texto, panel.Dx()-2*dentro)
		y := panel.Max.Y - altoAyuda + 4
		for i, ln := range lineas {
			if i == 3 {
				break
			}
			y += a.f.pequena.Metrics().Height.Round() + 4
			l.texto(panel.Min.X+dentro, y, ln, a.f.pequena, colSuave)
		}
		return
	}
	a.lista(l, image.Rect(panel.Min.X+20, panel.Min.Y+22, panel.Min.X+panel.Dx()*58/100, panel.Max.Y-20), ops)
	info := image.Rect(panel.Min.X+panel.Dx()*58/100+30, panel.Min.Y+22, panel.Max.X-22, panel.Max.Y-20)
	l.redondeado(info, 18, colTarjeta, 0, nil)
	y := l.parrafo(info.Min.X+18, info.Min.Y+4, info.Dx()-36, o.texto, a.f.negrita, colTexto)
	l.parrafo(info.Min.X+18, y+6, info.Dx()-36, o.ayuda, a.f.pequena, colSuave)
	lineas := ajustar(a.f.pequena, resumen, info.Dx()-36)
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
			if modoPort {
				t = "Sin Internet. Conecta el Wi-Fi en los ajustes de la consola."
			}
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
	x0 := panel.Min.X + dentro
	anchoUtil := panel.Dx() - 2*dentro
	titulo := "Sala " + codigoVisible(a.sala.Codigo)
	yTitulo := panel.Min.Y + elegir(36, 48)
	l.texto(x0, yTitulo, titulo, a.f.grande, colTexto)
	resto := anchoUtil - medir(a.f.grande, titulo) - 20
	if resto > 80 {
		l.texto(x0+medir(a.f.grande, titulo)+20, yTitulo-2, recortar(a.f.negrita, a.sala.JuegoTitulo, resto), a.f.negrita, colSuave)
	}
	estado, c := a.textoEstadoSala()
	y := yTitulo + elegir(28, 32)
	l.texto(x0, y, recortar(a.f.normal, estado, anchoUtil), a.f.normal, c)
	for i, ln := range ajustar(a.f.pequena, ayudaSistema(a.sala.Sistema), anchoUtil) {
		if i == 2 {
			break
		}
		y += a.f.pequena.Metrics().Height.Round() + 4
		l.texto(x0, y, ln, a.f.pequena, colSuave)
	}

	// Botones (pasan a otra fila si no caben).
	bot := a.botonesSala()
	if a.sel >= len(bot) {
		a.sel = 0
	}
	altoBoton := elegir(38, 44)
	bx, by := x0, y+elegir(12, 18)
	for i, b := range bot {
		w := medir(a.f.negrita, b.texto) + elegir(32, 48)
		if bx+w > panel.Max.X-dentro && bx > x0 {
			bx, by = x0, by+altoBoton+8
		}
		l.boton(image.Rect(bx, by, bx+w, by+altoBoton), b.texto, i == a.sel)
		bx += w + 12
	}

	// Tarjetas: tú y tus amigos (los más recientes).
	var tarjetas []*Progreso
	if p := leerProgreso(archivoPropio(a.sala.Codigo)); p != nil {
		p.Nombre = "Tú (" + a.jug.Nombre + ")"
		tarjetas = append(tarjetas, p)
	}
	amigos := progresosAmigos(a.sala.Codigo, a.jug.ID)
	tarjetas = append(tarjetas, amigos...)
	ty := by + altoBoton + 14
	abajo := panel.Max.Y - 12
	if len(amigos) == 0 {
		nota := "Cuando tus amigos entren, aquí verás cómo van, aunque no estéis a la vez."
		l.texto(x0, panel.Max.Y-12, recortar(a.f.pequena, nota, anchoUtil), a.f.pequena, colSuave)
		abajo = panel.Max.Y - 12 - a.f.pequena.Metrics().Height.Round() - 6
	}
	n := elegir(2, 3)
	hueco := 12
	tw := (anchoUtil - (n-1)*hueco) / n
	for i, p := range tarjetas {
		if i == n || abajo-ty < 90 {
			break
		}
		tx := x0 + i*(tw+hueco)
		a.tarjeta(l, image.Rect(tx, ty, tx+tw, abajo), p)
	}
}

func (a *app) tarjeta(l *lienzo, r image.Rectangle, p *Progreso) {
	l.redondeado(r, 14, colTarjeta, 0, nil)
	altoTexto := 2*(a.f.negrita.Metrics().Height.Round()+2) + 2*(a.f.pequena.Metrics().Height.Round()+2) + 8
	foto := image.Rect(r.Min.X+8, r.Min.Y+8, r.Max.X-8, r.Min.Y+8+(r.Dx()-16)*2/3)
	if foto.Max.Y > r.Max.Y-altoTexto {
		foto.Max.Y = r.Max.Y - altoTexto
	}
	if foto.Dy() > 30 {
		l.redondeado(foto, 10, colTexto, 0, nil)
		if img := a.imagenCaptura(p); img != nil {
			l.imagen(foto.Inset(3), img)
		} else {
			l.centrado(foto, "Sin captura todavía", a.f.pequena, colBlanco)
		}
	} else {
		foto.Max.Y = r.Min.Y + 4
	}
	x, y := r.Min.X+10, foto.Max.Y
	anchoTexto := r.Dx() - 20
	partida := "partida de siempre"
	if p.Partida == "sala" {
		partida = "partida de la sala"
	}
	lineas := []struct {
		t       string
		negrita bool
	}{
		{p.Nombre, true}, {p.Juego, false}, {partida + " · " + duracionTexto(p.JugadoMs), false}, {haceTexto(p.UltimaVez), false},
	}
	for _, ln := range lineas {
		cara, col := a.f.pequena, colSuave
		if ln.negrita {
			cara, col = a.f.negrita, colTexto
		}
		y += cara.Metrics().Height.Round() + 2
		if y > r.Max.Y-4 {
			break
		}
		l.texto(x, y, recortar(cara, ln.t, anchoTexto), cara, col)
	}
}

func (a *app) titulo(l *lienzo, t string) int {
	y := panel.Min.Y + elegir(34, 44)
	l.texto(panel.Min.X+dentro, y, recortar(a.f.titulo, t, panel.Dx()-2*dentro), a.f.titulo, colTexto)
	return y + elegir(14, 20)
}

func (a *app) dibujarJuegos(l *lienzo) {
	y := a.titulo(l, "Elige el juego")
	if len(a.juegos) == 0 {
		texto := "No hay juegos en la memoria USB. Conéctala a un ordenador con Windows, abre la parte JUEGOS y copia ahí tus juegos (.gb, .gbc, .gba, .nds, .3ds). Lee LEEME.txt para más detalles."
		if os.Getenv("POKEMULADOR_JUEGOS") != "" {
			texto = "No hay juegos en " + a.raiz + "."
		}
		l.parrafo(panel.Min.X+dentro, y, panel.Dx()-2*dentro, texto, a.f.normal, colTexto)
		return
	}
	filas := make([]opcion, len(a.juegos))
	for i, j := range a.juegos {
		filas[i] = opcion{texto: j.Titulo, etiqueta: sistemas[j.Sistema].nombre}
	}
	a.lista(l, image.Rect(panel.Min.X+dentro-4, y, panel.Max.X-dentro+4, panel.Max.Y-12), filas)
}

func (a *app) dibujarPartida(l *lienzo) {
	y := a.titulo(l, "¿Con qué partida vas a jugar en esta sala?")
	ops := []opcion{
		{"Partida nueva solo para esta sala", "", "Empiezas de cero y tu partida de siempre no se toca.", nil},
		{"Mi partida de siempre", "", "La misma que usas al jugar sin sala.", nil},
	}
	alto := elegir(84, 100)
	for i, o := range ops {
		yy := y + i*(alto+16)
		r := image.Rect(panel.Min.X+dentro, yy, panel.Max.X-dentro, yy+alto)
		fondo, texto, suave := colTarjeta, colTexto, colSuave
		if i == a.sel {
			fondo, texto, suave = colBoton, colBlanco, colBlanco
		}
		l.redondeado(r, 16, fondo, 0, nil)
		l.texto(r.Min.X+18, r.Min.Y+elegir(32, 40), recortar(a.f.negrita, o.texto, r.Dx()-36), a.f.negrita, texto)
		l.parrafo(r.Min.X+18, r.Min.Y+elegir(38, 48), r.Dx()-36, o.ayuda, a.f.pequena, suave)
	}
	if a.elegido != nil {
		l.texto(panel.Min.X+dentro, panel.Max.Y-14, recortar(a.f.pequena, "Juego: "+a.elegido.Titulo, panel.Dx()-2*dentro), a.f.pequena, colSuave)
	}
}

func (a *app) dibujarTeclado(l *lienzo) {
	titulo := "Escribe el código de la sala"
	if a.fl == fNombre {
		titulo = "Escribe tu nombre"
	}
	y := a.titulo(l, titulo)
	campo := image.Rect(panel.Min.X+dentro, y, panel.Max.X-dentro, y+elegir(44, 52))
	l.redondeado(campo, 12, colBlanco, 0, nil)
	mostrado := a.texto
	if a.fl == fUnirse {
		mostrado = strings.ToUpper(mostrado)
	}
	l.izquierda(campo.Inset(14), mostrado+"_", a.f.titulo, colTexto)

	filas := a.filasTeclado()
	hueco := elegir(6, 8)
	h := elegir(40, 46)
	wNormal := (panel.Dx() - 2*dentro - 9*hueco) / 10
	y = campo.Max.Y + elegir(10, 16)
	for fi, fila := range filas {
		w := wNormal
		if fi == len(filas)-1 {
			w = (panel.Dx() - 2*dentro - (len(fila)-1)*hueco) / len(fila)
		}
		x := panel.Min.X + dentro
		for ci, t := range fila {
			r := image.Rect(x, y, x+w, y+h)
			if fi == a.filaTecla && ci == a.colTecla {
				l.redondeado(r, 10, colBoton, 0, nil)
				l.centrado(r, t, a.f.negrita, colBlanco)
			} else {
				l.redondeado(r, 10, colTarjeta, 0, nil)
				l.centrado(r, t, a.f.negrita, colTexto)
			}
			x += w + hueco
		}
		y += h + hueco
	}
}

func (a *app) dibujarMensaje(l *lienzo) {
	y := l.parrafo(panel.Min.X+dentro+10, panel.Min.Y+30, panel.Dx()-2*dentro-20, a.mensaje, a.f.normal, colTexto)
	l.boton(image.Rect(panel.Min.X+dentro+10, y+24, panel.Min.X+dentro+190, y+24+elegir(40, 46)), "Aceptar", true)
}

func (a *app) dibujarConfirmar(l *lienzo) {
	y := l.parrafo(panel.Min.X+dentro+10, panel.Min.Y+30, panel.Dx()-2*dentro-20, a.confirmarTexto, a.f.normal, colTexto)
	h := elegir(40, 46)
	x := panel.Min.X + dentro + 10
	l.boton(image.Rect(x, y+24, x+150, y+24+h), "Sí", a.sel == 0)
	l.boton(image.Rect(x+170, y+24, x+320, y+24+h), "No", a.sel == 1)
}
