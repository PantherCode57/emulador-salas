// Pokemulador: scripts de Lua en el núcleo libretro de mGBA (para la partida compartida de GBA-PK).
//
// Si al abrir un juego de GBA existe <carpeta de sistema>/pokemulador/multijugador.lua, se carga
// como lo haría la ventana de scripts de mGBA: con emu, console, socket y callbacks ("frame",
// "keysRead", "reset", "shutdown"). Pokemulador solo escribe ese archivo al jugar desde una sala;
// si no está, el núcleo funciona exactamente igual que siempre.
//
// Este archivo se incluye dentro de libretro.c (lo hace nucleos/mgba/aplicar.py), así que usa
// directamente sus variables estáticas: core, logger y logCallback.

#ifdef ENABLE_SCRIPTING
#include <mgba/core/scripting.h>
#include <mgba/script/context.h>

static struct mScriptContext _pkmScripts;
static bool _pkmScriptsOn = false;

static void _pkmFrame(void* context) {
	UNUSED(context);
	if (_pkmScriptsOn) {
		mScriptContextTriggerCallback(&_pkmScripts, "frame");
	}
}

static void _pkmKeysRead(void* context) {
	UNUSED(context);
	if (_pkmScriptsOn) {
		mScriptContextTriggerCallback(&_pkmScripts, "keysRead");
	}
}

// GBA-PK escribe su estado en "buffers" de texto de la consola de mGBA. Aquí no hay consola,
// así que son buffers que no guardan nada.
static void _pkmBufferInit(struct mScriptTextBuffer* buffer, const char* name) {
	UNUSED(buffer);
	UNUSED(name);
}

static void _pkmBufferDeinit(struct mScriptTextBuffer* buffer) {
	free(buffer);
}

static void _pkmBufferSetName(struct mScriptTextBuffer* buffer, const char* text) {
	UNUSED(buffer);
	UNUSED(text);
}

static uint32_t _pkmBufferZero(const struct mScriptTextBuffer* buffer) {
	UNUSED(buffer);
	return 0;
}

static uint32_t _pkmBufferCols(const struct mScriptTextBuffer* buffer) {
	UNUSED(buffer);
	return 80;
}

static uint32_t _pkmBufferRows(const struct mScriptTextBuffer* buffer) {
	UNUSED(buffer);
	return 24;
}

static void _pkmBufferPrint(struct mScriptTextBuffer* buffer, const char* text) {
	UNUSED(buffer);
	UNUSED(text);
}

static void _pkmBufferClear(struct mScriptTextBuffer* buffer) {
	UNUSED(buffer);
}

static void _pkmBufferSetSize(struct mScriptTextBuffer* buffer, uint32_t cols, uint32_t rows) {
	UNUSED(buffer);
	UNUSED(cols);
	UNUSED(rows);
}

static void _pkmBufferMoveCursor(struct mScriptTextBuffer* buffer, uint32_t x, uint32_t y) {
	UNUSED(buffer);
	UNUSED(x);
	UNUSED(y);
}

static void _pkmBufferAdvance(struct mScriptTextBuffer* buffer, int32_t adv) {
	UNUSED(buffer);
	UNUSED(adv);
}

static struct mScriptTextBuffer* _pkmBufferFactory(void* context) {
	UNUSED(context);
	struct mScriptTextBuffer* buffer = calloc(1, sizeof(*buffer));
	buffer->init = _pkmBufferInit;
	buffer->deinit = _pkmBufferDeinit;
	buffer->setName = _pkmBufferSetName;
	buffer->getX = _pkmBufferZero;
	buffer->getY = _pkmBufferZero;
	buffer->cols = _pkmBufferCols;
	buffer->rows = _pkmBufferRows;
	buffer->print = _pkmBufferPrint;
	buffer->clear = _pkmBufferClear;
	buffer->setSize = _pkmBufferSetSize;
	buffer->moveCursor = _pkmBufferMoveCursor;
	buffer->advance = _pkmBufferAdvance;
	return buffer;
}

static void _pkmBuscarMotor(const char* clave, void* valor, void* destino) {
	UNUSED(clave);
	*(struct mScriptEngineContext**) destino = valor;
}

static void _pkmScriptsStart(const char* sysDir) {
	if (_pkmScriptsOn || !sysDir || !core) {
		return;
	}
#ifdef M_CORE_GBA
	if (core->platform(core) != mPLATFORM_GBA) {
		return;
	}
#endif
	char path[PATH_MAX];
	snprintf(path, sizeof(path), "%s%spokemulador%smultijugador.lua", sysDir, PATH_SEP, PATH_SEP);
	struct VFile* vf = VFileOpen(path, O_RDONLY);
	if (!vf) {
		return;
	}
	vf->close(vf);

	mScriptContextInit(&_pkmScripts);
	mScriptContextAttachStdlib(&_pkmScripts);
	mScriptContextAttachSocket(&_pkmScripts);
	mScriptContextRegisterEngines(&_pkmScripts);
	mScriptContextAttachLogger(&_pkmScripts, &logger);
	mScriptContextSetTextBufferFactory(&_pkmScripts, _pkmBufferFactory, NULL);
	mScriptContextAttachCore(&_pkmScripts, core);

	struct mCoreCallbacks callbacks = {
		.videoFrameEnded = _pkmFrame,
		.keysRead = _pkmKeysRead,
		.context = NULL,
	};
	core->addCoreCallbacks(core, &callbacks);
	_pkmScriptsOn = true;

	// Como la ventana de scripts de mGBA: el motor de Lua carga el archivo y luego lo ejecuta
	// (mScriptContextLoadFile solo lo carga).
	struct mScriptEngineContext* motor = NULL;
	HashTableEnumerate(&_pkmScripts.engines, _pkmBuscarMotor, &motor);
	bool cargado = false;
	const char* error = "no hay motor de Lua";
	vf = VFileOpen(path, O_RDONLY);
	if (motor && vf) {
		cargado = motor->load(motor, path, vf) && motor->run(motor);
		error = cargado ? "" : motor->getError(motor);
	}
	if (vf) {
		vf->close(vf);
	}
	if (logCallback) {
		logCallback(cargado ? RETRO_LOG_INFO : RETRO_LOG_ERROR, "Pokemulador: script de partida compartida %s: %s %s\n",
		            cargado ? "en marcha" : "con error", path, error ? error : "");
	}
}

static void _pkmScriptsReset(void) {
	if (_pkmScriptsOn) {
		mScriptContextTriggerCallback(&_pkmScripts, "reset");
	}
}

static void _pkmScriptsStop(void) {
	if (!_pkmScriptsOn) {
		return;
	}
	mScriptContextTriggerCallback(&_pkmScripts, "shutdown");
	_pkmScriptsOn = false;
	mScriptContextDetachCore(&_pkmScripts);
	mScriptContextDeinit(&_pkmScripts);
}
#else
static void _pkmScriptsStart(const char* sysDir) {
	UNUSED(sysDir);
}

static void _pkmScriptsReset(void) {
}

static void _pkmScriptsStop(void) {
}
#endif
