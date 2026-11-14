// Pokemulador para PC: biblioteca de juegos con aspecto Pokédex que abre cada juego
// con el emulador que le toca (RetroArch para GB/GBC/GBA/DS/N64 y Azahar para 3DS).
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use serde::{Deserialize, Serialize};
use std::collections::hash_map::RandomState;
use std::fs;
use std::hash::{BuildHasher, Hasher};
use std::io::{BufRead, BufReader};
#[cfg(windows)]
use std::os::windows::process::CommandExt;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use tauri::{AppHandle, Emitter, Manager, RunEvent, State};

/// Para que los programas de consola (sala y enlace) no abran una ventana negra.
#[cfg(windows)]
const SIN_VENTANA: u32 = 0x0800_0000;

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
    sala: Mutex<Option<Sala>>,
}

/// Sala online abierta: el código y los procesos que la mantienen (servidor de Azahar y enlace).
struct Sala {
    codigo: String,
    anfitrion: bool,
    procesos: Vec<Child>,
}

impl Sala {
    fn cerrar(&mut self) {
        for proceso in &mut self.procesos {
            let _ = proceso.kill();
            let _ = proceso.wait();
        }
        self.procesos.clear();
    }
}

#[derive(Serialize)]
struct EstadoSala {
    codigo: String,
    anfitrion: bool,
}

const PUERTO_SALA: u16 = 24872;
const LETRAS_CODIGO: &[u8] = b"ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
/// Clave de Tailscale metida al compilar (variable POKEMULADOR_TS_KEY). Nunca va en el repositorio.
const CLAVE_COMPILADA: Option<&str> = option_env!("POKEMULADOR_TS_KEY");

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

// ---------- Salas online ----------

fn clave_tailscale(app: &AppHandle) -> Option<String> {
    // Primero un archivo junto a la configuración (para cambiarla sin recompilar), luego la compilada.
    let archivo = config_path(app).with_file_name("tailscale.txt");
    if let Ok(texto) = fs::read_to_string(archivo) {
        let texto = texto.trim().to_string();
        if !texto.is_empty() {
            return Some(texto);
        }
    }
    CLAVE_COMPILADA.map(|c| c.trim().to_string()).filter(|c| !c.is_empty())
}

fn nuevo_codigo() -> String {
    let mut semilla = RandomState::new().build_hasher();
    semilla.write_u128(
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or_default(),
    );
    let mut x = semilla.finish() | 1;
    (0..6)
        .map(|_| {
            x ^= x << 13;
            x ^= x >> 7;
            x ^= x << 17;
            LETRAS_CODIGO[(x % LETRAS_CODIGO.len() as u64) as usize] as char
        })
        .collect()
}

fn codigo_valido(codigo: &str) -> bool {
    codigo.len() == 6 && codigo.bytes().all(|b| b.is_ascii_alphanumeric())
}

/// Deja escrita en Azahar la dirección de la sala (127.0.0.1:24872) para "Conectar directamente".
fn preparar_azahar(emuladores: &Path) {
    let ini = emuladores
        .join("Azahar")
        .join("user")
        .join("config")
        .join("qt-config.ini");
    let texto = fs::read_to_string(&ini).unwrap_or_default();
    let nuestras = [
        "Multiplayer\\ip\\default=false".to_string(),
        "Multiplayer\\ip=127.0.0.1".to_string(),
        "Multiplayer\\port\\default=false".to_string(),
        format!("Multiplayer\\port={PUERTO_SALA}"),
    ];
    let mut lineas: Vec<String> = texto
        .lines()
        .filter(|l| {
            !(l.starts_with("Multiplayer\\ip=")
                || l.starts_with("Multiplayer\\ip\\default=")
                || l.starts_with("Multiplayer\\port=")
                || l.starts_with("Multiplayer\\port\\default="))
        })
        .map(String::from)
        .collect();
    match lineas.iter().position(|l| l.trim() == "[UI]") {
        Some(i) => {
            for (n, linea) in nuestras.iter().enumerate() {
                lineas.insert(i + 1 + n, linea.clone());
            }
        }
        None => {
            lineas.push("[UI]".to_string());
            lineas.extend(nuestras.iter().cloned());
        }
    }
    if let Some(carpeta) = ini.parent() {
        let _ = fs::create_dir_all(carpeta);
    }
    let _ = fs::write(&ini, lineas.join("\n") + "\n");
}

fn sin_ventana(command: &mut Command) -> &mut Command {
    #[cfg(windows)]
    command.creation_flags(SIN_VENTANA);
    command
}

/// Arranca el enlace de Tailscale y reenvía cada línea de estado a la ventana ("sala-estado").
fn arrancar_enlace(app: &AppHandle, modo: &str, codigo: &str, clave: &str) -> Result<Child, String> {
    let exe = emulators_dir(app).join("Enlace").join("enlace.exe");
    let mut command = Command::new(exe);
    sin_ventana(&mut command)
        .arg(modo)
        .arg("--codigo")
        .arg(codigo)
        .arg("--puerto")
        .arg(PUERTO_SALA.to_string())
        .env("POKEMULADOR_TS_KEY", clave)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    let mut hijo = command
        .spawn()
        .map_err(|e| format!("No se pudo arrancar el enlace online: {e}"))?;
    let salida = hijo.stdout.take();
    let handle = app.clone();
    std::thread::spawn(move || {
        if let Some(salida) = salida {
            for linea in BufReader::new(salida).lines().map_while(Result::ok) {
                if let Ok(valor) = serde_json::from_str::<serde_json::Value>(&linea) {
                    let _ = handle.emit("sala-estado", valor);
                }
            }
        }
        let _ = handle.emit("sala-cerrada", ());
    });
    Ok(hijo)
}

fn abrir_sala(app: &AppHandle, data: &AppData, codigo: String, anfitrion: bool) -> Result<EstadoSala, String> {
    let clave = clave_tailscale(app)
        .ok_or("Esta versión de Pokemulador no tiene la clave de Tailscale para jugar online.")?;
    let emuladores = emulators_dir(app);

    let mut sala = data.sala.lock().unwrap();
    if let Some(anterior) = sala.as_mut() {
        anterior.cerrar();
    }
    *sala = None;

    preparar_azahar(&emuladores);
    let mut procesos = Vec::new();
    if anfitrion {
        let dir = emuladores.join("Azahar");
        let mut servidor = Command::new(dir.join("azahar-room.exe"));
        sin_ventana(&mut servidor)
            .current_dir(&dir)
            .arg("--room-name")
            .arg(format!("Pokemulador {codigo}"))
            .arg("--port")
            .arg(PUERTO_SALA.to_string())
            .arg("--password")
            .arg(&codigo)
            .arg("--max_members")
            .arg("4")
            .arg("--preferred-app")
            .arg("Pokemon")
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::null());
        procesos.push(
            servidor
                .spawn()
                .map_err(|e| format!("No se pudo abrir el servidor de la sala: {e}"))?,
        );
    }
    match arrancar_enlace(app, if anfitrion { "host" } else { "unirse" }, &codigo, &clave) {
        Ok(enlace) => procesos.push(enlace),
        Err(e) => {
            for mut p in procesos {
                let _ = p.kill();
            }
            return Err(e);
        }
    }
    *sala = Some(Sala {
        codigo: codigo.clone(),
        anfitrion,
        procesos,
    });
    Ok(EstadoSala { codigo, anfitrion })
}

#[tauri::command]
fn crear_sala(app: AppHandle, data: State<AppData>) -> Result<EstadoSala, String> {
    abrir_sala(&app, &data, nuevo_codigo(), true)
}

#[tauri::command]
fn unirse_sala(app: AppHandle, data: State<AppData>, codigo: String) -> Result<EstadoSala, String> {
    let codigo = codigo.trim().to_uppercase();
    if !codigo_valido(&codigo) {
        return Err("El código tiene que tener 6 letras o números.".into());
    }
    abrir_sala(&app, &data, codigo, false)
}

#[tauri::command]
fn salir_sala(data: State<AppData>) {
    if let Some(mut sala) = data.sala.lock().unwrap().take() {
        sala.cerrar();
    }
}

#[tauri::command]
fn estado_sala(app: AppHandle, data: State<AppData>) -> serde_json::Value {
    let sala = data.sala.lock().unwrap();
    serde_json::json!({
        "clave": clave_tailscale(&app).is_some(),
        "sala": sala.as_ref().map(|s| EstadoSala { codigo: s.codigo.clone(), anfitrion: s.anfitrion }),
    })
}

fn main() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let config = load_config(app.handle());
            app.manage(AppData {
                config: Mutex::new(config),
                sala: Mutex::new(None),
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            library,
            set_roms_dir,
            toggle_favorite,
            set_fullscreen,
            open_folder,
            launch,
            crear_sala,
            unirse_sala,
            salir_sala,
            estado_sala
        ])
        .build(tauri::generate_context!())
        .expect("no se pudo iniciar Pokemulador");

    app.run(|handle, event| {
        // Al cerrar Pokemulador cerramos también la sala, si había una abierta.
        if let RunEvent::Exit = event {
            if let Some(data) = handle.try_state::<AppData>() {
                if let Some(mut sala) = data.sala.lock().unwrap().take() {
                    sala.cerrar();
                }
            }
        }
    });
}
