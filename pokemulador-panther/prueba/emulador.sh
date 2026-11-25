#!/usr/bin/env bash
# Prueba Pokemulador en un emulador de Android (en GitHub, con android-emulator-runner):
#   1. la biblioteca con dos ROMs hechas a mano, y una de ellas abierta con el emulador Panther;
#   2. el menú de pausa (con «atrás» y tocando las opciones), la partida rápida y salir;
#   3. de pie y tumbado;
#   4. la partida: que se escribe, que hace copias y que es la misma con el emulador de antes;
#   5. lo importante: un juego colgado (parado a la fuerza) ya no impide abrir los demás, ni con Panther
#      ni con el de antes.
# Deja capturas, el registro (logcat) y resumen.txt en la carpeta de resultados.
#
#   emulador.sh <app.apk> <carpeta de resultados>
set -uo pipefail
APK=$1
OUT=$2
mkdir -p "$OUT"
PKG=com.swordfish.lemuroid.debug
MAIN=$PKG/com.swordfish.lemuroid.app.mobile.feature.main.MainActivity
DATOS=/sdcard/Android/data/$PKG/files
AQUI=$(cd "$(dirname "$0")" && pwd)
RES="$OUT/resumen.txt"
: > "$RES"
apunta() { echo "$*" | tee -a "$RES"; }
bien() { apunta "BIEN  $*"; }
mal() { apunta "MAL   $*"; }

cap() { adb exec-out screencap -p > "$OUT/$1.png"; echo "captura $1"; }
toca() {
	if [ -z "${1:-}" ] || [ -z "${2:-}" ]; then echo "no sé dónde tocar"; return; fi
	adb shell input tap "$1" "$2"; sleep "${3:-1}"
}
largo() { [ -n "${2:-}" ] && adb shell input swipe "$1" "$2" "$1" "$2" 900 && sleep "${3:-2}"; }
esperar_avisos() {
	for i in 1 2 3; do
		adb shell uiautomator dump /sdcard/avisos.xml > /dev/null 2>&1
		w=$(adb shell cat /sdcard/avisos.xml 2>/dev/null | python3 -c "
import re, sys
x = sys.stdin.read()
if 'responding' not in x and 'no responde' not in x: sys.exit()
m = re.search(r'text=\"(Wait|Esperar)\"[^>]*bounds=\"\[(\d+),(\d+)\]\[(\d+),(\d+)\]\"', x)
print((int(m[2]) + int(m[4])) // 2, (int(m[3]) + int(m[5])) // 2) if m else print('')")
		[ -z "$w" ] && return
		adb shell input tap $w
		sleep 3
	done
}
# El centro del texto que se ve en la pantalla (el que empieza así; con «=» delante, justo ese).
texto() {
	adb shell uiautomator dump /sdcard/ui.xml > /dev/null 2>&1
	adb shell cat /sdcard/ui.xml | python3 -c "
import re, sys
x, q = sys.stdin.read(), sys.argv[1]
justo = q.startswith('=')
q = q.lstrip('=')
for m in re.finditer(r'text=\"([^\"]*)\"[^>]*bounds=\"\[(\d+),(\d+)\]\[(\d+),(\d+)\]\"', x):
    if (m[1] == q) if justo else m[1].startswith(q):
        print((int(m[2]) + int(m[4])) // 2, (int(m[3]) + int(m[5])) // 2)
        break
" "$1"
}
busca_texto() { # con reintentos
	local d=""
	for i in 1 2 3 4; do
		d=$(texto "$1")
		[ -n "$d" ] && break
		esperar_avisos
		sleep 2
	done
	echo "$d"
}
# El centro de un botón de la pantalla del juego, según lo que apunta el emulador en el registro.
boton() {
	adb logcat -d -s Panther:I | grep "botón $1:" | tail -1 | awk '{for (i = 1; i <= NF; i++) if ($i == "'"$1"':") print $(i+1) + int($(i+3) / 2), $(i+2) + int($(i+4) / 2)}'
}
# Un punto de los 640×480 del menú, en la pantalla (de pie: el recuadro 4:3 es la zona del juego).
del_menu() {
	adb logcat -d -s Panther:I | grep "juego en" | tail -1 | python3 -c "
import re, sys
m = re.search(r'juego en (\d+)x(\d+)\+(\d+)\+(\d+)', sys.stdin.read())
if m:
    w, h, x, y = map(int, m.groups())
    mw, mh = (w, w * 3 // 4) if w * 3 // 4 <= h else (h * 4 // 3, h)
    print(x + (w - mw) // 2 + int(sys.argv[1]) * mw // 640, y + (h - mh) // 2 + int(sys.argv[2]) * mh // 480)
" "$1" "$2"
}
pid() { adb shell pidof "$PKG:$1" | tr -d '\r\n'; }
delante() { adb shell dumpsys window | grep mCurrentFocus | tail -1; }
espera_proceso() { # nombre, segundos
	for i in $(seq 1 "$2"); do [ -n "$(pid "$1")" ] && return 0; sleep 1; done
	return 1
}
espera_sin_proceso() {
	for i in $(seq 1 "$2"); do [ -z "$(pid "$1")" ] && return 0; sleep 1; done
	return 1
}
primer_byte() { adb shell "od -An -tu1 -N1 '$1' 2>/dev/null" | tr -d ' \r\n'; }
inicio() { adb shell am start -W -n "$MAIN" > /dev/null; sleep "${1:-4}"; esperar_avisos; }
abrir() { # nombre del juego en la biblioteca
	local d
	d=$(busca_texto "$1")
	echo "«$1» está en: $d"
	[ -n "$d" ] && toca $d "${2:-8}"
}
juego_nuevo_log() { adb logcat -c; }

adb root > /dev/null 2>&1; sleep 3; adb wait-for-device
adb install -r "$APK" || exit 1
adb shell pm grant $PKG android.permission.POST_NOTIFICATIONS
adb shell cmd locale set-app-locales $PKG --user 0 --locales es-ES
adb shell settings put secure immersive_mode_confirmations confirmed
adb shell settings put system accelerometer_rotation 0
adb shell settings put system user_rotation 0
adb logcat -c
sleep 20
esperar_avisos

# 1. La biblioteca
python3 "$AQUI/rom_partida.py" /tmp/partida.gba
python3 "$AQUI/../../panther-android/prueba/rom_gba.py" /tmp/colores.gba
python3 "$AQUI/../../lakka/prueba/rom_prueba.py" /tmp/prueba.gb
inicio 6
cap 01-primera-vez
adb shell mkdir -p "$DATOS/roms"
adb push /tmp/partida.gba "$DATOS/roms/Partida.gba"
adb push /tmp/colores.gba "$DATOS/roms/Colores.gba"
adb push /tmp/prueba.gb "$DATOS/roms/Verde.gb"
adb shell chmod -R 777 "$DATOS/roms"
adb shell am force-stop $PKG
inicio 10
if [ -z "$(texto Partida)" ]; then
	toca $(busca_texto Ajustes) 3
	cap 02-ajustes
	d=$(texto "Volver a")
	[ -z "$d" ] && d=$(texto "Rescan")
	toca $d 12
	inicio 4
	toca $(busca_texto Inicio) 3
fi
cap 03-biblioteca

# 2. Con Panther
juego_nuevo_log
abrir Partida 10
esperar_avisos
cap 04-panther-de-pie
if [ -n "$(pid panther)" ]; then bien "«Partida» se abre con Panther"; else mal "no se ha abierto Panther: $(delante)"; fi
adb shell input keyevent KEYCODE_BACK
sleep 2
cap 05-menu-con-atras
# tocar «Guardar partida rápida» (la segunda opción)
toca $(del_menu 320 $(( (480 - (78 + 6 * 40 + 54)) / 2 + 78 + 60 ))) 2
cap 06-partida-rapida
adb shell ls "$DATOS/states/panther" 2>/dev/null | grep -q '\.estado$' && bien "la opción del menú se elige tocándola (hay partida rápida)" || mal "no hay partida rápida al tocar el menú"
toca $(boton Rapido) 2
cap 07-rapido
toca $(boton Rapido) 2
adb shell settings put system user_rotation 1
sleep 6
cap 08-tumbado
adb shell input keyevent KEYCODE_BACK
sleep 2
cap 09-tumbado-menu
adb shell input keyevent KEYCODE_BACK
sleep 1
adb shell settings put system user_rotation 0
sleep 4
toca $(boton Menu) 2
cap 10-menu-con-el-boton
# «Salir del juego»: la última (la sexta, sin los Caramelos)
toca $(del_menu 320 $(( (480 - (78 + 6 * 40 + 54)) / 2 + 78 + 220 ))) 5
cap 11-vuelta-a-la-app
espera_sin_proceso panther 8 && bien "al salir se cierra el proceso de Panther" || mal "Panther sigue abierto al salir"
delante | grep -q MainActivity && bien "y se vuelve a Pokemulador" || mal "al salir no se vuelve a Pokemulador: $(delante)"

# 3. La partida
srm="$DATOS/saves/Partida.srm"
uno=$(primer_byte "$srm")
apunta "      primer byte de la partida: ${uno:-no hay partida}"
[ "$uno" = "0" ] && bien "Panther escribe la partida en la carpeta de Pokemulador" || mal "la partida de Panther no está bien"
inicio 4
abrir Partida 8
cap 12-sigues-donde-lo-dejaste
adb logcat -d -s Panther:I | tail -5
[ "$(primer_byte "$srm")" = "0" ] && bien "al volver sigue donde estaba (no empieza de cero)" || mal "ha empezado de cero: $(primer_byte "$srm")"
adb shell ls -la "$DATOS/saves" "$DATOS/saves/copias" 2>/dev/null
adb shell ls "$DATOS/saves/copias" 2>/dev/null | grep -q '^Partida-' && bien "hay copia de seguridad de la partida" || mal "no hay copia de la partida"
adb shell input keyevent KEYCODE_BACK; sleep 2
toca $(del_menu 320 $(( (480 - (78 + 6 * 40 + 54)) / 2 + 78 + 220 ))) 4
espera_sin_proceso panther 8

# la misma partida con el emulador de antes
inicio 4
largo $(busca_texto Partida) 2
cap 13-menu-del-juego
toca $(busca_texto "Abrir con el emulador de antes") 10
cap 14-con-el-de-antes
if [ -n "$(pid game)" ]; then bien "«Abrir con el emulador de antes» abre el núcleo de libretro"; else mal "no se abre el de antes: $(delante)"; fi
sleep 8
antes=$(pid game)
dos=$(primer_byte "$srm")
# salir con el menú
adb shell input keyevent KEYCODE_BACK; sleep 3
cap 15-menu-de-antes
d=$(busca_texto "Salir"); [ -z "$d" ] && d=$(texto "Quit")
toca $d 6
espera_sin_proceso game 12 && bien "al salir se cierra el proceso del juego" || mal "el proceso del juego sigue abierto al salir"
tres=$(primer_byte "$srm")
apunta "      primer byte con el de antes: $dos, y al salir: $tres"
{ [ "$tres" = "1" ] || [ "$tres" = "0" ]; } && bien "la partida es la misma en los dos" || mal "la partida no es la misma: $tres"

# 5. Un juego colgado
colgado() { # proceso, texto
	local p
	p=$(pid "$1")
	if [ -z "$p" ]; then mal "($2) no hay proceso que colgar"; return; fi
	adb shell kill -STOP "$p"
	sleep 1
	adb shell input keyevent KEYCODE_HOME; sleep 2
	# quitar la app de la lista de recientes, como hace la gente, y volver a entrar
	local t
	t=$(adb shell dumpsys activity recents | grep -oE "Recent #[0-9]+: Task\{[^}]*$PKG[^}]*" | grep -oE "#[0-9]+ type" | grep -oE "[0-9]+" | head -1)
	adb shell am stack remove "${t:-0}" > /dev/null 2>&1
	sleep 2
	apunta "      ($2) colgado $p; después de quitar la app de recientes: $(pid "$1")"
	inicio 5
	cap "$3-app"
	abrir Colores 6
	cap "$3-cerrando"
	sleep 12
	esperar_avisos
	cap "$3-abierto"
	if [ -n "$(pid panther)" ] && [ "$(pid panther)" != "$p" ] && delante | grep -q JuegoPanther; then
		bien "($2) con un juego colgado se puede abrir otro"
	else
		mal "($2) no deja abrir otro juego: $(delante) panther=$(pid panther) game=$(pid game)"
	fi
	adb shell ps -A | grep "$PKG" || true
}
salir_panther() {
	adb shell input keyevent KEYCODE_BACK; sleep 2
	toca $(del_menu 320 $(( (480 - (78 + 6 * 40 + 54)) / 2 + 78 + 220 ))) 4
	espera_sin_proceso panther 8
}
inicio 4
largo $(busca_texto Partida) 2
toca $(busca_texto "Abrir con el emulador de antes") 10
colgado game "el de antes" 16-colgado-antes
salir_panther
inicio 4
abrir Partida 8
colgado panther "Panther" 17-colgado-panther
salir_panther

# GB con Panther
inicio 4
toca $(busca_texto Inicio) 2
abrir Verde 8
cap 18-game-boy
[ -n "$(pid panther)" ] && bien "un juego de Game Boy se abre con Panther" || mal "Game Boy no se abre con Panther"
salir_panther
inicio 3
toca $(busca_texto Ajustes) 3
cap 19-ajustes

adb logcat -d > "$OUT/logcat.txt"
adb logcat -d -s Panther:I SDL:I AndroidRuntime:E DEBUG:I | tail -80
adb shell "find $DATOS -type f | head -60"
echo "=== resumen"
cat "$RES"
grep -q '^MAL' "$RES" && echo "::warning::hay pruebas que no salen bien"
true
