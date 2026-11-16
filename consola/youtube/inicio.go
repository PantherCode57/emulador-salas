package main

import (
	"strings"
	"time"
)

// filaInicio: una fila de tarjetas de la pantalla de inicio.
type filaInicio struct {
	clave  string // «historial», «descargados» o la página de YouTube de una fila de tu cuenta
	nombre string
	icono  string
	videos []Video
	pos    []int // por dónde ibas en cada uno (la barrita roja)
	feed   *feed
}

// filasInicio: con cuenta, tus filas (las que tienen algo); luego «Seguir viendo» y «Descargados».
func (a *app) filasInicio() []filaInicio {
	var filas []filaInicio
	if a.yt.cuenta.tiene() {
		for _, f := range a.feeds {
			if len(f.Videos) == 0 && !f.Cargando {
				continue
			}
			fi := filaInicio{clave: f.Clave, nombre: f.Nombre, icono: f.Icono, videos: f.Videos, feed: f}
			for _, v := range f.Videos {
				fi.pos = append(fi.pos, a.alm.posicion(v.ID))
			}
			filas = append(filas, fi)
		}
	}
	if len(a.alm.historial) > 0 {
		fi := filaInicio{clave: "historial", nombre: "Seguir viendo", icono: icHistorial}
		for _, h := range a.alm.historial {
			fi.videos, fi.pos = append(fi.videos, h.Video), append(fi.pos, h.Pos)
		}
		filas = append(filas, fi)
	}
	if len(a.alm.descargados) > 0 {
		fi := filaInicio{clave: "descargados", nombre: "Descargados", icono: icDescarga}
		for _, d := range a.alm.descargados {
			fi.videos, fi.pos = append(fi.videos, d.Video), append(fi.pos, a.alm.posicion(d.ID))
		}
		filas = append(filas, fi)
	}
	return filas
}

// focoActual: el número de la fila elegida (-1: la de buscar).
func (a *app) focoActual(filas []filaInicio) int {
	for i, f := range filas {
		if f.clave == a.foco {
			return i
		}
	}
	return -1
}

func (a *app) elegidoEn(f filaInicio) (Video, int, bool) {
	i := min(a.sel[f.clave], len(f.videos)-1)
	if i < 0 {
		return Video{}, 0, false
	}
	return f.videos[i], i, true
}

func (a *app) pulsarInicio(b uint16) {
	filas := a.filasInicio()
	k := a.focoActual(filas)
	if k < 0 {
		a.foco = "buscar"
	}
	switch b {
	case bArriba:
		if k > 0 {
			a.foco = filas[k-1].clave
		} else {
			a.foco = "buscar"
		}
	case bAbajo:
		if k+1 < len(filas) {
			a.foco = filas[k+1].clave
		}
	case bIzquierda, bDerecha:
		if k < 0 {
			a.focoCuenta = b == bDerecha
			break
		}
		f := filas[k]
		i := a.sel[f.clave]
		if b == bIzquierda {
			i = max(i-1, 0)
		} else {
			i = min(i+1, len(f.videos)-1)
		}
		a.sel[f.clave] = i
		if f.feed != nil && i >= len(f.videos)-4 {
			a.masFeed(f.feed)
		}
	case bA:
		if k < 0 {
			if a.focoCuenta {
				a.abrirCuenta()
			} else {
				a.abrirTeclado()
			}
			return
		}
		f := filas[k]
		if v, i, ok := a.elegidoEn(f); ok {
			a.ver(v, a.reanudarDe(v, a.alm.posicion(v.ID)), f.videos, i, f.feed)
		}
	case bStart:
		a.abrirTeclado()
	case bY:
		if k >= 0 && filas[k].clave != "descargados" {
			if v, _, ok := a.elegidoEn(filas[k]); ok {
				a.descargar(v)
			}
		}
	case bX:
		if k < 0 {
			break
		}
		f := filas[k]
		v, _, ok := a.elegidoEn(f)
		switch {
		case !ok:
		case f.clave == "historial":
			a.alm.quitarHistorial(v.ID)
			a.aviso("Quitado de «Seguir viendo»")
		case f.clave == "descargados":
			id := v.ID
			a.preguntar("¿Borrar «"+recortar(a.f.texto, a.f.limpiar(v.Titulo), 380)+"» de la consola?", func() {
				if err := a.alm.borrarDescarga(id); err != nil {
					a.aviso("No se ha podido borrar")
					return
				}
				a.aviso("Borrado")
			})
		case f.feed != nil: // la fila entera, como lista
			a.abrirLista(f.feed)
		}
	case bB:
		a.terminar()
	}
	if a.pant == pInicio {
		filas = a.filasInicio()
		if k := a.focoActual(filas); k >= 0 {
			if v, _, ok := a.elegidoEn(filas[k]); ok {
				a.marcarPreparar(&v)
			}
		}
	}
}

// reanudarDe: por dónde seguir (desde el principio si casi no lo habías empezado o casi lo acabaste).
func (a *app) reanudarDe(v Video, pos int) time.Duration {
	if v.Corto || pos < 10 || (v.Segundos > 0 && pos > v.Segundos-20) {
		return 0
	}
	return time.Duration(pos) * time.Second
}

// ---- Tu cuenta ----------------------------------------------------------------------------------

func (a *app) abrirCuenta() {
	a.pant = pCuenta
	a.cambiandoCta = false
	if !a.yt.cuenta.tiene() {
		a.abrirPaginaSesion()
	}
}

func (a *app) cerrarCuenta() {
	a.web.cerrar()
	a.web = nil
	a.cambiandoCta = false
	a.pant = pInicio
}

func (a *app) abrirPaginaSesion() {
	if a.web != nil {
		return
	}
	a.sesionEstado = "Esperando tu sesión…"
	a.web = &paginaSesion{validar: func(texto string) error {
		a.hacer(func() { a.sesionEstado = "Comprobando la sesión con YouTube…" })
		err := a.yt.ponerCuenta(texto)
		a.hacer(func() {
			if err != nil {
				a.sesionEstado = "YouTube no ha aceptado esa sesión. Prueba otra vez."
				return
			}
			a.cuentaMal = false
			a.cerrarCuenta()
			a.aviso("Hola, " + a.f.limpiar(a.yt.cuenta.nombre()))
			a.feeds = feedsCuenta()
			a.foco = a.feeds[0].Clave
			a.cargarFeeds()
		})
		return err
	}}
	if err := a.web.abrir(); err != nil {
		a.sesionEstado = "No se pudo abrir la página: " + err.Error()
		logf("página de la cuenta: %v", err)
	}
}

func (a *app) pulsarCuenta(b uint16) {
	tiene := a.yt.cuenta.tiene()
	switch b {
	case bB:
		a.cerrarCuenta()
	case bA:
		if tiene && !a.cambiandoCta {
			a.cambiandoCta = true
			a.abrirPaginaSesion()
		}
	case bX:
		if tiene {
			a.preguntar("¿Cerrar la sesión de tu cuenta en la consola? (En el PC y el móvil sigues dentro.)", func() {
				a.yt.cuenta.olvidar()
				a.feeds = feedsCuenta()
				a.cerrarCuenta()
				a.foco = "buscar"
				a.aviso("Sesión cerrada en la consola")
			})
		}
	case bY:
		if tiene {
			a.feeds = feedsCuenta()
			a.cargarFeeds()
			a.cerrarCuenta()
			a.aviso("Actualizando tus listas…")
		}
	}
}

// sesionCaducada: YouTube ya no acepta tu sesión (se cerró en el navegador o caducó).
func (a *app) sesionCaducada() {
	if a.cuentaMal {
		return
	}
	a.cuentaMal = true
	a.aviso("Tu sesión de YouTube ha caducado: pásala otra vez en «Cuenta»")
}

// ---- Buscar y listas ----------------------------------------------------------------------------

func (a *app) abrirTeclado() {
	a.desdeBuscar = a.pant
	a.tec.abrir()
	a.pant = pBuscar
}

func (a *app) pulsarBuscar(b uint16) {
	switch a.tec.pulsar(b) {
	case tecEnviar:
		q := strings.TrimSpace(a.tec.texto())
		if q == "" {
			return
		}
		a.buscar(q)
	case tecCancelar:
		a.pant = a.desdeBuscar
		if a.pant == pBuscar || a.pant == pViendo {
			a.pant = pInicio
		}
	}
}

func (a *app) buscar(q string) {
	if !hayRed() {
		a.aviso("Sin internet: no se puede buscar")
		return
	}
	a.consulta, a.res, a.resFeed, a.token, a.selRes, a.primeraRes, a.errBusca = q, nil, nil, "", 0, 0, ""
	a.buscando, a.cargandoMas = true, false
	a.pant = pResultados
	a.turnoBusca++
	t := a.turnoBusca
	go func() {
		vids, token, err := a.yt.buscar(q)
		a.hacer(func() {
			if t != a.turnoBusca {
				return
			}
			a.buscando = false
			if err != nil {
				logf("buscar %q: %v", q, err)
				a.errBusca = "No se ha podido buscar. Prueba otra vez."
				return
			}
			a.res, a.token = vids, token
			if len(a.res) > 0 {
				a.marcarPreparar(&a.res[0])
			}
		})
	}()
}

// abrirLista: una fila de tu cuenta entera, como la lista de una búsqueda.
func (a *app) abrirLista(f *feed) {
	a.consulta, a.resFeed, a.res = f.Nombre, f, f.Videos
	a.selRes, a.primeraRes, a.errBusca, a.buscando = a.sel[f.Clave], 0, "", false
	a.turnoBusca++
	a.pant = pResultados
}

func (a *app) cargarMas() {
	if a.resFeed != nil {
		a.masFeed(a.resFeed)
		return
	}
	if a.token == "" || a.cargandoMas || a.buscando || !hayRed() {
		return
	}
	a.cargandoMas = true
	t, token := a.turnoBusca, a.token
	go func() {
		vids, nuevo, err := a.yt.mas(token)
		a.hacer(func() {
			if t != a.turnoBusca {
				return
			}
			a.cargandoMas = false
			if err != nil {
				logf("más resultados: %v", err)
				return
			}
			a.res, a.token = juntar(a.res, vids), nuevo
			if len(vids) == 0 {
				a.token = ""
			}
		})
	}()
}

func (a *app) pulsarResultados(b uint16) {
	if a.resFeed != nil {
		a.res = a.resFeed.Videos
	}
	n := len(a.res)
	switch b {
	case bArriba:
		a.selRes = max(a.selRes-1, 0)
	case bAbajo:
		a.selRes = min(a.selRes+1, max(n-1, 0))
	case bL:
		a.selRes = max(a.selRes-4, 0)
	case bR:
		a.selRes = min(a.selRes+4, max(n-1, 0))
	case bA:
		if a.selRes < n {
			v := a.res[a.selRes]
			a.ver(v, a.reanudarDe(v, a.alm.posicion(v.ID)), a.res, a.selRes, a.resFeed)
		}
	case bY:
		if a.selRes < n {
			a.descargar(a.res[a.selRes])
		}
	case bX, bStart:
		a.abrirTeclado()
	case bB:
		if a.resFeed != nil {
			a.sel[a.resFeed.Clave] = a.selRes
			a.foco = a.resFeed.Clave
		}
		a.turnoBusca++
		a.buscando = false
		a.pant = pInicio
	}
	if n > 0 && a.selRes >= n-4 {
		a.cargarMas()
	}
	if a.pant == pResultados && a.selRes < len(a.res) {
		a.marcarPreparar(&a.res[a.selRes])
	}
}
