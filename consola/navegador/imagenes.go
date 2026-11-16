package main

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"sync"

	_ "golang.org/x/image/webp"
)

// Imágenes de las páginas: se bajan cuando van a verse (tres a la vez), se reducen al ancho de la
// pantalla y se guardan en memoria (las últimas 60).
type imagenes struct {
	app     *app
	mu      sync.Mutex
	listas  map[string]*image.RGBA
	orden   []string
	pedidas map[string]bool
	malas   map[string]bool
	cola    chan string
}

func nuevasImagenes(a *app) *imagenes {
	im := &imagenes{app: a, listas: map[string]*image.RGBA{}, pedidas: map[string]bool{}, malas: map[string]bool{},
		cola: make(chan string, 256)}
	for i := 0; i < 3; i++ {
		go im.trabajar()
	}
	return im
}

func (im *imagenes) trabajar() {
	for src := range im.cola {
		img := bajarImagen(src)
		im.mu.Lock()
		delete(im.pedidas, src)
		if img == nil {
			im.malas[src] = true
		} else {
			im.listas[src] = img
			im.orden = append(im.orden, src)
			if len(im.orden) > 60 {
				delete(im.listas, im.orden[0])
				im.orden = im.orden[1:]
			}
		}
		im.mu.Unlock()
		if img != nil {
			im.app.hacer(func() { im.app.imagenLista() })
		}
	}
}

func bajarImagen(src string) *image.RGBA {
	resp, err := pedirWeb(src)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	dec, _, err := image.Decode(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil
	}
	b := dec.Bounds()
	if b.Dx() < 40 || b.Dy() < 40 {
		return nil
	}
	w := min(b.Dx(), anchoTexto())
	h := b.Dy() * w / b.Dx()
	if h > maxAltImg {
		h, w = maxAltImg, b.Dx()*maxAltImg/b.Dy()
	}
	return escalar(dec, max(w, 1), max(h, 1), false)
}

// tam: lo que mide la imagen ya bajada (0, 0 si aún no está).
func (im *imagenes) tam(src string) (int, int) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if r, ok := im.listas[src]; ok {
		return r.Bounds().Dx(), r.Bounds().Dy()
	}
	if im.malas[src] {
		return -1, -1 // no se pudo: ni hueco
	}
	return 0, 0
}

// pedir: la imagen si ya está; si no, se pide (y nil).
func (im *imagenes) pedir(src string) (*image.RGBA, bool) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if r, ok := im.listas[src]; ok {
		return r, true
	}
	if im.malas[src] {
		return nil, false
	}
	if !im.pedidas[src] {
		select {
		case im.cola <- src:
			im.pedidas[src] = true
		default:
		}
	}
	return nil, true
}

// olvidarCola: al cambiar de página, fuera lo que aún no se había empezado a bajar.
func (im *imagenes) olvidarCola() {
	for {
		select {
		case src := <-im.cola:
			im.mu.Lock()
			delete(im.pedidas, src)
			im.mu.Unlock()
		default:
			return
		}
	}
}
