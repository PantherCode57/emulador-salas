"""Una ROM mínima de GBA que guarda partida, para probar que Pokemulador no la pierde.

Código ARM escrito a mano (no es ningún juego): al arrancar suma 1 al primer byte de la partida (SRAM,
0x0E000000) y llena la pantalla de colores, como panther-android/prueba/rom_gba.py. Así se ve cuántas
veces ha empezado el juego desde cero mirando el primer byte del .srm (la SRAM nueva está a 0xFF).
"""
import struct
import sys

rom = bytearray(0x400)
struct.pack_into("<I", rom, 0, 0xEA00002E)  # b 0xC0
rom[0xA0:0xAC] = b"PARTIDATEST "
rom[0xAC:0xB0] = b"PRTD"
rom[0xB0:0xB2] = b"00"
rom[0xB2] = 0x96
codigo = [
    0xE3A0040E,  # mov r0, #0x0E000000
    0xE5D01000,  # ldrb r1, [r0]
    0xE2811001,  # add r1, r1, #1
    0xE5C01000,  # strb r1, [r0]
    0xE3A00404, 0xE3A01B01, 0xE3811003, 0xE1C010B0,
    0xE3A00406, 0xE3A02C96,
    0xE0C020B2, 0xE2522001, 0x1AFFFFFC, 0xEAFFFFFE,
]
for i, ins in enumerate(codigo):
    struct.pack_into("<I", rom, 0xC0 + 4 * i, ins)
suma = 0
for b in rom[0xA0:0xBD]:
    suma = (suma - b) & 0xFF
rom[0xBD] = (suma - 0x19) & 0xFF
open(sys.argv[1], "wb").write(rom)
