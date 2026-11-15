#!/usr/bin/env bash
# Compila el núcleo libretro de mGBA de Pokemulador (con scripts de Lua) para Android (arm64) y
# para Windows (x64). Se lanza desde la raíz del repositorio, con el código de mGBA en ./mgba.
# Deja los dos archivos en ./nucleo-mgba:
#   libmgba_libretro_android.so   va dentro del APK en lugar del mGBA de Lemuroid
#   mgba_libretro.dll             va en pc/emuladores/RetroArch/cores
set -euo pipefail

RAIZ=$(pwd)
LUA_VERSION=${LUA_VERSION:-5.4.7}
NDK_VERSION=${ANDROID_NDK_VERSION:?Falta ANDROID_NDK_VERSION}

python3 nucleos/mgba/aplicar.py mgba

# --- Lua, como biblioteca estática ------------------------------------------------------------
curl -fsSL "https://www.lua.org/ftp/lua-${LUA_VERSION}.tar.gz" -o lua.tar.gz
if [ "$LUA_VERSION" = "5.4.7" ]; then
	echo "9fbf5e28ef86c69858f6d3d34eccc32e911c1a28b4120ff3e84aaa70cfbf1e30  lua.tar.gz" | sha256sum -c -
fi
tar xzf lua.tar.gz
LUA_SRC="$RAIZ/lua-${LUA_VERSION}/src"
LUA_C=$(ls "$LUA_SRC"/*.c | grep -v -E '/(lua|luac)\.c$')

compilar_lua() { # carpeta, compilador, ar, opciones
	mkdir -p "$1"
	(
		cd "$1"
		for f in $LUA_C; do "$2" -O2 -fPIC $4 -c "$f"; done
		"$3" rcs liblua.a ./*.o
	)
}

OPCIONES=(
	-DCMAKE_BUILD_TYPE=Release
	-DBUILD_LIBRETRO=ON -DSKIP_LIBRARY=ON -DBUILD_QT=OFF -DBUILD_SDL=OFF
	-DUSE_DEBUGGERS=OFF -DUSE_GDB_STUB=OFF -DUSE_EDITLINE=OFF -DUSE_FFMPEG=OFF -DUSE_ZLIB=OFF
	-DUSE_MINIZIP=OFF -DUSE_PNG=OFF -DUSE_LIBZIP=OFF -DUSE_SQLITE3=OFF -DUSE_ELF=OFF -DUSE_LZMA=OFF
	-DUSE_EPOXY=OFF -DUSE_DISCORD_RPC=OFF -DBUILD_GL=OFF -DBUILD_GLES2=OFF -DBUILD_GLES3=OFF
	-DENABLE_SCRIPTING=ON -DUSE_LUA=5.4 "-DLUA_INCLUDE_DIR=$LUA_SRC"
)

mkdir -p nucleo-mgba

# --- Android arm64 ----------------------------------------------------------------------------
echo "y" | "${ANDROID_SDK_ROOT}/cmdline-tools/latest/bin/sdkmanager" "ndk;$NDK_VERSION" "cmake;3.30.3" > /dev/null
NDK="$ANDROID_SDK_ROOT/ndk/$NDK_VERSION"
CMAKE_ANDROID="$ANDROID_SDK_ROOT/cmake/3.30.3/bin/cmake"
TC="$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin"
compilar_lua lua-android "$TC/aarch64-linux-android24-clang" "$TC/llvm-ar" "-DLUA_USE_POSIX"
"$CMAKE_ANDROID" -S mgba -B build-android "${OPCIONES[@]}" \
	"-DLUA_LIBRARY=$RAIZ/lua-android/liblua.a" \
	"-DCMAKE_TOOLCHAIN_FILE=$NDK/build/cmake/android.toolchain.cmake" \
	-DANDROID_ABI=arm64-v8a -DANDROID_PLATFORM=android-24
"$CMAKE_ANDROID" --build build-android --target mgba_libretro -j "$(nproc)"
SO=$(find build-android -name 'mgba_libretro*.so' | head -1)
"$TC/llvm-strip" -s "$SO"
cp "$SO" nucleo-mgba/libmgba_libretro_android.so

# --- Windows x64 (con MinGW) ------------------------------------------------------------------
sudo apt-get install -y -qq gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64 > /dev/null
compilar_lua lua-windows x86_64-w64-mingw32-gcc x86_64-w64-mingw32-ar ""
cmake -S mgba -B build-windows "${OPCIONES[@]}" \
	"-DLUA_LIBRARY=$RAIZ/lua-windows/liblua.a" \
	-DCMAKE_SYSTEM_NAME=Windows \
	-DCMAKE_C_COMPILER=x86_64-w64-mingw32-gcc -DCMAKE_CXX_COMPILER=x86_64-w64-mingw32-g++ \
	-DCMAKE_RC_COMPILER=x86_64-w64-mingw32-windres \
	-DCMAKE_FIND_ROOT_PATH=/usr/x86_64-w64-mingw32 \
	-DCMAKE_FIND_ROOT_PATH_MODE_PROGRAM=NEVER -DCMAKE_FIND_ROOT_PATH_MODE_LIBRARY=ONLY \
	-DCMAKE_FIND_ROOT_PATH_MODE_INCLUDE=ONLY \
	"-DCMAKE_SHARED_LINKER_FLAGS=-static -static-libgcc"
cmake --build build-windows --target mgba_libretro -j "$(nproc)"
DLL=$(find build-windows -name 'mgba_libretro*.dll' | head -1)
x86_64-w64-mingw32-strip -s "$DLL"
cp "$DLL" nucleo-mgba/mgba_libretro.dll

ls -la nucleo-mgba
