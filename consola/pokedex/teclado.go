package main

// Teclado en pantalla para buscar (como el de las otras apps de Panther OS, pero solo con lo que
// hace falta para un nombre o un número). Cruceta para moverse, A escribe, B borra, Y espacio,
// X lo borra todo, L y R mueven el cursor, START busca y SELECT vuelve sin buscar.
type resultadoTecla int

const (
	tecNada resultadoTecla = iota
	tecBuscar
	tecCancelar
	tecCambia // ha cambiado lo escrito
)

var filasTeclado = [4]string{"1234567890", "qwertyuiop", "asdfghjklñ", "zxcvbnm-.'"}

// La fila de abajo: espacio, borrar y buscar.
var especiales = [...]string{"espacio", "borrar", "Buscar"}

const maxBusqueda = 24

type teclado struct {
	fila, col int
	escrito   []rune
	cursor    int
}

func (t *teclado) abrir(texto string) {
	t.escrito = []rune(texto)
	t.cursor = len(t.escrito)
	t.fila, t.col = 1, 0
}

func (t *teclado) texto() string { return string(t.escrito) }

func (t *teclado) escribir(r rune) bool {
	if len(t.escrito) >= maxBusqueda {
		return false
	}
	t.escrito = append(t.escrito[:t.cursor], append([]rune{r}, t.escrito[t.cursor:]...)...)
	t.cursor++
	return true
}

func (t *teclado) borrar() bool {
	if t.cursor == 0 {
		return false
	}
	t.escrito = append(t.escrito[:t.cursor-1], t.escrito[t.cursor:]...)
	t.cursor--
	return true
}

func (t *teclado) columnas(fila int) int {
	if fila == 4 {
		return len(especiales)
	}
	return 10
}

// cambiarFila pasa a otra fila sin perder la columna: la fila de abajo tiene 3 teclas anchas.
func (t *teclado) cambiarFila(nueva int) {
	switch {
	case nueva == 4 && t.fila != 4:
		t.col = min(t.col*len(especiales)/10, len(especiales)-1)
	case nueva != 4 && t.fila == 4:
		t.col = min(t.col*10/len(especiales)+1, 9)
	}
	t.fila = nueva
}

func cambio(ok bool) resultadoTecla {
	if ok {
		return tecCambia
	}
	return tecNada
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
		t.cursor = min(t.cursor+1, len(t.escrito))
	case bB:
		return cambio(t.borrar())
	case bY:
		return cambio(t.escribir(' '))
	case bX:
		hay := len(t.escrito) > 0
		t.escrito, t.cursor = nil, 0
		return cambio(hay)
	case bStart:
		return tecBuscar
	case bSelect:
		return tecCancelar
	case bA:
		if t.fila < 4 {
			return cambio(t.escribir([]rune(filasTeclado[t.fila])[t.col]))
		}
		switch t.col {
		case 0:
			return cambio(t.escribir(' '))
		case 1:
			return cambio(t.borrar())
		case 2:
			return tecBuscar
		}
	}
	return tecNada
}
