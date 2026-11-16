//go:build !linux

package main

import (
	"image"
	"time"
)

// Fuera de la consola no hay nadie que pinte la capa: no hace nada (sirve para probar en el PC).
type capaCompartida struct{}

func abrirCapa(ruta string, maxAncho, maxAlto int) (*capaCompartida, error) { return nil, nil }

func (c *capaCompartida) poner(modo int, img *image.RGBA, x, y int, dura time.Duration) {}
func (c *capaCompartida) latido()                                                     {}
func (c *capaCompartida) vigilar()                                                    {}
func hayQuienPinte() bool                                                             { return false }
