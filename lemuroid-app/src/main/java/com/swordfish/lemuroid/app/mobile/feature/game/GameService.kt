package com.swordfish.lemuroid.app.mobile.feature.game

import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.Process
import androidx.core.app.NotificationManagerCompat
import androidx.core.app.ServiceCompat
import com.swordfish.lemuroid.app.mobile.shared.NotificationsManager
import com.swordfish.lemuroid.app.shared.game.BaseGameActivity
import dagger.android.DaggerService
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.cancel
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull
import timber.log.Timber
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.concurrent.thread
import kotlin.system.exitProcess

class GameService : DaggerService() {
    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onBind(intent: Intent?) = null

    override fun onCreate() {
        super.onCreate()
        serviceScope.launch {
            awaitTermination()
            hideNotification()
            // Pokemulador: esto se hacía en el hilo principal. Si ese hilo estaba colgado o se había
            // acabado por un fallo, el proceso no se cerraba nunca y no dejaba abrir más juegos.
            acabarProceso()
        }
    }

    override fun onStartCommand(
        intent: Intent?,
        flags: Int,
        startId: Int,
    ): Int {
        intent?.let { runCatching { displayNotification(it) }.onFailure { e -> Timber.e(e, "Sin el aviso del juego") } }
        return START_NOT_STICKY
    }

    private fun displayNotification(intent: Intent) {
        val gameIntent =
            intent.getParcelableExtra<Intent>(EXTRA_GAME_ACTIVITY_INTENT)
                ?: return
        val notification = NotificationsManager(applicationContext).gameRunningNotification(gameIntent)
        // Pokemulador: «specialUse» en vez de «dataSync». Desde Android 15, a las 6 horas en segundo
        // plano con «dataSync» el sistema hace fallar la app, y el juego se quedaba colgado por dentro.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                NotificationsManager.GAME_RUNNING_NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
            )
        } else {
            startForeground(NotificationsManager.GAME_RUNNING_NOTIFICATION_ID, notification)
        }
    }

    private fun hideNotification() {
        NotificationManagerCompat.from(this).cancel(NotificationsManager.GAME_RUNNING_NOTIFICATION_ID)
    }

    /** Pokemulador: se ha quitado la app de la lista de recientes. Si el juego ya no está, se acaba el proceso. */
    override fun onTaskRemoved(rootIntent: Intent?) {
        super.onTaskRemoved(rootIntent)
        serviceScope.launch {
            delay(ESPERA_TAREA_QUITADA_MS)
            if (BaseGameActivity.abiertas == 0) requestTermination()
        }
    }

    override fun onDestroy() {
        serviceScope.cancel()
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        hideNotification()
        super.onDestroy()
    }

    companion object {
        private const val EXTRA_GAME_ACTIVITY_INTENT = "EXTRA_GAME_ACTIVITY_INTENT"

        /** Lo que puede tardar en guardar la partida antes de dejarlo estar. */
        private const val PLAZO_TAREA_MS = 6000L

        /** Pase lo que pase, el proceso del juego se acaba este tiempo después de pedirlo. */
        private const val PLAZO_FINAL_MS = 9000L
        private const val PLAZO_SALIR_MS = 1500L
        private const val ESPERA_TAREA_QUITADA_MS = 3000L

        private data class GameProcessTask(
            val task: suspend () -> Unit = {},
            val terminate: Boolean = false,
        )

        private val tasks = Channel<GameProcessTask>(capacity = Channel.BUFFERED)

        // Las tareas usan el núcleo (código nativo): si se cuelga no se pueden cancelar, solo dejarlas.
        private val tareas = CoroutineScope(SupervisorJob() + Dispatchers.IO)
        private val vigilando = AtomicBoolean(false)

        fun startService(
            context: Context,
            gameActivityIntent: Intent,
        ) {
            context.startService(
                Intent(context, GameService::class.java).apply {
                    putExtra(EXTRA_GAME_ACTIVITY_INTENT, gameActivityIntent)
                },
            )
        }

        fun schedule(task: suspend () -> Unit) {
            val result = tasks.trySend(GameProcessTask(task = task))
            Timber.i("GameService.schedule sent=%s", result.isSuccess)
        }

        fun requestTermination() {
            val result = tasks.trySend(GameProcessTask(terminate = true))
            Timber.i("GameService.requestTermination sent=%s", result.isSuccess)
            acabarSeguro(PLAZO_FINAL_MS)
        }

        /**
         * Pokemulador: cierra este proceso pasado [ms], aunque el juego o el hilo principal estén
         * colgados. Sin esto, un juego colgado no dejaba abrir ninguno más.
         */
        fun acabarSeguro(ms: Long) {
            if (!vigilando.compareAndSet(false, true)) return
            thread(name = "fin-del-juego", isDaemon = true) {
                Thread.sleep(ms)
                Timber.e("El juego no se ha cerrado a tiempo: se cierra su proceso")
                Process.killProcess(Process.myPid())
            }
        }

        private fun acabarProceso() {
            // exitProcess ejecuta lo que los núcleos dejan para el final, que también se puede colgar.
            thread(name = "salir-del-juego", isDaemon = true) {
                Thread.sleep(PLAZO_SALIR_MS)
                Process.killProcess(Process.myPid())
            }
            exitProcess(0)
        }

        private suspend fun awaitTermination() {
            for (task in tasks) {
                val trabajo = tareas.async { runCatching { task.task() }.onFailure { Timber.e(it, "GameService task failed") } }
                if (withTimeoutOrNull(PLAZO_TAREA_MS) { trabajo.await() } == null) {
                    Timber.e("GameService: una tarea no ha acabado a tiempo")
                }

                if (task.terminate) {
                    return
                }
            }
        }
    }
}
