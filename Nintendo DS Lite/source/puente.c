// La conexión con el puente (puente/ en esta carpeta): buscarlo, saludarlo con su código y pedirle
// cosas. Todo va cifrado (cifrado.c). Cada petición es una línea y cada respuesta «R <n>» y n
// líneas. Solo hay una petición a la vez: puente_pedir() y, cuando llega, puente_respuesta().
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sistema.h"

#define RX_MAX 32768
#define MAX_LINEAS 96

static int estado = PU_SIN_RED, sock = -1;
static char ip[20], nombre[48], codigo[8];
static Flujo ent, sal;
static char crudo[256];
static int ncrudo;
static bool esperando_bien;
static char rx[RX_MAX], resp[RX_MAX];
static int nrx;
static char *lineas[MAX_LINEAS];
static int nlineas = -1;
static bool esperando, ping_interno;
static uint32_t desde, ultimo_uso;

static void cambiar(int e)
{
	estado = e;
	desde = ahora_ms;
}

static void cerrar(int e)
{
	if (sock >= 0)
		plat_tcp_cerrar(sock);
	sock = -1;
	nrx = ncrudo = 0;
	esperando = esperando_bien = ping_interno = false;
	nlineas = -1;
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

void puente_poner_codigo(const char *c)
{
	snprintf(codigo, sizeof codigo, "%s", c);
	puente_codigo_guardar(codigo);
	cerrar(PU_BUSCANDO);
}

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

static bool enviar_todo(const uint8_t *b, int n)
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

static bool enviar_cifrado(const char *s)
{
	uint8_t buf[640];
	int n = (int)strlen(s);
	if (n + 1 > (int)sizeof buf)
		n = sizeof buf - 1;
	memcpy(buf, s, n);
	buf[n++] = '\n';
	flujo_xor(&sal, buf, n);
	return enviar_todo(buf, n);
}

// recibir: lo que haya llegado, descifrado, al final de rx. false si se ha cerrado.
static bool recibir(void)
{
	uint8_t buf[2048];
	for (int vueltas = 0; vueltas < 16; vueltas++) {
		int k = plat_tcp_recibir(sock, buf, sizeof buf);
		if (k < 0)
			return false;
		if (k == 0)
			break;
		flujo_xor(&ent, buf, k);
		if (nrx + k > RX_MAX - 1)
			return false; // demasiado grande: algo va mal
		memcpy(rx + nrx, buf, k);
		nrx += k;
	}
	return true;
}

// respuesta_completa: si en rx hay ya una respuesta entera («R <n>» y n líneas), la pasa a lineas.
static bool respuesta_completa(void)
{
	char *fin = memchr(rx, '\n', nrx);
	if (!fin)
		return false;
	if (strncmp(rx, "R ", 2) != 0)
		return false;
	int n = atoi(rx + 2);
	char *p = fin + 1, *lim = rx + nrx;
	for (int i = 0; i < n; i++) {
		char *q = memchr(p, '\n', lim - p);
		if (!q)
			return false;
		p = q + 1;
	}
	int largo = (int)(p - (fin + 1));
	memcpy(resp, fin + 1, largo);
	resp[largo] = 0;
	nlineas = 0;
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
	return true;
}

static void saludo(void)
{
	if (!esperando_bien) {
		// «PANTHER-PUENTE 1 <32 cifras hexadecimales>», sin cifrar.
		int k = plat_tcp_recibir(sock, crudo + ncrudo, (int)sizeof crudo - 1 - ncrudo);
		if (k < 0) {
			cerrar(PU_ERROR);
			return;
		}
		ncrudo += k;
		crudo[ncrudo] = 0;
		char *fin = memchr(crudo, '\n', ncrudo);
		if (!fin) {
			if (ncrudo >= (int)sizeof crudo - 1 || ahora_ms - desde > 8000)
				cerrar(PU_ERROR);
			return;
		}
		*fin = 0;
		uint8_t nS[16], nD[16], clave[32];
		char *h = strrchr(crudo, ' ');
		if (strncmp(crudo, "PANTHER-PUENTE ", 15) != 0 || !h || strlen(h + 1) < 32 || !de_hex(h + 1, nS, 16)) {
			cerrar(PU_ERROR);
			return;
		}
		azar(nD, 16);
		char hola[64];
		int n = snprintf(hola, sizeof hola, "HOLA 1 ");
		for (int i = 0; i < 16; i++)
			n += snprintf(hola + n, sizeof hola - n, "%02x", nD[i]);
		hola[n++] = '\n';
		if (!enviar_todo((const uint8_t *)hola, n)) {
			cerrar(PU_ERROR);
			return;
		}
		clave_sesion(codigo, nS, nD, clave);
		flujo_iniciar(&sal, clave, 1);
		flujo_iniciar(&ent, clave, 2);
		nrx = 0;
		if (!enviar_cifrado("PIN-OK")) {
			cerrar(PU_ERROR);
			return;
		}
		esperando_bien = true;
		desde = ahora_ms;
		return;
	}
	// «R 1 / BIEN <nombre>»: si en vez de eso se cierra, el código no es el bueno.
	if (!recibir()) {
		cerrar(PU_CODIGO_MAL);
		return;
	}
	if (respuesta_completa()) {
		esperando_bien = false;
		bool bien = nlineas == 1 && strncmp(lineas[0], "BIEN ", 5) == 0;
		if (bien)
			snprintf(nombre, sizeof nombre, "%s", lineas[0] + 5);
		nlineas = -1;
		if (bien) {
			ultimo_uso = ahora_ms;
			cambiar(PU_LISTO);
		} else {
			cerrar(PU_CODIGO_MAL);
		}
	} else if (ahora_ms - desde > 8000) {
		cerrar(PU_ERROR);
	}
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
			sock = plat_tcp_abrir(ip, PUERTO_PUENTE);
			if (sock >= 0)
				cambiar(PU_CONECTANDO);
			else
				cerrar(PU_ERROR);
		}
		break;
	}
	case PU_CONECTANDO: {
		int l = plat_tcp_listo(sock);
		if (l > 0) {
			ncrudo = 0;
			esperando_bien = false;
			cambiar(PU_SALUDO);
		} else if (l < 0 || ahora_ms - desde > 8000) {
			cerrar(PU_ERROR);
		}
		break;
	}
	case PU_SALUDO:
		saludo();
		break;
	case PU_LISTO:
		if (!recibir()) {
			cerrar(PU_ERROR);
			break;
		}
		if (esperando && respuesta_completa()) {
			esperando = false;
			if (ping_interno) {
				ping_interno = false;
				nlineas = -1;
			}
		} else if (esperando && ahora_ms - desde > 20000) {
			cerrar(PU_ERROR);
		} else if (!esperando && nlineas < 0 && ahora_ms - ultimo_uso > 40000) {
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
	if (!enviar_cifrado(linea)) {
		cerrar(PU_ERROR);
		return false;
	}
	esperando = true;
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
