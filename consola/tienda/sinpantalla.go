package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// sinPantalla: -lista, -poner y -quitar, para probar la tienda por SSH. Hace lo mismo que los
// botones (el mismo trabajador), pero no reinicia EmulationStation: lo dice.
func sinPantalla(poner, quitar string) error {
	t := nuevaTienda()
	t.tarjetaApp = func(e *elemento, ruta string) error { return tarjetaDeApp(cargarFuentes(), e, ruta) }
	ctx, cancel := context.WithTimeout(t.ctx, 2*time.Hour)
	defer cancel()
	if ind, err := leerIndice(ctx, t.fuente); ind != nil {
		t.indice = ind
	} else {
		t.errIndice = err
		fmt.Printf("fuente de las apps (%s): %s; se usan las copias de la consola\n", t.fuente, mensaje(err))
	}
	zips := make([]string, len(t.juegos))
	for i, e := range t.juegos {
		zips[i] = e.juego.zip
	}
	if cat, err := leerCatalogoPM(ctx, zips); cat != nil {
		aplicarCatalogoPM(t.juegos, cat)
	} else {
		fmt.Printf("catálogo de PortMaster: %s; se usa el de la tienda\n", mensaje(err))
	}
	t.mirar()
	if poner == "" && quitar == "" {
		for _, l := range [][]*elemento{t.apps, t.juegos} {
			for _, e := range l {
				estado := "sin poner"
				switch {
				case e.puesto:
					estado = "PUESTO"
				case !e.disponible:
					estado = "no se puede: " + e.motivo
				}
				fmt.Printf("%-22s %-26s %9s  %s\n", e.id, e.nombre, tamTexto(e.tam), estado)
			}
		}
		return nil
	}
	id := poner + quitar
	var e *elemento
	for _, l := range [][]*elemento{t.apps, t.juegos} {
		for _, x := range l {
			if strings.EqualFold(x.id, id) || strings.EqualFold(x.id, id+".zip") || strings.EqualFold(x.nombre, id) {
				e = x
			}
		}
	}
	if e == nil {
		return fmt.Errorf("no hay nada que se llame %q (mira -lista)", id)
	}
	quitando := quitar != ""
	switch {
	case quitando && !e.puesto:
		return fmt.Errorf("%s no está puesto", e.nombre)
	case !quitando && e.puesto:
		fmt.Printf("%s ya está puesto\n", e.nombre)
		return nil
	case !quitando && !e.disponible:
		return errors.New(e.motivo)
	}
	ultimo := ""
	informar := func(fase string, h, tt int64) {
		s := fase
		if tt > 0 {
			s = fmt.Sprintf("%s %3d %%  %s de %s", fase, int(float64(h)/float64(tt)*100), tamTexto(h), tamTexto(tt))
		}
		if s != ultimo {
			ultimo = s
			fmt.Printf("\r%-60s", s)
		}
	}
	var err error
	if e.tipo == tApp {
		tr := trabajoApp{e: e, indice: t.indice}
		if yt := porNombre(t.apps, "youtube"); yt != nil && yt.puesto {
			tr.otraYT = true
		}
		if quitando {
			err = t.quitarApp(ctx, tr, informar)
		} else {
			err = t.ponerApp(ctx, tr, informar)
		}
	} else {
		tr := trabajoJuego{g: *e.juego, nombre: e.nombre, larga: e.larga, genero: e.genero}
		if im, cerr := capturaJuego(ctx, *e.juego); cerr == nil {
			_, tr.imagen = iconosDe(im)
		}
		if quitando {
			err = t.quitarJuego(ctx, tr, informar)
		} else {
			err = t.ponerJuego(ctx, tr, informar)
		}
	}
	fmt.Println()
	if err != nil {
		return err
	}
	if quitando {
		fmt.Printf("%s quitado. Reinicia EmulationStation para que se vaya su tarjeta.\n", e.nombre)
	} else {
		fmt.Printf("%s puesto. Reinicia EmulationStation para que salga su tarjeta.\n", e.nombre)
	}
	_ = os.Stdout.Sync()
	return nil
}

func porNombre(l []*elemento, id string) *elemento {
	for _, e := range l {
		if e.id == id {
			return e
		}
	}
	return nil
}
