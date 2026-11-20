// Zarpa: carrera por Los Ángeles. Un juego hecho para la consola (Panther OS en la XF35H): Zarpa,
// una pantera pequeña con bufanda y mochila, corre por una autopista de Los Ángeles al atardecer.
// Tres carriles; vallas que saltar, carteles de autopista para pasar agachada y camiones de tacos que
// esquivar; estrellas que coger, imán y escudo; trajes en la tienda; récord y estrellas guardados.
// Funciona sin internet.
//
// Como las apps de la consola, la pantalla es el emulador de Panther (o RetroArch con el núcleo
// «Pokemulador»): este programa le manda las imágenes (30 por segundo) y él le pasa los botones.
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
	carpetaDatos   = entorno("ZARPA_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "zarpa"))
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
	medir := flag.Bool("medir", false, "sin pantalla: cuánto tarda en dibujar cada imagen jugando")
	flag.Parse()
	f := cargarFuentes()
	if *medir {
		medirDibujo(f)
		return
	}
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
	inicio := time.Now()
	a := nuevaApp(f, cargarProgreso(filepath.Join(carpetaDatos, "progreso.json")))
	log.Printf("Zarpa en marcha (dibujos hechos en %.1f s)", time.Since(inicio).Seconds())
	botones := make(chan uint16, 64)
	a.srv = nuevoServidorPantalla(botones)
	if err := a.srv.escuchar(); err != nil {
		log.Fatalf("no se pudo abrir la pantalla (¿hay otra app abierta?): %v", err)
	}
	a.dibujar()
	pintarFB(a.l.img)
	a.srv.enviar(a.l.marco())
	ra, err := abrirRetroArch()
	if err != nil {
		log.Fatalf("no se pudo abrir la pantalla: %v", err)
	}
	a.son.abrir()
	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		log.Printf("señal %v: salgo", s)
		a.terminar()
	}()

	// 30 imágenes por segundo: cada una mueve el juego lo que ha pasado desde la anterior.
	reloj := time.NewTicker(time.Second / 30)
	defer reloj.Stop()
	antes := time.Now()
	lentos, cuenta := 0, 0
	for corriendo := true; corriendo; {
		select {
		case <-a.salir:
			corriendo = false
		case err := <-ra.hecho:
			logf("la pantalla se ha cerrado (%v): salgo", err)
			corriendo = false
		case m := <-botones:
			a.botones(m)
		case ahora := <-reloj.C:
			dt := ahora.Sub(antes).Seconds()
			antes = ahora
			a.paso(min(dt, 0.1))
			t0 := time.Now()
			if !a.luz.apagada {
				a.dibujar()
				a.srv.enviarCon(a.l.marcoEn)
			}
			if time.Since(t0) > 30*time.Millisecond {
				lentos++
			}
			if cuenta++; cuenta%900 == 0 { // cada 30 s, cuántas imágenes no han llegado a tiempo
				logf("imágenes lentas: %d de 900", lentos)
				lentos = 0
			}
		}
	}
	if a.pant == pJuego && a.c != nil && a.c.viva { // si se sale jugando, las estrellas se guardan
		a.pr.Estrellas += a.c.estrellas
		a.pr.Total += a.c.estrellas
	}
	a.pr.guardar()
	a.son.cerrar()
	a.luz.encender()
	ra.cerrar()
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}
