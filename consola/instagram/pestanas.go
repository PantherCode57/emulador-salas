package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---- Inicio --------------------------------------------------------------------------------------

func (a *app) cargarInicio(mas bool) {
	t := &a.inicio
	if t.cargando || (mas && !t.mas) {
		return
	}
	cursor := ""
	if mas {
		cursor = t.cursor
	}
	t.cargando, t.err = true, ""
	go func() {
		desde := time.Now()
		p, err := a.ig.Inicio(cursor)
		tarda := time.Since(desde).Seconds()
		a.hacer(func() {
			t.cargando, t.cargada = false, true
			if err != nil {
				logf("inicio: %v", err)
				t.err = a.fallo(err)
				return
			}
			if !mas {
				t.posts, a.selInicio = nil, 0
			}
			t.posts = append(t.posts, sinRepetir(t.posts, p.Posts)...)
			t.cursor, t.mas = p.Cursor, p.Mas
			logf("inicio: %d publicaciones (%d en total) en %.1f s", len(p.Posts), len(t.posts), tarda)
			a.adelantarInicio()
			if !mas { // los reels, un poco después, para que la pestaña salga ya con el primero
				time.AfterFunc(4*time.Second, func() { a.hacer(a.adelantarReels) })
			}
		})
	}()
}

func sinRepetir(hay, nuevos []Post) []Post {
	visto := map[string]bool{}
	for _, p := range hay {
		visto[p.PK] = true
	}
	var r []Post
	for _, p := range nuevos {
		if !visto[p.PK] {
			visto[p.PK] = true
			r = append(r, p)
		}
	}
	return r
}

// postInicio: la publicación elegida del inicio (nil si no hay).
func (a *app) postInicio() *Post {
	if a.selInicio < 0 || a.selInicio >= len(a.inicio.posts) {
		return nil
	}
	return &a.inicio.posts[a.selInicio]
}

func (a *app) medioDe(p *Post) (Medio, int) {
	i := min(a.fotoDe[p.PK], len(p.Medios)-1)
	return p.Medios[max(i, 0)], max(i, 0)
}

// adelantarInicio pide ya las fotos de las siguientes publicaciones.
func (a *app) adelantarInicio() {
	for k := a.selInicio + 1; k <= a.selInicio+3 && k < len(a.inicio.posts); k++ {
		p := a.inicio.posts[k]
		a.img.adelantar(p.Medios[0].Foto)
		a.img.adelantar(p.FotoUsuario)
	}
	if p := a.postInicio(); p != nil {
		for _, m := range p.Medios {
			a.img.adelantar(m.Foto)
		}
	}
}

func (a *app) pulsarInicio(b uint16) {
	t := &a.inicio
	p := a.postInicio()
	switch b {
	case bArriba:
		a.selInicio = max(a.selInicio-1, 0)
	case bAbajo:
		if a.selInicio+1 < len(t.posts) {
			a.selInicio++
		}
		if a.selInicio >= len(t.posts)-3 {
			a.cargarInicio(true)
		}
	case bIzquierda, bDerecha:
		if p == nil || len(p.Medios) < 2 {
			break
		}
		_, i := a.medioDe(p)
		if b == bIzquierda {
			i = max(i-1, 0)
		} else {
			i = min(i+1, len(p.Medios)-1)
		}
		a.fotoDe[p.PK] = i
	case bA:
		if p == nil {
			if t.err != "" {
				a.cargarInicio(false)
			}
			break
		}
		if m, _ := a.medioDe(p); m.esVideo() {
			a.verVideo(*p, m)
		}
	case bY:
		a.meGusta(p)
	case bX:
		a.inicio = tira{}
		a.selInicio = 0
		a.cargarInicio(false)
		a.aviso("Actualizando tu inicio…")
	case bB:
		a.terminar()
	}
	a.adelantarInicio()
}

// Dónde va el vídeo grande del inicio: empieza por debajo del recuadro de la batería y la hora de
// Panther (arriba a la derecha, hasta y = 26) para no quedar tapado por él.
const (
	yVideoGrande    = 30
	altoVideoGrande = 378
)

// verVideo: un vídeo del inicio, en grande (vuelve a empezar al acabar, como en el móvil).
func (a *app) verVideo(p Post, m Medio) {
	a.videoPost, a.videoMedio, a.videoVolver = p, m, a.pant
	a.pant = pVideo
	a.tocarVideo("video:"+p.PK+m.Video, m, 0, yVideoGrande, ancho, altoVideoGrande)
}

func (a *app) pulsarVideo(b uint16) {
	switch b {
	case bA:
		a.rep.pausar(!a.rep.enPausa())
	case bIzquierda, bDerecha:
		d := 5 * time.Second
		if b == bIzquierda {
			d = -d
		}
		if t, ok := a.rep.saltar(d); ok {
			a.turnoRep = t
		}
	case bY:
		for i := range a.inicio.posts {
			if a.inicio.posts[i].PK == a.videoPost.PK {
				a.meGusta(&a.inicio.posts[i])
				a.videoPost.MeGusta, a.videoPost.Likes = a.inicio.posts[i].MeGusta, a.inicio.posts[i].Likes
				return
			}
		}
		a.meGusta(&a.videoPost)
	case bB:
		a.pararVideo()
		a.pant = a.videoVolver
	}
}

// ---- Reels ---------------------------------------------------------------------------------------

func (a *app) cargarReels(mas bool) {
	t := &a.reels
	if t.cargando || (mas && !t.mas) {
		return
	}
	cursor := ""
	if mas {
		cursor = t.cursor
	}
	t.cargando, t.err = true, ""
	go func() {
		desde := time.Now()
		p, err := a.ig.Reels(cursor)
		tarda := time.Since(desde).Seconds()
		a.hacer(func() {
			t.cargando, t.cargada = false, true
			if err != nil {
				logf("reels: %v", err)
				if a.pant != pReels && !mas { // se pedían por adelantado: otra vez al entrar
					a.reels = tira{}
					return
				}
				// Si no llegan los reels, al menos los vídeos de tu inicio.
				if vs := soloVideos(a.inicio.posts); !mas && len(vs) > 0 {
					t.posts, a.selReel, t.mas = vs, 0, false
					a.aviso("No llegan los reels: te pongo los vídeos de tu inicio")
					if a.pant == pReels {
						a.ponerReel()
					}
					return
				}
				t.err = a.fallo(err)
				return
			}
			if !mas {
				t.posts, a.selReel = nil, 0
			}
			t.posts = append(t.posts, sinRepetir(t.posts, p.Posts)...)
			t.cursor, t.mas = p.Cursor, p.Mas
			logf("reels: %d (%d en total) en %.1f s", len(p.Posts), len(t.posts), tarda)
			if a.pant == pReels {
				a.ponerReel()
			} else if !mas {
				a.prepararPrimerReel()
			}
		})
	}()
}

// adelantarReels: la lista de reels antes de entrar en la pestaña (si no se ha pedido ya).
func (a *app) adelantarReels() {
	if a.pant != pReels && !a.reels.cargada && !a.reels.cargando && a.ig.cuenta.tiene() {
		a.cargarReels(false)
	}
}

// prepararPrimerReel: el primer reel, bajado y con su ffmpeg arrancado, para cuando entres en Reels.
func (a *app) prepararPrimerReel() {
	if len(a.reels.posts) == 0 {
		return
	}
	md := a.reels.posts[0].Medios[0]
	a.ad.pedir(md.Video, func(ruta string) {
		a.hacer(func() {
			if a.pant != pReels && a.selReel == 0 && a.reproduciendo == "" {
				a.rep.preparar(fuenteDe(md, ruta, anchoVert, altoVert))
			}
		})
	})
}

func (a *app) reelActual() *Post {
	if a.selReel < 0 || a.selReel >= len(a.reels.posts) {
		return nil
	}
	return &a.reels.posts[a.selReel]
}

// ponerReel pone el reel elegido (y pide ya la foto del siguiente, que se ve mientras carga).
func (a *app) ponerReel() {
	p := a.reelActual()
	if p == nil {
		return
	}
	a.tocarVideo("reel:"+p.PK, p.Medios[0], (ancho-anchoVert)/2, 0, anchoVert, altoVert)
	// el siguiente, ya bajado para cuando llegues (un poco después: primero que arranque este)
	// y, ya bajado, su ffmpeg arrancado y esperando (arrancar ffmpeg cuesta 1,5 s en la consola)
	if sel := a.selReel; sel+1 < len(a.reels.posts) {
		sig := a.reels.posts[sel+1].Medios[0]
		time.AfterFunc(1500*time.Millisecond, func() {
			a.hacer(func() {
				if a.selReel != sel || a.pant != pReels {
					return
				}
				a.ad.pedir(sig.Video, func(ruta string) {
					a.hacer(func() {
						if a.selReel == sel && a.pant == pReels {
							a.rep.preparar(fuenteDe(sig, ruta, anchoVert, altoVert))
						}
					})
				})
			})
		})
	}
	for k := a.selReel + 1; k <= a.selReel+2 && k < len(a.reels.posts); k++ {
		a.img.adelantar(a.reels.posts[k].Medios[0].Foto)
		a.img.adelantar(a.reels.posts[k].FotoUsuario)
	}
	if a.selReel >= len(a.reels.posts)-3 {
		a.cargarReels(true)
	}
}

func (a *app) pulsarReels(b uint16) {
	switch b {
	case bArriba, bAbajo:
		n := a.selReel + 1
		if b == bArriba {
			n = a.selReel - 1
		}
		if n >= 0 && n < len(a.reels.posts) {
			a.selReel = n
			a.ponerReel()
		}
	case bA:
		if a.reelActual() == nil && a.reels.err != "" {
			a.cargarReels(false)
			break
		}
		a.rep.pausar(!a.rep.enPausa())
	case bIzquierda, bDerecha:
		d := 5 * time.Second
		if b == bIzquierda {
			d = -d
		}
		if t, ok := a.rep.saltar(d); ok {
			a.turnoRep = t
		}
	case bY:
		a.meGusta(a.reelActual())
	case bX:
		a.pararVideo()
		a.reels = tira{}
		a.cargarReels(false)
		a.aviso("Buscando más reels…")
	case bB:
		a.irA(pInicio)
	}
}

// ---- Historias -----------------------------------------------------------------------------------

func (a *app) cargarHistorias() {
	if a.histCarga {
		return
	}
	a.histCarga, a.histErr = true, ""
	go func() {
		desde := time.Now()
		bs, err := a.ig.Historias()
		tarda := time.Since(desde).Seconds()
		a.hacer(func() {
			a.histCarga, a.histHecha = false, true
			if err != nil {
				logf("historias: %v", err)
				a.histErr = a.fallo(err)
				return
			}
			// las que no has visto, primero (como en el móvil)
			var nuevas, vistas []Bandeja
			for _, b := range bs {
				if b.Vista {
					vistas = append(vistas, b)
				} else {
					nuevas = append(nuevas, b)
				}
			}
			a.bandejas = append(nuevas, vistas...)
			a.selHist = min(a.selHist, max(len(a.bandejas)-1, 0))
			logf("historias: %d personas en %.1f s", len(a.bandejas), tarda)
			for i, b := range a.bandejas { // las de las primeras filas (las demás, al verlas)
				if i >= carasAdelantadas {
					break
				}
				a.img.adelantar(b.Foto)
			}
		})
	}()
}

const colsHist = 5

// carasAdelantadas: las caras de la fila de historias que se piden al cargarla (antes se pedían las
// más de cien de golpe y quitaban sitio y tiempo a las fotos del inicio).
const carasAdelantadas = 20

func (a *app) pulsarHistorias(b uint16) {
	n := len(a.bandejas)
	switch b {
	case bIzquierda:
		a.selHist = max(a.selHist-1, 0)
	case bDerecha:
		a.selHist = min(a.selHist+1, max(n-1, 0))
	case bArriba:
		if a.selHist >= colsHist {
			a.selHist -= colsHist
		}
	case bAbajo:
		a.selHist = min(a.selHist+colsHist, max(n-1, 0))
	case bA:
		if n == 0 {
			if a.histErr != "" {
				a.cargarHistorias()
			}
			break
		}
		a.abrirHistoria(a.selHist, 0)
	case bX:
		a.cargarHistorias()
		a.aviso("Actualizando las historias…")
	case bB:
		a.irA(pInicio)
	}
}

// abrirHistoria: las historias de la persona k de la fila, desde la i.
func (a *app) abrirHistoria(k, i int) {
	if k < 0 || k >= len(a.bandejas) {
		a.pararVideo()
		a.pant = pHistorias
		return
	}
	a.pant, a.hb, a.hi = pHistoria, k, i
	a.selHist = k
	b := &a.bandejas[k]
	if len(b.Items) == 0 {
		if !a.pedidaHist {
			a.pedidaHist = true
			id := b.ID
			go func() {
				desde := time.Now()
				items, err := a.ig.HistoriasDe(id)
				if err == nil {
					logf("historias de una persona: %d en %.1f s", len(items), time.Since(desde).Seconds())
				}
				a.hacer(func() {
					a.pedidaHist = false
					for k := range a.bandejas { // se guardan aunque ya se esté viendo a otra persona
						if a.bandejas[k].ID == id && err == nil {
							a.bandejas[k].Items = items
						}
					}
					if a.pant != pHistoria || a.hb >= len(a.bandejas) {
						return
					}
					if a.bandejas[a.hb].ID != id { // se pasó a otra persona mientras cargaba: ahora la suya
						a.abrirHistoria(a.hb, a.hi)
						return
					}
					if err != nil { // se vuelve a la lista (pasar a la siguiente fallaría igual, una tras otra)
						logf("historias: %v", err)
						a.aviso(a.fallo(err))
						a.pararVideo()
						a.pant = pHistorias
						return
					}
					if len(items) == 0 {
						a.siguientePersona(1)
						return
					}
					a.empezarHistoria()
				})
			}()
		}
		a.pararVideo()
		a.histDesde = time.Time{}
		return
	}
	a.empezarHistoria()
}

func (a *app) historiaActual() *Post {
	if a.hb >= len(a.bandejas) {
		return nil
	}
	b := &a.bandejas[a.hb]
	if a.hi < 0 || a.hi >= len(b.Items) {
		return nil
	}
	return &b.Items[a.hi]
}

// empezarHistoria pone la historia de ahora (las fotos duran 5 s; los vídeos, lo que duren).
func (a *app) empezarHistoria() {
	b := &a.bandejas[a.hb]
	a.hi = min(max(a.hi, 0), len(b.Items)-1)
	p := b.Items[a.hi]
	a.histDesde, a.histPausa = time.Now(), 0
	if m := p.Medios[0]; m.esVideo() {
		a.tocarVideo("historia:"+p.PK, m, (ancho-anchoVert)/2, 0, anchoVert, altoVert)
	} else {
		a.pararVideo()
	}
	if a.hi == len(b.Items)-1 {
		b.Vista = true
	}
	// lo siguiente, ya pedido
	if a.hi+1 < len(b.Items) {
		a.img.adelantar(b.Items[a.hi+1].Medios[0].Foto)
	} else if a.hb+1 < len(a.bandejas) {
		a.img.adelantar(a.bandejas[a.hb+1].Foto)
	}
}

const duraFoto = 5 * time.Second

// avanzarHistoria (cada 40 ms): las fotos pasan solas a los 5 s.
func (a *app) avanzarHistoria() {
	p := a.historiaActual()
	if p == nil || a.histDesde.IsZero() || p.Medios[0].esVideo() || a.histPausa > 0 {
		return
	}
	if a.img.pedir(p.Medios[0].Foto, anchoVert, altoVert, false, 0) == nil { // aún no ha llegado: espera
		a.histDesde = time.Now()
		return
	}
	if time.Since(a.histDesde) >= duraFoto {
		a.siguienteHistoria(1)
		a.cambios = true
	}
}

func (a *app) siguienteHistoria(d int) {
	if a.hb >= len(a.bandejas) {
		return
	}
	b := a.bandejas[a.hb]
	n := a.hi + d
	switch {
	case n < 0:
		if a.hb > 0 {
			a.abrirHistoria(a.hb-1, 0)
		} else {
			a.hi = 0
			a.empezarHistoria()
		}
	case n >= len(b.Items):
		a.siguientePersona(1)
	default:
		a.hi = n
		a.empezarHistoria()
	}
}

func (a *app) siguientePersona(d int) {
	a.abrirHistoria(a.hb+d, 0)
}

func (a *app) pulsarHistoria(b uint16) {
	switch b {
	case bA, bDerecha:
		a.siguienteHistoria(1)
	case bIzquierda:
		a.siguienteHistoria(-1)
	case bR, bAbajo:
		a.siguientePersona(1)
	case bL, bArriba:
		if a.hb > 0 {
			a.siguientePersona(-1)
		}
	case bY: // pausa (las fotos dejan de pasar)
		p := a.historiaActual()
		if p == nil {
			break
		}
		if p.Medios[0].esVideo() {
			a.rep.pausar(!a.rep.enPausa())
		} else if a.histPausa > 0 {
			a.histDesde, a.histPausa = time.Now().Add(-a.histPausa), 0
		} else {
			a.histPausa = max(time.Since(a.histDesde), time.Millisecond)
		}
	case bB:
		a.pararVideo()
		a.pant = pHistorias
	}
}

// ---- Mensajes ------------------------------------------------------------------------------------

func (a *app) cargarChats() {
	if a.chatsCarga {
		return
	}
	a.chatsCarga, a.chatsErr = true, ""
	go func() {
		desde := time.Now()
		cs, err := a.ig.Chats()
		if err == nil {
			logf("mensajes: %d chats en %.1f s", len(cs), time.Since(desde).Seconds())
		}
		a.hacer(func() {
			a.chatsCarga, a.chatsHecho, a.chatsEn = false, true, time.Now()
			if err != nil {
				logf("mensajes: %v", err)
				a.chatsErr = a.fallo(err)
				return
			}
			for i := range cs {
				if a.leidos[cs[i].ID+"|"+cs[i].Cuando.String()] {
					cs[i].SinLeer = false
				}
			}
			a.chats = cs
			a.selChat = min(a.selChat, max(len(cs)-1, 0))
			for _, c := range cs {
				a.img.adelantar(c.Foto)
			}
		})
	}()
}

func (a *app) sinLeer() int {
	n := 0
	for _, c := range a.chats {
		if c.SinLeer {
			n++
		}
	}
	return n
}

func (a *app) pulsarMensajes(b uint16) {
	n := len(a.chats)
	switch b {
	case bArriba:
		a.selChat = max(a.selChat-1, 0)
	case bAbajo:
		a.selChat = min(a.selChat+1, max(n-1, 0))
	case bA:
		if n == 0 {
			if a.chatsErr != "" {
				a.cargarChats()
			}
			break
		}
		c := a.chats[a.selChat]
		a.chats[a.selChat].SinLeer = false
		a.leidos[c.ID+"|"+c.Cuando.String()] = true
		a.chat, a.subir, a.chatErr = &Chat{ID: c.ID, Titulo: c.Titulo, Foto: c.Foto}, 0, ""
		a.pant = pChat
		a.cargarChat(false)
	case bX:
		a.cargarChats()
		a.aviso("Actualizando los mensajes…")
	case bB:
		a.irA(pInicio)
	}
}

// cargarChat: los últimos mensajes de la conversación abierta (antes: los de antes).
func (a *app) cargarChat(antes bool) {
	c := a.chat
	if c == nil || a.chatCarga || (antes && c.Cursor == "") {
		return
	}
	cursor := ""
	if antes {
		cursor = c.Cursor
	}
	a.chatCarga, a.chatEn = true, time.Now()
	go func() {
		desde := time.Now()
		ms, sig, err := a.ig.Conversacion(c.ID, cursor)
		if err == nil {
			logf("conversación: %d mensajes en %.1f s", len(ms), time.Since(desde).Seconds())
		}
		a.hacer(func() {
			a.chatCarga, a.chatEn = false, time.Now()
			if a.chat != c {
				return
			}
			if err != nil {
				logf("conversación: %v", err)
				a.chatErr = a.fallo(err)
				return
			}
			a.chatErr = ""
			switch {
			case antes:
				c.Mensajes = append(ms, c.Mensajes...)
			case len(ms) > 0 && len(c.Mensajes) > 0: // al actualizar, los de antes que ya estaban se quedan
				var viejos []Mensaje
				for _, m := range c.Mensajes {
					if m.Cuando.Before(ms[0].Cuando) {
						viejos = append(viejos, m)
					}
				}
				c.Mensajes = append(viejos, ms...)
			default:
				c.Mensajes = ms
			}
			if antes || c.Cursor == "" {
				c.Cursor = sig
			}
		})
	}()
}

func (a *app) pulsarChat(b uint16) {
	switch b {
	case bArriba:
		a.subir += 60
	case bAbajo:
		a.subir = max(a.subir-60, 0)
	case bA:
		a.tec.borrarTodo()
		a.tec.abrir()
		a.pant = pEscribir
	case bX:
		a.cargarChat(false)
	case bB:
		a.chat = nil
		a.pant = pMensajes
	}
}

func (a *app) pulsarEscribir(b uint16) {
	switch a.tec.pulsar(b) {
	case tecCancelar:
		a.pant = pChat
	case tecEnviar:
		texto := strings.TrimSpace(a.tec.texto())
		if texto == "" || a.enviando || a.chat == nil {
			break
		}
		c := a.chat
		a.enviando = true
		go func() {
			err := a.ig.Enviar(c.ID, texto)
			a.hacer(func() {
				a.enviando = false
				if err != nil {
					logf("enviar: %v", err)
					a.aviso("No se ha enviado: " + strings.TrimPrefix(a.fallo(err), "No ha cargado: "))
					return
				}
				a.tec.borrarTodo()
				if a.chat == c {
					c.Mensajes = append(c.Mensajes, Mensaje{Mio: true, Texto: texto, Cuando: time.Now()})
					a.subir = 0
					a.pant = pChat
					a.chatEn = time.Now().Add(-8 * time.Second) // que se actualice en 2 s
				}
				a.aviso("Enviado")
			})
		}()
	}
}

// ---- Tu cuenta -----------------------------------------------------------------------------------

func (a *app) abrirPagina() {
	if a.web != nil {
		return
	}
	a.sesionEstado = "Esperando tu sesión…"
	a.web = &paginaSesion{validar: func(texto string) error {
		a.hacer(func() { a.sesionEstado = "Comprobando la sesión con Instagram…" })
		err := a.ig.ponerCuenta(texto)
		limite := errors.Is(err, errLimite)
		a.hacer(func() {
			if err != nil && !limite {
				a.sesionEstado = "Instagram no ha aceptado esa sesión. Prueba otra vez."
				return
			}
			a.cuentaMal, a.cambiandoCta = false, false
			a.cerrarPagina()
			if limite {
				a.aviso("Guardada. Instagram va lento ahora: si no carga, espera unos minutos")
			} else if u := a.ig.cuenta.usuario(); u != "" {
				a.aviso("Hola, @" + u)
			} else {
				a.aviso("¡Listo! Ya tienes tu Instagram en la consola")
			}
			a.inicio, a.reels = tira{}, tira{}
			a.bandejas, a.histHecha, a.chats, a.chatsHecho = nil, false, nil, false
			a.irA(pInicio)
			a.cargarHistorias()
			a.cargarChats()
		})
		if limite {
			return nil // guardada: la página dice «¡Hecho!»
		}
		return err
	}}
	if err := a.web.abrir(); err != nil {
		a.sesionEstado = "No se pudo abrir la página: " + err.Error()
		logf("página de la cuenta: %v", err)
	}
}

func (a *app) cerrarPagina() {
	a.web.cerrar()
	a.web = nil
}

func (a *app) pulsarCuenta(b uint16) {
	tiene := a.ig.cuenta.tiene()
	switch b {
	case bA:
		if tiene && !a.cambiandoCta {
			a.cambiandoCta = true
			a.abrirPagina()
		}
	case bX:
		if tiene {
			a.preguntar("¿Cerrar la sesión de Instagram en la consola? (En el PC y el móvil sigues dentro.)", func() {
				a.ig.cuenta.olvidar()
				a.inicio, a.reels = tira{}, tira{}
				a.bandejas, a.histHecha, a.chats, a.chatsHecho = nil, false, nil, false
				a.cambiandoCta = false
				a.abrirPagina()
				a.aviso("Sesión cerrada en la consola")
			})
		}
	case bB:
		if a.cambiandoCta {
			a.cambiandoCta = false
			a.cerrarPagina()
			break
		}
		if tiene {
			a.irA(pInicio)
		} else {
			a.terminar()
		}
	}
}

// ---- Textos --------------------------------------------------------------------------------------

// cifra: 1.234, 12,3 mil, 1,2 M (como en Instagram en español).
func cifra(n int) string {
	switch {
	case n < 10000:
		s := fmt.Sprint(n)
		if n >= 1000 {
			s = s[:len(s)-3] + "." + s[len(s)-3:]
		}
		return s
	case n < 1000000:
		if n < 100000 {
			return strings.Replace(fmt.Sprintf("%.1f mil", float64(n)/1000), ".", ",", 1)
		}
		return fmt.Sprintf("%d mil", n/1000)
	}
	return strings.Replace(fmt.Sprintf("%.1f M", float64(n)/1e6), ".", ",", 1)
}

var meses = []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sept", "oct", "nov", "dic"}

// hace: «hace 5 min», «hace 3 h», «hace 2 d» o la fecha.
func hace(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "ahora"
	case d < time.Hour:
		return fmt.Sprintf("hace %d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("hace %d h", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("hace %d d", int(d.Hours()/24))
	case t.Year() == time.Now().Year():
		return fmt.Sprintf("%d %s", t.Day(), meses[t.Month()-1])
	}
	return fmt.Sprintf("%d %s %d", t.Day(), meses[t.Month()-1], t.Year())
}

// horaCorta: la hora de un mensaje (o el día, si no es de hoy).
func horaCorta(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if y, m, d := time.Now().Date(); t.Year() == y && t.Month() == m && t.Day() == d {
		return t.Format("15:04")
	}
	if time.Since(t) < 7*24*time.Hour {
		return [...]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}[t.Weekday()]
	}
	return fmt.Sprintf("%d %s", t.Day(), meses[t.Month()-1])
}
