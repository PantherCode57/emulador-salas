// ChaCha20 (RFC 8439), igual que en el puente (puente/cifrado.go), para que lo que va por el wifi
// abierto no se pueda leer. Cómo se saca la clave de la sesión está explicado allí.
#include <string.h>

#include "sistema.h"

#define ROTL(v, n) (((v) << (n)) | ((v) >> (32 - (n))))
#define CUARTO(a, b, c, d) \
	x[a] += x[b]; x[d] = ROTL(x[d] ^ x[a], 16); \
	x[c] += x[d]; x[b] = ROTL(x[b] ^ x[c], 12); \
	x[a] += x[b]; x[d] = ROTL(x[d] ^ x[a], 8); \
	x[c] += x[d]; x[b] = ROTL(x[b] ^ x[c], 7);

static uint32_t le32(const uint8_t *p)
{
	return (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

void chacha_bloque(const uint8_t clave[32], uint32_t contador, const uint8_t nonce[12], uint8_t sal[64])
{
	uint32_t s[16], x[16];
	s[0] = 0x61707865;
	s[1] = 0x3320646e;
	s[2] = 0x79622d32;
	s[3] = 0x6b206574;
	for (int i = 0; i < 8; i++)
		s[4 + i] = le32(clave + i * 4);
	s[12] = contador;
	for (int i = 0; i < 3; i++)
		s[13 + i] = le32(nonce + i * 4);
	memcpy(x, s, sizeof x);
	for (int i = 0; i < 10; i++) {
		CUARTO(0, 4, 8, 12)
		CUARTO(1, 5, 9, 13)
		CUARTO(2, 6, 10, 14)
		CUARTO(3, 7, 11, 15)
		CUARTO(0, 5, 10, 15)
		CUARTO(1, 6, 11, 12)
		CUARTO(2, 7, 8, 13)
		CUARTO(3, 4, 9, 14)
	}
	for (int i = 0; i < 16; i++) {
		uint32_t v = x[i] + s[i];
		sal[i * 4] = (uint8_t)v;
		sal[i * 4 + 1] = (uint8_t)(v >> 8);
		sal[i * 4 + 2] = (uint8_t)(v >> 16);
		sal[i * 4 + 3] = (uint8_t)(v >> 24);
	}
}

void flujo_iniciar(Flujo *f, const uint8_t clave[32], uint8_t sentido)
{
	memset(f, 0, sizeof *f);
	memcpy(f->clave, clave, 32);
	f->nonce[0] = sentido;
	f->pos = 64;
}

void flujo_xor(Flujo *f, uint8_t *b, int n)
{
	for (int i = 0; i < n; i++) {
		if (f->pos == 64) {
			chacha_bloque(f->clave, f->contador++, f->nonce, f->bloque);
			f->pos = 0;
		}
		b[i] ^= f->bloque[f->pos++];
	}
}

void clave_sesion(const char *codigo, const uint8_t nS[16], const uint8_t nD[16], uint8_t sal[32])
{
	uint8_t k[32] = {0}, n[12], b[64];
	const char *prefijo = "PANTHER-DS-1:";
	int l = (int)strlen(prefijo), m = (int)strlen(codigo);
	memcpy(k, prefijo, l);
	if (l + m > 32)
		m = 32 - l;
	memcpy(k + l, codigo, m);
	memcpy(n, nS, 6);
	memcpy(n + 6, nD, 6);
	chacha_bloque(k, 0, n, b);
	memcpy(sal, b, 32);
}
