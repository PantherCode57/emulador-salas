package main

import (
	"image"
	"image/color"
)

// Lo que se puede poner: las apps de Panther OS y los juegos libres de PortMaster que ya vienen con
// sus datos («ready to run»: rtr) y no piden una consola potente (sin «power» en reqs). Todos los
// de abajo se comprobaron en el ports.json de PortsMaster/PortMaster-New el 20/11/2026 (rtr: true,
// reqs: [], runtime: [] y aarch64). Lo que va aquí de cada juego (dirección, md5, tamaño) solo se
// usa si no se puede leer el ports.json de PortMaster, que manda.

type tipo int

const (
	tApp tipo = iota
	tJuego
)

// pieza: un archivo de una app. «nombre» es como se llama en la fuente de la tienda (los mismos que
// deja la compilación de .github/workflows/consola.yml en piezas/); «locales», dónde puede estar en
// ~/panther-instalar; «destino», dónde va (relativo a ~/panther/<app>; con «/» delante, absoluto).
type pieza struct {
	nombre  string
	locales []string
	destino string
	modo    uint32
	ytdlp   bool // yt-dlp: se copia de otra app o se baja de su página oficial
	nucleo  bool // el núcleo de la pantalla: se copia de otra app si no está en la fuente
}

type appPanther struct {
	sistema  string // carpeta de /roms y nombre del apartado de EmulationStation
	script   string // lo que abre EmulationStation (/roms/<sistema>/<script>)
	piezas   []pieza
	carpetas []string // carpetas que hay que crear (absolutas)
	servicio bool     // WhatsApp: el programa es también el servicio de Panther (no se borra)
}

type juegoPM struct {
	zip     string // nombre en PortMaster («cave.story-evo.zip»)
	url     string
	md5     string
	tam     int64
	items   []string // lo que pone en /roms/ports (como en su port.json)
	captura string   // la imagen de su carpeta en el repositorio de PortMaster
	anio    int
	autor   string   // quién lo hizo (para la lista de EmulationStation)
	runtime []string // lo que pide de PortMaster (libs/*.squashfs); los de la lista no piden nada
}

type elemento struct {
	id       string
	tipo     tipo
	nombre   string
	desc     string // corta, para la lista
	larga    string // para la ficha
	genero   string
	color    color.RGBA // el del icono dibujado (si no hay imagen)
	glifo    string
	app      *appPanther
	juego    *juegoPM
	licencia string

	// Lo que cambia (solo desde el bucle de la app):
	puesto     bool
	tam        int64 // lo que se baja (0: no se sabe)
	disponible bool  // se puede poner ahora
	motivo     string
	op         *operacion
	icono      *image.RGBA // 104×60 con las esquinas redondas
	imagen     *image.RGBA // para la ficha
	fallo      string      // lo último que salió mal
}

var ytdlp = pieza{nombre: "yt-dlp", destino: "bin/yt-dlp", modo: 0o755, ytdlp: true}

func catalogoApps() []*elemento {
	app := func(id, nombre, script, desc, larga string, c color.RGBA, glifo string, extra ...pieza) *elemento {
		a := &appPanther{sistema: id, script: script}
		e := &elemento{id: id, tipo: tApp, nombre: nombre, desc: desc, larga: larga, color: c, glifo: glifo, app: a}
		a.piezas = []pieza{
			{nombre: id, locales: []string{id + "-arm64", id + "/" + id}, destino: id, modo: 0o755},
			{nombre: "pokemulador_libretro.so", locales: []string{"pokemulador_libretro.so", "whatsapp/pokemulador_libretro.so"},
				destino: "pokemulador_libretro.so", modo: 0o644, nucleo: true},
			{nombre: script, locales: []string{script}, destino: "/roms/" + id + "/" + script, modo: 0o755},
		}
		a.piezas = append(a.piezas, extra...)
		return e
	}
	lista := []*elemento{
		app("whatsapp", "WhatsApp", "WhatsApp.sh",
			"Tus chats en la consola: lee y contesta, con avisos de los mensajes nuevos.",
			"Tus chats de WhatsApp en la consola: lees y contestas con el teclado en pantalla, ves fotos y stickers, y te avisa de los mensajes nuevos aunque estés jugando. Se vincula como un dispositivo más, escaneando un QR con el móvil.",
			color.RGBA{0x25, 0xd3, 0x66, 0xff}, ""),
		app("claude", "Claude", "Claude.sh",
			"Habla con Claude: le llega a la conversación abierta en el PC de casa.",
			"Un chat con Claude desde la consola. Lo que escribes le llega a la conversación de Claude que hay abierta en el PC de casa, y su respuesta sale aquí. Si el PC está apagado, tus mensajes esperan.",
			color.RGBA{0xd9, 0x77, 0x57, 0xff}, ""),
		app("instagram", "Instagram", "Instagram.sh",
			"Tu inicio, los reels, las historias y tus mensajes, con «me gusta».",
			"Instagram con tu cuenta: tu inicio con fotos y vídeos, los reels, las historias y tus mensajes, con «me gusta» y respuestas con el teclado en pantalla. La sesión se pega desde el PC o el móvil.",
			color.RGBA{0xd6, 0x29, 0x76, 0xff}, ""),
		app("duolingo", "Duolingo", "Duolingo.sh",
			"Tus lecciones con los botones; suman XP y racha como en el móvil.",
			"Duolingo con tu cuenta: tu camino, tu racha, tus gemas y tu liga, y las lecciones con los botones (elegir, formar frases, escuchar, emparejar y escribir). Al acabar se mandan a Duolingo y suman XP y racha.",
			color.RGBA{0x58, 0xcc, 0x02, 0xff}, ""),
		app("ajedrez", "Ajedrez", "Ajedrez.sh",
			"Lichess: reta a tus amigos, juega contra la máquina o haz problemas.",
			"Ajedrez con tu cuenta de Lichess: reta a un amigo por su nombre (él juega desde el móvil o la web), juega contra la máquina de Lichess, sigue tus partidas y resuelve problemas con pista.",
			color.RGBA{0x5a, 0x9a, 0x2a, 0xff}, ""),
		app("zarpa", "Zarpa", "Zarpa.sh",
			"Carreras con Zarpa, la pantera, por una autopista de Los Ángeles.",
			"Un juego de carreras hecho para la consola: Zarpa, una pantera pequeña con bufanda y mochila, corre por una autopista de Los Ángeles al atardecer. Salta vallas, agáchate bajo los carteles, esquiva los camiones de tacos y gasta tus estrellas en trajes. No necesita internet.",
			color.RGBA{0xf0, 0x8a, 0x24, 0xff}, "\uf1b9"),
		app("youtube", "YouTube", "Youtube.sh",
			"Busca y mira vídeos, con tu cuenta y tus suscripciones.",
			"YouTube en la consola: busca y mira vídeos, tus suscripciones y tu historial, y guarda vídeos para verlos sin conexión. Usa yt-dlp, que se baja de su página oficial si falta.",
			color.RGBA{0xe5, 0x2d, 0x27, 0xff}, "", ytdlp),
		app("spotin", "Spotin", "Spotin.sh",
			"Tu música de Spotify sonando en la consola: listas y búsqueda.",
			"Tu música de Spotify en la consola: tus listas, las canciones que te gustan y la búsqueda, con la pantalla apagada si quieres. El sonido sale de YouTube con yt-dlp, que se baja de su página oficial si falta.",
			color.RGBA{0x1d, 0xb9, 0x54, 0xff}, "", ytdlp),
		app("pokemon", "Pokémon", "Pokemon.sh",
			"Tus juegos de Pokémon con salas para jugar con amigos.",
			"Pokemulador en la consola: tus juegos de Pokémon de GB, GBC y GBA con salas para jugar con amigos por internet, verlos por el mapa (GBA-PK) e intercambiar. Los juegos no vienen con la app: son los tuyos.",
			color.RGBA{0xe3, 0x35, 0x3a, 0xff}, "",
			pieza{nombre: "enlace", locales: []string{"enlace"}, destino: "enlace", modo: 0o755},
			pieza{nombre: "mgba_libretro.so", locales: []string{"mgba_libretro.so"}, destino: "mgba_libretro.so", modo: 0o644},
			pieza{nombre: "GBA-PK_Multiplayer.lua", locales: []string{"pokemon-lua/GBA-PK_Multiplayer.lua"}, destino: "GBA-PK_Multiplayer.lua", modo: 0o644},
			pieza{nombre: "pokemulador_final.lua", locales: []string{"pokemon-lua/pokemulador_final.lua"}, destino: "pokemulador_final.lua", modo: 0o644},
			pieza{nombre: "caramelos-frlg.lua", locales: []string{"pokemon-lua/caramelos-frlg.lua"}, destino: "caramelos-frlg.lua", modo: 0o644}),
		app("pokedex", "Pokédex", "Pokedex.sh",
			"Todos los Pokémon con su número: marca las cartas que tienes.",
			"La Pokédex nacional entera, del 1 al 1025, con el número y el nombre de cada Pokémon. En «Cartas» marcas los que tienes y se guarda, y el buscador los encuentra por nombre o por número con el teclado en pantalla. Los dibujos de los Pokémon se bajan de PokeAPI a la consola la primera vez; lo demás va sin internet. Los juegos llegarán más adelante.",
			color.RGBA{0xe3, 0x2b, 0x3c, 0xff}, ""),
		app("navegador", "Navegador", "Navegador.sh",
			"Un navegador sencillo para leer páginas web con los botones.",
			"Un navegador para leer páginas web en la consola: textos, imágenes y enlaces, que se recorren con la cruceta, y el teclado en pantalla para escribir direcciones y buscar.",
			color.RGBA{0x1e, 0x88, 0xe5, 0xff}, ""),
	}
	for _, e := range lista {
		switch e.id {
		case "whatsapp":
			e.app.servicio = true
		case "youtube":
			e.app.carpetas = []string{"/roms/videos/youtube"}
		}
		e.licencia = "Panther OS"
	}
	return lista
}

func catalogoJuegos() []*elemento {
	j := func(nombre, genero, desc, larga string, c color.RGBA, g *juegoPM) *elemento {
		return &elemento{id: g.zip, tipo: tJuego, nombre: nombre, desc: desc, larga: larga, genero: genero,
			color: c, glifo: "", juego: g, licencia: "PortMaster"}
	}
	return []*elemento{
		j("Cave Story", "Plataformas, acción",
			"Un robot sin memoria despierta en una cueva llena de misterios.",
			"Un robot sin memoria despierta en una cueva habitada por los Mimiga, unos conejos pacíficos a los que alguien quiere usar como arma. Plataformas y disparos con mucha historia: uno de los juegos independientes más queridos. Va con NXEngine-evo.",
			color.RGBA{0xc0, 0x6a, 0x2b, 0xff},
			&juegoPM{zip: "cave.story-evo.zip", tam: 9387715, md5: "aad671048542c591730c8a925006b4f6",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2026-10-21_2023/cave.story-evo.zip",
				items: []string{"Cave Story-evo.sh", "nxengine-evo/"}, captura: "screenshot.jpg", anio: 2004, autor: "Studio Pixel"}),
		j("Celeste Classic", "Plataformas",
			"La primera Celeste: sube la montaña saltando y con el impulso.",
			"La versión original de Celeste, hecha en cuatro días para PICO-8: ayuda a Madeline a subir la montaña con saltos precisos y el impulso en el aire. Treinta niveles cortos, rápidos y difíciles.",
			color.RGBA{0x8a, 0x3f, 0xb0, 0xff},
			&juegoPM{zip: "cceleste.zip", tam: 1621592, md5: "2952660321ffb17f151c4a3d4d59d84c",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2025-10-04_1030/cceleste.zip",
				items: []string{"ccleste/", "Celeste Classic.sh"}, captura: "screenshot.jpg", anio: 2015, autor: "Matt Thorson y Noel Berry"}),
		j("SuperTux", "Plataformas",
			"Tux, el pingüino de Linux, en un juego de plataformas de los de antes.",
			"Tux, el pingüino de Linux, en un juego de plataformas como los clásicos: saltos, enemigos, monedas, secretos y jefes, en mundos de nieve, bosques y castillos. Es grande: tarda un rato en bajar.",
			color.RGBA{0x2f, 0x7f, 0xd0, 0xff},
			&juegoPM{zip: "supertux.zip", tam: 186491394, md5: "e6889bda80d1513a8671ad33aa89a8d9",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2025-10-04_1030/supertux.zip",
				items: []string{"supertux/", "SuperTux.sh"}, captura: "screenshot.jpg", anio: 2003, autor: "El equipo de SuperTux"}),
		j("Sonic Robo Blast 2", "Plataformas en 3D",
			"Un Sonic en 3D hecho por fans, con Sonic, Tails y Knuckles.",
			"Un juego de Sonic en 3D hecho por fans (Sonic Team Junior): corre con Sonic, Tails o Knuckles por zonas enormes, con jefes, emblemas que buscar y modos para jugar en compañía. Es grande: tarda un rato en bajar.",
			color.RGBA{0x1f, 0x4f, 0xc8, 0xff},
			&juegoPM{zip: "srb2.zip", tam: 190881717, md5: "f9d60fcf238b95796880d9deec3f3f2e",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2025-10-04_1030/srb2.zip",
				items: []string{"SRB2/", "SRB2.sh"}, captura: "screenshot.jpg", anio: 2000, autor: "Sonic Team Junior"}),
		j("Sonic Robo Blast 2 Kart", "Carreras",
			"Carreras de karts con los personajes de Sonic: derrapes y objetos.",
			"Carreras de karts con los personajes de Sonic Robo Blast 2: derrapa para coger turbo, usa objetos contra los demás y corre en muchos circuitos. Es grande: tarda un rato en bajar.",
			color.RGBA{0xd0, 0x3a, 0x2f, 0xff},
			&juegoPM{zip: "srb2kart.zip", tam: 221179868, md5: "0040ad2a5d91ab6fff8311373b9d85d3",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2025-10-04_1030/srb2kart.zip",
				items: []string{"Sonic Robo Blast 2 Kart.sh", "srb2kart/"}, captura: "screenshot.jpg", anio: 2018, autor: "Kart Krew"}),
		j("OpenTTD", "Estrategia, simulación",
			"Monta tu empresa de transportes: trenes, camiones, barcos y aviones.",
			"Construye tu empresa de transportes: vías, carreteras, puertos y aeropuertos para llevar pasajeros y mercancías entre ciudades que crecen. Basado en Transport Tycoon Deluxe; el ratón se mueve con la palanca.",
			color.RGBA{0x3c, 0x8d, 0x4a, 0xff},
			&juegoPM{zip: "openttd.zip", tam: 68230910, md5: "cbb46ed4139c2dbf6221bdad1a7aecda",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2026-05-14_1654/openttd.zip",
				items: []string{"OpenTTD.sh", "openttd"}, captura: "screenshot.png", anio: 2004, autor: "El equipo de OpenTTD"}),
		j("OpenTyrian", "Naves, disparos",
			"Tyrian, el de naves de los 90: dispara y mejora tu nave.",
			"Tyrian, un juego de naves de los años 90 que su autor regaló: dispara a todo lo que se mueve, gana dinero para mejorar tu nave entre nivel y nivel y sigue la historia de Trent Hawkins.",
			color.RGBA{0x6a, 0x4c, 0x9c, 0xff},
			&juegoPM{zip: "opentyrian.zip", tam: 5374583, md5: "749b4765627786604f4f3b3bfd1a47fd",
				url:   "https://github.com/PortsMaster/PortMaster-New/releases/download/2026-04-02_0757/opentyrian.zip",
				items: []string{"OpenTyrian.sh", "opentyrian/"}, captura: "screenshot.jpg", anio: 1995, autor: "Eclipse Software"}),
	}
}
