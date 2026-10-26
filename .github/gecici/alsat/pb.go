package main

// MEXC CANLI AKIŞ ÇÖZÜCÜLERİ (0.9) — Akış onayı (akis_*.go) için.
//
// Spot (wss://wbs-api.mexc.com/ws): veriler protobuf (ikili) gelir. Şemalar MEXC'nin resmî deposunda:
// github.com/mexcdevelop/websocket-proto (PushDataV3ApiWrapper.proto ve diğerleri). Dış bağımlılık olmasın diye
// burada protobuf'un sadece gereken kadarı var: alanları sırayla gezmek (varint, 64/32 bit, uzunluklu).
//   PushDataV3ApiWrapper: 1 channel (metin), 3 symbol, 6 sendTime, gövde (oneof): 303 publicLimitDepths,
//   314 publicAggreDeals.
//   PublicAggreDealsV3Api: 1 deals (tekrarlı), 2 eventType. Öğe: 1 price (metin), 2 quantity (metin),
//   3 tradeType (1 = alıcı saldırdı / alış, 2 = satıcı saldırdı / satış), 4 time (ms), 5 tradeId.
//   (26 Ekim 2026'da gerçek kayıtla doğrulandı: aynı işlemler REST aggTrades'te tradeType 2 ↔ isBuyerMaker=true.)
//   PublicLimitDepthsV3Api: 1 asks, 2 bids (öğe: 1 price, 2 quantity), 3 eventType, 4 version.
// Kontrol mesajları (abonelik onayı, PONG) metin olarak gelir: {"id":0,"code":0,"msg":"PONG"}.
//
// Vadeli (wss://contract.mexc.com/edge): JSON. push.deal → data: [{p, v (kontrat), T (1 alış, 2 satış), t (ms)}],
// push.ticker → data: {holdVol (açık pozisyon, kontrat), fundingRate, lastPrice, timestamp}. Kontrat → coin:
// × contractSize (/api/v1/contract/detail).

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Tek bir işlem (spot ya da vadeli). Miktar coin cinsinden (vadelide kontrat × kontrat boyu).
type akisIslem struct {
	Z     int64   // zaman (ms)
	Fiyat float64 //
	Adet  float64 // coin
	Alim  bool    // alıcı mı saldırdı (piyasa emriyle aldı)
}

// Defterin bir anlık görüntüsü (en iyi N basamak).
type defterAnlik struct {
	Z     int64
	Alis  [][2]float64 // alıcılar (yüksekten düşüğe): fiyat, miktar
	Satis [][2]float64 // satıcılar (düşükten yükseğe)
}

type mexcPaket struct {
	Kanal    string
	Sembol   string
	Zaman    int64
	Islemler []akisIslem
	Defter   *defterAnlik
}

var pbKisa = errors.New("protobuf: mesaj yarıda kesik")

func pbVarint(b []byte, i int) (uint64, int, error) {
	var x uint64
	for s := uint(0); s < 64; s += 7 {
		if i >= len(b) {
			return 0, i, pbKisa
		}
		c := b[i]
		i++
		x |= uint64(c&0x7F) << s
		if c < 0x80 {
			return x, i, nil
		}
	}
	return 0, i, errors.New("protobuf: varint çok uzun")
}

// pbGez: mesajın alanlarını sırayla verir. tur 0 → sayi, tur 2 → veri; 1 ve 5 atlanır (veri ham baytlar).
func pbGez(b []byte, f func(no int, tur int, sayi uint64, veri []byte) error) error {
	i := 0
	for i < len(b) {
		k, j, err := pbVarint(b, i)
		if err != nil {
			return err
		}
		i = j
		no, tur := int(k>>3), int(k&7)
		if no == 0 {
			return errors.New("protobuf: alan numarası 0")
		}
		switch tur {
		case 0:
			v, j, err := pbVarint(b, i)
			if err != nil {
				return err
			}
			i = j
			if err := f(no, tur, v, nil); err != nil {
				return err
			}
		case 1:
			if i+8 > len(b) {
				return pbKisa
			}
			if err := f(no, tur, 0, b[i:i+8]); err != nil {
				return err
			}
			i += 8
		case 2:
			n, j, err := pbVarint(b, i)
			if err != nil {
				return err
			}
			i = j
			if n > uint64(len(b)-i) {
				return pbKisa
			}
			if err := f(no, tur, 0, b[i:i+int(n)]); err != nil {
				return err
			}
			i += int(n)
		case 5:
			if i+4 > len(b) {
				return pbKisa
			}
			if err := f(no, tur, 0, b[i:i+4]); err != nil {
				return err
			}
			i += 4
		default:
			return fmt.Errorf("protobuf: desteklenmeyen tür %d", tur)
		}
	}
	return nil
}

func pbSayi(veri []byte) float64 {
	f, err := strconv.ParseFloat(string(veri), 64)
	if err != nil {
		return 0
	}
	return f
}

// mexcCoz: spot websocket'in ikili mesajını çözer.
func mexcCoz(b []byte) (mexcPaket, error) {
	var p mexcPaket
	err := pbGez(b, func(no, tur int, sayi uint64, veri []byte) error {
		switch {
		case no == 1 && tur == 2:
			p.Kanal = string(veri)
		case no == 3 && tur == 2:
			p.Sembol = string(veri)
		case no == 6 && tur == 0:
			p.Zaman = int64(sayi)
		case no == 314 && tur == 2:
			return pbGez(veri, func(no2, tur2 int, _ uint64, oge []byte) error {
				if no2 != 1 || tur2 != 2 {
					return nil
				}
				var is akisIslem
				var tip uint64
				if err := pbGez(oge, func(no3, tur3 int, s3 uint64, v3 []byte) error {
					switch {
					case no3 == 1 && tur3 == 2:
						is.Fiyat = pbSayi(v3)
					case no3 == 2 && tur3 == 2:
						is.Adet = pbSayi(v3)
					case no3 == 3 && tur3 == 0:
						tip = s3
					case no3 == 4 && tur3 == 0:
						is.Z = int64(s3)
					}
					return nil
				}); err != nil {
					return err
				}
				if is.Fiyat > 0 && is.Adet > 0 && (tip == 1 || tip == 2) {
					is.Alim = tip == 1
					p.Islemler = append(p.Islemler, is)
				}
				return nil
			})
		case no == 303 && tur == 2:
			d := &defterAnlik{}
			if err := pbGez(veri, func(no2, tur2 int, _ uint64, oge []byte) error {
				if (no2 != 1 && no2 != 2) || tur2 != 2 {
					return nil
				}
				var f, m float64
				if err := pbGez(oge, func(no3, tur3 int, _ uint64, v3 []byte) error {
					if tur3 == 2 && no3 == 1 {
						f = pbSayi(v3)
					} else if tur3 == 2 && no3 == 2 {
						m = pbSayi(v3)
					}
					return nil
				}); err != nil {
					return err
				}
				if f > 0 && m >= 0 {
					if no2 == 1 {
						d.Satis = append(d.Satis, [2]float64{f, m})
					} else {
						d.Alis = append(d.Alis, [2]float64{f, m})
					}
				}
				return nil
			}); err != nil {
				return err
			}
			p.Defter = d
		}
		return nil
	})
	if err != nil {
		return p, err
	}
	if p.Defter != nil {
		p.Defter.Z = p.Zaman
	}
	if p.Sembol == "" {
		// sembol alanı yoksa kanal adının sonundan (…@100ms@BTCUSDT ya da …@BTCUSDT@20)
		parcalar := strings.Split(p.Kanal, "@")
		for i := len(parcalar) - 1; i >= 0; i-- {
			if sembolKalip.MatchString(parcalar[i]) && strings.HasSuffix(parcalar[i], "USDT") {
				p.Sembol = parcalar[i]
				break
			}
		}
	}
	return p, nil
}

// spotKontrol: metin mesajı bir abonelik onayı / PONG mu, yoksa hata mı?
func spotKontrol(metin []byte) (tamam bool, mesaj string) {
	var j struct {
		ID   int    `json:"id"`
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(metin, &j); err != nil {
		return false, strings.TrimSpace(string(metin))
	}
	return j.Code == 0, j.Msg
}

// ---------- vadeli (JSON) ----------

type vadeliTicker struct {
	Z     int64
	Fiyat float64
	AcikP float64 // açık pozisyon (kontrat)
	Fon   float64 // fonlama oranı (ör. 0.0001 = %0,01)
}

type vadeliPaket struct {
	Kanal    string
	Sembol   string      // BTC_USDT
	Islemler []akisIslem // Adet kontrat cinsinden (çağıran kontrat boyuyla çarpar)
	Ticker   *vadeliTicker
}

func vadeliCoz(metin []byte) (vadeliPaket, error) {
	var ham struct {
		Channel string          `json:"channel"`
		Symbol  string          `json:"symbol"`
		Data    json.RawMessage `json:"data"`
		Ts      int64           `json:"ts"`
	}
	var p vadeliPaket
	if err := json.Unmarshal(metin, &ham); err != nil {
		return p, err
	}
	p.Kanal, p.Sembol = ham.Channel, ham.Symbol
	switch ham.Channel {
	case "push.deal":
		type islem struct {
			P float64 `json:"p"`
			V float64 `json:"v"`
			T int     `json:"T"`
			Z int64   `json:"t"`
		}
		var liste []islem
		if len(ham.Data) > 0 && ham.Data[0] == '{' {
			var tek islem
			if err := json.Unmarshal(ham.Data, &tek); err != nil {
				return p, err
			}
			liste = []islem{tek}
		} else if err := json.Unmarshal(ham.Data, &liste); err != nil {
			return p, err
		}
		for _, x := range liste {
			if x.P > 0 && x.V > 0 && (x.T == 1 || x.T == 2) {
				p.Islemler = append(p.Islemler, akisIslem{Z: x.Z, Fiyat: x.P, Adet: x.V, Alim: x.T == 1})
			}
		}
	case "push.ticker":
		var t struct {
			Symbol      string  `json:"symbol"`
			LastPrice   float64 `json:"lastPrice"`
			HoldVol     float64 `json:"holdVol"`
			FundingRate float64 `json:"fundingRate"`
			Timestamp   int64   `json:"timestamp"`
		}
		if err := json.Unmarshal(ham.Data, &t); err != nil {
			return p, err
		}
		if p.Sembol == "" {
			p.Sembol = t.Symbol
		}
		z := t.Timestamp
		if z == 0 {
			z = ham.Ts
		}
		p.Ticker = &vadeliTicker{Z: z, Fiyat: t.LastPrice, AcikP: t.HoldVol, Fon: t.FundingRate}
	}
	return p, nil
}

// spot sembolü (BTCUSDT) → vadeli sembolü (BTC_USDT)
func vadeliAdi(sembol string) string {
	if !strings.HasSuffix(sembol, "USDT") || len(sembol) <= 4 {
		return ""
	}
	return strings.TrimSuffix(sembol, "USDT") + "_USDT"
}
