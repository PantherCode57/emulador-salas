package com.swordfish.lemuroid.app.shared.game

import android.app.Activity
import android.app.AlertDialog
import android.content.Context
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.feature.online.PartidaCompartidaGba
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

    /** [onError]: no se ha podido abrir (ya se ha dicho por qué). */
    @OptIn(DelicateCoroutinesApi::class)
    fun launchGameAsync(
        activity: Activity,
        game: Game,
        loadSave: Boolean,
        leanback: Boolean,
        perfil: String? = null,
        sala: String? = null,
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
                gameLaunchTaskHandler.handleGameStart(activity.applicationContext)
                // Pokemulador: en GBA, prepara (o quita) la partida compartida de la sala.
                PartidaCompartidaGba.preparar(activity.applicationContext, game, sala)
                BaseGameActivity.launchGame(activity, coreConfig, game, loadSave, leanback, perfil, sala)
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
        if (!guardadoAutomatico(app) && !preguntar(activity)) return false
        withContext(Dispatchers.Main) { activity.displayToast(R.string.emudex_cerrando_juego) }
        if (JuegoAbierto.cerrar(app)) return true
        withContext(Dispatchers.Main) { activity.displayToast(R.string.emudex_no_se_cierra) }
        return false
    }

    private fun guardadoAutomatico(app: Context) =
        SharedPreferencesHelper.getSharedPreferences(app).getBoolean(app.getString(R.string.pref_key_autosave), true)

    private suspend fun preguntar(activity: Activity): Boolean {
        val respuesta = CompletableDeferred<Boolean>()
        withContext(Dispatchers.Main) {
            if (activity.isFinishing) {
                respuesta.complete(false)
                return@withContext
            }
            AlertDialog.Builder(activity)
                .setTitle(R.string.emudex_hay_juego_abierto)
                .setMessage(R.string.emudex_hay_juego_abierto_texto)
                .setPositiveButton(R.string.emudex_cerrarlo_y_seguir) { _, _ -> respuesta.complete(true) }
                .setNegativeButton(android.R.string.cancel) { _, _ -> respuesta.complete(false) }
                .setOnCancelListener { respuesta.complete(false) }
                .show()
        }
        return respuesta.await()
    }
}
