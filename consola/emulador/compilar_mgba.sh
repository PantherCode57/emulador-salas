#!/usr/bin/env bash
# Compila el código de mGBA 0.10.5 (solo la parte que emula GB, GBC y GBA, como biblioteca) y Lua 5.4
# para el emulador de Panther OS. Se lanza desde una carpeta con mgba-0.10.5 y lua-5.4.7 dentro y
# deja en ./mgba-lib:
#   lib/libmgba.a, lib/liblua.a        las bibliotecas
#   include/                           las cabeceras (con mgba/flags.h, que genera cmake)
# Funciona en la consola (Debian 13, arm64) y en la máquina arm64 de GitHub.
set -euo pipefail

RAIZ=$(pwd)
MGBA=${MGBA:-$RAIZ/mgba-0.10.5}
LUA=${LUA:-$RAIZ/lua-5.4.7}
SALIDA=${SALIDA:-$RAIZ/mgba-lib}
# Rápido en el procesador de la consola (Cortex-A35) y sin código que no se usa.
OPT="-O2 -ffunction-sections -fdata-sections -fno-plt"

mkdir -p "$SALIDA/lib" "$SALIDA/include" "$RAIZ/lua-obj"
# Lo que emula está todo en C: sin C++ (la consola no tiene compilador de C++).
sed -i 's/^project(mGBA)$/project(mGBA C)/' "$MGBA/CMakeLists.txt"
(
	cd "$RAIZ/lua-obj"
	for f in $(ls "$LUA"/src/*.c | grep -v -E '/(lua|luac)\.c$'); do
		gcc $OPT -DLUA_USE_POSIX -c "$f"
	done
	rm -f "$SALIDA/lib/liblua.a"
	ar rcs "$SALIDA/lib/liblua.a" ./*.o
)

cmake -S "$MGBA" -B "$RAIZ/build-mgba" -DCMAKE_BUILD_TYPE=Release \
	"-DCMAKE_C_FLAGS_RELEASE=$OPT -DNDEBUG" \
	-DBUILD_SHARED=OFF -DBUILD_STATIC=ON -DBUILD_SDL=OFF -DBUILD_QT=OFF -DBUILD_LIBRETRO=OFF \
	-DBUILD_PERF=OFF -DBUILD_TEST=OFF -DBUILD_SUITE=OFF -DBUILD_CINEMA=OFF -DBUILD_EXAMPLE=OFF \
	-DBUILD_PYTHON=OFF -DBUILD_HEADLESS=OFF -DBUILD_GL=OFF -DBUILD_GLES2=OFF -DBUILD_GLES3=OFF \
	-DUSE_DEBUGGERS=OFF -DUSE_GDB_STUB=OFF -DUSE_EDITLINE=OFF -DUSE_FFMPEG=OFF -DUSE_PNG=OFF \
	-DUSE_ZLIB=ON -DUSE_MINIZIP=OFF -DUSE_LIBZIP=OFF -DUSE_LZMA=ON -DUSE_SQLITE3=OFF -DUSE_ELF=OFF \
	-DUSE_EPOXY=OFF -DUSE_DISCORD_RPC=OFF \
	-DENABLE_SCRIPTING=ON -DUSE_LUA=5.4 "-DLUA_INCLUDE_DIR=$LUA/src" "-DLUA_LIBRARY=$SALIDA/lib/liblua.a"
# Sin biblioteca compartida, el objetivo «mgba» es la estática (libmgba.a).
cmake --build "$RAIZ/build-mgba" --target mgba -j "$(nproc)"

cp "$(find "$RAIZ/build-mgba" -name 'libmgba.a' | head -1)" "$SALIDA/lib/"
cp -r "$MGBA/include/." "$SALIDA/include/"
cp -r "$RAIZ/build-mgba/include/." "$SALIDA/include/"
cp "$LUA"/src/lua.h "$LUA"/src/luaconf.h "$LUA"/src/lualib.h "$LUA"/src/lauxlib.h "$SALIDA/include/"
ls -la "$SALIDA/lib"
