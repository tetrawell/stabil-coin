# Koruma arayüz testi (0.7, sanal para, test verisi). Program önceden çalışıyor olmalı (ALSAT_TEST=1).
# 1) Başlat penceresinde koruma sınırları ve sağlamlık testi görünür; hassasiyet değişince sınırlar değişir.
# 2) Kumbara korumayla açılır (sınırlar programda kayıtlı, kartta görünür).
# 3) "Değiştir" penceresi: hassasiyet önizleme, sınırlar elle 1'e çekilir (gerçek zamanda çabuk tetiklensin diye).
# 4) İlk zararda kumbara kenara alınır: kenardayken alım yok, gölge işlemler; ilk kârlı gölge işlemde oyuna döner.
# 5) Tekrar kenara alınırsa "Şimdi geri dön" denenir. Konsolda hata olmamalı.
# kullanım: python3 -I koruma_ekran_testi.py <cikti_klasoru> <dosya_adi> [bekleme_sn]
import sys, time, json, re
from playwright.sync_api import sync_playwright
cikti, dosya = sys.argv[1], sys.argv[2]
bekle = int(sys.argv[3]) if len(sys.argv) > 3 else 900
hatalar, sonuclar = [], []


def kontrol(ad, kosul, ek=''):
    print(('TAMAM ' if kosul else 'HATA  ') + ad + (f'  [{ek}]' if ek and not kosul else ''), flush=True)
    if not kosul:
        sonuclar.append(ad)


def kumbara(pg):
    d = pg.evaluate('() => window.__alsat.durum()')
    ks = [k for k in d['kumbaralar'] if k['durum'] != 'bitti']
    return ks[0] if ks else None


def bekle_kosul(pg, fn, sure, adim=2):
    son = time.time() + sure
    while time.time() < son:
        kb = kumbara(pg)
        if kb and fn(kb):
            return kb
        time.sleep(adim)
    return None


with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={'width': 2560, 'height': 1440}, device_scale_factor=1)
    pg.on('console', lambda m: hatalar.append(f'{m.type}: {m.text}') if m.type in ('error', 'warning') else None)
    pg.on('pageerror', lambda e: hatalar.append(f'pageerror: {e}'))
    pg.on('dialog', lambda d: (hatalar.append(f'dialog: {d.message}'), d.dismiss()))
    pg.goto('http://127.0.0.1:47322/')
    pg.wait_for_selector('.skart', timeout=15000)
    pg.click(f'.skart[data-ad="{dosya}"]')
    pg.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
    pg.wait_for_selector('#kumbaraKart [data-eylem="baslatAc"]', timeout=10000)
    time.sleep(1.5)

    # ---------- 1) başlat penceresi ----------
    pg.click('#kumbaraKart [data-eylem="baslatAc"]')
    pg.wait_for_selector('#modal:not([hidden]) #mKorumaIcerik', timeout=10000)
    pg.wait_for_function("() => /Kenara alınır/.test(document.querySelector('#mKorumaIcerik').textContent)", timeout=15000)
    t0 = time.time()
    pg.wait_for_function("() => { const e = document.querySelector('#mSaglamlik .saglamlik'); return e && !e.querySelector('.donen'); }", timeout=120000)
    print(f'sağlamlık testi {time.time()-t0:.1f} sn:', pg.inner_text('#mSaglamlik'))
    kontrol('başlatta sağlamlık testi sonucu', 'Sağlamlık testi' in pg.inner_text('#mSaglamlik'))
    dengeli = pg.inner_text('#mKorumaIcerik')
    print('DENGELİ:', dengeli)
    pg.screenshot(path=f'{cikti}/01_baslat_koruma.png')
    once = pg.text_content('#mKorumaIcerik')
    pg.click('#mHassasiyet button[data-hassasiyet="hassas"]')
    pg.wait_for_function("(e) => document.querySelector('#mKorumaIcerik').textContent !== e", arg=once, timeout=10000)
    hassas = pg.inner_text('#mKorumaIcerik')
    print('HASSAS:', hassas)
    pg.click('#mHassasiyet button[data-hassasiyet="sakin"]')
    time.sleep(1.5)
    sakin = pg.inner_text('#mKorumaIcerik')
    print('SAKİN:', sakin)
    kontrol('hassasiyet değişince sınırlar değişti', len({dengeli, hassas, sakin}) >= 2)
    pg.click('#mHassasiyet button[data-hassasiyet="dengeli"]')
    time.sleep(1.5)
    pg.fill('#mTutar', '100')
    pg.click('#mBaslat')
    pg.wait_for_selector('#modal', state='hidden', timeout=15000)

    # ---------- 2) kumbara korumayla açıldı ----------
    kb = bekle_kosul(pg, lambda k: k.get('koruma'), 15, 1)
    kontrol('kumbara korumayla açıldı', kb is not None and kb['koruma'].get('sinir', {}).get('seri', 0) >= 1, json.dumps(kb.get('koruma') if kb else None)[:300])
    if kb:
        kr = kb['koruma']
        print('koruma:', json.dumps({x: kr.get(x) for x in ('hassasiyet', 'sinir', 'karakter', 'saglamlik', 'kayipEtkin', 'kayipKaynak', 'tavan')}, ensure_ascii=False))
        kontrol('hassasiyet dengeli', kr.get('hassasiyet') == 'dengeli')
        kontrol('geçmiş işlemler ekrana gönderilmiyor', 'gecmis' not in kr)
        kontrol('tavan %10', kr.get('tavan') == 10)
        kontrol('sağlamlık sonucu kumbarada', bool(kr.get('saglamlik')))
    time.sleep(1.5)
    pg.screenshot(path=f'{cikti}/02_kart_koruma.png')
    kontrol('kartta koruma bölümü', pg.query_selector('#kumbaraKart .kk-koruma .kkr-rozet.normal') is not None)

    # ---------- 3) değiştir penceresi ----------
    pg.click('#kumbaraKart [data-eylem="korumaAyar"]')
    pg.wait_for_selector('#modal:not([hidden]) #kSeri', timeout=5000)
    time.sleep(0.5)
    pg.screenshot(path=f'{cikti}/03_koruma_ayar.png')
    seri_once = pg.input_value('#kSeri')
    pg.click('#kHassasiyet button[data-hassasiyet="sakin"]')
    time.sleep(1.5)
    print('seri dengeli/sakin:', seri_once, pg.input_value('#kSeri'), '| kayıp sakin', pg.input_value('#kKayip'))
    pg.click('#kHassasiyet button[data-hassasiyet="dengeli"]')
    time.sleep(1.5)
    kontrol('dengeliye dönünce otomatik değer geri geldi', pg.input_value('#kSeri') == seri_once)
    pg.fill('#kSeri', '1')
    pg.fill('#kGeri', '1')
    pg.click('#modal [data-m="korumaKaydet"]')
    pg.wait_for_selector('#modal', state='hidden', timeout=5000)
    kb = bekle_kosul(pg, lambda k: k['koruma']['sinir']['seri'] == 1, 10, 1)
    kontrol('sınırlar elle kaydedildi', kb is not None and kb['koruma'].get('elle') and kb['koruma']['sinir']['geriDonus'] == 1)

    # ---------- 4) ilk zararda kenara alma, gölge, oyuna dönüş ----------
    t0 = time.time()
    kb = bekle_kosul(pg, lambda k: k['koruma'].get('kenarda'), bekle, 3)
    kontrol('zararda kenara alındı', kb is not None, '')
    if kb:
        print(f'kenara alındı {time.time()-t0:.0f} sn:', kb['koruma'].get('kenarSebep'))
        n_islem = len(kb['islemler'])
        time.sleep(2)
        pg.screenshot(path=f'{cikti}/04_kenarda.png')
        kontrol('kartta kenarda rozeti', pg.query_selector('#kumbaraKart .kkr-rozet.kenarda') is not None)
        t1 = time.time()
        kb2 = bekle_kosul(pg, lambda k: not k['koruma'].get('kenarda') or len(k['koruma'].get('golge') or []) >= 1, bekle, 3)
        if kb2 and kb2['koruma'].get('golge'):
            print('gölge işlemler:', json.dumps(kb2['koruma']['golge'], ensure_ascii=False)[:400])
            pg.screenshot(path=f'{cikti}/05_golge.png')
        kb2 = bekle_kosul(pg, lambda k: not k['koruma'].get('kenarda'), bekle, 3)
        kontrol('kârlı gölge işlemde oyuna döndü', kb2 is not None)
        if kb2:
            print(f'oyuna döndü {time.time()-t1:.0f} sn sonra; dönemler:', json.dumps(kb2['koruma'].get('donemler'), ensure_ascii=False)[:400])
            kontrol('kenardayken gerçek alım yapılmadı', all(t['alis']['bitis'] < kb2['koruma']['donemler'][-1]['bas'] or t['alis']['bitis'] > kb2['koruma']['donemler'][-1]['bit'] for t in kb2['islemler'] if t.get('alis')))
            time.sleep(2)
            pg.screenshot(path=f'{cikti}/06_oyuna_dondu.png')
        # ---------- 5) tekrar kenara alınırsa elle geri dön ----------
        kb3 = bekle_kosul(pg, lambda k: k['koruma'].get('kenarda'), bekle, 3)
        if kb3:
            pg.click('#sekmeler button[data-sekme="kumbara"]'); time.sleep(0.5)
            pg.screenshot(path=f'{cikti}/07_tablo_golge.png')
            pg.click('#sekmeler button[data-sekme="olaylar"]'); time.sleep(0.5)
            pg.screenshot(path=f'{cikti}/08_olaylar.png')
            pg.click('#kumbaraKart [data-eylem="korumaDon"]')
            pg.wait_for_selector('#modal:not([hidden]) [data-m="evet"]', timeout=5000)
            pg.click('#modal [data-m="evet"]')
            kb4 = bekle_kosul(pg, lambda k: not k['koruma'].get('kenarda'), 10, 1)
            kontrol('elle geri dön', kb4 is not None and any('elle geri döndürüldü' in (d.get('donus') or '') for d in kb4['koruma']['donemler']))
    # grafikte kenar bölgesi ve küçük ekran
    pg.set_viewport_size({'width': 1600, 'height': 900}); time.sleep(1)
    pg.screenshot(path=f'{cikti}/09_kucuk_ekran.png')
    pg.set_viewport_size({'width': 2560, 'height': 1440}); time.sleep(0.5)
    kb = kumbara(pg)
    if kb:
        for o in kb['olaylar'][-20:]:
            print('   ', o['tur'], o['mesaj'][:170])
    b.close()
print('KONSOL:', len(hatalar))
for h in hatalar[:40]:
    print('  ', h)
kontrol('konsolda hata yok', not [h for h in hatalar if 'error' in h or 'pageerror' in h])
print('\nSONUÇ:', 'HEPSİ TAMAM' if not sonuclar else f'{len(sonuclar)} HATA: ' + '; '.join(sonuclar))
sys.exit(1 if sonuclar else 0)
