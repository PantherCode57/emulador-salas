/* El menú de pausa en Pokemulador: las mismas opciones que el de la consola (consola/emulador/menu.c),
 * dibujado como la Pokédex de la app (carcasa roja con la lente y las luces, pantalla crema). Se maneja
 * con la cruz y A, como siempre, y también tocando la opción con el dedo. */
#include "movil.h"

#include <stdio.h>
#include <string.h>

static const struct color CARCASA = {163, 45, 45, 255};
static const struct color HONDO = {80, 19, 19, 255};
static const struct color CREMA = {252, 235, 235, 255};
static const struct color ROJO = {226, 75, 74, 255};
static const struct color BLANCO = {255, 255, 255, 255};
static const struct color APAGADO = {200, 140, 140, 255};
static const struct color SUAVE = {121, 31, 31, 255};
static const struct color CLARO = {247, 193, 193, 255};
static const struct color LENTE = {55, 138, 221, 255};
static const struct color LENTE_LUZ = {181, 212, 244, 255};
static const struct color VERDE = {59, 109, 17, 255};
static const struct color LUCES[3] = {{240, 149, 149, 255}, {250, 199, 117, 255}, {151, 196, 89, 255}};

enum { O_SEGUIR, O_GUARDAR, O_CARGAR, O_VELOCIDAD, O_CARAMELOS, O_REINICIAR, O_SALIR, N_OPCIONES };
static const char *nombres[N_OPCIONES] = {
	"Seguir jugando", "Guardar partida rápida", "Cargar partida rápida", "Velocidad de «Rápido»",
	"Caramelos Raros", "Empezar el juego de nuevo", "Salir del juego",
};
static const int velocidades[] = {2, 3, 4, 5};
#define N_VEL (int)(sizeof(velocidades) / sizeof(velocidades[0]))

static char titulo[160];
static bool hay_partida, sucio, confirmar, con_caramelos, caramelos;
static int sel, factor = 3;
static struct capa_textura tex;
static int vistas[N_OPCIONES], n_vistas;

#define MW 452
#define FILA 40
#define ARRIBA 78                          /* donde empieza la primera opción */
#define MH (ARRIBA + n_vistas * FILA + 54) /* y debajo, la ayuda */
#define MX ((ANCHO - MW) / 2)
#define MY ((ALTO - MH) / 2)

void menu_abrir(const char *t, bool partida, bool caramelos_aqui) {
	snprintf(titulo, sizeof(titulo), "%s", t && *t ? t : "Juego");
	hay_partida = partida;
	con_caramelos = caramelos_aqui;
	caramelos = ajuste_caramelos();
	n_vistas = 0;
	for (int i = 0; i < N_OPCIONES; i++)
		if (i != O_CARAMELOS || con_caramelos)
			vistas[n_vistas++] = i;
	sel = 0;
	confirmar = false;
	factor = ajuste_factor_rapido();
	sucio = true;
	float x, y;
	mandos_toque(&x, &y); /* los toques de antes no cuentan */
}

static bool activa(int i) { return i != O_CARGAR || hay_partida; }

static void mover(int d) {
	int i = sel;
	do
		i = (i + d + n_vistas) % n_vistas;
	while (!activa(vistas[i]));
	sel = i;
	confirmar = false;
}

/* La opción que se ha tocado con el dedo (o -1). */
static int tocada(void) {
	float x, y;
	if (!mandos_toque(&x, &y))
		return -1;
	int mx, my;
	video_a_marco(x, y, &mx, &my);
	mx -= MX, my -= MY + ARRIBA;
	if (mx < 0 || mx >= MW || my < 0 || my >= n_vistas * FILA)
		return -1;
	int k = my / FILA;
	return activa(vistas[k]) ? k : -1;
}

enum accion_menu menu_pulsar(uint32_t nuevos, int *factor_rapido) {
	int k = tocada();
	if (k >= 0) {
		if (k != sel)
			confirmar = false;
		sel = k;
		nuevos = BOT_A;
	}
	if (!nuevos)
		return MENU_NADA;
	sucio = true;
	if (nuevos & BOT_ARRIBA)
		mover(-1);
	if (nuevos & BOT_ABAJO)
		mover(1);
	int op = vistas[sel];
	if (op == O_CARAMELOS && (nuevos & (BOT_IZQ | BOT_DER | BOT_A))) {
		caramelos = (nuevos & BOT_A) ? !caramelos : (nuevos & BOT_DER) != 0;
		ajuste_guardar_caramelos(caramelos);
		return MENU_NADA;
	}
	if (op == O_VELOCIDAD && (nuevos & (BOT_IZQ | BOT_DER | BOT_A))) {
		int v = 0;
		while (v < N_VEL && velocidades[v] != factor)
			v++;
		if (nuevos & BOT_IZQ)
			v = v <= 0 ? 0 : v - 1;
		else
			v = v >= N_VEL - 1 ? (nuevos & BOT_A ? 0 : N_VEL - 1) : v + 1;
		factor = velocidades[v];
		ajuste_guardar_factor(factor);
		*factor_rapido = factor;
		return MENU_NADA;
	}
	if (nuevos & (BOT_B | BOT_START))
		return MENU_SEGUIR;
	if (nuevos & BOT_A) {
		switch (op) {
		case O_SEGUIR: return MENU_SEGUIR;
		case O_GUARDAR: return MENU_GUARDAR;
		case O_CARGAR: return MENU_CARGAR;
		case O_REINICIAR:
			if (!confirmar) { /* empezar de nuevo pierde lo no guardado: se pregunta */
				confirmar = true;
				return MENU_NADA;
			}
			return MENU_REINICIAR;
		case O_SALIR: return MENU_SALIR;
		}
	}
	return MENU_NADA;
}

static void circulo(struct lienzo *l, int cx, int cy, int r, struct color c) {
	lienzo_redondeado(l, cx - r, cy - r, 2 * r, 2 * r, r, c);
}

static void pintar(void) {
	struct lienzo l;
	if (!lienzo_nuevo(&l, MW, MH))
		return;
	/* La carcasa, con la lente, las luces y el nombre del juego. */
	lienzo_redondeado(&l, 0, 0, MW, MH, 20, HONDO);
	lienzo_redondeado(&l, 3, 3, MW - 6, MH - 6, 17, CARCASA);
	circulo(&l, 38, 35, 22, HONDO);
	circulo(&l, 38, 35, 20, CREMA);
	circulo(&l, 38, 35, 16, LENTE);
	circulo(&l, 33, 30, 4, LENTE_LUZ);
	for (int i = 0; i < 3; i++) {
		circulo(&l, 76 + i * 17, 20, 6, HONDO);
		circulo(&l, 76 + i * 17, 20, 5, LUCES[i]);
	}
	texto_recortado(&l, 70, 52, MW - 70 - 20, LETRA_NEGRITA, 20, CREMA, titulo);
	/* La pantalla. */
	int py = ARRIBA - 10, ph = n_vistas * FILA + 16;
	lienzo_redondeado(&l, 12, py, MW - 24, ph, 14, HONDO);
	lienzo_redondeado(&l, 14, py + 2, MW - 28, ph - 4, 12, CREMA);
	for (int k = 0; k < n_vistas; k++) {
		int i = vistas[k];
		int y = ARRIBA + k * FILA;
		bool aqui = k == sel;
		if (aqui)
			lienzo_redondeado(&l, 20, y, MW - 40, FILA - 4, 10, ROJO);
		struct color c = !activa(i) ? APAGADO : aqui ? BLANCO : HONDO;
		const char *nombre = nombres[i];
		if (i == O_REINICIAR && confirmar)
			nombre = "Otra vez para empezar de cero";
		texto(&l, 34, y + 25, aqui ? LETRA_NEGRITA : LETRA_NORMAL, 18, c, nombre);
		if (i == O_VELOCIDAD) {
			char v[24];
			snprintf(v, sizeof(v), "‹  ×%d  ›", factor);
			texto(&l, MW - 34 - texto_ancho(LETRA_NEGRITA, 18, v), y + 25, LETRA_NEGRITA, 18, aqui ? BLANCO : SUAVE, v);
		}
		if (i == O_CARAMELOS) {
			const char *v = caramelos ? "Sí" : "No";
			texto(&l, MW - 34 - texto_ancho(LETRA_NEGRITA, 18, v), y + 25, LETRA_NEGRITA, 18,
			      aqui ? BLANCO : caramelos ? VERDE : SUAVE, v);
		}
		if (i == O_CARGAR && !hay_partida) {
			const char *n = "no hay";
			texto(&l, MW - 34 - texto_ancho(LETRA_NORMAL, 15, n), y + 25, LETRA_NORMAL, 15, APAGADO, n);
		}
	}
	texto(&l, 24, MH - 17, LETRA_NORMAL, 15, CLARO, "Toca una opción, o A para elegir y B para seguir");
	textura_subir(&tex, &l);
	lienzo_libre(&l);
	sucio = false;
}

void menu_dibujar(void) {
	if (sucio || !tex.tex)
		pintar();
	video_oscurecer(130);
	textura_dibujar(&tex, MX, MY, MW, MH, 255);
}
