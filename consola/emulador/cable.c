/* El cable link de Game Boy por la red, igual que el de Gambatte (su «GameLink» de libretro), para
 * que las salas de Pokemulador sigan funcionando con los amigos que juegan en el móvil o en el PC.
 *
 * El protocolo de Gambatte (libgambatte/libretro/net_serial.cpp), por TCP:
 *  - Quien lleva el reloj (el juego escribe SC con los bits 7 y 0) manda [SB, rápido] y espera la
 *    respuesta [SB del otro, x]: eso es lo que le llega.
 *  - Quien espera el reloj del otro (SC con el bit 7 y sin el 0) mira si han llegado 2 bytes
 *    [SB del otro, rápido]; si es así responde [su SB, 128] y su transferencia se completa.
 * El servidor escucha en el puerto (en todas las direcciones) y el cliente se conecta a la IP.
 */
#define _GNU_SOURCE /* accept4 */
#include "panther.h"

#include <mgba/core/core.h>
#include <mgba/internal/gb/gb.h>
#include <mgba/internal/gb/io.h>
#include <mgba/internal/gb/sio.h>

#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <poll.h>
#include <string.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <unistd.h>

static struct {
	struct GBSIODriver d;
	struct GB *gb;
	bool activo, servidor, esperando;
	char ip[64];
	int puerto;
	int escucha, fd;
	uint64_t intento;
	struct mTimingEvent mirar;
} cable = {.escucha = -1, .fd = -1};

static void cortar(const char *por) {
	if (cable.fd >= 0) {
		registro("cable link: se corta (%s)", por);
		close(cable.fd);
	}
	cable.fd = -1;
}

static bool escribir(const uint8_t *b, int n) {
	while (n > 0) {
		ssize_t r = send(cable.fd, b, (size_t)n, MSG_NOSIGNAL);
		if (r < 0 && errno == EINTR)
			continue;
		if (r <= 0)
			return false;
		b += r, n -= (int)r;
	}
	return true;
}

/* Lee n bytes esperando como mucho ms milisegundos. */
static bool leer(uint8_t *b, int n, int ms) {
	uint64_t hasta = ahora_ms() + (uint64_t)ms;
	while (n > 0) {
		int falta = (int)(hasta - ahora_ms());
		if (falta <= 0 || salir_ya)
			return false;
		struct pollfd p = {cable.fd, POLLIN, 0};
		int r = poll(&p, 1, falta > 200 ? 200 : falta);
		if (r < 0 && errno == EINTR)
			continue;
		if (r <= 0)
			continue;
		ssize_t k = recv(cable.fd, b, (size_t)n, 0);
		if (k == 0 || (k < 0 && errno != EAGAIN && errno != EINTR))
			return false;
		if (k > 0)
			b += k, n -= (int)k;
	}
	return true;
}

static void sb_nada(struct GBSIODriver *d, uint8_t v) { (void)d, (void)v; }

/* El juego empieza una transferencia. */
static uint8_t escribir_sc(struct GBSIODriver *d, uint8_t sc) {
	cable.esperando = false;
	if (!(sc & 0x80))
		return sc;
	struct GB *gb = cable.gb;
	if (sc & 1) { /* llevamos el reloj: mandamos y esperamos lo suyo */
		uint8_t in = 0xff;
		if (cable.fd >= 0) {
			uint8_t b[2] = {gb->memory.io[GB_REG_SB], (uint8_t)((gb->model >= GB_MODEL_CGB) ? (sc & 2) : 0)};
			if (!escribir(b, 2))
				cortar("no se puede mandar");
			else if (!leer(b, 2, 6000))
				cortar("el otro no contesta");
			else
				in = b[0];
		}
		d->p->pendingSB = in;
	} else { /* el reloj es del otro: a esperar a que mande */
		cable.esperando = true;
		mTimingDeschedule(&gb->timing, &cable.mirar);
		mTimingSchedule(&gb->timing, &cable.mirar, 512);
	}
	return sc;
}

/* Mientras esperamos el reloj del otro, se mira cada poco si ha mandado algo. */
static void mirar(struct mTiming *timing, void *contexto, uint32_t tarde) {
	(void)contexto, (void)tarde;
	if (!cable.esperando)
		return;
	struct GB *gb = cable.gb;
	if (cable.fd >= 0) {
		int hay = 0;
		if (ioctl(cable.fd, FIONREAD, &hay) == 0 && hay >= 2) {
			uint8_t b[2];
			if (!leer(b, 2, 1000)) {
				cortar("no se puede leer");
			} else {
				uint8_t r[2] = {gb->memory.io[GB_REG_SB], 128};
				if (!escribir(r, 2))
					cortar("no se puede contestar");
				cable.esperando = false;
				struct GBSIO *sio = &gb->sio;
				sio->pendingSB = b[0];
				sio->period = GBSIOCyclesPerTransfer[b[1] ? 1 : 0];
				sio->remainingBits = 8;
				mTimingDeschedule(timing, &sio->event);
				mTimingSchedule(timing, &sio->event, sio->period * (2 - gb->doubleSpeed));
				return;
			}
		}
	}
	mTimingSchedule(timing, &cable.mirar, 1024);
}

bool cable_empezar(struct mCore *core, bool servidor, const char *ip, int puerto) {
	if (core->platform(core) != mPLATFORM_GB)
		return false;
	cable.gb = core->board;
	cable.servidor = servidor;
	snprintf(cable.ip, sizeof(cable.ip), "%s", ip && *ip ? ip : "127.0.0.1");
	cable.puerto = puerto;
	cable.d = (struct GBSIODriver){.writeSB = sb_nada, .writeSC = escribir_sc};
	cable.mirar = (struct mTimingEvent){.context = &cable, .name = "Panther cable link", .callback = mirar, .priority = 0x31};
	GBSIOSetDriver(&cable.gb->sio, &cable.d);
	cable.activo = true;
	registro("cable link: %s en %s:%d", servidor ? "servidor" : "cliente", servidor ? "*" : cable.ip, puerto);
	return true;
}

/* Una vez por imagen, fuera del juego: conectar si no lo estamos. */
void cable_mantener(void) {
	if (!cable.activo || cable.fd >= 0 || ahora_ms() - cable.intento < 1000)
		return;
	cable.intento = ahora_ms();
	if (cable.servidor) {
		if (cable.escucha < 0) {
			int fd = socket(AF_INET, SOCK_STREAM | SOCK_CLOEXEC | SOCK_NONBLOCK, 0);
			int uno = 1;
			setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &uno, sizeof(uno));
			struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons((uint16_t)cable.puerto), .sin_addr.s_addr = htonl(INADDR_ANY)};
			if (fd < 0 || bind(fd, (struct sockaddr *)&a, sizeof(a)) != 0 || listen(fd, 1) != 0) {
				registro("cable link: no puedo escuchar en %d: %s", cable.puerto, strerror(errno));
				if (fd >= 0)
					close(fd);
				return;
			}
			cable.escucha = fd;
		}
		int fd = accept4(cable.escucha, NULL, NULL, SOCK_CLOEXEC);
		if (fd < 0)
			return;
		cable.fd = fd;
	} else {
		int fd = socket(AF_INET, SOCK_STREAM | SOCK_CLOEXEC, 0);
		struct sockaddr_in a = {.sin_family = AF_INET, .sin_port = htons((uint16_t)cable.puerto)};
		if (fd < 0 || inet_pton(AF_INET, cable.ip, &a.sin_addr) != 1) {
			if (fd >= 0)
				close(fd);
			return;
		}
		/* sin bloquear más de medio segundo */
		fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) | O_NONBLOCK);
		int r = connect(fd, (struct sockaddr *)&a, sizeof(a));
		if (r != 0 && errno == EINPROGRESS) {
			struct pollfd p = {fd, POLLOUT, 0};
			int err = 0;
			socklen_t l = sizeof(err);
			if (poll(&p, 1, 500) == 1 && getsockopt(fd, SOL_SOCKET, SO_ERROR, &err, &l) == 0 && err == 0)
				r = 0;
		}
		if (r != 0) {
			close(fd);
			return;
		}
		fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) & ~O_NONBLOCK);
		cable.fd = fd;
	}
	int uno = 1;
	setsockopt(cable.fd, IPPROTO_TCP, TCP_NODELAY, &uno, sizeof(uno));
	registro("cable link: conectado");
	aviso("Cable link conectado");
}

void cable_acabar(void) {
	if (cable.activo && cable.gb)
		GBSIOSetDriver(&cable.gb->sio, NULL);
	cortar("se acaba el juego");
	if (cable.escucha >= 0)
		close(cable.escucha);
	cable.escucha = -1;
	cable.activo = false;
}
