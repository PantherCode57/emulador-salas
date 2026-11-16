//go:build linux

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

// mando: el mando de la consola leído directamente (evdev), para el servicio: ver si pulsas el botón
// de abrir el chat y, con el chat abierto encima, quedárselo para él solo (EVIOCGRAB) para que ni el
// juego ni el menú se enteren de lo que pulsas.
type mando struct {
	f        *os.File
	mu       sync.Mutex
	estado   uint32
	agarrado bool
	cambios  chan uint32
}

// Botones extra (los de RetroPad están en pantalla.go).
const (
	bL2 = 1 << 12
	bR2 = 1 << 13
	bL3 = 1 << 14
	bR3 = 1 << 15
	bF  = 1 << 16
)

func bitDe(codigo uint16) uint32 {
	switch codigo {
	case 0x130: // BTN_SOUTH
		return bB
	case 0x131: // BTN_EAST
		return bA
	case 0x133: // BTN_NORTH
		return bX
	case 0x134: // BTN_WEST
		return bY
	case 0x136: // BTN_TL
		return bL
	case 0x137: // BTN_TR
		return bR
	case 0x138: // BTN_TL2
		return bL2
	case 0x139: // BTN_TR2
		return bR2
	case 0x220: // BTN_DPAD_UP
		return bArriba
	case 0x221:
		return bAbajo
	case 0x222:
		return bIzquierda
	case 0x223:
		return bDerecha
	case 0x2c0: // BTN_TRIGGER_HAPPY1
		return bSelect
	case 0x2c1:
		return bStart
	case 0x2c2:
		return bL3
	case 0x2c3:
		return bR3
	case 0x2c4:
		return bF
	case 0x13a: // BTN_SELECT
		return bSelect
	case 0x13b: // BTN_START
		return bStart
	case 0x13c: // BTN_MODE
		return bF
	case 0x13d: // BTN_THUMBL
		return bL3
	case 0x13e: // BTN_THUMBR
		return bR3
	}
	return 0
}

func ioc(dir, nr, tam uintptr) uintptr { return dir<<30 | tam<<16 | 'E'<<8 | nr }

func ioctl(fd uintptr, pet uintptr, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, pet, arg); e != 0 {
		return e
	}
	return nil
}

func abrirMando() (*mando, error) {
	rutas, _ := filepath.Glob("/dev/input/event*")
	for _, r := range rutas {
		f, err := os.OpenFile(r, os.O_RDONLY, 0)
		if err != nil {
			continue
		}
		var bits [96]byte // EV_KEY hasta KEY_MAX (0x2ff)
		if ioctl(f.Fd(), ioc(2, 0x20+1, uintptr(len(bits))), uintptr(unsafe.Pointer(&bits[0]))) != nil ||
			bits[0x130/8]&(1<<(0x130%8)) == 0 { // sin BTN_SOUTH: no es el mando
			f.Close()
			continue
		}
		m := &mando{f: f, cambios: make(chan uint32, 64)}
		m.estado = m.leerEstado()
		go m.leer()
		logf("mando: %s", r)
		return m, nil
	}
	return nil, fmt.Errorf("no encuentro el mando")
}

// leerEstado: lo que está pulsado de verdad ahora mismo.
func (m *mando) leerEstado() uint32 {
	var bits [96]byte
	if ioctl(m.f.Fd(), ioc(2, 0x18, uintptr(len(bits))), uintptr(unsafe.Pointer(&bits[0]))) != nil {
		return 0
	}
	var e uint32
	for c := 0; c < len(bits)*8; c++ {
		if bits[c/8]&(1<<(c%8)) != 0 {
			e |= bitDe(uint16(c))
		}
	}
	return e
}

func (m *mando) leer() {
	buf := make([]byte, 24*32)
	for {
		n, err := m.f.Read(buf)
		if err != nil {
			logf("mando: %v", err)
			return
		}
		m.mu.Lock()
		antes := m.estado
		for i := 0; i+24 <= n; i += 24 {
			tipo := binary.LittleEndian.Uint16(buf[i+16:])
			codigo := binary.LittleEndian.Uint16(buf[i+18:])
			valor := int32(binary.LittleEndian.Uint32(buf[i+20:]))
			if tipo == 0 && codigo == 3 { // SYN_DROPPED: se han perdido cosas
				m.estado = m.leerEstado()
				continue
			}
			if tipo != 1 { // EV_KEY
				continue
			}
			if b := bitDe(codigo); b != 0 {
				if valor != 0 {
					m.estado |= b
				} else {
					m.estado &^= b
				}
			}
		}
		e := m.estado
		m.mu.Unlock()
		if e != antes {
			select {
			case m.cambios <- e:
			default:
			}
		}
	}
}

func (m *mando) ahora() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.estado
}

// agarrar: con true, solo nosotros recibimos lo que se pulsa; con false, se suelta.
func (m *mando) agarrar(si bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.agarrado == si {
		return
	}
	v := 0
	if si {
		v = 1
	}
	if err := ioctl(m.f.Fd(), ioc(1, 0x90, 4), uintptr(v)); err != nil {
		logf("mando: no se puede %s: %v", map[bool]string{true: "agarrar", false: "soltar"}[si], err)
		return
	}
	m.agarrado = si
}
