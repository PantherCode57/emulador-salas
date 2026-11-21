package main

import "encoding/binary"

// ChaCha20 (RFC 8439), escrito a mano igual que en la DS («Nintendo DS Lite/source/cifrado.c»),
// para que lo que va por el wifi abierto no se pueda leer.
//
// Al conectarse, el puente y la DS se mandan 16 bytes al azar cada uno (nS y nD). La clave de la
// sesión son los 32 primeros bytes del bloque 0 de ChaCha20 con:
//   - clave: «PANTHER-DS-1:» + el código de 6 cifras, rellenado con ceros hasta 32 bytes;
//   - nonce: los 6 primeros bytes de nS y los 6 primeros de nD.
// Luego cada sentido va cifrado con esa clave: de la DS al puente con el nonce 01 00…, y del puente
// a la DS con el nonce 02 00…, con el contador desde 0. Lo primero que manda la DS, ya cifrado, es
// «PIN-OK»: si no se lee así, el código no es el bueno.

func rotl(v uint32, n uint) uint32 { return v<<n | v>>(32-n) }

func cuarto(s *[16]uint32, a, b, c, d int) {
	s[a] += s[b]
	s[d] = rotl(s[d]^s[a], 16)
	s[c] += s[d]
	s[b] = rotl(s[b]^s[c], 12)
	s[a] += s[b]
	s[d] = rotl(s[d]^s[a], 8)
	s[c] += s[d]
	s[b] = rotl(s[b]^s[c], 7)
}

func bloqueChaCha(clave *[32]byte, contador uint32, nonce *[12]byte) [64]byte {
	var s, x [16]uint32
	s[0], s[1], s[2], s[3] = 0x61707865, 0x3320646e, 0x79622d32, 0x6b206574
	for i := 0; i < 8; i++ {
		s[4+i] = binary.LittleEndian.Uint32(clave[i*4:])
	}
	s[12] = contador
	for i := 0; i < 3; i++ {
		s[13+i] = binary.LittleEndian.Uint32(nonce[i*4:])
	}
	x = s
	for i := 0; i < 10; i++ {
		cuarto(&x, 0, 4, 8, 12)
		cuarto(&x, 1, 5, 9, 13)
		cuarto(&x, 2, 6, 10, 14)
		cuarto(&x, 3, 7, 11, 15)
		cuarto(&x, 0, 5, 10, 15)
		cuarto(&x, 1, 6, 11, 12)
		cuarto(&x, 2, 7, 8, 13)
		cuarto(&x, 3, 4, 9, 14)
	}
	var out [64]byte
	for i := 0; i < 16; i++ {
		binary.LittleEndian.PutUint32(out[i*4:], x[i]+s[i])
	}
	return out
}

// flujo: un sentido de la conexión, cifrado (o descifrado: es lo mismo) byte a byte.
type flujo struct {
	clave    [32]byte
	nonce    [12]byte
	contador uint32
	bloque   [64]byte
	pos      int
}

func nuevoFlujo(clave [32]byte, sentido byte) *flujo {
	f := &flujo{clave: clave, pos: 64}
	f.nonce[0] = sentido
	return f
}

func (f *flujo) xor(b []byte) {
	for i := range b {
		if f.pos == 64 {
			f.bloque = bloqueChaCha(&f.clave, f.contador, &f.nonce)
			f.contador++
			f.pos = 0
		}
		b[i] ^= f.bloque[f.pos]
		f.pos++
	}
}

func claveSesion(codigo string, nS, nD []byte) [32]byte {
	var k [32]byte
	copy(k[:], "PANTHER-DS-1:"+codigo)
	var n [12]byte
	copy(n[0:6], nS[:6])
	copy(n[6:12], nD[:6])
	b := bloqueChaCha(&k, 0, &n)
	var s [32]byte
	copy(s[:], b[:32])
	return s
}

const (
	sentidoDS     = 1 // de la DS al puente
	sentidoPuente = 2 // del puente a la DS
)
