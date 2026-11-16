package main

import (
	"os"
	"os/exec"
	"syscall"
)

// grupoPropio: RetroArch va en su propio grupo de procesos (para cerrarlo entero, aunque se abra con
// un script que lance otro programa) y recibe SIGTERM si este programa se muere de golpe.
func grupoPropio(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}

// señalGrupo manda la señal a RetroArch y a todo lo que haya lanzado.
func señalGrupo(cmd *exec.Cmd, s syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	if err := syscall.Kill(-cmd.Process.Pid, s); err != nil {
		_ = cmd.Process.Signal(s)
	}
}

// tamTubo cambia lo que cabe en un tubo (F_SETPIPE_SZ).
func tamTubo(f *os.File, n int) {
	_, _, _ = syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), 1031, uintptr(n))
}
