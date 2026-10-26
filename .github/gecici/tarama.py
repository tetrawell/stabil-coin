# Geçici: bütün MEXC USDT coinleri için sarkaç ölçüleri + alış-satış defteri maliyeti (değerlendirme sonrası silinecek).
import json, math, os, sys, time, gzip, threading, urllib.request, urllib.error
from concurrent.futures import ThreadPoolExecutor

B = 'https://api.mexc.com/api/v3/'
SABIT = {"USDC", "FDUSD", "TUSD", "DAI", "USDE", "USDD", "PYUSD", "BUSD", "USDP", "USD1", "EUR", "EURC", "EURT", "USDS", "RLUSD", "USDQ", "USDR"}
KOD = {'1m': ('1m', 60000, 3), '5m': ('5m', 300000, 3), '15m': ('15m', 900000, 2)}
kilit = threading.Lock()
say = [0]

def get(p):
    bekle = 1.0
    for i in range(5):
        try:
            with urllib.request.urlopen(B + p, timeout=30) as r:
                with kilit: say[0] += 1
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 429 or e.code >= 500: time.sleep(bekle); bekle *= 2; continue
            return None
        except Exception:
            time.sleep(bekle); bekle *= 2
    return None

def mumlar(s, tf):
    kod, ms, sayfa = KOD[tf]
    hepsi, bitis = {}, None
    for _ in range(sayfa):
        yol = f'klines?symbol={s}&interval={kod}&limit=500'
        if bitis is not None: yol += f'&startTime={bitis - 500 * ms + 1}&endTime={bitis}'
        r = get(yol)
        if not r: break
        for k in r: hepsi[int(k[0])] = (int(k[0]), float(k[1]), float(k[2]), float(k[3]), float(k[4]))
        bitis = int(r[0][0]) - 1
        if len(r) < 500: break
    return [hepsi[t] for t in sorted(hepsi)]

def varyans(x):
    m = sum(x) / len(x); return sum((v - m) ** 2 for v in x) / len(x)

def oran(c, k=60):
    n = len(c)
    if n <= 3 * k or any(v <= 0 for v in c): return None
    lg = [math.log(v) for v in c]
    kisa = [lg[i + 1] - lg[i] for i in range(n - 1)]; uzun = [lg[i + k] - lg[i] for i in range(n - k)]
    vk = varyans(kisa)
    return varyans(uzun) / (k * vk) if vk > 0 else None

def btc_bagi(m, btc):
    bm = {x[0]: x[4] for x in btc}; x, y = [], []
    for i in range(1, len(m)):
        a0, a1 = m[i - 1][4], m[i][4]; b0, b1 = bm.get(m[i - 1][0]), bm.get(m[i][0])
        if b0 and b1 and a0 > 0 and a1 > 0: x.append(math.log(a1 / a0)); y.append(math.log(b1 / b0))
    if len(x) < 100: return None
    mx, my = sum(x) / len(x), sum(y) / len(y)
    sxy = sum((a - mx) * (b - my) for a, b in zip(x, y)); sxx = sum((a - mx) ** 2 for a in x); syy = sum((b - my) ** 2 for b in y)
    return sxy / math.sqrt(sxx * syy) if sxx > 0 and syy > 0 else None

def siglik(m):
    if len(m) < 200: return None
    hareketsiz = sum(1 for x in m if x[2] == x[3]) / len(m)
    d = [math.log(m[i][4] / m[i - 1][4]) for i in range(1, len(m)) if m[i][4] > 0 and m[i - 1][4] > 0]
    o = sum(d) / len(d)
    kov = sum((d[i] - o) * (d[i - 1] - o) for i in range(1, len(d))) / (len(d) - 1)
    return {'fark': 2 * math.sqrt(-kov) * 100 if kov < 0 else 0.0, 'hareketsiz': hareketsiz}

def ortanca(x):
    s = sorted(x); n = len(s)
    return None if n == 0 else (s[n // 2] if n % 2 else (s[n // 2 - 1] + s[n // 2]) / 2)

def salinimlar(m):
    c = [x[4] for x in m]; n = len(c)
    if n < 120: return {}
    bb = []
    for i in range(20, n + 1):
        w = c[i - 20:i]; mu = sum(w) / 20
        if mu > 0: bb.append(2 * math.sqrt(varyans(w)) / mu * 100)
    rng = []
    for i in range(0, n - 59, 60):
        hi = max(x[2] for x in m[i:i + 60]); lo = min(x[3] for x in m[i:i + 60])
        if lo > 0: rng.append((hi - lo) / lo * 100)
    mum = [(x[2] - x[3]) / x[3] * 100 for x in m if x[3] > 0]
    return {'bb': ortanca(bb), 'rng60': ortanca(rng), 'mum': ortanca(mum)}

def defter(d, tutarlar=(50, 100, 200)):
    if not d or not d.get('bids') or not d.get('asks'): return None
    bids = [(float(p), float(q)) for p, q in d['bids']]; asks = [(float(p), float(q)) for p, q in d['asks']]
    b0, a0 = bids[0][0], asks[0][0]; orta = (a0 + b0) / 2
    out = {'fark': 100 * (a0 / b0 - 1), 'derin05': sum(p * q for p, q in bids if p >= orta * 0.995),
           'derin05al': sum(p * q for p, q in asks if p <= orta * 1.005)}
    for t in tutarlar:
        for taraf, liste in (('al', asks), ('sat', bids)):
            kalan, adet, para = t, 0.0, 0.0
            for p, q in liste:
                u = min(kalan, p * q); para += u; adet += u / p; kalan -= u
                if kalan <= 1e-9: break
            out[f'{taraf}{t}'] = None if kalan > 1e-9 else 100 * abs((para / adet) / orta - 1)
    return out

def main():
    t24 = get('ticker/24hr') or []
    coinler = []
    for o in t24:
        s = o.get('symbol', '')
        if not s.endswith('USDT') or s[:-4] in SABIT or not s[:-4]: continue
        try: h = float(o['quoteVolume']); f = float(o['lastPrice'])
        except Exception: continue
        if h >= 1_000_000 and f > 0: coinler.append((s, h))
    bilgi = get('exchangeInfo') or {}
    taker = {x['symbol']: float(x.get('takerCommission') or 'nan') for x in bilgi.get('symbols', [])}
    print('coin', len(coinler), flush=True)
    btc = {tf: mumlar('BTCUSDT', tf) for tf in KOD}

    def isle(sh):
        s, h = sh
        try:
            m = {tf: mumlar(s, tf) for tf in KOD}
            d = defter(get(f'depth?symbol={s}&limit=100'))
            r = {'s': s, 'hacim': h, 'taker': taker.get(s), 'defter': d, 'siglik': siglik(m['1m']), 'tf': {}}
            for tf in KOD:
                c = [x[4] for x in m[tf]]
                r['tf'][tf] = {'n': len(c), 'oran': oran(c), 'btc': btc_bagi(m[tf], btc[tf]), **salinimlar(m[tf])}
            return r
        except Exception as e:
            return {'s': s, 'hata': str(e)[:100]}

    with ThreadPoolExecutor(4) as ex:
        out = list(ex.map(isle, coinler))
    os.makedirs('pi_veri', exist_ok=True)
    with gzip.open('pi_veri/tarama.json.gz', 'wt') as f: json.dump({'zaman': int(time.time() * 1000), 'coinler': out}, f)
    print('istek', say[0], 'bitti', len(out), flush=True)

main()
