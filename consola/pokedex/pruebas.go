package main

import (
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas: las pantallas con una colección de ejemplo, en PNG (para verlas sin consola).
func dibujarPruebas(f *fuentes, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "pokedex-prueba")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	guardar := func(a *app, nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}
	pulsar := func(a *app, bs ...uint16) {
		for _, b := range bs {
			a.pulsar(b)
		}
	}

	col := abrirColeccion(tmp)
	for _, n := range []int{1, 4, 7, 25, 26, 29, 32, 133, 150, 151} {
		col.cambiar(n)
	}
	a := nuevaApp(f, col)
	a.cargaHasta = time.Now().Add(time.Hour)
	if err := guardar(a, "1-carga"); err != nil {
		return err
	}
	a.cargaHasta = time.Time{}
	if err := guardar(a, "2-inicio"); err != nil {
		return err
	}
	pulsar(a, bAbajo)
	if err := guardar(a, "3-inicio-juegos"); err != nil {
		return err
	}
	pulsar(a, bA)
	if err := guardar(a, "4-juegos"); err != nil {
		return err
	}
	pulsar(a, bB, bArriba, bA)
	a.pulsadoDesde = time.Now().Add(-time.Second) // como si se mantuviera pulsado (sin dar la vuelta)
	for i := 0; i < 24; i++ {
		a.mover(1)
	}
	if err := guardar(a, "5-cartas"); err != nil {
		return err
	}
	pulsar(a, bA) // Pikachu (25) ya estaba: se quita
	pulsar(a, bR, bR)
	if err := guardar(a, "6-cartas-generacion-3"); err != nil {
		return err
	}
	pulsar(a, bY)
	for _, r := range "pika" {
		a.tec.escribir(r)
	}
	a.previa = buscar(a.tec.texto())
	if err := guardar(a, "7-buscar"); err != nil {
		return err
	}
	pulsar(a, bStart)
	if err := guardar(a, "8-lo-buscado"); err != nil {
		return err
	}
	pulsar(a, bY, bX)
	for _, r := range "25" {
		a.tec.escribir(r)
	}
	pulsar(a, bStart)
	if err := guardar(a, "9-por-numero"); err != nil {
		return err
	}
	pulsar(a, bY, bX)
	for _, r := range "zzz" {
		a.tec.escribir(r)
	}
	pulsar(a, bStart)
	if err := guardar(a, "10-sin-resultados"); err != nil {
		return err
	}
	pulsar(a, bSelect, bB) // vuelve a la lista y quita lo buscado
	a.ponerSel(total - 3)
	return guardar(a, "11-cartas-final")
}
