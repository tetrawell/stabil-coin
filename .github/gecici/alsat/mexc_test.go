package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- sahte MEXC (imza denetimli) ----------

type sahteEmir struct {
	ID, Istemci, Sembol, Taraf, Tur, Durum string
	Fiyat, Miktar, Dolan, DolanTutar       float64
	Dolumlar                               []map[string]any
}

type imzaliMexc struct {
	mu         sync.Mutex
	gizli      string
	izinler    string
	sayac      int
	emirler    map[string]*sahteEmir
	limitOran  float64 // limit emrin verildiği anda dolan kısmı
	iptalHata  bool
	bakiye     map[string][2]float64
	istekler   []string
	imzaHatasi int
}

func yeniImzaliMexc() *imzaliMexc {
	return &imzaliMexc{gizli: "gizli-anahtar", izinler: "SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE",
		emirler: map[string]*sahteEmir{}, bakiye: map[string][2]float64{"USDT": {500, 0}, "AGT": {0, 0}}}
}

func (s *imzaliMexc) yaz(w http.ResponseWriter, kod int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(kod)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *imzaliMexc) emirJSON(e *sahteEmir) map[string]any {
	return map[string]any{"symbol": e.Sembol, "orderId": e.ID, "clientOrderId": e.Istemci, "status": e.Durum,
		"executedQty": fmt.Sprint(e.Dolan), "cummulativeQuoteQty": fmt.Sprint(e.DolanTutar), "price": fmt.Sprint(e.Fiyat), "origQty": fmt.Sprint(e.Miktar)}
}

func (s *imzaliMexc) doldur(e *sahteEmir, miktar, fiyat float64, maker bool) {
	if miktar <= 0 {
		return
	}
	e.Dolan += miktar
	e.DolanTutar += miktar * fiyat
	kom, varlik := miktar*0.0005, "AGT"
	if e.Taraf == "SELL" {
		kom, varlik = miktar*fiyat*0.0005, "USDT"
	}
	if maker {
		kom = 0
	}
	e.Dolumlar = append(e.Dolumlar, map[string]any{"orderId": e.ID, "price": fmt.Sprint(fiyat), "qty": fmt.Sprint(miktar), "quoteQty": fmt.Sprint(miktar * fiyat),
		"commission": fmt.Sprint(kom), "commissionAsset": varlik, "isMaker": maker, "time": time.Now().UnixMilli()})
	if e.Dolan >= e.Miktar-1e-12 {
		e.Durum = "FILLED"
	} else {
		e.Durum = "PARTIALLY_FILLED"
	}
}

func (s *imzaliMexc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.istekler = append(s.istekler, r.Method+" "+r.URL.Path)
	q := r.URL.Query()
	yol := r.URL.Path
	switch yol {
	case "/api/v3/time":
		s.yaz(w, 200, map[string]any{"serverTime": time.Now().UnixMilli()})
		return
	case "/api/v3/exchangeInfo":
		s.yaz(w, 200, map[string]any{"symbols": []map[string]any{{"symbol": "AGTUSDT", "status": "1", "baseAsset": "AGT", "isSpotTradingAllowed": true,
			"quotePrecision": 3, "baseAssetPrecision": 2, "quoteAssetPrecision": 2, "baseSizePrecision": "0.01", "quoteAmountPrecision": "1",
			"quoteAmountPrecisionMarket": "1", "makerCommission": "0", "takerCommission": "0.0005", "orderTypes": []string{"LIMIT", "MARKET"}}}})
		return
	case "/api/v3/ticker/bookTicker":
		s.yaz(w, 200, map[string]any{"symbol": "AGTUSDT", "bidPrice": "1.000", "bidQty": "500", "askPrice": "1.010", "askQty": "500"})
		return
	case "/api/v3/ticker/price":
		s.yaz(w, 200, map[string]any{"symbol": q.Get("symbol"), "price": "1.005"})
		return
	}
	// imzalı uçlar
	if r.Header.Get("X-MEXC-APIKEY") != "mx0erisim" {
		s.yaz(w, 400, map[string]any{"code": 10072, "msg": "Api key info invalid"})
		return
	}
	ham := r.URL.RawQuery
	i := strings.LastIndex(ham, "&signature=")
	if i < 0 || hmacHex(s.gizli, ham[:i]) != ham[i+len("&signature="):] {
		s.imzaHatasi++
		s.yaz(w, 400, map[string]any{"code": 700002, "msg": "Signature for this request is not valid."})
		return
	}
	switch {
	case yol == "/api/v3/apiKeyInfo":
		s.yaz(w, 200, map[string]any{"note": "test", "accessKey": q.Get("accessKey"), "permissions": s.izinler, "remainingValidity": 66, "ipWhiteList": "", "status": "VALID"})
	case yol == "/api/v3/account":
		var b []map[string]any
		for ad, x := range s.bakiye {
			b = append(b, map[string]any{"asset": ad, "free": fmt.Sprint(x[0]), "locked": fmt.Sprint(x[1])})
		}
		s.yaz(w, 200, map[string]any{"balances": b})
	case yol == "/api/v3/tradeFee":
		s.yaz(w, 200, map[string]any{"data": map[string]any{"makerCommission": 0, "takerCommission": 0.0005}, "code": 0})
	case yol == "/api/v3/selfSymbols":
		s.yaz(w, 200, map[string]any{"code": 200, "data": []string{"AGTUSDT"}})
	case yol == "/api/v3/order/test":
		s.yaz(w, 200, map[string]any{})
	case yol == "/api/v3/order" && r.Method == "POST":
		s.sayac++
		e := &sahteEmir{ID: "C" + strconv.Itoa(s.sayac), Istemci: q.Get("newClientOrderId"), Sembol: q.Get("symbol"), Taraf: q.Get("side"), Tur: q.Get("type"), Durum: "NEW"}
		e.Fiyat, _ = strconv.ParseFloat(q.Get("price"), 64)
		e.Miktar, _ = strconv.ParseFloat(q.Get("quantity"), 64)
		if e.Tur == "MARKET" {
			f := 1.010
			if e.Taraf == "SELL" {
				f = 1.000
			}
			if qq := q.Get("quoteOrderQty"); qq != "" {
				t, _ := strconv.ParseFloat(qq, 64)
				e.Miktar = t / f
			}
			s.doldur(e, e.Miktar, f, false)
		} else if s.limitOran > 0 {
			s.doldur(e, asagiYuvarla(e.Miktar*s.limitOran, 2), e.Fiyat, true)
		}
		s.emirler[e.ID] = e
		s.yaz(w, 200, map[string]any{"orderId": e.ID, "symbol": e.Sembol})
	case yol == "/api/v3/order" && r.Method == "GET":
		var e *sahteEmir
		if id := q.Get("orderId"); id != "" {
			e = s.emirler[id]
		} else {
			for _, x := range s.emirler {
				if x.Istemci == q.Get("origClientOrderId") {
					e = x
				}
			}
		}
		if e == nil {
			s.yaz(w, 400, map[string]any{"code": -2013, "msg": "Order does not exist."})
			return
		}
		s.yaz(w, 200, s.emirJSON(e))
	case yol == "/api/v3/order" && r.Method == "DELETE":
		e := s.emirler[q.Get("orderId")]
		if e == nil || s.iptalHata {
			s.yaz(w, 400, map[string]any{"code": -2011, "msg": "Unknown order sent."})
			return
		}
		if e.Durum == "NEW" {
			e.Durum = "CANCELED"
		} else if e.Durum == "PARTIALLY_FILLED" {
			e.Durum = "PARTIALLY_CANCELED"
		}
		s.yaz(w, 200, s.emirJSON(e))
	case yol == "/api/v3/myTrades":
		e := s.emirler[q.Get("orderId")]
		out := []map[string]any{}
		if e != nil {
			out = e.Dolumlar
		}
		s.yaz(w, 200, out)
	default:
		s.yaz(w, 404, map[string]any{"code": 404, "msg": "yok"})
	}
}

func imzaliMexcKur(t *testing.T) *imzaliMexc {
	s := yeniImzaliMexc()
	sv := httptest.NewServer(s)
	eski := mexcAdresi
	mexcAdresi = sv.URL
	eskiP := piyasa
	piyasa = yeniMexcPiyasa()
	t.Cleanup(func() { sv.Close(); mexcAdresi = eski; piyasa = eskiP; G.kes("") })
	return s
}

func TestAnahtarParaCekmeIzniReddedilir(t *testing.T) {
	s := imzaliMexcKur(t)
	s.izinler = "SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE,SPOT_WITHDRAW_WRITE"
	r := G.dene("mx0erisim", "gizli-anahtar")
	if r.Tamam || len(r.Yasak) != 1 || !strings.Contains(r.Hata, "KULLANILMADI") {
		t.Fatalf("para çekme izinli anahtar reddedilmeliydi: %+v", r)
	}
	if G.istemci() != nil {
		t.Fatal("bağlantı kurulmamalıydı")
	}
}

func TestAnahtarEksikIzinVeYanlisGizli(t *testing.T) {
	s := imzaliMexcKur(t)
	s.izinler = "SPOT_ACCOUNT_READ"
	if r := G.dene("mx0erisim", "gizli-anahtar"); r.Tamam || len(r.Eksik) != 2 {
		t.Fatalf("eksik izin: %+v", r)
	}
	s.izinler = "SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE"
	if r := G.dene("mx0erisim", "yanlis"); r.Tamam || !strings.Contains(r.Hata, "İmza") {
		t.Fatalf("yanlış gizli anahtar: %+v", r)
	}
	r := G.dene("mx0erisim", "gizli-anahtar")
	if !r.Tamam || G.usdtSerbest() != 500 {
		t.Fatalf("uygun anahtar kabul edilmeli: %+v %v", r, G.usdtSerbest())
	}
}

func TestGercekLimitYarimSonraPiyasa(t *testing.T) {
	s := imzaliMexcKur(t)
	saatKur(t)
	s.limitOran = 0.4
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	kural, err := piyasa.Kurallar("AGTUSDT")
	if err != nil {
		t.Fatal(err)
	}
	d, err := limitSonraPiyasa(G.borsa, piyasa, kural, "BUY", 100, 0, "asTEST1", sessizOlay)
	if err != nil {
		t.Fatal(err)
	}
	if d.Yol != "limit+piyasa" || len(d.Parcalar) != 2 {
		t.Fatalf("yol: %+v", d)
	}
	if d.Parcalar[0].Fiyat != 1.001 || !d.Parcalar[0].Maker || d.Parcalar[1].Fiyat != 1.010 {
		t.Fatalf("parçalar: %+v", d.Parcalar)
	}
	if d.Tutar > 100+1e-9 {
		t.Fatalf("bütçe aşıldı: %v", d.Tutar)
	}
	// emirler istemci kimliğiyle verildi
	var istemciler []string
	for _, e := range s.emirler {
		istemciler = append(istemciler, e.Istemci)
	}
	if !strings.Contains(strings.Join(istemciler, ","), "asTEST1L") || !strings.Contains(strings.Join(istemciler, ","), "asTEST1P") {
		t.Fatalf("istemci kimlikleri: %v", istemciler)
	}
	if s.imzaHatasi > 0 {
		t.Fatalf("%d imza hatası", s.imzaHatasi)
	}
}

func TestYarimKalanGercekEmirKurtarilir(t *testing.T) {
	s := imzaliMexcKur(t)
	saatKur(t)
	veriKlasoru = t.TempDir()
	D = yeniDurum()
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	// program kapanmadan önce limit alış verilmiş ve %40'ı dolmuş (açık duruyor)
	s.mu.Lock()
	e := &sahteEmir{ID: "C99", Istemci: "asabcdef1L", Sembol: "AGTUSDT", Taraf: "BUY", Tur: "LIMIT", Durum: "NEW", Fiyat: 1.001, Miktar: 99.9}
	s.doldur(e, 40, 1.001, true)
	s.emirler[e.ID] = e
	s.mu.Unlock()
	durumMu.Lock()
	k := kumbaraOlustur(acmaIstegi{Dosya: "AGTUSDT_1m.json", Icerik: []byte(`{}`), Sembol: "AGTUSDT", Aralik: "1m", Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000, Mod: "gercek", Tutar: 100})
	k.ID = "abcdef123456"
	k.Bekleyen = &bekleyenEmir{Taraf: "BUY", Onek: "asabcdef1", Baslangic: simdiF().UnixMilli(), MotorMum: 7, Beklenen: 1.004}
	durumMu.Unlock()
	kurtar(k)
	if s.emirler["C99"].Durum != "PARTIALLY_CANCELED" {
		t.Fatalf("açık kalan emir iptal edilmeli: %s", s.emirler["C99"].Durum)
	}
	if k.Bekleyen != nil || k.acikIslem() == nil || k.Adet != 40 || !yakin(k.Nakit, 100-40*1.001, 1e-9) {
		t.Fatalf("dolan kısım kumbaraya işlenmeli: bekleyen %+v adet %v nakit %v", k.Bekleyen, k.Adet, k.Nakit)
	}
	if k.Islemler[0].MotorMum != 7 || k.Islemler[0].BeklenenAlis != 1.004 {
		t.Fatalf("motor bilgisi korunmalı: %+v", k.Islemler[0])
	}
}

func TestIptalEdilemeyenGercekEmir(t *testing.T) {
	s := imzaliMexcKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	s.iptalHata = true
	kural, _ := piyasa.Kurallar("AGTUSDT")
	// MEXC iptal isteğine "bilinmeyen emir" diyor ama emir hâlâ açık görünüyor: piyasa emri verilmemeli
	_, err := limitSonraPiyasa(G.borsa, piyasa, kural, "BUY", 100, 0, "asTEST2", sessizOlay)
	if _, ok := err.(belirsizHata); !ok {
		t.Fatalf("belirsiz hata bekleniyordu: %v", err)
	}
	for _, e := range s.emirler {
		if e.Tur == "MARKET" {
			t.Fatal("iptal kesinleşmeden piyasa emri verildi")
		}
	}
}
