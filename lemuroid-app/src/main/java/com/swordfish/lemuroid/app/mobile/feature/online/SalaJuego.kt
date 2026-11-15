package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import android.graphics.Bitmap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import timber.log.Timber

/**
 * Lo que necesita el proceso del juego cuando se juega desde una sala: cuenta el tiempo jugado
 * (solo mientras el juego está en pantalla) y guarda el progreso que ven los amigos.
 */
class SalaJuego(
    private val context: Context,
    private val codigo: String,
    private val juego: String,
) {
    private var inicioTramo = 0L
    private var acumulado = 0L

    @Synchronized
    fun reanudar() {
        if (inicioTramo == 0L) inicioTramo = System.currentTimeMillis()
    }

    @Synchronized
    fun pausar() {
        if (inicioTramo != 0L) {
            acumulado += System.currentTimeMillis() - inicioTramo
            inicioTramo = 0L
        }
    }

    /** Devuelve el tiempo jugado desde la última vez y vuelve a empezar a contar. */
    @Synchronized
    private fun tomarTiempo(): Long {
        val ahora = System.currentTimeMillis()
        if (inicioTramo != 0L) {
            acumulado += ahora - inicioTramo
            inicioTramo = ahora
        }
        val total = acumulado
        acumulado = 0L
        return total
    }

    suspend fun guardar(captura: Bitmap?) {
        val ms = tomarTiempo()
        withContext(Dispatchers.IO) {
            runCatching { ProgresoSala.guardar(context, codigo, juego, ms, captura) }
                .onFailure { Timber.e(it, "No se pudo guardar el progreso de la sala") }
        }
    }

    companion object {
        const val LADO_CAPTURA = 480
        const val CADA_CUANTO_MS = 5L * 60L * 1000L
    }
}
