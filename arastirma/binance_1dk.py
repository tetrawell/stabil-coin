"""Kripto Al-Sat araştırması (geçici): Binance'in herkese açık geçmiş verisinden (data.binance.vision) 1 dakikalık
spot ve vadeli mumları toplar (akış çıkışının 1 dk mumlarda yeniden oynatılması için). Telefon uygulamasıyla ilgisi yok.

Kullanım: python3 binance_1dk.py <çıkış klasörü> <parça no> <parça sayısı>

Her coin için:
  1m/<SPOT>.csv.gz   t,o,h,l,c,v,tbv,fv,ftbv   (spot 1 dk mum; tbv = saldırgan alım (coin); fv/ftbv = vadeli hacim /
                     saldırgan alım, vadelisi yoksa boş)
  ozet/<SPOT>.json   kapsam (eksik dosyalar, ilk/son zaman)
Sadece herkese açık dosyalar indirilir; hiçbir hesap/anahtar kullanılmaz.
"""
import concurrent.futures as cf
import datetime as dt
import gzip
import io
import json
import os
import sys
import time
import urllib.error
import urllib.request
import zipfile

TABAN = "https://data.binance.vision/data"
# (spot sembolü, vadeli sembolü) — 5 dk araştırmasındaki (arastirma-binance) 44 coin
COINLER = [
    ("BTCUSDT", "BTCUSDT"), ("ETHUSDT", "ETHUSDT"), ("SOLUSDT", "SOLUSDT"), ("XRPUSDT", "XRPUSDT"),
    ("BNBUSDT", "BNBUSDT"), ("ADAUSDT", "ADAUSDT"), ("DOGEUSDT", "DOGEUSDT"), ("DOTUSDT", "DOTUSDT"),
    ("BCHUSDT", "BCHUSDT"), ("LTCUSDT", "LTCUSDT"), ("LINKUSDT", "LINKUSDT"), ("AVAXUSDT", "AVAXUSDT"),
    ("SUIUSDT", "SUIUSDT"), ("NEARUSDT", "NEARUSDT"), ("FILUSDT", "FILUSDT"), ("INJUSDT", "INJUSDT"),
    ("ENAUSDT", "ENAUSDT"), ("ETHFIUSDT", "ETHFIUSDT"), ("ZROUSDT", "ZROUSDT"), ("TRUMPUSDT", "TRUMPUSDT"),
    ("TAOUSDT", "TAOUSDT"), ("ZECUSDT", "ZECUSDT"), ("PEPEUSDT", "1000PEPEUSDT"), ("WIFUSDT", "WIFUSDT"),
    ("ARBUSDT", "ARBUSDT"), ("UNIUSDT", "UNIUSDT"), ("TIAUSDT", "TIAUSDT"), ("WLDUSDT", "WLDUSDT"),
    ("ONDOUSDT", "ONDOUSDT"), ("PENDLEUSDT", "PENDLEUSDT"), ("XLMUSDT", "XLMUSDT"), ("TRXUSDT", "TRXUSDT"),
    ("CRVUSDT", "CRVUSDT"), ("ATOMUSDT", "ATOMUSDT"), ("ICPUSDT", "ICPUSDT"), ("STRKUSDT", "STRKUSDT"),
    ("BONKUSDT", "1000BONKUSDT"), ("SHIBUSDT", "1000SHIBUSDT"), ("JUPUSDT", "JUPUSDT"), ("KAITOUSDT", "KAITOUSDT"),
    ("PUMPUSDT", "PUMPUSDT"), ("ASTERUSDT", "ASTERUSDT"), ("XPLUSDT", "XPLUSDT"), ("WLFIUSDT", "WLFIUSDT"),
]
AYLAR = [(2026, 9)]                                                       # aylık dosyalar
GUNLER_BAS, GUNLER_BIT = dt.date(2026, 10, 1), dt.date(2026, 10, 26)      # Ekim 2026 günlük dosyalar


def getir(yol, deneme=6):
    """zip dosyasını indirip içindeki tek CSV'nin satırlarını döndürür; yoksa None."""
    url = f"{TABAN}/{yol}"
    for k in range(deneme):
        try:
            istek = urllib.request.Request(url, headers={"User-Agent": "KriptoArastirma/1.0"})
            with urllib.request.urlopen(istek, timeout=60) as r:
                veri = r.read()
            with zipfile.ZipFile(io.BytesIO(veri)) as z:
                ad = z.namelist()[0]
                return z.read(ad).decode().splitlines()
        except urllib.error.HTTPError as e:
            if e.code == 404:
                return None
            time.sleep(2 + 3 * k)
        except Exception:
            time.sleep(2 + 3 * k)
    print("ALINAMADI", url, flush=True)
    return "HATA"


def sayi_mi(s):
    try:
        float(s)
        return True
    except ValueError:
        return False


def kline_dosyalari(tur, sembol):
    """tur: 'spot' ya da 'futures/um'"""
    yollar = [f"{tur}/monthly/klines/{sembol}/1m/{sembol}-1m-{y}-{m:02d}.zip" for y, m in AYLAR]
    g = GUNLER_BAS
    while g <= GUNLER_BIT:
        yollar.append(f"{tur}/daily/klines/{sembol}/1m/{sembol}-1m-{g.isoformat()}.zip")
        g += dt.timedelta(days=1)
    return yollar


def klines_topla(tur, sembol, havuz, eksik):
    dakikalar = {}
    yollar = kline_dosyalari(tur, sembol)
    for yol, satirlar in zip(yollar, havuz.map(getir, yollar)):
        if satirlar is None or satirlar == "HATA":
            eksik.append(yol + (" (hata)" if satirlar == "HATA" else ""))
            continue
        for s in satirlar:
            p = s.split(",")
            if len(p) < 11 or not sayi_mi(p[0]):
                continue  # başlık satırı
            t = int(p[0])
            if t > 10**14:
                t //= 1000  # 2025'ten sonra spot dosyaları mikrosaniye
            dakikalar[t] = (p[1], p[2], p[3], p[4], float(p[5]), float(p[9]))
    return dakikalar


def yaz_gz(yol, satirlar):
    os.makedirs(os.path.dirname(yol), exist_ok=True)
    with gzip.open(yol, "wt", compresslevel=9) as f:
        f.write("\n".join(satirlar))
        f.write("\n")


def g6(x):
    return f"{x:.6g}"


def fiyat(s):
    """Binance fiyat metni ("0.26770000") → kısa ("0.2677")"""
    return repr(float(s))


def coin_isle(cikis, spot, vadeli):
    eksik = []
    ozet = {"spot": spot, "vadeli": vadeli}
    with cf.ThreadPoolExecutor(16) as havuz:
        sd = klines_topla("spot", spot, havuz, eksik)
        if not sd:
            ozet["durum"] = "spot verisi yok"
            return ozet
        fd = klines_topla("futures/um", vadeli, havuz, eksik)
    satirlar = []
    for t in sorted(sd):
        o, h, l, c, v, tb = sd[t]
        f = fd.get(t)
        fv, ftb = (g6(f[4]), g6(f[5])) if f else ("", "")
        satirlar.append(f"{t},{fiyat(o)},{fiyat(h)},{fiyat(l)},{fiyat(c)},{g6(v)},{g6(tb)},{fv},{ftb}")
    yaz_gz(f"{cikis}/1m/{spot}.csv.gz", satirlar)
    ozet.update(spot_dakika=len(sd), vadeli_dakika=len(fd), ilk=min(sd), son=max(sd), eksik=eksik, durum="tamam")
    return ozet


def main():
    cikis, parca, sayi = sys.argv[1], int(sys.argv[2]), int(sys.argv[3])
    for i, (spot, vadeli) in enumerate(COINLER):
        if i % sayi != parca:
            continue
        t0 = time.time()
        oz = coin_isle(cikis, spot, vadeli)
        os.makedirs(f"{cikis}/ozet", exist_ok=True)
        with open(f"{cikis}/ozet/{spot}.json", "w") as f:
            json.dump(oz, f)
        print(spot, oz.get("durum"), oz.get("spot_dakika"), oz.get("vadeli_dakika"), "eksik:", len(oz.get("eksik", [])),
              f"{time.time() - t0:.0f} sn", flush=True)


if __name__ == "__main__":
    main()
