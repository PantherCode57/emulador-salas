#!/bin/bash
# Spotin en la consola: el programa, yt-dlp (de donde sale el sonido), su tarjeta en la pantalla
# principal y su apartado en el menú. Se lanza como ark con lo de consola/instalar y el programa
# compilado (spotin-arm64) en ~/panther-instalar.
set -e
I=~/panther-instalar
mkdir -p ~/panther/spotin/bin
install -m 755 $I/spotin-arm64 ~/panther/spotin/spotin
cp ~/panther/whatsapp/pokemulador_libretro.so ~/panther/spotin/
# yt-dlp, de su página oficial: YouTube ya no da el sonido a los clientes sencillos de Spotin.
if [ ! -x ~/panther/spotin/bin/yt-dlp ]; then
	curl -fsSL -o ~/panther/spotin/bin/yt-dlp https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp
	chmod +x ~/panther/spotin/bin/yt-dlp
fi
[ -f $I/tarjetas-spotin/spotin.png ] && cp $I/tarjetas-spotin/spotin.png $I/tema-panther/tarjetas/
sudo systemctl stop emulationstation
trap 'sudo systemctl start emulationstation' EXIT
sleep 2
sudo python3 $I/menus.py
mkdir -p /roms/spotin
cp $I/Spotin.sh /roms/spotin/Spotin.sh
chmod 755 /roms/spotin/Spotin.sh 2>/dev/null || true
python3 $I/ajustes_menu.py
python3 $I/tema.py $I/tema-panther | tail -n 1
