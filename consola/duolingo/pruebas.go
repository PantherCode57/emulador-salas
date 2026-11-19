package main

import (
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// dibujarPruebas: las pantallas con datos de ejemplo, en PNG (para verlas sin consola).
func dibujarPruebas(f *fuentes, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	a := nuevaApp(f, nuevoDuolingo(&cuenta{}))
	a.son.callado = true
	guardar := func(nombre string) error {
		a.dibujar()
		out, err := os.Create(filepath.Join(dir, nombre+".png"))
		if err != nil {
			return err
		}
		defer out.Close()
		return png.Encode(out, a.l.img)
	}
	a.cargaHasta = time.Now().Add(time.Hour)
	if err := guardar("01-carga"); err != nil {
		return err
	}
	a.cargaHasta = time.Time{}
	a.pant, a.sesionTexto = pCuenta, "Esperando tu sesión…"
	if err := guardar("02-cuenta"); err != nil {
		return err
	}

	a.perfil = perfil{Usuario: "alex_aprende", Nombre: "Alex", Racha: 13, RachaHoy: false, XPTotal: 4820, XPHoy: 10,
		Meta: 20, Gemas: 535, Vidas: 4, Curso: "Inglés", Desde: "es", Aprende: "en", Liga: "Oro"}
	a.perfilHecho = true
	a.camino = []unidad{
		{Indice: 0, Seccion: "Sección 1", Titulo: "Unidad 1", Tema: "Pide en una cafetería", Niveles: []nivel{
			{Nombre: "Saludos", Tipo: "skill", Estado: estHecho},
			{Nombre: "Pedir un café", Tipo: "skill", Estado: estHecho},
			{Nombre: "Historia: En la cafetería", Tipo: "story", Estado: estHecho},
			{Nombre: "Cofre", Tipo: "chest", Estado: estHecho},
			{Nombre: "Repaso de la unidad", Tipo: "unit_review", Estado: estHecho},
		}},
		{Indice: 1, Seccion: "Sección 1", Titulo: "Unidad 2", Tema: "Describe a tu familia", Niveles: []nivel{
			{Nombre: "La familia", Tipo: "skill", Estado: estHecho, Peticion: peticion{tipo: "nivel"}},
			{Nombre: "Tus hermanos", Tipo: "skill", Estado: estActivo, Hechas: 2, Total: 4, Peticion: peticion{tipo: "nivel"}},
			{Nombre: "Cofre", Tipo: "chest", Estado: estBloqueado},
			{Nombre: "Historia: La boda", Tipo: "story", Estado: estBloqueado},
			{Nombre: "Los abuelos", Tipo: "skill", Estado: estBloqueado},
			{Nombre: "Repaso de la unidad", Tipo: "unit_review", Estado: estBloqueado},
		}},
	}
	a.unidad, a.nivelSel = 1, 1
	a.pant = pInicio
	if err := guardar("03-camino"); err != nil {
		return err
	}
	a.nivelSel = 4
	if err := guardar("04-camino-bloqueado"); err != nil {
		return err
	}
	a.pant = pPerfil
	if err := guardar("05-perfil"); err != nil {
		return err
	}
	a.pant = pCargando
	if err := guardar("06-preparando"); err != nil {
		return err
	}

	ejs := []*ejercicio{
		{Tipo: ejElegir, Clase: "select", Instruccion: "¿Cuál de estas es «el café»?", Opciones: []opcion{
			{Texto: "the tea"}, {Texto: "the coffee"}, {Texto: "the milk"}, {Texto: "the water"}}, Correcta: 1},
		{Tipo: ejBanco, Clase: "translate", Instruccion: "Traduce esta frase", Frase: "My brother is very tall.", Audio: "x",
			Opciones:   []opcion{{Texto: "Mi"}, {Texto: "alto"}, {Texto: "es"}, {Texto: "hermano"}, {Texto: "muy"}, {Texto: "hermana"}, {Texto: "baja"}, {Texto: "su"}},
			Soluciones: []string{"Mi hermano es muy alto"}, Mostrar: "Mi hermano es muy alto."},
		{Tipo: ejBanco, Clase: "listenTap", Instruccion: "Escribe lo que oyes", SoloOir: true, Audio: "x", AudioLento: "x",
			Opciones:   []opcion{{Texto: "She"}, {Texto: "has"}, {Texto: "two"}, {Texto: "sisters"}, {Texto: "brothers"}, {Texto: "three"}},
			Soluciones: []string{"She has two sisters"}, Mostrar: "She has two sisters."},
		{Tipo: ejParejas, Clase: "match", Instruccion: "Empareja las palabras",
			Izquierda: []opcion{{Texto: "mother"}, {Texto: "father"}, {Texto: "sister"}, {Texto: "grandmother"}, {Texto: "son"}},
			Derecha:   []opcion{{Texto: "madre"}, {Texto: "padre"}, {Texto: "hermana"}, {Texto: "abuela"}, {Texto: "hijo"}}},
		{Tipo: ejHuecos, Clase: "tapComplete", Instruccion: "Completa la frase", Audio: "x",
			Piezas:   []pieza{{Texto: "My "}, {Hueco: true}, {Texto: " is a "}, {Hueco: true}, {Texto: "."}},
			Opciones: []opcion{{Texto: "father"}, {Texto: "doctor"}, {Texto: "eats"}, {Texto: "red"}}, Correctas: []int{0, 1}},
		{Tipo: ejEscribir, Clase: "translate", Instruccion: "Escribe esto en inglés", Frase: "Tengo dos hermanos.", Idioma: "en",
			Soluciones: []string{"I [have/'ve got] two brothers"}, Mostrar: "I have two brothers."},
		{Tipo: ejElegir, Clase: "gapFill", Instruccion: "Elige la palabra que falta",
			Piezas:   []pieza{{Texto: "Her "}, {Hueco: true}, {Texto: " is my cousin."}},
			Opciones: []opcion{{Texto: "son"}, {Texto: "sun"}, {Texto: "soon"}}, Correcta: 0},
	}
	a.lec = nuevaLeccion(nil, ejs, 4, "Unidad 2 · Tus hermanos")
	a.pant = pLeccion
	paso := func(nombre string, preparar func()) error {
		a.nuevoTurno()
		if preparar != nil {
			preparar()
		}
		return guardar(nombre)
	}
	// 1: elegir, con una marcada
	if err := paso("07-elegir", func() { a.t.foco, a.t.elegida = 1, 1 }); err != nil {
		return err
	}
	a.comprobar()
	if err := guardar("08-elegir-bien"); err != nil {
		return err
	}
	a.continuar()
	// 2: banco, a medias y luego mal
	a.t.fichas, a.t.foco = []int{0, 3, 2}, 4
	if err := guardar("09-banco"); err != nil {
		return err
	}
	a.t.fichas = []int{0, 5, 2, 4, 1}
	a.comprobar()
	if err := guardar("10-banco-mal"); err != nil {
		return err
	}
	a.continuar()
	// 3: escuchar
	if err := guardar("11-escuchar"); err != nil {
		return err
	}
	a.t.fichas = []int{0, 1, 2, 3}
	a.comprobar()
	a.continuar()
	// 4: parejas, con dos hechas y una elegida
	a.t.hechas[0], a.t.hechas[2] = true, true
	a.t.selIzq, a.t.col, a.t.foco = 1, 1, 2
	if err := guardar("12-parejas"); err != nil {
		return err
	}
	a.t.comprobado, a.t.ver = true, veredicto{Bien: true}
	a.lec.responder(true)
	a.continuar()
	// 5: huecos
	a.t.fichas, a.t.foco = []int{0}, 1
	if err := guardar("13-huecos"); err != nil {
		return err
	}
	a.t.fichas = append(a.t.fichas, 1)
	a.comprobar()
	a.continuar()
	// 6: escribir
	for _, r := range "I have two brothrs" {
		a.tec.escribir(r)
	}
	if err := guardar("14-escribir"); err != nil {
		return err
	}
	a.comprobar()
	if err := guardar("15-escribir-casi"); err != nil {
		return err
	}
	a.continuar()
	// 7: hueco con opciones
	a.t.foco = 2
	if err := guardar("16-hueco-elegir"); err != nil {
		return err
	}

	a.pant, a.enviado = pFin, false
	if err := guardar("17-fin-mandando"); err != nil {
		return err
	}
	a.enviado, a.fin = true, resultadoFin{XP: 15, Racha: 14, Hora: a.lec.inicio.Add(3*time.Minute + 12*time.Second)}
	if err := guardar("18-fin"); err != nil {
		return err
	}
	a.pant = pSinVidas
	if err := guardar("19-sin-vidas"); err != nil {
		return err
	}
	a.pant = pLeccion
	a.confirmar("¿Salir de la lección? Lo que llevas no contará.", func() {})
	return guardar("20-salir")
}
