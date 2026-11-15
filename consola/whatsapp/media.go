package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
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

// Fotos y stickers que llegan: solo se ven, no se pueden enviar.
//
// De cada uno se guarda en <datos>/media:
//   <id>.pb       lo necesario para descargarlo (el mensaje de imagen o de sticker)
//   <id>-mini.jpg la miniatura que viene dentro del mensaje (por si no se puede descargar)
//   <id>.jpg/png  la imagen ya descargada, ajustada a la pantalla (png en stickers, por la transparencia)
// Las de mensajes en directo se descargan solas; las del historial, cuando se ven en un chat.

const (
	tipoFoto    = "foto"
	tipoSticker = "sticker"
)

// medioDe dice si el mensaje es una foto o un sticker, con su pie y la parte que se descarga.
func medioDe(m *waE2E.Message) (tipo, pie string, sub proto.Message, mini []byte) {
	if im := m.GetImageMessage(); im != nil {
		return tipoFoto, im.GetCaption(), im, im.GetJPEGThumbnail()
	}
	if st := m.GetStickerMessage(); st != nil {
		return tipoSticker, "", st, st.GetPngThumbnail()
	}
	return "", "", nil, nil
}

type imagenes struct {
	vista, completa image.Image
	mini            bool // solo está la miniatura (o nada): falta descargar la buena
}

type medios struct {
	a        *app
	dir      string
	enCurso  map[string]bool
	fallidos map[string]bool
	cache    map[string]*imagenes
	turno    chan struct{} // como mucho dos descargas a la vez
}

func nuevosMedios(a *app, dir string) *medios {
	_ = os.MkdirAll(dir, 0o700)
	return &medios{a: a, dir: dir, enCurso: map[string]bool{}, fallidos: map[string]bool{},
		cache: map[string]*imagenes{}, turno: make(chan struct{}, 2)}
}

func nombreSeguro(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, id)
}

func (md *medios) ruta(m *Mensaje, sufijo string) string {
	return filepath.Join(md.dir, nombreSeguro(m.ID)+sufijo)
}

func (md *medios) rutaImagen(m *Mensaje) string {
	if m.Tipo == tipoSticker {
		return md.ruta(m, ".png")
	}
	return md.ruta(m, ".jpg")
}

// guardar apunta lo necesario para descargarlo más tarde.
func (md *medios) guardar(m *Mensaje, sub proto.Message, mini []byte) {
	if b, err := proto.Marshal(sub); err == nil {
		escribirAtomico(md.ruta(m, ".pb"), b)
	}
	if len(mini) > 0 {
		escribirAtomico(md.ruta(m, "-mini.jpg"), mini)
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
	if w >= b.Dx() && h >= b.Dy() { // no se agranda (salvo las miniaturas, que se piden aparte)
		w, h = b.Dx(), b.Dy()
	}
	out := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, b, draw.Src, nil)
	return out
}

func leerImagen(ruta string) image.Image {
	f, err := os.Open(ruta)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

// imagenes devuelve lo mejor que haya de este mensaje: la imagen descargada o, mientras tanto (o si
// no se puede descargar), la miniatura que venía dentro. Se guarda en memoria para no leer el disco
// en cada fotograma. Con 'pedir', si falta la buena, se descarga. 'cargando' es que se está
// descargando; 'falta', que no se ha podido.
func (md *medios) imagenes(m *Mensaje, pedir bool) (img *imagenes, cargando, falta bool) {
	c := md.cache[m.ID]
	if c == nil {
		c = md.cargar(m)
		if len(md.cache) > 60 {
			md.cache = map[string]*imagenes{}
		}
		md.cache[m.ID] = c
	}
	if c.mini && pedir {
		md.pedir(m)
	}
	if c.vista == nil {
		return nil, md.enCurso[m.ID], md.fallidos[m.ID]
	}
	return c, md.enCurso[m.ID], md.fallidos[m.ID]
}

func (md *medios) esMini(m *Mensaje) bool {
	c := md.cache[m.ID]
	return c == nil || c.mini
}

func (md *medios) cargar(m *Mensaje) *imagenes {
	w, h := tamVista(m)
	if full := leerImagen(md.rutaImagen(m)); full != nil {
		return &imagenes{completa: full, vista: ajustarA(full, w, h)}
	}
	mini := leerImagen(md.ruta(m, "-mini.jpg"))
	if mini == nil {
		return &imagenes{mini: true}
	}
	// La miniatura es pequeña: se agranda hasta el tamaño del chat (se verá algo borrosa).
	b := mini.Bounds()
	esc := min(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()))
	grande := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(b.Dx())*esc)), max(1, int(float64(b.Dy())*esc))))
	xdraw.ApproxBiLinear.Scale(grande, grande.Bounds(), mini, b, draw.Src, nil)
	return &imagenes{vista: grande, completa: grande, mini: true}
}

// pedir descarga la foto o el sticker en segundo plano y avisa a la pantalla al acabar.
func (md *medios) pedir(m *Mensaje) {
	if md.enCurso[m.ID] || md.fallidos[m.ID] || md.a.w == nil || md.a.w.cli == nil {
		return
	}
	b, err := os.ReadFile(md.ruta(m, ".pb"))
	if err != nil {
		md.fallidos[m.ID] = true
		return
	}
	md.enCurso[m.ID] = true
	cli, copia := md.a.w.cli, *m
	go func() {
		md.turno <- struct{}{}
		defer func() { <-md.turno }()
		err := md.descargar(cli, &copia, b)
		md.a.hacer(func() {
			delete(md.enCurso, copia.ID)
			delete(md.cache, copia.ID) // que se vuelva a leer, ya con la buena
			if err != nil {
				log.Printf("descargar %s %s: %v", copia.Tipo, copia.ID, err)
				md.fallidos[copia.ID] = true
			}
		})
	}()
}

func (md *medios) descargar(cli *whatsmeow.Client, m *Mensaje, pb []byte) error {
	var dm whatsmeow.DownloadableMessage
	if m.Tipo == tipoSticker {
		st := &waE2E.StickerMessage{}
		if err := proto.Unmarshal(pb, st); err != nil {
			return err
		}
		dm = st
	} else {
		im := &waE2E.ImageMessage{}
		if err := proto.Unmarshal(pb, im); err != nil {
			return err
		}
		dm = im
	}
	datos, err := cli.Download(context.Background(), dm)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(bytes.NewReader(datos))
	if err != nil {
		return err // p. ej. stickers animados: se queda la miniatura
	}
	w, h := tamCompleta()
	img = ajustarA(img, w, h)
	var buf bytes.Buffer
	if m.Tipo == tipoSticker {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 88})
	}
	if err != nil {
		return err
	}
	escribirAtomico(md.rutaImagen(m), buf.Bytes())
	return nil
}

// pegar dibuja una imagen con su transparencia en el lienzo.
func (l *lienzo) pegar(img image.Image, x, y int) {
	b := img.Bounds()
	draw.Draw(l.img, image.Rect(x, y, x+b.Dx(), y+b.Dy()), img, b.Min, draw.Over)
}

// marcoFoto: el hueco gris de una foto que aún no está.
func (l *lienzo) huecoFoto(r image.Rectangle, texto string) {
	l.redondeado(r, 8, color.RGBA{0x11, 0x1b, 0x21, 0xff})
	l.centrado(r, texto, l.f.mini, colSuave)
}
