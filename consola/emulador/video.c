/* La pantalla, con SDL (KMS/DRM y OpenGL ES 2 de la Mali, como el menú de la consola).
 *
 * La imagen del juego se agranda así:
 *  - GB y GBC (160×144): ×3 exacto, 480×432, con los píxeles cuadrados y nítidos.
 *  - GBA (240×160): primero ×3 sin suavizar (720×480) y luego se encoge a 640×427 suavizando. Así
 *    los píxeles se ven nítidos y todos iguales, sin las rayas de un ×2,67 directo.
 *  - Las apps: tal cual (ya mandan 640×480).
 */
#include "panther.h"

static SDL_Window *ventana;
static SDL_Renderer *pintor;
static SDL_Texture *tex_juego, *tex_nitida;
static int img_w, img_h, nit_w, nit_h;
static Uint32 img_formato;
static enum escala img_escala;
static int sal_w = ANCHO, sal_h = ALTO; /* tamaño real de la pantalla */
static double hz = 60;

bool video_abrir(void) {
	SDL_SetHint(SDL_HINT_RENDER_DRIVER, "opengles2");
	SDL_SetHint(SDL_HINT_RENDER_VSYNC, "1");
	SDL_SetHint(SDL_HINT_VIDEO_DOUBLE_BUFFER, "1");
	if (SDL_InitSubSystem(SDL_INIT_VIDEO) != 0) {
		registro("no se puede abrir la pantalla: %s", SDL_GetError());
		return false;
	}
	ventana = SDL_CreateWindow("Panther", 0, 0, ANCHO, ALTO, SDL_WINDOW_FULLSCREEN | SDL_WINDOW_SHOWN);
	if (!ventana) {
		registro("no se puede abrir la pantalla: %s", SDL_GetError());
		return false;
	}
	pintor = SDL_CreateRenderer(ventana, -1, SDL_RENDERER_ACCELERATED | SDL_RENDERER_PRESENTVSYNC | SDL_RENDERER_TARGETTEXTURE);
	if (!pintor) {
		registro("no se puede dibujar: %s", SDL_GetError());
		return false;
	}
	SDL_ShowCursor(SDL_DISABLE);
	SDL_GetRendererOutputSize(pintor, &sal_w, &sal_h);
	SDL_DisplayMode modo;
	if (SDL_GetCurrentDisplayMode(0, &modo) == 0 && modo.refresh_rate > 20)
		hz = modo.refresh_rate;
	SDL_RendererInfo info;
	SDL_GetRendererInfo(pintor, &info);
	registro("pantalla: %dx%d a %.0f Hz (%s, %s)", sal_w, sal_h, hz, SDL_GetCurrentVideoDriver(), info.name);
	SDL_SetRenderDrawColor(pintor, 0, 0, 0, 255);
	SDL_RenderClear(pintor);
	SDL_RenderPresent(pintor);
	return true;
}

void video_cerrar(void) {
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

double video_hz(void) { return hz; }

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
	SDL_SetRenderTarget(pintor, NULL);
	SDL_SetRenderDrawColor(pintor, 0, 0, 0, 255);
	SDL_RenderClear(pintor);
}

void video_dibujar_imagen(void) {
	if (!tex_juego)
		return;
	SDL_Rect dst;
	if (img_escala == ESCALA_ENTERA) {
		int k = sal_w / img_w < sal_h / img_h ? sal_w / img_w : sal_h / img_h;
		if (k < 1)
			k = 1;
		dst.w = img_w * k, dst.h = img_h * k;
	} else {
		double k = (double)sal_w / img_w < (double)sal_h / img_h ? (double)sal_w / img_w : (double)sal_h / img_h;
		dst.w = (int)(img_w * k + 0.5), dst.h = (int)(img_h * k + 0.5);
	}
	dst.x = (sal_w - dst.w) / 2, dst.y = (sal_h - dst.h) / 2;
	if (img_escala != ESCALA_NITIDA) {
		SDL_RenderCopy(pintor, tex_juego, NULL, &dst);
		return;
	}
	int p = (dst.h + img_h - 1) / img_h; /* el entero que se pasa justo */
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

void video_oscurecer(uint8_t alfa) {
	SDL_SetRenderDrawBlendMode(pintor, SDL_BLENDMODE_BLEND);
	SDL_SetRenderDrawColor(pintor, 0, 0, 0, alfa);
	SDL_RenderFillRect(pintor, NULL);
}

void textura_subir_rgba(struct capa_textura *t, const uint8_t *rgba, int w, int h) {
	if (!pintor || w <= 0 || h <= 0)
		return;
	if (!t->tex || t->w != w || t->h != h) {
		if (t->tex)
			SDL_DestroyTexture(t->tex);
		t->tex = SDL_CreateTexture(pintor, SDL_PIXELFORMAT_RGBA32, SDL_TEXTUREACCESS_STATIC, w, h);
		if (!t->tex)
			return;
		SDL_SetTextureBlendMode(t->tex, SDL_BLENDMODE_BLEND);
		SDL_SetTextureScaleMode(t->tex, SDL_ScaleModeLinear);
		t->w = w, t->h = h;
	}
	SDL_UpdateTexture(t->tex, NULL, rgba, w * 4);
}

void textura_subir(struct capa_textura *t, const struct lienzo *l) { textura_subir_rgba(t, l->px, l->w, l->h); }

/* x, y, w y h van en píxeles de 640×480 (se ajustan si la pantalla fuera de otro tamaño). */
void textura_dibujar(struct capa_textura *t, int x, int y, int w, int h, uint8_t alfa) {
	if (!t->tex || alfa == 0)
		return;
	SDL_Rect dst = {x * sal_w / ANCHO, y * sal_h / ALTO, w * sal_w / ANCHO, h * sal_h / ALTO};
	SDL_SetTextureAlphaMod(t->tex, alfa);
	SDL_RenderCopy(pintor, t->tex, NULL, &dst);
}

void textura_libre(struct capa_textura *t) {
	if (t->tex)
		SDL_DestroyTexture(t->tex);
	t->tex = NULL;
	t->w = t->h = 0;
}

void video_presentar(void) { SDL_RenderPresent(pintor); }

/* Ir rápido: sin esperar al refresco de la pantalla (el ritmo lo lleva el programa). */
void video_sincronizar(bool con_refresco) {
	static int actual = 1;
	if (pintor && actual != (int)con_refresco) {
		SDL_RenderSetVSync(pintor, con_refresco ? 1 : 0);
		actual = con_refresco;
	}
}
