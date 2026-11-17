//go:build !linux

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func grupoPropio(cmd *exec.Cmd) {}

func señalGrupo(cmd *exec.Cmd, s syscall.Signal) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func tamTubo(f *os.File, n int) {}
