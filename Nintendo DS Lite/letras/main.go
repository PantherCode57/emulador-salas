// letras: dibuja las letras de Go (gofont, licencia BSD, las mismas que la Pokédex de la consola) en
// varios tamaños y las escribe como un archivo C para la DS, que no puede leer letras TTF deprisa.
// Cada letra va como una máscara de 8 bits (cuánto tapa cada punto) con su posición y su avance.
//
// Uso: go run ./letras -salida generado/letras.c
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type tamano struct {
	nombre string
	ttf    []byte
	px     float64
	letras []rune // nil: todas las de juegoCompleto
}

// Lo que sale en las pantallas: ASCII, las letras del español y unos pocos signos.
func juegoCompleto() []rune {
	var r []rune
	for c := rune(32); c <= 126; c++ {
		r = append(r, c)
	}
	for c := rune(0xA1); c <= 0xFF; c++ {
		r = append(r, c)
	}
	r = append(r, []rune("♀♂‘’“”…•€←↑→↓▲►▼◄")...)
	return r
}

func main() {
	salida := flag.String("salida", "generado/letras.c", "archivo C que se escribe")
	flag.Parse()

	tamanos := []tamano{
		{"letra_peq", goregular.TTF, 10, nil},
		{"letra_normal", goregular.TTF, 12, nil},
		{"letra_negrita", gobold.TTF, 12, nil},
		{"letra_titulo", gobold.TTF, 16, nil},
		{"letra_grande", gobold.TTF, 24, nil},
		{"letra_reloj", gobold.TTF, 48, []rune("0123456789:.-")},
	}

	if err := os.MkdirAll(filepath.Dir(*salida), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*salida)
	if err != nil {
		log.Fatal(err)
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "// Generado por letras/main.go: no se toca a mano.")
	fmt.Fprintln(w, "// Letras de Go (gofont), licencia BSD: https://go.dev/blog/go-fonts")
	fmt.Fprintln(w, "#include \"../source/letras.h\"")
	total := 0
	for _, t := range tamanos {
		total += escribir(w, t)
	}
	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	log.Printf("letras: %s (%d bytes de máscaras)", *salida, total)
}

func escribir(w *bufio.Writer, t tamano) int {
	fnt, err := opentype.Parse(t.ttf)
	if err != nil {
		log.Fatal(err)
	}
	cara, err := opentype.NewFace(fnt, &opentype.FaceOptions{Size: t.px, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		log.Fatal(err)
	}
	m := cara.Metrics()
	ascenso := m.Ascent.Ceil()
	alto := (m.Ascent + m.Descent).Ceil()

	letras := t.letras
	if letras == nil {
		letras = juegoCompleto()
	}
	sort.Slice(letras, func(i, j int) bool { return letras[i] < letras[j] })

	var buf sfnt.Buffer
	var alfa []byte
	type glifo struct {
		cp            rune
		x, y, w, h, a int
		pos           int
	}
	var glifos []glifo
	for _, r := range letras {
		if i, err := fnt.GlyphIndex(&buf, r); err != nil || i == 0 {
			continue // esta letra no la tiene
		}
		dr, mascara, mp, avance, ok := cara.Glyph(fixed.P(0, ascenso), r)
		if !ok {
			continue
		}
		g := glifo{cp: r, x: dr.Min.X, y: dr.Min.Y, w: dr.Dx(), h: dr.Dy(), a: avance.Round(), pos: len(alfa)}
		if g.w > 255 || g.h > 255 || g.x < -128 || g.x > 127 || g.y < -128 || g.y > 127 {
			log.Fatalf("%s: la letra %q no cabe", t.nombre, r)
		}
		a, _ := mascara.(*image.Alpha)
		for y := 0; y < g.h; y++ {
			for x := 0; x < g.w; x++ {
				if a != nil {
					alfa = append(alfa, a.AlphaAt(mp.X+x, mp.Y+y).A)
				} else {
					_, _, _, aa := mascara.At(mp.X+x, mp.Y+y).RGBA()
					alfa = append(alfa, byte(aa>>8))
				}
			}
		}
		glifos = append(glifos, g)
	}

	fmt.Fprintf(w, "\nstatic const uint8_t alfa_%s[%d] = {", t.nombre, len(alfa)+1)
	for i, b := range alfa {
		if i%24 == 0 {
			fmt.Fprint(w, "\n\t")
		}
		fmt.Fprintf(w, "%d,", b)
	}
	fmt.Fprint(w, "0,\n};\n")
	fmt.Fprintf(w, "static const Glifo glifos_%s[%d] = {\n", t.nombre, len(glifos))
	for _, g := range glifos {
		fmt.Fprintf(w, "\t{0x%04x, %d, %d, %d, %d, %d, %d},\n", g.cp, g.x, g.y, g.w, g.h, g.a, g.pos)
	}
	fmt.Fprint(w, "};\n")
	fmt.Fprintf(w, "const Letra %s = {glifos_%s, %d, alfa_%s, %d, %d};\n", t.nombre, t.nombre, len(glifos), t.nombre, alto, ascenso)
	return len(alfa)
}
