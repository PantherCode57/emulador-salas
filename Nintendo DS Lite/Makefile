# Panther OS para la Nintendo DS Lite, con BlocksDS (https://blocksds.skylyrac.net).
# Antes hay que dibujar las letras (hace falta Go):
#   cd letras && go run . -salida ../generado/letras.c
# Luego, con BlocksDS (por ejemplo con su imagen de Docker skylyrac/blocksds):
#   make
# Sale panther.nds, que es lo que se copia a la tarjeta de la flashcard.

BLOCKSDS	?= /opt/blocksds/core

NAME		:= panther
GAME_TITLE	:= Panther OS
GAME_SUBTITLE	:= para Nintendo DS Lite
GAME_AUTHOR	:= Pokemulador
GAME_ICON	:= icono.gif

SOURCEDIRS	:= source generado

# Con wifi: el ARM7 lleva DSWiFi.
ARM7ELF		:= $(BLOCKSDS)/sys/arm7/main_core/arm7_dswifi.elf
LIBS		:= -ldswifi9 -lnds9
LIBDIRS		:= $(BLOCKSDS)/libs/dswifi

# Cabecera de las de antes (0x200 y solo DS), no la de DSi (0x4000): las flashcards antiguas, como
# la EZ-Flash V, se quedan en «Cargando ROM…» con la moderna. Para la DS Lite no cambia nada.
NDSTOOL_ARGS	:= -h 0x200 -uc 0

include $(BLOCKSDS)/sys/default_makefiles/rom_arm9/Makefile
