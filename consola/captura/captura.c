/* Captura de la pantalla de la consola (Panther OS), sea lo que sea lo que se esté viendo: el menú,
 * un juego o una app. Lee la imagen que está enseñando la pantalla (DRM/KMS) y necesita root
 * (lo lanza el servicio de WhatsApp con sudo al pulsar L1+R1 o L2+R2).
 *
 *   captura foto <ruta.png>     una captura en PNG
 *   captura crudo <fps>         imágenes de 640×480 en BGRX por la salida, para ffmpeg (grabar)
 *   captura info                dice qué pantalla hay (sin guardar nada)
 *
 * (El kmsgrab de ffmpeg no vale aquí: el núcleo 4.4 de la consola no tiene GETFB2.)
 */
#include <errno.h>
#include <fcntl.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <time.h>
#include <unistd.h>
#include <xf86drm.h>
#include <xf86drmMode.h>
#include <zlib.h>

#define SAL_W 640
#define SAL_H 480

static int drm = -1;
static volatile int parar;

/* Las imágenes ya abiertas (la pantalla va cambiando entre dos o tres). */
static struct {
	uint32_t id;
	uint8_t *px;
	size_t tam;
	uint32_t w, h, paso, bpp;
} vistos[6];

static void al_parar(int s) {
	(void)s;
	parar = 1;
}

/* La imagen que se está enseñando ahora (la del primer CRTC encendido). */
static int actual(void) {
	drmModeRes *res = drmModeGetResources(drm);
	if (!res)
		return -1;
	uint32_t fb = 0;
	for (int i = 0; i < res->count_crtcs && !fb; i++) {
		drmModeCrtc *c = drmModeGetCrtc(drm, res->crtcs[i]);
		if (c) {
			if (c->mode_valid && c->buffer_id)
				fb = c->buffer_id;
			drmModeFreeCrtc(c);
		}
	}
	drmModeFreeResources(res);
	if (!fb)
		return -1;
	for (int i = 0; i < 6; i++)
		if (vistos[i].id == fb && vistos[i].px)
			return i;
	drmModeFB *f = drmModeGetFB(drm, fb);
	if (!f)
		return -1;
	int r = -1;
	if (f->handle && (f->bpp == 32 || f->bpp == 16)) {
		struct drm_mode_map_dumb m;
		memset(&m, 0, sizeof(m));
		m.handle = f->handle;
		if (drmIoctl(drm, DRM_IOCTL_MODE_MAP_DUMB, &m) == 0) {
			size_t tam = (size_t)f->pitch * f->height;
			void *p = mmap(NULL, tam, PROT_READ, MAP_SHARED, drm, (off_t)m.offset);
			if (p != MAP_FAILED) {
				/* el hueco más viejo */
				static int siguiente;
				r = siguiente;
				siguiente = (siguiente + 1) % 6;
				if (vistos[r].px)
					munmap(vistos[r].px, vistos[r].tam);
				vistos[r].id = fb, vistos[r].px = p, vistos[r].tam = tam;
				vistos[r].w = f->width, vistos[r].h = f->height, vistos[r].paso = f->pitch, vistos[r].bpp = f->bpp;
			}
		}
		struct drm_gem_close cierra = {.handle = f->handle};
		drmIoctl(drm, DRM_IOCTL_GEM_CLOSE, &cierra);
	}
	drmModeFreeFB(f);
	return r;
}

/* Copia la pantalla a 640×480 en BGRX. */
static void copiar(int i, uint8_t *dst) {
	memset(dst, 0, SAL_W * SAL_H * 4);
	uint32_t w = vistos[i].w < SAL_W ? vistos[i].w : SAL_W, h = vistos[i].h < SAL_H ? vistos[i].h : SAL_H;
	for (uint32_t y = 0; y < h; y++) {
		const uint8_t *s = vistos[i].px + (size_t)y * vistos[i].paso;
		uint8_t *d = dst + (size_t)y * SAL_W * 4;
		if (vistos[i].bpp == 32) {
			memcpy(d, s, (size_t)w * 4);
		} else { /* RGB565 */
			for (uint32_t x = 0; x < w; x++) {
				uint16_t v = (uint16_t)(s[x * 2] | (s[x * 2 + 1] << 8));
				d[x * 4] = (uint8_t)((v & 31) << 3);
				d[x * 4 + 1] = (uint8_t)(((v >> 5) & 63) << 2);
				d[x * 4 + 2] = (uint8_t)((v >> 11) << 3);
				d[x * 4 + 3] = 255;
			}
		}
	}
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

static int guardar_png(const char *ruta, const uint8_t *bgrx) {
	size_t crudo_n = (size_t)(SAL_W * 3 + 1) * SAL_H;
	uint8_t *crudo = malloc(crudo_n);
	uLongf comp_n = compressBound(crudo_n);
	uint8_t *comp = malloc(comp_n);
	if (!crudo || !comp)
		return 1;
	for (int y = 0; y < SAL_H; y++) {
		uint8_t *d = crudo + (size_t)y * (SAL_W * 3 + 1);
		const uint8_t *s = bgrx + (size_t)y * SAL_W * 4;
		*d++ = 0;
		for (int x = 0; x < SAL_W; x++, s += 4) {
			*d++ = s[2];
			*d++ = s[1];
			*d++ = s[0];
		}
	}
	if (compress2(comp, &comp_n, crudo, crudo_n, 6) != Z_OK)
		return 1;
	char tmp[4200];
	snprintf(tmp, sizeof(tmp), "%s.tmp", ruta);
	FILE *f = fopen(tmp, "wb");
	if (!f)
		return 1;
	static const uint8_t firma[8] = {0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'};
	uint8_t ihdr[13] = {0, 0, SAL_W >> 8, SAL_W & 255, 0, 0, SAL_H >> 8, SAL_H & 255, 8, 2, 0, 0, 0};
	fwrite(firma, 1, 8, f);
	trozo(f, "IHDR", ihdr, 13);
	trozo(f, "IDAT", comp, (uint32_t)comp_n);
	trozo(f, "IEND", NULL, 0);
	if (fclose(f) != 0 || rename(tmp, ruta) != 0)
		return 1;
	/* que sea del usuario de la consola (el que ha usado sudo), no de root */
	const char *u = getenv("SUDO_UID"), *g = getenv("SUDO_GID");
	if (u && g && chown(ruta, (uid_t)atoi(u), (gid_t)atoi(g)) != 0)
		perror(ruta);
	return 0;
}

int main(int argc, char **argv) {
	if (argc < 2) {
		fprintf(stderr, "uso: %s foto <ruta.png> | crudo <fps> | info\n", argv[0]);
		return 2;
	}
	drm = open("/dev/dri/card0", O_RDWR | O_CLOEXEC);
	if (drm < 0) {
		perror("/dev/dri/card0");
		return 1;
	}
	uint8_t *img = malloc(SAL_W * SAL_H * 4);
	if (!img)
		return 1;
	if (!strcmp(argv[1], "info")) {
		int i = actual();
		if (i < 0) {
			fprintf(stderr, "no se puede leer la pantalla: %s\n", strerror(errno));
			return 1;
		}
		copiar(i, img);
		printf("pantalla %ux%u, %u bits, %u bytes por fila\n", vistos[i].w, vistos[i].h, vistos[i].bpp, vistos[i].paso);
		return 0;
	}
	if (!strcmp(argv[1], "foto") && argc >= 3) {
		int i = actual();
		if (i < 0) {
			fprintf(stderr, "no se puede leer la pantalla: %s\n", strerror(errno));
			return 1;
		}
		copiar(i, img);
		return guardar_png(argv[2], img);
	}
	if (!strcmp(argv[1], "crudo")) {
		int fps = argc >= 3 ? atoi(argv[2]) : 20;
		if (fps < 1 || fps > 60)
			fps = 20;
		signal(SIGTERM, al_parar);
		signal(SIGINT, al_parar);
		signal(SIGPIPE, al_parar);
		struct timespec t;
		clock_gettime(CLOCK_MONOTONIC, &t);
		long paso = 1000000000L / fps;
		while (!parar) {
			int i = actual();
			if (i >= 0)
				copiar(i, img);
			if (fwrite(img, 1, SAL_W * SAL_H * 4, stdout) != SAL_W * SAL_H * 4)
				break;
			fflush(stdout);
			t.tv_nsec += paso;
			while (t.tv_nsec >= 1000000000L)
				t.tv_nsec -= 1000000000L, t.tv_sec++;
			struct timespec ahora;
			clock_gettime(CLOCK_MONOTONIC, &ahora);
			if (ahora.tv_sec > t.tv_sec + 1) /* vamos muy tarde: sin recuperar */
				t = ahora;
			clock_nanosleep(CLOCK_MONOTONIC, TIMER_ABSTIME, &t, NULL);
		}
		return 0;
	}
	fprintf(stderr, "no sé qué es «%s»\n", argv[1]);
	return 2;
}
