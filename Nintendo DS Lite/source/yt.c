// YouTube en la DS: buscar con el teclado, la lista de vídeos (con la miniatura arriba) y verlos
// (video.c). Lo hace el puente con la app Puente DS del móvil (puente/youtube.go).
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sistema.h"

enum { V_LISTA, V_BUSCAR, V_VIDEO, V_CODIGO };

typedef struct {
	char clave[12], mini[12], titulo[124], canal[52], dur[12], vistas[32];
} VideoDS;

#define MAX_VIDEOS 40
#define FILA_Y 28
#define FILA_H 32
#define FILAS 5
#define ROJO_YT RGB(255, 40, 40)

static int vista = V_LISTA;
static VideoDS videos[MAX_VIDEOS];
static int nvideos, sel, primero, primero_al_tocar;
static bool hay_mas, pedir_mas, buscando;
static char busqueda[200], estado_yt[16], aviso[120];
static uint32_t aviso_hasta, ultimo_estado, sel_desde;
static Teclado tec;
static Color mini[ANCHO * 144];
static char mini_de[12], mini_pedida[12];
static bool mini_bien;

int yt_n_videos(void) { return nvideos; }
bool yt_mini_puesta(void) { return mini_bien && mini_de[0]; }

static void poner_aviso(const char *s)
{
	snprintf(aviso, sizeof aviso, "%s", s);
	aviso_hasta = ahora_ms + 4000;
}

static void copiar(char *d, int n, const char *s)
{
	snprintf(d, n, "%s", s ? s : "");
}

void yt_entrar(void)
{
	vista = V_LISTA;
	aviso[0] = 0;
	estado_yt[0] = 0;
	mini_pedida[0] = 0;
}

static void procesar(char **l, int n)
{
	if (n <= 0)
		return;
	if (!strncmp(l[0], "ESTADO ", 7)) {
		copiar(estado_yt, sizeof estado_yt, l[0] + 7);
	} else if (!strncmp(l[0], "VIDEOS ", 7)) {
		if (!pedir_mas) {
			nvideos = sel = primero = 0;
			mini_de[0] = 0;
		}
		hay_mas = l[0][7] == '1';
		for (int i = 1; i < n && nvideos < MAX_VIDEOS; i++) {
			char *t[7];
			if (partir(l[i], t, 7) < 7 || strcmp(t[0], "V"))
				continue;
			VideoDS *v = &videos[nvideos++];
			copiar(v->clave, sizeof v->clave, t[1]);
			copiar(v->mini, sizeof v->mini, t[2]);
			copiar(v->titulo, sizeof v->titulo, t[3]);
			copiar(v->canal, sizeof v->canal, t[4]);
			copiar(v->dur, sizeof v->dur, t[5]);
			copiar(v->vistas, sizeof v->vistas, t[6]);
		}
		if (nvideos == 0)
			poner_aviso("No ha salido nada");
		else
			aviso[0] = 0;
		buscando = pedir_mas = false;
		sel_desde = ahora_ms;
	} else if (!strncmp(l[0], "ERROR ", 6)) {
		poner_aviso(l[0] + 6);
		buscando = pedir_mas = false;
		if (mini_pedida[0]) {
			copiar(mini_de, sizeof mini_de, mini_pedida);
			mini_bien = false;
			mini_pedida[0] = 0;
		}
	}
}

static bool red(void)
{
	char **l;
	int n = puente_respuesta(&l);
	bool cambio = false;
	if (n == 0) {
		const uint8_t *d;
		int nd = puente_datos(&d);
		if (nd > 0 && mini_pedida[0]) {
			for (int i = 0; i < ANCHO * 144; i++)
				mini[i] = RGB(0, 0, 0);
			int w, h;
			mini_bien = jpeg_tam(d, nd, &w, &h) && jpeg_pintar(d, nd, mini, ANCHO, 144, (ANCHO - w) / 2, (144 - h) / 2);
			copiar(mini_de, sizeof mini_de, mini_pedida);
			mini_pedida[0] = 0;
		}
		cambio = true;
	} else if (n > 0) {
		procesar(l, n);
		cambio = true;
	}
	if (!puente_libre() || vista == V_VIDEO)
		return cambio;
	char p[260];
	bool listo = !strcmp(estado_yt, "listo");
	if (!estado_yt[0] || ahora_ms - ultimo_estado > (listo ? 30000u : 4000u)) {
		ultimo_estado = ahora_ms;
		puente_pedir("YT ESTADO");
	} else if (!listo) {
		return cambio;
	} else if (buscando && !pedir_mas) {
		snprintf(p, sizeof p, "YT BUSCAR %s", busqueda);
		puente_pedir(p);
	} else if (pedir_mas) {
		puente_pedir("YT MAS");
	} else if (vista == V_LISTA && sel < nvideos && strcmp(videos[sel].mini, mini_de) && strcmp(videos[sel].mini, "-") &&
		ahora_ms - sel_desde > 250) {
		snprintf(p, sizeof p, "IMG %s 256 144 c", videos[sel].mini);
		if (puente_pedir(p))
			copiar(mini_pedida, sizeof mini_pedida, videos[sel].mini);
	}
	return cambio;
}

static void elegir(int i)
{
	if (i < 0 || i >= nvideos)
		return;
	if (i != sel)
		sel_desde = ahora_ms;
	sel = i;
	if (sel < primero)
		primero = sel;
	if (sel >= primero + FILAS)
		primero = sel - FILAS + 1;
	if (sel >= nvideos - 2 && hay_mas && !pedir_mas && !buscando)
		pedir_mas = true;
}

static void ver(int i)
{
	if (i < 0 || i >= nvideos)
		return;
	elegir(i);
	video_abrir(videos[i].clave, videos[i].titulo, 0);
	vista = V_VIDEO;
}

int yt_paso(const Entrada *e)
{
	int r = red() ? CAMBIO : NADA;
	if (aviso[0] && ahora_ms > aviso_hasta) {
		aviso[0] = 0;
		r = CAMBIO;
	}
	if ((imagenes % 15) == 0)
		r = CAMBIO;

	if (vista == V_VIDEO) {
		int v = video_paso(e);
		if (v == SALIR) {
			vista = V_LISTA;
			return CAMBIO;
		}
		return v != NADA ? v : r;
	}
	if (vista == V_CODIGO) {
		int c = codigo_paso(&tec, e, aviso, sizeof aviso);
		if (c == 1 || c == -1)
			vista = V_LISTA;
		if (c == 2)
			aviso_hasta = ahora_ms + 4000;
		return c != 0 ? CAMBIO : r;
	}
	if (vista == V_BUSCAR) {
		if (e->toque_ini && en(e->tx, e->ty, 0, 0, 64, 28)) {
			vista = V_LISTA;
			return CAMBIO;
		}
		int t = teclado_paso(&tec, e);
		if (t == TECLADO_CANCELAR) {
			vista = V_LISTA;
		} else if (t == TECLADO_ACEPTAR) {
			if (tec.texto[0]) {
				copiar(busqueda, sizeof busqueda, tec.texto);
				buscando = true;
				pedir_mas = false;
				poner_aviso("Buscando…");
			}
			vista = V_LISTA;
		}
		return t != TECLADO_NADA ? CAMBIO : r;
	}

	// V_LISTA
	if (toca_atras(e))
		return SALIR;
	int pu = puente_estado();
	if (plat_wifi_estado() != RED_CONECTADA) {
		if ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 36, 240, 46))) {
			wifi_entrar();
			return IR_WIFI;
		}
		return r;
	}
	if (pu == PU_SIN_CODIGO || pu == PU_CODIGO_MAL) {
		if ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 88, 240, 46))) {
			teclado_iniciar(&tec, "");
			vista = V_CODIGO;
			return CAMBIO;
		}
		return r;
	}
	if (pu != PU_LISTO || strcmp(estado_yt, "listo"))
		return r;
	bool tocar_buscar = e->toque_ini && (nvideos == 0 ? en(e->tx, e->ty, 8, 120, 240, 40) : en(e->tx, e->ty, 196, 2, 56, 22));
	if ((e->pulsado & (B_Y | B_X)) || (nvideos == 0 && (e->pulsado & B_A)) || tocar_buscar) {
		teclado_iniciar(&tec, busqueda);
		vista = V_BUSCAR;
		return CAMBIO;
	}
	if (nvideos == 0)
		return r;
	if (e->repetido & (B_ARRIBA | B_ABAJO)) {
		elegir(sel + ((e->repetido & B_ABAJO) ? 1 : -1));
		return CAMBIO;
	}
	if (e->pulsado & B_A) {
		ver(sel);
		return CAMBIO;
	}
	if (e->toque_ini)
		primero_al_tocar = primero;
	if (e->arrastrando && nvideos > FILAS) {
		int p = primero_al_tocar - e->arrastre_dy / FILA_H;
		p = p > nvideos - FILAS ? nvideos - FILAS : p < 0 ? 0 : p;
		if (p != primero) {
			primero = p;
			if (primero + FILAS >= nvideos - 1 && hay_mas && !pedir_mas)
				pedir_mas = true;
			return CAMBIO;
		}
	}
	if (e->toque && en(e->toque_x, e->toque_y, 0, FILA_Y, ANCHO, FILA_H * FILAS)) {
		ver(primero + (e->toque_y - FILA_Y) / FILA_H);
		return CAMBIO;
	}
	return r;
}

// --- Dibujo ---

static void dibujar_aviso(Color *p, int y)
{
	if (!aviso[0])
		return;
	int w = ancho_texto(&letra_peq, aviso) + 16;
	if (w > 240)
		w = 240;
	redondeado(p, (ANCHO - w) / 2, y, w, 18, 9, C_PANEL2, 240);
	texto_corto(p, &letra_peq, (ANCHO - w) / 2 + 8, y + 3, w - 16, aviso, C_AMARILLO);
}

static void fondo(Color *p)
{
	llenar(p, C_FONDO);
	degradado(p, 0, 0, ANCHO, 80, mezclar(C_FONDO, ROJO_YT, 90), C_FONDO);
	barra_arriba(p, "YouTube");
}

void yt_dibujar(Color *arriba, Color *abajo)
{
	if (vista == V_VIDEO) {
		video_dibujar(arriba, abajo);
		return;
	}
	if (vista == V_CODIGO) {
		codigo_dibujar(&tec, arriba, abajo, aviso);
		return;
	}
	if (vista == V_BUSCAR) {
		fondo(arriba);
		texto(arriba, &letra_titulo, 12, 30, "Buscar en YouTube", C_BLANCO);
		texto_parrafo(arriba, &letra_peq, 12, 54, 232, "Escribe lo que quieras ver y pulsa START.", C_SUAVE);
		teclado_dibujar_texto(&tec, arriba, 8, 100, 240, false);
		ayuda(arriba, "START buscar     B borrar     X mayúsculas");
		teclado_dibujar(&tec, abajo, "Buscar");
		return;
	}
	int pu = puente_estado();
	bool listo = plat_wifi_estado() == RED_CONECTADA && pu == PU_LISTO && !strcmp(estado_yt, "listo");
	if (!listo) {
		fondo(arriba);
		const char *t = "Hace falta el puente", *s = "YouTube va con la app Puente DS del móvil (en el mismo wifi): "
			"busca los vídeos y los hace pequeños para la DS.";
		if (plat_wifi_estado() != RED_CONECTADA) {
			t = "Sin wifi";
			s = "Conecta la DS a un wifi sin contraseña (por ejemplo, la zona wifi del móvil). Pulsa A.";
		} else if (pu == PU_LISTO && !strcmp(estado_yt, "sin-ayudante")) {
			t = "Falta la app del móvil";
			s = "Este puente no puede con YouTube: hace falta la app Puente DS de Android.";
		}
		texto(arriba, &letra_titulo, 12, 30, t, C_BLANCO);
		texto_parrafo(arriba, &letra_normal, 12, 56, 232, s, C_SUAVE);
		dibujar_aviso(arriba, 152);
		ayuda(arriba, "A seguir     B volver");
		llenar(abajo, C_FONDO);
		barra_abajo(abajo, "YouTube");
		char d[80];
		fila_estado(abajo, 36, estado_wifi(d, sizeof d), "Wifi", d);
		fila_estado(abajo, 88, estado_puente(d, sizeof d), "Puente", d);
		fila_estado(abajo, 140, pu == PU_LISTO ? 1 : 2, "YouTube", pu == PU_LISTO ? "Preguntando al puente…" : "Cuando esté el puente");
		return;
	}

	if (nvideos == 0) {
		fondo(arriba);
		texto(arriba, &letra_titulo, 12, 34, buscando ? "Buscando…" : "¿Qué quieres ver?", C_BLANCO);
		texto_parrafo(arriba, &letra_normal, 12, 60, 232, buscando ? busqueda
			: "Pulsa A o toca «Buscar» y escribe lo que quieras: el móvil busca los vídeos y te los pasa en pequeño.", C_SUAVE);
		dibujar_aviso(arriba, 152);
		ayuda(arriba, "A buscar     B salir");
		llenar(abajo, C_FONDO);
		barra_abajo(abajo, "YouTube");
		boton(abajo, 8, 120, 240, 40, "Buscar", ROJO_YT, RGB(170, 10, 20), true);
		return;
	}

	const VideoDS *v = &videos[sel];
	llenar(arriba, RGB(0, 0, 0));
	if (mini_bien && !strcmp(mini_de, v->mini))
		memcpy(arriba + 18 * ANCHO, mini, sizeof mini);
	else
		texto_centro(arriba, &letra_normal, ANCHO / 2, 84, "Cargando la miniatura…", C_GRIS);
	if (v->dur[0]) {
		int w = ancho_texto(&letra_peq, v->dur) + 8;
		redondeado(arriba, 250 - w, 142, w, 16, 4, C_NEGRO, 220);
		texto(arriba, &letra_peq, 254 - w, 144, v->dur, C_BLANCO);
	}
	barra_arriba(arriba, "YouTube");
	texto_corto(arriba, &letra_negrita, 6, 163, 244, v->titulo, C_BLANCO);
	char s[96];
	snprintf(s, sizeof s, "%s%s%s", v->canal, v->vistas[0] ? " · " : "", v->vistas);
	texto_corto(arriba, &letra_peq, 6, 178, 244, s, C_GRIS);
	dibujar_aviso(arriba, 120);

	llenar(abajo, C_FONDO);
	barra_abajo(abajo, "YouTube");
	redondeado(abajo, 196, 3, 56, 20, 8, ROJO_YT, 256);
	texto_centro(abajo, &letra_peq, 224, 6, "Buscar", C_BLANCO);
	for (int k = 0; k < FILAS && primero + k < nvideos; k++) {
		int i = primero + k, y = FILA_Y + k * FILA_H;
		const VideoDS *w = &videos[i];
		redondeado(abajo, 4, y + 1, 244, FILA_H - 3, 8, i == sel ? C_PANEL2 : C_PANEL, 256);
		if (i == sel)
			marco(abajo, 4, y + 1, 244, FILA_H - 3, 8, 1, ROJO_YT);
		int wd = ancho_texto(&letra_peq, w->dur);
		texto_corto(abajo, &letra_negrita, 10, y + 2, 226 - wd - 8, w->titulo, C_BLANCO);
		texto_derecha(abajo, &letra_peq, 242, y + 4, w->dur, C_GRIS);
		texto_corto(abajo, &letra_peq, 10, y + 16, 228, w->canal, C_GRIS);
	}
	if (nvideos > FILAS) {
		int alto = FILA_H * FILAS, h = alto * FILAS / nvideos;
		int y = FILA_Y + (alto - h) * primero / (nvideos - FILAS);
		redondeado(abajo, 250, y, 4, h, 2, C_OSCURO, 256);
	}
	if (pedir_mas)
		texto_centro(abajo, &letra_peq, ANCHO / 2, 180, "Cargando más…", C_GRIS);
}
