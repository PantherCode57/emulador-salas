//go:build !linux

package main

func programarReinicio()              { logf("reinicio: solo en la consola") }
func reiniciarES(pidTienda int)       {}
func reiniciarServicio(nombre string) {}
