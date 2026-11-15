#!/usr/bin/env python3
"""Pone a cada juego su tarjeta de la lista (<thumbnail>): la imagen ya recortada, con las esquinas
redondas y la sombra, que está en /roms/<apartado>/images/panther/ con el mismo nombre que su
<image>. Se lanza con EmulationStation parado (si no, al cerrarse guardaría sus listas encima).
Antes de cambiar una lista guarda una copia (gamelist.xml.panther-original), una sola vez."""
import html
import re
import shutil
from pathlib import Path

for lista in sorted(Path("/roms").glob("*/gamelist.xml")):
    carpeta = lista.parent
    if not (carpeta / "images" / "panther").is_dir():
        continue
    t = lista.read_text(encoding="utf-8", errors="replace")
    cambios = 0

    def juego(m):
        global cambios
        bloque = m.group(0)
        img = re.search(r"<image>\./images/([^<]+)</image>", bloque)
        # en el XML va escapado («&amp;»); el archivo, no
        if not img or not (carpeta / "images" / "panther" / html.unescape(img.group(1))).exists():
            return bloque
        nueva = f"<thumbnail>./images/panther/{img.group(1)}</thumbnail>"
        if "<thumbnail>" in bloque:
            bloque2 = re.sub(r"<thumbnail>[^<]*</thumbnail>", nueva, bloque, count=1)
        else:
            bloque2 = bloque.replace(img.group(0), img.group(0) + "\n\t\t" + nueva, 1)
        if bloque2 != bloque:
            cambios += 1
        return bloque2

    t2 = re.sub(r"<game>.*?</game>", juego, t, flags=re.S)
    if t2 != t:
        copia = lista.with_name(lista.name + ".panther-original")
        if not copia.exists():
            shutil.copy2(lista, copia)
        lista.write_text(t2, encoding="utf-8")
    print(f"{carpeta.name}: {cambios} tarjetas")
