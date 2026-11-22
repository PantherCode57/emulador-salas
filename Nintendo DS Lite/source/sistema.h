// Panther OS para la Nintendo DS Lite: lo que comparten todas las partes.
//
// Las dos pantallas son de 256×192 puntos con colores de 15 bits (como los usa la DS: rojo en los
// bits bajos y el bit 15 encendido). El sistema dibuja en dos memorias y la plataforma (la DS de
// verdad en plataforma_ds.c, o la prueba del PC en pc/prueba.c) las pasa a las pantallas.
#ifndef SISTEMA_H
#define SISTEMA_H

#include <stdbool.h>
#include <stdint.h>
#include <time.h>

#include "letras.h"

#define ANCHO 256
#define ALTO 192
#define VERSION "1"
#define PUERTO_PUENTE 8790

typedef uint16_t Color;

// Un color de 0..255 por canal a los 15 bits de la DS.
#define RGB(r, g, b) ((Color)((((r) >> 3) & 31) | ((((g) >> 3) & 31) << 5) | ((((b) >> 3) & 31) << 10) | 0x8000))

// Los colores de Panther OS (los mismos que en la consola).
#define C_NEGRO RGB(6, 6, 9)
#define C_FONDO RGB(11, 18, 32)
#define C_PANEL RGB(24, 32, 50)
#define C_PANEL2 RGB(36, 46, 70)
#define C_BLANCO RGB(255, 255, 255)
#define C_SUAVE RGB(200, 200, 212)
#define C_GRIS RGB(150, 150, 165)
#define C_OSCURO RGB(90, 96, 112)
#define C_VIOLETA RGB(196, 170, 255)
#define C_AZUL RGB(120, 200, 255)
#define C_VERDE RGB(30, 215, 96)
#define C_AMARILLO RGB(255, 200, 60)
#define C_ROJO RGB(240, 70, 80)

// Botones: los mismos bits que KEY_* de libnds, para que la DS los pase tal cual.
enum {
	B_A = 1 << 0,
	B_B = 1 << 1,
	B_SELECT = 1 << 2,
	B_START = 1 << 3,
	B_DER = 1 << 4,
	B_IZQ = 1 << 5,
	B_ARRIBA = 1 << 6,
	B_ABAJO = 1 << 7,
	B_R = 1 << 8,
	B_L = 1 << 9,
	B_X = 1 << 10,
	B_Y = 1 << 11,
	B_TOQUE = 1 << 12,
};

typedef struct {
	// Lo que pasa la plataforma:
	uint32_t pulsado;   // botones que se acaban de pulsar
	uint32_t mantenido; // botones que siguen pulsados
	uint32_t soltado;   // botones que se acaban de soltar
	uint32_t repetido;  // como pulsado, pero se repite si se deja pulsado (para las listas)
	int tx, ty;         // dónde se toca la pantalla de abajo (si mantenido tiene B_TOQUE)
	// Lo que añade el sistema:
	bool toque_ini;     // se acaba de tocar, en (tx, ty)
	bool toque;         // un toque corto (sin arrastrar) que se acaba de soltar, en (toque_x, toque_y)
	int toque_x, toque_y;
	bool arrastrando;   // se está tocando y el dedo ya se ha movido
	int arrastre_dy;    // cuánto se ha movido el dedo hacia abajo desde que tocó
} Entrada;

// --- Wifi y puente (los hace la plataforma) ---
// La DS Lite solo se conecta a redes abiertas o con WEP: con WPA/WPA2 no puede (es cosa de su wifi).
enum { SEG_ABIERTA, SEG_WEP, SEG_WPA };
enum { RED_APAGADA, RED_PARADA, RED_BUSCANDO, RED_CONECTANDO, RED_CONECTADA, RED_FALLO };

typedef struct {
	char ssid[33];
	int seguridad; // SEG_*
	int barras;    // 0..4
	bool guardada; // está en los ajustes de wifi de la DS (los de Nintendo WFC)
} Red;

void plat_wifi_buscar(void);                          // empieza a buscar redes
int plat_wifi_redes(Red *r, int max);                 // las que ha encontrado hasta ahora
void plat_wifi_conectar(const char *ssid, const uint8_t *clave, int nclave, bool guardada); // nclave 0 = abierta
void plat_wifi_guardadas(void);                       // conecta con las de los ajustes de la DS
int plat_wifi_n_guardadas(void);
int plat_wifi_estado(void);                           // RED_*
const char *plat_wifi_ip(void);                       // "192.168.1.40" o ""
// El puente es el programa (en la consola, el PC o el móvil) que hace el trabajo de las apps y le
// pasa a la DS lo justo para enseñarlo. Se le busca en la red por UDP, en el puerto PUERTO_PUENTE.
bool plat_puente(char *nombre, int n, char *ip, int nip);

// --- Más plataforma ---
uint32_t plat_ms(void);              // milisegundos desde que se encendió
void plat_hora(struct tm *t);        // la hora del reloj de la consola
int plat_bateria(void);              // 0..15, y +128 si está cargando
const char *plat_carpeta(void);      // la carpeta de Panther en la tarjeta, o NULL si no se puede

// --- El sistema (sistema.c) ---
void sistema_iniciar(void);
// Una imagen (1/60 s). Devuelve true si hay que pasar las pantallas otra vez.
bool sistema_paso(Entrada *e, Color *arriba, Color *abajo);
extern uint32_t ahora_ms;   // plat_ms() de esta imagen
extern struct tm hora;      // la hora de esta imagen
extern uint32_t imagenes;   // cuántas imágenes van
bool en(int x, int y, int x0, int y0, int w, int h); // (x, y) está en el rectángulo

// --- Dibujo (dibujo.c) ---
Color mezclar(Color a, Color b, int alfa); // alfa 0..256: 0 = a, 256 = b
void llenar(Color *p, Color c);
void rect(Color *p, int x, int y, int w, int h, Color c);
void rect_alfa(Color *p, int x, int y, int w, int h, Color c, int alfa);
void degradado(Color *p, int x, int y, int w, int h, Color c1, Color c2);
void redondeado(Color *p, int x, int y, int w, int h, int r, Color c, int alfa);
void redondeado_degradado(Color *p, int x, int y, int w, int h, int r, Color c1, Color c2);
void marco(Color *p, int x, int y, int w, int h, int r, int grosor, Color c);
void circulo(Color *p, float cx, float cy, float r, Color c, int alfa);
// hueco > 0: sin la parte de arriba de ese ancho (como el botón de apagar); hueco < 0: solo la mitad de arriba.
void aro(Color *p, float cx, float cy, float r, float grosor, Color c, float hueco);
void elipse(Color *p, float cx, float cy, float rx, float ry, float grosor, Color c);
void linea(Color *p, float x0, float y0, float x1, float y1, float grosor, Color c);
void triangulo(Color *p, float x0, float y0, float x1, float y1, float x2, float y2, Color c);

int texto(Color *p, const Letra *l, int x, int y, const char *s, Color c);
int texto_alfa(Color *p, const Letra *l, int x, int y, const char *s, Color c, int alfa);
int ancho_texto(const Letra *l, const char *s);
void texto_centro(Color *p, const Letra *l, int cx, int y, const char *s, Color c);
void texto_derecha(Color *p, const Letra *l, int xd, int y, const char *s, Color c);
void texto_corto(Color *p, const Letra *l, int x, int y, int max, const char *s, Color c); // con «…»
int texto_parrafo(Color *p, const Letra *l, int x, int y, int max, const char *s, Color c); // devuelve la y de abajo

// --- Piezas comunes (ui.c) ---
void barra_arriba(Color *p, const char *titulo);
void barra_abajo(Color *p, const char *titulo);
bool toca_atras(const Entrada *e);
void ayuda(Color *p, const char *s);
void boton(Color *p, int x, int y, int w, int h, const char *s, Color c1, Color c2, bool elegido);
void bateria(Color *p, int x, int y);
void icono_wifi(Color *p, int x, int y, Color c);
void fecha_larga(const struct tm *t, char *s, int n);
extern const char *const dias_semana[7];
extern const char *const meses[12];

// --- Apps (apps.c) ---
typedef struct {
	const char *nombre;
	const char *descripcion; // una línea, en la pantalla de inicio
	const char *que_hace;    // lo que hará cuando esté el puente
	Color c1, c2;            // los colores de su icono
	void (*icono)(Color *p, int cx, int cy);
} App;

#define N_APPS 6
extern const App apps[N_APPS];
void app_entrar(int i);
int app_paso(const Entrada *e); // SALIR, NADA o CAMBIO
void app_dibujar(Color *arriba, Color *abajo);
enum { SALIR = -1, NADA = 0, CAMBIO = 1, IR_WIFI = 2 };

// La pantalla del wifi (wifi.c).
void wifi_entrar(void);
int wifi_paso(const Entrada *e);
void wifi_dibujar(Color *arriba, Color *abajo);
void wifi_arrancar(void);   // al encender: se conecta con la red de la última vez
const char *wifi_resumen(void); // «Zona wifi · puente: buscando…»

// El teclado de la pantalla de abajo (teclado.c).
typedef struct {
	char texto[200];
	int fila, col;
	bool mayus, simbolos;
} Teclado;
enum { TECLADO_NADA, TECLADO_CAMBIO, TECLADO_ACEPTAR, TECLADO_CANCELAR };
void teclado_iniciar(Teclado *t, const char *inicial);
int teclado_paso(Teclado *t, const Entrada *e);
void teclado_dibujar(const Teclado *t, Color *abajo, const char *titulo);
void teclado_dibujar_texto(const Teclado *t, Color *p, int x, int y, int w, bool tapar); // la caja con lo escrito
// El código del puente, desde cualquier app (apps.c): 1 puesto, -1 cancelado, 2 cambio, 0 nada.
int codigo_paso(Teclado *t, const Entrada *e, char *aviso, int naviso);
void codigo_dibujar(const Teclado *t, Color *arriba, Color *abajo, const char *aviso);

// Lo que se guarda en la tarjeta (guardar.c).
bool wifi_leer(char *ssid, int nssid, char *clave, int nclave, bool *guardada);
bool wifi_guardar(const char *ssid, const char *clave, bool guardada);
bool puente_codigo_leer(char *c, int n);
bool puente_codigo_guardar(const char *c);

// --- Cifrado (cifrado.c): ChaCha20, como en el puente ---
typedef struct {
	uint8_t clave[32], nonce[12], bloque[64];
	uint32_t contador;
	int pos;
} Flujo;
void chacha_bloque(const uint8_t clave[32], uint32_t contador, const uint8_t nonce[12], uint8_t sal[64]);
void flujo_iniciar(Flujo *f, const uint8_t clave[32], uint8_t sentido);
void flujo_xor(Flujo *f, uint8_t *b, int n);
void clave_sesion(const char *codigo, const uint8_t nS[16], const uint8_t nD[16], uint8_t sal[32]);

// --- Conexión TCP (la plataforma) ---
int plat_tcp_abrir(const char *ip, int puerto);   // empieza a conectar; -1 si no se puede
int plat_tcp_listo(int s);                        // 1 conectado, 0 aún no, -1 no se ha podido
int plat_tcp_enviar(int s, const void *b, int n); // lo que se ha mandado (0 si ahora no cabe), -1 error
int plat_tcp_recibir(int s, void *b, int n);      // lo que ha llegado (0 si nada), -1 cerrado
void plat_tcp_cerrar(int s);
uint32_t plat_azar(void);                         // algo de azar (la hora, el barrido de la pantalla…)

// --- El sonido (la plataforma): la DS va tocando un anillo de muestras de 16 bits, una y otra vez ---
#define AUDIO_ANILLO 32768                   // muestras (unos 2 s)
int plat_audio_tasa(void);                   // muestras por segundo de verdad (16 005 en la DS)
void plat_audio_empezar(const int16_t *anillo); // lo toca desde el principio
void plat_audio_parar(void);
uint32_t plat_audio_tocadas(void);           // cuántas ha tocado desde que empezó
void plat_audio_escrito(const int16_t *p, int n); // que la DS vea lo que se acaba de escribir

// --- El puente (puente.c) ---
// Una conexión con el puente, con su saludo (la de las peticiones y la de cada vídeo).
enum { CANAL_CERRADO, CANAL_CONECTANDO, CANAL_SALUDO, CANAL_BIEN, CANAL_LISTO };
typedef struct {
	int sock, fase;
	Flujo ent, sal;
	char crudo[160];
	int ncrudo;
	uint32_t desde;
	char nombre[48];
} Canal;
bool canal_abrir(Canal *c, const char *ip);
int canal_paso(Canal *c, const char *codigo); // 1 listo, 0 aún no, -1 no se puede, -2 código malo
bool canal_enviar(Canal *c, const char *linea);
int canal_recibir(Canal *c, uint8_t *b, int n); // lo que ha llegado, descifrado (0 nada, -1 cerrado)
void canal_cerrar(Canal *c);

enum { PU_SIN_RED, PU_BUSCANDO, PU_CONECTANDO, PU_SALUDO, PU_SIN_CODIGO, PU_CODIGO_MAL, PU_LISTO, PU_ERROR };
void puente_iniciar(void);
void puente_paso(void);
int puente_estado(void);
const char *puente_nombre(void);
const char *puente_ip(void);
const char *puente_codigo(void);
void puente_poner_codigo(const char *codigo);
bool puente_libre(void);                // conectado y sin nada pendiente
bool puente_pedir(const char *linea);   // una petición (si está libre)
int puente_respuesta(char ***lineas);   // las líneas de la respuesta cuando llega; -1 si aún no
int puente_datos(const uint8_t **d);    // si la respuesta era una foto («B <n>»), sus bytes; -1 si no
void puente_olvidar(void);              // lo que esté por llegar ya no hace falta (al cambiar de app)
int partir(char *s, char **trozos, int max); // corta por los tabuladores

// --- Fotos en JPEG (jpeg.c) ---
bool jpeg_tam(const uint8_t *d, int n, int *w, int *h);
// La pinta con su esquina en (x0, y0) en p (de pitch × alto); lo que se salga no se pinta.
bool jpeg_pintar(const uint8_t *d, int n, Color *p, int pitch, int alto, int x0, int y0);

// --- Los vídeos (video.c): YouTube y los de Instagram, por otra conexión con el puente ---
void video_abrir(const char *clave, const char *titulo, uint32_t desde_ms);
void video_cerrar(void);
int video_paso(const Entrada *e);              // SALIR cuando se cierra; e == NULL: sin mandos
void video_dibujar(Color *arriba, Color *abajo); // abajo == NULL: solo la imagen
bool video_acabado(void);                      // ha llegado al final
int video_imagenes(void);                      // (para la prueba) cuántas se han enseñado
uint32_t video_muestras(void);                 // (para la prueba) cuánto sonido ha llegado

// --- Instagram (ig.c) y YouTube (yt.c) ---
void ig_entrar(void);
int ig_paso(const Entrada *e);
void ig_dibujar(Color *arriba, Color *abajo);
int ig_n_posts(void);                // (para la prueba)
int ig_n_chats(void);                // (para la prueba)
const char *ig_ultimo_texto(void);   // (para la prueba)
bool ig_foto_puesta(void);           // (para la prueba) la foto de la publicación está pintada
void yt_entrar(void);
int yt_paso(const Entrada *e);
void yt_dibujar(Color *arriba, Color *abajo);
int yt_n_videos(void);               // (para la prueba)
bool yt_mini_puesta(void);           // (para la prueba)

// --- WhatsApp (wa.c) y las filas de estado de las apps (apps.c) ---
void wa_entrar(void);
int wa_paso(const Entrada *e);
void wa_dibujar(Color *arriba, Color *abajo);
int wa_n_chats(void);                // (para la prueba)
const char *wa_ultimo_texto(void);   // (para la prueba) el último mensaje del chat abierto
void fila_estado(Color *p, int y, int estado, const char *titulo, const char *detalle); // 0 hecho, 1 esperando, 2 falta
int estado_wifi(char *detalle, int n);
int estado_puente(char *detalle, int n);

// Una conversación (WhatsApp e Instagram): burbujas de abajo arriba entre y0 e y1.
typedef struct {
	bool mio;
	char hora[10], de[44], texto[408];
} MsgDS;
void conversacion(Color *p, const MsgDS *m, int n, int *desplazar, int y0, int y1, Color mia, const char *vacio);

#endif
