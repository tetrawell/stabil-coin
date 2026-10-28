/*  GÖSTERGELER — kuralların baktığı çizgiler (ortalama, EMA, Fibonacci Bollinger, VWAP)
 *  ------------------------------------------------------------
 *  Her gösterge:
 *    id, ad, aciklama(a), ayarlar [{ k, etiket, min, max, adim, deger, tam?, ara? } | { k, tip:'secim', secenekler, deger }]
 *    isinma(a)         kaç mum geriye veri gerektiği (bot o kadar mum bekler)
 *    anahtar(a)        (isteğe bağlı) hesaplamayı etkileyen ayarlar; aynı anahtar = aynı sonuç (hız için)
 *    hesapla(m, a, H)  dizileri hesaplar (H: aşağıdaki matematik yardımcıları)
 *    cizgiler(G, a)    grafikte çizilecek çizgiler
 *  Hesaplamalar sadece o mum ve öncesine bakar; geleceği kullanmaz.
 */
(function (kok) {
  'use strict';

  // ---------- matematik ----------
  function ema(x, n) {
    const out = new Array(x.length).fill(NaN);
    if (n < 1 || x.length < n) return out;
    let t = 0;
    for (let i = 0; i < n; i++) t += x[i];
    let e = t / n;
    out[n - 1] = e;
    const k = 2 / (n + 1);
    for (let i = n; i < x.length; i++) { e = x[i] * k + e * (1 - k); out[i] = e; }
    return out;
  }

  function hlc3(m) {
    const n = m.c.length, out = new Array(n);
    for (let i = 0; i < n; i++) out[i] = (m.h[i] + m.l[i] + m.c[i]) / 3;
    return out;
  }

  // Hacim ağırlıklı hareketli ortalama (VWMA)
  function vwma(src, v, n) {
    const L = src.length, out = new Array(L).fill(NaN);
    let spv = 0, sv = 0, sp = 0;
    for (let i = 0; i < L; i++) {
      const vi = v[i] > 0 ? v[i] : 0;
      spv += src[i] * vi; sv += vi; sp += src[i];
      if (i >= n) { const vj = v[i - n] > 0 ? v[i - n] : 0; spv -= src[i - n] * vj; sv -= vj; sp -= src[i - n]; }
      if (i >= n - 1) out[i] = sv > 1e-12 ? spv / sv : sp / n;
    }
    return out;
  }

  // Standart sapma (TradingView ta.stdev gibi: n'e bölünür)
  function stdev(src, n) {
    const L = src.length, out = new Array(L).fill(NaN);
    if (!L) return out;
    const ref = src[0];
    let s = 0, ss = 0;
    for (let i = 0; i < L; i++) {
      const x = src[i] - ref;
      s += x; ss += x * x;
      if (i >= n) { const y = src[i - n] - ref; s -= y; ss -= y * y; }
      if (i >= n - 1) { const ort = s / n; out[i] = Math.sqrt(Math.max(0, ss / n - ort * ort)); }
    }
    return out;
  }

  // Sabitlenmiş VWAP ve hacim ağırlıklı sapma (σ). Her gün/hafta/ay başında (UTC) sıfırlanır.
  function periyotNo(ms, periyot) {
    const gun = Math.floor(ms / 864e5);
    if (periyot === 'hafta') return Math.floor((gun + 3) / 7); // pazartesi başlar
    if (periyot === 'ay') { const d = new Date(ms); return d.getUTCFullYear() * 12 + d.getUTCMonth(); }
    return gun;
  }
  function vwap(m, periyot) {
    const n = m.c.length;
    const vw = new Array(n), sapma = new Array(n), bas = new Array(n);
    let onceki = null, ref = 0, sv = 0, sx = 0, sxx = 0, adet = 0, sxDuz = 0, sxxDuz = 0;
    for (let i = 0; i < n; i++) {
      const p = periyotNo(m.t[i], periyot);
      const tp = (m.h[i] + m.l[i] + m.c[i]) / 3;
      if (p !== onceki) { onceki = p; ref = tp; sv = sx = sxx = 0; adet = 0; sxDuz = sxxDuz = 0; bas[i] = true; } else bas[i] = false;
      const v = m.v[i] > 0 ? m.v[i] : 0, x = tp - ref;
      sv += v; sx += v * x; sxx += v * x * x;
      adet++; sxDuz += x; sxxDuz += x * x;
      let ort, varyans;
      if (sv > 1e-12) { ort = sx / sv; varyans = sxx / sv - ort * ort; }
      else { ort = sxDuz / adet; varyans = sxxDuz / adet - ort * ort; }
      vw[i] = ref + ort;
      sapma[i] = Math.sqrt(Math.max(0, varyans));
    }
    return { vwap: vw, sapma, bas };
  }

  // Kapanıştan kapanışa getirilerin (log) son W mumdaki standart sapması: coinin "normal dalgalanması"
  function getiriSapmasi(c, W) {
    const n = c.length, out = new Array(n).fill(NaN), r = new Array(n).fill(0);
    let s = 0, ss = 0;
    for (let i = 1; i < n; i++) {
      r[i] = c[i] > 0 && c[i - 1] > 0 ? Math.log(c[i] / c[i - 1]) : 0;
      s += r[i]; ss += r[i] * r[i];
      if (i > W) { s -= r[i - W]; ss -= r[i - W] * r[i - W]; }
      if (i >= W) { const o = s / W; out[i] = Math.sqrt(Math.max(0, ss / W - o * o)); }
    }
    return out;
  }

  // ---------- vites (trend durumu) ----------
  // Eğim = ortalamanın son N mumdaki değişimi / (normal dalgalanma × √N)
  // 1 demek: ortalama, coinin normal dalgalanmasının 1 katı hızla gidiyor.
  const NORMAL_PENCERE = 200;
  const VITES_ANAHTAR = ['sertDusus', 'dusus', 'yatay', 'yukselis', 'sertYukselis'];
  const VITES_AD = ['Sert düşüş', 'Düşüş', 'Yatay', 'Yükseliş', 'Sert yükseliş'];
  function vitesSinifi(e, a) {
    if (!isFinite(e)) return -1;
    if (e <= -a.sinir2) return 0;
    if (e <= -a.sinir1) return 1;
    if (e < a.sinir1) return 2;
    if (e < a.sinir2) return 3;
    return 4;
  }

  const H = { ema, hlc3, vwma, stdev, vwap, periyotNo, getiriSapmasi };

  const FIB = [0.236, 0.382, 0.5, 0.618, 0.764, 1];
  const sayiYaz = (x, d = 3) => Number(x).toLocaleString('tr-TR', { maximumFractionDigits: d });

  const GOSTERGELER = [
    {
      id: 'ortalama',
      ad: 'Sarı ortalama',
      ayarlar: [{ k: 'uzunluk', etiket: 'Kaç mumun ortalaması', min: 2, max: 400, adim: 1, deger: 50, tam: true, ara: [5, 200] }],
      aciklama: a => `Son <b>${a.uzunluk}</b> mumun üssel ortalaması (EMA). Küçük sayı fiyata yakın ve hızlı, büyük sayı yavaş ve düz olur.`,
      isinma: a => a.uzunluk,
      hesapla: (m, a) => ({ ort: ema(m.c, a.uzunluk) }),
      cizgiler: (G, a) => [{ ad: `Ortalama (${a.uzunluk})`, renk: 'vurgu', kalinlik: 2, degerler: G.ort }],
    },
    {
      id: 'ema',
      ad: 'EMA trend çizgileri',
      ayarlar: [
        { k: 'hizli', etiket: 'Hızlı EMA (mum)', min: 2, max: 300, adim: 1, deger: 21, tam: true, ara: [5, 60] },
        { k: 'yavas', etiket: 'Yavaş EMA (mum)', min: 3, max: 500, adim: 1, deger: 55, tam: true, ara: [20, 250] },
      ],
      aciklama: a => `Hızlı EMA (<b>${a.hizli}</b>) yavaş EMA’nın (<b>${a.yavas}</b>) üstündeyse trend yukarı, altındaysa aşağı. 21 ve 55 Fibonacci sayılarıdır.`,
      isinma: a => Math.max(a.hizli, a.yavas),
      hesapla: (m, a) => ({ hizli: ema(m.c, a.hizli), yavas: ema(m.c, a.yavas) }),
      cizgiler: (G, a) => [
        { ad: `EMA ${a.hizli}`, renk: 'camgobegi', kalinlik: 1.5, degerler: G.hizli },
        { ad: `EMA ${a.yavas}`, renk: 'mor', kalinlik: 1.5, degerler: G.yavas },
      ],
    },
    {
      id: 'fbb',
      ad: 'Fibonacci Bollinger bantları',
      ayarlar: [
        { k: 'uzunluk', etiket: 'Kaç mum geriye baksın', min: 10, max: 1000, adim: 1, deger: 200, tam: true, ara: [20, 400] },
        { k: 'carpan', etiket: 'Bant genişliği (sapma çarpanı)', min: 0.1, max: 6, adim: 0.01, deger: 3, ara: [1, 4.5] },
      ],
      aciklama: a => `Orta çizgi: son <b>${a.uzunluk}</b> mumun hacim ağırlıklı ortalaması. Bantlar: <b>${sayiYaz(a.carpan, 2)}</b> sapmanın Fibonacci oranları (0,236 … 1). Piyasa hareketlenince bantlar açılır, sakinleşince daralır.`,
      isinma: a => a.uzunluk,
      anahtar: a => String(a.uzunluk), // çarpan hesaplamayı değiştirmez, kurallarda uygulanır
      hesapla: (m, a) => { const tp = hlc3(m); return { orta: vwma(tp, m.v, a.uzunluk), std: stdev(tp, a.uzunluk) }; },
      cizgiler: (G, a) => {
        const out = [{ ad: 'Fib Bollinger orta', renk: 'gri', kalinlik: 1, degerler: G.orta }];
        for (const f of FIB) {
          const k = f * a.carpan;
          out.push({ ad: null, renk: 'soluk', kalinlik: 1, degerler: G.orta.map((o, i) => o + k * G.std[i]) });
          out.push({ ad: null, renk: 'soluk', kalinlik: 1, degerler: G.orta.map((o, i) => o - k * G.std[i]) });
        }
        out[1].ad = 'Fib seviyeleri (0,236 … 1)';
        return out;
      },
    },
    {
      id: 'vwap',
      ad: 'VWAP ve sapma bantları',
      ayarlar: [{
        k: 'periyot', tip: 'secim', etiket: 'Ne zaman sıfırlansın', deger: 'gun',
        secenekler: [{ v: 'gun', ad: 'Her gün' }, { v: 'hafta', ad: 'Her hafta' }, { v: 'ay', ad: 'Her ay' }],
      }],
      aciklama: a => `Hacim ağırlıklı ortalama fiyat. Her <b>${{ gun: 'gün', hafta: 'hafta', ay: 'ay' }[a.periyot]}</b> başında sıfırlanır (gün başı Türkiye saatiyle 03:00). Sapma (σ): fiyatın VWAP etrafında normalde ne kadar oynadığı; piyasa hareketliyse büyür. 1 günlük mumlarda “her hafta” ya da “her ay” seç.`,
      isinma: () => 1,
      hesapla: (m, a) => vwap(m, a.periyot),
      cizgiler: G => [{ ad: 'VWAP', renk: 'pembe', kalinlik: 2, degerler: G.vwap.map((x, i) => (G.bas[i] ? NaN : x)) }],
    },
  ];

  GOSTERGELER.push({
    id: 'vites',
    ad: 'Trend durumu (vites)',
    ayarlar: [
      { k: 'uzunluk', etiket: 'Eğimi ölçülen ortalama (EMA, mum)', min: 5, max: 400, adim: 1, deger: 89, tam: true, ara: [21, 200] },
      { k: 'bakis', etiket: 'Eğim kaç mumda ölçülsün', min: 2, max: 200, adim: 1, deger: 21, tam: true, ara: [5, 60] },
      { k: 'sinir1', etiket: 'Yatay sınırı (eğim bunun altındaysa yatay)', min: 0.01, max: 10, adim: 0.01, deger: 0.35, ara: [0.1, 1.5] },
      { k: 'sinir2', etiket: 'Sert sınırı (eğim bunun üstündeyse sert)', min: 0.02, max: 20, adim: 0.01, deger: 1, ara: [0.5, 3.5] },
    ],
    aciklama: a => `Bot, EMA ${a.uzunluk} çizgisinin son <b>${a.bakis}</b> mumdaki eğimine bakar. Eğim, coinin son ${NORMAL_PENCERE} mumdaki normal dalgalanmasına göre ölçülür (1 = normalin 1 katı hızla gidiyor). Eğim ±<b>${sayiYaz(a.sinir1, 2)}</b> arasındaysa yatay, ±<b>${sayiYaz(a.sinir2, 2)}</b>’in ötesindeyse sert sayılır. Çizginin rengi vitesi gösterir.`,
    isinma: a => Math.max(a.uzunluk - 1 + a.bakis, NORMAL_PENCERE) + 1,
    anahtar: a => `${a.uzunluk}|${a.bakis}`, // sınırlar hesaplamayı değiştirmez, sınıflamada kullanılır
    hesapla: (m, a) => {
      const ort = ema(m.c, a.uzunluk), v = getiriSapmasi(m.c, NORMAL_PENCERE);
      const n = m.c.length, egim = new Array(n).fill(NaN), N = a.bakis;
      for (let i = N; i < n; i++) {
        if (isFinite(ort[i]) && isFinite(ort[i - N]) && v[i] > 0) egim[i] = Math.log(ort[i] / ort[i - N]) / (v[i] * Math.sqrt(N));
      }
      return { ort, egim };
    },
    cizgiler: (G, a) => [{ ad: `Vites çizgisi (EMA ${a.uzunluk})`, renk: 'metin2', kalinlik: 2, degerler: G.ort, durumlar: G.egim.map(e => vitesSinifi(e, a)) }],
  });

  kok.VITES = {
    ANAHTAR: VITES_ANAHTAR,
    AD: VITES_AD,
    NORMAL_PENCERE,
    sinif: vitesSinifi,
    durum: (b, i) => vitesSinifi(b.g('vites').egim[i], b.ga('vites')),
    egim: (b, i) => b.g('vites').egim[i],
  };
  kok.GOSTERGELER = GOSTERGELER;
  kok.GOSTERGE_MATEMATIK = H;
  kok.FIB_SEVIYELERI = FIB;
})(typeof window !== 'undefined' ? window : globalThis);
