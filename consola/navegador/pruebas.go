package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas (navegador -prueba carpeta): las pantallas con datos inventados, en PNG.
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	tmp, _ := os.MkdirTemp("", "navegador-prueba")
	defer os.RemoveAll(tmp)
	carpetaDatos = tmp
	redOK.Store(true)
	a := nuevaApp(f)
	a.cargaHasta = time.Time{}
	a.alm = nuevoAlmacen(tmp)
	a.img = &imagenes{app: a, listas: map[string]*image.RGBA{}, pedidas: map[string]bool{}, malas: map[string]bool{},
		cola: make(chan string, 64)}
	a.alm.cambiarGuardada("Pokémon Esmeralda - Wikipedia, la enciclopedia libre", "https://es.wikipedia.org/wiki/Pok%C3%A9mon_Esmeralda")
	a.alm.apuntar("Tortilla de patatas española, jugosa, alta, y esponjosa", "https://www.pequerecetas.com/receta/tortilla-de-patatas/")
	a.alm.apuntar("El tiempo en Madrid", "https://www.aemet.es/es/eltiempo/prediccion/municipios/madrid-id28079")

	guardar := func(nombre string) error {
		a.dibujar()
		fh, err := os.Create(filepath.Join(carpeta, nombre))
		if err != nil {
			return err
		}
		defer fh.Close()
		return png.Encode(fh, a.l.img)
	}
	a.pant, a.selInicio = pInicio, 1
	if err := guardar("1-inicio.png"); err != nil {
		return err
	}
	a.abrirTeclado()
	a.tec.borrador, a.tec.cursor = []rune("tortilla de patatas"), 19
	if err := guardar("2-buscar.png"); err != nil {
		return err
	}
	a.pant, a.consulta, a.selRes = pResultados, "tortilla de patatas", 0
	a.res = []resultado{
		{Titulo: "Tortilla de patatas española, jugosa, alta, y esponjosa - PequeRecetas", URL: "https://www.pequerecetas.com/receta/tortilla-de-patatas/",
			Resumen: "Aprende a hacer la mejor tortilla de patatas española con huevos, patatas y cebolla. Sigue nuestra receta paso a paso, con fotos y trucos."},
		{Titulo: "Tortilla de patatas con cebolla: receta tradicional española", URL: "https://www.directoalpaladar.com/recetas/tortilla",
			Resumen: "Receta sencilla de tortilla de patatas con cebolla para los que se inician en la cocina. Con fotos paso a paso."},
		{Titulo: "Tortilla de patatas tradicional jugosa", URL: "https://www.hogarmania.com/cocina/recetas/huevos/tortilla-patatas.html",
			Resumen: "Cómo preparar una tortilla de patatas tradicional, jugosa y con el punto justo de cuajado."},
		{Titulo: "Tortilla de patata · El cocinero casero", URL: "https://elcocinerocasero.com/receta/tortilla-de-patata/",
			Resumen: "Cómo hacer tortilla de patatas de manera fácil y con todas las fotos del paso a paso."},
	}
	if err := guardar("3-resultados.png"); err != nil {
		return err
	}
	// Una página inventada (como la de la receta), con una imagen ya bajada.
	foto := "https://ejemplo/tortilla.jpg"
	a.img.listas[foto] = degradado(anchoTexto(), 250)
	en := func(s string, e int) trozo { return trozo{texto: s, enlace: e} }
	tx := func(s string) trozo { return trozo{texto: s, enlace: -1} }
	p := &pagina{url: "https://www.pequerecetas.com/receta/tortilla-de-patatas/", titulo: "Tortilla de patatas española, jugosa, alta, y esponjosa - PequeRecetas",
		enlaces: []string{"https://www.pequerecetas.com/autor/ana", "https://www.pequerecetas.com/gastronomia-espanola",
			"https://www.pequerecetas.com/receta/tortilla-cebolla"},
		bloques: []bloque{
			{tipo: bTitulo1, trozos: []trozo{tx("Tortilla de patatas tradicional, jugosa y fácil")}},
			{tipo: bParrafo, trozos: []trozo{tx("Por "), en("Ana María", 0)}},
			{tipo: bParrafo, trozos: []trozo{tx("La tortilla de patatas (o tortilla española) es uno de esos platos fáciles y típicos de la "),
				en("gastronomía española", 1), tx(" que se conoce en todas partes. Para hacerla solo necesitamos tres ingredientes: huevos, patatas y un buen aceite (y cebolla si preferís la "),
				en("tortilla de patatas con cebolla", 2), tx(").")}},
			{tipo: bImagen, img: foto},
			{tipo: bTitulo2, trozos: []trozo{tx("Ingredientes")}},
			{tipo: bItem, nivel: 1, trozos: []trozo{tx("5 huevos")}},
			{tipo: bItem, nivel: 1, trozos: []trozo{tx("500 g de patatas")}},
			{tipo: bItem, nivel: 1, trozos: []trozo{tx("1 cebolla (opcional)")}},
		}}
	a.desdePagina = pResultados
	a.ponerPagina(p, 0, -1)
	a.pant = pPagina
	a.selOc = 1
	if err := guardar("4-pagina.png"); err != nil {
		return err
	}
	a.desplazar(300)
	if err := guardar("4b-pagina-abajo.png"); err != nil {
		return err
	}
	a.cargando, a.cargandoURL, a.cargaDesde = true, "https://es.wikipedia.org/wiki/Tortilla_de_patatas", time.Now().Add(-3*time.Second)
	if err := guardar("5-abriendo.png"); err != nil {
		return err
	}
	a.cargando = false
	a.alm.guardadas, a.alm.historial = nil, nil
	a.pant, a.selInicio = pInicio, 0
	return guardar("6-inicio-vacio.png")
}

// degradado: una «foto» de colores para las pruebas.
func degradado(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(200 - y*120/h), uint8(140 + x*60/w), uint8(60 + x*80/w), 255})
		}
	}
	redondear(img, 12)
	return img
}
