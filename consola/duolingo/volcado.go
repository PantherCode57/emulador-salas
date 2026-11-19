package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// volcar (-volcar <archivo>): para preparar los cursos que no son de idiomas (ajedrez, matemáticas,
// música). Guarda en un archivo, con la sesión de la consola, cómo es tu curso (su camino) y cómo
// llega una lección: la pide pero NO la manda, así que no cuenta ni cambia nada. No guarda nada
// tuyo (ni nombre ni usuario ni foto): solo los idiomas, la lista de cursos y el curso de ahora.
func volcar(ruta string) {
	c := cargarCuenta(filepath.Join(carpetaDatos, "cuenta.json"))
	if !c.tiene() {
		fmt.Println("no hay sesión guardada en", carpetaDatos)
		return
	}
	c.ruta = ""
	d := nuevoDuolingo(c)
	_, id := c.datos()
	out := map[string]any{}
	limpio := func(m map[string]any) map[string]any {
		r := map[string]any{}
		for _, k := range []string{"currentCourseId", "fromLanguage", "learningLanguage", "courses", "currentCourse"} {
			if v, ok := m[k]; ok {
				r[k] = v
			}
		}
		return r
	}
	for _, v := range []string{"2017-06-30", "2023-05-23"} {
		var m map[string]any
		err := d.pedir("GET", fmt.Sprintf("/%s/users/%d?fields=currentCourseId,fromLanguage,learningLanguage,courses,currentCourse", v, id), nil, &m)
		fmt.Printf("usuario %s: %s, campos %d\n", v, siError(err), len(m))
		out["usuario-"+v] = limpio(m)
	}
	u, _ := out["usuario-2023-05-23"].(map[string]any)
	curso := obj(u, "currentCourse")
	cursoID := texto(u, "currentCourseId")
	fmt.Printf("curso de ahora: %q, con camino: %v\n", cursoID, lista(curso, "pathSectioned") != nil)
	// el primer nivel activo, y cómo se pediría (como en un curso de idiomas)
	var pet peticion
	for _, s := range lista(curso, "pathSectioned") {
		for _, un := range lista(s, "units") {
			for li, lv := range lista(un, "levels") {
				if pet.tipo == "" && (texto(lv, "state") == "active" || texto(lv, "state") == "accessible") {
					n := nivelDe(lv, li, texto(un, "teachingObjective"), texto(curso, "treeId"), cursoID)
					pet = n.Peticion
					out["nivel"] = lv
					fmt.Printf("nivel activo: tipo %q, estado %q\n", texto(lv, "type"), texto(lv, "state"))
				}
			}
		}
	}
	tipos := append(append([]string{}, tiposPedidos...), "star", "normal", "fictional_board", "chessMatch",
		"chessPvpMatch", "chessMiniMatch", "mathChallengeBlob", "mathProductSelect", "mathMatch", "mathTypeFill")
	base := map[string]any{
		"challengeTypes": tipos, "fromLanguage": texto(u, "fromLanguage"), "learningLanguage": texto(u, "learningLanguage"),
		"isCustomIntroSkill": false, "isFinalLevel": false, "isGrammarSkill": false, "isRedoingPassedNode": false,
		"isV2": true, "juicy": true, "pathExperiments": experimentosCamino, "pathLevelSessionMetadata": map[string]any{},
		"shakeToReportEnabled": true, "showGrammarSkillSplash": false, "smartTipsVersion": 2, "type": "GLOBAL_PRACTICE",
	}
	for k, v := range pet.cuerpo {
		base[k] = v
	}
	variantes := []map[string]any{base, conCampos(base, map[string]any{"courseId": cursoID}),
		conCampos(base, map[string]any{"courseId": cursoID, "learningLanguage": nil})}
	var intentos []any
	for i, cuerpo := range variantes {
		codigo, datos, err := d.pedirCrudo("POST", "/2017-06-30/sessions", cuerpo)
		fmt.Printf("lección (prueba %d): %d %s\n", i+1, codigo, siError(err))
		in := map[string]any{"cuerpo": cuerpo, "codigo": codigo}
		var r map[string]any
		if leerJSON(datos, &r) == nil {
			in["respuesta"] = r
			cuenta := map[string]int{}
			for _, ch := range lista(r, "challenges") {
				cuenta[texto(ch, "type")]++
			}
			fmt.Printf("   ejercicios: %v\n", cuenta)
		} else if len(datos) > 0 {
			in["texto"] = string(datos[:min(len(datos), 2000)])
		}
		intentos = append(intentos, in)
		if codigo == 200 {
			break
		}
	}
	out["lecciones"] = intentos
	b, _ := json.MarshalIndent(out, "", " ")
	if err := os.WriteFile(ruta, b, 0o600); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("guardado en", ruta, len(b), "bytes")
}

// conCampos: una copia del mapa con esos campos cambiados (nil quita el campo).
func conCampos(m map[string]any, cambios map[string]any) map[string]any {
	r := map[string]any{}
	for k, v := range m {
		r[k] = v
	}
	for k, v := range cambios {
		if v == nil {
			delete(r, k)
		} else {
			r[k] = v
		}
	}
	return r
}
