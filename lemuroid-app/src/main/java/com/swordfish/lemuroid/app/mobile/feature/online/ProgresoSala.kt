package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.Base64
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.File

/**
 * "Cómo vas" en una sala guardada.
 *
 * Cada jugador tiene un archivo con su progreso en la sala (nombre, juego, tiempo jugado, cuándo
 * jugó por última vez y una captura de la pantalla). El proceso del juego lo actualiza al salir
 * de la partida y cada pocos minutos; el enlace de la sala lo manda a los amigos cuando cambia y
 * guarda el de ellos en la carpeta "amigos". Así se ve cómo va cada uno aunque ya no esté.
 */
object ProgresoSala {
    private const val VERSION = 1
    private const val LADO_CAPTURA = 480

    data class Progreso(
        val id: String,
        val nombre: String,
        val juego: String,
        val partidaPropia: Boolean,
        val jugadoMs: Long,
        val ultimaVez: Long,
        val captura: Bitmap?,
    )

    fun carpetaSala(
        context: Context,
        codigo: String,
    ): File = File(File(context.filesDir, "salas"), codigo)

    fun archivoPropio(
        context: Context,
        codigo: String,
    ): File = File(carpetaSala(context, codigo), "mi_progreso.json")

    fun carpetaAmigos(
        context: Context,
        codigo: String,
    ): File = File(carpetaSala(context, codigo), "amigos")

    /** Deja listo nuestro archivo al entrar en la sala (nombre y juego al día, sin tocar lo jugado). */
    @Synchronized
    fun preparar(
        context: Context,
        sala: SalaOnline.Sala,
    ) {
        val archivo = archivoPropio(context, sala.codigo)
        val json = leerJson(archivo) ?: JSONObject()
        json.put("version", VERSION)
            .put("id", SalaOnline.idJugador(context))
            .put("nombre", SalaOnline.nombreVisible(context))
            .put("juego", sala.juegoTitulo)
            .put("partida", if (sala.partidaPropia) "sala" else "principal")
        if (!json.has("jugadoMs")) json.put("jugadoMs", 0L)
        escribir(archivo, json)
    }

    /** Lo llama el proceso del juego: suma el tiempo jugado y guarda una captura nueva si la hay. */
    @Synchronized
    fun guardar(
        context: Context,
        codigo: String,
        juego: String,
        msJugados: Long,
        captura: Bitmap?,
    ) {
        val archivo = archivoPropio(context, codigo)
        val json = leerJson(archivo) ?: JSONObject()
        json.put("version", VERSION)
            .put("id", SalaOnline.idJugador(context))
            .put("nombre", SalaOnline.nombreVisible(context))
            .put("juego", juego)
            .put("jugadoMs", json.optLong("jugadoMs") + msJugados.coerceAtLeast(0))
            .put("ultimaVez", System.currentTimeMillis())
        captura?.let { json.put("captura", aBase64(it)) }
        escribir(archivo, json)
    }

    fun propio(
        context: Context,
        codigo: String,
    ): Progreso? = leerJson(archivoPropio(context, codigo))?.let(::aProgreso)

    fun amigos(
        context: Context,
        codigo: String,
    ): List<Progreso> {
        val propio = SalaOnline.idJugador(context)
        return carpetaAmigos(context, codigo)
            .listFiles { f -> f.name.endsWith(".json") }
            .orEmpty()
            .mapNotNull { leerJson(it)?.let(::aProgreso) }
            .filter { it.id != propio }
            .sortedByDescending { it.ultimaVez }
    }

    private fun aProgreso(json: JSONObject): Progreso? {
        val id = json.optString("id").takeIf { it.isNotBlank() } ?: return null
        val captura =
            json.optString("captura").takeIf { it.isNotBlank() }?.let {
                runCatching {
                    val bytes = Base64.decode(it, Base64.DEFAULT)
                    BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                }.getOrNull()
            }
        return Progreso(
            id = id,
            nombre = json.optString("nombre").ifBlank { id },
            juego = json.optString("juego"),
            partidaPropia = json.optString("partida") == "sala",
            jugadoMs = json.optLong("jugadoMs"),
            ultimaVez = json.optLong("ultimaVez"),
            captura = captura,
        )
    }

    private fun aBase64(bitmap: Bitmap): String {
        val escala = LADO_CAPTURA / maxOf(bitmap.width, bitmap.height).toFloat()
        val pequena =
            if (escala < 1f) {
                Bitmap.createScaledBitmap(
                    bitmap,
                    (bitmap.width * escala).toInt().coerceAtLeast(1),
                    (bitmap.height * escala).toInt().coerceAtLeast(1),
                    true,
                )
            } else {
                bitmap
            }
        val salida = ByteArrayOutputStream()
        pequena.compress(Bitmap.CompressFormat.JPEG, 70, salida)
        return Base64.encodeToString(salida.toByteArray(), Base64.NO_WRAP)
    }

    private fun leerJson(archivo: File): JSONObject? =
        runCatching { JSONObject(archivo.readText()) }.getOrNull()

    private fun escribir(
        archivo: File,
        json: JSONObject,
    ) {
        archivo.parentFile?.mkdirs()
        val temporal = File(archivo.parentFile, archivo.name + ".tmp")
        temporal.writeText(json.toString())
        if (!temporal.renameTo(archivo)) {
            archivo.writeText(json.toString())
            temporal.delete()
        }
    }
}
