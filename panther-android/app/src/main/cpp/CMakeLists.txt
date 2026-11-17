# La parte nativa de la app Panther: libSDL2.so (SDL 2, para la pantalla, el sonido y los mandos) y
# libmain.so (el emulador de la consola, consola/emulador, con lo del móvil de esta carpeta).
# Antes hay que lanzar ../../../../preparar.sh, que deja en terceros/ SDL2 y mGBA con Lua ya compilados.
cmake_minimum_required(VERSION 3.18)
project(panther C CXX)

if(NOT PANTHER_TERCEROS)
	message(FATAL_ERROR "Falta PANTHER_TERCEROS: lanza antes panther-android/preparar.sh")
endif()

set(SDL_SHARED ON CACHE BOOL "" FORCE)
set(SDL_STATIC OFF CACHE BOOL "" FORCE)
set(SDL_TEST OFF CACHE BOOL "" FORCE)
add_subdirectory(${PANTHER_TERCEROS}/SDL2-2.32.10 sdl2)

set(EMU ${CMAKE_CURRENT_SOURCE_DIR}/../../../../../consola/emulador)
set(MGBA ${PANTHER_TERCEROS}/mgba-lib/${ANDROID_ABI})

add_library(main SHARED
	movil.c movil_video.c movil_mandos.c movil_tactil.c
	${EMU}/juego.c ${EMU}/menu.c ${EMU}/avisos.c ${EMU}/lienzo.c ${EMU}/sonido.c
	${EMU}/ajustes.c ${EMU}/cable.c ${EMU}/ordenes.c)
target_include_directories(main PRIVATE ${CMAKE_CURRENT_SOURCE_DIR} ${EMU} ${MGBA}/include)
target_compile_options(main PRIVATE -std=gnu11 -O2 -Wall -Wextra -Wno-unused-parameter
	-Wno-missing-field-initializers -ffunction-sections -fdata-sections)
target_link_options(main PRIVATE -Wl,--gc-sections)
target_link_libraries(main SDL2 ${MGBA}/lib/libmgba.a ${MGBA}/lib/liblua.a z m log android)
