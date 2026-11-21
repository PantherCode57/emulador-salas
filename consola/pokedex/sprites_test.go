package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// pngDePrueba: un 96×96 transparente con un cuadrado de color en medio (como un sprite).
func pngDePrueba(t *testing.T, lado int) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := 48 - lado/2; y < 48+lado/2; y++ {
		for x := 48 - lado/2; x < 48+lado/2; x++ {
			im.Set(x, y, color.NRGBA{0xff, 0xcc, 0x00, 0xff})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestSpritesBajarYGuardar(t *testing.T) {
	cuadro := pngDePrueba(t, 60)
	var pedidas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidas.Add(1)
		switch r.URL.Path {
		case "/25.png":
			_, _ = w.Write(cuadro)
		case "/26.png":
			_, _ = w.Write([]byte("<html>esto no es un png</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	s := nuevosSprites(t.TempDir())
	s.fuente = srv.URL + "/"
	_ = os.MkdirAll(s.dir, 0o700)

	im, err := s.cargar(25)
	if err != nil || im == nil {
		t.Fatalf("cargar(25): %v", err)
	}
	if im.Bounds().Dx() != spriteAncho || im.Bounds().Dy() != spriteAlto {
		t.Fatalf("el sprite mide %v", im.Bounds())
	}
	if !existeArchivo(s.ruta(25)) {
		t.Fatal("no se ha guardado en la consola")
	}
	antes := pedidas.Load()
	if _, err := s.cargar(25); err != nil || pedidas.Load() != antes {
		t.Fatalf("la segunda vez tenía que salir de la consola (err %v, %d pedidas)", err, pedidas.Load()-antes)
	}
	if _, err := s.cargar(9999); !errors.Is(err, errNoExiste) {
		t.Fatalf("un 404 tenía que ser errNoExiste: %v", err)
	}
	if _, err := s.cargar(26); err == nil || existeArchivo(s.ruta(26)) {
		t.Fatalf("lo que no es un PNG no se guarda (err %v)", err)
	}
}

func TestSpritesCola(t *testing.T) {
	s := nuevosSprites(t.TempDir())
	s.pedir(1)
	s.pedir(2)
	s.pedir(1) // ya pedido
	if n, _ := s.sacar(); n != 2 {
		t.Fatalf("primero sale lo último pedido, no el %d", n)
	}
	if n, _ := s.sacar(); n != 1 {
		t.Fatalf("luego el %d", n)
	}
	if _, ok := s.sacar(); ok {
		t.Fatal("la cola tenía que estar vacía")
	}
	// Un fallo de red: no se vuelve a pedir enseguida. Un 404: tampoco.
	s.recibir(spriteListo{n: 1, err: errors.New("sin red")})
	s.recibir(spriteListo{n: 2, err: errNoExiste})
	s.pedir(1)
	s.pedir(2)
	if _, ok := s.sacar(); ok {
		t.Fatal("se ha vuelto a pedir lo que acaba de fallar")
	}
	s.noHay[1] = time.Now().Add(-time.Second) // pasado el rato, sí
	s.pedir(1)
	if n, ok := s.sacar(); !ok || n != 1 {
		t.Fatal("pasado el rato se tenía que volver a pedir")
	}
	if !s.recibir(spriteListo{n: 1, im: image.NewRGBA(image.Rect(0, 0, 1, 1))}) || s.img[1] == nil {
		t.Fatal("lo que llega se guarda y se vuelve a dibujar")
	}
	// Con muchísimos pedidos, lo más viejo se olvida.
	for n := 100; n < 400; n++ {
		s.pedir(n)
	}
	if len(s.cola) != 120 || s.pedidos[100] || !s.pedidos[399] {
		t.Fatalf("cola de %d; pedido el 100: %v", len(s.cola), s.pedidos[100])
	}
}

func TestPrepararSprite(t *testing.T) {
	for _, lado := range []int{10, 30, 80} {
		src, err := png.Decode(bytes.NewReader(pngDePrueba(t, lado)))
		if err != nil {
			t.Fatal(err)
		}
		im := prepararSprite(src)
		if im.Bounds() != image.Rect(0, 0, spriteAncho, spriteAlto) {
			t.Fatalf("mide %v", im.Bounds())
		}
		// Lo de color queda en medio y dentro.
		if _, _, _, a := im.At(spriteAncho/2, spriteAlto/2).RGBA(); a == 0 {
			t.Fatalf("lado %d: el centro ha quedado vacío", lado)
		}
	}
	vacio := prepararSprite(image.NewNRGBA(image.Rect(0, 0, 96, 96)))
	if vacio.Bounds().Dx() != spriteAncho {
		t.Fatal("uno vacío también tiene que medir lo mismo")
	}
}

// Con la red de verdad (POKEDEX_RED=1 go test -run DeVerdad): los trabajadores bajan lo pedido de
// PokeAPI y, de fondo, los que faltan.
func TestSpritesDeVerdad(t *testing.T) {
	if os.Getenv("POKEDEX_RED") == "" {
		t.Skip("solo con POKEDEX_RED=1")
	}
	s := nuevosSprites(t.TempDir())
	s.arrancar()
	s.pedir(1025)
	s.pedir(25)
	llegados := map[int]bool{}
	fin := time.After(30 * time.Second)
	for !(llegados[25] && llegados[1025]) {
		select {
		case r := <-s.listo:
			if r.err != nil {
				t.Fatalf("sprite %d: %v", r.n, r.err)
			}
			s.recibir(r)
			llegados[r.n] = true
		case <-fin:
			t.Fatal("no han llegado en 30 s")
		}
	}
	for i := 0; i < 100 && !existeArchivo(s.ruta(3)); i++ {
		time.Sleep(200 * time.Millisecond)
	}
	if !existeArchivo(s.ruta(1)) || !existeArchivo(s.ruta(3)) {
		t.Fatal("la bajada de fondo no ha empezado por el 1")
	}
}
