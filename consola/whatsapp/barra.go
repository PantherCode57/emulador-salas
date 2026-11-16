package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Lo que el servicio pone en todas las pantallas (menos la principal, que ya tiene la suya):
//   - el recuadro negro de arriba a la derecha con la batería, la hora y, si tienes mensajes sin
//     leer, el logo de WhatsApp (/dev/shm/panther-barra);
//   - la barra del volumen a la izquierda al pulsar + o − (/dev/shm/panther-volumen).

const (
	altoBarra    = 22
	maxAnchoBarr = 260
	anchoVolumen = 52
	altoVolumen  = 236
)

// ---- Batería ----

func leerSys(ruta string) string {
	b, err := os.ReadFile(ruta)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// bateria: el porcentaje (-1 si no se sabe) y si está cargando.
func bateria() (int, bool) {
	n, err := strconv.Atoi(leerSys("/sys/class/power_supply/battery/capacity"))
	if err != nil {
		return -1, false
	}
	estado := leerSys("/sys/class/power_supply/battery/status")
	return n, estado == "Charging" || estado == "Full"
}

// ---- El recuadro de arriba ----

// estadoBarra: lo que sale en el recuadro (si no cambia, no se vuelve a pintar).
type estadoBarra struct {
	hora      string
	bateria   int
	cargando  bool
	whatsapp  bool
	grabando  bool
}

func (a *app) estadoBarra() estadoBarra {
	b, c := bateria()
	e := estadoBarra{hora: time.Now().Format("15:04"), bateria: b, cargando: c, grabando: a.grabando()}
	if a.w != nil {
		for _, ch := range a.w.chats {
			if ch.NoLeidos > 0 && len(ch.Mensajes) > 0 {
				e.whatsapp = true
				break
			}
		}
	}
	return e
}

var logoPequeño = func() image.Image {
	if logoCarga == nil {
		return nil
	}
	return ajustarA(logoCarga, 16, 16)
}()

// pintarPila: el icono de la batería (22×11 con la puntita) lleno según lo que quede.
func pintarPila(l *lienzo, x, y, nivel int, cargando bool) {
	blanco := color.NRGBA{0xff, 0xff, 0xff, 0xe8}
	l.redondeado(image.Rect(x, y, x+20, y+11), 3, blanco)
	l.redondeado(image.Rect(x+1, y+1, x+19, y+10), 2, color.NRGBA{0, 0, 0, 0xff})
	l.rect(image.Rect(x+20, y+3, x+22, y+8), blanco)
	if nivel < 0 {
		return
	}
	c := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	switch {
	case cargando:
		c = color.NRGBA{0x4c, 0xd9, 0x64, 0xff}
	case nivel <= 15:
		c = color.NRGBA{0xff, 0x45, 0x3a, 0xff}
	}
	w := max(1, 16*min(nivel, 100)/100)
	l.redondeado(image.Rect(x+2, y+2, x+2+w, y+9), 1, c)
}

func (a *app) dibujarBarra(e estadoBarra) *image.RGBA {
	cp := letrasPanel()
	pct := ""
	if e.bateria >= 0 {
		pct = fmt.Sprintf("%d%%", e.bateria)
	}
	w := 10
	if e.grabando {
		w += 10 + 4 + medir(cp.mini, "REC") + 10
	}
	if e.whatsapp {
		w += 16 + 10
	}
	w += 22 + 5 + medir(cp.mini, pct) + 10 + medir(cp.texto, e.hora) + 10
	w = min(w, maxAnchoBarr)
	l := lienzoDe(a.f, w, altoBarra)
	l.redondeado(l.img.Bounds(), altoBarra/2, color.NRGBA{0, 0, 0, 0xe6})
	x := 10
	if e.grabando {
		l.circulo(x+5, altoBarra/2, 5, color.NRGBA{0xff, 0x3b, 0x30, 0xff})
		x += 14
		x = l.texto(x, baseCentrada(l.img.Bounds(), cp.mini), "REC", cp.mini, color.NRGBA{0xff, 0x9f, 0x99, 0xff}) + 10
	}
	if e.whatsapp {
		if logoPequeño != nil {
			l.pegar(logoPequeño, x, (altoBarra-16)/2)
		} else {
			l.circulo(x+8, altoBarra/2, 8, colVerde)
		}
		x += 16 + 10
	}
	pintarPila(l, x, (altoBarra-11)/2, e.bateria, e.cargando)
	x += 22 + 5
	if pct != "" {
		x = l.texto(x, baseCentrada(l.img.Bounds(), cp.mini), pct, cp.mini, colTexto) + 10
	}
	l.texto(x, baseCentrada(l.img.Bounds(), cp.texto), e.hora, cp.texto, colBlanco)
	return l.img
}

// ponerBarra: vuelve a pintar el recuadro si ha cambiado algo.
func (a *app) ponerBarra(forzar bool) {
	e := a.estadoBarra()
	if !forzar && e == a.barraVista {
		return
	}
	a.barraVista = e
	img := a.dibujarBarra(e)
	a.barra.poner(1, img, 640-8-img.Bounds().Dx(), 4, 0)
}

// ---- El volumen ----

var reVolumen = regexp.MustCompile(`\[(\d+)%\]`)

func leerVolumen() int {
	out, err := exec.Command("amixer", "sget", "Playback").Output()
	if err != nil {
		return -1
	}
	if m := reVolumen.FindSubmatch(out); m != nil {
		v, _ := strconv.Atoi(string(m[1]))
		return v
	}
	return -1
}

// cambiarVolumen: + y − suben y bajan de 5 en 5 (el control «Playback», el mismo que el menú).
func (a *app) cambiarVolumen(paso int) {
	signo := "+"
	if paso < 0 {
		signo, paso = "-", -paso
	}
	_ = exec.Command("amixer", "-q", "sset", "Playback", strconv.Itoa(paso)+"%"+signo).Run()
	v := leerVolumen()
	if v < 0 {
		return
	}
	img := a.dibujarVolumen(v)
	a.volumen.poner(1, img, 14, (480-altoVolumen)/2, 1500*time.Millisecond)
}

// pintarAltavoz: el altavoz (y las ondas si hay sonido), en blanco.
func pintarAltavoz(l *lienzo, cx, cy int, conSonido bool) {
	b := color.NRGBA{0xff, 0xff, 0xff, 0xf0}
	l.rect(image.Rect(cx-8, cy-3, cx-4, cy+4), b)
	for i := 0; i < 6; i++ { // el cono
		l.rect(image.Rect(cx-4+i, cy-3-i, cx-3+i, cy+4+i), b)
	}
	if conSonido {
		l.rect(image.Rect(cx+5, cy-2, cx+7, cy+3), b)
		l.rect(image.Rect(cx+8, cy-5, cx+10, cy+6), b)
	} else {
		for i := -3; i <= 3; i++ { // la equis
			l.rect(image.Rect(cx+7+i, cy+i, cx+9+i, cy+i+1), b)
			l.rect(image.Rect(cx+7+i, cy-i, cx+9+i, cy-i+1), b)
		}
	}
}

// dibujarVolumen: la barra de la izquierda, como la de Spotin pero de pie.
func (a *app) dibujarVolumen(v int) *image.RGBA {
	cp := letrasPanel()
	l := lienzoDe(a.f, anchoVolumen, altoVolumen)
	l.redondeado(l.img.Bounds(), anchoVolumen/2, color.NRGBA{0, 0, 0, 0xd8})
	l.centrado(image.Rect(0, 10, anchoVolumen, 34), fmt.Sprintf("%d", v), cp.nombre, colTexto)
	x0, x1 := anchoVolumen/2-4, anchoVolumen/2+4
	y0, y1 := 42, altoVolumen-44
	l.redondeado(image.Rect(x0, y0, x1, y1), 4, color.NRGBA{0x60, 0x60, 0x60, 0x90})
	lleno := (y1 - y0) * min(max(v, 0), 100) / 100
	if lleno > 0 {
		l.redondeado(image.Rect(x0, y1-lleno, x1, y1), 4, colVerde)
	}
	pintarAltavoz(l, anchoVolumen/2-1, altoVolumen-22, v > 0)
	return l.img
}
