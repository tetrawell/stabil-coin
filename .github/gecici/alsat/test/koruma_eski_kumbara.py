# Eski sürümden (korumasız) devam eden kumbara: program açılışta geçici koruma kurar, sayfa grafiği yükleyince
# ayarın geçmişinden asıl korumaya geçirir (0.7).
# Önce durum.json'daki çalışan kumbaraların "koruma" alanı silinir ve program yeniden açılır; sonra bu test:
# kullanım: python3 -I koruma_eski_kumbara.py <cikti_klasoru>
import sys, time, json
from playwright.sync_api import sync_playwright
cikti = sys.argv[1]
hatalar = []
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={'width': 2560, 'height': 1440}, device_scale_factor=1)
    pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type in ('error', 'warning') else None)
    pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
    pg.goto('http://127.0.0.1:47322/')
    pg.wait_for_selector('.skart', timeout=15000)
    d = pg.evaluate('() => window.__alsat.durum()')
    once = [k for k in d['kumbaralar'] if k['durum'] != 'bitti']
    print('açılışta geçici korumalı kumbara:', sum(1 for k in once if (k.get('koruma') or {}).get('gecici')), '/ korumasız:', sum(1 for k in once if not k.get('koruma')))
    t0 = time.time()
    tamam = False
    while time.time() - t0 < 60:
        d = pg.evaluate('() => window.__alsat.durum()')
        ks = [k for k in d['kumbaralar'] if k['durum'] != 'bitti']
        if ks and all(k.get('koruma') and not k['koruma'].get('gecici') for k in ks):
            tamam = True
            break
        time.sleep(1)
    print(f'koruma kuruldu: {tamam} ({time.time()-t0:.0f} sn)')
    for k in ks:
        kr = k.get('koruma') or {}
        print('  ', k['dosya'], json.dumps({x: kr.get(x) for x in ('hassasiyet', 'sinir', 'karakter', 'baslangic')}, ensure_ascii=False)[:400])
        print('   son olay:', k['olaylar'][-1]['mesaj'][:200] if k['olaylar'] else '')
    pg.click(f'.skart[data-ad="{ks[0]["dosya"]}"]')
    time.sleep(3)
    pg.screenshot(path=f'{cikti}/eski_kumbara_koruma.png')
    b.close()
print('KONSOL:', len(hatalar), hatalar[:10])
ok = tamam and not [h for h in hatalar if 'error' in h]
print('SONUÇ:', 'HEPSİ TAMAM' if ok else 'HATA')
sys.exit(0 if ok else 1)
