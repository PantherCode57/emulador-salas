//go:build android

package main

// En Android el sistema no deja a las apps listar las interfaces de red ni leer la
// configuración DNS como en Linux normal. Este módulo de Tailscale lo resuelve.
import _ "tailscale.com/feature/androidbin"
