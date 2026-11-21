package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tienda: lo que hay para poner, de dónde sale y qué está puesto. Sus campos solo se tocan desde el
// bucle de la app; lo que hace el trabajador (bajar, copiar, borrar) recibe una copia de lo que
// necesita y devuelve el resultado al bucle.
type tienda struct {
	apps, juegos []*elemento

	fuente    string        // dirección de la fuente de las apps (acaba en /)
	indice    *indiceTienda // lo que hay en la fuente (nil: no se ha podido leer)
	errIndice error
	local     string // ~/panther-instalar
	panther   string // ~/panther
	roms      string // /roms
	ports     string // /roms/ports
	descargas string // donde se dejan los zip mientras se bajan
	tarjetas  string // las tarjetas del tema (las de las apps)
	puestos   string // lo que la tienda ha puesto de cada juego

	// Para dar de alta en EmulationStation las apps nuevas (menu_es.go).
	esSistemas []string                             // es_systems.cfg (y es_systems.sd1)
	esAjustes  string                               // es_settings.cfg
	tema       string                               // el tema de Panther OS
	tarjetaApp func(e *elemento, ruta string) error // dibuja la tarjeta de una app (nil: no se dibuja)

	catalogoPM time.Time // cuándo se leyó el ports.json de PortMaster (cero: no se ha leído)
	errPM      error

	cambios bool // hay que reiniciar EmulationStation al salir

	ctx    context.Context
	cancel context.CancelFunc
}

// indiceTienda: tienda.json de la fuente (lo hace la compilación de la consola).
type indiceTienda struct {
	Version string                `json:"version"`
	Fecha   string                `json:"fecha"`
	Piezas  map[string]piezaIndex `json:"piezas"`
}

type piezaIndex struct {
	Tam    int64  `json:"tam"`
	SHA256 string `json:"sha256"`
}

const (
	fuentePorDefecto = "https://github.com/PantherCode57/emulador-salas/releases/download/panther-apps/"
	catalogoPMURL    = "https://github.com/PortsMaster/PortMaster-New/releases/latest/download/ports.json"
	imagenesPMURL    = "https://raw.githubusercontent.com/PortsMaster/PortMaster-New/main/ports/"
	ytdlpURL         = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
)

func nuevaTienda() *tienda {
	t := &tienda{
		apps:     catalogoApps(),
		juegos:   catalogoJuegos(),
		fuente:   entorno("TIENDA_FUENTE", fuentePorDefecto),
		local:    entorno("TIENDA_LOCAL", filepath.Join(casa, "panther-instalar")),
		panther:  entorno("TIENDA_PANTHER", filepath.Join(casa, "panther")),
		roms:     entorno("TIENDA_ROMS", "/roms"),
		tarjetas: entorno("TIENDA_TARJETAS", "/roms/themes/es-theme-panther/_panther/tarjetas"),
		puestos:  filepath.Join(carpetaDatos, "puestos"),
		esSistemas: strings.Split(entorno("TIENDA_ES_SISTEMAS",
			"/etc/emulationstation/es_systems.cfg:/etc/emulationstation/es_systems.sd1"), ":"),
		esAjustes: entorno("TIENDA_ES_AJUSTES", filepath.Join(casa, ".emulationstation", "es_settings.cfg")),
		tema:      entorno("TIENDA_TEMA", "/roms/themes/es-theme-panther"),
	}
	if !strings.HasSuffix(t.fuente, "/") {
		t.fuente += "/"
	}
	t.ports = entorno("TIENDA_PORTS", filepath.Join(t.roms, "ports"))
	t.descargas = entorno("TIENDA_DESCARGAS", filepath.Join(t.roms, ".panther-tienda"))
	t.ctx, t.cancel = context.WithCancel(context.Background())
	if b, err := os.ReadFile(filepath.Join(carpetaDatos, "tienda.json")); err == nil {
		var ind indiceTienda
		if json.Unmarshal(b, &ind) == nil && len(ind.Piezas) > 0 {
			t.indice = &ind
		}
	}
	return t
}

func (t *tienda) cancelarTodo() { t.cancel() }

// leerIndice: tienda.json de la fuente (con una copia, por si luego no hay conexión).
func leerIndice(ctx context.Context, fuente string) (*indiceTienda, error) {
	b, err := leerURL(ctx, fuente+"tienda.json", 1<<20)
	if err != nil {
		return nil, err
	}
	var ind indiceTienda
	if err := json.Unmarshal(b, &ind); err != nil {
		return nil, fmt.Errorf("tienda.json: %v", err)
	}
	if len(ind.Piezas) == 0 {
		return nil, errors.New("la fuente de la tienda está vacía")
	}
	_ = escribirAtomico(filepath.Join(carpetaDatos, "tienda.json"), b, 0o644)
	return &ind, nil
}

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

func tamDe(ruta string) int64 {
	st, err := os.Stat(ruta)
	if err != nil || st.IsDir() {
		return -1
	}
	return st.Size()
}

// rutaRoms: las rutas de las piezas que empiezan por /roms van a t.roms (para probar en el PC).
func (t *tienda) rutaRoms(r string) string {
	if strings.HasPrefix(r, "/roms/") {
		return filepath.Join(t.roms, strings.TrimPrefix(r, "/roms/"))
	}
	return r
}

func (t *tienda) destino(e *elemento, p pieza) string {
	if strings.HasPrefix(p.destino, "/") {
		return t.rutaRoms(p.destino)
	}
	return filepath.Join(t.panther, e.id, filepath.FromSlash(p.destino))
}

// buscarLocal: la pieza en ~/panther-instalar.
func (t *tienda) buscarLocal(p pieza) string {
	for _, l := range p.locales {
		r := filepath.Join(t.local, filepath.FromSlash(l))
		if tamDe(r) > 0 {
			return r
		}
	}
	return ""
}

// nucleoPuesto: el núcleo de la pantalla de cualquier app que ya esté puesta.
func (t *tienda) nucleoPuesto() string {
	m, _ := filepath.Glob(filepath.Join(t.panther, "*", "pokemulador_libretro.so"))
	for _, r := range m {
		if tamDe(r) > 0 {
			return r
		}
	}
	return ""
}

// ytdlpPuesto: el yt-dlp de Spotin o de YouTube.
func (t *tienda) ytdlpPuesto() string {
	for _, a := range []string{"spotin", "youtube"} {
		r := filepath.Join(t.panther, a, "bin", "yt-dlp")
		if tamDe(r) > 0 {
			return r
		}
	}
	return ""
}

// guardadas: al quitar una app, su versión se guarda aquí (en ~/panther-instalar puede haber una
// más vieja: muchas se ponen al día sin pasar por allí) y al volver a ponerla sale la misma.
func (t *tienda) guardadas(id string) string { return filepath.Join(t.panther, ".guardadas", id) }

// origen: de dónde sale una pieza: «url» (de la fuente), «local» (una copia) o nada.
type origen struct {
	url, local string
	tam        int64
	suma       string
	guardada   bool // de ~/panther/.guardadas (se mueve, no se copia)
}

// origenDe: primero la fuente de internet (lo más nuevo), luego lo que se guardó al quitarla,
// luego ~/panther-instalar.
func (t *tienda) origenDe(e *elemento, p pieza, ind *indiceTienda) (origen, bool) {
	if ind != nil {
		if pi, ok := ind.Piezas[p.nombre]; ok && !p.ytdlp {
			s := ""
			if pi.SHA256 != "" {
				s = "sha256:" + pi.SHA256
			}
			return origen{url: t.fuente + p.nombre, tam: pi.Tam, suma: s}, true
		}
	}
	if r := filepath.Join(t.guardadas(e.id), p.nombre); tamDe(r) > 0 {
		return origen{local: r, tam: tamDe(r), guardada: true}, true
	}
	if p.ytdlp {
		if r := t.ytdlpPuesto(); r != "" {
			return origen{local: r, tam: tamDe(r)}, true
		}
		return origen{url: ytdlpURL, tam: 3 << 20}, true
	}
	if r := t.buscarLocal(p); r != "" {
		return origen{local: r, tam: tamDe(r)}, true
	}
	if p.nucleo {
		if r := t.nucleoPuesto(); r != "" {
			return origen{local: r, tam: tamDe(r)}, true
		}
	}
	return origen{}, false
}

// marcaAMedias: mientras se pone un juego hay una marca; si la tienda se cierra a medias, se ve.
func (t *tienda) marcaAMedias(zip string) string {
	return filepath.Join(t.puestos, strings.TrimSuffix(zip, ".zip")+".a-medias")
}

func (t *tienda) registroJuego(zip string) string {
	return filepath.Join(t.puestos, strings.TrimSuffix(zip, ".zip")+".txt")
}

// scriptJuego: el .sh de un juego (lo que abre EmulationStation).
func scriptJuego(g *juegoPM) string {
	for _, it := range g.items {
		if strings.HasSuffix(strings.ToLower(it), ".sh") && !strings.Contains(strings.TrimSuffix(it, "/"), "/") {
			return it
		}
	}
	return ""
}

// mirar: qué está puesto, cuánto ocupa y si se puede poner (solo mira archivos: es rápido).
func (t *tienda) mirar() {
	for _, e := range t.apps {
		a := e.app
		prog := filepath.Join(t.panther, e.id, e.id)
		e.puesto = existe(t.rutaRoms("/roms/"+a.sistema+"/"+a.script)) && tamDe(prog) > 0
		var tam, ocupa int64
		falta := ""
		for _, p := range a.piezas {
			if n := tamDe(t.destino(e, p)); n > 0 {
				ocupa += n
			}
			o, ok := t.origenDe(e, p, t.indice)
			if !ok {
				if tamDe(t.destino(e, p)) > 0 {
					continue // ya está en su sitio
				}
				if falta == "" {
					falta = p.nombre
				}
				continue
			}
			tam += o.tam
		}
		e.tam = tam
		if e.puesto && ocupa > 0 {
			e.tam = ocupa
		}
		e.disponible = falta == ""
		e.motivo = ""
		if falta != "" {
			e.motivo = "No hay copia en la consola ni en la tienda de internet"
			if t.indice == nil && errors.Is(t.errIndice, errSinRed) {
				e.motivo = "No hay copia en la consola y no hay conexión"
			}
		}
	}
	for _, e := range t.juegos {
		g := e.juego
		s := scriptJuego(g)
		e.puesto = s != "" && existe(filepath.Join(t.ports, s)) && !existe(t.marcaAMedias(g.zip))
		e.tam = g.tam
		e.disponible = g.url != ""
		e.motivo = ""
		if !e.disponible {
			e.motivo = "PortMaster ya no lo ofrece para esta consola"
		}
	}
}

// ocupaJuego: lo que ocupa en la tarjeta lo que hay puesto de un juego (para decirlo al quitarlo).
func (t *tienda) ocupaJuego(g *juegoPM) int64 {
	var n int64
	for _, it := range g.items {
		nom, ok := nombreSeguro(it)
		if !ok {
			continue
		}
		_ = filepath.Walk(filepath.Join(t.ports, nom), func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				n += info.Size()
			}
			return nil
		})
	}
	return n
}

// mensaje: el error en una frase corta para la pantalla.
func mensaje(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errSinRed) {
		return "no hay conexión"
	}
	s := err.Error()
	if len([]rune(s)) > 90 {
		s = string([]rune(s)[:90]) + "…"
	}
	return s
}

// ---- Lo que hace el trabajador ----

// avance: el trabajador dice por dónde va.
type avance func(fase string, hecho, total int64)

// copiar: un archivo a otro sitio (pasando por .nuevo, para no dejarlo a medias).
func copiar(desde, hasta string, modo os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(hasta), 0o755); err != nil {
		return err
	}
	in, err := os.Open(desde)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := hasta + ".nuevo"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, modo)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Chmod(tmp, modo)
	return os.Rename(tmp, hasta)
}

// trabajoApp: lo que el trabajador necesita para poner o quitar una app.
type trabajoApp struct {
	e      *elemento // solo para leer lo que no cambia (id, app)
	indice *indiceTienda
	otraYT bool // YouTube está puesta (al quitar Spotin, se le deja yt-dlp)
}

func (t *tienda) ponerApp(ctx context.Context, tr trabajoApp, informar avance) error {
	e, a := tr.e, tr.e.app
	type paso struct {
		p    pieza
		o    origen
		dest string
	}
	var pasos []paso
	var total int64
	for _, p := range a.piezas {
		o, ok := t.origenDe(e, p, tr.indice)
		dest := t.destino(e, p)
		if !ok {
			if tamDe(dest) > 0 {
				continue // ya está en su sitio
			}
			return fmt.Errorf("falta %s", p.nombre)
		}
		if tamDe(dest) > 0 {
			// Lo que ya está se queda (WhatsApp sigue en marcha como servicio, y lo de
			// ~/panther-instalar puede ser más viejo), salvo que la fuente de internet tenga otro.
			if o.url == "" || p.ytdlp || o.suma == "" || sumaDe(dest, o.suma) {
				continue
			}
		}
		pasos = append(pasos, paso{p, o, dest})
		total += o.tam
	}
	var hecho int64
	programa := false
	for _, s := range pasos {
		if err := ctx.Err(); err != nil {
			return err
		}
		modo := os.FileMode(s.p.modo)
		if err := os.MkdirAll(filepath.Dir(s.dest), 0o755); err != nil {
			return err
		}
		switch {
		case s.o.url != "":
			tmp := s.dest + ".bajando"
			base := hecho
			err := bajar(ctx, s.o.url, tmp, s.o.tam, s.o.suma, func(h, tt int64) {
				informar("Bajando", base+h, max(total, base+tt))
			})
			if err != nil {
				return fmt.Errorf("%s: %w", s.p.nombre, err)
			}
			_ = os.Chmod(tmp, modo)
			if err := os.Rename(tmp, s.dest); err != nil {
				return err
			}
		case s.o.guardada && os.Rename(s.o.local, s.dest) == nil:
			_ = os.Chmod(s.dest, modo)
		default:
			informar("Poniendo", hecho, total)
			if err := copiar(s.o.local, s.dest, modo); err != nil {
				return fmt.Errorf("%s: %w", s.p.nombre, err)
			}
		}
		if s.p.nombre == e.id {
			programa = true
		}
		hecho += s.o.tam
		informar("Poniendo", hecho, total)
	}
	for _, c := range a.carpetas {
		_ = os.MkdirAll(t.rutaRoms(c), 0o755)
	}
	_ = os.RemoveAll(t.guardadas(e.id)) // lo que quedara de la copia guardada ya no hace falta
	_ = os.Remove(filepath.Dir(t.guardadas(e.id)))
	if a.servicio && programa {
		reiniciarServicio("panther-whatsapp") // con el programa nuevo
	}
	logf("puesta %s (%d piezas)", e.id, len(pasos))
	// Las apps que llegan después de la instalación (la Pokédex) no tienen apartado en el menú.
	if err := t.darDeAlta(e); err != nil {
		return fmt.Errorf("no se ha podido poner en el menú: %w", err)
	}
	return nil
}

func sumaDe(ruta, suma string) bool {
	h, quiero := nuevaSuma(suma)
	if h == nil {
		return true
	}
	if sumar(ruta, h) != nil {
		return false
	}
	return fmt.Sprintf("%x", h.Sum(nil)) == quiero
}

// guardar: mueve un archivo a la copia guardada de la app.
func (t *tienda) guardar(id, desde, nombre string, modo os.FileMode) error {
	dir := t.guardadas(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	hasta := filepath.Join(dir, nombre)
	if os.Rename(desde, hasta) == nil {
		return nil
	}
	if err := copiar(desde, hasta, modo); err != nil {
		return err
	}
	return os.Remove(desde)
}

func (t *tienda) quitarApp(ctx context.Context, tr trabajoApp, informar avance) error {
	e, a := tr.e, tr.e.app
	informar("Quitando", 0, 1)
	// Primero lo que abre EmulationStation: sin él, la tarjeta se va.
	script := t.rutaRoms("/roms/" + a.sistema + "/" + a.script)
	if tamDe(script) > 0 {
		if err := t.guardar(e.id, script, a.script, 0o755); err != nil {
			return err
		}
	}
	if a.servicio {
		// El programa de WhatsApp es también el servicio que pinta la hora, la batería y el
		// volumen en todas las pantallas: se queda.
		logf("quitada la tarjeta de %s (el servicio sigue)", e.id)
		return nil
	}
	dir := filepath.Join(t.panther, e.id)
	// yt-dlp: si se quita Spotin y YouTube usaba el suyo, se le pasa.
	if e.id == "spotin" && tr.otraYT {
		suyo := filepath.Join(t.panther, "youtube", "bin", "yt-dlp")
		mio := filepath.Join(dir, "bin", "yt-dlp")
		if tamDe(suyo) <= 0 && tamDe(mio) > 0 {
			_ = os.MkdirAll(filepath.Dir(suyo), 0o755)
			if os.Rename(mio, suyo) != nil {
				_ = copiar(mio, suyo, 0o755)
			}
		}
	}
	for _, p := range a.piezas {
		if strings.HasPrefix(p.destino, "/") {
			continue
		}
		if r := t.destino(e, p); tamDe(r) > 0 {
			if err := t.guardar(e.id, r, p.nombre, os.FileMode(p.modo)); err != nil {
				return err
			}
		}
	}
	// Las copias que dejaron las instalaciones (spotin.antes-rapido…) y lo que quedó a medias.
	for _, patron := range []string{e.id + ".antes-*", "*.nuevo", "*.bajando", "*.bajando.part"} {
		m, _ := filepath.Glob(filepath.Join(dir, patron))
		for _, r := range m {
			_ = os.Remove(r)
		}
	}
	_ = os.Remove(filepath.Join(dir, "bin")) // solo si está vacía
	_ = os.Remove(dir)
	informar("Quitando", 1, 1)
	logf("quitada %s (guardada en %s)", e.id, t.guardadas(e.id))
	return nil
}
