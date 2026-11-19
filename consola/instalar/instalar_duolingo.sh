#!/bin/bash
# Duolingo en la consola: el programa, su tarjeta en la pantalla principal y su apartado en el menú.
# Se lanza como ark con lo de consola/instalar y el programa compilado (duolingo-arm64) en
# ~/panther-instalar; la tarjeta, en ~/panther-instalar/tarjetas-internet/duolingo.png.
set -e
I=~/panther-instalar
mkdir -p ~/panther/duolingo
install -m 755 $I/duolingo-arm64 ~/panther/duolingo/duolingo
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/duolingo/
[ -f $I/tarjetas-internet/duolingo.png ] && cp $I/tarjetas-internet/duolingo.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/duolingo
cp $I/Duolingo.sh /roms/duolingo/Duolingo.sh
chmod 755 /roms/duolingo/Duolingo.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
