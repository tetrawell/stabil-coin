package main

// go test ./... — MEXC sayfalamasını sahte bir MEXC sunucusuyla dener (Kripto Simülatör 3.2 testinin aynısı).
// Gerçek MEXC tek seferde en fazla 500 mum verir ve endTime hariç tutulur.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"testing"
	"time"
)

type sahteMexc struct {
	sinir    int    // bir yanıtta en fazla kaç mum
	taraf    string // "eski": pencere taşarsa startTime'dan itibaren; "yeni": pencerenin en yenileri
	dahil    bool   // endTime dahil mi (gerçek MEXC: hayır)
	toplam   int    // borsadaki mum sayısı
	bosA     int    // en yeniden bosA..bosB mum geride olanlar yok (kesinti); bosB < bosA ise yok
	bosB     int
	yoksay   bool // startTime/endTime yok sayılır
	istekSay int
}

func (f *sahteMexc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.istekSay++
	q := r.URL.Query()
	ms := int64(60_000)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	if limit > f.sinir {
		limit = f.sinir
	}
	sonAcilis := time.Now().UnixMilli() / ms * ms
	var ts []int64
	for k := f.toplam - 1; k >= 0; k-- {
		if k >= f.bosA && k <= f.bosB {
			continue
		}
		ts = append(ts, sonAcilis-int64(k)*ms)
	}
	var sec []int64
	st, et := q.Get("startTime"), q.Get("endTime")
	if f.yoksay || (st == "" && et == "") {
		sec = ts[max(0, len(ts)-limit):]
	} else {
		s, _ := strconv.ParseInt(st, 10, 64)
		e, _ := strconv.ParseInt(et, 10, 64)
		var pen []int64
		for _, t := range ts {
			if t >= s && (t < e || (f.dahil && t == e)) {
				pen = append(pen, t)
			}
		}
		if f.taraf == "eski" {
			sec = pen[:min(limit, len(pen))]
		} else {
			sec = pen[max(0, len(pen)-limit):]
		}
	}
	out := make([][]any, 0, len(sec))
	for _, t := range sec {
		out = append(out, []any{t, "1.0", "1.1", "0.9", "1.05", "10", t + ms - 1, "10.5"})
	}
	_ = json.NewEncoder(w).Encode(out)
}

func TestMexcSayfalama(t *testing.T) {
	durumlar := []struct {
		ad        string
		f         sahteMexc
		adet      int
		bekle     int
		bosluklar []int // beklenen boşluk uzunlukları (mum)
		enCokIstk int
	}{
		{"gerçek MEXC davranışı", sahteMexc{sinir: 500, taraf: "eski", toplam: 100000, bosA: 1, bosB: 0}, 5000, 5000, nil, 10},
		{"eski taraf kesilirse", sahteMexc{sinir: 500, taraf: "yeni", toplam: 100000, bosA: 1, bosB: 0}, 5000, 5000, nil, 10},
		{"sınır 300, yeni taraf kesilirse", sahteMexc{sinir: 300, taraf: "eski", toplam: 100000, bosA: 1, bosB: 0}, 5000, 5000, nil, 30},
		{"sınır 300, eski taraf kesilirse", sahteMexc{sinir: 300, taraf: "yeni", toplam: 100000, bosA: 1, bosB: 0}, 5000, 5000, nil, 30},
		{"bitiş dahil", sahteMexc{sinir: 500, taraf: "yeni", dahil: true, toplam: 100000, bosA: 1, bosB: 0}, 5000, 5000, nil, 20},
		{"yeni listelenmiş coin", sahteMexc{sinir: 500, taraf: "eski", toplam: 1234, bosA: 1, bosB: 0}, 5000, 1234, nil, 6},
		{"300 mumluk kesinti", sahteMexc{sinir: 500, taraf: "eski", toplam: 100000, bosA: 2000, bosB: 2299}, 5000, 5000, []int{300}, 14},
		{"700 mumluk kesinti", sahteMexc{sinir: 500, taraf: "eski", toplam: 100000, bosA: 2000, bosB: 2699}, 5000, 5000, []int{700}, 14},
		{"tarih yok sayılırsa sonsuz döngü yok", sahteMexc{sinir: 500, taraf: "eski", toplam: 100000, bosA: 1, bosB: 0, yoksay: true}, 5000, 500, nil, 3},
	}
	eskiAdres := mexcAdresi
	defer func() { mexcAdresi = eskiAdres }()
	for _, d := range durumlar {
		t.Run(d.ad, func(t *testing.T) {
			f := d.f
			sv := httptest.NewServer(&f)
			defer sv.Close()
			mexcAdresi = sv.URL
			hepsi, _, err := mumlariTopla("mexc", "BTCUSDT", "1m", d.adet, 0)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(hepsi, func(i, j int) bool { return hepsi[i][0] < hepsi[j][0] })
			var temiz [][]float64
			for _, x := range hepsi {
				if len(temiz) > 0 && temiz[len(temiz)-1][0] == x[0] {
					continue
				}
				temiz = append(temiz, x)
			}
			if len(temiz) > d.adet {
				temiz = temiz[len(temiz)-d.adet:]
			}
			if len(temiz) != d.bekle {
				t.Fatalf("%d mum geldi, %d bekleniyordu", len(temiz), d.bekle)
			}
			var bosluk []int
			for i := 1; i < len(temiz); i++ {
				if fark := int((temiz[i][0]-temiz[i-1][0])/60_000) - 1; fark != 0 {
					bosluk = append(bosluk, fark)
				}
			}
			if len(bosluk) != len(d.bosluklar) || (len(bosluk) > 0 && bosluk[0] != d.bosluklar[0]) {
				t.Fatalf("boşluklar %v, beklenen %v", bosluk, d.bosluklar)
			}
			if son := int64(temiz[len(temiz)-1][0]); son != time.Now().UnixMilli()/60_000*60_000 && son != (time.Now().UnixMilli()/60_000-1)*60_000 {
				t.Fatalf("son mum güncel değil")
			}
			if f.istekSay > d.enCokIstk {
				t.Fatalf("%d istek yapıldı, en çok %d bekleniyordu", f.istekSay, d.enCokIstk)
			}
		})
	}
}
