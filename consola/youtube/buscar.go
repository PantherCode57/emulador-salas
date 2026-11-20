package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Búsqueda: la misma petición que hace la web de YouTube al buscar (innertube, cliente WEB, sin
// cuenta y solo vídeos). Si falla, la de yt-dlp («ytsearch»), que es más lenta pero no cambia.
const (
	urlInnertube    = "https://www.youtube.com/youtubei/v1/"
	versionWeb      = "2.20260213.01.00"
	agenteNavegador = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
	filtroSoloVideo = "EgIQAQ%3D%3D"
)

// Video: un vídeo de una búsqueda, del historial o de lo descargado.
type Video struct {
	ID       string `json:"id"`
	Titulo   string `json:"titulo"`
	Canal    string `json:"canal,omitempty"`
	Segundos int    `json:"segundos,omitempty"`
	Vistas   string `json:"vistas,omitempty"` // «1,2 M visualizaciones»
	Fecha    string `json:"fecha,omitempty"`  // «hace 3 años»
	Directo  bool   `json:"directo,omitempty"`
	Corto    bool   `json:"corto,omitempty"` // un Short (vertical)
	Mini     string `json:"mini,omitempty"`  // la miniatura, si no es la de siempre (la vertical de los Shorts)
	Seq      string `json:"-"`               // para pedir los Shorts que van detrás de este
}

func (v Video) miniatura() string {
	if v.Mini != "" {
		return v.Mini
	}
	return "https://i.ytimg.com/vi/" + v.ID + "/mqdefault.jpg"
}

// claveMini: el nombre de su miniatura (la vertical de un Short es otra imagen).
func (v Video) claveMini() string {
	if v.Corto && v.Mini != "" {
		return v.ID + "_v"
	}
	return v.ID
}

func (v Video) duracion() time.Duration { return time.Duration(v.Segundos) * time.Second }

// yt-dlp: el que se mantiene al día con lo que YouTube cambia. Es el mismo que usa Spotin.
var ytdlpBin = entorno("YOUTUBE_YTDLP", rutaYtdlp())

func rutaYtdlp() string {
	dir := filepath.Dir(ejecutable())
	for _, r := range []string{filepath.Join(dir, "bin", "yt-dlp"), filepath.Join(dir, "..", "spotin", "bin", "yt-dlp")} {
		if _, err := os.Stat(r); err == nil {
			return r
		}
	}
	return "yt-dlp"
}

func ejecutable() string {
	if e, err := os.Executable(); err == nil {
		return e
	}
	return "."
}

type youtube struct {
	http      *http.Client
	cuenta    *cuenta
	mu        sync.Mutex
	visitante string

	muCache sync.Mutex
	cache   map[string]fuenteGuardada
	enCurso map[string]chan struct{} // una sola consulta a yt-dlp a la vez por vídeo
}

type fuenteGuardada struct {
	f     fuenteVideo
	hasta time.Time
}

func nuevoYouTube() *youtube {
	return &youtube{http: &http.Client{Timeout: 20 * time.Second}, cache: map[string]fuenteGuardada{},
		enCurso: map[string]chan struct{}{}, cuenta: &cuenta{}}
}

// pedir: una petición a innertube, con tu sesión si la hay.
func (y *youtube) pedir(ruta string, cuerpo map[string]any) (map[string]any, error) {
	return y.pedirCon(y.cuenta, ruta, cuerpo)
}

func (y *youtube) pedirCon(c *cuenta, ruta string, cuerpo map[string]any) (map[string]any, error) {
	y.mu.Lock()
	vis := y.visitante
	y.mu.Unlock()
	cli := map[string]any{"clientName": "WEB", "clientVersion": versionWeb, "gl": "ES", "hl": "es"}
	if vis != "" {
		cli["visitorData"] = vis
	}
	cuerpo["context"] = map[string]any{"client": cli, "user": map[string]any{"lockedSafetyMode": false}}
	b, _ := json.Marshal(cuerpo)
	req, _ := http.NewRequest(http.MethodPost, urlInnertube+ruta+"?prettyPrint=false", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Language", "es-ES,es;q=0.9")
	req.Header.Set("User-Agent", agenteNavegador)
	req.Header.Set("X-YouTube-Client-Name", "1")
	req.Header.Set("X-YouTube-Client-Version", versionWeb)
	req.Header.Set("Origin", "https://www.youtube.com")
	if vis != "" {
		req.Header.Set("X-Goog-Visitor-Id", vis)
	}
	if c != nil {
		for k, v := range c.cabeceras() {
			req.Header.Set(k, v)
		}
	}
	resp, err := y.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, errSesion
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("YouTube responde %d", resp.StatusCode)
	}
	var m map[string]any
	if err := json.Unmarshal(rb, &m); err != nil {
		return nil, err
	}
	if v := txt(m, "responseContext", "visitorData"); v != "" {
		y.mu.Lock()
		y.visitante = v
		y.mu.Unlock()
	}
	return m, nil
}

// buscar: la primera página de vídeos (y la marca para pedir más).
func (y *youtube) buscar(q string) ([]Video, string, error) {
	m, err := y.pedir("search", map[string]any{"query": q, "params": filtroSoloVideo})
	if err == nil {
		vids, token := recogerVideos(m)
		if len(vids) > 0 {
			return vids, token, nil
		}
		err = errors.New("YouTube no ha dado vídeos")
	}
	logf("búsqueda de la web: %v; pruebo con yt-dlp", err)
	vids, err2 := buscarYtdlp(q)
	if err2 != nil {
		return nil, "", fmt.Errorf("%v (y yt-dlp: %v)", err, err2)
	}
	return vids, "", nil
}

// mas: la siguiente página de una búsqueda.
func (y *youtube) mas(token string) ([]Video, string, error) {
	m, err := y.pedir("search", map[string]any{"continuation": token})
	if err != nil {
		return nil, "", err
	}
	vids, nuevo := recogerVideos(m)
	return vids, nuevo, nil
}

// errSesion: YouTube no acepta tu sesión (caducó o se cerró).
var errSesion = errors.New("la sesión de YouTube ya no vale")

// Las piezas de las respuestas que son un vídeo (la web cambia de una a otra según la pantalla).
var piezasVideo = map[string]func(map[string]any) Video{
	"videoRenderer":         videoDe,
	"gridVideoRenderer":     videoDe,
	"compactVideoRenderer":  videoDe,
	"playlistVideoRenderer": videoDe,
	"lockupViewModel":       videoDeLockup,
	"reelItemRenderer":      cortoDe,
	"shortsLockupViewModel": cortoDeLockup,
}

// recogerVideos busca en toda la respuesta los vídeos (en orden) y la marca para seguir.
func recogerVideos(m any) ([]Video, string) {
	var out []Video
	token := ""
	vistos := map[string]bool{}
	var andar func(v any)
	andar = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			claves := make([]string, 0, len(x))
			for k := range x {
				claves = append(claves, k)
			}
			sort.Strings(claves)
			for _, k := range claves {
				if f, ok := piezasVideo[k]; ok {
					if pm, ok := x[k].(map[string]any); ok {
						if vid := f(pm); vid.ID != "" && !vistos[vid.ID] {
							vistos[vid.ID] = true
							out = append(out, vid)
						}
						continue
					}
				}
				if k == "continuationCommand" && token == "" {
					token = txt(x, k, "token")
				}
				andar(x[k])
			}
		case []any:
			for _, e := range x {
				andar(e)
			}
		}
	}
	andar(m)
	return out, token
}

// textoDe: los textos de YouTube vienen como {"simpleText": …}, {"runs": [{"text": …}, …]} o, en
// las piezas nuevas, {"content": …}.
func textoDe(v any) string {
	if s := txt(v, "simpleText"); s != "" {
		return s
	}
	if s := txt(v, "content"); s != "" {
		return s
	}
	var b strings.Builder
	for _, r := range lista(v, "runs") {
		b.WriteString(txt(r, "text"))
	}
	return b.String()
}

func videoDe(vr map[string]any) Video {
	v := Video{ID: txt(vr, "videoId"), Titulo: textoDe(vr["title"])}
	if v.Titulo == "" {
		v.Titulo = textoDe(vr["headline"])
	}
	for _, k := range []string{"ownerText", "longBylineText", "shortBylineText"} {
		if v.Canal = textoDe(vr[k]); v.Canal != "" {
			break
		}
	}
	v.Segundos = max(segundosDe(textoDe(vr["lengthText"])), 0)
	if v.Segundos == 0 {
		if n, err := strconv.Atoi(txt(vr, "lengthSeconds")); err == nil {
			v.Segundos = n
		}
	}
	v.Vistas = textoDe(vr["shortViewCountText"])
	if v.Vistas == "" {
		v.Vistas = textoDe(vr["viewCountText"])
	}
	v.Fecha = textoDe(vr["publishedTimeText"])
	if v.Segundos == 0 {
		if strings.Contains(fmt.Sprint(vr["navigationEndpoint"]), "reelWatchEndpoint") {
			v.Corto = true
		} else {
			v.Directo = true
		}
	}
	return v
}

// videoDeLockup: la pieza nueva («lockupViewModel») de la página de inicio y las suscripciones.
func videoDeLockup(l map[string]any) Video {
	if t := txt(l, "contentType"); t != "" && t != "LOCKUP_CONTENT_TYPE_VIDEO" {
		return Video{}
	}
	v := Video{ID: txt(l, "contentId")}
	md := ir(l, "metadata", "lockupMetadataViewModel")
	v.Titulo = textoDe(ir(md, "title"))
	var partes []string
	for _, fila := range lista(md, "metadata", "contentMetadataViewModel", "metadataRows") {
		for _, p := range lista(fila, "metadataParts") {
			if t := textoDe(ir(p, "text")); t != "" {
				partes = append(partes, t)
			}
		}
	}
	if len(partes) > 0 {
		v.Canal = partes[0]
	}
	for _, p := range partes[min(1, len(partes)):] {
		switch {
		case strings.HasPrefix(p, "hace") || strings.Contains(p, " ago"):
			v.Fecha = p
		case v.Vistas == "":
			v.Vistas = p
		}
	}
	// La duración va en una etiqueta encima de la miniatura («12:34», o «EN DIRECTO»).
	var buscar func(x any)
	buscar = func(x any) {
		switch y := x.(type) {
		case map[string]any:
			for k, e := range y {
				if s, ok := e.(string); ok && k == "text" {
					if n := segundosDe(s); n > 0 && v.Segundos == 0 {
						v.Segundos = n
					} else if strings.Contains(strings.ToUpper(s), "DIRECTO") || strings.EqualFold(s, "LIVE") {
						v.Directo = true
					}
				}
				buscar(e)
			}
		case []any:
			for _, e := range y {
				buscar(e)
			}
		}
	}
	buscar(ir(l, "contentImage"))
	return v
}

// cortoDe: un Short de las piezas viejas («reelItemRenderer»).
func cortoDe(r map[string]any) Video {
	v := Video{ID: txt(r, "videoId"), Titulo: textoDe(r["headline"]), Vistas: textoDe(r["viewCountText"]), Corto: true}
	v.Mini = txt(r, "thumbnail", "thumbnails", 0, "url")
	v.Seq = txt(r, "navigationEndpoint", "reelWatchEndpoint", "sequenceParams")
	return v
}

// cortoDeLockup: un Short de las piezas nuevas («shortsLockupViewModel»).
func cortoDeLockup(r map[string]any) Video {
	ep := ir(r, "onTap", "innertubeCommand", "reelWatchEndpoint")
	v := Video{ID: txt(ep, "videoId"), Corto: true}
	if v.ID == "" {
		v.ID = strings.TrimPrefix(txt(r, "entityId"), "shorts-shelf-item-")
	}
	v.Titulo = textoDe(ir(r, "overlayMetadata", "primaryText"))
	v.Vistas = textoDe(ir(r, "overlayMetadata", "secondaryText"))
	v.Mini = txt(r, "thumbnail", "sources", 0, "url")
	if v.Mini == "" {
		v.Mini = txt(ep, "thumbnail", "thumbnails", 0, "url")
	}
	v.Seq = txt(ep, "sequenceParams")
	return v
}

func segundosDe(s string) int {
	partes := strings.Split(strings.TrimSpace(s), ":")
	if len(partes) < 2 || len(partes) > 3 {
		return -1
	}
	t := 0
	for _, p := range partes {
		n, err := strconv.Atoi(p)
		if err != nil {
			return -1
		}
		t = t*60 + n
	}
	return t
}

// buscarYtdlp: la búsqueda de yt-dlp (20 resultados, sin bajar nada de cada vídeo).
func buscarYtdlp(q string) ([]Video, error) {
	out, err := ytdlpJSON([]string{"-J", "--flat-playlist", "--no-warnings"}, "ytsearch20:"+q, 45*time.Second)
	if err != nil {
		return nil, err
	}
	var d struct {
		Entries []struct {
			ID       string  `json:"id"`
			Title    string  `json:"title"`
			Channel  string  `json:"channel"`
			Uploader string  `json:"uploader"`
			Duration float64 `json:"duration"`
			Views    float64 `json:"view_count"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}
	var vids []Video
	for _, e := range d.Entries {
		v := Video{ID: e.ID, Titulo: e.Title, Canal: e.Channel, Segundos: int(e.Duration)}
		if v.Canal == "" {
			v.Canal = e.Uploader
		}
		if e.Views > 0 {
			v.Vistas = vistasCortas(e.Views)
		}
		v.Directo = v.Segundos == 0
		vids = append(vids, v)
	}
	return vids, nil
}

func vistasCortas(n float64) string {
	switch {
	case n >= 1e9:
		return strings.Replace(fmt.Sprintf("%.1f mil M de visualizaciones", n/1e9), ".", ",", 1)
	case n >= 1e6:
		return strings.Replace(fmt.Sprintf("%.1f M de visualizaciones", n/1e6), ".", ",", 1)
	case n >= 1e3:
		return fmt.Sprintf("%.0f mil visualizaciones", n/1e3)
	}
	return fmt.Sprintf("%.0f visualizaciones", n)
}

// ---- De dónde se ve un vídeo ------------------------------------------------------------------------

// Formas de sacar un vídeo, de mejor a peor para la consola (todo H.264, que su procesador abre sin
// esfuerzo; YouTube ya casi no da el MP4 de 360p con el sonido dentro, el 18):
//  0. vídeo de 360p y su sonido (aparte o juntos), con los clientes que elige yt-dlp;
//  1. lo mismo con el cliente de Safari, que da listas HLS (otro camino si el primero da 403);
//  2. que lo baje el propio yt-dlp mientras se ve (no se puede adelantar, pero entra casi siempre).
type formaVer struct {
	formato string
	extra   []string
}

// Los verticales (los Shorts) van a 360 de ancho (360×640): se ven a toda la altura de la pantalla.
const formatoVertical = "bv*[vcodec^=avc1][aspect_ratio<1][width<=360]+ba[acodec^=mp4a]/"

var formatosVer = []formaVer{
	{formato: formatoVertical + "18/bv*[height<=360][vcodec^=avc1]+ba[acodec^=mp4a]/bv*[height<=360][vcodec^=avc1]+ba/b[height<=480][vcodec^=avc1]/b[height<=480]"},
	{formato: "93/92/94/b[protocol^=m3u8][height<=480]/bv*[height<=360][vcodec^=avc1]+ba/b[height<=480]",
		extra: []string{"--extractor-args", "youtube:player_client=web_safari"}},
}

const formatoTubo = "18/b[height<=480][vcodec^=avc1]/b[height<=480]"

// intentosFuente: cuántas formas hay de sacar un vídeo (la última es la de yt-dlp por tubo).
const intentosFuente = 3

func urlVideo(id string) string { return "https://www.youtube.com/watch?v=" + id }

// fuente: de dónde se ve el vídeo, de la forma «intento» (0, 1 o 2).
func (y *youtube) fuente(id string, intento int) (*fuenteVideo, error) {
	if intento >= len(formatosVer) {
		return &fuenteVideo{Tubo: []string{"-f", formatoTubo, "-o", "-", "--no-part", "--quiet", "--no-warnings",
			"--no-playlist", urlVideo(id)}}, nil
	}
	clave := fmt.Sprintf("%s|%d", id, intento)
	for {
		y.muCache.Lock()
		if g, ok := y.cache[clave]; ok && time.Now().Before(g.hasta) {
			y.muCache.Unlock()
			f := g.f
			return &f, nil
		}
		espera, ok := y.enCurso[clave]
		if !ok {
			y.enCurso[clave] = make(chan struct{})
			y.muCache.Unlock()
			break
		}
		y.muCache.Unlock()
		<-espera // ya lo está pidiendo otro hilo (el que se adelanta): a esperar su respuesta
		y.muCache.Lock()
		_, hay := y.cache[clave]
		y.muCache.Unlock()
		if !hay {
			return nil, errors.New("no se ha podido sacar el vídeo")
		}
	}
	f, err := fuenteYtdlp(id, formatosVer[intento])
	if err != nil && strings.Contains(err.Error(), "nvalid") && strings.HasPrefix(formatosVer[intento].formato, formatoVertical) {
		// por si este yt-dlp no entiende «aspect_ratio»: sin lo de los verticales
		forma := formatosVer[intento]
		forma.formato = strings.TrimPrefix(forma.formato, formatoVertical)
		f, err = fuenteYtdlp(id, forma)
	}
	y.muCache.Lock()
	if err == nil {
		y.cache[clave] = fuenteGuardada{f: *f, hasta: time.Now().Add(4 * time.Hour)}
	}
	close(y.enCurso[clave])
	delete(y.enCurso, clave)
	y.muCache.Unlock()
	return f, err
}

// preparado: si ya se sabe de dónde se ve (para no esperar al abrirlo).
func (y *youtube) preparado(id string) bool {
	y.muCache.Lock()
	defer y.muCache.Unlock()
	g, ok := y.cache[id+"|0"]
	return ok && time.Now().Before(g.hasta)
}

func (y *youtube) olvidar(id string) {
	y.muCache.Lock()
	defer y.muCache.Unlock()
	for k := range y.cache {
		if strings.HasPrefix(k, id+"|") {
			delete(y.cache, k)
		}
	}
}

type formatoYtdlp struct {
	URL       string            `json:"url"`
	Vcodec    string            `json:"vcodec"`
	Acodec    string            `json:"acodec"`
	FPS       float64           `json:"fps"`
	Ancho     float64           `json:"width"`
	Alto      float64           `json:"height"`
	Cabeceras map[string]string `json:"http_headers"`
}

func fuenteYtdlp(id string, forma formaVer) (*fuenteVideo, error) {
	args := append([]string{"-J", "--no-warnings", "--no-playlist", "-f", forma.formato}, forma.extra...)
	out, err := ytdlpJSON(args, urlVideo(id), 60*time.Second)
	if err != nil {
		return nil, err
	}
	var d struct {
		formatoYtdlp
		Duracion  float64        `json:"duration"`
		Pedidos   []formatoYtdlp `json:"requested_formats"`
		EnDirecto bool           `json:"is_live"`
		Titulo    string         `json:"title"`
		Canal     string         `json:"channel"`
		Subido    string         `json:"uploader"`
		Vistas    float64        `json:"view_count"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}
	f := &fuenteVideo{Duracion: time.Duration(d.Duracion * float64(time.Second)), FPS: d.FPS,
		Titulo: d.Titulo, Canal: d.Canal}
	if f.Canal == "" {
		f.Canal = d.Subido
	}
	if d.Vistas > 0 {
		f.Vistas = vistasCortas(d.Vistas)
	}
	w, h := d.Ancho, d.Alto
	if len(d.Pedidos) >= 2 {
		for _, p := range d.Pedidos {
			if p.Vcodec != "" && p.Vcodec != "none" {
				f.Video, f.FPS, f.Cabeceras = p.URL, p.FPS, p.Cabeceras
				w, h = p.Ancho, p.Alto
			} else if p.Acodec != "" && p.Acodec != "none" {
				f.Audio = p.URL
			}
		}
	} else {
		f.Video, f.Cabeceras = d.URL, d.Cabeceras
	}
	f.Vertical = h > w*1.1
	if f.Video == "" {
		return nil, errors.New("yt-dlp no ha dado la dirección del vídeo")
	}
	return f, nil
}

// probarFilas (youtube -probar-filas "…"): los Shorts de una búsqueda, los que van detrás del
// primero (la secuencia) y, si hay cuenta guardada, tus filas.
func probarFilas(q string) {
	y := nuevoYouTube()
	y.cuenta = cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	m, err := y.pedir("search", map[string]any{"query": q})
	if err != nil {
		fmt.Println("no se pudo buscar:", err)
		return
	}
	vids, _ := recogerVideos(m)
	var cortos []Video
	for _, v := range vids {
		if v.Corto {
			cortos = append(cortos, v)
		}
	}
	fmt.Printf("%d vídeos, %d Shorts\n", len(vids), len(cortos))
	for i, v := range cortos {
		if i < 5 {
			fmt.Printf("  short %s  %-50.50s  %s  seq:%v mini:%v\n", v.ID, v.Titulo, v.Vistas, v.Seq != "", v.Mini != "")
		}
	}
	for _, v := range cortos {
		if v.Seq == "" {
			continue
		}
		sig, token, err := y.secuenciaShorts(v.Seq)
		fmt.Printf("secuencia: %d Shorts, ¿sigue?: %v, error: %v\n", len(sig), token != "", err)
		for i, s := range sig {
			if i < 5 {
				fmt.Printf("  %s seq:%v\n", s.ID, s.Seq != "")
			}
		}
		if token != "" {
			mas, token2, err := y.secuenciaShorts(token)
			fmt.Printf("la siguiente tanda: %d Shorts, ¿sigue?: %v, error: %v\n", len(mas), token2 != "", err)
		}
		break
	}
	if !y.cuenta.tiene() {
		fmt.Println("(sin cuenta guardada: no pruebo tus filas)")
		return
	}
	for _, f := range feedsCuenta() {
		if f.Cortos {
			continue
		}
		v, token, err := y.explorar(f.Clave)
		n := 0
		for _, x := range v {
			if x.Corto {
				n++
			}
		}
		fmt.Printf("%-22s %3d vídeos (%d Shorts)  ¿más?: %v  %v\n", f.Nombre, len(v)-n, n, token != "", err)
		for i, x := range v {
			if i < 3 {
				fmt.Printf("    %s  %-50.50s  %s  %s\n", x.ID, x.Titulo, x.Canal, fmtDur(x.duracion()))
			}
		}
	}
}

// probarBusqueda (youtube -probar "…"): busca, enseña los primeros resultados y de dónde se vería
// el primero.
func probarBusqueda(q string) {
	y := nuevoYouTube()
	t0 := time.Now()
	vids, token, err := y.buscar(q)
	if err != nil {
		fmt.Println("no se pudo buscar:", err)
		os.Exit(1)
	}
	fmt.Printf("%d vídeos en %v (¿hay más?: %v)\n", len(vids), time.Since(t0).Round(time.Millisecond), token != "")
	for i, v := range vids {
		if i >= 8 {
			break
		}
		fmt.Printf("  %s  %-60.60s  %s  %s  %s  %s\n", v.ID, v.Titulo, v.Canal, fmtDur(v.duracion()), v.Vistas, v.Fecha)
	}
	if len(vids) == 0 {
		return
	}
	for i := 0; i < len(formatosVer); i++ {
		t0 = time.Now()
		f, err := y.fuente(vids[0].ID, i)
		if err != nil {
			fmt.Printf("forma %d: %v\n", i, err)
			continue
		}
		fmt.Printf("forma %d en %v: %d s, %.0f fps, sonido aparte: %v\n  %.90s…\n", i, time.Since(t0).Round(time.Millisecond),
			int(f.Duracion.Seconds()), f.FPS, f.Audio != "", f.Video)
	}
}
