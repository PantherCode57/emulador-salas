package main

import (
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"strings"
	"unicode"

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

// Colores de Panther OS (los de las listas de juegos y la pantalla de Wi-Fi) y los de Instagram.
var (
	colFondo   = color.RGBA{0x0b, 0x12, 0x20, 0xff}
	colTexto   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	colSuave   = color.RGBA{0xc8, 0xc8, 0xd4, 0xff}
	colApagado = color.RGBA{0x96, 0x96, 0xa5, 0xff}
	colAyuda   = color.RGBA{0xbe, 0xbe, 0xc8, 0xff}
	colAzul    = color.RGBA{0x96, 0xc8, 0xff, 0xff} // «consola · año · género»
	colMarco   = color.RGBA{0x78, 0xc8, 0xff, 0xff} // borde de lo elegido
	colVerde   = color.RGBA{0x1e, 0xd7, 0x60, 0xff} // lo que suena
	colRojo    = color.RGBA{0xf1, 0x5c, 0x6d, 0xff}
	colOscuro  = color.RGBA{0x0a, 0x0e, 0x18, 0xe1} // reproductor pequeño
	colTecla   = color.RGBA{0xff, 0xff, 0xff, 0x26}
	colRosa    = color.RGBA{0xe1, 0x30, 0x6c, 0xff} // el de Instagram (y el corazón)
	colNaranja = color.RGBA{0xf7, 0x77, 0x37, 0xff}
	colMorado  = color.RGBA{0x83, 0x3a, 0xb4, 0xff}
	colBurbuja = color.RGBA{0x37, 0x97, 0xf0, 0xff} // tus mensajes
)

// Letras: Selawik (la de las listas del menú) si está el tema; si no, las de Go. Los iconos son
// los de Font Awesome que trae EmulationStation.
var (
	rutaLetras = entorno("INSTAGRAM_LETRAS", "/roms/themes/es-theme-panther/_panther/letra")
	rutaIconos = entorno("INSTAGRAM_ICONOS", "/usr/bin/emulationstation/resources/fontawesome-webfont.ttf")
	rutaFondo  = entorno("INSTAGRAM_FONDO", "/roms/themes/es-theme-panther/_panther/fondo-lista.jpg")
)

// Iconos (Font Awesome 4).
const (
	icInstagram    = ""
	icInicio       = ""
	icReels        = ""
	icHistorias    = ""
	icMensajes     = ""
	icUsuario      = ""
	icCorazon      = ""
	icCorazonVacio = ""
	icComentario   = ""
	icPlay         = ""
	icPausa        = ""
	icCamara       = ""
	icRefrescar    = ""
	icAltavoz      = ""
	icHecho        = ""
	icWifiNo       = ""
	icVarias       = ""
	icFoto         = ""
)

type fuentes struct {
	titulo, grande, nombre, normal, texto, pequena, mini font.Face
	iconoGrande, icono, iconoPeq                         font.Face
	reg                                                  *sfnt.Font
	ic                                                   *opentype.Font
	icTam                                                map[int]font.Face
}

// iconoDe: los iconos a cualquier tamaño (para las portadas dibujadas).
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
		nombre:  cara(semi, 17),
		normal:  cara(reg, 16),
		texto:   cara(reg, 18),
		pequena: cara(reg, 15),
		mini:    cara(reg, 13),
		reg:     reg,
	}
	if b, err := os.ReadFile(rutaIconos); err == nil {
		if ic, err := opentype.Parse(b); err == nil {
			f.iconoGrande, f.icono, f.iconoPeq = cara(ic, 26), cara(ic, 18), cara(ic, 15)
			f.ic, f.icTam = ic, map[int]font.Face{}
		}
	}
	if f.icono == nil { // sin iconos: no se dibujan
		f.iconoGrande, f.icono, f.iconoPeq = f.nombre, f.normal, f.mini
	}
	return f
}

// limpiar quita lo que las letras no saben dibujar (emojis sobre todo), para que no salgan cuadros.
func (f *fuentes) limpiar(s string) string {
	var b strings.Builder
	var buf sfnt.Buffer
	for _, r := range s {
		if r == '\n' || r == '\t' {
			b.WriteRune(' ')
			continue
		}
		if unicode.IsControl(r) || r == 0xfe0f || r == 0x200d {
			continue
		}
		if i, err := f.reg.GlyphIndex(&buf, r); err != nil || i == 0 {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

type lienzo struct {
	img   *image.RGBA
	f     *fuentes
	fondo *image.RGBA
}

func nuevoLienzo(f *fuentes) *lienzo {
	l := &lienzo{img: image.NewRGBA(image.Rect(0, 0, ancho, alto)), f: f}
	if im := abrirImagen(rutaFondo); im != nil {
		l.fondo = escalar(im, ancho, alto, true)
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

// escalar: a w×h; con llenar recorta lo que sobra (como la portada en su cuadrado).
func escalar(src image.Image, w, h int, llenar bool) *image.RGBA {
	b := src.Bounds()
	sr := b
	if llenar && b.Dx() > 0 && b.Dy() > 0 {
		k := math.Max(float64(w)/float64(b.Dx()), float64(h)/float64(b.Dy()))
		cw, ch := int(float64(w)/k), int(float64(h)/k)
		x0, y0 := b.Min.X+(b.Dx()-cw)/2, b.Min.Y+(b.Dy()-ch)/2
		sr = image.Rect(x0, y0, x0+cw, y0+ch)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if w >= sr.Dx() && h >= sr.Dy() { // ampliar: CatmullRom, que queda más fino
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, sr, xdraw.Src, nil)
		return dst
	}
	// Reducir con CatmullRom es lentísimo en la consola (su núcleo crece con la reducción: una foto de
	// 1080 a 480 y luego a su tamaño tardaba 1,8 s). Primero por cajas hasta que falte menos del doble
	// y el resto con ApproxBiLinear (en las fotos, casi igual: menos de 0,1 de 255 de diferencia).
	if f := min(sr.Dx()/max(w, 1), sr.Dy()/max(h, 1)); f >= 2 {
		src = reducirCaja(src, sr, f)
		sr = src.Bounds()
	}
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, sr, xdraw.Src, nil)
	return dst
}

// reducirCaja: la parte sr de src, f veces más pequeña, con la media de cada caja de f×f. Las fotos
// de Instagram llegan en YCbCr (JPEG y WebP): de esas se hace la media de la luz y el color se toma
// del centro de la caja, sin pasar cada punto a RGB.
func reducirCaja(src image.Image, sr image.Rectangle, f int) *image.RGBA {
	w, h := max(sr.Dx()/f, 1), max(sr.Dy()/f, 1)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if s, ok := src.(*image.YCbCr); ok {
		n := f * f
		for y := 0; y < h; y++ {
			y0 := sr.Min.Y + y*f
			p := dst.PixOffset(0, y)
			for x := 0; x < w; x++ {
				x0 := sr.Min.X + x*f
				suma := 0
				for j := 0; j < f; j++ {
					o := s.YOffset(x0, y0+j)
					for _, v := range s.Y[o : o+f] {
						suma += int(v)
					}
				}
				c := s.COffset(x0+f/2, y0+f/2)
				r, g, b := color.YCbCrToRGB(uint8(suma/n), s.Cb[c], s.Cr[c])
				dst.Pix[p], dst.Pix[p+1], dst.Pix[p+2], dst.Pix[p+3] = r, g, b, 0xff
				p += 4
			}
		}
		return dst
	}
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(sr)
		draw.Draw(rgba, sr, src, sr.Min, draw.Src)
	}
	n := uint32(f * f)
	for y := 0; y < h; y++ {
		p := dst.PixOffset(0, y)
		for x := 0; x < w; x++ {
			var s [4]uint32
			for j := 0; j < f; j++ {
				o := rgba.PixOffset(sr.Min.X+x*f, sr.Min.Y+y*f+j)
				for i := 0; i < f; i++ {
					for k := 0; k < 4; k++ {
						s[k] += uint32(rgba.Pix[o+i*4+k])
					}
				}
			}
			for k := 0; k < 4; k++ {
				dst.Pix[p+k] = uint8(s[k] / n)
			}
			p += 4
		}
	}
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

// sombra: la silueta de la imagen, desenfocada, para ponerla 5 px por debajo.
func sombra(im *image.RGBA, radio int, fuerza float64) *image.Alpha {
	b := im.Bounds()
	m := radio * 2
	a := image.NewAlpha(image.Rect(0, 0, b.Dx()+2*m, b.Dy()+2*m))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			a.Pix[(y+m)*a.Stride+x+m] = uint8(float64(im.Pix[im.PixOffset(x, y)+3]) * fuerza)
		}
	}
	for pasada := 0; pasada < 3; pasada++ { // tres cajas seguidas ≈ desenfoque gaussiano
		desenfocar(a, radio)
	}
	return a
}

func desenfocar(a *image.Alpha, r int) {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	tmp := make([]int, max(w, h))
	for y := 0; y < h; y++ {
		suma := 0
		for x := -r; x < w+r; x++ {
			if x+r < w {
				suma += int(a.Pix[y*a.Stride+min(max(x+r, 0), w-1)])
			}
			if x-r-1 >= 0 {
				suma -= int(a.Pix[y*a.Stride+x-r-1])
			}
			if x >= 0 && x < w {
				tmp[x] = suma / (2*r + 1)
			}
		}
		for x := 0; x < w; x++ {
			a.Pix[y*a.Stride+x] = uint8(tmp[x])
		}
	}
	for x := 0; x < w; x++ {
		suma := 0
		for y := -r; y < h+r; y++ {
			if y+r < h {
				suma += int(a.Pix[min(max(y+r, 0), h-1)*a.Stride+x])
			}
			if y-r-1 >= 0 {
				suma -= int(a.Pix[(y-r-1)*a.Stride+x])
			}
			if y >= 0 && y < h {
				tmp[y] = suma / (2*r + 1)
			}
		}
		for y := 0; y < h; y++ {
			a.Pix[y*a.Stride+x] = uint8(tmp[y])
		}
	}
}

// pegarSombra pone la sombra (negra) con su imagen encima.
func (l *lienzo) pegar(im *image.RGBA, sb *image.Alpha, x, y int) {
	if sb != nil {
		m := (sb.Rect.Dx() - im.Rect.Dx()) / 2
		r := image.Rect(x-m, y-m+5, x-m+sb.Rect.Dx(), y-m+5+sb.Rect.Dy())
		draw.DrawMask(l.img, r, image.Black, image.Point{}, sb, image.Point{}, draw.Over)
	}
	draw.Draw(l.img, image.Rect(x, y, x+im.Rect.Dx(), y+im.Rect.Dy()), im, image.Point{}, draw.Over)
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

// arriba: escribe con la parte de arriba de las letras en y (como el ancla «la» de la vista previa).
func (l *lienzo) arriba(x, y int, s string, cara font.Face, c color.Color) int {
	return l.texto(x, y+cara.Metrics().Ascent.Round(), s, cara, c)
}

func baseCentrada(r image.Rectangle, cara font.Face) int {
	m := cara.Metrics()
	return r.Min.Y + (r.Dy()+m.Ascent.Round()-m.Descent.Round())/2
}

func (l *lienzo) centrado(r image.Rectangle, s string, cara font.Face, c color.Color) {
	s = recortar(cara, s, r.Dx()-4)
	l.texto(r.Min.X+(r.Dx()-medir(cara, s))/2, baseCentrada(r, cara), s, cara, c)
}

func (l *lienzo) izquierda(r image.Rectangle, s string, cara font.Face, c color.Color) {
	l.texto(r.Min.X, baseCentrada(r, cara), recortar(cara, s, r.Dx()), cara, c)
}

func (l *lienzo) derecha(r image.Rectangle, s string, cara font.Face, c color.Color) {
	s = recortar(cara, s, r.Dx())
	l.texto(r.Max.X-medir(cara, s), baseCentrada(r, cara), s, cara, c)
}

func altoLinea(cara font.Face) int { return cara.Metrics().Height.Round() + 3 }

// ajustar parte el texto en líneas que caben en anchoMax (y corta las palabras demasiado largas).
func ajustar(cara font.Face, s string, anchoMax int) []string {
	var lineas []string
	actual := ""
	for _, p := range strings.Fields(s) {
		for medir(cara, p) > anchoMax {
			r := []rune(p)
			n := len(r)
			for n > 1 && medir(cara, string(r[:n])) > anchoMax {
				n--
			}
			if actual != "" {
				lineas = append(lineas, actual)
				actual = ""
			}
			lineas = append(lineas, string(r[:n]))
			p = string(r[n:])
		}
		prueba := p
		if actual != "" {
			prueba = actual + " " + p
		}
		if medir(cara, prueba) > anchoMax && actual != "" {
			lineas = append(lineas, actual)
			actual = p
		} else {
			actual = prueba
		}
	}
	if actual != "" || len(lineas) == 0 {
		lineas = append(lineas, actual)
	}
	return lineas
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

// ayuda: la línea de botones de abajo («A  Abrir      B  Volver»), centrada o desde x.
func (l *lienzo) ayuda(texto string, x int) {
	if x < 0 {
		x = (ancho - medir(l.f.pequena, texto)) / 2
	}
	l.texto(x, alto-34+l.f.pequena.Metrics().Ascent.Round(), texto, l.f.pequena, colAyuda)
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
