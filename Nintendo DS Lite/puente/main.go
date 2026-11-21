// Puente de Panther OS para la Nintendo DS Lite: hace el trabajo de las apps de la DS, que no puede
// con WhatsApp ni con lo demás (4 MB, 67 MHz y un wifi de 2005). La DS le pide lo justo para
// enseñarlo, por el wifi. Va en un aparato de la dueña de la DS (con sus cuentas), en el mismo wifi.
//
//   - La DS lo busca mandando «PANTHER-DS 1» por UDP a toda la red (puerto 8790) y aquí se contesta
//     «PANTHER-PUENTE 1 <nombre>».
//   - Luego se conecta por TCP al mismo puerto. Como el wifi de la DS va sin contraseña, hace falta
//     el código de 6 cifras de este puente (<datos>/codigo, sale en el registro al arrancar) y todo
//     va cifrado (cifrado.go).
//   - Se habla con líneas de texto: cada respuesta es «R <n>» y n líneas (protocolo.go).
//   - WhatsApp (wa_real.go): su propia sesión, vinculada desde la DS con el número de teléfono.
//
// Uso: puente-ds [-datos carpeta] [-puerto 8790] [-codigo 123456] [-nombre texto] [-dns ip:53,…]
//
//	[-probar-red] [-falso [-sin-vincular]]
//	-falso         WhatsApp de mentira (para las pruebas de la DS en el PC)
//	-sin-vincular  con -falso, empieza sin vincular (para probar la vinculación)
//	-dns           los servidores DNS (en Android no hay /etc/resolv.conf: los pasa la app)
//	-probar-red    al arrancar, mira si se llega a WhatsApp (DNS y TLS) y lo dice en el registro
//
// En Android lo arranca la app «Puente DS» (android/), que lo lleva como libpuente.so.
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata" // en Android no está la base de husos horarios: va dentro (y TZ la pone la app)

	// Los certificados para hablar con WhatsApp por TLS, dentro: en Android no están donde Go los busca
	// (y en un móvil antiguo pueden estar caducados). Solo se usan si no encuentra los del sistema.
	_ "golang.org/x/crypto/x509roots/fallback"
)

const version = "1"

func main() {
	datos := flag.String("datos", carpetaPorDefecto(), "carpeta de la sesión y el código")
	puerto := flag.Int("puerto", 8790, "puerto (UDP para buscarlo y TCP para hablar)")
	codigo := flag.String("codigo", "", "código de 6 cifras (si no, el guardado o uno nuevo)")
	nombre := flag.String("nombre", "Puente de Panther", "cómo sale en la DS")
	falso := flag.Bool("falso", false, "WhatsApp de mentira")
	sinVincular := flag.Bool("sin-vincular", false, "con -falso, empieza sin vincular")
	dns := flag.String("dns", "", "servidores DNS (ip:puerto separados por comas)")
	probarRed := flag.Bool("probar-red", false, "mirar al arrancar si se llega a WhatsApp")
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime)
	if *dns != "" {
		usarDNS(strings.Split(*dns, ","))
	}
	if *probarRed {
		go comprobarRed()
	}

	c := *codigo
	if c == "" {
		var err error
		if c, err = codigoGuardado(*datos); err != nil {
			log.Fatalf("código: %v", err)
		}
	}
	var wa whatsapp
	if *falso {
		wa = nuevoWAFalso(!*sinVincular)
	} else {
		r, err := nuevoWAReal(filepath.Join(*datos, "whatsapp"))
		if err != nil {
			log.Fatalf("WhatsApp: %v", err)
		}
		wa = r
	}
	p := &puente{codigo: c, nombre: *nombre, wa: wa, fallos: map[string]*intentos{}}
	log.Printf("puente %s en el puerto %d: «%s», código %s", version, *puerto, p.nombre, c)
	go func() {
		if err := p.contestarBusquedas(*puerto); err != nil {
			log.Fatalf("UDP: %v", err)
		}
	}()
	if err := p.escuchar(*puerto); err != nil {
		log.Fatalf("TCP: %v", err)
	}
}

func carpetaPorDefecto() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "datos-puente"
	}
	return filepath.Join(h, ".config", "panther", "puente-ds")
}

// codigoGuardado: el código del puente; la primera vez se hace uno al azar.
func codigoGuardado(dir string) (string, error) {
	ruta := filepath.Join(dir, "codigo")
	if b, err := os.ReadFile(ruta); err == nil {
		if c := strings.TrimSpace(string(b)); len(c) == 6 {
			return c, nil
		}
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	c := fmt.Sprintf("%06d", n.Int64())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return c, os.WriteFile(ruta, []byte(c+"\n"), 0o600)
}

// usarDNS: los servidores DNS que se le dicen (en Android no hay /etc/resolv.conf).
func usarDNS(servidores []string) {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			var ultimo error
			for _, s := range servidores {
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				if !strings.Contains(s, ":") || strings.Count(s, ":") > 1 && !strings.HasPrefix(s, "[") {
					s = net.JoinHostPort(s, "53")
				}
				c, err := d.DialContext(ctx, network, s)
				if err == nil {
					return c, nil
				}
				ultimo = err
			}
			return nil, ultimo
		},
	}
}

// comprobarRed: si se llega a WhatsApp (el DNS y los certificados del sistema), en el registro.
func comprobarRed() {
	for intento := 0; intento < 5; intento++ {
		c, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", "web.whatsapp.com:443", nil)
		if err == nil {
			c.Close()
			log.Printf("RED OK: se llega a WhatsApp")
			return
		}
		log.Printf("RED MAL: %v", err)
		time.Sleep(10 * time.Second)
	}
}
