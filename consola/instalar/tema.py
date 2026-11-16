#!/usr/bin/env python3
"""Tema «es-theme-panther» para EmulationStation (dArkOS4Clone en la XF35H).

Es una copia de es-theme-nes-box con otra pantalla de apartados (foto de fondo, hora grande y fecha
arriba a la izquierda, engranaje, Wi-Fi y batería arriba a la derecha y abajo una barra de tarjetas
de colores, una por apartado) y otras listas de juegos (tarjetas en columna y la hora en pequeño).
Necesita el menú con los cambios de Panther OS (consola/emulationstation/0001-panther.patch): usa
elementos y propiedades que el menú original no conoce.
Las listas copian al píxel la vista previa aprobada: las tarjetas de los juegos ya vienen
recortadas, con las esquinas redondas y la sombra (<thumbnail> de cada juego), y las medidas salen
de lista_exacta.py, que calcula dónde pone el menú cada texto. La letra es Selawik (Microsoft,
licencia SIL OFL), la versión libre de Segoe UI.
En la pantalla principal solo hay tres tarjetas en fila fija (Favoritos, WhatsApp y Emuladores);
las consolas van dentro de Emuladores, cada una con su tarjeta de la lista (consolas/<tema>.png).
Uso: tema.py <carpeta con fondo.jpg, fondo-lista.jpg, borde-tarjeta.png, borde-ajustes.png,
linea.png, wifi.png, ajustes.png, marca-barra.png, letra/*.ttf, bateria/*.png, tarjetas/*.png y
consolas/*.png>
Se lanza con EmulationStation parado. No toca el tema original.
"""
import re
import shutil
import sys
from pathlib import Path

TEMAS = Path("/roms/themes")
ORIGEN, DESTINO = TEMAS / "es-theme-nes-box", TEMAS / "es-theme-panther"
piezas = Path(sys.argv[1])

PANTHER_XML = """<theme>
  <formatVersion>7</formatVersion>
  <!-- Panther OS: en la pantalla principal, hora grande y fecha a la izquierda; en las listas de
       juegos («_lista»), la hora en pequeño arriba a la derecha y la batería a su izquierda. -->
  <view name="screen">
    <text name="clock">
      <pos>0.065 0.05</pos>
      <size>0.6 0.25</size>
      <fontPath>./hora.ttf</fontPath>
      <fontSize>0.23</fontSize>
      <color>FFFFFFFF</color>
      <alignment>left</alignment>
      <verticalAlignment>center</verticalAlignment>
      <lineSpacing>1.5</lineSpacing>
    </text>
    <text name="date">
      <pos>0.072 0.275</pos>
      <size>0.6 0.07</size>
      <fontPath>./hora.ttf</fontPath>
      <fontSize>0.05</fontSize>
      <color>FFFFFFE0</color>
      <alignment>left</alignment>
    </text>
    <!-- En las listas solo la hora, arriba a la derecha; la batería se aparta fuera de la pantalla. -->
    <text name="clock_lista">
      <pos>0.71875 0.0375</pos>
      <size>0.25 0.091667</size>
      <fontPath>./letra/selawksl.ttf</fontPath>
      <fontSize>0.0625</fontSize>
      <color>FFFFFFFF</color>
      <alignment>right</alignment>
      <verticalAlignment>top</verticalAlignment>
      <lineSpacing>1</lineSpacing>
    </text>
    <batteryIndicator name="batteryIndicator_lista">
      <pos>0 2</pos>
      <size>0.47 0.085</size>
    </batteryIndicator>
    <!-- Wi-Fi, batería y su porcentaje (ShowBattery = "text"), alineados a la derecha. -->
    <batteryIndicator name="batteryIndicator">
      <pos>0.5 0.025</pos>
      <size>0.47 0.085</size>
      <horizontalAlignment>right</horizontalAlignment>
      <itemSpacing>0.012</itemSpacing>
      <color>FFFFFFFF</color>
      <networkIcon>./wifi.png</networkIcon>
      <incharge>./bateria/incharge.png</incharge>
      <full>./bateria/full.png</full>
      <at75>./bateria/at75.png</at75>
      <at50>./bateria/at50.png</at50>
      <at25>./bateria/at25.png</at25>
      <empty>./bateria/empty.png</empty>
    </batteryIndicator>
  </view>
  <!-- Pantalla de apartados: foto, barra de tarjetas abajo y nada más encima. Con ↑ se elige la
       barra de arriba: el engranaje (opciones del sistema) o el Wi-Fi (pantalla de Wi-Fi). -->
  <view name="system">
    <image name="topbar_ajustes">
      <path>./ajustes.png</path>
      <pos>0.71 0.025</pos>
      <size>0.064 0.085</size>
    </image>
    <image name="topbar_marca">
      <path>./marca-barra.png</path>
    </image>
    <text name="topbar_texto">
      <fontSize>0.034</fontSize>
      <color>FFFFFFFF</color>
    </text>
    <image name="background" extra="true">
      <origin>0 0</origin>
      <pos>0 0</pos>
      <size>1 1</size>
      <path>./fondo.jpg</path>
      <zIndex>1</zIndex>
    </image>
    <image name="background_bottom" extra="true">
      <pos>0 2</pos>
      <size>0.001 0.001</size>
    </image>
    <image name="background_icon" extra="true">
      <pos>0 2</pos>
      <size>0.001 0.001</size>
    </image>
    <!-- Cinco tarjetas quietas y centradas (fixedRow): se mueve solo la elegida, que crece. El
         carrusel mide todo el ancho, así que de una tarjeta a la siguiente hay 0.2 (128 px). -->
    <carousel name="systemcarousel">
      <type>horizontal</type>
      <pos>0 0.775</pos>
      <size>1 0.21</size>
      <color>00000000</color>
      <logoAlignment>center</logoAlignment>
      <logoSize>0.17 0.13</logoSize>
      <logoScale>1.15</logoScale>
      <maxLogoCount>5</maxLogoCount>
      <fixedRow>true</fixedRow>
      <!-- las tarjetas no elegidas, opacas (sin el cambio del menú quedaban a medio transparentar) -->
      <minLogoOpacity>1</minLogoOpacity>
      <zIndex>40</zIndex>
    </carousel>
    <text name="systemInfo">
      <pos>0 2</pos>
      <size>0.001 0.001</size>
    </text>
    <helpsystem name="help">
      <pos>0 2</pos>
    </helpsystem>
  </view>
  <!-- Listas de juegos (vista «grid», puesta por apartado con <apartado>.defaultView), igual que la
       vista previa aprobada: columna de tres tarjetas a la izquierda (la elegida en el centro, un 12 %
       más grande y con el borde azul) y, a la derecha, el nombre, «consola · año · género», una línea
       fina, de qué va y la ayuda de los botones. Medidas de lista_exacta.py; el bloque de texto
       de la derecha va 65 px más abajo que en la vista previa: en medio de la pantalla y un poco
       hacia arriba (con 85 px quedaría centrado para una descripción de 4 líneas, la mayoría). -->
  <view name="grid">
    <image name="panther_fondo_lista" extra="true">
      <origin>0 0</origin>
      <pos>0 0</pos>
      <size>1 1</size>
      <path>./fondo-lista.jpg</path>
      <zIndex>1</zIndex>
    </image>
    <image name="panther_linea" extra="true">
      <origin>0 0</origin>
      <pos>0.453125 0.40625</pos>
      <size>0.509375 0.002083</size>
      <path>./linea.png</path>
      <zIndex>35</zIndex>
    </image>
    <text name="panther_ayuda" extra="true">
      <text>A  Jugar      B  Volver      Y  Favorito</text>
      <pos>0.453125 0.934375</pos>
      <size>0.509375 0.054167</size>
      <fontPath>./letra/selawk.ttf</fontPath>
      <fontSize>0.03125</fontSize>
      <color>BEBEC8FF</color>
      <alignment>left</alignment>
      <verticalAlignment>top</verticalAlignment>
      <lineSpacing>1</lineSpacing>
      <zIndex>40</zIndex>
    </text>
    <imagegrid name="gamegrid">
      <pos>-0.01875 0</pos>
      <size>0.440625 1.018973</size>
      <margin>0 0.032887</margin>
      <padding>0.034068 0.02686 0.034068 0</padding>
      <autoLayout>1 3</autoLayout>
      <autoLayoutSelectedZoom>1.12</autoLayoutSelectedZoom>
      <imageSource>thumbnail</imageSource>
      <scrollDirection>verticalCenter</scrollDirection>
      <animateSelection>true</animateSelection>
      <zIndex>30</zIndex>
    </imagegrid>
    <!-- La imagen de cada tarjeta ya trae las esquinas redondas y la sombra: se pinta tal cual. -->
    <gridtile name="default">
      <padding>0 0</padding>
      <backgroundColor>00000000</backgroundColor>
      <imageColor>FFFFFFFF</imageColor>
      <selectionMode>image</selectionMode>
    </gridtile>
    <gridtile name="selected">
      <padding>0 0</padding>
      <backgroundColor>00000000</backgroundColor>
      <imageColor>FFFFFFFF</imageColor>
      <selectionMode>image</selectionMode>
    </gridtile>
    <image name="gridtile.image">
      <pos>0.5 0.5</pos>
      <size>1 1</size>
      <color>FFFFFFFF</color>
    </image>
    <image name="gridtile.image:selected">
      <pos>0.5 0.5</pos>
      <size>1 1</size>
      <color>FFFFFFFF</color>
    </image>
    <!-- La capa de encima solo existe si está en las tarjetas normales: ahí va oculta. -->
    <image name="gridtile.overlay">
      <path>./borde-tarjeta.png</path>
      <origin>0.5 0.5</origin>
      <pos>0.5 0.5</pos>
      <size>1 1</size>
      <visible>false</visible>
    </image>
    <image name="gridtile.overlay:selected">
      <visible>true</visible>
    </image>
    <text name="gridtile">
      <pos>0 0.914699</pos>
      <size>1 0.148434</size>
      <fontPath>./letra/selawk.ttf</fontPath>
      <fontSize>0.027084</fontSize>
      <color>CDCDD7FF</color>
      <alignment>center</alignment>
      <verticalAlignment>top</verticalAlignment>
      <lineSpacing>1</lineSpacing>
    </text>
    <text name="gridtile:selected">
      <pos>0 0.906627</pos>
      <size>1 0.144578</size>
      <fontPath>./letra/selawksb.ttf</fontPath>
      <fontSize>0.029167</fontSize>
      <color>FFFFFFFF</color>
    </text>
    <text name="md_name">
      <pos>0.453125 0.2875</pos>
      <size>0.509375 0.091667</size>
      <fontPath>./letra/selawksb.ttf</fontPath>
      <fontSize>0.054167</fontSize>
      <color>FFFFFFFF</color>
      <alignment>left</alignment>
      <verticalAlignment>top</verticalAlignment>
      <lineSpacing>1</lineSpacing>
      <zIndex>40</zIndex>
    </text>
    <text name="md_genre">
      <pos>0.453125 0.357292</pos>
      <size>0.509375 0.054167</size>
      <fontPath>./letra/selawk.ttf</fontPath>
      <fontSize>0.03125</fontSize>
      <color>96C8FFFF</color>
      <alignment>left</alignment>
      <verticalAlignment>top</verticalAlignment>
      <lineSpacing>1</lineSpacing>
      <zIndex>40</zIndex>
    </text>
    <text name="md_description">
      <pos>0.453125 0.428125</pos>
      <size>0.509375 0.480208</size>
      <fontPath>./letra/selawk.ttf</fontPath>
      <fontSize>0.033334</fontSize>
      <color>E6E6EEFF</color>
      <alignment>left</alignment>
      <lineSpacing>1.4118</lineSpacing>
      <zIndex>40</zIndex>
    </text>
    <text name="md_lbl_rating"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_releasedate"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_developer"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_publisher"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_genre"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_players"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_lastplayed"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_lbl_playcount"><pos>0 2</pos><size>0.001 0.001</size></text>
    <rating name="md_rating"><pos>0 2</pos><size>0.001 0.001</size></rating>
    <datetime name="md_releasedate"><pos>0 2</pos><size>0.001 0.001</size></datetime>
    <text name="md_developer"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_publisher"><pos>0 2</pos><size>0.001 0.001</size></text>
    <text name="md_players"><pos>0 2</pos><size>0.001 0.001</size></text>
    <datetime name="md_lastplayed"><pos>0 2</pos><size>0.001 0.001</size></datetime>
    <text name="md_playcount"><pos>0 2</pos><size>0.001 0.001</size></text>
    <!-- La ayuda de los botones es el texto «panther_ayuda» de arriba. -->
    <helpsystem name="help">
      <pos>0 2</pos>
    </helpsystem>
  </view>
</theme>
"""

# Emuladores: la lista es de consolas, así que la ayuda dice «Abrir» en vez de «Jugar».
AYUDA_EMULADORES = """  <view name="grid">
    <text name="panther_ayuda">
      <text>A  Abrir      B  Volver      Y  Favorito</text>
    </text>
  </view>
"""

LOGO_NUEVO = """<theme>
  <formatVersion>7</formatVersion>
  <include>./../theme.xml</include>
  <view name="system">
    <image name="logo">
      <path>{tarjeta}</path>
    </image>
  </view>
</theme>
"""

if DESTINO.exists():
    shutil.rmtree(DESTINO)
shutil.copytree(ORIGEN, DESTINO, ignore=shutil.ignore_patterns(".git"))
p = DESTINO / "_panther"
shutil.copytree(piezas, p)
shutil.copy2("/usr/bin/emulationstation/resources/OpenSans-CondLight.ttf", p / "hora.ttf")
(p / "panther.xml").write_text(PANTHER_XML, encoding="utf-8")

raiz = DESTINO / "theme.xml"
t = raiz.read_text(encoding="utf-8", errors="replace")
t = t.replace("</theme>", "    <include>./_panther/panther.xml</include>\n</theme>")
raiz.write_text(t, encoding="utf-8")

for tarjeta in sorted((p / "tarjetas").glob("*.png")):
    sistema = tarjeta.stem
    ruta = f"./../_panther/tarjetas/{tarjeta.name}"
    # Las consolas ya no salen en la pantalla principal: su «logo» es la tarjeta de la lista de
    # Emuladores (el menú la usa como imagen de cada consola).
    if (p / "consolas" / tarjeta.name).exists():
        ruta = f"./../_panther/consolas/{tarjeta.name}"
    xml = DESTINO / sistema / "theme.xml"
    if xml.exists():
        x = xml.read_text(encoding="utf-8", errors="replace")
        # Solo el logo de la pantalla de apartados (el carrusel); el de las listas se queda.
        def cambiar(m):
            return re.sub(r'(<image name="logo">.*?<path>)[^<]*(</path>)', lambda n: n.group(1) + ruta + n.group(2),
                          m.group(0), count=1, flags=re.S)
        x2 = re.sub(r'<view name="system">.*?</view>', cambiar, x, count=1, flags=re.S)
        if x2 == x:  # no tenía logo en la vista de apartados: se añade
            x2 = x.replace("</theme>", f'  <view name="system">\n    <image name="logo">\n      <path>{ruta}</path>\n    </image>\n  </view>\n</theme>')
        xml.write_text(x2, encoding="utf-8")
    else:
        xml.parent.mkdir(exist_ok=True)
        x2 = LOGO_NUEVO.format(tarjeta=ruta)
        if sistema == "Emuladores":
            x2 = x2.replace("</theme>", AYUDA_EMULADORES + "</theme>")
        xml.write_text(x2, encoding="utf-8")
    print("tarjeta:", sistema)
print("tema listo:", DESTINO)
