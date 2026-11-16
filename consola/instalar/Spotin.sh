#!/bin/bash
# Spotin de Panther OS (tu música de Spotify): lo abre EmulationStation desde su tarjeta.
# El programa (consola/spotin) y el núcleo de la pantalla están en /home/ark/panther/spotin.
DIR=/home/ark/panther/spotin
# La pantalla (y en Pokémon, también los juegos de GB, GBC y GBA) es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./spotin
