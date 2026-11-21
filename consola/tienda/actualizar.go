package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// La Panther Store se pone al día sola: al abrirla, si la fuente tiene otra tienda (la pieza
// «tienda» de tienda.json, que deja la compilación de la consola), la baja comprobando su suma, la
// pone en lugar de esta (la de antes queda en tienda.antes) y se abre otra vez con ella. Así le
// llegan las apps nuevas del catálogo sin tocar la consola. Si no hay conexión, se abre la que hay.

// programaPropio: la ruta de este programa si es el de la consola (~/panther/tienda/tienda); si
// no (pruebas en el PC, otra copia), "" y no se cambia nada.
func programaPropio(panther string) string {
	if os.Getenv("TIENDA_ACTUALIZAR") == "no" {
		return ""
	}
	yo, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(yo); err == nil {
		yo = r
	}
	quiero := filepath.Join(panther, "tienda", "tienda")
	if r, err := filepath.EvalSymlinks(quiero); err == nil {
		quiero = r
	}
	if yo != quiero {
		return ""
	}
	return yo
}

// ponerseAlDia: si la fuente tiene otra versión de la tienda, la pone en «yo». Devuelve si la ha
// cambiado. ind es el tienda.json de la fuente.
func ponerseAlDia(ctx context.Context, fuente string, ind *indiceTienda, yo string, informar func(h, t int64)) (bool, error) {
	if ind == nil {
		return false, nil
	}
	p, ok := ind.Piezas["tienda"]
	if !ok || p.SHA256 == "" {
		return false, nil
	}
	suma := "sha256:" + p.SHA256
	if sumaDe(yo, suma) {
		return false, nil
	}
	logf("hay otra Panther Store (%s): la bajo", ind.Version)
	tmp := yo + ".bajando"
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := bajar(ctx, fuente+"tienda", tmp, p.Tam, suma, informar); err != nil {
		return false, fmt.Errorf("la tienda nueva: %w", err)
	}
	_ = os.Chmod(tmp, 0o755)
	antes := yo + ".antes"
	_ = os.Remove(antes)
	if err := os.Link(yo, antes); err != nil {
		_ = copiar(yo, antes, 0o755)
	}
	if err := os.Rename(tmp, yo); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	logf("Panther Store puesta al día (la de antes está en %s)", antes)
	return true, nil
}
