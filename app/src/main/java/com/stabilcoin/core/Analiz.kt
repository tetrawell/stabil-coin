package com.stabilcoin.core

import kotlin.math.abs
import kotlin.math.ln
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.sqrt

/** Tek bir mum. Zaman: mumun açılış anı (milisaniye). */
data class Mum(
    val zaman: Long,
    val acilis: Double,
    val yuksek: Double,
    val dusuk: Double,
    val kapanis: Double,
)

enum class Yon { YUKSELEN, YATAY, DUSEN }

/** Zigzag'in bulduğu dönüm noktası: bir tepe ya da bir dip. */
data class Donum(val index: Int, val fiyat: Double, val tepe: Boolean)

/** İki dönüm noktası arasındaki hareket (bir bacak). Boyu her zaman dipten tepeye yüzde. */
data class Bacak(val basIndex: Int, val sonIndex: Int, val yukari: Boolean, val boyYuzde: Double)

/** Bir coinin tek bir zaman dilimindeki sonucu. */
data class TfSonuc(
    val zamanDilimi: String,
    val uygun: Boolean,
    val puan: Int,
    val dalgaYuzde: Double,
    val dalgaSayisi: Int,
    val sureMs: Long,
    val enAz: Boolean,
    val yon: Yon,
    val pencereBasi: Int,
    val sokIndexleri: List<Int>,
    val kapanislar: DoubleArray,
    val neden: String,
)

/**
 * Stabil coin hesabı.
 *
 * Fikir (otobüs benzetmesi): coin bir yöne giderken hep benzer boyda yavaşlayıp hızlanıyorsa
 * karakterlidir. Program şimdiden geriye doğru bakar, ilk "şok"ta (dev mum, uzun iğne, kat
 * değiştiren sıçrama) durur. Şoktan şimdiye kadarki bölgede dalgalar benzer boydaysa ve yeterince
 * tekrarlanmışsa coin "X saattir stabil" sayılır.
 */
object Analiz {
    /** Bir mumun boyu, mumların ortanca boyunun bu katıysa şok adayıdır. */
    const val IGNE_MUM_KATI = 6.0

    /** ...ve aynı zamanda ortanca dalganın bu katı kadarsa şoktur. */
    const val IGNE_DALGA_KATI = 0.8

    /** Bir bacak, ortanca bacağın bu katıysa fiyat kat değiştirmiştir (sıçrama/çöküş). */
    const val SICRAMA_KATI = 2.5

    /** Karakteri kanıtlamak için gereken en az tam dalga (bir çıkış + bir iniş). */
    const val EN_AZ_DALGA = 5

    /** Dipten çıkışların ortancası, düşüşlerin ortancasının en az bu oranı olmalı (hızlı düşen elenir). */
    const val EN_AZ_CIKIS_ORANI = 0.5

    /** Bu puanın altı listede gösterilmez. */
    const val EN_AZ_PUAN = 65

    /** Dalga eşiği, "en küçük dalga" ayarının bu katlarıyla denenir; coinin kendi doğal dalgası aranır. */
    private val ESIK_KATLARI = doubleArrayOf(1.0, 1.25, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0, 13.0, 16.0)

    /** Sadece deneme için: her eşikteki adayı yazdırmak isteyen buraya bir fonksiyon bağlar. */
    var izle: ((String) -> Unit)? = null

    fun analiz(zamanDilimi: String, mumlar: List<Mum>, enKucukDalgaYuzde: Double): TfSonuc {
        val kapanislar = DoubleArray(mumlar.size) { mumlar[it].kapanis }
        if (mumlar.size < 60) {
            return bos(zamanDilimi, kapanislar, "Yeterli veri yok")
        }
        val mumBoylari = mumlar.map { boyYuzde(it.dusuk, it.yuksek) }
        val ortancaMum = max(ortanca(mumBoylari), 1e-9)

        var enIyi: Aday? = null
        var enIyiUygunsuz: Aday? = null
        for (kat in ESIK_KATLARI) {
            val esik = enKucukDalgaYuzde * kat
            val aday = degerlendir(mumlar, mumBoylari, ortancaMum, esik, kat) ?: continue
            izle?.invoke(
                String.format(
                    "  esik %.2f%% -> uygun=%s puan=%d dalga=%.2f%% x%d pencere=%d secim=%.1f %s",
                    esik, aday.uygun, aday.puan, aday.dalgaYuzde, aday.dalgaSayisi,
                    mumlar.size - aday.pencereBasi, aday.secimPuani, aday.neden,
                ),
            )
            if (aday.uygun) {
                if (enIyi == null || aday.secimPuani > enIyi.secimPuani) enIyi = aday
            } else {
                if (enIyiUygunsuz == null || aday.puan > enIyiUygunsuz.puan) enIyiUygunsuz = aday
            }
        }

        val secilen = enIyi ?: enIyiUygunsuz ?: return bos(zamanDilimi, kapanislar, "Dalga bulunamadı")
        val sure = if (secilen.pencereBasi < mumlar.size) {
            val mumAraligi = if (mumlar.size >= 2) mumlar[1].zaman - mumlar[0].zaman else 0L
            mumlar.last().zaman - mumlar[secilen.pencereBasi].zaman + mumAraligi
        } else 0L

        return TfSonuc(
            zamanDilimi = zamanDilimi,
            uygun = secilen.uygun,
            puan = secilen.puan,
            dalgaYuzde = secilen.dalgaYuzde,
            dalgaSayisi = secilen.dalgaSayisi,
            sureMs = sure,
            enAz = secilen.pencereBasi == 0,
            yon = secilen.yon,
            pencereBasi = secilen.pencereBasi,
            sokIndexleri = secilen.soklar,
            kapanislar = kapanislar,
            neden = secilen.neden,
        )
    }

    private class Aday(
        val uygun: Boolean,
        val puan: Int,
        val secimPuani: Double,
        val dalgaYuzde: Double,
        val dalgaSayisi: Int,
        val yon: Yon,
        val pencereBasi: Int,
        val soklar: List<Int>,
        val neden: String,
    )

    private fun degerlendir(
        m: List<Mum>,
        mumBoylari: List<Double>,
        ortancaMum: Double,
        esik: Double,
        kat: Double,
    ): Aday? {
        val n = m.size
        var bas = 0
        val soklar = mutableListOf<Int>()

        // 1) Şimdiden geriye: son şoku bul, pencereyi şoktan sonrasına daralt. Birkaç tur tekrarla.
        var bacaklar = bacaklar(zigzag(m, bas, esik))
        repeat(6) {
            if (bacaklar.size < 4) return@repeat
            val ortancaBacak = ortanca(bacaklar.map { it.boyYuzde })
            var kesim = bas

            // İğne / dev mum
            for (i in n - 1 downTo bas) {
                val b = mumBoylari[i]
                if (b >= IGNE_MUM_KATI * ortancaMum && b >= IGNE_DALGA_KATI * ortancaBacak) {
                    kesim = max(kesim, i + 1)
                    soklar.add(i)
                    break
                }
            }
            // Kat değiştiren sıçrama veya çöküş
            for (k in bacaklar.indices.reversed()) {
                val bc = bacaklar[k]
                if (bc.boyYuzde >= SICRAMA_KATI * ortancaBacak) {
                    if (bc.sonIndex > kesim) {
                        kesim = bc.sonIndex
                        soklar.add(bc.sonIndex)
                    }
                    break
                }
            }
            if (kesim <= bas) return@repeat
            bas = kesim
            bacaklar = bacaklar(zigzag(m, bas, esik))
        }

        if (bas >= n - 10) {
            return Aday(false, 0, 0.0, 0.0, 0, Yon.YATAY, bas, soklar.distinct(), "Az önce şok oldu")
        }

        val yukari = bacaklar.filter { it.yukari }.map { it.boyYuzde }
        val asagi = bacaklar.filter { !it.yukari }.map { it.boyYuzde }
        val dalgaSayisi = min(yukari.size, asagi.size)
        if (dalgaSayisi < 2) {
            return Aday(false, 0, 0.0, 0.0, dalgaSayisi, Yon.YATAY, bas, soklar.distinct(), "Bu boyda dalga yok")
        }

        // 2) Şu an süren hareket bir çöküş/sıçrama mı?
        val ortancaBacak = ortanca(bacaklar.map { it.boyYuzde })
        val sonDonumIndex = bacaklar.last().sonIndex
        val sonDonumFiyat = if (bacaklar.last().yukari) m[sonDonumIndex].yuksek else m[sonDonumIndex].dusuk
        val suankiHareket = abs(m.last().kapanis - sonDonumFiyat) / min(m.last().kapanis, sonDonumFiyat) * 100
        val suAnSok = suankiHareket >= SICRAMA_KATI * ortancaBacak

        // 3) Düzen: çıkışlar kendi aralarında, inişler kendi aralarında ne kadar benzer boyda?
        val duzen = (1.0 - (degiskenlik(yukari) + degiskenlik(asagi)) / 2.0).coerceIn(0.0, 1.0)

        // 4) Ritim: çıkışlar ve inişler benzer sürede mi tamamlanıyor?
        val yukariSure = bacaklar.filter { it.yukari }.map { (it.sonIndex - it.basIndex).toDouble() }
        val asagiSure = bacaklar.filter { !it.yukari }.map { (it.sonIndex - it.basIndex).toDouble() }
        val ritim = (1.0 - (degiskenlik(yukariSure) + degiskenlik(asagiSure)) / 2.0).coerceIn(0.0, 1.0)

        // 5) Kanal: fiyat, yönünün etrafında dar bir bantta mı kalıyor?
        val (bant, egilimYuzde) = bantVeEgilim(m, bas)
        val bantOrani = bant / max(ortancaBacak, 1e-9)
        val kanal = (1.0 - (bantOrani - 1.5) / 3.5).coerceIn(0.0, 1.0)

        val puan = (100 * (0.45 * duzen + 0.25 * ritim + 0.30 * kanal)).roundToInt()

        val ortancaCikis = ortanca(yukari)
        val ortancaInis = ortanca(asagi)
        val cikisOrani = ortancaCikis / max(ortancaInis, 1e-9)

        val yon = when {
            egilimYuzde >= 0.6 * ortancaBacak -> Yon.YUKSELEN
            egilimYuzde <= -0.6 * ortancaBacak -> Yon.DUSEN
            else -> Yon.YATAY
        }

        val neden = when {
            suAnSok -> "Şu an sert hareket var"
            dalgaSayisi < EN_AZ_DALGA -> "Az dalga ($dalgaSayisi)"
            cikisOrani < EN_AZ_CIKIS_ORANI -> "Çok hızlı düşüyor"
            puan < EN_AZ_PUAN -> "Dalgalar düzensiz"
            else -> ""
        }
        val uygun = neden.isEmpty()
        // Ölçek seçimi: düzenli olan, uzun süredir stabil olan ve dalgası büyük olan öne geçer.
        val pencereOrani = (n - bas).toDouble() / n
        val secimPuani = puan + 20.0 * pencereOrani + 3.0 * ln(kat) / ln(2.0)

        return Aday(uygun, puan, secimPuani, ortancaCikis, dalgaSayisi, yon, bas, soklar.distinct(), neden)
    }

    /** Fiyat, eşik yüzdesi kadar geri dönünce bir tepe ya da dip onaylanır. Son, onaylanmamış uç eklenmez. */
    fun zigzag(m: List<Mum>, bas: Int, esikYuzde: Double): List<Donum> {
        val donumler = ArrayList<Donum>()
        if (m.size - bas < 3) return donumler
        var yon = 0
        var tepeI = bas
        var tepe = m[bas].yuksek
        var dipI = bas
        var dip = m[bas].dusuk
        for (i in bas + 1 until m.size) {
            val h = m[i].yuksek
            val l = m[i].dusuk
            when (yon) {
                0 -> {
                    if (h > tepe) { tepe = h; tepeI = i }
                    if (l < dip) { dip = l; dipI = i }
                    if (boyYuzde(dip, tepe) >= esikYuzde) {
                        val dipOnce = dipI < tepeI || (dipI == tepeI && m[i].kapanis >= m[i].acilis)
                        if (dipOnce) {
                            donumler.add(Donum(dipI, dip, false)); yon = 1
                        } else {
                            donumler.add(Donum(tepeI, tepe, true)); yon = -1
                        }
                    }
                }
                1 -> {
                    if (h > tepe) {
                        tepe = h; tepeI = i
                    } else if (boyYuzde(l, tepe) >= esikYuzde) {
                        donumler.add(Donum(tepeI, tepe, true))
                        yon = -1; dip = l; dipI = i
                    }
                }
                else -> {
                    if (l < dip) {
                        dip = l; dipI = i
                    } else if (boyYuzde(dip, h) >= esikYuzde) {
                        donumler.add(Donum(dipI, dip, false))
                        yon = 1; tepe = h; tepeI = i
                    }
                }
            }
        }
        return donumler
    }

    fun bacaklar(d: List<Donum>): List<Bacak> {
        val sonuc = ArrayList<Bacak>()
        for (k in 1 until d.size) {
            val a = d[k - 1]
            val b = d[k]
            sonuc.add(Bacak(a.index, b.index, !a.tepe, boyYuzde(min(a.fiyat, b.fiyat), max(a.fiyat, b.fiyat))))
        }
        return sonuc
    }

    /**
     * Kapanışlara düz bir çizgi oturtur. İlk değer: fiyatın bu çizgiden saptığı bant genişliği (%).
     * İkinci değer: çizginin pencere boyunca toplam eğilimi (%), yani yön.
     */
    private fun bantVeEgilim(m: List<Mum>, bas: Int): Pair<Double, Double> {
        val n = m.size - bas
        if (n < 3) return 0.0 to 0.0
        var sx = 0.0; var sy = 0.0; var sxx = 0.0; var sxy = 0.0
        for (i in 0 until n) {
            val y = m[bas + i].kapanis
            sx += i; sy += y; sxx += i.toDouble() * i; sxy += i * y
        }
        val egim = (n * sxy - sx * sy) / (n * sxx - sx * sx)
        val kesen = (sy - egim * sx) / n
        var enUst = Double.NEGATIVE_INFINITY
        var enAlt = Double.POSITIVE_INFINITY
        for (i in 0 until n) {
            val cizgi = kesen + egim * i
            if (cizgi <= 0) continue
            val sapma = (m[bas + i].kapanis - cizgi) / cizgi * 100
            enUst = max(enUst, sapma); enAlt = min(enAlt, sapma)
        }
        val bant = if (enUst.isFinite() && enAlt.isFinite()) enUst - enAlt else 0.0
        val egilim = if (kesen > 0) egim * (n - 1) / kesen * 100 else 0.0
        return bant to egilim
    }

    private fun bos(tf: String, kapanislar: DoubleArray, neden: String) = TfSonuc(
        tf, false, 0, 0.0, 0, 0L, false, Yon.YATAY, kapanislar.size, emptyList(), kapanislar, neden,
    )

    fun boyYuzde(alt: Double, ust: Double): Double = if (alt <= 0) 0.0 else (ust - alt) / alt * 100

    fun ortanca(x: List<Double>): Double {
        if (x.isEmpty()) return 0.0
        val s = x.sorted()
        val o = s.size / 2
        return if (s.size % 2 == 1) s[o] else (s[o - 1] + s[o]) / 2
    }

    /** Değişkenlik katsayısı: sapma / ortalama. 0 = hepsi aynı boy. */
    private fun degiskenlik(x: List<Double>): Double {
        if (x.size < 2) return 1.0
        val ort = x.average()
        if (ort <= 0) return 1.0
        val v = x.sumOf { (it - ort) * (it - ort) } / (x.size - 1)
        return sqrt(v) / ort
    }
}
