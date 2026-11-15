"""Graba la imagen de Pokemulador USB en una memoria USB desde Windows (hace falta administrador).

    python grabar_usb.py <pokemulador-usb.img> <número de disco> <tamaño en bytes> [letra]

1. Comprueba que el disco es una memoria USB y que su tamaño es exactamente el indicado (para no
   equivocarse de disco).
2. La borra (diskpart clean) y escribe la imagen.
3. Mueve la copia de la tabla GPT al final de la memoria, para que quede sitio libre detrás.
4. Crea la parte JUEGOS (exFAT) con todo el espacio que queda y le pone la letra indicada.
Todo lo que pasa queda en grabar_usb.txt, junto a este archivo.
"""
import json
import os
import struct
import subprocess
import sys
import tempfile
import time
import zlib

REGISTRO = os.path.join(os.path.dirname(os.path.abspath(__file__)), "grabar_usb.txt")


def anotar(*cosas):
    linea = time.strftime("%H:%M:%S ") + " ".join(str(c) for c in cosas)
    print(linea, flush=True)
    with open(REGISTRO, "a", encoding="utf-8") as f:
        f.write(linea + "\n")


def diskpart(ordenes):
    with tempfile.NamedTemporaryFile("w", suffix=".txt", delete=False) as f:
        f.write("\n".join(ordenes) + "\n")
        ruta = f.name
    r = subprocess.run(["diskpart", "/s", ruta], capture_output=True, text=True, errors="replace")
    os.unlink(ruta)
    anotar("diskpart", ordenes, "->", r.returncode, r.stdout[-600:].strip().replace("\n", " | "))
    if r.returncode != 0:
        raise SystemExit("diskpart falló")


def disco(numero):
    ps = f"Get-Disk -Number {numero} | Select-Object FriendlyName,BusType,Size,IsBoot,IsSystem | ConvertTo-Json"
    r = subprocess.run(["powershell", "-NoProfile", "-Command", ps], capture_output=True, text=True)
    return json.loads(r.stdout)


def arreglar_gpt(dispositivo, total):
    """Pone la copia de la cabecera y de la tabla GPT al final del disco (como sgdisk -e)."""
    with open(dispositivo, "r+b", buffering=0) as d:
        d.seek(0)
        mbr = bytearray(d.read(512))
        cab = bytearray(d.read(512))
        if cab[:8] != b"EFI PART":
            raise SystemExit("la imagen no tiene tabla GPT")
        tam_cab, = struct.unpack_from("<I", cab, 12)
        lba_tabla, n_entradas, tam_entrada = struct.unpack_from("<QII", cab, 72)
        bytes_tabla = n_entradas * tam_entrada
        sectores_tabla = (bytes_tabla + 511) // 512
        d.seek(lba_tabla * 512)
        tabla = d.read(sectores_tabla * 512)

        def con_crc(c):
            struct.pack_into("<I", c, 16, 0)
            struct.pack_into("<I", c, 16, zlib.crc32(bytes(c[:tam_cab])) & 0xFFFFFFFF)
            return c

        ultimo_util = total - 1 - sectores_tabla - 1
        principal = bytearray(cab)
        struct.pack_into("<Q", principal, 32, total - 1)  # dónde está la copia
        struct.pack_into("<Q", principal, 48, ultimo_util)
        con_crc(principal)
        copia = bytearray(principal)
        struct.pack_into("<Q", copia, 24, total - 1)  # esta cabecera
        struct.pack_into("<Q", copia, 32, 1)  # la otra
        struct.pack_into("<Q", copia, 72, total - 1 - sectores_tabla)  # su tabla
        con_crc(copia)
        # MBR protector: que cubra todo el disco.
        struct.pack_into("<I", mbr, 0x1BE + 12, min(total - 1, 0xFFFFFFFF))

        d.seek(0)
        d.write(bytes(mbr))
        d.write(bytes(principal))
        d.seek((total - 1 - sectores_tabla) * 512)
        d.write(tabla)
        d.seek((total - 1) * 512)
        d.write(bytes(copia))
    anotar("GPT movida al final: último sector útil", ultimo_util)


def main():
    imagen, numero, tam_esperado = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
    letra = sys.argv[4] if len(sys.argv) > 4 else "E"
    info = disco(numero)
    anotar("disco", numero, info)
    if info.get("BusType") != "USB" or int(info.get("Size", 0)) != tam_esperado or info.get("IsBoot") or info.get("IsSystem"):
        raise SystemExit("ese disco no es la memoria USB esperada: no toco nada")
    tam_imagen = os.path.getsize(imagen)
    if tam_imagen % 512 or tam_imagen > tam_esperado:
        raise SystemExit("la imagen no cabe o no tiene un tamaño válido")

    diskpart([f"select disk {numero}", "clean"])
    time.sleep(2)
    dispositivo = rf"\\.\PhysicalDrive{numero}"
    bloque = 4 << 20
    escrito = 0
    inicio = time.time()
    with open(imagen, "rb") as origen, open(dispositivo, "r+b", buffering=0) as destino:
        while True:
            datos = origen.read(bloque)
            if not datos:
                break
            if len(datos) % 512:
                datos += b"\0" * (512 - len(datos) % 512)
            destino.write(datos)
            escrito += len(datos)
            if escrito % (256 << 20) == 0:
                anotar(f"escrito {escrito >> 20} MiB de {tam_imagen >> 20}")
    anotar(f"imagen escrita: {escrito >> 20} MiB en {time.time() - inicio:.0f} s")

    arreglar_gpt(dispositivo, tam_esperado // 512)
    time.sleep(1)
    diskpart(["rescan"])
    time.sleep(3)
    diskpart([f"select disk {numero}", "create partition primary", "format fs=exfat label=JUEGOS quick", f"assign letter={letra}"])
    anotar("hecho")


if __name__ == "__main__":
    try:
        main()
    except SystemExit as e:
        anotar("PARADO:", e)
        raise
    except Exception as e:
        anotar("ERROR:", repr(e))
        raise
