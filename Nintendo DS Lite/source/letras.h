// Las letras ya dibujadas (las hace letras/main.go en generado/letras.c).
#ifndef LETRAS_H
#define LETRAS_H

#include <stdint.h>

typedef struct {
	uint16_t cp;     // la letra (Unicode)
	int8_t x, y;     // dónde empieza la máscara, desde el punto de inicio y lo alto de la línea
	uint8_t w, h;    // tamaño de la máscara
	uint8_t avance;  // cuánto se mueve hacia la derecha después
	uint32_t pos;    // dónde empieza su máscara en alfa
} Glifo;

typedef struct {
	const Glifo *glifos; // ordenados por cp
	int n;
	const uint8_t *alfa;
	int alto;    // alto de una línea
	int ascenso; // de lo alto de la línea a la base de las letras
} Letra;

extern const Letra letra_peq, letra_normal, letra_negrita, letra_titulo, letra_grande, letra_reloj;

#endif
