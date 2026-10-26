/* UYGULAMA — Kripto Al-Sat ekranı (sürüm 0.2)
 * Solda strateji listesi, ortada grafik, sağda kumbara, simülatörün motoru ve dosyadan gelen ayarlar.
 * Kararları simülatörün motoru verir (gostergeler.js + kurallar.js + motor.js, simülatördekinin birebir kopyası).
 * Her çalışan kumbara için bir "izleyici" mumları getirir, motoru çalıştırır ve her mum kapanışında kararı
 * programa bildirir; emirleri program yürütür (limit → 15 sn → piyasa). Grafik seçili stratejinin izleyicisini gösterir:
 * kumbarası varsa kumbaranınkini (aynı mumlar, aynı başlangıç), yoksa bir önizlemeyi.
 * Grafik, çizimler ve açıklamalar Kripto Simülatör 3.2'nin uygulama.js'inden alındı. İki program aynı görünsün
 * diye o kısımları değiştirirken simülatördekine benzer tut.
 */
(function () {
  'use strict';
  const LW = window.LightweightCharts;
  const $ = s => document.querySelector(s);
  const kokStil = getComputedStyle(document.documentElement);
  const tok = n => kokStil.getPropertyValue(n).trim();
  const RENK = {
    yukari: tok('--yukari'), asagi: tok('--asagi'), vurgu: tok('--vurgu'), mavi: tok('--mavi'), turuncu: tok('--turuncu'),
    camgobegi: tok('--camgobegi'), mor: tok('--mor'), pembe: tok('--pembe'),
    metin: tok('--metin'), metin2: tok('--metin2'), metin3: tok('--metin3'), panel: tok('--panel'), panel2: tok('--panel2'),
    panel3: tok('--panel3'), cizgi: tok('--cizgi'), cizgi2: tok('--cizgi2'), zemin: tok('--zemin'),
  };
  function rgba(hex, a) {
    const h = hex.replace('#', '');
    const n = parseInt(h.length === 3 ? h.split('').map(x => x + x).join('') : h, 16);
    return `rgba(${(n >> 16) & 255},${(n >> 8) & 255},${n & 255},${a})`;
  }
  RENK.yesil = RENK.yukari; RENK.kirmizi = RENK.asagi; RENK.gri = RENK.metin3; RENK.soluk = rgba(RENK.metin2, 0.22);
  const YER_RENK = { baslat: RENK.mavi, tetik: RENK.yukari, vazgec: RENK.mor, sat: RENK.asagi };
  const VITES_RENK = ['#ee5a5e', '#f2967a', RENK.metin2, '#6fd3ad', '#25b387']; // sert düşüş … sert yükseliş
  VITES_RENK.forEach((r, j) => { RENK['v' + j] = r; });
  const VITES_ZEMIN = [0.13, 0.06, 0, 0.06, 0.13]; // grafikte zemin rengi kalınlığı
  const GOSTERGE_RENK = { ortalama: ['vurgu'], ema: ['camgobegi', 'mor'], fbb: ['gri', 'soluk'], vwap: ['pembe'], vites: ['v0', 'v1', 'v2', 'v3', 'v4'] };
  const KURAL = Object.fromEntries(KURALLAR.map(k => [k.id, k]));
  const GOSTERGE = Object.fromEntries(GOSTERGELER.map(g => [g.id, g]));

  // ---------- biçimler (simülatörle aynı) ----------
  const TR = 'tr-TR';
  const nfOnbellek = {};
  const nf = d => nfOnbellek[d] || (nfOnbellek[d] = new Intl.NumberFormat(TR, { minimumFractionDigits: d, maximumFractionDigits: d }));
  const nfEn = d => nfOnbellek['e' + d] || (nfOnbellek['e' + d] = new Intl.NumberFormat(TR, { minimumFractionDigits: 0, maximumFractionDigits: d }));
  let fiyatOnd = 2;
  const Bicim = {
    fiyat: x => (x == null || !isFinite(x) ? '–' : nf(fiyatOnd).format(x)),
    para: x => (x == null || !isFinite(x) ? '–' : nf(2).format(x) + ' $'),
    paraIsaret: x => {
      const r = Math.round(x * 100) / 100;
      return (r > 0 ? '+' : r < 0 ? '−' : '') + nf(2).format(Math.abs(r)) + ' $';
    },
    yuzde: (x, d = 2) => {
      const r = Math.round(x * 10 ** d) / 10 ** d;
      return (r > 0 ? '+' : r < 0 ? '−' : '') + '%' + nf(d).format(Math.abs(r));
    },
  };
  window.Bicim = Bicim;
  const f = x => Bicim.fiyat(x);
  const isaretSinif = x => (x > 0.000001 ? 'pos' : x < -0.000001 ? 'neg' : '');
  const AYLAR = ['Oca', 'Şub', 'Mar', 'Nis', 'May', 'Haz', 'Tem', 'Ağu', 'Eyl', 'Eki', 'Kas', 'Ara'];
  const iki = n => String(n).padStart(2, '0');
  function tarih(ms) {
    const d = new Date(ms);
    const gun = `${d.getDate()} ${AYLAR[d.getMonth()]}`;
    return S.aralik === '1d' ? `${gun} ${d.getFullYear()}` : `${gun} ${iki(d.getHours())}:${iki(d.getMinutes())}`;
  }
  function tarihUTC(sn) {
    const d = new Date(sn * 1000);
    const gun = `${d.getUTCDate()} ${AYLAR[d.getUTCMonth()]} ${String(d.getUTCFullYear()).slice(2)}`;
    return S.aralik === '1d' ? gun : `${gun}  ${iki(d.getUTCHours())}:${iki(d.getUTCMinutes())}`;
  }
  const ARALIK = {
    '1m': { ms: 60e3, ad: '1 dk' }, '5m': { ms: 300e3, ad: '5 dk' }, '15m': { ms: 900e3, ad: '15 dk' },
    '1h': { ms: 3600e3, ad: '1 sa' }, '4h': { ms: 14400e3, ad: '4 sa' }, '1d': { ms: 86400e3, ad: '1 gün' },
  };
  const aralikAd = a => (ARALIK[a] ? ARALIK[a].ad : '?');
  const ENFAZLA_MUM = 30000; // 0.8: programın (veri.go → enFazlaMum) bir seferde verdiği en fazla mum
  function sureYaz(ms) {
    const dk = Math.round(ms / 60000);
    const g = Math.floor(dk / 1440), s = Math.floor((dk % 1440) / 60), d = dk % 60;
    const p = [];
    if (g) p.push(g + ' gün');
    if (s) p.push(s + ' sa');
    if (d && !g) p.push(d + ' dk');
    return p.join(' ') || '0 dk';
  }
  const metin = n => (typeof n === 'function' ? n() : n || '');
  const ondalik = adim => { const s = String(adim); return s.includes('.') ? s.split('.')[1].length : 0; };
  const degerMetni = (x, a) => (a.tam ? String(Math.round(x)) : nfEn(ondalik(a.adim)).format(x));

  // ---------- Al-Sat'a özel biçimler ----------
  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  function gunSaat(ms) {
    if (!isFinite(ms) || !ms) return '–';
    const d = new Date(ms);
    return `${d.getDate()} ${AYLAR[d.getMonth()]} ${iki(d.getHours())}:${iki(d.getMinutes())}`;
  }
  const saatYaz = ms => { const d = new Date(ms); return `${iki(d.getHours())}:${iki(d.getMinutes())}`; };
  const saatSnYaz = ms => { const d = new Date(ms); return `${iki(d.getHours())}:${iki(d.getMinutes())}:${iki(d.getSeconds())}`; };
  // Kalan süre: "1 sa 12 dk", "3 gün 4 sa", "45 sn"
  function kalanYaz(ms) {
    if (ms <= 0) return 'doldu';
    const sn = Math.ceil(ms / 1000);
    const g = Math.floor(sn / 86400), s = Math.floor((sn % 86400) / 3600), d = Math.floor((sn % 3600) / 60), x = sn % 60;
    if (g) return `${g} gün${s ? ' ' + s + ' sa' : ''}`;
    if (s) return `${s} sa ${d} dk`;
    if (d) return `${d} dk${d < 10 && x ? ' ' + x + ' sn' : ''}`;
    return `${x} sn`;
  }
  // Toplam ömür: "2 saat", "1 gün", "14 gün"
  function sureMetni(ms) {
    const dk = Math.round(ms / 60000);
    if (dk % 1440 === 0) return `${dk / 1440} gün`;
    if (dk % 60 === 0) return `${dk / 60} saat`;
    return sureYaz(ms);
  }
  const yuzdeHTML = x => (isFinite(x) ? `<b class="${isaretSinif(x)}">${Bicim.yuzde(x, 1)}</b>` : '–');
  // 0.8: dosyadan gelen sayılar sayı olarak yazılır (dosyada sayı yerine yazı varsa sayfaya HTML olarak girmesin)
  const adetYaz = x => (x !== null && x !== '' && typeof x !== 'boolean' && isFinite(+x) ? nf(0).format(+x) : '–');
  const usdt = (x, d = 2) => (x == null || !isFinite(x) ? '–' : nf(d).format(x) + ' USDT');
  const usdtIsaret = x => {
    const r = Math.round(x * 100) / 100;
    return (r > 0 ? '+' : r < 0 ? '−' : '') + nf(2).format(Math.abs(r)) + ' USDT';
  };
  // Fiyat, grafikten bağımsız (liste ve kumbara için): büyüklüğe göre ondalık, grafikteki kuralla aynı
  const fiyatOndalik = p => (p >= 1000 ? 2 : p >= 10 ? 3 : p >= 1 ? 4 : Math.min(10, Math.ceil(-Math.log10(p)) + 4));
  const fg = x => (x == null || !isFinite(x) || x <= 0 ? '–' : nf(fiyatOndalik(x)).format(x));
  const miktarYaz = x => (x == null || !isFinite(x) ? '–' : nfEn(x >= 100 ? 2 : x >= 1 ? 4 : 8).format(x));
  const ort = a => (a.length ? a.reduce((s, x) => s + x, 0) / a.length : null);

  // ---------- durum ----------
  const S = {
    borsa: 'mexc', sembol: '', aralik: '15m',
    gorunum: { kutular: true, dip: true, cizgiler: true, gostergeler: true, gosterge: { ortalama: true, ema: true, fbb: true, vwap: true, vites: true },
      akis: true, ao: { yapi: true, ic: false, bolge: true, akis: true, buyuk: true } }, // 0.9: akış onayı çizimleri
    secili: null, // seçili strateji dosyasının adı
  };
  let LISTE = null;  // sunucudan gelen klasör bilgisi ve dosyalar
  let AKTIF = null;  // seçili strateji: { ad, dosya, kumbaraId, iz, icerik, strateji, kayboldu }
  let M = null, T = [], zamanIdx = new Map(), utcZaman = false;
  let R = null, k = 0, surum = 0;
  let secili = null, seciliIslem = null;
  let ses = null;
  let oturum = null, olayKaynagi = null, pencereAktif = true;
  let DURUM = null, dsSon = -1, durumIstekte = false, durumTekrar = false;
  const IZ = new Map();      // çalışan kumbara kimliği → izleyici
  let GZ = null;             // grafiğe bağlı izleyici
  const olaySon = new Map(); // kumbara kimliği → bildirimi yapılan son olayın zamanı
  let sekme = null;          // alt tablo: kumbara | simulator | olaylar (null: kendiliğinden)
  let baslatiliyor = null;   // başlatılmakta olan dosyanın adı
  let borsaFarki = 0;        // MEXC saati − bilgisayar saati (ms); mum kapanışları MEXC saatiyle beklenir
  const borsaSimdi = () => Date.now() + borsaFarki;
  const strateji = () => AKTIF.strateji;

  // ---------- strateji yardımcıları (motor.js ile aynı mantık) ----------
  const surumu = s => (typeof s.surum === 'number' ? s.surum : 1);
  const acikMi = (kr, s) => (s.kurallar[kr.id] ? !!s.kurallar[kr.id].acik : !!kr.acik);
  const kuralAyari = (kr, s) => Motor.ayarDegerleri(kr.ayarlar, s.kurallar[kr.id] && s.kurallar[kr.id].d, surumu(s));
  const gostergeAyari = (g, s) => Motor.ayarDegerleri(g.ayarlar, s.gostergeler && s.gostergeler[g.id]);

  // Al-Sat'ın motoru bu stratejiyi simülatörün çalıştırdığı gibi çalıştırabilir mi?
  function uyumKontrol(icerik) {
    const s = icerik && icerik.strateji;
    if (!s || typeof s !== 'object' || !s.kurallar || typeof s.kurallar !== 'object') return { ok: false, hata: 'Dosyada kurallar yok.' };
    const sv = surumu(s);
    if (sv > STRATEJI_SURUMU) {
      return { ok: false, hata: `Bu strateji simülatörün daha yeni bir sürümüyle yapılmış (strateji sürümü ${sv}). Al-Sat'ın motoru sürüm ${STRATEJI_SURUMU}'e kadar okuyabiliyor; Al-Sat'ı güncellemek gerekir.` };
    }
    const bilinmeyen = Object.keys(s.kurallar).filter(id => s.kurallar[id] && s.kurallar[id].acik && !KURAL[id]);
    if (bilinmeyen.length) {
      return { ok: false, hata: `Dosyada Al-Sat'ın bilmediği açık kural var (${bilinmeyen.join(', ')}). Simülatöre yeni kural eklenmiş olabilir; Al-Sat'ı güncellemek gerekir.` };
    }
    return { ok: true };
  }
  // Motora verilecek kopya. "para" sadece sanal hesabı etkiler, kararları değil; dosyada yoksa simülatörün varsayılanı.
  function calisanStrateji(s) {
    const c = JSON.parse(JSON.stringify(s));
    if (!c.para || typeof c.para !== 'object') c.para = {};
    if (!(c.para.baslangic > 0)) c.para.baslangic = 1000;
    if (!(c.para.masraf >= 0)) c.para.masraf = 0.1;
    return c;
  }
  // Simülatörün ayarı bulurken kullandığı mum sayısı (aynı sayıyla yüklenince iki programın grafiği aynı olur)
  function mumSayisiBul(icerik) {
    const n = icerik && icerik.gecmisSonuc && icerik.gecmisSonuc.mumSayisi;
    return Number.isFinite(n) && n >= 30 ? Math.min(ENFAZLA_MUM, Math.round(n)) : 1000;
  }

  // ---------- kumbara yardımcıları ----------
  const aktifKumbaralar = () => (DURUM ? DURUM.kumbaralar.filter(x => x.durum !== 'bitti') : []);
  const dosyaKumbarasi = ad => aktifKumbaralar().find(x => x.dosya === ad) || null;
  const kumbaraKaydi = id => (DURUM && id ? DURUM.kumbaralar.find(x => x.id === id) || null : null);
  const seciliKumbara = () => (AKTIF && AKTIF.kumbaraId ? kumbaraKaydi(AKTIF.kumbaraId) : null);
  function sonBiten(ad) {
    if (!DURUM) return null;
    let son = null;
    for (const x of DURUM.kumbaralar) if (x.dosya === ad && x.durum === 'bitti' && (!son || x.bitisZamani > son.bitisZamani)) son = x;
    return son;
  }
  // Tablo, olaylar ve grafikteki dolum işaretleri için: çalışan kumbara, yoksa bu dosyanın son kapanan kumbarası
  const gorunenKumbara = () => seciliKumbara() || (AKTIF ? sonBiten(AKTIF.ad) : null);

  // ---------- koruma (0.7): kenara alma ve gölge takip ----------
  // Sınırları program (Go) hesaplar; sayfa ayarın geçmiş işlemlerini (simülatörün motoruyla) gönderir ve sonucu gösterir.
  const HASSASIYET_AD = { sakin: 'Sakin', dengeli: 'Dengeli', hassas: 'Hassas' };
  const HASSASIYET_NOT = {
    sakin: '100 olası geleceğin 99\'unda görülmeyen kötülükte kenara alır. En geç alarm verir.',
    dengeli: '100 olası geleceğin 95\'inde görülmeyen kötülükte kenara alır.',
    hassas: '100 olası geleceğin 90\'ında görülmeyen kötülükte kenara alır. En erken alarm verir; arada boşuna durabilir.',
  };
  const kisaYuzde = x => '%' + nfEn(1).format(x);
  function sureUzun(ms) {
    const dk = ms / 60000;
    if (dk < 90) return Math.round(dk) + ' dk';
    if (dk < 2880) return nfEn(1).format(Math.round(dk / 6) / 10) + ' sa';
    return nfEn(1).format(Math.round(dk / 144) / 10) + ' gün';
  }
  function hizMetni(saatte) {
    if (saatte >= 1) return `saatte ~${nfEn(1).format(saatte)} işlem`;
    if (saatte * 24 >= 1) return `günde ~${nfEn(1).format(saatte * 24)} işlem`;
    return `haftada ~${nfEn(1).format(saatte * 168)} işlem`;
  }
  // Ayarın geçmiş işlemleri: kumbaranın açılışından önce kapananlar (koruma sınırlarını bunlardan hesaplar)
  function korumaGecmisi(iz, baslangic) {
    if (!iz || !iz.R || !iz.M || !iz.M.t.length) return null;
    const R = iz.R, M = iz.M, ms = iz.ms, sinir = baslangic + borsaFarki;
    const islemler = [];
    for (const t of R.islemler) {
      if (t.cikisIdx == null || M.t[t.cikisIdx] + ms > sinir || !isFinite(t.yuzde)) continue;
      islemler.push({ g: M.t[t.girisIdx], c: M.t[t.cikisIdx], y: t.yuzde });
    }
    const i0 = Math.min(Math.max(0, R.i0 || 0), M.t.length - 1);
    return { islemler, sure: Math.max(0, Math.min(sinir, M.t[M.t.length - 1] + ms) - M.t[i0]) };
  }
  function karakterHTML(kar) {
    if (!kar) return '';
    if (kar.yedek) return kar.islem ? `Geçmişte sadece <b>${kar.islem} işlem</b> var; hesap için az olduğundan yedek sınırlar kullanılıyor (hassasiyet bunları değiştirmez).`
      : 'Ayarın geçmişi henüz okunmadı; şimdilik yedek sınırlar kullanılıyor.';
    const oran = kar.islem ? Math.round((kar.karli / kar.islem) * 100) : 0;
    return `Bu ayarın geçmişi: <b>${kar.islem} işlem</b>, %${oran}'i kârlı, ${hizMetni(kar.saatteIslem)}${kar.sure > 0 ? ` · son ${sureUzun(kar.sure)}` : ''}`;
  }
  function korumaKurallarHTML(s, kayipEtkin, kayipKaynak, kapali) {
    const p = [];
    if (!kapali.seri && s.seri > 0) p.push(`art arda <b>${s.seri} zararda</b>`);
    if (kayipEtkin > 0) p.push(`<b>${kisaYuzde(kayipEtkin)} kayıpta</b>${kayipKaynak === 'tavan' ? ` (senin tavanın${!kapali.kayip ? `; ayarın kendi sınırı ${kisaYuzde(s.kayip)}` : ''})` : ''}`);
    if (!kapali.hiz && s.hizSayi > 0 && s.hizPencere > 0) p.push(`<b>${sureUzun(s.hizPencere)} içinde ${s.hizSayi} işlem</b> yapıp zarardaysa`);
    return p.length ? p.join(' · ') : 'hiçbir alarm açık değil';
  }
  function saglamlikHTML(sg) {
    if (!sg) return '';
    if (sg.yok) return `<div class="saglamlik"><b>Sağlamlık testi:</b> ${esc(sg.yok)}</div>`;
    const kotu = sg.bot < 0, geride = sg.bot < sg.alTut;
    const yorum = kotu ? 'Bu ayar görmediği dönemde zarar etmiş; sadece son günlerin grafiğine uygun olabilir. Küçük tutarla dene.'
      : geride ? 'Görmediği dönemde kâr etmiş ama sadece alıp tutmanın gerisinde kalmış.'
      : 'Görmediği dönemde de kâr etmiş; bu iyi bir işaret (garanti değil).';
    const oran = sg.islem ? Math.round((sg.karli / sg.islem) * 100) : 0;
    return `<div class="saglamlik ${kotu ? 'kotu' : geride ? 'orta' : 'iyi'}"><b>Sağlamlık testi</b> · ayarın görmediği önceki ${nfEn(1).format(sg.gun)} gün:
      bot <b class="${isaretSinif(sg.bot)}">${Bicim.yuzde(sg.bot)}</b> · tutsan ${Bicim.yuzde(sg.alTut)} · ${sg.islem} işlem (%${oran}'i kârlı) · en büyük düşüş ${kisaYuzde(sg.dusus)}
      <span>${yorum}</span></div>`;
  }
  // Sağlamlık testi: simülatörün ayarı bulduğu veriden ÖNCEKİ mumlarda (ayarın hiç görmediği dönem) aynı ayar ne yapardı?
  async function saglamlikTesti(sembol, aralik, strateji, bas) {
    // 0.8: ayarın bulunduğu verinin ilk mumundan ÖNCEKİ mumlar istenir (simülatör 20.000 mum kullanmış olsa da test yapılabilsin)
    const j = await mumGetir(sembol, aralik, 20000, bas);
    const m = { t: [], o: [], h: [], l: [], c: [], v: [], acikSon: false };
    for (const x of j.mumlar || []) {
      if (x[0] >= bas) break;
      m.t.push(x[0]); m.o.push(x[1]); m.h.push(x[2]); m.l.push(x[3]); m.c.push(x[4]); m.v.push(x[5]);
    }
    if (m.c.length < 200) return { yok: `ayarın görmediği yeterli eski veri yok (${m.c.length} mum; coin yeni olabilir).` };
    const R = Motor.calistir(m, strateji, KURALLAR);
    const i0 = R.i0, son = m.c.length - 1;
    if (!(son - i0 >= 100)) return { yok: 'ayarın görmediği dönem, göstergelerin ısınmasına yetmiyor.' };
    let tepe = R.ozk[i0], dusus = 0;
    for (let i = i0; i <= son; i++) { tepe = Math.max(tepe, R.ozk[i]); dusus = Math.max(dusus, 1 - R.ozk[i] / tepe); }
    const kapali = R.islemler.filter(t => t.cikisIdx != null);
    return {
      gun: (m.t[son] + ARALIK[aralik].ms - m.t[i0]) / 86400000,
      bot: (R.ozk[son] / R.ozk[i0] - 1) * 100,
      alTut: (R.alTut[son] / R.alTut[i0] - 1) * 100,
      islem: kapali.length, karli: kapali.filter(t => t.net > 0).length, dusus: dusus * 100,
    };
  }
  // Korumasız ya da geçici (yedek sınırlı) kumbaraya ayarın geçmişinden asıl korumayı kurdurur: eski sürümden kalan,
  // yeni ayarla devam eden ya da başlatılırken grafiği yüklenmemiş kumbara. Olmazsa 30 sn'de bir yeniden denenir.
  async function korumaBildir(iz) {
    if (!iz.kumbaraId || !oturum || !pencereAktif || iz.korumaMesgul || Date.now() - (iz.korumaSon || 0) < 30000) return;
    const kb = kumbaraKaydi(iz.kumbaraId);
    if (!kb || kb.durum === 'bitti' || (kb.koruma && !kb.koruma.gecici)) return;
    const g = korumaGecmisi(iz, kb.baslangic);
    if (!g) return;
    iz.korumaMesgul = true; iz.korumaSon = Date.now();
    try {
      const r = await fetch('/api/kumbara/koruma', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, id: iz.kumbaraId, islemler: g.islemler, sure: g.sure }) });
      await r.json().catch(() => ({})); // cevap her zaman okunur (0.5)
      if (r.ok) durumYenile(true);
    } catch (e) { /* 30 sn sonra yeniden denenir */ } finally { iz.korumaMesgul = false; }
  }
  const acikIslemi = kb => (kb && kb.islemler.length && !kb.islemler[kb.islemler.length - 1].satis ? kb.islemler[kb.islemler.length - 1] : null);
  const modAdi = kb => (kb.mod === 'gercek' ? 'GERÇEK PARA' : 'SANAL PARA');
  const varlikAdi = s => String(s || '').replace(/USDT$/, '');
  const SEBEP_AD = { anlikZarar: 'zarar durdur (anlık)', anlikHedef: 'hedef kâr (anlık)', sureDoldu: 'ömür doldu', elle: 'elle satıldı', motor: 'motor çıktı' };
  const sebepAdi = s => SEBEP_AD[s] || (KURAL[s] ? KURAL[s].ad.toLowerCase() : s || '—');
  const YOL_AD = { limit: 'limit', 'limit+piyasa': 'limit + piyasa', piyasa: 'piyasa', toz: 'satılamadı (çok küçük)', birakildi: 'satılmadan bırakıldı' };
  const ASAMA_AD = { bekle: 'Bekliyor', dipAra: 'Dip arıyor', coinde: 'Coinde' };

  // ---------- ayarlar (exe'nin yanındaki ayarlar.json: görünüm ve seçili strateji) ----------
  let kaydetZaman = null;
  function kaydet() {
    clearTimeout(kaydetZaman);
    kaydetZaman = setTimeout(() => {
      fetch('/api/ayarlar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ gorunum: S.gorunum, secili: S.secili }) }).then(r => r.text()).catch(() => {});
    }, 400);
  }
  function ayarlariYukle(j) {
    if (!j || typeof j !== 'object') return;
    const g = j.gorunum;
    if (g && typeof g === 'object') {
      for (const x of ['kutular', 'dip', 'cizgiler', 'gostergeler']) if (typeof g[x] === 'boolean') S.gorunum[x] = g[x];
      if (g.gosterge) for (const id of Object.keys(S.gorunum.gosterge)) if (typeof g.gosterge[id] === 'boolean') S.gorunum.gosterge[id] = g.gosterge[id];
      if (typeof g.akis === 'boolean') S.gorunum.akis = g.akis; // 0.9
      if (g.ao && typeof g.ao === 'object') for (const id of Object.keys(S.gorunum.ao)) if (typeof g.ao[id] === 'boolean') S.gorunum.ao[id] = g.ao[id];
    }
    if (typeof j.secili === 'string') S.secili = j.secili;
  }

  // ---------- grafikler (simülatörle aynı) ----------
  let anaGrafik, mumSeri, paraGrafik, botSeri, alTutSeri, masrafsizSeri;
  let cizgiListesi = []; // grafikteki gösterge ve kural çizgileri
  const katman = $('#katman');

  function tikYaz(t, tip) {
    const d = new Date(t * 1000);
    if (tip === 0) return String(d.getUTCFullYear());
    if (tip === 1) return AYLAR[d.getUTCMonth()];
    if (tip === 2) return `${d.getUTCDate()} ${AYLAR[d.getUTCMonth()]}`;
    return `${iki(d.getUTCHours())}:${iki(d.getUTCMinutes())}`;
  }
  function grafikSecenek(fiyatYazici) {
    return {
      autoSize: true,
      layout: { background: { type: 'solid', color: RENK.panel }, textColor: RENK.metin2, fontFamily: "'Plex','Segoe UI',sans-serif", fontSize: 12 },
      grid: { vertLines: { color: rgba(RENK.cizgi, 0.55) }, horzLines: { color: rgba(RENK.cizgi, 0.55) } },
      rightPriceScale: { borderColor: RENK.cizgi },
      timeScale: { borderColor: RENK.cizgi, timeVisible: true, secondsVisible: false, rightOffset: 6, tickMarkFormatter: tikYaz },
      crosshair: {
        mode: LW.CrosshairMode.Normal,
        vertLine: { color: RENK.metin3, labelBackgroundColor: RENK.panel3 },
        horzLine: { color: RENK.metin3, labelBackgroundColor: RENK.panel3 },
      },
      localization: { locale: TR, priceFormatter: fiyatYazici, timeFormatter: tarihUTC },
    };
  }
  const fiyatFormati = () => ({ type: 'price', precision: fiyatOnd, minMove: 1 / 10 ** fiyatOnd });

  function grafikleriKur() {
    anaGrafik = LW.createChart($('#anaGrafik'), grafikSecenek(x => Bicim.fiyat(x)));
    anaGrafik.priceScale('right').applyOptions({ scaleMargins: { top: 0.14, bottom: 0.08 } });
    mumSeri = anaGrafik.addCandlestickSeries({
      upColor: RENK.yukari, downColor: RENK.asagi, wickUpColor: RENK.yukari, wickDownColor: RENK.asagi,
      borderVisible: false, priceLineColor: RENK.metin3,
    });
    paraGrafik = LW.createChart($('#paraGrafik'), grafikSecenek(x => nf(2).format(x)));
    paraGrafik.applyOptions({ rightPriceScale: { scaleMargins: { top: 0.12, bottom: 0.08 } } });
    alTutSeri = paraGrafik.addLineSeries({ color: RENK.metin3, lineWidth: 1, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false });
    masrafsizSeri = paraGrafik.addLineSeries({ color: rgba(RENK.metin2, 0.7), lineWidth: 1, lineStyle: LW.LineStyle.Dashed, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false });
    botSeri = paraGrafik.addAreaSeries({ lineColor: RENK.vurgu, lineWidth: 2, topColor: rgba(RENK.vurgu, 0.22), bottomColor: rgba(RENK.vurgu, 0.0), priceLineVisible: false });

    // İki grafik birlikte kayar. Simülatörden farkı: aralıklar zaten aynıysa tekrar ayarlanmaz.
    // (Canlıda sürekli güncellenirken iki grafik birbirini itip grafiği yavaş yavaş sağa kaydırıyordu.)
    let kilit = false;
    const ayni = (a, b) => a && b && Math.abs(a.from - b.from) < 0.05 && Math.abs(a.to - b.to) < 0.05;
    const senk = (kaynak, hedef) => kaynak.timeScale().subscribeVisibleLogicalRangeChange(r => {
      if (kilit || !r || ayni(r, hedef.timeScale().getVisibleLogicalRange())) return;
      if (kaynak.timeScale().width() < 20) return; // 0.9: gizli (genişliği olmayan) grafik diğerlerini bozmasın
      if (kaynak === aoGrafik && aoZamanN <= 0) return; // 0.9: mumları henüz yüklenmemiş akış paneli de
      kilit = true;
      hedef.timeScale().setVisibleLogicalRange(r);
      kilit = false;
    });
    senk(anaGrafik, paraGrafik);
    senk(paraGrafik, anaGrafik);
    aoGrafikKur(); // 0.9: akış paneli (delta, CVD, açık pozisyon) da birlikte kayar
    senk(anaGrafik, aoGrafik); senk(aoGrafik, anaGrafik);
    senk(paraGrafik, aoGrafik); senk(aoGrafik, paraGrafik);

    anaGrafik.subscribeCrosshairMove(p => { const i = p && p.time != null ? zamanIdx.get(p.time) : null; lejantYaz(i); aoLejant(i); });
    const tikla = p => { if (p && p.time != null && zamanIdx.has(p.time)) incele(zamanIdx.get(p.time)); };
    anaGrafik.subscribeClick(tikla);
    paraGrafik.subscribeClick(tikla);

    new ResizeObserver(katmanBoyutla).observe($('#grafikKutu'));
    katmanBoyutla();
  }

  function katmanBoyutla() {
    const kutu = $('#grafikKutu');
    const dpr = window.devicePixelRatio || 1;
    katman.width = Math.round(kutu.clientWidth * dpr);
    katman.height = Math.round(kutu.clientHeight * dpr);
    katman.style.width = kutu.clientWidth + 'px';
    katman.style.height = kutu.clientHeight + 'px';
    sonImza = '';
  }

  const mumVeri = i => ({ time: T[i], open: M.o[i], high: M.h[i], low: M.l[i], close: M.c[i] });
  function cizgiVeri(c, i) {
    const v = c.degerler[i];
    if (!isFinite(v)) return { time: T[i] };
    if (c.durumlar) { const d = c.durumlar[i]; return { time: T[i], value: v, color: d >= 0 ? VITES_RENK[d] : RENK.metin3 }; }
    return { time: T[i], value: v };
  }
  const aralikDizi = (n, fn) => { const a = new Array(n); for (let i = 0; i < n; i++) a[i] = fn(i); return a; };

  function cizgileriKur() {
    const istenen = [];
    if (R && S.gorunum.gostergeler) {
      for (const g of GOSTERGELER) {
        if (!S.gorunum.gosterge[g.id]) continue;
        Motor.gostergeCizgileri(R, g.id).forEach((c, j) => istenen.push(Object.assign({ anahtar: `g_${g.id}_${j}` }, c)));
      }
    }
    if (R && S.gorunum.cizgiler) for (const c of R.cizgiler) istenen.push(Object.assign({ anahtar: 'k_' + c.id }, c));
    const eski = new Map(cizgiListesi.map(c => [c.anahtar, c]));
    const yeni = new Set(istenen.map(c => c.anahtar));
    for (const c of cizgiListesi) if (!yeni.has(c.anahtar)) anaGrafik.removeSeries(c.seri);
    cizgiListesi = istenen.map(c => {
      const ops = {
        color: RENK[c.renk] || c.renk || RENK.metin2,
        lineWidth: c.kalinlik >= 2 ? 2 : 1,
        lineStyle: c.kesik ? LW.LineStyle.Dashed : LW.LineStyle.Solid,
      };
      const e = eski.get(c.anahtar);
      if (e) { e.seri.applyOptions(ops); return Object.assign(c, { seri: e.seri, renkKodu: ops.color }); }
      const ek = { priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false, priceFormat: fiyatFormati() };
      if (c.renk === 'soluk') ek.autoscaleInfoProvider = () => null; // soluk Fib bantları fiyat ölçeğini büyütmesin
      const seri = anaGrafik.addLineSeries(Object.assign(ops, ek));
      return Object.assign(c, { seri, renkKodu: ops.color });
    });
  }

  // Grafikte gösterilen kumbara (sadece çalışan kumbara; para çizgisi ve "simülatörün beklediği" için)
  const grafikKumbarasi = () => (GZ && GZ.kumbaraId ? kumbaraKaydi(GZ.kumbaraId) : null);

  function serileriDoldur() {
    const n = k + 1;
    mumSeri.setData(aralikDizi(n, mumVeri));
    for (const c of cizgiListesi) c.seri.setData(aralikDizi(n, i => cizgiVeri(c, i)));
    paraSerileriniDoldur();
    aoSerileriDoldur(); // 0.9
  }
  // Alttaki para grafiği: kumbara varsa kumbaranın parası ve simülatörün aynı sürede beklediği, yoksa simülatörün sanal hesabı.
  // Kumbara çizgisi kumbaranın başladığı mumdan başlar; öncesi boş bırakılır (iki grafik aynı mumlarda kalsın).
  let paraImza = '';
  function paraSerileriniDoldur() {
    if (!M || !R) return;
    const n = k + 1;
    const kb = grafikKumbarasi();
    const sv = kb ? kumbaraSerileri(kb) : null;
    paraImza = '';
    if (sv) {
      const deger = i => (i < sv.basIdx || i - sv.basIdx >= sv.deger.length ? { time: T[i] } : { time: T[i], value: sv.deger[i - sv.basIdx] });
      const bek = i => (i < sv.basIdx || i - sv.basIdx >= sv.beklenen.length ? { time: T[i] } : { time: T[i], value: sv.beklenen[i - sv.basIdx] });
      botSeri.setData(aralikDizi(n, deger));
      alTutSeri.setData(aralikDizi(n, bek));
      masrafsizSeri.setData([]);
      paraImza = kb.id + '|' + sv.imza;
    } else {
      botSeri.setData(aralikDizi(n, i => ({ time: T[i], value: R.ozk[i] })));
      alTutSeri.setData(aralikDizi(n, i => ({ time: T[i], value: R.alTut[i] })));
      masrafsizSeri.setData(aralikDizi(n, i => ({ time: T[i], value: R.ozk0[i] })));
    }
    paraLejant();
  }

  // Strateji değişince ya da çalıştırılamayan bir dosya seçilince grafiği boşalt
  function grafigiTemizle() {
    R = null; M = null; T = []; zamanIdx = new Map(); vitesDz = null; GZ = null;
    for (const c of cizgiListesi) anaGrafik.removeSeries(c.seri);
    cizgiListesi = [];
    for (const p of fiyatCizgileri) mumSeri.removePriceLine(p);
    fiyatCizgileri = []; seviyeImza = ''; isaretImza = ''; tabloImza = ''; kumbaraTabloImza = ''; olayImza = ''; paraImza = '';
    mumSeri.setMarkers([]);
    mumSeri.setData([]); botSeri.setData([]); alTutSeri.setData([]); masrafsizSeri.setData([]);
    AO.veri = null; AO.anahtar = ''; AO.fp = null; AO.fpZ = 0; aoZamanN = -1; // 0.9
    if (aoGrafik) { aoZaman.setData([]); aoDelta.setData([]); aoCvd.setData([]); aoVcvd.setData([]); aoOi.setData([]); }
    const g = katman.getContext('2d');
    g.setTransform(1, 0, 0, 1, 0, 0);
    g.clearRect(0, 0, katman.width, katman.height);
    sonImza = '';
    $('#ozet').innerHTML = '';
    $('#islemTablo').innerHTML = '';
    $('#paraLejant').innerHTML = '';
    $('#lejant').innerHTML = '';
    $('#botKart').hidden = true;
    $('#veriBilgi').textContent = '';
    incelemeKapat();
    seritYaz();
  }

  // ---------- hesap ve gösterim ----------
  let vitesDz = null; // her mumun vites numarası
  const vitesGorunur = () => S.gorunum.gostergeler && S.gorunum.gosterge.vites;
  function hesaptanSonra() {
    surum++;
    vitesDz = Motor.vitesDurumlari(R);
    cizgileriKur();
  }
  function goster(yeniK) {
    if (!M || !R) return;
    k = Math.max(0, Math.min(yeniK, M.c.length - 1));
    serileriDoldur();
    arayuzuGuncelle();
  }
  function arayuzuGuncelle() {
    if (secili != null && secili > k) incelemeKapat();
    $('#botKart').hidden = false;
    isaretleriKoy();
    seviyeCizgileri();
    ozetCiz();
    tabloCiz();
    botPaneli();
    durumHaplari();
    lejantYaz(null);
    vitesKolu();
    merdivenSutunlari();
    seritYaz();
  }

  let isaretImza = '';
  function isaretleriKoy() {
    const mk = [];
    for (const t of R.islemler) {
      if (t.girisIdx > k) break;
      mk.push({ time: T[t.girisIdx], position: 'belowBar', color: RENK.yukari, shape: 'arrowUp', text: 'AL' });
      if (t.cikisIdx != null && t.cikisIdx <= k) mk.push({ time: T[t.cikisIdx], position: 'aboveBar', color: t.net >= 0 ? RENK.yukari : RENK.asagi, shape: 'arrowDown', text: 'SAT' });
    }
    const imza = surum + '|' + mk.length;
    if (imza === isaretImza) return;
    isaretImza = imza;
    mumSeri.setMarkers(mk);
  }

  let fiyatCizgileri = [], seviyeImza = '';
  function seviyeCizgileri() {
    let liste = [];
    const lg = R.log[k];
    if (S.gorunum.cizgiler && lg && lg.sonra && lg.sonra.asama !== 'bekle') {
      const d = lg.sonra;
      liste = Motor.seviyeler(R, k, d);
      if (d.asama === 'coinde') liste.unshift({ ad: 'Alış fiyatı', fiyat: d.giris, renk: 'metin' });
    }
    liste = liste.filter(s => isFinite(s.fiyat));
    const imza = liste.map(s => s.ad + s.fiyat).join('|');
    if (imza === seviyeImza) return;
    seviyeImza = imza;
    for (const p of fiyatCizgileri) mumSeri.removePriceLine(p);
    fiyatCizgileri = liste.map(s => mumSeri.createPriceLine({
      price: s.fiyat, color: RENK[s.renk] || RENK.metin2, lineWidth: 1,
      lineStyle: s.renk === 'metin' ? LW.LineStyle.Dotted : LW.LineStyle.Dashed, axisLabelVisible: true, title: s.ad,
    }));
  }

  // ---------- grafik üstü çizimler (işlem kutuları, dip arama bölgeleri) — simülatörle aynı ----------
  // Al-Sat eki: kumbaranın başladığı yer ve gerçekleşen alış/satışlar (elmas işaretleri, gerçek dolum fiyatında)
  let sonImza = '';
  function cizimDongusu() {
    requestAnimationFrame(cizimDongusu);
    if (!R || !M) return;
    olcekleriEsitle(); // 0.9: akış paneli mumlarla hizalı kalsın
    const ts = anaGrafik.timeScale();
    const vr = ts.getVisibleLogicalRange();
    const kb = gorunenKumbara();
    const imza = [vr ? vr.from.toFixed(3) : '', vr ? vr.to.toFixed(3) : '', mumSeri.priceToCoordinate(M.c[k]), mumSeri.priceToCoordinate(M.c[k] * 1.01),
      ts.width(), katman.width, katman.height, k, secili, seciliIslem, surum, S.gorunum.kutular, S.gorunum.dip, vitesGorunur(),
      kb ? kb.id + ':' + kb.islemler.length + ':' + kb.islemler.filter(t => t.satis).length : '',
      kb && kb.koruma ? kb.koruma.kenarda + ':' + (kb.koruma.donemler || []).length + ':' + (kb.koruma.golge || []).length : '',
      AO.surum, aoAcik(), JSON.stringify(S.gorunum.ao)].join('|'); // 0.9: akış çizimleri
    if (imza === sonImza) return;
    sonImza = imza;
    katmanCiz();
  }

  // Bir zamanın (ms) düştüğü mumun sırası (M.t artan sırada)
  function mumIndeksi(ms) {
    if (!M || !M.t.length || ms < M.t[0]) return -1;
    let a = 0, b = M.t.length - 1;
    while (a < b) { const o = (a + b + 1) >> 1; if (M.t[o] <= ms) a = o; else b = o - 1; }
    return a;
  }

  function katmanCiz() {
    const dpr = window.devicePixelRatio || 1;
    const g = katman.getContext('2d');
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    const W = katman.width / dpr, H = katman.height / dpr;
    g.clearRect(0, 0, W, H);
    if (!R) return;
    const ts = anaGrafik.timeScale();
    const vr = ts.getVisibleLogicalRange();
    if (!vr) return;
    const pw = ts.width(), ph = H - ts.height();
    const bs = ts.options().barSpacing, yarim = bs / 2;
    const x = i => ts.logicalToCoordinate(i);
    const y = p => mumSeri.priceToCoordinate(p);
    const gorunur = (a, b) => !(b < vr.from - 2 || a > vr.to + 2);
    const masraf = strateji().para.masraf / 100;
    g.save();
    g.beginPath(); g.rect(0, 0, pw, ph); g.clip();

    if (vitesGorunur() && vitesDz) {
      const bas = Math.max(0, Math.floor(vr.from) - 1), son = Math.min(k, Math.ceil(vr.to) + 1);
      for (let i = bas; i <= son;) {
        const d = vitesDz[i];
        let j = i;
        while (j + 1 <= son && vitesDz[j + 1] === d) j++;
        if (d >= 0) {
          const x1 = x(i) - yarim, x2 = x(j) + yarim;
          if (x1 != null && x2 != null) {
            if (VITES_ZEMIN[d]) { g.fillStyle = rgba(VITES_RENK[d], VITES_ZEMIN[d]); g.fillRect(x1, 0, x2 - x1, ph); }
            g.fillStyle = rgba(VITES_RENK[d], 0.85);
            g.fillRect(x1, 0, x2 - x1, 5);
          }
        }
        i = j + 1;
      }
    }

    // Isınma: göstergeler oluşana kadar bot karar veremez. Bu bölge soluk ve taralı gösterilir.
    if (R.i0 > 0 && vr.from < R.i0 + 1) {
      const x0 = x(0), xi = x(R.i0);
      if (x0 != null && xi != null) {
        const xa = Math.max(0, x0 - yarim), xb = Math.min(pw, xi - yarim);
        if (xb > xa) {
          g.fillStyle = rgba(RENK.zemin, 0.55);
          g.fillRect(xa, 0, xb - xa, ph);
          g.save();
          g.beginPath(); g.rect(xa, 0, xb - xa, ph); g.clip();
          g.strokeStyle = rgba(RENK.metin3, 0.16);
          g.lineWidth = 1;
          g.beginPath();
          const adim = 14, kok = x0 - yarim; // desen veriye bağlı: kaydırınca çizgiler mumlarla birlikte kayar
          for (let s = kok + Math.ceil((xa - ph - kok) / adim) * adim; s <= xb; s += adim) { g.moveTo(s, ph); g.lineTo(s + ph, 0); }
          g.stroke();
          g.restore();
          const sinirGorunur = xi - yarim <= pw;
          if (sinirGorunur) {
            g.strokeStyle = rgba(RENK.vurgu, 0.8);
            g.setLineDash([5, 4]);
            g.beginPath(); g.moveTo(xb + 0.5, 0); g.lineTo(xb + 0.5, ph); g.stroke();
            g.setLineDash([]);
          }
          const yazi = (s1, s2, kx) => {
            g.font = "600 12px 'Plex', 'Segoe UI', sans-serif";
            const w1 = g.measureText(s1).width;
            g.font = "400 12px 'Plex', 'Segoe UI', sans-serif";
            const w2 = g.measureText(s2).width;
            const kw = Math.max(w1, w2) + 20, kh = 42, ky = ph - kh - 16;
            const lx = kx(kw);
            if (lx == null) return false;
            g.fillStyle = rgba(RENK.panel, 0.92);
            g.fillRect(lx, ky, kw, kh);
            g.strokeStyle = rgba(RENK.vurgu, 0.7);
            g.strokeRect(lx + 0.5, ky + 0.5, kw - 1, kh - 1);
            g.font = "600 12px 'Plex', 'Segoe UI', sans-serif";
            g.fillStyle = RENK.vurgu;
            g.fillText(s1, lx + 10, ky + 17);
            g.font = "400 12px 'Plex', 'Segoe UI', sans-serif";
            g.fillStyle = RENK.metin2;
            g.fillText(s2, lx + 10, ky + 33);
            return true;
          };
          const baslik = R.isinma > R.n ? `ISINMA · ${nf(0).format(R.isinma)} mum gerekli, ${nf(0).format(R.n)} var` : `ISINMA · ilk ${nf(0).format(R.i0)} mum`;
          // Önce bölgenin içine (sınırın hemen soluna), sığmazsa sınırın sağına yaz
          const icerde = yazi(baslik, 'Bot göstergeleri hesaplıyor, burada işlem yapmaz',
            kw => (xb - xa >= kw + 16 ? (sinirGorunur ? xb - kw - 10 : xa + (xb - xa - kw) / 2) : null));
          if (!icerde && sinirGorunur) yazi('← ' + baslik, 'Bot bu çizgiden önce işlem yapmaz', kw => (xb + kw + 10 <= pw ? xb + 10 : null));
        }
      }
    }

    if (S.gorunum.dip) {
      for (const dp of R.dipler) {
        if (dp.bas > k) break;
        const bit = Math.min(dp.bit == null ? k : dp.bit, k);
        if (!gorunur(dp.bas, bit)) continue;
        const x1 = x(dp.bas) - yarim, x2 = x(bit) + yarim;
        g.fillStyle = rgba(RENK.mavi, 0.08);
        g.fillRect(x1, 0, x2 - x1, ph);
        g.fillStyle = rgba(RENK.mavi, dp.sonuc === 'iptal' ? 0.35 : 0.75);
        g.fillRect(x1, ph - 4, x2 - x1, 4);
      }
    }

    if (S.gorunum.kutular) {
      g.font = "600 11px 'JBMono', Consolas, monospace";
      for (const t of R.islemler) {
        if (t.girisIdx > k) break;
        const kapandi = t.cikisIdx != null && t.cikisIdx <= k;
        const bitIdx = kapandi ? t.cikisIdx : k;
        if (!gorunur(t.girisIdx, bitIdx)) continue;
        const cikisF = kapandi ? t.cikis : M.c[k];
        const net = kapandi ? t.net : t.adet * cikisF * (1 - masraf) - t.harca;
        const yzd = (net / t.harca) * 100;
        const renk = net >= 0 ? RENK.yukari : RENK.asagi;
        const x1 = x(t.girisIdx), x2 = x(bitIdx), y1 = y(t.giris), y2 = y(cikisF);
        if (x1 == null || x2 == null || y1 == null || y2 == null) continue;
        const ust = Math.min(y1, y2), yuk = Math.max(2, Math.abs(y2 - y1)), gen = Math.max(2, x2 - x1);
        const secik = seciliIslem === t.no;
        g.fillStyle = rgba(renk, kapandi ? (secik ? 0.28 : 0.16) : 0.1);
        g.fillRect(x1, ust, gen, yuk);
        g.setLineDash(kapandi ? [] : [4, 3]);
        g.lineWidth = secik ? 2 : 1;
        g.strokeStyle = rgba(renk, secik ? 1 : 0.8);
        g.strokeRect(x1 + 0.5, ust + 0.5, gen - 1, yuk - 1);
        g.setLineDash([2, 3]);
        g.lineWidth = 1;
        g.strokeStyle = rgba(RENK.metin, 0.45);
        g.beginPath(); g.moveTo(x1, y1); g.lineTo(x2, y2); g.stroke();
        g.setLineDash([]);
        if (gen > 6 || kapandi) {
          const yazi = Bicim.yuzde(yzd, 2) + (kapandi ? '' : ' (açık)');
          const yw = g.measureText(yazi).width + 10;
          let lx, ly;
          if (gen > yw + 6 && yuk > 20) { lx = x1 + (gen - yw) / 2; ly = ust + yuk / 2 - 9; }
          else { lx = x2 + 6; ly = y2 - 9; }
          g.fillStyle = rgba(RENK.panel, 0.88);
          g.fillRect(lx, ly, yw, 18);
          g.strokeStyle = rgba(renk, 0.9);
          g.strokeRect(lx + 0.5, ly + 0.5, yw - 1, 17);
          g.fillStyle = renk;
          g.fillText(yazi, lx + 5, ly + 13);
        }
      }
    }

    aoCiz(g, x, y, ph, pw, yarim, vr); // 0.9: akış onayı (yapı, emir akışı, kararlar)
    kumbaraCiz(g, x, y, ph, yarim);

    if (secili != null && secili <= k) {
      const xs = x(secili);
      if (xs != null) {
        g.strokeStyle = rgba(RENK.metin, 0.6);
        g.setLineDash([3, 3]);
        g.beginPath(); g.moveTo(xs + 0.5, 0); g.lineTo(xs + 0.5, ph); g.stroke();
        g.setLineDash([]);
      }
    }
    g.restore();
  }

  // Kumbaranın başladığı çizgi ve gerçekleşen alış/satışlar (gerçek dolum fiyatında elmas)
  function kumbaraCiz(g, x, y, ph, yarim) {
    const kb = gorunenKumbara();
    if (!kb || !M) return;
    // 0.7: kumbaranın kenarda kaldığı zamanlar (koruma): sarımsı bölge; içindeki motor işlemleri gölge (parasız) işlemlerdir
    if (kb.koruma && kb.koruma.donemler) {
      const pw = anaGrafik.timeScale().width();
      for (const dn of kb.koruma.donemler) {
        const i1 = Math.max(0, mumIndeksi(dn.bas));
        const i2 = dn.bit ? mumIndeksi(dn.bit) : k;
        if (i2 < 0 || i1 > k) continue;
        const x1 = x(i1), x2 = x(Math.min(k, i2));
        if (x1 == null || x2 == null) continue;
        const xa = x1 - yarim, xb = x2 + yarim;
        g.fillStyle = rgba(RENK.vurgu, 0.07);
        g.fillRect(xa, 0, xb - xa, ph);
        g.fillStyle = rgba(RENK.vurgu, 0.7);
        g.fillRect(xa, 0, xb - xa, 3);
        g.font = "600 11px 'Plex', 'Segoe UI', sans-serif";
        const yazi = dn.bit ? 'KENARDA · gölge işlemler (parasız)' : 'KENARDA · yeni alım yok, gölgede izleniyor';
        const w = g.measureText(yazi).width + 12;
        const lx = Math.min(Math.max(4, xa + 4), pw - w - 4), ly = ph - 52;
        g.fillStyle = rgba(RENK.panel, 0.9);
        g.fillRect(lx, ly, w, 18);
        g.strokeStyle = rgba(RENK.vurgu, 0.8);
        g.strokeRect(lx + 0.5, ly + 0.5, w - 1, 17);
        g.fillStyle = RENK.vurgu;
        g.fillText(yazi, lx + 6, ly + 13);
      }
    }
    const basIdx = mumIndeksi(kb.baslangic);
    if (basIdx >= 0 && basIdx <= k) {
      const xb = x(basIdx);
      if (xb != null) {
        const renk = kb.mod === 'gercek' ? RENK.turuncu : RENK.mavi;
        g.strokeStyle = rgba(renk, 0.85);
        g.setLineDash([6, 4]);
        g.beginPath(); g.moveTo(xb + 0.5, 0); g.lineTo(xb + 0.5, ph); g.stroke();
        g.setLineDash([]);
        g.font = "600 11px 'Plex', 'Segoe UI', sans-serif";
        const yazi = (kb.durum === 'bitti' ? 'Son kumbara başladı · ' : 'Kumbara başladı · ') + (kb.mod === 'gercek' ? 'gerçek' : 'sanal');
        const w = g.measureText(yazi).width + 12;
        const pw = anaGrafik.timeScale().width();
        const lx = xb + 4 + w <= pw - 4 ? xb + 4 : Math.max(4, xb - 4 - w); // sağda yer yoksa çizginin soluna
        const ly = ph - 30; // altta: üstteki lejant ve düğmelerle çakışmasın
        g.fillStyle = rgba(RENK.panel, 0.9);
        g.fillRect(lx, ly, w, 18);
        g.strokeStyle = rgba(renk, 0.8);
        g.strokeRect(lx + 0.5, ly + 0.5, w - 1, 17);
        g.fillStyle = renk;
        g.fillText(yazi, lx + 6, ly + 13);
      }
    }
    const elmas = (zaman, fiyat, renk, dolu) => {
      const i = mumIndeksi(zaman);
      if (i < 0 || i > k) return;
      const xi = x(i), yi = y(fiyat);
      if (xi == null || yi == null) return;
      const r = Math.max(5, Math.min(8, yarim + 2));
      g.beginPath();
      g.moveTo(xi, yi - r); g.lineTo(xi + r, yi); g.lineTo(xi, yi + r); g.lineTo(xi - r, yi); g.closePath();
      g.fillStyle = dolu ? renk : rgba(RENK.panel, 0.9);
      g.fill();
      g.lineWidth = 2;
      g.strokeStyle = dolu ? RENK.zemin : renk;
      g.stroke();
      g.lineWidth = 1;
    };
    for (const t of kb.islemler) {
      if (t.alis && t.alis.ortalama > 0) elmas(t.alis.bitis, t.alis.ortalama, RENK.yukari, true);
      if (t.satis && t.satis.ortalama > 0) elmas(t.satis.bitis, t.satis.ortalama, t.net >= 0 ? RENK.yukari : RENK.asagi, false);
    }
  }

  // ---------- lejantlar ----------
  function lejantYaz(i) {
    if (!M) { $('#lejant').innerHTML = ''; return; }
    if (i == null || i > k) i = k;
    const deg = i > 0 ? (M.c[i] / M.c[i - 1] - 1) * 100 : 0;
    const cizgiler = cizgiListesi.filter(c => c.ad).map(c => c.durumlar
      ? `<span><i class="cok-renk"></i>${c.ad}</span>`
      : `<span><i class="${c.kesik ? 'kesik' : ''}" style="border-color:${c.renk === 'soluk' ? RENK.metin3 : c.renkKodu}"></i>${c.ad}</span>`);
    if (vitesGorunur()) cizgiler.push(VITES.AD.map((ad, j) => `<span class="vites-cip"><i style="background:${VITES_RENK[j]}"></i>${ad}</span>`).join(''));
    cizgiler.push(`<span><b class="ok pos">▲</b>AL</span><span><b class="ok neg">▼</b>SAT</span>`);
    if (gorunenKumbara()) cizgiler.push(`<span><b class="elmas al">◆</b>kumbaranın alışı</span><span><b class="elmas sat">◇</b>satışı (gerçek fiyat)</span>`);
    const kbk = gorunenKumbara();
    if (kbk && kbk.koruma && (kbk.koruma.donemler || []).length) cizgiler.push(`<span><i class="kenar-ornek"></i>kenarda (koruma)</span>`);
    cizgiler.push(...aoLejantParcalari()); // 0.9
    $('#lejant').innerHTML = `
      <div class="lejant-ust">${S.sembol}<span>${aralikAd(S.aralik)} · MEXC · ${tarih(M.t[i])}</span></div>
      <div class="lejant-ohlc"><b>Açılış</b> ${f(M.o[i])} <b>En yüksek</b> ${f(M.h[i])} <b>En düşük</b> ${f(M.l[i])} <b>Kapanış</b> ${f(M.c[i])} <span class="${isaretSinif(deg)}">${Bicim.yuzde(deg)}</span></div>
      <div class="lejant-cizgiler">${cizgiler.join('')}</div>`;
  }
  function paraLejant() {
    if (!R) return;
    const kb = grafikKumbarasi();
    if (kb) {
      const sv = kumbaraSerileri(kb);
      const bek = sv && sv.beklenen.length ? sv.beklenen[sv.beklenen.length - 1] : null;
      $('#paraBaslik').textContent = 'Kumbaranın parası';
      $('#paraLejant').innerHTML = `
        <span><i style="border-color:${RENK.vurgu}"></i>Kumbara (${kb.mod === 'gercek' ? 'gerçek' : 'sanal'})<b>${usdt(kb.deger)}</b></span>
        <span><i style="border-color:${RENK.metin3}"></i>Simülatörün aynı sürede beklediği<b>${usdt(bek)}</b></span>`;
      return;
    }
    $('#paraBaslik').textContent = 'Simülatörün hesabı';
    $('#paraLejant').innerHTML = `
      <span><i style="border-color:${RENK.vurgu}"></i>Botun parası (sanal)<b>${Bicim.para(R.ozk[k])}</b></span>
      <span><i style="border-color:${RENK.metin3}"></i>Sadece alıp tutsaydın<b>${Bicim.para(R.alTut[k])}</b></span>
      <span><i class="kesik" style="border-color:${RENK.metin2}"></i>Masraf olmasaydı<b>${Bicim.para(R.ozk0[k])}</b></span>`;
  }

  // Kumbaranın değeri ve simülatörün aynı sürede beklediği, her mum için (kumbaranın başladığı mumdan itibaren).
  // Beklenen: aynı parayla başlayıp sadece kumbara açıldıktan sonra kapanan mumlarda verilen alımları yapan,
  // simülatörün masraf varsayımıyla hesaplanmış hesap (başlarken motor coindeyse o işleme atlanmaz; kumbara da öyle).
  const svOnbellek = { imza: '', sonuc: null };
  function kumbaraSerileri(kb) {
    if (!M || !R || !kb) return null;
    const ms = ARALIK[S.aralik].ms, n = M.t.length;
    const imza = [kb.id, surum, n, kb.islemler.length, kb.islemler.map(t => (t.satis ? t.satis.bitis : 0)).join(','), Math.round((kb.sonFiyat || 0) * 1e9)].join('|');
    if (svOnbellek.imza === imza) return svOnbellek.sonuc;
    let basIdx = mumIndeksi(kb.baslangic);
    if (basIdx < 0) basIdx = 0;
    const dolumlar = [];
    for (const t of kb.islemler) {
      if (t.alis) dolumlar.push({ z: t.alis.bitis, nakit: -t.alis.netTutar, adet: t.alis.netMiktar });
      if (t.satis && t.satis.yol !== 'toz' && t.satis.yol !== 'birakildi') dolumlar.push({ z: t.satis.bitis, nakit: t.satis.netTutar, adet: -t.satis.miktar });
    }
    dolumlar.sort((a, b) => a.z - b.z);
    const mf = (strateji().para.masraf || 0) / 100;
    const girisler = new Map(), cikislar = new Map();
    for (const t of R.islemler) {
      const kapanis = M.t[t.girisIdx] + ms;
      if (kapanis <= kb.baslangic || kapanis > kb.bitis) continue;
      girisler.set(t.girisIdx, t);
      if (t.cikisIdx != null) cikislar.set(t.cikisIdx, t);
    }
    let nakit = kb.baslangicTutar, adet = 0, j = 0, bNakit = kb.baslangicTutar, bAdet = 0, acik = null;
    const deger = [], beklenen = [];
    for (let i = basIdx; i < n; i++) {
      const son = M.t[i] + ms;
      while (j < dolumlar.length && dolumlar[j].z < son) { nakit += dolumlar[j].nakit; adet += dolumlar[j].adet; j++; }
      const fiyat = i === n - 1 && kb.sonFiyat > 0 ? kb.sonFiyat : M.c[i];
      deger.push(Math.max(0, nakit) + Math.max(0, adet) * fiyat);
      if (acik && cikislar.get(i) === acik) { bNakit = bAdet * acik.cikis * (1 - mf); bAdet = 0; acik = null; }
      const gr = girisler.get(i);
      if (gr && !acik) { bAdet = (bNakit * (1 - mf)) / gr.giris; bNakit = 0; acik = gr; }
      beklenen.push(bNakit + bAdet * M.c[i]);
    }
    svOnbellek.imza = imza;
    svOnbellek.sonuc = { basIdx, deger, beklenen, imza };
    return svOnbellek.sonuc;
  }

  // ---------- özet kartları ----------
  function ozetHesap() {
    let kazanc = 0, kayip = 0, masraf = 0, kazanan = 0, kaybeden = 0, acik = null;
    for (const t of R.islemler) {
      if (t.girisIdx > k) break;
      masraf += t.girisMasraf;
      if (t.cikisIdx != null && t.cikisIdx <= k) {
        masraf += t.cikisMasraf;
        if (t.brut >= 0) kazanc += t.brut; else kayip += t.brut;
        if (t.net > 0) kazanan++; else kaybeden++;
      } else acik = t;
    }
    const bas = strateji().para.baslangic;
    const acikKar = acik ? acik.adet * (M.c[k] - acik.giris) : 0;
    return { bas, ozk: R.ozk[k], ozk0: R.ozk0[k], alTut: R.alTut[k], kazanc, kayip, masraf, kazanan, kaybeden, acik, acikKar };
  }
  function isinmaYazisi() {
    if (R.isinma >= R.n) return `Açık kuralların kullandığı en uzun gösterge ${nf(0).format(R.isinma)} mum geriye bakıyor ama grafikte sadece ${nf(0).format(R.n)} mum var. Bot hiç başlayamaz.`;
    return `Göstergelerin oluşması için ilk ${nf(0).format(R.i0)} mum bekleniyor (açık kuralların kullandığı en uzun gösterge ${nf(0).format(R.i0)} mum geriye bakıyor). Bot bu sürede işlem yapmaz.`;
  }

  function ozetCiz() {
    const el = $('#ozet');
    const kb = grafikKumbarasi();
    if (kb) { kumbaraOzetCiz(el, kb); return; }
    if (k < R.i0) {
      el.innerHTML = `<div class="ozet-kart" style="grid-column:1/-1"><div class="ozet-etiket">Bot henüz başlamadı</div><div class="ozet-alt">${isinmaYazisi()}</div></div>`;
      return;
    }
    const o = ozetHesap();
    const botY = (o.ozk / o.bas - 1) * 100, tutY = (o.alTut / o.bas - 1) * 100, sifY = (o.ozk0 / o.bas - 1) * 100;
    const fark = botY - tutY;
    const toplam = o.kazanan + o.kaybeden;
    const isabet = toplam ? Math.round((o.kazanan / toplam) * 100) : 0;
    const satirlar = [
      ['Kazandıran işlemler', o.kazanc, RENK.yukari],
      ['Kaybettiren işlemler', o.kayip, RENK.asagi],
      ['Masraflar', -o.masraf, RENK.turuncu],
    ];
    if (o.acik) satirlar.push(['Açık işlem (şimdilik)', o.acikKar, o.acikKar >= 0 ? RENK.yukari : RENK.asagi]);
    const enBuyuk = Math.max(1e-9, ...satirlar.map(s => Math.abs(s[1])), Math.abs(o.ozk - o.bas));
    const cubuk = (deger, renk) => `<div class="cubuk"><i style="width:${(Math.abs(deger) / enBuyuk) * 100}%;background:${renk}"></i></div>`;
    el.innerHTML = `
      <div class="ozet-kart">
        <div class="ozet-etiket">Simülatörün hesabı (sanal)</div>
        <div class="ozet-deger">${Bicim.para(o.ozk)}<small class="${isaretSinif(botY)}">${Bicim.yuzde(botY)}</small></div>
        <div class="ozet-alt">Başlangıç <b>${Bicim.para(o.bas)}</b></div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Sadece alıp tutsaydın</div>
        <div class="ozet-deger">${Bicim.para(o.alTut)}<small class="${isaretSinif(tutY)}">${Bicim.yuzde(tutY)}</small></div>
        <div class="ozet-alt">Bot bundan <b class="${isaretSinif(fark)}">${nf(1).format(Math.abs(fark))} puan ${fark >= 0 ? 'önde' : 'geride'}</b></div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Biten işlemler</div>
        <div class="ozet-deger">${toplam}${o.acik ? '<small class="sol-gri">+1 açık</small>' : ''}</div>
        <div class="ozet-alt"><b class="pos">${o.kazanan}</b> kârlı · <b class="neg">${o.kaybeden}</b> zararlı${toplam ? ` · isabet <b>%${isabet}</b>` : ''}</div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Ödenen masraf</div>
        <div class="ozet-deger neg">${Bicim.para(o.masraf)}</div>
        <div class="ozet-alt">Masraf olmasaydı <b class="${isaretSinif(sifY)}">${Bicim.yuzde(sifY)}</b></div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Paran nereye gitti</div>
        <div class="nereye">
          ${satirlar.map(s => `<span>${s[0]}</span>${cubuk(s[1], s[2])}<b class="${isaretSinif(s[1])}">${Bicim.paraIsaret(s[1])}</b>`).join('')}
          <span class="net">Toplam</span><div class="net">${cubuk(o.ozk - o.bas, o.ozk >= o.bas ? RENK.yukari : RENK.asagi)}</div><b class="net ${isaretSinif(o.ozk - o.bas)}">${Bicim.paraIsaret(o.ozk - o.bas)}</b>
        </div>
      </div>`;
  }

  // Kumbara açıkken üstteki kartlar: kumbaranın gerçek sonucu ile simülatörün aynı sürede beklediği yan yana
  function kumbaraOzetCiz(el, kb) {
    const sv = kumbaraSerileri(kb);
    const bas = kb.baslangicTutar, deger = kb.deger, yz = (deger / bas - 1) * 100;
    const bek = sv && sv.beklenen.length ? sv.beklenen[sv.beklenen.length - 1] : null;
    const bekY = bek != null ? (bek / bas - 1) * 100 : null;
    const kapali = kb.islemler.filter(t => t.satis);
    const karli = kapali.filter(t => t.net > 0).length;
    const acik = acikIslemi(kb);
    const alisFark = ort(kb.islemler.filter(t => t.beklenenAlis > 0 && t.alis && t.alis.ortalama > 0).map(t => (t.alis.ortalama / t.beklenenAlis - 1) * 100));
    const satisFark = ort(kapali.filter(t => t.beklenenSatis > 0 && t.satis.ortalama > 0).map(t => (t.satis.ortalama / t.beklenenSatis - 1) * 100));
    const sonucFark = ort(kapali.filter(t => t.beklenenVar).map(t => t.yuzde - t.beklenenYuzde));
    const masraf = strateji().para.masraf;
    const farkSatir = (ad, x, iyiYon) => `<span>${ad}</span><b class="${x == null ? '' : x * iyiYon >= 0 ? 'pos' : 'neg'}">${x == null ? '–' : Bicim.yuzde(x, 2)}</b>`;
    el.innerHTML = `
      <div class="ozet-kart">
        <div class="ozet-etiket">Kumbara · ${kb.mod === 'gercek' ? 'gerçek para' : 'sanal para'}</div>
        <div class="ozet-deger">${nf(2).format(deger)}<small class="${isaretSinif(yz)}">${Bicim.yuzde(yz)}</small></div>
        <div class="ozet-alt">Başlangıç <b>${usdt(bas)}</b></div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Simülatör aynı sürede</div>
        <div class="ozet-deger">${bek != null ? nf(2).format(bek) : '–'}${bekY != null ? `<small class="${isaretSinif(bekY)}">${Bicim.yuzde(bekY)}</small>` : ''}</div>
        <div class="ozet-alt">${bekY == null ? 'Grafik yükleniyor' : Math.abs(yz - bekY) < 0.005 ? 'Kumbara şimdilik simülatörle <b>aynı</b>' : `Kumbara bundan <b class="${isaretSinif(yz - bekY)}">${nf(2).format(Math.abs(yz - bekY))} puan ${yz - bekY >= 0 ? 'önde' : 'geride'}</b>`}</div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Kumbaranın işlemleri</div>
        <div class="ozet-deger">${kapali.length}${acik ? '<small class="sol-gri">+1 açık</small>' : ''}</div>
        <div class="ozet-alt"><b class="pos">${karli}</b> kârlı · <b class="neg">${kapali.length - karli}</b> zararlı</div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Ödenen komisyon</div>
        <div class="ozet-deger neg">${nf(2).format(kb.komisyon)}<small class="sol-gri">USDT</small></div>
        <div class="ozet-alt">Simülatör işlem başına <b>%${nfEn(3).format(masraf)}</b> masraf varsaydı</div>
      </div>
      <div class="ozet-kart">
        <div class="ozet-etiket">Gerçek fiyat, simülatörün fiyatına göre</div>
        <div class="nereye fark-tablo">
          ${farkSatir('Alışta ortalama (eksi iyidir)', alisFark, -1)}
          ${farkSatir('Satışta ortalama (artı iyidir)', satisFark, 1)}
          ${farkSatir('İşlem sonucu farkı (ortalama)', sonucFark, 1)}
        </div>
      </div>`;
  }

  // ---------- alt tablo: kumbaranın işlemleri / simülatörün işlemleri / olaylar ----------
  let tabloImza = '', kumbaraTabloImza = '', olayImza = '';
  function gecerliSekme() {
    if (sekme) return sekme;
    return gorunenKumbara() ? 'kumbara' : 'simulator';
  }
  function tabloCiz(zorla) {
    const s = gecerliSekme();
    for (const b of document.querySelectorAll('#sekmeler button')) b.setAttribute('aria-pressed', String(b.dataset.sekme === s));
    $('#islemTablo').hidden = s === 'olaylar';
    $('#olayListe').hidden = s !== 'olaylar';
    if (s === 'simulator') simTabloCiz(zorla);
    else if (s === 'kumbara') kumbaraTabloCiz(zorla);
    else olaylariCiz(zorla);
  }

  function simTabloCiz(zorla) {
    const tablo = $('#islemTablo');
    $('#tabloNot').textContent = 'Simülatörün sanal hesabı. Satıra tıkla: grafikte göster ve hikâyesini anlat';
    if (!R) { tablo.innerHTML = ''; tabloImza = ''; return; }
    const gorunen = R.islemler.filter(t => t.girisIdx <= k);
    const acik = gorunen.find(t => t.cikisIdx == null || t.cikisIdx > k);
    const imza = [surum, gorunen.length, gorunen.filter(t => t.cikisIdx != null && t.cikisIdx <= k).length, seciliIslem, aoTabloImza()].join('|');
    const aoHarita = aoKayitHaritasi(); // 0.9
    kumbaraTabloImza = '';
    if (imza !== tabloImza || zorla) {
      tabloImza = imza;
      if (!gorunen.length) {
        tablo.innerHTML = `<tbody><tr><td class="tablo-bos">Henüz işlem yok. Bot kurallardaki şartların oluşmasını bekliyor.</td></tr></tbody>`;
        return;
      }
      const satir = t => {
        const kapandi = t.cikisIdx != null && t.cikisIdx <= k;
        const mumSay = (kapandi ? t.cikisIdx : k) - t.girisIdx;
        return `<tr data-no="${t.no}" class="${seciliIslem === t.no ? 'secili' : ''}">
          <td>${t.no}</td>
          <td><span class="zaman">${tarih(M.t[t.girisIdx])}</span>${f(t.giris)}</td>
          <td>${kapandi ? `<span class="zaman">${tarih(M.t[t.cikisIdx])}</span>${f(t.cikis)}` : '<span class="acik-etiket">hâlâ coinde</span>'}</td>
          <td class="sebep">${kapandi ? KURAL[t.cikisKural].ad : '—'}</td>
          ${aoTabloHucre(aoHarita.get(M.t[t.girisIdx]))}
          <td ${kapandi ? '' : 'data-acik-sure="1"'}>${mumSay} mum · ${sureYaz(mumSay * ARALIK[S.aralik].ms)}</td>
          <td class="sonuc ${kapandi ? isaretSinif(t.net) : ''}" ${kapandi ? '' : 'data-acik="1"'}>${kapandi ? `${Bicim.yuzde(t.yuzde)} <small>${Bicim.paraIsaret(t.net)}</small>` : ''}</td>
        </tr>`;
      };
      tablo.innerHTML = `<thead><tr><th>#</th><th>Alış</th><th>Satış</th><th>Neden sattı</th><th title="Akış onayının bu sinyale kararı (✓ onay, ✕ red; soluk = gölge)">Akış</th><th>Süre</th><th class="sonuc">Sonuç</th></tr></thead>
        <tbody>${gorunen.slice().reverse().map(satir).join('')}</tbody>`;
    }
    if (acik) {
      const hucre = tablo.querySelector('td[data-acik]');
      if (hucre) {
        const net = acik.adet * M.c[k] * (1 - strateji().para.masraf / 100) - acik.harca;
        hucre.className = 'sonuc ' + isaretSinif(net);
        hucre.innerHTML = `${Bicim.yuzde((net / acik.harca) * 100)} <small>şimdilik</small>`;
      }
      const sure = tablo.querySelector('td[data-acik-sure]');
      if (sure) { const ms = k - acik.girisIdx; sure.textContent = `${ms} mum · ${sureYaz(ms * ARALIK[S.aralik].ms)}`; }
    }
  }

  // Kumbaranın gerçekleşen işlemleri, simülatörün fiyatıyla yan yana
  function kumbaraTabloCiz(zorla) {
    const tablo = $('#islemTablo');
    const kb = gorunenKumbara();
    tabloImza = '';
    if (!kb) {
      $('#tabloNot').textContent = '';
      if (kumbaraTabloImza !== 'yok' || zorla) {
        kumbaraTabloImza = 'yok';
        tablo.innerHTML = `<tbody><tr><td class="tablo-bos">Bu strateji için kumbara yok. Sağdaki <b>Başlat…</b> ile sanal ya da gerçek parayla başlatabilirsin.</td></tr></tbody>`;
      }
      return;
    }
    $('#tabloNot').textContent = (kb.durum === 'bitti' ? `Son kumbara, kapandı ${gunSaat(kb.bitisZamani)} · ` : '') +
      `${kb.mod === 'gercek' ? 'Gerçek' : 'Sanal'} emirler. "Beklenen" simülatörün fiyatı. Satıra tıkla: grafikte göster`;
    const golge = kb.koruma && kb.koruma.kenarda ? kb.koruma.golge || [] : [];
    const imza = JSON.stringify([kb.id, kb.islemler.map(t => [t.satis ? t.satis.bitis : 0, t.beklenenVar, t.beklenenSatis]), acikIslemi(kb) ? Math.round(kb.sonFiyat * 1e9) : 0, golge.length]);
    if (imza === kumbaraTabloImza && !zorla) return;
    kumbaraTabloImza = imza;
    if (!kb.islemler.length && !golge.length) {
      tablo.innerHTML = `<tbody><tr><td class="tablo-bos">Henüz işlem yok. ${kb.durum === 'calisiyor' ? (kb.koruma && kb.koruma.kenarda ? 'Kumbara kenarda; motor gölgede izleniyor.' : 'Simülatörün motorunun sıradaki alım kararı bekleniyor.') : ''}</td></tr></tbody>`;
      return;
    }
    const farkHTML = (gercek, beklenen, alis) => {
      if (!(beklenen > 0) || !(gercek > 0)) return '';
      const yz = (gercek / beklenen - 1) * 100;
      const iyi = alis ? yz <= 0 : yz >= 0;
      return `<small class="fark"><span>beklenen ${fg(beklenen)}</span> <b class="${Math.abs(yz) < 0.005 ? '' : iyi ? 'pos' : 'neg'}">${Bicim.yuzde(yz, 2)}</b></small>`;
    };
    const satir = t => {
      const a = t.alis, s = t.satis;
      let sonuc;
      if (s) {
        sonuc = `<td class="sonuc ${isaretSinif(t.net)}">${Bicim.yuzde(t.yuzde)} <small>${usdtIsaret(t.net)}</small>${t.beklenenVar ? `<small class="bek">simülatör ${Bicim.yuzde(t.beklenenYuzde)}</small>` : ''}</td>`;
      } else {
        const anlik = kb.adet * kb.sonFiyat - t.harcanan;
        sonuc = `<td class="sonuc ${isaretSinif(anlik)}">${t.harcanan > 0 ? Bicim.yuzde((anlik / t.harcanan) * 100) : '–'} <small>şimdilik</small></td>`;
      }
      const kom = (a ? a.komisyon : 0) + (s ? s.komisyon : 0);
      return `<tr data-kno="${t.no}">
        <td>${t.no}</td>
        <td><span class="zaman">${a ? tarih(a.bitis) : ''}</span>${a ? fg(a.ortalama) : '–'}${a ? farkHTML(a.ortalama, t.beklenenAlis, true) : ''}</td>
        <td>${s ? (s.yol === 'toz' || s.yol === 'birakildi' ? `<span class="acik-etiket">${s.yol === 'toz' ? 'satılamadı' : 'satılmadı'}</span>` : `<span class="zaman">${tarih(s.bitis)}</span>${fg(s.ortalama)}${farkHTML(s.ortalama, t.beklenenSatis, false)}`) : '<span class="acik-etiket">hâlâ coinde</span>'}</td>
        <td class="sebep">${a ? YOL_AD[a.yol] || a.yol : ''}${s ? ' / ' + (YOL_AD[s.yol] || s.yol) : ''}</td>
        <td class="sebep">${s ? esc(sebepAdi(t.satisSebebi)) : '—'}</td>
        <td>${nf(kom >= 1 ? 2 : 4).format(kom)}</td>
        ${sonuc}
      </tr>`;
    };
    const kms = ARALIK[kb.aralik] ? ARALIK[kb.aralik].ms : 60000;
    const golgeSatir = gi => `<tr class="golge-satir">
        <td>G</td>
        <td><span class="zaman">${tarih(gi.g + kms)}</span>${fg(gi.gf)}</td>
        <td><span class="zaman">${tarih(gi.c + kms)}</span>${fg(gi.cf)}</td>
        <td class="sebep">gölge (parasız)</td>
        <td class="sebep">motor</td>
        <td>–</td>
        <td class="sonuc ${isaretSinif(gi.y)}">${Bicim.yuzde(gi.y)} <small>gölge</small></td>
      </tr>`;
    tablo.innerHTML = `<thead><tr><th>#</th><th>Alış (gerçek)</th><th>Satış (gerçek)</th><th>Nasıl doldu</th><th>Neden sattı</th><th>Komisyon $</th><th class="sonuc">Sonuç</th></tr></thead>
      <tbody>${golge.slice().reverse().map(golgeSatir).join('')}${kb.islemler.slice().reverse().map(satir).join('')}</tbody>`;
  }

  const OLAY_ISARET = { al: '▲', sat: '▼', uyari: '!', hata: '✕', bilgi: '·' };
  function olaylariCiz(zorla) {
    const el = $('#olayListe');
    const kb = gorunenKumbara();
    tabloImza = ''; kumbaraTabloImza = '';
    const genel = DURUM ? DURUM.genelOlaylar || [] : [];
    $('#tabloNot').textContent = kb ? `${kb.sembol} · ${aralikAd(kb.aralik)} kumbarasının olayları (en yeni üstte)` : 'Programın genel olayları';
    const imza = JSON.stringify([kb ? kb.id : '', kb ? kb.olaylar.length : 0, kb && kb.olaylar.length ? kb.olaylar[kb.olaylar.length - 1].zaman : 0, genel.length]);
    if (imza === olayImza && !zorla) return;
    olayImza = imza;
    const satir = o => `<li class="olay-${o.tur}"><time>${gunSaat(o.zaman)}:${iki(new Date(o.zaman).getSeconds())}</time><i>${OLAY_ISARET[o.tur] || '·'}</i><span>${esc(o.mesaj)}${o.tekrar ? ` <em>(${o.tekrar + 1} kez)</em>` : ''}</span></li>`;
    let h = '';
    if (kb) h += `<ul>${kb.olaylar.slice().reverse().map(satir).join('')}</ul>`;
    if (genel.length) h += `<div class="olay-baslik">Genel</div><ul>${genel.slice().reverse().map(satir).join('')}</ul>`;
    el.innerHTML = h || '<p class="tablo-bos">Henüz olay yok.</p>';
  }

  // ---------- bot paneli: akış şeması + düşünce (simülatörle aynı) ----------
  function akisKur() {
    const ok = (id, renk) => `<marker id="uc-${id}" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="${renk}"/></marker>`;
    const dugum = (id, x, baslik) => `<g class="dugum" id="d-${id}"><rect x="${x}" y="30" width="108" height="54" rx="10"/><text class="baslik" x="${x + 54}" y="53" text-anchor="middle">${baslik}</text><text class="alt" id="da-${id}" x="${x + 54}" y="71" text-anchor="middle"></text></g>`;
    $('#akis').innerHTML = `
      <defs>${ok('mavi', RENK.mavi)}${ok('yesil', RENK.yukari)}${ok('kirmizi', RENK.asagi)}${ok('gri', RENK.metin3)}</defs>
      <g class="ok" id="o-basla"><path class="ok-yol" d="M112,46 L144,46" style="stroke:${rgba(RENK.mavi, 0.55)}" marker-end="url(#uc-mavi)"/><text class="ok-yazi" x="129" y="22" text-anchor="middle">başla</text></g>
      <g class="ok" id="o-iptal"><path class="ok-yol" d="M146,70 L114,70" style="stroke:${rgba(RENK.metin3, 0.6)}" stroke-dasharray="3 3" marker-end="url(#uc-gri)"/><text class="ok-yazi" x="129" y="100" text-anchor="middle">vazgeç</text></g>
      <g class="ok" id="o-al"><path class="ok-yol" d="M254,57 L286,57" style="stroke:${rgba(RENK.yukari, 0.55)}" marker-end="url(#uc-yesil)"/><text class="ok-yazi" x="270" y="22" text-anchor="middle">AL</text></g>
      <g class="ok" id="o-sat"><path class="ok-yol" d="M342,84 L342,118 Q342,126 334,126 L66,126 Q58,126 58,118 L58,88" style="stroke:${rgba(RENK.asagi, 0.55)}" marker-end="url(#uc-kirmizi)"/>
        <rect x="181" y="117" width="38" height="18" rx="4" fill="${RENK.panel2}"/><text class="ok-yazi" x="200" y="130" text-anchor="middle">SAT</text></g>
      ${dugum('bekle', 4, '1 · Bekliyor')}${dugum('dipAra', 146, '2 · Dip arıyor')}${dugum('coinde', 288, '3 · Coinde')}`;
  }

  const OK_RENK = { basla: RENK.mavi, iptal: RENK.metin3, al: RENK.yukari, sat: RENK.asagi };
  function acikIslemNet(i, d) {
    const t = R.islemler.find(x => x.girisIdx === d.girisIdx);
    if (!t) return (M.c[i] / d.giris - 1) * 100;
    return ((t.adet * M.c[i] * (1 - strateji().para.masraf / 100) - t.harca) / t.harca) * 100;
  }
  function botPaneli() {
    const lg = R.log[k];
    $('#botZaman').textContent = M ? tarih(M.t[k]) : '';
    const asama = lg && lg.sonra ? lg.sonra.asama : 'bekle';
    const ASAMA_RENK = { bekle: RENK.metin2, dipAra: RENK.mavi, coinde: RENK.yukari };
    for (const a of ['bekle', 'dipAra', 'coinde']) {
      const g = document.getElementById('d-' + a);
      const aktif = a === asama && lg && lg.a !== 'isinma';
      g.classList.toggle('aktif', !!aktif);
      const r = g.querySelector('rect');
      r.style.fill = aktif ? rgba(ASAMA_RENK[a], 0.18) : '';
      r.style.stroke = aktif ? ASAMA_RENK[a] : '';
    }
    const d = lg && lg.sonra;
    $('#da-bekle').textContent = 'fırsat bekler';
    $('#da-dipAra').textContent = d && d.asama === 'dipAra' ? 'dip ' + f(d.enDusuk) : 'en düşüğü izler';
    $('#da-coinde').textContent = d && d.asama === 'coinde' ? Bicim.yuzde(acikIslemNet(k, d)) + ' (masraflı)' : 'satışı bekler';
    const olayTip = lg && lg.olay ? lg.olay.tip : null;
    for (const o of ['basla', 'iptal', 'al', 'sat']) {
      const g = document.getElementById('o-' + o);
      const parla = olayTip === o;
      g.classList.toggle('parla', parla);
      g.querySelector('.ok-yol').style.stroke = parla ? OK_RENK[o] : rgba(OK_RENK[o], o === 'iptal' ? 0.6 : 0.55);
    }
    gecisleriYaz(olayTip);
    $('#dusunce').innerHTML = dusunceHTML(k);
  }

  function gecisleriYaz(olayTip) {
    const s = strateji();
    const aktif = yer => KURALLAR.filter(kr => kr.yer === yer && acikMi(kr, s))
      .sort((a, b) => (a.oncelik || 5) - (b.oncelik || 5)).map(kr => kr.kisa(kuralAyari(kr, s)));
    const b = aktif('baslat'), t = aktif('tetik'), v = aktif('vazgec'), st = aktif('sat');
    const satirlar = [
      ['basla', 'başla', b.length ? b.join(' ve ') : 'kural kapalı: hep dip arar'],
      ['al', 'AL', t.length ? t.join(' ve ') : 'kural kapalı: hemen alır'],
      ['sat', 'SAT', st.length ? st.join(' ya da ') : 'kural kapalı: hiç satmaz'],
      ['iptal', 'vazgeç', v.length ? v.join(' ya da ') : 'kural kapalı: hiç vazgeçmez'],
    ];
    $('#gecisler').innerHTML = satirlar.map(([id, ad, kural]) =>
      `<div class="gecis ${olayTip === id ? 'parla' : ''}" style="border-left-color:${OK_RENK[id]}"><span class="g-ad" style="color:${OK_RENK[id]}">${ad}</span><span class="g-kural">${kural}</span></div>`).join('');
  }

  function kontrolListe(kontroller) {
    if (!kontroller || !kontroller.length) return '';
    return `<ul class="kontroller">${kontroller.map(c => `<li><span class="isaret ${c.ok ? 'ok' : 'yok'}"></span><div><div class="ad">${KURAL[c.id].ad}</div><div class="not">${metin(c.not)}</div></div></li>`).join('')}</ul>`;
  }

  function olayHTML(o) {
    if (o.tip === 'basla') return `<p class="olay basla">Bu mumda dip aramaya başladı.</p>`;
    if (o.tip === 'al') return `<p class="olay al">Bu mumda ALDI: ${f(o.fiyat)} fiyatından.</p>`;
    if (o.tip === 'iptal') return `<p class="olay iptal">Vazgeçti (${KURAL[o.kural] ? KURAL[o.kural].ad.toLowerCase() : ''}): ${metin(o.not)}</p>`;
    if (o.tip === 'sat') {
      const t = R.islemler[o.islem - 1];
      return `<p class="olay ${t.net >= 0 ? 'sat-k' : 'sat-z'}">Bu mumda SATTI (${KURAL[o.kural].ad.toLowerCase()}): ${f(o.fiyat)} · sonuç ${Bicim.yuzde(t.yuzde)}</p>`;
    }
    return '';
  }

  function dusunceHTML(i) {
    const lg = R.log[i];
    if (!lg || lg.a === 'isinma') return `<div class="bas">Isınıyor</div><div class="not">${isinmaYazisi()}</div>`;
    const d = lg.sonra;
    let bas;
    if (d.asama === 'bekle') bas = 'Bekliyor';
    else if (d.asama === 'dipAra') bas = `Dip arıyor. Şimdiye kadarki en düşük fiyat ${f(d.enDusuk)}`;
    else bas = `Coinde. ${f(d.giris)} fiyatından aldı. Fiyat farkı ${Bicim.yuzde((M.c[i] / d.giris - 1) * 100)}, masraflar düşünce ${Bicim.yuzde(acikIslemNet(i, d))}`;
    let h = `<div class="bas">${bas}</div>`;
    let bakilan = lg;
    if (lg.kapanmadi) {
      h += `<p class="olay iptal">Bu mum henüz kapanmadı. Bot karar vermek için kapanışını bekliyor.</p>`;
      const onceki = i > 0 ? R.log[i - 1] : null;
      if (onceki && onceki.a !== 'isinma') bakilan = onceki;
    }
    if (lg.olay) h += olayHTML(lg.olay);
    if (bakilan.k.length) {
      const ne = bakilan.a === 'bekle' ? 'Dip aramaya başlamak için baktığı şartlar' : bakilan.a === 'dipAra' ? 'Vazgeçmek ve almak için baktığı şartlar' : 'Satmak için baktığı şartlar';
      const ek = bakilan !== lg ? ' (son kapanan mumda)' : '';
      h += `<div class="not" style="margin:4px 0 6px;color:var(--metin3)">${ne}${ek}:</div>` + kontrolListe(bakilan.k);
    }
    if (d.asama !== 'bekle') {
      const sv = Motor.seviyeler(R, i, d).filter(s => isFinite(s.fiyat));
      if (d.asama === 'coinde') sv.unshift({ ad: 'Alış fiyatı', fiyat: d.giris, renk: 'metin' });
      if (sv.length) h += `<div class="seviyeler">${sv.map(s => `<span><i style="border-color:${RENK[s.renk] || RENK.metin2}"></i>${s.ad}</span><b>${f(s.fiyat)}</b>`).join('')}</div>`;
    }
    return h;
  }

  // ---------- inceleme: tıklanan mum ya da işlem ----------
  function incele(i) {
    if (!R || i == null || i > k) return;
    secili = i;
    const t = R.islemler.find(t => t.girisIdx <= k && i >= t.dipBas && i <= (t.cikisIdx != null && t.cikisIdx <= k ? t.cikisIdx : k));
    seciliIslem = t ? t.no : null;
    const el = $('#inceleme');
    el.hidden = false;
    el.innerHTML = (t ? islemHikayesi(t) : mumHikayesi(i)) + (aoAcik() ? aoFpKutu(i) : ''); // 0.9: o mumun footprint'i
    tabloCiz(true);
    $('#sag').scrollTo({ top: 0, behavior: 'smooth' });
  }
  function incelemeKapat() {
    secili = null; seciliIslem = null;
    $('#inceleme').hidden = true;
    if (R) tabloCiz(true);
  }

  function islemHikayesi(t) {
    const kapandi = t.cikisIdx != null && t.cikisIdx <= k;
    const masraf = strateji().para.masraf / 100;
    const net = kapandi ? t.net : t.adet * M.c[k] * (1 - masraf) - t.harca;
    const yzd = (net / t.harca) * 100;
    const adimlar = [];
    if (t.dipBas !== t.girisIdx) {
      adimlar.push(`<li style="--r:${RENK.mavi}"><div class="adim-ust"><b>Dip aramaya başladı</b><time>${tarih(M.t[t.dipBas])}</time></div>
        ${t.baslatKontrol.length ? kontrolListe(t.baslatKontrol) : '<p>Başlatma kuralı kapalı, bot her an dip arıyordu.</p>'}</li>`);
    }
    adimlar.push(`<li style="--r:${RENK.yukari}"><div class="adim-ust"><b>ALDI</b><time>${tarih(M.t[t.girisIdx])}</time></div>
      ${kontrolListe(t.tetikKontrol)}
      <p>${f(t.giris)} fiyatından ${Bicim.para(t.harca)} değerinde aldı. Masraf: ${Bicim.para(t.girisMasraf)}.</p>
      ${t.dipBas !== t.girisIdx ? `<p>Dip arama ${t.girisIdx - t.dipBas} mum sürdü. Gerçek dip ${f(t.enDusuk)} idi, bot dibin ${Bicim.yuzde((t.giris / t.enDusuk - 1) * 100)} üstünden aldı.</p>` : ''}</li>`);
    if (kapandi) {
      adimlar.push(`<li style="--r:${t.net >= 0 ? RENK.yukari : RENK.asagi}"><div class="adim-ust"><b>SATTI</b><span>${KURAL[t.cikisKural].ad}</span><time>${tarih(M.t[t.cikisIdx])}</time></div>
        <ul class="kontroller"><li><span class="isaret ok"></span><div><div class="not">${metin(t.cikisNot)}</div></div></li></ul>
        <p>${f(t.cikis)} fiyatından sattı. Eline ${Bicim.para(t.harca + t.net)} geçti. Masraf: ${Bicim.para(t.cikisMasraf)}.</p>
        <p>Aldıktan sonra görülen en yüksek fiyat ${f(t.tepe)} idi. Bot tepenin ${Bicim.yuzde((t.cikis / t.tepe - 1) * 100)} altından sattı.</p></li>`);
    } else {
      adimlar.push(`<li style="--r:${RENK.vurgu}"><div class="adim-ust"><b>Hâlâ coinde</b><time>${tarih(M.t[k])}</time></div>
        <p>Şu anki fiyat ${f(M.c[k])}. Satış şartlarından biri gelince satacak.</p></li>`);
    }
    const mumSay = (kapandi ? t.cikisIdx : k) - t.girisIdx;
    const brutY = ((kapandi ? t.cikis : M.c[k]) / t.giris - 1) * 100;
    return `<div class="inc-ust"><h3>İşlem #${t.no} (simülatör)</h3><span class="hap ${isaretSinif(net)}">${Bicim.yuzde(yzd)} · ${Bicim.paraIsaret(net)}${kapandi ? '' : ' (açık)'}</span><button class="kapat" data-eylem="kapat" aria-label="Kapat">×</button></div>
      <ol class="hikaye">${adimlar.join('')}</ol>
      <p class="inc-sonuc">${mumSay} mum sürdü (${sureYaz(mumSay * ARALIK[S.aralik].ms)}). Fiyat farkı ${Bicim.yuzde(brutY)}; masraflar düşünce ${Bicim.yuzde(yzd)}.</p>
      <div class="inc-butonlar"><button class="btn kucuk" data-eylem="islemeYaklas" data-no="${t.no}">Grafikte yakınlaş</button></div>`;
  }

  function mumHikayesi(i) {
    return `<div class="inc-ust"><h3>${tarih(M.t[i])} mumu</h3><button class="kapat" data-eylem="kapat" aria-label="Kapat">×</button></div>
      <div class="mum-bilgi">Açılış ${f(M.o[i])} · En yüksek ${f(M.h[i])} · En düşük ${f(M.l[i])} · Kapanış ${f(M.c[i])}</div>
      <div class="dusunce">${dusunceHTML(i)}</div>`;
  }

  // Grafiği en yeni muma yaslar (kaydırma animasyonu olmadan; yakınlık aynı kalır)
  function sagaYasla() {
    if (!M) return;
    const ts = anaGrafik.timeScale();
    const vr = ts.getVisibleLogicalRange();
    const sag = M.c.length - 1 + 6; // grafiğin sağ boşluğu (rightOffset) 6 mum
    if (!vr) { ts.scrollToRealTime(); return; }
    const gen = vr.to - vr.from;
    if (Math.abs(vr.to - sag) > 0.5) ts.setVisibleLogicalRange({ from: sag - gen, to: sag });
  }

  function isleminAraligi(t) {
    const bit = t.cikisIdx != null && t.cikisIdx <= k ? t.cikisIdx : k;
    const bas = Math.min(t.dipBas, t.girisIdx);
    const pay = Math.max(15, Math.round((bit - bas) * 0.6));
    anaGrafik.timeScale().setVisibleLogicalRange({ from: bas - pay, to: bit + pay });
  }
  // Kumbaranın bir işlemine yakınlaş
  function kumbaraIslemineYaklas(t) {
    if (!M || !t.alis) return;
    const a = mumIndeksi(t.alis.bitis);
    const b = t.satis ? mumIndeksi(t.satis.bitis) : k;
    if (a < 0) return;
    const pay = Math.max(15, Math.round((b - a) * 0.6));
    anaGrafik.timeScale().setVisibleLogicalRange({ from: a - pay, to: b + pay });
  }

  // ---------- kural durum hapları ----------
  function durumHaplari() {
    if (!R) return;
    let lg = R.log[k];
    if (lg && lg.kapanmadi && k > 0) lg = R.log[k - 1];
    const bakilan = new Map((lg && lg.k ? lg.k : []).map(c => [c.id, c.ok]));
    for (const el of document.querySelectorAll('.kural[data-id]')) {
      const id = el.dataset.id;
      const hap = el.querySelector('.durum');
      if (!hap) continue;
      if (bakilan.has(id)) {
        const ok = bakilan.get(id);
        hap.className = 'durum ' + (ok ? 'evet' : 'hayir');
        hap.textContent = ok ? 'şu an sağlandı' : 'şu an sağlanmadı';
        hap.title = 'Son kapanan mumda bu kural ' + (ok ? 'sağlandı' : 'sağlanmadı');
      } else {
        hap.className = 'durum';
        hap.textContent = 'bu aşamada bakılmıyor';
        hap.title = 'Bot şu an başka bir aşamada, bu kurala bakmıyor';
      }
    }
  }

  // ---------- dosyadan gelen ayarlar (salt okunur) ----------
  // Vites merdiveni: sütunlar = vitesler, satırlar = Fib bantları. Simülatördekinin tıklanamayan hâli.
  function vitesMerdiveni(kr, d) {
    const tablo = kr.ayarlar.filter(a => a.grup === 'vites');
    const secenekler = tablo[0].secenekler;
    const satis = kr.yer === 'sat';
    const nokta = satis ? RENK.turuncu : RENK.mavi;
    const W = 400, SOL = 66, SUT = (W - SOL) / 5, UST = 34, SATIR = 16, YOKBOSLUK = 5;
    const satirY = r => UST + 4 + r * SATIR + (secenekler[r].v === VITES_YOK ? YOKBOSLUK : 0);
    const merkezY = r => satirY(r) + SATIR / 2;
    const H = satirY(secenekler.length - 1) + SATIR + 6;
    const sutunX = j => SOL + j * SUT;
    const parca = [];
    tablo.forEach((a, j) => {
      const sozler = VITES.AD[j].split(' ');
      const cx = sutunX(j) + SUT / 2, cy = (UST - 4) / 2 + 1;
      parca.push(`<rect x="${sutunX(j) + 2}" y="2" width="${SUT - 4}" height="${UST - 6}" rx="5" fill="${rgba(VITES_RENK[j], 0.18)}" stroke="${rgba(VITES_RENK[j], 0.55)}"/>`);
      parca.push(sozler.length > 1
        ? `<text class="bas" x="${cx}" y="${cy - 2}" text-anchor="middle">${sozler[0]}</text><text class="bas" x="${cx}" y="${cy + 10}" text-anchor="middle">${sozler.slice(1).join(' ')}</text>`
        : `<text class="bas" x="${cx}" y="${cy + 4}" text-anchor="middle">${sozler[0]}</text>`);
    });
    tablo.forEach((a, j) => {
      const kutular = secenekler.map((o, r) => `<rect class="vm-hucre" x="${sutunX(j) + 3}" y="${satirY(r) + 1}" width="${SUT - 6}" height="${SATIR - 2}" rx="3"><title>${VITES.AD[j]}: ${o.ad}</title></rect>`).join('');
      parca.push(`<g class="vm-sutun" data-vm-sutun="${j}"><rect class="vm-arka" x="${sutunX(j) + 1}" y="${UST}" width="${SUT - 2}" height="${H - UST - 2}" rx="4"/>${kutular}</g>`);
    });
    secenekler.forEach((o, r) => parca.push(`<text class="${o.v === 0 ? 'orta' : ''}" x="${SOL - 6}" y="${merkezY(r) + 4}" text-anchor="end">${o.ad}</text>`));
    const ortaR = secenekler.findIndex(o => o.v === 0);
    parca.push(`<line x1="${SOL}" y1="${merkezY(ortaR)}" x2="${W}" y2="${merkezY(ortaR)}" stroke="${rgba(RENK.metin2, 0.35)}" stroke-dasharray="3 3" pointer-events="none"/>`);
    const yokR = secenekler.findIndex(o => o.v === VITES_YOK);
    parca.push(`<line x1="4" y1="${satirY(yokR) - 3}" x2="${W}" y2="${satirY(yokR) - 3}" stroke="${RENK.cizgi2}" pointer-events="none"/>`);
    const noktalar = tablo.map((a, j) => {
      const r = secenekler.findIndex(o => o.v === d[a.k]);
      return { x: sutunX(j) + SUT / 2, y: merkezY(Math.max(0, r)), yok: d[a.k] === VITES_YOK };
    });
    let yol = '', bagli = false;
    for (const n of noktalar) { if (n.yok) { bagli = false; continue; } yol += `${bagli ? 'L' : 'M'}${n.x},${n.y} `; bagli = true; }
    if (yol) parca.push(`<path d="${yol}" fill="none" stroke="${rgba(nokta, 0.6)}" stroke-width="1.5" pointer-events="none"/>`);
    for (const n of noktalar) {
      parca.push(n.yok
        ? `<path d="M${n.x - 4},${n.y - 4} L${n.x + 4},${n.y + 4} M${n.x + 4},${n.y - 4} L${n.x - 4},${n.y + 4}" stroke="${RENK.metin2}" stroke-width="1.6" pointer-events="none"/>`
        : `<circle cx="${n.x}" cy="${n.y}" r="5" fill="${nokta}" stroke="${RENK.panel2}" stroke-width="1.5" pointer-events="none"/>`);
    }
    return `<div class="vites-merdiveni salt-oku"><div class="vm-baslik"><span>${satis ? 'Satış' : 'Alış'} bandı, her vites için ayrı</span><span>dosyadan</span></div>
      <svg viewBox="0 0 ${W} ${H}" role="img" aria-label="${satis ? 'Satış' : 'Alış'} bandı vites tablosu">${parca.join('')}</svg></div>`;
  }
  // Oynatıcıdaki mumun vitesine ait sütunu vurgula
  function merdivenSutunlari() {
    const d = vitesDz && M ? vitesDz[k] : -1;
    for (const g of document.querySelectorAll('.vm-sutun')) g.classList.toggle('aktif', +g.dataset.vmSutun === d);
  }

  // Bot panelindeki vites kolu: şu anki vites, eğim, bu vitesin bantları, Fib genişliği
  function vitesKolu() {
    const el = $('#vitesKolu');
    const goster = !!(R && vitesDz && (kullanilanlar().has('vites') || vitesGorunur()));
    el.hidden = !goster;
    if (!goster) return;
    const d = vitesDz[k], e = R.b.g('vites').egim[k], s = strateji();
    const seritler = VITES.AD.map((ad, j) => `<span class="${j === d ? 'aktif' : ''}" style="${j === d ? `background:${VITES_RENK[j]}` : `border-color:${rgba(VITES_RENK[j], 0.35)}`}">${ad}</span>`).join('');
    const parca = [];
    if (d >= 0) {
      const anahtarAd = VITES.ANAHTAR[d];
      for (const [id, ne] of [['fbbAlt', 'alış'], ['fbbUst', 'satış']]) {
        const kr = KURAL[id];
        if (!acikMi(kr, s)) continue;
        const ks = kuralAyari(kr, s);
        if (!ks.vites) continue;
        const o = kr.ayarlar.find(x => x.k === anahtarAd).secenekler.find(x => x.v === ks[anahtarAd]);
        parca.push(`${ne} <b>${o ? o.ad : '?'}</b>`);
      }
    }
    let gen = '';
    const std = R.b.g('fbb').std;
    if (isFinite(std[k])) {
      let t = 0, n = 0;
      for (let i = Math.max(0, k - 199); i <= k; i++) if (isFinite(std[i])) { t += std[i]; n++; }
      if (n > 20 && t > 0) {
        const oran = std[k] / (t / n);
        gen = `Fib bantları normalin <b>${nf(1).format(oran)}</b> katı${oran >= 1.15 ? ' geniş' : oran <= 0.85 ? ' dar' : ''}`;
      }
    }
    el.innerHTML = `<div class="vk-ust"><span class="vk-baslik">Vites (trend durumu)</span><span class="vk-deger">${d >= 0 ? VITES.AD[d] : 'hesaplanıyor'}${isFinite(e) ? ` · eğim ${e >= 0 ? '+' : '−'}${nf(2).format(Math.abs(e))}` : ''}</span></div>
      <div class="vk-seritler">${seritler}</div>
      ${parca.length || gen ? `<div class="vk-alt">${parca.length ? 'Bu viteste ' + parca.join(' · ') : ''}${parca.length && gen ? ' · ' : ''}${gen}</div>` : ''}`;
  }

  function gostergeAciklama(g, a) {
    let h = g.aciklama(a);
    if (g.id === 'ema' && a.hizli >= a.yavas) h += ' <span class="neg">Hızlı EMA, yavaştan küçük olmalı.</span>';
    if (g.id === 'vites' && a.sinir1 >= a.sinir2) h += ' <span class="neg">Yatay sınırı, sert sınırından küçük olmalı.</span>';
    return h;
  }
  const kullanilanlar = () => Motor.kullanilanGostergeler(strateji(), KURALLAR);

  function degerYaz(a, v) {
    if (a.tip === 'anahtar') return v ? 'açık' : 'kapalı';
    if (a.tip === 'secim') { const o = a.secenekler.find(x => x.v === v); return o ? o.ad : String(v); }
    if (typeof v !== 'number' || !isFinite(v)) return '–';
    const s = degerMetni(v, a);
    return a.birim === '%' ? '%' + s : s + (a.birim ? ' ' + a.birim : '');
  }
  function kuralDegerleri(kr, d) {
    const out = [];
    let merdiven = false;
    for (const a of kr.ayarlar) {
      if (a.goster && !a.goster(d)) continue;
      if (a.grup === 'vites') { if (!merdiven) { out.push(vitesMerdiveni(kr, d)); merdiven = true; } continue; }
      out.push(`<div class="deger-satir"><span>${a.etiket}</span><b>${degerYaz(a, d[a.k])}</b></div>`);
    }
    return out.length ? `<div class="degerler">${out.join('')}</div>` : '';
  }
  const BOS_ASAMA = {
    baslat: 'Açık kural yok: bot her an dip arar.',
    tetik: 'Açık kural yok: dip aramaya başlar başlamaz alır.',
    vazgec: 'Açık kural yok: dip aramadan hiç vazgeçmez.',
    sat: 'Açık kural yok: hiç satmaz.',
  };

  function kurallariCiz() {
    const panel = $('#kuralPaneli');
    if (!AKTIF || !AKTIF.strateji) { panel.innerHTML = ''; $('#ayarBaslik').hidden = true; return; }
    $('#ayarBaslik').hidden = false;
    const s = strateji();
    const kul = kullanilanlar();
    const html = [];
    html.push(`<div class="grup"><div class="grup-ust" style="--g:${RENK.vurgu}"><span class="grup-no">çizgiler</span><div><h3>Göstergeler</h3><p>Kurallar bu çizgilere bakar</p></div></div>`);
    for (const g of GOSTERGELER) {
      const a = gostergeAyari(g, s);
      const kullan = kul.has(g.id);
      const renkler = (GOSTERGE_RENK[g.id] || []).map(r => `<i style="background:${RENK[r]}"></i>`).join('');
      const satirlar = g.ayarlar.map(x => `<div class="deger-satir"><span>${x.etiket}</span><b>${degerYaz(x, a[x.k])}</b></div>`).join('');
      html.push(`<div class="kural ${kullan ? '' : 'kapali'}" data-gosterge="${g.id}" style="--g:${g.id === 'vites' ? RENK.metin2 : RENK[GOSTERGE_RENK[g.id][0]]}">
        <div class="gosterge-bas"><span class="renkler">${renkler}</span><span class="kural-ad">${g.ad}</span>
          <span class="kullanim ${kullan ? 'evet' : ''}">${kullan ? 'kullanılıyor' : 'kullanılmıyor'}</span></div>
        <div class="kural-govde"><p class="kural-cumle">${gostergeAciklama(g, a)}</p><div class="degerler">${satirlar}</div></div>
        <label class="goz"><span class="anahtar"><input type="checkbox" data-goz="${g.id}" ${S.gorunum.gosterge[g.id] ? 'checked' : ''} aria-label="${g.ad} grafikte göster"><span></span></span>grafikte göster</label>
      </div>`);
    }
    html.push('</div>');
    for (const yer of ['baslat', 'tetik', 'vazgec', 'sat']) {
      const as = ASAMALAR[yer];
      const hepsi = KURALLAR.filter(x => x.yer === yer);
      const acik = hepsi.filter(x => acikMi(x, s)), kapali = hepsi.filter(x => !acikMi(x, s));
      html.push(`<div class="grup"><div class="grup-ust" style="--g:${YER_RENK[yer]}"><span class="grup-no">${as.no}</span><div><h3>${as.baslik}</h3><p>${as.alt}</p></div></div>`);
      if (!acik.length) html.push(`<p class="kapali-liste">${BOS_ASAMA[yer]}</p>`);
      for (const kr of acik) {
        const d = kuralAyari(kr, s);
        html.push(`<div class="kural" data-id="${kr.id}" style="--g:${YER_RENK[yer]}">
          <div class="kural-bas"><span class="kural-ad">${kr.ad}</span><span class="durum">–</span></div>
          <div class="kural-govde"><p class="kural-cumle">${kr.cumle(d)}</p>${kuralDegerleri(kr, d)}<p class="kural-ornek">${kr.ornek}</p></div></div>`);
      }
      if (kapali.length) html.push(`<p class="kapali-liste">Kapalı: ${kapali.map(x => x.ad).join(' · ')}</p>`);
      html.push('</div>');
    }
    html.push(`<div class="grup"><div class="grup-ust" style="--g:${RENK.turuncu}"><span class="grup-no">para</span><div><h3>Simülatörün varsayımı</h3><p>Sadece simülatörün sanal hesabını etkiler, kararları değil</p></div></div>
      <div class="kural" style="--g:${RENK.turuncu}"><div class="degerler">
        <div class="deger-satir"><span>Başlangıç parası (simülatörde)</span><b>${Bicim.para(s.para.baslangic)}</b></div>
        <div class="deger-satir"><span>İşlem başına masraf (komisyon + kayma)</span><b>%${nfEn(3).format(s.para.masraf)}</b></div>
      </div><p class="kural-ornek">Kumbaranın gerçek dolum fiyatı ve gerçek komisyonu, alttaki "Kumbaranın işlemleri" tablosunda bununla yan yana gösterilir.</p></div></div>`);
    panel.innerHTML = html.join('');
    const kb = seciliKumbara();
    $('#ayarKapsam').innerHTML = kb
      ? `Çalışan kumbaranın ayarı (kumbara açılırken <b>${esc(kb.dosya)}</b> dosyasından alındı), değiştirilemez.`
      : `<b>${esc(AKTIF.ad)}</b> dosyasından, değiştirilemez. Değiştirmek için simülatörde değiştirip yeniden <b>Dışa aktar</b>'a bas.`;
    if (R) durumHaplari();
    merdivenSutunlari();
  }

  // ---------- strateji listesi ----------
  function siraNo(d) {
    if (dosyaKumbarasi(d.ad)) return -1;
    if (d.yetim) return -1;
    if (d.durum === 'gecerli' && (!d.icerik || uyumKontrol(d.icerik).ok)) return 0;
    if (d.durum === 'suresiDoldu') return 1;
    return 2;
  }
  function calisabilir(d) { return d.durum !== 'hatali' && uyumKontrol(d.icerik).ok; }
  function rozetHTML(d) {
    if (d.yetim) return '<span class="rozet doldu">dosyası silindi</span>';
    if (!calisabilir(d)) return '<span class="rozet hatali">çalıştırılamaz</span>';
    if (d.durum === 'suresiDoldu' || d.bitis <= Date.now()) return '<span class="rozet doldu">süresi doldu</span>';
    return '<span class="rozet gecerli">geçerli</span>';
  }
  function hataMetni(d) {
    if (!d || d.yetim) return '';
    if (d.durum === 'hatali') return d.hata || 'Dosya okunamadı';
    const u = uyumKontrol(d.icerik);
    return u.ok ? '' : u.hata;
  }
  // Ömür çubuğu: data-bitis ile her saniye güncellenir. genis: sağdaki kartta toplam ömrü de yaz.
  function omurHTML(d, genis) {
    if (!d.bitis || !d.olusturuldu) return '';
    const sag = genis ? `${sureMetni(d.bitis - d.olusturuldu)} ömür · biter ${gunSaat(d.bitis)}` : `biter ${gunSaat(d.bitis)}`;
    return `<div class="omur" data-bitis="${d.bitis}" data-bas="${d.olusturuldu}">
      <div class="omur-cubuk"><i></i></div>
      <div class="omur-yazi"><span class="omur-kalan"></span><span>${sag}</span></div></div>`;
  }
  // Listede kumbaranın kısa durumu
  function kumbaraSeritHTML(kb) {
    const yz = (kb.deger / kb.baslangicTutar - 1) * 100;
    let ne = kb.emirde ? 'emir veriliyor…' : acikIslemi(kb) ? 'coinde' : kb.durum === 'calisiyor' ? 'alım bekliyor' : '';
    if (kb.durum === 'sureDoldu') ne = acikIslemi(kb) ? 'ömür doldu · satış bekliyor' : 'ömür doldu';
    if (kb.durum === 'durduruluyor') ne = kb.devam ? 'yeni ayar bekliyor' : 'durduruluyor';
    return `<div class="skumbara ${kb.mod}"><span class="mod-rozet ${kb.mod}">${kb.mod === 'gercek' ? 'GERÇEK' : 'SANAL'}</span>
      <span class="sk-deger">${nf(2).format(kb.deger)} <small>USDT</small></span><b class="${isaretSinif(yz)}">${Bicim.yuzde(yz, 1)}</b><span class="sk-ne">${ne}</span></div>`;
  }
  function kartHTML(d) {
    const ic = d.icerik || {}, gs = ic.gecmisSonuc;
    const ad = d.sembol ? `${esc(varlikAdi(d.sembol))}<small>${d.sembol.endsWith('USDT') ? 'USDT' : ''}</small>` : esc(d.ad);
    const hata = hataMetni(d);
    const kb = dosyaKumbarasi(d.ad);
    return `<button class="skart ${S.secili === d.ad ? 'secili' : ''} ${hata ? 'calisamaz' : ''} ${kb ? 'kumbarali ' + kb.mod : ''}" data-ad="${esc(d.ad)}">
      <div class="skart-ust"><span class="skart-ad">${ad}</span>${d.aralik && ARALIK[d.aralik] ? `<span class="skart-aralik">${ARALIK[d.aralik].ad}</span>` : ''}${rozetHTML(d)}</div>
      ${kb ? kumbaraSeritHTML(kb) : ''}
      ${hata ? `<div class="skart-hata">${esc(hata)}</div>` : d.yetim ? '<div class="skart-not">Dosya klasörden silindi; kumbara son işlemini bitiriyor.</div>' : omurHTML(d)}
      ${!hata && gs ? `<div class="skart-gecmis">Dışa aktarırken: bot ${yuzdeHTML(gs.botYuzde)} · tutsan ${yuzdeHTML(gs.alTutYuzde)} · ${adetYaz(gs.islemSayisi)} işlem</div>` : ''}
      ${d.not ? `<div class="skart-not">${esc(d.ad)}: ${esc(d.not)}</div>` : ''}
    </button>`;
  }
  // Listede gösterilenler: klasördeki dosyalar + dosyası silinmiş ama hâlâ çalışan kumbaralar
  function listeOgeleri() {
    if (!LISTE) return [];
    const ogeler = LISTE.dosyalar.slice();
    const adlar = new Set(ogeler.map(d => d.ad));
    for (const kb of aktifKumbaralar()) {
      if (adlar.has(kb.dosya)) continue;
      adlar.add(kb.dosya);
      ogeler.push({ ad: kb.dosya, sembol: kb.sembol, aralik: kb.aralik, durum: 'kumbara', bitis: kb.bitis, olusturuldu: kb.olusturuldu, icerik: null, yetim: true });
    }
    return ogeler;
  }
  let listeImza = '';
  function listeCiz(zorla) {
    const el = $('#stratejiListe');
    if (!LISTE) return;
    const ogeler = listeOgeleri();
    const kbImza = aktifKumbaralar().map(x => [x.dosya, x.mod, x.durum, x.emirde, acikIslemi(x) ? 1 : 0, Math.round(x.deger * 100), !!x.devam]);
    const imza = JSON.stringify([LISTE.klasor, LISTE.klasorVar, S.secili, LISTE.hata || '', ogeler.map(d => [d.ad, d.degisti, d.durum, d.hata, d.bitis]), kbImza]);
    if (imza === listeImza && !zorla) return;
    listeImza = imza;
    const gecerli = LISTE.dosyalar.filter(d => siraNo(d) <= 0 && d.durum === 'gecerli').length;
    const calisan = aktifKumbaralar().length;
    $('#listeSayi').textContent = LISTE.klasorVar ? `${LISTE.dosyalar.length} dosya · ${gecerli} geçerli${calisan ? ` · ${calisan} kumbara` : ''}` : '';
    if (!LISTE.klasorVar && !ogeler.length) {
      el.innerHTML = `<div class="liste-bos"><b>stratejiler klasörü bulunamadı</b>${LISTE.klasor ? `<span class="yol-kucuk">${esc(LISTE.klasor)}</span>` : ''}
        <span>Kripto Simülatör klasörünün içindeki <b>stratejiler</b> klasörünü seç.</span><button class="btn kucuk" data-eylem="klasorSec">Klasör seç</button></div>`;
      return;
    }
    if (LISTE.hata && !ogeler.length) { el.innerHTML = `<div class="liste-bos"><b>Klasör okunamadı</b><span>${esc(LISTE.hata)}</span></div>`; return; }
    if (!ogeler.length) {
      el.innerHTML = `<div class="liste-bos"><b>Klasörde strateji yok</b><span>Simülatörde ayarı bulup <b>Dışa aktar</b>'a bas. Dosya burada kendiliğinden görünür.</span></div>`;
      return;
    }
    const sirali = ogeler.slice().sort((a, b) => siraNo(a) - siraNo(b) || (a.bitis || 0) - (b.bitis || 0) || a.ad.localeCompare(b.ad));
    el.innerHTML = sirali.map(kartHTML).join('');
    zamanlariGuncelle();
  }
  function klasorYaz() {
    const el = $('#klasorYol');
    if (!LISTE) return;
    el.textContent = LISTE.klasor || 'bulunamadı';
    el.title = LISTE.klasor || '';
    el.classList.toggle('yok', !LISTE.klasorVar);
  }
  function motorUyarisi() {
    const el = $('#motorUyari');
    const m = LISTE && LISTE.motor;
    if (m && m.durum === 'farkli') {
      el.className = 'motor-uyari';
      el.innerHTML = `<b>Simülatörün motoru Al-Sat'takinden farklı</b> (${m.farkli.map(esc).join(', ')}). Simülatör güncellenmiş olabilir. Al-Sat da güncellenene kadar kararlar simülatördekinden farklı çıkabilir.`;
      el.hidden = false;
    } else el.hidden = true;
  }

  // Her saniye: ömür çubukları, geri sayımlar, sıradaki mumun kapanışı
  function zamanlariGuncelle() {
    const simdi = Date.now();
    for (const el of document.querySelectorAll('.omur[data-bitis]')) {
      const bitis = +el.dataset.bitis, bas = +el.dataset.bas;
      const kalan = bitis - simdi, toplam = Math.max(1, bitis - bas);
      const oran = Math.max(0, Math.min(1, kalan / toplam));
      const i = el.querySelector('.omur-cubuk i');
      i.style.width = (oran * 100).toFixed(2) + '%';
      i.className = kalan <= 0 ? 'bitti' : oran < 0.1 ? 'cokaz' : oran < 0.25 ? 'az' : '';
      el.querySelector('.omur-kalan').innerHTML = kalan <= 0 ? `<b class="neg">${esc(el.dataset.bittiYazi || 'süresi doldu')}</b>` : `<b>${kalanYaz(kalan)}</b> kaldı`;
    }
    for (const el of document.querySelectorAll('[data-geri]')) {
      const kalan = +el.dataset.geri - simdi;
      el.textContent = kalan > 0 ? kalanYaz(kalan) : 'şimdi';
    }
    const sayac = $('#seridiSayac');
    if (M && AKTIF) {
      const ms = ARALIK[S.aralik].ms;
      const son = M.t[M.t.length - 1];
      const kapanis = M.acikSon ? son + ms : son + 2 * ms;
      const kalan = kapanis - borsaSimdi();
      const saatNot = Math.abs(borsaFarki) >= 5000 ? ` · <span class="neg" title="Program farkı hesaba katıyor; istersen Windows saatini eşitle">bilgisayar saati MEXC'den ${nf(0).format(Math.abs(borsaFarki) / 1000)} sn ${borsaFarki < 0 ? 'ileri' : 'geri'}</span>` : '';
      sayac.innerHTML = (kalan > 0 ? `Sıradaki mum kapanışı <b>${saatYaz(kapanis - borsaFarki)}</b> · ${kalanYaz(kalan)} kaldı` : 'Mum kapandı, veri bekleniyor…') + saatNot;
    } else sayac.textContent = '';
  }

  // Sağdaki strateji kartı
  function stratejiKartiCiz() {
    const el = $('#stratejiKart');
    if (!AKTIF) { el.hidden = true; return; }
    el.hidden = false;
    const kb = seciliKumbara();
    const d = AKTIF.dosya;
    if (!d) {
      el.innerHTML = `<div class="sk-ust"><h3>${esc(kb ? kb.sembol : AKTIF.ad)} <small>· ${aralikAd(kb ? kb.aralik : S.aralik)}</small></h3><span class="rozet doldu">dosyası silindi</span></div>
        <p class="kirmizi-not"><b>Dosya klasörden kaldırıldı</b> (süresi dolan dosyalar program tarafından silinir; kopyası kayitlar\\arsiv klasöründe). ${kb ? 'Kumbara kendi kopyasıyla son işlemini bitiriyor.' : 'Grafik, son okunan ayarlarla gösteriliyor.'}</p>`;
      return;
    }
    const ic = d.icerik || {}, gs = ic.gecmisSonuc;
    const hata = hataMetni(d);
    const notlar = [];
    if (hata) notlar.push(`<p class="hata-not"><b>Bu dosya çalıştırılamaz.</b> ${esc(hata)}</p>`);
    else if (d.bitis <= Date.now()) notlar.push(`<p class="kirmizi-not"><b>Süresi doldu.</b> Bu ayar artık bayat sayılır; bununla yeni kumbara başlatılamaz. Program dosyayı birkaç dakika içinde stratejiler klasöründen siler (kopyası kayitlar\\arsiv klasöründe). Simülatörde yeniden dışa aktarırsan ömrü yenilenir.</p>`);
    if (AKTIF.kayboldu) notlar.push(`<p class="kirmizi-not"><b>Dosya klasörden kaldırıldı.</b> Grafik, son okunan ayarlarla gösteriliyor.</p>`);
    if (kb && kb.olusturuldu !== d.olusturuldu) {
      notlar.push(kb.devam
        ? `<p class="adim-not">Dosya simülatörde yeniden dışa aktarıldı. Kumbara eldeki işlemi eski ayarla bitirip aynı parayla yeni ayara geçecek.</p>`
        : `<p class="adim-not">Dosya simülatörde yeniden dışa aktarıldı. Kumbara yeni ayara geçmek için hazırlanıyor.</p>`);
    }
    if (d.not) notlar.push(`<p class="hata-not">${esc(d.not)}</p>`);
    el.innerHTML = `
      <div class="sk-ust"><h3>${d.sembol ? esc(d.sembol) : esc(d.ad)} <small>· ${aralikAd(d.aralik)}</small></h3>${rozetHTML(d)}</div>
      ${hata ? '' : `<div class="sk-omur">${omurHTML(d, true)}</div>`}
      <div class="bilgi-tablo">
        <span>Dosya</span><b class="rakam">${esc(d.ad)}</b>
        ${d.olusturuldu ? `<span>Dışa aktarıldı</span><b>${gunSaat(d.olusturuldu)}</b>` : ''}
        ${gs && gs.ilkMum ? `<span>Ayarın bulunduğu veri</span><b>${nf(0).format(gs.mumSayisi)} mum · ${gunSaat(Date.parse(gs.ilkMum))} – ${gunSaat(Date.parse(gs.sonMum))}</b>` : ''}
        ${gs ? `<span>O günkü sonuç</span><b>bot ${yuzdeHTML(gs.botYuzde)} · tutsan ${yuzdeHTML(gs.alTutYuzde)} · ${adetYaz(gs.islemSayisi)} işlem${gs.karliIslem != null ? ` (${adetYaz(gs.karliIslem)} kârlı)` : ''}</b>` : ''}
        ${!hata ? `<span>Grafikteki mumlar</span><b id="skMum">–</b>` : ''}
      </div>
      ${notlar.join('')}`;
    skMumYaz();
    zamanlariGuncelle();
  }
  function skMumYaz() {
    const el = document.getElementById('skMum');
    if (!el) return;
    if (!M || !GZ) { el.textContent = '–'; return; }
    const n = M.c.length, yeni = GZ.yuklenen ? n - GZ.yuklenen : 0;
    el.textContent = `${nf(0).format(n)} mum · ilk ${tarih(M.t[0])}${yeni > 0 ? ` · açıldığından beri ${yeni} yeni` : ''}`;
  }

  // Grafiğin altındaki durum şeridi
  function seritYaz() {
    const kar = $('#sonKarar');
    const kb = grafikKumbarasi();
    const emir = $('#seritEmir');
    if (kb && kb.emirde) { emir.hidden = false; emir.innerHTML = `<span class="donen mini"></span>${esc(kb.emirAciklama || 'Emir veriliyor')}`; }
    else emir.hidden = true;
    if (!M || !R || !AKTIF) {
      $('#seridiMum').textContent = '';
      kar.hidden = true;
      if (!AKTIF) canliDurum('bos');
      else if (!AKTIF.strateji) canliDurum('durdu', 'Bu dosya çalıştırılamaz');
      else if (canliHal !== 'hata') canliDurum('durdu', 'Yükleniyor');
      return;
    }
    const n = M.c.length;
    $('#seridiMum').innerHTML = `<b>${nf(0).format(n)}</b> mum`;
    const lg = R.log[R.son];
    let ne = 'Isınıyor';
    if (lg && lg.a !== 'isinma') ne = ASAMA_AD[lg.sonra.asama];
    let olay = '';
    if (lg && lg.olay) {
      if (lg.olay.tip === 'al') olay = ` · <b class="pos">ALDI</b> ${f(lg.olay.fiyat)}`;
      else if (lg.olay.tip === 'sat') olay = ` · <b class="neg">SATTI</b> ${f(lg.olay.fiyat)}`;
      else if (lg.olay.tip === 'basla') olay = ' · dip aramaya başladı';
      else if (lg.olay.tip === 'iptal') olay = ' · vazgeçti';
    }
    kar.hidden = R.son < 0;
    const t0 = M.t[R.son], t1 = t0 + ARALIK[S.aralik].ms;
    const aralikYazi = S.aralik === '1d' ? gunSaat(t0).replace(/ \d\d:\d\d$/, '') : `${saatYaz(t0)}–${saatYaz(t1)}`;
    kar.innerHTML = `Son kapanan mum (${aralikYazi}): motor <b>${ne}</b>${olay}`;
    skMumYaz();
  }
  let canliHal = 'bos';
  function canliDurum(hal, yazi) {
    canliHal = hal;
    const el = $('#canliIsaret');
    el.classList.toggle('durdu', hal === 'durdu' || hal === 'bos');
    el.classList.toggle('hata', hal === 'hata');
    $('#canliYazi').textContent = yazi || (hal === 'bos' ? 'Strateji seçilmedi' : hal === 'durdu' ? 'Durdu' : 'Canlı');
  }

  // ---------- izleyiciler: her kumbara (ve ekrandaki önizleme) için mumlar + motor ----------
  const canliAraligi = aralik => ({ '1m': 5, '5m': 10 }[aralik] || 20) * 1000; // simülatörün "Canlı izle"siyle aynı
  const hataYazisi = e => (e && e.message === 'Failed to fetch' ? 'Programla bağlantı koptu. Programı kapatıp yeniden aç.' : (e && e.message) || String(e));
  async function mumGetir(sembol, aralik, adet, bitis) {
    const r = await fetch(`/api/mumlar?borsa=mexc&sembol=${encodeURIComponent(sembol)}&aralik=${aralik}&adet=${adet}${bitis > 0 ? `&bitis=${Math.floor(bitis)}` : ''}`);
    let j;
    try { j = await r.json(); } catch (e) { throw new Error('Programdan cevap okunamadı'); }
    if (!r.ok || j.hata) throw new Error(j.hata || 'Bilinmeyen hata');
    return j;
  }

  class Izleyici {
    constructor(o) {
      this.kumbaraId = o.kumbaraId || null;
      this.dosyaAdi = o.dosyaAdi;
      this.sembol = o.sembol; this.aralik = o.aralik;
      this.icerik = o.icerik || null;
      this.strateji = o.strateji || null;
      this.adet = o.adet || 1000;
      this.ilkMum = o.ilkMum || 0; // sabit başlangıç: kumbara yeniden açılınca aynı mumdan yüklenir
      this.M = null; this.R = null;
      this.durum = 'bos'; this.hata = null; this.kaynak = '';
      this.mesgul = false; this.sonIstek = 0; this.sonDeneme = 0; this.yuklenen = 0;
      this.sonRapor = null; this.raporHata = null;
    }
    get ms() { return ARALIK[this.aralik].ms; }
    hesapla() { this.R = Motor.calistir(this.M, this.strateji, KURALLAR); }
    async yukle() {
      this.durum = 'yukleniyor'; this.hata = null; this.sonDeneme = Date.now();
      let adet = this.adet;
      if (this.ilkMum > 0) adet = Math.max(30, Math.ceil((borsaSimdi() - this.ilkMum) / this.ms) + 3);
      adet = Math.min(ENFAZLA_MUM, adet);
      const j = await mumGetir(this.sembol, this.aralik, adet);
      let mumlar = j.mumlar || [];
      if (this.ilkMum > 0) mumlar = mumlar.filter(x => x[0] >= this.ilkMum);
      if (mumlar.length < 30) throw new Error('Bu coin için yeterli geçmiş veri yok (en az 30 mum gerekli).');
      const m = { t: [], o: [], h: [], l: [], c: [], v: [], acikSon: !!j.acikSon };
      for (const x of mumlar) { m.t.push(x[0]); m.o.push(x[1]); m.h.push(x[2]); m.l.push(x[3]); m.c.push(x[4]); m.v.push(x[5]); }
      this.M = m; this.yuklenen = m.c.length; this.kaynak = j.kaynak || '';
      this.hesapla();
      this.durum = 'hazir'; this.sonIstek = borsaSimdi();
    }
    // Yeni mumları ekler; arada kaçan mum varsa (pencere uzun süre yavaşladıysa) onları da getirir
    async canliTur() {
      if (!this.M || this.mesgul) return false;
      this.mesgul = true; this.sonIstek = borsaSimdi();
      try {
        const m = this.M;
        const eksik = Math.ceil((borsaSimdi() - m.t[m.t.length - 1]) / this.ms) + 2;
        if (eksik > 1000) {
          this.ilkMum = m.t[0];
          await this.yukle(); // çok mum kaçmış: aynı başlangıçtan baştan yükle
          return true;
        }
        const j = await mumGetir(this.sembol, this.aralik, Math.max(3, eksik));
        for (const x of j.mumlar) {
          const son = m.t.length - 1;
          if (x[0] > m.t[son]) { m.t.push(x[0]); m.o.push(x[1]); m.h.push(x[2]); m.l.push(x[3]); m.c.push(x[4]); m.v.push(x[5]); continue; }
          for (let i = son; i >= Math.max(0, son - 5); i--) {
            if (m.t[i] === x[0]) { m.o[i] = x[1]; m.h[i] = x[2]; m.l[i] = x[3]; m.c[i] = x[4]; m.v[i] = x[5]; break; }
          }
        }
        m.acikSon = !!j.acikSon;
        this.hesapla();
        this.hata = null;
        return true;
      } catch (e) {
        this.hata = hataYazisi(e);
        return false;
      } finally {
        this.mesgul = false;
      }
    }
    // Her saniye ve her "tik"te: mum kapanır kapanmaz (1,5 sn sonra) ve düzenli aralıklarla veri ister
    kontrol(gorunur) {
      if (this.durum === 'hata' && this.strateji && Date.now() - this.sonDeneme > 30000) { this.yukleVeBildir(); return; }
      if (!this.M || this.mesgul || this.durum !== 'hazir') return;
      if (this.kumbaraId) korumaBildir(this); // asıl koruma kurulana kadar (30 sn'de bir)
      const simdi = borsaSimdi(), ms = this.ms, son = this.M.t[this.M.t.length - 1];
      const kapanis = this.M.acikSon ? son + ms : son + 2 * ms;
      const kapanisGecti = simdi >= kapanis + 1500 && this.sonIstek < kapanis + 1500;
      const periyot = gorunur ? canliAraligi(this.aralik) : this.kumbaraId ? 60000 : Infinity;
      if (kapanisGecti || simdi - this.sonIstek >= periyot) this.turVeBildir();
    }
    async turVeBildir() {
      const eskiN = this.M ? this.M.t.length : 0;
      const eskiIslem = this.R ? this.R.islemler.length : 0, eskiKapali = this.R ? this.R.kapaliIslem : 0;
      const ok = await this.canliTur();
      izleyiciDegisti(this, ok ? 'guncel' : 'hata', { eskiN, eskiIslem, eskiKapali });
      if (ok && this.kumbaraId) this.rapor();
    }
    async yukleVeBildir() {
      try {
        await this.yukle();
        if (this.kumbaraId) await capaBildir(this);
        if (this.kumbaraId) korumaBildir(this);
        izleyiciDegisti(this, 'yuklendi');
        if (this.kumbaraId) this.rapor();
      } catch (e) {
        this.durum = 'hata'; this.hata = hataYazisi(e);
        izleyiciDegisti(this, 'hata');
      }
    }
    // Son kapanan mumdaki karar programa bildirilir; program gerçek durumla eşleştirip gerekirse emir verir
    async rapor() {
      if (!this.kumbaraId || !this.R || !oturum || !pencereAktif) return;
      const R = this.R, M = this.M, i = R.son;
      if (i < 0) return;
      const lg = R.log[i];
      const d = lg && lg.sonra;
      const asama = d ? d.asama : 'bekle';
      const govde = { oturum, id: this.kumbaraId, mum: M.t[i], asama, stop: 0, hedef: 0, motorGirisMum: 0 };
      if (lg && lg.olay && (lg.olay.tip === 'al' || lg.olay.tip === 'sat')) govde.olay = { tip: lg.olay.tip, fiyat: lg.olay.fiyat, kural: lg.olay.kural || '' };
      if (asama === 'coinde') {
        govde.motorGirisMum = M.t[d.girisIdx];
        for (const s of Motor.seviyeler(R, i, d)) {
          if (!isFinite(s.fiyat)) continue;
          if (s.id === 'zararDurdur') govde.stop = s.fiyat;
          else if (s.id === 'hedefKar') govde.hedef = s.fiyat;
        }
      }
      const kb = kumbaraKaydi(this.kumbaraId);
      const sinir = kb ? kb.baslangic - 2 * this.ms : 0;
      govde.motorIslemleri = R.islemler.filter(t => M.t[t.girisIdx] >= sinir).slice(-40).map(t => ({
        g: M.t[t.girisIdx], gf: t.giris, c: t.cikisIdx != null ? M.t[t.cikisIdx] : null, cf: t.cikisIdx != null ? t.cikis : null, k: t.cikisKural || '',
      }));
      try {
        const r = await fetch('/api/kumbara/karar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(govde) });
        // 0.5: cevap her zaman okunur. Okunmayan cevap Chromium'da bir paylaşılan bellek tutamağını açık bırakıyordu;
        // dakikada ~15 bildirimle bir saatte 1024 sınırına varılıp pencere bozuluyordu (Pi'de saatte bir).
        const j = await r.json().catch(() => ({}));
        if (!r.ok) {
          this.raporHata = r.status === 404 ? null : j.hata || 'HTTP ' + r.status;
        } else {
          this.raporHata = null;
          this.sonRapor = { mum: govde.mum, zaman: Date.now(), asama };
        }
      } catch (e) {
        this.raporHata = hataYazisi(e);
      }
    }
  }

  // Kumbaranın sabit başlangıcı (ilk mum) programa bildirilir: program yeniden açılınca aynı yerden yüklenir
  async function capaBildir(iz) {
    if (!iz.kumbaraId || !iz.M || !oturum) return;
    try {
      const r = await fetch('/api/kumbara/capa', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, id: iz.kumbaraId, ilkMum: iz.M.t[0], mumSayisi: iz.yuklenen }) });
      const j = await r.json().catch(() => ({})); // cevap her zaman okunur (0.5)
      if (r.ok && j.ilkMum) iz.ilkMum = j.ilkMum;
    } catch (e) { /* sonra yeniden denenir */ }
  }

  // Bir izleyicide değişiklik oldu: grafiğe bağlıysa ekranı güncelle
  function izleyiciDegisti(iz, tur, once) {
    if (iz === GZ) {
      if (tur === 'yuklendi') gorunumeBagla(iz, true);
      else if (tur === 'guncel') gorunumuGuncelle(once);
      else if (tur === 'hata') {
        if (!M) { yukleniyor(false); hataGoster(iz.hata); canliDurum('hata', 'Veri alınamadı'); }
        else canliDurum('hata', 'Canlı veri alınamadı, tekrar denenecek');
      }
    }
    if (iz.kumbaraId && AKTIF && AKTIF.kumbaraId === iz.kumbaraId) kumbaraKartiCiz();
  }

  // Çalışan kumbaralarla izleyicileri eşle: yeni kumbaraya izleyici kur, kapanan kumbaranınkini bırak
  function izleyicileriEsitle() {
    const aktif = aktifKumbaralar();
    const idler = new Set(aktif.map(x => x.id));
    for (const [id, iz] of IZ) if (!idler.has(id)) { IZ.delete(id); iz.kumbaraId = null; }
    for (const kb of aktif) if (!IZ.has(kb.id) && kb.dosya !== baslatiliyor) izleyiciKur(kb);
  }
  async function izleyiciKur(kb) {
    const iz = new Izleyici({ kumbaraId: kb.id, dosyaAdi: kb.dosya, sembol: kb.sembol, aralik: kb.aralik, ilkMum: kb.ilkMum });
    iz.durum = 'yukleniyor';
    IZ.set(kb.id, iz);
    try {
      const r = await fetch('/api/kumbara/ayrinti?id=' + encodeURIComponent(kb.id));
      const j = await r.json();
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      const u = uyumKontrol(j.icerik);
      if (!u.ok) throw new Error(u.hata);
      iz.icerik = j.icerik;
      iz.strateji = calisanStrateji(j.icerik.strateji);
      iz.adet = j.mumSayisi > 0 ? j.mumSayisi : mumSayisiBul(j.icerik);
      if (AKTIF && AKTIF.kumbaraId === kb.id && AKTIF.iz === iz) { AKTIF.icerik = iz.icerik; AKTIF.strateji = iz.strateji; kurallariCiz(); }
    } catch (e) {
      iz.durum = 'hata'; iz.hata = hataYazisi(e); iz.sonDeneme = Date.now();
      izleyiciDegisti(iz, 'hata');
      return;
    }
    await iz.yukleVeBildir();
  }

  // ---------- grafiğin izleyiciye bağlanması ----------
  function zamanCevir(ms) { return utcZaman ? Math.floor(ms / 1000) : Math.floor(ms / 1000) - new Date(ms).getTimezoneOffset() * 60; }
  function zamanlariKur() {
    utcZaman = false;
    T = M.t.map(zamanCevir);
    for (let i = 1; i < T.length; i++) if (T[i] <= T[i - 1]) { utcZaman = true; T = M.t.map(zamanCevir); break; }
    zamanIdx = new Map(T.map((t, i) => [t, i]));
  }
  function fiyatBicimiKur() {
    const sirali = M.c.slice().sort((a, b) => a - b);
    const p = sirali[Math.floor(sirali.length / 2)] || 1;
    fiyatOnd = fiyatOndalik(p);
    const pf = fiyatFormati();
    mumSeri.applyOptions({ priceFormat: pf });
    for (const c of cizgiListesi) c.seri.applyOptions({ priceFormat: pf });
  }

  function gorunumeBagla(iz, ilk) {
    GZ = iz;
    if (!iz.M || !iz.R) return;
    M = iz.M; R = iz.R;
    zamanlariKur();
    incelemeKapat();
    fiyatBicimiKur();
    hesaptanSonra();
    isaretImza = ''; seviyeImza = ''; tabloImza = ''; kumbaraTabloImza = ''; olayImza = '';
    goster(M.c.length - 1);
    const n = M.c.length;
    if (n <= 700) anaGrafik.timeScale().fitContent();
    else anaGrafik.timeScale().setVisibleLogicalRange({ from: n - 450, to: n + 6 });
    const eksik = n < iz.adet && !iz.ilkMum ? ` (borsa ${nf(0).format(n)} mum verdi)` : '';
    $('#veriBilgi').textContent = `${iz.kaynak} · ${nf(0).format(n)} mum${eksik} · ${tarih(M.t[0])} – ${tarih(M.t[n - 1])}`;
    canliDurum('canli', `Canlı · son güncelleme ${saatSnYaz(Date.now())}`);
    yukleniyor(false); hataGoster(null); bosGoster(null);
    stratejiKartiCiz();
    kumbaraKartiCiz();
    aoZamanN = -1; aoSerileriDoldur(); aoYenile(true); // 0.9
    if (ilk) {
      if (R.isinma >= n) bildir(`Sadece ${nf(0).format(n)} mum geldi ama açık kurallar ilk ${nf(0).format(R.isinma)} mumu ısınmaya ayırıyor; bot hiç işlem yapamaz.`, 'dikkat');
      else if (n < iz.adet && !iz.ilkMum) bildir(`MEXC bu coin için ${nf(0).format(n)} mum verdi (${nf(0).format(iz.adet)} istendi). Coin yeni listelenmiş olabilir.`, 'dikkat');
    }
  }
  function gorunumuGuncelle(once) {
    if (!GZ || !GZ.M) return;
    if (GZ.M !== M || T.length > M.t.length) { gorunumeBagla(GZ); return; }
    for (let i = T.length; i < M.t.length; i++) { const z = zamanCevir(M.t[i]); T.push(z); zamanIdx.set(z, i); }
    R = GZ.R;
    const ts = anaGrafik.timeScale();
    const vr = ts.getVisibleLogicalRange();
    const sondaydi = !vr || vr.to >= k - 3;
    hesaptanSonra();
    goster(M.c.length - 1);
    if (sondaydi) sagaYasla();
    if (once && M.t.length > once.eskiN) aoYenile(true); // 0.9: mum kapandı: akış hemen yenilensin
    if (secili != null) incele(secili);
    // kumbarası olmayan stratejide simülatörün motorunun kararları bildirilir (kumbaralıda gerçek dolumlar bildirilir)
    if (once && !GZ.kumbaraId) {
      if (R.islemler.length > once.eskiIslem) {
        const t = R.islemler[R.islemler.length - 1];
        bildir(`Simülatörün motoru ALDI: ${S.sembol} · ${f(t.giris)}. Bu strateji için kumbara yok; emir verilmedi.`, 'al'); bip(880);
      } else if (R.kapaliIslem > once.eskiKapali) {
        const t = R.islemler.filter(x => x.cikisIdx != null).pop();
        bildir(`Simülatörün motoru SATTI: ${S.sembol} · ${f(t.cikis)} · ${Bicim.yuzde(t.yuzde)}. Bu strateji için kumbara yok; emir verilmedi.`, 'sat'); bip(440);
      }
    }
    canliDurum('canli', `Canlı · son güncelleme ${saatSnYaz(Date.now())}`);
  }

  // ---------- strateji seçme ----------
  function bosGoster(html) {
    const el = $('#bosKutu');
    if (html == null) { el.hidden = true; return; }
    el.innerHTML = html;
    el.hidden = false;
  }
  // Seçim dosya adıyla yapılır. O dosyanın çalışan kumbarası varsa grafik kumbaranın izleyicisini ve ayarını gösterir.
  function stratejiSec(ad, zorla) {
    const d = (LISTE && LISTE.dosyalar.find(x => x.ad === ad)) || null;
    const kb = dosyaKumbarasi(ad);
    if (!d && !kb) return;
    if (!zorla && AKTIF && AKTIF.ad === ad && (AKTIF.kumbaraId || null) === (kb ? kb.id : null) && !AKTIF.kayboldu) return;
    S.secili = ad;
    kaydet();
    yuklemeNo++; // önceki yükleme sonuçlanırsa yok sayılsın
    grafigiTemizle();
    hataGoster(null);
    yukleniyor(false);
    sekme = null;
    if (kb) {
      let iz = IZ.get(kb.id);
      if (!iz) { izleyiciKur(kb); iz = IZ.get(kb.id); }
      AKTIF = { ad, dosya: d, kumbaraId: kb.id, iz, icerik: iz.icerik, strateji: iz.strateji, kayboldu: false };
      S.sembol = kb.sembol; S.aralik = kb.aralik; S.borsa = 'mexc';
      document.title = `${kb.sembol} · ${aralikAd(kb.aralik)} — Kripto Al-Sat`;
      GZ = iz;
      bosGoster(null);
      listeCiz(true); stratejiKartiCiz(); kurallariCiz(); kumbaraKartiCiz(); tabloCiz(true);
      if (iz.durum === 'hazir') gorunumeBagla(iz, false);
      else if (iz.durum === 'hata') { hataGoster(iz.hata); canliDurum('hata', 'Veri alınamadı'); }
      else { yukleniyor(true, `MEXC'ten ${kb.sembol} için mumlar getiriliyor…`); canliDurum('durdu', 'Yükleniyor'); }
      return;
    }
    listeCiz(true);
    const hata = hataMetni(d);
    if (hata) {
      AKTIF = { ad, dosya: d, kumbaraId: null, iz: null, strateji: null };
      document.title = 'Kripto Al-Sat';
      bosGoster(`<b>Bu dosya çalıştırılamaz</b><p class="hata-yazi">${esc(hata)}</p>`);
      stratejiKartiCiz(); kurallariCiz(); kumbaraKartiCiz(); seritYaz(); tabloCiz(true);
      return;
    }
    const iz = new Izleyici({ dosyaAdi: ad, sembol: d.sembol, aralik: d.aralik, icerik: d.icerik, strateji: calisanStrateji(d.icerik.strateji), adet: mumSayisiBul(d.icerik) });
    AKTIF = { ad, dosya: d, kumbaraId: null, iz, icerik: d.icerik, strateji: iz.strateji, kayboldu: false };
    S.sembol = d.sembol; S.aralik = d.aralik; S.borsa = 'mexc';
    document.title = `${d.sembol} · ${aralikAd(d.aralik)} — Kripto Al-Sat`;
    GZ = iz;
    bosGoster(null);
    stratejiKartiCiz();
    kurallariCiz();
    kumbaraKartiCiz();
    tabloCiz(true);
    onizlemeYukle(iz);
  }
  async function onizlemeYukle(iz) {
    const no = yuklemeNo;
    yukleniyor(true, `MEXC'ten ${iz.sembol} için ${nf(0).format(iz.adet)} mum (${aralikAd(iz.aralik)}) getiriliyor…`);
    hataGoster(null);
    canliDurum('durdu', 'Yükleniyor');
    try {
      await iz.yukle();
      if (no !== yuklemeNo || !AKTIF || AKTIF.iz !== iz) return;
      gorunumeBagla(iz, true);
    } catch (e) {
      if (no !== yuklemeNo) return;
      iz.durum = 'hata'; iz.hata = hataYazisi(e); iz.sonDeneme = Date.now();
      canliDurum('hata', 'Veri alınamadı');
      hataGoster(iz.hata);
      kumbaraKartiCiz();
    } finally {
      if (no === yuklemeNo) yukleniyor(false);
    }
  }

  // Durum değişince (kumbara açıldı/kapandı, yeni ayara geçildi) seçimi düzelt
  function secimiDenetle() {
    if (!AKTIF) return;
    const kb = dosyaKumbarasi(AKTIF.ad);
    const simdiki = AKTIF.kumbaraId || null, olmali = kb ? kb.id : null;
    if (simdiki === olmali) return;
    if (olmali && AKTIF.iz && IZ.get(olmali) === AKTIF.iz) {
      // bu pencereden başlatıldı: aynı izleyici kumbaranın izleyicisi oldu
      AKTIF.kumbaraId = olmali; sekme = null;
      kumbaraArayuzu(true);
      return;
    }
    if (!olmali && AKTIF.iz && AKTIF.iz.M && AKTIF.dosya && AKTIF.iz.icerik && AKTIF.dosya.olusturuldu === (AKTIF.iz.icerik.olusturuldu ? Date.parse(AKTIF.iz.icerik.olusturuldu) : -1)) {
      // kumbara kapandı ve dosya aynı: grafik aynen önizleme olarak sürer
      AKTIF.kumbaraId = null;
      kumbaraArayuzu(true);
      return;
    }
    stratejiSec(AKTIF.ad, true);
  }

  // Seçili dosya klasörde değişti mi (yeniden dışa aktarıldı, silindi)?
  function seciliyiDenetle() {
    if (!AKTIF || !LISTE) return;
    const d = LISTE.dosyalar.find(x => x.ad === AKTIF.ad);
    if (!d) {
      if (AKTIF.kumbaraId) { if (AKTIF.dosya) { AKTIF.dosya = null; stratejiKartiCiz(); } return; }
      if (!AKTIF.kayboldu) { AKTIF.kayboldu = true; stratejiKartiCiz(); kumbaraKartiCiz(); bildir(`${AKTIF.ad} klasörden kaldırıldı.`, 'dikkat'); }
      return;
    }
    if (AKTIF.kayboldu) { AKTIF.kayboldu = false; }
    const eski = AKTIF.dosya;
    if (eski && d.degisti === eski.degisti && d.olusturuldu === eski.olusturuldu && d.durum === eski.durum) { AKTIF.dosya = d; return; }
    AKTIF.dosya = d;
    if (AKTIF.kumbaraId) { stratejiKartiCiz(); kumbaraKartiCiz(); return; } // kumbara kendi ayarıyla çalışır
    const hata = hataMetni(d);
    if (hata || !AKTIF.strateji || !eski) { AKTIF = null; stratejiSec(d.ad, true); return; }
    if (d.olusturuldu !== eski.olusturuldu || JSON.stringify(d.icerik.strateji) !== JSON.stringify(eski.icerik && eski.icerik.strateji)) {
      AKTIF.icerik = d.icerik;
      AKTIF.strateji = calisanStrateji(d.icerik.strateji);
      kurallariCiz();
      const iz = AKTIF.iz;
      if (iz) {
        iz.icerik = d.icerik; iz.strateji = AKTIF.strateji;
        if (iz.M) {
          iz.hesapla();
          const vr = anaGrafik.timeScale().getVisibleLogicalRange();
          const sondaydi = !vr || vr.to >= k - 3;
          R = iz.R; hesaptanSonra(); isaretImza = ''; seviyeImza = ''; tabloImza = '';
          goster(M.c.length - 1);
          if (sondaydi) sagaYasla();
        }
      }
      bildir(`${d.sembol} · ${aralikAd(d.aralik)} yenilendi (simülatörde yeniden dışa aktarılmış). Kararlar yeni ayarlarla yeniden hesaplandı.`, 'dikkat');
    }
    stratejiKartiCiz();
    kumbaraKartiCiz();
  }

  async function listeYenile() {
    try {
      const r = await fetch('/api/stratejiler');
      const j = await r.json();
      if (!r.ok) throw new Error(j.hata || r.status);
      LISTE = j;
      klasorYaz();
      listeCiz();
      motorUyarisi();
      seciliyiDenetle();
      if (!AKTIF) {
        const ogeler = listeOgeleri();
        let hedef = S.secili && ogeler.find(x => x.ad === S.secili);
        if (!hedef) hedef = ogeler.slice().sort((a, b) => siraNo(a) - siraNo(b) || (a.bitis || 0) - (b.bitis || 0))[0];
        if (hedef) stratejiSec(hedef.ad);
        else bosGoster(`<b>Soldan bir strateji seç</b><span>Grafik, simülatörün motorunun o strateji için verdiği kararlarla burada açılır.</span>`);
      }
    } catch (e) {
      $('#stratejiListe').innerHTML = `<div class="liste-bos"><b>Klasör okunamadı</b><span>${esc(hataYazisi(e))}</span></div>`;
      listeImza = '';
    }
  }

  // ---------- programın durumu: kumbaralar, cüzdanlar ----------
  async function durumYenile(zorla) {
    if (durumIstekte) { durumTekrar = true; return; }
    durumIstekte = true;
    try {
      const r = await fetch('/api/durum');
      const j = await r.json();
      if (!r.ok) throw new Error(j.hata || r.status);
      DURUM = j;
      dsSon = j.surum;
      if (typeof j.borsaFarki === 'number') borsaFarki = j.borsaFarki;
      yeniOlaylariBildir();
      uzakOlaylariBildir(); // 0.8
      durumHatasiGoster(j.durumHatasi); // 0.8
      izleyicileriEsitle();
      secimiDenetle();
      kumbaraArayuzu(zorla);
    } catch (e) {
      /* program kapanıyor olabilir; sonraki tikte yeniden denenir */
    } finally {
      durumIstekte = false;
      if (durumTekrar) { durumTekrar = false; setTimeout(() => durumYenile(), 50); }
    }
  }
  // Kumbaralarla ilgili her şeyi yeniden çiz (değişmeyenler imzayla atlanır)
  function kumbaraArayuzu(zorla) {
    ustCipler();
    listeCiz(zorla);
    kumbaraKartiCiz();
    if (M && R) {
      const kb = grafikKumbarasi();
      const sv = kb ? kumbaraSerileri(kb) : null;
      if ((kb ? kb.id + '|' + sv.imza : '') !== paraImza || zorla) paraSerileriniDoldur();
      else paraLejant();
      ozetCiz();
      seritYaz();
      lejantYaz(null);
      sonImza = '';
    }
    tabloCiz(zorla);
    if (modalAcik === 'mexc') mexcPenceresiYenile();
    if (modalAcik === 'sanal') sanalPenceresiYenile();
  }
  // Gerçekleşen alış/satışlar bildirilir (ses + bildirim)
  function yeniOlaylariBildir() {
    for (const kb of DURUM.kumbaralar) {
      const son = kb.olaylar.length ? kb.olaylar[kb.olaylar.length - 1].zaman : 0;
      const once = olaySon.get(kb.id);
      olaySon.set(kb.id, son);
      if (once === undefined) continue; // ilk görüş: eski olaylar bildirilmez
      for (const o of kb.olaylar) {
        if (o.zaman <= once) continue;
        const bas = `${kb.sembol} · ${aralikAd(kb.aralik)} (${kb.mod === 'gercek' ? 'GERÇEK' : 'sanal'}): `;
        if (o.tur === 'al') { bildir(bas + o.mesaj, 'al'); bip(880); }
        else if (o.tur === 'sat') { bildir(bas + o.mesaj, 'sat'); bip(440); }
        else if (o.tur === 'hata' && !o.tekrar) bildir(bas + o.mesaj, 'dikkat');
        else if (o.mesaj.startsWith('Koruma: ')) { bildir(bas + o.mesaj, 'dikkat'); bip(330); }
        else if (/^(Akış onay|Simülatör aldı ama akış)/.test(o.mesaj)) bildir(bas + o.mesaj, 'dikkat'); // 0.9
      }
    }
  }

  // ---------- üst şeritteki cüzdanlar ----------
  function ustCipler() {
    if (!DURUM) return;
    const s = DURUM.sanal, g = DURUM.gercek || {};
    $('#sanalCip').innerHTML = `<span class="cip-ad">Sanal cüzdan</span><b>${usdt(s.toplam)}</b>`;
    let mx;
    if (g.test) mx = `<span class="nokta gri"></span><span class="cip-ad">MEXC</span><b>test modu</b>`;
    else if (g.bagli) mx = `<span class="nokta yesil"></span><span class="cip-ad">MEXC</span><b>${g.usdt ? usdt(g.usdt.serbest) : 'bağlı'}</b>`;
    else if (g.kayitli && g.hata) mx = `<span class="nokta kirmizi"></span><span class="cip-ad">MEXC</span><b>bağlantı sorunu</b>`;
    else mx = `<span class="nokta gri"></span><span class="cip-ad">MEXC</span><b>bağlı değil</b>`;
    $('#mexcCip').innerHTML = mx;
    aoCip(); // 0.9
    const n = aktifKumbaralar().length;
    $('#calisiyor').hidden = n === 0;
    $('#calisiyorYazi').innerHTML = `${n} kumbara çalışıyor<span class="uzun"> · pencereyi kapatma, küçültebilirsin</span>`;
    $('#calisiyor').title = 'Kumbaralar program açıkken çalışır. Pencereyi küçültebilirsin; kapatırsan al-sat durur (açınca kaldığı yerden devam eder).';
  }

  // ---------- kumbara kartı (sağda) ----------
  const DURUM_AD = { calisiyor: 'Çalışıyor', sureDoldu: 'Ömür doldu · satış bekleniyor', durduruluyor: 'Durduruluyor · satınca kapanacak', bitti: 'Kapandı' };
  function baslatilamazNedeni() {
    const d = AKTIF && AKTIF.dosya;
    if (!d) return 'Dosya klasörde yok.';
    if (hataMetni(d)) return 'Bu dosya çalıştırılamaz.';
    if (d.durum === 'suresiDoldu' || d.bitis <= Date.now()) return 'Bu stratejinin ömrü dolmuş. Simülatörde yeniden dışa aktarınca yeniden başlatılabilir.';
    if (!pencereAktif) return 'Program başka bir pencerede açık.';
    if (!DURUM) return 'Programın durumu okunuyor…';
    return '';
  }
  function kumbaraKartiCiz() {
    aoKartCiz(); // 0.9: akış kartı da (değişmediyse yeniden çizilmez)
    const el = $('#kumbaraKart');
    if (!AKTIF) { el.hidden = true; return; }
    el.hidden = false;
    const kb = seciliKumbara();
    if (kb) { el.innerHTML = kumbaraKartHTML(kb); el.className = 'kart kumbara-kart ' + kb.mod; zamanlariGuncelle(); return; }
    el.className = 'kart kumbara-kart';
    const neden = baslatilamazNedeni();
    const biten = sonBiten(AKTIF.ad);
    const izHata = AKTIF.iz && AKTIF.iz.durum === 'hata' ? AKTIF.iz.hata : '';
    el.innerHTML = `
      <div class="kk-ust"><h2>Al-sat</h2><span class="kk-durum">kapalı</span></div>
      <p class="kk-aciklama">Başlatınca bu stratejinin kendi <b>kumbarası</b> açılır: ayırdığın parayla başlar; simülatörün motoru aldığında alır, sattığında satar. Kâr ederse kumbara büyür, zarar ederse küçülür; başka paraya dokunmaz.</p>
      ${neden ? `<p class="hata-not">${esc(neden)}</p>` : ''}
      ${izHata && !neden ? `<p class="hata-not">Grafik verisi alınamadı (${esc(izHata)}). Yine de başlatılabilir; kumbara veriyi kendisi tekrar dener.</p>` : ''}
      <button class="btn btn-ana kk-baslat" data-eylem="baslatAc" ${neden ? 'disabled' : ''}>Başlat…</button>
      ${biten ? bitenHTML(biten) : ''}`;
  }
  function bitenHTML(kb) {
    const iade = kb.iade || 0;
    const yz = (iade / kb.baslangicTutar - 1) * 100;
    const n = kb.islemler.filter(t => t.satis).length;
    return `<div class="kk-biten"><span class="mod-rozet ${kb.mod}">${kb.mod === 'gercek' ? 'GERÇEK' : 'SANAL'}</span>
      Son kumbara: ${usdt(kb.baslangicTutar)} → <b>${usdt(iade)}</b> <b class="${isaretSinif(yz)}">${Bicim.yuzde(yz)}</b> · ${n} işlem · kapandı (${esc(kb.bitisSebebi || '')}) ${gunSaat(kb.bitisZamani)}</div>`;
  }
  function kumbaraKartHTML(kb) {
    const t = acikIslemi(kb);
    const deger = kb.deger, bas = kb.baslangicTutar, yz = (deger / bas - 1) * 100;
    const varlik = varlikAdi(kb.sembol);
    const ms = ARALIK[kb.aralik] ? ARALIK[kb.aralik].ms : 60000;
    let durumAd = DURUM_AD[kb.durum] || kb.durum;
    if (kb.durum === 'durduruluyor' && kb.devam) durumAd = 'Yeni ayar bekliyor · satınca geçecek';
    if (kb.durum === 'durduruluyor' && !t && !kb.emirde && !kb.bekleyen) durumAd = 'Kapanıyor';
    if (kb.durum === 'calisiyor' && kb.koruma && kb.koruma.kenarda) durumAd = 'Kenarda · gölgede izleniyor';
    const satirlar = [];
    satirlar.push(`<span>Nakit</span><b>${usdt(kb.nakit)}</b>`);
    if (kb.adet > 0) satirlar.push(`<span>Coin</span><b>${miktarYaz(kb.adet)} ${esc(varlik)} ≈ ${usdt(kb.adet * kb.sonFiyat)}</b>`);
    satirlar.push(`<span>Ödenen komisyon</span><b>${usdt(kb.komisyon, kb.komisyon >= 1 ? 2 : 4)}</b>`);
    let poz = '';
    if (t && t.alis) {
      const a = t.alis;
      const anlik = kb.sonFiyat > 0 ? (kb.sonFiyat / a.ortalama - 1) * 100 : null;
      const fark = t.beklenenAlis > 0 ? (a.ortalama / t.beklenenAlis - 1) * 100 : null;
      poz = `<div class="kk-poz">
        <div class="kk-poz-ust"><b>Coinde</b><span>${gunSaat(a.bitis)}</span></div>
        <div class="bilgi-tablo">
          <span>Alış (gerçek)</span><b>${fg(a.ortalama)} <small>${YOL_AD[a.yol] || a.yol}</small></b>
          ${t.beklenenAlis > 0 ? `<span>Simülatörün fiyatı</span><b>${fg(t.beklenenAlis)} <small class="${fark <= 0 ? 'pos' : 'neg'}">${Bicim.yuzde(fark, 2)}</small></b>` : ''}
          <span>Şimdiki fiyat</span><b>${fg(kb.sonFiyat)} ${anlik != null ? `<small class="${isaretSinif(anlik)}">${Bicim.yuzde(anlik, 2)}</small>` : ''}</b>
          ${kb.stop > 0 ? `<span>Zarar durdur</span><b class="neg">${fg(kb.stop)} <small>anlık izleniyor, değince piyasa emri</small></b>` : ''}
          ${kb.hedef > 0 ? `<span>Hedef kâr</span><b class="pos">${fg(kb.hedef)} <small>anlık izleniyor</small></b>` : ''}
        </div></div>`;
    }
    let emir = '';
    if (kb.emirde) emir = `<div class="kk-emir"><span class="donen mini"></span>${esc(kb.emirAciklama || 'Emir veriliyor')}</div>`;
    else if (kb.bekleyen) emir = `<div class="kk-emir uyari">Yarım kalan emir MEXC'den soruluyor…</div>`;
    else if (kb.satisTekrar) emir = `<div class="kk-emir uyari">Satış tekrar denenecek: <b data-geri="${kb.satisTekrar}">…</b></div>`;
    const omur = kb.durum === 'calisiyor'
      ? `<div class="omur" data-bitis="${kb.bitis}" data-bas="${kb.olusturuldu}"><div class="omur-cubuk"><i></i></div><div class="omur-yazi"><span class="omur-kalan"></span><span>ömür biter ${gunSaat(kb.bitis)}</span></div></div>`
      : t ? `<div class="omur" data-bitis="${kb.ekBitis}" data-bas="${kb.bitis}" data-bitti-yazi="ek süre doldu"><div class="omur-cubuk"><i></i></div><div class="omur-yazi"><span class="omur-kalan"></span><span>en geç satış ${gunSaat(kb.ekBitis)}</span></div></div>` : '';
    const uyarilar = [];
    const g = (DURUM && DURUM.gercek) || {};
    if (kb.mod === 'gercek' && !g.bagli) uyarilar.push(`<b>MEXC bağlantısı yok.</b> Program emir veremez; bağlantı gelince kaldığı yerden devam eder.${g.hata ? ' (' + esc(g.hata) + ')' : ''}`);
    const iz = IZ.get(kb.id);
    if (iz && iz.durum === 'hata') uyarilar.push(`Grafik verisi alınamadı: ${esc(iz.hata)}. 30 saniyede bir tekrar deneniyor; bu sürede yeni karar verilemez (zarar durdur ve hedef yine izleniyor).`);
    else if (iz && iz.raporHata) uyarilar.push(`Karar programa iletilemedi: ${esc(iz.raporHata)}`);
    else if (kb.sonKararZamani && Date.now() - kb.sonKararZamani > 2 * ms + 90000) uyarilar.push(`Bir süredir yeni karar gelmedi (son: ${gunSaat(kb.sonKararZamani)}).`);
    const sonKarar = kb.sonKararMum
      ? `<div class="kk-karar">Son karar: <b>${saatSnYaz(kb.sonKararZamani)}</b> · mum ${saatYaz(kb.sonKararMum)}–${saatYaz(kb.sonKararMum + ms)} · motor <b>${ASAMA_AD[kb.motorAsama] || '–'}</b></div>`
      : `<div class="kk-karar">Motorun ilk kararı bekleniyor…</div>`;
    const satisTakildi = t && kb.satisTekrar && (kb.tekrarSayisi || 0) >= 2;
    const butonlar = kb.durum === 'calisiyor' || kb.durum === 'sureDoldu' || (kb.durum === 'durduruluyor' && !kb.hemenSat) || satisTakildi
      ? `<div class="kk-butonlar">
          ${kb.durum === 'calisiyor' ? `<button class="btn kucuk" data-eylem="yeniAlimYok" title="Yeni alım yapılmaz; elde coin varsa satış kuralı gelince satılır ve kumbara kapanır">Satınca durdur</button>` : ''}
          ${!kb.hemenSat ? `<button class="btn kucuk tehlike" data-eylem="simdiSat" title="${t ? 'Elde coin hemen piyasa emriyle satılır ve kumbara kapanır' : 'Kumbara hemen kapanır'}">${t ? 'Şimdi sat ve durdur' : 'Durdur'}</button>` : ''}
          ${satisTakildi ? `<button class="btn kucuk tehlike" data-eylem="satmadan" title="Satış defalarca başarısız oldu: kumbara satmadan kapanır, coin hesapta kalır">Satmadan kapat</button>` : ''}
        </div>` : '';
    return `
      <div class="kk-ust"><span class="mod-rozet ${kb.mod}">${modAdi(kb)}</span><h2>Kumbara</h2><span class="kk-durum ${kb.durum}${kb.koruma && kb.koruma.kenarda ? ' kenarda' : ''}">${durumAd}</span></div>
      <div class="kk-para">
        <div><span>Başlangıç</span><b>${usdt(bas)}</b></div>
        <div><span>Şimdi</span><b>${usdt(deger)} <small class="${isaretSinif(yz)}">${Bicim.yuzde(yz)}</small></b></div>
      </div>
      <div class="bilgi-tablo">${satirlar.join('')}</div>
      ${poz}${emir}${aoKumbaraSatiri(kb)}${omur}${sonKarar}
      ${korumaHTML(kb)}
      ${uyarilar.map(u => `<p class="hata-not">${u}</p>`).join('')}
      ${butonlar}`;
  }

  // Kumbara kartındaki koruma bölümü (0.7)
  function korumaHTML(kb) {
    const kr = kb.koruma;
    if (!kr) {
      return `<div class="kk-koruma"><div class="kkr-ust"><b>Koruma</b><span class="kkr-rozet">hazırlanıyor</span></div>
        <p class="kkr-not">Ayarın geçmişi grafikten okunuyor; grafik yüklenince kendiliğinden kurulur.</p></div>`;
    }
    const o = kr.olcum || {}, s = kr.sinir || {};
    const cubuk = (oran, uyari) => {
      const w = Math.max(0, Math.min(1, oran || 0));
      return `<div class="kkr-cubuk"><i class="${w >= 1 ? 'dolu' : uyari && w >= 0.6 ? 'yakin' : ''}" style="width:${(w * 100).toFixed(1)}%"></i></div>`;
    };
    const satirlar = [];
    if (kr.seriKapali) satirlar.push(`<div class="kkr-satir kapali"><div class="kkr-bas"><span>Art arda zarar</span><b>kapalı</b></div></div>`);
    else satirlar.push(`<div class="kkr-satir"><div class="kkr-bas"><span>Art arda zarar</span><b>${o.seri || 0} / ${s.seri}</b></div>${cubuk((o.seri || 0) / s.seri, true)}</div>`);
    // iki sınır: ayarın kendi sınırı (oyuna dönünce sıfırdan sayar) ve senin tavanın (toplam kayıp); sınırına yakın olan gösterilir
    const kayiplar = [];
    if (!kr.kayipKapali && s.kayip > 0) kayiplar.push({ d: o.dusus || 0, s: s.kayip, ad: 'ayarın kendi sınırı' });
    if (kr.tavan > 0) kayiplar.push({ d: o.tavanDusus || 0, s: kr.tavan, ad: 'senin tavanın (toplam kayıp)' });
    kayiplar.sort((a, b) => b.d / b.s - a.d / a.s || a.s - b.s);
    if (kayiplar.length) {
      const kk = kayiplar[0], diger = kayiplar[1];
      satirlar.push(`<div class="kkr-satir"><div class="kkr-bas"><span>Zirveden kayıp</span><b>${kisaYuzde(kk.d)} / ${kisaYuzde(kk.s)}</b></div>${cubuk(kk.d / kk.s, true)}
        <small>${kk.ad}${diger ? ` · ${diger.ad.replace(' (toplam kayıp)', '')}: ${kisaYuzde(diger.d)} / ${kisaYuzde(diger.s)}` : ''}</small></div>`);
    } else satirlar.push(`<div class="kkr-satir kapali"><div class="kkr-bas"><span>Zirveden kayıp</span><b>kapalı</b></div></div>`);
    if (kr.hizKapali || !(s.hizSayi > 0)) satirlar.push(`<div class="kkr-satir kapali"><div class="kkr-bas"><span>Aşırı hızlanma</span><b>${kr.hizKapali ? 'kapalı' : 'hesaplanamadı'}</b></div></div>`);
    else {
      const zararda = (o.hizNet || 0) < 0;
      satirlar.push(`<div class="kkr-satir"><div class="kkr-bas"><span>Aşırı hızlanma</span><b>${o.hizIslem || 0} / ${s.hizSayi} işlem</b></div>${cubuk((o.hizIslem || 0) / s.hizSayi, zararda)}
        <small>son ${sureUzun(s.hizPencere)} · toplam ${o.hizIslem ? `<span class="${isaretSinif(o.hizYuzde)}">${Bicim.yuzde(o.hizYuzde)}</span>` : '–'}${o.hizIslem && !zararda ? ' (kârdayken alarm çalmaz)' : ''}</small></div>`);
    }
    let kenar = '';
    if (kr.kenarda) {
      const golge = kr.golge || [];
      let art = 0;
      for (let i = golge.length - 1; i >= 0 && golge[i].y > 0; i--) art++;
      const tavanda = kr.tavan > 0 && (o.tavanDusus || 0) >= kr.tavan - 1e-9;
      kenar = `<div class="kkr-kenar">
        <p><b>Sebep:</b> ${esc(kr.kenarSebep || '')} <span class="kkr-zaman">${gunSaat(kr.kenarZamani)}</span></p>
        <p>Elde coin yok; ${usdt(kb.nakit)} kumbarada bekliyor. Motorun işlemleri gölgede (parasız) izleniyor.</p>
        ${tavanda ? `<p><b>Tavana ulaşıldı</b> (toplam kayıp ${kisaYuzde(o.tavanDusus)}): kumbara kendiliğinden dönmez. "Şimdi geri dön" dersen tavan bugünkü değerden yeniden sayılır.</p>` : ''}
        <div class="kkr-golge"><span>Gölge işlemler:</span>${golge.length ? golge.slice(-8).map(g => `<i class="${g.y > 0 ? 'pos' : 'neg'}">${g.y > 0 ? '✓' : '✕'} ${Bicim.yuzde(g.y)}</i>`).join('') : '<em>henüz yok</em>'}</div>
        ${tavanda ? '' : `<p class="kkr-ilerleme">Geri dönüş için art arda kârlı gölge işlem: <b>${art} / ${s.geriDonus}</b></p>`}
        <button class="btn kucuk" data-eylem="korumaDon">Şimdi geri dön</button>
      </div>`;
    }
    const kez = (kr.donemler || []).length;
    return `<div class="kk-koruma${kr.kenarda ? ' kenarda' : ''}">
      <div class="kkr-ust"><b>Koruma</b>${kr.kenarda ? '<span class="kkr-rozet kenarda">Kenarda · yeni alım yok</span>' : '<span class="kkr-rozet normal">Normal</span>'}</div>
      <p class="kkr-not">${karakterHTML(kr.karakter)}</p>
      ${kr.gecici ? '<p class="kkr-not">Geçici sınırlar: ayarın geçmişi grafik yüklenince okunacak ve sınırlar ona göre hesaplanacak.</p>' : ''}
      ${kenar}
      <div class="kkr-satirlar">${satirlar.join('')}</div>
      ${kr.saglamlik ? saglamlikHTML(kr.saglamlik) : ''}
      <div class="kkr-alt"><span>${HASSASIYET_AD[kr.hassasiyet] || kr.hassasiyet}${kr.elle ? ' · elle değiştirildi' : ''} · geri dönüş ${s.geriDonus} kârlı gölge${kez ? ` · ${kez} kez kenara alındı` : ''}</span><button class="btn kucuk" data-eylem="korumaAyar">Değiştir</button></div>
    </div>`;
  }

  // Koruma ayarları penceresi (kumbara kartındaki "Değiştir")
  let korumaPencere = null;
  function korumaAyarAc() {
    const kb = seciliKumbara();
    if (!kb || !kb.koruma) return;
    const kr = kb.koruma;
    korumaPencere = { id: kb.id, hassasiyet: kr.hassasiyet, oto: kr.oto, no: 0 };
    const s = kr.sinir;
    const hiz = s.hizPencere > 0;
    modalAc(`
      <div class="m-ust"><h3>${esc(kb.sembol)} · ${aralikAd(kb.aralik)} — koruma</h3><button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div>
      <div class="m-govde">
        <p>${karakterHTML(kr.karakter)}</p>
        <div class="koruma-hassasiyet"><span class="m-etiket">Hassasiyet</span>
          <div class="seg" id="kHassasiyet">${['sakin', 'dengeli', 'hassas'].map(h => `<button data-hassasiyet="${h}" aria-pressed="${h === kr.hassasiyet}">${HASSASIYET_AD[h]}</button>`).join('')}</div>
          <p class="m-not" id="kHassasiyetNot">${HASSASIYET_NOT[kr.hassasiyet]} Sınırlar ayarın kendi geçmişinden hesaplanır.</p>
        </div>
        <div class="koruma-alanlar">
          <label class="ka-satir"><input type="checkbox" id="kSeriAcik" ${kr.seriKapali ? '' : 'checked'}><span>Art arda zarar</span>
            <input class="m-girdi rakam" id="kSeri" type="number" min="1" max="50" step="1" value="${s.seri}"><em id="kSeriOto"></em></label>
          <label class="ka-satir"><input type="checkbox" id="kKayipAcik" ${kr.kayipKapali ? '' : 'checked'}><span>Zirveden kayıp (%)</span>
            <input class="m-girdi rakam" id="kKayip" type="number" min="0.5" max="90" step="0.1" value="${s.kayip}"><em id="kKayipOto"></em></label>
          <label class="ka-satir${hiz ? '' : ' kapali'}"><input type="checkbox" id="kHizAcik" ${kr.hizKapali || !hiz ? '' : 'checked'} ${hiz ? '' : 'disabled'}><span>Aşırı hızlanma${hiz ? ` (${sureUzun(s.hizPencere)} içinde, zarardayken)` : ' (hesaplanamadı)'}</span>
            <input class="m-girdi rakam" id="kHiz" type="number" min="2" max="500" step="1" value="${s.hizSayi || 0}" ${hiz ? '' : 'disabled'}><em id="kHizOto"></em></label>
          <label class="ka-satir"><input type="checkbox" checked disabled><span>Geri dönüş: art arda kârlı gölge işlem</span>
            <input class="m-girdi rakam" id="kGeri" type="number" min="1" max="10" step="1" value="${s.geriDonus}"><em id="kGeriOto"></em></label>
        </div>
        <div class="koruma-tavan">
          <label class="ka-satir"><span><b>Senin tavanın</b> (bütün kumbaralar, %)</span>
            <input class="m-girdi rakam" id="kTavan" type="number" min="0" max="90" step="0.5" value="${kr.tavan}"><em>0 = tavan yok</em></label>
          <p class="m-not">Ayarın kendi kayıp sınırı ne olursa olsun, kumbara en yüksek değerinden bu kadar düşünce kenara alınır. Bu toplam kayıptır: kumbara kenara alınıp oyuna dönse de en yüksek değeri unutulmaz.</p>
        </div>
        <p class="m-hata" id="mHata" hidden></p>
      </div>
      <div class="m-alt"><button class="btn" data-m="korumaOto">Otomatik değerlere dön</button><span class="bosluk"></span><button class="btn" data-m="kapat">Vazgeç</button><button class="btn btn-ana" data-m="korumaKaydet">Kaydet</button></div>`, 'koruma');
    korumaOtoYaz();
  }
  function korumaOtoYaz() {
    const o = korumaPencere && korumaPencere.oto;
    if (!o) return;
    $('#kSeriOto').textContent = `otomatik: ${o.seri}`;
    $('#kKayipOto').textContent = `otomatik: ${nfEn(1).format(o.kayip)}`;
    $('#kHizOto').textContent = o.hizSayi > 0 ? `otomatik: ${o.hizSayi}` : '';
    $('#kGeriOto').textContent = `otomatik: ${o.geriDonus}`;
  }
  async function korumaAyarHassasiyet(h) {
    const kp = korumaPencere;
    if (!kp) return;
    kp.hassasiyet = h;
    for (const b of document.querySelectorAll('#kHassasiyet button')) b.setAttribute('aria-pressed', b.dataset.hassasiyet === h);
    $('#kHassasiyetNot').textContent = HASSASIYET_NOT[h] + ' Sınırlar ayarın kendi geçmişinden hesaplanır.';
    const no = ++kp.no;
    try {
      const r = await fetch('/api/kumbara/koruma-ayar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, id: kp.id, onizle: true, hassasiyet: h }) });
      const j = await r.json().catch(() => ({}));
      if (no !== kp.no || modalAcik !== 'koruma' || korumaPencere !== kp) return;
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      kp.oto = j.oto;
      korumaOtoDoldur();
    } catch (e) { if (no === kp.no && modalAcik === 'koruma' && korumaPencere === kp) modalHata(hataYazisi(e)); }
  }
  function korumaOtoDoldur() {
    const o = korumaPencere && korumaPencere.oto;
    if (!o) return;
    $('#kSeri').value = o.seri;
    $('#kKayip').value = o.kayip;
    if (o.hizSayi > 0) $('#kHiz').value = o.hizSayi;
    $('#kGeri').value = o.geriDonus;
    korumaOtoYaz();
    modalHata('');
  }
  async function korumaKaydet() {
    const kp = korumaPencere;
    if (!kp || modalMesgul) return;
    const sayi = id => parseFloat(String($(id).value).replace(',', '.'));
    const o = kp.oto || {};
    const deger = (id, acik, oto) => { const x = sayi(id); return !acik && !isFinite(x) ? oto : x; }; // kapalı alarmın boş kutusu: otomatik değer
    const govde = {
      oturum, id: kp.id, hassasiyet: kp.hassasiyet,
      seri: Math.round(deger('#kSeri', $('#kSeriAcik').checked, o.seri)), kayip: deger('#kKayip', $('#kKayipAcik').checked, o.kayip),
      hizSayi: Math.round(deger('#kHiz', $('#kHizAcik').checked, o.hizSayi)) || 0, geriDonus: Math.round(sayi('#kGeri')),
      seriKapali: !$('#kSeriAcik').checked, kayipKapali: !$('#kKayipAcik').checked, hizKapali: !$('#kHizAcik').disabled && !$('#kHizAcik').checked,
      tavan: sayi('#kTavan'),
      gorulenOto: { seri: o.seri, kayip: o.kayip, hizSayi: o.hizSayi || 0, geriDonus: o.geriDonus }, // bu arada değiştiyse program kaydetmez
    };
    if (![govde.seri, govde.kayip, govde.geriDonus, govde.tavan].every(isFinite)) { modalHata('Bütün kutulara sayı yaz.'); return; }
    modalMesgul = true;
    try {
      const r = await fetch('/api/kumbara/koruma-ayar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(govde) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      modalMesgul = false;
      modalKapat();
      await durumYenile(true);
      bildir('Koruma ayarları kaydedildi.', 'al');
    } catch (e) { modalMesgul = false; modalHata(hataYazisi(e)); }
  }
  async function korumaDon() {
    const kb = seciliKumbara();
    if (!kb || !kb.koruma || !kb.koruma.kenarda) return;
    const kr = kb.koruma, o = kr.olcum || {};
    const tavanda = kr.tavan > 0 && (o.tavanDusus || 0) >= kr.tavan - 1e-9;
    const tamam = await onayIste('Kumbara oyuna dönsün mü?', 'Koruma kenara almayı bırakır ve alarmlar sıfırdan saymaya başlar. Motorun <b>sıradaki</b> alım kararında yeniden alım yapılır.' +
      (tavanda ? ` Kumbara tavana ulaşmıştı (toplam kayıp ${kisaYuzde(o.tavanDusus)}); dönerse tavan <b>bugünkü değerden yeniden</b> sayılır, yani kumbara bundan sonra en fazla bir tavan kadar daha kaybedebilir.` : ''), 'Oyuna döndür', false);
    modalKapat();
    if (!tamam) return;
    try {
      const r = await fetch('/api/kumbara/koruma-don', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, id: kb.id }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      await durumYenile(true);
    } catch (e) { bildir('Oyuna döndürülemedi: ' + hataYazisi(e), 'dikkat'); }
  }

  // ---------- pencereler (başlat, MEXC, sanal cüzdan, onay) ----------
  let modalAcik = null, modalMesgul = false, onayCozucu = null;
  function modalAc(html, tur) {
    $('#modalKutu').className = 'modal-kutu ' + tur;
    $('#modalKutu').innerHTML = html;
    $('#modal').hidden = false;
    modalAcik = tur; modalMesgul = false;
    const ilk = $('#modalKutu').querySelector('[autofocus]');
    if (ilk) setTimeout(() => ilk.focus(), 30);
  }
  function modalKapat() {
    if (modalMesgul) return;
    $('#modal').hidden = true;
    $('#modalKutu').innerHTML = '';
    modalAcik = null;
    if (onayCozucu) { const c = onayCozucu; onayCozucu = null; c(false); }
  }
  function modalHata(yazi) {
    const el = $('#mHata');
    if (!el) return;
    el.hidden = !yazi;
    el.textContent = yazi || '';
  }
  function onayIste(baslik, metinHTML, evet, tehlike) {
    return new Promise(coz => {
      modalAc(`<div class="m-ust"><h3>${esc(baslik)}</h3><button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div>
        <div class="m-govde"><p>${metinHTML}</p></div>
        <div class="m-alt"><button class="btn" data-m="kapat">Vazgeç</button><button class="btn ${tehlike ? 'tehlike-dolu' : 'btn-ana'}" data-m="evet" autofocus>${esc(evet)}</button></div>`, 'onay');
      onayCozucu = coz;
    });
  }

  // Başlat penceresi: sanal mı gerçek mi, ne kadar
  let baslatBilgi = null;
  async function baslatAc() {
    if (baslatilamazNedeni()) return;
    const d = AKTIF.dosya;
    let enAz = 5;
    try {
      const r = await fetch('/api/kural?sembol=' + encodeURIComponent(d.sembol));
      const j = await r.json();
      if (r.ok && j.enAz > 0) enAz = j.enAz;
    } catch (e) { /* varsayılan */ }
    let hassasiyet = 'dengeli';
    try {
      const r = await fetch('/api/ayarlar');
      const j = await r.json().catch(() => ({}));
      if (j.koruma && HASSASIYET_AD[j.koruma.hassasiyet]) hassasiyet = j.koruma.hassasiyet;
    } catch (e) { /* varsayılan */ }
    const s = DURUM.sanal, g = DURUM.gercek || {};
    const gercekSerbest = g.usdt ? Math.max(0, g.usdt.serbest - (g.ayrilan || 0)) : 0;
    const iz = AKTIF.iz;
    const lg = iz && iz.R && iz.R.son >= 0 ? iz.R.log[iz.R.son] : null;
    const coinde = !!(lg && lg.sonra && lg.sonra.asama === 'coinde');
    const ekSure = d.bitis + (d.bitis - d.olusturuldu) / 4;
    baslatBilgi = { enAz, sanalSerbest: s.serbest, gercekSerbest, gercekBagli: !!g.bagli, test: !!g.test, dosya: d.ad,
      koruma: { hassasiyet, gecmis: korumaGecmisi(iz, Date.now()), no: 0 }, saglamlik: null };
    const varsayilan = Math.min(100, Math.floor(s.serbest));
    modalAc(`
      <div class="m-ust"><h3>${esc(d.sembol)} · ${aralikAd(d.aralik)} — al-sat başlat</h3><button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div>
      <div class="m-govde">
        <div class="mod-secim">
          <label class="mod-kart"><input type="radio" name="mod" value="sanal" checked><span><b>Sanal para</b><small>Deneme. Programın içindeki sanal cüzdandan; fiyatlar, emir defteri ve komisyon gerçek MEXC'den.</small><em>Serbest: ${usdt(s.serbest)}</em></span></label>
          <label class="mod-kart gercek"><input type="radio" name="mod" value="gercek"><span><b>Gerçek para</b><small>MEXC hesabındaki USDT ile gerçek emirler verilir.</small><em>${g.test ? 'test modunda kapalı' : g.bagli ? 'Ayrılabilir: ' + usdt(gercekSerbest) : 'MEXC bağlı değil'}</em></span></label>
        </div>
        <label class="m-etiket" for="mTutar">Bu stratejiye ne kadar ayıralım? (USDT)</label>
        <div class="tutar-satir">
          <input id="mTutar" class="m-girdi" type="number" min="${enAz}" step="any" inputmode="decimal" value="${varsayilan >= enAz ? varsayilan : Math.ceil(enAz)}" autofocus>
          <div class="hizli">${[10, 25, 50, 100].map(x => `<button class="btn kucuk" data-tutar="${x}">${x}</button>`).join('')}<button class="btn kucuk" data-tutar="hepsi">Hepsi</button></div>
        </div>
        <p class="m-not" id="mTutarNot">En az ${usdt(enAz)}. Kumbara bu parayla başlar; her alışta kumbaradaki paranın tamamını kullanır, fazlasına dokunmaz.</p>
        <div class="koruma-kutu">
          <div class="kkr-ust"><b>Koruma</b><div class="seg" id="mHassasiyet">${['sakin', 'dengeli', 'hassas'].map(h => `<button data-hassasiyet="${h}" aria-pressed="${h === hassasiyet}">${HASSASIYET_AD[h]}</button>`).join('')}</div></div>
          <p class="m-not" id="mHassasiyetNot">${HASSASIYET_NOT[hassasiyet]}</p>
          <div id="mKorumaIcerik" class="koruma-icerik">Ayarın geçmişi hesaplanıyor…</div>
          <div id="mSaglamlik"></div>
        </div>
        ${aoBaslatHTML()}
        <ul class="m-bilgi">
          <li>Ömür: <b>${kalanYaz(d.bitis - Date.now())}</b> kaldı (biter ${gunSaat(d.bitis)}). Ömür dolunca yeni alım yapılmaz; elde coin varsa satış kuralı beklenir, en geç ${gunSaat(ekSure)}'de satılır.</li>
          <li>Emirler: önce fiyata en yakın limit emir; 15 saniyede dolmazsa piyasa emri. Zarar durdur seviyesine değince beklemeden piyasa emri.</li>
          ${coinde ? '<li class="dikkat">Simülatörün motoru şu an coinde. Bu işleme sonradan atlanmaz; sıradaki alım beklenir.</li>' : '<li>Kumbara, motorun açıldıktan sonraki ilk alım kararını bekler.</li>'}
          <li>Program açık kaldığı sürece çalışır. Pencereyi küçültebilirsin; kapatırsan al-sat durur (açınca kaldığı yerden devam eder).</li>
        </ul>
        <div class="gercek-onay" id="mGercekOnay" hidden>
          ${g.test ? '<p class="hata-not">Program test modunda; gerçek para kapalı.</p>'
            : !g.bagli ? '<p class="hata-not">Gerçek para için önce MEXC bağlantısını kur. <button class="btn kucuk" data-m="mexc">MEXC bağlantısı…</button></p>'
            : '<label class="onay-satir"><input type="checkbox" id="mOnay"> Gerçek parayla alıp satılacağını, kâr da zarar da olabileceğini anladım.</label><p class="m-not">Başlatmadan önce MEXC\'ye bir <b>deneme emri</b> gönderilir (gerçekleşmez): coin ve izinler böylece denetlenir.</p>'}
        </div>
        <p class="m-hata" id="mHata" hidden></p>
      </div>
      <div class="m-alt"><button class="btn" data-m="kapat">Vazgeç</button><button class="btn btn-ana" data-m="baslat" id="mBaslat">Başlat</button></div>`, 'baslat');
    baslatModuAyarla();
    korumaOnizle();
    saglamlikBaslat(d, iz);
  }
  // Başlat penceresinde: bu ayarın geçmişiyle koruma sınırları ne olur (program hesaplar, kaydetmez)
  async function korumaOnizle() {
    const b = baslatBilgi;
    if (!b || !b.koruma || modalAcik !== 'baslat') return;
    const kg = b.koruma;
    for (const btn of document.querySelectorAll('#mHassasiyet button')) btn.setAttribute('aria-pressed', btn.dataset.hassasiyet === kg.hassasiyet);
    $('#mHassasiyetNot').textContent = HASSASIYET_NOT[kg.hassasiyet];
    const el = $('#mKorumaIcerik');
    if (!kg.gecmis) { el.textContent = 'Grafik verisi olmadığı için koruma, kumbara başlayıp grafik yüklenince kurulacak.'; return; }
    const no = ++kg.no;
    try {
      const r = await fetch('/api/koruma/hesapla', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ dosya: b.dosya, islemler: kg.gecmis.islemler, sure: kg.gecmis.sure, hassasiyet: kg.hassasiyet }) });
      const j = await r.json().catch(() => ({}));
      if (no !== kg.no || baslatBilgi !== b || modalAcik !== 'baslat') return;
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      el.innerHTML = `<ul class="m-bilgi">
        <li>${karakterHTML(j.karakter)}</li>
        <li>Kenara alınır: ${korumaKurallarHTML(j.sinir, j.kayipEtkin, j.kayipKaynak, {})}.</li>
        <li>Kenardayken yeni alım yapılmaz, para kumbarada bekler; motor gölgede (parasız) izlenir. Gölgede art arda <b>${j.sinir.geriDonus} kârlı işlem</b> olunca kendiliğinden devam eder. Sınırlar sonradan kumbara kartından değiştirilebilir.</li></ul>`;
    } catch (e) {
      if (no === kg.no && baslatBilgi === b) el.textContent = 'Koruma şimdi hesaplanamadı (' + hataYazisi(e) + '); kumbara başlayınca kurulacak.';
    }
  }
  const saglamlikOnbellek = new Map();
  function saglamlikBaslat(d, iz) {
    const el = $('#mSaglamlik');
    if (!el) return;
    const anahtar = d.ad + '|' + d.olusturuldu;
    const b = baslatBilgi;
    if (saglamlikOnbellek.has(anahtar)) { b.saglamlik = saglamlikOnbellek.get(anahtar); el.innerHTML = saglamlikHTML(b.saglamlik); return; }
    const gs = d.icerik && d.icerik.gecmisSonuc;
    let bas = gs && gs.ilkMum ? Date.parse(gs.ilkMum) : NaN;
    if (!isFinite(bas) && iz && iz.M) bas = iz.M.t[0];
    const strateji = AKTIF && AKTIF.strateji;
    if (!isFinite(bas) || !strateji) { el.innerHTML = saglamlikHTML({ yok: 'hesaplanamadı (grafik verisi yok).' }); return; }
    el.innerHTML = '<div class="saglamlik"><span class="donen mini"></span> <b>Sağlamlık testi:</b> ayarın görmediği önceki günler indiriliyor ve hesaplanıyor…</div>';
    setTimeout(async () => {
      let sonuc;
      try { sonuc = await saglamlikTesti(d.sembol, d.aralik, strateji, bas); }
      catch (e) { sonuc = { yok: 'hesaplanamadı (' + hataYazisi(e) + ').', hata: true }; }
      if (!sonuc.hata) saglamlikOnbellek.set(anahtar, sonuc);
      if (baslatBilgi !== b || modalAcik !== 'baslat') return;
      b.saglamlik = sonuc.hata ? null : sonuc;
      const e2 = $('#mSaglamlik');
      if (e2) e2.innerHTML = saglamlikHTML(sonuc);
    }, 80);
  }
  function baslatModuAyarla() {
    const secim = $('#modalKutu input[name=mod]:checked');
    if (!secim || !baslatBilgi) return;
    const gercek = secim.value === 'gercek';
    $('#mGercekOnay').hidden = !gercek;
    $('#mBaslat').textContent = gercek ? 'Gerçek parayla başlat' : 'Sanal parayla başlat';
    $('#mBaslat').classList.toggle('tehlike-dolu', gercek);
    $('#mBaslat').classList.toggle('btn-ana', !gercek);
    for (const kart of document.querySelectorAll('.mod-kart')) kart.classList.toggle('secili', kart.querySelector('input').checked);
    modalHata('');
  }
  async function kumbaraBaslat() {
    if (modalMesgul || !AKTIF || !AKTIF.dosya) return;
    const mod = $('#modalKutu input[name=mod]:checked').value;
    const tutar = parseFloat(String($('#mTutar').value).replace(',', '.'));
    const b = baslatBilgi;
    if (!isFinite(tutar) || tutar <= 0) { modalHata('Tutarı yaz.'); return; }
    if (tutar < b.enAz) { modalHata(`En az ${usdt(b.enAz)} ayırmalısın.`); return; }
    if (mod === 'sanal' && tutar > b.sanalSerbest + 1e-9) { modalHata(`Sanal cüzdanda serbest ${usdt(b.sanalSerbest)} var.`); return; }
    if (mod === 'gercek') {
      if (b.test || !b.gercekBagli) { modalHata('Önce MEXC bağlantısını kur.'); return; }
      if (tutar > b.gercekSerbest + 1e-9) { modalHata(`MEXC hesabında başka kumbaralara ayrılmamış ${usdt(b.gercekSerbest)} var.`); return; }
      if (!$('#mOnay').checked) { modalHata('Gerçek parayla başlatmak için onay kutusunu işaretle.'); return; }
    }
    const iz = AKTIF.iz, d = AKTIF.dosya;
    const govde = { oturum, dosya: d.ad, olusturuldu: d.olusturuldu, mod, tutar, onay: mod === 'gercek',
      ilkMum: iz && iz.M ? iz.M.t[0] : 0, mumSayisi: iz && iz.M ? iz.yuklenen : 0,
      akisKapali: !!($('#mAkis') && !$('#mAkis').disabled && !$('#mAkis').checked) }; // 0.9
    const kg = b.koruma;
    if (kg && kg.gecmis) govde.koruma = { islemler: kg.gecmis.islemler, sure: kg.gecmis.sure, hassasiyet: kg.hassasiyet };
    if (b.saglamlik) govde.saglamlik = b.saglamlik;
    modalMesgul = true; baslatiliyor = d.ad;
    const btn = $('#mBaslat');
    btn.disabled = true; btn.textContent = mod === 'gercek' ? 'MEXC denetleniyor…' : 'Başlatılıyor…';
    try {
      const r = await fetch('/api/kumbara/baslat', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(govde) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      if (iz && AKTIF && AKTIF.iz === iz && iz.M) {
        iz.kumbaraId = j.id; iz.ilkMum = iz.M.t[0];
        IZ.set(j.id, iz);
        AKTIF.kumbaraId = j.id;
      }
      modalMesgul = false; baslatiliyor = null;
      modalKapat();
      sekme = null;
      await durumYenile(true);
      if (iz && iz.kumbaraId === j.id) { iz.rapor(); korumaBildir(iz); }
      bildir(`${d.sembol} · ${aralikAd(d.aralik)}: ${mod === 'gercek' ? 'GERÇEK PARA' : 'sanal'} kumbara açıldı (${usdt(tutar)}). Simülatörün motorunun sıradaki alım kararı bekleniyor.`, 'al');
    } catch (e) {
      modalMesgul = false; baslatiliyor = null;
      btn.disabled = false;
      baslatModuAyarla();
      modalHata(hataYazisi(e));
    }
  }

  // MEXC bağlantısı penceresi
  const IZIN_AD = {
    SPOT_ACCOUNT_READ: 'Hesap bilgisini görme', SPOT_DEAL_READ: 'Emirleri görme', SPOT_DEAL_WRITE: 'Alım-satım',
    SPOT_ACCOUNT_WRITE: 'Hesap ayarlarını değiştirme', CONTRACT_ACCOUNT_READ: 'Vadeli hesap görme', CONTRACT_ACCOUNT_WRITE: 'Vadeli hesap değiştirme',
    CONTRACT_DEAL_READ: 'Vadeli emir görme', CONTRACT_DEAL_WRITE: 'Vadeli alım-satım',
  };
  function mexcIcerik() {
    const g = (DURUM && DURUM.gercek) || {};
    const gercekKumbara = aktifKumbaralar().some(x => x.mod === 'gercek');
    if (g.test) return `<div class="m-govde"><p>Program test modunda çalışıyor; gerçek para kapalı.</p></div><div class="m-alt"><button class="btn" data-m="kapat">Kapat</button></div>`;
    if (g.bagli) {
      const izinler = (g.izinler || []).filter(x => IZIN_AD[x] && !(g.fazlaIzin || []).includes(x)).map(x => '✓ ' + IZIN_AD[x]).join(' · ');
      const coinler = (g.coinler || []).slice(0, 12).map(c => `${esc(c.ad)} ${miktarYaz(c.serbest + c.kilitli)}`).join(' · ');
      return `<div class="m-govde">
        <div class="mx-durum"><span class="nokta yesil"></span>Bağlı · anahtar <b class="rakam">${esc(g.maske)}</b></div>
        <div class="bilgi-tablo">
          <span>İzinler</span><b>${izinler || '–'}</b>
          <span>Para çekme / transfer</span><b class="pos">izin yok (güvenli)</b>
          <span>Anahtarın süresi</span><b>${g.kalanGun === -999 ? 'süresiz (IP adresine bağlı)' : g.kalanGun > 0 ? g.kalanGun + ' gün kaldı' : '–'}</b>
          <span>USDT</span><b>${g.usdt ? `serbest ${usdt(g.usdt.serbest)} · emirde ${usdt(g.usdt.kilitli)}` : '–'}</b>
          <span>Kumbaralara ayrılan</span><b>${usdt(g.ayrilan || 0)}</b>
          ${coinler ? `<span>Diğer coinler</span><b>${coinler}</b>` : ''}
          <span>Bakiye okundu</span><b>${g.bakiyeZamani ? saatSnYaz(g.bakiyeZamani) : '–'}</b>
        </div>
        ${(g.fazlaIzin || []).length ? `<p class="hata-not">Anahtarda bu programın kullanmadığı izinler de var: ${(g.fazlaIzin || []).map(x => esc(IZIN_AD[x] || x)).join(', ')}. İstersen MEXC'de kaldırabilirsin.</p>` : ''}
        <p class="m-not">Program kendi aldığı coinleri satar; senin elle aldığın coinlere ve kumbaralara ayrılmamış paraya dokunmaz.</p>
        <p class="m-hata" id="mHata" hidden></p>
      </div>
      <div class="m-alt">
        <button class="btn tehlike" data-m="anahtarSil" ${gercekKumbara ? 'disabled title="Önce gerçek parayla çalışan kumbaraları durdur"' : ''}>Anahtarı bu bilgisayardan sil</button>
        <span class="bosluk"></span>
        <button class="btn" data-m="yenile">Bakiyeyi yenile</button><button class="btn btn-ana" data-m="kapat">Tamam</button>
      </div>`;
    }
    return `<div class="m-govde">
      ${g.kayitli && g.hata ? `<p class="hata-not"><b>Kayıtlı anahtarla bağlanılamadı.</b> ${esc(g.hata)}</p>` : ''}
      <ol class="adimlar">
        <li>MEXC'de <b>API Yönetimi</b> (API Management) sayfasında yeni API anahtarı oluştur.</li>
        <li>İzinlerden <b>sadece</b> şu üçünü işaretle: <b>Hesap bilgilerini görüntüle</b> (View Account Details), <b>Emir bilgilerini görüntüle</b> (View Order Details), <b>İşlem</b> (Trade). <b>Para çekme, transfer ve vadeli işlem işaretsiz kalsın.</b></li>
        <li>MEXC'nin verdiği <b>Access Key</b> ve <b>Secret Key</b>'i aşağıya yapıştır. Bunları başka hiçbir yere (sohbete, mesaja) yazma.</li>
      </ol>
      <label class="m-etiket" for="mxErisim">Access Key</label>
      <input id="mxErisim" class="m-girdi rakam" autocomplete="off" spellcheck="false" autofocus>
      <label class="m-etiket" for="mxGizli">Secret Key</label>
      <input id="mxGizli" class="m-girdi rakam" type="password" autocomplete="new-password" spellcheck="false">
      <p class="m-not">Program anahtarı önce MEXC'ye sorar: <b>para çekme ya da transfer izni varsa kullanmaz ve kaydetmez.</b> Uygunsa sadece bu bilgisayarda, Windows'un kendi şifrelemesiyle saklar (masaüstüne, OneDrive'a değil). Gizli anahtar ekrana geri gelmez.</p>
      <p class="m-hata" id="mHata" hidden></p>
    </div>
    <div class="m-alt">${g.kayitli ? `<button class="btn tehlike" data-m="anahtarSil" ${gercekKumbara ? 'disabled' : ''}>Kayıtlı anahtarı sil</button>` : ''}<span class="bosluk"></span><button class="btn" data-m="kapat">Vazgeç</button><button class="btn btn-ana" data-m="baglan" id="mxBaglan">Bağlan</button></div>`;
  }
  function mexcAc() {
    modalAc(`<div class="m-ust"><h3>MEXC bağlantısı</h3><button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div><div id="mxIcerik">${mexcIcerik()}</div>`, 'mexc');
  }
  function mexcPenceresiYenile() {
    if (modalMesgul) return;
    const el = $('#mxIcerik');
    if (!el) return;
    // formda yazı varsa silme
    const erisim = $('#mxErisim'), gizli = $('#mxGizli');
    if ((erisim && erisim.value) || (gizli && gizli.value)) return;
    const hata = $('#mHata') && !$('#mHata').hidden ? $('#mHata').textContent : '';
    el.innerHTML = mexcIcerik();
    if (hata) modalHata(hata);
  }
  async function mexcBaglan() {
    const erisim = $('#mxErisim').value.trim(), gizli = $('#mxGizli').value.trim();
    if (!erisim || !gizli) { modalHata('Access Key ve Secret Key ikisi de gerekli.'); return; }
    modalMesgul = true;
    const btn = $('#mxBaglan');
    btn.disabled = true; btn.textContent = 'MEXC denetleniyor…';
    modalHata('');
    try {
      const r = await fetch('/api/anahtar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, erisim, gizli }) });
      const j = await r.json().catch(() => ({}));
      $('#mxGizli').value = ''; // gizli anahtar sayfada kalmasın
      if (!r.ok || !j.tamam) throw new Error(j.hata || 'HTTP ' + r.status);
      $('#mxErisim').value = '';
      modalMesgul = false;
      await durumYenile(true);
      $('#mxIcerik').innerHTML = mexcIcerik();
      if (j.uyarilar && j.uyarilar.length) modalHata(j.uyarilar.join(' '));
      bildir('MEXC bağlantısı kuruldu. Anahtar bu bilgisayarda şifreli saklandı.', 'al');
    } catch (e) {
      modalMesgul = false;
      btn.disabled = false; btn.textContent = 'Bağlan';
      modalHata(hataYazisi(e));
    }
  }
  async function anahtarSil() {
    modalMesgul = false;
    const tamam = await onayIste('MEXC anahtarını sil', 'Anahtar bu bilgisayardan silinir ve bağlantı kesilir. MEXC\'deki anahtar yerinde kalır (istersen MEXC\'den de silebilirsin). Gerçek parayla al-sat için anahtarı yeniden girmen gerekir.', 'Sil', true);
    if (!tamam) { mexcAc(); return; }
    try {
      const r = await fetch('/api/anahtar/sil', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      modalKapat();
      await durumYenile(true);
      bildir('MEXC anahtarı bu bilgisayardan silindi.');
    } catch (e) {
      modalKapat();
      bildir('Anahtar silinemedi: ' + hataYazisi(e), 'dikkat');
    }
  }

  // Sanal cüzdan penceresi
  function sanalIcerik() {
    const s = DURUM.sanal;
    const sanalVar = aktifKumbaralar().some(x => x.mod === 'sanal');
    const fark = s.toplam - s.baslangic;
    return `<div class="m-govde">
      <p>Sanal kumbaralar parayı buradan alır ve kapanınca buraya geri koyar. Gerçek parayla ilgisi yoktur.</p>
      <div class="bilgi-tablo">
        <span>Toplam (şimdiki değer)</span><b>${usdt(s.toplam)} <small class="${isaretSinif(fark)}">${usdtIsaret(fark)}</small></b>
        <span>Serbest</span><b>${usdt(s.serbest)}</b>
        <span>Kumbaralarda</span><b>${usdt(s.kumbaralarda)}</b>
        <span>Başlangıç</span><b>${usdt(s.baslangic)}</b>
      </div>
      <label class="m-etiket" for="svBakiye">Sanal cüzdanı sıfırla (yeni başlangıç parası)</label>
      <div class="tutar-satir"><input id="svBakiye" class="m-girdi" type="number" min="10" step="any" value="${Math.round(s.baslangic)}" ${sanalVar ? 'disabled' : ''}><button class="btn" data-m="sanalSifirla" ${sanalVar ? 'disabled' : ''}>Sıfırla</button></div>
      ${sanalVar ? '<p class="m-not">Sıfırlamak için önce çalışan sanal kumbaraları durdur.</p>' : ''}
      <p class="m-hata" id="mHata" hidden></p>
    </div>
    <div class="m-alt"><button class="btn btn-ana" data-m="kapat">Tamam</button></div>`;
  }
  function sanalAc() {
    if (!DURUM) return;
    modalAc(`<div class="m-ust"><h3>Sanal cüzdan</h3><button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div><div id="svIcerik">${sanalIcerik()}</div>`, 'sanal');
  }
  function sanalPenceresiYenile() {
    const el = $('#svIcerik');
    if (!el || document.activeElement === $('#svBakiye')) return;
    el.innerHTML = sanalIcerik();
  }
  async function sanalSifirla() {
    const bakiye = parseFloat(String($('#svBakiye').value).replace(',', '.'));
    if (!isFinite(bakiye) || bakiye < 10) { modalHata('En az 10 USDT yaz.'); return; }
    try {
      const r = await fetch('/api/cuzdan/sifirla', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, bakiye }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      await durumYenile(true);
      $('#svIcerik').innerHTML = sanalIcerik();
      bildir(`Sanal cüzdan sıfırlandı: ${usdt(bakiye)}`);
    } catch (e) { modalHata(hataYazisi(e)); }
  }

  // Kumbarayı durdurma
  async function kumbaraDurdur(nasil) {
    const kb = seciliKumbara();
    if (!kb) return;
    const t = acikIslemi(kb);
    let tamam;
    if (nasil === 'satmadan') {
      tamam = await onayIste('Satmadan kapat', `Satış birkaç kez başarısız oldu. Kumbara <b>satmadan</b> kapanır.${kb.mod === 'gercek' ? ` Eldeki ${esc(varlikAdi(kb.sembol))} MEXC hesabında kalır; program ona artık dokunmaz, istersen MEXC'de kendin satarsın.` : ' Sanal coin bugünkü fiyatıyla cüzdana döner.'}`, 'Satmadan kapat', true);
    } else if (nasil === 'yeniAlimYok') {
      tamam = await onayIste('Satınca durdur', `Kumbara yeni alım yapmayacak. ${t ? `Eldeki ${esc(varlikAdi(kb.sembol))}, simülatörün satış kuralı gelince satılır ve kumbara kapanır (ömür dolarsa en geç ${gunSaat(kb.ekBitis)}'de).` : 'Elde coin yok; kumbara hemen kapanır.'}`, 'Durdur', false);
    } else {
      tamam = await onayIste(t ? 'Şimdi sat ve durdur' : 'Durdur', t ? `Eldeki ${esc(varlikAdi(kb.sembol))} <b>hemen piyasa emriyle</b> satılır ve kumbara kapanır.${kb.mod === 'gercek' ? ' Bu gerçek bir satıştır.' : ''}` : 'Kumbara hemen kapanır; para cüzdana döner.', t ? 'Sat ve durdur' : 'Durdur', true);
    }
    if (!tamam) { modalKapat(); return; }
    modalKapat();
    try {
      const r = await fetch('/api/kumbara/durdur', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, id: kb.id, nasil }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      await durumYenile(true);
    } catch (e) { bildir('Durdurulamadı: ' + hataYazisi(e), 'dikkat'); }
  }
  async function hepsiniDurdur() {
    const n = aktifKumbaralar().length;
    if (!n) return;
    const gercek = aktifKumbaralar().filter(x => x.mod === 'gercek').length;
    const tamam = await onayIste('Hepsini sat ve durdur', `${n} kumbaranın hepsi durdurulur; elde coin olanlar <b>hemen piyasa emriyle</b> satılır.${gercek ? ` ${gercek} tanesi gerçek parayla çalışıyor; onlarınki gerçek satıştır.` : ''}`, 'Hepsini sat ve durdur', true);
    modalKapat();
    if (!tamam) return;
    try {
      const r = await fetch('/api/hepsini-durdur', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      await durumYenile(true);
      bildir(`Hepsi durduruluyor${j.satilan ? ` (${j.satilan} kumbarada satış yapılıyor)` : ''}.`, 'dikkat');
    } catch (e) { bildir('Durdurulamadı: ' + hataYazisi(e), 'dikkat'); }
  }

  // ---------- canlı izleme ----------
  function bip(frekans) {
    try {
      if (!ses) ses = new (window.AudioContext || window.webkitAudioContext)();
      const o = ses.createOscillator(), g = ses.createGain();
      o.frequency.value = frekans; o.type = 'sine';
      g.gain.setValueAtTime(0.0001, ses.currentTime);
      g.gain.exponentialRampToValueAtTime(0.25, ses.currentTime + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, ses.currentTime + 0.35);
      o.connect(g).connect(ses.destination);
      o.start(); o.stop(ses.currentTime + 0.4);
    } catch (e) { /* ses yoksa sorun değil */ }
  }

  // Her saniye ve programdan gelen her "tik"te: bütün izleyiciler mum kapanışını kontrol eder
  function canliKontrol() {
    if (!pencereAktif) return;
    for (const iz of IZ.values()) iz.kontrol(iz === GZ);
    if (GZ && !GZ.kumbaraId) GZ.kontrol(true);
  }

  let bildirimZaman = null;
  function bildir(yazi, tur) {
    const el = $('#bildirim');
    el.textContent = yazi;
    el.className = 'bildirim ' + (tur || '');
    el.hidden = false;
    clearTimeout(bildirimZaman);
    bildirimZaman = setTimeout(() => { el.hidden = true; }, tur ? 10000 : 6000);
  }

  let yuklemeNo = 0;
  function yukleniyor(acik, yazi) {
    $('#yukleniyor').hidden = !acik;
    if (yazi) $('#yukleniyorYazi').textContent = yazi;
  }
  function hataGoster(yazi) {
    $('#hataKutu').hidden = !yazi;
    if (yazi) $('#hataYazi').textContent = yazi;
  }

  // ---------- pencere oturumu ve programla canlı hat ----------
  async function oturumAc() {
    try {
      const r = await fetch('/api/oturum', { method: 'POST' });
      const j = await r.json();
      oturum = j.oturum;
    } catch (e) { oturum = null; }
    try {
      if (olayKaynagi) olayKaynagi.close();
      olayKaynagi = new EventSource('/api/baglanti?oturum=' + encodeURIComponent(oturum || ''));
      olayKaynagi.onmessage = e => {
        let d;
        try { d = JSON.parse(e.data); } catch (x) { return; }
        if (d.tip === 'tik') {
          if (typeof d.bf === 'number') borsaFarki = d.bf;
          pencereAktifAyarla(!!d.aktif);
          if (d.ds !== undefined && d.ds !== dsSon) durumYenile();
          canliKontrol();
        }
      };
    } catch (e) { /* tarayıcıda açıldıysa sorun değil */ }
    pencereAktifAyarla(true);
  }
  function pencereAktifAyarla(aktif) {
    if (aktif === pencereAktif) return;
    pencereAktif = aktif;
    $('#pencereUyari').hidden = aktif;
    if (!aktif) canliDurum('durdu', 'Başka pencerede açık');
    kumbaraKartiCiz();
  }

  // 0.8: kayıt dosyası (durum.json) okunamadıysa ekranın üstünde kalıcı uyarı
  function durumHatasiGoster(h) {
    let el = $('#durumHatasi');
    if (!h) { if (el) el.remove(); return; }
    if (!el) {
      el = document.createElement('div');
      el.id = 'durumHatasi'; el.className = 'durum-hatasi';
      document.body.prepend(el);
    }
    el.textContent = h;
  }

  // ---------- 0.8: eski sürümün kayıtlarını taşıma ----------
  // Bu sürümde hiç kumbara yoksa ve yakındaki bir klasörde eski bir Al-Sat'ın kaydı varsa bir kez sorulur (tasima.go).
  let TASIMA = null;
  async function tasimaSor() {
    try {
      const r = await fetch('/api/tasima');
      const j = await r.json().catch(() => ({}));
      TASIMA = j.teklif || null;
    } catch (e) { TASIMA = null; }
    if (!TASIMA || modalAcik) return;
    const t = TASIMA;
    const ne = [];
    if (t.calisan) ne.push(`<b>${t.calisan}</b> çalışan kumbara${t.gercek ? ` (${t.gercek}'${t.gercek === 1 ? 'i' : 'si'} gerçek parayla)` : ''}`);
    if (t.coinde) ne.push(`${t.coinde} tanesinin elinde şu an coin var`);
    const bitmis = t.toplam - t.calisan;
    if (bitmis > 0) ne.push(`${bitmis} bitmiş kumbaranın geçmişi`);
    modalAc(`<div class="m-ust"><h3>Eski sürümün kayıtları bulundu</h3><button class="kapat" data-m="tasiVazgec" aria-label="Kapat">×</button></div>
      <div class="m-govde">
        <p><b>${esc(t.ad)}</b> klasöründe eski Al-Sat'ın kaydı var: ${ne.join(', ')}. Son yazıldığı an: ${gunSaat(t.degisti)}.</p>
        <p>Bu sürüme taşıyayım mı? Taşınınca kumbaralar burada <b>kaldıkları yerden devam eder</b> (programı kapatıp açmışsın gibi): kapalıyken kaçan satış hemen yapılır, kaçan alımın peşinden koşulmaz. Ayarların (klasör, koruma) da gelir.</p>
        <p class="m-not">Eski klasördeki kayıt "taşındı" diye işaretlenir: eski sürümü yanlışlıkla açarsan kumbarasız açılır, aynı kumbaraları ikinci kez çalıştırmaz. Taşımazsan bu sürüm boş başlar; eski kumbaralar (ve ellerindeki coinler) burada görünmez.</p>
        <p class="m-hata" id="mHata" hidden></p>
      </div>
      <div class="m-alt"><button class="btn" data-m="tasiVazgec">Şimdi değil</button><span class="bosluk"></span><button class="btn btn-ana" data-m="tasi" autofocus>Taşı ve devam et</button></div>`, 'tasima');
  }
  async function tasi() {
    if (!TASIMA || modalMesgul) return;
    modalMesgul = true;
    modalHata('');
    try {
      const r = await fetch('/api/tasima', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, klasor: TASIMA.klasor }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      modalMesgul = false;
      modalKapat();
      TASIMA = null;
      bildir(`Eski kayıtlar taşındı: ${j.kumbara} kumbara. Kumbaralar kaldıkları yerden devam ediyor.`);
      try { const r2 = await fetch('/api/ayarlar'); ayarlariYukle(await r2.json()); } catch (e) { /* önemli değil */ }
      await durumYenile(true);
      listeImza = '';
      await listeYenile();
    } catch (e) {
      modalMesgul = false;
      modalHata(hataYazisi(e));
    }
  }
  async function tasiVazgec() {
    modalKapat();
    TASIMA = null;
    try { await fetch('/api/tasima', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, vazgec: true }) }); } catch (e) { /* önemli değil */ }
  }

  // ---------- 0.8: bilgisayardan strateji alma ----------
  // Bilgisayardaki simülatörün "Pi'ye gönder" butonu dosyayı ev ağından buraya yollar (uzak.go). Varsayılan kapalı.
  let UZAK = null, uzakSonGoru = null;
  const UZAK_DURUM_AD = { yeni: 'listeye eklendi', yenilendi: 'aynı adlı dosya yenilendi', calisanKumbara: 'çalışan kumbara yeni ayarla devam edecek' };
  async function uzakYenile(govde) {
    try {
      const r = await fetch('/api/uzak', govde ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(Object.assign({ oturum }, govde)) } : undefined);
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      UZAK = j;
      uzakCip();
      if (modalAcik === 'uzak') { const el = $('#uzIcerik'); if (el) el.innerHTML = uzakIcerik(); }
      return j;
    } catch (e) {
      if (govde) throw e;
      return null;
    }
  }
  function uzakCip() {
    const el = $('#uzakCip');
    if (!el || !UZAK) return;
    const nokta = UZAK.acik ? (UZAK.dinliyor ? 'yesil' : 'kirmizi') : 'gri';
    el.innerHTML = `<span class="nokta ${nokta}"></span><span class="cip-ad">Bilgisayardan al</span><b>${UZAK.acik ? (UZAK.dinliyor ? 'açık' : 'sorun var') : 'kapalı'}</b>`;
  }
  function uzakIcerik() {
    const u = UZAK;
    if (!u) return '<div class="m-govde"><p>Yükleniyor…</p></div>';
    const adresler = [];
    if (u.cihaz) adresler.push(`${esc(u.cihaz)}.local`);
    for (const a of u.adresler || []) adresler.push(esc(a));
    const son = (u.sonGelen || []).slice().reverse();
    return `<div class="m-govde">
      <p>Açınca, bilgisayarındaki <b>Kripto Simülatör</b>'ün <b>Pi'ye gönder</b> butonu strateji dosyasını ev ağı üzerinden buraya yollar. Dosya strateji klasörüne yazılır ve soldaki listede görünür. <b>Kumbarayı yine sen başlatırsın</b>; çalışan bir kumbaranın dosyası gelirse, simülatörde yeniden dışa aktarmadaki gibi eldeki işlem bitince yeni ayarla devam eder.</p>
      <label class="anahtar-satir uzak-anahtar"><span class="anahtar"><input type="checkbox" id="uzAcik" ${u.acik ? 'checked' : ''} aria-label="Bilgisayardan strateji al"><span></span></span><b>${u.acik ? 'Açık: bilgisayardan dosya alınıyor' : 'Kapalı: bilgisayardan dosya alınmıyor'}</b></label>
      ${u.acik && !u.dinliyor ? `<p class="hata-not">Kapı açılamadı${u.hata ? ': ' + esc(u.hata) : ''}. Programı kapatıp yeniden aç.</p>` : ''}
      ${u.acik ? `<div class="uzak-kutu">
        <div class="uzak-satir"><span>Simülatöre yazılacak <b>adres</b></span><code>${adresler[0] || '?'}</code></div>
        ${adresler.length > 1 ? `<div class="uzak-satir alt"><span>Olmazsa bunlardan biri</span><code>${adresler.slice(1).join(' · ')}</code></div>` : ''}
        <div class="uzak-satir"><span>Simülatöre yazılacak <b>eşleştirme kodu</b></span><code class="uzak-kod">${esc(u.kod || '')}</code></div>
        <p class="m-not">Kod, bu kumbaralara dosya gönderebilmenin anahtarıdır; kimseyle paylaşma. Başkası öğrendiyse "Yeni kod"a bas: eski kod hemen geçersiz olur, simülatöre yenisini yazarsın.</p>
      </div>` : ''}
      ${son.length ? `<div class="uzak-gelenler"><div class="m-etiket">Son gelenler</div>${son.map(x => `<div class="uzak-gelen ${x.basari ? '' : 'hatali'}"><time>${gunSaat(x.zaman)}</time><b>${esc(x.dosya || '?')}</b><span>${x.basari ? esc(UZAK_DURUM_AD[x.durum] || x.durum || '') : esc(x.mesaj || 'reddedildi')}</span></div>`).join('')}</div>` : ''}
      <p class="m-hata" id="mHata" hidden></p>
    </div>
    <div class="m-alt">${u.acik ? '<button class="btn" data-m="uzakYeniKod">Yeni kod</button>' : ''}<span class="bosluk"></span><button class="btn btn-ana" data-m="kapat">Tamam</button></div>`;
  }
  async function uzakAc() {
    modalAc(`<div class="m-ust"><h3>Bilgisayardan strateji al</h3><button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div><div id="uzIcerik">${uzakIcerik()}</div>`, 'uzak');
    await uzakYenile();
  }
  async function uzakAcKapat(acik) {
    try { await uzakYenile({ acik }); bildir(acik ? 'Bilgisayardan strateji alma açıldı.' : 'Bilgisayardan strateji alma kapatıldı.'); }
    catch (e) { modalHata(hataYazisi(e)); uzakYenile(); }
  }
  async function uzakYeniKod() {
    const ok = await onayIste('Yeni eşleştirme kodu', 'Eski kod hemen geçersiz olur. Bilgisayardaki simülatöre yeni kodu yazman gerekecek.', 'Yeni kod üret');
    if (!ok) { uzakAc(); return; }
    try { await uzakYenile({ yeniKod: true }); uzakAc(); bildir('Yeni eşleştirme kodu üretildi.'); } catch (e) { bildir('Yeni kod üretilemedi: ' + hataYazisi(e), 'dikkat'); }
  }
  // Bilgisayardan dosya geldiyse haber ver (genel olaylardan)
  function uzakOlaylariBildir() {
    const genel = DURUM ? DURUM.genelOlaylar || [] : [];
    const son = genel.length ? genel[genel.length - 1].zaman : 0;
    if (uzakSonGoru === null) { uzakSonGoru = son; return; }
    for (const o of genel) {
      if (o.zaman <= uzakSonGoru || !o.mesaj.startsWith('Bilgisayardan strateji geldi')) continue;
      bildir(o.mesaj, 'dikkat'); bip(660);
      listeYenile();
      if (modalAcik === 'uzak') uzakYenile();
    }
    uzakSonGoru = son;
  }

  async function klasorDegistir() {
    try {
      const r = await fetch('/api/klasor-sec', { method: 'POST' });
      const j = await r.json();
      if (j.iptal) return;
      if (!r.ok || j.hata) throw new Error(j.hata || r.status);
      bildir('Strateji klasörü: ' + j.klasor);
    } catch (e) {
      const yol = window.prompt(`Klasör seçme penceresi açılamadı (${e.message || e}).\nstratejiler klasörünün yolunu buraya yapıştır:`, (LISTE && LISTE.klasor) || '');
      if (!yol) return;
      const r2 = await fetch('/api/ayarlar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ klasor: yol.trim() }) });
      const j2 = await r2.json().catch(() => ({}));
      if (!r2.ok) { bildir(j2.hata || 'Klasör kaydedilemedi', 'dikkat'); return; }
    }
    AKTIF = null; S.secili = null; kaydet();
    yuklemeNo++;
    grafigiTemizle(); stratejiKartiCiz(); kurallariCiz(); kumbaraKartiCiz();
    listeImza = '';
    await listeYenile();
  }

  // ---------- 0.9: AKIŞ ONAYI (emir akışı + Smart Money Concepts + CVD + açık pozisyon) ----------
  // Program (akis_*.go) MEXC'nin canlı işlemlerini, defterini ve vadeli açık pozisyonunu toplar; simülatörün motoru "al"
  // deyince dört gözle bakıp onay verir ya da vermez. Sayfa her şeyi gösterir: grafikte yapı ve işaretler, altta akış
  // paneli (delta, CVD, açık pozisyon), sağda akış kartı (şu anki puan, maddeler, kararlar), üstte "Akış onayı" çipi
  // (ayarlar ve karne). Hesap programda yapılır; sayfa sadece çizer.
  const AO = {
    veri: null, anahtar: '', istekte: false, son: 0, hata: '', surum: 0,
    ayar: null, varsayilan: null,
    karne: null, karneHata: '', filtre: { kumbara: false, buAyarla: false }, sekme: 'karne',
    fp: null, fpZ: 0, fpIstekte: false,
    zIdx: null, zIdxImza: '',
  };
  let aoGrafik = null, aoZaman = null, aoDelta = null, aoCvd = null, aoVcvd = null, aoOi = null, aoZamanN = -1;
  const AO_BLOK = ['Yapı', 'Emir akışı', 'CVD', 'Açık pozisyon'];
  const AO_BLOK_SORU = ['Fiyat nerede? (destek, kırılım, likidite)', 'Şu an kim saldırıyor? (alıcı mı satıcı mı)', 'Baskı zayıflıyor mu? (birikimli delta)', 'Zorunlu satış bitti mi? (vadeli pozisyonlar)'];
  const AO_KARAR = {
    hesaplaniyor: { ad: 'hesaplanıyor', i: '…', s: 'bekle' }, onay: { ad: 'onay verdi, alındı', i: '✓', s: 'onay' },
    red: { ad: 'reddetti, alınmadı', i: '✕', s: 'red' }, bekliyor: { ad: 'onay bekleniyor', i: '⏳', s: 'bekle' },
    'bekleyip-onay': { ad: 'bekledi, onay verdi, alındı', i: '✓', s: 'onay' }, 'sure-doldu': { ad: 'bekleme süresi doldu, alınmadı', i: '✕', s: 'red' },
    'motor-cikti': { ad: 'motor çıktı, alınmadı', i: '✕', s: 'red' }, 'zarar-durdur': { ad: 'fiyat zarar durdura indi, alınmadı', i: '✕', s: 'red' },
    'fiyat-kacti': { ad: 'fiyat hedefe çıktı, alınmadı', i: '✕', s: 'red' }, iptal: { ad: 'iptal', i: '–', s: 'gri' }, hata: { ad: 'hesap yapılamadı', i: '!', s: 'red' },
    'golge-onay': { ad: 'gölge: onay verirdi', i: '✓', s: 'golge-onay' }, 'golge-red': { ad: 'gölge: reddederdi', i: '✕', s: 'golge-red' },
  };
  const aoKarar = x => AO_KARAR[x] || { ad: x, i: '·', s: 'gri' };
  const aoPuan = x => (x == null || !isFinite(x) ? '–' : (x > 0 ? '+' : x < 0 ? '−' : '') + nf(0).format(Math.abs(Math.round(x))));
  const aoAcik = () => !!(S.gorunum.akis);
  const aoGor = id => aoAcik() && !!(S.gorunum.ao && S.gorunum.ao[id]);
  function aoSayi(x) {
    if (x == null || !isFinite(x)) return '–';
    const a = Math.abs(x), isr = x < 0 ? '−' : '';
    if (a >= 1e9) return isr + nfEn(2).format(a / 1e9) + ' mr';
    if (a >= 1e6) return isr + nfEn(2).format(a / 1e6) + ' mn';
    if (a >= 1e4) return isr + nfEn(1).format(a / 1e3) + ' bin';
    return isr + nfEn(a >= 100 ? 0 : a >= 1 ? 2 : 4).format(a);
  }
  const aoSayiIsaret = x => (x > 0 ? '+' : '') + aoSayi(x);
  // programdan gelen mum zamanı (açılış, ms) → grafikteki mumun sırası (tam eşleşme; yoksa −1)
  function aoIdx(z) {
    const i = mumIndeksi(z);
    return i >= 0 && M.t[i] === z ? i : -1;
  }

  // Alttaki akış paneli: tek grafik, üç kat (üstte açık pozisyon, ortada CVD, altta delta). Ana grafikle birlikte kayar.
  function aoGrafikKur() {
    aoGrafik = LW.createChart($('#aoGrafik'), grafikSecenek(x => aoSayi(x)));
    aoGrafik.applyOptions({
      rightPriceScale: { scaleMargins: { top: 0.70, bottom: 0.02 }, borderColor: RENK.cizgi },
      timeScale: { visible: false }, grid: { horzLines: { visible: false }, vertLines: { color: rgba(RENK.cizgi, 0.45) } },
      handleScale: { axisPressedMouseMove: { price: false } },
    });
    aoZaman = aoGrafik.addLineSeries({ visible: false, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false });
    aoDelta = aoGrafik.addHistogramSeries({ priceScaleId: 'right', priceFormat: { type: 'custom', formatter: aoSayi }, priceLineVisible: false, lastValueVisible: false });
    aoCvd = aoGrafik.addLineSeries({ priceScaleId: 'cvd', color: RENK.camgobegi, lineWidth: 2, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false, priceFormat: { type: 'custom', formatter: aoSayi } });
    aoVcvd = aoGrafik.addLineSeries({ priceScaleId: 'vcvd', color: RENK.mor, lineWidth: 1, lineStyle: LW.LineStyle.Dashed, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false, priceFormat: { type: 'custom', formatter: aoSayi } });
    aoOi = aoGrafik.addLineSeries({ priceScaleId: 'oi', color: RENK.vurgu, lineWidth: 2, priceLineVisible: false, lastValueVisible: false, crosshairMarkerVisible: false, priceFormat: { type: 'custom', formatter: aoSayi } });
    aoGrafik.priceScale('cvd').applyOptions({ scaleMargins: { top: 0.36, bottom: 0.36 } });
    aoGrafik.priceScale('vcvd').applyOptions({ scaleMargins: { top: 0.36, bottom: 0.36 } });
    aoGrafik.priceScale('oi').applyOptions({ scaleMargins: { top: 0.06, bottom: 0.70 } });
    aoGrafik.subscribeCrosshairMove(p => aoLejant(p && p.time != null ? zamanIdx.get(p.time) : null));
    aoGrafik.subscribeClick(p => { if (p && p.time != null && zamanIdx.has(p.time)) incele(zamanIdx.get(p.time)); });
  }
  // Panel görünür mü (görünüm düğmesi "Akış")
  function aoPanelGoster() {
    const acik = aoAcik();
    $('#aoPanel').hidden = !acik;
    if (acik) {
      aoZamanN = -1; aoSerileriDoldur();
      // panel yeni göründü: aralığı ana grafikten alsın (tersi değil)
      requestAnimationFrame(() => { const r = anaGrafik.timeScale().getVisibleLogicalRange(); if (r && aoGrafik) aoGrafik.timeScale().setVisibleLogicalRange(r); });
    }
  }
  // Ana grafikle aynı sırada mum zamanları (boş seri): iki grafiğin mum sırası aynı olsun diye
  function aoZamanDoldur() {
    if (!aoZaman || !M) return;
    const n = k + 1;
    if (n === aoZamanN) return;
    aoZamanN = n;
    aoZaman.setData(aralikDizi(n, i => ({ time: T[i] })));
  }
  function aoSerileriDoldur() {
    if (!aoGrafik) return;
    const bos = () => { aoDelta.setData([]); aoCvd.setData([]); aoVcvd.setData([]); aoOi.setData([]); };
    if (!M || !T.length || !aoAcik()) { bos(); return; }
    aoZamanDoldur();
    const v = AO.veri;
    if (!v || !v.z || v.sembol !== S.sembol || v.aralik !== S.aralik) { bos(); aoLejant(null); return; }
    const eksik = new Set((v.eksik || []).map(j => v.z[j]));
    const d = [], c = [], vc = [], oi = [];
    for (let j = 0; j < v.z.length; j++) {
      const i = aoIdx(v.z[j]);
      if (i < 0 || i > k) continue;
      const t = T[i];
      if (v.delta[j] != null) d.push({ time: t, value: v.delta[j], color: rgba(v.delta[j] >= 0 ? RENK.yukari : RENK.asagi, eksik.has(v.z[j]) ? 0.3 : 0.75) });
      if (v.cvd[j] != null) c.push({ time: t, value: v.cvd[j] });
      if (v.vcvd[j] != null && v.vadeli) vc.push({ time: t, value: v.vcvd[j] });
      if (v.oi[j] != null) oi.push({ time: t, value: v.oi[j] });
    }
    aoDelta.setData(d); aoCvd.setData(c); aoVcvd.setData(vc); aoOi.setData(oi);
    aoLejant(null);
  }
  // Paneldeki değerler (fare hangi mumdaysa; yoksa son mum)
  function aoLejant(i) {
    const el = $('#aoLejant');
    if (!el) return;
    const v = AO.veri;
    if (!v || !M || v.sembol !== S.sembol) { el.innerHTML = `<span class="ao-l-bos">${AO.hata ? esc(AO.hata) : 'Akış verisi bekleniyor…'}</span>`; return; }
    let j = -1;
    if (i != null) { for (let x = v.z.length - 1; x >= 0; x--) if (aoIdx(v.z[x]) === i) { j = x; break; } }
    if (j < 0) j = v.z.length - 1;
    let oiJ = j; // süren mumda açık pozisyon henüz yoksa son bilinen
    while (oiJ > 0 && v.oi[oiJ] == null && j - oiJ < 3) oiJ--;
    const oiDeg = (() => {
      if (v.oi[oiJ] == null) return null;
      for (let x = oiJ - 1; x >= 0; x--) if (v.oi[x] != null) return (v.oi[oiJ] / v.oi[x] - 1) * 100;
      return null;
    })();
    const deg = (ad, renk, deger, ek, kesik) => `<span><i class="${kesik ? 'kesik' : ''}" style="border-color:${renk}"></i>${ad}<b>${deger}</b>${ek || ''}</span>`;
    const parca = [
      `<span class="ao-l-zaman">${tarih(v.z[j])}${j === v.z.length - 1 ? ' (süren mum)' : ''}</span>`,
      deg('Delta', RENK.yukari, v.delta[j] == null ? 'veri yok' : aoSayiIsaret(v.delta[j]), v.hacim[j] ? `<small>hacim ${aoSayi(v.hacim[j])}</small>` : ''),
      deg('CVD spot', RENK.camgobegi, aoSayi(v.cvd[j])),
    ];
    if (v.vadeli) {
      parca.push(deg('CVD vadeli', RENK.mor, aoSayi(v.vcvd[j]), '', true));
      parca.push(deg('Açık pozisyon', RENK.vurgu, v.oi[oiJ] == null ? 'birikiyor' : aoSayi(v.oi[oiJ]), oiDeg != null ? `<small class="${isaretSinif(oiDeg)}">${Bicim.yuzde(oiDeg, 2)} önceki muma göre</small>` : ''));
      if (isFinite(v.fon) && v.fon !== 0) parca.push(`<span>Fonlama<b>${Bicim.yuzde(v.fon * 100, 4)}</b></span>`);
    } else parca.push('<span class="ao-l-bos">vadelisi yok: açık pozisyon ve vadeli CVD gösterilmez</span>');
    el.innerHTML = parca.join('');
  }

  // Seçili coinin akış verisi: 5 saniyede bir (pencere görünürken), mum kapanınca hemen
  async function aoYenile(zorla) {
    if (!AKTIF || !S.sembol || !ARALIK[S.aralik] || !(AKTIF.iz || AKTIF.kumbaraId)) return;
    if (!zorla && document.visibilityState === 'hidden') return;
    const kb = seciliKumbara();
    const anahtar = `${S.sembol}|${S.aralik}|${kb ? kb.id : ''}`;
    if (AO.istekte || (!zorla && anahtar === AO.anahtar && Date.now() - AO.son < 5000)) return;
    AO.istekte = true;
    try {
      const r = await fetch(`/api/akis/grafik?sembol=${encodeURIComponent(S.sembol)}&aralik=${encodeURIComponent(S.aralik)}${kb ? '&kumbara=' + encodeURIComponent(kb.id) : ''}`);
      const j = await r.json().catch(() => ({})); // cevap her zaman okunur (0.5)
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      const kb2 = seciliKumbara();
      if (anahtar !== `${S.sembol}|${S.aralik}|${kb2 ? kb2.id : ''}`) return; // bu arada başka strateji seçildi
      AO.veri = j; AO.anahtar = anahtar; AO.hata = ''; AO.surum++;
      aoSerileriDoldur();
      aoKartCiz();
      if (gecerliSekme() === 'simulator') tabloCiz(); // "Akış" sütunu (değiştiyse)
      sonImza = '';
    } catch (e) {
      AO.hata = hataYazisi(e);
      aoKartCiz();
    } finally {
      AO.son = Date.now(); AO.istekte = false;
    }
  }
  function aoSecimDegisti() {
    AO.veri = null; AO.anahtar = ''; AO.son = 0; AO.fp = null; AO.fpZ = 0; aoZamanN = -1;
    aoSerileriDoldur();
    aoKartCiz();
    aoYenile(true);
  }

  // ---------- grafik üstü çizimler ----------
  function aoCiz(g, x, y, ph, pw, yarim, vr) {
    const v = AO.veri;
    if (!aoAcik() || !v || !v.z || !M || v.sembol !== S.sembol || v.aralik !== S.aralik) return;
    const ix = z => { const i = aoIdx(z); return i < 0 || i > k ? null : i; };
    const xi = z => { const i = ix(z); return i == null ? null : x(i); };
    const ilk = ix(v.z[0]);
    const gorunur = (a, b) => !(b < vr.from - 2 || a > vr.to + 2);
    const yazi = (s, px, py, renk, ops = {}) => {
      g.font = `${ops.kalin ? 600 : 500} ${ops.boy || 10.5}px 'Plex', 'Segoe UI', sans-serif`;
      const w = g.measureText(s).width;
      let lx = ops.orta ? px - w / 2 : ops.sag ? px - w : px;
      lx = Math.max(2, Math.min(pw - w - 2, lx));
      if (ops.zemin !== false) { g.fillStyle = rgba(RENK.panel, 0.82); g.fillRect(lx - 3, py - 10, w + 6, 13); }
      g.fillStyle = renk;
      g.fillText(s, lx, py);
    };
    // akış verisinin başladığı yer
    if (ilk != null && ilk > 0 && gorunur(ilk, ilk)) {
      const xa = x(ilk) - yarim;
      g.strokeStyle = rgba(RENK.camgobegi, 0.35);
      g.setLineDash([2, 4]);
      g.beginPath(); g.moveTo(xa + 0.5, 0); g.lineTo(xa + 0.5, ph); g.stroke();
      g.setLineDash([]);
      yazi('akış verisi →', xa + 4, ph - 74, rgba(RENK.camgobegi, 0.8));
    }
    // indirim / pahalı aralığı
    if (aoGor('yapi') && v.bant) {
      const i0 = ix(v.bant.z);
      const xa = i0 != null ? Math.max(0, x(i0) - yarim) : 0;
      const yU = y(v.bant.ust), yA = y(v.bant.alt), yO = y((v.bant.ust + v.bant.alt) / 2);
      if (yU != null && yA != null && yO != null && xa < pw) {
        g.fillStyle = rgba(RENK.asagi, 0.035); g.fillRect(xa, yU, pw - xa, yO - yU);
        g.fillStyle = rgba(RENK.yukari, 0.035); g.fillRect(xa, yO, pw - xa, yA - yO);
        g.strokeStyle = rgba(RENK.metin3, 0.45); g.setLineDash([1, 3]);
        g.beginPath(); g.moveTo(xa, yO + 0.5); g.lineTo(pw, yO + 0.5); g.stroke();
        g.setLineDash([]);
        yazi('pahalı bölge', pw - 4, yU + 12, rgba(RENK.asagi, 0.75), { sag: true, zemin: false });
        yazi('denge (%50)', pw - 4, yO - 3, rgba(RENK.metin2, 0.7), { sag: true, zemin: false });
        yazi('indirim bölgesi', pw - 4, yA - 4, rgba(RENK.yukari, 0.8), { sag: true, zemin: false });
      }
    }
    // talep/arz bölgeleri (order block) ve boşluklar (FVG)
    if (aoGor('bolge')) {
      for (const b of v.bolgeler || []) {
        if (b.ic && !aoGor('ic')) continue;
        const i0 = ix(b.z0), i1 = b.z1 ? ix(b.z1) : k;
        if (i0 == null || i1 == null || !gorunur(i0, i1)) continue;
        const xa = x(i0) - yarim, xb = b.z1 ? x(i1) + yarim : pw, ya = y(b.ust), yb = y(b.alt);
        if (xa == null || xb == null || ya == null || yb == null) continue;
        const ob = b.tur === 'OB', renk = ob ? (b.yon > 0 ? RENK.yukari : RENK.asagi) : (b.yon > 0 ? RENK.camgobegi : RENK.mor);
        const bitti = !!b.z1;
        g.fillStyle = rgba(renk, bitti ? 0.04 : ob ? 0.11 : 0.08);
        g.fillRect(xa, ya, xb - xa, Math.max(1, yb - ya));
        g.strokeStyle = rgba(renk, bitti ? 0.25 : 0.55);
        g.setLineDash(ob && !b.ic ? [] : [4, 3]);
        g.strokeRect(xa + 0.5, ya + 0.5, xb - xa - 1, Math.max(1, yb - ya - 1));
        g.setLineDash([]);
        if (!bitti && ob && !b.ic && xb - xa > 60) yazi(b.yon > 0 ? 'talep bölgesi' : 'arz bölgesi', xa + 4, ya + 11, rgba(renk, 0.95), { zemin: false });
      }
    }
    if (aoGor('yapi')) {
      // eşit tepe/dipler (likidite havuzu)
      for (const q of v.esitler || []) {
        const i0 = ix(q.z0), i1 = ix(q.z1);
        if (i0 == null || i1 == null) continue;
        const iS = q.supurme ? ix(q.supurme) : null;
        const son = iS != null ? iS : Math.min(k, i1 + 15);
        if (!gorunur(i0, son)) continue;
        const ya = y(q.f);
        if (ya == null) continue;
        g.strokeStyle = rgba(RENK.vurgu, q.supurme ? 0.4 : 0.8);
        g.setLineDash([2, 3]);
        g.beginPath(); g.moveTo(x(i0), ya + 0.5); g.lineTo(x(son), ya + 0.5); g.stroke();
        g.setLineDash([]);
        yazi(q.tepe ? (q.supurme ? 'eşit tepeler · süpürüldü' : 'eşit tepeler') : (q.supurme ? 'eşit dipler · süpürüldü' : 'eşit dipler'), x(i1) + 4, q.tepe ? ya - 4 : ya + 12, rgba(RENK.vurgu, 0.9), { zemin: false });
      }
      // yapı kırılımları
      for (const kr of v.kirilimlar || []) {
        if (kr.ic && !aoGor('ic')) continue;
        const i0 = ix(kr.z0), i1 = ix(kr.z);
        if (i0 == null || i1 == null || !gorunur(i0, i1)) continue;
        const ya = y(kr.f);
        if (ya == null) continue;
        const renk = kr.yon > 0 ? RENK.yukari : RENK.asagi;
        g.strokeStyle = rgba(renk, kr.ic ? 0.55 : 0.9);
        g.lineWidth = kr.ic ? 1 : 1.5;
        g.setLineDash(kr.ic ? [2, 3] : kr.choch ? [] : [6, 3]);
        g.beginPath(); g.moveTo(x(i0), ya + 0.5); g.lineTo(x(i1), ya + 0.5); g.stroke();
        g.setLineDash([]); g.lineWidth = 1;
        const ad = (kr.ic ? 'iç ' : '') + (kr.choch ? 'CHoCH' : 'BOS');
        yazi(ad, (x(i0) + x(i1)) / 2, kr.yon > 0 ? ya - 4 : ya + 12, rgba(renk, 0.95), { orta: true, kalin: !kr.ic, boy: kr.ic ? 9.5 : 10.5 });
      }
      // salınım etiketleri (HH/HL/LH/LL)
      for (const s of v.salinimlar || []) {
        if (s.ic && !aoGor('ic')) continue;
        const i = ix(s.z);
        if (i == null || !gorunur(i, i)) continue;
        const ya = y(s.f);
        if (ya == null) continue;
        const iyi = s.etiket === 'HH' || s.etiket === 'HL';
        yazi(s.etiket, x(i), s.tepe ? ya - 8 : ya + 17, rgba(iyi ? RENK.yukari : RENK.asagi, s.ic ? 0.6 : 0.95), { orta: true, kalin: !s.ic, boy: s.ic ? 9 : 10.5, zemin: false });
      }
      // likidite süpürmeleri
      for (const s of v.supurmeler || []) {
        if (!s.guclu && !aoGor('ic')) continue; // iç yapının süpürmeleri sadece "İç yapı" açıkken
        const i = ix(s.z);
        if (i == null || !gorunur(i, i)) continue;
        const xs = x(i), yl = y(s.yon > 0 ? M.l[i] : M.h[i]);
        if (yl == null) continue;
        const renk = rgba(RENK.vurgu, s.guclu ? 1 : 0.7);
        const yy = s.yon > 0 ? yl + 30 : yl - 30, r = s.guclu ? 6 : 4.5;
        g.fillStyle = renk;
        g.beginPath();
        if (s.yon > 0) { g.moveTo(xs, yy - r); g.lineTo(xs + r, yy + r * 0.7); g.lineTo(xs - r, yy + r * 0.7); }
        else { g.moveTo(xs, yy + r); g.lineTo(xs + r, yy - r * 0.7); g.lineTo(xs - r, yy - r * 0.7); }
        g.closePath(); g.fill();
        yazi('süpürme', xs, s.yon > 0 ? yy + 16 : yy - 9, renk, { orta: true, kalin: true, boy: 9.5 });
      }
    }
    if (aoGor('akis')) {
      // her mumun en çok işlem gören fiyatı (POC): yakınlaşınca
      if (yarim >= 3) {
        g.strokeStyle = rgba(RENK.metin, 0.55);
        for (let j = 0; j < v.z.length; j++) {
          if (v.poc[j] == null) continue;
          const i = ix(v.z[j]);
          if (i == null || !gorunur(i, i)) continue;
          const yp = y(v.poc[j]);
          if (yp == null) continue;
          g.beginPath(); g.moveTo(x(i) - yarim + 1, yp + 0.5); g.lineTo(x(i) + yarim - 1, yp + 0.5); g.stroke();
        }
      }
      // emilim ve üst üste dengesizlik
      for (const s of v.isaretler || []) {
        const i = ix(s.z);
        if (i == null || !gorunur(i, i)) continue;
        const xs = x(i), yp = y(s.f);
        if (yp == null) continue;
        if (s.tur === 'emilimDip' || s.tur === 'emilimTepe') {
          const dip = s.tur === 'emilimDip', renk = dip ? RENK.yukari : RENK.asagi, yy = dip ? yp + 14 : yp - 14;
          g.beginPath(); g.arc(xs, yy, 7, 0, Math.PI * 2);
          g.fillStyle = rgba(RENK.panel, 0.9); g.fill();
          g.lineWidth = 1.5; g.strokeStyle = renk; g.stroke(); g.lineWidth = 1;
          g.font = "700 9.5px 'Plex', 'Segoe UI', sans-serif"; g.fillStyle = renk;
          g.fillText('E', xs - 3, yy + 3.5);
        } else {
          const al = s.tur === 'alimYigin', renk = al ? RENK.yukari : RENK.asagi, xr = xs + yarim + 2;
          g.fillStyle = rgba(renk, 0.9);
          for (let n = 0; n < 3; n++) g.fillRect(xr, yp - 4 + n * 3, 6, 1.6);
        }
      }
    }
    // büyük işlemler: daire, büyüklüğü tutarla
    if (aoGor('buyuk') && (v.buyukler || []).length) {
      let enBuyuk = 0;
      for (const b of v.buyukler) enBuyuk = Math.max(enBuyuk, b.f * b.a);
      for (const b of v.buyukler) {
        const i = mumIndeksi(b.z);
        if (i < 0 || i > k || !gorunur(i, i)) continue;
        const yb = y(b.f);
        if (yb == null) continue;
        const r = 2.5 + 9 * Math.sqrt((b.f * b.a) / Math.max(1e-9, enBuyuk));
        g.beginPath(); g.arc(x(i), yb, r, 0, Math.PI * 2);
        g.fillStyle = rgba(b.al ? RENK.yukari : RENK.asagi, 0.28); g.fill();
        g.strokeStyle = rgba(b.al ? RENK.yukari : RENK.asagi, 0.85); g.stroke();
      }
    }
    // motorun alım sinyalleri ve akışın kararı (seçili kumbaranın)
    for (const r of v.kararlar || []) {
      const i = ix(r.motorMum);
      if (i == null || !gorunur(i, i)) continue;
      const kd = aoKarar(r.karar);
      const yl = y(M.l[i]);
      if (yl == null) continue;
      const golge = kd.s.startsWith('golge'), onay = kd.s === 'onay' || kd.s === 'golge-onay', red = kd.s === 'red' || kd.s === 'golge-red';
      const renk = onay ? RENK.yukari : red ? RENK.asagi : kd.s === 'bekle' ? RENK.vurgu : RENK.metin3;
      const s = `${kd.i} ${r.karar === 'hesaplaniyor' ? '' : r.karar === 'bekleyip-onay' && r.onayToplam ? aoPuan(r.toplam) + '→' + aoPuan(r.onayToplam) : aoPuan(r.toplam)}`.trim();
      g.font = "700 10.5px 'JBMono', Consolas, monospace";
      const w = g.measureText(s).width + 10, xs = x(i) - w / 2, ys = yl + 30;
      g.fillStyle = golge ? rgba(RENK.panel, 0.92) : rgba(renk, 0.92);
      g.beginPath(); g.roundRect ? g.roundRect(xs, ys, w, 16, 8) : g.rect(xs, ys, w, 16); g.fill();
      g.setLineDash(golge ? [3, 2] : []);
      g.strokeStyle = renk; g.stroke(); g.setLineDash([]);
      g.fillStyle = golge ? renk : RENK.zemin;
      g.fillText(s, xs + 5, ys + 12);
      if (r.karar === 'bekleyip-onay' && r.onayZ) {
        const j2 = mumIndeksi(r.onayZ + borsaFarki);
        if (j2 > i && j2 <= k) {
          const yo = y(r.onayFiyat);
          if (yo != null) {
            g.strokeStyle = rgba(RENK.yukari, 0.8); g.setLineDash([2, 2]);
            g.beginPath(); g.moveTo(x(i), ys); g.lineTo(x(j2), yo); g.stroke(); g.setLineDash([]);
            g.beginPath(); g.arc(x(j2), yo, 4, 0, Math.PI * 2); g.fillStyle = RENK.yukari; g.fill();
          }
        }
      }
    }
  }

  // ---------- akış kartı (sağda) ----------
  function aoGostergeHTML(toplam, esik) {
    const yer = p => ((Math.max(-100, Math.min(100, p)) + 100) / 200) * 100;
    const t = yer(toplam), e = yer(esik), o = 50;
    const sol = Math.min(t, o), gen = Math.abs(t - o);
    return `<div class="ao-gosterge" title="−100: kesin satış baskısı · +100: kesin alıcı · çizgi: onay eşiği">
      <div class="ao-g-zemin"><i class="${toplam >= 0 ? 'pos' : 'neg'}" style="left:${sol}%;width:${gen}%"></i><b style="left:${t}%"></b><em style="left:${e}%"></em></div>
      <div class="ao-g-olcek"><span>−100</span><span>0</span><span>+100</span></div></div>`;
  }
  function aoBlokHTML(b, i, ayar) {
    const agirlik = ayar && ayar.agirlik ? ayar.agirlik[i] : b.agirlik;
    const puanSinif = !b.var ? 'yok' : b.puan > 0 ? 'pos' : b.puan < 0 ? 'neg' : '';
    const w = b.var ? Math.min(50, Math.abs(b.puan) / 2) : 0;
    const maddeler = b.var
      ? (b.maddeler || []).length ? `<ul class="ao-maddeler">${b.maddeler.map(m => `<li><b class="${m.p > 0 ? 'pos' : 'neg'}">${aoPuan(m.p)}</b><span>${esc(m.m)}</span></li>`).join('')}</ul>` : '<p class="ao-blok-not">Belirgin bir işaret yok (0).</p>'
      : `<p class="ao-blok-not">${esc(b.neden || 'veri yok')}</p>`;
    return `<div class="ao-blok ${b.var ? '' : 'veri-yok'}${agirlik === 0 ? ' agirliksiz' : ''}">
      <div class="ao-blok-ust"><b>${esc(AO_BLOK[i] || b.ad)}</b><small>${AO_BLOK_SORU[i]}</small><span class="ao-agirlik" title="Ağırlık (ayarlardan değişir)">×${nfEn(1).format(agirlik)}</span><span class="ao-puan ${puanSinif}">${b.var ? aoPuan(b.puan) : '–'}</span></div>
      <div class="ao-cubuk"><i class="${b.puan >= 0 ? 'pos' : 'neg'}" style="${b.puan >= 0 ? 'left:50%' : `left:${50 - w}%`};width:${w}%"></i></div>
      ${maddeler}</div>`;
  }
  const AO_BAGLAM = {
    dip: ['Düşüşte alım', 'Satış bitti mi, alıcı geldi mi?'],
    kirilim: ['Kırılımda alım', 'Alıcılar gerçek mi?'],
    yatay: ['Yatay piyasada alım', 'Alıcı tarafı ağır basıyor mu?'],
  };
  function aoKayitSatiri(r, tiklanir) {
    const kd = aoKarar(r.karar);
    const sonuc = r.sonuc != null ? r.sonuc : null, motor = r.motorSonuc != null ? r.motorSonuc : null;
    let sonucH = '';
    if (motor != null) {
      const alinmadi = kd.s === 'red' || kd.s === 'golge-red';
      sonucH = `<b class="${isaretSinif(sonuc)}">${Bicim.yuzde(sonuc, 2)}</b>${alinmadi ? `<small>${sonuc < 0 ? 'kaçınıldı' : 'kaçırıldı'}</small>` : ''}`;
    } else if (kd.s !== 'gri' && r.karar !== 'hesaplaniyor') sonucH = '<small>işlem sürüyor</small>';
    return `<li class="ao-kayit ${kd.s}" ${tiklanir ? `data-ao-mum="${r.motorMum}"` : ''} title="${esc((r.ozet || '') + (r.not ? ' · ' + r.not : ''))}">
      <time>${saatYaz(r.z)}</time><i>${kd.i}</i><span class="ao-k-ad">${esc(kd.ad)}${r.golge === 'kenarda' ? ' (kenarda)' : ''}</span>
      <span class="ao-k-puan">${r.karar === 'hesaplaniyor' ? '' : r.karar === 'bekleyip-onay' && r.onayToplam ? `${aoPuan(r.toplam)}→${aoPuan(r.onayToplam)}` : aoPuan(r.toplam)}</span><span class="ao-k-sonuc">${sonucH}</span></li>`;
  }
  let aoKartImza = '';
  function aoKartCiz() {
    const el = $('#aoKart');
    if (!el) return;
    if (!AKTIF || (!AKTIF.iz && !AKTIF.kumbaraId)) { el.hidden = true; aoKartImza = ''; return; }
    el.hidden = false;
    const v = AO.veri && AO.veri.sembol === S.sembol && AO.veri.aralik === S.aralik ? AO.veri : null;
    const kb = seciliKumbara();
    const ak = kb && kb.akis ? kb.akis : null;
    const imza = JSON.stringify([AKTIF.ad, AO.surum, AO.hata, !!v, kb ? kb.id : '', ak, AO.ayar && [AO.ayar.acik, AO.ayar.agirlik]]);
    if (imza === aoKartImza) return;
    aoKartImza = imza;
    const genelAcik = AO.ayar ? !!AO.ayar.acik : ak ? !!ak.genelAcik : true;
    let rozet;
    if (!kb) rozet = '<span class="ao-rozet izle">sadece izleniyor</span>';
    else if (ak && ak.uygulaniyor) rozet = '<span class="ao-rozet acik">açık · alımlar onay bekler</span>';
    else if (!genelAcik) rozet = '<span class="ao-rozet kapali">genel ayarda kapalı · gölge</span>';
    else rozet = '<span class="ao-rozet kapali">bu kumbarada kapalı · gölge</span>';
    let govde = '';
    if (!v) {
      govde = `<p class="ao-not">${AO.hata ? `Akış hesabı alınamadı: ${esc(AO.hata)}` : 'Akış verisi getiriliyor…'}</p>`;
    } else {
      const s = v.sonuc;
      const bg = AO_BAGLAM[s.baglam] || AO_BAGLAM.yatay;
      let karar;
      if (s.eksik) karar = `<div class="ao-karar red"><b>Onay vermezdi</b><span>${esc(s.eksik)}</span></div>`;
      else if (s.yasak) karar = `<div class="ao-karar red"><b>Onay vermezdi · engel</b><span>${esc(s.yasak)}</span></div>`;
      else if (s.onay) karar = `<div class="ao-karar onay"><b>Onay verirdi</b><span>${aoPuan(s.toplam)} ≥ eşik ${aoPuan(s.esik)}</span></div>`;
      else karar = `<div class="ao-karar red"><b>Onay vermezdi</b><span>${aoPuan(s.toplam)} &lt; eşik ${aoPuan(s.esik)}</span></div>`;
      // veri durumu
      let ilkVeri = -1;
      for (let j = 0; j < v.z.length; j++) if (v.delta[j] != null) { ilkVeri = j; break; }
      const sure = ilkVeri >= 0 ? borsaSimdi() - v.z[ilkVeri] : 0;
      const eksikSay = (v.eksik || []).length;
      const durum = String(s.durum || '');
      const veri = `<div class="ao-veri"><span class="nokta ${/Canlı/.test(durum) ? 'yesil' : 'kirmizi'}"></span>${esc(durum || 'bilinmiyor')}${ilkVeri >= 0 ? ` · toplanan veri: son ${sureUzun(sure)}` : ''}${eksikSay ? ` · ${eksikSay} mumda eksik` : ''}${v.vadeli ? ' · vadeli var' : ' · vadelisi yok'}</div>`;
      govde = `
        <div class="ao-soru"><span>${esc(bg[0])}</span><b>“${esc(bg[1])}”</b><small>Simülatörün motoru şu an alsa sistemin cevabı (son kapanan mum + süren mum):</small></div>
        ${aoGostergeHTML(s.toplam, s.esik)}
        ${karar}
        <div class="ao-bloklar">${(s.bloklar || []).map((b, i) => aoBlokHTML(b, i, AO.ayar)).join('')}</div>
        ${veri}`;
    }
    // bu kumbaranın bekleyen onayı ve son kararları
    let kumbaraH = '';
    if (kb) {
      const kayitlar = (v && v.kararlar && v.kararlar.length ? v.kararlar : ak && ak.kayitlar ? ak.kayitlar : []).slice(-6).reverse();
      let bek = '';
      if (ak && ak.bekleme) {
        const kalan = ak.bekleme.bitis - Date.now();
        const son = kayitlar.find(r => r.motorMum === ak.bekleme.motorMum);
        bek = `<div class="ao-bekleme">⏳ Simülatör ${saatYaz(ak.bekleme.motorMum + (ARALIK[kb.aralik] ? ARALIK[kb.aralik].ms : 0))} kapanışında aldı (${fg(ak.bekleme.fiyat)}); onay bekleniyor · <b data-ao-geri="${ak.bekleme.bitis}">${kalanYaz(kalan)}</b> kaldı${son && son.sonToplam ? ` · son puan ${aoPuan(son.sonToplam)}` : ''}</div>`;
      }
      const o = ak && ak.onaylanan, r = ak && ak.reddedilen;
      const ozet = o && r && o.n + r.n > 0
        ? `<div class="ao-mini-karne"><span>Bu kumbarada sonucu belli:</span><b class="pos">onaylanan ${o.n} · %${o.n ? Math.round((o.kazanan / o.n) * 100) : 0} kârlı · ort. ${Bicim.yuzde(o.ort, 2)}</b><b class="neg">reddedilen ${r.n} · %${r.n ? Math.round((r.kazanan / r.n) * 100) : 0} kârlı · ort. ${Bicim.yuzde(r.ort, 2)}</b></div>` : '';
      kumbaraH = `${bek}
        <div class="ao-baslik2">Bu kumbaranın akış kararları</div>
        ${kayitlar.length ? `<ul class="ao-kayitlar">${kayitlar.map(r2 => aoKayitSatiri(r2, true)).join('')}</ul>` : '<p class="ao-not">Henüz karar yok: simülatörün motoru alım yapınca burada görünür.</p>'}
        ${ozet}
        <label class="anahtar-satir ao-anahtar"><span class="anahtar"><input type="checkbox" id="aoKumbaraAcik" ${ak && ak.kapali ? '' : 'checked'} ${genelAcik ? '' : 'disabled'} aria-label="Bu kumbarada akış onayı"><span></span></span>
          <b>${!genelAcik ? 'Genel ayarda kapalı (çipten açılır)' : ak && ak.kapali ? 'Bu kumbarada kapalı: simülatör gibi hemen alır' : 'Bu kumbarada açık: alım onay bekler'}</b></label>`;
    } else {
      kumbaraH = '<p class="ao-not">Bu stratejinin kumbarası yok. Kumbara başlatılırsa simülatörün alımları bu onaya bağlanır (açıkken).</p>';
    }
    el.innerHTML = `
      <div class="kart-ust"><h2>Akış onayı</h2>${rozet}</div>
      ${govde}
      ${kumbaraH}
      <div class="ao-butonlar"><button class="btn kucuk" data-ao="karne">Karne</button><button class="btn kucuk" data-ao="ayarlar">Ayarlar</button><span class="bosluk"></span><button class="btn kucuk" data-ao="nedir" title="Dört aracın ne yaptığı">Nasıl çalışır?</button></div>`;
  }

  // Üstteki çip
  function aoCip() {
    const el = $('#aoCip');
    if (!el) return;
    const a = AO.ayar;
    const acik = a ? a.acik : true;
    el.innerHTML = `<span class="nokta ${acik ? 'yesil' : 'gri'}"></span><span class="cip-ad">Akış onayı</span><b>${acik ? 'açık' : 'kapalı'}</b>`;
  }
  async function aoAyarGetir() {
    try {
      const r = await fetch('/api/akis/ayar');
      const j = await r.json().catch(() => ({}));
      if (r.ok && j.ayar) { AO.ayar = j.ayar; AO.varsayilan = j.varsayilan; }
    } catch (e) { /* sonra yeniden */ }
    aoCip();
  }

  // ---------- ayarlar ve karne penceresi ----------
  const AO_ALANLAR = [
    { grup: 'Karar', alanlar: [
      { id: 'esik', ad: 'Onay eşiği (puan)', min: -100, max: 100, adim: 1, not: 'Dört bölümün ağırlıklı ortalaması en az bu kadarsa onay verilir. Yükseltirsen daha az ama daha seçici alım; düşürürsen simülatöre yaklaşır.' },
      { id: 'beklemeMum', ad: 'Onay gelmezse beklenecek mum', min: 0, max: 10, adim: 1, tam: true, not: 'Simülatör aldığı an onay yoksa bu kadar mum boyunca 2 saniyede bir yeniden bakılır; onay gelirse o an alınır. 0 = beklemez.' },
      { id: 'kovalama', ad: 'Beklerken en fazla yukarıdan al (%)', min: 0, max: 3, adim: 0.05, not: 'Fiyat simülatörün alış fiyatından bundan fazla yukarıdaysa onay gelse de alınmaz (pahalıya kovalamasın).' },
      { id: 'akisSart', ad: 'Emir akışı verisi yoksa onay verme', onay: true, not: 'Program yeni açıldıysa ya da bağlantı koptuysa son mumların işlem verisi eksik olur. Kapatırsan o zaman sadece mumlara (yapıya) bakılır.' },
      { id: 'yasakYapi', ad: 'Ana yapı az önce aşağı kırıldıysa alma', onay: true, not: 'Son 5 mumda ana yapı aşağı kırıldıysa (ve ardından stoplar süpürülüp geri alınmadıysa) puan ne olursa olsun onay yok: "düşen bıçağı tutma".' },
      { id: 'yasakSaldiri', ad: 'Satış saldırısı sürüyorsa alma', onay: true, not: 'Süren mumda satıcılar çok baskınsa ve fiyat son mumun altına inmişse onay yok.' },
    ] },
    { grup: 'Ağırlıklar (0 = hesaba katılmaz)', agirlik: true },
    { grup: 'Yapı (Smart Money Concepts)', alanlar: [
      { id: 'icUzunluk', ad: 'İç yapı salınım uzunluğu (mum)', min: 2, max: 20, adim: 1, tam: true, not: 'Bir tepe/dip, iki yanındaki bu kadar mumdan yüksek/düşükse sayılır (küçük dalgalar). Yaygın değer 5.' },
      { id: 'anaUzunluk', ad: 'Ana yapı salınım uzunluğu (mum)', min: 5, max: 200, adim: 1, tam: true, not: 'Büyük dalgalar için. Yaygın değer 50. Kısa mumlarda (1–5 dk) daha küçük değer daha sık değişir.' },
      { id: 'esitHassas', ad: 'Eşit tepe/dip hassasiyeti (ATR katı)', min: 0.01, max: 1, adim: 0.01, not: 'İki dip arasındaki fark ortalama mum boyunun bu katından azsa "eşit dipler" (altında stoplar birikir).' },
      { id: 'supurmeMum', ad: 'Süpürme kaç mum geçerli', min: 1, max: 50, adim: 1, tam: true, not: 'Eski dibin altına inip geri dönen fitil, bu kadar mum boyunca "stoplar süpürüldü" diye puan verir.' },
      { id: 'fvgEsik', ad: 'En küçük boşluk (FVG, ATR katı)', min: 0.01, max: 2, adim: 0.01, not: 'Üç mumluk fiyat boşluğu bundan küçükse sayılmaz.' },
    ] },
    { grup: 'Emir akışı (Order Flow)', alanlar: [
      { id: 'dengesizlikOran', ad: 'Dengesizlik oranı (kat)', min: 1.5, max: 10, adim: 0.1, not: 'Bir fiyat kademesindeki saldırgan alım, bir alt kademedeki satışın bu katıysa dengesizlik (3 = %300; yaygın 3, ATAS varsayılanı 1,5).' },
      { id: 'dengesizlikKademe', ad: 'Üst üste kaç kademe', min: 2, max: 8, adim: 1, tam: true, not: 'Bu kadar kademe üst üste dengesizlikse "yığın" sayılır (yaygın 3).' },
      { id: 'kademeSayisi', ad: 'Ortalama mum kaç kademeye bölünsün', min: 3, max: 30, adim: 1, tam: true, not: 'Footprint satırlarının inceliği. Çok artırırsan her satıra az işlem düşer.' },
      { id: 'emilimHacim', ad: 'Emilim için hacim (ortalamanın katı)', min: 1, max: 5, adim: 0.1, not: 'Dipte yüksek hacimli satış fiyatı düşüremezse "emildi" sayılır; hacim ortalamanın en az bu katı olmalı.' },
    ] },
    { grup: 'CVD (birikimli delta)', alanlar: [
      { id: 'cvdBakis', ad: 'Kaç muma bakılsın', min: 5, max: 100, adim: 1, tam: true, not: 'CVD eğilimi ve fiyatla uyumsuzluk bu kadar mumda aranır.' },
    ] },
    { grup: 'Açık pozisyon (Open Interest, vadeli)', alanlar: [
      { id: 'oiBakis', ad: 'Kaç muma bakılsın', min: 5, max: 100, adim: 1, tam: true, not: 'Fiyat ile açık pozisyonun birlikte yönü bu kadar mumda ölçülür.' },
      { id: 'oiEsik', ad: 'Anlamlı değişim (%)', min: 0.05, max: 10, adim: 0.05, not: 'Açık pozisyon bundan az değiştiyse yorum yapılmaz.' },
      { id: 'fonYuksek', ad: 'Yüksek fonlama (%)', min: 0.005, max: 1, adim: 0.005, not: 'Fonlama bunun üstündeyse long\'lar kalabalık (düşüş riski) sayılır.' },
    ] },
  ];
  function aoSayiYaz(x, adim) { return nfEn(Math.max(0, ondalik(adim))).format(x); }
  function aoAyarHTML() {
    const a = AO.ayar, vs = AO.varsayilan || {};
    if (!a) return '<div class="m-govde"><p>Ayarlar okunuyor…</p></div>';
    const satir = f2 => {
      const vd = vs[f2.id];
      if (f2.onay) return `<label class="ao-a-satir onay"><input type="checkbox" data-ao-alan="${f2.id}" ${a[f2.id] ? 'checked' : ''}><span><b>${f2.ad}</b><small>${f2.not}${vd != null ? ` Varsayılan: ${vd ? 'açık' : 'kapalı'}.` : ''}</small></span></label>`;
      return `<label class="ao-a-satir"><span><b>${f2.ad}</b><small>${f2.not}${vd != null ? ` Varsayılan: ${aoSayiYaz(vd, f2.adim)}.` : ''}</small></span>
        <input class="m-girdi rakam" type="number" data-ao-alan="${f2.id}" min="${f2.min}" max="${f2.max}" step="${f2.adim}" value="${a[f2.id]}"></label>`;
    };
    const gruplar = AO_ALANLAR.map(gr => {
      if (gr.agirlik) {
        return `<div class="ao-a-grup"><h4>${gr.grup}</h4>${AO_BLOK.map((ad, i) => `<label class="ao-a-satir"><span><b>${ad}</b><small>${AO_BLOK_SORU[i]}</small></span>
          <input class="m-girdi rakam" type="number" data-ao-agirlik="${i}" min="0" max="3" step="0.5" value="${a.agirlik[i]}"></label>`).join('')}
          <p class="m-not">Toplam puan, verisi olan bölümlerin ağırlıklı ortalamasıdır. Karnede "Hangi bölüm işe yarıyor?" tablosuna bakıp iyi ayıran bölümün ağırlığını artır.</p></div>`;
      }
      return `<div class="ao-a-grup"><h4>${gr.grup}</h4>${gr.alanlar.map(satir).join('')}</div>`;
    }).join('');
    return `<div class="m-govde ao-ayar">
      <label class="anahtar-satir ao-genel"><span class="anahtar"><input type="checkbox" id="aoGenelAcik" ${a.acik ? 'checked' : ''} aria-label="Akış onayı"><span></span></span>
        <b>${a.acik ? 'Açık: kumbaraların alımları akış onayı bekler' : 'Kapalı: alımlar simülatör gibi hemen yapılır (sistemin kararı gölge olarak kaydedilir)'}</b></label>
      ${gruplar}
      <p class="m-not">Bu sayılar yaygın kullanılan değerler ve ilk tahmindir; sistemin kâr getirdiği kanıtlanmış değildir. Bir ayarı değiştirince karnede "Bu ayarlarla" seçeneği sadece yeni ayarla verilen kararları gösterir.</p>
      <p class="m-hata" id="mHata" hidden></p>
    </div>
    <div class="m-alt"><button class="btn" data-m="aoVarsayilan">Varsayılana dön</button><span class="bosluk"></span><button class="btn" data-m="kapat">Vazgeç</button><button class="btn btn-ana" data-m="aoKaydet">Kaydet</button></div>`;
  }
  const yuzdeOran = s => (s && s.n ? (s.kazanan / s.n) * 100 : null);
  function aoKarneKutu(baslik, alt, s, sinif) {
    const oran = yuzdeOran(s);
    return `<div class="ao-kk ${sinif}"><div class="ao-kk-bas"><b>${baslik}</b><small>${alt}</small></div>
      <div class="ao-kk-oran">${oran == null ? '–' : '%' + nf(0).format(oran)}<small>kazandı</small></div>
      <div class="ao-kk-cubuk"><i style="width:${oran == null ? 0 : oran}%"></i><em title="Hedef: 100 işlemden 51'i"></em></div>
      <div class="ao-kk-alt"><span>${s.n} sinyal · ${s.kazanan} kârlı</span><span>ort. <b class="${isaretSinif(s.ort)}">${s.n ? Bicim.yuzde(s.ort, 2) : '–'}</b> · toplam <b class="${isaretSinif(s.top)}">${s.n ? Bicim.yuzde(s.top, 1) : '–'}</b></span></div></div>`;
  }
  function aoKarneHTML() {
    const kr = AO.karne;
    const kb = seciliKumbara();
    const filtre = `<div class="ao-filtre">
      <div class="seg">${kb ? `<button data-ao-filtre="kumbara" data-deger="0" aria-pressed="${!AO.filtre.kumbara}">Bütün kumbaralar</button><button data-ao-filtre="kumbara" data-deger="1" aria-pressed="${AO.filtre.kumbara}">Bu kumbara</button>` : '<button aria-pressed="true">Bütün kumbaralar</button>'}</div>
      <div class="seg"><button data-ao-filtre="buAyarla" data-deger="0" aria-pressed="${!AO.filtre.buAyarla}">Bütün kayıtlar</button><button data-ao-filtre="buAyarla" data-deger="1" aria-pressed="${AO.filtre.buAyarla}">Bu ayarlarla</button></div></div>`;
    if (!kr) return `<div class="m-govde">${filtre}<p>${AO.karneHata ? esc(AO.karneHata) : 'Karne hesaplanıyor…'}</p></div><div class="m-alt"><button class="btn btn-ana" data-m="kapat">Kapat</button></div>`;
    const esik = AO.ayar ? AO.ayar.esik : null;
    const esikTablo = (kr.esikler || []).map(e => {
      const o = yuzdeOran(e);
      return `<tr class="${esik != null && Math.abs(e.esik - esik) < 5 ? 'simdiki' : ''}"><td>${aoPuan(e.esik)}</td><td>${e.n}</td><td>${o == null ? '–' : `<span class="${o >= 51 ? 'pos' : 'neg'}">%${nf(0).format(o)}</span>`}</td><td class="${isaretSinif(e.ort)}">${e.n ? Bicim.yuzde(e.ort, 2) : '–'}</td><td class="${isaretSinif(e.top)}">${e.n ? Bicim.yuzde(e.top, 1) : '–'}</td></tr>`;
    }).join('');
    const blokHucre = s => (s.n ? `${s.n} · <span class="${yuzdeOran(s) >= 51 ? 'pos' : 'neg'}">%${nf(0).format(yuzdeOran(s))}</span> · <span class="${isaretSinif(s.ort)}">${Bicim.yuzde(s.ort, 2)}</span>` : '–');
    const blokTablo = AO_BLOK.map((ad, i) => `<tr><td>${ad}</td><td>${blokHucre(kr.bloklar[i][0])}</td><td>${blokHucre(kr.bloklar[i][1])}</td></tr>`).join('');
    const kayitlar = (kr.kayitlar || []).slice(0, 80).map(x => {
      const r = x.kayit, kd = aoKarar(r.karar);
      return `<tr class="${kd.s}"><td>${gunSaat(r.z)}</td><td>${esc(x.sembol)} · ${aralikAd(x.aralik)}</td><td><i class="ao-t-i">${kd.i}</i>${esc(kd.ad)}</td><td>${r.karar === 'hesaplaniyor' ? '' : aoPuan(r.toplam)}${r.engel ? ' <small>engel</small>' : ''}</td>
        <td class="${isaretSinif(r.motorSonuc)}">${r.motorSonuc != null ? Bicim.yuzde(r.motorSonuc, 2) : '–'}</td><td class="${isaretSinif(r.sonuc)}">${r.sonuc != null && (kd.s === 'onay' || kd.s === 'golge-onay') ? Bicim.yuzde(r.sonuc, 2) : r.sonuc != null ? '<small>alınmadı</small>' : '–'}</td></tr>`;
    }).join('');
    return `<div class="m-govde ao-karne">
      ${filtre}
      <div class="ao-kk-satir">
        ${aoKarneKutu('Simülatör tek başına', 'bütün alım sinyalleri', kr.hepsi, 'hepsi')}
        ${aoKarneKutu('Süzgeçle', 'akışın onayladıkları', kr.onaylanan, 'onay')}
        ${aoKarneKutu('Süzgecin kaçındığı', 'akışın reddettikleri', kr.reddedilen, 'red')}
      </div>
      <p class="ao-hukum">${esc(kr.hukum)}${kr.bekleyen ? ` <span>(${kr.bekleyen} sinyalin sonucu henüz belli değil.)</span>` : ''}</p>
      <p class="m-not">Hedef: 100 işlemden en az 51'i kazansın (çizgi). Sonuçlar simülatörün kendi alış-satış fiyatıyla ve simülatörün masrafıyla hesaplanır; reddedilenler "alınsaydı ne olurdu"dur. Gölge kararlar (onay kapalıyken ya da kumbara kenardayken) de sayılır.</p>
      <h4>Eşik böyle olsaydı</h4>
      <p class="m-not">Sinyal anındaki puana göre (engelli olanlar hariç). Hangi eşikte kazanma oranı ve ortalama en iyi? Az sinyalle kurulan eşik şanstır; sinyal sayısına da bak.</p>
      <table class="ao-tablo"><thead><tr><th>Eşik</th><th>Sinyal</th><th>Kazanma</th><th>Ortalama</th><th>Toplam</th></tr></thead><tbody>${esikTablo}</tbody></table>
      <h4>Hangi bölüm işe yarıyor?</h4>
      <p class="m-not">Bölüm artıyken alınan sinyaller kazanıp eksiyken alınanlar kaybediyorsa o bölüm iyi ayırıyor demektir: ağırlığını artır. İkisi aynıysa ağırlığını azalt.</p>
      <table class="ao-tablo"><thead><tr><th>Bölüm</th><th>artıyken (≥ +10): sinyal · kazanma · ort.</th><th>eksiyken (≤ −10)</th></tr></thead><tbody>${blokTablo}</tbody></table>
      <h4>Son kararlar</h4>
      ${kayitlar ? `<div class="ao-tablo-kaydir"><table class="ao-tablo kayit"><thead><tr><th>Zaman</th><th>Strateji</th><th>Karar</th><th>Puan</th><th>Simülatör</th><th>Süzgeçle</th></tr></thead><tbody>${kayitlar}</tbody></table></div>` : '<p>Henüz kayıt yok.</p>'}
    </div>
    <div class="m-alt"><span class="m-not">Kayıtlar kayitlar/akis_karne.jsonl'de de saklanır.</span><span class="bosluk"></span><button class="btn" data-m="aoAyarlar">Ayarlar…</button><button class="btn btn-ana" data-m="kapat">Kapat</button></div>`;
  }
  function aoNedirHTML() {
    return `<div class="m-govde ao-nedir">
      <p>Simülatörün motoru <b>“al”</b> dediğinde Al-Sat dört ayrı gözle bakar ve tek soru sorar: <b>“Satış bitti mi, alıcı geldi mi?”</b> (motor bir kırılımda alıyorsa: <b>“Alıcılar gerçek mi?”</b>). Her bölüm −100 ile +100 arası puan verir; maddeleri kartta tek tek görünür.</p>
      <ol class="ao-nedir-liste">
        <li><b>Yapı (Smart Money Concepts):</b> mumlardan. Tepe/dipler (HH yükselen tepe, HL yükselen dip, LH alçalan tepe, LL alçalan dip), <b>BOS</b> (yapı kırılımı: trend devam), <b>CHoCH</b> (karakter değişimi: trend döndü), talep/arz bölgeleri (büyük alıcının/satıcının girdiği son mum), fiyat boşlukları (FVG), eşit dipler (altında stoplar birikir) ve <b>süpürme</b> (fitil eski dibin altına inip stopları alır, kapanış geri üstte: çoğu zaman dönüş işareti). İndirim bölgesi: son büyük dalganın alt yarısı.</li>
        <li><b>Emir akışı (Order Flow):</b> MEXC'nin her işleminden. <b>Delta</b> = saldırgan alım − saldırgan satış. <b>Emilim</b>: dipte çok satış geldi ama fiyat düşmedi (biri hepsini aldı). <b>Dengesizlik</b>: bir fiyat kademesinde alım, bir alttaki satışın 3 katı; 3 kademe üst üste olursa güçlü alıcı. Büyük işlemler, şu anki saldırı ve defter.</li>
        <li><b>CVD:</b> deltanın birikimi. Fiyat yeni dip yapıp CVD yapmazsa satıcılar zayıflıyor demektir (uyumsuzluk). Spot ile vadeli ayrı: hareketi spot alıcılar mı, kaldıraçlı vadeli mi sürüklüyor?</li>
        <li><b>Açık pozisyon (Open Interest):</b> vadelide açık duran pozisyonlar. Düşüşte azaldıysa pozisyonlar kapanmış, zorunlu satış bitmiş olabilir; arttıysa yeni açığa satışlar geliyor. Fonlama yüksekse long'lar kalabalık. Sadece MEXC'de vadelisi olan coinlerde.</li>
      </ol>
      <p><b>Grafikte:</b> yeşil/kırmızı kutular talep/arz bölgeleri, mavi/mor kesik kutular FVG, çizgiler BOS/CHoCH, sarı üçgen süpürme, <b>E</b> emilim, mumun yanındaki üç çizgi dengesizlik yığını, daireler büyük işlemler, mumdaki ince yatay çizgi o mumda en çok işlem gören fiyat (POC). Motorun alım sinyalinin altındaki etiket akışın kararıdır (✓ onay, ✕ red, ⏳ bekliyor; kesik kenarlı = gölge). Alttaki panel: delta (her mum), CVD (spot düz, vadeli kesik), açık pozisyon (sarı). Bir muma tıklarsan o mumun fiyat kademelerindeki alım-satımı (footprint) açılır.</p>
      <p class="m-not">Veri program açıkken toplanır (MEXC websocket); program kapalıyken kaçan son 2–6 saat işlem verisi açılınca geri doldurulur, açık pozisyonun geçmişi MEXC'de olmadığı için o sadece program açıkken birikir. Bu sistemin kâr getirdiği kanıtlanmış değildir; karne ne kadar işe yaradığını gösterir.</p>
    </div>
    <div class="m-alt"><span class="bosluk"></span><button class="btn btn-ana" data-m="kapat">Tamam</button></div>`;
  }
  async function aoKarneGetir() {
    const kb = seciliKumbara();
    const q = new URLSearchParams();
    if (AO.filtre.kumbara && kb) q.set('kumbara', kb.id);
    if (AO.filtre.buAyarla) q.set('buAyarla', '1');
    try {
      const r = await fetch('/api/akis/karne?' + q.toString());
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      AO.karne = j; AO.karneHata = '';
    } catch (e) { AO.karne = null; AO.karneHata = 'Karne alınamadı: ' + hataYazisi(e); }
  }
  async function aoPencereAc(sekme2) {
    AO.sekme = sekme2 || 'karne';
    const baslik = { karne: 'Akış onayı — karne', ayarlar: 'Akış onayı — ayarlar', nedir: 'Akış onayı nasıl çalışır?' }[AO.sekme];
    const sekmeler = `<div class="sekmeler ao-sekmeler">${[['karne', 'Karne'], ['ayarlar', 'Ayarlar'], ['nedir', 'Nasıl çalışır?']].map(([id, ad]) => `<button data-m="aoSekme" data-sekme="${id}" aria-pressed="${AO.sekme === id}">${ad}</button>`).join('')}</div>`;
    modalAc(`<div class="m-ust"><h3>${baslik}</h3>${sekmeler}<button class="kapat" data-m="kapat" aria-label="Kapat">×</button></div><div id="aoIcerik"></div>`, 'akis');
    aoPencereYaz();
    if (AO.sekme === 'karne') { AO.karne = null; aoPencereYaz(); await aoKarneGetir(); }
    else if (AO.sekme === 'ayarlar') await aoAyarGetir();
    if (modalAcik === 'akis') aoPencereYaz();
  }
  function aoPencereYaz() {
    const el = $('#aoIcerik');
    if (!el) return;
    el.innerHTML = AO.sekme === 'ayarlar' ? aoAyarHTML() : AO.sekme === 'nedir' ? aoNedirHTML() : aoKarneHTML();
  }
  async function aoKaydet(varsayilan) {
    if (modalMesgul || !AO.ayar) return;
    const a = JSON.parse(JSON.stringify(AO.ayar));
    if (!varsayilan) {
      for (const el of document.querySelectorAll('#aoIcerik [data-ao-alan]')) {
        const id = el.dataset.aoAlan;
        if (el.type === 'checkbox') a[id] = el.checked;
        else {
          const x = parseFloat(String(el.value).replace(',', '.'));
          if (!isFinite(x)) { modalHata('Bütün kutulara sayı yaz.'); return; }
          a[id] = x;
        }
      }
      for (const el of document.querySelectorAll('#aoIcerik [data-ao-agirlik]')) {
        const x = parseFloat(String(el.value).replace(',', '.'));
        if (!isFinite(x)) { modalHata('Bütün kutulara sayı yaz.'); return; }
        a.agirlik[+el.dataset.aoAgirlik] = x;
      }
      if (a.agirlik.every(x => x <= 0)) { modalHata('En az bir bölümün ağırlığı 0\'dan büyük olmalı.'); return; }
      const g = $('#aoGenelAcik');
      if (g) a.acik = g.checked;
    }
    modalMesgul = true;
    try {
      const r = await fetch('/api/akis/ayar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, ayar: a, varsayilan: !!varsayilan }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      AO.ayar = j.ayar; AO.varsayilan = j.varsayilan;
      modalMesgul = false;
      aoCip();
      if (varsayilan) { aoPencereYaz(); bildir('Akış onayı ayarları varsayılana döndü.'); }
      else { modalKapat(); bildir('Akış onayı ayarları kaydedildi.', 'al'); }
      aoYenile(true);
      durumYenile(true);
    } catch (e) { modalMesgul = false; modalHata(hataYazisi(e)); }
  }
  async function aoGenelAc(acik) {
    if (!AO.ayar) return;
    const a = Object.assign({}, AO.ayar, { acik });
    try {
      const r = await fetch('/api/akis/ayar', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, ayar: a }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      AO.ayar = j.ayar;
      aoCip();
      if (modalAcik === 'akis' && AO.sekme === 'ayarlar') {
        const b = document.querySelector('.ao-genel b');
        if (b) b.textContent = acik ? 'Açık: kumbaraların alımları akış onayı bekler' : 'Kapalı: alımlar simülatör gibi hemen yapılır (sistemin kararı gölge olarak kaydedilir)';
      }
      bildir(acik ? 'Akış onayı açıldı: alımlar onay bekleyecek.' : 'Akış onayı kapatıldı: alımlar simülatör gibi hemen yapılacak.', 'dikkat');
      durumYenile(true);
    } catch (e) { bildir('Akış onayı değiştirilemedi: ' + hataYazisi(e), 'dikkat'); aoAyarGetir(); }
  }
  async function aoKumbaraAc(acik) {
    const kb = seciliKumbara();
    if (!kb) return;
    try {
      const r = await fetch('/api/kumbara/akis', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ oturum, id: kb.id, kapali: !acik }) });
      const j = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(j.hata || 'HTTP ' + r.status);
      await durumYenile(true);
      aoKartCiz();
    } catch (e) { bildir('Değiştirilemedi: ' + hataYazisi(e), 'dikkat'); aoKartCiz(); }
  }

  // ---------- footprint (tıklanan mum) ----------
  function aoFpKutu(i) {
    if (!M || i == null) return '';
    const z = M.t[i];
    if (AO.fpZ !== z) { AO.fpZ = z; AO.fp = null; aoFootprintGetir(z); }
    return `<div class="ao-fp" id="aoFp">${aoFpHTML()}</div>`;
  }
  async function aoFootprintGetir(z) {
    if (!S.sembol) return;
    try {
      const r = await fetch(`/api/akis/footprint?sembol=${encodeURIComponent(S.sembol)}&aralik=${encodeURIComponent(S.aralik)}&z=${z}`);
      const j = await r.json().catch(() => ({}));
      if (AO.fpZ !== z) return;
      AO.fp = r.ok ? j : { hata: j.hata || 'HTTP ' + r.status };
    } catch (e) { if (AO.fpZ === z) AO.fp = { hata: hataYazisi(e) }; }
    const el = $('#aoFp');
    if (el) el.innerHTML = aoFpHTML();
  }
  function aoFpHTML() {
    const fp = AO.fp;
    const bas = '<div class="ao-fp-bas"><b>Emir akışı (footprint)</b><small>her fiyat kademesinde saldırgan satış | alım</small></div>';
    if (!fp) return bas + '<p class="ao-not">Getiriliyor…</p>';
    if (fp.hata) return bas + `<p class="ao-not">${esc(fp.hata)}</p>`;
    if (!fp.var || !(fp.satirlar || []).length) return bas + '<p class="ao-not">Bu mum için işlem verisi yok (program o sırada veri toplamıyordu ya da hiç işlem olmadı).</p>';
    let enBuyuk = 0;
    for (const s of fp.satirlar) enBuyuk = Math.max(enBuyuk, s.al, s.sat);
    const satir = s => {
      const poc = Math.abs(s.fiyat - fp.poc) / fp.poc < 1e-9;
      return `<tr class="${poc ? 'poc' : ''}"><td class="sat ${s.satimDeng ? 'deng' : ''}"><i style="width:${(s.sat / Math.max(1e-12, enBuyuk)) * 100}%"></i><span>${aoSayi(s.sat)}</span></td>
        <td class="fiyat">${fg(s.fiyat)}${poc ? ' <em>POC</em>' : ''}</td>
        <td class="al ${s.alimDeng ? 'deng' : ''}"><i style="width:${(s.al / Math.max(1e-12, enBuyuk)) * 100}%"></i><span>${aoSayi(s.al)}</span></td></tr>`;
    };
    return `${bas}
      <div class="ao-fp-ozet">Delta <b class="${isaretSinif(fp.delta)}">${aoSayiIsaret(fp.delta)}</b> · hacim <b>${aoSayi(fp.hacim)}</b> · <b class="${isaretSinif(fp.oran)}">${Bicim.yuzde(fp.oran * 100, 1)}</b> net${fp.eksik ? ' · <span class="neg">veri eksik</span>' : ''}</div>
      <table class="ao-fp-tablo"><thead><tr><th>satış</th><th>fiyat</th><th>alım</th></tr></thead><tbody>${fp.satirlar.map(satir).join('')}</tbody></table>
      <p class="m-not">Kalın kenarlı hücre <b>dengesizlik</b>: alım, bir alt kademedeki satışın ${AO.ayar ? nfEn(1).format(AO.ayar.dengesizlikOran) : 3} katı (satış için bir üstteki alımın). POC: mumda en çok işlem gören fiyat.</p>`;
  }

  // Lejant için akış işaretleri
  function aoLejantParcalari() {
    if (!aoAcik() || !AO.veri) return [];
    const p = [];
    if (aoGor('bolge')) p.push(`<span><i class="ao-ornek-kutu"></i>talep / arz bölgesi</span><span><i class="ao-ornek-fvg"></i>boşluk (FVG)</span>`);
    if (aoGor('yapi')) p.push(`<span><i style="border-color:${RENK.yukari}"></i>BOS / CHoCH</span><span><b class="ao-l-sup">▲</b>süpürme</span>`);
    if (aoGor('akis')) p.push(`<span><b class="ao-l-e">E</b>emilim</span>`);
    if (aoGor('buyuk')) p.push(`<span><b class="ao-l-buyuk">●</b>büyük işlem</span>`);
    p.push(`<span><b class="pos">✓</b>/<b class="neg">✕</b> akış kararı</span>`);
    return p;
  }
  function aoGorunumYaz() {
    for (const b of document.querySelectorAll('#aoPanel [data-aog]')) b.setAttribute('aria-pressed', String(!!(S.gorunum.ao && S.gorunum.ao[b.dataset.aog])));
  }

  // 0.9: üç grafiğin fiyat ölçeği aynı genişlikte olsun (mumlar ve akış paneli alt alta hizalı kalsın)
  let olcekGenislik = 0;
  function olcekleriEsitle() {
    if (!aoGrafik || $('#aoPanel').hidden) return;
    const w = Math.max(anaGrafik.priceScale('right').width(), aoGrafik.priceScale('right').width(), paraGrafik.priceScale('right').width());
    if (w > 0 && Math.abs(w - olcekGenislik) > 0.5) {
      olcekGenislik = w;
      for (const gr of [anaGrafik, aoGrafik, paraGrafik]) gr.priceScale('right').applyOptions({ minimumWidth: Math.ceil(w) });
    }
  }
  // kumbara kartında: onay bekleniyor / hesaplanıyor
  function aoKumbaraSatiri(kb) {
    const ak = kb.akis;
    if (!ak || kb.durum === 'bitti') return '';
    if (ak.bekleme) return `<div class="kk-emir ao-bek">⏳ Simülatör aldı; akış onayı bekleniyor · <b data-geri="${ak.bekleme.bitis}">${kalanYaz(ak.bekleme.bitis - Date.now())}</b> kaldı</div>`;
    const son = (ak.kayitlar || []).slice(-1)[0];
    if (son && son.karar === 'hesaplaniyor') return '<div class="kk-emir"><span class="donen mini"></span>Simülatör aldı; akış onayı hesaplanıyor…</div>';
    return '';
  }
  function aoBaslatHTML() {
    const acik = !AO.ayar || AO.ayar.acik;
    return `<label class="onay-satir ao-baslat"><input type="checkbox" id="mAkis" ${acik ? 'checked' : 'disabled'}><span><b>Akış onayı</b> ${acik
      ? '— simülatör alınca akış (emir akışı, yapı, CVD, açık pozisyon) da onay verirse alınsın. İşaretsizse simülatör gibi hemen alınır; akışın kararı yine gölge olarak kaydedilir (karnede görünür).'
      : '— genel ayarda kapalı (üstteki "Akış onayı" çipinden açılır). Bu kumbara simülatör gibi alır; akışın kararı gölge olarak kaydedilir.'}</span></label>`;
  }
  // simülatörün işlemleri tablosu: her motor alımına akışın kararı
  function aoKayitHaritasi() {
    const m = new Map();
    const v = AO.veri;
    if (v && v.kararlar && v.sembol === S.sembol && v.aralik === S.aralik) for (const r of v.kararlar) m.set(r.motorMum, r);
    return m;
  }
  const aoTabloImza = () => (AO.veri && AO.veri.kararlar ? AO.veri.kararlar.map(r => r.motorMum + r.karar + r.toplam).join(',') : '');
  function aoTabloHucre(r) {
    if (!r) return '<td class="sebep">–</td>';
    const kd = aoKarar(r.karar);
    return `<td class="ao-hucre ${kd.s}" title="${esc(kd.ad + (r.ozet ? ' · ' + r.ozet : ''))}">${kd.i} ${r.karar === 'hesaplaniyor' ? '' : aoPuan(r.toplam)}<small>${esc(kd.ad)}</small></td>`;
  }

  // ---------- olaylar ----------
  function segAyarla() {
    for (const b of document.querySelectorAll('#gorunum button')) b.setAttribute('aria-pressed', String(!!S.gorunum[b.dataset.g]));
  }

  function olaylariBagla() {
    $('#tekrarDene').addEventListener('click', () => {
      if (!AKTIF || !AKTIF.iz) return;
      hataGoster(null);
      if (AKTIF.kumbaraId) { yukleniyor(true, 'Veri getiriliyor…'); AKTIF.iz.yukleVeBildir(); } else onizlemeYukle(AKTIF.iz);
    });
    $('#bastanYukle').addEventListener('click', () => {
      if (!AKTIF || !AKTIF.iz || !AKTIF.strateji) return;
      const iz = AKTIF.iz;
      if (AKTIF.kumbaraId) {
        // kumbaranın başlangıcı değişmez: aynı ilk mumdan yeniden yüklenir
        if (iz.M) iz.ilkMum = iz.M.t[0];
        yukleniyor(true, 'Kumbaranın mumları baştan getiriliyor…');
        iz.yukleVeBildir();
      } else {
        yuklemeNo++;
        onizlemeYukle(iz);
      }
    });
    $('#sonaGit').addEventListener('click', () => { if (M) sagaYasla(); });
    $('#klasorDegistir').addEventListener('click', klasorDegistir);
    $('#buradaDevam').addEventListener('click', () => { oturumAc(); });
    $('#gorunum').addEventListener('click', e => {
      const b = e.target.closest('button'); if (!b) return;
      S.gorunum[b.dataset.g] = !S.gorunum[b.dataset.g];
      segAyarla(); kaydet();
      if ((b.dataset.g === 'cizgiler' || b.dataset.g === 'gostergeler') && M && R) { cizgileriKur(); seviyeImza = 'x'; goster(k); }
      if (b.dataset.g === 'akis') { aoPanelGoster(); lejantYaz(null); aoYenile(true); } // 0.9
      sonImza = '';
    });
    $('#stratejiListe').addEventListener('click', e => {
      if (e.target.closest('[data-eylem="klasorSec"]')) { klasorDegistir(); return; }
      const b = e.target.closest('.skart[data-ad]');
      if (b) stratejiSec(b.dataset.ad);
    });
    $('#kuralPaneli').addEventListener('change', e => {
      const el = e.target;
      if (!el.dataset.goz) return;
      S.gorunum.gosterge[el.dataset.goz] = el.checked;
      kaydet();
      if (M && R) { cizgileriKur(); goster(k); }
    });
    $('#sekmeler').addEventListener('click', e => {
      const b = e.target.closest('button[data-sekme]'); if (!b) return;
      sekme = b.dataset.sekme;
      tabloCiz(true);
    });
    $('#islemTablo').addEventListener('click', e => {
      const kt = e.target.closest('tr[data-kno]');
      if (kt) {
        const kb = gorunenKumbara();
        const t = kb && kb.islemler.find(x => x.no === parseInt(kt.dataset.kno, 10));
        if (t) kumbaraIslemineYaklas(t);
        return;
      }
      const tr = e.target.closest('tr[data-no]'); if (!tr || !R) return;
      const t = R.islemler[parseInt(tr.dataset.no, 10) - 1];
      incele(t.girisIdx);
      isleminAraligi(t);
    });
    $('#inceleme').addEventListener('click', e => {
      const b = e.target.closest('[data-eylem]'); if (!b) return;
      const ey = b.dataset.eylem;
      if (ey === 'kapat') incelemeKapat();
      else if (ey === 'islemeYaklas' && R) isleminAraligi(R.islemler[parseInt(b.dataset.no, 10) - 1]);
    });
    $('#kumbaraKart').addEventListener('click', e => {
      const b = e.target.closest('[data-eylem]'); if (!b || b.disabled) return;
      const ey = b.dataset.eylem;
      if (ey === 'baslatAc') baslatAc();
      else if (ey === 'yeniAlimYok') kumbaraDurdur('yeniAlimYok');
      else if (ey === 'simdiSat') kumbaraDurdur('simdiSat');
      else if (ey === 'satmadan') kumbaraDurdur('satmadan');
      else if (ey === 'korumaAyar') korumaAyarAc();
      else if (ey === 'korumaDon') korumaDon();
    });
    $('#sanalCip').addEventListener('click', sanalAc);
    $('#mexcCip').addEventListener('click', mexcAc);
    $('#uzakCip').addEventListener('click', uzakAc); // 0.8
    $('#aoCip').addEventListener('click', () => aoPencereAc('karne')); // 0.9
    $('#aoKart').addEventListener('click', e => {
      const b = e.target.closest('[data-ao]');
      if (b) { aoPencereAc(b.dataset.ao); return; }
      const r = e.target.closest('[data-ao-mum]');
      if (r && M) {
        const i = mumIndeksi(+r.dataset.aoMum);
        if (i >= 0) { anaGrafik.timeScale().setVisibleLogicalRange({ from: i - 45, to: i + 25 }); incele(i); }
      }
    });
    $('#aoKart').addEventListener('change', e => { if (e.target.id === 'aoKumbaraAcik') aoKumbaraAc(e.target.checked); });
    $('#aoPanel').addEventListener('click', e => {
      const b = e.target.closest('[data-aog]'); if (!b) return;
      S.gorunum.ao[b.dataset.aog] = !S.gorunum.ao[b.dataset.aog];
      aoGorunumYaz(); kaydet(); sonImza = ''; lejantYaz(null);
    });
    $('#hepsiniDurdur').addEventListener('click', hepsiniDurdur);
    $('#modal').addEventListener('click', e => {
      if (e.target === $('#modal')) { modalKapat(); return; }
      const t = e.target.closest('[data-tutar]');
      if (t) {
        const mod = $('#modalKutu input[name=mod]:checked');
        const b = baslatBilgi;
        let x = t.dataset.tutar === 'hepsi' ? (mod && mod.value === 'gercek' ? b.gercekSerbest : b.sanalSerbest) : +t.dataset.tutar;
        $('#mTutar').value = String(Math.floor(x * 100) / 100);
        modalHata('');
        return;
      }
      const hb = e.target.closest('[data-hassasiyet]');
      if (hb) {
        if (modalAcik === 'baslat' && baslatBilgi && baslatBilgi.koruma) { baslatBilgi.koruma.hassasiyet = hb.dataset.hassasiyet; korumaOnizle(); }
        else if (modalAcik === 'koruma') korumaAyarHassasiyet(hb.dataset.hassasiyet);
        return;
      }
      const af = e.target.closest('[data-ao-filtre]'); // 0.9: karne süzgeçleri
      if (af) {
        AO.filtre[af.dataset.aoFiltre] = af.dataset.deger === '1';
        AO.karne = null; aoPencereYaz();
        aoKarneGetir().then(() => { if (modalAcik === 'akis' && AO.sekme === 'karne') aoPencereYaz(); });
        return;
      }
      const b = e.target.closest('[data-m]'); if (!b || b.disabled) return;
      const m = b.dataset.m;
      if (m === 'kapat') modalKapat();
      else if (m === 'korumaKaydet') korumaKaydet();
      else if (m === 'korumaOto') korumaOtoDoldur();
      else if (m === 'evet') { const c = onayCozucu; onayCozucu = null; if (c) c(true); }
      else if (m === 'baslat') kumbaraBaslat();
      else if (m === 'mexc') { modalMesgul = false; mexcAc(); }
      else if (m === 'baglan') mexcBaglan();
      else if (m === 'anahtarSil') anahtarSil();
      else if (m === 'yenile') fetch('/api/gercek/yenile', { method: 'POST' }).then(r => r.text()).then(() => durumYenile(true)).catch(() => {});
      else if (m === 'sanalSifirla') sanalSifirla();
      else if (m === 'uzakYeniKod') uzakYeniKod();
      else if (m === 'tasi') tasi();
      else if (m === 'tasiVazgec') tasiVazgec();
      else if (m === 'aoSekme') aoPencereAc(b.dataset.sekme); // 0.9
      else if (m === 'aoKaydet') aoKaydet(false);
      else if (m === 'aoVarsayilan') aoKaydet(true);
      else if (m === 'aoAyarlar') aoPencereAc('ayarlar');
    });
    $('#modal').addEventListener('change', e => {
      if (e.target.name === 'mod') baslatModuAyarla();
      else if (e.target.id === 'uzAcik') uzakAcKapat(e.target.checked);
      else if (e.target.id === 'aoGenelAcik') aoGenelAc(e.target.checked); // 0.9
    });
    $('#modal').addEventListener('keydown', e => {
      if (e.key === 'Enter' && e.target.tagName === 'INPUT') {
        if (modalAcik === 'baslat') kumbaraBaslat();
        else if (modalAcik === 'mexc' && $('#mxBaglan')) mexcBaglan();
        else if (modalAcik === 'koruma') korumaKaydet();
        else if (modalAcik === 'akis' && AO.sekme === 'ayarlar') aoKaydet(false); // 0.9
      }
    });
    document.addEventListener('keydown', e => {
      if (e.key === 'Escape' && modalAcik) { modalKapat(); return; }
      const t = e.target;
      if (t && (t.tagName === 'INPUT' || t.tagName === 'SELECT' || t.tagName === 'TEXTAREA')) return;
      if (e.key === 'Escape') incelemeKapat();
      else if (e.key === 'End' && M) sagaYasla();
    });
    // Kumbara çalışırken pencere kapatılmak istenirse tarayıcı sorar (kapanırsa al-sat durur)
    window.addEventListener('beforeunload', e => {
      if (aktifKumbaralar().length && pencereAktif) { e.preventDefault(); e.returnValue = ''; }
    });
    // Pencere gerçekten kapanırken (ya da sayfa yenilenirken) programa haber ver: bekçi bununla
    // "kullanıcı kapattı" (program kapanır) ile "pencere kendiliğinden sustu" (program bekler) arasını ayırır.
    window.addEventListener('pagehide', () => {
      try { if (oturum) navigator.sendBeacon('/api/kapaniyor?oturum=' + encodeURIComponent(oturum)); } catch (e) { /* önemli değil */ }
    });
  }

  // Otomatik testler için durum okuma (ekranda bir şey değiştirmez)
  window.__alsat = {
    aralik: () => (anaGrafik ? anaGrafik.timeScale().getVisibleLogicalRange() : null),
    mum: () => (M ? M.c.length : 0),
    islemler: () => (R ? R.islemler.map(t => ({ g: t.girisIdx, c: t.cikisIdx, gf: t.giris, cf: t.cikis, k: t.cikisKural })) : []),
    son: () => (R ? { son: R.son, asama: R.log[R.son] && R.log[R.son].sonra ? R.log[R.son].sonra.asama : null } : null),
    durum: () => DURUM,
    izleyiciler: () => [...IZ.values()].map(iz => ({ id: iz.kumbaraId, dosya: iz.dosyaAdi, durum: iz.durum, mum: iz.M ? iz.M.t.length : 0, ilk: iz.M ? iz.M.t[0] : null, sonRapor: iz.sonRapor, hata: iz.hata, raporHata: iz.raporHata })),
    gorunen: () => (GZ ? { kumbaraId: GZ.kumbaraId, dosya: GZ.dosyaAdi, ilk: GZ.M ? GZ.M.t[0] : null } : null),
    akis: () => AO.veri, akisAyar: () => AO.ayar, akisKarne: () => AO.karne, // 0.9
  };

  // ---------- başlangıç ----------
  async function basla() {
    grafikleriKur();
    akisKur();
    olaylariBagla();
    requestAnimationFrame(cizimDongusu);
    await oturumAc();
    try {
      const r = await fetch('/api/ayarlar');
      ayarlariYukle(await r.json());
    } catch (e) { /* ilk açılış */ }
    segAyarla();
    seritYaz();
    await durumYenile(true);
    await listeYenile();
    uzakYenile(); // 0.8: "Bilgisayardan al" çipi
    aoAyarGetir(); aoPanelGoster(); aoGorunumYaz(); // 0.9: akış onayı
    tasimaSor(); // 0.8: eski sürümün kayıtları
    setInterval(listeYenile, 5000);
    setInterval(() => { zamanlariGuncelle(); canliKontrol(); aoYenile(); }, 1000); // 0.9: aoYenile kendisi 5 sn'de bir sorar
    setInterval(() => durumYenile(), 15000); // canlı hat koparsa diye
  }
  basla();
})();
