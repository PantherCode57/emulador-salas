/* Los mensajitos del propio emulador: «Partida guardada», «Velocidad ×3»… (abajo, un momento) y el
 * indicador de ir rápido (arriba a la derecha, mientras dura). */
#include "panther.h"

#include <stdio.h>
#include <string.h>

static const struct color FONDO = {18, 14, 30, 225};
static const struct color BLANCO = {245, 243, 250, 255};
static const struct color VIOLETA = {167, 139, 250, 255};

static struct capa_textura tex_aviso, tex_rapido;
static uint64_t aviso_desde;
static int rapido_hecho;

#define DURA 1600

void aviso(const char *s) {
	int tam = 19;
	int w = texto_ancho(LETRA_NORMAL, tam, s) + 40, h = 40;
	if (w > ANCHO - 40)
		w = ANCHO - 40;
	struct lienzo l;
	if (!lienzo_nuevo(&l, w, h))
		return;
	lienzo_redondeado(&l, 0, 0, w, h, h / 2, FONDO);
	texto_recortado(&l, 20, 27, w - 40, LETRA_NORMAL, tam, BLANCO, s);
	textura_subir(&tex_aviso, &l);
	lienzo_libre(&l);
	aviso_desde = ahora_ms();
}

static void dibujar_rapido(int factor) {
	char s[16];
	snprintf(s, sizeof(s), "×%d", factor);
	int tam = 18;
	int w = 16 + 26 + 6 + texto_ancho(LETRA_NEGRITA, tam, s) + 14, h = 32;
	struct lienzo l;
	if (!lienzo_nuevo(&l, w, h))
		return;
	lienzo_redondeado(&l, 0, 0, w, h, h / 2, FONDO);
	lienzo_triangulo_der(&l, 15, 9, 13, 14, VIOLETA);
	lienzo_triangulo_der(&l, 27, 9, 13, 14, VIOLETA);
	texto(&l, 16 + 26 + 6, 22, LETRA_NEGRITA, tam, BLANCO, s);
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
