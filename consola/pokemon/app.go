package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Pantallas.
type pantalla int

const (
	pJuegos    pantalla = iota // tus juegos y sus salas
	pSalas                     // crear, unirse o entrar en una sala del juego elegido
	pPartida                   // con qué partida se juega en la sala nueva
	pTeclado                   // el código de una sala o tu nombre
	pSala                      // dentro de la sala
	pConfirmar                 // ¿seguro?
)

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	ra     *retroarch // la pantalla (nil mientras se juega)
	juego  *retroarch // el juego abierto
	luz    *luz
	tareas chan func()
	salir  chan struct{}
	avisos chan struct{} // la sala activa ha cambiado

	jug       *Jugador
	juegos    []Juego
	salas     []Sala
	caramelos map[string]bool // juego (ruta relativa) → Caramelos Raros infinitos

	pant       pantalla
	selJuego   int
	selOpcion  int
	selPartida int

	tec     teclado
	tecPara string // "codigo" o "nombre"

	// Sala nueva (crear o unirse) y la sala en la que estamos.
	crear  bool
	codigo string
	sala   *Sala
	activa *salaActiva

	confirmTexto  string
	confirmSi     func()
	confirmVolver pantalla

	// El juego que se está jugando.
	orden          orden
	inicioJuego    time.Time
	proximaCaptura time.Time
	capturando     bool
	ignorarHasta   time.Time

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios   bool
	pulsados  uint16
	repetirEn time.Time
	tarjetas  map[string]*portada
}

func nuevaApp(f *fuentes) *app {
	a := &app{f: f, l: nuevoLienzo(f), luz: nuevaLuz(), tareas: make(chan func(), 64), salir: make(chan struct{}),
		avisos: make(chan struct{}, 8), cambios: true, tarjetas: map[string]*portada{}}
	a.jug = cargarJugador()
	a.juegos = buscarJuegos()
	a.salas = cargarSalas()
	a.caramelos = map[string]bool{}
	if b, err := os.ReadFile(filepath.Join(carpetaDatos, "caramelos.json")); err == nil {
		_ = json.Unmarshal(b, &a.caramelos)
	}
	a.cargaHasta = time.Now().Add(900 * time.Millisecond)
	if !a.jug.Elegido { // la primera vez, cómo te llamas en las salas
		a.abrirTeclado("nombre")
	}
	return a
}

func (a *app) hacer(fn func()) {
	select {
	case a.tareas <- fn:
	case <-a.salir:
	}
}

func (a *app) aviso(s string) {
	a.avisoTexto, a.avisoHasta, a.cambios = s, time.Now().Add(4*time.Second), true
}

func (a *app) terminar() {
	select {
	case <-a.salir:
	default:
		close(a.salir)
	}
}

func (a *app) cerrarTodo() {
	if a.juego != nil {
		a.juego.cerrar()
	}
	a.salirSala()
	a.ra.cerrar()
}

func (a *app) juegoElegido() *Juego {
	if a.selJuego >= 0 && a.selJuego < len(a.juegos) {
		return &a.juegos[a.selJuego]
	}
	return nil
}

// salasDe: las salas guardadas de un juego.
func (a *app) salasDe(j *Juego) []Sala {
	var r []Sala
	for _, s := range a.salas {
		if j != nil && s.Juego == j.Relativa {
			r = append(r, s)
		}
	}
	return r
}

// ---- Bucle -------------------------------------------------------------------------------------

func (a *app) bucle(botones <-chan uint16) {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	segundo := time.NewTicker(time.Second)
	defer segundo.Stop()
	for {
		var raHecho, juegoHecho <-chan struct{}
		if a.ra != nil {
			raHecho = a.ra.terminado
		}
		if a.juego != nil {
			juegoHecho = a.juego.terminado
		}
		select {
		case <-a.salir:
			return
		case <-raHecho:
			logf("la pantalla se ha cerrado (%v): salgo", a.ra.err)
			a.ra = nil
			a.terminar()
			return
		case <-juegoHecho:
			a.juegoTerminado()
		case fn := <-a.tareas:
			fn()
			a.cambios = true
		case <-a.avisos:
			if a.activa != nil {
				if s := a.activa.tomarAviso(); s != "" {
					a.aviso(s)
				}
			}
			a.cambios = true
		case m := <-botones:
			if a.juego == nil && time.Now().After(a.ignorarHasta) {
				a.botones(m)
			} else {
				a.pulsados = m
			}
		case <-reloj.C:
			a.repetir()
			if !a.avisoHasta.IsZero() && time.Now().After(a.avisoHasta) {
				a.avisoHasta, a.avisoTexto, a.cambios = time.Time{}, "", true
			}
			if !a.cargaHasta.IsZero() && time.Now().After(a.cargaHasta) {
				a.cargaHasta, a.cambios = time.Time{}, true
			}
		case <-segundo.C:
			a.progresoJuego()
			if a.pant == pSala || a.pant == pJuegos {
				a.cambios = true // «hace 2 min», el reloj
			}
		}
		if a.cambios && a.juego == nil && !a.luz.apagada {
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

func (a *app) repetir() {
	if a.juego != nil || a.pulsados&flechas == 0 || time.Now().Before(a.repetirEn) || a.luz.apagada {
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
	if a.luz.apagada {
		a.luz.encender()
		return
	}
	if b == bSelect && a.pant != pTeclado {
		a.luz.apagar()
		return
	}
	switch a.pant {
	case pJuegos:
		a.pulsarJuegos(b)
	case pSalas:
		a.pulsarSalas(b)
	case pPartida:
		a.pulsarPartida(b)
	case pTeclado:
		a.pulsarTeclado(b)
	case pSala:
		a.pulsarSala(b)
	case pConfirmar:
		switch b {
		case bA:
			a.pant = a.confirmVolver
			if a.confirmSi != nil {
				a.confirmSi()
			}
		case bB:
			a.pant = a.confirmVolver
		}
	}
}

func (a *app) preguntar(texto string, si func()) {
	a.confirmTexto, a.confirmSi, a.confirmVolver = texto, si, a.pant
	a.pant = pConfirmar
}

// ---- Juegos ------------------------------------------------------------------------------------

func (a *app) pulsarJuegos(b uint16) {
	n := len(a.juegos)
	switch b {
	case bArriba:
		a.selJuego = max(a.selJuego-1, 0)
	case bAbajo:
		a.selJuego = min(a.selJuego+1, max(n-1, 0))
	case bL:
		a.selJuego = max(a.selJuego-3, 0)
	case bR:
		a.selJuego = min(a.selJuego+3, max(n-1, 0))
	case bA:
		if j := a.juegoElegido(); j != nil {
			a.jugar(*j, nil)
		}
	case bY:
		if a.juegoElegido() != nil {
			a.pant, a.selOpcion = pSalas, 0
		}
	case bX:
		if j := a.juegoElegido(); j != nil && esRojoFuego(*j) {
			a.caramelos[j.Relativa] = !a.caramelos[j.Relativa]
			guardarJSON(filepath.Join(carpetaDatos, "caramelos.json"), a.caramelos)
			if a.caramelos[j.Relativa] {
				a.aviso("Caramelos Raros infinitos: activados")
			} else {
				a.aviso("Caramelos Raros infinitos: desactivados")
			}
		}
	case bStart:
		a.abrirTeclado("nombre")
	case bB:
		a.terminar()
	}
}

// ---- Salas del juego ----------------------------------------------------------------------------

// opcionesSalas: crear, unirse y las salas guardadas de este juego.
func (a *app) opcionesSalas() int { return 2 + len(a.salasDe(a.juegoElegido())) }

func (a *app) pulsarSalas(b uint16) {
	n := a.opcionesSalas()
	switch b {
	case bArriba:
		a.selOpcion = max(a.selOpcion-1, 0)
	case bAbajo:
		a.selOpcion = min(a.selOpcion+1, n-1)
	case bB:
		a.pant = pJuegos
	case bA:
		switch a.selOpcion {
		case 0:
			a.crear, a.codigo, a.selPartida, a.pant = true, "", 0, pPartida
		case 1:
			a.crear = false
			a.abrirTeclado("codigo")
		default:
			ss := a.salasDe(a.juegoElegido())
			if i := a.selOpcion - 2; i < len(ss) {
				a.entrar(ss[i])
			}
		}
	case bX: // borrar la sala elegida
		ss := a.salasDe(a.juegoElegido())
		if i := a.selOpcion - 2; i >= 0 && i < len(ss) {
			s := ss[i]
			a.preguntar("¿Borrar la sala "+codigoVisible(s.Codigo)+"? Tu partida no se borra.", func() { a.borrarSala(s.Codigo) })
		}
	}
}

func (a *app) borrarSala(codigo string) {
	if a.sala != nil && a.sala.Codigo == codigo {
		a.salirSala()
	}
	var lista []Sala
	for _, x := range a.salas {
		if x.Codigo != codigo {
			lista = append(lista, x)
		}
	}
	a.salas = lista
	guardarSalas(a.salas)
	_ = os.RemoveAll(carpetaSala(codigo))
	a.selOpcion = 0
	a.aviso("Sala borrada")
}

func (a *app) pulsarPartida(b uint16) {
	switch b {
	case bArriba, bAbajo:
		a.selPartida = 1 - a.selPartida
	case bB:
		a.pant = pSalas
	case bA:
		if j := a.juegoElegido(); j != nil {
			a.crearOUnirse(*j, a.selPartida == 1)
		}
	}
}

func (a *app) crearOUnirse(j Juego, propia bool) {
	codigo := a.codigo
	if a.crear || codigo == "" {
		codigo = aleatorio(6)
	}
	s := Sala{Codigo: strings.ToLower(codigo), Nombre: "Sala " + codigoVisible(codigo), Juego: j.Relativa,
		JuegoTitulo: j.Titulo, Sistema: j.Sistema, PartidaPropia: propia, Creada: ahoraMs()}
	lista := []Sala{s}
	for _, x := range a.salas {
		if x.Codigo != s.Codigo {
			lista = append(lista, x)
		}
	}
	a.salas = lista
	guardarSalas(a.salas)
	a.entrar(s)
}

func (a *app) entrar(s Sala) {
	if a.activa != nil && a.sala != nil && a.sala.Codigo == s.Codigo {
		a.pant = pSala
		return
	}
	a.salirSala()
	copia := s
	a.sala = &copia
	a.activa = entrarEnSala(a.sala, a.jug, a.avisos)
	a.pant = pSala
}

func (a *app) salirSala() {
	if a.activa != nil {
		a.activa.detener()
	}
	a.activa, a.sala = nil, nil
}

// ---- Teclado ------------------------------------------------------------------------------------

func (a *app) abrirTeclado(para string) {
	a.tecPara = para
	if para == "nombre" {
		inicial := a.jug.Nombre
		if !a.jug.Elegido {
			inicial = ""
		}
		a.tec.abrir(inicial, 16)
	} else {
		a.tec.abrir("", 6)
	}
	a.pant = pTeclado
}

func (a *app) pulsarTeclado(b uint16) {
	switch a.tec.pulsar(b) {
	case tecCancelar:
		a.cerrarTeclado()
		return
	case tecEnviar:
		a.aceptarTexto()
		return
	}
	if b == bB && len(a.tec.borrador) == 0 {
		a.cerrarTeclado()
	}
}

func (a *app) cerrarTeclado() {
	if a.tecPara == "nombre" {
		if !a.jug.Elegido { // la primera vez se queda el de siempre
			a.jug.Elegido = true
			a.jug.guardar()
		}
		a.pant = pJuegos
		return
	}
	a.pant = pSalas
}

func (a *app) aceptarTexto() {
	switch a.tecPara {
	case "codigo":
		c := strings.ToLower(strings.TrimSpace(a.tec.texto()))
		if !codigoValido(c) {
			a.aviso("El código tiene 6 letras o números, como lo ves en la sala de tu amigo")
			return
		}
		a.codigo, a.crear, a.selPartida = c, false, 0
		a.pant = pPartida
	case "nombre":
		n := strings.Join(strings.Fields(a.tec.texto()), " ")
		if n == "" {
			n = nombrePorDefecto
		}
		a.jug.Nombre, a.jug.Elegido = n, true
		a.jug.guardar()
		if a.sala != nil {
			prepararProgreso(a.sala, a.jug)
		}
		a.pant = pJuegos
		a.aviso("En las salas te llamarás «" + n + "»")
	}
}

// ---- Sala ---------------------------------------------------------------------------------------

func (a *app) juegoDeSala() *Juego {
	if a.sala == nil {
		return nil
	}
	for i := range a.juegos {
		if a.juegos[i].Relativa == a.sala.Juego {
			return &a.juegos[i]
		}
	}
	return nil
}

func (a *app) pulsarSala(b uint16) {
	switch b {
	case bA:
		j := a.juegoDeSala()
		if j == nil {
			a.aviso("No encuentro «" + a.sala.JuegoTitulo + "» en la consola")
			return
		}
		e := a.activa.Estado()
		if !e.RolDecidido {
			if e.Login != "" {
				a.aviso("Primero entra en Tailscale con el QR")
			} else {
				a.aviso("Espera a que la sala se conecte")
			}
			return
		}
		a.jugar(*j, a.sala)
	case bX:
		if a.sala != nil {
			codigo := a.sala.Codigo
			a.preguntar("¿Borrar la sala "+codigoVisible(codigo)+"? Tu partida no se borra.", func() {
				a.borrarSala(codigo)
				a.pant = pJuegos
			})
		}
	case bB:
		a.salirSala()
		a.pant = pJuegos
	}
}

// ---- Jugar --------------------------------------------------------------------------------------

func (a *app) jugar(j Juego, s *Sala) {
	var modo *bool
	if s != nil && a.activa != nil {
		if e := a.activa.Estado(); e.RolDecidido {
			m := e.Anfitrion
			modo = &m
		}
	}
	o, err := prepararJuego(j, s, modo, a.jug, a.caramelos[j.Relativa])
	if err != nil {
		a.aviso("No se pudo abrir el juego: " + err.Error())
		return
	}
	papel := "sin sala"
	if modo != nil {
		papel = map[bool]string{true: "sala abierta por nosotros", false: "sala de un amigo"}[*modo]
	}
	logf("jugando a %s (%s, GBA-PK %v, caramelos %v)", j.Relativa, papel, o.conGbaPk, o.conCaramel)
	// El logo con el nombre del juego mientras se cambia de una pantalla a otra.
	a.dibujarCarga("Abriendo " + j.Titulo + "…")
	a.srv.enviar(a.l.marco())
	time.Sleep(150 * time.Millisecond)
	a.ra.cerrar()
	a.ra = nil
	pintarFB(a.l.img)
	g, err := abrirRetroArch(o.args, o.env, "juego.txt", true)
	if err != nil {
		a.aviso("No se pudo abrir RetroArch: " + err.Error())
		a.volverPantalla()
		return
	}
	a.juego, a.orden = g, o
	a.inicioJuego = time.Now()
	a.proximaCaptura = time.Now().Add(40 * time.Second)
	if s != nil {
		sumarProgreso(s, a.jug, 0, "", true)
	}
}

// progresoJuego: cada poco, una captura para que los amigos vean por dónde vas.
func (a *app) progresoJuego() {
	if a.juego == nil || a.orden.sala == nil || a.capturando || time.Now().Before(a.proximaCaptura) {
		return
	}
	a.capturando = true
	a.proximaCaptura = time.Now().Add(2 * time.Minute)
	s, jug := a.orden.sala, a.jug
	go func() {
		c := capturar()
		a.hacer(func() {
			a.capturando = false
			if c != "" {
				sumarProgreso(s, jug, 0, c, true)
			}
		})
	}()
}

func (a *app) juegoTerminado() {
	g, o := a.juego, a.orden
	a.juego = nil
	duracion := time.Since(a.inicioJuego)
	if o.sala != nil {
		sumarProgreso(o.sala, a.jug, duracion.Milliseconds(), "", true)
	}
	if o.devolverA != "" {
		devolverPartida(o.juego, o.guardados, o.devolverA)
	}
	a.juegos = buscarJuegos()
	a.volverPantalla()
	if g != nil && g.err != nil && duracion < 5*time.Second {
		a.aviso("El juego se cerró nada más abrirse")
	}
}

// volverPantalla abre otra vez la pantalla de la app después de un juego.
func (a *app) volverPantalla() {
	a.dibujarCarga("")
	pintarFB(a.l.img)
	ra, err := abrirPantallaRA()
	if err != nil {
		logf("no se pudo abrir la pantalla otra vez: %v", err)
		a.terminar()
		return
	}
	a.ra = ra
	a.ignorarHasta = time.Now().Add(800 * time.Millisecond)
	a.pulsados = 0
	a.cambios = true
}
