// Pokemulador para PC: biblioteca de juegos con aspecto Pokédex que abre cada juego
// con el emulador que le toca (RetroArch para GB/GBC/GBA/DS/N64 y Azahar para 3DS).
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::{Deserialize, Serialize};
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::Mutex;
use tauri::{AppHandle, Emitter, Manager, State};

/// Lo que guardamos entre sesiones (en %APPDATA%\com.panthercode.pokemulador\config.json).
#[derive(Serialize, Deserialize, Clone)]
#[serde(default)]
struct Config {
    roms_dir: Option<String>,
    recents: Vec<String>,
    favorites: Vec<String>,
    fullscreen: bool,
}

impl Default for Config {
    fn default() -> Self {
        Config {
            roms_dir: None,
            recents: Vec::new(),
            favorites: Vec::new(),
            fullscreen: true,
        }
    }
}

#[derive(Serialize, Clone)]
struct Game {
    path: String,
    /// Nombre del archivo sin extensión: sirve para buscar la carátula.
    stem: String,
    /// Nombre bonito, sin "(USA, Europe)" ni "[!]".
    title: String,
    system: String,
}

#[derive(Serialize)]
struct Library {
    roms_dir: Option<String>,
    games: Vec<Game>,
    recents: Vec<String>,
    favorites: Vec<String>,
    fullscreen: bool,
    emulators_found: bool,
}

struct AppData {
    config: Mutex<Config>,
}

fn config_path(app: &AppHandle) -> PathBuf {
    app.path()
        .app_config_dir()
        .unwrap_or_else(|_| PathBuf::from("."))
        .join("config.json")
}

fn load_config(app: &AppHandle) -> Config {
    fs::read_to_string(config_path(app))
        .ok()
        .and_then(|text| serde_json::from_str(&text).ok())
        .unwrap_or_default()
}

fn save_config(app: &AppHandle, config: &Config) {
    let path = config_path(app);
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    if let Ok(text) = serde_json::to_string_pretty(config) {
        let _ = fs::write(path, text);
    }
}

/// Carpeta con RetroArch y Azahar: dentro de la instalación, o `pc/emuladores` mientras se desarrolla.
fn emulators_dir(app: &AppHandle) -> PathBuf {
    if let Ok(resources) = app.path().resource_dir() {
        let installed = resources.join("emuladores");
        if installed.join("RetroArch").exists() {
            return installed;
        }
    }
    Path::new(env!("CARGO_MANIFEST_DIR")).join("..").join("emuladores")
}

/// Consola según la extensión del archivo.
fn system_for(path: &Path) -> Option<&'static str> {
    let ext = path.extension()?.to_str()?.to_ascii_lowercase();
    let system = match ext.as_str() {
        "gb" => "gb",
        "gbc" => "gbc",
        "gba" => "gba",
        "nds" => "nds",
        "3ds" | "cci" | "cxi" | "3dsx" => "3ds",
        "n64" | "z64" | "v64" => "n64",
        _ => return None,
    };
    Some(system)
}

/// Núcleo de RetroArch para cada consola (3DS va aparte, con Azahar).
fn retroarch_core(system: &str) -> Option<&'static str> {
    match system {
        "gb" | "gbc" => Some("gambatte"),
        "gba" => Some("mgba"),
        "nds" => Some("melondsds"),
        "n64" => Some("mupen64plus_next"),
        _ => None,
    }
}

fn clean_title(stem: &str) -> String {
    let mut out = String::new();
    let mut depth = 0u32;
    for c in stem.chars() {
        match c {
            '(' | '[' => depth += 1,
            ')' | ']' => depth = depth.saturating_sub(1),
            _ if depth == 0 => out.push(c),
            _ => {}
        }
    }
    let title = out.split_whitespace().collect::<Vec<_>>().join(" ");
    if title.is_empty() {
        stem.to_string()
    } else {
        title
    }
}

fn scan_dir(dir: &Path, depth: u32, out: &mut Vec<Game>) {
    if depth > 8 {
        return;
    }
    let Ok(entries) = fs::read_dir(dir) else {
        return;
    };
    for entry in entries.flatten() {
        let path = entry.path();
        if path.is_dir() {
            scan_dir(&path, depth + 1, out);
            continue;
        }
        let Some(system) = system_for(&path) else {
            continue;
        };
        let stem = path
            .file_stem()
            .map(|s| s.to_string_lossy().to_string())
            .unwrap_or_default();
        out.push(Game {
            path: path.to_string_lossy().to_string(),
            title: clean_title(&stem),
            stem,
            system: system.to_string(),
        });
    }
}

fn build_library(app: &AppHandle, data: &AppData) -> Library {
    let config = data.config.lock().unwrap().clone();
    let mut games = Vec::new();
    if let Some(dir) = &config.roms_dir {
        scan_dir(Path::new(dir), 0, &mut games);
    }
    games.sort_by_key(|g| g.title.to_lowercase());
    Library {
        roms_dir: config.roms_dir,
        games,
        recents: config.recents,
        favorites: config.favorites,
        fullscreen: config.fullscreen,
        emulators_found: emulators_dir(app).join("RetroArch").join("retroarch.exe").exists(),
    }
}

#[tauri::command]
fn library(app: AppHandle, data: State<AppData>) -> Library {
    build_library(&app, &data)
}

#[tauri::command]
fn set_roms_dir(app: AppHandle, data: State<AppData>, dir: String) -> Library {
    {
        let mut config = data.config.lock().unwrap();
        config.roms_dir = Some(dir);
        save_config(&app, &config);
    }
    build_library(&app, &data)
}

#[tauri::command]
fn toggle_favorite(app: AppHandle, data: State<AppData>, path: String) -> bool {
    let mut config = data.config.lock().unwrap();
    let now_favorite = match config.favorites.iter().position(|p| p == &path) {
        Some(index) => {
            config.favorites.remove(index);
            false
        }
        None => {
            config.favorites.push(path);
            true
        }
    };
    save_config(&app, &config);
    now_favorite
}

#[tauri::command]
fn set_fullscreen(app: AppHandle, data: State<AppData>, value: bool) {
    let mut config = data.config.lock().unwrap();
    config.fullscreen = value;
    save_config(&app, &config);
}

#[tauri::command]
fn open_folder(path: String) -> Result<(), String> {
    Command::new("explorer")
        .arg(path)
        .spawn()
        .map(|_| ())
        .map_err(|e| format!("No se pudo abrir la carpeta: {e}"))
}

/// Abre el juego con su emulador y avisa a la ventana ("juego-cerrado") cuando se cierra.
#[tauri::command]
fn launch(app: AppHandle, data: State<AppData>, path: String) -> Result<(), String> {
    let rom = PathBuf::from(&path);
    if !rom.exists() {
        return Err("No se encuentra el archivo del juego. ¿Lo has movido o borrado?".into());
    }
    let system = system_for(&rom).ok_or("Este tipo de archivo no es compatible.")?;

    let fullscreen = {
        let mut config = data.config.lock().unwrap();
        config.recents.retain(|p| p != &path);
        config.recents.insert(0, path.clone());
        config.recents.truncate(20);
        save_config(&app, &config);
        config.fullscreen
    };

    let emulators = emulators_dir(&app);
    let mut command = match retroarch_core(system) {
        Some(core) => {
            let dir = emulators.join("RetroArch");
            let mut c = Command::new(dir.join("retroarch.exe"));
            c.current_dir(&dir)
                .arg("-L")
                .arg(dir.join("cores").join(format!("{core}_libretro.dll")))
                .arg("--appendconfig")
                .arg(dir.join("pokemulador.cfg"));
            if fullscreen {
                c.arg("-f");
            }
            c.arg(&rom);
            c
        }
        None => {
            let dir = emulators.join("Azahar");
            let mut c = Command::new(dir.join("azahar.exe"));
            // Azahar espera el juego como último argumento.
            c.current_dir(&dir)
                .arg(if fullscreen { "-f" } else { "-w" })
                .arg(&rom);
            c
        }
    };

    let mut child = command
        .spawn()
        .map_err(|e| format!("No se pudo abrir el emulador: {e}"))?;
    let handle = app.clone();
    std::thread::spawn(move || {
        let _ = child.wait();
        let _ = handle.emit("juego-cerrado", ());
    });
    Ok(())
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let config = load_config(app.handle());
            app.manage(AppData {
                config: Mutex::new(config),
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            library,
            set_roms_dir,
            toggle_favorite,
            set_fullscreen,
            open_folder,
            launch
        ])
        .run(tauri::generate_context!())
        .expect("no se pudo iniciar Pokemulador");
}
