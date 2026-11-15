# Consola portátil (Panther OS)

Lo que se le pone a una consola portátil con Linux (probado en una Game Console XF35H, RK3326,
640×480, con dArkOS4Clone 20261008), además de los juegos:

- `whatsapp/`: WhatsApp ligero. Lleva la sesión con [whatsmeow](https://github.com/tulir/whatsmeow)
  (se vincula como dispositivo escaneando un QR) y dibuja la pantalla. RetroArch, con el núcleo de
  la pantalla de salas (`lakka/nucleo`), solo la enseña y le pasa los botones. No usa navegador:
  gasta unos 50 MB de memoria.
  - Chats, conversación, teclado en pantalla y respuestas rápidas (`respuestas.txt`).
  - Avisos de mensajes nuevos.
  - `-prueba <carpeta>` dibuja las pantallas con datos inventados.
  - `-diagnostico 40s` pide un QR sin pantalla y con el registro detallado.
- `arranque/`: animación de arranque que dibuja en `/dev/fb0` a 60 fps antes de EmulationStation.
  Las letras se cargan de `/usr/local/share/panther/arranque` (`titulo.png`, `subtitulo.png`,
  `posiciones.txt`); no están en el repositorio.
- `instalar/`: instalación en la consola (por SSH, como el usuario `ark`):
  - `instalar.sh`: compila allí el núcleo de la pantalla e instala el WhatsApp, el tema, la
    animación (`panther-arranque.service`, en lugar de `welcome-message.service`) y los menús.
  - `menus.py`: nombres cortos en español, apartados «WhatsApp» y «Salas» en
    `es_systems.cfg`, e idioma español. Guarda antes una copia `*.panther-original`.
  - `WhatsApp.sh`: lo que abre EmulationStation. Usa `/opt/retroarch/bin/retroarch`, no el script
    `/usr/local/bin/retroarch`, que lo abre aparte y al cerrarlo dejaba RetroArch encima del menú.

Compilar para la consola (Go, sin cgo):

    cd consola/whatsapp && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" .
    cd consola/arranque && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o panther-arranque .

Las salas (`lakka/servicio` para arm64) están pendientes: el servicio lleva la clave de Tailscale
dentro, y no debe acabar en un artefacto descargable de un repositorio público.
