// Dibujo en las pantallas: rectángulos, formas redondeadas con los bordes suaves, líneas y letras.
#include <math.h>
#include <string.h>

#include "sistema.h"

Color mezclar(Color a, Color b, int alfa)
{
	if (alfa <= 0)
		return a | 0x8000;
	if (alfa >= 256)
		return b | 0x8000;
	int ra = a & 31, ga = (a >> 5) & 31, ba = (a >> 10) & 31;
	int rb = b & 31, gb = (b >> 5) & 31, bb = (b >> 10) & 31;
	int r = ra + (((rb - ra) * alfa) >> 8);
	int g = ga + (((gb - ga) * alfa) >> 8);
	int z = ba + (((bb - ba) * alfa) >> 8);
	return (Color)(r | (g << 5) | (z << 10) | 0x8000);
}

static inline void punto(Color *p, int x, int y, Color c, int alfa)
{
	if ((unsigned)x >= ANCHO || (unsigned)y >= ALTO || alfa <= 0)
		return;
	Color *d = &p[y * ANCHO + x];
	*d = alfa >= 256 ? (c | 0x8000) : mezclar(*d, c, alfa);
}

static inline int a_alfa(float cob, int alfa)
{
	if (cob <= 0)
		return 0;
	if (cob >= 1)
		return alfa;
	return (int)(cob * alfa);
}

void llenar(Color *p, Color c)
{
	c |= 0x8000;
	for (int i = 0; i < ANCHO * ALTO; i++)
		p[i] = c;
}

static bool recortar(int *x, int *y, int *w, int *h)
{
	if (*x < 0) {
		*w += *x;
		*x = 0;
	}
	if (*y < 0) {
		*h += *y;
		*y = 0;
	}
	if (*x + *w > ANCHO)
		*w = ANCHO - *x;
	if (*y + *h > ALTO)
		*h = ALTO - *y;
	return *w > 0 && *h > 0;
}

void rect(Color *p, int x, int y, int w, int h, Color c)
{
	if (!recortar(&x, &y, &w, &h))
		return;
	c |= 0x8000;
	for (int j = y; j < y + h; j++) {
		Color *f = &p[j * ANCHO + x];
		for (int i = 0; i < w; i++)
			f[i] = c;
	}
}

void rect_alfa(Color *p, int x, int y, int w, int h, Color c, int alfa)
{
	if (alfa >= 256) {
		rect(p, x, y, w, h, c);
		return;
	}
	if (!recortar(&x, &y, &w, &h))
		return;
	for (int j = y; j < y + h; j++) {
		Color *f = &p[j * ANCHO + x];
		for (int i = 0; i < w; i++)
			f[i] = mezclar(f[i], c, alfa);
	}
}

void degradado(Color *p, int x, int y, int w, int h, Color c1, Color c2)
{
	for (int j = 0; j < h; j++)
		rect(p, x, y + j, w, 1, mezclar(c1, c2, h > 1 ? j * 256 / (h - 1) : 0));
}

// Cuánto tapa el punto (px, py) un rectángulo con las esquinas redondeadas de radio r (0..1).
static float cob_redondeado(int px, int py, int x, int y, int w, int h, int r)
{
	if (px < x || py < y || px >= x + w || py >= y + h)
		return 0;
	if (r <= 0)
		return 1;
	float cx, cy;
	if (px < x + r)
		cx = x + r;
	else if (px >= x + w - r)
		cx = x + w - r;
	else
		return 1;
	if (py < y + r)
		cy = y + r;
	else if (py >= y + h - r)
		cy = y + h - r;
	else
		return 1;
	float dx = px + 0.5f - cx, dy = py + 0.5f - cy;
	return r - sqrtf(dx * dx + dy * dy) + 0.5f;
}

static void redondeado_base(Color *p, int x, int y, int w, int h, int r, Color c1, Color c2, int alfa)
{
	if (r * 2 > w)
		r = w / 2;
	if (r * 2 > h)
		r = h / 2;
	for (int j = y; j < y + h; j++) {
		if ((unsigned)j >= ALTO)
			continue;
		Color c = c1 == c2 ? c1 : mezclar(c1, c2, h > 1 ? (j - y) * 256 / (h - 1) : 0);
		bool fila_entera = j >= y + r && j < y + h - r;
		for (int i = x; i < x + w; i++) {
			if ((unsigned)i >= ANCHO)
				continue;
			if (fila_entera)
				punto(p, i, j, c, alfa);
			else
				punto(p, i, j, c, a_alfa(cob_redondeado(i, j, x, y, w, h, r), alfa));
		}
	}
}

void redondeado(Color *p, int x, int y, int w, int h, int r, Color c, int alfa)
{
	redondeado_base(p, x, y, w, h, r, c, c, alfa);
}

void redondeado_degradado(Color *p, int x, int y, int w, int h, int r, Color c1, Color c2)
{
	redondeado_base(p, x, y, w, h, r, c1, c2, 256);
}

void marco(Color *p, int x, int y, int w, int h, int r, int grosor, Color c)
{
	int ri = r - grosor;
	if (ri < 0)
		ri = 0;
	for (int j = y; j < y + h; j++) {
		for (int i = x; i < x + w; i++) {
			float fuera = cob_redondeado(i, j, x, y, w, h, r);
			if (fuera > 1)
				fuera = 1;
			float dentro = cob_redondeado(i, j, x + grosor, y + grosor, w - 2 * grosor, h - 2 * grosor, ri);
			if (dentro < 0)
				dentro = 0;
			if (dentro > 1)
				dentro = 1;
			punto(p, i, j, c, a_alfa(fuera - dentro, 256));
		}
	}
}

void circulo(Color *p, float cx, float cy, float r, Color c, int alfa)
{
	int x0 = (int)floorf(cx - r - 1), x1 = (int)ceilf(cx + r + 1);
	int y0 = (int)floorf(cy - r - 1), y1 = (int)ceilf(cy + r + 1);
	for (int j = y0; j <= y1; j++) {
		for (int i = x0; i <= x1; i++) {
			float dx = i + 0.5f - cx, dy = j + 0.5f - cy;
			punto(p, i, j, c, a_alfa(r - sqrtf(dx * dx + dy * dy) + 0.5f, alfa));
		}
	}
}

void aro(Color *p, float cx, float cy, float r, float grosor, Color c, float hueco)
{
	float ext = r + grosor / 2 + 1;
	for (int j = (int)floorf(cy - ext); j <= (int)ceilf(cy + ext); j++) {
		for (int i = (int)floorf(cx - ext); i <= (int)ceilf(cx + ext); i++) {
			float dx = i + 0.5f - cx, dy = j + 0.5f - cy;
			if (hueco > 0 && dy < 0 && fabsf(dx) < hueco / 2)
				continue;
			if (hueco < 0 && dy > 0)
				continue;
			float d = sqrtf(dx * dx + dy * dy);
			punto(p, i, j, c, a_alfa(grosor / 2 - fabsf(d - r) + 0.5f, 256));
		}
	}
}

void elipse(Color *p, float cx, float cy, float rx, float ry, float grosor, Color c)
{
	float ex = rx + grosor, ey = ry + grosor;
	for (int j = (int)floorf(cy - ey); j <= (int)ceilf(cy + ey); j++) {
		for (int i = (int)floorf(cx - ex); i <= (int)ceilf(cx + ex); i++) {
			float dx = i + 0.5f - cx, dy = j + 0.5f - cy;
			float f = dx * dx / (rx * rx) + dy * dy / (ry * ry) - 1;
			float gx = dx / (rx * rx), gy = dy / (ry * ry);
			float g = 2 * sqrtf(gx * gx + gy * gy);
			if (g < 0.0001f)
				continue; // el centro
			float d = f / g;
			punto(p, i, j, c, a_alfa(grosor / 2 - fabsf(d) + 0.5f, 256));
		}
	}
}

void linea(Color *p, float x0, float y0, float x1, float y1, float grosor, Color c)
{
	float m = grosor / 2 + 1;
	int ia = (int)floorf(fminf(x0, x1) - m), ib = (int)ceilf(fmaxf(x0, x1) + m);
	int ja = (int)floorf(fminf(y0, y1) - m), jb = (int)ceilf(fmaxf(y0, y1) + m);
	float vx = x1 - x0, vy = y1 - y0, l2 = vx * vx + vy * vy;
	for (int j = ja; j <= jb; j++) {
		for (int i = ia; i <= ib; i++) {
			float px = i + 0.5f - x0, py = j + 0.5f - y0;
			float t = l2 > 0 ? (px * vx + py * vy) / l2 : 0;
			if (t < 0)
				t = 0;
			if (t > 1)
				t = 1;
			float dx = px - t * vx, dy = py - t * vy;
			punto(p, i, j, c, a_alfa(grosor / 2 - sqrtf(dx * dx + dy * dy) + 0.5f, 256));
		}
	}
}

static inline float lado(float ax, float ay, float bx, float by, float px, float py)
{
	return (bx - ax) * (py - ay) - (by - ay) * (px - ax);
}

void triangulo(Color *p, float x0, float y0, float x1, float y1, float x2, float y2, Color c)
{
	int ia = (int)floorf(fminf(x0, fminf(x1, x2))), ib = (int)ceilf(fmaxf(x0, fmaxf(x1, x2)));
	int ja = (int)floorf(fminf(y0, fminf(y1, y2))), jb = (int)ceilf(fmaxf(y0, fmaxf(y1, y2)));
	float area = lado(x0, y0, x1, y1, x2, y2);
	if (area == 0)
		return;
	static const float sub[4][2] = {{0.25f, 0.25f}, {0.75f, 0.25f}, {0.25f, 0.75f}, {0.75f, 0.75f}};
	for (int j = ja; j <= jb; j++) {
		for (int i = ia; i <= ib; i++) {
			int dentro = 0;
			for (int k = 0; k < 4; k++) {
				float px = i + sub[k][0], py = j + sub[k][1];
				float a = lado(x0, y0, x1, y1, px, py), b = lado(x1, y1, x2, y2, px, py), d = lado(x2, y2, x0, y0, px, py);
				if (area > 0 ? (a >= 0 && b >= 0 && d >= 0) : (a <= 0 && b <= 0 && d <= 0))
					dentro++;
			}
			punto(p, i, j, c, dentro * 64);
		}
	}
}

// --- Letras ---

static const char *utf8(const char *s, uint32_t *cp)
{
	const unsigned char *u = (const unsigned char *)s;
	if (u[0] < 0x80) {
		*cp = u[0];
		return s + 1;
	}
	if ((u[0] & 0xE0) == 0xC0 && (u[1] & 0xC0) == 0x80) {
		*cp = ((u[0] & 0x1F) << 6) | (u[1] & 0x3F);
		return s + 2;
	}
	if ((u[0] & 0xF0) == 0xE0 && (u[1] & 0xC0) == 0x80 && (u[2] & 0xC0) == 0x80) {
		*cp = ((u[0] & 0x0F) << 12) | ((u[1] & 0x3F) << 6) | (u[2] & 0x3F);
		return s + 3;
	}
	if ((u[0] & 0xF8) == 0xF0 && (u[1] & 0xC0) == 0x80 && (u[2] & 0xC0) == 0x80 && (u[3] & 0xC0) == 0x80) {
		*cp = ((u[0] & 0x07) << 18) | ((u[1] & 0x3F) << 12) | ((u[2] & 0x3F) << 6) | (u[3] & 0x3F);
		return s + 4;
	}
	*cp = '?';
	return s + 1;
}

static const Glifo *glifo(const Letra *l, uint32_t cp)
{
	int a = 0, b = l->n - 1;
	while (a <= b) {
		int m = (a + b) / 2;
		if (l->glifos[m].cp == cp)
			return &l->glifos[m];
		if (l->glifos[m].cp < cp)
			a = m + 1;
		else
			b = m - 1;
	}
	return cp == '?' ? NULL : glifo(l, '?');
}

int texto_alfa(Color *p, const Letra *l, int x, int y, const char *s, Color c, int alfa)
{
	int x0 = x;
	while (*s) {
		uint32_t cp;
		s = utf8(s, &cp);
		if (cp == '\n')
			break;
		const Glifo *g = glifo(l, cp);
		if (!g)
			continue;
		const uint8_t *m = l->alfa + g->pos;
		for (int j = 0; j < g->h; j++) {
			int py = y + g->y + j;
			if ((unsigned)py >= ALTO) {
				m += g->w;
				continue;
			}
			for (int i = 0; i < g->w; i++, m++) {
				if (*m)
					punto(p, x + g->x + i, py, c, (*m * alfa + 128) / 255);
			}
		}
		x += g->avance;
	}
	return x - x0;
}

int texto(Color *p, const Letra *l, int x, int y, const char *s, Color c)
{
	return texto_alfa(p, l, x, y, s, c, 256);
}

int ancho_texto(const Letra *l, const char *s)
{
	int w = 0;
	while (*s) {
		uint32_t cp;
		s = utf8(s, &cp);
		if (cp == '\n')
			break;
		const Glifo *g = glifo(l, cp);
		if (g)
			w += g->avance;
	}
	return w;
}

void texto_centro(Color *p, const Letra *l, int cx, int y, const char *s, Color c)
{
	texto(p, l, cx - ancho_texto(l, s) / 2, y, s, c);
}

void texto_derecha(Color *p, const Letra *l, int xd, int y, const char *s, Color c)
{
	texto(p, l, xd - ancho_texto(l, s), y, s, c);
}

void texto_corto(Color *p, const Letra *l, int x, int y, int max, const char *s, Color c)
{
	if (ancho_texto(l, s) <= max) {
		texto(p, l, x, y, s, c);
		return;
	}
	char buf[160];
	int puntos = ancho_texto(l, "…"), w = 0, n = 0;
	const char *q = s;
	while (*q) {
		uint32_t cp;
		const char *sig = utf8(q, &cp);
		const Glifo *g = glifo(l, cp);
		int a = g ? g->avance : 0;
		if (w + a + puntos > max || n + (sig - q) + 4 >= (int)sizeof buf)
			break;
		memcpy(buf + n, q, sig - q);
		n += sig - q;
		w += a;
		q = sig;
	}
	while (n > 0 && buf[n - 1] == ' ')
		n--;
	memcpy(buf + n, "…", 4); // con el 0 del final
	texto(p, l, x, y, buf, c);
}

int texto_parrafo(Color *p, const Letra *l, int x, int y, int max, const char *s, Color c)
{
	char linea_[200];
	int alto = l->alto + 2;
	while (*s) {
		// Coge palabras mientras quepan.
		int n = 0, ultimo_corte = 0;
		const char *q = s, *tras_corte = s;
		while (*q && *q != '\n') {
			const char *fin = q;
			while (*fin && *fin != ' ' && *fin != '\n')
				fin++;
			int largo = fin - q;
			if (n + largo + 2 >= (int)sizeof linea_)
				break;
			memcpy(linea_ + n, q, largo);
			linea_[n + largo] = 0;
			if (ancho_texto(l, linea_) > max && ultimo_corte > 0)
				break;
			n += largo;
			ultimo_corte = n;
			tras_corte = fin;
			q = fin;
			if (*q == ' ') {
				linea_[n++] = ' ';
				q++;
				tras_corte = q;
			}
		}
		if (ultimo_corte == 0 && *q && *q != '\n') { // una palabra que no cabe: se corta
			texto_corto(p, l, x, y, max, s, c);
			while (*s && *s != ' ' && *s != '\n')
				s++;
		} else {
			linea_[ultimo_corte] = 0;
			texto(p, l, x, y, linea_, c);
			s = tras_corte;
		}
		while (*s == ' ')
			s++;
		if (*s == '\n')
			s++;
		y += alto;
	}
	return y;
}
