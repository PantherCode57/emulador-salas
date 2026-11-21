package main

import (
	"log"
	"strings"
	"sync"
	"time"
)

// Lo que contesta WhatsApp (el de verdad, wa_real.go, o el de mentira de abajo).
type waChat struct {
	JID      string `json:"jid"`
	Nombre   string `json:"nombre"`
	Grupo    bool   `json:"grupo"`
	NoLeidos int    `json:"no_leidos"`
	Hora     string `json:"hora"`
	Previa   string `json:"previa"`
}

type waMensaje struct {
	Mio   bool   `json:"mio"`
	De    string `json:"de"`
	Texto string `json:"texto"`
	Hora  string `json:"hora"`
	Error bool   `json:"error"`
}

type waRespuesta struct {
	Estado   string      `json:"estado"`
	Ver      int         `json:"ver"`
	Chats    []waChat    `json:"chats"`
	Nombre   string      `json:"nombre"`
	Mensajes []waMensaje `json:"mensajes"`
	Codigo   string      `json:"codigo"` // el de vincular, para meterlo en el móvil
	Error    string      `json:"error"`
}

type whatsapp interface {
	pedir(orden string) (waRespuesta, error)
}

func logEnviado(jid, texto string) {
	log.Printf("ENVIADO %s %q", jid, texto)
}

// waFalso: unos chats de mentira para probar la DS sin WhatsApp. Lo que se manda se apunta en el
// registro («ENVIADO …») para que la prueba lo compruebe.
type waFalso struct {
	mu        sync.Mutex
	ver       int
	chats     []waChat
	mensajes  map[string][]waMensaje
	vinculado bool
	vinculaEn time.Time // se vincula sola 2 s después de pedir el código
}

func nuevoWAFalso(vinculado bool) *waFalso {
	w := &waFalso{ver: 1, mensajes: map[string][]waMensaje{}, vinculado: vinculado}
	add := func(jid, nombre string, noLeidos int, ms ...waMensaje) {
		w.chats = append(w.chats, waChat{JID: jid, Nombre: nombre, Grupo: strings.HasSuffix(jid, "@g.us"), NoLeidos: noLeidos})
		w.mensajes[jid] = ms
	}
	add("111@s.whatsapp.net", "Lucía", 1,
		waMensaje{Texto: "¿Has probado ya la DS? 😂", Hora: "17:58"},
		waMensaje{Mio: true, Texto: "Sí, ya arranca Panther OS", Hora: "18:01"},
		waMensaje{Texto: "Qué guapo!! Mándame una foto cuando puedas, que quiero verla", Hora: "18:02"})
	add("222@g.us", "Clase 2ºB", 2,
		waMensaje{De: "Hugo", Texto: "Mañana hay examen de mates", Hora: "16:40"},
		waMensaje{De: "Sara", Texto: "Nooo 😭", Hora: "16:41"})
	add("333@s.whatsapp.net", "Mamá", 0,
		waMensaje{Texto: "Llámame cuando salgas", Hora: "14:10"},
		waMensaje{Mio: true, Texto: "Vale 👍", Hora: "14:12"})
	add("444@s.whatsapp.net", "Pablo", 0,
		waMensaje{Texto: "¿Jugamos a Pokémon esta tarde?", Hora: "ayer"})
	return w
}

func (w *waFalso) pedir(orden string) (waRespuesta, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.vinculado && !w.vinculaEn.IsZero() && time.Now().After(w.vinculaEn) {
		w.vinculado = true
		w.ver++
	}
	r := waRespuesta{Estado: "conectado", Ver: w.ver}
	cmd, arg, _ := strings.Cut(orden, " ")
	if !w.vinculado {
		r.Estado = "sin-vincular"
		if !w.vinculaEn.IsZero() {
			r.Estado = "vinculando"
		}
		switch cmd {
		case "estado":
		case "vincular":
			if len(arg) < 7 || strings.HasPrefix(arg, "0") {
				r.Error = "el número tiene que llevar el prefijo del país (34 en España) y sin 0 delante"
				return r, nil
			}
			log.Printf("VINCULAR %s", arg)
			w.vinculaEn = time.Now().Add(2 * time.Second)
			r.Estado, r.Codigo = "vinculando", "PANT-HER1"
		default:
			r.Error = "WhatsApp no está vinculado"
		}
		return r, nil
	}
	switch cmd {
	case "vincular":
		r.Error = "WhatsApp ya está vinculado"
	case "chats":
		for _, c := range w.chats {
			ms := w.mensajes[c.JID]
			u := ms[len(ms)-1]
			c.Hora, c.Previa = u.Hora, u.Texto
			if u.Mio {
				c.Previa = "Tú: " + u.Texto
			} else if u.De != "" {
				c.Previa = u.De + ": " + u.Texto
			}
			r.Chats = append(r.Chats, c)
		}
	case "chat":
		ms, ok := w.mensajes[arg]
		if !ok {
			r.Error = "no está ese chat"
			return r, nil
		}
		for i := range w.chats {
			if w.chats[i].JID == arg {
				r.Nombre = w.chats[i].Nombre
				w.chats[i].NoLeidos = 0
			}
		}
		r.Mensajes = ms
	case "enviar":
		jid, texto, _ := strings.Cut(arg, "\t")
		if _, ok := w.mensajes[jid]; !ok {
			r.Error = "no está ese chat"
			return r, nil
		}
		w.mensajes[jid] = append(w.mensajes[jid], waMensaje{Mio: true, Texto: texto, Hora: "18:45"})
		w.ver++
		r.Ver = w.ver
		logEnviado(jid, texto)
	}
	return r, nil
}
