package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// apkDePrueba: un apk con lo que trae el de verdad (el cargador y una imagen).
func apkDePrueba(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, c := range map[string]string{"assets/splash.png": "png", "lib/armeabi-v7a/libyoyo.so": "yoyo"} {
		w, _ := z.Create(n)
		w.Write([]byte(c))
	}
	z.Close()
	return b.String()
}

// gcrashDePrueba: Geometry Crash con su port y «GameJolt» de mentira. Devuelve el juego y cuántas
// veces se ha bajado el juego de GameJolt.
func gcrashDePrueba(t *testing.T, tt *tienda) (juegoPM, *int32) {
	t.Helper()
	port := zipDePrueba(t, map[string]string{
		"Geometry Crash.sh":          "#!/bin/bash\n",
		"gcrash/gcrash.apk":          apkDePrueba(t),
		"gcrash/libs/7zzs":           "7-zip",
		"gcrash/patch/patch.xdelta3": "parche",
		"gcrash/gamedata/data.ini":   "[game]\nresolution=\"720\"\n",
		"gcrash/gmloader":            "cargador",
		"gcrash/screenshot.png":      "",
	})
	sumaPort := md5.Sum(port)
	exe := strings.Repeat("instalador de windows ", 1000)
	var bajadas int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/port/gcrash.zip":
			http.ServeContent(w, r, "gcrash.zip", modTime0, bytes.NewReader(port))
		case r.URL.Path == "/api/803726" && r.Method == "POST":
			w.Write([]byte(`{"payload":{"url":"` + srv.URL + `/descarga/abc,123/geometry-crash-1.0.5.exe"}}`))
		case r.URL.Path == "/descarga/abc,123/geometry-crash-1.0.5.exe":
			atomic.AddInt32(&bajadas, 1)
			http.ServeContent(w, r, "x.exe", modTime0, strings.NewReader(exe))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	viejo := gamejoltURL
	gamejoltURL = srv.URL + "/api/"
	t.Cleanup(func() { gamejoltURL = viejo })

	// 7-Zip y xdelta3 de mentira: hacen lo mismo con los archivos.
	win := "data.win de la 1.0.5"
	s := md5.Sum([]byte(win))
	viejaSuma := gcrashDataWin
	gcrashDataWin = hex.EncodeToString(s[:])
	t.Cleanup(func() { gcrashDataWin = viejaSuma })
	viejoCorrer := correrPrograma
	correrPrograma = func(ctx context.Context, progreso func(int), prog string, args ...string) error {
		switch filepath.Base(prog) {
		case "7zzs":
			if b, err := os.ReadFile(args[1]); err != nil || string(b) != exe {
				return errors.New("7zzs: no es el instalador")
			}
			datos := strings.TrimPrefix(args[2], "-o")
			for n, c := range map[string]string{"data.win": win, "musica.ogg": "ogg", "Geometry Crash.exe": "exe",
				"D3DX9_43.dll": "dll", "options.ini": "[Windows]"} {
				if err := os.WriteFile(filepath.Join(datos, n), []byte(c), 0o644); err != nil {
					return err
				}
			}
			progreso(50)
			progreso(100)
		case "xdelta3":
			// -d -f -s data.win patch.xdelta3 game.droid
			b, err := os.ReadFile(args[3])
			if err != nil {
				return err
			}
			return os.WriteFile(args[5], append([]byte("parcheado: "), b...), 0o644)
		default:
			return errors.New("programa inesperado: " + prog)
		}
		return nil
	}
	t.Cleanup(func() { correrPrograma = viejoCorrer })
	escribir(t, filepath.Join(tt.roms, "tools", "PortMaster", "xdelta3"), "xdelta3")

	g := *porID(catalogoJuegos(), "gcrash.zip").juego
	x := *g.externo
	x.tam = int64(len(exe))
	g.externo = &x
	g.url, g.md5, g.tam = srv.URL+"/port/gcrash.zip", hex.EncodeToString(sumaPort[:]), int64(len(port))
	return g, &bajadas
}

func TestGeometryCrash(t *testing.T) {
	tt := tiendaDePrueba(t)
	g, bajadas := gcrashDePrueba(t, tt)
	tr := trabajoJuego{g: g, nombre: "Geometry Crash", larga: "De ritmo", genero: "Ritmo"}
	var fases []string
	informar := func(f string, h, tot int64) {
		if len(fases) == 0 || fases[len(fases)-1] != f {
			fases = append(fases, f)
		}
	}
	if err := tt.ponerJuego(context.Background(), tr, informar); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fases, ",") != "Bajando,Descomprimiendo,Bajando,Preparando,Poniendo" {
		t.Fatalf("fases: %v", fases)
	}
	datos := filepath.Join(tt.ports, "gcrash", "gamedata")
	if leer(t, filepath.Join(datos, "game.droid")) != "parcheado: data.win de la 1.0.5" {
		t.Fatal("no se ha puesto el parche")
	}
	for _, n := range []string{"data.win", "musica.ogg", "Geometry Crash.exe", "D3DX9_43.dll"} {
		if existe(filepath.Join(datos, n)) {
			t.Fatalf("sobra %s", n)
		}
	}
	z, err := zip.OpenReader(filepath.Join(tt.ports, "gcrash", "gcrash.apk"))
	if err != nil {
		t.Fatal(err)
	}
	vistos := map[string]uint16{}
	for _, f := range z.File {
		vistos[f.Name] = f.Method
	}
	z.Close()
	if m, ok := vistos["assets/musica.ogg"]; !ok || m != zip.Store || len(vistos) != 3 {
		t.Fatalf("el apk ha quedado mal: %v", vistos)
	}
	if existe(tt.descargas) || existe(tt.marcaAMedias("gcrash.zip")) {
		t.Fatal("se ha quedado algo en las descargas o la marca")
	}
	lista := leer(t, filepath.Join(tt.ports, "gamelist.xml"))
	if !strings.Contains(lista, "<path>./Geometry Crash.sh</path>") || !strings.Contains(lista, "<developer>BootDark</developer>") {
		t.Fatalf("la lista:\n%s", lista)
	}

	// Se juega, se quita (las partidas se quedan) y se vuelve a poner: sigue donde iba.
	escribir(t, filepath.Join(datos, "data.ini"), "[Niveles]\nnivel1=\"100\"\n")
	if err := tt.quitarJuego(context.Background(), tr, nada); err != nil {
		t.Fatal(err)
	}
	if existe(filepath.Join(tt.ports, "gcrash")) || existe(filepath.Join(tt.ports, "Geometry Crash.sh")) {
		t.Fatal("no se ha quitado")
	}
	if err := tt.ponerJuego(context.Background(), tr, nada); err != nil {
		t.Fatal(err)
	}
	if leer(t, filepath.Join(datos, "data.ini")) != "[Niveles]\nnivel1=\"100\"\n" {
		t.Fatal("se han perdido las partidas")
	}
	if *bajadas != 2 {
		t.Fatalf("se ha bajado %d veces de GameJolt", *bajadas)
	}
}

// Si lo de GameJolt no es la versión del parche, no se pone (y no sale como puesto).
func TestGeometryCrashOtraVersion(t *testing.T) {
	tt := tiendaDePrueba(t)
	g, _ := gcrashDePrueba(t, tt)
	gcrashDataWin = "00000000000000000000000000000000"
	err := tt.ponerJuego(context.Background(), trabajoJuego{g: g, nombre: "Geometry Crash"}, nada)
	if err == nil || !strings.Contains(err.Error(), "1.0.5") {
		t.Fatalf("tenía que fallar por la versión: %v", err)
	}
	if !existe(tt.marcaAMedias("gcrash.zip")) {
		t.Fatal("tiene que quedar a medias (no puesto)")
	}
}

func TestGameJoltSoloEseArchivo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"payload":{"url":"https://otro.sitio/virus.exe"}}`))
	}))
	defer srv.Close()
	viejo := gamejoltURL
	gamejoltURL = srv.URL + "/"
	defer func() { gamejoltURL = viejo }()
	if _, err := direccionGameJolt(context.Background(), 1, "geometry-crash-1.0.5.exe"); err == nil {
		t.Fatal("no tenía que aceptar otro archivo")
	}
}

func TestGeometryCrash32Bits(t *testing.T) {
	tt := tiendaDePrueba(t)
	tt.raiz = t.TempDir()
	tt.mirar()
	e := porID(tt.juegos, "gcrash.zip")
	if e.disponible || !strings.Contains(e.motivo, "32 bits") {
		t.Fatalf("sin las librerías de 32 bits no se tenía que poder: %v %q", e.disponible, e.motivo)
	}
	escribir(t, filepath.Join(tt.raiz, "lib", "ld-linux-armhf.so.3"), "x")
	for _, l := range e.juego.libs32 {
		escribir(t, filepath.Join(tt.raiz, "usr", "lib", "arm-linux-gnueabihf", l), "x")
	}
	tt.mirar()
	if !e.disponible || e.tam != e.juego.tam+e.juego.externo.tam {
		t.Fatalf("con todo, sí: %v %q tam %d", e.disponible, e.motivo, e.tam)
	}
}

func TestCatalogoPMGeometryCrash(t *testing.T) {
	juegos := catalogoJuegos()
	var c entradaPM // como en el ports.json de verdad: no «ready to run» y solo armhf
	c.Attr.Arch = []string{"armhf"}
	c.Source.URL, c.Source.MD5 = "https://x/gcrash-nuevo.zip", "otra"
	aplicarCatalogoPM(juegos, map[string]entradaPM{"gcrash.zip": c})
	g := porID(juegos, "gcrash.zip").juego
	if g.url == "" || g.md5 != "0fdf67998bd9ee36edd126efbfc33bcb" {
		t.Fatalf("el port de Geometry Crash va fijo: %+v", g)
	}
}

func TestLeerPorcentajes(t *testing.T) {
	var vistos []int
	leerPorcentajes(strings.NewReader("\b\b\b  12% 3 - data.win\b\b\b\b 57% 9 - musica.ogg\b\b100%"), func(p int) { vistos = append(vistos, p) })
	if len(vistos) == 0 || vistos[len(vistos)-1] != 100 {
		t.Fatalf("%v", vistos)
	}
}
