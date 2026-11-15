#!/usr/bin/env python3
"""Ajustes de EmulationStation para el tema de Panther OS: tema, hora, batería y Wi-Fi.
Se lanza con EmulationStation parado (si no, al cerrarse guardaría sus ajustes encima)."""
import re
import shutil
from pathlib import Path

ruta = Path("/home/ark/.emulationstation/es_settings.cfg")
orig = ruta.with_name(ruta.name + ".panther-original")
if not orig.exists():
    shutil.copy2(ruta, orig)
t = ruta.read_text(encoding="utf-8")


def poner(tipo, nombre, valor):
    global t
    linea = f'<{tipo} name="{nombre}" value="{valor}" />'
    patron = rf'<{tipo} name="{re.escape(nombre)}" value="[^"]*" />'
    if re.search(patron, t):
        t = re.sub(patron, linea, t)
    else:
        t = t.rstrip("\n") + "\n" + linea + "\n"


poner("string", "ThemeSet", "es-theme-panther")
poner("bool", "DrawClock", "true")
poner("bool", "ClockMode12", "false")
poner("bool", "ShowBatteryIndicator", "true")
# ShowBattery es un texto: "" (nada), "icon" o "text" (icono y porcentaje).
t = re.sub(r'<bool name="ShowBattery" value="[^"]*" />\n?', "", t)
poner("string", "ShowBattery", "text")
poner("bool", "networkIcon", "true")
# Lista de juegos con tarjetas (vista «grid» del tema) solo en los apartados de juegos y en las
# colecciones; Opciones, Herramientas, etc. se quedan con su lista.
for sistema in ("gb", "gbc", "gba", "n64", "nds", "ppsspp", "all", "favorites", "recent"):
    poner("string", f"{sistema}.defaultView", "grid")
ruta.write_text(t, encoding="utf-8")
print("ajustes:", ruta)
