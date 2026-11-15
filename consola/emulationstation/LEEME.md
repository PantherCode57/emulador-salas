# Menú de la consola (EmulationStation)

El menú de dArkOS4Clone 20261008 es EmulationStation-fcamod de lcdyk0517, compilado el 5/10/2026 desde
el commit `501afbab11`. El flujo `.github/workflows/emulationstation.yml` lo compila desde ese
commit con los cambios de aquí (`*.patch`, en orden) para Debian 13 arm64.

Para instalarlo en la consola se sustituye `/usr/bin/emulationstation/emulationstation` con
EmulationStation parado. Antes se guarda el original como `emulationstation.panther-original`
(si ya está, no se toca).
