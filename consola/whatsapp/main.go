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
	"image"
	"image/color"
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
	a.media = nuevosMedios(a, filepath.Join(carpetaDatos, "media"))
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
	// El logo, ya mismo en la pantalla (EmulationStation la acaba de soltar) y como primer marco
	// del núcleo: así no se ve negro mientras arranca RetroArch.
	a.dibujarCarga()
	pintarFB(a.l.img)
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
	// Al cerrar, otra vez el logo hasta que vuelve el menú.
	a.dibujarCarga()
	pintarFB(a.l.img)
	log.Printf("hasta luego")
	time.Sleep(100 * time.Millisecond)
}

// dibujarPruebas pinta cada pantalla con datos inventados, para revisar el diseño en el PC.
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	a := nuevaApp(f)
	a.cargaMin, a.cargaMax = time.Time{}, time.Time{} // sin pantalla de carga (se dibuja aparte)
	a.w = &wa{app: a, chats: map[string]*Chat{}, nombres: map[string]string{}, pedidoNombre: map[string]bool{}, pedidoHistorial: map[string]bool{}, lidPN: map[string]string{}}
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
			{ID: "4", Texto: "[Foto]: así ha quedado la tarta", Hora: ahora.Add(-2 * time.Minute), Tipo: tipoFoto, Pie: "así ha quedado la tarta"},
		}},
		{"120363000000000001@g.us", "Pokemulador 🎮", true, 5, []*Mensaje{
			{ID: "5", De: "Álex", Remite: "34600000002@s.whatsapp.net", Texto: "¿Quién se apunta a Esmeralda esta noche?", Hora: ahora.Add(-20 * time.Minute)},
			{ID: "6", De: "Lucía", Remite: "34600000003@s.whatsapp.net", Texto: "Yo! Código de sala: PIKA42", Hora: ahora.Add(-18 * time.Minute)},
			{ID: "7", Mio: true, Texto: "Voy con la consola nueva 😎", Hora: ahora.Add(-17 * time.Minute)},
			{ID: "s1", De: "Lucía", Remite: "34600000003@s.whatsapp.net", Texto: "[Sticker]", Hora: ahora.Add(-16 * time.Minute), Tipo: tipoSticker},
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
	// Una foto y un sticker inventados, ya «descargados».
	a.media = nuevosMedios(a, filepath.Join(carpeta, "media"))
	foto := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			foto.Set(x, y, color.RGBA{uint8(120 + x/8), uint8(60 + y/5), uint8(160 - x/10), 255})
		}
	}
	for y := 200; y < 480; y++ { // la «tarta»
		for x := 250; x < 550; x++ {
			if (x-400)*(x-400)/9+(y-340)*(y-340)/4 < 2500 {
				foto.Set(x, y, color.RGBA{250, 230, 240, 255})
			}
		}
	}
	sticker := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			if d := (x-256)*(x-256) + (y-256)*(y-256); d < 200*200 {
				c := color.RGBA{255, 200, 40, 255}
				if (x-190)*(x-190)+(y-200)*(y-200) < 900 || (x-322)*(x-322)+(y-200)*(y-200) < 900 || (d > 120*120 && d < 140*140 && y > 280) {
					c = color.RGBA{60, 40, 20, 255}
				}
				sticker.Set(x, y, c)
			}
		}
	}
	for _, p := range []struct {
		m   *Mensaje
		img image.Image
	}{{a.w.chats["34600000001@s.whatsapp.net"].Mensajes[3], foto}, {a.w.chats["120363000000000001@g.us"].Mensajes[3], sticker}} {
		w, h := tamCompleta()
		completa := ajustarA(p.img, w, h)
		vw, vh := tamVista(p.m)
		a.media.recordar(p.m.ID, &imagenes{completa: completa, vista: ajustarA(completa, vw, vh)})
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
	a.dibujarCarga()
	if out, err := os.Create(filepath.Join(carpeta, "0-carga.png")); err == nil {
		_ = png.Encode(out, a.l.img)
		out.Close()
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
		{"9-visor", func() {
			a.estado, a.avisoTexto = waConectado, ""
			a.abierto = a.w.chats["34600000001@s.whatsapp.net"]
			a.visor, a.visorI, a.pant = mediosDe(a.abierto), 0, pVisor
		}},
		{"10-sticker", func() { a.abierto, a.pant, a.desplazar = a.w.chats["120363000000000001@g.us"], pChat, 0 }},
	}
	for _, p := range pasos {
		p.prep()
		if err := guardar(p.nombre); err != nil {
			return err
		}
	}
	return nil
}
