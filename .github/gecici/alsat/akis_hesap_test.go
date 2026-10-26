package main

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// mumlarKur: kapanış dizisinden mumlar (fitiller kapanışın ±%0,1'i; açılış önceki kapanış)
func mumlarKur(kapanislar []float64, bas int64, ms int64) []mumOHLC {
	m := make([]mumOHLC, len(kapanislar))
	for i, c := range kapanislar {
		o := c
		if i > 0 {
			o = kapanislar[i-1]
		}
		m[i] = mumOHLC{Z: bas + int64(i)*ms, O: o, H: math.Max(o, c) * 1.001, L: math.Min(o, c) * 0.999, C: c}
	}
	return m
}

func dogru(a, b float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = a + (b-a)*float64(i+1)/float64(n)
	}
	return out
}

func TestSalinimlarVeKirilimlar(t *testing.T) {
	// 100 → 110 (tepe) → 104 (dip) → 112 (yeni tepe: 110 kırılır) → 107 → 100 (104 kırılır)
	var k []float64
	k = append(k, dogru(99, 100, 5)...)
	k = append(k, dogru(100, 110, 15)...)
	k = append(k, dogru(110, 104, 10)...)
	k = append(k, dogru(104, 112, 12)...)
	k = append(k, dogru(112, 107, 6)...)
	k = append(k, dogru(107, 100, 10)...)
	k = append(k, dogru(100, 101, 4)...)
	m := mumlarKur(k, 0, 60_000)
	atr := atrHesapla(m, 14)
	s := salinimlar(m, 3, true)
	if len(s) < 4 {
		t.Fatalf("salınım az: %+v", s)
	}
	for i := 1; i < len(s); i++ {
		if s[i].Tepe == s[i-1].Tepe {
			t.Fatal("tepe ve dip sırayla gelmeli")
		}
		if s[i].Onay != s[i].I+3 {
			t.Fatal("salınım uzunluk kadar mum sonra kesinleşmeli")
		}
	}
	// 110 civarındaki tepe bulunmalı (19. ve 20. mumun tepesi eşit: kesin büyük olan sonraki sayılır)
	var tepe110 *smcSalinim
	for i := range s {
		if s[i].Tepe && math.Abs(s[i].F-110*1.001) < 1e-6 {
			tepe110 = &s[i]
		}
	}
	if tepe110 == nil || tepe110.F != m[tepe110.I].H {
		t.Fatalf("110 tepesi bulunamadı: %+v", s)
	}
	kir, ob, trend := yapiHesapla(m, s, atr, true)
	// beklenen: 110'un üstüne çıkış = yukarı kırılım; 104'ün altına iniş = aşağı kırılım (yukarı trendden → CHoCH)
	var yukari, asagi *smcKirilim
	for i := range kir {
		if kir[i].Yon == 1 && math.Abs(kir[i].F-tepe110.F) < 1e-9 {
			yukari = &kir[i]
		}
		if kir[i].Yon == -1 && kir[i].F < 105 && kir[i].F > 103 {
			asagi = &kir[i]
		}
	}
	if yukari == nil || asagi == nil {
		t.Fatalf("kırılımlar: %+v", kir)
	}
	if m[yukari.I].C <= yukari.F || m[yukari.I-1].C > yukari.F {
		t.Fatal("yukarı kırılım, kapanışın seviyeyi ilk geçtiği mumda olmalı")
	}
	if !asagi.Choch {
		t.Fatal("yukarı trendde dibin kırılması CHoCH olmalı")
	}
	if trend != -1 {
		t.Fatalf("son trend aşağı olmalı: %d", trend)
	}
	// yukarı kırılımın talep bölgesi: tepe ile kırılım arasındaki en düşük mum (104 dibi)
	var talep *smcBolge
	for i := range ob {
		if ob[i].Yon == 1 && ob[i].Olus == yukari.I {
			talep = &ob[i]
		}
	}
	if talep == nil {
		t.Fatal("yukarı kırılımda talep bölgesi oluşmadı")
	}
	enDusuk := yukari.I0 + 1
	for j := yukari.I0 + 1; j < yukari.I; j++ {
		if m[j].L < m[enDusuk].L {
			enDusuk = j
		}
	}
	if talep.I0 != enDusuk || talep.Alt != m[enDusuk].L {
		t.Fatalf("talep bölgesi yanlış mum: %+v (beklenen %d)", talep, enDusuk)
	}
	// fiyat 100'e inince (bölgenin altında kapanış) talep bölgesi geçersiz
	if talep.I1 < 0 {
		t.Fatal("altında kapanılan talep bölgesi geçersiz olmalı")
	}
	// geleceğe bakmama: ilk 40 mumla hesap, tam hesabın ilk 40 mumdaki kısmıyla aynı
	kisa := salinimlar(m[:40], 3, true)
	for _, x := range kisa {
		ayni := false
		for _, y := range s {
			if y == x {
				ayni = true
			}
		}
		if !ayni {
			t.Fatalf("kısa veride farklı salınım (geleceğe bakıyor): %+v", x)
		}
	}
}

func TestFVGEsitDiplerVeSupurme(t *testing.T) {
	m := []mumOHLC{}
	ekle := func(o, h, l, c float64) { m = append(m, mumOHLC{Z: int64(len(m)) * 60_000, O: o, H: h, L: l, C: c}) }
	for i := 0; i < 20; i++ {
		ekle(100, 100.5, 99.5, 100)
	}
	// yukarı boşluk: 2 önceki mumun tepesi 100.5, sonraki mumun dibi 102
	ekle(100, 103, 99.8, 102.8)
	ekle(102.8, 104, 102, 103.5)
	for i := 0; i < 5; i++ {
		ekle(103.5, 104, 103, 103.5)
	}
	atr := atrHesapla(m, 14)
	f := fvgHesapla(m, atr, 0.1)
	var bul *smcBolge
	for i := range f {
		if f[i].Yon == 1 {
			bul = &f[i]
		}
	}
	if bul == nil || bul.Alt != 100.5 || bul.Ust != 102 || bul.I1 >= 0 {
		t.Fatalf("FVG: %+v", f)
	}
	// boşluk doldurulursa biter
	ekle(103.5, 103.6, 100.2, 100.4)
	f = fvgHesapla(m, atrHesapla(m, 14), 0.1)
	for _, x := range f {
		if x.Yon == 1 && x.Alt == 100.5 && x.I1 < 0 {
			t.Fatal("dolan FVG bitmeli")
		}
	}

	// eşit dipler ve süpürme
	m = nil
	for i := 0; i < 10; i++ {
		ekle(105, 105.5, 104.5, 105)
	}
	diz := []float64{104, 103, 102, 101, 102, 103, 104, 103, 102, 101.02, 102, 103, 104, 104.5, 104}
	for _, c := range diz {
		ekle(c, c+0.3, c-0.3, c)
	}
	// süpürme: fitil 100.4'e iner (eşit diplerin 100.7 altında), kapanış 101.5
	ekle(104, 104.2, 100.4, 101.5)
	ekle(101.5, 102.5, 101.2, 102.3)
	a := akisVarsayilan()
	a.IcUzunluk, a.AnaUzunluk = 3, 8
	a.EsitHassas = 0.3
	s := smcHesapla(m, a)
	esit := false
	for _, q := range s.Esitler {
		if !q.Tepe && q.F < 101 && q.F > 100.5 {
			esit = true
			if q.Supurme != len(m)-2 {
				t.Fatalf("eşit dipler süpürüldüğü mumda işaretlenmeli: %+v", q)
			}
		}
	}
	if !esit {
		t.Fatalf("eşit dipler bulunamadı: %+v", s.Esitler)
	}
	sup := false
	for _, x := range s.Supurmeler {
		if x.Yon == 1 && x.I == len(m)-2 && x.Guclu {
			sup = true
		}
	}
	if !sup {
		t.Fatalf("süpürme bulunamadı: %+v", s.Supurmeler)
	}
}

func TestFootprintDengesizlik(t *testing.T) {
	iz := map[int32][2]float64{}
	// satır boyu 2 kova; alt 4 satırda alım, bir alttaki satımın 5 katı
	for k := int32(1000); k < 1008; k++ {
		iz[k] = [2]float64{10, 2}
	}
	iz[1008] = [2]float64{1, 30} // üstte satış yığını: satım, bir üst satırdaki alımın çok üstünde
	iz[1009] = [2]float64{0, 30}
	iz[1010] = [2]float64{0, 30}
	iz[1011] = [2]float64{0, 30}
	iz[1012] = [2]float64{0.5, 0}
	f := fpHesapla(iz, 2, 3)
	if f.AlimYigin < 3 {
		t.Fatalf("üst üste alım dengesizliği bulunamadı: %+v", f)
	}
	if f.SatimYigin < 2 {
		t.Fatalf("satış yığını: %+v", f)
	}
	if f.POC < akisKovaFiyat(1008) || f.POC > akisKovaFiyat(1012) {
		t.Fatalf("POC en çok işlem gören satırda olmalı: %v", f.POC)
	}
	// eksi kovalar (fiyat < 1) doğru satıra düşmeli
	g := fpHesapla(map[int32][2]float64{-3: {1, 0}, -2: {1, 0}, -1: {1, 0}, 0: {1, 0}}, 2, 3)
	// satır = aşağı yuvarlanmış kova/2: −3→−2, −2→−1, −1→−1, 0→0
	if len(g.Satirlar) != 3 || g.Satirlar[0].K != -2 || g.Satirlar[1].K != -1 || g.Satirlar[1].Al != 2 || g.Satirlar[2].K != 0 {
		t.Fatalf("eksi kovalar: %+v", g.Satirlar)
	}
}

func TestAkisMumlaraTopla(t *testing.T) {
	ms := int64(300_000)
	var dk []akisDakika
	for i := int64(0); i < 10; i++ {
		if i == 7 {
			continue // eksik dakika
		}
		dk = append(dk, akisDakika{Z: i * 60_000, Al: 1, Sat: 2, VV: true, VAl: 3, OI: 100 + float64(i), OIA: 99 + float64(i), OIY: 101 + float64(i), K: 10, BAl: 0.5})
	}
	out := akisMumlaraTopla([]int64{0, ms, 2 * ms}, ms, dk)
	if out[0].Al != 5 || out[0].Sat != 10 || out[0].Eksik || out[0].VVOran != 1 || out[0].BAlT != 25 {
		t.Fatalf("ilk mum: %+v", out[0])
	}
	if !out[1].Eksik || out[1].Dakika != 4 {
		t.Fatalf("eksik dakikalı mum eksik sayılmalı: %+v", out[1])
	}
	// başlangıç OI: önceki dakikanın sonu (dakika 4: 104)
	if out[1].OIBas != 104 || out[1].OISon != 109 {
		t.Fatalf("OI başı/sonu: %+v", out[1])
	}
	if out[2].Var {
		t.Fatal("veri olmayan mum")
	}
}

func TestAkisAyarDuzelt(t *testing.T) {
	var a akisAyar
	json.Unmarshal([]byte(`{"acik":true,"esik":500,"agirlik":[9,-1,1,1],"beklemeMum":99,"kovalama":-2,"icUzunluk":60,"anaUzunluk":3}`), &a)
	a = akisDuzelt(a)
	if a.Esik != 100 || a.Agirlik[0] != 3 || a.Agirlik[1] != 0 || a.BeklemeMum != 10 || a.Kovalama != 0 {
		t.Fatalf("sınırlar: %+v", a)
	}
	if a.IcUzunluk != 20 || a.AnaUzunluk <= a.IcUzunluk {
		t.Fatalf("yapı uzunlukları: %d %d", a.IcUzunluk, a.AnaUzunluk)
	}
	if a.DengesizlikOran != 3 || a.CvdBakis != 20 {
		t.Fatal("eksik alanlar varsayılana dönmeli")
	}
	nanli := akisDuzelt(akisAyar{Esik: math.NaN(), Kovalama: math.Inf(1), FonYuksek: math.NaN()})
	if math.IsNaN(nanli.Esik) || math.IsInf(nanli.Kovalama, 0) || math.IsNaN(nanli.FonYuksek) {
		t.Fatalf("NaN/sonsuz değer: %+v", nanli)
	}
}

// ---------- senaryolar ----------

type senaryoMum struct {
	o, h, l, c float64
	al, sat    float64
	oi         float64
}

func senaryoGirdi(sm []senaryoMum, simdiki *senaryoMum, vadeli bool, a akisAyar) akisHesapGirdi {
	ms := int64(60_000)
	bas := int64(1_800_000_000_000) / ms * ms
	g := akisHesapGirdi{Sembol: "TESTUSDT", Aralik: "1m", Ms: ms, Ayar: a}
	for i, x := range sm {
		z := bas + int64(i)*ms
		g.Mumlar = append(g.Mumlar, mumOHLC{Z: z, O: x.o, H: x.h, L: x.l, C: x.c})
		d := akisDakika{Z: z, A: x.o, Y: x.h, D: x.l, K: x.c, Al: x.al, Sat: x.sat, N: 10}
		if vadeli {
			d.VV, d.OI, d.OIA, d.OIY, d.Fon = true, x.oi, x.oi, x.oi, 0.0001
		}
		g.Veri.Dakikalar = append(g.Veri.Dakikalar, d)
	}
	sonZ := bas + int64(len(sm))*ms
	g.Simdi = sonZ + 30_000
	g.Veri.SonFiyat = sm[len(sm)-1].c
	if simdiki != nil {
		d := akisDakika{Z: sonZ, A: simdiki.o, Y: simdiki.h, D: simdiki.l, K: simdiki.c, Al: simdiki.al, Sat: simdiki.sat, N: 5}
		if vadeli {
			d.VV, d.OI, d.OIA, d.OIY, d.Fon = true, simdiki.oi, simdiki.oi, simdiki.oi, 0.0001
		}
		g.Veri.Dakikalar = append(g.Veri.Dakikalar, d)
		g.Veri.SonFiyat = simdiki.c
		g.Veri.Simdiki = true
	}
	g.Veri.Vadeli, g.Veri.SpotCanli, g.Veri.VadCanli = vadeli, true, vadeli
	g.Veri.SonFon = 0.0001
	g.Veri.Durum = "Canlı"
	return g
}

// yükseliş → düşüş (dip 104) → tepki → dibin altına süpürme, emilim ve dönüş; düşüşte OI azalıyor
func idealDip() []senaryoMum {
	var sm []senaryoMum
	oi := 1000.0
	ek := func(o, c, al, sat float64) {
		h, l := math.Max(o, c)*1.0008, math.Min(o, c)*0.9992
		sm = append(sm, senaryoMum{o, h, l, c, al, sat, oi})
	}
	p := 100.0
	for i := 0; i < 30; i++ { // 100 → 110
		n := p + 10.0/30
		oi *= 1.001
		ek(p, n, 12, 8)
		p = n
	}
	for i := 0; i < 15; i++ { // 110 → 104
		n := p - 6.0/15
		oi *= 1.0005
		ek(p, n, 7, 13)
		p = n
	}
	for i := 0; i < 10; i++ { // 104 → 108
		n := p + 4.0/10
		oi *= 1.0005
		ek(p, n, 16, 6)
		p = n
	}
	for i := 0; i < 4; i++ { // 108 → 105
		n := p - 3.0/4
		oi *= 0.997
		ek(p, n, 9, 11)
		p = n
	}
	// süpürme mumu: 105 → fitil 103.4 → kapanış 104.6; hacim yüksek, delta eksi, kapanış üst yarıda
	oi *= 0.995
	sm = append(sm, senaryoMum{105, 105.1, 103.4, 104.6, 18, 42, oi})
	// dönüş mumu
	oi *= 0.999
	sm = append(sm, senaryoMum{104.6, 105.4, 104.5, 105.3, 14, 6, oi})
	return sm
}

func TestAkisIdealDipOnay(t *testing.T) {
	a := akisVarsayilan()
	a.IcUzunluk, a.AnaUzunluk = 3, 10
	sm := idealDip()
	g := senaryoGirdi(sm, &senaryoMum{105.3, 105.5, 105.2, 105.4, 4, 2, sm[len(sm)-1].oi}, true, a)
	h := akisHesapla(g)
	r := h.sonuc
	t.Logf("%s", r.Ozet)
	for _, b := range r.Bloklar {
		t.Logf("  %s %+.0f var=%v %s %v", b.Ad, b.Puan, b.Var, b.Neden, b.Maddeler)
	}
	if !r.Onay || r.Baglam != "dip" {
		t.Fatalf("ideal dip onaylanmalı: %+v", r)
	}
	ara := func(blok int, parca string) bool {
		for _, x := range r.Bloklar[blok].Maddeler {
			if strings.Contains(x.M, parca) && x.P > 0 {
				return true
			}
		}
		return false
	}
	if !ara(0, "Stoplar süpürüldü") {
		t.Fatal("yapı: süpürme görülmeli")
	}
	if !ara(1, "Dipte satış emildi") || !ara(1, "Delta eksiden artıya") {
		t.Fatal("emir akışı: emilim ve delta dönüşü görülmeli")
	}
	if !ara(2, "CVD yapmadı") {
		t.Fatal("CVD: pozitif uyumsuzluk görülmeli")
	}
	if !ara(3, "Düşüşte açık pozisyon") {
		t.Fatal("OI: düşüşte azalma görülmeli")
	}
	for _, b := range r.Bloklar {
		if !b.Var || b.Puan <= 0 {
			t.Fatalf("%s bloğu olumlu olmalı: %+v", b.Ad, b)
		}
	}
}

func TestAkisDusenBicakRed(t *testing.T) {
	a := akisVarsayilan()
	a.IcUzunluk, a.AnaUzunluk = 3, 10
	var sm []senaryoMum
	oi := 1000.0
	p := 100.0
	ek := func(o, c, al, sat float64) {
		h, l := math.Max(o, c)*1.0008, math.Min(o, c)*0.9992
		sm = append(sm, senaryoMum{o, h, l, c, al, sat, oi})
	}
	for i := 0; i < 30; i++ {
		n := p + 10.0/30
		ek(p, n, 12, 8)
		p = n
	}
	for i := 0; i < 15; i++ {
		n := p - 6.0/15
		ek(p, n, 7, 13)
		p = n
	}
	for i := 0; i < 8; i++ {
		n := p + 3.0/8
		ek(p, n, 12, 8)
		p = n
	}
	for i := 0; i < 12; i++ { // dibi kırıp hızla düşüş, satış saldırısı, OI artıyor (yeni short'lar)
		n := p - 7.0/12
		oi *= 1.003
		ek(p, n, 4, 20)
		p = n
	}
	son := sm[len(sm)-1]
	g := senaryoGirdi(sm, &senaryoMum{son.c, son.c, son.l * 0.997, son.l * 0.997, 2, 30, oi * 1.002}, true, a)
	r := akisHesapla(g).sonuc
	t.Logf("%s", r.Ozet)
	for _, b := range r.Bloklar {
		t.Logf("  %s %+.0f %v", b.Ad, b.Puan, b.Maddeler)
	}
	if r.Onay {
		t.Fatalf("düşen bıçak onaylanmamalı: %+v", r)
	}
	if r.Bloklar[1].Puan >= 0 || r.Bloklar[3].Puan >= 0 {
		t.Fatal("emir akışı ve OI olumsuz olmalı")
	}
	if r.Yasak == "" {
		t.Fatal("satış saldırısı ya da yapı kırılımı yasağı çalışmalı")
	}
}

func TestAkisVeriEksik(t *testing.T) {
	a := akisVarsayilan()
	a.IcUzunluk, a.AnaUzunluk = 3, 10
	sm := idealDip()
	g := senaryoGirdi(sm, nil, false, a)
	g.Veri.Dakikalar = nil // hiç akış verisi yok
	r := akisHesapla(g).sonuc
	if r.Onay || r.Eksik == "" || !strings.Contains(r.Eksik, "Emir akışı") {
		t.Fatalf("akış verisi yokken onay verilmemeli: %+v", r)
	}
	if !r.Bloklar[0].Var || r.Bloklar[1].Var || r.Bloklar[3].Var {
		t.Fatal("yapı mumlardan hesaplanır, akış ve OI veri yok")
	}
	// akış şartı kapatılırsa sadece yapıyla karar verilebilir
	a.AkisSart = false
	g.Ayar = a
	r = akisHesapla(g).sonuc
	if r.Eksik != "" {
		t.Fatalf("akış şartı kapalıyken eksik sayılmamalı: %s", r.Eksik)
	}
	if r.Toplam != r.Bloklar[0].Puan {
		t.Fatalf("toplam sadece verisi olan bloklardan: %v ≠ %v", r.Toplam, r.Bloklar[0].Puan)
	}
	// vadelisi olmayan coin: OI bloğu yok, ağırlıklar kalanlara dağılır
	g2 := senaryoGirdi(idealDip(), nil, false, akisVarsayilan())
	r2 := akisHesapla(g2).sonuc
	if r2.Bloklar[3].Var || !strings.Contains(r2.Bloklar[3].Neden, "vadelisi yok") {
		t.Fatalf("vadelisi olmayan coin: %+v", r2.Bloklar[3])
	}
	// ağırlık 0 olan blok toplama girmez
	a3 := akisVarsayilan()
	a3.IcUzunluk, a3.AnaUzunluk = 3, 10
	a3.Agirlik = [4]float64{0, 1, 0, 0}
	r3 := akisHesapla(senaryoGirdi(idealDip(), nil, true, a3)).sonuc
	if r3.Toplam != r3.Bloklar[1].Puan {
		t.Fatalf("sadece emir akışı ağırlıklı: %v ≠ %v", r3.Toplam, r3.Bloklar[1].Puan)
	}
	// çok az mum
	r4 := akisHesapla(akisHesapGirdi{Mumlar: make([]mumOHLC, 5), Ayar: akisVarsayilan(), Ms: 60_000}).sonuc
	if r4.Onay || r4.Eksik == "" {
		t.Fatal("az mumla onay olmaz")
	}
}
