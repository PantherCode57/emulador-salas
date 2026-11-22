package main

import (
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Las fotos para la DS: la DS no ve direcciones (son largas y caducan), sino claves cortas que da el
// puente al mandarle una lista; con ellas pide la foto ya hecha a su tamaño:
//
//	IMG <clave> <ancho> <alto> [c]   B <n> y n bytes: un JPEG (baseline, 4:2:0) de ese tamaño como mucho
//
// Sin «c», la foto entera, del tamaño que quepa; con «c», recortada para llenar ancho×alto.

// registro: las claves y lo que hay detrás (una dirección, o «pk:<número>» de una publicación).
type registro struct {
	mu    sync.Mutex
	n     int
	urls  map[string]string
	de    map[string]string
	orden []string
}

const maxClaves = 5000

func nuevoRegistro() *registro {
	return &registro{urls: map[string]string{}, de: map[string]string{}}
}

// clave: la de esa dirección (la misma si ya la tenía). "-" si no hay dirección.
func (r *registro) clave(u string) string {
	if u == "" {
		return "-"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.de[u]; ok {
		return c
	}
	r.n++
	c := "k" + strconv.FormatInt(int64(r.n), 36)
	r.urls[c] = u
	r.de[u] = c
	r.orden = append(r.orden, c)
	if len(r.orden) > maxClaves { // las más viejas se olvidan
		v := r.orden[0]
		r.orden = r.orden[1:]
		delete(r.de, r.urls[v])
		delete(r.urls, v)
	}
	return c
}

func (r *registro) url(c string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.urls[c]
	return u, ok
}

// cacheFotos: las últimas fotos ya hechas (la DS vuelve a pedirlas al ir y volver).
type cacheFotos struct {
	mu    sync.Mutex
	datos map[string][]byte
	orden []string
}

const maxFotos = 40

func (c *cacheFotos) leer(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.datos[k]
	return b, ok
}

func (c *cacheFotos) poner(k string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.datos == nil {
		c.datos = map[string][]byte{}
	}
	if _, ok := c.datos[k]; !ok {
		c.orden = append(c.orden, k)
	}
	c.datos[k] = b
	for len(c.orden) > maxFotos {
		delete(c.datos, c.orden[0])
		c.orden = c.orden[1:]
	}
}

var clienteFotos = &http.Client{Timeout: 25 * time.Second}

const agenteFotos = "Mozilla/5.0 (Linux; Android 10) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36"

// imagen: lo que contesta a IMG.
func (p *puente) imagen(arg string) ([]byte, error) {
	campos := strings.Fields(arg)
	if len(campos) < 3 {
		return nil, errors.New("IMG <clave> <ancho> <alto>")
	}
	w, err1 := strconv.Atoi(campos[1])
	h, err2 := strconv.Atoi(campos[2])
	if err1 != nil || err2 != nil || w < 8 || h < 8 || w > 256 || h > 192 {
		return nil, errors.New("ese tamaño no vale")
	}
	cubrir := len(campos) > 3 && campos[3] == "c"
	u, ok := p.registro().url(campos[0])
	if !ok || strings.HasPrefix(u, "pk:") {
		return nil, errors.New("esa foto ya no está")
	}
	k := fmt.Sprintf("%s %d %d %v", u, w, h, cubrir)
	if b, ok := p.fotos.leer(k); ok {
		return b, nil
	}
	var im image.Image
	if strings.HasPrefix(u, "falso:") { // cuadradas; las miniaturas, 16:9; las historias, de pie
		fw, fh := 600, 600
		if strings.Contains(u, "mini") {
			fw, fh = 640, 360
		} else if strings.Contains(u, "historia") {
			fw, fh = 360, 640
		}
		im = imagenFalsa(u, fw, fh)
	} else {
		var err error
		if im, err = bajarFoto(u); err != nil {
			return nil, err
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, escalar(im, w, h, cubrir), &jpeg.Options{Quality: 75}); err != nil {
		return nil, err
	}
	p.fotos.poner(k, b.Bytes())
	return b.Bytes(), nil
}

func bajarFoto(u string) (image.Image, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agenteFotos)
	req.Header.Set("Accept", "image/jpeg,image/png,image/*;q=0.8")
	if strings.Contains(req.URL.Host, "instagram") || strings.Contains(req.URL.Host, "fbcdn") {
		req.Header.Set("Referer", "https://www.instagram.com/")
	}
	resp, err := clienteFotos.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("la foto responde %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return nil, err
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("no se entiende la foto (%v)", err)
	}
	return im, nil
}

// escalar: la foto a w×h como mucho (cubrir: recortada para llenar w×h), haciendo la media de los
// puntos que caen en cada punto nuevo.
func escalar(im image.Image, w, h int, cubrir bool) *image.RGBA {
	b := im.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 8, 8))
	}
	// el trozo de la original que se usa y el tamaño final
	x0, y0, cw, ch := 0, 0, sw, sh
	dw, dh := w, h
	if cubrir {
		if sw*h > sh*w { // más ancha: se recortan los lados
			cw = sh * w / h
			x0 = (sw - cw) / 2
		} else {
			ch = sw * h / w
			y0 = (sh - ch) / 2
		}
	} else if sw*h > sh*w {
		dh = max(1, sh*w/sw)
	} else {
		dw = max(1, sw*h/sh)
	}
	src := aRGB(im)
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		ya := y0 + y*ch/dh
		yb := max(ya+1, y0+(y+1)*ch/dh)
		for x := 0; x < dw; x++ {
			xa := x0 + x*cw/dw
			xb := max(xa+1, x0+(x+1)*cw/dw)
			var r, g, bl, n int
			for yy := ya; yy < yb && yy < sh; yy++ {
				fila := src[yy*sw*3:]
				for xx := xa; xx < xb && xx < sw; xx++ {
					r += int(fila[xx*3])
					g += int(fila[xx*3+1])
					bl += int(fila[xx*3+2])
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}

// aRGB: los puntos de la foto en RGB (3 bytes cada uno), deprisa para las de JPEG.
func aRGB(im image.Image) []byte {
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*3)
	switch m := im.(type) {
	case *image.YCbCr:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				yi := m.YOffset(b.Min.X+x, b.Min.Y+y)
				ci := m.COffset(b.Min.X+x, b.Min.Y+y)
				r, g, bl := color.YCbCrToRGB(m.Y[yi], m.Cb[ci], m.Cr[ci])
				o := (y*w + x) * 3
				out[o], out[o+1], out[o+2] = r, g, bl
			}
		}
	case *image.RGBA:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := m.PixOffset(b.Min.X+x, b.Min.Y+y)
				o := (y*w + x) * 3
				out[o], out[o+1], out[o+2] = m.Pix[i], m.Pix[i+1], m.Pix[i+2]
			}
		}
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, bl, a := im.At(b.Min.X+x, b.Min.Y+y).RGBA()
				// lo transparente, sobre blanco
				fondo := (0xffff - a) >> 8
				o := (y*w + x) * 3
				out[o], out[o+1], out[o+2] = uint8(r>>8+fondo), uint8(g>>8+fondo), uint8(bl>>8+fondo)
			}
		}
	}
	return out
}

// imagenFalsa: una foto de mentira para las pruebas (un degradado de colores que sale del nombre,
// con un cuadrado blanco en medio).
func imagenFalsa(nombre string, w, h int) image.Image {
	f := fnv.New32a()
	_, _ = f.Write([]byte(nombre))
	s := f.Sum32()
	r0, g0, b0 := int(s&0xff), int(s>>8&0xff), int(s>>16&0xff)
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{uint8((r0 + x*255/w) / 2), uint8((g0 + y*255/h) / 2), uint8(b0), 255}
			if x > w/3 && x < 2*w/3 && y > h/3 && y < 2*h/3 {
				c = color.RGBA{255, 255, 255, 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	return im
}
