/* Órdenes por 127.0.0.1:55355 (UDP), como las de RetroArch, para la app Pokémon:
 *   SCREENSHOT   guarda una captura del juego (PNG) en la carpeta de capturas
 *   QUIT         guarda y sale
 * Solo se escucha en la propia consola.
 */
#include "panther.h"

#include <arpa/inet.h>
#include <errno.h>
#include <netinet/in.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>
#include <zlib.h>

static int fd = -1;

void ordenes_abrir(void) {
	fd = socket(AF_INET, SOCK_DGRAM | SOCK_CLOEXEC | SOCK_NONBLOCK, 0);
	struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons(55355), .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
	if (fd >= 0 && bind(fd, (struct sockaddr *)&a, sizeof(a)) != 0) {
		close(fd);
		fd = -1;
	}
}

/* Devuelve la orden que haya llegado (o NULL). */
const char *ordenes_leer(void) {
	static char buf[128];
	if (fd < 0)
		return NULL;
	ssize_t n = recv(fd, buf, sizeof(buf) - 1, 0);
	if (n <= 0)
		return NULL;
	while (n > 0 && (buf[n - 1] == '\n' || buf[n - 1] == '\r' || buf[n - 1] == ' '))
		n--;
	buf[n] = 0;
	return buf;
}

void ordenes_cerrar(void) {
	if (fd >= 0)
		close(fd);
	fd = -1;
}

static void trozo(FILE *f, const char *tipo, const uint8_t *datos, uint32_t n) {
	uint8_t c[8] = {(uint8_t)(n >> 24), (uint8_t)(n >> 16), (uint8_t)(n >> 8), (uint8_t)n, 0, 0, 0, 0};
	memcpy(c + 4, tipo, 4);
	fwrite(c, 1, 8, f);
	if (n)
		fwrite(datos, 1, n, f);
	uLong crc = crc32(0, (const Bytef *)tipo, 4);
	if (n)
		crc = crc32(crc, datos, n);
	uint8_t k[4] = {(uint8_t)(crc >> 24), (uint8_t)(crc >> 16), (uint8_t)(crc >> 8), (uint8_t)crc};
	fwrite(k, 1, 4, f);
}

/* Guarda la imagen (píxeles R, G, B, X) como PNG; antes en un temporal, para que nadie la lea a medias. */
bool guardar_png(const char *ruta, const uint8_t *px, int w, int h, int paso) {
	size_t crudo_n = (size_t)(w * 3 + 1) * h;
	uint8_t *crudo = malloc(crudo_n);
	uLongf comp_n = compressBound(crudo_n);
	uint8_t *comp = malloc(comp_n);
	if (!crudo || !comp) {
		free(crudo);
		free(comp);
		return false;
	}
	for (int y = 0; y < h; y++) {
		uint8_t *d = crudo + (size_t)y * (w * 3 + 1);
		const uint8_t *s = px + (size_t)y * paso;
		*d++ = 0;
		for (int x = 0; x < w; x++, s += 4) {
			*d++ = s[0];
			*d++ = s[1];
			*d++ = s[2];
		}
	}
	bool bien = compress2(comp, &comp_n, crudo, crudo_n, 6) == Z_OK;
	char tmp[1100];
	snprintf(tmp, sizeof(tmp), "%s.tmp", ruta);
	FILE *f = bien ? fopen(tmp, "wb") : NULL;
	if (f) {
		static const uint8_t firma[8] = {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'};
		uint8_t ihdr[13] = {(uint8_t)(w >> 24), (uint8_t)(w >> 16), (uint8_t)(w >> 8), (uint8_t)w,
		                    (uint8_t)(h >> 24), (uint8_t)(h >> 16), (uint8_t)(h >> 8), (uint8_t)h, 8, 2, 0, 0, 0};
		fwrite(firma, 1, 8, f);
		trozo(f, "IHDR", ihdr, 13);
		trozo(f, "IDAT", comp, (uint32_t)comp_n);
		trozo(f, "IEND", NULL, 0);
		bien = fclose(f) == 0 && rename(tmp, ruta) == 0;
	} else {
		bien = false;
	}
	free(crudo);
	free(comp);
	return bien;
}
