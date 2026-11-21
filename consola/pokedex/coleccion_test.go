package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNombres(t *testing.T) {
	if total != 1025 {
		t.Fatalf("hay %d Pokémon; la Pokédex nacional llega al 1025", total)
	}
	vistos := map[string]int{}
	for i, s := range nombres {
		if s == "" {
			t.Fatalf("el %d no tiene nombre", i+1)
		}
		if j, ok := vistos[s]; ok {
			t.Fatalf("%q sale dos veces (%d y %d)", s, j, i+1)
		}
		vistos[s] = i + 1
	}
	for n, s := range map[int]string{1: "Bulbasaur", 25: "Pikachu", 151: "Mew", 772: "Código Cero", 1025: "Pecharunt"} {
		if nombre(n) != s {
			t.Errorf("el %d es %q, no %q", n, nombre(n), s)
		}
	}
	for n, g := range map[int]int{1: 1, 151: 1, 152: 2, 251: 2, 252: 3, 905: 8, 906: 9, 1025: 9} {
		if generacion(n) != g {
			t.Errorf("el %d es de la generación %d, no de la %d", n, generacion(n), g)
		}
	}
}

func TestBuscar(t *testing.T) {
	casos := []struct {
		q       string
		primero int
		todos   []int // si no es nil, tiene que salir exactamente esto
	}{
		{"25", 25, nil},
		{"025", 25, nil},
		{"0025", 25, nil},
		{"pikachu", 25, []int{25}},
		{"PIKA", 25, []int{25}},
		{"mr mime", 122, nil},
		{"Mr. Mime", 122, nil},
		{"nidoran", 29, []int{29, 32}},
		{"nidoran macho", 32, []int{32}},
		{"flabebe", 669, []int{669}},
		{"Flabébé", 669, []int{669}},
		{"codigo cero", 772, []int{772}},
		{"farfetch'd", 83, nil},
		{"ho-oh", 250, []int{250}},
		{"1025", 1025, []int{1025}},
		{"porygon2", 233, []int{233}},
	}
	for _, c := range casos {
		r := buscar(c.q)
		if len(r) == 0 || r[0] != c.primero {
			t.Errorf("buscar(%q) = %v; el primero tenía que ser %d", c.q, primeros(r), c.primero)
			continue
		}
		if c.todos != nil && !iguales(r, c.todos) {
			t.Errorf("buscar(%q) = %v; tenía que ser %v", c.q, r, c.todos)
		}
	}
	// Los números que empiezan así van detrás del exacto y antes que los nombres.
	r := buscar("15")
	if !iguales(r[:4], []int{15, 150, 151, 152}) {
		t.Errorf("buscar(15) = %v", primeros(r))
	}
	for _, q := range []string{"", "   ", "-.'", "zzz", "0", "2000"} {
		if r := buscar(q); len(r) != 0 {
			t.Errorf("buscar(%q) = %v; no tenía que salir nada", q, primeros(r))
		}
	}
}

func TestColeccion(t *testing.T) {
	dir := t.TempDir()
	c := abrirColeccion(dir)
	if c.n != 0 {
		t.Fatalf("una colección nueva tiene %d", c.n)
	}
	c.cambiar(25)
	c.cambiar(1)
	c.cambiar(1025)
	c.cambiar(1) // desmarcado
	c.cambiar(0) // no existe
	c.cambiar(2000)
	otra := abrirColeccion(dir)
	if otra.n != 2 || !otra.tiene[25] || !otra.tiene[1025] || otra.tiene[1] {
		t.Fatalf("al volver a abrir: %d conseguidos (25: %v, 1025: %v, 1: %v)", otra.n, otra.tiene[25], otra.tiene[1025], otra.tiene[1])
	}
	// Un archivo roto no se pierde: se aparta y se empieza de cero.
	ruta := filepath.Join(dir, "cartas.json")
	if err := os.WriteFile(ruta, []byte("{roto"), 0o600); err != nil {
		t.Fatal(err)
	}
	if c := abrirColeccion(dir); c.n != 0 {
		t.Fatalf("con el archivo roto salen %d", c.n)
	}
	if b, _ := os.ReadFile(ruta + ".roto"); string(b) != "{roto" {
		t.Fatalf("no se guardó la copia del archivo roto")
	}
}

func TestBotones(t *testing.T) {
	a := nuevaApp(cargarFuentes(), abrirColeccion(t.TempDir()))
	a.cargaHasta = time.Time{}
	toca := func(b uint16) { a.botones(b); a.botones(0) } // pulsar y soltar
	toca(bA)                                              // Cartas
	if a.pant != pCartas {
		t.Fatalf("A en Cartas no abre la lista")
	}
	toca(bA) // Bulbasaur, conseguido
	if !a.col.tiene[1] {
		t.Fatalf("A no marca")
	}
	toca(bArriba) // da la vuelta: al último
	if a.lista[a.sel] != total {
		t.Fatalf("arriba desde el primero va al %d", a.lista[a.sel])
	}
	toca(bR) // ya en la última generación: se queda
	toca(bL) // al primero de la generación 9
	if a.lista[a.sel] != 906 {
		t.Fatalf("L va al %d", a.lista[a.sel])
	}
	toca(bL)
	if a.lista[a.sel] != 810 {
		t.Fatalf("L otra vez va al %d", a.lista[a.sel])
	}
	toca(bY)
	for _, b := range []uint16{bDerecha, bDerecha, bDerecha, bDerecha, bDerecha, bDerecha, bDerecha, bA} { // «i»
		toca(b)
	}
	if a.tec.texto() != "i" {
		t.Fatalf("se ha escrito %q", a.tec.texto())
	}
	a.tec.abrir("eevee")
	toca(bStart)
	if a.pant != pCartas || a.busqueda != "eevee" || len(a.lista) != 1 || a.lista[0] != 133 {
		t.Fatalf("buscar eevee: %v %q %v", a.pant, a.busqueda, a.lista)
	}
	toca(bB) // quita lo buscado y se queda en Eevee
	if a.busqueda != "" || len(a.lista) != total || a.lista[a.sel] != 133 {
		t.Fatalf("B tras buscar: %q, %d, en el %d", a.busqueda, len(a.lista), a.lista[a.sel])
	}
	toca(bB)
	if a.pant != pInicio {
		t.Fatalf("B en la lista no vuelve al inicio")
	}
	toca(bAbajo)
	toca(bA)
	if a.pant != pJuegos {
		t.Fatalf("Juegos no se abre")
	}
	toca(bB)
	toca(bB)
	select {
	case <-a.salir:
	default:
		t.Fatalf("B en el inicio no sale")
	}
}

func primeros(r []int) []int { return r[:min(len(r), 8)] }

func iguales(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
