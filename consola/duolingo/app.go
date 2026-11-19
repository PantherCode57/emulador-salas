package main

import (
	"errors"
	"time"
)

type pantalla int

const (
	pCuenta   pantalla = iota // pasar la sesión desde el PC o el móvil
	pInicio                   // el camino de tu curso
	pPerfil                   // tu racha, tus XP, tu liga
	pCargando                 // preparando una lección
	pLeccion
	pFin      // lección acabada
	pSinVidas // te has quedado sin vidas
)

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	luz    *luz
	duo    *duolingo
	son    *sonido
	web    *paginaSesion
	tareas chan func()
	salir  chan struct{}

	pant pantalla

	// lo que se sabe de tu cuenta
	perfil      perfil
	perfilHecho bool
	camino      []unidad
	unidad      int // la que se ve
	nivelSel    int // el nivel elegido en esa unidad
	cargando    bool
	errCarga    string
	cuentaMal   bool
	sesionTexto string

	// la lección de ahora
	lec     *leccion
	t       *turno
	tec     teclado
	fin     resultadoFin
	errLec  string
	pedido  int // sube con cada lección pedida (para no hacer caso a una que llega tarde)
	enviado bool

	confirmTexto string
	confirmSi    func()

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios    bool
	puntoVisto int64
	pulsados   uint16
	repetirEn  time.Time
}

func nuevaApp(f *fuentes, duo *duolingo) *app {
	a := &app{f: f, l: nuevoLienzo(f), duo: duo, son: nuevoSonido(), tareas: make(chan func(), 64),
		salir: make(chan struct{}), cambios: true, luz: nuevaLuz()}
	a.cargaHasta = time.Now().Add(800 * time.Millisecond)
	return a
}

// hacer: que el hilo de la app haga esto (desde otro hilo).
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

func (a *app) confirmar(texto string, si func()) {
	a.confirmTexto, a.confirmSi = texto, si
}

func (a *app) arrancar() {
	if !a.duo.cuenta.tiene() {
		a.irACuenta()
		return
	}
	a.pant = pInicio
	a.cargar()
}

// cargar pide tu perfil y tu camino (sin parar la app).
func (a *app) cargar() {
	if a.cargando {
		return
	}
	a.cargando, a.errCarga = true, ""
	go func() {
		p, err := a.duo.Perfil()
		var us []unidad
		actual := 0
		if err == nil {
			us, actual, err = a.duo.Camino()
		}
		a.hacer(func() {
			a.cargando = false
			if err != nil {
				a.errCarga = textoError(err)
				if errors.Is(err, errSesion) {
					a.cuentaMal = true
				}
				if p.Usuario != "" {
					a.perfil, a.perfilHecho = p, true
				}
				return
			}
			nuevo := !a.perfilHecho || len(a.camino) == 0
			if m := a.duo.nombreMateria(); m != "" {
				p.Curso = m
			}
			a.perfil, a.perfilHecho, a.camino = p, true, us
			if nuevo || a.unidad >= len(us) {
				a.unidad = actual
				a.nivelSel = nivelActual(us, actual)
			}
			a.nivelSel = min(a.nivelSel, max(0, len(a.unidadVista().Niveles)-1))
		})
	}()
}

func textoError(err error) string {
	switch {
	case errors.Is(err, errSesion):
		return "La sesión ha caducado: ve a Perfil (R) y pásala otra vez."
	case !hayRed():
		return "Sin internet."
	}
	return "No ha cargado: " + err.Error()
}

func (a *app) unidadVista() unidad {
	if a.unidad < 0 || a.unidad >= len(a.camino) {
		return unidad{}
	}
	return a.camino[a.unidad]
}

func nivelActual(us []unidad, u int) int {
	if u < 0 || u >= len(us) {
		return 0
	}
	for i, n := range us[u].Niveles {
		if n.Estado == estActivo {
			return i
		}
	}
	for i, n := range us[u].Niveles {
		if n.Estado != estHecho {
			return i
		}
	}
	return 0
}

// ---- Tu cuenta -----------------------------------------------------------------------------------

func (a *app) irACuenta() {
	a.pant = pCuenta
	if a.web != nil {
		return
	}
	a.sesionTexto = "Esperando tu sesión…"
	a.web = &paginaSesion{validar: func(texto string) error {
		a.hacer(func() { a.sesionTexto = "Comprobando la sesión con Duolingo…" })
		err := a.duo.ponerCuenta(texto)
		a.hacer(func() {
			if err != nil {
				a.sesionTexto = "Duolingo no ha aceptado esa sesión. Prueba otra vez."
				return
			}
			a.cuentaMal = false
			a.cerrarPagina()
			if u := a.duo.cuenta.Usuario; u != "" {
				a.aviso("¡Hola, " + u + "!")
			}
			a.perfilHecho, a.camino = false, nil
			a.pant = pInicio
			a.cargar()
		})
		return err
	}}
	if err := a.web.abrir(); err != nil {
		a.sesionTexto = "No se pudo abrir la página: " + err.Error()
		logf("página de la cuenta: %v", err)
	}
}

func (a *app) cerrarPagina() {
	a.web.cerrar()
	a.web = nil
}

// ---- El bucle ------------------------------------------------------------------------------------

func (a *app) bucle(botones <-chan uint16, ra *retroarch) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
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
			ahora := time.Now()
			if !a.avisoHasta.IsZero() && ahora.After(a.avisoHasta) {
				a.avisoHasta, a.avisoTexto, a.cambios = time.Time{}, "", true
			}
			if !a.cargaHasta.IsZero() && ahora.After(a.cargaHasta) {
				a.cargaHasta, a.cambios = time.Time{}, true
			}
			if a.t != nil && !a.t.malHasta.IsZero() && ahora.After(a.t.malHasta) {
				a.t.malHasta, a.t.malIzq, a.t.malDer, a.cambios = time.Time{}, -1, -1, true
			}
			if a.pant == pLeccion && a.t != nil && a.t.ajedrez != nil && !a.t.ajedrez.rivalEn.IsZero() {
				a.moverRival()
				a.cambios = true
			}
			if a.pant == pCargando || a.cargando || (a.pant == pFin && !a.enviado) {
				if p := ahora.UnixMilli() / 400; p != a.puntoVisto { // los puntitos (no hace falta más)
					a.puntoVisto, a.cambios = p, true
				}
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

func (a *app) repetir() {
	if a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
		return
	}
	a.repetirEn = time.Now().Add(110 * time.Millisecond)
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
	if a.confirmSi != nil {
		switch b {
		case bA:
			si := a.confirmSi
			a.confirmTexto, a.confirmSi = "", nil
			si()
		case bB:
			a.confirmTexto, a.confirmSi = "", nil
		}
		return
	}
	switch a.pant {
	case pCuenta:
		if b == bB {
			if a.duo.cuenta.tiene() {
				a.cerrarPagina()
				a.pant = pPerfil
			} else {
				a.terminar()
			}
		}
	case pInicio:
		a.pulsarInicio(b)
	case pPerfil:
		a.pulsarPerfil(b)
	case pCargando:
		if b == bB {
			a.pedido++
			a.pant = pInicio
		}
	case pLeccion:
		a.pulsarLeccion(b)
	case pFin, pSinVidas:
		if b == bA || b == bB || b == bStart {
			if a.pant == pFin && !a.enviado {
				return // aún se está mandando
			}
			a.lec, a.t = nil, nil
			a.son.olvidar()
			a.pant = pInicio
			a.cargar()
		}
	}
}

func (a *app) pulsarInicio(b uint16) {
	u := a.unidadVista()
	switch b {
	case bArriba:
		if a.nivelSel > 0 {
			a.nivelSel--
		} else if a.unidad > 0 {
			a.unidad--
			a.nivelSel = max(0, len(a.unidadVista().Niveles)-1)
		}
	case bAbajo:
		if a.nivelSel < len(u.Niveles)-1 {
			a.nivelSel++
		} else if a.unidad < len(a.camino)-1 {
			a.unidad++
			a.nivelSel = 0
		}
	case bIzquierda:
		if a.unidad > 0 {
			a.unidad--
			a.nivelSel = nivelActual(a.camino, a.unidad)
		}
	case bDerecha:
		if a.unidad < len(a.camino)-1 {
			a.unidad++
			a.nivelSel = nivelActual(a.camino, a.unidad)
		}
	case bA, bStart:
		if a.nivelSel < len(u.Niveles) {
			a.empezarNivel(u, u.Niveles[a.nivelSel])
		}
	case bX:
		a.empezar(peticion{tipo: "practica"}, "Práctica")
	case bY:
		if !a.cargando {
			a.cargar()
			a.aviso("Actualizando…")
		}
	case bR, bL:
		a.pant = pPerfil
	case bB:
		a.terminar()
	case bSelect:
		a.luz.apagar()
	}
}

func (a *app) empezarNivel(u unidad, n nivel) {
	switch {
	case n.Estado == estBloqueado:
		a.aviso("Aún no: primero termina los de antes")
	case n.Tipo == "story":
		a.aviso("Las historias no se pueden hacer aquí: hazlas en el móvil")
	case n.Tipo == "chest":
		a.aviso("Los cofres se abren en el móvil o en la web")
	case n.Peticion.tipo == "":
		a.aviso("Este nivel no se puede hacer desde la consola")
	default:
		a.empezar(n.Peticion, u.Titulo+" · "+n.Nombre)
	}
}

func (a *app) pulsarPerfil(b uint16) {
	switch b {
	case bL, bR, bB:
		a.pant = pInicio
	case bA:
		a.irACuenta()
	case bX:
		a.confirmar("¿Cerrar la sesión de Duolingo en la consola? Tu cuenta no cambia: solo se borra de aquí.", func() {
			a.duo.cuenta.olvidar()
			a.perfil, a.perfilHecho, a.camino = perfil{}, false, nil
			a.irACuenta()
		})
	case bSelect:
		a.luz.apagar()
	}
}

// ---- Lecciones -----------------------------------------------------------------------------------

func (a *app) empezar(p peticion, nombre string) {
	if !hayRed() {
		a.aviso("Sin internet")
		return
	}
	if a.perfil.Vidas == 0 {
		a.aviso("No te quedan vidas: espera a que se recarguen o practica para ganarlas")
	}
	a.pedido++
	pedido := a.pedido
	a.pant, a.errLec = pCargando, ""
	go func() {
		lec, err := a.duo.Empezar(p, a.perfil)
		if lec != nil {
			lec.nombre = nombre
			var urls []string
			for _, e := range lec.ejs {
				urls = append(urls, e.Audio)
				for _, o := range e.Opciones {
					urls = append(urls, o.Audio)
				}
				for _, o := range e.Izquierda {
					urls = append(urls, o.Audio)
				}
			}
			a.son.precargar(urls)
		}
		a.hacer(func() {
			if pedido != a.pedido {
				return // ya no la quiere
			}
			if err != nil {
				a.errLec = textoError(err)
				if errors.Is(err, errSesion) {
					a.cuentaMal = true
				}
				return
			}
			a.lec, a.enviado = lec, false
			a.pant = pLeccion
			a.nuevoTurno()
		})
	}()
}

// acabar manda la lección a Duolingo (para que cuente) y enseña lo ganado.
func (a *app) acabar() {
	a.pant, a.enviado, a.fin = pFin, false, resultadoFin{}
	a.son.fanfarria()
	lec := a.lec
	go func() {
		r, err := a.duo.Terminar(lec)
		a.hacer(func() {
			a.enviado = true
			if err != nil {
				r.Error = textoError(err)
				logf("al mandar la lección: %v", err)
			}
			a.fin = r
		})
	}()
}
