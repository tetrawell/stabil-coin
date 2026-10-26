package main

// TEST PİYASASI — sadece ALSAT_TEST=1 ile: sahte mumlardan (sahte_veri.go) türetilen alış-satış fiyatı, son işlemler,
// emir defteri ve coin kuralları. Sanal al-sat internet olmadan denenebilsin diye. Gerçek kullanımda devreye girmez.

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"time"
)

type sahtePiyasa struct{}

var sahteSonAralik = map[string]string{} // sahteMu ile korunur: coin için en son istenen mum süresi

// t anındaki fiyat: o anki mumun açılışından kapanışına doğru doğrusal yol (grafikteki son mumla uyumlu)
func sahteFiyatAn(sembol string, t int64) float64 {
	sahteMu.Lock()
	aralik := sahteSonAralik[sembol]
	sahteMu.Unlock()
	if aralik == "" {
		aralik = "1m"
	}
	testMumlari(sembol, aralik, 2) // seri yoksa kurulsun, bugüne kadar üretilsin
	sahteMu.Lock()
	defer sahteMu.Unlock()
	s := sahteSeriler[sembol+"|"+aralik]
	if s == nil {
		return sahteFiyat(sembol)
	}
	i := int((t - s.bas) / s.ms)
	if i < 0 {
		i = 0
	}
	for len(s.c) <= i {
		s.ekle()
	}
	oran := math.Min(1, math.Max(0, float64(t-(s.bas+int64(i)*s.ms))/float64(s.ms)))
	return s.o[i] + (s.c[i]-s.o[i])*oran
}

func (sahtePiyasa) Kurallar(sembol string) (sembolKurali, error) {
	p := sahteFiyat(sembol)
	k := sembolKurali{Sembol: sembol, Varlik: sembol[:len(sembol)-4], Acik: true, TutarOndalik: 2, EnAzTutar: 1, EnAzTutarPiyasa: 1,
		Maker: 0, Taker: 0.0005, Turler: []string{"LIMIT", "MARKET", "LIMIT_MAKER"}}
	switch {
	case p >= 1000:
		k.FiyatOndalik, k.MiktarOndalik = 2, 6
	case p >= 10:
		k.FiyatOndalik, k.MiktarOndalik = 3, 4
	case p >= 1:
		k.FiyatOndalik, k.MiktarOndalik = 4, 2
	default:
		k.FiyatOndalik, k.MiktarOndalik = 6, 1
	}
	k.EnAzMiktar = k.adim()
	return k, nil
}

func (sp sahtePiyasa) ustDefter(sembol string, t int64) (defterUstu, sembolKurali) {
	k, _ := sp.Kurallar(sembol)
	p := sahteFiyatAn(sembol, t)
	tik := k.tik()
	aralik := math.Max(2*tik, p*0.0003)
	bid := math.Floor((p-aralik/2)/tik) * tik
	ask := bid + math.Max(tik, math.Round(aralik/tik)*tik)
	return defterUstu{Bid: yakinYuvarla(bid, k.FiyatOndalik), BidMiktar: 400 / p, Ask: yakinYuvarla(ask, k.FiyatOndalik), AskMiktar: 400 / p}, k
}

func (sp sahtePiyasa) EnIyiFiyat(sembol string) (defterUstu, error) {
	du, _ := sp.ustDefter(sembol, time.Now().UnixMilli())
	return du, nil
}

func (sahtePiyasa) SonFiyat(sembol string) (float64, error) {
	return sahteFiyatAn(sembol, time.Now().UnixMilli()), nil
}

// Her saniye bir işlem: %35 piyasadan satış (alış fiyatına çarpar), %65 piyasadan alış
func (sp sahtePiyasa) SonIslemler(sembol string, adet int) ([]islemKaydi, error) {
	simdi := time.Now().UnixMilli() / 1000
	h := fnv.New32a()
	h.Write([]byte(sembol))
	tohum := int64(h.Sum32())
	var out []islemKaydi
	for sn := simdi - int64(min(adet, 120)); sn <= simdi; sn++ {
		rg := rand.New(rand.NewSource(tohum*1_000_003 + sn))
		t := sn*1000 + int64(rg.Intn(1000))
		du, _ := sp.ustDefter(sembol, t)
		satis := rg.Float64() < 0.35
		f := du.Ask
		if satis {
			f = du.Bid
		}
		out = append(out, islemKaydi{Fiyat: f, Miktar: (20 + rg.Float64()*300) / f, Zaman: t, AliciMaker: satis})
	}
	return out, nil
}

func (sp sahtePiyasa) Derinlik(sembol string, adet int) (defter, error) {
	du, k := sp.ustDefter(sembol, time.Now().UnixMilli())
	tik := k.tik()
	var d defter
	for i := 0; i < min(adet, 20); i++ {
		m := (300 + float64(i)*150) / du.Ask
		d.Satis = append(d.Satis, defterSeviye{yakinYuvarla(du.Ask+float64(i)*tik, k.FiyatOndalik), m})
		d.Alis = append(d.Alis, defterSeviye{yakinYuvarla(du.Bid-float64(i)*tik, k.FiyatOndalik), m})
	}
	if len(d.Satis) == 0 {
		return d, fmt.Errorf("defter boş")
	}
	return d, nil
}
