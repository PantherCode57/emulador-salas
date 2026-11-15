// Pokemulador para PC: biblioteca de juegos con aspecto Pokédex que abre cada juego
// con el emulador que le toca (RetroArch para GB/GBC/GBA/DS/N64 y Azahar para 3DS).
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod captura;
mod salas;

use salas::{SalaActiva, SalaGuardada};
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
use std::time::{Duration, Instant};
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
    /// Salas guardadas (se puede volver a entrar cuando se quiera).
    salas: Vec<SalaGuardada>,
    /// Identificador en las salas (distinto en cada equipo) y nombre que ven los amigos.
    id_jugador: Option<String>,
    nombre_visible: Option<String>,
}

impl Default for Config {
    fn default() -> Self {
        Config {
            roms_dir: None,
            recents: Vec::new(),
            favorites: Vec::new(),
            fullscreen: true,
            salas: Vec::new(),
            id_jugador: None,
            nombre_visible: None,
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
    sala: Mutex<Option<SalaActiva>>,
}

const PUERTO_SALA: u16 = 24872;
const LETRAS_CODIGO: &[u8] = b"ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
/// Clave de Tailscale metida al compilar (variable POKEMULADOR_TS_KEY). Nunca va en el repositorio.
const CLAVE_COMPILADA: Option<&str> = option_env!("POKEMULADOR_TS_KEY");

/// Carpeta de datos de Pokemulador (%APPDATA%\com.panthercode.pokemulador).
fn datos_dir(app: &AppHandle) -> PathBuf {
    app.path().app_config_dir().unwrap_or_else(|_| PathBuf::from("."))
}

fn config_path(app: &AppHandle) -> PathBuf {
    datos_dir(app).join("config.json")
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
    lanzar(&app, &data, &path, None)
}

/// Abre un juego. Si viene de una sala, usa la partida de esa sala y va guardando el progreso
/// (tiempo jugado y una captura) que ven los amigos.
fn lanzar(app: &AppHandle, data: &AppData, path: &str, sala: Option<SalaGuardada>) -> Result<(), String> {
    let rom = PathBuf::from(path);
    if !rom.exists() {
        return Err("No se encuentra el archivo del juego. ¿Lo has movido o borrado?".into());
    }
    let system = system_for(&rom).ok_or("Este tipo de archivo no es compatible.")?;

    let fullscreen = {
        let mut config = data.config.lock().unwrap();
        config.recents.retain(|p| p != path);
        config.recents.insert(0, path.to_string());
        config.recents.truncate(20);
        save_config(app, &config);
        config.fullscreen
    };

    let datos = datos_dir(app);
    let perfil = sala
        .as_ref()
        .filter(|s| s.partida_propia)
        .map(|s| salas::carpeta_perfil(&datos, &s.codigo));
    let emulators = emulators_dir(app);
    // GBA desde la sala en la que estamos: partida compartida (os veis por el mapa).
    if system == "gba" {
        let modo = sala.as_ref().and_then(|s| {
            data.sala
                .lock()
                .unwrap()
                .as_ref()
                .filter(|a| a.codigo == s.codigo)
                .and_then(|a| a.anfitrion)
        });
        let (_, nombre) = jugador(app, data);
        if let Err(e) = salas::partida_compartida_gba(&emulators, modo, &nombre) {
            eprintln!("{e}");
            let _ = salas::partida_compartida_gba(&emulators, None, &nombre);
        }
    }
    let mut command = match retroarch_core(system) {
        Some(core) => {
            let dir = emulators.join("RetroArch");
            let mut ajustes = dir.join("pokemulador.cfg").to_string_lossy().to_string();
            if let Some(perfil) = &perfil {
                ajustes = format!("{ajustes}|{}", salas::cfg_retroarch_perfil(perfil).display());
            }
            let mut c = Command::new(dir.join("retroarch.exe"));
            c.current_dir(&dir)
                .arg("-L")
                .arg(dir.join("cores").join(format!("{core}_libretro.dll")))
                .arg("--appendconfig")
                .arg(ajustes);
            if fullscreen {
                c.arg("-f");
            }
            c.arg(&rom);
            c
        }
        None => {
            preparar_azahar(&emulators);
            salas::almacenamiento_azahar(&emulators, perfil.as_deref());
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
    let (id, nombre) = jugador(app, data);
    std::thread::spawn(move || {
        let pid = child.id();
        let mut desde = Instant::now();
        let mut ultima_captura: Option<String> = None;
        let mut proxima_captura = Instant::now() + Duration::from_secs(20);
        while let Ok(None) = child.try_wait() {
            if let Some(sala) = &sala {
                if Instant::now() >= proxima_captura {
                    if let Some(c) = captura::capturar_base64(pid) {
                        ultima_captura = Some(c);
                    }
                    proxima_captura = Instant::now() + Duration::from_secs(60);
                }
                // Cada 5 minutos, para que los amigos que estén en la sala vean cómo vamos.
                if desde.elapsed() >= Duration::from_secs(300) {
                    let ms = desde.elapsed().as_millis() as u64;
                    salas::sumar_progreso(&datos, sala, &id, &nombre, ms, ultima_captura.as_deref());
                    desde = Instant::now();
                }
            }
            std::thread::sleep(Duration::from_millis(500));
        }
        if let Some(sala) = &sala {
            let ms = desde.elapsed().as_millis() as u64;
            salas::sumar_progreso(&datos, sala, &id, &nombre, ms, ultima_captura.as_deref());
        }
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

fn aleatorio(n: usize) -> String {
    let mut semilla = RandomState::new().build_hasher();
    semilla.write_u128(
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or_default(),
    );
    let mut x = semilla.finish() | 1;
    (0..n)
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

/// Nuestro identificador en las salas (se crea una vez) y el nombre que ven los amigos.
fn jugador(app: &AppHandle, data: &AppData) -> (String, String) {
    let mut config = data.config.lock().unwrap();
    let id = match &config.id_jugador {
        Some(id) => id.clone(),
        None => {
            let id = format!("PC-{}", aleatorio(4));
            config.id_jugador = Some(id.clone());
            save_config(app, &config);
            id
        }
    };
    let nombre = config
        .nombre_visible
        .clone()
        .filter(|n| !n.trim().is_empty())
        .unwrap_or_else(|| id.clone());
    (id, nombre)
}

/// Deja escrita en Azahar la dirección de la sala (127.0.0.1:24872) para "Conectar directamente".
fn preparar_azahar(emuladores: &Path) {
    let ini = emuladores
        .join("Azahar")
        .join("user")
        .join("config")
        .join("qt-config.ini");
    salas::fijar_ini(
        &ini,
        "UI",
        &[
            ("Multiplayer\\ip\\default", Some("false".into())),
            ("Multiplayer\\ip", Some("127.0.0.1".into())),
            ("Multiplayer\\port\\default", Some("false".into())),
            ("Multiplayer\\port", Some(PUERTO_SALA.to_string())),
        ],
    );
}

fn sin_ventana(command: &mut Command) -> &mut Command {
    #[cfg(windows)]
    command.creation_flags(SIN_VENTANA);
    command
}

/// El servidor de salas de Azahar, cuando nos toca abrir la sala a nosotros.
fn abrir_servidor(emuladores: &Path, codigo: &str) -> Result<Child, String> {
    let dir = emuladores.join("Azahar");
    let mut servidor = Command::new(dir.join("azahar-room.exe"));
    sin_ventana(&mut servidor)
        .current_dir(&dir)
        .arg("--room-name")
        .arg(format!("Pokemulador {codigo}"))
        .arg("--port")
        .arg(PUERTO_SALA.to_string())
        .arg("--password")
        .arg(codigo)
        .arg("--max_members")
        .arg("4")
        .arg("--preferred-app")
        .arg("Pokemon")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null());
    servidor
        .spawn()
        .map_err(|e| format!("No se pudo abrir el servidor de la sala: {e}"))
}

/// Arranca el enlace de Tailscale en modo sala. Cada línea de estado va a la ventana
/// ("sala-estado"); cuando decide si abrimos la sala, arranca o para el servidor de Azahar.
fn arrancar_enlace(app: &AppHandle, codigo: &str, clave: &str, id: &str) -> Result<Child, String> {
    let emuladores = emulators_dir(app);
    let datos = datos_dir(app);
    let exe = emuladores.join("Enlace").join("enlace.exe");
    let mut command = Command::new(exe);
    sin_ventana(&mut command)
        .arg("sala")
        .arg("--codigo")
        .arg(codigo)
        .arg("--puerto")
        .arg(PUERTO_SALA.to_string())
        .arg("--id")
        .arg(id)
        .arg("--progreso")
        .arg(salas::archivo_propio(&datos, codigo))
        .arg("--amigos")
        .arg(salas::carpeta_amigos(&datos, codigo))
        .env("POKEMULADOR_TS_KEY", clave)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null());
    let mut hijo = command
        .spawn()
        .map_err(|e| format!("No se pudo arrancar el enlace online: {e}"))?;
    let salida = hijo.stdout.take();
    let handle = app.clone();
    let codigo = codigo.to_string();
    std::thread::spawn(move || {
        if let Some(salida) = salida {
            for linea in BufReader::new(salida).lines().map_while(Result::ok) {
                let Ok(mut valor) = serde_json::from_str::<serde_json::Value>(&linea) else {
                    continue;
                };
                if valor["estado"] == "rol" {
                    let anfitrion = valor["rol"] == "host";
                    if let Some(data) = handle.try_state::<AppData>() {
                        let mut guardia = data.sala.lock().unwrap();
                        if let Some(activa) = guardia.as_mut().filter(|a| a.codigo == codigo) {
                            activa.anfitrion = Some(anfitrion);
                            if !anfitrion {
                                activa.cerrar_servidor();
                            } else if activa.servidor.is_none() {
                                activa.servidor = abrir_servidor(&emuladores, &codigo).ok();
                            }
                        }
                    }
                }
                valor["codigo"] = serde_json::json!(codigo);
                let _ = handle.emit("sala-estado", valor);
            }
        }
        let _ = handle.emit("sala-cerrada", serde_json::json!({ "codigo": codigo }));
    });
    Ok(hijo)
}

fn buscar_sala(data: &AppData, codigo: &str) -> Result<SalaGuardada, String> {
    data.config
        .lock()
        .unwrap()
        .salas
        .iter()
        .find(|s| s.codigo == codigo)
        .cloned()
        .ok_or_else(|| "Esa sala ya no está guardada.".to_string())
}

fn guardar_sala_en(
    app: &AppHandle,
    data: &AppData,
    codigo: String,
    nombre: String,
    juego: String,
    partida_propia: bool,
) -> Result<SalaGuardada, String> {
    let rom = PathBuf::from(&juego);
    let sistema = system_for(&rom).ok_or("Ese juego no es compatible.")?;
    let titulo = clean_title(&rom.file_stem().unwrap_or_default().to_string_lossy());
    let mut config = data.config.lock().unwrap();
    let anterior = config.salas.iter().find(|s| s.codigo == codigo).map(|s| s.creada);
    let sala = SalaGuardada {
        nombre: if nombre.trim().is_empty() {
            titulo.clone()
        } else {
            nombre.trim().chars().take(30).collect()
        },
        codigo: codigo.clone(),
        juego,
        juego_titulo: titulo,
        sistema: sistema.to_string(),
        partida_propia,
        creada: anterior.unwrap_or_else(salas::ahora_ms),
    };
    config.salas.retain(|s| s.codigo != codigo);
    config.salas.insert(0, sala.clone());
    save_config(app, &config);
    Ok(sala)
}

/// Lista de salas guardadas, con lo último que sabemos de los amigos de cada una.
#[tauri::command]
fn salas_guardadas(app: AppHandle, data: State<AppData>) -> serde_json::Value {
    let (id, nombre) = jugador(&app, &data);
    let datos = datos_dir(&app);
    let lista: Vec<serde_json::Value> = data
        .config
        .lock()
        .unwrap()
        .salas
        .iter()
        .map(|s| {
            let mut v = serde_json::to_value(s).unwrap_or_default();
            v["progreso"] = salas::leer_progreso(&datos, &s.codigo, &id, false);
            v
        })
        .collect();
    let activa = data
        .sala
        .lock()
        .unwrap()
        .as_ref()
        .map(|a| serde_json::json!({ "codigo": a.codigo, "anfitrion": a.anfitrion }));
    serde_json::json!({
        "clave": clave_tailscale(&app).is_some(),
        "id": id,
        "nombre": nombre,
        "salas": lista,
        "activa": activa,
    })
}

#[tauri::command]
fn crear_sala(
    app: AppHandle,
    data: State<AppData>,
    nombre: String,
    juego: String,
    partida_propia: bool,
) -> Result<SalaGuardada, String> {
    let usados: Vec<String> = data.config.lock().unwrap().salas.iter().map(|s| s.codigo.clone()).collect();
    let mut codigo = aleatorio(6);
    while usados.contains(&codigo) {
        codigo = aleatorio(6);
    }
    guardar_sala_en(&app, &data, codigo, nombre, juego, partida_propia)
}

#[tauri::command]
fn guardar_sala(
    app: AppHandle,
    data: State<AppData>,
    codigo: String,
    nombre: String,
    juego: String,
    partida_propia: bool,
) -> Result<SalaGuardada, String> {
    let codigo = codigo.trim().to_uppercase();
    if !codigo_valido(&codigo) {
        return Err("El código tiene que tener 6 letras o números.".into());
    }
    guardar_sala_en(&app, &data, codigo, nombre, juego, partida_propia)
}

/// Borra la sala, lo que sabíamos de los amigos y, si la tenía, su partida propia.
#[tauri::command]
fn borrar_sala(app: AppHandle, data: State<AppData>, codigo: String) {
    let activa = {
        let mut guardia = data.sala.lock().unwrap();
        if guardia.as_ref().is_some_and(|a| a.codigo == codigo) {
            guardia.take()
        } else {
            None
        }
    };
    if let Some(mut activa) = activa {
        activa.cerrar();
    }
    {
        let mut config = data.config.lock().unwrap();
        config.salas.retain(|s| s.codigo != codigo);
        save_config(&app, &config);
    }
    let datos = datos_dir(&app);
    let _ = fs::remove_dir_all(salas::carpeta_sala(&datos, &codigo));
    let _ = fs::remove_dir_all(salas::carpeta_perfil(&datos, &codigo));
}

#[tauri::command]
fn entrar_sala(app: AppHandle, data: State<AppData>, codigo: String) -> Result<(), String> {
    let clave = clave_tailscale(&app)
        .ok_or("Esta versión de Pokemulador no tiene la clave de Tailscale para jugar online.")?;
    let sala = buscar_sala(&data, &codigo)?;
    let anterior = data.sala.lock().unwrap().take();
    if let Some(mut anterior) = anterior {
        anterior.cerrar();
    }
    let (id, nombre) = jugador(&app, &data);
    salas::preparar_progreso(&datos_dir(&app), &sala, &id, &nombre);
    preparar_azahar(&emulators_dir(&app));
    let enlace = arrancar_enlace(&app, &codigo, &clave, &id)?;
    *data.sala.lock().unwrap() = Some(SalaActiva {
        codigo,
        anfitrion: None,
        enlace,
        servidor: None,
    });
    Ok(())
}

#[tauri::command]
fn salir_sala(data: State<AppData>) {
    let activa = data.sala.lock().unwrap().take();
    if let Some(mut activa) = activa {
        activa.cerrar();
    }
}

#[tauri::command]
fn jugar_sala(app: AppHandle, data: State<AppData>, codigo: String) -> Result<(), String> {
    let sala = buscar_sala(&data, &codigo)?;
    let juego = sala.juego.clone();
    lanzar(&app, &data, &juego, Some(sala))
}

/// Nuestro progreso y el de los amigos en una sala, con las capturas.
#[tauri::command]
fn progreso_sala(app: AppHandle, data: State<AppData>, codigo: String) -> serde_json::Value {
    let (id, _) = jugador(&app, &data);
    salas::leer_progreso(&datos_dir(&app), &codigo, &id, true)
}

#[tauri::command]
fn cambiar_nombre(app: AppHandle, data: State<AppData>, nombre: String) -> String {
    {
        let mut config = data.config.lock().unwrap();
        config.nombre_visible = Some(nombre.trim().chars().take(24).collect());
        save_config(&app, &config);
    }
    let (id, nombre) = jugador(&app, &data);
    // Si estamos en una sala, que los amigos vean el nombre nuevo.
    let activa = data.sala.lock().unwrap().as_ref().map(|a| a.codigo.clone());
    if let Some(sala) = activa.and_then(|c| buscar_sala(&data, &c).ok()) {
        salas::preparar_progreso(&datos_dir(&app), &sala, &id, &nombre);
    }
    nombre
}

fn perfil_de(app: &AppHandle, data: &AppData, codigo: Option<String>) -> Result<Option<PathBuf>, String> {
    match codigo {
        None => Ok(None),
        Some(codigo) => {
            let sala = buscar_sala(data, &codigo)?;
            Ok(sala
                .partida_propia
                .then(|| salas::carpeta_perfil(&datos_dir(app), &codigo)))
        }
    }
}

/// Carga un .sav (de linkcable.app, mGBA...) como partida del juego, o de la sala `codigo`.
#[tauri::command]
fn importar_partida(
    app: AppHandle,
    data: State<AppData>,
    juego: String,
    archivo: String,
    codigo: Option<String>,
) -> Result<String, String> {
    let rom = PathBuf::from(&juego);
    let sistema = system_for(&rom).ok_or("Este tipo de archivo no es compatible.")?;
    let perfil = perfil_de(&app, &data, codigo)?;
    salas::importar_partida(&emulators_dir(&app), perfil.as_deref(), &rom, sistema, Path::new(&archivo))
}

/// Guarda la partida del juego (o la de la sala `codigo`) en un archivo .sav.
#[tauri::command]
fn exportar_partida(
    app: AppHandle,
    data: State<AppData>,
    juego: String,
    archivo: String,
    codigo: Option<String>,
) -> Result<String, String> {
    let rom = PathBuf::from(&juego);
    let sistema = system_for(&rom).ok_or("Este tipo de archivo no es compatible.")?;
    let perfil = perfil_de(&app, &data, codigo)?;
    salas::exportar_partida(&emulators_dir(&app), perfil.as_deref(), &rom, sistema, Path::new(&archivo))
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
            salas_guardadas,
            crear_sala,
            guardar_sala,
            borrar_sala,
            entrar_sala,
            salir_sala,
            jugar_sala,
            progreso_sala,
            cambiar_nombre,
            importar_partida,
            exportar_partida
        ])
        .build(tauri::generate_context!())
        .expect("no se pudo iniciar Pokemulador");

    app.run(|handle, event| {
        // Al cerrar Pokemulador cerramos también la sala, si había una abierta.
        if let RunEvent::Exit = event {
            if let Some(data) = handle.try_state::<AppData>() {
                let activa = data.sala.lock().unwrap().take();
                if let Some(mut activa) = activa {
                    activa.cerrar();
                }
            }
        }
    });
}
