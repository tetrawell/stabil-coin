# Arayüz testi: sayfayı açar, konsol hatalarını toplar, her stratejiye tıklayıp ekran görüntüsü alır.
# kullanım: python3 -I ekran_testi.py <cikti_klasoru> [genislik] [yukseklik]
import sys, time, json
from playwright.sync_api import sync_playwright
cikti = sys.argv[1]
W = int(sys.argv[2]) if len(sys.argv) > 2 else 2560
H = int(sys.argv[3]) if len(sys.argv) > 3 else 1440
hatalar = []
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={'width': W, 'height': H}, device_scale_factor=1)
    pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type in ('error', 'warning') else None)
    pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
    pg.goto('http://127.0.0.1:47322/')
    pg.wait_for_selector('.skart', timeout=15000)
    time.sleep(4)
    pg.screenshot(path=f'{cikti}/01_acilis_{W}.png')
    adlar = pg.eval_on_selector_all('.skart', 'els => els.map(e => e.dataset.ad)')
    print('kartlar:', adlar)
    for i, ad in enumerate(adlar):
        pg.click(f'.skart[data-ad="{ad}"]')
        time.sleep(3.5)
        pg.screenshot(path=f'{cikti}/{i+2:02d}_{ad.replace(".json","")}_{W}.png')
        durum = pg.evaluate("() => ({ bos: !document.querySelector('#bosKutu').hidden, hata: !document.querySelector('#hataKutu').hidden, satir: document.querySelectorAll('#islemTablo tbody tr').length, karar: document.querySelector('#sonKarar').innerText, canli: document.querySelector('#canliYazi').innerText })")
        print(ad, json.dumps(durum, ensure_ascii=False))
    b.close()
print('KONSOL:', len(hatalar))
for h in hatalar[:40]:
    print('  ', h)
