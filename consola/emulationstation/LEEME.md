# Menú de la consola (EmulationStation)

El menú de dArkOS4Clone 20261008 es EmulationStation-fcamod de lcdyk0517, compilado el 5/10/2026 desde
el commit `501afbab11`. El flujo `.github/workflows/emulationstation.yml` lo compila desde ese
commit con los cambios de aquí (`*.patch`, en orden) para Debian 13 arm64.

Para instalarlo en la consola se sustituye `/usr/bin/emulationstation/emulationstation` con
EmulationStation parado. Antes se guarda el original como `emulationstation.panther-original`
(si ya está, no se toca).

## `0001-panther.patch`

- **Hora y batería por pantalla**: en la pantalla principal se usan `clock`, `date` (la fecha en
  español, «Domingo, 15 de noviembre») y `batteryIndicator` de la vista `screen` del tema; en las
  listas de juegos, `clock_lista` y la posición de `batteryIndicator_lista` si el tema las tiene.
- **Barra de arriba** en la pantalla principal: con ↑ se elige el Wi-Fi (el icono de red de la barra
  de estado) y con ←→ el engranaje que va a su izquierda (`topbar_ajustes` en la vista `system`).
  A abre las opciones del sistema (las de `/opt/system`) o la pantalla de Wi-Fi; ↓ o B vuelven a
  las tarjetas. Debajo sale una etiqueta («Wi-Fi · nombre de la red» o «Ajustes»); el marco de la
  elegida es `topbar_marca` y el texto `topbar_texto`.
- **Pantalla de Wi-Fi** (`GuiPantherWifi`): interruptor, red conectada (con «Olvidar» en X), redes
  disponibles con su cobertura y candado, A para conectar (pide la contraseña con el teclado de la
  pantalla si hace falta) e Y para buscar redes. Usa `nmcli` con `sudo` en otro hilo. El fondo es la
  imagen del ajuste `PantherWifiBackground`.
  Si se ha conectado lo decide el estado real del Wi-Fi (`nmcli device`), no el código de salida de
  nmcli. Una red guardada que no conecta no se borra: si la contraseña ya no vale se cambia (y si la
  nueva tampoco va, vuelve la de antes). Cuando falla, sale el motivo (contraseña, red no encontrada,
  red de instituto con usuario, WPA3…) y, si no se sabe, la línea de error de nmcli.
- **Apertura directa**: los apartados del ajuste `PantherDirectLaunch` (separados por comas) que
  tienen un solo juego lo abren al pulsar A, sin pasar por su lista.
- **Carrusel**: propiedad `minLogoOpacity` (0 a 1) para las tarjetas no elegidas; antes estaba fija
  a la mitad.

El tema (`consola/instalar/tema.py`) usa estas cosas, así que hay que instalar este menú antes que
el tema nuevo: con el menú original el tema no carga (no conoce `minLogoOpacity`).
