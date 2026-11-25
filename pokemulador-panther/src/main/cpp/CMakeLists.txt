# El emulador Panther dentro de Pokemulador: libSDL2.so y libpanther.so (el emulador de la consola,
# consola/emulador, con lo del móvil de panther-android y el aspecto de la Pokédex).
# Antes hay que lanzar panther-android/preparar.sh, que deja SDL2 y mGBA con Lua ya compilados.
cmake_minimum_required(VERSION 3.18)
project(pokemulador_panther C CXX)

if(NOT PANTHER_TERCEROS)
	message(FATAL_ERROR "Falta PANTHER_TERCEROS: lanza antes panther-android/preparar.sh")
endif()

set(SDL_SHARED ON CACHE BOOL "" FORCE)
set(SDL_STATIC OFF CACHE BOOL "" FORCE)
set(SDL_TEST OFF CACHE BOOL "" FORCE)
add_subdirectory(${PANTHER_TERCEROS}/SDL2-2.32.10 sdl2)

set(RAIZ ${CMAKE_CURRENT_SOURCE_DIR}/../../../..)
set(EMU ${RAIZ}/consola/emulador)
set(MOVIL ${RAIZ}/panther-android/app/src/main/cpp)
set(MGBA ${PANTHER_TERCEROS}/mgba-lib/${ANDROID_ABI})

add_library(panther SHARED
	poke_menu.c poke_avisos.c poke_ordenes.c
	${MOVIL}/movil.c ${MOVIL}/movil_video.c ${MOVIL}/movil_mandos.c ${MOVIL}/movil_tactil.c
	${EMU}/juego.c ${EMU}/lienzo.c ${EMU}/sonido.c ${EMU}/ajustes.c ${EMU}/cable.c ${EMU}/ordenes.c)
# De ordenes.c solo se usa guardar_png: las órdenes llegan de la app (poke_ordenes.c), no por la red.
set_source_files_properties(${EMU}/ordenes.c PROPERTIES COMPILE_DEFINITIONS
	"ordenes_abrir=consola_ordenes_abrir;ordenes_leer=consola_ordenes_leer;ordenes_cerrar=consola_ordenes_cerrar")
target_compile_definitions(panther PRIVATE TEMA_POKEDEX)
target_include_directories(panther PRIVATE ${MOVIL} ${EMU} ${MGBA}/include)
target_compile_options(panther PRIVATE -std=gnu11 -O2 -Wall -Wextra -Wno-unused-parameter
	-Wno-missing-field-initializers -ffunction-sections -fdata-sections)
target_link_options(panther PRIVATE -Wl,--gc-sections)
target_link_libraries(panther SDL2 ${MGBA}/lib/libmgba.a ${MGBA}/lib/liblua.a z m log android)
