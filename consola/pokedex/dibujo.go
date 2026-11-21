package main

import (
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Tamaño de la pantalla (el de la XF35H). Se puede cambiar con POKEMULADOR_ANCHO/ALTO.
var ancho, alto = 640, 480

// Colores de Panther OS (los de las listas de juegos y la pantalla de Wi-Fi).
var (
	colFondo   = color.RGBA{0x0b, 0x12, 0x20, 0xff}
	colTexto   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colSuave   = color.RGBA{0xc8, 0xc8, 0xd4, 0xff}
	colApagado = color.RGBA{0x96, 0x96, 0xa5, 0xff}
	colAyuda   = color.RGBA{0xbe, 0xbe, 0xc8, 0xff}
	colAzul    = color.RGBA{0x96, 0xc8, 0xff, 0xff}
	colMarco   = color.RGBA{0x78, 0xc8, 0xff, 0xff} // borde de lo elegido
	colVerde   = color.RGBA{0x1e, 0xd7, 0x60, 0xff} // conseguido
)

// Letras: Selawik (la de las listas del menú) si está el tema; si no, las de Go. Los iconos son
// los de Font Awesome que trae EmulationStation.
var (
	rutaLetras = entorno("POKEDEX_LETRAS", "/roms/themes/es-theme-panther/_panther/letra")
	rutaIconos = entorno("POKEDEX_ICONOS", "/usr/bin/emulationstation/resources/fontawesome-webfont.ttf")
	rutaFondo  = entorno("POKEDEX_FONDO", "/roms/themes/es-theme-panther/_panther/fondo-lista.jpg")
)

// Iconos (Font Awesome 4).
const (
	icBuscar = ""
	icHecho  = ""
	icReloj  = ""
	icLibro  = ""
	icCartas = ""
	icMando  = ""
)

type fuentes struct {
	titulo, grande, nombre, normal, texto, pequena, mini font.Face
	numero                                               font.Face // el número de cada Pokémon
	icono, iconoPeq                                      font.Face
	reg                                                  *sfnt.Font
	ic                                                   *opentype.Font
	icTam                                                map[int]font.Face
}

// iconoDe: los iconos a cualquier tamaño.
func (f *fuentes) iconoDe(tam float64) font.Face {
	if f.ic == nil {
		return f.grande
	}
	k := int(tam + 0.5)
	if c, ok := f.icTam[k]; ok {
		return c
	}
	c := cara(f.ic, float64(k))
	f.icTam[k] = c
	return c
}

// tiene: si la letra sabe dibujar ese carácter (♀ y ♂ no están en todas).
func (f *fuentes) tiene(r rune) bool {
	var buf sfnt.Buffer
	i, err := f.reg.GlyphIndex(&buf, r)
	return err == nil && i != 0
}

func cargarLetra(archivo string, reserva []byte) *opentype.Font {
	if b, err := os.ReadFile(rutaLetras + "/" + archivo); err == nil {
		if f, err := opentype.Parse(b); err == nil {
			return f
		}
	}
	f, err := opentype.Parse(reserva)
	if err != nil {
		panic("no se pudieron cargar las letras")
	}
	return f
}

func cara(f *opentype.Font, tam float64) font.Face {
	c, err := opentype.NewFace(f, &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	return c
}

func cargarFuentes() *fuentes {
	reg := cargarLetra("selawk.ttf", goregular.TTF)
	semi := cargarLetra("selawksb.ttf", gobold.TTF)
	f := &fuentes{
		titulo:  cara(semi, 26),
		grande:  cara(semi, 28),
		nombre:  cara(semi, 18),
		normal:  cara(reg, 16),
		texto:   cara(reg, 18),
		pequena: cara(reg, 15),
		mini:    cara(reg, 13),
		numero:  cara(semi, 15),
		reg:     reg,
	}
	if b, err := os.ReadFile(rutaIconos); err == nil {
		if ic, err := opentype.Parse(b); err == nil {
			f.icono, f.iconoPeq = cara(ic, 18), cara(ic, 14)
			f.ic, f.icTam = ic, map[int]font.Face{}
		}
	}
	if f.icono == nil { // sin iconos: no se dibujan
		f.icono, f.iconoPeq = f.normal, f.mini
	}
	return f
}

type lienzo struct {
	img   *image.RGBA
	f     *fuentes
	fondo *image.RGBA
}

func nuevoLienzo(f *fuentes) *lienzo {
	l := &lienzo{img: image.NewRGBA(image.Rect(0, 0, ancho, alto)), f: f}
	if im := abrirImagen(rutaFondo); im != nil {
		l.fondo = escalar(im, ancho, alto)
	}
	return l
}

func abrirImagen(ruta string) image.Image {
	fh, err := os.Open(ruta)
	if err != nil {
		return nil
	}
	defer fh.Close()
	im, _, err := image.Decode(fh)
	if err != nil {
		return nil
	}
	return im
}

// escalar: a w×h, recortando lo que sobra (como el fondo de las listas).
func escalar(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	sr := b
	if b.Dx() > 0 && b.Dy() > 0 {
		k := math.Max(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()))
		cw, ch := int(float64(w)/k), int(float64(h)/k)
		x0, y0 := b.Min.X+(b.Dx()-cw)/2, b.Min.Y+(b.Dy()-ch)/2
		sr = image.Rect(x0, y0, x0+cw, y0+ch)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sr, xdraw.Src, nil)
	return dst
}

// redondear deja las esquinas transparentes (con borde suave).
func redondear(im *image.RGBA, radio int) {
	w, h := im.Bounds().Dx(), im.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := 0.0, 0.0
			if x < radio {
				dx = float64(radio-x) - 0.5
			} else if x >= w-radio {
				dx = float64(x-(w-radio)) + 0.5
			}
			if y < radio {
				dy = float64(radio-y) - 0.5
			} else if y >= h-radio {
				dy = float64(y-(h-radio)) + 0.5
			}
			if dx == 0 || dy == 0 {
				continue
			}
			d := math.Hypot(dx, dy) - float64(radio)
			a := 1.0
			if d > 0.5 {
				a = 0
			} else if d > -0.5 {
				a = 0.5 - d
			}
			if a < 1 {
				i := im.PixOffset(x, y)
				for k := 0; k < 4; k++ {
					im.Pix[i+k] = uint8(float64(im.Pix[i+k]) * a)
				}
			}
		}
	}
}

func mezclaColor(a, b color.RGBA, k float64) color.RGBA {
	k = math.Max(0, math.Min(1, k))
	return color.RGBA{
		uint8(float64(a.R)*(1-k) + float64(b.R)*k + 0.5),
		uint8(float64(a.G)*(1-k) + float64(b.G)*k + 0.5),
		uint8(float64(a.B)*(1-k) + float64(b.B)*k + 0.5), 0xff}
}

// placa: el fondo de las tarjetas de Panther OS: rectángulo redondeado con un degradado de a a b
// en diagonal y la mitad de arriba un poco más clara.
func placa(w, h, radio int, a, b color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	blanco := color.RGBA{255, 255, 255, 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := mezclaColor(a, b, (float64(x)/float64(w)+float64(y)/float64(h))/2)
			if y < h/2 {
				c = mezclaColor(c, blanco, 0.11)
			}
			i := im.PixOffset(x, y)
			im.Pix[i], im.Pix[i+1], im.Pix[i+2], im.Pix[i+3] = c.R, c.G, c.B, 0xff
		}
	}
	redondear(im, radio)
	return im
}

// pegar pone una imagen (con transparencia) en el lienzo.
func (l *lienzo) pegar(im *image.RGBA, x, y int) {
	draw.Draw(l.img, image.Rect(x, y, x+im.Rect.Dx(), y+im.Rect.Dy()), im, image.Point{}, draw.Over)
}

// glifo: un icono centrado en (cx, cy).
func (l *lienzo) glifo(cara font.Face, g string, cx, cy int, c color.Color) {
	b, _ := font.BoundString(cara, g)
	w := (b.Max.X - b.Min.X).Round()
	h := (b.Max.Y - b.Min.Y).Round()
	l.texto(cx-w/2-b.Min.X.Round(), cy-h/2-b.Min.Y.Round(), g, cara, c)
}

func (l *lienzo) limpiarFondo() {
	if l.fondo != nil {
		copy(l.img.Pix, l.fondo.Pix)
	} else {
		draw.Draw(l.img, l.img.Bounds(), &image.Uniform{colFondo}, image.Point{}, draw.Src)
	}
}

func (l *lienzo) rect(r image.Rectangle, c color.Color) {
	draw.Draw(l.img, r, &image.Uniform{c}, image.Point{}, draw.Over)
}

// redondeado: rectángulo con las esquinas redondas.
func (l *lienzo) redondeado(r image.Rectangle, radio int, c color.Color) {
	if radio*2 > r.Dy() {
		radio = r.Dy() / 2
	}
	if radio*2 > r.Dx() {
		radio = r.Dx() / 2
	}
	u := &image.Uniform{c}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		dentro := 0
		t := min(y-r.Min.Y, r.Max.Y-1-y)
		if t < radio {
			v := float64(radio) - float64(t) - 0.5
			dentro = int(math.Round(float64(radio) - math.Sqrt(math.Max(0, float64(radio*radio)-v*v))))
		}
		draw.Draw(l.img, image.Rect(r.Min.X+dentro, y, r.Max.X-dentro, y+1), u, image.Point{}, draw.Over)
	}
}

// marcoRedondo: solo el borde (de grosor g) de un rectángulo redondeado.
func (l *lienzo) marcoRedondo(r image.Rectangle, radio, g int, c color.Color) {
	cr, cg, cb, ca := c.RGBA()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if !(image.Point{x, y}.In(l.img.Rect)) {
				continue
			}
			d := distanciaBorde(x-r.Min.X, y-r.Min.Y, r.Dx(), r.Dy(), radio)
			a := 0.0
			if d >= 0 && d < float64(g) {
				a = 1
			} else if d < 0 && d > -1 {
				a = 1 + d
			} else if d >= float64(g) && d < float64(g)+1 {
				a = float64(g) + 1 - d
			}
			if a <= 0 {
				continue
			}
			i := l.img.PixOffset(x, y)
			k := a * float64(ca) / 0xffff
			l.img.Pix[i] = uint8(float64(l.img.Pix[i])*(1-k) + float64(cr>>8)*k)
			l.img.Pix[i+1] = uint8(float64(l.img.Pix[i+1])*(1-k) + float64(cg>>8)*k)
			l.img.Pix[i+2] = uint8(float64(l.img.Pix[i+2])*(1-k) + float64(cb>>8)*k)
		}
	}
}

// distanciaBorde: cuánto hacia dentro está (x, y) del borde de un rectángulo w×h con esquinas de radio.
func distanciaBorde(x, y, w, h, radio int) float64 {
	fx, fy := float64(x)+0.5, float64(y)+0.5
	r := float64(radio)
	cx := math.Min(math.Max(fx, r), float64(w)-r)
	cy := math.Min(math.Max(fy, r), float64(h)-r)
	if (fx < r || fx > float64(w)-r) && (fy < r || fy > float64(h)-r) {
		return r - math.Hypot(fx-cx, fy-cy)
	}
	return math.Min(math.Min(fx, float64(w)-fx), math.Min(fy, float64(h)-fy))
}

func (l *lienzo) circulo(cx, cy, radio int, c color.Color) {
	l.redondeado(image.Rect(cx-radio, cy-radio, cx+radio, cy+radio), radio, c)
}

func medir(cara font.Face, s string) int { return font.MeasureString(cara, s).Round() }

// texto escribe con la línea base en y; devuelve dónde acaba.
func (l *lienzo) texto(x, y int, s string, cara font.Face, c color.Color) int {
	d := &font.Drawer{Dst: l.img, Src: image.NewUniform(c), Face: cara, Dot: fixed.P(x, y)}
	d.DrawString(s)
	return d.Dot.X.Round()
}

func baseCentrada(r image.Rectangle, cara font.Face) int {
	m := cara.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Round()-m.Descent.Round())/2
}

func (l *lienzo) centrado(r image.Rectangle, s string, cara font.Face, c color.Color) {
	s = recortar(cara, s, r.Dx()-4)
	l.texto(r.Min.X+(r.Dx()-medir(cara, s))/2, baseCentrada(r, cara), s, cara, c)
}

func (l *lienzo) izquierda(r image.Rectangle, s string, cara font.Face, c color.Color) int {
	return l.texto(r.Min.X, baseCentrada(r, cara), recortar(cara, s, r.Dx()), cara, c)
}

func (l *lienzo) derecha(r image.Rectangle, s string, cara font.Face, c color.Color) {
	s = recortar(cara, s, r.Dx())
	l.texto(r.Max.X-medir(cara, s), baseCentrada(r, cara), s, cara, c)
}

func recortar(cara font.Face, s string, anchoMax int) string {
	if medir(cara, s) <= anchoMax {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && medir(cara, string(r)+"…") > anchoMax {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// marco pasa la imagen al formato del núcleo (XRGB8888 en little endian: B, G, R, X).
func (l *lienzo) marco() []byte {
	out := make([]byte, len(l.img.Pix))
	l.marcoEn(out)
	return out
}

// marcoEn hace lo mismo en una memoria que ya existe.
func (l *lienzo) marcoEn(out []byte) {
	px := l.img.Pix
	for i := 0; i+3 < len(px) && i+3 < len(out); i += 4 {
		out[i], out[i+1], out[i+2], out[i+3] = px[i+2], px[i+1], px[i], 0xff
	}
}
