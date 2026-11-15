package com.swordfish.lemuroid.lib.storage

import android.content.Context
import java.io.File

class DirectoriesManager(private val appContext: Context) {
    @Deprecated("Use the external states directory")
    fun getInternalStatesDirectory(): File =
        File(appContext.filesDir, "states").apply {
            mkdirs()
        }

    fun getCoresDirectory(): File =
        File(appContext.filesDir, "cores").apply {
            mkdirs()
        }

    fun getSystemDirectory(): File =
        File(appContext.filesDir, "system").apply {
            mkdirs()
        }

    fun getStatesDirectory(): File = getStatesDirectoryFor(perfil)

    fun getStatesPreviewDirectory(): File = getStatesPreviewDirectoryFor(perfil)

    fun getSavesDirectory(): File = getSavesDirectoryFor(perfil)

    /** Carpeta de partidas guardadas de una partida concreta (null = la de siempre). */
    fun getSavesDirectoryFor(perfil: String?): File =
        File(getRaizPartidas(perfil), "saves").apply {
            mkdirs()
        }

    fun getStatesDirectoryFor(perfil: String?): File =
        File(getRaizPartidas(perfil), "states").apply {
            mkdirs()
        }

    fun getStatesPreviewDirectoryFor(perfil: String?): File =
        File(getRaizPartidas(perfil), "state-previews").apply {
            mkdirs()
        }

    /** Carpeta con todo lo de una partida aparte (la de una sala). */
    fun getPerfilDirectory(perfil: String): File = File(File(appContext.getExternalFilesDir(null), PERFILES), perfil)

    fun getInternalRomsDirectory(): File =
        File(appContext.getExternalFilesDir(null), "roms").apply {
            mkdirs()
        }

    private fun getRaizPartidas(perfil: String?): File? =
        if (perfil == null) appContext.getExternalFilesDir(null) else getPerfilDirectory(perfil)

    companion object {
        private const val PERFILES = "perfiles"

        /**
         * Partida que usa el juego que se está jugando (Pokemulador: cada sala puede tener la suya).
         * null es la partida de siempre. Lo fija el proceso del juego al abrirlo; como en ese
         * proceso solo hay un juego a la vez, vale con una variable para todo el proceso.
         */
        @Volatile
        var perfil: String? = null
    }
}
