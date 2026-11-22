package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Los vídeos para la DS (los de YouTube y los de Instagram). Van por una conexión aparte: la DS abre
// otra con el mismo saludo y el mismo código, y en ella pide
//
//	VER <clave> <desde ms> <muestras por segundo del sonido>
//
// El puente contesta «R 1 / OK <duración ms> <ancho> <alto> <con sonido 0/1>» (o un ERROR) y luego
// manda paquetes, cifrados como todo: [tipo][largo: 3 bytes][ms: 4 bytes][largo bytes], en little
// endian:
//
//	'V'  una imagen: un JPEG (baseline, 4:2:0) de ancho×alto
//	'A'  sonido en IMA ADPCM (adpcm.go): [muestra int16][índice][0] y 4 bits por muestra, el bajo primero
//	'F'  se ha acabado
//	'E'  un error (texto)
//
// La DS manda «L <bytes recibidos> <ms por donde va>» cada poco. El puente no se adelanta más de
// adelantoVideo a lo que la DS va enseñando y, si el wifi no da para todo (lo mandado y no recibido
// pasa de ventanaVideo), se salta imágenes y baja la calidad: el sonido no se salta nunca.
//
// Las imágenes y el sonido los da el ayudante de Android ya descodificados (YUV 4:2:0 pequeño y
// muestras de 16 bits a la frecuencia de la DS); aquí se hacen JPEG y ADPCM.

const (
	anchoVideo    = 256
	altoVideo     = 192
	areaVideo     = 256 * 144 // lo que puede descodificar la DS unas 12 veces por segundo
	fpsVideo      = 12
	adelantoVideo = 1500 // ms
	ventanaVideo  = 40 << 10
	bloqueSonido  = 800 // muestras por paquete de sonido (50 ms a 16 kHz)
)

type paqueteMedio struct {
	tipo byte // 'V' o 'A'
	ts   uint32
	yuv  []byte  // 'V': Y (ancho×alto), Cb y Cr (la mitad de ancho y de alto cada uno)
	pcm  []int16 // 'A'
}

type fuenteMedio interface {
	info() (dur uint32, w, h int, sonido bool)
	leer() (*paqueteMedio, error) // io.EOF al acabar
	cerrar()
}

func (k *conexion) paquete(tipo byte, ts uint32, d []byte) error {
	b := make([]byte, 8+len(d))
	b[0] = tipo
	b[1], b[2], b[3] = byte(len(d)), byte(len(d)>>8), byte(len(d)>>16)
	binary.LittleEndian.PutUint32(b[4:], ts)
	copy(b[8:], d)
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sal.xor(b)
	_ = k.c.SetWriteDeadline(time.Now().Add(20 * time.Second))
	_, err := k.c.Write(b)
	return err
}

func (p *puente) abrirFuente(fuente string, desde uint32, tasa int) (fuenteMedio, error) {
	if strings.HasPrefix(fuente, "falso:") {
		return &fuenteFalsa{desde: desde, dur: 6000, tasa: tasa}, nil
	}
	if p.ayuda == nil {
		return nil, errors.New("los vídeos van con la app Puente DS del móvil")
	}
	return p.ayuda.ver(fuente, desde, tasa)
}

// ver: lo que hace la conexión de un vídeo, hasta que la DS la cierra.
func (p *puente) ver(k *conexion, r *bufio.Reader, ip, arg string) {
	campos := strings.Fields(arg)
	if len(campos) < 3 {
		_ = k.responder("ERROR VER <clave> <desde> <frecuencia>")
		return
	}
	desde, _ := strconv.Atoi(campos[1])
	tasa, _ := strconv.Atoi(campos[2])
	if tasa < 8000 || tasa > 48000 {
		tasa = 16000
	}
	if desde < 0 {
		desde = 0
	}
	fuente := ""
	if strings.HasPrefix(campos[0], "archivo:") && p.pruebas {
		fuente = campos[0]
	} else if u, ok := p.registro().url(campos[0]); ok && !strings.HasPrefix(u, "pk:") {
		fuente = u
	} else {
		_ = k.responder("ERROR ese vídeo ya no está: vuelve a la lista")
		return
	}
	f, err := p.abrirFuente(fuente, uint32(desde), tasa)
	if err != nil {
		log.Printf("%s: vídeo: %v", ip, err)
		_ = k.responder("ERROR " + limpiar(err.Error(), 160))
		return
	}
	defer f.cerrar()
	dur, w, h, sonido := f.info()
	if k.responder(fmt.Sprintf("OK %d %d %d %s", dur, w, h, uno(sonido))) != nil {
		return
	}
	log.Printf("%s: vídeo desde %d ms (%d×%d, %d ms)", ip, desde, w, h, dur)

	var mu sync.Mutex
	reloj, recibido := uint32(desde), 0
	aviso := make(chan struct{}, 1)
	fin := make(chan struct{})
	go func() {
		defer close(fin)
		for {
			_ = k.c.SetReadDeadline(time.Now().Add(20 * time.Second))
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			var b, ms int
			if n, _ := fmt.Sscanf(l, "L %d %d", &b, &ms); n == 2 {
				mu.Lock()
				recibido, reloj = b, uint32(max(0, ms))
				mu.Unlock()
				select {
				case aviso <- struct{}{}:
				default:
				}
			}
		}
	}()

	enviado, calidad, saltadas, imagenes := 0, 50, 0, 0
	ultimoSalto, ultimaSubida := time.Time{}, time.Now()
	var cod estadoADPCM
	var pend []int16
	var tsPend uint32
	enviar := func(tipo byte, ts uint32, d []byte) bool {
		if err := k.paquete(tipo, ts, d); err != nil {
			return false
		}
		enviado += 8 + len(d)
		return true
	}
	// sonido: en bloques iguales, seguidos (la hora de cada uno sale de las muestras de antes)
	sonar := func(todo bool) bool {
		for len(pend) >= bloqueSonido || (todo && len(pend) > 0) {
			n := min(bloqueSonido, len(pend))
			b := pend[:n]
			if n%2 == 1 {
				b = append(append([]int16(nil), b...), b[n-1])
			}
			if !enviar('A', tsPend, cod.codificar(b)) {
				return false
			}
			pend = pend[n:]
			tsPend += uint32(n * 1000 / tasa)
		}
		return true
	}
	defer func() {
		log.Printf("%s: vídeo cerrado (%d imágenes, %d saltadas, %d KB)", ip, imagenes, saltadas, enviado>>10)
	}()
	for {
		pq, err := f.leer()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if !sonar(true) || !enviar('F', dur, nil) {
					return
				}
			} else {
				log.Printf("%s: vídeo: %v", ip, err)
				_ = enviar('E', 0, []byte(limpiar(err.Error(), 160)))
			}
			select { // que la DS acabe de enseñarlo y cierre
			case <-fin:
			case <-time.After(time.Minute):
			}
			return
		}
		// no adelantarse demasiado a la DS
		for {
			mu.Lock()
			rl := reloj
			mu.Unlock()
			if pq.ts <= rl+adelantoVideo {
				break
			}
			select {
			case <-fin:
				return
			case <-aviso:
			case <-time.After(100 * time.Millisecond):
			}
		}
		select {
		case <-fin:
			return
		default:
		}
		switch pq.tipo {
		case 'A':
			if len(pend) == 0 {
				tsPend = pq.ts
			}
			pend = append(pend, pq.pcm...)
			if !sonar(false) {
				return
			}
		case 'V':
			mu.Lock()
			rl, rec := reloj, recibido
			mu.Unlock()
			if pq.ts+150 < rl { // ya llega tarde
				saltadas++
				continue
			}
			if enviado-rec > ventanaVideo { // el wifi no da para todo
				saltadas++
				if time.Since(ultimoSalto) > 500*time.Millisecond {
					calidad = max(25, calidad-5)
					ultimoSalto = time.Now()
				}
				continue
			}
			if calidad < 60 && time.Since(ultimoSalto) > 3*time.Second && time.Since(ultimaSubida) > 3*time.Second {
				calidad += 5
				ultimaSubida = time.Now()
			}
			if !enviar('V', pq.ts, jpegYUV(pq.yuv, w, h, calidad)) {
				return
			}
			imagenes++
		}
	}
}

// jpegYUV: una imagen YUV 4:2:0 en JPEG (el de Go ya lo hace en 4:2:0 y baseline).
func jpegYUV(yuv []byte, w, h, calidad int) []byte {
	cw, ch := (w+1)/2, (h+1)/2
	if len(yuv) < w*h+2*cw*ch {
		return nil
	}
	im := &image.YCbCr{
		Y: yuv[:w*h], Cb: yuv[w*h : w*h+cw*ch], Cr: yuv[w*h+cw*ch : w*h+2*cw*ch],
		YStride: w, CStride: cw, SubsampleRatio: image.YCbCrSubsampleRatio420, Rect: image.Rect(0, 0, w, h),
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, im, &jpeg.Options{Quality: calidad})
	return b.Bytes()
}

// ---- El vídeo de mentira (para las pruebas): 6 s de barras de colores y un pitido ---------------

type fuenteFalsa struct {
	desde, dur uint32
	tasa       int
	nv, na     int // imágenes y bloques de sonido ya dados
}

const (
	anchoFalso = 256
	altoFalso  = 144
	bloqueFalso = 400
)

func (f *fuenteFalsa) info() (uint32, int, int, bool) { return f.dur, anchoFalso, altoFalso, true }
func (f *fuenteFalsa) cerrar()                        {}

func (f *fuenteFalsa) leer() (*paqueteMedio, error) {
	tv := f.desde + uint32(f.nv*1000/fpsVideo)
	ta := f.desde + uint32(f.na*bloqueFalso*1000/f.tasa)
	if tv >= f.dur && ta >= f.dur {
		return nil, io.EOF
	}
	if ta <= tv || tv >= f.dur {
		pcm := make([]int16, bloqueFalso)
		base := int(f.desde)*f.tasa/1000 + f.na*bloqueFalso
		for i := range pcm {
			pcm[i] = int16(6000 * math.Sin(2*math.Pi*440*float64(base+i)/float64(f.tasa)))
		}
		f.na++
		return &paqueteMedio{tipo: 'A', ts: ta, pcm: pcm}, nil
	}
	f.nv++
	return &paqueteMedio{tipo: 'V', ts: tv, yuv: barras(anchoFalso, altoFalso, int(tv))}, nil
}

// barras: 8 barras de colores que se mueven con el tiempo, en YUV 4:2:0.
func barras(w, h, ms int) []byte {
	cw, ch := w/2, h/2
	b := make([]byte, w*h+2*cw*ch)
	colores := [8][3]byte{{235, 128, 128}, {210, 16, 146}, {170, 166, 16}, {145, 54, 34}, {106, 202, 222}, {81, 90, 240}, {41, 240, 110}, {16, 128, 128}}
	mover := ms / 20
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := colores[((x+mover)%w)*8/w]
			b[y*w+x] = c[0]
			if x%2 == 0 && y%2 == 0 {
				b[w*h+(y/2)*cw+x/2] = c[1]
				b[w*h+cw*ch+(y/2)*cw+x/2] = c[2]
			}
		}
	}
	return b
}

// ---- El del ayudante de Android -----------------------------------------------------------------

type fuenteAyudante struct {
	c      net.Conn
	r      *bufio.Reader
	dur    uint32
	w, h   int
	sonido bool
}

type infoVideo struct {
	Dur    uint32 `json:"dur"`
	W      int    `json:"w"`
	H      int    `json:"h"`
	Sonido bool   `json:"sonido"`
	Error  string `json:"error"`
}

func (a *ayudante) ver(fuente string, desde uint32, tasa int) (*fuenteAyudante, error) {
	c, r, err := a.abrir(fmt.Sprintf("VER\t%s\t%d\t%d\t%d\t%d\t%d\t%d", fuente, desde, anchoVideo, altoVideo, areaVideo, tasa, fpsVideo))
	if err != nil {
		return nil, err
	}
	_ = c.SetReadDeadline(time.Now().Add(60 * time.Second))
	l, err := r.ReadBytes('\n')
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("la app no abre el vídeo (%v)", err)
	}
	var in infoVideo
	if err := json.Unmarshal(l, &in); err != nil {
		c.Close()
		return nil, err
	}
	if in.Error != "" {
		c.Close()
		return nil, errors.New(in.Error)
	}
	if in.W <= 0 || in.H <= 0 || in.W > anchoVideo || in.H > altoVideo {
		c.Close()
		return nil, fmt.Errorf("la app da un tamaño raro (%d×%d)", in.W, in.H)
	}
	return &fuenteAyudante{c: c, r: r, dur: in.Dur, w: in.W, h: in.H, sonido: in.Sonido}, nil
}

func (f *fuenteAyudante) info() (uint32, int, int, bool) { return f.dur, f.w, f.h, f.sonido }
func (f *fuenteAyudante) cerrar()                        { f.c.Close() }

// leer: [tipo][largo: 4 bytes][ms: 4 bytes][largo bytes], en little endian.
func (f *fuenteAyudante) leer() (*paqueteMedio, error) {
	_ = f.c.SetReadDeadline(time.Now().Add(60 * time.Second))
	var cab [9]byte
	if _, err := io.ReadFull(f.r, cab[:]); err != nil {
		return nil, fmt.Errorf("la app ha dejado de mandar el vídeo (%v)", err)
	}
	n := int(binary.LittleEndian.Uint32(cab[1:]))
	ts := binary.LittleEndian.Uint32(cab[5:])
	if n < 0 || n > 4<<20 {
		return nil, errors.New("la app manda algo raro")
	}
	d := make([]byte, n)
	if _, err := io.ReadFull(f.r, d); err != nil {
		return nil, fmt.Errorf("la app ha dejado de mandar el vídeo (%v)", err)
	}
	switch cab[0] {
	case 'V':
		cw, ch := (f.w+1)/2, (f.h+1)/2
		if n != f.w*f.h+2*cw*ch {
			return nil, errors.New("la app manda una imagen de otro tamaño")
		}
		return &paqueteMedio{tipo: 'V', ts: ts, yuv: d}, nil
	case 'A':
		pcm := make([]int16, n/2)
		for i := range pcm {
			pcm[i] = int16(binary.LittleEndian.Uint16(d[i*2:]))
		}
		return &paqueteMedio{tipo: 'A', ts: ts, pcm: pcm}, nil
	case 'F':
		return nil, io.EOF
	case 'E':
		return nil, errors.New(string(d))
	}
	return nil, fmt.Errorf("la app manda un paquete raro (%q)", cab[0])
}
