package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const ancho, alto = 640, 480

var (
	colFondo1   = color.RGBA{6, 6, 9, 255}
	colFondo2   = color.RGBA{11, 18, 32, 255}
	colPanel    = color.RGBA{24, 32, 50, 255}
	colBlanco   = color.RGBA{255, 255, 255, 255}
	colSuave    = color.RGBA{200, 200, 212, 255}
	colGris     = color.RGBA{150, 150, 165, 255}
	colVioleta  = color.RGBA{196, 170, 255, 255}
	colVerde    = color.RGBA{30, 215, 96, 255}
	colRojo     = color.RGBA{240, 80, 90, 255}
	colAmarillo = color.RGBA{255, 200, 60, 255}
)

var (
	letrasUna                       sync.Once
	lTitulo, lGrande, lNormal, lPeq font.Face
)

func letras() {
	letrasUna.Do(func() {
		cara := func(ttf []byte, tam float64) font.Face {
			f, err := opentype.Parse(ttf)
			if err != nil {
				log.Fatal(err)
			}
			c, err := opentype.NewFace(f, &opentype.FaceOptions{Size: tam, DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				log.Fatal(err)
			}
			return c
		}
		lTitulo = cara(gobold.TTF, 22)
		lGrande = cara(gobold.TTF, 30)
		lNormal = cara(goregular.TTF, 20)
		lPeq = cara(goregular.TTF, 16)
	})
}

type lienzo struct{ img *image.RGBA }

func (l lienzo) rect(x, y, w, h int, c color.Color) {
	draw.Draw(l.img, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Over)
}

func (l lienzo) texto(f font.Face, x, y int, s string, c color.Color) int {
	d := font.Drawer{Dst: l.img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y+f.Metrics().Ascent.Ceil())}
	d.DrawString(s)
	return d.Dot.X.Round() - x
}

func medir(f font.Face, s string) int { return font.MeasureString(f, s).Round() }

// parrafo: el texto en líneas que caben en w; devuelve la y de debajo.
func (l lienzo) parrafo(f font.Face, x, y, w int, s string, c color.Color) int {
	alto := f.Metrics().Height.Ceil() + 4
	for _, trozo := range strings.Split(s, "\n") {
		linea := ""
		for _, p := range strings.Fields(trozo) {
			prueba := p
			if linea != "" {
				prueba = linea + " " + p
			}
			if medir(f, prueba) > w && linea != "" {
				l.texto(f, x, y, linea, c)
				y += alto
				linea = p
			} else {
				linea = prueba
			}
		}
		l.texto(f, x, y, linea, c)
		y += alto
	}
	return y
}

func fondo(l lienzo, titulo string) {
	for y := 0; y < alto; y++ {
		k := float64(y) / alto
		c := color.RGBA{
			uint8(float64(colFondo1.R)*(1-k) + float64(colFondo2.R)*k),
			uint8(float64(colFondo1.G)*(1-k) + float64(colFondo2.G)*k),
			uint8(float64(colFondo1.B)*(1-k) + float64(colFondo2.B)*k), 255}
		l.rect(0, y, ancho, 1, c)
	}
	l.rect(0, 0, ancho, 44, color.RGBA{0, 0, 0, 150})
	l.texto(lTitulo, 24, 10, titulo, colBlanco)
	w := medir(lPeq, "Panther OS")
	l.texto(lPeq, ancho-24-w, 14, "Panther OS", colGris)
	for x := 0; x < ancho; x++ { // la línea violeta del arranque
		d := float64(x-ancho/2) / (ancho / 2)
		if a := 1 - d*d; a > 0 {
			l.rect(x, 44, 1, 2, color.NRGBA{196, 170, 255, uint8(200 * a)})
		}
	}
}

// dibujarMenu: una lista para elegir (o marcar) con la cruceta.
func dibujarMenu(m *menu) *image.RGBA {
	letras()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	l := lienzo{img}
	fondo(l, "Flashear tarjeta")
	x, an := 32, ancho-64
	y := l.parrafo(lGrande, x, 58, an, m.titulo, colBlanco) + 2
	if m.explica != "" {
		y = l.parrafo(lPeq, x, y, an, m.explica, colSuave) + 4
	}
	// Las filas que caben, con la elegida a la vista.
	altoFila := 34
	if m.conDetalle() {
		altoFila = 54
	}
	caben := (408 - y) / altoFila
	if caben < 1 {
		caben = 1
	}
	primera := 0
	if m.sel >= caben {
		primera = m.sel - caben + 1
	}
	for i := primera; i < len(m.ops) && i < primera+caben; i++ {
		o := m.ops[i]
		fy := y + (i-primera)*altoFila
		if i == m.sel {
			l.rect(x-8, fy, an+16, altoFila-4, colPanel)
			l.rect(x-8, fy, 4, altoFila-4, colVioleta)
		}
		tx := x + 4
		if m.multiple { // la casilla: verde si está marcada
			bx, by := tx, fy+7
			if o.marcada {
				l.rect(bx, by, 18, 18, colVerde)
				l.rect(bx+5, by+5, 8, 8, colBlanco)
			} else {
				l.rect(bx, by, 18, 18, colGris)
				l.rect(bx+2, by+2, 14, 14, colPanel)
			}
			tx += 30
		}
		col := colBlanco
		if o.apagada {
			col = colGris
		}
		l.texto(lNormal, tx, fy+5, o.titulo, col)
		if o.detalle != "" {
			l.textoCorto(lPeq, tx, fy+29, ancho-tx-32, o.detalle, colGris)
		}
	}
	if len(m.ops) > caben { // hay más abajo o arriba
		s := fmt.Sprintf("%d de %d", m.sel+1, len(m.ops))
		l.texto(lPeq, ancho-32-medir(lPeq, s), 412, s, colGris)
	}
	ayuda := m.ayuda
	if ayuda == "" {
		ayuda = "A elegir     B volver"
		if m.multiple {
			ayuda = "A marcar     X todo     Y nada     START seguir     B volver"
		}
	}
	l.rect(0, 440, ancho, 40, color.RGBA{0, 0, 0, 150})
	l.texto(lPeq, (ancho-medir(lPeq, ayuda))/2, 451, ayuda, colGris)
	return img
}

func (l lienzo) textoCorto(f font.Face, x, y, max int, s string, c color.Color) {
	if medir(f, s) > max {
		r := []rune(s)
		for len(r) > 0 && medir(f, string(r)+"…") > max {
			r = r[:len(r)-1]
		}
		s = string(r) + "…"
	}
	l.texto(f, x, y, s, c)
}

// dibujar: las pantallas de cambiar la tarjeta.
func dibujar(v vista) *image.RGBA {
	letras()
	img := image.NewRGBA(image.Rect(0, 0, ancho, alto))
	l := lienzo{img}
	fondo(l, "Flashear tarjeta")
	otra := v.otra
	if otra == "" {
		otra = "la otra tarjeta"
	}
	x, y, an := 40, 72, ancho-80
	pie := ""
	switch v.estado {
	case ePreparando:
		y = l.parrafo(lGrande, x, 170, an, "Preparando la tarjeta…", colBlanco) + 8
		l.parrafo(lNormal, x, y, an, "No la saques todavía.", colAmarillo)
	case eNoSePuede:
		y = l.parrafo(lGrande, x, 150, an, "No se puede hacer ahora", colRojo) + 8
		l.parrafo(lNormal, x, y, an, v.mensaje, colSuave)
	case eSacar:
		y = l.parrafo(lGrande, x, y, an, "Saca la tarjeta de Panther OS y mete "+otra, colBlanco) + 10
		y = l.parrafo(lNormal, x, y, an, "Ya puedes sacarla: está guardada y en solo lectura, no se estropea.", colVerde) + 4
		l.parrafo(lNormal, x, y, an, "Si después de meter la otra no pasa nada, pulsa un botón para que la consola mire la ranura otra vez.", colSuave)
		pie = "ranura"
	case eCopiando:
		y = l.parrafo(lGrande, x, 170, an, "Copiando…", colBlanco) + 8
		l.parrafo(lNormal, x, y, an, "No saques la tarjeta.", colAmarillo)
	case eListo:
		y = l.parrafo(lGrande, x, y, an, "¡Listo! Ya está en "+otra, colVerde) + 10
		for _, r := range v.hecho {
			y = l.parrafo(lPeq, x, y, an, "· "+r, colSuave)
		}
		y = l.parrafo(lNormal, x, y+6, an, "Sácala y vuelve a meter la de Panther OS. La consola se reiniciará sola.", colBlanco) + 2
		if v.luego != "" {
			l.parrafo(lPeq, x, y, an, v.luego, colGris)
		}
		pie = "ranura"
	case eError:
		y = l.parrafo(lGrande, x, y, an, "No se ha podido copiar", colRojo) + 8
		y = l.parrafo(lNormal, x, y, an, v.mensaje, colSuave) + 6
		l.parrafo(lNormal, x, y, an, "Saca esa tarjeta y vuelve a meter la de Panther OS: la consola se reiniciará sola.", colBlanco)
		pie = "ranura"
	case eReiniciando:
		l.parrafo(lGrande, x, 190, an, "Reiniciando…", colBlanco)
	}
	if pie != "" {
		l.rect(40, 360, ancho-80, 40, colPanel)
		l.texto(lNormal, 56, 368, "En la ranura: "+v.ranura, colVioleta)
		l.parrafo(lPeq, x, 414, an, "Si se queda parada: apaga la consola con el botón, mete la tarjeta de Panther OS y enciéndela.", colGris)
	}
	return img
}

var hayFB = true

func pintar(v vista)     { mostrar(dibujar(v)) }
func pintarMenu(m *menu) { mostrar(dibujarMenu(m)) }

// mostrar: en la pantalla (/dev/fb0, como la animación de arranque), si se puede.
func mostrar(img *image.RGBA) {
	if !hayFB {
		return
	}
	leer := func(f string) string {
		b, _ := os.ReadFile("/sys/class/graphics/fb0/" + f)
		return strings.TrimSpace(string(b))
	}
	bpp, _ := strconv.Atoi(leer("bits_per_pixel"))
	tam := strings.Split(leer("virtual_size"), ",")
	if len(tam) != 2 || (bpp != 32 && bpp != 16) {
		hayFB = false
		log.Printf("pantalla: %d bits, %v: no sé pintar en ella", bpp, tam)
		return
	}
	w, _ := strconv.Atoi(tam[0])
	h, _ := strconv.Atoi(tam[1])
	stride, _ := strconv.Atoi(leer("stride"))
	if stride == 0 {
		stride = w * bpp / 8
	}
	fb, err := os.OpenFile("/dev/fb0", os.O_WRONLY, 0)
	if err != nil {
		hayFB = false
		log.Printf("pantalla: %v", err)
		return
	}
	defer fb.Close()
	_ = os.WriteFile("/sys/class/graphics/fbcon/cursor_blink", []byte("0"), 0)
	_ = os.WriteFile("/sys/class/graphics/fb0/blank", []byte("0"), 0)
	fila := make([]byte, stride)
	for yy := 0; yy < h && yy < alto; yy++ {
		for xx := 0; xx < w && xx < ancho; xx++ {
			i := img.PixOffset(xx, yy)
			r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
			if bpp == 32 {
				fila[xx*4], fila[xx*4+1], fila[xx*4+2], fila[xx*4+3] = b, g, r, 0xff
			} else {
				p := uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
				fila[xx*2], fila[xx*2+1] = byte(p), byte(p>>8)
			}
		}
		_, _ = fb.WriteAt(fila, int64(yy*stride))
	}
}

// pruebas: todas las pantallas en PNG (las mira consola.yml).
func pruebas(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	imgs := map[string]*image.RGBA{}
	imgs["1-consolas"] = dibujarMenu(menuConsolas())
	imgs["2-darkos"] = dibujarMenu(menuTieneDarkOS())
	imgs["3-para-quien"] = dibujarMenu(menuParaQuien())
	apps := []appTienda{{"whatsapp", "WhatsApp", "app", true}, {"youtube", "YouTube", "app", true},
		{"spotin", "Spotin", "app", true}, {"instagram", "Instagram", "app", false}, {"pokemon", "Pokémon", "app", true},
		{"zarpa", "Zarpa", "app", false}, {"cceleste.zip", "Celeste Classic", "juego", false},
		{"cave.story-evo.zip", "Cave Story", "juego", true}, {"opentyrian.zip", "OpenTyrian", "juego", false}}
	imgs["4-que-pasar"] = dibujarMenu(menuApps(apps))
	ps := []partida{{"Pokemon - Edicion Esmeralda", "", 131072}, {"Pokemon - Rojo Fuego", "", 131072}, {"Mario Kart", "", 512}}
	imgs["5-partidas-gba"] = dibujarMenu(menuPartidas(ps))
	c := carga{resumen: []string{"La Panther Store y 5 apps y juegos (se ponen solas cuando esa consola tenga internet): WhatsApp, YouTube, Spotin, Pokémon, Cave Story",
		"Tus cuentas de 4 apps (WhatsApp no: se vincula otra vez)", "1 wifi guardado", "12 partidas (sin los juegos: esos los pones tú)"}}
	imgs["6-confirmar"] = dibujarMenu(menuConfirmar(destinos[2], c))
	imgs["7-sin-darkos"] = dibujarMenu(menuSinDarkOS())
	vistas := map[string]vista{
		"8-preparando":   {estado: ePreparando},
		"9-sacar":        {estado: eSacar, otra: "la de la otra consola", ranura: "la tarjeta de Panther OS"},
		"10-copiando":    {estado: eCopiando, ranura: "otra tarjeta (32 GB)"},
		"11-listo":       {estado: eListo, otra: "la de la otra consola", ranura: "otra tarjeta (32 GB)", hecho: c.resumen, luego: textoLuego("darkos")},
		"12-listo-ds":    {estado: eListo, otra: "la de la flashcard", ranura: "otra tarjeta (2 GB)", hecho: []string{"Panther OS para la DS (panther.nds, 349 KB)"}, luego: textoLuego("ds")},
		"13-error":       {estado: eError, ranura: "otra tarjeta (8 GB)", mensaje: "esa tarjeta no tiene dArkOS (no hay una partición del sistema)"},
		"14-no-se-puede": {estado: eNoSePuede, mensaje: "La tarjeta de Panther OS no se deja proteger (/roms, la tiene abierta «retroarch»). No has sacado nada: vuelves al menú."},
		"15-reiniciando": {estado: eReiniciando},
	}
	for n, v := range vistas {
		imgs[n] = dibujar(v)
	}
	for n, img := range imgs {
		f, err := os.Create(filepath.Join(dir, n+".png"))
		if err != nil {
			return err
		}
		err = png.Encode(f, img)
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
