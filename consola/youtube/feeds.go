package main

import (
	"errors"
	"sync"
)

// Con tu cuenta, la pantalla de inicio enseña lo mismo que la web de YouTube: tus recomendaciones,
// los Shorts, tus suscripciones, «Ver más tarde» y los vídeos que te gustan. Cada fila es una
// página de la web («browse») que se pide al abrir la app y que sigue cargando al llegar al final.
type feed struct {
	Clave    string // la página de YouTube (FEwhat_to_watch, FEsubscriptions, VLWL…)
	Nombre   string
	Icono    string
	Cortos   bool // la fila de los Shorts (tarjetas verticales)
	Videos   []Video
	Token    string
	Cargando bool
	Cargado  bool
	Error    string
}

func feedsCuenta() []*feed {
	return []*feed{
		{Clave: "FEwhat_to_watch", Nombre: "Para ti", Icono: icInicio},
		{Clave: "shorts", Nombre: "Shorts", Icono: icRayo, Cortos: true},
		{Clave: "FEsubscriptions", Nombre: "Suscripciones", Icono: icSuscripciones},
		{Clave: "VLWL", Nombre: "Ver más tarde", Icono: icReloj},
		{Clave: "VLLL", Nombre: "Vídeos que te gustan", Icono: icMeGusta},
	}
}

// explorar: la primera página de una de esas filas. En la de inicio y las suscripciones vienen
// mezclados vídeos y Shorts: cada uno va a su fila.
func (y *youtube) explorar(clave string) ([]Video, string, error) {
	m, err := y.pedir("browse", map[string]any{"browseId": clave})
	if err != nil {
		return nil, "", err
	}
	if sesionCaducada(m) {
		return nil, "", errSesion
	}
	vids, token := recogerVideos(m)
	return vids, token, nil
}

// masDe: la página siguiente de una fila.
func (y *youtube) masDe(token string) ([]Video, string, error) {
	m, err := y.pedir("browse", map[string]any{"continuation": token})
	if err != nil {
		return nil, "", err
	}
	vids, nuevo := recogerVideos(m)
	return vids, nuevo, nil
}

// secuenciaShorts: los Shorts que van detrás de uno (como al deslizar en el móvil). La primera vez
// con los «sequenceParams» de un Short; luego, con la marca que devuelve.
func (y *youtube) secuenciaShorts(seq string) ([]Video, string, error) {
	if seq == "" {
		return nil, "", errors.New("sin secuencia")
	}
	m, err := y.pedir("reel/reel_watch_sequence", map[string]any{"sequenceParams": seq})
	if err != nil {
		return nil, "", err
	}
	var vids []Video
	for _, e := range lista(m, "entries") {
		ep := ir(e, "command", "reelWatchEndpoint")
		id := txt(ep, "videoId")
		if id == "" {
			continue
		}
		v := Video{ID: id, Corto: true, Seq: txt(ep, "sequenceParams")}
		v.Mini = txt(ep, "thumbnail", "thumbnails", 0, "url")
		vids = append(vids, v)
	}
	sig := txt(m, "continuationEndpoint", "continuationCommand", "token")
	return vids, sig, nil
}

// cargarFeeds pide todas las filas a la vez (en hilos) y avisa a la app con cada una.
func (a *app) cargarFeeds() {
	if !a.yt.cuenta.tiene() || !hayRed() {
		return
	}
	var cortos *feed
	for _, f := range a.feeds {
		if f.Cortos {
			cortos = f
		}
	}
	var mu sync.Mutex
	if cortos != nil && !cortos.Cargando {
		// Los Shorts: los de tu inicio (abajo) y, para empezar seguro, los de una búsqueda de Shorts
		// (con tu cuenta, a tu gusto). Detrás, la secuencia sin fin.
		cortos.Cargando = true
		go func() {
			m, err := a.yt.pedir("search", map[string]any{"query": "#shorts"})
			var vids []Video
			if err == nil {
				todos, _ := recogerVideos(m)
				for _, v := range todos {
					if v.Corto {
						vids = append(vids, v)
					}
				}
			}
			a.hacer(func() {
				cortos.Cargando = false
				if err != nil {
					logf("Shorts: %v", err)
				}
				mu.Lock()
				cortos.Videos = juntar(cortos.Videos, vids)
				cortos.Cargado = true
				mu.Unlock()
			})
		}()
	}
	for _, f := range a.feeds {
		if f.Cortos || f.Cargando {
			continue
		}
		f.Cargando, f.Error = true, ""
		go func(f *feed) {
			vids, token, err := a.yt.explorar(f.Clave)
			a.hacer(func() {
				f.Cargando = false
				if err != nil {
					logf("fila %s: %v", f.Clave, err)
					f.Error = "No se ha podido cargar"
					if errors.Is(err, errSesion) {
						a.sesionCaducada()
					}
					return
				}
				f.Cargado = true
				var largos, nuevosCortos []Video
				for _, v := range vids {
					if v.Corto {
						nuevosCortos = append(nuevosCortos, v)
					} else {
						largos = append(largos, v)
					}
				}
				f.Videos, f.Token = largos, token
				if cortos != nil && len(nuevosCortos) > 0 {
					mu.Lock()
					cortos.Videos = juntar(cortos.Videos, nuevosCortos)
					cortos.Cargado = true
					mu.Unlock()
				}
			})
		}(f)
	}
}

// masFeed: sigue cargando una fila (al llegar a su final).
func (a *app) masFeed(f *feed) {
	if f == nil || f.Cargando || !hayRed() {
		return
	}
	if f.Cortos {
		a.masCortos(f)
		return
	}
	if f.Token == "" {
		return
	}
	f.Cargando = true
	token := f.Token
	go func() {
		vids, nuevo, err := a.yt.masDe(token)
		a.hacer(func() {
			f.Cargando = false
			if err != nil {
				logf("más de %s: %v", f.Clave, err)
				return
			}
			var largos []Video
			for _, v := range vids {
				if !v.Corto {
					largos = append(largos, v)
				}
			}
			f.Videos, f.Token = juntar(f.Videos, largos), nuevo
			if len(vids) == 0 {
				f.Token = ""
			}
		})
	}()
}

// masCortos: más Shorts detrás del último (la secuencia de YouTube).
func (a *app) masCortos(f *feed) {
	seq := f.Token
	if seq == "" && len(f.Videos) > 0 {
		seq = f.Videos[len(f.Videos)-1].Seq
		for i := len(f.Videos) - 1; seq == "" && i >= 0; i-- {
			seq = f.Videos[i].Seq
		}
	}
	if seq == "" {
		return
	}
	f.Cargando = true
	go func() {
		vids, sig, err := a.yt.secuenciaShorts(seq)
		a.hacer(func() {
			f.Cargando = false
			if err != nil {
				logf("más Shorts: %v", err)
				return
			}
			f.Videos, f.Token = juntar(f.Videos, vids), sig
		})
	}()
}

func juntar(a, b []Video) []Video {
	vistos := map[string]bool{}
	for _, v := range a {
		vistos[v.ID] = true
	}
	for _, v := range b {
		if !vistos[v.ID] {
			vistos[v.ID] = true
			a = append(a, v)
		}
	}
	return a
}
