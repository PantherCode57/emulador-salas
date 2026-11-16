package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Lo que se guarda en la consola para poder usar Spotin sin internet: la lista de listas, las
// canciones de cada una (según se van abriendo) y las canciones descargadas.

type Cancion struct {
	ID         string   `json:"id"` // id de Spotify
	Nombre     string   `json:"nombre"`
	Artistas   []string `json:"artistas"`
	Album      string   `json:"album,omitempty"`
	Portada    string   `json:"portada,omitempty"` // dirección de la portada del álbum
	DuracionMs int      `json:"ms"`
	Explicita  bool     `json:"explicita,omitempty"`
	Video      string   `json:"video,omitempty"` // vídeo de YouTube que se encontró para ella
}

func (c *Cancion) artistas() string { return strings.Join(c.Artistas, ", ") }
func (c *Cancion) duracion() time.Duration {
	return time.Duration(c.DuracionMs) * time.Millisecond
}

// Tipos de lista.
const (
	tipoLista       = "playlist"
	tipoAlbum       = "album"
	tipoMeGusta     = "megusta"
	tipoDescargadas = "descargadas"
	tipoBusqueda    = "busqueda"
)

type Lista struct {
	ID      string `json:"id"`
	Nombre  string `json:"nombre"`
	Dueno   string `json:"dueno,omitempty"`
	Portada string `json:"portada,omitempty"`
	Total   int    `json:"total"`
	Tipo    string `json:"tipo"`
}

type almacen struct {
	dir    string // ~/.config/panther/spotin
	musica string // /roms/musica/spotin
	mu     sync.Mutex

	Listas      []Lista              `json:"listas"`
	Actualizada time.Time            `json:"actualizada"`
	canciones   map[string][]Cancion // id de lista → canciones
	descargas   map[string]descarga  // id de canción → archivo
}

type descarga struct {
	Archivo string    `json:"archivo"`
	Cancion Cancion   `json:"cancion"`
	Cuando  time.Time `json:"cuando"`
}

func nuevoAlmacen(dir, musica string) *almacen {
	a := &almacen{dir: dir, musica: musica, canciones: map[string][]Cancion{}, descargas: map[string]descarga{}}
	_ = os.MkdirAll(filepath.Join(dir, "listas"), 0o700)
	_ = os.MkdirAll(musica, 0o755)
	if b, err := os.ReadFile(filepath.Join(dir, "biblioteca.json")); err == nil {
		_ = json.Unmarshal(b, a)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "descargas.json")); err == nil {
		_ = json.Unmarshal(b, &a.descargas)
	}
	// Fuera las descargas cuyo archivo ya no está.
	for id, d := range a.descargas {
		if _, err := os.Stat(d.Archivo); err != nil {
			delete(a.descargas, id)
		}
	}
	return a
}

func nombreSeguro(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

func (a *almacen) guardarBiblioteca(listas []Lista) {
	a.mu.Lock()
	a.Listas, a.Actualizada = listas, time.Now()
	b, _ := json.Marshal(a)
	a.mu.Unlock()
	escribirAtomico(filepath.Join(a.dir, "biblioteca.json"), b)
}

func (a *almacen) listas() []Lista {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Lista(nil), a.Listas...)
}

func (a *almacen) guardarCanciones(lista string, cs []Cancion) {
	a.mu.Lock()
	a.canciones[lista] = cs
	a.mu.Unlock()
	b, _ := json.Marshal(cs)
	escribirAtomico(filepath.Join(a.dir, "listas", nombreSeguro(lista)+".json"), b)
}

// cancionesDe: las guardadas (de memoria o del disco); nil si nunca se abrió.
func (a *almacen) cancionesDe(lista string) []Cancion {
	a.mu.Lock()
	cs, ok := a.canciones[lista]
	a.mu.Unlock()
	if ok {
		return cs
	}
	b, err := os.ReadFile(filepath.Join(a.dir, "listas", nombreSeguro(lista)+".json"))
	if err != nil {
		return nil
	}
	_ = json.Unmarshal(b, &cs)
	a.mu.Lock()
	a.canciones[lista] = cs
	a.mu.Unlock()
	return cs
}

// recordarVideo apunta el vídeo de YouTube de una canción en todas las listas guardadas que la tengan.
func (a *almacen) recordarVideo(id, video string) {
	a.mu.Lock()
	var cambiadas []string
	for l, cs := range a.canciones {
		for i := range cs {
			if cs[i].ID == id && cs[i].Video != video {
				cs[i].Video = video
				cambiadas = append(cambiadas, l)
			}
		}
	}
	a.mu.Unlock()
	for _, l := range cambiadas {
		a.guardarCanciones(l, a.cancionesDe(l))
	}
}

func (a *almacen) descargada(id string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	d, ok := a.descargas[id]
	return d.Archivo, ok
}

func (a *almacen) apuntarDescarga(c Cancion, archivo string) {
	a.mu.Lock()
	a.descargas[c.ID] = descarga{Archivo: archivo, Cancion: c, Cuando: time.Now()}
	b, _ := json.Marshal(a.descargas)
	a.mu.Unlock()
	escribirAtomico(filepath.Join(a.dir, "descargas.json"), b)
}

// descargadas: todas las canciones descargadas, la última primero.
func (a *almacen) descargadas() []Cancion {
	a.mu.Lock()
	ds := make([]descarga, 0, len(a.descargas))
	for _, d := range a.descargas {
		ds = append(ds, d)
	}
	a.mu.Unlock()
	sort.Slice(ds, func(i, j int) bool { return ds[i].Cuando.After(ds[j].Cuando) })
	cs := make([]Cancion, len(ds))
	for i, d := range ds {
		cs[i] = d.Cancion
	}
	return cs
}

func (a *almacen) cuantasDescargadas(cs []Cancion) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range cs {
		if _, ok := a.descargas[c.ID]; ok {
			n++
		}
	}
	return n
}
