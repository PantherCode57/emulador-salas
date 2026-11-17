package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas (instagram -prueba carpeta): todas las pantallas con datos inventados, en PNG.
// Las fotos son degradados (no se baja nada) y los vídeos, un degradado puesto donde iría el vídeo.
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	redOK.Store(true)
	a := nuevaApp(f)
	a.cargaHasta = time.Time{}
	a.img = &imagenes{orig: map[string]image.Image{}, listo: map[string]*image.RGBA{}, pedidas: map[string]bool{}}
	a.ig = nuevoInstagram(&cuenta{})
	a.rep = nuevoReproductorVideo(func([]byte, int, int) {}, func(int) {}, func(int, error) {})
	n := 0
	foto := func(tono float64, w, h int) string {
		n++
		u := fmt.Sprintf("https://prueba/%d.jpg", n)
		a.img.orig[clave(u)] = degradado(tono, w, h)
		return u
	}
	cara := func(tono float64) string { return foto(tono, 150, 150) }
	ahora := time.Now()
	posts := []Post{
		{PK: "1", Usuario: "viajes.por.el.mundo", FotoUsuario: cara(0.05), Likes: 12873, Comentarios: 214, Cuando: ahora.Add(-3 * time.Hour),
			Texto:  "Atardecer en el mirador después de tres horas de subida. Mereció la pena cada paso 🌄 #montaña #viajes",
			Medios: []Medio{{Foto: foto(0.08, 480, 600), W: 1080, H: 1350}, {Foto: foto(0.6, 480, 600), W: 1080, H: 1350}, {Foto: foto(0.8, 480, 600), W: 1080, H: 1350}}},
		{PK: "2", Usuario: "la_cocina_de_ana", FotoUsuario: cara(0.3), Likes: 845, Comentarios: 32, MeGusta: true, Cuando: ahora.Add(-26 * time.Hour),
			Texto:  "Pan casero en 30 minutos. La receta completa en el vídeo.",
			Medios: []Medio{{Foto: foto(0.12, 480, 480), W: 1080, H: 1080, Video: "x", Duracion: 48}}},
		{PK: "3", Usuario: "pokeguias", FotoUsuario: cara(0.55), Likes: 3402, Cuando: ahora.Add(-50 * time.Minute), Sugerido: true,
			Medios: []Medio{{Foto: foto(0.45, 480, 270), W: 1920, H: 1080}}},
	}
	a.inicio = tira{posts: posts, cargada: true, mas: true}
	a.reels = tira{posts: []Post{{PK: "9", Usuario: "humor.diario", FotoUsuario: cara(0.7), Likes: 128400, Comentarios: 1520,
		Texto: "Cuando dices «cinco minutos más» y ya es de noche 😂", Medios: []Medio{{Foto: foto(0.9, 270, 480), W: 720, H: 1280, Video: "x", Duracion: 21}}},
		{PK: "10", Usuario: "otro", Medios: []Medio{{Foto: foto(0.2, 270, 480), Video: "x"}}}}, cargada: true}
	nombres := []string{"lucia.gm", "dani_23", "maria.p", "pablo.r", "sofia", "hugo.fdz", "carla.m", "alex_99", "nerea", "mateo.s", "irene.v"}
	for i, nm := range nombres {
		b := Bandeja{ID: fmt.Sprint(100 + i), Usuario: nm, Foto: cara(float64(i) / 11), Vista: i >= 7}
		if i == 0 {
			b.Items = []Post{{PK: "h1", Usuario: nm, Cuando: ahora.Add(-2 * time.Hour), Medios: []Medio{{Foto: foto(0.15, 270, 480), W: 1080, H: 1920}}},
				{PK: "h2", Usuario: nm, Medios: []Medio{{Foto: foto(0.5, 270, 480)}}}, {PK: "h3", Usuario: nm, Medios: []Medio{{Foto: foto(0.75, 270, 480)}}}}
		}
		a.bandejas = append(a.bandejas, b)
	}
	a.histHecha = true
	a.chats = []Chat{
		{ID: "c1", Titulo: "lucia.gm", Foto: cara(0.1), Ultimo: "¿Mañana vienes al final?", Cuando: ahora.Add(-4 * time.Minute), SinLeer: true},
		{ID: "c2", Titulo: "Clase 4ºB", Foto: cara(0.4), Ultimo: "dani_23: alguien tiene los deberes de mates", Cuando: ahora.Add(-90 * time.Minute), SinLeer: true},
		{ID: "c3", Titulo: "pablo.r", Foto: cara(0.65), Ultimo: "Tú: jajaja vale", Cuando: ahora.Add(-30 * time.Hour)},
		{ID: "c4", Titulo: "maria.p", Ultimo: "[Reel de @humor.diario]", Cuando: ahora.Add(-5 * 24 * time.Hour)},
	}
	a.chatsHecho = true
	chat := &Chat{ID: "c1", Titulo: "lucia.gm", Foto: cara(0.1), Mensajes: []Mensaje{
		{Texto: "Holaa", Cuando: ahora.Add(-30 * time.Minute)},
		{Texto: "Ya he hecho lo de ciencias, si quieres te lo paso luego", Cuando: ahora.Add(-29 * time.Minute)},
		{Mio: true, Texto: "Vale, gracias!! Yo estoy con lo de historia", Cuando: ahora.Add(-20 * time.Minute)},
		{Texto: "[Reel de @humor.diario]", Cuando: ahora.Add(-10 * time.Minute)},
		{Texto: "¿Mañana vienes al final?", Cuando: ahora.Add(-4 * time.Minute)},
	}}

	guardar := func(nombre string) error {
		a.dibujar()
		fh, err := os.Create(filepath.Join(carpeta, nombre+".png"))
		if err != nil {
			return err
		}
		defer fh.Close()
		return png.Encode(fh, a.l.img)
	}
	// un vídeo «puesto» (como si llegara del reproductor)
	conVideo := func(md Medio, x, y, maxW, maxH int) {
		w, h := encajar(md.W, md.H, maxW, maxH)
		a.reproduciendo = "prueba"
		a.vv.w, a.vv.h, a.vv.pos = w, h, image.Pt(x+(maxW-w)/2, y+(maxH-h)/2)
	}
	pintarVideo := func(tono float64) {
		r := a.areaVideo()
		g := degradado(tono, r.Dx(), r.Dy())
		a.l.pegar(g, nil, r.Min.X, r.Min.Y)
	}

	pasos := []struct {
		nombre string
		poner  func()
	}{
		{"01-carga", func() { a.cargaHasta = time.Now().Add(time.Hour) }},
		{"02-inicio-varias", func() { a.cargaHasta = time.Time{}; a.pant = pInicio; a.fotoDe["1"] = 1 }},
		{"03-inicio-video", func() { a.selInicio = 1 }},
		{"04-inicio-sugerencia", func() { a.selInicio = 2 }},
		{"05-inicio-cargando", func() { a.inicio = tira{cargando: true} }},
		{"06-reels", func() { a.inicio = tira{posts: posts, cargada: true}; a.pant = pReels }},
		{"07-historias", func() { a.pant = pHistorias; a.selHist = 1 }},
		{"08-historia", func() { a.pant, a.hb, a.hi = pHistoria, 0, 1; a.histDesde = time.Now().Add(-2 * time.Second) }},
		{"09-mensajes", func() { a.pant = pMensajes; a.selChat = 1 }},
		{"10-chat", func() { a.pant, a.chat = pChat, chat }},
		{"11-escribir", func() {
			a.pant = pEscribir
			for _, r := range "Sí, a las 5 en la puerta" {
				a.tec.escribir(r)
			}
			a.tec.fila, a.tec.col = 2, 3
		}},
		{"12-cuenta-sin", func() { a.pant = pCuenta; a.sesionEstado = "Esperando tu sesión…" }},
		{"13-cuenta", func() {
			a.ig.cuenta = &cuenta{Cookie: "x", Usuario: "tu.usuario", Nombre: "Tu nombre", Foto: cara(0.85)}
			a.pant = pCuenta
		}},
		{"14-confirmar", func() {
			a.preguntar("¿Cerrar la sesión de Instagram en la consola? (En el PC y el móvil sigues dentro.)", nil)
		}},
		{"15-sin-red", func() {
			a.pant = pMensajes
			redOK.Store(false)
			a.aviso("Tu sesión de Instagram ha caducado: pásala otra vez en «Tú»")
		}},
	}
	for _, p := range pasos {
		p.poner()
		if err := guardar(p.nombre); err != nil {
			return err
		}
	}
	redOK.Store(true)
	a.avisoTexto = ""
	// Con vídeo: se pinta la pantalla y encima un degradado donde iría el vídeo.
	a.pant = pVideo
	a.videoPost, a.videoMedio = posts[1], posts[1].Medios[0]
	conVideo(a.videoMedio, 0, 8, ancho, 400)
	a.dibujar()
	pintarVideo(0.12)
	if err := guardarPNG(filepath.Join(carpeta, "16-video.png"), a.l.img); err != nil {
		return err
	}
	a.pant = pReels
	conVideo(a.reels.posts[0].Medios[0], (ancho-anchoVert)/2, 0, anchoVert, altoVert)
	a.dibujar()
	pintarVideo(0.9)
	if err := guardarPNG(filepath.Join(carpeta, "17-reel-sonando.png"), a.l.img); err != nil {
		return err
	}
	fmt.Println("pantallas en", carpeta)
	return nil
}

func guardarPNG(ruta string, im image.Image) error {
	fh, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer fh.Close()
	return png.Encode(fh, im)
}

// degradado: una «foto» de prueba (dos colores en diagonal y unos círculos).
func degradado(tono float64, w, h int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	c1, c2 := hsv(tono, 0.55, 0.85), hsv(math.Mod(tono+0.12, 1), 0.7, 0.45)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := (float64(x)/float64(w) + float64(y)/float64(h)) / 2
			c := color.RGBA{uint8(float64(c1.R)*(1-t) + float64(c2.R)*t), uint8(float64(c1.G)*(1-t) + float64(c2.G)*t),
				uint8(float64(c1.B)*(1-t) + float64(c2.B)*t), 0xff}
			dx, dy := float64(x)-float64(w)*0.65, float64(y)-float64(h)*0.35
			if math.Hypot(dx, dy) < float64(min(w, h))*0.18 {
				c = color.RGBA{0xff, 0xf2, 0xd0, 0xff}
			}
			im.SetRGBA(x, y, c)
		}
	}
	return im
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
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 0xff}
}
