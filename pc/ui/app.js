// Interfaz de Pokemulador para PC. Sin librerías: solo HTML, CSS y este archivo.

const SISTEMAS = {
  gb: { nombre: 'Game Boy', ini: 'GB', libretro: 'Nintendo - Game Boy' },
  gbc: { nombre: 'Game Boy Color', ini: 'GBC', libretro: 'Nintendo - Game Boy Color' },
  gba: { nombre: 'Game Boy Advance', ini: 'GBA', libretro: 'Nintendo - Game Boy Advance' },
  nds: { nombre: 'Nintendo DS', ini: 'DS', libretro: 'Nintendo - Nintendo DS' },
  '3ds': { nombre: 'Nintendo 3DS', ini: '3DS', libretro: 'Nintendo - Nintendo 3DS' },
  n64: { nombre: 'Nintendo 64', ini: 'N64', libretro: 'Nintendo - Nintendo 64' },
};

const TITULOS = { inicio: 'Inicio', consolas: 'Consolas', online: 'Online', ajustes: 'Ajustes', buscar: 'Buscar' };

const ICONO_PLAY = '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5v14l11-7z"/></svg>';

// Fuera de la app (por ejemplo, abriendo index.html en un navegador) usamos datos de ejemplo.
const tauri = window.__TAURI__ || crearSimulacion();
const invoke = tauri.core.invoke;

const estado = { biblioteca: null, vista: 'inicio', sistema: null, busqueda: '' };
const pantalla = document.getElementById('pantalla');
const titulo = document.getElementById('titulo');
const buscar = document.getElementById('buscar');

function esc(texto) {
  return String(texto).replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

function portadaUrl(juego) {
  const sistema = SISTEMAS[juego.system];
  const nombre = juego.stem.replace(/[&*/:`<>?\\|"]/g, '_');
  return `https://thumbnails.libretro.com/${encodeURIComponent(sistema.libretro)}/Named_Boxarts/${encodeURIComponent(nombre)}.png`;
}

function portada(juego) {
  return `<div class="portada" data-ini="${SISTEMAS[juego.system].ini}">` +
    `<img src="${portadaUrl(juego)}" alt="" loading="lazy" onerror="this.remove()"></div>`;
}

function juegos() { return estado.biblioteca ? estado.biblioteca.games : []; }
function indiceDe(ruta) { return juegos().findIndex((j) => j.path === ruta); }
function esFavorito(juego) { return estado.biblioteca.favorites.includes(juego.path); }

function avisar(texto) {
  const aviso = document.getElementById('aviso');
  aviso.textContent = texto;
  aviso.hidden = false;
  clearTimeout(avisar.t);
  avisar.t = setTimeout(() => { aviso.hidden = true; }, 3500);
}

// ---------- Vistas ----------

function vistaBienvenida() {
  return `<div class="bienvenida">
    <p class="grande">¡Bienvenido a Pokemulador!</p>
    <p>Elige la carpeta donde tienes tus juegos. Puede tener subcarpetas: Pokemulador encontrará los de Game Boy, GBA, DS, 3DS y N64.</p>
    <button class="boton" data-accion="elegir-carpeta">Elegir carpeta de juegos</button>
  </div>`;
}

function vistaInicio() {
  const b = estado.biblioteca;
  if (!b.roms_dir) return vistaBienvenida();

  let html = '';
  if (!b.emulators_found) {
    html += '<div class="alerta">No se encuentran los emuladores. Reinstala Pokemulador.</div>';
  }
  if (juegos().length === 0) {
    html += `<div class="bienvenida"><p class="grande">No hay juegos en esa carpeta</p>
      <p class="ruta">${esc(b.roms_dir)}</p>
      <button class="boton" data-accion="elegir-carpeta">Elegir otra carpeta</button></div>`;
    return html;
  }

  const recientes = b.recents.map(indiceDe).filter((i) => i >= 0);
  if (recientes.length > 0) {
    const i = recientes[0];
    const j = juegos()[i];
    html += `<h2>Seguir jugando</h2>
      <div class="seguir" data-accion="jugar" data-i="${i}">
        ${portada(j)}
        <div class="datos"><div class="nombre">${esc(j.title)}</div><div class="suave">${SISTEMAS[j.system].nombre}</div></div>
        <button class="play" data-accion="jugar" data-i="${i}" aria-label="Jugar">${ICONO_PLAY}</button>
      </div>`;
  }

  html += `<div class="online-banner">
      <div class="texto">Jugar online con tus amigos</div>
      <button class="boton" data-accion="ir" data-vista="online">Crear sala</button>
      <button class="boton sec" data-accion="ir" data-vista="online">Unirme</button>
    </div>`;

  html += fila('Recientes', recientes.slice(1, 13));
  html += fila('Favoritos', b.favorites.map(indiceDe).filter((i) => i >= 0));
  if (recientes.length === 0) {
    html += fila('Tus juegos', juegos().map((_, i) => i).slice(0, 12));
  }
  return html;
}

function fila(nombre, indices) {
  if (indices.length === 0) return '';
  const tarjetas = indices.map((i) => {
    const j = juegos()[i];
    return `<div class="mini" data-accion="jugar" data-i="${i}" title="${esc(j.title)}">
      ${portada(j)}<div class="nombre">${esc(j.title)}</div></div>`;
  }).join('');
  return `<h2>${nombre}</h2><div class="fila">${tarjetas}</div>`;
}

function vistaConsolas() {
  if (!estado.biblioteca.roms_dir) return vistaBienvenida();
  if (estado.sistema) return vistaListaSistema(estado.sistema);

  const cartuchos = Object.entries(SISTEMAS).map(([id, s]) => {
    const n = juegos().filter((j) => j.system === id).length;
    return `<button class="cartucho ${n === 0 ? 'vacio' : ''}" data-accion="sistema" data-sistema="${id}">
      <div class="etiqueta">${s.ini}</div>
      <div class="nombre">${s.nombre}</div>
      <div class="suave">${n === 1 ? '1 juego' : `${n} juegos`}</div>
    </button>`;
  }).join('');
  return `<div class="cartuchos">${cartuchos}</div>`;
}

function vistaListaSistema(sistema) {
  const indices = juegos().map((j, i) => (j.system === sistema ? i : -1)).filter((i) => i >= 0);
  return `<button class="volver" data-accion="volver">← Consolas</button>
    <h2>${SISTEMAS[sistema].nombre}</h2>
    ${lista(indices, false)}`;
}

function lista(indices, conSistema) {
  if (indices.length === 0) return '<p class="suave">No hay juegos aquí.</p>';
  const filas = indices.map((i, n) => {
    const j = juegos()[i];
    const fav = esFavorito(j);
    return `<div class="entrada" data-accion="jugar" data-i="${i}">
      <span class="num">Nº ${String(n + 1).padStart(3, '0')}</span>
      ${portada(j)}
      <div><div class="nombre">${esc(j.title)}</div>${conSistema ? `<div class="sistema">${SISTEMAS[j.system].nombre}</div>` : ''}</div>
      <button class="fav ${fav ? 'si' : ''}" data-accion="fav" data-i="${i}" aria-label="${fav ? 'Quitar de favoritos' : 'Añadir a favoritos'}">★</button>
      <button class="play" data-accion="jugar" data-i="${i}" aria-label="Jugar">${ICONO_PLAY}</button>
    </div>`;
  }).join('');
  return `<div class="lista">${filas}</div>`;
}

function vistaBuscar() {
  const q = estado.busqueda.toLowerCase();
  const indices = juegos().map((j, i) => (j.title.toLowerCase().includes(q) ? i : -1)).filter((i) => i >= 0);
  return `<h2>Resultados para «${esc(estado.busqueda)}»</h2>${lista(indices, true)}`;
}

function vistaOnline() {
  const casillas = '<span>·</span>'.repeat(6);
  return `<span class="etiqueta-pronto">Próximamente</span>
    <h2>Tu sala</h2>
    <div class="codigo">${casillas}</div>
    <p>Muy pronto podrás crear una sala, pasarle el código a tus amigos y jugar juntos aunque estéis en redes distintas. Servirá también para jugar del PC contra el móvil.</p>
    <div class="separador"></div>
    <h2>Unirme con código</h2>
    <div class="codigo">${casillas}</div>`;
}

function vistaAjustes() {
  const b = estado.biblioteca;
  return `<div class="ajuste">
      <h2>Carpeta de juegos</h2>
      <p class="ruta">${b.roms_dir ? esc(b.roms_dir) : 'Todavía no has elegido ninguna.'}</p>
      <p class="suave">${juegos().length} juegos encontrados</p>
      <div class="fila-botones">
        <button class="boton" data-accion="elegir-carpeta">Cambiar carpeta</button>
        ${b.roms_dir ? '<button class="boton sec" data-accion="reescanear">Volver a buscar juegos</button>' : ''}
        ${b.roms_dir ? '<button class="boton sec" data-accion="abrir-carpeta">Abrir carpeta</button>' : ''}
      </div>
    </div>
    <div class="ajuste">
      <label class="interruptor"><input type="checkbox" data-accion="pantalla-completa" ${b.fullscreen ? 'checked' : ''}> Jugar a pantalla completa</label>
    </div>
    <div class="ajuste">
      <h2>Controles con teclado</h2>
      <p class="suave">Game Boy, GBA, DS y N64 (RetroArch). Los mandos se reconocen solos.</p>
      <div class="teclas">
        <kbd>Flechas</kbd><span>Cruceta</span>
        <kbd>X</kbd><span>Botón A</span>
        <kbd>Z</kbd><span>Botón B</span>
        <kbd>Intro</kbd><span>Start</span>
        <kbd>Mayús dcha.</kbd><span>Select</span>
        <kbd>Q / W</kbd><span>L / R</span>
        <kbd>F1</kbd><span>Menú del emulador</span>
        <kbd>Esc</kbd><span>Salir del juego y volver a Pokemulador</span>
      </div>
      <p class="suave" style="margin-top:12px">En 3DS (Azahar) los controles se cambian en su menú Emulación › Configurar.</p>
    </div>`;
}

function pintar() {
  if (!estado.biblioteca) return;
  let vista = estado.vista;
  if (estado.busqueda) vista = 'buscar';
  titulo.textContent = vista === 'consolas' && estado.sistema ? SISTEMAS[estado.sistema].nombre : TITULOS[vista];

  const html = {
    inicio: vistaInicio,
    consolas: vistaConsolas,
    online: vistaOnline,
    ajustes: vistaAjustes,
    buscar: vistaBuscar,
  }[vista]();
  pantalla.innerHTML = html;

  document.querySelectorAll('.menu button').forEach((boton) => {
    boton.classList.toggle('activo', boton.dataset.vista === estado.vista && !estado.busqueda);
  });
}

// ---------- Acciones ----------

async function cargar() {
  estado.biblioteca = await invoke('library');
  pintar();
}

async function elegirCarpeta() {
  const carpeta = await tauri.dialog.open({ directory: true, title: 'Elige la carpeta de tus juegos' });
  if (!carpeta) return;
  estado.biblioteca = await invoke('set_roms_dir', { dir: carpeta });
  avisar(`${estado.biblioteca.games.length} juegos encontrados`);
  pintar();
}

async function jugar(i) {
  const juego = juegos()[i];
  try {
    await invoke('launch', { path: juego.path });
    avisar(`Abriendo ${juego.title}…`);
    estado.biblioteca.recents = [juego.path, ...estado.biblioteca.recents.filter((p) => p !== juego.path)];
  } catch (error) {
    avisar(String(error));
  }
}

async function alternarFavorito(i) {
  const juego = juegos()[i];
  const ahora = await invoke('toggle_favorite', { path: juego.path });
  const favs = estado.biblioteca.favorites.filter((p) => p !== juego.path);
  estado.biblioteca.favorites = ahora ? [...favs, juego.path] : favs;
  pintar();
}

pantalla.addEventListener('click', (evento) => {
  const objetivo = evento.target.closest('[data-accion]');
  if (!objetivo || objetivo.dataset.accion === 'pantalla-completa') return;
  evento.stopPropagation();
  const i = Number(objetivo.dataset.i);
  switch (objetivo.dataset.accion) {
    case 'jugar': jugar(i); break;
    case 'fav': alternarFavorito(i); break;
    case 'sistema': estado.sistema = objetivo.dataset.sistema; pintar(); pantalla.scrollTop = 0; break;
    case 'volver': estado.sistema = null; pintar(); break;
    case 'ir': irA(objetivo.dataset.vista); break;
    case 'elegir-carpeta': elegirCarpeta(); break;
    case 'reescanear': cargar().then(() => avisar(`${juegos().length} juegos encontrados`)); break;
    case 'abrir-carpeta': invoke('open_folder', { path: estado.biblioteca.roms_dir }); break;
    default: break;
  }
});

pantalla.addEventListener('change', (evento) => {
  if (evento.target.dataset.accion === 'pantalla-completa') {
    estado.biblioteca.fullscreen = evento.target.checked;
    invoke('set_fullscreen', { value: evento.target.checked });
  }
});

function irA(vista) {
  estado.vista = vista;
  estado.sistema = null;
  estado.busqueda = '';
  buscar.value = '';
  pintar();
  pantalla.scrollTop = 0;
}

document.querySelectorAll('.menu button').forEach((boton) => {
  boton.addEventListener('click', () => irA(boton.dataset.vista));
});

buscar.addEventListener('input', () => {
  estado.busqueda = buscar.value.trim();
  pintar();
});

tauri.event.listen('juego-cerrado', () => cargar());

cargar();

// ---------- Datos de ejemplo para ver la interfaz fuera de la app ----------

function crearSimulacion() {
  const ejemplo = [
    ['Pokemon - FireRed Version (USA, Europe)', 'gba'],
    ['Pokemon - Emerald Version (USA, Europe)', 'gba'],
    ['Pokemon - Platinum Version (USA)', 'nds'],
    ['Pokemon - HeartGold Version (USA)', 'nds'],
    ['Pokemon - Crystal Version (USA, Europe)', 'gbc'],
    ['Pokemon - Red Version (USA, Europe)', 'gb'],
    ['Pokemon Stadium 2 (USA)', 'n64'],
    ['Pokemon X', '3ds'],
  ].map(([stem, system]) => ({ path: `C:\\Juegos\\${stem}.${system}`, stem, title: stem.replace(/\s*\(.*?\)/g, ''), system }));
  const datos = {
    roms_dir: 'C:\\Juegos', games: ejemplo, recents: [ejemplo[2].path, ejemplo[0].path, ejemplo[4].path],
    favorites: [ejemplo[3].path], fullscreen: true, emulators_found: true,
  };
  return {
    core: { invoke: async (cmd) => (cmd === 'library' || cmd === 'set_roms_dir' ? datos : true) },
    dialog: { open: async () => 'C:\\Juegos' },
    event: { listen: () => {} },
  };
}
