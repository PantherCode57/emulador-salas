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

// Para entrar en tu cuenta sin navegador en la consola: mientras no hay sesión (o en «Perfil» al
// cambiarla), la consola abre una página en la red de casa (http://IP:8768). Desde el PC o el móvil
// se pega ahí el valor de la cookie jwt_token de duolingo.com, y la consola comprueba con Duolingo de
// quién es. La página se cierra al salir.
const puertoSesion = 8768

type paginaSesion struct {
	srv     *http.Server
	validar func(texto string) error
}

const plantillaSesion = `<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Duolingo · consola</title>
<style>
body{font-family:system-ui,Segoe UI,sans-serif;background:#0b1220;color:#eef;margin:0;padding:24px}
main{max-width:640px;margin:auto}h1{font-size:26px;margin:0 0 6px}p,li{line-height:1.55;color:#c8c8d4}
ol{padding-left:22px}li{margin:4px 0}code{background:#1e293b;padding:1px 6px;border-radius:6px;color:#fff}
textarea{width:100%;box-sizing:border-box;height:130px;border-radius:12px;border:2px solid #334;
background:#111827;color:#fff;padding:12px;font:13px monospace}
button{margin-top:14px;background:#58cc02;color:#fff;border:0;border-radius:14px;box-shadow:0 4px 0 #46a302;
padding:12px 26px;font-weight:700;font-size:16px;cursor:pointer}.msg{padding:12px 16px;border-radius:12px;margin:16px 0}
.ok{background:#12351f;color:#b7f5c9}.mal{background:#3a1720;color:#ffc2cb}.nota{font-size:14px;color:#9aa}
h2{font-size:17px;margin:22px 0 4px}</style>
</head><body><main>
<h1>Duolingo en la consola</h1><p>Para hacer tus lecciones en la consola (y que cuenten para tu racha y
tus XP) hace falta tu sesión de duolingo.com. Se queda guardada solo en la consola y solo se usa para
eso.</p>
{{AVISO}}
<h2>Desde el PC</h2>
<ol><li>Abre <b>duolingo.com</b> en Chrome o Edge y entra con tu cuenta (si ya estás dentro, vale).</li>
<li>Pulsa <b>F12</b> y ve a la pestaña <b>Aplicación</b> (<i>Application</i>). Si no la ves arriba, está
en <b>»</b> (en Edge, en <b>+</b>).</li>
<li>A la izquierda, en <b>Almacenamiento</b> (<i>Storage</i>), abre <b>Cookies</b> y pulsa
<code>https://www.duolingo.com</code>.</li>
<li>En la lista, pulsa la fila <code>jwt_token</code>. Abajo sale su valor (<b>Valor de la cookie</b>):
selecciónalo entero, cópialo con <b>Ctrl + C</b> y pégalo aquí. Empieza por <code>eyJ</code> y es largo.</li></ol>
<form method="post" action="/"><textarea name="token" placeholder="Pega aquí el valor de jwt_token (empieza por eyJ…)" autofocus></textarea>
<button type="submit">Enviar a la consola</button></form>
<h2>Desde el móvil</h2>
<p class="nota">En Firefox para Android, con la extensión <b>Cookie-Editor</b>: entra en duolingo.com, abre
Cookie-Editor, toca <code>jwt_token</code>, copia su valor y pégalo arriba. También vale la cabecera
<code>cookie</code> entera o un <code>cookies.txt</code>: la consola busca dentro el jwt_token.</p>
<p class="nota">No cierres sesión en ese navegador después: si la cierras, la consola también se sale.</p>
</main></body></html>`

func (p *paginaSesion) abrir() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		aviso := ""
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
			texto := r.FormValue("token")
			switch {
			case len(strings.TrimSpace(texto)) < 40:
				aviso = `<div class="msg mal">Eso es muy corto: el valor de jwt_token es largo (empieza por <code>eyJ</code> y tiene dos puntos por medio). Cópialo entero.</div>`
			default:
				if err := p.validar(texto); err != nil {
					aviso = `<div class="msg mal">No ha valido: ` + html.EscapeString(err.Error()) + `.</div>`
				} else {
					aviso = `<div class="msg ok">¡Hecho! La consola ya tiene tu cuenta. Ya puedes cerrar esta página.</div>`
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(strings.Replace(plantillaSesion, "{{AVISO}}", aviso, 1)))
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

// miIP: la dirección de la consola en la red (la primera que no es la de dentro).
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
