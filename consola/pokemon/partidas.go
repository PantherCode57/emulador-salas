package main

import (
	"bytes"
	"os"
	"path/filepath"
	"time"
)

// La partida de siempre es la misma que al jugar desde las listas del menú. RetroArch la deja en
// sitios distintos según el núcleo (con gpSP, en /roms/gba/gpSP/; otras veces junto al juego o en
// ~/.config/retroarch/saves/<núcleo>/): antes de jugar se trae la más nueva y al terminar se
// devuelve a donde estaba (con una copia .bak de la de antes).

type partida struct {
	ruta   string
	tam    int64
	cuando time.Time
}

func candidatasPartida(j Juego) []string {
	base := j.base()
	dir := filepath.Dir(j.Ruta)
	var rutas []string
	for _, ext := range []string{".srm", ".sav"} {
		rutas = append(rutas, filepath.Join(dir, base+ext))
		m, _ := filepath.Glob(filepath.Join(dir, "*", base+ext))
		rutas = append(rutas, m...)
		rutas = append(rutas, filepath.Join(carpetaSavesRA, base+ext))
		m, _ = filepath.Glob(filepath.Join(carpetaSavesRA, "*", base+ext))
		rutas = append(rutas, m...)
	}
	return rutas
}

// partidaMasNueva: la partida de siempre que haya (la más reciente), o vacía.
func partidaMasNueva(j Juego) partida {
	var mejor partida
	for _, r := range candidatasPartida(j) {
		fi, err := os.Stat(r)
		if err != nil || fi.IsDir() || fi.Size() == 0 {
			continue
		}
		if mejor.ruta == "" || fi.ModTime().After(mejor.cuando) {
			mejor = partida{ruta: r, tam: fi.Size(), cuando: fi.ModTime()}
		}
	}
	return mejor
}

// carpetaHabitual: dónde deja las partidas de este sistema el núcleo de siempre (la carpeta con la
// partida más reciente de ese sistema), para un juego que aún no tiene ninguna.
func carpetaHabitual(j Juego) string {
	dir := filepath.Dir(j.Ruta)
	var elegida string
	var cuando time.Time
	for _, patron := range []string{filepath.Join(dir, "*", "*.srm"), filepath.Join(dir, "*.srm")} {
		m, _ := filepath.Glob(patron)
		for _, r := range m {
			if fi, err := os.Stat(r); err == nil && fi.ModTime().After(cuando) {
				elegida, cuando = filepath.Dir(r), fi.ModTime()
			}
		}
	}
	if elegida == "" {
		return dir
	}
	return elegida
}

func copiarArchivo(origen, destino string) error {
	datos, err := os.ReadFile(origen)
	if err != nil {
		return err
	}
	if err := escribirAtomico(destino, datos); err != nil {
		return err
	}
	if fi, err := os.Stat(origen); err == nil {
		_ = os.Chtimes(destino, fi.ModTime(), fi.ModTime())
	}
	return nil
}

// traerPartida deja en «guardados» la partida de siempre más nueva y dice a dónde hay que devolverla.
func traerPartida(j Juego, guardados string) string {
	destinoPerfil := filepath.Join(guardados, j.base()+".srm")
	p := partidaMasNueva(j)
	if p.ruta == "" {
		return filepath.Join(carpetaHabitual(j), j.base()+".srm")
	}
	if fi, err := os.Stat(destinoPerfil); err != nil || p.cuando.After(fi.ModTime()) {
		if err := copiarArchivo(p.ruta, destinoPerfil); err != nil {
			logf("partida: no se pudo traer %s: %v", p.ruta, err)
		} else {
			logf("partida traída de %s", p.ruta)
		}
	}
	if filepath.Ext(p.ruta) == ".sav" { // se devuelve como .srm, que es lo que usa RetroArch
		return filepath.Join(filepath.Dir(p.ruta), j.base()+".srm")
	}
	return p.ruta
}

// devolverPartida: la partida que acaba de jugarse, otra vez a su sitio de siempre.
func devolverPartida(j Juego, guardados, destino string) {
	origen := filepath.Join(guardados, j.base()+".srm")
	nueva, err := os.ReadFile(origen)
	if err != nil || len(nueva) == 0 || destino == "" {
		return
	}
	if vieja, err := os.ReadFile(destino); err == nil {
		if bytes.Equal(vieja, nueva) {
			return
		}
		_ = escribirAtomico(destino+".bak", vieja)
	}
	if err := copiarArchivo(origen, destino); err != nil {
		logf("partida: no se pudo devolver a %s: %v", destino, err)
		return
	}
	logf("partida devuelta a %s", destino)
}
