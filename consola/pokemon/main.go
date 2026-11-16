// Pokémon para la consola (Panther OS: dArkOS4Clone en la XF35H): Pokemulador en la portátil.
//
// Enseña tus juegos de Game Boy, Game Boy Color y GBA (los de /roms/gb, /roms/gbc y /roms/gba, con
// los nombres en español de cada gamelist.xml y la tarjeta de su lista) y los abre con RetroArch,
// solo o en una sala de Pokemulador con tus amigos: las mismas salas que en el PC, el móvil y el
// USB. Casi todo sale del servicio del USB (lakka/servicio): las salas guardadas con su código de 6
// letras, el progreso de cada uno y cómo se abre el juego. El enlace (consola/enlace, con Tailscale
// dentro y sin clave: la primera vez se entra con tu cuenta escaneando un QR) lleva por la sala el
// cable link de Game Boy (Gambatte, puerto 56400) y la partida compartida de GBA (GBA-PK, puerto
// 4096) para veros por el mapa, combatir e intercambiar.
//
// Como Spotin y el WhatsApp de la consola, se abre desde EmulationStation; RetroArch, con el núcleo
// de la pantalla de salas (lakka/nucleo), solo enseña la imagen y nos pasa los botones. Al abrir un
// juego se cierra esa pantalla y se abre RetroArch con el juego; al salir del juego vuelve. El
// enlace sigue en marcha mientras se juega.
//
// La partida de siempre es la misma que al jugar desde las listas del menú: antes de abrir el juego
// se trae la más nueva que haya (/roms/<sistema>/, /roms/<sistema>/<núcleo>/ o
// ~/.config/retroarch/saves/…) y al cerrarlo se devuelve donde estaba, dejando antes una copia .bak.
//
// Con -prueba <carpeta> no se abre nada: dibuja las pantallas con datos inventados en PNG. Con
// -listar dice qué juegos, tarjetas y partidas encuentra (para revisar la consola por SSH).
package main

import (
	"crypto/rand"
	"encoding/json"
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

// Rutas de la consola. Todo se puede cambiar con variables de entorno (las de RetroArch, como en
// Spotin: CONSOLA_RETROARCH, CONSOLA_RETROARCH_CFG y CONSOLA_NUCLEO).
var (
	casa            = carpetaCasa()
	carpetaDatos    = entorno("POKEMON_DATOS", filepath.Join(casa, ".config", "panther", "pokemon"))
	carpetaPrograma = entorno("POKEMON_PROGRAMA", filepath.Join(casa, "panther", "pokemon"))
	carpetaRoms     = entorno("POKEMON_ROMS", "/roms")
	carpetaCores    = entorno("POKEMON_CORES", filepath.Join(casa, ".config", "retroarch", "cores"))
	carpetaSavesRA  = entorno("POKEMON_SAVES", filepath.Join(casa, ".config", "retroarch", "saves"))
	retroarchBin    = entorno("CONSOLA_RETROARCH", "/opt/retroarch/bin/retroarch")
	configRA        = entorno("CONSOLA_RETROARCH_CFG", filepath.Join(casa, ".config", "retroarch", "retroarch.cfg"))
	nucleoPantalla  = entorno("CONSOLA_NUCLEO", filepath.Join(carpetaPrograma, "pokemulador_libretro.so"))
)

func carpetaCasa() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return "/home/ark"
}

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func logf(format string, args ...any) { log.Printf(format, args...) }

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

// escribirAtomico escribe a un temporal y lo renombra, para no dejar archivos a medias si se apaga.
func escribirAtomico(ruta string, datos []byte) error {
	_ = os.MkdirAll(filepath.Dir(ruta), 0o755)
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, ruta); err != nil {
		_ = os.Remove(tmp)
		if err := os.WriteFile(ruta, datos, 0o644); err != nil {
			return err
		}
	}
	sincronizar()
	return nil
}

func guardarJSON(ruta string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	_ = escribirAtomico(ruta, b)
}

const alfabeto = "abcdefghijklmnopqrstuvwxyz0123456789"

func aleatorio(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alfabeto[int(b[i])%len(alfabeto)]
	}
	return string(b)
}

// Jugador: quién somos en las salas (el id no cambia; el nombre se pregunta una vez).
type Jugador struct {
	ID       string `json:"id"`
	Nombre   string `json:"nombre"`
	Elegido  bool   `json:"elegido"` // ya se preguntó el nombre
	Ultimo   string `json:"ultimo,omitempty"`
	nombreOK bool
}

const nombrePorDefecto = "Consola"

func archivoJugador() string { return filepath.Join(carpetaDatos, "jugador.json") }

func cargarJugador() *Jugador {
	j := &Jugador{}
	if b, err := os.ReadFile(archivoJugador()); err == nil {
		_ = json.Unmarshal(b, j)
	}
	if j.ID == "" {
		j.ID = "CONSOLA-" + strings.ToUpper(aleatorio(4))
	}
	if strings.TrimSpace(j.Nombre) == "" {
		j.Nombre = nombrePorDefecto
	}
	guardarJSON(archivoJugador(), j)
	return j
}

func (j *Jugador) guardar() { guardarJSON(archivoJugador(), j) }

func prepararRegistro() {
	ruta := filepath.Join(carpetaDatos, "registro.txt")
	if fi, err := os.Stat(ruta); err == nil && fi.Size() > 2<<20 {
		_ = os.Rename(ruta, ruta+".1")
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
}

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	listar := flag.Bool("listar", false, "dice qué juegos, tarjetas y partidas encuentra, y sale")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 320 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 240 {
		alto = v
	}
	if *listar {
		listarJuegos()
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

	if err := os.MkdirAll(carpetaDatos, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	prepararRegistro()
	logf("Pokémon de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	botones := make(chan uint16, 64)
	a.srv = nuevoServidorPantalla(botones)
	if err := a.srv.escuchar(); err != nil {
		log.Fatalf("no se pudo abrir la pantalla (¿hay otra app abierta?): %v", err)
	}
	// El logo ya mismo en la pantalla (EmulationStation la acaba de soltar) y como primer marco.
	a.dibujarCarga("")
	pintarFB(a.l.img)
	a.srv.enviar(a.l.marco())
	ra, err := abrirPantallaRA()
	if err != nil {
		log.Fatalf("no se pudo abrir RetroArch: %v", err)
	}
	a.ra = ra

	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		logf("señal %v: salgo", s)
		a.terminar()
	}()

	a.bucle(botones)
	a.cerrarTodo()
	a.luz.encender()
	a.dibujarCarga("Hasta luego")
	pintarFB(a.l.img)
	logf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// listarJuegos (pokemon -listar): lo que la app ve en la consola, sin abrir nada.
func listarJuegos() {
	js := buscarJuegos()
	fmt.Printf("%d juegos en %s\n", len(js), carpetaRoms)
	for _, j := range js {
		fmt.Printf("\n%s  [%s]  %s\n", j.Titulo, j.Sistema, j.Ruta)
		if j.Tarjeta != "" {
			fmt.Printf("  tarjeta: %s\n", j.Tarjeta)
		} else {
			fmt.Printf("  tarjeta: (no hay, se dibuja una)\n")
		}
		if p := partidaMasNueva(j); p.ruta != "" {
			fmt.Printf("  partida de siempre: %s (%d bytes, %s)\n", p.ruta, p.tam, p.cuando.Format("2006-01-02 15:04"))
		} else if d := carpetaHabitual(j); d != "" {
			fmt.Printf("  partida de siempre: ninguna todavía (se dejaría en %s)\n", d)
		} else {
			fmt.Printf("  partida de siempre: ninguna todavía\n")
		}
		n, gbapk := nucleoDe(j.Sistema)
		fmt.Printf("  núcleo: %s (existe: %v, GBA-PK: %v)\n", n, existe(n), gbapk)
	}
}
