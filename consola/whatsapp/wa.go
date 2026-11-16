package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

const maxMensajes = 60

// Mensaje: lo que guardamos de cada mensaje para enseñarlo.
type Mensaje struct {
	ID     string    `json:"id"`
	De     string    `json:"de"`     // nombre de quien lo manda (en grupos)
	Remite string    `json:"remite"` // JID de quien lo manda
	Mio    bool      `json:"mio"`
	Texto  string    `json:"texto"`
	Hora   time.Time `json:"hora"`
	Error  bool      `json:"error,omitempty"`
	Tipo   string    `json:"tipo,omitempty"` // "foto" o "sticker" (solo para ver)
	Pie    string    `json:"pie,omitempty"`  // pie de foto
}

// Chat: una conversación con sus últimos mensajes.
type Chat struct {
	JID      string     `json:"jid"`
	Nombre   string     `json:"nombre"`
	Grupo    bool       `json:"grupo"`
	Ultimo   time.Time  `json:"ultimo"`
	NoLeidos int        `json:"no_leidos"`
	Mensajes []*Mensaje `json:"mensajes"`
}

func (c *Chat) añadir(m *Mensaje) bool {
	for i, v := range c.Mensajes {
		if v.ID == m.ID && m.ID != "" {
			c.Mensajes[i] = m
			return false
		}
	}
	c.Mensajes = append(c.Mensajes, m)
	sort.SliceStable(c.Mensajes, func(i, j int) bool { return c.Mensajes[i].Hora.Before(c.Mensajes[j].Hora) })
	if len(c.Mensajes) > maxMensajes {
		c.Mensajes = c.Mensajes[len(c.Mensajes)-maxMensajes:]
	}
	if m.Hora.After(c.Ultimo) {
		c.Ultimo = m.Hora
	}
	return true
}

func (c *Chat) vistaPrevia() string {
	if len(c.Mensajes) == 0 {
		return ""
	}
	m := c.Mensajes[len(c.Mensajes)-1]
	switch {
	case m.Mio:
		return "Tú: " + m.Texto
	case c.Grupo && m.De != "":
		return m.De + ": " + m.Texto
	}
	return m.Texto
}

// estadoWA: lo que la pantalla necesita saber de la conexión.
type estadoWA int

const (
	waArrancando estadoWA = iota
	waQR                  // esperando a que escanees el código
	waQRCaducado          // el código ha caducado: hay que pedir otro
	waConectando
	waConectado
	waSinRed
	waDesvinculado
)

// wa lleva la sesión de WhatsApp. Todo lo que cambia los datos se hace en el bucle de la app
// (con app.hacer), así que aquí no hacen falta cerrojos para los chats.
type wa struct {
	app          *app
	contenedor   *sqlstore.Container
	cli          *whatsmeow.Client
	chats        map[string]*Chat
	sucio        bool
	nombres      map[string]string // pushnames vistos
	pedidoNombre map[string]bool
	// chats a los que ya se ha pedido el historial al móvil
	pedidoHistorial map[string]bool
	lidPN           map[string]string // código LID -> número (lo aprendido del historial y los mensajes)
}

// fotoAntigua: una foto o sticker que llegó antes de que el programa supiera enseñarlos (solo
// tiene el texto «[Foto]»). Para verla hay que volver a pedir ese mensaje al móvil.
func fotoAntigua(m *Mensaje) bool {
	return m.Tipo == "" && (strings.HasPrefix(m.Texto, "[Foto]") || strings.HasPrefix(m.Texto, "[Sticker]"))
}

// pedirAntiguas pide al móvil (historial bajo demanda) los últimos mensajes de un chat que tenga
// fotos o stickers antiguos; al llegar, se cambian por las imágenes. Una vez por chat.
func (w *wa) pedirAntiguas(c *Chat) {
	if w.cli == nil || w.app.estado != waConectado || w.pedidoHistorial[c.JID] || len(c.Mensajes) == 0 {
		return
	}
	hay := false
	for _, m := range c.Mensajes {
		if fotoAntigua(m) {
			hay = true
			break
		}
	}
	if !hay {
		return
	}
	chat, err := types.ParseJID(c.JID)
	if err != nil {
		return
	}
	w.pedidoHistorial[c.JID] = true
	// El móvil manda los mensajes anteriores al más nuevo que conocemos.
	ultimo := c.Mensajes[len(c.Mensajes)-1]
	info := &types.MessageInfo{MessageSource: types.MessageSource{Chat: chat, IsFromMe: ultimo.Mio, IsGroup: c.Grupo},
		ID: types.MessageID(ultimo.ID), Timestamp: ultimo.Hora}
	pedido := w.cli.BuildHistorySyncRequest(info, 50)
	cli := w.cli
	go func() {
		if _, err := cli.SendPeerMessage(context.Background(), pedido); err != nil {
			log.Printf("pedir historial: %v", err)
			w.app.hacer(func() { delete(w.pedidoHistorial, c.JID) })
			return
		}
		w.app.hacer(func() { w.app.aviso("Pidiendo al móvil las fotos antiguas de este chat…") })
	}()
}

func archivoChats() string { return filepath.Join(carpetaDatos, "chats.json") }

func nuevoWA(a *app) (*wa, error) {
	store.SetOSInfo("Panther OS", [3]uint32{1, 0, 0})
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(carpetaDatos, "sesion.db")+
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	c := sqlstore.NewWithDB(db, "sqlite3", waLog.Stdout("BD", "WARN", false))
	if err := c.Upgrade(context.Background()); err != nil {
		return nil, fmt.Errorf("base de datos: %w", err)
	}
	w := &wa{app: a, contenedor: c, chats: map[string]*Chat{}, nombres: map[string]string{}, pedidoNombre: map[string]bool{}, pedidoHistorial: map[string]bool{}, lidPN: map[string]string{}}
	w.cargar()
	return w, nil
}

func (w *wa) cargar() {
	b, err := os.ReadFile(archivoChats())
	if err != nil {
		return
	}
	var lista []*Chat
	if json.Unmarshal(b, &lista) == nil {
		for _, c := range lista {
			w.chats[c.JID] = c
		}
	}
}

func (w *wa) guardar() {
	if !w.sucio {
		return
	}
	w.sucio = false
	b, err := json.Marshal(w.ordenados())
	if err != nil {
		return
	}
	escribirAtomico(archivoChats(), b)
}

// visibles: los chats que se enseñan en la lista (los que tienen algún mensaje o sin leer).
// El historial trae también todos los contactos sin conversación: esos no salen.
func (w *wa) visibles() []*Chat {
	var r []*Chat
	for _, c := range w.ordenados() {
		if len(c.Mensajes) > 0 || c.NoLeidos > 0 {
			r = append(r, c)
		}
	}
	return r
}

func (w *wa) ordenados() []*Chat {
	lista := make([]*Chat, 0, len(w.chats))
	for _, c := range w.chats {
		lista = append(lista, c)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Ultimo.After(lista[j].Ultimo) })
	return lista
}

// arrancar conecta con la sesión guardada o, si no hay, saca el QR para vincular.
func (w *wa) arrancar() {
	ctx := context.Background()
	disp, err := w.contenedor.GetFirstDevice(ctx)
	if err != nil {
		log.Printf("no se pudo leer la sesión: %v", err)
		disp = w.contenedor.NewDevice()
	}
	w.cli = whatsmeow.NewClient(disp, waLog.Stdout("WA", "WARN", false))
	w.cli.AddEventHandler(func(evt any) { w.app.hacer(func() { w.evento(evt) }) })
	if w.cli.Store.ID == nil {
		w.vincular()
		return
	}
	w.app.estado = waConectando
	go func() {
		if err := w.cli.Connect(); err != nil {
			log.Printf("conectar: %v", err)
			w.app.hacer(func() { w.app.estado = waSinRed })
		}
	}()
}

// vincular pide códigos QR hasta que se escanee uno o caduquen.
func (w *wa) vincular() {
	w.app.estado = waArrancando
	go func() {
		qr, err := w.cli.GetQRChannel(context.Background())
		if err != nil {
			log.Printf("QR: %v", err)
			return
		}
		if err := w.cli.Connect(); err != nil {
			log.Printf("conectar para el QR: %v", err)
			w.app.hacer(func() { w.app.estado = waSinRed })
			return
		}
		for e := range qr {
			switch e.Event {
			case "code":
				codigo := e.Code
				w.app.hacer(func() { w.app.qr = codigo; w.app.estado = waQR })
			case "success":
				w.app.hacer(func() { w.app.estado = waConectando; w.app.aviso("Vinculado. Cargando tus chats…") })
			case "timeout":
				w.app.hacer(func() { w.app.estado = waQRCaducado })
				w.cli.Disconnect()
			default:
				log.Printf("QR: %s %v", e.Event, e.Error)
			}
		}
	}()
}

// otraVez vuelve a sacar un QR (tras caducar o tras desvincular).
func (w *wa) otraVez() {
	if w.cli != nil {
		w.cli.Disconnect()
	}
	w.arrancar()
}

func (w *wa) desvincular() {
	if w.cli == nil {
		return
	}
	cli := w.cli
	go func() {
		if err := cli.Logout(context.Background()); err != nil {
			log.Printf("cerrar sesión: %v", err)
		}
		w.app.hacer(func() {
			w.chats = map[string]*Chat{}
			w.sucio = true
			w.guardar()
			w.app.estado = waDesvinculado
		})
	}()
}

func (w *wa) cerrar() {
	if w.cli != nil {
		w.cli.Disconnect()
	}
	w.guardar()
}

func (w *wa) evento(evt any) {
	switch e := evt.(type) {
	case *events.Connected:
		w.app.estado = waConectado
		w.renombrar()
	case *events.AppStateSyncComplete: // ha llegado la agenda: nombres nuevos
		w.renombrar()
	case *events.Disconnected:
		if w.app.estado == waConectado {
			w.app.estado = waSinRed
		}
	case *events.StreamReplaced:
		w.app.estado = waSinRed
		w.app.aviso("Has abierto WhatsApp en otro sitio con esta sesión")
	case *events.LoggedOut:
		w.chats = map[string]*Chat{}
		w.sucio = true
		w.app.estado = waDesvinculado
	case *events.PushName:
		if e.NewPushName != "" {
			w.nombres[e.JID.ToNonAD().String()] = e.NewPushName
		}
	case *events.HistorySync:
		w.historial(e)
	case *events.Message:
		w.mensaje(e, true)
	}
	w.app.redibujar()
}

func (w *wa) chat(jid types.JID) *Chat {
	k := jid.ToNonAD().String()
	c := w.chats[k]
	if c == nil {
		c = &Chat{JID: k, Grupo: jid.Server == types.GroupServer}
		w.chats[k] = c
	}
	return c
}

func ignorar(jid types.JID) bool {
	return jid == types.StatusBroadcastJID || jid.Server == types.NewsletterServer || jid.Server == types.BroadcastServer
}

func (w *wa) mensaje(e *events.Message, enVivo bool) {
	if ignorar(e.Info.Chat) || e.Message == nil {
		return
	}
	texto := textoDe(e.Message)
	if texto == "" {
		return
	}
	if !e.Info.SenderAlt.IsEmpty() {
		w.aprender(e.Info.Sender, e.Info.SenderAlt)
	}
	if !e.Info.RecipientAlt.IsEmpty() && !e.Info.IsGroup {
		w.aprender(e.Info.Chat, e.Info.RecipientAlt)
	}
	if e.Info.PushName != "" && !e.Info.IsFromMe {
		w.nombres[e.Info.Sender.ToNonAD().String()] = e.Info.PushName
		if !e.Info.SenderAlt.IsEmpty() {
			w.nombres[e.Info.SenderAlt.ToNonAD().String()] = e.Info.PushName
		}
	}
	c := w.chat(e.Info.Chat)
	m := &Mensaje{
		ID:     e.Info.ID,
		Remite: e.Info.Sender.ToNonAD().String(),
		Mio:    e.Info.IsFromMe,
		Texto:  w.app.f.limpiar(texto),
		Hora:   e.Info.Timestamp,
	}
	if !m.Mio {
		m.De = w.nombreDe(e.Info.Sender, e.Info.PushName)
	}
	if e.IsEdit {
		m.Texto += " (editado)"
	}
	if tipo, pie, sub, mini := medioDe(e.Message); tipo != "" && m.ID != "" && w.app.media != nil {
		m.Tipo, m.Pie = tipo, w.app.f.limpiar(pie)
		w.app.media.guardar(m, sub, mini) // no se descarga: solo al abrirla con Y, y a la memoria
	}
	nuevo := c.añadir(m)
	w.sucio = true
	if c.Nombre == "" {
		w.ponerNombre(c, e.Info.Chat, e.Info.PushName, e.Info.IsFromMe)
	}
	if enVivo && nuevo && !m.Mio {
		if w.app.chatAbierto() != c.JID {
			c.NoLeidos++
			quien := c.Nombre
			if c.Grupo && m.De != "" {
				quien = c.Nombre + " · " + m.De
			}
			w.app.avisoNuevo(c, m, quien)
		} else {
			w.leido(c, []*Mensaje{m})
		}
	}
}

func (w *wa) historial(e *events.HistorySync) {
	for _, p := range e.Data.GetPushnames() {
		if p.GetPushname() != "" {
			w.nombres[p.GetID()] = p.GetPushname()
		}
	}
	for _, conv := range e.Data.GetConversations() {
		jid, err := types.ParseJID(conv.GetID())
		if err != nil || ignorar(jid) {
			continue
		}
		c := w.chat(jid)
		if pn := conv.GetPnJID(); pn != "" {
			if p, err := types.ParseJID(pn); err == nil {
				w.aprender(jid, p)
			}
		}
		if lid := conv.GetLidJID(); lid != "" {
			if l, err := types.ParseJID(lid); err == nil {
				w.aprender(l, jid)
			}
		}
		if n := conv.GetName(); n != "" {
			c.Nombre = w.app.f.limpiar(n)
		} else if n := conv.GetDisplayName(); n != "" && c.Nombre == "" {
			c.Nombre = w.app.f.limpiar(n)
		}
		if u := conv.GetUnreadCount(); u > 0 {
			c.NoLeidos = int(u)
		}
		if t := conv.GetConversationTimestamp(); t > 0 {
			if h := time.Unix(int64(t), 0); h.After(c.Ultimo) {
				c.Ultimo = h
			}
		}
		for _, hm := range conv.GetMessages() {
			ev, err := w.cli.ParseWebMessage(jid, hm.GetMessage())
			if err == nil {
				w.mensaje(ev, false)
			}
		}
		if c.Nombre == "" {
			w.ponerNombre(c, jid, "", false)
		}
	}
	w.renombrar()
	w.sucio = true
}

// ponerNombre busca cómo se llama un chat: agenda, nombre del grupo o el que se pone cada uno.
func (w *wa) ponerNombre(c *Chat, jid types.JID, pushName string, deMi bool) {
	if c.Grupo {
		if w.pedidoNombre[c.JID] || w.cli == nil {
			return
		}
		w.pedidoNombre[c.JID] = true
		cli := w.cli
		go func() {
			info, err := cli.GetGroupInfo(context.Background(), jid)
			if err != nil || info == nil {
				return
			}
			nombre := info.Name
			w.app.hacer(func() { c.Nombre = w.app.f.limpiar(nombre); w.sucio = true })
		}()
		return
	}
	if deMi {
		pushName = ""
	}
	c.Nombre = w.nombreDe(jid, pushName)
}

// candidatos: las formas de llamar a una persona. WhatsApp usa cada vez más un código interno
// («LID», @lid) en vez del número; la agenda va por número, así que se busca también el número
// que le corresponde (la tabla de whatsmeow y lo aprendido del historial y de los mensajes).
func (w *wa) candidatos(jid types.JID) []types.JID {
	jid = jid.ToNonAD()
	r := []types.JID{jid}
	if jid.Server != types.HiddenUserServer {
		return r
	}
	if w.cli != nil && w.cli.Store.LIDs != nil {
		if pn, err := w.cli.Store.LIDs.GetPNForLID(context.Background(), jid); err == nil && !pn.IsEmpty() {
			r = append(r, pn.ToNonAD())
		}
	}
	if s := w.lidPN[jid.String()]; s != "" {
		if pn, err := types.ParseJID(s); err == nil {
			r = append(r, pn.ToNonAD())
		}
	}
	return r
}

func (w *wa) nombreDe(jid types.JID, pushName string) string {
	cs := w.candidatos(jid)
	if w.cli != nil && w.cli.Store.Contacts != nil {
		for _, c := range cs {
			if ci, err := w.cli.Store.Contacts.GetContact(context.Background(), c); err == nil && ci.Found {
				for _, n := range []string{ci.FullName, ci.FirstName, ci.PushName, ci.BusinessName} {
					if n != "" {
						return w.app.f.limpiar(n)
					}
				}
			}
		}
	}
	if pushName != "" {
		return w.app.f.limpiar(pushName)
	}
	for _, c := range cs {
		if n := w.nombres[c.String()]; n != "" {
			return w.app.f.limpiar(n)
		}
	}
	for _, c := range cs {
		if c.Server == types.DefaultUserServer {
			return "+" + c.User
		}
	}
	return "Contacto"
}

func sinNombre(n string) bool { return n == "" || n == "Contacto" || strings.HasPrefix(n, "+") }

// renombrar vuelve a buscar el nombre de los chats y remitentes que salían sin nombre (por
// ejemplo, al llegar la agenda o el historial después de los mensajes).
func (w *wa) renombrar() {
	for _, c := range w.chats {
		if len(c.Mensajes) == 0 {
			continue
		}
		jid, err := types.ParseJID(c.JID)
		if err != nil {
			continue
		}
		if !c.Grupo && sinNombre(c.Nombre) {
			if n := w.nombreDe(jid, ""); n != c.Nombre && !(n == "Contacto" && c.Nombre != "") {
				c.Nombre, w.sucio = n, true
			}
		}
		if c.Grupo && c.Nombre == "" {
			w.ponerNombre(c, jid, "", false)
		}
		for _, m := range c.Mensajes {
			if c.Grupo && !m.Mio && sinNombre(m.De) && m.Remite != "" {
				if r, err := types.ParseJID(m.Remite); err == nil {
					if n := w.nombreDe(r, ""); n != m.De && n != "Contacto" {
						m.De, w.sucio = n, true
					}
				}
			}
		}
	}
}

// aprender apunta qué número corresponde a cada código LID.
func (w *wa) aprender(a, b types.JID) {
	a, b = a.ToNonAD(), b.ToNonAD()
	if a.Server == types.DefaultUserServer && b.Server == types.HiddenUserServer {
		a, b = b, a
	}
	if a.Server == types.HiddenUserServer && b.Server == types.DefaultUserServer {
		w.lidPN[a.String()] = b.String()
	}
}

// leido avisa al móvil de que has visto los mensajes (los dos ticks azules).
func (w *wa) leido(c *Chat, ms []*Mensaje) {
	if w.cli == nil || w.app.estado != waConectado {
		return
	}
	chat, err := types.ParseJID(c.JID)
	if err != nil {
		return
	}
	porRemite := map[string][]types.MessageID{}
	for _, m := range ms {
		if !m.Mio && m.ID != "" {
			porRemite[m.Remite] = append(porRemite[m.Remite], m.ID)
		}
	}
	cli := w.cli
	go func() {
		for remite, ids := range porRemite {
			r, _ := types.ParseJID(remite)
			if !c.Grupo {
				r = types.EmptyJID
			}
			if err := cli.MarkRead(context.Background(), ids, time.Now(), chat, r); err != nil {
				log.Printf("leído: %v", err)
			}
		}
	}()
}

// enviar manda un texto y lo añade al chat enseguida (si falla, se marca).
func (w *wa) enviar(c *Chat, texto string) {
	texto = strings.TrimSpace(texto)
	if texto == "" || w.cli == nil {
		return
	}
	chat, err := types.ParseJID(c.JID)
	if err != nil {
		return
	}
	m := &Mensaje{ID: string(w.cli.GenerateMessageID()), Mio: true, Texto: w.app.f.limpiar(texto), Hora: time.Now()}
	c.añadir(m)
	w.sucio = true
	cli := w.cli
	go func() {
		_, err := cli.SendMessage(context.Background(), chat, &waE2E.Message{Conversation: proto.String(texto)},
			whatsmeow.SendRequestExtra{ID: types.MessageID(m.ID)})
		if err != nil {
			log.Printf("enviar: %v", err)
			w.app.hacer(func() { m.Error = true; w.app.aviso("No se ha podido enviar el mensaje") })
		}
	}()
}

// textoDe saca lo que se enseña de cada tipo de mensaje.
func textoDe(m *waE2E.Message) string {
	pie := func(s string) string {
		if s = strings.TrimSpace(s); s != "" {
			return ": " + s
		}
		return ""
	}
	switch {
	case m.GetConversation() != "":
		return m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		return "[Foto]" + pie(m.GetImageMessage().GetCaption())
	case m.GetVideoMessage() != nil:
		return "[Vídeo]" + pie(m.GetVideoMessage().GetCaption())
	case m.GetAudioMessage() != nil:
		if m.GetAudioMessage().GetPTT() {
			return "[Nota de voz]"
		}
		return "[Audio]"
	case m.GetDocumentMessage() != nil:
		return "[Documento]" + pie(m.GetDocumentMessage().GetFileName())
	case m.GetStickerMessage() != nil:
		return "[Sticker]"
	case m.GetLocationMessage() != nil, m.GetLiveLocationMessage() != nil:
		return "[Ubicación]"
	case m.GetContactMessage() != nil:
		return "[Contacto]" + pie(m.GetContactMessage().GetDisplayName())
	case m.GetPollCreationMessage() != nil:
		return "[Encuesta]" + pie(m.GetPollCreationMessage().GetName())
	case m.GetPollCreationMessageV3() != nil:
		return "[Encuesta]" + pie(m.GetPollCreationMessageV3().GetName())
	}
	return ""
}
