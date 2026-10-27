"""Kripto Al-Sat araştırması (geçici): Binance'in herkese açık geçmiş verisinden (data.binance.vision) akış verisi toplar.

Kullanım: python3 binance_topla.py <çıkış klasörü> <parça no> <parça sayısı>

Her coin için:
  5m/<SPOT>.csv.gz       t,o,h,l,c,v,tbv,n,k,fv,ftbv,fc   (spot 1 dk mumlarından 5 dk; tbv = saldırgan alım (coin),
                         n = işlem sayısı, k = 5 dk içindeki dakika sayısı, fv/ftbv/fc = vadeli hacim/saldırgan alım/kapanış)
  metrics/<SPOT>.csv.gz  t,oi,oiv,top_say_lsr,top_lsr,say_lsr,taker_lsr   (vadeli, 5 dk; açık pozisyon ve oranlar)
  fon/<SPOT>.csv.gz      t,oran   (vadeli fonlama)
  ozet/<SPOT>.json       kapsam (eksik dosyalar, ilk/son zaman)
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
# (spot sembolü, vadeli sembolü)
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
AYLAR = [(2025, m) for m in range(10, 13)] + [(2026, m) for m in range(1, 10)]  # aylık dosyalar
GUNLER_BAS, GUNLER_BIT = dt.date(2026, 10, 1), dt.date(2026, 10, 26)            # Ekim 2026 günlük dosyalar
METRIK_BAS = dt.date(2025, 10, 1)


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
    for yol, satirlar in zip(kline_dosyalari(tur, sembol), havuz.map(getir, kline_dosyalari(tur, sembol))):
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
            dakikalar[t] = (float(p[1]), float(p[2]), float(p[3]), float(p[4]), float(p[5]), float(p[9]), int(float(p[8])))
    return dakikalar


def bes_dk(dakikalar):
    kova = {}
    for t in sorted(dakikalar):
        o, h, l, c, v, tb, n = dakikalar[t]
        b = t - t % 300_000
        x = kova.get(b)
        if x is None:
            kova[b] = [o, h, l, c, v, tb, n, 1]
        else:
            x[1] = max(x[1], h)
            x[2] = min(x[2], l)
            x[3] = c
            x[4] += v
            x[5] += tb
            x[6] += n
            x[7] += 1
    return kova


def yaz_gz(yol, satirlar):
    os.makedirs(os.path.dirname(yol), exist_ok=True)
    with gzip.open(yol, "wt", compresslevel=9) as f:
        f.write("\n".join(satirlar))
        f.write("\n")


def g6(x):
    return f"{x:.6g}"


def coin_isle(cikis, spot, vadeli):
    eksik = []
    ozet = {"spot": spot, "vadeli": vadeli}
    with cf.ThreadPoolExecutor(16) as havuz:
        sd = klines_topla("spot", spot, havuz, eksik)
        if not sd:
            ozet["durum"] = "spot verisi yok"
            return ozet
        fd = klines_topla("futures/um", vadeli, havuz, eksik)
        s5, f5 = bes_dk(sd), bes_dk(fd)
        satirlar = []
        for b in sorted(s5):
            o, h, l, c, v, tb, n, k = s5[b]
            f = f5.get(b)
            fv, ftb, fc = (g6(f[4]), g6(f[5]), repr(f[3])) if f else ("", "", "")
            satirlar.append(f"{b},{o!r},{h!r},{l!r},{c!r},{g6(v)},{g6(tb)},{n},{k},{fv},{ftb},{fc}")
        yaz_gz(f"{cikis}/5m/{spot}.csv.gz", satirlar)
        ozet.update(spot_dakika=len(sd), vadeli_dakika=len(fd), bes_dk=len(satirlar), ilk=min(s5), son=max(s5))
        # vadeli metrikler (günlük dosyalar)
        gunler, g = [], METRIK_BAS
        while g <= GUNLER_BIT:
            gunler.append(g)
            g += dt.timedelta(days=1)
        yollar = [f"futures/um/daily/metrics/{vadeli}/{vadeli}-metrics-{g.isoformat()}.zip" for g in gunler]
        msat = {}
        for yol, satirlar in zip(yollar, havuz.map(getir, yollar)):
            if satirlar is None or satirlar == "HATA":
                eksik.append(yol)
                continue
            for s in satirlar:
                p = s.split(",")
                if len(p) < 8 or not sayi_mi(p[2]):
                    continue
                try:
                    t = int(dt.datetime.strptime(p[0], "%Y-%m-%d %H:%M:%S").replace(tzinfo=dt.timezone.utc).timestamp() * 1000)
                except ValueError:
                    continue
                msat[t] = ",".join([str(t)] + [g6(float(x)) if sayi_mi(x) else "" for x in p[2:8]])
        yaz_gz(f"{cikis}/metrics/{spot}.csv.gz", [msat[t] for t in sorted(msat)])
        ozet["metrik"] = len(msat)
        # fonlama (aylık; Ekim 2026'nın aylık dosyası yoksa eksik kalır)
        yollar = [f"futures/um/monthly/fundingRate/{vadeli}/{vadeli}-fundingRate-{y}-{m:02d}.zip" for y, m in AYLAR + [(2026, 10)]]
        fsat = {}
        for yol, satirlar in zip(yollar, havuz.map(getir, yollar)):
            if satirlar is None or satirlar == "HATA":
                eksik.append(yol)
                continue
            for s in satirlar:
                p = s.split(",")
                if len(p) < 3 or not sayi_mi(p[0]):
                    continue
                fsat[int(p[0])] = f"{int(p[0])},{p[2]}"
        yaz_gz(f"{cikis}/fon/{spot}.csv.gz", [fsat[t] for t in sorted(fsat)])
        ozet["fon"] = len(fsat)
    ozet["eksik"] = eksik
    ozet["durum"] = "tamam"
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
        print(spot, oz.get("durum"), oz.get("bes_dk"), "eksik:", len(oz.get("eksik", [])), f"{time.time() - t0:.0f} sn", flush=True)


if __name__ == "__main__":
    main()
