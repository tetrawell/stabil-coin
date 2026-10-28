// ARAŞTIRMA (geçici): "En kârlı ayarı bul"un seçenekler grafiğinde hangi seçenek CANLIDA (Al-Sat'ın dosya ömrü
// boyunca) daha iyi sonuç verir? Kullanıcının akışı birebir: N mum yüklenir, bulucu (Simülatör 3.6'nın optimizer.js'i,
// değiştirilmeden) tam veride + 3 sınav döneminde çalışır; sonra seçilen ayar, hemen sonraki ve hiç görülmemiş dönemde
// (Al-Sat'taki ömrü kadar; ömür bitince açık işlem en geç ömrün dörtte biri kadar daha beklenir) ölçülür.
// Ayrıca "zararı da gören" puan (güvenilir kâr + log(1 − en büyük düşüş)) ile ayrı bir arama yapılır.
//
// node secim.js <deney> <parça> <parça sayısı> <veri> <çıkış> [coin sınırı] [pencere sınırı]
'use strict';
const fs = require('fs'), path = require('path'), vm = require('vm'), zlib = require('zlib'), os = require('os');
const { Worker, isMainThread, parentPort, workerData } = require('worker_threads');

const WEB = path.join(__dirname, 'web');
const MS = { '1m': 60e3, '5m': 300e3, '15m': 900e3, '1h': 3600e3 };
const KATLAR = [[0.55, 0.70], [0.70, 0.85], [0.85, 1]];
// optimizer.js'teki işlem sayısı aralıklarıyla aynı (oradaki const, bağlamdan okunamıyor)
const KOVALAR = [[1, 4], [5, 7], [8, 10], [11, 14], [15, 20], [21, 29], [30, 42], [43, 60], [61, 85], [86, 120], [121, 170], [171, 240], [241, 1e9]];
const kovaNo = n => KOVALAR.findIndex(k => n >= k[0] && n <= k[1]);
const DENEYLER = {
  // N: yüklenen mum, canli: Al-Sat ömrü (mum), adim: pencereler arası, en çok pencere/coin
  s1m: { aralik: '1m', N: 3000, canli: 1440, adim: 1440, pen: 24, coin: 80, sablon: 'sade' },
  s5m: { aralik: '5m', N: 2000, canli: 864, adim: 2592, pen: 18, coin: 132, sablon: 'sade' },
  s15m: { aralik: '15m', N: 1000, canli: 672, adim: 672, pen: 22, coin: 132, sablon: 'sade' },
  t5m: { aralik: '5m', N: 2000, canli: 864, adim: 2592, pen: 18, coin: 132, sablon: 'tepe' },
  kucuk: { aralik: '5m', N: 2000, canli: 864, adim: 2592, pen: 2, coin: 2, sablon: 'sade' },
};

function mumOku(veri, aralik, sembol) {
  const s = zlib.gunzipSync(fs.readFileSync(path.join(veri, aralik, sembol + '.csv.gz'))).toString().trim().split('\n');
  const m = { t: [], o: [], h: [], l: [], c: [], v: [], acikSon: false };
  for (const x of s) {
    const p = x.split(',');
    m.t.push(+p[0]); m.o.push(+p[1]); m.h.push(+p[2]); m.l.push(+p[3]); m.c.push(+p[4]); m.v.push(+p[5]);
  }
  for (const k of ['t', 'o', 'h', 'l', 'c', 'v']) m[k].pop(); // son mum kapanmamış olabilir
  return m;
}
const dilim = (m, a, b) => ({ t: m.t.slice(a, b), o: m.o.slice(a, b), h: m.h.slice(a, b), l: m.l.slice(a, b), c: m.c.slice(a, b), v: m.v.slice(a, b), acikSon: false });

// ---------------- bulucu bağlamı ----------------
function baglam() {
  const ctx = { console, setTimeout, Math, JSON, Date, Map, Set, Promise, Array, Object, Number, String, isFinite, Infinity, NaN };
  ctx.globalThis = ctx;
  ctx.postMessage = () => {};
  vm.createContext(ctx);
  ctx.importScripts = (...d) => d.forEach(f => vm.runInContext(fs.readFileSync(path.join(WEB, f), 'utf8'), ctx));
  vm.runInContext(fs.readFileSync(path.join(WEB, 'optimizer.js'), 'utf8'), ctx);
  // ölçüme en büyük düşüş eklenir (bulucunun kendi alanları aynen); puana "zararli" hedefi eklenir. Arama, sağlamlık,
  // sıkılaştırma ve seçenekler kodu değişmez: hepsi puanla()'yı çağırır.
  const eskiOlcum = ctx.olcum;
  ctx.olcum = function (R, m, s, bas) {
    const o = eskiOlcum(R, m, s, bas);
    let tepe = 0, dd = 0;
    for (let i = Math.min(R.i0, R.n); i < R.n; i++) {
      const v = R.ozk[i];
      if (v > tepe) tepe = v;
      else if (tepe > 0 && 1 - v / tepe > dd) dd = 1 - v / tepe;
    }
    o.dd = dd;
    return o;
  };
  const guv = ctx.guvenilir;
  ctx.puanla = function (o, hedef, baslangic) {
    if (hedef === 'enCok') return Math.log(Math.max(1e-9, o.son / baslangic));
    if (hedef === 'zararli') return guv(o) + Math.log(Math.max(1e-6, 1 - (o.dd || 0)));
    return guv(o);
  };
  return ctx;
}

function sablon(ctx, ad) {
  const s = ctx.VARSAYILAN_STRATEJI();
  s.kurallar.tepedenDonus.acik = ad === 'tepe';
  s.kurallar.fbbAlt.d.vites = false; s.kurallar.fbbAlt.d.seviye = -0.764;
  s.kurallar.fbbUst.d.vites = false; s.kurallar.fbbUst.d.seviye = 0.382;
  s.kurallar.zararDurdur.d.yuzde = 6;
  s.para.masraf = 0.125;
  return JSON.parse(JSON.stringify(s));
}

// işlem listesinden ölçüler: r = işlem getirileri (oran)
function olcu(r, ekle) {
  let c = 1, tepe = 1, dd = 0, karli = 0, enKotu = 0, zTop = 0, zN = 0, top = 0, kare = 0;
  for (const x of r) {
    c *= 1 + x;
    if (c > tepe) tepe = c; else if (1 - c / tepe > dd) dd = 1 - c / tepe;
    if (x > 0) karli++; else { zTop += x; zN++; }
    if (x < enKotu) enKotu = x;
    const lr = Math.log(Math.max(1e-6, 1 + x));
    top += lr; kare += lr * lr;
  }
  return Object.assign({ yuzde: +((c - 1) * 100).toFixed(3), islem: r.length, karli, enKotu: +(enKotu * 100).toFixed(3), ortZarar: zN ? +((zTop / zN) * 100).toFixed(3) : 0, ddIslem: +(dd * 100).toFixed(3), top: +top.toFixed(6), kare: +kare.toFixed(6) }, ekle || {});
}

// ayarı [0, son) mumlarında çalıştırır; [bas, kapanis) içinde AÇILAN işlemler; açık kalan "son"un kapanışından satılır
function islemGetirileri(ctx, m, s, bas, kapanis, son) {
  const mm = son < m.c.length ? dilim(m, 0, son) : m;
  const R = ctx.Motor.calistir(mm, s, ctx.KURALLAR, { hafif: true });
  const mf = s.para.masraf / 100, n = R.n, r = [];
  // mum düzeyinde para eğrisi (sadece bu işlemlerle)
  let E = 1, tepe = 1, ddMum = 0;
  for (const t of R.islemler) {
    if (t.girisIdx < bas || t.girisIdx >= kapanis) continue;
    const cik = t.cikisIdx != null ? t.cikisIdx : n - 1;
    for (let i = t.girisIdx; i <= cik; i++) {
      const v = E * (1 - mf) * (1 - mf) * mm.c[i] / t.giris;
      if (v > tepe) tepe = v; else if (1 - v / tepe > ddMum) ddMum = 1 - v / tepe;
    }
    const x = t.cikisIdx != null ? t.net / t.harca : (t.adet * mm.c[n - 1] * (1 - mf) - t.harca) / t.harca;
    E *= 1 + x;
    if (E > tepe) tepe = E; else if (1 - E / tepe > ddMum) ddMum = 1 - E / tepe;
    r.push(x);
  }
  return { r, ddMum };
}
function canliOlc(ctx, m, s, egBas, testBas, D) {
  const kapanis = testBas + D.canli, son = Math.min(m.c.length, kapanis + Math.round(D.canli / 4));
  const mm = dilim(m, egBas, son);
  const { r, ddMum } = islemGetirileri(ctx, mm, s, testBas - egBas, kapanis - egBas, mm.c.length);
  return olcu(r, { ddMum: +(ddMum * 100).toFixed(3) });
}
function sinavOlc(ctx, mEg, s, egitimSon, testSon) {
  const { r, ddMum } = islemGetirileri(ctx, mEg, s, egitimSon, testSon, testSon);
  return olcu(r, { ddMum: +(ddMum * 100).toFixed(3) });
}
const icOzet = a => ({ yuzde: +a.o.yuzde.toFixed(3), islem: a.o.islem, karli: a.o.karli, puan: +a.puan.toFixed(5), dd: +(100 * (a.o.dd || 0)).toFixed(3), enKotu: +a.o.enKotu.toFixed(3) });
const anahtarAyar = s => ({ zd: s.kurallar.zararDurdur.d.yuzde, td: s.kurallar.tepedenDonus.acik ? s.kurallar.tepedenDonus.d.yuzde : null, dd: s.kurallar.diptenDonus.d.yuzde, fu: s.gostergeler.fbb.uzunluk, fc: s.gostergeler.fbb.carpan, fa: s.kurallar.fbbAlt.d.seviye, fs: s.kurallar.fbbUst.d.seviye });

async function pencere(ctx, m, mDiger, is, D) {
  const stabil = require('./stabil.js');
  const egBas = is.testBas - D.N;
  const mEg = dilim(m, egBas, is.testBas);
  const N = D.N;
  const out = { kod: is.kod, aralik: D.aralik, N, testBas: m.t[is.testBas], sablon: D.sablon };
  const son = Math.min(m.c.length, is.testBas + D.canli);
  out.alTut = +((m.c[son - 1] / m.c[is.testBas - 1] - 1) * 100).toFixed(3);
  out.egitimAlTut = +((mEg.c[N - 1] / mEg.c[0] - 1) * 100).toFixed(3);
  // Stabil Coin uygulamasının bu andaki hesabı (kullanıcı coini böyle seçiyor)
  out.stabil = {};
  for (const [ar, adet] of [['1m', 1500], ['5m', 1500], ['15m', 1000], ['1h', 1000]]) {
    const md = mDiger[ar];
    if (!md) continue;
    let s2 = md.t.length;
    const an = m.t[is.testBas];
    while (s2 > 0 && md.t[s2 - 1] + MS[ar] > an) s2--;
    if (s2 < 60) continue;
    const mumlar = [];
    for (let i = Math.max(0, s2 - adet); i < s2; i++) mumlar.push({ zaman: md.t[i], acilis: md.o[i], yuksek: md.h[i], dusuk: md.l[i], kapanis: md.c[i] });
    const a = stabil.analiz(MS[ar], mumlar, 1);
    out.stabil[ar] = { u: a.uygun ? 1 : 0, p: a.puan, d: +(a.dalgaYuzde || 0).toFixed(2), s: Math.round((a.sureMs || 0) / 60000), y: a.yon };
  }
  const s0 = sablon(ctx, D.sablon);
  const ayar = h => ({ butce: 1500, minIslem: 30, hedef: h, sikilastir: true, masrafTaban: 0.125 });
  out.mevcut = { canli: canliOlc(ctx, m, s0, egBas, is.testBas, D) };
  const t0 = Date.now();
  for (const hedef of ['guvenilir', 'zararli']) {
    const b = await ctx.boruHatti(mEg, s0, ayar(hedef), 0, () => {});
    const H = {
      sart: b.arama.islemSartiSaglandi,
      oneri: { ic: icOzet(b.secilen), ayar: anahtarAyar(b.secilen.s), canli: canliOlc(ctx, m, b.secilen.s, egBas, is.testBas, D) },
      cephe: b.arama.cephe.map((a, i) => (a && a.islem >= 30 ? { k: i, ic: icOzet(a), ayar: anahtarAyar(a.s), canli: canliOlc(ctx, m, a.s, egBas, is.testBas, D) } : null)).filter(Boolean),
    };
    H.oneri.kova = kovaNo(b.secilen.o.islem);
    // sınav: 3 dönem, sayfadaki gibi (aramanın gördüğü kısım büyür; ölçüm hemen sonraki görülmemiş dilimde)
    H.sinav = [];
    for (const [a1, b1] of KATLAR) {
      const egitimSon = Math.floor(N * a1), testSon = b1 >= 1 ? N : Math.floor(N * b1);
      const mk = dilim(mEg, 0, egitimSon);
      const ak = ayar(hedef);
      ak.minIslem = Math.max(1, Math.round(30 * egitimSon / N));
      const bk = await ctx.boruHatti(mk, s0, ak, 0, () => {});
      H.sinav.push({
        egitimSon, testSon,
        bulunan: sinavOlc(ctx, mEg, bk.secilen.s, egitimSon, testSon),
        mevcut: sinavOlc(ctx, mEg, s0, egitimSon, testSon),
        alTut: +((mEg.c[testSon - 1] / mEg.c[egitimSon - 1] - 1) * 100).toFixed(3),
        cephe: bk.arama.cephe.map((x, i) => (x ? { k: i, ic: icOzet(x), test: sinavOlc(ctx, mEg, x.s, egitimSon, testSon) } : null)).filter(Boolean),
      });
    }
    out[hedef] = H;
  }
  out.sn = (Date.now() - t0) / 1000;
  return out;
}

if (!isMainThread) {
  const ctx = baglam();
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
      const r = await pencere(ctx, m, mDiger, is, D);
      parentPort.postMessage({ tamam: true, r });
    } catch (e) {
      parentPort.postMessage({ tamam: false, hata: String((e && e.stack) || e), is });
    }
  });
  return;
}

(async () => {
  const [deney, parcaS, parcaSayS, veri, cikis, sinirS, penS] = process.argv.slice(2);
  const D = DENEYLER[deney];
  if (!D) { console.error('deney yok:', deney); process.exit(1); }
  const parca = +parcaS, parcaSay = +parcaSayS;
  const hacim = JSON.parse(fs.readFileSync(path.join(veri, 'hacimler.json'), 'utf8'));
  let coinler = Object.keys(hacim).filter(s => fs.existsSync(path.join(veri, D.aralik, s + '.csv.gz')));
  coinler = coinler.slice(0, sinirS ? +sinirS : D.coin);
  const penSinir = penS ? +penS : D.pen;
  const isler = [];
  for (const kod of coinler) {
    const m = mumOku(veri, D.aralik, kod);
    const n = m.t.length;
    for (let g = 0; g < penSinir; g++) {
      const testBas = n - D.canli - Math.round(D.canli / 4) - g * D.adim;
      if (testBas - D.N < 0) break;
      isler.push({ deney, kod, testBas });
    }
  }
  const benim = isler.filter((_, i) => i % parcaSay === parca);
  console.log(`${deney}: ${coinler.length} coin, ${isler.length} pencere; bu parça ${benim.length}`);
  fs.mkdirSync(cikis, { recursive: true });
  const dosya = path.join(cikis, `${deney}_${parca}.json.gz`);
  const yaz = () => fs.writeFileSync(dosya, zlib.gzipSync(JSON.stringify(sonuc)));
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
        if (sonuc.length % 10 === 0) {
          yaz();
          console.log(`${sonuc.length}/${benim.length} pencere, ${((Date.now() - t0) / 60000).toFixed(1)} dk`);
        }
        ver();
      });
      isci.on('error', e => { console.log('İŞÇİ HATASI', e); ver(); });
      ver();
    }
  });
  yaz();
  console.log(`BİTTİ: ${sonuc.length} pencere, ${((Date.now() - t0) / 60000).toFixed(1)} dk`);
})();
