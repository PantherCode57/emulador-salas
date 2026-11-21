//go:build linux

package main

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
)

// escucharBotones: cualquier botón de la consola (los de /dev/input, sin EmulationStation en
// medio). No cuentan el de encender ni los del volumen.
func escucharBotones() <-chan struct{} {
	c := make(chan struct{}, 4)
	m, _ := filepath.Glob("/dev/input/event*")
	for _, x := range m {
		f, err := os.Open(x)
		if err != nil {
			continue
		}
		go func(f *os.File) {
			defer f.Close()
			b := make([]byte, 24) // struct input_event en 64 bits
			for {
				if _, err := io.ReadFull(f, b); err != nil {
					return
				}
				tipo := binary.LittleEndian.Uint16(b[16:])
				codigo := binary.LittleEndian.Uint16(b[18:])
				valor := int32(binary.LittleEndian.Uint32(b[20:]))
				if tipo != 1 || valor != 1 { // EV_KEY, al pulsar
					continue
				}
				if codigo == 116 || codigo == 114 || codigo == 115 { // KEY_POWER, KEY_VOLUMEDOWN, KEY_VOLUMEUP
					continue
				}
				select {
				case c <- struct{}{}:
				default:
				}
			}
		}(f)
	}
	return c
}
