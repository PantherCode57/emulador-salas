#!/bin/bash
# Instala en la consola:
#   - el recuadro de arriba a la derecha (batería, hora y WhatsApp) en todas las pantallas menos la
#     principal, y la barra del volumen a la izquierda;
#   - + y − para el volumen siempre (de 5 en 5), que ya no los mueve ogage (de 1 en 1);
#   - L1 + R1 para hacer una captura y L2 + R2 para grabar la pantalla.
# Junto a este script: emulationstation, whatsapp, emulador y captura.
set -e
cd "$(dirname "$0")"

# ogage: + y − pasan al servicio de Panther (SELECT + ←/→ siguen siendo suyos)
cp -n /home/ark/ogage.conf /home/ark/ogage.conf.antes-panther
sed -i -e 's/^VOLUME_UP=.*/VOLUME_UP=KEY_F23/' -e 's/^VOLUME_DOWN=.*/VOLUME_DOWN=KEY_F24/' /home/ark/ogage.conf
sudo systemctl restart ogage
sleep 4
if ! systemctl is-active --quiet ogage; then
	echo "ogage no arranca con el cambio: lo dejo como estaba"
	cp /home/ark/ogage.conf.antes-panther /home/ark/ogage.conf
	sudo systemctl restart ogage
fi

# Programas
poner() { # poner <archivo> <destino>
	[ -e "$1" ] || return 0
	cp "$1" "$2.nuevo" && chmod +x "$2.nuevo" && mv -f "$2.nuevo" "$2"
}
mkdir -p /home/ark/panther/emulador /roms/capturas /roms/videos/grabaciones
poner captura /home/ark/panther/emulador/captura
poner emulador /home/ark/panther/emulador/emulador
sudo systemctl stop panther-whatsapp
poner whatsapp /home/ark/panther/whatsapp/whatsapp

# El menú (copia del anterior en emulationstation.antes-recuadro)
sudo systemctl stop emulationstation
if [ -e emulationstation ]; then
	sudo cp -n /usr/bin/emulationstation/emulationstation /usr/bin/emulationstation/emulationstation.antes-recuadro
	sudo cp emulationstation /usr/bin/emulationstation/emulationstation.nuevo
	sudo chmod +x /usr/bin/emulationstation/emulationstation.nuevo
	sudo mv -f /usr/bin/emulationstation/emulationstation.nuevo /usr/bin/emulationstation/emulationstation
fi

# Sin la ventanita de volumen del menú (la barra de la izquierda la sustituye) y, en las listas, sin
# la hora pequeña (ya va en el recuadro).
python3 - <<'EOF'
import pathlib, re
p = pathlib.Path("/home/ark/.emulationstation/es_settings.cfg")
t = p.read_text(encoding="utf-8")
if '<bool name="VolumePopup"' in t:
    t = re.sub(r'<bool name="VolumePopup" value="[^"]*" />', '<bool name="VolumePopup" value="false" />', t)
else:
    t = t.replace("</config>", '\t<bool name="VolumePopup" value="false" />\n</config>')
p.write_text(t, encoding="utf-8")
for f in pathlib.Path("/roms/themes/es-theme-panther").rglob("*.xml"):
    x = f.read_text(encoding="utf-8")
    if "clock_lista" not in x:
        continue
    x2 = re.sub(r'(<text name="clock_lista">\s*<pos>)[^<]*(</pos>)', r"\g<1>0 2\g<2>", x)
    if x2 != x:
        f.write_text(x2, encoding="utf-8")
        print("tema:", f)
EOF

sudo systemctl start panther-whatsapp
sudo systemctl start emulationstation
echo "instalado"
