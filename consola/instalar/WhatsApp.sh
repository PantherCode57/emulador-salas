#!/bin/bash
# WhatsApp de Panther OS: lo abre EmulationStation desde el apartado «WhatsApp».
# El programa (consola/whatsapp) está en /home/ark/panther/whatsapp. Normalmente está siempre en
# marcha como servicio (panther-whatsapp.service): entonces esto solo le pide que se enseñe y espera
# a que salgas. Si el servicio no está, se abre el programa entero como antes.
DIR=/home/ark/panther/whatsapp
# La pantalla es el emulador de Panther (sin RetroArch).
export CONSOLA_RETROARCH=/home/ark/panther/emulador/emulador
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./whatsapp
