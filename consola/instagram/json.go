package main

import (
	"encoding/json"
	"strconv"
)

// Ayudas para leer las respuestas de Instagram sin declarar todos sus tipos: ir(v, "a",
// "b", 0, "c") baja por claves y posiciones y devuelve nil si algo no está.

func ir(v any, camino ...any) any {
	for _, p := range camino {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[k]
		case int:
			l, ok := v.([]any)
			if !ok {
				return nil
			}
			if k < 0 {
				k += len(l)
			}
			if k < 0 || k >= len(l) {
				return nil
			}
			v = l[k]
		}
		if v == nil {
			return nil
		}
	}
	return v
}

func txt(v any, camino ...any) string {
	switch x := ir(v, camino...).(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return ""
}

func num(v any, camino ...any) float64 {
	switch x := ir(v, camino...).(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func lista(v any, camino ...any) []any {
	l, _ := ir(v, camino...).([]any)
	return l
}

func ultimo(s, sep string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i:i+1] == sep {
			return s[i+1:]
		}
	}
	return s
}
