package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// La conversación con Claude. Cada mensaje va a chat.jsonl. Claude está en el PC, en la conversación
// de Claude Code que puso esta app, y habla con la consola por SSH, con archivos:
//   - lo que escribes se deja en pendientes/<id>.txt; el PC lo recoge y lo mueve a leidos/;
//   - sus respuestas llegan a respuestas/<algo>.txt; la app las pasa al chat y las borra;
//   - mientras el PC escucha, toca pc-vivo cada 2 segundos.
type mensaje struct {
	ID    int64  `json:"id"`
	Mio   bool   `json:"mio"`
	Texto string `json:"texto"`
	Hora  int64  `json:"hora"`
}

const maxMensajes = 300

type chat struct {
	dir  string
	msgs []mensaje
}

func abrirChat(dir string) *chat {
	c := &chat{dir: dir}
	for _, d := range []string{"pendientes", "respuestas", "leidos"} {
		_ = os.MkdirAll(filepath.Join(dir, d), 0o700)
	}
	if f, err := os.Open(filepath.Join(dir, "chat.jsonl")); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			var m mensaje
			if json.Unmarshal(sc.Bytes(), &m) == nil && m.Texto != "" {
				c.msgs = append(c.msgs, m)
			}
		}
		f.Close()
	}
	if len(c.msgs) > maxMensajes {
		c.msgs = c.msgs[len(c.msgs)-maxMensajes:]
		c.guardarTodo()
	}
	return c
}

func (c *chat) guardarTodo() {
	var b strings.Builder
	for _, m := range c.msgs {
		j, _ := json.Marshal(m)
		b.Write(j)
		b.WriteByte('\n')
	}
	escribirAtomico(filepath.Join(c.dir, "chat.jsonl"), []byte(b.String()))
}

func (c *chat) añadir(m mensaje) {
	c.msgs = append(c.msgs, m)
	if len(c.msgs) > maxMensajes {
		c.msgs = c.msgs[len(c.msgs)-maxMensajes:]
		c.guardarTodo()
		return
	}
	f, err := os.OpenFile(filepath.Join(c.dir, "chat.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	j, _ := json.Marshal(m)
	_, _ = f.Write(append(j, '\n'))
	f.Close()
}

// enviar deja el mensaje para el PC y lo pone en el chat.
func (c *chat) enviar(texto string) {
	ahora := time.Now()
	id := ahora.UnixMilli()
	if n := len(c.msgs); n > 0 && c.msgs[n-1].ID >= id {
		id = c.msgs[n-1].ID + 1
	}
	escribirAtomico(filepath.Join(c.dir, "pendientes", strconv.FormatInt(id, 10)+".txt"), []byte(texto+"\n"))
	c.añadir(mensaje{ID: id, Mio: true, Texto: texto, Hora: ahora.Unix()})
}

// recoger pasa al chat las respuestas que hayan llegado. Dice si había alguna.
func (c *chat) recoger() bool {
	m, _ := filepath.Glob(filepath.Join(c.dir, "respuestas", "*.txt"))
	if len(m) == 0 {
		return false
	}
	sort.Strings(m)
	hay := false
	for _, ruta := range m {
		datos, err := os.ReadFile(ruta)
		if err != nil {
			continue
		}
		hora := time.Now()
		if st, err := os.Stat(ruta); err == nil {
			hora = st.ModTime()
		}
		_ = os.Remove(ruta)
		texto := strings.TrimSpace(strings.ReplaceAll(string(datos), "\r", ""))
		if texto == "" {
			continue
		}
		id := hora.UnixMilli()
		if n := len(c.msgs); n > 0 && c.msgs[n-1].ID >= id {
			id = c.msgs[n-1].ID + 1
		}
		c.añadir(mensaje{ID: id, Texto: texto, Hora: hora.Unix()})
		hay = true
	}
	return hay
}

// escuchando: el PC ha tocado pc-vivo hace poco.
func (c *chat) escuchando() bool {
	st, err := os.Stat(filepath.Join(c.dir, "pc-vivo"))
	return err == nil && time.Since(st.ModTime()) < 15*time.Second
}

type estadoChat int

const (
	estListo       estadoChat = iota // escuchando, sin nada pendiente
	estEnviado                       // tu mensaje está esperando a que el PC lo recoja
	estEscribiendo                   // lo ha recogido y aún no ha contestado
	estDormido                       // el PC no escucha ahora
)

func (c *chat) estado() estadoChat {
	n := len(c.msgs)
	if n > 0 && c.msgs[n-1].Mio {
		ultimo := c.msgs[n-1]
		_, err := os.Stat(filepath.Join(c.dir, "pendientes", strconv.FormatInt(ultimo.ID, 10)+".txt"))
		recogido := os.IsNotExist(err)
		if recogido && time.Since(time.Unix(ultimo.Hora, 0)) < 15*time.Minute {
			return estEscribiendo
		}
		if !recogido && c.escuchando() {
			return estEnviado
		}
	}
	if c.escuchando() {
		return estListo
	}
	return estDormido
}
