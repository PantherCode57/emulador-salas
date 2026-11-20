/* Lo que se recuerda entre partidas, en ~/.config/panther/emulador.cfg: la velocidad de R2 y si van
 * los Caramelos Raros infinitos. */
#include "panther.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

static bool leido;
static int factor = 3;
static bool caramelos;

static void ruta(char *r, size_t n) {
	const char *casa = getenv("HOME");
	snprintf(r, n, "%s/.config/panther/emulador.cfg", casa ? casa : "/home/ark");
}

static void leer(void) {
	if (leido)
		return;
	leido = true;
	char r[512];
	ruta(r, sizeof(r));
	FILE *f = fopen(r, "r");
	if (!f)
		return;
	char linea[128];
	while (fgets(linea, sizeof(linea), f)) {
		int v;
		if (sscanf(linea, "rapido = %d", &v) == 1 && v >= 2 && v <= 8)
			factor = v;
		else if (sscanf(linea, "caramelos = %d", &v) == 1)
			caramelos = v != 0;
	}
	fclose(f);
}

static void guardar(void) {
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
		fprintf(f, "rapido = %d\ncaramelos = %d\n", factor, caramelos ? 1 : 0);
		fclose(f);
	}
}

int ajuste_factor_rapido(void) {
	leer();
	return factor;
}

void ajuste_guardar_factor(int v) {
	leer();
	factor = v;
	guardar();
}

bool ajuste_caramelos(void) {
	leer();
	return caramelos;
}

void ajuste_guardar_caramelos(bool si) {
	leer();
	caramelos = si;
	guardar();
}
