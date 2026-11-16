package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// descargar baja el sonido a «destino» por trozos de 4 MB (YouTube sirve muy despacio las
// peticiones de archivo entero). Se escribe en «destino.parte» y se renombra al acabar.
func descargar(f *fuenteSonido, destino string, progreso func(hecho, total int64)) error {
	const trozo = 4 << 20
	tmp := destino + ".parte"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	cliente := &http.Client{Timeout: 90 * time.Second}
	total, hecho := f.Tamano, int64(0)
	if total <= 0 {
		total = -1
	}
	fallo := func(e error) error {
		out.Close()
		_ = os.Remove(tmp)
		return e
	}
	for total < 0 || hecho < total {
		req, err := http.NewRequest(http.MethodGet, f.Ruta, nil)
		if err != nil {
			return fallo(err)
		}
		for k, v := range f.Cabeceras {
			req.Header.Set(k, v)
		}
		fin := hecho + trozo - 1
		if total > 0 && fin >= total {
			fin = total - 1
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", hecho, fin))
		resp, err := cliente.Do(req)
		if err != nil {
			return fallo(err)
		}
		switch resp.StatusCode {
		case http.StatusPartialContent:
			// Content-Range: bytes a-b/total
			if cr := resp.Header.Get("Content-Range"); total < 0 {
				if i := strings.LastIndex(cr, "/"); i >= 0 {
					if t, err := strconv.ParseInt(cr[i+1:], 10, 64); err == nil {
						total = t
					}
				}
			}
		case http.StatusOK:
			if resp.ContentLength > 0 {
				total = resp.ContentLength
			}
		default:
			resp.Body.Close()
			return fallo(fmt.Errorf("YouTube responde %d", resp.StatusCode))
		}
		n, err := io.Copy(out, resp.Body)
		resp.Body.Close()
		hecho += n
		if progreso != nil {
			progreso(hecho, total)
		}
		if err != nil {
			return fallo(err)
		}
		if n == 0 {
			return fallo(errors.New("la descarga se quedó parada"))
		}
		if resp.StatusCode == http.StatusOK { // vino entero de una vez
			break
		}
		if total < 0 {
			return fallo(errors.New("no se sabe el tamaño"))
		}
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, destino)
}
