/* Emulador de Panther OS: un solo programa para la consola (XF35H, RK3326, 640×480).
 *
 * - Juegos de GB, GBC y GBA con el código de mGBA metido dentro (sin RetroArch).
 * - La pantalla de las apps de la consola (WhatsApp, YouTube…): las apps le mandan sus imágenes por
 *   127.0.0.1:24890 como antes al núcleo de RetroArch.
 * - Encima de todo, los avisos de WhatsApp que deja el servicio en /dev/shm/panther-capa.
 */
#ifndef PANTHER_H
#define PANTHER_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include <SDL.h>

#define ANCHO 640
#define ALTO 480

/* Botones. Los 16 primeros son los mismos bits que los de RetroPad, que es como hablan las apps. */
enum {
	BOT_B = 1u << 0,
	BOT_Y = 1u << 1,
	BOT_SELECT = 1u << 2,
	BOT_START = 1u << 3,
	BOT_ARRIBA = 1u << 4,
	BOT_ABAJO = 1u << 5,
	BOT_IZQ = 1u << 6,
	BOT_DER = 1u << 7,
	BOT_A = 1u << 8,
	BOT_X = 1u << 9,
	BOT_L = 1u << 10,
	BOT_R = 1u << 11,
	BOT_L2 = 1u << 12,
	BOT_R2 = 1u << 13,
	BOT_L3 = 1u << 14,
	BOT_R3 = 1u << 15,
	BOT_F = 1u << 16,
};

void registro(const char *fmt, ...) __attribute__((format(printf, 1, 2)));
extern volatile int salir_ya; /* lo pone una señal (SIGTERM…): hay que guardar y salir */
uint64_t ahora_ms(void);      /* reloj de pared en milisegundos (el mismo que usa el servicio) */

/* ---- mandos.c: el mando de la consola, leído directamente (evdev) ---- */
bool mandos_abrir(void);
/* estado: lo pulsado ahora; nuevos: lo que se ha pulsado desde la última vez (aunque ya se soltara). */
void mandos_leer(uint32_t *estado, uint32_t *nuevos);

/* ---- sonido.c ---- */
bool sonido_abrir(void);
int sonido_frecuencia(void);
void sonido_meter(const int16_t *muestras, int marcos);
double sonido_ajuste(void); /* cuánto acelerar o frenar el sonido para que no se corte ni se retrase */
void sonido_pausar(bool pausa);
void sonido_vaciar(void);
void sonido_cerrar(void);

/* ---- lienzo.c: dibujar en memoria (RGBA sin premultiplicar) y letras con stb_truetype ---- */
/* Las medidas van en píxeles de 640×480. Con lienzo_escala = k (la app del móvil, en pantallas
 * grandes) el lienzo tiene k×k píxeles por cada uno y se dibuja más fino; en la consola es 1. */
extern int lienzo_escala;
struct lienzo {
	uint8_t *px;
	int w, h;   /* en píxeles de 640×480 */
	int k;      /* la escala con la que se hizo */
	int pw, ph; /* los píxeles de verdad: w×k, h×k */
};
struct color {
	uint8_t r, g, b, a;
};
enum letra { LETRA_NORMAL, LETRA_NEGRITA };

bool lienzo_nuevo(struct lienzo *l, int w, int h);
void lienzo_borrar(struct lienzo *l, struct color c);
void lienzo_libre(struct lienzo *l);
void lienzo_redondeado(struct lienzo *l, int x, int y, int w, int h, int radio, struct color c);
void lienzo_triangulo_der(struct lienzo *l, int x, int y, int w, int h, struct color c);
bool letras_cargar(void);
int texto_ancho(enum letra f, int tam, const char *s);
/* escribe con la línea base en y; devuelve dónde acaba */
int texto(struct lienzo *l, int x, int y, enum letra f, int tam, struct color c, const char *s);
/* recorta con «…» si no cabe */
void texto_recortado(struct lienzo *l, int x, int y, int ancho_max, enum letra f, int tam, struct color c, const char *s);

/* ---- video.c ---- */
enum escala { ESCALA_ENTERA, ESCALA_NITIDA, ESCALA_LLENAR };
bool video_abrir(void);
void video_cerrar(void);
double video_hz(void);
void video_imagen(const void *px, int w, int h, int paso_bytes, Uint32 formato, enum escala e);
void video_empezar(void);
void video_dibujar_imagen(void);
void video_oscurecer(uint8_t alfa);
/* Texturas para lo que va encima (menú, avisos…). */
struct capa_textura {
	SDL_Texture *tex;
	int w, h;
};
void textura_subir(struct capa_textura *t, const struct lienzo *l);
void textura_subir_rgba(struct capa_textura *t, const uint8_t *rgba, int w, int h);
void textura_dibujar(struct capa_textura *t, int x, int y, int w, int h, uint8_t alfa);
void textura_libre(struct capa_textura *t);
void video_presentar(void);
void video_sincronizar(bool con_refresco);

/* ---- capa.c: avisos y pantallas del servicio de WhatsApp (/dev/shm/panther-capa) ---- */
void capa_mirar(void);        /* lee si hay algo nuevo (una vez por imagen) */
bool capa_pantalla_completa(void); /* el chat está abierto encima: el juego se para */
void capa_dibujar(void);      /* dibuja el aviso o el chat (lo último, encima de todo) */

/* ---- avisos.c: los mensajitos del propio emulador («Partida guardada», el «x3»…) ---- */
void aviso(const char *texto);
void avisos_dibujar(bool rapido, int factor);

/* ---- juego.c: el juego (mGBA) ---- */
struct opciones {
	const char *rom;
	const char *dir_guardados; /* .srm; si no, junto al juego (como RetroArch) */
	const char *dir_estados;   /* partidas rápidas; si no, junto al juego */
	const char *dir_sistema;   /* donde mira pokemulador/multijugador.lua (las salas) */
	const char *dir_capturas;  /* donde van las capturas que pide la app Pokémon */
	const char *opciones_nucleo; /* opciones de Gambatte de la app Pokémon (el cable link de la sala) */
	bool sin_seguir;           /* no guardar ni retomar dónde ibas (la app Pokémon lo lleva ella) */
	const char *scripts[8];
	int n_scripts;
	int medir; /* sin pantalla: emula tantas imágenes lo más deprisa posible y dice cuánto tarda */
};
int juego(const struct opciones *o);

/* ---- menu.c: el menú de pausa ---- */
enum accion_menu { MENU_NADA, MENU_SEGUIR, MENU_GUARDAR, MENU_CARGAR, MENU_SALIR, MENU_REINICIAR };
/* con_caramelos: el juego es uno donde valen los Caramelos Raros infinitos (sale la opción). */
void menu_abrir(const char *titulo, bool hay_partida, bool con_caramelos);
enum accion_menu menu_pulsar(uint32_t nuevos, int *factor_rapido);
void menu_dibujar(void);

/* ---- cable.c: el cable link de GB por la red (como el de Gambatte) ---- */
struct mCore;
bool cable_empezar(struct mCore *core, bool servidor, const char *ip, int puerto);
void cable_mantener(void);
void cable_acabar(void);

/* ---- ordenes.c: órdenes como las de RetroArch (SCREENSHOT, QUIT) y capturas en PNG ---- */
void ordenes_abrir(void);
const char *ordenes_leer(void);
void ordenes_cerrar(void);
bool guardar_png(const char *ruta, const uint8_t *px, int w, int h, int paso);

/* ---- apps.c: la pantalla de las apps de la consola ---- */
int pantalla_apps(void);

/* ---- ajustes.c: lo que se guarda entre partidas (velocidad rápida, Caramelos Raros) ---- */
int ajuste_factor_rapido(void);
void ajuste_guardar_factor(int f);
bool ajuste_caramelos(void);
void ajuste_guardar_caramelos(bool si);

#endif
