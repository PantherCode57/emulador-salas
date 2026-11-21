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
  como en la consola. El puente es un programa en otro aparato de la misma red (la consola, el PC o el
  móvil) que hace el trabajo y le pasa a la DS lo justo para enseñarlo: los mensajes, las fotos en
  pequeño, la música en poca calidad, el vídeo muy pequeño y la página web ya dibujada.
  - El puente **todavía no está hecho**.
  - La DS ya lo busca: cuando está conectada, cada 2 segundos manda `PANTHER-DS 1` por UDP a toda la
    red en el puerto 8790. El puente tiene que contestar `PANTHER-PUENTE 1 <nombre>`.
  - Mientras no haya puente, cada app dice lo que le falta: wifi, puente o la propia app.

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
