package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"net/http"
	"net/url"
	"sync"
)

// Fotos de Instagram (las de las publicaciones, las historias y las caras de la gente): se bajan
// solo a la memoria (nada a la tarjeta), se reducen a 480 px y se guardan ya escaladas para cada
// tamaño que se pinta. Las últimas pedidas van primero.
type imagenes struct {
	app     *app
	mu      sync.Mutex
	orig    map[string]image.Image
	orden   []string // para tirar las más viejas
	listo   map[string]*image.RGBA
	pedidas map[string]bool
	cola    chan [2]string
	prisa   chan [2]string
}

const maxOriginales = 50

func nuevasImagenes(a *app) *imagenes {
	m := &imagenes{app: a, orig: map[string]image.Image{}, listo: map[string]*image.RGBA{}, pedidas: map[string]bool{},
		cola: make(chan [2]string, 256), prisa: make(chan [2]string, 64)}
	for i := 0; i < 3; i++ {
		go m.trabajar()
	}
	return m
}

// clave: la dirección sin lo de después de «?» (las firmas cambian, la foto no).
func clave(u string) string {
	if p, err := url.Parse(u); err == nil {
		u = p.Host + p.Path
	}
	h := sha1.Sum([]byte(u))
	return hex.EncodeToString(h[:8])
}

func (m *imagenes) trabajar() {
	for {
		var p [2]string
		select {
		case p = <-m.prisa:
		default:
			select {
			case p = <-m.prisa:
			case p = <-m.cola:
			}
		}
		k, u := p[0], p[1]
		im, err := bajarImagen(u)
		if err != nil {
			logf("foto: %v", err)
			m.mu.Lock()
			delete(m.pedidas, k) // otra vez más adelante
			m.mu.Unlock()
			continue
		}
		if b := im.Bounds(); b.Dx() > 480 || b.Dy() > 480 {
			if b.Dx() >= b.Dy() {
				im = escalar(im, 480, max(1, 480*b.Dy()/b.Dx()), false)
			} else {
				im = escalar(im, max(1, 480*b.Dx()/b.Dy()), 480, false)
			}
		}
		m.mu.Lock()
		m.orig[k] = im
		m.orden = append(m.orden, k)
		for len(m.orden) > maxOriginales {
			viejo := m.orden[0]
			m.orden = m.orden[1:]
			delete(m.orig, viejo)
			delete(m.pedidas, viejo)
		}
		m.mu.Unlock()
		if m.app != nil {
			m.app.hacer(func() {}) // redibujar
		}
	}
}

func bajarImagen(u string) (image.Image, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agenteNavegador)
	req.Header.Set("Accept", "image/webp,image/jpeg,image/*")
	req.Header.Set("Referer", urlWeb+"/")
	resp, err := clienteImagen.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("responde %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	im, _, err := image.Decode(bytes.NewReader(b))
	return im, err
}

// pedir: la foto de «u» a w×h (recortada para llenarlo si llenar; si no, entera y del tamaño que
// quepa) con esquinas de radio (radio < 0: redonda). nil si aún no está (y se pide).
func (m *imagenes) pedir(u string, w, h int, llenar bool, radio int) *image.RGBA {
	if u == "" || w <= 0 || h <= 0 {
		return nil
	}
	k := clave(u)
	ck := fmt.Sprintf("%s|%d|%d|%t|%d", k, w, h, llenar, radio)
	m.mu.Lock()
	if r, ok := m.listo[ck]; ok {
		m.mu.Unlock()
		return r
	}
	im, ok := m.orig[k]
	if !ok {
		m.pedirSinCandado(k, u, true)
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	var r *image.RGBA
	if llenar {
		r = escalar(im, w, h, true)
	} else {
		b := im.Bounds()
		k := min(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()))
		r = escalar(im, max(1, int(float64(b.Dx())*k+0.5)), max(1, int(float64(b.Dy())*k+0.5)), false)
	}
	switch {
	case radio < 0:
		circular(r)
	case radio > 0:
		redondear(r, radio)
	}
	m.mu.Lock()
	if len(m.listo) > 90 { // que no crezca sin fin
		m.listo = map[string]*image.RGBA{}
	}
	m.listo[ck] = r
	m.mu.Unlock()
	return r
}

// adelantar pide una foto que se verá pronto (sin prisa).
func (m *imagenes) adelantar(u string) {
	if u == "" {
		return
	}
	k := clave(u)
	m.mu.Lock()
	if _, ok := m.orig[k]; !ok {
		m.pedirSinCandado(k, u, false)
	}
	m.mu.Unlock()
}

func (m *imagenes) pedirSinCandado(k, u string, prisa bool) {
	if m.pedidas[k] || m.cola == nil {
		return
	}
	m.pedidas[k] = true
	c := m.cola
	if prisa {
		c = m.prisa
	}
	select {
	case c <- [2]string{k, u}:
	default:
		delete(m.pedidas, k)
	}
}

// circular deja solo el círculo de dentro (con borde suave).
func circular(im *image.RGBA) {
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	cx, cy, r := float64(w)/2, float64(h)/2, float64(min(w, h))/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := r - math.Sqrt(dx*dx+dy*dy)
			if d >= 1 {
				continue
			}
			a := max(0, d)
			i := im.PixOffset(x, y)
			for k := 0; k < 4; k++ {
				im.Pix[i+k] = uint8(float64(im.Pix[i+k]) * a)
			}
		}
	}
}

// inicial: un círculo de color con la primera letra (mientras llega la foto, o si no tiene).
func (a *app) inicial(cx, cy, radio int, nombre string) {
	l := a.l
	tonos := []color.RGBA{colRosa, colNaranja, colMorado, {0x40, 0x5d, 0xe6, 0xff}, {0x2e, 0x9e, 0x8f, 0xff}}
	n := 0
	for _, r := range nombre {
		n += int(r)
	}
	l.circulo(cx, cy, radio, tonos[n%len(tonos)])
	ini := "?"
	if rs := []rune(l.f.limpiar(nombre)); len(rs) > 0 {
		ini = string(rs[0])
	}
	cara := l.f.nombre
	if radio > 30 {
		cara = l.f.titulo
	} else if radio < 15 {
		cara = l.f.mini
	}
	l.centrado(image.Rect(cx-radio, cy-radio, cx+radio, cy+radio), ini, cara, colTexto)
}

// cara: la foto de perfil redonda en (cx, cy).
func (a *app) cara(cx, cy, radio int, foto, nombre string) {
	if im := a.img.pedir(foto, radio*2, radio*2, true, -1); im != nil {
		a.l.pegar(im, nil, cx-radio, cy-radio)
		return
	}
	a.inicial(cx, cy, radio, nombre)
}
