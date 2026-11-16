package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas (youtube -prueba carpeta): todas las pantallas con datos inventados, en PNG.
// Las miniaturas son degradados (no se baja nada).
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	tmp, _ := os.MkdirTemp("", "youtube-prueba")
	defer os.RemoveAll(tmp)
	carpetaDatos, carpetaVideos = tmp, filepath.Join(tmp, "videos")
	redOK.Store(true)
	a := nuevaApp(f)
	a.cargaHasta = time.Time{}
	a.alm = nuevoAlmacen(tmp, carpetaVideos)
	a.mini = &miniaturas{app: a, listo: map[string]*image.RGBA{}, orig: map[string]image.Image{}, pedidas: map[string]bool{},
		cola: make(chan [2]string, 1)}
	a.yt = nuevoYouTube()
	a.rep = nuevoReproductorVideo(func([]byte, int, int) {}, func(int) {}, func(int, error) {})
	vids := []Video{
		{ID: "prueba00001", Titulo: "Los 10 mejores momentos de la temporada (resumen completo)", Canal: "Deportes Hoy",
			Segundos: 754, Vistas: "1,2 M de visualizaciones", Fecha: "hace 2 semanas"},
		{ID: "prueba00002", Titulo: "Cómo hacer pan casero en 30 minutos", Canal: "La cocina de Ana", Segundos: 1832,
			Vistas: "845 mil visualizaciones", Fecha: "hace 1 año"},
		{ID: "prueba00003", Titulo: "Concierto acústico en directo desde el estudio", Canal: "Música en vivo",
			Vistas: "3,4 mil espectadores", Directo: true},
		{ID: "prueba00004", Titulo: "Guía de Pokémon Esmeralda: todas las MT y dónde encontrarlas", Canal: "Pokéguías",
			Segundos: 2710, Vistas: "96 mil visualizaciones", Fecha: "hace 3 años"},
		{ID: "prueba00005", Titulo: "Viaje en tren por los Alpes", Canal: "Trenes del mundo", Segundos: 3605,
			Vistas: "2,8 M de visualizaciones", Fecha: "hace 5 años"},
		{ID: "prueba00006", Titulo: "Videoclip oficial", Canal: "Artista", Segundos: 213, Vistas: "12 M de visualizaciones",
			Fecha: "hace 4 meses"},
	}
	cortos := []Video{
		{ID: "corto000001", Titulo: "El truco para que el pan quede crujiente 🍞", Canal: "La cocina de Ana", Vistas: "2,1 M de visualizaciones", Corto: true, Segundos: 41},
		{ID: "corto000002", Titulo: "Así se ve el tren desde la cabina", Canal: "Trenes del mundo", Vistas: "540 mil visualizaciones", Corto: true, Segundos: 28},
		{ID: "corto000003", Titulo: "Shiny en el primer intento", Canal: "Pokéguías", Vistas: "88 mil visualizaciones", Corto: true, Segundos: 19},
		{ID: "corto000004", Titulo: "Gol de chilena en el último minuto", Canal: "Deportes Hoy", Vistas: "4 M de visualizaciones", Corto: true, Segundos: 33},
	}
	for i, v := range append(append([]Video{}, vids...), cortos...) {
		a.mini.orig[v.claveMini()] = degradado(0.08+0.11*float64(i), 320, 180)
	}
	a.alm.apuntar(vids[4], 0)
	a.alm.apuntar(vids[1], 640)
	a.alm.apuntar(vids[3], 0)
	a.alm.apuntar(vids[0], 323)
	for _, v := range []Video{vids[5], vids[1]} {
		_ = os.MkdirAll(carpetaVideos, 0o755)
		_ = os.WriteFile(filepath.Join(carpetaVideos, v.ID+".mp4"), []byte("x"), 0o644)
		guardarJSON(filepath.Join(carpetaVideos, v.ID+".json"), Descargado{Video: v, FPS: 30, Cuando: ahoraMs()})
	}
	a.alm.cargarDescargados()

	guardar := func(nombre string, encima func()) error {
		a.dibujar()
		if encima != nil {
			encima()
		}
		fh, err := os.Create(filepath.Join(carpeta, nombre))
		if err != nil {
			return err
		}
		defer fh.Close()
		return png.Encode(fh, a.l.img)
	}
	// Sin cuenta.
	a.pant, a.foco = pInicio, "historial"
	if err := guardar("1-inicio.png", nil); err != nil {
		return err
	}
	a.foco = "buscar"
	a.descPend, a.descActual, a.descPct = 1, vids[3].Titulo, 42
	a.descEnCola[vids[3].ID] = true
	if err := guardar("1b-inicio-buscar.png", nil); err != nil {
		return err
	}
	a.descPend, a.descActual = 0, ""
	a.focoCuenta = true
	a.abrirCuenta()
	a.web.cerrar() // en las pruebas no hace falta la página
	a.web = nil
	a.sesionEstado = "Esperando tu sesión…"
	if err := guardar("8-cuenta-pasar.png", nil); err != nil {
		return err
	}
	// Con cuenta.
	a.yt.cuenta = &cuenta{ruta: filepath.Join(tmp, "cuenta.json"), Cookie: "SAPISID=prueba", Nombre: "Panther", Usuario: "@panther"}
	a.cambiandoCta = false
	if err := guardar("8b-cuenta.png", nil); err != nil {
		return err
	}
	a.feeds[0].Videos, a.feeds[0].Cargado, a.feeds[0].Token = []Video{vids[2], vids[5], vids[4], vids[0], vids[3]}, true, "más"
	a.feeds[1].Videos, a.feeds[1].Cargado = cortos, true
	a.feeds[2].Cargando = true
	a.pant, a.foco, a.focoCuenta = pInicio, a.feeds[0].Clave, false
	if err := guardar("9-inicio-cuenta.png", nil); err != nil {
		return err
	}
	a.foco = "shorts"
	a.sel["shorts"] = 1
	if err := guardar("9b-inicio-shorts.png", nil); err != nil {
		return err
	}
	a.abrirTeclado()
	a.desdeBuscar = pInicio
	a.tec.borrador, a.tec.cursor = []rune("pan casero"), 10
	if err := guardar("2-buscar.png", nil); err != nil {
		return err
	}
	a.pant, a.consulta, a.res, a.selRes = pResultados, "pan casero", vids, 1
	if err := guardar("3-resultados.png", nil); err != nil {
		return err
	}
	// Viendo: una imagen inventada en medio, como la pondría el reproductor.
	v := vids[0]
	a.viendo, a.pant, a.volverA = &v, pViendo, pResultados
	a.rep.fuente = &fuenteVideo{Duracion: v.duracion()}
	a.rep.inicio, a.rep.escritos, a.rep.marcos = 323*time.Second, int64(retrasoSonido.Seconds()*bytesPorSeg), 1
	enMedio := func(w, h int) func() {
		cuadro := degradado(0.6, w, h)
		return func() { draw.Draw(a.l.img, areaVideo(w, h), cuadro, image.Point{}, draw.Src) }
	}
	if err := guardar("4-viendo.png", enMedio(anchoVideo, altoVideo)); err != nil {
		return err
	}
	a.volumen, a.volHasta = 70, time.Now().Add(time.Minute)
	a.rep.pausado = true
	if err := guardar("4b-pausa-volumen.png", enMedio(anchoVideo, altoVideo)); err != nil {
		return err
	}
	a.volHasta = time.Time{}
	a.rep.marcos, a.cargando, a.cargaDesde = 0, true, time.Now().Add(-7*time.Second)
	if err := guardar("5-abriendo.png", nil); err != nil {
		return err
	}
	// Un Short.
	c := cortos[1]
	a.viendo, a.cola, a.colaI, a.colaFeed = &c, cortos, 1, a.feeds[1]
	a.rep.fuente = &fuenteVideo{Duracion: c.duracion(), Vertical: true}
	a.rep.inicio, a.rep.marcos, a.rep.pausado, a.cargando = 12*time.Second, 1, false, false
	if err := guardar("10-short.png", enMedio(anchoVert, altoVert)); err != nil {
		return err
	}
	a.volumen, a.volHasta = 55, time.Now().Add(time.Minute)
	a.aviso("Cargando más Shorts…")
	if err := guardar("10b-short-volumen.png", enMedio(anchoVert, altoVert)); err != nil {
		return err
	}
	a.volHasta, a.avisoTexto = time.Time{}, ""
	a.rep.fuente, a.rep.marcos, a.cargando = nil, 0, true
	if err := guardar("10c-short-abriendo.png", nil); err != nil {
		return err
	}
	a.cargando = false
	a.pant, a.foco = pInicio, "descargados"
	a.preguntar("¿Borrar «"+vids[5].Titulo+"» de la consola?", nil)
	if err := guardar("6-borrar.png", nil); err != nil {
		return err
	}
	a.yt.cuenta = &cuenta{}
	a.alm.historial, a.alm.descargados = nil, nil
	a.pant, a.foco = pInicio, "buscar"
	return guardar("7-inicio-vacio.png", nil)
}

// degradado: una «miniatura» de colores para las pruebas.
func degradado(tono float64, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x)/float64(w)*0.6 + float64(y)/float64(h)*0.4
			c := hsv(math.Mod(tono+0.18*t, 1), 0.55, 0.9-0.5*t)
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	return img
}

func hsv(h, s, v float64) color.RGBA {
	i := math.Floor(h * 6)
	f := h*6 - i
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}
