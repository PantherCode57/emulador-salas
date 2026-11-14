package com.swordfish.lemuroid.app.mobile.shared.compose.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Esquema único estilo Pokédex. No usamos los colores dinámicos de Android 12+
// (los que salen del fondo de pantalla) para que la app se vea igual en todos los móviles.
private val PokedexColorScheme =
    lightColorScheme(
        primary = PokedexColors.Accent,
        onPrimary = PokedexColors.Screen,
        primaryContainer = PokedexColors.Card,
        onPrimaryContainer = PokedexColors.Deep,
        inversePrimary = PokedexColors.CardStrong,
        secondary = PokedexColors.OnlineBlue,
        onSecondary = PokedexColors.OnlineBlueLight,
        secondaryContainer = PokedexColors.OnlineBlueBorder,
        onSecondaryContainer = Color(0xFF042C53),
        tertiary = Color(0xFFBA7517),
        onTertiary = Color(0xFFFAEEDA),
        tertiaryContainer = PokedexColors.LedYellow,
        onTertiaryContainer = Color(0xFF412402),
        background = PokedexColors.Screen,
        onBackground = PokedexColors.Deep,
        surface = PokedexColors.Screen,
        onSurface = PokedexColors.Deep,
        surfaceVariant = PokedexColors.Card,
        onSurfaceVariant = PokedexColors.FrameDark,
        surfaceTint = PokedexColors.Accent,
        inverseSurface = PokedexColors.Deep,
        inverseOnSurface = PokedexColors.Screen,
        error = Color(0xFFBA1A1A),
        onError = Color(0xFFFFFFFF),
        errorContainer = Color(0xFFFFDAD6),
        onErrorContainer = Color(0xFF410002),
        outline = PokedexColors.Frame,
        outlineVariant = PokedexColors.CardStrong,
        scrim = Color(0xFF000000),
        surfaceBright = PokedexColors.Screen,
        surfaceDim = PokedexColors.Card,
        surfaceContainerLowest = Color(0xFFFFFFFF),
        surfaceContainerLow = Color(0xFFFAE0E0),
        surfaceContainer = Color(0xFFF9D6D6),
        surfaceContainerHigh = Color(0xFFFFF6F6),
        surfaceContainerHighest = PokedexColors.Card,
    )

@Suppress("UNUSED_PARAMETER")
@Composable
fun AppTheme(
    darkTheme: Boolean = true,
    content: @Composable () -> Unit,
) {
    MaterialTheme(colorScheme = PokedexColorScheme) {
        content()
    }
}
