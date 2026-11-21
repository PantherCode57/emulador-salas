// El teclado de la pantalla de abajo: se toca con el dedo o se mueve con la cruceta y A.
// B borra (y si no queda nada, sale), START acepta, X cambia entre mayúsculas y minúsculas.
#include <string.h>

#include "sistema.h"

enum { T_LETRA, T_MAYUS, T_BORRAR, T_CAMBIAR, T_ESPACIO, T_OK };

typedef struct {
	const char *pone; // lo que escribe (en minúscula)
	int tipo;
	int ancho; // en medias teclas: cada fila suma 20
} Tecla;

#define FILAS 5
#define MAX_TECLAS 10

static const Tecla letras[FILAS][MAX_TECLAS + 1] = {
	{{"1", 0, 2}, {"2", 0, 2}, {"3", 0, 2}, {"4", 0, 2}, {"5", 0, 2}, {"6", 0, 2}, {"7", 0, 2}, {"8", 0, 2}, {"9", 0, 2}, {"0", 0, 2}, {0}},
	{{"q", 0, 2}, {"w", 0, 2}, {"e", 0, 2}, {"r", 0, 2}, {"t", 0, 2}, {"y", 0, 2}, {"u", 0, 2}, {"i", 0, 2}, {"o", 0, 2}, {"p", 0, 2}, {0}},
	{{"a", 0, 2}, {"s", 0, 2}, {"d", 0, 2}, {"f", 0, 2}, {"g", 0, 2}, {"h", 0, 2}, {"j", 0, 2}, {"k", 0, 2}, {"l", 0, 2}, {"ñ", 0, 2}, {0}},
	{{"", T_MAYUS, 3}, {"z", 0, 2}, {"x", 0, 2}, {"c", 0, 2}, {"v", 0, 2}, {"b", 0, 2}, {"n", 0, 2}, {"m", 0, 2}, {"", T_BORRAR, 3}, {0}},
	{{"", T_CAMBIAR, 4}, {" ", T_ESPACIO, 10}, {"", T_OK, 6}, {0}},
};

static const Tecla simbolos[FILAS][MAX_TECLAS + 1] = {
	{{"1", 0, 2}, {"2", 0, 2}, {"3", 0, 2}, {"4", 0, 2}, {"5", 0, 2}, {"6", 0, 2}, {"7", 0, 2}, {"8", 0, 2}, {"9", 0, 2}, {"0", 0, 2}, {0}},
	{{"!", 0, 2}, {"?", 0, 2}, {"@", 0, 2}, {"#", 0, 2}, {"$", 0, 2}, {"%", 0, 2}, {"&", 0, 2}, {"*", 0, 2}, {"(", 0, 2}, {")", 0, 2}, {0}},
	{{"-", 0, 2}, {"_", 0, 2}, {"=", 0, 2}, {"+", 0, 2}, {"/", 0, 2}, {":", 0, 2}, {";", 0, 2}, {"'", 0, 2}, {"\"", 0, 2}, {",", 0, 2}, {0}},
	{{".", 0, 2}, {"¿", 0, 2}, {"¡", 0, 2}, {"<", 0, 2}, {">", 0, 2}, {"[", 0, 2}, {"]", 0, 2}, {"€", 0, 2}, {"", T_BORRAR, 4}, {0}},
	{{"", T_CAMBIAR, 4}, {" ", T_ESPACIO, 10}, {"", T_OK, 6}, {0}},
};

static const Tecla (*tabla(const Teclado *t))[MAX_TECLAS + 1]
{
	return t->simbolos ? simbolos : letras;
}

static int n_teclas(const Teclado *t, int fila)
{
	int n = 0;
	while (n < MAX_TECLAS && tabla(t)[fila][n].ancho)
		n++;
	return n;
}

// Dónde está cada tecla en la pantalla de abajo.
static void caja(const Teclado *t, int fila, int col, int *x, int *y, int *w, int *h)
{
	int u = 0;
	for (int i = 0; i < col; i++)
		u += tabla(t)[fila][i].ancho;
	int a = tabla(t)[fila][col].ancho;
	*x = 3 + u * 25 / 2;
	*w = (u + a) * 25 / 2 - u * 25 / 2 - 2;
	*y = 31 + fila * 32;
	*h = 29;
}

static void mayuscula(const char *s, char *out)
{
	if (s[0] >= 'a' && s[0] <= 'z' && !s[1]) {
		out[0] = (char)(s[0] - 'a' + 'A');
		out[1] = 0;
	} else if (!strcmp(s, "ñ")) {
		strcpy(out, "Ñ");
	} else {
		strcpy(out, s);
	}
}

void teclado_iniciar(Teclado *t, const char *inicial)
{
	memset(t, 0, sizeof *t);
	strncpy(t->texto, inicial ? inicial : "", sizeof t->texto - 1);
	t->fila = 1;
}

static void borrar_letra(char *s)
{
	int n = (int)strlen(s);
	while (n > 0) {
		n--;
		if (((unsigned char)s[n] & 0xC0) != 0x80)
			break; // el principio de la letra (las de UTF-8 de más de un byte)
	}
	s[n] = 0;
}

static int pulsar(Teclado *t, int fila, int col)
{
	const Tecla *k = &tabla(t)[fila][col];
	switch (k->tipo) {
	case T_MAYUS:
		t->mayus = !t->mayus;
		return TECLADO_CAMBIO;
	case T_BORRAR:
		borrar_letra(t->texto);
		return TECLADO_CAMBIO;
	case T_CAMBIAR:
		t->simbolos = !t->simbolos;
		if (t->col >= n_teclas(t, t->fila))
			t->col = n_teclas(t, t->fila) - 1;
		return TECLADO_CAMBIO;
	case T_OK:
		return TECLADO_ACEPTAR;
	default: {
		char letra[8];
		if (t->mayus)
			mayuscula(k->pone, letra);
		else
			strcpy(letra, k->pone);
		if (strlen(t->texto) + strlen(letra) < sizeof t->texto)
			strcat(t->texto, letra);
		return TECLADO_CAMBIO;
	}
	}
}

int teclado_paso(Teclado *t, const Entrada *e)
{
	if (e->toque_ini) {
		for (int f = 0; f < FILAS; f++) {
			for (int c = 0; c < n_teclas(t, f); c++) {
				int x, y, w, h;
				caja(t, f, c, &x, &y, &w, &h);
				if (en(e->tx, e->ty, x - 1, y - 2, w + 2, h + 3)) {
					t->fila = f;
					t->col = c;
					return pulsar(t, f, c);
				}
			}
		}
	}
	if (e->pulsado & B_START)
		return TECLADO_ACEPTAR;
	if (e->pulsado & B_X) {
		t->mayus = !t->mayus;
		return TECLADO_CAMBIO;
	}
	if (e->repetido & B_B) {
		if (!t->texto[0])
			return (e->pulsado & B_B) ? TECLADO_CANCELAR : TECLADO_NADA;
		borrar_letra(t->texto);
		return TECLADO_CAMBIO;
	}
	if (e->pulsado & B_A)
		return pulsar(t, t->fila, t->col);
	if (e->repetido & (B_IZQ | B_DER)) {
		int n = n_teclas(t, t->fila);
		t->col = (t->col + ((e->repetido & B_DER) ? 1 : n - 1)) % n;
		return TECLADO_CAMBIO;
	}
	if (e->repetido & (B_ARRIBA | B_ABAJO)) {
		int x, y, w, h;
		caja(t, t->fila, t->col, &x, &y, &w, &h);
		int centro = x + w / 2;
		t->fila = (t->fila + ((e->repetido & B_ABAJO) ? 1 : FILAS - 1)) % FILAS;
		int mejor = 0, distancia = 9999;
		for (int c = 0; c < n_teclas(t, t->fila); c++) {
			caja(t, t->fila, c, &x, &y, &w, &h);
			int d = x + w / 2 - centro;
			if (d < 0)
				d = -d;
			if (d < distancia) {
				distancia = d;
				mejor = c;
			}
		}
		t->col = mejor;
		return TECLADO_CAMBIO;
	}
	return TECLADO_NADA;
}

void teclado_dibujar(const Teclado *t, Color *p, const char *titulo)
{
	llenar(p, C_FONDO);
	barra_abajo(p, titulo);
	for (int f = 0; f < FILAS; f++) {
		for (int c = 0; c < n_teclas(t, f); c++) {
			const Tecla *k = &tabla(t)[f][c];
			int x, y, w, h;
			caja(t, f, c, &x, &y, &w, &h);
			bool elegida = f == t->fila && c == t->col;
			Color fondo = k->tipo == T_OK ? RGB(110, 80, 220) : (k->tipo == T_LETRA || k->tipo == T_ESPACIO) ? C_PANEL2 : C_PANEL;
			if (k->tipo == T_MAYUS && t->mayus)
				fondo = RGB(110, 80, 220);
			redondeado(p, x, y, w, h, 6, fondo, 256);
			if (elegida)
				marco(p, x - 1, y - 1, w + 2, h + 2, 7, 2, C_VIOLETA);
			int cx = x + w / 2, cy = y + h / 2;
			switch (k->tipo) {
			case T_MAYUS: // una flecha hacia arriba
				triangulo(p, cx, cy - 7, cx - 6, cy, cx + 6, cy, C_BLANCO);
				rect(p, cx - 3, cy, 6, 6, C_BLANCO);
				break;
			case T_BORRAR: // una flecha hacia la izquierda con una X
				triangulo(p, cx - 11, cy, cx - 5, cy - 6, cx - 5, cy + 6, C_BLANCO);
				rect(p, cx - 5, cy - 6, 15, 12, C_BLANCO);
				linea(p, cx - 1, cy - 3, cx + 5, cy + 3, 1.6f, fondo);
				linea(p, cx + 5, cy - 3, cx - 1, cy + 3, 1.6f, fondo);
				break;
			case T_CAMBIAR:
				texto_centro(p, &letra_negrita, cx, cy - 7, t->simbolos ? "abc" : "#+=", C_BLANCO);
				break;
			case T_ESPACIO:
				texto_centro(p, &letra_peq, cx, cy - 6, "espacio", C_GRIS);
				break;
			case T_OK:
				texto_centro(p, &letra_negrita, cx, cy - 7, "Aceptar", C_BLANCO);
				break;
			default: {
				char letra[8];
				if (t->mayus)
					mayuscula(k->pone, letra);
				else
					strcpy(letra, k->pone);
				texto_centro(p, &letra_titulo, cx, cy - 9, letra, C_BLANCO);
			}
			}
		}
	}
}

void teclado_dibujar_texto(const Teclado *t, Color *p, int x, int y, int w, bool tapar)
{
	redondeado(p, x, y, w, 26, 8, C_PANEL2, 256);
	marco(p, x, y, w, 26, 8, 1, C_VIOLETA);
	char visto[72];
	if (tapar) {
		int n = 0;
		for (const char *s = t->texto; *s && n < 70; s++)
			if (((unsigned char)*s & 0xC0) != 0x80)
				visto[n++] = '*';
		visto[n] = 0;
	} else {
		strcpy(visto, t->texto);
	}
	// Si no cabe, se ve el final (lo último que se ha escrito).
	const char *v = visto;
	while (*v && ancho_texto(&letra_normal, v) > w - 20)
		v++;
	while (((unsigned char)*v & 0xC0) == 0x80)
		v++;
	int ancho = texto(p, &letra_normal, x + 8, y + 6, v, C_BLANCO);
	if ((imagenes / 30) % 2 == 0)
		rect(p, x + 9 + ancho, y + 6, 1, 14, C_VIOLETA);
}
