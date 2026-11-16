package main

import (
	"image"
	"image/color"
	"os"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// Lo que el servicio pinta para que se vea fuera de la app de WhatsApp:
//   - el aviso pequeño de arriba cuando te escriben (como en el móvil);
//   - la lista de lo que te han escrito, a la derecha de la pantalla principal.

const (
	anchoAviso = 392
	altoAviso  = 64
	anchoPanel = 228
	altoPanel  = 300 // como mucho (crece con los chats que haya)
	filasPanel = 5
)

// Dónde va cada cosa (en píxeles de 640×480).
var (
	sitioAviso = image.Pt((640-anchoAviso)/2, 10)
	// A la derecha de la hora grande y encima de las tarjetas de la pantalla principal.
	sitioPanel = image.Pt(640-anchoPanel-14, 66)
)

func lienzoDe(f *fuentes, w, h int) *lienzo {
	return &lienzo{img: image.NewRGBA(image.Rect(0, 0, w, h)), f: f}
}

// Letras del panel: Selawik, la del tema de la consola, si está; si no, las de Go.
type carasPanel struct{ nombre, texto, mini font.Face }

var (
	carasUna sync.Once
	caras    carasPanel
)

func letrasPanel() carasPanel {
	carasUna.Do(func() {
		leer := func(rutas []string, defecto []byte) *opentype.Font {
			for _, r := range rutas {
				if b, err := os.ReadFile(r); err == nil {
					if f, err := opentype.Parse(b); err == nil {
						return f
					}
				}
			}
			f, _ := opentype.Parse(defecto)
			return f
		}
		dir := entorno("PANTHER_LETRAS", "/home/ark/panther/emulador")
		neg := leer([]string{dir + "/selawksb.ttf"}, gobold.TTF)
		reg := leer([]string{dir + "/selawk.ttf"}, goregular.TTF)
		cara := func(f *opentype.Font, tam float64) font.Face {
			c, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingFull})
			return c
		}
		caras = carasPanel{nombre: cara(neg, 16), texto: cara(reg, 14), mini: cara(reg, 12)}
	})
	return caras
}

// pintarAvatar: la foto de perfil (redonda) o la inicial en un círculo de color.
func (a *app) pintarAvatar(l *lienzo, c *Chat, x, y, lado int, cara font.Face) {
	if a.av != nil {
		if img := a.av.de(c.JID); img != nil {
			if img.Bounds().Dx() != lado {
				img = ajustarA(img, lado, lado)
			}
			l.pegar(img, x, y)
			return
		}
	}
	ini := "?"
	if rs := []rune(c.Nombre); len(rs) > 0 {
		ini = string(rs[0])
	}
	l.circulo(x+lado/2, y+lado/2, lado/2, colorDe(c.JID))
	l.centrado(image.Rect(x, y, x+lado, y+lado), ini, cara, colBlanco)
}

func textoAviso(c *Chat, m *Mensaje) string {
	if c.Grupo && m.De != "" {
		return m.De + ": " + m.Texto
	}
	return m.Texto
}

// dibujarAviso: el aviso de arriba (foto, nombre y el principio del mensaje).
func (a *app) dibujarAviso(c *Chat, m *Mensaje) *image.RGBA {
	l := lienzoDe(a.f, anchoAviso, altoAviso)
	l.redondeado(l.img.Bounds(), 18, color.NRGBA{0x1f, 0x2c, 0x34, 0xf6})
	a.pintarAvatar(l, c, 11, 11, 42, l.f.negrita)
	nombre := c.Nombre
	if nombre == "" {
		nombre = "WhatsApp"
	}
	pista := "R3 abrir"
	wp := medir(l.f.mini, pista)
	l.izquierda(image.Rect(64, 9, anchoAviso-wp-26, 33), nombre, l.f.negrita, colTexto)
	l.derecha(image.Rect(anchoAviso-wp-16, 9, anchoAviso-14, 33), pista, l.f.mini, colSuave)
	l.izquierda(image.Rect(64, 35, anchoAviso-14, 57), textoAviso(c, m), l.f.pequena, colSuave)
	return l.img
}

// conNoLeidos: los chats con algo sin leer, el más reciente primero.
func (w *wa) conNoLeidos() []*Chat {
	var r []*Chat
	for _, c := range w.ordenados() {
		if c.NoLeidos > 0 && len(c.Mensajes) > 0 {
			r = append(r, c)
		}
	}
	return r
}

// dibujarPanel: lo que te han escrito, para la pantalla principal.
func (a *app) dibujarPanel() *image.RGBA {
	cp := letrasPanel()
	lista := a.w.conNoLeidos()
	filas := min(len(lista), filasPanel)
	alto := 44 + max(filas, 1)*52 + 8
	if len(lista) > filasPanel {
		alto += 22
	}
	alto = min(alto, altoPanel)
	l := lienzoDe(a.f, anchoPanel, alto)
	l.redondeado(l.img.Bounds(), 18, color.NRGBA{0x12, 0x0f, 0x1c, 0xc8})

	// Cabecera: WhatsApp y cuántos mensajes hay sin leer.
	l.circulo(22, 23, 6, colVerde)
	l.izquierda(image.Rect(36, 10, anchoPanel-60, 36), "WhatsApp", cp.nombre, colTexto)
	total := 0
	for _, c := range lista {
		total += c.NoLeidos
	}
	if total > 0 {
		n := itoa(total)
		w := max(24, medir(cp.mini, n)+12)
		b := image.Rect(anchoPanel-14-w, 13, anchoPanel-14, 33)
		l.redondeado(b, 10, colVerde)
		l.centrado(b, n, cp.mini, colFondo)
	}
	l.rect(image.Rect(14, 42, anchoPanel-14, 43), color.NRGBA{0xff, 0xff, 0xff, 0x1c})

	if filas == 0 {
		msg := "No tienes mensajes nuevos"
		switch a.estado {
		case waQR, waQRCaducado, waDesvinculado:
			msg = "Abre WhatsApp para vincularlo"
		case waSinRed:
			msg = "Sin conexión"
		case waArrancando, waConectando:
			msg = "Conectando…"
		}
		l.centrado(image.Rect(10, 46, anchoPanel-10, 46+52), msg, cp.texto, colSuave)
		return l.img
	}
	for i := 0; i < filas; i++ {
		c := lista[i]
		y := 48 + i*52
		a.pintarAvatar(l, c, 12, y+7, 36, cp.nombre)
		finNombre := anchoPanel - 14
		if c.NoLeidos > 0 {
			n := itoa(c.NoLeidos)
			w := max(20, medir(cp.mini, n)+10)
			b := image.Rect(anchoPanel-14-w, y+9, anchoPanel-14, y+27)
			l.redondeado(b, 9, colVerde)
			l.centrado(b, n, cp.mini, colFondo)
			finNombre = b.Min.X - 6
		}
		nombre := c.Nombre
		if nombre == "" {
			nombre = "…"
		}
		l.izquierda(image.Rect(56, y+5, finNombre, y+29), nombre, cp.nombre, colTexto)
		ultimo := c.Mensajes[len(c.Mensajes)-1]
		l.izquierda(image.Rect(56, y+29, anchoPanel-14, y+47), horaCorta(ultimo.Hora)+"  "+textoAviso(c, ultimo), cp.texto, colSuave)
	}
	if len(lista) > filasPanel {
		mas := len(lista) - filasPanel
		s := "y 1 chat más"
		if mas > 1 {
			s = "y " + itoa(mas) + " chats más"
		}
		l.centrado(image.Rect(10, alto-30, anchoPanel-10, alto-8), s, cp.mini, colSuave)
	}
	return l.img
}

// horaCorta: «12:04» si es de hoy; si no, el día.
func horaCorta(t time.Time) string { return hora(t) }
