package main

import (
	"os"
	"syscall"
)

// reabrir: la tienda nueva en lugar de esta (el mismo proceso, para EmulationStation), sin volver
// a mirar si hay otra.
func reabrir(yo string) error {
	return syscall.Exec(yo, os.Args, append(os.Environ(), "TIENDA_ACTUALIZAR=no"))
}
