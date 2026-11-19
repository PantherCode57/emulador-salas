package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os/exec"
	"strconv"
)

// Los sonidos: notas cortas hechas aquí y tocadas con aplay (como el resto de apps de la consola).
const frecuencia = 44100

type sonido struct{ callado bool }

func (s *sonido) notas(fs []float64, dur, volumen float64) {
	if s.callado {
		return
	}
	var pcm bytes.Buffer
	n := int(dur * frecuencia)
	for _, f := range fs {
		for i := 0; i < n; i++ {
			t := float64(i) / frecuencia
			env := math.Min(1, float64(i)/150) * math.Exp(-4*t/dur)
			v := (math.Sin(2*math.Pi*f*t) + 0.3*math.Sin(4*math.Pi*f*t)) * env * volumen
			m := int16(v * 32767)
			_ = binary.Write(&pcm, binary.LittleEndian, [2]int16{m, m})
		}
	}
	go func() {
		ap := exec.Command("aplay", "-q", "-t", "raw", "-f", "S16_LE", "-r", strconv.Itoa(frecuencia), "-c", "2")
		ap.Stdin = bytes.NewReader(pcm.Bytes())
		_ = ap.Run()
	}()
}

// una pieza que se mueve (más grave si come), el jaque, el reto que llega y el final
func (s *sonido) jugada(come bool) {
	if come {
		s.notas([]float64{196}, 0.07, 0.35)
	} else {
		s.notas([]float64{330}, 0.05, 0.25)
	}
}
func (s *sonido) jaque() { s.notas([]float64{880, 660}, 0.08, 0.22) }
func (s *sonido) reto()  { s.notas([]float64{523.25, 659.25, 783.99}, 0.09, 0.22) }
func (s *sonido) bien()  { s.notas([]float64{659.25, 987.77}, 0.09, 0.22) }
func (s *sonido) mal()   { s.notas([]float64{311.13, 233.08}, 0.13, 0.22) }
func (s *sonido) fin(gana bool) {
	if gana {
		s.notas([]float64{523.25, 659.25, 783.99, 1046.5}, 0.11, 0.22)
	} else {
		s.notas([]float64{392, 329.63, 261.63}, 0.14, 0.2)
	}
}
