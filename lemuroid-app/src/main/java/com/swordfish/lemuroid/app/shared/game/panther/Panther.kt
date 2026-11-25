package com.swordfish.lemuroid.app.shared.game.panther

import android.content.Context
import android.os.Build
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.lib.library.CoreID
import com.swordfish.lemuroid.lib.library.SystemID
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.preferences.SharedPreferencesHelper
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import java.io.File

/**
 * El emulador Panther (el de la consola, con mGBA dentro) en Pokemulador: abre los juegos de GB, GBC y
 * GBA en vez de los núcleos de libretro. Las partidas (.srm) son las mismas.
 */
object Panther {
    /** Donde guarda «seguir donde lo dejaste» y la partida rápida, dentro de los estados. */
    const val CARPETA_ESTADOS = "panther"
    private const val TOLERANCIA_MS = 30_000L

    private val EXTENSIONES =
        mapOf(SystemID.GB.dbname to "gb", SystemID.GBC.dbname to "gbc", SystemID.GBA.dbname to "gba")

    /** Si este juego se puede abrir con Panther en este móvil. */
    fun puede(
        context: Context,
        game: Game,
    ): Boolean =
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
            game.systemId in EXTENSIONES &&
            File(context.applicationInfo.nativeLibraryDir, "libpanther.so").exists()

    /** Si este juego se abre con Panther (Ajustes → Emulador Panther). */
    fun vale(
        context: Context,
        game: Game,
    ): Boolean = activado(context) && puede(context, game)

    fun activado(context: Context): Boolean =
        SharedPreferencesHelper.getSharedPreferences(context)
            .getBoolean(context.getString(R.string.pref_key_panther), true)

    fun extension(game: Game): String = EXTENSIONES[game.systemId] ?: "gba"

    /** El nombre de la partida, como en el resto de Pokemulador. */
    fun base(game: Game): String = game.fileName.substringBeforeLast(".")

    fun carpetaEstados(
        context: Context,
        perfil: String?,
    ): File = File(DirectoriesManager(context).getStatesDirectoryFor(perfil), CARPETA_ESTADOS)

    fun seguirDonde(
        context: Context,
        game: Game,
        perfil: String?,
    ): File = File(carpetaEstados(context, perfil), "${base(game)}.estado-auto")

    /**
     * Lo dejaste a medias con el emulador de antes y Panther no puede seguir desde ahí: pasa en GB y GBC
     * (Gambatte guarda de otra forma; en GBA los dos son mGBA y Panther sí sigue).
     */
    fun aMediasConElDeAntes(
        context: Context,
        game: Game,
        perfil: String?,
        nucleo: CoreID,
    ): Boolean {
        if (nucleo == CoreID.MGBA) return false
        if (seguirDonde(context, game, perfil).exists()) return false
        val directorios = DirectoriesManager(context)
        val estado = File(File(directorios.getStatesDirectoryFor(perfil), nucleo.coreName), "${game.fileName}.state")
        if (estado.length() == 0L) return false
        val partida = File(directorios.getSavesDirectoryFor(perfil), "${base(game)}.srm")
        return partida.lastModified() <= estado.lastModified() + TOLERANCIA_MS
    }
}
