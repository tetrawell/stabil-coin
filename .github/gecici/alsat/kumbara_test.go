package main

import (
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------- test saati: uyu beklemez, saati ileri alır ----------

type testSaati struct {
	mu sync.Mutex
	t  time.Time
}

func saatKur(t *testing.T) *testSaati {
	s := &testSaati{t: time.Date(2026, 10, 18, 12, 0, 0, 0, time.Local)}
	eskiUyu, eskiSimdi := uyu, simdiF
	uyu = func(d time.Duration) { s.mu.Lock(); s.t = s.t.Add(d); s.mu.Unlock() }
	simdiF = func() time.Time { s.mu.Lock(); defer s.mu.Unlock(); return s.t }
	t.Cleanup(func() { uyu, simdiF = eskiUyu, eskiSimdi })
	return s
}

func (s *testSaati) ileri(d time.Duration) { s.mu.Lock(); s.t = s.t.Add(d); s.mu.Unlock() }

// ---------- test piyasası ----------

type testPiyasa struct {
	mu        sync.Mutex
	du        defterUstu
	islemler  []islemKaydi
	defter    defter
	fiyat     float64
	kural     sembolKurali
	islemFn   func(simdi int64) []islemKaydi // her sorguda yeni işlem üretmek için
	fiyatHata error
}

func yeniTestPiyasa() *testPiyasa {
	return &testPiyasa{
		du:    defterUstu{Bid: 1.000, BidMiktar: 500, Ask: 1.010, AskMiktar: 500},
		fiyat: 1.005,
		kural: sembolKurali{Sembol: "AGTUSDT", Varlik: "AGT", Acik: true, FiyatOndalik: 3, MiktarOndalik: 2, TutarOndalik: 2,
			EnAzMiktar: 0.01, EnAzTutar: 1, EnAzTutarPiyasa: 1, Maker: 0, Taker: 0.0005, Turler: []string{"LIMIT", "MARKET"}},
		defter: defter{
			Satis: []defterSeviye{{1.010, 50}, {1.020, 100}, {1.030, 1000}},
			Alis:  []defterSeviye{{1.000, 50}, {0.990, 100}, {0.980, 1000}},
		},
	}
}

func (p *testPiyasa) EnIyiFiyat(string) (defterUstu, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.du, nil
}
func (p *testPiyasa) SonIslemler(string, int) ([]islemKaydi, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.islemFn != nil {
		p.islemler = append(p.islemler, p.islemFn(simdiF().UnixMilli())...)
	}
	return append([]islemKaydi(nil), p.islemler...), nil
}
func (p *testPiyasa) Derinlik(string, int) (defter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.defter, nil
}
func (p *testPiyasa) SonFiyat(string) (float64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fiyat, p.fiyatHata
}
func (p *testPiyasa) Kurallar(string) (sembolKurali, error) { return p.kural, nil }

func sessizOlay(string, string) {}

func yakin(a, b, tol float64) bool { return math.Abs(a-b) <= tol }

// ---------- sanal borsa + emir yürütücü ----------

func TestLimitAlisDolar(t *testing.T) {
	saat := saatKur(t)
	p := yeniTestPiyasa()
	sb := yeniSanalBorsa(p)
	// emir verildikten 3 sn sonra biri piyasadan satıyor (alış emirlerine çarpıyor), fiyat bizim fiyatımızda
	verildi := saat.t.UnixMilli()
	p.islemFn = func(simdi int64) []islemKaydi {
		if simdi >= verildi+3000 && simdi < verildi+4000 {
			return []islemKaydi{{Fiyat: 1.001, Miktar: 200, Zaman: simdi, AliciMaker: true}}
		}
		return nil
	}
	d, err := limitSonraPiyasa(sb, p, p.kural, "BUY", 100, 0, "t1", sessizOlay)
	if err != nil {
		t.Fatal(err)
	}
	if d.Yol != "limit" || !yakin(d.Ortalama, 1.001, 1e-9) || !yakin(d.Miktar, 99.90, 1e-9) {
		t.Fatalf("beklenmeyen dolum: %+v", d)
	}
	if d.LimitFiyat != 1.001 {
		t.Fatalf("limit fiyatı alıcı sırasının bir adım içi olmalıydı: %v", d.LimitFiyat)
	}
	if !yakin(d.NetTutar, 99.90*1.001, 1e-9) || d.Komisyon != 0 {
		t.Fatalf("tutar/komisyon: %+v", d)
	}
}

func TestLimitDolmazsaPiyasa(t *testing.T) {
	saatKur(t)
	p := yeniTestPiyasa()
	sb := yeniSanalBorsa(p)
	d, err := limitSonraPiyasa(sb, p, p.kural, "BUY", 100, 0, "t2", sessizOlay)
	if err != nil {
		t.Fatal(err)
	}
	if d.Yol != "piyasa" {
		t.Fatalf("15 sn sonra piyasa emri bekleniyordu: %+v", d)
	}
	// 50 coin × 1,010 = 50,5 USDT, kalan 49,5 USDT 1,020'den
	adet := 50 + 49.5/1.020
	if !yakin(d.Miktar, adet, 1e-9) || !yakin(d.Tutar, 100, 1e-9) {
		t.Fatalf("defter yürüyüşü yanlış: %+v", d)
	}
	if !yakin(d.NetMiktar, adet*(1-0.0005), 1e-9) {
		t.Fatalf("alışta komisyon coinden düşülmeli: %+v", d)
	}
	if d.Bitis-d.Baslangic < 15000 {
		t.Fatalf("en az 15 sn beklenmeliydi: %d ms", d.Bitis-d.Baslangic)
	}
}

func TestYarimDolumKalaniPiyasa(t *testing.T) {
	saat := saatKur(t)
	p := yeniTestPiyasa()
	sb := yeniSanalBorsa(p)
	verildi := saat.t.UnixMilli()
	p.islemFn = func(simdi int64) []islemKaydi {
		if simdi >= verildi+2000 && simdi < verildi+3000 {
			return []islemKaydi{{Fiyat: 1.001, Miktar: 40, Zaman: simdi, AliciMaker: true}}
		}
		return nil
	}
	d, err := limitSonraPiyasa(sb, p, p.kural, "BUY", 100, 0, "t3", sessizOlay)
	if err != nil {
		t.Fatal(err)
	}
	if d.Yol != "limit+piyasa" {
		t.Fatalf("yol: %+v", d)
	}
	limitTutar := 40 * 1.001
	if !yakin(d.Tutar, 100, 0.011) || d.Tutar > 100+1e-9 {
		t.Fatalf("toplam harcama bütçeyi aşmamalı: %v", d.Tutar)
	}
	var lim, pys float64
	for _, x := range d.Parcalar {
		if x.Kaynak == "limit" {
			lim += x.Tutar
		} else {
			pys += x.Tutar
		}
	}
	if !yakin(lim, limitTutar, 1e-9) || !yakin(pys, asagiYuvarla(100-limitTutar, 2), 1e-9) {
		t.Fatalf("parçalar: limit %v piyasa %v", lim, pys)
	}
}

func TestSiraOnundeyse(t *testing.T) {
	saat := saatKur(t)
	p := yeniTestPiyasa()
	p.du = defterUstu{Bid: 1.000, BidMiktar: 500, Ask: 1.001, AskMiktar: 500} // aralık 1 adım: sıranın sonuna ekleniriz
	sb := yeniSanalBorsa(p)
	verildi := saat.t.UnixMilli()
	p.islemFn = func(simdi int64) []islemKaydi {
		if simdi >= verildi+1000 && simdi < verildi+9000 {
			return []islemKaydi{{Fiyat: 1.000, Miktar: 100, Zaman: simdi, AliciMaker: true}} // saniyede 100 coin
		}
		return nil
	}
	d, err := limitSonraPiyasa(sb, p, p.kural, "BUY", 100, 0, "t4", sessizOlay)
	if err != nil {
		t.Fatal(err)
	}
	// önümüzde 500 coin vardı; 8 sn × 100 = 800 coin işlem: 500 önümüzdekine, 300 bize (100 coinlik emir tam dolar)
	if d.Yol != "limit" || !yakin(d.Ortalama, 1.000, 1e-9) {
		t.Fatalf("sıra hesabı: %+v", d)
	}
}

func TestSatisLimitVePiyasa(t *testing.T) {
	saatKur(t)
	p := yeniTestPiyasa()
	sb := yeniSanalBorsa(p)
	d, err := limitSonraPiyasa(sb, p, p.kural, "SELL", 0, 120, "t5", sessizOlay)
	if err != nil {
		t.Fatal(err)
	}
	if d.Yol != "piyasa" || !yakin(d.Miktar, 120, 1e-9) {
		t.Fatalf("satış: %+v", d)
	}
	gelir := 50*1.000 + 70*0.990
	if !yakin(d.Tutar, gelir, 1e-9) || !yakin(d.NetTutar, gelir*(1-0.0005), 1e-9) {
		t.Fatalf("satışta komisyon USDT'den düşülmeli: %+v", d)
	}
}

// İptal edilip edilmediği öğrenilemeyen limit emirden sonra piyasa emri VERİLMEMELİ
type bozukBorsa struct {
	piyasaEmriVerildi bool
}

func (b *bozukBorsa) Ad() string { return "gercek" }
func (b *bozukBorsa) LimitEmir(sembolKurali, string, float64, float64, string) (string, error) {
	return "1", nil
}
func (b *bozukBorsa) EmirDurumu(sembolKurali, string) (emirDurumu, error) {
	return emirDurumu{}, errors.New("bağlantı yok")
}
func (b *bozukBorsa) Iptal(sembolKurali, string) (emirDurumu, error) {
	return emirDurumu{}, errors.New("bağlantı yok")
}
func (b *bozukBorsa) PiyasaEmri(sembolKurali, string, float64, float64, string) (emirDurumu, error) {
	b.piyasaEmriVerildi = true
	return emirDurumu{}, errors.New("olmamalı")
}
func (b *bozukBorsa) Dolumlar(sembolKurali, string) ([]parca, error) { return nil, errors.New("yok") }

func TestIptalBelirsizseDur(t *testing.T) {
	saatKur(t)
	p := yeniTestPiyasa()
	b := &bozukBorsa{}
	_, err := limitSonraPiyasa(b, p, p.kural, "BUY", 100, 0, "t6", sessizOlay)
	var bh belirsizHata
	if !errors.As(err, &bh) {
		t.Fatalf("belirsiz hata bekleniyordu: %v", err)
	}
	if b.piyasaEmriVerildi {
		t.Fatal("iptal kesinleşmeden piyasa emri verildi")
	}
}

func TestDolumToplaMXKomisyon(t *testing.T) {
	k := sembolKurali{Varlik: "AGT"}
	d := dolumTopla("BUY", k, []parca{{Fiyat: 1, Miktar: 100, Tutar: 100, Komisyon: 0.02, KomisyonVarligi: "MX", KomisyonUSDT: 0.04}})
	if d.NetMiktar != 100 || !yakin(d.NetTutar, 100.04, 1e-12) || !yakin(d.Komisyon, 0.04, 1e-12) {
		t.Fatalf("MX komisyonu kumbaradan düşülmeli: %+v", d)
	}
	s := dolumTopla("SELL", k, []parca{{Fiyat: 1, Miktar: 100, Tutar: 100, Komisyon: 0.05, KomisyonVarligi: "USDT", KomisyonUSDT: 0.05}})
	if !yakin(s.NetTutar, 99.95, 1e-12) {
		t.Fatalf("satış: %+v", s)
	}
}

// ---------- kumbara: karar eşleştirme ----------

func testKumbarasi(t *testing.T) (*kumbara, *testSaati, *testPiyasa) {
	saat := saatKur(t)
	veriKlasoru = t.TempDir()
	D = yeniDurum()
	p := yeniTestPiyasa()
	eskiP, eskiS := piyasa, sanal
	piyasa, sanal = p, yeniSanalBorsa(p)
	eskiEylem := eylemiYap
	t.Cleanup(func() { piyasa, sanal, eylemiYap = eskiP, eskiS, eskiEylem })
	simdi := simdiF().UnixMilli()
	durumMu.Lock()
	k := kumbaraOlustur(acmaIstegi{Dosya: "AGTUSDT_1m.json", Icerik: []byte(`{"strateji":{"para":{"masraf":0.1}}}`), Sembol: "AGTUSDT", Aralik: "1m",
		Olusturuldu: simdi - 10*60_000, Bitis: simdi + 110*60_000, Mod: "sanal", Tutar: 100})
	durumMu.Unlock()
	return k, saat, p
}

// kumbara 12:00:00'da açıldı; 1 dk'lık mum 12:00:00–12:01:00 kapanışında motor alıyor
func TestKararTazeAlim(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	acilis := k.Baslangic / 60_000 * 60_000
	saat.ileri(62 * time.Second) // mum 12:01'de kapandı, karar 2 sn sonra geldi
	e := kararIsle(k, &kararIstegi{Mum: acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1.004}, MotorGirisMum: acilis})
	if e.tur != "al" || e.beklenen != 1.004 || e.motorMum != acilis {
		t.Fatalf("taze alım bekleniyordu: %+v", e)
	}
	// aynı mum yeniden bildirilirse ikinci kez alınmaz
	if e2 := kararIsle(k, &kararIstegi{Mum: acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1.004}}); e2.tur != "" {
		t.Fatalf("aynı mum iki kez alım: %+v", e2)
	}
}

func TestKararAcilmadanOnceKiMum(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	onceki := k.Baslangic/60_000*60_000 - 60_000 // kumbara açılmadan önce kapanan mum
	saat.ileri(5 * time.Second)
	if e := kararIsle(k, &kararIstegi{Mum: onceki, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}}); e.tur != "" {
		t.Fatalf("başlatılmadan önceki alıma atlanmamalı: %+v", e)
	}
}

func TestKararGecKalan(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	acilis := k.Baslangic / 60_000 * 60_000
	saat.ileri(60*time.Second + 45*time.Second) // kapanıştan 45 sn sonra (1 dk mumda sınır 30 sn)
	if e := kararIsle(k, &kararIstegi{Mum: acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}}); e.tur != "" {
		t.Fatalf("geç gelen alımın peşinden koşulmamalı: %+v", e)
	}
	if son := k.Olaylar[len(k.Olaylar)-1].Mesaj; !strings.Contains(son, "peşinden koşulmadı") {
		t.Fatalf("olay yazılmalıydı: %s", son)
	}
}

func acikIslemKoy(k *kumbara, motorMum int64, adet float64) {
	durumMu.Lock()
	defer durumMu.Unlock()
	alisIsle(k, dolumSonucu{Taraf: "BUY", Ortalama: 1, Miktar: adet, NetMiktar: adet, Tutar: adet, NetTutar: adet, Yol: "limit", Bitis: simdiF().UnixMilli()}, 1.002, motorMum, sembolKurali{Varlik: "AGT", FiyatOndalik: 3, MiktarOndalik: 2}, "")
}

func TestKararSatisVeKacanSatis(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	acilis := k.Baslangic / 60_000 * 60_000
	acikIslemKoy(k, acilis, 99)
	saat.ileri(3 * time.Minute)
	// motor hâlâ aynı işlemde: satış yok, seviyeler kaydedilir
	e := kararIsle(k, &kararIstegi{Mum: acilis + 120_000, Asama: "coinde", MotorGirisMum: acilis, Stop: 0.97, Hedef: 1.05})
	if e.tur != "" || k.Stop != 0.97 || k.Hedef != 1.05 {
		t.Fatalf("satış olmamalı, seviyeler kaydedilmeli: %+v stop %v hedef %v", e, k.Stop, k.Hedef)
	}
	// motor zarar durdurla sattı → hemen piyasa emri
	e = kararIsle(k, &kararIstegi{Mum: acilis + 180_000, Asama: "bekle", Olay: &kararOlayi{Tip: "sat", Fiyat: 0.97, Kural: "zararDurdur"}})
	if e.tur != "sat" || e.sebep != "zararDurdur" || !e.acil || e.beklenen != 0.97 {
		t.Fatalf("zarar durdur satışı: %+v", e)
	}
	if k.Stop != 0 {
		t.Fatal("motor çıkınca seviyeler silinmeli")
	}
}

func TestKararBaskaIslemdeyse(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	acilis := k.Baslangic / 60_000 * 60_000
	acikIslemKoy(k, acilis, 99)
	saat.ileri(30 * time.Minute)
	satisMum, satisFiyat := acilis+5*60_000, 1.03
	// program kapalıyken motor satmış ve yeniden almış: kumbara hâlâ eski coinde
	e := kararIsle(k, &kararIstegi{Mum: acilis + 29*60_000, Asama: "coinde", MotorGirisMum: acilis + 20*60_000,
		MotorIslemleri: []motorIslemi{{G: acilis, GF: 1.002, C: &satisMum, CF: &satisFiyat, K: "hedefKar"}}})
	if e.tur != "sat" || e.sebep != "hedefKar" || e.beklenen != 1.03 || e.acil {
		t.Fatalf("kaçan satış bekleniyordu: %+v", e)
	}
}

func TestBeklenenSatisSonradanDolar(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	acilis := k.Baslangic / 60_000 * 60_000
	acikIslemKoy(k, acilis, 99)
	durumMu.Lock()
	satisIsle(k, dolumSonucu{Taraf: "SELL", Ortalama: 0.96, Miktar: 99, NetTutar: 95, Yol: "piyasa"}, "anlikZarar", "", 0.97, sembolKurali{Varlik: "AGT"}, "")
	durumMu.Unlock()
	saat.ileri(2 * time.Minute)
	c, cf := acilis+60_000, 0.97
	kararIsle(k, &kararIstegi{Mum: acilis + 60_000, Asama: "bekle", MotorIslemleri: []motorIslemi{{G: acilis, GF: 1.002, C: &c, CF: &cf, K: "zararDurdur"}}})
	t0 := k.Islemler[0]
	if !t0.BeklenenVar || t0.MotorSebep != "zararDurdur" {
		t.Fatalf("simülatörün beklediği sonuç doldurulmalı: %+v", t0)
	}
	beklenen := ((1-0.001)*(0.97/1.002)*(1-0.001) - 1) * 100
	if !yakin(t0.BeklenenYuzde, beklenen, 1e-9) {
		t.Fatalf("beklenen yüzde %v, olan %v", beklenen, t0.BeklenenYuzde)
	}
}

// ---------- izleyici: zarar durdur, hedef, ömür ----------

func eylemYakala(t *testing.T) *[]eylem {
	var mu sync.Mutex
	var liste []eylem
	eylemiYap = func(k *kumbara, e eylem) {
		if e.tur == "" {
			return
		}
		mu.Lock()
		liste = append(liste, e)
		mu.Unlock()
	}
	return &liste
}

func TestIzleyiciZararDurdur(t *testing.T) {
	k, _, p := testKumbarasi(t)
	acikIslemKoy(k, 1, 99)
	k.Stop, k.Hedef = 0.97, 1.05
	liste := eylemYakala(t)
	p.fiyat = 0.98
	izleyiciTur()
	if len(*liste) != 0 {
		t.Fatalf("seviyeye gelmeden satış: %+v", *liste)
	}
	p.fiyat = 0.969
	izleyiciTur()
	if len(*liste) != 1 || (*liste)[0].sebep != "anlikZarar" || !(*liste)[0].acil {
		t.Fatalf("anlık zarar durdur bekleniyordu: %+v", *liste)
	}
	*liste = nil
	p.fiyat = 1.06
	izleyiciTur()
	if len(*liste) != 1 || (*liste)[0].sebep != "anlikHedef" || (*liste)[0].acil {
		t.Fatalf("hedefte limit kuralıyla satış bekleniyordu: %+v", *liste)
	}
}

func TestOmurVeEkSure(t *testing.T) {
	k, saat, p := testKumbarasi(t)
	acikIslemKoy(k, 1, 99)
	liste := eylemYakala(t)
	saat.ileri(time.Duration(k.Bitis-simdiF().UnixMilli()) * time.Millisecond)
	izleyiciTur()
	if k.Durum != "sureDoldu" || len(*liste) != 0 {
		t.Fatalf("ömür dolunca satış kuralı beklenmeli: durum %s eylem %+v", k.Durum, *liste)
	}
	// yeni alım yapılmaz
	acilis := simdiF().UnixMilli()/60_000*60_000 - 60_000
	if e := kararIsle(k, &kararIstegi{Mum: acilis + 60_000, Asama: "coinde", MotorGirisMum: 1}); e.tur != "" {
		t.Fatalf("eylem olmamalı: %+v", e)
	}
	saat.ileri(time.Duration(k.EkBitis-simdiF().UnixMilli()) * time.Millisecond)
	p.fiyat = 1.001
	izleyiciTur()
	if len(*liste) != 1 || (*liste)[0].sebep != "sureDoldu" {
		t.Fatalf("ek süre sonunda satış bekleniyordu: %+v", *liste)
	}
}

func TestOmurDoluncaKapanirVeParaDoner(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	if !yakin(D.SanalSerbest, 900, 1e-9) {
		t.Fatalf("sanal cüzdandan para ayrılmalı: %v", D.SanalSerbest)
	}
	eylemYakala(t)
	saat.ileri(time.Duration(k.Bitis-simdiF().UnixMilli()+1000) * time.Millisecond)
	izleyiciTur()
	if k.Durum != "bitti" || !yakin(D.SanalSerbest, 1000, 1e-9) {
		t.Fatalf("coinsiz kumbara ömür dolunca kapanmalı ve para dönmeli: %s %v", k.Durum, D.SanalSerbest)
	}
}

func TestYeniAyarlaDevam(t *testing.T) {
	k, _, _ := testKumbarasi(t)
	durumMu.Lock()
	k.Nakit = 103.456
	k.Devam = &devamPlani{Dosya: k.Dosya, Icerik: []byte(`{"sembol":"AGTUSDT","aralik":"1m"}`), Olusturuldu: simdiF().UnixMilli(), Bitis: simdiF().UnixMilli() + 3600_000}
	kumbaraBitir(k, "yeni ayara geçildi")
	durumMu.Unlock()
	if len(D.Kumbaralar) != 2 {
		t.Fatalf("yeni kumbara açılmalı: %d", len(D.Kumbaralar))
	}
	y := D.Kumbaralar[1]
	if y.BaslangicTutar != 103.45 || y.Durum != "calisiyor" || y.OncekiID != k.ID {
		t.Fatalf("yeni kumbara: %+v", y)
	}
	if !yakin(D.SanalSerbest, 900+0.006, 1e-9) {
		t.Fatalf("kuruş artığı cüzdana dönmeli: %v", D.SanalSerbest)
	}
}

func TestSanalAlimVeSatimUctanUca(t *testing.T) {
	k, saat, p := testKumbarasi(t)
	verildi := saat.t.UnixMilli()
	p.islemFn = func(simdi int64) []islemKaydi {
		if simdi >= verildi+2000 && simdi < verildi+3000 {
			return []islemKaydi{{Fiyat: 1.001, Miktar: 500, Zaman: simdi, AliciMaker: true}}
		}
		return nil
	}
	kumbaraAl(k, 1.004, 42)
	if k.acikIslem() == nil || !yakin(k.Adet, 99.90, 1e-9) || !yakin(k.Nakit, 100-99.90*1.001, 1e-9) {
		t.Fatalf("alım: adet %v nakit %v olaylar %+v", k.Adet, k.Nakit, k.Olaylar)
	}
	p.islemFn = nil
	kumbaraSat(k, "motor", "test", 1.02, true)
	if k.acikIslem() != nil || k.Adet != 0 {
		t.Fatalf("satış sonrası coin kalmamalı: %v", k.Adet)
	}
	t0 := k.Islemler[0]
	if t0.Satis == nil || t0.Satis.Yol != "piyasa" || !yakin(t0.Net, t0.Gelen-t0.Harcanan, 1e-12) {
		t.Fatalf("işlem: %+v", t0)
	}
	if _, err := os.Stat(veriKlasoru + "/kayitlar/islemler.csv"); err != nil {
		t.Fatal("islemler.csv yazılmalı")
	}
	if _, err := os.Stat(veriKlasoru + "/kayitlar/durum.json"); err != nil {
		t.Fatal("durum.json yazılmalı")
	}
	// yeniden yükleme: kayıt aynı
	durumYukle()
	if len(D.Kumbaralar) != 1 || len(D.Kumbaralar[0].Islemler) != 1 {
		t.Fatalf("durum.json'dan yüklenemedi: %+v", D.Kumbaralar)
	}
}

// ---------- MEXC imzası, izinler, anahtar saklama ----------

func TestHMACImza(t *testing.T) {
	// Binance/MEXC belgelerindeki örnek (HMAC SHA256, aynı yöntem)
	gizli := "NhqPtmdSJYdKjVHjA7PZj4Mge3R5YNiP1e3UZjInClVN65XAbvqqM6A7H5fATj0j"
	sorgu := "symbol=LTCBTC&side=BUY&type=LIMIT&timeInForce=GTC&quantity=1&price=0.1&recvWindow=5000&timestamp=1499827319559"
	if got := hmacHex(gizli, sorgu); got != "c8db56825ae71d6d79447849e617115f4a920fa2acdcab2b053c4b2838bd6b71" {
		t.Fatalf("imza yanlış: %s", got)
	}
}

func TestIzinDenetle(t *testing.T) {
	yasak, eksik := izinDenetle([]string{"SPOT_ACCOUNT_READ", "SPOT_DEAL_READ", "SPOT_DEAL_WRITE"})
	if len(yasak) != 0 || len(eksik) != 0 {
		t.Fatalf("uygun anahtar: %v %v", yasak, eksik)
	}
	yasak, _ = izinDenetle([]string{"SPOT_ACCOUNT_READ", "SPOT_DEAL_READ", "SPOT_DEAL_WRITE", "SPOT_WITHDRAW_WRITE"})
	if len(yasak) != 1 {
		t.Fatalf("para çekme izni yakalanmalı: %v", yasak)
	}
	yasak, _ = izinDenetle([]string{"spot_transfer_read", "SPOT_ACCOUNT_READ"})
	if len(yasak) != 1 {
		t.Fatalf("transfer izni yakalanmalı: %v", yasak)
	}
	_, eksik = izinDenetle([]string{"SPOT_ACCOUNT_READ"})
	if len(eksik) != 2 {
		t.Fatalf("eksik izinler: %v", eksik)
	}
}

func TestAnahtarSaklama(t *testing.T) {
	t.Setenv("ALSAT_ANAHTAR_KLASORU", t.TempDir())
	if err := anahtarKaydet(anahtarKaydi{Erisim: " mx0abc ", Gizli: "gizli123456"}); err != nil {
		t.Fatal(err)
	}
	ham, _ := os.ReadFile(anahtarYolu())
	if strings.Contains(string(ham), "gizli123456") {
		t.Fatal("gizli anahtar düz yazı saklanmamalı")
	}
	a, err := anahtarOku()
	if err != nil || a.Erisim != "mx0abc" || a.Gizli != "gizli123456" {
		t.Fatalf("okunamadı: %+v %v", a, err)
	}
	if err := anahtarSil(); err != nil {
		t.Fatal(err)
	}
	if _, err := anahtarOku(); !os.IsNotExist(err) {
		t.Fatalf("silinmeliydi: %v", err)
	}
	if anahtarMaskesi("mx0vglBqh0vDS2Kq") != "mx0v…S2Kq" {
		t.Fatal(anahtarMaskesi("mx0vglBqh0vDS2Kq"))
	}
}

// ---------- strateji klasörü: süresi dolanı silme, yeniden dışa aktarılanla devam ----------

func stratejiDosyasiYaz(t *testing.T, klasor, ad, sembol, aralik string, olus time.Time) {
	t.Helper()
	icerik := `{"tur":"kripto-simulator-strateji","surum":3,"olusturuldu":"` + olus.UTC().Format(time.RFC3339Nano) + `","borsa":"mexc","sembol":"` + sembol +
		`","aralik":"` + aralik + `","strateji":{"surum":3,"kurallar":{},"para":{"baslangic":1000,"masraf":0.1}}}`
	if err := os.WriteFile(klasor+"/"+ad, []byte(icerik), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSuresiDolanDosyaSilinirVeArsivlenir(t *testing.T) {
	saatKur(t)
	veriKlasoru = t.TempDir()
	D = yeniDurum()
	klasor := t.TempDir()
	t.Setenv("ALSAT_KLASOR", klasor)
	stratejiDosyasiYaz(t, klasor, "AGTUSDT_1m.json", "AGTUSDT", "1m", simdiF().Add(-25*time.Hour))   // 1 gün ömür: dolmuş
	stratejiDosyasiYaz(t, klasor, "BTCUSDT_15m.json", "BTCUSDT", "15m", simdiF().Add(-25*time.Hour)) // 1 hafta ömür: geçerli
	os.WriteFile(klasor+"/bozuk.json", []byte("{"), 0644)                                            // bozuk dosyaya dokunulmaz
	dosyalariTemizle()
	if _, err := os.Stat(klasor + "/AGTUSDT_1m.json"); !os.IsNotExist(err) {
		t.Fatal("süresi dolan dosya silinmeliydi")
	}
	for _, ad := range []string{"BTCUSDT_15m.json", "bozuk.json"} {
		if _, err := os.Stat(klasor + "/" + ad); err != nil {
			t.Fatalf("%s silinmemeliydi", ad)
		}
	}
	arsiv, _ := os.ReadDir(veriKlasoru + "/kayitlar/arsiv")
	if len(arsiv) != 1 || !strings.HasPrefix(arsiv[0].Name(), "AGTUSDT_1m_") {
		t.Fatalf("arşiv: %v", arsiv)
	}
	if len(D.GenelOlaylar) != 1 || !strings.Contains(D.GenelOlaylar[0].Mesaj, "silindi") {
		t.Fatalf("genel olay: %+v", D.GenelOlaylar)
	}
}

func TestYenidenDisaAktarilincaYeniAyarlaDevam(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	klasor := t.TempDir()
	t.Setenv("ALSAT_KLASOR", klasor)
	// aynı dosya, aynı ayarla: bir şey olmaz
	stratejiDosyasiYaz(t, klasor, "AGTUSDT_1m.json", "AGTUSDT", "1m", time.UnixMilli(k.Olusturuldu))
	yeniAyarKontrolu()
	if k.Durum != "calisiyor" || k.Devam != nil {
		t.Fatalf("değişmeyen dosyada devam planı olmamalı: %s", k.Durum)
	}
	// elde coin varken yeniden dışa aktarıldı: işlem bitince geçilecek, bu arada yeni alım yok
	acikIslemKoy(k, 1, 99)
	saat.ileri(time.Minute)
	stratejiDosyasiYaz(t, klasor, "AGTUSDT_1m.json", "AGTUSDT", "1m", simdiF())
	yeniAyarKontrolu()
	if k.Durum != "durduruluyor" || k.Devam == nil || len(D.Kumbaralar) != 1 {
		t.Fatalf("devam planı kurulmalı: %s %+v", k.Durum, k.Devam)
	}
	durumMu.Lock()
	satisIsle(k, dolumSonucu{Taraf: "SELL", Ortalama: 1.01, Miktar: 99, NetTutar: 100.5, Yol: "piyasa"}, "motor", "", 0, sembolKurali{Varlik: "AGT"}, "")
	durumMu.Unlock()
	eylemYakala(t)
	izleyiciTur()
	if k.Durum != "bitti" || len(D.Kumbaralar) != 2 {
		t.Fatalf("işlem bitince yeni kumbara açılmalı: %s, %d kumbara", k.Durum, len(D.Kumbaralar))
	}
	y := D.Kumbaralar[1]
	if y.Durum != "calisiyor" || y.Olusturuldu != simdiF().UnixMilli()/1000*1000 && y.Olusturuldu != simdiF().UnixMilli() || y.Mod != "sanal" || y.IlkMum != 0 {
		t.Fatalf("yeni kumbara: %+v", y)
	}
	if !yakin(y.BaslangicTutar, math.Floor(k.Iade*100)/100, 1e-9) {
		t.Fatalf("para yeni kumbaraya geçmeli: %v / %v", y.BaslangicTutar, k.Iade)
	}
}

// Bilgisayarın saati MEXC'den 40 sn ileriyse taze karar "geç" sanılmamalı
func TestSaatFarkiHesabaKatilir(t *testing.T) {
	k, saat, _ := testKumbarasi(t)
	borsaFarki.Store(-40_000) // MEXC saati = bilgisayar saati − 40 sn
	t.Cleanup(func() { borsaFarki.Store(0) })
	acilis := (k.Baslangic-40_000)/60_000*60_000 + 60_000 // MEXC saatiyle kumbaradan sonra açılan mum
	// mum MEXC saatiyle kapandıktan 2 sn sonra karar geliyor; bilgisayarın saatine göre bu 42 sn sonra
	saat.ileri(time.Duration(acilis+60_000+40_000+2_000-simdiF().UnixMilli()) * time.Millisecond)
	e := kararIsle(k, &kararIstegi{Mum: acilis, Asama: "coinde", Olay: &kararOlayi{Tip: "al", Fiyat: 1}, MotorGirisMum: acilis})
	if e.tur != "al" {
		t.Fatalf("saat farkı hesaba katılmalıydı: %+v, olaylar %+v", e, k.Olaylar)
	}
}

func TestSatmadanKapat(t *testing.T) {
	k, _, _ := testKumbarasi(t)
	acikIslemKoy(k, 1, 99)
	k.SonFiyat = 1.02
	k.SatisTekrar, k.TekrarSayisi = simdiF().UnixMilli()+60_000, 3
	durumMu.Lock()
	satmadanKapat(k)
	durumMu.Unlock()
	if k.Durum != "bitti" || k.Islemler[0].Satis == nil || k.Islemler[0].Satis.Yol != "birakildi" {
		t.Fatalf("satmadan kapanmalı: %s %+v", k.Durum, k.Islemler[0].Satis)
	}
	// sanal: coin bugünkü değeriyle cüzdana döner
	if !yakin(k.Iade, 1+99*1.02, 1e-9) || !yakin(D.SanalSerbest, 900+1+99*1.02, 1e-9) {
		t.Fatalf("iade %v cüzdan %v", k.Iade, D.SanalSerbest)
	}
}
