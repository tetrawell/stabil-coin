package com.stabilcoin.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.exp
import kotlin.random.Random

/**
 * Sarkaç ölçüsünün testleri. Gerçek MEXC mumlarıyla (test/resources) beklenen değerler, Kripto Simülatör
 * araştırmasında kullanılan Python hesabıyla üretildi (scratchpad/fikstur.py); uygulama aynı sonucu vermeli.
 */
class SarkacTest {
    private fun mumlar(dosya: String): List<Mum> {
        val akis = javaClass.classLoader!!.getResourceAsStream(dosya) ?: error("$dosya yok")
        return akis.bufferedReader().readLines().filter { it.isNotBlank() }.map { satir ->
            val p = satir.split(',')
            Mum(p[0].toLong(), p[1].toDouble(), p[2].toDouble(), p[3].toDouble(), p[4].toDouble())
        }
    }

    private fun kapanis(m: List<Mum>) = DoubleArray(m.size) { m[it].kapanis }

    @Test
    fun gercekMumlardaPythonIleAyni() {
        val xmr = mumlar("xmr_5m.csv")
        val btc = mumlar("btc_5m.csv")
        assertEquals(1500, xmr.size)
        assertEquals(1.0330720061671839, Sarkac.oran(kapanis(xmr))!!, 1e-9)
        assertEquals(0.200144992116885, Sarkac.btcBagi(xmr, btc)!!, 1e-9)
        assertEquals(-3, Sarkac.puan(1.0330720061671839))
    }

    @Test
    fun sigliktaPythonIleAyni() {
        val xmr = Sarkac.siglik(mumlar("xmr_1m.csv"))!!
        assertEquals(0.07383515955998404, xmr.farkYuzde, 1e-9)
        assertEquals(0.008, xmr.hareketsizPay, 1e-12)
        assertFalse(xmr.sig)

        val dustMum = mumlar("dust_1m.csv")
        val dust = Sarkac.siglik(dustMum)!!
        assertEquals(2.617724571897488, dust.farkYuzde, 1e-9)
        assertEquals(0.5126666666666667, dust.hareketsizPay, 1e-12)
        assertTrue(dust.sig)
        // Sığ coinin "sarkaç" görünmesi alış-satış zıplamasından: oran çok düşük ama yıldız almaz.
        val oran = Sarkac.oran(kapanis(dustMum))!!
        assertEquals(0.32256327255331, oran, 1e-9)
        assertFalse(Sarkac.yildiz(oran, 0.0, dust))
    }

    @Test
    fun rastgeleYuruyusteOranBireYakin() {
        // Tek seride oran şansla 1'den epey sapabilir; 30 serinin ortalaması 1'e yakın olmalı.
        val oranlar = (1..30).map { tohum ->
            val r = Random(tohum)
            var p = 100.0
            Sarkac.oran(DoubleArray(1500) { p *= exp(r.nextDouble(-1.0, 1.0) * 0.002); p })!!
        }
        val ort = oranlar.average()
        assertTrue("ortalama oran $ort", ort in 0.85..1.15)
    }

    @Test
    fun ortalamayaDonenDusukKacanYuksek() {
        val r = Random(11)
        var x = 0.0
        val sarkac = DoubleArray(1500) { x = 0.9 * x + r.nextDouble(-1.0, 1.0) * 0.002; 100.0 * exp(x) }
        assertTrue(Sarkac.oran(sarkac)!! < Sarkac.SARKAC_SINIRI)
        assertTrue(Sarkac.puan(Sarkac.oran(sarkac)!!) >= 40)

        var g = 0.0
        var p = 100.0
        val kacan = DoubleArray(1500) { g = 0.6 * g + r.nextDouble(-1.0, 1.0) * 0.002; p *= exp(g); p }
        assertTrue(Sarkac.oran(kacan)!! > 1.3)
    }

    @Test
    fun btcBagiZamanaGoreEslesir() {
        val r = Random(3)
        var a = 100.0
        val btc = (0 until 600).map { i -> a *= exp(r.nextDouble(-1.0, 1.0) * 0.003); Mum(i * 60_000L, a, a, a, a) }
        // Coin BTC'nin aynısı ama bazı mumları eksik: eşleşen adımlarda ilişki tam 1 olmalı.
        val coin = btc.filterIndexed { i, _ -> i % 7 != 3 }.map { it.copy(kapanis = it.kapanis * 0.5) }
        assertEquals(1.0, Sarkac.btcBagi(coin, btc)!!, 1e-9)
        // Bağımsız coin: ilişki sıfıra yakın.
        var b = 50.0
        val bagimsiz = btc.map { m -> b *= exp(r.nextDouble(-1.0, 1.0) * 0.003); m.copy(kapanis = b) }
        assertTrue(kotlin.math.abs(Sarkac.btcBagi(bagimsiz, btc)!!) < 0.15)
    }

    @Test
    fun yildizKurali() {
        val likit = Sarkac.Siglik(0.05, 0.01)
        val sig = Sarkac.Siglik(0.5, 0.01)
        assertTrue(Sarkac.yildiz(0.5, 0.2, likit))
        assertFalse(Sarkac.yildiz(0.65, 0.2, likit))   // sarkaç değil
        assertFalse(Sarkac.yildiz(0.5, 0.45, likit))   // BTC'ye bağlı
        assertFalse(Sarkac.yildiz(0.5, 0.2, sig))      // sığ
        assertFalse(Sarkac.yildiz(null, 0.2, likit))   // bilinmiyor
        assertFalse(Sarkac.yildiz(0.5, null, likit))
        assertFalse(Sarkac.yildiz(0.5, 0.2, null))
    }

    @Test
    fun puanVeEksikVeri() {
        assertEquals(50, Sarkac.puan(0.5))
        assertEquals(0, Sarkac.puan(1.0))
        assertEquals(-50, Sarkac.puan(1.5))
        assertEquals(-99, Sarkac.puan(5.0))
        assertNull(Sarkac.oran(DoubleArray(150) { 100.0 + it }))          // 3 × 60 mumdan az
        assertNull(Sarkac.oran(DoubleArray(500) { 100.0 }))               // fiyat hiç oynamamış
        assertNull(Sarkac.oran(DoubleArray(500) { if (it == 10) 0.0 else 100.0 + it % 3 }))
        assertNull(Sarkac.siglik(emptyList()))
        assertNotNull(Sarkac.oran(DoubleArray(500) { 100.0 + it % 3 }))
    }

    @Test
    fun stabilHesabiDegismedi() {
        // TfSonuc'a eklenen alanlar eski hesabı etkilememeli: aynı mumlarla iki kez aynı sonuç ve sarkaç alanları boş.
        val m = mumlar("xmr_5m.csv")
        val a = Analiz.analiz("5m", m, 1.0)
        val b = Analiz.analiz("5m", m, 1.0)
        assertEquals(a.puan, b.puan)
        assertEquals(a.uygun, b.uygun)
        assertNull(a.sarkacOrani)
        assertNull(a.btcBagi)
    }
}
