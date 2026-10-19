package com.stabilcoin.ui

import android.app.Application
import android.content.Context
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.stabilcoin.core.Analiz
import com.stabilcoin.core.Mum
import com.stabilcoin.core.TfSonuc
import com.stabilcoin.data.MexcIstemci
import com.stabilcoin.data.Parite
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger

/** Uygulamanın baktığı zaman dilimleri (simülatördekilerle aynı). */
val ZAMAN_DILIMLERI = listOf("1m", "5m", "15m", "1h", "4h", "1d")

fun tfAdi(tf: String): String = when (tf) {
    "1m" -> "1dk"
    "5m" -> "5dk"
    "15m" -> "15dk"
    "1h" -> "1s"
    "4h" -> "4s"
    "1d" -> "1g"
    else -> tf
}

data class Ayarlar(
    val enAzHacimUsdt: Double = 1_000_000.0,
    val enKucukDalgaYuzde: Double = 1.0,
)

data class CoinSonuc(
    val sembol: String,
    val hacimUsdt: Double,
    val tfler: List<TfSonuc>,
)

data class Durum(
    val yukleniyor: Boolean = false,
    val hesaplaniyor: Boolean = false,
    val ilerleme: Int = 0,
    val toplam: Int = 0,
    val coinSayisi: Int = 0,
    val coinler: List<CoinSonuc> = emptyList(),
    val hata: String? = null,
    val sonGuncelleme: Long? = null,
    val eksikGrafik: Int = 0,
)

class AnaModel(uygulama: Application) : AndroidViewModel(uygulama) {
    private val tercihler = uygulama.getSharedPreferences("ayarlar", Context.MODE_PRIVATE)

    private val _ayarlar = MutableStateFlow(
        Ayarlar(
            enAzHacimUsdt = tercihler.getFloat("hacim", 1_000_000f).toDouble(),
            enKucukDalgaYuzde = tercihler.getFloat("dalga", 1f).toDouble(),
        ),
    )
    val ayarlar: StateFlow<Ayarlar> = _ayarlar.asStateFlow()

    private val _durum = MutableStateFlow(Durum())
    val durum: StateFlow<Durum> = _durum.asStateFlow()

    /** Bu oturumda indirilen mumlar: sembol -> zaman dilimi -> mumlar. Uygulama kapanınca silinir. */
    private val onbellek = ConcurrentHashMap<String, Map<String, List<Mum>>>()
    private var pariteler: List<Parite> = emptyList()
    private var tarama: Job? = null

    init {
        tara()
    }

    /** MEXC'den listeyi ve grafikleri baştan indirip hesaplar. */
    fun tara() {
        tarama?.cancel()
        tarama = viewModelScope.launch {
            _durum.update { it.copy(yukleniyor = true, hata = null, ilerleme = 0, toplam = 0, eksikGrafik = 0) }
            try {
                val ayar = _ayarlar.value
                val liste = withContext(Dispatchers.IO) { MexcIstemci.pariteler(ayar.enAzHacimUsdt) }
                pariteler = liste
                onbellek.clear()
                val toplam = liste.size * ZAMAN_DILIMLERI.size
                _durum.update { it.copy(toplam = toplam, coinSayisi = liste.size) }

                val kapi = Semaphore(5)
                val biten = AtomicInteger(0)
                val hatali = AtomicInteger(0)
                coroutineScope {
                    for (p in liste) {
                        for (tf in ZAMAN_DILIMLERI) {
                            launch(Dispatchers.IO) {
                                kapi.withPermit {
                                    try {
                                        val m = MexcIstemci.mumlar(p.sembol, tf)
                                        onbellek.compute(p.sembol) { _, eski -> (eski ?: emptyMap()) + (tf to m) }
                                    } catch (e: CancellationException) {
                                        throw e
                                    } catch (e: Exception) {
                                        hatali.incrementAndGet()
                                    }
                                }
                                val n = biten.incrementAndGet()
                                if (n % 4 == 0 || n == toplam) _durum.update { it.copy(ilerleme = n) }
                            }
                        }
                    }
                }

                if (toplam > 0 && hatali.get() == toplam) {
                    _durum.update {
                        it.copy(yukleniyor = false, hata = "Grafikler indirilemedi. İnternetini kontrol edip tekrar dene.")
                    }
                    return@launch
                }
                hesapla()
                _durum.update {
                    it.copy(
                        yukleniyor = false,
                        sonGuncelleme = System.currentTimeMillis(),
                        eksikGrafik = hatali.get(),
                    )
                }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                _durum.update {
                    it.copy(yukleniyor = false, hata = "MEXC'ye ulaşılamadı. İnternetini kontrol edip tekrar dene.")
                }
            }
        }
    }

    /** İndirilmiş mumlarla, yeniden indirmeden hesaplar (dalga ayarı değişince). */
    private suspend fun hesapla() {
        val dalga = _ayarlar.value.enKucukDalgaYuzde
        val liste = pariteler
        val sonuc = withContext(Dispatchers.Default) {
            liste.mapNotNull { p ->
                val tfMumlari = onbellek[p.sembol] ?: return@mapNotNull null
                val tfler = ZAMAN_DILIMLERI.mapNotNull { tf ->
                    tfMumlari[tf]?.let { Analiz.analiz(tf, it, dalga) }
                }
                CoinSonuc(p.sembol, p.hacimUsdt, tfler)
            }
        }
        _durum.update { it.copy(coinler = sonuc) }
    }

    fun ayarlariUygula(yeni: Ayarlar) {
        val eski = _ayarlar.value
        if (yeni == eski) return
        _ayarlar.value = yeni
        tercihler.edit()
            .putFloat("hacim", yeni.enAzHacimUsdt.toFloat())
            .putFloat("dalga", yeni.enKucukDalgaYuzde.toFloat())
            .apply()
        if (yeni.enAzHacimUsdt != eski.enAzHacimUsdt || pariteler.isEmpty()) {
            tara()
        } else {
            viewModelScope.launch {
                _durum.update { it.copy(hesaplaniyor = true) }
                hesapla()
                _durum.update { it.copy(hesaplaniyor = false) }
            }
        }
    }
}
