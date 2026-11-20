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
// tamaño que se pinta. Las que se ven van primero. Bajarlas y escalarlas se hace en sus hilos, nunca
// en el de la pantalla, y solo se repinta cuando llega una que está a la vista (las que se piden por
// adelantado no repintan nada).
type imagenes struct {
	app        *app
	mu         sync.Mutex
	orig       map[string]image.Image
	orden      []string // para tirar las más viejas
	listo      map[string]*image.RGBA
	pedidas    map[string]bool
	quiere     map[string][]encargo // los tamaños que se han pedido de una que aún no ha llegado
	preparando map[string]bool
	cola       chan [2]string
	prisa      chan [2]string
	escalas    chan encargo
}

// encargo: una foto a un tamaño (lo que pide pedir).
type encargo struct {
	ck, k  string
	w, h   int
	llenar bool
	radio  int
}

const maxOriginales = 80

func nuevasImagenes(a *app) *imagenes {
	m := &imagenes{app: a, orig: map[string]image.Image{}, listo: map[string]*image.RGBA{}, pedidas: map[string]bool{},
		quiere: map[string][]encargo{}, preparando: map[string]bool{},
		cola: make(chan [2]string, 256), prisa: make(chan [2]string, 64), escalas: make(chan encargo, 64)}
	for i := 0; i < 4; i++ {
		go m.trabajar()
	}
	return m
}

func (m *imagenes) redibujar() {
	if m.app != nil {
		m.app.redibujar()
	}
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
		select { // primero lo que está a la vista
		case e := <-m.escalas:
			m.preparar(e)
			m.redibujar()
			continue
		case p = <-m.prisa:
		default:
			select {
			case e := <-m.escalas:
				m.preparar(e)
				m.redibujar()
				continue
			case p = <-m.prisa:
			case p = <-m.cola:
			}
		}
		k, u := p[0], p[1]
		m.mu.Lock()
		_, ya := m.orig[k] // pedida dos veces (sin prisa y luego con prisa) y ya está
		m.mu.Unlock()
		if ya {
			continue
		}
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
		es := m.quiere[k]
		delete(m.quiere, k)
		m.mu.Unlock()
		if len(es) > 0 { // está a la vista: ya a su tamaño, y se repinta una vez
			for _, e := range es {
				m.preparar(e)
			}
			m.redibujar()
		}
	}
}

// preparar: la foto ya bajada, al tamaño del encargo (en un hilo de las fotos).
func (m *imagenes) preparar(e encargo) {
	m.mu.Lock()
	im, ok := m.orig[e.k]
	_, hecho := m.listo[e.ck]
	m.mu.Unlock()
	if ok && !hecho {
		r := hacerTamano(im, e)
		m.mu.Lock()
		if len(m.listo) > 120 { // que no crezca sin fin
			m.listo = map[string]*image.RGBA{}
		}
		m.listo[e.ck] = r
		m.mu.Unlock()
	}
	m.mu.Lock()
	delete(m.preparando, e.ck)
	m.mu.Unlock()
}

func hacerTamano(im image.Image, e encargo) *image.RGBA {
	var r *image.RGBA
	if e.llenar {
		r = escalar(im, e.w, e.h, true)
	} else {
		b := im.Bounds()
		k := min(float64(e.w)/float64(b.Dx()), float64(e.h)/float64(b.Dy()))
		r = escalar(im, max(1, int(float64(b.Dx())*k+0.5)), max(1, int(float64(b.Dy())*k+0.5)), false)
	}
	switch {
	case e.radio < 0:
		circular(r)
	case e.radio > 0:
		redondear(r, e.radio)
	}
	return r
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
// quepa) con esquinas de radio (radio < 0: redonda). nil si aún no está a ese tamaño: se baja o se
// escala en los hilos de las fotos y luego se repinta. (Sin hilos, en las pruebas, se escala aquí.)
func (m *imagenes) pedir(u string, w, h int, llenar bool, radio int) *image.RGBA {
	if u == "" || w <= 0 || h <= 0 {
		return nil
	}
	k := clave(u)
	e := encargo{ck: fmt.Sprintf("%s|%d|%d|%t|%d", k, w, h, llenar, radio), k: k, w: w, h: h, llenar: llenar, radio: radio}
	m.mu.Lock()
	if r, ok := m.listo[e.ck]; ok {
		m.mu.Unlock()
		return r
	}
	im, ok := m.orig[k]
	if !ok {
		ya := false
		for _, x := range m.quiere[k] {
			ya = ya || x.ck == e.ck
		}
		if !ya && m.quiere != nil {
			m.quiere[k] = append(m.quiere[k], e)
		}
		m.pedirSinCandado(k, u, true)
		m.mu.Unlock()
		return nil
	}
	if m.escalas != nil {
		if !m.preparando[e.ck] {
			select {
			case m.escalas <- e:
				m.preparando[e.ck] = true
			default:
			}
		}
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	r := hacerTamano(im, e)
	m.mu.Lock()
	m.listo[e.ck] = r
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
	if m.cola == nil {
		return
	}
	if m.pedidas[k] {
		if prisa && len(m.quiere[k]) == 1 { // ya estaba pedida sin prisa y ahora se ve: también con prisa
			select {
			case m.prisa <- [2]string{k, u}:
			default:
			}
		}
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
