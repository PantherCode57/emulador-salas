#!/bin/bash
# WhatsApp de Panther OS: lo abre EmulationStation desde el apartado «WhatsApp».
# El programa (consola/whatsapp) y el núcleo de la pantalla están en /home/ark/panther/whatsapp.
DIR=/home/ark/panther/whatsapp
# El binario de verdad, no el script /usr/local/bin/retroarch (que lo abre aparte y, al cerrarlo,
# dejaba RetroArch abierto encima del menú).
export CONSOLA_RETROARCH=/opt/retroarch/bin/retroarch
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export CONSOLA_RETROARCH_CFG=/home/ark/.config/retroarch/retroarch.cfg
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./whatsapp
