#!/usr/bin/env bash
# Prueba de la pantalla de salas con un RetroArch de escritorio (Ubuntu, Xvfb), sin Lakka:
# el servicio, el núcleo de la pantalla, una sala de verdad con un amigo simulado (otro enlace)
# y abrir un juego. Las máquinas virtuales no sirven para esto porque el Mesa de Lakka no tiene
# controladores para sus tarjetas gráficas.
#
#   escritorio.sh <carpeta con las piezas> <carpeta de resultados>
# Necesita POKEMULADOR_TS_KEY en el entorno (para el amigo simulado).
set -uo pipefail
PIEZAS=$(realpath "$1")
OUT=$(realpath -m "$2")
mkdir -p "$OUT"
T=$(mktemp -d /tmp/pkm-prueba.XXXX)
mkdir -p "$T"/cores "$T"/juegos "$T"/datos "$T"/lib "$T"/amigo/amigos
cp "$PIEZAS"/pokemulador_libretro.so "$PIEZAS"/pokemulador_libretro.info "$T"/cores/
cp /usr/lib/x86_64-linux-gnu/libretro/gambatte_libretro.so "$T"/cores/
cp "$PIEZAS"/enlace "$PIEZAS"/GBA-PK_Multiplayer.lua "$PIEZAS"/pokemulador_final.lua "$T"/lib/
chmod +x "$T"/lib/enlace "$PIEZAS"/pokemulador-usb
python3 lakka/prueba/rom_prueba.py "$T/juegos/Juego de prueba.gb"
cat > "$T"/ra.cfg <<EOF
video_driver = "gl"
audio_driver = "null"
input_driver = "x"
video_fullscreen = "false"
video_windowed_position_width = "960"
video_windowed_position_height = "540"
menu_driver = "rgui"
pause_nonactive = "false"
libretro_info_path = "$T/cores"
user_language = "3"
config_save_on_exit = "false"
EOF

Xvfb :99 -screen 0 1280x720x24 &
export DISPLAY=:99
sleep 2

env POKEMULADOR_DATOS="$T"/datos POKEMULADOR_RETROARCH="$(command -v retroarch)" POKEMULADOR_CORES="$T"/cores \
	POKEMULADOR_LIB="$T"/lib POKEMULADOR_JUEGOS="$T"/juegos POKEMULADOR_APAGAR=- POKEMULADOR_RETROARCH_CFG="$T"/ra.cfg \
	"$PIEZAS"/pokemulador-usb > "$OUT"/servicio.txt 2>&1 &

captura() { import -window root "$OUT/$1.png"; echo "captura $1"; }
tecla() {
	for v in $(xdotool search --class retroarch 2>/dev/null); do xdotool windowfocus "$v" 2>/dev/null; done
	xdotool key --delay 200 "$@"
	sleep 1
}

sleep 15; captura 1-inicio
tecla x; sleep 2; captura 2-juegos
tecla x; sleep 2; captura 3-partida
tecla x; sleep 30; captura 4-sala

codigo=$(python3 -c "import json;print(json.load(open('$T/datos/salas.json'))[0]['codigo'])")
echo "sala: $codigo"
python3 - "$T"/amigo/mi.json <<'PY'
import json, sys, time
json.dump({"version": 1, "id": "PRUEBA-AMIGO", "nombre": "Amigo de prueba", "juego": "Pokemon Rojo Fuego",
           "partida": "principal", "jugadoMs": 3720000, "ultimaVez": int(time.time() * 1000)}, open(sys.argv[1], "w"))
PY
# El amigo: otro enlace en la misma sala (con la entrada abierta para que no se cierre).
(sleep 600 | "$T"/lib/enlace sala --codigo "$codigo" --id PRUEBA-AMIGO --progreso "$T"/amigo/mi.json \
	--amigos "$T"/amigo/amigos --local 24990 --juego 0 > "$OUT"/amigo.txt 2>&1) &
sleep 40; captura 5-sala-con-amigo

tecla x; sleep 15; captura 6-juego          # Jugar
sleep 45                                    # la primera captura del progreso llega a los 40 s
tecla F1; sleep 3; captura 7-menu-juego
echo -n QUIT | nc -u -w1 127.0.0.1 55355     # cerrar el juego: vuelve la pantalla de salas
sleep 10; captura 8-vuelta-a-la-sala

echo "== progreso propio"; python3 -c "
import json; p=json.load(open('$T/datos/salas/$codigo/mi_progreso.json')); c=p.pop('captura',''); print(p, 'captura:', len(c))"
echo "== lo que le llegó al amigo"; ls -la "$T"/amigo/amigos; for f in "$T"/amigo/amigos/*.json; do python3 -c "
import json; p=json.load(open('$f')); c=p.pop('captura',''); print(p, 'captura:', len(c))"; done
echo "== lo que nos llegó del amigo"; ls -la "$T"/datos/salas/"$codigo"/amigos
cp "$T"/datos/registro.txt "$T"/datos/retroarch.txt "$OUT"/ 2>/dev/null
tail -40 "$T"/datos/registro.txt
pkill -f pokemulador-usb; pkill -f retroarch; pkill -f "enlace sala"; true
