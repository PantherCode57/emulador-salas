package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// YouTube para la DS. Buscar y sacar los vídeos lo hace la app de Android (el «ayudante», con
// NewPipe Extractor, la librería de la app NewPipe), que también los descodifica (con lo que trae el
// móvil) y le pasa al puente las imágenes ya pequeñas y el sonido; el puente se lo prepara a la DS
// (video.go). La DS pide:
//
//	YT ESTADO          R 1 / ESTADO <listo|sin-ayudante>
//	YT BUSCAR <texto>  R n / VIDEOS <hay más 0/1>, y V\t<clave>\t<clave miniatura>\t<título>\t<canal>\t<duración>\t<vistas>
//	YT MAS             lo mismo, con lo siguiente de la última búsqueda
//
// La miniatura se pide con IMG y el vídeo con VER.

type ytVideo struct {
	URL    string `json:"url"`
	ID     string `json:"id"`
	Titulo string `json:"titulo"`
	Canal  string `json:"canal"`
	Seg    int64  `json:"seg"`
	Vistas int64  `json:"vistas"`
	Mini   string `json:"mini"`
}

type ytPagina struct {
	Videos []ytVideo `json:"videos"`
	Mas    bool      `json:"mas"`
	Error  string    `json:"error"`
}

type youtubeDS interface {
	listo() bool
	buscar(q string) (ytPagina, error)
	mas() (ytPagina, error)
}

func (p *puente) ordenYT(linea string) []string {
	if p.yt == nil {
		return []string{"ERROR este puente no lleva YouTube"}
	}
	cmd, arg, _ := strings.Cut(linea, " ")
	var pg ytPagina
	var err error
	switch cmd {
	case "ESTADO":
		if p.yt.listo() {
			return []string{"ESTADO listo"}
		}
		return []string{"ESTADO sin-ayudante"}
	case "BUSCAR":
		if strings.TrimSpace(arg) == "" {
			return []string{"ERROR ¿qué busco?"}
		}
		pg, err = p.yt.buscar(strings.TrimSpace(arg))
	case "MAS":
		pg, err = p.yt.mas()
	default:
		return []string{"ERROR no sé hacer «YT " + limpiar(cmd, 20) + "»"}
	}
	if err != nil {
		return []string{"ERROR " + limpiar(err.Error(), 120)}
	}
	reg := p.registro()
	l := []string{"VIDEOS " + uno(pg.Mas)}
	for _, v := range pg.Videos {
		if len(l) > 30 {
			break
		}
		mini := v.Mini
		if v.ID != "" && !strings.HasPrefix(v.URL, "falso:") {
			mini = "https://i.ytimg.com/vi/" + v.ID + "/mqdefault.jpg" // 320×180, en JPEG
		}
		l = append(l, strings.Join([]string{"V", reg.clave(v.URL), reg.clave(mini), limpiar(v.Titulo, 120),
			limpiar(v.Canal, 50), duracion(v.Seg), vistas(v.Vistas)}, "\t"))
	}
	return l
}

func duracion(s int64) string {
	if s <= 0 {
		return ""
	}
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func vistas(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n >= 1e9:
		return strings.Replace(strconv.FormatFloat(float64(n)/1e9, 'f', 1, 64), ".", ",", 1) + " mil M de vistas"
	case n >= 1e6:
		return strings.Replace(strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64), ".", ",", 1) + " M de vistas"
	case n >= 1e3:
		return strconv.FormatInt(n/1000, 10) + " mil vistas"
	}
	return strconv.FormatInt(n, 10) + " vistas"
}

// ---- El ayudante (la app de Android) ------------------------------------------------------------

// ayudante: el programa que hace lo que Go no puede en el móvil (NewPipe y los descodificadores de
// vídeo de Android). Escucha solo en el propio móvil (127.0.0.1) y pide una clave al conectarse.
// Cada petición es una conexión: una línea con la clave, otra con la petición, y la respuesta.
type ayudante struct {
	dir, clave string
}

func (a *ayudante) abrir(peticion string) (net.Conn, *bufio.Reader, error) {
	c, err := net.DialTimeout("tcp", a.dir, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("la app no contesta (%v)", err)
	}
	if _, err := c.Write([]byte(a.clave + "\n" + peticion + "\n")); err != nil {
		c.Close()
		return nil, nil, err
	}
	return c, bufio.NewReaderSize(c, 256<<10), nil
}

// pedirJSON: una petición que se contesta con una línea de JSON.
func (a *ayudante) pedirJSON(peticion string, plazo time.Duration, v any) error {
	c, r, err := a.abrir(peticion)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(plazo))
	l, err := r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("la app no ha contestado (%v)", err)
	}
	return json.Unmarshal(l, v)
}

type ytAyudante struct{ a *ayudante }

func (y ytAyudante) listo() bool { return y.a != nil }

func (y ytAyudante) pagina(peticion string) (ytPagina, error) {
	var pg ytPagina
	if y.a == nil {
		return pg, errors.New("YouTube va con la app Puente DS del móvil")
	}
	if err := y.a.pedirJSON(peticion, 40*time.Second, &pg); err != nil {
		return pg, err
	}
	if pg.Error != "" {
		return pg, errors.New(pg.Error)
	}
	return pg, nil
}

func (y ytAyudante) buscar(q string) (ytPagina, error) {
	return y.pagina("BUSCAR\t" + strings.ReplaceAll(q, "\t", " "))
}

func (y ytAyudante) mas() (ytPagina, error) { return y.pagina("MAS") }

// ---- El de mentira (para las pruebas) -----------------------------------------------------------

type ytFalso struct{ pagina int }

func (y *ytFalso) listo() bool { return true }

func (y *ytFalso) buscar(q string) (ytPagina, error) {
	y.pagina = 0
	pg := ytPagina{Mas: true}
	titulos := []string{"Cómo dibujar un gato", "Gatos graciosos 2026", "Aprende a hacer slime", "Pokémon: los 10 más raros",
		"Receta de tortitas fácil", "Canción para estudiar"}
	for i, t := range titulos {
		pg.Videos = append(pg.Videos, ytVideo{URL: "falso:video", ID: fmt.Sprintf("falso%d", i+1),
			Titulo: t + " (" + q + ")", Canal: "Canal de prueba", Seg: int64(6 + i*65), Vistas: int64(1500 * (i + 1) * (i + 1)),
			Mini: fmt.Sprintf("falso:mini-%d", i+1)})
	}
	return pg, nil
}

func (y *ytFalso) mas() (ytPagina, error) {
	y.pagina++
	return ytPagina{Videos: []ytVideo{{URL: "falso:video", ID: "falso-mas", Titulo: fmt.Sprintf("Otro vídeo (página %d)", y.pagina+1),
		Canal: "Canal de prueba", Seg: 30, Vistas: 99, Mini: "falso:mini-mas"}}}, nil
}
