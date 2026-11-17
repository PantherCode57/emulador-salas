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
- `instagram/`: Instagram con tu cuenta (la cookie de instagram.com se pega desde el PC en
  `http://IP:8767`): tu inicio (fotos, de varias fotos y vídeos), los reels, las historias y tus
  mensajes, con «me gusta» y respuestas con el teclado en pantalla. Habla con Instagram como su web
  (GraphQL con doc_id y api/v1); los vídeos los lee ffmpeg de su dirección y las fotos solo van a la
  memoria. Al ver historias o abrir chats no se marcan como vistos.
  - `-prueba <carpeta>` dibuja las pantallas con datos inventados.
  - `-probar` dice, con la sesión guardada, qué contesta Instagram a cada pestaña (solo cuántas).
  - Al pegar la cookie prueba varias direcciones y no se rinde con la primera que falla. Solo la
    rechaza si Instagram dice que no vale: manda al login o a la portada, o pide confirmar que eres
    tú (`/challenge`). Si solo pone límites (429), la guarda igual y avisa. El registro apunta
    adónde manda cada 302, sin lo que va detrás de «?».
- `claude/`: un chat con la conversación de Claude Code abierta en el PC de casa. La consola no
  lleva a Claude dentro: lo que escribes se deja en `~/.config/panther/claude/pendientes/`, el PC
  lo recoge por SSH (y lo mueve a `leidos/`) y deja la respuesta en `respuestas/`. Mientras el PC
  escucha toca `pc-vivo` cada 2 s, y la app enseña «Conectado al PC», «Escribiendo…» o «No escucha
  ahora». Los mensajes se guardan en `chat.jsonl` (los 300 últimos). Teclado en pantalla como en
  Instagram; START envía.
  - `-prueba <carpeta>` dibuja las pantallas con una conversación inventada.
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
