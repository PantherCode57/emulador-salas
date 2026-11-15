"""Lakka: las salas de Azahar leen también variables de entorno.

En el móvil, Pokemulador pasa citra_pkm_room_* como variables del núcleo. RetroArch (Lakka) solo
pasa las opciones que el núcleo declara, así que el servicio de Pokemulador las deja en el entorno
del proceso (POKEMULADOR_SALA_MODO, _PUERTO, _CLAVE y _NOMBRE). Se aplica después de
nucleos/azahar/aplicar.py, solo para el núcleo del USB.
"""
import pathlib
import sys

ruta = pathlib.Path(sys.argv[1]) / "src" / "citra_libretro" / "pokemulador_sala.cpp"
texto = ruta.read_text(encoding="utf-8")

ayuda = '''#include <cstdlib>

namespace {
// Lakka: si el núcleo no recibe la variable, la busca en el entorno del proceso.
std::string PkmVariable(const char* clave, const char* entorno, const char* defecto) {
    const std::string valor = LibRetro::FetchVariable(clave, "");
    if (!valor.empty() && valor != "off") {
        return valor;
    }
    const char* e = std::getenv(entorno);
    return (e != nullptr && *e != 0) ? std::string(e) : std::string(defecto);
}
} // namespace

namespace PokemuladorSala {
'''
assert texto.count("namespace PokemuladorSala {\n") == 1
texto = texto.replace("namespace PokemuladorSala {\n", ayuda, 1)

for clave, entorno, defecto in [
    ("citra_pkm_room_mode", "POKEMULADOR_SALA_MODO", "off"),
    ("citra_pkm_room_password", "POKEMULADOR_SALA_CLAVE", ""),
    ("citra_pkm_room_nickname", "POKEMULADOR_SALA_NOMBRE", "Jugador"),
    ("citra_pkm_room_port", "POKEMULADOR_SALA_PUERTO", "24872"),
]:
    viejo = f'LibRetro::FetchVariable("{clave}", "{defecto}")'
    assert texto.count(viejo) == 1, viejo
    texto = texto.replace(viejo, f'PkmVariable("{clave}", "{entorno}", "{defecto}")')

ruta.write_text(texto, encoding="utf-8")
print("Salas de Azahar: también por variables de entorno")
