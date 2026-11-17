/* Los mandos en el móvil: los botones de la pantalla, la tecla «atrás» (abre el menú de pausa) y
 * los mandos de verdad (Bluetooth o USB), con los mismos botones que la consola.
 *
 * juego.c lee la cola de SDL él mismo (solo mira si hay que salir), así que los toques se recogen
 * con SDL_AddEventWatch, que los ve antes de entrar en la cola. Android los manda desde su hilo, no
 * desde el del juego: por eso va todo con un cerrojo.
 */
#include "movil.h"

#include <string.h>

#define DEDOS 10
static struct {
	SDL_FingerID id;
	bool activo;
	uint32_t bits;
} dedos[DEDOS];
static SDL_mutex *cerrojo;
static uint32_t nuevos_pantalla; /* lo pulsado desde la última lectura (aunque ya se soltara) */
static bool atras;
static uint64_t mantener[32];    /* un toque muy corto se le enseña al juego unas imágenes */
static uint32_t pulsado_ahora;   /* para dibujar */
static uint32_t fisico_antes;
static SDL_GameController *mandos[4];
static SDL_Haptic *vibrador;

static int dedo(SDL_FingerID id, bool nuevo) {
	for (int i = 0; i < DEDOS; i++)
		if (dedos[i].activo && dedos[i].id == id)
			return i;
	if (!nuevo)
		return -1;
	for (int i = 0; i < DEDOS; i++)
		if (!dedos[i].activo)
			return i;
	return -1;
}

static int vigilar(void *datos, SDL_Event *e) {
	(void)datos;
	switch (e->type) {
	case SDL_FINGERDOWN:
	case SDL_FINGERMOTION:
	case SDL_FINGERUP: {
		uint32_t b = e->type == SDL_FINGERUP ? 0 : tactil_botones(e->tfinger.x, e->tfinger.y);
		SDL_LockMutex(cerrojo);
		int i = dedo(e->tfinger.fingerId, e->type == SDL_FINGERDOWN);
		if (i >= 0) {
			if (e->type == SDL_FINGERUP) {
				dedos[i].activo = false;
				dedos[i].bits = 0;
			} else {
				/* Al deslizar el dedo a otro botón, ese cuenta como pulsado de nuevo. */
				nuevos_pantalla |= b & ~dedos[i].bits;
				dedos[i].id = e->tfinger.fingerId;
				dedos[i].activo = true;
				dedos[i].bits = b;
			}
		}
		tactil_visible = 1;
		SDL_UnlockMutex(cerrojo);
		break;
	}
	case SDL_KEYDOWN:
		if (e->key.keysym.scancode == SDL_SCANCODE_AC_BACK && !e->key.repeat) {
			SDL_LockMutex(cerrojo);
			atras = true;
			SDL_UnlockMutex(cerrojo);
		}
		break;
	}
	return 1;
}

bool mandos_abrir(void) {
	cerrojo = SDL_CreateMutex();
	if (SDL_InitSubSystem(SDL_INIT_GAMECONTROLLER) != 0)
		registro("sin mandos: %s", SDL_GetError());
	if (SDL_InitSubSystem(SDL_INIT_HAPTIC) == 0 && SDL_NumHaptics() > 0) {
		vibrador = SDL_HapticOpen(0);
		if (vibrador && SDL_HapticRumbleInit(vibrador) != 0) {
			SDL_HapticClose(vibrador);
			vibrador = NULL;
		}
	}
	SDL_AddEventWatch(vigilar, NULL);
	return true;
}

/* Los mandos conectados (se abren solos al conectarlos). Botones como en la consola: el de abajo es
 * B y el de la derecha es A, como en las de Nintendo. */
static uint32_t fisicos(void) {
	for (int k = 0; k < 4; k++)
		if (mandos[k] && !SDL_GameControllerGetAttached(mandos[k])) {
			SDL_GameControllerClose(mandos[k]);
			mandos[k] = NULL;
		}
	int n = SDL_NumJoysticks();
	for (int i = 0; i < n && i < 8; i++) {
		if (!SDL_IsGameController(i))
			continue;
		SDL_JoystickID id = SDL_JoystickGetDeviceInstanceID(i);
		bool abierto = false;
		int libre = -1;
		for (int k = 0; k < 4; k++) {
			if (mandos[k] && SDL_JoystickInstanceID(SDL_GameControllerGetJoystick(mandos[k])) == id)
				abierto = true;
			if (!mandos[k] && libre < 0)
				libre = k;
		}
		if (!abierto && libre >= 0) {
			mandos[libre] = SDL_GameControllerOpen(i);
			if (mandos[libre])
				registro("mando: %s", SDL_GameControllerName(mandos[libre]));
		}
	}
	static const struct {
		SDL_GameControllerButton b;
		uint32_t bit;
	} tabla[] = {
		{SDL_CONTROLLER_BUTTON_A, BOT_B},
		{SDL_CONTROLLER_BUTTON_B, BOT_A},
		{SDL_CONTROLLER_BUTTON_X, BOT_Y},
		{SDL_CONTROLLER_BUTTON_Y, BOT_X},
		{SDL_CONTROLLER_BUTTON_BACK, BOT_SELECT},
		{SDL_CONTROLLER_BUTTON_START, BOT_START},
		{SDL_CONTROLLER_BUTTON_GUIDE, BOT_SELECT | BOT_START},
		{SDL_CONTROLLER_BUTTON_LEFTSHOULDER, BOT_L},
		{SDL_CONTROLLER_BUTTON_RIGHTSHOULDER, BOT_R},
		{SDL_CONTROLLER_BUTTON_LEFTSTICK, BOT_L3},
		{SDL_CONTROLLER_BUTTON_RIGHTSTICK, BOT_R3},
		{SDL_CONTROLLER_BUTTON_DPAD_UP, BOT_ARRIBA},
		{SDL_CONTROLLER_BUTTON_DPAD_DOWN, BOT_ABAJO},
		{SDL_CONTROLLER_BUTTON_DPAD_LEFT, BOT_IZQ},
		{SDL_CONTROLLER_BUTTON_DPAD_RIGHT, BOT_DER},
	};
	uint32_t b = 0;
	for (int k = 0; k < 4; k++) {
		SDL_GameController *m = mandos[k];
		if (!m)
			continue;
		for (size_t i = 0; i < sizeof(tabla) / sizeof(tabla[0]); i++)
			if (SDL_GameControllerGetButton(m, tabla[i].b))
				b |= tabla[i].bit;
		if (SDL_GameControllerGetAxis(m, SDL_CONTROLLER_AXIS_TRIGGERLEFT) > 16000)
			b |= BOT_L2;
		if (SDL_GameControllerGetAxis(m, SDL_CONTROLLER_AXIS_TRIGGERRIGHT) > 16000)
			b |= BOT_R2;
		int x = SDL_GameControllerGetAxis(m, SDL_CONTROLLER_AXIS_LEFTX);
		int y = SDL_GameControllerGetAxis(m, SDL_CONTROLLER_AXIS_LEFTY);
		if (x < -16000) b |= BOT_IZQ;
		if (x > 16000) b |= BOT_DER;
		if (y < -16000) b |= BOT_ARRIBA;
		if (y > 16000) b |= BOT_ABAJO;
	}
	/* Un teclado (en una tablet o un Chromebook), como el teclado de la consola. */
	const Uint8 *t = SDL_GetKeyboardState(NULL);
	if (t[SDL_SCANCODE_UP]) b |= BOT_ARRIBA;
	if (t[SDL_SCANCODE_DOWN]) b |= BOT_ABAJO;
	if (t[SDL_SCANCODE_LEFT]) b |= BOT_IZQ;
	if (t[SDL_SCANCODE_RIGHT]) b |= BOT_DER;
	if (t[SDL_SCANCODE_Z]) b |= BOT_B;
	if (t[SDL_SCANCODE_X]) b |= BOT_A;
	if (t[SDL_SCANCODE_A]) b |= BOT_Y;
	if (t[SDL_SCANCODE_S]) b |= BOT_X;
	if (t[SDL_SCANCODE_Q]) b |= BOT_L;
	if (t[SDL_SCANCODE_W]) b |= BOT_R;
	if (t[SDL_SCANCODE_SPACE]) b |= BOT_R2;
	if (t[SDL_SCANCODE_RETURN]) b |= BOT_START;
	if (t[SDL_SCANCODE_RSHIFT] || t[SDL_SCANCODE_BACKSPACE]) b |= BOT_SELECT;
	if (t[SDL_SCANCODE_ESCAPE]) b |= BOT_SELECT | BOT_START;
	return b;
}

void mandos_leer(uint32_t *estado, uint32_t *nuevos) {
	uint64_t ya = ahora_ms();
	uint32_t pantalla = 0, nuevos_p;
	bool atras_ahora;
	SDL_LockMutex(cerrojo);
	for (int i = 0; i < DEDOS; i++)
		if (dedos[i].activo)
			pantalla |= dedos[i].bits;
	nuevos_p = nuevos_pantalla;
	nuevos_pantalla = 0;
	atras_ahora = atras;
	atras = false;
	SDL_UnlockMutex(cerrojo);

	if (nuevos_p && vibrador)
		SDL_HapticRumblePlay(vibrador, 0.3f, 12);
	if (atras_ahora)
		nuevos_p |= BOT_SELECT | BOT_START | BOT_MENU_PANTALLA;
	for (int b = 0; b < 32; b++) {
		if (nuevos_p & (1u << b))
			mantener[b] = ya + 60;
		if (mantener[b] > ya)
			pantalla |= 1u << b;
	}
	pulsado_ahora = pantalla;

	uint32_t f = fisicos();
	uint32_t nuevos_f = f & ~fisico_antes;
	fisico_antes = f;
	if (nuevos_f)
		tactil_visible = 0;

	*estado = (pantalla | f) & ~BOT_MENU_PANTALLA;
	*nuevos = (nuevos_p | nuevos_f) & ~BOT_MENU_PANTALLA;
}

uint32_t mandos_pulsados_pantalla(void) { return pulsado_ahora; }
