package main

import (
	"image"
	"sync"
	"time"
)

// Pantallas de la app.
type pantalla int

const (
	pInicio     pantalla = iota // buscar, tus listas, seguir viendo y lo descargado
	pBuscar                     // teclado
	pResultados                 // lo que ha salido al buscar (o una fila entera de tu cuenta)
	pViendo                     // el vídeo
	pConfirmar                  // «¿Seguro?»
	pCuenta                     // tu cuenta de YouTube (pasarla desde el PC)
)

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	yt     *youtube
	alm    *almacen
	mini   *miniaturas
	rep    *reproductorVideo
	luz    *luz
	tareas chan func()
	salir  chan struct{}

	pant pantalla

	// Inicio: la fila elegida (por su clave, que no cambia al cargar más) y lo elegido en cada una.
	foco       string
	focoCuenta bool // en la fila de buscar, el botón «Cuenta» de la derecha
	sel        map[string]int
	feeds      []*feed

	// Tu cuenta
	web          *paginaSesion
	sesionEstado string
	cuentaMal    bool // YouTube dijo que la sesión ya no vale
	cambiandoCta bool // con cuenta, enseñando la página para pasar otra

	// Buscar y resultados
	tec         teclado
	consulta    string
	res         []Video
	resFeed     *feed // si la lista es una fila de tu cuenta (y no una búsqueda)
	selRes      int
	primeraRes  int
	token       string
	buscando    bool
	cargandoMas bool
	turnoBusca  int
	errBusca    string
	desdeBuscar pantalla // a dónde vuelve el teclado

	// Viendo
	viendo       *Video
	cola         []Video // de dónde salió (los Shorts pasan al siguiente)
	colaI        int
	colaFeed     *feed
	volverA      pantalla
	cargando     bool // pidiendo a yt-dlp de dónde se ve
	cargaDesde   time.Time
	turnoVideo   int // sube con cada vídeo: las respuestas de los anteriores no valen
	turnoRep     int // el turno del reproductor (cambia también al adelantar)
	intento      int
	desdeVideo   time.Duration
	guardadaEn   time.Time
	vv           vistaVideo
	terminaVideo bool

	// Adelantarse: lo que tarda yt-dlp (unos 20 s en la consola) se pide para el vídeo elegido
	// mientras miras la lista (y para el Short siguiente mientras ves uno), uno cada vez.
	preparar   string
	prepararEn time.Time
	preparando bool

	// Descargas
	descCola     chan Video
	descEnCola   map[string]bool
	descPend     int
	descActual   string
	descPct      float64
	descFallidas int

	// Confirmar
	confirmTexto  string
	confirmSi     func()
	confirmVolver pantalla

	volumen  int
	volHasta time.Time

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
}

// vistaVideo junta las imágenes del vídeo (las pone el reproductor desde su hilo) con lo que pinta
// la app alrededor (título, hora, barra de progreso).
type vistaVideo struct {
	mu      sync.Mutex
	activo  bool
	apagada bool   // pantalla apagada: no se manda nada (gasta menos)
	bandas  []byte // la pantalla entera de la app, en el formato del núcleo
	ultimo  []byte // la última imagen del vídeo (w×h)
	w, h    int
	cuando  time.Time
}

func nuevaApp(f *fuentes) *app {
	a := &app{f: f, l: nuevoLienzo(f), tareas: make(chan func(), 256), salir: make(chan struct{}), cambios: true, volumen: -1,
		luz: nuevaLuz(), descCola: make(chan Video, 256), descEnCola: map[string]bool{}, sel: map[string]int{},
		feeds: feedsCuenta(), foco: "buscar"}
	a.cargaHasta = time.Now().Add(1000 * time.Millisecond)
	return a
}

// hacer pide que algo se haga en el bucle de la app (desde otro hilo).
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

func (a *app) arrancar() {
	a.pant = pInicio
	a.foco = "buscar"
	if a.yt.cuenta.tiene() {
		a.foco = a.feeds[0].Clave
		a.cargarFeeds()
	} else if len(a.alm.historial) > 0 {
		a.foco = "historial"
	} else if len(a.alm.descargados) > 0 {
		a.foco = "descargados"
	}
}

// bucle: todo lo que cambia la app pasa por aquí, en un solo hilo.
func (a *app) bucle(botones <-chan uint16, ra *retroarch) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	progreso := time.NewTicker(500 * time.Millisecond)
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
			logf("RetroArch se ha cerrado (%v): salgo", err)
			a.terminar()
			return
		case fn := <-a.tareas:
			fn()
			a.cambios = true
		case m := <-botones:
			a.botones(m)
		case <-reloj.C:
			a.repetir()
			if !a.avisoHasta.IsZero() && time.Now().After(a.avisoHasta) {
				a.avisoHasta, a.avisoTexto, a.cambios = time.Time{}, "", true
			}
			if !a.cargaHasta.IsZero() && time.Now().After(a.cargaHasta) {
				a.cargaHasta, a.cambios = time.Time{}, true
			}
			if !a.volHasta.IsZero() && time.Now().After(a.volHasta) {
				a.volHasta, a.cambios = time.Time{}, true
			}
			a.adelantarse()
		case <-progreso.C:
			if a.pant == pViendo || a.descPend > 0 || a.buscando {
				a.cambios = true // la barra de progreso se mueve
			}
			if a.pant == pViendo && time.Since(a.guardadaEn) > 15*time.Second {
				a.guardarPosicion() // por si se apaga la consola a mitad
			}
		}
		if a.cambios && !a.luz.apagada { // con la pantalla apagada no se dibuja (gasta menos)
			a.cambios = false
			a.dibujar()
			a.enviarPantalla()
		}
	}
}

// enviarPantalla manda lo dibujado. Viendo un vídeo, lo dibujado es lo de alrededor: las imágenes
// las manda el reproductor (y aquí solo se repite la última si está en pausa o esperando).
func (a *app) enviarPantalla() {
	v := &a.vv
	if a.pant == pViendo && a.rep.hayImagen() {
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
	if !v.activo {
		return
	}
	v.ultimo, v.w, v.h, v.cuando = px, w, h, time.Now()
	if v.apagada || v.bandas == nil {
		if v.bandas == nil && !v.apagada { // la primera: que la app pinte ya lo de alrededor
			go a.hacer(func() {})
		}
		return
	}
	a.srv.enviarCon(v.pegar)
}

// areaVideo: dónde va la imagen del vídeo en la pantalla.
func areaVideo(w, h int) image.Rectangle {
	x0, y0 := (ancho-w)/2, (alto-h)/2
	return image.Rect(x0, y0, x0+w, y0+h)
}

// pegar: lo de alrededor y, en medio, la imagen del vídeo (con v.mu cogido).
func (v *vistaVideo) pegar(dst []byte) {
	if len(v.bandas) == len(dst) {
		copy(dst, v.bandas)
	}
	r := areaVideo(v.w, v.h)
	fila, filaV := ancho*4, v.w*4
	if r.Min.X == 0 && v.w == ancho {
		copy(dst[r.Min.Y*fila:r.Max.Y*fila], v.ultimo)
		return
	}
	for y := 0; y < v.h; y++ {
		o := (r.Min.Y+y)*fila + r.Min.X*4
		copy(dst[o:o+filaV], v.ultimo[y*filaV:(y+1)*filaV])
	}
}

const flechas = bArriba | bAbajo | bIzquierda | bDerecha

func (a *app) botones(m uint16) {
	nuevos := m &^ a.pulsados
	a.pulsados = m
	if m&flechas != 0 {
		a.repetirEn = time.Now().Add(350 * time.Millisecond)
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

// repetir: con una flecha mantenida, se repite cada 90 ms (viendo un vídeo, cada 400 ms; en los
// Shorts, arriba y abajo no se repiten).
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
		return
	}
	paso := 90 * time.Millisecond
	if a.pant == pViendo {
		paso = 400 * time.Millisecond
	}
	a.repetirEn = time.Now().Add(paso)
	for _, b := range []uint16{bArriba, bAbajo, bIzquierda, bDerecha} {
		if a.pulsados&b != 0 && !(a.pant == pViendo && a.enCorto() && (b == bArriba || b == bAbajo)) {
			a.pulsar(b)
		}
	}
}

// marcarPreparar: si te quedas un momento en este vídeo, se pide ya de dónde se ve.
func (a *app) marcarPreparar(v *Video) {
	if v == nil || v.Directo || a.alm.descargado(v.ID) != nil {
		a.preparar = ""
		return
	}
	if v.ID != a.preparar {
		a.preparar, a.prepararEn = v.ID, time.Now().Add(1200*time.Millisecond)
	}
}

func (a *app) adelantarse() {
	if a.preparar == "" || a.preparando || time.Now().Before(a.prepararEn) || !hayRed() {
		return
	}
	if a.pant == pViendo && a.cargando { // primero el que se va a ver
		return
	}
	id := a.preparar
	a.preparar = ""
	if a.yt.preparado(id) {
		return
	}
	a.preparando = true
	go func() {
		if _, err := a.yt.fuente(id, 0); err != nil {
			logf("adelantar %s: %v", id, err)
		}
		a.hacer(func() { a.preparando = false })
	}()
}

func (a *app) apagarPantalla(si bool) {
	if si {
		a.luz.apagar()
	} else {
		a.luz.encender()
	}
	a.vv.mu.Lock()
	a.vv.apagada = a.luz.apagada
	a.vv.mu.Unlock()
}

func (a *app) pulsar(b uint16) {
	a.cambios = true
	// Pantalla apagada: cualquier botón la enciende (y no hace nada más).
	if a.luz.apagada {
		a.apagarPantalla(false)
		return
	}
	if b == bSelect && a.pant != pBuscar && a.pant != pConfirmar {
		a.apagarPantalla(true)
		return
	}
	switch a.pant {
	case pInicio:
		a.pulsarInicio(b)
	case pBuscar:
		a.pulsarBuscar(b)
	case pResultados:
		a.pulsarResultados(b)
	case pViendo:
		a.pulsarViendo(b)
	case pCuenta:
		a.pulsarCuenta(b)
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

func (a *app) preguntar(texto string, si func()) {
	a.confirmTexto, a.confirmSi, a.confirmVolver = texto, si, a.pant
	a.pant = pConfirmar
}
