package main

import (
	"os/exec"
	"syscall"
)

// grupoPropio: RetroArch (y el enlace) van en su propio grupo de procesos (para cerrarlos enteros,
// aunque lancen otro programa) y reciben SIGTERM si este programa se muere de golpe.
func grupoPropio(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}

// señalGrupo manda la señal al proceso y a todo lo que haya lanzado.
func señalGrupo(cmd *exec.Cmd, s syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, s); err != nil {
		_ = cmd.Process.Signal(s)
	}
}

// sincronizar pasa a la tarjeta lo que esté a medio escribir (por si se apaga la consola).
func sincronizar() { syscall.Sync() }
