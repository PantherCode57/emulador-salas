package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Lo que se habla con la DS (todo cifrado salvo el saludo):
//
//	puente: PANTHER-PUENTE 1 <nS en hex>        (sin cifrar)
//	DS:     HOLA 1 <nD en hex>                  (sin cifrar)
//	DS:     PIN-OK                              (ya cifrado: si no se lee así, código malo y se cierra)
//	puente: R 1 / BIEN <nombre>
//
// Luego la DS pide y el puente contesta «R <n>» y n líneas:
//
//	PING                    R 1 / PONG
//	WA ESTADO               R 1 / ESTADO <estado> <versión>
//	WA CHATS                R n / CHATS <versión>, y C\t<jid>\t<nombre>\t<no leídos>\t<hora>\t<previa>
//	WA CHAT <jid>           R n / CHAT <versión>\t<nombre>, y M\t<mío 0/1>\t<hora>\t<de>\t<texto>
//	WA ENVIAR <jid>\t<txt>  R 1 / OK <versión>
//	WA VINCULAR <teléfono>  R 1 / CODIGO <código de 8 letras para el WhatsApp del móvil>
//	IG …                    Instagram (instagram.go)
//	YT …                    YouTube (youtube.go)
//	IMG <clave> <w> <h> [c] B <n> y n bytes: una foto en JPEG (imagenes.go)
//	VER <clave> <desde> <f> un vídeo: desde ahí la conexión es solo para él (video.go)
//
// Si algo falla: R 1 / ERROR <qué>.

type puente struct {
	codigo  string
	nombre  string
	wa      whatsapp
	ig      instagramDS // instagram.go
	yt      youtubeDS   // youtube.go
	ayuda   *ayudante   // la app de Android (YouTube y los vídeos), o nil
	pruebas bool        // deja ver «archivo:» (el vídeo de la prueba del emulador)

	mu     sync.Mutex
	fallos map[string]*intentos // códigos malos por IP

	unaVez sync.Once
	reg    *registro   // las claves de las fotos y los vídeos (imagenes.go)
	fotos  *cacheFotos // las últimas fotos hechas

	muIG     sync.Mutex
	igCursor string // lo siguiente del inicio de Instagram
}

// registro: las claves (se hace la primera vez, que las pruebas no lo ponen).
func (p *puente) registro() *registro {
	p.unaVez.Do(func() {
		if p.reg == nil {
			p.reg = nuevoRegistro()
		}
		if p.fotos == nil {
			p.fotos = &cacheFotos{}
		}
	})
	return p.reg
}

type intentos struct {
	n      int
	ultimo time.Time
}

const maxFallos = 5

func (p *puente) contestarBusquedas(puerto int) error {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{Port: puerto})
	if err != nil {
		return err
	}
	buf := make([]byte, 256)
	for {
		n, de, err := c.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(buf[:n]), "PANTHER-DS ") {
			log.Printf("me busca %s", de)
			_, _ = c.WriteToUDP([]byte("PANTHER-PUENTE "+version+" "+p.nombre+"\n"), de)
		}
	}
}

func (p *puente) escuchar(puerto int) error {
	ln, err := net.Listen("tcp4", fmt.Sprintf(":%d", puerto))
	if err != nil {
		return err
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go p.atender(c)
	}
}

func ipDe(c net.Conn) string {
	h, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	return h
}

// bloqueado: demasiados códigos malos desde esa IP en los últimos 5 minutos.
func (p *puente) bloqueado(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	f := p.fallos[ip]
	if f == nil {
		return false
	}
	if time.Since(f.ultimo) > 5*time.Minute {
		delete(p.fallos, ip)
		return false
	}
	return f.n >= maxFallos
}

func (p *puente) fallo(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f := p.fallos[ip]
	if f == nil {
		f = &intentos{}
		p.fallos[ip] = f
	}
	f.n++
	f.ultimo = time.Now()
}

// lectorCifrado: descifra lo que llega y lo da en líneas.
type lectorCifrado struct {
	c net.Conn
	f *flujo
}

func (l lectorCifrado) Read(b []byte) (int, error) {
	n, err := l.c.Read(b)
	l.f.xor(b[:n])
	return n, err
}

type conexion struct {
	c   net.Conn
	sal *flujo
	mu  sync.Mutex
}

// responder: «R <n>» y las líneas, cifrado.
func (k *conexion) responder(lineas ...string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "R %d\n", len(lineas))
	for _, l := range lineas {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	datos := []byte(b.String())
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sal.xor(datos)
	_ = k.c.SetWriteDeadline(time.Now().Add(20 * time.Second))
	_, err := k.c.Write(datos)
	return err
}

// responderDatos: «B <n>» y n bytes (una foto), cifrado.
func (k *conexion) responderDatos(d []byte) error {
	b := append([]byte(fmt.Sprintf("B %d\n", len(d))), d...)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sal.xor(b)
	_ = k.c.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := k.c.Write(b)
	return err
}

func (p *puente) atender(c net.Conn) {
	defer c.Close()
	ip := ipDe(c)
	if p.bloqueado(ip) {
		log.Printf("%s: bloqueado por códigos malos", ip)
		return
	}
	nS := make([]byte, 16)
	_, _ = rand.Read(nS)
	if _, err := io.WriteString(c, "PANTHER-PUENTE "+version+" "+hex.EncodeToString(nS)+"\n"); err != nil {
		return
	}
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	// El saludo de la DS, sin cifrar, byte a byte (lo que venga detrás ya va cifrado).
	var saludo []byte
	b := make([]byte, 1)
	for len(saludo) < 128 {
		if _, err := io.ReadFull(c, b); err != nil {
			return
		}
		if b[0] == '\n' {
			break
		}
		saludo = append(saludo, b[0])
	}
	campos := strings.Fields(string(saludo))
	if len(campos) != 3 || campos[0] != "HOLA" {
		log.Printf("%s: saludo raro %q", ip, saludo)
		return
	}
	nD, err := hex.DecodeString(campos[2])
	if err != nil || len(nD) < 6 {
		return
	}
	clave := claveSesion(p.codigo, nS, nD)
	r := bufio.NewReaderSize(lectorCifrado{c, nuevoFlujo(clave, sentidoDS)}, 4096)
	k := &conexion{c: c, sal: nuevoFlujo(clave, sentidoPuente)}

	// Lo primero tiene un largo fijo: con un código malo llega basura y no hay que esperar un «\n».
	primera := make([]byte, len("PIN-OK\n"))
	if _, err := io.ReadFull(r, primera); err != nil || string(primera) != "PIN-OK\n" {
		log.Printf("%s: código malo", ip)
		p.fallo(ip)
		return
	}
	log.Printf("%s: DS conectada", ip)
	if k.responder("BIEN "+p.nombre) != nil {
		return
	}
	for {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Minute))
		linea, err := r.ReadString('\n')
		if err != nil {
			log.Printf("%s: se ha ido (%v)", ip, err)
			return
		}
		orden := strings.TrimRight(linea, "\r\n")
		switch {
		case strings.HasPrefix(orden, "IMG "):
			d, err := p.imagen(strings.TrimPrefix(orden, "IMG "))
			if err != nil {
				err = k.responder("ERROR " + limpiar(err.Error(), 120))
			} else {
				err = k.responderDatos(d)
			}
			if err != nil {
				return
			}
		case strings.HasPrefix(orden, "VER "):
			p.ver(k, r, ip, strings.TrimPrefix(orden, "VER ")) // la conexión ya es la del vídeo
			return
		default:
			if err := k.responder(p.orden(orden)...); err != nil {
				return
			}
		}
	}
}

// orden: lo que contesta a cada petición de la DS.
func (p *puente) orden(linea string) []string {
	cmd, resto, _ := strings.Cut(linea, " ")
	switch cmd {
	case "PING":
		return []string{"PONG"}
	case "WA":
		return p.ordenWA(resto)
	case "IG":
		return p.ordenIG(resto)
	case "YT":
		return p.ordenYT(resto)
	}
	return []string{"ERROR no sé hacer «" + limpiar(cmd, 20) + "»"}
}

func (p *puente) ordenWA(linea string) []string {
	cmd, arg, _ := strings.Cut(linea, " ")
	var orden string
	switch cmd {
	case "ESTADO":
		orden = "estado"
	case "CHATS":
		orden = "chats"
	case "CHAT":
		orden = "chat " + arg
	case "VINCULAR":
		orden = "vincular " + arg
	case "ENVIAR":
		jid, texto, ok := strings.Cut(arg, "\t")
		if !ok || strings.TrimSpace(texto) == "" {
			return []string{"ERROR falta el mensaje"}
		}
		orden = "enviar " + jid + "\t" + strings.ReplaceAll(texto, "\n", " ")
	default:
		return []string{"ERROR no sé hacer «WA " + limpiar(cmd, 20) + "»"}
	}
	r, err := p.wa.pedir(orden)
	if err != nil {
		return []string{"ERROR " + err.Error()}
	}
	if r.Error != "" {
		return []string{"ERROR " + limpiar(r.Error, 120)}
	}
	ver := strconv.Itoa(r.Ver)
	switch cmd {
	case "ESTADO":
		return []string{"ESTADO " + r.Estado + " " + ver}
	case "VINCULAR":
		return []string{"CODIGO " + limpiar(r.Codigo, 20)}
	case "CHATS":
		l := []string{"CHATS " + ver}
		for _, c := range r.Chats {
			l = append(l, strings.Join([]string{"C", limpiar(c.JID, 80), limpiar(c.Nombre, 60),
				strconv.Itoa(c.NoLeidos), limpiar(c.Hora, 8), limpiar(c.Previa, 90)}, "\t"))
		}
		return l
	case "CHAT":
		l := []string{"CHAT " + ver + "\t" + limpiar(r.Nombre, 60)}
		for _, m := range r.Mensajes {
			mio := "0"
			if m.Mio {
				mio = "1"
			}
			texto := m.Texto
			if m.Error {
				texto += " (no se ha enviado)"
			}
			l = append(l, strings.Join([]string{"M", mio, limpiar(m.Hora, 8), limpiar(m.De, 40), limpiar(texto, 400)}, "\t"))
		}
		return l
	}
	return []string{"OK " + ver}
}

// Los emojis más corrientes, como se escribían antes (la DS no tiene dibujos para ellos).
var emojis = map[rune]string{
	'😂': "xD", '🤣': "xD", '😀': ":D", '😃': ":D", '😄': ":D", '😁': ":D", '😆': "xD", '😊': ":)",
	'🙂': ":)", '😉': ";)", '😍': "<3", '🥰': "<3", '😘': ":*", '😢': ":(", '😭': ":'(", '😞': ":(",
	'😡': ">:(", '😠': ">:(", '😮': ":O", '😱': ":O", '😎': "B)", '🤔': "(?)", '😅': "^^'",
	'❤': "<3", '💕': "<3", '💖': "<3", '💙': "<3", '💜': "<3", '👍': "(ok)", '👎': "(no)",
	'🙏': "(gracias)", '👏': "(bravo)", '🔥': "(fuego)", '🎉': "(fiesta)", '✅': "(sí)", '❌': "(x)",
}

// limpiar: sin tabuladores ni saltos de línea, y sin lo que la DS no sabe dibujar (sus letras son
// las de Latin-1 y unos pocos signos); como mucho max bytes.
func limpiar(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
		case r < 0x7f || (r >= 0xa0 && r <= 0xff):
			b.WriteRune(r)
		case strings.ContainsRune("♀♂‘’“”…•€←↑→↓▲►▼◄", r):
			b.WriteRune(r)
		case r == 0xfe0f || r == 0x200d || (r >= 0x1f3fb && r <= 0x1f3ff):
			// variantes, uniones y tonos de piel de los emojis
		default:
			if e, ok := emojis[r]; ok {
				b.WriteString(e)
			} else {
				b.WriteString("·")
			}
		}
	}
	t := strings.TrimSpace(b.String())
	if len(t) > max {
		t = t[:max]
		for !utf8.ValidString(t) {
			t = t[:len(t)-1]
		}
		t += "…"
	}
	return t
}
