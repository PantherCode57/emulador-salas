/* Los botones de la pantalla: dónde va cada uno, cuál tocas y cómo se dibujan (con los colores del
 * menú de Panther: fondo morado oscuro y el violeta de la consola).
 *
 *  De pie:  el juego arriba (4:3, de lado a lado) y debajo, como en una Game Boy:
 *           L  [Rápido] [Menú]  R  /  la cruz y A, B  /  SELECT  START
 *  Tumbado: el juego en medio; a la izquierda L, Menú, la cruz y SELECT; a la derecha R, Rápido,
 *           A, B y START. Si no hay sitio a los lados (una tablet), van encima, transparentes.
 */
#include "movil.h"

#include <math.h>
#include <stdlib.h>
#include <string.h>

enum forma { PASTILLA, CIRCULO, CRUZ };
enum { C_CRUZ, C_A, C_B, C_L, C_R, C_SELECT, C_START, C_RAPIDO, C_MENU, N_C };

struct control {
	enum forma forma;
	uint32_t bits;
	const char *texto;
	bool grande; /* letra grande y en negrita */
	SDL_Rect r;
	SDL_Texture *normal, *pulsado;
};

static struct control c[N_C] = {
	[C_CRUZ] = {CRUZ, BOT_ARRIBA | BOT_ABAJO | BOT_IZQ | BOT_DER, NULL, false, {0}, NULL, NULL},
	[C_A] = {CIRCULO, BOT_A, "A", true, {0}, NULL, NULL},
	[C_B] = {CIRCULO, BOT_B, "B", true, {0}, NULL, NULL},
	[C_L] = {PASTILLA, BOT_L, "L", true, {0}, NULL, NULL},
	[C_R] = {PASTILLA, BOT_R, "R", true, {0}, NULL, NULL},
	[C_SELECT] = {PASTILLA, BOT_SELECT, "SELECT", false, {0}, NULL, NULL},
	[C_START] = {PASTILLA, BOT_START, "START", false, {0}, NULL, NULL},
	[C_RAPIDO] = {PASTILLA, BOT_R2, "Rápido", false, {0}, NULL, NULL},
	[C_MENU] = {PASTILLA, BOT_SELECT | BOT_START | BOT_MENU_PANTALLA, "Menú", false, {0}, NULL, NULL},
};

static SDL_Texture *brazo; /* el brazo de la cruz encendido (hacia arriba; se gira para los demás) */
static int brazo_w, brazo_h;
static SDL_mutex *cerrojo;
static int pan_w, pan_h;
static SDL_Rect zona;
static bool encima; /* van encima del juego */
volatile int tactil_visible = 1;

static const struct color BOTON = {40, 33, 60, 255};
static const struct color BOTON_PULSADO = {124, 77, 255, 255};
static const struct color VIOLETA = {124, 77, 255, 255};
static const struct color VIOLETA_CLARO = {167, 139, 250, 255};
static const struct color BLANCO = {245, 243, 250, 255};
static const struct color TEXTO = {222, 216, 238, 255};
static const struct color TEXTO_SUAVE = {164, 158, 186, 255};
static const struct color FLECHA = {120, 112, 146, 255};

/* ---- Dónde va cada cosa ---- */

static void pon(int i, int cx, int cy, int w, int h) { c[i].r = (SDL_Rect){cx - w / 2, cy - h / 2, w, h}; }

/* Los botones en dos columnas (tumbado o encima): xl y xr son el centro de cada una; u, el alto. */
static void columnas(int h, float u, int xl, int xr, float col) {
	int fh = (int)(0.095f * u), lw = (int)(0.28f * u);
	int y_l = (int)(0.045f * u) + fh / 2;
	pon(C_L, xl, y_l, lw, fh);
	pon(C_R, xr, y_l, lw, fh);
	int ph = (int)(0.075f * u), pw = (int)(0.21f * u);
	int y_menu = y_l + fh / 2 + (int)(0.04f * u) + ph / 2;
	pon(C_MENU, xl, y_menu, pw, ph);
	pon(C_RAPIDO, xr, y_menu, pw, ph);
	int y_abajo = h - (int)(0.05f * u) - ph / 2;
	pon(C_SELECT, xl, y_abajo, pw, ph);
	pon(C_START, xr, y_abajo, pw, ph);
	float r = 0.17f * u;
	if (r > col * 0.47f)
		r = col * 0.47f;
	int cy = (int)(0.57f * u);
	pon(C_CRUZ, xl, cy, (int)(2 * r), (int)(2 * r));
	float rb = r * 0.52f;
	pon(C_A, xr + (int)(1.05f * rb), cy - (int)(0.36f * r), (int)(2 * rb), (int)(2 * rb));
	pon(C_B, xr - (int)(1.05f * rb), cy + (int)(0.36f * r), (int)(2 * rb), (int)(2 * rb));
}

static void encima_de_todo(int w, int h) {
	float u = (float)(w < h ? w : h);
	int lado = recorte_pantalla > (int)(0.03f * w) ? recorte_pantalla : (int)(0.03f * w);
	zona = (SDL_Rect){0, 0, w, h};
	float col = 0.4f * u;
	columnas(h, u, lado + (int)(col / 2), w - lado - (int)(col / 2), col);
	encima = true;
}

static void tumbado(int w, int h) {
	float u = (float)h;
	int lado = recorte_pantalla > (int)(0.03f * w) ? recorte_pantalla : (int)(0.03f * w);
	int col = (int)(0.38f * u);
	int libre = w - 2 * (lado + col);
	if (libre < (int)(1.15f * u)) {
		encima_de_todo(w, h);
		return;
	}
	int zw = libre < (int)(1.5f * u) ? libre : (int)(1.5f * u);
	zona = (SDL_Rect){(w - zw) / 2, 0, zw, h};
	int xl = (lado + zona.x) / 2;
	columnas(h, u, xl, w - xl, (float)(zona.x - lado));
	encima = false;
}

static void de_pie(int w, int h) {
	if (h < w * 3 / 2) { /* casi cuadrada: no caben debajo */
		encima_de_todo(w, h);
		return;
	}
	float u = (float)w;
	int arriba = recorte_pantalla > (int)(0.03f * h) ? recorte_pantalla : (int)(0.03f * h);
	zona = (SDL_Rect){0, arriba, w, w * 3 / 4};
	/* Los botones van juntos abajo, como un mando (donde llegan los pulgares); lo que sobra de
	 * pantalla queda entre el juego y ellos. De abajo arriba: SELECT y START, la cruz y A, B, y L, R. */
	int y0 = zona.y + zona.h + (int)(0.04f * u);
	int abajo = h - (int)(0.03f * h);
	int fh = (int)(0.095f * u), lw = (int)(0.25f * u);
	int ph = (int)(0.075f * u), pw = (int)(0.18f * u);
	int hueco = (int)(0.07f * u);
	float r = 0.2f * u;
	int cabe = (abajo - ph - hueco - hueco - fh - y0) / 2;
	if (r > cabe)
		r = (float)cabe;
	int ymed = abajo - ph - hueco - (int)r;
	int y_l = ymed - (int)r - hueco - fh / 2;
	pon(C_L, (int)(0.04f * u) + lw / 2, y_l, lw, fh);
	pon(C_R, w - (int)(0.04f * u) - lw / 2, y_l, lw, fh);
	pon(C_RAPIDO, w / 2 - (int)(0.015f * u) - pw / 2, y_l, pw, ph);
	pon(C_MENU, w / 2 + (int)(0.015f * u) + pw / 2, y_l, pw, ph);
	int sw = (int)(0.2f * u);
	pon(C_SELECT, w / 2 - (int)(0.025f * u) - sw / 2, abajo - ph / 2, sw, ph);
	pon(C_START, w / 2 + (int)(0.025f * u) + sw / 2, abajo - ph / 2, sw, ph);
	pon(C_CRUZ, (int)(0.07f * u + r), ymed, (int)(2 * r), (int)(2 * r));
	float rb = r * 0.52f;
	int ax = w - (int)(0.07f * u + rb);
	pon(C_A, ax, ymed - (int)(0.36f * r), (int)(2 * rb), (int)(2 * rb));
	pon(C_B, ax - (int)(2.25f * rb), ymed + (int)(0.36f * r), (int)(2 * rb), (int)(2 * rb));
	encima = false;
}

void tactil_disponer(int w, int h, SDL_Rect *juego) {
	if (!cerrojo)
		cerrojo = SDL_CreateMutex();
	if (w != pan_w || h != pan_h) {
		SDL_LockMutex(cerrojo);
		pan_w = w, pan_h = h;
		if (h >= w)
			de_pie(w, h);
		else
			tumbado(w, h);
		SDL_UnlockMutex(cerrojo);
		tactil_olvidar();
		/* (la prueba del emulador de Android lee de aquí dónde tiene que tocar) */
		static const char *nombres[N_C] = {"cruz", "A", "B", "L", "R", "SELECT", "START", "Rapido", "Menu"};
		for (int i = 0; i < N_C; i++)
			registro("botón %s: %d %d %d %d", nombres[i], c[i].r.x, c[i].r.y, c[i].r.w, c[i].r.h);
	}
	*juego = zona;
}

/* ---- Qué se toca ---- */

static float distancia(float x, float y, const SDL_Rect *r) {
	float cx = r->x + r->w / 2.0f, cy = r->y + r->h / 2.0f;
	return sqrtf((x - cx) * (x - cx) + (y - cy) * (y - cy));
}

static uint32_t direcciones(float x, float y, float alcance) {
	const SDL_Rect *r = &c[C_CRUZ].r;
	float rad = r->w / 2.0f;
	float dx = x - (r->x + rad), dy = y - (r->y + rad);
	float d = sqrtf(dx * dx + dy * dy);
	if (rad <= 0 || d > rad * alcance || d < rad * 0.12f)
		return 0;
	uint32_t b = 0;
	/* 8 direcciones: las diagonales son los 45° de en medio */
	if (fabsf(dx) > fabsf(dy) * 0.414f)
		b |= dx < 0 ? BOT_IZQ : BOT_DER;
	if (fabsf(dy) > fabsf(dx) * 0.414f)
		b |= dy < 0 ? BOT_ARRIBA : BOT_ABAJO;
	return b;
}

uint32_t tactil_botones(float fx, float fy) {
	if (!cerrojo)
		return 0;
	SDL_LockMutex(cerrojo);
	float x = fx * pan_w, y = fy * pan_h;
	uint32_t b = direcciones(x, y, 1.0f);
	if (!b) {
		/* El botón más cercano, con un margen alrededor para no tener que acertar justo. */
		int mejor = -1;
		float mejor_d = 1e9f;
		for (int i = 0; i < N_C; i++) {
			const SDL_Rect *r = &c[i].r;
			if (i == C_CRUZ || r->w <= 0)
				continue;
			float m = 0.28f * (r->w < r->h ? r->w : r->h);
			bool dentro;
			if (c[i].forma == CIRCULO)
				dentro = distancia(x, y, r) <= r->w / 2.0f + m;
			else
				dentro = x >= r->x - m && x <= r->x + r->w + m && y >= r->y - m && y <= r->y + r->h + m;
			float d = distancia(x, y, r);
			if (dentro && d < mejor_d)
				mejor = i, mejor_d = d;
		}
		if (mejor >= 0)
			b = c[mejor].bits;
		else
			b = direcciones(x, y, 1.45f);
	}
	SDL_UnlockMutex(cerrojo);
	return b;
}

/* ---- Dibujarlos ---- */

static void punto(struct lienzo *l, int x, int y, struct color col, int cob) {
	if (x < 0 || y < 0 || x >= l->pw || y >= l->ph)
		return;
	int sa = col.a * cob / 255;
	if (sa <= 0)
		return;
	uint8_t *d = l->px + ((size_t)y * l->pw + x) * 4;
	int fuera = d[3] * (255 - sa) / 255, oa = sa + fuera;
	d[0] = (uint8_t)((col.r * sa + d[0] * fuera) / oa);
	d[1] = (uint8_t)((col.g * sa + d[1] * fuera) / oa);
	d[2] = (uint8_t)((col.b * sa + d[2] * fuera) / oa);
	d[3] = (uint8_t)oa;
}

/* Triángulo con la punta hacia dir (0 arriba, 1 abajo, 2 izquierda, 3 derecha), suavizado. */
static void flecha(struct lienzo *l, float cx, float cy, float t, int dir, struct color col) {
	float px[3] = {0, -0.62f, 0.62f}, py[3] = {-0.5f, 0.32f, 0.32f};
	float vx[3], vy[3];
	for (int i = 0; i < 3; i++) {
		float a = px[i] * t, b = py[i] * t;
		switch (dir) {
		case 0: vx[i] = cx + a, vy[i] = cy + b; break;
		case 1: vx[i] = cx + a, vy[i] = cy - b; break;
		case 2: vx[i] = cx + b, vy[i] = cy + a; break;
		default: vx[i] = cx - b, vy[i] = cy + a; break;
		}
	}
	int x0 = (int)floorf(cx - t), x1 = (int)ceilf(cx + t), y0 = (int)floorf(cy - t), y1 = (int)ceilf(cy + t);
	for (int y = y0; y <= y1; y++) {
		for (int x = x0; x <= x1; x++) {
			int dentro = 0;
			for (int s = 0; s < 16; s++) {
				float sx = x + ((s & 3) + 0.5f) / 4, sy = y + ((s >> 2) + 0.5f) / 4;
				float d0 = (vx[1] - vx[0]) * (sy - vy[0]) - (vy[1] - vy[0]) * (sx - vx[0]);
				float d1 = (vx[2] - vx[1]) * (sy - vy[1]) - (vy[2] - vy[1]) * (sx - vx[1]);
				float d2 = (vx[0] - vx[2]) * (sy - vy[2]) - (vy[0] - vy[2]) * (sx - vx[2]);
				bool neg = d0 < 0 || d1 < 0 || d2 < 0, pos = d0 > 0 || d1 > 0 || d2 > 0;
				if (!(neg && pos))
					dentro++;
			}
			if (dentro)
				punto(l, x, y, col, dentro * 255 / 16);
		}
	}
}

static void texto_centrado(struct lienzo *l, int w, int h, enum letra f, int tam, struct color col, const char *s) {
	int tw = texto_ancho(f, tam, s);
	texto(l, (w - tw) / 2, h / 2 + (int)(tam * 0.36f), f, tam, col, s);
}

static SDL_Texture *subir(SDL_Renderer *p, struct lienzo *l) {
	SDL_Texture *t = SDL_CreateTexture(p, SDL_PIXELFORMAT_RGBA32, SDL_TEXTUREACCESS_STATIC, l->pw, l->ph);
	if (t) {
		SDL_SetTextureBlendMode(t, SDL_BLENDMODE_BLEND);
		SDL_SetTextureScaleMode(t, SDL_ScaleModeLinear);
		SDL_UpdateTexture(t, NULL, l->px, l->pw * 4);
	}
	lienzo_libre(l);
	return t;
}

static SDL_Texture *pintar(SDL_Renderer *p, const struct control *k, bool pulsado) {
	int w = k->r.w, h = k->r.h;
	struct lienzo l;
	if (w <= 0 || h <= 0 || !lienzo_nuevo(&l, w, h))
		return NULL;
	switch (k->forma) {
	case CIRCULO:
		lienzo_redondeado(&l, 0, 0, w, h, w / 2, pulsado ? VIOLETA_CLARO : VIOLETA);
		texto_centrado(&l, w, h, LETRA_NEGRITA, (int)(h * 0.44f), BLANCO, k->texto);
		break;
	case PASTILLA:
		lienzo_redondeado(&l, 0, 0, w, h, h / 2, pulsado ? BOTON_PULSADO : BOTON);
		if (k->grande)
			texto_centrado(&l, w, h, LETRA_NEGRITA, (int)(h * 0.5f), pulsado ? BLANCO : TEXTO, k->texto);
		else
			texto_centrado(&l, w, h, LETRA_NORMAL, (int)(h * 0.4f), pulsado ? BLANCO : TEXTO_SUAVE, k->texto);
		break;
	case CRUZ: {
		int a = (int)(w * 0.34f);
		lienzo_redondeado(&l, 0, (h - a) / 2, w, a, a / 4, BOTON);
		lienzo_redondeado(&l, (w - a) / 2, 0, a, h, a / 4, BOTON);
		float t = a * 0.34f, lejos = w * 0.33f;
		flecha(&l, w / 2.0f, h / 2.0f - lejos, t, 0, FLECHA);
		flecha(&l, w / 2.0f, h / 2.0f + lejos, t, 1, FLECHA);
		flecha(&l, w / 2.0f - lejos, h / 2.0f, t, 2, FLECHA);
		flecha(&l, w / 2.0f + lejos, h / 2.0f, t, 3, FLECHA);
		break;
	}
	}
	return subir(p, &l);
}

/* El brazo de arriba de la cruz, encendido: del borde hasta un poco pasado el centro. */
static void pintar_brazo(SDL_Renderer *p) {
	const SDL_Rect *r = &c[C_CRUZ].r;
	int a = (int)(r->w * 0.34f);
	brazo_w = a, brazo_h = r->h / 2 + a / 4;
	struct lienzo l;
	if (a <= 0 || !lienzo_nuevo(&l, brazo_w, brazo_h))
		return;
	lienzo_redondeado(&l, 0, 0, brazo_w, brazo_h, a / 4, BOTON_PULSADO);
	flecha(&l, brazo_w / 2.0f, r->h / 2.0f - r->w * 0.33f, a * 0.34f, 0, BLANCO);
	brazo = subir(p, &l);
}

static void construir(SDL_Renderer *p) {
	int antes = lienzo_escala;
	lienzo_escala = 1; /* aquí todo va ya en píxeles de verdad */
	for (int i = 0; i < N_C; i++) {
		c[i].normal = pintar(p, &c[i], false);
		c[i].pulsado = c[i].forma == CRUZ ? NULL : pintar(p, &c[i], true);
	}
	pintar_brazo(p);
	lienzo_escala = antes;
}

void tactil_olvidar(void) {
	for (int i = 0; i < N_C; i++) {
		if (c[i].normal)
			SDL_DestroyTexture(c[i].normal);
		if (c[i].pulsado)
			SDL_DestroyTexture(c[i].pulsado);
		c[i].normal = c[i].pulsado = NULL;
	}
	if (brazo)
		SDL_DestroyTexture(brazo);
	brazo = NULL;
}

static void dibujar_brazo(SDL_Renderer *p, int dir, uint8_t alfa) {
	if (!brazo)
		return;
	const SDL_Rect *r = &c[C_CRUZ].r;
	float cx = r->x + r->w / 2.0f, cy = r->y + r->h / 2.0f, m = brazo_h / 2.0f;
	static const double angulo[4] = {0, 180, 270, 90};
	float ox[4] = {0, 0, -1, 1}, oy[4] = {-1, 1, 0, 0};
	/* el centro del brazo, a media distancia entre el centro de la cruz y su borde */
	float bx = cx + ox[dir] * (r->w / 2.0f - m), by = cy + oy[dir] * (r->h / 2.0f - m);
	SDL_Rect dst = {(int)(bx - brazo_w / 2.0f), (int)(by - m), brazo_w, brazo_h};
	SDL_SetTextureAlphaMod(brazo, alfa);
	SDL_RenderCopyEx(p, brazo, NULL, &dst, angulo[dir], NULL, SDL_FLIP_NONE);
}

void tactil_dibujar(SDL_Renderer *p, uint32_t pulsados, bool rapido) {
	if (!tactil_visible || pan_w <= 0)
		return;
	if (!c[C_CRUZ].normal)
		construir(p);
	uint8_t alfa = encima ? 150 : 255;
	for (int i = 0; i < N_C; i++) {
		struct control *k = &c[i];
		bool on;
		if (i == C_MENU)
			on = pulsados & BOT_MENU_PANTALLA;
		else if (i == C_SELECT || i == C_START)
			on = (pulsados & k->bits) && !(pulsados & BOT_MENU_PANTALLA);
		else if (i == C_RAPIDO)
			on = rapido || (pulsados & BOT_R2);
		else
			on = k->forma != CRUZ && (pulsados & k->bits);
		SDL_Texture *t = on && k->pulsado ? k->pulsado : k->normal;
		if (!t)
			continue;
		SDL_SetTextureAlphaMod(t, alfa);
		SDL_RenderCopy(p, t, NULL, &k->r);
	}
	if (pulsados & BOT_ARRIBA) dibujar_brazo(p, 0, alfa);
	if (pulsados & BOT_ABAJO) dibujar_brazo(p, 1, alfa);
	if (pulsados & BOT_IZQ) dibujar_brazo(p, 2, alfa);
	if (pulsados & BOT_DER) dibujar_brazo(p, 3, alfa);
}
