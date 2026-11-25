/* Los avisos del emulador («Partida rápida guardada», «Velocidad ×3»…) con los colores de Pokemulador.
 * Son los de consola/emulador/avisos.c. */
#include "panther.h"

#include <stdio.h>
#include <string.h>

static const struct color FONDO = {80, 19, 19, 255};
static const struct color CREMA = {252, 235, 235, 255};
static const struct color AMARILLO = {250, 199, 117, 255};

static struct capa_textura tex_aviso, tex_rapido;
static uint64_t aviso_desde;
static int rapido_hecho;

#define DURA 1600

static void pastilla(struct lienzo *l, int w, int h) {
	lienzo_redondeado(l, 0, 0, w, h, h / 2, CREMA);
	lienzo_redondeado(l, 2, 2, w - 4, h - 4, h / 2 - 2, FONDO);
}

void aviso(const char *s) {
	int tam = 19;
	int w = texto_ancho(LETRA_NORMAL, tam, s) + 44, h = 42;
	if (w > ANCHO - 40)
		w = ANCHO - 40;
	struct lienzo l;
	if (!lienzo_nuevo(&l, w, h))
		return;
	pastilla(&l, w, h);
	texto_recortado(&l, 22, 28, w - 44, LETRA_NORMAL, tam, CREMA, s);
	textura_subir(&tex_aviso, &l);
	lienzo_libre(&l);
	aviso_desde = ahora_ms();
}

static void dibujar_rapido(int factor) {
	char s[16];
	snprintf(s, sizeof(s), "×%d", factor);
	int tam = 18;
	int w = 16 + 26 + 6 + texto_ancho(LETRA_NEGRITA, tam, s) + 14, h = 34;
	struct lienzo l;
	if (!lienzo_nuevo(&l, w, h))
		return;
	pastilla(&l, w, h);
	lienzo_triangulo_der(&l, 15, 10, 13, 14, AMARILLO);
	lienzo_triangulo_der(&l, 27, 10, 13, 14, AMARILLO);
	texto(&l, 16 + 26 + 6, 23, LETRA_NEGRITA, tam, CREMA, s);
	textura_subir(&tex_rapido, &l);
	lienzo_libre(&l);
	rapido_hecho = factor;
}

void avisos_dibujar(bool rapido, int factor) {
	if (rapido) {
		if (rapido_hecho != factor)
			dibujar_rapido(factor);
		textura_dibujar(&tex_rapido, ANCHO - 14 - tex_rapido.w, 14, tex_rapido.w, tex_rapido.h, 255);
	}
	if (!tex_aviso.tex || !aviso_desde)
		return;
	uint64_t e = ahora_ms() - aviso_desde;
	if (e > DURA) {
		aviso_desde = 0;
		return;
	}
	int alfa = 255;
	if (e < 150)
		alfa = (int)(255 * e / 150);
	else if (e > DURA - 300)
		alfa = (int)(255 * (DURA - e) / 300);
	textura_dibujar(&tex_aviso, (ANCHO - tex_aviso.w) / 2, ALTO - 30 - tex_aviso.h, tex_aviso.w, tex_aviso.h, (uint8_t)alfa);
}
