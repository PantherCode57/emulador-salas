package main

import (
	"image"
	"os"
	"strconv"
	"strings"
)

// pintarFB pone una imagen directamente en la pantalla (/dev/fb0) mientras nadie la está usando:
// justo después de que EmulationStation la suelte (antes de que arranque RetroArch) y al cerrar
// RetroArch (hasta que vuelve el menú). Así no se ve negro ni el texto de la consola entre medias.
// Si la pantalla no es de 32 bits y del mismo tamaño, no hace nada.
func pintarFB(img *image.RGBA) {
	leer := func(f string) string {
		b, _ := os.ReadFile("/sys/class/graphics/fb0/" + f)
		return strings.TrimSpace(string(b))
	}
	if leer("bits_per_pixel") != "32" {
		return
	}
	tam := strings.Split(leer("virtual_size"), ",")
	if len(tam) != 2 {
		return
	}
	w, _ := strconv.Atoi(tam[0])
	h, _ := strconv.Atoi(tam[1])
	stride, _ := strconv.Atoi(leer("stride"))
	b := img.Bounds()
	if w < b.Dx() || h < b.Dy() || stride < b.Dx()*4 {
		return
	}
	fb, err := os.OpenFile("/dev/fb0", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer fb.Close()
	// Sin cursor parpadeando encima.
	_ = os.WriteFile("/sys/class/graphics/fbcon/cursor_blink", []byte("0"), 0)
	fila := make([]byte, stride)
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			fila[x*4], fila[x*4+1], fila[x*4+2], fila[x*4+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], 0xff
		}
		_, _ = fb.WriteAt(fila, int64(y*stride))
	}
}
