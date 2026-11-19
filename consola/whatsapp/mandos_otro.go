//go:build !linux

package main

import "errors"

type mando struct{ cambios chan uint32 }

const (
	bL2 = 1 << 12
	bR2 = 1 << 13
	bL3 = 1 << 14
	bR3 = 1 << 15
	bF  = 1 << 16
)

func abrirMando() (*mando, error)             { return nil, errors.New("solo en la consola") }
func abrirTeclasVolumen() (<-chan int, error) { return nil, errors.New("solo en la consola") }
func (m *mando) ahora() uint32                { return 0 }
func (m *mando) agarrar(si bool)              {}
