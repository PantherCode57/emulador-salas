package main

import (
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Miniaturas de los vídeos (las de 320×180 de YouTube): se bajan una vez a la consola (miniaturas/)
// y se guardan en memoria ya escaladas y con las esquinas redondas, para cada tamaño que se pinta.
// Las de los vídeos descargados van junto al vídeo, para verlas sin internet.
type miniaturas struct {
	app     *app
	dir     string
	mu      sync.Mutex
	listo   map[string]*image.RGBA // id|ancho|alto
	orig    map[string]image.Image
	pedidas map[string]bool
	cola    chan [2]string // clave y dirección
}

func nuevasMiniaturas(a *app, dir string) *miniaturas {
	_ = os.MkdirAll(dir, 0o700)
	m := &miniaturas{app: a, dir: dir, listo: map[string]*image.RGBA{}, orig: map[string]image.Image{},
		pedidas: map[string]bool{}, cola: make(chan [2]string, 512)}
	for i := 0; i < 3; i++ {
		go m.trabajar()
	}
	return m
}

func (m *miniaturas) archivo(clave string) string { return filepath.Join(m.dir, clave+".jpg") }

var clienteImagenes = &http.Client{Timeout: 20 * time.Second}

// bajarMiniatura deja la imagen de «url» en «ruta» (si aún no está).
func bajarMiniatura(url, ruta string) error {
	if _, err := os.Stat(ruta); err == nil {
		return nil
	}
	resp, err := clienteImagenes.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("miniatura: %d", resp.StatusCode)
	}
	f, err := os.Create(ruta + ".tmp")
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		_ = os.Remove(ruta + ".tmp")
		return err
	}
	return os.Rename(ruta+".tmp", ruta)
}

func (m *miniaturas) trabajar() {
	for p := range m.cola {
		id, url := p[0], p[1]
		var im image.Image
		for _, r := range []string{filepath.Join(carpetaVideos, strings.TrimSuffix(id, "_v")+".jpg"), m.archivo(id)} {
			if im = abrirImagen(r); im != nil {
				break
			}
		}
		if im == nil && hayRed() && bajarMiniatura(url, m.archivo(id)) == nil {
			im = abrirImagen(m.archivo(id))
		}
		if im == nil {
			m.mu.Lock()
			delete(m.pedidas, id) // otra vez más adelante (quizá vuelva internet)
			m.mu.Unlock()
			continue
		}
		if b := im.Bounds(); b.Dx() > 320 {
			im = escalar(im, 320, 320*b.Dy()/b.Dx(), false)
		}
		m.mu.Lock()
		m.orig[id] = im
		m.mu.Unlock()
		m.app.hacer(func() {}) // redibujar
	}
}

// grande: la miniatura sin recortar las esquinas, a ancho×alto (para el fondo mientras carga).
func (m *miniaturas) grande(v Video, w, h int) *image.RGBA {
	return m.pedirRadio(v, w, h, 0)
}

// pedir: la miniatura de ese vídeo a w×h px (recortada para llenarlo). nil si aún no está.
func (m *miniaturas) pedir(v Video, w, h int) *image.RGBA {
	return m.pedirRadio(v, w, h, max(6, h/9))
}

func (m *miniaturas) pedirRadio(v Video, w, h, radio int) *image.RGBA {
	id := v.claveMini()
	if v.ID == "" {
		return nil
	}
	clave := fmt.Sprintf("%s|%d|%d|%d", id, w, h, radio)
	m.mu.Lock()
	if r, ok := m.listo[clave]; ok {
		m.mu.Unlock()
		return r
	}
	im, ok := m.orig[id]
	if !ok {
		if !m.pedidas[id] {
			m.pedidas[id] = true
			select {
			case m.cola <- [2]string{id, v.miniatura()}:
			default:
				delete(m.pedidas, id)
			}
		}
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()
	r := escalar(im, w, h, true)
	if radio > 0 {
		redondear(r, radio)
	}
	m.mu.Lock()
	if len(m.listo) > 150 { // que no crezca sin fin
		m.listo = map[string]*image.RGBA{}
	}
	m.listo[clave] = r
	m.mu.Unlock()
	return r
}
