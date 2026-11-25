/* El arranque de la app Panther del móvil. Lo llama la actividad Juego (Java) con:
 *
 *   --casa <carpeta>      sus archivos: los ajustes (.config/panther) y las letras (selawk.ttf…)
 *   --recorte <píxeles>   lo que tapa la cámara de la pantalla, para no poner botones debajo
 *   --script <x.lua>      scripts de Lua de mGBA (los Caramelos Raros)
 *   --guardados <carpeta> donde va la partida (.srm); si no, junto al juego
 *   --estados <carpeta>   donde va lo de seguir donde lo dejaste; si no, junto al juego
 *   --sistema <carpeta>   de ahí se carga pokemulador/multijugador.lua (las salas de Pokemulador)
 *   --opciones <archivo>  el cable link de la sala (las opciones de Gambatte)
 *   --capturas <carpeta>  donde van las capturas que pide la app
 *   --sin-seguir          no guardar ni retomar dónde ibas
 *   --sin-vibrar          los botones de la pantalla no vibran
 *   <juego>               el .gba, .gbc o .gb (o un .7z)
 *
 * Cada juego se abre en un proceso aparte (":juego" en el manifiesto) que se cierra al salir: así el
 * código de la consola, que guarda cosas en variables estáticas, empieza siempre de cero.
 */
#include "movil.h"

#include <android/log.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>

volatile int salir_ya;
const char *carpeta_programa = ".";
int recorte_pantalla;
bool vibrar_botones = true;
volatile int resultado_juego = -1;

void registro(const char *fmt, ...) {
	va_list a;
	va_start(a, fmt);
	__android_log_vprint(ANDROID_LOG_INFO, "Panther", fmt, a);
	va_end(a);
}

uint64_t ahora_ms(void) {
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	return (uint64_t)ts.tv_sec * 1000 + (uint64_t)ts.tv_nsec / 1000000;
}

/* Los avisos y el chat de WhatsApp que pone el servicio de la consola: en el móvil no hay. */
void capa_mirar(void) {}
bool capa_pantalla_completa(void) { return false; }
void capa_dibujar(void) {}

int main(int argc, char *argv[]) {
	salir_ya = 0;
	struct opciones o;
	memset(&o, 0, sizeof(o));
	const char *casa = NULL;
	for (int i = 1; i < argc; i++) {
		const char *a = argv[i];
		if (!strcmp(a, "--casa") && i + 1 < argc)
			casa = argv[++i];
		else if (!strcmp(a, "--recorte") && i + 1 < argc)
			recorte_pantalla = atoi(argv[++i]);
		else if (!strcmp(a, "--script") && i + 1 < argc) {
			if (o.n_scripts < 8)
				o.scripts[o.n_scripts++] = argv[++i];
		} else if (!strcmp(a, "--guardados") && i + 1 < argc)
			o.dir_guardados = argv[++i];
		else if (!strcmp(a, "--estados") && i + 1 < argc)
			o.dir_estados = argv[++i];
		else if (!strcmp(a, "--sistema") && i + 1 < argc)
			o.dir_sistema = argv[++i];
		else if (!strcmp(a, "--opciones") && i + 1 < argc)
			o.opciones_nucleo = argv[++i];
		else if (!strcmp(a, "--capturas") && i + 1 < argc)
			o.dir_capturas = argv[++i];
		else if (!strcmp(a, "--sin-seguir"))
			o.sin_seguir = true;
		else if (!strcmp(a, "--sin-vibrar"))
			vibrar_botones = false;
		else if (a[0] != '-')
			o.rom = a;
	}
	if (casa) {
		/* ajustes.c guarda la velocidad de «Rápido» en $HOME/.config/panther/emulador.cfg */
		carpeta_programa = casa;
		setenv("HOME", casa, 1);
		char dir[1024];
		snprintf(dir, sizeof(dir), "%s/.config", casa);
		mkdir(dir, 0755);
	}
	/* «Atrás» abre el menú de pausa en vez de cerrar el juego sin guardar. */
	SDL_SetHint(SDL_HINT_ANDROID_TRAP_BACK_BUTTON, "1");
	SDL_SetHint(SDL_HINT_TOUCH_MOUSE_EVENTS, "0");
	SDL_SetHint(SDL_HINT_MOUSE_TOUCH_EVENTS, "0");
	SDL_SetHint(SDL_HINT_ACCELEROMETER_AS_JOYSTICK, "0");
	if (!o.rom) {
		registro("no me han dicho qué juego abrir");
		resultado_juego = 2;
		return 2;
	}
	registro("Panther para el móvil: %s (recorte %d, %d scripts)", o.rom, recorte_pantalla, o.n_scripts);
	int r = juego(&o);
	SDL_Quit();
	resultado_juego = r;
	return r;
}
