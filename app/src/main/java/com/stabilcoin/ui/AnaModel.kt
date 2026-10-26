package com.stabilcoin.ui

import android.app.Application
import android.content.Context
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.stabilcoin.core.Analiz
import com.stabilcoin.core.Defter
import com.stabilcoin.core.Mum
import com.stabilcoin.core.Sarkac
import com.stabilcoin.core.TfSonuc
import com.stabilcoin.data.MexcIstemci
import com.stabilcoin.data.Parite
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Semaphore
import kotlinx.coroutines.sync.withPermit
import kotlinx.coroutines.withContext
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.ConcurrentLinkedQueue
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
    /** Kumbaraya koyulacak para (USDT). Alıp satma masrafı bu tutara göre hesaplanır. */
    val kumbaraUsdt: Double = 100.0,
)

data class CoinSonuc(
    val sembol: String,
    val hacimUsdt: Double,
    val tfler: List<TfSonuc>,
    /** 1 dk mumlardan sığlık (alış-satış farkı, hareketsiz mumlar). 1 dk grafik yoksa null. */
    val siglik: Sarkac.Siglik? = null,
    /** Alış-satış defterinden, ayardaki kumbara tutarıyla bir alış + bir satışın masrafı. Defter alınamadıysa null. */
    val masraf: Defter.Masraf? = null,
) {
    /** Sarkaç + BTC'den bağımsız + sığ değil (defterden önceki şartlar). */
    fun sarkacAday(t: TfSonuc): Boolean = Sarkac.yildiz(t.sarkacOrani, t.btcBagi, siglik)

    /** Bu zaman diliminde salınım, alıp satma masrafının kaç katı. Bilinmiyorsa null. */
    fun kat(t: TfSonuc): Double? = Defter.kat(t.salinimYuzde, masraf)

    /** Yıldızlı sarkaç: sarkaç adayı + salınımı alıp satma masrafının en az 3 katı. */
    fun yildiz(t: TfSonuc): Boolean = sarkacAday(t) && Defter.yeter(kat(t))
}

/** Sarkaç oranı ve BTC bağı için kıyas alınan coin. */
const val KIYAS_SEMBOL = "BTCUSDT"

/** İndirme listesinde zaman dilimlerinin yanında "alış-satış defteri" işi. */
private const val DEFTER = "defter"

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
    /** Alış-satış defteri alınamayan coin sayısı (bu coinlerde masraf bilinmez, yıldız verilmez). */
    val eksikDefter: Int = 0,
)

class AnaModel(uygulama: Application) : AndroidViewModel(uygulama) {
    private val tercihler = uygulama.getSharedPreferences("ayarlar", Context.MODE_PRIVATE)

    private val _ayarlar = MutableStateFlow(
        Ayarlar(
            enAzHacimUsdt = tercihler.getFloat("hacim", 1_000_000f).toDouble(),
            enKucukDalgaYuzde = tercihler.getFloat("dalga", 1f).toDouble(),
            kumbaraUsdt = tercihler.getFloat("kumbara", 100f).toDouble(),
        ),
    )
    val ayarlar: StateFlow<Ayarlar> = _ayarlar.asStateFlow()

    private val _durum = MutableStateFlow(Durum())
    val durum: StateFlow<Durum> = _durum.asStateFlow()

    /** Bu oturumda indirilen mumlar: sembol -> zaman dilimi -> mumlar. Uygulama kapanınca silinir. */
    private val onbellek = ConcurrentHashMap<String, Map<String, List<Mum>>>()

    /** Bu oturumda alınan alış-satış defterleri ve coin komisyonları (%). */
    private val defterler = ConcurrentHashMap<String, Defter.Kitap>()
    private var komisyonlar: Map<String, Double> = emptyMap()
    private var pariteler: List<Parite> = emptyList()
    private var tarama: Job? = null

    init {
        tara()
    }

    /** MEXC'den listeyi ve grafikleri baştan indirip hesaplar. */
    fun tara() {
        tarama?.cancel()
        tarama = viewModelScope.launch {
            _durum.update { it.copy(yukleniyor = true, hata = null, ilerleme = 0, toplam = 0, eksikGrafik = 0, eksikDefter = 0) }
            try {
                val ayar = _ayarlar.value
                val liste = withContext(Dispatchers.IO) { MexcIstemci.pariteler(ayar.enAzHacimUsdt) }
                pariteler = liste
                onbellek.clear()
                defterler.clear()
                komisyonlar = withContext(Dispatchers.IO) { MexcIstemci.komisyonlar(liste.map { it.sembol }) }
                // Sarkaç puanındaki "BTC bağı" için BTC grafikleri her zaman indirilir (hacim sınırı ne olursa olsun).
                val semboller = liste.map { it.sembol }.let { if (KIYAS_SEMBOL in it) it else it + KIYAS_SEMBOL }
                // Her coin için zaman dilimleri + bir de alış-satış defteri.
                val isler = ZAMAN_DILIMLERI + DEFTER
                val toplam = semboller.size * isler.size
                _durum.update { it.copy(toplam = toplam, coinSayisi = liste.size) }

                val kapi = Semaphore(5)
                val biten = AtomicInteger(0)
                val atlanan = ConcurrentLinkedQueue<Pair<String, String>>()
                coroutineScope {
                    for (sembol in semboller) {
                        for (tf in isler) {
                            launch(Dispatchers.IO) {
                                kapi.withPermit {
                                    try {
                                        indir(sembol, tf)
                                    } catch (e: CancellationException) {
                                        throw e
                                    } catch (e: Exception) {
                                        atlanan.add(sembol to tf)
                                    }
                                }
                                val n = biten.incrementAndGet()
                                if (n % 4 == 0 || n == toplam) _durum.update { it.copy(ilerleme = n) }
                            }
                        }
                    }
                }

                // MEXC anlık yoğunluk yüzünden bir grafiği vermediyse, sonda tek tek bir kez daha dene.
                // Böylece hiçbir coin sessizce atlanmaz.
                val kalan = ConcurrentLinkedQueue<Pair<String, String>>()
                if (atlanan.isNotEmpty() && atlanan.size < toplam) {
                    withContext(Dispatchers.IO) {
                        for ((sembol, tf) in atlanan) {
                            delay(300)
                            try {
                                indir(sembol, tf)
                            } catch (e: CancellationException) {
                                throw e
                            } catch (e: Exception) {
                                kalan.add(sembol to tf)
                            }
                        }
                    }
                } else {
                    kalan.addAll(atlanan)
                }
                val hatali = kalan.size
                val eksikDefter = kalan.count { it.second == DEFTER }

                if (toplam > 0 && hatali == toplam) {
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
                        eksikGrafik = hatali - eksikDefter,
                        eksikDefter = eksikDefter,
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

    /** Bir coinin bir zaman dilimindeki mumlarını ya da (DEFTER ise) alış-satış defterini indirip saklar. */
    private suspend fun indir(sembol: String, tf: String) {
        if (tf == DEFTER) {
            defterler[sembol] = MexcIstemci.defter(sembol)
        } else {
            val m = MexcIstemci.mumlar(sembol, tf)
            onbellek.compute(sembol) { _, eski -> (eski ?: emptyMap()) + (tf to m) }
        }
    }

    /** İndirilmiş mumlarla, yeniden indirmeden hesaplar (dalga ya da kumbara tutarı ayarı değişince). */
    private suspend fun hesapla() {
        val dalga = _ayarlar.value.enKucukDalgaYuzde
        val tutar = _ayarlar.value.kumbaraUsdt
        val kom = komisyonlar
        val liste = pariteler
        val btc = onbellek[KIYAS_SEMBOL]
        val sonuc = withContext(Dispatchers.Default) {
            liste.mapNotNull { p ->
                val tfMumlari = onbellek[p.sembol] ?: return@mapNotNull null
                val tfler = ZAMAN_DILIMLERI.mapNotNull { tf ->
                    tfMumlari[tf]?.let { m ->
                        // Stabil hesabı aynen kalır; sarkaç ölçüleri yanına eklenir (indirilen bütün mumlardan).
                        val kapanislar = DoubleArray(m.size) { m[it].kapanis }
                        Analiz.analiz(tf, m, dalga).copy(
                            sarkacOrani = Sarkac.oran(kapanislar),
                            btcBagi = btc?.get(tf)?.let { Sarkac.btcBagi(m, it) },
                            salinimYuzde = Defter.salinim(kapanislar),
                        )
                    }
                }
                val masraf = defterler[p.sembol]?.let {
                    Defter.masraf(it, tutar, kom[p.sembol] ?: Defter.VARSAYILAN_KOMISYON_YUZDE)
                }
                CoinSonuc(p.sembol, p.hacimUsdt, tfler, tfMumlari["1m"]?.let { Sarkac.siglik(it) }, masraf)
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
            .putFloat("kumbara", yeni.kumbaraUsdt.toFloat())
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
