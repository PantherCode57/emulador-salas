package main

import (
	"math/rand"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Ejercicios de una lección, ya pasados a lo que sabe enseñar la consola (ver duolingo.go para cómo
// salen de lo que manda Duolingo).
type tipoEj int

const (
	ejElegir   tipoEj = iota // una opción de varias (también «rellena el hueco» con una opción)
	ejBanco                  // formar la frase con fichas
	ejParejas                // emparejar palabras
	ejEscribir               // escribir con el teclado
	ejHuecos                 // rellenar varios huecos con fichas
)

type opcion struct {
	Texto string
	Audio string
}

type pieza struct {
	Texto string
	Hueco bool
}

type ejercicio struct {
	Tipo        tipoEj
	Indice      int    // en la sesión de Duolingo (challenges[i])
	Clase       string // el tipo de Duolingo («translate», «select»…)
	Instruccion string
	Frase       string  // lo que hay que traducir o la pregunta
	Piezas      []pieza // la frase con huecos (ejHuecos y, si hay, ejElegir)
	Audio       string  // la voz de la frase (o de lo que hay que oír)
	AudioLento  string
	SoloOir     bool // la frase no se enseña: hay que oírla
	Opciones    []opcion
	Correcta    int      // ejElegir
	Correctas   []int    // ejHuecos: la ficha buena de cada hueco
	Soluciones  []string // ejBanco y ejEscribir: lo que vale
	Mostrar     string   // la solución que se enseña si fallas
	Izquierda   []opcion // ejParejas: en el idioma que aprendes
	Derecha     []opcion // ejParejas: su traducción (Derecha[i] va con Izquierda[i])
	Idioma      string   // el de la respuesta (para el teclado)
}

// leccion: la sesión de Duolingo y cómo va.
type leccion struct {
	crudo       map[string]any // la sesión tal cual (se devuelve al acabar)
	ejs         []*ejercicio
	cola        []int // los que quedan; los fallados vuelven al final
	acertados   map[int]bool
	vidas       int // -1: sin límite
	inicio      time.Time
	fallos      int
	seguidos    int
	maxSeguidos int
	nombre      string // «Unidad 2 · Lección 3», «Repaso»…
	saltados    int    // los que la consola no sabe enseñar (hablar…)
	respuestas  map[int]respuesta
	pet         peticion
}

func nuevaLeccion(crudo map[string]any, ejs []*ejercicio, vidas int, nombre string) *leccion {
	l := &leccion{crudo: crudo, ejs: ejs, acertados: map[int]bool{}, vidas: vidas, inicio: time.Now(), nombre: nombre}
	for i := range ejs {
		l.cola = append(l.cola, i)
	}
	return l
}

func (l *leccion) actual() *ejercicio {
	if len(l.cola) == 0 {
		return nil
	}
	return l.ejs[l.cola[0]]
}

// progreso: de 0 a 1.
func (l *leccion) progreso() float64 {
	if len(l.ejs) == 0 {
		return 1
	}
	return float64(len(l.acertados)) / float64(len(l.ejs))
}

// responder apunta el resultado del ejercicio de ahora. Si se falla, vuelve al final de la cola.
func (l *leccion) responder(bien bool) {
	if len(l.cola) == 0 {
		return
	}
	i := l.cola[0]
	l.cola = l.cola[1:]
	if bien {
		l.acertados[i] = true
		l.seguidos++
		l.maxSeguidos = max(l.maxSeguidos, l.seguidos)
		return
	}
	l.fallos++
	l.seguidos = 0
	if l.vidas > 0 {
		l.vidas--
	}
	l.cola = append(l.cola, i)
}

// respuesta: lo que contestaste en un ejercicio (la última vez), para mandarlo al acabar.
type respuesta struct {
	Guess any // el número de la opción elegida o el texto
	Bien  bool
	Ms    int // lo que tardaste
}

func (l *leccion) apuntar(e *ejercicio, guess any, bien bool, ms int) {
	if l.respuestas == nil {
		l.respuestas = map[int]respuesta{}
	}
	l.respuestas[e.Indice] = respuesta{Guess: guess, Bien: bien, Ms: ms}
}

func (l *leccion) acabada() bool { return len(l.cola) == 0 }

func (l *leccion) sinVidas() bool { return l.vidas == 0 }

// ---- Comprobar respuestas --------------------------------------------------------------------------

// normalizar: en minúsculas, sin signos de puntuación y con un espacio entre palabras.
func normalizar(s string) string {
	s = strings.NewReplacer("’", "'", "‘", "'", "´", "'", "`", "'").Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// sinAcentos: lo mismo sin tildes (para perdonarlas, como Duolingo, avisando).
func sinAcentos(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// distancia: cuántas letras hay que cambiar para pasar de una a otra (Levenshtein).
func distancia(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			c := 1
			if ra[i-1] == rb[j-1] {
				c = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+c)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

// veredicto: si vale y, si vale con un fallito, cuál.
type veredicto struct {
	Bien bool
	Nota string // «Ojo con las tildes», «Tienes un error: …»
}

// comprobarFrase mira una frase escrita o hecha con fichas contra las que valen.
func comprobarFrase(respuesta string, soluciones []string, perdonar bool) veredicto {
	r := normalizar(respuesta)
	if r == "" {
		return veredicto{}
	}
	var buenas []string
	for _, s := range soluciones {
		buenas = append(buenas, expandir(s)...)
	}
	for _, s := range buenas {
		if normalizar(s) == r {
			return veredicto{Bien: true}
		}
	}
	if !perdonar {
		return veredicto{}
	}
	for _, s := range buenas {
		if sinAcentos(normalizar(s)) == sinAcentos(r) {
			return veredicto{Bien: true, Nota: "Ojo con las tildes: " + s}
		}
	}
	for _, s := range buenas {
		n := normalizar(s)
		if len([]rune(n)) >= 6 && distancia(sinAcentos(n), sinAcentos(r)) == 1 {
			return veredicto{Bien: true, Nota: "Tienes un error: " + s}
		}
	}
	return veredicto{}
}

// expandir: Duolingo escribe las variantes juntas, como «I [am/'m] a boy». Las saca todas (como
// mucho 400 por frase).
func expandir(s string) []string {
	i := strings.IndexByte(s, '[')
	if i < 0 {
		return []string{s}
	}
	j := strings.IndexByte(s[i:], ']')
	if j < 0 {
		return []string{s}
	}
	j += i
	var out []string
	for _, alt := range strings.Split(s[i+1:j], "/") {
		for _, resto := range expandir(s[j+1:]) {
			out = append(out, s[:i]+alt+resto)
			if len(out) >= 400 {
				return out
			}
		}
	}
	return out
}

// barajar devuelve los números 0..n-1 en otro orden.
func barajar(n int) []int {
	p := rand.Perm(n)
	return p
}
