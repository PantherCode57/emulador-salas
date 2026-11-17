#!/bin/bash
# Instagram en la consola: el programa, su tarjeta en la pantalla principal y su apartado en el menú.
# Se lanza como ark con lo de consola/instalar y el programa compilado (instagram-arm64) en
# ~/panther-instalar. Con Instagram la pantalla principal tiene 7 tarjetas (tema.py las hace un poco
# más pequeñas para que quepan).
set -e
I=~/panther-instalar
mkdir -p ~/panther/instagram
install -m 755 $I/instagram-arm64 ~/panther/instagram/instagram
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/instagram/
[ -f $I/tarjetas-internet/instagram.png ] && cp $I/tarjetas-internet/instagram.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/instagram
cp $I/Instagram.sh /roms/instagram/Instagram.sh
chmod 755 /roms/instagram/Instagram.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
