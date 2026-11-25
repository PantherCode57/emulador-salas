package com.swordfish.lemuroid.lib.saves

import com.swordfish.lemuroid.lib.library.SystemID
import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * Pokemulador: la partida de un juego es la misma (<juego>.srm) en el emulador Panther y en el de
 * antes (Gambatte y mGBA de libretro). Lo único que cambia es lo que cada uno pone detrás: el reloj
 * del cartucho.
 *
 *  - GB y GBC: Gambatte guarda 8 bytes (la hora en que el reloj del juego estaba a cero); mGBA, 48
 *    (el reloj y cuándo se guardó), que es también lo de VisualBoyAdvance y linkcable.app.
 *  - GBA: mGBA añade 16 bytes; el núcleo de libretro no, y no espera partidas más largas.
 */
object FormatoPartida {
    private const val RELOJ_GAMBATTE = 8
    private const val RELOJ_MGBA = 48
    private const val RELOJ_MGBA_32 = 44 // sin la parte alta de la hora
    private const val RELOJ_GBA = 16
    private const val MINIMO = 512

    /** Tal como la espera el núcleo de antes. */
    fun paraNucleo(
        systemId: String,
        datos: ByteArray,
    ): ByteArray {
        val (tam, sobra) = partir(datos) ?: return datos
        return when {
            esGameBoy(systemId) && (sobra == RELOJ_MGBA || sobra == RELOJ_MGBA_32) ->
                datos.copyOf(tam) + aGambatte(datos, tam, sobra)
            systemId == SystemID.GBA.dbname && sobra == RELOJ_GBA -> datos.copyOf(tam)
            else -> datos
        }
    }

    /** Tal como la espera Panther (mGBA). */
    fun paraPanther(
        systemId: String,
        datos: ByteArray,
        ahora: Long = System.currentTimeMillis() / 1000,
    ): ByteArray {
        val (tam, sobra) = partir(datos) ?: return datos
        return if (esGameBoy(systemId) && sobra == RELOJ_GAMBATTE) datos.copyOf(tam) + aMgba(datos, tam, ahora) else datos
    }

    private fun esGameBoy(systemId: String) = systemId == SystemID.GB.dbname || systemId == SystemID.GBC.dbname

    /** El tamaño de la memoria del cartucho (siempre una potencia de dos) y lo que sobra detrás. */
    private fun partir(datos: ByteArray): Pair<Int, Int>? {
        if (datos.size < MINIMO) return null
        val potencia = Integer.highestOneBit(datos.size)
        return potencia to datos.size - potencia
    }

    private fun aGambatte(
        datos: ByteArray,
        tam: Int,
        sobra: Int,
    ): ByteArray {
        val b = ByteBuffer.wrap(datos, tam, sobra).order(ByteOrder.LITTLE_ENDIAN)
        val segundos = b.int.toLong() and 0x3F
        val minutos = b.int.toLong() and 0x3F
        val horas = b.int.toLong() and 0x1F
        val dias = b.int.toLong() and 0xFF
        val diasAlto = b.int.toLong() and 0x01
        repeat(5) { b.int } // los mismos, retenidos
        val guardado = if (sobra == RELOJ_MGBA) b.long else b.int.toLong() and 0xFFFFFFFFL
        val lleva = segundos + minutos * 60 + horas * 3600 + (dias + diasAlto * 256) * 86400
        return ByteBuffer.allocate(RELOJ_GAMBATTE).order(ByteOrder.LITTLE_ENDIAN).putLong(guardado - lleva).array()
    }

    private fun aMgba(
        datos: ByteArray,
        tam: Int,
        ahora: Long,
    ): ByteArray {
        val cero = ByteBuffer.wrap(datos, tam, RELOJ_GAMBATTE).order(ByteOrder.LITTLE_ENDIAN).long
        // Una hora sin sentido (a cero o en el futuro): el reloj empieza de nuevo.
        val lleva = if (cero in 1..ahora) ahora - cero else 0L
        val dias = lleva / 86400
        val b = ByteBuffer.allocate(RELOJ_MGBA).order(ByteOrder.LITTLE_ENDIAN)
        repeat(2) {
            b.putInt((lleva % 60).toInt())
            b.putInt((lleva / 60 % 60).toInt())
            b.putInt((lleva / 3600 % 24).toInt())
            b.putInt((dias % 256).toInt())
            b.putInt((dias / 256 % 2).toInt())
        }
        b.putLong(ahora)
        return b.array()
    }
}
