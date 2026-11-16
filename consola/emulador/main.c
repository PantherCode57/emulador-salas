/* Emulador de Panther OS.
 *
 *   emulador <juego>                     un juego de GB, GBC o GBA (también .zip o .7z)
 *   emulador --script x.lua <juego>      con scripts de Lua de mGBA
 *   emulador --pantalla                  la pantalla de las apps de la consola
 *
 * También entiende cómo llamaban las apps a RetroArch, para poder ponerlo en su sitio sin tocarlas:
 *   emulador [--config c] -L <núcleo> [juego] [--appendconfig c]
 * Si el núcleo es el de la pantalla (pokemulador_libretro.so) hace de pantalla; si no, abre el juego
 * con las carpetas de partidas y de sistema del --appendconfig.
 */
#include "panther.h"

#include <limits.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

volatile int salir_ya;
const char *carpeta_programa = ".";

void registro(const char *fmt, ...) {
	char hora[16];
	time_t t = time(NULL);
	strftime(hora, sizeof(hora), "%H:%M:%S", localtime(&t));
	fprintf(stderr, "%s ", hora);
	va_list a;
	va_start(a, fmt);
	vfprintf(stderr, fmt, a);
	va_end(a);
	fputc('\n', stderr);
}

uint64_t ahora_ms(void) {
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	return (uint64_t)ts.tv_sec * 1000 + (uint64_t)ts.tv_nsec / 1000000;
}

static void al_pedir_salir(int s) {
	(void)s;
	salir_ya = 1;
}

/* Lee las líneas «clave = "valor"» de una configuración de RetroArch que nos interesan. */
static void leer_config(const char *ruta, struct opciones *o) {
	FILE *f = fopen(ruta, "r");
	if (!f)
		return;
	char linea[1024];
	while (fgets(linea, sizeof(linea), f)) {
		char clave[128], valor[900];
		if (sscanf(linea, " %127[^= ] = \"%899[^\"]\"", clave, valor) != 2)
			continue;
		if (!strcmp(clave, "savestate_auto_load") && !strcmp(valor, "false"))
			o->sin_seguir = true;
		const char **dst = NULL;
		if (!strcmp(clave, "savefile_directory"))
			dst = &o->dir_guardados;
		else if (!strcmp(clave, "savestate_directory"))
			dst = &o->dir_estados;
		else if (!strcmp(clave, "system_directory"))
			dst = &o->dir_sistema;
		else if (!strcmp(clave, "screenshot_directory"))
			dst = &o->dir_capturas;
		else if (!strcmp(clave, "core_options_path"))
			dst = &o->opciones_nucleo;
		if (dst && valor[0] && strcmp(valor, "default") != 0) {
			if (valor[0] == '~' && getenv("HOME")) {
				char *r = malloc(strlen(getenv("HOME")) + strlen(valor) + 1);
				sprintf(r, "%s%s", getenv("HOME"), valor + 1);
				*dst = r;
			} else {
				*dst = strdup(valor);
			}
		}
	}
	fclose(f);
}

int main(int argc, char **argv) {
	static char carpeta[PATH_MAX];
	ssize_t n = readlink("/proc/self/exe", carpeta, sizeof(carpeta) - 1);
	if (n > 0) {
		carpeta[n] = 0;
		char *b = strrchr(carpeta, '/');
		if (b)
			*b = 0;
		carpeta_programa = carpeta;
	}
	setvbuf(stderr, NULL, _IOLBF, 0);
	signal(SIGTERM, al_pedir_salir);
	signal(SIGINT, al_pedir_salir);
	signal(SIGHUP, al_pedir_salir);
	signal(SIGPIPE, SIG_IGN);

	struct opciones o;
	memset(&o, 0, sizeof(o));
	bool pantalla = false;
	const char *nucleo = NULL;
	for (int i = 1; i < argc; i++) {
		const char *a = argv[i];
		if (!strcmp(a, "--pantalla")) {
			pantalla = true;
		} else if (!strcmp(a, "--script") && i + 1 < argc) {
			if (o.n_scripts < 8)
				o.scripts[o.n_scripts++] = argv[++i];
		} else if (!strcmp(a, "--medir") && i + 1 < argc) {
			o.medir = atoi(argv[++i]);
		} else if (!strcmp(a, "-L") && i + 1 < argc) {
			nucleo = argv[++i];
		} else if ((!strcmp(a, "--appendconfig") || !strcmp(a, "--config") || !strcmp(a, "-c")) && i + 1 < argc) {
			/* --config es la de toda la consola (no tiene carpetas propias); --appendconfig, la de la app. */
			const char *ruta = argv[++i];
			if (!strcmp(a, "--appendconfig"))
				leer_config(ruta, &o);
		} else if (a[0] == '-') {
			/* opciones de RetroArch que no hacen falta aquí */
		} else {
			o.rom = a;
		}
	}
	if (nucleo && strstr(nucleo, "pokemulador_libretro"))
		pantalla = true;
	registro("Emulador de Panther OS (%s)", pantalla ? "pantalla de las apps" : (o.rom ? o.rom : "sin juego"));
	if (pantalla) {
		int r = pantalla_apps();
		SDL_Quit();
		return r;
	}
	if (!o.rom) {
		fprintf(stderr, "uso: %s <juego de GB, GBC o GBA> | --pantalla\n", argv[0]);
		return 2;
	}
	int r = juego(&o);
	SDL_Quit();
	return r;
}
