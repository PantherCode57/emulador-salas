/* Dibujar en memoria (RGBA, alfa sin premultiplicar) para el menú y los avisos, con la letra del tema
 * de Panther (Selawik) y stb_truetype (de dominio público, en terceros/). */
#include "panther.h"

#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define STB_TRUETYPE_IMPLEMENTATION
#define STBTT_STATIC
#pragma GCC diagnostic push
#pragma GCC diagnostic ignored "-Wunused-function"
#include "terceros/stb_truetype.h"
#pragma GCC diagnostic pop

bool lienzo_nuevo(struct lienzo *l, int w, int h) {
	l->px = calloc((size_t)w * h, 4);
	l->w = w;
	l->h = h;
	return l->px != NULL;
}

void lienzo_libre(struct lienzo *l) {
	free(l->px);
	l->px = NULL;
}

void lienzo_borrar(struct lienzo *l, struct color c) {
	for (int i = 0; i < l->w * l->h; i++) {
		uint8_t *p = l->px + i * 4;
		p[0] = c.r, p[1] = c.g, p[2] = c.b, p[3] = c.a;
	}
}

/* Pone el color c encima del píxel con una cobertura de 0 a 255. */
static inline void mezclar(struct lienzo *l, int x, int y, struct color c, int cobertura) {
	if (x < 0 || y < 0 || x >= l->w || y >= l->h)
		return;
	int sa = c.a * cobertura / 255;
	if (sa <= 0)
		return;
	uint8_t *d = l->px + ((size_t)y * l->w + x) * 4;
	int da = d[3];
	int fuera = da * (255 - sa) / 255;
	int oa = sa + fuera;
	if (oa <= 0)
		return;
	d[0] = (uint8_t)((c.r * sa + d[0] * fuera) / oa);
	d[1] = (uint8_t)((c.g * sa + d[1] * fuera) / oa);
	d[2] = (uint8_t)((c.b * sa + d[2] * fuera) / oa);
	d[3] = (uint8_t)oa;
}

void lienzo_redondeado(struct lienzo *l, int x, int y, int w, int h, int radio, struct color c) {
	if (radio * 2 > w)
		radio = w / 2;
	if (radio * 2 > h)
		radio = h / 2;
	for (int j = 0; j < h; j++) {
		for (int i = 0; i < w; i++) {
			/* distancia a la esquina redonda más cercana (si está en una esquina) */
			float cx = i < radio ? radio - 0.5f : (i >= w - radio ? w - radio - 0.5f : i);
			float cy = j < radio ? radio - 0.5f : (j >= h - radio ? h - radio - 0.5f : j);
			float dx = i - cx, dy = j - cy;
			float d = sqrtf(dx * dx + dy * dy);
			float cob = radio > 0 ? radio - d + 0.5f : 1.0f;
			if (cob <= 0)
				continue;
			mezclar(l, x + i, y + j, c, cob >= 1 ? 255 : (int)(cob * 255));
		}
	}
}

/* Triángulo que apunta a la derecha (para el icono de ir rápido), suavizado con 4×4 muestras. */
void lienzo_triangulo_der(struct lienzo *l, int x, int y, int w, int h, struct color c) {
	for (int j = 0; j < h; j++) {
		for (int i = 0; i < w; i++) {
			int dentro = 0;
			for (int sj = 0; sj < 4; sj++) {
				for (int si = 0; si < 4; si++) {
					float px = i + (si + 0.5f) / 4, py = j + (sj + 0.5f) / 4;
					float medio = h / 2.0f;
					float alto_aqui = medio * (1 - px / w);
					if (py >= medio - alto_aqui && py <= medio + alto_aqui)
						dentro++;
				}
			}
			if (dentro)
				mezclar(l, x + i, y + j, c, dentro * 255 / 16);
		}
	}
}

/* ---- Letras ---- */

static stbtt_fontinfo letras[2];
static unsigned char *datos_letra[2];
static bool hay_letra[2];

static unsigned char *leer_entero(const char *ruta) {
	FILE *f = fopen(ruta, "rb");
	if (!f)
		return NULL;
	fseek(f, 0, SEEK_END);
	long n = ftell(f);
	fseek(f, 0, SEEK_SET);
	unsigned char *b = n > 0 ? malloc((size_t)n) : NULL;
	if (b && fread(b, 1, (size_t)n, f) != (size_t)n) {
		free(b);
		b = NULL;
	}
	fclose(f);
	return b;
}

extern const char *carpeta_programa;

bool letras_cargar(void) {
	static const char *nombres[2][3] = {
		{"selawk.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", NULL},
		{"selawksb.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", NULL},
	};
	for (int f = 0; f < 2; f++) {
		for (int k = 0; nombres[f][k] && !hay_letra[f]; k++) {
			char ruta[512];
			if (nombres[f][k][0] == '/')
				snprintf(ruta, sizeof(ruta), "%s", nombres[f][k]);
			else
				snprintf(ruta, sizeof(ruta), "%s/%s", carpeta_programa, nombres[f][k]);
			unsigned char *b = leer_entero(ruta);
			if (b && stbtt_InitFont(&letras[f], b, stbtt_GetFontOffsetForIndex(b, 0))) {
				datos_letra[f] = b;
				hay_letra[f] = true;
			} else {
				free(b);
			}
		}
	}
	if (!hay_letra[LETRA_NEGRITA] && hay_letra[LETRA_NORMAL]) {
		letras[LETRA_NEGRITA] = letras[LETRA_NORMAL];
		hay_letra[LETRA_NEGRITA] = true;
	}
	if (!hay_letra[LETRA_NORMAL])
		registro("no encuentro la letra (selawk.ttf): el menú saldrá sin texto");
	return hay_letra[LETRA_NORMAL];
}

/* Siguiente carácter de un texto UTF-8. */
static int siguiente(const char **s) {
	const unsigned char *p = (const unsigned char *)*s;
	int c = *p;
	int n = 1;
	if (c >= 0xf0 && p[1] && p[2] && p[3]) {
		c = ((c & 7) << 18) | ((p[1] & 63) << 12) | ((p[2] & 63) << 6) | (p[3] & 63);
		n = 4;
	} else if (c >= 0xe0 && p[1] && p[2]) {
		c = ((c & 15) << 12) | ((p[1] & 63) << 6) | (p[2] & 63);
		n = 3;
	} else if (c >= 0xc0 && p[1]) {
		c = ((c & 31) << 6) | (p[1] & 63);
		n = 2;
	}
	*s += n;
	return c;
}

static float escala_de(enum letra f, int tam) { return stbtt_ScaleForPixelHeight(&letras[f], (float)tam); }

int texto_ancho(enum letra f, int tam, const char *s) {
	if (!hay_letra[f])
		return 0;
	float esc = escala_de(f, tam), x = 0;
	int antes = 0;
	while (*s) {
		int c = siguiente(&s);
		int avance, izq;
		stbtt_GetCodepointHMetrics(&letras[f], c, &avance, &izq);
		if (antes)
			x += esc * stbtt_GetCodepointKernAdvance(&letras[f], antes, c);
		x += esc * avance;
		antes = c;
	}
	return (int)ceilf(x);
}

int texto(struct lienzo *l, int x, int y, enum letra f, int tam, struct color c, const char *s) {
	if (!hay_letra[f])
		return x;
	float esc = escala_de(f, tam), pos = (float)x;
	int antes = 0;
	while (*s) {
		int cp = siguiente(&s);
		if (antes)
			pos += esc * stbtt_GetCodepointKernAdvance(&letras[f], antes, cp);
		int avance, izq;
		stbtt_GetCodepointHMetrics(&letras[f], cp, &avance, &izq);
		float dx = pos - floorf(pos);
		int x0, y0, x1, y1;
		stbtt_GetCodepointBitmapBoxSubpixel(&letras[f], cp, esc, esc, dx, 0, &x0, &y0, &x1, &y1);
		int bw = x1 - x0, bh = y1 - y0;
		if (bw > 0 && bh > 0 && bw < 256 && bh < 256) {
			unsigned char bm[256 * 256];
			stbtt_MakeCodepointBitmapSubpixel(&letras[f], bm, bw, bh, bw, esc, esc, dx, 0, cp);
			int ox = (int)floorf(pos) + x0, oy = y + y0;
			for (int j = 0; j < bh; j++)
				for (int i = 0; i < bw; i++)
					if (bm[j * bw + i])
						mezclar(l, ox + i, oy + j, c, bm[j * bw + i]);
		}
		pos += esc * avance;
		antes = cp;
	}
	return (int)ceilf(pos);
}

void texto_recortado(struct lienzo *l, int x, int y, int ancho_max, enum letra f, int tam, struct color c, const char *s) {
	if (texto_ancho(f, tam, s) <= ancho_max) {
		texto(l, x, y, f, tam, c, s);
		return;
	}
	char buf[512];
	size_t n = strlen(s);
	if (n > sizeof(buf) - 4)
		n = sizeof(buf) - 4;
	memcpy(buf, s, n);
	while (n > 0) {
		/* quitar un carácter entero (sin partir los de UTF-8) */
		do
			n--;
		while (n > 0 && ((unsigned char)buf[n] & 0xc0) == 0x80);
		memcpy(buf + n, "…", 4);
		if (texto_ancho(f, tam, buf) <= ancho_max)
			break;
	}
	texto(l, x, y, f, tam, c, buf);
}
