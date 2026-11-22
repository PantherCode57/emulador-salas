package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pokemulador/nds/puente/ig"
)

// Instagram para la DS: el de la consola (ig/), con la sesión de la dueña de la DS. La sesión la pone
// la app de Android: ella entra en instagram.com en el móvil (en la app Puente DS) y la app deja las
// cookies en <datos>/instagram/sesion-nueva; el puente las comprueba con Instagram y las guarda.
// La DS pide:
//
//	IG ESTADO              R 1 / ESTADO <sin-cuenta|comprobando|bien|caducada|reto|limite>\t<usuario>
//	IG INICIO              R n / INICIO <hay más 0/1>, y P\t<clave>\t<usuario>\t<texto>\t<likes>\t<comentarios>\t<cuándo>\t<me gusta 0/1>\t<medios>
//	IG MAS                 lo mismo, con lo siguiente del inicio
//	IG HISTORIAS           R n / HISTORIAS, y H\t<id>\t<usuario>\t<vista 0/1>
//	IG HISTORIA <id>       R n / HISTORIA\t<usuario>, y S\t<medio>\t<cuándo>
//	IG CHATS               R n / CHATS, y C\t<id>\t<nombre>\t<sin leer 0/1>\t<cuándo>\t<último>
//	IG CHAT <id>           R n / CHAT\t<nombre>, y M\t<mío 0/1>\t<hora>\t<quién>\t<texto>
//	IG ENVIAR <id>\t<txt>  R 1 / OK
//	IG MEGUSTA <clave> <0/1>  R 1 / OK
//
// <medios>: los de la publicación separados por espacios: «f<clave de la foto>» o, si es un vídeo,
// «v<clave de la foto>:<clave del vídeo>». Las fotos se piden con IMG y los vídeos con VER.

type instagramDS interface {
	estado() (estado, usuario string)
	inicio(cursor string) (ig.Pagina, error)
	historias() ([]ig.Bandeja, error)
	historiasDe(id string) ([]ig.Post, error)
	chats() ([]ig.Chat, error)
	conversacion(id string) ([]ig.Mensaje, error)
	enviar(id, texto string) error
	megusta(pk string, si bool) error
}

func (p *puente) ordenIG(linea string) []string {
	if p.ig == nil {
		return []string{"ERROR este puente no lleva Instagram"}
	}
	cmd, arg, _ := strings.Cut(linea, " ")
	reg := p.registro()
	switch cmd {
	case "ESTADO":
		e, u := p.ig.estado()
		return []string{"ESTADO " + e + "\t" + limpiar(u, 40)}
	case "INICIO", "MAS":
		p.muIG.Lock()
		cursor := p.igCursor
		p.muIG.Unlock()
		if cmd == "INICIO" {
			cursor = ""
		} else if cursor == "" {
			return []string{"INICIO 0"}
		}
		pg, err := p.ig.inicio(cursor)
		if err != nil {
			return errorIG(err)
		}
		p.muIG.Lock()
		p.igCursor = ""
		if pg.Mas {
			p.igCursor = pg.Cursor
		}
		p.muIG.Unlock()
		l := []string{"INICIO " + uno(pg.Mas)}
		for _, po := range pg.Posts {
			if len(l) > 40 {
				break
			}
			l = append(l, strings.Join([]string{"P", reg.clave("pk:" + po.PK), limpiar(po.Usuario, 40),
				limpiar(po.Texto, 300), strconv.Itoa(po.Likes), strconv.Itoa(po.Comentarios), cuando(po.Cuando),
				uno(po.MeGusta), medios(reg, po.Medios)}, "\t"))
		}
		return l
	case "HISTORIAS":
		bs, err := p.ig.historias()
		if err != nil {
			return errorIG(err)
		}
		l := []string{"HISTORIAS"}
		for _, b := range bs {
			if len(l) > 60 {
				break
			}
			l = append(l, strings.Join([]string{"H", limpiar(b.ID, 40), limpiar(b.Usuario, 40), uno(b.Vista)}, "\t"))
		}
		return l
	case "HISTORIA":
		ps, err := p.ig.historiasDe(arg)
		if err != nil {
			return errorIG(err)
		}
		usuario := ""
		if len(ps) > 0 {
			usuario = ps[0].Usuario
		}
		l := []string{"HISTORIA\t" + limpiar(usuario, 40)}
		for _, po := range ps {
			if len(l) > 60 || len(po.Medios) == 0 {
				continue
			}
			l = append(l, "S\t"+medios(reg, po.Medios[:1])+"\t"+cuando(po.Cuando))
		}
		return l
	case "CHATS":
		cs, err := p.ig.chats()
		if err != nil {
			return errorIG(err)
		}
		l := []string{"CHATS"}
		for _, c := range cs {
			if len(l) > 40 {
				break
			}
			l = append(l, strings.Join([]string{"C", limpiar(c.ID, 80), limpiar(c.Titulo, 60), uno(c.SinLeer),
				cuando(c.Cuando), limpiar(c.Ultimo, 90)}, "\t"))
		}
		return l
	case "CHAT":
		ms, err := p.ig.conversacion(arg)
		if err != nil {
			return errorIG(err)
		}
		nombre := ""
		if cs, err := p.ig.chats(); err == nil {
			for _, c := range cs {
				if c.ID == arg {
					nombre = c.Titulo
				}
			}
		}
		l := []string{"CHAT\t" + limpiar(nombre, 60)}
		if len(ms) > 40 {
			ms = ms[len(ms)-40:]
		}
		for _, m := range ms {
			quien := m.Quien
			if m.Mio {
				quien = ""
			}
			l = append(l, strings.Join([]string{"M", uno(m.Mio), hora(m.Cuando), limpiar(quien, 40), limpiar(m.Texto, 400)}, "\t"))
		}
		return l
	case "ENVIAR":
		id, texto, ok := strings.Cut(arg, "\t")
		if !ok || strings.TrimSpace(texto) == "" {
			return []string{"ERROR falta el mensaje"}
		}
		if err := p.ig.enviar(id, strings.ReplaceAll(texto, "\n", " ")); err != nil {
			return errorIG(err)
		}
		return []string{"OK"}
	case "MEGUSTA":
		clave, si, _ := strings.Cut(arg, " ")
		u, ok := reg.url(clave)
		if !ok || !strings.HasPrefix(u, "pk:") {
			return []string{"ERROR esa publicación ya no está"}
		}
		if err := p.ig.megusta(strings.TrimPrefix(u, "pk:"), si == "1"); err != nil {
			return errorIG(err)
		}
		return []string{"OK"}
	}
	return []string{"ERROR no sé hacer «IG " + limpiar(cmd, 20) + "»"}
}

func errorIG(err error) []string {
	switch {
	case errors.Is(err, ig.ErrSinCuenta):
		return []string{"ERROR falta entrar en Instagram (en la app Puente DS del móvil)"}
	case errors.Is(err, ig.ErrReto):
		return []string{"ERROR Instagram quiere que confirmes que eres tú: entra otra vez desde el móvil"}
	case errors.Is(err, ig.ErrSesion):
		return []string{"ERROR la sesión de Instagram ha caducado: entra otra vez desde el móvil"}
	case errors.Is(err, ig.ErrLimite):
		return []string{"ERROR Instagram no contesta ahora: prueba en un rato"}
	}
	return []string{"ERROR " + limpiar(err.Error(), 120)}
}

func medios(reg *registro, ms []ig.Medio) string {
	var l []string
	for _, m := range ms {
		if m.EsVideo() {
			l = append(l, "v"+reg.clave(m.Foto)+":"+reg.clave(m.Video))
		} else {
			l = append(l, "f"+reg.clave(m.Foto))
		}
		if len(l) == 10 {
			break
		}
	}
	return strings.Join(l, " ")
}

func uno(si bool) string {
	if si {
		return "1"
	}
	return "0"
}

// cuando: hace cuánto, en corto («ahora», «5 min», «3 h», «2 d», «4 nov»).
func cuando(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "ahora"
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d d", int(d.Hours()/24))
	}
	return fmt.Sprintf("%d %s", t.Day(), mesesCortos[t.Month()-1])
}

var mesesCortos = [12]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// hora: la hora de un mensaje (si es de otro día, el día).
func hora(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	if y, m, d := time.Now().Date(); t.Year() == y && t.Month() == m && t.Day() == d {
		return t.Format("15:04")
	}
	if time.Since(t) < 6*24*time.Hour {
		return [7]string{"dom", "lun", "mar", "mié", "jue", "vie", "sáb"}[t.Weekday()]
	}
	return fmt.Sprintf("%d %s", t.Day(), mesesCortos[t.Month()-1])
}

// ---- El de verdad -------------------------------------------------------------------------------

type igReal struct {
	c       *ig.Cliente
	carpeta string

	mu      sync.Mutex
	estadoX string // comprobando, caducada, reto o limite ("" = lo normal)
	cache   map[string]cacheIG
}

type cacheIG struct {
	cuando time.Time
	v      any
}

func nuevoIGReal(carpeta string) *igReal {
	r := &igReal{c: ig.Nuevo(carpeta), carpeta: carpeta, cache: map[string]cacheIG{}}
	go r.vigilarSesion()
	return r
}

// vigilarSesion: si la app ha dejado una sesión nueva (o que hay que salir), se hace.
func (r *igReal) vigilarSesion() {
	for {
		nueva := filepath.Join(r.carpeta, "sesion-nueva")
		if b, err := os.ReadFile(nueva); err == nil {
			_ = os.Remove(nueva)
			r.mu.Lock()
			r.estadoX = "comprobando"
			r.cache = map[string]cacheIG{}
			r.mu.Unlock()
			err := r.c.Poner(string(b))
			r.mu.Lock()
			switch {
			case err == nil || errors.Is(err, ig.ErrLimite):
				r.estadoX = ""
				log.Printf("instagram: sesión nueva de %s", r.c.Usuario())
			case errors.Is(err, ig.ErrReto):
				r.estadoX = "reto"
			default:
				r.estadoX = "caducada"
				log.Printf("instagram: la sesión nueva no vale: %v", err)
			}
			r.mu.Unlock()
		}
		salir := filepath.Join(r.carpeta, "salir")
		if _, err := os.Stat(salir); err == nil {
			_ = os.Remove(salir)
			r.c.Olvidar()
			r.mu.Lock()
			r.estadoX = ""
			r.cache = map[string]cacheIG{}
			r.mu.Unlock()
			log.Printf("instagram: sesión cerrada desde el móvil")
		}
		time.Sleep(2 * time.Second)
	}
}

func (r *igReal) estado() (string, string) {
	r.mu.Lock()
	e := r.estadoX
	r.mu.Unlock()
	if e == "comprobando" {
		return e, ""
	}
	if !r.c.Tiene() {
		return "sin-cuenta", ""
	}
	if e == "" {
		e = "bien"
	}
	return e, r.c.Usuario()
}

// anotar: lo que dice un error de la sesión (para ESTADO).
func (r *igReal) anotar(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case err == nil:
		if r.estadoX == "limite" {
			r.estadoX = ""
		}
	case errors.Is(err, ig.ErrReto):
		r.estadoX = "reto"
	case errors.Is(err, ig.ErrSesion):
		r.estadoX = "caducada"
	case errors.Is(err, ig.ErrLimite):
		r.estadoX = "limite"
	}
}

// guardado: lo de hace poco (para no pedírselo a Instagram cada vez que la DS vuelve a una lista).
func (r *igReal) guardado(clave string, dura time.Duration, f func() (any, error)) (any, error) {
	r.mu.Lock()
	c, ok := r.cache[clave]
	r.mu.Unlock()
	if ok && time.Since(c.cuando) < dura {
		return c.v, nil
	}
	v, err := f()
	r.anotar(err)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[clave] = cacheIG{time.Now(), v}
	r.mu.Unlock()
	return v, nil
}

func (r *igReal) olvidarGuardado(prefijo string) {
	r.mu.Lock()
	for k := range r.cache {
		if strings.HasPrefix(k, prefijo) {
			delete(r.cache, k)
		}
	}
	r.mu.Unlock()
}

func (r *igReal) inicio(cursor string) (ig.Pagina, error) {
	v, err := r.guardado("inicio:"+cursor, time.Minute, func() (any, error) { return r.c.Inicio(cursor) })
	if err != nil {
		return ig.Pagina{}, err
	}
	return v.(ig.Pagina), nil
}

func (r *igReal) historias() ([]ig.Bandeja, error) {
	v, err := r.guardado("historias", time.Minute, func() (any, error) { return r.c.Historias() })
	if err != nil {
		return nil, err
	}
	return v.([]ig.Bandeja), nil
}

func (r *igReal) historiasDe(id string) ([]ig.Post, error) {
	v, err := r.guardado("historia:"+id, 2*time.Minute, func() (any, error) { return r.c.HistoriasDe(id) })
	if err != nil {
		return nil, err
	}
	return v.([]ig.Post), nil
}

func (r *igReal) chats() ([]ig.Chat, error) {
	v, err := r.guardado("chats", 8*time.Second, func() (any, error) { return r.c.Chats() })
	if err != nil {
		return nil, err
	}
	return v.([]ig.Chat), nil
}

func (r *igReal) conversacion(id string) ([]ig.Mensaje, error) {
	v, err := r.guardado("chat:"+id, 4*time.Second, func() (any, error) { return r.c.Conversacion(id) })
	if err != nil {
		return nil, err
	}
	return v.([]ig.Mensaje), nil
}

func (r *igReal) enviar(id, texto string) error {
	err := r.c.Enviar(id, texto)
	r.anotar(err)
	r.olvidarGuardado("chat")
	return err
}

func (r *igReal) megusta(pk string, si bool) error {
	err := r.c.MeGusta(pk, si)
	r.anotar(err)
	r.olvidarGuardado("inicio:")
	return err
}

// ---- El de mentira (para las pruebas) -----------------------------------------------------------

type igFalso struct {
	mu       sync.Mutex
	posts    []ig.Post
	mensajes map[string][]ig.Mensaje
}

func nuevoIGFalso() *igFalso {
	hace := func(d time.Duration) time.Time { return time.Now().Add(-d) }
	return &igFalso{
		posts: []ig.Post{
			{PK: "1001", Usuario: "lucia.dibuja", Texto: "Mi dibujo nuevo 🎨 ¿os gusta?", Likes: 87, Comentarios: 12, Cuando: hace(2 * time.Hour),
				Medios: []ig.Medio{{Foto: "falso:foto-1", W: 1080, H: 1080}, {Foto: "falso:foto-2", W: 1080, H: 1350}}},
			{PK: "1002", Usuario: "clase2b", Texto: "Excursión al museo 🚌", Likes: 30, Comentarios: 4, Cuando: hace(26 * time.Hour), MeGusta: true,
				Medios: []ig.Medio{{Foto: "falso:foto-3", Video: "falso:video", W: 720, H: 1280, Duracion: 6}}},
			{PK: "1003", Usuario: "pablo_pk", Texto: "Mi equipo de Pokémon", Likes: 15, Comentarios: 1, Cuando: hace(3 * 24 * time.Hour),
				Medios: []ig.Medio{{Foto: "falso:foto-4", W: 1080, H: 1080}}},
		},
		mensajes: map[string][]ig.Mensaje{
			"t1": {{Quien: "Lucía", Texto: "¿Viste mi dibujo?", Cuando: hace(20 * time.Minute)}, {Mio: true, Texto: "Síii, es precioso", Cuando: hace(18 * time.Minute)}},
			"t2": {{Quien: "Hugo", Texto: "Mañana a las 5 en el parque", Cuando: hace(5 * time.Hour)}},
		},
	}
}

func (f *igFalso) estado() (string, string) { return "bien", "duena.de.la.ds" }

func (f *igFalso) inicio(cursor string) (ig.Pagina, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if cursor == "" {
		return ig.Pagina{Posts: append([]ig.Post(nil), f.posts[:2]...), Cursor: "2", Mas: true}, nil
	}
	return ig.Pagina{Posts: append([]ig.Post(nil), f.posts[2:]...)}, nil
}

func (f *igFalso) historias() ([]ig.Bandeja, error) {
	return []ig.Bandeja{{ID: "501", Usuario: "lucia.dibuja"}, {ID: "502", Usuario: "clase2b"}, {ID: "503", Usuario: "pablo_pk", Vista: true}}, nil
}

func (f *igFalso) historiasDe(id string) ([]ig.Post, error) {
	ahora := time.Now().Add(-time.Hour)
	return []ig.Post{
		{Usuario: "lucia.dibuja", Cuando: ahora, Medios: []ig.Medio{{Foto: "falso:historia-" + id + "-1", W: 1080, H: 1920}}},
		{Usuario: "lucia.dibuja", Cuando: ahora, Medios: []ig.Medio{{Foto: "falso:historia-" + id + "-2", W: 1080, H: 1920}}},
	}, nil
}

func (f *igFalso) chats() ([]ig.Chat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ultimo := func(id string) (string, time.Time) {
		ms := f.mensajes[id]
		m := ms[len(ms)-1]
		return m.Texto, m.Cuando
	}
	u1, c1 := ultimo("t1")
	u2, c2 := ultimo("t2")
	return []ig.Chat{{ID: "t1", Titulo: "Lucía", Ultimo: u1, Cuando: c1, SinLeer: true}, {ID: "t2", Titulo: "Hugo", Ultimo: u2, Cuando: c2}}, nil
}

func (f *igFalso) conversacion(id string) ([]ig.Mensaje, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ms, ok := f.mensajes[id]
	if !ok {
		return nil, errors.New("ese chat no está")
	}
	return append([]ig.Mensaje(nil), ms...), nil
}

func (f *igFalso) enviar(id, texto string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.mensajes[id]; !ok {
		return errors.New("ese chat no está")
	}
	f.mensajes[id] = append(f.mensajes[id], ig.Mensaje{Mio: true, Texto: texto, Cuando: time.Now()})
	log.Printf("IG ENVIADO %s %q", id, texto)
	return nil
}

func (f *igFalso) megusta(pk string, si bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.posts {
		if f.posts[i].PK == pk {
			f.posts[i].MeGusta = si
		}
	}
	log.Printf("IG MEGUSTA %s %s", pk, uno(si))
	return nil
}
