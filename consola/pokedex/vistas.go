package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
)

// Pantallas con el estilo de Panther OS (fondo de las listas, letra Selawik) y el rojo de la
// Pokédex. Los dibujos de los Pokémon no van en la app: se bajan a la consola (sprites.go).
var (
	colPokedex   = color.RGBA{0xe3, 0x2b, 0x3c, 0xff}
	colPokedexOs = color.RGBA{0xa3, 0x16, 0x2c, 0xff}
	colJuegos    = color.RGBA{0x3d, 0x7b, 0xe8, 0xff}
	colJuegosOs  = color.RGBA{0x5b, 0x3f, 0xc8, 0xff}
)

func (a *app) dibujar() {
	l := a.l
	l.limpiarFondo()
	if !a.cargaHasta.IsZero() {
		a.dibujarCarga()
		return
	}
	switch a.pant {
	case pInicio:
		a.dibujarInicio()
	case pCartas:
		a.dibujarCartas()
	case pJuegos:
		a.dibujarJuegos()
	case pBuscar:
		a.dibujarBuscar()
	}
	if a.avisoTexto != "" {
		a.dibujarAviso()
	}
}

func (a *app) translucido(r image.Rectangle, radio int, alfa uint8) {
	a.l.redondeado(r, radio, color.RGBA{alfa, alfa, alfa, alfa})
}

// logo: cuadrado redondeado rojo con un libro (la Pokédex es la enciclopedia de los Pokémon).
func (a *app) logo(x, y, tam int) {
	a.icono(x, y, tam, colPokedex, colPokedexOs, icLibro)
}

func (a *app) icono(x, y, tam int, c1, c2 color.RGBA, g string) {
	a.l.pegar(placa(tam, tam, tam/4, c1, c2), x, y)
	if a.f.ic != nil {
		a.l.glifo(a.f.iconoDe(float64(tam)*0.5), g, x+tam/2, y+tam/2, colTexto)
	}
}

func (a *app) dibujarCarga() {
	l := a.l
	a.logo(ancho/2-55, alto/2-120, 110)
	l.centrado(image.Rect(0, alto/2+4, ancho, alto/2+40), "Pokédex", l.f.grande, colTexto)
	l.centrado(image.Rect(0, alto-48, ancho, alto-20), "Panther OS", l.f.mini, colSuave)
}

func (a *app) dibujarAviso() {
	l := a.l
	s := recortar(l.f.pequena, a.avisoTexto, ancho-80)
	w := medir(l.f.pequena, s) + 36
	y := alto - 88
	if a.pant == pBuscar { // encima del teclado no: en la línea de lo que va saliendo
		y = 122
	}
	r := image.Rect((ancho-w)/2, y, (ancho+w)/2, y+28)
	l.redondeado(r, 14, color.RGBA{0, 0, 0, 0xe8})
	l.centrado(r, s, l.f.pequena, colTexto)
}

func (a *app) ayudaAbajo(s string) {
	l := a.l
	s = recortar(l.f.pequena, s, ancho-16)
	m := l.f.pequena.Metrics()
	l.texto((ancho-medir(l.f.pequena, s))/2, alto-18+(m.Ascent.Round()-m.Descent.Round())/2, s, l.f.pequena, colAyuda)
}

// cabecera: logo, título y una línea debajo (deja libre arriba a la derecha, donde va el recuadro
// de la batería y la hora).
func (a *app) cabecera(titulo, sub string) {
	l := a.l
	a.logo(22, 14, 44)
	l.izquierda(image.Rect(78, 10, 430, 42), titulo, l.f.grande, colTexto)
	l.izquierda(image.Rect(80, 44, ancho-20, 64), recortar(l.f.mini, sub, ancho-110), l.f.mini, colSuave)
}

func (a *app) cuenta() string {
	return fmt.Sprintf("%d de %d conseguidos", a.col.n, total)
}

// barra: lo conseguido, en verde.
func (a *app) barra(r image.Rectangle) {
	a.l.redondeado(r, r.Dy()/2, color.RGBA{0x30, 0x30, 0x30, 0x30})
	if a.col.n > 0 {
		w := max(r.Dy(), r.Dx()*a.col.n/total)
		a.l.redondeado(image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), r.Dy()/2, colVerde)
	}
}

// boton: uno de los dos botones grandes del inicio.
func (a *app) boton(r image.Rectangle, elegido bool, c1, c2 color.RGBA, g, titulo, sub string) {
	l := a.l
	if elegido {
		a.translucido(r, 22, 0x34)
		l.marcoRedondo(r, 22, 3, colMarco)
	} else {
		a.translucido(r, 22, 0x1a)
	}
	tam := 76
	a.icono(r.Min.X+22, r.Min.Y+(r.Dy()-tam)/2, tam, c1, c2, g)
	x := r.Min.X + 22 + tam + 22
	l.texto(x, r.Min.Y+50, titulo, l.f.titulo, colTexto)
	l.izquierda(image.Rect(x, r.Min.Y+60, r.Max.X-20, r.Min.Y+80), sub, l.f.pequena, colSuave)
}

func (a *app) dibujarInicio() {
	l := a.l
	a.cabecera("Pokédex", "Todos los Pokémon, del 1 al "+fmt.Sprint(total))
	w, h := 500, 122
	x := (ancho - w) / 2
	r1 := image.Rect(x, 112, x+w, 112+h)
	r2 := image.Rect(x, 112+h+22, x+w, 112+2*h+22)
	a.boton(r1, a.menu == 0, colPokedex, colPokedexOs, icCartas, "Cartas", a.cuenta())
	a.barra(image.Rect(r1.Min.X+120, r1.Min.Y+90, r1.Max.X-28, r1.Min.Y+98))
	a.boton(r2, a.menu == 1, colJuegos, colJuegosOs, icMando, "Juegos", "Muy pronto")
	// la etiqueta «Por llegar»
	e := "Por llegar"
	ew := medir(l.f.mini, e) + 20
	er := image.Rect(r2.Min.X+120, r2.Min.Y+88, r2.Min.X+120+ew, r2.Min.Y+108)
	l.redondeado(er, 10, color.RGBA{0x40, 0x40, 0x40, 0x40})
	l.centrado(er, e, l.f.mini, colTexto)
	a.ayudaAbajo("A  Entrar      B  Salir      SELECT  Apagar la pantalla")
}

func (a *app) dibujarJuegos() {
	l := a.l
	a.cabecera("Juegos", "Juegos con los Pokémon de la Pokédex")
	a.icono(ancho/2-48, 120, 96, colJuegos, colJuegosOs, icReloj)
	l.centrado(image.Rect(0, 236, ancho, 272), "Los juegos están por llegar", l.f.grande, colTexto)
	for i, s := range []string{
		"Muy pronto habrá juegos aquí.",
		"Mientras tanto, en Cartas puedes marcar los Pokémon que tienes.",
	} {
		l.centrado(image.Rect(20, 284+i*26, ancho-20, 308+i*26), s, l.f.pequena, colSuave)
	}
	a.ayudaAbajo("B  Volver")
}

// nombreVisible: el nombre como se escribe (si la letra no tiene ♀ o ♂, con palabras).
func (a *app) nombreVisible(n int) string {
	s := nombre(n)
	if strings.ContainsAny(s, "♀♂") {
		if !a.f.tiene('♀') {
			s = strings.ReplaceAll(s, "♀", " hembra")
		}
		if !a.f.tiene('♂') {
			s = strings.ReplaceAll(s, "♂", " macho")
		}
	}
	return s
}

// numero: «#0025» (la letra del tema no tiene «º»).
func numero(n int) string { return fmt.Sprintf("#%04d", n) }

func (a *app) dibujarCartas() {
	l := a.l
	sub := a.cuenta()
	if a.busqueda != "" {
		res := "resultados"
		if len(a.lista) == 1 {
			res = "resultado"
		}
		sub = fmt.Sprintf("Buscando «%s»: %d %s · %s", a.busqueda, len(a.lista), res, sub)
	} else if len(a.lista) > 0 {
		sub = fmt.Sprintf("Generación %d · %s", generacion(a.lista[a.sel]), sub)
	}
	a.cabecera("Cartas", sub)

	for i := 0; i < filasLista; i++ {
		k := a.primera + i
		if k >= len(a.lista) {
			break
		}
		n := a.lista[k]
		y := listaArriba + i*filaAlto
		r := image.Rect(22, y, ancho-30, y+filaAlto-6)
		elegido := k == a.sel
		if elegido {
			a.translucido(r, 14, 0x30)
			l.marcoRedondo(r, 14, 2, colMarco)
		} else {
			a.translucido(r, 14, 0x12)
		}
		l.izquierda(image.Rect(r.Min.X+16, r.Min.Y, r.Min.X+86, r.Max.Y), numero(n), l.f.numero, colAzul)
		sx, sy := r.Min.X+90, r.Min.Y+(r.Dy()-spriteAlto)/2
		if im := a.sp.img[n]; im != nil {
			l.pegar(im, sx, sy)
		} else { // aún no ha llegado (o no hay internet): un hueco
			a.translucido(image.Rect(sx+6, sy+3, sx+spriteAncho-6, sy+spriteAlto-3), 8, 0x14)
		}
		l.izquierda(image.Rect(r.Min.X+142, r.Min.Y, r.Max.X-150, r.Max.Y), a.nombreVisible(n), l.f.nombre, colTexto)
		cx, cy := r.Max.X-28, (r.Min.Y+r.Max.Y)/2
		if a.col.tiene[n] {
			if elegido {
				l.derecha(image.Rect(r.Max.X-150, r.Min.Y, cx-20, r.Max.Y), "Conseguido", l.f.mini, colVerde)
			}
			l.circulo(cx, cy, 12, colVerde)
			if a.f.ic != nil {
				l.glifo(a.f.iconoPeq, icHecho, cx, cy, colFondo)
			}
		} else {
			if elegido {
				l.derecha(image.Rect(r.Max.X-150, r.Min.Y, cx-20, r.Max.Y), "Me falta", l.f.mini, colApagado)
			}
			l.marcoRedondo(image.Rect(cx-12, cy-12, cx+12, cy+12), 12, 2, colApagado)
		}
	}
	a.pedirSprites()
	// barra de la derecha: dónde se está
	if len(a.lista) > filasLista {
		top, bot := listaArriba, listaArriba+filasLista*filaAlto-6
		l.redondeado(image.Rect(ancho-20, top, ancho-15, bot), 2, color.RGBA{0x28, 0x28, 0x28, 0x28})
		alto := max(18, (bot-top)*filasLista/len(a.lista))
		y := top + (bot-top-alto)*a.primera/max(1, len(a.lista)-filasLista)
		l.redondeado(image.Rect(ancho-20, y, ancho-15, y+alto), 2, colSuave)
	}
	if len(a.lista) > 0 && a.col.tiene[a.lista[a.sel]] {
		a.ayudaAbajo(a.ayudaCartas("A  Quitar"))
	} else {
		a.ayudaAbajo(a.ayudaCartas("A  Conseguido"))
	}
}

// pedirSprites: los dibujos de lo que se ve y de un poco más arriba y más abajo. Lo que se ve va
// lo último, que es lo primero que se baja.
func (a *app) pedirSprites() {
	ini, fin := a.primera, min(a.primera+filasLista, len(a.lista))
	for k := max(ini-filasLista, 0); k < min(fin+filasLista, len(a.lista)); k++ {
		if k < ini || k >= fin {
			a.sp.pedir(a.lista[k])
		}
	}
	for k := fin - 1; k >= ini; k-- {
		a.sp.pedir(a.lista[k])
	}
}

func (a *app) ayudaCartas(primero string) string {
	if a.busqueda != "" {
		return primero + "      Y  Buscar      B  Ver todos"
	}
	return primero + "      Y  Buscar      L R  Generación      B  Volver"
}

func (a *app) dibujarBuscar() {
	l := a.l
	a.cabecera("Buscar", "Por nombre o por número · START busca · SELECT vuelve")

	caja := image.Rect(22, 74, ancho-22, 120)
	a.translucido(caja, 16, 0x32)
	if a.f.ic != nil {
		l.glifo(a.f.icono, icBuscar, caja.Min.X+26, (caja.Min.Y+caja.Max.Y)/2, colSuave)
	}
	t := a.tec
	texto := string(t.escrito[:t.cursor]) + "|" + string(t.escrito[t.cursor:])
	col := color.Color(colTexto)
	if len(t.escrito) == 0 {
		texto, col = "|  Pikachu, 25…", colApagado
	}
	l.izquierda(image.Rect(caja.Min.X+48, caja.Min.Y, caja.Max.X-14, caja.Max.Y), texto, l.f.texto, col)

	// lo que va saliendo
	pr := image.Rect(30, 124, ancho-30, 146)
	switch {
	case normalizar(t.texto()) == "":
		l.izquierda(pr, "Escribe un nombre o un número", l.f.mini, colSuave)
	case len(a.previa) == 0:
		l.izquierda(pr, "No sale ningún Pokémon", l.f.mini, colApagado)
	default:
		var partes []string
		for _, n := range a.previa[:min(len(a.previa), 4)] {
			partes = append(partes, a.nombreVisible(n)+" "+numero(n))
		}
		res := "resultados"
		if len(a.previa) == 1 {
			res = "resultado"
		}
		s := fmt.Sprintf("%d %s: %s", len(a.previa), res, strings.Join(partes, ", "))
		if len(a.previa) > 4 {
			s += "…"
		}
		l.izquierda(pr, s, l.f.mini, colVerde)
	}

	y0 := 152
	for f := 0; f < 4; f++ {
		for c, r := range []rune(filasTeclado[f]) {
			k := image.Rect(14+c*61, y0+f*54, 14+c*61+55, y0+f*54+46)
			if t.fila == f && t.col == c {
				l.redondeado(k, 12, colTexto)
				l.centrado(k, string(r), l.f.nombre, colFondo)
			} else {
				a.translucido(k, 12, 38)
				l.centrado(k, string(r), l.f.nombre, colTexto)
			}
		}
	}
	anchos := []int{300, 150, 160}
	x := 14
	for i, w := range anchos {
		k := image.Rect(x, y0+4*54, x+w-6, y0+4*54+46)
		switch {
		case t.fila == 4 && t.col == i:
			l.redondeado(k, 12, colTexto)
			l.centrado(k, especiales[i], l.f.nombre, colFondo)
		case i == 2:
			l.redondeado(k, 12, colPokedex)
			l.centrado(k, especiales[i], l.f.nombre, colTexto)
		default:
			a.translucido(k, 12, 38)
			l.centrado(k, especiales[i], l.f.nombre, colTexto)
		}
		x += w
	}
	a.ayudaAbajo("A  Escribir    B  Borrar    Y  Espacio    X  Borrar todo    START  Buscar")
}
