//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const memoria = "/dev/shm/flashear"

// rutaRegistro: el registro en la memoria; en registroGuardado se deja una copia mientras la
// tarjeta se puede escribir, para poder mirarlo después.
var rutaRegistro string

const registroGuardado = "/home/ark/panther/flashear/registro.txt"

func guardarRegistro() {
	if b, err := os.ReadFile(rutaRegistro); err == nil {
		_ = os.WriteFile(registroGuardado, b, 0o644)
	}
}

func tarjetaDeRaiz() (tarjeta, error) {
	m, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return tarjeta{}, err
	}
	log.Printf("montajes al empezar:\n%s", m)
	return tarjetaDePanther(string(m))
}

func mkdev(maj, min uint32) uint64 {
	return uint64(min&0xff) | uint64(maj&0xfff)<<8 | uint64(min&^0xff)<<12 | uint64(maj&^0xfff)<<32
}

func orden(nombre string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, nombre, args...).CombinedOutput()
	log.Printf("%s %s: %v %s", nombre, strings.Join(args, " "), err, bytes.TrimSpace(out))
	return err
}

func pararMenu() {
	_ = orden("systemctl", "stop", "emulationstation")
	if tty, err := os.OpenFile("/dev/tty1", os.O_WRONLY, 0); err == nil {
		_, _ = tty.WriteString("\033[?25l\033[2J")
		tty.Close()
	}
}

func volverAlMenu() {
	_ = orden("systemctl", "start", "emulationstation")
}

// Lo que se ha hecho para proteger la tarjeta, para deshacerlo si al final no se puede.
var (
	congelados []string // puntos de montaje congelados (FIFREEZE)
	paradas    []string // servicios de Panther parados
)

const (
	fifreeze = 0xC0045877 // _IOWR('X', 119, int)
	fithaw   = 0xC0045878 // _IOWR('X', 120, int)
)

func ioctlPunto(punto string, cmd uintptr) error {
	f, err := os.Open(punto)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), cmd, 0); e != 0 {
		return e
	}
	return nil
}

// serviciosPanther: los servicios de Panther OS en marcha (WhatsApp, ahorro…), menos este.
func serviciosPanther() []string {
	out, err := exec.Command("systemctl", "list-units", "--type=service", "--state=active", "--plain", "--no-legend", "panther-*").Output()
	if err != nil {
		return nil
	}
	var r []string
	for _, l := range strings.Split(string(out), "\n") {
		c := strings.Fields(l)
		if len(c) > 0 && strings.HasPrefix(c[0], "panther-") && !strings.HasPrefix(c[0], "panther-flashear") {
			r = append(r, c[0])
		}
	}
	return r
}

func leerMontajes() string {
	m, _ := os.ReadFile("/proc/mounts")
	return string(m)
}

// abiertas: los montajes de la tarjeta que siguen pudiendo escribirse (sin contar los congelados).
func abiertas(disco string) []string {
	var r []string
	m := leerMontajes()
	for punto, ro := range montajesDe(m, disco) {
		if ro {
			continue
		}
		congelado := false
		for _, c := range congelados {
			congelado = congelado || c == punto
		}
		if !congelado {
			r = append(r, punto)
		}
	}
	sort.Strings(r)
	return r
}

// preparar: lo de antes de sacar la tarjeta (con ella aún dentro). Al final, cada montaje de la
// tarjeta de Panther OS tiene que estar en solo lectura o congelado (todo escrito en la tarjeta y
// lo que quiera escribir se queda esperando). Si no se puede, se deshace todo y se vuelve al menú.
func preparar(p tarjeta) (lector, error) {
	l, err := lectorDe(p.host)
	if err != nil {
		return l, fmt.Errorf("no encuentro el lector de la ranura: %v", err)
	}
	if err := orden("swapoff", "-a"); err != nil {
		return l, fmt.Errorf("no se puede quitar la memoria de intercambio: %v", err)
	}
	_ = orden("modprobe", "-a", "vfat", "nls_cp437", "nls_iso8859_1", "nls_utf8")
	fs, _ := os.ReadFile("/proc/filesystems")
	if !bytes.Contains(fs, []byte("vfat")) {
		return l, errors.New("esta consola no sabe abrir tarjetas FAT32 (falta vfat)")
	}
	paradas = serviciosPanther()
	if len(paradas) > 0 {
		_ = orden("systemctl", append([]string{"stop"}, paradas...)...)
	}
	antes := montajesDe(leerMontajes(), p.disco)
	log.Printf("montajes de la tarjeta (true = solo lectura): %v", antes)
	guardarRegistro()

	// 1. Todo guardado y en solo lectura con sysrq «u» (no hace caso de lo que esté abierto).
	syscall.Sync()
	if err := os.WriteFile("/proc/sysrq-trigger", []byte("s"), 0); err != nil {
		log.Printf("sysrq: %v", err)
	} else {
		time.Sleep(time.Second)
		_ = os.WriteFile("/proc/sysrq-trigger", []byte("u"), 0)
		for i := 0; i < 16 && len(abiertas(p.disco)) > 0; i++ {
			time.Sleep(250 * time.Millisecond)
		}
	}
	// 2. Lo que quede, a mano.
	for _, punto := range abiertas(p.disco) {
		_ = orden("mount", "-o", "remount,ro", punto)
	}
	// 3. Y lo que siga abierto, congelado (ext4, btrfs…).
	syscall.Sync()
	for _, punto := range abiertas(p.disco) {
		if err := ioctlPunto(punto, fifreeze); err != nil {
			log.Printf("congelar %s: %v", punto, err)
		} else {
			log.Printf("congelado %s", punto)
			congelados = append(congelados, punto)
		}
	}
	quedan := abiertas(p.disco)
	if len(quedan) == 0 {
		log.Printf("tarjeta protegida (congelados: %v):\n%s", congelados, leerMontajes())
		return l, nil
	}
	log.Printf("no se deja proteger %v:\n%s", quedan, leerMontajes())
	var detalle []string
	for _, punto := range quedan {
		d := punto
		if q := quienEscribe(punto); len(q) > 0 {
			d += ", la tiene abierta " + strings.Join(q, ", ")
		}
		detalle = append(detalle, d)
	}
	deshacer(p.disco, antes)
	return l, fmt.Errorf("La tarjeta de Panther OS no se deja proteger (%s). No has sacado nada: vuelves al menú.", strings.Join(detalle, "; "))
}

// quienEscribe: los programas que tienen archivos de ese montaje abiertos para escribir.
func quienEscribe(punto string) []string {
	visto := map[string]bool{}
	var r []string
	procs, _ := os.ReadDir("/proc")
	yo := strconv.Itoa(os.Getpid())
	for _, pr := range procs {
		pid := pr.Name()
		if pid == yo || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		fds, _ := os.ReadDir("/proc/" + pid + "/fd")
		for _, fd := range fds {
			destino, err := os.Readlink("/proc/" + pid + "/fd/" + fd.Name())
			if err != nil || !strings.HasPrefix(destino, "/") || !dentroDe(destino, punto) {
				continue
			}
			info, _ := os.ReadFile("/proc/" + pid + "/fdinfo/" + fd.Name())
			if !paraEscribir(string(info)) {
				continue
			}
			nombre := strings.TrimSpace(leerSys("/proc/" + pid + "/comm"))
			log.Printf("%s (%s) escribe en %s", nombre, pid, destino)
			if nombre != "" && !visto[nombre] && len(r) < 4 {
				visto[nombre] = true
				r = append(r, "«"+nombre+"»")
			}
		}
	}
	return r
}

// dentroDe: la ruta está en ese montaje. Para «/» no cuentan los que son de la memoria.
func dentroDe(ruta, punto string) bool {
	if punto == "/" {
		for _, fuera := range []string{"/dev/", "/proc/", "/sys/", "/run/", "/tmp/"} {
			if strings.HasPrefix(ruta, fuera) {
				return false
			}
		}
		return true
	}
	return ruta == punto || strings.HasPrefix(ruta, strings.TrimSuffix(punto, "/")+"/")
}

// paraEscribir: en /proc/<pid>/fdinfo/<n>, «flags:» en octal; O_WRONLY es 1 y O_RDWR 2.
func paraEscribir(fdinfo string) bool {
	for _, l := range strings.Split(fdinfo, "\n") {
		if v, ok := strings.CutPrefix(l, "flags:"); ok {
			f, err := strconv.ParseInt(strings.TrimSpace(v), 8, 64)
			return err == nil && f&3 != 0
		}
	}
	return false
}

// deshacer: como estaba antes de preparar (para volver al menú sin reiniciar).
func deshacer(disco string, antes map[string]bool) {
	for i := len(congelados) - 1; i >= 0; i-- {
		if err := ioctlPunto(congelados[i], fithaw); err != nil {
			log.Printf("descongelar %s: %v", congelados[i], err)
		}
	}
	congelados = nil
	for punto, ro := range montajesDe(leerMontajes(), disco) {
		if ro && !antes[punto] {
			_ = orden("mount", "-o", "remount,rw", punto)
		}
	}
	if len(paradas) > 0 {
		_ = orden("systemctl", append([]string{"start"}, paradas...)...)
	}
	guardarRegistro()
}

// --- Escribir en la otra tarjeta ---

// particionMontada: una partición de la otra tarjeta, montada en la memoria.
type particionMontada struct {
	nombre, tipo, punto string
}

// montarParticiones: todas las particiones de la tarjeta, montadas (con lectura y escritura) en
// /dev/shm/flashear/p1, p2… con el tipo que se deje (ext4, vfat o exfat).
func montarParticiones(t tarjeta) ([]particionMontada, func(), error) {
	d := filepath.Join(raizSys, "block", t.disco)
	m, _ := filepath.Glob(filepath.Join(d, t.disco+"p*"))
	sort.Strings(m)
	if len(m) == 0 {
		m = []string{d} // sin tabla de particiones: la tarjeta entera
	}
	var ps []particionMontada
	soltar := func() {
		syscall.Sync()
		for i := len(ps) - 1; i >= 0; i-- {
			if err := syscall.Unmount(ps[i].punto, 0); err != nil {
				log.Printf("umount %s: %v", ps[i].punto, err)
				_ = syscall.Unmount(ps[i].punto, syscall.MNT_DETACH)
			}
		}
	}
	for i, x := range m {
		nombre := filepath.Base(x)
		dev := leerSys(filepath.Join(x, "dev"))
		var maj, min uint32
		if _, err := fmt.Sscanf(dev, "%d:%d", &maj, &min); err != nil {
			continue
		}
		nodo := "/dev/" + nombre
		if st, err := os.Stat(nodo); err != nil || st.Mode()&os.ModeDevice == 0 {
			nodo = fmt.Sprintf("/dev/flashear-%d", i)
			_ = os.Remove(nodo)
			if err := syscall.Mknod(nodo, syscall.S_IFBLK|0o600, int(mkdev(maj, min))); err != nil {
				continue
			}
		}
		punto := fmt.Sprintf("%s/p%d", memoria, i+1)
		_ = os.MkdirAll(punto, 0o755)
		for _, tipo := range []struct{ fs, op string }{{"ext4", ""}, {"vfat", "utf8,shortname=mixed"}, {"vfat", ""}, {"exfat", ""}} {
			if err := syscall.Mount(nodo, punto, tipo.fs, syscall.MS_NOATIME, tipo.op); err == nil {
				ps = append(ps, particionMontada{nombre: nombre, tipo: tipo.fs, punto: punto})
				log.Printf("montada %s (%s)", nombre, tipo.fs)
				break
			}
		}
	}
	if len(ps) == 0 {
		return nil, soltar, errors.New("no se puede abrir ninguna partición de esa tarjeta (¿es de esa consola?)")
	}
	return ps, soltar, nil
}

// escribir: los archivos en una partición montada.
func escribir(raiz string, as []archivo) error {
	for _, a := range as {
		ruta := filepath.Join(raiz, filepath.FromSlash(a.ruta))
		if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
			return fmt.Errorf("%s: %v", a.ruta, err)
		}
		if a.enlace != "" {
			_ = os.Remove(ruta)
			if err := os.Symlink(a.enlace, ruta); err != nil {
				return fmt.Errorf("%s: %v", a.ruta, err)
			}
			continue
		}
		f, err := os.OpenFile(ruta, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, a.modo)
		if err != nil {
			return fmt.Errorf("%s: %v", a.ruta, err)
		}
		_, errw := f.Write(a.datos)
		errs := f.Sync()
		errc := f.Close()
		if err := errors.Join(errw, errs, errc); err != nil {
			return fmt.Errorf("%s: %v", a.ruta, err)
		}
		_ = os.Chmod(ruta, a.modo)
		if a.ark {
			_ = os.Chown(ruta, 1000, 1000)
			for dir := filepath.Dir(ruta); strings.HasPrefix(dir, filepath.Join(raiz, "home", "ark")+"/"); dir = filepath.Dir(dir) {
				_ = os.Chown(dir, 1000, 1000)
			}
		}
	}
	return nil
}

// rutasDe: los archivos y carpetas de una partición (3 niveles), para ver qué flashcard es.
func rutasDe(raiz string) []string {
	var r []string
	_ = filepath.WalkDir(raiz, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == raiz {
			return nil
		}
		rel, _ := filepath.Rel(raiz, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() && strings.Count(rel, "/") >= 2 {
			return filepath.SkipDir
		}
		r = append(r, rel)
		return nil
	})
	return r
}

// escribirCarga: lo elegido, en la otra tarjeta. No toca nada más de ella.
func escribirCarga(t tarjeta, c carga) error {
	ps, soltar, err := montarParticiones(t)
	defer soltar()
	if err != nil {
		return err
	}
	switch c.destino {
	case "ds", "gba":
		var fat *particionMontada
		for i := range ps {
			if ps[i].tipo == "vfat" {
				fat = &ps[i]
				break
			}
		}
		if fat == nil {
			return errors.New("esa tarjeta no es FAT32, como la piden las flashcards")
		}
		if err := escribir(fat.punto, c.raiz); err != nil {
			return fmt.Errorf("no se ha podido escribir: %v", err)
		}
		if len(c.partidas) > 0 {
			hay := rutasDe(fat.punto)
			for _, p := range c.partidas {
				juego := strings.TrimSuffix(p.ruta, ".sav")
				sitio := sitioPartida(hay, juego)
				ruta := filepath.Join(fat.punto, filepath.FromSlash(sitio))
				if _, err := os.Stat(ruta); err == nil { // la que hubiera, guardada
					_ = os.Rename(ruta, ruta+".antes")
				}
				p.ruta = sitio
				if err := escribir(fat.punto, []archivo{p}); err != nil {
					return fmt.Errorf("no se ha podido escribir la partida de %s: %v", juego, err)
				}
				log.Printf("partida de %s en %s", juego, sitio)
			}
		}
		for _, a := range c.raiz {
			got, err := os.ReadFile(filepath.Join(fat.punto, filepath.FromSlash(a.ruta)))
			if err != nil || !bytes.Equal(got, a.datos) {
				return fmt.Errorf("%s no ha quedado bien en la tarjeta", a.ruta)
			}
		}
	case "darkos":
		var sistema, roms *particionMontada
		for i := range ps {
			p := &ps[i]
			switch {
			case p.tipo == "ext4" && existeRuta(filepath.Join(p.punto, "etc", "os-release")):
				sistema = p
			case p.tipo != "ext4" && esDeJuegos(p.punto):
				roms = p
			}
		}
		if sistema == nil {
			return errors.New("esa tarjeta no tiene dArkOS (no hay una partición del sistema): primero hay que ponérselo con un PC")
		}
		if !existeRuta(filepath.Join(sistema.punto, "home", "ark")) {
			return errors.New("esa tarjeta tiene un sistema, pero no es dArkOS (no está el usuario ark)")
		}
		if err := escribir(sistema.punto, c.sistema); err != nil {
			return fmt.Errorf("no se ha podido escribir en el sistema: %v", err)
		}
		if len(c.roms) > 0 {
			if roms == nil {
				log.Printf("no encuentro la partición de los juegos: las partidas no van")
			} else if err := escribir(roms.punto, c.roms); err != nil {
				return fmt.Errorf("no se han podido escribir las partidas: %v", err)
			}
		}
	default:
		return fmt.Errorf("no sé escribir para %q", c.destino)
	}
	return nil
}

func existeRuta(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// esDeJuegos: la partición de los juegos de dArkOS (con carpetas como gba, nes, psx o ports).
func esDeJuegos(punto string) bool {
	n := 0
	for _, s := range []string{"gba", "gb", "gbc", "nes", "snes", "psx", "ports", "n64", "nds", "psp"} {
		if existeRuta(filepath.Join(punto, s)) {
			n++
		}
	}
	return n >= 2
}

func reiniciar() {
	syscall.Sync()
	if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
		log.Printf("reboot: %v", err)
		pintar(vista{estado: eNoSePuede, mensaje: "No se reinicia sola: apaga la consola con el botón y vuelve a encenderla."})
		select {}
	}
}
