#!/usr/bin/env python3
"""Ajustes de EmulationStation para el tema de Panther OS: tema, hora, batería, Wi-Fi, apartados
que se abren directamente y apartados ocultos.
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
# Con el menú de Panther OS: WhatsApp y Salas se abren sin pasar por su lista (si tienen un solo
# .sh) y la pantalla de Wi-Fi usa el fondo de las listas.
poner("string", "PantherDirectLaunch", "whatsapp,salas")
poner("string", "PantherWifiBackground", "/roms/themes/es-theme-panther/_panther/fondo-lista.jpg")
# La tarjeta de RetroArch (/opt/cmds) sobra: las opciones del sistema están en el engranaje de arriba.
m = re.search(r'<string name="HiddenSystems" value="([^"]*)" />', t)
ocultos = [x for x in (m.group(1).split(";") if m else []) if x]
if "retroarch" not in ocultos:
    ocultos.append("retroarch")
poner("string", "HiddenSystems", ";".join(ocultos))
ruta.write_text(t, encoding="utf-8")
print("ajustes:", ruta)
