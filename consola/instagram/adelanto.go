package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Los reels se adelantan: mientras ves uno, el siguiente se baja entero a la memoria (/dev/shm es
// RAM: nada a la tarjeta) y, al pasar a él, ffmpeg lo lee de ahí en vez de abrirlo por internet. Se
// baja de uno en uno, se guardan como mucho cuatro y al salir se borran (y al abrir, los que hubiera
// dejado una vez anterior que no saliera bien).
const (
	carpetaAdelantos = "/dev/shm"
	prefijoAdelanto  = "instagram-reel-"
	maxAdelantos     = 4
	maxTamAdelanto   = 40 << 20
)

var clienteAdelanto = &http.Client{Timeout: 2 * time.Minute}

type adelantos struct {
	mu       sync.Mutex
	listos   map[string]string // clave de la dirección → archivo
	orden    []string
	bajando  string // la clave del que se está bajando
	cancelar context.CancelFunc
	sonando  string // el archivo que está sonando (ese no se borra)
}

func nuevosAdelantos() *adelantos {
	viejos, _ := filepath.Glob(filepath.Join(carpetaAdelantos, prefijoAdelanto+"*"))
	for _, v := range viejos {
		_ = os.Remove(v)
	}
	return &adelantos{listos: map[string]string{}}
}

// usando: el archivo (o la dirección) que va a sonar ahora.
func (ad *adelantos) usando(ruta string) {
	if ad == nil {
		return
	}
	ad.mu.Lock()
	ad.sonando = ruta
	ad.mu.Unlock()
}

// ruta: el archivo del vídeo de u si ya está bajado entero ("" si no).
func (ad *adelantos) ruta(u string) string {
	if ad == nil || u == "" {
		return ""
	}
	ad.mu.Lock()
	defer ad.mu.Unlock()
	return ad.listos[clave(u)]
}

// pedir: que se baje el vídeo de u (si no está ya) y, cuando esté, listo(su archivo) (desde otro
// hilo). Si se estaba bajando otro, se deja: el de ahora es el que se va a ver.
func (ad *adelantos) pedir(u string, listo func(ruta string)) {
	if ad == nil || u == "" || !strings.HasPrefix(u, "http") {
		return
	}
	k := clave(u)
	ad.mu.Lock()
	if r := ad.listos[k]; r != "" {
		ad.mu.Unlock()
		if listo != nil {
			go listo(r)
		}
		return
	}
	if ad.bajando == k {
		ad.mu.Unlock()
		return
	}
	if ad.cancelar != nil {
		ad.cancelar()
	}
	ctx, cancelar := context.WithCancel(context.Background())
	ad.bajando, ad.cancelar = k, cancelar
	ad.mu.Unlock()
	go func() {
		defer cancelar()
		t := time.Now()
		ruta, n, err := bajarVideo(ctx, u, filepath.Join(carpetaAdelantos, prefijoAdelanto+k+".mp4"))
		ad.mu.Lock()
		defer ad.mu.Unlock()
		if ad.bajando == k {
			ad.bajando, ad.cancelar = "", nil
		}
		if err != nil {
			if ctx.Err() == nil {
				logf("adelantar un reel: %v", err)
			}
			return
		}
		logf("reel adelantado: %d KB en %.1f s", n>>10, time.Since(t).Seconds())
		ad.listos[k] = ruta
		ad.orden = append(ad.orden, k)
		ad.tirarViejos()
		if listo != nil {
			go listo(ruta)
		}
	}()
}

// tirarViejos deja como mucho maxAdelantos (con ad.mu cogido); el que está sonando no se borra.
func (ad *adelantos) tirarViejos() {
	uso := ad.sonando
	for i := 0; len(ad.orden) > maxAdelantos && i < len(ad.orden); {
		k := ad.orden[i]
		if ad.listos[k] == uso {
			i++
			continue
		}
		_ = os.Remove(ad.listos[k])
		delete(ad.listos, k)
		ad.orden = append(ad.orden[:i], ad.orden[i+1:]...)
	}
}

// limpiar borra todos (al salir).
func (ad *adelantos) limpiar() {
	if ad == nil {
		return
	}
	ad.mu.Lock()
	defer ad.mu.Unlock()
	if ad.cancelar != nil {
		ad.cancelar()
	}
	for _, r := range ad.listos {
		_ = os.Remove(r)
	}
	ad.listos, ad.orden = map[string]string{}, nil
}

// bajarVideo baja u entero a ruta (primero a ruta.parte, y se cambia de nombre al acabar).
func bajarVideo(ctx context.Context, u, ruta string) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", agenteNavegador)
	req.Header.Set("Referer", urlWeb+"/")
	resp, err := clienteAdelanto.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", 0, fmt.Errorf("responde %d", resp.StatusCode)
	}
	if resp.ContentLength > maxTamAdelanto {
		return "", 0, fmt.Errorf("demasiado grande (%d MB)", resp.ContentLength>>20)
	}
	parte := ruta + ".parte"
	fh, err := os.OpenFile(parte, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, err
	}
	n, err := io.Copy(fh, io.LimitReader(resp.Body, maxTamAdelanto+1))
	if cerr := fh.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxTamAdelanto {
		err = fmt.Errorf("demasiado grande")
	}
	if err == nil && resp.ContentLength > 0 && n != resp.ContentLength {
		err = fmt.Errorf("a medias (%d de %d)", n, resp.ContentLength)
	}
	if err != nil {
		_ = os.Remove(parte)
		return "", 0, err
	}
	if err := os.Rename(parte, ruta); err != nil {
		_ = os.Remove(parte)
		return "", 0, err
	}
	return ruta, n, nil
}
