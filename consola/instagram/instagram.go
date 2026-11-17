package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Instagram como lo pide su web (www.instagram.com) con tu sesión: las mismas direcciones y cabeceras
// que manda el navegador. El inicio y los reels van por GraphQL (doc_id, los de la web de 2026) y, si
// eso falla, por las direcciones de siempre (api/v1); las historias, los mensajes y los «me gusta»,
// por api/v1. Los doc_id se pueden cambiar sin compilar (INSTAGRAM_DOC_INICIO…) por si Instagram los
// cambia.
const (
	agenteNavegador = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
	urlWeb          = "https://www.instagram.com"
)

var (
	appID         = entorno("INSTAGRAM_APP_ID", "936619743392459")
	docInicio     = entorno("INSTAGRAM_DOC_INICIO", "26307883352181852")
	docInicioMas  = entorno("INSTAGRAM_DOC_INICIO_MAS", "26038778959150000")
	docReels      = entorno("INSTAGRAM_DOC_REELS", "26136666099278270")
	errSesion     = errors.New("la sesión no vale")
	errMensajes   = errors.New("Instagram no deja ver los mensajes desde aquí")
	errSinCuenta  = errors.New("sin cuenta")
	clienteDatos  = &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	clienteImagen = &http.Client{Timeout: 25 * time.Second}
)

// Medio: una foto o un vídeo (una publicación de varias tiene uno por cada una).
type Medio struct {
	Foto     string
	W, H     int // los de la publicación (para saber si es vertical)
	Video    string
	Duracion float64
}

func (m Medio) esVideo() bool { return m.Video != "" }

// Post: una publicación, un reel o una historia.
type Post struct {
	PK          string // el número (para el «me gusta»)
	Usuario     string // @nombre
	Nombre      string
	FotoUsuario string
	Texto       string
	Likes       int
	Comentarios int
	MeGusta     bool
	Cuando      time.Time
	Medios      []Medio
	Sugerido    bool // no es de alguien que sigues
}

// Bandeja: alguien con historias.
type Bandeja struct {
	ID      string
	Usuario string
	Foto    string
	Vista   bool
	Items   []Post
}

// Chat: una conversación de los mensajes.
type Chat struct {
	ID       string
	Titulo   string
	Foto     string
	Ultimo   string
	Cuando   time.Time
	SinLeer  bool
	Mensajes []Mensaje // del más viejo al más nuevo (solo al abrirlo)
	Cursor   string
}

type Mensaje struct {
	Mio    bool
	Quien  string
	Texto  string
	Cuando time.Time
}

type instagram struct {
	cuenta *cuenta
	mu     sync.Mutex
	claim  string // x-ig-www-claim, lo da Instagram en las respuestas
	disp   string // un id de aparato para el inicio (como la web)
}

func nuevoInstagram(c *cuenta) *instagram {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return &instagram{cuenta: c, claim: "0", disp: strings.ToUpper(hex.EncodeToString(b))}
}

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// pedir: una petición a la web de Instagram con tu sesión. form != nil la hace POST.
func (ig *instagram) pedir(metodo, ruta string, consulta url.Values, form url.Values, extra map[string]string) (map[string]any, error) {
	c := ig.cuenta
	c.mu.Lock()
	cookie := c.Cookie
	c.mu.Unlock()
	if cookie == "" {
		return nil, errSinCuenta
	}
	dir := urlWeb + ruta
	if strings.HasPrefix(ruta, "https://") {
		dir = ruta
	}
	if len(consulta) > 0 {
		dir += "?" + consulta.Encode()
	}
	var cuerpo io.Reader
	if form != nil {
		cuerpo = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(metodo, dir, cuerpo)
	if err != nil {
		return nil, err
	}
	ig.mu.Lock()
	claim := ig.claim
	ig.mu.Unlock()
	h := req.Header
	h.Set("User-Agent", agenteNavegador)
	h.Set("Accept", "*/*")
	h.Set("Accept-Language", "es-ES,es;q=0.9")
	h.Set("Cookie", cookie)
	h.Set("Referer", urlWeb+"/")
	h.Set("X-IG-App-ID", appID)
	h.Set("X-ASBD-ID", "129477")
	h.Set("X-IG-WWW-Claim", claim)
	h.Set("X-Requested-With", "XMLHttpRequest")
	h.Set("Sec-Fetch-Dest", "empty")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Sec-Fetch-Site", "same-origin")
	if t := valorCookie(cookie, "csrftoken"); t != "" {
		h.Set("X-CSRFToken", t)
	}
	if form != nil {
		h.Set("Content-Type", "application/x-www-form-urlencoded")
		h.Set("Origin", urlWeb)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	resp, err := clienteDatos.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if v := resp.Header.Get("X-IG-Set-WWW-Claim"); v != "" {
		ig.mu.Lock()
		ig.claim = v
		ig.mu.Unlock()
	}
	c.actualizar(resp.Cookies())
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 24<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 || (resp.StatusCode >= 300 && resp.StatusCode < 400) {
		logf("%s %s: %d", metodo, ruta, resp.StatusCode)
		return nil, errSesion
	}
	var m map[string]any
	if err := json.Unmarshal(rb, &m); err != nil {
		logf("%s %s: %d, no es JSON (%d bytes)", metodo, ruta, resp.StatusCode, len(rb))
		return nil, fmt.Errorf("Instagram responde %d", resp.StatusCode)
	}
	if msg := txt(m, "message"); msg == "login_required" || msg == "checkpoint_required" || txt(m, "require_login") == "true" {
		logf("%s %s: %s", metodo, ruta, msg)
		return nil, errSesion
	}
	if resp.StatusCode != 200 {
		logf("%s %s: %d %s", metodo, ruta, resp.StatusCode, txt(m, "message"))
		if msg := txt(m, "message"); msg != "" {
			return m, fmt.Errorf("Instagram: %s", msg)
		}
		return m, fmt.Errorf("Instagram responde %d", resp.StatusCode)
	}
	return m, nil
}

// graphql: una consulta como las de la web (POST /graphql/query con doc_id).
func (ig *instagram) graphql(doc, nombre string, variables map[string]any) (map[string]any, error) {
	v, _ := json.Marshal(variables)
	form := url.Values{"doc_id": {doc}, "variables": {string(v)}, "fb_api_caller_class": {"RelayModern"},
		"fb_api_req_friendly_name": {nombre}, "server_timestamps": {"true"}}
	m, err := ig.pedir(http.MethodPost, "/graphql/query", nil, form, map[string]string{"X-FB-Friendly-Name": nombre})
	if err != nil {
		return nil, err
	}
	if errs := lista(m, "errors"); len(errs) > 0 && ir(m, "data") == nil {
		return nil, fmt.Errorf("GraphQL: %s", txt(errs[0], "message"))
	}
	return m, nil
}

// ---- Leer las publicaciones ----------------------------------------------------------------------

// fotoMejor: de las fotos que da Instagram, la más pequeña que llegue a «quiero» de ancho y tenga la
// forma de la publicación (también trae recortes cuadrados).
func fotoMejor(cands []any, w, h int, quiero int) string {
	type c struct {
		u    string
		w, h int
	}
	var cs []c
	for _, x := range cands {
		cw, ch := int(num(x, "width")), int(num(x, "height"))
		if u := txt(x, "url"); u != "" && cw > 0 && ch > 0 {
			cs = append(cs, c{u, cw, ch})
		}
	}
	if len(cs) == 0 {
		return ""
	}
	if w > 0 && h > 0 { // las que tienen la misma forma
		var igual []c
		for _, x := range cs {
			if d := float64(x.w*h) / float64(x.h*w); d > 0.97 && d < 1.03 {
				igual = append(igual, x)
			}
		}
		if len(igual) > 0 {
			cs = igual
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].w < cs[j].w })
	for _, x := range cs {
		if x.w >= quiero {
			return x.u
		}
	}
	return cs[len(cs)-1].u
}

// videoMejor: el vídeo más pequeño que tenga al menos 480 de ancho (la consola descodifica unas 100
// imágenes por segundo de ese tamaño; los de 720 también van, pero más justos).
func videoMejor(vs []any) string {
	type c struct {
		u string
		w int
	}
	var cs []c
	for _, x := range vs {
		if u := txt(x, "url"); u != "" {
			cs = append(cs, c{u, int(num(x, "width"))})
		}
	}
	if len(cs) == 0 {
		return ""
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].w < cs[j].w })
	for _, x := range cs {
		if x.w >= 480 {
			return x.u
		}
	}
	return cs[len(cs)-1].u
}

func leerMedio(m any) Medio {
	w, h := int(num(m, "original_width")), int(num(m, "original_height"))
	md := Medio{W: w, H: h}
	md.Foto = fotoMejor(lista(m, "image_versions2", "candidates"), w, h, 480)
	if vs := lista(m, "video_versions"); len(vs) > 0 {
		md.Video = videoMejor(vs)
		md.Duracion = num(m, "video_duration")
	}
	return md
}

// leerPost: una publicación del formato de api/v1 (el mismo en el inicio, los reels y las historias).
func leerPost(m any) (Post, bool) {
	if m == nil || txt(m, "product_type") == "ad" || ir(m, "injected") != nil {
		return Post{}, false
	}
	p := Post{
		PK:          txt(m, "pk"),
		Usuario:     txt(m, "user", "username"),
		Nombre:      txt(m, "user", "full_name"),
		FotoUsuario: txt(m, "user", "profile_pic_url"),
		Texto:       txt(m, "caption", "text"),
		Likes:       int(num(m, "like_count")),
		Comentarios: int(num(m, "comment_count")),
		MeGusta:     ir(m, "has_liked") == true,
	}
	if p.PK == "" {
		p.PK, _, _ = strings.Cut(txt(m, "id"), "_")
	}
	if p.Usuario == "" {
		p.Usuario = txt(m, "owner", "username")
		p.FotoUsuario = txt(m, "owner", "profile_pic_url")
	}
	if t := num(m, "taken_at"); t > 0 {
		p.Cuando = time.Unix(int64(t), 0)
	}
	if hijos := lista(m, "carousel_media"); len(hijos) > 0 {
		for _, c := range hijos {
			if md := leerMedio(c); md.Foto != "" || md.Video != "" {
				p.Medios = append(p.Medios, md)
			}
		}
	} else if md := leerMedio(m); md.Foto != "" || md.Video != "" {
		p.Medios = []Medio{md}
	}
	return p, len(p.Medios) > 0
}

// Pagina: lo que llega de una lista (y por dónde seguir).
type Pagina struct {
	Posts  []Post
	Cursor string
	Mas    bool
}

// deConexion lee una lista de GraphQL (edges[].node.media) con su page_info.
func deConexion(conn any) Pagina {
	var p Pagina
	for _, e := range lista(conn, "edges") {
		n := ir(e, "node")
		m, sugerido := ir(n, "media"), false
		if m == nil {
			m, sugerido = ir(n, "explore_story", "media"), true
		}
		if post, ok := leerPost(m); ok {
			post.Sugerido = sugerido
			p.Posts = append(p.Posts, post)
		}
	}
	p.Cursor = txt(conn, "page_info", "end_cursor")
	p.Mas = ir(conn, "page_info", "has_next_page") == true && p.Cursor != ""
	return p
}

// deItems lee una lista de api/v1 (items[], con la publicación dentro de media_or_ad o de media).
func deItems(m map[string]any) Pagina {
	var p Pagina
	for _, it := range lista(m, "items") {
		x := ir(it, "media_or_ad")
		if x == nil {
			x = ir(it, "media")
		}
		if x == nil && ir(it, "pk") != nil {
			x = it
		}
		if post, ok := leerPost(x); ok {
			p.Posts = append(p.Posts, post)
		}
	}
	for _, it := range lista(m, "feed_items") {
		if post, ok := leerPost(ir(it, "media_or_ad")); ok {
			p.Posts = append(p.Posts, post)
		}
	}
	p.Cursor = txt(m, "next_max_id")
	if p.Cursor == "" {
		p.Cursor = txt(m, "paging_info", "max_id")
	}
	p.Mas = (ir(m, "more_available") == true || ir(m, "paging_info", "more_available") == true) && p.Cursor != ""
	return p
}

// Inicio: tu inicio (lo de la gente que sigues y alguna sugerencia).
func (ig *instagram) Inicio(cursor string) (Pagina, error) {
	vars := map[string]any{
		"data": map[string]any{"device_id": ig.disp, "is_async_ads_double_request": "0", "is_async_ads_in_headload_enabled": "0",
			"is_async_ads_rti": "0", "rti_delivery_backend": "0"},
		"after": nil, "before": nil, "first": 12, "last": nil, "variant": "home",
	}
	doc, nombre := docInicio, "PolarisFeedTimelineRootV2Query"
	if cursor != "" {
		vars["after"] = cursor
		doc, nombre = docInicioMas, "PolarisFeedRootPaginationCachedQuery_subscribe"
	}
	m, err := ig.graphql(doc, nombre, vars)
	if err == nil {
		if conn := ir(m, "data", "xdt_api__v1__feed__timeline__connection"); conn != nil {
			if p := deConexion(conn); len(p.Posts) > 0 || !p.Mas {
				return p, nil
			}
		}
		err = errors.New("GraphQL sin inicio")
	}
	if errors.Is(err, errSesion) {
		return Pagina{}, err
	}
	logf("inicio por GraphQL: %v; pruebo api/v1", err)
	form := url.Values{"device_id": {ig.disp}, "is_async_ads_rti": {"0"}, "is_async_ads_double_request": {"0"},
		"rti_delivery_backend": {"0"}, "is_async_ads_in_headload_enabled": {"0"}}
	if cursor != "" {
		form.Set("max_id", cursor)
		form.Set("reason", "pagination")
	} else {
		form.Set("reason", "cold_start_fetch")
	}
	m2, err2 := ig.pedir(http.MethodPost, "/api/v1/feed/timeline/", nil, form, nil)
	if err2 != nil {
		return Pagina{}, err2
	}
	return deItems(m2), nil
}

// Reels: los reels que te recomienda Instagram (la pestaña de Reels de la web).
func (ig *instagram) Reels(cursor string) (Pagina, error) {
	var err error
	// Dos formas de pedirlo: la de la web y la que usan otros programas (por si una deja de valer).
	for _, datos := range []map[string]any{{"container_module": "clips_tab_desktop_page"}, {"count": 10}} {
		vars := map[string]any{"data": datos, "after": nil, "before": nil, "first": 10, "last": nil}
		if cursor != "" {
			vars["after"] = cursor
		}
		var m map[string]any
		m, err = ig.graphql(docReels, "PolarisClipsTabDesktopPaginationQuery", vars)
		if errors.Is(err, errSesion) {
			return Pagina{}, err
		}
		if err == nil {
			if conn := ir(m, "data", "xdt_api__v1__clips__home__connection_v2"); conn != nil {
				p := deConexion(conn)
				p.Posts = soloVideos(p.Posts)
				if len(p.Posts) > 0 {
					return p, nil
				}
			}
			err = errors.New("GraphQL sin reels")
		}
	}
	if errors.Is(err, errSesion) {
		return Pagina{}, err
	}
	logf("reels por GraphQL: %v; pruebo api/v1", err)
	form := url.Values{"container_module": {"clips_viewer_clips_tab"}, "page_size": {"10"}}
	if cursor != "" {
		form.Set("max_id", cursor)
	}
	m2, err2 := ig.pedir(http.MethodPost, "/api/v1/clips/discover/", nil, form, nil)
	if err2 != nil {
		return Pagina{}, err2
	}
	p := deItems(m2)
	p.Posts = soloVideos(p.Posts)
	return p, nil
}

func soloVideos(ps []Post) []Post {
	var r []Post
	for _, p := range ps {
		if len(p.Medios) > 0 && p.Medios[0].esVideo() {
			r = append(r, p)
		}
	}
	return r
}

// Historias: la fila de arriba del inicio (quién tiene historias y si ya las has visto).
func (ig *instagram) Historias() ([]Bandeja, error) {
	m, err := ig.pedir(http.MethodGet, "/api/v1/feed/reels_tray/", nil, nil, nil)
	if err != nil {
		return nil, err
	}
	var bs []Bandeja
	for _, t := range lista(m, "tray") {
		b := Bandeja{ID: txt(t, "id"), Usuario: txt(t, "user", "username"), Foto: txt(t, "user", "profile_pic_url")}
		if b.ID == "" || b.Usuario == "" {
			continue
		}
		b.Vista = num(t, "seen") >= num(t, "latest_reel_media") && num(t, "latest_reel_media") > 0
		for _, it := range lista(t, "items") {
			if p, ok := leerPost(it); ok {
				b.Items = append(b.Items, p)
			}
		}
		bs = append(bs, b)
	}
	return bs, nil
}

// HistoriasDe: las historias de una persona de la fila.
func (ig *instagram) HistoriasDe(id string) ([]Post, error) {
	m, err := ig.pedir(http.MethodGet, "/api/v1/feed/reels_media/", url.Values{"reel_ids": {id}}, nil, nil)
	if err != nil {
		return nil, err
	}
	reel := ir(m, "reels", id)
	if reel == nil {
		if l := lista(m, "reels_media"); len(l) > 0 {
			reel = l[0]
		}
	}
	var ps []Post
	for _, it := range lista(reel, "items") {
		if p, ok := leerPost(it); ok {
			if p.Usuario == "" {
				p.Usuario = txt(reel, "user", "username")
				p.FotoUsuario = txt(reel, "user", "profile_pic_url")
			}
			ps = append(ps, p)
		}
	}
	return ps, nil
}

// MeGusta pone o quita tu «me gusta».
func (ig *instagram) MeGusta(pk string, si bool) error {
	que := "unlike"
	if si {
		que = "like"
	}
	_, err := ig.pedir(http.MethodPost, "/api/v1/web/likes/"+url.PathEscape(pk)+"/"+que+"/", nil, url.Values{}, nil)
	return err
}

// ---- Mensajes ------------------------------------------------------------------------------------

const urlMovil = "https://i.instagram.com"

// pedirMensajes: los mensajes, por la web y, si ahí no están (en 2026 la web contesta 404 a
// direct_v2), por i.instagram.com. Lo que diga i.instagram.com de la sesión no cuenta: allí una
// sesión de la web puede no valer aunque en la web sí.
func (ig *instagram) pedirMensajes(metodo, ruta string, consulta, form url.Values) (map[string]any, error) {
	m, err := ig.pedir(metodo, ruta, consulta, form, nil)
	if err == nil || errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) {
		return m, err
	}
	logf("%s en la web: %v; pruebo %s", ruta, err, urlMovil)
	m2, err2 := ig.pedir(metodo, urlMovil+ruta, consulta, form, nil)
	if err2 == nil {
		return m2, nil
	}
	logf("%s en %s: %v", ruta, urlMovil, err2)
	if !hayRed() {
		return nil, err2
	}
	return nil, errMensajes
}

func microsegundos(v any) time.Time {
	n, _ := strconv.ParseInt(txt(v), 10, 64)
	switch {
	case n > 1e15:
		return time.UnixMicro(n)
	case n > 1e12:
		return time.UnixMilli(n)
	case n > 0:
		return time.Unix(n, 0)
	}
	return time.Time{}
}

// textoMensaje: lo que se enseña de un mensaje (los que no son texto, con lo que son).
func textoMensaje(it any, nombres map[string]string) string {
	switch txt(it, "item_type") {
	case "text":
		return txt(it, "text")
	case "like":
		return "❤"
	case "link":
		return txt(it, "link", "text")
	case "media", "raven_media", "visual_media":
		if ir(it, "media", "video_versions") != nil || ir(it, "visual_media", "media", "video_versions") != nil {
			return "[Vídeo]"
		}
		return "[Foto]"
	case "media_share":
		return "[Publicación de @" + txt(it, "media_share", "user", "username") + "]"
	case "clip":
		return "[Reel de @" + txt(it, "clip", "clip", "user", "username") + "]"
	case "voice_media":
		return "[Audio]"
	case "animated_media":
		return "[GIF]"
	case "reel_share":
		if t := txt(it, "reel_share", "text"); t != "" {
			return t
		}
		return "[Respuesta a una historia]"
	case "story_share":
		return "[Historia compartida]"
	case "profile":
		return "[Perfil de @" + txt(it, "profile", "username") + "]"
	case "action_log":
		return txt(it, "action_log", "description")
	case "placeholder":
		return "[" + txt(it, "placeholder", "title") + "]"
	}
	if t := txt(it, "text"); t != "" {
		return t
	}
	if strings.HasPrefix(txt(it, "item_type"), "xma") {
		return "[Publicación compartida]"
	}
	return "[Mensaje]"
}

func leerChat(t any, yo string) Chat {
	c := Chat{ID: txt(t, "thread_id"), Titulo: txt(t, "thread_title")}
	us := lista(t, "users")
	if c.Titulo == "" && len(us) > 0 {
		c.Titulo = txt(us[0], "username")
	}
	if len(us) > 0 {
		c.Foto = txt(us[0], "profile_pic_url")
	}
	nombres := map[string]string{}
	for _, u := range us {
		nombres[txt(u, "pk")] = txt(u, "username")
	}
	if its := lista(t, "items"); len(its) > 0 {
		c.Ultimo = textoMensaje(its[0], nombres)
		if txt(its[0], "user_id") == yo {
			c.Ultimo = "Tú: " + c.Ultimo
		}
		c.Cuando = microsegundos(ir(its[0], "timestamp"))
	}
	if c.Cuando.IsZero() {
		c.Cuando = microsegundos(ir(t, "last_activity_at"))
	}
	visto := microsegundos(ir(t, "last_seen_at", yo, "timestamp"))
	ultimoDe := ""
	if its := lista(t, "items"); len(its) > 0 {
		ultimoDe = txt(its[0], "user_id")
	}
	c.SinLeer = ultimoDe != "" && ultimoDe != yo && (num(t, "read_state") == 1 || (!visto.IsZero() && c.Cuando.After(visto)))
	return c
}

// Chats: tus conversaciones (las de la bandeja principal).
func (ig *instagram) Chats() ([]Chat, error) {
	m, err := ig.pedirMensajes(http.MethodGet, "/api/v1/direct_v2/inbox/", url.Values{"persistentBadging": {"true"}, "folder": {""},
		"limit": {"20"}, "thread_message_limit": {"1"}}, nil)
	if err != nil {
		return nil, err
	}
	yo := txt(m, "viewer", "pk")
	if yo == "" {
		yo = ig.cuenta.id()
	}
	var cs []Chat
	for _, t := range lista(m, "inbox", "threads") {
		if c := leerChat(t, yo); c.ID != "" {
			cs = append(cs, c)
		}
	}
	return cs, nil
}

// Conversacion: los últimos mensajes de un chat (cursor: los de antes).
func (ig *instagram) Conversacion(id, cursor string) ([]Mensaje, string, error) {
	q := url.Values{"limit": {"20"}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	m, err := ig.pedirMensajes(http.MethodGet, "/api/v1/direct_v2/threads/"+url.PathEscape(id)+"/", q, nil)
	if err != nil {
		return nil, "", err
	}
	t := ir(m, "thread")
	yo := txt(t, "viewer_id")
	if yo == "" {
		yo = ig.cuenta.id()
	}
	nombres := map[string]string{}
	for _, u := range lista(t, "users") {
		nombres[txt(u, "pk")] = txt(u, "username")
	}
	its := lista(t, "items")
	ms := make([]Mensaje, 0, len(its))
	for i := len(its) - 1; i >= 0; i-- { // llegan del más nuevo al más viejo
		it := its[i]
		de := txt(it, "user_id")
		ms = append(ms, Mensaje{Mio: de == yo, Quien: nombres[de], Texto: textoMensaje(it, nombres), Cuando: microsegundos(ir(it, "timestamp"))})
	}
	sig := ""
	if ir(t, "has_older") == true {
		sig = txt(t, "oldest_cursor")
	}
	return ms, sig, nil
}

// Enviar manda un mensaje de texto a un chat.
func (ig *instagram) Enviar(id, texto string) error {
	ctx := uuid()
	form := url.Values{"action": {"send_item"}, "client_context": {ctx}, "mutation_token": {ctx},
		"offline_threading_id": {ctx}, "text": {texto}, "thread_ids": {"[" + id + "]"}}
	m, err := ig.pedirMensajes(http.MethodPost, "/api/v1/direct_v2/threads/broadcast/text/", nil, form)
	if err != nil {
		return err
	}
	if s := txt(m, "status"); s != "" && s != "ok" {
		return fmt.Errorf("Instagram: %s", s)
	}
	return nil
}

// ---- Tu cuenta -----------------------------------------------------------------------------------

// QuienSoy pregunta a Instagram de quién es la sesión. Prueba varias direcciones (alguna puede no
// valer con la sesión de la web); si ninguna dice el nombre pero la de las historias contesta, la
// sesión vale igual (y el nombre sale vacío).
func (ig *instagram) QuienSoy() (usuario, nombre, foto string, err error) {
	id := ig.cuenta.id()
	type intento struct {
		ruta     string
		consulta url.Values
		camino   []any
	}
	intentos := []intento{
		{"/api/v1/accounts/current_user/", url.Values{"edit": {"true"}}, []any{"user"}},
		{"/api/v1/direct_v2/inbox/", url.Values{"limit": {"1"}, "thread_message_limit": {"1"}}, []any{"viewer"}},
	}
	if id != "" {
		intentos = append([]intento{{"/api/v1/users/" + id + "/info/", nil, []any{"user"}}}, intentos...)
	}
	for _, it := range intentos {
		m, e := ig.pedir(http.MethodGet, it.ruta, it.consulta, nil, nil)
		if e != nil {
			if errors.Is(e, errSesion) {
				return "", "", "", e
			}
			err = e
			continue
		}
		u := ir(m, it.camino...)
		if s := txt(u, "username"); s != "" {
			return s, txt(u, "full_name"), txt(u, "profile_pic_url"), nil
		}
	}
	if _, e := ig.pedir(http.MethodGet, "/api/v1/feed/reels_tray/", nil, nil, nil); e == nil {
		logf("la sesión vale, pero Instagram no ha dicho el nombre (%v)", err)
		return "", "", "", nil
	} else if errors.Is(e, errSesion) {
		return "", "", "", e
	}
	if err == nil {
		err = errors.New("Instagram no dice de quién es la sesión")
	}
	return "", "", "", err
}
