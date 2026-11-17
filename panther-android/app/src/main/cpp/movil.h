/* La app Panther del móvil: lo que cambia respecto al emulador de la consola (consola/emulador).
 *
 * El juego, el menú de pausa, los avisos, la letra y el cable link son los de la consola, tal cual.
 * Aquí está solo lo del móvil:
 *  - movil.c          el arranque (lo llama la actividad Juego de Java) y el registro en logcat;
 *  - movil_video.c    la pantalla: el juego arriba (de pie) o en medio (tumbado) y los botones;
 *  - movil_tactil.c   dónde va cada botón, cuál tocas y cómo se dibujan;
 *  - movil_mandos.c   la pantalla táctil, la tecla «atrás» y los mandos Bluetooth o USB.
 */
#ifndef MOVIL_H
#define MOVIL_H

#include "panther.h"

/* Lo que tapa la cámara o la muesca de la pantalla, en píxeles (lo dice la app al abrir el juego). */
extern int recorte_pantalla;

/* El botón «Menú» de la pantalla (y la tecla «atrás») pulsa SELECT y START a la vez, que es lo que
 * abre el menú de pausa en la consola. Este bit es solo para dibujarlo encendido: el juego no lo ve. */
#define BOT_MENU_PANTALLA (1u << 20)

/* ---- movil_tactil.c ---- */
/* Coloca los botones para una pantalla de w×h y dice dónde va el juego. */
void tactil_disponer(int w, int h, SDL_Rect *juego);
/* Qué botones hay en el punto (x, y), de 0 a 1 como los da SDL. Se puede llamar desde otro hilo. */
uint32_t tactil_botones(float x, float y);
void tactil_dibujar(SDL_Renderer *pintor, uint32_t pulsados, bool rapido);
void tactil_olvidar(void); /* borra las texturas (cambia el tamaño o se cierra la pantalla) */
extern volatile int tactil_visible; /* se esconden al usar un mando de verdad */

/* ---- movil_mandos.c ---- */
uint32_t mandos_pulsados_pantalla(void); /* lo que está pulsado en la pantalla, para dibujarlo */

#endif
