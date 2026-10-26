package main

// AKIŞ ONAYI — GRAFİK VERİSİ (0.9). Sayfa, seçili coinin grafiğine akış sisteminin gördüğü her şeyi çizer:
//   - ana grafikte: yapı (salınım etiketleri HH/HL/LH/LL, BOS/CHoCH çizgileri, talep/arz bölgeleri, FVG, eşit
//     tepe/dipler, süpürmeler, indirim/pahalı aralığı), emir akışı işaretleri (emilim, üst üste dengesizlik), büyük
//     işlemler, motorun alım sinyalleri ve akışın kararı;
//   - alttaki akış panelinde: her mumun deltası, CVD (spot ve vadeli), açık pozisyon.
// Zamanlar mumun açılışıdır (MEXC saati, ms); sayfa kendi mumlarına bu zamanla eşler. Hesap akisHesapla'nınkiyle
// aynıdır (aynı fonksiyonlar), ekranda görünen ile karar veren aynı veri olsun.

import (
	"math"
	"net/http"
	"sort"
	"strconv"
)

type grafikSalinim struct {
	Z      int64   `json:"z"`
	F      float64 `json:"f"`
	Tepe   bool    `json:"tepe"`
	Etiket string  `json:"etiket"`
	Ic     bool    `json:"ic"`
	OnayZ  int64   `json:"onayZ"` // kesinleştiği mum (o ana kadar bilinmez)
}

type grafikKirilim struct {
	Z0    int64   `json:"z0"` // kırılan salınım
	Z     int64   `json:"z"`  // kırıldığı mum
	F     float64 `json:"f"`
	Yon   int     `json:"yon"`
	Choch bool    `json:"choch"`
	Ic    bool    `json:"ic"`
}

type grafikBolge struct {
	Z0   int64   `json:"z0"`
	Olus int64   `json:"olus"`
	Z1   int64   `json:"z1"` // 0: hâlâ geçerli
	Ust  float64 `json:"ust"`
	Alt  float64 `json:"alt"`
	Yon  int     `json:"yon"`
	Tur  string  `json:"tur"`
	Ic   bool    `json:"ic"`
}

type grafikEsit struct {
	Z0      int64   `json:"z0"`
	Z1      int64   `json:"z1"`
	F       float64 `json:"f"`
	Tepe    bool    `json:"tepe"`
	Supurme int64   `json:"supurme"` // süpürüldüğü mum (0: süpürülmedi)
}

type grafikSupurme struct {
	Z      int64   `json:"z"`
	F      float64 `json:"f"`
	Yon    int     `json:"yon"`
	Kaynak string  `json:"kaynak"`
	Guclu  bool    `json:"guclu"`
}

type grafikIsaret struct {
	Z   int64   `json:"z"`
	F   float64 `json:"f"`
	Tur string  `json:"tur"` // emilimDip | emilimTepe | alimYigin | satimYigin
	M   string  `json:"m"`   // açıklama
}

type grafikBant struct {
	Z   int64   `json:"z"`
	Ust float64 `json:"ust"`
	Alt float64 `json:"alt"`
}

type akisGrafik struct {
	Sembol     string          `json:"sembol"`
	Aralik     string          `json:"aralik"`
	Ms         int64           `json:"ms"`
	Z          []int64         `json:"z"`     // mumların açılışı (kapanmışlar + süren mum)
	Delta      []*float64      `json:"delta"` // spot: saldırgan alım − satım (coin)
	Hacim      []*float64      `json:"hacim"`
	CVD        []*float64      `json:"cvd"`
	VCVD       []*float64      `json:"vcvd"`
	OI         []*float64      `json:"oi"`
	POC        []*float64      `json:"poc"`
	Eksik      []int           `json:"eksik"` // verisi eksik mumların sırası
	Salinimlar []grafikSalinim `json:"salinimlar"`
	Kirilimlar []grafikKirilim `json:"kirilimlar"`
	Bolgeler   []grafikBolge   `json:"bolgeler"`
	Esitler    []grafikEsit    `json:"esitler"`
	Supurmeler []grafikSupurme `json:"supurmeler"`
	Isaretler  []grafikIsaret  `json:"isaretler"`
	Bant       *grafikBant     `json:"bant,omitempty"`
	Buyukler   []buyukIslem    `json:"buyukler"`
	Sonuc      akisSonuc       `json:"sonuc"`
	Kararlar   []onayKaydi     `json:"kararlar"`
	Vadeli     bool            `json:"vadeli"`
	Fon        float64         `json:"fon"`
	SatirBp    int32           `json:"satirBp"` // footprint satır boyu (baz puan)
}

func sayiP(x float64) *float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	return &x
}

// akisGrafikHazirla: hesabın çizilecek hâli (sadece son ~300 mum; eski yapı ve bölgeler kesilir)
func akisGrafikHazirla(h *akisHesap, a akisAyar) akisGrafik {
	g := akisGrafik{Sonuc: h.sonuc, Sembol: h.sonuc.Sembol, Aralik: h.sonuc.Aralik, Kararlar: []onayKaydi{}, SatirBp: h.satirBp,
		Salinimlar: []grafikSalinim{}, Kirilimlar: []grafikKirilim{}, Bolgeler: []grafikBolge{}, Esitler: []grafikEsit{},
		Supurmeler: []grafikSupurme{}, Isaretler: []grafikIsaret{}, Buyukler: []buyukIslem{}, Eksik: []int{}}
	if ar, ok := araliklar[h.sonuc.Aralik]; ok {
		g.Ms = ar.ms
	}
	n := len(h.akis) // kapanmış mumlar + süren mum
	if n < 2 || len(h.smc.ATR) == 0 {
		return g
	}
	m := h.mumlar
	e := len(m) - 1
	bas := max(0, n-300)
	zaman := func(i int) int64 {
		if i < len(m) {
			return m[i].Z
		}
		return m[e].Z + int64(i-e)*g.Ms
	}
	for i := bas; i < n; i++ {
		x := h.akis[i]
		g.Z = append(g.Z, zaman(i))
		if x.Var {
			g.Delta = append(g.Delta, sayiP(x.delta()))
			g.Hacim = append(g.Hacim, sayiP(x.hacim()))
		} else {
			g.Delta = append(g.Delta, nil)
			g.Hacim = append(g.Hacim, nil)
		}
		g.CVD = append(g.CVD, sayiP(h.cvd[i]))
		g.VCVD = append(g.VCVD, sayiP(h.vcvd[i]))
		if x.OISon > 0 {
			g.OI = append(g.OI, sayiP(x.OISon))
		} else {
			g.OI = append(g.OI, nil)
		}
		if fp := h.fp[i]; fp.POC > 0 {
			g.POC = append(g.POC, sayiP(fp.POC))
		} else {
			g.POC = append(g.POC, nil)
		}
		if x.Eksik || (!x.Var && i < n-1) {
			g.Eksik = append(g.Eksik, i-bas)
		}
	}
	gorunur := func(i int) bool { return i >= bas }
	for _, s := range h.smc.Salinimlar {
		if gorunur(s.I) && s.Onay <= e {
			g.Salinimlar = append(g.Salinimlar, grafikSalinim{Z: zaman(s.I), F: s.F, Tepe: s.Tepe, Etiket: s.Etiket, Ic: s.Ic, OnayZ: zaman(s.Onay)})
		}
	}
	for _, k := range h.smc.Kirilimlar {
		if gorunur(k.I) {
			g.Kirilimlar = append(g.Kirilimlar, grafikKirilim{Z0: zaman(max(k.I0, bas)), Z: zaman(k.I), F: k.F, Yon: k.Yon, Choch: k.Choch, Ic: k.Ic})
		}
	}
	// bölgeler: sadece hâlâ geçerli olanlar; her türden en yeni birkaçı (LuxAlgo'daki gibi; grafik kalabalık olmasın)
	bolgeler := append([]smcBolge(nil), h.smc.Bolgeler...)
	sort.SliceStable(bolgeler, func(i, j int) bool { return bolgeler[i].Olus > bolgeler[j].Olus })
	sayac := map[string]int{}
	enFazla := map[string]int{"OB": 5, "OBic": 4, "FVG": 4}
	for _, b := range bolgeler {
		if b.I1 >= 0 || b.Olus > e {
			continue
		}
		tur := b.Tur
		if b.Ic {
			tur += "ic"
		}
		if sayac[tur] >= enFazla[tur] {
			continue
		}
		sayac[tur]++
		g.Bolgeler = append(g.Bolgeler, grafikBolge{Z0: zaman(max(b.I0, bas)), Olus: zaman(b.Olus), Ust: b.Ust, Alt: b.Alt, Yon: b.Yon, Tur: b.Tur, Ic: b.Ic})
	}
	for _, q := range h.smc.Esitler {
		if !gorunur(q.I1) {
			continue
		}
		var sup int64
		if q.Supurme >= 0 {
			sup = zaman(q.Supurme)
		}
		g.Esitler = append(g.Esitler, grafikEsit{Z0: zaman(max(q.I0, bas)), Z1: zaman(q.I1), F: q.F, Tepe: q.Tepe, Supurme: sup})
	}
	for _, s := range h.smc.Supurmeler {
		if gorunur(s.I) {
			g.Supurmeler = append(g.Supurmeler, grafikSupurme{Z: zaman(s.I), F: s.F, Yon: s.Yon, Kaynak: s.Kaynak, Guclu: s.Guclu})
		}
	}
	if s := h.smc; s.AralikUst > s.AralikAlt && !math.IsInf(s.AralikUst, 0) && !math.IsInf(s.AralikAlt, 0) {
		g.Bant = &grafikBant{Z: zaman(max(s.AralikBas, bas)), Ust: s.AralikUst, Alt: s.AralikAlt}
	}
	g.Isaretler = akisIsaretleri(h, a, max(bas, 5))
	g.Vadeli, g.Fon = h.veri.Vadeli, h.veri.SonFon
	if len(m) > 0 {
		ilk := zaman(bas)
		for _, b := range h.veri.Buyukler {
			if b.Z >= ilk && !b.Vadeli {
				g.Buyukler = append(g.Buyukler, b)
			}
		}
		if len(g.Buyukler) > 400 {
			g.Buyukler = g.Buyukler[len(g.Buyukler)-400:]
		}
	}
	return g
}

// akisIsaretleri: puanlamadaki emilim ve üst üste dengesizlik ölçütleri, bütün görünen mumlar için
func akisIsaretleri(h *akisHesap, a akisAyar, bas int) []grafikIsaret {
	out := []grafikIsaret{}
	m := h.mumlar
	ortHacim := func(i int) float64 {
		var t float64
		var n int
		for j := max(0, i-20); j < i; j++ {
			if h.akis[j].Var && !h.akis[j].Eksik {
				t += h.akis[j].hacim()
				n++
			}
		}
		if n < 5 {
			return 0
		}
		return t / float64(n)
	}
	for i := bas; i < len(m); i++ {
		x := h.akis[i]
		if !x.Var || x.Eksik {
			continue
		}
		boy := m[i].H - m[i].L
		if ort := ortHacim(i); ort > 0 && boy > 0 && x.hacim() >= a.EmilimHacim*ort {
			yerel, yerelU := math.Inf(1), math.Inf(-1)
			for j := i - 5; j < i; j++ {
				yerel = math.Min(yerel, m[j].L)
				yerelU = math.Max(yerelU, m[j].H)
			}
			if x.oran() <= -0.15 && (m[i].C-m[i].L)/boy >= 0.5 && m[i].L <= yerel {
				out = append(out, grafikIsaret{Z: m[i].Z, F: m[i].L, Tur: "emilimDip",
					M: "Dipte satış emildi: hacim ortalamanın " + ondalik1(x.hacim()/ort) + " katı, delta " + isaretli(x.oran()*100) + ", kapanış mumun üst yarısında"})
			} else if x.oran() >= 0.15 && (m[i].H-m[i].C)/boy >= 0.5 && m[i].H >= yerelU {
				out = append(out, grafikIsaret{Z: m[i].Z, F: m[i].H, Tur: "emilimTepe",
					M: "Tepede alım emildi: alıcılar fiyatı çıkaramadı (delta " + isaretli(x.oran()*100) + ")"})
			}
		}
		fp := h.fp[i]
		if fp.AlimYigin >= a.DengesizlikKademe && fp.AlimYer > 0 {
			out = append(out, grafikIsaret{Z: m[i].Z, F: fp.AlimYer, Tur: "alimYigin", M: "Üst üste alım dengesizliği: " + strconv.Itoa(fp.AlimYigin) + " kademe"})
		}
		if fp.SatimYigin >= a.DengesizlikKademe && fp.SatimYer > 0 {
			out = append(out, grafikIsaret{Z: m[i].Z, F: fp.SatimYer, Tur: "satimYigin", M: "Üst üste satış dengesizliği: " + strconv.Itoa(fp.SatimYigin) + " kademe"})
		}
	}
	return out
}

// ---------- footprint (tek mum) ----------

type footprintSatiri struct {
	Fiyat     float64 `json:"fiyat"` // satırın ortası
	Alt       float64 `json:"alt"`
	Ust       float64 `json:"ust"`
	Al        float64 `json:"al"`
	Sat       float64 `json:"sat"`
	AlimDeng  bool    `json:"alimDeng"`  // bu satırdaki alım, bir alttaki satımın oran katı
	SatimDeng bool    `json:"satimDeng"` // bu satırdaki satım, bir üstteki alımın oran katı
}

type footprintCevap struct {
	Z        int64             `json:"z"`
	Satirlar []footprintSatiri `json:"satirlar"`
	POC      float64           `json:"poc"`
	Delta    float64           `json:"delta"`
	Hacim    float64           `json:"hacim"`
	Oran     float64           `json:"oran"`
	Eksik    bool              `json:"eksik"`
	Var      bool              `json:"var"`
}

// GET /api/akis/footprint?sembol=&aralik=&z= → bir mumun fiyat kademelerindeki alım/satım
func akisFootprintIstegi(w http.ResponseWriter, r *http.Request) {
	sembol, aralik, ok := akisSorgu(w, r)
	if !ok {
		return
	}
	z, _ := strconv.ParseInt(r.URL.Query().Get("z"), 10, 64)
	h, err := akisDegerlendirF(sembol, aralik)
	if err != nil {
		hataYaz(w, 502, "Akış hesabı yapılamadı: "+err.Error())
		return
	}
	jsonYaz(w, 200, footprintHazirla(h, z, akisAyarAl()))
}

func footprintHazirla(h *akisHesap, z int64, a akisAyar) footprintCevap {
	c := footprintCevap{Z: z, Satirlar: []footprintSatiri{}}
	m := h.mumlar
	if len(m) == 0 {
		return c
	}
	ms := int64(60_000)
	if ar, ok := araliklar[h.sonuc.Aralik]; ok {
		ms = ar.ms
	}
	i := -1
	for j := range h.akis {
		zj := m[min(j, len(m)-1)].Z
		if j >= len(m) {
			zj += int64(j-len(m)+1) * ms
		}
		if zj == z {
			i = j
			break
		}
	}
	if i < 0 {
		return c
	}
	x := h.akis[i]
	c.Var, c.Eksik, c.Delta, c.Hacim, c.Oran = x.Var, x.Eksik, x.delta(), x.hacim(), x.oran()
	if !x.Var || len(x.Iz) == 0 {
		return c
	}
	fp := fpHesapla(x.Iz, h.satirBp, a.DengesizlikOran)
	c.POC = fp.POC
	satir := map[int32]fpSatir{}
	for _, s := range fp.Satirlar {
		satir[s.K] = s
	}
	var top float64
	for _, s := range fp.Satirlar {
		top += s.Al + s.Sat
	}
	enAz := 0.02 * top
	bp := h.satirBp
	ilk, son := fp.Satirlar[0].K, fp.Satirlar[len(fp.Satirlar)-1].K
	for idx := len(fp.Satirlar) - 1; idx >= 0; idx-- { // yukarıdan aşağı
		s := fp.Satirlar[idx]
		alt := akisKovaFiyat(s.K * bp)
		ust := akisKovaFiyat((s.K + 1) * bp)
		fs := footprintSatiri{Fiyat: akisKovaFiyat(s.K*bp + bp/2), Alt: alt, Ust: ust, Al: s.Al, Sat: s.Sat}
		// uçlar sayılmaz (en alttaki alımın altında, en üstteki satımın üstünde karşılaştırılacak satır yok): fpHesapla ile aynı
		if s.K > ilk && s.Al >= enAz && s.Al >= a.DengesizlikOran*satir[s.K-1].Sat {
			fs.AlimDeng = true
		}
		if s.K < son && s.Sat >= enAz && s.Sat >= a.DengesizlikOran*satir[s.K+1].Al {
			fs.SatimDeng = true
		}
		c.Satirlar = append(c.Satirlar, fs)
	}
	return c
}
