package main

import (
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

// supervisor: siempre hay un RetroArch abierto, el de la pantalla de salas o el de un juego.
type supervisor struct {
	pedidos chan orden
	alAcabar func(err error, duracion time.Duration)
}

func nuevoSupervisor(alAcabar func(error, time.Duration)) *supervisor {
	return &supervisor{pedidos: make(chan orden, 1), alAcabar: alAcabar}
}

func ordenPantalla() orden {
	return orden{args: []string{"-L", filepath.Join(carpetaCores, "pokemulador_libretro.so"),
		"--appendconfig", archivoConfigPantalla()}}
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
	sig := ordenPantalla()
	fallosSeguidos := 0
	for {
		inicio := time.Now()
		cmd := exec.Command(retroarchBin, sig.args...)
		cmd.Env = append(os.Environ(), sig.env...)
		salida, errSalida := os.OpenFile(filepath.Join(carpetaDatos, "retroarch.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if errSalida == nil {
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
			if fallosSeguidos > 3 {
				time.Sleep(10 * time.Second)
			} else {
				time.Sleep(500 * time.Millisecond)
			}
		case o := <-s.pedidos:
			terminar(cmd, hecho)
			if sig.juego {
				s.alAcabar(nil, time.Since(inicio))
			}
			sig = o
			time.Sleep(300 * time.Millisecond)
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
// salas: solo los comandos de red (para abrir el menú de Lakka desde nuestra pantalla).
func archivoConfigPantalla() string {
	ruta := filepath.Join(carpetaDatos, "pantalla.cfg")
	escribirAtomico(ruta, []byte("network_cmd_enable = \"true\"\nnetwork_cmd_port = \"55355\"\npause_nonactive = \"false\"\n"))
	return ruta
}
