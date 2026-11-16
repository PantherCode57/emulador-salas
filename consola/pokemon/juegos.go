package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Juego: una ROM de /roms/gb, /roms/gbc o /roms/gba, con lo que dice de ella su gamelist.xml.
type Juego struct {
	Ruta     string // completa
	Relativa string // dentro de /roms ("gba/Pokemon - FireRed Version (USA).zip"): lo que se guarda en la sala
	Titulo   string // <name> (en español) o, si no lo hay, el nombre del archivo arreglado
	Sistema  string // gb, gbc, gba
	Genero   string // <genre>: «GBA  ·  2004  ·  Aventura y rol»
	Desc     string // <desc>
	Tarjeta  string // la tarjeta de la lista ya hecha (images/panther/…): redondeada, con sombra, 267×166
	Imagen   string // la imagen original (images/…), si la hay
}

// Sistemas: cómo se llaman en pantalla, en qué orden van y el color de su tarjeta (el de la barra
// del menú) para los juegos que no tienen una.
var sistemas = map[string]struct {
	nombre string
	orden  int
	color  [3]uint8
}{
	"gb":  {"Game Boy", 0, [3]uint8{96, 128, 56}},
	"gbc": {"Game Boy Color", 1, [3]uint8{214, 51, 132}},
	"gba": {"GBA", 2, [3]uint8{91, 79, 207}},
}

var extensiones = map[string]bool{".zip": true, ".gb": true, ".gbc": true, ".gba": true, ".sgb": true}

// base: el nombre del archivo sin la extensión (el que usa RetroArch para la partida).
func (j Juego) base() string {
	return strings.TrimSuffix(filepath.Base(j.Ruta), filepath.Ext(j.Ruta))
}

type listaXML struct {
	Juegos []struct {
		Path      string `xml:"path"`
		Name      string `xml:"name"`
		Desc      string `xml:"desc"`
		Genre     string `xml:"genre"`
		Image     string `xml:"image"`
		Thumbnail string `xml:"thumbnail"`
		Hidden    string `xml:"hidden"`
	} `xml:"game"`
}

func leerLista(ruta string) *listaXML {
	f, err := os.Open(ruta)
	if err != nil {
		return nil
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	d.Strict = false // las listas de EmulationStation a veces traen «&» sin escapar
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	l := &listaXML{}
	if d.Decode(l) != nil && len(l.Juegos) == 0 {
		return nil
	}
	return l
}

// rutaDeLista: una ruta de la lista («./Pokemon.zip», «./images/x.png») dentro de la carpeta.
func rutaDeLista(carpeta, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return filepath.Clean(p)
	}
	return filepath.Join(carpeta, filepath.FromSlash(strings.TrimPrefix(p, "./")))
}

// tarjetaDe busca la tarjeta de la lista: images/panther/ con el mismo nombre que su <image>
// (como tarjetas_listas.py), o la de <thumbnail> si ya apunta ahí.
func tarjetaDe(carpeta, imagen, miniatura, rom string) string {
	var pruebas []string
	if m := rutaDeLista(carpeta, miniatura); m != "" && strings.Contains(filepath.ToSlash(m), "/images/panther/") {
		pruebas = append(pruebas, m)
	}
	panther := filepath.Join(carpeta, "images", "panther")
	if im := rutaDeLista(carpeta, imagen); im != "" {
		n := filepath.Base(im)
		pruebas = append(pruebas, filepath.Join(panther, n), filepath.Join(panther, strings.TrimSuffix(n, filepath.Ext(n))+".png"))
	}
	base := strings.TrimSuffix(filepath.Base(rom), filepath.Ext(rom))
	pruebas = append(pruebas, filepath.Join(panther, base+".png"), filepath.Join(panther, base+"-image.png"))
	for _, p := range pruebas {
		if existe(p) {
			return p
		}
	}
	return ""
}

// buscarJuegos: los juegos de los tres sistemas, por sistema y por nombre.
func buscarJuegos() []Juego {
	var lista []Juego
	for sis := range sistemas {
		carpeta := filepath.Join(carpetaRoms, sis)
		vistos := map[string]bool{}
		if l := leerLista(filepath.Join(carpeta, "gamelist.xml")); l != nil {
			for _, g := range l.Juegos {
				ruta := rutaDeLista(carpeta, g.Path)
				if ruta == "" || vistos[ruta] || !extensiones[strings.ToLower(filepath.Ext(ruta))] || !existe(ruta) {
					continue
				}
				vistos[ruta] = true
				if strings.EqualFold(strings.TrimSpace(g.Hidden), "true") {
					continue
				}
				j := Juego{Ruta: ruta, Sistema: sis, Titulo: limpiarEspacios(g.Name),
					Genero: limpiarEspacios(g.Genre), Desc: limpiarEspacios(g.Desc)}
				if j.Titulo == "" {
					j.Titulo = tituloDe(filepath.Base(ruta))
				}
				j.Tarjeta = tarjetaDe(carpeta, g.Image, g.Thumbnail, ruta)
				if im := rutaDeLista(carpeta, g.Image); im != "" && existe(im) {
					j.Imagen = im
				}
				lista = append(lista, j)
			}
		}
		// Los que están en la carpeta pero no en la lista (recién copiados).
		entradas, _ := os.ReadDir(carpeta)
		for _, e := range entradas {
			ruta := filepath.Join(carpeta, e.Name())
			if e.IsDir() || vistos[ruta] || strings.HasPrefix(e.Name(), ".") || !extensiones[strings.ToLower(filepath.Ext(e.Name()))] {
				continue
			}
			lista = append(lista, Juego{Ruta: ruta, Sistema: sis, Titulo: tituloDe(e.Name()),
				Tarjeta: tarjetaDe(carpeta, "", "", ruta)})
		}
	}
	for i := range lista {
		if rel, err := filepath.Rel(carpetaRoms, lista[i].Ruta); err == nil {
			lista[i].Relativa = filepath.ToSlash(rel)
		} else {
			lista[i].Relativa = lista[i].Sistema + "/" + filepath.Base(lista[i].Ruta)
		}
	}
	sort.Slice(lista, func(i, j int) bool {
		a, b := sistemas[lista[i].Sistema].orden, sistemas[lista[j].Sistema].orden
		if a != b {
			return a < b
		}
		return strings.ToLower(lista[i].Titulo) < strings.ToLower(lista[j].Titulo)
	})
	return lista
}

func limpiarEspacios(s string) string { return strings.Join(strings.Fields(s), " ") }

var entreParentesis = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)

// tituloDe: "Pokemon - FireRed Version (USA, Europe).gba" -> "Pokemon - FireRed Version".
func tituloDe(nombre string) string {
	base := strings.TrimSuffix(nombre, filepath.Ext(nombre))
	t := entreParentesis.ReplaceAllString(base, "")
	t = strings.ReplaceAll(t, "_", " ")
	t = strings.Trim(strings.Join(strings.Fields(t), " "), " -")
	if t == "" {
		return base
	}
	return t
}

// genero: lo de debajo del nombre; si la lista no lo trae, el sistema.
func (j Juego) genero() string {
	if j.Genero != "" {
		return j.Genero
	}
	return sistemas[j.Sistema].nombre
}

// nucleoDe: el núcleo de cada sistema y si lleva GBA-PK (el mGBA de Pokemulador, con Lua).
func nucleoDe(sis string) (string, bool) {
	if v := os.Getenv("POKEMON_NUCLEO_" + strings.ToUpper(sis)); v != "" {
		return v, sis == "gba" && os.Getenv("POKEMON_GBAPK") != "0"
	}
	if sis == "gba" {
		propio := filepath.Join(carpetaPrograma, "mgba_libretro.so")
		if existe(propio) {
			return propio, true
		}
		return filepath.Join(carpetaCores, "mgba_libretro.so"), false
	}
	return filepath.Join(carpetaCores, "gambatte_libretro.so"), false
}
