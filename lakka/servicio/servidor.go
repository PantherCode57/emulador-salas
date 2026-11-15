package main

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
)

// Conversación con el núcleo "Pokemulador" (lakka/nucleo), por 127.0.0.1:24890:
//   núcleo -> servicio: 'I' + 2 bytes (botones del mando, little endian, bits de RetroPad)
//   servicio -> núcleo: 'F' + ancho (2 bytes) + alto (2 bytes) + píxeles XRGB8888
const direccionPantalla = "127.0.0.1:24890"

type servidorPantalla struct {
	botones chan<- uint16
	mu      sync.Mutex
	ultimo  []byte
	nuevo   *sync.Cond
	version int
}

func nuevoServidorPantalla(botones chan<- uint16) *servidorPantalla {
	s := &servidorPantalla{botones: botones}
	s.nuevo = sync.NewCond(&s.mu)
	return s
}

func (s *servidorPantalla) escuchar() error {
	ln, err := net.Listen("tcp", direccionPantalla)
	if err != nil {
		return err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				log.Printf("pantalla: %v", err)
				return
			}
			go s.atender(c)
		}
	}()
	return nil
}

// enseñar guarda el marco nuevo; el que esté conectado lo recibe en cuanto pueda.
func (s *servidorPantalla) enseñar(pixeles []byte) {
	cab := make([]byte, 5, 5+len(pixeles))
	cab[0] = 'F'
	binary.LittleEndian.PutUint16(cab[1:], uint16(ancho))
	binary.LittleEndian.PutUint16(cab[3:], uint16(alto))
	s.mu.Lock()
	s.ultimo = append(cab, pixeles...)
	s.version++
	s.mu.Unlock()
	s.nuevo.Broadcast()
}

func (s *servidorPantalla) atender(c net.Conn) {
	defer c.Close()
	if t, ok := c.(*net.TCPConn); ok {
		_ = t.SetNoDelay(true)
	}
	cerrado := make(chan struct{})
	// Escritor: manda el último marco cada vez que cambia.
	go func() {
		visto := -1
		for {
			s.mu.Lock()
			for s.version == visto {
				select {
				case <-cerrado:
					s.mu.Unlock()
					return
				default:
				}
				s.nuevo.Wait()
			}
			marco, v := s.ultimo, s.version
			s.mu.Unlock()
			visto = v
			if marco == nil {
				continue
			}
			if _, err := c.Write(marco); err != nil {
				return
			}
		}
	}()
	defer func() {
		close(cerrado)
		s.nuevo.Broadcast()
	}()
	// Lector: botones.
	buf := make([]byte, 3)
	for {
		if _, err := io.ReadFull(c, buf); err != nil {
			return
		}
		if buf[0] == 'I' {
			s.botones <- binary.LittleEndian.Uint16(buf[1:])
		}
	}
}
