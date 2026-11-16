/* Lo que el servicio de WhatsApp de la consola pone encima de todo:
 *  - /dev/shm/panther-capa: el aviso pequeño de arriba cuando te escriben (modo 1) o, si lo abres,
 *    el chat entero (modo 2; el juego se queda parado detrás);
 *  - /dev/shm/panther-barra: el recuadro de arriba a la derecha (batería, hora, WhatsApp);
 *  - /dev/shm/panther-volumen: la barra del volumen de la izquierda, un momento al pulsar + o −.
 *
 * Cada una es memoria compartida: 64 bytes de cabecera y luego la imagen en RGBA sin premultiplicar.
 * Todo en little endian:
 *    0  "PCAP"             4  versión (1)
 *    8  número de cambio   (impar mientras lo está escribiendo)
 *   12  modo               (0 nada, 1 se enseña, 2 pantalla entera)
 *   16  ancho  20  alto  24  x  28  y   (en píxeles de 640×480)
 *   32  cuándo empieza (ms desde 1970, 8 bytes)
 *   40  cuánto dura (ms)
 *   44  latido: segundos desde 1970 (el servicio lo pone cada segundo con el chat abierto; si deja de
 *       hacerlo es que se ha cerrado, y entonces el chat no se enseña)
 */
#include "panther.h"

#include <fcntl.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

#define CABECERA 64

struct lector {
	const char *ruta;
	uint8_t *mem;
	size_t tam;
	ino_t inodo;
	uint64_t intento, mirada;
	uint32_t visto;
	struct capa_textura tex;
	uint8_t *copia;
	size_t tam_copia;
	uint32_t modo, x, y, w, h, dura;
	uint64_t empieza;
};

static struct lector capa = {.ruta = "/dev/shm/panther-capa", .visto = 0xffffffff};
static struct lector barra = {.ruta = "/dev/shm/panther-barra", .visto = 0xffffffff};
static struct lector volumen = {.ruta = "/dev/shm/panther-volumen", .visto = 0xffffffff};

static uint32_t u32(struct lector *l, size_t off) { return __atomic_load_n((uint32_t *)(l->mem + off), __ATOMIC_ACQUIRE); }

static void soltar(struct lector *l) {
	if (l->mem)
		munmap(l->mem, l->tam);
	l->mem = NULL;
	l->modo = 0;
	l->visto = 0xffffffff;
}

static void abrir(struct lector *l) {
	int fd = open(l->ruta, O_RDONLY | O_CLOEXEC);
	if (fd < 0)
		return;
	struct stat st;
	if (fstat(fd, &st) == 0 && st.st_size >= CABECERA) {
		void *m = mmap(NULL, (size_t)st.st_size, PROT_READ, MAP_SHARED, fd, 0);
		if (m != MAP_FAILED) {
			l->mem = m;
			l->tam = (size_t)st.st_size;
			l->inodo = st.st_ino;
		}
	}
	close(fd);
}

static void mirar(struct lector *l, uint64_t t) {
	if (!l->mem) {
		if (t - l->intento < 1000)
			return;
		l->intento = t;
		abrir(l);
		if (!l->mem)
			return;
	}
	if (t - l->mirada > 2000) { /* ¿lo ha vuelto a crear el servicio? */
		l->mirada = t;
		struct stat st;
		if (stat(l->ruta, &st) != 0 || st.st_ino != l->inodo) {
			soltar(l);
			return;
		}
	}
	if (memcmp(l->mem, "PCAP", 4) != 0)
		return;
	uint32_t n = u32(l, 8);
	if (n == l->visto || (n & 1))
		return;
	uint32_t m = u32(l, 12), nw = u32(l, 16), nh = u32(l, 20);
	if (m != 0 && (nw == 0 || nh == 0 || nw > ANCHO || nh > ALTO || CABECERA + (size_t)nw * nh * 4 > l->tam))
		return;
	if (m != 0) {
		size_t bytes = (size_t)nw * nh * 4;
		if (l->tam_copia < bytes) {
			free(l->copia);
			l->copia = malloc(bytes);
			l->tam_copia = l->copia ? bytes : 0;
		}
		if (!l->copia)
			return;
		memcpy(l->copia, l->mem + CABECERA, bytes);
	}
	uint32_t nx = u32(l, 24), ny = u32(l, 28), nd = u32(l, 40);
	uint64_t ne = (uint64_t)u32(l, 32) | ((uint64_t)u32(l, 36) << 32);
	if (u32(l, 8) != n) /* lo estaba cambiando mientras lo copiábamos: a la siguiente */
		return;
	l->visto = n;
	l->modo = m, l->w = nw, l->h = nh, l->x = nx, l->y = ny, l->dura = nd, l->empieza = ne;
	if (m != 0)
		textura_subir_rgba(&l->tex, l->copia, (int)l->w, (int)l->h);
}

/* Cada segundo se toca /dev/shm/panther-capa-viendo: así el servicio sabe que hay alguien pintando la
 * capa (y que puede abrir el chat encima). */
static void decir_que_miro(uint64_t t) {
	static uint64_t ultimo;
	if (t - ultimo < 1000)
		return;
	ultimo = t;
	int fd = open("/dev/shm/panther-capa-viendo", O_WRONLY | O_CREAT | O_CLOEXEC, 0666);
	if (fd >= 0) {
		futimens(fd, NULL);
		close(fd);
	}
}

void capa_mirar(void) {
	uint64_t t = ahora_ms();
	decir_que_miro(t);
	mirar(&capa, t);
	mirar(&barra, t);
	mirar(&volumen, t);
}

bool capa_pantalla_completa(void) {
	if (!capa.mem || capa.modo != 2 || !capa.tex.tex)
		return false;
	uint32_t latido = u32(&capa, 44);
	uint32_t s = (uint32_t)(ahora_ms() / 1000);
	return s - latido <= 4;
}

#define ENTRA 220
#define SALE 260
#define APAGA 300

static void dibujar_aviso(void) {
	int64_t e = (int64_t)(ahora_ms() - capa.empieza);
	if (e < 0)
		e = 0;
	if (e > (int64_t)capa.dura + SALE)
		return;
	/* Baja desde arriba al llegar y sube al irse, como en el móvil. */
	double p;
	if (e < ENTRA)
		p = (double)e / ENTRA;
	else if (e > (int64_t)capa.dura)
		p = 1.0 - (double)(e - capa.dura) / SALE;
	else
		p = 1.0;
	double suave = 1 - (1 - p) * (1 - p) * (1 - p);
	int dy = (int)((1 - suave) * (capa.y + capa.h + 6));
	textura_dibujar(&capa.tex, (int)capa.x, (int)capa.y - dy, (int)capa.w, (int)capa.h, (uint8_t)(255 * suave));
}

void capa_dibujar(void) {
	bool chat = capa_pantalla_completa();
	if (chat)
		textura_dibujar(&capa.tex, (int)capa.x, (int)capa.y, (int)capa.w, (int)capa.h, 255);
	/* el recuadro de arriba (no encima del chat, que tiene su cabecera) */
	if (!chat && barra.mem && barra.modo == 1 && barra.tex.tex)
		textura_dibujar(&barra.tex, (int)barra.x, (int)barra.y, (int)barra.w, (int)barra.h, 255);
	/* el volumen: un momento y se apaga */
	if (volumen.mem && volumen.modo == 1 && volumen.tex.tex) {
		int64_t e = (int64_t)(ahora_ms() - volumen.empieza);
		if (e < 0)
			e = 0;
		if (e <= (int64_t)volumen.dura + APAGA) {
			double a = e <= (int64_t)volumen.dura ? 1.0 : 1.0 - (double)(e - volumen.dura) / APAGA;
			textura_dibujar(&volumen.tex, (int)volumen.x, (int)volumen.y, (int)volumen.w, (int)volumen.h, (uint8_t)(255 * a));
		}
	}
	if (!chat && capa.mem && capa.modo == 1 && capa.tex.tex)
		dibujar_aviso();
}
