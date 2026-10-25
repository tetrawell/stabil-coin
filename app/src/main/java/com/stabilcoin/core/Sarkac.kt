package com.stabilcoin.core

import kotlin.math.ln
import kotlin.math.roundToInt
import kotlin.math.sqrt

/**
 * Sarkaç ölçüsü: fiyat hep aynı ortalamaya geri mi dönüyor (sarkaç), yoksa bir yöne mi kaçıyor (yokuştan yuvarlanan top)?
 *
 * Nasıl ölçülür: kısa adımların (1 mum) oynaklığı ile uzun adımların (60 mum) oynaklığı karşılaştırılır.
 * Fiyat her adımda rastgele gitseydi 60 mumluk oynaklık, tek mumunkinin tam 60 katı olurdu (oran = 1).
 * Kısa inip çıkmalar birbirini siliyorsa oran 1'in altına düşer: fiyat geri dönüyor demektir.
 * Kısa hareketler birbirine ekleniyorsa oran 1'in üstüne çıkar: fiyat bir yöne kaçıyor demektir.
 *
 * Kripto Simülatör araştırmasında (Ekim 2026, 62 likit coin) bu oranın 0,6'nın altında olduğu ve BTC ile birlikte
 * hareket etmeyen coinlerde botun sonucu belirgin şekilde daha iyiydi; en net 5 dk grafikte. Kanıtlanmış değildir.
 *
 * Tuzak: az işlem gören (sığ) coinlerde fiyat alış ve satış fiyatı arasında zıplar; bu da sahte bir "sarkaç" gibi görünür.
 * Bu yüzden sığ coinler yıldız almaz (siglik).
 */
object Sarkac {
    /** Uzun adımın kaç mum olduğu. */
    const val UFUK = 60

    /** Oran bunun altındaysa coin sarkaç sayılır (puan 40 ve üstü). */
    const val SARKAC_SINIRI = 0.6

    /** Coinin kısa vadeli getirileri ile BTC'ninkiler arasındaki ilişki (korelasyon) bunun altındaysa coin "BTC'den bağımsız". */
    const val BTC_BAGI_SINIRI = 0.4

    /** 1 dk mumlardan tahmin edilen alış-satış farkı (%) bunun üstündeyse coin sığdır. */
    const val SIG_FARK_YUZDE = 0.25

    /** 1 dk mumlarda hiç kıpırdamayan (en yüksek = en düşük) mumların payı bunun üstündeyse coin sığdır. */
    const val SIG_HAREKETSIZ = 0.20

    /** Bir coinin 1 dk mumlarından çıkan sığlık ölçüleri. */
    data class Siglik(val farkYuzde: Double, val hareketsizPay: Double) {
        val sig: Boolean get() = farkYuzde > SIG_FARK_YUZDE || hareketsizPay > SIG_HAREKETSIZ
    }

    /**
     * Oran (varyans oranı): uzun adımların oynaklığı / (k × kısa adımların oynaklığı).
     * Yeterli mum yoksa ya da fiyat hiç oynamamışsa null.
     */
    fun oran(kapanislar: DoubleArray, k: Int = UFUK): Double? {
        val n = kapanislar.size
        if (k < 2 || n <= 3 * k) return null
        for (c in kapanislar) if (!(c > 0)) return null
        val lg = DoubleArray(n) { ln(kapanislar[it]) }
        val kisa = DoubleArray(n - 1) { lg[it + 1] - lg[it] }
        val uzun = DoubleArray(n - k) { lg[it + k] - lg[it] }
        val vKisa = varyans(kisa)
        if (!(vKisa > 0)) return null
        return varyans(uzun) / (k * vKisa)
    }

    /** Ekranda görünen puan: 100 × (1 − oran). 40 ve üstü sarkaç; eksi değer fiyatın bir yöne kaçtığını gösterir. */
    fun puan(oran: Double): Int = (100.0 * (1.0 - oran)).roundToInt().coerceIn(-99, 99)

    /**
     * Coin ile BTC'nin aynı mumlardaki getirileri arasındaki ilişki (-1..1). Mumlar zamanına göre eşleştirilir;
     * iki ardışık mum ikisinde de yoksa o adım atlanır. Yeterli ortak adım yoksa null.
     */
    fun btcBagi(coin: List<Mum>, btc: List<Mum>): Double? {
        if (coin.size < 3 || btc.size < 3) return null
        val btcFiyat = HashMap<Long, Double>(btc.size * 2)
        for (m in btc) btcFiyat[m.zaman] = m.kapanis
        val x = ArrayList<Double>(coin.size)
        val y = ArrayList<Double>(coin.size)
        for (i in 1 until coin.size) {
            val a0 = coin[i - 1].kapanis
            val a1 = coin[i].kapanis
            val b0 = btcFiyat[coin[i - 1].zaman] ?: continue
            val b1 = btcFiyat[coin[i].zaman] ?: continue
            if (!(a0 > 0 && a1 > 0 && b0 > 0 && b1 > 0)) continue
            x.add(ln(a1 / a0)); y.add(ln(b1 / b0))
        }
        if (x.size < 100) return null
        return iliski(x, y)
    }

    /** 1 dk mumlardan sığlık: hareketsiz mum payı ve Roll yöntemiyle tahmini alış-satış farkı (%). */
    fun siglik(mumlar1dk: List<Mum>): Siglik? {
        if (mumlar1dk.size < 200) return null
        val hareketsiz = mumlar1dk.count { it.yuksek == it.dusuk }.toDouble() / mumlar1dk.size
        val d = ArrayList<Double>(mumlar1dk.size)
        for (i in 1 until mumlar1dk.size) {
            val a = mumlar1dk[i - 1].kapanis
            val b = mumlar1dk[i].kapanis
            if (a > 0 && b > 0) d.add(ln(b / a))
        }
        if (d.size < 100) return null
        val ort = d.average()
        var kov = 0.0
        for (i in 1 until d.size) kov += (d[i] - ort) * (d[i - 1] - ort)
        kov /= (d.size - 1)
        val fark = if (kov < 0) 2.0 * sqrt(-kov) * 100.0 else 0.0
        return Siglik(fark, hareketsiz)
    }

    /** Yıldız: sarkaç + BTC'den bağımsız + sığ değil. Bilinmeyen bir ölçü varsa yıldız verilmez. */
    fun yildiz(oran: Double?, btcBagi: Double?, siglik: Siglik?): Boolean =
        oran != null && oran < SARKAC_SINIRI &&
            btcBagi != null && btcBagi < BTC_BAGI_SINIRI &&
            siglik != null && !siglik.sig

    private fun varyans(x: DoubleArray): Double {
        val ort = x.average()
        var s = 0.0
        for (v in x) s += (v - ort) * (v - ort)
        return s / x.size
    }

    private fun iliski(x: List<Double>, y: List<Double>): Double? {
        val mx = x.average()
        val my = y.average()
        var sxy = 0.0; var sxx = 0.0; var syy = 0.0
        for (i in x.indices) {
            val a = x[i] - mx
            val b = y[i] - my
            sxy += a * b; sxx += a * a; syy += b * b
        }
        if (!(sxx > 0 && syy > 0)) return null
        return sxy / sqrt(sxx * syy)
    }
}
