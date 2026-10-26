package main

// 0.9 akış onayı: kumbaralara bağlantı (akis_onay.go). Akışın kendi hesabı akis_hesap_test.go'da; burada değerlendirme
// sahtedir (istenen sonucu verir), sınanan şey alımın onaya bağlanması, bekleme, gölge, sonuçlar ve karne.

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// 0.8'den kalan testler eski davranışı (onaysız) sınar; akış onayı testleri onayı kendileri açar.
	a := akisVarsayilan()
	a.Acik = false
	akisAyarDegeri.Store(a)
	akisHesapBaslat = func(*kumbara, int64) {} // testlerde arka planda ağa çıkılmasın
	os.Exit(m.Run())
}

type akisOrtam struct {
	t        *testing.T
	k        *kumbara
	saat     *testSaati
	acilis   int64
	mu       sync.Mutex
	sonuc    akisSonuc
	hata     error
	cagri    int
	mumlar   []int64 // sırayla verilecek "son kapanan mum" (boşsa sonuc.Mum)
	eylemler []eylem
	hesaplar []int64
}

func akisOnayKur(t *testing.T, acik bool, bekleme int) *akisOrtam {
	k, saat, _ := testKumbarasi(t)
	o := &akisOrtam{t: t, k: k, saat: saat, acilis: k.Baslangic / 60_000 * 60_000}
	eskiD, eskiH, eskiU, eskiA := akisDegerlendirF, akisHesapBaslat, akisUyku, akisAyarAl()
	a := akisVarsayilan()
	a.Acik, a.BeklemeMum = acik, bekleme
	akisAyarDegeri.Store(a)
	o.sonuc = akisSonuc{Mum: o.acilis, Fiyat: 1.004, Toplam: 35, Esik: 20, Onay: true, Ozet: "Onay: +35 (eşik +20) · deneme", Baglam: "dip",
		Bloklar: []akisBlok{{Ad: "Yapı", Var: true, Puan: 40}, {Ad: "Emir akışı", Var: true, Puan: 30}, {Ad: "CVD", Var: true, Puan: -12}, {Ad: "Açık pozisyon"}}}
	akisDegerlendirF = func(string, string) (*akisHesap, error) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.cagri++
		if o.hata != nil {
			return nil, o.hata
		}
		s := o.sonuc
		if len(o.mumlar) > 0 {
			s.Mum, o.mumlar = o.mumlar[0], o.mumlar[1:]
		}
		return &akisHesap{sonuc: s}, nil
	}
	akisHesapBaslat = func(_ *kumbara, m int64) { o.mu.Lock(); o.hesaplar = append(o.hesaplar, m); o.mu.Unlock() }
	akisUyku = func(time.Duration) {}
	eylemiYap = func(_ *kumbara, e eylem) {
		if e.tur != "" {
			o.mu.Lock()
			o.eylemler = append(o.eylemler, e)
			o.mu.Unlock()
		}
	}
	akisKarneSifirla()
	akisBeklemeSon = sync.Map{}
	t.Cleanup(func() {
		akisDegerlendirF, akisHesapBaslat, akisUyku = eskiD, eskiH, eskiU
		akisAyarDegeri.Store(eskiA)
		akisKarneSifirla()
	})
	return o
}

func akisKarneSifirla() {
	akisKarneMu.Lock()
	akisKarneListe, akisKarneAnahtar, akisKarneYuklendi = nil, nil, false
	akisKarneMu.Unlock()
}

func (o *akisOrtam) ayarla(f func(a *akisAyar)) {
	a := akisAyarAl()
	f(&a)
	akisAyarDegeri.Store(a)
}

func (o *akisOrtam) sonucAyarla(f func(s *akisSonuc)) {
	o.mu.Lock()
	f(&o.sonuc)
	o.mu.Unlock()
}

// motorun alım sinyali (mum 12:00–12:01, karar 12:01:02'de)
func (o *akisOrtam) alimSinyali() eylem {
	o.saat.ileri(62 * time.Second)
	return kararIsle(o.k, &kararIstegi{Mum: o.acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1.004}, Stop: 0.98, Hedef: 1.05, MotorGirisMum: o.acilis})
}

func (o *akisOrtam) kayit() *onayKaydi {
	o.t.Helper()
	durumMu.Lock()
	defer durumMu.Unlock()
	x := o.k.onayKaydiBul(o.acilis)
	if x == nil {
		o.t.Fatalf("kayıt yok")
	}
	c := *x
	return &c
}

func (o *akisOrtam) eylemSayisi() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.eylemler)
}

func olayVarMi(k *kumbara, parca string) bool {
	durumMu.Lock()
	defer durumMu.Unlock()
	for _, x := range k.Olaylar {
		if strings.Contains(x.Mesaj, parca) {
			return true
		}
	}
	return false
}

func TestAkisOnayVerinceAlinir(t *testing.T) {
	o := akisOnayKur(t, true, 2)
	if e := o.alimSinyali(); e.tur != "" {
		t.Fatalf("onay açıkken alım hemen yapılmamalı (onay beklenir): %+v", e)
	}
	if len(o.hesaplar) != 1 || o.hesaplar[0] != o.acilis {
		t.Fatalf("hesap başlatılmadı: %v", o.hesaplar)
	}
	if x := o.kayit(); x.Karar != "hesaplaniyor" || !x.Uygulandi || x.Fiyat != 1.004 || x.Masraf != 0.1 {
		t.Fatalf("kayıt: %+v", x)
	}
	akisOnayHesapla(o.k, o.acilis)
	if o.eylemSayisi() != 1 || o.eylemler[0].tur != "al" || o.eylemler[0].beklenen != 1.004 || o.eylemler[0].motorMum != o.acilis {
		t.Fatalf("onaydan sonra alım bekleniyordu: %+v", o.eylemler)
	}
	x := o.kayit()
	if x.Karar != "onay" || x.Toplam != 35 || x.Bloklar[0] == nil || *x.Bloklar[0] != 40 || x.Bloklar[3] != nil || x.Engel != "" {
		t.Fatalf("kayıt: %+v", x)
	}
	if !olayVarMi(o.k, "Akış onay verdi") {
		t.Fatalf("olay yazılmadı: %+v", o.k.Olaylar)
	}
	// aynı mum yeniden bildirilirse ikinci kayıt ve alım yok
	if e := kararIsle(o.k, &kararIstegi{Mum: o.acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1.004}}); e.tur != "" {
		t.Fatalf("aynı mum iki kez: %+v", e)
	}
	if n := len(o.k.Akis.Kayitlar); n != 1 || len(o.hesaplar) != 1 {
		t.Fatalf("tek kayıt olmalı: %d %v", n, o.hesaplar)
	}
}

// onaydan sonra alım gerçekten emir yürütücüden geçer (sanal borsa, eskisiyle aynı yol)
func TestAkisOnayliAlimEmirYurutucudenGecer(t *testing.T) {
	o := akisOnayKur(t, true, 0)
	eylemiYap = func(k *kumbara, e eylem) { // gerçek eylem: arka planda kumbaraAl
		if e.tur == "al" {
			kumbaraAl(k, e.beklenen, e.motorMum)
		}
	}
	o.alimSinyali()
	akisOnayHesapla(o.k, o.acilis)
	durumMu.Lock()
	t2 := o.k.acikIslem()
	durumMu.Unlock()
	if t2 == nil || t2.MotorMum != o.acilis || t2.BeklenenAlis != 1.004 || t2.Alis == nil || t2.Alis.Miktar <= 0 {
		t.Fatalf("onaylı alım yapılmadı: %+v", t2)
	}
	// motor çıkınca satış eskisi gibi
	o.saat.ileri(60 * time.Second)
	if e := kararIsle(o.k, &kararIstegi{Mum: o.acilis + 60_000, Asama: "bekle", Olay: &kararOlayi{Tip: "sat", Fiyat: 1.01, Kural: "hedefKar"}}); e.tur != "sat" {
		t.Fatalf("motor sattı, kumbara da satmalı: %+v", e)
	}
}

func TestAkisRedBeklemesiz(t *testing.T) {
	o := akisOnayKur(t, true, 0)
	o.sonucAyarla(func(s *akisSonuc) { s.Onay, s.Toplam, s.Ozet = false, 5, "Red: +5 (eşik +20) · deneme" })
	o.alimSinyali()
	akisOnayHesapla(o.k, o.acilis)
	if o.eylemSayisi() != 0 {
		t.Fatalf("red: alım olmamalı: %+v", o.eylemler)
	}
	if x := o.kayit(); x.Karar != "red" || x.Toplam != 5 {
		t.Fatalf("kayıt: %+v", x)
	}
	if o.k.Akis.Bekleme != nil || !olayVarMi(o.k, "alım yapılmadı") {
		t.Fatalf("bekleme olmamalı, olay yazılmalı: %+v %+v", o.k.Akis.Bekleme, o.k.Olaylar)
	}
}

func TestAkisBekleyipOnay(t *testing.T) {
	o := akisOnayKur(t, true, 2)
	o.sonucAyarla(func(s *akisSonuc) { s.Onay, s.Toplam = false, 5 })
	o.alimSinyali()
	akisOnayHesapla(o.k, o.acilis)
	b := o.k.Akis.Bekleme
	if x := o.kayit(); x.Karar != "bekliyor" || b == nil || b.MotorMum != o.acilis || b.Bitis != o.acilis+60_000+2*60_000 {
		t.Fatalf("bekleme kurulmalıydı: %+v %+v", x, b)
	}
	// hâlâ red: bekler, son bakış yazılır
	o.saat.ileri(20 * time.Second)
	o.sonucAyarla(func(s *akisSonuc) { s.Toplam = 12 })
	akisBeklemeTuru()
	if x := o.kayit(); x.Karar != "bekliyor" || x.SonToplam != 12 || x.SonBakis == 0 || o.eylemSayisi() != 0 {
		t.Fatalf("beklemeye devam: %+v", x)
	}
	// onay geldi ama fiyat kaçtı (%0,3'ten fazla): alınmaz
	o.saat.ileri(20 * time.Second)
	o.sonucAyarla(func(s *akisSonuc) { s.Onay, s.Toplam, s.Fiyat = true, 30, 1.0075 })
	akisBeklemeTuru()
	if x := o.kayit(); x.Karar != "bekliyor" || o.eylemSayisi() != 0 {
		t.Fatalf("fiyat kaçmışken alınmamalı: %+v", x)
	}
	// fiyat uygun: alınır
	o.saat.ileri(20 * time.Second)
	o.sonucAyarla(func(s *akisSonuc) { s.Fiyat, s.Ozet = 1.006, "Onay: +30 (eşik +20) · geldi" })
	akisBeklemeTuru()
	x := o.kayit()
	if x.Karar != "bekleyip-onay" || x.OnayFiyat != 1.006 || x.OnayZ == 0 || x.OnayOzet == "" || o.k.Akis.Bekleme != nil {
		t.Fatalf("bekleyip onay: %+v", x)
	}
	if o.eylemSayisi() != 1 || o.eylemler[0].tur != "al" || o.eylemler[0].motorMum != o.acilis || o.eylemler[0].beklenen != 1.004 {
		t.Fatalf("alım: %+v", o.eylemler)
	}
	// sonuç: girişi onay fiyatından (süzgeçle), motorunki kendi fiyatından
	c, cf := o.acilis+5*60_000, 1.0241
	o.saat.ileri(5 * time.Minute)
	kararIsle(o.k, &kararIstegi{Mum: o.acilis + 5*60_000, Asama: "bekle", MotorIslemleri: []motorIslemi{{G: o.acilis, GF: 1.004, C: &c, CF: &cf, K: "hedefKar"}}})
	x = o.kayit()
	if x.MotorSonuc == nil || x.Sonuc == nil || !yakin(*x.MotorSonuc, (1.0241/1.004-1)*100-0.2, 1e-3) || !yakin(*x.Sonuc, (1.0241/1.006-1)*100-0.2, 1e-3) {
		t.Fatalf("sonuçlar: %+v", x)
	}
}

func TestAkisBeklemeBiter(t *testing.T) {
	durumlar := []struct {
		ad    string
		once  func(o *akisOrtam)
		karar string
	}{
		{"süre doldu", func(o *akisOrtam) { o.saat.ileri(3 * time.Minute) }, "sure-doldu"},
		{"motor çıktı", func(o *akisOrtam) {
			o.saat.ileri(60 * time.Second)
			kararIsle(o.k, &kararIstegi{Mum: o.acilis + 60_000, Asama: "bekle"})
		}, "motor-cikti"},
		{"zarar durdur", func(o *akisOrtam) { durumMu.Lock(); o.k.SonFiyat = 0.979; durumMu.Unlock() }, "zarar-durdur"},
		{"hedef", func(o *akisOrtam) { durumMu.Lock(); o.k.SonFiyat = 1.06; durumMu.Unlock() }, "fiyat-kacti"},
		{"onay kapatıldı", func(o *akisOrtam) { o.ayarla(func(a *akisAyar) { a.Acik = false }) }, "iptal"},
		{"kumbara durduruldu", func(o *akisOrtam) { durumMu.Lock(); o.k.Durum = "durduruluyor"; durumMu.Unlock() }, "iptal"},
	}
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			o := akisOnayKur(t, true, 2)
			o.sonucAyarla(func(s *akisSonuc) { s.Onay, s.Toplam = false, 5 })
			o.alimSinyali()
			akisOnayHesapla(o.k, o.acilis)
			if o.kayit().Karar != "bekliyor" {
				t.Fatalf("bekleme yok")
			}
			o.sonucAyarla(func(s *akisSonuc) { s.Onay, s.Toplam = true, 50 }) // onay gelse bile artık alınmamalı
			d.once(o)
			akisBeklemeTuru()
			if x := o.kayit(); x.Karar != d.karar || x.Not == "" {
				t.Fatalf("karar %q bekleniyordu: %+v", d.karar, x)
			}
			if o.k.Akis.Bekleme != nil || o.eylemSayisi() != 0 {
				t.Fatalf("bekleme bitmeli, alım olmamalı: %+v %+v", o.k.Akis.Bekleme, o.eylemler)
			}
			if !olayVarMi(o.k, "beklemesi bitti") {
				t.Fatalf("olay yazılmadı")
			}
		})
	}
}

func TestAkisGolgeKayitlari(t *testing.T) {
	t.Run("genel ayar kapalı", func(t *testing.T) {
		o := akisOnayKur(t, false, 2)
		e := o.alimSinyali()
		if e.tur != "al" || e.beklenen != 1.004 {
			t.Fatalf("onay kapalıyken eskisi gibi hemen alınmalı: %+v", e)
		}
		if x := o.kayit(); x.Uygulandi || x.Golge != "onay kapalı" {
			t.Fatalf("gölge kaydı: %+v", x)
		}
		o.sonucAyarla(func(s *akisSonuc) { s.Onay = false })
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "golge-red" || o.eylemSayisi() != 0 || o.k.Akis.Bekleme != nil {
			t.Fatalf("gölge red: %+v", x)
		}
	})
	t.Run("bu kumbarada kapalı", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.k.Akis = &akisKumbara{Kapali: true}
		if e := o.alimSinyali(); e.tur != "al" {
			t.Fatalf("kumbarada kapalıyken hemen alınmalı: %+v", e)
		}
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "golge-onay" || x.Uygulandi || o.eylemSayisi() != 0 {
			t.Fatalf("gölge onay: %+v", x)
		}
	})
	t.Run("korumada kenarda", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.k.Koruma = &koruma{Kenarda: true}
		if e := o.alimSinyali(); e.tur != "" {
			t.Fatalf("kenardayken alım yok: %+v", e)
		}
		if x := o.kayit(); x.Uygulandi || x.Golge != "kenarda" {
			t.Fatalf("kenarda gölge kaydı: %+v", x)
		}
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "golge-onay" || o.eylemSayisi() != 0 {
			t.Fatalf("kenarda: alım olmamalı: %+v %+v", x, o.eylemler)
		}
	})
	t.Run("geç gelen sinyal kaydedilmez", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.saat.ileri(10 * time.Minute)
		if e := kararIsle(o.k, &kararIstegi{Mum: o.acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1.004}}); e.tur != "" {
			t.Fatalf("geç karar: %+v", e)
		}
		if o.k.Akis != nil && len(o.k.Akis.Kayitlar) > 0 {
			t.Fatalf("geç karar kaydedilmemeli: %+v", o.k.Akis)
		}
	})
}

func TestAkisHesapHatasiVeIptal(t *testing.T) {
	t.Run("hata, beklemesiz", func(t *testing.T) {
		o := akisOnayKur(t, true, 0)
		o.hata = errors.New("MEXC cevap vermedi")
		o.alimSinyali()
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "hata" || !strings.Contains(x.Ozet, "MEXC cevap vermedi") || o.eylemSayisi() != 0 {
			t.Fatalf("hata: %+v", x)
		}
		if o.cagri != 8 {
			t.Fatalf("hata olunca birkaç kez denenmeli: %d", o.cagri)
		}
	})
	t.Run("hata, beklemeli: beklerken yeniden bakılır", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.hata = errors.New("geçici")
		o.alimSinyali()
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "bekliyor" {
			t.Fatalf("beklemeye geçmeli: %+v", x)
		}
		o.mu.Lock()
		o.hata = nil
		o.mu.Unlock()
		o.saat.ileri(5 * time.Second)
		akisBeklemeTuru()
		if x := o.kayit(); x.Karar != "bekleyip-onay" || o.eylemSayisi() != 1 {
			t.Fatalf("hata geçince onay: %+v", x)
		}
	})
	t.Run("motorun mumu gecikirse beklenir", func(t *testing.T) {
		o := akisOnayKur(t, true, 0)
		o.mumlar = []int64{o.acilis - 60_000, o.acilis - 60_000, o.acilis}
		o.alimSinyali()
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "onay" || o.cagri != 3 {
			t.Fatalf("doğru mumla hesap: %+v çağrı %d", x, o.cagri)
		}
	})
	t.Run("hesap sürerken kumbara durduruldu", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.alimSinyali()
		durumMu.Lock()
		o.k.Durum = "durduruluyor"
		durumMu.Unlock()
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "iptal" || o.eylemSayisi() != 0 || o.k.Akis.Bekleme != nil {
			t.Fatalf("iptal: %+v", x)
		}
	})
	t.Run("hesap sürerken emir başladı", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.alimSinyali()
		durumMu.Lock()
		o.k.Emirde = true
		durumMu.Unlock()
		akisOnayHesapla(o.k, o.acilis)
		if x := o.kayit(); x.Karar != "iptal" || o.eylemSayisi() != 0 {
			t.Fatalf("iptal: %+v", x)
		}
	})
	t.Run("kumbara beklerken kapandı, devamı onayı kapalı", func(t *testing.T) {
		o := akisOnayKur(t, true, 2)
		o.k.Akis = &akisKumbara{Kapali: false}
		o.sonucAyarla(func(s *akisSonuc) { s.Onay = false })
		o.alimSinyali()
		akisOnayHesapla(o.k, o.acilis)
		durumMu.Lock()
		kumbaraBitir(o.k, "durduruldu")
		durumMu.Unlock()
		if x := o.kayit(); x.Karar != "iptal" || x.Not != "Kumbara kapandı" || o.k.Akis.Bekleme != nil {
			t.Fatalf("kapanınca bekleme biter: %+v", x)
		}
	})
}

func TestAkisDevamKapaliKalir(t *testing.T) {
	o := akisOnayKur(t, true, 2)
	durumMu.Lock()
	o.k.Akis = &akisKumbara{Kapali: true}
	o.k.Devam = &devamPlani{Dosya: o.k.Dosya, Icerik: []byte(`{"sembol":"AGTUSDT","aralik":"1m","strateji":{"para":{"masraf":0.1}}}`), Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().Add(time.Hour).UnixMilli()}
	kumbaraBitir(o.k, "yeni ayara geçildi")
	var y *kumbara
	for _, x := range D.Kumbaralar {
		if x.OncekiID == o.k.ID {
			y = x
		}
	}
	durumMu.Unlock()
	if y == nil || y.Akis == nil || !y.Akis.Kapali {
		t.Fatalf("yeni ayarla devam eden kumbarada da onay kapalı kalmalı: %+v", y)
	}
}

func TestAkisKarne(t *testing.T) {
	o := akisOnayKur(t, true, 0)
	// 1) onaylanan, kârlı
	o.alimSinyali()
	akisOnayHesapla(o.k, o.acilis)
	c1, cf1 := o.acilis+3*60_000, 1.0141
	o.saat.ileri(3 * time.Minute)
	kararIsle(o.k, &kararIstegi{Mum: o.acilis + 3*60_000, Asama: "bekle", MotorIslemleri: []motorIslemi{{G: o.acilis, GF: 1.004, C: &c1, CF: &cf1, K: "hedefKar"}}})
	x := o.kayit()
	if x.Sonuc == nil || !yakin(*x.Sonuc, (1.0141/1.004-1)*100-0.2, 1e-3) || x.Cikis != 1.0141 || x.CikisMum != c1 {
		t.Fatalf("sonuç: %+v", x)
	}
	// 2) reddedilen, zararlı (puan −30)
	durumMu.Lock()
	o.k.Islemler = nil // ilk alım yapılmadı (sahte eylem): kumbara boş
	durumMu.Unlock()
	m2 := o.acilis + 11*60_000 // 12:11–12:12 mumu, karar 12:12:02'de
	o.saat.ileri(8 * time.Minute)
	o.sonucAyarla(func(s *akisSonuc) { s.Mum, s.Onay, s.Toplam = m2, false, -30 })
	kararIsle(o.k, &kararIstegi{Mum: m2, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 2}, MotorGirisMum: m2})
	akisOnayHesapla(o.k, m2)
	// 3) engelli (veri eksik) reddedilen, kârlı
	m3 := o.acilis + 21*60_000
	o.saat.ileri(10 * time.Minute)
	o.sonucAyarla(func(s *akisSonuc) { s.Mum, s.Onay, s.Toplam, s.Eksik = m3, false, 50, "Emir akışı verisi yok" })
	kararIsle(o.k, &kararIstegi{Mum: m3, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 3}, MotorGirisMum: m3})
	akisOnayHesapla(o.k, m3)
	c2, cf2, c3, cf3 := m2+60_000, 1.9, m3+60_000, 3.3
	o.saat.ileri(2 * time.Minute)
	kararIsle(o.k, &kararIstegi{Mum: m3 + 60_000, Asama: "bekle", MotorIslemleri: []motorIslemi{
		{G: o.acilis, GF: 1.004, C: &c1, CF: &cf1}, {G: m2, GF: 2, C: &c2, CF: &cf2}, {G: m3, GF: 3, C: &c3, CF: &cf3}}})
	kr := akisKarneHesapla("", false, 100)
	if kr.Hepsi.N != 3 || kr.Onaylanan.N != 1 || kr.Onaylanan.Kazanan != 1 || kr.Reddedilen.N != 2 || kr.Reddedilen.Kazanan != 1 || kr.Bekleyen != 0 {
		t.Fatalf("karne: %+v", kr)
	}
	if !yakin(kr.Reddedilen.Top, (1.9/2-1)*100-0.2+(3.3/3-1)*100-0.2, 1e-3) {
		t.Fatalf("reddedilen toplam: %+v", kr.Reddedilen)
	}
	esik := func(e float64) esikSatiri {
		for _, x := range kr.Esikler {
			if x.Esik == e {
				return x
			}
		}
		t.Fatalf("eşik yok: %v", e)
		return esikSatiri{}
	}
	// eşik −40: engelsiz iki sinyal (+35 ve −30); eşik 20: sadece +35; engelli olan (veri eksik) hiçbir eşikte sayılmaz
	if a, b, c := esik(-40), esik(20), esik(60); a.N != 2 || b.N != 1 || b.Kazanan != 1 || c.N != 0 {
		t.Fatalf("eşik taraması: %+v %+v %+v", a, b, c)
	}
	// blok: Yapı +40 → üç kayıtta da ≥ +10
	if kr.Bloklar[0][0].N != 3 || kr.Bloklar[2][1].N != 3 || kr.Bloklar[3][0].N+kr.Bloklar[3][1].N != 0 {
		t.Fatalf("bloklar: %+v", kr.Bloklar)
	}
	if !strings.Contains(kr.Hukum, "az örnek") || len(kr.Kayitlar) != 3 {
		t.Fatalf("hüküm/kayıtlar: %q %d", kr.Hukum, len(kr.Kayitlar))
	}
	// dosyada kalır: kumbaralar silinse de karne aynı
	b, err := os.ReadFile(filepath.Join(veriKlasoru, "kayitlar", "akis_karne.jsonl"))
	if err != nil || strings.Count(string(b), "\n") != 3 {
		t.Fatalf("karne dosyası: %v %q", err, b)
	}
	durumMu.Lock()
	D = yeniDurum()
	durumMu.Unlock()
	akisKarneSifirla()
	if kr2 := akisKarneHesapla("", false, 100); kr2.Hepsi.N != 3 || kr2.Onaylanan.N != 1 || kr2.Reddedilen.N != 2 {
		t.Fatalf("dosyadan karne: %+v", kr2)
	}
	// aynı sonuç ikinci kez yazılmaz
	akisKarneEkle(karneKaydi{Kumbara: o.k.ID, Kayit: onayKaydi{MotorMum: o.acilis, Karar: "onay", Sonuc: x.Sonuc, MotorSonuc: x.MotorSonuc}})
	if b2, _ := os.ReadFile(filepath.Join(veriKlasoru, "kayitlar", "akis_karne.jsonl")); strings.Count(string(b2), "\n") != 3 {
		t.Fatalf("tekrar yazıldı")
	}
	// "bu ayarlarla": ayar değişince eski kayıtlar süzülür
	o.ayarla(func(a *akisAyar) { a.Degisim = 12345 })
	if kr3 := akisKarneHesapla("", true, 100); kr3.Hepsi.N != 0 {
		t.Fatalf("bu ayarlarla: %+v", kr3.Hepsi)
	}
}

func TestAkisKumbaraOzeti(t *testing.T) {
	o := akisOnayKur(t, true, 0)
	durumMu.Lock()
	o.k.Akis = &akisKumbara{}
	for i := 0; i < 25; i++ {
		s := float64(i%3 - 1)
		o.k.Akis.Kayitlar = append(o.k.Akis.Kayitlar, onayKaydi{MotorMum: int64(i) * 60_000, Karar: []string{"onay", "red"}[i%2], Sonuc: &s, MotorSonuc: &s})
	}
	m := kumbaraOzeti(o.k, 10)
	durumMu.Unlock()
	b, _ := json.Marshal(m)
	var geri map[string]any
	_ = json.Unmarshal(b, &geri)
	ak, ok := geri["akis"].(map[string]any)
	if !ok {
		t.Fatalf("özette akış yok: %s", b)
	}
	if kayit, _ := ak["kayitlar"].([]any); len(kayit) != 10 || ak["toplamKayit"].(float64) != 25 || ak["uygulaniyor"] != true {
		t.Fatalf("akış özeti: %v", ak)
	}
	if strings.Count(string(b), `"motorMum"`) > 10 {
		t.Fatalf("bütün kayıtlar gönderilmemeli")
	}
}

func akisIstek(t *testing.T, h http.HandlerFunc, yontem, yol string, govde any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var r *http.Request
	if govde != nil {
		b, _ := json.Marshal(govde)
		r = httptest.NewRequest(yontem, yol, strings.NewReader(string(b)))
	} else {
		r = httptest.NewRequest(yontem, yol, nil)
	}
	w := httptest.NewRecorder()
	h(w, r)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func TestAkisAyarUcNoktasi(t *testing.T) {
	o := akisOnayKur(t, true, 2)
	aktifOturum.Store("oturum-1")
	t.Cleanup(func() { aktifOturum.Store("") })
	if w, m := akisIstek(t, akisAyarIstegi, "GET", "/api/akis/ayar", nil); w.Code != 200 || m["ayar"] == nil || m["varsayilan"] == nil {
		t.Fatalf("GET: %d %v", w.Code, m)
	}
	a := akisAyarAl()
	a.Esik = 33
	if w, _ := akisIstek(t, akisAyarIstegi, "POST", "/api/akis/ayar", map[string]any{"oturum": "eski", "ayar": a}); w.Code != 409 {
		t.Fatalf("eski pencere reddedilmeli: %d", w.Code)
	}
	if w, _ := akisIstek(t, akisAyarIstegi, "POST", "/api/akis/ayar", map[string]any{"oturum": "oturum-1", "ayar": a}); w.Code != 200 {
		t.Fatalf("POST: %d", w.Code)
	}
	y := akisAyarAl()
	if y.Esik != 33 || y.Degisim == 0 {
		t.Fatalf("eşik değişip değişim anı yazılmalı: %+v", y)
	}
	// sadece açık/kapalı değişince "ayar değişti" sayılmaz
	o.saat.ileri(time.Minute)
	y2 := y
	y2.Acik = false
	akisIstek(t, akisAyarIstegi, "POST", "/api/akis/ayar", map[string]any{"oturum": "oturum-1", "ayar": y2})
	if z := akisAyarAl(); z.Acik || z.Degisim != y.Degisim {
		t.Fatalf("açık/kapalı: %+v", z)
	}
	// sınır dışı değer düzeltilir
	y3 := akisAyarAl()
	y3.Esik, y3.BeklemeMum = 500, 99
	akisIstek(t, akisAyarIstegi, "POST", "/api/akis/ayar", map[string]any{"oturum": "oturum-1", "ayar": y3})
	if z := akisAyarAl(); z.Esik != 100 || z.BeklemeMum != 10 {
		t.Fatalf("düzeltme: %+v", z)
	}
	// varsayılana dön (açık/kapalı korunur)
	akisIstek(t, akisAyarIstegi, "POST", "/api/akis/ayar", map[string]any{"oturum": "oturum-1", "varsayilan": true})
	if z := akisAyarAl(); z.Esik != akisVarsayilan().Esik || z.Acik {
		t.Fatalf("varsayılan: %+v", z)
	}
	// ayarlar.json'a yazıldı; genel ayar ucu "akis"i değiştiremez
	b, _ := os.ReadFile(ayarYolu())
	if !strings.Contains(string(b), `"akis"`) {
		t.Fatalf("ayarlar.json: %s", b)
	}
	akisIstek(t, ayarlarIstegi, "POST", "/api/ayarlar", map[string]any{"akis": map[string]any{"esik": -100, "acik": true}})
	ayarMu.Lock()
	m := ayarlariOku()
	ayarMu.Unlock()
	if akisAyarOku(m).Esik != akisVarsayilan().Esik {
		t.Fatalf("genel ayar ucu akış ayarını değiştirmemeli")
	}
}

func TestAkisKumbaraUcNoktasi(t *testing.T) {
	o := akisOnayKur(t, true, 2)
	aktifOturum.Store("oturum-1")
	t.Cleanup(func() { aktifOturum.Store("") })
	o.sonucAyarla(func(s *akisSonuc) { s.Onay = false })
	o.alimSinyali()
	akisOnayHesapla(o.k, o.acilis)
	if o.k.Akis.Bekleme == nil {
		t.Fatalf("bekleme yok")
	}
	if w, _ := akisIstek(t, kumbaraAkisIstegi, "POST", "/api/kumbara/akis", map[string]any{"oturum": "x", "id": o.k.ID, "kapali": true}); w.Code != 409 {
		t.Fatalf("eski pencere: %d", w.Code)
	}
	if w, _ := akisIstek(t, kumbaraAkisIstegi, "POST", "/api/kumbara/akis", map[string]any{"oturum": "oturum-1", "id": o.k.ID, "kapali": true}); w.Code != 200 {
		t.Fatalf("kapat: %d", w.Code)
	}
	if !o.k.Akis.Kapali || o.k.Akis.Bekleme != nil || o.kayit().Karar != "iptal" || o.k.akisUygulaniyor() {
		t.Fatalf("kapatınca bekleme biter: %+v", o.k.Akis)
	}
	if w, _ := akisIstek(t, kumbaraAkisIstegi, "POST", "/api/kumbara/akis", map[string]any{"oturum": "oturum-1", "id": "yok", "kapali": false}); w.Code != 404 {
		t.Fatalf("olmayan kumbara: %d", w.Code)
	}
	akisIstek(t, kumbaraAkisIstegi, "POST", "/api/kumbara/akis", map[string]any{"oturum": "oturum-1", "id": o.k.ID, "kapali": false})
	if o.k.Akis.Kapali || !o.k.akisUygulaniyor() {
		t.Fatalf("yeniden açılmalı")
	}
	if w, m := akisIstek(t, akisKarneIstegi, "GET", "/api/akis/karne?kumbara="+o.k.ID, nil); w.Code != 200 || m["hepsi"] == nil || m["esikler"] == nil {
		t.Fatalf("karne ucu: %d %v", w.Code, m)
	}
}

// durum.json'a yazılıp okunur (program yeniden açılınca bekleyen onay ve kayıtlar kaybolmaz)
func TestAkisDurumDosyasi(t *testing.T) {
	o := akisOnayKur(t, true, 2)
	o.sonucAyarla(func(s *akisSonuc) { s.Onay = false })
	o.alimSinyali()
	akisOnayHesapla(o.k, o.acilis)
	durumMu.Lock()
	durumKaydet()
	D = yeniDurum()
	durumYukleIc()
	k := kumbaraBul(o.k.ID)
	durumMu.Unlock()
	if k == nil || k.Akis == nil || k.Akis.Bekleme == nil || len(k.Akis.Kayitlar) != 1 || k.Akis.Kayitlar[0].Karar != "bekliyor" {
		t.Fatalf("durum.json'dan akış: %+v", k)
	}
}
