package com.swordfish.lemuroid.app.mobile.feature.online

import android.text.format.DateUtils
import android.widget.Toast
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.shared.compose.ui.PokedexColors
import com.swordfish.lemuroid.lib.library.GameSystem
import com.swordfish.lemuroid.lib.library.db.RetrogradeDatabase
import com.swordfish.lemuroid.lib.library.db.entity.Game
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Salas online: cada sala se guarda con su juego y su partida. Se entra cuando se quiera y, cuando
 * coincidís, cada uno ve cómo va el otro (tiempo jugado y una captura de dónde va).
 */
@Composable
fun OnlineScreen(
    modifier: Modifier = Modifier,
    retrogradeDb: RetrogradeDatabase,
    onJugar: (Game, SalaOnline.Sala) -> Unit,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    LaunchedEffect(Unit) { withContext(Dispatchers.IO) { SalaOnline.cargar(context) } }
    val estado by SalaOnline.estado.collectAsState()
    val salas by SalaOnline.salas.collectAsState()
    var dialogo by remember { mutableStateOf<ModoDialogo?>(null) }
    var borrar by remember { mutableStateOf<SalaOnline.Sala?>(null) }
    // Para volver a leer el progreso de vez en cuando (por ejemplo, al volver de jugar).
    val pulso by produceState(0) {
        while (true) {
            delay(5_000)
            value++
        }
    }

    val jugar = { sala: SalaOnline.Sala ->
        scope.launch {
            val juego =
                withContext(Dispatchers.IO) {
                    retrogradeDb.gameDao().selectById(sala.juegoId)?.takeIf { it.fileUri == sala.juegoUri }
                        ?: retrogradeDb.gameDao().selectByFileUri(sala.juegoUri)
                        ?: retrogradeDb.gameDao().selectById(sala.juegoId)
                }
            if (juego == null) {
                Toast.makeText(context, R.string.emudex_juego_no_encontrado, Toast.LENGTH_LONG).show()
            } else {
                onJugar(juego, sala)
            }
        }
    }

    Column(
        modifier =
            modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        if (!SalaOnline.tieneClave) {
            Aviso(stringResource(R.string.emudex_online_sin_clave))
            return@Column
        }

        val activa = salas.firstOrNull { it.codigo == estado.codigo }
        if (activa != null) {
            SalaActiva(
                sala = activa,
                estado = estado,
                pulso = pulso,
                onJugar = { jugar(activa) },
                onSalir = { SalaOnline.salir(context) },
            )
        } else {
            Text(
                text = stringResource(R.string.emudex_online_intro),
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                text = stringResource(R.string.emudex_online_solo_3ds),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            if (estado.fase == "error" && estado.error != null) Aviso(estado.error!!)
        }

        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Button(modifier = Modifier.weight(1f), onClick = { dialogo = ModoDialogo.CREAR }) {
                Text(stringResource(R.string.emudex_create_room))
            }
            OutlinedButton(modifier = Modifier.weight(1f), onClick = { dialogo = ModoDialogo.UNIRSE }) {
                Text(stringResource(R.string.emudex_join_with_code))
            }
        }

        TarjetaNombre()

        val otras = salas.filter { it.codigo != estado.codigo }
        if (otras.isNotEmpty() || activa == null) {
            Text(stringResource(R.string.emudex_mis_salas), style = MaterialTheme.typography.titleMedium)
        }
        if (salas.isEmpty()) {
            Text(
                text = stringResource(R.string.emudex_sin_salas),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        otras.forEach { sala ->
            TarjetaSala(
                sala = sala,
                pulso = pulso,
                onEntrar = { SalaOnline.entrar(context, sala) },
                onBorrar = { borrar = sala },
            )
        }
    }

    dialogo?.let { modo ->
        DialogoSala(
            modo = modo,
            retrogradeDb = retrogradeDb,
            onDismiss = { dialogo = null },
            onConfirm = { codigo, nombre, juego, partidaPropia ->
                dialogo = null
                val sala =
                    if (modo == ModoDialogo.CREAR) {
                        SalaOnline.crearSala(context, nombre, juego, partidaPropia)
                    } else {
                        SalaOnline.guardarSala(context, codigo, nombre, juego, partidaPropia)
                    }
                SalaOnline.entrar(context, sala)
            },
        )
    }

    borrar?.let { sala ->
        AlertDialog(
            onDismissRequest = { borrar = null },
            title = { Text(stringResource(R.string.emudex_borrar_sala_titulo, sala.nombre)) },
            text = {
                Text(
                    stringResource(
                        if (sala.partidaPropia) R.string.emudex_borrar_sala_propia else R.string.emudex_borrar_sala_normal,
                    ),
                )
            },
            confirmButton = {
                TextButton(onClick = {
                    borrar = null
                    SalaOnline.borrarSala(context, sala)
                }) { Text(stringResource(R.string.emudex_borrar)) }
            },
            dismissButton = {
                TextButton(onClick = { borrar = null }) { Text(stringResource(android.R.string.cancel)) }
            },
        )
    }
}

private enum class ModoDialogo { CREAR, UNIRSE }

// ---------------------------------------------------------------------------------------------
// Sala en la que estamos

@Composable
private fun SalaActiva(
    sala: SalaOnline.Sala,
    estado: SalaOnline.Estado,
    pulso: Int,
    onJugar: () -> Unit,
    onSalir: () -> Unit,
) {
    val context = LocalContext.current
    val portapapeles = LocalClipboardManager.current

    Tarjeta {
        Text(stringResource(R.string.emudex_dentro_sala), style = MaterialTheme.typography.labelLarge)
        Text(sala.nombre, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
        Text(
            "${sala.juegoTitulo} · ${textoPartida(sala.partidaPropia)}",
            style = MaterialTheme.typography.bodyMedium,
        )
        CasillasCodigo(sala.codigo)

        val textoFase =
            when {
                estado.fase == "error" -> estado.error ?: stringResource(R.string.emudex_fase_error)
                estado.fase == "conectando" -> stringResource(R.string.emudex_fase_conectando)
                !estado.rolDecidido -> stringResource(R.string.emudex_fase_buscando_sala)
                estado.amigos > 0 && estado.anfitrion && estado.amigos > 1 ->
                    stringResource(R.string.emudex_amigos_varios, estado.amigos)
                estado.amigos > 0 -> stringResource(R.string.emudex_fase_con_amigo)
                estado.anfitrion -> stringResource(R.string.emudex_fase_anfitrion)
                else -> stringResource(R.string.emudex_fase_esperando_vuelta)
            }
        Text(
            text = textoFase,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.SemiBold,
            color = if (estado.fase == "error") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
        )
        when (estado.ruta) {
            "directa" -> TextoSuave(stringResource(R.string.emudex_ruta_directa))
            "repetidor" -> TextoSuave(stringResource(R.string.emudex_ruta_repetidor))
        }

        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Button(enabled = estado.rolDecidido && estado.fase != "error", onClick = onJugar) {
                Text(stringResource(R.string.emudex_jugar))
            }
            OutlinedButton(onClick = { portapapeles.setText(AnnotatedString(sala.codigo)) }) {
                Text(stringResource(R.string.emudex_copiar_codigo))
            }
        }
        OutlinedButton(onClick = onSalir) { Text(stringResource(R.string.emudex_salir_sala)) }
        if (sala.esDe3DS) {
            TextoSuave(stringResource(R.string.emudex_paso_comunicacion))
        } else {
            TextoSuave(stringResource(R.string.emudex_sala_sin_online))
        }
    }

    val propio by produceState<ProgresoSala.Progreso?>(null, sala.codigo, pulso) {
        value = withContext(Dispatchers.IO) { ProgresoSala.propio(context, sala.codigo) }
    }
    val amigos by produceState(emptyList<ProgresoSala.Progreso>(), sala.codigo, estado.novedades, pulso) {
        value = withContext(Dispatchers.IO) { ProgresoSala.amigos(context, sala.codigo) }
    }

    if (amigos.isEmpty()) {
        Aviso(stringResource(R.string.emudex_progreso_nadie))
    }
    amigos.forEach { TarjetaProgreso(stringResource(R.string.emudex_progreso_titulo, it.nombre), it) }
    propio?.let { TarjetaProgreso(stringResource(R.string.emudex_progreso_tuyo), it) }
}

@Composable
private fun TarjetaProgreso(
    titulo: String,
    progreso: ProgresoSala.Progreso,
) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = PokedexColors.Screen, contentColor = PokedexColors.Deep),
        shape = RoundedCornerShape(14.dp),
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            val captura = progreso.captura
            if (captura != null) {
                val imagen = remember(captura) { captura.asImageBitmap() }
                Image(
                    bitmap = imagen,
                    contentDescription = null,
                    contentScale = ContentScale.Fit,
                    modifier =
                        Modifier
                            .width(132.dp)
                            .heightIn(max = 180.dp)
                            .clip(RoundedCornerShape(8.dp))
                            .border(2.dp, PokedexColors.FrameDark, RoundedCornerShape(8.dp)),
                )
            }
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(titulo, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                if (progreso.juego.isNotBlank()) {
                    Text(progreso.juego, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
                Text(textoPartida(progreso.partidaPropia), style = MaterialTheme.typography.bodySmall)
                Text(
                    stringResource(R.string.emudex_jugado, textoTiempo(progreso.jugadoMs)),
                    style = MaterialTheme.typography.bodyMedium,
                    fontWeight = FontWeight.SemiBold,
                )
                Text(
                    if (progreso.ultimaVez > 0) {
                        stringResource(
                            R.string.emudex_ultima_vez,
                            DateUtils.getRelativeTimeSpanString(progreso.ultimaVez).toString(),
                        )
                    } else {
                        stringResource(R.string.emudex_nunca)
                    },
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Salas guardadas

@Composable
private fun TarjetaSala(
    sala: SalaOnline.Sala,
    pulso: Int,
    onEntrar: () -> Unit,
    onBorrar: () -> Unit,
) {
    val context = LocalContext.current
    val amigos by produceState(emptyList<ProgresoSala.Progreso>(), sala.codigo, pulso) {
        value = withContext(Dispatchers.IO) { ProgresoSala.amigos(context, sala.codigo) }
    }
    Tarjeta {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(modifier = Modifier.weight(1f)) {
                Text(sala.nombre, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                Text(
                    "${sala.juegoTitulo} · ${textoPartida(sala.partidaPropia)}",
                    style = MaterialTheme.typography.bodySmall,
                )
                Text(
                    stringResource(R.string.emudex_codigo_sala, sala.codigo),
                    style = MaterialTheme.typography.bodySmall,
                    fontFamily = FontFamily.Monospace,
                )
            }
            Button(onClick = onEntrar) { Text(stringResource(R.string.emudex_entrar)) }
        }
        amigos.forEach { amigo ->
            val cuando =
                if (amigo.ultimaVez > 0) DateUtils.getRelativeTimeSpanString(amigo.ultimaVez).toString() else ""
            TextoSuave(
                stringResource(R.string.emudex_resumen_amigo, amigo.nombre, textoTiempo(amigo.jugadoMs), cuando),
            )
        }
        HorizontalDivider(color = PokedexColors.CardStrong)
        TextButton(onClick = onBorrar) { Text(stringResource(R.string.emudex_borrar_sala)) }
    }
}

@Composable
private fun TarjetaNombre() {
    val context = LocalContext.current
    var editando by remember { mutableStateOf(false) }
    var nombre by remember { mutableStateOf(SalaOnline.nombreVisible(context)) }
    Tarjeta {
        Text(stringResource(R.string.emudex_tu_nombre), style = MaterialTheme.typography.labelLarge)
        if (editando) {
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    modifier = Modifier.weight(1f),
                    value = nombre,
                    onValueChange = { nombre = it.take(24) },
                    singleLine = true,
                )
                Button(onClick = {
                    SalaOnline.cambiarNombre(context, nombre)
                    nombre = SalaOnline.nombreVisible(context)
                    editando = false
                }) { Text(stringResource(R.string.emudex_guardar)) }
            }
        } else {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(nombre, modifier = Modifier.weight(1f), style = MaterialTheme.typography.titleMedium)
                TextButton(onClick = { editando = true }) { Text(stringResource(R.string.emudex_cambiar)) }
            }
        }
    }
}

// ---------------------------------------------------------------------------------------------
// Crear o unirse

@Composable
private fun DialogoSala(
    modo: ModoDialogo,
    retrogradeDb: RetrogradeDatabase,
    onDismiss: () -> Unit,
    onConfirm: (codigo: String, nombre: String, juego: Game, partidaPropia: Boolean) -> Unit,
) {
    var codigo by remember { mutableStateOf("") }
    var nombre by remember { mutableStateOf("") }
    var juego by remember { mutableStateOf<Game?>(null) }
    var partidaPropia by remember { mutableStateOf(false) }
    var eligiendoJuego by remember { mutableStateOf(false) }
    var aviso by remember { mutableStateOf<Int?>(null) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = {
            Text(stringResource(if (modo == ModoDialogo.CREAR) R.string.emudex_create_room else R.string.emudex_join_with_code))
        },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                if (modo == ModoDialogo.UNIRSE) {
                    OutlinedTextField(
                        modifier = Modifier.fillMaxWidth(),
                        value = codigo,
                        onValueChange = { nuevo -> codigo = nuevo.uppercase().filter { it.isLetterOrDigit() }.take(6) },
                        singleLine = true,
                        label = { Text(stringResource(R.string.emudex_codigo_amigo)) },
                        placeholder = { Text("K7P2QX", fontFamily = FontFamily.Monospace) },
                        textStyle = MaterialTheme.typography.titleMedium.copy(fontFamily = FontFamily.Monospace),
                        keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters),
                    )
                }
                Text(stringResource(R.string.emudex_elige_juego), style = MaterialTheme.typography.labelLarge)
                OutlinedButton(modifier = Modifier.fillMaxWidth(), onClick = { eligiendoJuego = true }) {
                    Text(
                        juego?.let { it.customName ?: it.title } ?: stringResource(R.string.emudex_elige_juego_boton),
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                Text(stringResource(R.string.emudex_que_partida), style = MaterialTheme.typography.labelLarge)
                OpcionPartida(
                    elegida = !partidaPropia,
                    titulo = stringResource(R.string.emudex_partida_de_siempre),
                    texto = stringResource(R.string.emudex_partida_siempre_desc),
                    onClick = { partidaPropia = false },
                )
                OpcionPartida(
                    elegida = partidaPropia,
                    titulo = stringResource(R.string.emudex_partida_nueva),
                    texto = stringResource(R.string.emudex_partida_nueva_desc),
                    onClick = { partidaPropia = true },
                )
                OutlinedTextField(
                    modifier = Modifier.fillMaxWidth(),
                    value = nombre,
                    onValueChange = { nombre = it.take(30) },
                    singleLine = true,
                    label = { Text(stringResource(R.string.emudex_nombre_sala)) },
                )
                aviso?.let {
                    Text(stringResource(it), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                val elegido = juego
                aviso =
                    when {
                        modo == ModoDialogo.UNIRSE && codigo.length != 6 -> R.string.emudex_codigo_invalido
                        elegido == null -> R.string.emudex_falta_juego
                        else -> null
                    }
                if (aviso == null && elegido != null) onConfirm(codigo, nombre, elegido, partidaPropia)
            }) {
                Text(stringResource(if (modo == ModoDialogo.CREAR) R.string.emudex_crear else R.string.emudex_entrar))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(android.R.string.cancel)) }
        },
    )

    if (eligiendoJuego) {
        SelectorJuego(
            retrogradeDb = retrogradeDb,
            onDismiss = { eligiendoJuego = false },
            onElegir = {
                juego = it
                eligiendoJuego = false
            },
        )
    }
}

@Composable
private fun OpcionPartida(
    elegida: Boolean,
    titulo: String,
    texto: String,
    onClick: () -> Unit,
) {
    Row(
        modifier =
            Modifier
                .fillMaxWidth()
                .clip(RoundedCornerShape(10.dp))
                .clickable(onClick = onClick)
                .padding(vertical = 4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        RadioButton(selected = elegida, onClick = onClick)
        Column {
            Text(titulo, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.SemiBold)
            Text(texto, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Composable
private fun SelectorJuego(
    retrogradeDb: RetrogradeDatabase,
    onDismiss: () -> Unit,
    onElegir: (Game) -> Unit,
) {
    val juegos by produceState<List<Game>?>(null) {
        // Primero los de 3DS, que son los que tienen intercambios y combates online.
        value =
            withContext(Dispatchers.IO) {
                retrogradeDb.gameDao().selectAllSorted().sortedBy { if (it.systemId == "3ds") 0 else 1 }
            }
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.emudex_elige_juego_boton)) },
        text = {
            val lista = juegos
            when {
                lista == null -> Text("…")
                lista.isEmpty() -> Text(stringResource(R.string.emudex_sin_juegos))
                else ->
                    LazyColumn(modifier = Modifier.heightIn(max = 420.dp)) {
                        items(lista, key = { it.id }) { juego ->
                            val recurso =
                                remember(juego.systemId) {
                                    runCatching { GameSystem.findById(juego.systemId).shortTitleResId }.getOrNull()
                                }
                            val sistema = recurso?.let { stringResource(it) } ?: juego.systemId
                            Column(
                                modifier =
                                    Modifier
                                        .fillMaxWidth()
                                        .clickable { onElegir(juego) }
                                        .padding(vertical = 10.dp),
                            ) {
                                Text(juego.customName ?: juego.title, style = MaterialTheme.typography.bodyLarge)
                                Text(sistema, style = MaterialTheme.typography.bodySmall)
                            }
                        }
                    }
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(android.R.string.cancel)) }
        },
    )
}

// ---------------------------------------------------------------------------------------------
// Piezas

@Composable
private fun textoPartida(partidaPropia: Boolean): String =
    stringResource(if (partidaPropia) R.string.emudex_partida_propia_corta else R.string.emudex_partida_siempre_corta)

@Composable
private fun textoTiempo(ms: Long): String {
    val minutos = ms / 60_000
    return when {
        minutos < 1 -> stringResource(R.string.emudex_tiempo_nada)
        minutos < 60 -> stringResource(R.string.emudex_tiempo_minutos, minutos.toInt())
        else -> stringResource(R.string.emudex_tiempo_horas, (minutos / 60).toInt(), (minutos % 60).toInt())
    }
}

@Composable
private fun CasillasCodigo(codigo: String) {
    val forma = RoundedCornerShape(8.dp)
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.CenterHorizontally),
    ) {
        codigo.padEnd(6, '·').forEach { letra ->
            Box(
                modifier =
                    Modifier
                        .size(width = 40.dp, height = 50.dp)
                        .border(2.dp, PokedexColors.FrameDark, forma)
                        .background(Color.White, forma),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    text = letra.toString(),
                    style = MaterialTheme.typography.headlineSmall,
                    fontFamily = FontFamily.Monospace,
                    color = PokedexColors.Deep,
                )
            }
        }
    }
}

@Composable
private fun TextoSuave(texto: String) {
    Text(texto, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

@Composable
private fun Tarjeta(contenido: @Composable () -> Unit) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors =
            CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.primaryContainer,
                contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
            ),
        shape = RoundedCornerShape(14.dp),
    ) {
        Column(
            modifier = Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) { contenido() }
    }
}

@Composable
private fun Aviso(texto: String) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = RoundedCornerShape(12.dp),
    ) {
        Text(texto, modifier = Modifier.padding(14.dp), style = MaterialTheme.typography.bodyMedium)
    }
}
