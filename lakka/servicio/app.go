package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

type pantallaID int

const (
	pInicio pantallaID = iota
	pSala
	pJuegos
	pPartida
	pTeclado
	pMensaje
	pConfirmar
)

// Para qué se está eligiendo juego o escribiendo.
type flujo int

const (
	fNinguno flujo = iota
	fCrear
	fUnirse
	fSolo
	fCambiarJuego
	fNombre
)

// Botones de RetroPad (los mismos números que libretro).
const (
	bB      = 0
	bY      = 1
	bSelect = 2
	bStart  = 3
	bArriba = 4
	bAbajo  = 5
	bIzq    = 6
	bDer    = 7
	bA      = 8
	bX      = 9
)

type resultado struct {
	err      error
	duracion time.Duration
}

type opcion struct {
	texto, etiqueta, ayuda string
	hacer                  func()
}

type app struct {
	jug    *Jugador
	salas  []Sala
	raiz   string
	juegos []Juego

	activa *salaActiva
	sala   *Sala // la sala en la que estamos (copia)

	pant    pantallaID
	sel     int
	desp    int
	fl      flujo
	codigo  string
	elegido *Juego

	texto      string
	filaTecla  int
	colTecla   int
	minusculas bool

	mensaje        string
	volverA        pantallaID
	confirmarTexto string
	confirmar      func()

	jugando        bool
	salaJuego      *Sala
	inicioTramo    time.Time
	proximaCaptura time.Time
	capturando     bool
	ignorarHasta   time.Time

	f        *fuentes
	sup      *supervisor
	srv      *servidorPantalla
	botones  chan uint16
	avisos   chan struct{}
	fin      chan resultado
	capturas chan string
	cache    map[string]image.Image
	sucio    bool
	m        mando
}

func nuevaApp() *app {
	a := &app{
		jug:      cargarJugador(),
		salas:    cargarSalas(),
		f:        cargarFuentes(),
		botones:  make(chan uint16, 64),
		avisos:   make(chan struct{}, 1),
		fin:      make(chan resultado, 1),
		capturas: make(chan string, 1),
		cache:    map[string]image.Image{},
	}
	if d := os.Getenv("POKEMULADOR_JUEGOS"); d != "" {
		a.raiz = d
	} else {
		a.raiz = carpetaJuegos()
		escribirLeeme(a.raiz)
	}
	a.juegos = buscarJuegos(a.raiz)
	log.Printf("juegos en %s: %d", a.raiz, len(a.juegos))
	a.ignorarHasta = time.Now().Add(time.Second)
	return a
}

func (a *app) finJuego(err error, duracion time.Duration) { a.fin <- resultado{err, duracion} }

func (a *app) cerrarTodo() {
	if a.activa != nil {
		a.activa.detener()
	}
}

func (a *app) bucle() {
	a.dibujar()
	tic := time.NewTicker(50 * time.Millisecond)
	refresco := time.NewTicker(3 * time.Second)
	for {
		select {
		case b := <-a.botones:
			for _, x := range a.m.cambiar(b, time.Now()) {
				a.pulsar(x)
			}
		case <-tic.C:
			for _, x := range a.m.repetir(time.Now()) {
				a.pulsar(x)
			}
			a.progresoJuego()
		case <-refresco.C:
			if a.pant == pSala || a.pant == pInicio {
				a.sucio = true
			}
		case <-a.avisos:
			a.sucio = true
		case c := <-a.capturas:
			a.capturando = false
			if a.salaJuego != nil {
				ahora := time.Now()
				ms := int64(0)
				if a.jugando {
					ms = ahora.Sub(a.inicioTramo).Milliseconds()
					a.inicioTramo = ahora
				}
				sumarProgreso(a.salaJuego, a.jug, ms, c)
			}
		case r := <-a.fin:
			a.juegoTerminado(r)
		}
		if a.sucio && !a.jugando {
			a.dibujar()
		}
	}
}

// ---------- Mando ----------

type mando struct {
	estado   uint16
	pulsado  [16]time.Time
	repetido [16]time.Time
}

func (m *mando) cambiar(nuevo uint16, t time.Time) []int {
	var res []int
	for b := 0; b < 16; b++ {
		bit := uint16(1) << b
		if nuevo&bit != 0 && m.estado&bit == 0 {
			res = append(res, b)
			m.pulsado[b], m.repetido[b] = t, t
		}
	}
	m.estado = nuevo
	return res
}

func (m *mando) repetir(t time.Time) []int {
	var res []int
	for _, b := range []int{bArriba, bAbajo, bIzq, bDer} {
		if m.estado&(1<<b) != 0 && t.Sub(m.pulsado[b]) > 400*time.Millisecond && t.Sub(m.repetido[b]) > 110*time.Millisecond {
			res = append(res, b)
			m.repetido[b] = t
		}
	}
	return res
}

// ---------- Navegación ----------

func (a *app) irA(p pantallaID) {
	a.pant, a.sel, a.desp = p, 0, 0
	a.sucio = true
}

func (a *app) mostrarMensaje(texto string, despues pantallaID) {
	a.mensaje, a.volverA = texto, despues
	a.irA(pMensaje)
}

func (a *app) preguntar(texto string, si func()) {
	a.confirmarTexto, a.confirmar, a.volverA = texto, si, a.pant
	a.irA(pConfirmar)
	a.sel = 1 // "No" por defecto
}

func (a *app) pulsar(b int) {
	if time.Now().Before(a.ignorarHasta) || a.jugando {
		return
	}
	a.sucio = true
	aceptar := b == bA || b == bStart
	volver := b == bB
	switch a.pant {
	case pInicio:
		ops := a.opcionesInicio()
		a.moverLista(b, len(ops))
		if aceptar && a.sel < len(ops) {
			ops[a.sel].hacer()
		}
	case pSala:
		n := len(a.botonesSala())
		switch b {
		case bIzq, bArriba:
			a.sel = (a.sel + n - 1) % n
		case bDer, bAbajo:
			a.sel = (a.sel + 1) % n
		}
		if aceptar {
			a.botonesSala()[a.sel].hacer()
		}
		if volver {
			a.irA(pInicio)
		}
	case pJuegos:
		a.moverLista(b, len(a.juegos))
		if aceptar && a.sel < len(a.juegos) {
			j := a.juegos[a.sel]
			a.juegoElegido(j)
		}
		if volver {
			if a.fl == fCambiarJuego {
				a.irA(pSala)
			} else {
				a.irA(pInicio)
			}
		}
	case pPartida:
		a.moverLista(b, 2)
		if aceptar && a.elegido != nil {
			a.crearOUnirse(*a.elegido, a.sel == 0)
		}
		if volver {
			a.irA(pJuegos)
		}
	case pTeclado:
		a.teclear(b)
	case pMensaje:
		if aceptar || volver {
			a.irA(a.volverA)
		}
	case pConfirmar:
		switch b {
		case bIzq, bDer, bArriba, bAbajo:
			a.sel = 1 - a.sel
		}
		if volver {
			a.irA(a.volverA)
		} else if aceptar {
			si, hacer := a.sel == 0, a.confirmar
			a.irA(a.volverA)
			if si && hacer != nil {
				hacer()
			}
		}
	}
}

func (a *app) moverLista(b, n int) {
	if n == 0 {
		return
	}
	switch b {
	case bArriba:
		a.sel = (a.sel + n - 1) % n
	case bAbajo:
		a.sel = (a.sel + 1) % n
	case bIzq:
		a.sel = max(a.sel-6, 0)
	case bDer:
		a.sel = min(a.sel+6, n-1)
	}
}

// ---------- Inicio ----------

func (a *app) opcionesInicio() []opcion {
	ops := []opcion{
		{"Crear sala", "", "Elige un juego y te damos un código para que tu amigo entre desde el PC, el móvil o su USB.", a.empezar(fCrear)},
		{"Unirse con un código", "", "Escribe el código de 6 letras o números que te ha pasado tu amigo.", a.empezar(fUnirse)},
	}
	for _, s := range a.salas {
		s := s
		etiqueta := sistemas[s.Sistema].nombre
		if a.sala != nil && a.sala.Codigo == s.Codigo {
			etiqueta = "Dentro"
		}
		partida := "Tu partida de siempre."
		if s.PartidaPropia {
			partida = "Partida propia de la sala."
		}
		ops = append(ops, opcion{"Sala " + codigoVisible(s.Codigo) + " · " + s.JuegoTitulo, etiqueta,
			partida + " Entra cuando quieras: tus amigos verán cómo vas aunque no estéis a la vez.",
			func() { a.entrar(s) }})
	}
	ops = append(ops,
		opcion{"Jugar sin sala", "", "Juega solo, con tu partida de siempre.", a.empezar(fSolo)},
		opcion{"Mi nombre: " + a.jug.Nombre, "", "Así te verán tus amigos en las salas.", a.empezar(fNombre)},
	)
	if modoPort {
		ops = append(ops,
			opcion{"Menú de RetroArch (mandos, ajustes)", "", "Abre el menú de RetroArch. Para volver aquí, mantén Start o pulsa la combinación de tu consola.", func() { comandoRetroArch("MENU_TOGGLE") }},
			opcion{"Salir", "", "Cierra Pokemulador y vuelve al menú de la consola. Si estás en una sala, sales de ella.", a.salir})
	} else {
		ops = append(ops, opcion{"Menú de Lakka (Wi-Fi, mandos)", "", "Abre el menú de Lakka. Para conectarte a Internet: Ajustes › Wi-Fi. Para volver aquí, pulsa F1 o mantén Start.", func() { comandoRetroArch("MENU_TOGGLE") }})
	}
	if ordenApagar != "" {
		ops = append(ops, opcion{"Apagar", "", "Apaga el aparato. Si es un ordenador con la memoria USB, después ya puedes quitarla.", func() {
			a.preguntar("¿Apagar?", func() {
				a.cerrarTodo()
				a.sup.cerrar()
				_ = exec.Command("sh", "-c", ordenApagar).Start()
			})
		}})
	}
	return ops
}

// salir (modo port): cierra la sala y RetroArch y termina, para volver a EmulationStation.
func (a *app) salir() {
	log.Printf("salir")
	a.dibujarAviso("Cerrando…")
	a.cerrarTodo()
	a.sup.cerrar()
	os.Exit(0)
}

func (a *app) empezar(f flujo) func() {
	return func() {
		a.fl = f
		switch f {
		case fUnirse:
			a.texto, a.filaTecla, a.colTecla, a.minusculas = "", 0, 0, false
			a.irA(pTeclado)
		case fNombre:
			a.texto, a.filaTecla, a.colTecla, a.minusculas = a.jug.Nombre, 0, 0, false
			a.irA(pTeclado)
		default:
			a.abrirJuegos()
		}
	}
}

func (a *app) abrirJuegos() {
	a.juegos = buscarJuegos(a.raiz)
	a.irA(pJuegos)
}

func (a *app) juegoElegido(j Juego) {
	switch a.fl {
	case fSolo:
		a.lanzar(j, nil)
	case fCambiarJuego:
		if a.sala != nil {
			a.sala.Juego, a.sala.JuegoTitulo, a.sala.Sistema = j.Relativa, j.Titulo, j.Sistema
			for i := range a.salas {
				if a.salas[i].Codigo == a.sala.Codigo {
					a.salas[i] = *a.sala
				}
			}
			guardarSalas(a.salas)
			prepararProgreso(a.sala, a.jug)
		}
		a.irA(pSala)
	default:
		jj := j
		a.elegido = &jj
		a.irA(pPartida)
	}
}

func (a *app) crearOUnirse(j Juego, propia bool) {
	codigo := a.codigo
	if a.fl == fCrear {
		codigo = aleatorio(6)
	}
	s := Sala{
		Codigo: strings.ToLower(codigo), Nombre: "Sala " + codigoVisible(codigo),
		Juego: j.Relativa, JuegoTitulo: j.Titulo, Sistema: j.Sistema,
		PartidaPropia: propia, Creada: ahoraMs(),
	}
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
		a.irA(pSala)
		return
	}
	a.salirSala()
	copia := s
	a.sala = &copia
	a.activa = entrarEnSala(a.sala, a.jug, a.avisos)
	a.irA(pSala)
}

func (a *app) salirSala() {
	if a.activa != nil {
		a.activa.detener()
	}
	a.activa, a.sala = nil, nil
}

// ---------- Sala ----------

func (a *app) botonesSala() []opcion {
	return []opcion{
		{"Jugar", "", "", a.jugarSala},
		{"Cambiar juego", "", "", func() { a.fl = fCambiarJuego; a.abrirJuegos() }},
		{"Salir de la sala", "", "", func() { a.salirSala(); a.irA(pInicio) }},
		{"Borrar sala", "", "", func() {
			if a.sala == nil {
				return
			}
			codigo := a.sala.Codigo
			a.preguntar("¿Borrar la sala "+codigoVisible(codigo)+"? Tu partida no se borra.", func() {
				a.salirSala()
				var lista []Sala
				for _, x := range a.salas {
					if x.Codigo != codigo {
						lista = append(lista, x)
					}
				}
				a.salas = lista
				guardarSalas(a.salas)
				_ = os.RemoveAll(carpetaSala(codigo))
				a.irA(pInicio)
			})
		}},
	}
}

func (a *app) jugarSala() {
	if a.sala == nil {
		return
	}
	a.juegos = buscarJuegos(a.raiz)
	for _, j := range a.juegos {
		if j.Relativa == a.sala.Juego {
			a.lanzar(j, a.sala)
			return
		}
	}
	a.fl = fCambiarJuego
	a.mostrarMensaje("No encuentro «"+a.sala.JuegoTitulo+"» en la memoria USB. Elige el juego otra vez.", pJuegos)
}

func (a *app) lanzar(j Juego, s *Sala) {
	var modo *bool
	if s != nil && a.activa != nil {
		if e := a.activa.Estado(); e.RolDecidido {
			m := e.Anfitrion
			modo = &m
		}
	}
	o, err := prepararJuego(j, s, modo, a.jug)
	if err != nil {
		a.mostrarMensaje("No se pudo abrir el juego: "+err.Error(), a.pant)
		return
	}
	log.Printf("jugando a %s (sala %v, modo %v)", j.Relativa, s != nil, modo)
	a.dibujarAviso("Abriendo " + j.Titulo + "…")
	ahora := time.Now()
	a.jugando, a.salaJuego = true, s
	a.inicioTramo, a.proximaCaptura = ahora, ahora.Add(40*time.Second)
	if s != nil {
		sumarProgreso(s, a.jug, 0, "")
	}
	a.sup.jugar(o)
}

func (a *app) progresoJuego() {
	if !a.jugando || a.salaJuego == nil || a.capturando || time.Now().Before(a.proximaCaptura) {
		return
	}
	a.capturando = true
	a.proximaCaptura = time.Now().Add(2 * time.Minute)
	go func() { a.capturas <- capturar() }()
}

func (a *app) juegoTerminado(r resultado) {
	ahora := time.Now()
	if a.jugando && a.salaJuego != nil {
		sumarProgreso(a.salaJuego, a.jug, ahora.Sub(a.inicioTramo).Milliseconds(), "")
	}
	a.jugando = false
	a.ignorarHasta = ahora.Add(800 * time.Millisecond)
	a.m = mando{}
	a.juegos = buscarJuegos(a.raiz)
	if a.sala != nil {
		a.irA(pSala)
	} else {
		a.irA(pInicio)
	}
	if r.err != nil && r.duracion < 5*time.Second {
		a.mostrarMensaje("El juego se cerró nada más abrirse. Puede que el archivo esté dañado o que este ordenador no pueda con él.", a.pant)
	}
}

// ---------- Teclado ----------

func (a *app) filasTeclado() [][]string {
	letras := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	if a.minusculas {
		letras = strings.ToLower(letras)
	}
	todo := strings.Split(letras+"0123456789", "")
	filas := [][]string{todo[0:10], todo[10:20], todo[20:30], todo[30:36]}
	if a.fl == fNombre {
		return append(filas, []string{"Borrar", "Espacio", "Aa", "Aceptar"})
	}
	return append(filas, []string{"Borrar", "Aceptar"})
}

func (a *app) teclear(b int) {
	filas := a.filasTeclado()
	switch b {
	case bArriba:
		a.filaTecla = (a.filaTecla + len(filas) - 1) % len(filas)
	case bAbajo:
		a.filaTecla = (a.filaTecla + 1) % len(filas)
	case bIzq:
		a.colTecla--
	case bDer:
		a.colTecla++
	}
	fila := filas[a.filaTecla]
	if a.colTecla < 0 {
		a.colTecla = len(fila) - 1
	}
	if a.colTecla >= len(fila) {
		if b == bDer {
			a.colTecla = 0
		} else {
			a.colTecla = len(fila) - 1
		}
	}
	limite := 6
	if a.fl == fNombre {
		limite = 12
	}
	switch b {
	case bA:
		t := fila[a.colTecla]
		switch t {
		case "Borrar":
			a.borrarLetra()
		case "Espacio":
			if len([]rune(a.texto)) < limite {
				a.texto += " "
			}
		case "Aa":
			a.minusculas = !a.minusculas
		case "Aceptar":
			a.aceptarTexto()
		default:
			if len([]rune(a.texto)) < limite {
				a.texto += t
			}
		}
	case bStart:
		a.aceptarTexto()
	case bB:
		if a.texto != "" {
			a.borrarLetra()
		} else {
			a.irA(pInicio)
		}
	case bY:
		a.borrarLetra()
	}
}

func (a *app) borrarLetra() {
	r := []rune(a.texto)
	if len(r) > 0 {
		a.texto = string(r[:len(r)-1])
	}
}

func (a *app) aceptarTexto() {
	switch a.fl {
	case fUnirse:
		c := strings.ToLower(strings.TrimSpace(a.texto))
		if !codigoValido(c) {
			a.mostrarMensaje("El código tiene 6 letras o números, como lo ves en la sala de tu amigo.", pTeclado)
			return
		}
		a.codigo = c
		a.abrirJuegos()
	case fNombre:
		n := strings.Join(strings.Fields(a.texto), " ")
		if n == "" {
			return
		}
		a.jug.Nombre = n
		guardarJSON(archivoJugador(), a.jug)
		if a.sala != nil {
			prepararProgreso(a.sala, a.jug)
		}
		a.irA(pInicio)
	}
}

// ---------- Utilidades ----------

func hayInternet() bool {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		c := strings.Fields(s.Text())
		if len(c) > 2 && c[1] == "00000000" && c[0] != "lo" && !strings.HasPrefix(c[0], "tailscale") {
			return true
		}
	}
	return false
}

func duracionTexto(ms int64) string {
	minutos := ms / 60000
	if minutos < 60 {
		return fmt.Sprintf("%d min jugado", minutos)
	}
	return fmt.Sprintf("%d h %d min jugado", minutos/60, minutos%60)
}

func haceTexto(ms int64) string {
	if ms <= 0 {
		return "Aún no ha jugado"
	}
	d := time.Since(time.UnixMilli(ms))
	switch {
	case d < time.Minute:
		return "Jugando ahora"
	case d < time.Hour:
		return fmt.Sprintf("Hace %d min", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("Hace %d h", int(d.Hours()))
	default:
		return fmt.Sprintf("Hace %d días", int(d.Hours()/24))
	}
}

func (a *app) imagenCaptura(p *Progreso) image.Image {
	if p.Captura == "" {
		return nil
	}
	clave := p.ID + fmt.Sprint(len(p.Captura)) + p.Captura[:min(len(p.Captura), 64)] + p.Captura[max(0, len(p.Captura)-64):]
	if img, ok := a.cache[clave]; ok {
		return img
	}
	img := decodificarCaptura(p.Captura)
	if len(a.cache) > 32 {
		a.cache = map[string]image.Image{}
	}
	a.cache[clave] = img
	return img
}

var _ color.Color = colFondo
