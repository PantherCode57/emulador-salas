package com.swordfish.lemuroid.lib.saves

import com.swordfish.lemuroid.common.kotlin.readBytesAtomicOrNull
import com.swordfish.lemuroid.common.kotlin.runCatchingWithRetry
import com.swordfish.lemuroid.common.kotlin.writeBytesAtomic
import com.swordfish.lemuroid.lib.library.SystemCoreConfig
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.saves.migrators.getSavesMigrator
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import timber.log.Timber
import java.io.File

class SavesManager(private val directoriesManager: DirectoriesManager) {
    /**
     * Pokemulador: si hay partida pero no se puede leer, falla (el juego no se abre). Antes daba null:
     * el juego empezaba de cero y, al salir, la partida vacía pisaba la buena.
     */
    suspend fun getSaveRAM(
        game: Game,
        systemCoreConfig: SystemCoreConfig,
    ): ByteArray? {
        return withContext(Dispatchers.IO) {
            runCatchingWithRetry(FILE_ACCESS_RETRIES) {
                val datos =
                    getSaveFile(getSaveRAMFileName(game)).readBytesAtomicOrNull()
                        ?: systemCoreConfig.getSavesMigrator()?.loadPreviousSaveForGame(game, directoriesManager)
                datos?.let { FormatoPartida.paraNucleo(game.systemId, it) }
            }.getOrThrow()
        }
    }

    suspend fun setSaveRAM(
        game: Game,
        data: ByteArray,
    ) {
        withContext(Dispatchers.IO) {
            val result =
                runCatchingWithRetry(FILE_ACCESS_RETRIES) {
                    if (data.isEmpty()) {
                        return@runCatchingWithRetry
                    }

                    val saveFile = getSaveFile(getSaveRAMFileName(game))
                    // Pokemulador: una partida vacía nunca pisa a una que tiene algo.
                    if (saveFile.length() > 0 && CopiasPartida.estaVacia(data) && !CopiasPartida.estaVacia(saveFile.readBytes())) {
                        Timber.e("No se guarda la partida vacía de ${game.fileName}: la que hay tiene datos")
                        return@runCatchingWithRetry
                    }
                    runCatching { CopiasPartida.hacer(saveFile) }
                        .onFailure { Timber.e(it, "No se pudo hacer la copia de la partida") }
                    saveFile.writeBytesAtomic(data)
                }
            result.onFailure { Timber.e(it, "No se pudo guardar la partida de ${game.fileName}") }
        }
    }

    suspend fun getSaveRAMInfo(game: Game): SaveInfo {
        return withContext(Dispatchers.IO) {
            val saveFile = getSaveFile(getSaveRAMFileName(game))
            val fileExists = saveFile.exists() && saveFile.length() > 0
            SaveInfo(fileExists, saveFile.lastModified())
        }
    }

    private suspend fun getSaveFile(fileName: String): File {
        return withContext(Dispatchers.IO) {
            val savesDirectory = directoriesManager.getSavesDirectory()
            File(savesDirectory, fileName)
        }
    }

    /** This name should make it compatible with RetroArch so that users can freely sync saves across the two application. */
    private fun getSaveRAMFileName(game: Game) = "${game.fileName.substringBeforeLast(".")}.srm"

    companion object {
        private const val FILE_ACCESS_RETRIES = 3
    }
}
