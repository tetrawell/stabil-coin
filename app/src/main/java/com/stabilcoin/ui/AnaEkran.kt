package com.stabilcoin.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.lifecycle.viewmodel.compose.viewModel
import com.stabilcoin.core.TfSonuc
import com.stabilcoin.core.Yon
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.math.max

private val TR: Locale = Locale.forLanguageTag("tr-TR")

enum class Siralama(val ad: String) { DALGA("Dalga"), SURE("Süre"), PUAN("Puan") }

/** Listede gösterilecek bir satır: coin ve hangi zaman dilimiyle listelendiği. */
data class Satir(val coin: CoinSonuc, val tf: TfSonuc)

fun listele(coinler: List<CoinSonuc>, seciliTf: String?, siralama: Siralama): List<Satir> {
    val satirlar = coinler.mapNotNull { c ->
        val tf = if (seciliTf != null) {
            c.tfler.firstOrNull { it.zamanDilimi == seciliTf && it.uygun }
        } else {
            c.tfler.filter { it.uygun }.maxByOrNull { it.puan }
        }
        tf?.let { Satir(c, it) }
    }
    return when (siralama) {
        Siralama.DALGA -> satirlar.sortedByDescending { it.tf.dalgaYuzde }
        Siralama.SURE -> satirlar.sortedByDescending { it.tf.sureMs }
        Siralama.PUAN -> satirlar.sortedByDescending { it.tf.puan }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AnaEkran(model: AnaModel = viewModel()) {
    val durum by model.durum.collectAsState()
    val ayarlar by model.ayarlar.collectAsState()
    var seciliTf by rememberSaveable { mutableStateOf<String?>(null) }
    var siralama by rememberSaveable { mutableStateOf(Siralama.DALGA) }
    var ayarAcik by remember { mutableStateOf(false) }

    val satirlar = remember(durum.coinler, seciliTf, siralama) { listele(durum.coinler, seciliTf, siralama) }
    val tfSayilari = remember(durum.coinler) {
        ZAMAN_DILIMLERI.associateWith { tf -> durum.coinler.count { c -> c.tfler.any { it.zamanDilimi == tf && it.uygun } } }
    }
    val toplamUygun = remember(durum.coinler) { durum.coinler.count { c -> c.tfler.any { it.uygun } } }

    Scaffold(
        containerColor = Renk.zemin,
        topBar = {
            TopAppBar(
                colors = TopAppBarDefaults.topAppBarColors(containerColor = Renk.zemin, titleContentColor = Renk.yazi),
                title = {
                    Column {
                        Text("Stabil Coinler", fontWeight = FontWeight.SemiBold, fontSize = 20.sp)
                        Text(altBaslik(durum, ayarlar), fontSize = 12.sp, color = Renk.soluk)
                    }
                },
                actions = {
                    IconButton(onClick = { model.tara() }, enabled = !durum.yukleniyor) {
                        Icon(Icons.Filled.Refresh, contentDescription = "Yenile", tint = if (durum.yukleniyor) Renk.kenar else Renk.yazi)
                    }
                    IconButton(onClick = { ayarAcik = true }) {
                        Icon(Icons.Filled.Settings, contentDescription = "Ayarlar", tint = Renk.yazi)
                    }
                },
            )
        },
    ) { ic ->
        Column(Modifier.padding(ic).fillMaxSize()) {
            if (durum.yukleniyor) {
                YuklemeCubugu(durum)
            }
            if (durum.hata != null && durum.coinler.isNotEmpty()) {
                Text(
                    "${durum.hata} Liste bir önceki taramadan.",
                    color = Renk.kirmizi, fontSize = 12.sp,
                    modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
                )
            }

            Row(
                Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(horizontal = 12.dp, vertical = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Cip("Tümü ($toplamUygun)", secili = seciliTf == null) { seciliTf = null }
                ZAMAN_DILIMLERI.forEach { tf ->
                    Cip("${tfAdi(tf)} (${tfSayilari[tf] ?: 0})", secili = seciliTf == tf) { seciliTf = tf }
                }
            }
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 12.dp).padding(bottom = 4.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text("Sırala:", color = Renk.soluk, fontSize = 13.sp)
                Siralama.entries.forEach { s -> Cip(s.ad, secili = siralama == s, kucuk = true) { siralama = s } }
            }

            when {
                durum.hata != null && durum.coinler.isEmpty() -> MesajKutusu(
                    baslik = "Bağlantı sorunu",
                    metin = durum.hata!!,
                    dugme = "Tekrar dene",
                    onDugme = { model.tara() },
                )
                durum.sonGuncelleme == null && durum.coinler.isEmpty() -> MesajKutusu(
                    baslik = "Coinler taranıyor",
                    metin = "MEXC'deki coinlerin grafikleri indiriliyor. Bu ilk açılışta 20-40 saniye sürebilir.",
                )
                satirlar.isEmpty() && !durum.yukleniyor -> MesajKutusu(
                    baslik = "Şu an uyan coin yok",
                    metin = "Bu zaman diliminde kurallara uyan coin bulunamadı. Başka bir zaman dilimine bakabilir ya da ayarlardan en küçük dalgayı düşürebilirsin.",
                )
                else -> LazyColumn(
                    contentPadding = PaddingValues(start = 12.dp, end = 12.dp, top = 6.dp, bottom = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    items(satirlar, key = { it.coin.sembol }) { s -> CoinKarti(s) }
                    if (durum.eksikGrafik > 0) {
                        item {
                            Text(
                                "${durum.eksikGrafik} grafik indirilemedi; o zaman dilimleri hesaba katılmadı.",
                                color = Renk.soluk, fontSize = 12.sp, modifier = Modifier.padding(8.dp),
                            )
                        }
                    }
                }
            }
        }
    }

    if (ayarAcik) {
        AyarPenceresi(
            mevcut = ayarlar,
            onKapat = { ayarAcik = false },
            onUygula = { model.ayarlariUygula(it); ayarAcik = false },
        )
    }
}

private fun altBaslik(d: Durum, a: Ayarlar): String {
    val hacim = hacimMetni(a.enAzHacimUsdt)
    val zaman = d.sonGuncelleme?.let { SimpleDateFormat("HH:mm", TR).format(Date(it)) }
    return buildString {
        append("MEXC · hacim ≥ $hacim · dalga ≥ %${sayi(a.enKucukDalgaYuzde)}")
        if (zaman != null) append(" · $zaman")
    }
}

@Composable
private fun YuklemeCubugu(d: Durum) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp)) {
        val oran = if (d.toplam > 0) d.ilerleme.toFloat() / d.toplam else 0f
        LinearProgressIndicator(
            progress = { oran },
            modifier = Modifier.fillMaxWidth().height(4.dp),
            color = Renk.yesil,
            trackColor = Renk.kenar,
        )
        Text(
            if (d.toplam == 0) "Coin listesi alınıyor…" else "${d.coinSayisi} coin · grafikler: ${d.ilerleme} / ${d.toplam}",
            color = Renk.soluk, fontSize = 12.sp, modifier = Modifier.padding(top = 4.dp),
        )
    }
}

@Composable
private fun Cip(metin: String, secili: Boolean, kucuk: Boolean = false, onClick: () -> Unit) {
    val sekil = RoundedCornerShape(50)
    Box(
        Modifier
            .border(1.dp, if (secili) Renk.yesil else Renk.kenar, sekil)
            .background(if (secili) Renk.yesil.copy(alpha = 0.15f) else Color.Transparent, sekil)
            .clickable(onClick = onClick)
            .padding(horizontal = if (kucuk) 10.dp else 12.dp, vertical = if (kucuk) 4.dp else 6.dp),
    ) {
        Text(metin, color = if (secili) Renk.yesil else Renk.yazi, fontSize = if (kucuk) 12.sp else 13.sp)
    }
}

@Composable
private fun MesajKutusu(baslik: String, metin: String, dugme: String? = null, onDugme: () -> Unit = {}) {
    Column(
        Modifier.fillMaxWidth().padding(32.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(baslik, color = Renk.yazi, fontSize = 17.sp, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(8.dp))
        Text(metin, color = Renk.soluk, fontSize = 14.sp, textAlign = TextAlign.Center)
        if (dugme != null) {
            Spacer(Modifier.height(12.dp))
            TextButton(onClick = onDugme) { Text(dugme, color = Renk.yesil) }
        }
    }
}

@Composable
private fun CoinKarti(s: Satir) {
    var gosterilen by remember(s.coin.sembol, s.tf.zamanDilimi) { mutableStateOf(s.tf.zamanDilimi) }
    val t = s.coin.tfler.firstOrNull { it.zamanDilimi == gosterilen } ?: s.tf
    val sekil = RoundedCornerShape(12.dp)

    Column(
        Modifier
            .fillMaxWidth()
            .background(Renk.kart, sekil)
            .border(1.dp, Renk.kenar, sekil)
            .padding(12.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(
                    s.coin.sembol.removeSuffix("USDT") + "/USDT",
                    color = Renk.yazi, fontSize = 17.sp, fontWeight = FontWeight.SemiBold,
                )
                Text("Hacim ${hacimMetni(s.coin.hacimUsdt)}", color = Renk.soluk, fontSize = 12.sp)
            }
            YonEtiketi(t)
            Spacer(Modifier.width(10.dp))
            PuanRozeti(t)
        }

        Spacer(Modifier.height(8.dp))
        MiniGrafik(t)
        Spacer(Modifier.height(8.dp))

        if (t.uygun) {
            Text(
                "${tfAdi(t.zamanDilimi)} grafikte ${sureMetni(t.sureMs, t.enAz)} stabil",
                color = Renk.yazi, fontSize = 14.sp, fontWeight = FontWeight.Medium,
            )
            Text(
                "Dalga %${sayi(t.dalgaYuzde)} · ${t.dalgaSayisi} dalga",
                color = Renk.soluk, fontSize = 13.sp,
            )
        } else {
            Text(
                "${tfAdi(t.zamanDilimi)} grafikte uygun değil: ${t.neden.ifEmpty { "kurallara uymuyor" }}",
                color = Renk.soluk, fontSize = 13.sp,
            )
        }

        Spacer(Modifier.height(8.dp))
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            ZAMAN_DILIMLERI.forEach { tf ->
                val sonuc = s.coin.tfler.firstOrNull { it.zamanDilimi == tf }
                TfKutusu(tf, uygun = sonuc?.uygun == true, secili = tf == gosterilen) { gosterilen = tf }
            }
        }
    }
}

@Composable
private fun TfKutusu(tf: String, uygun: Boolean, secili: Boolean, onClick: () -> Unit) {
    val sekil = RoundedCornerShape(6.dp)
    val zemin = if (uygun) Renk.yesil.copy(alpha = 0.18f) else Color.Transparent
    val kenar = when {
        secili -> Renk.yazi
        uygun -> Renk.yesil.copy(alpha = 0.6f)
        else -> Renk.kenar
    }
    Box(
        Modifier
            .border(if (secili) 1.5.dp else 1.dp, kenar, sekil)
            .background(zemin, sekil)
            .clickable(onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 4.dp),
    ) {
        Text(tfAdi(tf), color = if (uygun) Renk.yesil else Renk.soluk, fontSize = 12.sp, fontWeight = FontWeight.Medium)
    }
}

@Composable
private fun YonEtiketi(t: TfSonuc) {
    val (metin, renk) = when (t.yon) {
        Yon.YUKSELEN -> "↗ Yükselen" to Renk.yesil
        Yon.DUSEN -> "↘ Düşen" to Renk.kirmizi
        Yon.YATAY -> "→ Yatay" to Renk.mavi
    }
    Text(metin, color = renk, fontSize = 13.sp, fontWeight = FontWeight.Medium)
}

@Composable
private fun PuanRozeti(t: TfSonuc) {
    val renk = when {
        !t.uygun -> Renk.kenar
        t.puan >= 85 -> Renk.yesil
        t.puan >= 75 -> Renk.sari
        else -> Renk.soluk
    }
    Box(
        Modifier.size(40.dp).border(2.dp, renk, CircleShape),
        contentAlignment = Alignment.Center,
    ) {
        Text("${t.puan}", color = if (t.uygun) Renk.yazi else Renk.soluk, fontSize = 15.sp, fontWeight = FontWeight.SemiBold)
    }
}

/** Son mumların kapanışı: şoktan önceki kısım gri, stabil kısım yeşil, şoklar kırmızı nokta. */
@Composable
private fun MiniGrafik(t: TfSonuc) {
    val k = t.kapanislar
    val vurgu = if (t.uygun) Renk.yesil else Renk.soluk
    Canvas(Modifier.fillMaxWidth().height(72.dp)) {
        if (k.size < 2) return@Canvas
        var alt = k.minOrNull() ?: return@Canvas
        var ust = k.maxOrNull() ?: return@Canvas
        if (ust - alt < 1e-12) { ust += 1.0; alt -= 1.0 }
        val w = size.width
        val h = size.height
        fun x(i: Int): Float = i * w / (k.size - 1)
        fun y(v: Double): Float = (h * 0.95f - ((v - alt) / (ust - alt)).toFloat() * h * 0.9f)

        val bas = t.pencereBasi.coerceIn(0, k.size - 1)
        drawRect(
            color = vurgu.copy(alpha = 0.07f),
            topLeft = Offset(x(bas), 0f),
            size = Size(w - x(bas), h),
        )

        val adim = max(1, k.size / 240)
        val noktalar = ArrayList<Int>()
        var i = 0
        while (i < k.size) { noktalar.add(i); i += adim }
        if (noktalar.last() != k.size - 1) noktalar.add(k.size - 1)

        val eski = Path()
        val yeni = Path()
        var eskiBasladi = false
        for (j in noktalar) {
            if (j > bas) break
            if (!eskiBasladi) { eski.moveTo(x(j), y(k[j])); eskiBasladi = true } else eski.lineTo(x(j), y(k[j]))
        }
        if (eskiBasladi) eski.lineTo(x(bas), y(k[bas]))
        yeni.moveTo(x(bas), y(k[bas]))
        for (j in noktalar) if (j > bas) yeni.lineTo(x(j), y(k[j]))

        drawPath(eski, color = Renk.soluk.copy(alpha = 0.55f), style = Stroke(width = 1.5.dp.toPx(), join = StrokeJoin.Round))
        drawPath(yeni, color = vurgu, style = Stroke(width = 2.dp.toPx(), join = StrokeJoin.Round))
        for (s in t.sokIndexleri) {
            if (s in k.indices) drawCircle(Renk.kirmizi, radius = 3.5.dp.toPx(), center = Offset(x(s), y(k[s])))
        }
    }
}

@Composable
private fun AyarPenceresi(mevcut: Ayarlar, onKapat: () -> Unit, onUygula: (Ayarlar) -> Unit) {
    var hacim by remember { mutableStateOf(mevcut.enAzHacimUsdt) }
    var dalga by remember { mutableStateOf(mevcut.enKucukDalgaYuzde) }
    val hacimSecenekleri = listOf(250_000.0, 500_000.0, 1_000_000.0, 2_000_000.0, 5_000_000.0, 10_000_000.0)
    val dalgaSecenekleri = listOf(0.5, 0.75, 1.0, 1.5, 2.0, 3.0)

    AlertDialog(
        onDismissRequest = onKapat,
        containerColor = Renk.kart,
        title = { Text("Ayarlar", color = Renk.yazi) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text("En az günlük hacim", color = Renk.yazi, fontWeight = FontWeight.Medium)
                Text("Hacmi bunun altındaki coinlere bakılmaz.", color = Renk.soluk, fontSize = 12.sp)
                Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    hacimSecenekleri.forEach { v -> Cip(hacimMetni(v), secili = v == hacim, kucuk = true) { hacim = v } }
                }
                Spacer(Modifier.height(4.dp))
                Text("En küçük dalga", color = Renk.yazi, fontWeight = FontWeight.Medium)
                Text("Dipten tepeye bundan küçük oynayan coinler elenir.", color = Renk.soluk, fontSize = 12.sp)
                Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    dalgaSecenekleri.forEach { v -> Cip("%${sayi(v)}", secili = v == dalga, kucuk = true) { dalga = v } }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onUygula(Ayarlar(hacim, dalga)) }) { Text("Uygula", color = Renk.yesil) }
        },
        dismissButton = {
            TextButton(onClick = onKapat) { Text("Vazgeç", color = Renk.soluk) }
        },
    )
}

fun sayi(v: Double): String {
    val s = String.format(TR, "%.2f", v).trimEnd('0').trimEnd(',')
    return s.ifEmpty { "0" }
}

fun hacimMetni(usdt: Double): String = when {
    usdt >= 1_000_000_000 -> "${sayi(usdt / 1_000_000_000)} Mr$"
    else -> "${sayi(usdt / 1_000_000)} M$"
}

fun sureMetni(ms: Long, enAz: Boolean): String {
    val dk = ms / 60_000
    val metin = when {
        dk < 60 -> "$dk dakikadır"
        dk < 48 * 60 -> "${dk / 60} saattir"
        else -> "${dk / 1440} gündür"
    }
    return if (enAz) "en az $metin" else metin
}
