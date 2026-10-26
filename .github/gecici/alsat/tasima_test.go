package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Eski sürümün klasörünü (kumbaralarıyla) hazırlar; bu sürüm boş başlar
func tasimaKur(t *testing.T) (eski, yeni string) {
	t.Helper()
	saatKur(t)
	kok := t.TempDir()
	eski = filepath.Join(kok, "KriptoAlSat_v0_7")
	yeni = filepath.Join(kok, "KriptoAlSat_v0_8")
	_ = os.MkdirAll(eski, 0755)
	_ = os.MkdirAll(yeni, 0755)
	eskiExe, eskiVeri, eskiYer := exeKlasoru, veriKlasoru, tasimaYerleri
	t.Cleanup(func() {
		exeKlasoru, veriKlasoru, tasimaYerleri = eskiExe, eskiVeri, eskiYer
		tasimaReddi = false
		korumaGenel.Store(korumaGenelAyar{Tavan: 10, Hassasiyet: "dengeli"})
	})
	tasimaYerleri = func() []string { return []string{kok} }
	// eski sürüm: iki kumbara (biri gerçek parayla coinde, biri bitmiş), işlem dosyası, arşiv, ayarlar
	veriKlasoru = eski
	D = yeniDurum()
	durumMu.Lock()
	k1 := kumbaraOlustur(acmaIstegi{Dosya: "AGTUSDT_1m.json", Icerik: []byte(`{}`), Sembol: "AGTUSDT", Aralik: "1m", Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000, Mod: "gercek", Tutar: 50})
	k2 := kumbaraOlustur(acmaIstegi{Dosya: "BTCUSDT_5m.json", Icerik: []byte(`{}`), Sembol: "BTCUSDT", Aralik: "5m", Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000, Mod: "sanal", Tutar: 20})
	k2.Durum = "bitti"
	durumMu.Unlock()
	acikIslemKoy(k1, 1, 49)
	durumMu.Lock()
	degisti()
	durumMu.Unlock()
	_ = os.WriteFile(filepath.Join(eski, "kayitlar", "islemler.csv"), []byte("eski işlemler\n"), 0644)
	_ = os.MkdirAll(filepath.Join(eski, "kayitlar", "arsiv"), 0755)
	_ = os.WriteFile(filepath.Join(eski, "kayitlar", "arsiv", "XRPUSDT_1m.json"), []byte("{}"), 0644)
	_ = os.WriteFile(filepath.Join(eski, "ayarlar.json"), []byte(`{"koruma":{"tavan":7,"hassasiyet":"hassas"},"gorunum":{"x":1}}`), 0644)
	eskiZaman := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(filepath.Join(eski, "kayitlar", "durum.json"), eskiZaman, eskiZaman)
	// bu sürüm
	exeKlasoru, veriKlasoru = yeni, yeni
	D = yeniDurum()
	aktifOturum.Store("tasima-oturum")
	t.Cleanup(func() { aktifOturum.Store("") })
	return eski, yeni
}

func tasimaGet(t *testing.T) *eskiKayit {
	rec := httptest.NewRecorder()
	tasimaIstegi(rec, httptest.NewRequest("GET", "/api/tasima", nil))
	var j struct {
		Teklif *eskiKayit `json:"teklif"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &j)
	return j.Teklif
}

func tasimaPost(govde string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	tasimaIstegi(rec, httptest.NewRequest("POST", "/api/tasima", bytes.NewReader([]byte(govde))))
	return rec
}

func TestTasima_EskiKumbaralarTasinir(t *testing.T) {
	eski, yeni := tasimaKur(t)
	tk := tasimaGet(t)
	if tk == nil || tk.Ad != "KriptoAlSat_v0_7" || tk.Toplam != 2 || tk.Calisan != 1 || tk.Gercek != 1 || tk.Coinde != 1 {
		t.Fatalf("eski kayıt teklifi yanlış: %+v", tk)
	}
	if rec := tasimaPost(`{"oturum":"tasima-oturum","klasor":"/etc"}`); rec.Code != 404 {
		t.Errorf("bulunmayan klasör taşınmamalı: %d", rec.Code)
	}
	if rec := tasimaPost(`{"oturum":"baska","klasor":"` + tk.Klasor + `"}`); rec.Code != 400 {
		t.Errorf("eski pencere taşıma yapamamalı: %d", rec.Code)
	}
	rec := tasimaPost(`{"oturum":"tasima-oturum","klasor":"` + tk.Klasor + `"}`)
	if rec.Code != 200 {
		t.Fatalf("taşınamadı: %d %s", rec.Code, rec.Body.String())
	}
	durumMu.Lock()
	n := len(D.Kumbaralar)
	var gercek *kumbara
	for _, k := range D.Kumbaralar {
		if k.Mod == "gercek" {
			gercek = k
		}
	}
	durumMu.Unlock()
	if n != 2 || gercek == nil || gercek.acikIslem() == nil || gercek.Adet != 49 || gercek.Koruma == nil {
		t.Fatalf("kumbaralar bu sürümde kaldığı yerden devam etmeli: %d kumbara, %+v", n, gercek)
	}
	for _, y := range []string{"kayitlar/durum.json", "kayitlar/islemler.csv", "kayitlar/arsiv/XRPUSDT_1m.json"} {
		if _, err := os.Stat(filepath.Join(yeni, y)); err != nil {
			t.Errorf("%s kopyalanmadı", y)
		}
	}
	if _, err := os.Stat(filepath.Join(eski, "kayitlar", "durum.json")); err == nil {
		t.Errorf("eski durum.json işaretlenmedi: eski sürüm açılırsa aynı kumbaraları ikinci kez çalıştırır")
	}
	if b, err := os.ReadFile(filepath.Join(eski, "kayitlar", "TASINDI.txt")); err != nil || !strings.Contains(string(b), "taşındı") {
		t.Errorf("eski klasöre not bırakılmadı")
	}
	if g := korumaGenelAl(); g.Tavan != 7 || g.Hassasiyet != "hassas" {
		t.Errorf("eski koruma ayarları gelmedi: %+v", g)
	}
	if tasimaGet(t) != nil {
		t.Errorf("taşımadan sonra yeniden sorulmamalı")
	}
	// eski sürüm şimdi açılsa boş açılır
	veriKlasoruYedek := veriKlasoru
	veriKlasoru = eski
	Dyedek := D
	D = yeniDurum()
	durumYukle()
	if len(D.Kumbaralar) != 0 {
		t.Errorf("eski sürüm taşımadan sonra kumbarasız açılmalı (%d)", len(D.Kumbaralar))
	}
	veriKlasoru, D = veriKlasoruYedek, Dyedek
}

func TestTasima_EskiProgramAcikkenTasinmaz(t *testing.T) {
	eski, _ := tasimaKur(t)
	simdi := time.Now()
	_ = os.Chtimes(filepath.Join(eski, "kayitlar", "durum.json"), simdi, simdi)
	saat := saatKur(t)
	saat.mu.Lock()
	saat.t = simdi.Add(10 * time.Second)
	saat.mu.Unlock()
	tk := tasimaGet(t)
	if tk == nil {
		t.Fatal("teklif yok")
	}
	rec := tasimaPost(`{"oturum":"tasima-oturum","klasor":"` + tk.Klasor + `"}`)
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "hâlâ açık") {
		t.Fatalf("az önce yazılmış kayıt taşınmamalı: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(eski, "kayitlar", "durum.json")); err != nil {
		t.Errorf("eski kayıt yerinde kalmalı")
	}
}

func TestTasima_KumbaraVarsaSorulmaz(t *testing.T) {
	tasimaKur(t)
	durumMu.Lock()
	kumbaraOlustur(acmaIstegi{Dosya: "ETHUSDT_1m.json", Icerik: []byte(`{}`), Sembol: "ETHUSDT", Aralik: "1m", Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000, Mod: "sanal", Tutar: 10})
	durumMu.Unlock()
	if tasimaGet(t) != nil {
		t.Errorf("bu sürümde kumbara varken taşıma sorulmamalı")
	}
}

func TestTasima_SimdiDegil(t *testing.T) {
	tasimaKur(t)
	if tasimaGet(t) == nil {
		t.Fatal("teklif yok")
	}
	if rec := tasimaPost(`{"oturum":"tasima-oturum","vazgec":true}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if tasimaGet(t) != nil {
		t.Errorf("'şimdi değil' denince bu açılışta yeniden sorulmamalı")
	}
}
