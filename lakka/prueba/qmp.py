"""Manda órdenes a QEMU por QMP para probar el USB sin pantalla.

    qmp.py <socket> captura <archivo.png>
    qmp.py <socket> teclas x down down ret ...   (nombres "qcode" de QEMU)
    qmp.py <socket> apagar
"""
import json
import socket
import sys
import time


def hablar(ruta, orden):
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.connect(ruta)
    f = s.makefile("rw")
    f.readline()  # saludo
    for o in ({"execute": "qmp_capabilities"}, orden):
        f.write(json.dumps(o) + "\n")
        f.flush()
        while True:
            r = json.loads(f.readline())
            if "return" in r or "error" in r:
                if "error" in r:
                    print("QMP:", r["error"], file=sys.stderr)
                break
    s.close()


ruta, que = sys.argv[1], sys.argv[2]
if que == "captura":
    hablar(ruta, {"execute": "screendump", "arguments": {"filename": sys.argv[3], "format": "png"}})
elif que == "teclas":
    for t in sys.argv[3:]:
        hablar(ruta, {"execute": "send-key", "arguments": {"keys": [{"type": "qcode", "data": t}], "hold-time": 150}})
        time.sleep(0.6)
elif que == "apagar":
    hablar(ruta, {"execute": "system_powerdown"})
