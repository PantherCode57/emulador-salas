package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Hablar con Duolingo como su web: www.duolingo.com/2017-06-30 con tu jwt_token en
// «Authorization: Bearer». Las peticiones son las que manda el navegador (las mismas que usa
// DuolingoCLI, un cliente de terminal libre en el que se hacen lecciones de verdad):
//   - perfil: GET users/<id>?fields=…
//   - camino: GET users/<id>?fields=currentCourse (pathSectioned: secciones, unidades y niveles)
//   - lección: POST sessions (sus ejercicios) y, al acabar, PUT sessions/<id> con la misma sesión
//     y tus respuestas, que es lo que suma los XP y la racha.
//
// En el registro solo se apunta adónde se llama y qué contesta (código y tamaño), nunca lo de dentro.
const (
	agenteNavegador = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
	urlWeb          = "https://www.duolingo.com"
	urlLigas        = "https://duolingo-leaderboards-prod.duolingo.com/leaderboards/7d9f5dd1-8423-491a-91f2-2532052038ce/users/"
)

var (
	clienteDatos = &http.Client{Timeout: 30 * time.Second}
	errBloqueo   = errors.New("Duolingo quiere comprobar que no eres un robot: abre duolingo.com en el navegador, haz una lección allí y vuelve a probar")
)

var experimentosCamino = []string{
	"BACKEND_REACTIVATION_REVIEW_NODE_ANDROID", "BACKEND_REACTIVATION_REVIEW_NODE_IOS",
	"BACKEND_RESURRECTION_REVIEW_NODE_ANDROID", "BACKEND_RESURRECTION_REVIEW_NODE_IOS",
}

var ligas = []string{"Bronce", "Plata", "Oro", "Zafiro", "Rubí", "Esmeralda", "Amatista", "Perla", "Obsidiana", "Diamante"}

type duolingo struct {
	cuenta  *cuenta
	mu      sync.Mutex
	info    map[string]any // el último perfil (para los idiomas y el curso)
	materia string         // si el curso de ahora no es de idiomas: «chess», «math» o «music»
}

// nombreMateria: el nombre del curso de ahora si no es de idiomas («Ajedrez»…), o "".
func (d *duolingo) nombreMateria() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch d.materia {
	case "chess":
		return "Ajedrez"
	case "math":
		return "Matemáticas"
	case "music":
		return "Música"
	}
	return ""
}

func nuevoDuolingo(c *cuenta) *duolingo { return &duolingo{cuenta: c} }

// pedir hace una llamada y deja la respuesta (JSON) en «fuera».
func (d *duolingo) pedir(metodo, ruta string, cuerpo any, fuera *map[string]any) error {
	codigo, datos, err := d.pedirCrudo(metodo, ruta, cuerpo)
	if err != nil {
		return err
	}
	switch {
	case codigo == http.StatusUnauthorized:
		return errSesion
	case codigo == http.StatusForbidden && bytes.Contains(datos, []byte("blockScript")):
		return errBloqueo
	case codigo == http.StatusTooManyRequests:
		return errLimite
	case codigo >= 400:
		return fmt.Errorf("Duolingo responde %d", codigo)
	}
	if fuera == nil || len(bytes.TrimSpace(datos)) == 0 {
		return nil
	}
	if err := leerJSON(datos, fuera); err != nil {
		return errors.New("Duolingo no ha contestado como siempre")
	}
	return nil
}

func leerJSON(datos []byte, fuera *map[string]any) error {
	dec := json.NewDecoder(bytes.NewReader(datos))
	dec.UseNumber()
	return dec.Decode(fuera)
}

// pedirCrudo hace la llamada y devuelve el código y lo que conteste, tal cual.
func (d *duolingo) pedirCrudo(metodo, ruta string, cuerpo any) (int, []byte, error) {
	token, _ := d.cuenta.datos()
	if token == "" {
		return 0, nil, errSesion
	}
	dir := ruta
	if !strings.HasPrefix(ruta, "http") {
		dir = urlWeb + ruta
	}
	var body io.Reader
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, dir, body)
	if err != nil {
		return 0, nil, err
	}
	h := req.Header
	h.Set("Authorization", "Bearer "+token)
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Accept-Language", "es-ES,es;q=0.9,en;q=0.8")
	h.Set("User-Agent", agenteNavegador)
	if strings.HasPrefix(dir, urlWeb) {
		h.Set("Origin", urlWeb)
		h.Set("Referer", urlWeb+"/learn")
		h.Set("Sec-Fetch-Dest", "empty")
		h.Set("Sec-Fetch-Mode", "cors")
		h.Set("Sec-Fetch-Site", "same-origin")
	}
	if cuerpo != nil {
		h.Set("Content-Type", "application/json")
	}
	resp, err := clienteDatos.Do(req)
	if err != nil {
		logf("%s %s: %v", metodo, sinQuery(ruta), err)
		return 0, nil, err
	}
	defer resp.Body.Close()
	datos, _ := io.ReadAll(io.LimitReader(resp.Body, 48<<20))
	logf("%s %s: %d (%d bytes)", metodo, sinQuery(ruta), resp.StatusCode, len(datos))
	return resp.StatusCode, datos, nil
}

func sinQuery(s string) string {
	s, _, _ = strings.Cut(s, "?")
	if i := strings.Index(s, "/users/"); i >= 0 { // sin el número de tu cuenta
		resto := s[i+7:]
		_, despues, _ := strings.Cut(resto, "/")
		s = s[:i+7] + "…"
		if despues != "" {
			s += "/" + despues
		}
	}
	return s
}

// ponerCuenta comprueba el token con Duolingo (que diga de quién es) y lo guarda.
func (d *duolingo) ponerCuenta(pegado string) error {
	token, id, err := sacarToken(pegado)
	if err != nil {
		return err
	}
	prueba := &duolingo{cuenta: &cuenta{Token: token, ID: id}}
	var r map[string]any
	if err := prueba.pedir("GET", fmt.Sprintf("/2017-06-30/users/%d?fields=id,username,name", id), nil, &r); err != nil {
		if errors.Is(err, errSesion) {
			return errors.New("Duolingo dice que esa sesión no vale (¿cerraste sesión en ese navegador?). Entra otra vez y copia el jwt_token nuevo")
		}
		return err
	}
	c := d.cuenta
	c.mu.Lock()
	c.Token, c.ID, c.Usuario, c.Nombre = token, id, texto(r, "username"), texto(r, "name")
	c.mu.Unlock()
	c.guardar()
	logf("cuenta de Duolingo guardada")
	return nil
}

const camposPerfil = "id,username,name,streak,streakData,totalXp,xpGoal,xpGains,gems,gemsConfig,lingots,health,hasPlus,timezone,courses,currentCourseId,fromLanguage,learningLanguage"

func (d *duolingo) Perfil() (perfil, error) {
	_, id := d.cuenta.datos()
	var r map[string]any
	if err := d.pedir("GET", fmt.Sprintf("/2017-06-30/users/%d?fields=%s", id, camposPerfil), nil, &r); err != nil {
		return perfil{}, err
	}
	d.mu.Lock()
	d.info = r
	d.mu.Unlock()
	p := perfil{Usuario: texto(r, "username"), Nombre: texto(r, "name"), XPTotal: entero(r, "totalXp"),
		Meta: entero(r, "xpGoal"), Desde: texto(r, "fromLanguage"), Aprende: texto(r, "learningLanguage"),
		CursoID: texto(r, "currentCourseId")}
	p.Racha = entero(r, "streak")
	actual := obj(obj(r, "streakData"), "currentStreak")
	if p.Racha == 0 {
		p.Racha = entero(actual, "length")
	}
	hoy := time.Now().Format("2006-01-02")
	p.RachaHoy = texto(actual, "lastExtendedDate") == hoy || texto(actual, "endDate") == hoy
	if g, ok := numero(r, "gems"); ok {
		p.Gemas = g
	} else {
		p.Gemas = entero(obj(r, "gemsConfig"), "gems")
	}
	h := obj(r, "health")
	p.Vidas = -1
	if h != nil && verdad(h, "healthEnabled") && verdad(h, "useHealth") && !verdad(h, "unlimitedHeartsAvailable") && !verdad(r, "hasPlus") {
		p.Vidas = entero(h, "hearts")
	}
	y, m, dd := time.Now().Date()
	medianoche := time.Date(y, m, dd, 0, 0, 0, 0, time.Local).Unix()
	for _, g := range lista(r, "xpGains") {
		if t, ok := numero(g, "time"); ok && int64(t) >= medianoche {
			p.XPHoy += entero(g, "xp")
		}
	}
	if p.XPHoy > 0 {
		p.RachaHoy = true
	}
	p.Curso = idiomas[p.Aprende]
	if p.Curso == "" {
		for _, c := range lista(r, "courses") {
			if texto(c, "id") == p.CursoID {
				p.Curso = texto(c, "title")
			}
		}
	}
	p.Liga = d.liga(id)
	return p, nil
}

// liga: en qué liga estás (si no contesta enseguida, se deja).
func (d *duolingo) liga(id int64) string {
	var r map[string]any
	if err := d.pedir("GET", fmt.Sprintf("%s%d?client_unlocked=true", urlLigas, id), nil, &r); err != nil {
		return ""
	}
	t, ok := numero(r, "tier")
	if !ok {
		t, ok = numero(obj(obj(r, "active"), "cohort"), "tier")
	}
	if !ok || t < 0 || t >= len(ligas) {
		return ""
	}
	return ligas[t]
}

// Camino: las unidades de tu curso con sus niveles, y en cuál vas.
func (d *duolingo) Camino() ([]unidad, int, error) {
	_, id := d.cuenta.datos()
	var r map[string]any
	if err := d.pedir("GET", fmt.Sprintf("/2017-06-30/users/%d?fields=currentCourse", id), nil, &r); err != nil {
		return nil, 0, err
	}
	curso := obj(r, "currentCourse")
	if curso == nil {
		// los cursos que no son de idiomas (ajedrez, matemáticas, música) solo salen en la API nueva
		var r2 map[string]any
		if err := d.pedir("GET", fmt.Sprintf("/2023-05-23/users/%d?fields=currentCourseId,currentCourse,courses", id), nil, &r2); err != nil {
			return nil, 0, err
		}
		curso = obj(r2, "currentCourse")
		actualID := texto(r2, "currentCourseId")
		for _, c := range lista(r2, "courses") {
			if texto(c, "id") == actualID || (actualID == "" && texto(c, "id") == texto(curso, "id")) {
				d.mu.Lock()
				d.materia = texto(c, "subject")
				d.mu.Unlock()
			}
		}
	}
	if curso == nil {
		return nil, 0, errors.New("no tienes ningún curso empezado")
	}
	cursoID := texto(curso, "id")
	arbol := texto(curso, "treeId")
	var us []unidad
	actual := -1
	for _, s := range lista(curso, "pathSectioned") {
		if t := texto(s, "type"); t != "" && t != "learning" {
			continue
		}
		sec := entero(s, "index") + 1
		for ui, u := range lista(s, "units") {
			un := unidad{Indice: len(us), Seccion: fmt.Sprintf("Sección %d", sec), Titulo: fmt.Sprintf("Unidad %d", ui+1),
				Tema: texto(u, "teachingObjective")}
			for li, lv := range lista(u, "levels") {
				n := nivelDe(lv, li, un.Tema, arbol, cursoID)
				if n.Estado == estActivo && actual < 0 {
					actual = len(us)
				}
				un.Niveles = append(un.Niveles, n)
			}
			if len(un.Niveles) > 0 {
				us = append(us, un)
			}
		}
	}
	if len(us) == 0 {
		return nil, 0, errors.New("tu curso no tiene camino (¿es de los nuevos de matemáticas o música?)")
	}
	if actual < 0 { // todo hecho o nada empezado: la primera que no esté hecha
		actual = len(us) - 1
		for i, u := range us {
			if u.Niveles[len(u.Niveles)-1].Estado != estHecho {
				actual = i
				break
			}
		}
	}
	return us, actual, nil
}

// nivelDe: un nivel del camino y lo que hay que mandar para empezarlo (como DuolingoCLI, que
// lo copia del navegador).
func nivelDe(lv any, indice int, temaUnidad, arbolCurso, cursoID string) nivel {
	meta := obj(lv, "pathLevelMetadata")
	cd := obj(lv, "pathLevelClientData")
	tipo := texto(lv, "type")
	n := nivel{ID: texto(lv, "id"), Tipo: tipo, Hechas: entero(lv, "finishedSessions"), Total: entero(lv, "totalSessions")}
	if n.Total <= 0 {
		n.Total = 1
	}
	switch texto(lv, "state") {
	case "passed", "legendary":
		n.Estado = estHecho
	case "active", "accessible", "unit_test":
		n.Estado = estActivo
	default:
		n.Estado = estBloqueado
	}
	switch tipo {
	case "story":
		n.Nombre = "Historia"
	case "chest":
		n.Nombre = "Cofre"
	case "unit_review":
		n.Nombre = "Repaso de la unidad"
	case "practice":
		n.Nombre = "Práctica"
	case "alphabet":
		n.Nombre = "Alfabeto"
	case "unit_test", "test":
		n.Nombre = "Prueba de la unidad"
	default:
		n.Nombre = fmt.Sprintf("Nivel %d", indice+1)
		if o := texto(cd, "teachingObjective"); o != "" && o != temaUnidad {
			n.Nombre += " · " + o
		}
	}
	switch {
	case n.Estado == estHecho && (tipo == "skill" || tipo == "practice" || tipo == "unit_review"):
		n.Peticion = peticion{tipo: "practica"} // lo ya hecho se repasa con una práctica
		return n
	case n.Estado != estActivo || tipo == "story" || tipo == "chest" || tipo == "alphabet":
		return n
	}

	skill := texto(meta, "skillId")
	if skill == "" {
		skill = texto(cd, "skillId")
	}
	skills := textos(cd, "skillIds")
	espaciadas := textos(cd, "spacedRepetitionSkillIds")
	final := n.Hechas >= n.Total-1
	repasoNivel := verdad(lv, "hasLevelReview")
	arbol := texto(meta, "treeId")
	if arbol == "" {
		arbol = arbolCurso
	}
	if arbol == "" {
		arbol = cursoID
	}
	tipoSesion := "LESSON"
	var elegidas []string
	lexema := ""
	switch tipo {
	case "practice":
		switch {
		case texto(lv, "subtype") == "unit_practice":
			tipoSesion, elegidas = "UNIT_PRACTICE", skills
		case texto(cd, "practiceType") == "SPACED_REPETITION" && len(espaciadas) > 0:
			tipoSesion, elegidas = "SPACED_REPETITION", espaciadas
		default:
			tipoSesion, elegidas = "LEXEME_PRACTICE", skills
		}
		lexema = "practice_level"
		if final && repasoNivel {
			lexema = "practice_level_review"
		}
	case "unit_review":
		tipoSesion, elegidas = "UNIT_REVIEW", skills
	case "skill":
		if final && repasoNivel {
			tipoSesion = "LEVEL_REVIEW"
		}
	}
	if len(elegidas) == 0 {
		elegidas = skills
	}
	sesionMeta := obj(lv, "pathLevelSessionMetadata")
	if sesionMeta == nil {
		sesionMeta = map[string]any{}
	}
	c := map[string]any{
		"type": tipoSesion, "levelId": n.ID, "levelIndex": entero(meta, "crownLevelIndex"),
		"levelSessionIndex": n.Hechas, "isFinalLevel": final, "pathLevelSessionMetadata": sesionMeta,
	}
	if skill != "" {
		c["skillId"] = skill
	}
	if len(elegidas) > 0 {
		c["skillIds"] = elegidas
	}
	if lexema != "" {
		c["lexemePracticeType"] = lexema
	}
	if tipoSesion != "UNIT_REVIEW" { // con treeId, Duolingo da 400 en los repasos de unidad
		c["treeId"] = arbol
	}
	// lo que va en pathLevelSpecifics al acabar (sin lessonNumber el camino no avanza)
	esp := map[string]any{}
	for k, v := range meta {
		esp[k] = v
	}
	if _, ok := esp["nodeState"]; !ok {
		esp["nodeState"] = texto(lv, "state")
	}
	if _, ok := esp["treeId"]; !ok && tipo != "unit_review" {
		esp["treeId"] = arbol
	}
	if v, ok := esp["lessonNumber"]; (!ok || v == nil) && esp["skillId"] != nil {
		esp["lessonNumber"] = n.Hechas + 1
	}
	if final {
		esp["nodeState"] = "passed"
	}
	n.Peticion = peticion{tipo: "nivel", cuerpo: c, especificos: esp, final: final}
	return n
}

// Empezar pide a Duolingo los ejercicios de una lección y los pasa a los de la consola.
func (d *duolingo) Empezar(p peticion, pf perfil) (*leccion, error) {
	desde, aprende := pf.Desde, pf.Aprende
	if desde == "" || aprende == "" {
		d.mu.Lock()
		desde, aprende = texto(d.info, "fromLanguage"), texto(d.info, "learningLanguage")
		d.mu.Unlock()
	}
	if desde == "" || aprende == "" {
		return nil, errors.New("no se sabe tu curso: vuelve al camino y espera a que cargue")
	}
	cuerpo := map[string]any{
		"challengeTypes": tiposPedidos, "fromLanguage": desde, "learningLanguage": aprende,
		"isCustomIntroSkill": false, "isFinalLevel": false, "isGrammarSkill": false, "isRedoingPassedNode": false,
		"isV2": true, "juicy": true, "pathExperiments": experimentosCamino, "pathLevelSessionMetadata": map[string]any{},
		"shakeToReportEnabled": true, "showGrammarSkillSplash": false, "smartTipsVersion": 2, "type": "GLOBAL_PRACTICE",
	}
	for k, v := range p.cuerpo {
		cuerpo[k] = v
	}
	if p.tipo != "practica" && pf.Vidas == 0 {
		return nil, errors.New("no te quedan vidas. Practica (X en el camino) para recuperar una o espera a que se recarguen")
	}
	// En los cursos que no son de idiomas (el ajedrez) puede que haga falta decir el curso: si
	// Duolingo dice que no (400), se prueba con «courseId» y luego sin «learningLanguage».
	intentos := []map[string]any{cuerpo}
	d.mu.Lock()
	materia, cursoID := d.materia, texto(d.info, "currentCourseId")
	d.mu.Unlock()
	if materia != "" && cursoID != "" {
		intentos = append(intentos, conCampos(cuerpo, map[string]any{"courseId": cursoID}),
			conCampos(cuerpo, map[string]any{"courseId": cursoID, "learningLanguage": nil}))
	}
	var s map[string]any
	var err error
	for i, c := range intentos {
		s = nil
		if err = d.pedir("POST", "/2017-06-30/sessions", c, &s); err == nil {
			if i > 0 {
				logf("la lección ha salido con la forma %d", i+1)
			}
			break
		}
		if errors.Is(err, errSesion) || errors.Is(err, errBloqueo) || errors.Is(err, errLimite) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	if texto(s, "id") == "" {
		return nil, errors.New("Duolingo no ha dado la lección")
	}
	// como el navegador: la sesión que se devuelve al acabar lleva también de qué nivel es
	for _, k := range []string{"isFinalLevel", "levelId", "levelIndex", "levelSessionIndex", "pathLevelSessionMetadata", "skillId", "skillIds"} {
		if v, ok := p.cuerpo[k]; ok {
			s[k] = v
		}
	}
	var ejs []*ejercicio
	saltados := 0
	for i, c := range lista(s, "challenges") {
		m, _ := c.(map[string]any)
		if e := convertir(m, i, pf); e != nil {
			ejs = append(ejs, e)
		} else {
			saltados++
			logf("ejercicio que no sé enseñar: %s", texto(m, "type"))
		}
	}
	if len(ejs) == 0 {
		return nil, errors.New("esta lección no tiene ejercicios que se puedan hacer en la consola")
	}
	vidas := pf.Vidas
	if p.tipo == "practica" {
		vidas = -1 // practicando no se pierden vidas
	}
	l := nuevaLeccion(s, ejs, vidas, "")
	l.saltados, l.pet = saltados, p
	return l, nil
}

// Terminar manda la lección con tus respuestas (es lo que suma los XP y la racha).
func (d *duolingo) Terminar(l *leccion) (resultadoFin, error) {
	fin := time.Now()
	if fin.Sub(l.inicio) < time.Second {
		fin = l.inicio.Add(time.Second)
	}
	s := map[string]any{}
	for k, v := range l.crudo {
		s[k] = v
	}
	retos := lista(l.crudo, "challenges")
	nuevos := make([]any, len(retos))
	convertidos := map[int]bool{}
	for _, e := range l.ejs {
		convertidos[e.Indice] = true
	}
	for i, c := range retos {
		m, _ := c.(map[string]any)
		cp := map[string]any{}
		for k, v := range m {
			cp[k] = v
		}
		switch r, ok := l.respuestas[i]; {
		case ok:
			cp["correct"], cp["guess"], cp["timeTaken"] = r.Bien, r.Guess, r.Ms
			cp["numHintsTapped"], cp["wasIndicatorShown"], cp["highlights"] = 0, false, []any{}
		case !convertidos[i]: // los que la consola no enseña (como «No puedo escuchar ahora»)
			cp["correct"], cp["skipped"], cp["timeTaken"] = true, true, 0
		default: // sin contestar (te quedaste sin vidas)
			cp["correct"], cp["timeTaken"] = false, 0
		}
		nuevos[i] = cp
	}
	s["challenges"] = nuevos
	vidas := l.vidas
	if vidas < 0 {
		vidas = 0
	}
	d.mu.Lock()
	curso := texto(d.info, "currentCourseId")
	d.mu.Unlock()
	s["heartsLeft"] = vidas
	s["startTime"] = l.inicio.Unix()
	s["endTime"] = fin.Unix()
	s["enableBonusPoints"] = true
	s["failed"] = l.sinVidas()
	s["maxInLessonStreak"] = l.maxSeguidos
	s["shouldLearnThings"] = true
	s["dailyRefreshInfo"] = nil
	if curso != "" {
		s["courseId"] = curso
	}
	if l.pet.final {
		s["isFinalLevel"] = true
	}
	if l.pet.especificos != nil {
		s["pathLevelSpecifics"] = l.pet.especificos
	}
	var r map[string]any
	if err := d.pedir("PUT", "/2017-06-30/sessions/"+texto(l.crudo, "id"), s, &r); err != nil {
		return resultadoFin{Hora: fin}, err
	}
	res := resultadoFin{Hora: fin}
	if x, ok := numero(r, "xpGain"); ok {
		res.XP = x
	} else {
		res.XP = entero(r, "awardedXp")
	}
	if pf, err := d.Perfil(); err == nil {
		res.Racha = pf.Racha
	}
	logf("lección mandada: %d XP", res.XP)
	return res, nil
}
