package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// Internet: las páginas se piden como un móvil (así muchas webs mandan su versión ligera) y se
// guardan las cookies mientras la app está abierta (para las webs que redirigen con ellas).
const agenteMovil = "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36"

var clienteWeb = func() *http.Client {
	tarro, _ := cookiejar.New(nil)
	return &http.Client{Timeout: 25 * time.Second, Jar: tarro}
}()

func pedirWeb(dir string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, dir, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agenteMovil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.6")
	return clienteWeb.Do(req)
}

// abrirPagina baja la página y la convierte en algo que se pueda leer en la consola.
func abrirPagina(dir string) (*pagina, error) {
	resp, err := pedirWeb(dir)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("la web responde %d", resp.StatusCode)
	}
	tipo := resp.Header.Get("Content-Type")
	final := resp.Request.URL
	switch {
	case strings.HasPrefix(tipo, "image/"):
		return &pagina{url: final.String(), titulo: ultimo(final.Path, "/"),
			bloques: []bloque{{tipo: bImagen, img: final.String()}}}, nil
	case tipo != "" && !strings.Contains(tipo, "html") && !strings.Contains(tipo, "xml") && !strings.HasPrefix(tipo, "text/"):
		return nil, fmt.Errorf("esto no es una página (%s)", strings.Split(tipo, ";")[0])
	}
	cuerpo := io.LimitReader(resp.Body, 4<<20)
	if strings.HasPrefix(tipo, "text/plain") {
		b, _ := io.ReadAll(cuerpo)
		p := &pagina{url: final.String(), titulo: ultimo(final.Path, "/")}
		for _, l := range strings.Split(string(b), "\n") {
			p.bloques = append(p.bloques, bloque{tipo: bParrafo, trozos: []trozo{{texto: l, enlace: -1}}})
		}
		return p, nil
	}
	r, err := charset.NewReader(cuerpo, tipo)
	if err != nil {
		r = cuerpo
	}
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	return leerDocumento(doc, final), nil
}

func ultimo(s, sep string) string {
	s = strings.TrimSuffix(s, sep)
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[i+len(sep):]
	}
	return s
}

// ---- Buscar ---------------------------------------------------------------------------------------

type resultado struct {
	Titulo, URL, Resumen string
}

// buscarWeb: Bing; si no da nada, DuckDuckGo (su versión sin JavaScript, que a veces pide
// demostrar que no eres un robot y entonces no sirve).
func buscarWeb(q string) ([]resultado, error) {
	res, err := buscarBing(q)
	if len(res) > 0 {
		return res, nil
	}
	logf("Bing: %d resultados (%v); pruebo con DuckDuckGo", len(res), err)
	res2, err2 := buscarDDG(q)
	if len(res2) > 0 {
		return res2, nil
	}
	if err == nil {
		err = err2
	}
	if err == nil {
		err = errors.New("no ha salido nada")
	}
	return nil, err
}

func documentoDe(dir string) (*html.Node, *url.URL, error) {
	resp, err := pedirWeb(dir)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil, fmt.Errorf("responde %d", resp.StatusCode)
	}
	r, err := charset.NewReader(io.LimitReader(resp.Body, 4<<20), resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, nil, err
	}
	doc, err := html.Parse(r)
	return doc, resp.Request.URL, err
}

func buscarDDG(q string) ([]resultado, error) {
	doc, base, err := documentoDe("https://html.duckduckgo.com/html/?kl=es-es&q=" + url.QueryEscape(q))
	if err != nil {
		return nil, err
	}
	var res []resultado
	recorrer(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "div" || !tieneClase(n, "result") || tieneClase(n, "result--ad") {
			return true
		}
		var r resultado
		recorrer(n, func(m *html.Node) bool {
			if m.Type == html.ElementNode {
				switch {
				case m.Data == "a" && tieneClase(m, "result__a") && r.URL == "":
					r.Titulo, r.URL = textoNodo(m), enlaceDDG(atributo(m, "href"), base)
				case tieneClase(m, "result__snippet") && r.Resumen == "":
					r.Resumen = textoNodo(m)
				}
			}
			return true
		})
		if r.URL != "" && r.Titulo != "" {
			res = append(res, r)
		}
		return false
	})
	return res, nil
}

// enlaceDDG: DuckDuckGo pasa los enlaces por su redirección (/l/?uddg=…): se saca el de verdad.
func enlaceDDG(href string, base *url.URL) string {
	u, err := base.Parse(href)
	if err != nil {
		return ""
	}
	if strings.HasSuffix(u.Host, "duckduckgo.com") && strings.HasPrefix(u.Path, "/l/") {
		if d := u.Query().Get("uddg"); d != "" {
			return d
		}
	}
	return u.String()
}

func buscarBing(q string) ([]resultado, error) {
	doc, base, err := documentoDe("https://www.bing.com/search?setlang=es&cc=ES&q=" + url.QueryEscape(q))
	if err != nil {
		return nil, err
	}
	var res []resultado
	recorrer(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !tieneClase(n, "b_algo") {
			return true
		}
		var r resultado
		recorrer(n, func(m *html.Node) bool {
			if m.Type != html.ElementNode {
				return true
			}
			switch {
			// Ahora el enlace envuelve al título (<a><h2>…</h2></a>); antes iba dentro (<h2><a>).
			case m.Data == "a" && r.URL == "" && contiene(m, "h2"):
				r.URL, r.Titulo = enlaceBing(atributo(m, "href"), base), textoNodo(m)
				return false
			case m.Data == "h2" && r.URL == "":
				recorrer(m, func(a *html.Node) bool {
					if a.Type == html.ElementNode && a.Data == "a" && r.URL == "" {
						r.URL, r.Titulo = enlaceBing(atributo(a, "href"), base), textoNodo(a)
					}
					return true
				})
				return false
			case tieneClase(m, "b_caption"):
				r.Resumen = textoNodo(m)
				return false
			case m.Data == "p" && r.Resumen == "":
				r.Resumen = textoNodo(m)
			}
			return true
		})
		if r.URL != "" && r.Titulo != "" {
			res = append(res, r)
		}
		return false
	})
	return res, nil
}

// enlaceBing: a veces Bing pasa los enlaces por su redirección (/ck/a?…&u=a1<base64>): se saca el
// de verdad.
func enlaceBing(href string, base *url.URL) string {
	u, err := base.Parse(href)
	if err != nil {
		return ""
	}
	if strings.HasSuffix(u.Host, "bing.com") && strings.HasPrefix(u.Path, "/ck/") {
		if d := u.Query().Get("u"); strings.HasPrefix(d, "a1") {
			if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(d[2:], "=")); err == nil {
				if v, err := url.Parse(string(b)); err == nil && v.Host != "" {
					return v.String()
				}
			}
		}
	}
	return u.String()
}

// ---- Ayudas para el HTML ------------------------------------------------------------------------------

// contiene: si dentro de n hay alguna etiqueta así.
func contiene(n *html.Node, etiqueta string) bool {
	hay := false
	recorrer(n, func(m *html.Node) bool {
		if m != n && m.Type == html.ElementNode && m.Data == etiqueta {
			hay = true
		}
		return !hay
	})
	return hay
}

// recorrer visita los nodos en orden; si fn devuelve false, no baja por ese.
func recorrer(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		recorrer(c, fn)
	}
}

func atributo(n *html.Node, nombre string) string {
	for _, a := range n.Attr {
		if a.Key == nombre {
			return a.Val
		}
	}
	return ""
}

func tieneClase(n *html.Node, c string) bool {
	for _, x := range strings.Fields(atributo(n, "class")) {
		if x == c {
			return true
		}
	}
	return false
}

func textoNodo(n *html.Node) string {
	var b strings.Builder
	recorrer(n, func(m *html.Node) bool {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
			b.WriteByte(' ')
		}
		return m.Type != html.ElementNode || (m.Data != "script" && m.Data != "style")
	})
	return strings.Join(strings.Fields(b.String()), " ")
}

// pareceDireccion: lo escrito es una dirección («wikipedia.org», «https://…») y no una búsqueda.
func pareceDireccion(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " \t") {
		return "", false
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s, true
	}
	host := strings.SplitN(s, "/", 2)[0]
	if i := strings.LastIndex(host, "."); i > 0 && i < len(host)-2 && !strings.HasSuffix(host, ".") {
		return "https://" + s, true
	}
	return "", false
}

func dominio(dir string) string {
	u, err := url.Parse(dir)
	if err != nil {
		return dir
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}
