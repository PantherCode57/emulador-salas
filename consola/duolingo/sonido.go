package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Sonido: las voces de los ejercicios (los mp3 de Duolingo) y los avisos de bien y mal. Las voces
// se bajan a la memoria al empezar la lección (son pequeñas) para que suenen en cuanto se piden;
// ffmpeg las pasa a sonido sin comprimir y aplay lo toca, como en el resto de apps de la consola.
const (
	frecuencia = 44100
	ffmpegBin  = "ffmpeg"
	aplayBin   = "aplay"
)

var clienteVoces = &http.Client{Timeout: 20 * time.Second}

type sonido struct {
	mu      sync.Mutex
	voces   map[string][]byte
	bajando map[string]chan struct{}
	cmds    []*exec.Cmd // lo que suena ahora (ffmpeg y aplay)
	turno   int
	callado bool // sin sonido (las pruebas)
}

func nuevoSonido() *sonido {
	return &sonido{voces: map[string][]byte{}, bajando: map[string]chan struct{}{}}
}

// voz baja (o da la ya bajada) la voz de esa dirección.
func (s *sonido) voz(url string) []byte {
	s.mu.Lock()
	if b, ok := s.voces[url]; ok {
		s.mu.Unlock()
		return b
	}
	if ch, ok := s.bajando[url]; ok { // ya la está bajando otro: se espera
		s.mu.Unlock()
		<-ch
		s.mu.Lock()
		b := s.voces[url]
		s.mu.Unlock()
		return b
	}
	ch := make(chan struct{})
	s.bajando[url] = ch
	s.mu.Unlock()

	var datos []byte
	if req, err := http.NewRequest(http.MethodGet, url, nil); err == nil {
		req.Header.Set("User-Agent", agenteNavegador)
		if resp, err := clienteVoces.Do(req); err == nil {
			if resp.StatusCode == http.StatusOK {
				datos, _ = io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			} else {
				logf("voz: %d", resp.StatusCode)
			}
			resp.Body.Close()
		} else {
			logf("voz: %v", err)
		}
	}
	s.mu.Lock()
	if len(datos) > 0 {
		s.voces[url] = datos
	}
	delete(s.bajando, url)
	s.mu.Unlock()
	close(ch)
	return datos
}

// precargar baja las voces de la lección (de cuatro en cuatro) sin esperar.
func (s *sonido) precargar(urls []string) {
	go func() {
		turno := make(chan struct{}, 4)
		var wg sync.WaitGroup
		for _, u := range urls {
			if u == "" {
				continue
			}
			wg.Add(1)
			turno <- struct{}{}
			go func(u string) {
				defer wg.Done()
				s.voz(u)
				<-turno
			}(u)
		}
		wg.Wait()
	}()
}

// olvidar suelta las voces de la lección que se acaba.
func (s *sonido) olvidar() {
	s.mu.Lock()
	s.voces = map[string][]byte{}
	s.mu.Unlock()
}

// parar corta lo que suene.
func (s *sonido) parar() {
	s.mu.Lock()
	s.turno++
	cmds := s.cmds
	s.cmds = nil
	s.mu.Unlock()
	for _, c := range cmds {
		if c.Process != nil {
			_ = c.Process.Kill()
		}
	}
}

// decir toca la voz de esa dirección (y corta la de antes). No espera.
func (s *sonido) decir(url string) {
	if url == "" || s.callado {
		return
	}
	s.parar()
	s.mu.Lock()
	t := s.turno
	s.mu.Unlock()
	go func() {
		datos := s.voz(url)
		s.mu.Lock()
		vale := t == s.turno
		s.mu.Unlock()
		if len(datos) == 0 || !vale {
			return
		}
		s.tocarMp3(datos, t)
	}()
}

func (s *sonido) tocarMp3(datos []byte, t int) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return
	}
	ff := exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error", "-i", "pipe:0",
		"-f", "s16le", "-ac", "2", "-ar", strconv.Itoa(frecuencia), "pipe:1")
	ff.Stdin = bytes.NewReader(datos)
	ff.Stdout = pw
	ap := s.aplay()
	ap.Stdin = pr
	if err := ap.Start(); err != nil {
		pr.Close()
		pw.Close()
		logf("aplay: %v", err)
		return
	}
	if err := ff.Start(); err != nil {
		pr.Close()
		pw.Close()
		_ = ap.Process.Kill()
		_ = ap.Wait()
		logf("ffmpeg: %v", err)
		return
	}
	pr.Close()
	pw.Close()
	s.mu.Lock()
	if t != s.turno { // mientras arrancaba se ha pedido otra
		s.mu.Unlock()
		_ = ff.Process.Kill()
		_ = ap.Process.Kill()
	} else {
		s.cmds = []*exec.Cmd{ff, ap}
		s.mu.Unlock()
	}
	_ = ff.Wait()
	_ = ap.Wait()
}

func (s *sonido) aplay() *exec.Cmd {
	return exec.Command(aplayBin, "-q", "-t", "raw", "-f", "S16_LE", "-r", strconv.Itoa(frecuencia), "-c", "2")
}

// aviso: dos notas cortas que suben (bien) o bajan (mal), y un acorde al acabar la lección.
func (s *sonido) aviso(bien bool) {
	if bien {
		s.notas([]float64{659.25, 987.77}, 0.09)
	} else {
		s.notas([]float64{311.13, 233.08}, 0.13)
	}
}

func (s *sonido) fanfarria() {
	s.notas([]float64{523.25, 659.25, 783.99, 1046.5}, 0.11)
}

func (s *sonido) notas(fs []float64, dur float64) {
	if s.callado {
		return
	}
	var pcm bytes.Buffer
	n := int(dur * frecuencia)
	for _, f := range fs {
		for i := 0; i < n; i++ {
			t := float64(i) / frecuencia
			env := math.Min(1, float64(i)/220) * math.Exp(-3.2*t/dur) // ataque corto y se apaga
			v := (math.Sin(2*math.Pi*f*t) + 0.25*math.Sin(4*math.Pi*f*t)) * env * 0.22
			m := int16(v * 32767)
			_ = binary.Write(&pcm, binary.LittleEndian, [2]int16{m, m})
		}
	}
	go func() {
		ap := s.aplay()
		ap.Stdin = bytes.NewReader(pcm.Bytes())
		_ = ap.Run()
	}()
}
