//go:build linux

package main

import (
	"encoding/binary"
	"image"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// capaCompartida: una imagen que otro programa pinta encima de lo suyo. La leen el emulador de Panther
// y el menú de la consola (ver consola/emulador/capa.c, que explica el formato). Hay dos:
//   - /dev/shm/panther-capa: el aviso de arriba (modo 1) o el chat abierto encima (modo 2).
//   - /dev/shm/panther-panel: los mensajes de la derecha de la pantalla principal (modo 1).
type capaCompartida struct {
	ruta string
	mem  []byte
	tam  int
	ino  uint64
	// lo último que se puso, para volver a ponerlo si el archivo desaparece (ver vigilar)
	modo, x, y int
	img        *image.RGBA
	dura       time.Duration
	empieza    time.Time
}

const cabeceraCapa = 64

func abrirCapa(ruta string, maxAncho, maxAlto int) (*capaCompartida, error) {
	c := &capaCompartida{ruta: ruta, tam: cabeceraCapa + maxAncho*maxAlto*4}
	if err := c.mapear(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *capaCompartida) mapear() error {
	f, err := os.OpenFile(c.ruta, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_ = f.Chmod(0o644)
	st, err := f.Stat()
	if err != nil || st.Size() != int64(c.tam) {
		if err := f.Truncate(int64(c.tam)); err != nil {
			return err
		}
	}
	mem, err := syscall.Mmap(int(f.Fd()), 0, c.tam, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}
	if c.mem != nil {
		_ = syscall.Munmap(c.mem)
	}
	c.mem = mem
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		c.ino = s.Ino
	}
	copy(mem[0:4], "PCAP")
	binary.LittleEndian.PutUint32(mem[4:], 1)
	if atomic.LoadUint32(c.u32(8))&1 == 1 { // se quedó a medias (el servicio se cerró escribiendo)
		atomic.AddUint32(c.u32(8), 1)
	}
	return nil
}

// vigilar: systemd borra los archivos de /dev/shm de un usuario cuando se cierra su última sesión
// (RemoveIPC). Si ha desaparecido, se vuelve a crear con lo que tenía; el menú y el emulador lo
// vuelven a abrir solos.
func (c *capaCompartida) vigilar() {
	if c == nil {
		return
	}
	st, err := os.Stat(c.ruta)
	if err == nil {
		if s, ok := st.Sys().(*syscall.Stat_t); ok && s.Ino == c.ino {
			return
		}
	}
	if err := c.mapear(); err != nil {
		logf("no se puede volver a crear %s: %v", c.ruta, err)
		return
	}
	logf("%s había desaparecido: vuelto a crear", c.ruta)
	c.escribir(c.modo, c.img, c.x, c.y, c.dura, c.empieza)
}

func (c *capaCompartida) u32(off int) *uint32 { return (*uint32)(unsafe.Pointer(&c.mem[off])) }

// poner deja una imagen nueva (img nil = quitar lo que hubiera). La imagen de Go va premultiplicada;
// aquí se guarda sin premultiplicar, que es lo que esperan los que la leen.
func (c *capaCompartida) poner(modo int, img *image.RGBA, x, y int, dura time.Duration) {
	if c == nil {
		return
	}
	c.modo, c.img, c.x, c.y, c.dura, c.empieza = modo, img, x, y, dura, time.Now()
	c.escribir(modo, img, x, y, dura, c.empieza)
}

func (c *capaCompartida) escribir(modo int, img *image.RGBA, x, y int, dura time.Duration, empieza time.Time) {
	atomic.AddUint32(c.u32(8), 1) // impar: escribiendo
	w, h := 0, 0
	if img != nil && modo != 0 {
		b := img.Bounds()
		w, h = b.Dx(), b.Dy()
		if cabeceraCapa+w*h*4 > c.tam {
			w, h, modo = 0, 0, 0
		} else {
			dst := c.mem[cabeceraCapa:]
			for j := 0; j < h; j++ {
				fila := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+j):]
				for i := 0; i < w; i++ {
					p := fila[i*4 : i*4+4]
					d := dst[(j*w+i)*4 : (j*w+i)*4+4]
					a := uint32(p[3])
					switch a {
					case 0:
						d[0], d[1], d[2], d[3] = 0, 0, 0, 0
					case 255:
						d[0], d[1], d[2], d[3] = p[0], p[1], p[2], 255
					default:
						d[0] = uint8(min(uint32(p[0])*255/a, 255))
						d[1] = uint8(min(uint32(p[1])*255/a, 255))
						d[2] = uint8(min(uint32(p[2])*255/a, 255))
						d[3] = uint8(a)
					}
				}
			}
		}
	}
	le := binary.LittleEndian
	le.PutUint32(c.mem[12:], uint32(modo))
	le.PutUint32(c.mem[16:], uint32(w))
	le.PutUint32(c.mem[20:], uint32(h))
	le.PutUint32(c.mem[24:], uint32(x))
	le.PutUint32(c.mem[28:], uint32(y))
	le.PutUint64(c.mem[32:], uint64(empieza.UnixMilli()))
	le.PutUint32(c.mem[40:], uint32(dura.Milliseconds()))
	c.latido()
	atomic.AddUint32(c.u32(8), 1) // par: listo
}

// latido: con el chat abierto encima se pone cada segundo; si el servicio se cae, deja de llegar y
// quien lo pinta lo quita solo.
func (c *capaCompartida) latido() {
	if c != nil {
		atomic.StoreUint32(c.u32(44), uint32(time.Now().Unix()))
	}
}

// hayQuienPinte: el emulador y el menú tocan /dev/shm/panther-capa-viendo cada segundo mientras están
// en pantalla. Si nadie lo hace (un emulador de los de antes, por ejemplo), abrir el chat encima no
// se vería: entonces no se abre.
func hayQuienPinte() bool {
	st, err := os.Stat("/dev/shm/panther-capa-viendo")
	return err == nil && time.Since(st.ModTime()) < 3*time.Second
}
