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

const estado = {
  biblioteca: null,
  vista: 'inicio',
  sistema: null,
  busqueda: '',
  online: { clave: false, id: null, nombre: null, salas: [], activa: null, fase: null, error: null, amigos: 0, ruta: null, progreso: null },
};
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
      <button class="mas" data-accion="partida" data-i="${i}" title="Cargar o guardar la partida" aria-label="Cargar o guardar la partida">⋯</button>
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

function casillasCodigo(codigo) {
  return [...codigo.padEnd(6, '·')].map((c) => `<span class="${c === '·' ? '' : 'lleno'}">${esc(c)}</span>`).join('');
}

// ---------- Online: salas guardadas ----------

function textoTiempo(ms) {
  const min = Math.floor((ms || 0) / 60000);
  if (min < 1) return 'menos de 1 min';
  if (min < 60) return `${min} min`;
  return `${Math.floor(min / 60)} h ${min % 60} min`;
}

function haceCuanto(ms) {
  if (!ms) return '';
  const seg = Math.max(0, Math.round((Date.now() - ms) / 1000));
  if (seg < 60) return 'hace un momento';
  const min = Math.round(seg / 60);
  if (min < 60) return `hace ${min} min`;
  const h = Math.round(min / 60);
  if (h < 24) return `hace ${h} ${h === 1 ? 'hora' : 'horas'}`;
  const d = Math.round(h / 24);
  return `hace ${d} ${d === 1 ? 'día' : 'días'}`;
}

function textoPartida(propia) { return propia ? 'partida de la sala' : 'partida de siempre'; }

function salaActiva() {
  const o = estado.online;
  return o.activa ? o.salas.find((s) => s.codigo === o.activa.codigo) : null;
}

function tarjetaProgreso(titulo, p) {
  const captura = p.captura ? `<img class="captura" src="data:image/jpeg;base64,${p.captura}" alt="Por dónde va">` : '';
  return `<div class="progreso">${captura}
    <div><div class="nombre">${esc(titulo)}</div>
      ${p.juego ? `<div>${esc(p.juego)}</div>` : ''}
      <div class="suave">${textoPartida(p.partida === 'sala')}</div>
      <div><strong>${textoTiempo(p.jugadoMs)} jugado</strong></div>
      <div class="suave">${p.ultimaVez ? `Última vez: ${haceCuanto(p.ultimaVez)}` : 'Todavía no ha jugado en la sala'}</div>
    </div></div>`;
}

function rolDecidido() {
  const a = estado.online.activa;
  return Boolean(a) && a.anfitrion !== null && a.anfitrion !== undefined;
}

function textoFaseSala() {
  const o = estado.online;
  if (o.fase === 'error') return o.error || 'Algo ha fallado en la sala.';
  if (o.fase === 'conectando' || !o.fase) return 'Conectando con Tailscale…';
  if (!rolDecidido()) return 'Mirando si tu amigo ya está dentro…';
  if (o.amigos > 1) return `${o.amigos} amigos en la sala contigo.`;
  if (o.amigos > 0) return 'Tu amigo está en la sala contigo.';
  if (o.activa.anfitrion) return 'Sala abierta. Puedes jugar ya: cuando tu amigo entre, os conectáis solos.';
  return 'Tu amigo no está ahora. Puedes jugar igual: si vuelve, os conectáis solos.';
}

function vistaSalaActiva(sala) {
  const o = estado.online;
  const listo = rolDecidido() && o.fase !== 'error';
  const prog = o.progreso || { propio: null, amigos: [] };
  const ruta = o.ruta ? `<p class="suave">conexión ${o.ruta === 'directa' ? 'directa' : 'por repetidor'}</p>` : '';
  const ayuda = sala.sistema === '3ds'
    ? `<ol class="pasos">
        <li>Pulsa <strong>Jugar</strong>: se abre ${esc(sala.juego_titulo)} con tu ${textoPartida(sala.partida_propia)}.</li>
        <li>En Azahar, ve a <strong>Multijugador › Conectar directamente a una sala</strong> (si está a pantalla completa, pulsa Esc para ver los menús).</li>
        <li>La dirección ya está puesta. En <strong>Contraseña</strong> escribe <strong>${esc(sala.codigo)}</strong> y pulsa Conectar.</li>
        <li>En el juego, usa la comunicación local (Festival Plaza, PSS…) para intercambiar o combatir.</li>
      </ol>
      <p class="suave">Cada jugador necesita un nombre distinto en Azahar: Emulación › Configurar › Sistema › Nombre de usuario.</p>`
    : '<p class="suave">En este juego no hay intercambios ni combates online (solo en 3DS), pero tu amigo verá cómo vas.</p>';
  const amigos = prog.amigos.length
    ? prog.amigos.map((p) => tarjetaProgreso(`Cómo va ${p.nombre || p.id}`, p)).join('')
    : '<div class="alerta">Todavía no sabes nada de tu amigo en esta sala. Cuando coincidáis dentro, veréis cómo va cada uno, y se queda guardado para luego.</div>';
  return `<div class="ajuste sala-activa">
      <div class="suave">Estás en la sala</div>
      <h2>${esc(sala.nombre)}</h2>
      <p>${esc(sala.juego_titulo)} · ${textoPartida(sala.partida_propia)}</p>
      <div class="codigo">${casillasCodigo(sala.codigo)}</div>
      <p class="${o.fase === 'error' ? 'error' : ''}"><strong>${esc(textoFaseSala())}</strong></p>
      ${ruta}
      <div class="fila-botones">
        <button class="boton" data-accion="jugar-sala" ${listo ? '' : 'disabled'}>Jugar</button>
        <button class="boton sec" data-accion="copiar-codigo">Copiar código</button>
        <button class="boton sec" data-accion="salir-sala">Salir de la sala</button>
      </div>
      ${ayuda}
    </div>
    ${amigos}
    ${prog.propio ? tarjetaProgreso('Cómo vas tú', prog.propio) : ''}`;
}

function tarjetaSala(sala) {
  const amigos = ((sala.progreso || {}).amigos || []).map((p) =>
    `<div class="suave">${esc(p.nombre || p.id)}: ${textoTiempo(p.jugadoMs)} jugado${p.ultimaVez ? ` · ${haceCuanto(p.ultimaVez)}` : ''}</div>`).join('');
  return `<div class="ajuste">
      <div class="fila-sala">
        <div><div class="nombre">${esc(sala.nombre)}</div>
          <div class="suave">${esc(sala.juego_titulo)} · ${textoPartida(sala.partida_propia)}</div>
          <div class="ruta">Código ${esc(sala.codigo)}</div></div>
        <button class="boton" data-accion="entrar-sala" data-codigo="${esc(sala.codigo)}">Entrar</button>
      </div>
      ${amigos}
      <button class="volver borrar" data-accion="borrar-sala" data-codigo="${esc(sala.codigo)}">Borrar sala</button>
    </div>`;
}

function vistaOnline() {
  const o = estado.online;
  if (!o.clave) {
    return '<div class="alerta">Esta versión de Pokemulador no tiene la clave de Tailscale, así que no puede jugar online.</div>';
  }
  const activa = salaActiva();
  let html = activa
    ? vistaSalaActiva(activa)
    : `<p>Crea una sala para un juego y pásale el código a tu amigo. La sala se queda guardada: entráis cuando queráis, cada uno avanza con su partida y, cuando coincidís, veis cómo va el otro.</p>
       <p class="suave">Los intercambios y combates online son en juegos de 3DS (también con amigos en el móvil). En el resto de juegos la sala sirve para ver cómo va cada uno.</p>
       ${o.fase === 'error' && o.error ? `<div class="alerta">${esc(o.error)}</div>` : ''}`;
  html += `<div class="fila-botones" style="margin:16px 0">
      <button class="boton" data-accion="nueva-sala" data-modo="crear">Crear sala</button>
      <button class="boton sec" data-accion="nueva-sala" data-modo="unirse">Unirme con código</button>
    </div>
    <div class="ajuste"><div class="suave">Tu nombre en las salas</div>
      <div class="fila-botones"><strong style="flex:1">${esc(o.nombre || '')}</strong>
        <button class="boton sec" data-accion="cambiar-nombre">Cambiar</button></div></div>`;
  const otras = o.salas.filter((s) => !activa || s.codigo !== activa.codigo);
  if (otras.length || !activa) html += '<h2>Mis salas</h2>';
  html += o.salas.length === 0
    ? '<p class="suave">Todavía no tienes salas. Crea una o únete con el código de un amigo.</p>'
    : otras.map(tarjetaSala).join('');
  return html;
}

// ---------- Ventanas (crear sala, partidas) ----------

const modal = document.getElementById('modal');

function abrirModal(html) {
  modal.innerHTML = `<div class="caja" role="dialog" aria-modal="true">${html}</div>`;
  modal.hidden = false;
  const primero = modal.querySelector('input, select, button');
  if (primero) primero.focus();
}

function cerrarModal() { modal.hidden = true; modal.innerHTML = ''; }

function opcionesJuegos() {
  const orden = (j) => (j.system === '3ds' ? 0 : 1);
  const ordenados = juegos().map((j, i) => [j, i]).sort((a, b) => orden(a[0]) - orden(b[0]));
  return ordenados.map(([j, i]) => `<option value="${i}">${esc(j.title)} (${SISTEMAS[j.system].ini})</option>`).join('');
}

function modalNuevaSala(modo) {
  if (juegos().length === 0) { avisar('Primero elige la carpeta de tus juegos en Ajustes.'); return; }
  abrirModal(`<h2>${modo === 'crear' ? 'Crear sala' : 'Unirme con código'}</h2>
    ${modo === 'unirse' ? `<label class="campo">Código de la sala
      <input id="m-codigo" class="entrada-codigo" maxlength="6" placeholder="K7P2QX" autocomplete="off"></label>` : ''}
    <label class="campo">Juego<select id="m-juego">${opcionesJuegos()}</select></label>
    <div class="campo">¿Con qué partida juegas en esta sala?
      <label class="opcion"><input type="radio" name="m-partida" value="siempre" checked>
        <span><strong>Mi partida de siempre</strong><br><span class="suave">Lo que avances en la sala también cuenta cuando juegues fuera de ella.</span></span></label>
      <label class="opcion"><input type="radio" name="m-partida" value="propia">
        <span><strong>Partida nueva solo para esta sala</strong><br><span class="suave">Empiezas de cero aquí y tu partida de siempre se queda tal cual.</span></span></label>
    </div>
    <label class="campo">Nombre de la sala (opcional)<input id="m-nombre" maxlength="30" autocomplete="off"></label>
    <p id="m-error" class="error" hidden></p>
    <div class="fila-botones">
      <button class="boton" data-accion="m-crear-sala" data-modo="${modo}">${modo === 'crear' ? 'Crear' : 'Entrar'}</button>
      <button class="boton sec" data-accion="m-cerrar">Cancelar</button>
    </div>`);
}

async function confirmarNuevaSala(modo) {
  const error = document.getElementById('m-error');
  const juego = juegos()[Number(document.getElementById('m-juego').value)];
  const propia = modal.querySelector('input[name="m-partida"]:checked').value === 'propia';
  const nombre = document.getElementById('m-nombre').value;
  try {
    let sala;
    if (modo === 'crear') {
      sala = await invoke('crear_sala', { nombre, juego: juego.path, partidaPropia: propia });
    } else {
      const codigo = document.getElementById('m-codigo').value.trim().toUpperCase();
      if (codigo.length !== 6) throw new Error('El código tiene 6 letras o números.');
      sala = await invoke('guardar_sala', { codigo, nombre, juego: juego.path, partidaPropia: propia });
    }
    cerrarModal();
    await entrarSala(sala.codigo);
  } catch (e) {
    error.textContent = e.message || String(e);
    error.hidden = false;
  }
}

function modalNombre() {
  abrirModal(`<h2>Tu nombre en las salas</h2>
    <label class="campo">Es lo que ven tus amigos<input id="m-nombre-jugador" maxlength="24" value="${esc(estado.online.nombre || '')}" autocomplete="off"></label>
    <div class="fila-botones">
      <button class="boton" data-accion="m-guardar-nombre">Guardar</button>
      <button class="boton sec" data-accion="m-cerrar">Cancelar</button>
    </div>`);
}

function salasConPartidaPropia(juego) {
  return estado.online.salas.filter((s) => s.partida_propia && s.juego === juego.path);
}

function modalPartida(i) {
  const juego = juegos()[i];
  if (juego.system === '3ds') {
    avisar('En 3DS la partida va dentro de la tarjeta SD de Azahar: no se puede cargar como archivo .sav.');
    return;
  }
  const salas = salasConPartidaPropia(juego);
  const destino = salas.length
    ? `<label class="campo">¿Qué partida?<select id="m-perfil"><option value="">Mi partida de siempre</option>
        ${salas.map((s) => `<option value="${esc(s.codigo)}">Partida de la sala «${esc(s.nombre)}»</option>`).join('')}</select></label>`
    : '';
  abrirModal(`<h2>Partida de ${esc(juego.title)}</h2>
    <p>Puedes cargar un archivo <strong>.sav</strong> (por ejemplo, el que bajas de linkcable.app con «Download save»). La partida que tengas ahora se guarda como copia por si acaso.</p>
    <p class="suave">También puedes sacar tu partida a un .sav para llevarla a linkcable.app (con «Load save») o a otro emulador.</p>
    ${destino}
    <div class="fila-botones">
      <button class="boton" data-accion="m-importar" data-i="${i}">Cargar partida (.sav)</button>
      <button class="boton sec" data-accion="m-exportar" data-i="${i}">Guardar en un archivo</button>
      <button class="boton sec" data-accion="m-cerrar">Cerrar</button>
    </div>`);
}

function perfilElegido() {
  const campo = document.getElementById('m-perfil');
  return campo && campo.value ? campo.value : null;
}

async function importarPartida(i) {
  const juego = juegos()[i];
  const codigo = perfilElegido();
  const archivo = await tauri.dialog.open({
    title: 'Elige la partida (.sav)',
    filters: [{ name: 'Partidas', extensions: ['sav', 'srm', 'dsv'] }, { name: 'Todos los archivos', extensions: ['*'] }],
  });
  if (!archivo) return;
  try {
    avisar(await invoke('importar_partida', { juego: juego.path, archivo, codigo }));
    cerrarModal();
  } catch (e) { avisar(String(e)); }
}

async function exportarPartida(i) {
  const juego = juegos()[i];
  const codigo = perfilElegido();
  const archivo = await tauri.dialog.save({
    title: 'Guardar la partida',
    defaultPath: `${juego.stem}.sav`,
    filters: [{ name: 'Partidas', extensions: ['sav'] }],
  });
  if (!archivo) return;
  try {
    avisar(await invoke('exportar_partida', { juego: juego.path, archivo, codigo }));
    cerrarModal();
  } catch (e) { avisar(String(e)); }
}

modal.addEventListener('click', (evento) => {
  if (evento.target === modal) { cerrarModal(); return; }
  const objetivo = evento.target.closest('[data-accion]');
  if (!objetivo) return;
  switch (objetivo.dataset.accion) {
    case 'm-cerrar': cerrarModal(); break;
    case 'm-crear-sala': confirmarNuevaSala(objetivo.dataset.modo); break;
    case 'm-guardar-nombre':
      invoke('cambiar_nombre', { nombre: document.getElementById('m-nombre-jugador').value }).then((nombre) => {
        estado.online.nombre = nombre;
        cerrarModal();
        pintar();
      });
      break;
    case 'm-importar': importarPartida(Number(objetivo.dataset.i)); break;
    case 'm-exportar': exportarPartida(Number(objetivo.dataset.i)); break;
    default: break;
  }
});

modal.addEventListener('input', (evento) => {
  if (evento.target.id === 'm-codigo') {
    evento.target.value = evento.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '');
  }
});

document.addEventListener('keydown', (evento) => {
  if (evento.key === 'Escape' && !modal.hidden) cerrarModal();
});

// ---------- Online: acciones ----------

async function cargarSalas() {
  const datos = await invoke('salas_guardadas');
  const o = estado.online;
  o.clave = datos.clave;
  o.id = datos.id;
  o.nombre = datos.nombre;
  o.salas = datos.salas || [];
  if (datos.activa) {
    if (!o.activa || o.activa.codigo !== datos.activa.codigo) {
      o.fase = 'conectando'; o.amigos = 0; o.ruta = null; o.error = null;
    }
    o.activa = { ...(o.activa || {}), ...datos.activa };
    o.progreso = await invoke('progreso_sala', { codigo: datos.activa.codigo });
  } else {
    o.activa = null;
    o.progreso = null;
  }
}

async function refrescarOnline() {
  await cargarSalas();
  if (estado.vista === 'online') pintar();
}

async function entrarSala(codigo) {
  try {
    await invoke('entrar_sala', { codigo });
    Object.assign(estado.online, { activa: { codigo, anfitrion: null }, fase: 'conectando', error: null, amigos: 0, ruta: null, progreso: null });
  } catch (e) {
    Object.assign(estado.online, { activa: null, fase: 'error', error: String(e) });
  }
  await refrescarOnline();
}

async function salirSala() {
  await invoke('salir_sala');
  Object.assign(estado.online, { activa: null, fase: null, error: null, amigos: 0, ruta: null, progreso: null });
  await refrescarOnline();
}

async function borrarSala(codigo) {
  const sala = estado.online.salas.find((s) => s.codigo === codigo);
  if (!sala) return;
  const texto = sala.partida_propia
    ? 'Se borrará también la partida propia de esta sala. Tu partida de siempre no se toca.'
    : 'Tu partida no se borra: solo la sala y lo que sabías de tus amigos en ella.';
  const si = await tauri.dialog.ask(texto, { title: `¿Borrar la sala «${sala.nombre}»?`, kind: 'warning', okLabel: 'Borrar', cancelLabel: 'Cancelar' });
  if (!si) return;
  await invoke('borrar_sala', { codigo });
  await refrescarOnline();
}

async function jugarSala() {
  const sala = salaActiva();
  if (!sala) return;
  try {
    await invoke('jugar_sala', { codigo: sala.codigo });
    avisar(`Abriendo ${sala.juego_titulo}…`);
  } catch (e) { avisar(String(e)); }
}

tauri.event.listen('sala-estado', (evento) => {
  const datos = evento.payload || {};
  const o = estado.online;
  if (!o.activa || datos.codigo !== o.activa.codigo) return;
  switch (datos.estado) {
    case 'rol': o.activa.anfitrion = datos.rol === 'host'; break;
    case 'amigos': o.amigos = datos.n || 0; break;
    case 'ruta': o.ruta = datos.tipo; break;
    case 'progreso':
      invoke('progreso_sala', { codigo: o.activa.codigo }).then((p) => {
        o.progreso = p;
        if (estado.vista === 'online') pintar();
      });
      return;
    case 'error': o.fase = 'error'; o.error = datos.mensaje; break;
    default: if (datos.estado) o.fase = datos.estado; break;
  }
  if (estado.vista === 'online') pintar();
});

tauri.event.listen('sala-cerrada', (evento) => {
  const o = estado.online;
  if (!o.activa || (evento.payload && evento.payload.codigo !== o.activa.codigo)) return;
  if (o.fase !== 'error') {
    o.fase = 'error';
    o.error = 'Se ha cerrado la conexión de la sala.';
  }
  if (estado.vista === 'online') pintar();
});

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
        <kbd>F1</kbd><span>Menú del emulador (en portátiles puede ser Fn + F1)</span>
        <kbd>Intro 2 s</kbd><span>Mantener pulsado también abre el menú</span>
        <kbd>Esc</kbd><span>Salir del juego y volver a Pokemulador (se guarda solo)</span>
      </div>
    </div>
    <div class="ajuste">
      <h2>Teclas en 3DS (Azahar)</h2>
      <p class="suave">Azahar tiene su barra de menús arriba. A pantalla completa no se ve: pulsa Esc para verla.</p>
      <div class="teclas">
        <kbd>Esc</kbd><span>Salir de pantalla completa y ver los menús</span>
        <kbd>F11</kbd><span>Volver a pantalla completa</span>
        <kbd>Ctrl+C</kbd><span>Guardar partida rápida</span>
        <kbd>Ctrl+V</kbd><span>Cargar la última partida rápida</span>
        <kbd>F4</kbd><span>Pausar o seguir</span>
        <kbd>F9</kbd><span>Cambiar las pantallas de sitio</span>
        <kbd>F10</kbd><span>Cambiar cómo se colocan las pantallas</span>
      </div>
      <p class="suave" style="margin-top:12px">Los controles de 3DS se cambian en el menú Emulación › Configurar.</p>
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
  await cargarSalas();
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
    case 'partida': modalPartida(i); break;
    case 'nueva-sala': modalNuevaSala(objetivo.dataset.modo); break;
    case 'entrar-sala': entrarSala(objetivo.dataset.codigo); break;
    case 'borrar-sala': borrarSala(objetivo.dataset.codigo); break;
    case 'jugar-sala': jugarSala(); break;
    case 'cambiar-nombre': modalNombre(); break;
    case 'salir-sala': salirSala(); break;
    case 'copiar-codigo':
      navigator.clipboard.writeText(estado.online.activa.codigo).then(() => avisar('Código copiado'));
      break;
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

tauri.event.listen('juego-cerrado', () => { setTimeout(cargar, 300); });

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
  const respuestas = {
    library: datos,
    set_roms_dir: datos,
    salas_guardadas: {
      clave: true, id: 'PC-AB12', nombre: 'Pablo', activa: null,
      salas: [{ codigo: 'K7P2QX', nombre: 'Ultra Luna con Dani', juego: ejemplo[7].path, juego_titulo: 'Pokemon X', sistema: '3ds', partida_propia: true, creada: 0,
        progreso: { propio: null, amigos: [{ id: 'Jugador-ZZ99', nombre: 'Dani', jugadoMs: 7380000, ultimaVez: Date.now() - 3600000 }] } }],
    },
    progreso_sala: { propio: { nombre: 'Pablo', juego: 'Pokemon X', partida: 'sala', jugadoMs: 5400000, ultimaVez: Date.now() - 86400000 }, amigos: [] },
    crear_sala: { codigo: 'K7P2QX' },
    guardar_sala: { codigo: 'K7P2QX' },
    importar_partida: 'Partida cargada. Ya puedes jugar con ella.',
  };
  return {
    core: { invoke: async (cmd) => (cmd in respuestas ? respuestas[cmd] : true) },
    dialog: { open: async () => 'C:/Juegos', save: async () => null, ask: async () => true },
    event: { listen: () => {} },
  };
}
