/* Lo que la app (Java) le dice al emulador, en vez de las órdenes por la red de la consola:
 *   SCREENSHOT   guarda una captura del juego en la carpeta de capturas (para las salas)
 *   QUIT         guarda dónde ibas y sale
 */
#include "movil.h"

#include <jni.h>
#include <pthread.h>
#include <string.h>

#define CUANTAS 8
#define LARGO 64
static pthread_mutex_t cerrojo_ordenes = PTHREAD_MUTEX_INITIALIZER;
static char cola[CUANTAS][LARGO];
static int n_cola;

JNIEXPORT void JNICALL Java_com_pokemulador_panther_Emulador_orden(JNIEnv *env, jclass clase, jstring texto) {
	(void)clase;
	const char *s = (*env)->GetStringUTFChars(env, texto, NULL);
	if (!s)
		return;
	pthread_mutex_lock(&cerrojo_ordenes);
	if (n_cola < CUANTAS) {
		strncpy(cola[n_cola], s, LARGO - 1);
		cola[n_cola][LARGO - 1] = 0;
		n_cola++;
	}
	pthread_mutex_unlock(&cerrojo_ordenes);
	(*env)->ReleaseStringUTFChars(env, texto, s);
}

JNIEXPORT jint JNICALL Java_com_pokemulador_panther_Emulador_resultado(JNIEnv *env, jclass clase) {
	(void)env, (void)clase;
	return resultado_juego;
}

void ordenes_abrir(void) {}
void ordenes_cerrar(void) {}

const char *ordenes_leer(void) {
	static char una[LARGO];
	bool hay = false;
	pthread_mutex_lock(&cerrojo_ordenes);
	if (n_cola > 0) {
		memcpy(una, cola[0], LARGO);
		n_cola--;
		memmove(cola[0], cola[1], (size_t)n_cola * LARGO);
		hay = true;
	}
	pthread_mutex_unlock(&cerrojo_ordenes);
	return hay ? una : NULL;
}
