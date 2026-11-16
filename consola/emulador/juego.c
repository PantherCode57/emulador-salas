/* El juego: GB, GBC y GBA con el código de mGBA 0.10.5 (MPL 2.0), que va dentro del programa.
 *
 * - La partida del juego (.srm) es la misma que usaba RetroArch: junto al juego, o en la carpeta que
 *   diga la app que lo abre. Así no se pierde nada al cambiar.
 * - R2: ir rápido (×3, o lo que se elija en el menú). SELECT + START: el menú de pausa.
 * - Al salir se guarda dónde ibas y, la próxima vez, sigues ahí (si nadie ha jugado entretanto a ese
 *   juego con otra partida).
 * - Los scripts de Lua de mGBA (las salas de GBA-PK y los Caramelos Raros) con --script, o el de
 *   <sistema>/pokemulador/multijugador.lua como hacía el núcleo de Pokemulador.
 */
#include "panther.h"

#include <mgba/core/blip_buf.h>
#include <mgba/core/core.h>
#include <mgba/core/log.h>
#include <mgba/core/serialize.h>
#include <mgba-util/vfs.h>
#ifdef ENABLE_SCRIPTING
#include <mgba-util/socket.h>
#include <mgba/core/scripting.h>
#include <mgba/script/context.h>
#endif

#include <fcntl.h>
#include <limits.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

/* Teclas de GBA (las de GB son las 8 primeras). */
enum { K_A, K_B, K_SELECT, K_START, K_DER, K_IZQ, K_ARRIBA, K_ABAJO, K_R, K_L };

static uint32_t teclas(uint32_t b) {
	uint32_t k = 0;
	if (b & BOT_A) k |= 1u << K_A;
	if (b & BOT_B) k |= 1u << K_B;
	if (b & BOT_SELECT) k |= 1u << K_SELECT;
	if (b & BOT_START) k |= 1u << K_START;
	if (b & BOT_DER) k |= 1u << K_DER;
	if (b & BOT_IZQ) k |= 1u << K_IZQ;
	if (b & BOT_ARRIBA) k |= 1u << K_ARRIBA;
	if (b & BOT_ABAJO) k |= 1u << K_ABAJO;
	if (b & BOT_R) k |= 1u << K_R;
	if (b & BOT_L) k |= 1u << K_L;
	/* SELECT y START a la vez abren el menú: el juego no los ve juntos. */
	if ((b & BOT_SELECT) && (b & BOT_START))
		k &= ~((1u << K_SELECT) | (1u << K_START));
	return k;
}

static void log_mgba(struct mLogger *l, int categoria, enum mLogLevel nivel, const char *fmt, va_list args) {
	(void)l;
	if (!(nivel & (mLOG_FATAL | mLOG_ERROR)))
		return;
	char buf[600];
	vsnprintf(buf, sizeof(buf), fmt, args);
	registro("mgba [%s] %s", mLogCategoryName(categoria), buf);
}
static struct mLogger registro_mgba = {.log = log_mgba};

/* ---- Scripts de Lua ---- */
#ifdef ENABLE_SCRIPTING
static struct mScriptContext guion;
static bool hay_guion;
static struct mCoreCallbacks avisos_guion;

static void guion_imagen(void *c) {
	(void)c;
	if (hay_guion)
		mScriptContextTriggerCallback(&guion, "frame");
}
static void guion_teclas(void *c) {
	(void)c;
	if (hay_guion)
		mScriptContextTriggerCallback(&guion, "keysRead");
}

/* GBA-PK escribe en «buffers» de la consola de scripts de mGBA. Aquí no hay consola: no guardan nada. */
static void b_init(struct mScriptTextBuffer *b, const char *n) { (void)b, (void)n; }
static void b_deinit(struct mScriptTextBuffer *b) { free(b); }
static void b_nombre(struct mScriptTextBuffer *b, const char *t) { (void)b, (void)t; }
static uint32_t b_cero(const struct mScriptTextBuffer *b) { (void)b; return 0; }
static uint32_t b_cols(const struct mScriptTextBuffer *b) { (void)b; return 80; }
static uint32_t b_filas(const struct mScriptTextBuffer *b) { (void)b; return 24; }
static void b_print(struct mScriptTextBuffer *b, const char *t) { (void)b, (void)t; }
static void b_borrar(struct mScriptTextBuffer *b) { (void)b; }
static void b_tam(struct mScriptTextBuffer *b, uint32_t c, uint32_t f) { (void)b, (void)c, (void)f; }
static void b_cursor(struct mScriptTextBuffer *b, uint32_t x, uint32_t y) { (void)b, (void)x, (void)y; }
static void b_avanzar(struct mScriptTextBuffer *b, int32_t a) { (void)b, (void)a; }
static struct mScriptTextBuffer *fabrica_buffers(void *c) {
	(void)c;
	struct mScriptTextBuffer *b = calloc(1, sizeof(*b));
	b->init = b_init, b->deinit = b_deinit, b->setName = b_nombre, b->getX = b_cero, b->getY = b_cero;
	b->cols = b_cols, b->rows = b_filas, b->print = b_print, b->clear = b_borrar, b->setSize = b_tam;
	b->moveCursor = b_cursor, b->advance = b_avanzar;
	return b;
}
static void buscar_motor(const char *clave, void *valor, void *destino) {
	(void)clave;
	*(struct mScriptEngineContext **)destino = valor;
}

static void guiones_empezar(struct mCore *core, const char **rutas, int n) {
	if (n <= 0)
		return;
	SocketSubsystemInit();
	mScriptContextInit(&guion);
	mScriptContextAttachStdlib(&guion);
	mScriptContextAttachSocket(&guion);
	mScriptContextRegisterEngines(&guion);
	mScriptContextAttachLogger(&guion, &registro_mgba);
	mScriptContextSetTextBufferFactory(&guion, fabrica_buffers, NULL);
	mScriptContextAttachCore(&guion, core);
	avisos_guion = (struct mCoreCallbacks){.videoFrameEnded = guion_imagen, .keysRead = guion_teclas};
	core->addCoreCallbacks(core, &avisos_guion);
	hay_guion = true;
	struct mScriptEngineContext *motor = NULL;
	HashTableEnumerate(&guion.engines, buscar_motor, &motor);
	for (int i = 0; i < n; i++) {
		struct VFile *vf = VFileOpen(rutas[i], O_RDONLY);
		bool bien = motor && vf && motor->load(motor, rutas[i], vf) && motor->run(motor);
		registro("script %s: %s %s", rutas[i], bien ? "en marcha" : "con error",
		         !bien && motor ? (motor->getError(motor) ? motor->getError(motor) : "") : "");
		if (vf)
			vf->close(vf);
	}
}

static void guiones_avisar(const char *que) {
	if (hay_guion)
		mScriptContextTriggerCallback(&guion, que);
}

static void guiones_acabar(void) {
	if (!hay_guion)
		return;
	mScriptContextTriggerCallback(&guion, "shutdown");
	hay_guion = false;
	mScriptContextDetachCore(&guion);
	mScriptContextDeinit(&guion);
	SocketSubsystemDeinit();
}
#else
static void guiones_empezar(struct mCore *core, const char **rutas, int n) { (void)core, (void)rutas, (void)n; }
static void guiones_avisar(const char *que) { (void)que; }
static void guiones_acabar(void) {}
#endif

/* ---- Archivos ---- */

static bool es_juego(struct VFile *vf) { return mCoreIsCompatible(vf) != mPLATFORM_NONE; }

static bool existe(const char *ruta) {
	struct stat st;
	return stat(ruta, &st) == 0;
}

static bool guardar_estado(struct mCore *core, const char *ruta) {
	char tmp[PATH_MAX + NAME_MAX + 32];
	snprintf(tmp, sizeof(tmp), "%s.tmp", ruta);
	struct VFile *vf = VFileOpen(tmp, O_CREAT | O_TRUNC | O_RDWR); /* mGBA lo mapea: tiene que poder leerlo */
	if (!vf)
		return false;
	/* Sin la partida del juego dentro: cargar esto no toca lo que tienes guardado en el juego. */
	bool bien = mCoreSaveStateNamed(core, vf, SAVESTATE_RTC | SAVESTATE_METADATA);
	vf->close(vf);
	if (!bien || rename(tmp, ruta) != 0) {
		unlink(tmp);
		return false;
	}
	return true;
}

static bool cargar_estado(struct mCore *core, const char *ruta) {
	struct VFile *vf = VFileOpen(ruta, O_RDONLY);
	if (!vf)
		return false;
	bool bien = mCoreLoadStateNamed(core, vf, SAVESTATE_RTC);
	vf->close(vf);
	return bien;
}

/* El cable link de la sala, de las opciones de Gambatte que escribe la app Pokémon:
 *   gambatte_gb_link_mode = "Network Server" | "Network Client" | "Not Connected"
 *   gambatte_gb_link_network_port = "56400"
 *   gambatte_gb_link_network_server_ip_1..12 = una cifra cada una (127.000.000.001) */
static void cable_de_opciones(struct mCore *core, const char *ruta) {
	FILE *f = ruta ? fopen(ruta, "r") : NULL;
	if (!f)
		return;
	char modo[128] = "", cifras[13] = "000000000000", linea[512];
	int puerto = 56400;
	while (fgets(linea, sizeof(linea), f)) {
		char clave[128], valor[128];
		if (sscanf(linea, " %127[^= ] = \"%127[^\"]\"", clave, valor) != 2)
			continue;
		int n;
		if (!strcmp(clave, "gambatte_gb_link_mode"))
			snprintf(modo, sizeof(modo), "%s", valor);
		else if (!strcmp(clave, "gambatte_gb_link_network_port"))
			puerto = atoi(valor);
		else if (sscanf(clave, "gambatte_gb_link_network_server_ip_%d", &n) == 1 && n >= 1 && n <= 12)
			cifras[n - 1] = valor[0];
	}
	fclose(f);
	bool servidor = !strcmp(modo, "Network Server");
	if (!servidor && strcmp(modo, "Network Client") != 0)
		return;
	char ip[32];
	snprintf(ip, sizeof(ip), "%d.%d.%d.%d", (cifras[0] - '0') * 100 + (cifras[1] - '0') * 10 + (cifras[2] - '0'),
	         (cifras[3] - '0') * 100 + (cifras[4] - '0') * 10 + (cifras[5] - '0'),
	         (cifras[6] - '0') * 100 + (cifras[7] - '0') * 10 + (cifras[8] - '0'),
	         (cifras[9] - '0') * 100 + (cifras[10] - '0') * 10 + (cifras[11] - '0'));
	cable_empezar(core, servidor, ip, puerto > 0 ? puerto : 56400);
}

/* Una captura como las de RetroArch: «<juego>-AAMMDD-HHMMSS.png». */
static void capturar(const char *dir, const char *base, const color_t *px, unsigned w, unsigned h) {
	char hora[32], ruta[PATH_MAX + NAME_MAX + 40];
	time_t t = time(NULL);
	strftime(hora, sizeof(hora), "%y%m%d-%H%M%S", localtime(&t));
	snprintf(ruta, sizeof(ruta), "%s/%s-%s.png", dir, base, hora);
	if (!guardar_png(ruta, (const uint8_t *)px, (int)w, (int)h, 256 * (int)sizeof(color_t)))
		registro("no se ha podido guardar la captura %s", ruta);
}

/* Al ir rápido no se dibujan las imágenes que no se van a ver (es lo que más cuesta). */
static void saltar_imagenes(struct mCore *core, int n) {
	mCoreConfigSetIntValue(&core->config, "frameskip", n);
	core->reloadConfigOption(core, "frameskip", &core->config);
}

/* «Pokemon - FireRed Version (USA)» -> «Pokemon - FireRed Version». */
static void nombre_bonito(char *dst, size_t n, const char *base) {
	snprintf(dst, n, "%s", base);
	char *p = strstr(dst, " (");
	if (p && p != dst)
		*p = 0;
}

int juego(const struct opciones *o) {
	mLogSetDefaultLogger(&registro_mgba);

	/* El juego (también dentro de un .zip o .7z, como los tienes). */
	const char *ext = strrchr(o->rom, '.');
	struct VDir *archivo = NULL;
	struct VFile *vf = NULL;
	if (ext && (!strcasecmp(ext, ".zip") || !strcasecmp(ext, ".7z"))) {
		archivo = VDirOpenArchive(o->rom);
		if (archivo)
			vf = VDirFindFirst(archivo, es_juego);
	} else {
		vf = VFileOpen(o->rom, O_RDONLY);
	}
	if (!vf) {
		registro("no puedo abrir %s", o->rom);
		return 1;
	}
	struct mCore *core = mCoreFindVF(vf);
	if (!core || !core->init(core)) {
		registro("%s no es un juego de GB, GBC ni GBA", o->rom);
		return 1;
	}
	mCoreInitConfig(core, "panther");
	mCoreConfigSetDefaultValue(&core->config, "idleOptimization", "detect");
	mCoreConfigSetDefaultIntValue(&core->config, "sgb.borders", 0);
	mCoreConfigSetDefaultValue(&core->config, "gb.model", "CGB"); /* los de GB, con los colores de la GBC */
	mCoreConfigSetDefaultIntValue(&core->config, "useBios", 0);
	mCoreLoadForeignConfig(core, &core->config);

	static color_t pixeles[256 * 224];
	core->setVideoBuffer(core, pixeles, 256);
	core->setAudioBufferSize(core, 2048);
	if (!core->loadROM(core, vf)) {
		registro("no se ha podido cargar %s", o->rom);
		core->deinit(core);
		return 1;
	}

	/* Nombres de archivo, como RetroArch: el del juego sin la extensión. */
	char dir_juego[PATH_MAX], base[NAME_MAX + 1];
	snprintf(dir_juego, sizeof(dir_juego), "%s", o->rom);
	char *barra = strrchr(dir_juego, '/');
	const char *nombre = barra ? barra + 1 : o->rom;
	snprintf(base, sizeof(base), "%s", nombre);
	char *punto = strrchr(base, '.');
	if (punto && punto != base)
		*punto = 0;
	if (barra)
		*barra = 0;
	else
		snprintf(dir_juego, sizeof(dir_juego), ".");
	char srm[PATH_MAX + NAME_MAX + 16], rapido_ruta[PATH_MAX + NAME_MAX + 16], auto_ruta[PATH_MAX + NAME_MAX + 16];
	snprintf(srm, sizeof(srm), "%s/%s.srm", o->dir_guardados ? o->dir_guardados : dir_juego, base);
	snprintf(rapido_ruta, sizeof(rapido_ruta), "%s/%s.estado", o->dir_estados ? o->dir_estados : dir_juego, base);
	snprintf(auto_ruta, sizeof(auto_ruta), "%s/%s.estado-auto", o->dir_estados ? o->dir_estados : dir_juego, base);
	struct stat st_srm, st_auto;
	bool habia_srm = stat(srm, &st_srm) == 0;
	if (!mCoreLoadSaveFile(core, srm, false))
		registro("no puedo abrir la partida %s", srm);
	core->reset(core);

	const char *scripts[10];
	int n_scripts = 0;
	char multijugador[PATH_MAX];
	if (o->dir_sistema && core->platform(core) == mPLATFORM_GBA) {
		snprintf(multijugador, sizeof(multijugador), "%s/pokemulador/multijugador.lua", o->dir_sistema);
		if (existe(multijugador))
			scripts[n_scripts++] = multijugador;
	}
	for (int i = 0; i < o->n_scripts && n_scripts < 10; i++)
		scripts[n_scripts++] = o->scripts[i];

	if (o->medir > 0) {
		guiones_empezar(core, scripts, n_scripts);
		for (int salto = 0; salto <= 2; salto += 2) {
			saltar_imagenes(core, salto);
			Uint64 t0 = SDL_GetPerformanceCounter();
			for (int i = 0; i < o->medir; i++) {
				core->setKeys(core, 0);
				core->runFrame(core);
				struct blip_t *ch[2] = {core->getAudioChannel(core, 0), core->getAudioChannel(core, 1)};
				static int16_t tira[4096 * 2];
				for (int c = 0; c < 2; c++)
					while (blip_samples_avail(ch[c]) > 0)
						blip_read_samples(ch[c], tira, 4096, 1);
			}
			double s = (double)(SDL_GetPerformanceCounter() - t0) / SDL_GetPerformanceFrequency();
			double fps = (double)core->frequency(core) / core->frameCycles(core);
			printf("%s: %d imágenes en %.2f s: %.0f por segundo (%.1f veces la velocidad normal)\n",
			       salto ? "dibujando 1 de cada 3" : "dibujando todas", o->medir, s, o->medir / s, o->medir / s / fps);
		}
		guiones_acabar();
		core->deinit(core);
		return 0;
	}
	if (!video_abrir()) {
		core->deinit(core);
		return 1;
	}
	sonido_abrir();
	mandos_abrir();
	letras_cargar();

	/* ¿Seguir donde lo dejaste? Solo si la partida del juego no ha cambiado después (si has jugado a
	 * este juego con otro programa, manda lo que hiciste allí). */
	if (!o->sin_seguir && stat(auto_ruta, &st_auto) == 0 && (!habia_srm || st_auto.st_mtime + 30 >= st_srm.st_mtime)) {
		if (cargar_estado(core, auto_ruta))
			aviso("Sigues donde lo dejaste");
	}
	guiones_empezar(core, scripts, n_scripts);
	cable_de_opciones(core, o->opciones_nucleo);
	ordenes_abrir();

	char titulo[NAME_MAX + 1];
	nombre_bonito(titulo, sizeof(titulo), base);
	unsigned w, h;
	core->desiredVideoDimensions(core, &w, &h);
	enum escala escala = core->platform(core) == mPLATFORM_GBA ? ESCALA_NITIDA : ESCALA_ENTERA;
	struct blip_t *izq = core->getAudioChannel(core, 0), *der = core->getAudioChannel(core, 1);
	double hz = video_hz();
	double fps = (double)core->frequency(core) / core->frameCycles(core);
	registro("%s: %ux%u a %.3f imágenes/s", titulo, w, h, fps);

	static int16_t muestras[4096 * 2];
	bool en_menu = false, rapido = false, chat = false;
	int factor = ajuste_factor_rapido(), salto_puesto = 0;
	Uint64 frec_reloj = SDL_GetPerformanceFrequency();

	while (!salir_ya) {
		Uint64 empieza = SDL_GetPerformanceCounter();
		SDL_Event ev;
		while (SDL_PollEvent(&ev))
			if (ev.type == SDL_QUIT)
				salir_ya = 1;
		uint32_t estado, nuevos;
		mandos_leer(&estado, &nuevos);
		capa_mirar();
		cable_mantener();
		for (const char *orden; (orden = ordenes_leer()) != NULL;) {
			if (!strcmp(orden, "SCREENSHOT"))
				capturar(o->dir_capturas ? o->dir_capturas : dir_juego, base, pixeles, w, h);
			else if (!strcmp(orden, "QUIT"))
				salir_ya = 1;
		}
		bool chat_ahora = capa_pantalla_completa();
		if (chat_ahora != chat) {
			chat = chat_ahora;
			nuevos = 0;
		}
		if (!chat && !en_menu) {
			bool combo = (estado & BOT_SELECT) && (estado & BOT_START) && (nuevos & (BOT_SELECT | BOT_START));
			if (combo) { /* (R3 y F son del chat de WhatsApp: los mira el servicio) */
				en_menu = true;
				menu_abrir(titulo, existe(rapido_ruta));
			} else if (nuevos & BOT_R2) {
				rapido = !rapido;
				char s[40];
				snprintf(s, sizeof(s), "Velocidad ×%d", factor);
				aviso(rapido ? s : "Velocidad normal");
			}
		} else if (!chat) {
			switch (menu_pulsar(nuevos, &factor)) {
			case MENU_SEGUIR:
				en_menu = false;
				break;
			case MENU_GUARDAR:
				aviso(guardar_estado(core, rapido_ruta) ? "Partida rápida guardada" : "No se ha podido guardar");
				en_menu = false;
				break;
			case MENU_CARGAR:
				aviso(cargar_estado(core, rapido_ruta) ? "Partida rápida cargada" : "No se ha podido cargar");
				en_menu = false;
				break;
			case MENU_REINICIAR:
				core->reset(core);
				guiones_avisar("reset");
				aviso("Juego empezado de nuevo");
				en_menu = false;
				break;
			case MENU_SALIR:
				salir_ya = 1;
				break;
			case MENU_NADA:
				break;
			}
		}

		bool corre = !en_menu && !chat;
		bool deprisa = corre && rapido;
		sonido_pausar(!corre || deprisa);
		video_sincronizar(!deprisa);
		int salto = deprisa ? factor - 1 : 0;
		if (salto != salto_puesto) {
			saltar_imagenes(core, salto);
			salto_puesto = salto;
		}
		if (corre) {
			uint32_t k = teclas(estado);
			int n = deprisa ? factor : 1;
			double frec = sonido_frecuencia() * fps / hz * sonido_ajuste();
			blip_set_rates(izq, core->frequency(core), frec);
			blip_set_rates(der, core->frequency(core), frec);
			for (int i = 0; i < n; i++) {
				core->setKeys(core, k);
				core->runFrame(core);
				int hay = blip_samples_avail(izq);
				while (hay > 0) {
					int m = hay > 4096 ? 4096 : hay;
					blip_read_samples(izq, muestras, m, 1);
					blip_read_samples(der, muestras + 1, m, 1);
					if (!deprisa)
						sonido_meter(muestras, m);
					hay -= m;
				}
			}
			video_imagen(pixeles, (int)w, (int)h, 256 * (int)sizeof(color_t), SDL_PIXELFORMAT_XBGR8888, escala);
		}

		video_empezar();
		video_dibujar_imagen();
		if (en_menu && !chat)
			menu_dibujar();
		avisos_dibujar(deprisa, factor);
		capa_dibujar();
		video_presentar();

		if (deprisa) {
			/* Sin esperar al refresco: n imágenes del juego en lo que dura una de la pantalla. */
			double pasado = (double)(SDL_GetPerformanceCounter() - empieza) / frec_reloj;
			double falta = 1.0 / hz - pasado;
			if (falta > 0.001)
				SDL_Delay((Uint32)(falta * 1000));
		}
	}

	registro("saliendo: guardo dónde ibas");
	guiones_acabar();
	cable_acabar();
	ordenes_cerrar();
	if (!o->sin_seguir && !guardar_estado(core, auto_ruta))
		registro("no se ha podido guardar %s", auto_ruta);
	sonido_cerrar();
	core->unloadROM(core);
	mCoreConfigDeinit(&core->config);
	core->deinit(core);
	if (archivo)
		archivo->close(archivo);
	video_cerrar();
	return 0;
}
