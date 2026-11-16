#!/bin/bash
# Navegador de Panther OS (un lector de páginas web): lo abre EmulationStation desde su tarjeta.
# El programa (consola/navegador) y el núcleo de la pantalla están en /home/ark/panther/navegador.
DIR=/home/ark/panther/navegador
# La pantalla (y en Pokémon, también los juegos de GB, GBC y GBA) es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./navegador
