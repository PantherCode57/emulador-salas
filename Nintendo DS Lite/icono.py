# El icono de Panther OS en el menú de la flashcard: 32×32, 16 colores, el 0 es transparente.
# Una P sencilla hecha con rectángulos y la línea violeta de la animación de arranque.
import sys
from PIL import Image, ImageDraw

pal = [
    (255, 0, 255),   # 0 transparente
    (11, 18, 32),    # 1 fondo
    (24, 32, 50),    # 2 fondo claro
    (255, 255, 255), # 3 blanco
    (196, 170, 255), # 4 violeta
    (140, 115, 210), # 5 violeta medio
    (80, 66, 130),   # 6 violeta oscuro
    (36, 46, 70),    # 7 borde
]
img = Image.new("P", (32, 32), 0)
flat = []
for c in pal + [(0, 0, 0)] * (16 - len(pal)):
    flat += list(c)
img.putpalette(flat + [0] * (768 - len(flat)))
d = ImageDraw.Draw(img)
d.rounded_rectangle([0, 0, 31, 31], radius=7, fill=7)
d.rounded_rectangle([1, 1, 30, 30], radius=6, fill=1)
d.rectangle([2, 2, 29, 12], fill=2)
d.rounded_rectangle([1, 1, 30, 12], radius=6, fill=2)
d.rectangle([1, 9, 30, 12], fill=2)
# La P
d.rectangle([10, 6, 13, 23], fill=3)
d.rectangle([10, 6, 19, 8], fill=3)
d.rectangle([10, 14, 19, 16], fill=3)
d.rectangle([19, 7, 21, 15], fill=3)
d.point([(19, 6)], fill=2)
d.point([(19, 16)], fill=1)
d.point([(20, 7), (20, 15)], fill=3)
# La línea de abajo, más fuerte en el medio
for x in range(5, 27):
    k = abs(x - 15.5) / 11
    d.point([(x, 26), (x, 27)], fill=4 if k < 0.4 else 5 if k < 0.75 else 6)
img.save(sys.argv[1], transparency=0)
print("icono:", sys.argv[1])
