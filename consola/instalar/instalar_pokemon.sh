#!/bin/bash
# Pokémon (Pokemulador en la consola): el programa, el enlace de las salas (entra en Tailscale con
# tu cuenta, sin clave), el mGBA de Pokemulador (GBA-PK), su tarjeta en la pantalla principal y su
# apartado en el menú. Se lanza como ark con lo de consola/instalar y lo compilado en
# ~/panther-instalar (pokemon-arm64, enlace, mgba_libretro.so).
set -e
I=~/panther-instalar
P=~/panther/pokemon
mkdir -p $P ~/.config/panther/pokemon
install -m 755 $I/pokemon-arm64 $P/pokemon
[ -f $I/enlace ] && install -m 755 $I/enlace $P/enlace
[ -f $I/mgba_libretro.so ] && install -m 644 $I/mgba_libretro.so $P/mgba_libretro.so
# GBA-PK (veros por el mapa en GBA) y los Caramelos Raros de Rojo Fuego: scripts de Lua del mGBA.
install -m 644 $I/pokemon-lua/*.lua $P/
cp ~/panther/whatsapp/pokemulador_libretro.so $P/
[ -f $I/tarjetas-pokemon/pokemon.png ] && cp $I/tarjetas-pokemon/pokemon.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/pokemon
cp $I/Pokemon.sh /roms/pokemon/Pokemon.sh
chmod 755 /roms/pokemon/Pokemon.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
