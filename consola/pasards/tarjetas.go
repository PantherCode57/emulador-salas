package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Las tarjetas de la ranura, vistas desde /sys (en las pruebas, desde una carpeta de mentira).
var raizSys = "/sys"

type tarjeta struct {
	disco string // mmcblk0
	cid   string // número de serie de la tarjeta
	tipo  string // SD (las de la ranura) o MMC (la memoria de dentro)
	host  string // el lector, en /sys (…/ff370000.mmc)
	tam   int64  // bytes
}

// El lector de la ranura y su controlador, para hacerle mirar otra vez.
type lector struct {
	host   string
	driver string // /sys/bus/platform/drivers/…
}

func leerSys(ruta string) string {
	b, _ := os.ReadFile(ruta)
	return strings.TrimSpace(string(b))
}

func leerTarjeta(disco string) (tarjeta, error) {
	d := filepath.Join(raizSys, "block", disco)
	real, err := filepath.EvalSymlinks(d)
	if err != nil {
		return tarjeta{}, err
	}
	cid := leerSys(filepath.Join(d, "device", "cid"))
	if cid == "" {
		return tarjeta{}, fmt.Errorf("%s: sin número de serie", disco)
	}
	real = filepath.ToSlash(real)
	i := strings.Index(real, "/mmc_host/")
	if i < 0 {
		return tarjeta{}, fmt.Errorf("%s: no es de un lector de tarjetas", disco)
	}
	sectores, _ := strconv.ParseInt(leerSys(filepath.Join(d, "size")), 10, 64)
	return tarjeta{disco: disco, cid: cid, tipo: leerSys(filepath.Join(d, "device", "type")),
		host: real[:i], tam: sectores * 512}, nil
}

// tarjetasSD: todas las tarjetas SD que se ven (sin la memoria de dentro ni sus «boot»).
func tarjetasSD() []tarjeta {
	m, _ := filepath.Glob(filepath.Join(raizSys, "block", "mmcblk*"))
	var r []tarjeta
	for _, x := range m {
		n := filepath.Base(x)
		if strings.Contains(n, "boot") || strings.Contains(n, "rpmb") {
			continue
		}
		if t, err := leerTarjeta(n); err == nil && t.tipo == "SD" {
			r = append(r, t)
		}
	}
	return r
}

// tarjetaDePanther: la tarjeta de Panther OS. La XF35H tiene una sola ranura, así que es la tarjeta
// SD que tiene algo montado (/boot, /roms…). No se mira «/»: en Panther OS no es un disco normal
// (sale como 0:18).
func tarjetaDePanther(mounts string) (tarjeta, error) {
	sds := tarjetasSD()
	if len(sds) == 0 {
		return tarjeta{}, fmt.Errorf("no veo ninguna tarjeta SD en la ranura")
	}
	var montadas []tarjeta
	for _, t := range sds {
		if len(montajesPropios(mounts, t.disco)) > 0 {
			montadas = append(montadas, t)
		}
	}
	if len(montadas) == 1 {
		return montadas[0], nil
	}
	if len(montadas) == 0 && len(sds) == 1 && strings.Contains(mounts, "/dev/root ") {
		return sds[0], nil // solo «/dev/root»: con una sola ranura, es esta
	}
	var n []string
	for _, t := range sds {
		n = append(n, t.disco)
	}
	return tarjeta{}, fmt.Errorf("no sé cuál es la tarjeta de Panther OS (SD: %s; montadas: %d)", strings.Join(n, ", "), len(montadas))
}

// tarjetasEn: las tarjetas que hay ahora en ese lector (cuando se cambia, la nueva puede salir con
// otro nombre, mmcblk1 en vez de mmcblk0, porque el sistema aún tiene abierta la de antes).
func tarjetasEn(host string) []tarjeta {
	m, _ := filepath.Glob(filepath.Join(raizSys, "block", "mmcblk*"))
	var r []tarjeta
	for _, x := range m {
		n := filepath.Base(x)
		if strings.Contains(n, "boot") || strings.Contains(n, "rpmb") {
			continue
		}
		t, err := leerTarjeta(n)
		if err == nil && t.host == host {
			r = append(r, t)
		}
	}
	return r
}

// particion: la primera partición de la tarjeta (o la tarjeta entera si no tiene) y su maj:min.
func particion(disco string) (nombre, dev string, err error) {
	d := filepath.Join(raizSys, "block", disco)
	m, _ := filepath.Glob(filepath.Join(d, disco+"p*"))
	mejor, n := "", 1<<30
	for _, x := range m {
		k, err := strconv.Atoi(strings.TrimPrefix(filepath.Base(x), disco+"p"))
		if err == nil && k < n {
			mejor, n = x, k
		}
	}
	if mejor != "" {
		if dev := leerSys(filepath.Join(mejor, "dev")); dev != "" {
			return filepath.Base(mejor), dev, nil
		}
	}
	if dev := leerSys(filepath.Join(d, "dev")); dev != "" {
		return disco, dev, nil
	}
	return "", "", fmt.Errorf("%s: no se ve ninguna partición", disco)
}

func lectorDe(host string) (lector, error) {
	drv, err := filepath.EvalSymlinks(filepath.Join(host, "driver"))
	if err != nil {
		return lector{}, err
	}
	return lector{host: host, driver: drv}, nil
}

// releer: suelta el lector y lo vuelve a coger, para que mire qué tarjeta hay.
func (l lector) releer() error {
	nombre := []byte(filepath.Base(l.host))
	if err := os.WriteFile(filepath.Join(l.driver, "unbind"), nombre, 0); err != nil {
		log.Printf("unbind: %v", err) // puede que ya estuviera suelto
	}
	time.Sleep(800 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(l.driver, "bind"), nombre, 0); err != nil {
		return err
	}
	time.Sleep(3 * time.Second) // lo que tarda en reconocer la tarjeta
	return nil
}

// montajesPropios: los puntos de montaje de las particiones de ese disco, con si están en solo lectura.
func montajesPropios(mounts, disco string) map[string]bool {
	r := map[string]bool{}
	for _, l := range strings.Split(mounts, "\n") {
		c := strings.Fields(l)
		if len(c) < 4 {
			continue
		}
		dev, punto, opciones := c[0], c[1], c[3]
		if dev != "/dev/"+disco && !strings.HasPrefix(dev, "/dev/"+disco+"p") {
			continue
		}
		r[punto] = strings.Split(opciones, ",")[0] == "ro"
	}
	return r
}

// montajesDe: los de ese disco y «/dev/root» (el disco de arranque, que en esta consola es la
// tarjeta de Panther OS), que son los que tienen que quedar en solo lectura.
func montajesDe(mounts, disco string) map[string]bool {
	r := montajesPropios(mounts, disco)
	for _, l := range strings.Split(mounts, "\n") {
		c := strings.Fields(l)
		if len(c) >= 4 && c[0] == "/dev/root" {
			r[c[1]] = strings.Split(c[3], ",")[0] == "ro"
		}
	}
	return r
}

func todoSoloLectura(mounts, disco string) bool {
	m := montajesDe(mounts, disco)
	if len(m) == 0 {
		return false
	}
	for _, ro := range m {
		if !ro {
			return false
		}
	}
	return true
}
