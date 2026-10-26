package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 9) Bilgisayarın saati MEXC'den 5 sn ileriyse, kapanmasına 3 sn kalan (MEXC saatiyle) mum "kapandı" diye gönderiliyordu.
// 0.8: program MEXC saat farkını (borsaFarki) kullanır ve 1 sn pay bırakır.
func TestInceleme9_AcikSonYerelSaatle(t *testing.T) {
	const ileri = 5 * time.Second // bilgisayar saati MEXC'den bu kadar ileri
	kalanMs := int64(3000)
	sv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mexcSimdi := time.Now().Add(-ileri).UnixMilli()
		kapanis := mexcSimdi + kalanMs // şu anki 1m mumun kapanışı (MEXC saatiyle)
		acilis := kapanis - 60_000 + 1
		var out [][]any
		for i := 40; i >= 1; i-- {
			a := acilis - int64(i)*60_000
			out = append(out, []any{a, "1", "1", "1", "1", "10", a + 59_999, "10"})
		}
		out = append(out, []any{acilis, "1", "1.2", "0.9", "1.1", "10", kapanis, "10"})
		json.NewEncoder(w).Encode(out)
	}))
	defer sv.Close()
	eski := mexcAdresi
	mexcAdresi = sv.URL
	defer func() { mexcAdresi = eski }()
	eskiFark := borsaFarki.Load()
	borsaFarki.Store(-ileri.Milliseconds()) // programın 10 dk'da bir ölçtüğü fark
	defer borsaFarki.Store(eskiFark)
	sor := func() bool {
		rec := httptest.NewRecorder()
		mumlarIstegi(rec, httptest.NewRequest("GET", "/api/mumlar?borsa=mexc&sembol=AGTUSDT&aralik=1m&adet=30", nil))
		var j struct {
			AcikSon bool `json:"acikSon"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &j)
		return j.AcikSon
	}
	if !sor() {
		t.Errorf("0.7 HATASI GERİ GELDİ: kapanmamış mum (MEXC saatiyle 3 sn kaldı) kapanmış sayıldı")
	}
	kalanMs = -400 // mum MEXC saatiyle 0,4 sn önce kapandı: 1 sn pay içinde, hâlâ açık sayılır
	if !sor() {
		t.Errorf("1 sn pay çalışmıyor: az önce kapanan mum hemen kapanmış sayıldı")
	}
	kalanMs = -3000 // 3 sn önce kapandı: kapanmış sayılmalı
	if sor() {
		t.Errorf("3 sn önce kapanan mum hâlâ açık sayılıyor")
	}
}

// 10) Satış: limit satış borsaya ulaştı ama 502 döndü → piyasa satışı "bakiye yetersiz" → tekrar → hesapta serbest coin yok
// → işlem "elle satılmış" sayılıp kapanıyor; coin borsada sahipsiz limit emirde kilitli, zarar durdur yok.
func TestInceleme10_SatisLimitBelirsiz(t *testing.T) {
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
	a.kanca(func(w http.ResponseWriter, r *http.Request) bool {
		q := r.URL.Query()
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && q.Get("type") == "LIMIT" && q.Get("side") == "SELL" {
			icIsle(s, r)
			s.mu.Lock()
			s.bakiye["AGT"] = [2]float64{0, 100} // coin limit emirde kilitli
			s.mu.Unlock()
			w.WriteHeader(502)
			w.Write([]byte("bad gateway"))
			return true
		}
		if r.URL.Path == "/api/v3/order" && r.Method == "POST" && q.Get("type") == "MARKET" && q.Get("side") == "SELL" {
			w.WriteHeader(400)
			w.Write([]byte(`{"code":30005,"msg":"Oversold"}`))
			return true
		}
		return false
	})
	kumbaraSat(k, "motor", "test", 1.0, false)
	t.Logf("1. deneme: açık işlem=%v adet=%.2f satisTekrar=%v", k.acikIslem() != nil, k.Adet, k.SatisTekrar > 0)
	kumbaraSat(k, "motor", "satış tekrar deneniyor", 1.0, true) // izleyicinin tekrar denemesi
	la, _, _, liste := emirleriSay(s)
	t.Logf("2. deneme: açık işlem=%v adet=%.2f; borsa=%v", k.acikIslem() != nil, k.Adet, liste)
	for _, o := range k.Olaylar {
		t.Logf("  olay: %s", o.Mesaj)
	}
	if k.acikIslem() == nil && k.Adet == 0 && la == 1 {
		t.Errorf("0.7 HATASI GERİ GELDİ: kumbara işlemi kapattı (sonuç -%%100 yazıldı) ama 100 AGT borsada sahipsiz limit satışta duruyor")
	}
}

// 11) durum.json boyutu ve her karar bildiriminde yazılması
func TestInceleme11_DurumBoyutu(t *testing.T) {
	saatKur(t)
	veriKlasoru = t.TempDir()
	D = yeniDurum()
	ic := []byte(`{"tur":"kripto-simulator-strateji","strateji":{"kurallar":{}},"aciklama":"` + string(make([]byte, 0)) + `"}`)
	ic = append([]byte(nil), ic...)
	pad := make([]byte, 6000)
	for i := range pad {
		pad[i] = 'x'
	}
	ic = []byte(`{"tur":"kripto-simulator-strateji","strateji":{"pad":"` + string(pad) + `"}}`)
	durumMu.Lock()
	for i := 0; i < 45; i++ {
		k := kumbaraOlustur(acmaIstegi{Dosya: "X.json", Icerik: ic, Sembol: "AGTUSDT", Aralik: "1m", Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000, Mod: "gercek", Tutar: 100})
		for j := 0; j < 300; j++ {
			k.Olaylar = append(k.Olaylar, kumbaraOlayi{Zaman: int64(j), Tur: "bilgi", Mesaj: "Limit alış verildi: 99,90 adet × 1,001 (alıcı sırasının başı). 15 sn bekleniyor " + string(rune('a'+j%26))})
		}
		for j := 0; j < 60; j++ {
			d := dolumSonucu{Taraf: "BUY", Parcalar: []parca{{Fiyat: 1, Miktar: 1}, {Fiyat: 1, Miktar: 1}}}
			k.Islemler = append(k.Islemler, kumbaraIslemi{No: j + 1, Alis: &d, Satis: &d})
		}
		if i >= 5 {
			k.Durum, k.BitisZamani = "bitti", int64(i)
		}
	}
	basla := time.Now()
	for i := 0; i < 10; i++ {
		degisti()
	}
	sure := time.Since(basla) / 10
	durumMu.Unlock()
	st, _ := os.Stat(filepath.Join(kayitKlasoru(), "durum.json"))
	t.Logf("durum.json %d KB; her kayıt (kilit tutulurken) %v; 1m kumbara ekranda: 12 kayıt/dk → %.1f GB/gün", st.Size()/1024, sure, float64(st.Size())*12*60*24/1e9)
}
