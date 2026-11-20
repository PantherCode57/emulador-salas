package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Tu cuenta de YouTube: las cookies de la sesión de youtube.com (se pegan desde el PC en la página
// de la consola). Solo se usan para pedir tus listas (recomendaciones, suscripciones, Shorts, «Ver
// más tarde»…); los vídeos se siguen sacando sin cuenta, que es lo más seguro para ella.
// Se guardan solo en la consola (cuenta.json, que solo puede leer ark) y nunca salen en el registro.
type cuenta struct {
	mu      sync.Mutex
	ruta    string
	Cookie  string `json:"cookie"`
	Nombre  string `json:"nombre"`
	Usuario string `json:"usuario,omitempty"` // @lo_que_sea
	Foto    string `json:"foto,omitempty"`
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

func (c *cuenta) nombre() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Nombre
}

func (c *cuenta) guardar() {
	c.mu.Lock()
	b, _ := json.MarshalIndent(c, "", " ")
	c.mu.Unlock()
	tmp := c.ruta + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, c.ruta)
	}
}

// olvidar borra la sesión de la consola.
func (c *cuenta) olvidar() {
	c.mu.Lock()
	c.Cookie, c.Nombre, c.Usuario, c.Foto = "", "", "", ""
	c.mu.Unlock()
	_ = os.Remove(c.ruta)
}

// cabeceras: las de una petición con tu sesión (como las manda la web de YouTube).
func (c *cuenta) cabeceras() map[string]string {
	c.mu.Lock()
	cookie := c.Cookie
	c.mu.Unlock()
	if cookie == "" {
		return nil
	}
	h := map[string]string{"Cookie": cookie, "X-Goog-AuthUser": "0", "X-Origin": "https://www.youtube.com",
		"X-Youtube-Bootstrap-Logged-In": "true"}
	if a := autorizacion(cookie, time.Now()); a != "" {
		h["Authorization"] = a
	}
	return h
}

// autorizacion: «SAPISIDHASH …», la firma que YouTube pide junto a las cookies.
func autorizacion(cookie string, ahora time.Time) string {
	ts := fmt.Sprint(ahora.Unix())
	var partes []string
	for _, p := range [][2]string{{"SAPISID", "SAPISIDHASH"}, {"__Secure-1PAPISID", "SAPISID1PHASH"}, {"__Secure-3PAPISID", "SAPISID3PHASH"}} {
		v := valorCookie(cookie, p[0])
		if v == "" && p[0] == "SAPISID" {
			v = valorCookie(cookie, "__Secure-3PAPISID")
		}
		if v == "" {
			continue
		}
		h := sha1.Sum([]byte(ts + " " + v + " https://www.youtube.com"))
		partes = append(partes, p[1]+" "+ts+"_"+hex.EncodeToString(h[:]))
	}
	return strings.Join(partes, " ")
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
			if d := strings.TrimPrefix(c[0], "."); d != "youtube.com" && !strings.HasSuffix(d, ".youtube.com") {
				continue
			}
			pares = append(pares, strings.TrimSpace(c[5])+"="+strings.TrimSpace(c[6]))
		}
	} else {
		s = strings.ReplaceAll(s, "\n", "")
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

// ponerCuenta comprueba la sesión con YouTube (que diga de quién es) y la guarda.
func (y *youtube) ponerCuenta(texto string) error {
	cookie := limpiarCookies(texto)
	if valorCookie(cookie, "SAPISID") == "" && valorCookie(cookie, "__Secure-3PAPISID") == "" {
		return errors.New("faltan las cookies de la sesión (SAPISID). Copia la cabecera «cookie» entera")
	}
	prueba := &cuenta{Cookie: cookie}
	m, err := y.pedirCon(prueba, "account/account_menu", map[string]any{})
	if err != nil {
		return err
	}
	var cab any
	for _, ac := range lista(m, "actions") {
		if h := ir(ac, "openPopupAction", "popup", "multiPageMenuRenderer", "header", "activeAccountHeaderRenderer"); h != nil {
			cab = h
			break
		}
	}
	if cab == nil {
		return errors.New("YouTube no reconoce esa sesión (¿has entrado en tu cuenta en esa ventana?)")
	}
	y.cuenta.mu.Lock()
	y.cuenta.Cookie = cookie
	y.cuenta.Nombre = textoDe(ir(cab, "accountName"))
	y.cuenta.Usuario = textoDe(ir(cab, "channelHandle"))
	y.cuenta.Foto = txt(cab, "accountPhoto", "thumbnails", 0, "url")
	y.cuenta.mu.Unlock()
	y.cuenta.guardar()
	logf("cuenta de YouTube guardada (%d cookies)", strings.Count(cookie, ";")+1)
	return nil
}

// conSesionCaducada: YouTube contesta como si no hubiera sesión.
func sesionCaducada(m map[string]any) bool {
	for _, s := range lista(m, "responseContext", "serviceTrackingParams") {
		for _, p := range lista(s, "params") {
			if txt(p, "key") == "logged_in" {
				return txt(p, "value") == "0"
			}
		}
	}
	return false
}
