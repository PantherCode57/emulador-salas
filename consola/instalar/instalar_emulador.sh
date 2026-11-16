#!/bin/bash
# Instala en la consola el emulador de Panther OS y el WhatsApp siempre conectado.
# Se lanza desde ~/panther-instalar con estos archivos al lado:
#   emulador, selawk.ttf, selawksb.ttf     -> ~/panther/emulador
#   whatsapp, WhatsApp.sh                  -> ~/panther/whatsapp y /roms/whatsapp
#   panther-whatsapp.service               -> /etc/systemd/system
# No reinicia el menú (hay que hacerlo después, sin nada abierto).
set -e
cd "$(dirname "$0")"

# El emulador y su letra
mkdir -p /home/ark/panther/emulador
for f in emulador selawk.ttf selawksb.ttf; do
	[ -e "$f" ] && cp "$f" /home/ark/panther/emulador/
done
chmod +x /home/ark/panther/emulador/emulador

# GB, GBC y GBA se abren con él (copia del menú anterior en es_systems.cfg.antes-emulador)
sudo cp -n /etc/emulationstation/es_systems.cfg /etc/emulationstation/es_systems.cfg.antes-emulador
python3 - <<'EOF'
import re
p = "/etc/emulationstation/es_systems.cfg"
x = open(p, encoding="utf-8").read()
cmd = "<command>sudo perfmax %GOVERNOR% %ROM%; nice -n -19 /home/ark/panther/emulador/emulador %ROM%; sudo perfnorm</command>"
for s in ["gb", "gbc", "gba"]:
    def cambiar(m):
        b = re.sub(r"<command>.*?</command>", lambda _: cmd, m.group(0), flags=re.S)
        return re.sub(r"\s*<emulators>.*?</emulators>", "", b, flags=re.S)
    x, n = re.subn(r"<system>\s*<name>%s</name>.*?</system>" % s, cambiar, x, count=1, flags=re.S)
    print(s, "listo" if n else "no está")
open(p, "w", encoding="utf-8").write(x)
EOF

# WhatsApp: el programa (ahora también servicio) y su arranque
if [ -e whatsapp ]; then
	sudo systemctl stop panther-whatsapp 2>/dev/null || true
	cp whatsapp /home/ark/panther/whatsapp/whatsapp.nuevo
	chmod +x /home/ark/panther/whatsapp/whatsapp.nuevo
	mv -f /home/ark/panther/whatsapp/whatsapp.nuevo /home/ark/panther/whatsapp/whatsapp
fi
[ -e WhatsApp.sh ] && cp WhatsApp.sh /roms/whatsapp/WhatsApp.sh && chmod +x /roms/whatsapp/WhatsApp.sh
if [ -e panther-whatsapp.service ]; then
	sudo cp panther-whatsapp.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable panther-whatsapp
	sudo systemctl restart panther-whatsapp
fi
echo "instalado"
