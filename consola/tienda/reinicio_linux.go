package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Para que la pantalla principal enseñe o quite una tarjeta hay que reiniciar EmulationStation.
// No se puede hacer desde la tienda mientras está abierta: la tienda es hija de EmulationStation y
// systemd la cerraría con él. Al salir, la tienda deja en marcha (con systemd-run, fuera de
// EmulationStation) «tienda -reiniciar-es <pid>», que espera a que la tienda acabe y a que no haya
// ninguna app ni juego abierto (que EmulationStation no tenga hijos), y entonces lo reinicia.

const unidadReinicio = "panther-tienda-reinicio"

func programarReinicio() {
	yo, err := os.Executable()
	if err != nil {
		logf("reinicio: %v", err)
		return
	}
	pid := itoa(os.Getpid())
	args := []string{"-n", "systemd-run", "--quiet", "--collect", "--unit=" + unidadReinicio,
		"--uid=" + itoa(os.Getuid()), "--gid=" + itoa(os.Getgid()),
		"--setenv=HOME=" + casa, "--setenv=TIENDA_DATOS=" + carpetaDatos,
		yo, "-reiniciar-es", pid}
	if out, err := exec.Command("sudo", args...).CombinedOutput(); err == nil {
		logf("reinicio de EmulationStation programado")
		return
	} else {
		logf("systemd-run: %v %s", err, strings.TrimSpace(string(out)))
		if strings.Contains(string(out), "already") { // ya hay uno esperando
			return
		}
	}
	// Sin systemd-run: aparte, en su propia sesión. Al reiniciar, systemd lo cerrará también, pero
	// para entonces ya ha pedido el reinicio.
	cmd := exec.Command(yo, "-reiniciar-es", pid)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		logf("reinicio: %v", err)
		return
	}
	_ = cmd.Process.Release()
}

func vivo(pid int) bool {
	_, err := os.Stat("/proc/" + itoa(pid))
	return err == nil
}

// procesos: pid → (padre, programa).
func procesos() map[int][2]string {
	out := map[int][2]string{}
	m, _ := filepath.Glob("/proc/[0-9]*/stat")
	for _, r := range m {
		b, err := os.ReadFile(r)
		if err != nil {
			continue
		}
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		campos := strings.Fields(s[i+1:])
		if len(campos) < 2 {
			continue
		}
		pid, _ := strconv.Atoi(filepath.Base(filepath.Dir(r)))
		cmd, _ := os.ReadFile(filepath.Join(filepath.Dir(r), "cmdline"))
		prog := strings.SplitN(string(cmd), "\x00", 2)[0]
		out[pid] = [2]string{campos[1], prog}
	}
	return out
}

// esOcupado: el pid de EmulationStation (0 si no está) y si tiene algo abierto.
func esOcupado() (int, bool) {
	ps := procesos()
	es := 0
	for pid, p := range ps {
		if filepath.Base(p[1]) == "emulationstation" {
			es = pid
			break
		}
	}
	if es == 0 {
		return 0, false
	}
	padre := itoa(es)
	for _, p := range ps {
		if p[0] == padre {
			return es, true
		}
	}
	return es, false
}

func reiniciarES(pidTienda int) {
	logf("reinicio: espero a que se cierre la tienda (%d)", pidTienda)
	for i := 0; i < 600 && vivo(pidTienda); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	// Que EmulationStation esté en su menú (sin hijos) dos veces seguidas. Si abres otra cosa, se
	// espera a que la cierres (como mucho 12 horas).
	libre := 0
	for fin := time.Now().Add(12 * time.Hour); time.Now().Before(fin); {
		es, ocupado := esOcupado()
		switch {
		case es == 0:
			libre = 0
		case ocupado:
			libre = 0
		default:
			libre++
		}
		if libre >= 2 {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if libre < 2 {
		logf("reinicio: EmulationStation no ha quedado libre; no lo reinicio")
		return
	}
	if out, err := exec.Command("systemctl", "is-active", "emulationstation").Output(); err != nil || strings.TrimSpace(string(out)) != "active" {
		logf("reinicio: EmulationStation no está en marcha como servicio; no lo reinicio")
		return
	}
	logf("reinicio: reinicio EmulationStation para poner las tarjetas al día")
	if out, err := exec.Command("sudo", "-n", "systemctl", "restart", "emulationstation").CombinedOutput(); err != nil {
		logf("reinicio: %v %s", err, strings.TrimSpace(string(out)))
	}
}

// reiniciarServicio: si el servicio está en marcha, otra vez (con el programa nuevo).
func reiniciarServicio(nombre string) {
	if out, err := exec.Command("systemctl", "is-active", nombre).Output(); err != nil || strings.TrimSpace(string(out)) != "active" {
		return
	}
	if out, err := exec.Command("sudo", "-n", "systemctl", "restart", nombre).CombinedOutput(); err != nil {
		logf("%s: %v %s", nombre, err, strings.TrimSpace(string(out)))
	}
}
