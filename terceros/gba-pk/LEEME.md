# GBA-PK Multiplayer (partida compartida de GBA)

`GBA-PK_Multiplayer.lua` es **GBA-PK Multiplayer**, de **TheHunterManX**:
https://github.com/TheHunterManX/GBA-PK-multiplayer (copia exacta del commit
`6ef535cf15479ccc87ac8ced775751511e4915a3`, release 1.0 / Alpha 4.0).

Licencia: Creative Commons Atribución-NoComercial 4.0 (`LICENSE.md`). Pokemulador es de uso
privado y sin ánimo de lucro; el archivo no se modifica aquí.

Lo que hace en Pokemulador: al jugar a Rojo Fuego, Verde Hoja, Rubí, Zafiro o Esmeralda desde una
sala, la app (Android o PC) escribe una copia en `<carpeta de sistema>/pokemulador/multijugador.lua`
con dos cambios: el nombre del jugador y, al final, unas líneas que abren la partida compartida (si
tienes la sala abierta) o entran en la del anfitrión, siempre en `127.0.0.1:4096`. El enlace de la
sala lleva esa conexión por Tailscale (`pc/enlace/juego.go`). El núcleo de mGBA de Pokemulador
carga ese archivo al abrir el juego (`nucleos/mgba`).
