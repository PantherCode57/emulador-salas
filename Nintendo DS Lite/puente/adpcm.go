package main

// IMA ADPCM: 4 bits por muestra en vez de 16, para que el sonido de los vídeos quepa en el wifi de
// la DS (16 000 muestras por segundo son 8 KB/s). Cada paquete lleva delante dónde estaba el
// descodificador (la muestra y el índice), así la DS puede empezar por cualquiera.

var pasosADPCM = [89]int32{7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31, 34, 37, 41, 45, 50, 55, 60, 66,
	73, 80, 88, 97, 107, 118, 130, 143, 157, 173, 190, 209, 230, 253, 279, 307, 337, 371, 408, 449, 494, 544, 598, 658,
	724, 796, 876, 963, 1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066, 2272, 2499, 2749, 3024, 3327, 3660, 4026, 4428,
	4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635, 13899, 15289, 16818, 18500, 20350, 22385,
	24623, 27086, 29794, 32767}

var indicesADPCM = [16]int32{-1, -1, -1, -1, 2, 4, 6, 8, -1, -1, -1, -1, 2, 4, 6, 8}

type estadoADPCM struct {
	pred, idx int32
}

// paso: lo que hace el descodificador con un código (el codificador hace lo mismo para no
// separarse de él).
func (e *estadoADPCM) paso(n byte) int16 {
	paso := pasosADPCM[e.idx]
	dif := paso >> 3
	if n&4 != 0 {
		dif += paso
	}
	if n&2 != 0 {
		dif += paso >> 1
	}
	if n&1 != 0 {
		dif += paso >> 2
	}
	if n&8 != 0 {
		e.pred -= dif
	} else {
		e.pred += dif
	}
	e.pred = min(32767, max(-32768, e.pred))
	e.idx = min(88, max(0, e.idx+indicesADPCM[n&15]))
	return int16(e.pred)
}

func (e *estadoADPCM) codigo(m int16) byte {
	dif := int32(m) - e.pred
	var n byte
	if dif < 0 {
		n = 8
		dif = -dif
	}
	paso := pasosADPCM[e.idx]
	if dif >= paso {
		n |= 4
		dif -= paso
	}
	paso >>= 1
	if dif >= paso {
		n |= 2
		dif -= paso
	}
	paso >>= 1
	if dif >= paso {
		n |= 1
	}
	e.paso(n)
	return n
}

// codificar: un paquete con esas muestras (tienen que ser pares).
func (e *estadoADPCM) codificar(m []int16) []byte {
	out := make([]byte, 4+len(m)/2)
	out[0], out[1], out[2] = byte(uint16(e.pred)), byte(uint16(e.pred)>>8), byte(e.idx)
	for i, s := range m {
		n := e.codigo(s)
		if i&1 == 0 {
			out[4+i/2] = n
		} else {
			out[4+i/2] |= n << 4
		}
	}
	return out
}

// descodificarADPCM: lo que hace la DS (para las pruebas).
func descodificarADPCM(b []byte) []int16 {
	if len(b) < 4 {
		return nil
	}
	e := estadoADPCM{pred: int32(int16(uint16(b[0]) | uint16(b[1])<<8)), idx: min(88, int32(b[2]))}
	out := make([]int16, 0, (len(b)-4)*2)
	for _, c := range b[4:] {
		out = append(out, e.paso(c&15), e.paso(c>>4))
	}
	return out
}
