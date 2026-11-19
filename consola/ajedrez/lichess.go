package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Lichess, con su API oficial (lichess.org/api) y tu token personal: la «Board API», que es la que
// Lichess tiene para jugar desde tableros y programas que no son su web. Con ella se reta a un
// amigo, se juega contra la máquina de Lichess, se aceptan retos y se mandan las jugadas. Lo que va
// pasando llega por dos «streams» (líneas de JSON que no se cierran): el de tus avisos
// (/api/stream/event) y el de cada partida (/api/board/game/stream/<id>).
//
// En el registro solo se apunta adónde se llama y qué contesta (código), nunca el token.
const (
	urlLichess = "https://lichess.org"
	agente     = "PantherOS-ajedrez/1.0 (consola portatil con Linux)" // Lichess pide decir qué programa es
)

var (
	clienteDatos  = &http.Client{Timeout: 25 * time.Second}
	clienteStream = &http.Client{} // sin límite: los streams duran lo que dure la partida
	errSesion     = errors.New("el token no vale (¿lo has borrado en Lichess?)")
	errLimite     = errors.New("Lichess dice que esperes un minuto")
)

type lichess struct {
	cuenta *cuenta
}

func (li *lichess) peticion(ctx context.Context, metodo, ruta string, form url.Values) (*http.Request, error) {
	token := li.cuenta.token()
	if token == "" {
		return nil, errSesion
	}
	var cuerpo io.Reader
	if form != nil {
		cuerpo = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, metodo, urlLichess+ruta, cuerpo)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", agente)
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req, nil
}

// pedir: una llamada normal (no stream). Devuelve el JSON de la respuesta.
func (li *lichess) pedir(metodo, ruta string, form url.Values) (map[string]any, error) {
	req, err := li.peticion(context.Background(), metodo, ruta, form)
	if err != nil {
		return nil, err
	}
	resp, err := clienteDatos.Do(req)
	if err != nil {
		logf("%s %s: %v", metodo, sinQuery(ruta), err)
		return nil, err
	}
	defer resp.Body.Close()
	datos, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	logf("%s %s: %d", metodo, sinQuery(ruta), resp.StatusCode)
	var m map[string]any
	_ = json.Unmarshal(datos, &m)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return m, errSesion
	case resp.StatusCode == http.StatusTooManyRequests:
		return m, errLimite
	case resp.StatusCode >= 400:
		if e := texto(m, "error"); e != "" {
			return m, fmt.Errorf("Lichess: %s", e)
		}
		return m, fmt.Errorf("Lichess responde %d", resp.StatusCode)
	}
	return m, nil
}

func sinQuery(s string) string {
	s, _, _ = strings.Cut(s, "?")
	return s
}

// stream: lee las líneas de JSON hasta que se cierre o se cancele ctx (las vacías son para que no
// se corte la conexión).
func (li *lichess) stream(ctx context.Context, metodo, ruta string, form url.Values, cadaUna func(map[string]any)) error {
	req, err := li.peticion(ctx, metodo, ruta, form)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/x-ndjson")
	resp, err := clienteStream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	logf("stream %s: %d", sinQuery(ruta), resp.StatusCode)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errSesion
	case resp.StatusCode == http.StatusTooManyRequests:
		return errLimite
	case resp.StatusCode >= 400:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		if e := texto(m, "error"); e != "" {
			return fmt.Errorf("Lichess: %s", e)
		}
		return fmt.Errorf("Lichess responde %d", resp.StatusCode)
	}
	r := bufio.NewReaderSize(resp.Body, 64<<10)
	for {
		linea, err := r.ReadBytes('\n')
		if l := bytes.TrimSpace(linea); len(l) > 0 {
			var m map[string]any
			if json.Unmarshal(l, &m) == nil {
				cadaUna(m)
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// ---- Lo que se le pide -----------------------------------------------------------------------------

type miCuenta struct {
	Usuario  string
	ID       string
	Ratings  map[string]int // blitz, rapid, classical, correspondence, puzzle
	Jugando  int
	Partidas int
}

func (li *lichess) Cuenta() (miCuenta, error) {
	m, err := li.pedir("GET", "/api/account", nil)
	if err != nil {
		return miCuenta{}, err
	}
	c := miCuenta{Usuario: texto(m, "username"), ID: texto(m, "id"), Ratings: map[string]int{}}
	for k, v := range obj(m, "perfs") {
		if r, ok := numero(v, "rating"); ok {
			c.Ratings[k] = r
		}
	}
	c.Partidas = entero(obj(m, "count"), "all")
	c.Jugando = entero(obj(m, "count"), "playing")
	return c, nil
}

// partidaEnCurso: una de «Partidas empezadas».
type partidaEnCurso struct {
	ID        string
	Rival     string
	RivalElo  int
	Maquina   int // nivel, si es contra la máquina
	Blancas   bool
	TeToca    bool
	Segundos  int
	Velocidad string
	FEN       string
}

func (li *lichess) Jugando() ([]partidaEnCurso, error) {
	m, err := li.pedir("GET", "/api/account/playing?nb=20", nil)
	if err != nil {
		return nil, err
	}
	var out []partidaEnCurso
	for _, g := range lista(m, "nowPlaying") {
		op := obj(g, "opponent")
		p := partidaEnCurso{ID: texto(g, "gameId"), Rival: texto(op, "username"), RivalElo: entero(op, "rating"),
			Maquina: entero(op, "ai"), Blancas: texto(g, "color") == "white", TeToca: verdad(g, "isMyTurn"),
			Segundos: entero(g, "secondsLeft"), Velocidad: texto(g, "speed"), FEN: texto(g, "fen")}
		if p.Maquina > 0 {
			p.Rival = fmt.Sprintf("Máquina, nivel %d", p.Maquina)
		}
		out = append(out, p)
	}
	return out, nil
}

// Siguiendo: la gente a la que sigues en Lichess (para retarla sin escribir su nombre).
func (li *lichess) Siguiendo() ([]string, error) {
	var out []string
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := li.stream(ctx, "GET", "/api/rel/following", nil, func(m map[string]any) {
		if u := texto(m, "username"); u != "" {
			out = append(out, u)
		}
	})
	return out, err
}

// ritmo: el tiempo de una partida.
type ritmo struct {
	Nombre     string
	Minutos    int // 0: por días
	Incremento int
	Dias       int
}

var ritmos = []ritmo{
	{Nombre: "5 + 3", Minutos: 5, Incremento: 3},
	{Nombre: "10 + 0", Minutos: 10},
	{Nombre: "15 + 10", Minutos: 15, Incremento: 10},
	{Nombre: "30 + 0", Minutos: 30},
	{Nombre: "1 día por jugada", Dias: 1},
	{Nombre: "3 días por jugada", Dias: 3},
}

func (r ritmo) form(f url.Values) {
	if r.Dias > 0 {
		f.Set("days", itoa(r.Dias))
		return
	}
	f.Set("clock.limit", itoa(r.Minutos*60))
	f.Set("clock.increment", itoa(r.Incremento))
}

// Retar manda un reto a alguien y espera a que conteste (el reto se mantiene mientras se espera:
// keepAliveStream). Devuelve el número de la partida si acepta.
func (li *lichess) Retar(ctx context.Context, usuario string, r ritmo, color string, puntua bool, creado func(id string)) (string, error) {
	f := url.Values{}
	r.form(f)
	f.Set("color", color)
	f.Set("rated", fmt.Sprint(puntua))
	f.Set("variant", "standard")
	if r.Dias == 0 {
		f.Set("keepAliveStream", "true")
	}
	if r.Dias > 0 { // por días no caduca: no hace falta mantenerlo
		m, err := li.pedir("POST", "/api/challenge/"+url.PathEscape(usuario), f)
		if err != nil {
			return "", err
		}
		id := texto(obj(m, "challenge"), "id")
		if id == "" {
			id = texto(m, "id")
		}
		creado(id)
		return "", nil // ya llegará por los avisos cuando acepte
	}
	var id, fin string
	err := li.stream(ctx, "POST", "/api/challenge/"+url.PathEscape(usuario), f, func(m map[string]any) {
		if c := obj(m, "challenge"); c != nil && id == "" {
			id = texto(c, "id")
			creado(id)
		} else if c := texto(m, "id"); c != "" && id == "" {
			id = c
			creado(id)
		}
		if d := texto(m, "done"); d != "" {
			fin = d
		}
	})
	if err != nil {
		return "", err
	}
	switch fin {
	case "accepted":
		return id, nil
	case "declined":
		return "", errors.New("no ha aceptado el reto")
	case "":
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", errors.New("el reto se ha cerrado sin respuesta")
	}
	return "", errors.New("el reto ha caducado")
}

func (li *lichess) CancelarReto(id string) {
	if id != "" {
		_, _ = li.pedir("POST", "/api/challenge/"+url.PathEscape(id)+"/cancel", url.Values{})
	}
}

// RetarMaquina: una partida contra la máquina de Lichess (Stockfish, del nivel 1 al 8).
func (li *lichess) RetarMaquina(nivel int, r ritmo, color string) (string, error) {
	f := url.Values{}
	r.form(f)
	f.Set("level", itoa(nivel))
	f.Set("color", color)
	m, err := li.pedir("POST", "/api/challenge/ai", f)
	if err != nil {
		return "", err
	}
	if id := texto(m, "id"); id != "" {
		return id, nil
	}
	return "", errors.New("Lichess no ha empezado la partida")
}

func (li *lichess) Aceptar(id string) error {
	_, err := li.pedir("POST", "/api/challenge/"+url.PathEscape(id)+"/accept", url.Values{})
	return err
}

func (li *lichess) Rechazar(id string) error {
	_, err := li.pedir("POST", "/api/challenge/"+url.PathEscape(id)+"/decline", url.Values{"reason": {"later"}})
	return err
}

func (li *lichess) Mover(partida, uci string) error {
	_, err := li.pedir("POST", "/api/board/game/"+url.PathEscape(partida)+"/move/"+url.PathEscape(uci), url.Values{})
	return err
}

func (li *lichess) Rendirse(partida string) error {
	_, err := li.pedir("POST", "/api/board/game/"+url.PathEscape(partida)+"/resign", url.Values{})
	return err
}

func (li *lichess) Abortar(partida string) error {
	_, err := li.pedir("POST", "/api/board/game/"+url.PathEscape(partida)+"/abort", url.Values{})
	return err
}

func (li *lichess) Tablas(partida string, si bool) error {
	que := "no"
	if si {
		que = "yes"
	}
	_, err := li.pedir("POST", "/api/board/game/"+url.PathEscape(partida)+"/draw/"+que, url.Values{})
	return err
}

// problema: un ejercicio de Lichess (la posición y la solución, en UCI).
type problema struct {
	ID       string
	Elo      int
	FEN      string
	Jugadas  string // las de la partida hasta el problema (si no hay FEN)
	Ultima   string // la última jugada del rival (para pintarla)
	Solucion []string
	Temas    []string
}

func (li *lichess) Problema(dificultad string) (problema, error) {
	ruta := "/api/puzzle/next"
	if dificultad != "" {
		ruta += "?difficulty=" + url.QueryEscape(dificultad)
	}
	m, err := li.pedir("GET", ruta, nil)
	if err != nil {
		return problema{}, err
	}
	pz := obj(m, "puzzle")
	p := problema{ID: texto(pz, "id"), Elo: entero(pz, "rating"), FEN: texto(pz, "fen"), Ultima: texto(pz, "lastMove"),
		Jugadas: texto(obj(m, "game"), "pgn")}
	for _, s := range lista(pz, "solution") {
		if v, ok := s.(string); ok {
			p.Solucion = append(p.Solucion, v)
		}
	}
	for _, s := range lista(pz, "themes") {
		if v, ok := s.(string); ok {
			p.Temas = append(p.Temas, v)
		}
	}
	if len(p.Solucion) == 0 {
		return p, errors.New("Lichess no ha dado el problema")
	}
	return p, nil
}

// MandarProblema: si lo has resuelto (para tu puntuación de problemas).
func (li *lichess) MandarProblema(id string, bien bool) {
	cuerpo := fmt.Sprintf(`{"solutions":[{"id":%q,"win":%v,"rated":true}]}`, id, bien)
	req, err := li.peticion(context.Background(), "POST", "/api/puzzle/batch/mix", nil)
	if err != nil {
		return
	}
	req.Body = io.NopCloser(strings.NewReader(cuerpo))
	req.ContentLength = int64(len(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := clienteDatos.Do(req); err == nil {
		logf("POST /api/puzzle/batch/mix: %d", resp.StatusCode)
		resp.Body.Close()
	}
}
