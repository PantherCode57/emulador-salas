#!/usr/bin/env python3
"""Menús de Panther OS en EmulationStation (dArkOS4Clone). Se ejecuta en la consola con sudo y con
EmulationStation parado (si no, al cerrarse guardaría sus ajustes encima).

- Nombres cortos en español para los sistemas que se usan.
- Apartados propios «WhatsApp» y «Salas» (carpetas /roms/whatsapp y /roms/salas, con scripts .sh).
- Idioma de los menús: español.
Antes de cambiar nada guarda una copia (*.panther-original) de cada archivo, una sola vez.
"""
import re
import shutil
from pathlib import Path

NOMBRES = {
    "gb": "Game Boy",
    "gbc": "Game Boy Color",
    "gba": "GBA",
    "n64": "Nintendo 64",
    "nds": "Nintendo DS",
    "ppsspp": "PSP",  # el sistema de /roms/psp se llama así en dArkOS
}

NUEVOS = """	<system>
		<name>whatsapp</name>
		<fullname>WhatsApp</fullname>
		<path>/roms/whatsapp/</path>
		<extension>.sh .SH</extension>
		<command>%ROM%</command>
		<platform>whatsapp</platform>
		<theme>whatsapp</theme>
		<manufacturer>Panther OS</manufacturer>
		<release>2026</release>
	</system>
	<system>
		<name>salas</name>
		<fullname>Salas</fullname>
		<path>/roms/salas/</path>
		<extension>.sh .SH</extension>
		<command>sudo perfmax %GOVERNOR% %ROM%; nice -n -19 %ROM%; sudo perfnorm</command>
		<platform>salas</platform>
		<theme>salas</theme>
		<manufacturer>Pokemulador</manufacturer>
		<release>2026</release>
	</system>
"""


def copia(ruta: Path):
    orig = ruta.with_name(ruta.name + ".panther-original")
    if ruta.exists() and not orig.exists():
        shutil.copy2(ruta, orig)


def sistemas(ruta: Path):
    if not ruta.exists():
        return
    copia(ruta)
    t = ruta.read_text(encoding="utf-8", errors="replace")

    def cambiar(m):
        bloque = m.group(0)
        nombre = re.search(r"<name>(.*?)</name>", bloque).group(1).strip()
        if nombre in NOMBRES:
            bloque = re.sub(r"<fullname>.*?</fullname>", f"<fullname>{NOMBRES[nombre]}</fullname>", bloque, count=1)
        return bloque

    t = re.sub(r"<system>.*?</system>", cambiar, t, flags=re.S)
    if "<name>whatsapp</name>" not in t:
        t, n = re.subn(r"(<systemList[^>]*>[ \t]*\r?\n)", lambda m: m.group(1) + NUEVOS, t, count=1)
        if n == 0:
            raise SystemExit(f"no encuentro <systemList> en {ruta}")
    ruta.write_text(t, encoding="utf-8")
    print("menús:", ruta)


def idioma(ruta: Path):
    copia(ruta)
    t = ruta.read_text(encoding="utf-8") if ruta.exists() else '<?xml version="1.0"?>\n'
    if 'name="Language"' in t:
        t = re.sub(r'<string name="Language" value="[^"]*" />', '<string name="Language" value="es" />', t)
    else:
        t = t.rstrip("\n") + '\n<string name="Language" value="es" />\n'
    ruta.write_text(t, encoding="utf-8")
    print("idioma:", ruta)


for r in ("/etc/emulationstation/es_systems.cfg", "/etc/emulationstation/es_systems.sd1"):
    sistemas(Path(r))
idioma(Path("/home/ark/.emulationstation/es_settings.cfg"))
for c in ("/roms/whatsapp", "/roms/salas"):
    Path(c).mkdir(exist_ok=True)
