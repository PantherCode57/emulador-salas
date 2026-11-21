#!/bin/bash
# Pokédex de Panther OS (todos los Pokémon para marcar las cartas que tienes): lo abre
# EmulationStation desde su tarjeta. El programa (consola/pokedex) y el núcleo de la pantalla están
# en /home/ark/panther/pokedex.
DIR=/home/ark/panther/pokedex
# La pantalla es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./pokedex
