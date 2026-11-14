# Pokemulador para PC

Programa para Windows con el mismo aspecto Pokédex que la app de Android. Muestra tu
biblioteca de juegos y abre cada uno con el emulador que le toca:

| Consola | Emulador |
|---|---|
| Game Boy y Game Boy Color | RetroArch + Gambatte |
| Game Boy Advance | RetroArch + mGBA |
| Nintendo DS | RetroArch + melonDS DS |
| Nintendo 64 | RetroArch + Mupen64Plus-Next |
| Nintendo 3DS | Azahar |

Los juegos no vienen incluidos: cada uno usa los suyos.

## Salas online (Tailscale dentro de la app)

"Crear sala" da un código de 6 letras. Pokemulador arranca el servidor de salas de Azahar
(`azahar-room.exe`) y el **enlace** (`enlace/`, Go + tsnet), que lo publica en una red privada
de Tailscale con el nombre `pkm-<código>`. El amigo escribe el código, su enlace busca ese
nombre y abre `127.0.0.1:24872` para su Azahar. Funciona con Wi-Fi o datos; si no hay conexión
directa, Tailscale usa sus repetidores.

La clave es un secreto OAuth de Tailscale (`tskey-client-…`) con permiso *Auth Keys: Write* y la
etiqueta `tag:pokemulador`. La política de la red está en `config/tailscale-politica.json`.
**La clave nunca va al repositorio**: se guarda en `secretos/tailscale.txt` (ignorado por git)
y se mete en el programa al compilar. También se puede poner en
`%APPDATA%\com.panthercode.pokemulador\tailscale.txt` para cambiarla sin recompilar.

## Cómo se fabrica

Hace falta Rust, Node.js, Go y las herramientas de compilación de Visual Studio (C++).
En `D:` (exFAT) hay que compilar con pocos trabajos a la vez o falla con "recursos insuficientes".

```powershell
cd pc
powershell -ExecutionPolicy Bypass -File .\preparar-emuladores.ps1   # RetroArch, núcleos, Azahar y el enlace
npm install
$env:POKEMULADOR_TS_KEY = (Get-Content secretos\tailscale.txt -Raw).Trim()
$env:CARGO_BUILD_JOBS = 2
npx tauri build                                                     # crea el .exe y el instalador
```

- Programa: `src-tauri/target/release/pokemulador.exe`
- Instalador: `src-tauri/target/release/bundle/nsis/Pokemulador_0.1.0_x64-setup.exe`

## Partes

- `ui/`: la interfaz (HTML, CSS y JavaScript, sin librerías). Abriendo `ui/index.html`
  en un navegador se ve con juegos de ejemplo.
- `src-tauri/src/main.rs`: busca los juegos, guarda recientes y favoritos, y abre los emuladores.
- `config/pokemulador.cfg`: ajustes que se aplican a RetroArch (idioma, guardado automático…).
