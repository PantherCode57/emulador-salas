package main

import (
	"strings"
	"time"
)

// ver abre un vídeo (el descargado si lo está; si no, de internet) desde «desde». cola es la lista
// de la que sale (con los Shorts, arriba y abajo pasan al anterior y al siguiente).
func (a *app) ver(v Video, desde time.Duration, cola []Video, i int, f *feed) {
	if a.pant != pViendo {
		a.volverA = a.pant
	}
	a.guardarPosicion()
	a.rep.parar()
	vid := v
	a.viendo, a.intento, a.desdeVideo, a.terminaVideo = &vid, 0, desde, false
	a.cola, a.colaI, a.colaFeed = cola, i, f
	a.pant = pViendo
	a.vv.mu.Lock()
	a.vv.activo, a.vv.ultimo, a.vv.bandas = true, nil, nil
	a.vv.mu.Unlock()
	if !v.Corto {
		a.alm.apuntar(v, int(desde.Seconds()))
	}
	a.guardadaEn = time.Now()
	a.abrirFuente()
}

// enCorto: lo que se ve es un Short (o un vídeo vertical): pantalla y botones de los Shorts.
func (a *app) enCorto() bool {
	if a.viendo == nil {
		return false
	}
	if a.rep.abierto() {
		return a.rep.vertical()
	}
	return a.viendo.Corto
}

func (a *app) abrirFuente() {
	v := *a.viendo
	a.turnoVideo++
	t := a.turnoVideo
	a.cargando, a.cargaDesde = true, time.Now()
	if d := a.alm.descargado(v.ID); d != nil {
		a.empezarVideo(&fuenteVideo{Video: d.Archivo, Duracion: v.duracion(), FPS: d.FPS, Vertical: d.Vertical || v.Corto})
		return
	}
	if !hayRed() {
		a.salirVideo()
		a.aviso("Sin internet: solo se pueden ver los vídeos descargados")
		return
	}
	intento := a.intento
	go func() {
		f, err := a.yt.fuente(v.ID, intento)
		a.hacer(func() {
			if t != a.turnoVideo || a.pant != pViendo {
				return
			}
			if err != nil {
				a.falloVideo(err)
				return
			}
			if f.Duracion == 0 {
				f.Duracion = v.duracion()
			}
			if intento >= len(formatosVer) { // por tubo no se sabe la forma: la que diga la lista
				f.Vertical = v.Corto
			}
			a.completarDatos(f)
			a.empezarVideo(f)
		})
	}()
}

// completarDatos: los Shorts de la secuencia llegan sin título; yt-dlp sí lo dice.
func (a *app) completarDatos(f *fuenteVideo) {
	v := a.viendo
	if v == nil {
		return
	}
	cambiar := func(x *Video) {
		if x.Titulo == "" {
			x.Titulo = f.Titulo
		}
		if x.Canal == "" {
			x.Canal = f.Canal
		}
		if x.Vistas == "" {
			x.Vistas = f.Vistas
		}
		if x.Segundos == 0 && f.Duracion > 0 {
			x.Segundos = int(f.Duracion.Seconds())
		}
	}
	cambiar(v)
	if a.colaFeed != nil {
		for i := range a.colaFeed.Videos {
			if a.colaFeed.Videos[i].ID == v.ID {
				cambiar(&a.colaFeed.Videos[i])
			}
		}
	}
	if a.colaI < len(a.cola) && a.cola[a.colaI].ID == v.ID {
		cambiar(&a.cola[a.colaI])
	}
}

func (a *app) empezarVideo(f *fuenteVideo) {
	turno, err := a.rep.tocar(f, a.desdeVideo)
	a.turnoRep = turno
	a.cargando = false
	if err != nil {
		a.falloVideo(err)
		return
	}
	// Con los Shorts, el siguiente se va pidiendo ya (mientras ves este).
	if a.viendo.Corto || f.Vertical {
		if a.colaI+1 < len(a.cola) {
			sig := a.cola[a.colaI+1]
			a.preparar, a.prepararEn = "", time.Time{}
			a.marcarPreparar(&sig)
			a.prepararEn = time.Now().Add(3 * time.Second)
		}
		if a.colaFeed != nil && a.colaI >= len(a.cola)-3 {
			a.masFeed(a.colaFeed)
		}
	}
}

// videoAcabado y videoFallido llegan desde el hilo del reproductor.
func (a *app) videoAcabado(turno int) {
	a.hacer(func() {
		if turno != a.turnoRep || a.pant != pViendo {
			return
		}
		if a.viendo != nil && !a.viendo.Corto {
			a.alm.apuntar(*a.viendo, 0)
		}
		if a.enCorto() && a.pasarCorto(1) { // los Shorts siguen con el siguiente
			return
		}
		a.terminaVideo = true
		a.salirVideo()
	})
}

func (a *app) videoFallido(turno int, err error) {
	a.hacer(func() {
		if turno != a.turnoRep || a.pant != pViendo {
			return
		}
		a.falloVideo(err)
	})
}

// falloVideo: se prueba la siguiente forma de sacar el vídeo; si no queda ninguna, se avisa.
func (a *app) falloVideo(err error) {
	logf("vídeo %s (forma %d): %v", a.viendo.ID, a.intento, err)
	if a.alm.descargado(a.viendo.ID) == nil && a.intento+1 < intentosFuente {
		a.yt.olvidar(a.viendo.ID)
		if p := a.rep.posicion(); p > a.desdeVideo {
			a.desdeVideo = p
		}
		a.intento++
		a.rep.parar()
		a.abrirFuente()
		return
	}
	msg := "No se ha podido ver este vídeo"
	if strings.Contains(err.Error(), "Sign in") || strings.Contains(err.Error(), "age") {
		msg = "YouTube pide iniciar sesión para ver este vídeo"
	}
	if a.enCorto() && a.pasarCorto(1) {
		a.aviso(msg + ": paso al siguiente")
		return
	}
	a.salirVideo()
	a.aviso(msg)
}

// pasarCorto: al Short anterior (-1) o al siguiente (1). false si no hay más.
func (a *app) pasarCorto(d int) bool {
	if a.colaFeed != nil {
		a.cola = a.colaFeed.Videos
	}
	i := a.colaI + d
	if i < 0 || i >= len(a.cola) {
		if d > 0 && a.colaFeed != nil {
			a.masFeed(a.colaFeed)
		}
		return false
	}
	a.ver(a.cola[i], 0, a.cola, i, a.colaFeed)
	return true
}

// guardarPosicion apunta en el historial por dónde va el vídeo que se está viendo.
func (a *app) guardarPosicion() {
	a.guardadaEn = time.Now()
	if a.viendo == nil || a.viendo.Corto || a.pant != pViendo || a.cargando || !a.rep.abierto() {
		return
	}
	a.alm.apuntar(*a.viendo, int(a.rep.posicion().Seconds()))
}

func (a *app) salirVideo() {
	if !a.terminaVideo {
		a.guardarPosicion()
	}
	a.turnoVideo++
	a.rep.parar()
	a.vv.mu.Lock()
	a.vv.activo, a.vv.ultimo = false, nil
	a.vv.mu.Unlock()
	a.cargando = false
	a.pant = a.volverA
	if a.pant == pViendo || a.pant == pBuscar || a.pant == pConfirmar || a.pant == pCuenta {
		a.pant = pInicio
	}
	// Al volver, lo elegido es lo último que has visto.
	if a.colaFeed != nil {
		a.sel[a.colaFeed.Clave] = a.colaI
		if a.pant == pResultados && a.resFeed == a.colaFeed {
			a.selRes = a.colaI
		}
	} else if a.pant == pInicio && a.viendo != nil && !a.viendo.Corto && len(a.alm.historial) > 0 && a.foco == "historial" {
		a.sel["historial"] = 0
	}
	a.viendo = nil
}

func (a *app) pulsarViendo(b uint16) {
	listo := !a.cargando && a.rep.abierto()
	corto := a.enCorto()
	switch b {
	case bB:
		a.salirVideo()
	case bA:
		if listo {
			a.rep.pausar(!a.rep.enPausa())
		}
	case bIzquierda, bDerecha:
		paso := 10 * time.Second
		if corto {
			paso = 5 * time.Second
		}
		if b == bIzquierda {
			paso = -paso
		}
		a.adelantar(paso)
	case bL, bR:
		if corto { // en los Shorts, arriba y abajo cambian de Short: el volumen va en L y R
			if b == bL {
				a.cambiarVolumen(-5)
			} else {
				a.cambiarVolumen(5)
			}
		} else if b == bL {
			a.adelantar(-time.Minute)
		} else {
			a.adelantar(time.Minute)
		}
	case bArriba, bAbajo:
		switch {
		case corto && b == bArriba:
			if !a.pasarCorto(-1) {
				a.aviso("Es el primero")
			}
		case corto:
			if !a.pasarCorto(1) {
				a.aviso("Cargando más Shorts…")
			}
		case b == bArriba:
			a.cambiarVolumen(5)
		default:
			a.cambiarVolumen(-5)
		}
	case bY:
		if a.viendo != nil {
			a.descargar(*a.viendo)
		}
	}
}

func (a *app) adelantar(d time.Duration) {
	if a.cargando || !a.rep.abierto() {
		return
	}
	turno, ok := a.rep.saltar(d)
	if !ok {
		a.aviso("Este vídeo no se puede adelantar")
		return
	}
	a.turnoRep = turno
}
