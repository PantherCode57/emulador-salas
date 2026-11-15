#!/usr/bin/env bash
# Monta la imagen del USB de Pokemulador a partir de Lakka (se lanza con sudo, en Linux).
#
#   construir.sh <Lakka.img> <carpeta con nuestras piezas> <salida.img>
#
# La carpeta de piezas trae: mgba_libretro.so, azahar_libretro.so, pokemulador_libretro.so,
# pokemulador-usb, enlace, GBA-PK_Multiplayer.lua, pokemulador_final.lua, LICENSE-GBA-PK.md.
#
# Lo que hace:
#   1. Saca el sistema de Lakka (SYSTEM, squashfs) y le quita lo que no usamos: los ~200 núcleos
#      que no son de Game Boy, GBA, DS o 3DS, las bases de datos de juegos, los shaders y overlays,
#      y los temas de iconos que no se ven.
#   2. Le pone nuestras piezas y hace que el sistema arranque nuestro servicio en vez de RetroArch.
#   3. Hace una imagen nueva más pequeña: parte "LAKKA" (FAT32, el sistema) y parte "LAKKA_DISK"
#      (ext4, partidas y ajustes) con los mismos identificadores que espera el arranque de Lakka.
#      La parte JUEGOS (exFAT, la que se ve desde Windows) se crea al grabar el USB o, si no, el
#      servicio la crea solo la primera vez con el espacio que quede.
set -euo pipefail

ORIGEN=$(realpath "$1")
PIEZAS=$(realpath "$2")
SALIDA=$3
TAM_DATOS_MB=${TAM_DATOS_MB:-512}
TRABAJO=$(mktemp -d /tmp/pokemulador-imagen.XXXX)
trap 'set +e; umount -q "$TRABAJO"/lakka "$TRABAJO"/p1 "$TRABAJO"/p2 2>/dev/null; losetup -D; rm -rf "$TRABAJO"' EXIT

echo "== Lakka original"
DEV=$(losetup -fP --show -r "$ORIGEN"); sleep 1
mkdir -p "$TRABAJO"/lakka
mount -o ro "${DEV}p1" "$TRABAJO"/lakka
UUID_FAT=$(blkid -o value -s UUID "${DEV}p1" | tr -d '-')
UUID_EXT=$(blkid -o value -s UUID "${DEV}p2")
echo "FAT $UUID_FAT  ext4 $UUID_EXT"

echo "== Sacando el sistema"
SYS="$TRABAJO"/sistema
unsquashfs -q -no-progress -d "$SYS" "$TRABAJO"/lakka/SYSTEM
du -sh "$SYS"

echo "== Recortando"
LIB="$SYS"/usr/lib/libretro
CONSERVAR="gambatte melondsds mgba"
for f in "$LIB"/*_libretro.so "$LIB"/*_libretro.info; do
	nombre=$(basename "$f"); nucleo=${nombre%_libretro.*}
	case " $CONSERVAR " in *" $nucleo "*) ;; *) rm -f "$f" ;; esac
done
strings "$LIB"/gambatte_libretro.so | grep -q gambatte_gb_link_mode && echo "gambatte con cable link: sí" || echo "AVISO: gambatte sin cable link"
for d in "$SYS"/usr/share/libretro-database "$SYS"/usr/share/retroarch/shaders "$SYS"/usr/share/retroarch/overlays "$SYS"/usr/share/retroarch/system; do
	rm -rf "$d"; mkdir -p "$d"
done
# Temas de iconos: el menú de Lakka usa XMB con el tema "monochrome".
if [ -d "$SYS"/usr/share/retroarch/assets/xmb ]; then
	for t in "$SYS"/usr/share/retroarch/assets/xmb/*; do
		[ "$(basename "$t")" = monochrome ] || rm -rf "$t"
	done
fi
for t in wallpapers switch ctr nxrgui devices; do rm -rf "$SYS"/usr/share/retroarch/assets/$t; done
du -sh "$SYS"/usr/share/retroarch/assets/* 2>/dev/null | sort -h | tail -12 || true
if [ "${SIN_FIRMWARE_NVIDIA:-0}" = 1 ]; then
	rm -rf "$SYS"/usr/lib/kernel-overlays/base/lib/firmware/nvidia
fi
du -sh "$SYS"/usr/lib/kernel-overlays/base/lib/firmware/nvidia 2>/dev/null || true

echo "== Nuestras piezas"
install -m 0755 "$PIEZAS"/mgba_libretro.so "$LIB"/mgba_libretro.so
install -m 0755 "$PIEZAS"/azahar_libretro.so "$LIB"/azahar_libretro.so
install -m 0644 "$PIEZAS"/azahar_libretro.info "$LIB"/azahar_libretro.info
install -m 0755 "$PIEZAS"/pokemulador_libretro.so "$LIB"/pokemulador_libretro.so
install -m 0644 "$PIEZAS"/pokemulador_libretro.info "$LIB"/pokemulador_libretro.info
install -m 0755 "$PIEZAS"/pokemulador-usb "$SYS"/usr/bin/pokemulador-usb
mkdir -p "$SYS"/usr/lib/pokemulador
install -m 0755 "$PIEZAS"/enlace "$SYS"/usr/lib/pokemulador/enlace
for f in GBA-PK_Multiplayer.lua pokemulador_final.lua LICENSE-GBA-PK.md; do
	install -m 0644 "$PIEZAS"/$f "$SYS"/usr/lib/pokemulador/$f
done

# El sistema arranca nuestro servicio (que abre RetroArch) en lugar de RetroArch directamente.
UNIDAD="$SYS"/usr/lib/systemd/system/retroarch.service
sed -i 's#^ExecStart=/usr/bin/retroarch$#ExecStart=/usr/bin/pokemulador-usb#; s#^Type=notify$#Type=simple#; s#^KillMode=process$#KillMode=mixed#; s#^TimeoutStopSec=10$#TimeoutStopSec=20#' "$UNIDAD"
grep -q "pokemulador-usb" "$UNIDAD" || { echo "no se pudo cambiar retroarch.service"; exit 1; }
cat "$UNIDAD"

# Ajustes de RetroArch por defecto (se usan la primera vez).
CFG="$SYS"/etc/retroarch.cfg
fijar() {
	if grep -q "^$1 = " "$CFG"; then sed -i "s#^$1 = .*#$1 = \"$2\"#" "$CFG"; else echo "$1 = \"$2\"" >> "$CFG"; fi
}
fijar user_language 3
fijar network_cmd_enable true
fijar input_menu_toggle_gamepad_combo 7
fijar quit_press_twice false
fijar pause_nonactive false
fijar menu_show_load_core false
fijar menu_show_load_content false
fijar menu_show_online_updater false
fijar menu_show_core_updater false
fijar rgui_browser_directory /var/media/JUEGOS
fijar video_shader_enable false

echo "== Sistema nuevo"
mksquashfs "$SYS" "$TRABAJO"/SYSTEM -comp zstd -Xcompression-level 19 -b 1M -noappend -no-xattrs -quiet
ls -la "$TRABAJO"/SYSTEM "$TRABAJO"/lakka/SYSTEM

echo "== Imagen"
TAM_SISTEMA=$(du -m --apparent-size "$TRABAJO"/SYSTEM | cut -f1)
TAM_KERNEL=$(du -m --apparent-size "$TRABAJO"/lakka/KERNEL | cut -f1)
TAM_P1=$(( (TAM_SISTEMA + TAM_KERNEL + 48 + 3) / 4 * 4 ))
TOTAL=$(( 1 + TAM_P1 + TAM_DATOS_MB + 1 ))
echo "parte del sistema: ${TAM_P1} MiB, datos: ${TAM_DATOS_MB} MiB, total ${TOTAL} MiB"
rm -f "$SALIDA"
truncate -s ${TOTAL}M "$SALIDA"
sgdisk -o "$SALIDA" >/dev/null
sgdisk -n 1:2048:+${TAM_P1}M -t 1:0700 -c 1:system -A 1:set:2 -A 1:set:63 "$SALIDA" >/dev/null
sgdisk -n 2:0:+${TAM_DATOS_MB}M -t 2:8300 -c 2:storage "$SALIDA" >/dev/null
sgdisk -p "$SALIDA"
NUEVO=$(losetup -fP --show "$SALIDA"); sleep 1
mkfs.vfat -F 32 -n LAKKA -i "$UUID_FAT" "${NUEVO}p1" >/dev/null
mkfs.ext4 -q -F -L LAKKA_DISK -U "$UUID_EXT" -m 0 "${NUEVO}p2"
mkdir -p "$TRABAJO"/p1 "$TRABAJO"/p2
mount "${NUEVO}p1" "$TRABAJO"/p1
cp "$TRABAJO"/lakka/KERNEL "$TRABAJO"/lakka/KERNEL.md5 "$TRABAJO"/p1/
cp "$TRABAJO"/SYSTEM "$TRABAJO"/p1/SYSTEM
echo "$(md5sum "$TRABAJO"/SYSTEM | cut -d' ' -f1)  target/SYSTEM" > "$TRABAJO"/p1/SYSTEM.md5
cp -r "$TRABAJO"/lakka/EFI "$TRABAJO"/p1/
# Menús de arranque: directo, sin esperar.
sed -e 's/^SAY Lakka Boot Menu/SAY Pokemulador/' -e 's/^TIMEOUT 50/TIMEOUT 10/' "$TRABAJO"/lakka/syslinux.cfg > "$TRABAJO"/p1/syslinux.cfg
sed -e 's/^set timeout="25"/set timeout="1"/' -e 's/Run Lakka (live)/Pokemulador/g' "$TRABAJO"/lakka/EFI/BOOT/grub.cfg > "$TRABAJO"/p1/EFI/BOOT/grub.cfg
cat "$TRABAJO"/p1/syslinux.cfg "$TRABAJO"/p1/EFI/BOOT/grub.cfg
cat > "$TRABAJO"/p1/LEEME.txt <<'TXT'
Pokemulador USB (basado en Lakka, https://www.lakka.tv)

Esta parte es el sistema: no la toques. Tus juegos van en la parte JUEGOS.
Para arrancar: con el ordenador apagado, conecta la memoria, enciende y pulsa la tecla del
menú de arranque (F12, F11, F8, F9 o Esc según la marca) y elige la memoria USB.
Si no aparece, desactiva "Secure Boot" en la BIOS.
TXT
sync
umount "$TRABAJO"/p1
# Arranque con BIOS antigua: syslinux en la parte del sistema y su MBR para GPT.
syslinux --install "${NUEVO}p1"
dd if=/usr/lib/syslinux/mbr/gptmbr.bin of="$SALIDA" bs=440 count=1 conv=notrunc status=none
# Datos: vacía (Lakka la prepara al arrancar) y sin la marca de agrandarse, para dejar sitio a JUEGOS.
mount "${NUEVO}p2" "$TRABAJO"/p2
mkdir -p "$TRABAJO"/p2/pokemulador
sync
umount "$TRABAJO"/p2
losetup -d "$NUEVO"
ls -la "$SALIDA"
