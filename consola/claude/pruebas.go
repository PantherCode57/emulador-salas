package main

import (
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas: las pantallas con una conversación de ejemplo, en PNG (para verlas sin consola).
func dibujarPruebas(f *fuentes, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "claude-prueba")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	guardar := func(a *app, nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}

	ch := abrirChat(tmp)
	a := nuevaApp(f, ch)
	a.cargaHasta = time.Now().Add(time.Hour)
	if err := guardar(a, "1-carga"); err != nil {
		return err
	}
	a.cargaHasta = time.Time{}
	a.estado = estListo
	if err := guardar(a, "2-vacio"); err != nil {
		return err
	}

	ahora := time.Now().Unix()
	ej := []mensaje{
		{Mio: true, Texto: "Hola Claude! Te escribo desde la consola"},
		{Texto: "¡Hola! Te leo perfectamente desde el PC. ¿Qué tal funciona la app? Si quieres puedo explicarte cómo usar el teclado o lo que necesites."},
		{Mio: true, Texto: "Mañana voy a enseñar Spotin y YouTube en el instituto. Me ayudas a preparar qué decir?"},
		{Texto: "Claro. Empieza por lo que más impresiona:\n1. Spotin: tu música de Spotify sonando en la consola.\n2. YouTube: busca un vídeo y ponlo.\n3. Cuenta que todo lo has montado tú en Panther OS."},
		{Mio: true, Texto: "Genial, gracias"},
	}
	for i, m := range ej {
		m.ID, m.Hora = int64(i+1), ahora
		ch.msgs = append(ch.msgs, m)
	}
	a.estado = estEscribiendo
	if err := guardar(a, "3-chat"); err != nil {
		return err
	}
	a.scroll = 120
	a.estado = estDormido
	if err := guardar(a, "4-arriba-sin-pc"); err != nil {
		return err
	}
	a.scroll = 0
	a.pant = pEscribir
	for _, r := range "Qué tal lo hago" {
		a.tec.escribir(r)
	}
	a.tec.abrir()
	if err := guardar(a, "5-teclado"); err != nil {
		return err
	}
	a.pant = pChat
	a.aviso("Claude no escucha ahora: le llegará cuando vuelva")
	return guardar(a, "6-aviso")
}
