/* El menú de pausa del emulador (SELECT + START a la vez). */
#include "panther.h"

#include <stdio.h>
#include <string.h>

static const struct color FONDO = {20, 16, 32, 246};
static const struct color BLANCO = {245, 243, 250, 255};
static const struct color SUAVE = {164, 158, 186, 255};
static const struct color APAGADO = {96, 90, 116, 255};
static const struct color VIOLETA = {124, 77, 255, 255};
static const struct color LINEA = {255, 255, 255, 22};

enum { O_SEGUIR, O_GUARDAR, O_CARGAR, O_VELOCIDAD, O_CARAMELOS, O_REINICIAR, O_SALIR, N_OPCIONES };
static const char *nombres[N_OPCIONES] = {
	"Seguir jugando", "Guardar partida rápida", "Cargar partida rápida", "Velocidad con R2",
	"Caramelos Raros mod", "Empezar el juego de nuevo", "Salir del juego",
};
static const int velocidades[] = {2, 3, 4, 5};
#define N_VEL (int)(sizeof(velocidades) / sizeof(velocidades[0]))

static char titulo[160];
static bool hay_partida, sucio, confirmar, con_caramelos, caramelos;
static int sel, factor = 3;
static struct capa_textura tex;
/* Las opciones que salen (la de los Caramelos, solo en los juegos donde valen). */
static int vistas[N_OPCIONES], n_vistas;

#define MW 400
#define FILA 42
#define ARRIBA 74
#define MH (ARRIBA + n_vistas * FILA + 52)

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

enum accion_menu menu_pulsar(uint32_t nuevos, int *factor_rapido) {
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
		int k = 0;
		while (k < N_VEL && velocidades[k] != factor)
			k++;
		if (nuevos & BOT_IZQ)
			k = k <= 0 ? 0 : k - 1;
		else
			k = k >= N_VEL - 1 ? (nuevos & BOT_A ? 0 : N_VEL - 1) : k + 1;
		factor = velocidades[k];
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
		case O_SALIR:
			if (op == O_REINICIAR && !confirmar) { /* empezar de nuevo pierde lo no guardado: se pregunta */
				confirmar = true;
				return MENU_NADA;
			}
			return op == O_SALIR ? MENU_SALIR : MENU_REINICIAR;
		}
	}
	return MENU_NADA;
}

static void pintar(void) {
	struct lienzo l;
	if (!lienzo_nuevo(&l, MW, MH))
		return;
	lienzo_redondeado(&l, 0, 0, MW, MH, 18, FONDO);
	lienzo_redondeado(&l, 24, 56, MW - 48, 2, 1, VIOLETA);
	texto_recortado(&l, 24, 40, MW - 48, LETRA_NEGRITA, 22, BLANCO, titulo);
	for (int k = 0; k < n_vistas; k++) {
		int i = vistas[k];
		int y = ARRIBA + k * FILA;
		if (k == sel)
			lienzo_redondeado(&l, 12, y, MW - 24, FILA - 4, (FILA - 4) / 2, VIOLETA);
		struct color c = !activa(i) ? APAGADO : BLANCO;
		const char *nombre = nombres[i];
		if (i == O_REINICIAR && confirmar)
			nombre = "Pulsa A otra vez para empezar";
		texto(&l, 30, y + 26, LETRA_NORMAL, 19, c, nombre);
		if (i == O_VELOCIDAD) {
			char v[24];
			snprintf(v, sizeof(v), "‹  ×%d  ›", factor);
			int vw = texto_ancho(LETRA_NEGRITA, 19, v);
			texto(&l, MW - 30 - vw, y + 26, LETRA_NEGRITA, 19, vistas[sel] == i ? BLANCO : SUAVE, v);
		}
		if (i == O_CARAMELOS) {
			const char *v = caramelos ? "‹  ON  ›" : "‹  OFF  ›";
			int vw = texto_ancho(LETRA_NEGRITA, 19, v);
			struct color cv = vistas[sel] == i ? BLANCO : caramelos ? (struct color){120, 220, 140, 255} : SUAVE;
			texto(&l, MW - 30 - vw, y + 26, LETRA_NEGRITA, 19, cv, v);
		}
		if (i == O_CARGAR && !hay_partida) {
			const char *n = "no hay";
			texto(&l, MW - 30 - texto_ancho(LETRA_NORMAL, 16, n), y + 26, LETRA_NORMAL, 16, APAGADO, n);
		}
	}
	lienzo_redondeado(&l, 24, MH - 44, MW - 48, 1, 0, LINEA);
	texto(&l, 28, MH - 17, LETRA_NORMAL, 16, SUAVE, "A  elegir      B  seguir jugando");
	textura_subir(&tex, &l);
	lienzo_libre(&l);
	sucio = false;
}

void menu_dibujar(void) {
	if (sucio || !tex.tex)
		pintar();
	video_oscurecer(150);
	textura_dibujar(&tex, (ANCHO - MW) / 2, (ALTO - MH) / 2, MW, MH, 255);
}
