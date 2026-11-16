package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Portadas de los álbumes y de las listas: se bajan una vez a la consola (portadas/) y se guardan en
// memoria ya escaladas, con las esquinas redondas y su sombra, para cada tamaño que se pinta.
type portada struct {
	img *image.RGBA
	sb  *image.Alpha
}

type portadas struct {
	app     *app
	dir     string
	mu      sync.Mutex
	listo   map[string]*portada // url|tamaño → lista para pintar
	orig    map[string]image.Image
	pedidas map[string]bool
	cola    chan string
}

func nuevasPortadas(a *app, dir string) *portadas {
	_ = os.MkdirAll(dir, 0o700)
	p := &portadas{app: a, dir: dir, listo: map[string]*portada{}, orig: map[string]image.Image{},
		pedidas: map[string]bool{}, cola: make(chan string, 512)}
	for i := 0; i < 3; i++ {
		go p.trabajar()
	}
	return p
}

func (p *portadas) archivo(url string) string {
	h := sha1.Sum([]byte(url))
	return filepath.Join(p.dir, hex.EncodeToString(h[:10])+".jpg")
}

var clienteImagenes = &http.Client{Timeout: 20 * time.Second}

func (p *portadas) trabajar() {
	for url := range p.cola {
		ruta := p.archivo(url)
		im := abrirImagen(ruta)
		if im == nil && hayRed() {
			if resp, err := clienteImagenes.Get(url); err == nil {
				if resp.StatusCode == 200 {
					if f, err := os.Create(ruta + ".tmp"); err == nil {
						_, err = io.Copy(f, resp.Body)
						f.Close()
						if err == nil {
							_ = os.Rename(ruta+".tmp", ruta)
							im = abrirImagen(ruta)
						}
					}
				}
				resp.Body.Close()
			}
		}
		if im == nil {
			continue
		}
		// Se guarda ya reducida (las portadas de Spotify vienen a 300 o 640 px).
		if b := im.Bounds(); b.Dx() > 320 {
			im = escalar(im, 320, 320*b.Dy()/b.Dx(), false)
		}
		p.mu.Lock()
		p.orig[url] = im
		p.mu.Unlock()
		p.app.hacer(func() {}) // redibujar
	}
}

// pedir: la portada de url a lado×lado px (con sombra si se pide). nil si aún no está.
func (p *portadas) pedir(url string, lado int, conSombra bool) *portada {
	if url == "" {
		return nil
	}
	clave := fmt.Sprintf("%s|%d|%v", url, lado, conSombra)
	p.mu.Lock()
	if r, ok := p.listo[clave]; ok {
		p.mu.Unlock()
		return r
	}
	im, ok := p.orig[url]
	if !ok {
		if !p.pedidas[url] {
			p.pedidas[url] = true
			select {
			case p.cola <- url:
			default:
				p.pedidas[url] = false
			}
		}
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()
	r := preparar(escalar(im, lado, lado, true), lado, conSombra)
	p.mu.Lock()
	if len(p.listo) > 200 { // que no crezca sin fin
		p.listo = map[string]*portada{}
	}
	p.listo[clave] = r
	p.mu.Unlock()
	return r
}

func radioDe(lado int) int { return max(6, int(math.Round(float64(lado)*0.095))) }

func preparar(img *image.RGBA, lado int, conSombra bool) *portada {
	redondear(img, radioDe(lado))
	r := &portada{img: img}
	if conSombra {
		r.sb = sombra(img, 6, 0.6)
	}
	return r
}

// Portadas dibujadas: «Tus me gusta», «Descargadas» y las que no tienen imagen.
var cacheDibujadas = map[string]*portada{}

func portadaDibujada(f *fuentes, clave, icono string, tono float64, lado int, conSombra bool) *portada {
	k := fmt.Sprintf("%s|%d|%v", clave, lado, conSombra)
	if r, ok := cacheDibujadas[k]; ok {
		return r
	}
	img := image.NewRGBA(image.Rect(0, 0, lado, lado))
	for y := 0; y < lado; y++ {
		for x := 0; x < lado; x++ {
			t := float64(x+y) / float64(2*lado)
			c := hsv(math.Mod(tono+0.12*t, 1), 0.65, 0.95-0.45*t)
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, 255
		}
	}
	ic := f.iconoDe(float64(lado) * 0.38)
	l := &lienzo{img: img, f: f}
	l.centrado(img.Bounds(), icono, ic, colTexto)
	r := preparar(img, lado, conSombra)
	cacheDibujadas[k] = r
	return r
}

func hsv(h, s, v float64) color.RGBA {
	i := math.Floor(h * 6)
	f := h*6 - i
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}
