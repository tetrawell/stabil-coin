package main

// TEST VERİSİ — sadece ALSAT_TEST=1 ile çalışır; gerçek kullanımda hiç devreye girmez.
// Simülatördekinden farkı: program açık kaldıkça geçmiş mumlar değişmez, yeni mumlar zamanla eklenir
// ve son (kapanmamış) mum yavaş yavaş dolar. Böylece internet olmadan canlı izleme denenebilir.

import (
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"
)

type sahteSeri struct {
	ms            int64
	bas           int64 // ilk mumun açılış zamanı
	o, h, l, c, v []float64
	rg            *rand.Rand
	egilim        float64
}

var (
	sahteMu      sync.Mutex
	sahteSeriler = map[string]*sahteSeri{}
)

func sahteFiyat(sembol string) float64 {
	switch {
	case strings.HasPrefix(sembol, "BTC"):
		return 61000
	case strings.HasPrefix(sembol, "ETH"):
		return 2400
	case strings.HasPrefix(sembol, "STRK"):
		return 0.41
	}
	h := fnv.New32a()
	h.Write([]byte(sembol))
	return 0.05 + float64(h.Sum32()%5000)/1000
}

// i. mumu üretir (sırayla, bir kez).
func (s *sahteSeri) ekle() {
	i := len(s.c)
	o := s.c[i-1]
	if i%120 == 0 {
		s.egilim = (s.rg.Float64() - 0.5) * 0.0016
	}
	c := o * (1 + s.egilim + s.rg.NormFloat64()*0.0035)
	h := math.Max(o, c) * (1 + math.Abs(s.rg.NormFloat64())*0.0015)
	l := math.Min(o, c) * (1 - math.Abs(s.rg.NormFloat64())*0.0015)
	s.o = append(s.o, o)
	s.h = append(s.h, h)
	s.l = append(s.l, l)
	s.c = append(s.c, c)
	s.v = append(s.v, 100+s.rg.Float64()*900)
}

func testMumlari(sembol, aralik string, adet int) [][6]float64 {
	sahteMu.Lock()
	defer sahteMu.Unlock()
	sahteSonAralik[sembol] = aralik
	ms := araliklar[aralik].ms
	simdi := time.Now().UnixMilli()
	acikBas := simdi / ms * ms // kapanmamış mumun açılışı
	anahtar := sembol + "|" + aralik
	s := sahteSeriler[anahtar]
	if s == nil {
		tohum := int64(0)
		for _, ch := range anahtar {
			tohum = tohum*31 + int64(ch)
		}
		p := sahteFiyat(sembol)
		s = &sahteSeri{ms: ms, bas: acikBas - int64(enFazlaMum+500)*ms, rg: rand.New(rand.NewSource(tohum))}
		s.o, s.h, s.l, s.c, s.v = []float64{p}, []float64{p}, []float64{p}, []float64{p}, []float64{500}
		sahteSeriler[anahtar] = s
	}
	acikIdx := int((acikBas - s.bas) / ms)
	for len(s.c) <= acikIdx {
		s.ekle()
	}
	if adet > acikIdx+1 {
		adet = acikIdx + 1
	}
	out := make([][6]float64, 0, adet)
	for i := acikIdx - adet + 1; i <= acikIdx; i++ {
		t := float64(s.bas + int64(i)*ms)
		if i < acikIdx {
			out = append(out, [6]float64{t, s.o[i], s.h[i], s.l[i], s.c[i], s.v[i]})
			continue
		}
		// kapanmamış mum: geçen süre kadar dolu
		oran := math.Min(1, math.Max(0.02, float64(simdi-int64(t))/float64(ms)))
		o := s.o[i]
		c := o + (s.c[i]-o)*oran
		h := math.Max(o, c) + (s.h[i]-math.Max(o, s.c[i]))*oran
		l := math.Min(o, c) - (math.Min(o, s.c[i])-s.l[i])*oran
		out = append(out, [6]float64{t, o, h, l, c, s.v[i] * oran})
	}
	return out
}
