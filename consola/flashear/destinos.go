package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Las consolas a las que se les puede pasar algo, y lo que se les pasa. Todo se prepara en la
// memoria antes de sacar la tarjeta de Panther OS (después ya no se puede leer nada de ella).
// Nunca se pasan juegos comerciales (las ROMs): solo Panther OS, sus apps, los juegos libres de la
// Panther Store y, si es para el mismo dueño, sus cuentas y partidas.

type destino struct {
	id, nombre, detalle string
}

var destinos = []destino{
	{"ds", "Nintendo DS · DS Lite · DSi", "Con flashcard (R4, EZ-Flash, Acekard…): Panther OS para la DS."},
	{"gba", "Game Boy Advance", "Con flashcard (EZ-Flash Omega, EverDrive…): tus partidas de GBA."},
	{"darkos", "Consola china con dArkOS", "Como la tuya (R36S, RG351, XF35H…): las apps de Panther OS."},
}

// Dónde está cada cosa en esta consola (en las pruebas, en carpetas de mentira).
var (
	dirRoms   = "/roms"
	dirConfig = "/home/ark/.config/panther"
	dirRedes  = "/etc/NetworkManager/system-connections"
	binTienda = "/home/ark/panther/tienda/tienda"
)

const maxCarga = 300 << 20 // lo que cabe en la memoria con holgura

// archivo: algo que se escribe en la tarjeta nueva.
type archivo struct {
	ruta   string // dentro de su partición, sin «/» delante
	datos  []byte
	modo   os.FileMode
	ark    bool   // del usuario ark (1000:1000) en la consola nueva
	enlace string // si es un enlace simbólico: adónde apunta
}

// carga: todo lo que se escribe, por partición.
type carga struct {
	destino  string
	raiz     []archivo // DS y GBA: la partición FAT de la flashcard
	sistema  []archivo // dArkOS: la partición del sistema (ext4)
	roms     []archivo // dArkOS: la partición de los juegos
	partidas []archivo // GBA: partidas (ruta = nombre del juego + .sav; el sitio se decide al escribir)
	resumen  []string
}

func (c *carga) tam() int {
	n := 0
	for _, l := range [][]archivo{c.raiz, c.sistema, c.roms, c.partidas} {
		for _, a := range l {
			n += len(a.datos)
		}
	}
	return n
}

// --- Nintendo DS ---

func cargaDS(nds []byte) (carga, error) {
	if len(nds) < 512 {
		return carga{}, fmt.Errorf("falta panther.nds: quita y vuelve a poner la app en la Panther Store")
	}
	return carga{destino: "ds",
		raiz:    []archivo{{ruta: "panther.nds", datos: nds, modo: 0o644}},
		resumen: []string{fmt.Sprintf("Panther OS para la DS (panther.nds, %d KB)", len(nds)/1024)}}, nil
}

// --- Partidas ---

type partida struct {
	juego string // el nombre del juego (como el de su archivo, sin la extensión)
	ruta  string // dónde está en esta consola
	tam   int64
}

// buscarPartidas: las partidas (.srm y .sav) que hay en /roms (de un sistema o de todos).
func buscarPartidas(sistema string) []partida {
	base := dirRoms
	if sistema != "" {
		base = filepath.Join(dirRoms, sistema)
	}
	var r []partida
	_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(base, p)
			if strings.Count(filepath.ToSlash(rel), "/") >= 2 || strings.HasPrefix(d.Name(), ".") && p != base {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".srm" && ext != ".sav" {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() == 0 || info.Size() > 4<<20 {
			return nil
		}
		r = append(r, partida{juego: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())), ruta: p, tam: info.Size()})
		return nil
	})
	sort.Slice(r, func(i, j int) bool { return strings.ToLower(r[i].juego) < strings.ToLower(r[j].juego) })
	return r
}

// --- Game Boy Advance ---

func cargaGBA(elegidas []partida) (carga, error) {
	c := carga{destino: "gba"}
	for _, p := range elegidas {
		b, err := os.ReadFile(p.ruta)
		if err != nil {
			return c, fmt.Errorf("no se puede leer la partida de %s: %v", p.juego, err)
		}
		c.partidas = append(c.partidas, archivo{ruta: p.juego + ".sav", datos: b, modo: 0o644})
	}
	if len(c.partidas) == 0 {
		return c, fmt.Errorf("no has elegido ninguna partida")
	}
	c.resumen = []string{fmt.Sprintf("%d partidas de GBA (las que ya hubiera en la tarjeta se guardan como .sav.antes)", len(c.partidas))}
	return c, nil
}

// sitioPartida: dónde va la partida de un juego en la tarjeta de una flashcard de GBA, según lo que
// ya tiene. «archivos» son las rutas de la tarjeta (con «/»).
//   - EZ-Flash Omega/Definitive: SAVER/<juego>.sav
//   - EverDrive GBA: GBASYS/save/<juego>.sav
//   - otras: al lado del juego, si está; y si no, en «PARTIDAS GBA».
func sitioPartida(archivos []string, juego string) string {
	hay := map[string]bool{}
	for _, a := range archivos {
		hay[strings.ToLower(a)] = true
	}
	switch {
	case hay["saver"]:
		return "SAVER/" + juego + ".sav"
	case hay["gbasys/save"]:
		return "GBASYS/save/" + juego + ".sav"
	}
	quiero := strings.ToLower(juego + ".gba")
	for _, a := range archivos {
		if strings.ToLower(path.Base(a)) == quiero {
			return path.Join(path.Dir(a), juego+".sav")
		}
	}
	return "PARTIDAS GBA/" + juego + ".sav"
}

// --- Consola china con dArkOS ---

// appTienda: lo que hay en la Panther Store (tienda -lista-json).
type appTienda struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"` // «app» o «juego»
	Puesto bool   `json:"puesto"`
}

func listaTienda() ([]appTienda, error) {
	out, err := exec.Command(binTienda, "-lista-json").Output()
	if err != nil {
		return nil, fmt.Errorf("la Panther Store no contesta (¿está al día?): %v", err)
	}
	var l []appTienda
	if err := json.Unmarshal(out, &l); err != nil {
		return nil, fmt.Errorf("la Panther Store contesta raro: %v", err)
	}
	var r []appTienda
	for _, a := range l {
		if a.ID != "flashear" && a.ID != "pasards" {
			r = append(r, a)
		}
	}
	return r, nil
}

// appsPara: las apps que se pueden pasar. Claude habla con el PC de casa del dueño: a la consola de
// otra persona no va.
func appsPara(apps []appTienda, paraMi bool) []appTienda {
	var r []appTienda
	for _, a := range apps {
		if !paraMi && a.ID == "claude" {
			continue
		}
		r = append(r, a)
	}
	return r
}

// Cuentas que no se copian: la sesión de WhatsApp es un dispositivo vinculado; con dos consolas a la
// vez se echan una a otra. En la nueva se vincula otra vez.
var sinCopiar = map[string]bool{"whatsapp": true}

func cargaDarkOS(apps []appTienda, paraMi bool) (carga, error) {
	c := carga{destino: "darkos"}
	tienda, err := os.ReadFile(binTienda)
	if err != nil {
		return c, fmt.Errorf("no está la Panther Store en esta consola: %v", err)
	}
	var ids, nombres []string
	for _, a := range apps {
		ids = append(ids, a.ID)
		nombres = append(nombres, a.Nombre)
	}
	f := "home/ark/panther/flasheo/"
	c.sistema = append(c.sistema,
		archivo{ruta: f + "tienda", datos: tienda, modo: 0o755, ark: true},
		archivo{ruta: f + "instalar.sh", datos: []byte(instalador), modo: 0o755, ark: true},
		archivo{ruta: f + "apps.txt", datos: []byte(strings.Join(ids, "\n") + "\n"), modo: 0o644, ark: true},
		archivo{ruta: "etc/systemd/system/panther-flasheo.service", datos: []byte(servicioFlasheo), modo: 0o644},
		archivo{ruta: "etc/systemd/system/multi-user.target.wants/panther-flasheo.service", enlace: "/etc/systemd/system/panther-flasheo.service"},
	)
	c.resumen = append(c.resumen, "La Panther Store y "+cuantas(len(apps), "app o juego", "apps y juegos")+
		" (se ponen solas cuando esa consola tenga internet): "+strings.Join(nombres, ", "))
	if paraMi {
		cuentas := 0
		ls, _ := os.ReadDir(dirConfig)
		for _, d := range ls {
			if !d.IsDir() || sinCopiar[d.Name()] {
				continue
			}
			base := filepath.Join(dirConfig, d.Name())
			_ = filepath.WalkDir(base, func(p string, e fs.DirEntry, err error) error {
				if err != nil || e.IsDir() {
					return nil
				}
				if strings.HasPrefix(e.Name(), "registro") || strings.HasSuffix(e.Name(), ".tmp") {
					return nil // los registros no hacen falta
				}
				info, err := e.Info()
				if err != nil || info.Size() > 20<<20 {
					return nil
				}
				b, err := os.ReadFile(p)
				if err != nil {
					return nil
				}
				rel, _ := filepath.Rel(dirConfig, p)
				c.sistema = append(c.sistema, archivo{ruta: f + "cuentas/" + filepath.ToSlash(rel), datos: b, modo: 0o600, ark: true})
				return nil
			})
			cuentas++
		}
		if cuentas > 0 {
			c.resumen = append(c.resumen, fmt.Sprintf("Tus cuentas de %d apps (WhatsApp no: se vincula otra vez)", cuentas))
		}
		redes, _ := filepath.Glob(filepath.Join(dirRedes, "*.nmconnection"))
		for _, r := range redes {
			if b, err := os.ReadFile(r); err == nil {
				c.sistema = append(c.sistema, archivo{ruta: "etc/NetworkManager/system-connections/" + filepath.Base(r), datos: b, modo: 0o600})
			}
		}
		if len(redes) > 0 {
			c.resumen = append(c.resumen, cuantas(len(redes), "wifi guardado", "wifis guardados"))
		}
		ps := buscarPartidas("")
		for _, p := range ps {
			b, err := os.ReadFile(p.ruta)
			if err != nil {
				continue
			}
			rel, _ := filepath.Rel(dirRoms, p.ruta)
			c.roms = append(c.roms, archivo{ruta: filepath.ToSlash(rel), datos: b, modo: 0o644})
		}
		if len(ps) > 0 {
			c.resumen = append(c.resumen, cuantas(len(ps), "partida", "partidas")+" (sin los juegos: esos los pones tú)")
		}
	}
	if c.tam() > maxCarga {
		return c, fmt.Errorf("es demasiado para la memoria (%d MB): elige menos", c.tam()>>20)
	}
	return c, nil
}

func cuantas(n int, uno, varios string) string {
	if n == 1 {
		return "1 " + uno
	}
	return fmt.Sprintf("%d %s", n, varios)
}

// El servicio que deja en la consola nueva: arranca instalar.sh al encenderla y se quita al acabar.
const servicioFlasheo = `[Unit]
Description=Panther OS: pone lo que dejó «Flashear tarjeta» (se quita al acabar)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ark
Environment=HOME=/home/ark
ExecStart=/bin/bash /home/ark/panther/flasheo/instalar.sh

[Install]
WantedBy=multi-user.target
`

// instalador: lo que hace la consola nueva al encenderse. Espera a tener internet (cuando se conecte
// el wifi desde su menú), pone la pantalla de las apps (el emulador de Panther y su núcleo), la
// Panther Store en el menú y las apps elegidas, y copia las cuentas si las hay.
const instalador = `#!/bin/bash
# Lo dejó «Flashear tarjeta» de Panther OS. Al encender esta consola, en cuanto tiene internet, pone
# con la Panther Store las apps y los juegos elegidos y, si era para la misma persona, sus cuentas.
# Luego se quita. Lo que va pasando queda en registro.txt, aquí al lado.
D=/home/ark/panther/flasheo
P=/home/ark/panther
FUENTE=https://github.com/PantherCode57/emulador-salas/releases/download/panther-apps
exec >> "$D/registro.txt" 2>&1
echo "$(date) Flashear tarjeta: empiezo"
until curl -fsI --max-time 15 https://github.com > /dev/null 2>&1; do sleep 20; done
echo "$(date) hay internet"
mkdir -p "$P/tienda" "$P/emulador"
install -m 755 "$D/tienda" "$P/tienda/tienda"
curl -fsSL --retry 3 -o "$P/emulador/emulador" "$FUENTE/emulador" && chmod 755 "$P/emulador/emulador"
curl -fsSL --retry 3 -o "$P/tienda/pokemulador_libretro.so" "$FUENTE/pokemulador_libretro.so"
"$P/tienda/tienda" -alta-tienda || echo "no se ha podido poner la Panther Store en el menú"
while read -r id; do
	[ -n "$id" ] || continue
	echo "$(date) pongo $id"
	"$P/tienda/tienda" -poner "$id" || echo "no se ha podido poner $id"
done < "$D/apps.txt"
if [ -d "$D/cuentas" ]; then
	mkdir -p /home/ark/.config/panther
	cp -a "$D/cuentas/." /home/ark/.config/panther/
	rm -rf "$D/cuentas"
	echo "cuentas copiadas"
fi
sudo -n systemctl disable panther-flasheo.service
echo "$(date) hecho"
sudo -n systemctl restart emulationstation
`
