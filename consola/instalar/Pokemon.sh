#!/bin/bash
# Pokémon de Panther OS (Pokemulador en la consola): lo abre EmulationStation desde su tarjeta.
# El programa (consola/pokemon), el enlace de las salas, el mGBA de Pokemulador y el núcleo de la
# pantalla están en /home/ark/panther/pokemon.
DIR=/home/ark/panther/pokemon
export CONSOLA_RETROARCH=/opt/retroarch/bin/retroarch
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export CONSOLA_RETROARCH_CFG=/home/ark/.config/retroarch/retroarch.cfg
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./pokemon
