package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg" // fotos y miniaturas
	_ "image/png"
	"log"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // stickers
	"google.golang.org/protobuf/proto"
)

// Fotos y stickers que llegan: solo se ven, no se pueden enviar, y NO se guardan en la consola.
//
// En <datos>/media solo se guarda, de cada uno, lo necesario para pedirlo (el mensaje de imagen o
// de sticker sin sus miniaturas: <id>.pb). Nada se descarga solo: la imagen se baja cuando la
// abres con Y, se queda en la memoria mientras el programa está abierto y se olvida al salir.
// Las miniaturas que traen los mensajes que llegan en directo también se quedan solo en memoria.

const (
	tipoFoto    = "foto"
	tipoSticker = "sticker"
)

// medioDe dice si el mensaje es una foto o un sticker, con su pie, la parte que se descarga (sin
// miniaturas) y la miniatura aparte.
func medioDe(m *waE2E.Message) (tipo, pie string, sub proto.Message, mini []byte) {
	if im := m.GetImageMessage(); im != nil {
		c := proto.Clone(im).(*waE2E.ImageMessage)
		mini = c.GetJPEGThumbnail()
		c.JPEGThumbnail = nil
		return tipoFoto, im.GetCaption(), c, mini
	}
	if st := m.GetStickerMessage(); st != nil {
		c := proto.Clone(st).(*waE2E.StickerMessage)
		mini = c.GetPngThumbnail()
		c.PngThumbnail = nil
		return tipoSticker, "", c, mini
	}
	return "", "", nil, nil
}

type imagenes struct {
	vista, completa image.Image
	mini            bool // solo está la miniatura: falta la buena (se pide al abrirla)
}

type medios struct {
	a        *app
	dir      string
	enCurso  map[string]bool
	fallidos map[string]bool
	minis    map[string][]byte    // miniaturas de los mensajes de esta sesión (solo en memoria)
	cache    map[string]*imagenes // lo ya descargado o decodificado (solo en memoria)
	orden    []string             // para olvidar lo más antiguo si hay demasiado
	turno    chan struct{}        // como mucho dos descargas a la vez
}

func nuevosMedios(a *app, dir string) *medios {
	_ = os.MkdirAll(dir, 0o700)
	return &medios{a: a, dir: dir, enCurso: map[string]bool{}, fallidos: map[string]bool{},
		minis: map[string][]byte{}, cache: map[string]*imagenes{}, turno: make(chan struct{}, 2)}
}

func nombreSeguro(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

func (md *medios) rutaPB(m *Mensaje) string {
	return filepath.Join(md.dir, nombreSeguro(m.ID)+".pb")
}

// guardar apunta lo necesario para pedirlo más tarde; la miniatura, solo en memoria.
func (md *medios) guardar(m *Mensaje, sub proto.Message, mini []byte) {
	if b, err := proto.Marshal(sub); err == nil {
		escribirAtomico(md.rutaPB(m), b)
	}
	if len(mini) > 0 {
		md.minis[m.ID] = mini
	}
}

// Tamaños máximos: en el visor (pantalla entera menos la barra) y dentro del chat.
func tamCompleta() (int, int) { return ancho, alto - 64 }

func tamVista(m *Mensaje) (int, int) {
	if m.Tipo == tipoSticker {
		return 130, 130
	}
	return 260, 190
}

func ajustarA(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return img
	}
	w, h := maxW, maxW*b.Dy()/b.Dx()
	if h > maxH {
		w, h = maxH*b.Dx()/b.Dy(), maxH
	}
	if w >= b.Dx() && h >= b.Dy() { // no se agranda
		w, h = b.Dx(), b.Dy()
	}
	out := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, b, draw.Src, nil)
	return out
}

// agrandar: las miniaturas son pequeñas; se estiran hasta el tamaño del chat (algo borrosas).
func agrandar(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	esc := min(float64(maxW)/float64(b.Dx()), float64(maxH)/float64(b.Dy()))
	out := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*esc)), max(1, int(float64(b.Dy())*esc))))
	xdraw.ApproxBiLinear.Scale(out, out.Bounds(), img, b, draw.Src, nil)
	return out
}

func (md *medios) recordar(id string, c *imagenes) {
	if _, ya := md.cache[id]; !ya {
		md.orden = append(md.orden, id)
	}
	md.cache[id] = c
	for len(md.orden) > 30 { // unos 30 MB como mucho
		delete(md.cache, md.orden[0])
		md.orden = md.orden[1:]
	}
}

// imagenes devuelve lo que haya en memoria de este mensaje: la imagen ya abierta o la miniatura
// de esta sesión. Con 'pedir' (al abrirla en el visor), si falta la buena, se descarga.
func (md *medios) imagenes(m *Mensaje, pedir bool) (img *imagenes, cargando, falta bool) {
	c := md.cache[m.ID]
	if c == nil {
		c = &imagenes{mini: true}
		if b := md.minis[m.ID]; len(b) > 0 {
			if mini, _, err := image.Decode(bytes.NewReader(b)); err == nil {
				w, h := tamVista(m)
				g := agrandar(mini, w, h)
				c.vista, c.completa = g, g
			}
		}
		md.recordar(m.ID, c)
	}
	if c.mini && pedir {
		md.pedir(m)
	}
	if c.vista == nil {
		return nil, md.enCurso[m.ID], md.fallidos[m.ID]
	}
	return c, md.enCurso[m.ID], md.fallidos[m.ID]
}

// pedir descarga la foto o el sticker a la memoria (no a la tarjeta) y avisa a la pantalla.
func (md *medios) pedir(m *Mensaje) {
	if md.enCurso[m.ID] || md.fallidos[m.ID] || md.a.w == nil || md.a.w.cli == nil {
		return
	}
	b, err := os.ReadFile(md.rutaPB(m))
	if err != nil {
		md.fallidos[m.ID] = true
		return
	}
	md.enCurso[m.ID] = true
	cli, copia := md.a.w.cli, *m
	go func() {
		md.turno <- struct{}{}
		defer func() { <-md.turno }()
		completa, err := descargar(cli, &copia, b)
		md.a.hacer(func() {
			delete(md.enCurso, copia.ID)
			if err != nil {
				log.Printf("descargar %s %s: %v", copia.Tipo, copia.ID, err)
				md.fallidos[copia.ID] = true
				return
			}
			w, h := tamVista(&copia)
			md.recordar(copia.ID, &imagenes{completa: completa, vista: ajustarA(completa, w, h)})
		})
	}()
}

func descargar(cli *whatsmeow.Client, m *Mensaje, pb []byte) (image.Image, error) {
	var dm whatsmeow.DownloadableMessage
	if m.Tipo == tipoSticker {
		st := &waE2E.StickerMessage{}
		if err := proto.Unmarshal(pb, st); err != nil {
			return nil, err
		}
		dm = st
	} else {
		im := &waE2E.ImageMessage{}
		if err := proto.Unmarshal(pb, im); err != nil {
			return nil, err
		}
		dm = im
	}
	datos, err := cli.Download(context.Background(), dm)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return nil, err // p. ej. stickers animados: se queda la miniatura
	}
	w, h := tamCompleta()
	return ajustarA(img, w, h), nil
}

// pegar dibuja una imagen con su transparencia en el lienzo.
func (l *lienzo) pegar(img image.Image, x, y int) {
	b := img.Bounds()
	draw.Draw(l.img, image.Rect(x, y, x+b.Dx(), y+b.Dy()), img, b.Min, draw.Over)
}

// huecoFoto: el hueco de una foto que aún no se ha abierto.
func (l *lienzo) huecoFoto(r image.Rectangle, texto string) {
	l.redondeado(r, 8, color.RGBA{0x11, 0x1b, 0x21, 0xff})
	l.centrado(r, texto, l.f.mini, colSuave)
}
