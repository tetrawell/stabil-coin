# Simülatör 3.4 → Al-Sat 0.8 "Pi'ye gönder" uçtan uca testi (iki gerçek program, gerçek tarayıcı, ev ağı yerine 127.0.0.1).
# 1) Al-Sat'ta "Bilgisayardan al" açılır, kod okunur.  2) Simülatörde Pi bağlantısı kurulur (dene + kaydet ve gönder).
# 3) Dosya Al-Sat'ın klasörüne yazılır, listede görünür, Al-Sat haber verir.  4) Aynı dosya yeniden: "yenilendi".
# 5) Al-Sat'ta yeni kod üretilince eski kodla gönderim reddedilir.  6) Al-Sat'ta kapatılınca gönderim olmaz.
# kullanım: python3 -I gonderim_testi.py <calisma_klasoru> <alsat_ikilisi> <simulator_ikilisi> <alsat_kaynak_klasoru>
import sys, os, time, json, subprocess, shutil, glob
from playwright.sync_api import sync_playwright
S, ALSAT, SIM, KAYNAK = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
UZAK_PORT = 47333
STRAT = os.path.join(S, 'alsat_stratejiler')
VERI = os.path.join(S, 'alsat_veri')
ANAHTAR = os.path.join(S, 'alsat_anahtar')
SIMK = os.path.join(S, 'simulator')
SIMBAG = os.path.join(S, 'sim_baglanti')
EKRAN = os.path.join(S, 'ekran')
for d in (STRAT, VERI, ANAHTAR, SIMK, SIMBAG, EKRAN):
    shutil.rmtree(d, ignore_errors=True)
    os.makedirs(d)
shutil.copy(SIM, os.path.join(SIMK, 'kriptosim'))
import socket
for port in (47321, 47322, UZAK_PORT):  # açık kalmış eski bir program testi yanıltmasın
    with socket.socket() as so:
        if so.connect_ex(('127.0.0.1', port)) == 0:
            sys.exit(f'{port} kapısında açık bir program var; önce onu kapat')
sonuc = []

def kontrol(ad, ok, ayrinti=''):
    sonuc.append((ad, ok))
    print(('TAMAM ' if ok else 'HATA  ') + ad + (f' — {ayrinti}' if ayrinti else ''), flush=True)

env_a = dict(os.environ, ALSAT_TEST='1', ALSAT_PENCERE_YOK='1', ALSAT_VERI_KLASORU=VERI, ALSAT_KLASOR=STRAT,
             ALSAT_ANAHTAR_KLASORU=ANAHTAR, ALSAT_SILME_YOK='1', ALSAT_UZAK_PORT=str(UZAK_PORT))
env_s = dict(os.environ, SIM_TEST='1', SIM_PENCERE_YOK='1', SIM_ALSAT_KLASORU=SIMBAG)
alsat = subprocess.Popen([ALSAT], env=env_a, stdout=open(os.path.join(S, 'alsat.log'), 'w'), stderr=subprocess.STDOUT)
sim = subprocess.Popen([os.path.join(SIMK, 'kriptosim')], env=env_s, cwd=SIMK, stdout=open(os.path.join(S, 'sim.log'), 'w'), stderr=subprocess.STDOUT)
time.sleep(2)
hatalar = []
try:
    with sync_playwright() as pw:
        br = pw.chromium.launch()

        def sayfa(adres, bekle):
            pg = br.new_page(viewport={'width': 1700, 'height': 1000})
            # (beklenen hata cevapları — yanlış kod, kapalı — tarayıcıda "Failed to load resource" diye görünür; sayılmaz)
            pg.on('console', lambda m: hatalar.append(f'{adres} {m.type}: {m.text}') if m.type == 'error' and 'Failed to load resource' not in m.text else None)
            pg.on('pageerror', lambda e: hatalar.append(f'{adres} pageerror: {e}'))
            pg.goto(adres)
            pg.wait_for_selector(bekle, timeout=30000)
            return pg

        a = sayfa('http://127.0.0.1:47322/', '#uzakCip')
        time.sleep(1.5)
        a.click('#uzakCip')
        a.wait_for_selector('#uzAcik')
        a.click('label.uzak-anahtar')
        a.wait_for_selector('.uzak-kod', timeout=10000)
        time.sleep(0.5)
        kod = a.inner_text('.uzak-kod').strip()
        a.screenshot(path=f'{EKRAN}/01_alsat_bilgisayardan_al.png')
        kontrol('Al-Sat: bilgisayardan alma açıldı, kod göründü', len(kod.replace('-', '')) == 12, kod[:4] + '-…')
        kontrol('Al-Sat: kapı dinliyor', 'açık' in a.inner_text('#uzakCip'))
        a.click('#modalKutu [data-m="kapat"]')

        s = sayfa('http://127.0.0.1:47321/', '#ozet .ozet-kart')
        time.sleep(1.5)
        s.click('#piyeGonder')  # bağlantı yok: pencere açılır
        s.wait_for_selector('#piAdres')
        s.fill('#piAdres', f'127.0.0.1:{UZAK_PORT}')
        s.fill('#piKod', kod.lower().replace('-', ' '))
        s.click('#modalKutu [data-m="piDene"]')
        s.wait_for_function("() => { const e = document.querySelector('#piSonuc'); return e && !e.hidden && !e.textContent.includes('Deneniyor'); }", timeout=20000)
        dene = s.inner_text('#piSonuc')
        s.screenshot(path=f'{EKRAN}/02_sim_pi_baglan.png')
        kontrol('Simülatör: bağlantı denemesi tamam', 'Bağlantı tamam' in dene and '0.8' in dene, dene[:100])
        s.click('#modalKutu [data-m="piKaydetGonder"]')
        s.wait_for_function("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].some(e => e.textContent.includes(\"Pi'ye\"))", timeout=20000)
        time.sleep(0.5)
        s.screenshot(path=f'{EKRAN}/03_sim_gonderildi.png')
        bild = s.evaluate("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].map(e => e.textContent).join(' | ')")
        kontrol('Simülatör: "Pi\'ye gönderildi" bildirimi', "Pi'ye gönderildi" in bild, bild[:160])
        dosyalar = [os.path.basename(x) for x in glob.glob(os.path.join(STRAT, '*.json'))]
        kontrol('Al-Sat: dosya strateji klasörüne yazıldı', len(dosyalar) == 1, str(dosyalar))
        ad = dosyalar[0] if dosyalar else ''
        icerik = json.load(open(os.path.join(STRAT, ad), encoding='utf-8')) if ad else {}
        kontrol('dosya simülatörün dışa aktardığıyla aynı biçimde', icerik.get('tur') == 'kripto-simulator-strateji' and icerik.get('borsa') == 'mexc' and 'strateji' in icerik)
        yerel = glob.glob(os.path.join(SIMK, 'stratejiler', '*.json'))
        kontrol('simülatör kendi stratejiler klasörüne de kaydetti', len(yerel) == 1)
        a.wait_for_selector(f'.skart[data-ad="{ad}"]', timeout=15000)
        a.wait_for_function("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].some(e => e.textContent.includes('Bilgisayardan strateji geldi'))", timeout=15000)
        a.screenshot(path=f'{EKRAN}/04_alsat_geldi.png')
        kontrol('Al-Sat: dosya listede, "Bilgisayardan strateji geldi" bildirimi', True)
        st = os.stat(os.path.join(STRAT, ad)).st_mtime
        time.sleep(1.2)
        s.click('#piyeGonder')
        s.wait_for_function("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].filter(e => e.textContent.includes(\"Pi'ye gönderildi\")).length >= 2 || [...document.querySelectorAll('.bildirim, #bildirimler *')].some(e => e.textContent.includes('yenilendi'))", timeout=20000)
        bild = s.evaluate("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].map(e => e.textContent).join(' | ')")
        kontrol('aynı dosya yeniden: "yenilendi"', 'yenilendi' in bild and os.stat(os.path.join(STRAT, ad)).st_mtime > st, bild[-160:])

        # Al-Sat'ta yeni kod → eski kodla gönderim reddedilir
        a.click('#uzakCip'); a.wait_for_selector('[data-m="uzakYeniKod"]')
        a.click('[data-m="uzakYeniKod"]'); a.wait_for_selector('[data-m="evet"]'); a.click('[data-m="evet"]')
        a.wait_for_selector('.uzak-kod', timeout=10000)
        time.sleep(0.5)
        yeni = a.inner_text('.uzak-kod').strip()
        kontrol('Al-Sat: yeni kod üretildi', yeni != kod and len(yeni.replace('-', '')) == 12)
        a.click('#modalKutu [data-m="kapat"]')
        n0 = s.evaluate("() => document.querySelectorAll('.bildirim, #bildirimler *').length")
        s.click('#piyeGonder')
        s.wait_for_function("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].some(e => e.textContent.includes('gönderilemedi'))", timeout=20000)
        bild = s.evaluate("() => [...document.querySelectorAll('.bildirim, #bildirimler *')].map(e => e.textContent).join(' | ')")
        s.screenshot(path=f'{EKRAN}/05_sim_eski_kod.png')
        kontrol('eski kodla gönderim reddedildi, sebebi yazıyor', 'Eşleştirme kodu yanlış' in bild, bild[-160:])
        # Al-Sat'ta kapatılınca
        a.click('#uzakCip'); a.wait_for_selector('label.uzak-anahtar'); a.click('label.uzak-anahtar')
        a.wait_for_function("() => document.querySelector('#uzakCip').textContent.includes('kapalı')", timeout=10000)
        a.click('#modalKutu [data-m="kapat"]')
        s.click('#piAyar'); s.wait_for_selector('#piKod'); s.fill('#piKod', yeni)
        s.click('#modalKutu [data-m="piDene"]')
        s.wait_for_function("() => { const e = document.querySelector('#piSonuc'); return e && !e.hidden && !e.textContent.includes('Deneniyor'); }", timeout=30000)
        dene = s.inner_text('#piSonuc')
        s.screenshot(path=f'{EKRAN}/06_sim_alsat_kapali.png')
        kontrol('Al-Sat kapatınca bağlanılamıyor, anlaşılır yazı', 'cevap vermedi' in dene or 'kapalı' in dene, dene[:140])
        br.close()
finally:
    for p in (alsat, sim):
        p.terminate()
        try:
            p.wait(timeout=10)
        except Exception:
            p.kill()
print('Konsol hataları:', hatalar[:10])
kontrol('sayfalarda konsol hatası yok', not hatalar)
basarisiz = [a for a, ok in sonuc if not ok]
print('\nSONUÇ:', 'HEPSİ TAMAM' if not basarisiz else f'{len(basarisiz)} HATA: {basarisiz}')
sys.exit(1 if basarisiz else 0)
