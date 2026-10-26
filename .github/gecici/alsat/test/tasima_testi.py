# 0.7 → 0.8 geçişinin uçtan uca testi (gerçek iki program, gerçek tarayıcı, test verisi; ~3 dk):
# 1) 0.7 kendi klasöründe açılır, sanal kumbara başlatılır, program kapanır.  2) 0.8 yan klasörde açılır:
# "Eski sürümün kayıtları bulundu" penceresi çıkar.  3) "Taşı ve devam et": kumbara 0.8'de kaldığı yerden sürer,
# eski kayıt işaretlenir.  4) 0.8 açıkken 0.7 açılmak istenirse ikinci program olmaz.  5) 0.8 kapatılıp 0.7 açılınca
# 0.7 kumbarasız açılır (aynı kumbarayı ikinci kez çalıştırmaz).
# kullanım: python3 -I tasima_testi.py <calisma> <0.7 linux ikilisi> <0.8 linux ikilisi> <0.8 kaynak klasörü>
import sys, os, time, json, subprocess, shutil, glob, urllib.request, socket
from playwright.sync_api import sync_playwright
S, ESKI, YENI, KAYNAK = sys.argv[1:5]
shutil.rmtree(S, ignore_errors=True)
D7, D8, STRAT, EKRAN = (os.path.join(S, x) for x in ('KriptoAlSat_v0_7', 'KriptoAlSat_v0_8', 'stratejiler', 'ekran'))
for d in (D7, D8, STRAT, EKRAN):
    os.makedirs(d)
shutil.copy(ESKI, os.path.join(D7, 'KriptoAlSat'))
shutil.copy(YENI, os.path.join(D8, 'KriptoAlSat'))
subprocess.run(['node', os.path.join(KAYNAK, 'test', 'hizli_strateji.js'), os.path.join(KAYNAK, 'web'), STRAT, 'DOGEUSDT', '1m', '0.25'], check=True)
sonuc = []

def kontrol(ad, ok, ayrinti=''):
    sonuc.append((ad, ok))
    print(('TAMAM ' if ok else 'HATA  ') + ad + (f' — {ayrinti}' if ayrinti else ''), flush=True)

for port in (47322,):
    with socket.socket() as so:
        if so.connect_ex(('127.0.0.1', port)) == 0:
            sys.exit(f'{port} kapısında açık bir program var; önce onu kapat')
env = dict(os.environ, ALSAT_TEST='1', ALSAT_PENCERE_YOK='1', ALSAT_KLASOR=STRAT, ALSAT_SILME_YOK='1',
           ALSAT_ANAHTAR_KLASORU=os.path.join(S, 'anahtar'), ALSAT_UZAK_PORT='47334', PATH='/usr/bin:/bin')
env.pop('ALSAT_VERI_KLASORU', None)  # kayıtlar programın yanında (gerçekteki gibi)

def ac(klasor, ad):
    p = subprocess.Popen([os.path.join(klasor, 'KriptoAlSat')], env=env, cwd=klasor, stdout=open(os.path.join(S, ad + '.log'), 'a'), stderr=subprocess.STDOUT)
    time.sleep(2)
    return p

def kapat(p):
    p.terminate()
    try:
        p.wait(timeout=15)
    except Exception:
        p.kill()

def durum():
    return json.loads(urllib.request.urlopen('http://127.0.0.1:47322/api/durum', timeout=5).read())

hatalar = []
with sync_playwright() as pw:
    br = pw.chromium.launch()

    def sayfa():
        pg = br.new_page(viewport={'width': 2560, 'height': 1440})
        pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
        pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type == 'error' and 'Failed to load resource' not in m.text else None)
        pg.on('dialog', lambda d: d.accept())
        pg.goto('http://127.0.0.1:47322/')
        pg.wait_for_selector('.skart', timeout=20000)
        return pg

    # 1) 0.7: sanal kumbara
    p7 = ac(D7, 'alsat07')
    pg = sayfa()
    ad = os.path.basename(glob.glob(os.path.join(STRAT, '*.json'))[0])
    pg.click(f'.skart[data-ad="{ad}"]')
    pg.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
    pg.wait_for_selector('#kumbaraKart [data-eylem="baslatAc"]', timeout=10000)
    time.sleep(1)
    pg.click('#kumbaraKart [data-eylem="baslatAc"]')
    pg.wait_for_selector('#modal:not([hidden]) #mTutar', timeout=10000)
    pg.click('.mod-kart:not(.gercek)')
    pg.fill('#mTutar', '100')
    pg.click('#mBaslat')
    pg.wait_for_selector('#modal', state='hidden', timeout=20000)
    pg.wait_for_function("() => { const d = window.__alsat.durum(); return d && d.kumbaralar.some(k => k.durum !== 'bitti'); }", timeout=15000)
    time.sleep(3)
    d7 = durum()
    k7 = [k for k in d7['kumbaralar'] if k['durum'] != 'bitti']
    kontrol('0.7: sanal kumbara açıldı', len(k7) == 1, k7[0]['id'] if k7 else '')
    pg.screenshot(path=f'{EKRAN}/01_07_kumbara.png')
    pg.close()
    kapat(p7)
    eski_durum = os.path.join(D7, 'kayitlar', 'durum.json')
    kontrol('0.7 kaydını kendi klasörüne yazdı', os.path.exists(eski_durum))
    t = time.time() - 600
    os.utime(eski_durum, (t, t))  # 0.7 kapanalı 10 dk olmuş gibi

    # 2) 0.8: taşıma penceresi
    p8 = ac(D8, 'alsat08')
    pg = sayfa()
    pg.wait_for_selector('#modal:not([hidden]) [data-m="tasi"]', timeout=15000)
    time.sleep(0.5)
    metin = pg.inner_text('#modalKutu')
    pg.screenshot(path=f'{EKRAN}/02_08_tasima_penceresi.png')
    kontrol('0.8: "Eski sürümün kayıtları bulundu" penceresi', 'KriptoAlSat_v0_7' in metin and '1' in metin, metin[:160].replace('\n', ' '))
    pg.click('[data-m="tasi"]')
    pg.wait_for_selector('#modal', state='hidden', timeout=15000)
    pg.wait_for_function("() => { const d = window.__alsat.durum(); return d && d.kumbaralar.some(k => k.durum !== 'bitti'); }", timeout=15000)
    time.sleep(4)
    d8 = durum()
    k8 = [k for k in d8['kumbaralar'] if k['durum'] != 'bitti']
    kontrol('kumbara 0.8\'de aynı kimlikle sürüyor', len(k8) == 1 and k8[0]['id'] == k7[0]['id'], k8[0]['id'] if k8 else '')
    iz = pg.evaluate('() => window.__alsat.izleyiciler()')
    kontrol('0.8: kumbaranın izleyicisi aynı ilk mumdan çalışıyor', any(i['id'] == k7[0]['id'] and i['ilk'] == k7[0]['ilkMum'] for i in iz), json.dumps(iz)[:160])
    kontrol('eski kayıt işaretlendi', not os.path.exists(eski_durum) and glob.glob(eski_durum + '.tasindi-*') and os.path.exists(os.path.join(D7, 'kayitlar', 'TASINDI.txt')))
    kontrol('0.8 kayıtları kendi klasöründe', os.path.exists(os.path.join(D8, 'kayitlar', 'durum.json')))
    pg.screenshot(path=f'{EKRAN}/03_08_tasindi.png')
    pg.reload()
    pg.wait_for_selector('.skart', timeout=20000)
    time.sleep(3)
    kontrol('yeniden yükleyince bir daha sorulmuyor', pg.evaluate("() => document.querySelector('#modal').hidden"))

    # 4) 0.8 açıkken 0.7 açılırsa ikinci program olmaz
    p7b = subprocess.Popen([os.path.join(D7, 'KriptoAlSat')], env=env, cwd=D7, stdout=open(os.path.join(S, 'alsat07b.log'), 'w'), stderr=subprocess.STDOUT)
    try:
        p7b.wait(timeout=20)
        biter = True
    except Exception:
        biter = False
        p7b.kill()
    kontrol('0.8 açıkken 0.7 ikinci program olarak çalışmadı', biter)
    pg.close()
    kapat(p8)

    # 5) 0.8 kapalıyken 0.7: kumbarasız
    p7c = ac(D7, 'alsat07c')
    time.sleep(1)
    d7c = durum()
    kontrol('0.7 yeniden açılınca kumbarasız (aynı kumbarayı ikinci kez çalıştırmaz)', len([k for k in d7c['kumbaralar'] if k['durum'] != 'bitti']) == 0, str(len(d7c['kumbaralar'])))
    kapat(p7c)
    # 6) 0.8 yeniden: kumbara yerinde, taşıma sorulmuyor
    p8 = ac(D8, 'alsat08b')
    pg = sayfa()
    time.sleep(3)
    d8b = durum()
    kontrol('0.8 yeniden açılınca kumbara yerinde, soru yok', any(k['id'] == k7[0]['id'] and k['durum'] != 'bitti' for k in d8b['kumbaralar']) and pg.evaluate("() => document.querySelector('#modal').hidden"))
    pg.close()
    kapat(p8)
    br.close()
print('Konsol hataları:', hatalar[:8])
kontrol('sayfalarda konsol hatası yok', not hatalar)
basarisiz = [a for a, ok in sonuc if not ok]
print('\nSONUÇ:', 'HEPSİ TAMAM' if not basarisiz else f'{len(basarisiz)} HATA: {basarisiz}')
sys.exit(1 if basarisiz else 0)
