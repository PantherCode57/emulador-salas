package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Vídeo: un solo ffmpeg lee el vídeo (de internet o descargado) y saca dos cosas a la vez:
//   - las imágenes, ya a 640×360 (los verticales, como los Shorts, a 270×480) y en el formato del
//     núcleo (B, G, R, X), por su salida normal;
//   - el sonido sin comprimir, por el descriptor 3, que va a aplay.
//
// aplay marca el paso: cuando su memoria está llena, ffmpeg espera. Cada imagen se enseña cuando el
// sonido llega a su momento; las que llegan tarde se saltan (mejor eso que separarse del sonido).
const (
	anchoVideo  = 640
	altoVideo   = 360
	anchoVert   = 270 // los vídeos verticales (Shorts), a toda la altura
	altoVert    = 480
	frecuencia  = 44100
	bytesPorSeg = frecuencia * 2 * 2 // estéreo, 16 bits
	ffmpegBin   = "ffmpeg"
	ytdlpBin    = "yt-dlp" // no se usa en Instagram (los vídeos llegan con su dirección)
	maxCola     = 40       // imágenes que pueden ir por delante del sonido (unos 37 MB)
	tuboSonido  = 16 << 10
)

// aplayBin: el que suena (en -medir se cambia por uno que no suena).
var aplayBin = "aplay"

// retrasoSonido: lo que tarda en oírse lo que se manda a aplay (su memoria, la del tubo y la de la
// tarjeta), menos lo que tarda RetroArch en enseñar una imagen. Se puede afinar sin compilar.
var retrasoSonido = time.Duration(entornoInt("INSTAGRAM_RETRASO_MS", 200)) * time.Millisecond

func entornoInt(clave string, defecto int) int {
	if n, err := strconv.Atoi(os.Getenv(clave)); err == nil {
		return n
	}
	return defecto
}

// fuenteVideo: de dónde sale el vídeo.
type fuenteVideo struct {
	Video     string            // archivo o dirección (con el sonido dentro si Audio == "")
	Audio     string            // dirección del sonido, si va aparte
	Cabeceras map[string]string // las que hay que mandar al pedirlo (las que dice yt-dlp)
	Duracion  time.Duration
	FPS       float64
	Vertical  bool   // más alto que ancho (los Shorts): se enseña a 270×480
	Titulo    string // lo que dice yt-dlp (los Shorts de la secuencia llegan sin título)
	Canal     string
	Vistas    string
	Tubo      []string // si hay: yt-dlp con estos argumentos lo saca por su salida (sin poder adelantar)
	AnchoSal  int      // si hay: el tamaño de las imágenes que salen (si no, 640×360 o 270×480)
	AltoSal   int
}

// tamano: el de las imágenes que salen de ffmpeg.
func (f *fuenteVideo) tamano() (int, int) {
	if f.AnchoSal > 0 && f.AltoSal > 0 {
		return f.AnchoSal, f.AltoSal
	}
	if f.Vertical {
		return anchoVert, altoVert
	}
	return anchoVideo, altoVideo
}

type reproductorVideo struct {
	mu     sync.Mutex
	aplay  *exec.Cmd
	salida *os.File
	ffmpeg *exec.Cmd
	tubo   *exec.Cmd
	fuente *fuenteVideo
	inicio time.Duration // dónde empezó el ffmpeg de ahora
	// Sonido mandado a aplay desde entonces. Cuando se acaba, el reloj sigue con la hora.
	escritos  int64
	finSonido bool
	finEn     time.Time
	relojFin  time.Duration
	pausado   bool
	turno     int // sube con cada vídeo o salto: los hilos del ffmpeg viejo se callan
	marcos    int // imágenes enseñadas en este turno
	libres    chan []byte
	tam       int        // bytes de cada imagen del vídeo de ahora
	prep      *preparado // el ffmpeg del vídeo que se va a ver después, ya arrancado

	// Avisos (desde otros hilos).
	alMarco  func(px []byte, w, h int) // una imagen que hay que enseñar ya
	alAcabar func(turno int)
	alFallar func(turno int, err error)
}

func nuevoReproductorVideo(alMarco func([]byte, int, int), alAcabar func(int), alFallar func(int, error)) *reproductorVideo {
	return &reproductorVideo{alMarco: alMarco, alAcabar: alAcabar, alFallar: alFallar, libres: make(chan []byte, maxCola+8)}
}

func (r *reproductorVideo) tomar(tam int) []byte {
	for {
		select {
		case b := <-r.libres:
			if len(b) == tam {
				return b
			}
		default:
			return make([]byte, tam)
		}
	}
}

func (r *reproductorVideo) devolver(b []byte) {
	r.mu.Lock()
	tam := r.tam
	r.mu.Unlock()
	if len(b) != tam {
		return
	}
	select {
	case r.libres <- b:
	default:
	}
}

func (r *reproductorVideo) abrirAplay() error {
	if r.aplay != nil {
		return nil
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.Command(aplayBin, "-q", "-t", "raw", "-f", "S16_LE", "-r", strconv.Itoa(frecuencia), "-c", "2",
		"--buffer-time=150000")
	cmd.Stdin = pr
	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return err
	}
	pr.Close()
	tamTubo(pw, tuboSonido) // poco tubo: el sonido se para enseguida al pausar y el reloj va más fino
	r.aplay, r.salida = cmd, pw
	go func() { _ = cmd.Wait() }()
	return nil
}

func (r *reproductorVideo) cerrarAplay() {
	if r.salida != nil {
		_ = r.salida.Close()
	}
	if r.aplay != nil && r.aplay.Process != nil {
		_ = r.aplay.Process.Kill()
	}
	r.aplay, r.salida = nil, nil
}

// tocar empieza un vídeo desde «desde». Devuelve su turno (el que traen los avisos de fin y fallo).
func (r *reproductorVideo) tocar(f *fuenteVideo, desde time.Duration) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(f.Tubo) > 0 {
		desde = 0
	}
	err := r.empezar(f, desde)
	return r.turno, err
}

// fpsSalida: las imágenes por segundo que se enseñan (los de 50 y 60 se quedan en la mitad).
func fpsSalida(fps float64) float64 {
	switch {
	case fps <= 0:
		return 30
	case fps > 31:
		return min(fps/2, 30)
	}
	return fps
}

// sondeo: cuánto lee ffmpeg del vídeo antes de empezar. Por defecto lee hasta 5 MB y 5 s para saber
// cómo es, y en la consola eso es más de un segundo; los MP4 de Instagram lo dicen todo al principio.
// (analyzeduration 0 sería «el de siempre», no «nada».)
var sondeo = []string{"-probesize", "65536", "-analyzeduration", "100000"}

func argsEntrada(ruta string, cab map[string]string, desde time.Duration, decodificar bool) []string {
	var args []string
	if strings.HasPrefix(ruta, "http") {
		args = append(args, "-reconnect", "1", "-reconnect_streamed", "1", "-reconnect_on_network_error", "1",
			"-reconnect_delay_max", "8", "-rw_timeout", "15000000")
		var b strings.Builder
		for k, v := range cab {
			if strings.EqualFold(k, "User-Agent") {
				args = append(args, "-user_agent", v)
				continue
			}
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
		if b.Len() > 0 {
			args = append(args, "-headers", b.String())
		}
	}
	if decodificar { // el vídeo: los 4 núcleos y los atajos de H.264 que casi no se notan
		args = append(args, sondeo...)
		args = append(args, "-threads", "4", "-flags2", "+fast")
		if os.Getenv("INSTAGRAM_RAPIDO") == "1" {
			args = append(args, "-skip_loop_filter", "all")
		}
	}
	if desde > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.2f", desde.Seconds()))
	}
	return append(args, "-i", ruta)
}

// preparado: un ffmpeg ya arrancado y esperando. En la consola, solo arrancar ffmpeg tarda 1,5 s
// (carga unas 200 bibliotecas): el del reel siguiente se arranca mientras se ve este, saca sus
// primeras imágenes y se queda parado hasta que se lean (sus tubos se llenan). Al pasar al siguiente,
// sale casi al momento.
type preparado struct {
	clave     string
	cmd, tubo *exec.Cmd
	vr, ar    *os.File
	errOut    io.ReadCloser
	fps       float64
	w, h      int
	cuando    time.Time
}

// claveFuente: qué vídeo y cómo (para saber si el preparado es el que se pide).
func claveFuente(f *fuenteVideo) string {
	w, h := f.tamano()
	return fmt.Sprintf("%s|%s|%dx%d|%g", f.Video, f.Audio, w, h, fpsSalida(f.FPS))
}

// lanzar arranca ffmpeg para f desde «desde», sin leer aún lo que saca.
func (r *reproductorVideo) lanzar(f *fuenteVideo, desde time.Duration) (*preparado, error) {
	fps := fpsSalida(f.FPS)
	w, h := f.tamano()
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	var tubo *exec.Cmd
	if len(f.Tubo) > 0 {
		tubo = exec.Command(ytdlpBin, f.Tubo...)
		args = append(args, "-threads", "4", "-flags2", "+fast", "-i", "pipe:0")
	} else {
		args = append(args, argsEntrada(f.Video, f.Cabeceras, desde, true)...)
		if f.Audio != "" {
			args = append(args, argsEntrada(f.Audio, f.Cabeceras, desde, false)...)
		}
	}
	vf := fmt.Sprintf("fps=%g,scale=%d:%d:force_original_aspect_ratio=decrease:flags=fast_bilinear,"+
		"pad=%d:%d:(ow-iw)/2:(oh-ih)/2,format=bgr0", fps, w, h, w, h)
	sonido := "0:a:0"
	if f.Audio != "" {
		sonido = "1:a:0"
	}
	args = append(args, "-map", "0:v:0", "-vf", vf, "-f", "rawvideo", "pipe:1",
		"-map", sonido, "-f", "s16le", "-ar", strconv.Itoa(frecuencia), "-ac", "2", "pipe:3")

	vr, vw, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	ar, aw, err := os.Pipe()
	if err != nil {
		vr.Close()
		vw.Close()
		return nil, err
	}
	cmd := exec.Command(ffmpegBin, args...)
	cmd.Stdout = vw
	cmd.ExtraFiles = []*os.File{aw}
	errOut, _ := cmd.StderrPipe()
	if tubo != nil {
		in, err := tubo.StdoutPipe()
		if err != nil {
			return nil, err
		}
		cmd.Stdin = in
		if err := tubo.Start(); err != nil {
			return nil, err
		}
	}
	if err := cmd.Start(); err != nil {
		vr.Close()
		vw.Close()
		ar.Close()
		aw.Close()
		if tubo != nil && tubo.Process != nil {
			_ = tubo.Process.Kill()
		}
		return nil, err
	}
	vw.Close()
	aw.Close()
	tamTubo(vr, 1<<20)
	tamTubo(ar, 1<<20) // si no, el ffmpeg preparado se para con el sonido antes de sacar la primera imagen
	return &preparado{clave: claveFuente(f), cmd: cmd, tubo: tubo, vr: vr, ar: ar, errOut: errOut, fps: fps, w: w, h: h,
		cuando: time.Now()}, nil
}

// preparar deja arrancado el ffmpeg de un vídeo que se va a ver enseguida (desde el principio).
func (r *reproductorVideo) preparar(f *fuenteVideo) {
	if len(f.Tubo) > 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.prep != nil && r.prep.clave == claveFuente(f) {
		return
	}
	r.descartarSinCandado()
	p, err := r.lanzar(f, 0)
	if err != nil {
		logf("preparar un vídeo: %v", err)
		return
	}
	// con menos prioridad, para que no frene el vídeo que se está viendo (sus hilos, que crea después de
	// cargar las bibliotecas, la heredan)
	bajarPrioridad(p.cmd.Process.Pid)
	r.prep = p
	time.AfterFunc(3*time.Minute, func() { // si no se ha usado, que no se quede ahí esperando
		r.mu.Lock()
		if r.prep == p {
			r.descartarSinCandado()
		}
		r.mu.Unlock()
	})
}

// descartar: fuera el preparado (al salir de los reels, para no tener un ffmpeg esperando).
func (r *reproductorVideo) descartar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.descartarSinCandado()
}

func (r *reproductorVideo) descartarSinCandado() {
	p := r.prep
	if p == nil {
		return
	}
	r.prep = nil
	for _, c := range []*exec.Cmd{p.cmd, p.tubo} {
		if c != nil && c.Process != nil {
			_ = c.Process.Signal(syscall.SIGKILL)
		}
	}
	p.vr.Close()
	p.ar.Close()
	go func() {
		_, _ = io.Copy(io.Discard, p.errOut)
		_ = p.cmd.Wait()
		if p.tubo != nil {
			_ = p.tubo.Wait()
		}
	}()
}

func (r *reproductorVideo) empezar(f *fuenteVideo, desde time.Duration) error {
	r.pararFfmpeg()
	if err := r.abrirAplay(); err != nil {
		return err
	}
	var p *preparado
	if r.prep != nil && desde == 0 && r.prep.clave == claveFuente(f) && time.Since(r.prep.cuando) < 3*time.Minute {
		p, r.prep = r.prep, nil
		logf("vídeo: ya estaba preparado (desde hace %.1f s)", time.Since(p.cuando).Seconds())
	} else {
		if r.prep != nil && time.Since(r.prep.cuando) >= 3*time.Minute {
			r.descartarSinCandado()
		}
		var err error
		if p, err = r.lanzar(f, desde); err != nil {
			return err
		}
	}
	cmd, tubo, vr, ar, errOut, fps, w, h := p.cmd, p.tubo, p.vr, p.ar, p.errOut, p.fps, p.w, p.h
	r.ffmpeg, r.tubo, r.fuente, r.inicio, r.escritos, r.pausado = cmd, tubo, f, desde, 0, false
	r.finSonido, r.marcos, r.tam = false, 0, w*h*4
	r.turno++
	turno := r.turno
	ultimoError := &strings.Builder{}
	var muErr sync.Mutex
	go func() { // lo que diga ffmpeg, por si falla
		s := bufio.NewScanner(errOut)
		for s.Scan() {
			muErr.Lock()
			ultimoError.Reset()
			ultimoError.WriteString(s.Text())
			muErr.Unlock()
			logf("ffmpeg: %s", s.Text())
		}
	}()
	cola := make(chan []byte, maxCola)
	var hilos sync.WaitGroup
	hilos.Add(3)
	go func() { defer hilos.Done(); r.copiarSonido(ar, turno) }()
	go func() { defer hilos.Done(); r.leerVideo(vr, turno, cola, w*h*4) }()
	go func() { defer hilos.Done(); r.presentar(cola, turno, fps, desde, w, h) }()
	go func() {
		hilos.Wait()
		werr := cmd.Wait()
		if tubo != nil {
			_ = tubo.Wait()
		}
		r.mu.Lock()
		mio := r.turno == turno
		vistos, oido := r.marcos, r.escritos
		if mio {
			r.ffmpeg, r.tubo = nil, nil
		}
		r.mu.Unlock()
		if !mio {
			return
		}
		muErr.Lock()
		msg := ultimoError.String()
		muErr.Unlock()
		if vistos < 3 && oido < bytesPorSeg && (werr != nil || msg != "") {
			// casi no ha salido nada: no es el final, es que no se pudo abrir
			if msg == "" {
				msg = werr.Error()
			}
			r.alFallar(turno, errors.New(msg))
			return
		}
		r.alAcabar(turno)
	}()
	return nil
}

// copiarSonido pasa el sonido de ffmpeg a aplay (esperando mientras está en pausa).
func (r *reproductorVideo) copiarSonido(out *os.File, turno int) {
	defer out.Close()
	buf := make([]byte, 4096)
	for {
		n, err := out.Read(buf)
		if n > 0 {
			r.mu.Lock()
			for r.pausado && r.turno == turno {
				r.mu.Unlock()
				time.Sleep(20 * time.Millisecond)
				r.mu.Lock()
			}
			if r.turno != turno {
				r.mu.Unlock()
				return
			}
			if r.salida == nil { // aplay se cerró: otra vez
				if err := r.abrirAplay(); err != nil {
					logf("aplay: %v", err)
				}
			}
			salida := r.salida
			r.mu.Unlock()
			if salida != nil {
				if _, werr := salida.Write(buf[:n]); werr != nil {
					logf("aplay: %v", werr)
					r.mu.Lock()
					r.cerrarAplay()
					r.mu.Unlock()
				}
			}
			r.mu.Lock()
			if r.turno == turno {
				r.escritos += int64(n)
			}
			r.mu.Unlock()
		}
		if err != nil {
			r.mu.Lock()
			if r.turno == turno && !r.finSonido { // se acabó el sonido: el reloj sigue con la hora
				r.relojFin = r.relojSinCandado()
				r.finSonido, r.finEn = true, time.Now()
			}
			r.mu.Unlock()
			return
		}
	}
}

// leerVideo saca las imágenes de ffmpeg a la cola. Nunca espera: si la cola está llena, se tira la
// más vieja (si ffmpeg se parase aquí, también se pararía el sonido).
func (r *reproductorVideo) leerVideo(vr *os.File, turno int, cola chan []byte, tam int) {
	defer close(cola)
	defer vr.Close()
	for {
		px := r.tomar(tam)
		if _, err := io.ReadFull(vr, px); err != nil {
			r.devolver(px)
			return
		}
		r.mu.Lock()
		viejo := r.turno != turno
		r.mu.Unlock()
		if viejo {
			r.devolver(px)
			return
		}
		select {
		case cola <- px:
		default:
			select {
			case v := <-cola:
				r.devolver(v)
			default:
			}
			cola <- px
		}
	}
}

// presentar enseña cada imagen en su momento. La primera, en cuanto llega (al adelantar en pausa
// se ve dónde se ha quedado).
func (r *reproductorVideo) presentar(cola <-chan []byte, turno int, fps float64, desde time.Duration, w, h int) {
	paso := time.Duration(float64(time.Second) / fps)
	var anterior []byte
	n := 0
	for px := range cola {
		t := desde + time.Duration(n)*paso
		n++
		tarde := false
		for n > 1 {
			r.mu.Lock()
			if r.turno != turno {
				r.mu.Unlock()
				return
			}
			ahora, pausado := r.relojSinCandado(), r.pausado
			r.mu.Unlock()
			if pausado {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			if ahora >= t {
				tarde = ahora > t+2*paso
				break
			}
			time.Sleep(min(t-ahora, 20*time.Millisecond))
		}
		r.mu.Lock()
		mio := r.turno == turno
		r.mu.Unlock()
		if !mio {
			return
		}
		if tarde && len(cola) > 0 { // va con retraso: esta se salta
			r.devolver(px)
			continue
		}
		r.alMarco(px, w, h)
		r.mu.Lock()
		r.marcos++
		r.mu.Unlock()
		if anterior != nil {
			r.devolver(anterior)
		}
		anterior = px
	}
}

// relojSinCandado: por dónde va lo que se oye (con r.mu cogido).
func (r *reproductorVideo) relojSinCandado() time.Duration {
	if r.finSonido {
		d := r.relojFin
		if !r.pausado {
			d += time.Since(r.finEn)
		}
		return d
	}
	return r.inicio + time.Duration(r.escritos)*time.Second/bytesPorSeg - retrasoSonido
}

func (r *reproductorVideo) pararFfmpeg() {
	r.turno++
	for _, c := range []*exec.Cmd{r.ffmpeg, r.tubo} {
		if c != nil && c.Process != nil {
			_ = c.Process.Signal(syscall.SIGKILL)
		}
	}
	r.ffmpeg, r.tubo = nil, nil
}

// parar: fuera el vídeo.
func (r *reproductorVideo) parar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pararFfmpeg()
	r.fuente = nil
	r.pausado = false
}

func (r *reproductorVideo) cerrar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.descartarSinCandado()
	r.pararFfmpeg()
	r.fuente = nil
	r.cerrarAplay()
}

func (r *reproductorVideo) pausar(p bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil || r.pausado == p {
		return
	}
	if r.finSonido { // el reloj de la hora también se para
		if p {
			r.relojFin += time.Since(r.finEn)
		} else {
			r.finEn = time.Now()
		}
	}
	r.pausado = p
}

func (r *reproductorVideo) enPausa() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pausado
}

func (r *reproductorVideo) abierto() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fuente != nil
}

// vertical: el vídeo de ahora es de los verticales (los Shorts).
func (r *reproductorVideo) vertical() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fuente != nil && r.fuente.Vertical
}

// hayImagen: ya se ha enseñado alguna imagen de este vídeo (o de este salto).
func (r *reproductorVideo) hayImagen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.marcos > 0
}

func (r *reproductorVideo) posicion() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil {
		return 0
	}
	return max(r.relojSinCandado(), r.inicio)
}

func (r *reproductorVideo) duracion() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil {
		return 0
	}
	return r.fuente.Duracion
}

// saltar: adelanta o atrasa (un ffmpeg nuevo desde ahí). Devuelve el turno nuevo, o false si este
// vídeo no se puede adelantar.
func (r *reproductorVideo) saltar(d time.Duration) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fuente == nil || len(r.fuente.Tubo) > 0 {
		return r.turno, false
	}
	pos := max(r.relojSinCandado(), r.inicio) + d
	if dur := r.fuente.Duracion; dur > 0 && pos > dur-3*time.Second {
		pos = dur - 3*time.Second
	}
	pos = max(pos, 0)
	pausado := r.pausado
	if err := r.empezar(r.fuente, pos); err != nil {
		logf("saltar: %v", err)
		return r.turno, false
	}
	r.pausado = pausado
	return r.turno, true
}
