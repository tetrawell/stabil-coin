//go:build canli

package main

// GEÇİCİ CANLI DENEME (sadece GitHub Actions'ta, -tags canli): akış toplayıcı gerçek MEXC'ye bağlanır, birkaç dakika
// toplar; dakika çubukları MEXC'nin kendi 1 dk mumlarıyla (hacim, açılış/kapanış) karşılaştırılır, açık pozisyon
// REST ile karşılaştırılır, akış hesabı ve grafik verisi yazdırılır.

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"
)

func TestCanliMexc(t *testing.T) {
	veriKlasoru = t.TempDir()
	akisAyarDegeri.Store(akisVarsayilan())
	semboller := []string{"BTCUSDT", "ETHUSDT", "DOGEUSDT", "DOSUSDT", "XMRUSDT", "OLMAYANCOINUSDT"}
	for _, s := range semboller {
		akisIste(s)
	}
	akisBaslat()
	bekle := 260 * time.Second
	basla := time.Now()
	for time.Since(basla) < bekle {
		time.Sleep(20 * time.Second)
		AK.mu.Lock()
		fmt.Printf("[%3.0f sn] spot: %q vadeli: %q\n", time.Since(basla).Seconds(), AK.spotDurum, AK.vadDurum)
		for _, s := range semboller {
			c := AK.coinler[s]
			if c == nil {
				fmt.Printf("   %-16s yok\n", s)
				continue
			}
			fmt.Printf("   %-16s dk=%d açık=%d spotCanlı=%v vadCanlı=%v vadeli=%q kontrat=%g OI=%.4g fon=%.6f geri=%q hata=%q büyük=%d\n",
				s, len(c.dakikalar), len(c.acik), c.spotCanli > 0, c.vadCanli > 0, c.Vadeli, c.Kontrat, c.sonOI, c.sonFon, c.geriDurum, c.hata, len(c.buyukler))
		}
		AK.mu.Unlock()
	}
	fmt.Println("\n=== dakika çubukları ↔ MEXC 1 dk mumları ===")
	for _, s := range semboller[:5] {
		ham, _, err := mumSayfasi("mexc", s, "1m", 400, 0, 0)
		if err != nil {
			fmt.Printf("%s mum hatası: %v\n", s, err)
			continue
		}
		mum := map[int64][]float64{}
		for _, x := range ham {
			mum[int64(x[0])] = x
		}
		AK.mu.Lock()
		c := AK.coinler[s]
		var dk []akisDakika
		if c != nil {
			for _, d := range c.dakikalar {
				dk = append(dk, *d)
			}
		}
		AK.mu.Unlock()
		var canliOran, geriOran []float64
		for _, d := range dk {
			x, ok := mum[d.Z]
			if !ok || x[5] <= 0 {
				continue
			}
			oran := (d.Al + d.Sat) / x[5]
			tur := "canlı"
			if d.Geri {
				tur = "geri"
				geriOran = append(geriOran, oran)
			} else if !d.Eksik {
				canliOran = append(canliOran, oran)
			} else {
				tur = "eksik"
			}
			if d.Z >= dk[len(dk)-1].Z-6*60_000 || (d.Geri && len(geriOran) <= 3) {
				fmt.Printf("%s %s %-5s hacim biz=%.6g mexc=%.6g oran=%.4f | A %g/%g K %g/%g Y %g/%g D %g/%g | N=%d VV=%v vadeliHacim=%.4g OI=%.6g DE=%.3f\n",
					s, time.UnixMilli(d.Z).UTC().Format("15:04"), tur, d.Al+d.Sat, x[5], oran, d.A, x[1], d.K, x[4], d.Y, x[2], d.D, x[3], d.N, d.VV, d.VAl+d.VSat, d.OI, d.DE)
			}
		}
		fmt.Printf("%s ÖZET: canlı dakika %d (hacim oranı ort %.4f, en az %.4f, en çok %.4f) · geri doldurulan %d (ort %.4f)\n", s,
			len(canliOran), ort(canliOran), enAz(canliOran), enCok(canliOran), len(geriOran), ort(geriOran))
		// alım/satım yönü: canlı (websocket) dakikalar ↔ aynı dakikaların REST aggTrades'i
		var canlilar []akisDakika
		for _, d := range dk {
			if !d.Eksik && !d.Geri && d.N > 0 {
				canlilar = append(canlilar, d)
			}
		}
		if len(canlilar) > 3 {
			canlilar = canlilar[len(canlilar)-3:]
		}
		for _, d := range canlilar {
			is, _, err := akisAggTrades(s, d.Z, d.Z+59_999, 10)
			if err != nil {
				fmt.Printf("%s aggTrades hatası: %v\n", s, err)
				continue
			}
			var al, sat float64
			for _, x := range is {
				if x.Alim {
					al += x.Adet
				} else {
					sat += x.Adet
				}
			}
			fmt.Printf("%s %s yön: websocket alım %.6g satış %.6g | REST alım %.6g satış %.6g\n", s, time.UnixMilli(d.Z).UTC().Format("15:04"), d.Al, d.Sat, al, sat)
		}
		fmt.Println()
	}
	fmt.Println("=== açık pozisyon ↔ REST ===")
	for _, s := range []string{"BTC_USDT", "ETH_USDT", "DOGE_USDT", "DOS_USDT"} {
		b, err := getJSON(akisVadeliRest + "/api/v1/contract/ticker?symbol=" + s)
		if err != nil {
			fmt.Println(s, "REST hatası:", err)
			continue
		}
		var r struct {
			Data struct {
				HoldVol     float64 `json:"holdVol"`
				FundingRate float64 `json:"fundingRate"`
				LastPrice   float64 `json:"lastPrice"`
			} `json:"data"`
		}
		_ = json.Unmarshal(b, &r)
		AK.mu.Lock()
		sp := s[:len(s)-5] + "USDT"
		var oi, fon, kontrat float64
		if c := AK.coinler[sp]; c != nil {
			oi, fon, kontrat = c.sonOI, c.sonFon, c.Kontrat
		}
		AK.mu.Unlock()
		fmt.Printf("%s REST holdVol=%g (× kontrat %g = %.6g coin) fon=%g fiyat=%g | biz OI=%.6g fon=%g\n", s, r.Data.HoldVol, kontrat, r.Data.HoldVol*kontrat, r.Data.FundingRate, r.Data.LastPrice, oi, fon)
	}
	fmt.Println("\n=== akış hesabı ===")
	for _, s := range semboller[:5] {
		for _, ar := range []string{"1m", "5m", "15m"} {
			h, err := akisDegerlendir(s, ar)
			if err != nil {
				fmt.Printf("%s %s hata: %v\n", s, ar, err)
				continue
			}
			r := h.sonuc
			fmt.Printf("%s %s → %s | bağlam %s | durum %q\n", s, ar, r.Ozet, r.Baglam, r.Durum)
			for _, b := range r.Bloklar {
				if b.Var {
					var m []string
					for _, x := range b.Maddeler {
						m = append(m, strconv.FormatFloat(x.P, 'f', 0, 64)+" "+x.M)
					}
					fmt.Printf("      %-14s %+4.0f  %v\n", b.Ad, b.Puan, m)
				} else {
					fmt.Printf("      %-14s  yok  (%s)\n", b.Ad, b.Neden)
				}
			}
			if ar == "1m" {
				g := akisGrafikHazirla(h, akisAyarAl())
				veri := 0
				for _, x := range g.Delta {
					if x != nil {
						veri++
					}
				}
				fmt.Printf("      grafik: %d mum (%d'inde akış), salınım %d, kırılım %d, bölge %d, eşit %d, süpürme %d, işaret %d, büyük %d, satırBp %d\n",
					len(g.Z), veri, len(g.Salinimlar), len(g.Kirilimlar), len(g.Bolgeler), len(g.Esitler), len(g.Supurmeler), len(g.Isaretler), len(g.Buyukler), g.SatirBp)
				if n := len(g.Z); n >= 2 {
					fp := footprintHazirla(h, g.Z[n-2], akisAyarAl())
					fmt.Printf("      footprint (son kapanan mum): %d satır, POC %g, delta %.4g, hacim %.4g\n", len(fp.Satirlar), fp.POC, fp.Delta, fp.Hacim)
					for _, sat := range fp.Satirlar {
						fmt.Printf("         %12.6g  satış %10.4g  alım %10.4g  %v %v\n", sat.Fiyat, sat.Sat, sat.Al, sat.SatimDeng, sat.AlimDeng)
					}
				}
			}
		}
	}
}

func ort(a []float64) float64 {
	if len(a) == 0 {
		return math.NaN()
	}
	t := 0.0
	for _, x := range a {
		t += x
	}
	return t / float64(len(a))
}
func enAz(a []float64) float64 {
	m := math.Inf(1)
	for _, x := range a {
		m = math.Min(m, x)
	}
	return m
}
func enCok(a []float64) float64 {
	m := math.Inf(-1)
	for _, x := range a {
		m = math.Max(m, x)
	}
	return m
}
