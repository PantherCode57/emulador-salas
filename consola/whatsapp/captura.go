package main

import (
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Capturas y grabaciones de la pantalla, estés donde estés:
//   - L1 + R1 a la vez: una captura (PNG) en /roms/capturas;
//   - L2 + R2 a la vez: empieza a grabar (MP4, sin sonido) en /roms/videos/grabaciones, y otra vez
//     para parar. Mientras graba, el recuadro de arriba lleva «REC».
// La imagen la saca «captura» (consola/captura), que lee la pantalla con permisos de root.

var (
	programaCaptura = entorno("PANTHER_CAPTURA", "/home/ark/panther/emulador/captura")
	dirCapturas     = entorno("PANTHER_CAPTURAS", "/roms/capturas")
	dirGrabaciones  = entorno("PANTHER_GRABACIONES", "/roms/videos/grabaciones")
)

type grabacion struct {
	cmd   *exec.Cmd
	ruta  string
	hecho chan struct{}
}

func (a *app) grabando() bool { return a.grab != nil }

// toast: un aviso cortito arriba (en el sitio del de WhatsApp).
func (a *app) toast(s string) {
	if a.modo == modoCapa { // con el chat abierto encima, el aviso va dentro del chat
		a.aviso(s)
		return
	}
	cp := letrasPanel()
	w := medir(cp.texto, s) + 40
	l := lienzoDe(a.f, w, 34)
	l.redondeado(l.img.Bounds(), 17, color.NRGBA{0, 0, 0, 0xe6})
	l.centrado(l.img.Bounds(), s, cp.texto, colBlanco)
	a.capa.poner(1, l.img, (640-w)/2, 10, 1500*time.Millisecond)
}

func nombreConHora(dir, ext string) string {
	return filepath.Join(dir, time.Now().Format("2006-01-02_15-04-05")+ext)
}

func (a *app) hacerCaptura() {
	_ = os.MkdirAll(dirCapturas, 0o755)
	ruta := nombreConHora(dirCapturas, ".png")
	go func() {
		err := exec.Command("sudo", "-n", programaCaptura, "foto", ruta).Run()
		a.hacer(func() {
			if err != nil {
				logf("captura: %v", err)
				a.toast("No se ha podido hacer la captura")
				return
			}
			a.toast("Captura guardada")
		})
	}()
}

// cambiarGrabacion: empieza o para de grabar.
func (a *app) cambiarGrabacion() {
	if a.grab != nil {
		g := a.grab
		a.grab = nil
		// Se para el que lee la pantalla; ffmpeg ve que se acaba y cierra el vídeo bien.
		if g.cmd.Process != nil {
			señalGrupo(g.cmd, syscall.SIGTERM)
		}
		go func() {
			select {
			case <-g.hecho:
			case <-time.After(10 * time.Second):
				señalGrupo(g.cmd, syscall.SIGKILL)
			}
			a.hacer(func() { a.toast("Grabación guardada"); a.ponerBarra(true) })
		}()
		a.ponerBarra(true)
		return
	}
	_ = os.MkdirAll(dirGrabaciones, 0o755)
	ruta := nombreConHora(dirGrabaciones, ".mp4")
	// 20 imágenes por segundo: lo que da de sí leer la pantalla y comprimir sin frenar los juegos.
	orden := "sudo -n " + programaCaptura + " crudo 20 | nice -n 10 ffmpeg -hide_banner -loglevel error -y " +
		"-f rawvideo -pixel_format bgr0 -video_size 640x480 -framerate 20 -i - " +
		"-c:v libx264 -preset ultrafast -crf 26 -pix_fmt yuv420p -movflags +faststart '" + ruta + "'"
	cmd := exec.Command("sh", "-c", orden)
	grupoPropio(cmd)
	if err := cmd.Start(); err != nil {
		logf("grabar: %v", err)
		a.toast("No se ha podido grabar")
		return
	}
	g := &grabacion{cmd: cmd, ruta: ruta, hecho: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(g.hecho)
		a.hacer(func() {
			if a.grab == g { // se ha parado sola (algo ha fallado)
				a.grab = nil
				a.toast("La grabación se ha cortado")
				a.ponerBarra(true)
			}
		})
	}()
	a.grab = g
	a.toast("Grabando… (L2 + R2 para parar)")
	a.ponerBarra(true)
}

// ---- Combinaciones de botones del servicio (en cualquier pantalla) ----

func (a *app) combinaciones(e, nuevos uint32) {
	if nuevos&(bL|bR) != 0 && e&bL != 0 && e&bR != 0 {
		a.hacerCaptura()
	}
	if nuevos&(bL2|bR2) != 0 && e&bL2 != 0 && e&bR2 != 0 {
		a.cambiarGrabacion()
	}
}
