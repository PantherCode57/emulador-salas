// Ajedrez para la consola (Panther OS: dArkOS4Clone en la XF35H), con tu cuenta de Lichess: retar a
// tus amigos (ellos juegan desde el móvil o la web de Lichess), jugar contra la máquina de Lichess,
// aceptar los retos que te lleguen, seguir tus partidas empezadas y hacer problemas.
//
// Usa la API oficial de Lichess (la «Board API», que es para jugar desde programas y tableros que no
// son su web) con un token tuyo (ver web.go). La pantalla es el emulador de Panther (o RetroArch con
// el núcleo «Pokemulador»), como en las otras apps: este programa le manda las imágenes y él le pasa
// los botones.
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
// Con -probar dice, con el token guardado, qué contesta Lichess (solo si va y cuántos).
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

var (
	carpetaDatos   = entorno("AJEDREZ_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "ajedrez"))
	retroarchBin   = entorno("CONSOLA_RETROARCH", "retroarch")
	nucleoPantalla = entorno("CONSOLA_NUCLEO", "pokemulador_libretro.so")
)

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func logf(format string, args ...any) { log.Printf(format, args...) }

func itoa(n int) string { return strconv.Itoa(n) }

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	probar := flag.Bool("probar", false, "sin pantalla: con el token guardado, dice qué contesta Lichess")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
	}
	if *probar {
		probarCuenta()
		return
	}
	f := cargarFuentes()
	if *prueba != "" {
		if err := dibujarPruebas(f, *prueba); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := os.MkdirAll(carpetaDatos, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if reg, err := os.OpenFile(filepath.Join(carpetaDatos, "registro.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
	}
	log.Printf("Ajedrez de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f, &lichess{cuenta: cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))})
	botones := make(chan uint16, 64)
	a.srv = nuevoServidorPantalla(botones)
	if err := a.srv.escuchar(); err != nil {
		log.Fatalf("no se pudo abrir la pantalla (¿hay otra app abierta?): %v", err)
	}
	a.dibujarCarga()
	pintarFB(a.l.img)
	a.srv.enviar(a.l.marco())
	ra, err := abrirRetroArch()
	if err != nil {
		log.Fatalf("no se pudo abrir la pantalla: %v", err)
	}
	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		log.Printf("señal %v: salgo", s)
		a.terminar()
	}()
	go vigilarRed(a)
	a.arrancar()
	a.bucle(botones, ra)

	if a.cancelarReto != nil {
		a.cancelarReto()
	}
	if a.pa != nil && a.pa.cancelar != nil {
		a.pa.cancelar()
	}
	a.web.cerrar()
	a.luz.encender()
	ra.cerrar()
	a.cargaHasta = time.Now().Add(time.Hour)
	a.l.limpiarFondo()
	a.dibujarCarga()
	pintarFB(a.l.img)
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// probarCuenta: con el token de la consola, qué contesta Lichess (si va y cuántos; nada de dentro).
func probarCuenta() {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay token guardado en", carpetaDatos)
		return
	}
	c.ruta = ""
	li := &lichess{cuenta: c}
	log.SetOutput(os.Stdout)
	yo, err := li.Cuenta()
	fmt.Printf("cuenta: %s (puntuaciones: %d)\n", siError(err), len(yo.Ratings))
	ps, err := li.Jugando()
	fmt.Printf("partidas empezadas: %s, %d\n", siError(err), len(ps))
	sig, err := li.Siguiendo()
	fmt.Printf("siguiendo: %s, %d\n", siError(err), len(sig))
	pr, err := li.Problema("")
	_, leido := posicionProblema(pr)
	fmt.Printf("problema: %s, se lee: %v, jugadas de la solución: %d, con FEN: %v\n", siError(err), leido, len(pr.Solucion), pr.FEN != "")
}

func siError(err error) string {
	if err != nil {
		return "MAL (" + err.Error() + ")"
	}
	return "bien"
}
