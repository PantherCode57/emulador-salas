// Salas online de Pokemulador dentro del núcleo libretro de Azahar.
#pragma once

namespace PokemuladorSala {

/// Lee las variables citra_pkm_room_* y, si hay sala, la crea (anfitrión) y/o entra en ella.
void Iniciar();

/// Sale de la sala y la cierra si la habíamos creado nosotros.
void Detener();

} // namespace PokemuladorSala
