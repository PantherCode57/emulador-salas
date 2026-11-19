// Instagram para la consola (Panther OS: dArkOS4Clone en la XF35H): tu inicio, los reels, las
// historias y tus mensajes, con tus «me gusta» y para contestar.
//
// Habla con Instagram como su web (www.instagram.com) con tu sesión, que se pasa desde el PC (ver
// web.go). Las fotos se bajan solo a la memoria. Los vídeos los lee ffmpeg directamente de su
// dirección (imágenes para la pantalla y sonido para aplay, como en el YouTube de la consola).
// La pantalla es el emulador de Panther (o RetroArch con el núcleo «Pokemulador»), como en las
// otras apps: este programa le manda las imágenes y él le pasa los botones.
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
// Con -probar dice, con la sesión guardada, qué contesta Instagram a cada cosa (solo cuántas y si
// ha ido bien; nada de lo que hay dentro). Con -probar -js, antes busca los doc_id en los JS de la web.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	carpetaDatos   = entorno("INSTAGRAM_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "instagram"))
	retroarchBin   = entorno("CONSOLA_RETROARCH", "retroarch")
	nucleoPantalla = entorno("CONSOLA_NUCLEO", "pokemulador_libretro.so")
)

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func logf(format string, args ...any) { log.Printf(format, args...) }

func itoa(n int) string { return strconv.Itoa(n) }

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	probar := flag.Bool("probar", false, "sin pantalla: con la sesión guardada, dice qué contesta Instagram (solo cuántas)")
	js := flag.Bool("js", false, "con -probar: antes busca los doc_id en los JS de la web de Instagram")
	megusta := flag.Bool("megusta", false, "con -probar: da «me gusta» a una publicación de tu inicio y lo quita enseguida (cambia tu cuenta)")
	enviar := flag.String("enviar", "", "con -probar y -chat: manda este texto a ese chat (cambia tu cuenta)")
	chat := flag.String("chat", "", "con -probar -enviar: el nombre del chat, como sale en la consola")
	flag.Parse()
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 640 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 400 {
		alto = v
	}
	if *probar && (*megusta || *enviar != "") {
		probarAcciones(*megusta, *enviar, *chat)
		return
	}
	if *probar {
		probarSesion(*js)
		return
	}
	f := cargarFuentes()
	if *prueba != "" {
		if err := dibujarPruebas(f, *prueba); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := os.MkdirAll(carpetaDatos, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if reg, err := os.OpenFile(filepath.Join(carpetaDatos, "registro.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
	}
	log.Printf("Instagram de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	a.ig = nuevoInstagram(cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json")))
	a.img = nuevasImagenes(a)
	a.rep = nuevoReproductorVideo(a.componer, a.videoAcabado, a.videoFallido)
	botones := make(chan uint16, 64)
	a.srv = nuevoServidorPantalla(botones)
	if err := a.srv.escuchar(); err != nil {
		log.Fatalf("no se pudo abrir la pantalla (¿hay otra app abierta?): %v", err)
	}
	// El logo ya mismo en la pantalla (EmulationStation la acaba de soltar) y como primer marco.
	a.dibujarCarga()
	pintarFB(a.l.img)
	a.srv.enviar(a.l.marco())

	ra, err := abrirRetroArch()
	if err != nil {
		log.Fatalf("no se pudo abrir la pantalla: %v", err)
	}
	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		log.Printf("señal %v: salgo", s)
		a.terminar()
	}()

	go vigilarRed(a)
	a.arrancar()
	a.bucle(botones, ra)

	a.rep.cerrar()
	a.web.cerrar()
	ra.cerrar()
	a.cargaHasta = time.Now().Add(time.Hour)
	a.l.limpiarFondo()
	a.dibujarCarga()
	pintarFB(a.l.img)
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// probarSesion: con la sesión de la consola, qué contesta Instagram a cada pestaña (cuántas cosas,
// si van con foto o vídeo y por dónde han ido). No escribe nada de lo que hay dentro (ni nombres ni
// textos ni tokens). Con js, antes busca los doc_id en los JS de la web (y los guarda en docs.json).
// Los «me gusta» y mandar mensajes no se prueban: cambian tu cuenta.
func probarSesion(js bool) {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay sesión guardada en", carpetaDatos)
		return
	}
	c.ruta = "" // no se guarda nada de la cuenta al probar
	ig := nuevoInstagram(c)
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
	_, _, _, err := ig.QuienSoy()
	fmt.Println("quién soy:", siError(err))
	for _, r := range []string{"/", rutaMensajes} {
		d, err := ig.pagina(r, false)
		if err != nil {
			fmt.Printf("página %s: %s\n", r, siError(err))
			continue
		}
		fmt.Printf("página %s: bien (lsd %v, fb_dtsg %v, versión %v, aparato %v, %d JS)\n", r, d.lsd != "", d.dtsg != "",
			d.hs != "" && d.spinR != "", d.dispDirect != "", len(d.scripts))
	}
	if js {
		nombres := make([]string, 0, len(docsIncluidos))
		for n := range docsIncluidos {
			nombres = append(nombres, n)
		}
		sort.Strings(nombres)
		for _, r := range []string{"/", rutaMensajes} {
			ig.muDocs.Lock()
			ig.buscado = nil
			ig.muDocs.Unlock()
			var faltan []string
			for _, n := range nombres {
				if ig.cargarDocs().Docs[n] == "" || time.Since(ig.cargarDocs().Cuando) > time.Minute {
					faltan = append(faltan, n)
				}
			}
			n, err := ig.descubrir(r, faltan...)
			fmt.Printf("doc_id en los JS de %s: %s, %d de %d\n", r, siError(err), n, len(faltan))
		}
		for _, n := range nombres {
			web := ig.cargarDocs().Docs[n]
			estado := "no está en los JS"
			switch {
			case web == docsIncluidos[n]:
				estado = "el incluido"
			case web != "":
				estado = "otro (" + web + ")"
			}
			fmt.Printf("  %s: %s, %d variables\n", n, estado, len(ig.cargarDocs().Prov[n]))
		}
	}
	contar := func(nombre string, p Pagina, err error) {
		fotos, videos, varias := 0, 0, 0
		for _, x := range p.Posts {
			if len(x.Medios) > 1 {
				varias++
			}
			if x.Medios[0].esVideo() {
				videos++
			} else if x.Medios[0].Foto != "" {
				fotos++
			}
		}
		fmt.Printf("%s: %s, %d (fotos %d, vídeos %d, de varias %d), sigue: %v\n", nombre, siError(err), len(p.Posts), fotos, videos, varias, p.Mas)
	}
	p, err := ig.Inicio("")
	contar("inicio", p, err)
	if err == nil && p.Mas {
		p2, err := ig.Inicio(p.Cursor)
		contar("inicio (más)", p2, err)
	}
	p, err = ig.Reels("")
	contar("reels", p, err)
	if err == nil && p.Mas {
		p2, err := ig.Reels(p.Cursor)
		contar("reels (más)", p2, err)
	}
	bs, err := ig.Historias()
	fmt.Printf("historias: %s, %d personas\n", siError(err), len(bs))
	for i := 0; err == nil && i < len(bs) && i < 2; i++ {
		items, err := ig.HistoriasDe(bs[i].ID)
		contar(fmt.Sprintf("historias de la %dª", i+1), Pagina{Posts: items}, err)
	}
	cs, err := ig.Chats()
	sin, conNombre, conFoto, conUltimo, conHora := 0, 0, 0, 0, 0
	for _, c := range cs {
		sin += uno(c.SinLeer)
		conNombre += uno(c.Titulo != "")
		conFoto += uno(c.Foto != "")
		conUltimo += uno(c.Ultimo != "" && !strings.HasSuffix(c.Ultimo, "[Mensaje]"))
		conHora += uno(!c.Cuando.IsZero())
	}
	fmt.Printf("chats: %s, %d (sin leer %d; con nombre %d, foto %d, último mensaje %d, hora %d)\n",
		siError(err), len(cs), sin, conNombre, conFoto, conUltimo, conHora)
	if err == nil && len(cs) > 0 {
		ms, sig, err := ig.Conversacion(cs[0].ID, "")
		mios, conQuien, conTexto, conHora := 0, 0, 0, 0
		for _, m := range ms {
			mios += uno(m.Mio)
			conQuien += uno(!m.Mio && m.Quien != "")
			conTexto += uno(m.Texto != "" && m.Texto != "[Mensaje]")
			conHora += uno(!m.Cuando.IsZero())
		}
		otros := map[string]bool{}
		for _, m := range ms {
			if !m.Mio {
				otros[m.Quien] = true
			}
		}
		fmt.Printf("mensajes del primero: %s, %d (tuyos %d; de otros con nombre %d, de %d personas; con texto %d, hora %d), hay de antes: %v\n",
			siError(err), len(ms), mios, conQuien, len(otros), conTexto, conHora, sig != "")
		if err == nil && sig != "" && len(ms) > 0 {
			ms2, _, err := ig.Conversacion(cs[0].ID, sig)
			viejos := len(ms2) > 0 && !ms2[len(ms2)-1].Cuando.After(ms[0].Cuando)
			fmt.Printf("los de antes: %s, %d, más viejos que los primeros: %v\n", siError(err), len(ms2), viejos)
		}
	}
	for _, n := range []string{"usePolarisLikeMediaXIGLikeMutation", "IGDirectTextSendMutation"} {
		cs := ig.candidatos(n)
		fmt.Printf("%s: no se prueba (cambia tu cuenta); %d doc_id para probar\n", n, len(cs))
	}
}

// probarAcciones: lo que cambia tu cuenta, solo si lo pides (-megusta, -enviar … -chat …). Tampoco
// escribe nada de lo que hay dentro: ni de quién es la publicación ni el chat.
func probarAcciones(megusta bool, texto, nombreChat string) {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay sesión guardada en", carpetaDatos)
		return
	}
	c.ruta = ""
	ig := nuevoInstagram(c)
	log.SetOutput(os.Stdout)
	log.SetFlags(0)
	if megusta {
		p, err := ig.Inicio("")
		var post *Post
		for i := range p.Posts {
			if x := &p.Posts[i]; x.PK != "" && !x.Sugerido && !x.MeGusta {
				post = x
				break
			}
		}
		switch {
		case err != nil:
			fmt.Println("me gusta: no se ha podido abrir el inicio:", siError(err))
		case post == nil:
			fmt.Printf("me gusta: en las %d primeras del inicio no hay ninguna sin tu «me gusta»\n", len(p.Posts))
		default:
			err := ig.MeGusta(post.PK, true)
			fmt.Println("me gusta:", siError(err))
			time.Sleep(3 * time.Second)
			err2 := ig.MeGusta(post.PK, false)
			fmt.Println("quitar el me gusta:", siError(err2))
			if err == nil && err2 != nil {
				fmt.Println("¡OJO! se ha quedado puesto: quítalo a mano o vuelve a probar")
			}
		}
	}
	if texto != "" {
		cs, err := ig.Chats()
		if err != nil {
			fmt.Println("enviar: no se han podido abrir los chats:", siError(err))
			return
		}
		quiero := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(nombreChat), "@"))
		var elegidos []Chat
		for _, x := range cs {
			if quiero != "" && strings.ToLower(x.Titulo) == quiero {
				elegidos = append(elegidos, x)
			}
		}
		if len(elegidos) != 1 {
			fmt.Printf("enviar: no se manda nada (%d chats con ese nombre entre los %d primeros)\n", len(elegidos), len(cs))
			return
		}
		err = ig.Enviar(elegidos[0].ID, texto)
		fmt.Println("enviar:", siError(err))
		if err == nil {
			time.Sleep(2 * time.Second)
			ms, _, err := ig.Conversacion(elegidos[0].ID, "")
			ultimo := len(ms) > 0 && ms[len(ms)-1].Mio && ms[len(ms)-1].Texto == texto
			fmt.Printf("en la conversación: %s, el último es el tuyo con ese texto: %v\n", siError(err), ultimo)
		}
	}
}

func uno(si bool) int {
	if si {
		return 1
	}
	return 0
}

func siError(err error) string {
	if err != nil {
		return "MAL (" + err.Error() + ")"
	}
	return "bien"
}
