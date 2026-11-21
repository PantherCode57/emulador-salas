package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
)

// Los dibujos de los Pokémon (los sprites pequeños) no van en la app ni en el repositorio: son de
// Nintendo. La Pokédex los baja de PokeAPI (su repositorio de sprites) cuando salen en la lista y
// los guarda en la consola (~/.config/panther/pokedex/sprites/<n>.png). Mientras la app está
// abierta, va bajando poco a poco los que faltan, para que luego salgan también sin internet. Sin
// conexión se ve un hueco.
var fuenteSprites = entorno("POKEDEX_SPRITES", "https://raw.githubusercontent.com/PokeAPI/sprites/master/sprites/pokemon/")

// Lo que mide el dibujo en la lista (cabe en la fila).
const spriteAncho, spriteAlto = 44, 36

type spriteListo struct {
	n   int
	im  *image.RGBA
	err error
}

type sprites struct {
	dir    string
	fuente string
	cli    *http.Client

	// Solo desde el bucle de la app:
	img     map[int]*image.RGBA
	pedidos map[int]bool      // en la cola o bajándose
	noHay   map[int]time.Time // falló: no se vuelve a pedir hasta entonces

	listo chan spriteListo // del trabajador al bucle

	mu      sync.Mutex
	cola    []int // la última que entra es la primera que sale (lo que se ve ahora)
	aviso   chan struct{}
	sinRed  time.Time // hasta cuándo no se baja nada de fondo (tras un fallo de red)
	siguien int       // por dónde va la bajada de fondo
}

func nuevosSprites(dir string) *sprites {
	return &sprites{
		dir:     dir,
		fuente:  fuenteSprites,
		cli:     &http.Client{Timeout: 20 * time.Second},
		img:     map[int]*image.RGBA{},
		pedidos: map[int]bool{},
		noHay:   map[int]time.Time{},
		listo:   make(chan spriteListo, 64),
		aviso:   make(chan struct{}, 1),
		siguien: 1,
	}
}

func (s *sprites) ruta(n int) string { return filepath.Join(s.dir, strconv.Itoa(n)+".png") }

// arrancar: los que bajan (dos para lo que se ve y uno que, cuando no hay nada que hacer, baja de
// fondo lo que falta).
func (s *sprites) arrancar() {
	_ = os.MkdirAll(s.dir, 0o700)
	for i := 0; i < 3; i++ {
		go s.trabajar(i == 0)
	}
}

// de: el dibujo del Pokémon si ya está en la memoria; si no, lo pide (desde el bucle).
func (s *sprites) de(n int) *image.RGBA {
	if im, ok := s.img[n]; ok {
		return im
	}
	s.pedir(n)
	return nil
}

// pedir: a la cola, si no está ya o no falló hace poco (desde el bucle).
func (s *sprites) pedir(n int) {
	if _, ok := s.img[n]; ok || s.pedidos[n] {
		return
	}
	if t, ok := s.noHay[n]; ok && time.Now().Before(t) {
		return
	}
	s.pedidos[n] = true
	s.mu.Lock()
	s.cola = append(s.cola, n)
	if len(s.cola) > 120 { // lo más viejo ya no se ve: fuera
		for _, v := range s.cola[:len(s.cola)-120] {
			delete(s.pedidos, v)
		}
		s.cola = append([]int{}, s.cola[len(s.cola)-120:]...)
	}
	s.mu.Unlock()
	select {
	case s.aviso <- struct{}{}:
	default:
	}
}

var errNoExiste = errors.New("no hay dibujo")

// recibir: lo que trae el trabajador (desde el bucle). Devuelve si hay que volver a dibujar.
func (s *sprites) recibir(r spriteListo) bool {
	delete(s.pedidos, r.n)
	switch {
	case r.err == nil:
		s.img[r.n] = r.im
		return true
	case errors.Is(r.err, errNoExiste):
		s.noHay[r.n] = time.Now().Add(24 * time.Hour)
	default:
		s.noHay[r.n] = time.Now().Add(30 * time.Second)
	}
	return false
}

func (s *sprites) sacar() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cola) == 0 {
		return 0, false
	}
	n := s.cola[len(s.cola)-1]
	s.cola = s.cola[:len(s.cola)-1]
	if len(s.cola) > 0 { // que otro trabajador coja el siguiente
		select {
		case s.aviso <- struct{}{}:
		default:
		}
	}
	return n, true
}

func (s *sprites) trabajar(deFondo bool) {
	for {
		if n, ok := s.sacar(); ok {
			im, err := s.cargar(n)
			s.listo <- spriteListo{n: n, im: im, err: err}
			continue
		}
		if !deFondo {
			<-s.aviso
			continue
		}
		if s.bajarDeFondo() {
			time.Sleep(100 * time.Millisecond) // sin prisa: es de fondo
			continue
		}
		select {
		case <-s.aviso:
		case <-time.After(5 * time.Second):
		}
	}
}

// bajarDeFondo: el siguiente que no está en la consola (solo al disco, no a la memoria).
// Devuelve si ha hecho algo (false: no queda nada o no hay red).
func (s *sprites) bajarDeFondo() bool {
	s.mu.Lock()
	if time.Now().Before(s.sinRed) || s.siguien > total {
		s.mu.Unlock()
		return false
	}
	n := s.siguien
	for n <= total && existeArchivo(s.ruta(n)) {
		n++
	}
	s.siguien = n + 1
	s.mu.Unlock()
	if n > total {
		return false
	}
	if _, err := s.bajar(n); err != nil && !errors.Is(err, errNoExiste) {
		s.mu.Lock()
		s.sinRed = time.Now().Add(time.Minute)
		s.siguien = n // se vuelve a probar luego
		s.mu.Unlock()
		return false
	}
	return true
}

// delDisco: los que ya están en la consola, sin bajar nada (para las pantallas de prueba).
func (s *sprites) delDisco() {
	for n := 1; n <= total; n++ {
		if b, err := os.ReadFile(s.ruta(n)); err == nil {
			if src, err := png.Decode(bytes.NewReader(b)); err == nil {
				s.img[n] = prepararSprite(src)
			}
		}
	}
}

func existeArchivo(r string) bool {
	st, err := os.Stat(r)
	return err == nil && st.Size() > 0
}

// cargar: de la consola o, si no está, de internet; ya preparado para la lista.
func (s *sprites) cargar(n int) (*image.RGBA, error) {
	b, err := os.ReadFile(s.ruta(n))
	if err != nil || len(b) == 0 {
		if b, err = s.bajar(n); err != nil {
			return nil, err
		}
	}
	src, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		_ = os.Remove(s.ruta(n)) // estropeado: se baja otra vez la próxima vez
		return nil, err
	}
	return prepararSprite(src), nil
}

// bajar: el PNG de internet, comprobado, a la consola.
func (s *sprites) bajar(n int) ([]byte, error) {
	req, err := http.NewRequest("GET", s.fuente+strconv.Itoa(n)+".png", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PantherPokedex/1 (consola portátil)")
	resp, err := s.cli.Do(req)
	if err != nil {
		var dns *net.DNSError
		if errors.As(err, &dns) {
			s.mu.Lock()
			s.sinRed = time.Now().Add(time.Minute)
			s.mu.Unlock()
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errNoExiste
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sprite %d: contesta %d", n, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	if _, err := png.DecodeConfig(bytes.NewReader(b)); err != nil {
		return nil, fmt.Errorf("sprite %d: no es un PNG", n)
	}
	tmp := s.ruta(n) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		if os.Rename(tmp, s.ruta(n)) != nil {
			_ = os.Remove(tmp)
		}
	}
	return b, nil
}

// prepararSprite: recorta lo transparente de alrededor y lo encaja en spriteAncho×spriteAlto
// (los pequeños se agrandan al doble sin suavizar, como dibujos de píxeles que son).
func prepararSprite(src image.Image) *image.RGBA {
	b := src.Bounds()
	caja := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a > 0x1000 {
				caja = caja.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, spriteAncho, spriteAlto))
	if caja.Empty() {
		return dst
	}
	k := math.Min(float64(spriteAncho)/float64(caja.Dx()), float64(spriteAlto)/float64(caja.Dy()))
	var esc xdraw.Scaler = xdraw.CatmullRom
	if k >= 1 {
		k = math.Min(math.Floor(k), 2)
		esc = xdraw.NearestNeighbor
	}
	w, h := int(math.Round(float64(caja.Dx())*k)), int(math.Round(float64(caja.Dy())*k))
	x0, y0 := (spriteAncho-w)/2, (spriteAlto-h)/2
	esc.Scale(dst, image.Rect(x0, y0, x0+w, y0+h), src, caja, xdraw.Over, nil)
	return dst
}
