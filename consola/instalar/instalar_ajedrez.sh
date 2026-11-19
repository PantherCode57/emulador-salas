#!/bin/bash
# Ajedrez (Lichess) en la consola: el programa, su tarjeta en la pantalla principal y su apartado en el menú.
# Se lanza como ark con lo de consola/instalar y el programa compilado (ajedrez-arm64) en
# ~/panther-instalar; la tarjeta, en ~/panther-instalar/tarjetas-internet/ajedrez.png.
set -e
I=~/panther-instalar
mkdir -p ~/panther/ajedrez
install -m 755 $I/ajedrez-arm64 ~/panther/ajedrez/ajedrez
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/ajedrez/
[ -f $I/tarjetas-internet/ajedrez.png ] && cp $I/tarjetas-internet/ajedrez.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/ajedrez
cp $I/Ajedrez.sh /roms/ajedrez/Ajedrez.sh
chmod 755 /roms/ajedrez/Ajedrez.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
