package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Sonido: ffmpeg convierte la canción (de internet o descargada) en sonido sin comprimir y aplay lo
// manda a la tarjeta de sonido (por el «default» de ALSA, que en la consola mezcla con dmix). aplay
// se queda abierto entre canciones; ffmpeg se abre una vez por canción (y otra vez al adelantar).
const (
	frecuencia    = 44100
	bytesPorSeg   = frecuencia * 2 * 2 // estéreo, 16 bits
	trozoLectura  = 4096
	ffmpegBin     = "ffmpeg"
	aplayBin      = "aplay"
	esperaReinten = 2 * time.Second
)

// fuenteSonido: de dónde sale la canción. Ruta es un archivo o una dirección; Cabeceras, las que hay
// que mandar al pedirla (User-Agent del cliente de YouTube).
type fuenteSonido struct {
	Ruta      string
	Cabeceras map[string]string
	Duracion  time.Duration
	Tipo      string // mimeType de YouTube (audio/webm; codecs="opus", audio/mp4…)
	Tamano    int64  // bytes, si se sabe
}

type reproductor struct {
	mu       sync.Mutex
	cond     *sync.Cond
	aplay    *exec.Cmd
	salida   io.WriteCloser
	ffmpeg   *exec.Cmd
	fuente   *fuenteSonido
	inicio   time.Duration // dónde empezó el ffmpeg de ahora
	escritos int64         // bytes de sonido mandados desde entonces
	pausado  bool
	turno    int // sube con cada canción o salto: el hilo de lectura viejo se calla

	// Avisos al bucle de la app (desde otro hilo).
	alAcabar func()
	alFallar func(error)
}

func nuevoReproductor(alAcabar func(), alFallar func(error)) *reproductor {
	r := &reproductor{alAcabar: alAcabar, alFallar: alFallar}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *reproductor) abrirAplay() error {
	if r.aplay != nil {
		return nil
	}
	cmd := exec.Command(aplayBin, "-q", "-t", "raw", "-f", "S16_LE", "-r", fmt.Sprint(frecuencia), "-c", "2",
		"--buffer-time=300000")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	r.aplay, r.salida = cmd, in
	go func() { _ = cmd.Wait() }()
	return nil
}

func (r *reproductor) cerrarAplay() {
	if r.salida != nil {
		_ = r.salida.Close()
	}
	if r.aplay != nil && r.aplay.Process != nil {
		_ = r.aplay.Process.Kill()
	}
	r.aplay, r.salida = nil, nil
}

// tocar empieza una canción desde «desde».
func (r *reproductor) tocar(f *fuenteSonido, desde time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.empezar(f, desde)
}

func (r *reproductor) empezar(f *fuenteSonido, desde time.Duration) error {
	r.pararFfmpeg()
	if err := r.abrirAplay(); err != nil {
		return err
	}
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	if strings.HasPrefix(f.Ruta, "http") {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_on_network_error", "1",
			"-reconnect_delay_max", "8", "-rw_timeout", "15000000")
		if len(f.Cabeceras) > 0 {
			var b strings.Builder
			for k, v := range f.Cabeceras {
				if strings.EqualFold(k, "User-Agent") {
					args = append(args, "-user_agent", v)
					continue
				}
				fmt.Fprintf(&b, "%s: %s\r\n", k, v)
			}
			if b.Len() > 0 {
				args = append(args, "-headers", b.String())
			}
		}
	}
	if desde > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.2f", desde.Seconds()))
	}
	args = append(args, "-i", f.Ruta, "-vn", "-f", "s16le", "-ar", fmt.Sprint(frecuencia), "-ac", "2", "pipe:1")
	cmd := exec.Command(ffmpegBin, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	errOut, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	r.ffmpeg, r.fuente, r.inicio, r.escritos, r.pausado = cmd, f, desde, 0, false
	r.turno++
	turno := r.turno
	var ultimoError string
	go func() { // lo que diga ffmpeg, por si falla
		s := bufio.NewScanner(errOut)
		for s.Scan() {
			ultimoError = s.Text()
			logf("ffmpeg: %s", ultimoError)
		}
	}()
	go r.copiar(cmd, out, turno, &ultimoError)
	return nil
}

// copiar pasa el sonido de ffmpeg a aplay (esperando mientras está en pausa).
func (r *reproductor) copiar(cmd *exec.Cmd, out io.Reader, turno int, ultimoError *string) {
	buf := make([]byte, trozoLectura)
	total := int64(0)
	for {
		n, err := out.Read(buf)
		if n > 0 {
			r.mu.Lock()
			for r.pausado && r.turno == turno {
				r.cond.Wait()
			}
			if r.turno != turno {
				r.mu.Unlock()
				return
			}
			if r.salida == nil { // aplay se cerró (por ejemplo, otro programa cogió el sonido): otra vez
				if err := r.abrirAplay(); err != nil {
					logf("aplay: %v", err)
				}
			}
			salida := r.salida
			r.mu.Unlock()
			if salida != nil {
				if _, werr := salida.Write(buf[:n]); werr != nil {
					logf("aplay: %v", werr)
					r.mu.Lock()
					r.cerrarAplay()
					r.mu.Unlock()
				}
			}
			r.mu.Lock()
			if r.turno == turno {
				r.escritos += int64(n)
			}
			r.mu.Unlock()
			total += int64(n)
		}
		if err != nil {
			werr := cmd.Wait()
			r.mu.Lock()
			mio := r.turno == turno
			if mio {
				r.ffmpeg = nil
			}
			r.mu.Unlock()
			if !mio {
				return
			}
			if total < bytesPorSeg/2 && (werr != nil || *ultimoError != "") {
				// casi no ha sonado nada: no es el final, es que no se pudo abrir
				msg := *ultimoError
				if msg == "" && werr != nil {
					msg = werr.Error()
				}
				r.alFallar(errors.New(msg))
				return
			}
			r.alAcabar()
			return
		}
	}
}

func (r *reproductor) pararFfmpeg() {
	r.turno++
	r.cond.Broadcast()
	if r.ffmpeg != nil && r.ffmpeg.Process != nil {
		_ = r.ffmpeg.Process.Signal(syscall.SIGKILL)
	}
	r.ffmpeg = nil
}

// parar: silencio y fuera la canción.
func (r *reproductor) parar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pararFfmpeg()
	r.fuente = nil
	r.pausado = false
}

func (r *reproductor) cerrar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pararFfmpeg()
	r.cerrarAplay()
}

func (r *reproductor) pausar(p bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil {
		return
	}
	r.pausado = p
	r.cond.Broadcast()
}

func (r *reproductor) enPausa() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pausado
}

func (r *reproductor) sonando() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fuente != nil
}

// posicion: por dónde va (lo ya mandado a aplay, menos lo que aún tiene en su memoria).
func (r *reproductor) posicion() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil {
		return 0
	}
	p := r.inicio + time.Duration(r.escritos)*time.Second/bytesPorSeg - 250*time.Millisecond
	return max(p, r.inicio)
}

// saltar: adelanta o atrasa (un ffmpeg nuevo desde ahí).
func (r *reproductor) saltar(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil {
		return
	}
	pos := r.inicio + time.Duration(r.escritos)*time.Second/bytesPorSeg + d
	if pos < 0 {
		pos = 0
	}
	if r.fuente.Duracion > 0 && pos > r.fuente.Duracion-2*time.Second {
		pos = r.fuente.Duracion - 2*time.Second
	}
	pausado := r.pausado
	if err := r.empezar(r.fuente, pos); err != nil {
		logf("saltar: %v", err)
		return
	}
	r.pausado = pausado
}
