package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Luz de la pantalla: SELECT la apaga (la música sigue) y cualquier botón la enciende otra vez.
type luz struct {
	ruta    string
	antes   string
	apagada bool
}

func nuevaLuz() *luz {
	m, _ := filepath.Glob("/sys/class/backlight/*/brightness")
	l := &luz{}
	if len(m) > 0 {
		l.ruta = m[0]
	}
	return l
}

func (l *luz) escribir(v string) {
	if l.ruta == "" {
		return
	}
	if os.WriteFile(l.ruta, []byte(v), 0) != nil { // sin permiso: con sudo
		cmd := exec.Command("sudo", "-n", "tee", l.ruta)
		cmd.Stdin = strings.NewReader(v)
		_ = cmd.Run()
	}
}

func (l *luz) apagar() {
	if l.apagada || l.ruta == "" {
		return
	}
	b, _ := os.ReadFile(l.ruta)
	l.antes = strings.TrimSpace(string(b))
	if l.antes == "" || l.antes == "0" {
		l.antes = "128"
	}
	l.escribir("0")
	l.apagada = true
}

func (l *luz) encender() {
	if !l.apagada {
		return
	}
	l.escribir(l.antes)
	l.apagada = false
}
