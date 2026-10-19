"""Test için MEXC'den gerçek veri çeker (uygulamanın kendisi bunu kullanmaz).

Uygulamanın yaptığının aynısını yapar: günlük hacmi 1 milyon $ üstündeki USDT paritelerinin
6 zaman dilimindeki son 1000 mumunu indirir. Ayrıca MEXC'nin zaman dilimi kodlarını ve mum
limitini dener, böylece uygulamanın varsayımları doğrulanır.
"""
import concurrent.futures as cf
import gzip
import json
import os
import sys
import time
import urllib.error
import urllib.request

ADRES = "https://api.mexc.com"
SABIT = {"USDC", "FDUSD", "TUSD", "DAI", "USDE", "USDD", "PYUSD", "BUSD", "USDP", "USD1",
         "EUR", "EURC", "EURT", "USDS", "RLUSD", "USDQ", "USDR"}
DILIMLER = {"1m": "1m", "5m": "5m", "15m": "15m", "1h": "60m", "4h": "4h", "1d": "1d"}
SAYFA = {"1m": 3, "5m": 2, "15m": 2, "1h": 2, "4h": 2, "1d": 2}
ARALIK_MS = {"1m": 60_000, "5m": 300_000, "15m": 900_000, "1h": 3_600_000, "4h": 14_400_000, "1d": 86_400_000}
EN_AZ_HACIM = 1_000_000


def getir(yol, deneme=4):
    son = ""
    for k in range(deneme):
        try:
            istek = urllib.request.Request(ADRES + yol, headers={"User-Agent": "StabilCoin-test/1.0"})
            with urllib.request.urlopen(istek, timeout=25) as r:
                return r.status, r.read().decode()
        except urllib.error.HTTPError as e:
            son = e.read().decode(errors="replace")[:300]
            if e.code == 429 or e.code >= 500:
                time.sleep(1 + 2 * k)
                continue
            return e.code, son
        except Exception as e:  # bağlantı hatası
            son = str(e)
            time.sleep(1 + 2 * k)
    return -1, son


def main(cikis):
    os.makedirs(cikis, exist_ok=True)
    rapor = {"baslangic": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())}

    kod, govde = getir("/api/v3/ticker/24hr")
    rapor["ticker_kod"] = kod
    if kod != 200:
        rapor["ticker_hata"] = govde
        json.dump(rapor, open(os.path.join(cikis, "rapor.json"), "w"), indent=1)
        print("Ticker alınamadı:", kod, govde)
        return
    tickerlar = json.loads(govde)
    with gzip.open(os.path.join(cikis, "ticker.json.gz"), "wt") as f:
        json.dump(tickerlar, f)

    pariteler = []
    for t in tickerlar:
        s = t.get("symbol", "")
        if not s.endswith("USDT"):
            continue
        taban = s[:-4]
        if not taban or taban in SABIT:
            continue
        try:
            hacim = float(t.get("quoteVolume") or 0)
            fiyat = float(t.get("lastPrice") or 0)
        except ValueError:
            continue
        if fiyat > 0:
            pariteler.append((s, hacim))
    pariteler.sort(key=lambda x: -x[1])
    for esik in (250_000, 500_000, 1_000_000, 2_000_000, 5_000_000, 10_000_000):
        rapor[f"hacim_ustu_{esik}"] = sum(1 for _, h in pariteler if h >= esik)
    secilen = [p for p in pariteler if p[1] >= EN_AZ_HACIM]
    rapor["secilen"] = len(secilen)

    # Uygulamanın varsayımlarını dene
    denemeler = {}
    for ad, yol in {
        "60m_1000": "/api/v3/klines?symbol=BTCUSDT&interval=60m&limit=1000",
        "1h_1000": "/api/v3/klines?symbol=BTCUSDT&interval=1h&limit=1000",
        "1m_1000": "/api/v3/klines?symbol=BTCUSDT&interval=1m&limit=1000",
        "1m_1500": "/api/v3/klines?symbol=BTCUSDT&interval=1m&limit=1500",
        "1d_1000": "/api/v3/klines?symbol=BTCUSDT&interval=1d&limit=1000",
    }.items():
        k, g = getir(yol, deneme=2)
        try:
            n = len(json.loads(g)) if k == 200 else None
        except Exception:
            n = None
        denemeler[ad] = {"kod": k, "mum": n, "ornek": g[:200] if k != 200 else ""}
    rapor["denemeler"] = denemeler

    # Sayfalama denemesi: farklı yazımlarla önceki 500 mum geliyor mu?
    k1, g1 = getir("/api/v3/klines?symbol=BTCUSDT&interval=1m&limit=500")
    sayfalama = {}
    if k1 == 200:
        p1 = json.loads(g1)
        ilk = p1[0][0]
        bitis = ilk - 1
        bas = bitis - 500 * 60_000 + 1
        for ad, ek in {
            "sadece_end": f"&endTime={bitis}",
            "sadece_start": f"&startTime={bas}",
            "start_end": f"&startTime={bas}&endTime={bitis}",
            "start_end_sn": f"&startTime={bas // 1000}&endTime={bitis // 1000}",
        }.items():
            k2, g2 = getir("/api/v3/klines?symbol=BTCUSDT&interval=1m&limit=500" + ek)
            try:
                p2 = json.loads(g2) if k2 == 200 else []
            except Exception:
                p2 = []
            sayfalama[ad] = {
                "kod": k2, "mum": len(p2),
                "ilk_fark_dk": ((p2[0][0] - bas) / 60000) if p2 else None,
                "son_fark_dk": ((ilk - p2[-1][0]) / 60000) if p2 else None,
                "hata": g2[:120] if k2 != 200 else "",
            }
    rapor["sayfalama"] = sayfalama

    def is_(parite_dilim):
        s, dilim = parite_dilim
        hepsi = []
        bitis = None
        for _ in range(SAYFA[dilim]):
            yol = f"/api/v3/klines?symbol={s}&interval={DILIMLER[dilim]}&limit=500"
            if bitis is not None:
                baslangic = bitis - 500 * ARALIK_MS[dilim] + 1
                yol += f"&startTime={baslangic}&endTime={bitis}"
            k, g = getir(yol)
            if k != 200:
                return s, dilim, None, f"{k}: {g[:150]}"
            try:
                parca = json.loads(g)
            except Exception as e:
                return s, dilim, None, str(e)
            if not parca:
                break
            hepsi = parca + hepsi
            bitis = parca[0][0] - 1
            if len(parca) < 500:
                break
        tekil = {}
        for m in hepsi:
            tekil[m[0]] = m
        sirali = [tekil[t] for t in sorted(tekil)]
        return s, dilim, sirali, None

    isler = [(s, d) for s, _ in secilen for d in DILIMLER]
    hatalar = []
    satir = 0
    bas = time.time()
    with gzip.open(os.path.join(cikis, "mumlar.csv.gz"), "wt") as f, cf.ThreadPoolExecutor(5) as hav:
        f.write("sembol,dilim,zaman,acilis,yuksek,dusuk,kapanis\n")
        for s, d, veri, hata in hav.map(is_, isler):
            if hata:
                hatalar.append([s, d, hata])
                continue
            for m in veri:
                f.write(f"{s},{d},{m[0]},{m[1]},{m[2]},{m[3]},{m[4]}\n")
                satir += 1
    rapor["sure_sn"] = round(time.time() - bas, 1)
    rapor["istek"] = len(isler)
    rapor["hata_sayisi"] = len(hatalar)
    rapor["hatalar"] = hatalar[:50]
    rapor["satir"] = satir
    with open(os.path.join(cikis, "hacimler.csv"), "w") as f:
        f.write("sembol,hacim_usdt\n")
        for s, h in secilen:
            f.write(f"{s},{h}\n")
    json.dump(rapor, open(os.path.join(cikis, "rapor.json"), "w"), indent=1, ensure_ascii=False)
    print(json.dumps({k: v for k, v in rapor.items() if k != "hatalar"}, indent=1, ensure_ascii=False))


if __name__ == "__main__":
    main(sys.argv[1])
