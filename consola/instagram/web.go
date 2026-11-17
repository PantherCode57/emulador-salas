package main

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"strings"
	"time"
)

// Para entrar en tu cuenta sin navegador en la consola: mientras no hay sesión (o en «Cuenta» al
// cambiarla), la consola abre una página en la red de casa (http://IP:8767). Desde el PC se pega ahí
// la cabecera «cookie» de instagram.com y la consola comprueba con Instagram de quién es. La página
// se cierra al salir.
const puertoSesion = 8767

type paginaSesion struct {
	srv     *http.Server
	validar func(texto string) error
}

const plantillaSesion = `<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Instagram · consola</title>
<style>
body{font-family:system-ui,Segoe UI,sans-serif;background:#0b1220;color:#eef;margin:0;padding:24px}
main{max-width:640px;margin:auto}h1{font-size:26px;margin:0 0 6px}p,li{line-height:1.55;color:#c8c8d4}
ol{padding-left:22px}li{margin:4px 0}code{background:#1e293b;padding:1px 6px;border-radius:6px;color:#fff}
textarea{width:100%%;box-sizing:border-box;height:130px;border-radius:12px;border:2px solid #334;
background:#111827;color:#fff;padding:12px;font:13px monospace}
button{margin-top:14px;background:linear-gradient(90deg,#f77737,#e1306c,#833ab4);color:#fff;border:0;border-radius:999px;
padding:12px 26px;font-weight:600;font-size:16px;cursor:pointer}.msg{padding:12px 16px;border-radius:12px;margin:16px 0}
.ok{background:#12351f;color:#b7f5c9}.mal{background:#3a1720;color:#ffc2cb}.nota{font-size:14px;color:#9aa}</style>
</head><body><main>
<h1>Instagram en la consola</h1><p>Para que la consola enseñe tu inicio, tus historias, los reels y tus
mensajes hace falta tu sesión de instagram.com. Se queda guardada solo en la consola y solo se usa para
eso.</p>
%s
<ol><li>En el PC, abre una <b>ventana de incógnito</b> en Chrome o Edge (<b>Ctrl + Mayús + N</b>) y entra en
<b>instagram.com</b> con tu cuenta.</li>
<li>Pulsa <b>F12</b>, ve a la pestaña <b>Red</b> (<i>Network</i>) y pulsa <b>F5</b> para recargar la página.</li>
<li>En la cajita de <b>Filtrar</b> (<i>Filter</i>) de la pestaña Red escribe <code>graphql</code> y pulsa una de
las líneas que salen (suelen llamarse <code>query</code>). Si no sale ninguna, escribe <code>api/v1</code>.
En <b>Encabezados</b> (<i>Headers</i>), baja hasta <b>Encabezados de solicitud</b> y busca <code>cookie</code>.
No vale la primera línea (<code>www.instagram.com</code>): Chrome no enseña su cookie.</li>
<li>Clic derecho sobre su valor → <b>Copiar valor</b> (o selecciónalo entero), y pégalo aquí. Tiene que llevar
<code>sessionid=</code>:</li></ol>
<form method="post" action="/"><textarea name="cookie" placeholder="Pega aquí la cookie (lleva cosas como csrftoken=…; ds_user_id=…; sessionid=…)" autofocus></textarea>
<button type="submit">Enviar a la consola</button></form>
<p class="nota">5. Después cierra la ventana de incógnito <b>sin cerrar sesión</b> (si cierras sesión, la consola también
se sale). También vale el contenido de un <code>cookies.txt</code> de instagram.com.</p></main></body></html>`

func (p *paginaSesion) abrir() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		aviso := ""
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
			texto := r.FormValue("cookie")
			switch {
			case len(strings.TrimSpace(texto)) < 40:
				aviso = `<div class="msg mal">Eso es muy corto: la cookie de Instagram es un texto largo (con sessionid, csrftoken, ds_user_id…).</div>`
			default:
				if err := p.validar(texto); err != nil {
					aviso = `<div class="msg mal">No ha valido: ` + html.EscapeString(err.Error()) + `.</div>`
				} else {
					aviso = `<div class="msg ok">¡Hecho! La consola ya tiene tu cuenta. Ya puedes cerrar esta página y la ventana de incógnito.</div>`
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
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	p.srv = srv
	go func() { _ = srv.Serve(ln) }()
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
