package main

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// yt-dlp siempre abierto mientras Spotin está abierto (ytdlp_servidor.py): arrancar Python y cargar
// yt-dlp tarda unos 16 s en la consola, y antes se hacía con cada canción (por eso cada una tardaba
// ~19 s). Ahora se arranca al abrir Spotin y cada canción tarda solo lo que tarda YouTube (~3 s).
// Atiende una canción a la vez. Si falla o se cuelga, se cierra, se vuelve a abrir para la siguiente
// y esa canción se pide como antes (un yt-dlp para ella sola).

//go:embed ytdlp_servidor.py
var guionYtdlp string

type ytdlpVivo struct {
	mu     sync.Mutex // una canción a la vez
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	listo  chan struct{}
	muerto chan struct{}
	err    error // si no ha podido arrancar
}

// respuestaYtdlp: lo mismo que daba «yt-dlp -J» (lo que hace falta).
type respuestaYtdlp struct {
	URL       string            `json:"url"`
	Ext       string            `json:"ext"`
	Acodec    string            `json:"acodec"`
	Tamano    float64           `json:"filesize"`
	Aprox     float64           `json:"filesize_approx"`
	Cabeceras map[string]string `json:"http_headers"`
	Error     string            `json:"error"`
}

func arrancarYtdlpVivo() *ytdlpVivo {
	v := &ytdlpVivo{listo: make(chan struct{}), muerto: make(chan struct{})}
	go v.arrancar()
	return v
}

func (v *ytdlpVivo) arrancar() {
	fallar := func(err error) {
		v.err = err
		logf("yt-dlp siempre abierto: %v (se pide cada canción como antes)", err)
		close(v.listo)
	}
	if _, err := os.Stat(ytdlpBin); err != nil {
		fallar(err)
		return
	}
	guion := filepath.Join(os.TempDir(), "spotin-ytdlp.py")
	if err := os.WriteFile(guion, []byte(guionYtdlp), 0o644); err != nil {
		fallar(err)
		return
	}
	inicio := time.Now()
	cmd := exec.Command("python3", guion, ytdlpBin)
	grupoPropio(cmd) // se cierra con Spotin
	in, err := cmd.StdinPipe()
	if err != nil {
		fallar(err)
		return
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		fallar(err)
		return
	}
	errOut, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		fallar(err)
		return
	}
	v.cmd, v.in, v.out = cmd, in, bufio.NewReaderSize(out, 64<<10)
	go func() { // lo que diga yt-dlp (los errores de cada canción)
		s := bufio.NewScanner(errOut)
		for s.Scan() {
			if t := strings.TrimSpace(s.Text()); t != "" {
				logf("yt-dlp: %s", recortarTexto(t, 200))
			}
		}
	}()
	go func() { _ = cmd.Wait(); close(v.muerto) }()
	linea, err := v.out.ReadBytes('\n')
	if err != nil || !strings.Contains(string(linea), `"listo"`) {
		fallar(errors.New("no ha arrancado"))
		return
	}
	logf("yt-dlp siempre abierto, listo en %.1f s", time.Since(inicio).Seconds())
	close(v.listo)
}

// errorYouTube: lo que contesta YouTube (vídeo no disponible…), no un fallo de yt-dlp.
type errorYouTube struct{ msg string }

func (e errorYouTube) Error() string { return e.msg }

func recortarTexto(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func (v *ytdlpVivo) vivo() bool {
	select {
	case <-v.muerto:
		return false
	default:
		return v.err == nil
	}
}

// pedir: el sonido de un vídeo. Espera a que esté listo (como mucho 30 s, lo que tarda en abrirse).
func (v *ytdlpVivo) pedir(video string, plazo time.Duration) (*respuestaYtdlp, error) {
	select {
	case <-v.listo:
	case <-time.After(30 * time.Second):
		return nil, errors.New("yt-dlp aún se está abriendo")
	}
	if v.err != nil {
		return nil, v.err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.vivo() {
		return nil, errors.New("yt-dlp se ha cerrado")
	}
	if _, err := io.WriteString(v.in, video+"\n"); err != nil {
		return nil, err
	}
	type leido struct {
		b   []byte
		err error
	}
	ch := make(chan leido, 1)
	go func() {
		b, err := v.out.ReadBytes('\n')
		ch <- leido{b, err}
	}()
	select {
	case l := <-ch:
		if l.err != nil {
			return nil, l.err
		}
		var r respuestaYtdlp
		if err := json.Unmarshal(l.b, &r); err != nil {
			return nil, err
		}
		if r.Error != "" {
			return nil, errorYouTube{r.Error}
		}
		return &r, nil
	case <-time.After(plazo):
		v.cerrar() // colgado: fuera (el siguiente abre otro)
		return nil, fmt.Errorf("yt-dlp no contesta en %v", plazo)
	}
}

func (v *ytdlpVivo) cerrar() {
	if v == nil || v.cmd == nil || v.cmd.Process == nil {
		return
	}
	señalGrupo(v.cmd, syscall.SIGKILL)
}
