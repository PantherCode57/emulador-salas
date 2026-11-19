// Duolingo para la consola (Panther OS: dArkOS4Clone en la XF35H): tu curso, tu racha y tus
// lecciones con los botones, y al acabarlas se mandan a Duolingo para que cuenten (XP y racha) como
// en el móvil.
//
// Habla con Duolingo como su web (www.duolingo.com) con tu sesión, que se pasa desde el PC o el
// móvil (ver web.go). Las voces de los ejercicios se bajan a la memoria y suenan con ffmpeg y aplay.
// La pantalla es el emulador de Panther (o RetroArch con el núcleo «Pokemulador»), como en las otras
// apps: este programa le manda las imágenes y él le pasa los botones. La consola no tiene micrófono:
// los ejercicios de hablar no se piden.
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
// Con -probar dice, con la sesión guardada, qué contesta Duolingo (solo cuántos y de qué tipo; nada
// de tu cuenta). Empieza una lección pero no la manda, así que no cuenta.
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
	carpetaDatos   = entorno("DUOLINGO_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "duolingo"))
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
	probar := flag.Bool("probar", false, "sin pantalla: con la sesión guardada, dice qué contesta Duolingo (solo cuántos)")
	volcarA := flag.String("volcar", "", "sin pantalla: guarda en este archivo cómo es tu curso y una lección (sin mandarla)")
	flag.Parse()
	if *volcarA != "" {
		log.SetOutput(os.Stdout)
		volcar(*volcarA)
		return
	}
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
	log.Printf("Duolingo de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f, nuevoDuolingo(cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))))
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

	a.son.parar()
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

// probarSesion: con la sesión de la consola, qué contesta Duolingo (cuántos y de qué tipo). No
// escribe nada de tu cuenta, y la lección que empieza no se manda.
func probarSesion() {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay sesión guardada en", carpetaDatos)
		return
	}
	c.ruta = "" // no se guarda nada al probar
	d := nuevoDuolingo(c)
	log.SetOutput(os.Stdout)
	p, err := d.Perfil()
	fmt.Printf("perfil: %s (curso %q, racha>0 %v, meta %v, vidas %d)\n", siError(err), p.Curso, p.Racha > 0, p.Meta > 0, p.Vidas)
	us, actual, err := d.Camino()
	niveles, tipos := 0, map[string]int{}
	for _, u := range us {
		niveles += len(u.Niveles)
		for _, n := range u.Niveles {
			tipos[n.Tipo+"/"+n.Estado]++
		}
	}
	fmt.Printf("camino: %s, %d unidades, %d niveles, la de ahora la %d, tipos %v\n", siError(err), len(us), niveles, actual+1, tipos)
	probarLeccion := func(nombre string, pt peticion) {
		lec, err := d.Empezar(pt, p)
		if err != nil {
			fmt.Printf("%s: %s\n", nombre, siError(err))
			return
		}
		cuenta := map[string]int{}
		for _, e := range lec.ejs {
			cuenta[e.Clase]++
		}
		fmt.Printf("%s: bien, %d ejercicios (saltados %d): %v\n", nombre, len(lec.ejs), lec.saltados, cuenta)
	}
	probarLeccion("práctica", peticion{tipo: "practica"})
	if actual < len(us) {
		for _, n := range us[actual].Niveles {
			if n.Estado == estActivo && n.Peticion.tipo != "" {
				probarLeccion("lección de ahora", n.Peticion)
				break
			}
		}
	}
}

func siError(err error) string {
	if err != nil {
		return "MAL (" + err.Error() + ")"
	}
	return "bien"
}
