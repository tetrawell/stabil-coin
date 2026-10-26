package main

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------- sınır hesabı ----------

// n işlemlik örnek geçmiş: karli tanesi kazanç (ort. kaz), gerisi kayıp (ort. kay), saatte bir işlem
func ornekGecmis(n, karli int, kaz, kay float64, tohum int64) []korumaIslemi {
	rg := rand.New(rand.NewSource(tohum))
	y := make([]float64, n)
	for i := range y {
		if i < karli {
			y[i] = math.Max(0.05, kaz*(0.4+1.2*rg.Float64()))
		} else {
			y[i] = -math.Max(0.1, kay*(0.5+rg.Float64()))
		}
	}
	rg.Shuffle(n, func(i, j int) { y[i], y[j] = y[j], y[i] })
	out := make([]korumaIslemi, n)
	bas := int64(1_760_000_000_000)
	for i := range y {
		g := bas + int64(i)*3_600_000
		out[i] = korumaIslemi{G: g, C: g + 1_800_000, Y: y[i]}
	}
	return out
}

func TestKorumaHesabiHerCoineFarkli(t *testing.T) {
	saat := int64(3_600_000)
	hareketli := korumaGirdisi{Islemler: ornekGecmis(60, 32, 8.9, 3.5, 1), Sure: 60 * saat, AralikMs: 60_000, OmurMs: 24 * saat, Hassasiyet: "dengeli"}
	sakin := korumaGirdisi{Islemler: ornekGecmis(40, 30, 0.6, 1.0, 2), Sure: 40 * saat, AralikMs: 900_000, OmurMs: 7 * 24 * saat, Hassasiyet: "dengeli"}
	h, hk := korumaHesapla(hareketli)
	s, sk := korumaHesapla(sakin)
	t.Logf("hareketli: %+v %+v", h, hk)
	t.Logf("sakin: %+v %+v", s, sk)
	if hk.Yedek || sk.Yedek {
		t.Fatal("20'den fazla işlemde torba kullanılmalı")
	}
	if h.Kayip <= s.Kayip*2 {
		t.Fatalf("hareketli coinin kayıp sınırı sakinden çok büyük olmalı: %v / %v", h.Kayip, s.Kayip)
	}
	if h.Seri < 3 || s.Seri < 3 || h.Seri > 20 {
		t.Fatalf("seri sınırı 3 ile 20 arasında olmalı: %d %d", h.Seri, s.Seri)
	}
	if h.Kayip < 1.2*hk.EnBuyukZarar-1e-9 {
		t.Fatalf("tek bir normal zarar alarmı çalıştırmamalı: kayıp %v, en büyük zarar %v", h.Kayip, hk.EnBuyukZarar)
	}
	if h.GeriDonus != 2 || s.GeriDonus != 3 {
		t.Fatalf("geri dönüş: %%53 kârlıda 2, %%75 kârlıda 3 olmalı: %d %d", h.GeriDonus, s.GeriDonus)
	}
	if math.Abs(hk.SaatteIslem-1) > 1e-9 || h.HizPencere != 4*saat {
		t.Fatalf("saatte 1 işlem → hız penceresi 4 saat olmalı: %v %d", hk.SaatteIslem, h.HizPencere)
	}
	if h.HizSayi < 8 {
		t.Fatalf("hız sınırı en az normalin 2 katı (8) olmalı: %d", h.HizSayi)
	}
	// aynı geçmiş hep aynı sınırları verir
	if h2, _ := korumaHesapla(hareketli); h2 != h {
		t.Fatalf("hesap tekrarlanabilir olmalı: %+v / %+v", h, h2)
	}
	// hassasiyet: sakin ≥ dengeli ≥ hassas
	hareketli.Hassasiyet = "sakin"
	sa, _ := korumaHesapla(hareketli)
	hareketli.Hassasiyet = "hassas"
	ha, _ := korumaHesapla(hareketli)
	if !(sa.Seri >= h.Seri && h.Seri >= ha.Seri && sa.Kayip >= h.Kayip && h.Kayip >= ha.Kayip && sa.HizSayi >= h.HizSayi && h.HizSayi >= ha.HizSayi) {
		t.Fatalf("hassasiyet sırası bozuk: sakin %+v dengeli %+v hassas %+v", sa, h, ha)
	}
}

func TestKorumaAzGecmisYedekKurallar(t *testing.T) {
	g := korumaGirdisi{Islemler: ornekGecmis(9, 5, 2, 1.5, 3), Sure: 9 * 3_600_000, AralikMs: 14_400_000, OmurMs: 21 * 86_400_000, Hassasiyet: "dengeli", Stop: 4}
	s, k := korumaHesapla(g)
	if !k.Yedek || s.Seri != 3 || s.GeriDonus != 2 || s.HizSayi != 8 {
		t.Fatalf("yedek kurallar bekleniyordu: %+v %+v", s, k)
	}
	if s.Kayip != yukariYuvarla(2*math.Max(k.EnBuyukZarar, 4), 1) {
		t.Fatalf("yedek kayıp: en büyük zararın ya da zarar durdurun 2 katı olmalı: %v (%v)", s.Kayip, k.EnBuyukZarar)
	}
	// hiç geçmiş yok, zarar durdur yok: simülatörün sonucundan hız, %8 kayıp
	s, k = korumaHesapla(korumaGirdisi{AralikMs: 60_000, OmurMs: 86_400_000, SimIslem: 50, SimSure: 5000 * 60_000})
	if s.Kayip != 8 || s.HizSayi != 8 || s.HizPencere != int64(4/(50.0/(5000*60_000))) {
		t.Fatalf("geçmişsiz yedek: %+v %+v", s, k)
	}
	// hiç bilgi yoksa hız alarmı hesaplanamaz
	if s, _ = korumaHesapla(korumaGirdisi{AralikMs: 60_000}); s.HizSayi != 0 || s.HizPencere != 0 {
		t.Fatalf("bilgisiz hız alarmı kapalı olmalı: %+v", s)
	}
}

func TestKorumaDosyaBilgisi(t *testing.T) {
	ic := []byte(`{"strateji":{"kurallar":{"zararDurdur":{"acik":true,"d":{"yuzde":5.3}}}},"gecmisSonuc":{"mumSayisi":5000,"islemSayisi":60}}`)
	stop, n, sure := korumaDosyaBilgisi(ic, 60_000)
	if stop != 5.3 || n != 60 || sure != 5000*60_000 {
		t.Fatalf("dosya bilgisi: %v %v %v", stop, n, sure)
	}
	if stop, _, _ = korumaDosyaBilgisi([]byte(`{"strateji":{"kurallar":{"zararDurdur":{"acik":false,"d":{"yuzde":5}}}}}`), 60_000); stop != 0 {
		t.Fatal("kapalı zarar durdur sayılmamalı")
	}
}

// ---------- alarmlar ----------

var testKurali = sembolKurali{Varlik: "AGT", FiyatOndalik: 4, MiktarOndalik: 2}

// kumbaradaki bütün parayla 1 fiyattan alıp 1+y/100'den satar (masrafsız)
func islemYap(k *kumbara, saat *testSaati, y float64) {
	durumMu.Lock()
	defer durumMu.Unlock()
	tutar := k.Nakit
	alisIsle(k, dolumSonucu{Taraf: "BUY", Ortalama: 1, Miktar: tutar, NetMiktar: tutar, Tutar: tutar, NetTutar: tutar, Yol: "limit", Bitis: simdiF().UnixMilli()}, 1, 1, testKurali, "")
	saat.ileri(5 * time.Minute)
	gelen := tutar * (1 + y/100)
	satisIsle(k, dolumSonucu{Taraf: "SELL", Ortalama: 1 + y/100, Miktar: tutar, NetMiktar: tutar, Tutar: gelen, NetTutar: gelen, Yol: "limit", Bitis: simdiF().UnixMilli()}, "motor", "", 0, testKurali, "")
	saat.ileri(time.Minute)
}

func korumaliKumbara(t *testing.T, s korumaSinirlari, tavan float64) (*kumbara, *testSaati) {
	k, saat, _ := testKumbarasi(t)
	eski := korumaGenelAl()
	korumaGenel.Store(korumaGenelAyar{Tavan: tavan, Hassasiyet: "dengeli"})
	t.Cleanup(func() { korumaGenel.Store(eski) })
	durumMu.Lock()
	korumaKurTest(k, nil, 0, "dengeli")
	k.Koruma.Sinir = s
	durumMu.Unlock()
	saat.ileri(time.Second)
	return k, saat
}

// durumMu kilitliyken: geçmişten hesaplayıp asıl korumayı kurar
func korumaKurTest(k *kumbara, islemler []korumaIslemi, sure int64, h string) {
	h, s, kar, temiz := korumaSinirHesapla(k.Icerik, k.Aralik, k.Bitis-k.Baslangic, islemler, sure, h)
	korumaKur(k, h, s, kar, temiz, false)
}

func denetle(k *kumbara) bool {
	durumMu.Lock()
	defer durumMu.Unlock()
	return korumaDenetle(k)
}

func TestKorumaArtArdaZarar(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 3, Kayip: 50, GeriDonus: 2}, 0)
	for i, y := range []float64{-1, -1, 2, -1, -1} {
		islemYap(k, saat, y)
		if denetle(k) {
			t.Fatalf("%d. işlemde erken kenara alındı", i+1)
		}
	}
	islemYap(k, saat, -1)
	if !denetle(k) || !k.Koruma.Kenarda || !strings.Contains(k.Koruma.KenarSebep, "art arda 3 zarar") {
		t.Fatalf("3. ardışık zararda kenara alınmalıydı: %+v", k.Koruma)
	}
	if son := k.Olaylar[len(k.Olaylar)-1]; son.Tur != "uyari" || !strings.HasPrefix(son.Mesaj, "Koruma: ") {
		t.Fatalf("olay yazılmalıydı: %+v", son)
	}
	if len(k.Koruma.Donemler) != 1 || k.Koruma.Donemler[0].Bit != 0 {
		t.Fatalf("kenar dönemi açılmalıydı: %+v", k.Koruma.Donemler)
	}
}

func TestKorumaTavanVeCoininSiniri(t *testing.T) {
	// coinin kendi sınırı %25, tavan %10 → %10'da kenara
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 20, Kayip: 25, GeriDonus: 2}, 10)
	islemYap(k, saat, 5) // tepe 105 (bakılmadan geçilse de zincirden sayılır)
	islemYap(k, saat, -6)
	if denetle(k) {
		t.Fatal("%6 düşüşte kenara alınmamalı")
	}
	islemYap(k, saat, -5) // 105 → 98,7 → 93,77: %10,7
	if !denetle(k) || !strings.Contains(k.Koruma.KenarSebep, "senin tavanın") {
		t.Fatalf("tavanda kenara alınmalıydı: %+v", k.Koruma)
	}
	// tavan yok: coinin kendi sınırı geçerli
	k2, saat2 := korumaliKumbara(t, korumaSinirlari{Seri: 20, Kayip: 12, GeriDonus: 2}, 0)
	islemYap(k2, saat2, -11)
	if denetle(k2) {
		t.Fatal("%11 düşüşte (sınır %12) kenara alınmamalı")
	}
	islemYap(k2, saat2, -2)
	if !denetle(k2) || !strings.Contains(k2.Koruma.KenarSebep, "bu ayarın kendi sınırı") {
		t.Fatalf("coinin sınırında kenara alınmalıydı: %+v", k2.Koruma)
	}
}

func TestKorumaAsiriHizlanma(t *testing.T) {
	s := korumaSinirlari{Seri: 20, Kayip: 50, HizSayi: 5, HizPencere: 40 * 60_000, GeriDonus: 2}
	k, saat := korumaliKumbara(t, s, 0)
	// 6 dk'da bir işlem; kârda: alarm yok
	for _, y := range []float64{1, 1, -0.5, 1, 0.5, 1} {
		islemYap(k, saat, y)
		if denetle(k) {
			t.Fatalf("kârda hız alarmı çalışmamalı: %+v", k.Koruma)
		}
	}
	k2, saat2 := korumaliKumbara(t, s, 0)
	for i, y := range []float64{1, -1, -1, 0.5, -1} {
		islemYap(k2, saat2, y)
		if got := denetle(k2); got != (i == 4) {
			t.Fatalf("%d. işlem: kenara alma %v", i+1, got)
		}
	}
	if !strings.Contains(k2.Koruma.KenarSebep, "5 işlem yaptı ve toplamda zararda") {
		t.Fatalf("hız sebebi: %s", k2.Koruma.KenarSebep)
	}
	// pencere dışına çıkan işlemler sayılmaz
	k3, saat3 := korumaliKumbara(t, s, 0)
	for _, y := range []float64{-1, -1, 2, -1} {
		islemYap(k3, saat3, y)
		denetle(k3)
	}
	saat3.ileri(time.Hour)
	islemYap(k3, saat3, -0.5)
	if denetle(k3) || k3.Koruma.Kenarda {
		t.Fatal("pencere dışındaki işlemler sayılmamalı")
	}
}

func TestKorumaKurulmadanOncekiIslemlerSayilmaz(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	islemYap(k, saat, -1)
	islemYap(k, saat, -1)
	durumMu.Lock()
	korumaKurTest(k, nil, 0, "dengeli")
	k.Koruma.Sinir.Seri = 3
	durumMu.Unlock()
	islemYap(k, saat, -1)
	if denetle(k) {
		t.Fatal("koruma kurulmadan önceki zararlar sayılmamalı")
	}
	if o := korumaOlc(k, simdiF().UnixMilli()); o.Seri != 1 {
		t.Fatalf("seri 1 olmalı: %+v", o)
	}
}

func TestKorumaKapaliAlarm(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 2, Kayip: 50, GeriDonus: 2}, 0)
	k.Koruma.SeriKapali = true
	for i := 0; i < 4; i++ {
		islemYap(k, saat, -0.5)
		if denetle(k) {
			t.Fatal("kapalı alarm kenara almamalı")
		}
	}
}

// ---------- kenarda: alım yok, gölge işlemler, oyuna dönüş ----------

func TestKorumaGolgeVeOyunaDonus(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 1, Kayip: 50, GeriDonus: 2}, 0)
	islemYap(k, saat, -1)
	if !denetle(k) {
		t.Fatal("kenara alınmalıydı")
	}
	kenar := k.Koruma.KenarZamani
	mum := func(ms int64) int64 { return ms / 60_000 * 60_000 }
	f := func(x float64) *float64 { return &x }
	i := func(x int64) *int64 { return &x }
	onceki := mum(kenar) - 3*60_000 // kenara alınmadan önce açılmış işlem: sayılmaz
	saat.ileri(2 * time.Minute)
	m1 := mum(simdiF().UnixMilli())
	saat.ileri(61 * time.Second)
	// motor yeni mumda aldı: kenarda olduğu için alım yok
	e := kararIsle(k, &kararIstegi{Mum: m1, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}, MotorGirisMum: m1,
		MotorIslemleri: []motorIslemi{{G: onceki, GF: 1, C: i(onceki + 60_000), CF: f(1.05)}}})
	if e.tur != "" {
		t.Fatalf("kenardayken alım yapılmamalı: %+v", e)
	}
	if len(k.Koruma.Golge) != 0 {
		t.Fatalf("kenardan önce açılan işlem gölge sayılmamalı: %+v", k.Koruma.Golge)
	}
	// gölge: zarar, kâr, kâr → oyuna döner
	gecmis := []motorIslemi{{G: onceki, GF: 1, C: i(onceki + 60_000), CF: f(1.05)}, {G: m1, GF: 1, C: i(m1 + 60_000), CF: f(0.99)}}
	saat.ileri(time.Minute)
	kararIsle(k, &kararIstegi{Mum: m1 + 60_000, Asama: "bekle", MotorIslemleri: gecmis})
	if len(k.Koruma.Golge) != 1 || k.Koruma.Golge[0].Y >= 0 || !k.Koruma.Kenarda {
		t.Fatalf("bir zararlı gölge işlem bekleniyordu: %+v", k.Koruma)
	}
	gecmis = append(gecmis, motorIslemi{G: m1 + 120_000, GF: 1, C: i(m1 + 180_000), CF: f(1.01)})
	saat.ileri(2 * time.Minute)
	kararIsle(k, &kararIstegi{Mum: m1 + 180_000, Asama: "bekle", MotorIslemleri: gecmis})
	if !k.Koruma.Kenarda {
		t.Fatal("bir kârlı gölge işlem yetmemeli (sınır 2)")
	}
	gecmis = append(gecmis, motorIslemi{G: m1 + 240_000, GF: 1, C: i(m1 + 300_000), CF: f(1.008)})
	saat.ileri(2 * time.Minute)
	son := m1 + 300_000
	e = kararIsle(k, &kararIstegi{Mum: son, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}, MotorIslemleri: gecmis})
	if k.Koruma.Kenarda {
		t.Fatalf("art arda 2 kârlı gölge işlemde oyuna dönmeliydi: %+v", k.Koruma)
	}
	if e.tur != "" {
		t.Fatal("oyuna dönmeden önce kapanan mumun alımı yapılmamalı")
	}
	if d := k.Koruma.Donemler; len(d) != 1 || d[0].Bit == 0 || !strings.Contains(d[0].Donus, "art arda 2 kârlı") {
		t.Fatalf("kenar dönemi kapanmalıydı: %+v", d)
	}
	// sonraki mumda yeni alım yapılır
	saat.ileri(time.Minute)
	if e = kararIsle(k, &kararIstegi{Mum: son + 60_000, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}, MotorGirisMum: son + 60_000}); e.tur != "al" {
		t.Fatalf("oyuna döndükten sonra alım yapılmalı: %+v", e)
	}
}

func TestKorumaElleGeriDonusVeSifirlama(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 2, Kayip: 50, GeriDonus: 2}, 0)
	islemYap(k, saat, -1)
	islemYap(k, saat, -1)
	if !denetle(k) {
		t.Fatal("kenara alınmalıydı")
	}
	durumMu.Lock()
	oyunaDon(k, "elle geri döndürüldü", k.SonKararMum)
	durumMu.Unlock()
	if k.Koruma.Kenarda || math.Abs(k.Koruma.Tepe-k.deger()) > 1e-9 {
		t.Fatalf("oyuna dönünce tepe sıfırlanmalı: %+v", k.Koruma)
	}
	islemYap(k, saat, -1)
	if denetle(k) {
		t.Fatal("oyuna dönünce seri sıfırdan sayılmalı")
	}
	islemYap(k, saat, -1)
	if !denetle(k) {
		t.Fatal("yeni seride yine kenara alınmalı")
	}
	if len(k.Koruma.Donemler) != 2 {
		t.Fatalf("iki kenar dönemi bekleniyordu: %+v", k.Koruma.Donemler)
	}
}

func TestKorumaKumbaraKapanincaDonemKapanir(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 1, Kayip: 50, GeriDonus: 2}, 0)
	islemYap(k, saat, -1)
	denetle(k)
	durumMu.Lock()
	kumbaraBitir(k, "durduruldu")
	durumMu.Unlock()
	if d := k.Koruma.Donemler; d[0].Bit == 0 || d[0].Donus != "kumbara kapandı" {
		t.Fatalf("kumbara kapanınca kenar dönemi kapanmalı: %+v", d)
	}
}

// ---------- ekrana giden özet, kayıt ----------

func TestKorumaOzetiVeKayit(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	durumMu.Lock()
	korumaKurTest(k, ornekGecmis(30, 15, 2, 1.5, 4), 30*3_600_000, "hassas")
	k.Koruma.Saglamlik = &saglamlikSonucu{Gun: 10.4, Bot: -12, AlTut: -30, Islem: 85, Karli: 34, Dusus: 25}
	durumMu.Unlock()
	islemYap(k, saat, -1)
	denetle(k)
	m := kumbaraOzeti(k, 10)
	kr, ok := m["koruma"].(map[string]any)
	if !ok {
		t.Fatal("özette koruma olmalı")
	}
	if _, var_ := kr["gecmis"]; var_ {
		t.Fatal("geçmiş işlemler ekrana gönderilmemeli")
	}
	if kr["hassasiyetAd"] != "Hassas" || kr["olcum"] == nil || kr["kayipEtkin"] == nil {
		t.Fatalf("özet eksik: %+v", kr)
	}
	// durum.json'a yazılıp okununca aynı
	b, _ := json.Marshal(k)
	var k2 kumbara
	if err := json.Unmarshal(b, &k2); err != nil || k2.Koruma == nil || len(k2.Koruma.Gecmis) != 30 || k2.Koruma.Sinir != k.Koruma.Sinir || k2.Koruma.Saglamlik.Islem != 85 {
		t.Fatalf("kayıt bozuk: %v %+v", err, k2.Koruma)
	}
}

func TestKorumaGenelAyar(t *testing.T) {
	a := korumaAyariOku(map[string]json.RawMessage{})
	if a.Tavan != 10 || a.Hassasiyet != "dengeli" {
		t.Fatalf("varsayılan: %+v", a)
	}
	a = korumaAyariOku(map[string]json.RawMessage{"koruma": json.RawMessage(`{"tavan":0,"hassasiyet":"sakin"}`)})
	if a.Tavan != 0 || a.Hassasiyet != "sakin" {
		t.Fatalf("tavan yok + sakin: %+v", a)
	}
	a = korumaAyariOku(map[string]json.RawMessage{"koruma": json.RawMessage(`{"tavan":500,"hassasiyet":"cilgin"}`)})
	if a.Tavan != 10 || a.Hassasiyet != "dengeli" {
		t.Fatalf("geçersiz değerler varsayılana dönmeli: %+v", a)
	}
	eskiKlasor := veriKlasoru
	veriKlasoru = t.TempDir()
	t.Cleanup(func() { veriKlasoru = eskiKlasor })
	if err := korumaGenelYaz(func(g *korumaGenelAyar) { g.Tavan = 15 }); err != nil {
		t.Fatal(err)
	}
	if korumaGenelAl().Tavan != 15 {
		t.Fatal("tavan kaydedilmeli")
	}
	korumaGenelYukle()
	if korumaGenelAl().Tavan != 15 {
		t.Fatal("tavan dosyadan okunmalı")
	}
	korumaGenel.Store(korumaGenelAyar{Tavan: 10, Hassasiyet: "dengeli"})
}

func TestSaglamlikTemizle(t *testing.T) {
	if (&saglamlikSonucu{Gun: math.NaN()}).temizle() != nil {
		t.Fatal("NaN reddedilmeli")
	}
	if (&saglamlikSonucu{Islem: 3, Karli: 5}).temizle() != nil {
		t.Fatal("kârlı işlem toplamdan çok olamaz")
	}
	if (&saglamlikSonucu{Gun: 3, Bot: 5, Islem: 3, Karli: 2}).temizle() == nil {
		t.Fatal("geçerli sonuç kabul edilmeli")
	}
}

// ---------- incelemeden sonra eklenenler ----------

// Tavan toplam kaybı sınırlar: oyuna dönünce coinin tepesi sıfırlanır ama tavanınki sıfırlanmaz
func TestKorumaTavanToplamKaybi(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 2, Kayip: 50, GeriDonus: 2}, 10)
	islemYap(k, saat, -4)
	islemYap(k, saat, -3) // 100 → 92,9: art arda 2 zarar
	if !denetle(k) || !strings.Contains(k.Koruma.KenarSebep, "art arda 2") {
		t.Fatalf("seri ile kenara alınmalıydı: %+v", k.Koruma)
	}
	durumMu.Lock()
	oyunaDon(k, "test", 0)
	k.Koruma.Sinir.Seri = 10 // bundan sonra sadece tavan çalışsın
	durumMu.Unlock()
	islemYap(k, saat, -2) // 92,9 → 91,0: toplam %9
	if denetle(k) {
		t.Fatal("%9 toplam kayıpta (tavan %10) kenara alınmamalı")
	}
	islemYap(k, saat, -1.5) // → 89,7: toplam %10,3
	if !denetle(k) || !strings.Contains(k.Koruma.KenarSebep, "senin tavanın") {
		t.Fatalf("toplam %%10 kayıpta tavan kenara almalıydı: %+v", k.Koruma)
	}
}

// Kârlı işlem kenara aldırmaz; değer zinciri işlemin net sonucuyla (paranın bir kısmıyla yapılan işlemde de doğru)
func TestKorumaKarliIslemKenaraAlmaz(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 5, Kayip: 4, GeriDonus: 2}, 0)
	durumMu.Lock()
	yarim := k.Nakit / 2 // gerçek parada hesapta daha az USDT olunca böyle olabilir
	alisIsle(k, dolumSonucu{Taraf: "BUY", Ortalama: 1, Miktar: yarim, NetMiktar: yarim, Tutar: yarim, NetTutar: yarim, Yol: "limit", Bitis: simdiF().UnixMilli()}, 1, 1, testKurali, "")
	saat.ileri(time.Minute)
	satisIsle(k, dolumSonucu{Taraf: "SELL", Ortalama: 1.1, Miktar: yarim, NetMiktar: yarim, Tutar: yarim * 1.1, NetTutar: yarim * 1.1, Yol: "limit", Bitis: simdiF().UnixMilli()}, "motor", "", 0, testKurali, "")
	durumMu.Unlock()
	saat.ileri(time.Minute)
	if denetle(k) {
		t.Fatal("kârlı işlem kenara aldırmamalı")
	}
	if math.Abs(k.Koruma.Tepe-105) > 1e-9 || math.Abs(k.Koruma.SonDeger-105) > 1e-9 {
		t.Fatalf("tepe ve değer 105 olmalı: %v %v", k.Koruma.Tepe, k.Koruma.SonDeger)
	}
}

// Koruma kurulurken açık olan işlem alarmlarda sayılmaz (değeri ise zincire girer)
func TestKorumaKurulurkenAcikIslemSayilmaz(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	acikIslemKoy(k, 1, 100)
	durumMu.Lock()
	korumaKurTest(k, nil, 0, "dengeli")
	k.Koruma.Sinir.Seri = 1
	satisIsle(k, dolumSonucu{Taraf: "SELL", Ortalama: 0.95, Miktar: 100, NetMiktar: 100, Tutar: 95, NetTutar: 95, Yol: "limit", Bitis: simdiF().Add(time.Minute).UnixMilli()}, "motor", "", 0, testKurali, "")
	durumMu.Unlock()
	saat.ileri(2 * time.Minute)
	if denetle(k) {
		t.Fatal("kurulurken açık olan işlem kenara aldırmamalı")
	}
	if math.Abs(k.Koruma.SonDeger-95) > 1e-9 {
		t.Fatalf("değer 95 olmalı: %v", k.Koruma.SonDeger)
	}
	islemYap(k, saat, -1)
	if !denetle(k) {
		t.Fatal("sonraki zarar sayılmalı (sınır 1)")
	}
}

// Çok büyük sonuçlar NaN üretmez, özet yine hazırlanır
func TestKorumaTasmaYok(t *testing.T) {
	l := make([]korumaIslemi, 400)
	for i := range l {
		l[i] = korumaIslemi{G: int64(1_760_000_000_000 + i*60_000), C: int64(1_760_000_030_000 + i*60_000), Y: 1000}
	}
	s, kar := korumaHesapla(korumaGirdisi{Islemler: l, Sure: 400 * 60_000, AralikMs: 60_000, OmurMs: 86_400_000, Hassasiyet: "dengeli"})
	if math.IsNaN(kar.EnBuyukDusus) || math.IsNaN(s.Kayip) || math.IsInf(s.Kayip, 0) {
		t.Fatalf("NaN/Inf olmamalı: %+v %+v", s, kar)
	}
	k, _, _ := testKumbarasi(t)
	durumMu.Lock()
	korumaKurTest(k, l, 400*60_000, "dengeli")
	m := kumbaraOzeti(k, 5)
	durumMu.Unlock()
	if m["koruma"] == nil || m["id"] != k.ID {
		t.Fatalf("özet bozuk: %v", m["koruma"])
	}
	if _, err := json.Marshal(D); err != nil {
		t.Fatal("durum yazılabilmeli:", err)
	}
}

// Zararsız geçmişte bile tek bir zarar durdur tetiklemesi kayıp alarmını çalıştırmaz
func TestKorumaZararDurdurTabani(t *testing.T) {
	l := ornekGecmis(30, 30, 1, 1, 5)
	s, _ := korumaHesapla(korumaGirdisi{Islemler: l, Sure: 30 * 3_600_000, AralikMs: 60_000, OmurMs: 86_400_000, Hassasiyet: "hassas", Stop: 3})
	if s.Kayip < 3.6-1e-9 || s.Seri != 3 {
		t.Fatalf("kayıp en az zarar durdurun 1,2 katı, seri 3 olmalı: %+v", s)
	}
}

// Korumasız kumbaraya hemen geçici koruma; sayfa geçmişi gönderince durum korunarak asıl sınırlara geçilir
func TestKorumaGeciciVeGuncelleme(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	aktifOturum.Store("test-oturum")
	t.Cleanup(func() { aktifOturum.Store("") })
	durumMu.Lock()
	korumaGeciciKur(k)
	durumMu.Unlock()
	if k.Koruma == nil || !k.Koruma.Gecici || !k.Koruma.Karakter.Yedek {
		t.Fatalf("geçici (yedek) koruma bekleniyordu: %+v", k.Koruma)
	}
	islemYap(k, saat, 3)
	denetle(k)
	tepe := k.Koruma.Tepe
	govde, _ := json.Marshal(map[string]any{"oturum": "test-oturum", "id": k.ID, "islemler": ornekGecmis(40, 25, 1, 1, 6), "sure": 40 * 3_600_000})
	w := httptest.NewRecorder()
	kumbaraKorumaIstegi(w, httptest.NewRequest("POST", "/api/kumbara/koruma", bytes.NewReader(govde)))
	if w.Code != 200 {
		t.Fatalf("istek: %d %s", w.Code, w.Body.String())
	}
	kr := k.Koruma
	if kr.Gecici || kr.Karakter.Yedek || kr.Karakter.Islem != 40 || len(kr.Gecmis) != 40 || kr.Tepe != tepe {
		t.Fatalf("asıl sınırlara geçmeli, durum korunmalı: %+v", kr)
	}
	// ikinci istek hiçbir şeyi değiştirmez
	once := *kr
	w = httptest.NewRecorder()
	kumbaraKorumaIstegi(w, httptest.NewRequest("POST", "/api/kumbara/koruma", bytes.NewReader(govde)))
	if w.Code != 200 || kr.Sinir != once.Sinir || kr.Baslangic != once.Baslangic {
		t.Fatal("kurulu korumaya ikinci istek dokunmamalı")
	}
}

// Eski sürümün durum.json'undaki çalışan kumbaraya açılışta geçici koruma
func TestDurumYukleyinceGeciciKoruma(t *testing.T) {
	k, _, _ := testKumbarasi(t)
	durumMu.Lock()
	durumKaydet()
	durumMu.Unlock()
	if k.Koruma != nil {
		t.Fatal("test kumbarası korumasız başlamalı")
	}
	durumYukle()
	durumMu.Lock()
	y := kumbaraBul(k.ID)
	durumMu.Unlock()
	if y == nil || y.Koruma == nil || !y.Koruma.Gecici {
		t.Fatalf("açılışta geçici koruma kurulmalı: %+v", y)
	}
}

func TestKorumaAyarIstegi(t *testing.T) {
	k, _ := korumaliKumbara(t, korumaSinirlari{Seri: 3, Kayip: 8, GeriDonus: 2}, 10)
	eskiKlasor := veriKlasoru
	t.Cleanup(func() { veriKlasoru = eskiKlasor; korumaGenel.Store(korumaGenelAyar{Tavan: 10, Hassasiyet: "dengeli"}) })
	aktifOturum.Store("test-oturum")
	t.Cleanup(func() { aktifOturum.Store("") })
	gonder := func(m map[string]any) *httptest.ResponseRecorder {
		m["oturum"], m["id"] = "test-oturum", k.ID
		b, _ := json.Marshal(m)
		w := httptest.NewRecorder()
		kumbaraKorumaAyarIstegi(w, httptest.NewRequest("POST", "/api/kumbara/koruma-ayar", bytes.NewReader(b)))
		return w
	}
	if w := gonder(map[string]any{"hassasiyet": "dengeli", "seri": 0, "kayip": 8, "geriDonus": 2}); w.Code != 400 {
		t.Fatalf("seri 0 reddedilmeli: %d", w.Code)
	}
	if w := gonder(map[string]any{"hassasiyet": "cilgin", "seri": 3, "kayip": 8, "geriDonus": 2}); w.Code != 400 {
		t.Fatalf("geçersiz hassasiyet reddedilmeli: %d", w.Code)
	}
	// kapalı alarmın boş kutusu otomatik değerle doldurulur; tavan yazılır
	w := gonder(map[string]any{"hassasiyet": "hassas", "seri": 4, "seriKapali": false, "kayip": 0, "kayipKapali": true, "geriDonus": 3, "tavan": 12.5})
	if w.Code != 200 {
		t.Fatalf("kaydedilmeli: %d %s", w.Code, w.Body.String())
	}
	kr := k.Koruma
	if kr.Sinir.Seri != 4 || kr.Sinir.Kayip != kr.Oto.Kayip || !kr.KayipKapali || kr.Sinir.GeriDonus != 3 || kr.Hassasiyet != "hassas" || !kr.Elle {
		t.Fatalf("ayarlar yanlış: %+v", kr)
	}
	if korumaGenelAl().Tavan != 12.5 {
		t.Fatalf("tavan kaydedilmeli: %v", korumaGenelAl().Tavan)
	}
	// önizleme hiçbir şeyi değiştirmez
	once := kr.Sinir
	if w := gonder(map[string]any{"hassasiyet": "sakin", "onizle": true}); w.Code != 200 || kr.Sinir != once || kr.Hassasiyet != "hassas" {
		t.Fatalf("önizleme değiştirmemeli: %d %+v", w.Code, kr.Sinir)
	}
}

// Borsa saati bilgisayarınkinden farklıyken de gölge işlem doğru seçilir; oyuna dönülen mumun alımı yapılmaz
func TestKorumaGolgeBorsaFarki(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 1, Kayip: 50, GeriDonus: 1}, 0)
	eski := borsaFarki.Load()
	borsaFarki.Store(90_000) // borsa 1,5 dk ileride
	t.Cleanup(func() { borsaFarki.Store(eski) })
	islemYap(k, saat, -1)
	denetle(k)
	kenar := k.Koruma.KenarZamani
	f := func(x float64) *float64 { return &x }
	i := func(x int64) *int64 { return &x }
	borsaMum := func(yerel int64) int64 { return (yerel + 90_000) / 60_000 * 60_000 }
	once := borsaMum(kenar) - 60_000 // borsa saatiyle kenara almadan önce kapanan mum
	sonra := borsaMum(kenar) + 60_000
	saat.ileri(3 * time.Minute)
	kararIsle(k, &kararIstegi{Mum: sonra + 60_000, Asama: "bekle", MotorIslemleri: []motorIslemi{
		{G: once, GF: 1, C: i(once + 60_000), CF: f(1.02)}, // sayılmaz
	}})
	if !k.Koruma.Kenarda || len(k.Koruma.Golge) != 0 {
		t.Fatalf("kenardan önceki işlem gölge sayılmamalı: %+v", k.Koruma.Golge)
	}
	donus := sonra + 60_000
	kararIsle(k, &kararIstegi{Mum: donus, Asama: "bekle", MotorIslemleri: []motorIslemi{
		{G: once, GF: 1, C: i(once + 60_000), CF: f(1.02)},
		{G: sonra, GF: 1, C: i(sonra + 60_000), CF: f(1.01)},
	}})
	if k.Koruma.Kenarda || k.Koruma.DonusMum != donus {
		t.Fatalf("kenardan sonraki kârlı işlemle oyuna dönmeli: %+v", k.Koruma)
	}
	// aynı mumun alımı yapılmaz (yeni bir bildirim gibi gelse de)
	k.SonKararMum = donus - 60_000
	if e := kararIsle(k, &kararIstegi{Mum: donus, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}}); e.tur != "" {
		t.Fatalf("oyuna dönülen mumun alımı yapılmamalı: %+v", e)
	}
}

// Tavana ulaşan kumbara kendiliğinden dönmez; elle dönünce tavan bugünkü değerden yeniden sayılır
func TestKorumaTavandaKendiligindenDonmez(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 20, Kayip: 50, GeriDonus: 1}, 10)
	islemYap(k, saat, -6)
	islemYap(k, saat, -5) // toplam %10,7
	if !denetle(k) || !k.Koruma.TavanNotu {
		t.Fatalf("tavanda kenara alınmalı ve not yazılmalı: %+v", k.Koruma)
	}
	f := func(x float64) *float64 { return &x }
	i := func(x int64) *int64 { return &x }
	saat.ileri(2 * time.Minute)
	m := simdiF().UnixMilli() / 60_000 * 60_000
	saat.ileri(3 * time.Minute)
	kararIsle(k, &kararIstegi{Mum: m + 120_000, Asama: "bekle", MotorIslemleri: []motorIslemi{{G: m, GF: 1, C: i(m + 60_000), CF: f(1.05)}}})
	if !k.Koruma.Kenarda || len(k.Koruma.Golge) != 1 {
		t.Fatalf("kârlı gölge işleme rağmen tavanda kenarda kalmalı: %+v", k.Koruma)
	}
	aktifOturum.Store("test-oturum")
	t.Cleanup(func() { aktifOturum.Store("") })
	b, _ := json.Marshal(map[string]any{"oturum": "test-oturum", "id": k.ID})
	w := httptest.NewRecorder()
	kumbaraKorumaDonIstegi(w, httptest.NewRequest("POST", "/api/kumbara/koruma-don", bytes.NewReader(b)))
	if w.Code != 200 || k.Koruma.Kenarda || math.Abs(k.Koruma.TavanTepe-k.deger()) > 1e-9 {
		t.Fatalf("elle dönünce tavan yeniden sayılmalı: %d %+v", w.Code, k.Koruma)
	}
	islemYap(k, saat, -1)
	if denetle(k) {
		t.Fatal("elle döndükten sonra küçük bir zarar hemen kenara aldırmamalı")
	}
}

// Yeni ayarla devam eden kumbarada tavan aynı tepeden sayılır
func TestKorumaYeniAyardaTavanTasinir(t *testing.T) {
	k, saat := korumaliKumbara(t, korumaSinirlari{Seri: 20, Kayip: 50, GeriDonus: 2}, 10)
	islemYap(k, saat, 5)
	islemYap(k, saat, -6)
	denetle(k)
	tepe := k.Koruma.TavanTepe
	durumMu.Lock()
	k.Devam = &devamPlani{Dosya: k.Dosya, Icerik: []byte(`{"sembol":"AGTUSDT","aralik":"1m","strateji":{"para":{"masraf":0.1}}}`), Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().Add(time.Hour).UnixMilli()}
	k.Durum = "durduruluyor"
	kumbaraBitir(k, "yeni ayara geçildi")
	var y *kumbara
	for _, x := range D.Kumbaralar {
		if x.OncekiID == k.ID {
			y = x
		}
	}
	durumMu.Unlock()
	if y == nil || y.Koruma == nil || !y.Koruma.Gecici || math.Abs(y.Koruma.TavanTepe-tepe) > 1e-9 {
		t.Fatalf("yeni kumbara geçici korumalı ve tavanı taşınmış olmalı: %+v", y)
	}
	if k.Koruma.Gecmis != nil {
		t.Fatal("biten kumbarada geçmiş silinmeli")
	}
}

// Pencere açıkken koruma güncellendiyse eski değerler kaydedilmez
func TestKorumaAyarGorulenOtoDegistiyse(t *testing.T) {
	k, _ := korumaliKumbara(t, korumaSinirlari{Seri: 3, Kayip: 8, GeriDonus: 2}, 10)
	aktifOturum.Store("test-oturum")
	t.Cleanup(func() { aktifOturum.Store("") })
	b, _ := json.Marshal(map[string]any{"oturum": "test-oturum", "id": k.ID, "hassasiyet": "dengeli", "seri": 3, "kayip": 8, "geriDonus": 2,
		"gorulenOto": map[string]any{"seri": 99, "kayip": 8, "hizSayi": 0, "geriDonus": 2}})
	w := httptest.NewRecorder()
	kumbaraKorumaAyarIstegi(w, httptest.NewRequest("POST", "/api/kumbara/koruma-ayar", bytes.NewReader(b)))
	if w.Code != 409 {
		t.Fatalf("değişmiş otomatik değerlerde 409 bekleniyordu: %d %s", w.Code, w.Body.String())
	}
}
