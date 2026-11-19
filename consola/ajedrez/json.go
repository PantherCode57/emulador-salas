package main

import (
	"encoding/json"
	"strconv"
)

// Ayudas para leer el JSON de Lichess sin declarar todos sus tipos (si algo falta, dan el cero).

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
		case int:
			return x, true
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
