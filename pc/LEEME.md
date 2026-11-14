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

## Cómo se fabrica

Hace falta Rust, Node.js y las herramientas de compilación de Visual Studio (C++).

```powershell
cd pc
powershell -ExecutionPolicy Bypass -File .\preparar-emuladores.ps1   # descarga RetroArch, núcleos y Azahar
npm install
npx tauri build                                                     # crea el .exe y el instalador
```

- Programa: `src-tauri/target/release/pokemulador.exe`
- Instalador: `src-tauri/target/release/bundle/nsis/Pokemulador_0.1.0_x64-setup.exe`

## Partes

- `ui/`: la interfaz (HTML, CSS y JavaScript, sin librerías). Abriendo `ui/index.html`
  en un navegador se ve con juegos de ejemplo.
- `src-tauri/src/main.rs`: busca los juegos, guarda recientes y favoritos, y abre los emuladores.
- `config/pokemulador.cfg`: ajustes que se aplican a RetroArch (idioma, guardado automático…).
