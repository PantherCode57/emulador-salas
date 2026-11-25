package com.swordfish.lemuroid.app.shared.game

import android.app.ActivityManager
import android.content.Context
import android.content.Intent
import android.os.Process
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext
import timber.log.Timber
import java.io.File

/**
 * Pokemulador: cierra el juego que se haya quedado abierto, para poder abrir otro.
 *
 * Los juegos van en un proceso aparte y solo puede haber uno. Antes, si ese proceso se quedaba colgado
 * (un fallo, la conexión de una sala que se corta…), la app decía «cierra el juego» para siempre y no
 * dejaba entrar en ninguno: deslizarla fuera de la lista de recientes no lo cierra, porque tiene un
 * aviso fijo. Ahora se le pide que guarde y se cierre y, si en unos segundos no contesta, se le cierra
 * a la fuerza. Las partidas no se tocan: están en sus archivos.
 */
object JuegoAbierto {
    /** El juego (en su proceso) guarda y se cierra al recibir esto. */
    const val ACCION_CERRAR = "com.swordfish.lemuroid.CERRAR_JUEGO"

    private val PROCESOS = listOf(":game", ":panther")
    private const val ESPERA_CORTA_MS = 1500L
    private const val ESPERA_POR_LAS_BUENAS_MS = 7000L
    private const val ESPERA_A_LA_FUERZA_MS = 3000L
    private const val CADA_MS = 150L

    fun hay(context: Context): Boolean = GameProcessLock.isHeldByAnotherProcess(context.applicationContext)

    /** Devuelve true si ya no queda ningún juego abierto. */
    suspend fun cerrar(context: Context): Boolean =
        withContext(Dispatchers.IO) {
            val app = context.applicationContext
            if (!hay(app)) return@withContext true
            app.sendBroadcast(Intent(ACCION_CERRAR).setPackage(app.packageName))
            if (esperar(app, ESPERA_CORTA_MS)) return@withContext true
            // Uno en segundo plano que Android ha dejado parado no contesta (y ya guardó al pararse).
            if (!parado(app) && esperar(app, ESPERA_POR_LAS_BUENAS_MS)) return@withContext true
            Timber.e("El juego no se cierra: se cierra su proceso")
            matar(app)
            esperar(app, ESPERA_A_LA_FUERZA_MS)
        }

    private suspend fun esperar(
        app: Context,
        ms: Long,
    ): Boolean {
        val hasta = System.currentTimeMillis() + ms
        while (System.currentTimeMillis() < hasta) {
            if (!hay(app)) return true
            delay(CADA_MS)
        }
        return !hay(app)
    }

    private fun procesos(app: Context): List<ActivityManager.RunningAppProcessInfo> {
        val gestor = app.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        return gestor.runningAppProcesses.orEmpty().filter { it.uid == Process.myUid() && esDeJuego(it.processName) }
    }

    private fun parado(app: Context): Boolean {
        val vistos = procesos(app)
        return vistos.isNotEmpty() &&
            vistos.all { it.importance >= ActivityManager.RunningAppProcessInfo.IMPORTANCE_CACHED }
    }

    private fun matar(app: Context) {
        val propio = Process.myPid()
        val vistos = procesos(app).map { it.pid }
        val apuntado = GameProcessLock.procesoApuntado(app)?.takeIf { esDeJuego(nombre(it)) }
        (vistos + listOfNotNull(apuntado)).distinct().filter { it != propio }.forEach { pid ->
            Timber.e("Cerrando el proceso del juego $pid")
            Process.killProcess(pid)
        }
    }

    private fun esDeJuego(proceso: String?) = proceso != null && PROCESOS.any { proceso.endsWith(it) }

    /** El nombre de un proceso nuestro (de los demás no se puede leer). */
    private fun nombre(pid: Int): String? =
        runCatching { File("/proc/$pid/cmdline").readText().substringBefore('\u0000').trim() }.getOrNull()
}
