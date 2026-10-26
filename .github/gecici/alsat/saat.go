package main

// BORSA SAATİ — mumların zamanı MEXC'nin saatiyle gelir. Bilgisayarın saati MEXC'den farklıysa
// (ör. 40 sn ileri) mum kapanışı yanlış anda beklenir ve taze alım kararı "geç kalmış" sanılabilir.
// Program MEXC'nin saatini 10 dakikada bir sorar; fark hem burada hem sayfada hesaba katılır.

import (
	"encoding/json"
	"log"
	"sync/atomic"
	"time"
)

var borsaFarki atomic.Int64 // MEXC saati − bilgisayar saati (ms)

func borsaSaatiOlc() {
	if testModu {
		return
	}
	en := int64(-1)
	var fark int64
	for deneme := 0; deneme < 3; deneme++ {
		once := time.Now().UnixMilli()
		b, err := getJSON(mexcAdresi + "/api/v3/time")
		sonra := time.Now().UnixMilli()
		if err != nil {
			continue
		}
		var r struct {
			ServerTime int64 `json:"serverTime"`
		}
		if json.Unmarshal(b, &r) != nil || r.ServerTime == 0 {
			continue
		}
		if gidis := sonra - once; en < 0 || gidis < en { // en kısa gidiş-dönüş en doğru ölçümdür
			en, fark = gidis, r.ServerTime-(once+sonra)/2
		}
	}
	if en < 0 {
		return
	}
	eski := borsaFarki.Swap(fark)
	if (fark > 2000 || fark < -2000) && (eski-fark > 1000 || fark-eski > 1000) {
		log.Printf("Bilgisayarın saati MEXC'den %.1f sn farklı; program bunu hesaba katıyor", float64(fark)/1000)
	}
}

func borsaSaatiDongusu() {
	for {
		borsaSaatiOlc()
		uyu(10 * time.Minute)
	}
}
