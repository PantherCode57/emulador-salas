#!/bin/bash
# YouTube de Panther OS: lo abre EmulationStation desde su tarjeta.
# El programa (consola/youtube) y el núcleo de la pantalla están en /home/ark/panther/youtube;
# yt-dlp, el de Spotin.
DIR=/home/ark/panther/youtube
# La pantalla (y en Pokémon, también los juegos de GB, GBC y GBA) es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./youtube
