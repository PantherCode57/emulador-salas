package main

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// Cliente para todo lo que se baja: sin límite de tiempo total (los juegos grandes tardan), pero
// sí para conectar y para la primera respuesta.
var cliente = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
	},
}

const agente = "PantherStore/1 (consola portátil)"

// errSinRed: no hay conexión (para decirlo así, sin el error largo de Go).
var errSinRed = errors.New("no hay conexión a internet")

func explicarRed(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	var dns *net.DNSError
	var op *net.OpError
	if errors.As(err, &dns) || errors.As(err, &op) {
		return errSinRed
	}
	return err
}

// leerURL baja algo pequeño (un índice, una imagen) a la memoria.
func leerURL(ctx context.Context, url string, max int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agente)
	resp, err := cliente.Do(req)
	if err != nil {
		return nil, explicarRed(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s contesta %d", recortarURL(url), resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, explicarRed(err)
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s es demasiado grande", recortarURL(url))
	}
	return b, nil
}

func recortarURL(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexByte(u, '/'); i > 0 {
		return u[:i]
	}
	return u
}

// suma: cómo se comprueba lo bajado («md5:…» o «sha256:…»; vacío, sin comprobar).
func nuevaSuma(s string) (hash.Hash, string) {
	switch {
	case strings.HasPrefix(s, "md5:"):
		return md5.New(), strings.ToLower(s[4:])
	case strings.HasPrefix(s, "sha256:"):
		return sha256.New(), strings.ToLower(s[7:])
	}
	return nil, ""
}

// bajar guarda url en destino (pasando por destino.part) y va diciendo cuánto lleva. Si ya hay un
// .part, sigue donde se quedó (Range); si el servidor no sabe seguir, empieza de nuevo.
func bajar(ctx context.Context, url, destino string, tam int64, suma string, progreso func(hecho, total int64)) error {
	part := destino + ".part"
	var desde int64
	if st, err := os.Stat(part); err == nil && st.Size() > 0 && (tam == 0 || st.Size() < tam) {
		desde = st.Size()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", agente)
	if desde > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", desde))
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return explicarRed(err)
	}
	defer resp.Body.Close()
	modo := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	switch {
	case resp.StatusCode == 206 && desde > 0:
		modo = os.O_WRONLY | os.O_APPEND
	case resp.StatusCode == 200:
		desde = 0
	default:
		return fmt.Errorf("%s contesta %d", recortarURL(url), resp.StatusCode)
	}
	total := tam
	if total == 0 && resp.ContentLength > 0 {
		total = desde + resp.ContentLength
	}
	f, err := os.OpenFile(part, modo, 0o644)
	if err != nil {
		return err
	}
	buf := make([]byte, 256<<10)
	hecho := desde
	ultimo := time.Time{}
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			hecho += int64(n)
			if time.Since(ultimo) > 150*time.Millisecond {
				ultimo = time.Now()
				progreso(hecho, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			f.Close()
			return explicarRed(err)
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	progreso(hecho, total)
	if tam > 0 && hecho != tam {
		return fmt.Errorf("se ha bajado a medias (%s de %s)", tamTexto(hecho), tamTexto(tam))
	}
	if h, quiero := nuevaSuma(suma); h != nil {
		if err := sumar(part, h); err != nil {
			return err
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != quiero {
			_ = os.Remove(part)
			return fmt.Errorf("lo bajado no es lo que tenía que ser (la suma no coincide)")
		}
	}
	return os.Rename(part, destino)
}

func sumar(ruta string, h hash.Hash) error {
	f, err := os.Open(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(h, f)
	return err
}

// tamTexto: «8,4 MB», «186 MB», «512 KB».
func tamTexto(n int64) string {
	switch {
	case n <= 0:
		return "—"
	case n < 1<<20:
		return fmt.Sprintf("%d KB", max(1, n>>10))
	case n < 100<<20:
		return strings.Replace(fmt.Sprintf("%.1f MB", float64(n)/(1<<20)), ".", ",", 1)
	case n < 1<<30:
		return fmt.Sprintf("%d MB", n>>20)
	}
	return strings.Replace(fmt.Sprintf("%.1f GB", float64(n)/(1<<30)), ".", ",", 1)
}
