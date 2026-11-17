// Claude en la consola (Panther OS: dArkOS4Clone en la XF35H): un chat con la conversación de
// Claude Code que hay abierta en el PC de casa.
//
// La consola no puede llevar a Claude dentro: lo que escribes se deja en una carpeta y el PC lo
// recoge por SSH, se lo da a Claude y deja aquí la respuesta (ver chat.go). Por eso solo contesta
// mientras el PC y esa conversación están abiertos; si no, los mensajes esperan. Como el resto de
// apps de la consola, el emulador de Panther (con el núcleo de la pantalla de salas) enseña la
// imagen y nos pasa los botones.
//
// Con -prueba <carpeta> dibuja las pantallas de ejemplo en PNG.
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
	carpetaDatos   = entorno("CLAUDE_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "claude"))
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

func escribirAtomico(ruta string, datos []byte) {
	tmp := ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o600) != nil {
		return
	}
	if os.Rename(tmp, ruta) != nil {
		_ = os.WriteFile(ruta, datos, 0o600)
		_ = os.Remove(tmp)
	}
}

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
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
	log.Printf("Claude de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f, abrirChat(carpetaDatos))
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
	a.bucle(botones, ra)

	a.luz.encender()
	ra.cerrar()
	a.cargaHasta = time.Now().Add(time.Hour)
	a.l.limpiarFondo()
	a.dibujarCarga()
	pintarFB(a.l.img)
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}
