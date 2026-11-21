#!/bin/bash
# Pokémon de Panther OS (Pokemulador en la consola): lo abre EmulationStation desde su tarjeta.
# El programa (consola/pokemon), el enlace de las salas, el mGBA de Pokemulador y el núcleo de la
# pantalla están en /home/ark/panther/pokemon.
DIR=/home/ark/panther/pokemon
EMU=/home/ark/panther/emulador/emulador
FUENTE=https://github.com/PantherCode57/emulador-salas/releases/download/panther-apps

# El emulador de Panther de antes del 20/11/2026 se compiló sin los scripts de Lua, y sin ellos no
# van las salas de GBA (GBA-PK: en Rojo Fuego sale EXIT y no MULTI). Si el que hay es ese, se pone
# el de la release de la Panther Store, comprobando su suma. Solo pasa una vez; si no hay internet,
# se abre con el que hay y se vuelve a probar la próxima vez. El de antes queda en emulador.antes-lua.
if [ -x "$EMU" ] && ! grep -q -a "en marcha" "$EMU"; then
	T="$EMU.bajando"
	J=/dev/shm/pokemon-tienda.json
	if curl -fsSL --max-time 60 "$FUENTE/tienda.json" -o "$J" &&
		SUMA=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["piezas"]["emulador"]["sha256"])' "$J") &&
		curl -fsSL --max-time 180 "$FUENTE/emulador" -o "$T" &&
		[ "$(sha256sum "$T" | cut -d' ' -f1)" = "$SUMA" ] && grep -q -a "en marcha" "$T"; then
		[ -f "$EMU.antes-lua" ] || cp -p "$EMU" "$EMU.antes-lua"
		chmod 755 "$T" && mv -f "$T" "$EMU"
	fi
	rm -f "$T" "$J"
fi

# La Panther Store de antes del 21/11/2026 no se ponía al día sola ni sabía dar de alta en el menú
# las apps nuevas (la Pokédex). Si la que hay es esa, se pone la de la release, comprobando su suma,
# aparte y sin esperar (Pokémon se abre ya). Solo pasa una vez: desde entonces la tienda se pone al
# día sola al abrirla. La de antes queda en tienda.antes-sola. «PantherStore/2» es lo que la tienda
# nueva dice al pedir cosas (consola/tienda/red.go).
TIENDA=/home/ark/panther/tienda/tienda
if [ -x "$TIENDA" ] && ! grep -q -a "PantherStore/2" "$TIENDA"; then
	S=/dev/shm/pokemon-tienda-nueva.sh
	cat > "$S" <<'FIN'
T="$1.bajando"
J=/dev/shm/pokemon-tienda-nueva.json
rm -f "$T" "$J"
if curl -fsSL --max-time 60 "$2/tienda.json" -o "$J" &&
	SUMA=$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["piezas"]["tienda"]["sha256"])' "$J") &&
	curl -fsSL --max-time 300 "$2/tienda" -o "$T" &&
	[ "$(sha256sum "$T" | cut -d' ' -f1)" = "$SUMA" ] && grep -q -a "PantherStore/2" "$T"; then
	[ -f "$1.antes-sola" ] || cp -p "$1" "$1.antes-sola"
	chmod 755 "$T" && mv -f "$T" "$1"
fi
rm -f "$T" "$J" "$0"
FIN
	if command -v setsid > /dev/null; then
		setsid -f bash "$S" "$TIENDA" "$FUENTE" > /dev/null 2>&1 < /dev/null
	else
		nohup bash "$S" "$TIENDA" "$FUENTE" > /dev/null 2>&1 < /dev/null &
	fi
fi

# La pantalla (y en Pokémon, también los juegos de GB, GBC y GBA) es el emulador de Panther, sin RetroArch.
export CONSOLA_RETROARCH=$EMU
export CONSOLA_NUCLEO="$DIR/pokemulador_libretro.so"
export POKEMULADOR_ANCHO=640 POKEMULADOR_ALTO=480
cd "$DIR" || exit 1
exec ./pokemon
