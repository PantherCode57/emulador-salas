package main

import (
	"bufio"
	"bytes"
	"context"
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

// yt-dlp siempre abierto mientras YouTube está abierto (ytdlp_servidor.py): arrancar Python y cargar
// yt-dlp tarda unos 16 s en la consola, y antes se hacía con cada vídeo (por eso cada uno tardaba
// ~20 s). Ahora se arranca al abrir la app y cada vídeo tarda lo que tarda YouTube en contestar.
// Atiende una petición a la vez, con las mismas opciones que la línea de órdenes y la misma
// respuesta que «yt-dlp -J». Si falla o se cuelga, se cierra, se vuelve a abrir para la siguiente y
// esa petición se hace como antes (un yt-dlp para ella sola).

//go:embed ytdlp_servidor.py
var guionYtdlp string

type ytdlpVivo struct {
	mu     sync.Mutex // una petición a la vez
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	listo  chan struct{}
	muerto chan struct{}
	err    error // si no ha podido arrancar
}

// errorYouTube: lo que contesta YouTube (vídeo no disponible…), no un fallo de yt-dlp.
type errorYouTube struct{ msg string }

func (e errorYouTube) Error() string { return e.msg }

var (
	muVivo sync.Mutex
	vivo   *ytdlpVivo
)

// abrirYtdlp: el yt-dlp siempre abierto (si se ha cerrado, otro). nil si no puede abrirse.
func abrirYtdlp() *ytdlpVivo {
	muVivo.Lock()
	defer muVivo.Unlock()
	if vivo == nil || (!vivo.vivo() && vivo.err == nil) {
		vivo = arrancarYtdlpVivo()
	}
	if vivo.err != nil {
		return nil
	}
	return vivo
}

func cerrarYtdlp() {
	muVivo.Lock()
	defer muVivo.Unlock()
	vivo.cerrar()
}

func arrancarYtdlpVivo() *ytdlpVivo {
	v := &ytdlpVivo{listo: make(chan struct{}), muerto: make(chan struct{})}
	go v.arrancar()
	return v
}

func (v *ytdlpVivo) arrancar() {
	fallar := func(err error) {
		v.err = err
		logf("yt-dlp siempre abierto: %v (se pide cada vídeo como antes)", err)
		close(v.listo)
	}
	if _, err := os.Stat(ytdlpBin); err != nil {
		fallar(err)
		return
	}
	guion := filepath.Join(os.TempDir(), "youtube-ytdlp.py")
	if err := os.WriteFile(guion, []byte(guionYtdlp), 0o644); err != nil {
		fallar(err)
		return
	}
	inicio := time.Now()
	cmd := exec.Command("python3", guion, ytdlpBin)
	grupoPropio(cmd) // se cierra con la app
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
	v.cmd, v.in, v.out = cmd, in, bufio.NewReaderSize(out, 256<<10)
	go func() {
		s := bufio.NewScanner(errOut)
		for s.Scan() {
			if t := strings.TrimSpace(s.Text()); t != "" {
				if len(t) > 200 {
					t = t[:200] + "…"
				}
				logf("yt-dlp: %s", t)
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

func (v *ytdlpVivo) vivo() bool {
	select {
	case <-v.muerto:
		return false
	default:
		return v.err == nil
	}
}

// pedir: lo que diría «yt-dlp <args> <url>» (con -J), en JSON.
func (v *ytdlpVivo) pedir(args []string, url string, plazo time.Duration) ([]byte, error) {
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
	pet, _ := json.Marshal(map[string]any{"args": args, "url": url})
	if _, err := v.in.Write(append(pet, '\n')); err != nil {
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
		if bytes.HasPrefix(l.b, []byte(`{"error"`)) {
			var e struct {
				Error string `json:"error"`
			}
			_ = json.Unmarshal(l.b, &e)
			return nil, errorYouTube{e.Error}
		}
		return l.b, nil
	case <-time.After(plazo):
		v.cerrar() // colgado: fuera (la siguiente abre otro)
		return nil, fmt.Errorf("yt-dlp no contesta en %v", plazo)
	}
}

func (v *ytdlpVivo) cerrar() {
	if v == nil || v.cmd == nil || v.cmd.Process == nil {
		return
	}
	señalGrupo(v.cmd, syscall.SIGKILL)
}

// ytdlpJSON: «yt-dlp -J <args> <url>», con el yt-dlp siempre abierto si se puede y, si no, como
// antes. Lo que dice YouTube (no se puede ver…) no se vuelve a preguntar.
func ytdlpJSON(args []string, url string, plazo time.Duration) ([]byte, error) {
	if v := abrirYtdlp(); v != nil {
		inicio := time.Now()
		out, err := v.pedir(args, url, plazo)
		if err == nil {
			logf("yt-dlp: %s en %.1f s", ultimo(url, "="), time.Since(inicio).Seconds())
			return out, nil
		}
		var ey errorYouTube
		if errors.As(err, &ey) {
			return nil, err
		}
		logf("yt-dlp siempre abierto: %v (lo pido como antes)", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), plazo+30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ytdlpBin, append(append([]string{}, args...), url)...)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errOut.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return nil, fmt.Errorf("%v: %s", err, msg)
	}
	return out, nil
}
