// Prueba de Panther OS para la DS en el PC: hace de DS (botones, toques, wifi y puente de mentira),
// pasa por todas las pantallas y guarda una foto de cada una (las dos pantallas, una encima de otra,
// al doble de tamaño). La pasa nintendo-ds.yml:
//   ./prueba <carpeta de fotos> <carpeta que hace de tarjeta>
// Al final escribe «prueba ds: todo bien» o «prueba ds: MAL: …».
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

#include "../source/sistema.h"

static Color arriba[ANCHO * ALTO] __attribute__((aligned(32))), abajo[ANCHO * ALTO] __attribute__((aligned(32)));
static uint32_t imagen;
static const char *fotos, *tarjeta;
static char fallos[2000];

static void mal(const char *que)
{
	if (strlen(fallos) + strlen(que) + 3 < sizeof fallos) {
		if (fallos[0])
			strcat(fallos, "; ");
		strcat(fallos, que);
	}
}

// --- La DS de mentira ---

uint32_t plat_ms(void) { return imagen * 1000 / 60; }

void plat_hora(struct tm *t)
{
	memset(t, 0, sizeof *t);
	t->tm_year = 2026 - 1900;
	t->tm_mon = 10;
	t->tm_mday = 21;
	t->tm_wday = 6; // sábado
	t->tm_hour = 17;
	t->tm_min = 5 + (int)(imagen / 3600);
}

int plat_bateria(void) { return 15; }
const char *plat_carpeta(void) { return tarjeta; }

static const Red redes_cerca[] = {
	{"Zona wifi del móvil", SEG_ABIERTA, 4, false},
	{"Casa-5G", SEG_WPA, 3, false},
	{"Vecino_2.4", SEG_WPA, 2, false},
	{"Biblioteca", SEG_ABIERTA, 1, false},
	{"RouterViejo", SEG_WEP, 2, false},
	{"Impresora-DIRECT", SEG_WPA, 1, false},
	{"Bar de abajo", SEG_ABIERTA, 1, false},
};
#define N_CERCA ((int)(sizeof redes_cerca / sizeof redes_cerca[0]))

static int estado = RED_PARADA;
static uint32_t buscando_desde, conectando_desde;
static char ultima_red[40];
static uint8_t ultima_clave[32];
static int n_ultima_clave = -1;
static bool hay_puente;

void plat_wifi_buscar(void)
{
	estado = RED_BUSCANDO;
	buscando_desde = imagen;
}

int plat_wifi_redes(Red *r, int max)
{
	if (estado == RED_BUSCANDO && imagen - buscando_desde < 30)
		return 0;
	int n = N_CERCA < max ? N_CERCA : max;
	memcpy(r, redes_cerca, n * sizeof *r);
	return n;
}

void plat_wifi_conectar(const char *ssid, const uint8_t *clave, int nclave, bool guardada)
{
	(void)guardada;
	snprintf(ultima_red, sizeof ultima_red, "%s", ssid);
	memcpy(ultima_clave, clave, nclave);
	n_ultima_clave = nclave;
	estado = RED_CONECTANDO;
	conectando_desde = imagen;
}

void plat_wifi_guardadas(void) { estado = RED_FALLO; }
int plat_wifi_n_guardadas(void) { return 0; }

int plat_wifi_estado(void)
{
	if (estado == RED_CONECTANDO && imagen - conectando_desde > 45)
		estado = RED_CONECTADA;
	return estado;
}

const char *plat_wifi_ip(void) { return estado == RED_CONECTADA ? "192.168.43.27" : ""; }

bool plat_puente(char *nombre, int n, char *ip, int nip)
{
	if (!hay_puente || estado != RED_CONECTADA)
		return false;
	snprintf(nombre, n, "Consola XF35H");
	snprintf(ip, nip, "192.168.43.10");
	return true;
}

// --- Las fotos (PNG sin comprimir, para no necesitar nada más) ---

static uint32_t tabla_crc[256];

static uint32_t crc(const uint8_t *b, size_t n, uint32_t c)
{
	if (!tabla_crc[1]) {
		for (uint32_t i = 0; i < 256; i++) {
			uint32_t v = i;
			for (int k = 0; k < 8; k++)
				v = (v & 1) ? 0xEDB88320u ^ (v >> 1) : v >> 1;
			tabla_crc[i] = v;
		}
	}
	c = ~c;
	for (size_t i = 0; i < n; i++)
		c = tabla_crc[(c ^ b[i]) & 0xFF] ^ (c >> 8);
	return ~c;
}

static void be32(uint8_t *b, uint32_t v)
{
	b[0] = (uint8_t)(v >> 24);
	b[1] = (uint8_t)(v >> 16);
	b[2] = (uint8_t)(v >> 8);
	b[3] = (uint8_t)v;
}

static void trozo(FILE *f, const char *tipo, const uint8_t *d, uint32_t n)
{
	uint8_t c[4];
	be32(c, n);
	fwrite(c, 1, 4, f);
	fwrite(tipo, 1, 4, f);
	fwrite(d, 1, n, f);
	uint32_t v = crc((const uint8_t *)tipo, 4, 0);
	v = crc(d, n, v);
	be32(c, v);
	fwrite(c, 1, 4, f);
}

static void foto(const char *nombre)
{
	const int esc = 2, sep = 6;
	const int w = ANCHO * esc, h = (ALTO * 2 + sep) * esc;
	size_t fila = 1 + (size_t)w * 3, crudo_n = fila * h;
	uint8_t *crudo = calloc(1, crudo_n);
	for (int y = 0; y < h; y++) {
		uint8_t *f = crudo + fila * y;
		f[0] = 0;
		int sy = y / esc;
		for (int x = 0; x < w; x++) {
			Color c = RGB(40, 40, 46);
			if (sy < ALTO)
				c = arriba[sy * ANCHO + x / esc];
			else if (sy >= ALTO + sep)
				c = abajo[(sy - ALTO - sep) * ANCHO + x / esc];
			int r = c & 31, g = (c >> 5) & 31, b = (c >> 10) & 31;
			f[1 + x * 3] = (uint8_t)(r << 3 | r >> 2);
			f[2 + x * 3] = (uint8_t)(g << 3 | g >> 2);
			f[3 + x * 3] = (uint8_t)(b << 3 | b >> 2);
		}
	}
	// zlib con bloques sin comprimir
	size_t bloques = (crudo_n + 65534) / 65535;
	size_t zn = 2 + crudo_n + bloques * 5 + 4;
	uint8_t *z = malloc(zn), *q = z;
	*q++ = 0x78;
	*q++ = 0x01;
	uint32_t a = 1, b = 0;
	for (size_t i = 0; i < crudo_n; i += 65535) {
		size_t n = crudo_n - i < 65535 ? crudo_n - i : 65535;
		*q++ = (i + n == crudo_n) ? 1 : 0;
		*q++ = (uint8_t)n;
		*q++ = (uint8_t)(n >> 8);
		*q++ = (uint8_t)~n;
		*q++ = (uint8_t)(~n >> 8);
		memcpy(q, crudo + i, n);
		for (size_t k = 0; k < n; k++) {
			a = (a + crudo[i + k]) % 65521;
			b = (b + a) % 65521;
		}
		q += n;
	}
	be32(q, (b << 16) | a);
	q += 4;

	char ruta[512];
	snprintf(ruta, sizeof ruta, "%s/%s.png", fotos, nombre);
	FILE *f = fopen(ruta, "wb");
	if (!f) {
		mal("no se puede escribir una foto");
		free(crudo);
		free(z);
		return;
	}
	static const uint8_t firma[8] = {0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'};
	fwrite(firma, 1, 8, f);
	uint8_t ihdr[13];
	be32(ihdr, w);
	be32(ihdr + 4, h);
	ihdr[8] = 8;
	ihdr[9] = 2;
	ihdr[10] = ihdr[11] = ihdr[12] = 0;
	trozo(f, "IHDR", ihdr, 13);
	trozo(f, "IDAT", z, (uint32_t)(q - z));
	trozo(f, "IEND", (const uint8_t *)"", 0);
	fclose(f);
	free(crudo);
	free(z);
	printf("foto: %s\n", ruta);
}

// --- Lo que hace la persona que prueba ---

typedef struct {
	uint32_t imagen;
	uint32_t botones; // se pulsan en esa imagen (y se sueltan en la siguiente)
	int tx, ty;       // si botones es B_TOQUE: dónde se toca (durante 3 imágenes)
	const char *foto; // o una foto
} Paso;

// Dónde está cada tecla del teclado (lo mismo que en teclado.c).
#define TECLA_X(u, ancho) (3 + (u) * 25 / 2 + ((ancho) * 25 / 2 - 2) / 2)
#define TECLA_Y(fila) (31 + (fila) * 32 + 14)

static const Paso guion[] = {
	{80, 0, 0, 0, "01_arranque"},
	{200, 0, 0, 0, "02_inicio"},
	{205, B_DER, 0, 0, NULL},
	{208, B_ABAJO, 0, 0, NULL},
	{212, 0, 0, 0, "03_inicio_navegador"},
	{215, B_TOQUE, 46, 43, NULL}, // WhatsApp
	{226, 0, 0, 0, "04_whatsapp_sin_wifi"},
	{230, B_A, 0, 0, NULL},       // a elegir el wifi
	{236, 0, 0, 0, "05_wifi_buscando"},
	{300, 0, 0, 0, "06_wifi_redes"},
	{305, B_ABAJO, 0, 0, NULL},
	{308, B_A, 0, 0, NULL},       // Casa-5G: WPA, no vale
	{312, 0, 0, 0, "07_wifi_wpa"},
	{315, B_TOQUE, 100, 44, NULL}, // la zona wifi del móvil
	{330, 0, 0, 0, "08_conectando"},
	{400, 0, 0, 0, "09_conectado"},
	{410, B_TOQUE, 100, 30 + 4 * 28 + 12, NULL}, // RouterViejo (WEP): pide la clave
	{420, B_TOQUE, TECLA_X(0, 2), TECLA_Y(2), NULL},  // a
	{426, B_TOQUE, TECLA_X(11, 2), TECLA_Y(3), NULL}, // b
	{432, B_TOQUE, TECLA_X(7, 2), TECLA_Y(3), NULL},  // c
	{438, B_TOQUE, TECLA_X(0, 2), TECLA_Y(0), NULL},  // 1
	{444, B_TOQUE, TECLA_X(2, 2), TECLA_Y(0), NULL},  // 2
	{452, 0, 0, 0, "10_teclado_clave"},
	{455, B_X, 0, 0, NULL},       // mayúsculas
	{458, 0, 0, 0, "11_teclado_mayusculas"},
	{460, B_TOQUE, TECLA_X(0, 4), TECLA_Y(4), NULL},  // símbolos
	{464, 0, 0, 0, "12_teclado_simbolos"},
	{466, B_TOQUE, TECLA_X(0, 4), TECLA_Y(4), NULL},  // otra vez letras
	{470, B_START, 0, 0, NULL},   // aceptar
	{480, 0, 0, 0, "13_conectando_wep"},
	{560, B_B, 0, 0, NULL},       // de vuelta a WhatsApp
	{566, 0, 0, 0, "14_whatsapp_con_wifi"},
	{570, 0, 0, 0, NULL},         // (aquí aparece el puente)
	{600, 0, 0, 0, "15_whatsapp_con_puente"},
	{605, B_B, 0, 0, NULL},       // al inicio
	{610, B_DER, 0, 0, NULL},
	{612, B_DER, 0, 0, NULL},
	{620, 0, 0, 0, "16_inicio_final"},
	{625, B_TOQUE, 46 + 82, 43, NULL}, // Spotin
	{630, 0, 0, 0, "17_spotin"},
	{634, B_B, 0, 0, NULL},
	{636, B_TOQUE, 46 + 164, 43, NULL}, // Instagram
	{640, 0, 0, 0, "18_instagram"},
	{644, B_B, 0, 0, NULL},
	{646, B_TOQUE, 46, 43 + 78, NULL}, // YouTube
	{650, 0, 0, 0, "19_youtube"},
	{654, B_B, 0, 0, NULL},
	{656, B_TOQUE, 46 + 164, 43 + 78, NULL}, // Tienda
	{660, 0, 0, 0, "20_tienda"},
	{664, B_B, 0, 0, NULL},
	{666, B_TOQUE, 46 + 82, 43 + 78, NULL}, // Navegador
	{670, 0, 0, 0, "21_navegador"},
};
#define N_GUION ((int)(sizeof guion / sizeof guion[0]))

static void leer_archivo(const char *ruta, char *s, int n)
{
	s[0] = 0;
	FILE *f = fopen(ruta, "r");
	if (!f)
		return;
	size_t l = fread(s, 1, n - 1, f);
	s[l] = 0;
	fclose(f);
}

int main(int argc, char **argv)
{
	if (argc < 3) {
		fprintf(stderr, "uso: prueba <carpeta de fotos> <carpeta que hace de tarjeta>\n");
		return 2;
	}
	fotos = argv[1];
	tarjeta = argv[2];
	mkdir(fotos, 0755);
	mkdir(tarjeta, 0755);
	char ruta[512];
	snprintf(ruta, sizeof ruta, "%s/wifi.txt", tarjeta);
	remove(ruta);

	sistema_iniciar();
	int toque_hasta = -1, tx = 0, ty = 0;
	uint32_t antes = 0;
	for (imagen = 1; imagen <= 700; imagen++) {
		Entrada e;
		memset(&e, 0, sizeof e);
		for (int i = 0; i < N_GUION; i++) {
			if (guion[i].imagen != imagen)
				continue;
			if (guion[i].botones == B_TOQUE) {
				toque_hasta = (int)imagen + 3;
				tx = guion[i].tx;
				ty = guion[i].ty;
			} else {
				e.pulsado |= guion[i].botones;
			}
		}
		if ((int)imagen < toque_hasta) {
			e.mantenido |= B_TOQUE;
			e.tx = tx;
			e.ty = ty;
		}
		e.mantenido |= e.pulsado;
		e.soltado = antes & ~e.mantenido;
		e.repetido = e.pulsado;
		e.pulsado |= e.mantenido & B_TOQUE & ~antes;
		antes = e.mantenido;
		if (imagen == 570)
			hay_puente = true;

		// Comprobaciones por el camino.
		if (imagen == 311 && estado == RED_CONECTANDO)
			mal("se ha puesto a conectar con una red WPA");
		if (imagen == 320 && strcmp(ultima_red, "Zona wifi del móvil") != 0)
			mal("tocar la primera red no conecta con ella");
		if (imagen == 320 && n_ultima_clave != 0)
			mal("a una red abierta le manda clave");

		sistema_paso(&e, arriba, abajo);

		for (int i = 0; i < N_GUION; i++)
			if (guion[i].imagen == imagen && guion[i].foto)
				foto(guion[i].foto);
	}

	if (strcmp(ultima_red, "RouterViejo") != 0 || n_ultima_clave != 5 || memcmp(ultima_clave, "abc12", 5) != 0)
		mal("la clave WEP no llega bien");
	char guardado[256];
	leer_archivo(ruta, guardado, sizeof guardado);
	if (strcmp(guardado, "red=RouterViejo\nclave=abc12\nguardada=0\n") != 0)
		mal("wifi.txt no queda bien");

	// Y que al encender otra vez se conecta sola con la de wifi.txt.
	ultima_red[0] = 0;
	wifi_arrancar();
	if (strcmp(ultima_red, "RouterViejo") != 0)
		mal("al encender no se conecta con la red de la última vez");

	if (fallos[0]) {
		printf("prueba ds: MAL: %s\n", fallos);
		return 1;
	}
	printf("prueba ds: todo bien\n");
	return 0;
}
