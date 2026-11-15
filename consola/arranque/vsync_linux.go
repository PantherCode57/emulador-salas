package main

import (
	"syscall"
	"unsafe"
)

const fbioWaitForVsync = 0x40044620 // _IOW('F', 0x20, __u32)

// esperarVsync espera al refresco de la pantalla para que no se vea el corte a medio dibujar.
// Si el controlador no lo admite, no pasa nada: se sigue al ritmo del reloj.
func esperarVsync(fd uintptr) {
	var cero uint32
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd, fbioWaitForVsync, uintptr(unsafe.Pointer(&cero)))
}
