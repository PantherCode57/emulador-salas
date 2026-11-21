// La Nintendo DS de verdad (BlocksDS): las dos pantallas, los botones, el reloj, la tarjeta de la
// flashcard y el wifi. En el PC no se compila: allí está pc/prueba.c.
#ifdef __NDS__

#include <arpa/inet.h>
#include <netinet/in.h>
#include <stdio.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <time.h>

#include <dswifi9.h>
#include <fat.h>
#include <nds.h>

#include "sistema.h"

static Color arriba[ANCHO * ALTO] __attribute__((aligned(32)));
static Color abajo[ANCHO * ALTO] __attribute__((aligned(32)));
static char carpeta[48];
static volatile uint32_t vblanks;

static void contar_vblank(void)
{
	vblanks++;
}

uint32_t plat_ms(void)
{
	return (uint32_t)((uint64_t)vblanks * 100000 / 5983); // la DS va a 59,83 imágenes por segundo
}

void plat_hora(struct tm *t)
{
	time_t ahora = time(NULL); // el reloj de la DS ya guarda la hora de aquí
	gmtime_r(&ahora, t);
}

int plat_bateria(void)
{
	return (int)(getBatteryLevel() & (BATTERY_LEVEL_MASK | BATTERY_CHARGER_CONNECTED));
}

const char *plat_carpeta(void)
{
	return carpeta[0] ? carpeta : NULL;
}

// --- Wifi ---

static bool wifi_ok;
static int estado = RED_APAGADA;
static char ip_texto[20];

void plat_wifi_buscar(void)
{
	if (!wifi_ok)
		return;
	Wifi_ScanMode();
	estado = RED_BUSCANDO;
}

static int barras_de(int rssi)
{
	if (rssi < 0) { // en la DSi viene en dBm
		return rssi > -55 ? 4 : rssi > -67 ? 3 : rssi > -78 ? 2 : 1;
	}
	return rssi >= 40 ? 4 : rssi >= 28 ? 3 : rssi >= 16 ? 2 : rssi > 0 ? 1 : 0;
}

int plat_wifi_redes(Red *r, int max)
{
	if (!wifi_ok)
		return 0;
	int n = Wifi_GetNumAP(), k = 0;
	for (int i = 0; i < n && k < max; i++) {
		Wifi_AccessPoint ap;
		if (Wifi_GetAPData(i, &ap) != WIFI_RETURN_OK)
			continue;
		if (ap.flags & WFLAG_APDATA_NINTENDO_TAG)
			continue; // otra DS jugando en local, no es un wifi
		Red *d = &r[k++];
		int l = ap.ssid_len > 32 ? 32 : ap.ssid_len;
		memcpy(d->ssid, ap.ssid, l);
		d->ssid[l] = 0;
		d->seguridad = ap.security_type == AP_SECURITY_OPEN ? SEG_ABIERTA : ap.security_type == AP_SECURITY_WEP ? SEG_WEP : SEG_WPA;
		if (!(ap.flags & WFLAG_APDATA_COMPATIBLE))
			d->seguridad = SEG_WPA;
		d->barras = barras_de(ap.rssi);
		d->guardada = (ap.flags & WFLAG_APDATA_CONFIG_IN_WFC) != 0;
	}
	// Las mejores primero.
	for (int i = 1; i < k; i++) {
		Red t = r[i];
		int j = i;
		int pt = (t.seguridad != SEG_WPA) * 10 + t.barras;
		while (j > 0 && (r[j - 1].seguridad != SEG_WPA) * 10 + r[j - 1].barras < pt) {
			r[j] = r[j - 1];
			j--;
		}
		r[j] = t;
	}
	return k;
}

void plat_wifi_conectar(const char *ssid, const uint8_t *clave, int nclave, bool guardada)
{
	if (!wifi_ok)
		return;
	static Wifi_AccessPoint ap;
	memset(&ap, 0, sizeof ap);
	int l = (int)strlen(ssid);
	if (l > 32)
		l = 32;
	memcpy(ap.ssid, ssid, l);
	ap.ssid_len = (u8)l;
	Wifi_SetIP(0, 0, 0, 0, 0); // la IP la da el router (DHCP)
	if (guardada)
		Wifi_ConnectWfcAP(&ap);
	else
		Wifi_ConnectSecureAP(&ap, nclave ? clave : NULL, (size_t)nclave);
	estado = RED_CONECTANDO;
}

void plat_wifi_guardadas(void)
{
	if (!wifi_ok)
		return;
	Wifi_AutoConnect();
	estado = RED_CONECTANDO;
}

int plat_wifi_n_guardadas(void)
{
	if (!wifi_ok)
		return 0;
	int n = Wifi_GetData(WIFIGETDATA_NUMWFCAPS, 0, NULL);
	return n > 0 ? n : 0;
}

int plat_wifi_estado(void)
{
	return estado;
}

const char *plat_wifi_ip(void)
{
	return estado == RED_CONECTADA ? ip_texto : "";
}

static void wifi_paso(void)
{
	if (!wifi_ok)
		return;
	if (estado == RED_CONECTANDO || estado == RED_CONECTADA) {
		int s = Wifi_AssocStatus();
		if (s == ASSOCSTATUS_ASSOCIATED) {
			if (estado != RED_CONECTADA) {
				struct in_addr ip;
				ip.s_addr = Wifi_GetIP();
				snprintf(ip_texto, sizeof ip_texto, "%s", inet_ntoa(ip));
			}
			estado = RED_CONECTADA;
		} else if (s == ASSOCSTATUS_CANNOTCONNECT) {
			estado = RED_FALLO;
		} else if (estado == RED_CONECTADA && s == ASSOCSTATUS_DISCONNECTED) {
			estado = RED_FALLO;
		}
	}
}

// --- El puente: se le pregunta por UDP a toda la red y contesta con su nombre ---

static int sock = -1;
static uint32_t ultimo_envio;
static bool puente_visto;
static char puente_nombre[40], puente_ip[20];

static void puente_paso(void)
{
	if (estado != RED_CONECTADA) {
		if (sock >= 0) {
			closesocket(sock);
			sock = -1;
		}
		puente_visto = false;
		return;
	}
	if (sock < 0) {
		sock = socket(AF_INET, SOCK_DGRAM, 0);
		if (sock < 0)
			return;
		int si = 1;
		setsockopt(sock, SOL_SOCKET, SO_BROADCAST, &si, sizeof si);
		ioctl(sock, FIONBIO, (char *)&si);
		struct sockaddr_in yo = {0};
		yo.sin_family = AF_INET;
		yo.sin_port = htons(0);
		yo.sin_addr.s_addr = htonl(INADDR_ANY);
		bind(sock, (struct sockaddr *)&yo, sizeof yo);
		ultimo_envio = 0;
	}
	uint32_t ahora = plat_ms();
	if (!puente_visto && (ultimo_envio == 0 || ahora - ultimo_envio > 2000)) {
		ultimo_envio = ahora ? ahora : 1;
		const char *pregunta = "PANTHER-DS " VERSION "\n";
		struct in_addr gw, mascara, d1, d2;
		struct in_addr ip = Wifi_GetIPInfo(&gw, &mascara, &d1, &d2);
		struct sockaddr_in a = {0};
		a.sin_family = AF_INET;
		a.sin_port = htons(PUERTO_PUENTE);
		a.sin_addr.s_addr = ip.s_addr | ~mascara.s_addr; // la dirección de toda la red
		sendto(sock, pregunta, strlen(pregunta), 0, (struct sockaddr *)&a, sizeof a);
		a.sin_addr.s_addr = htonl(INADDR_BROADCAST);
		sendto(sock, pregunta, strlen(pregunta), 0, (struct sockaddr *)&a, sizeof a);
	}
	char buf[96];
	struct sockaddr_in de;
	socklen_t l = sizeof de;
	int n = recvfrom(sock, buf, sizeof buf - 1, 0, (struct sockaddr *)&de, &l);
	if (n > 0) {
		buf[n] = 0;
		// «PANTHER-PUENTE 1 Consola XF35H»
		if (!strncmp(buf, "PANTHER-PUENTE ", 15)) {
			const char *nombre = strchr(buf + 15, ' ');
			snprintf(puente_nombre, sizeof puente_nombre, "%s", nombre ? nombre + 1 : "Puente");
			char *fin = strchr(puente_nombre, '\n');
			if (fin)
				*fin = 0;
			snprintf(puente_ip, sizeof puente_ip, "%s", inet_ntoa(de.sin_addr));
			puente_visto = true;
		}
	}
}

bool plat_puente(char *nombre, int n, char *ip, int nip)
{
	if (!puente_visto)
		return false;
	snprintf(nombre, n, "%s", puente_nombre);
	snprintf(ip, nip, "%s", puente_ip);
	return true;
}

// --- El programa ---

int main(int argc, char **argv)
{
	(void)argc;
	(void)argv;
	irqSet(IRQ_VBLANK, contar_vblank);
	irqEnable(IRQ_VBLANK);

	videoSetMode(MODE_5_2D);
	videoSetModeSub(MODE_5_2D);
	vramSetBankA(VRAM_A_MAIN_BG);
	vramSetBankC(VRAM_C_SUB_BG);
	int fondo_arriba = bgInit(3, BgType_Bmp16, BgSize_B16_256x256, 0, 0);
	int fondo_abajo = bgInitSub(3, BgType_Bmp16, BgSize_B16_256x256, 0, 0);
	Color *vram_arriba = bgGetGfxPtr(fondo_arriba);
	Color *vram_abajo = bgGetGfxPtr(fondo_abajo);
	lcdMainOnTop();
	keysSetRepeat(20, 5);

	// La tarjeta: lo que se apunta va en panther/ (al lado de donde esté la flashcard).
	if (fatInitDefault()) {
		snprintf(carpeta, sizeof carpeta, "%spanther", fatGetDefaultDrive());
		mkdir(carpeta, 0777);
	}

	wifi_ok = Wifi_InitDefault(INIT_ONLY | WIFI_ATTEMPT_DSI_MODE);
	estado = wifi_ok ? RED_PARADA : RED_APAGADA;

	sistema_iniciar();
	while (1) {
		scanKeys();
		Entrada e;
		memset(&e, 0, sizeof e);
		e.pulsado = keysDown();
		e.mantenido = keysHeld();
		e.soltado = keysUp();
		e.repetido = keysDownRepeat();
		if (e.mantenido & KEY_TOUCH) {
			touchPosition t;
			touchRead(&t);
			e.tx = t.px;
			e.ty = t.py;
		}
		wifi_paso();
		puente_paso();
		bool cambio = sistema_paso(&e, arriba, abajo);
		cothread_yield_irq(IRQ_VBLANK);
		if (cambio) {
			DC_FlushRange(arriba, sizeof arriba);
			DC_FlushRange(abajo, sizeof abajo);
			dmaCopy(arriba, vram_arriba, sizeof arriba);
			dmaCopy(abajo, vram_abajo, sizeof abajo);
		}
	}
}

#endif
