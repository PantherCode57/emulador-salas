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
- Spotin (`spotin/`) y yt-dlp: arrancar Python y cargar yt-dlp tarda unos 16 s en la consola, y antes
  se hacía con cada canción. Ahora `ytdlp_servidor.py` (va dentro del programa) lo deja abierto
  mientras Spotin está abierto, y cada canción tarda lo que tarda YouTube (~3 s). Si falla o se
  cuelga, se pide como antes. El registro (`registro.txt`) ya no se borra al abrir: se va añadiendo
  y se vacía al pasar de 256 KB.
  El YouTube de la consola hace lo mismo (`youtube/ytdlp_servidor.py`, con las mismas opciones que la
  línea de órdenes y la misma respuesta que `yt-dlp -J`): el primer vídeo tarda ~11 s (baja el
  reproductor de YouTube) y los siguientes ~5,5 s, en vez de ~27 s cada uno.
- `ahorro/`: ahorro de batería (`panther-ahorro.service`, como root; se instala con
  `instalar/instalar_ahorro.sh`). Mira cada segundo qué se hace:
  - jugando o viendo vídeos (YouTube, Instagram) no toca nada;
  - en el menú y en las apps ligeras deja la memoria (DMC) hasta 328 MHz;
  - con la pantalla apagada, memoria a 194 MHz y gráfica a 200 MHz;
  - si no tocas nada en 60 s (fuera de juegos y vídeos), baja la luz a un cuarto hasta que pulses algo.
  Medido en el menú: normal ~630 mA; con la memoria baja y poca luz, ~330 mA. El procesador ya baja
  solo («ondemand») y no se toca. `-una-vez` dice qué ve sin cambiar nada.
- `zarpa/`: Zarpa, un juego de carreras hecho para la consola (original: el personaje, los dibujos y
  la música son propios y salen del código). Zarpa, una pantera pequeña con bufanda y mochila, corre
  por una autopista de Los Ángeles al atardecer: tres carriles, vallas que saltar, carteles de
  autopista para pasar agachada, camiones de tacos que esquivar, estrellas, imán y escudo, y trajes
  en la tienda. El récord y las estrellas se guardan en `~/.config/panther/zarpa/progreso.json`.
  Sin internet.
  - Va a 30 imágenes por segundo: los dibujos se hacen al arrancar (2 s) ya escalados, el suelo va
    por filas ya pintadas y los dibujos se pegan saltando lo transparente (`rapido.go`): ~11 ms
    por imagen en la XF35H.
  - `-prueba <carpeta>` dibuja las pantallas; `-medir` dice cuánto tarda cada imagen.
- `claude/`: un chat con la conversación de Claude Code abierta en el PC de casa. La consola no
  lleva a Claude dentro: lo que escribes se deja en `~/.config/panther/claude/pendientes/`, el PC
  lo recoge por SSH (y lo mueve a `leidos/`) y deja la respuesta en `respuestas/`. Mientras el PC
  escucha toca `pc-vivo` cada 2 s, y la app enseña «Conectado al PC», «Escribiendo…» o «No escucha
  ahora». Los mensajes se guardan en `chat.jsonl` (los 300 últimos). Teclado en pantalla como en
  Instagram; START envía.
  - `-prueba <carpeta>` dibuja las pantallas con una conversación inventada.
- `pokedex/`: la Pokédex. Al entrar hay dos botones: «Cartas» y «Juegos» (está por llegar).
  - Cartas: la Pokédex nacional entera (del 1 al 1025) en una lista, con el número y el nombre en
    español de cada Pokémon (`nombres.go`, sacado de los datos de PokeAPI). A marca o desmarca
    «conseguido», y se guarda en `~/.config/panther/pokedex/cartas.json`. L y R saltan de
    generación; izquierda y derecha, de 8 en 8.
  - Y abre el buscador, con el teclado en pantalla: por nombre (sin tildes ni signos: «mr mime»,
    «flabebe») o por número («25»). B quita lo buscado y se queda en el mismo Pokémon.
  - A la izquierda del nombre, el sprite pequeño de cada Pokémon. No van en la app ni en el
    repositorio (son de Nintendo): se bajan de PokeAPI (`raw.githubusercontent.com/PokeAPI/sprites`,
    `POKEDEX_SPRITES`) cuando salen en la lista y se guardan en `~/.config/panther/pokedex/sprites`.
    Mientras la app está abierta va bajando de fondo los que faltan (unos 2 MB en total), así que
    luego salen sin internet. Sin conexión se ve un hueco. Lo demás no necesita internet.
  - `-prueba <carpeta>` dibuja las pantallas con una colección inventada.
- `pasards/`: «Pasar a la DS». Copia Panther OS para la Nintendo DS Lite (`panther.nds`, de la
  carpeta «Nintendo DS Lite» del repositorio) a la microSD de la flashcard. No hace falta lector:
  se usa la ranura de la consola, que es la misma de la tarjeta de Panther OS.
  - `PasarDS.sh` lo copia a la memoria (`/dev/shm/pasards`) y lo arranca desde allí como servicio
    aparte (`systemd-run`). Desde ese momento no necesita nada de la tarjeta.
  - El programa para EmulationStation, quita la memoria de intercambio y deja la tarjeta de Panther
    OS guardada y en solo lectura (`sync` y sysrq «u»).
  - Luego se cambia la tarjeta por la de la flashcard. La nueva se reconoce porque su CID es otro.
    Si la consola no la nota, al pulsar un botón suelta el lector de la ranura y lo vuelve a coger
    (`unbind`/`bind`).
  - En la raíz de la tarjeta de la flashcard solo se añade `panther.nds`. La tarjeta tiene que ser
    FAT32.
  - Al volver a meter la de Panther OS, la reconoce por su CID y reinicia la consola.
  - Si no se pulsa nada en 30 s, se cancela y vuelve el menú.
  - `-prueba <carpeta>` dibuja todas sus pantallas.
- `tienda/`: la Panther Store. Pone y quita con A las apps de Panther OS (WhatsApp, Claude,
  Instagram, Duolingo, Ajedrez, Zarpa, YouTube, Spotin, Pokémon, Pokédex, Navegador y Pasar a la DS) y juegos libres de PortMaster
  que ya vienen con sus datos («ready to run» y sin «power»: Cave Story, Celeste Classic, SuperTux,
  Sonic Robo Blast 2, SRB2Kart, OpenTTD y OpenTyrian), con su icono, de qué va, lo que ocupa y si ya
  está puesto, y el avance mientras se baja. L y R cambian entre apps y juegos; X abre la ficha.
  - Apps: se bajan de la fuente de la tienda (`TIENDA_FUENTE`, con un `tienda.json` con el tamaño
    y el sha256 de cada pieza; por defecto, la release `panther-apps` del repositorio, que pone al
    día `consola.yml` en cada subida a `desarrollo`) y, si no hay, se copian de `~/panther-instalar`. Al quitar una app se
    guarda tal cual en `~/panther/.guardadas/<app>` (las de `~/panther-instalar` pueden ser más
    viejas) y al volver a ponerla sale la misma; la sesión de `~/.config/panther` no se toca. De
    WhatsApp solo se quita la tarjeta: su programa es el servicio que pinta la hora, la batería y el
    volumen. Si se quita Spotin, YouTube se queda con su yt-dlp.
  - Juegos: como PortMaster, en `/roms/ports` (el .sh y su carpeta). Lee el `ports.json` de
    PortMaster (cada 12 h) para la dirección, el md5 y el tamaño, y deja de ofrecer los que dejen
    de ser «rtr» o pidan «power» o «2gb». Los apunta en `/roms/ports/gamelist.xml` con su nombre y
    su descripción en español y les hace la tarjeta de la lista (`images/panther/`), como en los
    otros apartados. Las capturas salen del repositorio de PortMaster. Al quitar un juego se borran
    sus partidas (están en su carpeta), y lo pregunta.
  - Se pone al día sola: al abrirla, si la fuente tiene otra tienda (la pieza `tienda` de
    `tienda.json`), la baja comprobando su suma, deja la de antes en `tienda.antes` y se abre con la
    nueva. La de la consola de antes del 21/11/2026 no sabía: se la pone `Pokemon.sh` una vez
    (busca «PantherStore/2» en el programa), así que basta con quitar y poner Pokémon.
  - Las apps que llegan después de la instalación (la Pokédex) se dan de alta al ponerlas, como
    harían `menus.py`, `ajustes_menu.py` y `tema.py`: su apartado en `es_systems.cfg` (con sudo;
    copia en `*.antes-tienda`), su tarjeta (la dibuja la tienda con su color, su icono y su nombre)
    y el `theme.xml` de su apartado, y al reiniciar EmulationStation (parado), que se abra
    directamente y su sitio en la pantalla principal. Con las que ya estaban no cambia nada.
  - Las tarjetas: EmulationStation solo las cambia al reiniciarse. Al salir de la tienda, si has
    cambiado algo, deja en marcha (`systemd-run`) `tienda -reiniciar-es <pid>`, que espera a que la
    tienda se cierre y a que no haya ninguna app ni juego abierto, y lo reinicia.
  - `-prueba <carpeta>` dibuja las pantallas con datos inventados; `-tarjeta <png>` dibuja su
    tarjeta (264×152) con las letras del tema; `-lista`, `-poner <id>` y `-quitar <id>` hacen lo
    mismo que los botones por SSH (sin reiniciar EmulationStation).
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
