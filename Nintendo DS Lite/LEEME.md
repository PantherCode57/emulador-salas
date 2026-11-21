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
- **Peticiones:** son líneas de texto y cada respuesta es `R <n>` y n líneas (`puente/protocolo.go`).
- **WhatsApp** (`puente/wa_real.go`): una sesión propia con whatsmeow, como el WhatsApp de la consola.
  - Se vincula desde la DS sin QR: se escribe el número con el prefijo (34…) y el puente pide a
    WhatsApp un código de 8 letras.
  - El código se mete en el móvil: WhatsApp → Dispositivos vinculados → Vincular con el número de
    teléfono.
  - En la DS: lista de chats con los no leídos, conversación con burbujas, escribir con el teclado y
    cuatro respuestas rápidas. Al abrir un chat, los mensajes quedan leídos también en el móvil.
  - Los emojis que la DS no sabe dibujar salen como se escribían antes (`:)`, `xD`, `<3`…).
- `puente-ds -falso` usa un WhatsApp de mentira; con él, la prueba del PC hace todo el camino de
  verdad por TCP: código malo, código bueno, vincular, chats, escribir y respuesta rápida.
- Las demás apps (Spotin, Instagram, YouTube, Navegador y Tienda) aún dicen «llegará pronto».
- **Falta** dónde va el puente: tiene que ser un aparato de ella (lo más práctico, su móvil, que
  además puede hacer la zona wifi para la DS).

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
