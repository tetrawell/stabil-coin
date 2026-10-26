package main

// PİYASA VERİSİ — MEXC'nin herkese açık uçları (anahtar gerekmez):
// en iyi alış/satış, son işlemler, emir defteri, son fiyat, coin kuralları (fiyat/miktar hassasiyeti, en az tutar, komisyon).
// Sanal emirler bu verilerle gerçeğe benzetilir; gerçek emirler de fiyatı ve kuralları buradan alır.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type defterUstu struct{ Bid, BidMiktar, Ask, AskMiktar float64 }

type islemKaydi struct {
	Fiyat, Miktar float64
	Zaman         int64
	AliciMaker    bool // true: satıcı piyasadan sattı (alış emirlerine çarptı)
}

type defterSeviye struct{ Fiyat, Miktar float64 }

// Alis: yüksekten düşüğe (bids), Satis: düşükten yükseğe (asks)
type defter struct{ Alis, Satis []defterSeviye }

type sembolKurali struct {
	Sembol          string   `json:"sembol"`
	Varlik          string   `json:"varlik"` // coinin kendisi (ör. BTC)
	Acik            bool     `json:"acik"`   // borsada işlem açık ve API ile spot alım-satım serbest
	FiyatOndalik    int      `json:"fiyatOndalik"`
	MiktarOndalik   int      `json:"miktarOndalik"`
	TutarOndalik    int      `json:"tutarOndalik"`
	EnAzMiktar      float64  `json:"enAzMiktar"`
	EnAzTutar       float64  `json:"enAzTutar"`       // limit emirde en az USDT
	EnAzTutarPiyasa float64  `json:"enAzTutarPiyasa"` // piyasa emrinde en az USDT
	Maker           float64  `json:"maker"`           // oran: 0.0005 = %0,05
	Taker           float64  `json:"taker"`
	Turler          []string `json:"turler"`
	TarafTuru       int      `json:"tarafTuru"` // MEXC tradeSideType: 1 alım-satım, 2 sadece alış, 3 sadece satış, 4 kapalı (0: bilinmiyor)
}

func (k sembolKurali) alinabilir() bool {
	return k.TarafTuru == 0 || k.TarafTuru == 1 || k.TarafTuru == 2
}
func (k sembolKurali) satilabilir() bool {
	return k.TarafTuru == 0 || k.TarafTuru == 1 || k.TarafTuru == 3
}

func (k sembolKurali) adim() float64 { return math.Pow10(-k.MiktarOndalik) }
func (k sembolKurali) tik() float64  { return math.Pow10(-k.FiyatOndalik) }

type piyasaKaynagi interface {
	EnIyiFiyat(sembol string) (defterUstu, error)
	SonIslemler(sembol string, adet int) ([]islemKaydi, error)
	Derinlik(sembol string, adet int) (defter, error)
	SonFiyat(sembol string) (float64, error)
	Kurallar(sembol string) (sembolKurali, error)
}

// ---------- sayı yardımcıları ----------

func asagiYuvarla(x float64, ondalik int) float64 {
	p := math.Pow10(ondalik)
	return math.Floor(x*p+1e-7) / p
}
func yakinYuvarla(x float64, ondalik int) float64 {
	p := math.Pow10(ondalik)
	return math.Round(x*p) / p
}
func sayiMetni(x float64, ondalik int) string { return strconv.FormatFloat(x, 'f', ondalik, 64) }

func jsonSayi(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	case int:
		return float64(x)
	}
	return 0
}

// ---------- MEXC herkese açık uçlar ----------

type mexcPiyasa struct {
	mu       sync.Mutex
	kurallar map[string]kuralKaydi
}

type kuralKaydi struct {
	k     sembolKurali
	zaman time.Time
}

func yeniMexcPiyasa() *mexcPiyasa { return &mexcPiyasa{kurallar: map[string]kuralKaydi{}} }

func piyasaGet(yol string, q url.Values) ([]byte, error) {
	u := mexcAdresi + yol
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var b []byte
	var err error
	for deneme := 0; deneme < 3; deneme++ {
		b, err = getJSON(u)
		if err == nil {
			return b, nil
		}
		var bh borsaHatasi
		if errors.As(err, &bh) {
			return nil, err
		}
		uyu(time.Duration(300*(deneme+1)) * time.Millisecond)
	}
	return nil, err
}

func (m *mexcPiyasa) EnIyiFiyat(sembol string) (defterUstu, error) {
	b, err := piyasaGet("/api/v3/ticker/bookTicker", url.Values{"symbol": {sembol}})
	if err != nil {
		return defterUstu{}, err
	}
	var r map[string]any
	if err := json.Unmarshal(b, &r); err != nil {
		return defterUstu{}, err
	}
	d := defterUstu{Bid: jsonSayi(r["bidPrice"]), BidMiktar: jsonSayi(r["bidQty"]), Ask: jsonSayi(r["askPrice"]), AskMiktar: jsonSayi(r["askQty"])}
	if d.Bid <= 0 || d.Ask <= 0 || d.Ask < d.Bid {
		return d, fmt.Errorf("%s için alış/satış fiyatı okunamadı", sembol)
	}
	return d, nil
}

func (m *mexcPiyasa) SonIslemler(sembol string, adet int) ([]islemKaydi, error) {
	b, err := piyasaGet("/api/v3/trades", url.Values{"symbol": {sembol}, "limit": {strconv.Itoa(adet)}})
	if err != nil {
		return nil, err
	}
	var ham []map[string]any
	if err := json.Unmarshal(b, &ham); err != nil {
		return nil, err
	}
	out := make([]islemKaydi, 0, len(ham))
	for _, x := range ham {
		am, _ := x["isBuyerMaker"].(bool)
		out = append(out, islemKaydi{Fiyat: jsonSayi(x["price"]), Miktar: jsonSayi(x["qty"]), Zaman: int64(jsonSayi(x["time"])), AliciMaker: am})
	}
	return out, nil
}

func (m *mexcPiyasa) Derinlik(sembol string, adet int) (defter, error) {
	b, err := piyasaGet("/api/v3/depth", url.Values{"symbol": {sembol}, "limit": {strconv.Itoa(adet)}})
	if err != nil {
		return defter{}, err
	}
	var r struct {
		Bids [][]any `json:"bids"`
		Asks [][]any `json:"asks"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return defter{}, err
	}
	var d defter
	for _, x := range r.Bids {
		if len(x) >= 2 {
			d.Alis = append(d.Alis, defterSeviye{jsonSayi(x[0]), jsonSayi(x[1])})
		}
	}
	for _, x := range r.Asks {
		if len(x) >= 2 {
			d.Satis = append(d.Satis, defterSeviye{jsonSayi(x[0]), jsonSayi(x[1])})
		}
	}
	return d, nil
}

func (m *mexcPiyasa) SonFiyat(sembol string) (float64, error) {
	b, err := piyasaGet("/api/v3/ticker/price", url.Values{"symbol": {sembol}})
	if err != nil {
		return 0, err
	}
	var r map[string]any
	if err := json.Unmarshal(b, &r); err != nil {
		return 0, err
	}
	f := jsonSayi(r["price"])
	if f <= 0 {
		return 0, fmt.Errorf("%s fiyatı okunamadı", sembol)
	}
	return f, nil
}

// 0.8: izleyicinin 2 sn'de bir sorduğu fiyat: tek deneme, 5 sn zaman aşımı. (SonFiyat 3 deneme × 20 sn bekleyebilir;
// MEXC bir an takılınca izleyici bir dakika boyunca hiçbir kumbaranın zarar durdurunu denetleyemiyordu.)
var fiyatIstemci = &http.Client{Timeout: 5 * time.Second}

func (m *mexcPiyasa) hizliFiyat(sembol string) (float64, error) {
	req, _ := http.NewRequest("GET", mexcAdresi+"/api/v3/ticker/price?"+url.Values{"symbol": {sembol}}.Encode(), nil)
	req.Header.Set("User-Agent", "KriptoAlSat/"+surumAdi)
	r, err := fiyatIstemci.Do(req)
	if err != nil {
		return 0, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		return 0, err
	}
	if r.StatusCode != 200 {
		return 0, fmt.Errorf("sunucu %d döndü", r.StatusCode)
	}
	var j map[string]any
	if err := json.Unmarshal(b, &j); err != nil {
		return 0, err
	}
	f := jsonSayi(j["price"])
	if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("%s fiyatı okunamadı", sembol)
	}
	return f, nil
}

// İzleyicinin fiyatları: bütün coinler aynı anda sorulur (biri takılırsa diğerleri beklemez)
func izleyiciFiyatlari(semboller map[string]bool) map[string]float64 {
	var mu sync.Mutex
	var wg sync.WaitGroup
	out := map[string]float64{}
	for s := range semboller {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			var f float64
			var err error
			if mp, ok := piyasa.(*mexcPiyasa); ok {
				f, err = mp.hizliFiyat(s)
			} else {
				f, err = piyasa.SonFiyat(s)
			}
			if err == nil && f > 0 {
				mu.Lock()
				out[s] = f
				mu.Unlock()
			}
		}(s)
	}
	wg.Wait()
	return out
}

// Coin kuralları (10 dakika önbellekte)
func (m *mexcPiyasa) Kurallar(sembol string) (sembolKurali, error) {
	m.mu.Lock()
	if kk, ok := m.kurallar[sembol]; ok && time.Since(kk.zaman) < 10*time.Minute {
		m.mu.Unlock()
		return kk.k, nil
	}
	m.mu.Unlock()
	b, err := piyasaGet("/api/v3/exchangeInfo", url.Values{"symbol": {sembol}})
	if err != nil {
		return sembolKurali{}, err
	}
	k, err := kuralCoz(b, sembol)
	if err != nil {
		return k, err
	}
	m.mu.Lock()
	m.kurallar[sembol] = kuralKaydi{k, time.Now()}
	m.mu.Unlock()
	return k, nil
}

func kuralCoz(b []byte, sembol string) (sembolKurali, error) {
	var r struct {
		Symbols []map[string]any `json:"symbols"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return sembolKurali{}, err
	}
	for _, s := range r.Symbols {
		if ad, _ := s["symbol"].(string); ad != sembol {
			continue
		}
		k := sembolKurali{Sembol: sembol, TutarOndalik: 2}
		k.Varlik, _ = s["baseAsset"].(string)
		durum := fmt.Sprint(s["status"])
		izin, _ := s["isSpotTradingAllowed"].(bool)
		k.Acik = (durum == "1" || durum == "ENABLED" || durum == "TRADING") && izin
		k.FiyatOndalik = int(jsonSayi(s["quotePrecision"]))
		k.MiktarOndalik = int(jsonSayi(s["baseAssetPrecision"]))
		if v := int(jsonSayi(s["quoteAssetPrecision"])); v > 0 {
			k.TutarOndalik = v
		}
		if k.TutarOndalik > 8 {
			k.TutarOndalik = 8
		}
		k.EnAzMiktar = jsonSayi(s["baseSizePrecision"])
		k.EnAzTutar = jsonSayi(s["quoteAmountPrecision"])
		k.EnAzTutarPiyasa = jsonSayi(s["quoteAmountPrecisionMarket"])
		if k.EnAzTutarPiyasa <= 0 {
			k.EnAzTutarPiyasa = k.EnAzTutar
		}
		k.TarafTuru = int(jsonSayi(s["tradeSideType"]))
		k.Maker = jsonSayi(s["makerCommission"])
		k.Taker = jsonSayi(s["takerCommission"])
		if s["takerCommission"] == nil {
			k.Taker = 0.0005
		}
		if t, ok := s["orderTypes"].([]any); ok {
			for _, x := range t {
				k.Turler = append(k.Turler, fmt.Sprint(x))
			}
		}
		if k.FiyatOndalik <= 0 || k.FiyatOndalik > 12 || k.MiktarOndalik < 0 || k.MiktarOndalik > 12 {
			return k, fmt.Errorf("%s için fiyat/miktar hassasiyeti okunamadı", sembol)
		}
		if k.EnAzMiktar <= 0 {
			k.EnAzMiktar = k.adim()
		}
		return k, nil
	}
	return sembolKurali{}, fmt.Errorf("MEXC'de %s bulunamadı", sembol)
}
