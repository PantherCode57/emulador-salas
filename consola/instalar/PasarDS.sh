#!/bin/bash
# Pasar a la DS (Panther Store): copia Panther OS para la Nintendo DS Lite (panther.nds) a la
# tarjeta de la flashcard por la ranura de la consola, cambiando las tarjetas con la consola
# encendida. Lo hace el programa (consola/pasards). Aquí solo se copia a la memoria y se arranca
# desde ahí como un servicio aparte, para que siga cuando se pare EmulationStation y se saque la
# tarjeta de Panther OS.
DIR=/home/ark/panther/pasards
M=/dev/shm/pasards
# Si /dev/shm no deja ejecutar programas, en /run (también está en la memoria).
if grep -qs ' /dev/shm [^ ]* [^ ]*noexec' /proc/mounts; then
	M=/run/pasards
fi
sudo umount -l /dev/shm/pasards/flashcard 2> /dev/null
sudo rm -f "$M/pasards" "$M/panther.nds"
sudo mkdir -p "$M"
sudo cp "$DIR/pasards" "$DIR/panther.nds" "$M/" || exit 1
sudo chmod 755 "$M/pasards"
sudo systemctl reset-failed panther-pasards 2> /dev/null
sudo systemd-run --unit=panther-pasards --collect --quiet \
	"$M/pasards" -nds "$M/panther.nds" -registro "$M/registro.txt" || exit 1
# El programa para EmulationStation (y con él esto). Mientras tanto, que no vuelva el menú.
exec sleep 120
