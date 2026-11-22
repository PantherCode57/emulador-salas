package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// verVideo: lo que hace la DS con un vídeo (VER y sus paquetes, diciendo por dónde va según llegan).
// Devuelve cuántas imágenes (JPEG bien hechos del tamaño del OK) y muestras de sonido llegan, y si
// llega el final.
func verVideo(t *testing.T, d *clienteDS, clave string, plazo time.Duration) (int, int, bool) {
	t.Helper()
	_ = d.c.SetDeadline(time.Now().Add(plazo))
	if err := d.mandar("VER " + clave + " 0 16000"); err != nil {
		t.Fatal(err)
	}
	l, err := d.respuesta()
	if err != nil {
		t.Fatal(err)
	}
	var dur, w, h, sonido int
	if len(l) != 1 {
		t.Fatalf("VER: %q", l)
	}
	if n, _ := fmt.Sscanf(l[0], "OK %d %d %d %d", &dur, &w, &h, &sonido); n != 4 {
		t.Fatalf("VER: %q", l)
	}
	imagenes, muestras, recibido := 0, 0, 0
	for {
		var cab [8]byte
		if _, err := io.ReadFull(d.r, cab[:]); err != nil {
			t.Logf("vídeo cortado: %v", err)
			return imagenes, muestras, false
		}
		n := int(cab[1]) | int(cab[2])<<8 | int(cab[3])<<16
		ts := binary.LittleEndian.Uint32(cab[4:])
		datos := make([]byte, n)
		if _, err := io.ReadFull(d.r, datos); err != nil {
			t.Fatal(err)
		}
		recibido += 8 + n
		switch cab[0] {
		case 'V':
			im, err := jpeg.Decode(bytes.NewReader(datos))
			if err != nil {
				t.Fatalf("una imagen del vídeo no es un JPEG: %v", err)
			}
			if im.Bounds().Dx() != w || im.Bounds().Dy() != h {
				t.Fatalf("imagen de %v, el OK decía %d×%d", im.Bounds(), w, h)
			}
			imagenes++
		case 'A':
			muestras += len(descodificarADPCM(datos))
		case 'F':
			return imagenes, muestras, true
		case 'E':
			t.Fatalf("error del vídeo: %s", datos)
		default:
			t.Fatalf("paquete raro %q", cab[0])
		}
		if err := d.mandar(fmt.Sprintf("L %d %d", recibido, ts)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestADPCM(t *testing.T) {
	var e estadoADPCM
	m := make([]int16, 1600)
	for i := range m {
		m[i] = int16(8000 * math.Sin(2*math.Pi*440*float64(i)/16000))
	}
	var otra []int16
	for i := 0; i < len(m); i += 400 {
		otra = append(otra, descodificarADPCM(e.codificar(m[i:i+400]))...)
	}
	if len(otra) != len(m) {
		t.Fatalf("salen %d muestras de %d", len(otra), len(m))
	}
	var err2, sen float64
	for i := range m {
		d := float64(m[i]) - float64(otra[i])
		err2 += d * d
		sen += float64(m[i]) * float64(m[i])
	}
	if snr := 10 * math.Log10(sen/err2); snr < 20 {
		t.Fatalf("el ADPCM suena mal: %.1f dB", snr)
	}
}

func puenteCompleto() *puente {
	return &puente{codigo: "123456", nombre: "Prueba", wa: nuevoWAFalso(true), ig: nuevoIGFalso(), yt: &ytFalso{},
		fallos: map[string]*intentos{}}
}

func TestInstagramFalso(t *testing.T) {
	p := puenteCompleto()
	if l := p.orden("IG ESTADO"); l[0] != "ESTADO bien\tduena.de.la.ds" {
		t.Fatalf("estado: %q", l)
	}
	l := p.orden("IG INICIO")
	if len(l) != 3 || l[0] != "INICIO 1" {
		t.Fatalf("inicio: %q", l)
	}
	c := strings.Split(l[1], "\t")
	if len(c) != 9 || c[0] != "P" || c[2] != "lucia.dibuja" || !strings.HasPrefix(c[3], "Mi dibujo nuevo") || c[4] != "87" {
		t.Fatalf("publicación: %q", c)
	}
	ms := strings.Fields(c[8])
	if len(ms) != 2 || ms[0][0] != 'f' || ms[1][0] != 'f' {
		t.Fatalf("medios: %q", c[8])
	}
	// la foto, a la medida de la DS
	d, err := p.imagen(ms[0][1:] + " 256 192")
	if err != nil {
		t.Fatal(err)
	}
	im, err := jpeg.Decode(bytes.NewReader(d))
	if err != nil {
		t.Fatal(err)
	}
	if b := im.Bounds(); b.Dx() != 192 || b.Dy() != 192 {
		t.Fatalf("una foto cuadrada en 256×192 tendría que ser de 192×192: %v", b)
	}
	if d, err = p.imagen(ms[0][1:] + " 256 144 c"); err != nil {
		t.Fatal(err)
	}
	if im, err = jpeg.Decode(bytes.NewReader(d)); err != nil || im.Bounds().Dx() != 256 || im.Bounds().Dy() != 144 {
		t.Fatalf("recortada tendría que llenar 256×144: %v", err)
	}
	if _, err := p.imagen("nada 64 64"); err == nil {
		t.Fatal("una clave que no existe no tendría que dar foto")
	}
	// el vídeo de la segunda publicación
	c2 := strings.Split(l[2], "\t")
	if v := strings.Fields(c2[8]); len(v) != 1 || v[0][0] != 'v' || !strings.Contains(v[0], ":") {
		t.Fatalf("el vídeo: %q", c2[8])
	}
	if l := p.orden("IG MAS"); len(l) != 2 || l[0] != "INICIO 0" {
		t.Fatalf("más: %q", l)
	}
	if l := p.orden("IG MEGUSTA " + c[1] + " 1"); l[0] != "OK" {
		t.Fatalf("me gusta: %q", l)
	}
	if l := p.orden("IG MEGUSTA nada 1"); !strings.HasPrefix(l[0], "ERROR") {
		t.Fatalf("me gusta de nada: %q", l)
	}
	if l := p.orden("IG HISTORIAS"); len(l) != 4 || l[1] != "H\t501\tlucia.dibuja\t0" {
		t.Fatalf("historias: %q", l)
	}
	if l := p.orden("IG HISTORIA 501"); len(l) != 3 || l[0] != "HISTORIA\tlucia.dibuja" || !strings.HasPrefix(l[1], "S\tf") {
		t.Fatalf("historia: %q", l)
	}
	if l := p.orden("IG CHATS"); len(l) != 3 || !strings.HasPrefix(l[1], "C\tt1\tLucía\t1\t") {
		t.Fatalf("chats: %q", l)
	}
	if l := p.orden("IG ENVIAR t1\thola"); l[0] != "OK" {
		t.Fatalf("enviar: %q", l)
	}
	if l := p.orden("IG CHAT t1"); len(l) != 4 || l[0] != "CHAT\tLucía" || !strings.HasSuffix(l[3], "\thola") || !strings.HasPrefix(l[3], "M\t1\t") {
		t.Fatalf("chat: %q", l)
	}
	if l := (&puente{}).orden("IG ESTADO"); !strings.HasPrefix(l[0], "ERROR") {
		t.Fatalf("sin Instagram: %q", l)
	}
}

func TestYouTubeYVideoFalsos(t *testing.T) {
	p := puenteCompleto()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go p.atender(c)
		}
	}()
	d, err := conectar(t, ln.Addr().String(), "123456")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.mandar("YT BUSCAR gatos"); err != nil {
		t.Fatal(err)
	}
	l, err := d.respuesta()
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 7 || l[0] != "VIDEOS 1" {
		t.Fatalf("buscar: %q", l)
	}
	c := strings.Split(l[1], "\t")
	if len(c) != 7 || c[3] != "Cómo dibujar un gato (gatos)" || c[5] != "0:06" || c[6] != "1 mil vistas" {
		t.Fatalf("vídeo: %q", c)
	}
	// la miniatura, por la misma conexión («B <n>» y los bytes)
	if err := d.mandar("IMG " + c[2] + " 256 144 c"); err != nil {
		t.Fatal(err)
	}
	cab, err := d.r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(cab), "B "))
	if err != nil {
		t.Fatalf("IMG: %q", cab)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(d.r, b); err != nil {
		t.Fatal(err)
	}
	if im, err := jpeg.Decode(bytes.NewReader(b)); err != nil || im.Bounds().Dx() != 256 {
		t.Fatalf("miniatura: %v", err)
	}
	if err := d.mandar("PING"); err != nil { // y la conexión sigue bien después de los bytes
		t.Fatal(err)
	}
	if l, err := d.respuesta(); err != nil || l[0] != "PONG" {
		t.Fatalf("después de la foto: %q %v", l, err)
	}
	// el vídeo, por otra conexión
	v, err := conectar(t, ln.Addr().String(), "123456")
	if err != nil {
		t.Fatal(err)
	}
	im, mu, fin := verVideo(t, v, c[1], 30*time.Second)
	if im < 60 || mu < 6*16000-2000 || !fin {
		t.Fatalf("vídeo de mentira: %d imágenes, %d muestras, acabado %v", im, mu, fin)
	}
	w, err := conectar(t, ln.Addr().String(), "123456")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.mandar("VER archivo:prueba 0 16000"); err != nil {
		t.Fatal(err)
	}
	if l, err := w.respuesta(); err != nil || !strings.HasPrefix(l[0], "ERROR") {
		t.Fatalf("sin -pruebas no se puede pedir un archivo: %q %v", l, err)
	}
}
