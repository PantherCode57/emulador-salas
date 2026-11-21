//go:build linux

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const memoria = "/dev/shm/pasards"

// errSoloLectura: algo ya está en solo lectura y el menú no puede volver: hay que reiniciar.
var errSoloLectura = errors.New("no se ha podido dejar la tarjeta en solo lectura; la consola se reinicia")

func tarjetaDeRaiz() (tarjeta, error) {
	var st syscall.Stat_t
	if err := syscall.Stat("/", &st); err != nil {
		return tarjeta{}, err
	}
	d := uint64(st.Dev)
	maj := uint32((d>>8)&0xfff) | uint32((d>>32)&^0xfff)
	min := uint32(d&0xff) | uint32((d>>12)&^0xff)
	return tarjetaDeDispositivo(maj, min)
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

// preparar: lo de antes de sacar la tarjeta (con ella aún dentro).
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

	// Todo guardado y en solo lectura.
	syscall.Sync()
	_ = os.WriteFile("/proc/sysrq-trigger", []byte("s"), 0)
	time.Sleep(time.Second)
	_ = os.WriteFile("/proc/sysrq-trigger", []byte("u"), 0)
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		m, _ := os.ReadFile("/proc/mounts")
		if todoSoloLectura(string(m), p.disco) {
			log.Printf("en solo lectura:\n%s", m)
			return l, nil
		}
	}
	// Si sysrq no está, a mano.
	m, _ := os.ReadFile("/proc/mounts")
	for punto, ro := range montajesDe(string(m), p.disco) {
		if !ro {
			_ = orden("mount", "-o", "remount,ro", punto)
		}
	}
	m, _ = os.ReadFile("/proc/mounts")
	if todoSoloLectura(string(m), p.disco) {
		return l, nil
	}
	log.Printf("no queda en solo lectura:\n%s", m)
	for _, ro := range montajesDe(string(m), p.disco) {
		if ro {
			return l, errSoloLectura
		}
	}
	return l, errors.New("la tarjeta de Panther OS no se deja poner en solo lectura")
}

// copiarA: panther.nds a la raíz de la tarjeta de la flashcard. No toca nada más.
func copiarA(t tarjeta, datos []byte) error {
	nombre, dev, err := particion(t.disco)
	if err != nil {
		return err
	}
	var maj, min uint32
	if _, err := fmt.Sscanf(dev, "%d:%d", &maj, &min); err != nil {
		return fmt.Errorf("%s: %v", nombre, err)
	}
	_ = os.MkdirAll(memoria, 0o755)
	// El disco lo pone el núcleo en /dev (devtmpfs, en la memoria). Si no está, se hace en /dev, que
	// sí deja abrir discos (/dev/shm no: es «nodev»).
	nodo := "/dev/" + nombre
	if st, err := os.Stat(nodo); err != nil || st.Mode()&os.ModeDevice == 0 {
		nodo = "/dev/pasards-flashcard"
		_ = os.Remove(nodo)
		if err := syscall.Mknod(nodo, syscall.S_IFBLK|0o600, int(mkdev(maj, min))); err != nil {
			return fmt.Errorf("no se puede abrir la tarjeta: %v", err)
		}
	}
	log.Printf("tarjeta de la flashcard: %s (%s)", nodo, dev)
	punto := memoria + "/flashcard"
	_ = os.MkdirAll(punto, 0o755)
	var errm error
	for _, opciones := range []string{"utf8,shortname=mixed", "utf8", ""} {
		if errm = syscall.Mount(nodo, punto, "vfat", syscall.MS_NOATIME, opciones); errm == nil {
			break
		}
	}
	if errm != nil {
		log.Printf("mount %s: %v", nombre, errm)
		return errors.New("no se puede abrir esa tarjeta: tiene que ser FAT32, como la piden las flashcards")
	}
	defer func() {
		syscall.Sync()
		if err := syscall.Unmount(punto, 0); err != nil {
			log.Printf("umount: %v", err)
			_ = syscall.Unmount(punto, syscall.MNT_DETACH)
		}
	}()
	if ls, err := os.ReadDir(punto); err == nil {
		var n []string
		for _, e := range ls {
			n = append(n, e.Name())
		}
		log.Printf("en la tarjeta (%s): %s", nombre, strings.Join(n, ", "))
	}
	destino := punto + "/panther.nds"
	f, err := os.OpenFile(destino, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("no se puede escribir en la tarjeta: %v", err)
	}
	_, errw := f.Write(datos)
	errs := f.Sync()
	errc := f.Close()
	if err := errors.Join(errw, errs, errc); err != nil {
		return fmt.Errorf("no se ha podido escribir panther.nds: %v", err)
	}
	if got, err := os.ReadFile(destino); err != nil || !bytes.Equal(got, datos) {
		return errors.New("panther.nds no ha quedado bien en la tarjeta")
	}
	return nil
}

func reiniciar() {
	syscall.Sync()
	if err := syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART); err != nil {
		log.Printf("reboot: %v", err)
		pintar(vista{estado: eNoSePuede, mensaje: "No se reinicia sola: apaga la consola con el botón y vuelve a encenderla."})
		select {}
	}
}
