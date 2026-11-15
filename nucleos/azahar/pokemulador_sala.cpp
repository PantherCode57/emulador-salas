// Salas online de Pokemulador dentro del núcleo libretro de Azahar.
//
// Pokemulador (la app) pasa estas variables al núcleo antes de cargar el juego:
//   citra_pkm_room_mode      off | host | join
//   citra_pkm_room_port      puerto local de la sala (24872)
//   citra_pkm_room_password  código de la sala
//   citra_pkm_room_nickname  nombre del jugador en la sala (tiene que ser único)
//
// "host": el núcleo abre la sala en 127.0.0.1 y entra en ella. El enlace de Tailscale de la
// app la publica para los amigos. "join": el núcleo entra en 127.0.0.1:puerto, donde el enlace
// de la app lleva el tráfico hasta la sala del amigo. Es la misma lógica que usa la app
// oficial de Azahar para Android (src/android/app/src/main/jni/multiplayer.cpp).
//
// Las salas se pueden guardar y cada uno entra cuando quiere, así que el anfitrión puede llegar
// después que su amigo, o irse y volver. Por eso un hilo vigila cada 5 segundos y, si no estamos
// dentro de la sala, vuelve a intentar entrar.

#include <chrono>
#include <condition_variable>
#include <memory>
#include <mutex>
#include <string>
#include <thread>

#include "citra_libretro/environment.h"
#include "citra_libretro/pokemulador_sala.h"
#include "common/logging/log.h"
#include "core/core.h"
#include "core/hle/service/cfg/cfg.h"
#include "network/network.h"
#include "network/room.h"
#include "network/room_member.h"
#include "network/verify_user.h"

namespace PokemuladorSala {

namespace {
bool red_iniciada = false;
bool sala_propia = false;

struct DatosSala {
    std::string nombre;
    std::string clave;
    u16 puerto = Network::DefaultRoomPort;
    std::string consola;
};
DatosSala datos;

std::thread vigilante;
std::mutex mutex_vigilante;
std::condition_variable aviso_vigilante;
bool parar_vigilante = false;

void Entrar(Network::RoomMember& member) {
    member.Join(datos.nombre, datos.consola, "127.0.0.1", datos.puerto, 0, Network::NoPreferredMac,
                datos.clave);
}

void Vigilar() {
    std::unique_lock lock(mutex_vigilante);
    while (!aviso_vigilante.wait_for(lock, std::chrono::seconds(5),
                                     [] { return parar_vigilante; })) {
        auto member = Network::GetRoomMember().lock();
        if (member && member->GetState() == Network::RoomMember::State::Idle) {
            LOG_INFO(Frontend, "Pokemulador: volviendo a entrar en la sala");
            Entrar(*member);
        }
    }
}

void PararVigilante() {
    if (!vigilante.joinable()) {
        return;
    }
    {
        std::lock_guard lock(mutex_vigilante);
        parar_vigilante = true;
    }
    aviso_vigilante.notify_all();
    vigilante.join();
}
} // namespace

void Iniciar() {
    PararVigilante();
    const std::string modo = LibRetro::FetchVariable("citra_pkm_room_mode", "off");
    if (modo != "host" && modo != "join") {
        return;
    }
    const std::string clave = LibRetro::FetchVariable("citra_pkm_room_password", "");
    const std::string nombre = LibRetro::FetchVariable("citra_pkm_room_nickname", "Jugador");
    u16 puerto = Network::DefaultRoomPort;
    try {
        puerto = static_cast<u16>(
            std::stoi(LibRetro::FetchVariable("citra_pkm_room_port", "24872")));
    } catch (...) {
    }

    if (!red_iniciada) {
        red_iniciada = Network::Init();
    }
    if (!red_iniciada) {
        LibRetro::DisplayMessage("Pokemulador: no se pudo iniciar la red de la sala.");
        return;
    }

    auto member = Network::GetRoomMember().lock();
    if (!member) {
        return;
    }
    if (member->IsConnected()) {
        member->Leave();
    }

    if (modo == "host") {
        auto room = Network::GetRoom().lock();
        if (room && room->GetState() != Network::Room::State::Open) {
            sala_propia = room->Create("Pokemulador", "", "127.0.0.1", puerto, clave, 4, nombre,
                                       "Pokemon", 0,
                                       std::make_unique<Network::VerifyUser::NullBackend>(), {});
            // Igual que la app oficial: un momento para que la sala esté lista antes de entrar.
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }
        if (!sala_propia) {
            LibRetro::DisplayMessage("Pokemulador: no se pudo abrir la sala.");
            return;
        }
    }

    datos.nombre = nombre;
    datos.clave = clave;
    datos.puerto = puerto;
    datos.consola = Service::CFG::GetConsoleIdHash(Core::System::GetInstance());
    Entrar(*member);
    parar_vigilante = false;
    vigilante = std::thread(Vigilar);
    LOG_INFO(Frontend, "Pokemulador: sala {} en 127.0.0.1:{}", modo, puerto);
    LibRetro::DisplayMessage(modo == "host" ? "Sala online abierta" : "Entrando en la sala online");
}

void Detener() {
    PararVigilante();
    if (!red_iniciada) {
        return;
    }
    if (auto member = Network::GetRoomMember().lock()) {
        if (member->IsConnected()) {
            member->Leave();
        }
    }
    if (sala_propia) {
        if (auto room = Network::GetRoom().lock()) {
            room->Destroy();
        }
        sala_propia = false;
    }
}

} // namespace PokemuladorSala
