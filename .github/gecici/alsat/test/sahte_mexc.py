# Sahte MEXC (sadece test).
# Mumlar: /api/v3/klines gerçek MEXC gibi davranır (en fazla 500 mum, endTime hariç, pencere taşarsa startTime
# tarafından). Mumlar açılış zamanına göre sabittir: aynı anda iki program sorarsa aynı veriyi alır. Son (kapanmamış)
# mum, mum süresi içinde yavaş yavaş dolar. /api/v3/ticker/24hr küçük bir liste verir.
# Piyasa: bookTicker, ticker/price, depth, trades, exchangeInfo, time, defaultSymbols (fiyat, coinin en son istenen
# mum süresindeki son mumdan).
# İmzalı uçlar (HMAC SHA256, X-MEXC-APIKEY): apiKeyInfo, account, selfSymbols, tradeFee, order (POST/GET/DELETE),
# order/test, myTrades. Bakiyeler tutulur, emirler dolar, komisyon kesilir.
# Test denetimi: POST /test/ayar {"izinler": "...", "limit": "hemen|yarim|hic|normal", "bakiye": {"USDT": 500}}
#               GET /test/durum  → bakiyeler ve emirler
# kullanım: python3 -I sahte_mexc.py <port> [erisim] [gizli]
import sys, json, time, random, math, threading, hmac, hashlib
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

ARALIK = {'1m': 60000, '5m': 300000, '15m': 900000, '60m': 3600000, '4h': 14400000, '1d': 86400000}
BASLANGIC = int(time.time() * 1000)
ERISIM = sys.argv[2] if len(sys.argv) > 2 else 'mx0testerisim'
GIZLI = sys.argv[3] if len(sys.argv) > 3 else 'test-gizli-anahtar'
seriler = {}
son_aralik = {}
kilit = threading.Lock()
ayar = {'izinler': 'SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE', 'limit': 'normal', 'durum': 'VALID', 'kalan': 66}
bakiye = {'USDT': [500.0, 0.0]}
emirler = {}
sayac = [1000]

def seri(sembol, aralik):
    ms = ARALIK[aralik]
    anahtar = sembol + '|' + aralik
    s = seriler.get(anahtar)
    if s is None:
        bas = (BASLANGIC // ms) * ms - 12000 * ms
        rg = random.Random(anahtar)
        p = 61000.0 if sembol.startswith('BTC') else 0.12 if sembol.startswith('AGT') else 2.5
        s = {'ms': ms, 'bas': bas, 'rg': rg, 'egilim': 0.0, 'o': [], 'h': [], 'l': [], 'c': [p], 'v': []}
        s['c'] = []
        s['son'] = p
        seriler[anahtar] = s
    return s

def uret(s, n):
    while len(s['c']) <= n:
        i = len(s['c'])
        rg = s['rg']
        if i % 150 == 0:
            s['egilim'] = (rg.random() - 0.5) * 0.002
        o = s['son']
        c = o * (1 + s['egilim'] + rg.gauss(0, 0.004))
        h = max(o, c) * (1 + abs(rg.gauss(0, 0.0018)))
        l = min(o, c) * (1 - abs(rg.gauss(0, 0.0018)))
        v = 50 + rg.random() * 950
        s['o'].append(o); s['h'].append(h); s['l'].append(l); s['c'].append(c); s['v'].append(v)
        s['son'] = c

def mum(s, i, simdi):
    t = s['bas'] + i * s['ms']
    o, h, l, c, v = s['o'][i], s['h'][i], s['l'][i], s['c'][i], s['v'][i]
    if t + s['ms'] > simdi:  # kapanmamış mum: geçen süre kadar dolu
        oran = min(1.0, max(0.02, (simdi - t) / s['ms']))
        c2 = o + (c - o) * oran
        h = max(o, c2) + (h - max(o, c)) * oran
        l = min(o, c2) - (min(o, c) - l) * oran
        c, v = c2, v * oran
    fmt = lambda x: repr(round(x, 8))
    return [t, fmt(o), fmt(h), fmt(l), fmt(c), fmt(v), t + s['ms'] - 1, fmt(v * c)]

# ---------- piyasa ----------
def kural(sembol):
    p = 61000.0 if sembol.startswith('BTC') else 0.12 if sembol.startswith('AGT') else 2.5
    if p >= 1000:
        fo, mo = 2, 6
    elif p >= 1:
        fo, mo = 4, 2
    else:
        fo, mo = 5, 1
    return {'symbol': sembol, 'status': '1', 'baseAsset': sembol[:-4], 'quoteAsset': 'USDT', 'isSpotTradingAllowed': True,
            'quotePrecision': fo, 'baseAssetPrecision': mo, 'quoteAssetPrecision': 2, 'baseSizePrecision': str(10 ** -mo if mo else 1),
            'quoteAmountPrecision': '1', 'quoteAmountPrecisionMarket': '1', 'makerCommission': '0', 'takerCommission': '0.0005',
            'orderTypes': ['LIMIT', 'MARKET', 'LIMIT_MAKER']}

def fiyat(sembol, simdi=None):
    simdi = simdi or int(time.time() * 1000)
    aralik = son_aralik.get(sembol, '1m')
    s = seri(sembol, aralik)
    acik = (simdi - s['bas']) // s['ms']
    uret(s, acik)
    m = mum(s, acik, simdi)
    return float(m[4])

def ust(sembol):
    k = kural(sembol)
    tik = 10 ** -k['quotePrecision']
    p = fiyat(sembol)
    aralik = max(2 * tik, p * 0.0003)
    bid = math.floor((p - aralik / 2) / tik) * tik
    ask = bid + max(tik, round(aralik / tik) * tik)
    return round(bid, k['quotePrecision']), round(ask, k['quotePrecision']), k

# ---------- hesap ve emirler ----------
def bk(varlik):
    return bakiye.setdefault(varlik, [0.0, 0.0])

def doldur(e, miktar, f, maker):
    if miktar <= 0:
        return
    k = kural(e['symbol'])
    varlik = k['baseAsset']
    tutar = miktar * f
    oran = 0.0 if maker else 0.0005
    if e['side'] == 'BUY':
        kom, kv = miktar * oran, varlik
        if e['type'] == 'LIMIT':
            bk('USDT')[1] -= tutar  # kilitliden düş
        else:
            bk('USDT')[0] -= tutar
        bk(varlik)[0] += miktar - kom
    else:
        kom, kv = tutar * oran, 'USDT'
        if e['type'] == 'LIMIT':
            bk(varlik)[1] -= miktar
        else:
            bk(varlik)[0] -= miktar
        bk('USDT')[0] += tutar - kom
    e['executedQty'] += miktar
    e['cummulativeQuoteQty'] += tutar
    e['fills'].append({'symbol': e['symbol'], 'id': len(e['fills']) + 1, 'orderId': e['orderId'], 'price': repr(f), 'qty': repr(miktar),
                       'quoteQty': repr(tutar), 'commission': repr(kom), 'commissionAsset': kv, 'time': int(time.time() * 1000),
                       'isBuyer': e['side'] == 'BUY', 'isMaker': maker, 'clientOrderId': e['clientOrderId']})
    e['status'] = 'FILLED' if e['executedQty'] >= e['origQty'] - 1e-12 else 'PARTIALLY_FILLED'

def limit_ilerlet(e):
    # "normal" modda limit emir, karşı taraf fiyatı emir fiyatına gelince dolar
    if e['type'] != 'LIMIT' or e['status'] not in ('NEW', 'PARTIALLY_FILLED') or ayar['limit'] != 'normal':
        return
    bid, ask, k = ust(e['symbol'])
    kalan = e['origQty'] - e['executedQty']
    # alış emri: biri bizim fiyatımızdan satınca (satış fiyatı bizim fiyatımıza inince) dolar; satış emri tersi
    if (e['side'] == 'BUY' and ask <= e['price']) or (e['side'] == 'SELL' and bid >= e['price']):
        doldur(e, kalan, e['price'], True)

def emir_json(e):
    return {'symbol': e['symbol'], 'orderId': e['orderId'], 'clientOrderId': e['clientOrderId'], 'price': repr(e['price']),
            'origQty': repr(e['origQty']), 'executedQty': repr(e['executedQty']), 'cummulativeQuoteQty': repr(e['cummulativeQuoteQty']),
            'status': e['status'], 'type': e['type'], 'side': e['side']}

class Isleyici(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass
    def yaz(self, kod, veri):
        b = json.dumps(veri).encode()
        self.send_response(kod)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(b)))
        self.end_headers()
        self.wfile.write(b)
    def hata(self, kod, mkod, msg):
        self.yaz(kod, {'code': mkod, 'msg': msg})
    def imza_tamam(self, u):
        if self.headers.get('X-MEXC-APIKEY') != ERISIM:
            self.hata(400, 10072, 'Api key info invalid')
            return False
        ham = u.query
        i = ham.rfind('&signature=')
        if i < 0 or not hmac.compare_digest(hmac.new(GIZLI.encode(), ham[:i].encode(), hashlib.sha256).hexdigest(), ham[i + 11:]):
            self.hata(400, 700002, 'Signature for this request is not valid.')
            return False
        q = {k: v[0] for k, v in parse_qs(ham[:i]).items()}
        if abs(int(q.get('timestamp', '0')) - int(time.time() * 1000)) > int(q.get('recvWindow', '5000')):
            self.hata(400, 700003, 'Timestamp for this request is outside of the recvWindow.')
            return False
        return True

    def do_GET(self):
        self.isle('GET')
    def do_POST(self):
        self.isle('POST')
    def do_DELETE(self):
        self.isle('DELETE')

    def isle(self, yontem):
        u = urlparse(self.path)
        q = {k: v[0] for k, v in parse_qs(u.query).items()}
        yol = u.path
        with kilit:
            # ---- test denetimi ----
            if yol == '/test/ayar' and yontem == 'POST':
                n = int(self.headers.get('Content-Length', '0'))
                d = json.loads(self.rfile.read(n) or b'{}')
                for k in ('izinler', 'limit', 'durum', 'kalan'):
                    if k in d:
                        ayar[k] = d[k]
                for v, m in (d.get('bakiye') or {}).items():
                    bk(v)[0] = float(m)
                return self.yaz(200, {'ayar': ayar})
            if yol == '/test/durum':
                return self.yaz(200, {'bakiye': bakiye, 'emirler': [emir_json(e) | {'fills': len(e['fills'])} for e in emirler.values()], 'ayar': ayar})
            # ---- herkese açık ----
            if yol == '/api/v3/time':
                return self.yaz(200, {'serverTime': int(time.time() * 1000)})
            if yol == '/api/v3/ticker/24hr':
                return self.yaz(200, [{'symbol': s, 'quoteVolume': '1000000', 'lastPrice': '1', 'openPrice': '1'} for s in ('BTCUSDT', 'AGTUSDT', 'ETHUSDT')])
            if yol == '/api/v3/exchangeInfo':
                s = q.get('symbol', 'AGTUSDT')
                return self.yaz(200, {'symbols': [kural(s)]})
            if yol == '/api/v3/defaultSymbols':
                return self.yaz(200, {'code': 200, 'data': ['AGTUSDT', 'BTCUSDT', 'ETHUSDT', 'DOGEUSDT'], 'msg': None})
            if yol == '/api/v3/ticker/bookTicker':
                bid, ask, k = ust(q['symbol'])
                return self.yaz(200, {'symbol': q['symbol'], 'bidPrice': repr(bid), 'bidQty': '500', 'askPrice': repr(ask), 'askQty': '500'})
            if yol == '/api/v3/ticker/price':
                return self.yaz(200, {'symbol': q['symbol'], 'price': repr(round(fiyat(q['symbol']), 8))})
            if yol == '/api/v3/depth':
                bid, ask, k = ust(q['symbol'])
                tik = 10 ** -k['quotePrecision']
                n = min(int(q.get('limit', '100')), 50)
                return self.yaz(200, {'bids': [[repr(round(bid - i * tik, k['quotePrecision'])), repr(300 + 150 * i)] for i in range(n)],
                                      'asks': [[repr(round(ask + i * tik, k['quotePrecision'])), repr(300 + 150 * i)] for i in range(n)]})
            if yol == '/api/v3/trades':
                bid, ask, k = ust(q['symbol'])
                simdi = int(time.time() * 1000)
                return self.yaz(200, [{'id': None, 'price': repr(bid if i % 3 == 0 else ask), 'qty': '50', 'quoteQty': '5', 'time': simdi - i * 900,
                                       'isBuyerMaker': i % 3 == 0, 'isBestMatch': True} for i in range(30)])
            if yol == '/api/v3/klines':
                sembol, aralik = q.get('symbol', ''), q.get('interval', '')
                if aralik not in ARALIK or not sembol.endswith('USDT'):
                    return self.hata(400, -1121, 'Invalid symbol.')
                son_aralik[sembol] = aralik
                limit = min(500, int(q.get('limit', '500')))
                simdi = int(time.time() * 1000)
                s = seri(sembol, aralik)
                acik = (simdi - s['bas']) // s['ms']
                uret(s, acik)
                if 'startTime' in q and 'endTime' in q:
                    st, et = int(q['startTime']), int(q['endTime'])
                    i0 = max(0, -(-(st - s['bas']) // s['ms']))
                    i1 = min(acik, (et - 1 - s['bas']) // s['ms'])
                    idx = list(range(i0, i1 + 1))[:limit]
                else:
                    idx = list(range(max(0, acik - limit + 1), acik + 1))
                return self.yaz(200, [mum(s, i, simdi) for i in idx])
            # ---- imzalı ----
            if not yol.startswith('/api/v3/'):
                return self.hata(404, 404, 'yok')
            if not self.imza_tamam(u):
                return
            if yol == '/api/v3/apiKeyInfo':
                return self.yaz(200, {'note': 'test', 'accessKey': q.get('accessKey'), 'createTime': '2026-10-01T10:00:00+03:00',
                                      'permissions': ayar['izinler'], 'remainingValidity': ayar['kalan'], 'ipWhiteList': '', 'status': ayar['durum']})
            if 'SPOT_ACCOUNT_READ' not in ayar['izinler'] and yol in ('/api/v3/account', '/api/v3/selfSymbols', '/api/v3/tradeFee'):
                return self.hata(400, 700007, 'No permission to access the endpoint.')
            if yol == '/api/v3/account':
                return self.yaz(200, {'canTrade': True, 'balances': [{'asset': a, 'free': repr(round(x[0], 10)), 'locked': repr(round(x[1], 10))} for a, x in bakiye.items()]})
            if yol == '/api/v3/selfSymbols':
                return self.yaz(200, {'code': 200, 'data': ['AGTUSDT', 'BTCUSDT', 'ETHUSDT', 'DOGEUSDT'], 'msg': None})
            if yol == '/api/v3/tradeFee':
                return self.yaz(200, {'data': {'makerCommission': 0, 'takerCommission': 0.0005}, 'code': 0, 'msg': 'success', 'timestamp': int(time.time() * 1000)})
            if yol in ('/api/v3/order', '/api/v3/order/test') and 'SPOT_DEAL_WRITE' not in ayar['izinler'] and yontem in ('POST', 'DELETE'):
                return self.hata(400, 700007, 'No permission to access the endpoint.')
            if yol == '/api/v3/order/test':
                return self.yaz(200, {})
            if yol == '/api/v3/order' and yontem == 'POST':
                sembol, taraf, tur = q['symbol'], q['side'], q['type']
                k = kural(sembol)
                varlik = k['baseAsset']
                sayac[0] += 1
                e = {'symbol': sembol, 'orderId': str(sayac[0]), 'clientOrderId': q.get('newClientOrderId', ''), 'side': taraf, 'type': tur,
                     'price': float(q.get('price', '0')), 'origQty': float(q.get('quantity', '0')), 'executedQty': 0.0, 'cummulativeQuoteQty': 0.0,
                     'status': 'NEW', 'fills': []}
                bid, ask, _ = ust(sembol)
                if tur == 'LIMIT':
                    if taraf == 'BUY':
                        gerek = e['price'] * e['origQty']
                        if bk('USDT')[0] < gerek - 1e-9:
                            return self.hata(400, 10101, 'Insufficient balance')
                        bk('USDT')[0] -= gerek; bk('USDT')[1] += gerek
                    else:
                        if bk(varlik)[0] < e['origQty'] - 1e-12:
                            return self.hata(400, 30005, 'Oversold')
                        bk(varlik)[0] -= e['origQty']; bk(varlik)[1] += e['origQty']
                    if ayar['limit'] == 'hemen':
                        doldur(e, e['origQty'], e['price'], True)
                    elif ayar['limit'] == 'yarim':
                        doldur(e, round(e['origQty'] * 0.4, k['baseAssetPrecision']), e['price'], True)
                else:
                    if taraf == 'BUY':
                        tutar = float(q.get('quoteOrderQty', '0'))
                        if bk('USDT')[0] < tutar - 1e-9:
                            return self.hata(400, 10101, 'Insufficient balance')
                        e['origQty'] = tutar / ask
                        doldur(e, e['origQty'], ask, False)
                    else:
                        if bk(varlik)[0] < e['origQty'] - 1e-12:
                            return self.hata(400, 30005, 'Oversold')
                        doldur(e, e['origQty'], bid, False)
                emirler[e['orderId']] = e
                return self.yaz(200, {'symbol': sembol, 'orderId': e['orderId'], 'orderListId': -1, 'price': q.get('price', ''), 'origQty': repr(e['origQty']),
                                      'type': tur, 'side': taraf, 'transactTime': int(time.time() * 1000)})
            if yol == '/api/v3/order' and yontem == 'GET':
                e = emirler.get(q.get('orderId', ''))
                if e is None and q.get('origClientOrderId'):
                    e = next((x for x in emirler.values() if x['clientOrderId'] == q['origClientOrderId']), None)
                if e is None:
                    return self.hata(400, -2013, 'Order does not exist.')
                limit_ilerlet(e)
                return self.yaz(200, emir_json(e))
            if yol == '/api/v3/order' and yontem == 'DELETE':
                e = emirler.get(q.get('orderId', ''))
                if e is None or e['status'] not in ('NEW', 'PARTIALLY_FILLED'):
                    return self.hata(400, -2011, 'Unknown order sent.')
                k = kural(e['symbol'])
                kalan = e['origQty'] - e['executedQty']
                if e['side'] == 'BUY':
                    bos = kalan * e['price']
                    bk('USDT')[1] -= bos; bk('USDT')[0] += bos
                else:
                    bk(k['baseAsset'])[1] -= kalan; bk(k['baseAsset'])[0] += kalan
                e['status'] = 'CANCELED' if e['executedQty'] == 0 else 'PARTIALLY_CANCELED'
                return self.yaz(200, emir_json(e))
            if yol == '/api/v3/myTrades':
                e = emirler.get(q.get('orderId', ''))
                return self.yaz(200, e['fills'] if e else [])
            return self.hata(404, 404, 'yok')

ThreadingHTTPServer(('127.0.0.1', int(sys.argv[1])), Isleyici).serve_forever()
