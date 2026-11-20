#!/bin/bash
# Panther Store en la consola: el programa, su tarjeta «Panther Store» en la pantalla principal y su
# apartado en el menú, y la lista de juegos de PortMaster («Ports») con tarjetas.
# Se lanza como ark con lo de consola/instalar y el programa compilado (tienda-arm64) en
# ~/panther-instalar. La tarjeta la dibuja la propia tienda con las letras del tema (-tarjeta).
set -e
I=~/panther-instalar
# Si hay una app o un juego abierto, no: al parar EmulationStation se cerraría con él.
ES=$(pgrep -f "^/usr/bin/emulationstation/emulationstation( |$)" | head -n 1 || true)
if [ -n "$ES" ] && [ -n "$(pgrep -P "$ES")" ]; then
	echo "Hay algo abierto en la consola: ciérralo y vuelve a lanzar esto."
	exit 1
fi
mkdir -p ~/panther/tienda
install -m 755 $I/tienda-arm64 ~/panther/tienda/tienda
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/tienda/
~/panther/tienda/tienda -tarjeta $I/tema-panther/tarjetas/tienda.png
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/tienda
cp $I/Tienda.sh /roms/tienda/Tienda.sh
chmod 755 /roms/tienda/Tienda.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
