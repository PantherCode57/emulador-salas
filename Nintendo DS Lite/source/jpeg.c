// Fotos en JPEG para la DS: las de Instagram, las miniaturas de YouTube y cada imagen de los vídeos.
// El puente siempre las manda «baseline» (las de Go: Huffman, 8 bits, 4:2:0 o en gris), así que
// esto lee solo eso (también 4:4:4 y 4:2:2, y con marcas de reinicio), no los progresivos.
// La transformada es la entera de siempre (la «islow» de libjpeg) y lo que más se repite va en la
// memoria rápida de la DS (ITCM) y en instrucciones ARM.
#include <string.h>

#include "sistema.h"

#ifdef __NDS__
#include <nds/ndstypes.h>
#define RAPIDO ITCM_CODE ARM_CODE
#else
#define RAPIDO
#endif

typedef struct {
	uint16_t rapida[512]; // los 9 primeros bits: (largo << 8) | valor; 0 si el código es más largo
	int32_t maxcode[18], mincode[17], valptr[17];
	uint8_t vals[256];
	bool hay;
} Huff;

typedef struct {
	int id, h, v, tq, td, ta;
	int pred;
} Comp;

typedef struct {
	const uint8_t *p, *fin;
	uint32_t buf;
	int nbits;
	bool marca;
} Bits;

static const uint8_t zigzag[64] = {
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
};

static Huff huff[2][4]; // [0] DC, [1] AC
static uint16_t cuant[4][64]; // en el orden del zigzag, como vienen
static Comp comp[3];
static int ncomp, ancho_j, alto_j, reinicio, hmax, vmax;
static uint8_t planos[3][256]; // las muestras de un MCU, por componente (16×16 como mucho)

static int be16(const uint8_t *p) { return (p[0] << 8) | p[1]; }

static bool hacer_huff(Huff *h, const uint8_t *bits, const uint8_t *vals, int nvals)
{
	memset(h, 0, sizeof *h);
	memcpy(h->vals, vals, nvals);
	int code = 0, k = 0;
	for (int l = 1; l <= 16; l++) {
		h->valptr[l] = k;
		h->mincode[l] = code;
		code += bits[l - 1];
		k += bits[l - 1];
		h->maxcode[l] = bits[l - 1] ? code - 1 : -1;
		if (code > (1 << l))
			return false;
		code <<= 1;
	}
	h->maxcode[17] = 0x7fffffff;
	// la tabla rápida
	code = 0;
	k = 0;
	for (int l = 1; l <= 16; l++) {
		for (int i = 0; i < bits[l - 1]; i++, k++, code++) {
			if (l <= 9) {
				int base = code << (9 - l), n = 1 << (9 - l);
				for (int j = 0; j < n; j++)
					h->rapida[base + j] = (uint16_t)((l << 8) | vals[k]);
			}
		}
		code <<= 1;
	}
	h->hay = true;
	return true;
}

static RAPIDO void llenar_bits(Bits *b)
{
	while (b->nbits <= 24) {
		uint32_t c = 0;
		if (!b->marca && b->p < b->fin) {
			c = *b->p++;
			if (c == 0xFF) {
				uint8_t c2 = b->p < b->fin ? *b->p : 0;
				if (c2 == 0) {
					b->p++; // un 0xFF de los datos
				} else { // una marca: de aquí no se pasa (lo que falte son ceros)
					b->marca = true;
					b->p--;
					c = 0;
				}
			}
		}
		b->buf |= c << (24 - b->nbits);
		b->nbits += 8;
	}
}

static RAPIDO int simbolo(Bits *b, const Huff *h)
{
	llenar_bits(b);
	uint16_t r = h->rapida[b->buf >> 23];
	if (r) {
		int l = r >> 8;
		b->buf <<= l;
		b->nbits -= l;
		return r & 0xFF;
	}
	uint32_t code = b->buf >> 16;
	for (int l = 10; l <= 16; l++) {
		int32_t c = (int32_t)(code >> (16 - l));
		if (c <= h->maxcode[l]) {
			b->buf <<= l;
			b->nbits -= l;
			int i = h->valptr[l] + c - h->mincode[l];
			return (i >= 0 && i < 256) ? h->vals[i] : -1;
		}
	}
	return -1;
}

static RAPIDO int recibir(Bits *b, int n)
{
	if (n == 0)
		return 0;
	llenar_bits(b);
	int v = (int)(b->buf >> (32 - n));
	b->buf <<= n;
	b->nbits -= n;
	return v < (1 << (n - 1)) ? v - (1 << n) + 1 : v;
}

// La transformada inversa entera (como la «islow» de libjpeg: 13 bits de constantes y 2 de más en
// la primera pasada). Se multiplica en vez de correr bits a la izquierda: los números pueden ser
// negativos.
#define CB 13
#define P1 2
#define F_0_298631336 2446
#define F_0_390180644 3196
#define F_0_541196100 4433
#define F_0_765366865 6270
#define F_0_899976223 7373
#define F_1_175875602 9633
#define F_1_501321110 12299
#define F_1_847759065 15137
#define F_1_961570560 16069
#define F_2_053119869 16819
#define F_2_562915447 20995
#define F_3_072711026 25172
#define DESC(x, n) (((x) + (1 << ((n) - 1))) >> (n))

static RAPIDO void idct(int32_t *c, uint8_t *sal, int paso)
{
	int32_t ws[64];
	for (int col = 0; col < 8; col++) {
		int32_t *e = c + col, *w = ws + col;
		if (!e[8] && !e[16] && !e[24] && !e[32] && !e[40] && !e[48] && !e[56]) {
			int32_t dc = e[0] * (1 << P1);
			for (int k = 0; k < 8; k++)
				w[k * 8] = dc;
			continue;
		}
		int32_t z2 = e[16], z3 = e[48];
		int32_t z1 = (z2 + z3) * F_0_541196100;
		int32_t t2 = z1 - z3 * F_1_847759065, t3 = z1 + z2 * F_0_765366865;
		z2 = e[0];
		z3 = e[32];
		int32_t t0 = (z2 + z3) * (1 << CB), t1 = (z2 - z3) * (1 << CB);
		int32_t t10 = t0 + t3, t13 = t0 - t3, t11 = t1 + t2, t12 = t1 - t2;
		t0 = e[56];
		t1 = e[40];
		t2 = e[24];
		t3 = e[8];
		z1 = t0 + t3;
		z2 = t1 + t2;
		z3 = t0 + t2;
		int32_t z4 = t1 + t3, z5 = (z3 + z4) * F_1_175875602;
		t0 *= F_0_298631336;
		t1 *= F_2_053119869;
		t2 *= F_3_072711026;
		t3 *= F_1_501321110;
		z1 *= -F_0_899976223;
		z2 *= -F_2_562915447;
		z3 = z3 * -F_1_961570560 + z5;
		z4 = z4 * -F_0_390180644 + z5;
		t0 += z1 + z3;
		t1 += z2 + z4;
		t2 += z2 + z3;
		t3 += z1 + z4;
		w[0] = DESC(t10 + t3, CB - P1);
		w[56] = DESC(t10 - t3, CB - P1);
		w[8] = DESC(t11 + t2, CB - P1);
		w[48] = DESC(t11 - t2, CB - P1);
		w[16] = DESC(t12 + t1, CB - P1);
		w[40] = DESC(t12 - t1, CB - P1);
		w[24] = DESC(t13 + t0, CB - P1);
		w[32] = DESC(t13 - t0, CB - P1);
	}
	for (int fila = 0; fila < 8; fila++) {
		int32_t *w = ws + fila * 8;
		uint8_t *o = sal + fila * paso;
		int32_t v[8];
		if (!w[1] && !w[2] && !w[3] && !w[4] && !w[5] && !w[6] && !w[7]) {
			int32_t dc = DESC(w[0], P1 + 3) + 128;
			uint8_t d = (uint8_t)(dc < 0 ? 0 : dc > 255 ? 255 : dc);
			memset(o, d, 8);
			continue;
		}
		int32_t z2 = w[2], z3 = w[6];
		int32_t z1 = (z2 + z3) * F_0_541196100;
		int32_t t2 = z1 - z3 * F_1_847759065, t3 = z1 + z2 * F_0_765366865;
		int32_t t0 = (w[0] + w[4]) * (1 << CB), t1 = (w[0] - w[4]) * (1 << CB);
		int32_t t10 = t0 + t3, t13 = t0 - t3, t11 = t1 + t2, t12 = t1 - t2;
		t0 = w[7];
		t1 = w[5];
		t2 = w[3];
		t3 = w[1];
		z1 = t0 + t3;
		z2 = t1 + t2;
		z3 = t0 + t2;
		int32_t z4 = t1 + t3, z5 = (z3 + z4) * F_1_175875602;
		t0 *= F_0_298631336;
		t1 *= F_2_053119869;
		t2 *= F_3_072711026;
		t3 *= F_1_501321110;
		z1 *= -F_0_899976223;
		z2 *= -F_2_562915447;
		z3 = z3 * -F_1_961570560 + z5;
		z4 = z4 * -F_0_390180644 + z5;
		t0 += z1 + z3;
		t1 += z2 + z4;
		t2 += z2 + z3;
		t3 += z1 + z4;
		const int n = CB + P1 + 3;
		v[0] = DESC(t10 + t3, n);
		v[7] = DESC(t10 - t3, n);
		v[1] = DESC(t11 + t2, n);
		v[6] = DESC(t11 - t2, n);
		v[2] = DESC(t12 + t1, n);
		v[5] = DESC(t12 - t1, n);
		v[3] = DESC(t13 + t0, n);
		v[4] = DESC(t13 - t0, n);
		for (int k = 0; k < 8; k++) {
			int32_t x = v[k] + 128;
			o[k] = (uint8_t)(x < 0 ? 0 : x > 255 ? 255 : x);
		}
	}
}

// bloque: un bloque de 8×8 de una componente, ya en muestras (en sal, con ese paso).
static RAPIDO bool bloque(Bits *b, Comp *c, uint8_t *sal, int paso)
{
	int32_t coef[64];
	memset(coef, 0, sizeof coef);
	const uint16_t *q = cuant[c->tq];
	int t = simbolo(b, &huff[0][c->td]);
	if (t < 0 || t > 11)
		return false;
	c->pred += recibir(b, t);
	coef[0] = c->pred * q[0];
	const Huff *ac = &huff[1][c->ta];
	for (int k = 1; k < 64;) {
		int rs = simbolo(b, ac);
		if (rs < 0)
			return false;
		int r = rs >> 4, s = rs & 15;
		if (s == 0) {
			if (r != 15)
				break;
			k += 16;
			continue;
		}
		k += r;
		if (k > 63)
			return false;
		coef[zigzag[k]] = recibir(b, s) * q[k];
		k++;
	}
	idct(coef, sal, paso);
	return true;
}

// pintar_mcu: las muestras del MCU, ya en colores de la DS.
static RAPIDO void pintar_mcu(Color *p, int pitch, int alto_p, int px, int py, int mw, int mh)
{
	for (int y = 0; y < mh; y++) {
		int yy = py + y;
		if (yy < 0 || yy >= alto_p)
			continue;
		Color *fila = p + yy * pitch;
		if (ncomp == 1) {
			const uint8_t *g = planos[0] + y * 16;
			for (int x = 0; x < mw; x++) {
				int xx = px + x;
				if (xx < 0 || xx >= pitch)
					continue;
				int v = g[x] >> 3;
				fila[xx] = (Color)(v | (v << 5) | (v << 10) | 0x8000);
			}
			continue;
		}
		const uint8_t *yp = planos[0] + (y * comp[0].v / vmax) * 16;
		const uint8_t *cb = planos[1] + (y * comp[1].v / vmax) * 16;
		const uint8_t *cr = planos[2] + (y * comp[2].v / vmax) * 16;
		int s0 = comp[0].h == hmax ? 0 : 1, s1 = comp[1].h == hmax ? 0 : 1, s2 = comp[2].h == hmax ? 0 : 1;
		for (int x = 0; x < mw; x++) {
			int xx = px + x;
			if (xx < 0 || xx >= pitch)
				continue;
			int Y = yp[x >> s0] << 16, u = cb[x >> s1] - 128, v = cr[x >> s2] - 128;
			int r = (Y + 91881 * v + 32768) >> 16;
			int g = (Y - 22554 * u - 46802 * v + 32768) >> 16;
			int bl = (Y + 116130 * u + 32768) >> 16;
			r = r < 0 ? 0 : r > 255 ? 255 : r;
			g = g < 0 ? 0 : g > 255 ? 255 : g;
			bl = bl < 0 ? 0 : bl > 255 ? 255 : bl;
			fila[xx] = (Color)((r >> 3) | ((g >> 3) << 5) | ((bl >> 3) << 10) | 0x8000);
		}
	}
}

// cabeceras: lee hasta el principio de los datos (SOS). Devuelve dónde empiezan, o NULL.
static const uint8_t *cabeceras(const uint8_t *d, int n, bool solo_tam)
{
	if (n < 4 || d[0] != 0xFF || d[1] != 0xD8)
		return NULL;
	const uint8_t *p = d + 2, *fin = d + n;
	bool sof = false;
	reinicio = 0;
	while (p + 4 <= fin) {
		if (p[0] != 0xFF) {
			p++;
			continue;
		}
		int m = p[1];
		if (m == 0xFF) {
			p++;
			continue;
		}
		if (m == 0xD8 || (m >= 0xD0 && m <= 0xD7) || m == 0x01) {
			p += 2;
			continue;
		}
		int l = be16(p + 2);
		const uint8_t *s = p + 4, *sig = p + 2 + l;
		if (l < 2 || sig > fin)
			return NULL;
		switch (m) {
		case 0xC0:
		case 0xC1: { // baseline (o extendido de 8 bits, que se lee igual)
			if (l < 8 || s[0] != 8)
				return NULL;
			alto_j = be16(s + 1);
			ancho_j = be16(s + 3);
			ncomp = s[5];
			if ((ncomp != 1 && ncomp != 3) || l < 8 + 3 * ncomp || ancho_j <= 0 || alto_j <= 0)
				return NULL;
			hmax = vmax = 1;
			for (int i = 0; i < ncomp; i++) {
				comp[i].id = s[6 + i * 3];
				comp[i].h = s[7 + i * 3] >> 4;
				comp[i].v = s[7 + i * 3] & 15;
				comp[i].tq = s[8 + i * 3] & 3;
				if (comp[i].h < 1 || comp[i].h > 2 || comp[i].v < 1 || comp[i].v > 2)
					return NULL;
				if (ncomp == 1)
					comp[i].h = comp[i].v = 1;
				if (comp[i].h > hmax)
					hmax = comp[i].h;
				if (comp[i].v > vmax)
					vmax = comp[i].v;
			}
			sof = true;
			if (solo_tam)
				return s;
			break;
		}
		case 0xC2:
		case 0xC3:
		case 0xC5:
		case 0xC6:
		case 0xC7:
		case 0xC9:
		case 0xCA:
		case 0xCB:
		case 0xCD:
		case 0xCE:
		case 0xCF:
			return NULL; // progresivo, sin pérdida o aritmético: no
		case 0xC4: { // tablas de Huffman
			const uint8_t *q = s;
			while (q + 17 <= sig) {
				int clase = q[0] >> 4, id = q[0] & 3, total = 0;
				for (int i = 0; i < 16; i++)
					total += q[1 + i];
				if (clase > 1 || total > 256 || q + 17 + total > sig)
					return NULL;
				if (!hacer_huff(&huff[clase][id], q + 1, q + 17, total))
					return NULL;
				q += 17 + total;
			}
			break;
		}
		case 0xDB: { // tablas de cuantización
			const uint8_t *q = s;
			while (q < sig) {
				int prec = q[0] >> 4, id = q[0] & 3;
				if (q + 1 + 64 * (prec + 1) > sig)
					return NULL;
				for (int i = 0; i < 64; i++)
					cuant[id][i] = prec ? (uint16_t)be16(q + 1 + i * 2) : q[1 + i];
				q += 1 + 64 * (prec + 1);
			}
			break;
		}
		case 0xDD:
			reinicio = be16(s);
			break;
		case 0xDA: { // los datos
			if (!sof || l < 6 || s[0] != ncomp)
				return NULL;
			for (int i = 0; i < ncomp; i++) {
				int id = s[1 + i * 2], k = -1;
				for (int j = 0; j < ncomp; j++)
					if (comp[j].id == id)
						k = j;
				if (k < 0)
					return NULL;
				comp[k].td = s[2 + i * 2] >> 4 & 3;
				comp[k].ta = s[2 + i * 2] & 3;
				if (!huff[0][comp[k].td].hay || !huff[1][comp[k].ta].hay)
					return NULL;
			}
			return sig;
		}
		case 0xD9:
			return NULL;
		}
		p = sig;
	}
	return NULL;
}

bool jpeg_tam(const uint8_t *d, int n, int *w, int *h)
{
	if (!cabeceras(d, n, true))
		return false;
	*w = ancho_j;
	*h = alto_j;
	return true;
}

bool jpeg_pintar(const uint8_t *d, int n, Color *p, int pitch, int alto_p, int x0, int y0)
{
	for (int c = 0; c < 2; c++)
		for (int i = 0; i < 4; i++)
			huff[c][i].hay = false;
	const uint8_t *datos = cabeceras(d, n, false);
	if (!datos)
		return false;
	Bits b = {datos, d + n, 0, 0, false};
	int mw = hmax * 8, mh = vmax * 8;
	int mx = (ancho_j + mw - 1) / mw, my = (alto_j + mh - 1) / mh;
	for (int i = 0; i < ncomp; i++)
		comp[i].pred = 0;
	int hasta_reinicio = reinicio;
	for (int y = 0; y < my; y++) {
		for (int x = 0; x < mx; x++) {
			if (reinicio) {
				if (hasta_reinicio == 0) { // RSTn: se empieza de cero
					const uint8_t *q = b.p;
					while (q + 1 < b.fin && !(q[0] == 0xFF && q[1] >= 0xD0 && q[1] <= 0xD7))
						q++;
					if (q + 1 >= b.fin)
						return true; // se ha cortado: lo que haya
					b.p = q + 2;
					b.buf = 0;
					b.nbits = 0;
					b.marca = false;
					for (int i = 0; i < ncomp; i++)
						comp[i].pred = 0;
					hasta_reinicio = reinicio;
				}
				hasta_reinicio--;
			}
			for (int c = 0; c < ncomp; c++) {
				Comp *k = &comp[c];
				for (int by = 0; by < k->v; by++)
					for (int bx = 0; bx < k->h; bx++)
						if (!bloque(&b, k, planos[c] + by * 8 * 16 + bx * 8, 16))
							return y > 0; // si se rompe a medias, lo de arriba ya está pintado
			}
			int px = x * mw, py = y * mh;
			int w = ancho_j - px < mw ? ancho_j - px : mw, h = alto_j - py < mh ? alto_j - py : mh;
			pintar_mcu(p, pitch, alto_p, x0 + px, y0 + py, w, h);
		}
	}
	return true;
}
