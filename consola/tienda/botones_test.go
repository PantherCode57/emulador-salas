package main

import (
	"crypto/md5"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// esperarBucle hace lo del bucle (las tareas del trabajador) hasta que no quede nada en marcha.
func esperarBucle(t *testing.T, a *app) {
	t.Helper()
	fin := time.After(10 * time.Second)
	for a.ops > 0 || len(a.tareas) > 0 {
		select {
		case fn := <-a.tareas:
			fn()
		case <-fin:
			t.Fatal("el trabajador no acaba")
		}
	}
}

// Con los botones, como en la consola: poner una app con A, quitarla (A, y A en la pregunta),
// cambiar a los juegos con R, poner uno y ver la ficha.
func TestConLosBotones(t *testing.T) {
	tt := tiendaDePrueba(t)
	escribir(t, filepath.Join(tt.local, "claude-arm64"), "claude")
	escribir(t, filepath.Join(tt.local, "Claude.sh"), "#!/bin/bash\n")
	escribir(t, filepath.Join(tt.panther, "whatsapp", "pokemulador_libretro.so"), "nucleo")
	datos := zipDePrueba(t, nil)
	suma := md5.Sum(datos)
	f := cargarFuentes()
	a := nuevaApp(f, tt)
	a.cargaHasta = time.Time{}
	a.arrancarTrabajador()
	defer a.terminar()
	// El primer juego de la lista apunta a nuestro servidor.
	g := *tt.juegos[0].juego
	g.url, g.md5, g.tam = servir(t, datos), hex.EncodeToString(suma[:]), int64(len(datos))
	g.items = []string{"Juego.sh", "juego/"}
	tt.juegos[0].juego = &g
	tt.mirar()

	// Claude es la segunda: abajo y A.
	a.pulsar(bAbajo)
	if a.elegido().id != "claude" {
		t.Fatalf("elegido %s", a.elegido().id)
	}
	a.pulsar(bA)
	if a.ops != 1 || a.elegido().op == nil {
		t.Fatal("A no ha empezado a poner Claude")
	}
	esperarBucle(t, a)
	if !a.elegido().puesto || !tt.cambios {
		t.Fatalf("Claude no ha quedado puesta (%s)", a.elegido().fallo)
	}
	a.dibujar()

	// A otra vez: pregunta; B no hace nada; A, A la quita.
	a.pulsar(bA)
	if a.dlg == nil || !strings.HasPrefix(a.dlg.titulo, "¿Quitar") {
		t.Fatal("no pregunta antes de quitar")
	}
	a.dibujar()
	a.pulsar(bB)
	if a.dlg != nil || a.ops != 0 {
		t.Fatal("B tenía que cancelar la pregunta")
	}
	a.pulsar(bA)
	a.pulsar(bA)
	esperarBucle(t, a)
	if a.elegido().puesto {
		t.Fatal("Claude no se ha quitado")
	}

	// R: los juegos. A pone el primero; X abre su ficha.
	a.pulsar(bR)
	if a.pest != 1 {
		t.Fatal("R no cambia a los juegos")
	}
	a.pulsar(bA)
	a.dibujar()
	esperarBucle(t, a)
	if !a.elegido().puesto {
		t.Fatalf("el juego no ha quedado puesto (%s)", a.elegido().fallo)
	}
	a.pulsar(bX)
	if a.pant != pFicha {
		t.Fatal("X no abre la ficha")
	}
	a.dibujar()
	a.pulsar(bB)
	if a.pant != pLista {
		t.Fatal("B no vuelve a la lista")
	}

	// Un juego sin poner que falla (no hay servidor): sale el fallo y no queda puesto.
	a.pulsar(bAbajo)
	e := a.elegido()
	gg := *e.juego
	gg.url = "http://127.0.0.1:1/nada.zip"
	e.juego = &gg
	a.pulsar(bA)
	esperarBucle(t, a)
	if e.puesto || e.fallo == "" {
		t.Fatal("tenía que salir el fallo")
	}
	a.dibujar()

	// B sale (no hay nada en marcha).
	a.pulsar(bB)
	select {
	case <-a.salir:
	default:
		t.Fatal("B no sale")
	}
}
