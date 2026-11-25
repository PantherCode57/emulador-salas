package com.swordfish.lemuroid.lib.saves

import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Pokemulador: copias de seguridad de las partidas (los .srm), en la carpeta «copias» que hay al lado.
 *
 * Antes de cambiar una partida se guarda la que había (como mucho una cada [CADA_MS], para no llenar
 * el móvil) y se dejan las últimas [CUANTAS] de cada juego. Así, si algo sale mal, siempre se puede
 * volver a una anterior desde el menú del juego (mantener pulsado › Recuperar una copia).
 */
object CopiasPartida {
    private const val CARPETA = "copias"
    private const val CUANTAS = 15
    private const val CADA_MS = 10L * 60L * 1000L
    private val MARCA = Regex("-(\\d{8}-\\d{6})\\.srm$")

    data class Copia(
        val archivo: File,
        val fecha: Date,
    )

    /** La carpeta de las copias de una carpeta de partidas. */
    fun carpeta(saves: File): File = File(saves, CARPETA)

    /**
     * Guarda una copia de [partida]. Con [siempre] se hace aunque haya una reciente; nunca se repite
     * una copia igual a la última, ni se copian partidas vacías.
     */
    fun hacer(
        partida: File,
        siempre: Boolean = false,
    ): File? {
        if (!partida.exists() || partida.length() == 0L) return null
        val base = partida.name.removeSuffix(".srm")
        val anteriores = listar(partida)
        val ultima = anteriores.firstOrNull()
        if (!siempre && ultima != null && System.currentTimeMillis() - ultima.archivo.lastModified() < CADA_MS) return null
        val datos = partida.readBytes()
        if (estaVacia(datos)) return null
        if (ultima != null && ultima.archivo.length() == datos.size.toLong() && ultima.archivo.readBytes().contentEquals(datos)) {
            return null
        }
        val copias = carpeta(partida.parentFile!!).apply { mkdirs() }
        val destino = File(copias, "$base-${marca()}.srm")
        val temporal = File(copias, destino.name + ".tmp")
        temporal.writeBytes(datos)
        if (!temporal.renameTo(destino)) {
            temporal.delete()
            return null
        }
        listar(partida).drop(CUANTAS).forEach { it.archivo.delete() }
        return destino
    }

    /** Las copias de [partida], de la más nueva a la más vieja. */
    fun listar(partida: File): List<Copia> {
        val base = partida.name.removeSuffix(".srm")
        return carpeta(partida.parentFile!!)
            .listFiles { f -> f.isFile && f.name.startsWith("$base-") && MARCA.containsMatchIn(f.name) }
            .orEmpty()
            .mapNotNull { f ->
                val marca = MARCA.find(f.name)!!
                // (otro juego cuyo nombre empiece igual: «Pokemon» y «Pokemon-Rojo»)
                if (f.name.removeSuffix(marca.value) != base) return@mapNotNull null
                val fecha =
                    runCatching { formato().parse(marca.groupValues[1]) }.getOrNull() ?: return@mapNotNull null
                Copia(f, fecha)
            }
            .sortedByDescending { it.archivo.name }
    }

    /** Una partida sin nada (todo a 0xFF o todo a 0): la de un cartucho recién borrado. */
    fun estaVacia(datos: ByteArray): Boolean {
        if (datos.isEmpty()) return true
        val primero = datos[0]
        if (primero != 0.toByte() && primero != 0xFF.toByte()) return false
        return datos.all { it == primero }
    }

    fun marca(): String = formato().format(Date())

    private fun formato() = SimpleDateFormat("yyyyMMdd-HHmmss", Locale.ROOT)
}
