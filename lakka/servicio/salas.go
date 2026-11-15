package main

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sala guardada: los mismos campos que en el PC (pc/src-tauri/src/salas.rs).
type Sala struct {
	Codigo        string `json:"codigo"`
	Nombre        string `json:"nombre"`
	Juego         string `json:"juego"` // ruta dentro de JUEGOS
	JuegoTitulo   string `json:"juegoTitulo"`
	Sistema       string `json:"sistema"`
	PartidaPropia bool   `json:"partidaPropia"`
	Creada        int64  `json:"creada"`
}

func archivoSalas() string           { return filepath.Join(carpetaDatos, "salas.json") }
func carpetaSala(c string) string     { return filepath.Join(carpetaDatos, "salas", strings.ToLower(c)) }
func archivoPropio(c string) string   { return filepath.Join(carpetaSala(c), "mi_progreso.json") }
func carpetaAmigos(c string) string   { return filepath.Join(carpetaSala(c), "amigos") }
func ahoraMs() int64                  { return time.Now().UnixMilli() }
func codigoVisible(c string) string   { return strings.ToUpper(c) }
func codigoValido(c string) bool {
	if len(c) != 6 {
		return false
	}
	for _, r := range strings.ToLower(c) {
		if !strings.ContainsRune(alfabeto, r) {
			return false
		}
	}
	return true
}

func cargarSalas() []Sala {
	var lista []Sala
	if b, err := os.ReadFile(archivoSalas()); err == nil {
		_ = json.Unmarshal(b, &lista)
	}
	sort.SliceStable(lista, func(i, j int) bool { return lista[i].Creada > lista[j].Creada })
	return lista
}

func guardarSalas(lista []Sala) { guardarJSON(archivoSalas(), lista) }

// Progreso: lo que se cuentan los de la sala (mismo JSON que el PC y el móvil).
type Progreso struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	Nombre    string `json:"nombre"`
	Juego     string `json:"juego"`
	Partida   string `json:"partida"` // "sala" o "principal"
	JugadoMs  int64  `json:"jugadoMs"`
	UltimaVez int64  `json:"ultimaVez,omitempty"`
	Captura   string `json:"captura,omitempty"` // JPEG en base64
}

func leerProgreso(ruta string) *Progreso {
	b, err := os.ReadFile(ruta)
	if err != nil {
		return nil
	}
	p := &Progreso{}
	if json.Unmarshal(b, p) != nil {
		return nil
	}
	return p
}

func partidaDe(s *Sala) string {
	if s.PartidaPropia {
		return "sala"
	}
	return "principal"
}

// prepararProgreso deja al día nombre y juego al entrar en la sala; sumarProgreso añade tiempo
// jugado y, si la hay, una captura nueva. El enlace ve el cambio y se lo manda a los demás.
func prepararProgreso(s *Sala, j *Jugador) {
	p := leerProgreso(archivoPropio(s.Codigo))
	if p == nil {
		p = &Progreso{}
	}
	p.Version, p.ID, p.Nombre, p.Juego, p.Partida = 1, j.ID, j.Nombre, s.JuegoTitulo, partidaDe(s)
	guardarJSON(archivoPropio(s.Codigo), p)
}

func sumarProgreso(s *Sala, j *Jugador, ms int64, captura string) {
	p := leerProgreso(archivoPropio(s.Codigo))
	if p == nil {
		p = &Progreso{}
	}
	p.Version, p.ID, p.Nombre, p.Juego, p.Partida = 1, j.ID, j.Nombre, s.JuegoTitulo, partidaDe(s)
	p.JugadoMs += ms
	p.UltimaVez = ahoraMs()
	if captura != "" {
		p.Captura = captura
	}
	guardarJSON(archivoPropio(s.Codigo), p)
}

// progresosAmigos: los de los demás, los más recientes primero.
func progresosAmigos(codigo, idPropio string) []*Progreso {
	var lista []*Progreso
	archivos, _ := filepath.Glob(filepath.Join(carpetaAmigos(codigo), "*.json"))
	for _, a := range archivos {
		if p := leerProgreso(a); p != nil && p.ID != "" && p.ID != idPropio {
			lista = append(lista, p)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].UltimaVez > lista[j].UltimaVez })
	return lista
}

// ---------- Sala activa: el enlace en marcha ----------

// Estado de la sala en la que estamos, según lo que va diciendo el enlace.
type EstadoSala struct {
	Codigo      string
	Fase        string // conectando, buscando, listo, conectado, error
	RolDecidido bool
	Anfitrion   bool
	Amigos      int
	Error       string
}

type salaActiva struct {
	mu       sync.Mutex
	estado   EstadoSala
	cmd      *exec.Cmd
	entrada  io.WriteCloser
	parar    chan struct{}
	avisos   chan<- struct{}
	terminada chan struct{}
}

// entrarEnSala arranca el enlace y lo vuelve a arrancar si se cae (por ejemplo, sin Internet),
// hasta que se llame a detener.
func entrarEnSala(s *Sala, j *Jugador, avisos chan<- struct{}) *salaActiva {
	prepararProgreso(s, j)
	_ = os.MkdirAll(carpetaAmigos(s.Codigo), 0o755)
	a := &salaActiva{
		estado:    EstadoSala{Codigo: s.Codigo, Fase: "conectando"},
		parar:     make(chan struct{}),
		avisos:    avisos,
		terminada: make(chan struct{}),
	}
	go a.bucle(s.Codigo, j.ID)
	return a
}

func (a *salaActiva) avisar() {
	select {
	case a.avisos <- struct{}{}:
	default:
	}
}

func (a *salaActiva) Estado() EstadoSala {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.estado
}

func (a *salaActiva) bucle(codigo, id string) {
	defer close(a.terminada)
	for {
		err := a.lanzar(codigo, id)
		select {
		case <-a.parar:
			return
		default:
		}
		a.mu.Lock()
		a.estado.RolDecidido = false
		if a.estado.Error == "" {
			a.estado.Error = "Se cortó la conexión con la sala."
			if err != nil {
				a.estado.Error += " " + err.Error()
			}
		}
		a.estado.Fase = "error"
		a.mu.Unlock()
		a.avisar()
		select {
		case <-a.parar:
			return
		case <-time.After(10 * time.Second):
		}
		a.mu.Lock()
		a.estado.Error = ""
		a.estado.Fase = "conectando"
		a.mu.Unlock()
		a.avisar()
	}
}

func (a *salaActiva) lanzar(codigo, id string) error {
	cmd := exec.Command(filepath.Join(carpetaPropia, "enlace"), "sala",
		"--codigo", strings.ToLower(codigo), "--id", id,
		"--progreso", archivoPropio(codigo), "--amigos", carpetaAmigos(codigo))
	cmd.Env = append(os.Environ(), "POKEMULADOR_TS_KEY="+claveTailscale)
	entrada, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	salida, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	a.mu.Lock()
	a.cmd, a.entrada = cmd, entrada
	a.mu.Unlock()

	lector := bufio.NewScanner(salida)
	lector.Buffer(make([]byte, 64*1024), 1<<20)
	for lector.Scan() {
		var m map[string]any
		if json.Unmarshal(lector.Bytes(), &m) != nil {
			log.Printf("enlace: %s", lector.Text())
			continue
		}
		a.procesar(m)
	}
	return cmd.Wait()
}

func (a *salaActiva) procesar(m map[string]any) {
	estado, _ := m["estado"].(string)
	a.mu.Lock()
	switch estado {
	case "conectando", "buscando":
		a.estado.Fase = estado
	case "rol":
		rol, _ := m["rol"].(string)
		a.estado.RolDecidido = true
		a.estado.Anfitrion = rol == "host"
	case "listo":
		a.estado.Fase = "listo"
	case "conectado":
		a.estado.Fase = "conectado"
	case "amigos":
		if n, ok := m["n"].(float64); ok {
			a.estado.Amigos = int(n)
		}
	case "error":
		msg, _ := m["mensaje"].(string)
		a.estado.Error = msg
		a.estado.Fase = "error"
	case "aviso", "progreso":
	}
	a.mu.Unlock()
	if estado != "" && estado != "aviso" {
		log.Printf("sala %s: %v", a.estado.Codigo, m)
	}
	a.avisar()
}

// detener cierra el enlace (al cerrarle la entrada se despide de los amigos y sale).
func (a *salaActiva) detener() {
	select {
	case <-a.parar:
		return
	default:
		close(a.parar)
	}
	a.mu.Lock()
	cmd, entrada := a.cmd, a.entrada
	a.mu.Unlock()
	if entrada != nil {
		_ = entrada.Close()
	}
	if cmd != nil && cmd.Process != nil {
		select {
		case <-a.terminada:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-a.terminada
		}
	}
}
