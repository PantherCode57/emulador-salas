#!/bin/bash
# Claude de Panther OS (un chat con la conversación de Claude Code del PC de casa): lo abre
# EmulationStation desde su tarjeta. El programa (consola/claude) y el núcleo de la pantalla están en
# /home/ark/panther/claude.
DIR=/home/ark/panther/claude
# La pantalla es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./claude
