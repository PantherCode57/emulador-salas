// Animación de arranque de Panther OS para la consola (dArkOS4Clone en la XF35H).
//
// Dibuja directamente en /dev/fb0, a 60 imágenes por segundo, antes de que se abra EmulationStation:
// una línea que crece desde el centro difuminándose por los lados, el título que aparece subiendo y,
// debajo, el texto pequeño. Las letras vienen ya dibujadas (máscaras PNG en escala de grises) en la
// carpeta de datos, con sus posiciones en posiciones.txt; así el programa no lleva ningún texto.
//
// Uso: panther-arranque [-datos /usr/local/share/panther/arranque] [-fb /dev/fb0] [-prueba salida.png]
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ancho, alto = 640, 480
	duracion    = 2200 * time.Millisecond
	quieto      = 700 * time.Millisecond
)

var (
	fondo   = [3]float64{6, 6, 9}
	blanco  = [3]float64{245, 245, 250}
	gris    = [3]float64{150, 150, 162}
	violeta = [3]float64{196, 170, 255}
)

type mascara struct {
	w, h int
	a    []float64 // 0..1
	x, y int
}

func cargarMascara(ruta string) (*mascara, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	m := &mascara{w: b.Dx(), h: b.Dy(), a: make([]float64, b.Dx()*b.Dy())}
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			r, _, _, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			m.a[y*m.w+x] = float64(r) / 0xffff
		}
	}
	return m, nil
}

type escena struct {
	titulo, sub *mascara
	yLinea      int
	largoLinea  float64
}

func cargar(dir string) (*escena, error) {
	e := &escena{}
	var err error
	if e.titulo, err = cargarMascara(filepath.Join(dir, "titulo.png")); err != nil {
		return nil, err
	}
	if e.sub, err = cargarMascara(filepath.Join(dir, "subtitulo.png")); err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "posiciones.txt"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		c := strings.Fields(s.Text())
		if len(c) != 3 {
			continue
		}
		a, _ := strconv.Atoi(c[1])
		b, _ := strconv.Atoi(c[2])
		switch c[0] {
		case "titulo":
			e.titulo.x, e.titulo.y = a, b
		case "subtitulo":
			e.sub.x, e.sub.y = a, b
		case "linea":
			e.yLinea, e.largoLinea = a, float64(b)
		}
	}
	return e, nil
}

func suave(t float64) float64 {
	t = math.Min(math.Max(t, 0), 1)
	return t * t * (3 - 2*t)
}

func salidaCubica(t float64) float64 {
	t = math.Min(math.Max(t, 0), 1)
	return 1 - math.Pow(1-t, 3)
}

func tramo(t, a, b float64) float64 { return (t - a) / (b - a) }

// lienzo en coma flotante (RGB) para mezclar sin perder suavidad.
type lienzo struct{ px []float64 }

func (l *lienzo) limpiar() {
	for i := 0; i < len(l.px); i += 3 {
		l.px[i], l.px[i+1], l.px[i+2] = fondo[0], fondo[1], fondo[2]
	}
}

func (l *lienzo) mezclar(x, y int, c [3]float64, a float64) {
	if a <= 0 || x < 0 || y < 0 || x >= ancho || y >= alto {
		return
	}
	i := (y*ancho + x) * 3
	for k := 0; k < 3; k++ {
		l.px[i+k] = l.px[i+k]*(1-a) + c[k]*a
	}
}

// pegar pone una máscara con transparencia y desplazamiento vertical con decimales.
func (l *lienzo) pegar(m *mascara, dy float64, c [3]float64, alfa float64) {
	if alfa <= 0 {
		return
	}
	y0 := int(math.Floor(dy))
	frac := dy - float64(y0)
	for y := 0; y <= m.h; y++ {
		for x := 0; x < m.w; x++ {
			var a float64
			if y < m.h {
				a = m.a[y*m.w+x] * (1 - frac)
			}
			if y > 0 {
				a += m.a[(y-1)*m.w+x] * frac
			}
			l.mezclar(m.x+x, m.y+y0+y, c, a*alfa)
		}
	}
}

func (l *lienzo) linea(y int, largo, brillo float64) {
	if largo < 1 {
		return
	}
	for x := 0; x < ancho; x++ {
		u := 1 - math.Abs(float64(x)-(ancho-1)/2)/(largo/2)
		if u <= 0 {
			continue
		}
		p := math.Pow(u, 1.6) * brillo
		l.mezclar(x, y-1, violeta, p*0.35)
		l.mezclar(x, y, violeta, p)
		l.mezclar(x, y+1, violeta, p*0.35)
	}
}

func (e *escena) dibujar(l *lienzo, t float64) {
	l.limpiar()
	l.linea(e.yLinea, e.largoLinea*salidaCubica(tramo(t, 0, 0.9)), suave(tramo(t, 0, 0.25)))
	p := tramo(t, 0.35, 1.25)
	l.pegar(e.titulo, 10*(1-salidaCubica(p)), blanco, suave(p))
	q := tramo(t, 1.0, 1.8)
	l.pegar(e.sub, 6*(1-salidaCubica(q)), gris, suave(q))
}

// aBGRX pasa el lienzo al formato de la pantalla (32 bits, little endian: B, G, R, X).
func (l *lienzo) aBGRX(out []byte) {
	for i, j := 0, 0; i < len(l.px); i, j = i+3, j+4 {
		out[j] = byte(l.px[i+2] + 0.5)
		out[j+1] = byte(l.px[i+1] + 0.5)
		out[j+2] = byte(l.px[i] + 0.5)
		out[j+3] = 0xff
	}
}

func main() {
	datos := flag.String("datos", "/usr/local/share/panther/arranque", "carpeta con titulo.png, subtitulo.png y posiciones.txt")
	dispositivo := flag.String("fb", "/dev/fb0", "pantalla")
	prueba := flag.String("prueba", "", "en vez de usar la pantalla, guarda el último fotograma en este PNG")
	flag.Parse()

	e, err := cargar(*datos)
	if err != nil {
		fmt.Fprintln(os.Stderr, "panther-arranque:", err)
		os.Exit(1)
	}
	l := &lienzo{px: make([]float64, ancho*alto*3)}
	marco := make([]byte, ancho*alto*4)

	if *prueba != "" {
		e.dibujar(l, duracion.Seconds())
		img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
		for i, j := 0, 0; i < len(l.px); i, j = i+3, j+4 {
			img.Pix[j], img.Pix[j+1], img.Pix[j+2], img.Pix[j+3] = byte(l.px[i]+0.5), byte(l.px[i+1]+0.5), byte(l.px[i+2]+0.5), 0xff
		}
		f, err := os.Create(*prueba)
		if err == nil {
			_ = png.Encode(f, img)
			f.Close()
		}
		return
	}

	fb, err := os.OpenFile(*dispositivo, os.O_WRONLY, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "panther-arranque:", err)
		os.Exit(1)
	}
	defer fb.Close()
	// Sin cursor parpadeando ni texto de la consola encima.
	_ = os.WriteFile("/sys/class/graphics/fbcon/cursor_blink", []byte("0"), 0)
	if tty, err := os.OpenFile("/dev/tty1", os.O_WRONLY, 0); err == nil {
		_, _ = tty.WriteString("\033[?25l\033[2J")
		tty.Close()
	}

	inicio := time.Now()
	paso := time.Second / 60
	for {
		t := time.Since(inicio)
		if t > duracion+quieto {
			break
		}
		e.dibujar(l, math.Min(t.Seconds(), duracion.Seconds()))
		l.aBGRX(marco)
		esperarVsync(fb.Fd())
		_, _ = fb.WriteAt(marco, 0)
		if r := paso - time.Since(inicio.Add(t)); r > 0 {
			time.Sleep(r)
		}
	}
}
