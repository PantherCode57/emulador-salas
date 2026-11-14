package com.swordfish.lemuroid.app.mobile.feature.online

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import com.swordfish.lemuroid.R
import com.swordfish.lemuroid.app.mobile.shared.compose.ui.PokedexColors

/**
 * Pantalla de salas online. De momento solo muestra cómo será: crear sala y unirse
 * con código llegarán cuando esté listo el servidor (fase 3).
 */
@Composable
fun OnlineScreen(modifier: Modifier = Modifier) {
    Column(
        modifier =
            modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Surface(
            color = MaterialTheme.colorScheme.tertiaryContainer,
            shape = RoundedCornerShape(50),
        ) {
            Text(
                text = stringResource(R.string.emudex_online_soon),
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onTertiaryContainer,
            )
        }

        Text(
            text = stringResource(R.string.emudex_your_room),
            style = MaterialTheme.typography.titleMedium,
        )
        RoomCodeBoxes()

        Text(
            text = stringResource(R.string.emudex_online_description),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        Box(
            modifier =
                Modifier
                    .fillMaxWidth()
                    .height(2.dp)
                    .background(PokedexColors.CardStrong),
        )

        Text(
            text = stringResource(R.string.emudex_join_with_code),
            style = MaterialTheme.typography.titleMedium,
        )
        RoomCodeBoxes()
    }
}

@Composable
private fun RoomCodeBoxes() {
    val shape = RoundedCornerShape(8.dp)
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.CenterHorizontally),
    ) {
        repeat(ROOM_CODE_LENGTH) {
            Box(
                modifier =
                    Modifier
                        .size(width = 40.dp, height = 50.dp)
                        .border(2.dp, PokedexColors.FrameDark, shape)
                        .background(Color.White, shape),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    text = "·",
                    style = MaterialTheme.typography.headlineSmall,
                    fontFamily = FontFamily.Monospace,
                    color = PokedexColors.CardStrong,
                )
            }
        }
    }
}

private const val ROOM_CODE_LENGTH = 6
