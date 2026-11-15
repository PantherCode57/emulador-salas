"""Prepara el código de mGBA 0.10.5 para el núcleo libretro de Pokemulador.

Uso: python3 aplicar.py <carpeta de mgba>

- El núcleo libretro de mGBA se compila sin scripts; aquí se le añaden las fuentes de scripting
  (las mismas que usa la ventana de scripts de mGBA) y Lua.
- libretro.c carga <sistema>/pokemulador/multijugador.lua al abrir un juego de GBA, si existe
  (ver pokemulador_scripts.inc). Es lo que usa la partida compartida de GBA-PK.
"""
import pathlib
import shutil
import sys

AQUI = pathlib.Path(__file__).resolve().parent
MGBA = pathlib.Path(sys.argv[1]).resolve()


def cambiar(ruta, viejo, nuevo, veces=1):
    texto = ruta.read_text(encoding="utf-8")
    if texto.count(viejo) != veces:
        sys.exit(f"No encuentro el sitio a cambiar en {ruta.name} ({texto.count(viejo)} veces):\n{viejo}")
    ruta.write_text(texto.replace(viejo, nuevo), encoding="utf-8", newline="\n")


# --- CMakeLists.txt: el núcleo libretro con scripts y Lua ---------------------------------------
cmake = MGBA / "CMakeLists.txt"
cambiar(
    cmake,
    "\tadd_library(${BINARY_NAME}_libretro SHARED ${CORE_SRC} ${RETRO_SRC})\n",
    "\tif(ENABLE_SCRIPTING)\n"
    "\t\t# Pokemulador: el núcleo libretro también ejecuta scripts de Lua (partida compartida de GBA-PK).\n"
    "\t\tlist(APPEND RETRO_SRC ${SCRIPT_SRC} ${CORE_SCRIPT_SRC})\n"
    "\tendif()\n"
    "\tadd_library(${BINARY_NAME}_libretro SHARED ${CORE_SRC} ${RETRO_SRC})\n",
)
cambiar(
    cmake,
    '${OS_DEFINES};${FUNCTION_DEFINES};MINIMAL_CORE=2")\n'
    "\ttarget_link_libraries(${BINARY_NAME}_libretro ${OS_LIB})\n",
    '${OS_DEFINES};${FUNCTION_DEFINES};${FEATURE_DEFINES};MINIMAL_CORE=2")\n'
    "\ttarget_link_libraries(${BINARY_NAME}_libretro ${DEPENDENCY_LIB} ${OS_LIB})\n",
)

# --- scripting.c: funciones de archivo que el núcleo libretro no tiene --------------------------
# El núcleo libretro se compila con MINIMAL_CORE=2, sin cargar archivos, guardados rápidos por
# número ni capturas (eso lo hace el programa que lo usa). Los scripts de mGBA las ofrecen, pero
# GBA-PK no las usa: aquí no hacen nada.
cambiar(
    MGBA / "src" / "core" / "scripting.c",
    "#include <mgba-util/vfs.h>\n",
    "#include <mgba-util/vfs.h>\n"
    "\n"
    "#if defined(MINIMAL_CORE) && MINIMAL_CORE >= 2\n"
    "// Pokemulador: el núcleo libretro no tiene estas funciones de archivo.\n"
    "static bool mCoreLoadFile(struct mCore* core, const char* path) { UNUSED(core); UNUSED(path); return false; }\n"
    "static bool mCoreAutoloadSave(struct mCore* core) { UNUSED(core); return false; }\n"
    "static bool mCoreLoadSaveFile(struct mCore* core, const char* path, bool temporary) {\n"
    "\tUNUSED(core); UNUSED(path); UNUSED(temporary); return false;\n"
    "}\n"
    "static bool mCoreSaveState(struct mCore* core, int slot, int flags) { UNUSED(core); UNUSED(slot); UNUSED(flags); return false; }\n"
    "static bool mCoreLoadState(struct mCore* core, int slot, int flags) { UNUSED(core); UNUSED(slot); UNUSED(flags); return false; }\n"
    "static void mCoreTakeScreenshot(struct mCore* core) { UNUSED(core); }\n"
    "static bool mCoreTakeScreenshotVF(struct mCore* core, struct VFile* vf) { UNUSED(core); UNUSED(vf); return false; }\n"
    "#endif\n",
)

# --- libretro.c: cargar el script al abrir el juego ---------------------------------------------
libretro = MGBA / "src" / "platform" / "libretro" / "libretro.c"
shutil.copy(AQUI / "pokemulador_scripts.inc", libretro.parent / "pokemulador_scripts.inc")
cambiar(libretro, "void retro_init(void) {\n", '#include "pokemulador_scripts.inc"\n\nvoid retro_init(void) {\n')
# Al final de retro_load_game, cuando el juego ya está listo.
cambiar(
    libretro,
    "\tcore->reset(core);\n\t_setupMaps(core);\n\n\treturn true;\n}\n",
    "\tcore->reset(core);\n\t_setupMaps(core);\n\n\t_pkmScriptsStart(sysDir);\n\n\treturn true;\n}\n",
)
cambiar(
    libretro,
    "void retro_unload_game(void) {\n\tif (!core) {\n\t\treturn;\n\t}\n",
    "void retro_unload_game(void) {\n\tif (!core) {\n\t\treturn;\n\t}\n\t_pkmScriptsStop();\n",
)
cambiar(
    libretro,
    "void retro_reset(void) {\n\tcore->reset(core);\n\t_setupMaps(core);\n",
    "void retro_reset(void) {\n\tcore->reset(core);\n\t_setupMaps(core);\n\t_pkmScriptsReset();\n",
)
print("mGBA preparado para Pokemulador")
