/* La pantalla del móvil, con SDL (OpenGL ES 2). Hace lo mismo que video.c de la consola, pero:
 *
 *  - El juego no ocupa toda la pantalla: va en la «zona» que deja libre movil_tactil.c (arriba si
 *    tienes el móvil de pie, en medio si está tumbado) y alrededor van los botones.
 *  - Lo que en la consola son 640×480 (el menú de pausa, los avisos) va en un recuadro 4:3 dentro de
 *    esa zona, y se dibuja a 2× o 3× (lienzo_escala) para que las letras se vean finas.
 *  - Los móviles tienen pantallas de 60, 90 o 120 Hz: el juego no puede ir al ritmo de la pantalla
 *    como en la consola, así que lleva su propio reloj (59,73 imágenes por segundo, las de la GB y
 *    la GBA) y le pide a Android 60 Hz (eso lo hace la actividad Juego).
 */
#include "movil.h"

#include <errno.h>
#include <time.h>

#define HZ 59.7275 /* imágenes por segundo de la GB, la GBC y la GBA */

static SDL_Window *ventana;
static SDL_Renderer *pintor;
static SDL_Texture *tex_juego, *tex_nitida;
static int img_w, img_h, nit_w, nit_h;
static Uint32 img_formato;
static enum escala img_escala;
static int pan_w, pan_h; /* la pantalla entera */
static SDL_Rect zona;    /* donde va el juego */
static SDL_Rect marco;   /* el recuadro 4:3 de dentro: los 640×480 de la consola */
static bool a_su_ritmo = true;
static struct timespec siguiente;

static const SDL_Color FONDO = {13, 10, 20, 255};

static void colocar(void) {
	int w, h;
	if (!pintor || SDL_GetRendererOutputSize(pintor, &w, &h) != 0 || w <= 0 || h <= 0)
		return;
	if (w == pan_w && h == pan_h)
		return;
	pan_w = w, pan_h = h;
	tactil_disponer(w, h, &zona);
	int mw = zona.w, mh = zona.w * 3 / 4;
	if (mh > zona.h) {
		mh = zona.h;
		mw = zona.h * 4 / 3;
	}
	marco = (SDL_Rect){zona.x + (zona.w - mw) / 2, zona.y + (zona.h - mh) / 2, mw, mh};
	int k = (mw + 639) / 640;
	lienzo_escala = k < 1 ? 1 : (k > 4 ? 4 : k);
	registro("pantalla %dx%d: juego en %dx%d+%d+%d, menú a escala %d", w, h, zona.w, zona.h, zona.x, zona.y, lienzo_escala);
}

bool video_abrir(void) {
	SDL_SetHint(SDL_HINT_RENDER_DRIVER, "opengles2");
	SDL_SetHint(SDL_HINT_RENDER_VSYNC, "0");
	if (SDL_InitSubSystem(SDL_INIT_VIDEO) != 0) {
		registro("no se puede abrir la pantalla: %s", SDL_GetError());
		return false;
	}
	/* Con RESIZABLE, SDL deja girar el móvil (de pie y tumbado, según el bloqueo de rotación). */
	ventana = SDL_CreateWindow("Panther", 0, 0, ANCHO, ALTO,
	                           SDL_WINDOW_FULLSCREEN | SDL_WINDOW_RESIZABLE | SDL_WINDOW_SHOWN | SDL_WINDOW_ALLOW_HIGHDPI);
	if (!ventana) {
		registro("no se puede abrir la pantalla: %s", SDL_GetError());
		return false;
	}
	pintor = SDL_CreateRenderer(ventana, -1, SDL_RENDERER_ACCELERATED | SDL_RENDERER_TARGETTEXTURE);
	if (!pintor) {
		registro("no se puede dibujar: %s", SDL_GetError());
		return false;
	}
	SDL_RendererInfo info;
	SDL_GetRendererInfo(pintor, &info);
	colocar();
	registro("pantalla: %s, %s", SDL_GetCurrentVideoDriver(), info.name);
	SDL_SetRenderDrawColor(pintor, FONDO.r, FONDO.g, FONDO.b, 255);
	SDL_RenderClear(pintor);
	SDL_RenderPresent(pintor);
	clock_gettime(CLOCK_MONOTONIC, &siguiente);
	return true;
}

void video_cerrar(void) {
	tactil_olvidar();
	if (tex_juego)
		SDL_DestroyTexture(tex_juego);
	if (tex_nitida)
		SDL_DestroyTexture(tex_nitida);
	tex_juego = tex_nitida = NULL;
	if (pintor)
		SDL_DestroyRenderer(pintor);
	if (ventana)
		SDL_DestroyWindow(ventana);
	pintor = NULL;
	ventana = NULL;
}

double video_hz(void) { return HZ; }

void video_imagen(const void *px, int w, int h, int paso_bytes, Uint32 formato, enum escala e) {
	if (!pintor)
		return;
	if (!tex_juego || w != img_w || h != img_h || formato != img_formato) {
		if (tex_juego)
			SDL_DestroyTexture(tex_juego);
		tex_juego = SDL_CreateTexture(pintor, formato, SDL_TEXTUREACCESS_STREAMING, w, h);
		if (!tex_juego) {
			registro("textura del juego: %s", SDL_GetError());
			return;
		}
		SDL_SetTextureScaleMode(tex_juego, SDL_ScaleModeNearest);
		img_w = w, img_h = h, img_formato = formato;
	}
	img_escala = e;
	SDL_UpdateTexture(tex_juego, NULL, px, paso_bytes);
}

void video_empezar(void) {
	colocar();
	SDL_SetRenderTarget(pintor, NULL);
	SDL_SetRenderDrawColor(pintor, FONDO.r, FONDO.g, FONDO.b, 255);
	SDL_RenderClear(pintor);
	SDL_SetRenderDrawColor(pintor, 0, 0, 0, 255);
	SDL_RenderFillRect(pintor, &zona);
}

/* Como en la consola: GB y GBC a un número entero de veces; GBA, primero a un entero sin suavizar y
 * luego al tamaño de la zona suavizando (los píxeles se ven nítidos y todos iguales). */
void video_dibujar_imagen(void) {
	if (!tex_juego || zona.w <= 0)
		return;
	SDL_Rect dst;
	if (img_escala == ESCALA_ENTERA) {
		int k = zona.w / img_w < zona.h / img_h ? zona.w / img_w : zona.h / img_h;
		if (k < 1)
			k = 1;
		dst.w = img_w * k, dst.h = img_h * k;
	} else {
		double k = (double)zona.w / img_w < (double)zona.h / img_h ? (double)zona.w / img_w : (double)zona.h / img_h;
		dst.w = (int)(img_w * k + 0.5), dst.h = (int)(img_h * k + 0.5);
	}
	dst.x = zona.x + (zona.w - dst.w) / 2, dst.y = zona.y + (zona.h - dst.h) / 2;
	if (img_escala != ESCALA_NITIDA) {
		SDL_RenderCopy(pintor, tex_juego, NULL, &dst);
		return;
	}
	int p = (dst.h + img_h - 1) / img_h;
	if (p < 1)
		p = 1;
	if (!tex_nitida || nit_w != img_w * p || nit_h != img_h * p) {
		if (tex_nitida)
			SDL_DestroyTexture(tex_nitida);
		tex_nitida = SDL_CreateTexture(pintor, SDL_PIXELFORMAT_RGBA32, SDL_TEXTUREACCESS_TARGET, img_w * p, img_h * p);
		if (!tex_nitida) {
			SDL_RenderCopy(pintor, tex_juego, NULL, &dst);
			return;
		}
		SDL_SetTextureScaleMode(tex_nitida, SDL_ScaleModeLinear);
		nit_w = img_w * p, nit_h = img_h * p;
	}
	SDL_SetRenderTarget(pintor, tex_nitida);
	SDL_RenderCopy(pintor, tex_juego, NULL, NULL);
	SDL_SetRenderTarget(pintor, NULL);
	SDL_RenderCopy(pintor, tex_nitida, NULL, &dst);
}

/* Detrás del menú de pausa se oscurece el juego, no los botones (hacen falta para moverse por él). */
void video_oscurecer(uint8_t alfa) {
	SDL_SetRenderDrawBlendMode(pintor, SDL_BLENDMODE_BLEND);
	SDL_SetRenderDrawColor(pintor, 0, 0, 0, alfa);
	SDL_RenderFillRect(pintor, &zona);
}

static void subir(struct capa_textura *t, const uint8_t *px, int pw, int ph, int w, int h) {
	if (!pintor || pw <= 0 || ph <= 0)
		return;
	int tw = 0, th = 0;
	if (t->tex)
		SDL_QueryTexture(t->tex, NULL, NULL, &tw, &th);
	if (!t->tex || tw != pw || th != ph) {
		if (t->tex)
			SDL_DestroyTexture(t->tex);
		t->tex = SDL_CreateTexture(pintor, SDL_PIXELFORMAT_RGBA32, SDL_TEXTUREACCESS_STATIC, pw, ph);
		if (!t->tex)
			return;
		SDL_SetTextureBlendMode(t->tex, SDL_BLENDMODE_BLEND);
		SDL_SetTextureScaleMode(t->tex, SDL_ScaleModeLinear);
	}
	t->w = w, t->h = h;
	SDL_UpdateTexture(t->tex, NULL, px, pw * 4);
}

void textura_subir_rgba(struct capa_textura *t, const uint8_t *rgba, int w, int h) { subir(t, rgba, w, h, w, h); }

/* El lienzo tiene lienzo_escala píxeles por cada uno de 640×480: la textura va con todos, pero sus
 * medidas (t->w, t->h) siguen en 640×480, que es como las usan el menú y los avisos. */
void textura_subir(struct capa_textura *t, const struct lienzo *l) { subir(t, l->px, l->pw, l->ph, l->w, l->h); }

void textura_dibujar(struct capa_textura *t, int x, int y, int w, int h, uint8_t alfa) {
	if (!t->tex || alfa == 0)
		return;
	SDL_Rect dst = {marco.x + x * marco.w / ANCHO, marco.y + y * marco.h / ALTO, w * marco.w / ANCHO, h * marco.h / ALTO};
	SDL_SetTextureAlphaMod(t->tex, alfa);
	SDL_RenderCopy(pintor, t->tex, NULL, &dst);
}

void textura_libre(struct capa_textura *t) {
	if (t->tex)
		SDL_DestroyTexture(t->tex);
	t->tex = NULL;
	t->w = t->h = 0;
}

static void esperar_turno(void) {
	const long periodo = (long)(1e9 / HZ);
	siguiente.tv_nsec += periodo;
	while (siguiente.tv_nsec >= 1000000000L) {
		siguiente.tv_nsec -= 1000000000L;
		siguiente.tv_sec++;
	}
	struct timespec ahora;
	clock_gettime(CLOCK_MONOTONIC, &ahora);
	long long falta = (long long)(siguiente.tv_sec - ahora.tv_sec) * 1000000000LL + (siguiente.tv_nsec - ahora.tv_nsec);
	if (falta < -2LL * periodo) {
		/* Se ha quedado atrás (la app estuvo en segundo plano, o el móvil va justo): se sigue desde ahora. */
		siguiente = ahora;
		return;
	}
	if (falta > 0)
		while (clock_nanosleep(CLOCK_MONOTONIC, TIMER_ABSTIME, &siguiente, NULL) == EINTR)
			;
}

void video_presentar(void) {
	tactil_dibujar(pintor, mandos_pulsados_pantalla(), !a_su_ritmo);
	SDL_RenderPresent(pintor);
	if (a_su_ritmo)
		esperar_turno();
}

/* Ir rápido: sin esperar (el ritmo lo lleva juego.c). */
void video_sincronizar(bool con_refresco) {
	if (con_refresco && !a_su_ritmo)
		clock_gettime(CLOCK_MONOTONIC, &siguiente);
	a_su_ritmo = con_refresco;
}
