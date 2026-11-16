package main

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// Para entrar en Spotify sin navegador en la consola: mientras no hay sesión, la consola abre una
// página en la red de casa (http://IP:8765). Desde el PC se pega ahí la cookie «sp_dc» de
// open.spotify.com y la consola comprueba que vale. La página se cierra en cuanto hay sesión.
const puertoSesion = 8765

type paginaSesion struct {
	srv     *http.Server
	validar func(spdc string) error // comprueba la cookie y la guarda (en el hilo que haga falta)
}

var reSpDc = regexp.MustCompile(`sp_dc=([A-Za-z0-9_\-]+)`)

// limpiarCookie acepta el valor solo o la cabecera Cookie entera.
func limpiarCookie(s string) string {
	s = strings.TrimSpace(s)
	if m := reSpDc.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return strings.Trim(s, "\"' ;")
}

const plantillaSesion = `<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Spotin · consola</title>
<style>
body{font-family:system-ui,Segoe UI,sans-serif;background:#0b1220;color:#eef;margin:0;padding:24px}
main{max-width:620px;margin:auto}h1{font-size:26px;margin:0 0 6px}p,li{line-height:1.5;color:#c8c8d4}
ol{padding-left:22px}code{background:#1e293b;padding:1px 6px;border-radius:6px;color:#fff}
textarea{width:100%%;box-sizing:border-box;height:110px;border-radius:12px;border:2px solid #334;
background:#111827;color:#fff;padding:12px;font:14px monospace}
button{margin-top:14px;background:#1ed760;color:#06140b;border:0;border-radius:999px;padding:12px 26px;
font-weight:600;font-size:16px;cursor:pointer}.msg{padding:12px 16px;border-radius:12px;margin:16px 0}
.ok{background:#12351f;color:#b7f5c9}.mal{background:#3a1720;color:#ffc2cb}</style></head><body><main>
<h1>Spotin en la consola</h1><p>Para que la consola entre en tu cuenta de Spotify hace falta la cookie
<code>sp_dc</code> de tu sesión. Se queda guardada solo en la consola.</p>
%s
<ol><li>En el PC, abre <b>open.spotify.com</b> en Chrome o Edge y entra en tu cuenta.</li>
<li>Pulsa <b>F12</b> y ve a la pestaña <b>Aplicación</b> (<i>Application</i>).</li>
<li>A la izquierda: <b>Cookies</b> → <b>https://open.spotify.com</b>.</li>
<li>Busca <code>sp_dc</code>, haz doble clic en su valor, cópialo y pégalo aquí:</li></ol>
<form method="post" action="/"><textarea name="spdc" placeholder="Pega aquí el valor de sp_dc" autofocus></textarea>
<button type="submit">Enviar a la consola</button></form></main></body></html>`

func (p *paginaSesion) abrir() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		aviso := ""
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
			valor := limpiarCookie(r.FormValue("spdc"))
			switch {
			case len(valor) < 40:
				aviso = `<div class="msg mal">Eso no parece la cookie <code>sp_dc</code> (es un texto largo, de más de 100 letras).</div>`
			default:
				if err := p.validar(valor); err != nil {
					aviso = `<div class="msg mal">Spotify no la ha aceptado: ` + html.EscapeString(err.Error()) +
						`. Comprueba que has copiado el valor entero.</div>`
				} else {
					aviso = `<div class="msg ok">¡Hecho! La consola ya tiene tu sesión. Puedes cerrar esta página.</div>`
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprintf(w, plantillaSesion, aviso)
	})
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", puertoSesion))
	if err != nil {
		return err
	}
	p.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	return nil
}

func (p *paginaSesion) cerrar() {
	if p == nil || p.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.srv.Shutdown(ctx)
	p.srv = nil
}

// miIP: la dirección de la consola en la red de casa (la primera que no es la de dentro).
func miIP() string {
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		dirs, _ := i.Addrs()
		for _, d := range dirs {
			if ip, _, err := net.ParseCIDR(d.String()); err == nil && ip.To4() != nil {
				return ip.String()
			}
		}
	}
	return ""
}

// ---- ¿Hay internet? (se mira cada poco; las pantallas lo usan para avisar)

var redOK atomic.Bool

func init() { redOK.Store(true) }

func hayRed() bool { return redOK.Load() }

func vigilarRed(a *app) {
	for {
		ok := false
		d := net.Dialer{Timeout: 4 * time.Second}
		if c, err := d.Dial("tcp", "open.spotify.com:443"); err == nil {
			c.Close()
			ok = true
		}
		if redOK.Swap(ok) != ok {
			a.hacer(func() {}) // que se vea el aviso de «sin internet» (o que se quite)
		}
		if ok {
			time.Sleep(30 * time.Second)
		} else {
			time.Sleep(8 * time.Second)
		}
	}
}
