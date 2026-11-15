/*
 * Pokemulador: la pantalla de salas del USB (Lakka).
 *
 * Núcleo libretro mínimo, sin juego: enseña lo que dibuja el servicio pokemulador-usb y le manda
 * los botones del mando. Todo lo demás (salas, juegos, partidas) lo hace el servicio.
 *   núcleo -> servicio: 'I' + 2 bytes (botones de RetroPad, little endian)
 *   servicio -> núcleo: 'F' + ancho (2 bytes) + alto (2 bytes) + píxeles XRGB8888
 * por TCP en 127.0.0.1:24890. Solo se declaran las partes de libretro.h que hacen falta.
 */
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <netinet/tcp.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <unistd.h>

#define RETRO_API_VERSION 1
#define RETRO_ENVIRONMENT_SET_PIXEL_FORMAT 10
#define RETRO_ENVIRONMENT_SET_SUPPORT_NO_GAME 18
#define RETRO_ENVIRONMENT_SET_GEOMETRY 37
#define RETRO_ENVIRONMENT_EXPERIMENTAL 0x10000
#define RETRO_ENVIRONMENT_GET_INPUT_BITMASKS (51 | RETRO_ENVIRONMENT_EXPERIMENTAL)
#define RETRO_PIXEL_FORMAT_XRGB8888 1
#define RETRO_DEVICE_JOYPAD 1
#define RETRO_DEVICE_ID_JOYPAD_MASK 256
#define RETRO_REGION_NTSC 0

#define EXPORTAR __attribute__((visibility("default")))

struct retro_system_info {
	const char *library_name;
	const char *library_version;
	const char *valid_extensions;
	bool need_fullpath;
	bool block_extract;
};
struct retro_game_geometry {
	unsigned base_width, base_height, max_width, max_height;
	float aspect_ratio;
};
struct retro_system_timing {
	double fps, sample_rate;
};
struct retro_system_av_info {
	struct retro_game_geometry geometry;
	struct retro_system_timing timing;
};
struct retro_game_info {
	const char *path;
	const void *data;
	size_t size;
	const char *meta;
};

typedef bool (*retro_environment_t)(unsigned cmd, void *data);
typedef void (*retro_video_refresh_t)(const void *data, unsigned width, unsigned height, size_t pitch);
typedef void (*retro_audio_sample_t)(int16_t left, int16_t right);
typedef size_t (*retro_audio_sample_batch_t)(const int16_t *data, size_t frames);
typedef void (*retro_input_poll_t)(void);
typedef int16_t (*retro_input_state_t)(unsigned port, unsigned device, unsigned index, unsigned id);

/* Tamaño de la pantalla: 960x540 o el de POKEMULADOR_ANCHO/ALTO (p. ej. 640x480 en una consola).
 * Si el servicio manda otro tamaño, se cambia sobre la marcha. */
#define MAX_ANCHO 1920
#define MAX_ALTO 1080
#define TAM_MAX (MAX_ANCHO * MAX_ALTO * 4)
#define FONDO 0x009a3330u

static retro_environment_t entorno;
static retro_video_refresh_t video;
static retro_audio_sample_batch_t audio;
static retro_input_poll_t sondear;
static retro_input_state_t estado_entrada;

static unsigned ancho = 960, alto = 540;
static unsigned nuevo_ancho, nuevo_alto;
static uint32_t *marco;
static uint8_t *recibido;
static uint8_t cabecera[5];
static size_t llevamos;      /* bytes leídos del mensaje actual (cabecera + píxeles) */
static size_t tam_pixeles;
static int conexion = -1;
static int espera_conectar;
static bool hay_mascara;
static int ultima_mascara = -1;
static int16_t silencio[800 * 2];

EXPORTAR void retro_set_environment(retro_environment_t cb) {
	entorno = cb;
	bool si = true;
	entorno(RETRO_ENVIRONMENT_SET_SUPPORT_NO_GAME, &si);
}
EXPORTAR void retro_set_video_refresh(retro_video_refresh_t cb) { video = cb; }
EXPORTAR void retro_set_audio_sample(retro_audio_sample_t cb) { (void)cb; }
EXPORTAR void retro_set_audio_sample_batch(retro_audio_sample_batch_t cb) { audio = cb; }
EXPORTAR void retro_set_input_poll(retro_input_poll_t cb) { sondear = cb; }
EXPORTAR void retro_set_input_state(retro_input_state_t cb) { estado_entrada = cb; }
EXPORTAR unsigned retro_api_version(void) { return RETRO_API_VERSION; }

EXPORTAR void retro_get_system_info(struct retro_system_info *info) {
	memset(info, 0, sizeof(*info));
	info->library_name = "Pokemulador";
	info->library_version = "1.0";
	info->valid_extensions = "";
	info->need_fullpath = false;
}

EXPORTAR void retro_get_system_av_info(struct retro_system_av_info *info) {
	memset(info, 0, sizeof(*info));
	info->geometry.base_width = ancho;
	info->geometry.base_height = alto;
	info->geometry.max_width = MAX_ANCHO;
	info->geometry.max_height = MAX_ALTO;
	info->geometry.aspect_ratio = (float)ancho / (float)alto;
	info->timing.fps = 60.0;
	info->timing.sample_rate = 48000.0;
}

static void cerrar(void) {
	if (conexion >= 0)
		close(conexion);
	conexion = -1;
	llevamos = 0;
	ultima_mascara = -1;
	espera_conectar = 30;
}

static void conectar(void) {
	if (conexion >= 0)
		return;
	if (espera_conectar > 0) {
		espera_conectar--;
		return;
	}
	int fd = socket(AF_INET, SOCK_STREAM, 0);
	if (fd < 0)
		return;
	struct sockaddr_in dir;
	memset(&dir, 0, sizeof(dir));
	dir.sin_family = AF_INET;
	dir.sin_port = htons(24890);
	dir.sin_addr.s_addr = htonl(INADDR_LOOPBACK);
	if (connect(fd, (struct sockaddr *)&dir, sizeof(dir)) != 0) {
		close(fd);
		espera_conectar = 30;
		return;
	}
	int uno = 1;
	setsockopt(fd, IPPROTO_TCP, TCP_NODELAY, &uno, sizeof(uno));
	fcntl(fd, F_SETFL, fcntl(fd, F_GETFL) | O_NONBLOCK);
	conexion = fd;
	llevamos = 0;
	ultima_mascara = -1;
}

/* Lee todo lo que haya llegado; cuando se completa un marco, pasa a enseñarse. */
static void leer(void) {
	for (int vueltas = 0; vueltas < 64 && conexion >= 0; vueltas++) {
		ssize_t n;
		if (llevamos < sizeof(cabecera)) {
			n = recv(conexion, cabecera + llevamos, sizeof(cabecera) - llevamos, 0);
		} else {
			n = recv(conexion, recibido + (llevamos - sizeof(cabecera)), tam_pixeles - (llevamos - sizeof(cabecera)), 0);
		}
		if (n == 0) {
			cerrar();
			return;
		}
		if (n < 0) {
			if (errno != EAGAIN && errno != EWOULDBLOCK && errno != EINTR)
				cerrar();
			return;
		}
		llevamos += (size_t)n;
		if (llevamos == sizeof(cabecera)) {
			unsigned w = cabecera[1] | (cabecera[2] << 8), h = cabecera[3] | (cabecera[4] << 8);
			if (cabecera[0] != 'F' || w == 0 || h == 0 || w > MAX_ANCHO || h > MAX_ALTO) {
				cerrar();
				return;
			}
			nuevo_ancho = w;
			nuevo_alto = h;
			tam_pixeles = (size_t)w * h * 4;
		} else if (llevamos > sizeof(cabecera) && llevamos == sizeof(cabecera) + tam_pixeles) {
			if (nuevo_ancho != ancho || nuevo_alto != alto) {
				ancho = nuevo_ancho;
				alto = nuevo_alto;
				struct retro_game_geometry g = {ancho, alto, MAX_ANCHO, MAX_ALTO, (float)ancho / (float)alto};
				entorno(RETRO_ENVIRONMENT_SET_GEOMETRY, &g);
			}
			memcpy(marco, recibido, tam_pixeles);
			llevamos = 0;
		}
	}
}

static unsigned tam_entorno(const char *var, unsigned defecto, unsigned minimo, unsigned maximo) {
	const char *v = getenv(var);
	if (v == NULL)
		return defecto;
	long n = strtol(v, NULL, 10);
	return (n >= (long)minimo && n <= (long)maximo) ? (unsigned)n : defecto;
}

EXPORTAR void retro_init(void) {
	ancho = tam_entorno("POKEMULADOR_ANCHO", 960, 320, MAX_ANCHO);
	alto = tam_entorno("POKEMULADOR_ALTO", 540, 240, MAX_ALTO);
	marco = malloc(TAM_MAX);
	recibido = malloc(TAM_MAX);
	for (size_t i = 0; marco != NULL && i < (size_t)ancho * alto; i++)
		marco[i] = FONDO;
}
EXPORTAR void retro_deinit(void) {
	cerrar();
	free(marco);
	free(recibido);
	marco = NULL;
	recibido = NULL;
}
EXPORTAR void retro_set_controller_port_device(unsigned port, unsigned device) { (void)port; (void)device; }
EXPORTAR void retro_reset(void) {}

EXPORTAR void retro_run(void) {
	sondear();
	uint16_t mascara = 0;
	for (unsigned puerto = 0; puerto < 4; puerto++) {
		if (hay_mascara) {
			mascara |= (uint16_t)estado_entrada(puerto, RETRO_DEVICE_JOYPAD, 0, RETRO_DEVICE_ID_JOYPAD_MASK);
		} else {
			for (unsigned b = 0; b < 16; b++)
				if (estado_entrada(puerto, RETRO_DEVICE_JOYPAD, 0, b))
					mascara |= (uint16_t)(1u << b);
		}
	}
	conectar();
	if (conexion >= 0 && (int)mascara != ultima_mascara) {
		uint8_t m[3] = {'I', (uint8_t)(mascara & 0xff), (uint8_t)(mascara >> 8)};
		ssize_t n = send(conexion, m, sizeof(m), MSG_NOSIGNAL);
		if (n == (ssize_t)sizeof(m))
			ultima_mascara = mascara;
		else if (n < 0 && errno != EAGAIN && errno != EWOULDBLOCK)
			cerrar();
	}
	if (conexion >= 0)
		leer();
	video(marco, ancho, alto, (size_t)ancho * 4);
	audio(silencio, 800);
}

EXPORTAR bool retro_load_game(const struct retro_game_info *game) {
	(void)game;
	if (marco == NULL || recibido == NULL)
		return false;
	int formato = RETRO_PIXEL_FORMAT_XRGB8888;
	if (!entorno(RETRO_ENVIRONMENT_SET_PIXEL_FORMAT, &formato))
		return false;
	hay_mascara = entorno(RETRO_ENVIRONMENT_GET_INPUT_BITMASKS, NULL);
	espera_conectar = 0;
	conectar();
	return true;
}

EXPORTAR bool retro_load_game_special(unsigned type, const struct retro_game_info *info, size_t num) {
	(void)type; (void)info; (void)num;
	return false;
}
EXPORTAR void retro_unload_game(void) { cerrar(); }
EXPORTAR unsigned retro_get_region(void) { return RETRO_REGION_NTSC; }
EXPORTAR size_t retro_serialize_size(void) { return 0; }
EXPORTAR bool retro_serialize(void *data, size_t size) { (void)data; (void)size; return false; }
EXPORTAR bool retro_unserialize(const void *data, size_t size) { (void)data; (void)size; return false; }
EXPORTAR void retro_cheat_reset(void) {}
EXPORTAR void retro_cheat_set(unsigned index, bool enabled, const char *code) { (void)index; (void)enabled; (void)code; }
EXPORTAR void *retro_get_memory_data(unsigned id) { (void)id; return NULL; }
EXPORTAR size_t retro_get_memory_size(unsigned id) { (void)id; return 0; }
