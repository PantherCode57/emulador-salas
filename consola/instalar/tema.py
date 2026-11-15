#!/usr/bin/env python3
"""Tema «es-theme-panther» para EmulationStation (dArkOS4Clone en la XF35H).

Es una copia de es-theme-nes-box (las listas de juegos se quedan igual) con otra pantalla de
apartados: foto de fondo, hora grande arriba a la izquierda, batería y Wi-Fi arriba a la derecha, y
abajo una barra horizontal de tarjetas de colores (una por apartado).
Uso: tema.py <carpeta con fondo.jpg, wifi.png, bateria/*.png y tarjetas/*.png>
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
  <!-- Panther OS: hora grande y batería en todas las pantallas. -->
  <view name="screen">
    <text name="clock">
      <pos>0.065 0.05</pos>
      <size>0.6 0.25</size>
      <fontPath>./hora.ttf</fontPath>
      <fontSize>0.23</fontSize>
      <color>FFFFFFFF</color>
      <alignment>left</alignment>
      <verticalAlignment>center</verticalAlignment>
    </text>
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
  <!-- Pantalla de apartados: foto, barra de tarjetas abajo y nada más encima. -->
  <view name="system">
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
    <carousel name="systemcarousel">
      <type>horizontal</type>
      <pos>0 0.775</pos>
      <size>1 0.21</size>
      <color>00000000</color>
      <logoAlignment>center</logoAlignment>
      <!-- 0.17 × 1.15 = 0.1955 < 1/5: la elegida crece sin montarse sobre las vecinas. -->
      <logoSize>0.17 0.13</logoSize>
      <logoScale>1.15</logoScale>
      <maxLogoCount>5</maxLogoCount>
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
  <!-- Listas de juegos (vista «grid», puesta por apartado con <apartado>.defaultView): columna de
       tarjetas a la izquierda (pantalla de título y el nombre en pequeño; la elegida en el centro,
       algo más grande y con borde) y, a la derecha y centrados, el nombre, «consola · año · género»
       y de qué va. Empieza por debajo de la hora, que es la misma en todas las pantallas. -->
  <view name="grid">
    <image name="panther_fondo_lista" extra="true">
      <origin>0 0</origin>
      <pos>0 0</pos>
      <size>1 1</size>
      <path>./fondo-lista.jpg</path>
      <zIndex>1</zIndex>
    </image>
    <imagegrid name="gamegrid">
      <pos>0.03 0.33</pos>
      <size>0.33 0.66</size>
      <margin>0 0.03</margin>
      <autoLayout>1 2</autoLayout>
      <autoLayoutSelectedZoom>1.06</autoLayoutSelectedZoom>
      <imageSource>image</imageSource>
      <scrollDirection>verticalCenter</scrollDirection>
      <animateSelection>true</animateSelection>
      <zIndex>30</zIndex>
    </imagegrid>
    <gridtile name="default">
      <padding>6 6</padding>
      <imageSizeMode>minSize</imageSizeMode>
      <backgroundColor>00000000</backgroundColor>
      <selectionMode>image</selectionMode>
    </gridtile>
    <gridtile name="selected">
      <padding>6 6</padding>
      <imageSizeMode>minSize</imageSizeMode>
      <backgroundColor>00000000</backgroundColor>
      <selectionMode>image</selectionMode>
    </gridtile>
    <!-- Esquinas redondas y, en la elegida, el borde azul por encima de la imagen. -->
    <image name="gridtile.image">
      <roundCorners>0.07</roundCorners>
    </image>
    <image name="gridtile.image:selected">
      <roundCorners>0.07</roundCorners>
    </image>
    <image name="gridtile.overlay:selected">
      <path>./borde-elegida.png</path>
    </image>
    <text name="gridtile">
      <size>1 0.16</size>
      <fontSize>0.03</fontSize>
      <color>D2D2DCFF</color>
      <alignment>center</alignment>
    </text>
    <text name="gridtile:selected">
      <color>FFFFFFFF</color>
    </text>
    <text name="md_name">
      <pos>0.40 0.30</pos>
      <size>0.57 0.09</size>
      <fontSize>0.055</fontSize>
      <color>FFFFFFFF</color>
      <alignment>center</alignment>
      <zIndex>40</zIndex>
    </text>
    <text name="md_genre">
      <pos>0.40 0.395</pos>
      <size>0.57 0.05</size>
      <fontSize>0.032</fontSize>
      <color>96C8FFFF</color>
      <alignment>center</alignment>
      <zIndex>40</zIndex>
    </text>
    <text name="md_description">
      <pos>0.41 0.47</pos>
      <size>0.55 0.40</size>
      <fontSize>0.036</fontSize>
      <color>E6E6EEFF</color>
      <alignment>center</alignment>
      <lineSpacing>1.25</lineSpacing>
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
    <helpsystem name="help">
      <pos>0.40 0.93</pos>
      <textColor>DDDDDDFF</textColor>
      <iconColor>DDDDDDFF</iconColor>
      <fontSize>0.028</fontSize>
    </helpsystem>
  </view>
</theme>
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
        xml.write_text(LOGO_NUEVO.format(tarjeta=ruta), encoding="utf-8")
    print("tarjeta:", sistema)
print("tema listo:", DESTINO)
