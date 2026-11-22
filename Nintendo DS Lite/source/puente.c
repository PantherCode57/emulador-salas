// La conexión con el puente (puente/ en esta carpeta): buscarlo, saludarlo con su código y pedirle
// cosas. Todo va cifrado (cifrado.c). Cada petición es una línea y cada respuesta «R <n>» y n
// líneas, o «B <n>» y n bytes (una foto). Solo hay una petición a la vez: puente_pedir() y, cuando
// llega, puente_respuesta() (y puente_datos() si era una foto).
//
// El saludo va aparte (canal_*), porque los vídeos usan otra conexión con el mismo saludo (video.c).
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sistema.h"

#define RX_MAX 65536
#define MAX_LINEAS 96

// --- Una conexión con el puente: el saludo y lo cifrado ---

// Bytes al azar: lo que se tenga a mano (la hora, las imágenes, el barrido de la pantalla…) pasado
// por ChaCha20. No tienen que ser secretos: la clave sale también del azar del puente.
static void azar(uint8_t *b, int n)
{
	uint8_t k[32], nonce[12] = {0}, bloque[64];
	for (int i = 0; i < 8; i++) {
		uint32_t v = plat_azar() ^ (uint32_t)(i * 0x9e3779b9u);
		k[i * 4] = (uint8_t)v;
		k[i * 4 + 1] = (uint8_t)(v >> 8);
		k[i * 4 + 2] = (uint8_t)(v >> 16);
		k[i * 4 + 3] = (uint8_t)(v >> 24);
	}
	chacha_bloque(k, ahora_ms, nonce, bloque);
	memcpy(b, bloque, n > 64 ? 64 : n);
}

static bool de_hex(const char *s, uint8_t *b, int n)
{
	for (int i = 0; i < n; i++) {
		int v = 0;
		for (int k = 0; k < 2; k++) {
			char c = s[i * 2 + k];
			int d = (c >= '0' && c <= '9') ? c - '0' : (c >= 'a' && c <= 'f') ? c - 'a' + 10 : (c >= 'A' && c <= 'F') ? c - 'A' + 10 : -1;
			if (d < 0)
				return false;
			v = v * 16 + d;
		}
		b[i] = (uint8_t)v;
	}
	return true;
}

static bool enviar_todo(int sock, const uint8_t *b, int n)
{
	int hecho = 0, vueltas = 0;
	while (hecho < n) {
		int k = plat_tcp_enviar(sock, b + hecho, n - hecho);
		if (k < 0 || ++vueltas > 20000)
			return false;
		hecho += k;
	}
	return true;
}

bool canal_abrir(Canal *c, const char *ip)
{
	memset(c, 0, sizeof *c);
	c->sock = plat_tcp_abrir(ip, PUERTO_PUENTE);
	if (c->sock < 0) {
		c->fase = CANAL_CERRADO;
		return false;
	}
	c->fase = CANAL_CONECTANDO;
	c->desde = ahora_ms;
	return true;
}

void canal_cerrar(Canal *c)
{
	if (c->fase != CANAL_CERRADO && c->sock >= 0)
		plat_tcp_cerrar(c->sock);
	c->sock = -1;
	c->fase = CANAL_CERRADO;
}

bool canal_enviar(Canal *c, const char *s)
{
	uint8_t buf[640];
	int n = (int)strlen(s);
	if (n + 1 > (int)sizeof buf)
		n = sizeof buf - 1;
	memcpy(buf, s, n);
	buf[n++] = '\n';
	flujo_xor(&c->sal, buf, n);
	return enviar_todo(c->sock, buf, n);
}

int canal_recibir(Canal *c, uint8_t *b, int n)
{
	int k = plat_tcp_recibir(c->sock, b, n);
	if (k > 0)
		flujo_xor(&c->ent, b, k);
	return k;
}

int canal_paso(Canal *c, const char *codigo)
{
	switch (c->fase) {
	case CANAL_CONECTANDO: {
		int l = plat_tcp_listo(c->sock);
		if (l > 0) {
			c->fase = CANAL_SALUDO;
			c->ncrudo = 0;
			c->desde = ahora_ms;
		} else if (l < 0 || ahora_ms - c->desde > 8000) {
			return -1;
		}
		return 0;
	}
	case CANAL_SALUDO: {
		// «PANTHER-PUENTE 1 <32 cifras hexadecimales>», sin cifrar.
		int k = plat_tcp_recibir(c->sock, c->crudo + c->ncrudo, (int)sizeof c->crudo - 1 - c->ncrudo);
		if (k < 0)
			return -1;
		c->ncrudo += k;
		c->crudo[c->ncrudo] = 0;
		char *fin = memchr(c->crudo, '\n', c->ncrudo);
		if (!fin)
			return (c->ncrudo >= (int)sizeof c->crudo - 1 || ahora_ms - c->desde > 8000) ? -1 : 0;
		*fin = 0;
		uint8_t nS[16], nD[16], clave[32];
		char *h = strrchr(c->crudo, ' ');
		if (strncmp(c->crudo, "PANTHER-PUENTE ", 15) != 0 || !h || strlen(h + 1) < 32 || !de_hex(h + 1, nS, 16))
			return -1;
		azar(nD, 16);
		char hola[64];
		int n = snprintf(hola, sizeof hola, "HOLA 1 ");
		for (int i = 0; i < 16; i++)
			n += snprintf(hola + n, sizeof hola - n, "%02x", nD[i]);
		hola[n++] = '\n';
		if (!enviar_todo(c->sock, (const uint8_t *)hola, n))
			return -1;
		clave_sesion(codigo, nS, nD, clave);
		flujo_iniciar(&c->sal, clave, 1);
		flujo_iniciar(&c->ent, clave, 2);
		if (!canal_enviar(c, "PIN-OK"))
			return -1;
		c->fase = CANAL_BIEN;
		c->ncrudo = 0;
		c->desde = ahora_ms;
		return 0;
	}
	case CANAL_BIEN: {
		// «R 1 / BIEN <nombre>»: si en vez de eso se cierra, el código no es el bueno.
		int k = canal_recibir(c, (uint8_t *)c->crudo + c->ncrudo, (int)sizeof c->crudo - 1 - c->ncrudo);
		if (k < 0)
			return -2;
		c->ncrudo += k;
		c->crudo[c->ncrudo] = 0;
		char *a = memchr(c->crudo, '\n', c->ncrudo);
		char *b = a ? memchr(a + 1, '\n', c->ncrudo - (a + 1 - c->crudo)) : NULL;
		if (!b)
			return (c->ncrudo >= (int)sizeof c->crudo - 1 || ahora_ms - c->desde > 8000) ? -1 : 0;
		*b = 0;
		if (strncmp(c->crudo, "R 1\nBIEN ", 9) != 0)
			return -2;
		snprintf(c->nombre, sizeof c->nombre, "%s", c->crudo + 9);
		c->fase = CANAL_LISTO;
		return 1;
	}
	case CANAL_LISTO:
		return 1;
	}
	return -1;
}

// --- La conexión de las peticiones ---

static int estado = PU_SIN_RED;
static Canal can = {.sock = -1};
static char ip[20], nombre[48], codigo[8];
static char rx[RX_MAX], resp[RX_MAX];
static int nrx;
static char *lineas[MAX_LINEAS];
static int nlineas = -1, ndatos = -1;
static bool esperando, ping_interno, descartar;
static uint32_t desde, ultimo_uso;

static void cambiar(int e)
{
	estado = e;
	desde = ahora_ms;
}

static void cerrar(int e)
{
	canal_cerrar(&can);
	nrx = 0;
	esperando = ping_interno = descartar = false;
	nlineas = ndatos = -1;
	cambiar(e);
}

void puente_iniciar(void)
{
	if (!puente_codigo_leer(codigo, sizeof codigo))
		codigo[0] = 0;
}

int puente_estado(void) { return estado; }
const char *puente_nombre(void) { return nombre; }
const char *puente_ip(void) { return ip; }
const char *puente_codigo(void) { return codigo; }

void puente_poner_codigo(const char *c)
{
	snprintf(codigo, sizeof codigo, "%s", c);
	puente_codigo_guardar(codigo);
	cerrar(PU_BUSCANDO);
}

// recibir: lo que haya llegado, descifrado, al final de rx. false si se ha cerrado.
static bool recibir(void)
{
	for (int vueltas = 0; vueltas < 24; vueltas++) {
		if (nrx >= RX_MAX - 1)
			return false; // demasiado grande: algo va mal
		int n = RX_MAX - 1 - nrx;
		int k = canal_recibir(&can, (uint8_t *)rx + nrx, n > 2048 ? 2048 : n);
		if (k < 0)
			return false;
		if (k == 0)
			break;
		nrx += k;
	}
	return true;
}

// respuesta_completa: si en rx hay ya una respuesta entera («R <n>» y n líneas, o «B <n>» y n
// bytes), la pasa a resp (y a lineas). -1 si lo que llega no tiene sentido.
static int respuesta_completa(void)
{
	char *fin = memchr(rx, '\n', nrx);
	if (!fin)
		return 0;
	int cab = (int)(fin + 1 - rx);
	if (rx[0] == 'B' && rx[1] == ' ') {
		int n = atoi(rx + 2);
		if (n < 0 || n > RX_MAX - 64)
			return -1;
		if (nrx < cab + n)
			return 0;
		memcpy(resp, rx + cab, n);
		ndatos = n;
		nlineas = 0;
		memmove(rx, rx + cab + n, nrx - cab - n);
		nrx -= cab + n;
		return 1;
	}
	if (strncmp(rx, "R ", 2) != 0)
		return -1;
	int n = atoi(rx + 2);
	char *p = fin + 1, *lim = rx + nrx;
	for (int i = 0; i < n; i++) {
		char *q = memchr(p, '\n', lim - p);
		if (!q)
			return 0;
		p = q + 1;
	}
	int largo = (int)(p - (fin + 1));
	memcpy(resp, fin + 1, largo);
	resp[largo] = 0;
	nlineas = 0;
	ndatos = -1;
	char *s = resp;
	for (int i = 0; i < n; i++) {
		char *q = strchr(s, '\n');
		if (!q)
			break;
		*q = 0;
		if (nlineas < MAX_LINEAS)
			lineas[nlineas++] = s;
		s = q + 1;
	}
	int usado = (int)(p - rx);
	memmove(rx, p, nrx - usado);
	nrx -= usado;
	return 1;
}

void puente_paso(void)
{
	if (plat_wifi_estado() != RED_CONECTADA) {
		if (estado != PU_SIN_RED)
			cerrar(PU_SIN_RED);
		return;
	}
	switch (estado) {
	case PU_SIN_RED:
		cambiar(PU_BUSCANDO);
		break;
	case PU_BUSCANDO: {
		char n[48];
		if (plat_puente(n, sizeof n, ip, sizeof ip)) {
			snprintf(nombre, sizeof nombre, "%s", n);
			if (!codigo[0]) {
				cambiar(PU_SIN_CODIGO);
				break;
			}
			if (canal_abrir(&can, ip))
				cambiar(PU_CONECTANDO);
			else
				cerrar(PU_ERROR);
		}
		break;
	}
	case PU_CONECTANDO:
	case PU_SALUDO: {
		int r = canal_paso(&can, codigo);
		if (r == 1) {
			snprintf(nombre, sizeof nombre, "%s", can.nombre);
			nrx = 0;
			nlineas = ndatos = -1;
			ultimo_uso = ahora_ms;
			cambiar(PU_LISTO);
		} else if (r == -1) {
			cerrar(PU_ERROR);
		} else if (r == -2) {
			cerrar(PU_CODIGO_MAL);
		} else if (estado == PU_CONECTANDO && can.fase >= CANAL_SALUDO) {
			estado = PU_SALUDO;
		}
		break;
	}
	case PU_LISTO:
		if (!recibir()) {
			cerrar(PU_ERROR);
			break;
		}
		if (esperando) {
			int r = respuesta_completa();
			if (r < 0) {
				cerrar(PU_ERROR);
			} else if (r > 0) {
				esperando = false;
				if (ping_interno || descartar) {
					ping_interno = descartar = false;
					nlineas = ndatos = -1;
				}
			} else if (ahora_ms - desde > 25000) {
				cerrar(PU_ERROR);
			}
		} else if (nlineas < 0 && ahora_ms - ultimo_uso > 40000) {
			if (puente_pedir("PING")) // que el puente no cierre por no hablar
				ping_interno = true;
		}
		break;
	case PU_ERROR:
		if (ahora_ms - desde > 3000)
			cambiar(PU_BUSCANDO);
		break;
	default: // PU_SIN_CODIGO, PU_CODIGO_MAL: esperan a puente_poner_codigo
		break;
	}
}

bool puente_libre(void)
{
	return estado == PU_LISTO && !esperando && nlineas < 0;
}

bool puente_pedir(const char *linea)
{
	if (!puente_libre())
		return false;
	if (!canal_enviar(&can, linea)) {
		cerrar(PU_ERROR);
		return false;
	}
	esperando = true;
	descartar = false;
	desde = ultimo_uso = ahora_ms;
	return true;
}

int puente_respuesta(char ***l)
{
	if (nlineas < 0)
		return -1;
	int n = nlineas;
	*l = lineas;
	nlineas = -1;
	return n;
}

int puente_datos(const uint8_t **d)
{
	int n = ndatos;
	*d = (const uint8_t *)resp;
	ndatos = -1;
	return n;
}

void puente_olvidar(void)
{
	if (esperando)
		descartar = true;
	nlineas = ndatos = -1;
}

// partir: corta una línea por los tabuladores (la modifica). Devuelve cuántos trozos.
int partir(char *s, char **trozos, int max)
{
	int n = 0;
	while (n < max) {
		trozos[n++] = s;
		char *t = strchr(s, '\t');
		if (!t)
			break;
		*t = 0;
		s = t + 1;
	}
	return n;
}
