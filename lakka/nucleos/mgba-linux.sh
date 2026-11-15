#!/usr/bin/env bash
# Compila el mGBA de Pokemulador (scripts de Lua para GBA-PK) para Lakka (Linux x86_64).
# Se lanza desde la raíz del repositorio, con el código de mGBA en ./mgba. Deja mgba_libretro.so
# en ./nucleo-mgba-linux. Mismas opciones que nucleos/mgba/compilar.sh (Android).
set -euo pipefail

RAIZ=$(pwd)
LUA_VERSION=${LUA_VERSION:-5.4.7}
python3 nucleos/mgba/aplicar.py mgba

curl -fsSL "https://www.lua.org/ftp/lua-${LUA_VERSION}.tar.gz" -o lua.tar.gz
if [ "$LUA_VERSION" = "5.4.7" ]; then
	echo "9fbf5e28ef86c69858f6d3d34eccc32e911c1a28b4120ff3e84aaa70cfbf1e30  lua.tar.gz" | sha256sum -c -
fi
tar xzf lua.tar.gz
LUA_SRC="$RAIZ/lua-${LUA_VERSION}/src"
mkdir -p lua-linux
(
	cd lua-linux
	for f in $(ls "$LUA_SRC"/*.c | grep -v -E '/(lua|luac)\.c$'); do gcc -O2 -fPIC -DLUA_USE_POSIX -c "$f"; done
	ar rcs liblua.a ./*.o
)

cmake -S mgba -B build-linux -G Ninja \
	-DCMAKE_BUILD_TYPE=Release \
	-DBUILD_LIBRETRO=ON -DSKIP_LIBRARY=ON -DBUILD_QT=OFF -DBUILD_SDL=OFF \
	-DUSE_DEBUGGERS=OFF -DUSE_GDB_STUB=OFF -DUSE_EDITLINE=OFF -DUSE_FFMPEG=OFF -DUSE_ZLIB=OFF \
	-DUSE_MINIZIP=OFF -DUSE_PNG=OFF -DUSE_LIBZIP=OFF -DUSE_SQLITE3=OFF -DUSE_ELF=OFF -DUSE_LZMA=OFF \
	-DUSE_EPOXY=OFF -DUSE_DISCORD_RPC=OFF -DBUILD_GL=OFF -DBUILD_GLES2=OFF -DBUILD_GLES3=OFF \
	-DENABLE_SCRIPTING=ON -DUSE_LUA=5.4 "-DLUA_INCLUDE_DIR=$LUA_SRC" "-DLUA_LIBRARY=$RAIZ/lua-linux/liblua.a" \
	"-DCMAKE_SHARED_LINKER_FLAGS=-static-libgcc"
cmake --build build-linux --target mgba_libretro -j "$(nproc)"
SO=$(find build-linux -name 'mgba_libretro*.so' | head -1)
strip -s "$SO"
mkdir -p nucleo-mgba-linux
cp "$SO" nucleo-mgba-linux/mgba_libretro.so
ls -la nucleo-mgba-linux
