/* El mando de la consola, leído directamente del núcleo de Linux (evdev), sin SDL ni RetroArch.
 *
 * El de la XF35H se llama «GO-Super Gamepad». Los botones (los mismos que tenía RetroArch):
 *   B = BTN_SOUTH, A = BTN_EAST, X = BTN_NORTH, Y = BTN_WEST, L1/R1 = BTN_TL/TR, L2/R2 = BTN_TL2/TR2,
 *   cruceta = BTN_DPAD_*, SELECT/START/L3/R3 = BTN_TRIGGER_HAPPY1..4 y el quinto (si lo hay) = F.
 * También vale un teclado (flechas, Z = B, X = A, Intro = START, Mayúsculas = SELECT) para probar.
 *
 * Mientras el chat de WhatsApp está abierto encima, el servicio se queda el mando para él solo
 * (EVIOCGRAB) y aquí no llega nada: el juego no se entera de esas pulsaciones.
 */
#include "panther.h"

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <linux/input.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#define MAX_DISPOSITIVOS 4

static _Atomic uint32_t estado_actual;
static _Atomic uint32_t pulsados;
static uint32_t estado_de[MAX_DISPOSITIVOS];
static int fds[MAX_DISPOSITIVOS];
static int n_fds;

static uint32_t bit_de(int codigo) {
	switch (codigo) {
	case BTN_SOUTH: return BOT_B;
	case BTN_EAST: return BOT_A;
	case BTN_NORTH: return BOT_X;
	case BTN_WEST: return BOT_Y;
	case BTN_TL: return BOT_L;
	case BTN_TR: return BOT_R;
	case BTN_TL2: return BOT_L2;
	case BTN_TR2: return BOT_R2;
	case BTN_DPAD_UP: return BOT_ARRIBA;
	case BTN_DPAD_DOWN: return BOT_ABAJO;
	case BTN_DPAD_LEFT: return BOT_IZQ;
	case BTN_DPAD_RIGHT: return BOT_DER;
	case BTN_TRIGGER_HAPPY1: return BOT_SELECT;
	case BTN_TRIGGER_HAPPY2: return BOT_START;
	case BTN_TRIGGER_HAPPY3: return BOT_L3;
	case BTN_TRIGGER_HAPPY4: return BOT_R3;
	case BTN_TRIGGER_HAPPY5: return BOT_F;
	case BTN_SELECT: return BOT_SELECT;
	case BTN_START: return BOT_START;
	case BTN_MODE: return BOT_F;
	case BTN_THUMBL: return BOT_L3;
	case BTN_THUMBR: return BOT_R3;
	/* teclado */
	case KEY_UP: return BOT_ARRIBA;
	case KEY_DOWN: return BOT_ABAJO;
	case KEY_LEFT: return BOT_IZQ;
	case KEY_RIGHT: return BOT_DER;
	case KEY_Z: return BOT_B;
	case KEY_X: return BOT_A;
	case KEY_A: return BOT_Y;
	case KEY_S: return BOT_X;
	case KEY_Q: return BOT_L;
	case KEY_W: return BOT_R;
	case KEY_ENTER: return BOT_START;
	case KEY_RIGHTSHIFT: return BOT_SELECT;
	case KEY_ESC: return BOT_F;
	}
	return 0;
}

#define BITS_LONG (sizeof(unsigned long) * 8)
#define TIENE(b, n) (((b)[(n) / BITS_LONG] >> ((n) % BITS_LONG)) & 1)

/* ¿Es un mando o un teclado? (tiene BTN_SOUTH o la tecla Z). */
static bool sirve(int fd) {
	unsigned long teclas[(KEY_MAX + BITS_LONG) / BITS_LONG];
	memset(teclas, 0, sizeof(teclas));
	if (ioctl(fd, EVIOCGBIT(EV_KEY, sizeof(teclas)), teclas) < 0)
		return false;
	return TIENE(teclas, BTN_SOUTH) || TIENE(teclas, KEY_Z);
}

/* Lo que está pulsado de verdad ahora (por si se perdió algo mientras el chat tenía el mando). */
static uint32_t leer_estado(int fd) {
	unsigned long teclas[(KEY_MAX + BITS_LONG) / BITS_LONG];
	memset(teclas, 0, sizeof(teclas));
	if (ioctl(fd, EVIOCGKEY(sizeof(teclas)), teclas) < 0)
		return 0;
	uint32_t m = 0;
	for (int c = 0; c < KEY_MAX; c++)
		if (TIENE(teclas, c))
			m |= bit_de(c);
	return m;
}

static void publicar(void) {
	uint32_t m = 0;
	for (int i = 0; i < n_fds; i++)
		m |= estado_de[i];
	atomic_store(&estado_actual, m);
}

static void *hilo(void *arg) {
	(void)arg;
	struct input_event ev[32];
	for (;;) {
		fd_set fs;
		FD_ZERO(&fs);
		int max = -1;
		for (int i = 0; i < n_fds; i++) {
			if (fds[i] >= 0) {
				FD_SET(fds[i], &fs);
				if (fds[i] > max)
					max = fds[i];
			}
		}
		if (max < 0)
			return NULL;
		/* Si no pasa nada en medio segundo se mira el estado real: así, al soltar el chat el mando, nada
		 * se queda «pulsado». */
		struct timeval espera = {0, 500000};
		int r = select(max + 1, &fs, NULL, NULL, &espera);
		if (r < 0) {
			if (errno == EINTR)
				continue;
			return NULL;
		}
		if (r == 0) {
			/* Solo para soltar lo que ya no está pulsado: lo que se pulsa mientras el chat tiene el
			 * mando no tiene que llegar al juego. */
			for (int i = 0; i < n_fds; i++)
				if (fds[i] >= 0)
					estado_de[i] &= leer_estado(fds[i]);
			publicar();
			continue;
		}
		for (int i = 0; i < n_fds; i++) {
			if (fds[i] < 0 || !FD_ISSET(fds[i], &fs))
				continue;
			ssize_t n = read(fds[i], ev, sizeof(ev));
			if (n < 0) {
				if (errno != EAGAIN && errno != EINTR) {
					close(fds[i]);
					fds[i] = -1;
					estado_de[i] = 0;
				}
				continue;
			}
			for (size_t k = 0; k < (size_t)n / sizeof(ev[0]); k++) {
				if (ev[k].type == EV_SYN && ev[k].code == SYN_DROPPED) {
					estado_de[i] = leer_estado(fds[i]);
					continue;
				}
				if (ev[k].type != EV_KEY)
					continue;
				uint32_t b = bit_de(ev[k].code);
				if (!b)
					continue;
				if (ev[k].value) {
					estado_de[i] |= b;
					atomic_fetch_or(&pulsados, b);
				} else {
					estado_de[i] &= ~b;
				}
			}
		}
		publicar();
	}
}

bool mandos_abrir(void) {
	DIR *d = opendir("/dev/input");
	if (!d)
		return false;
	struct dirent *e;
	while ((e = readdir(d)) != NULL && n_fds < MAX_DISPOSITIVOS) {
		if (strncmp(e->d_name, "event", 5) != 0)
			continue;
		char ruta[300];
		snprintf(ruta, sizeof(ruta), "/dev/input/%s", e->d_name);
		int fd = open(ruta, O_RDONLY | O_NONBLOCK | O_CLOEXEC);
		if (fd < 0)
			continue;
		if (!sirve(fd)) {
			close(fd);
			continue;
		}
		char nombre[128] = "";
		ioctl(fd, EVIOCGNAME(sizeof(nombre)), nombre);
		registro("mando: %s (%s)", nombre, ruta);
		estado_de[n_fds] = leer_estado(fd);
		fds[n_fds++] = fd;
	}
	closedir(d);
	if (n_fds == 0) {
		registro("no encuentro ningún mando");
		return false;
	}
	publicar();
	pthread_t h;
	pthread_create(&h, NULL, hilo, NULL);
	pthread_detach(h);
	return true;
}

void mandos_leer(uint32_t *estado, uint32_t *nuevos) {
	*estado = atomic_load(&estado_actual);
	*nuevos = atomic_exchange(&pulsados, 0);
}
