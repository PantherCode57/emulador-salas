package ig

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Los mensajes como la web de noviembre de 2026, que ya no usa direct_v2: la bandeja
// (PolarisDirectInboxQuery), una conversación (IGDThreadDetailQuery y, para los de antes,
// IGDMessageListOffMsysQuery) y mandar un texto (IGDirectTextSendMutation). Son las consultas
// «OffMsys» de su web, por GraphQL y sin el chat en tiempo real (Lightspeed); las mismas que usa
// mautrix-meta. Si eso falla, por api/v1 (instagram.go). Las conversaciones que salen de aquí van por
// su thread_fbid (el número corto), no por el thread_id de api/v1.

const rutaMensajes = "/direct/inbox/"

// Chats: tus conversaciones (las de la bandeja principal).
func (ig *instagram) Chats() ([]Chat, error) {
	cs, err := ig.chatsGraphql()
	if err == nil {
		return cs, nil
	}
	if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) || errors.Is(err, errLimite) {
		return nil, err
	}
	logf("mensajes por GraphQL: %v; pruebo api/v1", err)
	return ig.chatsV1()
}

// Conversacion: los últimos mensajes de un chat (cursor: los de antes).
func (ig *instagram) Conversacion(id, cursor string) ([]Mensaje, string, error) {
	ms, sig, err := ig.conversacionGraphql(id, cursor)
	if err == nil {
		return ms, sig, nil
	}
	if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) || errors.Is(err, errLimite) {
		return nil, "", err
	}
	logf("conversación por GraphQL: %v; pruebo api/v1", err)
	return ig.conversacionV1(id, cursor)
}

// Enviar manda un mensaje de texto a un chat. Si GraphQL lo rechaza (no si no lo entiende), no se
// prueba api/v1: podría llegar dos veces.
func (ig *instagram) Enviar(id, texto string) error {
	err := ig.enviarGraphql(id, texto)
	if err == nil || !(errors.Is(err, errDoc) || errors.Is(err, errRecargar)) {
		return err
	}
	logf("enviar por GraphQL: %v; pruebo api/v1", err)
	if err2 := ig.enviarV1(id, texto); err2 != nil {
		if errors.Is(err2, errSesion) || errors.Is(err2, errLimite) {
			return err2
		}
		return err
	}
	return nil
}

// dispMensajes: el aparato de los mensajes que dice la página (o uno inventado, siempre el mismo).
func (ig *instagram) dispMensajes() (string, error) {
	d, err := ig.pagina(rutaMensajes, false)
	if err != nil {
		return "", err
	}
	if d.dispDirect != "" {
		return d.dispDirect, nil
	}
	ig.mu.Lock()
	defer ig.mu.Unlock()
	if ig.dispDM == "" {
		ig.dispDM = uuid()
	}
	return ig.dispDM, nil
}

func (ig *instagram) chatsGraphql() ([]Chat, error) {
	disp, err := ig.dispMensajes()
	if err != nil {
		return nil, err
	}
	vars := map[string]any{"device_id_for_iris_subscription": disp}
	m, err := ig.consulta("PolarisDirectInboxQuery", rutaMensajes, vars)
	if err != nil {
		return nil, err
	}
	mb := ir(m, "data", "get_slide_mailbox_for_iris_subscription")
	if mb == nil { // por la otra dirección
		if m2, err := ig.consultaEn(otroFin(finConsulta("PolarisDirectInboxQuery")), "PolarisDirectInboxQuery", rutaMensajes, vars); err == nil {
			if mb = ir(m2, "data", "get_slide_mailbox_for_iris_subscription"); mb != nil {
				m = m2
			}
		}
	}
	if mb == nil {
		if es := lista(m, "errors"); len(es) > 0 {
			logf("bandeja: %s", corto(txt(es[0], "message")))
		}
		logf("bandeja: %s", formaJSON(m, 4))
		return nil, errors.New("GraphQL sin la bandeja")
	}
	yo := ig.cuenta.id()
	cs := []Chat{}
	visto := map[string]bool{}
	poner := func(t any) {
		if c, ok := ig.leerHilo(t, yo); ok && !visto[c.ID] {
			visto[c.ID] = true
			cs = append(cs, c)
		}
	}
	for _, t := range lista(mb, "pinned_threads_v2") { // las fijadas, arriba, como en la web
		poner(ir(t, "as_ig_direct_thread"))
	}
	for _, e := range lista(mb, "threads_by_folder", "edges") {
		poner(ir(e, "node", "as_ig_direct_thread"))
	}
	// cuántos llevan lo que hace falta para saber qué es tuyo y qué está sin leer (solo números)
	recibos, marcados := 0, 0
	for _, e := range lista(mb, "threads_by_folder", "edges") {
		t := ir(e, "node", "as_ig_direct_thread")
		marcados += uno(ir(t, "marked_as_unread") == true)
		for _, r := range lista(t, "slide_read_receipts") {
			recibos += uno(txt(r, "participant_fbid") == ig.yoFBID() && ig.yoFBID() != "")
		}
	}
	logf("bandeja: %d conversaciones, tu número %v, con tu «leído» %d, marcadas sin leer %d", len(cs), ig.yoFBID() != "", recibos, marcados)
	return cs, nil
}

// leerHilo: una conversación de la bandeja de GraphQL.
func (ig *instagram) leerHilo(t any, yo string) (Chat, bool) {
	c := Chat{ID: txt(t, "thread_fbid")}
	if c.ID == "" {
		c.ID = txt(t, "thread_key")
	}
	if t == nil || c.ID == "" {
		return c, false
	}
	miFB := txt(t, "viewer", "interop_messaging_user_fbid")
	nombres := map[string]string{}
	var otros []any
	for _, u := range lista(t, "users") {
		if yo != "" && (txt(u, "id") == yo || txt(u, "pk") == yo) {
			if miFB == "" {
				miFB = txt(u, "interop_messaging_user_fbid")
			}
			continue
		}
		otros = append(otros, u)
		if fb := txt(u, "interop_messaging_user_fbid"); fb != "" {
			nombres[fb] = txt(u, "username")
		}
	}
	ig.apuntarYoFB(miFB)
	miFB = ig.yoFBID()
	c.Titulo = txt(t, "thread_title")
	if c.Titulo == "" {
		var ns []string
		for _, u := range otros {
			if n := txt(u, "username"); n != "" && len(ns) < 3 {
				ns = append(ns, n)
			}
		}
		c.Titulo = strings.Join(ns, ", ")
	}
	if c.Titulo == "" {
		c.Titulo = txt(t, "thread_title_fallback")
	}
	if len(otros) > 0 {
		c.Foto = txt(otros[0], "profile_pic_url")
	}
	if f := txt(t, "thread_image_url"); f != "" && (c.Foto == "" || ir(t, "is_group") == true) {
		c.Foto = f
	}
	ms := mensajesSlide(lista(t, "slide_messages", "edges"), yo, miFB, nombres)
	if len(ms) > 0 {
		u := ms[len(ms)-1]
		c.Ultimo, c.Cuando = u.Texto, u.Cuando
		if u.Mio {
			c.Ultimo = "Tú: " + c.Ultimo
		}
	}
	if w := microsegundos(ir(t, "last_activity_timestamp_ms")); !w.IsZero() {
		c.Cuando = w
	}
	c.SinLeer = ir(t, "marked_as_unread") == true
	if !c.SinLeer && len(ms) > 0 && !ms[len(ms)-1].Mio && miFB != "" {
		for _, r := range lista(t, "slide_read_receipts") {
			if txt(r, "participant_fbid") == miFB {
				w := microsegundos(ir(r, "watermark_timestamp_ms"))
				c.SinLeer = !w.IsZero() && ms[len(ms)-1].Cuando.After(w)
			}
		}
	}
	return c, true
}

// El número de tu cuenta en los mensajes (interop_messaging_user_fbid): con él se sabe qué es tuyo.
func (ig *instagram) apuntarYoFB(fb string) {
	if fb == "" || fb == "0" {
		return
	}
	ig.mu.Lock()
	if ig.yoFB == "" {
		ig.yoFB = fb
	}
	ig.mu.Unlock()
}

func (ig *instagram) yoFBID() string {
	ig.mu.Lock()
	defer ig.mu.Unlock()
	return ig.yoFB
}

// mensajesSlide: los mensajes de GraphQL, del más viejo al más nuevo.
func mensajesSlide(edges []any, yo, miFB string, nombres map[string]string) []Mensaje {
	ms := make([]Mensaje, 0, len(edges))
	for _, e := range edges {
		n := ir(e, "node")
		if n == nil {
			continue
		}
		de := txt(n, "sender_fbid")
		mio := (miFB != "" && de == miFB) || (yo != "" && (txt(n, "sender", "igid") == yo || txt(n, "sender_igid") == yo))
		quien := txt(n, "sender", "user_dict", "username")
		if quien == "" {
			quien = nombres[de]
		}
		if quien == "" {
			quien = txt(n, "sender", "name")
		}
		ms = append(ms, Mensaje{Mio: mio, Quien: quien, Texto: textoSlide(n), Cuando: microsegundos(ir(n, "timestamp_ms"))})
	}
	sort.SliceStable(ms, func(i, j int) bool { return ms[i].Cuando.Before(ms[j].Cuando) })
	return ms
}

// textoSlide: lo que se enseña de un mensaje de GraphQL (los que no son texto, con lo que son).
func textoSlide(n any) string {
	c := ir(n, "content")
	for _, t := range []string{txt(n, "text_body"), txt(c, "text_body"), txt(c, "xma_text_body")} {
		if t != "" {
			return t
		}
	}
	switch tipo := txt(c, "__typename"); tipo {
	case "SlideMessageImageContent", "SlideMessageRavenImageContent":
		return "[Foto]"
	case "SlideMessageVideosContent", "SlideMessageRavenVideoContent":
		return "[Vídeo]"
	case "SlideMessageMultiMediaContent":
		return "[Fotos]"
	case "SlideMessageAudiosContent":
		return "[Audio]"
	case "SlideMessageAnimatedMediaContent":
		return "[GIF]"
	case "SlideMessageAdminText":
		var partes []string
		for _, f := range lista(c, "text_fragments") {
			partes = append(partes, txt(f, "plaintext"))
		}
		if t := strings.TrimSpace(strings.Join(partes, "")); t != "" {
			return t
		}
	default:
		if strings.Contains(tipo, "Sticker") {
			return "[Sticker]"
		}
		if s := txt(n, "igd_snippet"); s != "" {
			return s
		}
		if strings.Contains(tipo, "XMA") {
			return "[Publicación compartida]"
		}
	}
	if s := txt(n, "igd_snippet"); s != "" {
		return s
	}
	return "[Mensaje]"
}

func (ig *instagram) conversacionGraphql(id, cursor string) ([]Mensaje, string, error) {
	var t any
	if cursor == "" {
		m, err := ig.consulta("IGDThreadDetailQuery", rutaMensajes, map[string]any{"min_uq_seq_id": nil, "thread_fbid": id})
		if err != nil {
			return nil, "", err
		}
		t = ir(m, "data", "get_slide_thread_nullable", "as_ig_direct_thread")
	} else {
		m, err := ig.consulta("IGDMessageListOffMsysQuery", rutaMensajes, map[string]any{"after": cursor, "before": nil, "first": 20,
			"last": nil, "newer_than_message_id": nil, "older_than_message_id": nil, "id": id})
		if err != nil {
			return nil, "", err
		}
		t = ir(m, "data", "fetch__SlideThread", "as_ig_direct_thread")
	}
	if t == nil {
		return nil, "", errors.New("GraphQL sin la conversación")
	}
	yo := ig.cuenta.id()
	nombres := map[string]string{}
	for _, u := range lista(t, "users") {
		if fb := txt(u, "interop_messaging_user_fbid"); fb != "" {
			nombres[fb] = txt(u, "username")
			if yo != "" && (txt(u, "id") == yo || txt(u, "pk") == yo) {
				ig.apuntarYoFB(fb)
			}
		}
	}
	ig.apuntarYoFB(txt(t, "viewer", "interop_messaging_user_fbid"))
	ms := mensajesSlide(lista(t, "slide_messages", "edges"), yo, ig.yoFBID(), nombres)
	sig := ""
	if ir(t, "slide_messages", "page_info", "has_next_page") == true {
		sig = txt(t, "slide_messages", "page_info", "end_cursor")
	}
	return ms, sig, nil
}

// idEnvio: como el offline_threading_id de la web (la hora en milisegundos y 22 bits más).
func idEnvio() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return strconv.FormatInt(time.Now().UnixMilli()<<22|int64(binary.BigEndian.Uint32(b[:])&0x3fffff), 10)
}

func (ig *instagram) enviarGraphql(id, texto string) error {
	vars := map[string]any{
		"ig_thread_igid": id, "offline_threading_id": idEnvio(), "recipient_igids": nil,
		"replied_to_client_context": nil, "replied_to_item_id": nil, "reply_to_message_id": nil, "sampled": nil,
		"text": map[string]any{"sensitive_string_value": texto}, "mentions": []any{}, "mentioned_user_ids": []any{},
		"commands": nil, "forwarded_from_thread_id": nil, "is_forwarded_from_own_message": nil,
		"send_attribution": "igd_web_chat_tab:in_thread",
	}
	m, err := ig.consulta("IGDirectTextSendMutation", rutaMensajes, vars)
	if err != nil {
		return err
	}
	if r := ir(m, "data", "xig_direct_text_send_with_slide_messaging_response"); r != nil {
		return nil
	}
	if es := lista(m, "errors"); len(es) > 0 {
		return fmt.Errorf("Instagram: %s", corto(txt(es[0], "message")))
	}
	return errors.New("Instagram no lo ha enviado")
}
