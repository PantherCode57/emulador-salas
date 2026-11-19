package main

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"sync"
)

// Tu cuenta de Lichess: un token personal (empieza por «lip_») que haces tú en lichess.org y pegas
// en la página de la consola. Se guarda solo en la consola (cuenta.json, que solo puede leer ark) y
// nunca sale en el registro. Lo puedes borrar cuando quieras en Lichess (y la consola deja de entrar).
type cuenta struct {
	mu      sync.Mutex
	ruta    string
	Token   string   `json:"token"`
	Usuario string   `json:"usuario,omitempty"`
	Rivales []string `json:"rivales,omitempty"` // a quién has retado (los últimos, para no escribirlos)
}

func cargarCuenta(ruta string) *cuenta {
	c := &cuenta{ruta: ruta}
	if b, err := os.ReadFile(ruta); err == nil {
		_ = json.Unmarshal(b, c)
	}
	return c
}

func (c *cuenta) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Token
}

func (c *cuenta) tiene() bool { return c.token() != "" }

func (c *cuenta) usuario() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Usuario
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
	c.Token, c.Usuario = "", ""
	c.mu.Unlock()
	c.guardar()
}

// apuntarRival: el primero de la lista es el último al que has retado.
func (c *cuenta) apuntarRival(u string) {
	c.mu.Lock()
	n := []string{u}
	for _, r := range c.Rivales {
		if r != u && len(n) < 8 {
			n = append(n, r)
		}
	}
	c.Rivales = n
	c.mu.Unlock()
	c.guardar()
}

func (c *cuenta) rivales() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.Rivales...)
}

var reToken = regexp.MustCompile(`lip_[A-Za-z0-9]{10,}`)

// ponerCuenta: comprueba el token con Lichess (que diga de quién es) y lo guarda.
func (li *lichess) ponerCuenta(pegado string) (string, error) {
	tok := reToken.FindString(pegado)
	if tok == "" {
		return "", errors.New("eso no parece un token de Lichess (empieza por lip_ y sigue con letras y números)")
	}
	prueba := &lichess{cuenta: &cuenta{Token: tok}}
	yo, err := prueba.Cuenta()
	if err != nil {
		if errors.Is(err, errSesion) {
			return "", errors.New("Lichess dice que ese token no vale. Haz uno nuevo y cópialo entero")
		}
		return "", err
	}
	c := li.cuenta
	c.mu.Lock()
	c.Token, c.Usuario = tok, yo.Usuario
	c.mu.Unlock()
	c.guardar()
	logf("cuenta de Lichess guardada")
	return yo.Usuario, nil
}
