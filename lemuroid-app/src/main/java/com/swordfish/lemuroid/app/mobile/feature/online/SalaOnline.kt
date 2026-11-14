package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import com.swordfish.lemuroid.BuildConfig
import com.swordfish.lemuroid.lib.core.CoreVariable
import com.swordfish.lemuroid.lib.library.SystemID
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONObject
import java.io.File
import java.security.SecureRandom
import kotlin.concurrent.thread

/**
 * Salas online de Pokemulador (proceso principal de la app).
 *
 * Arranca el enlace de Tailscale (libenlace.so, el mismo programa que en PC) y lee su estado.
 * Deja escrito sala_online.json para que el proceso del juego sepa a qué sala entrar: el núcleo
 * de 3DS (Azahar) recibe las variables citra_pkm_room_* y crea la sala o entra en ella.
 */
object SalaOnline {
    const val PUERTO = 24872
    private const val ARCHIVO = "sala_online.json"
    private const val LETRAS = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

    data class Estado(
        val codigo: String? = null,
        val anfitrion: Boolean = false,
        /** conectando, listo, buscando, conectado o error (las mismas fases que en PC). */
        val fase: String? = null,
        val error: String? = null,
        val amigos: Int = 0,
        val ruta: String? = null,
    )

    private val estadoInterno = MutableStateFlow(Estado())
    val estado: StateFlow<Estado> = estadoInterno.asStateFlow()

    @Volatile
    private var proceso: Process? = null

    val tieneClave: Boolean
        get() = BuildConfig.POKEMULADOR_TS_KEY.isNotBlank()

    fun crear(context: Context) = abrir(context, nuevoCodigo(), true)

    fun unirse(
        context: Context,
        codigo: String,
    ) = abrir(context, codigo.trim().uppercase(), false)

    @Synchronized
    fun salir(context: Context) {
        val app = context.applicationContext
        cerrarProceso()
        File(app.filesDir, ARCHIVO).delete()
        SalaService.detener(app)
        estadoInterno.value = Estado()
    }

    /** Si la app se reinició, borra la sala que pudiera quedar de antes (ya no hay enlace). */
    @Synchronized
    fun limpiarSiHuerfana(context: Context) {
        if (proceso == null) {
            File(context.applicationContext.filesDir, ARCHIVO).delete()
        }
    }

    /** Variables para el núcleo de 3DS. Se llama desde el proceso del juego. */
    fun variablesNucleo(
        context: Context,
        systemID: SystemID,
    ): List<CoreVariable> {
        if (systemID != SystemID.NINTENDO_3DS) return emptyList()
        val json =
            try {
                JSONObject(File(context.filesDir, ARCHIVO).readText())
            } catch (e: Exception) {
                return emptyList()
            }
        return listOf(
            CoreVariable("citra_pkm_room_mode", if (json.optBoolean("anfitrion")) "host" else "join"),
            CoreVariable("citra_pkm_room_port", PUERTO.toString()),
            CoreVariable("citra_pkm_room_password", json.optString("codigo")),
            CoreVariable("citra_pkm_room_nickname", json.optString("nombre", "Jugador")),
        )
    }

    @Synchronized
    private fun abrir(
        context: Context,
        codigo: String,
        anfitrion: Boolean,
    ) {
        val app = context.applicationContext
        cerrarProceso()
        estadoInterno.value = Estado(codigo = codigo, anfitrion = anfitrion, fase = "conectando")

        File(app.filesDir, ARCHIVO).writeText(
            JSONObject()
                .put("codigo", codigo)
                .put("anfitrion", anfitrion)
                .put("nombre", nombreJugador(app))
                .toString(),
        )

        try {
            val exe = File(app.applicationInfo.nativeLibraryDir, "libenlace.so")
            val builder =
                ProcessBuilder(
                    exe.absolutePath,
                    if (anfitrion) "host" else "unirse",
                    "--codigo",
                    codigo,
                    "--puerto",
                    PUERTO.toString(),
                )
            builder.environment()["POKEMULADOR_TS_KEY"] = BuildConfig.POKEMULADOR_TS_KEY
            builder.environment()["TMPDIR"] = app.cacheDir.absolutePath
            builder.environment()["HOME"] = app.filesDir.absolutePath
            builder.redirectErrorStream(true)
            val nuevo = builder.start()
            proceso = nuevo
            thread(name = "sala-online", isDaemon = true) { leer(nuevo) }
            SalaService.iniciar(app, codigo)
        } catch (e: Exception) {
            File(app.filesDir, ARCHIVO).delete()
            estadoInterno.value =
                Estado(codigo, anfitrion, "error", "No se pudo arrancar el enlace online: ${e.message}")
        }
    }

    private fun leer(p: Process) {
        try {
            p.inputStream.bufferedReader().useLines { lineas -> lineas.forEach(::procesar) }
        } catch (e: Exception) {
            // El proceso se cerró: lo tratamos abajo.
        }
        if (proceso === p) {
            val actual = estadoInterno.value
            if (actual.codigo != null && actual.fase != "error") {
                estadoInterno.value = actual.copy(fase = "error", error = "Se ha cerrado la conexión de la sala.")
            }
        }
    }

    private fun procesar(linea: String) {
        val json =
            try {
                JSONObject(linea)
            } catch (e: Exception) {
                return
            }
        val actual = estadoInterno.value
        estadoInterno.value =
            when (val fase = json.optString("estado")) {
                "amigos" -> actual.copy(amigos = json.optInt("n"))
                "ruta" -> actual.copy(ruta = json.optString("tipo"))
                "error" -> actual.copy(fase = "error", error = json.optString("mensaje"))
                "" -> actual
                else -> actual.copy(fase = fase)
            }
    }

    private fun cerrarProceso() {
        proceso?.let { p ->
            proceso = null
            try {
                p.outputStream.close()
            } catch (e: Exception) {
            }
            p.destroy()
        }
    }

    private fun nuevoCodigo(): String {
        val random = SecureRandom()
        return (1..6).map { LETRAS[random.nextInt(LETRAS.length)] }.joinToString("")
    }

    /** Nombre en la sala: tiene que ser distinto en cada móvil, así que lo generamos una vez. */
    private fun nombreJugador(context: Context): String {
        val prefs = context.getSharedPreferences("pokemulador_online", Context.MODE_PRIVATE)
        prefs.getString("nombre", null)?.let { return it }
        val random = SecureRandom()
        val nombre = "Jugador-" + (1..4).map { LETRAS[random.nextInt(LETRAS.length)] }.joinToString("")
        prefs.edit().putString("nombre", nombre).apply()
        return nombre
    }
}
