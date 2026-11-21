package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestChaChaRFC8439(t *testing.T) {
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	n := [12]byte{0, 0, 0, 9, 0, 0, 0, 0x4a, 0, 0, 0, 0}
	b := bloqueChaCha(&k, 1, &n)
	if got := hex.EncodeToString(b[:16]); got != "10f1e7e4d13b5915500fdd1fa32071c4" {
		t.Fatalf("bloque de ChaCha20 mal: %s", got)
	}
	if got := hex.EncodeToString(b[48:64]); got != "b5129cd1de164eb9cbd083e8a2503c4e" {
		t.Fatalf("final del bloque mal: %s", got)
	}
}

// clienteDS: lo que hace la DS, en Go.
type clienteDS struct {
	c   net.Conn
	r   *bufio.Reader
	sal *flujo
}

func conectar(t *testing.T, dir, codigo string) (*clienteDS, error) {
	c, err := net.Dial("tcp", dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	br := bufio.NewReader(c)
	saludo, err := br.ReadString('\n')
	if err != nil {
		return nil, err
	}
	campos := strings.Fields(saludo)
	nS, _ := hex.DecodeString(campos[2])
	nD := make([]byte, 16)
	_, _ = rand.Read(nD)
	_, _ = io.WriteString(c, "HOLA 1 "+hex.EncodeToString(nD)+"\n")
	k := claveSesion(codigo, nS, nD)
	d := &clienteDS{c: c, sal: nuevoFlujo(k, sentidoDS)}
	d.r = bufio.NewReader(lectorCifrado{readerConn{br, c}, nuevoFlujo(k, sentidoPuente)})
	if err := d.mandar("PIN-OK"); err != nil {
		return nil, err
	}
	l, err := d.respuesta()
	if err != nil {
		return nil, err
	}
	if len(l) != 1 || !strings.HasPrefix(l[0], "BIEN ") {
		t.Fatalf("saludo: %q", l)
	}
	return d, nil
}

// readerConn: un net.Conn que lee de lo que ya ha leído el bufio del saludo.
type readerConn struct {
	*bufio.Reader
	net.Conn
}

func (r readerConn) Read(b []byte) (int, error) { return r.Reader.Read(b) }

func (d *clienteDS) mandar(s string) error {
	b := []byte(s + "\n")
	d.sal.xor(b)
	_, err := d.c.Write(b)
	return err
}

func (d *clienteDS) respuesta() ([]string, error) {
	cab, err := d.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(cab), "R "))
	if err != nil {
		return nil, err
	}
	var l []string
	for i := 0; i < n; i++ {
		s, err := d.r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		l = append(l, strings.TrimRight(s, "\n"))
	}
	return l, nil
}

func arrancar(t *testing.T) (*puente, string) {
	p := &puente{codigo: "123456", nombre: "Consola de prueba", wa: nuevoWAFalso(true), fallos: map[string]*intentos{}}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go p.atender(c)
		}
	}()
	return p, ln.Addr().String()
}

func TestPuenteWhatsApp(t *testing.T) {
	_, dir := arrancar(t)
	d, err := conectar(t, dir, "123456")
	if err != nil {
		t.Fatal(err)
	}
	pedir := func(s string) []string {
		if err := d.mandar(s); err != nil {
			t.Fatal(err)
		}
		l, err := d.respuesta()
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	if l := pedir("PING"); l[0] != "PONG" {
		t.Fatal(l)
	}
	l := pedir("WA CHATS")
	if len(l) != 5 || l[0] != "CHATS 1" || !strings.HasPrefix(l[1], "C\t111@s.whatsapp.net\tLucía\t1\t18:02\t") {
		t.Fatalf("chats: %q", l)
	}
	if !strings.Contains(l[2], "Sara: Nooo :'(") {
		t.Fatalf("el emoji tendría que ser «:'(»: %q", l[2])
	}
	l = pedir("WA CHAT 111@s.whatsapp.net")
	if len(l) != 4 || l[0] != "CHAT 1\tLucía" || l[1] != "M\t0\t17:58\t\t¿Has probado ya la DS? xD" {
		t.Fatalf("chat: %q", l)
	}
	if l = pedir("WA ENVIAR 111@s.whatsapp.net\thola desde la DS"); l[0] != "OK 2" {
		t.Fatalf("enviar: %q", l)
	}
	if l = pedir("WA CHAT 111@s.whatsapp.net"); len(l) != 5 || !strings.HasSuffix(l[4], "\thola desde la DS") {
		t.Fatalf("no está lo enviado: %q", l)
	}
	if l = pedir("WA CHAT nadie"); !strings.HasPrefix(l[0], "ERROR ") {
		t.Fatalf("chat que no existe: %q", l)
	}
	if l = pedir("ABRACADABRA"); !strings.HasPrefix(l[0], "ERROR ") {
		t.Fatal(l)
	}
}

func TestCodigoMalo(t *testing.T) {
	p, dir := arrancar(t)
	for i := 0; i < maxFallos; i++ {
		if _, err := conectar(t, dir, "000000"); err == nil {
			t.Fatal("con el código malo no tendría que entrar")
		}
	}
	if !p.bloqueado("127.0.0.1") {
		t.Fatal("después de 5 códigos malos tendría que estar bloqueado")
	}
	if _, err := conectar(t, dir, "123456"); err == nil {
		t.Fatal("bloqueado, ni con el bueno")
	}
}

func TestLimpiar(t *testing.T) {
	casos := map[string]string{
		"hola\tque\ntal":   "hola que tal",
		"jajaja 😂😂":        "jajaja xDxD",
		"👍🏽 vale":          "(ok) vale",
		"❤️":               "<3",
		"ñandú €5 «sí»":    "ñandú €5 «sí»",
		"chino 你好":         "chino ··",
		"\x01raro\x7f":     "raro",
		"Pokémon ♀ y ♂ ok": "Pokémon ♀ y ♂ ok",
	}
	for in, quiero := range casos {
		if got := limpiar(in, 100); got != quiero {
			t.Errorf("limpiar(%q) = %q, quería %q", in, got, quiero)
		}
	}
	if got := limpiar(strings.Repeat("á", 10), 5); got != "áá…" {
		t.Errorf("recortar: %q", got)
	}
}

func TestVincularFalso(t *testing.T) {
	p := &puente{codigo: "123456", nombre: "Prueba", wa: nuevoWAFalso(false), fallos: map[string]*intentos{}}
	if l := p.orden("WA ESTADO"); l[0] != "ESTADO sin-vincular 1" {
		t.Fatalf("estado: %q", l)
	}
	if l := p.orden("WA CHATS"); !strings.HasPrefix(l[0], "ERROR ") {
		t.Fatalf("sin vincular no hay chats: %q", l)
	}
	if l := p.orden("WA VINCULAR 0612345678"); !strings.HasPrefix(l[0], "ERROR ") {
		t.Fatalf("un número sin prefijo no vale: %q", l)
	}
	if l := p.orden("WA VINCULAR 34612345678"); l[0] != "CODIGO PANT-HER1" {
		t.Fatalf("vincular: %q", l)
	}
	if l := p.orden("WA ESTADO"); l[0] != "ESTADO vinculando 1" {
		t.Fatalf("vinculando: %q", l)
	}
	time.Sleep(2100 * time.Millisecond)
	if l := p.orden("WA ESTADO"); l[0] != "ESTADO conectado 2" {
		t.Fatalf("tendría que haberse vinculado: %q", l)
	}
}

// TestRemoto: lo de la DS contra un puente que ya está en marcha (el del emulador de Android, con
// adb forward): PUENTE_REMOTO=127.0.0.1:8790 PUENTE_CODIGO=123456 go test -run TestRemoto
func TestRemoto(t *testing.T) {
	dir := os.Getenv("PUENTE_REMOTO")
	if dir == "" {
		t.Skip("sin PUENTE_REMOTO")
	}
	d, err := conectar(t, dir, os.Getenv("PUENTE_CODIGO"))
	if err != nil {
		t.Fatal(err)
	}
	for _, orden := range []string{"PING", "WA ESTADO", "WA CHATS"} {
		if err := d.mandar(orden); err != nil {
			t.Fatal(err)
		}
		l, err := d.respuesta()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s → %q", orden, l)
		if len(l) == 0 || strings.HasPrefix(l[0], "ERROR") {
			t.Fatalf("%s: %q", orden, l)
		}
	}
}
