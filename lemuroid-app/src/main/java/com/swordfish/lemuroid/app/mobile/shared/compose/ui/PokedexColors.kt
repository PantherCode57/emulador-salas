package com.swordfish.lemuroid.app.mobile.shared.compose.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

/** Paleta del estilo Pokédex: carcasa roja, pantalla crema y lente azul. */
object PokedexColors {
    val Frame = Color(0xFFA32D2D)
    val FrameDark = Color(0xFF791F1F)
    val Deep = Color(0xFF501313)
    val Accent = Color(0xFFE24B4A)
    val Card = Color(0xFFF7C1C1)
    val CardStrong = Color(0xFFF09595)
    val Screen = Color(0xFFFCEBEB)
    val Lens = Color(0xFF378ADD)
    val LedRed = Color(0xFFF09595)
    val LedYellow = Color(0xFFFAC775)
    val LedGreen = Color(0xFF97C459)
    val OnlineBlue = Color(0xFF185FA5)
    val OnlineBlueLight = Color(0xFFE6F1FB)
    val OnlineBlueBorder = Color(0xFFB5D4F4)
    val OnlineBlueDark = Color(0xFF0C447C)
}

/** Lente azul y las tres luces de la Pokédex, para la barra superior. */
@Composable
fun PokedexLens(modifier: Modifier = Modifier) {
    Row(
        modifier = modifier.padding(start = 12.dp, end = 4.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(
            modifier =
                Modifier
                    .size(30.dp)
                    .border(3.dp, PokedexColors.Screen, CircleShape)
                    .padding(3.dp)
                    .background(PokedexColors.Lens, CircleShape),
        )
        PokedexLed(PokedexColors.LedRed)
        PokedexLed(PokedexColors.LedYellow)
        PokedexLed(PokedexColors.LedGreen)
    }
}

@Composable
private fun PokedexLed(color: Color) {
    Box(
        modifier =
            Modifier
                .size(9.dp)
                .background(color, CircleShape),
    )
}
