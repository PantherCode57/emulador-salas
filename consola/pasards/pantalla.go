package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const ancho, alto = 640, 480

var (
	colFondo1   = color.RGBA{6, 6, 9, 255}
	colFondo2   = color.RGBA{11, 18, 32, 255}
	colPanel    = color.RGBA{24, 32, 50, 255}
	colBlanco   = color.RGBA{255, 255, 255, 255}
	colSuave    = color.RGBA{200, 200, 212, 255}
	colGris     = color.RGBA{150, 150, 165, 255}
	colVioleta  = color.RGBA{196, 170, 255, 255}
	colVerde    = color.RGBA{30, 215, 96, 255}
	colRojo     = color.RGBA{240, 80, 90, 255}
	colAmarillo = color.RGBA{255, 200, 60, 255}
)

var (
	letrasUna                       sync.Once
	lTitulo, lGrande, lNormal, lPeq font.Face
)

func letras() {
	letrasUna.Do(func() {
		cara := func(ttf []byte, tam float64) font.Face {
			f, err := opentype.Parse(ttf)
			if err != nil {
				log.Fatal(err)
			}
			c, err := opentype.NewFace(f, &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				log.Fatal(err)
			}
			return c
		}
		lTitulo = cara(gobold.TTF, 22)
		lGrande = cara(gobold.TTF, 30)
		lNormal = cara(goregular.TTF, 20)
		lPeq = cara(goregular.TTF, 16)
	})
}

type lienzo struct{ img *image.RGBA }

func (l lienzo) rect(x, y, w, h int, c color.Color) {
	draw.Draw(l.img, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Over)
}

func (l lienzo) texto(f font.Face, x, y int, s string, c color.Color) int {
	d := font.Drawer{Dst: l.img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y+f.Metrics().Ascent.Ceil())}
	d.DrawString(s)
	return d.Dot.X.Round() - x
}

func medir(f font.Face, s string) int { return font.MeasureString(f, s).Round() }

// parrafo: el texto en líneas que caben en w; devuelve la y de debajo.
func (l lienzo) parrafo(f font.Face, x, y, w int, s string, c color.Color) int {
	alto := f.Metrics().Height.Ceil() + 4
	for _, trozo := range strings.Split(s, "\n") {
		linea := ""
		for _, p := range strings.Fields(trozo) {
			prueba := p
			if linea != "" {
				prueba = linea + " " + p
			}
			if medir(f, prueba) > w && linea != "" {
				l.texto(f, x, y, linea, c)
				y += alto
				linea = p
			} else {
				linea = prueba
			}
		}
		l.texto(f, x, y, linea, c)
		y += alto
	}
	return y
}

func dibujar(v vista) *image.RGBA {
	letras()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	l := lienzo{img}
	for y := 0; y < alto; y++ {
		k := float64(y) / alto
		c := color.RGBA{
			uint8(float64(colFondo1.R)*(1-k) + float64(colFondo2.R)*k),
			uint8(float64(colFondo1.G)*(1-k) + float64(colFondo2.G)*k),
			uint8(float64(colFondo1.B)*(1-k) + float64(colFondo2.B)*k), 255}
		l.rect(0, y, ancho, 1, c)
	}
	l.rect(0, 0, ancho, 44, color.RGBA{0, 0, 0, 150})
	l.texto(lTitulo, 24, 10, "Pasar a la DS", colBlanco)
	w := medir(lPeq, "Panther OS")
	l.texto(lPeq, ancho-24-w, 14, "Panther OS", colGris)
	// la línea violeta del arranque
	for x := 0; x < ancho; x++ {
		d := float64(x-ancho/2) / (ancho / 2)
		a := 1 - d*d
		if a > 0 {
			l.rect(x, 44, 1, 2, color.NRGBA{196, 170, 255, uint8(200 * a)})
		}
	}

	x, y, an := 40, 72, ancho-80
	pie := ""
	switch v.estado {
	case eInicio:
		y = l.parrafo(lGrande, x, y, an, "Copiar Panther OS a la tarjeta de la flashcard", colBlanco) + 10
		y = l.parrafo(lNormal, x, y, an, "1. Pulsa un botón. La consola cierra el menú y deja su tarjeta guardada y en solo lectura.", colSuave) + 4
		y = l.parrafo(lNormal, x, y, an, "2. Saca la tarjeta de Panther OS y mete la de la flashcard: se le copia panther.nds.", colSuave) + 4
		y = l.parrafo(lNormal, x, y, an, "3. Saca la de la flashcard y vuelve a meter la de Panther OS: la consola se reinicia sola.", colSuave) + 6
		l.parrafo(lPeq, x, y, an, "Solo se añade panther.nds: no se borra nada de la tarjeta de la flashcard.", colGris)
		l.rect(40, 376, ancho-80, 56, colPanel)
		s := "Pulsa cualquier botón para empezar"
		l.texto(lTitulo, (ancho-medir(lTitulo, s))/2, 384, s, colVioleta)
		s = fmt.Sprintf("Si no pulsas nada, se cancela en %d s", v.cuenta)
		l.texto(lPeq, (ancho-medir(lPeq, s))/2, 410, s, colGris)
	case ePreparando:
		y = l.parrafo(lGrande, x, 170, an, "Preparando la tarjeta…", colBlanco) + 8
		l.parrafo(lNormal, x, y, an, "No la saques todavía.", colAmarillo)
	case eNoSePuede:
		y = l.parrafo(lGrande, x, 150, an, "No se puede hacer ahora", colRojo) + 8
		l.parrafo(lNormal, x, y, an, v.mensaje, colSuave)
	case eSacar:
		y = l.parrafo(lGrande, x, y, an, "Saca la tarjeta de Panther OS y mete la de la flashcard", colBlanco) + 10
		y = l.parrafo(lNormal, x, y, an, "Ya puedes sacarla: está guardada y en solo lectura, no se estropea.", colVerde) + 4
		l.parrafo(lNormal, x, y, an, "Si después de meter la de la flashcard no pasa nada, pulsa un botón para que la consola mire la ranura otra vez.", colSuave)
		pie = "ranura"
	case eCopiando:
		y = l.parrafo(lGrande, x, 170, an, "Copiando panther.nds…", colBlanco) + 8
		l.parrafo(lNormal, x, y, an, "No saques la tarjeta.", colAmarillo)
	case eListo:
		y = l.parrafo(lGrande, x, y, an, "¡Listo! Panther OS ya está en la tarjeta de la flashcard", colVerde) + 10
		y = l.parrafo(lNormal, x, y, an, "Sácala y vuelve a meter la de Panther OS. La consola se reiniciará sola.", colBlanco) + 4
		l.parrafo(lNormal, x, y, an, "En la DS: mete la tarjeta en la flashcard y abre «Panther OS» (panther.nds) en su menú.", colSuave)
		pie = "ranura"
	case eError:
		y = l.parrafo(lGrande, x, y, an, "No se ha podido copiar", colRojo) + 8
		y = l.parrafo(lNormal, x, y, an, v.mensaje, colSuave) + 6
		l.parrafo(lNormal, x, y, an, "Saca esa tarjeta y vuelve a meter la de Panther OS: la consola se reiniciará sola.", colBlanco)
		pie = "ranura"
	case eReiniciando:
		l.parrafo(lGrande, x, 190, an, "Reiniciando…", colBlanco)
	}
	if pie != "" {
		l.rect(40, 360, ancho-80, 40, colPanel)
		s := "En la ranura: " + v.ranura
		l.texto(lNormal, 56, 368, s, colVioleta)
		l.parrafo(lPeq, x, 414, an, "Si se queda parada: apaga la consola con el botón, mete la tarjeta de Panther OS y enciéndela.", colGris)
	}
	return img
}

var hayFB = true

// pintar: en la pantalla (/dev/fb0, como la animación de arranque), si se puede.
func pintar(v vista) {
	if !hayFB {
		return
	}
	img := dibujar(v)
	leer := func(f string) string {
		b, _ := os.ReadFile("/sys/class/graphics/fb0/" + f)
		return strings.TrimSpace(string(b))
	}
	bpp, _ := strconv.Atoi(leer("bits_per_pixel"))
	tam := strings.Split(leer("virtual_size"), ",")
	if len(tam) != 2 || (bpp != 32 && bpp != 16) {
		hayFB = false
		log.Printf("pantalla: %d bits, %v: no sé pintar en ella", bpp, tam)
		return
	}
	w, _ := strconv.Atoi(tam[0])
	h, _ := strconv.Atoi(tam[1])
	stride, _ := strconv.Atoi(leer("stride"))
	if stride == 0 {
		stride = w * bpp / 8
	}
	fb, err := os.OpenFile("/dev/fb0", os.O_WRONLY, 0)
	if err != nil {
		hayFB = false
		log.Printf("pantalla: %v", err)
		return
	}
	defer fb.Close()
	_ = os.WriteFile("/sys/class/graphics/fbcon/cursor_blink", []byte("0"), 0)
	_ = os.WriteFile("/sys/class/graphics/fb0/blank", []byte("0"), 0)
	fila := make([]byte, stride)
	for yy := 0; yy < h && yy < alto; yy++ {
		for xx := 0; xx < w && xx < ancho; xx++ {
			i := img.PixOffset(xx, yy)
			r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
			if bpp == 32 {
				fila[xx*4], fila[xx*4+1], fila[xx*4+2], fila[xx*4+3] = b, g, r, 0xff
			} else {
				p := uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
				fila[xx*2], fila[xx*2+1] = byte(p), byte(p>>8)
			}
		}
		_, _ = fb.WriteAt(fila, int64(yy*stride))
	}
}

// pruebas: todas las pantallas en PNG (las mira consola.yml).
func pruebas(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	vistas := map[string]vista{
		"1-inicio":      {estado: eInicio, cuenta: 27},
		"2-preparando":  {estado: ePreparando},
		"3-sacar":       {estado: eSacar, ranura: "la tarjeta de Panther OS"},
		"4-sacar-vacia": {estado: eSacar, ranura: "vacía"},
		"5-copiando":    {estado: eCopiando, ranura: "otra tarjeta (8 GB)"},
		"6-listo":       {estado: eListo, ranura: "otra tarjeta (8 GB)"},
		"7-error":       {estado: eError, ranura: "otra tarjeta (8 GB)", mensaje: "no se puede abrir esa tarjeta: tiene que ser FAT32, como la piden las flashcards"},
		"8-no-se-puede": {estado: eNoSePuede, mensaje: "no se puede quitar la memoria de intercambio: exit status 255"},
		"9-reiniciando": {estado: eReiniciando},
	}
	for n, v := range vistas {
		f, err := os.Create(filepath.Join(dir, n+".png"))
		if err != nil {
			return err
		}
		err = png.Encode(f, dibujar(v))
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
