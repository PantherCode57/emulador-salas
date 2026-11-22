# Panther OS para la Nintendo DS Lite

Panther OS para una Nintendo DS Lite con flashcard. Es un programa de DS (`panther.nds`) que se abre
desde el menú de la flashcard. Por ahora tiene:

- **Arranque**: la línea que crece desde el centro y el nombre que sube, como en la consola.
- **Pantalla de inicio**: arriba la hora, la fecha, la batería y el wifi. Abajo las apps: WhatsApp,
  Spotin, Instagram, YouTube, Navegador y Tienda. Se abren tocándolas o con la cruceta y A.
- **Wifi**: lista de las redes que hay cerca y conexión a una. La red elegida se apunta en la tarjeta
  (`panther/wifi.txt`) y al encender la DS se conecta sola. También valen las redes guardadas en los
  ajustes de Nintendo WFC de la DS.
- **Teclado en pantalla** (táctil o con cruceta) con mayúsculas, ñ y símbolos.

## Lo que la DS Lite no puede hacer sola

- **El wifi solo entra en redes abiertas (sin contraseña) o con WEP.** Con WPA o WPA2, que es lo que
  usa casi cualquier router de casa, no se conecta. Es una limitación del chip de wifi de la DS y no se
  arregla con programas. Lo más fácil es usar la zona wifi del móvil sin contraseña mientras se usa la
  DS.
- **WhatsApp, Spotin, Instagram, YouTube y el navegador necesitan un «puente».** La DS tiene 4 MB de
  memoria y un procesador de 67 MHz, y su wifi da como mucho 1-2 Mbit/s. No le da para abrir esas apps
  como en la consola. El puente es un programa en otro aparato de la misma red que hace el trabajo y
  le pasa a la DS lo justo para enseñarlo. Tiene que ir en un aparato **de la dueña de la DS y con sus
  cuentas**, nunca con las de otra persona.

## El puente (`puente/`, en Go)

- **Buscarlo:** la DS, cuando tiene wifi, manda `PANTHER-DS 1` por UDP a toda la red (puerto 8790) y
  el puente contesta `PANTHER-PUENTE 1 <nombre>`.
- **Código y cifrado:** luego se conectan por TCP en el mismo puerto. El puente tiene un código de 6
  cifras (`<datos>/codigo`, sale en su registro al arrancar) que se escribe en la DS una vez y se
  guarda en `panther/puente.txt`. Todo va cifrado con ChaCha20; la clave de cada conexión sale del
  código y de 16 bytes al azar de cada lado (`puente/cifrado.go` y `source/cifrado.c`). Sin el código
  no se entra, y con 5 códigos malos seguidos esa IP queda fuera 5 minutos. Así nadie más de un wifi
  abierto puede leer los chats.
- **Peticiones:** son líneas de texto y cada respuesta es `R <n>` y n líneas, o `B <n>` y n bytes si
  es una foto (`puente/protocolo.go`).
- **WhatsApp** (`puente/wa_real.go`): una sesión propia con whatsmeow, como el WhatsApp de la consola.
  - Se vincula desde la DS sin QR: se escribe el número con el prefijo (34…) y el puente pide a
    WhatsApp un código de 8 letras.
  - El código se mete en el móvil: WhatsApp → Dispositivos vinculados → Vincular con el número de
    teléfono.
  - En la DS: lista de chats con los no leídos, conversación con burbujas, escribir con el teclado y
    cuatro respuestas rápidas. Al abrir un chat, los mensajes quedan leídos también en el móvil.
  - Los emojis que la DS no sabe dibujar salen como se escribían antes (`:)`, `xD`, `<3`…).
- **Instagram** (`puente/instagram.go`): el mismo Instagram que la app de la consola (`puente/ig/` son
  copias de `consola/instagram`; si allí se arregla algo, se vuelve a copiar).
  - La sesión la pone ella en su móvil: en la app Puente DS, «Entrar en Instagram» abre la web de
    Instagram dentro de la app, escribe su usuario y su contraseña allí y la sesión pasa al puente.
  - En la DS hay tres pestañas: **Inicio** (la foto arriba; abajo quién, el texto, «me gusta», las
    fotos de cada publicación con ◄► y los vídeos con X), **Historias** (pasan solas o tocando a los
    lados) y **Mensajes** (como WhatsApp, con respuestas rápidas).
- **YouTube** (`puente/youtube.go`): buscar con el teclado, la lista con la miniatura arriba y verlos.
  Lo busca la app de Android con NewPipe Extractor (la librería de la app NewPipe). Sin cuenta.
- **Fotos** (`IMG`, `puente/imagenes.go`): la DS no ve direcciones, sino claves cortas; el puente baja
  la foto y se la manda ya en JPEG pequeño, a la medida de sus pantallas. La DS los lee con
  `source/jpeg.c`.
- **Vídeos** (`VER`, `puente/video.go` y `source/video.c`): van por otra conexión con el mismo
  saludo. El móvil descodifica el vídeo con lo que trae (MediaCodec) y el puente le manda a la DS
  imágenes JPEG de 256×144 como mucho (unas 12 por segundo) y el sonido en ADPCM a 16 kHz. Manda el
  sonido: la imagen que se ve es la que toca según lo que lleva sonado. La DS le dice al puente por
  dónde va; si el wifi no da para todo, el puente se salta imágenes y baja la calidad, nunca el
  sonido. Se puede pausar y saltar 10 s (◄►) o tocar la barra.
- `puente-ds -falso` usa WhatsApp, Instagram y YouTube de mentira; con él, la prueba del PC hace todo
  el camino de verdad por TCP: código malo, código bueno, vincular, chats, escribir, respuesta rápida,
  el inicio de Instagram con sus fotos, un vídeo, las historias, los mensajes, buscar en YouTube, la
  miniatura y un vídeo entero con pausa.
- Spotin, el navegador y la tienda aún dicen «llegará pronto».

## La app del móvil (`android/`, «Puente DS»)

- Lleva el puente de Go dentro (como `libpuente.so`) y lo tiene en marcha en segundo plano, con un
  aviso fijo. Arranca sola al encender el móvil. Desde Android 5.
- Lleva también el «ayudante» (`Ayudante.java`), lo que el puente de Go no puede hacer en Android:
  buscar en YouTube y sacar sus vídeos (NewPipe Extractor), y descodificar los vídeos (`Video.java`).
  Solo escucha dentro del móvil y con una clave.
- En su pantalla: el código para la DS, la IP del móvil, Instagram (entrar o salir) y el registro.
- GitHub la compila y la prueba en un emulador de Android (`.github/workflows/nintendo-ds-android.yml`):
  se le habla como la DS y se le pide un vídeo de prueba de 4 s, que descodifica Android.

## Ponerlo en la flashcard

1. Saca la tarjeta microSD de la flashcard y métela en el PC.
2. Copia `panther.nds` a la tarjeta (en la raíz o en cualquier carpeta).
3. Vuelve a meter la tarjeta en la flashcard, enciende la DS y abre «Panther OS» en el menú.

La flashcard se encarga de que el programa pueda leer y escribir en la tarjeta (el «parche DLDI»). Si
no lo hace, Panther OS funciona igual, pero no se acuerda del wifi.

## Cómo está hecho

- Está escrito en C con [BlocksDS](https://blocksds.skylyrac.net), que es libre. Usa libnds para las
  pantallas, los botones y la tarjeta, y DSWiFi para el wifi.
- Las letras son las de Go (gofont, licencia BSD), las mismas que usa la Pokédex de la consola.
  `letras/main.go` las dibuja en varios tamaños y las guarda en `generado/letras.c`.
- `source/`: todo el sistema.
  - `sistema.c`: el arranque, la pantalla de inicio y el paso de una pantalla a otra.
  - `apps.c`: las seis apps y sus iconos. Los iconos son dibujos sencillos, no los logotipos de cada
    marca.
  - `wa.c`, `ig.c` y `yt.c`: WhatsApp, Instagram y YouTube; `video.c`: los vídeos; `jpeg.c`: las fotos.
  - `puente.c` y `cifrado.c`: la conexión con el puente.
  - `wifi.c`, `teclado.c`, `ui.c` (las piezas comunes) y `dibujo.c` (formas y letras).
  - `plataforma_ds.c`: lo que habla con la DS de verdad. Es lo único que depende de la DS.
- `pc/prueba.c`: hace de DS en el PC (botones, toques, un wifi de mentira y un puente de mentira).
  Pasa por todas las pantallas, saca una foto de cada una y comprueba que la clave del wifi llega bien,
  que `wifi.txt` se guarda y que al encender se conecta sola.
- `icono.gif` es el icono del menú de la flashcard (32×32, 16 colores). Lo hace `icono.py`.
- GitHub lo compila todo con `.github/workflows/nintendo-ds.yml` cada vez que cambia esta carpeta. Deja
  `panther.nds` y las fotos de la prueba en el artefacto `panther-ds`.

Para compilarlo a mano hacen falta Go y BlocksDS (por ejemplo con Docker). `make` no se lleva bien con
los espacios del nombre de la carpeta, así que primero se copia a otro sitio:

```sh
cp -r "Nintendo DS Lite" /tmp/ds && cd /tmp/ds
(cd letras && go run . -salida ../generado/letras.c)
docker run --rm -v "$PWD":/work -w /work skylyrac/blocksds:slim-v1.25.0 make
```
