package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font"
)

// Medidas de los iconos: el de la lista y la imagen grande de la ficha (las tarjetas de las apps
// son de 264×152; estas, de la misma forma).
const (
	iconoAncho, iconoAlto   = 104, 60
	imagenAncho, imagenAlto = 288, 166
)

// Colores de la tienda: de azul cielo a añil, en diagonal.
var (
	colTiendaA = color.RGBA{0x0e, 0xa5, 0xe9, 0xff}
	colTiendaB = color.RGBA{0x63, 0x66, 0xf1, 0xff}
)

func mezclaColor(a, b color.RGBA, k float64) color.RGBA {
	k = math.Max(0, math.Min(1, k))
	return color.RGBA{
		uint8(float64(a.R)*(1-k) + float64(b.R)*k + 0.5),
		uint8(float64(a.G)*(1-k) + float64(b.G)*k + 0.5),
		uint8(float64(a.B)*(1-k) + float64(b.B)*k + 0.5), 0xff}
}

// aclarar: como la franja de arriba de las tarjetas (un 11 % de blanco).
func aclarar(c color.RGBA, k float64) color.RGBA {
	return mezclaColor(c, color.RGBA{255, 255, 255, 255}, k)
}

// placa: el fondo de las tarjetas de Panther OS en una imagen nueva: rectángulo redondeado de
// color (o degradado de a a b en diagonal) con la mitad de arriba un poco más clara.
func placa(w, h, radio int, a, b color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	mitad := h / 2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := mezclaColor(a, b, (float64(x)/float64(w)+float64(y)/float64(h))/2)
			if y < mitad {
				c = aclarar(c, 0.11)
			}
			i := im.PixOffset(x, y)
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, 0xff
		}
	}
	redondear(im, radio)
	return im
}

// glifoCentrado: un icono de Font Awesome en blanco, centrado en (cx, cy).
func glifoCentrado(im *image.RGBA, cara font.Face, g string, cx, cy int, c color.Color) {
	b, _ := font.BoundString(cara, g)
	w := (b.Max.X - b.Min.X).Round()
	h := (b.Max.Y - b.Min.Y).Round()
	x := cx - w/2 - b.Min.X.Round()
	y := cy - h/2 - b.Min.Y.Round()
	l := &lienzo{img: im}
	l.texto(x, y, g, cara, c)
}

// iconoDibujado: para lo que no tiene imagen (o mientras se baja): su color, su icono y su nombre.
func (f *fuentes) iconoDibujado(e *elemento, w, h int) *image.RGBA {
	b := mezclaColor(e.color, color.RGBA{0, 0, 0, 255}, 0.25)
	im := placa(w, h, max(6, h/7), e.color, b)
	if f.ic != nil {
		glifoCentrado(im, f.iconoDe(float64(h)*0.42), e.glifo, w/2, h*40/100, colTexto)
	}
	if h >= 50 {
		cara := f.mini
		if h >= 120 {
			cara = f.titulo2
		}
		l := &lienzo{img: im, f: f}
		s := recortar(cara, e.nombre, w-10)
		l.texto((w-medir(cara, s))/2, h*88/100, s, cara, colTexto)
	}
	return im
}

// logo de la tienda (cuadrado redondeado con la bolsa).
func (f *fuentes) logoTienda(tam int) *image.RGBA {
	im := placa(tam, tam, tam/4, colTiendaA, colTiendaB)
	if f.ic != nil {
		glifoCentrado(im, f.iconoDe(float64(tam)*0.55), icBolsa, tam/2, tam/2+tam/40, colTexto)
	}
	return im
}

// iconosDe: el icono de la lista y la imagen de la ficha a partir de una imagen cualquiera.
func iconosDe(src image.Image) (icono, imagen *image.RGBA) {
	imagen = escalar(src, imagenAncho, imagenAlto, true)
	redondear(imagen, 14)
	icono = escalar(src, iconoAncho, iconoAlto, true)
	redondear(icono, 9)
	return icono, imagen
}

// iconoApp: la tarjeta de la app en el tema (la misma de la pantalla principal).
func (t *tienda) iconoApp(e *elemento) (*image.RGBA, *image.RGBA) {
	im := abrirImagen(filepath.Join(t.tarjetas, e.app.sistema+".png"))
	if im == nil {
		return nil, nil
	}
	icono := escalar(im, iconoAncho, iconoAlto, false)
	imagen := escalar(im, imagenAncho, imagenAlto, false)
	return icono, imagen
}

// capturaJuego: la captura del juego, de la copia guardada o del repositorio de PortMaster.
func capturaJuego(ctx context.Context, g juegoPM) (image.Image, error) {
	dir := filepath.Join(carpetaDatos, "imagenes")
	copia := filepath.Join(dir, stem(g.zip)+".png")
	if im := abrirImagen(copia); im != nil {
		return im, nil
	}
	if g.captura == "" {
		return nil, os.ErrNotExist
	}
	b, err := leerURL(ctx, imagenesPMURL+stem(g.zip)+"/"+g.captura, 8<<20)
	if err != nil {
		return nil, err
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	// Se guarda ya del tamaño de la ficha (las capturas son de 640×480 o más).
	im := escalar(src, imagenAncho*2, imagenAlto*2, true)
	if os.MkdirAll(dir, 0o755) == nil {
		var buf bytes.Buffer
		if png.Encode(&buf, im) == nil {
			_ = escribirAtomico(copia, buf.Bytes(), 0o644)
		}
	}
	return im, nil
}

// guardarTarjeta: la tarjeta «Panther Store» de la pantalla principal (264×152, como las otras).
func guardarTarjeta(f *fuentes, ruta string) error {
	return guardarPlaca(f, colTiendaA, colTiendaB, icBolsa, "Panther Store", ruta)
}

// tarjetaDeApp: la de una app que llega por la tienda, con su color, su icono y su nombre. Se
// dibuja en el trabajador, así que no usa las caras de las letras de la pantalla (no se pueden
// usar a la vez desde dos sitios): guardarPlaca hace las suyas.
func tarjetaDeApp(f *fuentes, e *elemento, ruta string) error {
	return guardarPlaca(f, e.color, mezclaColor(e.color, color.RGBA{0, 0, 0, 255}, 0.25), e.glifo, e.nombre, ruta)
}

func guardarPlaca(f *fuentes, a, b color.RGBA, glifo, nombre, ruta string) error {
	im := image.NewRGBA(image.Rect(0, 0, 264, 152))
	p := placa(260, 146, 20, a, b)
	draw.Draw(im, image.Rect(2, 3, 262, 149), p, image.Point{}, draw.Over)
	if f.ic != nil {
		glifoCentrado(im, cara(f.ic, 60), glifo, 132, 52, colTexto)
	}
	var letra font.Face
	for tam := 34.0; tam >= 22; tam-- {
		letra = cara0(f, tam)
		if medir(letra, nombre) <= 232 {
			break
		}
	}
	l := &lienzo{img: im, f: f}
	l.texto((264-medir(letra, nombre))/2, 133, nombre, letra, colTexto)
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil && !strings.Contains(ruta, string(os.PathSeparator)) {
		return err
	}
	out, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, im)
}

// cara0: la letra seminegrita de las tarjetas a otro tamaño.
func cara0(f *fuentes, tam float64) font.Face {
	return cara(f.semi, tam)
}
