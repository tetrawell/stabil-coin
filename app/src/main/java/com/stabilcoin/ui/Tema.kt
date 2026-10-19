package com.stabilcoin.ui

import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

/** Grafik ekranlarına yakın, koyu bir görünüm. */
object Renk {
    val zemin = Color(0xFF0E1116)
    val kart = Color(0xFF161B22)
    val kenar = Color(0xFF2A313C)
    val yazi = Color(0xFFE6EDF3)
    val soluk = Color(0xFF8B949E)
    val yesil = Color(0xFF2EBD85)
    val kirmizi = Color(0xFFF6465D)
    val sari = Color(0xFFF0B90B)
    val mavi = Color(0xFF58A6FF)
}

@Composable
fun StabilTema(icerik: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Renk.yesil,
            onPrimary = Renk.zemin,
            background = Renk.zemin,
            onBackground = Renk.yazi,
            surface = Renk.kart,
            onSurface = Renk.yazi,
            surfaceVariant = Renk.kart,
            onSurfaceVariant = Renk.soluk,
            surfaceContainerHigh = Renk.kart,
            outline = Renk.kenar,
            error = Renk.kirmizi,
        ),
        content = icerik,
    )
}
