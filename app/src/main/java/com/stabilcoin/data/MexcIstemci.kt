package com.stabilcoin.data

import com.stabilcoin.core.Mum
import kotlinx.coroutines.delay
import org.json.JSONArray
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.ConcurrentHashMap

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

    /** Uygulamadaki zaman dilimi -> MEXC'nin kullandığı kod(lar). İlk çalışan kod hatırlanır. */
    private val ARALIK_KODLARI = mapOf(
        "1m" to listOf("1m"),
        "5m" to listOf("5m"),
        "15m" to listOf("15m"),
        "1h" to listOf("60m", "1h"),
        "4h" to listOf("4h"),
        "1d" to listOf("1d"),
    )
    private val calisanKod = ConcurrentHashMap<String, String>()

    @Volatile
    private var mumLimiti = 1000

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

    /** Bir paritenin, verilen zaman dilimindeki son mumları (eskiden yeniye). */
    suspend fun mumlar(sembol: String, zamanDilimi: String): List<Mum> {
        val kodlar = calisanKod[zamanDilimi]?.let { listOf(it) }
            ?: ARALIK_KODLARI[zamanDilimi]
            ?: throw IllegalArgumentException("Bilinmeyen zaman dilimi: $zamanDilimi")
        var sonHata: Exception? = null
        for (kod in kodlar) {
            for (deneme in 0..1) {
                val limit = mumLimiti
                try {
                    val metin = getirTekrarli("/api/v3/klines?symbol=$sembol&interval=$kod&limit=$limit")
                    calisanKod[zamanDilimi] = kod
                    return coz(metin)
                } catch (e: MexcHatasi) {
                    sonHata = e
                    val limitSorunu = e.kod == 400 && limit > 500 &&
                        (e.message?.contains("limit", ignoreCase = true) == true)
                    if (limitSorunu) {
                        mumLimiti = 500
                        continue
                    }
                    break
                }
            }
        }
        throw sonHata ?: IOException("Mum verisi alınamadı")
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
