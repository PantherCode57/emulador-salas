package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Lo que se guarda en la consola (~/.config/panther/navegador): las páginas guardadas (Y) y las
// últimas que has abierto.
type sitio struct {
	Titulo string `json:"titulo"`
	URL    string `json:"url"`
	Cuando int64  `json:"cuando,omitempty"`
}

const maxHistorial = 50

type almacen struct {
	dir        string
	guardadas  []sitio
	historial  []sitio
}

func nuevoAlmacen(dir string) *almacen {
	s := &almacen{dir: dir}
	leer := func(nombre string, v any) {
		if b, err := os.ReadFile(filepath.Join(dir, nombre)); err == nil {
			_ = json.Unmarshal(b, v)
		}
	}
	leer("guardadas.json", &s.guardadas)
	leer("historial.json", &s.historial)
	return s
}

func (s *almacen) escribir(nombre string, v any) {
	b, _ := json.MarshalIndent(v, "", " ")
	escribirAtomico(filepath.Join(s.dir, nombre), b)
}

func sinEsta(l []sitio, u string) []sitio {
	var n []sitio
	for _, x := range l {
		if x.URL != u {
			n = append(n, x)
		}
	}
	return n
}

// apuntar: la página va la primera del historial.
func (s *almacen) apuntar(titulo, u string) {
	s.historial = append([]sitio{{Titulo: titulo, URL: u, Cuando: time.Now().UnixMilli()}}, sinEsta(s.historial, u)...)
	if len(s.historial) > maxHistorial {
		s.historial = s.historial[:maxHistorial]
	}
	s.escribir("historial.json", s.historial)
}

func (s *almacen) quitarHistorial(u string) {
	s.historial = sinEsta(s.historial, u)
	s.escribir("historial.json", s.historial)
}

func (s *almacen) guardada(u string) bool {
	for _, x := range s.guardadas {
		if x.URL == u {
			return true
		}
	}
	return false
}

// cambiarGuardada guarda la página o la quita de las guardadas. Devuelve si queda guardada.
func (s *almacen) cambiarGuardada(titulo, u string) bool {
	si := !s.guardada(u)
	if si {
		s.guardadas = append([]sitio{{Titulo: titulo, URL: u, Cuando: time.Now().UnixMilli()}}, s.guardadas...)
	} else {
		s.guardadas = sinEsta(s.guardadas, u)
	}
	s.escribir("guardadas.json", s.guardadas)
	return si
}
