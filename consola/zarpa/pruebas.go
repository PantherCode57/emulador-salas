package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// medirDibujo: 300 imágenes de una partida (moviendo y dibujando, como en la consola) y lo que
// tarda cada una: tiene que bajar de 33 ms para ir a 30 imágenes por segundo.
func medirDibujo(f *fuentes) {
	t0 := time.Now()
	a := nuevaApp(f, &progreso{Traje: "noche", Trajes: []string{"noche"}})
	fmt.Printf("preparar los dibujos: %.1f s\n", time.Since(t0).Seconds())
	a.son.callado = true
	a.empezar()
	buf := make([]byte, ancho*alto*4)
	var total, peor time.Duration
	for i := 0; i < 300; i++ {
		if i%20 == 0 {
			a.c.mover([]int{-1, 1}[i/20%2])
		}
		for _, o := range a.c.objs { // que no choque (como al jugar bien): fuera lo que tenga delante
			if o.tipo <= oCamion && o.carril == a.c.carril && o.s-a.c.recorrido < 2 {
				o.cogido = true
			}
		}
		t := time.Now()
		a.paso(1.0 / 30)
		a.dibujar()
		a.l.marcoEn(buf)
		d := time.Since(t)
		total += d
		peor = max(peor, d)
	}
	fmt.Printf("cada imagen: %.1f ms de media, la peor %.1f ms\n", float64(total.Microseconds())/300/1000, float64(peor.Microseconds())/1000)
}

// dibujarPruebas: las pantallas de ejemplo en PNG (sin consola ni sonido).
func dibujarPruebas(f *fuentes, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pr := &progreso{Record: 12480, RecordM: 1834, Estrellas: 640, Traje: "noche", Trajes: []string{"noche", "playa"}}
	a := nuevaApp(f, pr)
	a.son.callado = true
	guardar := func(nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}
	for i := 0; i < 20; i++ {
		a.paso(1.0 / 30)
	}
	if err := guardar("1-titulo"); err != nil {
		return err
	}

	// una partida: se adelanta hasta que haya cosas delante
	a.empezar()
	c := a.c
	c.rng.Seed(42)
	for i := 0; i < 160 && c.viva; i++ {
		c.carril = 0
		c.paso(1.0 / 30)
		// que no choque: se quita lo que tenga justo delante en su carril
		for _, o := range c.objs {
			if o.tipo <= oCamion && o.carril == 0 && o.s-c.recorrido < 3 {
				o.cogido = true
			}
		}
	}
	c.estrellas = 37
	if err := guardar("2-corriendo"); err != nil {
		return err
	}
	c.carril, c.carrilX = -1, -0.5
	c.saltar()
	for i := 0; i < 6; i++ {
		c.paso(1.0 / 30)
	}
	if err := guardar("3-saltando"); err != nil {
		return err
	}
	for i := 0; i < 30; i++ {
		c.paso(1.0 / 30)
	}
	c.carril, c.carrilX = 1, 1
	c.agacharse()
	c.paso(1.0 / 30)
	c.iman, c.escudo = 5, 9
	if err := guardar("4-agachada-con-poderes"); err != nil {
		return err
	}
	// el choque y el final
	c.iman, c.escudo = 0, 0
	c.poner(oCamion, c.carril, c.recorrido+0.2, 0)
	for i := 0; i < 3; i++ {
		a.paso(1.0 / 30)
	}
	if err := guardar("5-choque"); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		a.paso(1.0 / 30)
	}
	a.nuevoRecord = true
	if err := guardar("6-fin"); err != nil {
		return err
	}
	a.pant = pPausa
	if err := guardar("7-pausa"); err != nil {
		return err
	}
	a.pant, a.selTienda = pTienda, 2
	if err := guardar("8-tienda"); err != nil {
		return err
	}
	a.selTienda = 4
	return guardar("9-tienda-neon")
}
