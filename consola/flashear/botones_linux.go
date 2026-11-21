//go:build linux

package main

import (
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
)

// escucharBotones: los botones de la consola leídos directamente (/dev/input, sin EmulationStation en
// medio), con los mismos códigos que usa el WhatsApp de la consola (mandos_linux.go). La cruceta
// puede llegar como botones o como «hat» (ABS_HAT0X/Y). No cuentan el de encender ni el volumen.
func escucharBotones() <-chan tecla {
	c := make(chan tecla, 8)
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
				t := tNada
				switch {
				case tipo == 1 && valor == 1: // EV_KEY, al pulsar
					t = teclaDe(codigo)
				case tipo == 3 && (codigo == 0x10 || codigo == 0x11) && valor != 0: // ABS_HAT0X/Y
					switch {
					case codigo == 0x10 && valor < 0:
						t = tIzq
					case codigo == 0x10:
						t = tDer
					case valor < 0:
						t = tArriba
					default:
						t = tAbajo
					}
				}
				if t == tNada {
					continue
				}
				select {
				case c <- t:
				default:
				}
			}
		}(f)
	}
	return c
}

func teclaDe(codigo uint16) tecla {
	switch codigo {
	case 116, 114, 115: // KEY_POWER, KEY_VOLUMEDOWN, KEY_VOLUMEUP
		return tNada
	case 0x131: // BTN_EAST: la A de la consola
		return tA
	case 0x130: // BTN_SOUTH: la B
		return tB
	case 0x133: // BTN_NORTH
		return tX
	case 0x134: // BTN_WEST
		return tY
	case 0x220: // BTN_DPAD_UP
		return tArriba
	case 0x221:
		return tAbajo
	case 0x222:
		return tIzq
	case 0x223:
		return tDer
	case 0x13b, 0x2c1: // BTN_START, BTN_TRIGGER_HAPPY2
		return tStart
	case 0x13a, 0x2c0: // BTN_SELECT, BTN_TRIGGER_HAPPY1
		return tSelect
	}
	return tOtra
}
