// Los vídeos en la DS (YouTube y los de Instagram). Van por otra conexión con el puente (con el
// mismo saludo), que manda las imágenes en JPEG pequeños y el sonido en ADPCM (puente/video.go).
//
// El reloj es el sonido: la DS toca un anillo de muestras (plat_audio_*) y la imagen que se enseña
// es la última que ya toca según lo que lleva tocado. Si el sonido se acaba antes de tiempo (el wifi
// no llega), se para, se espera a tener un poco y sigue. Cada poco se le dice al puente por dónde
// va («L <bytes> <ms>»), para que no se adelante y se salte imágenes si el wifi no da para todas.
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sistema.h"

enum { V_CERRADO, V_CONECTANDO, V_PIDIENDO, V_VIENDO, V_PAUSA, V_FIN, V_ERROR };

#define RX_V 65536
#define COLA_BYTES (192 * 1024)
#define MAX_CUADROS 40

typedef struct {
	uint32_t ts;
	int ini, n;
} Cuadro;

static int fase = V_CERRADO;
static Canal can = {.sock = -1};
static char clave[32], titulo[128], aviso[120];
static uint32_t desde, dur, pedido_en;
static int vw, vh;
static bool con_sonido, fin_recibido, acabado, visto_algo;
static uint8_t rx[RX_V];
static int nrx;
static uint32_t recibidos, ultimo_l;

static uint8_t cola[COLA_BYTES];
static Cuadro cuadros[MAX_CUADROS];
static int ncuadros, cola_fin;
static Color imagen[ANCHO * ALTO];
static int n_imagenes;

static int16_t anillo[AUDIO_ANILLO] __attribute__((aligned(32)));
static uint32_t escritas, ts0, reloj_parado, muestras_total, ms_inicio;
static bool sonando, en_marcha, ts0_puesto;
static int tasa;

bool video_acabado(void) { return acabado; }
int video_imagenes(void) { return n_imagenes; }
uint32_t video_muestras(void) { return muestras_total; }

static uint32_t reloj(void)
{
	if (sonando)
		return ts0 + (uint32_t)((uint64_t)plat_audio_tocadas() * 1000 / (uint32_t)tasa);
	if (!con_sonido && en_marcha)
		return reloj_parado + (ahora_ms - ms_inicio);
	return reloj_parado;
}

static void soltar(void)
{
	plat_audio_parar();
	canal_cerrar(&can);
	sonando = en_marcha = false;
}

static void empezar(uint32_t ms)
{
	soltar();
	desde = reloj_parado = ms;
	nrx = ncuadros = cola_fin = 0;
	escritas = recibidos = 0;
	ts0_puesto = fin_recibido = acabado = false;
	aviso[0] = 0;
	tasa = plat_audio_tasa();
	if (puente_estado() != PU_LISTO || !canal_abrir(&can, puente_ip())) {
		snprintf(aviso, sizeof aviso, "Hace falta el puente: mira el wifi");
		fase = V_ERROR;
		return;
	}
	fase = V_CONECTANDO;
	pedido_en = ahora_ms;
}

void video_abrir(const char *c, const char *t, uint32_t desde_ms)
{
	snprintf(clave, sizeof clave, "%s", c);
	snprintf(titulo, sizeof titulo, "%s", t ? t : "");
	dur = 0;
	vw = vh = 0;
	visto_algo = false;
	n_imagenes = 0;
	muestras_total = 0;
	for (int i = 0; i < ANCHO * ALTO; i++)
		imagen[i] = RGB(0, 0, 0);
	empezar(desde_ms);
}

void video_cerrar(void)
{
	soltar();
	fase = V_CERRADO;
}

// --- Lo que llega ---

static void sonido(uint32_t ts, const uint8_t *d, int n)
{
	if (n < 4)
		return;
	if (!ts0_puesto) {
		ts0 = ts;
		ts0_puesto = true;
	}
	int pred = (int16_t)(d[0] | (d[1] << 8)), idx = d[2] > 88 ? 88 : d[2];
	static const int16_t pasos[89] = {7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31, 34, 37, 41, 45,
		50, 55, 60, 66, 73, 80, 88, 97, 107, 118, 130, 143, 157, 173, 190, 209, 230, 253, 279, 307, 337, 371, 408,
		449, 494, 544, 598, 658, 724, 796, 876, 963, 1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066, 2272, 2499,
		2749, 3024, 3327, 3660, 4026, 4428, 4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635,
		13899, 15289, 16818, 18500, 20350, 22385, 24623, 27086, 29794, 32767};
	static const int8_t indices[16] = {-1, -1, -1, -1, 2, 4, 6, 8, -1, -1, -1, -1, 2, 4, 6, 8};
	int muestras = (n - 4) * 2;
	uint32_t tocadas = sonando ? plat_audio_tocadas() : 0;
	int libre = AUDIO_ANILLO - (int)(escritas - tocadas) - 512;
	if (muestras > libre)
		return; // no cabe (no debería pasar: el puente no se adelanta tanto)
	int pos = (int)(escritas % AUDIO_ANILLO), primera = pos;
	for (int i = 0; i < muestras; i++) {
		int c = (d[4 + i / 2] >> ((i & 1) * 4)) & 15;
		int paso = pasos[idx], dif = paso >> 3;
		if (c & 4)
			dif += paso;
		if (c & 2)
			dif += paso >> 1;
		if (c & 1)
			dif += paso >> 2;
		pred += (c & 8) ? -dif : dif;
		pred = pred < -32768 ? -32768 : pred > 32767 ? 32767 : pred;
		idx += indices[c];
		idx = idx < 0 ? 0 : idx > 88 ? 88 : idx;
		anillo[pos] = (int16_t)pred;
		if (++pos == AUDIO_ANILLO) {
			plat_audio_escrito(anillo + primera, pos - primera);
			pos = primera = 0;
		}
	}
	if (pos > primera)
		plat_audio_escrito(anillo + primera, pos - primera);
	escritas += muestras;
	muestras_total += muestras;
}

static void imagen_nueva(uint32_t ts, const uint8_t *d, int n)
{
	if (n > COLA_BYTES)
		return;
	while (ncuadros > 0 && (ncuadros == MAX_CUADROS || cola_fin + n > COLA_BYTES)) {
		// sin sitio: se compacta y, si aun así no cabe, fuera la más vieja
		if (cuadros[0].ini > 0) {
			int base = cuadros[0].ini;
			memmove(cola, cola + base, cola_fin - base);
			cola_fin -= base;
			for (int i = 0; i < ncuadros; i++)
				cuadros[i].ini -= base;
			if (ncuadros < MAX_CUADROS && cola_fin + n <= COLA_BYTES)
				break;
		}
		memmove(cuadros, cuadros + 1, (ncuadros - 1) * sizeof cuadros[0]);
		ncuadros--;
		if (ncuadros == 0)
			cola_fin = 0;
	}
	if (cola_fin + n > COLA_BYTES)
		return;
	memcpy(cola + cola_fin, d, n);
	cuadros[ncuadros].ts = ts;
	cuadros[ncuadros].ini = cola_fin;
	cuadros[ncuadros].n = n;
	ncuadros++;
	cola_fin += n;
}

// paquetes: los que estén enteros en rx.
static void paquetes(void)
{
	int p = 0;
	while (nrx - p >= 8) {
		const uint8_t *c = rx + p;
		int n = c[1] | (c[2] << 8) | (c[3] << 16);
		uint32_t ts = (uint32_t)c[4] | ((uint32_t)c[5] << 8) | ((uint32_t)c[6] << 16) | ((uint32_t)c[7] << 24);
		if (n > RX_V - 8) {
			snprintf(aviso, sizeof aviso, "El puente manda algo raro");
			fase = V_ERROR;
			soltar();
			return;
		}
		if (nrx - p < 8 + n)
			break;
		switch (c[0]) {
		case 'V':
			imagen_nueva(ts, c + 8, n);
			break;
		case 'A':
			sonido(ts, c + 8, n);
			break;
		case 'F':
			fin_recibido = true;
			break;
		case 'E': {
			int k = n < (int)sizeof aviso - 1 ? n : (int)sizeof aviso - 1;
			memcpy(aviso, c + 8, k);
			aviso[k] = 0;
			fase = V_ERROR;
			soltar();
			return;
		}
		}
		p += 8 + n;
	}
	if (p > 0) {
		memmove(rx, rx + p, nrx - p);
		nrx -= p;
	}
}

static bool leer(void)
{
	for (int vueltas = 0; vueltas < 24; vueltas++) {
		int libre = RX_V - nrx;
		if (libre <= 0)
			break;
		int k = canal_recibir(&can, rx + nrx, libre > 2048 ? 2048 : libre);
		if (k < 0)
			return false;
		if (k == 0)
			break;
		nrx += k;
		recibidos += k;
	}
	return true;
}

// la respuesta al VER: «R 1» y «OK <dur> <w> <h> <sonido>» (o ERROR …)
static void respuesta_ver(void)
{
	char *a = memchr(rx, '\n', nrx);
	char *b = a ? memchr(a + 1, '\n', nrx - (a + 1 - (char *)rx)) : NULL;
	if (!b) {
		if (ahora_ms - pedido_en > 70000) {
			snprintf(aviso, sizeof aviso, "El puente no contesta");
			fase = V_ERROR;
			soltar();
		}
		return;
	}
	*b = 0;
	char *l = a + 1;
	int usado = (int)(b + 1 - (char *)rx);
	if (!strncmp(l, "OK ", 3)) {
		unsigned d = 0;
		int w = 0, h = 0, s = 0;
		sscanf(l + 3, "%u %d %d %d", &d, &w, &h, &s);
		dur = d;
		vw = w > ANCHO ? ANCHO : w;
		vh = h > ALTO ? ALTO : h;
		con_sonido = s != 0;
		fase = V_VIENDO;
		ultimo_l = 0;
	} else {
		snprintf(aviso, sizeof aviso, "%s", !strncmp(l, "ERROR ", 6) ? l + 6 : "El puente no puede con este vídeo");
		fase = V_ERROR;
		soltar();
	}
	memmove(rx, rx + usado, nrx - usado);
	nrx -= usado;
}

// enseñar: la imagen que toca según el reloj (y fuera las que ya han pasado).
static bool ensenar(void)
{
	uint32_t r = reloj();
	int k = -1;
	for (int i = 0; i < ncuadros; i++)
		if (cuadros[i].ts <= r || (!visto_algo && i == 0))
			k = i;
	if (k < 0)
		return false;
	Cuadro c = cuadros[k];
	int x0 = (ANCHO - vw) / 2, y0 = (ALTO - vh) / 2;
	jpeg_pintar(cola + c.ini, c.n, imagen, ANCHO, ALTO, x0, y0);
	n_imagenes++;
	visto_algo = true;
	memmove(cuadros, cuadros + k + 1, (ncuadros - k - 1) * sizeof cuadros[0]);
	ncuadros -= k + 1;
	if (ncuadros == 0)
		cola_fin = 0;
	return true;
}

static void reloj_paso(void)
{
	if (con_sonido) {
		uint32_t t = sonando ? plat_audio_tocadas() : 0;
		if (sonando && t + (uint32_t)tasa / 20 >= escritas && !fin_recibido) {
			// se acaba el sonido: a esperar a tener más (lo poco que quede se pierde)
			reloj_parado = reloj();
			plat_audio_parar();
			sonando = false;
			escritas = 0;
			ts0_puesto = false;
		} else if (sonando && fin_recibido && t >= escritas) {
			reloj_parado = reloj();
			plat_audio_parar();
			sonando = false;
			acabado = true;
		} else if (!sonando && !acabado && escritas > 0 && (escritas >= (uint32_t)tasa * 2 / 5 || fin_recibido)) {
			plat_audio_empezar(anillo);
			sonando = true;
		}
	} else {
		if (!en_marcha && ncuadros > 0) {
			en_marcha = true;
			ms_inicio = ahora_ms;
		}
		if (fin_recibido && ncuadros == 0 && en_marcha) {
			reloj_parado = reloj();
			en_marcha = false;
			acabado = true;
		}
	}
	if (acabado)
		fase = V_FIN;
}

static void saltar(int ms)
{
	int64_t p = (int64_t)reloj() + ms;
	if (p < 0)
		p = 0;
	if (dur > 0 && p > (int64_t)dur - 1000)
		p = dur > 1000 ? dur - 1000 : 0;
	empezar((uint32_t)p);
}

int video_paso(const Entrada *e)
{
	bool cambio = false;
	switch (fase) {
	case V_CONECTANDO: {
		int r = canal_paso(&can, puente_codigo());
		if (r == 1) {
			char p[96];
			snprintf(p, sizeof p, "VER %s %u %d", clave, (unsigned)desde, tasa);
			if (canal_enviar(&can, p)) {
				fase = V_PIDIENDO;
				pedido_en = ahora_ms;
			} else {
				r = -1;
			}
		}
		if (r < 0) {
			snprintf(aviso, sizeof aviso, "No se puede hablar con el puente");
			fase = V_ERROR;
			soltar();
			cambio = true;
		}
		break;
	}
	case V_PIDIENDO:
		if (!leer()) {
			snprintf(aviso, sizeof aviso, "El puente ha cerrado");
			fase = V_ERROR;
			soltar();
		} else {
			respuesta_ver();
		}
		cambio = fase != V_PIDIENDO;
		if (fase != V_VIENDO)
			break;
		/* fall through */
	case V_VIENDO:
		if (!leer()) {
			if (!fin_recibido) {
				snprintf(aviso, sizeof aviso, "Se ha cortado: pulsa A para seguir");
				reloj_parado = reloj();
				soltar();
				fase = V_PAUSA;
				cambio = true;
				break;
			}
		}
		paquetes();
		if (fase != V_VIENDO) {
			cambio = true;
			break;
		}
		reloj_paso();
		if (ensenar())
			cambio = true;
		if (fase == V_VIENDO && ahora_ms - ultimo_l > 150) {
			char l[48];
			snprintf(l, sizeof l, "L %u %u", (unsigned)recibidos, (unsigned)reloj());
			canal_enviar(&can, l);
			ultimo_l = ahora_ms;
		}
		if ((imagenes % 10) == 0)
			cambio = true; // la barra del tiempo
		break;
	default:
		break;
	}
	if (fase == V_FIN && can.fase != CANAL_CERRADO)
		soltar();

	if (!e)
		return cambio ? CAMBIO : NADA;
	if (toca_atras(e)) {
		video_cerrar();
		return SALIR;
	}
	bool pausa = (e->pulsado & B_A) || (e->toque_ini && en(e->tx, e->ty, 98, 120, 60, 44));
	if (pausa) {
		if (fase == V_VIENDO || fase == V_CONECTANDO || fase == V_PIDIENDO) {
			reloj_parado = reloj();
			soltar();
			fase = V_PAUSA;
		} else if (fase == V_FIN) {
			empezar(0);
		} else if (fase == V_PAUSA || fase == V_ERROR) {
			empezar(reloj_parado);
		}
		return CAMBIO;
	}
	if ((e->pulsado & B_IZQ) || (e->toque_ini && en(e->tx, e->ty, 22, 120, 64, 44))) {
		saltar(-10000);
		return CAMBIO;
	}
	if ((e->pulsado & B_DER) || (e->toque_ini && en(e->tx, e->ty, 170, 120, 64, 44))) {
		saltar(10000);
		return CAMBIO;
	}
	if (e->toque_ini && dur > 0 && en(e->tx, e->ty, 8, 86, 240, 22)) {
		int x = e->tx - 12;
		x = x < 0 ? 0 : x > 232 ? 232 : x;
		empezar((uint32_t)((uint64_t)dur * x / 232));
		return CAMBIO;
	}
	return cambio ? CAMBIO : NADA;
}

// --- Dibujo ---

static void tiempo(char *s, int n, uint32_t ms)
{
	unsigned t = ms / 1000;
	if (t >= 3600)
		snprintf(s, n, "%u:%02u:%02u", t / 3600, t / 60 % 60, t % 60);
	else
		snprintf(s, n, "%u:%02u", t / 60, t % 60);
}

void video_dibujar(Color *arriba, Color *abajo)
{
	memcpy(arriba, imagen, sizeof imagen);
	bool cargando = fase == V_CONECTANDO || fase == V_PIDIENDO || (fase == V_VIENDO && !sonando && con_sonido) ||
		(fase == V_VIENDO && !con_sonido && !en_marcha);
	if (cargando || fase == V_ERROR || fase == V_PAUSA || fase == V_FIN) {
		const char *t = fase == V_ERROR ? "No se puede ver" : fase == V_PAUSA ? "En pausa" : fase == V_FIN ? "Se ha acabado"
			: fase == V_PIDIENDO ? "Preparando el vídeo…" : "Cargando…";
		int w = ancho_texto(&letra_negrita, t) + 24;
		redondeado(arriba, (ANCHO - w) / 2, 78, w, 30, 12, C_NEGRO, 200);
		texto_centro(arriba, &letra_negrita, ANCHO / 2, 85, t, fase == V_ERROR ? C_ROJO : C_BLANCO);
		if (aviso[0] && (fase == V_ERROR || fase == V_PAUSA)) {
			rect_alfa(arriba, 0, 150, ANCHO, 42, C_NEGRO, 200);
			texto_parrafo(arriba, &letra_peq, 10, 154, 236, aviso, C_SUAVE);
		}
	}
	if (!abajo)
		return;

	llenar(abajo, C_FONDO);
	barra_abajo(abajo, "Vídeo");
	texto_parrafo(abajo, &letra_negrita, 10, 32, 236, titulo, C_BLANCO);
	// la barra del tiempo
	uint32_t r = reloj();
	char a[16], b[16], s[40];
	tiempo(a, sizeof a, r);
	tiempo(b, sizeof b, dur);
	if (dur)
		snprintf(s, sizeof s, "%s / %s", a, b);
	else
		snprintf(s, sizeof s, "%s", a);
	texto(abajo, &letra_peq, 12, 72, s, C_SUAVE);
	redondeado(abajo, 12, 94, 232, 6, 3, C_PANEL2, 256);
	if (dur > 0) {
		int w = (int)((uint64_t)232 * (r > dur ? dur : r) / dur);
		if (w > 0)
			redondeado(abajo, 12, 94, w, 6, 3, C_ROJO, 256);
		circulo(abajo, 12 + w, 97, 6, C_BLANCO, 256);
	}
	// los botones: 10 s atrás, pausa, 10 s adelante
	redondeado(abajo, 22, 120, 64, 44, 12, C_PANEL, 256);
	triangulo(abajo, 40, 142, 52, 134, 52, 150, C_BLANCO);
	triangulo(abajo, 52, 142, 64, 134, 64, 150, C_BLANCO);
	texto_centro(abajo, &letra_peq, 54, 166, "10 s", C_GRIS);
	redondeado(abajo, 98, 120, 60, 44, 12, C_ROJO, 256);
	bool parado = fase != V_VIENDO && fase != V_CONECTANDO && fase != V_PIDIENDO;
	if (parado) {
		triangulo(abajo, 120, 130, 120, 154, 140, 142, C_BLANCO);
	} else {
		rect(abajo, 117, 131, 7, 22, C_BLANCO);
		rect(abajo, 132, 131, 7, 22, C_BLANCO);
	}
	redondeado(abajo, 170, 120, 64, 44, 12, C_PANEL, 256);
	triangulo(abajo, 192, 134, 204, 142, 192, 150, C_BLANCO);
	triangulo(abajo, 204, 134, 216, 142, 204, 150, C_BLANCO);
	texto_centro(abajo, &letra_peq, 202, 166, "10 s", C_GRIS);
	ayuda(abajo, fase == V_FIN ? "A otra vez     B salir" : "A pausa     ◄► 10 s     B salir");
}
