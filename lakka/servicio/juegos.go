package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Juego: una ROM de la parte JUEGOS de la memoria USB.
type Juego struct {
	Ruta     string // completa
	Relativa string // dentro de la carpeta de juegos (lo que se guarda en la sala)
	Titulo   string
	Sistema  string // gb, gbc, gba, nds, 3ds
}

// Sistemas: qué emulador abre cada uno y cómo se llama en pantalla.
var sistemas = map[string]struct {
	nucleo, nombre string
	orden          int
}{
	"gb":  {"gambatte_libretro.so", "Game Boy", 0},
	"gbc": {"gambatte_libretro.so", "Game Boy Color", 1},
	"gba": {"mgba_libretro.so", "GBA", 2},
	"nds": {"melondsds_libretro.so", "DS", 3},
	"3ds": {"azahar_libretro.so", "3DS", 4},
}

var extensiones = map[string]string{
	".gb": "gb", ".gbc": "gbc", ".sgb": "gb", ".gba": "gba", ".nds": "nds",
	".3ds": "3ds", ".cci": "3ds", ".cxi": "3ds", ".3dsx": "3ds", ".app": "3ds",
}

const etiquetaJuegos = "JUEGOS"

// carpetaJuegos: dónde está montada la parte JUEGOS (exFAT, se ve desde Windows). Si no la hay,
// se intenta crear con el espacio libre de la memoria; si tampoco, se usa /storage/roms.
func carpetaJuegos() string {
	disp := "/dev/disk/by-label/" + etiquetaJuegos
	if _, err := os.Stat(disp); err != nil {
		if !crearParteJuegos() {
			_ = os.MkdirAll("/storage/roms", 0o755)
			return "/storage/roms"
		}
	}
	real, err := filepath.EvalSymlinks(disp)
	if err != nil {
		return "/storage/roms"
	}
	// Lakka (udevil) suele montarla sola en /var/media/JUEGOS; esperamos un poco.
	for i := 0; i < 20; i++ {
		if m := puntoMontaje(real); m != "" {
			return m
		}
		time.Sleep(500 * time.Millisecond)
	}
	destino := "/var/media/" + etiquetaJuegos
	_ = os.MkdirAll(destino, 0o755)
	tipo := "exfat"
	if out, err := exec.Command("blkid", "-o", "value", "-s", "TYPE", real).Output(); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			tipo = t
		}
	}
	if out, err := exec.Command("mount", "-t", tipo, "-o", "rw,noatime", real, destino).CombinedOutput(); err != nil {
		log.Printf("no se pudo montar %s: %v %s", real, err, out)
		return "/storage/roms"
	}
	log.Printf("JUEGOS montada en %s", destino)
	return destino
}

func puntoMontaje(dispositivo string) string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		campos := strings.Fields(s.Text())
		if len(campos) >= 2 && campos[0] == dispositivo {
			return strings.ReplaceAll(campos[1], `\040`, " ")
		}
	}
	return ""
}

// crearParteJuegos: la primera vez, si la memoria tiene espacio libre detrás de nuestras dos
// partes (sistema y datos), crea la tercera en exFAT con la etiqueta JUEGOS.
func crearParteJuegos() bool {
	datos := dispositivoDe("/storage")
	if datos == "" {
		return false
	}
	disco := discoDe(datos)
	if disco == "" || existe(particion(disco, 3)) {
		return false
	}
	libre, err := exec.Command("parted", "-s", "-f", "-m", disco, "unit", "MiB", "print", "free").Output()
	if err != nil {
		return false
	}
	// La última línea "free" dice cuánto queda al final del disco.
	var inicio, fin float64
	for _, l := range strings.Split(string(libre), "\n") {
		c := strings.Split(strings.TrimSuffix(l, ";"), ":")
		if len(c) >= 5 && c[4] == "free" {
			inicio, _ = strconv.ParseFloat(strings.TrimSuffix(c[1], "MiB"), 64)
			fin, _ = strconv.ParseFloat(strings.TrimSuffix(c[2], "MiB"), 64)
		}
	}
	if fin-inicio < 256 {
		return false
	}
	log.Printf("creando la parte JUEGOS en %s (%.0f MiB)", disco, fin-inicio)
	pasos := [][]string{
		{"parted", "-s", "-f", "-a", "optimal", disco, "mkpart", etiquetaJuegos, "fat32", strconv.Itoa(int(inicio)+1) + "MiB", "100%"},
		{"partprobe", disco},
	}
	for _, p := range pasos {
		if out, err := exec.Command(p[0], p[1:]...).CombinedOutput(); err != nil {
			log.Printf("%v: %v %s", p, err, out)
			return false
		}
	}
	nueva := particion(disco, 3)
	for i := 0; i < 20 && !existe(nueva); i++ {
		time.Sleep(250 * time.Millisecond)
	}
	if out, err := exec.Command("mkfs.exfat", "-L", etiquetaJuegos, nueva).CombinedOutput(); err != nil {
		log.Printf("mkfs.exfat: %v %s", err, out)
		return false
	}
	_ = exec.Command("udevadm", "settle").Run()
	return existe("/dev/disk/by-label/" + etiquetaJuegos)
}

func dispositivoDe(montaje string) string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return ""
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		c := strings.Fields(s.Text())
		if len(c) >= 2 && c[1] == montaje && strings.HasPrefix(c[0], "/dev/") {
			return c[0]
		}
	}
	return ""
}

var finNumero = regexp.MustCompile(`p?\d+$`)

func discoDe(part string) string {
	if strings.Contains(part, "mmcblk") || strings.Contains(part, "nvme") {
		return finNumero.ReplaceAllString(part, "")
	}
	return strings.TrimRight(part, "0123456789")
}

func particion(disco string, n int) string {
	if strings.Contains(disco, "mmcblk") || strings.Contains(disco, "nvme") {
		return disco + "p" + strconv.Itoa(n)
	}
	return disco + strconv.Itoa(n)
}

func existe(ruta string) bool {
	_, err := os.Stat(ruta)
	return err == nil
}

var entreParentesis = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)

// tituloDe: "Pokemon - FireRed Version (USA, Europe).gba" -> "Pokemon - FireRed Version".
func tituloDe(nombre string) string {
	base := strings.TrimSuffix(nombre, filepath.Ext(nombre))
	t := entreParentesis.ReplaceAllString(base, "")
	t = strings.ReplaceAll(t, "_", " ")
	t = strings.Join(strings.Fields(t), " ")
	t = strings.Trim(t, " -")
	if t == "" {
		return base
	}
	return t
}

// sistemaPermitido: POKEMULADOR_SISTEMAS limita los sistemas (en consolas que no pueden con DS o
// 3DS, por ejemplo "gb,gbc,gba").
func sistemaPermitido(sis string) bool {
	lista := os.Getenv("POKEMULADOR_SISTEMAS")
	if strings.TrimSpace(lista) == "" {
		return true
	}
	for _, s := range strings.Split(lista, ",") {
		if strings.EqualFold(strings.TrimSpace(s), sis) {
			return true
		}
	}
	return false
}

// nucleoDe: el archivo del núcleo de un sistema (se puede cambiar con POKEMULADOR_NUCLEO_GBA, …).
func nucleoDe(sis string) string {
	if v := os.Getenv("POKEMULADOR_NUCLEO_" + strings.ToUpper(sis)); v != "" {
		return v
	}
	return sistemas[sis].nucleo
}

func buscarJuegos(raiz string) []Juego {
	var lista []Juego
	raiz = filepath.Clean(raiz)
	_ = filepath.WalkDir(raiz, func(ruta string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		nombre := d.Name()
		if d.IsDir() {
			if ruta != raiz && (strings.HasPrefix(nombre, ".") || strings.EqualFold(nombre, "System Volume Information")) {
				return filepath.SkipDir
			}
			if strings.Count(strings.TrimPrefix(ruta, raiz), string(os.PathSeparator)) > 4 {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(nombre, ".") {
			return nil
		}
		sis, ok := extensiones[strings.ToLower(filepath.Ext(nombre))]
		if !ok || !sistemaPermitido(sis) {
			return nil
		}
		rel, _ := filepath.Rel(raiz, ruta)
		lista = append(lista, Juego{Ruta: ruta, Relativa: filepath.ToSlash(rel), Titulo: tituloDe(nombre), Sistema: sis})
		return nil
	})
	sort.Slice(lista, func(i, j int) bool {
		a, b := sistemas[lista[i].Sistema].orden, sistemas[lista[j].Sistema].orden
		if a != b {
			return a < b
		}
		return strings.ToLower(lista[i].Titulo) < strings.ToLower(lista[j].Titulo)
	})
	return lista
}

// Deja un aviso en la parte JUEGOS para quien la abra desde Windows.
func escribirLeeme(raiz string) {
	ruta := filepath.Join(raiz, "LEEME.txt")
	if existe(ruta) {
		return
	}
	texto := "Pokemulador USB\r\n\r\n" +
		"Copia aquí tus juegos (puedes usar carpetas):\r\n" +
		"  Game Boy / Color: .gb .gbc\r\n" +
		"  GBA: .gba\r\n" +
		"  DS: .nds\r\n" +
		"  3DS: .3ds o .cci (descifrados; los .cia no sirven)\r\n\r\n" +
		"Si pones una partida .sav con el mismo nombre que el juego (de Game Boy o GBA),\r\n" +
		"se usa como tu partida de siempre la primera vez que lo abras.\r\n"
	_ = os.WriteFile(ruta, []byte(texto), 0o644)
}
