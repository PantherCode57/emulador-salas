package main

import (
	"encoding/binary"
	"io"
	"math"
	"math/rand"
	"os/exec"
	"strconv"
	"sync"
)

// Sonido: un solo aplay abierto todo el rato, al que se le manda el sonido mezclado a trozos de 20 ms:
// la música (una melodía propia, hecha aquí con ondas sencillas) y los efectos (salto, estrella,
// choque…). Así no hay que abrir un programa por cada estrella.
const (
	frecuencia = 22050
	trozo      = frecuencia / 50 // 20 ms
)

type voz struct {
	pcm []float32
	pos int
}

type sonido struct {
	mu      sync.Mutex
	voces   []*voz
	musica  bool
	callado bool
	efectos map[string][]float32
	cmd     *exec.Cmd
	salida  io.WriteCloser
	t       int // muestras desde el principio (para la música)
}

func nuevoSonido() *sonido {
	s := &sonido{efectos: map[string][]float32{}, musica: true}
	s.efectos["salto"] = barrido(380, 760, 0.12, 0.35, onda(0.5))
	s.efectos["agacharse"] = barrido(420, 200, 0.1, 0.3, onda(0.5))
	s.efectos["carril"] = barrido(900, 700, 0.04, 0.18, onda(0.5))
	s.efectos["estrella"] = append(barrido(1318, 1318, 0.05, 0.25, onda(0.25)), barrido(1975, 1975, 0.09, 0.25, onda(0.25))...)
	s.efectos["poder"] = arpegio([]float64{523, 659, 784, 1046, 1318}, 0.06, 0.3)
	s.efectos["escudo"] = ruido(0.18, 0.4, 0.5)
	s.efectos["choque"] = append(ruido(0.35, 0.6, 0.9), barrido(220, 60, 0.4, 0.5, onda(0.5))...)
	s.efectos["record"] = arpegio([]float64{523, 659, 784, 1046, 784, 1046, 1318}, 0.09, 0.3)
	return s
}

// onda: cuadrada con ese ancho de pulso (0.5 = cuadrada; 0.25 suena más fina).
func onda(ancho float64) func(fase float64) float64 {
	return func(f float64) float64 {
		if f-math.Floor(f) < ancho {
			return 1
		}
		return -1
	}
}

func barrido(f0, f1, dur, vol float64, forma func(float64) float64) []float32 {
	n := int(dur * frecuencia)
	out := make([]float32, n)
	fase := 0.0
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n)
		fase += (f0 + (f1-f0)*t) / frecuencia
		env := math.Min(1, float64(i)/80) * (1 - t)
		out[i] = float32(forma(fase) * vol * env)
	}
	return out
}

func arpegio(fs []float64, nota, vol float64) []float32 {
	var out []float32
	for _, f := range fs {
		out = append(out, barrido(f, f, nota, vol, onda(0.5))...)
	}
	return out
}

func ruido(dur, vol, caida float64) []float32 {
	n := int(dur * frecuencia)
	out := make([]float32, n)
	r := rand.New(rand.NewSource(3))
	v := 0.0
	for i := 0; i < n; i++ {
		v = v*0.6 + (r.Float64()*2-1)*0.4 // ruido algo grave
		out[i] = float32(v * vol * math.Pow(1-float64(i)/float64(n), caida*3))
	}
	return out
}

func (s *sonido) abrir() {
	if s.callado {
		return
	}
	cmd := exec.Command("aplay", "-q", "-t", "raw", "-f", "S16_LE", "-r", strconv.Itoa(frecuencia), "-c", "1", "--buffer-time=120000")
	in, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	grupoPropio(cmd)
	if cmd.Start() != nil {
		return
	}
	s.cmd, s.salida = cmd, in
	go s.mezclar()
}

func (s *sonido) cerrar() {
	if s.salida != nil {
		_ = s.salida.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}

func (s *sonido) efecto(nombre string) {
	pcm := s.efectos[nombre]
	if pcm == nil || s.callado {
		return
	}
	s.mu.Lock()
	if len(s.voces) < 12 {
		s.voces = append(s.voces, &voz{pcm: pcm})
	}
	s.mu.Unlock()
}

func (s *sonido) conMusica(si bool) {
	s.mu.Lock()
	s.musica = si
	s.mu.Unlock()
}

// La música: 4 compases (La menor, Fa, Do, Sol) a 112 pulsos por minuto, con un bajo, un arpegio y
// una melodía sencilla por encima. Hecha para este juego.
var (
	acordes = [4][3]float64{{220, 261.63, 329.63}, {174.61, 220, 261.63}, {130.81, 164.81, 196}, {196, 246.94, 293.66}}
	melodia = [32]float64{659.25, 0, 587.33, 523.25, 0, 523.25, 587.33, 659.25,
		523.25, 0, 440, 0, 523.25, 587.33, 523.25, 0,
		523.25, 0, 493.88, 523.25, 0, 587.33, 659.25, 0,
		587.33, 0, 493.88, 0, 392, 0, 493.88, 0}
)

func (s *sonido) muestraMusica(t int) float64 {
	const muestrasOctavo = frecuencia * 60 / 112 / 2
	octavo := t / muestrasOctavo
	enOctavo := float64(t%muestrasOctavo) / muestrasOctavo
	compas := (octavo / 8) % 4
	ac := acordes[compas]
	seg := float64(t) / frecuencia
	// el bajo: triangular, en el primer y quinto octavo de cada compás
	bajo := 0.0
	if octavo%4 != 3 {
		f := ac[0] / 2
		fase := seg * f
		bajo = (2*math.Abs(2*(fase-math.Floor(fase+0.5))) - 1) * 0.22 * (1 - enOctavo*0.5)
	}
	// el arpegio: las notas del acorde una tras otra, cortitas
	nota := ac[octavo%3] * 2
	arp := onda(0.25)(seg*nota) * 0.045 * math.Max(0, 1-enOctavo*1.6)
	// la melodía
	mel := 0.0
	if f := melodia[octavo%32]; f > 0 {
		env := math.Min(1, enOctavo*20) * (1 - enOctavo*0.6)
		mel = (onda(0.5)(seg*f)*0.6 + math.Sin(2*math.Pi*seg*f)*0.4) * 0.07 * env
	}
	return bajo + arp + mel
}

func (s *sonido) mezclar() {
	buf := make([]byte, trozo*2)
	for {
		s.mu.Lock()
		musica := s.musica
		for i := 0; i < trozo; i++ {
			v := 0.0
			if musica {
				v = s.muestraMusica(s.t + i)
			}
			for _, vz := range s.voces {
				if vz.pos < len(vz.pcm) {
					v += float64(vz.pcm[vz.pos])
					vz.pos++
				}
			}
			v = math.Max(-1, math.Min(1, v*0.8))
			binary.LittleEndian.PutUint16(buf[i*2:], uint16(int16(v*32000)))
		}
		s.t += trozo
		vivas := s.voces[:0]
		for _, vz := range s.voces {
			if vz.pos < len(vz.pcm) {
				vivas = append(vivas, vz)
			}
		}
		s.voces = vivas
		s.mu.Unlock()
		if _, err := s.salida.Write(buf); err != nil { // aplay marca el paso: espera si va adelantado
			return
		}
	}
}
