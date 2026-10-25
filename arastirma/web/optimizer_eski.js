/*  AYAR BULUCU — "En kârlı ayarı bul" butonunun arka planda çalışan kısmı.
 *  Açık kuralların ve onların kullandığı göstergelerin değerlerini dener,
 *  seçilen mum süresi ve mum sayısında en çok kazandıran ayarı bulur.
 *  Ayrıca "sınav" yapar: aynı aramayı verinin sadece ilk %70'inde yapıp,
 *  bulunan ayarın hiç görmediği son %30'da ne yaptığını ölçer (geçmişe uydurma kontrolü).
 */
importScripts('gostergeler.js', 'kurallar.js', 'motor.js');

const KT = Object.fromEntries(KURALLAR.map(k => [k.id, k]));
const GT = Object.fromEntries(GOSTERGELER.map(g => [g.id, g]));
let iptalNo = -1;

onmessage = e => {
  const d = e.data;
  if (d.tip === 'iptal') iptalNo = d.no;
  else if (d.tip === 'basla') calis(d).catch(err => postMessage({ tip: 'hata', no: d.no, mesaj: String((err && err.message) || err) }));
};

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

async function ara(m, s0, P, butce, onbellek, minIslem, no, ilerle) {
  const rnd = rastgeleUretec(20261017);
  const gauss = () => { let u = 0, v = 0; while (!u) u = rnd(); while (!v) v = rnd(); return Math.sqrt(-2 * Math.log(u)) * Math.cos(2 * Math.PI * v); };
  let sayac = 0, enIyi = null, yedek = null;
  const ust = [], K = 6;

  function degerlendir(s) {
    const R = Motor.calistir(m, s, KURALLAR, { hafif: true, onbellek });
    const aday = { s, son: R.ozk[R.n - 1], islem: R.islemler.length };
    sayac++;
    if (aday.islem >= minIslem) {
      if (!enIyi || aday.son > enIyi.son) enIyi = aday;
      if (ust.length < K || aday.son > ust[ust.length - 1].son) {
        ust.push(aday); ust.sort((a, b) => b.son - a.son); if (ust.length > K) ust.pop();
      }
    } else if (aday.islem > 0 && (!yedek || aday.son > yedek.son)) yedek = aday;
    return aday;
  }

  const mevcut = degerlendir(kopyala(s0));
  const rastgeleSay = Math.floor(butce * 0.45);
  while (sayac < rastgeleSay) {
    const s = kopyala(s0);
    for (const p of P) koy(s, p.yol, p.tip === 'secim' ? p.secenekler[Math.floor(rnd() * p.secenekler.length)] : yuvarla(p, p.lo + rnd() * (p.hi - p.lo)));
    duzelt(s);
    degerlendir(s);
    if (sayac % 20 === 0) { ilerle(sayac, enIyi || yedek); await bekle(); if (iptalNo === no) return null; }
  }
  const yerelBas = sayac, yerelSay = butce - sayac;
  while (sayac < butce) {
    const oran = (sayac - yerelBas) / Math.max(1, yerelSay);
    const olcek = 0.22 * (1 - oran) + 0.015;
    const havuz = ust.length ? ust : [enIyi || yedek || mevcut];
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
    if (sayac % 20 === 0) { ilerle(sayac, enIyi || yedek); await bekle(); if (iptalNo === no) return null; }
  }
  return { enIyi: enIyi || yedek || mevcut, islemSartiSaglandi: !!enIyi, mevcut, sayac };
}

function dilimle(m, n) {
  return { t: m.t.slice(0, n), o: m.o.slice(0, n), h: m.h.slice(0, n), l: m.l.slice(0, n), c: m.c.slice(0, n), v: m.v.slice(0, n), acikSon: false };
}

async function calis(msg) {
  const no = msg.no, m = msg.m, s0 = msg.s;
  const P = parametreler(s0);
  if (!P.length) { postMessage({ tip: 'bitti', no, sonuc: { bos: true } }); return; }
  const butce = msg.butce || 600, minIslem = msg.minIslem == null ? 5 : msg.minIslem;
  const n = m.c.length, bolme = Math.floor(n * 0.7);
  const toplam = butce * 2;
  const bas = s0.para.baslangic;
  const yzd = a => (a ? (a.son / bas - 1) * 100 : null);

  const onbellek = new Map();
  const tam = await ara(m, s0, P, butce, onbellek, minIslem, no,
    (sayac, en) => postMessage({ tip: 'ilerleme', no, yapilan: sayac, toplam, asama: 'tam', enIyi: yzd(en) }));
  if (!tam) { postMessage({ tip: 'iptal', no }); return; }

  const egitim = await ara(dilimle(m, bolme), s0, P, butce, new Map(), Math.max(1, Math.round(minIslem * 0.7)), no,
    sayac => postMessage({ tip: 'ilerleme', no, yapilan: butce + sayac, toplam, asama: 'sinav', enIyi: yzd(tam.enIyi) }));
  if (!egitim) { postMessage({ tip: 'iptal', no }); return; }

  const olc = s => {
    const R = Motor.calistir(m, s, KURALLAR, { hafif: true, onbellek });
    const b = R.ozk[bolme - 1], sn = R.ozk[n - 1];
    return { yuzde: (sn / b - 1) * 100, islem: R.islemler.filter(t => t.girisIdx >= bolme).length };
  };
  postMessage({
    tip: 'bitti', no,
    sonuc: {
      enIyi: tam.enIyi.s, enIyiYuzde: yzd(tam.enIyi), enIyiIslem: tam.enIyi.islem, islemSartiSaglandi: tam.islemSartiSaglandi,
      mevcutYuzde: yzd(tam.mevcut), mevcutIslem: tam.mevcut.islem, minIslem,
      alTutYuzde: (m.c[n - 1] / m.c[0] - 1) * 100,
      denenen: tam.sayac + egitim.sayac,
      parametreler: P.map(p => p.yol),
      sinav: {
        bolmeIdx: bolme,
        egitimYuzde: yzd(egitim.enIyi),
        bulunan: olc(egitim.enIyi.s),
        mevcut: olc(s0),
        alTut: (m.c[n - 1] / m.c[bolme - 1] - 1) * 100,
      },
    },
  });
}
