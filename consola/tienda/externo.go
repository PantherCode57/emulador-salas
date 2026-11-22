package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Juegos de PortMaster que no traen el juego («ready to run»: false) porque su autor no ha dado
// permiso para repartirlo, aunque lo dé gratis en su página. PortMaster pide bajarlo a mano en el PC
// y copiarlo a la consola; aquí lo baja la tienda desde la consola, de la página del autor (como
// haría uno con el navegador), y lo deja preparado. El juego nunca pasa por la fuente de la tienda.

type externoPM struct {
	pagina   string // la página del juego
	gamejolt int    // el «build» de GameJolt (su número en la página de descargas)
	archivo  string // cómo se llama lo que se baja
	tam      int64
	// preparar: lo que el .sh del port haría la primera vez que se abre (sacar el juego, ponerle el
	// parche…), hecho al ponerlo, con su barra, para que luego se abra a la primera.
	preparar func(ctx context.Context, p preparacion) error
}

type preparacion struct {
	dir      string   // la carpeta del juego en /roms/ports
	archivo  string   // lo bajado de su página
	pm       []string // dónde puede estar PortMaster (para sus herramientas)
	informar avance
}

// GameJolt da la dirección de cada descarga (firmada y con caducidad) al pulsar «Descargar»; es la
// misma petición que hace su web.
var gamejoltURL = entorno("TIENDA_GAMEJOLT", "https://gamejolt.com/site-api/web/discover/games/builds/get-download-url/")

func direccionGameJolt(ctx context.Context, build int, archivo string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", gamejoltURL+strconv.Itoa(build), strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", agente)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := cliente.Do(req)
	if err != nil {
		return "", explicarRed(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GameJolt contesta %d", resp.StatusCode)
	}
	var r struct {
		Payload struct {
			URL string `json:"url"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return "", fmt.Errorf("GameJolt no da la descarga: %v", err)
	}
	// Tiene que ser ese archivo, y por https (no vale cualquier cosa que llegue).
	u, err := url.Parse(r.Payload.URL)
	if err != nil || (u.Scheme != "https" && !strings.HasPrefix(gamejoltURL, "http://127.0.0.1")) ||
		path.Base(u.Path) != archivo {
		return "", errors.New("GameJolt no da la descarga de este juego")
	}
	return r.Payload.URL, nil
}

// bajarExterno baja el juego de su página a destino.
func bajarExterno(ctx context.Context, x *externoPM, destino string, informar avance) error {
	informar("Bajando", 0, x.tam)
	u, err := direccionGameJolt(ctx, x.gamejolt, x.archivo)
	if err != nil {
		return err
	}
	return bajar(ctx, u, destino, x.tam, "", func(h, tt int64) { informar("Bajando", h, tt) })
}

// carpetaJuego: la carpeta del juego en /roms/ports (lo que no es el .sh).
func carpetaJuego(g *juegoPM) string {
	s := scriptJuego(g)
	for _, it := range g.items {
		if n, ok := nombreSeguro(it); ok && n != s {
			return n
		}
	}
	return ""
}

// carpetasPM: dónde puede estar PortMaster (lo mismo que miran sus .sh, y la de dArkOS).
func (t *tienda) carpetasPM() []string {
	return []string{
		filepath.Join(t.roms, "tools", "PortMaster"),
		filepath.Join(t.raiz, "opt", "system", "Tools", "PortMaster"),
		filepath.Join(t.raiz, "opt", "tools", "PortMaster"),
		filepath.Join(casa, ".local", "share", "PortMaster"),
		filepath.Join(t.ports, "PortMaster"),
	}
}

func herramientaPM(pm []string, nombre string) string {
	for _, d := range pm {
		if r := filepath.Join(d, nombre); tamDe(r) > 0 {
			return r
		}
	}
	if r, err := exec.LookPath(nombre); err == nil {
		return r
	}
	return ""
}

// falta32: lo primero que le falta a la consola para un juego de 32 bits ("" si lo tiene todo).
func (t *tienda) falta32(libs []string) string {
	dirs := []string{"usr/lib32", "lib32", "usr/lib/arm-linux-gnueabihf", "lib/arm-linux-gnueabihf"}
	esta := func(n string, extra ...string) bool {
		for _, d := range append(extra, dirs...) {
			if existe(filepath.Join(t.raiz, filepath.FromSlash(d), n)) {
				return true
			}
		}
		return false
	}
	if !esta("ld-linux-armhf.so.3", "lib", "usr/lib") {
		return "ld-linux-armhf.so.3"
	}
	for _, l := range libs {
		if !esta(l) {
			return l
		}
	}
	return ""
}

// correrPrograma: un programa de un port (7-Zip, xdelta3), con su avance en % si lo dice. Es una
// variable para las pruebas (en el PC no se pueden correr los de la consola).
var correrPrograma = func(ctx context.Context, progreso func(pct int), prog string, args ...string) error {
	cmd := exec.CommandContext(ctx, prog, args...)
	var errores bytes.Buffer
	cmd.Stderr = &errores
	var salida io.ReadCloser
	if progreso != nil {
		var err error
		if salida, err = cmd.StdoutPipe(); err != nil {
			return err
		}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if salida != nil {
		leerPorcentajes(salida, progreso)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s := strings.TrimSpace(errores.String()); s != "" {
			if len(s) > 160 {
				s = s[len(s)-160:]
			}
			return fmt.Errorf("%v: %s", err, s)
		}
		return err
	}
	return nil
}

var rePorcentaje = regexp.MustCompile(`(\d{1,3})%`)

// leerPorcentajes: 7-Zip con -bsp1 va diciendo «  42% 3 - data.win» (con retrocesos, sin saltos de
// línea).
func leerPorcentajes(r io.Reader, progreso func(int)) {
	buf := make([]byte, 4096)
	resto := ""
	for {
		n, err := r.Read(buf)
		if n > 0 {
			resto += string(buf[:n])
			if m := rePorcentaje.FindAllStringSubmatch(resto, -1); len(m) > 0 {
				if p, e := strconv.Atoi(m[len(m)-1][1]); e == nil && p <= 100 {
					progreso(p)
				}
			}
			if len(resto) > 64 {
				resto = resto[len(resto)-64:]
			}
		}
		if err != nil {
			return
		}
	}
}

func md5DeArchivo(ruta string) string {
	h := md5.New()
	if sumar(ruta, h) != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// meterEnApk: mete esos archivos en assets/ del apk, sin comprimir (lo que hacía el .sh con
// «zip -r -0», sin necesitar zip en la consola).
func meterEnApk(apk string, archivos []string) error {
	r, err := zip.OpenReader(apk)
	if err != nil {
		return fmt.Errorf("%s: %v", filepath.Base(apk), err)
	}
	defer r.Close()
	tmp := apk + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	falla := func(err error) error {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	w := zip.NewWriter(f)
	nuevos := map[string]bool{}
	for _, a := range archivos {
		nuevos["assets/"+filepath.Base(a)] = true
	}
	for _, zf := range r.File {
		if nuevos[zf.Name] {
			continue // se cambia por el nuevo
		}
		if err := w.Copy(zf); err != nil {
			return falla(err)
		}
	}
	for _, a := range archivos {
		in, err := os.Open(a)
		if err != nil {
			return falla(err)
		}
		h := &zip.FileHeader{Name: "assets/" + filepath.Base(a), Method: zip.Store, Modified: time.Now()}
		out, err := w.CreateHeader(h)
		if err == nil {
			_, err = io.Copy(out, in)
		}
		in.Close()
		if err != nil {
			return falla(err)
		}
	}
	if err := w.Close(); err != nil {
		return falla(err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	r.Close()
	return os.Rename(tmp, apk)
}

// ---- Geometry Crash ----

// La data.win de la versión 1.0.5 de GameJolt: la que arregla el parche del port (es la que mira
// su .sh antes de ponerlo).
var gcrashDataWin = "4b97bb2da8c515d787fe70aa03550ce5"

// prepararGCrash: lo que hace «Geometry Crash.sh» la primera vez, en el mismo orden. Si se queda a
// medias, el .sh lo sigue haciendo al abrirlo (mira lo mismo).
func prepararGCrash(ctx context.Context, p preparacion) error {
	datos := filepath.Join(p.dir, "gamedata")
	win := filepath.Join(datos, "data.win")
	droid := filepath.Join(datos, "game.droid")
	paso := func(desde, hasta int) func(int) {
		return func(pct int) { p.informar("Preparando", int64(desde+(hasta-desde)*pct/100), 100) }
	}
	// 1. Sacar el juego del instalador de Windows, con el 7-Zip que trae el port.
	if p.archivo != "" {
		siete := filepath.Join(p.dir, "libs", "7zzs")
		_ = os.Chmod(siete, 0o755)
		paso(0, 0)(0)
		if err := correrPrograma(ctx, paso(0, 55), siete, "x", p.archivo, "-o"+datos, "-aoa", "-bsp1", "-bso0"); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("no se ha podido sacar el juego de lo bajado: %v", err)
		}
	}
	// 2. El parche del port (los botones y la pantalla de la consola): de data.win sale game.droid.
	if existe(win) {
		if md5DeArchivo(win) != gcrashDataWin {
			return errors.New("lo de GameJolt no es la versión 1.0.5, la que espera el port de PortMaster")
		}
		x := herramientaPM(p.pm, "xdelta3")
		if x == "" {
			return errors.New("falta xdelta3 de PortMaster: ponlo al día en Ajustes → PortMaster")
		}
		paso(55, 55)(0)
		if err := correrPrograma(ctx, nil, x, "-d", "-f", "-s", win, filepath.Join(p.dir, "patch", "patch.xdelta3"), droid); err != nil {
			_ = os.Remove(droid)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("no se ha podido poner el parche: %v", err)
		}
		_ = os.Remove(win)
	}
	if tamDe(droid) <= 0 {
		return errors.New("no ha salido el juego (falta game.droid)")
	}
	// 3. La música va dentro del apk (assets/), sin comprimir.
	paso(85, 85)(0)
	oggs, _ := filepath.Glob(filepath.Join(datos, "*.ogg"))
	if len(oggs) > 0 {
		if err := meterEnApk(filepath.Join(p.dir, "gcrash.apk"), oggs); err != nil {
			return fmt.Errorf("no se ha podido meter la música: %v", err)
		}
		for _, o := range oggs {
			_ = os.Remove(o)
		}
	}
	// 4. Lo que solo sirve en Windows (el programa y sus librerías): sobra.
	for _, pat := range []string{"*.exe", "*.dll"} {
		l, _ := filepath.Glob(filepath.Join(datos, pat))
		for _, a := range l {
			_ = os.Remove(a)
		}
	}
	p.informar("Preparando", 100, 100)
	return nil
}

// ---- Las partidas ----

func (t *tienda) carpetaPartidas(g *juegoPM) string {
	return filepath.Join(carpetaDatos, "partidas", stem(g.zip))
}

func copiarArchivo(desde, hasta string) error {
	b, err := os.ReadFile(desde)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(hasta), 0o755); err != nil {
		return err
	}
	return escribirAtomico(hasta, b, 0o644)
}

// guardarPartidas: copia las partidas del juego fuera de /roms/ports (antes de quitarlo).
func (t *tienda) guardarPartidas(g *juegoPM) error {
	var hay []string
	for _, pat := range g.partidas {
		l, _ := filepath.Glob(filepath.Join(t.ports, filepath.FromSlash(pat)))
		for _, a := range l {
			if st, err := os.Stat(a); err == nil && st.Mode().IsRegular() && st.Size() < 4<<20 {
				hay = append(hay, a)
			}
		}
	}
	if len(hay) == 0 {
		return nil // no hay nada (y lo guardado de antes se queda)
	}
	dir := t.carpetaPartidas(g)
	nuevo := dir + ".nuevo"
	_ = os.RemoveAll(nuevo)
	for _, a := range hay {
		rel, err := filepath.Rel(t.ports, a)
		if err != nil {
			return err
		}
		if err := copiarArchivo(a, filepath.Join(nuevo, rel)); err != nil {
			_ = os.RemoveAll(nuevo)
			return err
		}
	}
	_ = os.RemoveAll(dir)
	return os.Rename(nuevo, dir)
}

// devolverPartidas: las vuelve a poner en su sitio (al poner el juego).
func (t *tienda) devolverPartidas(g *juegoPM) error {
	dir := t.carpetaPartidas(g)
	if !existe(dir) {
		return nil
	}
	carpeta := carpetaJuego(g)
	return filepath.Walk(dir, func(r string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, r)
		if err != nil {
			return err
		}
		// solo dentro de la carpeta del juego
		if carpeta == "" || strings.SplitN(filepath.ToSlash(rel), "/", 2)[0] != carpeta {
			return nil
		}
		return copiarArchivo(r, filepath.Join(t.ports, rel))
	})
}
