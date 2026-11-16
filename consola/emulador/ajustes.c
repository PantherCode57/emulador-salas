/* Lo que se recuerda entre partidas, en ~/.config/panther/emulador.cfg (de momento, la velocidad de R2). */
#include "panther.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

static int factor = 0;

static void ruta(char *r, size_t n) {
	const char *casa = getenv("HOME");
	snprintf(r, n, "%s/.config/panther/emulador.cfg", casa ? casa : "/home/ark");
}

int ajuste_factor_rapido(void) {
	if (factor)
		return factor;
	factor = 3;
	char r[512];
	ruta(r, sizeof(r));
	FILE *f = fopen(r, "r");
	if (f) {
		char linea[128];
		while (fgets(linea, sizeof(linea), f)) {
			int v;
			if (sscanf(linea, "rapido = %d", &v) == 1 && v >= 2 && v <= 8)
				factor = v;
		}
		fclose(f);
	}
	return factor;
}

void ajuste_guardar_factor(int v) {
	factor = v;
	char r[512];
	ruta(r, sizeof(r));
	char dir[512];
	snprintf(dir, sizeof(dir), "%s", r);
	char *barra = strrchr(dir, '/');
	if (barra) {
		*barra = 0;
		mkdir(dir, 0755);
	}
	FILE *f = fopen(r, "w");
	if (f) {
		fprintf(f, "rapido = %d\n", v);
		fclose(f);
	}
}
