// Panther OS para la Nintendo DS Lite: la animación de arranque, la pantalla de inicio (la hora
// arriba y las apps abajo) y el paso de una pantalla a otra.
#include <stdio.h>
#include <string.h>

#include "sistema.h"

uint32_t ahora_ms, imagenes;
struct tm hora;

enum { V_ARRANQUE, V_INICIO, V_APP, V_WIFI };
static int vista = V_ARRANQUE, vista_antes_wifi = V_INICIO;
static int elegida;
static uint32_t inicio_arranque;
static int ultimo_minuto = -1;
static bool sucio = true;

static bool tocando_antes, movido;
static int ini_x, ini_y;

#define DURACION_ARRANQUE 2200
#define QUIETO 700

#define TILE_W 72
#define TILE_H 70

static void caja_app(int i, int *x, int *y)
{
	*x = 10 + (i % 3) * 82;
	*y = 8 + (i / 3) * 78;
}

void sistema_iniciar(void)
{
	ahora_ms = plat_ms();
	inicio_arranque = ahora_ms;
	plat_hora(&hora);
	wifi_arrancar();
}

// Los toques: cuándo empieza uno, si el dedo se arrastra y si ha sido un toque corto.
static void procesar_toque(Entrada *e)
{
	bool tocando = (e->mantenido & B_TOQUE) != 0;
	e->toque_ini = tocando && !tocando_antes;
	e->toque = false;
	e->arrastrando = false;
	e->arrastre_dy = 0;
	if (e->toque_ini) {
		ini_x = e->tx;
		ini_y = e->ty;
		movido = false;
	}
	if (tocando) {
		int dx = e->tx - ini_x, dy = e->ty - ini_y;
		if (dx * dx + dy * dy > 64)
			movido = true;
		e->arrastrando = movido;
		e->arrastre_dy = dy;
	}
	if (!tocando && tocando_antes && !movido) {
		e->toque = true;
		e->toque_x = ini_x;
		e->toque_y = ini_y;
	}
	tocando_antes = tocando;
}

// --- Arranque: una línea que crece desde el centro y el nombre que sube (como en la consola) ---

static float suave(float t)
{
	if (t <= 0)
		return 0;
	if (t >= 1)
		return 1;
	float u = 1 - t;
	return 1 - u * u * u;
}

static void arranque_dibujar(Color *arriba, Color *abajo, uint32_t t)
{
	llenar(arriba, C_NEGRO);
	float p = suave((float)t / DURACION_ARRANQUE);
	int medio = (int)(104 * p);
	for (int d = -medio; d <= medio; d++) {
		float k = medio > 0 ? 1 - (float)(d < 0 ? -d : d) / medio : 0;
		rect_alfa(arriba, ANCHO / 2 + d, 104, 1, 2, C_VIOLETA, (int)(256 * k * k));
	}
	float titulo = suave(((float)t - 500) / 1000);
	if (titulo > 0) {
		int y = 66 + (int)((1 - titulo) * 12);
		const char *s = "Panther OS";
		texto_alfa(arriba, &letra_grande, ANCHO / 2 - ancho_texto(&letra_grande, s) / 2, y, s, C_BLANCO, (int)(256 * titulo));
	}
	float sub = suave(((float)t - 1100) / 900);
	if (sub > 0) {
		const char *s = "para Nintendo DS Lite";
		texto_alfa(arriba, &letra_normal, ANCHO / 2 - ancho_texto(&letra_normal, s) / 2, 114, s, C_GRIS, (int)(256 * sub));
	}
	llenar(abajo, C_NEGRO);
	if (sub > 0) {
		const char *s = "Versión " VERSION;
		texto_alfa(abajo, &letra_peq, ANCHO / 2 - ancho_texto(&letra_peq, s) / 2, 170, s, C_OSCURO, (int)(256 * sub));
	}
}

// --- Inicio ---

static int inicio_paso(const Entrada *e)
{
	for (int i = 0; i < N_APPS; i++) {
		int x, y;
		caja_app(i, &x, &y);
		if (e->toque && en(e->toque_x, e->toque_y, x, y, TILE_W, TILE_H)) {
			elegida = i;
			app_entrar(i);
			vista = V_APP;
			return CAMBIO;
		}
	}
	if ((e->toque && en(e->toque_x, e->toque_y, 10, 164, 236, 24)) || (e->pulsado & B_SELECT)) {
		vista_antes_wifi = V_INICIO;
		wifi_entrar();
		vista = V_WIFI;
		return CAMBIO;
	}
	if (e->pulsado & B_A) {
		app_entrar(elegida);
		vista = V_APP;
		return CAMBIO;
	}
	int antes = elegida;
	if ((e->repetido & B_DER) && elegida % 3 < 2)
		elegida++;
	if ((e->repetido & B_IZQ) && elegida % 3 > 0)
		elegida--;
	if ((e->repetido & B_ABAJO) && elegida < 3)
		elegida += 3;
	if ((e->repetido & B_ARRIBA) && elegida >= 3)
		elegida -= 3;
	if (elegida != antes)
		return CAMBIO;
	return (imagenes % 30) == 0 ? CAMBIO : NADA; // el wifi y la batería cambian solos
}

static void tile(Color *p, int i, bool elegido)
{
	int x, y;
	caja_app(i, &x, &y);
	const App *a = &apps[i];
	if (elegido)
		marco(p, x - 3, y - 3, TILE_W + 6, TILE_H + 6, 17, 2, C_BLANCO);
	redondeado_degradado(p, x, y, TILE_W, TILE_H, 14, a->c1, a->c2);
	a->icono(p, x + TILE_W / 2, y + 27);
	texto_centro(p, &letra_negrita, x + TILE_W / 2, y + 51, a->nombre, C_BLANCO);
}

static void inicio_dibujar(Color *arriba, Color *abajo)
{
	// Arriba: la hora, la fecha y la app elegida.
	degradado(arriba, 0, 0, ANCHO, ALTO, C_NEGRO, C_FONDO);
	texto(arriba, &letra_negrita, 6, 2, "Panther OS", C_GRIS);
	bateria(arriba, 232, 4);
	icono_wifi(arriba, 210, 3, C_BLANCO);
	char s[64];
	snprintf(s, sizeof s, "%02d:%02d", hora.tm_hour, hora.tm_min);
	texto_centro(arriba, &letra_reloj, ANCHO / 2, 22, s, C_BLANCO);
	fecha_larga(&hora, s, sizeof s);
	texto_centro(arriba, &letra_normal, ANCHO / 2, 82, s, C_SUAVE);

	const App *a = &apps[elegida];
	redondeado(arriba, 10, 112, 236, 52, 12, C_PANEL, 220);
	redondeado_degradado(arriba, 18, 120, 36, 36, 10, a->c1, a->c2);
	texto(arriba, &letra_titulo, 64, 120, a->nombre, C_BLANCO);
	texto_corto(arriba, &letra_peq, 65, 141, 172, a->descripcion, C_SUAVE);
	ayuda(arriba, "Toca una app, o elígela con la cruceta y A");

	// El icono pequeño de la app elegida, encima de su cuadrado (los iconos son de unos 28 puntos).
	a->icono(arriba, 36, 138);

	// Abajo: las apps y el wifi.
	degradado(abajo, 0, 0, ANCHO, ALTO, C_FONDO, C_NEGRO);
	for (int i = 0; i < N_APPS; i++)
		tile(abajo, i, i == elegida);
	redondeado(abajo, 10, 164, 236, 24, 10, C_PANEL, 256);
	icono_wifi(abajo, 20, 171, C_VIOLETA);
	texto_corto(abajo, &letra_peq, 42, 170, 196, wifi_resumen(), C_SUAVE);
}

bool sistema_paso(Entrada *e, Color *arriba, Color *abajo)
{
	imagenes++;
	ahora_ms = plat_ms();
	plat_hora(&hora);
	procesar_toque(e);

	int r = NADA;
	switch (vista) {
	case V_ARRANQUE: {
		uint32_t t = ahora_ms - inicio_arranque;
		bool saltar = imagenes > 10 && (e->pulsado || e->toque_ini);
		if (t < DURACION_ARRANQUE + QUIETO && !saltar) {
			arranque_dibujar(arriba, abajo, t);
			return true;
		}
		vista = V_INICIO;
		r = CAMBIO;
		break;
	}
	case V_INICIO:
		r = inicio_paso(e);
		break;
	case V_APP:
		r = app_paso(e);
		if (r == SALIR) {
			vista = V_INICIO;
		} else if (r == IR_WIFI) {
			vista_antes_wifi = V_APP;
			vista = V_WIFI;
		}
		break;
	case V_WIFI:
		r = wifi_paso(e);
		if (r == SALIR)
			vista = vista_antes_wifi;
		break;
	}
	if (hora.tm_min != ultimo_minuto) {
		ultimo_minuto = hora.tm_min;
		sucio = true;
	}
	if (r != NADA)
		sucio = true;
	if (!sucio)
		return false;
	sucio = false;
	switch (vista) {
	case V_INICIO:
		inicio_dibujar(arriba, abajo);
		break;
	case V_APP:
		app_dibujar(arriba, abajo);
		break;
	case V_WIFI:
		wifi_dibujar(arriba, abajo);
		break;
	}
	return true;
}
