#!/usr/bin/env bash
# Prueba la app Panther en un emulador de Android (en GitHub, con android-emulator-runner):
#   1. la pantalla de inicio, vacía y con dos ROMs de prueba (una de GB y otra de GBA, hechas a mano);
#   2. abre la de GBA tocando su tarjeta, de pie: el juego arriba y los botones debajo;
#   3. «atrás» abre el menú de pausa; con la cruz y A se guarda una partida rápida;
#   4. tumbado; y por último «Salir del juego» desde el menú, que vuelve a la lista.
# Deja capturas y el registro (logcat) en la carpeta de resultados.
#
#   emulador.sh <app.apk> <carpeta de resultados>
set -uo pipefail
APK=$1
OUT=$2
mkdir -p "$OUT"
PKG=com.panthercode.panther
DIR=/sdcard/Android/data/$PKG/files/juegos
AQUI=$(cd "$(dirname "$0")" && pwd)

cap() { adb exec-out screencap -p > "$OUT/$1.png"; echo "captura $1"; }
toca() { adb shell input tap "$1" "$2"; sleep "${3:-1}"; }
# El centro de un botón de la pantalla del juego, según lo que apunta el emulador en el registro.
boton() {
	adb logcat -d -s Panther:I | grep "botón $1:" | tail -1 | awk '{for (i = 1; i <= NF; i++) if ($i == "'"$1"':") print $(i+1) + int($(i+3) / 2), $(i+2) + int($(i+4) / 2)}'
}
# Toca la tarjeta del juego con ese nombre en la pantalla de inicio.
tarjeta() {
	adb shell uiautomator dump /sdcard/ui.xml > /dev/null
	adb shell cat /sdcard/ui.xml | python3 -c "
import re, sys
x = sys.stdin.read()
m = re.search(r'text=\"' + re.escape(sys.argv[1]) + r'\"[^>]*bounds=\"\[(\d+),(\d+)\]\[(\d+),(\d+)\]\"', x)
print((int(m[1]) + int(m[3])) // 2, (int(m[2]) + int(m[4])) // 2) if m else print('')
" "$1"
}

adb install -r "$APK" || exit 1
adb shell settings put system accelerometer_rotation 0
adb shell settings put system user_rotation 0
adb shell wm size
adb logcat -c

adb shell am start -W -n $PKG/.Inicio
sleep 5
cap 1-inicio-vacio

python3 "$AQUI/rom_gba.py" /tmp/prueba.gba
python3 "$AQUI/../../lakka/prueba/rom_prueba.py" /tmp/prueba.gb
adb shell mkdir -p "$DIR"
adb push /tmp/prueba.gba "$DIR/Prueba de Panther (GBA).gba"
adb push /tmp/prueba.gb "$DIR/Prueba de Game Boy.gb"
adb shell am force-stop $PKG
adb shell am start -W -n $PKG/.Inicio
sleep 4
cap 2-inicio-con-juegos

donde=$(tarjeta "Prueba de Panther")
echo "tarjeta de GBA en: $donde"
[ -n "$donde" ] && toca $donde 10
cap 3-juego-de-pie
adb shell dumpsys window | grep -E "mCurrentFocus|mFocusedApp" | head -3

adb shell input keyevent KEYCODE_BACK
sleep 2
cap 4-menu-con-atras
abajo=$(boton cruz | awk '{print $1, $2}')
read -r cx cy <<< "$abajo"
r=$(adb logcat -d -s Panther:I | grep "botón cruz:" | tail -1 | awk '{print $NF}')
echo "cruz en $cx $cy (lado $r)"
toca "$cx" $((cy + r * 3 / 10)) 1      # abajo: «Guardar partida rápida»
cap 5-menu-abajo
toca $(boton A) 2                      # A
cap 6-partida-rapida-guardada

toca $(boton Rapido) 2
cap 7-rapido

adb shell settings put system user_rotation 1
sleep 5
cap 8-tumbado
adb shell settings put system user_rotation 0
sleep 4

# Salir: menú, cuatro veces abajo (la de cargar ya está activa: cinco) y A.
adb shell input keyevent KEYCODE_BACK
sleep 2
read -r cx cy <<< "$(boton cruz)"
r=$(adb logcat -d -s Panther:I | grep "botón cruz:" | tail -1 | awk '{print $NF}')
for i in 1 2 3 4 5; do toca "$cx" $((cy + r * 3 / 10)) 1; done
cap 9-menu-salir
toca $(boton A) 6
cap 10-vuelta-a-la-lista

adb logcat -d > "$OUT/logcat.txt"
adb logcat -d -s Panther:I SDL:I AndroidRuntime:E DEBUG:I | tail -120
adb shell ls -la "$DIR"
true
