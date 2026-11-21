package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSitioPartida(t *testing.T) {
	casos := []struct {
		hay   []string
		juego string
		sitio string
	}{
		{[]string{"SAVER", "Pokemon Esmeralda.gba"}, "Pokemon Esmeralda", "SAVER/Pokemon Esmeralda.sav"},
		{[]string{"GBASYS", "GBASYS/save", "juegos/Pokemon.gba"}, "Pokemon", "GBASYS/save/Pokemon.sav"},
		{[]string{"juegos", "juegos/pokemon rojo fuego.GBA"}, "Pokemon Rojo Fuego", "juegos/Pokemon Rojo Fuego.sav"},
		{[]string{"otra cosa.txt"}, "Zelda", "PARTIDAS GBA/Zelda.sav"},
	}
	for _, c := range casos {
		if got := sitioPartida(c.hay, c.juego); got != c.sitio {
			t.Errorf("sitioPartida(%v, %q) = %q, quería %q", c.hay, c.juego, got, c.sitio)
		}
	}
}

// consolaFalsa: /roms, ~/.config/panther, los wifis y la Panther Store, en carpetas de mentira.
func consolaFalsa(t *testing.T) {
	r := t.TempDir()
	viejos := []string{dirRoms, dirConfig, dirRedes, binTienda}
	dirRoms, dirConfig, dirRedes = filepath.Join(r, "roms"), filepath.Join(r, "config"), filepath.Join(r, "redes")
	binTienda = filepath.Join(r, "tienda")
	t.Cleanup(func() { dirRoms, dirConfig, dirRedes, binTienda = viejos[0], viejos[1], viejos[2], viejos[3] })
	poner := func(ruta, texto string) {
		p := filepath.Join(r, ruta)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(texto), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	poner("tienda", "programa de la tienda")
	poner("roms/gba/Pokemon Esmeralda.gba", "juego comercial: no se copia")
	poner("roms/gba/Pokemon Esmeralda.srm", "partida esmeralda")
	poner("roms/gb/Tetris.sav", "partida tetris")
	poner("roms/gba/.ocultos/x.srm", "no")
	poner("config/whatsapp/sesion.db", "sesión de whatsapp: no se copia")
	poner("config/instagram/cuenta.json", `{"sessionid":"x"}`)
	poner("config/instagram/registro.txt", "registro: no hace falta")
	poner("config/spotin/cuenta.json", "spotin")
	poner("redes/Casa.nmconnection", "[wifi]\nssid=Casa\n")
}

func TestPartidas(t *testing.T) {
	consolaFalsa(t)
	ps := buscarPartidas("gba")
	if len(ps) != 1 || ps[0].juego != "Pokemon Esmeralda" {
		t.Fatalf("partidas de GBA: %+v", ps)
	}
	c, err := cargaGBA(ps)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.partidas) != 1 || c.partidas[0].ruta != "Pokemon Esmeralda.sav" || string(c.partidas[0].datos) != "partida esmeralda" {
		t.Fatalf("carga de GBA: %+v", c.partidas)
	}
	if todas := buscarPartidas(""); len(todas) != 2 {
		t.Fatalf("todas las partidas: %+v", todas)
	}
}

func rutas(as []archivo) map[string]archivo {
	r := map[string]archivo{}
	for _, a := range as {
		r[a.ruta] = a
	}
	return r
}

func TestDarkOSParaMi(t *testing.T) {
	consolaFalsa(t)
	apps := []appTienda{{ID: "youtube", Nombre: "YouTube"}, {ID: "cave.story-evo.zip", Nombre: "Cave Story", Tipo: "juego"}}
	c, err := cargaDarkOS(apps, true)
	if err != nil {
		t.Fatal(err)
	}
	s := rutas(c.sistema)
	if string(s["home/ark/panther/flasheo/apps.txt"].datos) != "youtube\ncave.story-evo.zip\n" {
		t.Fatalf("apps.txt: %q", s["home/ark/panther/flasheo/apps.txt"].datos)
	}
	if a := s["home/ark/panther/flasheo/tienda"]; string(a.datos) != "programa de la tienda" || a.modo != 0o755 || !a.ark {
		t.Fatalf("la tienda: %+v", a)
	}
	if !strings.Contains(string(s["home/ark/panther/flasheo/instalar.sh"].datos), "tienda\" -poner") {
		t.Fatal("falta el instalador")
	}
	if s["etc/systemd/system/multi-user.target.wants/panther-flasheo.service"].enlace != "/etc/systemd/system/panther-flasheo.service" {
		t.Fatal("falta el enlace para que arranque solo")
	}
	if _, ok := s["home/ark/panther/flasheo/cuentas/instagram/cuenta.json"]; !ok {
		t.Fatal("tendría que llevar la cuenta de Instagram")
	}
	if _, ok := s["home/ark/panther/flasheo/cuentas/instagram/registro.txt"]; ok {
		t.Fatal("los registros no van")
	}
	for r := range s {
		if strings.Contains(r, "whatsapp") {
			t.Fatalf("la sesión de WhatsApp no se copia: %s", r)
		}
	}
	if a := s["etc/NetworkManager/system-connections/Casa.nmconnection"]; a.modo != 0o600 || a.ark {
		t.Fatalf("el wifi: %+v", a)
	}
	ro := rutas(c.roms)
	if len(ro) != 2 || string(ro["gba/Pokemon Esmeralda.srm"].datos) != "partida esmeralda" {
		t.Fatalf("partidas: %v", ro)
	}
	for r := range ro {
		if strings.HasSuffix(r, ".gba") {
			t.Fatal("los juegos no se copian")
		}
	}
}

func TestDarkOSParaOtraPersona(t *testing.T) {
	consolaFalsa(t)
	c, err := cargaDarkOS([]appTienda{{ID: "youtube", Nombre: "YouTube"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range c.sistema {
		if strings.Contains(a.ruta, "cuentas/") || strings.Contains(a.ruta, "NetworkManager") {
			t.Fatalf("para otra persona no va nada mío: %s", a.ruta)
		}
	}
	if len(c.roms) != 0 {
		t.Fatal("para otra persona no van mis partidas")
	}
}

func TestMenu(t *testing.T) {
	m := &menu{ops: []opcion{{titulo: "a"}, {titulo: "b", apagada: true}, {titulo: "c"}}}
	if m.paso(tAbajo); m.sel != 1 {
		t.Fatal("abajo")
	}
	if r := m.paso(tA); r != -3 {
		t.Fatal("una opción apagada no se elige")
	}
	m.paso(tAbajo)
	if m.paso(tAbajo); m.sel != 0 {
		t.Fatal("da la vuelta")
	}
	if r := m.paso(tA); r != 0 {
		t.Fatalf("elegir: %d", r)
	}
	if r := m.paso(tB); r != volver {
		t.Fatal("B vuelve")
	}
	l := &menu{multiple: true, ops: []opcion{{titulo: "a"}, {titulo: "b", marcada: true}}}
	l.paso(tA)
	if !l.ops[0].marcada {
		t.Fatal("A marca")
	}
	l.paso(tY)
	if len(marcadas(l)) != 0 {
		t.Fatal("Y quita todas")
	}
	l.paso(tX)
	if len(marcadas(l)) != 2 {
		t.Fatal("X marca todas")
	}
	if r := l.paso(tStart); r != aceptar {
		t.Fatal("START sigue")
	}
}
