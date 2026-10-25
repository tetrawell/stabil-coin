// Stabil Coin uygulamasının hesabı (app/src/main/java/com/stabilcoin/core/Analiz.kt), JavaScript'e birebir çevrildi.
// mumlar: [{zaman, acilis, yuksek, dusuk, kapanis}], enKucukDalgaYuzde: uygulamadaki "en küçük dalga" ayarı (varsayılan 1)
'use strict';
const IGNE_MUM_KATI = 6.0, IGNE_DALGA_KATI = 1.0, SICRAMA_KATI = 3.0, EN_AZ_DALGA = 5, EN_AZ_CIKIS_ORANI = 0.5, EN_AZ_PUAN = 60;
const ESIK_KATLARI = [1.0, 1.25, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0, 13.0, 16.0];

const boyYuzde = (alt, ust) => (alt <= 0 ? 0 : ((ust - alt) / alt) * 100);
function ortanca(x) {
  if (!x.length) return 0;
  const s = x.slice().sort((a, b) => a - b), o = s.length >> 1;
  return s.length % 2 === 1 ? s[o] : (s[o - 1] + s[o]) / 2;
}
function degiskenlik(x) {
  if (x.length < 2) return 1;
  const ort = x.reduce((a, b) => a + b, 0) / x.length;
  if (ort <= 0) return 1;
  const v = x.reduce((a, b) => a + (b - ort) * (b - ort), 0) / (x.length - 1);
  return Math.sqrt(v) / ort;
}
const sinirla = (x, a, b) => Math.min(b, Math.max(a, x));

function zigzag(m, bas, esikYuzde) {
  const d = [];
  if (m.length - bas < 3) return d;
  let yon = 0, tepeI = bas, tepe = m[bas].yuksek, dipI = bas, dip = m[bas].dusuk;
  for (let i = bas + 1; i < m.length; i++) {
    const h = m[i].yuksek, l = m[i].dusuk;
    if (yon === 0) {
      if (h > tepe) { tepe = h; tepeI = i; }
      if (l < dip) { dip = l; dipI = i; }
      if (boyYuzde(dip, tepe) >= esikYuzde) {
        const dipOnce = dipI < tepeI || (dipI === tepeI && m[i].kapanis >= m[i].acilis);
        if (dipOnce) { d.push({ index: dipI, fiyat: dip, tepe: false }); yon = 1; }
        else { d.push({ index: tepeI, fiyat: tepe, tepe: true }); yon = -1; }
      }
    } else if (yon === 1) {
      if (h > tepe) { tepe = h; tepeI = i; }
      else if (boyYuzde(l, tepe) >= esikYuzde) { d.push({ index: tepeI, fiyat: tepe, tepe: true }); yon = -1; dip = l; dipI = i; }
    } else {
      if (l < dip) { dip = l; dipI = i; }
      else if (boyYuzde(dip, h) >= esikYuzde) { d.push({ index: dipI, fiyat: dip, tepe: false }); yon = 1; tepe = h; tepeI = i; }
    }
  }
  return d;
}
function bacaklar(d) {
  const s = [];
  for (let k = 1; k < d.length; k++) {
    const a = d[k - 1], b = d[k];
    s.push({ basIndex: a.index, sonIndex: b.index, yukari: !a.tepe, boyYuzde: boyYuzde(Math.min(a.fiyat, b.fiyat), Math.max(a.fiyat, b.fiyat)) });
  }
  return s;
}
function bantVeEgilim(m, bas) {
  const n = m.length - bas;
  if (n < 3) return [0, 0];
  let sx = 0, sy = 0, sxx = 0, sxy = 0;
  for (let i = 0; i < n; i++) { const y = m[bas + i].kapanis; sx += i; sy += y; sxx += i * i; sxy += i * y; }
  const egim = (n * sxy - sx * sy) / (n * sxx - sx * sx);
  const kesen = (sy - egim * sx) / n;
  let enUst = -Infinity, enAlt = Infinity;
  for (let i = 0; i < n; i++) {
    const cizgi = kesen + egim * i;
    if (cizgi <= 0) continue;
    const sapma = ((m[bas + i].kapanis - cizgi) / cizgi) * 100;
    enUst = Math.max(enUst, sapma); enAlt = Math.min(enAlt, sapma);
  }
  const bant = isFinite(enUst) && isFinite(enAlt) ? enUst - enAlt : 0;
  const egilim = kesen > 0 ? ((egim * (n - 1)) / kesen) * 100 : 0;
  return [bant, egilim];
}
function degerlendir(m, mumBoylari, ortancaMum, esik, kat) {
  const n = m.length;
  let bas = 0;
  const soklar = [];
  const tumBacaklar = bacaklar(zigzag(m, 0, esik));
  if (tumBacaklar.length >= 4) {
    const normalBacak = ortanca(tumBacaklar.map(x => x.boyYuzde));
    let kesim = 0;
    for (let i = n - 1; i >= 0; i--) {
      const b = mumBoylari[i];
      if (b >= IGNE_MUM_KATI * ortancaMum && b >= IGNE_DALGA_KATI * normalBacak) { kesim = i + 1; soklar.push(i); break; }
    }
    for (let k = tumBacaklar.length - 1; k >= 0; k--) {
      const bc = tumBacaklar[k];
      if (bc.boyYuzde >= SICRAMA_KATI * normalBacak) { if (bc.sonIndex > kesim) { kesim = bc.sonIndex; soklar.push(bc.sonIndex); } break; }
    }
    bas = kesim;
  }
  const bk = bas === 0 ? tumBacaklar : bacaklar(zigzag(m, bas, esik));
  if (bas >= n - 10) return { uygun: false, puan: 0, secimPuani: 0, dalgaYuzde: 0, dalgaSayisi: 0, yon: 'YATAY', pencereBasi: bas, neden: 'Az önce şok oldu' };
  const yukari = bk.filter(x => x.yukari).map(x => x.boyYuzde), asagi = bk.filter(x => !x.yukari).map(x => x.boyYuzde);
  const dalgaSayisi = Math.min(yukari.length, asagi.length);
  if (dalgaSayisi < 2) return { uygun: false, puan: 0, secimPuani: 0, dalgaYuzde: 0, dalgaSayisi, yon: 'YATAY', pencereBasi: bas, neden: 'Bu boyda dalga yok' };
  const ortancaBacak = ortanca(bk.map(x => x.boyYuzde));
  const sonDonumIndex = bk[bk.length - 1].sonIndex;
  const sonDonumFiyat = bk[bk.length - 1].yukari ? m[sonDonumIndex].yuksek : m[sonDonumIndex].dusuk;
  const suanki = (Math.abs(m[n - 1].kapanis - sonDonumFiyat) / Math.min(m[n - 1].kapanis, sonDonumFiyat)) * 100;
  const suAnSok = suanki >= SICRAMA_KATI * ortancaBacak;
  const duzen = sinirla(1 - (degiskenlik(yukari) + degiskenlik(asagi)) / 2, 0, 1);
  const ys = bk.filter(x => x.yukari).map(x => x.sonIndex - x.basIndex), as = bk.filter(x => !x.yukari).map(x => x.sonIndex - x.basIndex);
  const ritim = sinirla(1 - (degiskenlik(ys) + degiskenlik(as)) / 2, 0, 1);
  const [bant, egilimYuzde] = bantVeEgilim(m, bas);
  const bantOrani = bant / Math.max(ortancaBacak, 1e-9);
  const kanal = sinirla(1 - (bantOrani - 1.5) / 3.5, 0, 1);
  const puan = Math.round(100 * (0.55 * duzen + 0.15 * ritim + 0.30 * kanal));
  const ortancaCikis = ortanca(yukari), ortancaInis = ortanca(asagi);
  const cikisOrani = ortancaCikis / Math.max(ortancaInis, 1e-9);
  const yon = egilimYuzde >= 0.6 * ortancaBacak ? 'YUKSELEN' : egilimYuzde <= -0.6 * ortancaBacak ? 'DUSEN' : 'YATAY';
  const neden = suAnSok ? 'Şu an sert hareket var' : dalgaSayisi < EN_AZ_DALGA ? `Az dalga (${dalgaSayisi})` : cikisOrani < EN_AZ_CIKIS_ORANI ? 'Çok hızlı düşüyor' : puan < EN_AZ_PUAN ? 'Dalgalar düzensiz' : '';
  const uygun = neden === '';
  const pencereOrani = (n - bas) / n;
  const secimPuani = puan + 20 * pencereOrani + (3 * Math.log(kat)) / Math.log(2);
  return { uygun, puan, secimPuani, dalgaYuzde: ortancaCikis, dalgaSayisi, yon, pencereBasi: bas, neden, duzen, ritim, kanal };
}
function analiz(aralikMs, mumlar, enKucukDalgaYuzde = 1) {
  if (mumlar.length < 60) return { uygun: false, puan: 0, neden: 'Yeterli veri yok', sureMs: 0 };
  const boylar = mumlar.map(x => boyYuzde(x.dusuk, x.yuksek));
  const ortancaMum = Math.max(ortanca(boylar), 1e-9);
  let enIyi = null, enIyiUygunsuz = null;
  for (const kat of ESIK_KATLARI) {
    const a = degerlendir(mumlar, boylar, ortancaMum, enKucukDalgaYuzde * kat, kat);
    if (a.uygun) { if (!enIyi || a.secimPuani > enIyi.secimPuani) enIyi = a; }
    else if (!enIyiUygunsuz || a.puan > enIyiUygunsuz.puan) enIyiUygunsuz = a;
  }
  const s = enIyi || enIyiUygunsuz;
  if (!s) return { uygun: false, puan: 0, neden: 'Dalga bulunamadı', sureMs: 0 };
  const sure = s.pencereBasi < mumlar.length ? mumlar[mumlar.length - 1].zaman - mumlar[s.pencereBasi].zaman + aralikMs : 0;
  return Object.assign({}, s, { sureMs: sure });
}
module.exports = { analiz, zigzag, bacaklar };
