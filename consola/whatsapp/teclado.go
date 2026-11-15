package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Teclado en pantalla. Cruceta para moverse, A escribe, B borra, Y espacio, X cambia de letras,
// L y R mueven el cursor, START envía y SELECT vuelve al chat (el borrador se guarda).
type resultadoTecla int

const (
	tecNada resultadoTecla = iota
	tecEnviar
	tecCancelar
)

// Capas: minúsculas, mayúsculas y símbolos. La última fila es especial.
var capas = [3][4]string{
	{"1234567890", "qwertyuiop", "asdfghjklñ", "zxcvbnm,.?"},
	{"1234567890", "QWERTYUIOP", "ASDFGHJKLÑ", "ZXCVBNM,.?"},
	{"áéíóúüç¿¡!", "@#€%&*()-+", "\"':;/_=<>~", "ÁÉÍÓÚ$[]{}"},
}

var especiales = []string{"ABC", "espacio", "borrar", "Enviar"}

type teclado struct {
	capa      int
	fila, col int
	borrador  []rune
	cursor    int
}

func (t *teclado) abrir() {
	t.fila, t.col = 1, 0
	if t.cursor > len(t.borrador) {
		t.cursor = len(t.borrador)
	}
}

func (t *teclado) texto() string { return string(t.borrador) }

func (t *teclado) borrarTodo() { t.borrador, t.cursor, t.capa = nil, 0, 0 }

func (t *teclado) escribir(r rune) {
	t.borrador = append(t.borrador[:t.cursor], append([]rune{r}, t.borrador[t.cursor:]...)...)
	t.cursor++
	if t.capa == 1 { // como en el móvil: la mayúscula vale para una letra
		t.capa = 0
	}
}

func (t *teclado) borrar() {
	if t.cursor > 0 {
		t.borrador = append(t.borrador[:t.cursor-1], t.borrador[t.cursor:]...)
		t.cursor--
	}
}

func (t *teclado) columnas(fila int) int {
	if fila == 4 {
		return len(especiales)
	}
	return 10
}

// cambiarFila pasa a otra fila sin perder la columna: la fila de abajo tiene 4 teclas anchas.
func (t *teclado) cambiarFila(nueva int) {
	switch {
	case nueva == 4 && t.fila != 4:
		t.col = min(t.col*len(especiales)/10, len(especiales)-1)
	case nueva != 4 && t.fila == 4:
		t.col = min(t.col*10/len(especiales)+1, 9)
	}
	t.fila = nueva
}

func (t *teclado) pulsar(b uint16) resultadoTecla {
	switch b {
	case bArriba:
		t.cambiarFila((t.fila + 4) % 5)
	case bAbajo:
		t.cambiarFila((t.fila + 1) % 5)
	case bIzquierda:
		t.col = (t.col + t.columnas(t.fila) - 1) % t.columnas(t.fila)
	case bDerecha:
		t.col = (t.col + 1) % t.columnas(t.fila)
	case bL:
		t.cursor = max(t.cursor-1, 0)
	case bR:
		t.cursor = min(t.cursor+1, len(t.borrador))
	case bB:
		t.borrar()
	case bY:
		t.escribir(' ')
	case bX:
		t.capa = (t.capa + 1) % 3
	case bStart:
		return tecEnviar
	case bSelect:
		return tecCancelar
	case bA:
		if t.fila < 4 {
			t.escribir([]rune(capas[t.capa][t.fila])[t.col])
			return tecNada
		}
		switch t.col {
		case 0:
			t.capa = (t.capa + 1) % 3
		case 1:
			t.escribir(' ')
		case 2:
			t.borrar()
		case 3:
			return tecEnviar
		}
	}
	return tecNada
}

// Respuestas rápidas: se pueden cambiar en respuestas.txt (una por línea).
var rapidasPorDefecto = []string{
	"Vale",
	"Ahora voy",
	"Estoy jugando, luego te escribo",
	"Sí",
	"No",
	"Jajaja",
	"¿Echamos una partida en las salas?",
	"Gracias",
}

func cargarRapidas() []string {
	ruta := filepath.Join(carpetaDatos, "respuestas.txt")
	f, err := os.Open(ruta)
	if err != nil {
		_ = os.WriteFile(ruta, []byte("# Respuestas rápidas del WhatsApp de la consola: una por línea.\n"+
			strings.Join(rapidasPorDefecto, "\n")+"\n"), 0o644)
		return rapidasPorDefecto
	}
	defer f.Close()
	var r []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		if l := strings.TrimSpace(s.Text()); l != "" && !strings.HasPrefix(l, "#") {
			r = append(r, l)
		}
	}
	if len(r) == 0 {
		return rapidasPorDefecto
	}
	return r
}
