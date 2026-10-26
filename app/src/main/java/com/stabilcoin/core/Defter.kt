package com.stabilcoin.core

import kotlin.math.abs
import kotlin.math.sqrt

/**
 * Alış-satış defteri: bir coini alıp satmanın gerçek masrafı ve coinin salınımının bu masrafı karşılayıp karşılamadığı.
 *
 * Neden (Raspberry Pi'deki Kripto Al-Sat, 25-26 Ekim 2026, 21 sanal kumbara): simülatörün beklediği ile gerçekleşen
 * arasındaki farkın neredeyse tamamı, defteri ince dört coinden geldi (AXIS, BDX, DOS, MBG). Hacim bunu göstermiyordu:
 * AXIS ile BNB'nin günlük hacmi aynıydı ama fiyatın %0,5 altına kadar bekleyen alıcı BNB'de 134.000 $, AXIS'te 582 $.
 *
 * İnce defter tek başına eleme sebebi değildir. Asıl soru: coinin salınımı alıp satma masrafını kaç kez karşılıyor?
 * Köprü geçiş ücreti gibi: karşıda kazanılacak para ücretin birkaç katı değilse geçmeye değmez.
 */
object Defter {
    /** ★ için salınım, alıp satma masrafının en az bu katı olmalı. (26 Ekim 2026 taramasında ★ adayları ya 1,7 katın
     *  altında ya da 4,7 katın üstündeydi; 2 ile 4 arası her sınır aynı sonucu verdi.) */
    const val YETER_KAT = 3.0

    /** Komisyonu öğrenilemeyen coinde tek işlemin komisyonu (%). Simülatördeki varsayılan masrafla aynı. */
    const val VARSAYILAN_KOMISYON_YUZDE = 0.1

    /** Salınımın ölçüldüğü pencere (mum). */
    const val SALINIM_PENCERE = 20

    data class Basamak(val fiyat: Double, val miktar: Double)

    /** Defterin bir anlık görüntüsü: alıcılar yüksek fiyattan düşüğe, satıcılar düşükten yükseğe. */
    data class Kitap(val alicilar: List<Basamak>, val saticilar: List<Basamak>)

    /** Belli bir tutarla bir alış ve bir satışın masrafı. */
    data class Masraf(
        val tutarUsdt: Double,
        /** En iyi satıcı ile en iyi alıcı arasındaki fark (%). */
        val farkYuzde: Double,
        /** Bu tutarda piyasa alımının ortalama fiyatı, orta fiyattan % kaç yukarıda. Defter yetmediyse null. */
        val alisYuzde: Double?,
        /** Bu tutarda piyasa satışının ortalama fiyatı, orta fiyattan % kaç aşağıda. Defter yetmediyse null. */
        val satisYuzde: Double?,
        /** Tek işlemin (alış ya da satış) komisyonu (%). */
        val komisyonYuzde: Double,
    ) {
        /** Bir alış + bir satış: fiyat kayması + iki komisyon (%). Defter bu tutara yetmiyorsa null. */
        val toplamYuzde: Double?
            get() = if (alisYuzde == null || satisYuzde == null) null else alisYuzde + satisYuzde + 2 * komisyonYuzde
    }

    /** Defterden, verilen tutarda alıp satmanın masrafı. Defter boş ya da bozuksa null. */
    fun masraf(k: Kitap, tutarUsdt: Double, komisyonYuzde: Double): Masraf? {
        val b0 = k.alicilar.firstOrNull()?.fiyat ?: return null
        val a0 = k.saticilar.firstOrNull()?.fiyat ?: return null
        if (!(b0 > 0 && a0 >= b0)) return null
        val orta = (a0 + b0) / 2
        return Masraf(
            tutarUsdt = tutarUsdt,
            farkYuzde = (a0 / b0 - 1) * 100,
            alisYuzde = dolum(k.saticilar, tutarUsdt, orta),
            satisYuzde = dolum(k.alicilar, tutarUsdt, orta),
            komisyonYuzde = komisyonYuzde,
        )
    }

    /** Tutar kadar piyasa emri defteri basamak basamak yer; ortalama fiyatın orta fiyattan uzaklığı (%). */
    private fun dolum(liste: List<Basamak>, tutar: Double, orta: Double): Double? {
        var kalan = tutar
        var adet = 0.0
        var para = 0.0
        for (b in liste) {
            if (!(b.fiyat > 0 && b.miktar > 0)) continue
            val u = minOf(kalan, b.fiyat * b.miktar)
            para += u
            adet += u / b.fiyat
            kalan -= u
            if (kalan <= 1e-9) break
        }
        if (kalan > 1e-9 || adet <= 0) return null
        return abs(para / adet / orta - 1) * 100
    }

    /**
     * Salınım (%): fiyatın son 20 mumluk ortalamasından tipik uzaklığı (2 standart sapma; Bollinger bandının ortadan
     * kenara mesafesi). Her 20 mumluk pencere için hesaplanır, ortancası alınır. Yeterli mum yoksa null.
     */
    fun salinim(kapanislar: DoubleArray, pencere: Int = SALINIM_PENCERE): Double? {
        val n = kapanislar.size
        if (n < 6 * pencere) return null
        val degerler = ArrayList<Double>(n - pencere + 1)
        for (i in pencere..n) {
            var top = 0.0
            for (j in i - pencere until i) top += kapanislar[j]
            val ort = top / pencere
            if (!(ort > 0)) continue
            var v = 0.0
            for (j in i - pencere until i) { val d = kapanislar[j] - ort; v += d * d }
            degerler.add(2 * sqrt(v / pencere) / ort * 100)
        }
        return if (degerler.isEmpty()) null else Analiz.ortanca(degerler)
    }

    /** Salınım masrafın kaç katı. Biri bilinmiyorsa ya da defter tutara yetmiyorsa null. */
    fun kat(salinimYuzde: Double?, m: Masraf?): Double? {
        val toplam = m?.toplamYuzde ?: return null
        if (salinimYuzde == null || !(toplam > 0)) return null
        return salinimYuzde / toplam
    }

    fun yeter(kat: Double?): Boolean = kat != null && kat >= YETER_KAT
}
