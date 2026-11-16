// Spotin para la consola (Panther OS: dArkOS4Clone en la XF35H): la música de tu cuenta de Spotify.
//
// Es una versión pequeña de Spotin (la app de Android, un Spotui modificado): de Spotify saca tus
// listas, las canciones, las portadas, la búsqueda y las letras (con la cookie «sp_dc» de tu sesión,
// que se pasa desde el PC); el sonido lo busca en YouTube Music y lo toca con ffmpeg y aplay. Las
// canciones se pueden descargar para oírlas sin internet. SELECT apaga la pantalla y la música sigue.
//
// Como el WhatsApp de la consola, se abre desde EmulationStation y RetroArch (con el núcleo de la
// pantalla de salas, lakka/nucleo) solo enseña la imagen y nos pasa los botones.
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
package main

import (
	"flag"
	"fmt"
	"image/png"
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
	carpetaDatos   = entorno("SPOTIN_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "spotin"))
	carpetaMusica  = entorno("SPOTIN_MUSICA", "/roms/musica/spotin")
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
	probar := flag.String("probar", "", "sin pantalla: busca esta canción («artista - título») en YouTube, dice qué sonido encuentra y sale")
	bajarLista := flag.String("bajar-lista", "", "sin pantalla: busca esta lista pública de Spotify por su nombre y descarga sus canciones")
	soloDatos := flag.Bool("sin-sonido", false, "con -bajar-lista: guarda la lista y las portadas, sin descargar las canciones")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 320 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 240 {
		alto = v
	}
	if *probar != "" {
		probarYouTube(*probar)
		return
	}
	if *bajarLista != "" {
		bajarListaPublica(*bajarLista, *soloDatos)
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
	log.Printf("Spotin de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	a.alm = nuevoAlmacen(carpetaDatos, carpetaMusica)
	a.port = nuevasPortadas(a, filepath.Join(carpetaDatos, "portadas"))
	a.sp = nuevaCuentaSpotify(filepath.Join(carpetaDatos, "sesion.json"))
	a.yt = nuevoYouTube()
	a.rep = nuevoReproductor(a.cancionAcabada, a.cancionFallida)
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

// dibujarPruebas pinta cada pantalla con datos inventados, para revisar el diseño en el PC.
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	tmp, _ := os.MkdirTemp("", "spotin-prueba")
	defer os.RemoveAll(tmp)
	a := nuevaApp(f)
	a.cargaHasta = time.Time{}
	a.alm = nuevoAlmacen(tmp, filepath.Join(tmp, "musica"))
	a.port = nuevasPortadas(a, filepath.Join(tmp, "portadas"))
	a.rep = nuevoReproductor(func() {}, func(error) {})
	a.sp = nuevaCuentaSpotify(filepath.Join(tmp, "sesion.json"))
	cs := []Cancion{
		{ID: "1", Nombre: "Carretera de noche", Artistas: []string{"Los Faroles"}, Album: "Kilómetro cero", DuracionMs: 221000},
		{ID: "2", Nombre: "Kilómetro cero", Artistas: []string{"Marta Ríos"}, DuracionMs: 245000},
		{ID: "3", Nombre: "Ventanilla bajada", Artistas: []string{"Duna"}, DuracionMs: 198000},
		{ID: "4", Nombre: "Mapa sin nombres", Artistas: []string{"Alba y el Norte"}, DuracionMs: 237000},
		{ID: "5", Nombre: "Gasolinera 24 h", Artistas: []string{"Los Faroles"}, DuracionMs: 179000},
		{ID: "6", Nombre: "Última salida", Artistas: []string{"Duna"}, DuracionMs: 203000},
	}
	a.alm.guardarBiblioteca([]Lista{
		{ID: "megusta", Nombre: "Tus me gusta", Tipo: tipoMeGusta, Total: 214},
		{ID: "viaje", Nombre: "Para el viaje", Tipo: tipoLista, Total: 48, Dueno: "Tú"},
		{ID: "lofi", Nombre: "Lo-fi para estudiar", Tipo: tipoLista, Total: 120},
	})
	a.alm.guardarCanciones("viaje", cs)
	guardar := func(nombre string) error {
		a.dibujar()
		fh, err := os.Create(filepath.Join(carpeta, nombre))
		if err != nil {
			return err
		}
		defer fh.Close()
		return png.Encode(fh, a.l.img)
	}
	a.pant, a.selLista = pBiblioteca, 1
	if err := guardar("1-biblioteca.png"); err != nil {
		return err
	}
	a.abrirLista(Lista{ID: "viaje", Nombre: "Para el viaje", Tipo: tipoLista, Total: 48})
	a.cargando = false
	a.cola, a.colaI, a.colaDe, a.selCancion = cs, 0, "Para el viaje", 1
	a.buscando = true
	if err := guardar("2-lista.png"); err != nil {
		return err
	}
	a.pant = pSonando
	a.letra = []lineaLetra{{0, "Las luces van quedando atrás"}, {1, "y el mapa ya no dice dónde"},
		{60000, "la radio canta para nadie"}, {90000, "mientras la noche nos responde"}}
	a.letraDe = "1"
	if err := guardar("3-reproductor.png"); err != nil {
		return err
	}
	a.pant = pSesion
	a.sesionEstado = "Esperando la sesión…"
	if err := guardar("4-sesion.png"); err != nil {
		return err
	}
	a.pant = pBuscar
	a.tec.borrador = []rune("los far")
	a.tec.cursor = len(a.tec.borrador)
	return guardar("5-buscar.png")
}

func itoa(n int) string { return strconv.Itoa(n) }

// bajarListaPublica (spotin -bajar-lista "nombre"): sin sesión, busca la lista pública con ese
// nombre, la guarda en la biblioteca de la consola y descarga todas sus canciones. Para dejar música
// en la consola antes de pasar la sesión.
func bajarListaPublica(nombre string, soloDatos bool) {
	_ = os.MkdirAll(carpetaDatos, 0o700)
	if reg, err := os.OpenFile(filepath.Join(carpetaDatos, "bajar-lista.txt"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
	}
	alm := nuevoAlmacen(carpetaDatos, carpetaMusica)
	sp := nuevaCuentaSpotify(filepath.Join(carpetaDatos, "sesion.json"))
	sp.anonimo = true
	l, err := sp.listaPublica(nombre)
	if err != nil {
		log.Fatalf("lista: %v", err)
	}
	cs, err := sp.canciones(l)
	if err != nil {
		log.Fatalf("canciones de %q: %v", l.Nombre, err)
	}
	l.Total = len(cs)
	log.Printf("%q: %d canciones", l.Nombre, len(cs))
	ls := alm.listas()
	nueva := true
	for i := range ls {
		if ls[i].ID == l.ID {
			ls[i], nueva = l, false
		}
	}
	if nueva {
		ls = append(ls, l)
	}
	alm.guardarBiblioteca(ls)
	alm.guardarCanciones(l.ID, cs)
	yt := nuevoYouTube()
	port := &portadas{dir: filepath.Join(carpetaDatos, "portadas")}
	_ = os.MkdirAll(port.dir, 0o700)
	bajarPortada := func(u string) {
		if u == "" {
			return
		}
		ruta := port.archivo(u)
		if _, err := os.Stat(ruta); err == nil {
			return
		}
		if resp, err := clienteImagenes.Get(u); err == nil {
			if resp.StatusCode == 200 {
				if f, err := os.Create(ruta + ".tmp"); err == nil {
					_, err = io.Copy(f, resp.Body)
					f.Close()
					if err == nil {
						_ = os.Rename(ruta+".tmp", ruta)
					}
				}
			}
			resp.Body.Close()
		}
	}
	bajarPortada(l.Portada)
	bien, mal := 0, 0
	for i, c := range cs {
		bajarPortada(c.Portada)
		if soloDatos {
			continue
		}
		if _, ok := alm.descargada(c.ID); ok {
			bien++
			continue
		}
		hecho := false
		for intento := 0; intento < 2 && !hecho; intento++ {
			video := c.Video
			if video == "" || intento == 1 {
				v, err := yt.buscar(c, video)
				if err != nil {
					log.Printf("[%d/%d] %s: %v", i+1, len(cs), c.Nombre, err)
					continue
				}
				video = v
			}
			f, cli, err := yt.sonido(video)
			if err != nil {
				log.Printf("[%d/%d] %s (%s): %v", i+1, len(cs), c.Nombre, video, err)
				continue
			}
			ext := ".webm"
			if strings.Contains(f.Tipo, "mp4") {
				ext = ".m4a"
			}
			destino := filepath.Join(alm.musica, nombreSeguro(c.ID)+ext)
			if err := descargar(f, destino, nil); err != nil {
				log.Printf("[%d/%d] %s: %v", i+1, len(cs), c.Nombre, err)
				continue
			}
			c.Video = video
			alm.apuntarDescarga(c, destino)
			alm.recordarVideo(c.ID, video)
			log.Printf("[%d/%d] %s — %s (%s, %s)", i+1, len(cs), c.Nombre, c.artistas(), cli, f.Tipo)
			hecho = true
		}
		if hecho {
			bien++
		} else {
			mal++
		}
	}
	log.Printf("terminado: %d descargadas, %d sin descargar", bien, mal)
}
