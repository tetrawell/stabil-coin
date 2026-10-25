"""Kripto Simülatör araştırması için MEXC'den geçmiş mum verisi toplar (sadece herkese açık fiyatlar).

Kullanım: python3 veri_topla.py <çıkış klasörü> <parça no> <parça sayısı>

- 24 saatlik hacme göre USDT paritelerini seçer (sabit coinler hariç).
- Her coin ve mum süresi için MEXC'den 500'erli pencerelerle geriye gider (simülatörün yaptığı gibi:
  startTime + endTime, endTime hariç).
- Dosya: <çıkış>/<aralık>/<SEMBOL>.csv.gz  satırlar: acilis_ms,acilis,yuksek,dusuk,kapanis,hacim,usdt_hacim
"""
import concurrent.futures as cf
import gzip
import json
import os
import sys
import threading
import time
import urllib.error
import urllib.request

ADRES = "https://api.mexc.com"
SABIT = {"USDC", "FDUSD", "TUSD", "DAI", "USDE", "USDD", "PYUSD", "BUSD", "USDP", "USD1", "EUR", "EURC",
         "EURT", "USDS", "RLUSD", "USDQ", "USDR", "USDF", "BRL", "TRY", "GOLD(XAUT)", "GOLD(PAXG)", "XAUT", "PAXG"}
MS = {"1m": 60_000, "5m": 300_000, "15m": 900_000, "1h": 3_600_000, "4h": 14_400_000, "1d": 86_400_000}
KOD = {"1m": "1m", "5m": "5m", "15m": "15m", "1h": "60m", "4h": "4h", "1d": "1d"}
# Her mum süresi için en fazla kaç gün geriye gidilsin ve hangi hacmin üstündeki coinler alınsın
GUN = {"1m": 45, "5m": 180, "15m": 365, "1h": 730, "4h": 1460, "1d": 3000}
HACIM = {"1m": 500_000, "5m": 250_000, "15m": 250_000, "1h": 250_000, "4h": 250_000, "1d": 250_000}
# Kullanıcının kumbaralarında kullandığı coinler (hacmi düşmüş olsa bile alınsın)
KULLANICI = ["BTCUSDT", "ETHUSDT", "MBGUSDT", "BCHUSDT", "ETHFIUSDT", "WLDUSDT", "TRUMPUSDT", "UPCUSDT", "ZROUSDT",
             "INJUSDT", "TIAUSDT", "DOTUSDT", "GRVTUSDT", "GRAMUSDT", "SOLUSDT", "CRVUSDT", "ADAUSDT", "TAOUSDT",
             "FILUSDT", "CTMUSDT", "ARBUSDT", "BDXUSDT", "NEARUSDT", "SUIUSDT", "ENAUSDT", "STRKUSDT", "AXISUSDT",
             "LITUSDT", "PUMPUSDT", "XMRUSDT", "AGTUSDT", "DOGEUSDT", "XRPUSDT", "BNBUSDT"]

_kilit = threading.Lock()
_son = [0.0]
ARALIK_SN = 0.06  # bir işte saniyede en fazla ~16 istek


def bekle_sira():
    with _kilit:
        simdi = time.time()
        bekle = _son[0] + ARALIK_SN - simdi
        if bekle > 0:
            time.sleep(bekle)
        _son[0] = max(simdi, _son[0] + ARALIK_SN)


def getir(yol, deneme=6):
    son = ""
    for k in range(deneme):
        bekle_sira()
        try:
            istek = urllib.request.Request(ADRES + yol, headers={"User-Agent": "KriptoArastirma/1.0"})
            with urllib.request.urlopen(istek, timeout=30) as r:
                return r.status, r.read().decode()
        except urllib.error.HTTPError as e:
            son = e.read().decode(errors="replace")[:300]
            if e.code == 429 or e.code >= 500:
                time.sleep(2 + 3 * k)
                continue
            return e.code, son
        except Exception as e:  # bağlantı hatası
            son = str(e)
            time.sleep(2 + 3 * k)
    return -1, son


def topla(sembol, ar, gun):
    ms = MS[ar]
    simdi = int(time.time() * 1000)
    hedef = simdi - gun * 86_400_000
    hepsi = {}
    imlec = None
    bos = 0
    istek = 0
    while True:
        if imlec is None:
            yol = f"/api/v3/klines?symbol={sembol}&interval={KOD[ar]}&limit=500"
        else:
            bas = imlec - 500 * ms
            yol = f"/api/v3/klines?symbol={sembol}&interval={KOD[ar]}&limit=500&startTime={bas}&endTime={imlec}"
        kod, govde = getir(yol)
        istek += 1
        if kod != 200:
            return sembol, ar, hepsi, f"{kod}: {govde[:150]}", istek
        try:
            parca = json.loads(govde)
        except Exception as e:
            return sembol, ar, hepsi, f"json: {e}", istek
        yeni = [x for x in parca if imlec is None or int(x[0]) < imlec]
        if not yeni:
            bos += 1
            if imlec is None or bos >= 3:
                break
            imlec = imlec - 500 * ms  # boşluk olabilir; bir pencere daha geriye bak
            continue
        bos = 0
        for x in yeni:
            hepsi[int(x[0])] = x
        en_eski = min(int(x[0]) for x in yeni)
        if en_eski <= hedef or istek > 20000:
            break
        imlec = en_eski
    return sembol, ar, hepsi, None, istek


def main(cikis, parca_no, parca_say):
    os.makedirs(cikis, exist_ok=True)
    kod, govde = getir("/api/v3/ticker/24hr")
    if kod != 200:
        print("Ticker alınamadı:", kod, govde)
        sys.exit(1)
    tickerlar = json.loads(govde)
    hacim = {}
    for t in tickerlar:
        s = t.get("symbol", "")
        if not s.endswith("USDT") or len(s) <= 4 or s[:-4] in SABIT:
            continue
        try:
            h, f = float(t.get("quoteVolume") or 0), float(t.get("lastPrice") or 0)
        except ValueError:
            continue
        if f > 0:
            hacim[s] = h
    sirali = sorted(hacim, key=lambda s: -hacim[s])
    if parca_no == 0:
        with open(os.path.join(cikis, "hacimler.json"), "w") as f:
            json.dump({s: hacim[s] for s in sirali}, f, indent=0)
    isler = []
    for ar in MS:
        secilen = [s for s in sirali if hacim[s] >= HACIM[ar]]
        for s in KULLANICI:
            if s in hacim and s not in secilen:
                secilen.append(s)
        for j, s in enumerate(secilen):
            if j % parca_say == parca_no:
                isler.append((s, ar))
    print(f"parça {parca_no}/{parca_say}: {len(isler)} iş", flush=True)
    rapor = []
    bas = time.time()
    with cf.ThreadPoolExecutor(6) as havuz:
        gelecek = [havuz.submit(topla, s, ar, GUN[ar]) for s, ar in isler]
        for g in cf.as_completed(gelecek):
            s, ar, veri, hata, istek = g.result()
            klasor = os.path.join(cikis, ar)
            os.makedirs(klasor, exist_ok=True)
            satirlar = [veri[t] for t in sorted(veri)]
            if satirlar:
                with gzip.open(os.path.join(klasor, s + ".csv.gz"), "wt") as f:
                    for x in satirlar:
                        f.write(f"{int(x[0])},{x[1]},{x[2]},{x[3]},{x[4]},{x[5]},{x[7] if len(x) > 7 else ''}\n")
            ilk = satirlar[0][0] if satirlar else None
            rapor.append({"sembol": s, "aralik": ar, "mum": len(satirlar), "ilk": ilk, "istek": istek, "hata": hata})
            print(f"{s:14s} {ar:4s} {len(satirlar):7d} mum  {istek:4d} istek  {time.time() - bas:6.0f} sn {hata or ''}", flush=True)
    with open(os.path.join(cikis, f"rapor_{parca_no}.json"), "w") as f:
        json.dump(rapor, f, indent=0)


if __name__ == "__main__":
    main(sys.argv[1], int(sys.argv[2]), int(sys.argv[3]))
