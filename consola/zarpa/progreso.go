package main

import (
	"encoding/json"
	"os"
)

// progreso: lo que se guarda entre partidas (en ~/.config/panther/zarpa/progreso.json).
type progreso struct {
	ruta      string
	Record    int      `json:"record"`    // los puntos más altos
	RecordM   int      `json:"record_m"`  // los metros de esa partida
	Estrellas int      `json:"estrellas"` // las que tienes para gastar en la tienda
	Total     int      `json:"total"`     // todas las que has cogido
	Partidas  int      `json:"partidas"`
	Trajes    []string `json:"trajes"` // los comprados
	Traje     string   `json:"traje"`  // el puesto
	SinMusica bool     `json:"sin_musica"`
}

func cargarProgreso(ruta string) *progreso {
	p := &progreso{ruta: ruta, Traje: trajes[0].ID, Trajes: []string{trajes[0].ID}}
	if b, err := os.ReadFile(ruta); err == nil {
		_ = json.Unmarshal(b, p)
	}
	if !p.tiene(trajes[0].ID) {
		p.Trajes = append(p.Trajes, trajes[0].ID)
	}
	return p
}

func (p *progreso) tiene(id string) bool {
	for _, t := range p.Trajes {
		if t == id {
			return true
		}
	}
	return false
}

func (p *progreso) guardar() {
	if p.ruta == "" {
		return
	}
	b, _ := json.MarshalIndent(p, "", " ")
	tmp := p.ruta + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, p.ruta)
	}
}

// acabarPartida: suma lo de la partida y dice si es récord.
func (p *progreso) acabarPartida(c *carrera) bool {
	p.Partidas++
	p.Estrellas += c.estrellas
	p.Total += c.estrellas
	nuevo := c.puntos() > p.Record
	if nuevo {
		p.Record, p.RecordM = c.puntos(), c.metros()
	}
	p.guardar()
	return nuevo
}
