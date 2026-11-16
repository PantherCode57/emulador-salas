#!/usr/bin/env python3
"""Menús de Panther OS en EmulationStation (dArkOS4Clone). Se ejecuta en la consola con sudo y con
EmulationStation parado (si no, al cerrarse guardaría sus ajustes encima).

- Nombres cortos en español para los sistemas que se usan.
- Las consolas, dentro de la tarjeta «Emuladores», con su marca, año y descripción.
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

# Las consolas van dentro de la tarjeta «Emuladores» (grupo de EmulationStation). La lista enseña
# «marca · año · N juegos» y esta descripción (las dos cosas las lee el menú de Panther OS).
EMULADORES = {
    "gb": ("Nintendo", 1989, "La primera portátil de Nintendo. Aquí están Pokémon Rojo, Azul y Amarillo, "
                             "donde empezó todo."),
    "gbc": ("Nintendo", 1998, "La Game Boy con pantalla a color. Aquí están Pokémon Oro, Plata y Cristal, "
                              "el Pinball, el Puzzle Challenge y el juego de cartas."),
    "gba": ("Nintendo", 2001, "La portátil de 32 bits de Nintendo. Aquí están Pokémon Esmeralda, Rubí, Zafiro, "
                              "Rojo Fuego y Verde Hoja, el Mundo Misterioso y el Pinball."),
    "n64": ("Nintendo", 1996, "La consola de Nintendo de los 64 bits. Aquí están Super Mario 64, Pokémon Stadium, "
                              "Pokémon Snap y Pokémon Puzzle League."),
    "nds": ("Nintendo", 2004, "La portátil de dos pantallas de Nintendo. Aquí están Pokémon Negro, Pokémon "
                              "Diamante y Super Mario 64 DS."),
    "ppsspp": ("Sony", 2004, "La portátil de Sony. Aquí está Grand Theft Auto: Vice City Stories."),
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
		<name>spotin</name>
		<fullname>Spotin</fullname>
		<path>/roms/spotin/</path>
		<extension>.sh .SH</extension>
		<command>%ROM%</command>
		<platform>spotin</platform>
		<theme>spotin</theme>
		<manufacturer>Panther OS</manufacturer>
		<release>2026</release>
	</system>
	<system>
		<name>youtube</name>
		<fullname>YouTube</fullname>
		<path>/roms/youtube/</path>
		<extension>.sh .SH</extension>
		<command>%ROM%</command>
		<platform>youtube</platform>
		<theme>youtube</theme>
		<manufacturer>Panther OS</manufacturer>
		<release>2026</release>
	</system>
	<system>
		<name>navegador</name>
		<fullname>Navegador</fullname>
		<path>/roms/navegador/</path>
		<extension>.sh .SH</extension>
		<command>%ROM%</command>
		<platform>navegador</platform>
		<theme>navegador</theme>
		<manufacturer>Panther OS</manufacturer>
		<release>2026</release>
	</system>
	<system>
		<name>pokemon</name>
		<fullname>Pokémon</fullname>
		<path>/roms/pokemon/</path>
		<extension>.sh .SH</extension>
		<command>%ROM%</command>
		<platform>pokemon</platform>
		<theme>pokemon</theme>
		<manufacturer>Pokemulador</manufacturer>
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
        if nombre in EMULADORES:
            marca, anio, texto = EMULADORES[nombre]
            bloque = re.sub(r"\s*<(group|manufacturer|release|description)>.*?</\1>", "", bloque, flags=re.S)
            extra = (f"\n\t\t<group>Emuladores</group>\n\t\t<manufacturer>{marca}</manufacturer>"
                     f"\n\t\t<release>{anio}</release>\n\t\t<description>{texto}</description>")
            bloque = re.sub(r"(</fullname>)", lambda m: m.group(1) + extra, bloque, count=1)
        return bloque

    t = re.sub(r"<system>.*?</system>", cambiar, t, flags=re.S)
    # Los apartados nuevos que falten, cada uno por separado (Spotin llegó después que los otros).
    faltan = ""
    for bloque in re.findall(r"\t<system>.*?</system>\n", NUEVOS, flags=re.S):
        nombre = re.search(r"<name>(.*?)</name>", bloque).group(1)
        if f"<name>{nombre}</name>" not in t:
            faltan += bloque
    if faltan:
        t, n = re.subn(r"(<systemList[^>]*>[ \t]*\r?\n)", lambda m: m.group(1) + faltan, t, count=1)
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
for c in ("/roms/whatsapp", "/roms/salas", "/roms/spotin", "/roms/pokemon", "/roms/youtube", "/roms/navegador"):
    Path(c).mkdir(exist_ok=True)
