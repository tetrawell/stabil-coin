package main

// 0.8 testleri: belirsiz emirden sonra kurtarma, kısmi satışın tamamlanması, gereksiz disk yazımı, uzaktan strateji alma.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Ağ koptuğu için sonucu öğrenilemeyen limit alış: kumbara Bekleyen'i tutar; bağlantı gelince kurtar emri iptal eder,
// hesapta ve kumbarada fazladan bir şey kalmaz.
func TestSurum08_BelirsizAlisSonraKurtarilir(t *testing.T) {
	s, a := arizaliKur(t)
	saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	k := gercekKumbara(t, 100)
	var mu sync.Mutex
	kopuk := false
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && r.URL.Query().Get("type") == "LIMIT" {
			icIsle(s, r)
			kopuk = true
			baglantiKes(w)
			return true
		}
		if kopuk && strings.HasPrefix(r.URL.Path, "/api/v3/") && r.URL.Path != "/api/v3/time" {
			baglantiKes(w)
			return true
		}
		return false
	})
	kumbaraAl(k, 1.004, 1)
	if k.Bekleyen == nil || k.acikIslem() != nil || k.Emirde {
		t.Fatalf("belirsiz alışta Bekleyen tutulmalı, işlem açılmamalı (bekleyen=%v açık=%v)", k.Bekleyen, k.acikIslem() != nil)
	}
	la, pd, _, _ := emirleriSay(s)
	if la != 1 || pd != 0 {
		t.Fatalf("borsada 1 açık limit olmalı, piyasa emri olmamalı (limit %d, piyasa %d)", la, pd)
	}
	mu.Lock()
	kopuk = false // internet geri geldi
	mu.Unlock()
	kurtar(k)
	la, pd, _, liste := emirleriSay(s)
	if k.Bekleyen != nil || la != 0 || pd != 0 || k.Nakit != 100 || k.acikIslem() != nil {
		t.Errorf("kurtar sonrası her şey temiz olmalı: bekleyen=%v açık limit=%d piyasa=%d nakit=%.2f emirler=%v", k.Bekleyen, la, pd, k.Nakit, liste)
	}
}

// Satışın bir kısmı doldu, kalanı için piyasa emri bir kez reddedildi: işlem açık kalır, tekrar denemede kalan
// satılır, işlem tek kayıtta (bütün gelirle) kapanır.
func TestSurum08_KismiSatisTamamlanir(t *testing.T) {
	s, a := arizaliKur(t)
	saat := saatKur(t)
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatal(r.Hata)
	}
	k := gercekKumbara(t, 100)
	acikIslemKoy(k, 1, 100)
	durumMu.Lock()
	k.Nakit = 0
	durumMu.Unlock()
	s.mu.Lock()
	s.bakiye["AGT"] = [2]float64{100, 0}
	s.limitOran = 0.1
	s.mu.Unlock()
	var mu sync.Mutex
	red := true
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		mu.Lock()
		defer mu.Unlock()
		if red && r.URL.Path == "/api/v3/order" && r.Method == "POST" && r.URL.Query().Get("type") == "MARKET" {
			red = false
			w.WriteHeader(429)
			w.Write([]byte(`{"code":429,"msg":"Too Many Requests"}`))
			return true
		}
		return false
	})
	kumbaraSat(k, "motor", "test", 1.0, false)
	if k.acikIslem() == nil || k.Adet < 89 || k.SatisTekrar == 0 {
		t.Fatalf("kısmi satıştan sonra işlem açık ve tekrar planlı olmalı (açık=%v adet=%.2f tekrar=%d)", k.acikIslem() != nil, k.Adet, k.SatisTekrar)
	}
	ilkGelir := k.Nakit
	s.mu.Lock()
	s.limitOran = 0
	s.mu.Unlock()
	eylemKoru(t)
	eylemiYap = func(k *kumbara, e eylem) {
		if e.tur == "sat" {
			kumbaraSat(k, e.sebep, e.aciklama, e.beklenen, e.acil)
		}
	}
	saat.ileri(4 * time.Second)
	izleyiciTur()
	t1 := lastIslem(k)
	if k.acikIslem() != nil || k.Adet > 0.01 || t1 == nil || t1.Satis == nil {
		t.Fatalf("tekrar denemede kalan satılmalıydı (açık=%v adet=%.2f)", k.acikIslem() != nil, k.Adet)
	}
	if len(t1.KismiSatis) != 1 || t1.Gelen < ilkGelir+80 || t1.Satis.Miktar < 99.9 {
		t.Errorf("işlem kaydı bütün satışı içermeli: kısmi=%d gelen=%.2f satılan=%.2f", len(t1.KismiSatis), t1.Gelen, t1.Satis.Miktar)
	}
	if k.Nakit < 99 {
		t.Errorf("kumbara bütün satış gelirini almalı (nakit %.2f)", k.Nakit)
	}
}

// Ekranda gösterilen kumbara 5 sn'de bir aynı kararı bildirir: değişen bir şey yoksa durum.json yeniden yazılmaz.
func TestSurum08_DegismeyenKararDiskeYazilmaz(t *testing.T) {
	saatKur(t)
	k := gercekKumbara(t, 100)
	durumMu.Lock()
	degisti()
	durumMu.Unlock()
	yol := filepath.Join(kayitKlasoru(), "durum.json")
	eylemKoru(t)
	eylemYakala(t)
	oturum := "test-oturum"
	aktifOturum.Store(oturum)
	t.Cleanup(func() { aktifOturum.Store("") })
	gonder := func(mum int64) {
		b, _ := json.Marshal(kararIstegi{Oturum: oturum, ID: k.ID, Mum: mum, Asama: "bekle"})
		rec := httptest.NewRecorder()
		kumbaraKararIstegi(rec, httptest.NewRequest("POST", "/api/kumbara/karar", bytes.NewReader(b)))
		if rec.Code != 200 {
			t.Fatalf("karar isteği %d: %s", rec.Code, rec.Body.String())
		}
	}
	mum := simdiF().UnixMilli()/60000*60000 - 60000
	gonder(mum)
	_ = os.WriteFile(yol, []byte(`{"isaret":1}`), 0644) // yazılırsa bu işaret kaybolur
	gonder(mum)
	gonder(mum)
	b, _ := os.ReadFile(yol)
	if string(b) != `{"isaret":1}` {
		t.Errorf("aynı karar tekrar bildirilince durum.json yeniden yazıldı")
	}
	gonder(mum + 60000) // yeni mum: yazılmalı
	b, _ = os.ReadFile(yol)
	if string(b) == `{"isaret":1}` {
		t.Errorf("yeni mumun kararı diske yazılmadı")
	}
}

// eylemiYap'ı test bitince eski hâline getirir
func eylemKoru(t *testing.T) {
	eski := eylemiYap
	t.Cleanup(func() { eylemiYap = eski })
}

// ---------- uzaktan strateji alma ----------

func uzakKur(t *testing.T) (klasor string, kod string) {
	t.Helper()
	saatKur(t)
	veriKlasoru = t.TempDir()
	klasor = filepath.Join(t.TempDir(), "stratejiler")
	_ = os.MkdirAll(klasor, 0755)
	t.Setenv("ALSAT_KLASOR", klasor)
	t.Setenv("ALSAT_ANAHTAR_KLASORU", t.TempDir())
	D = yeniDurum()
	uzak.mu.Lock()
	uzak.kayit = uzakKaydi{Acik: true, Kod: "ABCD-EFGH-JKLM"}
	uzak.gorulen = map[string]int64{}
	uzak.hatalar = map[string][]int64{}
	uzak.sonGelen = nil
	uzak.yuklendi = true
	uzak.mu.Unlock()
	t.Cleanup(func() {
		uzak.mu.Lock()
		uzak.kayit = uzakKaydi{}
		uzak.yuklendi = false
		uzak.mu.Unlock()
	})
	return klasor, "ABCD-EFGH-JKLM"
}

func uzakOrnek(sembol, aralik string, olus time.Time) []byte {
	return []byte(`{"tur":"kripto-simulator-strateji","surum":3,"olusturuldu":"` + olus.UTC().Format(time.RFC3339Nano) + `","borsa":"mexc","sembol":"` + sembol + `","aralik":"` + aralik + `","strateji":{"surum":3,"kurallar":{}},"gecmisSonuc":{"mumSayisi":5000,"islemSayisi":7}}`)
}

func uzakIstek(t *testing.T, kod, yol, dosya string, govde []byte, zaman int64, ip string) *httptest.ResponseRecorder {
	t.Helper()
	z := strconv.FormatInt(zaman, 10)
	r := httptest.NewRequest("POST", yol, bytes.NewReader(govde))
	r.RemoteAddr = ip + ":51000"
	r.Header.Set("X-KAS-Zaman", z)
	r.Header.Set("X-KAS-Dosya", dosya)
	r.Header.Set("X-KAS-Imza", uzakImza(kod, yol, z, dosya, govde))
	rec := httptest.NewRecorder()
	uzakSunucu().ServeHTTP(rec, r)
	return rec
}

// İki programın imzası aynı olmalı: simülatörün testi (kaynak/alsat_test.go) aynı örnekte aynı sonucu bekler.
// (Değer Python'un hmac modülüyle ayrıca hesaplandı.)
func TestSurum08_UzakImzaOrnegi(t *testing.T) {
	got := uzakImza("abcd-efgh-jklm", "/kripto-alsat/strateji", "1792900000000", "BTCUSDT_1m.json", []byte(`{"a":1}`))
	const beklenen = "a3aa5309773f96bd91b2ff25a8588b250fea394f1d7e2db8c9253b097815defb"
	if got != beklenen {
		t.Fatalf("imza değişti: %s (simülatörle uyuşmaz)", got)
	}
	if uzakImza("ABCD EFGH JKLM", "/kripto-alsat/strateji", "1792900000000", "BTCUSDT_1m.json", []byte(`{"a":1}`)) != beklenen {
		t.Fatal("kod yazımı (küçük harf, boşluk, tire) imzayı değiştirmemeli")
	}
}

func TestSurum08_UzakStratejiAlma(t *testing.T) {
	klasor, kod := uzakKur(t)
	simdi := time.Now()
	govde := uzakOrnek("BTCUSDT", "1m", simdiF())
	// doğru kodla: dosya yazılır
	rec := uzakIstek(t, kod, "/kripto-alsat/strateji", "BTCUSDT_1m.json", govde, simdi.UnixMilli(), "192.168.1.20")
	if rec.Code != 200 {
		t.Fatalf("doğru imzalı istek reddedildi: %d %s", rec.Code, rec.Body.String())
	}
	if b, err := os.ReadFile(filepath.Join(klasor, "BTCUSDT_1m.json")); err != nil || !bytes.Contains(b, []byte(`"BTCUSDT"`)) {
		t.Fatalf("dosya strateji klasörüne yazılmadı: %v", err)
	}
	// aynı istek ikinci kez: tekrar oynatma reddedilir
	z := strconv.FormatInt(simdi.UnixMilli(), 10)
	r := httptest.NewRequest("POST", "/kripto-alsat/strateji", bytes.NewReader(govde))
	r.RemoteAddr = "192.168.1.20:51000"
	r.Header.Set("X-KAS-Zaman", z)
	r.Header.Set("X-KAS-Dosya", "BTCUSDT_1m.json")
	r.Header.Set("X-KAS-Imza", uzakImza(kod, "/kripto-alsat/strateji", z, "BTCUSDT_1m.json", govde))
	rec2 := httptest.NewRecorder()
	uzakSunucu().ServeHTTP(rec2, r)
	if rec2.Code != 409 {
		t.Errorf("aynı imzalı istek ikinci kez kabul edildi: %d", rec2.Code)
	}
	// yanlış kod
	if rec := uzakIstek(t, "WXYZ-WXYZ-WXYZ", "/kripto-alsat/strateji", "ETHUSDT_1m.json", uzakOrnek("ETHUSDT", "1m", simdiF()), time.Now().UnixMilli(), "192.168.1.21"); rec.Code != 401 {
		t.Errorf("yanlış kodla istek kabul edildi: %d", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(klasor, "ETHUSDT_1m.json")); err == nil {
		t.Errorf("yanlış kodla dosya yazıldı")
	}
	// gövde imzadan sonra değiştirilirse (araya girme) reddedilir
	z = strconv.FormatInt(time.Now().UnixMilli()+1, 10)
	g1 := uzakOrnek("SOLUSDT", "1m", simdiF())
	r = httptest.NewRequest("POST", "/kripto-alsat/strateji", bytes.NewReader(bytes.Replace(g1, []byte(`"islemSayisi":7`), []byte(`"islemSayisi":8`), 1)))
	r.RemoteAddr = "192.168.1.22:51000"
	r.Header.Set("X-KAS-Zaman", z)
	r.Header.Set("X-KAS-Dosya", "SOLUSDT_1m.json")
	r.Header.Set("X-KAS-Imza", uzakImza(kod, "/kripto-alsat/strateji", z, "SOLUSDT_1m.json", g1))
	rec3 := httptest.NewRecorder()
	uzakSunucu().ServeHTTP(rec3, r)
	if rec3.Code != 401 {
		t.Errorf("değiştirilmiş gövde kabul edildi: %d", rec3.Code)
	}
	// eski zaman damgası
	if rec := uzakIstek(t, kod, "/kripto-alsat/strateji", "XRPUSDT_1m.json", uzakOrnek("XRPUSDT", "1m", simdiF()), time.Now().Add(-10*time.Minute).UnixMilli(), "192.168.1.23"); rec.Code != 401 {
		t.Errorf("10 dk eski istek kabul edildi: %d", rec.Code)
	}
	// dosya adı ile içerik uyuşmazsa ya da yol kaçırma denenirse
	for _, ad := range []string{"../evil.json", "BTCUSDT_1m.json.exe", "ETHUSDT_1m.json"} {
		if rec := uzakIstek(t, kod, "/kripto-alsat/strateji", ad, uzakOrnek("DOGEUSDT", "1m", simdiF()), time.Now().UnixMilli()+int64(len(ad)), "192.168.1.24"); rec.Code == 200 {
			t.Errorf("geçersiz dosya adı kabul edildi: %s", ad)
		}
	}
	// Binance verisiyle bulunmuş dosya
	bin := bytes.Replace(uzakOrnek("ADAUSDT", "1m", simdiF()), []byte(`"mexc"`), []byte(`"binance"`), 1)
	if rec := uzakIstek(t, kod, "/kripto-alsat/strateji", "ADAUSDT_1m.json", bin, time.Now().UnixMilli()+7, "192.168.1.25"); rec.Code == 200 {
		t.Errorf("Binance dosyası kabul edildi")
	}
	// kapalıyken
	uzak.mu.Lock()
	uzak.kayit.Acik = false
	uzak.mu.Unlock()
	if rec := uzakIstek(t, kod, "/kripto-alsat/dene", "", []byte("{}"), time.Now().UnixMilli()+9, "192.168.1.26"); rec.Code != 403 {
		t.Errorf("kapalıyken istek kabul edildi: %d", rec.Code)
	}
}

func TestSurum08_UzakHataliDenemeEngeli(t *testing.T) {
	_, kod := uzakKur(t)
	for i := 0; i < 12; i++ {
		uzakIstek(t, "QQQQ-QQQQ-QQQQ", "/kripto-alsat/dene", "", []byte("{}"), time.Now().UnixMilli()+int64(i), "10.0.0.9")
	}
	if rec := uzakIstek(t, kod, "/kripto-alsat/dene", "", []byte("{}"), time.Now().UnixMilli()+100, "10.0.0.9"); rec.Code != 429 {
		t.Errorf("10 hatalı denemeden sonra adres engellenmedi: %d", rec.Code)
	}
	if rec := uzakIstek(t, kod, "/kripto-alsat/dene", "", []byte("{}"), time.Now().UnixMilli()+101, "10.0.0.10"); rec.Code != 200 {
		t.Errorf("başka adres etkilenmemeli: %d %s", rec.Code, rec.Body.String())
	}
}

// Tarayıcıdan (bir web sitesinden) gelen istek değerlendirilmez ve adresi engelletmez
func TestSurum08_UzakTarayicidanGelmez(t *testing.T) {
	_, kod := uzakKur(t)
	for i := 0; i < 15; i++ {
		r := httptest.NewRequest("POST", "/kripto-alsat/strateji", bytes.NewReader([]byte("{}")))
		r.RemoteAddr = "10.0.0.20:5000"
		r.Header.Set("Origin", "http://kotu.example")
		rec := httptest.NewRecorder()
		uzakSunucu().ServeHTTP(rec, r)
		if rec.Code != 403 {
			t.Fatalf("tarayıcıdan gelen istek: %d", rec.Code)
		}
	}
	if rec := uzakIstek(t, kod, "/kripto-alsat/dene", "", []byte("{}"), time.Now().UnixMilli(), "10.0.0.20"); rec.Code != 200 {
		t.Errorf("tarayıcı istekleri bilgisayarın adresini engelletmemeli: %d %s", rec.Code, rec.Body.String())
	}
}

// Çalışan kumbaranın dosyası gelirse cevap bunu söyler (kumbara, yeniden dışa aktarmadaki gibi yeni ayarla devam eder)
func TestSurum08_UzakCalisanKumbara(t *testing.T) {
	klasor, kod := uzakKur(t)
	_ = os.WriteFile(filepath.Join(klasor, "BTCUSDT_1m.json"), uzakOrnek("BTCUSDT", "1m", simdiF().Add(-time.Minute)), 0644)
	durumMu.Lock()
	kumbaraOlustur(acmaIstegi{Dosya: "BTCUSDT_1m.json", Icerik: []byte(`{}`), Sembol: "BTCUSDT", Aralik: "1m", Olusturuldu: simdiF().Add(-time.Minute).UnixMilli(), Bitis: simdiF().Add(time.Hour).UnixMilli(), Mod: "sanal", Tutar: 50})
	durumMu.Unlock()
	rec := uzakIstek(t, kod, "/kripto-alsat/strateji", "BTCUSDT_1m.json", uzakOrnek("BTCUSDT", "1m", simdiF()), time.Now().UnixMilli(), "192.168.1.30")
	var j map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &j)
	if rec.Code != 200 || j["durum"] != "calisanKumbara" {
		t.Errorf("çalışan kumbaranın dosyası: %d %v", rec.Code, j)
	}
}

// Bir coinin fiyat isteği takılırsa diğer kumbaraların fiyatı (ve zarar durdur denetimi) beklemez.
func TestSurum08_TakilanFiyatDigerleriniBeklemez(t *testing.T) {
	birak := make(chan struct{})
	sv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("symbol") == "TAKILUSDT" {
			select {
			case <-birak:
			case <-time.After(3 * time.Second):
			}
			return
		}
		w.Write([]byte(`{"symbol":"` + r.URL.Query().Get("symbol") + `","price":"1.234"}`))
	}))
	defer sv.Close()
	defer close(birak)
	eskiA, eskiP, eskiZ := mexcAdresi, piyasa, fiyatIstemci.Timeout
	mexcAdresi, piyasa, fiyatIstemci.Timeout = sv.URL, yeniMexcPiyasa(), 400*time.Millisecond
	t.Cleanup(func() { mexcAdresi, piyasa, fiyatIstemci.Timeout = eskiA, eskiP, eskiZ })
	bas := time.Now()
	f := izleyiciFiyatlari(map[string]bool{"TAKILUSDT": true, "AGTUSDT": true, "BTCUSDT": true})
	if sure := time.Since(bas); sure > 1500*time.Millisecond {
		t.Errorf("takılan fiyat isteği izleyiciyi %v bekletti", sure)
	}
	if f["AGTUSDT"] != 1.234 || f["BTCUSDT"] != 1.234 {
		t.Errorf("diğer coinlerin fiyatı gelmeliydi: %v", f)
	}
	if _, vardir := f["TAKILUSDT"]; vardir {
		t.Errorf("takılan coinin fiyatı olmamalı")
	}
}

func gBagli() (bool, bool, string) {
	G.mu.Lock()
	defer G.mu.Unlock()
	return G.Bagli, G.kalici, G.hata
}

// MEXC cevabında "remainingValidity" hiç yoksa anahtar "süresi dolmuş" sayılmaz (0.7'de sayılıyordu).
func TestSurum08_KalanGunYoksaBaglanir(t *testing.T) {
	_, a := arizaliKur(t)
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/apiKeyInfo" {
			w.Write([]byte(`{"permissions":"SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE","status":"VALID"}`))
			return true
		}
		return false
	})
	if r := G.dene("mx0erisim", "gizli-anahtar"); !r.Tamam {
		t.Fatalf("kalan gün bilgisi olmayan anahtar reddedildi: %+v", r)
	}
	// süresi gerçekten dolmuş anahtar yine reddedilir
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/apiKeyInfo" {
			w.Write([]byte(`{"permissions":"SPOT_ACCOUNT_READ,SPOT_DEAL_READ,SPOT_DEAL_WRITE","status":"VALID","remainingValidity":0}`))
			return true
		}
		return false
	})
	if r := G.dene("mx0erisim", "gizli-anahtar"); r.Tamam || !r.Kesin {
		t.Fatalf("süresi dolmuş anahtar kesin reddedilmeli: %+v", r)
	}
}

// 6 saatlik izin denetimi: MEXC'nin geçici sorununda (ör. ağ geçidinin 403 sayfası) bağlantı kesilmez, 30 dk sonra
// yeniden denenir; para çekme izni eklenmişse kesilir.
func TestSurum08_AltiSaatlikDenetim(t *testing.T) {
	s, a := arizaliKur(t)
	saat := saatKur(t)
	t.Setenv("ALSAT_ANAHTAR_KLASORU", t.TempDir())
	if err := anahtarKaydet(anahtarKaydi{Erisim: "mx0erisim", Gizli: "gizli-anahtar"}); err != nil {
		t.Fatal(err)
	}
	G.kayitliIleBaglan()
	if b, _, h := gBagli(); !b {
		t.Fatal("bağlanmalıydı: " + h)
	}
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/apiKeyInfo" {
			w.WriteHeader(403)
			w.Write([]byte("<html>Access denied</html>"))
			return true
		}
		return false
	})
	saat.ileri(6*time.Hour + time.Minute)
	gercekTur()
	if b, _, h := gBagli(); !b {
		t.Fatalf("geçici hatada bağlantı kesildi: %s", h)
	}
	a.kanca(nil)
	s.mu.Lock()
	s.izinler += ",SPOT_WITHDRAW_WRITE"
	s.mu.Unlock()
	saat.ileri(10 * time.Minute)
	gercekTur()
	if b, _, _ := gBagli(); !b {
		t.Fatal("30 dk dolmadan yeniden denetim yapılmamalıydı")
	}
	saat.ileri(25 * time.Minute)
	gercekTur()
	if b, k, h := gBagli(); b || !k || !strings.Contains(h, "para çekme") {
		t.Fatalf("para çekme izni eklenince bağlantı kalıcı kesilmeli (bağlı=%v kalıcı=%v %s)", b, k, h)
	}
}

// Açılışta MEXC geçici hata verirse program yeniden denemeyi bırakmaz (0.7: geçici olmayan her hatada bırakıyordu)
func TestSurum08_AcilistaGeciciHataYenidenDener(t *testing.T) {
	_, a := arizaliKur(t)
	saatKur(t)
	t.Setenv("ALSAT_ANAHTAR_KLASORU", t.TempDir())
	if err := anahtarKaydet(anahtarKaydi{Erisim: "mx0erisim", Gizli: "gizli-anahtar"}); err != nil {
		t.Fatal(err)
	}
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v3/apiKeyInfo" {
			w.WriteHeader(400)
			w.Write([]byte(`{"code":700003,"msg":"Timestamp for this request is outside of the recvWindow."}`))
			return true
		}
		return false
	})
	G.kayitliIleBaglan()
	if b, k, h := gBagli(); b || k {
		t.Fatalf("saat hatasında bağlantı kurulmamalı ama kalıcı da sayılmamalı (bağlı=%v kalıcı=%v %s)", b, k, h)
	}
	a.kanca(nil)
	gercekTur()
	if b, _, h := gBagli(); !b {
		t.Fatalf("MEXC düzelince yeniden bağlanmalıydı: %s", h)
	}
}

// Dosyada sayfada çalışabilecek bir yazı (ya da beklenmeyen tür) varsa dosya çalıştırılmaz; simülatörün gerçek
// dosyası (bütün kurallar açık) geçer.
func TestSurum08_DosyaDegerDenetimi(t *testing.T) {
	saatKur(t)
	tam, err := os.ReadFile("test/tam_strateji.json")
	if err != nil {
		t.Fatal(err)
	}
	klasor := t.TempDir()
	yaz := func(strateji, gecmis string) stratejiDosyasi {
		b := `{"tur":"kripto-simulator-strateji","surum":3,"olusturuldu":"` + simdiF().UTC().Format(time.RFC3339Nano) + `","borsa":"mexc","sembol":"BTCUSDT","aralik":"1m","strateji":` + strateji + `,"gecmisSonuc":` + gecmis + `,"aciklama":"Bu dosyayı <b>Kripto</b> Simülatör motoru okur."}`
		yol := filepath.Join(klasor, "BTCUSDT_1m.json")
		_ = os.WriteFile(yol, []byte(b), 0644)
		return dosyaOku(yol, varsayilanOmur, simdiF())
	}
	gs := `{"mumSayisi":5000,"ilkMum":"2026-10-14T09:20:00.000Z","sonMum":"2026-10-17T23:39:00.000Z","botYuzde":4.3,"alTutYuzde":-2.1,"islemSayisi":24,"karliIslem":15}`
	if d := yaz(string(tam), gs); d.Durum != "gecerli" {
		t.Fatalf("simülatörün gerçek dosyası reddedildi: %s", d.Hata)
	}
	kotuler := map[string][2]string{
		"kural değerinde html": {strings.Replace(string(tam), `"seviye":-0.382`, `"seviye":"<img src=x onerror=alert(1)>"`, 1), gs},
		"anahtarda html":       {strings.Replace(string(tam), `"fbbAlt"`, `"<svg onload=alert(1)>"`, 1), gs},
		"işlem sayısında html": {string(tam), strings.Replace(gs, `"islemSayisi":24`, `"islemSayisi":"<script>alert(1)</script>"`, 1)},
		"tarih yerine html":    {string(tam), strings.Replace(gs, `"2026-10-14T09:20:00.000Z"`, `"<b>x</b>"`, 1)},
		"çok büyük sayı":       {strings.Replace(string(tam), `"seviye":-0.382`, `"seviye":1e999`, 1), gs},
	}
	for ad, x := range kotuler {
		if d := yaz(x[0], x[1]); d.Durum == "gecerli" || d.Hata == "" {
			t.Errorf("%s: dosya kabul edildi", ad)
		}
	}
}

// Ekran sunucusunun güvenlik katmanı: başka site çerçeveye alamaz, başka siteden okuma ve yazma isteği gelemez.
func TestSurum08_GuvenlikKatmani(t *testing.T) {
	h := guvenlik(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
	iste := func(yontem, yol, host string, b map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(yontem, yol, nil)
		r.Host = host
		for k, v := range b {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	yer := "127.0.0.1:47322"
	durumlar := []struct {
		ad, yontem, yol, host string
		b                     map[string]string
		kod                   int
	}{
		{"sayfa", "GET", "/", yer, map[string]string{"Sec-Fetch-Site": "none"}, 200},
		{"kendi isteği", "GET", "/api/durum", yer, map[string]string{"Sec-Fetch-Site": "same-origin"}, 200},
		{"tarayıcı dışı (ör. ikinci program)", "GET", "/api/ping", yer, nil, 200},
		{"başka siteden okuma", "GET", "/api/baglanti", yer, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"aynı bilgisayardaki başka kapıdan okuma", "GET", "/api/durum", yer, map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
		{"başka siteden yazma", "POST", "/api/kumbara/durdur", yer, map[string]string{"Origin": "http://kotu.example"}, 403},
		{"başka siteden yazma (Origin'siz)", "POST", "/api/kumbara/durdur", yer, map[string]string{"Sec-Fetch-Site": "cross-site"}, 403},
		{"kendi yazması", "POST", "/api/kumbara/durdur", yer, map[string]string{"Origin": "http://" + yer, "Sec-Fetch-Site": "same-origin"}, 200},
		{"DNS hilesi", "GET", "/api/durum", "kotu.example:47322", nil, 403},
	}
	for _, d := range durumlar {
		w := iste(d.yontem, d.yol, d.host, d.b)
		if w.Code != d.kod {
			t.Errorf("%s: %d bekleniyordu, %d geldi", d.ad, d.kod, w.Code)
		}
		if d.host == yer && (w.Header().Get("X-Frame-Options") != "DENY" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")) {
			t.Errorf("%s: çerçeve koruması başlıkları yok", d.ad)
		}
	}
}
