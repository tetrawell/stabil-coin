# Kumbara arayüz testi (sanal para): strateji seçilir, "Başlat…" penceresiyle sanal kumbara açılır,
# motorun ilk alımı ve satışı beklenir, sekmeler, MEXC ve sanal cüzdan pencereleri açılır, sonunda kumbara durdurulur.
# Program önceden çalışıyor olmalı (ALSAT_TEST=1 ile sahte veri ve sahte piyasa önerilir).
# kullanım: python3 -I kumbara_ekran_testi.py <cikti_klasoru> <dosya_adi> [tutar] [bekleme_sn]
import sys, time, json
from playwright.sync_api import sync_playwright
cikti, dosya = sys.argv[1], sys.argv[2]
tutar = sys.argv[3] if len(sys.argv) > 3 else '100'
bekle = int(sys.argv[4]) if len(sys.argv) > 4 else 300
hatalar = []

def olaylar(pg):
    d = pg.evaluate('() => window.__alsat.durum()')
    ks = [k for k in d['kumbaralar'] if k['durum'] != 'bitti']
    return ks[0] if ks else None

with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={'width': 2560, 'height': 1440}, device_scale_factor=1)
    pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type in ('error', 'warning') else None)
    pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
    pg.on('dialog', lambda d: (hatalar.append(f'dialog: {d.message}'), d.dismiss()))
    pg.goto('http://127.0.0.1:47322/')
    pg.wait_for_selector('.skart', timeout=15000)
    time.sleep(2)
    pg.screenshot(path=f'{cikti}/01_acilis.png')
    pg.click(f'.skart[data-ad="{dosya}"]')
    pg.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
    pg.wait_for_selector('#kumbaraKart [data-eylem="baslatAc"]', timeout=10000)
    time.sleep(1.5)
    pg.screenshot(path=f'{cikti}/02_secildi.png')
    pg.click('#kumbaraKart [data-eylem="baslatAc"]')
    pg.wait_for_selector('#modal:not([hidden]) #mTutar', timeout=10000)
    time.sleep(0.5)
    pg.screenshot(path=f'{cikti}/03_baslat_penceresi.png')
    # gerçek parayı seçince onay alanı görünmeli (test modunda kapalı yazar)
    pg.click('.mod-kart.gercek')
    time.sleep(0.3)
    pg.screenshot(path=f'{cikti}/03b_gercek_secili.png')
    pg.click('.mod-kart:not(.gercek)')
    pg.fill('#mTutar', tutar)
    pg.click('#mBaslat')
    pg.wait_for_selector('#modal', state='hidden', timeout=15000)
    t0 = time.time()
    pg.wait_for_function("() => { const d = window.__alsat.durum(); return d && d.kumbaralar.some(k => k.durum !== 'bitti'); }", timeout=10000)
    time.sleep(2)
    kb = olaylar(pg)
    print('kumbara açıldı:', kb['id'], kb['mod'], kb['baslangicTutar'], 'ilkMum', kb['ilkMum'], 'mumSayisi', kb['mumSayisi'])
    print('izleyiciler:', json.dumps(pg.evaluate('() => window.__alsat.izleyiciler()'), ensure_ascii=False))
    pg.screenshot(path=f'{cikti}/04_kumbara_acik.png')
    # ilk alım ve satış
    alindi = satildi = False
    while time.time() - t0 < bekle and not (alindi and satildi):
        time.sleep(3)
        kb = olaylar(pg)
        if not kb:
            break
        if not alindi and kb['islemler']:
            alindi = True
            print(f'ALIM {time.time()-t0:.0f} sn:', json.dumps(kb['islemler'][0]['alis'], ensure_ascii=False)[:300])
            time.sleep(1)
            pg.screenshot(path=f'{cikti}/05_alim.png')
        if alindi and kb['islemler'] and kb['islemler'][0].get('satis'):
            satildi = True
            t = kb['islemler'][0]
            print(f'SATIŞ {time.time()-t0:.0f} sn: sebep', t.get('satisSebebi'), 'net', t['net'], 'yüzde', t['yuzde'], 'beklenen', t.get('beklenenYuzde'), t.get('beklenenVar'))
            time.sleep(1)
            pg.screenshot(path=f'{cikti}/06_satis.png')
    print('alındı:', alindi, 'satıldı:', satildi)
    kb = olaylar(pg)
    if kb:
        print('son durum:', kb['durum'], 'nakit', kb['nakit'], 'adet', kb['adet'], 'değer', kb['deger'], 'işlem', len(kb['islemler']))
        for o in kb['olaylar'][-14:]:
            print('   ', o['tur'], o['mesaj'][:150])
    # sekmeler
    pg.click('#sekmeler button[data-sekme="olaylar"]'); time.sleep(0.5)
    pg.screenshot(path=f'{cikti}/07_olaylar.png')
    pg.click('#sekmeler button[data-sekme="simulator"]'); time.sleep(0.5)
    pg.screenshot(path=f'{cikti}/08_simulator_tablosu.png')
    pg.click('#sekmeler button[data-sekme="kumbara"]'); time.sleep(0.5)
    # pencereler
    pg.click('#mexcCip'); pg.wait_for_selector('#modal:not([hidden])'); time.sleep(0.3)
    pg.screenshot(path=f'{cikti}/09_mexc.png')
    pg.keyboard.press('Escape')
    pg.click('#sanalCip'); pg.wait_for_selector('#modal:not([hidden])'); time.sleep(0.3)
    pg.screenshot(path=f'{cikti}/10_sanal.png')
    pg.keyboard.press('Escape')
    # 1600x900 görünüm
    pg.set_viewport_size({'width': 1600, 'height': 900}); time.sleep(1)
    pg.screenshot(path=f'{cikti}/11_kucuk_ekran.png')
    pg.set_viewport_size({'width': 2560, 'height': 1440}); time.sleep(0.5)
    # durdur: şimdi sat
    btn = pg.query_selector('#kumbaraKart [data-eylem="simdiSat"]')
    if btn:
        btn.click(); pg.wait_for_selector('#modal:not([hidden])'); time.sleep(0.3)
        pg.screenshot(path=f'{cikti}/12_durdur_onay.png')
        pg.click('#modal [data-m="evet"]')
        pg.wait_for_function("() => !window.__alsat.durum().kumbaralar.some(k => k.durum !== 'bitti')", timeout=40000)
        time.sleep(1.5)
        d = pg.evaluate('() => window.__alsat.durum()')
        k = d['kumbaralar'][-1]
        print('kapandı:', k['durum'], k.get('bitisSebebi'), 'iade', k.get('iade'), '| sanal cüzdan', d['sanal'])
        pg.screenshot(path=f'{cikti}/13_kapandi.png')
    b.close()
print('KONSOL:', len(hatalar))
for h in hatalar[:40]:
    print('  ', h)
