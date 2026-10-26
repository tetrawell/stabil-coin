# Bekçi testi (0.3): gerçek Chromium penceresi (Xvfb sanal ekranında), test verisi, sanal para.
# Programın kendisi pencereyi açar (sahte "chromium-browser" betiği gerçek Chromium'u çalıştırır).
# Senaryolar:
#   1) kumbara çalışırken sayfa çöker (pencere haber veremez) → program kapanmaz, 30 sn sonra pencereyi yeniden açar
#      (0.4, Pi: bozuk tarayıcıyı kapatıp temiz açar → tek pencere kalır; pencere ekran kartısız açılır)
#   2) sayfa yenilenir → program kapanmaz
#   3) tarayıcının tamamı çöker → program kapanmaz, pencereyi yeniden açar
#   4) kullanıcı pencereyi kapatır ("çıkmak istiyor musun" onaylanır) → program kapanır (0.2 gibi)
#   5) kumbara yokken sayfa çöker → program kapanır (0.2 gibi)
# kullanım: python3 -I test/bekci_testi.py <çalışma> <al-sat ikilisi (linux)> <kaynak> <chrome yolu>
import json, os, shutil, signal, subprocess, sys, time, urllib.request
from playwright.sync_api import sync_playwright

CAL, IKILI, KAYNAK, CHROME = [os.path.abspath(a) for a in sys.argv[1:5]]
ADRES = 'http://127.0.0.1:47322'
EKRAN = ':77'
hatalar = []


def kontrol(ad, kosul, ek=''):
    print(('TAMAM ' if kosul else 'HATA  ') + ad + (f'  [{ek}]' if ek and not kosul else ''), flush=True)
    if not kosul:
        hatalar.append(ad)


def kur():
    shutil.rmtree(CAL, ignore_errors=True)
    for d in ('bin', 'home', 'veri', 'strat'):
        os.makedirs(os.path.join(CAL, d))
    betik = os.path.join(CAL, 'bin', 'chromium-browser')
    with open(betik, 'w') as f:
        f.write(f'#!/bin/sh\necho "$(date +%T) $*" >> {CAL}/baslatmalar.txt\n'
                f'exec {CHROME} --remote-debugging-port=9333 --no-sandbox "$@"\n')
    os.chmod(betik, 0o755)
    subprocess.run(['node', os.path.join(KAYNAK, 'test', 'hizli_strateji.js'), os.path.join(KAYNAK, 'web'),
                    os.path.join(CAL, 'strat'), 'DOGEUSDT', '1m', '0.25'], check=True, capture_output=True)


def program_ac():
    env = dict(os.environ, PATH=os.path.join(CAL, 'bin') + ':' + os.environ['PATH'], HOME=os.path.join(CAL, 'home'),
               DISPLAY=EKRAN, ALSAT_TEST='1', ALSAT_BEKCI='1', ALSAT_YENIDEN_AC_SN='30',
               ALSAT_KLASOR=os.path.join(CAL, 'strat'), ALSAT_VERI_KLASORU=os.path.join(CAL, 'veri'))
    env.pop('ALSAT_PENCERE_YOK', None)
    return subprocess.Popen([IKILI], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def kayit():
    try:
        return open(os.path.join(CAL, 'veri', 'alsat_kayit.txt'), encoding='utf-8').read().splitlines()
    except FileNotFoundError:
        return []


def bekle_kayit(metin, sure, bas=0):
    son = time.time() + sure
    while time.time() < son:
        for s in kayit()[bas:]:
            if metin in s:
                return s
        time.sleep(0.5)
    return None


def api(yol, govde=None):
    istek = urllib.request.Request(ADRES + yol, data=None if govde is None else json.dumps(govde).encode(),
                                   headers={'Content-Type': 'application/json'}, method='GET' if govde is None else 'POST')
    with urllib.request.urlopen(istek, timeout=10) as r:
        return json.loads(r.read() or b'{}')


def kumbara_baslat():
    oturum = api('/api/oturum', {})['oturum']
    d = next(x for x in api('/api/stratejiler')['dosyalar'] if x['ad'] == 'DOGEUSDT_1m.json')
    return api('/api/kumbara/baslat', {'dosya': d['ad'], 'olusturuldu': d['olusturuldu'], 'mod': 'sanal', 'tutar': 100,
                                       'ilkMum': 0, 'mumSayisi': 300, 'oturum': oturum})


def renderer_oldur():
    # sayfanın çökmesi: sayfa süreçleri öldürülür (sayfa "kapanıyorum" diyemez), tarayıcının kendisi açık kalır
    for p in os.listdir('/proc'):
        if not p.isdigit():
            continue
        try:
            ham = open(f'/proc/{p}/cmdline', 'rb').read()
        except OSError:
            continue
        # Chromium alt süreçlerin komut satırını tek yazıya çevirir; içinde aramak yeterli
        if ham.startswith(CHROME.encode()) and b'--type=renderer' in ham:
            try:
                os.kill(int(p), signal.SIGKILL)
            except OSError:
                pass


def chrome_pidleri(ana=True):
    pidler = []
    for p in os.listdir('/proc'):
        if not p.isdigit():
            continue
        try:
            cmd = open(f'/proc/{p}/cmdline', 'rb').read().split(b'\0')
        except OSError:
            continue
        if cmd and cmd[0].decode(errors='ignore') == CHROME and (not ana or not any(c.startswith(b'--type=') for c in cmd)):
            pidler.append(int(p))
    return pidler


def program_sayfasi(pw):
    tarayici = pw.chromium.connect_over_cdp('http://127.0.0.1:9333')
    for c in tarayici.contexts:
        for s in c.pages:
            if s.url.startswith(ADRES):
                try:
                    s.evaluate('1')
                    return tarayici, s
                except Exception:
                    pass
    return tarayici, None


def sayfa_hedefleri():
    with urllib.request.urlopen('http://127.0.0.1:9333/json/list', timeout=5) as r:
        return [h['id'] for h in json.loads(r.read()) if h.get('type') == 'page' and h.get('url', '').startswith(ADRES)]


def hedef_kapat(hid):
    for yontem in ('PUT', 'GET'):
        try:
            urllib.request.urlopen(urllib.request.Request(f'http://127.0.0.1:9333/json/close/{hid}', method=yontem), timeout=5).read()
            return
        except Exception:
            pass


def calisiyor(p):
    return p.poll() is None


xvfb = subprocess.Popen(['Xvfb', EKRAN, '-screen', '0', '1600x900x24'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
time.sleep(1.5)
try:
    kur()
    with sync_playwright() as pw:
        # ---------- 1. çalıştırma: kumbara var ----------
        p = program_ac()
        kontrol('program pencereyi açtı ve pencere bağlandı', bekle_kayit('Pencere hattı açıldı', 60) is not None)
        bas = open(os.path.join(CAL, 'baslatmalar.txt')).read()
        if 'PI_SURUMU' in os.environ:
            kontrol('0.6: pencere ekran kartıyla açılıyor (--disable-gpu yok)', '--disable-gpu' not in bas, bas[-300:])
        r = kumbara_baslat()
        kontrol('sanal kumbara başladı', 'kumbara' in r or 'id' in json.dumps(r), json.dumps(r)[:200])
        time.sleep(5)

        # 1) sayfa çöker
        n = len(kayit())
        eski_hedefler = sayfa_hedefleri()
        # Simülatörün tarayıcısı yerine geçen, BAŞKA profille açık bir Chromium: bekçi buna asla dokunmamalı
        sim = subprocess.Popen([CHROME, '--headless=new', '--no-sandbox', '--remote-debugging-port=9334',
                                '--user-data-dir=' + os.path.join(CAL, 'sim_profil'), 'about:blank'],
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        time.sleep(3)
        eski_tarayici = [x for x in chrome_pidleri() if x != sim.pid]
        renderer_oldur()
        kontrol('1) sayfa çöktü, hat koptu', bekle_kayit('Pencere hattı kapandı', 20, n) is not None)
        kontrol('1) program kapanmadı, bekliyor', bekle_kayit('kumbara çalıştığı için program açık kalıyor', 35, n) is not None and calisiyor(p))
        kontrol('1) 30 sn sonra pencereyi yeniden açtı', bekle_kayit('yeniden açılıyor', 30, n) is not None)
        kontrol('1) pencere geri geldi', bekle_kayit('Pencere geri geldi', 40, n) is not None and calisiyor(p))
        if 'PI_SURUMU' in os.environ:
            kontrol('1) 0.4: bozuk pencerenin tarayıcısı kapatıldı', bekle_kayit('Bozuk pencerenin tarayıcısı kapatılıyor', 5, n) is not None
                    and not any(os.path.exists(f'/proc/{pid}') and open(f'/proc/{pid}/stat').read().split()[2] != 'Z' for pid in eski_tarayici), str(eski_tarayici))
            kontrol('1) 0.4: bu kapatma "kullanıcı kapattı" sayılmadı', bekle_kayit('temiz pencere açmak için kapatıldı', 5, n) is not None
                    and not any('çıkılıyor' in x for x in kayit()[n:]))
            time.sleep(2)
            kontrol('1) 0.4: pencereler birikmedi (tek pencere var)', len(sayfa_hedefleri()) == 1, str(sayfa_hedefleri()))
        kontrol('1) başka profildeki Chromium (Simülatör gibi) kapatılmadı', sim.poll() is None)
        sim.kill()
        sim.wait(5)
        for hid in eski_hedefler:  # çöken eski pencereyi kapat (kullanıcı da kapatırdı)
            hedef_kapat(hid)

        # 2) sayfa yenilenir
        time.sleep(3)
        n = len(kayit())
        tarayici, sayfa = program_sayfasi(pw)
        sayfa.reload()
        tarayici.close()
        kontrol('2) yenilemede sayfa haber verdi', bekle_kayit('kapatılıyor ya da yenileniyor', 15, n) is not None)
        kontrol('2) yenilenen sayfa bağlandı', bekle_kayit('Pencere hattı açıldı', 15, n) is not None)
        time.sleep(30)
        kontrol('2) yenilemeden sonra program açık', calisiyor(p) and not any('çıkılıyor' in s for s in kayit()[n:]))

        # 3) tarayıcının tamamı çöker
        n = len(kayit())
        for pid in chrome_pidleri():
            os.kill(pid, signal.SIGKILL)
        kontrol('3) tarayıcı çöktü (program bunu "kapatıldı" saymadı)', bekle_kayit('beklenmedik şekilde kapandı', 20, n) is not None)
        kontrol('3) program kapanmadı, bekliyor', bekle_kayit('kumbara çalıştığı için program açık kalıyor', 35, n) is not None and calisiyor(p))
        kontrol('3) pencere yeniden açıldı ve geri geldi', bekle_kayit('Pencere geri geldi', 75, n) is not None and calisiyor(p))

        # 4) kullanıcı pencereyi kapatır ve "çıkmak istiyor musun"u onaylar
        time.sleep(3)
        n = len(kayit())
        with urllib.request.urlopen('http://127.0.0.1:9333/json/list', timeout=5) as r:
            hedef = next(h for h in json.loads(r.read()) if h.get('type') == 'page' and h.get('url', '').startswith(ADRES))
        cikti = subprocess.run(['node', os.path.join(KAYNAK, 'test', 'pencere_kapat.js'), hedef['webSocketDebuggerUrl']],
                               capture_output=True, text=True, timeout=20).stdout.strip()
        sorduMu = [cikti]
        kontrol('4) kumbara varken kapatırken "çıkmak istiyor musun" soruldu', 'beforeunload' in sorduMu, str(sorduMu))
        kontrol('4) sayfa kapanırken haber verdi', bekle_kayit('kapatılıyor ya da yenileniyor', 15, n) is not None)
        son = bekle_kayit('çıkılıyor', 40, n)
        try:
            p.wait(10)
        except subprocess.TimeoutExpired:
            pass
        kontrol('4) program kapandı (pencere kapatıldı)', son is not None and 'kapatıldı' in son and not calisiyor(p), str(son))
        for pid in chrome_pidleri():
            os.kill(pid, signal.SIGKILL)
        time.sleep(2)

        if calisiyor(p):
            p.kill()
            p.wait(5)
        # ---------- 2. çalıştırma: kumbara yok ----------
        # durum.json'daki kumbarayı kaldır (temiz başla)
        shutil.rmtree(os.path.join(CAL, 'veri', 'kayitlar'), ignore_errors=True)
        n = len(kayit())
        p = program_ac()
        kontrol('5) program açıldı, pencere bağlandı', bekle_kayit('Pencere hattı açıldı', 60, n) is not None)
        time.sleep(3)
        n = len(kayit())
        renderer_oldur()
        son = bekle_kayit('çıkılıyor', 40, n)
        try:
            p.wait(10)
        except subprocess.TimeoutExpired:
            pass
        kontrol('5) kumbara yokken susan pencerede program kapandı (0.2 gibi)', son is not None and 'kumbara yok' in son and not calisiyor(p), str(son))
        for pid in chrome_pidleri():
            os.kill(pid, signal.SIGKILL)
finally:
    xvfb.terminate()

print('\n--- kayıt defterinin sonu ---')
for s in kayit()[-40:]:
    print(s)
print('\nSONUÇ:', 'HEPSİ TAMAM' if not hatalar else f'{len(hatalar)} HATA: ' + '; '.join(hatalar))
sys.exit(1 if hatalar else 0)
