package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// La web de Instagram de 2026 pide casi todo por GraphQL (POST /graphql/query) con datos de su propia
// página: los tokens de tu sesión (lsd y fb_dtsg), unos números de la versión de la web
// (haste_session, hsi, __spin_r…) y, para cada consulta, un doc_id que está en sus JS y que cambia cada
// pocos días. Aquí se abre la página con tu sesión y se sacan esos datos. Los doc_id van incluidos (los
// de la web del 19/11/2026); si Instagram ya no los acepta, se buscan en sus JS, como hace gallery-dl,
// y se guardan en docs.json (son los mismos para todo el mundo: no hay nada tuyo). Los tokens no se
// guardan ni se escriben en el registro.

const urlJS = "https://static.cdninstagram.com/rsrc.php/"

// docsIncluidos: los doc_id de la web del 19/11/2026 (sacados de sus JS).
var docsIncluidos = map[string]string{
	"PolarisFeedTimelineRootV2Query":                 "29233130896381434",
	"PolarisFeedRootPaginationCachedQuery_subscribe": "29629781476659432",
	"PolarisClipsTabRootPaginationQuery":             "40684727481141196",
	"PolarisStoriesV3ReelPageStandaloneQuery":        "28739385832429456",
	"PolarisStoriesV3ReelPageGalleryQuery":           "29644407348582972",
	"usePolarisLikeMediaXIGLikeMutation":             "27182485238052618",
	"usePolarisLikeMediaXIGUnlikeMutation":           "27345296031770102",
	"PolarisAPILikePostMutation":                     "28051398997871885",
	"PolarisAPIUnlikePostMutation":                   "29100661379564082",
	"PolarisDirectInboxQuery":                        "28881837051457834",
	"IGDThreadDetailQuery":                           "29486096901027304",
	"IGDMessageListOffMsysQuery":                     "29008944932098151",
	"IGDirectTextSendMutation":                       "28737274835943013",
}

// provIncluidos: las variables «__relay_internal__pv__…» que lleva cada consulta (las pone el
// navegador según la versión de la web; sin ellas, Instagram no contesta).
var provIncluidos = map[string][]string{
	"PolarisFeedTimelineRootV2Query": {"PolarisShortDramaEnabled", "PolarisMultiCaptionCarouselEnabled", "PolarisThreadsStyleCommentsEnabled",
		"PolarisReelsRecoDebugOverlayEnabled", "PolarisAdDebugToolEnabled", "PolarisSuggestedUserFeedUnitCardsEnabled"},
	"PolarisFeedRootPaginationCachedQuery_subscribe": {"PolarisShortDramaEnabled", "PolarisMultiCaptionCarouselEnabled", "PolarisThreadsStyleCommentsEnabled",
		"PolarisReelsRecoDebugOverlayEnabled", "PolarisAdDebugToolEnabled", "PolarisSuggestedUserFeedUnitCardsEnabled"},
	"PolarisClipsTabRootPaginationQuery":      {"PolarisReelsRecoDebugOverlayEnabled", "PolarisShortDramaEnabled", "PolarisThreadsStyleCommentsEnabled"},
	"PolarisStoriesV3ReelPageStandaloneQuery": {"PolarisCommunityNoteStoriesLabelEnabled"},
	"PolarisStoriesV3ReelPageGalleryQuery":    {"PolarisCommunityNoteStoriesLabelEnabled"},
	"PolarisDirectInboxQuery":                 {"IGDIsProfessionalAccountGK", "IGDMaxUnreadMessagesCount", "IGDPinnedThreadsRenderEnabledGK", "IGDThreadListActionsEnabledGK"},
	"IGDThreadDetailQuery":                    {"IGDEnableOffMsysChatThemesQE", "IGDInitialMessagePageCount", "IGDSafetyNoticesWebEnabled"},
	"IGDMessageListOffMsysQuery":              {"IGDInitialMessagePageCount"},
}

// valoresProv: lo que vale cada una (las que no están aquí ni en los JS, false: son interruptores de
// cosas nuevas de la web).
var valoresProv = map[string]any{
	"IGDMaxUnreadMessagesCount":       5,
	"IGDInitialMessagePageCount":      20,
	"IGDPinnedThreadsRenderEnabledGK": true,
	"IGDThreadListActionsEnabledGK":   true,
}

var (
	// errDoc: Instagram no reconoce la consulta (doc_id viejo, faltan variables o contesta otra cosa).
	errDoc = errors.New("Instagram no reconoce la consulta")
	// errRecargar: Instagram pide abrir otra vez la página (los tokens han caducado).
	errRecargar = errors.New("Instagram pide recargar la página")
)

// datosWeb: lo que hace falta de la página de Instagram para pedir por GraphQL como su web.
type datosWeb struct {
	lsd, dtsg, jazoest  string
	hs, hsi, rev        string
	spinR, spinB, spinT string
	usuario, av         string   // USER_ID y ACCOUNT_ID de la página (con tu sesión pueden ser «0»)
	dtsgSesion          bool     // el fb_dtsg es el de una sesión (DTSGInitialData), no el de invitado
	dispDirect          string   // el aparato de los mensajes (IGDMqttWebDeviceID)
	scripts             []string // sus JS (primero los que la página carga ya)
	cuando              time.Time
}

// docsWeb: los doc_id encontrados en los JS de la web (en docs.json).
type docsWeb struct {
	Cuando time.Time           `json:"cuando"`
	Docs   map[string]string   `json:"docs"`
	Prov   map[string][]string `json:"prov"`
	Valor  map[string]any      `json:"valor"`
}

func rutaDocs() string { return filepath.Join(carpetaDatos, "docs.json") }

// cadenaTras lee la cadena JSON que va justo después de «antes» (sin las comillas).
func cadenaTras(s, antes string) string {
	i := strings.Index(s, antes)
	if i < 0 {
		return ""
	}
	s = s[i+len(antes):]
	for j := 0; j < len(s) && j < 8192; j++ {
		switch s[j] {
		case '\\':
			j++
		case '"':
			var v string
			if json.Unmarshal([]byte(`"`+s[:j]+`"`), &v) != nil {
				return ""
			}
			return v
		}
	}
	return ""
}

var (
	reSpinR   = regexp.MustCompile(`"__spin_r":(\d+)`)
	reSpinT   = regexp.MustCompile(`"__spin_t":(\d+)`)
	reRev     = regexp.MustCompile(`"server_revision":(\d+)`)
	reJSAttr  = regexp.MustCompile(`(?:href|src)="(https://static\.cdninstagram\.com/rsrc\.php/[^"]+?\.js[^"]*)"`)
	reJSTexto = regexp.MustCompile(`https://static\.cdninstagram\.com/rsrc\.php/[^"'\s<>\\]+?\.js`)
)

func primero(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// leerDatosWeb saca de la página lo que hace falta (como gallery-dl y mautrix-meta en 2026).
func leerDatosWeb(pag string) *datosWeb {
	d := &datosWeb{cuando: time.Now()}
	var eqmc map[string]any
	if i := strings.Index(pag, ` id="__eqmc"`); i >= 0 {
		if a := strings.Index(pag[i:], ">"); a >= 0 {
			resto := pag[i+a+1:]
			if b := strings.Index(resto, "</script>"); b >= 0 {
				_ = json.Unmarshal([]byte(resto[:b]), &eqmc)
			}
		}
	}
	d.lsd = cadenaTras(pag, `"LSD",[],{"token":"`)
	if d.lsd == "" {
		d.lsd = cadenaTras(pag, `"lsd":"`)
	}
	if d.lsd == "" {
		d.lsd = txt(eqmc, "l")
	}
	d.dtsg = cadenaTras(pag, `"DTSGInitialData",[],{"token":"`)
	d.dtsgSesion = d.dtsg != ""
	for _, antes := range []string{`"DTSGInitData",[],{"token":"`, `"dtsg":{"token":"`} {
		if d.dtsg == "" {
			d.dtsg = cadenaTras(pag, antes)
		}
	}
	if d.dtsg == "" {
		d.dtsg = txt(eqmc, "f")
	}
	if u, err := url.Parse(txt(eqmc, "u")); err == nil {
		d.jazoest = u.Query().Get("jazoest")
	}
	if d.jazoest == "" && d.dtsg != "" { // como la web: «2» y la suma de las letras de fb_dtsg
		n := 0
		for _, r := range d.dtsg {
			n += int(r)
		}
		d.jazoest = "2" + strconv.Itoa(n)
	}
	d.hs = cadenaTras(pag, `"haste_session":"`)
	d.hsi = cadenaTras(pag, `"hsi":"`)
	d.spinR = primero(reSpinR, pag)
	d.spinB = cadenaTras(pag, `"__spin_b":"`)
	d.spinT = primero(reSpinT, pag)
	if d.rev = primero(reRev, pag); d.rev == "" {
		d.rev = d.spinR
	}
	d.usuario = cadenaTras(pag, `"USER_ID":"`)
	d.av = cadenaTras(pag, `"ACCOUNT_ID":"`)
	if i := strings.Index(pag, `"IGDMqttWebDeviceID"`); i >= 0 {
		d.dispDirect = cadenaTras(pag[i:min(len(pag), i+400)], `"clientId":"`)
	}
	// los JS: primero los que la página carga (href/src) y después los que tiene apuntados
	visto := map[string]bool{}
	for _, m := range reJSAttr.FindAllStringSubmatch(pag, -1) {
		if u := html.UnescapeString(m[1]); !visto[u] {
			visto[u] = true
			d.scripts = append(d.scripts, u)
		}
	}
	for _, u := range reJSTexto.FindAllString(strings.ReplaceAll(pag, `\/`, `/`), -1) {
		if !visto[u] {
			visto[u] = true
			d.scripts = append(d.scripts, u)
		}
	}
	return d
}

// pagina: los datos de la página de Instagram (ruta: "/" o "/direct/inbox/"), de hace menos de una hora.
func (ig *instagram) pagina(ruta string, otraVez bool) (*datosWeb, error) {
	ig.muWeb.Lock()
	defer ig.muWeb.Unlock()
	if d := ig.paginas[ruta]; d != nil && !otraVez && time.Since(d.cuando) < time.Hour {
		return d, nil
	}
	st, cuerpo, err := ig.pedirCrudo(http.MethodGet, ruta, nil, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if st != 200 {
		logf("página %s: %d", ruta, st)
		if st == 429 {
			return nil, errLimite
		}
		return nil, fmt.Errorf("Instagram responde %d", st)
	}
	d := leerDatosWeb(string(cuerpo))
	logf("página %s: %d KB, lsd %v, fb_dtsg %v (de sesión %v), versión %v, aparato %v, %d JS", ruta, len(cuerpo)>>10,
		d.lsd != "", d.dtsg != "", d.dtsgSesion, d.hs != "" && d.spinR != "", d.dispDirect != "", len(d.scripts))
	if !d.dtsgSesion {
		// la página es la de sin sesión (solo trae el fb_dtsg de invitado): GraphQL tampoco te
		// reconocería (api/v1 puede que sí)
		return nil, errors.New("la página de Instagram no ha reconocido tu sesión")
	}
	if d.lsd == "" || d.dtsg == "" {
		return nil, errors.New("en la página de Instagram no están sus datos")
	}
	if ig.paginas == nil {
		ig.paginas = map[string]*datosWeb{}
	}
	ig.paginas[ruta] = d
	return d, nil
}

// ---- Los doc_id ----------------------------------------------------------------------------------

func (ig *instagram) cargarDocs() *docsWeb {
	if ig.docs == nil {
		ig.docs = &docsWeb{}
		if b, err := os.ReadFile(rutaDocs()); err == nil {
			_ = json.Unmarshal(b, ig.docs)
		}
	}
	return ig.docs
}

type candidato struct {
	doc  string
	prov []string
	de   string // de dónde ha salido (para el registro)
}

// candidatos: los doc_id para una consulta, en orden: el que digas tú (INSTAGRAM_DOC_<nombre>), el de
// los JS de la web (si es de hace menos de una semana) y el incluido.
func (ig *instagram) candidatos(nombre string) []candidato {
	ig.muDocs.Lock()
	defer ig.muDocs.Unlock()
	var cs []candidato
	poner := func(doc string, prov []string, de string) {
		if doc == "" {
			return
		}
		for _, c := range cs {
			if c.doc == doc {
				return
			}
		}
		cs = append(cs, candidato{doc, prov, de})
	}
	poner(os.Getenv("INSTAGRAM_DOC_"+nombre), provIncluidos[nombre], "tuyo")
	if d := ig.cargarDocs(); time.Since(d.Cuando) < 7*24*time.Hour {
		prov := d.Prov[nombre]
		if prov == nil {
			prov = provIncluidos[nombre]
		}
		poner(d.Docs[nombre], prov, "de la web")
	}
	poner(docsIncluidos[nombre], provIncluidos[nombre], "incluido")
	return cs
}

func (ig *instagram) valorProv(p string) any {
	ig.muDocs.Lock()
	defer ig.muDocs.Unlock()
	if v, ok := ig.cargarDocs().Valor[p]; ok && v != nil {
		return v
	}
	if v, ok := valoresProv[p]; ok {
		return v
	}
	return false
}

var (
	reOperacion  = regexp.MustCompile(`__d\("([A-Za-z0-9_]+)_instagramRelayOperation",\[\],\(function\([^)]*\)\{[^}]*?exports="(\d+)"`)
	reParametros = regexp.MustCompile(`__d\("([A-Za-z0-9_]+)(?:\.graphql|\$Parameters)",\[([^\]]*)\]`)
	reProveedor  = regexp.MustCompile(`__d\("([A-Za-z0-9_]+)\.relayprovider",\[[^\]]*\],\(function\([^)]*\)\{"use strict";var e=\{get:function\(\)\{return ([^}]{1,60})\}\}`)
	reDepProv    = regexp.MustCompile(`"([A-Za-z0-9_]+)\.relayprovider"`)
)

// buscarLuego: si una consulta no la reconoce Instagram, se buscan sus doc_id en los JS sin esperar
// (la próxima vez ya se usan). Como mucho una vez cada 6 horas por consulta: son muchos JS.
func (ig *instagram) buscarLuego(ruta, nombre string) {
	ig.muDocs.Lock()
	if ig.buscado == nil {
		ig.buscado = map[string]time.Time{}
	}
	if time.Since(ig.buscado[nombre]) < 6*time.Hour {
		ig.muDocs.Unlock()
		return
	}
	ig.buscado[nombre] = time.Now()
	ig.muDocs.Unlock()
	go func() {
		if _, err := ig.descubrir(ruta, nombre); err != nil {
			logf("doc_id de %s: %v", nombre, err)
		}
	}()
}

// descubrir busca en los JS de la página los doc_id (y sus variables) de las consultas que faltan
// (y, de paso, apunta todos los que salen), y los guarda en docs.json. De una en una.
func (ig *instagram) descubrir(ruta string, faltan ...string) (int, error) {
	ig.muBusca.Lock()
	defer ig.muBusca.Unlock()
	d, err := ig.pagina(ruta, false)
	if err != nil {
		return 0, err
	}
	docs := map[string]string{}
	prov := map[string][]string{}
	valor := map[string]any{}
	quedan := func() bool {
		for _, n := range faltan {
			if docs[n] == "" {
				return true
			}
		}
		return false
	}
	leidos, bytes := 0, 0
	fin := time.Now().Add(2 * time.Minute)
	for _, u := range d.scripts {
		if !quedan() || bytes > 60<<20 || leidos >= 400 || time.Now().After(fin) {
			break
		}
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", agenteNavegador)
		req.Header.Set("Referer", urlWeb+"/")
		resp, err := clienteImagen.Do(req)
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		leidos++
		bytes += len(b)
		if resp.StatusCode != 200 {
			continue
		}
		js := string(b)
		for _, m := range reOperacion.FindAllStringSubmatch(js, -1) {
			docs[m[1]] = m[2]
		}
		for _, m := range reParametros.FindAllStringSubmatch(js, -1) {
			ps := prov[m[1]]
			for _, p := range reDepProv.FindAllStringSubmatch(m[2], -1) {
				nuevo := true
				for _, q := range ps {
					nuevo = nuevo && q != p[1]
				}
				if nuevo {
					ps = append(ps, p[1])
				}
			}
			if ps == nil {
				ps = []string{}
			}
			prov[m[1]] = ps
		}
		for _, m := range reProveedor.FindAllStringSubmatch(js, -1) {
			switch v := strings.TrimSpace(m[2]); {
			case v == "!0":
				valor[m[1]] = true
			case v == "!1":
				valor[m[1]] = false
			default:
				if n, err := strconv.Atoi(v); err == nil {
					valor[m[1]] = n
				}
			}
		}
	}
	var encontrados []string
	for _, n := range faltan {
		if docs[n] != "" {
			encontrados = append(encontrados, n)
		}
	}
	logf("doc_id: %d JS (%d KB) de %d, %d consultas; de las que faltaban, %d de %d", leidos, bytes>>10, len(d.scripts), len(docs), len(encontrados), len(faltan))
	if len(docs) == 0 {
		return 0, errors.New("en los JS de Instagram no hay doc_id")
	}
	ig.muDocs.Lock()
	viejo := ig.cargarDocs()
	nuevo := &docsWeb{Cuando: time.Now(), Docs: docs, Prov: map[string][]string{}, Valor: valor}
	if time.Since(viejo.Cuando) < 7*24*time.Hour { // lo que ya se sabía y no ha salido ahora se queda
		for k, v := range viejo.Docs {
			if docs[k] == "" {
				docs[k] = v
			}
		}
		for k, v := range viejo.Valor {
			if _, ok := valor[k]; !ok {
				valor[k] = v
			}
		}
		for k, v := range viejo.Prov {
			nuevo.Prov[k] = v
		}
	}
	for k, v := range prov { // solo los de las consultas (las variables de los fragmentos no van)
		if docs[k] != "" {
			nuevo.Prov[k] = v
		}
	}
	ig.docs = nuevo
	if b, err := json.Marshal(nuevo); err == nil {
		_ = os.MkdirAll(carpetaDatos, 0o700)
		_ = os.WriteFile(rutaDocs(), b, 0o600)
	}
	ig.muDocs.Unlock()
	return len(encontrados), nil
}

// ---- La consulta ---------------------------------------------------------------------------------

// consulta: una consulta GraphQL como las de la web de 2026 (con lsd y fb_dtsg de la página y el
// doc_id de sus JS). ruta: la página desde la que la haría la web. Si Instagram no la reconoce, prueba
// los otros doc_id y, si ninguno vale, los busca en sus JS para la próxima vez.
func (ig *instagram) consulta(nombre, ruta string, vars map[string]any) (map[string]any, error) {
	return ig.consultaEn(finConsulta(nombre), nombre, ruta, vars)
}

// finConsulta: a dónde va cada consulta. Las de los mensajes, a /api/graphql/ (como mautrix-meta: en
// /graphql/query la bandeja llega vacía); las demás, a /graphql/query, como gallery-dl.
func finConsulta(nombre string) string {
	for _, p := range []string{"IGD", "useIGD", "PolarisDirect"} {
		if strings.HasPrefix(nombre, p) {
			return "/api/graphql/"
		}
	}
	return "/graphql/query"
}

// otroFin: la otra dirección de GraphQL (por si en una algo llega vacío).
func otroFin(fin string) string {
	if fin == "/api/graphql/" {
		return "/graphql/query"
	}
	return "/api/graphql/"
}

func (ig *instagram) consultaEn(fin, nombre, ruta string, vars map[string]any) (map[string]any, error) {
	if ig.cuenta == nil || !ig.cuenta.tiene() {
		return nil, errSinCuenta
	}
	var ultimo error = errDoc
	recargada := false
	for _, c := range ig.candidatos(nombre) {
		m, err := ig.enviarConsulta(fin, nombre, ruta, c, vars)
		if errors.Is(err, errRecargar) && !recargada { // otra vez con la página recién abierta
			recargada = true
			if _, e := ig.pagina(ruta, true); e == nil {
				m, err = ig.enviarConsulta(fin, nombre, ruta, c, vars)
			}
		}
		if err == nil {
			return m, nil
		}
		if !errors.Is(err, errDoc) && !errors.Is(err, errRecargar) {
			return nil, err
		}
		logf("%s (doc %s): %v", nombre, c.de, err)
		ultimo = err
	}
	if errors.Is(ultimo, errDoc) {
		ig.buscarLuego(ruta, nombre)
	}
	return nil, ultimo
}

func (ig *instagram) enviarConsulta(fin, nombre, ruta string, c candidato, vars map[string]any) (map[string]any, error) {
	d, err := ig.pagina(ruta, false)
	if err != nil {
		return nil, err
	}
	v := make(map[string]any, len(vars)+len(c.prov))
	for k, x := range vars {
		v[k] = x
	}
	for _, p := range c.prov {
		if k := "__relay_internal__pv__" + p + "relayprovider"; v[k] == nil {
			v[k] = ig.valorProv(p)
		}
	}
	vj, _ := json.Marshal(v)
	ig.mu.Lock()
	ig.nReq++
	req := strconv.FormatInt(int64(ig.nReq), 36)
	ig.mu.Unlock()
	form := url.Values{
		"av": {d.av}, "__d": {"www"}, "__user": {"0"}, "__a": {"1"}, "__req": {req}, "__hs": {d.hs}, "dpr": {"1"},
		"__ccg": {"EXCELLENT"}, "__rev": {d.rev}, "__hsi": {d.hsi}, "__comet_req": {"7"},
		"fb_dtsg": {d.dtsg}, "jazoest": {d.jazoest}, "lsd": {d.lsd},
		"__spin_r": {d.spinR}, "__spin_b": {d.spinB}, "__spin_t": {d.spinT},
		"fb_api_caller_class": {"RelayModern"}, "fb_api_req_friendly_name": {nombre},
		"variables": {string(vj)}, "server_timestamps": {"true"}, "doc_id": {c.doc},
	}
	for k, x := range form {
		if len(x) == 1 && (x[0] == "" || k == "av" && x[0] == "0") {
			delete(form, k)
		}
	}
	extra := map[string]string{"X-FB-Friendly-Name": nombre, "X-FB-LSD": d.lsd, "X-IG-Max-Touch-Points": "0", "Referer": urlWeb + ruta}
	st, cuerpo, err := ig.pedirCrudo(http.MethodPost, fin, nil, form, extra, false)
	if err != nil {
		return nil, err
	}
	return leerRespuestaGraphql(nombre, st, cuerpo)
}

// leerRespuestaGraphql: lo que ha contestado Instagram (sin escribir nada de dentro en el registro).
func leerRespuestaGraphql(nombre string, st int, cuerpo []byte) (map[string]any, error) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(cuerpo)), "for (;;);"))
	if s == "" || s[0] != '{' {
		logf("%s: %d, no es JSON (%d bytes)", nombre, st, len(cuerpo))
		if st == 401 || st == 403 {
			return nil, errRecargar
		}
		return nil, errDoc
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		// a veces llegan varias respuestas seguidas (@defer): vale la primera
		dec := json.NewDecoder(strings.NewReader(s))
		if dec.Decode(&m) != nil {
			logf("%s: %d, JSON roto (%d bytes)", nombre, st, len(cuerpo))
			return nil, errDoc
		}
	}
	if msg := txt(m, "message"); msg == "checkpoint_required" || msg == "challenge_required" {
		return nil, errReto
	} else if msg == "login_required" {
		return nil, errSesion
	}
	if cod := int(num(m, "error")); cod != 0 {
		logf("%s: %d, error %d (%s)", nombre, st, cod, corto(txt(m, "errorSummary")))
		switch cod {
		case 1357001:
			return nil, errSesion
		case 1357004, 1357005:
			return nil, errRecargar
		}
		return nil, errDoc
	}
	if st == 429 {
		return nil, errLimite
	}
	if ir(m, "data") == nil {
		if errs := lista(m, "errors"); len(errs) > 0 {
			logf("%s: %d, %s", nombre, st, corto(txt(errs[0], "message")))
		} else {
			logf("%s: %d, sin datos", nombre, st)
		}
		return nil, errDoc
	}
	if st != 200 {
		logf("%s: %d", nombre, st)
	}
	return m, nil
}

// corto: un mensaje de error de Instagram para el registro (los suyos no llevan nada tuyo, pero por si
// acaso, sin pasarse).
func corto(s string) string {
	if r := []rune(s); len(r) > 90 {
		return string(r[:90]) + "…"
	}
	return s
}

// formaJSON: la forma de una respuesta (los nombres de los campos y cuántos hay en cada lista, sin
// ningún valor), para el registro cuando algo no llega donde se esperaba.
func formaJSON(v any, prof int) string {
	switch x := v.(type) {
	case map[string]any:
		if prof <= 0 {
			return "{…}"
		}
		ks := make([]string, 0, len(x))
		for k := range x {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		var b strings.Builder
		b.WriteString("{")
		for i, k := range ks {
			if i > 0 {
				b.WriteString(" ")
			}
			if i >= 12 {
				b.WriteString("…")
				break
			}
			b.WriteString(rutaLog(k) + ":" + formaJSON(x[k], prof-1))
		}
		b.WriteString("}")
		return b.String()
	case []any:
		if len(x) == 0 {
			return "[]"
		}
		return "[" + strconv.Itoa(len(x)) + "×" + formaJSON(x[0], prof-1) + "]"
	case nil:
		return "null"
	case string:
		return "s"
	case float64:
		return "n"
	case bool:
		return "b"
	}
	return "?"
}
