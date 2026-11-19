package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Tu cuenta de Duolingo: el jwt_token de tu sesión de duolingo.com (se pega desde el PC o el móvil
// en la página de la consola). Se guarda solo en la consola (cuenta.json, que solo puede leer ark) y
// nunca sale en el registro.
type cuenta struct {
	mu      sync.Mutex
	ruta    string
	Token   string `json:"token"`
	ID      int64  `json:"id"`
	Usuario string `json:"usuario,omitempty"`
	Nombre  string `json:"nombre,omitempty"`
}

func cargarCuenta(ruta string) *cuenta {
	c := &cuenta{ruta: ruta}
	if b, err := os.ReadFile(ruta); err == nil {
		_ = json.Unmarshal(b, c)
	}
	return c
}

func (c *cuenta) tiene() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Token != "" && c.ID != 0
}

func (c *cuenta) datos() (token string, id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Token, c.ID
}

func (c *cuenta) guardar() {
	if c.ruta == "" {
		return
	}
	c.mu.Lock()
	b, _ := json.MarshalIndent(c, "", " ")
	c.mu.Unlock()
	tmp := c.ruta + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, c.ruta)
	}
}

func (c *cuenta) olvidar() {
	c.mu.Lock()
	c.Token, c.ID, c.Usuario, c.Nombre = "", 0, "", ""
	c.mu.Unlock()
	if c.ruta != "" {
		_ = os.Remove(c.ruta)
	}
}

// Un JWT: tres trozos en base64 (sin relleno) separados por puntos; el primero empieza por «eyJ»
// (que es «{"» en base64).
var reJWT = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

// sacarToken busca el jwt_token en lo pegado: el valor solo, la cabecera «cookie» entera, un
// cookies.txt o la fila de la tabla de F12. Devuelve el token y el número de la cuenta (el «sub» de
// dentro del token).
func sacarToken(texto string) (string, int64, error) {
	texto = strings.ReplaceAll(texto, "%2E", ".") // por si se copió «decodificado como URL» al revés
	var token string
	if i := strings.Index(texto, "jwt_token"); i >= 0 { // si viene con su nombre, el que va detrás
		token = reJWT.FindString(texto[i:])
	}
	if token == "" {
		token = reJWT.FindString(texto)
	}
	if token == "" {
		return "", 0, errors.New("ahí no está el jwt_token. En F12 → Aplicación → Cookies → https://www.duolingo.com, copia el valor de la fila «jwt_token» (empieza por eyJ)")
	}
	partes := strings.Split(token, ".")
	carga, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(partes[1], "="))
	if err != nil {
		return "", 0, errors.New("ese jwt_token está cortado: cópialo entero otra vez")
	}
	var d struct {
		Sub json.RawMessage `json:"sub"`
	}
	if json.Unmarshal(carga, &d) != nil || len(d.Sub) == 0 {
		return "", 0, errors.New("ese jwt_token no es de una cuenta de Duolingo")
	}
	id, err := strconv.ParseInt(strings.Trim(string(d.Sub), `"`), 10, 64)
	if err != nil || id <= 0 {
		return "", 0, errors.New("ese jwt_token no es de una cuenta de Duolingo")
	}
	return token, id, nil
}
