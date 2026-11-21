package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// total: cuántos Pokémon hay en la Pokédex nacional.
const total = len(nombres)

// nombre del Pokémon con ese número (del 1 al total).
func nombre(n int) string {
	if n < 1 || n > total {
		return ""
	}
	return nombres[n-1]
}

// generacion: la generación del número n (de 1 a 9).
func generacion(n int) int {
	g := 1
	for i, ini := range inicioGeneracion {
		if n >= ini {
			g = i + 1
		}
	}
	return g
}

// coleccion: las cartas conseguidas. Se guarda en cartas.json cada vez que se marca una.
type coleccion struct {
	ruta  string
	tiene [total + 1]bool
	n     int // cuántas hay
}

type archivoCartas struct {
	Conseguidos []int `json:"conseguidos"`
}

func abrirColeccion(dir string) *coleccion {
	c := &coleccion{ruta: filepath.Join(dir, "cartas.json")}
	b, err := os.ReadFile(c.ruta)
	if err != nil {
		return c
	}
	var a archivoCartas
	if json.Unmarshal(b, &a) != nil {
		logf("cartas.json no se entiende: se empieza de cero (queda una copia en cartas.json.roto)")
		_ = os.WriteFile(c.ruta+".roto", b, 0o600)
		return c
	}
	for _, n := range a.Conseguidos {
		if n >= 1 && n <= total && !c.tiene[n] {
			c.tiene[n] = true
			c.n++
		}
	}
	return c
}

// cambiar marca o desmarca un Pokémon y lo guarda. Devuelve cómo queda.
func (c *coleccion) cambiar(n int) bool {
	if n < 1 || n > total {
		return false
	}
	c.tiene[n] = !c.tiene[n]
	if c.tiene[n] {
		c.n++
	} else {
		c.n--
	}
	c.guardar()
	return c.tiene[n]
}

func (c *coleccion) guardar() {
	a := archivoCartas{Conseguidos: []int{}}
	for n := 1; n <= total; n++ {
		if c.tiene[n] {
			a.Conseguidos = append(a.Conseguidos, n)
		}
	}
	b, _ := json.Marshal(a)
	if err := escribirAtomico(c.ruta, b); err != nil {
		logf("no se pudo guardar %s: %v", c.ruta, err)
	}
}

// ---- Buscar ----

var quitarTildes = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalizar: en minúsculas, sin tildes (la ñ queda en n) y solo letras y números («Mr. Mime» →
// «mrmime»). ♀ y ♂ se buscan como «hembra» y «macho».
func normalizar(s string) string {
	s = strings.NewReplacer("♀", "hembra", "♂", "macho").Replace(s)
	s, _, _ = transform.String(quitarTildes, strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var normalizados = func() [total]string {
	var out [total]string
	for i, s := range nombres {
		out[i] = normalizar(s)
	}
	return out
}()

// buscar: los números de los Pokémon que encajan con lo escrito, por número o por nombre.
// Primero el número exacto, luego los números que empiezan así, luego los nombres que empiezan
// así y por último los que lo llevan dentro (cada grupo en orden de la Pokédex).
func buscar(q string) []int {
	q = normalizar(q)
	if q == "" {
		return nil
	}
	nivel := map[int]int{}
	poner := func(n, k int) {
		if v, ok := nivel[n]; !ok || k < v {
			nivel[n] = k
		}
	}
	if soloCifras(q) {
		limpio := strings.TrimLeft(q, "0")
		if v, err := strconv.Atoi(q); err == nil && v >= 1 && v <= total {
			poner(v, 0)
		}
		if limpio != "" {
			for n := 1; n <= total; n++ {
				if strings.HasPrefix(strconv.Itoa(n), limpio) {
					poner(n, 1)
				}
			}
		}
	}
	for i, s := range normalizados {
		switch {
		case strings.HasPrefix(s, q):
			poner(i+1, 2)
		case strings.Contains(s, q):
			poner(i+1, 3)
		}
	}
	out := make([]int, 0, len(nivel))
	for n := range nivel {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if nivel[out[i]] != nivel[out[j]] {
			return nivel[out[i]] < nivel[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

func soloCifras(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
