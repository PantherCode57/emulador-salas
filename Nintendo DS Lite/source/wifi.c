// La pantalla del wifi: las redes que hay cerca, y conectarse a una.
//
// El wifi de la DS Lite solo entra en redes abiertas (sin contraseña) o con WEP; con WPA o WPA2 no
// puede, y eso no se arregla con programas. La red elegida se apunta en la tarjeta (wifi.txt) y al
// encender se conecta sola. También sirven las que se guardaron en los ajustes de Nintendo WFC.
#include <stdio.h>
#include <string.h>

#include "sistema.h"

#define MAX_REDES 24
#define FILA_Y 30
#define FILA_H 28
#define FILAS_VISTAS 5

static Red redes[MAX_REDES];
static int n_redes, sel, primero, primero_al_tocar;
static bool con_teclado;
static Teclado tec;
static Red elegida;
static char red_actual[40]; // a la que se conecta o está conectada
static char aviso[110];

// Las claves WEP son de 5 o 13 letras, o de 10 o 26 cifras hexadecimales.
static int clave_bytes(const char *s, uint8_t *out)
{
	int n = (int)strlen(s);
	if (n == 5 || n == 13) {
		memcpy(out, s, n);
		return n;
	}
	if (n != 10 && n != 26)
		return -1;
	for (int i = 0; i < n / 2; i++) {
		int v = 0;
		for (int k = 0; k < 2; k++) {
			char c = s[i * 2 + k];
			int d = (c >= '0' && c <= '9') ? c - '0' : (c >= 'a' && c <= 'f') ? c - 'a' + 10 : (c >= 'A' && c <= 'F') ? c - 'A' + 10 : -1;
			if (d < 0)
				return -1;
			v = v * 16 + d;
		}
		out[i] = (uint8_t)v;
	}
	return n / 2;
}

static void conectar(const char *ssid, const char *clave, bool guardada)
{
	uint8_t bytes[32] = {0};
	int nb = 0;
	if (clave[0]) {
		nb = clave_bytes(clave, bytes);
		if (nb < 0)
			nb = 0;
	}
	plat_wifi_conectar(ssid, bytes, nb, guardada && !clave[0]);
	snprintf(red_actual, sizeof red_actual, "%s", ssid[0] ? ssid : "(sin nombre)");
	aviso[0] = 0;
}

void wifi_arrancar(void)
{
	char ssid[40], clave[72];
	bool guardada = false;
	if (wifi_leer(ssid, sizeof ssid, clave, sizeof clave, &guardada) && ssid[0]) {
		conectar(ssid, clave, guardada);
	} else if (plat_wifi_n_guardadas() > 0) {
		plat_wifi_guardadas();
		snprintf(red_actual, sizeof red_actual, "la red guardada en la DS");
	}
}

const char *wifi_resumen(void)
{
	static char s[128];
	switch (plat_wifi_estado()) {
	case RED_APAGADA:
		return "Wifi: no arranca";
	case RED_BUSCANDO:
		return "Wifi: buscando redes…";
	case RED_CONECTANDO:
		snprintf(s, sizeof s, "Wifi: conectando con %s…", red_actual);
		return s;
	case RED_CONECTADA:
		if (puente_estado() == PU_LISTO)
			snprintf(s, sizeof s, "Wifi: %s · puente: %s", red_actual, puente_nombre());
		else
			snprintf(s, sizeof s, "Wifi: %s · sin puente", red_actual);
		return s;
	default:
		return "Wifi: sin conectar · toca para elegir";
	}
}

void wifi_entrar(void)
{
	con_teclado = false;
	aviso[0] = 0;
	int e = plat_wifi_estado();
	if (e != RED_CONECTADA && e != RED_CONECTANDO && e != RED_BUSCANDO)
		plat_wifi_buscar();
	n_redes = plat_wifi_redes(redes, MAX_REDES);
	sel = primero = 0;
}

static void elegir(int i)
{
	if (i < 0 || i >= n_redes)
		return;
	Red *r = &redes[i];
	if (r->seguridad == SEG_WPA) {
		snprintf(aviso, sizeof aviso, "%s es WPA: la DS Lite no puede entrar en ella", r->ssid[0] ? r->ssid : "Esa red");
	} else if (r->seguridad == SEG_WEP && !r->guardada) {
		elegida = *r;
		teclado_iniciar(&tec, "");
		con_teclado = true;
	} else {
		conectar(r->ssid, "", r->guardada);
		wifi_guardar(r->ssid, "", r->guardada);
	}
}

static void ver_elegida(void)
{
	if (sel < primero)
		primero = sel;
	if (sel >= primero + FILAS_VISTAS)
		primero = sel - FILAS_VISTAS + 1;
}

int wifi_paso(const Entrada *e)
{
	if (con_teclado) {
		if (e->toque_ini && en(e->tx, e->ty, 0, 0, 64, 28)) { // «Atrás»
			con_teclado = false;
			return CAMBIO;
		}
		switch (teclado_paso(&tec, e)) {
		case TECLADO_ACEPTAR: {
			uint8_t b[32];
			if (clave_bytes(tec.texto, b) < 0) {
				snprintf(aviso, sizeof aviso, "La clave WEP tiene 5 o 13 letras, o 10 o 26 cifras");
				return CAMBIO;
			}
			conectar(elegida.ssid, tec.texto, false);
			wifi_guardar(elegida.ssid, tec.texto, false);
			con_teclado = false;
			return CAMBIO;
		}
		case TECLADO_CANCELAR:
			con_teclado = false;
			return CAMBIO;
		case TECLADO_CAMBIO:
			return CAMBIO;
		}
		return (imagenes % 30) == 0 ? CAMBIO : NADA;
	}

	if (toca_atras(e))
		return SALIR;
	int cambio = NADA;
	if ((imagenes % 20) == 0) {
		n_redes = plat_wifi_redes(redes, MAX_REDES);
		if (sel >= n_redes)
			sel = n_redes > 0 ? n_redes - 1 : 0;
		cambio = CAMBIO;
	}
	if (e->pulsado & B_Y) {
		plat_wifi_buscar();
		snprintf(aviso, sizeof aviso, "Buscando redes otra vez…");
		return CAMBIO;
	}
	bool hay_guardadas = plat_wifi_n_guardadas() > 0;
	if (hay_guardadas && ((e->pulsado & B_X) || (e->toque && en(e->toque_x, e->toque_y, 8, 172, 240, 19)))) {
		plat_wifi_guardadas();
		snprintf(red_actual, sizeof red_actual, "la red guardada en la DS");
		wifi_guardar("", "", false); // la próxima vez, también las de la DS
		return CAMBIO;
	}
	if (e->repetido & (B_ARRIBA | B_ABAJO)) {
		if (n_redes > 0) {
			sel += (e->repetido & B_ABAJO) ? 1 : -1;
			if (sel < 0)
				sel = 0;
			if (sel >= n_redes)
				sel = n_redes - 1;
			ver_elegida();
		}
		return CAMBIO;
	}
	if (e->pulsado & B_A) {
		elegir(sel);
		return CAMBIO;
	}
	// Con el dedo: arrastrar mueve la lista y un toque corto elige la red.
	if (e->toque_ini)
		primero_al_tocar = primero;
	if (e->arrastrando && n_redes > FILAS_VISTAS) {
		int p = primero_al_tocar - e->arrastre_dy / FILA_H;
		if (p > n_redes - FILAS_VISTAS)
			p = n_redes - FILAS_VISTAS;
		if (p < 0)
			p = 0;
		if (p != primero) {
			primero = p;
			return CAMBIO;
		}
	}
	if (e->toque && en(e->toque_x, e->toque_y, 0, FILA_Y, ANCHO, FILA_H * FILAS_VISTAS)) {
		int i = primero + (e->toque_y - FILA_Y) / FILA_H;
		if (i < n_redes) {
			sel = i;
			elegir(i);
		}
		return CAMBIO;
	}
	return cambio;
}

static void barras(Color *p, int x, int y, int n, Color c)
{
	for (int i = 0; i < 4; i++) {
		int h = 3 + i * 2;
		rect(p, x + i * 4, y + 10 - h, 3, h, i < n ? c : C_OSCURO);
	}
}

static void dibujar_arriba(Color *p)
{
	llenar(p, C_FONDO);
	degradado(p, 0, 0, ANCHO, 90, RGB(40, 34, 80), C_FONDO);
	barra_arriba(p, "Panther OS");

	if (con_teclado) {
		char t[80];
		snprintf(t, sizeof t, "Clave WEP de %s", elegida.ssid);
		texto_corto(p, &letra_titulo, 12, 30, 232, t, C_BLANCO);
		teclado_dibujar_texto(&tec, p, 12, 58, 232, false);
		texto_parrafo(p, &letra_peq, 12, 92, 232,
			"Son 5 o 13 letras, o 10 o 26 cifras (de 0 a 9 y de A a F). Se apunta en la tarjeta para que "
			"la DS se conecte sola la próxima vez.", C_GRIS);
		if (aviso[0])
			texto_parrafo(p, &letra_peq, 12, 140, 232, aviso, C_AMARILLO);
		ayuda(p, "START aceptar     B borrar     X mayúsculas");
		return;
	}

	int e = plat_wifi_estado();
	const char *titulo = "Sin conectar", *sub = "Elige una red en la pantalla de abajo";
	char linea2[96];
	switch (e) {
	case RED_APAGADA:
		titulo = "El wifi no arranca";
		sub = "Apaga la DS y vuelve a encenderla";
		break;
	case RED_BUSCANDO:
		titulo = "Buscando redes…";
		snprintf(linea2, sizeof linea2, "%d a la vista", n_redes);
		sub = linea2;
		break;
	case RED_CONECTANDO:
		titulo = "Conectando…";
		sub = red_actual;
		break;
	case RED_CONECTADA:
		titulo = "Conectado";
		snprintf(linea2, sizeof linea2, "%s · la DS es %s", red_actual, plat_wifi_ip());
		sub = linea2;
		break;
	case RED_FALLO:
		titulo = "No se ha podido conectar";
		snprintf(linea2, sizeof linea2, "con %s. Prueba otra vez o elige otra.", red_actual);
		sub = linea2;
		break;
	}
	// Las barras grandes.
	for (int i = 0; i < 4; i++) {
		int h = 8 + i * 6;
		bool on = e == RED_CONECTADA || ((e == RED_CONECTANDO || e == RED_BUSCANDO) && i <= (int)((imagenes / 15) % 4));
		redondeado(p, 14 + i * 9, 58 - h, 6, h, 2, on ? C_VIOLETA : C_OSCURO, 256);
	}
	texto(p, &letra_titulo, 56, 28, titulo, C_BLANCO);
	texto_corto(p, &letra_peq, 57, 48, 190, sub, C_SUAVE);
	if (e == RED_CONECTADA) {
		char s[96];
		estado_puente(s + 8, sizeof s - 8);
		memcpy(s, "Puente: ", 8);
		texto_corto(p, &letra_peq, 57, 62, 190, s, C_GRIS);
	}

	redondeado(p, 8, 82, 240, 70, 10, C_PANEL, 256);
	texto_parrafo(p, &letra_peq, 16, 86, 226,
		"La DS Lite solo entra en wifis abiertos (sin contraseña) o con WEP. El de casa suele ser WPA2 "
		"y no le vale: puedes usar la zona wifi del móvil sin contraseña mientras uses la DS.", C_SUAVE);
	if (aviso[0])
		texto_corto(p, &letra_peq, 10, 158, 236, aviso, C_AMARILLO);
	ayuda(p, plat_wifi_n_guardadas() > 0 ? "A conectar   Y buscar   X las de la DS   B volver"
	                                       : "A conectar     Y buscar otra vez     B volver");
}

static void dibujar_abajo(Color *p)
{
	if (con_teclado) {
		teclado_dibujar(&tec, p, "Clave del wifi");
		return;
	}
	llenar(p, C_FONDO);
	barra_abajo(p, "Wifi");
	if (n_redes == 0) {
		texto_centro(p, &letra_normal, ANCHO / 2, 90,
			plat_wifi_estado() == RED_BUSCANDO ? "Buscando redes…" : "No hay redes a la vista", C_GRIS);
	}
	int e = plat_wifi_estado();
	for (int k = 0; k < FILAS_VISTAS && primero + k < n_redes; k++) {
		int i = primero + k;
		const Red *r = &redes[i];
		int y = FILA_Y + k * FILA_H;
		redondeado(p, 6, y + 1, 238, FILA_H - 3, 8, i == sel ? C_PANEL2 : C_PANEL, 256);
		if (i == sel)
			marco(p, 6, y + 1, 238, FILA_H - 3, 8, 1, C_VIOLETA);
		barras(p, 14, y + 7, r->barras, r->seguridad == SEG_WPA ? C_GRIS : C_BLANCO);
		texto_corto(p, &letra_normal, 36, y + 6, 128, r->ssid[0] ? r->ssid : "(sin nombre)",
			r->seguridad == SEG_WPA ? C_GRIS : C_BLANCO);
		const char *etiqueta;
		Color c;
		if (e == RED_CONECTADA && !strcmp(r->ssid, red_actual)) {
			etiqueta = "Conectada";
			c = C_VERDE;
		} else if (r->seguridad == SEG_WPA) {
			etiqueta = "WPA: no vale";
			c = C_OSCURO;
		} else if (r->guardada) {
			etiqueta = "Guardada";
			c = C_VIOLETA;
		} else if (r->seguridad == SEG_WEP) {
			etiqueta = "WEP";
			c = C_AMARILLO;
		} else {
			etiqueta = "Abierta";
			c = C_VERDE;
		}
		texto_derecha(p, &letra_peq, 236, y + 8, etiqueta, c);
	}
	if (n_redes > FILAS_VISTAS) { // la barrita de la derecha
		int alto = FILA_H * FILAS_VISTAS;
		int h = alto * FILAS_VISTAS / n_redes, y = FILA_Y + (alto - h) * primero / (n_redes - FILAS_VISTAS);
		redondeado(p, 248, y, 4, h, 2, C_OSCURO, 256);
	}
	if (plat_wifi_n_guardadas() > 0)
		boton(p, 8, 172, 240, 19, "Usar las guardadas en la DS", C_PANEL2, C_PANEL, false);
}

void wifi_dibujar(Color *arriba, Color *abajo)
{
	dibujar_arriba(arriba);
	dibujar_abajo(abajo);
}
