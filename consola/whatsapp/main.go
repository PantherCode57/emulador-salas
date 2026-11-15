// WhatsApp ligero para la consola (Panther OS: dArkOS4Clone en la XF35H).
//
// Se abre como un «port» desde EmulationStation (consola/ports/WhatsApp.sh). Este programa lleva la
// sesión de WhatsApp con whatsmeow (la consola se vincula como dispositivo escaneando un QR) y
// dibuja la pantalla; RetroArch, con el núcleo «Pokemulador» de la pantalla de salas (lakka/nucleo),
// solo la enseña y nos pasa los botones. Sin navegador: gasta unos 30 MB de memoria.
//
// Con -prueba <carpeta> no se conecta a nada: dibuja las pantallas con datos de ejemplo en PNG.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

var (
	carpetaDatos   = entorno("WHATSAPP_DATOS", filepath.Join(os.Getenv("HOME"), ".config", "panther", "whatsapp"))
	retroarchBin   = entorno("CONSOLA_RETROARCH", "retroarch")
	nucleoPantalla = entorno("CONSOLA_NUCLEO", "pokemulador_libretro.so")
)

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func logf(format string, args ...any) { log.Printf(format, args...) }

func escribirAtomico(ruta string, datos []byte) {
	tmp := ruta + ".tmp"
	if os.WriteFile(tmp, datos, 0o600) != nil {
		return
	}
	if os.Rename(tmp, ruta) != nil {
		_ = os.WriteFile(ruta, datos, 0o600)
		_ = os.Remove(tmp)
	}
}

func main() {
	prueba := flag.String("prueba", "", "dibuja las pantallas de ejemplo en esta carpeta y sale")
	diag := flag.Duration("diagnostico", 0, "sin pantalla: pide el QR con registro detallado durante este tiempo y sale")
	flag.Parse()
	if *diag > 0 {
		diagnostico(*diag)
		return
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ANCHO")); err == nil && v >= 320 {
		ancho = v
	}
	if v, err := strconv.Atoi(os.Getenv("POKEMULADOR_ALTO")); err == nil && v >= 240 {
		alto = v
	}
	f := cargarFuentes()
	if *prueba != "" {
		if err := dibujarPruebas(f, *prueba); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := os.MkdirAll(carpetaDatos, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if reg, err := os.OpenFile(filepath.Join(carpetaDatos, "registro.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
		log.SetOutput(io.MultiWriter(reg, os.Stderr))
		os.Stdout = reg // los mensajes de whatsmeow también
	}
	log.Printf("WhatsApp de la consola en marcha (%dx%d)", ancho, alto)

	a := nuevaApp(f)
	botones := make(chan uint16, 64)
	a.srv = nuevoServidorPantalla(botones)
	if err := a.srv.escuchar(); err != nil {
		log.Fatalf("no se pudo abrir la pantalla (¿hay otra app abierta?): %v", err)
	}
	w, err := nuevoWA(a)
	if err != nil {
		log.Fatalf("no se pudo preparar WhatsApp: %v", err)
	}
	a.w = w
	a.dibujar()
	a.srv.enviar(a.l.marco())

	ra, err := abrirRetroArch()
	if err != nil {
		log.Fatalf("no se pudo abrir RetroArch: %v", err)
	}
	señal := make(chan os.Signal, 1)
	signal.Notify(señal, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go func() {
		s := <-señal
		log.Printf("señal %v: salgo", s)
		a.terminar()
	}()

	w.arrancar()
	a.bucle(botones, ra)

	w.cerrar()
	ra.cerrar()
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// dibujarPruebas pinta cada pantalla con datos inventados, para revisar el diseño en el PC.
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	a := nuevaApp(f)
	a.w = &wa{app: a, chats: map[string]*Chat{}, nombres: map[string]string{}, pedidoNombre: map[string]bool{}}
	ahora := time.Now()
	ej := []struct {
		jid, nombre string
		grupo       bool
		no          int
		ms          []*Mensaje
	}{
		{"34600000001@s.whatsapp.net", "Mamá", false, 2, []*Mensaje{
			{ID: "1", Texto: "¿Vas a venir a cenar?", Hora: ahora.Add(-40 * time.Minute)},
			{ID: "2", Mio: true, Texto: "Sí, en media hora estoy", Hora: ahora.Add(-35 * time.Minute)},
			{ID: "3", Texto: "Vale, compra pan de camino por favor, que se nos ha acabado y mañana no abren", Hora: ahora.Add(-3 * time.Minute)},
			{ID: "4", Texto: "[Foto]: así ha quedado la tarta", Hora: ahora.Add(-2 * time.Minute)},
		}},
		{"120363000000000001@g.us", "Pokemulador 🎮", true, 5, []*Mensaje{
			{ID: "5", De: "Álex", Remite: "34600000002@s.whatsapp.net", Texto: "¿Quién se apunta a Esmeralda esta noche?", Hora: ahora.Add(-20 * time.Minute)},
			{ID: "6", De: "Lucía", Remite: "34600000003@s.whatsapp.net", Texto: "Yo! Código de sala: PIKA42", Hora: ahora.Add(-18 * time.Minute)},
			{ID: "7", Mio: true, Texto: "Voy con la consola nueva 😎", Hora: ahora.Add(-17 * time.Minute)},
		}},
		{"34600000004@s.whatsapp.net", "Dani", false, 0, []*Mensaje{
			{ID: "8", Mio: true, Texto: "Mañana te paso el GTA", Hora: ahora.Add(-26 * time.Hour), Error: true},
		}},
		{"34600000005@s.whatsapp.net", "+34 600 000 005", false, 0, []*Mensaje{{ID: "9", Texto: "[Nota de voz]", Hora: ahora.Add(-4 * 24 * time.Hour)}}},
		{"34600000006@s.whatsapp.net", "Entrenamiento fútbol sala", false, 0, []*Mensaje{{ID: "10", Texto: "El jueves a las 19:00", Hora: ahora.Add(-9 * 24 * time.Hour)}}},
		{"34600000007@s.whatsapp.net", "Abuela", false, 1, []*Mensaje{{ID: "11", Texto: "Un beso cariño", Hora: ahora.Add(-30 * 24 * time.Hour)}}},
		{"34600000008@s.whatsapp.net", "Clase 2ºB", true, 0, []*Mensaje{{ID: "12", De: "Profe", Texto: "Mañana examen", Hora: ahora.Add(-31 * 24 * time.Hour)}}},
	}
	for _, e := range ej {
		c := &Chat{JID: e.jid, Nombre: f.limpiar(e.nombre), Grupo: e.grupo, NoLeidos: e.no}
		for _, m := range e.ms {
			m.Texto = f.limpiar(m.Texto)
			c.añadir(m)
		}
		a.w.chats[c.JID] = c
	}
	guardar := func(nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(carpeta, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}
	pasos := []struct {
		nombre string
		prep   func()
	}{
		{"1-qr", func() {
			a.estado, a.qr = waQR, "2@ejemplo-de-codigo-qr-de-whatsapp-para-probar-el-dibujo,abcdefghijklmnopqrstuvwxyz0123456789,ABCDEFGHIJKLMNOPQRSTUVWXYZ=="
		}},
		{"2-chats", func() { a.estado, a.pant, a.selChat = waConectado, pChats, 1 }},
		{"3-chat", func() { a.abierto, a.pant = a.w.chats["34600000001@s.whatsapp.net"], pChat }},
		{"4-grupo", func() { a.abierto = a.w.chats["120363000000000001@g.us"] }},
		{"5-teclado", func() {
			a.pant = pTeclado
			a.tec.borrador, a.tec.cursor = []rune("Vale, ahora la compro"), 21
			a.tec.fila, a.tec.col = 2, 3
		}},
		{"6-rapidas", func() { a.pant, a.rapidas, a.selRapida = pRapidas, rapidasPorDefecto, 2 }},
		{"7-menu", func() { a.pant, a.selMenu = pMenu, 0 }},
		{"8-aviso", func() { a.pant = pChats; a.estado = waSinRed; a.aviso("Lucía: ¿Echamos una partida en las salas?") }},
	}
	for _, p := range pasos {
		p.prep()
		if err := guardar(p.nombre); err != nil {
			return err
		}
	}
	return nil
}
