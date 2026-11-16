package main

// Teclado en pantalla (el de Spotin) para el código de la sala y tu nombre. Cruceta para moverse,
// A escribe, B borra, Y espacio, X cambia de letras, L y R mueven el cursor, START acepta y SELECT
// vuelve.
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

var especiales = []string{"ABC", "espacio", "borrar", "Aceptar"}

type teclado struct {
	capa      int
	fila, col int
	borrador  []rune
	cursor    int
	limite    int // letras como mucho (0: sin límite)
}

// abrir deja el teclado listo con un texto ya escrito (el cursor al final).
func (t *teclado) abrir(texto string, limite int) {
	t.borrador, t.cursor, t.limite, t.capa = []rune(texto), len([]rune(texto)), limite, 0
	t.fila, t.col = 1, 0
}

func (t *teclado) texto() string { return string(t.borrador) }

func (t *teclado) escribir(r rune) {
	if t.limite > 0 && len(t.borrador) >= t.limite {
		return
	}
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
