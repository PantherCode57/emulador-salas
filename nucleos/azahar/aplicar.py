"""Mete las salas de Pokemulador en el código de Azahar antes de compilar su núcleo libretro.

Uso: python aplicar.py <carpeta del código de Azahar>
Falla con un mensaje claro si el código de Azahar ha cambiado y algún punto ya no encaja.
"""
import pathlib
import shutil
import sys

aqui = pathlib.Path(__file__).resolve().parent
azahar = pathlib.Path(sys.argv[1]).resolve()
libretro = azahar / "src" / "citra_libretro"


def cambiar(archivo, buscado, nuevo):
    texto = archivo.read_text(encoding="utf-8")
    if buscado not in texto:
        sys.exit(f"No encuentro el punto a cambiar en {archivo.name}:\n{buscado}")
    archivo.write_text(texto.replace(buscado, nuevo, 1), encoding="utf-8")


for nombre in ("pokemulador_sala.h", "pokemulador_sala.cpp"):
    shutil.copy(aqui / nombre, libretro / nombre)

cmake = libretro / "CMakeLists.txt"
cambiar(cmake, "add_library(citra_libretro SHARED\n        citra_libretro.cpp",
        "add_library(citra_libretro SHARED\n        citra_libretro.cpp\n        pokemulador_sala.cpp")
cambiar(cmake, "target_link_libraries(citra_libretro PRIVATE citra_common citra_core)",
        "target_link_libraries(citra_libretro PRIVATE citra_common citra_core network)")

nucleo = libretro / "citra_libretro.cpp"
cambiar(nucleo, '#include "citra_libretro/environment.h"\n',
        '#include "citra_libretro/environment.h"\n#include "citra_libretro/pokemulador_sala.h"\n')
cambiar(nucleo, "    setup_memory_maps();\n\n    return true;",
        "    setup_memory_maps();\n    PokemuladorSala::Iniciar();\n\n    return true;")
cambiar(nucleo, '    LOG_DEBUG(Frontend, "Unloading game...");\n    Core::System::GetInstance().Shutdown();',
        '    LOG_DEBUG(Frontend, "Unloading game...");\n    PokemuladorSala::Detener();\n'
        '    Core::System::GetInstance().Shutdown();')

print("Salas de Pokemulador añadidas al núcleo de Azahar")
