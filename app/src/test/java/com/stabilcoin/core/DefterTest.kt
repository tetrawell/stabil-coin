package com.stabilcoin.core

import com.stabilcoin.data.MexcIstemci
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Defter (alıp satma masrafı) ve salınım testleri. Gerçek MEXC defter görüntüleri (26 Ekim 2026, test/resources) ve
 * beklenen değerler Python ile hesaplandı (scratchpad/fikstur_defter.py); uygulama aynı sonucu vermeli.
 */
class DefterTest {
    private fun oku(dosya: String): String =
        (javaClass.classLoader!!.getResourceAsStream(dosya) ?: error("$dosya yok")).bufferedReader().readText()

    private val beklenen by lazy { JSONObject(oku("defter_beklenen.json")) }

    private fun kapanis(dosya: String): DoubleArray =
        oku(dosya).lines().filter { it.isNotBlank() }.map { it.split(',')[4].toDouble() }.toDoubleArray()

    @Test
    fun gercekDefterlerdePythonIleAyni() {
        for (ad in listOf("bdx", "near", "axis")) {
            val kitap = MexcIstemci.defterCoz(oku("defter_$ad.json"))
            assertEquals(100, kitap.alicilar.size)
            assertEquals(100, kitap.saticilar.size)
            val b = beklenen.getJSONObject(ad)
            for (t in listOf(50, 100, 200, 5000)) {
                val m = Defter.masraf(kitap, t.toDouble(), 0.08)!!
                assertEquals("$ad fark", b.getDouble("fark"), m.farkYuzde, 1e-9)
                assertEquals("$ad al$t", b.getDouble("al$t"), m.alisYuzde!!, 1e-9)
                assertEquals("$ad sat$t", b.getDouble("sat$t"), m.satisYuzde!!, 1e-9)
                assertEquals(b.getDouble("al$t") + b.getDouble("sat$t") + 0.16, m.toplamYuzde!!, 1e-9)
            }
        }
    }

    @Test
    fun inceDefterdeMasrafTutarlaBuyur() {
        val bdx = MexcIstemci.defterCoz(oku("defter_bdx.json"))
        val near = MexcIstemci.defterCoz(oku("defter_near.json"))
        val b50 = Defter.masraf(bdx, 50.0, 0.08)!!.toplamYuzde!!
        val b200 = Defter.masraf(bdx, 200.0, 0.08)!!.toplamYuzde!!
        assertTrue("BDX: 200 USDT 50'den pahalı", b200 > b50 + 0.3)
        // NEAR'ın defteri kalın: 50 ile 200 USDT aynı masraf.
        assertEquals(Defter.masraf(near, 50.0, 0.05)!!.toplamYuzde!!, Defter.masraf(near, 200.0, 0.05)!!.toplamYuzde!!, 1e-12)
    }

    @Test
    fun komisyonlarExchangeInfodan() {
        val k = MexcIstemci.komisyonCoz(oku("exchangeinfo.json"))
        val b = beklenen.getJSONObject("komisyon")
        assertEquals(3, k.size)
        for (s in listOf("BDXUSDT", "NEARUSDT", "ADIUSDT")) assertEquals(s, b.getDouble(s), k[s]!!, 1e-12)
    }

    @Test
    fun salinimPythonIleAyni() {
        assertEquals(beklenen.getDouble("salinim_xmr_5m"), Defter.salinim(kapanis("xmr_5m.csv"))!!, 1e-9)
        assertEquals(beklenen.getDouble("salinim_dust_1m"), Defter.salinim(kapanis("dust_1m.csv"))!!, 1e-9)
        assertNull(Defter.salinim(DoubleArray(119) { 100.0 + it % 3 }))
        assertEquals(0.0, Defter.salinim(DoubleArray(500) { 100.0 })!!, 0.0)
    }

    @Test
    fun defterYetmezseBilinmez() {
        val kitap = Defter.Kitap(
            alicilar = listOf(Defter.Basamak(9.9, 1.0), Defter.Basamak(9.8, 1.0)),
            saticilar = listOf(Defter.Basamak(10.1, 1.0), Defter.Basamak(10.2, 1.0)),
        )
        val az = Defter.masraf(kitap, 15.0, 0.05)!!
        // 15 USDT: 10,1'den 1 adet (10,1 USDT) + 10,2'den 4,9 USDT. Ortalama fiyat = 15 / (1 + 4,9/10,2)
        val ort = 15.0 / (1.0 + 4.9 / 10.2)
        assertEquals((ort / 10.0 - 1) * 100, az.alisYuzde!!, 1e-12)
        assertEquals((10.1 / 9.9 - 1) * 100, az.farkYuzde, 1e-12)
        val cok = Defter.masraf(kitap, 50.0, 0.05)!!
        assertNull(cok.alisYuzde)
        assertNull(cok.toplamYuzde)
        assertNull(Defter.kat(1.0, cok))
        assertFalse(Defter.yeter(Defter.kat(1.0, cok)))
        assertNull(Defter.masraf(Defter.Kitap(emptyList(), kitap.saticilar), 10.0, 0.05))
        // Ters (bozuk) defter
        assertNull(Defter.masraf(Defter.Kitap(listOf(Defter.Basamak(11.0, 1.0)), listOf(Defter.Basamak(10.0, 1.0))), 5.0, 0.05))
    }

    @Test
    fun katVeYildizSiniri() {
        val m = Defter.Masraf(100.0, 0.1, 0.05, 0.05, 0.05) // toplam %0,2
        assertEquals(0.2, m.toplamYuzde!!, 1e-12)
        assertEquals(3.0, Defter.kat(0.6, m)!!, 1e-12)
        assertTrue(Defter.yeter(Defter.kat(0.6, m)))
        assertFalse(Defter.yeter(Defter.kat(0.59, m)))
        assertNull(Defter.kat(null, m))
        assertNull(Defter.kat(0.6, null))
        assertNotNull(Defter.kat(0.0, m))
    }
}
