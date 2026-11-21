// Lo que Panther OS apunta en la tarjeta de la flashcard, en la carpeta panther/.
// wifi.txt: la red de la última vez (red=, clave= y guardada=1 si es de los ajustes de la DS).
#include <stdio.h>
#include <string.h>

#include "sistema.h"

static bool ruta(char *s, int n, const char *archivo)
{
	const char *c = plat_carpeta();
	if (!c)
		return false;
	snprintf(s, n, "%s/%s", c, archivo);
	return true;
}

static void quitar_fin(char *s)
{
	int n = (int)strlen(s);
	while (n > 0 && (s[n - 1] == '\n' || s[n - 1] == '\r'))
		s[--n] = 0;
}

bool wifi_leer(char *ssid, int nssid, char *clave, int nclave, bool *guardada)
{
	char r[160], l[160];
	ssid[0] = clave[0] = 0;
	*guardada = false;
	if (!ruta(r, sizeof r, "wifi.txt"))
		return false;
	FILE *f = fopen(r, "r");
	if (!f)
		return false;
	while (fgets(l, sizeof l, f)) {
		quitar_fin(l);
		if (!strncmp(l, "red=", 4))
			snprintf(ssid, nssid, "%s", l + 4);
		else if (!strncmp(l, "clave=", 6))
			snprintf(clave, nclave, "%s", l + 6);
		else if (!strcmp(l, "guardada=1"))
			*guardada = true;
	}
	fclose(f);
	return ssid[0] != 0;
}

bool wifi_guardar(const char *ssid, const char *clave, bool guardada)
{
	char r[160];
	if (!ruta(r, sizeof r, "wifi.txt"))
		return false;
	FILE *f = fopen(r, "w");
	if (!f)
		return false;
	fprintf(f, "red=%s\nclave=%s\nguardada=%d\n", ssid, clave, guardada ? 1 : 0);
	return fclose(f) == 0;
}

// puente.txt: el código de 6 cifras del puente («codigo=123456»).
bool puente_codigo_leer(char *c, int n)
{
	char r[160], l[64];
	c[0] = 0;
	if (!ruta(r, sizeof r, "puente.txt"))
		return false;
	FILE *f = fopen(r, "r");
	if (!f)
		return false;
	while (fgets(l, sizeof l, f)) {
		quitar_fin(l);
		if (!strncmp(l, "codigo=", 7))
			snprintf(c, n, "%s", l + 7);
	}
	fclose(f);
	return c[0] != 0;
}

bool puente_codigo_guardar(const char *c)
{
	char r[160];
	if (!ruta(r, sizeof r, "puente.txt"))
		return false;
	FILE *f = fopen(r, "w");
	if (!f)
		return false;
	fprintf(f, "codigo=%s\n", c);
	return fclose(f) == 0;
}
