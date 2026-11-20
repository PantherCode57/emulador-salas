package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pantallas de la app.
type pantalla int

const (
	pSesion     pantalla = iota // pasar la sesión desde el PC
	pBiblioteca                 // las listas
	pLista                      // las canciones de una lista (o de una búsqueda)
	pSonando                    // reproductor
	pLetra                      // la letra a pantalla completa
	pBuscar                     // teclado para buscar
)

type lineaLetra struct {
	Ms    int    `json:"ms"`
	Texto string `json:"t"`
}

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	sp     *cuentaSpotify
	yt     *youtube
	alm    *almacen
	port   *portadas
	rep    *reproductor
	luz    *luz
	web    *paginaSesion
	tareas chan func()
	salir  chan struct{}

	pant, antes pantalla

	// Biblioteca
	selLista     int
	cargandoBibl bool

	// Lista abierta
	lista      Lista
	canciones  []Cancion
	selCancion int
	primera    int
	cargando   bool

	// Lo que suena
	cola      []Cancion
	colaI     int
	colaDe    string // nombre de la lista de la que sale
	buscando  bool   // buscando el sonido de la canción
	letra     []lineaLetra
	letraDe   string
	turnoPlay int // para no hacer caso a respuestas de canciones ya pasadas
	fallos    int

	// Descargas
	descCola     chan Cancion
	descPend     int
	descActual   string
	descPct      float64
	descFallidas int

	// Buscar
	tec teclado

	sesionEstado string // texto bajo la dirección en la pantalla de sesión

	volumen  int       // 0..100 (-1 si no se sabe)
	volHasta time.Time // hasta cuándo se ve la barra del volumen

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
}

func nuevaApp(f *fuentes) *app {
	a := &app{f: f, l: nuevoLienzo(f), tareas: make(chan func(), 256), salir: make(chan struct{}), cambios: true, volumen: -1,
		luz: nuevaLuz(), descCola: make(chan Cancion, 4096)}
	a.cargaHasta = time.Now().Add(1200 * time.Millisecond)
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
		case <-progreso.C:
			if a.rep.sonando() || a.descPend > 0 {
				a.cambios = true // la barra de progreso y la letra se mueven
			}
		}
		if a.cambios && !a.luz.apagada { // con la pantalla apagada no se dibuja (gasta menos)
			a.cambios = false
			a.dibujar()
			a.srv.enviar(a.l.marco())
		}
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

// repetir: con una flecha mantenida, se repite cada 70 ms (en el reproductor, cada 400 ms).
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
		return
	}
	paso := 70 * time.Millisecond
	if a.pant == pSonando {
		paso = 400 * time.Millisecond
	}
	a.repetirEn = time.Now().Add(paso)
	for _, b := range []uint16{bArriba, bAbajo, bIzquierda, bDerecha} {
		if a.pulsados&b != 0 {
			a.pulsar(b)
		}
	}
}

func (a *app) pulsar(b uint16) {
	a.cambios = true
	// Pantalla apagada: cualquier botón la enciende (y no hace nada más).
	if a.luz.apagada {
		a.luz.encender()
		return
	}
	if b == bSelect && a.pant != pBuscar {
		a.luz.apagar()
		return
	}
	switch a.pant {
	case pSesion:
		switch b {
		case bB:
			a.terminar()
		case bX: // sin sesión también se puede ver (y oír lo descargado) lo que ya hay en la consola
			if a.hayGuardado() {
				a.pant = pBiblioteca
			}
		}
	case pBiblioteca:
		a.pulsarBiblioteca(b)
	case pLista:
		a.pulsarLista(b)
	case pSonando:
		a.pulsarSonando(b)
	case pLetra:
		if b == bB || b == bY {
			a.pant = pSonando
		} else {
			a.pulsarSonando(b)
		}
	case pBuscar:
		a.pulsarBuscar(b)
	}
}

// ---- Sesión y biblioteca ----------------------------------------------------------------------

// arrancar: con sesión, a la biblioteca (de lo guardado y, si hay red, se actualiza); sin sesión,
// la pantalla para pasarla desde el PC.
func (a *app) arrancar() {
	if a.sp.tieneSesion() {
		a.pant = pBiblioteca
		a.actualizarBiblioteca()
		return
	}
	a.pant = pSesion
	a.abrirPaginaSesion()
}

func (a *app) abrirPaginaSesion() {
	if a.web != nil {
		return
	}
	a.sesionEstado = "Esperando la sesión…"
	a.web = &paginaSesion{validar: func(spdc string) error {
		a.hacer(func() { a.sesionEstado = "Comprobando la sesión con Spotify…" })
		err := a.sp.ponerCookie(spdc)
		a.hacer(func() {
			if err != nil {
				a.sesionEstado = "Spotify no ha aceptado esa sesión. Prueba otra vez."
				return
			}
			a.web.cerrar()
			a.web = nil
			a.pant = pBiblioteca
			a.aviso("Sesión de Spotify guardada")
			a.actualizarBiblioteca()
		})
		return err
	}}
	if err := a.web.abrir(); err != nil {
		a.sesionEstado = "No se pudo abrir la página: " + err.Error()
		logf("página de sesión: %v", err)
	}
}

func (a *app) listasBiblioteca() []Lista {
	ls := []Lista{{ID: "megusta", Nombre: "Tus me gusta", Tipo: tipoMeGusta}}
	if n := len(a.alm.descargadas()); n > 0 {
		ls = append(ls, Lista{ID: "descargadas", Nombre: "Descargadas", Tipo: tipoDescargadas, Total: n})
	}
	for _, l := range a.alm.listas() {
		if l.Tipo == tipoMeGusta {
			ls[0].Total = l.Total
			continue
		}
		ls = append(ls, l)
	}
	return ls
}

func (a *app) hayGuardado() bool {
	return len(a.alm.listas()) > 0 || len(a.alm.descargadas()) > 0
}

func (a *app) actualizarBiblioteca() {
	if a.cargandoBibl || !hayRed() || !a.sp.tieneSesion() {
		return
	}
	a.cargandoBibl = true
	go func() {
		ls, err := a.sp.biblioteca()
		a.hacer(func() {
			a.cargandoBibl = false
			if err != nil {
				logf("biblioteca: %v", err)
				if errors.Is(err, errSinSesion) {
					a.pant = pSesion
					a.abrirPaginaSesion()
					a.aviso("La sesión de Spotify ha caducado: pásala otra vez desde el PC")
				} else if len(a.alm.listas()) == 0 {
					a.aviso("No se pudo cargar tu biblioteca")
				}
				return
			}
			a.alm.guardarBiblioteca(ls)
			a.selLista = min(a.selLista, len(a.listasBiblioteca())-1)
		})
	}()
}

func (a *app) pulsarBiblioteca(b uint16) {
	ls := a.listasBiblioteca()
	switch b {
	case bArriba:
		a.selLista = max(a.selLista-1, 0)
	case bAbajo:
		a.selLista = min(a.selLista+1, len(ls)-1)
	case bL:
		a.selLista = max(a.selLista-5, 0)
	case bR:
		a.selLista = min(a.selLista+5, len(ls)-1)
	case bA:
		if a.selLista < len(ls) {
			a.abrirLista(ls[a.selLista])
		}
	case bY:
		a.abrirBuscar()
	case bX:
		if a.rep.sonando() || a.buscando {
			a.pant = pSonando
		}
	case bB:
		a.terminar()
	}
}

// ---- Lista abierta --------------------------------------------------------------------------------

func (a *app) abrirLista(l Lista) {
	a.lista, a.selCancion, a.primera, a.pant = l, 0, 0, pLista
	switch l.Tipo {
	case tipoDescargadas:
		a.canciones = a.alm.descargadas()
		return
	case tipoBusqueda:
		return // ya están en a.canciones
	}
	a.canciones = a.alm.cancionesDe(l.ID)
	if !hayRed() || !a.sp.tieneSesion() {
		if a.canciones == nil {
			a.aviso("Sin internet: esta lista no se ha abierto nunca en la consola")
		}
		return
	}
	a.cargando = true
	go func() {
		cs, err := a.sp.canciones(l)
		a.hacer(func() {
			a.cargando = false
			if err != nil {
				logf("lista %s: %v", l.ID, err)
				if a.canciones == nil {
					a.aviso("No se pudo abrir la lista")
				}
				return
			}
			// Se conservan los vídeos ya encontrados.
			antes := map[string]string{}
			for _, c := range a.alm.cancionesDe(l.ID) {
				if c.Video != "" {
					antes[c.ID] = c.Video
				}
			}
			for i := range cs {
				if cs[i].Video == "" {
					cs[i].Video = antes[cs[i].ID]
				}
			}
			a.alm.guardarCanciones(l.ID, cs)
			if a.pant == pLista && a.lista.ID == l.ID {
				a.canciones = cs
				a.selCancion = min(a.selCancion, max(len(cs)-1, 0))
			}
		})
	}()
}

func (a *app) pulsarLista(b uint16) {
	n := len(a.canciones)
	switch b {
	case bArriba:
		a.selCancion = max(a.selCancion-1, 0)
	case bAbajo:
		a.selCancion = min(a.selCancion+1, max(n-1, 0))
	case bL:
		a.selCancion = max(a.selCancion-5, 0)
	case bR:
		a.selCancion = min(a.selCancion+5, max(n-1, 0))
	case bA:
		if n > 0 {
			a.tocarCola(a.canciones, a.selCancion, a.lista.Nombre)
			a.pant = pSonando
		}
	case bY:
		if a.lista.Tipo == tipoBusqueda { // en lo buscado, solo la canción marcada
			if a.selCancion < n {
				a.descargar([]Cancion{a.canciones[a.selCancion]})
			}
		} else {
			a.descargar(a.canciones)
		}
	case bX:
		if a.rep.sonando() || a.buscando {
			a.pant = pSonando
		}
	case bB:
		if a.lista.Tipo == tipoBusqueda {
			a.abrirBuscar()
			return
		}
		a.pant = pBiblioteca
	}
}

// ---- Reproducir -----------------------------------------------------------------------------------

func (a *app) actual() *Cancion {
	if a.colaI >= 0 && a.colaI < len(a.cola) {
		return &a.cola[a.colaI]
	}
	return nil
}

func (a *app) tocarCola(cs []Cancion, i int, de string) {
	a.cola = append([]Cancion(nil), cs...)
	a.colaDe = de
	a.tocar(i)
}

// tocar pone la canción i de la cola: la descargada si está; si no, la busca en YouTube.
func (a *app) tocar(i int) {
	if i < 0 || i >= len(a.cola) {
		a.rep.parar()
		a.buscando = false
		return
	}
	a.colaI = i
	a.turnoPlay++
	turno := a.turnoPlay
	c := a.cola[i]
	a.letra, a.letraDe = nil, ""
	a.cargarLetra(c)
	if archivo, ok := a.alm.descargada(c.ID); ok {
		a.buscando = false
		if err := a.rep.tocar(&fuenteSonido{Ruta: archivo, Duracion: c.duracion()}, 0); err != nil {
			a.aviso("No se pudo tocar: " + err.Error())
		}
		a.prepararSiguiente()
		return
	}
	if !hayRed() {
		// Sin internet: a la siguiente que esté descargada.
		for j := i + 1; j < len(a.cola); j++ {
			if _, ok := a.alm.descargada(a.cola[j].ID); ok {
				a.aviso("Sin internet: salto a la siguiente descargada")
				a.tocar(j)
				return
			}
		}
		a.rep.parar()
		a.buscando = false
		a.aviso("Sin internet y esta canción no está descargada")
		return
	}
	a.rep.parar()
	a.buscando = true
	go func() {
		f, video, err := a.fuenteDe(c, false)
		a.hacer(func() {
			if turno != a.turnoPlay {
				return
			}
			a.buscando = false
			if err != nil {
				logf("sonido de %s: %v", c.Nombre, err)
				a.aviso("No encuentro «" + c.Nombre + "»: paso a la siguiente")
				a.tocar(a.colaI + 1)
				return
			}
			a.guardarVideo(c.ID, video)
			if err := a.rep.tocar(f, 0); err != nil {
				a.aviso("No se pudo tocar: " + err.Error())
			}
			a.prepararSiguiente()
		})
	}()
}

// fuenteDe busca el vídeo (si no se sabe) y pide su sonido. Con otra, busca otro vídeo distinto.
func (a *app) fuenteDe(c Cancion, otra bool) (*fuenteSonido, string, error) {
	video := c.Video
	if video == "" || otra {
		v, err := a.yt.buscar(c, video)
		if err != nil {
			return nil, "", err
		}
		video = v
	}
	f, _, err := a.yt.sonido(video)
	if err != nil {
		return nil, video, err
	}
	f.Duracion = c.duracion()
	return f, video, nil
}

func (a *app) guardarVideo(id, video string) {
	if video == "" {
		return
	}
	for i := range a.cola {
		if a.cola[i].ID == id {
			a.cola[i].Video = video
		}
	}
	for i := range a.canciones {
		if a.canciones[i].ID == id {
			a.canciones[i].Video = video
		}
	}
	go a.alm.recordarVideo(id, video)
}

// prepararSiguiente busca ya el vídeo de la siguiente, para que no haya espera entre canciones.
func (a *app) prepararSiguiente() {
	j := a.colaI + 1
	if j >= len(a.cola) || !hayRed() {
		return
	}
	c := a.cola[j]
	if _, ok := a.alm.descargada(c.ID); ok {
		return
	}
	if c.Video != "" {
		go func() { _, _, _ = a.yt.sonido(c.Video) }()
		return
	}
	go func() {
		v, err := a.yt.buscar(c, "")
		if err != nil {
			return
		}
		a.hacer(func() { a.guardarVideo(c.ID, v) })
		_, _, _ = a.yt.sonido(v) // la dirección del sonido, ya pedida para cuando toque
	}()
}

// alAcabar y alFallar los llama el reproductor desde su hilo.
func (a *app) cancionAcabada() {
	a.hacer(func() {
		a.fallos = 0
		a.tocar(a.colaI + 1)
	})
}

func (a *app) cancionFallida(err error) {
	a.hacer(func() {
		logf("se cortó la canción: %v", err)
		c := a.actual()
		if c == nil {
			return
		}
		a.fallos++
		if a.fallos > 2 {
			a.fallos = 0
			a.aviso("No se pudo tocar «" + c.Nombre + "»")
			a.tocar(a.colaI + 1)
			return
		}
		// La dirección del sonido caduca: se pide otra (la segunda vez, con otro vídeo).
		cc, turno, otra := *c, a.turnoPlay, a.fallos == 2
		a.yt.olvidar(cc.Video)
		a.buscando = true
		go func() {
			f, video, err := a.fuenteDe(cc, otra)
			a.hacer(func() {
				if turno != a.turnoPlay {
					return
				}
				a.buscando = false
				if err != nil {
					a.tocar(a.colaI + 1)
					return
				}
				a.guardarVideo(cc.ID, video)
				_ = a.rep.tocar(f, 0)
			})
		}()
	})
}

func (a *app) pulsarSonando(b uint16) {
	switch b {
	case bArriba:
		a.cambiarVolumen(5)
	case bAbajo:
		a.cambiarVolumen(-5)
	case bA:
		if a.rep.sonando() {
			a.rep.pausar(!a.rep.enPausa())
		} else if a.actual() != nil && !a.buscando {
			a.tocar(a.colaI)
		}
	case bR:
		if a.colaI+1 < len(a.cola) {
			a.tocar(a.colaI + 1)
		}
	case bL:
		if a.rep.posicion() > 4*time.Second || a.colaI == 0 {
			a.rep.saltar(-a.rep.posicion())
		} else {
			a.tocar(a.colaI - 1)
		}
	case bDerecha:
		a.rep.saltar(10 * time.Second)
	case bIzquierda:
		a.rep.saltar(-10 * time.Second)
	case bY:
		if a.pant == pSonando {
			a.pant = pLetra
		}
	case bX:
		if a.lista.ID != "" || a.lista.Tipo == tipoBusqueda {
			a.pant = pLista
		}
	case bB:
		if a.lista.ID != "" || a.lista.Tipo == tipoBusqueda {
			a.pant = pLista
		} else {
			a.pant = pBiblioteca
		}
	}
}

// ---- Letra --------------------------------------------------------------------------------------

func (a *app) cargarLetra(c Cancion) {
	ruta := filepath.Join(carpetaDatos, "letras", nombreSeguro(c.ID)+".json")
	if b, err := os.ReadFile(ruta); err == nil {
		var ls []lineaLetra
		if json.Unmarshal(b, &ls) == nil {
			a.letra, a.letraDe = ls, c.ID
			return
		}
	}
	if !hayRed() {
		return
	}
	go func() {
		ls, err := a.sp.letra(c)
		if err != nil {
			logf("letra de %s: %v", c.Nombre, err)
			ls = []lineaLetra{} // se guarda vacía: no se vuelve a pedir
		}
		b, _ := json.Marshal(ls)
		_ = os.MkdirAll(filepath.Dir(ruta), 0o700)
		escribirAtomico(ruta, b)
		a.hacer(func() {
			if a.actual() != nil && a.actual().ID == c.ID {
				a.letra, a.letraDe = ls, c.ID
			}
		})
	}()
}

// lineaActual: la línea de la letra que toca ahora (-1 si aún no empezó o no va sincronizada).
func (a *app) lineaActual() int {
	if len(a.letra) == 0 {
		return -1
	}
	ms := int(a.rep.posicion() / time.Millisecond)
	k := -1
	for i, l := range a.letra {
		if l.Ms <= ms {
			k = i
		}
	}
	return k
}

// ---- Descargas ------------------------------------------------------------------------------------

// descargar pone estas canciones en la cola de descargas (las que no estén ya).
func (a *app) descargar(cs []Cancion) {
	if !hayRed() {
		a.aviso("Sin internet no se puede descargar")
		return
	}
	n, nombre := 0, ""
	for _, c := range cs {
		if _, ok := a.alm.descargada(c.ID); ok {
			continue
		}
		select {
		case a.descCola <- c:
			n++
			nombre = c.Nombre
		default:
		}
	}
	switch {
	case n == 0 && len(cs) == 1:
		a.aviso("Esta canción ya está descargada")
		return
	case n == 0:
		a.aviso("Ya está todo descargado")
		return
	}
	a.descPend += n
	if n == 1 {
		a.aviso("Descargando " + nombre)
	} else {
		a.aviso(fmt.Sprintf("Descargando %d canciones", n))
	}
}

// descargador: baja las canciones de la cola una a una (en su propio hilo).
func (a *app) descargador() {
	for {
		var c Cancion
		select {
		case c = <-a.descCola:
		case <-a.salir:
			return
		}
		if _, ok := a.alm.descargada(c.ID); ok {
			a.hacer(func() { a.descPend = max(a.descPend-1, 0) })
			continue
		}
		a.hacer(func() { a.descActual, a.descPct = c.Nombre, 0 })
		err := a.bajar(c)
		a.hacer(func() {
			a.descPend = max(a.descPend-1, 0)
			a.descActual = ""
			if err != nil {
				a.descFallidas++
				logf("descarga de %s: %v", c.Nombre, err)
			}
			if a.descPend == 0 {
				if a.descFallidas > 0 {
					a.aviso(fmt.Sprintf("Descargas terminadas (%d no se pudieron)", a.descFallidas))
				} else {
					a.aviso("Descargas terminadas")
				}
				a.descFallidas = 0
			}
		})
	}
}

func (a *app) bajar(c Cancion) error {
	for intento := 0; intento < 2; intento++ {
		f, video, err := a.fuenteDe(c, intento == 1)
		if err != nil {
			if intento == 1 {
				return err
			}
			continue
		}
		a.hacer(func() { a.guardarVideo(c.ID, video) })
		ext := ".webm"
		if strings.Contains(f.Tipo, "mp4") {
			ext = ".m4a"
		}
		destino := filepath.Join(a.alm.musica, nombreSeguro(c.ID)+ext)
		err = descargar(f, destino, func(hecho, total int64) {
			if total > 0 {
				p := float64(hecho) / float64(total)
				a.hacer(func() { a.descPct = p })
			}
		})
		if err == nil {
			c.Video = video
			a.alm.apuntarDescarga(c, destino)
			// La portada también, para verla sin internet.
			if c.Portada != "" {
				a.port.pedir(c.Portada, 40, false)
			}
			return nil
		}
		logf("descarga de %s (intento %d): %v", c.Nombre, intento+1, err)
	}
	return errors.New("no se pudo descargar")
}

// ---- Buscar ---------------------------------------------------------------------------------------

func (a *app) abrirBuscar() {
	if !hayRed() {
		a.aviso("Sin internet no se puede buscar")
		return
	}
	a.antes = a.pant
	a.pant = pBuscar
	a.tec.abrir()
}

func (a *app) pulsarBuscar(b uint16) {
	switch a.tec.pulsar(b) {
	case tecCancelar:
		a.pant = pBiblioteca
		if a.antes == pLista || a.antes == pSonando {
			a.pant = a.antes
		}
	case tecEnviar:
		q := strings.TrimSpace(a.tec.texto())
		if q == "" {
			return
		}
		a.lista = Lista{ID: "", Nombre: "«" + q + "»", Tipo: tipoBusqueda}
		a.canciones, a.selCancion, a.primera, a.cargando = nil, 0, 0, true
		a.pant = pLista
		go func() {
			cs, err := a.sp.buscar(q)
			a.hacer(func() {
				a.cargando = false
				if err != nil {
					logf("buscar %q: %v", q, err)
					a.aviso("No se pudo buscar")
					return
				}
				if a.lista.Tipo == tipoBusqueda {
					a.canciones = cs
					if len(cs) == 0 {
						a.aviso("No hay resultados")
					}
				}
			})
		}()
	}
	if b == bB && len(a.tec.borrador) == 0 && a.pant == pBuscar {
		// B con la caja vacía: atrás.
		a.pant = pBiblioteca
	}
}
