package main

import (
	"strings"
	"time"
)

type pantalla int

const (
	pInicio     pantalla = iota // buscar, guardadas y las últimas
	pBuscar                     // teclado
	pResultados                 // lo que ha salido al buscar
	pPagina                     // una página
)

// visita: una página por la que has pasado (para volver con B a donde estabas).
type visita struct {
	url          string
	scroll, selOc int
}

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	luz    *luz
	alm    *almacen
	img    *imagenes
	tareas chan func()
	salir  chan struct{}

	pant pantalla

	// Inicio: 0 es la caja de buscar; luego las guardadas, las últimas y las sugerencias.
	selInicio int

	// Buscar y resultados
	tec         teclado
	desdeBuscar pantalla
	consulta    string
	res         []resultado
	selRes      int
	primeraRes  int
	buscando    bool
	errBusca    string

	// Página
	pag         *pagina
	maq         *maqueta
	scroll      int
	selOc       int // el enlace elegido (su ocurrencia en la maqueta), -1 ninguno
	cargando    bool
	cargandoURL string
	cargaDesde  time.Time
	errPag      string
	atras       []visita
	desdePagina pantalla
	memoria     map[string]*pagina // las últimas páginas (para volver sin bajarlas otra vez)
	ordenMem    []string
	turno       int

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
}

func nuevaApp(f *fuentes) *app {
	fuentesActuales = f
	a := &app{f: f, l: nuevoLienzo(f), tareas: make(chan func(), 256), salir: make(chan struct{}), cambios: true,
		luz: nuevaLuz(), selOc: -1, memoria: map[string]*pagina{}}
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
	a.avisoHasta = time.Now().Add(3500 * time.Millisecond)
	a.cambios = true
}

func (a *app) terminar() {
	select {
	case <-a.salir:
	default:
		close(a.salir)
	}
}

func (a *app) bucle(botones <-chan uint16, ra *retroarch) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	segundo := time.NewTicker(time.Second)
	defer segundo.Stop()
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
		case <-segundo.C:
			if a.cargando || a.buscando {
				a.cambios = true // los segundos que lleva
			}
		}
		if a.cambios && !a.luz.apagada {
			a.cambios = false
			a.dibujar()
			a.srv.enviarCon(a.l.marcoEn)
		}
	}
}

const flechas = bArriba | bAbajo | bIzquierda | bDerecha

func (a *app) botones(m uint16) {
	nuevos := m &^ a.pulsados
	a.pulsados = m
	if m&flechas != 0 {
		a.repetirEn = time.Now().Add(300 * time.Millisecond)
	}
	for b := uint16(1); b != 0 && b <= bR; b <<= 1 {
		if nuevos&b != 0 {
			a.pulsar(b)
		}
	}
}

// repetir: con una flecha mantenida, se repite (en las páginas, más rápido: se desliza).
func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
		return
	}
	paso := 90 * time.Millisecond
	if a.pant == pPagina {
		paso = 50 * time.Millisecond
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
	if a.luz.apagada {
		a.luz.encender()
		return
	}
	if b == bSelect && a.pant != pBuscar {
		a.luz.apagar()
		return
	}
	switch a.pant {
	case pInicio:
		a.pulsarInicio(b)
	case pBuscar:
		a.pulsarBuscar(b)
	case pResultados:
		a.pulsarResultados(b)
	case pPagina:
		a.pulsarPagina(b)
	}
}

// ---- Inicio -------------------------------------------------------------------------------------

// sugerencias: para empezar, si aún no hay nada guardado.
var sugerencias = []sitio{
	{Titulo: "Wikipedia en español", URL: "https://es.m.wikipedia.org/"},
	{Titulo: "Noticias (RTVE)", URL: "https://www.rtve.es/noticias/"},
	{Titulo: "El tiempo (AEMET)", URL: "https://www.aemet.es/es/eltiempo/prediccion/espana"},
}

type entradaInicio struct {
	s       sitio
	seccion string // «Guardadas», «Últimas», «Para empezar»
}

func (a *app) entradasInicio() []entradaInicio {
	var e []entradaInicio
	for _, s := range a.alm.guardadas {
		e = append(e, entradaInicio{s, "Guardadas"})
	}
	for i, s := range a.alm.historial {
		if i >= 20 {
			break
		}
		e = append(e, entradaInicio{s, "Últimas"})
	}
	if len(a.alm.guardadas) == 0 {
		for _, s := range sugerencias {
			e = append(e, entradaInicio{s, "Para empezar"})
		}
	}
	return e
}

func (a *app) pulsarInicio(b uint16) {
	e := a.entradasInicio()
	switch b {
	case bArriba:
		a.selInicio = max(a.selInicio-1, 0)
	case bAbajo:
		a.selInicio = min(a.selInicio+1, len(e))
	case bL:
		a.selInicio = max(a.selInicio-5, 0)
	case bR:
		a.selInicio = min(a.selInicio+5, len(e))
	case bA:
		if a.selInicio == 0 {
			a.abrirTeclado()
		} else if x := e[a.selInicio-1]; true {
			a.desdePagina = pInicio
			a.atras = nil
			a.abrir(x.s.URL, false)
		}
	case bStart:
		a.abrirTeclado()
	case bX:
		if a.selInicio > 0 {
			x := e[a.selInicio-1]
			switch x.seccion {
			case "Guardadas":
				a.alm.cambiarGuardada(x.s.Titulo, x.s.URL)
				a.aviso("Quitada de las guardadas")
			case "Últimas":
				a.alm.quitarHistorial(x.s.URL)
				a.aviso("Quitada de las últimas")
			}
			a.selInicio = min(a.selInicio, len(a.entradasInicio()))
		}
	case bY:
		if a.selInicio > 0 {
			x := e[a.selInicio-1]
			if x.seccion != "Guardadas" && !a.alm.guardada(x.s.URL) {
				a.alm.cambiarGuardada(x.s.Titulo, x.s.URL)
				a.aviso("Guardada")
			}
		}
	case bB:
		a.terminar()
	}
}

// ---- Buscar -------------------------------------------------------------------------------------

func (a *app) abrirTeclado() {
	a.desdeBuscar = a.pant
	a.tec.abrir()
	a.pant = pBuscar
}

func (a *app) pulsarBuscar(b uint16) {
	switch a.tec.pulsar(b) {
	case tecEnviar:
		q := strings.TrimSpace(a.tec.texto())
		if q == "" {
			return
		}
		if dir, ok := pareceDireccion(q); ok {
			a.desdePagina = a.desdeBuscar
			if a.desdePagina == pPagina {
				a.abrir(dir, true)
			} else {
				a.atras = nil
				a.abrir(dir, false)
			}
			return
		}
		a.buscar(q)
	case tecCancelar:
		a.pant = a.desdeBuscar
		if a.pant == pBuscar {
			a.pant = pInicio
		}
	}
}

func (a *app) buscar(q string) {
	if !hayRed() {
		a.aviso("Sin internet")
		return
	}
	a.consulta, a.res, a.selRes, a.primeraRes, a.errBusca, a.buscando = q, nil, 0, 0, "", true
	a.pant = pResultados
	a.turno++
	t := a.turno
	go func() {
		res, err := buscarWeb(q)
		a.hacer(func() {
			if t != a.turno {
				return
			}
			a.buscando = false
			if err != nil {
				logf("buscar %q: %v", q, err)
				a.errBusca = "No se ha podido buscar. Prueba otra vez."
				return
			}
			a.res = res
		})
	}()
}

func (a *app) pulsarResultados(b uint16) {
	n := len(a.res)
	switch b {
	case bArriba:
		a.selRes = max(a.selRes-1, 0)
	case bAbajo:
		a.selRes = min(a.selRes+1, max(n-1, 0))
	case bL:
		a.selRes = max(a.selRes-4, 0)
	case bR:
		a.selRes = min(a.selRes+4, max(n-1, 0))
	case bA:
		if a.selRes < n {
			a.desdePagina = pResultados
			a.atras = nil
			a.abrir(a.res[a.selRes].URL, false)
		}
	case bX, bStart:
		a.abrirTeclado()
	case bB:
		a.turno++
		a.buscando = false
		a.pant = pInicio
	}
}

// ---- Páginas ------------------------------------------------------------------------------------

// abrir baja una página (o la saca de la memoria). conAtras: la de ahora se apunta para volver.
func (a *app) abrir(dir string, conAtras bool) {
	a.abrirEn(dir, conAtras, 0, -1)
}

func (a *app) abrirEn(dir string, conAtras bool, scroll, selOc int) {
	if conAtras && a.pag != nil && a.pant == pPagina && !a.cargando {
		a.atras = append(a.atras, visita{url: a.pag.url, scroll: a.scroll, selOc: a.selOc})
		if len(a.atras) > 30 {
			a.atras = a.atras[1:]
		}
	}
	a.pant = pPagina
	a.img.olvidarCola()
	a.turno++
	t := a.turno
	if p, ok := a.memoria[dir]; ok {
		a.ponerPagina(p, scroll, selOc)
		return
	}
	if !hayRed() {
		a.cargando, a.errPag = false, "Sin internet"
		return
	}
	a.cargando, a.cargandoURL, a.cargaDesde, a.errPag = true, dir, time.Now(), ""
	go func() {
		p, err := abrirPagina(dir)
		a.hacer(func() {
			if t != a.turno {
				return
			}
			a.cargando = false
			if err != nil {
				logf("abrir %s: %v", dir, err)
				a.errPag = "No se ha podido abrir: " + err.Error()
				return
			}
			a.memoria[dir] = p
			a.memoria[p.url] = p
			a.ordenMem = append(a.ordenMem, dir, p.url)
			for len(a.ordenMem) > 16 {
				delete(a.memoria, a.ordenMem[0])
				a.ordenMem = a.ordenMem[1:]
			}
			a.ponerPagina(p, scroll, selOc)
		})
	}()
}

func (a *app) ponerPagina(p *pagina, scroll, selOc int) {
	a.pag, a.errPag, a.cargando = p, "", false
	a.maq = maquetar(p, a.f, a.img.tam)
	a.scroll = max(0, min(scroll, a.maq.alto-altoVista()))
	a.selOc = selOc
	if a.selOc >= len(a.maq.ocurrencias) || !a.ocVisible(a.selOc) {
		a.selOc = a.primeraVisible()
	}
	a.alm.apuntar(p.titulo, p.url)
}

// imagenLista: llegó una imagen; se rehace la maqueta sin que se mueva lo que estás leyendo.
func (a *app) imagenLista() {
	if a.pag == nil || a.maq == nil {
		return
	}
	bloque, desfase := -1, 0
	for _, ln := range a.maq.lineas {
		if ln.y+ln.alto > a.scroll {
			bloque, desfase = ln.bloque, a.scroll-ln.y
			break
		}
	}
	a.maq = maquetar(a.pag, a.f, a.img.tam)
	for _, ln := range a.maq.lineas {
		if ln.bloque == bloque {
			a.scroll = max(0, ln.y+desfase)
			break
		}
	}
	a.scroll = max(0, min(a.scroll, a.maq.alto-altoVista()))
}

func (a *app) ocVisible(i int) bool {
	if a.maq == nil || i < 0 || i >= len(a.maq.ocurrencias) {
		return false
	}
	ln := a.maq.lineas[a.maq.ocurrencias[i].linea]
	return ln.y >= a.scroll && ln.y+ln.alto <= a.scroll+altoVista()
}

func (a *app) primeraVisible() int {
	if a.maq == nil {
		return -1
	}
	for i := range a.maq.ocurrencias {
		if a.ocVisible(i) {
			return i
		}
	}
	return -1
}

func (a *app) desplazar(d int) {
	if a.maq == nil {
		return
	}
	a.scroll = max(0, min(a.scroll+d, a.maq.alto-altoVista()))
	if !a.ocVisible(a.selOc) {
		a.selOc = a.primeraVisible()
	}
}

// irAEnlace: al enlace anterior o siguiente (y la página se mueve para que se vea).
func (a *app) irAEnlace(d int) {
	if a.maq == nil || len(a.maq.ocurrencias) == 0 {
		return
	}
	i := a.selOc
	switch {
	case i < 0 && d > 0:
		i = 0
		for i < len(a.maq.ocurrencias) && a.maq.lineas[a.maq.ocurrencias[i].linea].y < a.scroll {
			i++
		}
	case i < 0:
		i = len(a.maq.ocurrencias) - 1
		for i > 0 && a.maq.lineas[a.maq.ocurrencias[i].linea].y > a.scroll+altoVista() {
			i--
		}
	default:
		i += d
	}
	if i < 0 || i >= len(a.maq.ocurrencias) {
		return
	}
	a.selOc = i
	ln := a.maq.lineas[a.maq.ocurrencias[i].linea]
	if ln.y < a.scroll {
		a.scroll = max(0, ln.y-40)
	} else if ln.y+ln.alto > a.scroll+altoVista() {
		a.scroll = min(ln.y+ln.alto-altoVista()+40, max(0, a.maq.alto-altoVista()))
	}
}

func (a *app) volver() {
	if n := len(a.atras); n > 0 {
		v := a.atras[n-1]
		a.atras = a.atras[:n-1]
		a.abrirEn(v.url, false, v.scroll, v.selOc)
		return
	}
	a.turno++
	a.cargando = false
	a.pant = a.desdePagina
	if a.pant == pPagina || a.pant == pBuscar {
		a.pant = pInicio
	}
}

func (a *app) pulsarPagina(b uint16) {
	if a.cargando || a.errPag != "" || a.maq == nil {
		switch b {
		case bB:
			a.volver()
		case bStart:
			a.abrirTeclado()
		}
		return
	}
	switch b {
	case bArriba:
		a.desplazar(-44)
	case bAbajo:
		a.desplazar(44)
	case bL:
		a.desplazar(-(altoVista() - 40))
	case bR:
		a.desplazar(altoVista() - 40)
	case bIzquierda:
		a.irAEnlace(-1)
	case bDerecha:
		a.irAEnlace(1)
	case bA:
		if a.ocVisible(a.selOc) {
			a.abrir(a.pag.enlaces[a.maq.ocurrencias[a.selOc].enlace], true)
		}
	case bY:
		if a.alm.cambiarGuardada(a.pag.titulo, a.pag.url) {
			a.aviso("Página guardada (sale en el inicio)")
		} else {
			a.aviso("Quitada de las guardadas")
		}
	case bX:
		a.scroll = 0
		a.selOc = a.primeraVisible()
	case bStart:
		a.abrirTeclado()
	case bB:
		a.volver()
	}
}
