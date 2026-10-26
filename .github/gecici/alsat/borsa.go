package main

// EMİRLER — sanal ve gerçek borsanın ortak dili ve emir yürütücü.
// Kullanıcının kuralı: alış ve satış önce fiyata en yakın yerden LİMİT emirle denenir
// (alışta alıcı sırasının başı, satışta satıcı sırasının başı; aralık genişse bir adım içeri).
// 15 saniyede dolmazsa emir iptal edilir, kalan kısım PİYASA emriyle tamamlanır (fırsat kaçmasın).
// Zarar durdurda (anlık izleme) beklemeden doğrudan piyasa emri verilir.

import (
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	uyu    = time.Sleep // testlerde beklemeden geçilir
	simdiF = time.Now
)

const limitBekleme = 15 * time.Second

// Bir dolum parçası (borsanın bildirdiği tek bir gerçekleşme)
type parca struct {
	Fiyat           float64 `json:"fiyat"`
	Miktar          float64 `json:"miktar"`
	Tutar           float64 `json:"tutar"`
	Komisyon        float64 `json:"komisyon"`
	KomisyonVarligi string  `json:"komisyonVarligi"`
	KomisyonUSDT    float64 `json:"komisyonUSDT"`
	Maker           bool    `json:"maker"`
	Zaman           int64   `json:"zaman"`
	Kaynak          string  `json:"kaynak"` // "limit" | "piyasa"
}

type emirDurumu struct {
	ID         string
	IstemciID  string
	Durum      string // NEW | PARTIALLY_FILLED | FILLED | CANCELED | PARTIALLY_CANCELED | REJECTED | EXPIRED
	Dolan      float64
	DolanTutar float64
}

func (e emirDurumu) bitti() bool {
	switch e.Durum {
	case "FILLED", "CANCELED", "PARTIALLY_CANCELED", "REJECTED", "EXPIRED":
		return true
	}
	return false
}

type borsa interface {
	Ad() string // "sanal" | "gercek"
	LimitEmir(k sembolKurali, taraf string, fiyat, miktar float64, istemciID string) (string, error)
	EmirDurumu(k sembolKurali, id string) (emirDurumu, error)
	Iptal(k sembolKurali, id string) (emirDurumu, error)
	// Alışta tutar (USDT), satışta miktar (coin) verilir.
	PiyasaEmri(k sembolKurali, taraf string, miktar, tutar float64, istemciID string) (emirDurumu, error)
	Dolumlar(k sembolKurali, id string) ([]parca, error)
}

// Bir alışın ya da satışın toplam sonucu
type dolumSonucu struct {
	Taraf      string  `json:"taraf"` // BUY | SELL
	Ortalama   float64 `json:"ortalama"`
	Miktar     float64 `json:"miktar"`    // brüt coin
	NetMiktar  float64 `json:"netMiktar"` // alışta: komisyon coinden kesildiyse düşülmüş
	Tutar      float64 `json:"tutar"`     // brüt USDT (alışta harcanan, satışta gelen)
	NetTutar   float64 `json:"netTutar"`  // alışta: harcanan + USDT komisyonu, satışta: gelen − USDT komisyonu
	Komisyon   float64 `json:"komisyon"`  // USDT karşılığı toplam
	Yol        string  `json:"yol"`       // limit | limit+piyasa | piyasa
	LimitFiyat float64 `json:"limitFiyat,omitempty"`
	Parcalar   []parca `json:"parcalar"`
	Baslangic  int64   `json:"baslangic"`
	Bitis      int64   `json:"bitis"`
}

func dolumTopla(taraf string, k sembolKurali, parcalar []parca) dolumSonucu {
	d := dolumSonucu{Taraf: taraf, Parcalar: parcalar}
	limit, piyasa := false, false
	var komCoin, komUSDT, komDiger float64
	for _, p := range parcalar {
		d.Miktar += p.Miktar
		d.Tutar += p.Tutar
		d.Komisyon += p.KomisyonUSDT
		switch {
		case p.KomisyonVarligi == k.Varlik && k.Varlik != "":
			komCoin += p.Komisyon
		case p.KomisyonVarligi == "USDT":
			komUSDT += p.Komisyon
		default:
			komDiger += p.KomisyonUSDT // ör. MX ile ödenen komisyon: hesaptaki başka bir coinden düşer
		}
		if p.Kaynak == "piyasa" {
			piyasa = true
		} else {
			limit = true
		}
	}
	if d.Miktar > 0 {
		d.Ortalama = d.Tutar / d.Miktar
	}
	// Başka coinle ödenen komisyon da kumbaranın hesabından düşülür (sonuç dürüst olsun; hesaptaki
	// USDT'nin bir kısmı hiçbir kumbaraya ayrılmamış kalır, bu güvenli taraftır).
	if taraf == "BUY" {
		d.NetMiktar = d.Miktar - komCoin
		d.NetTutar = d.Tutar + komUSDT + komDiger
	} else {
		d.NetMiktar = d.Miktar
		d.NetTutar = d.Tutar - komUSDT - komDiger
	}
	switch {
	case limit && piyasa:
		d.Yol = "limit+piyasa"
	case piyasa:
		d.Yol = "piyasa"
	case limit:
		d.Yol = "limit"
	}
	return d
}

// Limit fiyatı: alışta alıcı sırasının başı (aralık en az 2 adımsa bir adım içeri), satışta satıcı sırasının başı.
func limitFiyati(taraf string, du defterUstu, k sembolKurali) float64 {
	t := k.tik()
	if taraf == "BUY" {
		f := du.Bid
		if du.Bid+t < du.Ask-t*0.5 {
			f = du.Bid + t
		}
		return yakinYuvarla(f, k.FiyatOndalik)
	}
	f := du.Ask
	if du.Ask-t > du.Bid+t*0.5 {
		f = du.Ask - t
	}
	return yakinYuvarla(f, k.FiyatOndalik)
}

type olayFn func(tur, mesaj string)

// Emrin son durumu öğrenilemedi (ör. limit emir iptal edilemedi): güvenlik için piyasa emri verilmez,
// gerçek parada kumbara yarım kalan emri sonra MEXC'den sorar.
type belirsizHata struct{ mesaj string }

func (e belirsizHata) Error() string { return e.mesaj }

func yuzdeMetni(x float64) string { return fmt.Sprintf("%%%s", trSayi(x*100, 0)) }

// Türkçe sayı yazımı (ondalık virgül)
func trSayi(x float64, ondalik int) string {
	s := sayiMetni(x, ondalik)
	b := []byte(s)
	for i := range b {
		if b[i] == '.' {
			b[i] = ','
		}
	}
	return string(b)
}

func fiyatMetni(x float64, k sembolKurali) string { return trSayi(x, k.FiyatOndalik) }

// Alış (butce USDT) ya da satış (miktar coin): limit → 15 sn → piyasa
func limitSonraPiyasa(b borsa, p piyasaKaynagi, k sembolKurali, taraf string, butce, miktar float64, onek string, olay olayFn) (dolumSonucu, error) {
	basla := simdiF()
	var du defterUstu
	var err error
	for deneme := 0; deneme < 3; deneme++ {
		if du, err = p.EnIyiFiyat(k.Sembol); err == nil {
			break
		}
		uyu(time.Second)
	}
	if err != nil {
		return dolumSonucu{}, fmt.Errorf("fiyat alınamadı: %v", err)
	}
	fiyat := limitFiyati(taraf, du, k)
	var lmiktar float64
	if taraf == "BUY" {
		lmiktar = asagiYuvarla(butce/fiyat, k.MiktarOndalik)
	} else {
		lmiktar = asagiYuvarla(miktar, k.MiktarOndalik)
	}
	if lmiktar < k.EnAzMiktar || lmiktar*fiyat < k.EnAzTutar {
		// limit için çok küçük; piyasa emri yeterliyse onunla dene
		olay("bilgi", "Tutar limit emir için çok küçük, doğrudan piyasa emri veriliyor")
		return dogrudanPiyasa(b, p, k, taraf, butce, miktar, onek, olay)
	}
	tarafAd := map[string]string{"BUY": "alış", "SELL": "satış"}[taraf]
	sira := map[string]string{"BUY": "alıcı", "SELL": "satıcı"}[taraf]
	id, err := b.LimitEmir(k, taraf, fiyat, lmiktar, onek+"L")
	if err != nil {
		var bh belirsizHata
		if errors.As(err, &bh) {
			// 0.8: limit emir MEXC'ye ulaşmış olabilir; üstüne piyasa emri verilirse iki kez alınıp satılabilir
			return dolumSonucu{}, err
		}
		olay("uyari", fmt.Sprintf("Limit %s verilemedi (%v); piyasa emri deneniyor", tarafAd, err))
		return dogrudanPiyasa(b, p, k, taraf, butce, miktar, onek, olay)
	}
	olay("bilgi", fmt.Sprintf("Limit %s verildi: %s adet × %s (%s sırasının başı). 15 sn bekleniyor", tarafAd, trSayi(lmiktar, k.MiktarOndalik), fiyatMetni(fiyat, k), sira))
	var son emirDurumu
	for simdiF().Sub(basla) < limitBekleme {
		uyu(time.Second)
		d, err := b.EmirDurumu(k, id)
		if err != nil {
			continue
		}
		son = d
		if d.Durum == "FILLED" || d.bitti() {
			break
		}
	}
	if son.Durum != "FILLED" {
		// iptal edilip edilmediği kesin bilinmeden piyasa emri verilmez (yoksa iki kez alınabilir)
		kesin := false
		for deneme := 0; deneme < 4 && !kesin; deneme++ {
			if d, err := b.Iptal(k, id); err == nil && d.bitti() {
				son, kesin = d, true
				break
			}
			if d, err := b.EmirDurumu(k, id); err == nil {
				son = d
				if d.bitti() {
					kesin = true
					break
				}
			}
			uyu(time.Second)
		}
		if !kesin {
			return dolumSonucu{}, belirsizHata{"limit emir iptal edilemedi; güvenlik için piyasa emri verilmedi"}
		}
	}
	parcalar, err := limitDolumlari(b, k, id, son)
	if err != nil {
		olay("uyari", "Limit emrin dolumları okunamadı: "+err.Error())
	}
	for i := range parcalar {
		parcalar[i].Kaynak = "limit"
	}
	if son.Durum == "FILLED" {
		olay("bilgi", fmt.Sprintf("Limit %s doldu", tarafAd))
		d := dolumTopla(taraf, k, parcalar)
		d.LimitFiyat, d.Baslangic, d.Bitis = fiyat, basla.UnixMilli(), simdiF().UnixMilli()
		return d, nil
	}
	// kalan kısım piyasa emriyle
	var dolanTutar, dolanMiktar float64
	for _, x := range parcalar {
		dolanTutar += x.Tutar
		dolanMiktar += x.Miktar
	}
	oran := 0.0
	if lmiktar > 0 {
		oran = dolanMiktar / lmiktar
	}
	var kalanTutar, kalanMiktar float64
	yeterli := false
	if taraf == "BUY" {
		kalanTutar = asagiYuvarla(butce-dolanTutar, k.TutarOndalik)
		yeterli = kalanTutar >= k.EnAzTutarPiyasa && kalanTutar > 0
	} else {
		kalanMiktar = asagiYuvarla(miktar-dolanMiktar, k.MiktarOndalik)
		yeterli = kalanMiktar >= k.EnAzMiktar && kalanMiktar*du.Bid >= k.EnAzTutarPiyasa
	}
	if !yeterli {
		olay("bilgi", fmt.Sprintf("15 sn doldu; limit emrin %s'i doldu. Kalan kısım en az emir tutarının altında, piyasa emri verilmedi", yuzdeMetni(oran)))
		d := dolumTopla(taraf, k, parcalar)
		d.LimitFiyat, d.Baslangic, d.Bitis = fiyat, basla.UnixMilli(), simdiF().UnixMilli()
		if d.Miktar <= 0 {
			return d, fmt.Errorf("emir dolmadı ve kalan tutar çok küçük")
		}
		return d, nil
	}
	if dolanMiktar > 0 {
		olay("uyari", fmt.Sprintf("15 sn doldu; limit emrin %s'i doldu. Kalanı piyasa emriyle", yuzdeMetni(oran)))
	} else {
		olay("uyari", "15 sn doldu, limit emir dolmadı. Piyasa emri veriliyor")
	}
	pid, perr := piyasaVeDolum(b, k, taraf, kalanMiktar, kalanTutar, onek+"P")
	var pbh belirsizHata
	if perr != nil && errors.As(perr, &pbh) {
		// 0.8: piyasa emrinin sonucu belirsiz: limitin dolan kısmıyla birlikte kumbara sonra MEXC'ye sorar (kurtar)
		return dolumSonucu{}, perr
	}
	if perr != nil && len(parcalar) == 0 {
		return dolumSonucu{}, fmt.Errorf("piyasa emri verilemedi: %w", perr)
	}
	if perr != nil {
		olay("hata", "Piyasa emri verilemedi: "+perr.Error())
	}
	parcalar = append(parcalar, pid...)
	d := dolumTopla(taraf, k, parcalar)
	d.LimitFiyat, d.Baslangic, d.Bitis = fiyat, basla.UnixMilli(), simdiF().UnixMilli()
	return d, nil
}

// Doğrudan piyasa emri (zarar durdur, elle "şimdi sat", çok küçük tutarlar)
func dogrudanPiyasa(b borsa, p piyasaKaynagi, k sembolKurali, taraf string, butce, miktar float64, onek string, olay olayFn) (dolumSonucu, error) {
	basla := simdiF()
	var tutar, adet float64
	if taraf == "BUY" {
		tutar = asagiYuvarla(butce, k.TutarOndalik)
		if tutar < k.EnAzTutarPiyasa {
			return dolumSonucu{}, fmt.Errorf("tutar (%s USDT) MEXC'nin en az emir tutarının (%s USDT) altında", trSayi(tutar, 2), trSayi(k.EnAzTutarPiyasa, 2))
		}
	} else {
		adet = asagiYuvarla(miktar, k.MiktarOndalik)
		if adet < k.EnAzMiktar {
			return dolumSonucu{}, fmt.Errorf("satılacak miktar MEXC'nin en az miktarının altında")
		}
	}
	parcalar, err := piyasaVeDolum(b, k, taraf, adet, tutar, onek+"P")
	if err != nil && len(parcalar) == 0 {
		return dolumSonucu{}, err
	}
	d := dolumTopla(taraf, k, parcalar)
	d.Baslangic, d.Bitis = basla.UnixMilli(), simdiF().UnixMilli()
	olay("bilgi", fmt.Sprintf("Piyasa emri doldu: ortalama %s", fiyatMetni(d.Ortalama, k)))
	return d, nil
}

func piyasaVeDolum(b borsa, k sembolKurali, taraf string, miktar, tutar float64, istemciID string) ([]parca, error) {
	e, err := b.PiyasaEmri(k, taraf, miktar, tutar, istemciID)
	if err != nil {
		return nil, err
	}
	if e.Dolan <= 0 {
		return nil, fmt.Errorf("piyasa emri dolmadı (%s)", e.Durum)
	}
	parcalar := emrinDolumlari(b, k, e.ID, e, false)
	for i := range parcalar {
		parcalar[i].Kaynak = "piyasa"
	}
	return parcalar, nil
}

func limitDolumlari(b borsa, k sembolKurali, id string, son emirDurumu) ([]parca, error) {
	if son.Dolan <= 0 {
		return nil, nil
	}
	return emrinDolumlari(b, k, id, son, true), nil
}

// 0.8: Bir emrin dolum parçaları. Esas olan EMRİN KENDİ TOPLAMIDIR (dolan miktar ve tutar); MEXC'nin işlem listesi
// (myTrades) gecikebilir ya da eksik gelebilir. Liste toplamla uyuşursa ayrıntılı parçalar (gerçek komisyonlarıyla)
// kullanılır; uyuşmazsa emrin toplamından tek parça yapılır, komisyon listedeki orandan ya da standart orandan tahmin
// edilir. (0.7'de eksik liste esas alınıyordu: kalan bütçe yanlış hesaplanıp kumbaradan fazla harcanabiliyordu.)
func emrinDolumlari(b borsa, k sembolKurali, id string, e emirDurumu, maker bool) []parca {
	var parcalar []parca
	for deneme := 0; deneme < 4; deneme++ {
		p, err := b.Dolumlar(k, id)
		if err == nil {
			parcalar = p
			if math.Abs(toplamMiktar(p)-e.Dolan) <= e.Dolan*1e-6+1e-12 {
				return p
			}
		}
		uyu(time.Second)
	}
	return toplamdanParca(parcalar, e, k, maker)
}

// Emrin toplamından tek parça; komisyon varlığı ve oranı (varsa) eldeki eksik listeden, yoksa standart oranla USDT
func toplamdanParca(eldeki []parca, e emirDurumu, k sembolKurali, maker bool) []parca {
	f := 0.0
	if e.Dolan > 0 {
		f = e.DolanTutar / e.Dolan
	}
	p := parca{Fiyat: f, Miktar: e.Dolan, Tutar: e.DolanTutar, Maker: maker, Zaman: simdiF().UnixMilli()}
	elMiktar := toplamMiktar(eldeki)
	if elMiktar > 0 && eldeki[0].KomisyonVarligi != "" {
		oran := e.Dolan / elMiktar
		p.KomisyonVarligi = eldeki[0].KomisyonVarligi
		for _, x := range eldeki {
			if x.KomisyonVarligi == p.KomisyonVarligi {
				p.Komisyon += x.Komisyon * oran
			}
			p.KomisyonUSDT += x.KomisyonUSDT * oran
		}
		return []parca{p}
	}
	r := k.Taker
	if maker {
		r = k.Maker
	}
	p.Komisyon, p.KomisyonVarligi, p.KomisyonUSDT = e.DolanTutar*r, "USDT", e.DolanTutar*r
	return []parca{p}
}

func toplamMiktar(p []parca) float64 {
	t := 0.0
	for _, x := range p {
		t += x.Miktar
	}
	return t
}
