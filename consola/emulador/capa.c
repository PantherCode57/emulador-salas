/* Lo que el servicio de WhatsApp de la consola pone encima de todo: el aviso pequeño de arriba cuando
 * te escriben y, si lo abres, el chat entero (el juego se queda parado detrás).
 *
 * El servicio lo deja en /dev/shm/panther-capa (memoria compartida, 64 bytes de cabecera y luego la
 * imagen en RGBA sin premultiplicar). Todo en little endian:
 *    0  "PCAP"             4  versión (1)
 *    8  número de cambio   (impar mientras lo está escribiendo)
 *   12  modo               (0 nada, 1 aviso, 2 pantalla entera)
 *   16  ancho  20  alto  24  x  28  y   (en píxeles de 640×480)
 *   32  cuándo empieza el aviso (ms desde 1970, 8 bytes)
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

#define RUTA "/dev/shm/panther-capa"
#define CABECERA 64
#define MAX_PX (ANCHO * ALTO * 4)

static uint8_t *mem;
static size_t tam;
static ino_t inodo;
static uint64_t ultimo_intento, ultima_mirada;
static uint32_t visto = 0xffffffff;
static struct capa_textura tex;
static uint8_t *copia;

static uint32_t modo, x, y, w, h, dura;
static uint64_t empieza;

static uint32_t u32(size_t off) { return __atomic_load_n((uint32_t *)(mem + off), __ATOMIC_ACQUIRE); }

static void soltar(void) {
	if (mem)
		munmap(mem, tam);
	mem = NULL;
	modo = 0;
	visto = 0xffffffff;
}

static void abrir(void) {
	int fd = open(RUTA, O_RDONLY | O_CLOEXEC);
	if (fd < 0)
		return;
	struct stat st;
	if (fstat(fd, &st) == 0 && st.st_size >= CABECERA) {
		void *m = mmap(NULL, (size_t)st.st_size, PROT_READ, MAP_SHARED, fd, 0);
		if (m != MAP_FAILED) {
			mem = m;
			tam = (size_t)st.st_size;
			inodo = st.st_ino;
		}
	}
	close(fd);
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
	if (!mem) {
		if (t - ultimo_intento < 1000)
			return;
		ultimo_intento = t;
		abrir();
		if (!mem)
			return;
	}
	if (t - ultima_mirada > 2000) { /* ¿lo ha vuelto a crear el servicio? */
		ultima_mirada = t;
		struct stat st;
		if (stat(RUTA, &st) != 0 || st.st_ino != inodo) {
			soltar();
			return;
		}
	}
	if (memcmp(mem, "PCAP", 4) != 0)
		return;
	uint32_t n = u32(8);
	if (n == visto || (n & 1))
		return;
	uint32_t m = u32(12), nw = u32(16), nh = u32(20);
	if (m != 0 && (nw == 0 || nh == 0 || nw > ANCHO || nh > ALTO || CABECERA + (size_t)nw * nh * 4 > tam))
		return;
	if (m != 0) {
		if (!copia)
			copia = malloc(MAX_PX);
		if (!copia)
			return;
		memcpy(copia, mem + CABECERA, (size_t)nw * nh * 4);
	}
	uint32_t nx = u32(24), ny = u32(28), nd = u32(40);
	uint64_t ne = (uint64_t)u32(32) | ((uint64_t)u32(36) << 32);
	if (u32(8) != n) /* lo estaba cambiando mientras lo copiábamos: a la siguiente */
		return;
	visto = n;
	modo = m, w = nw, h = nh, x = nx, y = ny, dura = nd, empieza = ne;
	if (m != 0)
		textura_subir_rgba(&tex, copia, (int)w, (int)h);
}

bool capa_pantalla_completa(void) {
	if (!mem || modo != 2 || !tex.tex)
		return false;
	uint32_t latido = u32(44);
	uint32_t s = (uint32_t)(ahora_ms() / 1000);
	return s - latido <= 4;
}

#define ENTRA 220
#define SALE 260

void capa_dibujar(void) {
	if (!mem || !tex.tex)
		return;
	if (modo == 2) {
		if (capa_pantalla_completa())
			textura_dibujar(&tex, (int)x, (int)y, (int)w, (int)h, 255);
		return;
	}
	if (modo != 1)
		return;
	int64_t e = (int64_t)(ahora_ms() - empieza);
	if (e < 0)
		e = 0;
	if (e > (int64_t)dura + SALE)
		return;
	/* Baja desde arriba al llegar y sube al irse, como en el móvil. */
	double p;
	if (e < ENTRA)
		p = (double)e / ENTRA;
	else if (e > (int64_t)dura)
		p = 1.0 - (double)(e - dura) / SALE;
	else
		p = 1.0;
	double suave = 1 - (1 - p) * (1 - p) * (1 - p);
	int dy = (int)((1 - suave) * (y + h + 6));
	textura_dibujar(&tex, (int)x, (int)y - dy, (int)w, (int)h, (uint8_t)(255 * suave));
}

/* ¿Hay que seguir dibujando aunque no cambie nada? (el aviso se está moviendo). */
bool capa_animando(void) {
	if (!mem || modo != 1)
		return false;
	int64_t e = (int64_t)(ahora_ms() - empieza);
	return e >= 0 && e <= (int64_t)dura + SALE;
}
