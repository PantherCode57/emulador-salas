package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// orden: qué RetroArch hay que tener abierto. Sin juego es la pantalla de salas.
type orden struct {
	args  []string
	env   []string
	juego bool
}

// supervisor: siempre hay un RetroArch abierto, el de la pantalla de salas o el de un juego,
// hasta que se pide cerrar (opción "Salir" en modo port).
type supervisor struct {
	pedidos  chan orden
	alAcabar func(err error, duracion time.Duration)
	parar    chan struct{}
	parado   chan struct{}
}

func nuevoSupervisor(alAcabar func(error, time.Duration)) *supervisor {
	return &supervisor{pedidos: make(chan orden, 1), alAcabar: alAcabar,
		parar: make(chan struct{}), parado: make(chan struct{})}
}

func ordenPantalla() orden {
	return orden{
		args: argsRetroArch("-L", filepath.Join(carpetaCores, "pokemulador_libretro.so"),
			"--appendconfig", archivoConfigPantalla()),
		env: []string{fmt.Sprintf("POKEMULADOR_ANCHO=%d", ancho), fmt.Sprintf("POKEMULADOR_ALTO=%d", alto)},
	}
}

// cerrar cierra el RetroArch que haya abierto y para el supervisor (espera como mucho 12 s).
func (s *supervisor) cerrar() {
	select {
	case <-s.parar:
	default:
		close(s.parar)
	}
	select {
	case <-s.parado:
	case <-time.After(12 * time.Second):
	}
}

// jugar cierra lo que haya abierto y abre el juego.
func (s *supervisor) jugar(o orden) {
	o.juego = true
	select {
	case <-s.pedidos:
	default:
	}
	s.pedidos <- o
}

func (s *supervisor) bucle() {
	defer close(s.parado)
	sig := ordenPantalla()
	fallosSeguidos := 0
	for {
		select {
		case <-s.parar:
			return
		default:
		}
		inicio := time.Now()
		cmd := exec.Command(retroarchBin, append([]string{"--verbose"}, sig.args...)...)
		cmd.Env = append(os.Environ(), sig.env...)
		// Registro de RetroArch: se va añadiendo y se vacía al pasar de 1 MB.
		registroRA := filepath.Join(carpetaDatos, "retroarch.txt")
		modo := os.O_CREATE | os.O_WRONLY | os.O_APPEND
		if fi, err := os.Stat(registroRA); err == nil && fi.Size() > 1<<20 {
			modo |= os.O_TRUNC
		}
		salida, errSalida := os.OpenFile(registroRA, modo, 0o644)
		if errSalida == nil {
			fmt.Fprintf(salida, "\n===== %s %v\n", time.Now().Format(time.RFC3339), sig.args)
			cmd.Stdout, cmd.Stderr = salida, salida
		}
		cerrarSalida := func() {
			if errSalida == nil {
				_ = salida.Close()
			}
		}
		log.Printf("abriendo RetroArch %v", sig.args)
		if err := cmd.Start(); err != nil {
			cerrarSalida()
			log.Printf("no se pudo abrir RetroArch: %v", err)
			if sig.juego {
				s.alAcabar(err, 0)
			}
			sig = ordenPantalla()
			time.Sleep(3 * time.Second)
			continue
		}
		hecho := make(chan error, 1)
		go func() {
			err := cmd.Wait()
			cerrarSalida()
			hecho <- err
		}()

		select {
		case err := <-hecho:
			duro := time.Since(inicio)
			log.Printf("RetroArch se cerró (%v) tras %v", err, duro.Round(time.Second))
			if sig.juego {
				s.alAcabar(err, duro)
				fallosSeguidos = 0
			} else if duro < 5*time.Second {
				fallosSeguidos++
			} else {
				fallosSeguidos = 0
			}
			// Si hay un juego pedido mientras tanto, va primero.
			select {
			case o := <-s.pedidos:
				sig = o
				continue
			default:
			}
			sig = ordenPantalla()
			switch {
			case fallosSeguidos == 4:
				// La pantalla de salas no arranca: al menos, el menú normal de RetroArch/Lakka.
				log.Printf("la pantalla de salas no arranca: abro RetroArch sin ella")
				sig = orden{args: argsRetroArch()}
			case fallosSeguidos > 4:
				time.Sleep(10 * time.Second)
			default:
				time.Sleep(500 * time.Millisecond)
			}
		case o := <-s.pedidos:
			terminar(cmd, hecho)
			if sig.juego {
				s.alAcabar(nil, time.Since(inicio))
			}
			sig = o
			time.Sleep(300 * time.Millisecond)
		case <-s.parar:
			log.Printf("cerrando RetroArch para salir")
			terminar(cmd, hecho)
			return
		}
	}
}

// terminar cierra RetroArch con calma (guarda la partida y la configuración) y, si no responde,
// a la fuerza.
func terminar(cmd *exec.Cmd, hecho chan error) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-hecho:
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		<-hecho
	}
}

// comandoRetroArch manda un comando de red a RetroArch (MENU_TOGGLE, SCREENSHOT...).
func comandoRetroArch(c string) {
	conn, err := net.Dial("udp", "127.0.0.1:55355")
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(c))
}

// Ajustes que se añaden a la configuración de RetroArch mientras está abierta la pantalla de
// salas: los comandos de red (para abrir el menú de Lakka desde nuestra pantalla) y que "Cerrar
// contenido" cierre RetroArch, para que el servicio vuelva a abrir la pantalla de salas.
func archivoConfigPantalla() string {
	ruta := filepath.Join(carpetaDatos, "pantalla.cfg")
	escribirAtomico(ruta, []byte("network_cmd_enable = \"true\"\nnetwork_cmd_port = \"55355\"\npause_nonactive = \"false\"\nquit_on_close_content = \"1\"\n"))
	return ruta
}
