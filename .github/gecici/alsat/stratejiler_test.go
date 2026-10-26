package main

// go test ./... — strateji dosyalarının okunması, ömür hesabı, motor karşılaştırması ve klasör bulma.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stratejiYaz(t *testing.T, klasor, ad string, ic map[string]any) string {
	t.Helper()
	b, _ := json.Marshal(ic)
	yol := filepath.Join(klasor, ad)
	if err := os.WriteFile(yol, b, 0644); err != nil {
		t.Fatal(err)
	}
	return yol
}

func ornekStrateji(sembol, aralik string, olusturuldu time.Time) map[string]any {
	return map[string]any{
		"tur": "kripto-simulator-strateji", "surum": 3, "olusturuldu": olusturuldu.UTC().Format("2006-01-02T15:04:05.000Z"),
		"borsa": "mexc", "sembol": sembol, "aralik": aralik,
		"strateji":    map[string]any{"surum": 3, "kurallar": map[string]any{}, "para": map[string]any{"baslangic": 1000, "masraf": 0.1}},
		"gecmisSonuc": map[string]any{"mumSayisi": 5000},
	}
}

func TestStratejiDosyalari(t *testing.T) {
	k := t.TempDir()
	simdi := time.Date(2026, 10, 17, 20, 0, 0, 0, time.UTC)
	omur := omurTablosu(map[string]json.RawMessage{})
	gun := 24 * time.Hour
	if omur["1m"] != 1440 || omur["5m"] != 4320 || omur["15m"] != 10080 || omur["1h"] != 20160 || omur["4h"] != 30240 || omur["1d"] != 43200 {
		t.Fatalf("varsayılan ömür tablosu yanlış: %v", omur)
	}

	// 1 dk: 1 gün ömür, 25 dk önce oluşturuldu → geçerli, 23 sa 35 dk kaldı
	d := dosyaOku(stratejiYaz(t, k, "AGTUSDT_1m.json", ornekStrateji("AGTUSDT", "1m", simdi.Add(-25*time.Minute))), omur, simdi)
	if d.Durum != "gecerli" || d.KalanMs != (gun-25*time.Minute).Milliseconds() || d.Not != "" {
		t.Fatalf("1 dk dosyası: %+v", d)
	}
	if d.Bitis-d.Olusturuldu != gun.Milliseconds() {
		t.Fatalf("1 dk ömrü 1 gün olmalı")
	}
	// 15 dk: 1 hafta ömür, 8 gün önce → süresi doldu (1 gün önce)
	d = dosyaOku(stratejiYaz(t, k, "BTCUSDT_15m.json", ornekStrateji("BTCUSDT", "15m", simdi.Add(-8*gun))), omur, simdi)
	if d.Durum != "suresiDoldu" || d.KalanMs != -gun.Milliseconds() {
		t.Fatalf("süresi dolmuş dosya: %+v", d)
	}
	// 15 dk: 6 gün önce → hâlâ geçerli
	d = dosyaOku(stratejiYaz(t, k, "ETHUSDT_15m.json", ornekStrateji("ETHUSDT", "15m", simdi.Add(-6*gun))), omur, simdi)
	if d.Durum != "gecerli" || d.KalanMs != gun.Milliseconds() {
		t.Fatalf("6 günlük 15 dk dosyası geçerli olmalı: %+v", d)
	}
	// 5 dk 3 gün, 1 sa 2 hafta, 4 sa 3 hafta, 1 gün 1 ay
	for _, c := range []struct {
		aralik string
		omur   time.Duration
	}{{"5m", 3 * gun}, {"1h", 14 * gun}, {"4h", 21 * gun}, {"1d", 30 * gun}} {
		d = dosyaOku(stratejiYaz(t, k, "SUIUSDT_"+c.aralik+".json", ornekStrateji("SUIUSDT", c.aralik, simdi.Add(-time.Hour))), omur, simdi)
		if d.Durum != "gecerli" || d.Bitis-d.Olusturuldu != c.omur.Milliseconds() {
			t.Fatalf("%s ömrü %v olmalı: %+v", c.aralik, c.omur, d)
		}
	}
	// Binance verisi → çalıştırılamaz
	ic := ornekStrateji("SOLUSDT", "15m", simdi.Add(-time.Hour))
	ic["borsa"] = "binance"
	d = dosyaOku(stratejiYaz(t, k, "SOLUSDT_15m.json", ic), omur, simdi)
	if d.Durum != "hatali" || !strings.Contains(d.Hata, "Binance") {
		t.Fatalf("Binance dosyası reddedilmeli: %+v", d)
	}
	// bozuk JSON
	_ = os.WriteFile(filepath.Join(k, "bozuk.json"), []byte(`{"tur": `), 0644)
	d = dosyaOku(filepath.Join(k, "bozuk.json"), omur, simdi)
	if d.Durum != "hatali" || !strings.Contains(d.Hata, "bozuk") {
		t.Fatalf("bozuk dosya: %+v", d)
	}
	// başka bir JSON
	d = dosyaOku(stratejiYaz(t, k, "ayarlar.json", map[string]any{"surum": 2}), omur, simdi)
	if d.Durum != "hatali" {
		t.Fatalf("strateji olmayan dosya reddedilmeli: %+v", d)
	}
	// gelecekte oluşturulmuş (saat hatası)
	d = dosyaOku(stratejiYaz(t, k, "ETHUSDT_5m.json", ornekStrateji("ETHUSDT", "5m", simdi.Add(time.Hour))), omur, simdi)
	if d.Durum != "hatali" || !strings.Contains(d.Hata, "saat") {
		t.Fatalf("gelecek tarihli dosya: %+v", d)
	}
	// ad içerikle uyuşmuyor (ör. OneDrive kopyası) → not düşülür ama çalışır
	d = dosyaOku(stratejiYaz(t, k, "BTCUSDT_15m (2).json", ornekStrateji("BTCUSDT", "15m", simdi.Add(-time.Hour))), omur, simdi)
	if d.Durum != "gecerli" || d.Not == "" {
		t.Fatalf("adı farklı dosya: %+v", d)
	}
	// UTF-8 BOM'lu dosya (Not Defteri ile kaydedilmiş olabilir)
	b, _ := json.Marshal(ornekStrateji("XRPUSDT", "1h", simdi.Add(-time.Hour)))
	_ = os.WriteFile(filepath.Join(k, "XRPUSDT_1h.json"), append([]byte("\xef\xbb\xbf"), b...), 0644)
	d = dosyaOku(filepath.Join(k, "XRPUSDT_1h.json"), omur, simdi)
	if d.Durum != "gecerli" {
		t.Fatalf("BOM'lu dosya okunmalı: %+v", d)
	}

	// ayarlardaki ömür tablosu kullanılır, saçma değerler yok sayılır
	o2 := omurTablosu(map[string]json.RawMessage{"omur": json.RawMessage(`{"1m": 60, "15m": -5, "2m": 30}`)})
	if o2["1m"] != 60 || o2["15m"] != 10080 {
		t.Fatalf("ayarlı ömür tablosu yanlış: %v", o2)
	}
}

func TestMotorKarsilastir(t *testing.T) {
	kok := t.TempDir()
	strat := filepath.Join(kok, "KriptoSimulator", "stratejiler")
	web := filepath.Join(kok, "KriptoSimulator", "kaynak", "web")
	_ = os.MkdirAll(strat, 0755)
	if m := motorKarsilastir(strat); m["durum"] != "bulunamadi" {
		t.Fatalf("kaynak yokken: %v", m)
	}
	_ = os.MkdirAll(web, 0755)
	for _, ad := range []string{"gostergeler.js", "kurallar.js", "motor.js"} {
		b, _ := webDosyalari.ReadFile("web/" + ad)
		// Windows satır sonlarıyla açılmış olsa da aynı sayılmalı
		_ = os.WriteFile(filepath.Join(web, ad), []byte(strings.ReplaceAll(string(b), "\n", "\r\n")), 0644)
	}
	if m := motorKarsilastir(strat); m["durum"] != "ayni" {
		t.Fatalf("aynı motor: %v", m)
	}
	_ = os.WriteFile(filepath.Join(web, "kurallar.js"), []byte("// yeni kural"), 0644)
	m := motorKarsilastir(strat)
	if m["durum"] != "farkli" || len(m["farkli"].([]string)) != 1 {
		t.Fatalf("farklı motor: %v", m)
	}
}

func TestKlasorBul(t *testing.T) {
	ev := t.TempDir()
	t.Setenv("USERPROFILE", ev)
	t.Setenv("ALSAT_KLASOR", "")
	eskiExe, eskiVeri := exeKlasoru, veriKlasoru
	defer func() { exeKlasoru, veriKlasoru = eskiExe, eskiVeri }()
	exeKlasoru = filepath.Join(ev, "baska", "KriptoAlSat")
	veriKlasoru = t.TempDir()
	_ = os.MkdirAll(exeKlasoru, 0755)

	if y, kaynak := klasorBul(); y != "" || kaynak != "yok" {
		t.Fatalf("hiç klasör yokken: %q %q", y, kaynak)
	}
	// Masaüstünde iki simülatör sürümü: yenisi seçilmeli
	eski := filepath.Join(ev, "OneDrive", "Masaüstü", "KriptoSimulator_v3_1", "KriptoSimulator", "stratejiler")
	yeni := filepath.Join(ev, "OneDrive", "Masaüstü", "KriptoSimulator_v3_2", "KriptoSimulator", "stratejiler")
	_ = os.MkdirAll(eski, 0755)
	_ = os.MkdirAll(yeni, 0755)
	y, kaynak := klasorBul()
	if y != yeni || kaynak != "bulundu" {
		t.Fatalf("bulunan %q (%s), beklenen %q", y, kaynak, yeni)
	}
	// bulunan ayarlara yazıldı; sonraki seferde ayardan gelir
	if y2, k2 := klasorBul(); y2 != yeni || k2 != "ayar" {
		t.Fatalf("ikinci sefer %q %q", y2, k2)
	}
	// ayarlardaki klasör silinirse kendiliğinden başka yere geçmez, kullanıcıya söylenir
	_ = os.RemoveAll(yeni)
	if y3, k3 := klasorBul(); y3 != yeni || k3 != "yok" {
		t.Fatalf("silinen klasör: %q %q", y3, k3)
	}
}
