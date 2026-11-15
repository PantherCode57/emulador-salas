#!/usr/bin/env python3
"""Tarjetas opacas en el carrusel de EmulationStation (dArkOS4Clone 20261008).

SystemView::renderCarousel pinta los apartados que no están elegidos al 50 % como mínimo:
    opacity = Math::max(0x80, round(0x80 + 0x7F * (1 - |distancia|)))
y ese 0x80 no se puede cambiar desde el tema. Este script cambia, en el programa, la instrucción
`mov w0, #0x80` (justo antes de la llamada a Math::max) por `mov w0, #0xff`: todas opacas.
Comprueba antes los bytes exactos y guarda una copia del original
(emulationstation.panther-original). Con --deshacer vuelve a poner el original.
Se lanza con sudo y con EmulationStation parado.
"""
import os
import shutil
import sys

ES = "/usr/bin/emulationstation/emulationstation"
COPIA = ES + ".panther-original"
DIRECCION = 0x641368                     # dentro de SystemView::renderCarousel
ANTES = bytes.fromhex("00108052")         # mov w0, #0x80
DESPUES = bytes.fromhex("e01f8052")       # mov w0, #0xff


def desplazamiento(datos, va):
    import struct
    phoff = struct.unpack_from("<Q", datos, 0x20)[0]
    tam, num = struct.unpack_from("<HH", datos, 0x36)
    for i in range(num):
        tipo, _, off, vaddr, _, filesz = struct.unpack_from("<IIQQQQ", datos, phoff + i * tam)
        if tipo == 1 and vaddr <= va < vaddr + filesz:
            return va - vaddr + off
    raise SystemExit("no encuentro la dirección en el programa")


if "--deshacer" in sys.argv:
    if os.path.exists(COPIA):
        shutil.copy2(COPIA, ES + ".nuevo")
        os.replace(ES + ".nuevo", ES)
        print("puesto el original")
    sys.exit(0)

datos = bytearray(open(ES, "rb").read())
off = desplazamiento(datos, DIRECCION)
actual = bytes(datos[off:off + 4])
if actual == DESPUES:
    print("ya estaba cambiado")
    sys.exit(0)
if actual != ANTES:
    raise SystemExit(f"en 0x{off:x} hay {actual.hex()} y no {ANTES.hex()}: es otra versión, no toco nada")
if not os.path.exists(COPIA):
    shutil.copy2(ES, COPIA)
datos[off:off + 4] = DESPUES
nuevo = ES + ".nuevo"
with open(nuevo, "wb") as f:
    f.write(datos)
shutil.copymode(ES, nuevo)
os.replace(nuevo, ES)
print(f"cambiado en 0x{off:x}: {ANTES.hex()} -> {DESPUES.hex()} (copia en {COPIA})")
