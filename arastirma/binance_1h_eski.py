"""Kripto Al-Sat araştırması (geçici): B planı hipotezlerini eski yıllarda (2019 – Ekim 2024) sınamak için Binance spot
1 saatlik mumları (herkese açık data.binance.vision). Çıkış: 1h/<SEMBOL>.csv.gz  t,o,h,l,c,v,tbv"""
import concurrent.futures as cf, gzip, io, os, sys, time, urllib.error, urllib.request, zipfile

TABAN = "https://data.binance.vision/data/spot/monthly/klines"
COINLER = ["BTCUSDT", "ETHUSDT", "BNBUSDT", "XRPUSDT", "ADAUSDT", "DOGEUSDT", "LTCUSDT", "LINKUSDT", "TRXUSDT", "XLMUSDT",
           "ATOMUSDT", "BCHUSDT", "ETCUSDT", "EOSUSDT", "DOTUSDT", "SOLUSDT", "AVAXUSDT", "UNIUSDT", "FILUSDT", "NEARUSDT"]
AYLAR = [(y, m) for y in range(2019, 2025) for m in range(1, 13) if (y, m) <= (2024, 10)]

def getir(url, deneme=6):
    for k in range(deneme):
        try:
            with urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "KriptoArastirma/1.0"}), timeout=60) as r:
                veri = r.read()
            with zipfile.ZipFile(io.BytesIO(veri)) as z:
                return z.read(z.namelist()[0]).decode().splitlines()
        except urllib.error.HTTPError as e:
            if e.code == 404:
                return None
            time.sleep(2 + 3 * k)
        except Exception:
            time.sleep(2 + 3 * k)
    return None

def main():
    cikis = sys.argv[1]
    os.makedirs(f"{cikis}/1h", exist_ok=True)
    with cf.ThreadPoolExecutor(16) as havuz:
        for s in COINLER:
            urls = [f"{TABAN}/{s}/1h/{s}-1h-{y}-{m:02d}.zip" for y, m in AYLAR]
            satir = {}
            for sat in havuz.map(getir, urls):
                for x in sat or []:
                    p = x.split(",")
                    if len(p) < 11 or not p[0].isdigit():
                        continue
                    t = int(p[0])
                    if t > 10**14:
                        t //= 1000
                    satir[t] = f"{t},{p[1]},{p[2]},{p[3]},{p[4]},{float(p[5]):.6g},{float(p[9]):.6g}"
            with gzip.open(f"{cikis}/1h/{s}.csv.gz", "wt", compresslevel=9) as f:
                f.write("\n".join(satir[t] for t in sorted(satir)) + "\n")
            print(s, len(satir), flush=True)

if __name__ == "__main__":
    main()
