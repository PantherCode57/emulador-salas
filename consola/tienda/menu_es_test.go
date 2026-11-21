package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sistemasDePrueba = `<?xml version="1.0"?>
<systemList>
	<system>
		<name>claude</name>
		<fullname>Claude</fullname>
		<path>/roms/claude/</path>
	</system>
	<system>
		<name>pokemon</name>
		<fullname>Pokémon</fullname>
		<path>/roms/pokemon/</path>
	</system>
</systemList>
`

// conMenu: la tienda de prueba con un menú de EmulationStation de mentira.
func conMenu(t *testing.T) *tienda {
	tt := tiendaDePrueba(t)
	base := filepath.Dir(tt.panther)
	tt.esSistemas = []string{filepath.Join(base, "etc", "es_systems.cfg"), filepath.Join(base, "etc", "es_systems.sd1")}
	tt.esAjustes = filepath.Join(base, "es", "es_settings.cfg")
	tt.tema = filepath.Join(tt.roms, "themes", "es-theme-panther")
	tt.tarjetas = filepath.Join(tt.tema, "_panther", "tarjetas")
	escribir(t, tt.esSistemas[0], sistemasDePrueba) // el .sd1 no está
	escribir(t, filepath.Join(tt.tarjetas, "claude.png"), "tarjeta")
	escribir(t, filepath.Join(tt.tema, "claude", "theme.xml"), "tema de claude")
	escribir(t, filepath.Join(tt.local, "tema-panther", "tarjetas", "claude.png"), "tarjeta")
	tt.tarjetaApp = func(e *elemento, ruta string) error {
		return os.WriteFile(ruta, []byte("tarjeta de "+e.id), 0o644)
	}
	return tt
}

func TestAltaDeUnaAppNueva(t *testing.T) {
	tt := conMenu(t)
	escribir(t, filepath.Join(tt.local, "pokedex-arm64"), "pokedex")
	escribir(t, filepath.Join(tt.local, "Pokedex.sh"), "#!/bin/bash\n")
	escribir(t, filepath.Join(tt.panther, "whatsapp", "pokemulador_libretro.so"), "nucleo")
	tt.mirar()
	e := porID(tt.apps, "pokedex")
	if e == nil || e.puesto || !e.disponible {
		t.Fatalf("pokedex: %+v", e)
	}
	ctx := context.Background()
	if err := tt.ponerApp(ctx, trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	s := leer(t, tt.esSistemas[0])
	if strings.Count(s, "<name>pokedex</name>") != 1 || !strings.Contains(s, "<path>/roms/pokedex/</path>") ||
		!strings.Contains(s, "<fullname>Pokédex</fullname>") || !strings.Contains(s, "<name>claude</name>") {
		t.Fatalf("es_systems.cfg:\n%s", s)
	}
	if leer(t, tt.esSistemas[0]+".antes-tienda") != sistemasDePrueba {
		t.Fatal("no se guardó la copia de es_systems.cfg")
	}
	if existe(tt.esSistemas[1]) {
		t.Fatal("se creó es_systems.sd1, que no estaba")
	}
	if leer(t, filepath.Join(tt.tarjetas, "pokedex.png")) != "tarjeta de pokedex" {
		t.Fatal("falta la tarjeta")
	}
	if !existe(filepath.Join(tt.local, "tema-panther", "tarjetas", "pokedex.png")) {
		t.Fatal("falta la copia de la tarjeta en las piezas del tema")
	}
	x := leer(t, filepath.Join(tt.tema, "pokedex", "theme.xml"))
	if !strings.Contains(x, "./../_panther/tarjetas/pokedex.png") || !strings.Contains(x, "<include>./../theme.xml</include>") {
		t.Fatalf("theme.xml:\n%s", x)
	}
	// Quitar y poner otra vez: nada repetido.
	tt.mirar()
	if err := tt.quitarApp(ctx, trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	tt.mirar()
	if err := tt.ponerApp(ctx, trabajoApp{e: e}, nada); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(leer(t, tt.esSistemas[0]), "<name>pokedex</name>"); n != 1 {
		t.Fatalf("el apartado sale %d veces", n)
	}
	// Las que ya estaban dadas de alta no cambian.
	escribir(t, filepath.Join(tt.local, "claude-arm64"), "claude")
	escribir(t, filepath.Join(tt.local, "Claude.sh"), "#!/bin/bash\n")
	antes := leer(t, tt.esSistemas[0])
	tt.mirar()
	if err := tt.ponerApp(ctx, trabajoApp{e: porID(tt.apps, "claude")}, nada); err != nil {
		t.Fatal(err)
	}
	if leer(t, tt.esSistemas[0]) != antes || leer(t, filepath.Join(tt.tarjetas, "claude.png")) != "tarjeta" ||
		leer(t, filepath.Join(tt.tema, "claude", "theme.xml")) != "tema de claude" {
		t.Fatal("se ha tocado lo de Claude")
	}
}

func TestAltaSinMenu(t *testing.T) {
	tt := conMenu(t)
	escribir(t, tt.esSistemas[0], "<?xml version=\"1.0\"?>\n<nada/>\n")
	e := porID(tt.apps, "pokedex")
	if err := tt.darDeAlta(e); err == nil {
		t.Fatal("sin <systemList> tendría que fallar")
	}
}

const ajustesDePrueba = `<?xml version="1.0"?>
<string name="ThemeSet" value="es-theme-panther" />
<string name="PantherDirectLaunch" value="whatsapp,spotin,pokemon,youtube,navegador,instagram,claude,duolingo,ajedrez,zarpa,tienda,salas" />
<string name="PantherSystemOrder" value="whatsapp,claude,instagram,duolingo,ajedrez,zarpa,youtube,spotin,pokemon,navegador,ports,Emuladores,tienda,salas" />
`

func TestAjustesDelMenu(t *testing.T) {
	apps := catalogoApps()
	puestas := map[string]bool{"whatsapp": true, "claude": true, "pokemon": true, "navegador": true}
	for _, e := range apps {
		e.puesto = puestas[e.id]
	}
	if _, cambia := ajustesES(ajustesDePrueba, apps); cambia {
		t.Fatal("sin la Pokédex no tendría que cambiar nada")
	}
	porID(apps, "pokedex").puesto = true
	nuevo, cambia := ajustesES(ajustesDePrueba, apps)
	if !cambia {
		t.Fatal("con la Pokédex tendría que cambiar")
	}
	if !strings.Contains(nuevo, `"whatsapp,claude,instagram,duolingo,ajedrez,zarpa,youtube,spotin,pokemon,pokedex,navegador,ports,Emuladores,tienda,salas"`) {
		t.Fatalf("orden:\n%s", nuevo)
	}
	if !strings.Contains(nuevo, `name="PantherDirectLaunch" value="whatsapp,spotin,pokemon,youtube,navegador,instagram,claude,duolingo,ajedrez,zarpa,pokedex,tienda,salas"`) {
		t.Fatalf("apertura directa:\n%s", nuevo)
	}
	if !strings.Contains(nuevo, `<string name="ThemeSet" value="es-theme-panther" />`) {
		t.Fatal("se ha perdido otro ajuste")
	}
	if otra, cambia := ajustesES(nuevo, apps); cambia || otra != nuevo {
		t.Fatal("la segunda vez no tendría que cambiar nada")
	}
	// Sin los ajustes de Panther OS no se añade nada.
	solo := "<?xml version=\"1.0\"?>\n<string name=\"ThemeSet\" value=\"x\" />\n"
	if _, cambia := ajustesES(solo, apps); cambia {
		t.Fatal("sin PantherSystemOrder no tendría que cambiar")
	}
	// Si ninguna de antes está en la lista, delante de Ports.
	l := ponerEnLista(strings.Split("ports,Emuladores,tienda", ","), "pokedex", []string{"whatsapp"}, "ports", "Emuladores")
	if strings.Join(l, ",") != "pokedex,ports,Emuladores,tienda" {
		t.Fatalf("delante de ports: %v", l)
	}
}

func TestPonerAjustes(t *testing.T) {
	tt := conMenu(t)
	escribir(t, tt.esAjustes, ajustesDePrueba)
	escribir(t, filepath.Join(tt.roms, "pokedex", "Pokedex.sh"), "#!/bin/bash\n")
	escribir(t, filepath.Join(tt.panther, "pokedex", "pokedex"), "programa")
	if !tt.ajustesPendientes() {
		t.Fatal("con la Pokédex puesta hay ajustes pendientes")
	}
	if err := tt.ponerAjustes(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(leer(t, tt.esAjustes), "pokemon,pokedex,navegador") {
		t.Fatalf("es_settings.cfg:\n%s", leer(t, tt.esAjustes))
	}
	if leer(t, tt.esAjustes+".antes-tienda") != ajustesDePrueba {
		t.Fatal("falta la copia de es_settings.cfg")
	}
	if tt.ajustesPendientes() {
		t.Fatal("ya no tendría que quedar nada")
	}
}

func TestPonerseAlDia(t *testing.T) {
	nueva := []byte("tienda nueva")
	h := sha256.Sum256(nueva)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tienda" {
			_, _ = w.Write(nueva)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()
	yo := filepath.Join(dir, "tienda")
	escribir(t, yo, "tienda vieja")
	ind := &indiceTienda{Version: "abc", Piezas: map[string]piezaIndex{"tienda": {Tam: int64(len(nueva)), SHA256: hex.EncodeToString(h[:])}}}
	ctx := context.Background()
	cambiada, err := ponerseAlDia(ctx, srv.URL+"/", ind, yo, func(int64, int64) {})
	if err != nil || !cambiada {
		t.Fatalf("cambiada=%v err=%v", cambiada, err)
	}
	if leer(t, yo) != "tienda nueva" || leer(t, yo+".antes") != "tienda vieja" {
		t.Fatalf("yo=%q antes=%q", leer(t, yo), leer(t, yo+".antes"))
	}
	if cambiada, err := ponerseAlDia(ctx, srv.URL+"/", ind, yo, func(int64, int64) {}); cambiada || err != nil {
		t.Fatalf("la segunda vez: cambiada=%v err=%v", cambiada, err)
	}
	// Si lo bajado no es lo que dice tienda.json, se queda la que hay.
	escribir(t, yo, "tienda vieja")
	ind.Piezas["tienda"] = piezaIndex{Tam: int64(len(nueva)), SHA256: strings.Repeat("0", 64)}
	if cambiada, err := ponerseAlDia(ctx, srv.URL+"/", ind, yo, func(int64, int64) {}); cambiada || err == nil {
		t.Fatalf("con la suma mal: cambiada=%v err=%v", cambiada, err)
	}
	if leer(t, yo) != "tienda vieja" {
		t.Fatal("se ha cambiado la tienda con una suma mala")
	}
	// Sin la pieza en la fuente, nada.
	if cambiada, err := ponerseAlDia(ctx, srv.URL+"/", &indiceTienda{Piezas: map[string]piezaIndex{}}, yo, nil); cambiada || err != nil {
		t.Fatal("sin la pieza no tendría que hacer nada")
	}
}

func TestTarjetaDeApp(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "pokedex.png")
	if err := tarjetaDeApp(cargarFuentes(), porID(catalogoApps(), "pokedex"), ruta); err != nil {
		t.Fatal(err)
	}
	im := abrirImagen(ruta)
	if im == nil || im.Bounds().Dx() != 264 || im.Bounds().Dy() != 152 {
		t.Fatalf("la tarjeta no es de 264×152: %v", im)
	}
}
