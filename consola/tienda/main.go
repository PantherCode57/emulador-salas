// Panther Store (Panther OS: dArkOS4Clone en la XF35H): poner y quitar con un botón las apps de
// Panther OS y juegos libres de PortMaster que ya vienen con sus datos.
//
// Las apps se bajan ya compiladas (la consola no puede compilar) de la fuente de la tienda
// (TIENDA_FUENTE) y, si no hay, se copian de ~/panther-instalar, donde las deja la instalación.
// Los juegos se bajan de PortMaster y se ponen como él, en /roms/ports. Para que la pantalla
// principal enseñe o quite su tarjeta hay que reiniciar EmulationStation: se hace al salir de la
// tienda, cuando no queda ninguna app ni juego abierto (ver reinicio.go).
//
// Como el resto de apps de la consola, el emulador de Panther (con el núcleo de la pantalla de
// salas) enseña la imagen y nos pasa los botones.
//
//	-prueba <carpeta>   dibuja las pantallas con datos inventados (PNG) y sale
//	-tarjeta <archivo>  dibuja la tarjeta de la pantalla principal (264×152) y sale
//	-reiniciar-es <pid> espera a que acabe ese proceso y a que no haya nada abierto, y reinicia
//	                    EmulationStation (lo lanza la propia tienda al salir)
//	-lista, -poner <id>, -quitar <id>
//	                    lo mismo que con los botones, sin pantalla (para probar por SSH); no
//	                    reinicia EmulationStation
package main

import (
	"context"
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
	casa           = entorno("HOME", "/home/ark")
	carpetaDatos   = entorno("TIENDA_DATOS", filepath.Join(casa, ".config", "panther", "tienda"))
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

func escribirAtomico(ruta string, datos []byte, modo os.FileMode) error {
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, datos, modo); err != nil {
		return err
	}
	if err := os.Rename(tmp, ruta); err != nil {
		_ = os.Remove(tmp)
		return os.WriteFile(ruta, datos, modo)
	}
	return nil
}

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	tarjeta := flag.String("tarjeta", "", "dibuja la tarjeta de la pantalla principal en este PNG y sale")
	reiniciar := flag.Int("reiniciar-es", 0, "espera a que acabe este proceso y reinicia EmulationStation")
	poner := flag.String("poner", "", "pone una app o un juego (claude, cceleste.zip…) sin pantalla y sale")
	quitar := flag.String("quitar", "", "quita una app o un juego sin pantalla y sale")
	lista := flag.Bool("lista", false, "dice lo que hay en la tienda y lo que está puesto, y sale")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
	}
	if *reiniciar != 0 {
		_ = os.MkdirAll(carpetaDatos, 0o700)
		if reg, err := os.OpenFile(filepath.Join(carpetaDatos, "registro.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			log.SetOutput(io.MultiWriter(reg, os.Stderr))
		}
		reiniciarES(*reiniciar)
		return
	}
	if *poner != "" || *quitar != "" || *lista {
		_ = os.MkdirAll(carpetaDatos, 0o700)
		abrirRegistro()
		if err := sinPantalla(*poner, *quitar); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	f := cargarFuentes()
	if *tarjeta != "" {
		if err := guardarTarjeta(f, *tarjeta); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
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
	abrirRegistro()
	log.Printf("Panther Store en marcha (%dx%d)", ancho, alto)

	t := nuevaTienda()
	t.tarjetaApp = func(e *elemento, ruta string) error { return tarjetaDeApp(f, e, ruta) }
	a := nuevaApp(f, t)
	a.dibujarCarga()
	pintarFB(a.l.img)
	alDia(a)
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
	a.empezar()
	a.bucle(botones, ra)
	a.t.cancelarTodo()

	a.luz.encender()
	ra.cerrar()
	a.cargaHasta = time.Now().Add(time.Hour)
	a.saliendo = a.t.cambios
	a.l.limpiarFondo()
	a.dibujarCarga()
	pintarFB(a.l.img)
	if a.t.cambios {
		programarReinicio()
	}
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// alDia: antes de abrir la pantalla, si en la fuente hay otra Panther Store, se pone y se abre esa
// (ver actualizar.go). Sin conexión, en unos segundos se sigue con esta.
func alDia(a *app) {
	t := a.t
	yo := programaPropio(t.panther)
	if yo == "" {
		return
	}
	ctx, cancel := context.WithTimeout(t.ctx, 6*time.Second)
	ind, err := leerIndice(ctx, t.fuente)
	cancel()
	if ind == nil {
		logf("al día: no se ha podido mirar la fuente (%v)", err)
		return
	}
	t.indice = ind
	ultimo := -1
	pintar := func(h, total int64) {
		pct := 0
		if total > 0 {
			pct = int(h * 100 / total)
		}
		if pct/5 == ultimo {
			return
		}
		ultimo = pct / 5
		a.alDia = fmt.Sprintf("Poniendo al día la Panther Store… %d %%", pct)
		a.l.limpiarFondo()
		a.dibujarCarga()
		pintarFB(a.l.img)
	}
	cambiada, err := ponerseAlDia(t.ctx, t.fuente, ind, yo, pintar)
	a.alDia = ""
	if err != nil {
		logf("al día: %v", err)
	}
	if cambiada {
		if err := reabrir(yo); err != nil {
			logf("al día: no se ha podido abrir la nueva: %v", err)
		}
	}
}

// abrirRegistro: registro.txt se va añadiendo y se vacía al pasar de 256 KB.
func abrirRegistro() {
	ruta := filepath.Join(carpetaDatos, "registro.txt")
	modo := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if st, err := os.Stat(ruta); err == nil && st.Size() > 256<<10 {
		modo |= os.O_TRUNC
	}
	if reg, err := os.OpenFile(ruta, modo, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
	}
}
