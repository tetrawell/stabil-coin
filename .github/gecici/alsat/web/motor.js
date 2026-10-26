/*  MOTOR — botu mum mum çalıştırır.
 *  Kural: bot her mumda SADECE o mumun kapanışına kadar olanı bilir.
 *  Geleceğe bakmaz; bu yüzden geçmişte gördüğün sonuç, canlıda olacakla aynı mantıktadır.
 *
 *  Bu dosya ekrandan bağımsızdır (DOM kullanmaz). Başka bir program (ör. ileride yapılacak
 *  otomatik al-sat programı) gostergeler.js + kurallar.js + motor.js'i yükleyip, kaydedilen
 *  strateji dosyasıyla aynı kararları verebilir:  Motor.sonKarar(mumlar, strateji)
 */
(function (kok) {
  'use strict';

  const GOSTERGE = () => Object.fromEntries(kok.GOSTERGELER.map(g => [g.id, g]));

  // surum: stratejinin kaydedildiği sürüm. Sonradan eklenen bir ayar eski bir dosyada yoksa eskiDeger kullanılır.
  function ayarDegerleri(tanimlar, kayit, surum) {
    const a = {};
    for (const x of tanimlar) a[x.k] = surum != null && x.yeni && surum < x.yeni && x.eskiDeger !== undefined ? x.eskiDeger : x.deger;
    if (kayit) Object.assign(a, kayit);
    return a;
  }
  const surumu = s => (typeof s.surum === 'number' ? s.surum : 1);

  function hazirla(m, s, onbellek) {
    const G = GOSTERGE();
    const hesaplanan = {}, ayarlar = {};
    const b = {
      t: m.t, o: m.o, h: m.h, l: m.l, c: m.c, v: m.v,
      n: m.c.length,
      f: (kok.Bicim && kok.Bicim.fiyat) || (x => (Math.round(x * 1e6) / 1e6).toString()),
    };
    b.ga = id => ayarlar[id] || (ayarlar[id] = ayarDegerleri(G[id].ayarlar, s.gostergeler && s.gostergeler[id]));
    b.g = id => {
      if (hesaplanan[id]) return hesaplanan[id];
      const tanim = G[id], a = b.ga(id);
      const anahtar = id + '|' + (tanim.anahtar ? tanim.anahtar(a) : JSON.stringify(a));
      let r = onbellek && onbellek.get(anahtar);
      if (!r) { r = tanim.hesapla(m, a, kok.GOSTERGE_MATEMATIK); if (onbellek) onbellek.set(anahtar, r); }
      return (hesaplanan[id] = r);
    };
    Object.defineProperty(b, 'ort', { get: () => b.g('ortalama').ort });
    return b;
  }

  const acikMi = (kural, s) => (s.kurallar[kural.id] ? !!s.kurallar[kural.id].acik : !!kural.acik);
  const kopya = d => ({ asama: d.asama, enDusuk: d.enDusuk, dipBas: d.dipBas, giris: d.giris, girisIdx: d.girisIdx, enYuksek: d.enYuksek });

  /** Bir kuralın kullandığı göstergeler (kural ayarına göre değişebilir: ör. vites açıksa vites göstergesi) */
  function kuralGostergeleri(k, a) {
    return typeof k.gosterge === 'function' ? k.gosterge(a) : k.gosterge || [];
  }
  /** Açık kuralların kullandığı göstergeler */
  function kullanilanGostergeler(s, kurallar) {
    const set = new Set();
    for (const k of kurallar) {
      if (!acikMi(k, s)) continue;
      for (const g of kuralGostergeleri(k, ayarDegerleri(k.ayarlar, s.kurallar[k.id] && s.kurallar[k.id].d, surumu(s)))) set.add(g);
    }
    return set;
  }

  /**
   * m: { t[], o[], h[], l[], c[], v[], acikSon }  (acikSon: son mum henüz kapanmadı)
   * s: strateji, kurallar: KURALLAR listesi
   * sec: { onbellek: Map (göstergeleri tekrar hesaplamamak için), hafif: true (sadece sonuç, açıklama yok) }
   */
  function calistir(m, s, kurallar, sec) {
    sec = sec || {};
    const hafif = !!sec.hafif;
    const b = hazirla(m, s, sec.onbellek);
    const n = b.n;
    const mf = Math.max(0, s.para.masraf) / 100;
    const bas = s.para.baslangic;

    const A = {};
    for (const k of kurallar) A[k.id] = ayarDegerleri(k.ayarlar, s.kurallar[k.id] && s.kurallar[k.id].d, surumu(s));
    const aktif = kurallar.filter(k => acikMi(k, s));
    const grup = yer => aktif.filter(k => k.yer === yer);
    const baslatK = grup('baslat'), tetikK = grup('tetik'), vazgecK = grup('vazgec');
    const devamK = baslatK.filter(k => typeof k.devam === 'function'); // dip ararken almadan önce tekrar bakılan başlatma kuralları
    const satK = grup('sat').sort((x, y) => (x.oncelik || 5) - (y.oncelik || 5));

    // göstergeler oluşana kadar karar yok
    const G = GOSTERGE();
    let isinma = 1;
    for (const id of kullanilanGostergeler(s, kurallar)) isinma = Math.max(isinma, G[id].isinma(b.ga(id)));
    const i0 = Math.min(n, isinma);
    const son = m.acikSon ? n - 2 : n - 1; // kapanmamış mumla karar verilmez

    let nakit = bas, adet = 0, alTutAdet = 0;
    let nakit0 = bas, adet0 = 0; // masraf hiç olmasaydı
    const d = { asama: 'bekle', enDusuk: null, dipBas: null, giris: null, girisIdx: null, enYuksek: null };
    const ozk = new Array(n), ozk0 = new Array(n), alTut = new Array(n);
    const log = hafif ? null : new Array(n);
    const islemler = [], dipler = [];
    let acikIslem = null, acikDip = null;

    function al(i, fiyat, kontroller) {
      const harca = nakit;
      const masraf = harca * mf;
      adet = (harca - masraf) / fiyat;
      nakit = 0;
      adet0 = nakit0 / fiyat; nakit0 = 0;
      if (acikDip) { acikDip.bit = i; acikDip.sonuc = 'al'; }
      acikIslem = {
        no: islemler.length + 1,
        dipBas: acikDip ? acikDip.bas : i,
        baslatKontrol: acikDip ? acikDip.kontrol : [],
        tetikKontrol: kontroller,
        enDusuk: d.enDusuk,
        girisIdx: i, giris: fiyat, harca, girisMasraf: masraf, adet,
        cikisIdx: null, cikis: null, cikisMasraf: 0, cikisKural: null, cikisNot: '', tepe: null,
      };
      islemler.push(acikIslem);
      acikDip = null;
      d.asama = 'coinde'; d.giris = fiyat; d.girisIdx = i; d.enYuksek = fiyat; d.enDusuk = null; d.dipBas = null;
      return { tip: 'al', fiyat, islem: acikIslem.no };
    }

    function sat(i, fiyat, kural, sonuc) {
      const t = acikIslem;
      const gelir = adet * fiyat;
      const masraf = gelir * mf;
      nakit = gelir - masraf;
      nakit0 = adet0 * fiyat; adet0 = 0;
      t.cikisIdx = i; t.cikis = fiyat; t.cikisMasraf = masraf;
      t.cikisKural = kural.id; t.cikisNot = sonuc.not; t.tepe = d.enYuksek;
      t.brut = t.adet * (fiyat - t.giris);
      t.net = nakit - t.harca;
      t.yuzde = (t.net / t.harca) * 100;
      adet = 0;
      acikIslem = null;
      d.asama = 'bekle'; d.giris = null; d.girisIdx = null; d.enYuksek = null;
      return { tip: 'sat', fiyat, kural: kural.id, islem: t.no };
    }

    for (let i = 0; i < n; i++) {
      if (i < i0) {
        ozk[i] = bas; ozk0[i] = bas; alTut[i] = bas;
        if (log) log[i] = { a: 'isinma', once: null, sonra: null, k: [], olay: null };
        continue;
      }
      if (i === i0) alTutAdet = (bas * (1 - mf)) / b.c[i];

      const once = log ? kopya(d) : null;
      const kontroller = [];
      let olay = null;

      if (i <= son) {
        if (d.asama === 'coinde') {
          d.enYuksek = Math.max(d.enYuksek, b.h[i]);
          let ilk = null;
          for (const k of satK) {
            const r = k.kontrol(b, i, A[k.id], d);
            if (log) kontroller.push({ id: k.id, ok: !!r.ok, not: r.not });
            if (r.ok && !ilk) { ilk = { k, r }; if (!log) break; }
          }
          if (ilk) olay = sat(i, ilk.r.fiyat != null ? ilk.r.fiyat : b.c[i], ilk.k, ilk.r);
        } else if (d.asama === 'dipAra') {
          d.enDusuk = Math.min(d.enDusuk, b.l[i]);
          let vazgecti = null;
          for (const k of vazgecK) {
            const r = k.kontrol(b, i, A[k.id], d);
            if (log) kontroller.push({ id: k.id, ok: !!r.ok, not: r.not, vazgec: true });
            if (r.ok && !vazgecti) { vazgecti = { k, r }; if (!log) break; }
          }
          if (vazgecti) {
            olay = { tip: 'iptal', kural: vazgecti.k.id, not: vazgecti.r.not };
            acikDip.bit = i; acikDip.sonuc = 'iptal'; acikDip = null;
            d.asama = 'bekle'; d.enDusuk = null; d.dipBas = null;
          } else {
            let hepsi = true;
            for (const k of devamK) {
              const r = k.devam(b, i, A[k.id], d);
              if (!r) continue;
              if (log) kontroller.push({ id: k.id, ok: !!r.ok, not: r.not, devam: true });
              if (!r.ok) { hepsi = false; if (!log) break; }
            }
            if (hepsi || log) {
              for (const k of tetikK) {
                const r = k.kontrol(b, i, A[k.id], d);
                if (log) kontroller.push({ id: k.id, ok: !!r.ok, not: r.not });
                if (!r.ok) { hepsi = false; if (!log) break; }
              }
            }
            if (hepsi) olay = al(i, b.c[i], log ? kontroller.filter(x => !x.vazgec) : []);
          }
        } else {
          let hepsi = true;
          for (const k of baslatK) {
            const r = k.kontrol(b, i, A[k.id], d);
            if (log) kontroller.push({ id: k.id, ok: !!r.ok, not: r.not });
            if (!r.ok) { hepsi = false; if (!log) break; }
          }
          if (hepsi) {
            d.asama = 'dipAra'; d.enDusuk = b.l[i]; d.dipBas = i;
            acikDip = { bas: i, bit: null, sonuc: 'devam', kontrol: log ? kontroller.slice() : [] };
            dipler.push(acikDip);
            olay = { tip: 'basla' };
            if (tetikK.length === 0) olay = al(i, b.c[i], []);
          }
        }
      }

      ozk[i] = nakit + adet * b.c[i];
      ozk0[i] = nakit0 + adet0 * b.c[i];
      alTut[i] = alTutAdet * b.c[i];
      if (log) log[i] = { a: once.asama, once, sonra: kopya(d), k: kontroller, olay, kapanmadi: i > son };
    }

    const cizgiler = [];
    if (!hafif) {
      for (const k of aktif) {
        if (!k.cizgi) continue;
        const c = k.cizgi(b, A[k.id]);
        (Array.isArray(c) ? c : [c]).forEach((x, j) => cizgiler.push(Object.assign({ id: k.id + '_' + j }, x)));
      }
    }
    const kapali = islemler.filter(t => t.cikisIdx != null).length;
    return { b, A, i0, isinma, son, n, islemler, kapaliIslem: kapali, dipler, ozk, ozk0, alTut, log, ort: b.ort, cizgiler, aktif };
  }

  /** Botun o andaki durumuna göre beklediği fiyat çizgileri */
  function seviyeler(R, i, d) {
    if (!d) return [];
    const yerler = d.asama === 'bekle' ? ['baslat'] : d.asama === 'dipAra' ? ['tetik', 'vazgec'] : ['sat'];
    const out = [];
    if (d.asama === 'dipAra') {
      for (const k of R.aktif) if (k.yer === 'baslat' && k.devamSeviye) for (const s of k.devamSeviye(R.b, i, R.A[k.id], d)) out.push(Object.assign({ id: k.id }, s));
    }
    for (const k of R.aktif) if (yerler.includes(k.yer) && k.seviye) for (const s of k.seviye(R.b, i, R.A[k.id], d)) out.push(Object.assign({ id: k.id }, s));
    return out;
  }

  /**
   * "En kârlı ayarı bul"un deneyebileceği ayarlar: açık kuralların görünen ayarları ve onların kullandığı
   * göstergelerin ayarları. Açma-kapama anahtarları (hangi kuralın/modun açık olacağı) kullanıcıya bırakılır.
   */
  function ayarlanabilirler(s, kurallar) {
    const GT = GOSTERGE(), out = [];
    const uygun = (a, deger) => a.tip !== 'anahtar' && (!a.goster || a.goster(deger));
    for (const id of kullanilanGostergeler(s, kurallar)) {
      const d = ayarDegerleri(GT[id].ayarlar, s.gostergeler && s.gostergeler[id]);
      for (const a of GT[id].ayarlar) if (uygun(a, d)) out.push({ yol: ['g', id, a.k], a });
    }
    for (const k of kurallar) {
      if (!acikMi(k, s)) continue;
      const d = ayarDegerleri(k.ayarlar, s.kurallar[k.id] && s.kurallar[k.id].d, surumu(s));
      for (const a of k.ayarlar) if (uygun(a, d)) out.push({ yol: ['k', k.id, a.k], a });
    }
    return out;
  }

  /** Her mumun vites numarası (0 sert düşüş … 4 sert yükseliş, -1 hesaplanamadı) */
  function vitesDurumlari(R) {
    const G = R.b.g('vites'), a = R.b.ga('vites');
    return G.egim.map(e => kok.VITES.sinif(e, a));
  }

  /** Bir göstergenin grafikte çizilecek çizgileri */
  function gostergeCizgileri(R, id) {
    const tanim = GOSTERGE()[id];
    return tanim.cizgiler(R.b.g(id), R.b.ga(id));
  }

  /**
   * Başka bir program için: son KAPANMIŞ mumda bot ne yapardı?
   * mumlar: [[acilisZamaniMs, acilis, yuksek, dusuk, kapanis, hacim], ...]  (eskiden yeniye)
   * strateji: dışa aktarılan dosyadaki "strateji" nesnesi
   * döner: { asama: 'bekle'|'dipAra'|'coinde', olay: null|{tip:'al'|'sat'|'basla'|'iptal', fiyat?}, mumZamani }
   */
  function sonKarar(mumlar, strateji, kapanmamisSonMumVar) {
    const m = { t: [], o: [], h: [], l: [], c: [], v: [], acikSon: !!kapanmamisSonMumVar };
    for (const x of mumlar) { m.t.push(x[0]); m.o.push(x[1]); m.h.push(x[2]); m.l.push(x[3]); m.c.push(x[4]); m.v.push(x[5]); }
    const R = calistir(m, strateji, kok.KURALLAR);
    const i = R.son;
    const lg = R.log[i];
    return { asama: lg && lg.sonra ? lg.sonra.asama : 'bekle', olay: lg ? lg.olay : null, mumZamani: m.t[i] };
  }

  kok.Motor = { calistir, seviyeler, gostergeCizgileri, kullanilanGostergeler, kuralGostergeleri, ayarDegerleri, ayarlanabilirler, vitesDurumlari, sonKarar };
})(typeof window !== 'undefined' ? window : globalThis);
