package main

// SANAL BORSA — gerçek MEXC verisiyle, gerçek para kullanmadan emirleri taklit eder.
//
// Limit emir: emir verildiği andaki alış-satış sırasına bakılır. Sıranın başına geçtiysek (aralığın içine fiyat
// koyduysak) önümüzde kimse yok sayılır; mevcut en iyi fiyata eklendiysek o fiyatta bekleyen miktar önümüzdedir.
// Sonra MEXC'nin son işlemlerine bakılır: alış emri için birinin piyasadan SATTIĞI ve fiyatın bizim fiyatımıza
// indiği işlemler sayılır. Önümüzdeki miktar bittikçe bizim emrimiz dolar. Fiyat bizim fiyatımızın da altına
// inerse emrin tamamı dolmuş sayılır. (Satış emri için tersi.) Limit dolumda MEXC'nin maker komisyonu uygulanır.
//
// Piyasa emri: MEXC'nin emir defterinde fiyat basamakları tek tek tüketilir (büyük tutarda fiyat kayar).
// Taker komisyonu uygulanır: alışta coinden, satışta USDT'den kesilir (MEXC de böyle yapar).

import (
	"fmt"
	"math"
	"strconv"
	"sync"
)

type sanalEmir struct {
	ID, IstemciID, Taraf string
	Sembol               string
	Fiyat, Miktar        float64
	Yer                  int64   // verildiği an (ms)
	Onunde               float64 // önümüzdeki sıra (coin)
	Hacim                float64 // verildikten sonra bizim fiyatımızda gerçekleşen karşı taraf hacmi
	Dolan, DolanTutar    float64
	Durum                string
	Parcalar             []parca
	gorulen              map[string]bool
}

type sanalBorsa struct {
	p       piyasaKaynagi
	mu      sync.Mutex
	emirler map[string]*sanalEmir
	sayac   int64
}

func yeniSanalBorsa(p piyasaKaynagi) *sanalBorsa {
	return &sanalBorsa{p: p, emirler: map[string]*sanalEmir{}}
}

func (s *sanalBorsa) Ad() string { return "sanal" }

// s.mu kilitliyken çağrılır. Eski emirleri bellekten atar.
func (s *sanalBorsa) yeniID() string {
	s.sayac++
	if s.sayac%50 == 0 {
		esik := simdiF().UnixMilli() - 2*3600*1000
		for id, e := range s.emirler {
			if e.Yer < esik {
				delete(s.emirler, id)
			}
		}
	}
	return "S" + strconv.FormatInt(s.sayac, 10)
}

func (s *sanalBorsa) LimitEmir(k sembolKurali, taraf string, fiyat, miktar float64, istemciID string) (string, error) {
	du, err := s.p.EnIyiFiyat(k.Sembol)
	if err != nil {
		return "", err
	}
	e := &sanalEmir{IstemciID: istemciID, Taraf: taraf, Sembol: k.Sembol, Fiyat: fiyat, Miktar: miktar, Yer: simdiF().UnixMilli(), Durum: "NEW", gorulen: map[string]bool{}}
	t := k.tik()
	if taraf == "BUY" {
		if fiyat >= du.Ask-t*0.01 {
			return "", fmt.Errorf("limit alış fiyatı satış fiyatına ulaşıyor")
		}
		switch {
		case fiyat > du.Bid+t*0.01:
			e.Onunde = 0 // sıranın başı
		case math.Abs(fiyat-du.Bid) <= t*0.01:
			e.Onunde = du.BidMiktar
		default:
			e.Onunde = du.BidMiktar * 2 // en iyi fiyatın altında: önümüzde daha çok kişi var (kaba tahmin)
		}
	} else {
		if fiyat <= du.Bid+t*0.01 {
			return "", fmt.Errorf("limit satış fiyatı alış fiyatına ulaşıyor")
		}
		switch {
		case fiyat < du.Ask-t*0.01:
			e.Onunde = 0
		case math.Abs(fiyat-du.Ask) <= t*0.01:
			e.Onunde = du.AskMiktar
		default:
			e.Onunde = du.AskMiktar * 2
		}
	}
	s.mu.Lock()
	e.ID = s.yeniID()
	s.emirler[e.ID] = e
	s.mu.Unlock()
	return e.ID, nil
}

// Son işlemlere bakıp emri günceller
func (s *sanalBorsa) guncelle(k sembolKurali, e *sanalEmir) {
	if e.Durum != "NEW" && e.Durum != "PARTIALLY_FILLED" {
		return
	}
	islemler, err := s.p.SonIslemler(k.Sembol, 500)
	if err != nil {
		return
	}
	t := k.tik()
	for _, x := range islemler {
		if x.Zaman < e.Yer {
			continue
		}
		anahtar := fmt.Sprintf("%d|%g|%g|%v", x.Zaman, x.Fiyat, x.Miktar, x.AliciMaker)
		if e.gorulen[anahtar] {
			continue
		}
		e.gorulen[anahtar] = true
		if e.Taraf == "BUY" {
			if !x.AliciMaker || x.Fiyat > e.Fiyat+t*0.01 {
				continue // bize satan olmadı
			}
			if x.Fiyat < e.Fiyat-t*0.01 {
				e.Hacim = math.Inf(1) // fiyat bizim fiyatımızın da altına indi: emrimiz kesin doldu
			} else {
				e.Hacim += x.Miktar
			}
		} else {
			if x.AliciMaker || x.Fiyat < e.Fiyat-t*0.01 {
				continue
			}
			if x.Fiyat > e.Fiyat+t*0.01 {
				e.Hacim = math.Inf(1)
			} else {
				e.Hacim += x.Miktar
			}
		}
	}
	dolan := math.Min(e.Miktar, math.Max(0, e.Hacim-e.Onunde))
	if dolan > e.Dolan+e.Miktar*1e-9 {
		fark := dolan - e.Dolan
		tutar := fark * e.Fiyat
		p := parca{Fiyat: e.Fiyat, Miktar: fark, Tutar: tutar, Maker: true, Zaman: simdiF().UnixMilli(), Kaynak: "limit"}
		if e.Taraf == "BUY" {
			p.Komisyon, p.KomisyonVarligi = fark*k.Maker, k.Varlik
		} else {
			p.Komisyon, p.KomisyonVarligi = tutar*k.Maker, "USDT"
		}
		p.KomisyonUSDT = tutar * k.Maker
		e.Parcalar = append(e.Parcalar, p)
		e.Dolan = dolan
		e.DolanTutar += tutar
	}
	if e.Dolan >= e.Miktar*(1-1e-9) {
		e.Durum = "FILLED"
	} else if e.Dolan > 0 {
		e.Durum = "PARTIALLY_FILLED"
	}
}

func (s *sanalBorsa) durumu(e *sanalEmir) emirDurumu {
	return emirDurumu{ID: e.ID, IstemciID: e.IstemciID, Durum: e.Durum, Dolan: e.Dolan, DolanTutar: e.DolanTutar}
}

func (s *sanalBorsa) bul(id string) (*sanalEmir, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.emirler[id]
	if !ok {
		return nil, fmt.Errorf("emir bulunamadı")
	}
	return e, nil
}

func (s *sanalBorsa) EmirDurumu(k sembolKurali, id string) (emirDurumu, error) {
	e, err := s.bul(id)
	if err != nil {
		return emirDurumu{}, err
	}
	s.guncelle(k, e)
	return s.durumu(e), nil
}

func (s *sanalBorsa) Iptal(k sembolKurali, id string) (emirDurumu, error) {
	e, err := s.bul(id)
	if err != nil {
		return emirDurumu{}, err
	}
	s.guncelle(k, e)
	switch e.Durum {
	case "NEW":
		e.Durum = "CANCELED"
	case "PARTIALLY_FILLED":
		e.Durum = "PARTIALLY_CANCELED"
	}
	return s.durumu(e), nil
}

// Emir defterinde basamak basamak doldurur
func (s *sanalBorsa) PiyasaEmri(k sembolKurali, taraf string, miktar, tutar float64, istemciID string) (emirDurumu, error) {
	d, err := s.p.Derinlik(k.Sembol, 100)
	if err != nil {
		return emirDurumu{}, err
	}
	e := &sanalEmir{IstemciID: istemciID, Taraf: taraf, Sembol: k.Sembol, Yer: simdiF().UnixMilli(), gorulen: map[string]bool{}}
	zaman := simdiF().UnixMilli()
	if taraf == "BUY" {
		kalan := tutar
		for _, sv := range d.Satis {
			if kalan <= 1e-9 {
				break
			}
			al := math.Min(kalan, sv.Fiyat*sv.Miktar)
			adet := al / sv.Fiyat
			e.Parcalar = append(e.Parcalar, parca{Fiyat: sv.Fiyat, Miktar: adet, Tutar: al, Komisyon: adet * k.Taker, KomisyonVarligi: k.Varlik, KomisyonUSDT: al * k.Taker, Zaman: zaman, Kaynak: "piyasa"})
			e.Dolan += adet
			e.DolanTutar += al
			kalan -= al
		}
	} else {
		kalan := miktar
		for _, sv := range d.Alis {
			if kalan <= 1e-12 {
				break
			}
			sat := math.Min(kalan, sv.Miktar)
			gelir := sat * sv.Fiyat
			e.Parcalar = append(e.Parcalar, parca{Fiyat: sv.Fiyat, Miktar: sat, Tutar: gelir, Komisyon: gelir * k.Taker, KomisyonVarligi: "USDT", KomisyonUSDT: gelir * k.Taker, Zaman: zaman, Kaynak: "piyasa"})
			e.Dolan += sat
			e.DolanTutar += gelir
			kalan -= sat
		}
	}
	if e.Dolan <= 0 {
		return emirDurumu{}, fmt.Errorf("emir defteri boş")
	}
	e.Durum = "FILLED"
	s.mu.Lock()
	e.ID = s.yeniID()
	s.emirler[e.ID] = e
	s.mu.Unlock()
	return s.durumu(e), nil
}

func (s *sanalBorsa) Dolumlar(k sembolKurali, id string) ([]parca, error) {
	e, err := s.bul(id)
	if err != nil {
		return nil, err
	}
	return append([]parca(nil), e.Parcalar...), nil
}
