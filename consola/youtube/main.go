// YouTube para la consola (Panther OS: dArkOS4Clone en la XF35H): buscar vídeos, verlos y
// descargarlos para verlos sin internet.
//
// La búsqueda se hace como la web de YouTube (innertube, sin cuenta) y el vídeo lo sacan yt-dlp (que
// da la dirección del vídeo de 360p) y ffmpeg (que lo convierte en imágenes de 640×360 y en sonido).
// Las imágenes se mandan al núcleo de la pantalla de salas (lakka/nucleo) como en el WhatsApp y
// Spotin de la consola, y el sonido va a aplay; la imagen sigue al sonido para que no se separen.
// SELECT apaga la pantalla y el sonido sigue (para oír música).
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
// Con -probar "lo que sea" busca en YouTube y dice qué vídeo saldría y de dónde (sin pantalla).
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
	carpetaDatos   = entorno("YOUTUBE_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "youtube"))
	carpetaVideos  = entorno("YOUTUBE_VIDEOS", "/roms/videos/youtube")
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
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, ruta) != nil {
		_ = os.WriteFile(ruta, datos, 0o644)
		_ = os.Remove(tmp)
	}
}

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	probar := flag.String("probar", "", "sin pantalla: busca esto en YouTube, dice qué vídeos salen y de dónde se vería el primero")
	probarF := flag.String("probar-filas", "", "sin pantalla: los Shorts de esta búsqueda, su secuencia y (con cuenta) tus filas")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
	}
	if *probar != "" {
		probarBusqueda(*probar)
		return
	}
	if *probarF != "" {
		probarFilas(*probarF)
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
	// El registro se va añadiendo (para ver luego qué ha fallado), y se vacía si pasa de 256 KB.
	rutaReg := filepath.Join(carpetaDatos, "registro.txt")
	if st, err := os.Stat(rutaReg); err == nil && st.Size() > 256<<10 {
		_ = os.Truncate(rutaReg, 0)
	}
	if reg, err := os.OpenFile(rutaReg, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
	}
	log.Printf("YouTube de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	a.alm = nuevoAlmacen(carpetaDatos, carpetaVideos)
	a.mini = nuevasMiniaturas(a, filepath.Join(carpetaDatos, "miniaturas"))
	a.yt = nuevoYouTube()
	a.yt.cuenta = cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	abrirYtdlp() // yt-dlp se va abriendo ya: cuando elijas un vídeo, ya estará listo
	a.rep = nuevoReproductorVideo(a.componer, a.videoAcabado, a.videoFallido)
	botones := make(chan uint16, 64)
	a.srv = nuevoServidorPantalla(botones)
	if err := a.srv.escuchar(); err != nil {
		log.Fatalf("no se pudo abrir la pantalla (¿hay otra app abierta?): %v", err)
	}
	// El logo ya mismo en la pantalla (EmulationStation la acaba de soltar) y como primer marco.
	a.dibujarCarga()
	pintarFB(a.l.img)
	a.srv.enviar(a.l.marco())

	ra, err := abrirRetroArch()
	if err != nil {
		log.Fatalf("no se pudo abrir RetroArch: %v", err)
	}
	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		log.Printf("señal %v: salgo", s)
		a.terminar()
	}()

	go vigilarRed(a)
	go a.descargador()
	a.arrancar()
	a.bucle(botones, ra)

	a.luz.encender()
	a.guardarPosicion()
	a.rep.cerrar()
	cerrarYtdlp()
	a.pararDescargas()
	a.web.cerrar()
	ra.cerrar()
	a.cargaHasta = time.Now().Add(time.Hour)
	a.l.limpiarFondo()
	a.dibujarCarga()
	pintarFB(a.l.img)
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}
