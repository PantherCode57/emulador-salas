#!/bin/bash
# YouTube en la consola: el programa, su tarjeta en la pantalla principal y su apartado en el menú.
# Usa el yt-dlp de Spotin (~/panther/spotin/bin/yt-dlp). Se lanza como ark con lo de
# consola/instalar y el programa compilado (youtube-arm64) en ~/panther-instalar.
set -e
I=~/panther-instalar
mkdir -p ~/panther/youtube /roms/videos/youtube
install -m 755 $I/youtube-arm64 ~/panther/youtube/youtube
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/youtube/
[ -f $I/tarjetas-internet/youtube.png ] && cp $I/tarjetas-internet/youtube.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/youtube
cp $I/Youtube.sh /roms/youtube/Youtube.sh
chmod 755 /roms/youtube/Youtube.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
