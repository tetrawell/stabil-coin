# Geçici: MEXC canlı veri örnekleri (Kripto Al-Sat 0.9 "Akış onayı" geliştirmesi için test verisi). İş bitince silinecek.
import asyncio, base64, json, os, time, urllib.request
import websockets

OUT = 'pi_veri'
os.makedirs(OUT, exist_ok=True)
SURE = 90

def get(url):
    try:
        with urllib.request.urlopen(url, timeout=30) as r:
            return {'durum': r.status, 'govde': json.load(r)}
    except Exception as e:
        return {'hata': str(e)[:300]}

async def spot():
    kayit = []
    kanallar = ['spot@public.aggre.deals.v3.api.pb@100ms@BTCUSDT', 'spot@public.aggre.deals.v3.api.pb@100ms@DOSUSDT',
                'spot@public.limit.depth.v3.api.pb@BTCUSDT@20', 'spot@public.aggre.bookTicker.v3.api.pb@100ms@BTCUSDT']
    async with websockets.connect('wss://wbs-api.mexc.com/ws', max_size=None, ping_interval=None) as ws:
        await ws.send(json.dumps({'method': 'SUBSCRIPTION', 'params': kanallar}))
        bas = time.time(); son_ping = bas
        while time.time() - bas < SURE:
            if time.time() - son_ping > 15:
                await ws.send(json.dumps({'method': 'PING'})); son_ping = time.time()
                kayit.append({'z': int(time.time() * 1000), 'giden': 'PING'})
            try:
                m = await asyncio.wait_for(ws.recv(), timeout=2)
            except asyncio.TimeoutError:
                continue
            if isinstance(m, bytes):
                kayit.append({'z': int(time.time() * 1000), 'b': base64.b64encode(m).decode()})
            else:
                kayit.append({'z': int(time.time() * 1000), 't': m})
    json.dump(kayit, open(f'{OUT}/spot_ws.json', 'w'))
    print('spot', len(kayit), flush=True)

async def vadeli():
    kayit = []
    async with websockets.connect('wss://contract.mexc.com/edge', max_size=None, ping_interval=None) as ws:
        for m in ({'method': 'sub.ticker', 'param': {'symbol': 'BTC_USDT'}}, {'method': 'sub.deal', 'param': {'symbol': 'BTC_USDT'}},
                  {'method': 'sub.ticker', 'param': {'symbol': 'DOS_USDT'}}, {'method': 'sub.deal', 'param': {'symbol': 'DOS_USDT'}}):
            await ws.send(json.dumps(m))
        bas = time.time(); son_ping = bas
        while time.time() - bas < SURE:
            if time.time() - son_ping > 15:
                await ws.send(json.dumps({'method': 'ping'})); son_ping = time.time()
                kayit.append({'z': int(time.time() * 1000), 'giden': 'ping'})
            try:
                m = await asyncio.wait_for(ws.recv(), timeout=2)
            except asyncio.TimeoutError:
                continue
            if isinstance(m, bytes):
                kayit.append({'z': int(time.time() * 1000), 'b': base64.b64encode(m).decode()})
            else:
                kayit.append({'z': int(time.time() * 1000), 't': m})
    json.dump(kayit, open(f'{OUT}/vadeli_ws.json', 'w'))
    print('vadeli', len(kayit), flush=True)

async def main():
    await asyncio.gather(spot(), vadeli())

asyncio.run(main())
simdi = int(time.time() * 1000)
rest = {
    'aggTrades_son': get('https://api.mexc.com/api/v3/aggTrades?symbol=BTCUSDT&limit=1000'),
    'aggTrades_aralik': get(f'https://api.mexc.com/api/v3/aggTrades?symbol=DOSUSDT&startTime={simdi - 3600_000}&endTime={simdi}&limit=1000'),
    'aggTrades_2saat': get(f'https://api.mexc.com/api/v3/aggTrades?symbol=DOSUSDT&startTime={simdi - 7200_000}&endTime={simdi}&limit=1000'),
    'trades': get('https://api.mexc.com/api/v3/trades?symbol=BTCUSDT&limit=20'),
    'v_ticker_btc': get('https://contract.mexc.com/api/v1/contract/ticker?symbol=BTC_USDT'),
    'v_detay_btc': get('https://contract.mexc.com/api/v1/contract/detail?symbol=BTC_USDT'),
    'v_deals_btc': get('https://contract.mexc.com/api/v1/contract/deals/BTC_USDT?limit=20'),
    'v_ticker_dos': get('https://contract.mexc.com/api/v1/contract/ticker?symbol=DOS_USDT'),
}
tum = get('https://contract.mexc.com/api/v1/contract/detail')
if 'govde' in tum and isinstance(tum['govde'].get('data'), list):
    rest['v_liste'] = [{k: x.get(k) for k in ('symbol', 'contractSize', 'priceUnit', 'state', 'quoteCoin', 'settleCoin', 'baseCoin')} for x in tum['govde']['data']]
else:
    rest['v_liste'] = tum
json.dump(rest, open(f'{OUT}/rest.json', 'w'))
print('rest bitti', flush=True)
