#!/bin/bash
# Zarpa (el juego de carreras por Los Ángeles): lo abre EmulationStation desde su tarjeta.
# El programa (consola/zarpa) y el núcleo de la pantalla están en /home/ark/panther/zarpa.
DIR=/home/ark/panther/zarpa
# La pantalla es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./zarpa
