package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tiendaDePrueba: una tienda con todo en carpetas temporales.
func tiendaDePrueba(t *testing.T) *tienda {
	t.Helper()
	tmp := t.TempDir()
	carpetaDatos = filepath.Join(tmp, "datos")
	tt := nuevaTienda()
	tt.local = filepath.Join(tmp, "panther-instalar")
	tt.panther = filepath.Join(tmp, "panther")
	tt.roms = filepath.Join(tmp, "roms")
	tt.ports = filepath.Join(tt.roms, "ports")
	tt.descargas = filepath.Join(tt.roms, ".panther-tienda")
	tt.puestos = filepath.Join(carpetaDatos, "puestos")
	tt.indice = nil
	return tt
}

func escribir(t *testing.T, ruta, texto string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(texto), 0o644); err != nil {
		t.Fatal(err)
	}
}

func leer(t *testing.T, ruta string) string {
	t.Helper()
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func nada(string, int64, int64) {}

func TestPonerYQuitarApp(t *testing.T) {
	tt := tiendaDePrueba(t)
	escribir(t, filepath.Join(tt.local, "claude-arm64"), "claude viejo")
	escribir(t, filepath.Join(tt.local, "Claude.sh"), "#!/bin/bash\n")
	escribir(t, filepath.Join(tt.panther, "whatsapp", "pokemulador_libretro.so"), "nucleo")
	tt.mirar()
	e := porID(tt.apps, "claude")
	if e.puesto || !e.disponible {
		t.Fatalf("claude: puesto=%v disponible=%v (%s)", e.puesto, e.disponible, e.motivo)
	}
	if nav := porID(tt.apps, "navegador"); nav.disponible {
		t.Fatal("el navegador no tiene copia: no debería poder ponerse")
	}
	ctx := context.Background()
	if err := tt.ponerApp(ctx, trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	tt.mirar()
	if !e.puesto {
		t.Fatal("claude no sale como puesta")
	}
	if got := leer(t, filepath.Join(tt.panther, "claude", "pokemulador_libretro.so")); got != "nucleo" {
		t.Fatalf("núcleo: %q", got)
	}
	// Se pone al día en su sitio (como pasa en la consola) y se quita: tiene que volver ESA versión.
	escribir(t, filepath.Join(tt.panther, "claude", "claude"), "claude nuevo")
	escribir(t, filepath.Join(tt.panther, "claude", "claude.antes-rapido"), "copia")
	if err := tt.quitarApp(ctx, trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	tt.mirar()
	if e.puesto || existe(filepath.Join(tt.roms, "claude", "Claude.sh")) || existe(filepath.Join(tt.panther, "claude")) {
		t.Fatal("claude no se ha quitado del todo")
	}
	if err := tt.ponerApp(ctx, trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	if got := leer(t, filepath.Join(tt.panther, "claude", "claude")); got != "claude nuevo" {
		t.Fatalf("al volver a ponerla sale %q (la de ~/panther-instalar es más vieja)", got)
	}
	if existe(tt.guardadas("claude")) {
		t.Fatal("la copia guardada se tenía que ir")
	}
}

func TestWhatsAppSoloTarjeta(t *testing.T) {
	tt := tiendaDePrueba(t)
	escribir(t, filepath.Join(tt.panther, "whatsapp", "whatsapp"), "servicio")
	escribir(t, filepath.Join(tt.panther, "whatsapp", "pokemulador_libretro.so"), "nucleo")
	escribir(t, filepath.Join(tt.roms, "whatsapp", "WhatsApp.sh"), "#!/bin/bash\n")
	tt.mirar()
	e := porID(tt.apps, "whatsapp")
	if !e.puesto {
		t.Fatal("whatsapp tenía que salir como puesta")
	}
	if err := tt.quitarApp(context.Background(), trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	if !existe(filepath.Join(tt.panther, "whatsapp", "whatsapp")) {
		t.Fatal("el programa de WhatsApp es el servicio: no se puede borrar")
	}
	tt.mirar()
	if e.puesto || !e.disponible {
		t.Fatalf("puesto=%v disponible=%v", e.puesto, e.disponible)
	}
	if err := tt.ponerApp(context.Background(), trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	if !existe(filepath.Join(tt.roms, "whatsapp", "WhatsApp.sh")) || leer(t, filepath.Join(tt.panther, "whatsapp", "whatsapp")) != "servicio" {
		t.Fatal("tenía que volver la tarjeta sin tocar el programa")
	}
}

func TestSpotinLeDejaYtdlpAYouTube(t *testing.T) {
	tt := tiendaDePrueba(t)
	for _, id := range []string{"spotin", "youtube"} {
		escribir(t, filepath.Join(tt.panther, id, id), id)
		escribir(t, filepath.Join(tt.panther, id, "pokemulador_libretro.so"), "nucleo")
	}
	escribir(t, filepath.Join(tt.panther, "spotin", "bin", "yt-dlp"), "ytdlp")
	escribir(t, filepath.Join(tt.roms, "spotin", "Spotin.sh"), "#!/bin/bash\n")
	e := porID(tt.apps, "spotin")
	if err := tt.quitarApp(context.Background(), trabajoApp{e: e, otraYT: true}, nada); err != nil {
		t.Fatal(err)
	}
	if leer(t, filepath.Join(tt.panther, "youtube", "bin", "yt-dlp")) != "ytdlp" {
		t.Fatal("YouTube se ha quedado sin yt-dlp")
	}
}

// zipDePrueba: un juego de mentira con su .sh, su carpeta y su gameinfo.
func zipDePrueba(t *testing.T, extra map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	archivos := map[string]string{
		"Juego.sh":           "#!/bin/bash\necho hola\n",
		"juego/juego":        "binario",
		"juego/datos/a.dat":  strings.Repeat("x", 5000),
		"juego/gameinfo.xml": "<gameList/>",
	}
	for k, v := range extra {
		archivos[k] = v
	}
	for n, c := range archivos {
		w, err := z.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(c))
	}
	z.Close()
	return b.Bytes()
}

func servir(t *testing.T, datos []byte) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "juego.zip", modTime0, bytes.NewReader(datos))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/juego.zip"
}

func TestPonerYQuitarJuego(t *testing.T) {
	tt := tiendaDePrueba(t)
	escribir(t, filepath.Join(tt.ports, "gamelist.xml"), "<?xml version=\"1.0\"?>\n<gameList>\n\t<game>\n\t\t<path>./Otro.sh</path>\n\t\t<name>Otro</name>\n\t</game>\n</gameList>\n")
	datos := zipDePrueba(t, nil)
	suma := md5.Sum(datos)
	g := juegoPM{zip: "juego.zip", url: servir(t, datos), md5: hex.EncodeToString(suma[:]), tam: int64(len(datos)),
		items: []string{"Juego.sh", "juego/"}, anio: 2004, autor: "Alguien & Cía"}
	tr := trabajoJuego{g: g, nombre: "El Juego <1>", larga: "De qué va", genero: "Plataformas"}
	if err := tt.ponerJuego(context.Background(), tr, nada); err != nil {
		t.Fatal(err)
	}
	if leer(t, filepath.Join(tt.ports, "juego", "datos", "a.dat")) != strings.Repeat("x", 5000) {
		t.Fatal("el juego no se ha descomprimido bien")
	}
	lista := leer(t, filepath.Join(tt.ports, "gamelist.xml"))
	for _, quiero := range []string{"<path>./Otro.sh</path>", "<path>./Juego.sh</path>", "<name>El Juego &lt;1&gt;</name>",
		"<developer>Alguien &amp; Cía</developer>", "<releasedate>20040101T000000</releasedate>"} {
		if !strings.Contains(lista, quiero) {
			t.Fatalf("falta %s en la lista:\n%s", quiero, lista)
		}
	}
	if existe(filepath.Join(tt.descargas, "juego.zip")) || existe(tt.marcaAMedias("juego.zip")) {
		t.Fatal("se ha quedado el zip o la marca")
	}
	// Otra vez: no se repite en la lista.
	if err := tt.ponerJuego(context.Background(), tr, nada); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(leer(t, filepath.Join(tt.ports, "gamelist.xml")), "./Juego.sh"); n != 1 {
		t.Fatalf("el juego sale %d veces en la lista", n)
	}
	if err := tt.quitarJuego(context.Background(), tr, nada); err != nil {
		t.Fatal(err)
	}
	if existe(filepath.Join(tt.ports, "Juego.sh")) || existe(filepath.Join(tt.ports, "juego")) {
		t.Fatal("el juego no se ha quitado")
	}
	lista = leer(t, filepath.Join(tt.ports, "gamelist.xml"))
	if strings.Contains(lista, "Juego.sh") || !strings.Contains(lista, "./Otro.sh") {
		t.Fatalf("la lista ha quedado mal:\n%s", lista)
	}
}

func TestZipConRutasRaras(t *testing.T) {
	tt := tiendaDePrueba(t)
	for _, malo := range []string{"../fuera.txt", "PortMaster/control.txt", "gamelist.xml"} {
		datos := zipDePrueba(t, map[string]string{malo: "mal"})
		suma := md5.Sum(datos)
		g := juegoPM{zip: "juego.zip", url: servir(t, datos), md5: hex.EncodeToString(suma[:]), tam: int64(len(datos)),
			items: []string{"Juego.sh", "juego/"}}
		if err := tt.ponerJuego(context.Background(), trabajoJuego{g: g, nombre: "J"}, nada); err == nil {
			t.Fatalf("un zip con %s no se tenía que poner", malo)
		}
	}
	if existe(filepath.Join(tt.roms, "fuera.txt")) {
		t.Fatal("se ha escrito fuera de /roms/ports")
	}
}

func TestSumaMal(t *testing.T) {
	tt := tiendaDePrueba(t)
	datos := zipDePrueba(t, nil)
	g := juegoPM{zip: "juego.zip", url: servir(t, datos), md5: "00000000000000000000000000000000", tam: int64(len(datos)),
		items: []string{"Juego.sh", "juego/"}}
	if err := tt.ponerJuego(context.Background(), trabajoJuego{g: g, nombre: "J"}, nada); err == nil {
		t.Fatal("con la suma mal no se tenía que poner")
	}
	if existe(filepath.Join(tt.ports, "Juego.sh")) {
		t.Fatal("se ha puesto algo")
	}
}

func TestCatalogoPMManda(t *testing.T) {
	juegos := catalogoJuegos()
	cat := map[string]entradaPM{}
	var nuevo, potente entradaPM
	nuevo.Attr.RTR = true
	nuevo.Source.URL, nuevo.Source.MD5, nuevo.Source.Size = "https://x/nuevo.zip", "abc", 123
	potente.Attr.RTR = true
	potente.Attr.Reqs = []string{"power"}
	potente.Source.URL = "https://x/p.zip"
	cat["cceleste.zip"] = nuevo
	cat["srb2.zip"] = potente
	aplicarCatalogoPM(juegos, cat)
	if g := porID(juegos, "cceleste.zip").juego; g.url != "https://x/nuevo.zip" || g.tam != 123 {
		t.Fatalf("no se ha cogido lo de PortMaster: %+v", g)
	}
	if g := porID(juegos, "srb2.zip").juego; g.url != "" {
		t.Fatal("un juego que pide «power» no se tenía que ofrecer")
	}
}

var modTime0 = time.Unix(0, 0)
