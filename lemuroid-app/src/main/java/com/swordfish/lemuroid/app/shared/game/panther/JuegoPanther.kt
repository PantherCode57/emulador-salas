package com.swordfish.lemuroid.app.shared.game.panther

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.os.Build
import android.os.Bundle
import android.os.Process
import android.system.Os
import android.view.Gravity
import android.view.WindowManager
import android.widget.ProgressBar
import android.widget.RelativeLayout
import androidx.core.content.ContextCompat
import com.pokemulador.panther.Emulador
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.feature.online.SalaJuego
import com.swordfish.lemuroid.app.mobile.feature.online.SalaOnline
import com.swordfish.lemuroid.app.mobile.feature.settings.SettingsManager
import com.swordfish.lemuroid.app.shared.game.BaseGameActivity
import com.swordfish.lemuroid.app.shared.game.JuegoAbierto
import com.swordfish.lemuroid.app.shared.settings.HapticFeedbackMode
import com.swordfish.lemuroid.common.kotlin.readBytesAtomicOrNull
import com.swordfish.lemuroid.common.kotlin.writeBytesAtomic
import com.swordfish.lemuroid.lib.core.CoresSelection
import com.swordfish.lemuroid.lib.library.CoreID
import com.swordfish.lemuroid.lib.library.GameSystem
import com.swordfish.lemuroid.lib.library.LemuroidLibrary
import com.swordfish.lemuroid.lib.library.SystemID
import com.swordfish.lemuroid.lib.library.db.RetrogradeDatabase
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.preferences.SharedPreferencesHelper
import com.swordfish.lemuroid.lib.saves.CopiasPartida
import com.swordfish.lemuroid.lib.saves.FormatoPartida
import com.swordfish.lemuroid.lib.saves.SavesCoherencyEngine
import com.swordfish.lemuroid.lib.saves.SavesManager
import com.swordfish.lemuroid.lib.saves.StatesManager
import com.swordfish.lemuroid.lib.storage.DirectoriesManager
import com.swordfish.lemuroid.lib.storage.RomFiles
import dagger.android.AndroidInjection
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeoutOrNull
import org.libsdl.app.SDLActivity
import timber.log.Timber
import java.io.File
import java.util.concurrent.CompletableFuture
import javax.inject.Inject
import kotlin.concurrent.thread
import kotlin.math.abs

/**
 * Un juego de GB, GBC o GBA con el emulador Panther (el de la consola, en C, con SDL), con el aspecto
 * de Pokemulador. Va en su propio proceso (":panther"), que se cierra al salir: así cada juego empieza
 * de cero, y solo puede haber uno abierto (GameProcessLock).
 *
 * Usa las partidas de Pokemulador (la de siempre o la de la sala) y sus salas: el script de GBA-PK que
 * deja PartidaCompartidaGba y el cable link de Game Boy.
 */
class JuegoPanther : SDLActivity() {
    @Inject
    lateinit var lemuroidLibrary: LemuroidLibrary

    @Inject
    lateinit var retrogradeDb: RetrogradeDatabase

    @Inject
    lateinit var coresSelection: CoresSelection

    @Inject
    lateinit var statesManager: StatesManager

    @Inject
    lateinit var savesManager: SavesManager

    @Inject
    lateinit var savesCoherencyEngine: SavesCoherencyEngine

    @Inject
    lateinit var directoriesManager: DirectoriesManager

    @Inject
    lateinit var settingsManager: SettingsManager

    private lateinit var game: Game
    private val argumentos = CompletableFuture<List<String>>()
    private val aparte = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val abierto = System.currentTimeMillis()
    private var espera: ProgressBar? = null

    @Volatile
    private var error: String? = null

    @Volatile
    private var recorte = 0

    @Volatile
    private var enPantalla = false

    private var sala: SalaJuego? = null
    private var captura: Bitmap? = null
    private var capturas: Job? = null

    /** La app pide cerrar este juego (guardando) para poder abrir otro. */
    private val cerrarJuego =
        object : BroadcastReceiver() {
            override fun onReceive(
                context: Context?,
                intent: Intent?,
            ) {
                // Parado no atiende órdenes; al cerrarse la actividad, guarda igual.
                if (enPantalla && argumentos.isDone) Emulador.orden(Emulador.SALIR) else finish()
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        game = intent.getSerializableExtra(EXTRA_GAME) as Game
        // La partida que toca (la de siempre o la de una sala), antes que nada.
        DirectoriesManager.perfil = intent.getStringExtra(EXTRA_PERFIL)
        AndroidInjection.inject(this)
        super.onCreate(savedInstanceState)
        Thread.setDefaultUncaughtExceptionHandler { _, e -> trasUnFallo(e) }
        sala = intent.getStringExtra(EXTRA_SALA)?.let { SalaJuego(applicationContext, it, game.displayName()) }
        ponerEspera()
        pedir60Hz()
        mirarRecorte()
        ContextCompat.registerReceiver(
            this,
            cerrarJuego,
            IntentFilter(JuegoAbierto.ACCION_CERRAR),
            ContextCompat.RECEIVER_NOT_EXPORTED,
        )
        thread(name = "preparar-juego") {
            try {
                argumentos.complete(runBlocking { preparar() })
            } catch (e: Throwable) {
                Timber.e(e, "No se pudo preparar el juego")
                error = getString(R.string.emudex_panther_no_abre)
                argumentos.complete(emptyList())
            }
            runOnUiThread { quitarEspera() }
        }
    }

    override fun getLibraries(): Array<String> = arrayOf("SDL2", "panther")

    /** Lo llama el hilo del juego: espera a que esté todo listo. */
    override fun getArguments(): Array<String> {
        val lista = argumentos.get()
        return if (lista.isEmpty()) emptyArray() else (lista + listOf("--recorte", recorte.toString())).toTypedArray()
    }

    // ---------------------------------------------------------------------------------------------
    // Preparar el juego

    private suspend fun preparar(): List<String> {
        val base = Panther.base(game)
        val casa = File(filesDir, "panther").apply { mkdirs() }
        prepararLetras(casa)

        val seguir = settingsManager.autoSave()
        val rom = prepararRom(base)
        val saves = directoriesManager.getSavesDirectory()
        prepararPartida(File(saves, "$base.srm"))
        val estados = File(directoriesManager.getStatesDirectory(), Panther.CARPETA_ESTADOS).apply { mkdirs() }
        prepararSeguir(File(estados, "$base.estado-auto"), seguir)
        val fotos = File(cacheDir, "panther-capturas").apply { mkdirs() }
        fotos.listFiles()?.forEach { it.delete() }
        escribirAjustes(casa)

        val lista =
            mutableListOf(
                "--casa", casa.absolutePath,
                "--guardados", saves.absolutePath,
                "--estados", estados.absolutePath,
                "--sistema", directoriesManager.getSystemDirectory().absolutePath,
                "--capturas", fotos.absolutePath,
            )
        cableDeLaSala(casa)?.let { lista += listOf("--opciones", it.absolutePath) }
        if (!seguir) lista += "--sin-seguir"
        if (settingsManager.hapticFeedbackMode() == HapticFeedbackMode.NONE.name.lowercase()) lista += "--sin-vibrar"
        // El juego, lo último.
        lista += rom.absolutePath
        return lista
    }

    private fun prepararLetras(casa: File) {
        val instalada = runCatching { packageManager.getPackageInfo(packageName, 0).lastUpdateTime }.getOrDefault(0L)
        listOf("selawk.ttf", "selawksb.ttf").forEach { nombre ->
            val destino = File(casa, nombre)
            if (destino.length() > 0 && destino.lastModified() >= instalada) return@forEach
            assets.open(nombre).use { entrada -> destino.outputStream().use { entrada.copyTo(it) } }
        }
    }

    /**
     * El juego como archivo (Android no da la ruta de los de la carpeta elegida: se usa la copia que ya
     * hacía Pokemulador), con el nombre de la partida, que es de donde lo saca el emulador.
     */
    private fun prepararRom(base: String): File {
        val datos = retrogradeDb.dataFileDao().selectDataFilesForGame(game.id)
        val archivos = lemuroidLibrary.getGameFiles(game, datos, false)
        val rom = (archivos as RomFiles.Standard).files.first()
        check(rom.length() > 0) { "El juego está vacío: $rom" }
        val carpeta = File(cacheDir, "panther-juego").apply { mkdirs() }
        carpeta.listFiles()?.forEach { it.delete() }
        val enlace = File(carpeta, "$base.${Panther.extension(game)}")
        Os.symlink(rom.absolutePath, enlace.absolutePath)
        return enlace
    }

    /** Una copia de seguridad de la partida, y la partida como la espera Panther. */
    private suspend fun prepararPartida(partida: File) {
        val datos = partida.readBytesAtomicOrNull()
        if (datos == null) {
            // Puede estar donde la guardaba una versión vieja de la app.
            val nucleo = coresSelection.getCoreConfigForSystem(GameSystem.findById(game.systemId))
            savesManager.getSaveRAM(game, nucleo)?.takeIf { it.isNotEmpty() }?.let {
                partida.writeBytesAtomic(FormatoPartida.paraPanther(game.systemId, it))
            }
            return
        }
        runCatching { CopiasPartida.hacer(partida, siempre = true) }
            .onFailure { Timber.e(it, "No se pudo hacer la copia de la partida") }
        val preparada = FormatoPartida.paraPanther(game.systemId, datos)
        if (!preparada.contentEquals(datos)) partida.writeBytesAtomic(preparada)
    }

    /** Desde dónde se sigue. */
    private suspend fun prepararSeguir(
        auto: File,
        seguir: Boolean,
    ) {
        if (!intent.getBooleanExtra(EXTRA_LOAD_SAVE, true)) {
            // «Reiniciar»: desde el principio. Lo de antes se aparta, no se borra.
            if (auto.exists()) auto.renameTo(File(auto.path + ".antes"))
            return
        }
        if (!seguir || game.systemId != SystemID.GBA.dbname) return
        // En GBA, el emulador de antes también es mGBA: si lo último fue con él, se sigue desde ahí.
        val nucleo = coresSelection.getCoreConfigForSystem(GameSystem.findById(game.systemId)).coreID
        if (nucleo != CoreID.MGBA) return
        val deAntes = statesManager.getAutoSaveInfo(game, nucleo)
        if (!deAntes.exists || (auto.exists() && auto.lastModified() + TOLERANCIA_MS >= deAntes.date)) return
        if (savesCoherencyEngine.shouldDiscardAutoSaveState(game, nucleo)) return
        statesManager.getAutoSave(game, nucleo)?.let { auto.writeBytesAtomic(it.state) }
    }

    /** El cable link de Game Boy de la sala, como lo espera el emulador de la consola. */
    private fun cableDeLaSala(casa: File): File? {
        val opciones = SalaOnline.variablesNucleo(applicationContext, GameSystem.findById(game.systemId).id)
        if (opciones.isEmpty()) return null
        val archivo = File(casa, "cable.opt")
        archivo.writeText(opciones.joinToString("") { "${it.key} = \"${it.value}\"\n" })
        return archivo
    }

    /** Los ajustes del emulador (velocidad y Caramelos Raros): los Caramelos, los de Ajustes → Trucos. */
    private fun archivoAjustes(casa: File) = File(casa, ".config/panther/emulador.cfg")

    private fun leerAjuste(
        casa: File,
        clave: String,
    ): Int? =
        runCatching {
            archivoAjustes(casa).readLines()
                .firstOrNull { it.startsWith("$clave = ") }
                ?.substringAfter("= ")?.trim()?.toInt()
        }.getOrNull()

    private fun escribirAjustes(casa: File) {
        val rapido = leerAjuste(casa, "rapido") ?: RAPIDO
        val caramelos =
            SharedPreferencesHelper.getSharedPreferences(this)
                .getBoolean(getString(R.string.pref_key_caramelos_frlg), false)
        archivoAjustes(casa).apply {
            parentFile?.mkdirs()
            writeText("rapido = $rapido\ncaramelos = ${if (caramelos) 1 else 0}\n")
        }
    }

    /** Si los Caramelos se han puesto o quitado desde el menú del juego, que quede así. */
    private fun recogerAjustes() {
        val caramelos = leerAjuste(File(filesDir, "panther"), "caramelos") ?: return
        val preferencias = SharedPreferencesHelper.getSharedPreferences(this)
        val clave = getString(R.string.pref_key_caramelos_frlg)
        if (preferencias.getBoolean(clave, false) != (caramelos == 1)) {
            preferencias.edit().putBoolean(clave, caramelos == 1).commit()
        }
    }

    // ---------------------------------------------------------------------------------------------
    // La pantalla

    private fun ponerEspera() {
        val ruedita = ProgressBar(this)
        val sitio =
            RelativeLayout.LayoutParams(
                RelativeLayout.LayoutParams.WRAP_CONTENT,
                RelativeLayout.LayoutParams.WRAP_CONTENT,
            )
        sitio.addRule(RelativeLayout.CENTER_IN_PARENT)
        runCatching { mLayout?.addView(ruedita, sitio) ?: addContentView(ruedita, centrado()) }
        espera = ruedita
    }

    private fun centrado() =
        android.widget.FrameLayout.LayoutParams(
            android.widget.FrameLayout.LayoutParams.WRAP_CONTENT,
            android.widget.FrameLayout.LayoutParams.WRAP_CONTENT,
            Gravity.CENTER,
        )

    private fun quitarEspera() {
        espera?.let { (it.parent as? android.view.ViewGroup)?.removeView(it) }
        espera = null
    }

    /** Las GB y GBA van a 60 imágenes por segundo: si la pantalla puede ponerse a 60 Hz, mejor. */
    @Suppress("DEPRECATION")
    private fun pedir60Hz() {
        val pantalla = windowManager.defaultDisplay
        val actual = pantalla.mode
        val mejor =
            pantalla.supportedModes
                .filter { it.physicalWidth == actual.physicalWidth && it.physicalHeight == actual.physicalHeight }
                .minByOrNull { abs(it.refreshRate - 60f) } ?: return
        if (mejor.modeId != actual.modeId) {
            window.attributes = window.attributes.apply { preferredDisplayModeId = mejor.modeId }
        }
    }

    /** Lo que tapa la cámara: el emulador no pone botones debajo. */
    private fun mirarRecorte() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.P) return
        window.attributes =
            window.attributes.apply {
                layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES
            }
        window.decorView.setOnApplyWindowInsetsListener { vista, margenes ->
            margenes.displayCutout?.let {
                recorte = maxOf(maxOf(it.safeInsetTop, it.safeInsetBottom), maxOf(it.safeInsetLeft, it.safeInsetRight))
            }
            vista.onApplyWindowInsets(margenes)
        }
    }

    // ---------------------------------------------------------------------------------------------
    // Las salas: el tiempo jugado y la captura que ven los amigos

    override fun onResume() {
        super.onResume()
        enPantalla = true
        val enSala = sala ?: return
        enSala.reanudar()
        capturas =
            aparte.launch {
                var ultimo = System.currentTimeMillis()
                while (true) {
                    delay(CAPTURA_CADA_MS)
                    tomarCaptura()
                    if (System.currentTimeMillis() - ultimo >= SalaJuego.CADA_CUANTO_MS) {
                        ultimo = System.currentTimeMillis()
                        enSala.guardar(captura)
                    }
                }
            }
    }

    override fun onPause() {
        enPantalla = false
        capturas?.cancel()
        sala?.let { enSala ->
            enSala.pausar()
            aparte.launch { enSala.guardar(captura) }
        }
        super.onPause()
    }

    private suspend fun tomarCaptura() {
        if (!argumentos.isDone || Emulador.resultado() != -1) return
        val fotos = File(cacheDir, "panther-capturas")
        Emulador.orden(Emulador.CAPTURA)
        delay(ESPERA_CAPTURA_MS)
        val foto = fotos.listFiles { f -> f.name.endsWith(".png") }?.maxByOrNull { it.lastModified() } ?: return
        BitmapFactory.decodeFile(foto.path)?.let { captura = it }
        fotos.listFiles { f -> f.name.endsWith(".png") }?.forEach { it.delete() }
    }

    // ---------------------------------------------------------------------------------------------
    // Salir

    override fun finish() {
        ponerResultado()
        super.finish()
        @Suppress("DEPRECATION")
        overridePendingTransition(android.R.anim.fade_in, android.R.anim.fade_out)
    }

    private fun ponerResultado() {
        val fallo = error ?: getString(R.string.emudex_panther_no_abre).takeIf { noSeHaAbierto() }
        if (fallo != null) {
            setResult(
                BaseGameActivity.RESULT_ERROR,
                Intent().putExtra(BaseGameActivity.PLAY_GAME_RESULT_ERROR, fallo),
            )
            return
        }
        setResult(
            Activity.RESULT_OK,
            Intent()
                .putExtra(BaseGameActivity.PLAY_GAME_RESULT_SESSION_DURATION, System.currentTimeMillis() - abierto)
                .putExtra(BaseGameActivity.PLAY_GAME_RESULT_GAME, game)
                .putExtra(BaseGameActivity.PLAY_GAME_RESULT_LEANBACK, false),
        )
    }

    private fun noSeHaAbierto(): Boolean = runCatching { Emulador.resultado() > 0 }.getOrDefault(true)

    /** Un fallo de la parte de Java: se vuelve a la app y se cierra el proceso, pase lo que pase. */
    private fun trasUnFallo(e: Throwable) {
        Timber.e(e, "Fallo en el juego")
        acabarSeguro(PLAZO_TRAS_FALLO_MS)
        error = e.message ?: e.javaClass.simpleName
        runCatching { finish() }
    }

    override fun onDestroy() {
        // SDL espera aquí a que el juego guarde y salga, sin plazo: si está colgado, hay que cortar.
        acabarSeguro(PLAZO_SALIR_MS)
        runCatching { unregisterReceiver(cerrarJuego) }
        capturas?.cancel()
        super.onDestroy()
        runCatching { recogerAjustes() }
        sala?.let { runBlocking { withTimeoutOrNull(PLAZO_SALA_MS) { it.guardar(captura) } } }
        aparte.cancel()
        // El emulador ya ha guardado: se cierra el proceso (y queda libre para otro juego).
        Process.killProcess(Process.myPid())
    }

    private fun acabarSeguro(ms: Long) {
        thread(name = "fin-del-juego", isDaemon = true) {
            Thread.sleep(ms)
            Process.killProcess(Process.myPid())
        }
    }

    companion object {
        private const val EXTRA_GAME = "GAME"
        private const val EXTRA_LOAD_SAVE = "LOAD_SAVE"
        private const val EXTRA_PERFIL = "POKEMULADOR_PERFIL"
        private const val EXTRA_SALA = "POKEMULADOR_SALA"

        private const val RAPIDO = 3
        private const val TOLERANCIA_MS = 30_000L
        private const val CAPTURA_CADA_MS = 20_000L
        private const val ESPERA_CAPTURA_MS = 400L
        private const val PLAZO_SALIR_MS = 6_000L
        private const val PLAZO_TRAS_FALLO_MS = 2_000L
        private const val PLAZO_SALA_MS = 1_500L

        fun abrir(
            activity: Activity,
            game: Game,
            loadSave: Boolean,
            perfil: String?,
            sala: String?,
        ) {
            @Suppress("DEPRECATION")
            activity.startActivityForResult(
                Intent(activity, JuegoPanther::class.java)
                    .putExtra(EXTRA_GAME, game)
                    .putExtra(EXTRA_LOAD_SAVE, loadSave)
                    .putExtra(EXTRA_PERFIL, perfil)
                    .putExtra(EXTRA_SALA, sala),
                BaseGameActivity.REQUEST_PLAY_GAME,
            )
            @Suppress("DEPRECATION")
            activity.overridePendingTransition(android.R.anim.fade_in, android.R.anim.fade_out)
        }
    }
}
