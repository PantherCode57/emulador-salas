package main

import (
	"errors"
	"time"
)

// Lo que la app sabe de tu cuenta y de tu curso (lo saca duolingo.go de lo que manda Duolingo).
type perfil struct {
	Usuario  string
	Nombre   string
	Racha    int
	RachaHoy bool // hoy ya has hecho algo (la llama encendida)
	XPTotal  int
	XPHoy    int
	Meta     int // XP al día
	Gemas    int
	Vidas    int // -1: sin límite (Super o vidas quitadas)
	Curso    string
	Desde    string // tu idioma («es»)
	Aprende  string // el que aprendes («en»)
	CursoID  string
	Liga     string
}

const (
	estHecho     = "passed"
	estActivo    = "active"
	estBloqueado = "locked"
)

// peticion: lo que hace falta para pedirle a Duolingo una lección de un nivel.
type peticion struct {
	tipo        string         // «practica» o «nivel»
	cuerpo      map[string]any // lo que se añade a lo que se manda al empezarla
	especificos map[string]any // pathLevelSpecifics: lo que se manda al acabarla
	final       bool           // es la última lección del nivel
}

type nivel struct {
	ID       string
	Nombre   string
	Tipo     string // skill, story, chest, unit_review, practice…
	Estado   string // passed, active, locked
	Hechas   int    // lecciones hechas del nivel
	Total    int
	Peticion peticion
}

type unidad struct {
	Indice  int
	Titulo  string // «Unidad 3»
	Tema    string // «Pide comida»
	Seccion string // «Sección 1»
	Niveles []nivel
}

// resultadoFin: lo que contesta Duolingo al mandar la lección.
type resultadoFin struct {
	XP    int
	Racha int
	Hora  time.Time
	Error string
}

var (
	errSesion = errors.New("la sesión no vale")
	errLimite = errors.New("Duolingo dice que esperes un poco")
)
