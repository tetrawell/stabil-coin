package main

// 0.8 güvenlik incelemesinin bulduğu sorunların testleri (bağımsız inceleme, 25 Ekim 2026). İmzalı sahte MEXC'ye
// arıza eklenir (502, bağlantı kopması, eksik işlem listesi...). Her test 0.7'de başarısız oluyordu; 0.8'de geçmeli.

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type arizali struct {
	ic   *imzaliMexc
	mu   sync.Mutex
	hook func(w http.ResponseWriter, r *http.Request) bool
}

func (a *arizali) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	h := a.hook
	a.mu.Unlock()
	if h != nil && h(w, r) {
		return
	}
	a.ic.ServeHTTP(w, r)
}

func (a *arizali) kanca(h func(w http.ResponseWriter, r *http.Request) bool) {
	a.mu.Lock()
	a.hook = h
	a.mu.Unlock()
}

func arizaliKur(t *testing.T) (*imzaliMexc, *arizali) {
	s := yeniImzaliMexc()
	a := &arizali{ic: s}
	sv := httptest.NewServer(a)
	eski := mexcAdresi
	mexcAdresi = sv.URL
	eskiP := piyasa
	piyasa = yeniMexcPiyasa()
	t.Cleanup(func() { sv.Close(); mexcAdresi = eski; piyasa = eskiP; G.kes("") })
	return s, a
}

func icIsle(s *imzaliMexc, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, r)
	return rec
}

func baglantiKes(w http.ResponseWriter) {
	hj, _ := w.(http.Hijacker)
	c, _, _ := hj.Hijack()
	c.Close()
}

func emirleriSay(s *imzaliMexc) (limitAcik, piyasaDolu int, harcanan float64, liste []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.emirler {
		liste = append(liste, e.ID+":"+e.Tur+":"+e.Taraf+":"+e.Durum+":"+e.Istemci)
		if e.Tur == "LIMIT" && (e.Durum == "NEW" || e.Durum == "PARTIALLY_FILLED") {
			limitAcik++
		}
		if e.Tur == "MARKET" && e.Durum == "FILLED" {
			piyasaDolu++
		}
		harcanan += e.DolanTutar
	}
	return
}

func gercekKumbara(t *testing.T, tutar float64) *kumbara {
	veriKlasoru = t.TempDir()
	D = yeniDurum()
	durumMu.Lock()
	defer durumMu.Unlock()
	return kumbaraOlustur(acmaIstegi{Dosya: "AGTUSDT_1m.json", Icerik: []byte(`{}`), Sembol: "AGTUSDT", Aralik: "1m",
		Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000, Mod: "gercek", Tutar: tutar})
}

// 1) Limit emir MEXC'ye ulaştı ama cevap 502 döndü: program "emir yok" sayıp aynı bütçeyle piyasa emri veriyor.
func TestInceleme1_LimitBelirsizseIkinciEmir(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	kural, _ := piyasa.Kurallar("AGTUSDT")
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && r.URL.Query().Get("type") == "LIMIT" {
			icIsle(s, r) // emir borsada oluştu
			w.WriteHeader(502)
			w.Write([]byte("<html>502 Bad Gateway</html>"))
			return true
		}
		return false
	})
	d, err := limitSonraPiyasa(G.borsa, piyasa, kural, "BUY", 100, 0, "asTEST9", sessizOlay)
	la, pd, _, liste := emirleriSay(s)
	t.Logf("sonuç err=%v yol=%s tutar=%.2f; borsadaki emirler: %v", err, d.Yol, d.Tutar, liste)
	if la == 1 && pd == 1 {
		t.Errorf("0.7 HATASI GERİ GELDİ: 100 USDT bütçe için borsada açık limit alış (≈100 USDT) + dolmuş piyasa alışı (100 USDT) var")
	}
}

// 1b) Ağ kopması: limit emir gitti, bağlantı koptu, istemci kimliğiyle sorgu da ağ hatası verdi → yine piyasa emri.
func TestInceleme1b_AgKopunca(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	kural, _ := piyasa.Kurallar("AGTUSDT")
	kopuk := false
	var mu sync.Mutex
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && r.URL.Query().Get("type") == "LIMIT" {
			icIsle(s, r)
			kopuk = true
			baglantiKes(w)
			return true
		}
		if kopuk && r.URL.Path == "/api/v3/order" && r.Method == "GET" && r.URL.Query().Get("origClientOrderId") != "" {
			baglantiKes(w) // internet hâlâ kopuk
			return true
		}
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" {
			kopuk = false // internet geri geldi
		}
		return false
	})
	d, err := limitSonraPiyasa(G.borsa, piyasa, kural, "BUY", 100, 0, "asTEST8", sessizOlay)
	la, pd, _, liste := emirleriSay(s)
	t.Logf("sonuç err=%v yol=%s tutar=%.2f; borsadaki emirler: %v", err, d.Yol, d.Tutar, liste)
	if la == 1 && pd == 1 {
		t.Errorf("0.7 HATASI GERİ GELDİ: ağ kopunca 'sorulamadı' = 'emir yok' sayıldı; ikinci (piyasa) emir verildi")
	}
}

// 2) Piyasa emri doldu, ama durum/dolum sorguları birkaç saniye hata verdi → kumbara alımı unutuyor, Bekleyen siliniyor.
func TestInceleme2_DolanPiyasaEmriUnutuluyor(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	k := gercekKumbara(t, 100)
	var mu sync.Mutex
	bozuk := false
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && r.URL.Query().Get("type") == "MARKET" {
			bozuk = true
			return false // emir normal verilir ve dolar
		}
		if bozuk && ((r.URL.Path == "/api/v3/order" && r.Method == "GET") || r.URL.Path == "/api/v3/myTrades") {
			w.WriteHeader(503)
			w.Write([]byte("Service Unavailable"))
			return true
		}
		return false
	})
	kumbaraAl(k, 1.004, 1)
	_, pd, harc, liste := emirleriSay(s)
	t.Logf("1. alış sonrası: bekleyen=%v açıkİşlem=%v nakit=%.2f adet=%.2f borsa=%v harcanan=%.2f", k.Bekleyen, k.acikIslem() != nil, k.Nakit, k.Adet, liste, harc)
	for _, o := range k.Olaylar {
		t.Logf("  olay: %s", o.Mesaj)
	}
	if pd == 1 && k.Bekleyen == nil && k.acikIslem() == nil && k.Nakit == 100 {
		t.Errorf("0.7 HATASI GERİ GELDİ: borsada dolan piyasa alışı kumbarada yok; Bekleyen silindi (kurtar hiç çalışmayacak)")
	}
	mu.Lock()
	bozuk = false
	mu.Unlock()
	a.kanca(nil)
	kumbaraAl(k, 1.004, 2) // motorun sıradaki alımı
	_, pd, harc, _ = emirleriSay(s)
	t.Logf("2. alış sonrası: borsada dolu piyasa alışı=%d, toplam harcanan=%.2f (kumbara 100)", pd, harc)
	if harc > 150 {
		t.Errorf("0.7 HATASI GERİ GELDİ: 100 USDT'lik kumbara borsada %.2f USDT harcadı", harc)
	}
}

// 3) kurtar: iptal başarısız, emir hâlâ açık → Bekleyen yine de siliniyor
func TestInceleme3_KurtarAcikEmriBirakiyor(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	k := gercekKumbara(t, 100)
	k.ID = "abcdef123456"
	s.mu.Lock()
	e := &sahteEmir{ID: "C99", Istemci: "asabcdef1L", Sembol: "AGTUSDT", Taraf: "BUY", Tur: "LIMIT", Durum: "NEW", Fiyat: 1.001, Miktar: 99.9}
	s.doldur(e, 40, 1.001, true)
	s.emirler[e.ID] = e
	s.mu.Unlock()
	durumMu.Lock()
	k.Bekleyen = &bekleyenEmir{Taraf: "BUY", Onek: "asabcdef1", Baslangic: simdiF().UnixMilli(), MotorMum: 7, Beklenen: 1.004}
	durumMu.Unlock()
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/order" && r.Method == "DELETE" {
			w.WriteHeader(503)
			w.Write([]byte("busy"))
			return true
		}
		return false
	})
	kurtar(k)
	t.Logf("kurtar sonrası: bekleyen=%v adet=%.2f nakit=%.2f borsadaki emir=%s", k.Bekleyen, k.Adet, k.Nakit, s.emirler["C99"].Durum)
	if k.Bekleyen == nil && s.emirler["C99"].Durum == "PARTIALLY_FILLED" {
		t.Errorf("0.7 HATASI GERİ GELDİ: iptal edilemeyen emir borsada açık kaldı ama Bekleyen silindi; kalan 59.9 dolarsa kumbara bilmeyecek")
	}
}

// 4) Satışta limit %10 doldu, kalan için piyasa emri reddedildi (429) → işlem kapandı sayılıyor, %90 coin korumasız
func TestInceleme4_YarimSatisKorumasizArtik(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	k := gercekKumbara(t, 100)
	acikIslemKoy(k, 1, 100)
	s.mu.Lock()
	s.bakiye["AGT"] = [2]float64{100, 0}
	s.limitOran = 0.1
	s.mu.Unlock()
	fiyat := "1.005"
	var mu sync.Mutex
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && r.URL.Query().Get("type") == "MARKET" {
			w.WriteHeader(429)
			w.Write([]byte(`{"code":429,"msg":"Too Many Requests"}`))
			return true
		}
		if r.URL.Path == "/api/v3/ticker/price" {
			w.Write([]byte(`{"symbol":"AGTUSDT","price":"` + fiyat + `"}`))
			return true
		}
		return false
	})
	kumbaraSat(k, "motor", "test", 1.0, false)
	t.Logf("satış sonrası: açıkİşlem=%v adet=%.2f nakit=%.2f", k.acikIslem() != nil, k.Adet, k.Nakit)
	durumMu.Lock()
	k.Stop = 0.97
	durumMu.Unlock()
	mu.Lock()
	fiyat = "0.500"
	mu.Unlock()
	liste := eylemYakala(t)
	izleyiciTur()
	t.Logf("fiyat 0.50 (zarar durdur 0.97) iken izleyicinin eylemleri: %+v", *liste)
	if k.acikIslem() == nil && k.Adet > 80 && len(*liste) == 0 {
		t.Errorf("0.7 HATASI GERİ GELDİ: %.2f AGT kumbarada ama açık işlem yok: zarar durdur izlenmiyor", k.Adet)
	}
	durumMu.Lock()
	k.Durum = "sureDoldu"
	durumMu.Unlock()
	izleyiciTur()
	t.Logf("ömür dolunca: durum=%s adet=%.2f iade=%.2f", k.Durum, k.Adet, k.Iade)
	if k.Durum == "bitti" && k.Adet > 80 {
		t.Errorf("0.7 HATASI GERİ GELDİ: kumbara %.2f AGT satılmadan kapandı", k.Adet)
	}
}

// 5) Limit emir iki parça doldu, myTrades bir parçayı gösterdi (gecikme) → kalan bütçe yanlış, kumbaradan fazla harcandı
func TestInceleme5_EksikDolumlaFazlaHarcama(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	kural, _ := piyasa.Kurallar("AGTUSDT")
	s.mu.Lock()
	s.limitOran = 0.2
	s.mu.Unlock()
	ikinci := false
	var mu sync.Mutex
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		if r.URL.Path == "/api/v3/order" && r.Method == "GET" && q.Get("orderId") == "C1" && !ikinci {
			ikinci = true
			s.mu.Lock()
			s.doldur(s.emirler["C1"], 20, 1.001, true) // ikinci parça
			s.mu.Unlock()
			return false
		}
		if r.URL.Path == "/api/v3/myTrades" && q.Get("orderId") == "C1" {
			rec := icIsle(s, r)
			var ham []map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &ham)
			if len(ham) > 1 {
				ham = ham[:1] // borsanın işlem geçmişi henüz ikinci parçayı göstermiyor
			}
			json.NewEncoder(w).Encode(ham)
			return true
		}
		return false
	})
	d, err := limitSonraPiyasa(G.borsa, piyasa, kural, "BUY", 100, 0, "asTEST5", sessizOlay)
	_, _, harc, liste := emirleriSay(s)
	t.Logf("err=%v kumbaranın gördüğü: tutar=%.2f miktar=%.2f; borsada gerçekte harcanan=%.2f; emirler=%v", err, d.Tutar, d.Miktar, harc, liste)
	if harc > 100.5 {
		t.Errorf("0.7 HATASI GERİ GELDİ: 100 USDT bütçeyle borsada %.2f USDT harcandı; kumbara %.2f sanıyor", harc, d.Tutar)
	}
}

// 6) apiKeyInfo isteği ağ hatası verirse hata metninde erişim anahtarı (URL'deki accessKey=) yer alıyor
func TestInceleme6_AnahtarHataMetninde(t *testing.T) {
	_, a := arizaliKur(t)
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/apiKeyInfo" {
			baglantiKes(w)
			return true
		}
		return false
	})
	t.Setenv("ALSAT_ANAHTAR_KLASORU", t.TempDir())
	if err := anahtarKaydet(anahtarKaydi{Erisim: "mx0erisim", Gizli: "gizli-anahtar"}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	G.mu.Lock()
	G.kalici = false
	G.mu.Unlock()
	G.kayitliIleBaglan()
	oz := G.ozet(0)
	t.Logf("sayfaya giden hata: %v", oz["hata"])
	t.Logf("kayıt: %s", strings.TrimSpace(buf.String()))
	if strings.Contains(oz["hata"].(string), "mx0erisim") && strings.Contains(buf.String(), "mx0erisim") {
		t.Errorf("0.7 HATASI GERİ GELDİ: erişim anahtarı hem kayıt dosyasına hem sayfaya (G.hata) gidiyor")
	}
}

// 7) durum.json bozuksa program boş durumla açılıp ilk kayıtta üstüne yazıyordu (coin tutan kumbara unutuluyordu).
// 0.8: önceki kayıt (.onceki) yüklenir; o da yoksa dosya korunur, yeni kumbara açılmaz.
func TestInceleme7_BozukDurum(t *testing.T) {
	saatKur(t)
	t.Cleanup(func() { durumHatasi = "" })
	k := gercekKumbara(t, 100)
	acikIslemKoy(k, 1, 100)
	durumMu.Lock()
	degisti() // ilk kayıt
	durumMu.Unlock()
	yol := filepath.Join(kayitKlasoru(), "durum.json")
	b, _ := os.ReadFile(yol)
	_ = os.WriteFile(yol, b[:len(b)/2], 0644) // ör. elektrik kesintisi / SD kart: yarım dosya
	D = yeniDurum()                           // program yeniden açıldı
	durumYukle()
	durumMu.Lock()
	n := len(D.Kumbaralar)
	degisti() // sıradaki herhangi bir değişiklik
	durumMu.Unlock()
	b2, _ := os.ReadFile(yol)
	if n != 0 || durumHatasi == "" {
		t.Errorf("tek ve bozuk kayıtta program boş durumla açılmamalı ve uyarmalı (yüklenen %d, uyarı %q)", n, durumHatasi)
	}
	if !bytes.Equal(b2, b[:len(b)/2]) {
		t.Errorf("0.7 HATASI GERİ GELDİ: bozuk durum.json'ın üstüne yazıldı (elle kurtarılamaz)")
	}
	// iki kayıt varken son kayıt bozulursa bir önceki yüklenir
	durumHatasi = ""
	D = yeniDurum()
	k = gercekKumbara(t, 100)
	acikIslemKoy(k, 1, 100)
	durumMu.Lock()
	degisti()
	degisti()
	durumMu.Unlock()
	yol = filepath.Join(kayitKlasoru(), "durum.json")
	b, _ = os.ReadFile(yol)
	_ = os.WriteFile(yol, b[:len(b)/3], 0644)
	D = yeniDurum()
	durumYukle()
	if len(D.Kumbaralar) != 1 || D.Kumbaralar[0].acikIslem() == nil || durumHatasi != "" {
		t.Errorf("bozuk durum.json'dan sonra önceki kayıt yüklenmedi (kumbara %d, uyarı %q)", len(D.Kumbaralar), durumHatasi)
	}
}

// 8) Piyasa satışı 2 parça doldu, myTrades bir parçayı gösterdi → hayalet 'Adet'; sonraki satış kullanıcının kendi coinini satar
func TestInceleme8_HayaletAdet(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	k := gercekKumbara(t, 100)
	acikIslemKoy(k, 1, 100)
	s.mu.Lock()
	s.bakiye["AGT"] = [2]float64{100, 0}
	s.mu.Unlock()
	kirp := true
	var mu sync.Mutex
	var sonSatisMiktari string
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && q.Get("type") == "MARKET" && q.Get("side") == "SELL" {
			sonSatisMiktari = q.Get("quantity")
			rec := icIsle(s, r)
			if kirp {
				// borsa emri iki işlemde doldurdu (60 + 40)
				s.mu.Lock()
				for _, e := range s.emirler {
					if e.Tur == "MARKET" && e.Taraf == "SELL" && len(e.Dolumlar) == 1 {
						d0 := e.Dolumlar[0]
						d1 := map[string]any{}
						for kk, v := range d0 {
							d1[kk] = v
						}
						d0["qty"], d0["quoteQty"] = "60", "60"
						d1["qty"], d1["quoteQty"] = "40", "40"
						e.Dolumlar = append(e.Dolumlar, d1)
					}
				}
				s.mu.Unlock()
			}
			w.Write(rec.Body.Bytes())
			return true
		}
		if kirp && r.URL.Path == "/api/v3/myTrades" {
			rec := icIsle(s, r)
			var ham []map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &ham)
			if len(ham) > 1 {
				ham = ham[:1]
			}
			json.NewEncoder(w).Encode(ham)
			return true
		}
		return false
	})
	kumbaraSat(k, "anlikZarar", "test", 0.97, true)
	t.Logf("1. satış: borsaya giden miktar=%s, kumbarada kalan 'Adet'=%.2f, açık işlem=%v", sonSatisMiktari, k.Adet, k.acikIslem() != nil)
	mu.Lock()
	kirp = false
	mu.Unlock()
	// sıradaki işlem: 99 AGT alındı; hesapta 99 + kullanıcının kendi 50 AGT'si var
	acikIslemKoy(k, 2, 99)
	s.mu.Lock()
	s.bakiye["AGT"] = [2]float64{149, 0}
	s.mu.Unlock()
	kumbaraSat(k, "anlikZarar", "test", 0.97, true)
	t.Logf("2. satış: borsaya giden miktar=%s (kumbara bu işlemde 99 aldı)", sonSatisMiktari)
	if sonSatisMiktari == "139.00" {
		t.Errorf("0.7 HATASI GERİ GELDİ: kumbara 99 AGT aldığı işlemde 139 AGT sattı: kullanıcının 40 AGT'si satıldı")
	}
}
