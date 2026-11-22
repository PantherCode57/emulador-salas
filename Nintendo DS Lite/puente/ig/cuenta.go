package ig

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
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

// limpiarCookies acepta la cabecera «cookie» copiada del navegador (con o sin «cookie:» delante), un
// cookies.txt (el de las extensiones que exportan cookies) o solo el valor de sessionid (el que se
// copia en F12 → Aplicación → Cookies), y devuelve «a=b; c=d».
func limpiarCookies(s string) string {
	s = strings.TrimSpace(s)
	if v := strings.Trim(s, "\"' \r\n"); v != "" && !strings.ContainsAny(v, "=;\t\n ") {
		// solo el valor de sessionid; si estaba «decodificado como URL», con «:» en vez de «%3A»
		return "sessionid=" + strings.ReplaceAll(v, ":", "%3A")
	}
	var pares []string
	if strings.Contains(s, "\t") { // separado por tabuladores: un cookies.txt o filas de la tabla de F12
		for _, l := range strings.Split(s, "\n") {
			l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "#HttpOnly_"))
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			c := strings.Split(l, "\t")
			for i := range c {
				c[i] = strings.TrimSpace(c[i])
			}
			if d := strings.TrimPrefix(c[0], "."); d == "instagram.com" || strings.HasSuffix(d, ".instagram.com") {
				if len(c) >= 7 { // cookies.txt: dominio, …, nombre, valor
					pares = append(pares, c[5]+"="+c[6])
				}
				continue
			}
			if len(c) >= 2 && c[1] != "" && reNombreCookie.MatchString(c[0]) && !strings.Contains(c[0], ".") {
				pares = append(pares, c[0]+"="+c[1]) // tabla de F12: nombre, valor, dominio…
			}
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

// El sessionid empieza por el número de tu cuenta (lo mismo que ds_user_id) y «%3A».
var reIDSesion = regexp.MustCompile(`^([0-9]+)%3A`)

// completarCookie: si solo se ha pegado el sessionid, pone ds_user_id (sale del propio sessionid) y
// el csrftoken (lo da instagram.com al abrirlo, como al navegador). Devuelve "" si el sessionid no
// tiene la forma de siempre.
func completarCookie(cookie string) string {
	sesion := valorCookie(cookie, "sessionid")
	if valorCookie(cookie, "ds_user_id") == "" {
		m := reIDSesion.FindStringSubmatch(sesion)
		if m == nil {
			return ""
		}
		cookie += "; ds_user_id=" + m[1]
	}
	if valorCookie(cookie, "csrftoken") == "" {
		cookie += "; csrftoken=" + tokenCsrf(cookie)
	}
	return cookie
}

var reCsrfPagina = regexp.MustCompile(`"csrf_token":"([A-Za-z0-9]{20,64})"`)

// tokenCsrf: el csrftoken que manda instagram.com al abrir su página con tu sesión (en Set-Cookie o
// dentro de la página). Si no lo manda, uno al azar con su forma (32 letras y números): Instagram
// manda el suyo en cuanto quiere otro y se cambia solo (actualizar).
func tokenCsrf(cookie string) string {
	if req, err := http.NewRequest(http.MethodGet, urlWeb+"/", nil); err == nil {
		req.Header.Set("User-Agent", agenteNavegador)
		req.Header.Set("Accept", "text/html,application/xhtml+xml")
		req.Header.Set("Accept-Language", "es-ES,es;q=0.9")
		req.Header.Set("Cookie", cookie)
		if resp, err := clienteDatos.Do(req); err == nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			resp.Body.Close()
			for _, c := range resp.Cookies() {
				if c.Name == "csrftoken" && reNombreCookie.MatchString(c.Value) {
					logf("csrftoken de instagram.com (%d)", resp.StatusCode)
					return c.Value
				}
			}
			if m := reCsrfPagina.FindSubmatch(b); m != nil {
				logf("csrftoken de la página de instagram.com (%d)", resp.StatusCode)
				return string(m[1])
			}
			logf("instagram.com (%d) no da csrftoken: uno al azar", resp.StatusCode)
		}
	}
	const letras = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letras[int(b[i])%len(letras)]
	}
	return string(b)
}

// ponerCuenta comprueba la sesión con Instagram (que diga de quién es) y la guarda. Si Instagram no
// contesta bien pero tampoco dice que no valga (errLimite), la guarda igual y devuelve errLimite
// para avisar.
func (ig *instagram) ponerCuenta(texto string) error {
	cookie := limpiarCookies(texto)
	if valorCookie(cookie, "sessionid") == "" {
		return errors.New("ahí no está el sessionid. En F12 → Aplicación → Cookies → https://www.instagram.com, copia el valor de la fila «sessionid»")
	}
	if cookie = completarCookie(cookie); cookie == "" {
		return errors.New("eso no parece el valor de sessionid (empieza por números y %3A). Cópialo entero otra vez")
	}
	prueba := &cuenta{Cookie: cookie}
	otro := &instagram{cuenta: prueba, claim: "0", disp: ig.disp}
	usuario, nombre, foto, err := otro.QuienSoy()
	if err != nil && !errors.Is(err, errLimite) {
		if errors.Is(err, errReto) {
			return errors.New("Instagram quiere que confirmes que eres tú: abre instagram.com en el navegador, acepta el aviso que salga y copia la cookie otra vez")
		}
		if errors.Is(err, errSesion) {
			return errors.New("Instagram dice que esa sesión no vale (¿cerraste sesión en esa ventana?). Entra otra vez y copia la cookie nueva")
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
	return err // nil, o errLimite si no se ha podido comprobar
}
