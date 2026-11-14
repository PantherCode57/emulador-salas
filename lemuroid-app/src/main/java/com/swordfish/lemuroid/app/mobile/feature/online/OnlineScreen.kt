package com.swordfish.lemuroid.app.mobile.feature.online

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.shared.compose.ui.PokedexColors

/** Pantalla de salas online: crear una sala con código o unirse con el código de un amigo. */
@Composable
fun OnlineScreen(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val estado by SalaOnline.estado.collectAsState()

    Column(
        modifier =
            modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        when {
            !SalaOnline.tieneClave -> Aviso(stringResource(R.string.emudex_online_sin_clave))
            estado.codigo == null ->
                SinSala(
                    error = estado.error,
                    onCrear = { SalaOnline.crear(context) },
                    onUnirse = { SalaOnline.unirse(context, it) },
                )
            else -> ConSala(estado, onSalir = { SalaOnline.salir(context) })
        }
    }
}

@Composable
private fun SinSala(
    error: String?,
    onCrear: () -> Unit,
    onUnirse: (String) -> Unit,
) {
    var codigo by remember { mutableStateOf("") }
    var codigoMal by remember { mutableStateOf(false) }

    Text(
        text = stringResource(R.string.emudex_online_solo_3ds),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )

    Tarjeta {
        Text(stringResource(R.string.emudex_crear_sala_titulo), style = MaterialTheme.typography.titleMedium)
        Text(stringResource(R.string.emudex_crear_sala_texto), style = MaterialTheme.typography.bodyMedium)
        Button(onClick = onCrear) { Text(stringResource(R.string.emudex_create_room)) }
    }

    Tarjeta {
        Text(stringResource(R.string.emudex_join_with_code), style = MaterialTheme.typography.titleMedium)
        Row(
            horizontalArrangement = Arrangement.spacedBy(10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            OutlinedTextField(
                modifier = Modifier.weight(1f),
                value = codigo,
                onValueChange = { nuevo ->
                    codigo = nuevo.uppercase().filter { it.isLetterOrDigit() }.take(6)
                    codigoMal = false
                },
                singleLine = true,
                placeholder = { Text("K7P2QX", fontFamily = FontFamily.Monospace) },
                textStyle = MaterialTheme.typography.titleMedium.copy(fontFamily = FontFamily.Monospace),
                keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters),
                isError = codigoMal,
            )
            Button(onClick = {
                if (codigo.length == 6) {
                    onUnirse(codigo)
                } else {
                    codigoMal = true
                }
            }) { Text(stringResource(R.string.emudex_entrar)) }
        }
        if (codigoMal) {
            Text(
                text = stringResource(R.string.emudex_codigo_invalido),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
            )
        }
    }

    if (error != null) Aviso(error)
}

@Composable
private fun ConSala(
    estado: SalaOnline.Estado,
    onSalir: () -> Unit,
) {
    val portapapeles = LocalClipboardManager.current
    val codigo = estado.codigo.orEmpty()

    Text(
        text = stringResource(if (estado.anfitrion) R.string.emudex_your_room else R.string.emudex_sala_amigo),
        style = MaterialTheme.typography.titleMedium,
    )
    CasillasCodigo(codigo)

    val textoFase =
        when (estado.fase) {
            "conectando" -> stringResource(R.string.emudex_fase_conectando)
            "listo" -> stringResource(R.string.emudex_fase_listo)
            "buscando" -> stringResource(R.string.emudex_fase_buscando)
            "conectado" -> stringResource(R.string.emudex_fase_conectado)
            "error" -> estado.error ?: stringResource(R.string.emudex_fase_error)
            else -> stringResource(R.string.emudex_fase_preparando)
        }
    Text(
        text = textoFase,
        style = MaterialTheme.typography.titleSmall,
        fontWeight = FontWeight.SemiBold,
        color = if (estado.fase == "error") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
    )

    val detalles = mutableListOf<String>()
    if (estado.anfitrion && estado.amigos > 0) {
        detalles += if (estado.amigos == 1) {
            stringResource(R.string.emudex_amigos_uno)
        } else {
            stringResource(R.string.emudex_amigos_varios, estado.amigos)
        }
    }
    when (estado.ruta) {
        "directa" -> detalles += stringResource(R.string.emudex_ruta_directa)
        "repetidor" -> detalles += stringResource(R.string.emudex_ruta_repetidor)
    }
    if (detalles.isNotEmpty()) {
        Text(
            text = detalles.joinToString(" · "),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        if (estado.anfitrion) {
            OutlinedButton(onClick = { portapapeles.setText(AnnotatedString(codigo)) }) {
                Text(stringResource(R.string.emudex_copiar_codigo))
            }
        }
        OutlinedButton(onClick = onSalir) { Text(stringResource(R.string.emudex_salir_sala)) }
    }

    if (estado.fase == "listo" || estado.fase == "conectado") {
        Tarjeta {
            Text(stringResource(R.string.emudex_como_jugar), style = MaterialTheme.typography.titleMedium)
            Paso(1, stringResource(R.string.emudex_paso_abrir_juego))
            Paso(
                2,
                stringResource(
                    if (estado.anfitrion) R.string.emudex_paso_orden_anfitrion else R.string.emudex_paso_orden_invitado,
                ),
            )
            Paso(3, stringResource(R.string.emudex_paso_comunicacion))
            Text(
                text = stringResource(R.string.emudex_paso_reabrir),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
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
                        .size(width = 42.dp, height = 52.dp)
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
private fun Paso(
    numero: Int,
    texto: String,
) {
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("$numero.", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold)
        Text(texto, style = MaterialTheme.typography.bodyMedium)
    }
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
            verticalArrangement = Arrangement.spacedBy(10.dp),
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
