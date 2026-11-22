// Package ig: Instagram para el puente de la DS. Es el mismo Instagram de la app de la consola:
// instagram.go, mensajes.go, pagina.go, cuenta.go y json.go son copias de consola/instagram con
// «package ig» en vez de «package main» (si allí se arregla algo porque Instagram ha cambiado, se
// vuelve a copiar aquí). Este archivo pone lo poco que esos archivos toman del resto de la app de
// la consola y lo que usa el puente.
package ig

import (
	"log"
	"os"
	"path/filepath"
)

// carpetaDatos: donde se guardan la sesión (cuenta.json) y lo que se aprende de la web de Instagram
// (web.json y docs.json). La pone Nuevo.
var carpetaDatos = "instagram"

func entorno(clave, defecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return defecto
}

func logf(format string, args ...any) { log.Printf("instagram: "+format, args...) }

func uno(si bool) int {
	if si {
		return 1
	}
	return 0
}

// hayRed: en la consola se mira aparte; en el puente se da por hecho (si no hay, ya fallará).
func hayRed() bool { return true }

// Errores que el puente distingue.
var (
	ErrSesion    = errSesion
	ErrSinCuenta = errSinCuenta
	ErrLimite    = errLimite
	ErrReto      = errReto
	ErrMensajes  = errMensajes
)

// Cliente: la cuenta de Instagram de la dueña de la DS.
type Cliente struct {
	ig *instagram
	c  *cuenta
}

// Nuevo: el Instagram con la sesión guardada en carpeta (si la hay).
func Nuevo(carpeta string) *Cliente {
	carpetaDatos = carpeta
	_ = os.MkdirAll(carpeta, 0o700)
	c := cargarCuenta(filepath.Join(carpeta, "cuenta.json"))
	return &Cliente{ig: nuevoInstagram(c), c: c}
}

func (k *Cliente) Tiene() bool     { return k.c.tiene() }
func (k *Cliente) Usuario() string { return k.c.usuario() }

// Poner: la sesión nueva (las cookies de instagram.com, como las da el navegador del móvil al
// entrar). Se comprueba con Instagram y se guarda. Devuelve ErrLimite si no se ha podido comprobar
// pero se ha guardado igual.
func (k *Cliente) Poner(cookies string) error { return k.ig.ponerCuenta(cookies) }

// Olvidar: salir de la cuenta (se borra la sesión guardada).
func (k *Cliente) Olvidar() { k.c.olvidar() }

func (k *Cliente) Inicio(cursor string) (Pagina, error)     { return k.ig.Inicio(cursor) }
func (k *Cliente) Reels(cursor string) (Pagina, error)      { return k.ig.Reels(cursor) }
func (k *Cliente) Historias() ([]Bandeja, error)            { return k.ig.Historias() }
func (k *Cliente) HistoriasDe(id string) ([]Post, error)    { return k.ig.HistoriasDe(id) }
func (k *Cliente) Chats() ([]Chat, error)                   { return k.ig.Chats() }
func (k *Cliente) Enviar(id, texto string) error            { return k.ig.Enviar(id, texto) }
func (k *Cliente) MeGusta(pk string, si bool) error         { return k.ig.MeGusta(pk, si) }
func (k *Cliente) QuienSoy() (usuario, nombre, foto string, err error) { return k.ig.QuienSoy() }

// Conversacion: los últimos mensajes de un chat, del más viejo al más nuevo.
func (k *Cliente) Conversacion(id string) ([]Mensaje, error) {
	m, _, err := k.ig.Conversacion(id, "")
	return m, err
}

// EsVideo: si el medio es un vídeo (en la consola no hacía falta fuera del paquete).
func (m Medio) EsVideo() bool { return m.esVideo() }
