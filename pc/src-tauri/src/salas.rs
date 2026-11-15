//! Salas guardadas, partida propia de cada sala, progreso de los amigos y partidas .sav.
//! Usa los mismos archivos y el mismo formato de progreso que la app de Android.

use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Child;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

#[derive(Serialize, Deserialize, Clone)]
pub struct SalaGuardada {
    pub codigo: String,
    pub nombre: String,
    /// Ruta del juego en este PC.
    pub juego: String,
    pub juego_titulo: String,
    pub sistema: String,
    /// true: la sala tiene su propia partida; false: se juega con la de siempre.
    pub partida_propia: bool,
    pub creada: u64,
}

/// La sala en la que estamos dentro: el enlace de Tailscale y, si nos toca abrirla, el servidor.
pub struct SalaActiva {
    pub codigo: String,
    /// None hasta que el enlace decide si abrimos la sala o entramos en la de un amigo.
    pub anfitrion: Option<bool>,
    pub enlace: Child,
    pub servidor: Option<Child>,
}

impl SalaActiva {
    pub fn cerrar(&mut self) {
        // Cerrando su entrada, el enlace se despide de los amigos y sale solo.
        drop(self.enlace.stdin.take());
        let limite = Instant::now() + Duration::from_millis(1500);
        while Instant::now() < limite {
            if let Ok(Some(_)) = self.enlace.try_wait() {
                break;
            }
            std::thread::sleep(Duration::from_millis(100));
        }
        let _ = self.enlace.kill();
        let _ = self.enlace.wait();
        self.cerrar_servidor();
    }

    pub fn cerrar_servidor(&mut self) {
        if let Some(mut servidor) = self.servidor.take() {
            let _ = servidor.kill();
            let _ = servidor.wait();
        }
    }
}

pub fn ahora_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or_default()
}

// ---------- Carpetas ----------

pub fn carpeta_sala(datos: &Path, codigo: &str) -> PathBuf {
    datos.join("salas").join(codigo)
}

pub fn archivo_propio(datos: &Path, codigo: &str) -> PathBuf {
    carpeta_sala(datos, codigo).join("mi_progreso.json")
}

pub fn carpeta_amigos(datos: &Path, codigo: &str) -> PathBuf {
    carpeta_sala(datos, codigo).join("amigos")
}

/// Carpeta con la partida propia de una sala (saves y states de RetroArch, sdmc de Azahar).
pub fn carpeta_perfil(datos: &Path, codigo: &str) -> PathBuf {
    datos.join("perfiles").join(format!("sala-{codigo}"))
}

fn escribir_atomico(ruta: &Path, datos: &[u8]) -> std::io::Result<()> {
    if let Some(padre) = ruta.parent() {
        fs::create_dir_all(padre)?;
    }
    let temporal = ruta.with_extension("tmp");
    fs::write(&temporal, datos)?;
    fs::rename(&temporal, ruta).or_else(|_| {
        let r = fs::write(ruta, datos);
        let _ = fs::remove_file(&temporal);
        r
    })
}

fn leer_json(ruta: &Path) -> Option<Value> {
    fs::read_to_string(ruta).ok().and_then(|t| serde_json::from_str(&t).ok())
}

// ---------- Progreso ----------

/// Deja listo nuestro progreso al entrar en la sala (nombre y juego al día).
pub fn preparar_progreso(datos: &Path, sala: &SalaGuardada, id: &str, nombre: &str) {
    let ruta = archivo_propio(datos, &sala.codigo);
    let mut valor = leer_json(&ruta).filter(Value::is_object).unwrap_or_else(|| json!({}));
    valor["version"] = json!(1);
    valor["id"] = json!(id);
    valor["nombre"] = json!(nombre);
    valor["juego"] = json!(sala.juego_titulo);
    valor["partida"] = json!(if sala.partida_propia { "sala" } else { "principal" });
    if valor.get("jugadoMs").is_none() {
        valor["jugadoMs"] = json!(0);
    }
    let _ = escribir_atomico(&ruta, valor.to_string().as_bytes());
}

/// Suma tiempo jugado y, si la hay, pone una captura nueva.
pub fn sumar_progreso(datos: &Path, sala: &SalaGuardada, id: &str, nombre: &str, ms: u64, captura: Option<&str>) {
    let ruta = archivo_propio(datos, &sala.codigo);
    let mut valor = leer_json(&ruta).filter(Value::is_object).unwrap_or_else(|| json!({}));
    let jugado = valor.get("jugadoMs").and_then(Value::as_u64).unwrap_or(0) + ms;
    valor["version"] = json!(1);
    valor["id"] = json!(id);
    valor["nombre"] = json!(nombre);
    valor["juego"] = json!(sala.juego_titulo);
    valor["partida"] = json!(if sala.partida_propia { "sala" } else { "principal" });
    valor["jugadoMs"] = json!(jugado);
    valor["ultimaVez"] = json!(ahora_ms());
    if let Some(c) = captura {
        valor["captura"] = json!(c);
    }
    let _ = escribir_atomico(&ruta, valor.to_string().as_bytes());
}

/// Nuestro progreso y el de los amigos (los más recientes primero).
pub fn leer_progreso(datos: &Path, codigo: &str, id_propio: &str, con_capturas: bool) -> Value {
    let limpiar = |mut v: Value| {
        if !con_capturas {
            if let Some(o) = v.as_object_mut() {
                o.remove("captura");
            }
        }
        v
    };
    let mut amigos: Vec<Value> = fs::read_dir(carpeta_amigos(datos, codigo))
        .map(|lista| {
            lista
                .flatten()
                .filter(|e| e.path().extension().is_some_and(|x| x == "json"))
                .filter_map(|e| leer_json(&e.path()))
                .filter(|v| v.get("id").and_then(Value::as_str).is_some_and(|id| id != id_propio))
                .map(limpiar)
                .collect()
        })
        .unwrap_or_default();
    amigos.sort_by_key(|v| std::cmp::Reverse(v.get("ultimaVez").and_then(Value::as_u64).unwrap_or(0)));
    json!({
        "propio": leer_json(&archivo_propio(datos, codigo)).map(limpiar),
        "amigos": amigos,
    })
}

// ---------- Ajustes de los emuladores para cada partida ----------

/// Cambia unas claves dentro de una sección de un .ini (las quita y las vuelve a poner).
pub fn fijar_ini(ini: &Path, seccion: &str, claves: &[(&str, Option<String>)]) {
    let texto = fs::read_to_string(ini).unwrap_or_default();
    let cabecera = format!("[{seccion}]");
    let mut lineas: Vec<String> = Vec::new();
    let mut dentro = false;
    for linea in texto.lines() {
        let t = linea.trim();
        if t.starts_with('[') {
            dentro = t == cabecera;
        }
        if dentro {
            let clave = t.split('=').next().unwrap_or("");
            if claves.iter().any(|(k, _)| *k == clave) {
                continue;
            }
        }
        lineas.push(linea.to_string());
    }
    let nuevas: Vec<String> = claves
        .iter()
        .filter_map(|(k, v)| v.as_ref().map(|v| format!("{k}={v}")))
        .collect();
    match lineas.iter().position(|l| l.trim() == cabecera) {
        Some(i) => {
            for (n, l) in nuevas.into_iter().enumerate() {
                lineas.insert(i + 1 + n, l);
            }
        }
        None if !nuevas.is_empty() => {
            lineas.push(cabecera);
            lineas.extend(nuevas);
        }
        None => {}
    }
    if let Some(carpeta) = ini.parent() {
        let _ = fs::create_dir_all(carpeta);
    }
    let _ = fs::write(ini, lineas.join("\n") + "\n");
}

fn barra_final(ruta: &Path) -> String {
    let mut s = ruta.to_string_lossy().replace('\\', "/");
    if !s.ends_with('/') {
        s.push('/');
    }
    s
}

/// Azahar: la tarjeta SD (donde están las partidas) de la sala, o la de siempre si `perfil` es None.
pub fn almacenamiento_azahar(emuladores: &Path, perfil: Option<&Path>) {
    let usuario = emuladores.join("Azahar").join("user");
    let ini = usuario.join("config").join("qt-config.ini");
    let claves: Vec<(&str, Option<String>)> = match perfil {
        Some(perfil) => {
            let sdmc = perfil.join("sdmc");
            let _ = fs::create_dir_all(&sdmc);
            vec![
                ("use_custom_storage\\default", Some("false".into())),
                ("use_custom_storage", Some("true".into())),
                // La memoria interna se comparte; solo cambia la tarjeta SD.
                ("nand_directory", Some(barra_final(&usuario.join("nand")))),
                ("sdmc_directory", Some(barra_final(&sdmc))),
            ]
        }
        None => vec![
            ("use_custom_storage\\default", Some("true".into())),
            ("use_custom_storage", Some("false".into())),
            ("nand_directory", None),
            ("sdmc_directory", None),
        ],
    };
    fijar_ini(&ini, "Data%20Storage", &claves);
}

/// RetroArch: un .cfg que manda las partidas y los guardados rápidos a la carpeta de la sala.
pub fn cfg_retroarch_perfil(perfil: &Path) -> PathBuf {
    let cfg = perfil.join("retroarch-perfil.cfg");
    let saves = perfil.join("saves");
    let states = perfil.join("states");
    let _ = fs::create_dir_all(&saves);
    let _ = fs::create_dir_all(&states);
    let texto = format!(
        "# Partida propia de una sala de Pokemulador.\nsavefile_directory = \"{}\"\nsavestate_directory = \"{}\"\n",
        saves.display(),
        states.display()
    );
    let _ = fs::write(&cfg, texto);
    cfg
}

// ---------- Partida compartida de GBA (GBA-PK) ----------

/// Escribe (o quita) el script de la partida compartida que carga el núcleo de mGBA de Pokemulador.
/// `modo`: Some(true) si tenemos la sala abierta (abrimos la partida), Some(false) si entramos en la
/// del amigo, None para jugar sin partida compartida. Ver terceros/gba-pk/LEEME.md.
pub fn partida_compartida_gba(emuladores: &Path, modo: Option<bool>, nombre: &str) -> Result<(), String> {
    let destino = emuladores
        .join("RetroArch")
        .join("system")
        .join("pokemulador")
        .join("multijugador.lua");
    let Some(anfitrion) = modo else {
        let _ = fs::remove_file(&destino);
        return Ok(());
    };
    let carpeta = emuladores.join("GBA-PK");
    let base = fs::read_to_string(carpeta.join("GBA-PK_Multiplayer.lua"))
        .map_err(|e| format!("Falta GBA-PK en la instalación: {e}"))?;
    let final_ = fs::read_to_string(carpeta.join("pokemulador_final.lua"))
        .map_err(|e| format!("Falta GBA-PK en la instalación: {e}"))?
        .replace("__MODO__", if anfitrion { "h" } else { "c" });
    // GBA-PK admite hasta 10 caracteres; nos quedamos con letras, números y espacios.
    let mut nombre: String = nombre
        .chars()
        .filter(|c| c.is_alphanumeric() || *c == ' ')
        .take(10)
        .collect::<String>()
        .trim()
        .to_string();
    if nombre.is_empty() {
        nombre = "Jugador".into();
    }
    let texto = base.replacen("local Nickname = \"\"", &format!("local Nickname = \"{nombre}\""), 1) + "\n" + &final_;
    escribir_atomico(&destino, texto.as_bytes()).map_err(|e| format!("No se pudo preparar la partida compartida: {e}"))
}

// ---------- Partidas .sav ----------

/// Carpeta que usa RetroArch para cada núcleo (ordena las partidas por núcleo).
fn carpeta_nucleo(sistema: &str) -> Option<&'static str> {
    match sistema {
        "gb" | "gbc" => Some("Gambatte"),
        "gba" => Some("mGBA"),
        "nds" => Some("melonDS DS"),
        "n64" => Some("Mupen64Plus-Next"),
        _ => None,
    }
}

fn raiz_partidas(emuladores: &Path, perfil: Option<&Path>) -> PathBuf {
    perfil.map(Path::to_path_buf).unwrap_or_else(|| emuladores.join("RetroArch"))
}

fn ruta_partida(emuladores: &Path, perfil: Option<&Path>, rom: &Path, sistema: &str) -> Result<PathBuf, String> {
    let nucleo = carpeta_nucleo(sistema)
        .ok_or("En 3DS la partida va dentro de la tarjeta SD de Azahar, no es un archivo suelto.")?;
    let base = rom.file_stem().ok_or("El juego no tiene nombre.")?.to_string_lossy().to_string();
    Ok(raiz_partidas(emuladores, perfil).join("saves").join(nucleo).join(format!("{base}.srm")))
}

const PIE_DESMUME: &[u8] = b"|<--Snip above here to create a raw sav by excluding this DeSmuME savedata footer:";

/// Igual que en Android: quita el pie de DeSmuME y lo que sobre de una potencia de dos (el reloj
/// que añaden algunos emuladores), salvo los 8 bytes de reloj que usa Gambatte en Game Boy.
fn ajustar(sistema: &str, mut datos: Vec<u8>) -> Vec<u8> {
    if let Some(i) = datos.windows(PIE_DESMUME.len()).position(|w| w == PIE_DESMUME) {
        datos.truncate(i);
    }
    if datos.len() >= 512 {
        let potencia = 1usize << (usize::BITS - 1 - datos.len().leading_zeros());
        let sobra = datos.len() - potencia;
        let reloj_gambatte = matches!(sistema, "gb" | "gbc") && sobra == 8;
        if sobra > 0 && !reloj_gambatte {
            datos.truncate(potencia);
        }
    }
    datos
}

pub fn importar_partida(
    emuladores: &Path,
    perfil: Option<&Path>,
    rom: &Path,
    sistema: &str,
    archivo: &Path,
) -> Result<String, String> {
    let destino = ruta_partida(emuladores, perfil, rom, sistema)?;
    let leidos = fs::read(archivo).map_err(|e| format!("No se pudo leer el archivo: {e}"))?;
    if leidos.is_empty() {
        return Err("El archivo está vacío (0 bytes). Vuelve a descargarlo de la web.".into());
    }
    if leidos.len() > 64 * 1024 * 1024 {
        return Err("Ese archivo no es una partida (es demasiado grande).".into());
    }
    let datos = ajustar(sistema, leidos);
    let base = rom.file_stem().unwrap_or_default().to_string_lossy().to_string();
    let raiz = raiz_partidas(emuladores, perfil);
    let copias = raiz.join("saves").join("copias");
    let marca = ahora_ms();

    let mut hay_copia = false;
    if destino.exists() && fs::metadata(&destino).map(|m| m.len() > 0).unwrap_or(false) {
        let _ = fs::create_dir_all(&copias);
        fs::copy(&destino, copias.join(format!("{base}-{marca}.srm")))
            .map_err(|e| format!("No se pudo guardar una copia de la partida de ahora: {e}"))?;
        hay_copia = true;
    }
    escribir_atomico(&destino, &datos).map_err(|e| format!("No se pudo guardar la partida: {e}"))?;

    // RetroArch reanuda desde su guardado automático: lo apartamos para que empiece en la importada.
    if let Some(nucleo) = carpeta_nucleo(sistema) {
        let estado = raiz.join("states").join(nucleo).join(format!("{base}.state.auto"));
        if estado.exists() {
            let _ = fs::create_dir_all(&copias);
            let _ = fs::rename(&estado, copias.join(format!("{base}-{marca}.state.auto")));
        }
    }
    Ok(if hay_copia {
        "Partida cargada. La que tenías antes está guardada como copia por si acaso.".into()
    } else {
        "Partida cargada. Ya puedes jugar con ella.".into()
    })
}

pub fn exportar_partida(
    emuladores: &Path,
    perfil: Option<&Path>,
    rom: &Path,
    sistema: &str,
    archivo: &Path,
) -> Result<String, String> {
    let origen = ruta_partida(emuladores, perfil, rom, sistema)?;
    if !origen.exists() {
        return Err("Todavía no hay ninguna partida guardada de este juego.".into());
    }
    fs::copy(&origen, archivo).map_err(|e| format!("No se pudo guardar el archivo: {e}"))?;
    Ok("Partida guardada en el archivo.".into())
}

#[cfg(test)]
mod pruebas {
    use super::*;

    fn carpeta_temporal(nombre: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("pokemulador-prueba-{nombre}-{}", ahora_ms()));
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    #[test]
    fn recorta_el_reloj_de_gba_y_deja_el_de_gambatte() {
        assert_eq!(ajustar("gba", vec![1; 131072 + 16]).len(), 131072);
        assert_eq!(ajustar("gba", vec![1; 131072]).len(), 131072);
        assert_eq!(ajustar("gbc", vec![1; 32768 + 8]).len(), 32768 + 8);
        assert_eq!(ajustar("gbc", vec![1; 32768 + 48]).len(), 32768);
        let mut ds = vec![7u8; 524288];
        ds.extend_from_slice(PIE_DESMUME);
        ds.extend_from_slice(&[0; 40]);
        assert_eq!(ajustar("nds", ds).len(), 524288);
    }

    #[test]
    fn importa_con_copia_y_aparta_el_guardado_automatico() {
        let raiz = carpeta_temporal("importar");
        let emuladores = raiz.join("emuladores");
        let rom = raiz.join("Pokemon Emerald.gba");
        let destino = emuladores.join("RetroArch/saves/mGBA/Pokemon Emerald.srm");
        let auto = emuladores.join("RetroArch/states/mGBA/Pokemon Emerald.state.auto");
        fs::create_dir_all(destino.parent().unwrap()).unwrap();
        fs::create_dir_all(auto.parent().unwrap()).unwrap();
        fs::write(&destino, vec![9u8; 131072]).unwrap();
        fs::write(&auto, b"estado").unwrap();
        let sav = raiz.join("linkcable.sav");
        fs::write(&sav, vec![5u8; 131072 + 16]).unwrap();

        let mensaje = importar_partida(&emuladores, None, &rom, "gba", &sav).unwrap();
        assert!(mensaje.contains("copia"));
        assert_eq!(fs::read(&destino).unwrap(), vec![5u8; 131072]);
        assert!(!auto.exists());
        let copias: Vec<_> = fs::read_dir(emuladores.join("RetroArch/saves/copias")).unwrap().flatten().collect();
        assert_eq!(copias.len(), 2);

        // Y en la partida propia de una sala, a su carpeta.
        let perfil = raiz.join("perfiles/sala-K7P2QX");
        importar_partida(&emuladores, Some(&perfil), &rom, "gba", &sav).unwrap();
        assert!(perfil.join("saves/mGBA/Pokemon Emerald.srm").exists());

        let salida = raiz.join("sacada.sav");
        exportar_partida(&emuladores, None, &rom, "gba", &salida).unwrap();
        assert_eq!(fs::read(&salida).unwrap().len(), 131072);
        assert!(importar_partida(&emuladores, None, &raiz.join("x.3ds"), "3ds", &sav).is_err());
        let vacio = raiz.join("vacio.sav");
        fs::write(&vacio, b"").unwrap();
        assert!(importar_partida(&emuladores, None, &rom, "gba", &vacio).is_err());
        let _ = fs::remove_dir_all(&raiz);
    }

    #[test]
    fn cambia_el_ini_de_azahar_sin_tocar_lo_demas() {
        let raiz = carpeta_temporal("ini");
        let emuladores = raiz.join("emuladores");
        let ini = emuladores.join("Azahar/user/config/qt-config.ini");
        fs::create_dir_all(ini.parent().unwrap()).unwrap();
        fs::write(&ini, "[UI]\nfullscreen=true\n\n[Data%20Storage]\nuse_virtual_sd=true\nuse_custom_storage=false\n").unwrap();

        let perfil = raiz.join("perfiles/sala-K7P2QX");
        almacenamiento_azahar(&emuladores, Some(&perfil));
        let texto = fs::read_to_string(&ini).unwrap();
        assert!(texto.contains("fullscreen=true"));
        assert!(texto.contains("use_virtual_sd=true"));
        assert!(texto.contains("use_custom_storage=true"));
        assert!(texto.contains(r"use_custom_storage\default=false"));
        assert!(texto.contains("sala-K7P2QX/sdmc/"));
        assert_eq!(texto.matches("[Data%20Storage]").count(), 1);
        assert_eq!(texto.matches("use_custom_storage=").count(), 1);

        almacenamiento_azahar(&emuladores, None);
        let texto = fs::read_to_string(&ini).unwrap();
        assert!(texto.contains("use_custom_storage=false"));
        assert!(!texto.contains("sdmc_directory"));
        assert!(texto.contains("use_virtual_sd=true"));
        let _ = fs::remove_dir_all(&raiz);
    }

    #[test]
    fn progreso_suma_tiempo_y_lee_amigos() {
        let datos = carpeta_temporal("progreso");
        let sala = SalaGuardada {
            codigo: "K7P2QX".into(),
            nombre: "Prueba".into(),
            juego: "x.3ds".into(),
            juego_titulo: "Pokémon Ultra Luna".into(),
            sistema: "3ds".into(),
            partida_propia: true,
            creada: 0,
        };
        preparar_progreso(&datos, &sala, "PC-AAAA", "Pablo");
        sumar_progreso(&datos, &sala, "PC-AAAA", "Pablo", 60_000, Some("Zm90bw=="));
        sumar_progreso(&datos, &sala, "PC-AAAA", "Pablo", 30_000, None);
        fs::create_dir_all(carpeta_amigos(&datos, "K7P2QX")).unwrap();
        fs::write(
            carpeta_amigos(&datos, "K7P2QX").join("Jugador-ZZ.json"),
            r#"{"id":"Jugador-ZZ","nombre":"Dani","jugadoMs":5,"captura":"abc"}"#,
        )
        .unwrap();
        let v = leer_progreso(&datos, "K7P2QX", "PC-AAAA", false);
        assert_eq!(v["propio"]["jugadoMs"], 90_000);
        assert_eq!(v["propio"]["partida"], "sala");
        assert!(v["propio"].get("captura").is_none());
        assert_eq!(v["amigos"][0]["nombre"], "Dani");
        let v = leer_progreso(&datos, "K7P2QX", "PC-AAAA", true);
        assert_eq!(v["propio"]["captura"], "Zm90bw==");
        let _ = fs::remove_dir_all(&datos);
    }
}
