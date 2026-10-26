// node test/simulator_motor_test.js  (Kripto Simülatör 3.2 motor testinin aynısı; Al-Sat'taki kopya motor üzerinde çalışır)
const fs = require('fs');
const vm = require('vm');
const path = require('path');
for (const f of ['gostergeler.js', 'kurallar.js', 'motor.js']) vm.runInThisContext(fs.readFileSync(path.join(__dirname, '..', 'web', f), 'utf8'));
const { Motor, KURALLAR, VARSAYILAN_STRATEJI, GOSTERGE_MATEMATIK: H } = globalThis;
let hata = 0;
const esit = (a, b, ad, tol = 1e-9) => {
  const ok = Math.abs(a - b) <= tol * Math.max(1, Math.abs(b));
  console.log((ok ? 'TAMAM ' : 'HATA  ') + ad + (ok ? '' : `  (beklenen ${b}, çıkan ${a})`));
  if (!ok) hata++;
};
const dogru = (x, ad) => { console.log((x ? 'TAMAM ' : 'HATA  ') + ad); if (!x) hata++; };

function mumlar(fiyatlar) {
  const m = { t: [], o: [], h: [], l: [], c: [], v: [], acikSon: false };
  fiyatlar.forEach((p, i) => {
    const [o, h, l, c, v] = Array.isArray(p) ? p : [p, p, p, p];
    m.t.push(1760000000000 + i * 900000); m.o.push(o); m.h.push(h); m.l.push(l); m.c.push(c); m.v.push(v == null ? 1 + (i % 7) : v);
  });
  return m;
}
// İlk sürümdeki kurallar: ortalama altı + dipten dönüş + tepeden dönüş + zarar durdur
function eskiStrateji() {
  const s = VARSAYILAN_STRATEJI();
  for (const k of Object.keys(s.kurallar)) s.kurallar[k].acik = false;
  for (const k of ['ortalamaAlti', 'diptenDonus', 'tepedenDonus', 'zararDurdur', 'ortalamaUstu']) s.kurallar[k].acik = true;
  s.kurallar.tepedenDonus.d.yuzde = 0.7;
  s.gostergeler.ortalama.uzunluk = 5;
  s.para.masraf = 0;
  return s;
}
const SENARYO = [100, 100, 100, 100, 100, 100,
  [100, 100, 97, 97], [97, 97, 96, 96.2], [96.2, 96.6, 96.1, 96.5], [96.5, 99, 96.4, 98.9], [98.9, 98.95, 98.2, 98.3], 98.3, 98.3];

// 1) Elle hesaplanabilen senaryo (ilk sürümle aynı sonuç)
{
  const R = Motor.calistir(mumlar(SENARYO), eskiStrateji(), KURALLAR);
  dogru(R.islemler.length === 1, 'tek işlem oldu');
  const t = R.islemler[0];
  esit(t.girisIdx, 8, 'alış 8. mumda'); esit(t.giris, 96.5, 'alış fiyatı 96.5');
  esit(t.cikisIdx, 10, 'satış 10. mumda'); esit(t.cikis, 98.3, 'satış fiyatı 98.3');
  dogru(t.cikisKural === 'tepedenDonus', 'satış sebebi: tepeden dönüş');
  dogru(R.dipler[0].bas === 6 && R.dipler[0].sonuc === 'al', 'dip arama 6. mumda başladı');
}

// 2) Masraf
{
  const s = eskiStrateji(); s.para.masraf = 0.1;
  const R = Motor.calistir(mumlar(SENARYO), s, KURALLAR);
  const t = R.islemler[0];
  esit(R.ozk[R.n - 1], 1000 * 0.999 / 96.5 * 98.3 * 0.999, 'masraflı son para doğru');
  esit(t.net, t.brut - t.girisMasraf - t.cikisMasraf, 'net = brüt − masraflar');
}

// 3) Göstergeler: elle hesapla karşılaştır
{
  const tp = [10, 11, 12, 11, 13, 14], v = [1, 2, 1, 3, 1, 2];
  const w = H.vwma(tp, v, 3);
  esit(w[2], (10 * 1 + 11 * 2 + 12 * 1) / 4, 'VWMA 3. değer');
  esit(w[5], (11 * 3 + 13 * 1 + 14 * 2) / 6, 'VWMA son değer');
  const sd = H.stdev(tp, 3);
  const naif = a => { const o = a.reduce((x, y) => x + y) / a.length; return Math.sqrt(a.reduce((x, y) => x + (y - o) ** 2, 0) / a.length); };
  esit(sd[5], naif([11, 13, 14]), 'standart sapma doğru');
  // VWAP: aynı gün içinde birikimli, gün değişince sıfırlanır
  const gun = 864e5, t0 = Math.floor(1760000000000 / gun) * gun;
  const m = { t: [t0 + 1000, t0 + 2000, t0 + 3000, t0 + gun + 1000, t0 + gun + 2000], h: [11, 12, 13, 21, 22], l: [9, 10, 11, 19, 20], c: [10, 11, 12, 20, 21], v: [1, 2, 3, 1, 1] };
  const V = H.vwap(m, 'gun');
  esit(V.vwap[2], (10 * 1 + 11 * 2 + 12 * 3) / 6, 'VWAP gün içinde birikimli');
  esit(V.vwap[3], 20, 'VWAP yeni günde sıfırlandı');
  esit(V.sapma[2], Math.sqrt((1 * (10 - 68 / 6) ** 2 + 2 * (11 - 68 / 6) ** 2 + 3 * (12 - 68 / 6) ** 2) / 6), 'VWAP sapması hacim ağırlıklı');
  dogru(V.bas[3] && !V.bas[2], 'VWAP sıfırlanma noktası işaretli');
}

// Rastgele ama tekrarlanabilir bir piyasa
function piyasa(N, tohum) {
  let x = 100; const out = [];
  const rnd = () => (tohum = (tohum * 16807) % 2147483647) / 2147483647;
  for (let i = 0; i < N; i++) {
    const o = x; x = x * (1 + (rnd() - 0.5) * 0.012 + Math.sin(i / 90) * 0.0008);
    const h = Math.max(o, x) * (1 + rnd() * 0.003), l = Math.min(o, x) * (1 - rnd() * 0.003);
    out.push([o, h, l, x, 50 + rnd() * 200]);
  }
  return out;
}

// 4) Geleceği görmüyor: varsayılan (Fib Bollinger + EMA) ve VWAP kurallarıyla
{
  const fiyat = piyasa(3000, 7);
  const deneme = (ad, s) => {
    const tam = Motor.calistir(mumlar(fiyat), s, KURALLAR);
    let ayni = true;
    for (const N of [600, 1300, 2100, 2777]) {
      const kisa = Motor.calistir(mumlar(fiyat.slice(0, N)), s, KURALLAR);
      const a = tam.islemler.filter(t => t.cikisIdx != null && t.cikisIdx < N - 1);
      const b = kisa.islemler.filter(t => t.cikisIdx != null && t.cikisIdx < N - 1);
      if (a.length !== b.length || a.some((t, j) => t.girisIdx !== b[j].girisIdx || t.cikisIdx !== b[j].cikisIdx || t.cikis !== b[j].cikis)) ayni = false;
      if (Math.abs(tam.ozk[N - 2] - kisa.ozk[N - 2]) > 1e-9) ayni = false;
    }
    dogru(ayni && tam.islemler.length > 3, `${ad}: geleceği görmüyor (${tam.islemler.length} işlem)`);
    return tam;
  };
  const sv = VARSAYILAN_STRATEJI();
  const tam = deneme('varsayılan kurallar', sv);
  const s2 = VARSAYILAN_STRATEJI();
  s2.kurallar.fbbAlt.acik = false; s2.kurallar.vwapAlti.acik = true; s2.kurallar.vwapUst.acik = true; s2.kurallar.aramaUzadi.acik = true;
  deneme('VWAP kuralları', s2);
  const s3 = VARSAYILAN_STRATEJI();
  s3.kurallar.vwapYakin.acik = true; s3.kurallar.emaAsagi.acik = true; s3.kurallar.hedefKar.acik = true; s3.kurallar.fbbUst.d.seviye = 0;
  deneme('VWAP yakını + EMA satış + hedef', s3);

  // hafif mod aynı sonucu vermeli
  const hafif = Motor.calistir(mumlar(fiyat), sv, KURALLAR, { hafif: true, onbellek: new Map() });
  dogru(hafif.islemler.length === tam.islemler.length && hafif.islemler.every((t, j) => t.girisIdx === tam.islemler[j].girisIdx && t.cikisIdx === tam.islemler[j].cikisIdx), 'hızlı mod aynı işlemleri buluyor');
  esit(hafif.ozk[hafif.n - 1], tam.ozk[tam.n - 1], 'hızlı mod aynı sonucu veriyor');
  dogru(tam.ozk0[tam.n - 1] > tam.ozk[tam.n - 1], 'masrafsız sonuç daha iyi');

  // açıklamalar o anki değerleri gösteriyor
  const ilk = tam.islemler[0];
  const donus = ilk.tetikKontrol.find(k => k.id === 'diptenDonus');
  dogru(donus && donus.not().includes('En düşük'), 'alış açıklaması üretiliyor');
  dogru(tam.islemler.every(t => t.tetikKontrol.every(k => !k.vazgec)), 'alış kontrollerine vazgeç kuralları karışmıyor');
  dogru(tam.islemler.every(t => t.tetikKontrol.some(k => k.devam && k.ok)), 'vites açıkken her alımda “dip vitesin bandına indi” onayı var');

  // Fib Bollinger satışı alış fiyatının altında olmuyor
  dogru(tam.islemler.filter(t => t.cikisKural === 'fbbUst').every(t => t.cikis >= t.giris), 'Fib bandı satışları alışın altında değil');

  // hız
  const on = new Map();
  let t0 = performance.now(); for (let r = 0; r < 20; r++) Motor.calistir(mumlar(fiyat), sv, KURALLAR); const yavas = (performance.now() - t0) / 20;
  t0 = performance.now(); for (let r = 0; r < 50; r++) Motor.calistir(mumlar(fiyat), sv, KURALLAR, { hafif: true, onbellek: on }); const hizli = (performance.now() - t0) / 50;
  console.log(`hız (3000 mum): ekran modu ≈ ${yavas.toFixed(1)} ms, ayar bulucu modu ≈ ${hizli.toFixed(1)} ms`);

  // başka program için son karar
  const k = Motor.sonKarar(fiyat.map((x, i) => [1760000000000 + i * 900000, x[0], x[1], x[2], x[3], x[4]]), sv, false);
  dogru(['bekle', 'dipAra', 'coinde'].includes(k.asama), `sonKarar çalışıyor (şu an: ${k.asama})`);
}

// 6) Vites (trend durumu)
{
  const { VITES, VITES_YOK } = globalThis;
  // önce yatay, sonra sert düşüş, sonra yatay, sonra sert yükseliş
  let x = 100, tohum = 5; const rnd = () => (tohum = (tohum * 16807) % 2147483647) / 2147483647;
  const fiyat = [], parca = [[0, 600], [-0.004, 300], [0, 400], [0.004, 300], [0, 200]];
  for (const [egilim, n] of parca) for (let i = 0; i < n; i++) { const o = x; x *= 1 + egilim + (rnd() - 0.5) * 0.006; fiyat.push([o, Math.max(o, x) * 1.001, Math.min(o, x) * 0.999, x, 100]); }
  const M = mumlar(fiyat);
  const s = VARSAYILAN_STRATEJI();
  const R = Motor.calistir(M, s, KURALLAR);
  const durum = Motor.vitesDurumlari(R);
  const say = (bas, son, f) => { let k = 0, n = 0; for (let i = bas; i < son; i++) { n++; if (f(durum[i])) k++; } return k / n; };
  dogru(say(700, 900, d => d <= 1) > 0.8, `sert düşüşte vites “düşüş/sert düşüş” diyor (%${Math.round(100 * say(700, 900, d => d <= 1))})`);
  dogru(say(700, 900, d => d === 0) > 0.4, `sert düşüşte vites çoğunlukla “sert düşüş” (%${Math.round(100 * say(700, 900, d => d === 0))})`);
  dogru(say(1400, 1600, d => d >= 3) > 0.8, `sert yükselişte vites “yükseliş/sert yükseliş” diyor (%${Math.round(100 * say(1400, 1600, d => d >= 3))})`);
  dogru(say(300, 600, d => d === 2) > 0.5, `yatayda vites çoğunlukla “yatay” (%${Math.round(100 * say(300, 600, d => d === 2))})`);
  dogru(Motor.kullanilanGostergeler(s, KURALLAR).has('vites'), 'vites açıkken vites göstergesi kullanılıyor');

  // her alımda dip, o anki vitesin alış bandına inmiş olmalı; "Alma" vitesinde alım olmamalı
  const s2 = VARSAYILAN_STRATEJI();
  s2.kurallar.fbbAlt.d.sertDusus = VITES_YOK;
  s2.kurallar.fbbAlt.d.dusus = VITES_YOK;
  const R2 = Motor.calistir(M, s2, KURALLAR);
  const d2 = Motor.vitesDurumlari(R2);
  dogru(R2.islemler.length > 0 && R2.islemler.every(t => d2[t.girisIdx] >= 2), `“Alma” seçilen düşüş vitesinde hiç alım yok (${R2.islemler.length} alımın hepsi yatay/yükselişte)`);
  const fbb = R2.b.g('fbb'), A = R2.b.ga('fbb');
  const derin = R2.islemler.every(t => {
    const sv = s2.kurallar.fbbAlt.d[VITES.ANAHTAR[d2[t.girisIdx]]];
    const bant = fbb.orta[t.girisIdx] + sv * A.carpan * fbb.std[t.girisIdx];
    return t.enDusuk <= bant + 1e-9;
  });
  dogru(derin, 'her alımda görülen dip, alım anındaki vitesin bandına inmiş');

  // satış: "Satma" seçilen viteste banttan satış olmamalı
  const s3 = VARSAYILAN_STRATEJI();
  for (const k of VITES.ANAHTAR) s3.kurallar.fbbUst.d[k] = VITES_YOK;
  const R3 = Motor.calistir(M, s3, KURALLAR);
  dogru(R3.islemler.every(t => t.cikisKural !== 'fbbUst'), 'bütün viteslerde “Satma” seçilince banttan satış olmuyor');

  // vites kapalıyken vites göstergesi hiç kullanılmıyor
  const s4 = VARSAYILAN_STRATEJI();
  s4.kurallar.fbbAlt.d.vites = false; s4.kurallar.fbbUst.d.vites = false;
  dogru(!Motor.kullanilanGostergeler(s4, KURALLAR).has('vites'), 'vites kapalıyken vites göstergesi kullanılmıyor (eski sabit davranış)');
  const R4 = Motor.calistir(M, s4, KURALLAR);
  dogru(R4.islemler.length > 0 && R4.islemler.every(t => !t.tetikKontrol.some(k => k.devam)), 'vites kapalıyken ek dip onayı yok');
}

// 7) Eski (sürüm 2) strateji dosyaları kaydedildikleri gibi, vitessiz çalışır
{
  let x = 100, tohum = 9; const rnd = () => (tohum = (tohum * 16807) % 2147483647) / 2147483647;
  const fiyat = [];
  for (let i = 0; i < 1500; i++) { const o = x; x *= 1 + (rnd() - 0.5) * 0.01 + Math.sin(i / 80) * 0.001; fiyat.push([o, Math.max(o, x) * 1.002, Math.min(o, x) * 0.998, x, 100]); }
  const M = mumlar(fiyat);
  const eski = VARSAYILAN_STRATEJI();
  eski.surum = 2; delete eski.gostergeler.vites;
  for (const id of ['fbbAlt', 'fbbUst']) for (const k of ['vites', ...globalThis.VITES.ANAHTAR]) delete eski.kurallar[id].d[k];
  const yeniKapali = VARSAYILAN_STRATEJI();
  yeniKapali.kurallar.fbbAlt.d.vites = false; yeniKapali.kurallar.fbbUst.d.vites = false;
  const a = Motor.calistir(M, eski, KURALLAR), b = Motor.calistir(M, yeniKapali, KURALLAR);
  dogru(a.islemler.length === b.islemler.length && a.islemler.every((t, j) => t.girisIdx === b.islemler[j].girisIdx && t.cikisIdx === b.islemler[j].cikisIdx),
    `sürüm 2 dosyası vitessiz (sabit bantla) çalışıyor (${a.islemler.length} işlem, vites kapalı yeni ayarla aynı)`);
  dogru(!Motor.kullanilanGostergeler(eski, KURALLAR).has('vites'), 'sürüm 2 dosyasında vites göstergesi kullanılmıyor');
}

// 5) Kapanmamış son mumla karar verilmez
{
  const m = mumlar(SENARYO.slice(0, 9));
  m.acikSon = true;
  const R = Motor.calistir(m, eskiStrateji(), KURALLAR);
  dogru(R.islemler.length === 0, 'kapanmamış mumda alım yapılmadı');
}

console.log(hata ? `\n${hata} HATA` : '\nTüm testler geçti');
process.exit(hata ? 1 : 0);
