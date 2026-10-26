# Birden çok kumbara, sayfa yenileme, ömür sonu ve süresi dolan dosyanın silinmesi (test verisiyle, sanal para).
# Kendi çalışma klasörünü kurar ve programı ALSAT_TEST=1 ile kendisi açar. 1 dk'lık ömür 6 dakikaya indirilir.
# kullanım: python3 -I coklu_kumbara_testi.py <calisma_klasoru> <alsat_ikilisi> <proje_kaynak_klasoru>
import sys, os, time, json, subprocess, shutil
from playwright.sync_api import sync_playwright
S, ALSAT, KAYNAK = sys.argv[1], sys.argv[2], sys.argv[3]
STRAT, VERI, EKRAN = os.path.join(S, 'stratejiler'), os.path.join(S, 'veri'), os.path.join(S, 'ekran')
for d in (STRAT, VERI, EKRAN):
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
sonuc = []

def kontrol(ad, ok, ayrinti=''):
    sonuc.append((ad, ok))
    print(('TAMAM ' if ok else 'HATA  ') + ad + (f' — {ayrinti}' if ayrinti else ''), flush=True)

yaz = lambda sembol, once: subprocess.run(['node', os.path.join(KAYNAK, 'test', 'hizli_strateji.js'), os.path.join(KAYNAK, 'web'), STRAT, sembol, '1m', '0.25', str(once)], check=True, capture_output=True)
yaz('DOGEUSDT', '0')
yaz('SHIBUSDT', '0')
yaz('EXPUSDT', '5.5')  # 6 dk ömürlü: yarım dakika sonra dolar, sonra klasörden silinir
json.dump({'omur': {'1m': 6}}, open(os.path.join(VERI, 'ayarlar.json'), 'w'))
env = dict(os.environ, ALSAT_TEST='1', ALSAT_PENCERE_YOK='1', ALSAT_VERI_KLASORU=VERI, ALSAT_KLASOR=STRAT, ALSAT_ANAHTAR_KLASORU=os.path.join(S, 'anahtar'))
env.pop('ALSAT_SILME_YOK', None)
alsat = subprocess.Popen([ALSAT], env=env)
time.sleep(1.5)
hatalar = []
try:
    with sync_playwright() as pw:
        br = pw.chromium.launch()
        pg = br.new_page(viewport={'width': 2560, 'height': 1440})
        pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type == 'error' else None)
        pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
        pg.on('dialog', lambda d: d.accept())
        pg.goto('http://127.0.0.1:47322/')
        pg.wait_for_selector('.skart', timeout=15000)
        durum = lambda: pg.evaluate('() => window.__alsat.durum()')
        aktifler = lambda: [k for k in durum()['kumbaralar'] if k['durum'] != 'bitti']

        def baslat(ad, tutar):
            pg.click(f'.skart[data-ad="{ad}"]')
            pg.wait_for_selector('#kumbaraKart [data-eylem="baslatAc"]:not([disabled])', timeout=30000)
            pg.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
            pg.click('#kumbaraKart [data-eylem="baslatAc"]')
            pg.wait_for_selector('#mTutar')
            pg.fill('#mTutar', str(tutar))
            pg.click('#mBaslat')
            pg.wait_for_selector('#modal', state='hidden', timeout=15000)
            pg.wait_for_function(f"() => window.__alsat.durum().kumbaralar.some(k => k.durum !== 'bitti' && k.dosya === '{ad}')", timeout=10000)

        baslat('DOGEUSDT_1m.json', 100)
        baslat('SHIBUSDT_1m.json', 50)
        ks = aktifler()
        kontrol('iki kumbara açıldı', len(ks) == 2, ', '.join(f"{k['dosya']} {k['baslangicTutar']}" for k in ks))
        kontrol('sanal cüzdandan 150 ayrıldı', abs(durum()['sanal']['serbest'] - 850) < 1e-6, str(durum()['sanal']))
        capalar = {k['id']: k['ilkMum'] for k in ks}
        # başka bir strateji seçiliyken kumbaralar arka planda karar vermeye devam eder
        pg.click('.skart[data-ad="EXPUSDT_1m.json"]')
        t0 = time.time()
        while time.time() - t0 < 200:
            iz = pg.evaluate('() => window.__alsat.izleyiciler()')
            if len(iz) == 2 and all(i['sonRapor'] and i['sonRapor']['zaman'] > t0 * 1000 for i in iz):
                break
            time.sleep(3)
        iz = pg.evaluate('() => window.__alsat.izleyiciler()')
        kontrol('arka plandaki kumbaralar karar bildiriyor', len(iz) == 2 and all(i['sonRapor'] and i['sonRapor']['zaman'] > t0 * 1000 for i in iz),
                json.dumps([(i['dosya'], i['sonRapor']) for i in iz], ensure_ascii=False))
        pg.screenshot(path=f'{EKRAN}/01_iki_kumbara.png')
        # sayfa yenilenince izleyiciler aynı ilk mumdan kurulur
        pg.reload()
        pg.wait_for_selector('.skart', timeout=15000)
        pg.wait_for_function('() => window.__alsat.izleyiciler().length === 2 && window.__alsat.izleyiciler().every(i => i.durum === "hazir")', timeout=30000)
        iz = pg.evaluate('() => window.__alsat.izleyiciler()')
        kontrol('yenilemeden sonra aynı ilk mumlar', all(capalar.get(i['id']) == i['ilk'] for i in iz), json.dumps([(i['ilk'], capalar.get(i['id'])) for i in iz]))
        pg.click('.skart[data-ad="DOGEUSDT_1m.json"]'); time.sleep(2)
        g = pg.evaluate('() => window.__alsat.gorunen()')
        kontrol('grafik kumbaranın izleyicisini gösteriyor', g and g['kumbaraId'] is not None and g['ilk'] == capalar.get(g['kumbaraId']), json.dumps(g))
        pg.screenshot(path=f'{EKRAN}/02_yenilendi_doge.png')
        # süresi dolan dosya silinir ve arşivlenir
        t0 = time.time()
        while time.time() - t0 < 120 and os.path.exists(os.path.join(STRAT, 'EXPUSDT_1m.json')):
            time.sleep(3)
        kontrol('süresi dolan dosya silindi', not os.path.exists(os.path.join(STRAT, 'EXPUSDT_1m.json')), f'{time.time()-t0:.0f} sn')
        arsiv = os.listdir(os.path.join(VERI, 'kayitlar', 'arsiv')) if os.path.isdir(os.path.join(VERI, 'kayitlar', 'arsiv')) else []
        kontrol('arşive kopyalandı', any(a.startswith('EXPUSDT_1m_') for a in arsiv), ', '.join(arsiv))
        time.sleep(6)
        kontrol('listeden kalktı', pg.query_selector('.skart[data-ad="EXPUSDT_1m.json"]') is None)
        # ömür sonu: yeni alım yok, elde coin varsa en geç ek sürede satılır, kumbaralar kapanır
        t0 = time.time()
        while time.time() - t0 < 600 and aktifler():
            time.sleep(5)
        d = durum()
        bitenler = [k for k in d['kumbaralar'] if k['durum'] == 'bitti']
        kontrol('ömür sonunda iki kumbara da kapandı', len(bitenler) == 2 and not aktifler(), ', '.join(f"{k['dosya']}: {k.get('bitisSebebi')}" for k in bitenler))
        for k in bitenler:
            # alım ömür bitmeden başlamış olmalı (dolumu birkaç saniye sonra tamamlanabilir)
            son_al = max([t['alis']['baslangic'] for t in k['islemler'] if t.get('alis')] or [0])
            kontrol(f"{k['dosya']}: ömür dolduktan sonra alım başlamadı", son_al <= k['bitis'], f"son alım başlangıcı {son_al}, ömür sonu {k['bitis']}, {len(k['islemler'])} işlem")
            kontrol(f"{k['dosya']}: coin kalmadı", k['adet'] * k['sonFiyat'] < 1, f"adet {k['adet']}")
        beklenen = 1000 - 150 + sum(k.get('iade', 0) for k in bitenler)
        kontrol('para sanal cüzdana döndü', abs(d['sanal']['serbest'] - beklenen) < 1e-6, f"serbest {d['sanal']['serbest']:.4f}, beklenen {beklenen:.4f}")
        pg.click('#sekmeler button[data-sekme="olaylar"]'); time.sleep(0.5)
        pg.screenshot(path=f'{EKRAN}/03_omur_bitti.png')
        for k in bitenler:
            print('  ', k['dosya'], [o['mesaj'][:90] for o in k['olaylar'][-4:]])
        br.close()
finally:
    alsat.kill()
print('KONSOL HATALARI:', len(hatalar))
for h in hatalar[:20]:
    print('  ', h)
basarisiz = [a for a, ok in sonuc if not ok]
print('SONUÇ:', 'HEPSİ TAMAM' if not basarisiz else f'{len(basarisiz)} HATA: ' + '; '.join(basarisiz))
