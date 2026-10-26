package main

import (
	"testing"
	"time"
)

// 0.3: bekçi, kullanıcı pencereyi kapatınca programı kapatır; kumbara çalışırken pencere kendiliğinden susarsa bekler.
func TestBekciKarari(t *testing.T) {
	cokEski := 1000 * time.Hour
	durum := func(g bekciGozlem) int { k, _ := bekciKarari(g); return k }
	temel := bekciGozlem{HicBaglandi: true, AcikGecen: time.Hour, KumbaraVar: true, SonAcmaGecen: cokEski}

	// pencere bağlıyken hiçbir şey yapılmaz
	g := temel
	g.Baglanti = 1
	if durum(g) != bekciDevam {
		t.Fatal("bağlıyken devam etmeli")
	}

	// 0.2'deki olay: kumbara çalışıyor, pencere kapatılmadan 25 sn sustu → kapanmamalı, beklemeli
	g = temel
	g.Sessiz = 25 * time.Second
	if durum(g) != bekciBekle {
		t.Fatalf("kumbara varken susan pencere için beklemeli: %d", durum(g))
	}
	// 2 dk geçti → pencereyi yeniden aç
	g.Sessiz = yenidenAcSuresi
	if durum(g) != bekciYenidenAc {
		t.Fatal("2 dk sonra pencere yeniden açılmalı")
	}
	// az önce açmayı denedi → 10 dk dolmadan tekrar açmaz, bekler
	g.SonAcmaGecen = 3 * time.Minute
	if durum(g) != bekciBekle {
		t.Fatal("yeniden açma 10 dk'da bir denenmeli")
	}
	g.SonAcmaGecen = yenidenAcAraligi
	if durum(g) != bekciYenidenAc {
		t.Fatal("10 dk sonra tekrar denemeli")
	}

	// kullanıcı pencereyi kapattı (sayfa haber verdi) → 20 sn sonra kapan (0.2 gibi)
	g = temel
	g.Kapatildi = true
	g.Sessiz = 10 * time.Second
	if durum(g) != bekciDevam {
		t.Fatal("20 sn dolmadan kapanmamalı (sayfa yenilenebilir)")
	}
	g.Sessiz = 21 * time.Second
	if k, sebep := bekciKarari(g); k != bekciCik || sebep != "pencere kapatıldı" {
		t.Fatalf("kapatılan pencerede çıkmalı: %d %s", k, sebep)
	}

	// tarayıcı normal şekilde kapandı (sayfa haber veremediyse) → kapan
	g = temel
	g.TarayiciYok = true
	g.Sessiz = 21 * time.Second
	if k, sebep := bekciKarari(g); k != bekciCik || sebep != "tarayıcı kapatıldı" {
		t.Fatalf("tarayıcı kapanınca çıkmalı: %d %s", k, sebep)
	}

	// çalışan kumbara yok → 0.2 gibi 20 sn sonra kapan
	g = temel
	g.KumbaraVar = false
	g.Sessiz = 21 * time.Second
	if durum(g) != bekciCik {
		t.Fatal("kumbara yokken susan pencerede çıkmalı")
	}

	// pencere hiç açılmadı: kumbara yoksa 3 dk sonra kapan, varsa kapanma ve açmayı dene
	g = bekciGozlem{AcikGecen: 4 * time.Minute, SonAcmaGecen: cokEski}
	if durum(g) != bekciCik {
		t.Fatal("pencere hiç açılmadıysa ve kumbara yoksa çıkmalı")
	}
	g = bekciGozlem{AcikGecen: 30 * time.Second, SonAcmaGecen: cokEski, KumbaraVar: true}
	if durum(g) != bekciBekle {
		t.Fatal("pencere açılmadı ama kumbara var: beklemeli")
	}
	g.AcikGecen = yenidenAcSuresi
	if durum(g) != bekciYenidenAc {
		t.Fatal("pencere açılmadı ama kumbara var: yeniden açmayı denemeli")
	}
}

func TestKullaniciKapatti(t *testing.T) {
	sonKopanOturum.Store("")
	kapananOturumlar.Range(func(k, _ any) bool { kapananOturumlar.Delete(k); return true })
	if kullaniciKapatti() {
		t.Fatal("hiç kopan yokken false olmalı")
	}
	sonKopanOturum.Store("aaa")
	if kullaniciKapatti() {
		t.Fatal("haber vermeyen pencere kapatılmış sayılmamalı")
	}
	kapananOturumlar.Store("aaa", time.Now().UnixNano())
	if !kullaniciKapatti() {
		t.Fatal("haber veren pencere kapatılmış sayılmalı")
	}
	// sayfa yenilendi: yeni oturum bağlandı, sonra o da haber vermeden sustu
	sonKopanOturum.Store("bbb")
	if kullaniciKapatti() {
		t.Fatal("yeni oturum haber vermedi; kapatılmış sayılmamalı")
	}
}

// 0.4: bekçi tarayıcıyı kendisi kapatırken o pencerenin "kapanıyorum" haberi "kullanıcı kapattı" sayılmaz
func TestBekciKendiKapattigiPencereyiSaymaz(t *testing.T) {
	kapananOturumlar.Range(func(k, _ any) bool { kapananOturumlar.Delete(k); return true })
	sonKopanOturum.Store("eski")
	yoksayilanOturum.Store("eski")
	kapananOturumlar.Store("eski", time.Now().UnixNano())
	if kullaniciKapatti() {
		t.Fatal("bekçinin kapattığı pencere kullanıcı kapatması sayılmamalı")
	}
	// yeni pencere bağlanıp sonra kullanıcı onu kapatırsa normal kural geçerli
	sonKopanOturum.Store("yeni")
	kapananOturumlar.Store("yeni", time.Now().UnixNano())
	if !kullaniciKapatti() {
		t.Fatal("kullanıcının kapattığı yeni pencere kapatılmış sayılmalı")
	}
	yoksayilanOturum.Store("")
}
