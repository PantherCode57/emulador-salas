package com.swordfish.lemuroid.app.shared.game

import android.app.Activity
import android.app.AlertDialog
import android.content.Context
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.feature.online.PartidaCompartidaGba
import com.swordfish.lemuroid.app.shared.game.panther.JuegoPanther
import com.swordfish.lemuroid.app.shared.game.panther.Panther
import com.swordfish.lemuroid.app.shared.main.GameLaunchTaskHandler
import com.swordfish.lemuroid.common.displayToast
import com.swordfish.lemuroid.lib.core.CoresSelection
import com.swordfish.lemuroid.lib.library.GameSystem
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.preferences.SharedPreferencesHelper
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.DelicateCoroutinesApi
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.GlobalScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import timber.log.Timber
import java.util.concurrent.atomic.AtomicBoolean

class GameLauncher(
    private val coresSelection: CoresSelection,
    private val gameLaunchTaskHandler: GameLaunchTaskHandler,
) {
    /** Pokemulador: mientras se abre un juego no se abre otro (dos toques seguidos). */
    private val abriendo = AtomicBoolean(false)

    /**
     * [conElDeAntes]: con el núcleo de libretro aunque el juego vaya con Panther.
     * [onError]: no se ha podido abrir (ya se ha dicho por qué).
     */
    @OptIn(DelicateCoroutinesApi::class)
    fun launchGameAsync(
        activity: Activity,
        game: Game,
        loadSave: Boolean,
        leanback: Boolean,
        perfil: String? = null,
        sala: String? = null,
        conElDeAntes: Boolean = false,
        onError: () -> Unit = {},
    ): Boolean {
        if (!abriendo.compareAndSet(false, true)) return false

        GlobalScope.launch {
            try {
                if (!cerrarElAnterior(activity)) {
                    withContext(Dispatchers.Main) { onError() }
                    return@launch
                }
                val system = GameSystem.findById(game.systemId)
                val coreConfig = coresSelection.getCoreConfigForSystem(system)
                val app = activity.applicationContext
                // Pokemulador: GB, GBC y GBA van con el emulador Panther.
                var panther = !leanback && !conElDeAntes && Panther.vale(app, game)
                if (panther && loadSave && guardadoAutomatico(app) &&
                    Panther.aMediasConElDeAntes(app, game, perfil, coreConfig.coreID)
                ) {
                    panther =
                        preguntar(
                            activity,
                            R.string.emudex_panther_a_medias,
                            R.string.emudex_panther_a_medias_texto,
                            R.string.emudex_panther_abrir,
                            R.string.emudex_panther_con_el_de_antes,
                        ) ?: run {
                            withContext(Dispatchers.Main) { onError() }
                            return@launch
                        }
                }
                gameLaunchTaskHandler.handleGameStart(app)
                // En GBA, prepara (o quita) la partida compartida de la sala. Panther lleva los Caramelos dentro.
                PartidaCompartidaGba.preparar(app, game, sala, conCaramelos = !panther)
                if (panther) {
                    JuegoPanther.abrir(activity, game, loadSave, perfil, sala)
                } else {
                    BaseGameActivity.launchGame(activity, coreConfig, game, loadSave, leanback, perfil, sala)
                }
            } catch (e: Throwable) {
                Timber.e(e, "No se pudo abrir el juego")
                withContext(Dispatchers.Main) { onError() }
            } finally {
                abriendo.set(false)
            }
        }

        return true
    }

    /**
     * Pokemulador: si ha quedado un juego abierto (o colgado) se cierra, guardando, y se sigue. Antes
     * salía «cierra el juego» y, si estaba colgado, ya no se podía abrir ninguno.
     */
    private suspend fun cerrarElAnterior(activity: Activity): Boolean {
        val app = activity.applicationContext
        if (!JuegoAbierto.hay(app)) return true
        // Sin el guardado automático se perdería lo que no se haya guardado dentro del juego.
        if (!guardadoAutomatico(app)) {
            val cerrarlo =
                preguntar(
                    activity,
                    R.string.emudex_hay_juego_abierto,
                    R.string.emudex_hay_juego_abierto_texto,
                    R.string.emudex_cerrarlo_y_seguir,
                    android.R.string.cancel,
                )
            if (cerrarlo != true) return false
        }
        withContext(Dispatchers.Main) { activity.displayToast(R.string.emudex_cerrando_juego) }
        if (JuegoAbierto.cerrar(app)) return true
        withContext(Dispatchers.Main) { activity.displayToast(R.string.emudex_no_se_cierra) }
        return false
    }

    private fun guardadoAutomatico(app: Context) =
        SharedPreferencesHelper.getSharedPreferences(app).getBoolean(app.getString(R.string.pref_key_autosave), true)

    /** true, false o null (no ha elegido ninguna). */
    private suspend fun preguntar(
        activity: Activity,
        titulo: Int,
        texto: Int,
        si: Int,
        no: Int,
    ): Boolean? {
        val respuesta = CompletableDeferred<Boolean?>()
        withContext(Dispatchers.Main) {
            if (activity.isFinishing) {
                respuesta.complete(null)
                return@withContext
            }
            AlertDialog.Builder(activity)
                .setTitle(titulo)
                .setMessage(texto)
                .setPositiveButton(si) { _, _ -> respuesta.complete(true) }
                .setNegativeButton(no) { _, _ -> respuesta.complete(false) }
                .setOnCancelListener { respuesta.complete(null) }
                .show()
        }
        // Si la pantalla se va (al girar el móvil), el diálogo desaparece sin avisar.
        while (!respuesta.isCompleted) {
            delay(MIRAR_CADA_MS)
            if (activity.isDestroyed) respuesta.complete(null)
        }
        return respuesta.await()
    }

    companion object {
        private const val MIRAR_CADA_MS = 200L
    }
}
