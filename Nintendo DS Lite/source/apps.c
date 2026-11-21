// Las apps de Panther OS en la DS: WhatsApp, Spotin, Instagram, YouTube, el navegador y la tienda.
//
// La DS sola no puede con ellas (tiene 4 MB de memoria, un procesador de 67 MHz y un wifi que solo
// entra en redes abiertas o con WEP), así que el trabajo lo hará el puente y la DS enseñará lo que
// le mande. Mientras no esté el puente, cada app dice qué le falta para funcionar.
// Los iconos son dibujos sencillos hechos aquí, no los logotipos de cada marca.
#include <math.h>
#include <stdio.h>
#include <string.h>

#include "sistema.h"

static void icono_chat(Color *p, int cx, int cy)
{
	circulo(p, cx + 1, cy - 2, 12.5f, C_BLANCO, 256);
	triangulo(p, cx - 12, cy + 12, cx - 8, cy + 1, cx - 1, cy + 8, C_BLANCO);
	Color c = RGB(7, 94, 84);
	for (int i = -1; i <= 1; i++)
		circulo(p, cx + 1 + i * 5.5f, cy - 2, 2, c, 256);
}

static void icono_musica(Color *p, int cx, int cy)
{
	circulo(p, cx - 7, cy + 8, 4.5f, C_BLANCO, 256);
	circulo(p, cx + 7, cy + 5, 4.5f, C_BLANCO, 256);
	linea(p, cx - 3, cy + 8, cx - 3, cy - 9, 2.2f, C_BLANCO);
	linea(p, cx + 11, cy + 5, cx + 11, cy - 12, 2.2f, C_BLANCO);
	linea(p, cx - 3, cy - 10, cx + 11, cy - 13, 4.5f, C_BLANCO);
}

static void icono_foto(Color *p, int cx, int cy)
{
	marco(p, cx - 14, cy - 12, 28, 24, 5, 2, C_BLANCO);
	triangulo(p, cx - 9, cy + 7, cx - 2, cy - 3, cx + 5, cy + 7, C_BLANCO);
	triangulo(p, cx + 1, cy + 7, cx + 5, cy + 1, cx + 10, cy + 7, C_BLANCO);
	circulo(p, cx + 5, cy - 5, 2.5f, C_BLANCO, 256);
}

static void icono_play(Color *p, int cx, int cy)
{
	aro(p, cx, cy, 13, 2.5f, C_BLANCO, 0);
	triangulo(p, cx - 4, cy - 7, cx - 4, cy + 7, cx + 8, cy, C_BLANCO);
}

static void icono_mundo(Color *p, int cx, int cy)
{
	aro(p, cx, cy, 13, 2.2f, C_BLANCO, 0);
	elipse(p, cx, cy, 5.5f, 13, 1.8f, C_BLANCO);
	linea(p, cx - 13, cy, cx + 13, cy, 1.8f, C_BLANCO);
	linea(p, cx - 10.5f, cy - 7, cx + 10.5f, cy - 7, 1.5f, C_BLANCO);
	linea(p, cx - 10.5f, cy + 7, cx + 10.5f, cy + 7, 1.5f, C_BLANCO);
}

static void icono_bolsa(Color *p, int cx, int cy)
{
	aro(p, cx, cy - 4, 6.5f, 2.5f, C_BLANCO, -1);
	redondeado(p, cx - 12, cy - 5, 24, 19, 4, C_BLANCO, 256);
	circulo(p, cx - 6, cy, 1.6f, RGB(90, 50, 200), 256);
	circulo(p, cx + 6, cy, 1.6f, RGB(90, 50, 200), 256);
}

const App apps[N_APPS] = {
	{"WhatsApp", "Tus chats y mensajes",
		"Tus chats y tus mensajes, y contestas con el teclado de abajo. Va con tu cuenta: se vincula "
		"desde aquí con tu número, como un dispositivo más.",
		RGB(0, 175, 135), RGB(7, 94, 84), icono_chat},
	{"Spotin", "Tu música",
		"Buscarás canciones y las oirás por los altavoces de la DS. El puente baja la canción y se la "
		"pasa a la DS ya preparada, en poca calidad para que su wifi pueda con ella.",
		RGB(30, 215, 96), RGB(12, 120, 55), icono_musica},
	{"Instagram", "Fotos, reels e historias",
		"Verás tu inicio, las historias y tus mensajes. El puente baja las fotos y las hace pequeñas, a "
		"la medida de las pantallas de la DS.",
		RGB(247, 119, 55), RGB(193, 53, 132), icono_foto},
	{"YouTube", "Vídeos",
		"Buscarás vídeos y los verás en pequeño. La DS no puede con un vídeo normal: el puente lo "
		"convierte en uno muy pequeño, con pocas imágenes por segundo.",
		RGB(255, 60, 60), RGB(170, 10, 20), icono_play},
	{"Navegador", "Páginas web",
		"Abrirás páginas web. El puente las abre de verdad y le manda a la DS cómo se ven; tú tocas, "
		"bajas y escribes desde la DS.",
		RGB(70, 140, 250), RGB(40, 60, 170), icono_mundo},
	{"Tienda", "Panther Store",
		"Pondrás al día Panther OS de la DS y las apps nuevas cuando las haya. Lo que bajes se guarda "
		"en la tarjeta.",
		RGB(160, 120, 255), RGB(90, 50, 200), icono_bolsa},
};

static int actual;

void app_entrar(int i)
{
	actual = i;
	if (i == 0)
		wa_entrar();
}

// Las tres filas de la pantalla de abajo: wifi, puente y la app.
void fila_estado(Color *p, int y, int estado, const char *titulo, const char *detalle)
{
	// estado: 0 = hecho, 1 = esperando, 2 = falta
	redondeado(p, 8, y, 240, 46, 10, C_PANEL, 256);
	int cx = 28, cy = y + 23;
	if (estado == 0) {
		circulo(p, cx, cy, 10, C_VERDE, 256);
		linea(p, cx - 5, cy, cx - 1, cy + 4, 2.2f, C_BLANCO);
		linea(p, cx - 1, cy + 4, cx + 5, cy - 4, 2.2f, C_BLANCO);
	} else if (estado == 1) {
		aro(p, cx, cy, 8.5f, 2.5f, C_OSCURO, 0);
		float a = (imagenes % 60) / 60.0f * 6.2832f;
		for (int i = 0; i < 3; i++) {
			float b = a + i * 0.35f;
			circulo(p, cx + 8.5f * cosf(b), cy + 8.5f * sinf(b), 2, C_AMARILLO, 256 - i * 70);
		}
	} else {
		aro(p, cx, cy, 8.5f, 2.5f, C_OSCURO, 0);
	}
	texto(p, &letra_negrita, 46, y + 6, titulo, C_BLANCO);
	texto_corto(p, &letra_peq, 46, y + 25, 196, detalle, estado == 0 ? C_SUAVE : C_GRIS);
}

int estado_wifi(char *detalle, int n)
{
	int e = plat_wifi_estado();
	if (e == RED_CONECTADA) {
		snprintf(detalle, n, "Conectado. La DS es %s", plat_wifi_ip());
		return 0;
	}
	if (e == RED_CONECTANDO) {
		snprintf(detalle, n, "Conectando…");
		return 1;
	}
	snprintf(detalle, n, "Toca aquí para elegir un wifi abierto");
	return 2;
}

int estado_puente(char *detalle, int n)
{
	switch (puente_estado()) {
	case PU_SIN_RED:
		snprintf(detalle, n, "Primero hace falta el wifi");
		return 2;
	case PU_BUSCANDO:
		snprintf(detalle, n, "Buscándolo en el wifi…");
		return 1;
	case PU_CONECTANDO:
	case PU_SALUDO:
		snprintf(detalle, n, "Conectando con %s…", puente_nombre());
		return 1;
	case PU_SIN_CODIGO:
		snprintf(detalle, n, "Falta su código: pulsa A para escribirlo");
		return 2;
	case PU_CODIGO_MAL:
		snprintf(detalle, n, "Ese código no es: pulsa A para escribirlo");
		return 2;
	case PU_LISTO:
		snprintf(detalle, n, "%s (%s)", puente_nombre(), puente_ip());
		return 0;
	}
	snprintf(detalle, n, "No contesta: vuelvo a probar…");
	return 1;
}

int app_paso(const Entrada *e)
{
	if (actual == 0)
		return wa_paso(e);
	if (toca_atras(e))
		return SALIR;
	if ((e->toque_ini && en(e->tx, e->ty, 8, 36, 240, 46)) || (e->pulsado & B_A)) {
		wifi_entrar();
		return IR_WIFI;
	}
	// Lo que se mueve (el círculo de esperar) se dibuja otra vez cada poco.
	return (imagenes % 4) == 0 ? CAMBIO : NADA;
}

void app_dibujar(Color *arriba, Color *abajo)
{
	if (actual == 0) {
		wa_dibujar(arriba, abajo);
		return;
	}
	const App *a = &apps[actual];

	// Arriba: qué es la app y qué hará.
	llenar(arriba, C_FONDO);
	degradado(arriba, 0, 0, ANCHO, 96, mezclar(C_FONDO, a->c2, 110), C_FONDO);
	barra_arriba(arriba, "Panther OS");
	redondeado_degradado(arriba, 14, 30, 60, 60, 16, a->c1, a->c2);
	a->icono(arriba, 44, 58);
	texto(arriba, &letra_grande, 86, 36, a->nombre, C_BLANCO);
	texto(arriba, &letra_normal, 87, 66, a->descripcion, C_SUAVE);
	redondeado(arriba, 10, 100, 236, 72, 10, C_PANEL, 230);
	texto_parrafo(arriba, &letra_peq, 18, 105, 222, a->que_hace, C_SUAVE);
	ayuda(arriba, "A: elegir el wifi     B: volver");

	// Abajo: lo que le falta para funcionar.
	llenar(abajo, C_FONDO);
	barra_abajo(abajo, a->nombre);
	char detalle[80];
	int w = estado_wifi(detalle, sizeof detalle);
	fila_estado(abajo, 36, w, "Wifi", detalle);
	int pu = estado_puente(detalle, sizeof detalle);
	fila_estado(abajo, 88, pu, "Puente", detalle);
	fila_estado(abajo, 140, 2, a->nombre, "Llegará pronto: primero va WhatsApp");
}
