// Instagram en la DS: el inicio (las fotos arriba y lo demás abajo), las historias y los mensajes,
// con lo que manda el puente (puente/instagram.go). La cuenta es la de la dueña de la DS: entra en
// Instagram desde la app Puente DS de su móvil, y el puente guarda la sesión.
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sistema.h"

enum { T_INICIO, T_HISTORIAS, T_MENSAJES };
enum { V_TAB, V_HISTORIA, V_CHAT, V_ESCRIBIR, V_VIDEO, V_CODIGO };

typedef struct {
	char clave[12], usuario[44], texto[304], cuando[12];
	int likes, comentarios, nmedios;
	bool megusta;
	char medios[10][28];
} PostDS;

typedef struct {
	char id[44], usuario[44];
	bool vista;
} BandejaDS;

typedef struct {
	char id[84], nombre[64], cuando[12], previa[96];
	bool sinleer;
} ChatIG;

#define MAX_POSTS 24
#define MAX_BANDEJAS 30
#define MAX_ITEMS 30
#define MAX_CHATS 30
#define MAX_MSGS 40
#define FILA_Y 52
#define FILA_H 28
#define FILAS 4
#define ROSA RGB(225, 48, 108)
#define NARANJA RGB(247, 119, 55)
#define MORADO RGB(131, 58, 180)

static int tab = T_INICIO, vista = V_TAB;
static char estado_ig[16], usuario_ig[44], aviso[120];
static uint32_t aviso_hasta, ultimo_estado;

static PostDS posts[MAX_POSTS];
static int nposts, post_sel, medio_sel;
static bool hay_mas, pedir_inicio = true, pedir_mas;
static char megusta_pend[16];

static BandejaDS bandejas[MAX_BANDEJAS];
static int nbandejas, band_sel, band_primero;
static bool pedir_historias = true;
static char items[MAX_ITEMS][28], historia_de[44];
static int nitems, item_sel;
static bool pedir_historia;
static uint32_t item_desde;

static ChatIG chats[MAX_CHATS];
static int nchats, chat_sel, chat_primero;
static bool pedir_chats = true, pedir_chat;
static MsgDS msgs[MAX_MSGS];
static int nmsgs, desplazar;
static char abierto_id[84], abierto_nombre[64], pendiente[420];
static Teclado tec;

// la foto que se ve (la de la publicación o la de la historia)
static Color foto[ANCHO * ALTO];
static char foto_de[12], foto_pedida[12];
static bool foto_bien;
static uint32_t foto_en;

static const char *const rapidas[4] = {"Vale", "Jajaja", "¡Qué guay!", "Luego te digo"};

static void pasar_item(int d);

int ig_n_posts(void) { return nposts; }
int ig_n_chats(void) { return nchats; }
const char *ig_ultimo_texto(void) { return nmsgs > 0 ? msgs[nmsgs - 1].texto : ""; }
bool ig_foto_puesta(void) { return foto_bien && foto_de[0]; }

static void poner_aviso(const char *s)
{
	snprintf(aviso, sizeof aviso, "%s", s);
	aviso_hasta = ahora_ms + 4000;
}

static void copiar(char *d, int n, const char *s)
{
	snprintf(d, n, "%s", s ? s : "");
}

static bool lista(void)
{
	return !strcmp(estado_ig, "bien") || !strcmp(estado_ig, "limite");
}

void ig_entrar(void)
{
	vista = V_TAB;
	aviso[0] = 0;
	estado_ig[0] = 0;
	foto_pedida[0] = 0;
	pedir_chats = pedir_historias = true;
	if (nposts == 0)
		pedir_inicio = true;
}

// la foto de un medio («fk12» o «vk3:k4»): la clave de la foto
static void foto_de_medio(const char *m, char *clave, int n)
{
	clave[0] = 0;
	if (!m[0])
		return;
	const char *f = m + 1, *dos = strchr(f, ':');
	int l = dos ? (int)(dos - f) : (int)strlen(f);
	if (l >= n)
		l = n - 1;
	memcpy(clave, f, l);
	clave[l] = 0;
	if (!strcmp(clave, "-"))
		clave[0] = 0;
}

static const char *video_de_medio(const char *m)
{
	const char *dos = m[0] == 'v' ? strchr(m, ':') : NULL;
	return dos && dos[1] && strcmp(dos + 1, "-") ? dos + 1 : NULL;
}

// la foto que toca ahora
static const char *medio_actual(void)
{
	if (vista == V_HISTORIA)
		return item_sel < nitems ? items[item_sel] : "";
	if (tab == T_INICIO && post_sel < nposts && medio_sel < posts[post_sel].nmedios)
		return posts[post_sel].medios[medio_sel];
	return "";
}

static void leer_post(PostDS *p, char *l)
{
	char *t[9];
	if (partir(l, t, 9) < 9)
		return;
	copiar(p->clave, sizeof p->clave, t[1]);
	copiar(p->usuario, sizeof p->usuario, t[2]);
	copiar(p->texto, sizeof p->texto, t[3]);
	p->likes = atoi(t[4]);
	p->comentarios = atoi(t[5]);
	copiar(p->cuando, sizeof p->cuando, t[6]);
	p->megusta = t[7][0] == '1';
	p->nmedios = 0;
	char *s = t[8];
	while (*s && p->nmedios < 10) {
		char *esp = strchr(s, ' ');
		if (esp)
			*esp = 0;
		copiar(p->medios[p->nmedios++], sizeof p->medios[0], s);
		if (!esp)
			break;
		s = esp + 1;
	}
}

static void procesar(char **l, int n)
{
	if (n <= 0)
		return;
	if (!strncmp(l[0], "ESTADO ", 7)) {
		char *t[2];
		int k = partir(l[0] + 7, t, 2);
		bool antes = lista();
		copiar(estado_ig, sizeof estado_ig, t[0]);
		copiar(usuario_ig, sizeof usuario_ig, k > 1 ? t[1] : "");
		if (!antes && lista())
			pedir_inicio = pedir_historias = pedir_chats = true;
	} else if (!strncmp(l[0], "INICIO ", 7)) {
		hay_mas = l[0][7] == '1';
		if (!pedir_mas) { // el principio: lo de antes fuera
			nposts = 0;
			post_sel = medio_sel = 0;
		}
		for (int i = 1; i < n; i++) {
			if (nposts == MAX_POSTS) { // lleno: fuera las primeras
				int quitar = 8;
				memmove(posts, posts + quitar, (MAX_POSTS - quitar) * sizeof posts[0]);
				nposts -= quitar;
				post_sel = post_sel >= quitar ? post_sel - quitar : 0;
			}
			if (!strncmp(l[i], "P\t", 2))
				leer_post(&posts[nposts++], l[i]);
		}
		pedir_inicio = pedir_mas = false;
	} else if (!strcmp(l[0], "HISTORIAS")) {
		nbandejas = 0;
		for (int i = 1; i < n && nbandejas < MAX_BANDEJAS; i++) {
			char *t[4];
			if (partir(l[i], t, 4) < 4 || strcmp(t[0], "H"))
				continue;
			BandejaDS *b = &bandejas[nbandejas++];
			copiar(b->id, sizeof b->id, t[1]);
			copiar(b->usuario, sizeof b->usuario, t[2]);
			b->vista = t[3][0] == '1';
		}
		if (band_sel >= nbandejas)
			band_sel = 0;
		pedir_historias = false;
	} else if (!strncmp(l[0], "HISTORIA", 8)) {
		nitems = 0;
		for (int i = 1; i < n && nitems < MAX_ITEMS; i++) {
			char *t[3];
			if (partir(l[i], t, 3) < 2 || strcmp(t[0], "S"))
				continue;
			copiar(items[nitems++], sizeof items[0], t[1]);
		}
		item_sel = 0;
		item_desde = ahora_ms;
		pedir_historia = false;
		if (nitems == 0) {
			poner_aviso("No tiene historias ahora");
			vista = V_TAB;
		} else {
			pasar_item(0);
		}
	} else if (!strcmp(l[0], "CHATS")) {
		nchats = 0;
		for (int i = 1; i < n && nchats < MAX_CHATS; i++) {
			char *t[6];
			if (partir(l[i], t, 6) < 6 || strcmp(t[0], "C"))
				continue;
			ChatIG *c = &chats[nchats++];
			copiar(c->id, sizeof c->id, t[1]);
			copiar(c->nombre, sizeof c->nombre, t[2]);
			c->sinleer = t[3][0] == '1';
			copiar(c->cuando, sizeof c->cuando, t[4]);
			copiar(c->previa, sizeof c->previa, t[5]);
		}
		if (chat_sel >= nchats)
			chat_sel = nchats > 0 ? nchats - 1 : 0;
		pedir_chats = false;
	} else if (!strncmp(l[0], "CHAT", 4)) {
		nmsgs = 0;
		for (int i = 1; i < n && nmsgs < MAX_MSGS; i++) {
			char *m[5];
			if (partir(l[i], m, 5) < 5 || strcmp(m[0], "M"))
				continue;
			MsgDS *g = &msgs[nmsgs++];
			g->mio = m[1][0] == '1';
			copiar(g->hora, sizeof g->hora, m[2]);
			copiar(g->de, sizeof g->de, m[3]);
			copiar(g->texto, sizeof g->texto, m[4]);
		}
		pedir_chat = false;
	} else if (!strcmp(l[0], "OK")) {
		if (megusta_pend[0] && !megusta_pend[12]) {
			megusta_pend[0] = 0;
		} else {
			poner_aviso("Enviado");
			pedir_chat = pedir_chats = true;
		}
	} else if (!strncmp(l[0], "ERROR ", 6)) {
		poner_aviso(l[0] + 6);
		if (megusta_pend[0] && !megusta_pend[12]) { // no ha ido: se deja como estaba
			for (int i = 0; i < nposts; i++)
				if (!strcmp(posts[i].clave, megusta_pend)) {
					posts[i].megusta = !posts[i].megusta;
					posts[i].likes += posts[i].megusta ? 1 : -1;
				}
			megusta_pend[0] = 0;
		}
		pedir_inicio = pedir_mas = pedir_historias = pedir_historia = pedir_chats = pedir_chat = false;
		if (foto_pedida[0]) {
			copiar(foto_de, sizeof foto_de, foto_pedida); // no se vuelve a pedir sin parar
			foto_bien = false;
			foto_pedida[0] = 0;
			foto_en = ahora_ms;
		}
		ultimo_estado = 0; // a ver qué pasa con la sesión
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
		if (nd > 0 && foto_pedida[0]) {
			for (int i = 0; i < ANCHO * ALTO; i++)
				foto[i] = RGB(0, 0, 0);
			int w, h;
			foto_bien = jpeg_tam(d, nd, &w, &h) && jpeg_pintar(d, nd, foto, ANCHO, ALTO, (ANCHO - w) / 2, (ALTO - h) / 2);
			copiar(foto_de, sizeof foto_de, foto_pedida);
			foto_pedida[0] = 0;
			foto_en = ahora_ms;
		}
		cambio = true;
	} else if (n > 0) {
		procesar(l, n);
		cambio = true;
	}
	if (!puente_libre() || vista == V_VIDEO)
		return cambio;
	char p[480];
	bool listo = lista();
	char f[12];
	foto_de_medio(medio_actual(), f, sizeof f);
	if (pendiente[0]) {
		snprintf(p, sizeof p, "IG ENVIAR %s\t%s", abierto_id, pendiente);
		if (puente_pedir(p))
			pendiente[0] = 0;
	} else if (megusta_pend[0] && megusta_pend[12]) {
		snprintf(p, sizeof p, "IG MEGUSTA %s %c", megusta_pend, megusta_pend[12]);
		megusta_pend[12] = 0;
		puente_pedir(p);
	} else if (!estado_ig[0] || ahora_ms - ultimo_estado > (listo ? 20000u : 3000u)) {
		ultimo_estado = ahora_ms;
		puente_pedir("IG ESTADO");
	} else if (!listo) {
		return cambio;
	} else if (vista == V_HISTORIA && pedir_historia) {
		snprintf(p, sizeof p, "IG HISTORIA %s", historia_de);
		puente_pedir(p);
	} else if (f[0] && strcmp(f, foto_de) && (vista == V_HISTORIA || (vista == V_TAB && tab == T_INICIO))) {
		snprintf(p, sizeof p, "IMG %s 256 192", f);
		if (puente_pedir(p))
			copiar(foto_pedida, sizeof foto_pedida, f);
	} else if (tab == T_INICIO && (pedir_inicio || pedir_mas)) {
		puente_pedir(pedir_mas ? "IG MAS" : "IG INICIO");
	} else if (tab == T_HISTORIAS && pedir_historias) {
		puente_pedir("IG HISTORIAS");
	} else if (tab == T_MENSAJES && (vista == V_CHAT || vista == V_ESCRIBIR) && pedir_chat) {
		snprintf(p, sizeof p, "IG CHAT %s", abierto_id);
		puente_pedir(p);
	} else if (tab == T_MENSAJES && pedir_chats) {
		puente_pedir("IG CHATS");
	}
	return cambio;
}

static void cambiar_post(int d)
{
	int s = post_sel + d;
	if (s < 0)
		s = 0;
	if (s >= nposts) {
		s = nposts - 1;
		if (hay_mas && !pedir_mas) {
			pedir_mas = true;
			poner_aviso("Cargando más…");
		}
	}
	if (s != post_sel) {
		post_sel = s;
		medio_sel = 0;
	}
	if (post_sel >= nposts - 2 && hay_mas && !pedir_mas)
		pedir_mas = true;
}

static void me_gusta(void)
{
	if (post_sel >= nposts || megusta_pend[0])
		return;
	PostDS *p = &posts[post_sel];
	p->megusta = !p->megusta;
	p->likes += p->megusta ? 1 : -1;
	memset(megusta_pend, 0, sizeof megusta_pend);
	copiar(megusta_pend, 12, p->clave);
	megusta_pend[12] = p->megusta ? '1' : '0';
}

static void ver_video(const char *medio, const char *titulo)
{
	const char *v = video_de_medio(medio);
	if (!v)
		return;
	video_abrir(v, titulo, 0);
	vista = V_VIDEO;
}

static void abrir_chat(int i)
{
	if (i < 0 || i >= nchats)
		return;
	chat_sel = i;
	copiar(abierto_id, sizeof abierto_id, chats[i].id);
	copiar(abierto_nombre, sizeof abierto_nombre, chats[i].nombre);
	nmsgs = desplazar = 0;
	pedir_chat = true;
	vista = V_CHAT;
}

static void abrir_historia(int i)
{
	if (i < 0 || i >= nbandejas)
		return;
	band_sel = i;
	copiar(historia_de, sizeof historia_de, bandejas[i].id);
	bandejas[i].vista = true;
	nitems = item_sel = 0;
	pedir_historia = true;
	vista = V_HISTORIA;
}

// pasar_item: a la siguiente historia (o fuera, si era la última).
static void pasar_item(int d)
{
	video_cerrar();
	item_sel += d;
	item_desde = ahora_ms;
	if (item_sel < 0)
		item_sel = 0;
	if (item_sel >= nitems) {
		vista = V_TAB;
		return;
	}
	if (items[item_sel][0] == 'v') {
		const char *v = video_de_medio(items[item_sel]);
		if (v)
			video_abrir(v, historia_de, 0);
	}
}

static int lista_paso(const Entrada *e, int *sel, int *primero, int n)
{
	if (e->repetido & (B_ARRIBA | B_ABAJO)) {
		if (n > 0) {
			*sel += (e->repetido & B_ABAJO) ? 1 : -1;
			*sel = *sel < 0 ? 0 : *sel >= n ? n - 1 : *sel;
			if (*sel < *primero)
				*primero = *sel;
			if (*sel >= *primero + FILAS)
				*primero = *sel - FILAS + 1;
		}
		return 0;
	}
	if (e->toque && en(e->toque_x, e->toque_y, 0, FILA_Y, ANCHO, FILA_H * FILAS)) {
		int i = *primero + (e->toque_y - FILA_Y) / FILA_H;
		if (i < n)
			return i + 1;
	}
	if ((e->pulsado & B_A) && n > 0)
		return *sel + 1;
	return -1;
}

int ig_paso(const Entrada *e)
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
			vista = V_TAB;
			return CAMBIO;
		}
		return v != NADA ? v : r;
	}
	if (vista == V_CODIGO) {
		int c = codigo_paso(&tec, e, aviso, sizeof aviso);
		if (c == 1 || c == -1)
			vista = V_TAB;
		if (c == 2)
			aviso_hasta = ahora_ms + 4000;
		return c != 0 ? CAMBIO : r;
	}
	if (vista == V_ESCRIBIR) {
		if (e->toque_ini && en(e->tx, e->ty, 0, 0, 64, 28)) {
			vista = V_CHAT;
			return CAMBIO;
		}
		int t = teclado_paso(&tec, e);
		if (t == TECLADO_CANCELAR) {
			vista = V_CHAT;
		} else if (t == TECLADO_ACEPTAR) {
			if (tec.texto[0]) {
				copiar(pendiente, sizeof pendiente, tec.texto);
				desplazar = 0;
			}
			vista = V_CHAT;
		}
		return t != TECLADO_NADA ? CAMBIO : r;
	}
	if (vista == V_CHAT) {
		if (toca_atras(e)) {
			vista = V_TAB;
			pedir_chats = true;
			return CAMBIO;
		}
		if ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 34, 240, 38))) {
			teclado_iniciar(&tec, "");
			vista = V_ESCRIBIR;
			return CAMBIO;
		}
		for (int i = 0; i < 4; i++)
			if (e->toque_ini && en(e->tx, e->ty, 8 + (i % 2) * 124, 80 + (i / 2) * 40, 116, 34)) {
				copiar(pendiente, sizeof pendiente, rapidas[i]);
				desplazar = 0;
				return CAMBIO;
			}
		if ((e->repetido & B_ARRIBA) || (e->toque_ini && en(e->tx, e->ty, 8, 162, 116, 26))) {
			desplazar += 40;
			return CAMBIO;
		}
		if ((e->repetido & B_ABAJO) || (e->toque_ini && en(e->tx, e->ty, 132, 162, 116, 26))) {
			desplazar = desplazar > 40 ? desplazar - 40 : 0;
			return CAMBIO;
		}
		if (e->pulsado & B_X) {
			pedir_chat = true;
			return CAMBIO;
		}
		return r;
	}
	if (vista == V_HISTORIA) {
		bool es_video = item_sel < nitems && items[item_sel][0] == 'v';
		if (es_video && video_paso(NULL) != NADA)
			r = CAMBIO;
		if (toca_atras(e)) {
			video_cerrar();
			vista = V_TAB;
			return CAMBIO;
		}
		bool siguiente = (e->pulsado & (B_A | B_DER)) || (e->toque && e->toque_x >= 128);
		bool anterior = (e->pulsado & B_IZQ) || (e->toque && e->toque_x < 128);
		if (nitems > 0 && (siguiente || anterior)) {
			pasar_item(siguiente ? 1 : -1);
			return CAMBIO;
		}
		// solas: las fotos 5 s después de verse y los vídeos al acabar
		char f[12];
		foto_de_medio(item_sel < nitems ? items[item_sel] : "", f, sizeof f);
		bool foto_vista = f[0] && !strcmp(f, foto_de) && ahora_ms - foto_en > 5000 && ahora_ms - item_desde > 5000;
		if (nitems > 0 && ((!es_video && foto_vista) || (es_video && video_acabado()))) {
			pasar_item(1);
			return CAMBIO;
		}
		return r;
	}

	// V_TAB
	if (toca_atras(e))
		return SALIR;
	if (plat_wifi_estado() != RED_CONECTADA || puente_estado() != PU_LISTO || !lista()) {
		if (plat_wifi_estado() != RED_CONECTADA && ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 36, 240, 46)))) {
			wifi_entrar();
			return IR_WIFI;
		}
		int pu = puente_estado();
		if ((pu == PU_SIN_CODIGO || pu == PU_CODIGO_MAL) && ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 88, 240, 46)))) {
			teclado_iniciar(&tec, "");
			vista = V_CODIGO;
			return CAMBIO;
		}
		return r;
	}
	// las pestañas
	int t = -1;
	if (e->pulsado & B_L)
		t = tab > 0 ? tab - 1 : tab;
	if (e->pulsado & B_R)
		t = tab < 2 ? tab + 1 : tab;
	if (e->toque_ini && en(e->tx, e->ty, 0, 28, ANCHO, 22))
		t = e->tx * 3 / ANCHO;
	if (t >= 0 && t != tab) {
		tab = t;
		if (tab == T_HISTORIAS)
			pedir_historias = true;
		if (tab == T_MENSAJES)
			pedir_chats = true;
		return CAMBIO;
	}
	if (tab == T_INICIO) {
		if (e->repetido & B_ABAJO) {
			cambiar_post(1);
			return CAMBIO;
		}
		if (e->repetido & B_ARRIBA) {
			cambiar_post(-1);
			return CAMBIO;
		}
		if (post_sel < nposts) {
			PostDS *p = &posts[post_sel];
			if ((e->pulsado & B_DER) && medio_sel < p->nmedios - 1) {
				medio_sel++;
				return CAMBIO;
			}
			if ((e->pulsado & B_IZQ) && medio_sel > 0) {
				medio_sel--;
				return CAMBIO;
			}
			if ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 152, 76, 32))) {
				me_gusta();
				return CAMBIO;
			}
			bool video = medio_sel < p->nmedios && p->medios[medio_sel][0] == 'v';
			if (video && ((e->pulsado & B_X) || (e->toque_ini && en(e->tx, e->ty, 90, 152, 76, 32)))) {
				ver_video(p->medios[medio_sel], p->usuario);
				return CAMBIO;
			}
			if (e->toque_ini && en(e->tx, e->ty, 172, 152, 36, 32)) {
				cambiar_post(-1);
				return CAMBIO;
			}
			if (e->toque_ini && en(e->tx, e->ty, 212, 152, 36, 32)) {
				cambiar_post(1);
				return CAMBIO;
			}
		}
		if (e->pulsado & B_Y) {
			pedir_inicio = true;
			poner_aviso("Actualizando…");
			return CAMBIO;
		}
		return r;
	}
	if (tab == T_HISTORIAS) {
		int i = lista_paso(e, &band_sel, &band_primero, nbandejas);
		if (i > 0) {
			abrir_historia(i - 1);
			return CAMBIO;
		}
		return i == 0 ? CAMBIO : r;
	}
	// T_MENSAJES
	if (e->pulsado & B_Y) {
		pedir_chats = true;
		poner_aviso("Actualizando…");
		return CAMBIO;
	}
	int i = lista_paso(e, &chat_sel, &chat_primero, nchats);
	if (i > 0) {
		abrir_chat(i - 1);
		return CAMBIO;
	}
	return i == 0 ? CAMBIO : r;
}

// --- Dibujo ---

static void corazon(Color *p, int cx, int cy, Color c)
{
	circulo(p, cx - 4, cy - 2, 4.5f, c, 256);
	circulo(p, cx + 4, cy - 2, 4.5f, c, 256);
	triangulo(p, cx - 8.5f, cy, cx + 8.5f, cy, cx, cy + 9, c);
}

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

static void fondo(Color *p, const char *titulo)
{
	llenar(p, C_FONDO);
	degradado(p, 0, 0, ANCHO, 80, mezclar(C_FONDO, ROSA, 110), C_FONDO);
	barra_arriba(p, titulo);
}

static void pestanas(Color *abajo)
{
	static const char *const nombres[3] = {"Inicio", "Historias", "Mensajes"};
	for (int i = 0; i < 3; i++) {
		int x = i * ANCHO / 3;
		rect(abajo, x, 28, ANCHO / 3, 22, i == tab ? C_PANEL2 : C_FONDO);
		texto_centro(abajo, &letra_negrita, x + ANCHO / 6, 31, nombres[i], i == tab ? C_BLANCO : C_GRIS);
		if (i == tab)
			rect(abajo, x + 8, 48, ANCHO / 3 - 16, 2, ROSA);
	}
}

static void dibujar_foto(Color *arriba)
{
	if (foto_bien && foto_de[0]) {
		memcpy(arriba, foto, sizeof foto);
	} else {
		llenar(arriba, RGB(0, 0, 0));
		texto_centro(arriba, &letra_normal, ANCHO / 2, 88, foto_de[0] ? "Esta foto no se puede ver" : "Cargando la foto…", C_GRIS);
	}
}

static void dibujar_sin(Color *arriba, Color *abajo)
{
	fondo(arriba, "Instagram");
	const char *t = "Hace falta el puente", *s = "El puente es la app Puente DS del móvil (en el mismo wifi): "
		"lleva tu Instagram y la DS enseña lo que le pasa.";
	if (plat_wifi_estado() != RED_CONECTADA) {
		t = "Sin wifi";
		s = "Conecta la DS a un wifi sin contraseña (por ejemplo, la zona wifi del móvil). Pulsa A.";
	} else if (puente_estado() == PU_LISTO && estado_ig[0] && !lista()) {
		if (!strcmp(estado_ig, "comprobando")) {
			t = "Entrando…";
			s = "El puente está mirando con Instagram que la sesión vale.";
		} else if (!strcmp(estado_ig, "sin-cuenta")) {
			t = "Entra en Instagram";
			s = "En el móvil, abre la app Puente DS y pulsa «Entrar en Instagram». Se hace una sola vez.";
		} else if (!strcmp(estado_ig, "reto")) {
			t = "Instagram pide confirmar";
			s = "Abre Instagram en el móvil, acepta el aviso que salga y vuelve a entrar desde la app Puente DS.";
		} else {
			t = "La sesión ha caducado";
			s = "Vuelve a entrar desde la app Puente DS del móvil («Entrar en Instagram»).";
		}
	}
	texto(arriba, &letra_titulo, 12, 30, t, C_BLANCO);
	texto_parrafo(arriba, &letra_normal, 12, 56, 232, s, C_SUAVE);
	dibujar_aviso(arriba, 152);
	ayuda(arriba, "B volver");
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, "Instagram");
	char d[80];
	fila_estado(abajo, 36, estado_wifi(d, sizeof d), "Wifi", d);
	fila_estado(abajo, 88, estado_puente(d, sizeof d), "Puente", d);
	fila_estado(abajo, 140, puente_estado() == PU_LISTO ? 1 : 2, "Instagram",
		puente_estado() != PU_LISTO ? "Cuando esté el puente" : !estado_ig[0] ? "Preguntando al puente…" : "Falta entrar desde el móvil");
}

static void dibujar_inicio(Color *arriba, Color *abajo)
{
	if (nposts == 0) {
		fondo(arriba, "Instagram");
		texto_centro(arriba, &letra_normal, ANCHO / 2, 90, pedir_inicio ? "Cargando tu inicio…" : "No hay nada nuevo", C_GRIS);
	} else {
		dibujar_foto(arriba);
		const PostDS *p = &posts[post_sel];
		if (p->nmedios > 1) {
			for (int i = 0; i < p->nmedios; i++)
				circulo(arriba, ANCHO / 2 + (i * 2 - p->nmedios + 1) * 5, 184, 2.5f, i == medio_sel ? C_AZUL : C_GRIS, 256);
		}
		if (medio_sel < p->nmedios && p->medios[medio_sel][0] == 'v') {
			circulo(arriba, ANCHO / 2, ALTO / 2, 20, C_NEGRO, 160);
			triangulo(arriba, ANCHO / 2 - 7, ALTO / 2 - 11, ANCHO / 2 - 7, ALTO / 2 + 11, ANCHO / 2 + 12, ALTO / 2, C_BLANCO);
		}
	}
	dibujar_aviso(arriba, 160);

	llenar(abajo, C_FONDO);
	barra_abajo(abajo, usuario_ig[0] ? usuario_ig : "Instagram");
	pestanas(abajo);
	if (nposts == 0)
		return;
	const PostDS *p = &posts[post_sel];
	char s[64];
	snprintf(s, sizeof s, "@%s", p->usuario);
	texto_corto(abajo, &letra_negrita, 10, 56, 180, s, C_BLANCO);
	texto_derecha(abajo, &letra_peq, 246, 58, p->cuando, C_GRIS);
	// el texto (lo que no quepa lo tapa lo de abajo)
	texto_parrafo(abajo, &letra_peq, 10, 74, 236, p->texto, C_SUAVE);
	rect(abajo, 0, 126, ANCHO, ALTO - 126, C_FONDO);
	snprintf(s, sizeof s, "%d me gusta · %d comentarios", p->likes, p->comentarios);
	texto(abajo, &letra_peq, 10, 132, s, C_GRIS);
	// botones
	redondeado(abajo, 8, 152, 76, 32, 10, p->megusta ? ROSA : C_PANEL2, 256);
	corazon(abajo, 26, 166, C_BLANCO);
	texto(abajo, &letra_peq, 40, 161, p->megusta ? "Te gusta" : "Me gusta", C_BLANCO);
	bool video = medio_sel < p->nmedios && p->medios[medio_sel][0] == 'v';
	if (video) {
		redondeado(abajo, 90, 152, 76, 32, 10, NARANJA, 256);
		triangulo(abajo, 100, 160, 100, 176, 112, 168, C_BLANCO);
		texto(abajo, &letra_peq, 118, 161, "Verlo", C_BLANCO);
	} else if (p->nmedios > 1) {
		snprintf(s, sizeof s, "◄ %d/%d ►", medio_sel + 1, p->nmedios);
		texto_centro(abajo, &letra_peq, 128, 161, s, C_SUAVE);
	}
	boton(abajo, 172, 152, 36, 32, "▲", C_PANEL2, C_PANEL, false);
	boton(abajo, 212, 152, 36, 32, "▼", C_PANEL2, C_PANEL, false);
}

static void filas(Color *abajo, int n, int sel, int primero, bool bandeja)
{
	for (int k = 0; k < FILAS && primero + k < n; k++) {
		int i = primero + k, y = FILA_Y + k * FILA_H;
		redondeado(abajo, 4, y + 1, 244, FILA_H - 3, 8, i == sel ? C_PANEL2 : C_PANEL, 256);
		if (i == sel)
			marco(abajo, 4, y + 1, 244, FILA_H - 3, 8, 1, ROSA);
		if (bandeja) {
			const BandejaDS *b = &bandejas[i];
			aro(abajo, 20, y + 13, 8, 2.5f, b->vista ? C_OSCURO : ROSA, 0);
			texto_corto(abajo, &letra_negrita, 36, y + 6, 200, b->usuario, b->vista ? C_GRIS : C_BLANCO);
		} else {
			const ChatIG *c = &chats[i];
			int wh = ancho_texto(&letra_peq, c->cuando);
			texto_corto(abajo, &letra_negrita, 10, y + 1, 226 - wh - 10, c->nombre, C_BLANCO);
			texto_derecha(abajo, &letra_peq, 242, y + 3, c->cuando, c->sinleer ? ROSA : C_GRIS);
			texto_corto(abajo, &letra_peq, 10, y + 14, 216, c->previa, c->sinleer ? C_SUAVE : C_GRIS);
			if (c->sinleer)
				circulo(abajo, 238, y + 19, 4, ROSA, 256);
		}
	}
	if (n > FILAS) {
		int alto = FILA_H * FILAS, h = alto * FILAS / n;
		int y = FILA_Y + (alto - h) * primero / (n - FILAS);
		redondeado(abajo, 250, y, 4, h, 2, C_OSCURO, 256);
	}
}

static void dibujar_historias(Color *arriba, Color *abajo)
{
	fondo(arriba, "Historias");
	if (nbandejas == 0) {
		texto_centro(arriba, &letra_normal, ANCHO / 2, 90, pedir_historias ? "Cargando las historias…" : "Nadie tiene historias ahora", C_GRIS);
	} else {
		const BandejaDS *b = &bandejas[band_sel];
		aro(arriba, ANCHO / 2, 78, 34, 4, b->vista ? C_OSCURO : ROSA, 0);
		circulo(arriba, ANCHO / 2, 78, 28, C_PANEL2, 256);
		char ini[4] = {b->usuario[0] ? (char)(b->usuario[0] >= 'a' && b->usuario[0] <= 'z' ? b->usuario[0] - 32 : b->usuario[0]) : '?', 0};
		texto_centro(arriba, &letra_grande, ANCHO / 2, 64, ini, C_BLANCO);
		texto_centro(arriba, &letra_negrita, ANCHO / 2, 122, b->usuario, C_BLANCO);
		texto_centro(arriba, &letra_peq, ANCHO / 2, 140, b->vista ? "Ya las has visto" : "Historias nuevas", b->vista ? C_GRIS : ROSA);
	}
	dibujar_aviso(arriba, 156);
	ayuda(arriba, "A ver     L/R pestañas     B salir");
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, usuario_ig[0] ? usuario_ig : "Instagram");
	pestanas(abajo);
	filas(abajo, nbandejas, band_sel, band_primero, true);
}

static void dibujar_historia(Color *arriba, Color *abajo)
{
	bool es_video = item_sel < nitems && items[item_sel][0] == 'v';
	if (es_video)
		video_dibujar(arriba, NULL);
	else if (nitems == 0) {
		llenar(arriba, RGB(0, 0, 0));
		texto_centro(arriba, &letra_normal, ANCHO / 2, 88, "Cargando…", C_GRIS);
	} else
		dibujar_foto(arriba);
	// las rayitas de arriba: cuántas hay y por cuál va
	if (nitems > 0) {
		int w = (ANCHO - 8) / nitems;
		for (int i = 0; i < nitems; i++)
			rect_alfa(arriba, 4 + i * w, 3, w - 2, 3, i <= item_sel ? C_BLANCO : C_GRIS, 220);
	}
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, "Historia");
	BandejaDS *b = band_sel < nbandejas ? &bandejas[band_sel] : NULL;
	texto_centro(abajo, &letra_titulo, ANCHO / 2, 50, b ? b->usuario : "", C_BLANCO);
	char s[32];
	snprintf(s, sizeof s, "%d de %d", nitems ? item_sel + 1 : 0, nitems);
	texto_centro(abajo, &letra_normal, ANCHO / 2, 80, s, C_SUAVE);
	boton(abajo, 8, 120, 116, 44, "◄ Antes", C_PANEL2, C_PANEL, false);
	boton(abajo, 132, 120, 116, 44, "Siguiente ►", ROSA, MORADO, false);
	ayuda(abajo, "Toca a los lados para pasar     B salir");
}

static void dibujar_mensajes(Color *arriba, Color *abajo)
{
	fondo(arriba, "Mensajes");
	if (nchats == 0) {
		texto_centro(arriba, &letra_normal, ANCHO / 2, 90, pedir_chats ? "Cargando los mensajes…" : "Aún no hay mensajes", C_GRIS);
	} else {
		const ChatIG *c = &chats[chat_sel];
		texto_corto(arriba, &letra_titulo, 12, 30, 190, c->nombre, C_BLANCO);
		texto_derecha(arriba, &letra_peq, 244, 34, c->cuando, C_GRIS);
		texto_parrafo(arriba, &letra_normal, 12, 56, 232, c->previa, C_SUAVE);
	}
	dibujar_aviso(arriba, 156);
	ayuda(arriba, "A abrir     Y actualizar     B salir");
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, usuario_ig[0] ? usuario_ig : "Instagram");
	pestanas(abajo);
	filas(abajo, nchats, chat_sel, chat_primero, false);
}

static void dibujar_chat(Color *arriba, Color *abajo)
{
	fondo(arriba, abierto_nombre);
	conversacion(arriba, msgs, nmsgs, &desplazar, 20, 174, MORADO, "Cargando mensajes…");
	barra_arriba(arriba, abierto_nombre);
	dibujar_aviso(arriba, 154);
	ayuda(arriba, "▲▼ moverse     A escribir     B volver");
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, abierto_nombre);
	boton(abajo, 8, 34, 240, 38, pendiente[0] ? "Enviando…" : "Escribir un mensaje", ROSA, MORADO, true);
	for (int i = 0; i < 4; i++)
		boton(abajo, 8 + (i % 2) * 124, 80 + (i / 2) * 40, 116, 34, rapidas[i], C_PANEL2, C_PANEL, false);
	boton(abajo, 8, 162, 116, 26, "▲ Antes", C_PANEL, C_PANEL, false);
	boton(abajo, 132, 162, 116, 26, "▼ Después", C_PANEL, C_PANEL, false);
}

void ig_dibujar(Color *arriba, Color *abajo)
{
	if (vista == V_VIDEO) {
		video_dibujar(arriba, abajo);
		return;
	}
	if (vista == V_CODIGO) {
		codigo_dibujar(&tec, arriba, abajo, aviso);
		return;
	}
	if (vista == V_ESCRIBIR) {
		fondo(arriba, abierto_nombre);
		conversacion(arriba, msgs, nmsgs, &desplazar, 20, 134, MORADO, "Cargando mensajes…");
		barra_arriba(arriba, abierto_nombre);
		teclado_dibujar_texto(&tec, arriba, 8, 142, 240, false);
		ayuda(arriba, "START enviar     B borrar     X mayúsculas");
		teclado_dibujar(&tec, abajo, "Mensaje");
		return;
	}
	if (vista == V_CHAT) {
		dibujar_chat(arriba, abajo);
		return;
	}
	if (vista == V_HISTORIA) {
		dibujar_historia(arriba, abajo);
		return;
	}
	if (plat_wifi_estado() != RED_CONECTADA || puente_estado() != PU_LISTO || !lista()) {
		dibujar_sin(arriba, abajo);
		return;
	}
	if (tab == T_INICIO)
		dibujar_inicio(arriba, abajo);
	else if (tab == T_HISTORIAS)
		dibujar_historias(arriba, abajo);
	else
		dibujar_mensajes(arriba, abajo);
}
