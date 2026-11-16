# Emulador de Panther OS

Un solo programa (unos 1,3 MB) para la consola, sin RetroArch:

- **Juegos de GB, GBC y GBA** con el código de [mGBA](https://mgba.io) 0.10.5 dentro (licencia MPL 2.0),
  y Lua 5.4 (licencia MIT) para los scripts de mGBA: las salas de GBA-PK y los Caramelos Raros.
- **La pantalla de las apps** de la consola (WhatsApp, YouTube, Spotin, Pokémon, Navegador): hablan
  con él por `127.0.0.1:24890` igual que con el núcleo `pokemulador_libretro` de antes.
- **Encima de todo**, los avisos de WhatsApp y el chat abierto con R3 (los pone el servicio de
  WhatsApp en `/dev/shm/panther-capa`; el formato está en `capa.c`).

En los juegos:

| Botón | Qué hace |
| --- | --- |
| R2 | ir rápido (×3; se cambia en el menú) |
| SELECT + START | menú de pausa: guardar y cargar partida rápida, velocidad, empezar de nuevo, salir |
| R3 | abrir el chat de WhatsApp del último aviso (B vuelve al juego) |

La partida del juego es el `.srm` de siempre (el de RetroArch), junto al juego o en la carpeta que
diga la app. Al salir se guarda dónde ibas (`.estado-auto`) y la próxima vez se sigue ahí, salvo si
la partida del juego ha cambiado después con otro programa.

## Compilar

En la consola (tiene gcc, cmake y SDL2) o en la máquina arm64 de GitHub (`consola.yml`):

```bash
# en una carpeta con mgba-0.10.5 y lua-5.4.7 descomprimidos
bash compilar_mgba.sh            # deja mgba-lib/ con libmgba.a y liblua.a
make MGBA=/ruta/a/mgba-lib       # en consola/emulador
```

`terceros/stb_truetype.h` es de dominio público (Sean Barrett). La letra es Selawik (la del tema),
que se copia junto al programa como `selawk.ttf` y `selawksb.ttf`.
