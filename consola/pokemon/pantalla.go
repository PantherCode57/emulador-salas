package main

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Conversación con el núcleo «Pokemulador» (lakka/nucleo) por 127.0.0.1:24890, la misma que la
// pantalla de salas:
//
//	núcleo -> nosotros: 'I' + 2 bytes (botones de RetroPad, little endian)
//	nosotros -> núcleo: 'F' + ancho (2 bytes) + alto (2 bytes) + píxeles XRGB8888
const direccionPantalla = "127.0.0.1:24890"

// Bits de los botones de RetroPad.
const (
	bB = 1 << iota
	bY
	bSelect
	bStart
	bArriba
	bAbajo
	bIzquierda
	bDerecha
	bA
	bX
	bL
	bR
)

type servidorPantalla struct {
	botones chan<- uint16
	mu      sync.Mutex
	ultimo  []byte
	version int
	nuevo   *sync.Cond
}

func nuevoServidorPantalla(botones chan<- uint16) *servidorPantalla {
	s := &servidorPantalla{botones: botones}
	s.nuevo = sync.NewCond(&s.mu)
	return s
}

func (s *servidorPantalla) escuchar() error {
	ln, err := net.Listen("tcp", direccionPantalla)
	if err != nil {
		return err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				log.Printf("pantalla: %v", err)
				return
			}
			go s.atender(c)
		}
	}()
	return nil
}

// enviar deja un marco nuevo para el núcleo.
func (s *servidorPantalla) enviar(pixeles []byte) {
	cab := make([]byte, 5, 5+len(pixeles))
	cab[0] = 'F'
	binary.LittleEndian.PutUint16(cab[1:], uint16(ancho))
	binary.LittleEndian.PutUint16(cab[3:], uint16(alto))
	s.mu.Lock()
	s.ultimo = append(cab, pixeles...)
	s.version++
	s.nuevo.Broadcast()
	s.mu.Unlock()
}

func (s *servidorPantalla) atender(c net.Conn) {
	defer c.Close()
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	cerrado := make(chan struct{})
	go func() { // botones
		defer close(cerrado)
		m := make([]byte, 3)
		for {
			if _, err := io.ReadFull(c, m); err != nil {
				return
			}
			if m[0] == 'I' {
				s.botones <- binary.LittleEndian.Uint16(m[1:])
			}
		}
	}()
	go func() { // despierta al escritor cuando se cierra la conexión
		<-cerrado
		s.mu.Lock()
		s.nuevo.Broadcast()
		s.mu.Unlock()
	}()
	visto := -1
	for {
		s.mu.Lock()
		for s.version == visto {
			select {
			case <-cerrado:
				s.mu.Unlock()
				return
			default:
			}
			s.nuevo.Wait()
		}
		datos, v := s.ultimo, s.version
		s.mu.Unlock()
		if _, err := c.Write(datos); err != nil {
			return
		}
		visto = v
	}
}

// retroarch: un RetroArch abierto, el de la pantalla (el núcleo que enseña nuestros marcos) o el de
// un juego. «terminado» se cierra cuando acaba, y entonces «err» dice cómo.
type retroarch struct {
	cmd       *exec.Cmd
	juego     bool
	inicio    time.Time
	terminado chan struct{}
	err       error
}

// abrirRetroArch lanza RetroArch con la configuración de la consola y lo que se le añada (--appendconfig
// y el núcleo), y deja lo que diga en el registro indicado.
func abrirRetroArch(args, env []string, registro string, juego bool) (*retroarch, error) {
	if configRA != "" {
		if _, err := os.Stat(configRA); err == nil {
			args = append([]string{"--config", configRA}, args...)
		}
	}
	cmd := exec.Command(retroarchBin, args...)
	cmd.Env = append(os.Environ(), env...)
	grupoPropio(cmd)
	var salida *os.File
	if f, err := os.Create(filepath.Join(carpetaDatos, registro)); err == nil {
		salida = f
		cmd.Stdout, cmd.Stderr = f, f
	}
	logf("abriendo RetroArch %v", args)
	if err := cmd.Start(); err != nil {
		if salida != nil {
			salida.Close()
		}
		return nil, err
	}
	r := &retroarch{cmd: cmd, juego: juego, inicio: time.Now(), terminado: make(chan struct{})}
	go func() {
		r.err = cmd.Wait()
		if salida != nil {
			salida.Close()
		}
		close(r.terminado)
	}()
	return r, nil
}

// abrirPantalla: RetroArch con el núcleo de la pantalla, sin sonido (el de los juegos sí lo tiene).
func abrirPantallaRA() (*retroarch, error) {
	cfg := filepath.Join(carpetaDatos, "pantalla.cfg")
	escribirAtomico(cfg, []byte("pause_nonactive = \"false\"\nvideo_font_enable = \"false\"\n"+
		"config_save_on_exit = \"false\"\nmenu_show_load_content_animation = \"false\"\n"+
		"audio_enable = \"false\"\naudio_mute_enable = \"true\"\n"))
	return abrirRetroArch([]string{"-L", nucleoPantalla, "--appendconfig", cfg},
		[]string{"POKEMULADOR_ANCHO=" + strconv.Itoa(ancho), "POKEMULADOR_ALTO=" + strconv.Itoa(alto)}, "retroarch.txt", false)
}

// cerrar cierra RetroArch con calma (guarda la partida) y, si no responde, a la fuerza.
func (r *retroarch) cerrar() {
	if r == nil || r.cmd.Process == nil {
		return
	}
	select {
	case <-r.terminado:
		return
	default:
	}
	señalGrupo(r.cmd, syscall.SIGTERM)
	select {
	case <-r.terminado:
	case <-time.After(8 * time.Second):
		señalGrupo(r.cmd, syscall.SIGKILL)
		<-r.terminado
	}
	// Por si quedaba algo del grupo (un script que lanzó RetroArch aparte), otra vez.
	señalGrupo(r.cmd, syscall.SIGKILL)
}

// comandoRetroArch manda un comando de red al RetroArch del juego (SCREENSHOT...).
func comandoRetroArch(c string) {
	conn, err := net.Dial("udp", "127.0.0.1:55355")
	if err != nil {
		return
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(c))
}
