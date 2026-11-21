package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Dar de alta en EmulationStation las apps que llegan por la tienda. Las de la instalación
// (WhatsApp, Claude… y la propia tienda) las dieron de alta menus.py, ajustes_menu.py y tema.py
// por SSH; las que llegan después (la Pokédex) solo tienen la tienda, así que al ponerlas la tienda
// hace lo mismo que esos scripts:
//
//   - su apartado en es_systems.cfg (y en es_systems.sd1), con sudo: son de root;
//   - su tarjeta de la pantalla principal (la dibuja la tienda con su color, su icono y su nombre)
//     y el theme.xml de su apartado en el tema;
//   - en es_settings.cfg, que se abra directamente (PantherDirectLaunch) y su sitio en la pantalla
//     principal (PantherSystemOrder). Esto solo se puede escribir con EmulationStation parado (al
//     cerrarse guarda sus ajustes encima): lo hace «tienda -reiniciar-es» entre pararlo y
//     arrancarlo (ver reinicio_linux.go).
//
// Con las apps que ya estaban dadas de alta no cambia nada. Al quitar una app no se borra nada de
// esto: sin su .sh, EmulationStation no enseña el apartado.

// bloqueSistema: el apartado de una app, como los de menus.py.
func bloqueSistema(e *elemento) string {
	s := e.app.sistema
	return "\t<system>\n" +
		"\t\t<name>" + s + "</name>\n" +
		"\t\t<fullname>" + e.nombre + "</fullname>\n" +
		"\t\t<path>/roms/" + s + "/</path>\n" +
		"\t\t<extension>.sh .SH</extension>\n" +
		"\t\t<command>%ROM%</command>\n" +
		"\t\t<platform>" + s + "</platform>\n" +
		"\t\t<theme>" + s + "</theme>\n" +
		"\t\t<manufacturer>Panther OS</manufacturer>\n" +
		"\t\t<release>2026</release>\n" +
		"\t</system>\n"
}

var reListaSistemas = regexp.MustCompile(`<systemList[^>]*>[ \t]*\r?\n`)

// conSistema: es_systems.cfg con el apartado de la app (si ya estaba, igual y false).
func conSistema(texto string, e *elemento) (string, bool, error) {
	if strings.Contains(texto, "<name>"+e.app.sistema+"</name>") {
		return texto, false, nil
	}
	m := reListaSistemas.FindStringIndex(texto)
	if m == nil {
		return texto, false, errors.New("no se encuentra <systemList>")
	}
	return texto[:m[1]] + bloqueSistema(e) + texto[m[1]:], true, nil
}

// temaApp: el theme.xml del apartado (el logo de la pantalla principal es su tarjeta), como el que
// hace tema.py para los apartados que no tiene el tema de origen.
func temaApp(sistema string) string {
	return "<theme>\n  <formatVersion>7</formatVersion>\n  <include>./../theme.xml</include>\n" +
		"  <view name=\"system\">\n    <image name=\"logo\">\n" +
		"      <path>./../_panther/tarjetas/" + sistema + ".png</path>\n" +
		"    </image>\n  </view>\n</theme>\n"
}

// darDeAlta: lo que necesita EmulationStation para enseñar la app (menos los ajustes, que van al
// reiniciarlo). Solo toca lo que falta.
func (t *tienda) darDeAlta(e *elemento) error {
	a := e.app
	for _, ruta := range t.esSistemas {
		b, err := os.ReadFile(ruta)
		if err != nil {
			continue // no está (es_systems.sd1 no siempre)
		}
		nuevo, cambia, err := conSistema(string(b), e)
		if err != nil {
			return fmt.Errorf("%s: %v", ruta, err)
		}
		if !cambia {
			continue
		}
		if err := escribirDeRoot(ruta, []byte(nuevo)); err != nil {
			return fmt.Errorf("%s: %v", ruta, err)
		}
		logf("alta: apartado %s en %s", a.sistema, ruta)
	}
	if !existe(t.tema) {
		return nil
	}
	tarjeta := filepath.Join(t.tarjetas, a.sistema+".png")
	if !existe(tarjeta) && t.tarjetaApp != nil {
		if err := t.tarjetaApp(e, tarjeta); err != nil {
			return fmt.Errorf("su tarjeta: %v", err)
		}
		logf("alta: tarjeta %s", tarjeta)
	}
	// Una copia en las piezas del tema de ~/panther-instalar: si se vuelve a lanzar tema.py (que
	// hace el tema de nuevo con esas piezas), la tarjeta no se pierde.
	if piezas := filepath.Join(t.local, "tema-panther", "tarjetas"); existe(piezas) && existe(tarjeta) {
		if copia := filepath.Join(piezas, a.sistema+".png"); !existe(copia) {
			_ = copiar(tarjeta, copia, 0o644)
		}
	}
	xml := filepath.Join(t.tema, a.sistema, "theme.xml")
	if !existe(xml) && existe(tarjeta) {
		if err := os.MkdirAll(filepath.Dir(xml), 0o755); err != nil {
			return err
		}
		if err := escribirAtomico(xml, []byte(temaApp(a.sistema)), 0o644); err != nil {
			return err
		}
		logf("alta: %s", xml)
	}
	return nil
}

// escribirDeRoot: un archivo de root (es_systems.cfg). Antes guarda una copia (*.antes-tienda),
// una sola vez.
func escribirDeRoot(ruta string, datos []byte) error {
	if copia := ruta + ".antes-tienda"; !existe(copia) {
		if copiar(ruta, copia, 0o644) != nil {
			_ = exec.Command("sudo", "-n", "cp", "-p", ruta, copia).Run()
		}
	}
	if escribirAtomico(ruta, datos, 0o644) == nil {
		return nil
	}
	tmp := filepath.Join(carpetaDatos, filepath.Base(ruta)+".nuevo")
	if err := os.WriteFile(tmp, datos, 0o644); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if out, err := exec.Command("sudo", "-n", "install", "-m", "644", tmp, ruta).CombinedOutput(); err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ---- es_settings.cfg ----

func reAjuste(nombre string) *regexp.Regexp {
	return regexp.MustCompile(`<string name="` + regexp.QuoteMeta(nombre) + `" value="([^"]*)" />`)
}

// ponerEnLista añade id a la lista separada por comas, detrás del último de «antes» que esté en
// ella; si no hay ninguno, delante de «delante» (o al final).
func ponerEnLista(lista []string, id string, antes []string, delante ...string) []string {
	for _, x := range lista {
		if strings.TrimSpace(x) == id {
			return lista
		}
	}
	donde := -1
	for i, x := range lista {
		for _, a := range antes {
			if strings.TrimSpace(x) == a {
				donde = i + 1
			}
		}
	}
	if donde < 0 {
		donde = len(lista)
		for _, d := range delante {
			for i, x := range lista {
				if strings.TrimSpace(x) == d && i < donde {
					donde = i
				}
			}
			if donde < len(lista) {
				break
			}
		}
	}
	out := append([]string{}, lista[:donde]...)
	out = append(out, id)
	return append(out, lista[donde:]...)
}

// ajustesES: es_settings.cfg con las apps puestas en PantherDirectLaunch y en PantherSystemOrder
// (detrás de la app de antes en el catálogo, o delante de Ports). Solo si el ajuste ya existe (si
// no, ese menú no es el de Panther OS). Devuelve si cambia.
func ajustesES(texto string, apps []*elemento) (string, bool) {
	cambia := false
	for _, ajuste := range []string{"PantherDirectLaunch", "PantherSystemOrder"} {
		re := reAjuste(ajuste)
		m := re.FindStringSubmatchIndex(texto)
		if m == nil {
			continue
		}
		valor := texto[m[2]:m[3]]
		var lista []string
		if valor != "" {
			lista = strings.Split(valor, ",")
		}
		var anteriores []string
		for _, e := range apps {
			if e.puesto {
				lista = ponerEnLista(lista, e.app.sistema, anteriores, "ports", "Emuladores", "tienda")
			}
			anteriores = append(anteriores, e.app.sistema)
		}
		nuevo := strings.Join(lista, ",")
		if nuevo != valor {
			texto = texto[:m[2]] + nuevo + texto[m[3]:]
			cambia = true
		}
	}
	return texto, cambia
}

// ajustesPendientes: si hay que cambiar es_settings.cfg (con lo que está puesto ahora).
func (t *tienda) ajustesPendientes() bool {
	b, err := os.ReadFile(t.esAjustes)
	if err != nil {
		return false
	}
	t.mirar()
	_, cambia := ajustesES(string(b), t.apps)
	return cambia
}

// ponerAjustes escribe es_settings.cfg (con EmulationStation parado).
func (t *tienda) ponerAjustes() error {
	b, err := os.ReadFile(t.esAjustes)
	if err != nil {
		return err
	}
	t.mirar()
	nuevo, cambia := ajustesES(string(b), t.apps)
	if !cambia {
		return nil
	}
	if copia := t.esAjustes + ".antes-tienda"; !existe(copia) {
		_ = copiar(t.esAjustes, copia, 0o644)
	}
	logf("alta: ajustes del menú en %s", t.esAjustes)
	return escribirAtomico(t.esAjustes, []byte(nuevo), 0o644)
}
