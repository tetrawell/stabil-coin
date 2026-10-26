# İki programın aynı veride aynı kararları verdiğini dener.
# 1) Simülatörde (sahte MEXC ile) ayar bulunur, uygulanır, "Dışa aktar" ile dosya yazılır.
# 2) Kripto Al-Sat aynı klasörden dosyayı okur.
# 3) İki program aynı anda yüklenir; işlem tabloları karşılaştırılır. Sonra canlıda birkaç mum beklenip tekrar karşılaştırılır.
# kullanım: python3 -I esitlik_testi.py <scratch> <alsat_ikili> <bekleme_sn>
import sys, os, time, json, subprocess, shutil
from playwright.sync_api import sync_playwright
S, ALSAT, BEKLE = sys.argv[1], sys.argv[2], int(sys.argv[3])
STRAT = os.path.join(S, 'sim_bin', 'stratejiler')
VERI = os.path.join(S, 'veri_esit')
shutil.rmtree(VERI, ignore_errors=True)
os.makedirs(VERI, exist_ok=True)
EKRAN = os.path.join(S, 'ekran')

def tablo(pg):
    return pg.eval_on_selector_all('#islemTablo tbody tr', 'rs => rs.map(r => Array.from(r.cells).map(c => c.innerText.replace(/\\s+/g, " ").trim()))')

def karsilastir(ad, a, b):
    # açık işlemin sonucu (son mum dolarken değişir) ve süresi karşılaştırılmaz
    def temiz(t):
        return [r[:4] if (len(r) > 2 and 'coinde' in r[2]) else r for r in t]
    ta, tb = temiz(a), temiz(b)
    ok = ta == tb
    print(f'{ad}: simülatör {len(a)} satır, al-sat {len(b)} satır -> {"AYNI" if ok else "FARKLI"}')
    if not ok:
        for i, (x, y) in enumerate(zip(ta, tb)):
            if x != y:
                print('  ilk fark satır', i, '\n   sim  :', x, '\n   alsat:', y)
                break
    return ok

def dakika_basi_bekle(ms, pay=6):
    # mum sınırına yakın yükleme yapma: sınırdan sonra 'pay' saniye bekle
    simdi = time.time()
    kalan = (ms / 1000) - (simdi % (ms / 1000))
    if kalan < 20:
        time.sleep(kalan + pay)

hatalar = []
with sync_playwright() as p:
    br = p.chromium.launch()
    sim = br.new_page(viewport={'width': 1800, 'height': 1000})
    sim.on('pageerror', lambda e: hatalar.append(f'sim pageerror: {e}'))
    sim.goto('http://127.0.0.1:47321/')
    sim.wait_for_function("document.querySelectorAll('#islemTablo tbody tr').length > 0", timeout=30000)

    def sim_yukle(sembol, aralik, adet):
        if sim.input_value('#coinAra') != sembol:
            sim.click('#coinAra'); sim.fill('#coinAra', sembol); sim.keyboard.press('Enter')
            time.sleep(1.5)
        sim.click(f'#aralikSeg button[data-v="{aralik}"]')
        time.sleep(1.0)
        sim.select_option('#mumAdet', str(adet))
        time.sleep(1.0)
        sim.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)

    def sim_bul_ve_aktar():
        sim.click('#enKarliBul')
        sim.wait_for_function("(() => { const b = document.querySelector('#bulucu'); return b && !b.hidden && !/aranıyor/.test(b.innerText); })()", timeout=240000)
        uygula = sim.query_selector('#bulucu [data-eylem="uygula"]')
        if uygula:
            uygula.click(); time.sleep(1.0)
        print('bulucu:', sim.inner_text('#bulucu h3'))
        sim.click('#disariAktar')
        sim.wait_for_function("document.querySelector('#bildirim').innerText.includes('Kaydedildi')", timeout=10000)
        print('dışa aktarıldı:', sim.inner_text('#bildirim'))

    # 1) iki strateji hazırla
    sim_yukle('BTCUSDT', '15m', 1000); sim_bul_ve_aktar()
    sim_yukle('AGTUSDT', '1m', 2000); sim_bul_ve_aktar()
    print('dosyalar:', sorted(os.listdir(STRAT)))

    # 2) Al-Sat'ı başlat
    env = dict(os.environ, ALSAT_MEXC_ADRES='http://127.0.0.1:47400', ALSAT_PENCERE_YOK='1', ALSAT_VERI_KLASORU=VERI, ALSAT_KLASOR=STRAT, ALSAT_SILME_YOK='1')
    alsat_proc = subprocess.Popen([ALSAT], env=env)
    time.sleep(1.5)
    al = br.new_page(viewport={'width': 2560, 'height': 1440})
    al.on('pageerror', lambda e: hatalar.append(f'alsat pageerror: {e}'))
    al.on('console', lambda m: hatalar.append(f'alsat {m.type}: {m.text}') if m.type == 'error' else None)
    al.goto('http://127.0.0.1:47322/')
    al.wait_for_selector('.skart', timeout=15000)

    sonuc = True
    for sembol, aralik, adet, ms in [('BTCUSDT', '15m', 1000, 900000), ('AGTUSDT', '1m', 2000, 60000)]:
        ad = f'{sembol}_{aralik}.json'
        al.click(f'.skart[data-ad="{ad}"]')
        al.wait_for_function("document.querySelectorAll('#islemTablo tbody tr').length > 0", timeout=30000)
        sim_yukle(sembol, aralik, adet)
        dakika_basi_bekle(min(ms, 60000))
        # aynı anda yükle
        sim.click('#getir'); al.click('#bastanYukle')
        time.sleep(4)
        sim.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
        al.wait_for_selector('#yukleniyor', state='hidden', timeout=30000)
        a, b = tablo(sim), tablo(al)
        sonuc &= karsilastir(f'{sembol} {aralik} yükleme', a, b)
        print('  sim mum:', sim.inner_text('#konumSayi'), '| alsat:', al.inner_text('#seridiMum'))
        print('  sim özet:', sim.inner_text('.ozet-kart .ozet-deger').split('\n')[0], '| alsat özet:', al.inner_text('.ozet-kart .ozet-deger').split('\n')[0])
        al.screenshot(path=f'{EKRAN}/esit_{sembol}_{aralik}_alsat.png')
        sim.screenshot(path=f'{EKRAN}/esit_{sembol}_{aralik}_sim.png')

    # 3) canlı: AGTUSDT 1 dk; simülatörde Canlı izle açılır, Al-Sat zaten canlı
    sim.click('#canli')
    print(f'canlı bekleniyor ({BEKLE} sn)…')
    time.sleep(BEKLE)
    a, b = tablo(sim), tablo(al)
    sonuc &= karsilastir('AGTUSDT 1m canlı sonrası', a, b)
    print('  sim mum:', sim.inner_text('#konumSayi'), '| alsat:', al.inner_text('#seridiMum'), '|', al.inner_text('#sonKarar'))
    al.screenshot(path=f'{EKRAN}/esit_canli_alsat.png')
    br.close()
    alsat_proc.terminate()
print('SONUÇ:', 'HEPSİ AYNI' if sonuc else 'FARK VAR')
print('HATALAR:', len(hatalar))
for h in hatalar[:20]:
    print('  ', h)
