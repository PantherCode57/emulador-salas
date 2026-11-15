// Servicio de Pokemulador para el USB (Lakka). Sustituye a RetroArch como programa principal del
// sistema: arranca RetroArch con el núcleo "Pokemulador" (la pantalla de salas, que dibuja este
// servicio y el núcleo solo enseña), y cuando eliges un juego cierra esa pantalla y abre el juego
// con el emulador que toca. Al cerrar el juego vuelve a la pantalla de salas.
//
// Las salas son las mismas que en el PC y en el móvil: el enlace (pc/enlace, Tailscale dentro del
// programa) reparte los papeles, pasa el progreso de cada uno y lleva el tráfico de los juegos
// (GBA-PK para veros por el mapa en GBA, cable link de Game Boy y salas de Azahar en 3DS).
package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// La pone el flujo de GitHub al compilar (secreto POKEMULADOR_TS_KEY).
var claveTailscale = ""

// Rutas del sistema (Lakka) y de nuestros datos. Todo se puede cambiar con variables de entorno,
// para usarlo en otro Linux (por ejemplo, una consola portátil con RetroArch):
//
//	POKEMULADOR_PORT=1          se abre como un "port" (desde EmulationStation): sale una opción
//	                            "Salir" que lo cierra todo y termina con código 0.
//	POKEMULADOR_JUEGOS          carpeta de juegos (si no, la parte JUEGOS de la memoria USB).
//	POKEMULADOR_SISTEMAS        sistemas que se enseñan, p. ej. "gb,gbc,gba" (por defecto todos).
//	POKEMULADOR_NUCLEO_<SIS>    otro nombre de núcleo para un sistema (POKEMULADOR_NUCLEO_GBA=...).
//	POKEMULADOR_RETROARCH_CFG   configuración de RetroArch que se le pasa con --config.
//	POKEMULADOR_APAGAR          orden para apagar ("-" para quitar la opción; en modo port no hay).
//	POKEMULADOR_ANCHO / _ALTO   tamaño de la pantalla de salas (por defecto 960×540).
var (
	carpetaDatos  = entorno("POKEMULADOR_DATOS", "/storage/pokemulador")
	retroarchBin  = entorno("POKEMULADOR_RETROARCH", "/usr/bin/retroarch")
	carpetaCores  = entorno("POKEMULADOR_CORES", "/tmp/cores")
	carpetaPropia = entorno("POKEMULADOR_LIB", "/usr/lib/pokemulador")
	modoPort      = os.Getenv("POKEMULADOR_PORT") == "1"
	configRA      = os.Getenv("POKEMULADOR_RETROARCH_CFG")
	ordenApagar   = ordenDeApagado()
)

func ordenDeApagado() string {
	v, hay := os.LookupEnv("POKEMULADOR_APAGAR")
	switch {
	case hay && (v == "-" || v == ""):
		return ""
	case hay:
		return v
	case modoPort:
		return ""
	default:
		return "systemctl poweroff"
	}
}

// argsRetroArch añade la configuración propia de RetroArch si la hay.
func argsRetroArch(args ...string) []string {
	if configRA != "" {
		return append([]string{"--config", configRA}, args...)
	}
	return args
}

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

// Jugador: quién somos en las salas (el id no cambia; el nombre se puede cambiar).
type Jugador struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

func archivoJugador() string { return filepath.Join(carpetaDatos, "jugador.json") }

func cargarJugador() *Jugador {
	j := &Jugador{}
	if b, err := os.ReadFile(archivoJugador()); err == nil {
		_ = json.Unmarshal(b, j)
	}
	if j.ID == "" {
		j.ID = "USB-" + strings.ToUpper(aleatorio(4))
	}
	if strings.TrimSpace(j.Nombre) == "" {
		j.Nombre = "Jugador USB"
	}
	guardarJSON(archivoJugador(), j)
	return j
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

// guardarJSON escribe a un temporal y lo renombra, para no dejar archivos a medias si se va la luz.
func guardarJSON(ruta string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	escribirAtomico(ruta, b)
}

func escribirAtomico(ruta string, datos []byte) {
	_ = os.MkdirAll(filepath.Dir(ruta), 0o755)
	tmp := ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o644) != nil {
		return
	}
	if os.Rename(tmp, ruta) != nil {
		_ = os.WriteFile(ruta, datos, 0o644)
		_ = os.Remove(tmp)
	}
	sincronizar()
}

func sincronizar() { syscall.Sync() }

func prepararRegistro() {
	_ = os.MkdirAll(carpetaDatos, 0o755)
	ruta := filepath.Join(carpetaDatos, "registro.txt")
	if fi, err := os.Stat(ruta); err == nil && fi.Size() > 2<<20 {
		_ = os.Rename(ruta, ruta+".1")
	}
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	log.SetFlags(log.LstdFlags)
}

func main() {
	prepararRegistro()
	log.Printf("Pokemulador USB en marcha (pantalla %dx%d, port %v)", ancho, alto, modoPort)
	diagnostico()

	a := nuevaApp()
	a.sup = nuevoSupervisor(a.finJuego)
	a.srv = nuevoServidorPantalla(a.botones)
	if err := a.srv.escuchar(); err != nil {
		log.Printf("no se pudo abrir la pantalla: %v", err)
	}

	salida := make(chan os.Signal, 1)
	signal.Notify(salida, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-salida
		log.Printf("señal %v: cerrando", s)
		a.cerrarTodo()
		a.sup.cerrar() // que RetroArch guarde la partida antes de apagar
		os.Exit(0)
	}()

	go a.sup.bucle()
	a.bucle()
}

func fallo(format string, args ...any) error { return fmt.Errorf(format, args...) }

// diagnostico deja en el registro con qué gráficos cuenta el aparato (para saber por qué no
// arranca RetroArch si pasa).
func diagnostico() {
	if d, err := os.ReadDir("/dev/dri"); err == nil {
		var nombres []string
		for _, e := range d {
			nombres = append(nombres, e.Name())
		}
		log.Printf("/dev/dri: %v", nombres)
	} else {
		log.Printf("/dev/dri: %v", err)
	}
	if b, err := os.ReadFile("/proc/fb"); err == nil {
		log.Printf("/proc/fb: %q", strings.TrimSpace(string(b)))
	}
	if out, err := exec.Command("dmesg").Output(); err == nil {
		n := 0
		for _, l := range strings.Split(string(out), "\n") {
			if b := strings.ToLower(l); (strings.Contains(b, "drm") || strings.Contains(b, "fb0") || strings.Contains(b, "gpu")) && n < 25 {
				log.Printf("dmesg: %s", strings.TrimSpace(l))
				n++
			}
		}
	}
}
