package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

// Abrir un juego (de lakka/servicio/lanzar.go): la partida, el script de la partida compartida de
// GBA (GBA-PK) y los Caramelos Raros, las opciones del cable link de Game Boy y RetroArch.

const puertoCableGB = 56400

func carpetaPerfil(s *Sala) string {
	if s != nil && s.PartidaPropia {
		return filepath.Join(carpetaDatos, "partidas", "sala-"+strings.ToLower(s.Codigo))
	}
	return filepath.Join(carpetaDatos, "partidas", "siempre")
}

func carpetaSistema() string  { return filepath.Join(carpetaDatos, "sistema") }
func carpetaCapturas() string { return filepath.Join(carpetaDatos, "capturas") }

// esRojoFuego: Rojo Fuego o Verde Hoja (donde valen los Caramelos Raros infinitos).
func esRojoFuego(j Juego) bool {
	b := strings.ToLower(j.base())
	return j.Sistema == "gba" && (strings.Contains(b, "firered") || strings.Contains(b, "leafgreen") ||
		strings.Contains(b, "rojo fuego") || strings.Contains(b, "verde hoja"))
}

type orden struct {
	args, env  []string
	juego      Juego
	sala       *Sala
	guardados  string
	devolverA  string // a dónde vuelve la partida de siempre al terminar
	conGbaPk   bool
	conCaramel bool
}

// prepararJuego deja todo listo para abrir el juego. modo: nil sin sala, true si tenemos la sala
// abierta, false si es la de un amigo.
func prepararJuego(j Juego, s *Sala, modo *bool, jug *Jugador, caramelos bool) (orden, error) {
	if _, ok := sistemas[j.Sistema]; !ok {
		return orden{}, fmt.Errorf("no sé abrir este juego")
	}
	if !existe(j.Ruta) {
		return orden{}, fmt.Errorf("no encuentro %s", j.Relativa)
	}
	nucleo, gbapk := nucleoDe(j.Sistema)
	if !existe(nucleo) {
		return orden{}, fmt.Errorf("falta el emulador (%s)", filepath.Base(nucleo))
	}
	perfil := carpetaPerfil(s)
	guardados := filepath.Join(perfil, "guardados")
	estados := filepath.Join(perfil, "estados")
	for _, c := range []string{guardados, estados, carpetaSistema(), carpetaCapturas(), filepath.Join(carpetaDatos, "opciones"),
		filepath.Join(carpetaSistema(), "pokemulador")} {
		_ = os.MkdirAll(c, 0o755)
	}
	o := orden{juego: j, sala: s, guardados: guardados}
	if s == nil || !s.PartidaPropia {
		o.devolverA = traerPartida(j, guardados)
	}

	// GBA: lo que carga el mGBA de Pokemulador al abrir el juego.
	script := filepath.Join(carpetaSistema(), "pokemulador", "multijugador.lua")
	var lua strings.Builder
	if j.Sistema == "gba" && gbapk && modo != nil {
		if texto, err := textoGbaPk(*modo, jug.Nombre); err != nil {
			logf("GBA-PK: %v", err)
		} else {
			lua.WriteString(texto)
			o.conGbaPk = true
		}
	}
	if gbapk && caramelos && esRojoFuego(j) {
		if b, err := os.ReadFile(filepath.Join(carpetaPrograma, "caramelos-frlg.lua")); err == nil {
			lua.WriteString("\n")
			lua.Write(b)
			o.conCaramel = true
		} else {
			logf("caramelos: %v", err)
		}
	}
	if lua.Len() > 0 {
		_ = escribirAtomico(script, []byte(lua.String()))
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

	cfg := filepath.Join(carpetaDatos, "juego.cfg")
	ajustes := [][2]string{
		{"savefile_directory", guardados},
		{"savestate_directory", estados},
		{"system_directory", carpetaSistema()},
		{"screenshot_directory", carpetaCapturas()},
		{"savestate_auto_save", "false"},
		{"savestate_auto_load", "false"},
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
		{"audio_enable", "true"},
		{"audio_mute_enable", "false"},
	}
	var b strings.Builder
	for _, a := range ajustes {
		fmt.Fprintf(&b, "%s = \"%s\"\n", a[0], a[1])
	}
	_ = escribirAtomico(cfg, []byte(b.String()))
	o.args = []string{"-L", nucleo, j.Ruta, "--appendconfig", cfg}
	return o, nil
}

// textoGbaPk: el script de GBA-PK con nuestro apodo y, al final, cómo entra en la partida.
func textoGbaPk(anfitrion bool, nombre string) (string, error) {
	base, err := os.ReadFile(filepath.Join(carpetaPrograma, "GBA-PK_Multiplayer.lua"))
	if err != nil {
		return "", err
	}
	final, err := os.ReadFile(filepath.Join(carpetaPrograma, "pokemulador_final.lua"))
	if err != nil {
		return "", err
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
	return strings.Replace(string(base), `local Nickname = ""`, `local Nickname = "`+apodo+`"`, 1) +
		"\n" + strings.ReplaceAll(string(final), "__MODO__", modo), nil
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
	_ = escribirAtomico(ruta, []byte(strings.Join(lineas, "\n")+"\n"))
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
