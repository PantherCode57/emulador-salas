#!/usr/bin/env bash
# Prepara lo que la app Panther necesita de fuera, en panther-android/terceros (no va a git):
#   SDL2-2.32.10/          el código de SDL 2 (lo compila Gradle) con su parte de Java
#   mgba-lib/<abi>/        mGBA 0.10.5 (solo GB, GBC y GBA, como biblioteca) y Lua 5.4, ya compilados
#                          para Android, como hace consola/emulador/compilar_mgba.sh para la consola
#
#   ANDROID_SDK_ROOT=... bash preparar.sh
# Necesita el NDK 27.1.12297006 y CMake 3.22.1 del SDK de Android (sdkmanager "ndk;27.1.12297006"
# "cmake;3.22.1"). Funciona en Linux y en Windows con Git Bash.
set -euo pipefail

cd "$(dirname "$0")"
AQUI=$PWD
T=$AQUI/terceros
SDK=${ANDROID_SDK_ROOT:-${ANDROID_HOME:?Falta ANDROID_SDK_ROOT}}
NDK=${ANDROID_NDK:-$SDK/ndk/27.1.12297006}
CMAKE_DIR=$SDK/cmake/3.22.1/bin
ABIS=${ABIS:-arm64-v8a x86_64}
SDL_V=2.32.10
MGBA_V=0.10.5
LUA_V=5.4.7
case "$(uname -s)" in
	Linux*) HOST=linux-x86_64 ;;
	Darwin*) HOST=darwin-x86_64 ;;
	*) HOST=windows-x86_64 ;;
esac
TC=$NDK/toolchains/llvm/prebuilt/$HOST/bin
# CMake y clang de Windows no entienden las rutas de Git Bash (/c/...).
ruta() { if command -v cygpath > /dev/null; then cygpath -m "$1"; else echo "$1"; fi; }

mkdir -p "$T"
cd "$T"
bajar() { # dirección, archivo, sha256
	[ -f "$2" ] || curl -fsSL "$1" -o "$2"
	echo "$3  $2" | sha256sum -c -
}
bajar "https://github.com/libsdl-org/SDL/releases/download/release-$SDL_V/SDL2-$SDL_V.tar.gz" "SDL2-$SDL_V.tar.gz" \
	5f5993c530f084535c65a6879e9b26ad441169b3e25d789d83287040a9ca5165
bajar "https://github.com/mgba-emu/mgba/archive/refs/tags/$MGBA_V.tar.gz" "mgba-$MGBA_V.tar.gz" \
	91d6fbd32abcbdf030d58d3f562de25ebbc9d56040d513ff8e5c19bee9dacf14
bajar "https://www.lua.org/ftp/lua-$LUA_V.tar.gz" "lua-$LUA_V.tar.gz" \
	9fbf5e28ef86c69858f6d3d34eccc32e911c1a28b4120ff3e84aaa70cfbf1e30
[ -d "SDL2-$SDL_V" ] || tar xzf "SDL2-$SDL_V.tar.gz"
[ -d "mgba-$MGBA_V" ] || tar xzf "mgba-$MGBA_V.tar.gz"
[ -d "lua-$LUA_V" ] || tar xzf "lua-$LUA_V.tar.gz"

MGBA=$T/mgba-$MGBA_V
LUA=$T/lua-$LUA_V
# Como en la consola: lo que emula es todo C. Y en la biblioteca estática también los archivos
# comprimidos (.7z), que mGBA solo mete en la compartida.
sed -i 's/^project(mGBA)$/project(mGBA C)/' "$MGBA/CMakeLists.txt"
sed -i 's/add_library(${BINARY_NAME} STATIC ${SRC})$/add_library(${BINARY_NAME} STATIC ${SRC} ${VFS_SRC})/' "$MGBA/CMakeLists.txt"

for ABI in $ABIS; do
	SALIDA=$T/mgba-lib/$ABI
	if [ -f "$SALIDA/lib/libmgba.a" ] && [ -f "$SALIDA/lib/liblua.a" ]; then
		echo "== $ABI: ya está"
		continue
	fi
	case $ABI in
		arm64-v8a) OBJETIVO=aarch64-linux-android26 ;;
		x86_64) OBJETIVO=x86_64-linux-android26 ;;
		armeabi-v7a) OBJETIVO=armv7a-linux-androideabi26 ;;
		*) echo "no sé compilar para $ABI"; exit 1 ;;
	esac
	echo "== $ABI: Lua"
	mkdir -p "$SALIDA/lib" "$SALIDA/include" "$T/lua-obj-$ABI"
	(
		cd "$T/lua-obj-$ABI"
		for f in "$LUA"/src/*.c; do
			case $(basename "$f") in lua.c | luac.c) continue ;; esac
			"$TC/clang" --target=$OBJETIVO -O2 -fPIC -ffunction-sections -fdata-sections -DLUA_USE_POSIX -c "$(ruta "$f")"
		done
		rm -f "$SALIDA/lib/liblua.a"
		"$TC/llvm-ar" rcs "$(ruta "$SALIDA/lib/liblua.a")" ./*.o
	)
	echo "== $ABI: mGBA"
	"$CMAKE_DIR/cmake" -S "$(ruta "$MGBA")" -B "$(ruta "$T/build-mgba-$ABI")" -G Ninja \
		"-DCMAKE_MAKE_PROGRAM=$(ruta "$CMAKE_DIR/ninja")" \
		"-DCMAKE_TOOLCHAIN_FILE=$(ruta "$NDK/build/cmake/android.toolchain.cmake")" \
		-DANDROID_ABI=$ABI -DANDROID_PLATFORM=android-26 \
		-DCMAKE_BUILD_TYPE=Release -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
		"-DCMAKE_C_FLAGS_RELEASE=-O2 -DNDEBUG -ffunction-sections -fdata-sections" \
		-DBUILD_SHARED=OFF -DBUILD_STATIC=ON -DBUILD_SDL=OFF -DBUILD_QT=OFF -DBUILD_LIBRETRO=OFF \
		-DBUILD_PERF=OFF -DBUILD_TEST=OFF -DBUILD_SUITE=OFF -DBUILD_CINEMA=OFF -DBUILD_EXAMPLE=OFF \
		-DBUILD_PYTHON=OFF -DBUILD_HEADLESS=OFF -DBUILD_GL=OFF -DBUILD_GLES2=OFF -DBUILD_GLES3=OFF \
		-DUSE_DEBUGGERS=OFF -DUSE_GDB_STUB=OFF -DUSE_EDITLINE=OFF -DUSE_FFMPEG=OFF -DUSE_PNG=OFF \
		-DUSE_ZLIB=OFF -DUSE_MINIZIP=OFF -DUSE_LIBZIP=OFF -DUSE_LZMA=ON -DUSE_SQLITE3=OFF -DUSE_ELF=OFF \
		-DUSE_EPOXY=OFF -DUSE_DISCORD_RPC=OFF \
		-DENABLE_SCRIPTING=ON -DUSE_LUA=5.4 "-DLUA_INCLUDE_DIR=$(ruta "$LUA/src")" \
		"-DLUA_LIBRARY=$(ruta "$SALIDA/lib/liblua.a")"
	"$CMAKE_DIR/cmake" --build "$(ruta "$T/build-mgba-$ABI")" --target mgba
	cp "$(find "$T/build-mgba-$ABI" -name 'libmgba.a' | head -1)" "$SALIDA/lib/"
	cp -r "$MGBA/include/." "$SALIDA/include/"
	cp -r "$T/build-mgba-$ABI/include/." "$SALIDA/include/"
	cp "$LUA"/src/lua.h "$LUA"/src/luaconf.h "$LUA"/src/lualib.h "$LUA"/src/lauxlib.h "$SALIDA/include/"
	ls -la "$SALIDA/lib"
done
echo "== listo: ya se puede compilar la app (../gradlew -p panther-android assembleDebug)"
