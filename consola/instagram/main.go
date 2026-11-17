// Instagram para la consola (Panther OS: dArkOS4Clone en la XF35H): tu inicio, los reels, las
// historias y tus mensajes, con tus «me gusta» y para contestar.
//
// Habla con Instagram como su web (www.instagram.com) con tu sesión, que se pasa desde el PC (ver
// web.go). Las fotos se bajan solo a la memoria. Los vídeos los lee ffmpeg directamente de su
// dirección (imágenes para la pantalla y sonido para aplay, como en el YouTube de la consola).
// La pantalla es el emulador de Panther (o RetroArch con el núcleo «Pokemulador»), como en las
// otras apps: este programa le manda las imágenes y él le pasa los botones.
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
// Con -probar dice, con la sesión guardada, qué contesta Instagram a cada cosa (solo cuántas y si
// ha ido bien; nada de lo que hay dentro).
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
	carpetaDatos   = entorno("INSTAGRAM_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "instagram"))
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
	probar := flag.Bool("probar", false, "sin pantalla: con la sesión guardada, dice qué contesta Instagram (solo cuántas)")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
	}
	if *probar {
		probarSesion()
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
	log.Printf("Instagram de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	a.ig = nuevoInstagram(cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json")))
	a.img = nuevasImagenes(a)
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

	a.rep.cerrar()
	a.web.cerrar()
	ra.cerrar()
	a.cargaHasta = time.Now().Add(time.Hour)
	a.l.limpiarFondo()
	a.dibujarCarga()
	pintarFB(a.l.img)
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// probarSesion: con la sesión de la consola, qué contesta Instagram a cada pestaña (cuántas cosas y
// si van con foto o vídeo). No escribe nada de lo que hay dentro (ni nombres ni textos).
func probarSesion() {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay sesión guardada en", carpetaDatos)
		return
	}
	c.ruta = "" // no se guarda nada al probar
	ig := nuevoInstagram(c)
	log.SetOutput(os.Stdout)
	_, _, _, err := ig.QuienSoy()
	fmt.Println("quién soy:", siError(err))
	contar := func(nombre string, p Pagina, err error) {
		fotos, videos, varias := 0, 0, 0
		for _, x := range p.Posts {
			if len(x.Medios) > 1 {
				varias++
			}
			if x.Medios[0].esVideo() {
				videos++
			} else if x.Medios[0].Foto != "" {
				fotos++
			}
		}
		fmt.Printf("%s: %s, %d (fotos %d, vídeos %d, de varias %d), sigue: %v\n", nombre, siError(err), len(p.Posts), fotos, videos, varias, p.Mas)
	}
	p, err := ig.Inicio("")
	contar("inicio", p, err)
	if err == nil && p.Mas {
		p2, err := ig.Inicio(p.Cursor)
		contar("inicio (más)", p2, err)
	}
	p, err = ig.Reels("")
	contar("reels", p, err)
	bs, err := ig.Historias()
	fmt.Printf("historias: %s, %d personas\n", siError(err), len(bs))
	if err == nil && len(bs) > 0 {
		items, err := ig.HistoriasDe(bs[0].ID)
		fmt.Printf("historias de la primera: %s, %d\n", siError(err), len(items))
	}
	cs, err := ig.Chats()
	fmt.Printf("chats: %s, %d\n", siError(err), len(cs))
	if err == nil && len(cs) > 0 {
		ms, _, err := ig.Conversacion(cs[0].ID, "")
		fmt.Printf("mensajes del primero: %s, %d\n", siError(err), len(ms))
	}
}

func siError(err error) string {
	if err != nil {
		return "MAL (" + err.Error() + ")"
	}
	return "bien"
}
