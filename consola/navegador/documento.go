package main

import (
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// Una página, como la ve la consola: solo lo que se lee (títulos, párrafos, listas, citas e
// imágenes) con sus enlaces. Fuera menús, anuncios, avisos de cookies, formularios y JavaScript.
type tipoBloque int

const (
	bParrafo tipoBloque = iota
	bTitulo1
	bTitulo2
	bTitulo3
	bItem
	bCita
	bCodigo
	bImagen
)

type trozo struct {
	texto   string
	enlace  int // -1: no es un enlace
	negrita bool
}

type bloque struct {
	tipo   tipoBloque
	trozos []trozo
	img    string // la dirección de la imagen
	alt    string
	ancho  int // lo que dice la página de la imagen (0 si no lo dice)
	alto   int
	nivel  int // sangría de las listas
}

type pagina struct {
	url     string
	titulo  string
	bloques []bloque
	enlaces []string
}

const (
	maxBloques = 3000
	maxLetras  = 400_000
)

// Lo que no se lee.
var etiquetasFuera = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "svg": true, "canvas": true,
	"iframe": true, "form": true, "nav": true, "footer": true, "aside": true, "button": true,
	"input": true, "select": true, "textarea": true, "label": true, "dialog": true, "menu": true,
	"object": true, "embed": true, "video": true, "audio": true, "map": true, "head": true,
}

// Clases e id que suelen ser menús, anuncios y demás (por palabras: «site-nav», «cookie_banner»…).
var palabrasFuera = map[string]bool{
	"nav": true, "navbar": true, "navigation": true, "menu": true, "footer": true, "sidebar": true,
	"cookie": true, "cookies": true, "consent": true, "gdpr": true, "banner": true, "advert": true,
	"advertisement": true, "ad": true, "ads": true, "adsbygoogle": true, "social": true, "share": true,
	"sharing": true, "related": true, "comments": true, "breadcrumb": true, "breadcrumbs": true,
	"subscribe": true, "newsletter": true, "popup": true, "modal": true, "promo": true, "sponsored": true,
	"skip": true, "toolbar": true, "masthead": true, "mw-jump-link": true, "noprint": true,
}

func fueraPorClase(n *html.Node) bool {
	if atributo(n, "aria-hidden") == "true" || atributo(n, "hidden") != "" || atributo(n, "role") == "navigation" {
		return true
	}
	if st := strings.ReplaceAll(strings.ToLower(atributo(n, "style")), " ", ""); strings.Contains(st, "display:none") {
		return true
	}
	for _, v := range []string{atributo(n, "class"), atributo(n, "id")} {
		for _, p := range strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == '-' || r == '_' || unicode.IsSpace(r) }) {
			if palabrasFuera[p] {
				return true
			}
		}
	}
	return false
}

type lector struct {
	p       *pagina
	base    *url.URL
	actual  *bloque
	enlace  int
	negrita int
	lista   int // profundidad de listas
	pre     int
	letras  int
	enlaces map[string]int
}

// leerDocumento saca la página que se lee de un documento HTML.
func leerDocumento(doc *html.Node, base *url.URL) *pagina {
	p := &pagina{url: base.String()}
	var titulo, cuerpo, articulo, principal *html.Node
	recorrer(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch n.Data {
		case "title":
			if titulo == nil {
				titulo = n
			}
		case "base":
			if h := atributo(n, "href"); h != "" {
				if u, err := base.Parse(h); err == nil {
					base = u
				}
			}
		case "body":
			cuerpo = n
		case "article":
			if articulo == nil && len(textoNodo(n)) > 400 {
				articulo = n
			}
		case "main":
			if principal == nil {
				principal = n
			}
		}
		return true
	})
	if titulo != nil {
		p.titulo = textoNodo(titulo)
	}
	raiz := cuerpo
	switch {
	case principal != nil:
		raiz = principal
	case articulo != nil:
		raiz = articulo
	}
	if raiz == nil {
		raiz = doc
	}
	l := &lector{p: p, base: base, enlace: -1, enlaces: map[string]int{}}
	l.leer(raiz)
	l.cerrar()
	// Sin casi nada en <main>: toda la página.
	if raiz != cuerpo && cuerpo != nil && l.letras < 300 {
		p.bloques, p.enlaces = nil, nil
		l = &lector{p: p, base: base, enlace: -1, enlaces: map[string]int{}}
		l.leer(cuerpo)
		l.cerrar()
	}
	if p.titulo == "" {
		p.titulo = dominio(p.url)
	}
	return p
}

func (l *lector) nuevo(t tipoBloque) {
	l.cerrar()
	l.actual = &bloque{tipo: t, nivel: l.lista}
}

// cerrar acaba el bloque de ahora (si tiene algo).
func (l *lector) cerrar() {
	b := l.actual
	l.actual = nil
	if b == nil || len(l.p.bloques) >= maxBloques {
		return
	}
	// Fuera los espacios de los bordes.
	for len(b.trozos) > 0 && strings.TrimSpace(b.trozos[0].texto) == "" {
		b.trozos = b.trozos[1:]
	}
	for len(b.trozos) > 0 && strings.TrimSpace(b.trozos[len(b.trozos)-1].texto) == "" {
		b.trozos = b.trozos[:len(b.trozos)-1]
	}
	if len(b.trozos) == 0 {
		return
	}
	b.trozos[0].texto = strings.TrimLeft(b.trozos[0].texto, " ")
	l.p.bloques = append(l.p.bloques, *b)
}

func (l *lector) texto(s string) {
	if l.letras > maxLetras {
		return
	}
	if l.pre == 0 && s != "\n" {
		// Los espacios de los bordes cuentan: separan palabras de nodos distintos («hola <a>mundo</a>»).
		r := []rune(s)
		inicio, fin := len(r) > 0 && unicode.IsSpace(r[0]), len(r) > 0 && unicode.IsSpace(r[len(r)-1])
		m := strings.Join(strings.Fields(s), " ")
		switch {
		case m == "":
			m = " "
		default:
			if inicio {
				m = " " + m
			}
			if fin {
				m += " "
			}
		}
		s = m
	}
	if l.actual == nil {
		if strings.TrimSpace(s) == "" {
			return
		}
		l.actual = &bloque{tipo: bParrafo, nivel: l.lista}
	}
	l.letras += len(s)
	t := trozo{texto: s, enlace: l.enlace, negrita: l.negrita > 0}
	if n := len(l.actual.trozos); n > 0 {
		u := &l.actual.trozos[n-1]
		if u.enlace == t.enlace && u.negrita == t.negrita {
			u.texto = juntarTexto(u.texto, s)
			return
		}
	}
	l.actual.trozos = append(l.actual.trozos, t)
}

func juntarTexto(a, b string) string {
	if strings.HasSuffix(a, " ") && strings.HasPrefix(b, " ") {
		return a + strings.TrimLeft(b, " ")
	}
	return a + b
}

var bloquesHTML = map[string]tipoBloque{
	"p": bParrafo, "div": bParrafo, "section": bParrafo, "article": bParrafo, "header": bParrafo,
	"figcaption": bParrafo, "dt": bParrafo, "dd": bParrafo, "tr": bParrafo, "caption": bParrafo,
	"h1": bTitulo1, "h2": bTitulo2, "h3": bTitulo3, "h4": bTitulo3, "h5": bTitulo3, "h6": bTitulo3,
	"li": bItem, "blockquote": bCita, "pre": bCodigo,
}

func (l *lector) leer(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		l.texto(n.Data)
		return
	case html.ElementNode:
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			l.leer(c)
		}
		return
	}
	if etiquetasFuera[n.Data] || fueraPorClase(n) {
		return
	}
	switch n.Data {
	case "br":
		l.texto("\n")
		return
	case "hr":
		l.cerrar()
		return
	case "img":
		l.imagen(n)
		return
	case "picture":
		// La primera <img> de dentro.
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "img" {
				l.imagen(c)
				return
			}
		}
		return
	case "td", "th":
		if l.actual != nil && len(l.actual.trozos) > 0 {
			l.texto("  ·  ")
		}
	}
	tipo, esBloque := bloquesHTML[n.Data]
	if esBloque {
		// En un <li> con <p> dentro, el párrafo sigue siendo el punto de la lista.
		if !(tipo == bParrafo && l.actual != nil && l.actual.tipo == bItem && len(l.actual.trozos) == 0) {
			l.nuevo(tipo)
		}
	}
	switch n.Data {
	case "ul", "ol":
		l.cerrar()
		l.lista++
		defer func() { l.cerrar(); l.lista-- }()
	case "pre":
		l.pre++
		defer func() { l.pre-- }()
	case "b", "strong":
		l.negrita++
		defer func() { l.negrita-- }()
	case "a":
		if i := l.indiceEnlace(atributo(n, "href")); i >= 0 {
			antes := l.enlace
			l.enlace = i
			defer func() { l.enlace = antes }()
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		l.leer(c)
	}
	if esBloque {
		l.cerrar()
	}
}

func (l *lector) indiceEnlace(href string) int {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "mailto:") ||
		strings.HasPrefix(href, "tel:") {
		return -1
	}
	u, err := l.base.Parse(href)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return -1
	}
	u.Fragment = ""
	s := u.String()
	if i, ok := l.enlaces[s]; ok {
		return i
	}
	l.p.enlaces = append(l.p.enlaces, s)
	l.enlaces[s] = len(l.p.enlaces) - 1
	return len(l.p.enlaces) - 1
}

func (l *lector) imagen(n *html.Node) {
	src := ""
	for _, k := range []string{"data-src", "data-lazy-src", "data-original", "src"} {
		if v := strings.TrimSpace(atributo(n, k)); v != "" && !strings.HasPrefix(v, "data:") {
			src = v
			break
		}
	}
	if src == "" {
		if ss := atributo(n, "srcset"); ss != "" {
			src = strings.Fields(strings.Split(ss, ",")[0] + " ")[0]
		}
	}
	if src == "" || strings.HasSuffix(strings.ToLower(strings.Split(src, "?")[0]), ".svg") {
		return
	}
	w, _ := strconv.Atoi(strings.TrimSuffix(atributo(n, "width"), "px"))
	h, _ := strconv.Atoi(strings.TrimSuffix(atributo(n, "height"), "px"))
	if (w > 0 && w < 60) || (h > 0 && h < 60) { // iconos
		return
	}
	u, err := l.base.Parse(src)
	if err != nil {
		return
	}
	l.cerrar()
	if len(l.p.bloques) < maxBloques {
		l.p.bloques = append(l.p.bloques, bloque{tipo: bImagen, img: u.String(), alt: strings.TrimSpace(atributo(n, "alt")),
			ancho: w, alto: h, nivel: l.lista})
	}
}
