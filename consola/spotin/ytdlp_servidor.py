"""yt-dlp siempre abierto para Spotin: arrancar Python y cargar yt-dlp tarda ~16 s en la consola, y
antes se hacía con cada canción. Así se carga una vez (al abrir Spotin) y luego cada canción solo
tarda lo que tarda YouTube en contestar (~3 s).

Se le pasa la ruta del yt-dlp (el zipapp de siempre, que también vale para importarlo). Lee números de
vídeo, uno por línea, y contesta una línea de JSON por cada uno: la dirección del sonido, su tipo,
su tamaño y las cabeceras que hay que mandar (lo mismo que sacaba «yt-dlp -J»), o «error».
"""
import json
import sys

sys.path.insert(0, sys.argv[1])
import yt_dlp  # noqa: E402

FORMATO = "bestaudio[ext=webm]/bestaudio[ext=m4a]/bestaudio"
ydl = yt_dlp.YoutubeDL({"quiet": True, "no_warnings": True, "noplaylist": True, "skip_download": True,
                        "noprogress": True, "format": FORMATO})


def contestar(d):
    sys.stdout.write(json.dumps(d) + "\n")
    sys.stdout.flush()


contestar({"listo": True})
for linea in sys.stdin:
    video = linea.strip()
    if not video:
        continue
    try:
        info = ydl.extract_info("https://music.youtube.com/watch?v=" + video, download=False)
        f = info
        if not f.get("url"):  # por si el formato elegido no viene arriba
            f = (info.get("requested_downloads") or info.get("requested_formats") or [{}])[0]
        r = {k: f.get(k) for k in ("url", "ext", "acodec", "filesize", "filesize_approx", "http_headers")}
    except Exception as e:  # noqa: BLE001 - se cuenta a Spotin y sigue con la siguiente
        lineas = str(e).strip().splitlines()
        r = {"error": lineas[-1] if lineas else repr(e)}
    r["video"] = video
    contestar(r)
