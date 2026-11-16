/* El sonido, con SDL (que en la consola va por ALSA).
 *
 * El juego va al ritmo de la pantalla (una imagen por refresco), así que el sonido no puede ir a su
 * ritmo exacto: se estira o se encoge un poquito (como mucho un 0,5 %, que no se oye) según lo lleno
 * que esté lo que queda por sonar. Así ni se corta ni se va retrasando.
 */
#include "panther.h"

static SDL_AudioDeviceID dispositivo;
static int frecuencia = 48000;
static bool pausado;

static int objetivo(void) { return frecuencia / 25; } /* unos 40 ms por sonar */

bool sonido_abrir(void) {
	if (SDL_InitSubSystem(SDL_INIT_AUDIO) != 0) {
		registro("sin sonido: %s", SDL_GetError());
		return false;
	}
	SDL_AudioSpec quiero, tengo;
	SDL_zero(quiero);
	quiero.freq = 48000;
	quiero.format = AUDIO_S16SYS;
	quiero.channels = 2;
	quiero.samples = 512;
	dispositivo = SDL_OpenAudioDevice(NULL, 0, &quiero, &tengo, SDL_AUDIO_ALLOW_FREQUENCY_CHANGE);
	if (!dispositivo) {
		registro("sin sonido: %s", SDL_GetError());
		return false;
	}
	frecuencia = tengo.freq;
	registro("sonido: %d Hz, %d muestras por bloque (%s)", tengo.freq, tengo.samples, SDL_GetCurrentAudioDriver());
	SDL_PauseAudioDevice(dispositivo, 0);
	return true;
}

int sonido_frecuencia(void) { return frecuencia; }

void sonido_meter(const int16_t *muestras, int marcos) {
	if (!dispositivo || pausado || marcos <= 0)
		return;
	Uint32 cola = SDL_GetQueuedAudioSize(dispositivo) / 4;
	if (cola > (Uint32)objetivo() * 4) {
		/* Se ha acumulado demasiado (después de ir rápido, por ejemplo): fuera, para no ir con retraso. */
		SDL_ClearQueuedAudio(dispositivo);
		cola = 0;
	}
	if (cola == 0) {
		/* Se había quedado vacío: un poco de silencio delante para que no se vuelva a cortar enseguida. */
		static int16_t silencio[4096 * 2];
		int n = objetivo() / 2;
		if (n > 4096)
			n = 4096;
		SDL_QueueAudio(dispositivo, silencio, (Uint32)n * 4);
	}
	SDL_QueueAudio(dispositivo, muestras, (Uint32)marcos * 4);
}

double sonido_ajuste(void) {
	if (!dispositivo || pausado)
		return 1.0;
	double cola = SDL_GetQueuedAudioSize(dispositivo) / 4.0;
	double d = (objetivo() - cola) / objetivo();
	if (d > 1)
		d = 1;
	if (d < -1)
		d = -1;
	return 1.0 + 0.005 * d;
}

void sonido_pausar(bool pausa) {
	if (!dispositivo || pausa == pausado)
		return;
	pausado = pausa;
	if (pausa)
		SDL_ClearQueuedAudio(dispositivo);
	SDL_PauseAudioDevice(dispositivo, pausa ? 1 : 0);
}

void sonido_vaciar(void) {
	if (dispositivo)
		SDL_ClearQueuedAudio(dispositivo);
}

void sonido_cerrar(void) {
	if (dispositivo) {
		SDL_CloseAudioDevice(dispositivo);
		dispositivo = 0;
	}
}
