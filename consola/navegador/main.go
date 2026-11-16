// Navegador para la consola (Panther OS: dArkOS4Clone en la XF35H): un lector de páginas web.
//
// Un navegador de verdad no cabe en 1 GB de memoria ni en este procesador, así que este hace lo
// que sí se puede: buscar (DuckDuckGo, o Bing si no contesta), abrir páginas y enseñar lo que se
// lee de ellas (títulos, texto, listas e imágenes) con sus enlaces, sin JavaScript, anuncios ni
// menús. Como el WhatsApp, Spotin y YouTube de la consola, RetroArch (con el núcleo de la pantalla
// de salas) enseña la imagen y nos pasa los botones.
//
// Con -prueba <carpeta> dibuja las pantallas de ejemplo en PNG. Con -probar <dirección o búsqueda>
// enseña en texto lo que saldría (sin pantalla).
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
	"strings"
	"syscall"
	"time"
)

var (
	carpetaDatos   = entorno("NAVEGADOR_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "navegador"))
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
	probar := flag.String("probar", "", "sin pantalla: abre esta dirección (o la busca) y enseña lo que saldría")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
	}
	f := cargarFuentes()
	fuentesActuales = f
	if *probar != "" {
		probarWeb(f, *probar)
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
	log.Printf("Navegador de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	a.alm = nuevoAlmacen(carpetaDatos)
	a.img = nuevasImagenes(a)
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

// probarWeb (navegador -probar …): lo que saldría en la pantalla, en texto.
func probarWeb(f *fuentes, q string) {
	dir, ok := pareceDireccion(q)
	if !ok {
		t0 := time.Now()
		res, err := buscarWeb(q)
		fmt.Printf("búsqueda en %v: %d resultados (%v)\n", time.Since(t0).Round(time.Millisecond), len(res), err)
		for i, r := range res {
			if i < 6 {
				fmt.Printf("  %-60.60s  %s\n      %.100s\n", r.Titulo, r.URL, r.Resumen)
			}
		}
		if len(res) == 0 {
			return
		}
		dir = res[0].URL
	}
	t0 := time.Now()
	p, err := abrirPagina(dir)
	if err != nil {
		fmt.Println("no se pudo abrir:", err)
		return
	}
	m := maquetar(p, f, func(string) (int, int) { return 0, 0 })
	imgs := 0
	for _, b := range p.bloques {
		if b.tipo == bImagen {
			imgs++
		}
	}
	fmt.Printf("\n%s  (%s) en %v\n%d bloques, %d imágenes, %d enlaces, %d líneas, %d px de alto\n\n", p.titulo, p.url,
		time.Since(t0).Round(time.Millisecond), len(p.bloques), imgs, len(p.enlaces), len(m.lineas), m.alto)
	n := 0
	for _, b := range p.bloques {
		if n >= 25 {
			break
		}
		var s strings.Builder
		for _, t := range b.trozos {
			if t.enlace >= 0 {
				s.WriteString("[" + t.texto + "]")
			} else {
				s.WriteString(t.texto)
			}
		}
		marca := map[tipoBloque]string{bParrafo: "  ", bTitulo1: "# ", bTitulo2: "## ", bTitulo3: "### ", bItem: "  • ",
			bCita: "  > ", bCodigo: "  | ", bImagen: "  [imagen] "}[b.tipo]
		if b.tipo == bImagen {
			s.WriteString(b.img)
		}
		fmt.Printf("%s%.160s\n", marca, s.String())
		n++
	}
}
