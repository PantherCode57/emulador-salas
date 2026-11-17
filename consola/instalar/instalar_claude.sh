#!/bin/bash
# Claude en la consola: el programa, su tarjeta en la pantalla principal y su apartado en el menú, y
# las tarjetas grandes (tema.py: se ven cuatro y el carrusel se mueve).
# Se lanza como ark con lo de consola/instalar y el programa compilado (claude-arm64) en
# ~/panther-instalar; la tarjeta, en ~/panther-instalar/tarjetas-internet/claude.png.
set -e
I=~/panther-instalar
mkdir -p ~/panther/claude
install -m 755 $I/claude-arm64 ~/panther/claude/claude
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/claude/
[ -f $I/tarjetas-internet/claude.png ] && cp $I/tarjetas-internet/claude.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/claude
cp $I/Claude.sh /roms/claude/Claude.sh
chmod 755 /roms/claude/Claude.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
