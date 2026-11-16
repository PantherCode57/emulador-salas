package main

import (
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// Volumen: el control «Playback» de la tarjeta de sonido de la consola (el mismo que cambian el
// menú y los botones del sistema). En el reproductor, arriba y abajo lo suben y lo bajan.
var reVolumen = regexp.MustCompile(`\[(\d+)%\]`)

func leerVolumen() int {
	out, err := exec.Command("amixer", "sget", "Playback").Output()
	if err != nil {
		return -1
	}
	if m := reVolumen.FindSubmatch(out); m != nil {
		v, _ := strconv.Atoi(string(m[1]))
		return v
	}
	return -1
}

func (a *app) cambiarVolumen(paso int) {
	signo := "+"
	if paso < 0 {
		signo, paso = "-", -paso
	}
	_ = exec.Command("amixer", "-q", "sset", "Playback", strconv.Itoa(paso)+"%"+signo).Run()
	a.volumen = leerVolumen()
	a.volHasta = time.Now().Add(1500 * time.Millisecond)
}
