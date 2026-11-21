#!/bin/bash
# Flashear tarjeta (Panther Store): pasa Panther OS a la tarjeta de otra consola (la DS con
# flashcard, una GBA con flashcard o una china con dArkOS) por la ranura de esta, cambiando las
# tarjetas con la consola encendida. Lo hace el programa (consola/flashear). Aquí solo se copia a la
# memoria y se arranca desde ahí como un servicio aparte, para que siga cuando se pare
# EmulationStation y se saque la tarjeta de Panther OS.
DIR=/home/ark/panther/flashear
M=/dev/shm/flashear
# Si /dev/shm no deja ejecutar programas, en /run (también está en la memoria).
if grep -qs ' /dev/shm [^ ]* [^ ]*noexec' /proc/mounts; then
	M=/run/flashear
fi
for p in /dev/shm/flashear/p*; do sudo umount -l "$p" 2> /dev/null; done
sudo rm -f "$M/flashear" "$M/panther.nds"
sudo mkdir -p "$M"
sudo cp "$DIR/flashear" "$M/" || exit 1
sudo cp "$DIR/panther.nds" "$M/" 2> /dev/null
sudo chmod 755 "$M/flashear"
sudo systemctl reset-failed panther-flashear 2> /dev/null
sudo systemd-run --unit=panther-flashear --collect --quiet \
	"$M/flashear" -nds "$M/panther.nds" -registro "$M/registro.txt" || exit 1
# El programa para EmulationStation (y con él esto). Mientras tanto, que no vuelva el menú.
exec sleep 120
