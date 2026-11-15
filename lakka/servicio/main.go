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
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// La pone el flujo de GitHub al compilar (secreto POKEMULADOR_TS_KEY).
var claveTailscale = ""

// Rutas del sistema (Lakka) y de nuestros datos. Se pueden cambiar para probar en otro Linux.
var (
	carpetaDatos  = entorno("POKEMULADOR_DATOS", "/storage/pokemulador")
	retroarchBin  = entorno("POKEMULADOR_RETROARCH", "/usr/bin/retroarch")
	carpetaCores  = entorno("POKEMULADOR_CORES", "/tmp/cores")
	carpetaPropia = entorno("POKEMULADOR_LIB", "/usr/lib/pokemulador")
)

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
	log.Printf("Pokemulador USB en marcha")

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
		os.Exit(0)
	}()

	go a.sup.bucle()
	a.bucle()
}

func fallo(format string, args ...any) error { return fmt.Errorf(format, args...) }
