# El emulador Panther dentro de Pokemulador

Los juegos de GB, GBC y GBA de Pokemulador (Android) se abren con el emulador de la consola Panther
(`consola/emulador`: mGBA 0.10.5 con Lua, y SDL2), con el aspecto de la app: carcasa de Pokédex, menú de
pausa y avisos propios.

- `src/main/cpp`: lo que cambia para Pokemulador. `poke_menu.c` (el menú, que también se toca con el
  dedo), `poke_avisos.c` y `poke_ordenes.c` (lo que la app le dice al emulador). El resto es de
  `consola/emulador` y `panther-android/app/src/main/cpp` (con `TEMA_POKEDEX`).
- `lemuroid-app/.../shared/game/panther`: `JuegoPanther` (la pantalla del juego, en su proceso) y `Panther`
  (cuándo se usa). Se quita en Ajustes › Emulador Panther, y un juego suelto se puede abrir con el de
  antes manteniéndolo pulsado.
- Las partidas son las de siempre (`saves/<juego>.srm`, o las de la sala); `FormatoPartida` arregla lo
  único que cambia entre los dos emuladores, el reloj del cartucho.
- Las salas van igual: el script de GBA-PK (`PartidaCompartidaGba`) y el cable link de Game Boy.

Antes de compilar: `ANDROID_SDK_ROOT=… bash panther-android/preparar.sh`.

`prueba/emulador.sh` lo prueba en un emulador de Android (flujo `pokemulador-prueba.yml`, ramas `prueba/…`).
