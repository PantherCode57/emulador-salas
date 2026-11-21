#!/usr/bin/env bash
# Prueba la app Puente DS en un emulador de Android (en GitHub, con android-emulator-runner):
#   1. la instala y la abre con el WhatsApp de mentira y el código 123456;
#   2. desde el PC (adb forward) se le habla como la DS: saludo, código, cifrado, PING y chats;
#   3. mira en el registro que el puente llega a WhatsApp (DNS y certificados de Android).
# Deja una captura, el registro del puente y el de Android en la carpeta de resultados.
#
#   prueba.sh <app.apk> <prueba de Go (puente.test)> <carpeta de resultados>
set -uo pipefail
APK=$1
PRUEBA=$2
OUT=$3
mkdir -p "$OUT"
PKG=com.panthercode.puenteds

adb install -r "$APK" || { echo "MAL: no se instala"; exit 1; }
adb shell pm grant $PKG android.permission.POST_NOTIFICATIONS 2> /dev/null || true
adb shell am start -n $PKG/.Principal --ez falso true --es codigo 123456
sleep 20
adb exec-out screencap -p > "$OUT/app.png"
adb forward tcp:8790 tcp:8790

bien=0
for i in 1 2 3 4 5 6 7 8; do
	if PUENTE_REMOTO=127.0.0.1:8790 PUENTE_CODIGO=123456 "$PRUEBA" -test.run TestRemoto -test.v -test.count=1 > "$OUT/remoto.txt" 2>&1; then
		bien=1
		break
	fi
	sleep 5
done
adb shell run-as $PKG cat files/registro.txt > "$OUT/registro.txt" 2>&1 || true
adb shell dumpsys activity services $PKG > "$OUT/servicio.txt" 2>&1 || true
adb logcat -d > "$OUT/logcat.txt" 2>&1 || true
echo "--- registro del puente:"
cat "$OUT/registro.txt"
echo "--- la DS de mentira:"
cat "$OUT/remoto.txt"

[ "$bien" = 1 ] || { echo "MAL: el puente del emulador no contesta como a la DS"; exit 1; }
grep -q "SKIP" "$OUT/remoto.txt" && { echo "MAL: la prueba no se ha hecho"; exit 1; }
grep -q "RED OK" "$OUT/registro.txt" || { echo "MAL: desde Android el puente no llega a WhatsApp (DNS o certificados)"; exit 1; }
echo "prueba android: todo bien"
