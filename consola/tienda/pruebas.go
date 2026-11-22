package main

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas: las pantallas con datos inventados, en PNG (para verlas sin consola). No baja nada
// ni toca la consola: todo va en una carpeta temporal.
func porID(l []*elemento, id string) *elemento {
	for _, e := range l {
		if e.id == id {
			return e
		}
	}
	return nil
}

func dibujarPruebas(f *fuentes, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "tienda-prueba")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	carpetaDatos = filepath.Join(tmp, "datos")

	guardar := func(a *app, nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}

	t := nuevaTienda()
	t.local = filepath.Join(tmp, "panther-instalar")
	t.panther = filepath.Join(tmp, "panther")
	t.roms = filepath.Join(tmp, "roms")
	t.ports = filepath.Join(t.roms, "ports")
	t.descargas = filepath.Join(t.roms, ".panther-tienda")
	t.puestos = filepath.Join(carpetaDatos, "puestos")
	// Lo de 32 bits que pide Geometry Crash, de mentira.
	t.raiz = tmp
	for _, l := range append([]string{"ld-linux-armhf.so.3"}, porID(t.juegos, "gcrash.zip").juego.libs32...) {
		r := filepath.Join(tmp, "usr", "lib32", l)
		_ = os.MkdirAll(filepath.Dir(r), 0o755)
		_ = os.WriteFile(r, []byte("x"), 0o644)
	}
	if os.Getenv("TIENDA_TARJETAS") == "" {
		t.tarjetas = filepath.Join(tmp, "tarjetas")
	}
	// Lo que hay en ~/panther-instalar (de mentira) y lo que está puesto.
	for _, e := range t.apps {
		for _, p := range e.app.piezas {
			if len(p.locales) > 0 && e.id != "navegador" {
				r := filepath.Join(t.local, filepath.FromSlash(p.locales[0]))
				_ = os.MkdirAll(filepath.Dir(r), 0o755)
				_ = os.WriteFile(r, make([]byte, 3<<20+len(e.id)*700_000), 0o644)
			}
		}
	}
	for _, id := range []string{"whatsapp", "claude", "youtube", "spotin"} {
		for _, e := range t.apps {
			if e.id != id {
				continue
			}
			_ = os.MkdirAll(filepath.Join(t.panther, id), 0o755)
			_ = os.WriteFile(filepath.Join(t.panther, id, id), []byte("x"), 0o755)
			_ = os.MkdirAll(filepath.Join(t.roms, id), 0o755)
			_ = os.WriteFile(filepath.Join(t.roms, id, e.app.script), []byte("#!/bin/bash\n"), 0o755)
		}
	}
	_ = os.MkdirAll(t.ports, 0o755)
	_ = os.WriteFile(filepath.Join(t.ports, "Celeste Classic.sh"), []byte("#!/bin/bash\n"), 0o755)
	t.errIndice = errors.New("404")

	a := nuevaApp(f, t)
	a.cargaHasta = time.Now().Add(time.Hour)
	if err := guardar(a, "1-carga"); err != nil {
		return err
	}
	a.cargaHasta = time.Time{}

	// Apps: Instagram bajando, Duolingo en cola, el navegador sin fuente.
	op := func(e *elemento, empezada bool, fase string, hecho, total int64) {
		ctx, cancel := context.WithCancel(context.Background())
		e.op = &operacion{e: e, fase: fase, hecho: hecho, total: total, empezada: empezada, ctx: ctx, cancel: cancel}
		a.ops++
	}
	op(porID(t.apps, "instagram"), true, "Bajando", 5<<20, 8<<20+300<<10)
	op(porID(t.apps, "duolingo"), false, "En cola", 0, 0)
	a.sel[0] = 2
	if err := guardar(a, "2-apps"); err != nil {
		return err
	}
	a.sel[0], a.prim[0] = 8, 5
	if err := guardar(a, "3-apps-abajo"); err != nil {
		return err
	}
	a.sel[0], a.prim[0] = 9, 7 // la Pokédex
	if err := guardar(a, "3-apps-pokedex"); err != nil {
		return err
	}
	a.sel[0], a.prim[0] = 8, 5

	// Juegos: SuperTux descomprimiendo, Cave Story que falló, Celeste puesto.
	for _, e := range t.apps {
		e.op = nil
	}
	a.ops = 0
	a.pest = 1
	op(porID(t.juegos, "supertux.zip"), true, "Descomprimiendo", 120<<20, 186<<20)
	porID(t.juegos, "cave.story-evo.zip").fallo = "no hay conexión"
	t.cambios = true
	a.sel[1] = 1
	if err := guardar(a, "4-juegos"); err != nil {
		return err
	}
	a.sel[1] = 0
	a.pant = pFicha
	if err := guardar(a, "5-ficha-juego"); err != nil {
		return err
	}
	// Geometry Crash: la ficha y preparándose (lo bajado de GameJolt).
	a.pant = pLista
	a.sel[1], a.prim[1] = 7, 4
	gc := porID(t.juegos, "gcrash.zip")
	if err := guardar(a, "5b-juegos-abajo"); err != nil {
		return err
	}
	a.pant = pFicha
	if err := guardar(a, "5c-ficha-geometry-crash"); err != nil {
		return err
	}
	op(gc, true, "Preparando", 40, 100)
	if err := guardar(a, "5d-geometry-crash-preparando"); err != nil {
		return err
	}
	gc.op = nil
	a.ops--
	a.sel[1], a.prim[1] = 0, 0
	a.pant = pFicha
	a.pest, a.sel[0], a.prim[0] = 0, 1, 0
	if err := guardar(a, "6-ficha-app"); err != nil {
		return err
	}
	a.pant = pLista
	a.pest = 1
	a.sel[1] = 1
	a.accion(porID(t.juegos, "cceleste.zip"))
	if err := guardar(a, "7-quitar"); err != nil {
		return err
	}
	a.dlg = nil
	a.pest, a.sel[0] = 0, 0
	a.accion(porID(t.apps, "whatsapp"))
	if err := guardar(a, "8-quitar-whatsapp"); err != nil {
		return err
	}
	a.dlg = nil
	a.aviso("Celeste Classic ya está. Su tarjeta sale al salir de la tienda")
	if err := guardar(a, "9-aviso"); err != nil {
		return err
	}
	a.avisoTexto = ""
	a.saliendo = true
	a.cargaHasta = time.Now().Add(time.Hour)
	if err := guardar(a, "10-saliendo"); err != nil {
		return err
	}
	if err := tarjetaDeApp(f, porNombre(t.apps, "pokedex"), filepath.Join(dir, "tarjeta-pokedex.png")); err != nil {
		return err
	}
	return guardarTarjeta(f, filepath.Join(dir, "tarjeta-tienda.png"))
}
