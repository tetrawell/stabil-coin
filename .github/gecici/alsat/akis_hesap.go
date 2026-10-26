package main

// AKIŞ ONAYI — HESAP (0.9). Simülatörün motoru "al" dediğinde dört ayrı gözle bakılır ve tek soru sorulur:
//
//     "Satış bitti mi, alıcı geldi mi?"   (motor bir kırılımda alıyorsa: "Alıcılar gerçek mi?")
//
// Simülatörün stratejileri çoğunlukla düşüşte alır (bant altı, ortalama altı, dipten dönüş). Böyle bir alımı en çok
// kaybettiren şey, süren bir satışın ortasında almaktır. Dört araç bu soruyu farklı yerden ve farklı zaman ölçeğinde
// cevaplar; her biri −100..+100 puan verir, maddeleri ekranda tek tek görünür:
//
//  1. YAPI (Smart Money Concepts) — "Nerede?" Mumlardan: ana ve iç yapı (salınım tepe/dipleri, BOS = yapı kırılımı,
//     CHoCH = karakter değişimi), talep/arz bölgeleri (order block), boşluklar (FVG), eşit tepe/dipler (likidite),
//     likidite süpürmesi (fitil eski dibin altına inip kapanış geri üstte), indirim/pahalı bölge (aralığın yarısı).
//     Algoritmalar LuxAlgo'nun "Smart Money Concepts" göstergesindeki yaygın tanımlara göre (iç yapı 5, ana yapı 50
//     mum; eşitlik ATR'nin 0,1 katı) ve geleceğe bakmadan (salınım, uzunluk kadar mum sonra kesinleşir).
//  2. EMİR AKIŞI (Order Flow) — "Şu an kim saldırıyor?" Spot işlemlerden: mumun deltası (saldırgan alım − satım),
//     emilim (yüksek hacimli satış fiyatı düşüremedi), delta dönüşü, footprint'te üst üste dengesizlik (çapraz
//     karşılaştırma: bir kademedeki alım, bir alt kademedeki satışın 3 katı; 3 kademe üst üste), büyük işlemler,
//     şu anki saldırı, defter eğilimi.
//  3. CVD (birikimli hacim deltası) — "Baskı zayıflıyor mu?" Eğilim, fiyatla uyumsuzluk (fiyat yeni dip yaptı ama
//     CVD yapmadı), spot ile vadeli farkı (hareketi spot alıcılar mı, kaldıraçlı vadeli mi sürüklüyor).
//  4. AÇIK POZİSYON (Open Interest, vadeli) — "Zorunlu satış bitti mi?" Fiyat ve OI'nin birlikte yönü
//     (düşüşte OI azaldı = pozisyonlar kapandı/temizlendi; düşüşte OI arttı = yeni açığa satışlar), ani temizlik,
//     fonlama (long'lar ya da short'lar kalabalık mı).
//
// Toplam = ağırlıklı ortalama (verisi olan bloklar). Onay: toplam ≥ eşik ve yasak yok (ana yapı az önce aşağı
// kırıldıysa ya da satış saldırısı sürüyorsa). Bütün sayılar ayarlanabilir (akisAyar); varsayılanlar yaygın
// kullanılan değerler ve ilk tahmindir — kullanıcı "Onay karnesi"ne bakarak ayarlayacak. Bu sistemin kâr
// getirdiği kanıtlanmış değildir.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---------- ayarlar ----------

type akisAyar struct {
	Acik         bool       `json:"acik"`         // kumbaralara onay uygulansın mı (kumbara başına da kapatılabilir)
	Esik         float64    `json:"esik"`         // onay için en az toplam puan (−100..100)
	Agirlik      [4]float64 `json:"agirlik"`      // Yapı, Emir akışı, CVD, Açık pozisyon (0–3)
	BeklemeMum   int        `json:"beklemeMum"`   // onay o an gelmezse kaç mum daha beklenir (0–10)
	Kovalama     float64    `json:"kovalama"`     // beklerken motorun fiyatından en fazla % kaç yukarıda alınır
	AkisSart     bool       `json:"akisSart"`     // emir akışı verisi yoksa onay verme
	YasakYapi    bool       `json:"yasakYapi"`    // ana yapı az önce aşağı kırıldıysa onay verme
	YasakSaldiri bool       `json:"yasakSaldiri"` // satış saldırısı sürüyorsa onay verme

	IcUzunluk  int     `json:"icUzunluk"`  // iç yapı salınım uzunluğu (mum)
	AnaUzunluk int     `json:"anaUzunluk"` // ana yapı salınım uzunluğu
	EsitHassas float64 `json:"esitHassas"` // eşit tepe/dip: fark ATR'nin bu katından az
	SupurmeMum int     `json:"supurmeMum"` // süpürme kaç mum boyunca geçerli
	FvgEsik    float64 `json:"fvgEsik"`    // FVG en az ATR'nin bu katı

	DengesizlikOran   float64 `json:"dengesizlikOran"`   // çapraz dengesizlik oranı (3 = %300)
	DengesizlikKademe int     `json:"dengesizlikKademe"` // kaç kademe üst üste
	KademeSayisi      int     `json:"kademeSayisi"`      // ortalama mum boyu kaç kademeye bölünür
	EmilimHacim       float64 `json:"emilimHacim"`       // emilim için hacim, ortalamanın bu katı

	CvdBakis int `json:"cvdBakis"` // CVD kaç mumluk eğilime bakar

	OiBakis   int     `json:"oiBakis"`   // açık pozisyon kaç muma bakar
	OiEsik    float64 `json:"oiEsik"`    // anlamlı OI değişimi (%)
	FonYuksek float64 `json:"fonYuksek"` // yüksek fonlama (%, 8 saatlik)

	Degisim int64 `json:"degisim"` // ayarların (açık/kapalı dışında) en son değiştiği an; karnede "bu ayarlarla"
}

func akisVarsayilan() akisAyar {
	return akisAyar{
		Acik: true, Esik: 20, Agirlik: [4]float64{1, 1, 1, 1}, BeklemeMum: 2, Kovalama: 0.3,
		AkisSart: true, YasakYapi: true, YasakSaldiri: true,
		IcUzunluk: 5, AnaUzunluk: 50, EsitHassas: 0.1, SupurmeMum: 10, FvgEsik: 0.1,
		DengesizlikOran: 3, DengesizlikKademe: 3, KademeSayisi: 8, EmilimHacim: 1.5,
		CvdBakis: 20, OiBakis: 20, OiEsik: 0.5, FonYuksek: 0.05,
	}
}

func sinirla(x, alt, ust float64) float64 { return math.Max(alt, math.Min(ust, x)) }
func sinirlaI(x, alt, ust int) int        { return max(alt, min(ust, x)) }

// akisDuzelt: sınır dışı ya da eksik değerleri düzeltir (dosyadan gelen ayar sayfaya ve hesaba güvenle girsin).
func akisDuzelt(a akisAyar) akisAyar {
	v := akisVarsayilan()
	sayiMi := func(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
	if !sayiMi(a.Esik) {
		a.Esik = v.Esik
	}
	a.Esik = sinirla(a.Esik, -100, 100)
	for i := range a.Agirlik {
		if !sayiMi(a.Agirlik[i]) {
			a.Agirlik[i] = 1
		}
		a.Agirlik[i] = sinirla(a.Agirlik[i], 0, 3)
	}
	a.BeklemeMum = sinirlaI(a.BeklemeMum, 0, 10)
	if !sayiMi(a.Kovalama) {
		a.Kovalama = v.Kovalama
	}
	a.Kovalama = sinirla(a.Kovalama, 0, 3)
	duzI := func(x *int, vs, alt, ust int) {
		if *x == 0 {
			*x = vs
		}
		*x = sinirlaI(*x, alt, ust)
	}
	duzF := func(x *float64, vs, alt, ust float64) {
		if *x == 0 || !sayiMi(*x) {
			*x = vs
		}
		*x = sinirla(*x, alt, ust)
	}
	duzI(&a.IcUzunluk, v.IcUzunluk, 2, 20)
	duzI(&a.AnaUzunluk, v.AnaUzunluk, 5, 200)
	if a.AnaUzunluk <= a.IcUzunluk {
		a.AnaUzunluk = min(200, a.IcUzunluk*3)
	}
	duzF(&a.EsitHassas, v.EsitHassas, 0.01, 1)
	duzI(&a.SupurmeMum, v.SupurmeMum, 1, 50)
	duzF(&a.FvgEsik, v.FvgEsik, 0.01, 2)
	duzF(&a.DengesizlikOran, v.DengesizlikOran, 1.5, 10)
	duzI(&a.DengesizlikKademe, v.DengesizlikKademe, 2, 8)
	duzI(&a.KademeSayisi, v.KademeSayisi, 3, 30)
	duzF(&a.EmilimHacim, v.EmilimHacim, 1, 5)
	duzI(&a.CvdBakis, v.CvdBakis, 5, 100)
	duzI(&a.OiBakis, v.OiBakis, 5, 100)
	duzF(&a.OiEsik, v.OiEsik, 0.05, 10)
	duzF(&a.FonYuksek, v.FonYuksek, 0.005, 1)
	return a
}

var akisAyarDegeri atomic.Value // akisAyar

func akisAyarOku(m map[string]json.RawMessage) akisAyar {
	a := akisVarsayilan()
	if v, ok := m["akis"]; ok {
		_ = json.Unmarshal(v, &a)
	}
	return akisDuzelt(a)
}

func akisAyarYukle() {
	ayarMu.Lock()
	m := ayarlariOku()
	ayarMu.Unlock()
	akisAyarDegeri.Store(akisAyarOku(m))
}

func akisAyarAl() akisAyar {
	if a, ok := akisAyarDegeri.Load().(akisAyar); ok {
		return a
	}
	return akisVarsayilan()
}

// durumMu kilitliyken ÇAĞRILMAZ
func akisAyarYaz(a akisAyar) error {
	a = akisDuzelt(a)
	ayarMu.Lock()
	defer ayarMu.Unlock()
	m := ayarlariOku()
	b, _ := json.Marshal(a)
	m["akis"] = b
	if err := ayarlariYaz(m); err != nil {
		return err
	}
	akisAyarDegeri.Store(a)
	return nil
}

// ---------- mum önbelleği (yapı hesabı için; sadece kapanmış mumlar) ----------

type akisMumKaydi struct {
	mumlar []mumOHLC
	deneme time.Time
	hata   string
}

type mumOHLC struct {
	Z          int64
	O, H, L, C float64
}

var (
	akisMumMu sync.Mutex
	akisMumlr = map[string]*akisMumKaydi{}
)

const akisMumAdet = 600

// akisMumlari: coinin bu mum süresindeki son ~600 KAPANMIŞ mumu. Yeni mum kapanmışsa en yeni sayfa eklenir.
func akisMumlari(sembol, aralik string) ([]mumOHLC, error) {
	ar, ok := araliklar[aralik]
	if !ok {
		return nil, fmt.Errorf("mum süresi geçersiz")
	}
	anahtar := sembol + "|" + aralik
	simdi := akisSimdi()
	sonKapanan := (simdi-1000)/ar.ms*ar.ms - ar.ms // kapanmış en yeni mumun açılışı
	akisMumMu.Lock()
	k := akisMumlr[anahtar]
	if k == nil {
		k = &akisMumKaydi{}
		akisMumlr[anahtar] = k
	}
	if n := len(k.mumlar); n > 0 && k.mumlar[n-1].Z >= sonKapanan {
		out := k.mumlar
		akisMumMu.Unlock()
		return out, nil
	}
	if time.Since(k.deneme) < 3*time.Second {
		out := k.mumlar
		akisMumMu.Unlock()
		if len(out) == 0 {
			return nil, fmt.Errorf("mumlar alınamadı: %s", k.hata)
		}
		return out, nil
	}
	k.deneme = time.Now()
	ilk := len(k.mumlar) == 0
	akisMumMu.Unlock()

	var yeni []mumOHLC
	var err error
	if testModu {
		for _, x := range testMumlari(sembol, aralik, akisMumAdet+1) {
			yeni = append(yeni, mumOHLC{int64(x[0]), x[1], x[2], x[3], x[4]})
		}
	} else {
		var ham [][]float64
		if ilk {
			ham, _, err = mumlariTopla("mexc", sembol, aralik, akisMumAdet, 0)
		} else {
			ham, _, err = mumSayfasi("mexc", sembol, aralik, 10, 0, 0)
		}
		for _, x := range ham {
			if len(x) >= 7 && int64(x[6]) <= simdi-1000 { // sadece kapanmış mumlar
				yeni = append(yeni, mumOHLC{int64(x[0]), x[1], x[2], x[3], x[4]})
			}
		}
	}
	akisMumMu.Lock()
	defer akisMumMu.Unlock()
	if err != nil && len(k.mumlar) == 0 {
		k.hata = err.Error()
		return nil, err
	}
	// birleştir (aynı açılış zamanı → yenisi), sırala, sadece kapanmışlar, en fazla 800
	m := map[int64]mumOHLC{}
	for _, x := range k.mumlar {
		m[x.Z] = x
	}
	for _, x := range yeni {
		if x.Z <= sonKapanan {
			m[x.Z] = x
		}
	}
	out := make([]mumOHLC, 0, len(m))
	for _, x := range m {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Z < out[j].Z })
	if len(out) > 800 {
		out = out[len(out)-800:]
	}
	k.mumlar, k.hata = out, ""
	return out, nil
}

// ---------- mum süresine göre akış (dakikalardan) ----------

type akisMum struct {
	Var       bool    // bu mum için akış verisi var
	Eksik     bool    // mumun bir kısmında veri yoktu
	Al, Sat   float64 // spot saldırgan alım/satım (coin)
	VAl, VSat float64 // vadeli
	VVOran    float64 // vadeli verisi olan dakikaların payı
	BAlT      float64 // büyük alımlar (USDT)
	BSatT     float64
	OIBas     float64 // mumun başındaki OI (ilk dakikanın en düşüğü değil, açılış değeri yaklaşığı)
	OISon     float64 // mumun sonunda
	OIA, OIY  float64
	Fon       float64
	DE        float64
	DESay     int
	Iz        map[int32][2]float64 // 1 baz puanlık kovalar
	Dakika    int
	Sure      int64 // verisi olan süre (ms); süren mumda geçen süre
}

func (m *akisMum) hacim() float64 { return m.Al + m.Sat }
func (m *akisMum) delta() float64 { return m.Al - m.Sat }
func (m *akisMum) oran() float64 {
	if h := m.hacim(); h > 0 {
		return m.delta() / h
	}
	return 0
}

// akisMumlaraTopla: dakika çubuklarını mumlara dağıtır (zamanlar mumların açılışı; son eleman süren mum olabilir).
func akisMumlaraTopla(zamanlar []int64, ms int64, dakikalar []akisDakika) []akisMum {
	out := make([]akisMum, len(zamanlar))
	j := 0
	sonOI := 0.0 // önceki dakikanın sonundaki OI: mumun başlangıç değeri
	for i, z := range zamanlar {
		m := &out[i]
		beklenen := int(ms / 60_000)
		for j < len(dakikalar) && dakikalar[j].Z < z {
			j++
		}
		k := j
		vv := 0
		for k < len(dakikalar) && dakikalar[k].Z < z+ms {
			d := &dakikalar[k]
			m.Var = true
			m.Dakika++
			if d.Eksik {
				m.Eksik = true
			}
			m.Al += d.Al
			m.Sat += d.Sat
			if d.VV {
				vv++
				m.VAl += d.VAl
				m.VSat += d.VSat
				if d.OI > 0 {
					if m.OIBas == 0 {
						m.OIBas = sonOI
						if m.OIBas == 0 {
							m.OIBas = d.OIA
						}
						if m.OIBas == 0 {
							m.OIBas = d.OI
						}
					}
					m.OISon = d.OI
					if m.OIA == 0 || d.OIA < m.OIA {
						m.OIA = d.OIA
					}
					m.OIY = math.Max(m.OIY, d.OIY)
				}
				m.Fon = d.Fon
			}
			if d.VV && d.OI > 0 {
				sonOI = d.OI
			} else {
				sonOI = 0
			}
			if d.K > 0 {
				m.BAlT += d.BAl * d.K
				m.BSatT += d.BSat * d.K
			}
			if d.DE != 0 {
				m.DE += d.DE
				m.DESay++
			}
			if len(d.Iz) > 0 {
				if m.Iz == nil {
					m.Iz = map[int32][2]float64{}
				}
				for _, x := range d.Iz {
					v := m.Iz[int32(x[0])]
					v[0] += x[1]
					v[1] += x[2]
					m.Iz[int32(x[0])] = v
				}
			}
			k++
		}
		if m.Dakika < beklenen && i < len(zamanlar)-1 {
			m.Eksik = true // aradaki dakikalar yok (bağlantı yoktu ya da program kapalıydı)
		}
		if m.Dakika > 0 {
			m.VVOran = float64(vv) / float64(m.Dakika)
			m.Sure = int64(m.Dakika) * 60_000
		}
		if m.DESay > 0 {
			m.DE /= float64(m.DESay)
		}
		j = k
	}
	return out
}

// ---------- yapı (Smart Money Concepts) ----------

type smcSalinim struct {
	I      int     // salınımın mumu
	Onay   int     // kesinleştiği mum (I + uzunluk)
	F      float64 // fiyat
	Tepe   bool
	Etiket string // HH, LH, HL, LL
	Ic     bool
}

type smcKirilim struct {
	I0    int // kırılan salınımın mumu
	I     int // kırıldığı mum
	F     float64
	Yon   int // +1 yukarı, −1 aşağı
	Choch bool
	Ic    bool
}

type smcBolge struct {
	I0   int // bölgenin mumu
	Olus int // oluştuğu (bilindiği) mum
	I1   int // geçersiz olduğu mum (−1: hâlâ geçerli)
	Ust  float64
	Alt  float64
	Yon  int    // +1 talep (alıcı), −1 arz (satıcı)
	Tur  string // OB | FVG
	Ic   bool
}

type smcEsit struct {
	I0, I1  int
	F       float64
	Tepe    bool // EQH (tepe = alış tarafı likidite), değilse EQL
	Supurme int  // süpürüldüğü mum (−1: süpürülmedi)
}

type smcSupurme struct {
	I      int
	F      float64 // süpürülen seviye
	Yon    int     // +1: satış tarafı (dip altı) süpürüldü → yukarı dönüş beklenir; −1: tepe üstü
	Kaynak string
	Guclu  bool // ana yapı ya da eşit dipler/tepeler
}

type smcSonuc struct {
	ATR        []float64
	Salinimlar []smcSalinim
	Kirilimlar []smcKirilim
	Bolgeler   []smcBolge
	Esitler    []smcEsit
	Supurmeler []smcSupurme
	AnaTrend   int
	IcTrend    int
	AralikUst  float64
	AralikAlt  float64
	AralikBas  int
}

func atrHesapla(m []mumOHLC, n int) []float64 {
	out := make([]float64, len(m))
	var atr float64
	for i := range m {
		tr := m[i].H - m[i].L
		if i > 0 {
			tr = math.Max(tr, math.Max(math.Abs(m[i].H-m[i-1].C), math.Abs(m[i].L-m[i-1].C)))
		}
		if i < n {
			atr = (atr*float64(i) + tr) / float64(i+1)
		} else {
			atr = (atr*float64(n-1) + tr) / float64(n)
		}
		out[i] = atr
	}
	return out
}

// salinimlar: LuxAlgo'nun swings(len) mantığı. Mum c, sonraki len mumun hepsinden yüksekse tepe (düşükse dip);
// tepe ve dip sırayla gelir. c + len'de kesinleşir (geleceğe bakmaz: o ana kadar bilinmez).
func salinimlar(m []mumOHLC, uz int, ic bool) []smcSalinim {
	var out []smcSalinim
	os := -1
	for t := uz; t < len(m); t++ {
		c := t - uz
		ust, alt := math.Inf(-1), math.Inf(1)
		for j := c + 1; j <= t; j++ {
			ust = math.Max(ust, m[j].H)
			alt = math.Min(alt, m[j].L)
		}
		yeni := os
		if m[c].H > ust {
			yeni = 0
		} else if m[c].L < alt {
			yeni = 1
		}
		if yeni == 0 && os != 0 {
			out = append(out, smcSalinim{I: c, Onay: t, F: m[c].H, Tepe: true, Ic: ic})
		} else if yeni == 1 && os != 1 {
			out = append(out, smcSalinim{I: c, Onay: t, F: m[c].L, Tepe: false, Ic: ic})
		}
		os = yeni
	}
	// etiketler: önceki aynı türle karşılaştır
	var sonT, sonD *smcSalinim
	for i := range out {
		s := &out[i]
		if s.Tepe {
			if sonT == nil || s.F > sonT.F {
				s.Etiket = "HH"
			} else {
				s.Etiket = "LH"
			}
			sonT = s
		} else {
			if sonD == nil || s.F >= sonD.F {
				s.Etiket = "HL"
			} else {
				s.Etiket = "LL"
			}
			sonD = s
		}
	}
	return out
}

// yapiHesapla: kırılımlar (BOS/CHoCH), order block'lar ve trend; salınımlar kesinleştiği mumdan itibaren kullanılır.
func yapiHesapla(m []mumOHLC, sal []smcSalinim, atr []float64, ic bool) (kir []smcKirilim, ob []smcBolge, trend int) {
	type aktif struct {
		s       smcSalinim
		kirildi bool
		var_    bool
	}
	var tepe, dip aktif
	si := 0
	for i := range m {
		for si < len(sal) && sal[si].Onay <= i {
			if sal[si].Tepe {
				tepe = aktif{s: sal[si], var_: true}
			} else {
				dip = aktif{s: sal[si], var_: true}
			}
			si++
		}
		if tepe.var_ && !tepe.kirildi && m[i].C > tepe.s.F && i > tepe.s.I {
			kir = append(kir, smcKirilim{I0: tepe.s.I, I: i, F: tepe.s.F, Yon: 1, Choch: trend == -1, Ic: ic})
			trend = 1
			tepe.kirildi = true
			// talep bölgesi: kırılan tepeden kırılıma kadarki en düşük mum
			k := i - 1
			for j := tepe.s.I + 1; j < i; j++ {
				if m[j].L < m[k].L {
					k = j
				}
			}
			if k >= 0 {
				alt, ust := m[k].L, m[k].H
				if ust-alt > 2*atr[k] {
					ust = alt + atr[k] // dev mumun tamamı bölge sayılmaz
				}
				ob = append(ob, smcBolge{I0: k, Olus: i, I1: -1, Ust: ust, Alt: alt, Yon: 1, Tur: "OB", Ic: ic})
			}
		}
		if dip.var_ && !dip.kirildi && m[i].C < dip.s.F && i > dip.s.I {
			kir = append(kir, smcKirilim{I0: dip.s.I, I: i, F: dip.s.F, Yon: -1, Choch: trend == 1, Ic: ic})
			trend = -1
			dip.kirildi = true
			k := i - 1
			for j := dip.s.I + 1; j < i; j++ {
				if m[j].H > m[k].H {
					k = j
				}
			}
			if k >= 0 {
				alt, ust := m[k].L, m[k].H
				if ust-alt > 2*atr[k] {
					alt = ust - atr[k]
				}
				ob = append(ob, smcBolge{I0: k, Olus: i, I1: -1, Ust: ust, Alt: alt, Yon: -1, Tur: "OB", Ic: ic})
			}
		}
		// geçersiz olan bölgeler: talep bölgesinin altında kapanış, arz bölgesinin üstünde kapanış
		for b := range ob {
			x := &ob[b]
			if x.I1 >= 0 || i <= x.Olus {
				continue
			}
			if (x.Yon == 1 && m[i].C < x.Alt) || (x.Yon == -1 && m[i].C > x.Ust) {
				x.I1 = i
			}
		}
	}
	return kir, ob, trend
}

func fvgHesapla(m []mumOHLC, atr []float64, esik float64) []smcBolge {
	var out []smcBolge
	for i := 2; i < len(m); i++ {
		if m[i].L > m[i-2].H && m[i-1].C > m[i-2].H && m[i].L-m[i-2].H >= esik*atr[i] {
			out = append(out, smcBolge{I0: i - 1, Olus: i, I1: -1, Ust: m[i].L, Alt: m[i-2].H, Yon: 1, Tur: "FVG"})
		}
		if m[i].H < m[i-2].L && m[i-1].C < m[i-2].L && m[i-2].L-m[i].H >= esik*atr[i] {
			out = append(out, smcBolge{I0: i - 1, Olus: i, I1: -1, Ust: m[i-2].L, Alt: m[i].H, Yon: -1, Tur: "FVG"})
		}
		for b := range out {
			x := &out[b]
			if x.I1 >= 0 || i <= x.Olus {
				continue
			}
			// boşluk tamamen dolduysa biter
			if (x.Yon == 1 && m[i].L < x.Alt) || (x.Yon == -1 && m[i].H > x.Ust) {
				x.I1 = i
			}
		}
	}
	return out
}

func smcHesapla(m []mumOHLC, a akisAyar) smcSonuc {
	var s smcSonuc
	if len(m) == 0 {
		return s
	}
	s.ATR = atrHesapla(m, 14)
	ic := salinimlar(m, a.IcUzunluk, true)
	ana := salinimlar(m, a.AnaUzunluk, false)
	s.Salinimlar = append(append(s.Salinimlar, ana...), ic...)
	k1, ob1, t1 := yapiHesapla(m, ic, s.ATR, true)
	k2, ob2, t2 := yapiHesapla(m, ana, s.ATR, false)
	s.Kirilimlar = append(k2, k1...)
	sort.SliceStable(s.Kirilimlar, func(i, j int) bool { return s.Kirilimlar[i].I < s.Kirilimlar[j].I })
	s.IcTrend, s.AnaTrend = t1, t2
	s.Bolgeler = append(append(ob2, ob1...), fvgHesapla(m, s.ATR, a.FvgEsik)...)

	// eşit tepe/dipler (3 mumluk salınımlarla) ve likidite süpürmeleri
	kucuk := salinimlar(m, 3, true)
	var sonT, sonD *smcSalinim
	for i := range kucuk {
		x := &kucuk[i]
		esik := a.EsitHassas * s.ATR[x.I]
		if x.Tepe {
			if sonT != nil && math.Abs(x.F-sonT.F) < esik {
				s.Esitler = append(s.Esitler, smcEsit{I0: sonT.I, I1: x.I, F: math.Max(x.F, sonT.F), Tepe: true, Supurme: -1})
			}
			sonT = x
		} else {
			if sonD != nil && math.Abs(x.F-sonD.F) < esik {
				s.Esitler = append(s.Esitler, smcEsit{I0: sonD.I, I1: x.I, F: math.Min(x.F, sonD.F), Tepe: false, Supurme: -1})
			}
			sonD = x
		}
	}
	// süpürme adayları: ana ve iç salınım dipleri/tepeleri + eşit seviyeler; her seviye bir kez süpürülür
	type seviye struct {
		f       float64
		gecerli int // bu mumdan itibaren bilinir
		dip     bool
		kaynak  string
		guclu   bool
		esit    int // s.Esitler içindeki sıra (−1 değil)
		bitti   bool
	}
	var sv []seviye
	for _, x := range ana {
		sv = append(sv, seviye{f: x.F, gecerli: x.Onay, dip: !x.Tepe, kaynak: map[bool]string{true: "ana tepe", false: "ana dip"}[x.Tepe], guclu: true, esit: -1})
	}
	for _, x := range ic {
		sv = append(sv, seviye{f: x.F, gecerli: x.Onay, dip: !x.Tepe, kaynak: map[bool]string{true: "iç tepe", false: "iç dip"}[x.Tepe], esit: -1})
	}
	for i, x := range s.Esitler {
		onay := x.I1 + 3
		sv = append(sv, seviye{f: x.F, gecerli: onay, dip: !x.Tepe, kaynak: map[bool]string{true: "eşit tepeler", false: "eşit dipler"}[x.Tepe], guclu: true, esit: i})
	}
	for i := range m {
		for j := range sv {
			v := &sv[j]
			if v.bitti || i <= v.gecerli {
				continue
			}
			if v.dip {
				if m[i].L < v.f {
					v.bitti = true
					geri := m[i].C > v.f
					if !geri && i+1 < len(m) && m[i+1].C > v.f && m[i+1].L >= m[i].L-s.ATR[i]*0.5 {
						// iki mumluk: kapanış altta, hemen ardından geri alındı
						geri = true
						i2 := i + 1
						s.Supurmeler = append(s.Supurmeler, smcSupurme{I: i2, F: v.f, Yon: 1, Kaynak: v.kaynak, Guclu: v.guclu})
					} else if geri {
						s.Supurmeler = append(s.Supurmeler, smcSupurme{I: i, F: v.f, Yon: 1, Kaynak: v.kaynak, Guclu: v.guclu})
					}
					if v.esit >= 0 {
						s.Esitler[v.esit].Supurme = i
					}
				}
			} else if m[i].H > v.f {
				v.bitti = true
				if m[i].C < v.f {
					s.Supurmeler = append(s.Supurmeler, smcSupurme{I: i, F: v.f, Yon: -1, Kaynak: v.kaynak, Guclu: v.guclu})
				}
				if v.esit >= 0 {
					s.Esitler[v.esit].Supurme = i
				}
			}
		}
	}
	sort.SliceStable(s.Supurmeler, func(i, j int) bool { return s.Supurmeler[i].I < s.Supurmeler[j].I })

	// aralık (indirim/pahalı): son ana tepe ile son ana dipten erken olanından bugüne en yüksek ve en düşük
	bas := max(0, len(m)-100)
	var sonAnaT, sonAnaD = -1, -1
	for _, x := range ana {
		if x.Onay >= len(m) {
			continue
		}
		if x.Tepe {
			sonAnaT = x.I
		} else {
			sonAnaD = x.I
		}
	}
	if sonAnaT >= 0 && sonAnaD >= 0 {
		bas = min(sonAnaT, sonAnaD)
	}
	s.AralikBas = bas
	s.AralikUst, s.AralikAlt = math.Inf(-1), math.Inf(1)
	for i := bas; i < len(m); i++ {
		s.AralikUst = math.Max(s.AralikUst, m[i].H)
		s.AralikAlt = math.Min(s.AralikAlt, m[i].L)
	}
	return s
}

// ---------- footprint ----------

type fpSatir struct {
	K       int32 // satır (kova / satırBoyu)
	Al, Sat float64
}

type fpSonuc struct {
	Satirlar   []fpSatir
	SatirBp    int32
	POC        float64 // en çok işlem gören fiyat
	AlimYigin  int     // en uzun üst üste alım dengesizliği (kademe)
	AlimYer    float64 // o yığının ortasının fiyatı
	SatimYigin int
	SatimYer   float64
}

func fpHesapla(iz map[int32][2]float64, satirBp int32, oran float64) fpSonuc {
	r := fpSonuc{SatirBp: satirBp}
	if len(iz) == 0 || satirBp < 1 {
		return r
	}
	sat := map[int32]*fpSatir{}
	var top float64
	for k, v := range iz {
		s := k / satirBp
		if k < 0 && k%satirBp != 0 {
			s-- // aşağı yuvarla
		}
		x := sat[s]
		if x == nil {
			x = &fpSatir{K: s}
			sat[s] = x
		}
		x.Al += v[0]
		x.Sat += v[1]
		top += v[0] + v[1]
	}
	for _, x := range sat {
		r.Satirlar = append(r.Satirlar, *x)
	}
	sort.Slice(r.Satirlar, func(i, j int) bool { return r.Satirlar[i].K < r.Satirlar[j].K })
	fiyat := func(s int32) float64 { return akisKovaFiyat(s*satirBp + satirBp/2) }
	enCok := -1.0
	for _, x := range r.Satirlar {
		if x.Al+x.Sat > enCok {
			enCok, r.POC = x.Al+x.Sat, fiyat(x.K)
		}
	}
	enAz := 0.02 * top // çok küçük kademeler dengesizlik sayılmaz
	// çapraz karşılaştırma: kademedeki alım ↔ bir alttaki satım; kademedeki satım ↔ bir üstteki alım
	al := func(s int32) float64 {
		if x := sat[s]; x != nil {
			return x.Al
		}
		return 0
	}
	st := func(s int32) float64 {
		if x := sat[s]; x != nil {
			return x.Sat
		}
		return 0
	}
	if len(r.Satirlar) == 0 {
		return r
	}
	ilk, son := r.Satirlar[0].K, r.Satirlar[len(r.Satirlar)-1].K
	yigin := 0
	// mumun en alt satırındaki alımın karşılaştırılacağı alt satır yok (en üstteki satımın da üst satırı): uçlar sayılmaz
	for s := ilk + 1; s <= son; s++ {
		a, b := al(s), st(s-1)
		if a >= enAz && a >= oran*b {
			yigin++
			if yigin > r.AlimYigin {
				r.AlimYigin, r.AlimYer = yigin, fiyat(s-int32(yigin-1)/2)
			}
		} else {
			yigin = 0
		}
	}
	yigin = 0
	for s := ilk; s <= son-1; s++ {
		a, b := st(s), al(s+1)
		if a >= enAz && a >= oran*b {
			yigin++
			if yigin > r.SatimYigin {
				r.SatimYigin, r.SatimYer = yigin, fiyat(s-int32(yigin-1)/2)
			}
		} else {
			yigin = 0
		}
	}
	return r
}

// ---------- puanlama ----------

type akisMadde struct {
	P float64 `json:"p"`
	M string  `json:"m"`
}

type akisBlok struct {
	Ad       string      `json:"ad"`
	Puan     float64     `json:"puan"`
	Var      bool        `json:"var"`
	Agirlik  float64     `json:"agirlik"`
	Neden    string      `json:"neden,omitempty"` // verisi yoksa neden
	Maddeler []akisMadde `json:"maddeler"`
}

func (b *akisBlok) ekle(p float64, m string) {
	if math.Abs(p) < 0.5 {
		return
	}
	b.Maddeler = append(b.Maddeler, akisMadde{P: math.Round(p), M: m})
}

func (b *akisBlok) topla() {
	var t float64
	for _, x := range b.Maddeler {
		t += x.P
	}
	b.Puan = math.Round(sinirla(t, -100, 100))
}

type akisSonuc struct {
	Sembol  string     `json:"sembol"`
	Aralik  string     `json:"aralik"`
	Z       int64      `json:"z"`   // hesap anı (MEXC saati)
	Mum     int64      `json:"mum"` // son kapanan mumun açılışı
	Fiyat   float64    `json:"fiyat"`
	Baglam  string     `json:"baglam"`
	Bloklar []akisBlok `json:"bloklar"`
	Toplam  float64    `json:"toplam"`
	Esik    float64    `json:"esik"`
	Onay    bool       `json:"onay"`
	Yasak   string     `json:"yasak,omitempty"`
	Eksik   string     `json:"eksik,omitempty"`
	Ozet    string     `json:"ozet"`
	Durum   string     `json:"durum"`
}

// akisHesapGirdi: bir hesabın bütün girdileri (testlerde doğrudan kurulur)
type akisHesapGirdi struct {
	Sembol, Aralik string
	Ms             int64
	Mumlar         []mumOHLC // kapanmış mumlar
	Veri           akisAnlikVeri
	Simdi          int64
	Ayar           akisAyar
}

type akisHesap struct {
	sonuc   akisSonuc
	smc     smcSonuc
	akis    []akisMum // len(Mumlar)+1: sonuncusu süren mum
	cvd     []float64
	vcvd    []float64
	fp      []fpSonuc // her mumun footprint özeti (sadece son ~150 mum)
	satirBp int32
	mumlar  []mumOHLC     // hesaba giren kapanmış mumlar (grafik için)
	veri    akisAnlikVeri // hesaba giren akış verisi (grafik için)
}

func nanMi(x float64) bool { return math.IsNaN(x) }

func ondalik1(x float64) string { return trSayi(x, 1) }

// isaretli: +%12,3 / −%4,5 (yüzde olarak verilen sayı)
func isaretli(x float64) string {
	if x < 0 {
		return "−%" + trSayi(-x, 1)
	}
	return "+%" + trSayi(x, 1)
}

func akisHesapla(g akisHesapGirdi) *akisHesap {
	a := g.Ayar
	h := &akisHesap{mumlar: g.Mumlar, veri: g.Veri}
	r := &h.sonuc
	r.Sembol, r.Aralik, r.Z, r.Esik = g.Sembol, g.Aralik, g.Simdi, a.Esik
	r.Durum = g.Veri.Durum
	m := g.Mumlar
	bloklar := []akisBlok{{Ad: "Yapı"}, {Ad: "Emir akışı"}, {Ad: "CVD"}, {Ad: "Açık pozisyon"}}
	for i := range bloklar {
		bloklar[i].Agirlik = a.Agirlik[i]
	}
	if len(m) < 30 {
		r.Eksik = "Grafik verisi az"
		r.Bloklar = bloklar
		r.Ozet = "Hesap için yeterli mum yok"
		return h
	}
	e := len(m) - 1
	r.Mum = m[e].Z
	fiyat := m[e].C
	if g.Veri.SonFiyat > 0 && g.Veri.SpotCanli {
		fiyat = g.Veri.SonFiyat
	}
	r.Fiyat = fiyat
	h.smc = smcHesapla(m, a)
	s := &h.smc
	atr := s.ATR[e]

	// akış: kapanmış mumlar + süren mum
	zamanlar := make([]int64, len(m)+1)
	for i := range m {
		zamanlar[i] = m[i].Z
	}
	zamanlar[len(m)] = m[e].Z + g.Ms
	h.akis = akisMumlaraTopla(zamanlar, g.Ms, g.Veri.Dakikalar)
	simdiki := &h.akis[len(m)]
	if simdiki.Var {
		simdiki.Sure = max(0, min(g.Ms, g.Simdi-zamanlar[len(m)]))
	}
	// CVD (veri başladığı mumdan itibaren birikimli)
	h.cvd = make([]float64, len(h.akis))
	h.vcvd = make([]float64, len(h.akis))
	var c1, c2 float64
	basladi, vbasladi := false, false
	for i, x := range h.akis {
		if x.Var {
			basladi = true
			c1 += x.delta()
		}
		if x.VVOran > 0 {
			vbasladi = true
			c2 += x.VAl - x.VSat
		}
		h.cvd[i], h.vcvd[i] = c1, c2
		if !basladi {
			h.cvd[i] = math.NaN()
		}
		if !vbasladi {
			h.vcvd[i] = math.NaN()
		}
	}
	// footprint satır boyu: ortalama mum boyunun 1/KademeSayisi'i (baz puan)
	atrBp := atr / m[e].C * 10000
	h.satirBp = int32(math.Max(1, math.Round(atrBp/float64(a.KademeSayisi))))
	h.fp = make([]fpSonuc, len(h.akis))
	for i := max(0, len(h.akis)-150); i < len(h.akis); i++ {
		h.fp[i] = fpHesapla(h.akis[i].Iz, h.satirBp, a.DengesizlikOran)
	}

	// --- bağlam ---
	N := min(a.CvdBakis, e)
	sonKirilimYukari := false
	for _, k := range s.Kirilimlar {
		if k.Yon == 1 && k.I >= e-2 {
			sonKirilimYukari = true
		}
	}
	konum := 0.5
	if s.AralikUst > s.AralikAlt {
		konum = (fiyat - s.AralikAlt) / (s.AralikUst - s.AralikAlt)
	}
	degisim := (m[e].C - m[e-N].C) / m[e-N].C * 100
	switch {
	case sonKirilimYukari || fiyat > s.AralikUst:
		r.Baglam = "kirilim"
	case konum < 0.5 || degisim < 0:
		r.Baglam = "dip"
	default:
		r.Baglam = "yatay"
	}

	// --- 1) YAPI ---
	y := &bloklar[0]
	y.Var = true
	if s.AnaTrend == 1 {
		y.ekle(25, "Ana yapı yukarı (son kırılım yukarı)")
	} else if s.AnaTrend == -1 {
		y.ekle(-25, "Ana yapı aşağı (son kırılım aşağı)")
	}
	var sonIc *smcKirilim
	for i := range s.Kirilimlar {
		if s.Kirilimlar[i].Ic {
			sonIc = &s.Kirilimlar[i]
		}
	}
	if sonIc != nil {
		if sonIc.Yon == 1 && sonIc.I >= e-10 {
			if sonIc.Choch {
				y.ekle(20, "İç yapı yukarı döndü (CHoCH)")
			} else {
				y.ekle(10, "İç yapı yukarı kırıldı (BOS)")
			}
		} else if sonIc.Yon == -1 && sonIc.I >= e-5 {
			y.ekle(-15, "İç yapı az önce aşağı kırıldı")
		}
	}
	if r.Baglam != "kirilim" {
		k := sinirla(konum, 0, 1)
		if k <= 0.5 {
			y.ekle(20*(0.5-k)/0.5, fmt.Sprintf("İndirim bölgesinde (aralıkta %s seviyesi; 0 dip, 100 tepe)", trSayi(k*100, 0)))
		} else {
			y.ekle(-20*(k-0.5)/0.5, fmt.Sprintf("Pahalı bölgede (aralıkta %s seviyesi; 0 dip, 100 tepe)", trSayi(k*100, 0)))
		}
	}
	icinde := func(b smcBolge) bool {
		return b.I1 < 0 && b.Olus <= e && ((fiyat >= b.Alt && fiyat <= b.Ust*1.0005) || (m[e].L <= b.Ust && m[e].C >= b.Alt))
	}
	talepOB, talepFVG, arzOB := false, false, false
	for _, b := range s.Bolgeler {
		if !icinde(b) {
			continue
		}
		switch {
		case b.Tur == "OB" && b.Yon == 1:
			talepOB = true
		case b.Tur == "FVG" && b.Yon == 1:
			talepFVG = true
		case b.Tur == "OB" && b.Yon == -1:
			arzOB = true
		}
	}
	if talepOB {
		y.ekle(25, "Talep bölgesinde (alıcı order block'u)")
	} else if talepFVG {
		y.ekle(15, "Fiyat boşluğunun içinde (FVG)")
	}
	if arzOB {
		y.ekle(-20, "Arz bölgesinin içinde (satıcı order block'u)")
	}
	var supurme *smcSupurme
	for i := range s.Supurmeler {
		x := &s.Supurmeler[i]
		if x.Yon == 1 && x.I >= e-a.SupurmeMum && x.I <= e && fiyat > x.F {
			supurme = x
		}
	}
	if supurme != nil {
		p := 25.0
		if supurme.Guclu {
			p = 35
		}
		y.ekle(p, fmt.Sprintf("Stoplar süpürüldü (%s altı, %d mum önce)", supurme.Kaynak, e-supurme.I))
	}
	for _, x := range s.Supurmeler {
		if x.Yon == -1 && x.I >= e-5 {
			y.ekle(-20, "Tepedeki stoplar süpürülüp geri düştü")
			break
		}
	}
	for _, q := range s.Esitler {
		if !q.Tepe && q.Supurme < 0 && q.I1+3 <= e && q.F < fiyat && fiyat-q.F < 0.5*atr {
			y.ekle(-15, "Hemen altında süpürülmemiş eşit dipler (fiyatı çekebilir)")
			break
		}
	}
	y.topla()

	// --- 2) EMİR AKIŞI ---
	ak := &bloklar[1]
	sonAkis := h.akis[e]
	gecmisVar := 0
	for i := max(0, e-20); i <= e; i++ {
		if h.akis[i].Var && !h.akis[i].Eksik {
			gecmisVar++
		}
	}
	ortHacim := 0.0
	{
		var t float64
		var n int
		for i := max(0, e-20); i < e; i++ {
			if h.akis[i].Var && !h.akis[i].Eksik {
				t += h.akis[i].hacim()
				n++
			}
		}
		if n > 0 {
			ortHacim = t / float64(n)
		}
	}
	switch {
	case !sonAkis.Var || sonAkis.hacim() == 0:
		ak.Neden = "Son mumda işlem verisi yok"
	case gecmisVar < 5 || ortHacim == 0:
		ak.Neden = "Veri toplanıyor (en az 5 mum gerekir)"
	default:
		ak.Var = true
		ro := sonAkis.oran()
		ak.ekle(15*sinirla(ro/0.3, -1, 1), fmt.Sprintf("Son mumda %s baskın (delta %s)", map[bool]string{true: "alıcılar", false: "satıcılar"}[ro >= 0], isaretli(ro*100)))
		// emilim
		for i := max(5, e-2); i <= e; i++ {
			x := h.akis[i]
			if !x.Var || x.Eksik || x.hacim() < a.EmilimHacim*ortHacim {
				continue
			}
			boy := m[i].H - m[i].L
			if boy <= 0 {
				continue
			}
			yerel := math.Inf(1)
			yerelU := math.Inf(-1)
			for j := i - 5; j < i; j++ {
				yerel = math.Min(yerel, m[j].L)
				yerelU = math.Max(yerelU, m[j].H)
			}
			if x.oran() <= -0.15 && (m[i].C-m[i].L)/boy >= 0.5 && m[i].L <= yerel {
				ak.ekle(35, fmt.Sprintf("Dipte satış emildi (hacim ortalamanın %s katı, delta %s, kapanış mumun üst yarısında)", ondalik1(x.hacim()/ortHacim), isaretli(x.oran()*100)))
				break
			}
			if x.oran() >= 0.15 && (m[i].H-m[i].C)/boy >= 0.5 && m[i].H >= yerelU {
				ak.ekle(-25, "Tepede alım emildi (alıcılar fiyatı çıkaramadı)")
				break
			}
		}
		// yorgunluk: son 3 itişte dipler alçalırken satış hacmi azalıyor
		if e >= 3 && m[e].L < m[e-1].L && m[e-1].L < m[e-2].L {
			a1, a2, a3 := h.akis[e-2], h.akis[e-1], h.akis[e]
			if a1.Var && a2.Var && a3.Var && a3.Sat < a2.Sat && a2.Sat < a1.Sat {
				ak.ekle(20, "Satıcılar yoruluyor (her yeni dipte daha az satış)")
			}
		}
		// delta dönüşü
		onceki := h.akis[e-1]
		if onceki.Var && onceki.delta() < 0 && ro >= 0.1 {
			ak.ekle(20, "Delta eksiden artıya döndü (alıcılar devreye girdi)")
		} else if sonAkis.delta() < 0 && simdiki.Var && simdiki.Sure >= 15_000 && simdiki.oran() >= 0.15 && simdiki.hacim() >= 0.2*ortHacim {
			ak.ekle(15, "Şu anki mumda alıcılar devreye girdi")
		}
		// footprint dengesizlikleri (son kapanan mum ve süren mum)
		fpE := h.fp[e]
		ortaFiyat := (m[e].H + m[e].L) / 2
		if fpE.AlimYigin >= a.DengesizlikKademe && fpE.AlimYer <= ortaFiyat {
			ak.ekle(20, fmt.Sprintf("Mumun alt yarısında üst üste alım dengesizliği (%d kademe)", fpE.AlimYigin))
		} else if fs := h.fp[len(m)]; fs.AlimYigin >= a.DengesizlikKademe && simdiki.Sure >= 15_000 {
			ak.ekle(15, fmt.Sprintf("Şu anki mumda üst üste alım dengesizliği (%d kademe)", fs.AlimYigin))
		}
		if fpE.SatimYigin >= a.DengesizlikKademe {
			ak.ekle(-20, fmt.Sprintf("Satıcılar kademe kademe bastırıyor (%d kademe satış dengesizliği)", fpE.SatimYigin))
		}
		// büyük işlemler (son 15 dk)
		var bAl, bSat float64
		for _, b := range g.Veri.Buyukler {
			if b.Z >= g.Simdi-15*60_000 && !b.Vadeli {
				if b.Alim {
					bAl += b.Fiyat * b.Adet
				} else {
					bSat += b.Fiyat * b.Adet
				}
			}
		}
		if net := bAl - bSat; bAl+bSat > 0 && math.Abs(net) >= 0.25*(bAl+bSat) {
			if net > 0 {
				ak.ekle(15, fmt.Sprintf("Büyük işlemlerde alım ağır (son 15 dk: %s USDT net)", trSayi(net, 0)))
			} else {
				ak.ekle(-15, fmt.Sprintf("Büyük işlemlerde satış ağır (son 15 dk: %s USDT net)", trSayi(-net, 0)))
			}
		}
		// şu anki saldırı
		if simdiki.Var && simdiki.Sure >= 15_000 && simdiki.oran() <= -0.4 && simdiki.hacim() >= 0.3*ortHacim {
			ak.ekle(-30, fmt.Sprintf("Şu an satış saldırısı var (delta %s)", isaretli(simdiki.oran()*100)))
		}
		// defter eğilimi (son 2 dakika)
		var de float64
		var deN int
		for i := len(g.Veri.Dakikalar) - 1; i >= 0 && i >= len(g.Veri.Dakikalar)-2; i-- {
			if g.Veri.Dakikalar[i].DE != 0 {
				de += g.Veri.Dakikalar[i].DE
				deN++
			}
		}
		if deN > 0 {
			de /= float64(deN)
			if de >= 0.3 {
				ak.ekle(10, "Defterde alıcılar ağır basıyor")
			} else if de <= -0.3 {
				ak.ekle(-10, "Defterde satıcılar ağır basıyor")
			}
		}
	}
	ak.topla()

	// --- 3) CVD ---
	cv := &bloklar[2]
	cvdTam := 0
	for i := max(0, e-N); i <= e; i++ {
		if h.akis[i].Var && !h.akis[i].Eksik {
			cvdTam++
		}
	}
	if cvdTam < N*7/10 || nanMi(h.cvd[e-N]) {
		cv.Neden = fmt.Sprintf("Veri toplanıyor: son %d mumun %d tanesinde veri var, en az %d gerekir", N+1, cvdTam, N*7/10)
	} else {
		cv.Var = true
		var vol float64
		for i := e - N + 1; i <= e; i++ {
			vol += h.akis[i].hacim()
		}
		egim := 0.0
		if vol > 0 {
			egim = (h.cvd[e] - h.cvd[e-N]) / vol
		}
		agir := 30.0
		if r.Baglam == "dip" {
			agir = 15 // düşüşte CVD'nin düşmesi beklenir; asıl ipucu uyumsuzluk
		}
		cv.ekle(agir*sinirla(egim/0.15, -1, 1), fmt.Sprintf("CVD son %d mumda %s (net %s payı %s)", N, map[bool]string{true: "yükseliyor", false: "düşüyor"}[egim >= 0], map[bool]string{true: "alım", false: "satış"}[egim >= 0], "%"+trSayi(math.Abs(egim)*100, 1)))
		// uyumsuzluk: son 5 mumun en düşüğü ↔ önceki dip
		enDusuk := func(bas, bit int) int {
			k := bas
			for i := bas; i <= bit; i++ {
				if m[i].L < m[k].L {
					k = i
				}
			}
			return k
		}
		enYuksek := func(bas, bit int) int {
			k := bas
			for i := bas; i <= bit; i++ {
				if m[i].H > m[k].H {
					k = i
				}
			}
			return k
		}
		if e-N <= e-6 {
			ai, bi := enDusuk(e-4, e), enDusuk(e-N, e-6)
			if m[ai].L < m[bi].L && !nanMi(h.cvd[bi]) {
				var v float64
				for i := bi; i <= ai; i++ {
					v += h.akis[i].hacim()
				}
				if h.cvd[ai] > h.cvd[bi]+0.05*v {
					cv.ekle(40, "Fiyat yeni dip yaptı ama CVD yapmadı (satıcılar zayıflıyor)")
				}
			}
			if r.Baglam != "dip" {
				ai, bi = enYuksek(e-4, e), enYuksek(e-N, e-6)
				if m[ai].H > m[bi].H && !nanMi(h.cvd[bi]) {
					var v float64
					for i := bi; i <= ai; i++ {
						v += h.akis[i].hacim()
					}
					if h.cvd[ai] < h.cvd[bi]-0.05*v {
						cv.ekle(-35, "Fiyat yeni tepe yaptı ama CVD yapmadı (alıcılar zayıf)")
					}
				}
			}
		}
		// spot ↔ vadeli
		var sa, ss, va, vs float64
		vvSay := 0
		for i := e - N + 1; i <= e; i++ {
			x := h.akis[i]
			sa += x.Al
			ss += x.Sat
			if x.VVOran >= 0.99 {
				vvSay++
				va += x.VAl
				vs += x.VSat
			}
		}
		if vvSay >= N*7/10 && sa+ss > 0 && va+vs > 0 {
			fark := (sa-ss)/(sa+ss) - (va-vs)/(va+vs)
			if fark >= 0.1 {
				cv.ekle(20, "Spot alıcılar vadeliden önde (hareket gerçek alımla)")
			} else if fark <= -0.1 {
				cv.ekle(-15, "Hareketi vadeli (kaldıraç) sürüklüyor, spot geride")
			}
		}
	}
	cv.topla()

	// --- 4) AÇIK POZİSYON ---
	oi := &bloklar[3]
	NO := min(a.OiBakis, e)
	if !g.Veri.Vadeli {
		oi.Neden = "Bu coinin MEXC'de vadelisi yok"
	} else {
		bas := e - NO
		if r.Baglam == "dip" {
			bas = e - NO
			for i := e - NO; i <= e; i++ {
				if m[i].H > m[bas].H {
					bas = i // düşüş bacağının başı: penceredeki en yüksek mum
				}
			}
			bas = min(bas, e-1)
		}
		vvSay := 0
		for i := bas; i <= e; i++ {
			if h.akis[i].VVOran >= 0.99 && h.akis[i].OISon > 0 {
				vvSay++
			}
		}
		oiBas, oiSon := h.akis[bas].OIBas, h.akis[e].OISon
		if simdiki.VVOran > 0 && simdiki.OISon > 0 {
			oiSon = simdiki.OISon
		}
		if vvSay < (e-bas+1)*7/10 || oiBas <= 0 || oiSon <= 0 {
			oi.Neden = "Veri toplanıyor (açık pozisyonun geçmişi MEXC'de yok; program açıkken birikir)"
		} else {
			oi.Var = true
			dOI := (oiSon - oiBas) / oiBas * 100
			dP := (fiyat - m[bas].O) / m[bas].O * 100
			esik := a.OiEsik
			if dP < 0 {
				switch {
				case dOI <= -esik:
					oi.ekle(35, fmt.Sprintf("Düşüşte açık pozisyon %%%s azaldı (pozisyonlar kapandı, satış yakıtı bitti)", ondalik1(-dOI)))
				case dOI >= esik:
					diverjans := false
					for _, x := range cv.Maddeler {
						if strings.HasPrefix(x.M, "Fiyat yeni dip") {
							diverjans = true
						}
					}
					if diverjans {
						oi.ekle(-10, fmt.Sprintf("Düşüşte açık pozisyon %%%s arttı ama satış emiliyor", ondalik1(dOI)))
					} else {
						oi.ekle(-30, fmt.Sprintf("Düşüşte açık pozisyon %%%s arttı (yeni açığa satışlar)", ondalik1(dOI)))
					}
				}
			} else {
				switch {
				case dOI >= esik:
					oi.ekle(25, fmt.Sprintf("Yükselişte yeni pozisyonlar açılıyor (açık pozisyon +%%%s)", ondalik1(dOI)))
				case dOI <= -esik:
					oi.ekle(-10, fmt.Sprintf("Yükseliş açığa satış kapatmasıyla (açık pozisyon %%%s azaldı)", ondalik1(-dOI)))
				}
			}
			// ani temizlik: son 10 dk içinde 5 dakikada 2 × eşik düşüş
			dk := g.Veri.Dakikalar
			for i := len(dk) - 1; i >= 0 && dk[i].Z >= g.Simdi-10*60_000; i-- {
				if !dk[i].VV || dk[i].OIA <= 0 {
					continue
				}
				for j := i - 1; j >= 0 && j >= i-5; j-- {
					if dk[j].VV && dk[j].OIY > 0 && (dk[i].OIA-dk[j].OIY)/dk[j].OIY*100 <= -2*esik {
						ne := "az önce"
						if dkOnce := (g.Simdi - dk[i].Z) / 60_000; dkOnce > 0 {
							ne = fmt.Sprintf("%d dk önce", dkOnce)
						}
						oi.ekle(20, fmt.Sprintf("Ani pozisyon temizliği (%s, açık pozisyon %%%s düştü)", ne, ondalik1((dk[j].OIY-dk[i].OIA)/dk[j].OIY*100)))
						i = -1
						break
					}
				}
			}
			fon := g.Veri.SonFon * 100
			if fon >= a.FonYuksek {
				oi.ekle(-15, fmt.Sprintf("Fonlama yüksek (%%%s): long'lar kalabalık", strings.Replace(fmt.Sprintf("%.3f", fon), ".", ",", 1)))
			} else if fon <= -a.FonYuksek/2 {
				oi.ekle(10, fmt.Sprintf("Fonlama eksi (%%%s): short'lar kalabalık", strings.Replace(fmt.Sprintf("%.3f", fon), ".", ",", 1)))
			}
		}
	}
	oi.topla()

	// --- toplam ve karar ---
	var top, agir float64
	for i := range bloklar {
		if bloklar[i].Var && bloklar[i].Agirlik > 0 {
			top += bloklar[i].Agirlik * bloklar[i].Puan
			agir += bloklar[i].Agirlik
		}
	}
	if agir > 0 {
		r.Toplam = math.Round(top / agir)
	}
	if a.AkisSart && !bloklar[1].Var {
		r.Eksik = "Emir akışı verisi yok: " + bloklar[1].Neden
	}
	if agir == 0 && r.Eksik == "" {
		r.Eksik = "Hiçbir bölümün verisi yok"
	}
	if a.YasakYapi {
		for i := len(s.Kirilimlar) - 1; i >= 0; i-- {
			k := s.Kirilimlar[i]
			if k.Ic {
				continue
			}
			if k.Yon == -1 && k.I >= e-5 {
				supuruldu := false
				for _, x := range s.Supurmeler {
					if x.Yon == 1 && x.I > k.I {
						supuruldu = true
					}
				}
				if !supuruldu {
					r.Yasak = "Ana yapı az önce aşağı kırıldı"
				}
			}
			break
		}
	}
	if a.YasakSaldiri && r.Yasak == "" && simdiki.Var && simdiki.Sure >= 10_000 && ortHacim > 0 &&
		simdiki.oran() <= -0.5 && simdiki.hacim() >= 0.5*ortHacim && fiyat < m[e].L {
		r.Yasak = "Satış saldırısı sürüyor (fiyat son mumun altında)"
	}
	r.Onay = r.Eksik == "" && r.Yasak == "" && r.Toplam >= a.Esik
	r.Bloklar = bloklar
	r.Ozet = akisOzet(r)
	return h
}

// akisOzet: tek satırlık açıklama (en etkili 3 madde)
func akisOzet(r *akisSonuc) string {
	var b strings.Builder
	if r.Onay {
		fmt.Fprintf(&b, "Onay: %+.0f (eşik %+.0f)", r.Toplam, r.Esik)
	} else {
		fmt.Fprintf(&b, "Red: %+.0f (eşik %+.0f)", r.Toplam, r.Esik)
	}
	switch {
	case r.Eksik != "":
		b.WriteString(" · " + r.Eksik)
		return b.String()
	case r.Yasak != "":
		b.WriteString(" · " + r.Yasak)
	}
	var hepsi []akisMadde
	for _, bl := range r.Bloklar {
		if bl.Var {
			hepsi = append(hepsi, bl.Maddeler...)
		}
	}
	sort.SliceStable(hepsi, func(i, j int) bool {
		if r.Onay {
			return hepsi[i].P > hepsi[j].P
		}
		return hepsi[i].P < hepsi[j].P
	})
	n := 0
	for _, x := range hepsi {
		if n == 3 || x.M == "" || (r.Onay && x.P <= 0) || (!r.Onay && x.P >= 0) {
			break
		}
		if n == 0 {
			b.WriteString(" · ")
		} else {
			b.WriteString("; ")
		}
		b.WriteString(x.M)
		n++
	}
	return b.String()
}

// akisDegerlendir: coinin bu mum süresinde, şu an motor alsa sistem ne derdi?
func akisDegerlendir(sembol, aralik string) (*akisHesap, error) {
	ar, ok := araliklar[aralik]
	if !ok {
		return nil, fmt.Errorf("mum süresi geçersiz")
	}
	m, err := akisMumlari(sembol, aralik)
	if err != nil {
		return nil, err
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("mum yok")
	}
	v := akisAnlik(sembol, m[0].Z)
	return akisHesapla(akisHesapGirdi{Sembol: sembol, Aralik: aralik, Ms: ar.ms, Mumlar: m, Veri: v, Simdi: akisSimdi(), Ayar: akisAyarAl()}), nil
}
