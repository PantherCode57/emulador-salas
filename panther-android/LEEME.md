# Panther para el móvil

La app de Android con el **emulador de Panther OS**, el de la consola (`consola/emulador`): GB, GBC y GBA
con mGBA 0.10.5 y Lua por dentro, el mismo menú de pausa, «Rápido» a ×3 y seguir donde lo dejaste. Es
una app aparte de Pokemulador (`com.panthercode.panther`), sin internet y sin juegos dentro.

- **Añadir juegos**: abre el selector de archivos de Android, donde están el móvil y Google Drive (lo
  descarga al elegirlo). Vale `.gba`, `.gbc`, `.gb`, `.7z` y `.zip` (se saca lo de dentro). Si eliges
  también una partida `.sav` con el mismo nombre que el juego, se le pone.
- Los juegos y sus partidas (`.srm`, las mismas que en RetroArch y en la consola) quedan en
  `Android/data/com.panthercode.panther/files/juegos`.
- En cada juego (⋯ o mantener pulsado): traer una partida, guardarla en el móvil o en Drive, Caramelos
  Raros infinitos (Rojo Fuego y Verde Hoja de EE. UU., con `consola/trucos/caramelos-frlg.lua`) y quitarlo.
- En el juego: de pie, el juego arriba y los botones debajo; tumbado, el juego en medio y los botones a
  los lados. **Menú** (o la tecla atrás) abre el menú de pausa de la consola. Vale también un mando
  Bluetooth o USB (y entonces se esconden los botones de la pantalla).

## Cómo está hecho

| Parte | Dónde |
| --- | --- |
| El juego, el menú, los avisos, la letra, el cable link | `consola/emulador` (compartido con la consola) |
| El arranque, la pantalla y los botones táctiles | `app/src/main/cpp/movil*.c` |
| La lista de juegos y traerlos | `app/src/main/java/com/panthercode/panther` |
| SDL 2.32.10 (pantalla, sonido, mandos) | lo descarga `preparar.sh` en `terceros/` |

## Compilar

```bash
ANDROID_SDK_ROOT=... bash panther-android/preparar.sh   # SDL2, y mGBA y Lua para Android
./gradlew -p panther-android assembleDebug               # con el gradlew de la raíz
```

Necesita el NDK 27.1.12297006 y CMake 3.22.1 del SDK de Android. En GitHub lo hace
`.github/workflows/panther-android.yml`, que además lo prueba en un emulador de Android con dos ROMs
de prueba hechas a mano (`prueba/rom_gba.py` y `lakka/prueba/rom_prueba.py`) y deja capturas.

La letra es Selawik (licencia SIL OFL 1.1, en `app/src/main/assets/LICENCIA-Selawik.txt`).
