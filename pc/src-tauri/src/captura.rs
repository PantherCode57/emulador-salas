//! Captura de la ventana del emulador para que los amigos de la sala vean por dónde vas.
//! Devuelve un JPEG pequeño en base64, igual que la app de Android.

/// Lado mayor de la captura, en píxeles.
const LADO: u32 = 480;

/// Captura la ventana principal del proceso `pid` y la devuelve como JPEG en base64.
pub fn capturar_base64(pid: u32) -> Option<String> {
    let (ancho, alto, rgb) = ventana::capturar(pid)?;
    let (ancho, alto, rgb) = reducir(ancho, alto, &rgb);
    let mut jpeg = Vec::new();
    jpeg_encoder::Encoder::new(&mut jpeg, 70)
        .encode(&rgb, ancho as u16, alto as u16, jpeg_encoder::ColorType::Rgb)
        .ok()?;
    Some(base64(&jpeg))
}

/// Reduce la imagen promediando bloques de píxeles (queda suave aunque el juego sea pixel art).
fn reducir(ancho: u32, alto: u32, rgb: &[u8]) -> (u32, u32, Vec<u8>) {
    let mayor = ancho.max(alto);
    if mayor <= LADO {
        return (ancho, alto, rgb.to_vec());
    }
    let nuevo_ancho = (ancho * LADO / mayor).max(1);
    let nuevo_alto = (alto * LADO / mayor).max(1);
    let mut salida = Vec::with_capacity((nuevo_ancho * nuevo_alto * 3) as usize);
    for y in 0..nuevo_alto {
        let y0 = y * alto / nuevo_alto;
        let y1 = ((y + 1) * alto / nuevo_alto).max(y0 + 1);
        for x in 0..nuevo_ancho {
            let x0 = x * ancho / nuevo_ancho;
            let x1 = ((x + 1) * ancho / nuevo_ancho).max(x0 + 1);
            let mut suma = [0u32; 3];
            let mut n = 0u32;
            for yy in y0..y1 {
                for xx in x0..x1 {
                    let i = ((yy * ancho + xx) * 3) as usize;
                    suma[0] += rgb[i] as u32;
                    suma[1] += rgb[i + 1] as u32;
                    suma[2] += rgb[i + 2] as u32;
                    n += 1;
                }
            }
            salida.extend(suma.iter().map(|s| (s / n) as u8));
        }
    }
    (nuevo_ancho, nuevo_alto, salida)
}

pub fn base64(datos: &[u8]) -> String {
    const TABLA: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut salida = String::with_capacity(datos.len().div_ceil(3) * 4);
    for trozo in datos.chunks(3) {
        let b = [trozo[0], *trozo.get(1).unwrap_or(&0), *trozo.get(2).unwrap_or(&0)];
        let n = ((b[0] as u32) << 16) | ((b[1] as u32) << 8) | b[2] as u32;
        salida.push(TABLA[(n >> 18) as usize & 63] as char);
        salida.push(TABLA[(n >> 12) as usize & 63] as char);
        salida.push(if trozo.len() > 1 { TABLA[(n >> 6) as usize & 63] as char } else { '=' });
        salida.push(if trozo.len() > 2 { TABLA[n as usize & 63] as char } else { '=' });
    }
    salida
}

#[cfg(windows)]
mod ventana {
    use std::mem::{size_of, zeroed};
    use windows_sys::core::BOOL;
    use windows_sys::Win32::Foundation::{HWND, LPARAM, POINT, RECT};
    use windows_sys::Win32::Graphics::Gdi::{
        BitBlt, ClientToScreen, CreateCompatibleBitmap, CreateCompatibleDC, DeleteDC, DeleteObject, GetDC,
        GetDIBits, ReleaseDC, SelectObject, BITMAPINFO, BITMAPINFOHEADER, BI_RGB, CAPTUREBLT, DIB_RGB_COLORS,
        SRCCOPY,
    };
    use windows_sys::Win32::Storage::Xps::PrintWindow;
    use windows_sys::Win32::UI::HiDpi::{SetThreadDpiAwarenessContext, DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2};
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        EnumWindows, GetClientRect, GetForegroundWindow, GetWindow, GetWindowThreadProcessId, IsWindowVisible,
        GW_OWNER,
    };

    /// PW_CLIENTONLY | PW_RENDERFULLCONTENT: solo el interior, también si se dibuja con OpenGL o Vulkan.
    const MODO_CAPTURA: u32 = 0x1 | 0x2;

    struct Busqueda {
        pid: u32,
        mejor: HWND,
        area: i64,
    }

    unsafe extern "system" fn revisar(hwnd: HWND, datos: LPARAM) -> BOOL {
        let busqueda = &mut *(datos as *mut Busqueda);
        let mut pid = 0u32;
        GetWindowThreadProcessId(hwnd, &mut pid);
        if pid == busqueda.pid && IsWindowVisible(hwnd) != 0 && GetWindow(hwnd, GW_OWNER).is_null() {
            let mut r: RECT = zeroed();
            if GetClientRect(hwnd, &mut r) != 0 {
                let area = (r.right - r.left) as i64 * (r.bottom - r.top) as i64;
                if area > busqueda.area {
                    busqueda.area = area;
                    busqueda.mejor = hwnd;
                }
            }
        }
        1
    }

    /// Píxeles RGB de la ventana más grande del proceso.
    pub fn capturar(pid: u32) -> Option<(u32, u32, Vec<u8>)> {
        unsafe {
            // Medidas en píxeles reales, aunque Windows tenga la pantalla escalada (125 %, 150 %...).
            let antes = SetThreadDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
            let resultado = capturar_sin_escala(pid);
            if !antes.is_null() {
                SetThreadDpiAwarenessContext(antes);
            }
            resultado
        }
    }

    unsafe fn capturar_sin_escala(pid: u32) -> Option<(u32, u32, Vec<u8>)> {
        {
            let mut busqueda = Busqueda { pid, mejor: std::ptr::null_mut(), area: 0 };
            EnumWindows(Some(revisar), &mut busqueda as *mut Busqueda as LPARAM);
            let hwnd = busqueda.mejor;
            if hwnd.is_null() {
                return None;
            }
            let mut r: RECT = zeroed();
            GetClientRect(hwnd, &mut r);
            let (ancho, alto) = (r.right - r.left, r.bottom - r.top);
            if ancho < 16 || alto < 16 {
                return None;
            }

            // Si el juego está delante, copiamos lo que se ve en pantalla en su sitio: así sale bien
            // aunque el emulador dibuje con Direct3D, OpenGL o Vulkan. Si no, se le pide a la ventana
            // que se pinte ella (no siempre funciona; si sale de un solo color, se descarta).
            let delante = GetForegroundWindow() == hwnd;
            let dc_pantalla = GetDC(std::ptr::null_mut());
            let dc = CreateCompatibleDC(dc_pantalla);
            let mapa = CreateCompatibleBitmap(dc_pantalla, ancho, alto);
            let anterior = SelectObject(dc, mapa);
            let dibujado = if delante {
                let mut origen = POINT { x: 0, y: 0 };
                ClientToScreen(hwnd, &mut origen);
                BitBlt(dc, 0, 0, ancho, alto, dc_pantalla, origen.x, origen.y, SRCCOPY | CAPTUREBLT) != 0
            } else {
                PrintWindow(hwnd, dc, MODO_CAPTURA) != 0
            };

            let mut info: BITMAPINFO = zeroed();
            info.bmiHeader.biSize = size_of::<BITMAPINFOHEADER>() as u32;
            info.bmiHeader.biWidth = ancho;
            info.bmiHeader.biHeight = -alto; // de arriba abajo
            info.bmiHeader.biPlanes = 1;
            info.bmiHeader.biBitCount = 32;
            info.bmiHeader.biCompression = BI_RGB;
            let mut bgra = vec![0u8; (ancho * alto * 4) as usize];
            let lineas = GetDIBits(
                dc,
                mapa,
                0,
                alto as u32,
                bgra.as_mut_ptr().cast(),
                &mut info,
                DIB_RGB_COLORS,
            );

            SelectObject(dc, anterior);
            DeleteObject(mapa);
            DeleteDC(dc);
            ReleaseDC(std::ptr::null_mut(), dc_pantalla);
            if !dibujado || lineas == 0 {
                return None;
            }

            let mut rgb = Vec::with_capacity((ancho * alto * 3) as usize);
            let primero = [bgra[0], bgra[1], bgra[2]];
            let mut distinto = false;
            for p in bgra.chunks_exact(4) {
                distinto |= (0..3).any(|c| p[c].abs_diff(primero[c]) > 12);
                rgb.extend_from_slice(&[p[2], p[1], p[0]]);
            }
            // Una captura de un solo color (toda negra o toda blanca) no sirve.
            if !distinto {
                return None;
            }
            Some((ancho as u32, alto as u32, rgb))
        }
    }
}

#[cfg(not(windows))]
mod ventana {
    pub fn capturar(_pid: u32) -> Option<(u32, u32, Vec<u8>)> {
        None
    }
}

#[cfg(test)]
mod pruebas {
    #[test]
    fn base64_como_el_estandar() {
        assert_eq!(super::base64(b"foto"), "Zm90bw==");
        assert_eq!(super::base64(b"Pokemon"), "UG9rZW1vbg==");
        assert_eq!(super::base64(b"abc"), "YWJj");
    }

    #[test]
    fn reduce_al_tamano_de_la_captura() {
        let (w, h, px) = super::reducir(1920, 1080, &vec![200u8; 1920 * 1080 * 3]);
        assert_eq!((w, h), (480, 270));
        assert_eq!(px.len(), (480 * 270 * 3) as usize);
        assert!(px.iter().all(|&v| v == 200));
    }
}

#[cfg(all(test, windows))]
mod prueba_real {
    /// cargo test --release -- --ignored captura_de_una_ventana_de_verdad
    #[test]
    #[ignore]
    fn captura_de_una_ventana_de_verdad() {
        // RetroArch tal cual (sin juego) abre su menú: es la misma ventana que al jugar.
        let dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../emuladores/RetroArch");
        let mut bloc = std::process::Command::new(dir.join("retroarch.exe")).current_dir(&dir).spawn().unwrap();
        std::thread::sleep(std::time::Duration::from_secs(6));
        let resultado = super::capturar_base64(bloc.id());
        let _ = bloc.kill();
        let b64 = resultado.expect("no se pudo capturar la ventana");
        assert!(b64.len() > 1000);
        std::fs::write(std::env::temp_dir().join("captura-prueba.b64"), b64).unwrap();
    }
}
