#!/bin/bash
# Duolingo de Panther OS: lo abre EmulationStation desde su tarjeta.
# El programa (consola/duolingo) y el núcleo de la pantalla están en /home/ark/panther/duolingo.
DIR=/home/ark/panther/duolingo
# La pantalla es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./duolingo
