package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import com.swordfish.lemuroid.lib.library.SystemID
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import timber.log.Timber
import java.io.File

/**
 * Partida compartida de GBA: veros andando por el mapa en Rojo Fuego, Verde Hoja, Rubí, Zafiro y
 * Esmeralda (con GBA-PK Multiplayer, de TheHunterManX; ver terceros/gba-pk).
 *
 * Al abrir un juego de GBA desde una sala se escribe <sistema>/pokemulador/multijugador.lua, que el
 * núcleo de mGBA de Pokemulador carga solo. Con la sala abierta por nosotros, el script abre la
 * partida; si no, entra en la del amigo por 127.0.0.1:4096, que el enlace lleva hasta él. En
 * cualquier otro caso se borra el archivo y el juego va como siempre.
 */
object PartidaCompartidaGba {
    private const val SCRIPT = "GBA-PK_Multiplayer.lua"
    private const val FINAL = "pokemulador_final.lua"

    private fun archivo(context: Context) =
        File(File(DirectoriesManager(context).getSystemDirectory(), "pokemulador"), "multijugador.lua")

    fun esDeGba(game: Game) = game.systemId == SystemID.GBA.dbname

    /** Se llama antes de abrir cualquier juego (proceso principal). */
    fun preparar(
        context: Context,
        game: Game,
        sala: String?,
    ) {
        val destino = archivo(context)
        val estado = SalaOnline.estado.value
        val activa = sala != null && esDeGba(game) && estado.codigo == sala && estado.rolDecidido
        if (!activa) {
            destino.delete()
            return
        }
        try {
            val base = context.assets.open(SCRIPT).bufferedReader().use { it.readText() }
            val final =
                context.assets.open(FINAL).bufferedReader().use { it.readText() }
                    .replace("__MODO__", if (estado.anfitrion) "h" else "c")
            val nombre = nombreParaJuego(SalaOnline.nombreVisible(context))
            val texto = base.replaceFirst("local Nickname = \"\"", "local Nickname = \"$nombre\"") + "\n" + final
            destino.parentFile?.mkdirs()
            val temporal = File(destino.parentFile, destino.name + ".tmp")
            temporal.writeText(texto)
            if (!temporal.renameTo(destino)) {
                destino.writeText(texto)
                temporal.delete()
            }
        } catch (e: Exception) {
            Timber.e(e, "No se pudo preparar la partida compartida")
            destino.delete()
        }
    }

    /** GBA-PK admite hasta 10 caracteres; nos quedamos con letras, números y espacios. */
    private fun nombreParaJuego(nombre: String): String =
        nombre.filter { it.isLetterOrDigit() || it == ' ' }.trim().take(10).ifBlank { "Jugador" }
}
