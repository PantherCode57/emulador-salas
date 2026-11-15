package com.swordfish.lemuroid.app.mobile.feature.online

import android.content.Context
import com.swordfish.lemuroid.BuildConfig
import com.swordfish.lemuroid.lib.core.CoreVariable
import com.swordfish.lemuroid.lib.library.SystemID
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.security.SecureRandom
import kotlin.concurrent.thread

/**
 * Salas online de Pokemulador (proceso principal de la app).
 *
 * Las salas se guardan: cada una tiene su código, su juego y su partida (la de siempre o una
 * propia solo para la sala). Se puede entrar cuando se quiera: el enlace de Tailscale
 * (libenlace.so, el mismo programa que en PC) mira si algún amigo ya la tiene abierta y, si no,
 * la abre él. También manda nuestro progreso a los amigos y guarda el suyo (ver ProgresoSala).
 *
 * Deja escrito sala_online.json para que el proceso del juego sepa a qué sala entrar: el núcleo
 * de 3DS (Azahar) recibe las variables citra_pkm_room_* y crea la sala o entra en ella.
 */
object SalaOnline {
    const val PUERTO = 24872
    private const val ARCHIVO = "sala_online.json"
    private const val ARCHIVO_SALAS = "salas/salas.json"
    private const val LETRAS = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
    private const val PREFERENCIAS = "pokemulador_online"

    data class Sala(
        val codigo: String,
        val nombre: String,
        val juegoId: Int,
        val juegoUri: String,
        val juegoTitulo: String,
        val sistema: String,
        /** true: la sala tiene su propia partida; false: se juega con la partida de siempre. */
        val partidaPropia: Boolean,
        val creada: Long,
    ) {
        /** Carpeta de partidas que usa el juego (null = la de siempre). */
        val perfil: String?
            get() = if (partidaPropia) perfilDeSala(codigo) else null

        val esDe3DS: Boolean
            get() = sistema == SystemID.NINTENDO_3DS.dbname

        val esDeGba: Boolean
            get() = sistema == SystemID.GBA.dbname

        val esDeGameBoy: Boolean
            get() = sistema == SystemID.GB.dbname || sistema == SystemID.GBC.dbname

        val esDeDs: Boolean
            get() = sistema == SystemID.NDS.dbname
    }

    data class Estado(
        /** Código de la sala en la que estamos dentro (null = en ninguna). */
        val codigo: String? = null,
        val anfitrion: Boolean = false,
        /** Ya sabemos si abrimos la sala o entramos en la de un amigo: se puede jugar. */
        val rolDecidido: Boolean = false,
        /** conectando, buscando, listo, conectado o error. */
        val fase: String? = null,
        val error: String? = null,
        val amigos: Int = 0,
        val ruta: String? = null,
        /** Sube cada vez que llega el progreso de un amigo, para volver a pintarlo. */
        val novedades: Int = 0,
    )

    private val estadoInterno = MutableStateFlow(Estado())
    val estado: StateFlow<Estado> = estadoInterno.asStateFlow()

    private val salasInterno = MutableStateFlow<List<Sala>>(emptyList())
    val salas: StateFlow<List<Sala>> = salasInterno.asStateFlow()

    @Volatile
    private var proceso: Process? = null

    @Volatile
    private var salasCargadas = false

    val tieneClave: Boolean
        get() = BuildConfig.POKEMULADOR_TS_KEY.isNotBlank()

    fun perfilDeSala(codigo: String) = "sala-$codigo"

    // -----------------------------------------------------------------------------------------
    // Salas guardadas

    @Synchronized
    fun cargar(context: Context) {
        if (salasCargadas) return
        salasCargadas = true
        val archivo = File(context.applicationContext.filesDir, ARCHIVO_SALAS)
        val lista =
            runCatching {
                val json = JSONArray(archivo.readText())
                (0 until json.length()).mapNotNull { i -> json.optJSONObject(i)?.let(::salaDeJson) }
            }.getOrDefault(emptyList())
        salasInterno.value = lista
    }

    /** Crea una sala nueva con un código al azar. */
    fun crearSala(
        context: Context,
        nombre: String,
        juego: Game,
        partidaPropia: Boolean,
    ): Sala {
        cargar(context)
        val usados = salasInterno.value.map { it.codigo }.toSet()
        var codigo = nuevoCodigo()
        while (codigo in usados) codigo = nuevoCodigo()
        return guardarSala(context, codigo, nombre, juego, partidaPropia)
    }

    /** Guarda la sala de un amigo (o la actualiza si ya la teníamos). */
    @Synchronized
    fun guardarSala(
        context: Context,
        codigo: String,
        nombre: String,
        juego: Game,
        partidaPropia: Boolean,
    ): Sala {
        cargar(context)
        val limpio = codigo.trim().uppercase()
        val anterior = salasInterno.value.firstOrNull { it.codigo == limpio }
        val sala =
            Sala(
                codigo = limpio,
                nombre = nombre.trim().ifBlank { juego.customName ?: juego.title },
                juegoId = juego.id,
                juegoUri = juego.fileUri,
                juegoTitulo = juego.customName ?: juego.title,
                sistema = juego.systemId,
                partidaPropia = partidaPropia,
                creada = anterior?.creada ?: System.currentTimeMillis(),
            )
        salasInterno.value = listOf(sala) + salasInterno.value.filter { it.codigo != limpio }
        escribirSalas(context)
        return sala
    }

    /** Borra la sala, lo que sabemos de los amigos y, si la tenía, su partida propia. */
    @Synchronized
    fun borrarSala(
        context: Context,
        sala: Sala,
    ) {
        val app = context.applicationContext
        if (estadoInterno.value.codigo == sala.codigo) salir(app)
        salasInterno.value = salasInterno.value.filter { it.codigo != sala.codigo }
        escribirSalas(app)
        ProgresoSala.carpetaSala(app, sala.codigo).deleteRecursively()
        DirectoriesManager(app).getPerfilDirectory(perfilDeSala(sala.codigo)).deleteRecursively()
    }

    /** Salas con partida propia de este juego (para importar una partida en ellas). */
    fun salasConPartidaPropia(
        context: Context,
        juego: Game,
    ): List<Sala> {
        cargar(context)
        return salasInterno.value.filter { it.partidaPropia && (it.juegoId == juego.id || it.juegoUri == juego.fileUri) }
    }

    private fun escribirSalas(context: Context) {
        val archivo = File(context.applicationContext.filesDir, ARCHIVO_SALAS)
        archivo.parentFile?.mkdirs()
        val json = JSONArray()
        salasInterno.value.forEach { json.put(salaAJson(it)) }
        val temporal = File(archivo.parentFile, archivo.name + ".tmp")
        temporal.writeText(json.toString())
        if (!temporal.renameTo(archivo)) {
            archivo.writeText(json.toString())
            temporal.delete()
        }
    }

    private fun salaAJson(sala: Sala) =
        JSONObject()
            .put("codigo", sala.codigo)
            .put("nombre", sala.nombre)
            .put("juegoId", sala.juegoId)
            .put("juegoUri", sala.juegoUri)
            .put("juegoTitulo", sala.juegoTitulo)
            .put("sistema", sala.sistema)
            .put("partidaPropia", sala.partidaPropia)
            .put("creada", sala.creada)

    private fun salaDeJson(json: JSONObject): Sala? {
        val codigo = json.optString("codigo").takeIf { it.length == 6 } ?: return null
        return Sala(
            codigo = codigo,
            nombre = json.optString("nombre"),
            juegoId = json.optInt("juegoId"),
            juegoUri = json.optString("juegoUri"),
            juegoTitulo = json.optString("juegoTitulo"),
            sistema = json.optString("sistema"),
            partidaPropia = json.optBoolean("partidaPropia"),
            creada = json.optLong("creada"),
        )
    }

    // -----------------------------------------------------------------------------------------
    // Entrar y salir

    @Synchronized
    fun entrar(
        context: Context,
        sala: Sala,
    ) {
        val app = context.applicationContext
        cerrarProceso()
        estadoInterno.value = Estado(codigo = sala.codigo, fase = "conectando")
        ProgresoSala.preparar(app, sala)
        // Hasta saber si abrimos la sala o entramos en la de un amigo, el núcleo no entra en ninguna.
        File(app.filesDir, ARCHIVO).delete()

        try {
            val exe = File(app.applicationInfo.nativeLibraryDir, "libenlace.so")
            val builder =
                ProcessBuilder(
                    exe.absolutePath,
                    "sala",
                    "--codigo",
                    sala.codigo,
                    "--puerto",
                    PUERTO.toString(),
                    "--id",
                    idJugador(app),
                    "--progreso",
                    ProgresoSala.archivoPropio(app, sala.codigo).absolutePath,
                    "--amigos",
                    ProgresoSala.carpetaAmigos(app, sala.codigo).absolutePath,
                )
            builder.environment()["POKEMULADOR_TS_KEY"] = BuildConfig.POKEMULADOR_TS_KEY
            builder.environment()["TMPDIR"] = app.cacheDir.absolutePath
            builder.environment()["HOME"] = app.filesDir.absolutePath
            builder.redirectErrorStream(true)
            val nuevo = builder.start()
            proceso = nuevo
            thread(name = "sala-online", isDaemon = true) { leer(app, sala, nuevo) }
            SalaService.iniciar(app, sala.codigo)
        } catch (e: Exception) {
            estadoInterno.value =
                Estado(sala.codigo, fase = "error", error = "No se pudo arrancar el enlace online: ${e.message}")
        }
    }

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

    /** Puerto del cable link de Game Boy (Gambatte), que el enlace lleva por la sala. */
    private const val PUERTO_CABLE_GB = 56400

    /**
     * Variables para los núcleos cuando estamos en una sala. Se llama desde el proceso del juego.
     * 3DS: Azahar entra en la sala (o la abre). Game Boy: el cable link de Gambatte va por la sala
     * (servidor si la tenemos abierta; si no, cliente de 127.0.0.1, que el enlace lleva al anfitrión).
     */
    fun variablesNucleo(
        context: Context,
        systemID: SystemID,
    ): List<CoreVariable> {
        if (systemID != SystemID.NINTENDO_3DS && systemID != SystemID.GB && systemID != SystemID.GBC) return emptyList()
        val json =
            try {
                JSONObject(File(context.filesDir, ARCHIVO).readText())
            } catch (e: Exception) {
                return emptyList()
            }
        if (systemID == SystemID.GB || systemID == SystemID.GBC) {
            val modo = if (json.optBoolean("anfitrion")) "Network Server" else "Network Client"
            return listOf(
                CoreVariable("gambatte_gb_link_mode", modo),
                CoreVariable("gambatte_gb_link_network_port", PUERTO_CABLE_GB.toString()),
            ) +
                "127000000001".mapIndexed { i, cifra ->
                    CoreVariable("gambatte_gb_link_network_server_ip_${i + 1}", cifra.toString())
                }
        }
        return listOf(
            CoreVariable("citra_pkm_room_mode", if (json.optBoolean("anfitrion")) "host" else "join"),
            CoreVariable("citra_pkm_room_port", PUERTO.toString()),
            CoreVariable("citra_pkm_room_password", json.optString("codigo")),
            CoreVariable("citra_pkm_room_nickname", json.optString("nombre", "Jugador")),
        )
    }

    private fun escribirSalaActiva(
        context: Context,
        codigo: String,
        anfitrion: Boolean,
    ) {
        File(context.filesDir, ARCHIVO).writeText(
            JSONObject()
                .put("codigo", codigo)
                .put("anfitrion", anfitrion)
                .put("nombre", idJugador(context))
                .toString(),
        )
    }

    private fun leer(
        context: Context,
        sala: Sala,
        p: Process,
    ) {
        try {
            p.inputStream.bufferedReader().useLines { lineas -> lineas.forEach { procesar(context, sala, p, it) } }
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

    private fun procesar(
        context: Context,
        sala: Sala,
        p: Process,
        linea: String,
    ) {
        val json =
            try {
                JSONObject(linea)
            } catch (e: Exception) {
                return
            }
        if (proceso !== p) return
        val actual = estadoInterno.value
        estadoInterno.value =
            when (val fase = json.optString("estado")) {
                "rol" -> {
                    val anfitrion = json.optString("rol") == "host"
                    escribirSalaActiva(context, sala.codigo, anfitrion)
                    actual.copy(anfitrion = anfitrion, rolDecidido = true)
                }
                "amigos" -> actual.copy(amigos = json.optInt("n"))
                "ruta" -> actual.copy(ruta = json.optString("tipo"))
                "progreso" -> actual.copy(novedades = actual.novedades + 1)
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
            // Le damos un momento para despedirse de los amigos antes de cerrarlo del todo.
            thread(isDaemon = true) {
                if (!runCatching { p.waitFor(2, java.util.concurrent.TimeUnit.SECONDS) }.getOrDefault(false)) {
                    p.destroy()
                }
            }
        }
    }

    // -----------------------------------------------------------------------------------------
    // Jugador

    private fun nuevoCodigo(): String {
        val random = SecureRandom()
        return (1..6).map { LETRAS[random.nextInt(LETRAS.length)] }.joinToString("")
    }

    /** Identificador en las salas: tiene que ser distinto en cada móvil, así que lo generamos una vez. */
    fun idJugador(context: Context): String {
        val prefs = context.getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE)
        prefs.getString("nombre", null)?.let { return it }
        val random = SecureRandom()
        val nombre = "Jugador-" + (1..4).map { LETRAS[random.nextInt(LETRAS.length)] }.joinToString("")
        prefs.edit().putString("nombre", nombre).commit()
        return nombre
    }

    /** Nombre que ven los amigos (se puede cambiar). */
    fun nombreVisible(context: Context): String {
        val prefs = context.getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE)
        return prefs.getString("nombre_visible", null)?.takeIf { it.isNotBlank() } ?: idJugador(context)
    }

    fun cambiarNombre(
        context: Context,
        nombre: String,
    ) {
        val app = context.applicationContext
        app.getSharedPreferences(PREFERENCIAS, Context.MODE_PRIVATE)
            .edit()
            .putString("nombre_visible", nombre.trim().take(24))
            .commit()
        // Si estamos en una sala, que los amigos vean el nombre nuevo.
        val codigo = estadoInterno.value.codigo ?: return
        salasInterno.value.firstOrNull { it.codigo == codigo }?.let { ProgresoSala.preparar(app, it) }
    }
}
