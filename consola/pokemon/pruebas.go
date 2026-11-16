package main

import (
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas (pokemon -prueba carpeta): todas las pantallas con datos inventados, en PNG.
func dibujarPruebas(f *fuentes, carpeta string) error {
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return err
	}
	tmp, _ := os.MkdirTemp("", "pokemon-prueba")
	defer os.RemoveAll(tmp)
	carpetaDatos = tmp
	a := nuevaApp(f)
	a.cargaHasta = time.Time{}
	a.jug = &Jugador{ID: "CONSOLA-PRUEBA", Nombre: "Panther", Elegido: true}
	tarjetas := os.Getenv("POKEMON_PRUEBA_TARJETAS") // carpeta con las tarjetas de GBA ya hechas
	a.juegos = []Juego{
		{Relativa: "gba/Pokemon - Ruby Version (USA).zip", Ruta: "/roms/gba/Pokemon - Ruby Version (USA).zip", Titulo: "Pokémon Rubí",
			Sistema: "gba", Genero: "GBA  ·  2003  ·  Aventura y rol", Tarjeta: filepath.Join(tarjetas, "Pokemon - Ruby Version (USA).png")},
		{Relativa: "gba/Pokemon - Emerald Version (USA, Europe).zip", Ruta: "/roms/gba/Pokemon - Emerald Version (USA, Europe).zip",
			Titulo: "Pokémon Esmeralda", Sistema: "gba", Genero: "GBA  ·  2004  ·  Aventura y rol",
			Tarjeta: filepath.Join(tarjetas, "Pokemon - Emerald Version (USA, Europe).png")},
		{Relativa: "gba/Pokemon - FireRed Version (USA).zip", Ruta: "/roms/gba/Pokemon - FireRed Version (USA).zip", Titulo: "Pokémon Rojo Fuego",
			Sistema: "gba", Genero: "GBA  ·  2004  ·  Aventura y rol", Tarjeta: filepath.Join(tarjetas, "Pokemon - FireRed Version (USA).png")},
		{Relativa: "gbc/Pokemon - Edicion Cristal (Spain).zip", Ruta: "/roms/gbc/Pokemon - Edicion Cristal (Spain).zip", Titulo: "Pokémon Cristal",
			Sistema: "gbc", Genero: "Game Boy Color  ·  2001  ·  Aventura y rol"},
	}
	ahora := ahoraMs()
	a.salas = []Sala{
		{Codigo: "k7p2qx", Nombre: "Sala con Dylan", Juego: a.juegos[1].Relativa, JuegoTitulo: "Pokémon Esmeralda", Sistema: "gba", Creada: ahora},
		{Codigo: "m4te77", Nombre: "Los del instituto", Juego: a.juegos[1].Relativa, JuegoTitulo: "Pokémon Esmeralda", Sistema: "gba",
			PartidaPropia: true, Creada: ahora - 1000},
	}
	// Progreso de los amigos de la primera sala.
	guardarJSON(filepath.Join(carpetaAmigos("k7p2qx"), "dylan.json"), Progreso{Version: 1, ID: "DYLAN", Nombre: "Dylan",
		Juego: "Pokémon Esmeralda", Partida: "principal", JugadoMs: 23 * 3600000, UltimaVez: ahora - 30000})
	guardarJSON(filepath.Join(carpetaAmigos("k7p2qx"), "lucia.json"), Progreso{Version: 1, ID: "LUCIA", Nombre: "Lucía",
		Juego: "Pokémon Zafiro", Partida: "principal", JugadoMs: 48 * 3600000, UltimaVez: ahora - 2*3600000})
	guardarJSON(archivoPropio("k7p2qx"), Progreso{Version: 1, ID: a.jug.ID, Nombre: a.jug.Nombre, JugadoMs: 31 * 3600000})
	a.caramelos[a.juegos[2].Relativa] = true

	guardar := func(nombre string) error {
		a.dibujar()
		fh, err := os.Create(filepath.Join(carpeta, nombre))
		if err != nil {
			return err
		}
		defer fh.Close()
		return png.Encode(fh, a.l.img)
	}
	a.pant, a.selJuego = pJuegos, 1
	if err := guardar("1-juegos.png"); err != nil {
		return err
	}
	a.selJuego = 2
	if err := guardar("1b-rojo-fuego.png"); err != nil {
		return err
	}
	a.selJuego, a.pant, a.selOpcion = 1, pSalas, 2
	if err := guardar("2-salas.png"); err != nil {
		return err
	}
	a.pant, a.crear, a.selPartida = pPartida, true, 0
	if err := guardar("3-partida.png"); err != nil {
		return err
	}
	a.abrirTeclado("codigo")
	a.tec.borrador, a.tec.cursor = []rune("k7p"), 3
	if err := guardar("4-codigo.png"); err != nil {
		return err
	}
	s := a.salas[0]
	a.sala = &s
	a.activa = &salaActiva{estado: EstadoSala{Codigo: s.Codigo, Fase: "conectado", RolDecidido: true, Ruta: "directa"}}
	a.pant = pSala
	if err := guardar("5-sala.png"); err != nil {
		return err
	}
	a.activa.estado = EstadoSala{Codigo: s.Codigo, Fase: "conectando", Login: "https://login.tailscale.com/a/ejemplo"}
	if err := guardar("6-tailscale.png"); err != nil {
		return err
	}
	a.activa.estado = EstadoSala{Codigo: s.Codigo, Fase: "conectado", RolDecidido: true, Ruta: "directa"}
	a.preguntar("¿Borrar la sala K7P2QX? Tu partida no se borra.", nil)
	return guardar("7-confirmar.png")
}
