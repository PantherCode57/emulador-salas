package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Spotify, como lo hace Spotin en el móvil (spotify/SpotifyAuth.kt y Spotify.kt): con la cookie
// «sp_dc» se pide el token del reproductor web (con un TOTP cuyo secreto publica un gist de la
// comunidad) y con él se usa la API GraphQL de open.spotify.com. No hace falta Premium: de Spotify
// solo sale la información; el sonido viene de YouTube.

const (
	urlToken      = "https://open.spotify.com/api/token"
	urlHoraServer = "https://open.spotify.com/api/server-time"
	urlGist       = "https://api.github.com/gists/22ed9c6ba463899e933427f7de1f0eef"
	urlGraphQL    = "https://api-partner.spotify.com/pathfinder/v2/query"
	agenteChrome  = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// Las «persisted queries» de GraphQL (SpotifyHashProvider.kt). Si Spotify cambia alguna, se puede
// poner la nueva en hashes.json (en la carpeta de datos), sin compilar otra vez.
var hashes = map[string]string{
	"libraryV3":          "973e511ca44261fda7eebac8b653155e7caee3675abb4fb110cc1b8c78b091c3",
	"fetchPlaylist":      "346811f856fb0b7e4f6c59f8ebea78dd081c6e2fb01b77c954b26259d5fc6763",
	"fetchLibraryTracks": "087278b20b743578a6262c2b0b4bcd20d879c503cc359a2285baf083ef944240",
	"searchDesktop":      "4801118d4a100f756e833d33984436a3899cff359c532f8fd3aaf174b60b3b49",
	"getAlbum":           "b9bfabef66ed756e5e13f68a942deb60bd4125ec1f1be8cc42769dc0259b4b10",
}

var errSinSesion = errors.New("la sesión de Spotify no vale")

type cuentaSpotify struct {
	ruta string
	mu   sync.Mutex
	spdc string

	token   string
	caduca  time.Time
	secreto string
	version int
	anonimo bool // sin cuenta: solo lo público (para bajar una lista pública sin sesión)
	http    *http.Client
	agente  string
}

func nuevaCuentaSpotify(ruta string) *cuentaSpotify {
	s := &cuentaSpotify{ruta: ruta, http: &http.Client{Timeout: 30 * time.Second}}
	s.agente = fmt.Sprintf("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.%d.0 Safari/537.36",
		136+rand.Intn(5), rand.Intn(500))
	if b, err := os.ReadFile(ruta); err == nil {
		var g struct {
			SpDc string `json:"sp_dc"`
		}
		if json.Unmarshal(b, &g) == nil {
			s.spdc = g.SpDc
		}
	}
	if b, err := os.ReadFile(strings.TrimSuffix(ruta, "sesion.json") + "hashes.json"); err == nil {
		var h map[string]string
		if json.Unmarshal(b, &h) == nil {
			for k, v := range h {
				hashes[k] = v
			}
		}
	}
	return s
}

func (s *cuentaSpotify) tieneSesion() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.spdc != ""
}

// ponerCookie comprueba la cookie (pidiendo un token) y, si vale, la guarda.
func (s *cuentaSpotify) ponerCookie(spdc string) error {
	var err error
	for intento := 0; intento < 3; intento++ { // el gist a veces falla: como Spotin, tres veces
		var tok string
		var cad time.Time
		tok, cad, err = s.pedirToken(spdc)
		if err == nil {
			s.mu.Lock()
			s.spdc, s.token, s.caduca = spdc, tok, cad
			s.mu.Unlock()
			b, _ := json.Marshal(map[string]string{"sp_dc": spdc})
			escribirAtomico(s.ruta, b)
			return nil
		}
		if errors.Is(err, errSinSesion) {
			return err
		}
		time.Sleep(800 * time.Millisecond)
	}
	return err
}

func (s *cuentaSpotify) get(u string, cab map[string]string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", agenteChrome)
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en")
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return b, resp.StatusCode, err
}

// secretoTOTP: el del gist (el de versión más alta). Se guarda una hora para no gastar el límite
// de GitHub (60 peticiones por hora).
func (s *cuentaSpotify) secretoTOTP(forzar bool) (string, int, error) {
	cache := strings.TrimSuffix(s.ruta, "sesion.json") + "totp.json"
	type guardado struct {
		S     string    `json:"s"`
		V     int       `json:"v"`
		Desde time.Time `json:"desde"`
	}
	var g guardado
	if !forzar {
		if b, err := os.ReadFile(cache); err == nil && json.Unmarshal(b, &g) == nil && g.S != "" && time.Since(g.Desde) < time.Hour {
			return g.S, g.V, nil
		}
	}
	b, code, err := s.get(urlGist, nil)
	if err != nil {
		return "", 0, err
	}
	if code != 200 {
		if g.S != "" { // sin GitHub: el que había, aunque sea viejo
			return g.S, g.V, nil
		}
		return "", 0, fmt.Errorf("GitHub responde %d", code)
	}
	var gist struct {
		Files map[string]struct {
			Content string `json:"content"`
		} `json:"files"`
	}
	if err := json.Unmarshal(b, &gist); err != nil {
		return "", 0, err
	}
	mejor := guardado{V: -1}
	for _, f := range gist.Files {
		var ns []struct {
			S string `json:"s"`
			V int    `json:"v"`
		}
		if json.Unmarshal([]byte(f.Content), &ns) != nil {
			continue
		}
		for _, n := range ns {
			if n.V > mejor.V && n.S != "" {
				mejor = guardado{S: n.S, V: n.V}
			}
		}
	}
	if mejor.S == "" {
		return "", 0, errors.New("el gist no trae el secreto")
	}
	mejor.Desde = time.Now()
	bb, _ := json.Marshal(mejor)
	escribirAtomico(cache, bb)
	return mejor.S, mejor.V, nil
}

var reNoBase32 = regexp.MustCompile(`[^A-Z2-7]`)

// base32 como en SpotifyAuth.kt: sin «=», salta lo que no es del alfabeto y tira los bits que sobran.
func base32Suelto(s string) []byte {
	const alfabeto = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	var out []byte
	buf, bits := 0, 0
	for _, r := range reNoBase32.ReplaceAllString(strings.ToUpper(s), "") {
		buf = buf<<5 | strings.IndexRune(alfabeto, r)
		bits += 5
		if bits >= 8 {
			out = append(out, byte(buf>>(bits-8)))
			bits -= 8
			buf &= 1<<bits - 1
		}
	}
	return out
}

func totp(secreto string, horaSeg int64) (string, error) {
	clave := base32Suelto(secreto)
	if len(clave) == 0 {
		return "", errors.New("secreto vacío")
	}
	var contador [8]byte
	binary.BigEndian.PutUint64(contador[:], uint64(horaSeg/30))
	m := hmac.New(sha1.New, clave)
	m.Write(contador[:])
	h := m.Sum(nil)
	o := h[len(h)-1] & 0x0f
	codigo := (uint32(h[o])&0x7f)<<24 | uint32(h[o+1])<<16 | uint32(h[o+2])<<8 | uint32(h[o+3])
	return fmt.Sprintf("%06d", codigo%1000000), nil
}

func (s *cuentaSpotify) pedirToken(spdc string) (string, time.Time, error) {
	for intento := 0; intento < 2; intento++ {
		secreto, version, err := s.secretoTOTP(intento == 1)
		if err != nil {
			return "", time.Time{}, err
		}
		b, code, err := s.get(urlHoraServer, nil)
		if err != nil {
			return "", time.Time{}, err
		}
		var hora struct {
			ServerTime int64 `json:"serverTime"`
		}
		if code != 200 || json.Unmarshal(b, &hora) != nil || hora.ServerTime == 0 {
			hora.ServerTime = time.Now().Unix()
		}
		codigo, err := totp(secreto, hora.ServerTime)
		if err != nil {
			return "", time.Time{}, err
		}
		u := urlToken + "?reason=transport&productType=web-player&totp=" + codigo + "&totpServer=" + codigo +
			"&totpVer=" + strconv.Itoa(version)
		cab := map[string]string{}
		if spdc != "" {
			cab["Cookie"] = "sp_dc=" + spdc
		}
		b, code, err = s.get(u, cab)
		if err != nil {
			return "", time.Time{}, err
		}
		if code == 400 || code == 403 { // el secreto pudo cambiar: otra vez con el del gist de ahora
			continue
		}
		if code != 200 {
			return "", time.Time{}, fmt.Errorf("Spotify responde %d", code)
		}
		var t struct {
			AccessToken string `json:"accessToken"`
			Caduca      int64  `json:"accessTokenExpirationTimestampMs"`
			IsAnonymous bool   `json:"isAnonymous"`
		}
		if err := json.Unmarshal(b, &t); err != nil {
			return "", time.Time{}, err
		}
		if t.AccessToken == "" || (t.IsAnonymous && spdc != "") {
			return "", time.Time{}, errSinSesion
		}
		return t.AccessToken, time.UnixMilli(t.Caduca), nil
	}
	return "", time.Time{}, errors.New("Spotify no acepta el código (¿ha cambiado el secreto?)")
}

// tokenValido: el de ahora, o uno nuevo si le queda menos de un minuto.
func (s *cuentaSpotify) tokenValido(forzar bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.spdc == "" && !s.anonimo {
		return "", errSinSesion
	}
	if !forzar && s.token != "" && time.Now().Before(s.caduca.Add(-time.Minute)) {
		return s.token, nil
	}
	t, c, err := s.pedirToken(s.spdc)
	if err != nil {
		return "", err
	}
	s.token, s.caduca = t, c
	return t, nil
}

func (s *cuentaSpotify) cabecerasWeb(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("app-platform", "WebPlayer")
	req.Header.Set("Origin", "https://open.spotify.com")
	req.Header.Set("Referer", "https://open.spotify.com/")
	req.Header.Set("User-Agent", s.agente)
}

// gql: una consulta de GraphQL (con reintentos si Spotify pide calma, y token nuevo si caducó).
func (s *cuentaSpotify) gql(op string, vars map[string]any) (map[string]any, error) {
	cuerpo, _ := json.Marshal(map[string]any{
		"variables": vars, "operationName": op,
		"extensions": map[string]any{"persistedQuery": map[string]any{"version": 1, "sha256Hash": hashes[op]}},
	})
	forzar := false
	for intento := 0; intento < 4; intento++ {
		token, err := s.tokenValido(forzar)
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequest(http.MethodPost, urlGraphQL, bytes.NewReader(cuerpo))
		s.cabecerasWeb(req, token)
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
		resp, err := s.http.Do(req)
		if err != nil {
			if intento < 2 {
				time.Sleep(time.Second)
				continue
			}
			return nil, err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == 401:
			if forzar {
				return nil, errSinSesion
			}
			forzar = true
			continue
		case resp.StatusCode == 429:
			espera, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
			if espera <= 0 {
				espera = 2 * (intento + 1)
			}
			time.Sleep(time.Duration(min(espera, 10)) * time.Second)
			continue
		case resp.StatusCode == 412:
			return nil, fmt.Errorf("%s: Spotify ha cambiado la consulta (hash viejo)", op)
		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return nil, fmt.Errorf("%s: Spotify responde %d", op, resp.StatusCode)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, err
		}
		if errs := lista(m, "errors"); len(errs) > 0 {
			return nil, fmt.Errorf("%s: %s", op, txt(errs[0], "message"))
		}
		return m, nil
	}
	return nil, fmt.Errorf("%s: Spotify no responde", op)
}

// ---- De las respuestas a Lista y Cancion --------------------------------------------------------

// imagen: de una lista de «sources», la más pequeña que tenga al menos 280 px (o la primera).
func imagen(fuentes []any) string {
	mejor, mejorW := "", 0
	for _, f := range fuentes {
		u := txt(f, "url")
		if u == "" {
			continue
		}
		w := int(num(f, "width"))
		if mejor == "" {
			mejor, mejorW = u, w
			continue
		}
		if w >= 280 && (mejorW < 280 || w < mejorW) {
			mejor, mejorW = u, w
		}
	}
	return mejor
}

func cancionDe(d any, uri string, album *Cancion) (Cancion, bool) {
	if t := txt(d, "__typename"); t != "" && t != "Track" {
		return Cancion{}, false
	}
	if uri == "" {
		uri = txt(d, "uri")
	}
	if uri == "" {
		uri = txt(d, "_uri")
	}
	c := Cancion{ID: ultimo(uri, ":"), Nombre: txt(d, "name")}
	if c.ID == "" || c.Nombre == "" {
		return c, false
	}
	for _, a := range lista(d, "artists", "items") {
		if n := txt(a, "profile", "name"); n != "" {
			c.Artistas = append(c.Artistas, n)
		}
	}
	if album != nil {
		c.Album, c.Portada = album.Album, album.Portada
	} else {
		c.Album = txt(d, "albumOfTrack", "name")
		c.Portada = imagen(lista(d, "albumOfTrack", "coverArt", "sources"))
	}
	for _, p := range [][]any{{"duration", "totalMilliseconds"}, {"trackDuration", "totalMilliseconds"}, {"durationMs"}, {"duration_ms"}} {
		if ms := num(d, p...); ms > 0 {
			c.DuracionMs = int(ms)
			break
		}
	}
	c.Explicita = txt(d, "contentRating", "label") == "EXPLICIT"
	return c, true
}

// biblioteca: tus listas y tus álbumes guardados (y cuántas canciones te gustan).
func (s *cuentaSpotify) biblioteca() ([]Lista, error) {
	var ls []Lista
	for _, filtro := range []string{"Playlists", "Albums"} {
		offset := 0
		for {
			m, err := s.gql("libraryV3", map[string]any{
				"filters": []string{filtro}, "order": nil, "textFilter": "",
				"features": []string{"LIKED_SONGS", "YOUR_EPISODES_V2", "PRERELEASES", "EVENTS"},
				"limit": 50, "offset": offset, "flatten": filtro == "Playlists", "expandedFolders": []string{},
				"folderUri": nil, "includeFoldersWhenFlattening": filtro != "Playlists",
			})
			if err != nil {
				if filtro == "Albums" && len(ls) > 0 { // los álbumes son lo de menos
					break
				}
				return nil, err
			}
			items := lista(m, "data", "me", "libraryV3", "items")
			for _, it := range items {
				w := ir(it, "item")
				uri, d := txt(w, "_uri"), ir(w, "data")
				switch {
				case filtro == "Playlists" && txt(d, "__typename") == "Playlist" && uri != "":
					var fuentes []any
					for _, g := range lista(d, "images", "items") {
						fuentes = append(fuentes, lista(g, "sources")...)
					}
					ls = append(ls, Lista{ID: ultimo(uri, ":"), Nombre: txt(d, "name"), Tipo: tipoLista,
						Dueno: txt(d, "ownerV2", "data", "name"), Portada: imagen(fuentes)})
				case filtro == "Albums" && strings.Contains(txt(w, "__typename"), "Album"):
					if uri == "" {
						uri = txt(d, "uri")
					}
					var arts []string
					for _, a := range lista(d, "artists", "items") {
						arts = append(arts, txt(a, "profile", "name"))
					}
					if n := txt(d, "name"); n != "" && uri != "" {
						ls = append(ls, Lista{ID: ultimo(uri, ":"), Nombre: n, Tipo: tipoAlbum,
							Dueno: strings.Join(arts, ", "), Portada: imagen(lista(d, "coverArt", "sources"))})
					}
				}
			}
			offset += len(items) // por las de verdad (las carpetas y demás también cuentan)
			total := int(num(m, "data", "me", "libraryV3", "totalCount"))
			if len(items) == 0 || offset >= total {
				break
			}
		}
	}
	// Cuántas canciones te gustan (para enseñarlo sin abrir la lista).
	if m, err := s.gql("fetchLibraryTracks", map[string]any{"offset": 0, "limit": 1}); err == nil {
		ls = append([]Lista{{ID: "megusta", Nombre: "Tus me gusta", Tipo: tipoMeGusta,
			Total: int(num(m, "data", "me", "library", "tracks", "totalCount"))}}, ls...)
	}
	return ls, nil
}

// canciones: todas las de una lista, de tus me gusta o de un álbum.
func (s *cuentaSpotify) canciones(l Lista) ([]Cancion, error) {
	var cs []Cancion
	switch l.Tipo {
	case tipoMeGusta:
		for offset := 0; ; {
			m, err := s.gql("fetchLibraryTracks", map[string]any{"offset": offset, "limit": 50})
			if err != nil {
				return nil, err
			}
			items := lista(m, "data", "me", "library", "tracks", "items")
			for _, it := range items {
				uri := txt(it, "track", "_uri")
				if uri == "" {
					uri = txt(it, "track", "uri")
				}
				if c, ok := cancionDe(ir(it, "track", "data"), uri, nil); ok {
					cs = append(cs, c)
				}
			}
			offset += len(items)
			if len(items) == 0 || offset >= int(num(m, "data", "me", "library", "tracks", "totalCount")) {
				break
			}
		}
	case tipoAlbum:
		for offset := 0; ; {
			m, err := s.gql("getAlbum", map[string]any{"uri": "spotify:album:" + l.ID, "locale": "", "offset": offset, "limit": 50})
			if err != nil {
				return nil, err
			}
			al := ir(m, "data", "albumUnion")
			album := &Cancion{Album: txt(al, "name"), Portada: imagen(lista(al, "coverArt", "sources"))}
			items := lista(al, "tracksV2", "items")
			for _, it := range items {
				if c, ok := cancionDe(ir(it, "track"), "", album); ok {
					cs = append(cs, c)
				}
			}
			offset += len(items)
			if len(items) == 0 || offset >= int(num(al, "tracksV2", "totalCount")) {
				break
			}
		}
	default:
		for offset := 0; ; {
			m, err := s.gql("fetchPlaylist", map[string]any{"uri": "spotify:playlist:" + l.ID, "offset": offset,
				"limit": 100, "enableWatchFeedEntrypoint": false})
			if err != nil {
				return nil, err
			}
			items := lista(m, "data", "playlistV2", "content", "items")
			for _, it := range items {
				uri := txt(it, "itemV2", "_uri")
				if uri == "" {
					uri = txt(it, "itemV2", "uri")
				}
				if c, ok := cancionDe(ir(it, "itemV2", "data"), uri, nil); ok {
					cs = append(cs, c)
				}
			}
			offset += len(items)
			if len(items) == 0 || offset >= int(num(m, "data", "playlistV2", "content", "totalCount")) {
				break
			}
		}
	}
	return cs, nil
}

func (s *cuentaSpotify) buscar(q string) ([]Cancion, error) {
	m, err := s.gql("searchDesktop", map[string]any{"searchTerm": q, "offset": 0, "limit": 30, "numberOfTopResults": 5,
		"includeAudiobooks": false, "includeArtistHasConcertsField": false, "includePreReleases": false,
		"includeLocalConcertsField": false, "includeAuthors": false})
	if err != nil {
		return nil, err
	}
	var cs []Cancion
	for _, it := range lista(m, "data", "searchV2", "tracksV2", "items") {
		w := ir(it, "item")
		if txt(w, "__typename") != "TrackResponseWrapper" {
			continue
		}
		uri := txt(w, "_uri")
		if uri == "" {
			uri = txt(w, "uri")
		}
		if c, ok := cancionDe(ir(w, "data"), uri, nil); ok {
			cs = append(cs, c)
		}
	}
	return cs, nil
}

// ---- Letras: las de Spotify y, si no, las de LRCLIB ---------------------------------------------

func (s *cuentaSpotify) letra(c Cancion) ([]lineaLetra, error) {
	if ls, err := s.letraSpotify(c.ID); err == nil && len(ls) > 0 {
		return ls, nil
	}
	return letraLRCLIB(c)
}

func (s *cuentaSpotify) letraSpotify(id string) ([]lineaLetra, error) {
	token, err := s.tokenValido(false)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, "https://spclient.wg.spotify.com/color-lyrics/v2/track/"+id+
		"?format=json&vocalRemoval=false&market=from_token", nil)
	s.cabecerasWeb(req, token)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sin letra en Spotify (%d)", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	sincro := txt(m, "lyrics", "syncType") == "LINE_SYNCED"
	var ls []lineaLetra
	for _, l := range lista(m, "lyrics", "lines") {
		w := strings.TrimSpace(txt(l, "words"))
		if w == "" || w == "♪" {
			continue
		}
		ms := 0
		if sincro {
			ms = int(num(l, "startTimeMs"))
		}
		ls = append(ls, lineaLetra{Ms: ms, Texto: w})
	}
	return ls, nil
}

var (
	reFeat    = regexp.MustCompile(`(?i)\s*[(\[][^)\]]*(feat\.?|ft\.?|with )[^)\]]*[)\]]`)
	reVersion = regexp.MustCompile(`(?i)\s*[(\[][^)\]]*(remaster|remastered|live|version|edit|mono|stereo|deluxe|bonus)[^)\]]*[)\]]`)
	reLRC     = regexp.MustCompile(`\[(\d{1,2}):(\d{2})(?:[.:](\d{1,3}))?]`)
)

func tituloLimpio(t string) string {
	if i := strings.Index(t, " - "); i > 0 {
		t = t[:i]
	}
	return strings.TrimSpace(reVersion.ReplaceAllString(reFeat.ReplaceAllString(t, ""), ""))
}

func letraLRCLIB(c Cancion) ([]lineaLetra, error) {
	artista := ""
	if len(c.Artistas) > 0 {
		artista = c.Artistas[0]
	}
	titulo := tituloLimpio(c.Nombre)
	cli := &http.Client{Timeout: 8 * time.Second}
	pedir := func(ruta string, q url.Values) ([]byte, error) {
		req, _ := http.NewRequest(http.MethodGet, "https://lrclib.net/api/"+ruta+"?"+q.Encode(), nil)
		req.Header.Set("User-Agent", "SpotinConsola/1.0 (Panther OS)")
		req.Header.Set("Accept", "application/json")
		resp, err := cli.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("LRCLIB %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	}
	type res struct {
		Synced   string  `json:"syncedLyrics"`
		Plain    string  `json:"plainLyrics"`
		Duracion float64 `json:"duration"`
		Artista  string  `json:"artistName"`
	}
	var elegido *res
	q := url.Values{"track_name": {titulo}, "artist_name": {artista}}
	if c.Album != "" {
		q.Set("album_name", c.Album)
	}
	if c.DuracionMs > 0 {
		q.Set("duration", strconv.Itoa(c.DuracionMs/1000))
	}
	if b, err := pedir("get", q); err == nil {
		var r res
		if json.Unmarshal(b, &r) == nil && (r.Synced != "" || r.Plain != "") {
			elegido = &r
		}
	}
	if elegido == nil {
		if b, err := pedir("search", url.Values{"track_name": {titulo}, "artist_name": {artista}}); err == nil {
			var rs []res
			if json.Unmarshal(b, &rs) == nil {
				mejor := 1e18
				for i := range rs {
					d := 0.0
					if c.DuracionMs > 0 && rs[i].Duracion > 0 {
						d = abs(rs[i].Duracion - float64(c.DuracionMs)/1000)
					}
					if rs[i].Synced == "" {
						d += 100000
					}
					if d < mejor && (rs[i].Synced != "" || rs[i].Plain != "") {
						mejor, elegido = d, &rs[i]
					}
				}
			}
		}
	}
	if elegido == nil {
		return nil, errors.New("sin letra")
	}
	if elegido.Synced != "" {
		var ls []lineaLetra
		for _, linea := range strings.Split(elegido.Synced, "\n") {
			marcas := reLRC.FindAllStringSubmatchIndex(linea, -1)
			if len(marcas) == 0 {
				continue
			}
			texto := strings.TrimSpace(linea[marcas[len(marcas)-1][1]:])
			for _, mm := range marcas {
				mi, _ := strconv.Atoi(linea[mm[2]:mm[3]])
				se, _ := strconv.Atoi(linea[mm[4]:mm[5]])
				ms := mi*60000 + se*1000
				if mm[6] >= 0 {
					fr := linea[mm[6]:mm[7]]
					f, _ := strconv.Atoi(fr)
					switch len(fr) {
					case 1:
						ms += f * 100
					case 2:
						ms += f * 10
					default:
						ms += f
					}
				}
				if texto != "" {
					ls = append(ls, lineaLetra{Ms: ms, Texto: texto})
				}
			}
		}
		ordenarLetra(ls)
		return ls, nil
	}
	var ls []lineaLetra
	for _, linea := range strings.Split(elegido.Plain, "\n") {
		if t := strings.TrimSpace(linea); t != "" {
			ls = append(ls, lineaLetra{Texto: t})
		}
	}
	return ls, nil
}

func ordenarLetra(ls []lineaLetra) {
	for i := 1; i < len(ls); i++ {
		for j := i; j > 0 && ls[j].Ms < ls[j-1].Ms; j-- {
			ls[j], ls[j-1] = ls[j-1], ls[j]
		}
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// listaPublica busca una lista pública por su nombre (sin sesión) y devuelve la que se llame igual.
func (s *cuentaSpotify) listaPublica(nombre string) (Lista, error) {
	m, err := s.gql("searchDesktop", map[string]any{"searchTerm": nombre, "offset": 0, "limit": 30, "numberOfTopResults": 5,
		"includeAudiobooks": false, "includeArtistHasConcertsField": false, "includePreReleases": false,
		"includeLocalConcertsField": false, "includeAuthors": false})
	if err != nil {
		return Lista{}, err
	}
	var parecidas []Lista
	for _, it := range lista(m, "data", "searchV2", "playlists", "items") {
		d := ir(it, "data")
		if txt(d, "__typename") != "Playlist" {
			continue
		}
		var fuentes []any
		for _, g := range lista(d, "images", "items") {
			fuentes = append(fuentes, lista(g, "sources")...)
		}
		l := Lista{ID: ultimo(txt(d, "uri"), ":"), Nombre: txt(d, "name"), Tipo: tipoLista,
			Dueno: txt(d, "ownerV2", "data", "name"), Portada: imagen(fuentes)}
		logf("lista encontrada: %q de %q (%s)", l.Nombre, l.Dueno, l.ID)
		if strings.EqualFold(strings.TrimSpace(l.Nombre), strings.TrimSpace(nombre)) {
			parecidas = append(parecidas, l)
		}
	}
	if len(parecidas) == 0 {
		return Lista{}, fmt.Errorf("no hay ninguna lista pública que se llame %q", nombre)
	}
	return parecidas[0], nil
}
