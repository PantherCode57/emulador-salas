//go:build !linux

package main

import "errors"

// Fuera de la consola no hay nada que hacer (sirve para las pruebas en el PC).

var errSoloLectura = errors.New("solo lectura")

func tarjetaDeRaiz() (tarjeta, error)       { return tarjeta{}, errors.New("solo en la consola") }
func pararMenu()                            {}
func volverAlMenu()                         {}
func preparar(p tarjeta) (lector, error)    { return lector{}, errors.New("solo en la consola") }
func copiarA(t tarjeta, datos []byte) error { return errors.New("solo en la consola") }
func reiniciar()                            {}
func escucharBotones() <-chan struct{}      { return make(chan struct{}) }
