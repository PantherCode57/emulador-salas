// Ahorro de batería de Panther OS: un servicio pequeño (panther-ahorro.service, como root) que mira
// cada segundo qué se está haciendo en la consola y baja lo que no hace falta. Medido en la XF35H
// (con el menú abierto): normal ~630 mA; con la memoria a 194 MHz y la luz a 60, ~330 mA. El
// procesador casi no cambia nada (ya baja solo con «ondemand»), así que no se toca.
//
//   - Jugando (RetroArch, el emulador de Panther con un juego, PPSSPP, mupen64plus, drastic, un juego
//     de PortMaster o Zarpa) o viendo vídeos (YouTube, Instagram): todo como estaba (no se toca nada).
//   - En el menú o en una app (WhatsApp, Spotin, Claude…): la memoria hasta 328 MHz (como mucho).
//   - Con la pantalla apagada (SELECT en Spotin): la memoria a 194 MHz y la gráfica a 200 MHz.
//   - Si en el menú o en una app no tocas nada en 60 s, la pantalla baja a un cuarto de luz, y vuelve
//     al pulsar cualquier botón.
//
// Al pararse lo deja todo como estaba. Con -una-vez dice qué ve y qué haría, y sale.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	rutaMemoria = "/sys/class/devfreq/dmc/max_freq"
	rutaGrafica = "/sys/class/devfreq/ff400000.gpu/max_freq"
	rutaCapa    = "/dev/shm/panther-capa" // el chat de WhatsApp encima: su latido cuenta como tocar
	inactivo    = 60 * time.Second
)

type estado int

const (
	enMenu  estado = iota // EmulationStation solo
	enApp                 // una app ligera de Panther
	enVideo               // YouTube o Instagram
	enJuego
)

func (e estado) String() string {
	return [...]string{"menú", "app", "vídeo", "juego"}[e]
}

var ultimoToque atomic.Int64 // UnixNano de la última vez que se pulsó algo

func leer(ruta string) string {
	b, _ := os.ReadFile(ruta)
	return strings.TrimSpace(string(b))
}

func leerInt(ruta string) int {
	n, _ := strconv.Atoi(leer(ruta))
	return n
}

func escribir(ruta string, v int) {
	if leerInt(ruta) == v {
		return
	}
	if err := os.WriteFile(ruta, []byte(strconv.Itoa(v)), 0); err != nil {
		log.Printf("%s: %v", ruta, err)
	}
}

// queSeHace: mirando los procesos.
func queSeHace() estado {
	ds, _ := os.ReadDir("/proc")
	e := enMenu
	for _, d := range ds {
		if d.Name()[0] < '0' || d.Name()[0] > '9' {
			continue
		}
		b, err := os.ReadFile("/proc/" + d.Name() + "/cmdline")
		if err != nil || len(b) == 0 {
			continue
		}
		args := strings.Split(strings.TrimRight(string(b), "\x00"), "\x00")
		prog := filepath.Base(args[0])
		todo := strings.Join(args, " ")
		switch {
		case prog == "retroarch" || strings.HasPrefix(prog, "PPSSPP") || strings.HasPrefix(prog, "mupen64plus") ||
			prog == "drastic" || prog == "gptokeyb" || prog == "flycast" || prog == "scummvm":
			return enJuego
		case prog == "emulador" && strings.Contains(todo, "pokemulador_libretro.so"):
			// el emulador de Panther haciendo de pantalla de una app
			if strings.Contains(todo, "/panther/youtube/") || strings.Contains(todo, "/panther/instagram/") ||
				strings.Contains(todo, "/panther/zarpa/") { // los vídeos y el juego de Zarpa: a toda velocidad
				e = max(e, enVideo)
			} else {
				e = max(e, enApp)
			}
		case prog == "emulador":
			return enJuego
		}
	}
	return e
}

// escucharBotones: cualquier botón de cualquier mando o tecla cuenta como tocar (los sticks no, que
// se mueven solos un poco).
func escucharBotones() {
	var mu sync.Mutex
	vistos := map[string]bool{}
	for {
		rutas, _ := filepath.Glob("/dev/input/event*")
		for _, r := range rutas {
			mu.Lock()
			ya := vistos[r]
			mu.Unlock()
			if ya {
				continue
			}
			f, err := os.Open(r)
			if err != nil {
				continue
			}
			mu.Lock()
			vistos[r] = true
			mu.Unlock()
			go func(r string, f *os.File) {
				defer f.Close()
				ev := make([]byte, 24) // struct input_event en arm64: tiempo (16), tipo, código, valor
				for {
					if _, err := f.Read(ev); err != nil {
						mu.Lock()
						vistos[r] = false
						mu.Unlock()
						return
					}
					tipo := binary.LittleEndian.Uint16(ev[16:])
					codigo := binary.LittleEndian.Uint16(ev[18:])
					if tipo == 1 || (tipo == 3 && codigo >= 16 && codigo <= 17) { // teclas y la cruceta
						ultimoToque.Store(time.Now().UnixNano())
					}
				}
			}(r, f)
		}
		time.Sleep(10 * time.Second) // por si se conecta un mando
	}
}

type ahorro struct {
	luz        string
	memMax     int // los de antes, para dejarlos como estaban
	gpuMax     int
	atenuada   int // la luz que había antes de bajarla (0: no está bajada)
	luzBajada  int // la que se puso al bajarla
	ultimo     estado
	ultimoModo string
}

func nuevoAhorro() *ahorro {
	a := &ahorro{memMax: leerInt(rutaMemoria), gpuMax: leerInt(rutaGrafica)}
	if m, _ := filepath.Glob("/sys/class/backlight/*/brightness"); len(m) > 0 {
		a.luz = m[0]
	}
	if a.memMax == 0 {
		a.memMax = 666000000
	}
	if a.gpuMax == 0 {
		a.gpuMax = 520000000
	}
	return a
}

func (a *ahorro) restaurarLuz() {
	if a.atenuada == 0 || a.luz == "" {
		return
	}
	if ahora := leerInt(a.luz); ahora == a.luzBajada { // si la has cambiado tú mientras, se deja
		escribir(a.luz, a.atenuada)
	}
	a.atenuada = 0
}

func (a *ahorro) paso(dry bool) string {
	e := queSeHace()
	luz := leerInt(a.luz)
	apagada := a.luz != "" && luz == 0 && a.atenuada == 0
	if st, err := os.Stat(rutaCapa); err == nil && time.Since(st.ModTime()) < 3*time.Second {
		ultimoToque.Store(time.Now().UnixNano()) // el chat de WhatsApp está encima
	}
	quieto := time.Since(time.Unix(0, ultimoToque.Load()))
	modo, mem, gpu := "", a.memMax, a.gpuMax
	switch {
	case e >= enVideo:
		modo = "como estaba (" + e.String() + ")"
	case apagada:
		modo, mem, gpu = "pantalla apagada", 194000000, 200000000
	default:
		modo, mem = "ahorro ("+e.String()+")", min(a.memMax, 328000000)
	}
	if dry {
		return fmt.Sprintf("%s; memoria hasta %d MHz, gráfica hasta %d MHz; luz %d; sin tocar %v", modo, mem/1e6, gpu/1e6, luz, quieto.Round(time.Second))
	}
	escribir(rutaMemoria, mem)
	escribir(rutaGrafica, gpu)
	// la luz: se baja si no tocas nada en un rato (no jugando ni viendo vídeos)
	switch {
	case e >= enVideo || apagada || quieto < inactivo:
		a.restaurarLuz()
	case a.atenuada == 0 && luz > 20 && a.luz != "":
		a.atenuada = luz
		a.luzBajada = max(luz/4, 12)
		escribir(a.luz, a.luzBajada)
	case a.atenuada != 0 && luz != a.luzBajada: // la has subido tú: ya no está bajada
		a.atenuada = 0
	}
	if modo != a.ultimoModo {
		log.Printf("%s", modo)
		a.ultimoModo = modo
	}
	a.ultimo = e
	return modo
}

func (a *ahorro) dejarComoEstaba() {
	a.restaurarLuz()
	escribir(rutaMemoria, a.memMax)
	escribir(rutaGrafica, a.gpuMax)
}

func main() {
	unaVez := flag.Bool("una-vez", false, "dice qué ve y qué haría, sin cambiar nada, y sale")
	flag.Parse()
	a := nuevoAhorro()
	if *unaVez {
		ultimoToque.Store(time.Now().UnixNano())
		fmt.Println(a.paso(true))
		return
	}
	log.SetFlags(log.Ltime)
	log.Printf("ahorro de batería en marcha (memoria hasta %d MHz, gráfica hasta %d MHz)", a.memMax/1e6, a.gpuMax/1e6)
	ultimoToque.Store(time.Now().UnixNano())
	go escucharBotones()
	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	reloj := time.NewTicker(time.Second)
	defer reloj.Stop()
	for {
		select {
		case s := <-señal:
			log.Printf("señal %v: lo dejo todo como estaba", s)
			a.dejarComoEstaba()
			return
		case <-reloj.C:
			a.paso(false)
		}
	}
}
