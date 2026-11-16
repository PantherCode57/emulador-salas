package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Descargas: las hace yt-dlp, una detrás de otra, en el MP4 de 360p (lo que mejor va en la consola).
// Se guardan en /roms/videos/youtube con sus datos y su miniatura, para verlas sin internet.
const formatoDescarga = formatoVertical + "18/bv*[height<=360][vcodec^=avc1]+ba[ext=m4a]/b[height<=480][vcodec^=avc1]/b[height<=480]"

var (
	muDescarga  sync.Mutex
	cmdDescarga *exec.Cmd
)

func (a *app) descargar(v Video) {
	switch {
	case v.Directo:
		a.aviso("Los directos no se pueden descargar")
	case a.alm.descargado(v.ID) != nil:
		a.aviso("Ya está descargado")
	case a.descEnCola[v.ID]:
		a.aviso("Ya se está descargando")
	case !hayRed():
		a.aviso("Sin internet: no se puede descargar")
	default:
		select {
		case a.descCola <- v:
			a.descEnCola[v.ID] = true
			a.descPend++
			a.aviso("Se descarga «" + recortar(a.f.pequena, a.f.limpiar(v.Titulo), 300) + "»")
		default:
			a.aviso("Hay demasiadas descargas esperando")
		}
	}
}

func (a *app) descargador() {
	for {
		select {
		case <-a.salir:
			return
		case v := <-a.descCola:
			a.bajarVideo(v)
		}
	}
}

func (a *app) bajarVideo(v Video) {
	a.hacer(func() { a.descActual, a.descPct = v.Titulo, 0 })
	ultimo := -1
	d, err := bajarConYtdlp(v, func(p float64) {
		if int(p) != ultimo {
			ultimo = int(p)
			a.hacer(func() { a.descPct = p })
		}
	})
	a.hacer(func() {
		a.descPend--
		delete(a.descEnCola, v.ID)
		if a.descPend <= 0 {
			a.descPend, a.descActual = 0, ""
		}
		if err != nil {
			logf("descarga de %s: %v", v.ID, err)
			a.descFallidas++
			a.aviso("No se ha podido descargar «" + recortar(a.f.pequena, a.f.limpiar(v.Titulo), 260) + "»")
			return
		}
		a.alm.cargarDescargados()
		a.aviso("Descargado: «" + recortar(a.f.pequena, a.f.limpiar(d.Titulo), 300) + "»")
	})
}

func bajarConYtdlp(v Video, progreso func(float64)) (*Descargado, error) {
	if err := os.MkdirAll(carpetaVideos, 0o755); err != nil {
		return nil, err
	}
	ruta := filepath.Join(carpetaVideos, v.ID+".mp4")
	cmd := exec.Command(ytdlpBin, "-f", formatoDescarga, "--merge-output-format", "mp4", "-o", ruta,
		"--no-playlist", "--no-warnings", "--newline", "--progress",
		"--progress-template", "download:PCT %(progress._percent_str)s",
		"--print", "after_move:FPS %(fps)s", "--print", "after_move:TAM %(width)s %(height)s", urlVideo(v.ID))
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	muDescarga.Lock()
	if err := cmd.Start(); err != nil {
		muDescarga.Unlock()
		return nil, err
	}
	cmdDescarga = cmd
	muDescarga.Unlock()
	fps := 0.0
	vertical := false
	ultimaLinea := ""
	leido := make(chan struct{})
	go func() {
		defer close(leido)
		s := bufio.NewScanner(pr)
		for s.Scan() {
			l := strings.TrimSpace(s.Text())
			switch {
			case strings.HasPrefix(l, "PCT "):
				if p, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(l, "PCT "), "%")), 64); err == nil {
					progreso(p)
				}
			case strings.HasPrefix(l, "FPS "):
				fps, _ = strconv.ParseFloat(strings.TrimPrefix(l, "FPS "), 64)
			case strings.HasPrefix(l, "TAM "):
				var w, h float64
				if _, err := fmt.Sscanf(l, "TAM %g %g", &w, &h); err == nil {
					vertical = h > w*1.1
				}
			case l != "":
				ultimaLinea = l
			}
		}
	}()
	err := cmd.Wait()
	pw.Close()
	<-leido
	muDescarga.Lock()
	cmdDescarga = nil
	muDescarga.Unlock()
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, ultimaLinea)
	}
	st, err := os.Stat(ruta)
	if err != nil {
		return nil, fmt.Errorf("yt-dlp no ha dejado el vídeo: %s", ultimaLinea)
	}
	// La miniatura, junto al vídeo (la de la memoria si ya estaba bajada).
	jpg := filepath.Join(carpetaVideos, v.ID+".jpg")
	if b, err := os.ReadFile(filepath.Join(carpetaDatos, "miniaturas", v.claveMini()+".jpg")); err == nil {
		_ = os.WriteFile(jpg, b, 0o644)
	} else {
		_ = bajarMiniatura(v.miniatura(), jpg)
	}
	d := &Descargado{Video: v, FPS: fps, Vertical: vertical, Bytes: st.Size(), Cuando: ahoraMs()}
	guardarJSON(filepath.Join(carpetaVideos, v.ID+".json"), d)
	return d, nil
}

// pararDescargas corta la descarga en curso (al salir; yt-dlp la sigue la próxima vez).
func (a *app) pararDescargas() {
	muDescarga.Lock()
	defer muDescarga.Unlock()
	if cmdDescarga != nil && cmdDescarga.Process != nil {
		_ = cmdDescarga.Process.Kill()
	}
}
