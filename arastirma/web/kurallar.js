/*  KURALLAR — botun alım-satım kuralları
 *  ------------------------------------------------------------
 *  Botun 3 aşaması var:
 *     1) Bekliyor   → "baslat" kurallarının HEPSİ sağlanınca → 2) Dip arıyor
 *     2) Dip arıyor → "vazgec" kurallarından BİRİ sağlanırsa   → 1) Bekliyor (vazgeçer)
 *                   → "tetik" kurallarının HEPSİ sağlanınca    → AL → 3) Coinde
 *     3) Coinde     → "sat" kurallarından BİRİ sağlanınca      → SAT → 1) Bekliyor
 *
 *  Yeni kural eklemek = bu listeye yeni bir nesne eklemek:
 *    id        benzersiz kısa ad (strateji dosyalarında bu ad kullanılır, değiştirme)
 *    yer       'baslat' | 'tetik' | 'vazgec' | 'sat'
 *    ad        ekranda görünen başlık
 *    ornek     günlük hayattan kısa bir örnek
 *    acik      varsayılan olarak açık mı
 *    gosterge  kullandığı göstergeler (gostergeler.js'teki id'ler)
 *    oncelik   (sadece 'sat') aynı mumda iki satış sebebi varsa küçük sayı önce
 *    ayarlar   [{ k, etiket, min, max, adim, deger, ara? }] ya da [{ k, tip:'secim', secenekler, deger }]
 *              ara: "en kârlı ayarı bul" butonunun deneyeceği aralık
 *    kisa(a)          akış şemasındaki kısa etiket
 *    cumle(a)         kuralı anlatan cümle (değerler <b> içinde)
 *    kontrol(b,i,a,d) bu mum kapanınca kural sağlanıyor mu?  → { ok, not: () => '...', fiyat? }
 *                     b: mumlar ve göstergeler (b.g('fbb') hesaplanmış dizi, b.ga('fbb') ayarları)
 *                     i: mum sırası, a: kuralın ayar değerleri, d: botun durumu
 *                     fiyat: emir mum içinde dolduysa dolduğu fiyat (yoksa kapanış)
 *    cizgi(b,a)       (isteğe bağlı) grafikte sürekli çizilen çizgi(ler)
 *    seviye(b,i,a,d)  (isteğe bağlı) o anki durumda bekleyen fiyat çizgileri
 *    devam(b,i,a,d)   (isteğe bağlı, sadece 'baslat') dip ararken, almadan önce her mumda tekrar bakılır;
 *                     sağlanmazsa bot o mumda almaz (dip aramaya devam eder). null dönerse bakılmaz.
 *    devamSeviye(b,i,a,d)  (isteğe bağlı) dip ararken grafikte gösterilecek fiyat çizgileri
 *  Ayar türleri: sayı (varsayılan), tip:'secim' (seçenekler), tip:'anahtar' (açık/kapalı).
 *    goster(a): ayar ancak bu doğruysa görünür ve "en kârlı ayarı bul" tarafından denenir.
 *    grup:'vites': vites tablosunun bir satırı (durum: 0 sert düşüş … 4 sert yükseliş).
 *    yeni + eskiDeger: ayar bu strateji sürümünde eklendi; daha eski sürümle kaydedilmiş bir stratejide
 *    (ayar dosyada yoksa) eskiDeger kullanılır, böylece eski dosyalar kaydedildikleri gibi çalışır.
 *  gosterge bir dizi ya da ayara göre dizi döndüren fonksiyon olabilir.
 */
(function (kok) {
  'use strict';

  const yz = (x, ondalik = 2) =>
    '%' + Number(x).toLocaleString('tr-TR', { minimumFractionDigits: 0, maximumFractionDigits: ondalik });
  const sy = (x, d = 3) => Number(x).toLocaleString('tr-TR', { maximumFractionDigits: d });
  // Fib Bollinger seviyeleri: eksi = alt bantlar, 0 = orta çizgi, artı = üst bantlar
  const seviyeAd = v => (v < 0 ? `alt ${sy(-v)}` : v === 0 ? 'orta çizgi' : `üst ${sy(v)}`);
  const FIB_GERI_CEKILME = [-1, -0.764, -0.618, -0.5, -0.382, -0.236, 0, 0.236, 0.382, 0.5]
    .map(v => ({ v, ad: v < 0 ? `Alt ${sy(-v)}` : v === 0 ? 'Orta' : `Üst ${sy(v)}` }));
  const FIB_UST = [0, 0.236, 0.382, 0.5, 0.618, 0.764, 1].map(v => ({ v, ad: v === 0 ? 'Orta' : sy(v) }));
  const fibAd = v => (v === 0 ? 'orta çizgi' : sy(v));

  // ---------- vites tabloları ----------
  // YOK = bu viteste bu kural çalışmasın (alış tablosunda "Alma", satış tablosunda "Satma")
  const YOK = 9;
  const VITES = kok.VITES;
  const ALIS_VITES = [0.5, 0.382, 0.236, 0, -0.236, -0.382, -0.5, -0.618, -0.764, -1]
    .map(v => ({ v, ad: v < 0 ? `Alt ${sy(-v)}` : v === 0 ? 'Orta' : `Üst ${sy(v)}` })).concat([{ v: YOK, ad: 'Alma' }]);
  const SATIS_VITES = [1, 0.764, 0.618, 0.5, 0.382, 0.236, 0, -0.236, -0.382, -0.5]
    .map(v => ({ v, ad: v < 0 ? `Alt ${sy(-v)}` : v === 0 ? 'Orta' : `Üst ${sy(v)}` })).concat([{ v: YOK, ad: 'Satma' }]);
  const VARS_ALIS = [-1, -0.618, -0.382, 0, 0.236];   // sert düşüş … sert yükseliş
  const VARS_SATIS = [-0.382, 0, 0.382, 0.618, 1];
  const vitesTablosu = (secenekler, varsayilan, ne) => VITES.ANAHTAR.map((k, j) => ({
    k, tip: 'secim', grup: 'vites', durum: j, etiket: `${VITES.AD[j]} vitesinde ${ne}`, secenekler, deger: varsayilan[j], goster: a => !!a.vites,
  }));
  const egimYaz = (b, i) => { const e = VITES.egim(b, i); return isFinite(e) ? (e >= 0 ? '+' : '−') + sy(Math.abs(e), 2) : '?'; };
  // Bu mumdaki vitese göre tablodaki seviye: { v: vites no, sv: seviye } (vites hesaplanamadıysa v = -1)
  function vitesSeviyesi(b, i, a) {
    const v = VITES.durum(b, i);
    return { v, sv: v < 0 ? YOK : a[VITES.ANAHTAR[v]] };
  }

  function fbbBant(b, i, seviye, yon) {
    const G = b.g('fbb'), a = b.ga('fbb');
    return G.orta[i] + yon * seviye * a.carpan * G.std[i];
  }
  function vwapBant(b, i, k) {
    const G = b.g('vwap');
    return G.vwap[i] + k * G.sapma[i];
  }

  const KURALLAR = [
    // ================= 1 → 2  DİP ARAMAYA BAŞLA =================
    {
      id: 'fbbAlt',
      yer: 'baslat',
      gosterge: a => (a.vites ? ['fbb', 'vites'] : ['fbb']),
      ad: 'Fiyat Fibonacci Bollinger bandına insin',
      ornek: 'Lastiği ne kadar gerdiğine bakmak gibi: piyasa sakinse az gerilince, hareketliyse çok gerilince banda değer. Düz piyasada alt bant, yükselen trendde orta çizgi ya da üst bant mantıklıdır.',
      acik: true,
      ayarlar: [
        { k: 'vites', tip: 'anahtar', etiket: 'Bandı trend durumu (vites) seçsin', deger: true, yeni: 3, eskiDeger: false },
        { k: 'seviye', tip: 'secim', etiket: 'Hangi banda insin', secenekler: FIB_GERI_CEKILME, deger: -0.382, goster: a => !a.vites },
      ].concat(vitesTablosu(ALIS_VITES, VARS_ALIS, 'alış bandı')),
      kisa: a => (a.vites ? 'Fib bandı (vitese göre)' : `Fib ${seviyeAd(a.seviye)}`),
      cumle: a => (a.vites
        ? `Bot her mumda trend durumuna (vitese) bakar ve tablodan o vitesin bandını seçer. Mumun en düşüğü o banda değince dip aramaya başlar. Almadan önce, görülen dibin o anki vitesin bandına ulaştığına da bakar (dip ararken vites değişirse yeni bandı bekler).`
        : a.seviye === 0
          ? `Mumun en düşüğü Fibonacci Bollinger’ın <b>orta çizgisine</b> değince bot dip aramaya başlar.`
          : `Mumun en düşüğü Fibonacci Bollinger’ın <b>${seviyeAd(a.seviye)}</b> bandına değince bot dip aramaya başlar.`),
      kontrol(b, i, a) {
        if (a.vites) {
          const { v, sv } = vitesSeviyesi(b, i, a);
          if (v < 0) return { ok: false, not: () => 'Vites henüz hesaplanamadı (yeterli mum yok)' };
          if (sv === YOK) return { ok: false, not: () => `Vites: ${VITES.AD[v]} (eğim ${egimYaz(b, i)}). Bu viteste alım kapalı.` };
          const bant = fbbBant(b, i, sv, 1);
          return { ok: b.l[i] <= bant, not: () => `Vites: ${VITES.AD[v]} (eğim ${egimYaz(b, i)}) → ${seviyeAd(sv)} bandı ${b.f(bant)}. Mumun en düşüğü ${b.f(b.l[i])}` };
        }
        const bant = fbbBant(b, i, a.seviye, 1);
        return { ok: b.l[i] <= bant, not: () => `Mumun en düşüğü ${b.f(b.l[i])}, ${seviyeAd(a.seviye)} ${b.f(bant)}` };
      },
      // Dip ararken, almadan hemen önce: görülen dip o anki vitesin bandına ulaştı mı?
      // (Vites dip ararken değişebilir; örneğin düşüş sertleşirse bot daha derin bandı bekler.)
      devam(b, i, a, d) {
        if (!a.vites) return null;
        const { v, sv } = vitesSeviyesi(b, i, a);
        if (v < 0) return { ok: false, not: () => 'Vites henüz hesaplanamadı' };
        if (sv === YOK) return { ok: false, not: () => `Vites: ${VITES.AD[v]} (eğim ${egimYaz(b, i)}). Bu viteste alım kapalı, bot bekliyor.` };
        const bant = fbbBant(b, i, sv, 1), dip = d.enDusuk;
        return { ok: dip <= bant, not: () => `Vites: ${VITES.AD[v]} (eğim ${egimYaz(b, i)}) → ${seviyeAd(sv)} bandı ${b.f(bant)}. Görülen dip ${b.f(dip)}: ${dip <= bant ? 'yeterince derin' : 'henüz o banda inmedi'}` };
      },
      cizgi: (b, a) => (a.vites
        ? { ad: 'Vitese göre alış bandı', renk: 'mavi', kesik: true, degerler: b.c.map((_, i) => { const { sv } = vitesSeviyesi(b, i, a); return sv === YOK ? NaN : fbbBant(b, i, sv, 1); }) }
        : { ad: `Fib ${seviyeAd(a.seviye)} (alış bölgesi)`, renk: 'mavi', kesik: true, degerler: b.c.map((_, i) => fbbBant(b, i, a.seviye, 1)) }),
      seviye: (b, i, a) => {
        if (!a.vites) return [{ ad: `Fib ${seviyeAd(a.seviye)}`, fiyat: fbbBant(b, i, a.seviye, 1), renk: 'mavi' }];
        const { v, sv } = vitesSeviyesi(b, i, a);
        return sv === YOK ? [] : [{ ad: `Fib ${seviyeAd(sv)} (${VITES.AD[v]})`, fiyat: fbbBant(b, i, sv, 1), renk: 'mavi' }];
      },
      devamSeviye: (b, i, a) => {
        if (!a.vites) return [];
        const { v, sv } = vitesSeviyesi(b, i, a);
        return sv === YOK ? [] : [{ ad: `Vites bandı (${VITES.AD[v]})`, fiyat: fbbBant(b, i, sv, 1), renk: 'mavi' }];
      },
    },
    {
      id: 'emaTrend',
      yer: 'baslat',
      gosterge: ['ema'],
      ad: 'Trend yukarı olsun (EMA)',
      ornek: 'Akıntıya karşı değil, akıntıyla yüzmek gibi. Bunu açarsan Fib bandını orta ya da üst seç: yükselen trendde fiyat alt bantlara pek inmez.',
      acik: false,
      ayarlar: [{ k: 'fark', etiket: 'Hızlı EMA, yavaşın en az yüzde kaç üstünde', min: 0, max: 10, adim: 0.01, deger: 0, ara: [0, 2], birim: '%' }],
      kisa: a => (a.fark > 0 ? `trend yukarı (EMA farkı ${yz(a.fark)})` : 'trend yukarı (EMA)'),
      cumle: a => `Hızlı EMA, yavaş EMA’nın ${a.fark > 0 ? `en az <b>${yz(a.fark)}</b>` : ''} üstünde olmalı (trend yukarı).`,
      kontrol(b, i, a) {
        const G = b.g('ema'), A = b.ga('ema');
        const fark = (G.hizli[i] / G.yavas[i] - 1) * 100;
        return { ok: fark >= a.fark, not: () => `EMA ${A.hizli} = ${b.f(G.hizli[i])}, EMA ${A.yavas} = ${b.f(G.yavas[i])}. Fark ${fark >= 0 ? '+' : '−'}${yz(Math.abs(fark))}` };
      },
    },
    {
      id: 'vwapAlti',
      yer: 'baslat',
      gosterge: ['vwap'],
      ad: 'Fiyat VWAP’ın altına insin',
      ornek: 'Günün ortalama fiyatının belirgin altında, “ucuz” bölgede alışverişe başlamak gibi.',
      acik: false,
      ayarlar: [{ k: 'sapma', etiket: 'VWAP’ın en az kaç sapma (σ) altında', min: 0, max: 5, adim: 0.01, deger: 1, ara: [0, 3], birim: 'σ' }],
      kisa: a => `VWAP −${sy(a.sapma, 2)}σ altı`,
      cumle: a => `Fiyat VWAP’ın en az <b>${sy(a.sapma, 2)} sapma</b> altında kapanınca dip aramaya başlar.`,
      kontrol(b, i, a) {
        const bant = vwapBant(b, i, -a.sapma), G = b.g('vwap');
        const uzak = G.sapma[i] > 0 ? (b.c[i] - G.vwap[i]) / G.sapma[i] : 0;
        return { ok: b.c[i] <= bant, not: () => `Fiyat ${b.f(b.c[i])}, VWAP ${b.f(G.vwap[i])}. Fiyat VWAP’tan ${sy(uzak, 2)}σ uzakta; sınır ${b.f(bant)}` };
      },
      cizgi: (b, a) => ({ ad: `VWAP −${sy(a.sapma, 2)}σ`, renk: 'pembe', kesik: true, degerler: b.c.map((_, i) => vwapBant(b, i, -a.sapma)) }),
    },
    {
      id: 'vwapYakin',
      yer: 'baslat',
      gosterge: ['vwap'],
      ad: 'Fiyat VWAP’a yakın olsun',
      ornek: 'Yükselen bir günde fiyat günün ortalamasına kadar geri gelince almak gibi (sığ geri çekilme).',
      acik: false,
      ayarlar: [{ k: 'sapma', etiket: 'VWAP’tan en fazla kaç sapma (σ) uzakta', min: 0.01, max: 3, adim: 0.01, deger: 0.5, ara: [0.05, 1.5], birim: 'σ' }],
      kisa: a => `VWAP ±${sy(a.sapma, 2)}σ yakını`,
      cumle: a => `Fiyat VWAP’ın en fazla <b>${sy(a.sapma, 2)} sapma</b> uzağında (altında ya da üstünde) olmalı.`,
      kontrol(b, i, a) {
        const G = b.g('vwap');
        const uzak = Math.abs(b.c[i] - G.vwap[i]);
        return { ok: uzak <= a.sapma * G.sapma[i], not: () => `Fiyat ${b.f(b.c[i])}, VWAP ${b.f(G.vwap[i])}. İzin verilen bölge ${b.f(vwapBant(b, i, -a.sapma))} – ${b.f(vwapBant(b, i, a.sapma))}` };
      },
      cizgi: (b, a) => [
        { ad: `VWAP ±${sy(a.sapma, 2)}σ`, renk: 'pembe', kesik: true, degerler: b.c.map((_, i) => vwapBant(b, i, a.sapma)) },
        { ad: null, renk: 'pembe', kesik: true, degerler: b.c.map((_, i) => vwapBant(b, i, -a.sapma)) },
      ],
    },
    {
      id: 'ortalamaAlti',
      yer: 'baslat',
      gosterge: ['ortalama'],
      ad: 'Fiyat sarı ortalamanın altına insin',
      ornek: 'Normalde 100 TL olan bir şey 98,5 TL’ye düşünce ilgilenmeye başlamak gibi.',
      acik: false,
      ayarlar: [{ k: 'yuzde', etiket: 'Ortalamanın yüzde kaç altı', min: 0.01, max: 20, adim: 0.01, deger: 1.5, ara: [0.1, 5], birim: '%' }],
      kisa: a => `ort. ${yz(a.yuzde)} altı`,
      cumle: a => `Fiyat sarı ortalama çizgisinin en az <b>${yz(a.yuzde)}</b> altına inince bot dip aramaya başlar.`,
      kontrol(b, i, a) {
        const sinir = b.ort[i] * (1 - a.yuzde / 100);
        const fark = (b.c[i] / b.ort[i] - 1) * 100;
        return { ok: b.c[i] <= sinir, not: () => `Fiyat ${b.f(b.c[i])}, ortalamanın ${fark < 0 ? yz(-fark) + ' altında' : yz(fark) + ' üstünde'}. Sınır: ${b.f(sinir)}` };
      },
      cizgi: (b, a) => ({ ad: 'Ortalama altı sınırı', renk: 'mavi', kesik: true, degerler: b.ort.map(x => x * (1 - a.yuzde / 100)) }),
    },

    // ================= 2 → 3  AL =================
    {
      id: 'diptenDonus',
      yer: 'tetik',
      ad: 'Dipten dönünce al',
      ornek: 'Top yere çarpıp zıplamaya başlayınca dibi gördüğünü anlarsın. Dibi ancak dönüşten sonra bilebilirsin.',
      acik: true,
      ayarlar: [{ k: 'yuzde', etiket: 'Dipten yüzde kaç yukarı dönsün', min: 0.01, max: 10, adim: 0.01, deger: 0.5, ara: [0.05, 3], birim: '%' }],
      kisa: a => `dipten ${yz(a.yuzde)} dönüş`,
      cumle: a => `Dip ararken görülen en düşük fiyattan <b>${yz(a.yuzde)}</b> yukarı dönünce bot alır.`,
      kontrol(b, i, a, d) {
        const dip = d.enDusuk;
        const hedef = dip * (1 + a.yuzde / 100);
        const donus = (b.c[i] / dip - 1) * 100;
        return { ok: b.c[i] >= hedef, not: () => `En düşük ${b.f(dip)}. Fiyat ${b.f(b.c[i])}, dipten ${yz(donus)} yukarıda. Alış çizgisi: ${b.f(hedef)}` };
      },
      seviye: (b, i, a, d) => [
        { ad: 'Dip', fiyat: d.enDusuk, renk: 'gri' },
        { ad: 'Alış çizgisi', fiyat: d.enDusuk * (1 + a.yuzde / 100), renk: 'yesil' },
      ],
    },

    // ================= 2 → 1  VAZGEÇ =================
    {
      id: 'ortalamaUstu',
      yer: 'vazgec',
      gosterge: ['ortalama'],
      ad: 'Fiyat sarı ortalamanın üstüne çıkarsa vazgeç',
      ornek: 'İndirim bitti, fiyat normale döndü: artık “ucuzdan alma” fırsatı kalmadı.',
      acik: true,
      ayarlar: [{ k: 'yuzde', etiket: 'Ortalamanın yüzde kaç üstüne çıkarsa', min: 0, max: 10, adim: 0.01, deger: 0, ara: [0, 2], birim: '%' }],
      kisa: a => (a.yuzde > 0 ? `ortalamanın ${yz(a.yuzde)} üstü` : 'ortalamanın üstü'),
      cumle: a => `Dip ararken fiyat sarı ortalamanın ${a.yuzde > 0 ? `<b>${yz(a.yuzde)}</b> ` : ''}üstünde kapanırsa bot dip aramayı bırakır.`,
      kontrol(b, i, a) {
        const sinir = b.ort[i] * (1 + a.yuzde / 100);
        return { ok: b.c[i] > sinir, not: () => `Fiyat ${b.f(b.c[i])}, vazgeçme sınırı ${b.f(sinir)}` };
      },
      seviye: (b, i, a) => [{ ad: 'Vazgeçme sınırı', fiyat: b.ort[i] * (1 + a.yuzde / 100), renk: 'gri' }],
    },
    {
      id: 'aramaUzadi',
      yer: 'vazgec',
      ad: 'Dip arama çok uzarsa vazgeç',
      ornek: 'Otobüs gelmiyorsa bir süre sonra beklemeyi bırakmak gibi.',
      acik: false,
      ayarlar: [{ k: 'mum', etiket: 'En fazla kaç mum dip arasın', min: 1, max: 2000, adim: 1, deger: 48, tam: true, ara: [3, 200], birim: 'mum' }],
      kisa: a => `${a.mum} mumdan uzun`,
      cumle: a => `Dip arama <b>${a.mum}</b> mumdan uzun sürerse bot vazgeçer.`,
      kontrol(b, i, a, d) {
        const gecen = i - d.dipBas;
        return { ok: gecen >= a.mum, not: () => `Dip arama ${gecen} mumdur sürüyor (sınır ${a.mum})` };
      },
    },

    // ================= 3 → 1  SAT =================
    {
      id: 'zararDurdur',
      yer: 'sat',
      oncelik: 1,
      ad: 'Zarar durdur',
      ornek: '“Şu kadar kaybedersem kalkarım” demek gibi. Büyük kaybı keser.',
      acik: true,
      ayarlar: [{ k: 'yuzde', etiket: 'Alıştan yüzde kaç düşerse', min: 0.1, max: 30, adim: 0.01, deger: 3, ara: [0.5, 10], birim: '%' }],
      kisa: a => `zarar ${yz(a.yuzde)}`,
      cumle: a => `Fiyat alış fiyatının <b>${yz(a.yuzde)}</b> altına değerse bot hemen satar.`,
      kontrol(b, i, a, d) {
        const giris = d.giris;
        const seviye = giris * (1 - a.yuzde / 100);
        const ok = b.l[i] <= seviye;
        return { ok, fiyat: ok ? Math.min(b.o[i], seviye) : undefined, not: () => `Alış ${b.f(giris)}. Zarar durdur ${b.f(seviye)}. Bu mumun en düşüğü ${b.f(b.l[i])}` };
      },
      seviye: (b, i, a, d) => [{ ad: 'Zarar durdur', fiyat: d.giris * (1 - a.yuzde / 100), renk: 'kirmizi' }],
    },
    {
      id: 'fbbUst',
      yer: 'sat',
      oncelik: 2,
      gosterge: a => (a.vites ? ['fbb', 'vites'] : ['fbb']),
      ad: 'Fibonacci Bollinger bandına değince sat',
      ornek: 'Gerilen lastiğin geri dönmesini beklemek gibi: fiyat ortaya ya da yukarıdaki banda gelince sat.',
      acik: true,
      ayarlar: [
        { k: 'vites', tip: 'anahtar', etiket: 'Bandı trend durumu (vites) seçsin', deger: true, yeni: 3, eskiDeger: false },
        { k: 'seviye', tip: 'secim', etiket: 'Hangi bant (orta ya da üst)', secenekler: FIB_UST, deger: 0.382, goster: a => !a.vites },
      ].concat(vitesTablosu(SATIS_VITES, VARS_SATIS, 'satış bandı')),
      kisa: a => (a.vites ? 'Fib bandı (vitese göre)' : `Fib ${a.seviye === 0 ? 'orta' : 'üst ' + sy(a.seviye)}`),
      cumle: a => (a.vites
        ? `Bot her mumda trend durumuna (vitese) bakar ve tablodan o vitesin satış bandını seçer. Fiyat o banda değince oradan satar. (Bant alış fiyatının altındaysa beklenir.)`
        : a.seviye === 0
          ? `Fiyat Fibonacci Bollinger’ın <b>orta çizgisine</b> değince bot oradan satar. (Çizgi alış fiyatının altındaysa beklenir.)`
          : `Fiyat Fibonacci Bollinger’ın <b>${sy(a.seviye)}</b> üst bandına değince bot oradan satar. (Bant alış fiyatının altındaysa beklenir.)`),
      kontrol(b, i, a, d) {
        let sv = a.seviye, vno = -1;
        if (a.vites) {
          const r = vitesSeviyesi(b, i, a);
          if (r.v < 0) return { ok: false, not: () => 'Vites henüz hesaplanamadı' };
          if (r.sv === YOK) return { ok: false, not: () => `Vites: ${VITES.AD[r.v]} (eğim ${egimYaz(b, i)}). Bu viteste banttan satış kapalı.` };
          sv = r.sv; vno = r.v;
        }
        const bant = fbbBant(b, i, sv, 1), giris = d.giris;
        const ok = b.h[i] >= bant && bant >= giris;
        return {
          ok, fiyat: ok ? Math.max(b.o[i], bant) : undefined,
          not: () => `${vno >= 0 ? `Vites: ${VITES.AD[vno]} (eğim ${egimYaz(b, i)}) → ${seviyeAd(sv)} bandı` : `Bant (${fibAd(sv)})`} ${b.f(bant)}. Bu mumun en yükseği ${b.f(b.h[i])}${bant < giris ? '. Bant alış fiyatının altında, bekleniyor' : ''}`,
        };
      },
      cizgi: (b, a) => (a.vites
        ? { ad: 'Vitese göre satış bandı', renk: 'turuncu', kesik: true, degerler: b.c.map((_, i) => { const { sv } = vitesSeviyesi(b, i, a); return sv === YOK ? NaN : fbbBant(b, i, sv, 1); }) }
        : { ad: `Fib ${a.seviye === 0 ? 'orta' : 'üst bant ' + sy(a.seviye)}`, renk: 'turuncu', kesik: true, degerler: b.c.map((_, i) => fbbBant(b, i, a.seviye, 1)) }),
      seviye: (b, i, a) => {
        if (!a.vites) return [{ ad: `Fib ${a.seviye === 0 ? 'orta' : 'üst ' + sy(a.seviye)}`, fiyat: fbbBant(b, i, a.seviye, 1), renk: 'turuncu' }];
        const { v, sv } = vitesSeviyesi(b, i, a);
        return sv === YOK ? [] : [{ ad: `Fib ${seviyeAd(sv)} (${VITES.AD[v]})`, fiyat: fbbBant(b, i, sv, 1), renk: 'turuncu' }];
      },
    },
    {
      id: 'vwapUst',
      yer: 'sat',
      oncelik: 2,
      gosterge: ['vwap'],
      ad: 'VWAP bandına değince sat',
      ornek: 'Ucuzdan aldığın şeyi günün ortalama fiyatına (ya da üstüne) gelince satmak gibi.',
      acik: false,
      ayarlar: [{ k: 'sapma', etiket: 'VWAP’ın kaç sapma (σ) üstü (0 = VWAP’ın kendisi)', min: 0, max: 5, adim: 0.01, deger: 1, ara: [0, 3], birim: 'σ' }],
      kisa: a => (a.sapma > 0 ? `VWAP +${sy(a.sapma, 2)}σ` : 'VWAP’a dönüş'),
      cumle: a => (a.sapma > 0
        ? `Fiyat VWAP’ın <b>${sy(a.sapma, 2)} sapma</b> üstüne değince bot oradan satar. (Alış fiyatının altındaysa beklenir.)`
        : `Fiyat VWAP çizgisine değince bot oradan satar. (Alış fiyatının altındaysa beklenir.)`),
      kontrol(b, i, a, d) {
        const bant = vwapBant(b, i, a.sapma);
        const ok = b.h[i] >= bant && bant >= d.giris;
        return { ok, fiyat: ok ? Math.max(b.o[i], bant) : undefined, not: () => `VWAP +${sy(a.sapma, 2)}σ = ${b.f(bant)}. Bu mumun en yükseği ${b.f(b.h[i])}${bant < d.giris ? '. Bant alış fiyatının altında, bekleniyor' : ''}` };
      },
      cizgi: (b, a) => ({ ad: `VWAP +${sy(a.sapma, 2)}σ`, renk: 'pembe', kesik: true, degerler: b.c.map((_, i) => vwapBant(b, i, a.sapma)) }),
      seviye: (b, i, a) => [{ ad: `VWAP +${sy(a.sapma, 2)}σ`, fiyat: vwapBant(b, i, a.sapma), renk: 'pembe' }],
    },
    {
      id: 'hedefKar',
      yer: 'sat',
      oncelik: 2,
      ad: 'Hedef kâra ulaşınca sat',
      ornek: '“Şu fiyata gelirse satarım” diye önceden fiyat koymak gibi.',
      acik: false,
      ayarlar: [{ k: 'yuzde', etiket: 'Alıştan yüzde kaç yukarıda', min: 0.05, max: 50, adim: 0.01, deger: 3, ara: [0.3, 15], birim: '%' }],
      kisa: a => `hedef ${yz(a.yuzde)}`,
      cumle: a => `Fiyat alış fiyatının <b>${yz(a.yuzde)}</b> üstüne değerse bot satar.`,
      kontrol(b, i, a, d) {
        const giris = d.giris;
        const seviye = giris * (1 + a.yuzde / 100);
        const ok = b.h[i] >= seviye;
        return { ok, fiyat: ok ? Math.max(b.o[i], seviye) : undefined, not: () => `Alış ${b.f(giris)}. Hedef ${b.f(seviye)}. Bu mumun en yükseği ${b.f(b.h[i])}` };
      },
      seviye: (b, i, a, d) => [{ ad: 'Hedef', fiyat: d.giris * (1 + a.yuzde / 100), renk: 'yesil' }],
    },
    {
      id: 'tepedenDonus',
      yer: 'sat',
      oncelik: 3,
      ad: 'Tepeden dönünce sat',
      ornek: 'Merdivende bir basamak aşağı indiysen zirveyi geçmişsindir.',
      acik: true,
      ayarlar: [{ k: 'yuzde', etiket: 'Tepeden yüzde kaç düşünce', min: 0.01, max: 20, adim: 0.01, deger: 1, ara: [0.1, 5], birim: '%' }],
      kisa: a => `tepeden ${yz(a.yuzde)}`,
      cumle: a => `Aldıktan sonra görülen en yüksek fiyattan <b>${yz(a.yuzde)}</b> aşağı düşünce bot satar. (Mum kapanışına bakar.)`,
      kontrol(b, i, a, d) {
        const tepe = d.enYuksek;
        const sinir = tepe * (1 - a.yuzde / 100);
        const dusus = (1 - b.c[i] / tepe) * 100;
        return { ok: b.c[i] <= sinir, not: () => `En yüksek ${b.f(tepe)}. Fiyat ${b.f(b.c[i])}, tepeden ${yz(Math.max(0, dusus))} aşağıda. Satış çizgisi: ${b.f(sinir)}` };
      },
      seviye: (b, i, a, d) => [{ ad: 'Satış çizgisi', fiyat: d.enYuksek * (1 - a.yuzde / 100), renk: 'turuncu' }],
    },
    {
      id: 'emaAsagi',
      yer: 'sat',
      oncelik: 4,
      gosterge: ['ema'],
      ad: 'Trend aşağı dönünce sat (EMA)',
      ornek: 'Akıntı yön değiştirince kıyıya çıkmak gibi.',
      acik: false,
      ayarlar: [],
      kisa: () => 'trend aşağı (EMA)',
      cumle: () => `Hızlı EMA, yavaş EMA’nın altına inince bot mum kapanışında satar.`,
      kontrol(b, i) {
        const G = b.g('ema'), A = b.ga('ema');
        return { ok: G.hizli[i] < G.yavas[i], not: () => `EMA ${A.hizli} = ${b.f(G.hizli[i])}, EMA ${A.yavas} = ${b.f(G.yavas[i])}` };
      },
    },
  ];

  const ASAMALAR = {
    baslat: { no: '1 → 2', baslik: 'Dip aramaya ne zaman başlasın?', alt: 'Açık olanların hepsi sağlanmalı' },
    tetik: { no: '2 → 3', baslik: 'Ne zaman alsın?', alt: 'Açık olanların hepsi sağlanmalı' },
    vazgec: { no: '2 → 1', baslik: 'Dip aramayı ne zaman bıraksın?', alt: 'Biri sağlanınca vazgeçer' },
    sat: { no: '3 → 1', baslik: 'Ne zaman satsın?', alt: 'Biri sağlanınca satar' },
  };

  const STRATEJI_SURUMU = 3; // 3: vites eklendi
  const VARSAYILAN_STRATEJI = () => ({
    surum: STRATEJI_SURUMU,
    gostergeler: Object.fromEntries(kok.GOSTERGELER.map(g => [g.id, Object.fromEntries(g.ayarlar.map(x => [x.k, x.deger]))])),
    para: { baslangic: 1000, masraf: 0.1 },
    kurallar: Object.fromEntries(
      KURALLAR.map(k => [k.id, { acik: k.acik, d: Object.fromEntries(k.ayarlar.map(x => [x.k, x.deger])) }])
    ),
  });

  kok.KURALLAR = KURALLAR;
  kok.VITES_YOK = YOK;
  kok.ASAMALAR = ASAMALAR;
  kok.STRATEJI_SURUMU = STRATEJI_SURUMU;
  kok.VARSAYILAN_STRATEJI = VARSAYILAN_STRATEJI;
})(typeof window !== 'undefined' ? window : globalThis);
