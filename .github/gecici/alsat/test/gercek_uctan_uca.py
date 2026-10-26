# Gerçek para akışının uçtan uca testi — SAHTE MEXC ile (gerçek borsaya hiç gidilmez).
# 1) Para çekme izinli anahtar reddedilir ve kaydedilmez.  2) Uygun anahtarla bağlanılır.
# 3) Gerçek parayla kumbara açılır (deneme emri dahil).  4) Limit dolmazsa 15 sn sonra piyasa emri.
# 5) Sahte borsadaki bakiye ile kumbaranın kaydı uyuşur.  6) Program kapatılıp açılınca kumbara aynı ilk mumdan sürer.
# 7) "Hepsini sat ve durdur".
# kullanım: python3 -I gercek_uctan_uca.py <calisma_klasoru> <alsat_ikilisi> <proje_kaynak_klasoru>
import sys, os, time, json, subprocess, shutil, urllib.request
from playwright.sync_api import sync_playwright
S, ALSAT, KAYNAK = sys.argv[1], sys.argv[2], sys.argv[3]
PORT = 47410
MEXC = f'http://127.0.0.1:{PORT}'
STRAT = os.path.join(S, 'stratejiler')
VERI = os.path.join(S, 'veri')
ANAHTAR = os.path.join(S, 'anahtar')
EKRAN = os.path.join(S, 'ekran')
for d in (STRAT, VERI, ANAHTAR, EKRAN):
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
sonuc = []

def kontrol(ad, ok, ayrinti=''):
    sonuc.append((ad, ok))
    print(('TAMAM ' if ok else 'HATA  ') + ad + (f' — {ayrinti}' if ayrinti else ''), flush=True)

def mexc(yol, govde=None):
    req = urllib.request.Request(MEXC + yol, data=json.dumps(govde).encode() if govde is not None else None, method='POST' if govde is not None else 'GET')
    return json.loads(urllib.request.urlopen(req, timeout=5).read())

sahte = subprocess.Popen(['python3', '-I', os.path.join(KAYNAK, 'test', 'sahte_mexc.py'), str(PORT), 'mx0testerisim', 'test-gizli-anahtar'])
time.sleep(1)
subprocess.run(['node', os.path.join(KAYNAK, 'test', 'hizli_strateji.js'), os.path.join(KAYNAK, 'web'), STRAT, 'AGTUSDT', '1m', '0.5', '2'], check=True)
env = dict(os.environ, ALSAT_MEXC_ADRES=MEXC, ALSAT_PENCERE_YOK='1', ALSAT_VERI_KLASORU=VERI, ALSAT_KLASOR=STRAT,
           ALSAT_ANAHTAR_KLASORU=ANAHTAR, ALSAT_SILME_YOK='1')
env.pop('ALSAT_TEST', None)

def alsat_ac():
    p = subprocess.Popen([ALSAT], env=env)
    time.sleep(1.5)
    return p

hatalar = []
alsat = alsat_ac()
try:
    with sync_playwright() as pw:
        br = pw.chromium.launch()

        def sayfa():
            pg = br.new_page(viewport={'width': 2560, 'height': 1440})
            pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type == 'error' else None)
            pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
            pg.on('dialog', lambda d: d.accept())
            pg.goto('http://127.0.0.1:47322/')
            pg.wait_for_selector('.skart', timeout=15000)
            pg.wait_for_function('() => window.__alsat.durum()', timeout=10000)
            return pg

        pg = sayfa()
        kumbara = lambda: next((k for k in pg.evaluate('() => window.__alsat.durum()')['kumbaralar'] if k['durum'] != 'bitti'), None)

        # 1) para çekme izinli anahtar
        mexc('/test/ayar', {'izinler': 'SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE,SPOT_WITHDRAW_WRITE', 'limit': 'hic', 'bakiye': {'USDT': 500}})
        pg.click('#mexcCip'); pg.wait_for_selector('#mxErisim')
        pg.fill('#mxErisim', 'mx0testerisim'); pg.fill('#mxGizli', 'test-gizli-anahtar')
        pg.screenshot(path=f'{EKRAN}/01_mexc_form.png')
        pg.click('#mxBaglan')
        pg.wait_for_selector('#mHata:not([hidden])', timeout=15000)
        yazi = pg.inner_text('#mHata')
        pg.screenshot(path=f'{EKRAN}/02_para_cekme_reddi.png')
        kontrol('para çekme izinli anahtar reddedildi', 'KULLANILMADI' in yazi and 'PARA ÇEKME' in yazi, yazi[:120])
        kontrol('reddedilen anahtar kaydedilmedi', not os.path.exists(os.path.join(ANAHTAR, 'mexc_anahtar.dat')))
        kontrol('gizli anahtar kutusu temizlendi', pg.input_value('#mxGizli') == '')

        # 2) uygun anahtar
        mexc('/test/ayar', {'izinler': 'SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE'})
        pg.fill('#mxGizli', 'test-gizli-anahtar')
        pg.click('#mxBaglan')
        pg.wait_for_function('() => window.__alsat.durum().gercek.bagli', timeout=15000)
        time.sleep(0.5)
        pg.screenshot(path=f'{EKRAN}/03_mexc_bagli.png')
        g = pg.evaluate('() => window.__alsat.durum().gercek')
        kontrol('bağlandı, bakiye okundu', g['bagli'] and abs(g['usdt']['serbest'] - 500) < 1e-6, json.dumps(g['usdt']))
        ham = open(os.path.join(ANAHTAR, 'mexc_anahtar.dat'), 'rb').read()
        kontrol('anahtar dosyası düz yazı değil', b'test-gizli-anahtar' not in ham)
        kontrol('sayfaya gizli anahtar gelmiyor', 'test-gizli-anahtar' not in json.dumps(pg.evaluate('() => window.__alsat.durum()')))
        pg.keyboard.press('Escape')

        # 3) gerçek parayla kumbara
        pg.click('.skart[data-ad="AGTUSDT_1m.json"]')
        pg.wait_for_selector('#kumbaraKart [data-eylem="baslatAc"]:not([disabled])', timeout=30000)
        pg.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
        pg.click('#kumbaraKart [data-eylem="baslatAc"]')
        pg.wait_for_selector('#mTutar')
        pg.click('.mod-kart.gercek')
        pg.fill('#mTutar', '20')
        pg.click('#mBaslat')
        pg.wait_for_selector('#mHata:not([hidden])', timeout=5000)
        kontrol('onaysız gerçek başlatma engellendi', 'onay' in pg.inner_text('#mHata'))
        pg.check('#mOnay')
        pg.screenshot(path=f'{EKRAN}/04_gercek_baslat.png')
        pg.click('#mBaslat')
        pg.wait_for_selector('#modal', state='hidden', timeout=20000)
        pg.wait_for_function("() => window.__alsat.durum().kumbaralar.some(k => k.durum !== 'bitti' && k.mod === 'gercek')", timeout=10000)
        kb = kumbara()
        kontrol('gerçek kumbara açıldı', kb['mod'] == 'gercek' and kb['baslangicTutar'] == 20 and kb['ilkMum'] > 0, f"ilkMum {kb['ilkMum']}")
        ilk_mum = kb['ilkMum']

        # 4) ilk alım: limit dolmaz (sahte borsa "hic"), 15 sn sonra piyasa emri
        t0 = time.time()
        while time.time() - t0 < 420:
            kb = kumbara()
            if kb and kb['islemler']:
                break
            time.sleep(2)
        kb = kumbara()
        kontrol('alım yapıldı', bool(kb and kb['islemler']), f'{time.time()-t0:.0f} sn')
        if kb and kb['islemler']:
            a = kb['islemler'][0]['alis']
            kontrol('limit dolmayınca piyasa emri', a['yol'] == 'piyasa' and a['bitis'] - a['baslangic'] >= 15000, f"yol {a['yol']}, {(a['bitis']-a['baslangic'])/1000:.1f} sn")
            pg.screenshot(path=f'{EKRAN}/05_gercek_alim.png')
            d = mexc('/test/durum')
            agt = d['bakiye'].get('AGT', [0, 0])
            kontrol('borsadaki coin = kumbaradaki coin', abs(agt[0] + agt[1] - kb['adet']) < 1e-9, f"borsa {agt}, kumbara {kb['adet']}")
            kontrol('borsadaki USDT harcaması = kumbaranın', abs((500 - d['bakiye']['USDT'][0]) - (20 - kb['nakit'])) < 1e-6, f"borsa {d['bakiye']['USDT']}, kumbara nakit {kb['nakit']}")
            kontrol('emirler istemci kimliğiyle', all(e['clientOrderId'].startswith('as') for e in d['emirler']), ', '.join(e['clientOrderId'] for e in d['emirler']))

        # 5) program kapanıp açılıyor (kumbara coindeyse onunla)
        kb = kumbara()
        coindeydi = bool(kb and kb['islemler'] and not kb['islemler'][-1].get('satis'))
        pg.close()
        alsat.kill(); alsat.wait()
        time.sleep(1)
        alsat = alsat_ac()
        pg = sayfa()
        pg.wait_for_function('() => window.__alsat.izleyiciler().some(i => i.durum === "hazir" && i.sonRapor)', timeout=30000)
        iz = pg.evaluate('() => window.__alsat.izleyiciler()')[0]
        kontrol('yeniden açılınca aynı ilk mumdan yüklendi', iz['ilk'] == ilk_mum, f"ilk {iz['ilk']} beklenen {ilk_mum}")
        g = pg.evaluate('() => window.__alsat.durum().gercek')
        kontrol('kayıtlı anahtarla yeniden bağlandı', g['bagli'])
        kb = kumbara()
        kontrol('kumbara sürüyor', kb is not None and kb['mod'] == 'gercek', f"coindeydi {coindeydi}, işlem {len(kb['islemler']) if kb else '-'}")
        pg.click(f'.skart[data-ad="AGTUSDT_1m.json"]'); time.sleep(2)
        pg.screenshot(path=f'{EKRAN}/06_yeniden_acildi.png')

        # 6) yarım dolan limit: kalan piyasa emriyle (sıradaki alımda)
        mexc('/test/ayar', {'limit': 'yarim'})
        t0 = time.time()
        n0 = len(kumbara()['islemler'])
        yarim = None
        while time.time() - t0 < 420:
            kb = kumbara()
            if kb and len(kb['islemler']) > n0 and kb['islemler'][-1]['alis']:
                yarim = kb['islemler'][-1]['alis']
                break
            time.sleep(2)
        if yarim:
            kontrol('yarım dolan limit + piyasa', yarim['yol'] == 'limit+piyasa', f"yol {yarim['yol']}")
        else:
            kontrol('yarım dolum denenebildi', False, 'süre içinde yeni alım olmadı')

        # 7) hepsini sat ve durdur
        pg.click('#hepsiniDurdur'); pg.wait_for_selector('#modal:not([hidden])')
        pg.click('#modal [data-m="evet"]')
        pg.wait_for_function("() => !window.__alsat.durum().kumbaralar.some(k => k.durum !== 'bitti')", timeout=60000)
        time.sleep(1)
        d = mexc('/test/durum')
        agt = d['bakiye'].get('AGT', [0, 0])
        son = pg.evaluate('() => window.__alsat.durum()')['kumbaralar'][-1]
        kontrol('durdurunca borsada coin kalmadı (toz hariç)', agt[0] + agt[1] < 0.1 * 1 + 1e-9, f'AGT {agt}')
        usdt_fark = d['bakiye']['USDT'][0] - 500
        kontrol('borsadaki USDT farkı = kumbaranın sonucu', abs(usdt_fark - (son['iade'] - 20)) < 1e-6, f"borsa {usdt_fark:+.6f}, kumbara {son['iade'] - 20:+.6f}")
        pg.click('#sekmeler button[data-sekme="kumbara"]'); time.sleep(0.5)
        pg.screenshot(path=f'{EKRAN}/07_bitti.png')
        csv = open(os.path.join(VERI, 'kayitlar', 'islemler.csv'), encoding='utf-8-sig').read()
        kontrol('işlemler.csv yazıldı', csv.count('\n') >= 2, f'{csv.count(chr(10)) - 1} satır')
        br.close()
finally:
    alsat.kill()
    sahte.kill()
print('KONSOL HATALARI:', len(hatalar))
for h in hatalar[:20]:
    print('  ', h)
basarisiz = [a for a, ok in sonuc if not ok]
print('SONUÇ:', 'HEPSİ TAMAM' if not basarisiz else f'{len(basarisiz)} HATA: ' + '; '.join(basarisiz))
