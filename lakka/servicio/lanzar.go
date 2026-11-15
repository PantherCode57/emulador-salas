package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

const (
	puertoSala3DS = 24872
	puertoCableGB = 56400
)

// carpetaPerfil: la partida de siempre o la propia de una sala (cada una con sus guardados).
func carpetaPerfil(s *Sala) string {
	if s != nil && s.PartidaPropia {
		return filepath.Join(carpetaDatos, "partidas", "sala-"+strings.ToLower(s.Codigo))
	}
	return filepath.Join(carpetaDatos, "partidas", "siempre")
}

func carpetaSistema() string  { return filepath.Join(carpetaDatos, "sistema") }
func carpetaCapturas() string { return filepath.Join(carpetaDatos, "capturas") }

// prepararJuego deja todo listo para abrir el juego y devuelve lo que hay que pasarle a RetroArch.
// modo: nil sin sala (o sin saber aún quién la abre), true si tenemos la sala abierta, false si
// es la de un amigo.
func prepararJuego(j Juego, s *Sala, modo *bool, jug *Jugador) (orden, error) {
	if _, ok := sistemas[j.Sistema]; !ok {
		return orden{}, fmt.Errorf("no sé abrir este juego")
	}
	if !existe(j.Ruta) {
		return orden{}, fmt.Errorf("no encuentro %s en la memoria", j.Relativa)
	}
	perfil := carpetaPerfil(s)
	guardados := filepath.Join(perfil, "guardados")
	estados := filepath.Join(perfil, "estados")
	for _, c := range []string{guardados, estados, carpetaSistema(), carpetaCapturas(), filepath.Join(carpetaDatos, "opciones")} {
		_ = os.MkdirAll(c, 0o755)
	}
	importarPartida(j, s, guardados)

	// GBA: partida compartida (GBA-PK) solo desde una sala con los papeles repartidos.
	script := filepath.Join(carpetaSistema(), "pokemulador", "multijugador.lua")
	if j.Sistema == "gba" && modo != nil {
		if err := escribirGbaPk(script, *modo, jug.Nombre); err != nil {
			log.Printf("GBA-PK: %v", err)
		}
	} else {
		_ = os.Remove(script)
	}

	// Opciones de los núcleos: un archivo por sistema, con lo que toque de la sala.
	opciones := filepath.Join(carpetaDatos, "opciones", j.Sistema+".opt")
	cambios := map[string]string{}
	if j.Sistema == "gb" || j.Sistema == "gbc" {
		cambios["gambatte_gb_link_mode"] = "Not Connected"
		if modo != nil {
			if *modo {
				cambios["gambatte_gb_link_mode"] = "Network Server"
			} else {
				cambios["gambatte_gb_link_mode"] = "Network Client"
			}
		}
		cambios["gambatte_gb_link_network_port"] = fmt.Sprint(puertoCableGB)
		for i, c := range "127000000001" {
			cambios[fmt.Sprintf("gambatte_gb_link_network_server_ip_%d", i+1)] = string(c)
		}
	}
	fijarOpciones(opciones, cambios)

	var env []string
	if j.Sistema == "3ds" && modo != nil && s != nil {
		m := "join"
		if *modo {
			m = "host"
		}
		env = append(env,
			"POKEMULADOR_SALA_MODO="+m,
			fmt.Sprintf("POKEMULADOR_SALA_PUERTO=%d", puertoSala3DS),
			"POKEMULADOR_SALA_CLAVE="+strings.ToLower(s.Codigo),
			"POKEMULADOR_SALA_NOMBRE="+jug.Nombre)
	}

	cfg := filepath.Join(carpetaDatos, "juego.cfg")
	ajustes := [][2]string{
		{"savefile_directory", guardados},
		{"savestate_directory", estados},
		{"system_directory", carpetaSistema()},
		{"screenshot_directory", carpetaCapturas()},
		{"savestate_auto_save", "true"},
		{"savestate_auto_load", "true"},
		{"sort_savefiles_enable", "false"},
		{"sort_savestates_enable", "false"},
		{"sort_savefiles_by_content_enable", "false"},
		{"sort_savestates_by_content_enable", "false"},
		{"quit_on_close_content", "1"},
		{"network_cmd_enable", "true"},
		{"network_cmd_port", "55355"},
		{"global_core_options", "true"},
		{"core_options_path", opciones},
		{"config_save_on_exit", "false"},
		{"pause_nonactive", "false"},
		{"autosave_interval", "60"},
	}
	var b strings.Builder
	for _, a := range ajustes {
		fmt.Fprintf(&b, "%s = \"%s\"\n", a[0], a[1])
	}
	escribirAtomico(cfg, []byte(b.String()))

	return orden{
		args: argsRetroArch("-L", filepath.Join(carpetaCores, nucleoDe(j.Sistema)), j.Ruta, "--appendconfig", cfg),
		env:  env,
	}, nil
}

// importarPartida: si junto al juego hay un .sav con el mismo nombre (traído de linkcable, del PC
// o del móvil) y aún no tenemos partida de siempre, se usa esa.
func importarPartida(j Juego, s *Sala, guardados string) {
	if s != nil && s.PartidaPropia {
		return
	}
	if j.Sistema != "gb" && j.Sistema != "gbc" && j.Sistema != "gba" {
		return
	}
	nombre := strings.TrimSuffix(filepath.Base(j.Ruta), filepath.Ext(j.Ruta))
	destino := filepath.Join(guardados, nombre+".srm")
	if existe(destino) {
		return
	}
	for _, ext := range []string{".sav", ".srm", ".SAV", ".SRM"} {
		origen := filepath.Join(filepath.Dir(j.Ruta), nombre+ext)
		if datos, err := os.ReadFile(origen); err == nil && len(datos) > 0 {
			escribirAtomico(destino, datos)
			log.Printf("partida importada de %s", origen)
			return
		}
	}
}

// escribirGbaPk prepara el script de GBA-PK que carga el mGBA de Pokemulador.
func escribirGbaPk(destino string, anfitrion bool, nombre string) error {
	base, err := os.ReadFile(filepath.Join(carpetaPropia, "GBA-PK_Multiplayer.lua"))
	if err != nil {
		return err
	}
	final, err := os.ReadFile(filepath.Join(carpetaPropia, "pokemulador_final.lua"))
	if err != nil {
		return err
	}
	modo := "c"
	if anfitrion {
		modo = "h"
	}
	var limpio []rune
	for _, r := range nombre {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' {
			limpio = append(limpio, r)
		}
	}
	apodo := strings.TrimSpace(string(limpio))
	if len(apodo) > 10 {
		apodo = strings.TrimSpace(apodo[:10])
	}
	if apodo == "" {
		apodo = "Jugador"
	}
	texto := strings.Replace(string(base), `local Nickname = ""`, `local Nickname = "`+apodo+`"`, 1) +
		"\n" + strings.ReplaceAll(string(final), "__MODO__", modo)
	escribirAtomico(destino, []byte(texto))
	return nil
}

// fijarOpciones cambia unas claves del archivo de opciones de los núcleos sin tocar el resto.
func fijarOpciones(ruta string, cambios map[string]string) {
	var lineas []string
	if f, err := os.Open(ruta); err == nil {
		s := bufio.NewScanner(f)
		for s.Scan() {
			l := s.Text()
			clave := strings.TrimSpace(strings.SplitN(l, "=", 2)[0])
			if _, es := cambios[clave]; es {
				continue
			}
			lineas = append(lineas, l)
		}
		f.Close()
	}
	for k, v := range cambios {
		lineas = append(lineas, fmt.Sprintf("%s = \"%s\"", k, v))
	}
	escribirAtomico(ruta, []byte(strings.Join(lineas, "\n")+"\n"))
}

// capturar pide una captura a RetroArch y la devuelve en JPEG pequeño (base64), como el PC.
func capturar() string {
	antes := map[string]bool{}
	previas, _ := filepath.Glob(filepath.Join(carpetaCapturas(), "*.png"))
	for _, p := range previas {
		antes[p] = true
	}
	comandoRetroArch("SCREENSHOT")
	var nueva string
	for i := 0; i < 30 && nueva == ""; i++ {
		time.Sleep(200 * time.Millisecond)
		ahora, _ := filepath.Glob(filepath.Join(carpetaCapturas(), "*.png"))
		for _, p := range ahora {
			if !antes[p] {
				nueva = p
			}
		}
	}
	for _, p := range previas {
		_ = os.Remove(p)
	}
	if nueva == "" {
		return ""
	}
	time.Sleep(300 * time.Millisecond) // que termine de escribirla
	defer os.Remove(nueva)
	f, err := os.Open(nueva)
	if err != nil {
		return ""
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return ""
	}
	return jpegBase64(img, 480)
}

func jpegBase64(img image.Image, lado int) string {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > lado || h > lado {
		if w >= h {
			w, h = lado, h*lado/w
		} else {
			w, h = w*lado/h, lado
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	var buf bytes.Buffer
	if jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 70}) != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodificarCaptura(b64 string) image.Image {
	datos, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(io.NopCloser(bytes.NewReader(datos)))
	if err != nil {
		return nil
	}
	return img
}
