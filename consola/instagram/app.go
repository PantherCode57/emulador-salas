package main

import (
	"errors"
	"image"
	"sync"
	"time"
)

// Pantallas de la app. Las cinco primeras son las pestañas de arriba (L y R pasan de una a otra).
type pantalla int

const (
	pInicio    pantalla = iota // tu inicio: una publicación cada vez
	pReels                     // reels a pantalla completa, uno detrás de otro
	pHistorias                 // quién tiene historias
	pMensajes                  // tus chats
	pCuenta                    // tu cuenta (pasarla desde el PC)
	pHistoria                  // viendo historias
	pVideo                     // un vídeo del inicio, en grande
	pChat                      // una conversación
	pEscribir                  // el teclado para contestar
	pConfirmar                 // «¿Seguro?»
)

var pestanas = []pantalla{pInicio, pReels, pHistorias, pMensajes, pCuenta}

func esPestana(p pantalla) bool { return p <= pCuenta }

// tira: una lista que se va cargando por páginas (el inicio, los reels).
type tira struct {
	posts    []Post
	cursor   string
	mas      bool
	cargando bool
	cargada  bool
	err      string
}

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	ig     *instagram
	img    *imagenes
	rep    *reproductorVideo
	tareas chan func()
	salir  chan struct{}

	pant pantalla

	// Inicio
	inicio    tira
	selInicio int
	fotoDe    map[string]int // en las de varias fotos, cuál se está viendo (por publicación)

	// Reels
	reels   tira
	selReel int

	// Historias
	bandejas   []Bandeja
	histCarga  bool
	histHecha  bool
	histErr    string
	selHist    int
	hb, hi     int       // viendo: de quién y cuál
	histDesde  time.Time // cuándo empezó la de ahora
	histPausa  time.Duration
	pedidaHist bool

	// Mensajes
	chats      []Chat
	chatsCarga bool
	chatsHecho bool
	chatsErr   string
	chatsEn    time.Time
	selChat    int
	leidos     map[string]bool // los que has abierto aquí (Instagram no se entera)
	chat       *Chat
	chatCarga  bool
	chatErr    string
	chatEn     time.Time
	subir      int // cuánto se ha subido en la conversación (px)
	tec        teclado
	enviando   bool

	// Tu cuenta
	web          *paginaSesion
	sesionEstado string
	cuentaMal    bool
	cambiandoCta bool

	// Vídeo (reels, vídeos del inicio e historias)
	vv            vistaVideo
	reproduciendo string // qué suena (para no volver a empezarlo)
	fuente        *fuenteVideo
	turnoRep      int
	videoPost     Post
	videoMedio    Medio
	videoVolver   pantalla
	videoErr      string

	// Confirmar
	confirmTexto  string
	confirmSi     func()
	confirmVolver pantalla

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
	lrPend    int       // L o R esperando a ver si es L1 + R1 (la captura): -1, +1 o 0
	lrEn      time.Time // cuándo se cumple
}

// vistaVideo junta las imágenes del vídeo (las pone el reproductor desde su hilo) con lo que pinta
// la app alrededor.
type vistaVideo struct {
	mu     sync.Mutex
	activo bool
	bandas []byte // la pantalla entera de la app, en el formato del núcleo
	ultimo []byte // la última imagen del vídeo (w×h)
	w, h   int
	pos    image.Point // dónde va
	cuando time.Time
}

func nuevaApp(f *fuentes) *app {
	a := &app{f: f, l: nuevoLienzo(f), tareas: make(chan func(), 256), salir: make(chan struct{}), cambios: true,
		fotoDe: map[string]int{}, leidos: map[string]bool{}}
	a.cargaHasta = time.Now().Add(900 * time.Millisecond)
	return a
}

func (a *app) hacer(fn func()) {
	select {
	case a.tareas <- fn:
	case <-a.salir:
	}
}

func (a *app) aviso(s string) {
	a.avisoTexto = s
	a.avisoHasta = time.Now().Add(4 * time.Second)
	a.cambios = true
}

func (a *app) terminar() {
	select {
	case <-a.salir:
	default:
		close(a.salir)
	}
}

// fallo: lo que se enseña cuando algo no ha salido (y si es la sesión, se avisa de pasarla otra vez).
func (a *app) fallo(err error) string {
	switch {
	case errors.Is(err, errSesion):
		if !a.cuentaMal {
			a.cuentaMal = true
			a.aviso("Tu sesión de Instagram ha caducado: pásala otra vez en «Tú»")
		}
		return "Tu sesión ha caducado: pásala otra vez en la pestaña «Tú»."
	case errors.Is(err, errMensajes):
		return "Instagram no deja ver los mensajes desde la consola (los ha cambiado de sitio). De momento, míralos en el móvil."
	case errors.Is(err, errSinCuenta):
		return "Primero pasa tu cuenta desde el PC (pestaña «Tú»)."
	case !hayRed():
		return "Sin internet."
	}
	return "No ha cargado: " + err.Error()
}

func (a *app) arrancar() {
	if !a.ig.cuenta.tiene() {
		a.irA(pCuenta)
		return
	}
	a.irA(pInicio)
	a.cargarHistorias() // para que las pestañas salgan ya hechas
	a.cargarChats()
}

// bucle: todo lo que cambia la app pasa por aquí, en un solo hilo.
func (a *app) bucle(botones <-chan uint16, ra *retroarch) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	progreso := time.NewTicker(250 * time.Millisecond)
	defer progreso.Stop()
	var raHecho <-chan error
	if ra != nil {
		raHecho = ra.hecho
	}
	for {
		select {
		case <-a.salir:
			return
		case err := <-raHecho:
			logf("la pantalla se ha cerrado (%v): salgo", err)
			a.terminar()
			return
		case fn := <-a.tareas:
			fn()
			a.cambios = true
		case m := <-botones:
			a.botones(m)
		case <-reloj.C:
			a.repetir()
			a.pestanaPendiente()
			ahora := time.Now()
			if !a.avisoHasta.IsZero() && ahora.After(a.avisoHasta) {
				a.avisoHasta, a.avisoTexto, a.cambios = time.Time{}, "", true
			}
			if !a.cargaHasta.IsZero() && ahora.After(a.cargaHasta) {
				a.cargaHasta, a.cambios = time.Time{}, true
			}
			if a.pant == pHistoria {
				a.avanzarHistoria()
			}
		case <-progreso.C:
			switch a.pant {
			case pReels, pVideo, pHistoria:
				a.cambios = true // la barra de progreso se mueve
			case pMensajes:
				if time.Since(a.chatsEn) > time.Minute && !a.chatsCarga {
					a.cargarChats()
				}
			case pChat:
				if a.chat != nil && time.Since(a.chatEn) > 10*time.Second && !a.chatCarga {
					a.cargarChat(false)
				}
			}
		}
		if a.cambios {
			a.cambios = false
			a.dibujar()
			a.enviarPantalla()
		}
	}
}

// enVideo: lo de ahora lleva un vídeo encima (lo pega el reproductor).
func (a *app) enVideo() bool {
	return a.reproduciendo != "" && (a.pant == pReels || a.pant == pVideo || a.pant == pHistoria)
}

// enviarPantalla manda lo dibujado. Con un vídeo, lo dibujado es lo de alrededor: las imágenes las
// manda el reproductor (y aquí solo se repite la última si está en pausa o esperando).
func (a *app) enviarPantalla() {
	v := &a.vv
	if a.enVideo() && a.rep.hayImagen() {
		v.mu.Lock()
		if len(v.bandas) != len(a.l.img.Pix) {
			v.bandas = make([]byte, len(a.l.img.Pix))
		}
		a.l.marcoEn(v.bandas)
		if v.ultimo != nil && (a.rep.enPausa() || time.Since(v.cuando) > 150*time.Millisecond) {
			a.srv.enviarCon(v.pegar)
		}
		v.mu.Unlock()
		return
	}
	a.srv.enviarCon(a.l.marcoEn)
}

// componer (desde el hilo del reproductor): una imagen nueva del vídeo con lo de alrededor.
func (a *app) componer(px []byte, w, h int) {
	v := &a.vv
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.activo || w != v.w || h != v.h {
		return
	}
	v.ultimo, v.cuando = px, time.Now()
	if v.bandas == nil {
		go a.hacer(func() {}) // la primera: que la app pinte ya lo de alrededor
		return
	}
	a.srv.enviarCon(v.pegar)
}

// pegar: lo de alrededor y, encima, la imagen del vídeo (con v.mu cogido).
func (v *vistaVideo) pegar(dst []byte) {
	if len(v.bandas) == len(dst) {
		copy(dst, v.bandas)
	}
	if v.ultimo == nil || len(v.ultimo) < v.w*v.h*4 {
		return
	}
	fila, filaV := ancho*4, v.w*4
	for y := 0; y < v.h; y++ {
		yy := v.pos.Y + y
		if yy < 0 || yy >= alto {
			continue
		}
		o := yy*fila + v.pos.X*4
		copy(dst[o:o+filaV], v.ultimo[y*filaV:(y+1)*filaV])
	}
}

// encajar: el tamaño de un vídeo de w×h dentro de maxW×maxH (en números pares, como pide ffmpeg).
func encajar(w, h, maxW, maxH int) (int, int) {
	if w <= 0 || h <= 0 {
		w, h = 9, 16
	}
	k := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
	ow, oh := int(float64(w)*k)&^1, int(float64(h)*k)&^1
	return max(ow, 2), max(oh, 2)
}

// tocarVideo pone un vídeo en el rectángulo de maxW×maxH que empieza en (x, y) (centrado dentro).
func (a *app) tocarVideo(clave string, md Medio, x, y, maxW, maxH int) {
	if a.reproduciendo == clave || md.Video == "" {
		return
	}
	w, h := encajar(md.W, md.H, maxW, maxH)
	f := &fuenteVideo{Video: md.Video, Duracion: time.Duration(md.Duracion * float64(time.Second)), FPS: 30,
		Vertical: md.H > md.W, AnchoSal: w, AltoSal: h, Cabeceras: map[string]string{"User-Agent": agenteNavegador}}
	a.vv.mu.Lock()
	a.vv.activo, a.vv.ultimo, a.vv.bandas, a.vv.w, a.vv.h = true, nil, nil, w, h // bandas: las del vídeo anterior no valen
	a.vv.pos = image.Pt(x+(maxW-w)/2, y+(maxH-h)/2)
	a.vv.mu.Unlock()
	turno, err := a.rep.tocar(f, 0)
	a.turnoRep, a.reproduciendo, a.fuente, a.videoErr = turno, clave, f, ""
	if err != nil {
		logf("vídeo: %v", err)
		a.videoErr = "No se ha podido ver el vídeo"
	}
}

func (a *app) pararVideo() {
	if a.reproduciendo == "" {
		return
	}
	a.rep.parar()
	a.reproduciendo, a.fuente = "", nil
	a.vv.mu.Lock()
	a.vv.activo, a.vv.ultimo = false, nil
	a.vv.mu.Unlock()
}

// areaVideo: dónde está el vídeo de ahora (para pintar alrededor).
func (a *app) areaVideo() image.Rectangle {
	a.vv.mu.Lock()
	defer a.vv.mu.Unlock()
	return image.Rect(a.vv.pos.X, a.vv.pos.Y, a.vv.pos.X+a.vv.w, a.vv.pos.Y+a.vv.h)
}

// videoAcabado (desde el reproductor): los reels y los vídeos vuelven a empezar (como en el móvil); en
// las historias se pasa a la siguiente.
func (a *app) videoAcabado(turno int) {
	a.hacer(func() {
		if turno != a.turnoRep || a.fuente == nil {
			return
		}
		if a.pant == pHistoria {
			a.siguienteHistoria(1)
			return
		}
		t, err := a.rep.tocar(a.fuente, 0)
		if err == nil {
			a.turnoRep = t
		}
	})
}

func (a *app) videoFallido(turno int, err error) {
	a.hacer(func() {
		if turno != a.turnoRep {
			return
		}
		logf("vídeo: %v", err)
		a.videoErr = "No se ha podido ver el vídeo"
		if a.pant == pHistoria {
			a.siguienteHistoria(1)
		}
	})
}

// ---- Botones -------------------------------------------------------------------------------------

const flechas = bArriba | bAbajo | bIzquierda | bDerecha

func (a *app) botones(m uint16) {
	nuevos := m &^ a.pulsados
	a.pulsados = m
	if m&flechas != 0 {
		a.repetirEn = time.Now().Add(350 * time.Millisecond)
	}
	// L y R juntos son la captura de pantalla (la hace el servicio): no cambian de pestaña.
	if m&(bL|bR) == bL|bR {
		a.lrPend = 0
		nuevos &^= bL | bR
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

// repetir: con una flecha mantenida, se repite cada 90 ms (en los reels y las historias, no).
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) {
		return
	}
	if a.pant == pReels || a.pant == pHistoria || a.pant == pVideo || a.pant == pInicio {
		return
	}
	a.repetirEn = time.Now().Add(90 * time.Millisecond)
	for _, b := range []uint16{bArriba, bAbajo, bIzquierda, bDerecha} {
		if a.pulsados&b != 0 {
			a.pulsar(b)
		}
	}
}

func (a *app) pulsar(b uint16) {
	a.cambios = true
	if esPestana(a.pant) && (b == bL || b == bR) {
		a.lrPend, a.lrEn = 1, time.Now().Add(140*time.Millisecond)
		if b == bL {
			a.lrPend = -1
		}
		return
	}
	switch a.pant {
	case pInicio:
		a.pulsarInicio(b)
	case pReels:
		a.pulsarReels(b)
	case pHistorias:
		a.pulsarHistorias(b)
	case pMensajes:
		a.pulsarMensajes(b)
	case pCuenta:
		a.pulsarCuenta(b)
	case pHistoria:
		a.pulsarHistoria(b)
	case pVideo:
		a.pulsarVideo(b)
	case pChat:
		a.pulsarChat(b)
	case pEscribir:
		a.pulsarEscribir(b)
	case pConfirmar:
		switch b {
		case bA:
			a.pant = a.confirmVolver
			if a.confirmSi != nil {
				a.confirmSi()
			}
		case bB, bSelect:
			a.pant = a.confirmVolver
		}
	}
}

// pestanaPendiente: L o R cambian de pestaña si en un momento no se ha pulsado el otro (L1 + R1 es
// la captura).
func (a *app) pestanaPendiente() {
	if a.lrPend == 0 || time.Now().Before(a.lrEn) {
		return
	}
	d := a.lrPend
	a.lrPend = 0
	if !esPestana(a.pant) {
		return
	}
	i := 0
	for k, p := range pestanas {
		if p == a.pant {
			i = k
		}
	}
	a.irA(pestanas[(i+d+len(pestanas))%len(pestanas)])
	a.cambios = true
}

// irA cambia de pestaña (y deja o pone lo que tenga que sonar).
func (a *app) irA(p pantalla) {
	if a.pant == pCuenta && p != pCuenta {
		a.cerrarPagina()
	}
	a.pant = p
	if p != pReels {
		a.pararVideo()
	}
	switch p {
	case pInicio:
		if !a.inicio.cargada && !a.inicio.cargando {
			a.cargarInicio(false)
		}
	case pReels:
		if !a.reels.cargada && !a.reels.cargando {
			a.cargarReels(false)
		}
		a.ponerReel()
	case pHistorias:
		if !a.histHecha && !a.histCarga {
			a.cargarHistorias()
		}
	case pMensajes:
		if !a.chatsCarga && (!a.chatsHecho || time.Since(a.chatsEn) > 30*time.Second) {
			a.cargarChats()
		}
	case pCuenta:
		a.cambiandoCta = false
		if !a.ig.cuenta.tiene() || a.cuentaMal {
			a.abrirPagina()
		}
	}
}

func (a *app) preguntar(texto string, si func()) {
	a.confirmTexto, a.confirmSi, a.confirmVolver = texto, si, a.pant
	a.pant = pConfirmar
}

// meGusta pone o quita tu «me gusta» (se ve ya; si Instagram no lo acepta, se deshace).
func (a *app) meGusta(p *Post) {
	if p == nil || p.PK == "" {
		return
	}
	p.MeGusta = !p.MeGusta
	if p.MeGusta {
		p.Likes++
	} else if p.Likes > 0 {
		p.Likes--
	}
	pk, si := p.PK, p.MeGusta
	go func() {
		err := a.ig.MeGusta(pk, si)
		a.hacer(func() {
			if err == nil {
				return
			}
			logf("me gusta: %v", err)
			for _, t := range []*tira{&a.inicio, &a.reels} {
				for i := range t.posts {
					if t.posts[i].PK == pk && t.posts[i].MeGusta == si {
						t.posts[i].MeGusta = !si
						if si && t.posts[i].Likes > 0 {
							t.posts[i].Likes--
						} else if !si {
							t.posts[i].Likes++
						}
					}
				}
			}
			if a.videoPost.PK == pk {
				a.videoPost.MeGusta = !si
			}
			a.aviso(a.fallo(err))
		})
	}()
}
