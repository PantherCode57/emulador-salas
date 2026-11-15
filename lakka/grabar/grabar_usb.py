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


def preparar_gpt(cabeza, total):
    """Cambia en el primer MiB de la imagen la tabla GPT para que ocupe todo el disco (como
    sgdisk -e) y devuelve la copia que va al final: (bytes, sector donde empieza)."""
    if cabeza[512:520] != b"EFI PART":
        raise SystemExit("la imagen no tiene tabla GPT")
    cab = bytearray(cabeza[512:1024])
    tam_cab, = struct.unpack_from("<I", cab, 12)
    lba_tabla, n_entradas, tam_entrada = struct.unpack_from("<QII", cab, 72)
    sectores_tabla = (n_entradas * tam_entrada + 511) // 512
    tabla = bytes(cabeza[lba_tabla * 512:(lba_tabla + sectores_tabla) * 512])

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
    cabeza[512:1024] = principal
    # MBR protector: que cubra todo el disco.
    struct.pack_into("<I", cabeza, 0x1BE + 12, min(total - 1, 0xFFFFFFFF))
    anotar("GPT para todo el disco: último sector útil", ultimo_util)
    return tabla + bytes(copia), total - 1 - sectores_tabla


def escribir_imagen(imagen, dispositivo, tam_imagen, total):
    """Escribe la imagen en el disco. El primer MiB (la tabla de particiones) va al final: si
    Windows la ve antes, monta las particiones y ya no deja escribir dentro de ellas."""
    bloque = 4 << 20
    cabeza = 1 << 20
    inicio = time.time()
    disco = DiscoWindows(dispositivo)
    try:
        with open(imagen, "rb") as origen:
            primero = bytearray(origen.read(cabeza))
            cola, lba_cola = preparar_gpt(primero, total)
            disco.escribir(lba_cola * 512, cola)
            posicion = cabeza
            while True:
                datos = origen.read(bloque)
                if not datos:
                    break
                if len(datos) % 512:
                    datos += b"\0" * (512 - len(datos) % 512)
                disco.escribir(posicion, datos)
                posicion += len(datos)
                if posicion % (128 << 20) == 0:
                    anotar(f"escrito {posicion >> 20} MiB de {tam_imagen >> 20}")
            disco.escribir(0, bytes(primero))
    finally:
        disco.cerrar()
    anotar(f"imagen escrita: {tam_imagen >> 20} MiB en {time.time() - inicio:.0f} s")


class DiscoWindows:
    """Escritura directa en un disco con CreateFile/WriteFile (lo que usan los grabadores de USB)."""

    def __init__(self, ruta):
        import ctypes
        from ctypes import wintypes

        self.k = ctypes.WinDLL("kernel32", use_last_error=True)
        self.ctypes = ctypes
        self.k.CreateFileW.restype = wintypes.HANDLE
        self.k.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD, wintypes.LPVOID,
                                       wintypes.DWORD, wintypes.DWORD, wintypes.HANDLE]
        self.k.SetFilePointerEx.argtypes = [wintypes.HANDLE, ctypes.c_longlong, ctypes.POINTER(ctypes.c_longlong), wintypes.DWORD]
        self.k.WriteFile.argtypes = [wintypes.HANDLE, wintypes.LPCVOID, wintypes.DWORD, ctypes.POINTER(wintypes.DWORD), wintypes.LPVOID]
        self.k.FlushFileBuffers.argtypes = [wintypes.HANDLE]
        self.k.CloseHandle.argtypes = [wintypes.HANDLE]
        GENERIC_READ, GENERIC_WRITE = 0x80000000, 0x40000000
        FILE_SHARE_READ, FILE_SHARE_WRITE, OPEN_EXISTING = 1, 2, 3
        FILE_FLAG_WRITE_THROUGH = 0x80000000
        self.h = self.k.CreateFileW(ruta, GENERIC_READ | GENERIC_WRITE, FILE_SHARE_READ | FILE_SHARE_WRITE,
                                    None, OPEN_EXISTING, FILE_FLAG_WRITE_THROUGH, None)
        if self.h in (None, ctypes.c_void_p(-1).value):
            raise SystemExit(f"no se pudo abrir {ruta}: error {ctypes.get_last_error()}")

    def escribir(self, posicion, datos):
        ctypes = self.ctypes
        if not self.k.SetFilePointerEx(self.h, posicion, None, 0):
            raise SystemExit(f"no se pudo ir al byte {posicion}: error {ctypes.get_last_error()}")
        hechos = ctypes.c_ulong(0)
        if not self.k.WriteFile(self.h, datos, len(datos), ctypes.byref(hechos), None) or hechos.value != len(datos):
            raise SystemExit(f"no se pudo escribir en el byte {posicion}: error {ctypes.get_last_error()}")

    def cerrar(self):
        self.k.FlushFileBuffers(self.h)
        self.k.CloseHandle(self.h)


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
    escribir_imagen(imagen, dispositivo, tam_imagen, tam_esperado // 512)
    time.sleep(2)
    diskpart(["rescan"])
    time.sleep(3)
    # La parte del sistema es de tipo EFI (Windows no le da letra); por si acaso, se le quita.
    diskpart([f"select disk {numero}", "select partition 1", "remove noerr", "select disk " + str(numero),
              "create partition primary", "format fs=exfat label=JUEGOS quick", f"assign letter={letra} noerr"])
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
