package main

import (
	"context"
	"errors"
	"strings"
	"time"
)

type pantalla int

const (
	pCuenta    pantalla = iota // pasar el token
	pMenu                      // jugar con un amigo, contra la máquina, problemas, partidas empezadas
	pRival                     // a quién retas
	pEscribir                  // escribir su nombre
	pOpciones                  // ritmo, color…
	pEsperando                 // el reto ya ha salido
	pEmpezadas                 // tus partidas en curso
	pPartida
	pProblema
)

// un reto que te llega
type retoEntrante struct {
	ID, De, Ritmo string
	Elo           int
	Puntua        bool
}

type app struct {
	f      *fuentes
	l      *lienzo
	srv    *servidorPantalla
	luz    *luz
	li     *lichess
	son    *sonido
	web    *paginaSesion
	tareas chan func()
	salir  chan struct{}

	pant        pantalla
	yo          miCuenta
	hayYo       bool
	errCuenta   string
	sesionTexto string

	// menús
	menuSel   int
	lista     []string // pRival: los nombres
	listaSel  int
	empezadas []partidaEnCurso
	empSel    int
	cargando  bool
	tec       teclado

	// pOpciones
	contraMaquina bool
	rival         string
	opSel         int
	ritmoSel      int
	colorSel      int // 0 al azar, 1 blancas, 2 negras
	puntua        bool
	nivel         int

	// el reto que has mandado
	esperandoA   string
	retoID       string
	cancelarReto context.CancelFunc

	retos []retoEntrante // los que te llegan

	pa *partidaViva
	pz *problemaVivo

	menuPartida bool
	mpSel       int

	confirmTexto string
	confirmSi    func()

	avisoTexto string
	avisoHasta time.Time
	cargaHasta time.Time

	cambios      bool
	pulsados     uint16
	repetirEn    time.Time
	segundoVisto int64
}

func nuevaApp(f *fuentes, li *lichess) *app {
	a := &app{f: f, l: nuevoLienzo(f), li: li, son: &sonido{}, tareas: make(chan func(), 128),
		salir: make(chan struct{}), cambios: true, luz: nuevaLuz(), nivel: 3, ritmoSel: 1}
	a.cargaHasta = time.Now().Add(700 * time.Millisecond)
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

func (a *app) confirmar(texto string, si func()) { a.confirmTexto, a.confirmSi = texto, si }

func textoError(err error) string {
	switch {
	case errors.Is(err, errSesion):
		return "Tu token de Lichess ya no vale: ve a la cuenta (SELECT en el menú) y pon uno nuevo."
	case !hayRed():
		return "Sin internet."
	}
	return err.Error()
}

func (a *app) arrancar() {
	if !a.li.cuenta.tiene() {
		a.irACuenta()
		return
	}
	a.pant = pMenu
	a.cargarCuenta()
	go a.escucharAvisos()
}

func (a *app) cargarCuenta() {
	go func() {
		yo, err := a.li.Cuenta()
		ps, err2 := a.li.Jugando()
		a.hacer(func() {
			if err != nil {
				a.errCuenta = textoError(err)
				return
			}
			a.yo, a.hayYo, a.errCuenta = yo, true, ""
			if err2 == nil {
				a.empezadas = ps
			}
		})
	}()
}

// escucharAvisos: el stream de tus avisos de Lichess (retos y partidas que empiezan), mientras la
// app esté abierta. Si se corta, se vuelve a abrir.
func (a *app) escucharAvisos() {
	espera := 2 * time.Second
	for {
		select {
		case <-a.salir:
			return
		default:
		}
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-a.salir
			cancel()
		}()
		inicio := time.Now()
		err := a.li.stream(ctx, "GET", "/api/stream/event", nil, func(m map[string]any) {
			a.hacer(func() { a.alAviso(m) })
		})
		cancel()
		if errors.Is(err, errSesion) {
			a.hacer(func() { a.errCuenta = textoError(err) })
			return
		}
		if time.Since(inicio) > time.Minute {
			espera = 2 * time.Second
		}
		select {
		case <-a.salir:
			return
		case <-time.After(espera):
		}
		espera = min(espera*2, 60*time.Second)
	}
}

func ritmoDe(c map[string]any) string {
	tc := obj(c, "timeControl")
	switch texto(tc, "type") {
	case "clock":
		return texto(tc, "show")
	case "correspondence":
		return itoa(entero(tc, "daysPerTurn")) + " día(s) por jugada"
	}
	return "sin reloj"
}

func (a *app) alAviso(m map[string]any) {
	switch texto(m, "type") {
	case "challenge":
		c := obj(m, "challenge")
		de := obj(c, "challenger")
		if texto(c, "status") != "created" || strings.EqualFold(texto(de, "id"), a.yo.ID) || texto(de, "id") == "" {
			return // los tuyos también llegan aquí
		}
		for _, r := range a.retos {
			if r.ID == texto(c, "id") {
				return
			}
		}
		a.retos = append(a.retos, retoEntrante{ID: texto(c, "id"), De: texto(de, "name"), Elo: entero(de, "rating"),
			Ritmo: ritmoDe(c), Puntua: verdad(c, "rated")})
		a.son.reto()
		if a.luz.apagada {
			a.luz.encender()
		}
	case "challengeCanceled", "challengeDeclined":
		id := texto(obj(m, "challenge"), "id")
		for i, r := range a.retos {
			if r.ID == id {
				a.retos = append(a.retos[:i], a.retos[i+1:]...)
				break
			}
		}
	case "gameStart":
		g := obj(m, "game")
		id := texto(g, "gameId")
		if id == "" || (a.pa != nil && a.pa.id == id) {
			return
		}
		esperada := a.pant == pEsperando && (id == a.retoID || a.contraMaquina)
		if esperada || a.pant == pMenu || a.pant == pEmpezadas || a.pant == pRival || a.pant == pOpciones {
			a.abrirPartida(id)
		} else {
			a.aviso("Ha empezado una partida contra " + texto(obj(g, "opponent"), "username") + ": la tienes en Partidas empezadas")
		}
	}
}

// ---- Cuenta --------------------------------------------------------------------------------------

func (a *app) irACuenta() {
	a.pant = pCuenta
	if a.web != nil {
		return
	}
	a.sesionTexto = "Esperando tu token…"
	a.web = &paginaSesion{validar: func(texto string) error {
		a.hacer(func() { a.sesionTexto = "Comprobando el token con Lichess…" })
		u, err := a.li.ponerCuenta(texto)
		a.hacer(func() {
			if err != nil {
				a.sesionTexto = "Lichess no ha aceptado ese token. Prueba otra vez."
				return
			}
			a.web.cerrar()
			a.web = nil
			a.aviso("¡Hola, " + u + "!")
			a.pant = pMenu
			a.cargarCuenta()
			go a.escucharAvisos()
		})
		return err
	}}
	if err := a.web.abrir(); err != nil {
		a.sesionTexto = "No se pudo abrir la página: " + err.Error()
	}
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
			if a.pz != nil && a.pant == pProblema {
				a.moverRivalProblema()
			}
			// el reloj de la partida y los puntitos: una vez cada 400 ms basta
			if p := ahora.UnixMilli() / 400; p != a.segundoVisto && (a.pant == pPartida || a.pant == pEsperando || a.cargando) {
				a.segundoVisto, a.cambios = p, true
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
	a.repetirEn = time.Now().Add(120 * time.Millisecond)
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
	// un reto que te llega se contesta antes que nada (menos en mitad de una partida: ahí sale arriba)
	if len(a.retos) > 0 && a.pant != pPartida {
		r := a.retos[0]
		switch b {
		case bA:
			a.retos = a.retos[1:]
			go func() {
				if err := a.li.Aceptar(r.ID); err != nil {
					a.hacer(func() { a.aviso(textoError(err)) })
				}
			}()
			a.aviso("Aceptado: empieza la partida…")
		case bB:
			a.retos = a.retos[1:]
			go func() { _ = a.li.Rechazar(r.ID) }()
		}
		return
	}
	switch a.pant {
	case pCuenta:
		if b == bB {
			if a.li.cuenta.tiene() {
				a.web.cerrar()
				a.web = nil
				a.pant = pMenu
			} else {
				a.terminar()
			}
		}
	case pMenu:
		a.pulsarMenu(b)
	case pRival:
		a.pulsarRival(b)
	case pEscribir:
		switch a.tec.pulsar(b) {
		case tecEnviar:
			if n := strings.TrimSpace(a.tec.texto()); n != "" {
				a.rival = n
				a.irAOpciones(false)
			}
		case tecCancelar:
			a.pant = pRival
		}
	case pOpciones:
		a.pulsarOpciones(b)
	case pEsperando:
		if b == bB {
			if a.cancelarReto != nil {
				a.cancelarReto()
			}
			id := a.retoID
			go a.li.CancelarReto(id)
			a.pant = pMenu
		}
	case pEmpezadas:
		a.pulsarEmpezadas(b)
	case pPartida:
		a.pulsarPartida(b)
	case pProblema:
		a.pulsarProblema(b)
	}
}

var opcionesMenu = []string{"Jugar con un amigo", "Contra la máquina", "Problemas", "Partidas empezadas"}

func (a *app) pulsarMenu(b uint16) {
	switch b {
	case bArriba:
		a.menuSel = (a.menuSel + len(opcionesMenu) - 1) % len(opcionesMenu)
	case bAbajo:
		a.menuSel = (a.menuSel + 1) % len(opcionesMenu)
	case bIzquierda, bDerecha:
		a.menuSel ^= 1
	case bA, bStart:
		switch a.menuSel {
		case 0:
			a.irARival()
		case 1:
			a.irAOpciones(true)
		case 2:
			a.nuevoProblema()
		case 3:
			a.irAEmpezadas()
		}
	case bSelect:
		a.irACuenta()
	case bB:
		a.terminar()
	}
}

// ---- Retar a alguien -----------------------------------------------------------------------------

func (a *app) irARival() {
	a.pant, a.listaSel = pRival, 0
	a.lista = append([]string{}, a.li.cuenta.rivales()...)
	a.cargando = true
	go func() {
		sig, _ := a.li.Siguiendo()
		a.hacer(func() {
			a.cargando = false
			for _, s := range sig {
				ya := false
				for _, x := range a.lista {
					if strings.EqualFold(x, s) {
						ya = true
					}
				}
				if !ya {
					a.lista = append(a.lista, s)
				}
			}
		})
	}()
}

func (a *app) pulsarRival(b uint16) {
	n := len(a.lista) + 1 // la primera es «Escribir un nombre»
	switch b {
	case bArriba:
		a.listaSel = (a.listaSel + n - 1) % n
	case bAbajo:
		a.listaSel = (a.listaSel + 1) % n
	case bA, bStart:
		if a.listaSel == 0 {
			a.tec.borrarTodo()
			a.tec.abrir()
			a.pant = pEscribir
			return
		}
		a.rival = a.lista[a.listaSel-1]
		a.irAOpciones(false)
	case bB:
		a.pant = pMenu
	}
}

func (a *app) irAOpciones(maquina bool) {
	a.contraMaquina, a.opSel, a.pant = maquina, 0, pOpciones
}

// las filas de pOpciones
func (a *app) filasOpciones() []string {
	if a.contraMaquina {
		return []string{"nivel", "ritmo", "color", "ir"}
	}
	return []string{"ritmo", "color", "puntua", "ir"}
}

func (a *app) pulsarOpciones(b uint16) {
	filas := a.filasOpciones()
	fila := filas[a.opSel]
	cambiar := func(d int) {
		switch fila {
		case "nivel":
			a.nivel = min(max(a.nivel+d, 1), 8)
		case "ritmo":
			a.ritmoSel = (a.ritmoSel + len(ritmos) + d) % len(ritmos)
		case "color":
			a.colorSel = (a.colorSel + 3 + d) % 3
		case "puntua":
			a.puntua = !a.puntua
		}
	}
	switch b {
	case bArriba:
		a.opSel = (a.opSel + len(filas) - 1) % len(filas)
	case bAbajo:
		a.opSel = (a.opSel + 1) % len(filas)
	case bIzquierda:
		cambiar(-1)
	case bDerecha:
		cambiar(1)
	case bA:
		if fila != "ir" {
			cambiar(1)
			return
		}
		a.lanzarReto()
	case bStart:
		a.lanzarReto()
	case bB:
		if a.contraMaquina {
			a.pant = pMenu
		} else {
			a.pant = pRival
		}
	}
}

var nombresColor = []string{"Al azar", "Blancas", "Negras"}
var clavesColor = []string{"random", "white", "black"}

func (a *app) lanzarReto() {
	if !hayRed() {
		a.aviso("Sin internet")
		return
	}
	r, color := ritmos[a.ritmoSel], clavesColor[a.colorSel]
	if a.contraMaquina {
		a.pant, a.esperandoA, a.retoID = pEsperando, "", ""
		nivel := a.nivel
		go func() {
			id, err := a.li.RetarMaquina(nivel, r, color)
			a.hacer(func() {
				if err != nil {
					a.pant = pMenu
					a.aviso(textoError(err))
					return
				}
				if a.pant == pEsperando {
					a.abrirPartida(id)
				}
			})
		}()
		return
	}
	rival, puntua := a.rival, a.puntua
	a.li.cuenta.apuntarRival(rival)
	ctx, cancel := context.WithCancel(context.Background())
	a.pant, a.esperandoA, a.retoID, a.cancelarReto = pEsperando, rival, "", cancel
	go func() {
		id, err := a.li.Retar(ctx, rival, r, color, puntua, func(id string) {
			a.hacer(func() { a.retoID = id })
		})
		a.hacer(func() {
			if a.pant != pEsperando || a.esperandoA != rival {
				return
			}
			switch {
			case err != nil && ctx.Err() != nil: // cancelado
			case err != nil:
				a.pant = pMenu
				a.aviso(rival + ": " + textoError(err))
			case r.Dias > 0:
				a.pant = pMenu
				a.aviso("Reto mandado a " + rival + ": cuando acepte saldrá en Partidas empezadas")
			case id != "":
				a.abrirPartida(id)
			}
		})
	}()
}

// ---- Partidas empezadas --------------------------------------------------------------------------

func (a *app) irAEmpezadas() {
	a.pant, a.empSel, a.cargando = pEmpezadas, 0, true
	go func() {
		ps, err := a.li.Jugando()
		a.hacer(func() {
			a.cargando = false
			if err != nil {
				a.aviso(textoError(err))
				return
			}
			a.empezadas = ps
			a.empSel = min(a.empSel, max(0, len(ps)-1))
		})
	}()
}

func (a *app) pulsarEmpezadas(b uint16) {
	switch b {
	case bArriba:
		a.empSel = max(a.empSel-1, 0)
	case bAbajo:
		a.empSel = min(a.empSel+1, max(0, len(a.empezadas)-1))
	case bA, bStart:
		if a.empSel < len(a.empezadas) {
			a.abrirPartida(a.empezadas[a.empSel].ID)
		}
	case bY:
		a.irAEmpezadas()
	case bB:
		a.pant = pMenu
	}
}
