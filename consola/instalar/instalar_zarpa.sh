#!/bin/bash
# Zarpa en la consola: el programa, su tarjeta en la pantalla principal y su apartado en el menú.
# Se lanza como ark con lo de consola/instalar y el programa compilado (zarpa-arm64) en
# ~/panther-instalar; la tarjeta, en ~/panther-instalar/tarjetas-internet/zarpa.png.
set -e
I=~/panther-instalar
mkdir -p ~/panther/zarpa
install -m 755 $I/zarpa-arm64 ~/panther/zarpa/zarpa
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/zarpa/
[ -f $I/tarjetas-internet/zarpa.png ] && cp $I/tarjetas-internet/zarpa.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/zarpa
cp $I/Zarpa.sh /roms/zarpa/Zarpa.sh
chmod 755 /roms/zarpa/Zarpa.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
