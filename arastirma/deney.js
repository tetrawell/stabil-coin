// Kripto Simülatör araştırması: "En kârlı ayarı bul" ayarın GÖRMEDİĞİ günlerde ne yapıyor?
// Simülatörün kendi motoru (web/gostergeler.js, kurallar.js, motor.js) ve iki bulucusu kullanılır:
//   eski: 3.3'teki optimizer.js (en çok kâr, en az 5 işlem)      → web/optimizer_eski.js
//   yeni: 3.4'teki optimizer.js (güvenilir kâr + sağlamlık + sıkılaştırma)
// Her pencerede ayar sadece geçmiş mumlarda aranır, hemen sonraki ve aramanın hiç görmediği dönemde ölçülür
// (Al-Sat gibi: ölçüm başlarken açık olan işleme atlanmaz, sadece sonra açılanlar sayılır).
//
// node deney.js <deney> <parça no> <parça sayısı> <veri klasörü> <çıkış klasörü> [coin sınırı]
//   deney: d1m | d5m | d15m | kucuk (yerel deneme)
'use strict';
const fs = require('fs'), path = require('path'), vm = require('vm'), zlib = require('zlib'), os = require('os');
const { Worker, isMainThread, parentPort, workerData } = require('worker_threads');
const stabil = require('./stabil.js');

const WEB = path.join(__dirname, 'web');
const MS = { '1m': 60e3, '5m': 300e3, '15m': 900e3, '1h': 3600e3, '4h': 14400e3, '1d': 86400e3 };
// Kullanıcının kural seçimi (kumbaralarındaki dosyalarda açık olanlar), masraf %0,05 (dosyalarındaki değer)
const SENIN = ['fbbAlt', 'ortalamaAlti', 'diptenDonus', 'ortalamaUstu', 'zararDurdur', 'fbbUst', 'tepedenDonus'];

const DENEYLER = {
  // 1 dk: her gün (1.440 mum) için önceki 5.000 / 20.000 mumda ara, o günü ölç
  d1m: { aralik: '1m', test: 1440, adim: 1440, gun: 26, egitim: [5000, 20000], coin: 80, cesit: ['eski', 'yeni', 'yeniSikisiz', 'yeniEnCok'], uzunCesit: ['eski', 'yeni'] },
  // 5 dk: 5.000 mumda (17 gün) ara, sonraki 3 günü (Al-Sat'ta 5 dk dosyasının ömrü) ölç
  d5m: { aralik: '5m', test: 864, adim: 864, gun: 50, egitim: [5000], coin: 60, cesit: ['eski', 'yeni'] },
  // 15 dk: 5.000 mumda (52 gün) ara, sonraki 1 haftayı ölç
  d15m: { aralik: '15m', test: 672, adim: 672, gun: 40, egitim: [5000], coin: 60, cesit: ['eski', 'yeni'] },
  // yerel deneme
  kucuk: { aralik: '1m', test: 1440, adim: 1440, gun: 2, egitim: [5000], coin: 2, cesit: ['eski', 'yeni'] },
};

function mumOku(veri, aralik, sembol) {
  const yol = path.join(veri, aralik, sembol + '.csv.gz');
  const satirlar = zlib.gunzipSync(fs.readFileSync(yol)).toString().trim().split('\n');
  const m = { t: [], o: [], h: [], l: [], c: [], v: [], acikSon: false };
  for (const s of satirlar) {
    const p = s.split(',');
    m.t.push(+p[0]); m.o.push(+p[1]); m.h.push(+p[2]); m.l.push(+p[3]); m.c.push(+p[4]); m.v.push(+p[5]);
  }
  // son mum kapanmamış olabilir: at
  for (const k of ['t', 'o', 'h', 'l', 'c', 'v']) m[k].pop();
  return m;
}
const dilim = (m, a, b) => ({ t: m.t.slice(a, b), o: m.o.slice(a, b), h: m.h.slice(a, b), l: m.l.slice(a, b), c: m.c.slice(a, b), v: m.v.slice(a, b), acikSon: false });

// ---------------- işçi ----------------
function baglam(optimizerDosyasi) {
  const ctx = { console, setTimeout, Math, JSON, Date, Map, Set, Promise, Array, Object, Number, String, isFinite, Infinity, NaN };
  ctx.globalThis = ctx;
  ctx.postMessage = () => {};
  vm.createContext(ctx);
  ctx.importScripts = (...d) => d.forEach(f => vm.runInContext(fs.readFileSync(path.join(WEB, f), 'utf8'), ctx));
  vm.runInContext(fs.readFileSync(path.join(WEB, optimizerDosyasi), 'utf8'), ctx);
  return ctx;
}

function sablon(ctx) {
  const s = ctx.VARSAYILAN_STRATEJI();
  s.para.masraf = 0.05;
  for (const k of Object.keys(s.kurallar)) s.kurallar[k].acik = SENIN.includes(k);
  return JSON.parse(JSON.stringify(s));
}

// ayarı [egBas, testSon) mumlarında çalıştırır; testBas'tan sonra AÇILAN işlemleri ölçer
function olc(ctx, m, s, egBas, testBas, testSon) {
  const mm = dilim(m, egBas, testSon);
  const R = ctx.Motor.calistir(mm, s, ctx.KURALLAR, { hafif: true });
  const bas = testBas - egBas, n = R.n, mf = s.para.masraf / 100;
  let carpan = 1, k = 0, karli = 0, icerde = 0;
  const islemler = [];
  for (const t of R.islemler) {
    if (t.girisIdx < bas) continue;
    const acik = t.cikisIdx == null;
    const r = acik ? (t.adet * mm.c[n - 1] * (1 - mf) - t.harca) / t.harca : t.net / t.harca;
    carpan *= 1 + r; k++; if (r > 0) karli++;
    icerde += (acik ? n - 1 : t.cikisIdx) - t.girisIdx;
    islemler.push([mm.t[t.girisIdx], +(r * 100).toFixed(4), acik ? 1 : 0, t.cikisKural || '']);
  }
  return { yuzde: (carpan - 1) * 100, islem: k, karli, icerde: icerde / Math.max(1, n - bas), islemler };
}
function egitimOlc(ctx, m, s) {
  const R = ctx.Motor.calistir(m, s, ctx.KURALLAR, { hafif: true });
  return { yuzde: (R.ozk[R.n - 1] / s.para.baslangic - 1) * 100, islem: R.islemler.length, karli: R.islemler.filter(t => t.net > 0).length };
}
const anahtarAyarlar = s => ({
  zd: s.kurallar.zararDurdur.d.yuzde, td: s.kurallar.tepedenDonus.d.yuzde, dd: s.kurallar.diptenDonus.d.yuzde,
  fu: s.gostergeler.fbb.uzunluk, oa: s.kurallar.ortalamaAlti.d.yuzde,
  va: ['sertDusus', 'dusus', 'yatay', 'yukselis', 'sertYukselis'].map(k => s.kurallar.fbbAlt.d[k]),
});

async function pencere(ctxE, ctxY, m, mDiger, is, D) {
  const out = { kod: is.kod, aralik: D.aralik, testBas: m.t[is.testBas], testSon: m.t[is.testSon - 1], cesitler: {} };
  out.alTut = (m.c[is.testSon - 1] / m.c[is.testBas - 1] - 1) * 100;
  out.fiyat = m.c[is.testBas - 1];
  // stabil coin hesabı (uygulamanın yaptığı gibi, ölçümün başladığı anda, o ana kadarki mumlarla)
  out.stabil = {};
  for (const [ar, adet] of [['1m', 1500], ['5m', 1500], ['15m', 1000], ['1h', 1000]]) {
    const md = mDiger[ar];
    if (!md) continue;
    let son = md.t.length;
    const an = m.t[is.testBas];
    while (son > 0 && md.t[son - 1] + MS[ar] > an) son--; // sadece kapanmış mumlar
    if (son < 60) continue;
    const mumlar = [];
    for (let i = Math.max(0, son - adet); i < son; i++) mumlar.push({ zaman: md.t[i], acilis: md.o[i], yuksek: md.h[i], dusuk: md.l[i], kapanis: md.c[i] });
    const a = stabil.analiz(MS[ar], mumlar, 1);
    out.stabil[ar] = { u: a.uygun ? 1 : 0, p: a.puan, d: +(a.dalgaYuzde || 0).toFixed(2), s: Math.round((a.sureMs || 0) / 60000), y: a.yon };
  }
  {
    // aramadan: kullanıcının kural seçimi, varsayılan değerlerle (5.000 mumluk başlangıçtan)
    const egBas = is.testBas - D.egitim[0];
    const s0 = sablon(ctxY);
    out.varsayilan = olc(ctxY, m, s0, egBas, is.testBas, is.testSon);
  }
  for (const W of D.egitim) {
    const egBas = is.testBas - W;
    if (egBas < 0) continue;
    const mEg = dilim(m, egBas, is.testBas);
    out['egitimAlTut' + W] = (mEg.c[mEg.c.length - 1] / mEg.c[0] - 1) * 100;
    const cesitler = W === D.egitim[0] ? D.cesit : (D.uzunCesit || D.cesit);
    for (const ad of cesitler) {
      const t0 = Date.now();
      let s, ek = {};
      if (ad === 'eski') {
        const s0 = sablon(ctxE);
        const r = await ctxE.ara(mEg, s0, ctxE.parametreler(s0), 1500, new Map(), 5, 0, () => {});
        s = r.enIyi.s; ek.sart = r.islemSartiSaglandi;
      } else {
        const s0 = sablon(ctxY);
        const ayar = { butce: 1500, minIslem: 5, hedef: ad === 'yeniEnCok' ? 'enCok' : 'guvenilir', sikilastir: ad !== 'yeniSikisiz' };
        const b = await ctxY.boruHatti(mEg, s0, ayar, 0, () => {});
        s = b.secilen.s; ek.sart = b.arama.islemSartiSaglandi;
        ek.sik = b.sik.rapor.length;
        if (ad === 'yeni') {
          // seçenekler: her işlem sayısı aralığının en iyisi görmediği dönemde
          ek.cephe = b.arama.cephe.map((a, i) => (a ? { k: i, eY: +a.o.yuzde.toFixed(3), eI: a.o.islem, t: (x => ({ y: +x.yuzde.toFixed(3), i: x.islem, k: x.karli }))(olc(ctxY, m, a.s, egBas, is.testBas, is.testSon)) } : null)).filter(Boolean);
        }
      }
      const eg = egitimOlc(ad === 'eski' ? ctxE : ctxY, mEg, s);
      const test = olc(ad === 'eski' ? ctxE : ctxY, m, s, egBas, is.testBas, is.testSon);
      out.cesitler[ad + (W === D.egitim[0] ? '' : '_' + W)] = Object.assign({ eg, test, ayar: anahtarAyarlar(s), sn: (Date.now() - t0) / 1000 }, ek);
    }
  }
  return out;
}

if (!isMainThread) {
  const ctxE = baglam('optimizer_eski.js'), ctxY = baglam('optimizer.js');
  const onbellek = {};
  const al = (kod, ar) => {
    const a = kod + '|' + ar;
    if (!(a in onbellek)) { try { onbellek[a] = mumOku(workerData.veri, ar, kod); } catch (e) { onbellek[a] = null; } }
    return onbellek[a];
  };
  parentPort.on('message', async is => {
    try {
      const D = DENEYLER[is.deney];
      const m = al(is.kod, D.aralik);
      const mDiger = {};
      for (const ar of ['1m', '5m', '15m', '1h']) mDiger[ar] = ar === D.aralik ? m : al(is.kod, ar);
      const r = await pencere(ctxE, ctxY, m, mDiger, is, D);
      parentPort.postMessage({ tamam: true, r });
    } catch (e) {
      parentPort.postMessage({ tamam: false, hata: String((e && e.stack) || e), is });
    }
  });
  return;
}

// ---------------- ana iş ----------------
(async () => {
  const [deney, parcaS, parcaSayS, veri, cikis, sinirS] = process.argv.slice(2);
  const D = DENEYLER[deney];
  if (!D) { console.error('deney yok:', deney); process.exit(1); }
  const parca = +parcaS, parcaSay = +parcaSayS;
  const hacim = JSON.parse(fs.readFileSync(path.join(veri, 'hacimler.json'), 'utf8'));
  // coinler: bu mum süresinde verisi olanlar, hacme göre; en az (eğitim + 3 test) mum
  let coinler = Object.keys(hacim).filter(s => fs.existsSync(path.join(veri, D.aralik, s + '.csv.gz')));
  coinler = coinler.slice(0, sinirS ? +sinirS : D.coin);
  const isler = [];
  for (const kod of coinler) {
    const m = mumOku(veri, D.aralik, kod);
    const n = m.t.length;
    for (let g = 1; g <= D.gun; g++) {
      const testSon = n - (g - 1) * D.adim, testBas = testSon - D.test;
      if (testBas - D.egitim[0] < 0) break;
      isler.push({ deney, kod, testBas, testSon });
    }
  }
  const benim = isler.filter((_, i) => i % parcaSay === parca);
  console.log(`${deney}: ${coinler.length} coin, ${isler.length} pencere; bu parça ${benim.length}`);
  fs.mkdirSync(cikis, { recursive: true });
  const dosya = path.join(cikis, `${deney}_${parca}.json`);
  const sonuc = [];
  const isciSay = Math.max(1, Math.min(os.cpus().length, benim.length));
  let sira = 0, biten = 0;
  const t0 = Date.now();
  await new Promise(bitir => {
    for (let w = 0; w < isciSay; w++) {
      const isci = new Worker(__filename, { workerData: { veri } });
      const ver = () => { if (sira < benim.length) isci.postMessage(benim[sira++]); else { isci.terminate(); if (++biten === isciSay) bitir(); } };
      isci.on('message', msg => {
        if (msg.tamam) sonuc.push(msg.r); else console.log('HATA', msg.hata);
        if (sonuc.length % 5 === 0) {
          fs.writeFileSync(dosya, JSON.stringify(sonuc));
          console.log(`${sonuc.length}/${benim.length} pencere, ${((Date.now() - t0) / 60000).toFixed(1)} dk`);
        }
        ver();
      });
      isci.on('error', e => { console.log('İŞÇİ HATASI', e); ver(); });
      ver();
    }
  });
  fs.writeFileSync(dosya, JSON.stringify(sonuc));
  console.log(`BİTTİ: ${sonuc.length} pencere, ${((Date.now() - t0) / 60000).toFixed(1)} dk`);
})();
