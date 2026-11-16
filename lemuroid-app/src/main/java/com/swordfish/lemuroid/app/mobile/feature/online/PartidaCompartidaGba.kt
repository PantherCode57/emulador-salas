package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.lib.library.SystemID
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.preferences.SharedPreferencesHelper
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import timber.log.Timber
import java.io.File

/**
 * Partida compartida de GBA: veros andando por el mapa en Rojo Fuego, Verde Hoja, Rubí, Zafiro y
 * Esmeralda (con GBA-PK Multiplayer, de TheHunterManX; ver terceros/gba-pk).
 *
 * Al abrir un juego de GBA se escribe <sistema>/pokemulador/multijugador.lua, que el núcleo de mGBA
 * de Pokemulador carga solo. Lleva:
 *  - desde una sala, GBA-PK: con la sala abierta por nosotros, el script abre la partida; si no,
 *    entra en la del amigo por 127.0.0.1:4096, que el enlace lleva hasta él;
 *  - con el truco activado en los ajustes, los Caramelos Raros infinitos de Rojo Fuego y Verde Hoja
 *    (consola/trucos/caramelos-frlg.lua, el mismo de la consola; él mismo mira qué juego es).
 * Si no hay nada de eso, se borra el archivo y el juego va como siempre.
 */
object PartidaCompartidaGba {
    private const val SCRIPT = "GBA-PK_Multiplayer.lua"
    private const val FINAL = "pokemulador_final.lua"
    private const val CARAMELOS = "caramelos-frlg.lua"

    private fun archivo(context: Context) =
        File(File(DirectoriesManager(context).getSystemDirectory(), "pokemulador"), "multijugador.lua")

    fun esDeGba(game: Game) = game.systemId == SystemID.GBA.dbname

    /** El truco de los Caramelos Raros (Ajustes → Trucos). */
    fun caramelosActivados(context: Context): Boolean =
        SharedPreferencesHelper.getSharedPreferences(context)
            .getBoolean(context.getString(R.string.pref_key_caramelos_frlg), false)

    /** Se llama antes de abrir cualquier juego (proceso principal). */
    fun preparar(
        context: Context,
        game: Game,
        sala: String?,
    ) {
        val destino = archivo(context)
        val estado = SalaOnline.estado.value
        val enSala = sala != null && esDeGba(game) && estado.codigo == sala && estado.rolDecidido
        val caramelos = esDeGba(game) && caramelosActivados(context)
        if (!enSala && !caramelos) {
            destino.delete()
            return
        }
        try {
            val texto = StringBuilder()
            if (enSala) {
                val base = context.assets.open(SCRIPT).bufferedReader().use { it.readText() }
                val final =
                    context.assets.open(FINAL).bufferedReader().use { it.readText() }
                        .replace("__MODO__", if (estado.anfitrion) "h" else "c")
                val nombre = nombreParaJuego(SalaOnline.nombreVisible(context))
                texto.append(base.replaceFirst("local Nickname = \"\"", "local Nickname = \"$nombre\""))
                texto.append("\n").append(final)
            }
            if (caramelos) {
                texto.append("\n").append(context.assets.open(CARAMELOS).bufferedReader().use { it.readText() })
            }
            destino.parentFile?.mkdirs()
            val temporal = File(destino.parentFile, destino.name + ".tmp")
            temporal.writeText(texto.toString())
            if (!temporal.renameTo(destino)) {
                destino.writeText(texto.toString())
                temporal.delete()
            }
        } catch (e: Exception) {
            Timber.e(e, "No se pudo preparar el script de GBA")
            destino.delete()
        }
    }

    /** GBA-PK admite hasta 10 caracteres; nos quedamos con letras, números y espacios. */
    private fun nombreParaJuego(nombre: String): String =
        nombre.filter { it.isLetterOrDigit() || it == ' ' }.trim().take(10).ifBlank { "Jugador" }
}
