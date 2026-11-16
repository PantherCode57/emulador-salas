package main

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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

// servidorPantalla guarda el último marco y se lo manda al núcleo. Los marcos van en tres memorias
// que se reutilizan (los vídeos mandan 30 marcos por segundo de 1,2 MB): una es la que se está
// mandando, otra la última que se dejó y en la tercera se pinta la siguiente.
type servidorPantalla struct {
	botones chan<- uint16
	mu      sync.Mutex
	bufs    [3][]byte
	actual  int    // la última que se dejó (-1: ninguna)
	enUso   [3]int // conexiones mandándola
	version int
	nuevo   *sync.Cond
	prod    sync.Mutex // un solo pintor a la vez
}

func nuevoServidorPantalla(botones chan<- uint16) *servidorPantalla {
	s := &servidorPantalla{botones: botones, actual: -1}
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

// enviarCon deja un marco nuevo: llenar pinta los píxeles (XRGB8888) en la memoria que se le da.
func (s *servidorPantalla) enviarCon(llenar func(px []byte)) {
	s.prod.Lock()
	defer s.prod.Unlock()
	s.mu.Lock()
	i := 0
	for ; i < 3; i++ {
		if i != s.actual && s.enUso[i] == 0 {
			break
		}
	}
	if i == 3 { // dos conexiones a la vez (no debería pasar): se pinta encima de una
		i = (s.actual + 1) % 3
	}
	n := 5 + ancho*alto*4
	if len(s.bufs[i]) != n {
		s.bufs[i] = make([]byte, n)
	}
	b := s.bufs[i]
	s.mu.Unlock()
	b[0] = 'F'
	binary.LittleEndian.PutUint16(b[1:], uint16(ancho))
	binary.LittleEndian.PutUint16(b[3:], uint16(alto))
	llenar(b[5:])
	s.mu.Lock()
	s.actual = i
	s.version++
	s.nuevo.Broadcast()
	s.mu.Unlock()
}

// enviar deja un marco ya hecho.
func (s *servidorPantalla) enviar(pixeles []byte) {
	s.enviarCon(func(px []byte) { copy(px, pixeles) })
}

func (s *servidorPantalla) atender(c net.Conn) {
	defer c.Close()
	if tc, ok := c.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetWriteBuffer(4 << 20)
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
	visto := 0
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
		i, v := s.actual, s.version
		datos := s.bufs[i]
		s.enUso[i]++
		s.mu.Unlock()
		_, err := c.Write(datos)
		s.mu.Lock()
		s.enUso[i]--
		s.mu.Unlock()
		if err != nil {
			return
		}
		visto = v
	}
}

// retroarch abre RetroArch con el núcleo de la pantalla y avisa cuando se cierra.
type retroarch struct {
	cmd   *exec.Cmd
	hecho chan error
}

func abrirRetroArch() (*retroarch, error) {
	cfg := filepath.Join(carpetaDatos, "pantalla.cfg")
	// RetroArch sin sonido: el sonido de los vídeos lo suena este programa (ffmpeg y aplay).
	_ = os.WriteFile(cfg, []byte("pause_nonactive = \"false\"\nvideo_font_enable = \"false\"\n"+
		"config_save_on_exit = \"false\"\nmenu_show_load_content_animation = \"false\"\n"+
		"audio_enable = \"false\"\naudio_mute_enable = \"true\"\n"), 0o644)
	args := []string{"-L", nucleoPantalla, "--appendconfig", cfg}
	if c := os.Getenv("CONSOLA_RETROARCH_CFG"); c != "" {
		args = append([]string{"--config", c}, args...)
	}
	cmd := exec.Command(retroarchBin, args...)
	cmd.Env = append(os.Environ(), "POKEMULADOR_ANCHO="+itoa(ancho), "POKEMULADOR_ALTO="+itoa(alto))
	grupoPropio(cmd)
	if f, err := os.Create(filepath.Join(carpetaDatos, "retroarch.txt")); err == nil {
		cmd.Stdout, cmd.Stderr = f, f
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	r := &retroarch{cmd: cmd, hecho: make(chan error, 1)}
	go func() { r.hecho <- cmd.Wait() }()
	return r, nil
}

func (r *retroarch) cerrar() {
	if r == nil || r.cmd.Process == nil {
		return
	}
	señalGrupo(r.cmd, syscall.SIGTERM)
	select {
	case <-r.hecho:
	case <-time.After(6 * time.Second):
		señalGrupo(r.cmd, syscall.SIGKILL)
	}
	// Por si quedaba algo del grupo (un script que lanzó RetroArch aparte), otra vez.
	señalGrupo(r.cmd, syscall.SIGKILL)
}
