#!/bin/bash
# Spotin de Panther OS (tu música de Spotify): lo abre EmulationStation desde su tarjeta.
# El programa (consola/spotin) y el núcleo de la pantalla están en /home/ark/panther/spotin.
DIR=/home/ark/panther/spotin
export CONSOLA_RETROARCH=/opt/retroarch/bin/retroarch
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export CONSOLA_RETROARCH_CFG=/home/ark/.config/retroarch/retroarch.cfg
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./spotin
