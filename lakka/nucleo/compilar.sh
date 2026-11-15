#!/usr/bin/env bash
# Compila el núcleo de la pantalla de salas para Lakka (x86_64). Deja pokemulador_libretro.so.
set -euo pipefail
cd "$(dirname "$0")"
${CC:-gcc} -O2 -fPIC -shared -fvisibility=hidden -Wall -Wextra -std=c11 -D_DEFAULT_SOURCE \
	-o pokemulador_libretro.so pokemulador_libretro.c
strip -s pokemulador_libretro.so
ls -la pokemulador_libretro.so
