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
poner("string", "PantherDirectLaunch", "whatsapp,spotin,pokemon,youtube,salas")
poner("string", "PantherWifiBackground", "/roms/themes/es-theme-panther/_panther/fondo-lista.jpg")
# Pantalla principal: solo Favoritos, WhatsApp y Emuladores (las consolas), en ese orden y quietas.
# Fuera RetroArch (las opciones están en el engranaje), «Todos» y lo que trae el sistema que no son
# juegos del usuario (PICO-8 de prueba, ScummVM, Java, Wolfenstein, ALG y PyMO).
m = re.search(r'<string name="HiddenSystems" value="([^"]*)" />', t)
ocultos = [x for x in (m.group(1).split(";") if m else []) if x]
for s in ("retroarch", "pico-8", "scummvm", "j2me", "wolf", "alg", "pymo"):
    if s not in ocultos:
        ocultos.append(s)
poner("string", "HiddenSystems", ";".join(ocultos))
# Sin colecciones automáticas en la pantalla principal (el usuario quitó «Favoritos» el 16/11/2026;
# las marcas de favorito de los juegos se quedan).
poner("string", "CollectionSystemsAuto", "")
poner("string", "PantherSystemOrder", "whatsapp,youtube,spotin,pokemon,Emuladores,salas")
poner("string", "Emuladores.defaultView", "grid")
# Carpeta de piezas del tema para las pantallas propias (letra, fondo, bordes).
poner("string", "PantherPiezas", "/roms/themes/es-theme-panther/_panther")
ruta.write_text(t, encoding="utf-8")
print("ajustes:", ruta)
