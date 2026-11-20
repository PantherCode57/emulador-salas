package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// medirTiempos (instagram -medir): cuánto tarda cada cosa con tu sesión, sin pantalla. Primero como
// al abrir la app (el inicio, las historias y los mensajes a la vez, con las fotos que pide la app);
// después cada cosa por separado. Solo escribe tiempos y cuántas cosas: nada de lo que hay dentro.
// Con INSTAGRAM_MEDIR_REGISTRO=1 también sale el registro (por dónde va cada cosa).
func medirTiempos(f *fuentes) {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay sesión guardada en", carpetaDatos)
		return
	}
	c.ruta = "" // no se guarda nada de la cuenta
	log.SetFlags(log.Lmicroseconds)
	if os.Getenv("INSTAGRAM_MEDIR_REGISTRO") == "1" {
		log.SetOutput(os.Stdout)
	} else {
		log.SetOutput(io.Discard)
	}
	redOK.Store(true)
	s := func(d time.Duration) string { return fmt.Sprintf("%.2f s", d.Seconds()) }
	ig := nuevoInstagram(c)
	medirAbrir("abrir la app, sin tokens", ig)
	ig2 := nuevoInstagram(c) // como al abrirla otra vez: con los tokens de web.json
	ig2.web = ig.web
	medirAbrir("abrir la app, con los tokens guardados", ig2)

	// ---- Las páginas de Instagram (de cualquiera salen los tokens de GraphQL) ----
	for _, r := range []string{"/", rutaMensajes, "/reels/"} {
		t := time.Now()
		b, err := ig.bajarPagina(r)
		fmt.Printf("bajar la página %s: %s (%s, %d KB)\n", r, s(time.Since(t)), siError(err), len(b)>>10)
	}

	// ---- Cada página de cada pestaña ----
	medirPag := func(nombre string, fn func() (Pagina, error)) Pagina {
		t := time.Now()
		p, err := fn()
		fmt.Printf("%s: %s (%s, %d)\n", nombre, s(time.Since(t)), siError(err), len(p.Posts))
		return p
	}
	p := medirPag("inicio, otra vez", func() (Pagina, error) { return ig.Inicio("") })
	if p.Mas {
		medirPag("inicio, página siguiente", func() (Pagina, error) { return ig.Inicio(p.Cursor) })
	}
	reels := medirPag("reels", func() (Pagina, error) { return ig.Reels("") })
	if reels.Mas {
		medirPag("reels, página siguiente", func() (Pagina, error) { return ig.Reels(reels.Cursor) })
	}

	// ---- Fotos, de una en una ----
	var bajar, escala []time.Duration
	for _, x := range p.Posts {
		if len(bajar) >= 6 || x.Medios[0].Foto == "" {
			continue
		}
		t := time.Now()
		im, err := bajarImagen(x.Medios[0].Foto)
		if err != nil {
			continue
		}
		bajar = append(bajar, time.Since(t))
		t = time.Now()
		b := im.Bounds()
		if b.Dx() > 480 || b.Dy() > 480 {
			im = escalar(im, 480, max(1, 480*b.Dy()/max(b.Dx(), 1)), false)
		}
		_ = escalar(im, ancho, 380, false)
		escala = append(escala, time.Since(t))
	}
	fmt.Printf("una foto: bajarla y descodificarla %s, reducirla %s (de %d)\n", media(bajar), media(escala), len(bajar))

	// ---- El primer reel: hasta la primera imagen de ffmpeg ----
	if len(reels.Posts) > 0 {
		md := reels.Posts[0].Medios[0]
		w, h := encajar(md.W, md.H, anchoVert, altoVert)
		vf := fmt.Sprintf("fps=30,scale=%d:%d:force_original_aspect_ratio=decrease:flags=fast_bilinear,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,format=bgr0", w, h, w, h)
		cab := map[string]string{"User-Agent": agenteNavegador}
		probar := func(nombre string, extra []string, ruta string) {
			args := append([]string{"-nostdin", "-hide_banner", "-loglevel", "error"}, extra...)
			args = append(args, argsEntrada(ruta, cab, 0, true)...)
			args = append(args, "-map", "0:v:0", "-an", "-vf", vf, "-f", "rawvideo", "pipe:1")
			d, err := primerMarco(args, w*h*4)
			fmt.Printf("primer reel, %s: %s (%s)\n", nombre, s(d), siError(err))
		}
		elegido := sondeo
		for _, v := range []struct {
			n string
			s []string
		}{{"con el sondeo de siempre (5 MB, 5 s)", []string{"-probesize", "5000000", "-analyzeduration", "5000000"}},
			{"con 64 KB y 0,1 s", []string{"-probesize", "65536", "-analyzeduration", "100000"}},
			{"con 32 KB y 0,1 s, sin fps", []string{"-probesize", "32768", "-analyzeduration", "100000", "-fpsprobesize", "0"}}} {
			sondeo = v.s
			probar(v.n, nil, md.Video)
		}
		sondeo = elegido
		probar("como queda (otra vez)", nil, md.Video)
		if len(reels.Posts) > 1 { // bajado antes (como si se hubiera adelantado mientras se veía el anterior)
			md2 := reels.Posts[1].Medios[0]
			t := time.Now()
			ruta, n, err := bajarATemporal(md2.Video)
			fmt.Printf("bajar entero el reel siguiente: %s (%d KB, %s)\n", s(time.Since(t)), n>>10, siError(err))
			if err == nil {
				w, h = encajar(md2.W, md2.H, anchoVert, altoVert)
				vf = fmt.Sprintf("fps=30,scale=%d:%d:force_original_aspect_ratio=decrease:flags=fast_bilinear,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,format=bgr0", w, h, w, h)
				probar("ya bajado", nil, ruta)
				// como en la app: su ffmpeg arrancado mientras se ve el anterior
				r := nuevoReproductorVideo(func([]byte, int, int) {}, func(int) {}, func(int, error) {})
				if p, err := r.lanzar(fuenteDe(md2, ruta, anchoVert, altoVert), 0); err != nil {
					fmt.Println("preparar ffmpeg:", siError(err))
				} else {
					time.Sleep(3 * time.Second)
					t := time.Now()
					go func() { _, _ = io.Copy(io.Discard, p.ar) }() // el sonido, como haría aplay
					_, err := io.ReadFull(p.vr, make([]byte, p.w*p.h*4))
					fmt.Printf("primer reel, ya bajado y con su ffmpeg preparado: %s (%s)\n", s(time.Since(t)), siError(err))
					r.prep = p
					r.descartar()
				}
				os.Remove(ruta)
			}
		}
	}

	// ---- Los reels como en la app: el reproductor de verdad, sin sonido ----
	if len(reels.Posts) > 2 {
		medirReproductor(reels.Posts[1].Medios[0], reels.Posts[2].Medios[0])
	}

	// ---- Abrir las historias de una persona y los mensajes ----
	if bs, err := ig.Historias(); err == nil && len(bs) > 1 {
		t := time.Now()
		items, err := ig.HistoriasDe(bs[1].ID)
		d := time.Since(t)
		tf := time.Duration(0)
		if err == nil && len(items) > 0 && items[0].Medios[0].Foto != "" {
			t = time.Now()
			if _, e := bajarImagen(items[0].Medios[0].Foto); e == nil {
				tf = time.Since(t)
			}
		}
		fmt.Printf("abrir historias: %s (%s, %d), su primera foto %s más\n", s(d), siError(err), len(items), s(tf))
	}
	if cs, err := ig.Chats(); err == nil && len(cs) > 0 {
		t := time.Now()
		ms, _, err := ig.Conversacion(cs[0].ID, "")
		fmt.Printf("abrir una conversación: %s (%s, %d)\n", s(time.Since(t)), siError(err), len(ms))
	}

	// ---- Pintar (con datos inventados): lo que cuesta cada repintado ----
	a, _, _, _ := appDePrueba(f)
	buf := make([]byte, ancho*alto*4)
	for _, pt := range []struct {
		nombre string
		p      pantalla
	}{{"inicio", pInicio}, {"historias", pHistorias}, {"mensajes", pMensajes}} {
		a.pant = pt.p
		a.dibujar()
		a.l.marcoEn(buf)
		var ds []time.Duration
		for i := 0; i < 15; i++ {
			t := time.Now()
			a.dibujar()
			a.l.marcoEn(buf)
			ds = append(ds, time.Since(t))
		}
		fmt.Printf("pintar %s: %s cada vez\n", pt.nombre, media(ds))
	}
}

// medirAbrir: como al abrir la app (el inicio, las historias y los mensajes a la vez, con las fotos que
// pide la app), hasta la primera foto del inicio y hasta que llegan todas las que se han pedido.
func medirAbrir(nombre string, ig *instagram) {
	s := func(d time.Duration) string { return fmt.Sprintf("%.2f s", d.Seconds()) }
	img := nuevasImagenes(nil)
	t0 := time.Now()
	var wg sync.WaitGroup
	var tIni, tHist, tChats time.Duration
	var ini Pagina
	var nHist, nChats int
	var errIni, errHist, errChats error
	listoIni := make(chan struct{})
	wg.Add(3)
	go func() {
		defer wg.Done()
		defer close(listoIni)
		ini, errIni = ig.Inicio("")
		tIni = time.Since(t0)
	}()
	go func() {
		defer wg.Done()
		bs, err := ig.Historias()
		tHist, errHist, nHist = time.Since(t0), err, len(bs)
		for i, b := range bs { // como la app
			if i < carasAdelantadas {
				img.adelantar(b.Foto)
			}
		}
	}()
	go func() {
		defer wg.Done()
		cs, err := ig.Chats()
		tChats, errChats, nChats = time.Since(t0), err, len(cs)
		for _, c := range cs {
			img.adelantar(c.Foto)
		}
	}()
	<-listoIni
	tFoto := time.Duration(0)
	if errIni == nil && len(ini.Posts) > 0 {
		p := ini.Posts[0]
		for k := 1; k <= 3 && k < len(ini.Posts); k++ { // como adelantarInicio
			img.adelantar(ini.Posts[k].Medios[0].Foto)
			img.adelantar(ini.Posts[k].FotoUsuario)
		}
		fin := time.Now().Add(60 * time.Second)
		for time.Now().Before(fin) {
			if img.pedir(p.Medios[0].Foto, ancho, 380, false, 0) != nil {
				tFoto = time.Since(t0)
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wg.Wait()
	fmt.Printf("%s: inicio %s (%s, %d), su primera foto %s; historias %s (%s, %d); mensajes %s (%s, %d)\n", nombre,
		s(tIni), siError(errIni), len(ini.Posts), s(tFoto), s(tHist), siError(errHist), nHist, s(tChats), siError(errChats), nChats)
	fin := time.Now().Add(90 * time.Second)
	for time.Now().Before(fin) {
		img.mu.Lock()
		quedan := len(img.pedidas) - len(img.orig)
		img.mu.Unlock()
		if quedan <= 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	img.mu.Lock()
	nFotos := len(img.orig)
	img.mu.Unlock()
	fmt.Printf("  fotos pedidas al abrir: %d, todas a los %s\n", nFotos, s(time.Since(t0)))
}

// medirReproductor: un reel en frío y, mientras «se ve», el siguiente bajado y con su ffmpeg preparado
// (como ponerReel); luego se pasa a él. Con un aplay de mentira que tira el sonido (no suena nada).
func medirReproductor(md1, md2 Medio) {
	s := func(d time.Duration) string { return fmt.Sprintf("%.2f s", d.Seconds()) }
	falso := filepath.Join(os.TempDir(), "instagram-medir-aplay")
	// tira el sonido al ritmo de verdad (44100 × 2 × 2 bytes por segundo), como aplay
	guion := "#!/bin/sh\nwhile dd bs=17640 count=1 iflag=fullblock of=/dev/null status=none; do sleep 0.1; done\n"
	if err := os.WriteFile(falso, []byte(guion), 0o700); err != nil {
		fmt.Println("reproductor: no se puede probar sin sonido:", err)
		return
	}
	defer os.Remove(falso)
	antes := aplayBin
	aplayBin = falso
	defer func() { aplayBin = antes }()

	primera := make(chan struct{}, 8)
	r := nuevoReproductorVideo(func([]byte, int, int) {
		select {
		case primera <- struct{}{}:
		default:
		}
	}, func(int) {}, func(_ int, err error) { fmt.Println("reproductor:", siError(err)) })
	defer r.cerrar()
	ad := nuevosAdelantos()
	defer ad.limpiar()
	esperar := func() (time.Duration, bool) {
		t := time.Now()
		select {
		case <-primera:
			return time.Since(t), true
		case <-time.After(20 * time.Second):
			return time.Since(t), false
		}
	}
	vaciar := func() {
		for len(primera) > 0 {
			<-primera
		}
	}
	if _, err := r.tocar(fuenteDe(md1, md1.Video, anchoVert, altoVert), 0); err != nil {
		fmt.Println("reproductor:", siError(err))
		return
	}
	d, ok := esperar()
	fmt.Printf("reproductor, un reel sin preparar: primera imagen a los %s (%v)\n", s(d), ok)
	r.mu.Lock()
	m0 := r.marcos
	r.mu.Unlock()
	time.Sleep(3 * time.Second)
	r.mu.Lock()
	fmt.Printf("reproductor: %d imágenes en 3 s sin nada más (a 30 por segundo, 90)\n", r.marcos-m0)
	r.mu.Unlock()
	listo := make(chan string, 1)
	ad.pedir(md2.Video, func(ruta string) { listo <- ruta })
	select {
	case ruta := <-listo:
		r.mu.Lock()
		antesPrep := r.marcos
		r.mu.Unlock()
		r.preparar(fuenteDe(md2, ruta, anchoVert, altoVert))
		time.Sleep(3 * time.Second) // se sigue viendo el de antes
		r.mu.Lock()
		durante := r.marcos - antesPrep
		r.mu.Unlock()
		fmt.Printf("reproductor: mientras se preparaba el siguiente, %d imágenes del de ahora en 3 s (a 30 por segundo, 90)\n", durante)
		vaciar()
		if _, err := r.tocar(fuenteDe(md2, ad.ruta(md2.Video), anchoVert, altoVert), 0); err != nil {
			fmt.Println("reproductor:", siError(err))
			return
		}
		d, ok = esperar()
		fmt.Printf("reproductor, el siguiente (bajado y preparado): primera imagen a los %s (%v)\n", s(d), ok)
		time.Sleep(time.Second) // que siga un poco, para ver que suena y no se para
		r.mu.Lock()
		marcos := r.marcos
		r.mu.Unlock()
		fmt.Printf("reproductor: %d imágenes del siguiente en el primer segundo\n", marcos)
	case <-time.After(60 * time.Second):
		fmt.Println("reproductor: el siguiente no se ha bajado en 60 s")
	}
}

// media: la mediana de unos tiempos.
func media(ds []time.Duration) string {
	if len(ds) == 0 {
		return "-"
	}
	c := append([]time.Duration(nil), ds...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	m := c[len(c)/2]
	if m < time.Second {
		return fmt.Sprintf("%d ms", m.Milliseconds())
	}
	return fmt.Sprintf("%.2f s", m.Seconds())
}

// primerMarco: cuánto tarda ffmpeg en sacar la primera imagen entera.
func primerMarco(args []string, tam int) (time.Duration, error) {
	cmd := exec.Command(ffmpegBin, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	t := time.Now()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	_, err = io.ReadFull(out, make([]byte, tam))
	return time.Since(t), err
}

// bajarATemporal baja un vídeo entero a la memoria (/dev/shm) para medir.
func bajarATemporal(u string) (string, int64, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", agenteNavegador)
	resp, err := clienteImagen.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	fh, err := os.CreateTemp("/dev/shm", "instagram-medir-*.mp4")
	if err != nil {
		return "", 0, err
	}
	defer fh.Close()
	n, err := io.Copy(fh, io.LimitReader(resp.Body, 64<<20))
	return fh.Name(), n, err
}
