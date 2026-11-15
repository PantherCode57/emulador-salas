#!/bin/bash
# Instala en la consola (Panther OS / dArkOS4Clone) lo que hay en ~/panther-instalar:
# WhatsApp, su apartado en el menú, el tema de WhatsApp y Salas, la animación de arranque y los
# menús en español. Se lanza como el usuario ark (usa sudo donde hace falta).
set -e
I="$HOME/panther-instalar"
TEMA=/roms/themes/es-theme-nes-box

echo "== Núcleo de la pantalla (se compila aquí) y WhatsApp"
mkdir -p "$HOME/panther/whatsapp"
gcc -O2 -fPIC -shared -fvisibility=hidden -Wall -std=c11 -D_DEFAULT_SOURCE \
	-o "$HOME/panther/whatsapp/pokemulador_libretro.so" "$I/whatsapp/pokemulador_libretro.c"
install -m 755 "$I/whatsapp/whatsapp" "$HOME/panther/whatsapp/whatsapp"

echo "== Tema: WhatsApp y Salas"
for s in whatsapp salas; do
	mkdir -p "$TEMA/$s"
	cp "$I/tema/$s/"*.png "$TEMA/$s/"
	cp "$TEMA/gba/theme.xml" "$TEMA/$s/theme.xml"
done

echo "== Animación de arranque"
sudo install -m 755 "$I/panther-arranque" /usr/local/bin/panther-arranque
sudo mkdir -p /usr/local/share/panther/arranque
sudo cp "$I/arranque/"* /usr/local/share/panther/arranque/
sudo cp "$I/panther-arranque.service" /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable panther-arranque.service
sudo systemctl disable welcome-message.service 2>/dev/null || true

echo "== Menús en español y apartados nuevos (con EmulationStation parado)"
sudo systemctl stop emulationstation
sleep 1
sudo python3 "$I/menus.py"
cp "$I/WhatsApp.sh" /roms/whatsapp/WhatsApp.sh
chmod 755 /roms/whatsapp/WhatsApp.sh 2>/dev/null || true
for s in gb gbc gba n64 nds psp; do
	[ -f "$I/gamelists/$s.xml" ] && cp "$I/gamelists/$s.xml" "/roms/$s/gamelist.xml"
done

echo "== Prueba de la animación"
sudo /usr/local/bin/panther-arranque || echo "la animación ha fallado"

sudo systemctl start emulationstation
echo "== Hecho"
