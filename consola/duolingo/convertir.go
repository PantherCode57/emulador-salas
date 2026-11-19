package main

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Lo que manda Duolingo, en JSON sin tipos (map[string]any, con los números como json.Number para
// devolver la sesión tal cual). Estas funciones sacan lo que haga falta sin romperse si algo falta.

func obj(v any, k string) map[string]any {
	if m, ok := v.(map[string]any); ok {
		if o, ok := m[k].(map[string]any); ok {
			return o
		}
	}
	return nil
}

func lista(v any, k string) []any {
	if m, ok := v.(map[string]any); ok {
		if a, ok := m[k].([]any); ok {
			return a
		}
	}
	return nil
}

func texto(v any, k string) string {
	if m, ok := v.(map[string]any); ok {
		switch x := m[k].(type) {
		case string:
			return x
		case json.Number:
			return x.String()
		}
	}
	return ""
}

func numero(v any, k string) (int, bool) {
	if m, ok := v.(map[string]any); ok {
		switch x := m[k].(type) {
		case json.Number:
			if n, err := x.Int64(); err == nil {
				return int(n), true
			}
			if f, err := x.Float64(); err == nil {
				return int(f), true
			}
		case float64:
			return int(x), true
		case string:
			if n, err := strconv.Atoi(x); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

func entero(v any, k string) int {
	n, _ := numero(v, k)
	return n
}

func verdad(v any, k string) bool {
	if m, ok := v.(map[string]any); ok {
		b, _ := m[k].(bool)
		return b
	}
	return false
}

func enteros(v any, k string) []int {
	var out []int
	for _, x := range lista(v, k) {
		switch n := x.(type) {
		case json.Number:
			if i, err := n.Int64(); err == nil {
				out = append(out, int(i))
			}
		case float64:
			out = append(out, int(n))
		}
	}
	return out
}

func textos(v any, k string) []string {
	var out []string
	for _, x := range lista(v, k) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// opcionesDe: las opciones de un ejercicio; pueden venir como texto o como objetos con «text»,
// «phrase» o «character» (y su voz en «tts»).
func opcionesDe(c map[string]any, k string) []opcion {
	var out []opcion
	for _, x := range lista(c, k) {
		switch o := x.(type) {
		case string:
			out = append(out, opcion{Texto: o})
		case map[string]any:
			t := texto(o, "text")
			if t == "" {
				t = texto(o, "phrase")
			}
			if t == "" {
				t = texto(o, "character")
			}
			if t == "" {
				t = texto(o, "value")
			}
			out = append(out, opcion{Texto: t, Audio: texto(o, "tts")})
		}
	}
	return out
}

// solucionesDe: todo lo que vale como respuesta (correctSolutions, compactTranslations, el grafo del
// «grader» y correctTokens).
func solucionesDe(c map[string]any) (todas []string, mostrar string) {
	add := func(s string) {
		if strings.TrimSpace(s) != "" {
			todas = append(todas, s)
		}
	}
	for _, s := range textos(c, "correctSolutions") {
		add(s)
	}
	for _, g := range lista(c, "compactTranslations") {
		switch x := g.(type) {
		case string:
			add(x)
		case []any:
			for _, y := range x {
				if s, ok := y.(string); ok {
					add(s)
				}
			}
		}
	}
	for _, s := range textos(c, "correctAnswers") {
		add(s)
	}
	if len(todas) > 0 {
		mostrar = expandir(todas[0])[0] // «I [am/'m] a boy» → «I am a boy»
	}
	for _, s := range caminosGrader(obj(c, "grader"), 300) {
		add(s)
		if mostrar == "" {
			mostrar = s
		}
	}
	if ts := textos(c, "correctTokens"); len(ts) > 0 {
		s := unirFichas(ts)
		add(s)
		if mostrar == "" {
			mostrar = s
		}
	}
	return todas, mostrar
}

// caminosGrader: el «grader» de Duolingo es un grafo del vértice 0 al último; cada camino es una
// respuesta que vale (las aristas «typo» son fallos que se perdonan: no se cuentan aquí).
func caminosGrader(g map[string]any, maximo int) []string {
	vs := lista(g, "vertices")
	if len(vs) == 0 {
		return nil
	}
	fin := len(vs) - 1
	var out []string
	var ir func(u int, acc string)
	ir = func(u int, acc string) {
		if len(out) >= maximo {
			return
		}
		if u == fin {
			if s := strings.TrimSpace(acc); s != "" {
				out = append(out, s)
			}
			return
		}
		aristas, _ := vs[u].([]any)
		for _, a := range aristas {
			if texto(a, "type") == "typo" {
				continue
			}
			to, ok := numero(a, "to")
			if !ok || to <= u || to > fin {
				continue
			}
			t := texto(a, "orig")
			if t == "" {
				t = texto(a, "lenient")
			}
			ir(to, acc+t)
		}
	}
	ir(0, "")
	return out
}

// piezasDe: la frase con huecos (displayTokens con isBlank). Devuelve también lo que va en los huecos.
func piezasDe(c map[string]any) ([]pieza, []string) {
	var ps []pieza
	var huecos []string
	toks := lista(c, "displayTokens")
	conEspacios := false
	blanco := make([]bool, len(toks))
	for i, t := range toks {
		if strings.TrimSpace(texto(t, "text")) == "" && texto(t, "text") != "" {
			conEspacios = true
		}
		blanco[i] = verdad(t, "isBlank")
	}
	// los espacios entre dos trozos de hueco son del hueco (un hueco de varias palabras)
	for i := 1; i+1 < len(toks); i++ {
		if !blanco[i] && strings.TrimSpace(texto(toks[i], "text")) == "" && blanco[i-1] && blanco[i+1] {
			blanco[i] = true
		}
	}
	for i, t := range toks {
		s := texto(t, "text")
		if blanco[i] && i > 0 && blanco[i-1] && len(huecos) > 0 { // sigue el mismo hueco
			if conEspacios {
				huecos[len(huecos)-1] += s
			} else {
				huecos[len(huecos)-1] += " " + s
			}
			continue
		}
		if !conEspacios && i > 0 && !strings.ContainsAny(s[:min(1, len(s))], ".,!?;:") {
			if n := len(ps); n > 0 && !ps[n-1].Hueco {
				ps[n-1].Texto += " "
			} else {
				ps = append(ps, pieza{Texto: " "})
			}
		}
		if blanco[i] {
			ps = append(ps, pieza{Hueco: true})
			huecos = append(huecos, s)
			continue
		}
		if n := len(ps); n > 0 && !ps[n-1].Hueco {
			ps[n-1].Texto += s
		} else {
			ps = append(ps, pieza{Texto: s})
		}
	}
	for i := range huecos {
		huecos[i] = strings.Join(strings.Fields(huecos[i]), " ")
	}
	return ps, huecos
}

// barajarOpciones cambia de orden las fichas (Duolingo las manda con las buenas primero) y dice dónde
// ha ido a parar cada una.
func barajarOpciones(os []opcion) ([]opcion, []int) {
	orden := barajar(len(os))
	nuevas := make([]opcion, len(os))
	donde := make([]int, len(os))
	for nuevo, viejo := range orden {
		nuevas[nuevo] = os[viejo]
		donde[viejo] = nuevo
	}
	return nuevas, donde
}

// Instrucciones en español, según el tipo.
var instrucciones = map[string]string{
	"select":                     "Elige la imagen correcta",
	"characterSelect":            "Elige el carácter correcto",
	"selectTranscription":        "¿Qué has oído?",
	"selectPronunciation":        "¿Cuál suena así?",
	"assist":                     "Elige el significado correcto",
	"reverseAssist":              "Elige la traducción correcta",
	"judge":                      "Elige la traducción correcta",
	"form":                       "Elige la opción correcta",
	"definition":                 "Elige el significado correcto",
	"gapFill":                    "Elige la palabra que falta",
	"listenIsolation":            "¿Qué palabra has oído?",
	"readComprehension":          "Lee y contesta",
	"listenComprehension":        "Escucha y contesta",
	"dialogue":                   "Elige la respuesta correcta",
	"translate":                  "Traduce esta frase",
	"listenTap":                  "Escribe lo que oyes",
	"listen":                     "Escribe lo que oyes",
	"tapComplete":                "Completa la frase",
	"tapCloze":                   "Completa la palabra",
	"match":                      "Empareja las palabras",
	"listenMatch":                "Empareja lo que oyes",
	"characterMatch":             "Empareja los caracteres",
	"extendedMatch":              "Empareja las palabras",
	"extendedListenMatch":        "Empareja lo que oyes",
	"name":                       "Escribe esto en el idioma que aprendes",
	"listenComplete":             "Escribe la palabra que falta",
	"completeReverseTranslation": "Completa la traducción",
	"partialReverseTranslate":    "Completa la traducción",
	"typeCloze":                  "Completa la palabra",
	"typeComplete":               "Completa la frase",
}

// tiposPedidos: los ejercicios que la consola sabe enseñar (no se piden los de hablar: no hay
// micrófono). Duolingo solo manda los que se le piden.
var tiposPedidos = []string{
	"assist", "characterMatch", "characterSelect", "completeReverseTranslation", "definition", "dialogue",
	"extendedMatch", "extendedListenMatch", "form", "gapFill", "judge", "listen", "listenComplete",
	"listenMatch", "match", "name", "listenComprehension", "listenIsolation", "listenTap",
	"partialReverseTranslate", "readComprehension", "reverseAssist", "select", "selectPronunciation",
	"selectTranscription", "tapCloze", "tapComplete", "translate", "typeCloze", "typeComplete",
}

// convertir pasa un ejercicio de Duolingo a uno de la consola (nil si no se sabe enseñar).
func convertir(c map[string]any, indice int, pf perfil) *ejercicio {
	tipo := texto(c, "type")
	e := &ejercicio{Indice: indice, Clase: tipo, Instruccion: instrucciones[tipo], Audio: texto(c, "tts"),
		AudioLento: texto(c, "slowTts"), Idioma: pf.Aprende}
	if e.Instruccion == "" {
		e.Instruccion = "Contesta"
	}
	if t := texto(c, "targetLanguage"); t != "" {
		e.Idioma = t
	}
	prompt := texto(c, "prompt")
	switch tipo {
	case "select", "characterSelect", "selectTranscription", "selectPronunciation", "assist", "reverseAssist",
		"judge", "form", "definition", "readComprehension", "listenComprehension", "dialogue", "gapFill":
		e.Tipo = ejElegir
		e.Opciones = opcionesDe(c, "choices")
		e.Correcta = -1
		if n, ok := numero(c, "correctIndex"); ok {
			e.Correcta = n
		} else if is := enteros(c, "correctIndices"); len(is) > 0 {
			e.Correcta = is[0]
		}
		e.Frase = prompt
		switch tipo {
		case "select":
			e.Instruccion = "¿Cuál es «" + prompt + "»?"
			e.Frase = ""
		case "readComprehension":
			e.Frase = strings.TrimSpace(texto(c, "passage") + " " + texto(c, "question"))
		case "dialogue":
			var b []string
			for _, d := range lista(c, "dialogue") {
				var ps []string
				for _, t := range lista(d, "displayTokens") {
					ps = append(ps, texto(t, "text"))
				}
				b = append(b, strings.Join(ps, ""))
			}
			e.Frase = strings.Join(b, " — ")
		case "form":
			if pp := textos(c, "promptPieces"); len(pp) > 0 {
				e.Frase = strings.Join(pp, " ____ ")
			}
		case "gapFill":
			e.Piezas, _ = piezasDe(c)
		case "selectTranscription", "listenComprehension":
			e.SoloOir = e.Audio != "" && tipo == "selectTranscription"
		}
		if e.Correcta < 0 || e.Correcta >= len(e.Opciones) {
			return nil
		}
	case "listenIsolation":
		e.Tipo = ejElegir
		e.Opciones = opcionesDe(c, "options")
		e.Correcta = entero(c, "correctIndex")
		var ps []string
		for i, t := range lista(c, "tokens") {
			if i >= entero(c, "blankRangeStart") && i < entero(c, "blankRangeEnd") {
				ps = append(ps, "____")
			} else {
				ps = append(ps, texto(t, "value"))
			}
		}
		e.Frase = strings.Join(ps, "")
		if e.Correcta < 0 || e.Correcta >= len(e.Opciones) {
			return nil
		}
	case "translate", "listenTap":
		ops := opcionesDe(c, "choices")
		e.Soluciones, e.Mostrar = solucionesDe(c)
		if tipo == "translate" {
			e.Frase = prompt
			if e.Idioma == pf.Aprende && pf.Aprende != "" {
				e.Instruccion = "Escribe esto en " + nombreIdioma(pf.Aprende)
			}
		} else {
			e.SoloOir = e.Audio != ""
			if prompt != "" {
				e.Soluciones = append(e.Soluciones, prompt)
				if e.Mostrar == "" {
					e.Mostrar = prompt
				}
			}
			if !e.SoloOir {
				e.Frase = texto(c, "solutionTranslation")
			}
		}
		if is := enteros(c, "correctIndices"); len(is) > 0 && len(ops) > 0 {
			var ps []string
			for _, i := range is {
				if i >= 0 && i < len(ops) {
					ps = append(ps, ops[i].Texto)
				}
			}
			e.Soluciones = append(e.Soluciones, unirFichas(ps))
			if e.Mostrar == "" {
				e.Mostrar = unirFichas(ps)
			}
		}
		if len(ops) > 0 {
			e.Tipo = ejBanco
			e.Opciones, _ = barajarOpciones(ops)
		} else {
			e.Tipo = ejEscribir
		}
		if len(e.Soluciones) == 0 {
			return nil
		}
	case "listen":
		e.Tipo, e.SoloOir = ejEscribir, true
		e.Soluciones, e.Mostrar = solucionesDe(c)
		if prompt != "" {
			e.Soluciones = append(e.Soluciones, prompt)
			e.Mostrar = prompt
		}
		if len(e.Soluciones) == 0 || e.Audio == "" {
			return nil
		}
	case "name":
		e.Tipo = ejEscribir
		e.Frase = prompt
		e.Soluciones, e.Mostrar = solucionesDe(c)
		if pf.Aprende != "" {
			e.Instruccion = "Escribe «" + prompt + "» en " + nombreIdioma(pf.Aprende)
			e.Frase = ""
		}
		if len(e.Soluciones) == 0 {
			return nil
		}
	case "tapComplete", "tapCloze":
		ops := opcionesDe(c, "choices")
		ps, _ := piezasDe(c)
		correctas := enteros(c, "correctIndices")
		if len(correctas) == 0 {
			if n, ok := numero(c, "correctIndex"); ok {
				correctas = []int{n}
			}
		}
		huecos := 0
		for _, p := range ps {
			if p.Hueco {
				huecos++
			}
		}
		if len(ops) == 0 || huecos == 0 || len(correctas) != huecos {
			return nil
		}
		nuevas, donde := barajarOpciones(ops)
		e.Tipo, e.Opciones, e.Piezas = ejHuecos, nuevas, ps
		for _, i := range correctas {
			if i < 0 || i >= len(donde) {
				return nil
			}
			e.Correctas = append(e.Correctas, donde[i])
		}
		if tr := texto(c, "solutionTranslation"); tr != "" {
			e.Frase = tr
		}
	case "listenComplete", "completeReverseTranslation", "partialReverseTranslate", "typeCloze", "typeComplete":
		ps, huecos := piezasDe(c)
		if len(huecos) == 0 {
			return nil
		}
		e.Tipo, e.Piezas = ejEscribir, ps
		sol := strings.Join(huecos, " ")
		e.Soluciones, e.Mostrar = []string{sol}, sol
		var b strings.Builder
		for _, p := range ps {
			if p.Hueco {
				b.WriteString("____")
			} else {
				b.WriteString(p.Texto)
			}
		}
		e.Frase = b.String()
		if tipo != "listenComplete" && prompt != "" {
			e.Frase = prompt + "  →  " + e.Frase
		}
		e.Piezas = nil // el hueco ya va en la frase
	case "match", "listenMatch", "characterMatch", "extendedMatch", "extendedListenMatch":
		e.Tipo = ejParejas
		for _, p := range lista(c, "pairs") {
			apr := texto(p, "learningToken")
			tu := texto(p, "fromToken")
			if apr == "" {
				apr = texto(p, "learningWord")
			}
			if tu == "" {
				tu = texto(p, "translation")
			}
			if apr == "" {
				apr = texto(p, "character")
			}
			if tu == "" {
				tu = texto(p, "transliteration")
			}
			if apr == "" || tu == "" {
				continue
			}
			e.Izquierda = append(e.Izquierda, opcion{Texto: apr, Audio: texto(p, "tts")})
			e.Derecha = append(e.Derecha, opcion{Texto: tu})
		}
		if len(e.Izquierda) < 2 {
			return nil
		}
		if len(e.Izquierda) > 6 { // no caben más en la pantalla
			e.Izquierda, e.Derecha = e.Izquierda[:6], e.Derecha[:6]
		}
	default:
		return nil
	}
	return e
}
