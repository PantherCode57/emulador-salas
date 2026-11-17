package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Tu cuenta de Instagram: las cookies de tu sesión de instagram.com (se pegan desde el PC en la
// página de la consola). Se guardan solo en la consola (cuenta.json, que solo puede leer ark) y nunca
// salen en el registro. Instagram cambia alguna de vez en cuando (csrftoken, rur…): se van guardando.
type cuenta struct {
	mu      sync.Mutex
	ruta    string
	Cookie  string `json:"cookie"`
	Usuario string `json:"usuario"`
	Nombre  string `json:"nombre,omitempty"`
	Foto    string `json:"foto,omitempty"`
	cambio  time.Time
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
	return c.Cookie != ""
}

func (c *cuenta) id() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return valorCookie(c.Cookie, "ds_user_id")
}

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
	c.Cookie, c.Usuario, c.Nombre, c.Foto = "", "", "", ""
	c.mu.Unlock()
	if c.ruta != "" {
		_ = os.Remove(c.ruta)
	}
}

// actualizar pone en la cookie lo que cambie Instagram en sus respuestas (Set-Cookie) y lo guarda.
func (c *cuenta) actualizar(nuevas []*http.Cookie) {
	if len(nuevas) == 0 {
		return
	}
	c.mu.Lock()
	if c.Cookie == "" {
		c.mu.Unlock()
		return
	}
	pares := strings.Split(c.Cookie, "; ")
	cambiado := false
	for _, n := range nuevas {
		if !reNombreCookie.MatchString(n.Name) {
			continue
		}
		borrar := n.MaxAge < 0 || n.Value == "" || n.Value == `""` || (!n.Expires.IsZero() && n.Expires.Before(time.Now()))
		if borrar && n.Name == "sessionid" {
			continue // si Instagram la quita, ya lo dirá con un 401 (y se pide otra)
		}
		hecho := false
		for i, p := range pares {
			k, v, _ := strings.Cut(p, "=")
			if k != n.Name {
				continue
			}
			hecho = true
			if borrar {
				pares = append(pares[:i], pares[i+1:]...)
				cambiado = true
			} else if v != n.Value {
				pares[i] = n.Name + "=" + n.Value
				cambiado = true
			}
			break
		}
		if !hecho && !borrar {
			pares = append(pares, n.Name+"="+n.Value)
			cambiado = true
		}
	}
	if cambiado {
		c.Cookie = strings.Join(pares, "; ")
	}
	guardar := cambiado && time.Since(c.cambio) > 10*time.Second
	if guardar {
		c.cambio = time.Now()
	}
	c.mu.Unlock()
	if guardar {
		c.guardar()
	}
}

func valorCookie(cookie, nombre string) string {
	for _, p := range strings.Split(cookie, ";") {
		k, v, ok := strings.Cut(strings.TrimSpace(p), "=")
		if ok && k == nombre {
			return v
		}
	}
	return ""
}

var reNombreCookie = regexp.MustCompile(`^[A-Za-z0-9_\-.]+$`)

// limpiarCookies acepta la cabecera «cookie» copiada del navegador (con o sin «cookie:» delante) o
// un cookies.txt (el de las extensiones que exportan cookies) y devuelve «a=b; c=d».
func limpiarCookies(s string) string {
	s = strings.TrimSpace(s)
	var pares []string
	if strings.Contains(s, "\t") { // cookies.txt: dominio, …, nombre, valor (separados por tabuladores)
		for _, l := range strings.Split(s, "\n") {
			l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "#HttpOnly_"))
			c := strings.Split(l, "\t")
			if len(c) < 7 || strings.HasPrefix(l, "#") {
				continue
			}
			if d := strings.TrimPrefix(c[0], "."); d != "instagram.com" && !strings.HasSuffix(d, ".instagram.com") {
				continue
			}
			pares = append(pares, strings.TrimSpace(c[5])+"="+strings.TrimSpace(c[6]))
		}
	} else {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", "")
		if k, v, ok := strings.Cut(s, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "cookie") {
			s = v
		}
		for _, p := range strings.Split(s, ";") {
			p = strings.TrimSpace(p)
			if k, _, ok := strings.Cut(p, "="); ok && reNombreCookie.MatchString(k) {
				pares = append(pares, p)
			}
		}
	}
	return strings.Join(pares, "; ")
}

// ponerCuenta comprueba la sesión con Instagram (que diga de quién es) y la guarda.
func (ig *instagram) ponerCuenta(texto string) error {
	cookie := limpiarCookies(texto)
	switch {
	case valorCookie(cookie, "sessionid") == "":
		return errors.New("falta la cookie de la sesión (sessionid). Copia la cabecera «cookie» entera de una petición de instagram.com")
	case valorCookie(cookie, "ds_user_id") == "" || valorCookie(cookie, "csrftoken") == "":
		return errors.New("faltan cookies (ds_user_id o csrftoken). Copia la cabecera «cookie» entera")
	}
	prueba := &cuenta{Cookie: cookie}
	otro := &instagram{cuenta: prueba, claim: "0", disp: ig.disp}
	usuario, nombre, foto, err := otro.QuienSoy()
	if err != nil {
		if errors.Is(err, errSesion) {
			return errors.New("Instagram no reconoce esa sesión (¿has entrado en tu cuenta en esa ventana?)")
		}
		return err
	}
	prueba.mu.Lock()
	cookie = prueba.Cookie // con lo que haya cambiado Instagram al contestar
	prueba.mu.Unlock()
	ig.cuenta.mu.Lock()
	ig.cuenta.Cookie, ig.cuenta.Usuario, ig.cuenta.Nombre, ig.cuenta.Foto = cookie, usuario, nombre, foto
	ig.cuenta.mu.Unlock()
	ig.cuenta.guardar()
	ig.mu.Lock()
	ig.claim = otro.claim
	ig.mu.Unlock()
	logf("cuenta de Instagram guardada (%d cookies)", strings.Count(cookie, ";")+1)
	return nil
}
