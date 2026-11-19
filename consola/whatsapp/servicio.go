package main

import (
	"bufio"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// El WhatsApp de la consola como servicio: siempre conectado, aunque estés jugando.
//
//   - Cuando te escriben, pone arriba un aviso pequeño durante 2 segundos (foto, nombre y el principio
//     del mensaje), encima del juego o del menú (/dev/shm/panther-capa).
//   - Con R3 (o el botón F) se abre ese chat encima de lo que estés haciendo: el juego se queda parado
//     detrás y con B vuelves a él por donde ibas.
//   - Deja la lista de lo que te han escrito para la pantalla principal (/dev/shm/panther-panel).
//   - Al abrir WhatsApp desde el menú (WhatsApp.sh), ese programa le pide por @panther-whatsapp que
//     se enseñe en pantalla y espera a que salgas.

const direccionServicio = "@panther-whatsapp"

type modoPantalla int

const (
	modoSolo  modoPantalla = iota // el programa de siempre, sin servicio
	modoFondo                     // servicio sin pantalla
	modoApp                       // abierto desde el menú
	modoCapa                      // el chat encima de otra cosa
)

func servicio(f *fuentes) {
	if err := os.MkdirAll(carpetaDatos, 0o700); err != nil {
		log.Fatal(err)
	}
	if reg, err := os.OpenFile(filepath.Join(carpetaDatos, "servicio.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
		os.Stdout = reg
	}
	log.Printf("servicio de WhatsApp en marcha")
	a := nuevaApp(f)
	a.modo = modoFondo
	a.media = nuevosMedios(a, filepath.Join(carpetaDatos, "media"))
	var err error
	if a.capa, err = abrirCapa("/dev/shm/panther-capa", ancho, alto); err != nil {
		logf("sin avisos: %v", err)
	}
	if a.panel, err = abrirCapa("/dev/shm/panther-panel", anchoPanel, altoPanel); err != nil {
		logf("sin panel: %v", err)
	}
	a.capa.poner(0, nil, 0, 0, 0)
	if a.barra, err = abrirCapa("/dev/shm/panther-barra", maxAnchoBarr, altoBarra); err != nil {
		logf("sin recuadro de arriba: %v", err)
	}
	if a.volumen, err = abrirCapa("/dev/shm/panther-volumen", anchoVolumen, altoVolumen); err != nil {
		logf("sin barra de volumen: %v", err)
	}
	a.volumen.poner(0, nil, 0, 0, 0)
	if a.teclasVol, err = abrirTeclasVolumen(); err != nil {
		logf("%v", err)
	}
	if a.mando, err = abrirMando(); err != nil {
		logf("sin mando: %v (no se podrá abrir el chat encima)", err)
	}
	w, err := nuevoWA(a)
	if err != nil {
		log.Fatalf("no se pudo preparar WhatsApp: %v", err)
	}
	a.w = w
	a.botonesCore = make(chan uint16, 64)

	ln, err := net.Listen("unix", direccionServicio)
	if err != nil {
		log.Fatalf("¿ya hay otro servicio? %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				linea, err := bufio.NewReader(c).ReadString('\n')
				if err != nil || strings.TrimSpace(linea) != "abrir" {
					c.Close()
					return
				}
				a.hacer(func() { a.pedirAbrir(c) })
			}()
		}
	}()

	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		log.Printf("señal %v: salgo", s)
		a.terminar()
	}()

	w.arrancar()
	a.panelSucio = true
	a.bucleServicio()

	if a.modo == modoApp {
		a.cerrarApp()
	}
	if a.modo == modoCapa {
		a.capa.poner(0, nil, 0, 0, 0)
		a.mando.agarrar(false)
	}
	if a.grab != nil {
		a.cambiarGrabacion()
		time.Sleep(2 * time.Second)
	}
	a.panel.poner(0, nil, 0, 0, 0)
	a.barra.poner(0, nil, 0, 0, 0)
	w.cerrar()
	ln.Close()
	log.Printf("servicio parado")
}

func (a *app) bucleServicio() {
	reloj := time.NewTicker(40 * time.Millisecond)
	defer reloj.Stop()
	guardado := time.NewTicker(20 * time.Second)
	defer guardado.Stop()
	var cambiosMando <-chan uint32
	if a.mando != nil {
		cambiosMando = a.mando.cambios
	}
	a.ponerBarra(true)
	barraReloj := time.NewTicker(2 * time.Second)
	defer barraReloj.Stop()
	for {
		var raHecho <-chan error
		if a.ra != nil {
			raHecho = a.ra.hecho
		}
		select {
		case <-a.salir:
			return
		case fn := <-a.tareas:
			fn()
			a.vaciarTareas()
			a.cambios = true
			if v := a.w.version(); v != a.panelVisto { // el panel, solo si han cambiado los chats
				a.panelVisto, a.panelSucio = v, true
			}
		case m := <-a.botonesCore:
			if a.modo == modoApp {
				a.botones(m)
				a.dibujarYa = true
			}
		case e := <-cambiosMando:
			a.mandoCambia(e)
			a.dibujarYa = true
		case d := <-a.teclasVol:
			a.cambiarVolumen(5 * d)
		case <-barraReloj.C:
			for _, c := range []*capaCompartida{a.capa, a.panel, a.barra, a.volumen} {
				c.vigilar() // por si systemd las ha borrado al cerrarse una sesión
			}
			a.ponerBarra(false) // la hora, la batería y si hay mensajes
		case err := <-raHecho:
			logf("la pantalla de WhatsApp se ha cerrado (%v)", err)
			a.ra = nil
			a.cerrarApp()
		case <-reloj.C:
			if a.modo == modoApp || a.modo == modoCapa {
				a.repetir()
				if c := a.enCarga(); c != a.enCargaAntes {
					a.enCargaAntes, a.cambios = c, true
				}
				if !a.avisoHasta.IsZero() && time.Now().After(a.avisoHasta) {
					a.avisoHasta, a.avisoTexto, a.cambios = time.Time{}, "", true
				}
			}
			if a.modo == modoCapa {
				a.capa.latido()
			}
		case <-guardado.C:
			a.w.guardar()
		}
		if a.modo == modoFondo {
			a.cambios = false // sin pantalla no se pinta nada
		}
		if a.tocaPintar() {
			switch a.modo {
			case modoApp:
				a.dibujar()
				if a.srv != nil {
					a.srv.enviarCon(a.l.marcoEn)
				}
			case modoCapa:
				if !a.esperaSoltar {
					a.dibujar()
					a.capa.poner(2, a.l.img, 0, 0, 0)
				}
			}
		}
		if a.panelSucio && time.Since(a.panelHecho) > 700*time.Millisecond {
			a.panelSucio = false
			a.panelHecho = time.Now()
			a.panel.poner(1, a.dibujarPanel(), sitioPanel.X, sitioPanel.Y, 0)
			a.ponerBarra(false) // el logo de WhatsApp sale o se va con los mensajes sin leer
		}
	}
}

// ---- El chat encima de lo que estés haciendo ----

// mandoCambia: lo que se pulsa en el mando (lo vemos siempre; solo nos lo quedamos con el chat abierto).
func (a *app) mandoCambia(e uint32) {
	nuevos := e &^ a.mandoAntes
	a.mandoAntes = e
	if !a.esperaSoltar {
		a.combinaciones(e, nuevos) // capturas (L1+R1) y grabar (L2+R2), en cualquier pantalla
	}
	switch a.modo {
	case modoFondo:
		if nuevos&(bR3|bF) != 0 && hayQuienPinte() {
			a.abrirCapa()
		}
	case modoCapa:
		if a.esperaSoltar {
			if e == 0 { // ya no hay nada pulsado: ahora sí
				a.esperaSoltar = false
				if a.cerrando {
					a.cerrando = false
					a.mando.agarrar(false)
					a.modo = modoFondo
					a.panelSucio = true
				} else {
					a.mando.agarrar(true)
					a.pulsados = 0
					a.cambios = true
				}
			}
			return
		}
		if nuevos&(bR3|bF) != 0 {
			a.cerrarCapa()
			return
		}
		a.botones(uint16(e))
	}
}

// abrirCapa: se abre al soltar el botón (así el juego no se queda con nada pulsado).
func (a *app) abrirCapa() {
	a.modo = modoCapa
	a.esperaSoltar, a.cerrando = true, false
	a.cargaMin, a.cargaMax = time.Time{}, time.Time{}
	if a.ultimoAviso != nil && time.Since(a.ultimoAvisoHora) < 10*time.Minute {
		a.abrir(a.ultimoAviso)
	} else {
		a.pant, a.selChat, a.primera = pChats, 0, 0
	}
	a.ultimoAviso = nil
	a.cambios = true
	// La imagen ya, aunque el mando se quede hasta que sueltes: así se ve que se ha abierto.
	a.dibujar()
	a.capa.poner(2, a.l.img, 0, 0, 0)
	if a.mando.ahora() == 0 {
		a.mandoCambia(0)
	}
}

// cerrarCapa: se quita el chat (el juego sigue) y el mando se suelta cuando no haya nada pulsado.
func (a *app) cerrarCapa() {
	a.capa.poner(0, nil, 0, 0, 0)
	a.esperaSoltar, a.cerrando = true, true
	a.panelSucio = true
	if a.mando.ahora() == 0 {
		a.mandoCambia(0)
	}
}

// salirPantalla: «Salir» en el menú de la app.
func (a *app) salirPantalla() {
	switch a.modo {
	case modoApp:
		a.cerrarApp()
	case modoCapa:
		a.cerrarCapa()
	default:
		a.terminar()
	}
}

// ---- Avisos ----

// avisoNuevo: te acaban de escribir.
func (a *app) avisoNuevo(c *Chat, m *Mensaje, quien string) {
	a.panelSucio = true
	if a.modo != modoFondo {
		a.aviso(quien + ": " + m.Texto)
		return
	}
	// Los que llegan al conectarse (de cuando la consola estaba apagada) no avisan: salen en la lista.
	if time.Since(m.Hora) > 2*time.Minute {
		return
	}
	a.ultimoAviso, a.ultimoAvisoHora = c, time.Now()
	a.capa.poner(1, a.dibujarAviso(c, m), sitioAviso.X, sitioAviso.Y, 2*time.Second)
}

// ---- WhatsApp desde el menú ----

func (a *app) pedirAbrir(c net.Conn) {
	if a.modo == modoApp {
		_, _ = io.WriteString(c, "ocupado\n")
		c.Close()
		return
	}
	if a.modo == modoCapa {
		a.capa.poner(0, nil, 0, 0, 0)
		a.mando.agarrar(false)
		a.esperaSoltar, a.cerrando = false, false
	}
	a.srv = nuevoServidorPantalla(a.botonesCore)
	if err := a.srv.escuchar(); err != nil {
		logf("no se pudo abrir la pantalla: %v", err)
		_, _ = io.WriteString(c, "cerrado\n")
		c.Close()
		a.srv = nil
		return
	}
	a.cliente = c
	a.modo = modoApp
	a.pant, a.selChat, a.primera = pChats, 0, 0
	a.cargaMin, a.cargaMax = time.Now().Add(time.Second), time.Now().Add(6*time.Second)
	a.dibujarCarga()
	pintarFB(a.l.img)
	a.srv.enviarCon(a.l.marcoEn)
	ra, err := abrirRetroArch()
	if err != nil {
		logf("no se pudo abrir la pantalla: %v", err)
		a.cerrarApp()
		return
	}
	a.ra = ra
	// Si el menú cierra WhatsApp.sh (o se cae), cerramos también.
	go func() {
		b := make([]byte, 1)
		_, _ = c.Read(b)
		a.hacer(func() {
			if a.cliente == c {
				a.cerrarApp()
			}
		})
	}()
}

func (a *app) cerrarApp() {
	a.w.guardar() // al salir, lo de ahora ya guardado
	if a.ra != nil {
		a.ra.cerrar()
		a.ra = nil
	}
	if a.srv != nil {
		a.srv.cerrar()
		a.srv = nil
	}
	a.dibujarCarga()
	pintarFB(a.l.img)
	if a.cliente != nil {
		_, _ = io.WriteString(a.cliente, "cerrado\n")
		a.cliente.Close()
		a.cliente = nil
	}
	a.modo = modoFondo
	a.panelSucio = true
}

// pedirAlServicio: lo que hace WhatsApp.sh. Si el servicio está, le pide que se enseñe y espera a
// que salgas; si no está, devuelve false y se abre el programa de siempre.
func pedirAlServicio() bool {
	c, err := net.Dial("unix", direccionServicio)
	if err != nil {
		return false
	}
	defer c.Close()
	if _, err := io.WriteString(c, "abrir\n"); err != nil {
		return false
	}
	r := bufio.NewReader(c)
	for {
		linea, err := r.ReadString('\n')
		if err != nil {
			return true // el servicio se ha ido: volvemos al menú
		}
		switch strings.TrimSpace(linea) {
		case "cerrado", "ocupado":
			return true
		}
	}
}
