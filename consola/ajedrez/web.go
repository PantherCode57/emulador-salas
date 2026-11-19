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

// La página para pasar tu cuenta: mientras no hay token (o al cambiarlo), la consola abre
// http://IP:8769 en la red de casa. Desde el PC o el móvil se hace el token en Lichess (el enlace
// ya lleva marcados los permisos que hacen falta) y se pega ahí.
const puertoSesion = 8769

// Los permisos: jugar partidas, retar y ver los retos, los problemas y a quién sigues.
const urlToken = "https://lichess.org/account/oauth/token/create?scopes[]=board:play&scopes[]=challenge:read" +
	"&scopes[]=challenge:write&scopes[]=puzzle:read&scopes[]=puzzle:write&scopes[]=follow:read&description=Panther+OS+consola"

type paginaSesion struct {
	srv     *http.Server
	validar func(texto string) error
}

const plantillaSesion = `<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><title>Ajedrez · consola</title>
<style>
body{font-family:system-ui,Segoe UI,sans-serif;background:#161512;color:#eee;margin:0;padding:24px}
main{max-width:640px;margin:auto}h1{font-size:26px;margin:0 0 6px}p,li{line-height:1.55;color:#c9c6c0}
ol{padding-left:22px}li{margin:6px 0}code{background:#2a2723;padding:1px 6px;border-radius:6px;color:#fff}
a.boton{display:inline-block;background:#629924;color:#fff;text-decoration:none;border-radius:10px;padding:10px 18px;font-weight:600}
textarea{width:100%;box-sizing:border-box;height:90px;border-radius:12px;border:2px solid #3a3733;
background:#1f1d1a;color:#fff;padding:12px;font:14px monospace}
button{margin-top:14px;background:#629924;color:#fff;border:0;border-radius:12px;padding:12px 26px;font-weight:700;font-size:16px;cursor:pointer}
.msg{padding:12px 16px;border-radius:12px;margin:16px 0}.ok{background:#1d3311;color:#cdeab3}.mal{background:#3a1720;color:#ffc2cb}
.nota{font-size:14px;color:#9a968f}</style>
</head><body><main>
<h1>Ajedrez en la consola</h1><p>Para jugar con tus amigos, contra la máquina y hacer problemas desde la
consola hace falta un <b>token</b> de tu cuenta de Lichess. Es como una llave que solo sirve para
eso: se queda guardado solo en la consola y lo puedes borrar en Lichess cuando quieras.</p>
{{AVISO}}
<ol><li>Entra en <b>lichess.org</b> con tu cuenta (si no tienes, hazte una: es gratis).</li>
<li>Pulsa este botón: abre la página para hacer el token, con los permisos ya marcados.<br><br>
<a class="boton" href="{{URL}}" target="_blank" rel="noopener">Hacer el token en Lichess</a></li>
<li>Abajo del todo pulsa <b>Crear</b> (<i>Create</i>). Sale el token una sola vez: empieza por
<code>lip_</code>. Cópialo y pégalo aquí.</li></ol>
<form method="post" action="/"><textarea name="token" placeholder="Pega aquí el token (lip_…)" autofocus></textarea>
<button type="submit">Enviar a la consola</button></form>
<p class="nota">Con el token, la consola solo puede jugar tus partidas, mandar y aceptar retos, ver tus
problemas y a quién sigues. No puede cambiar tu contraseña ni tu correo.</p>
</main></body></html>`

func (p *paginaSesion) abrir() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		aviso := ""
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			if err := p.validar(r.FormValue("token")); err != nil {
				aviso = `<div class="msg mal">No ha valido: ` + html.EscapeString(err.Error()) + `.</div>`
			} else {
				aviso = `<div class="msg ok">¡Hecho! La consola ya tiene tu cuenta de Lichess. Ya puedes cerrar esta página.</div>`
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		pag := strings.Replace(plantillaSesion, "{{AVISO}}", aviso, 1)
		pag = strings.Replace(pag, "{{URL}}", html.EscapeString(urlToken), 1)
		_, _ = w.Write([]byte(pag))
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
