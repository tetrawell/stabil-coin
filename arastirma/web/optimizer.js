/*  AYAR BULUCU (3.4) — "En kârlı ayarı bul" butonunun arka planda çalışan kısmı (Web Worker).
 *
 *  Sayfa bir ya da birkaç Worker açar; her Worker bir İŞ yapar:
 *    { tur: 'tam' }                       bütün mumlarda arar (kullanıcıya önerilen ayar bu)
 *    { tur: 'kat', egitimSon, testSon }   SINAV: [0, egitimSon) mumlarında arar, ayarın hiç görmediği
 *                                         [egitimSon, testSon) mumlarında ölçer (sayfa 3 kat açar: %55, %70, %85)
 *
 *  Her iş aynı dört adımı yapar:
 *    1) ARAMA: açık kuralların ve onların göstergelerinin değerlerini dener (rastgele + en iyilerin çevresinde).
 *       Neye göre "en iyi" (ayar.hedef):
 *         'guvenilir' (varsayılan): GÜVENİLİR KÂR = kâr − şans payı. Şans payı, işlem sayısı az ya da sonuçlar
 *                     dağınıksa büyür. Böylece 5 şanslı işlemle gelen kâr, 40 tutarlı işlemin önüne geçemez.
 *         'enCok':    eski davranış (3.3): sadece son para.
 *    2) SAĞLAMLIK: en iyi birkaç ayarın değerleri biraz oynatılır; oynatınca da iyi kalan seçilir
 *       (sivri bir tepe değil, geniş bir düzlük: canlıda fiyat biraz farklı gidince çökmeyen ayar).
 *    3) SIKILAŞTIRMA (ayar.sikilastir): sonucu değiştirmeyen, boşta kalan ayarlar güvenli tarafa çekilir
 *       (ör. hiç değmeyen %11 zarar durdur, sonuç aynı kalıyorsa %6'ya) ya da düzlüğün ortasına konur.
 *    4) SEÇENEKLER (cephe): her işlem sayısı aralığı için bulunan en iyi ayar saklanır; kullanıcı
 *       "daha çok işlem, daha az kâr" seçeneklerini grafikte görüp seçebilir.
 *  Sadece değerleri dener; hangi kuralın açık olacağına kullanıcı karar verir.
 *  Motor dosyalarını (gostergeler.js, kurallar.js, motor.js) DEĞİŞTİRMEZ: Kripto Al-Sat aynı kararları verir.
 */
importScripts('gostergeler.js', 'kurallar.js', 'motor.js');

const BULUCU_SURUMU = 2; // 1: 3.3 ve öncesi (en çok kâr, tek sınav), 2: 3.4
let iptalNo = -1;

onmessage = e => {
  const d = e.data;
  if (d.tip === 'iptal') iptalNo = d.no;
  else if (d.tip === 'basla') calis(d).catch(err => postMessage({ tip: 'hata', no: d.no, mesaj: String((err && err.message) || err) }));
};

// ---------- yardımcılar ----------
function rastgeleUretec(tohum) {
  return function () {
    tohum |= 0; tohum = (tohum + 0x6d2b79f5) | 0;
    let t = Math.imul(tohum ^ (tohum >>> 15), 1 | tohum);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
function parametreler(s) {
  return Motor.ayarlanabilirler(s, KURALLAR).map(({ yol, a }) => tanim(yol, a));
}
function tanim(yol, a) {
  if (a.tip === 'secim') return { yol, tip: 'secim', secenekler: a.secenekler.map(x => x.v) };
  const [lo, hi] = a.ara || [a.min, a.max];
  return { yol, tip: 'sayi', lo, hi, min: a.min, max: a.max, adim: a.adim, tam: !!a.tam };
}
const al = (s, y) => (y[0] === 'g' ? s.gostergeler[y[1]][y[2]] : s.kurallar[y[1]].d[y[2]]);
const koy = (s, y, v) => { if (y[0] === 'g') s.gostergeler[y[1]][y[2]] = v; else s.kurallar[y[1]].d[y[2]] = v; };
const yolAnahtari = y => y.join('|');
function yuvarla(p, x) {
  x = Math.min(p.max, Math.max(p.min, x));
  if (p.tam) return Math.round(x);
  return +(Math.round(x / p.adim) * p.adim).toFixed(6);
}
function duzelt(s) {
  const e = s.gostergeler.ema;
  if (e && e.hizli >= e.yavas) { const t = e.hizli; e.hizli = e.yavas; e.yavas = t; if (e.hizli === e.yavas) e.yavas += 1; }
  const v = s.gostergeler.vites; // yatay sınırı, sert sınırından küçük olmalı
  if (v && v.sinir1 >= v.sinir2) { const t = v.sinir1; v.sinir1 = v.sinir2; v.sinir2 = t; if (v.sinir1 >= v.sinir2) v.sinir2 = +(v.sinir1 + 0.01).toFixed(2); }
}
const kopyala = s => JSON.parse(JSON.stringify(s));
const bekle = () => new Promise(r => setTimeout(r, 0));
function dilimle(m, n) {
  return { t: m.t.slice(0, n), o: m.o.slice(0, n), h: m.h.slice(0, n), l: m.l.slice(0, n), c: m.c.slice(0, n), v: m.v.slice(0, n), acikSon: false };
}

// Sonucu değiştirmeyen değerler arasında hangisi daha güvenli? (3. adım)
//   kucuk: küçük değer güvenli (zarar durdur, tepeden dönüş: kaybı/geri vermeyi daha erken keser)
//   alma:  düşüş vitesinde alış bandı: "Alma" sonucu değiştirmiyorsa o, değilse en derin bant
//   erken: düşüş vitesinde satış bandı: en alçak bant (daha erken sat)
//   (diğerleri: düzlüğün ortası)
const GUVENLI = {
  'k|zararDurdur|yuzde': 'kucuk',
  'k|tepedenDonus|yuzde': 'kucuk',
  'k|fbbAlt|sertDusus': 'alma',
  'k|fbbAlt|dusus': 'alma',
  'k|fbbUst|sertDusus': 'erken',
  'k|fbbUst|dusus': 'erken',
};

// İşlem sayısı aralıkları (seçenekler grafiği): her aralıkta bulunan en iyi ayar saklanır
const KOVALAR = [[1, 4], [5, 7], [8, 10], [11, 14], [15, 20], [21, 29], [30, 42], [43, 60], [61, 85], [86, 120], [121, 170], [171, 240], [241, 1e9]];
const kovaNo = n => { for (let i = 0; i < KOVALAR.length; i++) if (n >= KOVALAR[i][0] && n <= KOVALAR[i][1]) return i; return -1; };

// ---------- ölçüm ve puan ----------
// bas: sadece bu mumdan (dahil) sonra AÇILAN işlemler sayılır (Al-Sat gibi: başlarken açık olan işleme atlanmaz)
// Açık kalan işlem son mumun kapanışından, satış masrafı düşülerek sayılır.
function olcum(R, m, s, bas) {
  const mf = Math.max(0, s.para.masraf) / 100, n = R.n;
  let k = 0, karli = 0, top = 0, kare = 0, carpan = 1, enKotu = 0;
  for (const t of R.islemler) {
    if (t.girisIdx < bas) continue;
    const r = t.cikisIdx != null ? t.net / t.harca : (t.adet * m.c[n - 1] * (1 - mf) - t.harca) / t.harca;
    const lr = Math.log(Math.max(1e-6, 1 + r));
    k++; if (r > 0) karli++;
    top += lr; kare += lr * lr; carpan *= 1 + r;
    if (r < enKotu) enKotu = r;
  }
  return { islem: k, karli, top, kare, yuzde: (carpan - 1) * 100, enKotu: enKotu * 100, son: R.ozk[n - 1] };
}

// GÜVENİLİR KÂR: işlemlerin toplam (log) getirisi − Z × belirsizlik × √işlem.
// Belirsizlik, işlemlerin dağınıklığından hesaplanır; tek tük işlemde 3 sanal işlemlik bir önsel eklenir
// (birkaç işlem aynı çıktı diye şans payı sıfır sayılmasın). Hiç işlem yoksa 0.
const Z = 1, ONSEL = 3;
function guvenilir(o) {
  const k = o.islem;
  if (!k) return 0;
  const ort = o.top / k;
  const sd2 = k > 1 ? Math.max(0, (o.kare - k * ort * ort) / (k - 1)) : 0;
  const onsel = o.kare / k;
  const sd = Math.sqrt(((k - 1) * sd2 + ONSEL * onsel) / (k - 1 + ONSEL));
  return o.top - Z * sd * Math.sqrt(k);
}
function puanla(o, hedef, baslangic) {
  return hedef === 'enCok' ? Math.log(Math.max(1e-9, o.son / baslangic)) : guvenilir(o);
}

// ---------- 1) arama ----------
async function ara(m, s0, P, ayar, onbellek, no, ilerle, sayac0) {
  const { butce, minIslem, hedef } = ayar;
  const bas = s0.para.baslangic;
  const rnd = rastgeleUretec(20261017);
  const gauss = () => { let u = 0, v = 0; while (!u) u = rnd(); while (!v) v = rnd(); return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v); };
  let sayac = 0, enIyi = null, yedek = null;
  const ust = [], K = 6;
  const cephe = new Array(KOVALAR.length).fill(null);

  function degerlendir(s) {
    const R = Motor.calistir(m, s, KURALLAR, { hafif: true, onbellek });
    const o = olcum(R, m, s, 0);
    const aday = { s, o, puan: puanla(o, hedef, bas), islem: o.islem };
    sayac++;
    const kv = kovaNo(o.islem);
    if (kv >= 0 && (!cephe[kv] || aday.puan > cephe[kv].puan)) cephe[kv] = aday;
    if (aday.islem >= minIslem) {
      if (!enIyi || aday.puan > enIyi.puan) enIyi = aday;
      if (ust.length < K || aday.puan > ust[ust.length - 1].puan) {
        ust.push(aday); ust.sort((a, b) => b.puan - a.puan); if (ust.length > K) ust.pop();
      }
    } else if (aday.islem > 0 && (!yedek || aday.puan > yedek.puan)) yedek = aday;
    return aday;
  }

  const mevcut = degerlendir(kopyala(s0));
  const rastgeleSay = Math.floor(butce * 0.45);
  while (sayac < rastgeleSay) {
    const s = kopyala(s0);
    for (const p of P) koy(s, p.yol, p.tip === 'secim' ? p.secenekler[Math.floor(rnd() * p.secenekler.length)] : yuvarla(p, p.lo + rnd() * (p.hi - p.lo)));
    duzelt(s);
    degerlendir(s);
    if (sayac % 20 === 0) { ilerle(sayac0 + sayac, enIyi || yedek); await bekle(); if (iptalNo === no) return null; }
  }
  const yerelBas = sayac, yerelSay = butce - sayac;
  while (sayac < butce) {
    const oran = (sayac - yerelBas) / Math.max(1, yerelSay);
    const olcek = 0.22 * (1 - oran) + 0.015;
    // ebeveyn: çoğunlukla en iyilerden, bazen seçenekler grafiğinin başka bir aralığından (daha çok işlemli ayarlar da gelişsin)
    let havuz = ust.length ? ust : [enIyi || yedek || mevcut];
    if (rnd() < 0.3) { const dolu = cephe.filter(x => x && x.islem >= minIslem); if (dolu.length) havuz = dolu; }
    const ebeveyn = havuz[Math.floor(Math.pow(rnd(), 2) * havuz.length)];
    const s = kopyala(ebeveyn.s);
    const olasilik = Math.max(1 / P.length, 0.3);
    let degisti = false;
    const degistir = p => {
      if (p.tip === 'secim') {
        const i = p.secenekler.indexOf(al(s, p.yol));
        const j = rnd() < 0.6 && i >= 0 ? Math.min(p.secenekler.length - 1, Math.max(0, i + (rnd() < 0.5 ? -1 : 1))) : Math.floor(rnd() * p.secenekler.length);
        koy(s, p.yol, p.secenekler[j]);
      } else {
        let yeni = yuvarla(p, al(s, p.yol) + gauss() * olcek * (p.hi - p.lo));
        if (yeni === al(s, p.yol)) yeni = yuvarla(p, yeni + (rnd() < 0.5 ? -1 : 1) * (p.tam ? 1 : p.adim));
        koy(s, p.yol, yeni);
      }
      degisti = true;
    };
    for (const p of P) if (rnd() < olasilik) degistir(p);
    if (!degisti) degistir(P[Math.floor(rnd() * P.length)]);
    duzelt(s);
    degerlendir(s);
    if (sayac % 20 === 0) { ilerle(sayac0 + sayac, enIyi || yedek); await bekle(); if (iptalNo === no) return null; }
  }
  return { enIyi: enIyi || yedek || mevcut, islemSartiSaglandi: !!enIyi, mevcut, sayac, ust, cephe, degerlendir, rnd };
}

// ---------- 2) sağlamlık: sivri tepe değil, düzlük ----------
const SAGLAM_ADAY = 6, SAGLAM_OYNAT = 10;
async function saglamSec(arama, P, ayar, no) {
  const { rnd, degerlendir } = arama;
  const gauss = () => { let u = 0, v = 0; while (!u) u = rnd(); while (!v) v = rnd(); return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v); };
  // farklı ayarlardan oluşan en iyi adaylar
  const gorulen = new Set(), adaylar = [];
  for (const a of arama.ust) {
    const imza = JSON.stringify(P.map(p => al(a.s, p.yol)));
    if (gorulen.has(imza)) continue;
    gorulen.add(imza); adaylar.push(a);
    if (adaylar.length >= SAGLAM_ADAY) break;
  }
  if (adaylar.length < 2) return { secilen: arama.enIyi, adaylar: [] };
  let sayac = 0;
  for (const a of adaylar) {
    const puanlar = [a.puan];
    for (let j = 0; j < SAGLAM_OYNAT; j++) {
      const s = kopyala(a.s);
      for (const p of P) {
        if (p.tip === 'secim') {
          if (rnd() < 0.2) { const i = p.secenekler.indexOf(al(s, p.yol)); const k = Math.min(p.secenekler.length - 1, Math.max(0, i + (rnd() < 0.5 ? -1 : 1))); koy(s, p.yol, p.secenekler[k]); }
        } else koy(s, p.yol, yuvarla(p, al(s, p.yol) + gauss() * 0.03 * (p.hi - p.lo)));
      }
      duzelt(s);
      puanlar.push(degerlendir(s).puan);
      if (++sayac % 20 === 0) { await bekle(); if (iptalNo === no) return null; }
    }
    a.saglam = puanlar.reduce((x, y) => x + y, 0) / puanlar.length;
  }
  adaylar.sort((x, y) => y.saglam - x.saglam);
  return { secilen: adaylar[0], adaylar };
}

// ---------- 3) sıkılaştırma: boşta kalan ayarları güvenli tarafa çek ----------
// Sonuç (puan) en fazla TOL kadar kötüleşebilir; işlem sayısı en az işlem şartının altına inemez.
const TOL_TABAN = 0.0005; // %0,05 puan: "sonuç değişmiyor" demek
async function sikilastir(arama, secilen, P, ayar, no) {
  const { degerlendir } = arama;
  let s = kopyala(secilen.s);
  const ref = secilen.puan;
  const tol = Math.max(TOL_TABAN, 0.01 * Math.abs(ref));
  const uygun = a => a.puan >= ref - tol && a.islem >= ayar.minIslem;
  const rapor = [];
  let sayac = 0;
  const dene = async (yol, v) => {
    const s2 = kopyala(s); koy(s2, yol, v); duzelt(s2);
    const a = degerlendir(s2);
    if (++sayac % 20 === 0) { await bekle(); if (iptalNo === no) throw new Error('__iptal'); }
    return a;
  };
  // önce güvenli yönü olanlar (zarar durdur gibi), sonra diğerleri
  const sirali = P.slice().sort((a, b) => (GUVENLI[yolAnahtari(b.yol)] ? 1 : 0) - (GUVENLI[yolAnahtari(a.yol)] ? 1 : 0));
  for (const p of sirali) {
    const yon = GUVENLI[yolAnahtari(p.yol)] || 'orta';
    const simdiki = al(s, p.yol);
    if (p.tip === 'secim') {
      const sonuc = [];
      for (const v of p.secenekler) sonuc.push({ v, ok: v === simdiki ? true : uygun(await dene(p.yol, v)) });
      const i0 = p.secenekler.indexOf(simdiki);
      if (i0 < 0) continue;
      let a = i0, b = i0;
      while (a > 0 && sonuc[a - 1].ok) a--;
      while (b < sonuc.length - 1 && sonuc[b + 1].ok) b++;
      let hedef = simdiki;
      const YOK = typeof VITES_YOK !== 'undefined' ? VITES_YOK : 9;
      if (yon === 'alma') {
        if (sonuc.some(x => x.v === YOK && x.ok)) hedef = YOK;
        else { const sayilar = sonuc.slice(a, b + 1).map(x => x.v).filter(v => v !== YOK); if (sayilar.length) hedef = Math.min(...sayilar); }
      } else if (yon === 'erken') {
        const sayilar = sonuc.slice(a, b + 1).map(x => x.v).filter(v => v !== YOK);
        if (sayilar.length) hedef = Math.min(...sayilar);
      } else if (b > a) {
        hedef = p.secenekler[Math.round((a + b) / 2)];
      }
      if (hedef !== simdiki) {
        const kontrol = await dene(p.yol, hedef);
        if (uygun(kontrol)) {
          koy(s, p.yol, hedef); duzelt(s);
          rapor.push({ yol: p.yol, eski: simdiki, yeni: hedef, secenekler: sonuc.filter(x => x.ok).map(x => x.v), yon });
        }
      }
      continue;
    }
    // sayısal ayar: aralığı 20 parçaya bölüp dene, şimdiki değeri içeren "sonucun değişmediği" bölgeyi bul
    const lo = Math.min(p.lo, simdiki), hi = Math.max(p.hi, simdiki);
    if (!(hi > lo)) continue;
    const izgara = [];
    for (let i = 0; i <= 20; i++) izgara.push(yuvarla(p, lo + ((hi - lo) * i) / 20));
    izgara.push(simdiki);
    const degerler = [...new Set(izgara)].sort((x, y) => x - y);
    const ok = new Map([[simdiki, true]]);
    for (const v of degerler) if (!ok.has(v)) ok.set(v, uygun(await dene(p.yol, v)));
    const i0 = degerler.indexOf(simdiki);
    let a = i0, b = i0;
    while (a > 0 && ok.get(degerler[a - 1])) a--;
    while (b < degerler.length - 1 && ok.get(degerler[b + 1])) b++;
    let alt = degerler[a], ust = degerler[b];
    // kenarları inceltir: uygun olmayan son değer ile uygun ilk değer arasında yarıya bölerek
    const incelt = async (kotu, iyi) => {
      for (let j = 0; j < 6; j++) {
        const orta = yuvarla(p, (kotu + iyi) / 2);
        if (orta === kotu || orta === iyi) break;
        if (uygun(await dene(p.yol, orta))) iyi = orta; else kotu = orta;
      }
      return iyi;
    };
    if (a > 0) alt = await incelt(degerler[a - 1], alt);
    if (yon === 'orta' && b < degerler.length - 1) ust = await incelt(degerler[b + 1], ust);
    let hedef = simdiki;
    if (yon === 'kucuk') {
      // en küçük uygun değerin hemen kenarında durma: %5 pay bırak (canlıda fiyat biraz farklı gidince takılmasın)
      hedef = Math.min(simdiki, yuvarla(p, Math.max(alt / 0.95, alt + (p.tam ? 1 : p.adim))));
      if (hedef < alt) hedef = alt;
    } else if (ust > alt) {
      hedef = yuvarla(p, (alt + ust) / 2);
    }
    if (hedef !== simdiki) {
      const kontrol = await dene(p.yol, hedef);
      if (uygun(kontrol)) {
        koy(s, p.yol, hedef); duzelt(s);
        rapor.push({ yol: p.yol, eski: simdiki, yeni: hedef, alt, ust, yon, kenar: { alt: a === 0 && alt <= lo, ust: b === degerler.length - 1 } });
      }
    }
  }
  const son = degerlendir(kopyala(s));
  return { secilen: son.puan >= ref - tol ? son : secilen, rapor: son.puan >= ref - tol ? rapor : [], sayac };
}

// ---------- bütün adımlar (arama → sağlamlık → sıkılaştırma) ----------
// Sayfadaki Worker da, araştırma betikleri de bunu çağırır (aynı kod, aynı sonuç).
// Döner: null (denenecek ayar yok) | 'iptal' | { P, arama, secim, sik, secilen, denenen }
async function boruHatti(m, s0, ayar, no, ilerle) {
  const P = parametreler(s0);
  if (!P.length) return null;
  const arama = await ara(m, s0, P, ayar, new Map(), no, ilerle, 0);
  if (!arama) return 'iptal';
  let yapilan = arama.sayac;
  let secim = { secilen: arama.enIyi, adaylar: [] };
  if (arama.islemSartiSaglandi) {
    const r = await saglamSec(arama, P, ayar, no);
    if (!r) return 'iptal';
    secim = r;
  }
  yapilan += SAGLAM_ADAY * SAGLAM_OYNAT;
  ilerle(yapilan, secim.secilen);
  let sik = { secilen: secim.secilen, rapor: [], sayac: 0 };
  if (ayar.sikilastir && arama.islemSartiSaglandi) {
    try { sik = await sikilastir(arama, secim.secilen, P, ayar, no); } catch (e) { if (String(e.message) === '__iptal') return 'iptal'; throw e; }
  }
  const denenen = arama.sayac + (secim.adaylar.length ? secim.adaylar.length * SAGLAM_OYNAT : 0) + sik.sayac;
  return { P, arama, secim, sik, secilen: sik.secilen, denenen };
}
const toplamAdim = (ayar, P) => ayar.butce + SAGLAM_ADAY * SAGLAM_OYNAT + (ayar.sikilastir ? P.length * 22 + 40 : 0);

// Sınav ölçümü: s ayarı [0, testSon) mumlarında çalışır; sadece egitimSon'dan sonra AÇILAN işlemler sayılır
function testOlc(m0, s, egitimSon, testSon) {
  const mt = dilimle(m0, testSon);
  const R = Motor.calistir(mt, s, KURALLAR, { hafif: true, onbellek: new Map() });
  const o = olcum(R, mt, s, egitimSon);
  return { yuzde: o.yuzde, islem: o.islem, karli: o.karli, enKotu: o.enKotu };
}

// ---------- bir iş ----------
async function calis(msg) {
  const no = msg.no, m0 = msg.m, s0 = msg.s;
  const is = msg.is || { tur: 'tam' };
  const ayar = Object.assign({ butce: 1500, minIslem: 5, hedef: 'guvenilir', sikilastir: true }, msg.ayar || {});
  if (ayar.hedef !== 'enCok') ayar.hedef = 'guvenilir';
  const n0 = m0.c.length;
  const bas = s0.para.baslangic;
  const yzd = a => (a ? a.o.yuzde : null);

  // sınav katında arama sadece eğitim kısmını görür; en az işlem şartı veri uzunluğuyla orantılı küçülür
  const kat = is.tur === 'kat';
  const egitimSon = kat ? Math.max(30, Math.min(n0, is.egitimSon | 0)) : n0;
  const testSon = kat ? Math.max(egitimSon, Math.min(n0, is.testSon | 0)) : n0;
  const m = kat ? dilimle(m0, egitimSon) : m0;
  const aramaAyari = Object.assign({}, ayar, { minIslem: kat ? Math.max(1, Math.round(ayar.minIslem * egitimSon / n0)) : ayar.minIslem });

  const toplam = toplamAdim(ayar, parametreler(s0));
  const ilerle = (sayac, en) => postMessage({ tip: 'ilerleme', no, yapilan: Math.min(toplam, sayac), toplam, enIyi: yzd(en) });
  const b = await boruHatti(m, s0, aramaAyari, no, ilerle);
  if (b === null) { postMessage({ tip: 'bitti', no, sonuc: { bos: true, tur: is.tur } }); return; }
  if (b === 'iptal') { postMessage({ tip: 'iptal', no }); return; }
  const { P, arama, secim, sik, secilen, denenen } = b;
  ilerle(toplam, secilen);

  const ozet = a => ({ yuzde: a.o.yuzde, islem: a.o.islem, karli: a.o.karli, puan: a.puan, guvenilir: guvenilir(a.o) });
  const cephe = arama.cephe.map((a, i) => (a ? Object.assign({ kova: KOVALAR[i], s: a.s }, ozet(a)) : null));

  if (!kat) {
    postMessage({
      tip: 'bitti', no,
      sonuc: {
        surum: BULUCU_SURUMU, tur: 'tam', hedef: ayar.hedef, minIslem: ayar.minIslem, sikilastir: ayar.sikilastir,
        enIyi: secilen.s, enIyiYuzde: secilen.o.yuzde, enIyiIslem: secilen.o.islem, enIyiKarli: secilen.o.karli,
        enIyiGuvenilir: guvenilir(secilen.o), enIyiSon: (secilen.o.son / bas - 1) * 100,
        islemSartiSaglandi: arama.islemSartiSaglandi,
        mevcutYuzde: arama.mevcut.o.yuzde, mevcutIslem: arama.mevcut.o.islem, mevcutKarli: arama.mevcut.o.karli,
        mevcutGuvenilir: guvenilir(arama.mevcut.o), mevcutSon: (arama.mevcut.o.son / bas - 1) * 100,
        alTutYuzde: (m.c[n0 - 1] / m.c[0] - 1) * 100,
        denenen,
        parametreler: P.map(p => p.yol),
        sikilastirma: sik.rapor,
        saglamlik: secim.adaylar.map(a => ({ puan: a.puan, saglam: a.saglam, yuzde: a.o.yuzde, islem: a.o.islem })),
        cephe,
      },
    });
    return;
  }

  // SINAV: bulunan ayar (ve her seçenek), görmediği [egitimSon, testSon) mumlarında
  postMessage({
    tip: 'bitti', no,
    sonuc: {
      surum: BULUCU_SURUMU, tur: 'kat', egitimSon, testSon,
      egitimYuzde: secilen.o.yuzde, egitimIslem: secilen.o.islem,
      bulunan: testOlc(m0, secilen.s, egitimSon, testSon),
      mevcut: testOlc(m0, s0, egitimSon, testSon),
      alTut: (m0.c[testSon - 1] / m0.c[egitimSon - 1] - 1) * 100,
      cephe: cephe.map(c => (c ? { kova: c.kova, egitimYuzde: c.yuzde, egitimIslem: c.islem, test: testOlc(m0, c.s, egitimSon, testSon) } : null)),
      denenen,
    },
  });
}
