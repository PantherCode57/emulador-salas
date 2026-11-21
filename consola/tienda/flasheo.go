package main

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
)

// Lo que usa «Flashear tarjeta» (consola/flashear):
//   - -lista-json: lo que hay en la tienda y si está puesto aquí, para elegir qué se pasa a otra
//     consola.
//   - -alta-tienda: la Panther Store en el menú de una consola nueva. Lo hace el instalador que
//     «Flashear tarjeta» deja en las consolas con dArkOS, antes de poner las apps con -poner.

type filaLista struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	Tipo   string `json:"tipo"`
	Puesto bool   `json:"puesto"`
}

func listaJSON() error {
	t := nuevaTienda()
	t.mirar()
	var l []filaLista
	for _, e := range t.apps {
		l = append(l, filaLista{ID: e.id, Nombre: e.nombre, Tipo: "app", Puesto: e.puesto})
	}
	for _, e := range t.juegos {
		l = append(l, filaLista{ID: e.id, Nombre: e.nombre, Tipo: "juego", Puesto: e.puesto})
	}
	return json.NewEncoder(os.Stdout).Encode(l)
}

// tiendaSh: lo mismo que consola/instalar/Tienda.sh.
const tiendaSh = `#!/bin/bash
# Panther Store (poner y quitar apps de Panther OS y juegos libres): lo abre EmulationStation desde su
# tarjeta. El programa (consola/tienda) y el núcleo de la pantalla están en /home/ark/panther/tienda.
DIR=/home/ark/panther/tienda
# La pantalla es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./tienda
`

func altaTienda() error {
	t := nuevaTienda()
	sh := t.rutaRoms("/roms/tienda/Tienda.sh")
	if !existe(sh) {
		if err := os.MkdirAll(filepath.Dir(sh), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(sh, []byte(tiendaSh), 0o755); err != nil {
			return err
		}
	}
	e := &elemento{id: "tienda", tipo: tApp, nombre: "Panther Store", color: color.RGBA{0x8c, 0x6c, 0xf0, 0xff},
		app: &appPanther{sistema: "tienda", script: "Tienda.sh"}}
	return t.darDeAlta(e)
}
