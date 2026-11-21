// Pasar a la DS: copia Panther OS para la Nintendo DS Lite (panther.nds, de «Nintendo DS Lite/»)
// a la tarjeta microSD de la flashcard, usando la ranura de la consola. Es la misma ranura donde
// va la tarjeta de Panther OS, y no hace falta ningún lector.
//
// Cómo puede cambiarse la tarjeta con la consola encendida:
//   - PasarDS.sh copia este programa y panther.nds a la memoria (/dev/shm) y lo arranca desde ahí
//     como un servicio aparte (systemd-run). Desde ese momento ya no necesita nada de la tarjeta.
//   - Antes de que la saques: para EmulationStation, quita la memoria de intercambio, guarda todo
//     en la tarjeta (sync) y la deja en solo lectura (sysrq «u»). Así, sacarla no estropea nada.
//   - Sacas la de Panther OS y metes la de la flashcard. Si la consola no se entera sola, el programa
//     vuelve a mirar la ranura cada pocos segundos (suelta el lector y lo vuelve a coger).
//   - Copia panther.nds a la raíz de la tarjeta de la flashcard (solo ese archivo: lo demás no se
//     toca), lo guarda y la suelta.
//   - Sacas la de la flashcard y vuelves a meter la de Panther OS. La reconoce por su número de
//     serie (CID) y reinicia la consola.
//
// Si algo se tuerce, la de Panther OS está en solo lectura: se apaga la consola con el botón, se
// mete la de Panther OS y se enciende.
//
// Uso: pasards -nds /dev/shm/pasards/panther.nds [-registro archivo] [-prueba carpeta]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type estado int

const (
	eInicio estado = iota
	ePreparando
	eNoSePuede // falló antes de tocar la tarjeta: vuelve al menú
	eSacar     // la de Panther OS ya se puede sacar
	eCopiando
	eListo
	eError // no se ha podido copiar: hay que volver a meter la de Panther OS
	eReiniciando
)

// Lo que se enseña en la pantalla.
type vista struct {
	estado  estado
	cuenta  int    // segundos que quedan en el inicio
	ranura  string // qué hay ahora en la ranura
	mensaje string // en eNoSePuede y eError
}

const esperaInicio = 30 // segundos para pulsar un botón y empezar

func main() {
	nds := flag.String("nds", "/dev/shm/pasards/panther.nds", "el programa para la DS")
	registro := flag.String("registro", "/dev/shm/pasards/registro.txt", "el registro (en la memoria)")
	prueba := flag.String("prueba", "", "dibuja todas las pantallas en esta carpeta y sale")
	flag.Parse()

	if *prueba != "" {
		if err := pruebas(*prueba); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	rutaRegistro = *registro
	if f, err := os.OpenFile(*registro, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	}
	// Al sacar la tarjeta puede morirse lo que nos arrancó: que no nos lleve con él.
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)

	// Fuera EmulationStation (y con él PasarDS.sh): la pantalla y los botones son nuestros. Si no
	// se sigue adelante, volverAlMenu lo vuelve a arrancar.
	pararMenu()

	datos, err := os.ReadFile(*nds)
	if err != nil || len(datos) < 512 {
		log.Printf("no está panther.nds: %v", err)
		pintar(vista{estado: eNoSePuede, mensaje: "Falta panther.nds: quita y vuelve a poner la app en la Panther Store."})
		time.Sleep(6 * time.Second)
		volverAlMenu()
		return
	}

	botones := escucharBotones()
	panther, err := tarjetaDeRaiz()
	if err != nil {
		log.Printf("tarjeta de Panther OS: %v", err)
		pintar(vista{estado: eNoSePuede, mensaje: "No encuentro la tarjeta de Panther OS: " + err.Error()})
		guardarRegistro()
		time.Sleep(6 * time.Second)
		volverAlMenu()
		return
	}
	log.Printf("Panther OS va en %s (%s, CID %s), lector %s", panther.disco, panther.tipo, panther.cid, panther.host)

	if !esperarInicio(botones) {
		log.Printf("cancelado")
		volverAlMenu()
		return
	}

	pintar(vista{estado: ePreparando})
	lector, err := preparar(panther)
	if err != nil {
		log.Printf("preparar: %v", err)
		v := vista{estado: eNoSePuede, mensaje: err.Error()}
		pintar(v)
		time.Sleep(10 * time.Second)
		volverAlMenu()
		return
	}
	log.Printf("lista para cambiar de tarjeta (lector %s)", lector.driver)
	cambiarTarjetas(panther, lector, datos, botones)
}

// esperarInicio: cuenta atrás; un botón empieza. Si no se pulsa nada, se cancela.
func esperarInicio(botones <-chan struct{}) bool {
	vaciar(botones)
	fin := time.Now().Add(esperaInicio * time.Second)
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		queda := int(time.Until(fin).Seconds() + 0.99)
		if queda <= 0 {
			return false
		}
		pintar(vista{estado: eInicio, cuenta: queda})
		select {
		case <-botones:
			return true
		case <-t.C:
		}
	}
}

func vaciar(c <-chan struct{}) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// cambiarTarjetas: desde aquí la de Panther OS está en solo lectura y se puede sacar.
//
// La ranura se mira cada 300 ms. Si la consola no nota sola que se ha cambiado la tarjeta, al
// pulsar un botón se le hace mirar otra vez (se suelta el lector y se vuelve a coger). Eso solo se
// hace con un botón, para que no parezca que se ha sacado la de Panther OS sin sacarla.
func cambiarTarjetas(panther tarjeta, l lector, datos []byte, botones <-chan struct{}) {
	v := vista{estado: eSacar}
	seQuito := false // la de Panther OS ha salido de la ranura alguna vez
	antes := ""
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		hay := tarjetasEn(panther.host)
		otra, estaPanther := clasificar(hay, panther.cid)
		if !estaPanther {
			seQuito = true
		}
		v.ranura = describirRanura(hay, panther.cid)
		if v.ranura != antes {
			log.Printf("ranura: %s", v.ranura)
			antes = v.ranura
		}

		if v.estado == eSacar && otra != nil {
			v.estado = eCopiando
			pintar(v)
			log.Printf("tarjeta nueva: %s (CID %s)", otra.disco, otra.cid)
			if err := copiarA(*otra, datos); err != nil {
				log.Printf("copiar: %v", err)
				v.estado, v.mensaje = eError, err.Error()
			} else {
				log.Printf("panther.nds copiado (%d bytes)", len(datos))
				v.estado = eListo
			}
		}
		// Ha vuelto la de Panther OS (después de salir): a reiniciar. También si se vuelve a meter
		// sin haber metido la de la flashcard.
		if seQuito && estaPanther && otra == nil && v.estado != eCopiando {
			v.estado = eReiniciando
			pintar(v)
			log.Printf("ha vuelto la de Panther OS: reinicio")
			time.Sleep(1500 * time.Millisecond)
			reiniciar()
			return
		}
		pintar(v)

		select {
		case <-botones:
			log.Printf("botón: vuelvo a mirar la ranura")
			pintar(vista{estado: v.estado, ranura: "mirando…", mensaje: v.mensaje})
			if err := l.releer(); err != nil {
				log.Printf("releer: %v", err)
			}
			vaciar(botones)
		case <-t.C:
		}
	}
}

// clasificar: la tarjeta que no es la de Panther OS (si hay) y si está la de Panther OS.
func clasificar(hay []tarjeta, cidPanther string) (*tarjeta, bool) {
	var otra *tarjeta
	esta := false
	for i := range hay {
		if hay[i].cid == cidPanther {
			esta = true
		} else if otra == nil {
			otra = &hay[i]
		}
	}
	return otra, esta
}

func describirRanura(hay []tarjeta, cidPanther string) string {
	if len(hay) == 0 {
		return "vacía"
	}
	for _, t := range hay {
		if t.cid == cidPanther {
			return "la tarjeta de Panther OS"
		}
	}
	return "otra tarjeta (" + tamTexto(hay[0].tam) + ")"
}

func tamTexto(b int64) string {
	if b <= 0 {
		return "no se sabe cuánto mide"
	}
	gb := float64(b) / 1e9
	if gb < 1 {
		return fmt.Sprintf("%d MB", b/1e6)
	}
	return fmt.Sprintf("%.0f GB", gb)
}
