/* La pantalla de las apps de la consola (WhatsApp, YouTube, Spotin, Pokémon, Navegador): antes era
 * RetroArch con el núcleo «Pokemulador»; ahora es este programa, que habla igual con ellas por
 * 127.0.0.1:24890:
 *     nosotros -> app: 'I' + 2 bytes (botones de RetroPad, little endian)
 *     app -> nosotros: 'F' + ancho (2 bytes) + alto (2 bytes) + píxeles XRGB8888
 * La app nos cierra (SIGTERM) cuando acaba. Encima van también los avisos de WhatsApp.
 */
#include "panther.h"

#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#define MAX_W 1920
#define MAX_H 1080

static int conexion = -1;
static uint8_t cabecera[5];
static size_t llevamos, tam_pixeles;
static uint8_t *recibido;
static int nuevo_w, nuevo_h;
static int ultima_mascara = -1;
static uint64_t intento;

static void cerrar(void) {
	if (conexion >= 0)
		close(conexion);
	conexion = -1;
	llevamos = 0;
	ultima_mascara = -1;
}

static void conectar(void) {
	if (conexion >= 0 || ahora_ms() - intento < 300)
		return;
	intento = ahora_ms();
	int fd = socket(AF_INET, SOCK_STREAM | SOCK_CLOEXEC, 0);
	if (fd < 0)
		return;
	struct sockaddr_in dir;
	memset(&dir, 0, sizeof(dir));
	dir.sin_family = AF_INET;
	dir.sin_port = htons(24890);
	dir.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
	if (connect(fd, (struct sockaddr *)&dir, sizeof(dir)) != 0) {
		close(fd);
		return;
	}
	int uno = 1;
	setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &uno, sizeof(uno));
	fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) | O_NONBLOCK);
	conexion = fd;
	llevamos = 0;
	ultima_mascara = -1;
}

/* Lee lo que haya llegado. Devuelve true si se ha completado una imagen nueva. */
static bool leer(void) {
	bool nueva = false;
	for (int vueltas = 0; vueltas < 256 && conexion >= 0; vueltas++) {
		ssize_t n;
		if (llevamos < sizeof(cabecera))
			n = recv(conexion, cabecera + llevamos, sizeof(cabecera) - llevamos, 0);
		else
			n = recv(conexion, recibido + (llevamos - sizeof(cabecera)), tam_pixeles - (llevamos - sizeof(cabecera)), 0);
		if (n == 0) {
			cerrar();
			break;
		}
		if (n < 0) {
			if (errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR)
				cerrar();
			break;
		}
		llevamos += (size_t)n;
		if (llevamos == sizeof(cabecera)) {
			int w = cabecera[1] | (cabecera[2] << 8), h = cabecera[3] | (cabecera[4] << 8);
			if (cabecera[0] != 'F' || w <= 0 || h <= 0 || w > MAX_W || h > MAX_H) {
				cerrar();
				break;
			}
			nuevo_w = w, nuevo_h = h;
			tam_pixeles = (size_t)w * h * 4;
		} else if (llevamos > sizeof(cabecera) && llevamos == sizeof(cabecera) + tam_pixeles) {
			video_imagen(recibido, nuevo_w, nuevo_h, nuevo_w * 4, SDL_PIXELFORMAT_XRGB8888, ESCALA_LLENAR);
			llevamos = 0;
			nueva = true;
		}
	}
	return nueva;
}

int pantalla_apps(void) {
	recibido = malloc((size_t)MAX_W * MAX_H * 4);
	if (!recibido || !video_abrir())
		return 1;
	mandos_abrir();
	uint64_t sin_conexion = ahora_ms();
	bool tuvo = false;
	while (!salir_ya) {
		SDL_Event ev;
		while (SDL_PollEvent(&ev))
			if (ev.type == SDL_QUIT)
				salir_ya = 1;
		conectar();
		uint32_t estado, nuevos;
		mandos_leer(&estado, &nuevos);
		capa_mirar();
		if (capa_pantalla_completa())
			estado = 0; /* el chat tiene el mando: a la app no le llega nada */
		/* Una pulsación muy corta (entre dos imágenes) también cuenta. */
		int mascara = (int)((estado | nuevos) & 0xffff);
		if (conexion >= 0 && mascara != ultima_mascara) {
			uint8_t m[3] = {'I', (uint8_t)(mascara & 0xff), (uint8_t)(mascara >> 8)};
			ssize_t n = send(conexion, m, sizeof(m), MSG_NOSIGNAL);
			if (n == (ssize_t)sizeof(m))
				ultima_mascara = (int)(estado & 0xffff) == mascara ? mascara : -1;
			else if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK)
				cerrar();
		}
		if (conexion >= 0) {
			tuvo = true;
			sin_conexion = ahora_ms();
			leer();
		} else if (tuvo && ahora_ms() - sin_conexion > 20000) {
			registro("la app ya no está: salgo");
			break;
		}
		video_empezar();
		video_dibujar_imagen();
		capa_dibujar();
		video_presentar();
	}
	cerrar();
	video_cerrar();
	return 0;
}
