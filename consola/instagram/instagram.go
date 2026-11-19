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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Instagram como lo pide su web (www.instagram.com) con tu sesión: las mismas direcciones y cabeceras
// que manda el navegador. Casi todo va por GraphQL, como en la web de noviembre de 2026 (ver
// pagina.go: los tokens de la página y los doc_id de sus JS) y, si eso falla, por las direcciones de
// siempre (api/v1). La fila de historias va por api/v1, que sigue contestando. Un doc_id se puede
// cambiar sin compilar con INSTAGRAM_DOC_<nombre de la consulta>.
const (
	agenteNavegador = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
	urlWeb          = "https://www.instagram.com"
)

var (
	appID        = entorno("INSTAGRAM_APP_ID", "936619743392459")
	errSesion    = errors.New("la sesión no vale")
	errMensajes  = errors.New("Instagram no deja ver los mensajes desde aquí")
	errSinCuenta = errors.New("sin cuenta")
	// errReto: Instagram quiere que confirmes en su web que eres tú (también cuenta como errSesion).
	errReto = fmt.Errorf("%w: Instagram pide confirmar que eres tú", errSesion)
	// errLimite: Instagram no contesta bien (demasiadas peticiones o bloquea esa dirección), pero
	// no ha dicho que la sesión no valga.
	errLimite     = errors.New("Instagram no contesta ahora mismo")
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
	nReq   int    // __req de GraphQL
	dispDM string // el aparato de los mensajes, si la página no lo dice
	yoFB   string // tu número en los mensajes (interop_messaging_user_fbid)

	muWeb   sync.Mutex           // la página de Instagram se pide de una en una
	paginas map[string]*datosWeb // los tokens de la página (pagina.go), por ruta
	muDocs  sync.Mutex
	docs    *docsWeb             // los doc_id de sus JS
	buscado map[string]time.Time // cuándo se buscó en los JS cada consulta
	muBusca sync.Mutex           // se buscan de una en una
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

var reNumLargo = regexp.MustCompile(`\d{6,}`)

// rutaLog: una ruta para el registro, sin los números largos (de personas, publicaciones o chats).
func rutaLog(r string) string { return reNumLargo.ReplaceAllString(r, "N") }

// pedirCrudo: una petición a la web de Instagram con tu sesión, como la haría el navegador (pagina:
// como al abrir la página; si no, como sus peticiones de datos). form != nil la hace POST. Las
// redirecciones al login o a confirmar que eres tú, y los 429, ya salen como error.
func (ig *instagram) pedirCrudo(metodo, ruta string, consulta url.Values, form url.Values, extra map[string]string, pagina bool) (int, []byte, error) {
	c := ig.cuenta
	if c == nil {
		return 0, nil, errSinCuenta
	}
	c.mu.Lock()
	cookie := c.Cookie
	c.mu.Unlock()
	if cookie == "" {
		return 0, nil, errSinCuenta
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
		return 0, nil, err
	}
	ig.mu.Lock()
	claim := ig.claim
	ig.mu.Unlock()
	h := req.Header
	h.Set("User-Agent", agenteNavegador)
	h.Set("Accept-Language", "es-ES,es;q=0.9")
	h.Set("Cookie", cookie)
	if pagina {
		h.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
		h.Set("Sec-Fetch-Dest", "document")
		h.Set("Sec-Fetch-Mode", "navigate")
		h.Set("Sec-Fetch-Site", "none")
		h.Set("Sec-Fetch-User", "?1")
		h.Set("Upgrade-Insecure-Requests", "1")
	} else {
		h.Set("Accept", "*/*")
		h.Set("Referer", urlWeb+"/")
		h.Set("X-IG-App-ID", appID)
		h.Set("X-ASBD-ID", "359341")
		h.Set("X-IG-WWW-Claim", claim)
		h.Set("X-Requested-With", "XMLHttpRequest")
		h.Set("Sec-Fetch-Dest", "empty")
		h.Set("Sec-Fetch-Mode", "cors")
		h.Set("Sec-Fetch-Site", "same-origin")
		if t := valorCookie(cookie, "csrftoken"); t != "" {
			h.Set("X-CSRFToken", t)
		}
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
		return 0, nil, err
	}
	defer resp.Body.Close()
	if v := resp.Header.Get("X-IG-Set-WWW-Claim"); v != "" {
		ig.mu.Lock()
		ig.claim = v
		ig.mu.Unlock()
	}
	c.actualizar(resp.Cookies())
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 24<<20))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		// a dónde manda (sin lo de detrás de «?», que puede llevar datos): al login es que la sesión
		// no vale; a /challenge, que quiere que confirmes que eres tú; a otro sitio, no se sabe
		destino := ""
		if u, err := url.Parse(resp.Header.Get("Location")); err == nil {
			destino = rutaLog(u.Host + u.Path)
		}
		logf("%s %s: %d a %s", metodo, rutaLog(ruta), resp.StatusCode, destino)
		switch {
		case strings.Contains(destino, "/challenge") || strings.Contains(destino, "/checkpoint"):
			return 0, nil, errReto
		case destino == "" || strings.Contains(destino, "/accounts/login") || strings.HasSuffix(destino, "instagram.com/"):
			return 0, nil, errSesion
		}
		return 0, nil, fmt.Errorf("Instagram manda a otra página (%d)", resp.StatusCode)
	}
	if resp.StatusCode == 429 {
		logf("%s %s: 429 (demasiadas peticiones)", metodo, rutaLog(ruta))
		return 0, nil, errLimite
	}
	return resp.StatusCode, rb, nil
}

// pedir: una petición de datos (api/v1) a la web de Instagram con tu sesión. form != nil la hace POST.
func (ig *instagram) pedir(metodo, ruta string, consulta url.Values, form url.Values, extra map[string]string) (map[string]any, error) {
	st, rb, err := ig.pedirCrudo(metodo, rutaLog(ruta), consulta, form, extra, false)
	if err != nil {
		return nil, err
	}
	if st == 401 || st == 403 {
		logf("%s %s: %d", metodo, rutaLog(ruta), st)
		return nil, errSesion
	}
	var m map[string]any
	if err := json.Unmarshal(rb, &m); err != nil {
		logf("%s %s: %d, no es JSON (%d bytes)", metodo, rutaLog(ruta), st, len(rb))
		return nil, fmt.Errorf("Instagram responde %d", st)
	}
	if msg := txt(m, "message"); msg == "checkpoint_required" || msg == "challenge_required" {
		logf("%s %s: %s", metodo, rutaLog(ruta), msg)
		return nil, errReto
	} else if msg == "login_required" || txt(m, "require_login") == "true" {
		logf("%s %s: %s", metodo, rutaLog(ruta), msg)
		return nil, errSesion
	}
	if st != 200 {
		logf("%s %s: %d %s", metodo, rutaLog(ruta), st, txt(m, "message"))
		if msg := txt(m, "message"); msg != "" {
			return m, fmt.Errorf("Instagram: %s", msg)
		}
		return m, fmt.Errorf("Instagram responde %d", st)
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
	nombre := "PolarisFeedTimelineRootV2Query"
	if cursor != "" {
		vars["after"] = cursor
		nombre = "PolarisFeedRootPaginationCachedQuery_subscribe"
	}
	m, err := ig.consulta(nombre, "/", vars)
	if err == nil {
		if conn := ir(m, "data", "xdt_api__v1__feed__timeline__connection"); conn != nil {
			if p := deConexion(conn); len(p.Posts) > 0 || !p.Mas {
				return p, nil
			}
		}
		err = errors.New("GraphQL sin inicio")
	}
	if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) {
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
	vars := map[string]any{"data": map[string]any{"container_module": "clips_tab_desktop_page"}, "after": nil, "before": nil, "first": 10, "last": nil}
	if cursor != "" {
		vars["after"] = cursor
	}
	m, err := ig.consulta("PolarisClipsTabRootPaginationQuery", "/reels/", vars)
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
	if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) {
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

// historiasDeReel: las historias de un «reel» (lo de una persona), venga de donde venga.
func historiasDeReel(reel any) []Post {
	ps := []Post{}
	for _, it := range lista(reel, "items") {
		if p, ok := leerPost(it); ok {
			if p.Usuario == "" {
				p.Usuario = txt(reel, "user", "username")
				p.FotoUsuario = txt(reel, "user", "profile_pic_url")
			}
			ps = append(ps, p)
		}
	}
	return ps
}

// reelDe: de una lista de reels, el de esa persona (o el primero).
func reelDe(reels []any, id string) any {
	for _, r := range reels {
		if txt(r, "id") == id || txt(r, "user", "pk") == id || txt(r, "user", "id") == id {
			return r
		}
	}
	if len(reels) > 0 {
		return reels[0]
	}
	return nil
}

// HistoriasDe: las historias de una persona de la fila. Como la web de 2026: por GraphQL (la página de
// las historias de una persona o la galería) y, si no, por api/v1 (en noviembre de 2026 daba 500).
func (ig *instagram) HistoriasDe(id string) ([]Post, error) {
	var errs []string
	m, err := ig.consulta("PolarisStoriesV3ReelPageStandaloneQuery", "/", map[string]any{"media_id": nil, "reel_ids_arr": []string{id}, "is_highlight": false})
	if err == nil {
		if rm := ir(m, "data", "xdt_api__v1__feed__reels_media"); rm != nil {
			return historiasDeReel(reelDe(lista(rm, "reels_media"), id)), nil
		}
		err = errors.New("sin historias")
	}
	if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) {
		return nil, err
	}
	errs = append(errs, "GraphQL: "+err.Error())
	m, err = ig.consulta("PolarisStoriesV3ReelPageGalleryQuery", "/", map[string]any{"initial_reel_id": id, "reel_ids": []string{id}, "first": 1})
	if err == nil {
		if conn := ir(m, "data", "xdt_api__v1__feed__reels_media__connection"); conn != nil {
			var reels []any
			for _, e := range lista(conn, "edges") {
				if n := ir(e, "node"); n != nil {
					reels = append(reels, n)
				}
			}
			return historiasDeReel(reelDe(reels, id)), nil
		}
		err = errors.New("sin historias")
	}
	if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) {
		return nil, err
	}
	errs = append(errs, "galería: "+err.Error())
	logf("historias por GraphQL: %s; pruebo api/v1", strings.Join(errs, "; "))
	m2, err := ig.pedir(http.MethodGet, "/api/v1/feed/reels_media/", url.Values{"reel_ids": {id}}, nil, nil)
	if err != nil {
		return nil, err
	}
	reel := ir(m2, "reels", id)
	if reel == nil {
		reel = reelDe(lista(m2, "reels_media"), id)
	}
	return historiasDeReel(reel), nil
}

// MeGusta pone o quita tu «me gusta». Como la web de 2026 (la mutación de GraphQL de su botón) y, si
// no, como la de antes (en noviembre de 2026 daba 404).
func (ig *instagram) MeGusta(pk string, si bool) error {
	type forma struct{ nombre, campo string }
	formas := []forma{{"usePolarisLikeMediaXIGLikeMutation", "xig_media_like"}, {"PolarisAPILikePostMutation", "xig_media_like"}}
	if !si {
		formas = []forma{{"usePolarisLikeMediaXIGUnlikeMutation", "xig_media_unlike"}, {"PolarisAPIUnlikePostMutation", "xig_media_unlike"}}
	}
	var err error
	for k, f := range formas {
		input := map[string]any{"media_id": pk}
		if k == 0 && si {
			input["container_module"] = "feed_timeline"
		}
		var m map[string]any
		if m, err = ig.consulta(f.nombre, "/", map[string]any{"input": input}); err == nil {
			if ir(m, "data", f.campo) != nil {
				return nil
			}
			err = errors.New("GraphQL no lo ha hecho")
			if es := lista(m, "errors"); len(es) > 0 {
				logf("%s: %s", f.nombre, corto(txt(es[0], "message")))
			}
		}
		if errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) || errors.Is(err, errLimite) {
			return err
		}
		logf("me gusta por %s: %v", f.nombre, err)
	}
	que := "unlike"
	if si {
		que = "like"
	}
	_, err2 := ig.pedir(http.MethodPost, "/api/v1/web/likes/"+url.PathEscape(pk)+"/"+que+"/", nil, url.Values{}, nil)
	if err2 != nil && !errors.Is(err2, errSesion) && !errors.Is(err2, errLimite) {
		return err // el de GraphQL dice más
	}
	return err2
}

// ---- Mensajes ------------------------------------------------------------------------------------

const urlMovil = "https://i.instagram.com"

// pedirMensajes: los mensajes, por la web y, si ahí no están (en 2026 la web contesta 404 a
// direct_v2), por i.instagram.com. Lo que diga i.instagram.com de la sesión no cuenta: allí una
// sesión de la web puede no valer aunque en la web sí.
func (ig *instagram) pedirMensajes(metodo, ruta string, consulta, form url.Values) (map[string]any, error) {
	m, err := ig.pedir(metodo, rutaLog(ruta), consulta, form, nil)
	if err == nil || errors.Is(err, errSesion) || errors.Is(err, errSinCuenta) {
		return m, err
	}
	logf("%s en la web: %v; pruebo %s", rutaLog(ruta), err, urlMovil)
	m2, err2 := ig.pedir(metodo, urlMovil+ruta, consulta, form, nil)
	if err2 == nil {
		return m2, nil
	}
	logf("%s en %s: %v", rutaLog(ruta), urlMovil, err2)
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

// chatsV1: tus conversaciones por api/v1 (la bandeja principal; en noviembre de 2026, 404).
func (ig *instagram) chatsV1() ([]Chat, error) {
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

// conversacionV1: los últimos mensajes de un chat por api/v1 (cursor: los de antes).
func (ig *instagram) conversacionV1(id, cursor string) ([]Mensaje, string, error) {
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

// enviarV1 manda un mensaje de texto a un chat por api/v1.
func (ig *instagram) enviarV1(id, texto string) error {
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

// QuienSoy pregunta a Instagram de quién es la sesión. Prueba varias direcciones, porque con la
// sesión de la web unas contestan y otras no (en noviembre de 2026, users/<id>/info daba 429 y
// accounts/current_user mandaba a otra página). La sesión vale si contesta cualquiera, aunque no
// diga el nombre (entonces sale vacío), y solo no vale si Instagram lo dice (manda al login o pide
// confirmar que eres tú) y nada más contesta. Si solo hay límites o errores raros, devuelve
// errLimite: puede que valga.
func (ig *instagram) QuienSoy() (usuario, nombre, foto string, err error) {
	id := ig.cuenta.id()
	type intento struct {
		ruta     string
		consulta url.Values
		camino   []any
	}
	intentos := []intento{
		{"/api/v1/accounts/edit/web_form_data/", nil, []any{"form_data"}},
		{"/api/v1/accounts/current_user/", url.Values{"edit": {"true"}}, []any{"user"}},
	}
	if id != "" {
		intentos = append(intentos, intento{"/api/v1/users/" + id + "/info/", nil, []any{"user"}})
	}
	var errSes error
	limite := false // algún 429: con una sesión que no vale, Instagram manda al login, no pone límites
	anotar := func(e error) {
		switch {
		case errors.Is(e, errLimite):
			limite = true
			err = e
		case errors.Is(e, errReto):
			errSes = e
		case errors.Is(e, errSesion):
			if errSes == nil {
				errSes = e
			}
		default:
			err = e
		}
	}
	vale := false
	for _, it := range intentos {
		m, e := ig.pedir(http.MethodGet, it.ruta, it.consulta, nil, nil)
		if e != nil {
			anotar(e)
			continue
		}
		vale = true
		u := ir(m, it.camino...)
		if s := txt(u, "username"); s != "" {
			n := txt(u, "full_name")
			if n == "" {
				n = strings.TrimSpace(txt(u, "first_name"))
			}
			return s, n, txt(u, "profile_pic_url"), nil
		}
	}
	if !vale {
		if _, e := ig.pedir(http.MethodGet, "/api/v1/feed/reels_tray/", nil, nil, nil); e == nil {
			vale = true
		} else {
			anotar(e)
		}
	}
	if !vale {
		if _, e := ig.Inicio(""); e == nil {
			vale = true
		} else {
			anotar(e)
		}
	}
	if vale {
		logf("la sesión vale, pero Instagram no ha dicho el nombre")
		return "", "", "", nil
	}
	if errSes != nil && (errors.Is(errSes, errReto) || !limite) {
		return "", "", "", errSes
	}
	logf("no se ha podido comprobar la sesión: %v", err)
	return "", "", "", fmt.Errorf("%w (%v)", errLimite, err)
}
