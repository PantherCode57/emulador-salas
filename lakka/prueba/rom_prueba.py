"""Hace una ROM mínima de Game Boy (un bucle vacío) para probar que se abre un juego."""
import sys

rom = bytearray(32768)
rom[0x100:0x104] = bytes([0x00, 0xC3, 0x50, 0x01])  # nop; jp $0150
rom[0x150:0x152] = bytes([0x18, 0xFE])  # jr -2
rom[0x134:0x13E] = b"PKMPRUEBA "
x = 0
for i in range(0x134, 0x14D):
    x = (x - rom[i] - 1) & 0xFF
rom[0x14D] = x
open(sys.argv[1], "wb").write(rom)
