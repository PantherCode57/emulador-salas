// Piezas que salen en todas las pantallas: las barras de arriba, el botón de volver, la batería…
#include <ctype.h>
#include <stdio.h>
#include <string.h>

#include "sistema.h"

const char *const dias_semana[7] = {"domingo", "lunes", "martes", "miércoles", "jueves", "viernes", "sábado"};
const char *const meses[12] = {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
	"agosto", "septiembre", "octubre", "noviembre", "diciembre"};

void fecha_larga(const struct tm *t, char *s, int n)
{
	snprintf(s, n, "%s, %d de %s", dias_semana[t->tm_wday % 7], t->tm_mday, meses[t->tm_mon % 12]);
	s[0] = (char)toupper((unsigned char)s[0]);
}

bool en(int x, int y, int x0, int y0, int w, int h)
{
	return x >= x0 && y >= y0 && x < x0 + w && y < y0 + h;
}

void bateria(Color *p, int x, int y)
{
	int v = plat_bateria();
	int nivel = v & 15;
	marco(p, x, y, 18, 10, 2, 1, C_SUAVE);
	rect(p, x + 18, y + 3, 2, 4, C_SUAVE);
	int w = (nivel * 14 + 14) / 15;
	rect(p, x + 2, y + 2, w, 6, nivel > 5 ? C_VERDE : C_ROJO);
	if (v & 128) { // cargando: un rayo
		triangulo(p, x + 10, y + 1, x + 6, y + 6, x + 9, y + 6, C_AMARILLO);
		triangulo(p, x + 8, y + 4, x + 12, y + 4, x + 8, y + 9, C_AMARILLO);
	}
}

void icono_wifi(Color *p, int x, int y, Color c)
{
	int estado = plat_wifi_estado();
	int encendidas = 0;
	if (estado == RED_CONECTADA)
		encendidas = 4;
	else if (estado == RED_CONECTANDO || estado == RED_BUSCANDO)
		encendidas = 1 + (int)((imagenes / 15) % 4);
	for (int i = 0; i < 4; i++) {
		int h = 3 + i * 2;
		rect(p, x + i * 4, y + 10 - h, 3, h, i < encendidas ? c : C_OSCURO);
	}
	if (estado == RED_FALLO || estado == RED_APAGADA) {
		linea(p, x + 9, y + 1, x + 14, y + 6, 1.5f, C_ROJO);
		linea(p, x + 14, y + 1, x + 9, y + 6, 1.5f, C_ROJO);
	}
}

void barra_arriba(Color *p, const char *titulo)
{
	rect_alfa(p, 0, 0, ANCHO, 18, C_NEGRO, 190);
	texto(p, &letra_negrita, 6, 2, titulo, C_BLANCO);
	char h[8];
	snprintf(h, sizeof h, "%02d:%02d", hora.tm_hour, hora.tm_min);
	bateria(p, 232, 4);
	texto_derecha(p, &letra_negrita, 226, 2, h, C_SUAVE);
	icono_wifi(p, 226 - ancho_texto(&letra_negrita, h) - 22, 3, C_BLANCO);
}

void barra_abajo(Color *p, const char *titulo)
{
	rect(p, 0, 0, ANCHO, 26, C_PANEL);
	rect(p, 0, 26, ANCHO, 1, C_PANEL2);
	redondeado(p, 4, 3, 54, 20, 8, C_PANEL2, 256);
	linea(p, 16, 8, 11, 13, 2, C_BLANCO);
	linea(p, 11, 13, 16, 18, 2, C_BLANCO);
	texto(p, &letra_peq, 21, 6, "Atrás", C_BLANCO);
	texto_centro(p, &letra_negrita, 140, 5, titulo, C_BLANCO);
}

bool toca_atras(const Entrada *e)
{
	return (e->pulsado & B_B) || (e->toque_ini && en(e->tx, e->ty, 0, 0, 64, 28));
}

void ayuda(Color *p, const char *s)
{
	rect_alfa(p, 0, ALTO - 16, ANCHO, 16, C_NEGRO, 200);
	texto_centro(p, &letra_peq, ANCHO / 2, ALTO - 15, s, C_GRIS);
}

void boton(Color *p, int x, int y, int w, int h, const char *s, Color c1, Color c2, bool elegido)
{
	redondeado_degradado(p, x, y, w, h, 8, c1, c2);
	if (elegido)
		marco(p, x - 2, y - 2, w + 4, h + 4, 10, 2, C_BLANCO);
	texto_centro(p, &letra_negrita, x + w / 2, y + (h - letra_negrita.alto) / 2, s, C_BLANCO);
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

// conversacion: los mensajes, de abajo arriba, entre y0 e y1 (desplazar: cuánto se ha subido).
void conversacion(Color *p, const MsgDS *msgs, int nmsgs, int *desplazar, int y0, int y1, Color mia, const char *vacio)
{
	int total = 0, alto[64], ancho[64];
	if (nmsgs > 64)
		nmsgs = 64;
	for (int i = 0; i < nmsgs; i++)
		total += (alto[i] = alto_burbuja(&msgs[i], &ancho[i])) + 4;
	int maximo = total - (y1 - y0);
	if (maximo < 0)
		maximo = 0;
	if (*desplazar > maximo)
		*desplazar = maximo;
	if (nmsgs == 0) {
		texto_centro(p, &letra_normal, ANCHO / 2, (y0 + y1) / 2 - 8, vacio, C_GRIS);
		return;
	}
	int yb = y1 + *desplazar;
	for (int i = nmsgs - 1; i >= 0 && yb > y0; i--) {
		const MsgDS *m = &msgs[i];
		int y = yb - alto[i], w = ancho[i];
		int x = m->mio ? ANCHO - 6 - w : 6;
		if (y < y1) {
			redondeado(p, x, y, w, alto[i], 8, m->mio ? mia : C_PANEL2, 256);
			int ty = y + 4;
			if (!m->mio && m->de[0]) {
				texto_corto(p, &letra_peq, x + 7, ty, w - 14, m->de, C_AZUL);
				ty += 13;
			}
			ty = texto_parrafo(p, &letra_normal, x + 7, ty, 176, m->texto, C_BLANCO);
			texto_derecha(p, &letra_peq, x + w - 7, ty - 1, m->hora, m->mio ? mezclar(mia, C_BLANCO, 150) : C_GRIS);
		}
		yb = y - 4;
	}
}
