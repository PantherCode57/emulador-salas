//go:build linux

package main

import "testing"

func TestQuienEscribe(t *testing.T) {
	casos := []struct {
		ruta, punto string
		dentro      bool
	}{
		{"/roms/gba/juego.srm", "/roms", true},
		{"/roms", "/roms", true},
		{"/romsviejo/x", "/roms", false},
		{"/home/ark/x", "/", true},
		{"/dev/shm/pasards/registro.txt", "/", false},
		{"/run/user/1000/x", "/", false},
		{"/boot/Image", "/boot/", true},
	}
	for _, c := range casos {
		if got := dentroDe(c.ruta, c.punto); got != c.dentro {
			t.Errorf("dentroDe(%q, %q) = %v", c.ruta, c.punto, got)
		}
	}
	if !paraEscribir("pos:\t0\nflags:\t0100002\nmnt_id:\t25\n") {
		t.Error("O_RDWR es para escribir")
	}
	if !paraEscribir("pos:\t0\nflags:\t02101\n") {
		t.Error("O_WRONLY|O_APPEND es para escribir")
	}
	if paraEscribir("pos:\t0\nflags:\t0100000\n") {
		t.Error("O_RDONLY no es para escribir")
	}
}
