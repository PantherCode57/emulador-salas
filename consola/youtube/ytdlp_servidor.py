"""yt-dlp siempre abierto para el YouTube de la consola: arrancar Python y cargar yt-dlp tarda ~16 s
en la consola, y antes se hacía con cada vídeo. Así se carga una vez (al abrir YouTube) y luego cada
vídeo solo tarda lo que tarda YouTube en contestar.

Se le pasa la ruta del yt-dlp (el zipapp de siempre, que también vale para importarlo). Lee una
petición por línea, en JSON: {"args": [opciones como en la línea de órdenes], "url": "..."}, y
contesta una línea con lo mismo que daba «yt-dlp -J» con esas opciones, o {"error": "..."}.
"""
import json
import sys

sys.path.insert(0, sys.argv[1])
import yt_dlp  # noqa: E402

descargadores = {}  # uno por cada juego de opciones (crearlo es rápido, pero así se reutiliza)


def descargador(args):
    clave = json.dumps(args)
    if clave not in descargadores:
        opciones = yt_dlp.parse_options(args).ydl_opts
        opciones.update({"quiet": True, "no_warnings": True, "noprogress": True, "simulate": True,
                         "skip_download": True})
        descargadores[clave] = yt_dlp.YoutubeDL(opciones)
    return descargadores[clave]


def contestar(texto):
    sys.stdout.write(texto + "\n")
    sys.stdout.flush()


contestar(json.dumps({"listo": True}))
for linea in sys.stdin:
    if not linea.strip():
        continue
    try:
        pet = json.loads(linea)
        ydl = descargador(pet.get("args") or [])
        info = ydl.extract_info(pet["url"], download=False)
        contestar(json.dumps(ydl.sanitize_info(info)))
    except BaseException as e:  # noqa: BLE001 - también SystemExit, si una opción no vale
        if isinstance(e, KeyboardInterrupt):
            raise
        lineas = str(e).strip().splitlines()
        contestar(json.dumps({"error": lineas[-1] if lineas else repr(e)}))
