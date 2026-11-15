package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import android.net.Uri
import com.swordfish.lemuroid.lib.library.GameSystem
import com.swordfish.lemuroid.lib.library.SystemID
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Cargar y sacar partidas como archivo .sav (el formato de linkcable.app, mGBA, VisualBoyAdvance,
 * melonDS...). Pokemulador guarda la partida de cada juego como <juego>.srm, que es lo mismo.
 */
object PartidasArchivo {
    private const val MAXIMO = 64 * 1024 * 1024
    private val PIE_DESMUME =
        "|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:"
            .toByteArray(Charsets.US_ASCII)

    /** En 3DS la partida va dentro de la tarjeta SD del emulador, no es un archivo suelto. */
    fun admiteArchivo(game: Game): Boolean =
        runCatching { GameSystem.findById(game.systemId).id != SystemID.NINTENDO_3DS }.getOrDefault(false)

    fun nombreSugerido(game: Game): String = "${base(game)}.sav"

    /** Pone el .sav como partida del juego. Devuelve el mensaje para el usuario. */
    suspend fun importar(
        context: Context,
        game: Game,
        uri: Uri,
        perfil: String?,
    ): String =
        withContext(Dispatchers.IO) {
            try {
                val leidos =
                    context.contentResolver.openInputStream(uri)?.use { entrada ->
                        val bytes = entrada.readBytes()
                        if (bytes.size > MAXIMO) null else bytes
                    } ?: return@withContext "Ese archivo no es una partida (es demasiado grande)."
                if (leidos.isEmpty()) {
                    return@withContext "El archivo está vacío (0 bytes). Vuelve a descargarlo de la web."
                }
                val datos = ajustarTamano(game, quitarPieDesmume(leidos))

                val directorios = DirectoriesManager(context)
                val saves = directorios.getSavesDirectoryFor(perfil)
                val destino = File(saves, "${base(game)}.srm")
                val copias = File(saves, "copias")
                val marca = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.ROOT).format(Date())
                var hayCopia = false
                if (destino.exists() && destino.length() > 0) {
                    copias.mkdirs()
                    destino.copyTo(File(copias, "${base(game)}-$marca.srm"), overwrite = true)
                    hayCopia = true
                }
                // Si había una partida de DeSmuME (.dsv) más nueva, ganaría a la importada.
                File(saves, "${base(game)}.dsv").takeIf { it.exists() }?.let {
                    copias.mkdirs()
                    it.renameTo(File(copias, "${base(game)}-$marca.dsv"))
                }

                val temporal = File(saves, "${base(game)}.srm.tmp")
                temporal.writeBytes(datos)
                if (!temporal.renameTo(destino)) {
                    destino.writeBytes(datos)
                    temporal.delete()
                }

                // El juego se reanuda desde el guardado automático; lo apartamos para que empiece
                // desde la partida que acabamos de poner.
                directorios.getStatesDirectoryFor(perfil).listFiles()?.forEach { carpetaNucleo ->
                    listOf("${game.fileName}.state", "${game.fileName}.state.metadata").forEach { nombre ->
                        val estado = File(carpetaNucleo, nombre)
                        if (estado.exists()) {
                            copias.mkdirs()
                            estado.renameTo(File(copias, "${carpetaNucleo.name}-$marca-$nombre"))
                        }
                    }
                }

                if (hayCopia) {
                    "Partida cargada. La que tenías antes está guardada como copia por si acaso."
                } else {
                    "Partida cargada. Ya puedes jugar con ella."
                }
            } catch (e: Exception) {
                "No se pudo cargar la partida: ${e.message}"
            }
        }

    /** Guarda la partida del juego en un .sav (para llevarla a otro emulador o a la web). */
    suspend fun exportar(
        context: Context,
        game: Game,
        uri: Uri,
        perfil: String?,
    ): String =
        withContext(Dispatchers.IO) {
            try {
                val origen = File(DirectoriesManager(context).getSavesDirectoryFor(perfil), "${base(game)}.srm")
                if (!origen.exists() || origen.length() == 0L) {
                    return@withContext "Todavía no hay ninguna partida guardada de este juego."
                }
                context.contentResolver.openOutputStream(uri, "wt")?.use { it.write(origen.readBytes()) }
                    ?: return@withContext "No se pudo escribir el archivo."
                "Partida guardada en el archivo."
            } catch (e: Exception) {
                "No se pudo guardar la partida: ${e.message}"
            }
        }

    private fun base(game: Game) = game.fileName.substringBeforeLast(".")

    private fun quitarPieDesmume(datos: ByteArray): ByteArray {
        if (datos.size < PIE_DESMUME.size) return datos
        fuera@ for (i in 0..datos.size - PIE_DESMUME.size) {
            for (j in PIE_DESMUME.indices) {
                if (datos[i + j] != PIE_DESMUME[j]) continue@fuera
            }
            return datos.copyOf(i)
        }
        return datos
    }

    /**
     * Las partidas son siempre de un tamaño potencia de dos (32 KB, 128 KB...). Algunos emuladores
     * añaden detrás el reloj del cartucho (16 bytes en GBA, 44-48 en Game Boy). Si sobra algo lo
     * quitamos, porque el emulador no carga partidas más grandes de lo que espera. En Game Boy el
     * núcleo de Pokemulador guarda el reloj en 8 bytes extra: esas se dejan.
     */
    private fun ajustarTamano(
        game: Game,
        datos: ByteArray,
    ): ByteArray {
        if (datos.size < 512) return datos
        val potencia = Integer.highestOneBit(datos.size)
        val sobra = datos.size - potencia
        if (sobra == 0) return datos
        val esGameBoy =
            runCatching { GameSystem.findById(game.systemId).id in setOf(SystemID.GB, SystemID.GBC) }
                .getOrDefault(false)
        if (esGameBoy && sobra == 8) return datos
        return datos.copyOf(potencia)
    }
}
