package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Lo que se guarda en la consola:
//   - ~/.config/panther/youtube/historial.json: lo último que has visto y por dónde ibas;
//   - /roms/videos/youtube/<id>.mp4 con <id>.json (sus datos) y <id>.jpg (su miniatura): lo descargado.
type Visto struct {
	Video
	Pos    int   `json:"pos"`    // segundos
	Cuando int64 `json:"cuando"` // unix ms
}

type Descargado struct {
	Video
	FPS      float64 `json:"fps,omitempty"`
	Vertical bool    `json:"vertical,omitempty"`
	Archivo  string  `json:"-"`
	Bytes    int64   `json:"bytes,omitempty"`
	Cuando   int64   `json:"cuando"`
}

const maxHistorial = 60

type almacen struct {
	dir, videos string
	historial   []Visto
	descargados []Descargado
}

func nuevoAlmacen(dir, videos string) *almacen {
	s := &almacen{dir: dir, videos: videos}
	if b, err := os.ReadFile(filepath.Join(dir, "historial.json")); err == nil {
		_ = json.Unmarshal(b, &s.historial)
	}
	s.cargarDescargados()
	return s
}

func ahoraMs() int64 { return time.Now().UnixMilli() }

func (s *almacen) guardarHistorial() {
	b, _ := json.MarshalIndent(s.historial, "", " ")
	escribirAtomico(filepath.Join(s.dir, "historial.json"), b)
}

// apuntar pone el vídeo el primero del historial, con por dónde va.
func (s *almacen) apuntar(v Video, pos int) {
	n := []Visto{{Video: v, Pos: pos, Cuando: ahoraMs()}}
	for _, h := range s.historial {
		if h.ID != v.ID {
			n = append(n, h)
		}
	}
	if len(n) > maxHistorial {
		n = n[:maxHistorial]
	}
	s.historial = n
	s.guardarHistorial()
}

// posicion: por dónde ibas en ese vídeo (0 si no lo has visto o lo acabaste).
func (s *almacen) posicion(id string) int {
	for _, h := range s.historial {
		if h.ID == id {
			return h.Pos
		}
	}
	return 0
}

func (s *almacen) quitarHistorial(id string) {
	var n []Visto
	for _, h := range s.historial {
		if h.ID != id {
			n = append(n, h)
		}
	}
	s.historial = n
	s.guardarHistorial()
}

func (s *almacen) cargarDescargados() {
	s.descargados = nil
	m, _ := filepath.Glob(filepath.Join(s.videos, "*.json"))
	for _, j := range m {
		b, err := os.ReadFile(j)
		if err != nil {
			continue
		}
		var d Descargado
		if json.Unmarshal(b, &d) != nil || d.ID == "" {
			continue
		}
		d.Archivo = strings.TrimSuffix(j, ".json") + ".mp4"
		if st, err := os.Stat(d.Archivo); err != nil || st.Size() == 0 {
			continue
		}
		s.descargados = append(s.descargados, d)
	}
	sort.Slice(s.descargados, func(i, j int) bool { return s.descargados[i].Cuando > s.descargados[j].Cuando })
}

func (s *almacen) descargado(id string) *Descargado {
	for i := range s.descargados {
		if s.descargados[i].ID == id {
			return &s.descargados[i]
		}
	}
	return nil
}

func (s *almacen) rutaVideo(id string) string { return filepath.Join(s.videos, id+".mp4") }

// borrarDescarga quita el vídeo descargado (y sus datos) de la tarjeta.
func (s *almacen) borrarDescarga(id string) error {
	base := filepath.Join(s.videos, id)
	err := os.Remove(base + ".mp4")
	_ = os.Remove(base + ".json")
	_ = os.Remove(base + ".jpg")
	s.cargarDescargados()
	return err
}

func guardarJSON(ruta string, v any) {
	b, _ := json.MarshalIndent(v, "", " ")
	_ = os.MkdirAll(filepath.Dir(ruta), 0o755)
	escribirAtomico(ruta, b)
}
