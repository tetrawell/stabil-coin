package com.stabilcoin.data

import com.stabilcoin.core.Defter
import com.stabilcoin.core.Mum
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

/** MEXC'deki bir USDT paritesi ve son 24 saatlik hacmi (USDT cinsinden). */
data class Parite(val sembol: String, val hacimUsdt: Double, val sonFiyat: Double)

class MexcHatasi(val kod: Int, mesaj: String) : IOException(mesaj)

/**
 * MEXC'nin herkese açık piyasa verisi. Hesap, anahtar ya da şifre gerekmez; sadece okunur.
 */
object MexcIstemci {
    private const val ADRES = "https://api.mexc.com"

    /** Fiyatı zaten sabit olan coinler; taramada vakit kaybettirmesinler. */
    private val SABIT_COINLER = setOf(
        "USDC", "FDUSD", "TUSD", "DAI", "USDE", "USDD", "PYUSD", "BUSD", "USDP", "USD1",
        "EUR", "EURC", "EURT", "USDS", "RLUSD", "USDQ", "USDR",
    )

    /** Uygulamadaki zaman dilimi -> MEXC'nin kullandığı kod. (MEXC "1h" kabul etmiyor, "60m" istiyor.) */
    private val ARALIK_KODU = mapOf(
        "1m" to "1m",
        "5m" to "5m",
        "15m" to "15m",
        "1h" to "60m",
        "4h" to "4h",
        "1d" to "1d",
    )

    private val ARALIK_MS = mapOf(
        "1m" to 60_000L,
        "5m" to 300_000L,
        "15m" to 900_000L,
        "1h" to 3_600_000L,
        "4h" to 14_400_000L,
        "1d" to 86_400_000L,
    )

    /**
     * MEXC bir istekte en fazla 500 mum veriyor. Kaç sayfa (500'lük) çekileceği:
     * 1dk ve 5dk'da 1500 mum (25 saat / 5 gün), diğerlerinde 1000 mum.
     */
    private val SAYFA_SAYISI = mapOf("1m" to 3, "5m" to 3, "15m" to 2, "1h" to 2, "4h" to 2, "1d" to 2)
    private const val SAYFA_BOYU = 500

    /** Defterden kaç basamak alınır (her iki taraf). 1.000 USDT'lik işlemin masrafına fazlasıyla yeter. */
    private const val DEFTER_BASAMAK = 100

    private fun getir(yol: String): String {
        val baglanti = URL(ADRES + yol).openConnection() as HttpURLConnection
        baglanti.connectTimeout = 10_000
        baglanti.readTimeout = 20_000
        baglanti.setRequestProperty("User-Agent", "StabilCoin/1.0")
        try {
            val kod = baglanti.responseCode
            if (kod != 200) {
                val govde = baglanti.errorStream?.bufferedReader()?.use { it.readText() } ?: ""
                throw MexcHatasi(kod, "MEXC $kod: ${govde.take(200)}")
            }
            return baglanti.inputStream.bufferedReader().use { it.readText() }
        } finally {
            baglanti.disconnect()
        }
    }

    /** Yoğunluk (429) ya da geçici sunucu/bağlantı hatasında biraz bekleyip tekrar dener. */
    private suspend fun getirTekrarli(yol: String): String {
        var bekle = 1000L
        repeat(3) {
            try {
                return getir(yol)
            } catch (e: MexcHatasi) {
                if (e.kod == 429 || e.kod >= 500) {
                    delay(bekle); bekle *= 2
                } else {
                    throw e
                }
            } catch (e: IOException) {
                delay(bekle); bekle *= 2
            }
        }
        return getir(yol)
    }

    /** Günlük hacmi sınırın üstündeki USDT pariteleri, hacme göre büyükten küçüğe. */
    suspend fun pariteler(enAzHacimUsdt: Double): List<Parite> {
        val dizi = JSONArray(getirTekrarli("/api/v3/ticker/24hr"))
        val liste = ArrayList<Parite>()
        for (i in 0 until dizi.length()) {
            val o = dizi.optJSONObject(i) ?: continue
            val sembol = o.optString("symbol")
            if (!sembol.endsWith("USDT")) continue
            val taban = sembol.removeSuffix("USDT")
            if (taban.isEmpty() || taban in SABIT_COINLER) continue
            val hacim = o.optString("quoteVolume").toDoubleOrNull() ?: continue
            val fiyat = o.optString("lastPrice").toDoubleOrNull() ?: 0.0
            if (hacim >= enAzHacimUsdt && fiyat > 0) liste.add(Parite(sembol, hacim, fiyat))
        }
        return liste.sortedByDescending { it.hacimUsdt }
    }

    /**
     * Bir paritenin, verilen zaman dilimindeki son mumları (eskiden yeniye).
     * Önce en yeni 500 mum gelir; daha eskiler için başlangıç ve bitiş zamanı birlikte verilir
     * (MEXC sadece bitiş verilince bunu yok sayıp yine en yeni mumları gönderiyor).
     */
    suspend fun mumlar(sembol: String, zamanDilimi: String): List<Mum> {
        val kod = ARALIK_KODU[zamanDilimi] ?: throw IllegalArgumentException("Bilinmeyen zaman dilimi: $zamanDilimi")
        val aralik = ARALIK_MS.getValue(zamanDilimi)
        val sayfa = SAYFA_SAYISI.getValue(zamanDilimi)
        val hepsi = HashMap<Long, Mum>()
        var bitis: Long? = null
        for (k in 0 until sayfa) {
            var yol = "/api/v3/klines?symbol=$sembol&interval=$kod&limit=$SAYFA_BOYU"
            val b = bitis
            if (b != null) {
                val baslangic = b - SAYFA_BOYU * aralik + 1
                yol += "&startTime=$baslangic&endTime=$b"
            }
            val parca = coz(getirTekrarli(yol))
            if (parca.isEmpty()) break
            for (m in parca) hepsi[m.zaman] = m
            bitis = parca.first().zaman - 1
            if (parca.size < SAYFA_BOYU) break
        }
        return hepsi.values.sortedBy { it.zaman }
    }

    /** Alış-satış defterinin anlık görüntüsü (en iyi 100 basamak). */
    suspend fun defter(sembol: String): Defter.Kitap = defterCoz(getirTekrarli("/api/v3/depth?symbol=$sembol&limit=$DEFTER_BASAMAK"))

    /**
     * Coinlerin tek işlem (piyasa emri) komisyonu, % (ör. 0,05). MEXC bunu coin coin farklı alıyor
     * (Ekim 2026: büyük coinlerde %0,05, küçüklerde %0,08, bazılarında %0,2). Bir seferde 50 coin sorulur.
     * Sorulamayan coin haritada olmaz (yerine varsayılan kullanılır).
     */
    suspend fun komisyonlar(semboller: List<String>): Map<String, Double> {
        val sonuc = HashMap<String, Double>()
        for (grup in semboller.distinct().chunked(50)) {
            try {
                sonuc.putAll(komisyonCoz(getirTekrarli("/api/v3/exchangeInfo?symbols=" + grup.joinToString(","))))
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                // bu grup bilinmiyor: varsayılan komisyon kullanılacak
            }
        }
        return sonuc
    }

    internal fun defterCoz(metin: String): Defter.Kitap {
        val o = JSONObject(metin)
        fun liste(ad: String): List<Defter.Basamak> {
            val d = o.optJSONArray(ad) ?: return emptyList()
            val l = ArrayList<Defter.Basamak>(d.length())
            for (i in 0 until d.length()) {
                val b = d.optJSONArray(i) ?: continue
                val f = b.optString(0).toDoubleOrNull() ?: continue
                val m = b.optString(1).toDoubleOrNull() ?: continue
                l.add(Defter.Basamak(f, m))
            }
            return l
        }
        return Defter.Kitap(
            alicilar = liste("bids").sortedByDescending { it.fiyat },
            saticilar = liste("asks").sortedBy { it.fiyat },
        )
    }

    internal fun komisyonCoz(metin: String): Map<String, Double> {
        val d = JSONObject(metin).optJSONArray("symbols") ?: return emptyMap()
        val sonuc = HashMap<String, Double>()
        for (i in 0 until d.length()) {
            val s = d.optJSONObject(i) ?: continue
            val k = s.optString("takerCommission").toDoubleOrNull() ?: continue
            if (k >= 0 && k < 0.05) sonuc[s.optString("symbol")] = k * 100
        }
        return sonuc
    }

    private fun coz(metin: String): List<Mum> {
        val d = JSONArray(metin)
        val sonuc = ArrayList<Mum>(d.length())
        for (i in 0 until d.length()) {
            val k = d.getJSONArray(i)
            sonuc.add(
                Mum(
                    zaman = k.getLong(0),
                    acilis = k.getDouble(1),
                    yuksek = k.getDouble(2),
                    dusuk = k.getDouble(3),
                    kapanis = k.getDouble(4),
                ),
            )
        }
        sonuc.sortBy { it.zaman }
        return sonuc
    }
}
