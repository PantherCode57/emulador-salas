//go:build !linux

package main

import "errors"

// Fuera de la consola no hay nada que hacer (sirve para las pruebas en el PC).

var rutaRegistro string

func guardarRegistro() {}

func tarjetaDeRaiz() (tarjeta, error)        { return tarjeta{}, errors.New("solo en la consola") }
func pararMenu()                             {}
func volverAlMenu()                          {}
func preparar(p tarjeta) (lector, error)     { return lector{}, errors.New("solo en la consola") }
func escribirCarga(t tarjeta, c carga) error { return errors.New("solo en la consola") }
func reiniciar()                             {}
func escucharBotones() <-chan tecla          { return make(chan tecla) }
