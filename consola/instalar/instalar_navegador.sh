#!/bin/bash
# Navegador en la consola (un lector de páginas web): el programa, su tarjeta en la pantalla
# principal y su apartado en el menú. Se lanza como ark con lo de consola/instalar y el programa
# compilado (navegador-arm64) en ~/panther-instalar.
set -e
I=~/panther-instalar
mkdir -p ~/panther/navegador
install -m 755 $I/navegador-arm64 ~/panther/navegador/navegador
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/navegador/
[ -f $I/tarjetas-internet/navegador.png ] && cp $I/tarjetas-internet/navegador.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/navegador
cp $I/Navegador.sh /roms/navegador/Navegador.sh
chmod 755 /roms/navegador/Navegador.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
