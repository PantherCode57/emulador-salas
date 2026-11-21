// WhatsApp en la DS: la lista de chats, la conversación y escribir, con lo que manda el puente
// (puente.c). La sesión es la de la dueña de la DS, en el puente: se vincula desde aquí con su
// número (el puente le da un código de 8 letras para meterlo en el WhatsApp del móvil).
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sistema.h"

enum { V_LISTA, V_CHAT, V_ESCRIBIR, V_CODIGO, V_TELEFONO, V_VINCULO };

typedef struct {
	char jid[84], nombre[64], hora[10], previa[100];
	int noleidos;
} ChatDS;

typedef struct {
	bool mio;
	char hora[10], de[44], texto[408];
} MsgDS;

#define MAX_CHATS 40
#define MAX_MSGS 40
#define FILA_Y 28
#define FILA_H 32
#define FILAS 5

static ChatDS chats[MAX_CHATS];
static int nchats, sel, primero, primero_al_tocar;
static MsgDS msgs[MAX_MSGS];
static int nmsgs;
static char abierto_jid[84], abierto_nombre[64];
static int vista = V_LISTA;
static int ver = -1;
static char estado_wa[20];
static bool cargado, pedir_chats = true, pedir_chat;
static uint32_t ultimo_estado;
static char pendiente[420], telefono[24];
static char codigo_vinculo[24];
static char aviso[120];
static uint32_t aviso_hasta;
static Teclado tec;
static int desplazar;

static const char *const rapidas[4] = {"Vale", "Ahora no puedo", "Jajaja", "Te llamo luego"};

static void poner_aviso(const char *s)
{
	snprintf(aviso, sizeof aviso, "%s", s);
	aviso_hasta = ahora_ms + 4000;
}

int wa_n_chats(void) { return nchats; }

const char *wa_ultimo_texto(void)
{
	return nmsgs > 0 ? msgs[nmsgs - 1].texto : "";
}

void wa_entrar(void)
{
	vista = V_LISTA;
	pedir_chats = true;
	aviso[0] = 0;
}

static bool vinculado(void)
{
	return strcmp(estado_wa, "conectado") == 0 || strcmp(estado_wa, "conectando") == 0 || strcmp(estado_wa, "sin-red") == 0;
}

static void copiar(char *d, int n, const char *s)
{
	snprintf(d, n, "%s", s ? s : "");
}

// procesar: lo que contesta el puente.
static void procesar(char **l, int n)
{
	if (n <= 0)
		return;
	if (!strncmp(l[0], "CHATS ", 6)) {
		ver = atoi(l[0] + 6);
		nchats = 0;
		for (int i = 1; i < n && nchats < MAX_CHATS; i++) {
			char *t[6];
			if (partir(l[i], t, 6) < 6 || strcmp(t[0], "C"))
				continue;
			ChatDS *c = &chats[nchats++];
			copiar(c->jid, sizeof c->jid, t[1]);
			copiar(c->nombre, sizeof c->nombre, t[2]);
			c->noleidos = atoi(t[3]);
			copiar(c->hora, sizeof c->hora, t[4]);
			copiar(c->previa, sizeof c->previa, t[5]);
		}
		if (sel >= nchats)
			sel = nchats > 0 ? nchats - 1 : 0;
		pedir_chats = false;
		cargado = true;
	} else if (!strncmp(l[0], "CHAT ", 5)) {
		char *t[2];
		partir(l[0] + 5, t, 2);
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
	} else if (!strncmp(l[0], "ESTADO ", 7)) {
		char *e = l[0] + 7, *esp = strchr(e, ' ');
		int v = esp ? atoi(esp + 1) : 0;
		if (esp)
			*esp = 0;
		bool antes = vinculado();
		copiar(estado_wa, sizeof estado_wa, e);
		if (v != ver || (!antes && vinculado())) {
			pedir_chats = true;
			if (vista == V_CHAT || vista == V_ESCRIBIR)
				pedir_chat = true;
		}
		if (vista == V_VINCULO && vinculado()) {
			poner_aviso("¡WhatsApp vinculado!");
			vista = V_LISTA;
		}
	} else if (!strncmp(l[0], "OK ", 3)) {
		pedir_chat = pedir_chats = true;
		poner_aviso("Enviado");
	} else if (!strncmp(l[0], "CODIGO ", 7)) {
		copiar(codigo_vinculo, sizeof codigo_vinculo, l[0] + 7);
	} else if (!strncmp(l[0], "ERROR ", 6)) {
		poner_aviso(l[0] + 6);
		if (vista == V_VINCULO && !codigo_vinculo[0])
			vista = V_TELEFONO;
	}
}

// red: pedir al puente lo que haga falta, de uno en uno.
static bool red(void)
{
	char **l;
	int n = puente_respuesta(&l);
	bool cambio = false;
	if (n >= 0) {
		procesar(l, n);
		cambio = true;
	}
	if (!puente_libre())
		return cambio;
	char p[480];
	if (pendiente[0]) {
		snprintf(p, sizeof p, "WA ENVIAR %s\t%s", abierto_jid, pendiente);
		if (puente_pedir(p))
			pendiente[0] = 0;
	} else if (telefono[0]) {
		snprintf(p, sizeof p, "WA VINCULAR %s", telefono);
		if (puente_pedir(p))
			telefono[0] = 0;
	} else if (pedir_chats && vinculado()) {
		puente_pedir("WA CHATS");
	} else if (pedir_chat && vinculado() && (vista == V_CHAT || vista == V_ESCRIBIR)) {
		snprintf(p, sizeof p, "WA CHAT %s", abierto_jid);
		puente_pedir(p);
	} else if (ahora_ms - ultimo_estado > 2500 || !estado_wa[0]) {
		ultimo_estado = ahora_ms;
		puente_pedir("WA ESTADO");
	}
	return cambio;
}

static void abrir_chat(int i)
{
	if (i < 0 || i >= nchats)
		return;
	sel = i;
	copiar(abierto_jid, sizeof abierto_jid, chats[i].jid);
	copiar(abierto_nombre, sizeof abierto_nombre, chats[i].nombre);
	nmsgs = 0;
	desplazar = 0;
	pedir_chat = true;
	vista = V_CHAT;
}

static void ver_elegido(void)
{
	if (sel < primero)
		primero = sel;
	if (sel >= primero + FILAS)
		primero = sel - FILAS + 1;
}

// Las vistas con el teclado.
static int paso_teclado(const Entrada *e)
{
	if (e->toque_ini && en(e->tx, e->ty, 0, 0, 64, 28)) { // «Atrás»
		vista = (vista == V_ESCRIBIR) ? V_CHAT : V_LISTA;
		return CAMBIO;
	}
	int r = teclado_paso(&tec, e);
	if (r == TECLADO_CANCELAR) {
		vista = (vista == V_ESCRIBIR) ? V_CHAT : V_LISTA;
		return CAMBIO;
	}
	if (r != TECLADO_ACEPTAR)
		return r == TECLADO_CAMBIO ? CAMBIO : NADA;
	switch (vista) {
	case V_ESCRIBIR:
		if (tec.texto[0]) {
			copiar(pendiente, sizeof pendiente, tec.texto);
			desplazar = 0;
		}
		vista = V_CHAT;
		break;
	case V_CODIGO: {
		int n = (int)strlen(tec.texto);
		bool cifras = n == 6;
		for (int i = 0; i < n; i++)
			cifras = cifras && tec.texto[i] >= '0' && tec.texto[i] <= '9';
		if (!cifras) {
			poner_aviso("El código son 6 cifras");
			return CAMBIO;
		}
		puente_poner_codigo(tec.texto);
		vista = V_LISTA;
		break;
	}
	case V_TELEFONO: {
		char d[24];
		int k = 0;
		for (const char *s = tec.texto; *s && k < (int)sizeof d - 1; s++)
			if (*s >= '0' && *s <= '9')
				d[k++] = *s;
		d[k] = 0;
		if (k < 8 || d[0] == '0') {
			poner_aviso("Con el prefijo del país delante (34 en España)");
			return CAMBIO;
		}
		copiar(telefono, sizeof telefono, d);
		codigo_vinculo[0] = 0;
		vista = V_VINCULO;
		break;
	}
	}
	return CAMBIO;
}

int wa_paso(const Entrada *e)
{
	int r = red() ? CAMBIO : NADA;
	if (aviso[0] && ahora_ms > aviso_hasta) {
		aviso[0] = 0;
		r = CAMBIO;
	}
	if ((imagenes % 15) == 0)
		r = CAMBIO; // lo que se mueve (esperas, el cursor)

	if (vista == V_ESCRIBIR || vista == V_CODIGO || vista == V_TELEFONO) {
		int t = paso_teclado(e);
		return t != NADA ? t : r;
	}
	int pu = puente_estado();

	if (vista == V_VINCULO) {
		if (toca_atras(e)) {
			vista = V_LISTA;
			return CAMBIO;
		}
		return r;
	}

	if (vista == V_CHAT) {
		if (toca_atras(e)) {
			vista = V_LISTA;
			pedir_chats = true;
			return CAMBIO;
		}
		if ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 34, 240, 38))) {
			teclado_iniciar(&tec, "");
			vista = V_ESCRIBIR;
			return CAMBIO;
		}
		for (int i = 0; i < 4; i++) {
			if (e->toque_ini && en(e->tx, e->ty, 8 + (i % 2) * 124, 80 + (i / 2) * 40, 116, 34)) {
				copiar(pendiente, sizeof pendiente, rapidas[i]);
				desplazar = 0;
				return CAMBIO;
			}
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

	// V_LISTA
	if (toca_atras(e))
		return SALIR;
	if (plat_wifi_estado() != RED_CONECTADA) {
		if ((e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 8, 36, 240, 46))) {
			wifi_entrar();
			return IR_WIFI;
		}
		return r;
	}
	if (pu == PU_SIN_CODIGO || pu == PU_CODIGO_MAL) {
		if ((e->pulsado & B_A) || (e->toque && en(e->toque_x, e->toque_y, 8, 150, 240, 34))) {
			teclado_iniciar(&tec, "");
			vista = V_CODIGO;
			return CAMBIO;
		}
		return r;
	}
	if (pu != PU_LISTO)
		return r;
	if (estado_wa[0] && !vinculado()) {
		if ((e->pulsado & B_A) || (e->toque && en(e->toque_x, e->toque_y, 8, 150, 240, 34))) {
			teclado_iniciar(&tec, "34");
			vista = V_TELEFONO;
			return CAMBIO;
		}
		return r;
	}
	if (e->pulsado & B_Y) {
		pedir_chats = true;
		poner_aviso("Actualizando…");
		return CAMBIO;
	}
	if (e->repetido & (B_ARRIBA | B_ABAJO)) {
		if (nchats > 0) {
			sel += (e->repetido & B_ABAJO) ? 1 : -1;
			if (sel < 0)
				sel = 0;
			if (sel >= nchats)
				sel = nchats - 1;
			ver_elegido();
		}
		return CAMBIO;
	}
	if (e->pulsado & B_A) {
		abrir_chat(sel);
		return CAMBIO;
	}
	if (e->toque_ini)
		primero_al_tocar = primero;
	if (e->arrastrando && nchats > FILAS) {
		int p = primero_al_tocar - e->arrastre_dy / FILA_H;
		if (p > nchats - FILAS)
			p = nchats - FILAS;
		if (p < 0)
			p = 0;
		if (p != primero) {
			primero = p;
			return CAMBIO;
		}
	}
	if (e->toque && en(e->toque_x, e->toque_y, 0, FILA_Y, ANCHO, FILA_H * FILAS)) {
		abrir_chat(primero + (e->toque_y - FILA_Y) / FILA_H);
		return CAMBIO;
	}
	return r;
}

// --- Dibujo ---

#define VERDE_WA RGB(0, 168, 132)
#define BURBUJA_MIA RGB(0, 92, 75)

static void fondo_arriba(Color *p, const char *titulo)
{
	llenar(p, C_FONDO);
	degradado(p, 0, 0, ANCHO, 80, mezclar(C_FONDO, apps[0].c2, 120), C_FONDO);
	barra_arriba(p, titulo);
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

// alto_burbuja: lo que ocupa un mensaje en la conversación.
static int alto_burbuja(const MsgDS *m, int *ancho)
{
	int max = 176, w = ancho_texto(&letra_normal, m->texto);
	int alto_texto = w > max ? texto_parrafo(NULL, &letra_normal, 0, 0, max, m->texto, 0) : letra_normal.alto + 2;
	if (w > max)
		w = max;
	int wh = ancho_texto(&letra_peq, m->hora);
	if (w < wh)
		w = wh;
	if (!m->mio && m->de[0]) {
		int wd = ancho_texto(&letra_peq, m->de);
		if (w < wd)
			w = wd > max ? max : wd;
	}
	*ancho = w + 14;
	return 5 + ((!m->mio && m->de[0]) ? 13 : 0) + alto_texto + letra_peq.alto + 4;
}

// conversacion: los mensajes, de abajo arriba, entre y0 e y1.
static void conversacion(Color *p, int y0, int y1)
{
	int total = 0, alto[MAX_MSGS], ancho[MAX_MSGS];
	for (int i = 0; i < nmsgs; i++)
		total += (alto[i] = alto_burbuja(&msgs[i], &ancho[i])) + 4;
	int maximo = total - (y1 - y0);
	if (maximo < 0)
		maximo = 0;
	if (desplazar > maximo)
		desplazar = maximo;
	if (nmsgs == 0) {
		texto_centro(p, &letra_normal, ANCHO / 2, (y0 + y1) / 2 - 8,
			cargado || vinculado() ? "Cargando mensajes…" : "Sin mensajes", C_GRIS);
		return;
	}
	int yb = y1 + desplazar;
	for (int i = nmsgs - 1; i >= 0 && yb > y0; i--) {
		const MsgDS *m = &msgs[i];
		int y = yb - alto[i], w = ancho[i];
		int x = m->mio ? ANCHO - 6 - w : 6;
		if (y < y1) {
			redondeado(p, x, y, w, alto[i], 8, m->mio ? BURBUJA_MIA : C_PANEL2, 256);
			int ty = y + 4;
			if (!m->mio && m->de[0]) {
				texto_corto(p, &letra_peq, x + 7, ty, w - 14, m->de, C_AZUL);
				ty += 13;
			}
			ty = texto_parrafo(p, &letra_normal, x + 7, ty, 176, m->texto, C_BLANCO);
			texto_derecha(p, &letra_peq, x + w - 7, ty - 1, m->hora, m->mio ? RGB(160, 220, 200) : C_GRIS);
		}
		yb = y - 4;
	}
}

static void dibujar_estado(Color *abajo)
{
	char d[80];
	fila_estado(abajo, 36, estado_wifi(d, sizeof d), "Wifi", d);
	fila_estado(abajo, 88, estado_puente(d, sizeof d), "Puente", d);
	int pu = puente_estado();
	if (pu == PU_SIN_CODIGO || pu == PU_CODIGO_MAL) {
		boton(abajo, 8, 150, 240, 34, "Escribir el código del puente", VERDE_WA, apps[0].c2, true);
	} else if (pu == PU_LISTO && estado_wa[0] && !vinculado()) {
		boton(abajo, 8, 150, 240, 34, "Vincular WhatsApp", VERDE_WA, apps[0].c2, true);
	} else {
		fila_estado(abajo, 140, pu == PU_LISTO ? 1 : 2, "WhatsApp",
			pu == PU_LISTO ? "Preguntando al puente…" : "Cuando esté el puente");
	}
}

static void dibujar_lista(Color *arriba, Color *abajo)
{
	int pu = puente_estado();
	bool listo = pu == PU_LISTO && vinculado();

	fondo_arriba(arriba, "WhatsApp");
	if (!listo) {
		const char *t = "Hace falta el puente", *s = "El puente es un programa en un aparato tuyo (en el mismo wifi) "
			"que lleva tu WhatsApp. La DS solo enseña lo que le pasa.";
		if (plat_wifi_estado() != RED_CONECTADA) {
			t = "Sin wifi";
			s = "Conecta la DS a un wifi sin contraseña (por ejemplo, la zona wifi del móvil). Pulsa A.";
		} else if (pu == PU_SIN_CODIGO || pu == PU_CODIGO_MAL) {
			t = pu == PU_CODIGO_MAL ? "Ese código no es" : "El código del puente";
			s = "El puente enseña un código de 6 cifras al ponerlo en marcha. Se escribe aquí una sola vez y "
				"así nadie más de este wifi puede ver tus chats.";
		} else if (pu == PU_LISTO && estado_wa[0] && !vinculado()) {
			t = "Vincular WhatsApp";
			s = "Escribe tu número y el puente te dará un código para meterlo en el WhatsApp de tu móvil, "
				"en Dispositivos vinculados. Solo la primera vez.";
		}
		texto(arriba, &letra_titulo, 12, 30, t, C_BLANCO);
		texto_parrafo(arriba, &letra_normal, 12, 56, 232, s, C_SUAVE);
		dibujar_aviso(arriba, 152);
		ayuda(arriba, "A seguir     B volver");
		llenar(abajo, C_FONDO);
		barra_abajo(abajo, "WhatsApp");
		dibujar_estado(abajo);
		return;
	}

	if (nchats == 0) {
		texto_centro(arriba, &letra_normal, ANCHO / 2, 90, cargado ? "Aún no hay chats" : "Cargando chats…", C_GRIS);
	} else {
		const ChatDS *c = &chats[sel];
		texto_corto(arriba, &letra_titulo, 12, 30, 190, c->nombre, C_BLANCO);
		texto_derecha(arriba, &letra_peq, 244, 34, c->hora, C_GRIS);
		int y = texto_parrafo(arriba, &letra_normal, 12, 56, 232, c->previa, C_SUAVE);
		if (c->noleidos > 0) {
			char s[32];
			snprintf(s, sizeof s, "%d sin leer", c->noleidos);
			texto(arriba, &letra_negrita, 12, y + 4, s, C_VERDE);
		}
	}
	dibujar_aviso(arriba, 152);
	ayuda(arriba, "A abrir     Y actualizar     B salir");

	llenar(abajo, C_FONDO);
	barra_abajo(abajo, "WhatsApp");
	for (int k = 0; k < FILAS && primero + k < nchats; k++) {
		int i = primero + k, y = FILA_Y + k * FILA_H;
		const ChatDS *c = &chats[i];
		redondeado(abajo, 4, y + 1, 244, FILA_H - 3, 8, i == sel ? C_PANEL2 : C_PANEL, 256);
		if (i == sel)
			marco(abajo, 4, y + 1, 244, FILA_H - 3, 8, 1, VERDE_WA);
		int wh = ancho_texto(&letra_peq, c->hora);
		texto_corto(abajo, &letra_negrita, 10, y + 2, 226 - wh - 10, c->nombre, C_BLANCO);
		texto_derecha(abajo, &letra_peq, 242, y + 4, c->hora, c->noleidos ? C_VERDE : C_GRIS);
		texto_corto(abajo, &letra_peq, 10, y + 16, c->noleidos ? 200 : 228, c->previa, C_GRIS);
		if (c->noleidos > 0) {
			char s[8];
			snprintf(s, sizeof s, "%d", c->noleidos > 99 ? 99 : c->noleidos);
			circulo(abajo, 234, y + 22, 7, C_VERDE, 256);
			texto_centro(abajo, &letra_peq, 234, y + 16, s, C_NEGRO);
		}
	}
	if (nchats > FILAS) {
		int alto = FILA_H * FILAS, h = alto * FILAS / nchats;
		int y = FILA_Y + (alto - h) * primero / (nchats - FILAS);
		redondeado(abajo, 250, y, 4, h, 2, C_OSCURO, 256);
	}
}

static void dibujar_chat(Color *arriba, Color *abajo)
{
	fondo_arriba(arriba, abierto_nombre);
	conversacion(arriba, 20, 174);
	barra_arriba(arriba, abierto_nombre); // encima de lo que se salga por arriba
	dibujar_aviso(arriba, 154);
	ayuda(arriba, "▲▼ moverse     A escribir     B volver");

	llenar(abajo, C_FONDO);
	barra_abajo(abajo, abierto_nombre);
	boton(abajo, 8, 34, 240, 38, pendiente[0] ? "Enviando…" : "Escribir un mensaje", VERDE_WA, apps[0].c2, true);
	for (int i = 0; i < 4; i++)
		boton(abajo, 8 + (i % 2) * 124, 80 + (i / 2) * 40, 116, 34, rapidas[i], C_PANEL2, C_PANEL, false);
	boton(abajo, 8, 162, 116, 26, "▲ Antes", C_PANEL, C_PANEL, false);
	boton(abajo, 132, 162, 116, 26, "▼ Después", C_PANEL, C_PANEL, false);
}

static void dibujar_teclado(Color *arriba, Color *abajo)
{
	if (vista == V_ESCRIBIR) {
		fondo_arriba(arriba, abierto_nombre);
		conversacion(arriba, 20, 134);
		barra_arriba(arriba, abierto_nombre);
		teclado_dibujar_texto(&tec, arriba, 8, 142, 240, false);
		ayuda(arriba, "START enviar     B borrar     X mayúsculas");
		teclado_dibujar(&tec, abajo, "Mensaje");
		return;
	}
	bool codigo = vista == V_CODIGO;
	fondo_arriba(arriba, "WhatsApp");
	texto(arriba, &letra_titulo, 12, 30, codigo ? "Código del puente" : "Tu número de teléfono", C_BLANCO);
	texto_parrafo(arriba, &letra_peq, 12, 52, 232, codigo
		? "Son las 6 cifras que enseña el puente al ponerlo en marcha. Se apunta en la tarjeta: solo hace falta una vez."
		: "Con el prefijo del país delante y sin espacios (34 en España). Por ejemplo: 34612345678.", C_SUAVE);
	teclado_dibujar_texto(&tec, arriba, 8, 112, 240, false);
	dibujar_aviso(arriba, 146);
	ayuda(arriba, "START aceptar     B borrar");
	teclado_dibujar(&tec, abajo, codigo ? "Código del puente" : "Vincular WhatsApp");
}

static void dibujar_vinculo(Color *arriba, Color *abajo)
{
	fondo_arriba(arriba, "WhatsApp");
	if (!codigo_vinculo[0]) {
		texto(arriba, &letra_titulo, 12, 30, "Pidiendo el código…", C_BLANCO);
		texto_parrafo(arriba, &letra_normal, 12, 56, 232, "El puente se lo pide a WhatsApp. Tarda unos segundos.", C_SUAVE);
	} else {
		texto(arriba, &letra_normal, 12, 28, "Mete este código en tu WhatsApp:", C_SUAVE);
		redondeado(arriba, 28, 48, 200, 40, 10, C_PANEL2, 256);
		texto_centro(arriba, &letra_grande, ANCHO / 2, 54, codigo_vinculo, C_VERDE);
		texto_parrafo(arriba, &letra_peq, 12, 96, 232,
			"En el móvil: WhatsApp → Ajustes → Dispositivos vinculados → Vincular un dispositivo → "
			"«Vincular con el número de teléfono». Caduca en unos 2 minutos.", C_SUAVE);
	}
	dibujar_aviso(arriba, 156);
	ayuda(arriba, "B volver");
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, "Vincular WhatsApp");
	fila_estado(abajo, 60, 1, "WhatsApp", codigo_vinculo[0] ? "Esperando a que metas el código…" : "Pidiendo el código…");
}

void wa_dibujar(Color *arriba, Color *abajo)
{
	switch (vista) {
	case V_CHAT:
		dibujar_chat(arriba, abajo);
		break;
	case V_ESCRIBIR:
	case V_CODIGO:
	case V_TELEFONO:
		dibujar_teclado(arriba, abajo);
		break;
	case V_VINCULO:
		dibujar_vinculo(arriba, abajo);
		break;
	default:
		dibujar_lista(arriba, abajo);
	}
}
