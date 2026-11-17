"""Hace una ROM mínima de GBA para probar la app: pone la pantalla en modo 3 y la llena de colores.

Es código ARM escrito a mano (no es ningún juego):
    mov r0, #0x04000000 ; mov r1, #0x400 ; orr r1, r1, #3 ; strh r1, [r0]   (modo 3 con el fondo 2)
    mov r0, #0x06000000 ; mov r2, #0x9600                                   (240×160 píxeles)
    bucle: strh r2, [r0], #2 ; subs r2, r2, #1 ; bne bucle ; b .
mGBA la reconoce como juego de GBA por el 0xEA del byte 3 (el salto) y el 0x96 de 0xB2.
"""
import struct
import sys

rom = bytearray(0x400)
struct.pack_into("<I", rom, 0, 0xEA00002E)  # b 0xC0
rom[0xA0:0xAC] = b"PANTHERTEST "
rom[0xAC:0xB0] = b"PNTR"
rom[0xB0:0xB2] = b"00"
rom[0xB2] = 0x96
codigo = [
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
