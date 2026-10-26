package main

// GERÇEK BORSA — MEXC spot API'si (imzalı istekler).
// Anahtar tarayıcıya hiç gitmez; bütün imzalı istekler buradan yapılır.
// İmza: HMAC SHA256 (gizli anahtarla) sorgu dizesinin üzerine, başlıkta X-MEXC-APIKEY.
// Gereken izinler: SPOT_ACCOUNT_READ, SPOT_DEAL_READ, SPOT_DEAL_WRITE. Para çekme ya da transfer izni olan anahtar KULLANILMAZ.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type mexcHata struct {
	Kod   int
	Mesaj string
	HTTP  int
}

func (e mexcHata) Error() string {
	if t, ok := mexcHataTurkce[e.Kod]; ok {
		return fmt.Sprintf("%s (MEXC kodu %d: %s)", t, e.Kod, e.Mesaj)
	}
	return fmt.Sprintf("MEXC: %s (kod %d)", e.Mesaj, e.Kod)
}

var mexcHataTurkce = map[int]string{
	10101:  "Bakiye yetersiz",
	30004:  "Pozisyon çok küçük",
	30005:  "Satılacak miktar bakiyeden fazla",
	30002:  "Emir miktarı en az miktarın altında",
	10007:  "Bu coin API ile alınıp satılamıyor",
	30020:  "Bu coinde işlem iznin yok",
	30016:  "Bu coinde işlem kapalı",
	30000:  "Bu coinde işlem durduruldu",
	70011:  "API ile alım-satım bu coin için kısıtlı",
	700007: "API anahtarının bu iş için izni yok",
	700006: "Bu bilgisayarın IP adresi API anahtarında izinli değil",
	700003: "Bilgisayarın saati MEXC ile uyuşmuyor",
	700002: "İmza hatalı (anahtar ya da gizli anahtar yanlış olabilir)",
	602:    "İmza hatalı (anahtar ya da gizli anahtar yanlış olabilir)",
	10072:  "API anahtarı geçersiz",
	-2011:  "Emir bulunamadı (dolmuş ya da iptal edilmiş olabilir)",
	-2013:  "Emir bulunamadı",
	429:    "Çok sık istek (MEXC kısa süre bekletiyor)",
}

type mexcIstemci struct {
	erisim, gizli string
	saatFarki     atomic.Int64 // MEXC saati − bilgisayar saati (ms)
	http          *http.Client
}

func yeniMexcIstemci(erisim, gizli string) *mexcIstemci {
	return &mexcIstemci{erisim: strings.TrimSpace(erisim), gizli: strings.TrimSpace(gizli), http: &http.Client{Timeout: 15 * time.Second}}
}

func hmacHex(gizli, veri string) string {
	h := hmac.New(sha256.New, []byte(gizli))
	h.Write([]byte(veri))
	return hex.EncodeToString(h.Sum(nil))
}

// MEXC saatini öğren (imzalı isteklerde zaman damgası MEXC'ninkine yakın olmalı)
func (m *mexcIstemci) saatEsitle() error {
	once := time.Now().UnixMilli()
	b, err := getJSON(mexcAdresi + "/api/v3/time")
	if err != nil {
		return err
	}
	sonra := time.Now().UnixMilli()
	var r struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.ServerTime == 0 {
		return fmt.Errorf("MEXC saati okunamadı")
	}
	m.saatFarki.Store(r.ServerTime - (once+sonra)/2)
	return nil
}

func (m *mexcIstemci) istek(method, yol string, q url.Values, imzali bool) ([]byte, error) {
	if q == nil {
		q = url.Values{}
	}
	var qs string
	if imzali {
		q.Set("recvWindow", "10000")
		q.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli()+m.saatFarki.Load(), 10))
		qs = q.Encode()
		qs += "&signature=" + hmacHex(m.gizli, qs)
	} else {
		qs = q.Encode()
	}
	u := mexcAdresi + yol
	if qs != "" {
		u += "?" + qs
	}
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "KriptoAlSat/"+surumAdi)
	req.Header.Set("Content-Type", "application/json")
	if imzali {
		req.Header.Set("X-MEXC-APIKEY", m.erisim)
	}
	r, err := m.http.Do(req)
	if err != nil {
		// 0.8: Go'nun hata metni isteğin tam adresini içerir (apiKeyInfo'da erişim anahtarı da var); kayda ve sayfaya
		// adres yerine sadece yol yazılır. Zaman aşımı bilgisi korunur.
		var ue *url.Error
		if errors.As(err, &ue) {
			return nil, agHatasi{yol: method + " " + yol, ic: ue.Err, zaman: ue.Timeout()}
		}
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		var e struct {
			Code json.Number `json:"code"`
			Msg  string      `json:"msg"`
		}
		_ = json.Unmarshal(b, &e)
		kod, _ := strconv.Atoi(e.Code.String())
		if e.Msg == "" {
			e.Msg = fmt.Sprintf("HTTP %d", r.StatusCode)
		}
		return nil, mexcHata{Kod: kod, Mesaj: e.Msg, HTTP: r.StatusCode}
	}
	return b, nil
}

// Ağ hatası (bağlantı koptu, zaman aşımı): isteğin MEXC'ye ulaşıp ulaşmadığı bilinmez.
type agHatasi struct {
	yol   string
	ic    error
	zaman bool
}

func (e agHatasi) Error() string { return "MEXC bağlantı hatası (" + e.yol + "): " + e.ic.Error() }
func (e agHatasi) Unwrap() error { return e.ic }
func (e agHatasi) Timeout() bool { return e.zaman }

// İmzalı istek; zaman damgası hatasında saati eşitleyip bir kez daha dener.
func (m *mexcIstemci) imzali(method, yol string, q url.Values) ([]byte, error) {
	kopya := url.Values{}
	for k, v := range q {
		kopya[k] = append([]string(nil), v...)
	}
	b, err := m.istek(method, yol, q, true)
	if he, ok := err.(mexcHata); ok && (he.Kod == 700003 || he.Kod == 10073) {
		if m.saatEsitle() == nil {
			return m.istek(method, yol, kopya, true)
		}
	}
	return b, err
}

// ---------- hesap ve anahtar bilgisi ----------

type anahtarBilgisi struct {
	Izinler  []string `json:"izinler"`
	KalanGun int      `json:"kalanGun"` // -999: süresiz
	// 0.8: MEXC kalan günü bildirdi mi (bildirmediyse KalanGun -1: bilinmiyor)
	KalanBiliniyor bool   `json:"-"`
	IPListesi      string `json:"ipListesi"`
	Durum          string `json:"durum"`
}

func (m *mexcIstemci) anahtarBilgisi() (anahtarBilgisi, error) {
	b, err := m.imzali("GET", "/api/v3/apiKeyInfo", url.Values{"accessKey": {m.erisim}})
	if err != nil {
		return anahtarBilgisi{}, err
	}
	var ham map[string]any
	if err := json.Unmarshal(b, &ham); err != nil {
		return anahtarBilgisi{}, fmt.Errorf("anahtar bilgisi okunamadı")
	}
	if v, ok := ham["data"].(map[string]any); ok { // bazı yanıtlar "data" içinde gelir
		ham = v
	}
	var ab anahtarBilgisi
	switch p := ham["permissions"].(type) {
	case string:
		for _, x := range strings.Split(p, ",") {
			if x = strings.TrimSpace(x); x != "" {
				ab.Izinler = append(ab.Izinler, strings.ToUpper(x))
			}
		}
	case []any:
		for _, x := range p {
			ab.Izinler = append(ab.Izinler, strings.ToUpper(strings.TrimSpace(fmt.Sprint(x))))
		}
	}
	// 0.8: alan hiç yoksa "süresi dolmuş" (0) sayılmaz: MEXC cevabın biçimini değiştirirse çalışan bağlantı kesilmesin
	if v, ok := ham["remainingValidity"]; ok && v != nil {
		ab.KalanGun, ab.KalanBiliniyor = int(jsonSayi(v)), true
	} else {
		ab.KalanGun = -1
	}
	ab.IPListesi = fmt.Sprint(ham["ipWhiteList"])
	if ab.IPListesi == "<nil>" {
		ab.IPListesi = ""
	}
	ab.Durum = fmt.Sprint(ham["status"])
	return ab, nil
}

// İzin denetimi: sorun varsa açıklama döner (boş = uygun)
// Para çekme ya da transfer ile ilgili HER izin yasaktır (sadece görme izni bile): kullanıcının isteği
// "sadece al-sat izni olsun". Gereken üç izinden biri eksikse de anahtar kullanılmaz.
func izinDenetle(izinler []string) (yasak []string, eksik []string) {
	olan := map[string]bool{}
	for _, x := range izinler {
		x = strings.ToUpper(strings.TrimSpace(x))
		olan[x] = true
		if strings.Contains(x, "WITHDRAW") || strings.Contains(x, "TRANSFER") {
			yasak = append(yasak, x)
		}
	}
	for _, g := range []string{"SPOT_ACCOUNT_READ", "SPOT_DEAL_READ", "SPOT_DEAL_WRITE"} {
		if !olan[g] {
			eksik = append(eksik, g)
		}
	}
	return
}

type bakiye struct{ Serbest, Kilitli float64 }

func (m *mexcIstemci) bakiyeler() (map[string]bakiye, error) {
	b, err := m.imzali("GET", "/api/v3/account", nil)
	if err != nil {
		return nil, err
	}
	var r struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   any    `json:"free"`
			Locked any    `json:"locked"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("bakiye okunamadı")
	}
	out := map[string]bakiye{}
	for _, x := range r.Balances {
		out[x.Asset] = bakiye{jsonSayi(x.Free), jsonSayi(x.Locked)}
	}
	return out, nil
}

// Kullanıcının API ile işlem yapabildiği coinler (boş liste = bilgi yok)
func (m *mexcIstemci) apiSembolleri() ([]string, error) {
	b, err := m.imzali("GET", "/api/v3/selfSymbols", nil)
	if err != nil {
		return nil, err
	}
	return sembolListesiCoz(b)
}

func varsayilanApiSembolleri() ([]string, error) {
	b, err := piyasaGet("/api/v3/defaultSymbols", nil)
	if err != nil {
		return nil, err
	}
	return sembolListesiCoz(b)
}

func sembolListesiCoz(b []byte) ([]string, error) {
	var r struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	sort.Strings(r.Data)
	return r.Data, nil
}

func (m *mexcIstemci) komisyon(sembol string) (maker, taker float64, err error) {
	b, err := m.imzali("GET", "/api/v3/tradeFee", url.Values{"symbol": {sembol}})
	if err != nil {
		return 0, 0, err
	}
	var r struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(b, &r); err != nil || r.Data == nil {
		return 0, 0, fmt.Errorf("komisyon okunamadı")
	}
	return jsonSayi(r.Data["makerCommission"]), jsonSayi(r.Data["takerCommission"]), nil
}

// Deneme emri: MEXC'ye gönderilir ama eşleştirilmez (izin ve coin kontrolü için)
func (m *mexcIstemci) denemeEmri(k sembolKurali, fiyat, miktar float64) error {
	q := url.Values{"symbol": {k.Sembol}, "side": {"BUY"}, "type": {"LIMIT"},
		"quantity": {sayiMetni(miktar, k.MiktarOndalik)}, "price": {sayiMetni(fiyat, k.FiyatOndalik)}}
	_, err := m.imzali("POST", "/api/v3/order/test", q)
	return err
}

// ---------- borsa arayüzü (gerçek emirler) ----------

type mexcBorsa struct {
	ist *mexcIstemci
	p   piyasaKaynagi
}

func (g *mexcBorsa) Ad() string { return "gercek" }

func idMetni(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case json.Number:
		return x.String()
	}
	return fmt.Sprint(v)
}

// MEXC emri KESİN reddetti mi? Sadece 4xx cevap (iş kuralı: bakiye, en az tutar, imza, istek sınırı) kesindir;
// emir eşleştirmeye hiç girmemiştir. Ağ hatası, zaman aşımı ve 5xx (ağ geçidi) cevaplarında emir MEXC'ye ulaşmış
// olabilir: akıbeti belirsizdir.
func kesinRed(err error) bool {
	var he mexcHata
	return errors.As(err, &he) && he.HTTP >= 400 && he.HTTP < 500
}

// "Böyle emir yok" cevabı (istemci kimliğiyle sorguda)
func emirYok(err error) bool {
	var he mexcHata
	return errors.As(err, &he) && (he.Kod == -2013 || he.Kod == -2011 || he.HTTP == 404)
}

// Emir gönderilirken cevap gelmediyse (bağlantı koptu, 5xx) emir MEXC'ye ulaşmış olabilir: istemci kimliğiyle sorulur.
// (Aynı emri körlemesine yeniden göndermek iki kez alıp satmaya yol açabilir.)
// 0.8: emir bulunursa numarası; MEXC iki kez arka arkaya "böyle emir yok" derse emir verilmemiştir (asıl hata döner);
// hiçbir şey öğrenilemezse belirsizHata (çağıran piyasa emrine GEÇMEZ; kumbara sonra MEXC'ye yeniden sorar).
// 0.7'de her MEXC hatası (502 dahil) ve sorgunun cevapsız kalması "emir yok" sayılıyordu: limit emir açıkken aynı
// bütçeyle bir de piyasa emri verilebiliyordu.
func (g *mexcBorsa) gonderilenEmir(k sembolKurali, istemciID string, err error) (string, error) {
	if kesinRed(err) {
		return "", err // MEXC emri reddetti: emir yok
	}
	yokSayisi := 0
	for deneme := 0; deneme < 6; deneme++ {
		uyu(time.Second)
		d, e2 := g.IstemciEmri(k, istemciID)
		if e2 == nil && d.ID != "" && d.ID != "<nil>" {
			return d.ID, nil
		}
		if emirYok(e2) {
			if yokSayisi++; yokSayisi >= 2 {
				return "", err // MEXC iki kez "böyle emir yok" dedi: emir verilmemiş
			}
			continue
		}
		yokSayisi = 0
	}
	return "", belirsizHata{"emir gönderilirken MEXC'den cevap gelmedi ve emrin verilip verilmediği öğrenilemedi (" + err.Error() + "); güvenlik için başka emir verilmedi, MEXC'ye sonra yeniden sorulacak"}
}

func (g *mexcBorsa) LimitEmir(k sembolKurali, taraf string, fiyat, miktar float64, istemciID string) (string, error) {
	q := url.Values{"symbol": {k.Sembol}, "side": {taraf}, "type": {"LIMIT"},
		"quantity": {sayiMetni(miktar, k.MiktarOndalik)}, "price": {sayiMetni(fiyat, k.FiyatOndalik)}, "newClientOrderId": {istemciID}}
	b, err := g.ist.imzali("POST", "/api/v3/order", q)
	if err != nil {
		return g.gonderilenEmir(k, istemciID, err)
	}
	var r map[string]any
	id := ""
	if json.Unmarshal(b, &r) == nil {
		id = idMetni(r["orderId"])
	}
	if id == "" || id == "<nil>" {
		// emir kabul edildi ama cevap okunamadı: istemci kimliğiyle sor
		return g.gonderilenEmir(k, istemciID, fmt.Errorf("emir yanıtı okunamadı"))
	}
	return id, nil
}

func emirCoz(b []byte) (emirDurumu, error) {
	var r map[string]any
	if err := json.Unmarshal(b, &r); err != nil {
		return emirDurumu{}, fmt.Errorf("emir durumu okunamadı")
	}
	e := emirDurumu{ID: idMetni(r["orderId"]), IstemciID: fmt.Sprint(r["clientOrderId"]), Durum: fmt.Sprint(r["status"]), Dolan: jsonSayi(r["executedQty"])}
	e.DolanTutar = jsonSayi(r["cummulativeQuoteQty"])
	if e.DolanTutar == 0 {
		e.DolanTutar = jsonSayi(r["cumulativeQuoteQty"])
	}
	return e, nil
}

func (g *mexcBorsa) EmirDurumu(k sembolKurali, id string) (emirDurumu, error) {
	b, err := g.ist.imzali("GET", "/api/v3/order", url.Values{"symbol": {k.Sembol}, "orderId": {id}})
	if err != nil {
		return emirDurumu{}, err
	}
	return emirCoz(b)
}

// İstemci kimliğiyle sorgu (program çökerse yarım kalan emri bulmak için)
func (g *mexcBorsa) IstemciEmri(k sembolKurali, istemciID string) (emirDurumu, error) {
	b, err := g.ist.imzali("GET", "/api/v3/order", url.Values{"symbol": {k.Sembol}, "origClientOrderId": {istemciID}})
	if err != nil {
		return emirDurumu{}, err
	}
	return emirCoz(b)
}

func (g *mexcBorsa) Iptal(k sembolKurali, id string) (emirDurumu, error) {
	_, err := g.ist.imzali("DELETE", "/api/v3/order", url.Values{"symbol": {k.Sembol}, "orderId": {id}})
	if err != nil {
		if he, ok := err.(mexcHata); !ok || he.Kod != -2011 {
			// iptal edilemediyse yine de durumu sor
			if d, e2 := g.EmirDurumu(k, id); e2 == nil {
				return d, nil
			}
			return emirDurumu{}, err
		}
	}
	// iptal yanıtında dolan miktar eksik olabilir; son durumu sor
	for deneme := 0; deneme < 3; deneme++ {
		d, e2 := g.EmirDurumu(k, id)
		if e2 == nil && d.bitti() {
			return d, nil
		}
		uyu(500 * time.Millisecond)
	}
	return g.EmirDurumu(k, id)
}

func (g *mexcBorsa) PiyasaEmri(k sembolKurali, taraf string, miktar, tutar float64, istemciID string) (emirDurumu, error) {
	q := url.Values{"symbol": {k.Sembol}, "side": {taraf}, "type": {"MARKET"}, "newClientOrderId": {istemciID}}
	if taraf == "BUY" {
		q.Set("quoteOrderQty", sayiMetni(tutar, k.TutarOndalik))
	} else {
		q.Set("quantity", sayiMetni(miktar, k.MiktarOndalik))
	}
	b, err := g.ist.imzali("POST", "/api/v3/order", q)
	var he mexcHata
	if err != nil && errors.As(err, &he) && he.Kod == 10101 {
		// limit emrin iptalinden sonra serbest kalan para birkaç yüz ms gecikebilir
		uyu(1500 * time.Millisecond)
		b, err = g.ist.imzali("POST", "/api/v3/order", q)
	}
	var id string
	if err != nil {
		var e2 error
		if id, e2 = g.gonderilenEmir(k, istemciID, err); e2 != nil {
			return emirDurumu{}, e2
		}
	} else {
		var r map[string]any
		if json.Unmarshal(b, &r) == nil {
			id = idMetni(r["orderId"])
		}
		if id == "" || id == "<nil>" {
			var e2 error
			if id, e2 = g.gonderilenEmir(k, istemciID, fmt.Errorf("emir yanıtı okunamadı")); e2 != nil {
				return emirDurumu{}, e2
			}
		}
	}
	var son emirDurumu
	okundu := false
	for deneme := 0; deneme < 16; deneme++ {
		d, err := g.EmirDurumu(k, id)
		if err == nil {
			son, okundu = d, true
			if d.bitti() {
				return d, nil
			}
		}
		uyu(500 * time.Millisecond)
	}
	// 0.8: piyasa emri verildi ama sonu öğrenilemedi (durum okunamadı ya da hâlâ açık): "dolmadı" SAYILMAZ.
	// 0.7'de bu durumda 0 dolmuş dönüyordu; kumbara alımı unutup sonraki sinyalde ikinci kez alabiliyordu.
	if son.ID == "" {
		son.ID = id
	}
	if !okundu {
		return son, belirsizHata{"piyasa emri verildi ama sonucu MEXC'den okunamadı; MEXC'ye sonra yeniden sorulacak"}
	}
	return son, belirsizHata{"piyasa emri verildi ama MEXC'de hâlâ açık görünüyor (" + son.Durum + "); MEXC'ye sonra yeniden sorulacak"}
}

func (g *mexcBorsa) Dolumlar(k sembolKurali, id string) ([]parca, error) {
	b, err := g.ist.imzali("GET", "/api/v3/myTrades", url.Values{"symbol": {k.Sembol}, "orderId": {id}, "limit": {"100"}})
	if err != nil {
		return nil, err
	}
	var ham []map[string]any
	if err := json.Unmarshal(b, &ham); err != nil {
		return nil, fmt.Errorf("dolumlar okunamadı")
	}
	var out []parca
	mxFiyat := 0.0
	for _, x := range ham {
		if oid := idMetni(x["orderId"]); oid != "" && oid != "<nil>" && oid != id {
			continue
		}
		p := parca{Fiyat: jsonSayi(x["price"]), Miktar: jsonSayi(x["qty"]), Tutar: jsonSayi(x["quoteQty"]),
			Komisyon: jsonSayi(x["commission"]), KomisyonVarligi: fmt.Sprint(x["commissionAsset"]), Zaman: int64(jsonSayi(x["time"]))}
		p.Maker, _ = x["isMaker"].(bool)
		if p.Tutar == 0 {
			p.Tutar = p.Fiyat * p.Miktar
		}
		switch p.KomisyonVarligi {
		case "USDT":
			p.KomisyonUSDT = p.Komisyon
		case k.Varlik:
			p.KomisyonUSDT = p.Komisyon * p.Fiyat
		case "MX":
			if mxFiyat == 0 {
				mxFiyat, _ = g.p.SonFiyat("MXUSDT")
			}
			p.KomisyonUSDT = p.Komisyon * mxFiyat
		}
		out = append(out, p)
	}
	return out, nil
}
