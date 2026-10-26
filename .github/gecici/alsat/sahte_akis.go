package main

// TEST VERİSİ (0.9) — sadece ALSAT_TEST=1 ile: akış onayının ekranı internet olmadan denenebilsin diye sahte mumlarla
// uyumlu uydurma işlem geçmişi (son 6 saat) ve vadeli verisi. Gerçek kullanımda hiç çağrılmaz (akisGeriDoldur).

import (
	"math"
	"math/rand"
)

func akisSahteGeri(sembol string) {
	if !testModu {
		return
	}
	sahteMu.Lock()
	aralik := sahteSonAralik[sembol]
	sahteMu.Unlock()
	if aralik == "" {
		aralik = "1m"
	}
	testMumlari(sembol, aralik, 2) // seri kurulsun
	AK.mu.Lock()
	c := AK.coinler[sembol]
	if c == nil || c.ilkKayit == 0 {
		AK.mu.Unlock()
		return
	}
	bitis := c.ilkKayit / 60_000 * 60_000
	AK.mu.Unlock()
	bas := bitis - 360*60_000

	type mum struct {
		z          int64
		o, h, l, c float64
	}
	var mumlar []mum
	sahteMu.Lock()
	s := sahteSeriler[sembol+"|"+aralik]
	if s != nil {
		for i := range s.c {
			z := s.bas + int64(i)*s.ms
			if z+s.ms <= bas || z >= bitis {
				continue
			}
			mumlar = append(mumlar, mum{z, s.o[i], s.h[i], s.l[i], s.c[i]})
		}
	}
	ms := int64(60_000)
	if s != nil {
		ms = s.ms
	}
	sahteMu.Unlock()
	rg := rand.New(rand.NewSource(int64(len(sembol)) * 7919))
	var islemler []akisIslem
	adim := max(int64(2000), ms/40)
	for _, m := range mumlar {
		// mumun içinde fiyat: açılış → (düşüşteyse önce tepe) → dip/tepe → kapanış
		yol := []float64{m.o, m.l, m.h, m.c}
		if m.c < m.o {
			yol = []float64{m.o, m.h, m.l, m.c}
		}
		for t := m.z; t < m.z+ms; t += adim {
			if t < bas || t >= bitis {
				continue
			}
			oran := float64(t-m.z) / float64(ms) * 3
			p := int(math.Min(2, math.Floor(oran)))
			kesir := oran - float64(p)
			f := yol[p] + (yol[p+1]-yol[p])*kesir
			yon := 0.0
			if yol[p+1] > yol[p] {
				yon = 0.22
			} else if yol[p+1] < yol[p] {
				yon = -0.22
			}
			for j := 0; j < 1+rg.Intn(3); j++ {
				adet := (20 + rg.ExpFloat64()*80) / f
				if rg.Float64() < 0.004 {
					adet *= 80 // ara sıra büyük işlem
				}
				islemler = append(islemler, akisIslem{Z: t / 1000 * 1000, Fiyat: f * (1 + (rg.Float64()-0.5)*0.0006), Adet: adet, Alim: rg.Float64() < 0.5+yon})
			}
		}
	}
	AK.mu.Lock()
	defer AK.mu.Unlock()
	eklenen := akisGeriEkle(c, islemler, bas, bitis)
	// vadeli: hacim spotun ~3 katı, açık pozisyon fiyatla birlikte rastgele yürür
	oi := 1_000_000.0
	if len(mumlar) > 0 {
		oi = 1_000_000 / mumlar[0].c
	}
	for _, d := range eklenen {
		d.VV = true
		d.VAl, d.VSat = d.Al*(2.5+rg.Float64()), d.Sat*(2.5+rg.Float64())
		degisim := (d.Al - d.Sat) / math.Max(1e-9, d.Al+d.Sat)
		a := oi
		oi *= 1 + (rg.Float64()-0.5)*0.002 + degisim*0.001
		d.OIA, d.OIY, d.OI, d.Fon = math.Min(a, oi), math.Max(a, oi), oi, 0.0001
	}
	if c.Vadeli == "" {
		c.Vadeli, c.Kontrat = vadeliAdi(sembol), 1
	}
	c.geriDurum = ""
}
