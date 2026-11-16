# Emulador de Panther OS. Se compila en la consola (o en la máquina arm64 de GitHub) después de
# compilar_mgba.sh, que deja mGBA y Lua en mgba-lib.
MGBA ?= $(HOME)/panther-src/mgba-lib

CFLAGS ?= -O2
CFLAGS += -std=gnu11 -Wall -Wextra -Wno-unused-parameter -ffunction-sections -fdata-sections \
	$(shell pkg-config --cflags sdl2) -I$(MGBA)/include
# mGBA viene compilado con -flto: al enlazar se optimiza todo junto (en 4 hilos, que si no tarda mucho).
LDFLAGS += -flto=4 -Wl,--gc-sections -Wl,--as-needed -s
LDLIBS += $(MGBA)/lib/libmgba.a $(MGBA)/lib/liblua.a $(shell pkg-config --libs sdl2) -lz -lm -lpthread -ldl

OBJ = main.o juego.o apps.o video.o sonido.o mandos.o capa.o avisos.o menu.o lienzo.o ajustes.o

emulador: $(OBJ)
	$(CC) $(LDFLAGS) -o $@ $(OBJ) $(LDLIBS)
	@ls -la $@

$(OBJ): panther.h

clean:
	rm -f $(OBJ) emulador

.PHONY: clean
