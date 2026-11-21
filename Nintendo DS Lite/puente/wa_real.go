package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// waReal: la sesión de WhatsApp de la dueña de la DS, en el puente (como el WhatsApp de la consola,
// con whatsmeow). Se vincula como un dispositivo más, sin QR: desde la DS se escribe el número y el
// puente pide a WhatsApp un código de 8 letras, que se mete en el móvil (Dispositivos vinculados →
// Vincular con el número de teléfono). Los chats se guardan en <datos>/chats.json.

const maxMensajesWA = 60

type msgR struct {
	ID     string    `json:"id"`
	De     string    `json:"de,omitempty"`
	Remite string    `json:"remite,omitempty"`
	Mio    bool      `json:"mio,omitempty"`
	Texto  string    `json:"texto"`
	Hora   time.Time `json:"hora"`
	Error  bool      `json:"error,omitempty"`
}

type chatR struct {
	JID      string    `json:"jid"`
	Nombre   string    `json:"nombre"`
	Grupo    bool      `json:"grupo,omitempty"`
	Ultimo   time.Time `json:"ultimo"`
	NoLeidos int       `json:"no_leidos,omitempty"`
	Mensajes []*msgR   `json:"mensajes"`
}

func (c *chatR) añadir(m *msgR) bool {
	for i := len(c.Mensajes) - 1; i >= 0; i-- {
		if c.Mensajes[i].ID == m.ID && m.ID != "" {
			c.Mensajes[i] = m
			return false
		}
	}
	n := len(c.Mensajes)
	c.Mensajes = append(c.Mensajes, m)
	if n > 0 && m.Hora.Before(c.Mensajes[n-1].Hora) {
		sort.SliceStable(c.Mensajes, func(i, j int) bool { return c.Mensajes[i].Hora.Before(c.Mensajes[j].Hora) })
	}
	if len(c.Mensajes) > maxMensajesWA {
		c.Mensajes = c.Mensajes[len(c.Mensajes)-maxMensajesWA:]
	}
	if m.Hora.After(c.Ultimo) {
		c.Ultimo = m.Hora
	}
	return true
}

func (c *chatR) previa() string {
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

type waReal struct {
	mu        sync.Mutex
	dir       string
	cont      *sqlstore.Container
	cli       *whatsmeow.Client
	estado    string
	vinculo   *vinculacion
	chats     map[string]*chatR
	nombres   map[string]string
	lidPN     map[string]string
	pedidoGrp map[string]bool
	ver       int
	sucio     bool
}

// vinculacion: la conexión abierta para pedir el código (dura unos 160 segundos).
type vinculacion struct {
	listo chan struct{} // se cierra con el primer QR (ya se puede pedir el código)
}

func nuevoWAReal(dir string) (*waReal, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	store.SetOSInfo("Panther DS", [3]uint32{1, 0, 0})
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "sesion.db")+
		"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	c := sqlstore.NewWithDB(db, "sqlite3", waLog.Stdout("BD", "WARN", false))
	if err := c.Upgrade(context.Background()); err != nil {
		return nil, fmt.Errorf("base de datos: %w", err)
	}
	w := &waReal{dir: dir, cont: c, chats: map[string]*chatR{}, nombres: map[string]string{},
		lidPN: map[string]string{}, pedidoGrp: map[string]bool{}, ver: 1}
	w.cargar()
	w.mu.Lock()
	w.nuevoCliente()
	w.mu.Unlock()
	go w.guardarDeVezEnCuando()
	return w, nil
}

// nuevoCliente: con la sesión guardada se conecta; si no hay, espera a que se vincule desde la DS.
func (w *waReal) nuevoCliente() {
	ctx := context.Background()
	disp, err := w.cont.GetFirstDevice(ctx)
	if err != nil || disp == nil {
		disp = w.cont.NewDevice()
	}
	cli := whatsmeow.NewClient(disp, waLog.Stdout("WA", "WARN", false))
	cli.AddEventHandler(func(evt any) { w.evento(cli, evt) })
	w.cli = cli
	if cli.Store.ID == nil {
		w.estado = "sin-vincular"
		return
	}
	w.estado = "conectando"
	go func() {
		for {
			if err := cli.Connect(); err == nil {
				return
			} else {
				log.Printf("WhatsApp: conectar: %v", err)
			}
			w.mu.Lock()
			if w.cli != cli {
				w.mu.Unlock()
				return
			}
			w.estado = "sin-red"
			w.mu.Unlock()
			time.Sleep(15 * time.Second)
			if cli.IsConnected() {
				return
			}
		}
	}()
}

// vincular: el código para meter en el WhatsApp del móvil.
func (w *waReal) vincular(telefono string) (string, error) {
	w.mu.Lock()
	cli := w.cli
	if cli.Store.ID != nil {
		w.mu.Unlock()
		return "", errors.New("WhatsApp ya está vinculado")
	}
	v := &vinculacion{listo: make(chan struct{})}
	w.vinculo = v
	w.estado = "vinculando"
	w.mu.Unlock()
	if cli.IsConnected() {
		cli.Disconnect() // una vinculación de antes: se empieza otra (sin el cerrojo: avisa de eventos)
	}

	qr, err := cli.GetQRChannel(context.Background())
	if err != nil {
		return "", fmt.Errorf("no se puede empezar a vincular: %v", err)
	}
	go func() {
		primero := true
		for it := range qr {
			switch it.Event {
			case "code":
				if primero {
					primero = false
					close(v.listo)
				}
			case "success":
				log.Printf("WhatsApp: vinculado")
			default:
				log.Printf("WhatsApp: vincular: %s %v", it.Event, it.Error)
				w.mu.Lock()
				if w.vinculo == v && w.cli.Store.ID == nil {
					w.estado = "sin-vincular"
				}
				w.mu.Unlock()
			}
		}
	}()
	if err := cli.Connect(); err != nil {
		return "", fmt.Errorf("no se puede conectar con WhatsApp: %v", err)
	}
	select {
	case <-v.listo:
	case <-time.After(20 * time.Second):
		return "", errors.New("WhatsApp no contesta")
	}
	codigo, err := cli.PairPhone(context.Background(), telefono, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		if errors.Is(err, whatsmeow.ErrPhoneNumberIsNotInternational) || errors.Is(err, whatsmeow.ErrPhoneNumberTooShort) {
			return "", errors.New("el número tiene que llevar el prefijo del país (34 en España) y sin 0 delante")
		}
		return "", fmt.Errorf("WhatsApp no da el código: %v", err)
	}
	return codigo, nil
}

func (w *waReal) evento(cli *whatsmeow.Client, evt any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cli != cli {
		return
	}
	switch e := evt.(type) {
	case *events.PairSuccess:
		w.estado = "conectando"
	case *events.Connected:
		w.estado = "conectado"
		w.renombrar()
	case *events.AppStateSyncComplete:
		w.renombrar()
	case *events.Disconnected:
		if w.estado == "conectado" {
			w.estado = "sin-red"
		}
	case *events.LoggedOut:
		log.Printf("WhatsApp: se ha desvinculado")
		w.chats = map[string]*chatR{}
		w.marcar()
		go func() {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.nuevoCliente()
		}()
	case *events.PushName:
		if e.NewPushName != "" {
			w.nombres[e.JID.ToNonAD().String()] = e.NewPushName
		}
	case *events.HistorySync:
		for _, p := range e.Data.GetPushnames() {
			if p.GetPushname() != "" {
				w.nombres[p.GetID()] = p.GetPushname()
			}
		}
		w.historial(e.Data.GetConversations())
	case *events.Message:
		w.mensaje(e, true)
	}
}

func (w *waReal) marcar() {
	w.sucio = true
	w.ver++
}

func (w *waReal) chat(jid types.JID) *chatR {
	k := jid.ToNonAD().String()
	c := w.chats[k]
	if c == nil {
		c = &chatR{JID: k, Grupo: jid.Server == types.GroupServer}
		w.chats[k] = c
	}
	return c
}

func ignorarJID(jid types.JID) bool {
	return jid == types.StatusBroadcastJID || jid.Server == types.NewsletterServer || jid.Server == types.BroadcastServer
}

func (w *waReal) mensaje(e *events.Message, enVivo bool) {
	if ignorarJID(e.Info.Chat) || e.Message == nil {
		return
	}
	texto := textoDe(e.Message)
	if texto == "" {
		return
	}
	if !e.Info.SenderAlt.IsEmpty() {
		w.aprender(e.Info.Sender, e.Info.SenderAlt)
	}
	if e.Info.PushName != "" && !e.Info.IsFromMe {
		w.nombres[e.Info.Sender.ToNonAD().String()] = e.Info.PushName
	}
	c := w.chat(e.Info.Chat)
	m := &msgR{ID: e.Info.ID, Remite: e.Info.Sender.ToNonAD().String(), Mio: e.Info.IsFromMe, Texto: texto, Hora: e.Info.Timestamp}
	if !m.Mio {
		m.De = w.nombreDe(e.Info.Sender, e.Info.PushName)
	}
	if e.IsEdit {
		m.Texto += " (editado)"
	}
	nuevo := c.añadir(m)
	w.marcar()
	if c.Nombre == "" {
		w.ponerNombre(c, e.Info.Chat, e.Info.PushName, e.Info.IsFromMe)
	}
	if enVivo && nuevo && !m.Mio {
		c.NoLeidos++
	}
}

func (w *waReal) historial(convs []*waHistorySync.Conversation) {
	for _, conv := range convs {
		jid, err := types.ParseJID(conv.GetID())
		if err != nil || ignorarJID(jid) {
			continue
		}
		c := w.chat(jid)
		if pn := conv.GetPnJID(); pn != "" {
			if p, err := types.ParseJID(pn); err == nil {
				w.aprender(jid, p)
			}
		}
		if n := conv.GetName(); n != "" {
			c.Nombre = n
		} else if n := conv.GetDisplayName(); n != "" && c.Nombre == "" {
			c.Nombre = n
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
			if ev, err := w.cli.ParseWebMessage(jid, hm.GetMessage()); err == nil {
				w.mensaje(ev, false)
			}
		}
		if c.Nombre == "" {
			w.ponerNombre(c, jid, "", false)
		}
	}
	w.marcar()
	w.renombrar()
}

func (w *waReal) ponerNombre(c *chatR, jid types.JID, pushName string, deMi bool) {
	if c.Grupo {
		if w.pedidoGrp[c.JID] {
			return
		}
		w.pedidoGrp[c.JID] = true
		cli := w.cli
		go func() {
			info, err := cli.GetGroupInfo(context.Background(), jid)
			if err != nil || info == nil {
				return
			}
			w.mu.Lock()
			c.Nombre = info.Name
			w.marcar()
			w.mu.Unlock()
		}()
		return
	}
	if deMi {
		pushName = ""
	}
	c.Nombre = w.nombreDe(jid, pushName)
}

func (w *waReal) aprender(a, b types.JID) {
	a, b = a.ToNonAD(), b.ToNonAD()
	if a.Server == types.DefaultUserServer && b.Server == types.HiddenUserServer {
		a, b = b, a
	}
	if a.Server == types.HiddenUserServer && b.Server == types.DefaultUserServer {
		w.lidPN[a.String()] = b.String()
	}
}

func (w *waReal) candidatos(jid types.JID) []types.JID {
	jid = jid.ToNonAD()
	r := []types.JID{jid}
	if jid.Server != types.HiddenUserServer {
		return r
	}
	if w.cli.Store.LIDs != nil {
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

func (w *waReal) nombreDe(jid types.JID, pushName string) string {
	cs := w.candidatos(jid)
	if w.cli.Store.Contacts != nil {
		for _, c := range cs {
			if ci, err := w.cli.Store.Contacts.GetContact(context.Background(), c); err == nil && ci.Found {
				for _, n := range []string{ci.FullName, ci.FirstName, ci.PushName, ci.BusinessName} {
					if n != "" {
						return n
					}
				}
			}
		}
	}
	if pushName != "" {
		return pushName
	}
	for _, c := range cs {
		if n := w.nombres[c.String()]; n != "" {
			return n
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

func (w *waReal) renombrar() {
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
				c.Nombre = n
				w.marcar()
			}
		}
		if c.Grupo && c.Nombre == "" {
			w.ponerNombre(c, jid, "", false)
		}
	}
}

func (w *waReal) visibles() []*chatR {
	var r []*chatR
	for _, c := range w.chats {
		if len(c.Mensajes) > 0 || c.NoLeidos > 0 {
			r = append(r, c)
		}
	}
	sort.Slice(r, func(i, j int) bool { return r[i].Ultimo.After(r[j].Ultimo) })
	return r
}

func horaDS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.Local()
	ahora := time.Now()
	if t.YearDay() == ahora.YearDay() && t.Year() == ahora.Year() {
		return t.Format("15:04")
	}
	return t.Format("02/01")
}

func (w *waReal) pedir(orden string) (waRespuesta, error) {
	cmd, arg, _ := strings.Cut(strings.TrimSpace(orden), " ")
	if cmd == "vincular" { // fuera del cerrojo: tarda unos segundos
		codigo, err := w.vincular(arg)
		if err != nil {
			return waRespuesta{Error: err.Error()}, nil
		}
		return waRespuesta{Estado: "vinculando", Codigo: codigo}, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	r := waRespuesta{Estado: w.estado, Ver: w.ver}
	switch cmd {
	case "estado":
	case "chats":
		for _, c := range w.visibles() {
			if len(r.Chats) == 40 {
				break
			}
			r.Chats = append(r.Chats, waChat{JID: c.JID, Nombre: c.Nombre, Grupo: c.Grupo, NoLeidos: c.NoLeidos,
				Hora: horaDS(c.Ultimo), Previa: c.previa()})
		}
	case "chat":
		c := w.chats[arg]
		if c == nil {
			r.Error = "no está ese chat"
			return r, nil
		}
		if c.NoLeidos > 0 {
			n := min(c.NoLeidos, len(c.Mensajes))
			w.leido(c, c.Mensajes[len(c.Mensajes)-n:])
			c.NoLeidos = 0
			w.marcar()
		}
		r.Nombre = c.Nombre
		ms := c.Mensajes
		if len(ms) > 40 {
			ms = ms[len(ms)-40:]
		}
		for _, m := range ms {
			r.Mensajes = append(r.Mensajes, waMensaje{Mio: m.Mio, De: m.De, Texto: m.Texto, Hora: horaDS(m.Hora), Error: m.Error})
		}
		r.Ver = w.ver
	case "enviar":
		jid, texto, _ := strings.Cut(arg, "\t")
		c := w.chats[jid]
		if c == nil {
			r.Error = "no está ese chat"
			return r, nil
		}
		if w.estado != "conectado" {
			r.Error = "WhatsApp no está conectado"
			return r, nil
		}
		w.enviar(c, texto)
		r.Ver = w.ver
	default:
		r.Error = "no sé hacer «" + cmd + "»"
	}
	return r, nil
}

// leido: los dos ticks azules (en el móvil también quedan leídos).
func (w *waReal) leido(c *chatR, ms []*msgR) {
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
				log.Printf("WhatsApp: leído: %v", err)
			}
		}
	}()
}

func (w *waReal) enviar(c *chatR, texto string) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return
	}
	chat, err := types.ParseJID(c.JID)
	if err != nil {
		return
	}
	cli := w.cli
	m := &msgR{ID: string(cli.GenerateMessageID()), Mio: true, Texto: texto, Hora: time.Now()}
	c.añadir(m)
	w.marcar()
	go func() {
		_, err := cli.SendMessage(context.Background(), chat, &waE2E.Message{Conversation: proto.String(texto)},
			whatsmeow.SendRequestExtra{ID: types.MessageID(m.ID)})
		if err != nil {
			log.Printf("WhatsApp: enviar: %v", err)
			w.mu.Lock()
			m.Error = true
			w.marcar()
			w.mu.Unlock()
		}
	}()
}

func (w *waReal) archivo() string { return filepath.Join(w.dir, "chats.json") }

func (w *waReal) cargar() {
	b, err := os.ReadFile(w.archivo())
	if err != nil {
		return
	}
	var lista []*chatR
	if json.Unmarshal(b, &lista) == nil {
		for _, c := range lista {
			w.chats[c.JID] = c
		}
	}
}

func (w *waReal) guardarDeVezEnCuando() {
	for range time.Tick(15 * time.Second) {
		w.mu.Lock()
		if !w.sucio {
			w.mu.Unlock()
			continue
		}
		w.sucio = false
		b, err := json.Marshal(w.visibles())
		w.mu.Unlock()
		if err == nil {
			tmp := w.archivo() + ".tmp"
			if os.WriteFile(tmp, b, 0o600) == nil {
				_ = os.Rename(tmp, w.archivo())
			}
		}
	}
}

// textoDe: lo que se enseña de cada tipo de mensaje (como en el WhatsApp de la consola).
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
	}
	return ""
}
