package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// YouTube Music, como Spotin (innertube/ y SongPlayer.kt): se busca la canción con el cliente
// WEB_REMIX (filtro «canciones»), se puntúan los resultados contra los datos de Spotify y el sonido
// se pide con los clientes de las gafas VR de Android, que dan la dirección directa del sonido (sin
// PoToken ni descifrar nada).

const (
	urlYTM          = "https://music.youtube.com/youtubei/v1/"
	versionWebRemix = "1.20260213.01.00"
	agenteFirefox   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
	filtroCanciones = "EgWKAQIIAWoKEAkQBRAKEAMQBA%3D%3D"
	filtroVideos    = "EgWKAQIQAWoKEAkQChAFEAMQBA%3D%3D"
)

type clienteYT struct {
	nombre, version, agente string
	id                      int
	extra                   map[string]string
}

var clientesSonido = []clienteYT{
	{nombre: "ANDROID_VR", id: 28, version: "1.43.32",
		agente: "com.google.android.apps.youtube.vr.oculus/1.43.32 (Linux; U; Android 12; en_US; Quest 3; Build/SQ3A.220605.009.A1; Cronet/107.0.5284.2)",
		extra:  map[string]string{"osName": "Android", "osVersion": "12", "deviceMake": "Oculus", "deviceModel": "Quest 3", "androidSdkVersion": "32"}},
	{nombre: "ANDROID_VR", id: 28, version: "1.61.48",
		agente: "com.google.android.apps.youtube.vr.oculus/1.61.48 (Linux; U; Android 12; en_US; Quest 3; Build/SQ3A.220605.009.A1; Cronet/132.0.6808.3)",
		extra:  map[string]string{"osName": "Android", "osVersion": "12", "deviceMake": "Oculus", "deviceModel": "Quest 3", "androidSdkVersion": "32"}},
	{nombre: "ANDROID_VR", id: 28, version: "1.61.48",
		agente: "com.google.android.apps.youtube.vr.oculus/1.61.48 (Linux; U; Android 12; en_US; Oculus Quest 3; Build/SQ3A.220605.009.A1; Cronet/132.0.6808.3)"},
	{nombre: "IOS", id: 5, version: "21.03.1",
		agente: "com.google.ios.youtube/21.03.1 (iPhone16,2; U; CPU iOS 18_2 like Mac OS X;)",
		extra:  map[string]string{"osVersion": "18.2.22C152"}},
}

type youtube struct {
	mu        sync.Mutex
	visitante string
	visto     time.Time
	http      *http.Client

	// Direcciones del sonido ya pedidas (valen unas 6 horas): así la siguiente canción empieza ya.
	muCache sync.Mutex
	cache   map[string]sonidoGuardado
	// Solo una consulta a yt-dlp a la vez por vídeo (la de tocar y la de adelantarse a la siguiente).
	enCurso map[string]chan struct{}

	muVivo sync.Mutex
	vivo   *ytdlpVivo // yt-dlp siempre abierto (ytdlp.go)
}

// ytdlpVivo: el yt-dlp siempre abierto (si se ha cerrado, se abre otro).
func (y *youtube) ytdlpVivo() *ytdlpVivo {
	y.muVivo.Lock()
	defer y.muVivo.Unlock()
	if y.vivo == nil || (!y.vivo.vivo() && y.vivo.err == nil) {
		y.vivo = arrancarYtdlpVivo()
	}
	if y.vivo.err != nil {
		return nil
	}
	return y.vivo
}

func (y *youtube) cerrar() {
	y.muVivo.Lock()
	defer y.muVivo.Unlock()
	y.vivo.cerrar()
}

type sonidoGuardado struct {
	f     fuenteSonido
	cli   string
	hasta time.Time
}

// yt-dlp: el que se mantiene al día con lo que YouTube cambia. Sin él, se prueban los clientes de
// Spotin (que YouTube bloquea si no se demuestra que no eres un robot).
var ytdlpBin = entorno("SPOTIN_YTDLP", filepath.Join(filepath.Dir(ejecutable()), "bin", "yt-dlp"))

func ejecutable() string {
	if e, err := os.Executable(); err == nil {
		return e
	}
	return "."
}

func nuevoYouTube() *youtube {
	y := &youtube{http: &http.Client{Timeout: 25 * time.Second}, cache: map[string]sonidoGuardado{},
		enCurso: map[string]chan struct{}{}}
	y.ytdlpVivo() // se abre ya: cuando elijas una canción, ya estará listo
	return y
}

// sonido: la dirección del sonido del vídeo (de la memoria si se pidió hace poco).
func (y *youtube) sonido(video string) (*fuenteSonido, string, error) {
	for {
		y.muCache.Lock()
		if g, ok := y.cache[video]; ok && time.Now().Before(g.hasta) {
			y.muCache.Unlock()
			f := g.f
			return &f, g.cli, nil
		}
		if espera, ok := y.enCurso[video]; ok { // ya lo está pidiendo otro hilo: a esperar su respuesta
			y.muCache.Unlock()
			<-espera
			y.muCache.Lock()
			_, hay := y.cache[video]
			y.muCache.Unlock()
			if hay {
				continue
			}
			break
		}
		y.enCurso[video] = make(chan struct{})
		y.muCache.Unlock()
		break
	}
	f, cli, err := y.sonidoNuevo(video)
	y.muCache.Lock()
	if err == nil {
		y.cache[video] = sonidoGuardado{f: *f, cli: cli, hasta: time.Now().Add(5 * time.Hour)}
	}
	if c, ok := y.enCurso[video]; ok {
		close(c)
		delete(y.enCurso, video)
	}
	y.muCache.Unlock()
	return f, cli, err
}

// olvidar: la dirección guardada ya no vale (YouTube la cortó).
func (y *youtube) olvidar(video string) {
	y.muCache.Lock()
	delete(y.cache, video)
	y.muCache.Unlock()
}

func (y *youtube) sonidoNuevo(video string) (*fuenteSonido, string, error) {
	if _, err := os.Stat(ytdlpBin); err == nil {
		f, err := y.sonidoYtdlp(video)
		if err == nil {
			return f, "yt-dlp", nil
		}
		logf("yt-dlp %s: %v", video, err)
	}
	return y.sonidoClientes(video)
}

func (y *youtube) sonidoYtdlp(video string) (*fuenteSonido, error) {
	if v := y.ytdlpVivo(); v != nil {
		inicio := time.Now()
		r, err := v.pedir(video, 45*time.Second)
		if err == nil {
			logf("sonido de %s en %.1f s", video, time.Since(inicio).Seconds())
			return fuenteDeYtdlp(r)
		}
		var ey errorYouTube
		if errors.As(err, &ey) {
			return nil, err // lo dice YouTube: pedirlo otra vez con yt-dlp daría lo mismo
		}
		logf("yt-dlp siempre abierto, %s: %v (lo pido como antes)", video, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ytdlpBin, "-J", "--no-warnings", "--no-playlist", "--no-progress",
		"-f", "bestaudio[ext=webm]/bestaudio[ext=m4a]/bestaudio", "https://music.youtube.com/watch?v="+video)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(errOut.String())
		if i := strings.LastIndex(msg, "\n"); i >= 0 {
			msg = msg[i+1:]
		}
		return nil, fmt.Errorf("%v: %s", err, msg)
	}
	var d respuestaYtdlp
	if err := json.Unmarshal(out, &d); err != nil {
		return nil, err
	}
	return fuenteDeYtdlp(&d)
}

// fuenteDeYtdlp: lo que dice yt-dlp, como fuente de sonido.
func fuenteDeYtdlp(d *respuestaYtdlp) (*fuenteSonido, error) {
	if d.URL == "" {
		return nil, errors.New("yt-dlp no ha dado la dirección")
	}
	tipo := "audio/webm; codecs=\"" + d.Acodec + "\""
	if d.Ext == "m4a" || d.Ext == "mp4" {
		tipo = "audio/mp4; codecs=\"" + d.Acodec + "\""
	}
	tam := int64(d.Tamano)
	if tam == 0 {
		tam = int64(d.Aprox)
	}
	return &fuenteSonido{Ruta: d.URL, Cabeceras: d.Cabeceras, Tipo: tipo, Tamano: tam}, nil
}

// visitor: el «visitorData» de una visita anónima a YouTube Music (se pide una vez cada pocas horas).
func (y *youtube) visitor() string {
	y.mu.Lock()
	defer y.mu.Unlock()
	if y.visitante != "" && time.Since(y.visto) < 6*time.Hour {
		return y.visitante
	}
	req, _ := http.NewRequest(http.MethodGet, "https://music.youtube.com/sw.js_data", nil)
	req.Header.Set("User-Agent", agenteFirefox)
	resp, err := y.http.Do(req)
	if err != nil {
		return y.visitante
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if len(b) > 5 {
		var v any
		if json.Unmarshal(b[5:], &v) == nil {
			for _, e := range lista(v, 0, 2) {
				if s, ok := e.(string); ok && (strings.HasPrefix(s, "Cgt") || strings.HasPrefix(s, "Cgs")) {
					y.visitante, y.visto = s, time.Now()
					break
				}
			}
		}
	}
	return y.visitante
}

func (y *youtube) pedir(ruta string, cli clienteYT, cuerpo map[string]any) (map[string]any, error) {
	vis := ""
	if cli.nombre == "WEB_REMIX" || os.Getenv("SPOTIN_VISITOR") == "1" {
		vis = y.visitor()
	}
	c := map[string]any{"clientName": cli.nombre, "clientVersion": cli.version, "gl": "ES", "hl": "es"}
	for k, v := range cli.extra {
		c[k] = v
	}
	if vis != "" {
		c["visitorData"] = vis
	}
	cuerpo["context"] = map[string]any{"client": c,
		"request": map[string]any{"internalExperimentFlags": []any{}, "useSsl": true},
		"user":    map[string]any{"lockedSafetyMode": false}}
	b, _ := json.Marshal(cuerpo)
	var ultimo error
	for intento := 0; intento < 3; intento++ {
		base := urlYTM
		if cli.nombre != "WEB_REMIX" {
			base = "https://www.youtube.com/youtubei/v1/"
		}
		req, _ := http.NewRequest(http.MethodPost, base+ruta+"?prettyPrint=false", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", "es,ES;q=0.9,en;q=0.8")
		req.Header.Set("X-Goog-Api-Format-Version", "1")
		req.Header.Set("X-YouTube-Client-Name", strconv.Itoa(cli.id))
		req.Header.Set("X-YouTube-Client-Version", cli.version)
		if cli.nombre == "WEB_REMIX" {
			req.Header.Set("X-Origin", "https://music.youtube.com")
			req.Header.Set("Referer", "https://music.youtube.com/")
		}
		req.Header.Set("User-Agent", cli.agente)
		if vis != "" {
			req.Header.Set("X-Goog-Visitor-Id", vis)
		}
		resp, err := y.http.Do(req)
		if err != nil {
			ultimo = err
			time.Sleep(time.Duration(500*(intento+1)) * time.Millisecond)
			continue
		}
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("YouTube responde %d", resp.StatusCode)
		}
		var m map[string]any
		if err := json.Unmarshal(rb, &m); err != nil {
			return nil, err
		}
		return m, nil
	}
	return nil, ultimo
}

// ---- Búsqueda ---------------------------------------------------------------------------------------

type candidato struct {
	video, titulo, album string
	artistas             []string
	segundos             int
	esVideo, explicita   bool
	nota                 float64
	vale                 bool
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

func (y *youtube) resultados(q, filtro string) ([]candidato, error) {
	cli := clienteYT{nombre: "WEB_REMIX", id: 67, version: versionWebRemix, agente: agenteFirefox}
	m, err := y.pedir("search", cli, map[string]any{"query": q, "params": filtro})
	if err != nil {
		return nil, err
	}
	var cs []candidato
	vistos := map[string]bool{}
	for _, sec := range lista(m, "contents", "tabbedSearchResultsRenderer", "tabs", 0, "tabRenderer", "content", "sectionListRenderer", "contents") {
		for _, it := range lista(sec, "musicShelfRenderer", "contents") {
			r := ir(it, "musicResponsiveListItemRenderer")
			if r == nil {
				continue
			}
			col := func(i int) []any {
				f := ir(r, "flexColumns", i)
				if runs := lista(f, "musicResponsiveListItemFlexColumnRenderer", "text", "runs"); runs != nil {
					return runs
				}
				return lista(f, "musicResponsiveListItemFixedColumnRenderer", "text", "runs")
			}
			c := candidato{video: txt(r, "playlistItemData", "videoId")}
			t := col(0)
			if c.video == "" || len(t) == 0 || vistos[c.video] {
				continue
			}
			if ir(r, "navigationEndpoint", "browseEndpoint") != nil { // artistas, álbumes, pódcast
				continue
			}
			c.titulo = txt(t[0], "text")
			var grupos [][]any
			actual := []any{}
			for _, run := range col(1) {
				if txt(run, "text") == " • " {
					grupos = append(grupos, actual)
					actual = []any{}
					continue
				}
				actual = append(actual, run)
			}
			grupos = append(grupos, actual)
			if len(grupos) == 0 || len(grupos[0]) == 0 {
				continue
			}
			if txt(grupos[0][0], "text") == "Episode" || txt(grupos[0][0], "text") == "Episodio" {
				continue
			}
			for i := 0; i < len(grupos[0]); i += 2 {
				c.artistas = append(c.artistas, txt(grupos[0][i], "text"))
			}
			if len(grupos) > 2 && ir(grupos[1], 0, "navigationEndpoint", "browseEndpoint") != nil {
				c.album = txt(grupos[1][0], "text")
			}
			c.segundos = -1
			if ult := grupos[len(grupos)-1]; len(ult) > 0 {
				c.segundos = segundosDe(txt(ult[0], "text"))
			}
			for _, b := range lista(r, "badges") {
				if txt(b, "musicInlineBadgeRenderer", "icon", "iconType") == "MUSIC_EXPLICIT_BADGE" {
					c.explicita = true
				}
			}
			tipo := txt(r, "overlay", "musicItemThumbnailOverlayRenderer", "content", "musicPlayButtonRenderer", "playNavigationEndpoint",
				"watchEndpoint", "watchEndpointMusicSupportedConfigs", "watchEndpointMusicConfig", "musicVideoType")
			c.esVideo = tipo != "" && tipo != "MUSIC_VIDEO_TYPE_ATV"
			if filtro == filtroVideos {
				c.esVideo = true
			}
			vistos[c.video] = true
			cs = append(cs, c)
		}
	}
	return cs, nil
}

// --- Parecido entre textos (como ytmusicTransferScore de Spotin) ---

var (
	reFeatBuscar = regexp.MustCompile(`(?i)\s*[(\[]\s*(feat|ft)\..*?[)\]]`)
	reNoLetra    = regexp.MustCompile(`[^\p{L}\p{Nd}\s]`)
	reEspacios   = regexp.MustCompile(`\s+`)
)

var sinTilde = strings.NewReplacer("á", "a", "à", "a", "ä", "a", "â", "a", "ã", "a", "é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i", "ó", "o", "ò", "o", "ö", "o", "ô", "o", "õ", "o", "ú", "u", "ù", "u", "ü", "u",
	"û", "u", "ñ", "n", "ç", "c")

func normal(s string) string {
	s = strings.ToLower(reFeatBuscar.ReplaceAllString(s, ""))
	s = sinTilde.Replace(s)
	s = reNoLetra.ReplaceAllString(s, " ")
	return strings.TrimSpace(reEspacios.ReplaceAllString(s, " "))
}

func parecido(a, b string) float64 {
	na, nb := normal(a), normal(b)
	if na == nb {
		return 1
	}
	ra, rb := []rune(na), []rune(nb)
	if len(ra) < 2 || len(rb) < 2 {
		return 0
	}
	pares := func(r []rune) map[string]bool {
		m := map[string]bool{}
		for i := 0; i+1 < len(r); i++ {
			m[string(r[i:i+2])] = true
		}
		return m
	}
	pa, pb := pares(ra), pares(rb)
	comun := 0
	for k := range pa {
		if pb[k] {
			comun++
		}
	}
	return 2 * float64(comun) / float64(len(pa)+len(pb))
}

// Marcas de versión («live», «remix»…): si salen en el resultado y no en Spotify, no es la canción.
var marcas = []struct {
	re   *regexp.Regexp
	dura bool
}{
	{regexp.MustCompile(`(^|\s)(re\s*mix|rmx|club mix|dance mix|dub mix|vip mix)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(alternative|alternate|alt version|demo|demo version|unreleased|rough mix|early version)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(sped\s*up|speed\s*up|fast version)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(slowed|slowed reverb|slow version)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(nightcore|daycore)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(live|concert|session|performance|en vivo|en directo)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(acoustic|unplugged|piano version|guitar version|acustica|acustico)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(cover|covered by|tribute)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(karaoke|minus one)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(instrumental|no vocals)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(mashup|mash up|bootleg|rework|flip)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(fan edit|fanmade|right version|edit audio)(\s|$)`), true},
	{regexp.MustCompile(`(^|\s)(clean|clean version|edited version)(\s|$)`), false},
	{regexp.MustCompile(`(^|\s)(explicit|explicit version|uncensored|uncut)(\s|$)`), false},
	{regexp.MustCompile(`(^|\s)(extended mix|extended version|12 inch)(\s|$)`), false},
	{regexp.MustCompile(`(^|\s)(radio edit|single edit|edit version)(\s|$)`), false},
	{regexp.MustCompile(`(^|\s)(remaster|remastered|anniversary edition)(\s|$)`), false},
}

func puntuar(c *candidato, s Cancion) {
	titulo := c.titulo
	if c.esVideo && strings.Contains(titulo, "-") {
		titulo = strings.TrimSpace(strings.SplitN(titulo, "-", 2)[1])
	}
	notaTitulo := parecido(titulo, s.Nombre)
	artista := parecido(strings.Join(c.artistas, " "), s.artistas())
	if c.esVideo {
		artista = math.Max(artista, parecido(strings.SplitN(c.titulo, "-", 2)[0], s.artistas()))
	}
	partes := []float64{notaTitulo, artista}
	esperado := float64(s.DuracionMs) / 1000
	durNota := -1.0
	if esperado > 0 && c.segundos > 0 {
		durNota = math.Max(0, math.Min(1, 1-math.Abs(float64(c.segundos)-esperado)*2/esperado))
		partes = append(partes, durNota*5)
	}
	albumNota := -1.0
	if !c.esVideo && s.Album != "" && c.album != "" {
		albumNota = parecido(c.album, s.Album)
		partes = append(partes, albumNota)
	}
	pena, dura := 0.0, false
	ref := normal(s.Nombre)
	texto := normal(c.titulo + " " + c.album)
	for _, m := range marcas {
		if m.re.MatchString(texto) && !m.re.MatchString(ref) {
			if m.dura {
				pena += 1.75
				dura = true
			} else {
				pena += 0.65
			}
		}
	}
	suma := 0.0
	for _, p := range partes {
		suma += p
	}
	k := 2.0
	if c.esVideo {
		k = 1
	}
	c.nota = math.Max(0, suma/float64(len(partes))*k-pena)
	minimo := 1.35
	switch {
	case durNota >= 0 && c.esVideo:
		minimo = 1.55
	case durNota >= 0:
		minimo = 2.25
	case c.esVideo:
		minimo = 0.78
	}
	if c.explicita != s.Explicita {
		minimo += 0.6
	}
	c.vale = c.nota >= minimo && !dura && notaTitulo >= 0.45 &&
		(artista >= 0.32 || (albumNota >= 0.45 && artista >= 0.18) || (durNota >= 0.94 && artista >= 0.25))
}

func consultaDe(s Cancion) string {
	art := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s.artistas(), ",", " "), "  ", " "))
	q := strings.TrimSpace(reFeatBuscar.ReplaceAllString(s.Nombre, "") + " " + art)
	if s.Explicita {
		q += " explicit"
	}
	return q
}

// buscar: el mejor vídeo para la canción (sin «evitar», el que ya falló).
func (y *youtube) buscar(s Cancion, evitar string) (string, error) {
	q := consultaDe(s)
	var todos []candidato
	for _, filtro := range []string{filtroCanciones, filtroVideos} {
		cs, err := y.resultados(q, filtro)
		if err != nil {
			if filtro == filtroCanciones {
				return "", err
			}
			break
		}
		for i := range cs {
			puntuar(&cs[i], s)
		}
		sort.SliceStable(cs, func(i, j int) bool {
			if cs[i].vale != cs[j].vale {
				return cs[i].vale
			}
			return cs[i].nota > cs[j].nota
		})
		for _, c := range cs {
			if c.vale && c.video != evitar {
				return c.video, nil
			}
		}
		todos = append(todos, cs...)
	}
	// Ninguno pasa el listón: el que más se parezca.
	sort.SliceStable(todos, func(i, j int) bool { return todos[i].nota > todos[j].nota })
	for _, c := range todos {
		if c.video != evitar && c.nota > 0.5 {
			return c.video, nil
		}
	}
	return "", errors.New("no está en YouTube Music")
}

// ---- Sonido ---------------------------------------------------------------------------------------

// sonidoClientes: la dirección del sonido con los clientes de Spotin (la mejor: Opus si hay).
func (y *youtube) sonidoClientes(video string) (*fuenteSonido, string, error) {
	var ultimo error = errors.New("sin sonido")
	for _, cli := range clientesSonido {
		m, err := y.pedir("player", cli, map[string]any{"videoId": video, "contentCheckOk": true, "racyCheckOk": true})
		if err != nil {
			ultimo = err
			continue
		}
		if txt(m, "videoDetails", "videoId") != video {
			ultimo = errors.New("YouTube ha dado otro vídeo")
			logf("youtube %s %s: %v (%s: %s)", cli.nombre, cli.version, ultimo, txt(m, "playabilityStatus", "status"), txt(m, "playabilityStatus", "reason"))
			continue
		}
		if est := txt(m, "playabilityStatus", "status"); est != "OK" {
			ultimo = fmt.Errorf("%s: %s", est, txt(m, "playabilityStatus", "reason"))
			logf("youtube %s %s: %v", cli.nombre, cli.version, ultimo)
			continue
		}
		var mejor map[string]any
		mejorNota := -1.0
		for _, f := range lista(m, "streamingData", "adaptiveFormats") {
			fm, _ := f.(map[string]any)
			if fm == nil || fm["width"] != nil || txt(fm, "url") == "" {
				continue
			}
			if v, ok := ir(fm, "audioTrack", "isAutoDubbed").(bool); ok && v {
				continue
			}
			mime := txt(fm, "mimeType")
			if !strings.HasPrefix(mime, "audio/") {
				continue
			}
			nota := num(fm, "bitrate")
			if strings.HasPrefix(mime, "audio/webm") {
				nota += 10240
			}
			if nota > mejorNota {
				mejor, mejorNota = fm, nota
			}
		}
		if mejor == nil {
			ultimo = errors.New("sin formato de sonido directo")
			logf("youtube %s %s: %v", cli.nombre, cli.version, ultimo)
			continue
		}
		f := &fuenteSonido{Ruta: txt(mejor, "url"), Tipo: txt(mejor, "mimeType"),
			Cabeceras: map[string]string{"User-Agent": cli.agente}}
		if n, err := strconv.ParseInt(txt(mejor, "contentLength"), 10, 64); err == nil {
			f.Tamano = n
		}
		if !y.probar(f) {
			ultimo = errors.New("la dirección del sonido no responde")
			logf("youtube %s %s: %v", cli.nombre, cli.version, ultimo)
			continue
		}
		return f, cli.nombre + " " + cli.version, nil
	}
	return nil, "", ultimo
}

// probar: un byte, para saber si la dirección sirve (403 = no).
func (y *youtube) probar(f *fuenteSonido) bool {
	req, _ := http.NewRequest(http.MethodGet, f.Ruta, nil)
	for k, v := range f.Cabeceras {
		req.Header.Set(k, v)
	}
	req.Header.Set("Range", "bytes=0-0")
	resp, err := y.http.Do(req)
	if err != nil {
		return true // sin respuesta: se intenta igual
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

// probarYouTube (spotin -probar "artista - título"): para revisar la búsqueda en la consola.
func probarYouTube(q string) {
	art, tit := "", q
	if i := strings.Index(q, " - "); i > 0 {
		art, tit = q[:i], q[i+3:]
	}
	y := nuevoYouTube()
	c := Cancion{Nombre: tit, Artistas: []string{art}}
	v, err := y.buscar(c, "")
	if err != nil {
		fmt.Println("búsqueda:", err)
		os.Exit(1)
	}
	fmt.Println("vídeo:", v)
	f, cli, err := y.sonido(v)
	if err != nil {
		fmt.Println("sonido:", err)
		os.Exit(1)
	}
	fmt.Printf("sonido con %s: %s, %d bytes\n", cli, f.Tipo, f.Tamano)
	tmp := filepath.Join(os.TempDir(), "spotin-prueba.webm")
	if err := descargar(f, tmp, func(h, t int64) { fmt.Printf("  %d / %d\n", h, t) }); err != nil {
		fmt.Println("descarga:", err)
		os.Exit(1)
	}
	st, _ := os.Stat(tmp)
	fmt.Println("descargado:", st.Size(), "bytes en", tmp)
}
