package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Juegos de PortMaster, puestos como los pone él: el zip se descomprime en /roms/ports (el .sh que
// abre EmulationStation y la carpeta del juego) y el juego se apunta en /roms/ports/gamelist.xml.
// Los .sh buscan PortMaster en /opt/system/Tools/PortMaster, que en dArkOS ya está.

// ---- El catálogo de PortMaster (ports.json) ----

type entradaPM struct {
	Items []string `json:"items"`
	Attr  struct {
		Title   string   `json:"title"`
		RTR     bool     `json:"rtr"`
		Runtime []string `json:"runtime"`
		Reqs    []string `json:"reqs"`
		Arch    []string `json:"arch"`
		Image   struct {
			Screenshot string `json:"screenshot"`
		} `json:"image"`
	} `json:"attr"`
	Source struct {
		URL  string `json:"url"`
		MD5  string `json:"md5"`
		Size int64  `json:"size"`
	} `json:"source"`
}

// leerCatalogoPM: ports.json de PortMaster (se guarda una copia y se vuelve a pedir cada 12 h).
// Devuelve las entradas de los juegos que hay en la tienda.
func leerCatalogoPM(ctx context.Context, zips []string) (map[string]entradaPM, error) {
	copia := filepath.Join(carpetaDatos, "ports.json")
	var datos []byte
	if st, err := os.Stat(copia); err == nil && time.Since(st.ModTime()) < 12*time.Hour {
		datos, _ = os.ReadFile(copia)
	}
	var errRed error
	if datos == nil {
		b, err := leerURL(ctx, catalogoPMURL, 32<<20)
		if err == nil {
			datos = b
			_ = escribirAtomico(copia, b, 0o644)
		} else {
			errRed = err
			datos, _ = os.ReadFile(copia) // la de antes, aunque sea vieja
		}
	}
	if datos == nil {
		return nil, errRed
	}
	var todo struct {
		Ports map[string]json.RawMessage `json:"ports"`
	}
	if err := json.Unmarshal(datos, &todo); err != nil {
		return nil, fmt.Errorf("ports.json: %v", err)
	}
	out := map[string]entradaPM{}
	for _, z := range zips {
		raw, ok := todo.Ports[z]
		if !ok {
			continue
		}
		var e entradaPM
		if json.Unmarshal(raw, &e) == nil {
			out[z] = e
		}
	}
	return out, nil
}

// aplicarCatalogoPM: lo que dice PortMaster manda sobre lo apuntado en catalogo.go. Si un juego
// deja de ser «ready to run» o pide una consola potente, ya no se ofrece (si está puesto se puede
// quitar igual).
func aplicarCatalogoPM(juegos []*elemento, cat map[string]entradaPM) {
	for _, e := range juegos {
		c, ok := cat[e.juego.zip]
		if !ok {
			continue
		}
		g := *e.juego
		if g.externo != nil {
			// Su port va fijo: es el que se ha comprobado con el archivo de la página del juego (si
			// PortMaster lo cambia, puede que pida otra versión). Solo se mira que no pida más consola.
			if c.Attr.Image.Screenshot != "" {
				g.captura = c.Attr.Image.Screenshot
			}
			if contiene(c.Attr.Reqs, "power") || contiene(c.Attr.Reqs, "2gb") {
				g.url = ""
			}
			e.juego = &g
			continue
		}
		if c.Source.URL != "" {
			g.url, g.md5, g.tam = c.Source.URL, c.Source.MD5, c.Source.Size
		}
		if len(c.Items) > 0 {
			g.items = c.Items
		}
		if c.Attr.Image.Screenshot != "" {
			g.captura = c.Attr.Image.Screenshot
		}
		if !c.Attr.RTR || contiene(c.Attr.Reqs, "power") || contiene(c.Attr.Reqs, "2gb") ||
			(len(c.Attr.Arch) > 0 && !contiene(c.Attr.Arch, "aarch64")) {
			g.url = ""
		}
		g.runtime = c.Attr.Runtime
		e.juego = &g
	}
}

func contiene(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// ---- Poner y quitar ----

// trabajoJuego: lo que el trabajador necesita (una copia: el bucle puede cambiar el elemento).
type trabajoJuego struct {
	g      juegoPM
	nombre string
	larga  string
	genero string
	imagen *image.RGBA // la captura (para la tarjeta de la lista); puede ser nil
}

// protegidos: lo de /roms/ports que nunca es de un juego.
var protegidos = map[string]bool{"": true, ".": true, "..": true, "portmaster": true, "images": true,
	"gamelist.xml": true, "autoinstall": true, "videos": true, "manuals": true}

// nombreSeguro: un elemento de primer nivel de /roms/ports («SuperTux.sh», «supertux»).
func nombreSeguro(it string) (string, bool) {
	n := strings.TrimSuffix(strings.TrimSpace(it), "/")
	if n == "" || strings.ContainsAny(n, `/\`) || protegidos[strings.ToLower(n)] || strings.HasPrefix(n, ".") {
		return "", false
	}
	return n, true
}

func (t *tienda) ponerJuego(ctx context.Context, tr trabajoJuego, informar avance) error {
	g := tr.g
	if g.url == "" {
		return errors.New("ya no está en PortMaster")
	}
	for _, r := range g.runtime {
		if !existe(filepath.Join(t.roms, "tools", "PortMaster", "libs", r)) {
			return fmt.Errorf("necesita %s de PortMaster, que no está en la consola", r)
		}
	}
	hace := g.tam * 3
	if g.externo != nil {
		hace += g.externo.tam * 4 // lo bajado, lo que sale de dentro y el juego con el parche
	}
	if libre := espacioLibre(t.ports); libre >= 0 && libre < hace+(64<<20) {
		return fmt.Errorf("no cabe: hacen falta %s y quedan %s", tamTexto(hace), tamTexto(libre))
	}
	if err := os.MkdirAll(t.descargas, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(t.puestos, 0o755); err != nil {
		return err
	}
	zipRuta := filepath.Join(t.descargas, g.zip)
	if err := bajar(ctx, g.url, zipRuta, g.tam, "md5:"+g.md5, func(h, tt int64) { informar("Bajando", h, tt) }); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// La marca va antes de descomprimir: si se corta a medias, el juego no sale como puesto.
	marca := t.marcaAMedias(g.zip)
	_ = os.WriteFile(marca, []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644)
	puestos, err := descomprimir(ctx, zipRuta, t.ports, func(h, tt int64) { informar("Descomprimiendo", h, tt) })
	if len(puestos) > 0 { // lo que se ha puesto, para poder quitarlo aunque falle a medias
		_ = os.WriteFile(t.registroJuego(g.zip), []byte(strings.Join(puestos, "\n")+"\n"), 0o644)
	}
	if err != nil {
		return err
	}
	_ = os.Remove(zipRuta)
	if x := g.externo; x != nil {
		// El juego, de su página, y prepararlo (ver externo.go).
		carpeta := carpetaJuego(&g)
		if carpeta == "" {
			return errors.New("el port no tiene carpeta")
		}
		bajado := filepath.Join(t.descargas, x.archivo)
		if tamDe(bajado) != x.tam {
			if err := bajarExterno(ctx, x, bajado, informar); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		p := preparacion{dir: filepath.Join(t.ports, carpeta), archivo: bajado, pm: t.carpetasPM(), informar: informar}
		if err := x.preparar(ctx, p); err != nil {
			if ctx.Err() == nil {
				_ = os.Remove(bajado) // por si llegó mal: la próxima vez se baja de nuevo
			}
			return err
		}
		_ = os.Remove(bajado)
	}
	if len(g.partidas) > 0 {
		if err := t.devolverPartidas(&g); err != nil {
			logf("partidas de %s: %v", g.zip, err)
		}
	}
	_ = os.Remove(t.descargas) // solo si ha quedado vacía
	informar("Poniendo", 1, 1)
	if err := t.apuntarEnLista(tr); err != nil {
		logf("lista de EmulationStation: %v", err) // el juego sale igual, con el nombre del .sh
	}
	_ = os.Remove(marca)
	logf("puesto %s (%s)", g.zip, strings.Join(puestos, ", "))
	return nil
}

// descomprimir pone el zip en dir y devuelve lo que ha puesto en el primer nivel.
func descomprimir(ctx context.Context, zipRuta, dir string, informar func(h, tt int64)) ([]string, error) {
	z, err := zip.OpenReader(zipRuta)
	if err != nil {
		return nil, fmt.Errorf("el zip está mal: %v", err)
	}
	defer z.Close()
	var total int64
	primeros := map[string]bool{}
	var orden []string
	for _, f := range z.File {
		nombre := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		if strings.HasPrefix(nombre, "/") || nombre == ".." || strings.HasPrefix(nombre, "../") || strings.Contains(nombre, "/../") {
			return nil, fmt.Errorf("el zip tiene una ruta rara: %s", f.Name)
		}
		primero := strings.SplitN(nombre, "/", 2)[0]
		if _, ok := nombreSeguro(primero); !ok {
			return nil, fmt.Errorf("el zip quiere poner %s, que no es de un juego", primero)
		}
		if !primeros[primero] {
			primeros[primero] = true
			orden = append(orden, primero)
		}
		total += int64(f.UncompressedSize64)
	}
	var hecho int64
	ultimo := time.Time{}
	buf := make([]byte, 256<<10)
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return orden, err
		}
		nombre := path.Clean(strings.ReplaceAll(f.Name, `\`, "/"))
		dest := filepath.Join(dir, filepath.FromSlash(nombre))
		if f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return orden, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return orden, err
		}
		modo := os.FileMode(0o644)
		if f.Mode()&0o111 != 0 || strings.HasSuffix(strings.ToLower(nombre), ".sh") {
			modo = 0o755
		}
		in, err := f.Open()
		if err != nil {
			return orden, err
		}
		// Los enlaces simbólicos se ponen como archivo con su destino dentro, como hace PortMaster
		// (en /roms, que es exFAT, no los hay).
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, modo)
		if err != nil {
			in.Close()
			return orden, err
		}
		for {
			n, rerr := in.Read(buf)
			if n > 0 {
				if _, werr := out.Write(buf[:n]); werr != nil {
					in.Close()
					out.Close()
					return orden, werr
				}
				hecho += int64(n)
				if time.Since(ultimo) > 150*time.Millisecond {
					ultimo = time.Now()
					informar(hecho, total)
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				in.Close()
				out.Close()
				return orden, fmt.Errorf("el zip está mal: %v", rerr)
			}
		}
		in.Close()
		if err := out.Close(); err != nil {
			return orden, err
		}
		_ = os.Chmod(dest, modo)
	}
	informar(total, total)
	return orden, nil
}

func (t *tienda) quitarJuego(ctx context.Context, tr trabajoJuego, informar avance) error {
	g := tr.g
	informar("Quitando", 0, 1)
	if len(g.partidas) > 0 {
		if err := t.guardarPartidas(&g); err != nil {
			return fmt.Errorf("no se han podido guardar tus partidas: %v", err)
		}
	}
	nombres := map[string]bool{}
	var orden []string
	poner := func(it string) {
		if n, ok := nombreSeguro(it); ok && !nombres[n] {
			nombres[n] = true
			orden = append(orden, n)
		}
	}
	// Primero el .sh: sin él, EmulationStation ya no lo enseña.
	if s := scriptJuego(&g); s != "" {
		poner(s)
	}
	for _, it := range g.items {
		poner(it)
	}
	if b, err := os.ReadFile(t.registroJuego(g.zip)); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			poner(l)
		}
	}
	for i, n := range orden {
		if err := os.RemoveAll(filepath.Join(t.ports, n)); err != nil {
			return err
		}
		informar("Quitando", int64(i+1), int64(len(orden)+1))
	}
	if err := t.borrarDeLista(scriptJuego(&g), g.zip); err != nil {
		logf("lista de EmulationStation: %v", err)
	}
	_ = os.Remove(t.registroJuego(g.zip))
	_ = os.Remove(t.marcaAMedias(g.zip))
	_ = os.Remove(filepath.Join(t.descargas, g.zip+".part"))
	if g.externo != nil {
		_ = os.Remove(filepath.Join(t.descargas, g.externo.archivo))
		_ = os.Remove(filepath.Join(t.descargas, g.externo.archivo+".part"))
	}
	_ = os.Remove(t.descargas)
	informar("Quitando", 1, 1)
	logf("quitado %s (%s)", g.zip, strings.Join(orden, ", "))
	return nil
}

// ---- La lista de EmulationStation (/roms/ports/gamelist.xml) ----

func xmlTexto(s string) string { return html.EscapeString(s) }

var reJuego = regexp.MustCompile(`(?s)[ \t]*<game>.*?</game>[ \t]*\r?\n?`)

func rutaEnLista(script string) string { return "./" + script }

func (t *tienda) leerLista() string {
	b, err := os.ReadFile(filepath.Join(t.ports, "gamelist.xml"))
	if err != nil || !strings.Contains(string(b), "<gameList") {
		return "<?xml version=\"1.0\"?>\n<gameList>\n</gameList>\n"
	}
	return string(b)
}

// quitarEntrada: el texto de la lista sin la entrada de ese .sh.
func quitarEntrada(lista, script string) string {
	quiero := "<path>" + xmlTexto(rutaEnLista(script)) + "</path>"
	return reJuego.ReplaceAllStringFunc(lista, func(b string) string {
		if strings.Contains(b, quiero) || strings.Contains(b, "<path>"+rutaEnLista(script)+"</path>") {
			return ""
		}
		return b
	})
}

func stem(zip string) string { return strings.TrimSuffix(zip, ".zip") }

// apuntarEnLista pone el juego en la lista con su nombre y su descripción en español, y la tarjeta
// de la lista (images/panther/<juego>.png, como las de los otros apartados).
func (t *tienda) apuntarEnLista(tr trabajoJuego) error {
	g := tr.g
	script := scriptJuego(&g)
	if script == "" {
		return errors.New("el juego no tiene .sh")
	}
	carpeta := ""
	for _, it := range g.items {
		if n, ok := nombreSeguro(it); ok && n != script {
			carpeta = n
			break
		}
	}
	img := tr.imagen
	imagenLista := ""
	if carpeta != "" {
		for _, c := range []string{"cover.png", "screenshot.jpg", "screenshot.png", "cover.jpg"} {
			if existe(filepath.Join(t.ports, carpeta, c)) {
				imagenLista = "./" + carpeta + "/" + c
				if img == nil && strings.HasPrefix(c, "screenshot") {
					if im := abrirImagen(filepath.Join(t.ports, carpeta, c)); im != nil {
						img = escalar(im, 288, 166, true)
					}
				}
				break
			}
		}
	}
	miniatura := ""
	if img != nil {
		dir := filepath.Join(t.ports, "images", "panther")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			var b bytes.Buffer
			if png.Encode(&b, tarjetaLista(img)) == nil &&
				escribirAtomico(filepath.Join(dir, stem(g.zip)+".png"), b.Bytes(), 0o644) == nil {
				miniatura = "./images/panther/" + stem(g.zip) + ".png"
			}
		}
	}
	var e strings.Builder
	e.WriteString("\t<game>\n")
	fmt.Fprintf(&e, "\t\t<path>%s</path>\n", xmlTexto(rutaEnLista(script)))
	fmt.Fprintf(&e, "\t\t<name>%s</name>\n", xmlTexto(tr.nombre))
	fmt.Fprintf(&e, "\t\t<desc>%s</desc>\n", xmlTexto(tr.larga))
	if imagenLista != "" {
		fmt.Fprintf(&e, "\t\t<image>%s</image>\n", xmlTexto(imagenLista))
	}
	if miniatura != "" {
		fmt.Fprintf(&e, "\t\t<thumbnail>%s</thumbnail>\n", xmlTexto(miniatura))
	}
	if g.anio > 0 {
		fmt.Fprintf(&e, "\t\t<releasedate>%04d0101T000000</releasedate>\n", g.anio)
	}
	if g.autor != "" {
		fmt.Fprintf(&e, "\t\t<developer>%s</developer>\n", xmlTexto(g.autor))
	}
	// «consola · año · género», como en las otras listas.
	ficha := "PortMaster"
	if g.anio > 0 {
		ficha += fmt.Sprintf("  ·  %d", g.anio)
	}
	if tr.genero != "" {
		ficha += "  ·  " + tr.genero
	}
	fmt.Fprintf(&e, "\t\t<genre>%s</genre>\n", xmlTexto(ficha))
	e.WriteString("\t</game>\n")
	lista := quitarEntrada(t.leerLista(), script)
	i := strings.LastIndex(lista, "</gameList>")
	if i < 0 {
		return errors.New("gamelist.xml sin </gameList>")
	}
	lista = lista[:i] + e.String() + lista[i:]
	return escribirAtomico(filepath.Join(t.ports, "gamelist.xml"), []byte(lista), 0o644)
}

func (t *tienda) borrarDeLista(script, zip string) error {
	_ = os.Remove(filepath.Join(t.ports, "images", "panther", stem(zip)+".png"))
	if script == "" || !existe(filepath.Join(t.ports, "gamelist.xml")) {
		return nil
	}
	antes := t.leerLista()
	despues := quitarEntrada(antes, script)
	if despues == antes {
		return nil
	}
	return escribirAtomico(filepath.Join(t.ports, "gamelist.xml"), []byte(despues), 0o644)
}

// tarjetaLista: la tarjeta de la lista de juegos, como las de los otros apartados: 267×166, con la
// imagen en 233×130 (esquinas redondas) y la sombra debajo.
func tarjetaLista(src image.Image) *image.RGBA {
	im := escalar(src, 233, 130, true)
	redondear(im, 12)
	sb := sombra(im, 8, 0.6)
	out := image.NewRGBA(image.Rect(0, 0, 267, 166))
	m := (sb.Rect.Dx() - im.Rect.Dx()) / 2
	draw.DrawMask(out, image.Rect(17-m, 13-m+5, 17-m+sb.Rect.Dx(), 13-m+5+sb.Rect.Dy()), image.Black, image.Point{}, sb, image.Point{}, draw.Over)
	draw.Draw(out, image.Rect(17, 13, 250, 143), im, image.Point{}, draw.Over)
	return out
}
