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
  - Basta con pegar el valor de `sessionid` (F12 → Aplicación → Cookies → instagram.com). La app
    saca `ds_user_id` del propio sessionid (empieza por el número de la cuenta). El `csrftoken` lo
    coge de instagram.com al abrirlo con esa sesión, y si no se lo da, pone uno al azar. También
    valen la cabecera `cookie` entera, un `cookies.txt` o filas copiadas de la tabla de F12.
- `duolingo/`: Duolingo con tu cuenta (el valor de la cookie `jwt_token` de duolingo.com se pega
  desde el PC o el móvil en `http://IP:8768`): tu camino (secciones, unidades y niveles), tu racha,
  tus gemas, tus vidas, la meta del día y tu liga, y las lecciones con los botones. Al acabar se
  mandan a Duolingo, así que suman XP y racha como en el móvil. Habla con Duolingo como su web
  (`2017-06-30/users`, y `sessions` con POST para empezar y PUT para acabar), con las mismas
  peticiones que DuolingoCLI (un cliente libre de terminal).
  - Ejercicios: elegir (también rellenar un hueco con una opción), formar la frase con fichas,
    escuchar y formarla, rellenar huecos con fichas, emparejar y escribir con el teclado en pantalla.
    Las voces son las de Duolingo (se bajan a la memoria al empezar y suenan con ffmpeg y aplay).
  - No se piden los ejercicios de hablar (no hay micrófono). Las historias y los cofres se hacen
    en el móvil. Los niveles ya hechos se repasan con una práctica, y X practica desde el camino.
  - Lo que fallas vuelve al final de la lección, como en Duolingo. Las tildes y un error de una
    letra se perdonan, avisando.
  - `-prueba <carpeta>` dibuja las pantallas con datos inventados.
  - `-probar` dice, con la sesión guardada, qué contesta Duolingo (cuántas unidades, de qué tipo
    son los ejercicios). Empieza una lección pero no la manda, así que no cuenta.
  - **Ajedrez:** los cursos que no son de idiomas solo salen en `2023-05-23/users`, y la app los
    lee de ahí si en `2017-06-30` no hay curso. Los ejercicios de ajedrez llegan con la posición
    (`fen`) y `chessPuzzleInfo` (`correctMoves`, `enemyMoves`, `validPaths`,
    `moveEvaluationsForPositions`, `maxMoves`), con las jugadas en UCI. Se juegan en un tablero:
    la cruceta mueve el cursor, A coge y deja la pieza e Y da una pista. Las respuestas del rival
    las pone la consola. En los de estrellas, los peones negros son las estrellas que hay que comerse.
    Las piezas se dibujan con la letra DejaVu Sans de la consola (`DUOLINGO_PIEZAS`).
  - `-volcar <archivo>` guarda cómo es tu curso y cómo llega una lección (la pide, pero no la
    manda), sin tu nombre ni tu usuario: sirve para preparar los cursos nuevos.
- `ajedrez/`: ajedrez con tu cuenta de Lichess, por su API oficial (la «Board API», que es para
  jugar desde programas y tableros que no son su web). El token personal (`lip_…`) se hace en
  Lichess con el enlace de la página de la consola (`http://IP:8769`, con los permisos ya marcados)
  y se guarda en `~/.config/panther/ajedrez/cuenta.json`.
  - Retar a un amigo por su nombre (o a los últimos retados y a quien sigues), con reloj (5+3,
    10+0, 15+10, 30+0) o por días; él juega desde el móvil o la web de Lichess. Los retos con reloj
    se mantienen con `keepAliveStream` mientras se espera.
  - Contra la máquina de Lichess (Stockfish, niveles 1 a 8), seguir las partidas empezadas y
    aceptar o rechazar los retos que llegan (por el stream `/api/stream/event`).
  - La partida llega por `/api/board/game/stream/<id>`; las jugadas van en UCI. Las reglas
    (`reglas.go`, comprobadas con perft) no dejan mandar jugadas que no valen, y las jugadas se
    escriben en español (Cf3, 0-0, Dh4#). Con START: tablas, rendirse, anular o salir.
  - Problemas de Lichess (`/api/puzzle/next`), con pista (Y) y el resultado mandado a Lichess.
  - `-prueba <carpeta>` dibuja las pantallas con datos inventados; `-probar` dice, con el token
    guardado, qué contesta Lichess (solo si va y cuántos).
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
