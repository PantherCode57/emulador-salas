// Flashear tarjeta: pasa Panther OS (y, si es para el mismo dueño, sus cuentas y partidas) a la
// tarjeta de otra consola usando la ranura de esta, que es la misma donde va la tarjeta de Panther
// OS. No hace falta ningún lector. Al abrirla se elige la consola:
//   - Nintendo DS con flashcard: Panther OS para la DS (panther.nds, de «Nintendo DS Lite/»).
//   - Game Boy Advance con flashcard: tus partidas de GBA.
//   - Consola china con dArkOS (como la XF35H): pregunta antes si la tarjeta ya tiene dArkOS. Si lo
//     tiene, se le dejan la Panther Store y la lista de lo elegido, y un servicio que lo pone todo
//     en cuanto esa consola tiene internet (destinos.go). Si es para ti, también tus cuentas (menos
//     la sesión de WhatsApp), tus wifis y tus partidas.
//
// Nunca se pasan juegos comerciales.
//
// Cómo puede cambiarse la tarjeta con la consola encendida:
//   - Flashear.sh copia este programa (y panther.nds) a la memoria (/dev/shm) y lo arranca desde ahí
//     como un servicio aparte (systemd-run). Lo que se va a copiar se lee antes, a la memoria.
//   - Antes de que la saques: para EmulationStation y los servicios de Panther, quita la memoria de
//     intercambio y deja la tarjeta guardada y en solo lectura (o congelada). Así no se estropea.
//   - Sacas la de Panther OS y metes la otra. Si la consola no se entera sola, un botón la hace
//     mirar otra vez (suelta el lector y lo vuelve a coger).
//   - Se escribe lo elegido (solo eso: lo demás de esa tarjeta no se toca), y al volver a meter la de
//     Panther OS, que se reconoce por su número de serie (CID), la consola se reinicia.
//
// Si algo se tuerce: se apaga con el botón, se mete la de Panther OS y se enciende.
//
// Uso: flashear -nds /dev/shm/flashear/panther.nds [-registro archivo] [-prueba carpeta]
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
	ePreparando estado = iota
	eNoSePuede         // falló antes de tocar la tarjeta: vuelve al menú
	eSacar             // la de Panther OS ya se puede sacar
	eCopiando
	eListo
	eError // no se ha podido copiar: hay que volver a meter la de Panther OS
	eReiniciando
)

// Lo que se enseña en las pantallas de cambiar la tarjeta.
type vista struct {
	estado  estado
	otra    string   // «la de la flashcard», «la de la otra consola»
	ranura  string   // qué hay ahora en la ranura
	mensaje string   // en eNoSePuede y eError
	hecho   []string // en eListo: lo que se ha copiado
	luego   string   // en eListo: qué hacer en la otra consola
}

func main() {
	nds := flag.String("nds", "/dev/shm/flashear/panther.nds", "el programa para la DS")
	registro := flag.String("registro", "/dev/shm/flashear/registro.txt", "el registro (en la memoria)")
	prueba := flag.String("prueba", "", "dibuja todas las pantallas en esta carpeta y sale")
	verInstalador := flag.Bool("instalador", false, "escribe el instalador que se deja en las consolas con dArkOS y sale")
	flag.Parse()

	if *prueba != "" {
		if err := pruebas(*prueba); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *verInstalador {
		fmt.Print(instalador)
		return
	}

	rutaRegistro = *registro
	if f, err := os.OpenFile(*registro, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	}
	// Al sacar la tarjeta puede morirse lo que nos arrancó: que no nos lleve con él.
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGPIPE)

	// Fuera EmulationStation (y con él Flashear.sh): la pantalla y los botones son nuestros. Si no se
	// sigue adelante, volverAlMenu lo vuelve a arrancar.
	pararMenu()
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

	d, c, ok := elegirTodo(*nds, botones)
	if !ok {
		log.Printf("cancelado")
		volverAlMenu()
		return
	}
	log.Printf("para %s: %d bytes, %v", d.id, c.tam(), c.resumen)

	pintar(vista{estado: ePreparando})
	lector, err := preparar(panther)
	if err != nil {
		log.Printf("preparar: %v", err)
		pintar(vista{estado: eNoSePuede, mensaje: err.Error()})
		time.Sleep(10 * time.Second)
		volverAlMenu()
		return
	}
	log.Printf("lista para cambiar de tarjeta (lector %s)", lector.driver)
	cambiarTarjetas(panther, lector, d, c, botones)
}

// elegirTodo: los menús, hasta tener qué se pasa y a qué consola. false si se sale.
func elegirTodo(rutaNDS string, botones <-chan tecla) (destino, carga, bool) {
	consolas := menuConsolas()
	for {
		i := elegir(consolas, botones)
		if i < 0 || i >= len(destinos) {
			return destino{}, carga{}, false
		}
		d := destinos[i]
		c, err := elegirCarga(d, rutaNDS, botones)
		if err == errVolver {
			continue
		}
		if err != nil {
			log.Printf("%s: %v", d.id, err)
			elegir(&menu{titulo: "No se puede", explica: err.Error(), ops: []opcion{{titulo: "Vale"}}, ayuda: "A o B volver"}, botones)
			continue
		}
		conf := menuConfirmar(d, c)
		if elegir(conf, botones) == 0 {
			return d, c, true
		}
	}
}

type errorTexto string

func (e errorTexto) Error() string { return string(e) }

const errVolver = errorTexto("volver")

// elegirCarga: las preguntas de cada consola, y lo que se va a pasar (ya leído, en la memoria).
func elegirCarga(d destino, rutaNDS string, botones <-chan tecla) (carga, error) {
	switch d.id {
	case "ds":
		nds, _ := os.ReadFile(rutaNDS)
		return cargaDS(nds)
	case "gba":
		if elegir(menuParaQuien(), botones) != 0 {
			return carga{}, errorTexto("Para otra persona no hay nada que pasar a la GBA todavía (no hay Panther OS para GBA): sus partidas son suyas.")
		}
		ps := buscarPartidas("gba")
		if len(ps) == 0 {
			return carga{}, errorTexto("No hay partidas de GBA en esta consola (en /roms/gba).")
		}
		m := menuPartidas(ps)
		if elegir(m, botones) != aceptar {
			return carga{}, errVolver
		}
		var sel []partida
		for _, i := range marcadas(m) {
			sel = append(sel, ps[i])
		}
		return cargaGBA(sel)
	case "darkos":
		switch elegir(menuTieneDarkOS(), botones) {
		case 0:
		case 1:
			elegir(menuSinDarkOS(), botones)
			return carga{}, errVolver
		default:
			return carga{}, errVolver
		}
		quien := elegir(menuParaQuien(), botones)
		if quien < 0 {
			return carga{}, errVolver
		}
		apps, err := listaTienda()
		if err != nil {
			return carga{}, err
		}
		apps = appsPara(apps, quien == 0)
		m := menuApps(apps)
		if elegir(m, botones) != aceptar {
			return carga{}, errVolver
		}
		var sel []appTienda
		for _, i := range marcadas(m) {
			sel = append(sel, apps[i])
		}
		return cargaDarkOS(sel, quien == 0)
	}
	return carga{}, errVolver
}

func vaciar(c <-chan tecla) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func textoOtra(destino string) string {
	if destino == "darkos" {
		return "la de la otra consola"
	}
	return "la de la flashcard"
}

// cambiarTarjetas: desde aquí la de Panther OS está en solo lectura y se puede sacar.
//
// La ranura se mira cada 300 ms. Si la consola no nota sola que se ha cambiado la tarjeta, al
// pulsar un botón se le hace mirar otra vez (se suelta el lector y se vuelve a coger). Eso solo se
// hace con un botón, para que no parezca que se ha sacado la de Panther OS sin sacarla.
func cambiarTarjetas(panther tarjeta, l lector, d destino, c carga, botones <-chan tecla) {
	v := vista{estado: eSacar, otra: textoOtra(d.id)}
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
			if err := escribirCarga(*otra, c); err != nil {
				log.Printf("copiar: %v", err)
				v.estado, v.mensaje = eError, err.Error()
			} else {
				log.Printf("copiado")
				v.estado, v.hecho, v.luego = eListo, c.resumen, textoLuego(d.id)
			}
		}
		// Ha vuelto la de Panther OS (después de salir): a reiniciar. También si se vuelve a meter
		// sin haber metido la otra.
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
			w := v
			w.ranura = "mirando…"
			pintar(w)
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
