#!/bin/bash
# Pokédex en la consola por SSH (como ark). No suele hacer falta: se pone desde la Panther Store.
# Esto hace lo mismo con lo de consola/instalar en ~/panther-instalar: pone la Panther Store nueva
# (tienda-arm64, que ya sabe dar de alta la Pokédex en el menú), y con ella el programa, el núcleo
# de la pantalla, Pokedex.sh, su tarjeta y su apartado; luego, con EmulationStation parado, los
# ajustes del menú (ajustes_menu.py: que se abra directamente y su sitio después de Pokémon).
set -e
I=~/panther-instalar
# Si hay una app o un juego abierto, no: al parar EmulationStation se cerraría con él.
ES=$(pgrep -f "^/usr/bin/emulationstation/emulationstation( |$)" | head -n 1 || true)
if [ -n "$ES" ] && [ -n "$(pgrep -P "$ES")" ]; then
	echo "Hay algo abierto en la consola: ciérralo y vuelve a lanzar esto."
	exit 1
fi
if [ -f $I/tienda-arm64 ]; then
	mkdir -p ~/panther/tienda
	install -m 755 $I/tienda-arm64 ~/panther/tienda/tienda
fi
~/panther/tienda/tienda -poner pokedex
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
python3 $I/ajustes_menu.py
