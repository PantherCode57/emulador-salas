package com.swordfish.lemuroid.app.shared.game

import android.content.Context
import android.content.SharedPreferences
import android.view.KeyEvent
import android.view.MotionEvent
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.ui.unit.Density
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.swordfish.lemuroid.app.mobile.feature.game.GameService
import com.swordfish.lemuroid.app.mobile.feature.online.SalaJuego
import com.swordfish.lemuroid.app.mobile.feature.settings.SettingsManager
import com.swordfish.lemuroid.app.shared.game.viewmodel.GameViewModelInput
import com.swordfish.lemuroid.app.shared.game.viewmodel.GameViewModelRetroGameView
import com.swordfish.lemuroid.app.shared.game.viewmodel.GameViewModelSaves
import com.swordfish.lemuroid.app.shared.game.viewmodel.GameViewModelSideEffects
import com.swordfish.lemuroid.app.shared.game.viewmodel.GameViewModelTilt
import com.swordfish.lemuroid.app.shared.game.viewmodel.GameViewModelTouchControls
import com.swordfish.lemuroid.app.shared.input.InputDeviceManager
import com.swordfish.lemuroid.app.shared.rumble.RumbleManager
import com.swordfish.lemuroid.app.shared.settings.ControllerConfigsManager
import com.swordfish.lemuroid.app.shared.settings.HapticFeedbackMode
import com.swordfish.lemuroid.common.graphics.takeScreenshot
import com.swordfish.lemuroid.common.longAnimationDuration
import com.swordfish.lemuroid.lib.controller.ControllerConfig
import com.swordfish.lemuroid.lib.core.CoreVariablesManager
import com.swordfish.lemuroid.lib.game.GameLoader
import com.swordfish.lemuroid.lib.library.GameSystem
import com.swordfish.lemuroid.lib.library.SystemCoreConfig
import com.swordfish.lemuroid.lib.library.db.entity.Game
import com.swordfish.lemuroid.lib.saves.SavesManager
import com.swordfish.lemuroid.lib.saves.StatesManager
import com.swordfish.lemuroid.lib.saves.StatesPreviewManager
import com.swordfish.libretrodroid.GLRetroView
import com.swordfish.touchinput.radial.sensors.TiltConfiguration
import com.swordfish.touchinput.radial.settings.TouchControllerSettingsManager
import gg.padkit.inputevents.InputEvent
import gg.padkit.inputstate.InputState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import timber.log.Timber
import kotlin.concurrent.thread

class BaseGameScreenViewModel(
    private val appContext: Context,
    game: Game,
    settingsManager: SettingsManager,
    inputDeviceManager: InputDeviceManager,
    controllerConfigsManager: ControllerConfigsManager,
    system: GameSystem,
    systemCoreConfig: SystemCoreConfig,
    sharedPreferences: SharedPreferences,
    savesManager: SavesManager,
    statesManager: StatesManager,
    statesPreviewManager: StatesPreviewManager,
    coreVariablesManager: CoreVariablesManager,
    rumbleManager: RumbleManager,
) : ViewModel(), DefaultLifecycleObserver {
    class Factory(
        private val appContext: Context,
        private val game: Game,
        private val settingsManager: SettingsManager,
        private val inputDeviceManager: InputDeviceManager,
        private val controllerConfigsManager: ControllerConfigsManager,
        private val system: GameSystem,
        private val systemCoreConfig: SystemCoreConfig,
        private val sharedPreferences: SharedPreferences,
        private val savesManager: SavesManager,
        private val statesManager: StatesManager,
        private val statesPreviewManager: StatesPreviewManager,
        private val coreVariablesManager: CoreVariablesManager,
        private val rumbleManager: RumbleManager,
    ) : ViewModelProvider.Factory {
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
            return BaseGameScreenViewModel(
                appContext,
                game,
                settingsManager,
                inputDeviceManager,
                controllerConfigsManager,
                system,
                systemCoreConfig,
                sharedPreferences,
                savesManager,
                statesManager,
                statesPreviewManager,
                coreVariablesManager,
                rumbleManager,
            ) as T
        }
    }

    private val sideEffects = GameViewModelSideEffects(viewModelScope)
    val retroGameView =
        GameViewModelRetroGameView(
            appContext,
            system,
            systemCoreConfig,
            settingsManager,
            coreVariablesManager,
            sideEffects,
            rumbleManager,
            viewModelScope,
        )
    private val tilt = GameViewModelTilt(appContext, settingsManager)
    private val inputs =
        GameViewModelInput(
            appContext,
            system,
            systemCoreConfig,
            inputDeviceManager,
            controllerConfigsManager,
            retroGameView,
            tilt,
            sideEffects,
            viewModelScope,
        )
    private val touchControls =
        GameViewModelTouchControls(
            settingsManager,
            TouchControllerSettingsManager(sharedPreferences),
            retroGameView,
            inputs,
            tilt,
            sideEffects,
            viewModelScope,
        )
    private val saves =
        GameViewModelSaves(
            appContext,
            system,
            game,
            systemCoreConfig,
            retroGameView,
            settingsManager,
            savesManager,
            statesManager,
            statesPreviewManager,
            sideEffects,
        )

    val loadingState = MutableStateFlow(false)

    private inline fun withLoading(block: () -> Unit) {
        loadingState.value = true
        try {
            block()
        } finally {
            // Pokemulador: sin el «finally», salir antes de tiempo dejaba el juego «cargando» para
            // siempre: ni menú, ni guardar, ni salir.
            loadingState.value = false
        }
    }

    fun getGameState(): Flow<GameViewModelRetroGameView.GameState> {
        return retroGameView.getGameState()
    }

    fun getSideEffects(): Flow<GameViewModelSideEffects.UiEffect> {
        return sideEffects.getUiEffects()
    }

    fun getTiltConfiguration(): Flow<TiltConfiguration> {
        return tilt.getTiltConfiguration()
    }

    fun getSimulatedTiltEvents(): Flow<InputState> {
        return tilt.getSimulatedTiltEvents()
    }

    fun getTouchControlsSettings(
        density: Density,
        insets: WindowInsets,
    ): Flow<TouchControllerSettingsManager.Settings?> {
        return touchControls.getTouchControlsSettings(density, insets)
    }

    fun getTouchHapticFeedbackMode(): Flow<HapticFeedbackMode> {
        return touchControls.getTouchHapticFeedbackMode()
    }

    fun createRetroView(
        context: Context,
        lifecycle: LifecycleOwner,
    ): GLRetroView {
        val (gameData, result) = retroGameView.createRetroView(context, lifecycle)
        viewModelScope.launch {
            gameData.quickSaveData?.let {
                saves.restoreAutoSaveAsync(it)
            }
        }
        return result
    }

    suspend fun loadGame(
        applicationContext: Context,
        game: Game,
        systemCoreConfig: SystemCoreConfig,
        gameLoader: GameLoader,
        requestLoadSave: Boolean,
    ) {
        Timber.i("Calling load game: $game")
        retroGameView.initialize(applicationContext, game, systemCoreConfig, gameLoader, requestLoadSave)
    }

    fun showEditControls(show: Boolean) {
        touchControls.showEditControls(show)
    }

    fun isEditControlShown(): Flow<Boolean> {
        return touchControls.isEditControlsShown()
    }

    fun updateTouchControllerSettings(touchControllerSettings: TouchControllerSettingsManager.Settings) {
        touchControls.updateTouchControllerSettings(touchControllerSettings)
    }

    fun resetTouchControls() {
        touchControls.resetTouchControls()
    }

    fun onScreenOrientationChanged(orientation: TouchControllerSettingsManager.Orientation) {
        touchControls.updateScreenOrientation(orientation)
    }

    fun isTouchControllerVisible(): Flow<Boolean> {
        return touchControls.isTouchControllerVisible()
    }

    fun getTouchControllerConfig(): Flow<ControllerConfig> {
        return touchControls.getTouchControllerConfig()
    }

    fun changeTiltConfiguration(tiltConfig: TiltConfiguration) {
        tilt.changeTiltConfiguration(tiltConfig)
    }

    fun isMenuPressed(): Flow<Boolean> {
        return touchControls.isMenuPressed()
    }

    suspend fun saveSlot(index: Int) {
        if (loadingState.value) return
        withLoading {
            saves.saveSlot(index)
        }
    }

    suspend fun loadSlot(index: Int) {
        if (loadingState.value) return
        withLoading {
            saves.loadSlot(index)
        }
    }

    fun saveQuickSave() {
        Timber.d("Saving quick save")
        if (loadingState.value) return
        withLoading {
            saves.saveQuickSave()
        }
    }

    fun loadQuickSave() {
        Timber.d("Loading quick save")
        if (loadingState.value) return
        withLoading {
            saves.loadQuickSave()
        }
    }

    fun toggleFastForward() {
        Timber.d("Loading quick save")
        retroGameView.retroGameView?.apply {
            frameSpeed = if (frameSpeed == 1) 2 else 1
        }
    }

    suspend fun reset() =
        withLoading {
            try {
                delay(appContext.longAnimationDuration().toLong())
                retroGameView.retroGameViewFlow().reset()
            } catch (e: Throwable) {
                Timber.e(e, "Error in reset")
            }
        }

    /** Abre el menú del juego (Pokemulador: también con el botón «atrás» del móvil). */
    fun showMenu() {
        if (loadingState.value) return
        sideEffects.showMenu(tilt, inputs)
    }

    fun requestFinish() {
        if (terminando) return
        terminando = true
        viewModelScope.launch {
            loadingState.value = true
            // Pokemulador: guardar tiene un plazo. Antes, si el juego estaba colgado, esto esperaba
            // para siempre (en el hilo principal) y no se podía ni salir.
            enPlazo(PLAZO_GUARDAR_MS) { saves.writeSaveSnapshot(capturar()) }
            enPlazo(PLAZO_SALA_MS) { guardarProgresoSala() }
            sideEffects.requestSuccessfulFinish()
        }
    }

    @Volatile
    private var terminando = false

    @Volatile
    private var enPantalla = false

    private val abierto = System.currentTimeMillis()

    // Lo que habla con el núcleo puede colgarse y no se puede cancelar: va aparte y, si no acaba a
    // tiempo, se deja.
    private val conElNucleo = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    private suspend fun <T> enPlazo(
        ms: Long,
        bloque: suspend () -> T?,
    ): T? {
        val trabajo =
            conElNucleo.async {
                runCatching { bloque() }.onFailure { Timber.e(it, "Fallo al guardar") }.getOrNull()
            }
        return withTimeoutOrNull(ms) { trabajo.await() }
    }

    private suspend fun capturar(): GameViewModelSaves.SaveSnapshot? =
        enPlazo(PLAZO_CAPTURAR_MS) { saves.captureSaveSnapshot(true) }
            // El hilo del juego no contesta: al menos, la partida (está en la memoria del núcleo).
            ?: enPlazo(PLAZO_CAPTURAR_MS) { saves.capturarSoloPartida() }

    /** Pokemulador: el juego ha fallado. Intenta salvar la partida, sin esperar más de la cuenta. */
    fun guardarDeUrgencia() {
        if (terminando || System.currentTimeMillis() - abierto < JUGADO_MINIMO_MS) return
        val hilo =
            thread(name = "guardar-de-urgencia", isDaemon = true) {
                runCatching { runBlocking { saves.writeSaveSnapshot(saves.capturarSoloPartida()) } }
            }
        runCatching { hilo.join(PLAZO_URGENCIA_MS) }
    }

    /**
     * Pokemulador: cuando guardas dentro del juego, la partida se escribe en el móvil a los pocos
     * segundos. Antes solo se escribía al salir del juego o al cambiar de app: si se cerraba de golpe,
     * se perdía lo guardado desde entonces.
     */
    private fun vigilarPartida() {
        viewModelScope.launch(Dispatchers.IO) {
            retroGameView.waitRetroGameViewInitialized()
            delay(VIGILAR_DESDE_MS)
            var guardada = enPlazo(PLAZO_CAPTURAR_MS) { saves.leerPartida() }?.contentHashCode()
            var anterior = guardada
            while (isActive) {
                delay(VIGILAR_CADA_MS)
                if (!enPantalla || loadingState.value || terminando) continue
                val ahora = enPlazo(PLAZO_CAPTURAR_MS) { saves.leerPartida() }?.takeIf { it.isNotEmpty() }?.contentHashCode()
                // Se espera a que deje de cambiar, para no pillar al juego a medio guardar.
                if (ahora != null && ahora == anterior && ahora != guardada) {
                    withContext(NonCancellable) {
                        enPlazo(PLAZO_GUARDAR_MS) { saves.writeSaveSnapshot(saves.captureSaveSnapshot(true)) }
                    }
                    Timber.i("Partida guardada en el móvil")
                    guardada = ahora
                }
                if (ahora != null) anterior = ahora
            }
        }
    }

    override fun onResume(owner: LifecycleOwner) {
        super.onResume(owner)
        enPantalla = true
    }

    override fun onPause(owner: LifecycleOwner) {
        enPantalla = false
        super.onPause(owner)
    }

    companion object {
        private const val PLAZO_CAPTURAR_MS = 2500L
        private const val PLAZO_GUARDAR_MS = 6000L
        private const val PLAZO_SALA_MS = 1500L
        private const val PLAZO_URGENCIA_MS = 1500L
        private const val JUGADO_MINIMO_MS = 15_000L
        private const val VIGILAR_DESDE_MS = 10_000L
        private const val VIGILAR_CADA_MS = 4_000L
    }

    fun requestBackgroundSave() {
        if (loadingState.value) return
        GameService.schedule {
            val snapshot = saves.captureSaveSnapshot(false)
            saves.writeSaveSnapshot(snapshot)
            guardarProgresoSala()
        }
    }

    /** Pokemulador: si se juega desde una sala, el tiempo y la captura que ven los amigos. */
    var salaJuego: SalaJuego? = null

    private var progresoPeriodico: Job? = null

    private suspend fun guardarProgresoSala() {
        val sala = salaJuego ?: return
        val captura =
            runCatching {
                retroGameView.retroGameView?.takeScreenshot(SalaJuego.LADO_CAPTURA, 2)
            }.getOrNull()
        sala.guardar(captura)
    }

    fun handleVirtualInputEvent(events: List<InputEvent>) {
        touchControls.handleVirtualInputEvent(events)
    }

    private var partidaVigilada = false

    override fun onCreate(owner: LifecycleOwner) {
        super.onCreate(owner)
        if (!partidaVigilada) {
            partidaVigilada = true
            vigilarPartida()
        }
        if (salaJuego != null && progresoPeriodico == null) {
            // Si los amigos están en la sala a la vez, que vean cómo vamos mientras jugamos.
            progresoPeriodico =
                viewModelScope.launch {
                    while (true) {
                        delay(SalaJuego.CADA_CUANTO_MS)
                        if (!loadingState.value) guardarProgresoSala()
                    }
                }
        }

        owner.lifecycle.addObserver(tilt)
        owner.lifecycle.addObserver(inputs)
        owner.lifecycle.addObserver(retroGameView)
        owner.lifecycle.addObserver(touchControls)
    }

    fun sendKeyEvent(
        keyCode: Int,
        event: KeyEvent,
    ): Boolean {
        return inputs.sendKeyEvent(keyCode, event)
    }

    fun sendMotionEvent(event: MotionEvent): Boolean {
        return inputs.sendMotionEvent(event)
    }
}
