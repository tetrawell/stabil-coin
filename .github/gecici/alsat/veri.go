package main

// MUM VERİSİ — Kripto Simülatör 3.2'nin main.go'sundaki kodun aynısı.
// İki programın grafikleri ve kararları uyuşsun diye veri aynı yerden, aynı yöntemle gelir:
// MEXC /api/v3/klines, 500'erli sayfalar, aynı sıralama, tekrar atma ve "son mum kapanmadı" kontrolü.
// Farklar: mum sınırı 30.000 (0.8; önce 20.000): simülatör 3.4 en fazla 20.000 mumla ayar bulur, kumbara ömrü boyunca
// mumlar büyür ve program yeniden açılınca hepsi aynı ilk mumdan yüklenebilmeli. "bitis" ile belirli bir andan
// ÖNCEKİ mumlar istenebilir (sağlamlık testi için; simülatörde yok). Test verisi: sahte_veri.go.
// Bu dosyayı değiştirirken simülatördekiyle aynı kalmasına dikkat et.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	istemci     = &http.Client{Timeout: 20 * time.Second}
	sembolKalip = regexp.MustCompile(`^[A-Z0-9]{2,30}$`)
)

func sayi(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

var binanceSunuculari = []string{"https://api.binance.com", "https://data-api.binance.vision", "https://api1.binance.com", "https://api2.binance.com"}

// MEXC adresi. Testte sahte sunucuya yönlendirmek için ALSAT_MEXC_ADRES verilebilir.
var mexcAdresi = ortamVeya("ALSAT_MEXC_ADRES", "https://api.mexc.com")

// MEXC'nin belgesinde "en fazla 1000" yazsa da pratikte tek seferde en fazla 500 mum veriyor.
const mexcSayfa = 500

// Bir istekte en fazla kaç mum (simülatör 3.4'te 20.000; 0.7'ye kadar burada 20.000).
const enFazlaMum = 30000

var araliklar = map[string]struct {
	binance, mexc string
	ms            int64
}{
	"1m":  {"1m", "1m", 60_000},
	"5m":  {"5m", "5m", 300_000},
	"15m": {"15m", "15m", 900_000},
	"1h":  {"1h", "60m", 3_600_000},
	"4h":  {"4h", "4h", 14_400_000},
	"1d":  {"1d", "1d", 86_400_000},
}

type borsaHatasi struct{ mesaj string }

func (e borsaHatasi) Error() string { return e.mesaj }

func getJSON(adres string) ([]byte, error) {
	req, _ := http.NewRequest("GET", adres, nil)
	req.Header.Set("User-Agent", "KriptoAlSat/"+surumAdi)
	r, err := istemci.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		return nil, err
	}
	if r.StatusCode != 200 {
		var e struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Msg != "" {
			return nil, borsaHatasi{fmt.Sprintf("%s (kod %d)", e.Msg, e.Code)}
		}
		return nil, fmt.Errorf("sunucu %d döndü", r.StatusCode)
	}
	return b, nil
}

// Tek sayfa mum çeker; [ [acilisZamani, o,h,l,c,v, kapanisZamani], ... ]
// bitis 0 ise en yeni mumlar gelir. MEXC'de eski sayfa için bas ve bitis birlikte verilir.
func mumSayfasi(borsa, sembol, aralik string, limit int, bas, bitis int64) ([][]float64, string, error) {
	var kaynaklar []string
	ar := araliklar[aralik]
	if borsa == "mexc" {
		u := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d", mexcAdresi, sembol, ar.mexc, limit)
		if bitis > 0 {
			u += fmt.Sprintf("&startTime=%d&endTime=%d", bas, bitis)
		}
		kaynaklar = []string{u}
	} else {
		for _, s := range binanceSunuculari {
			u := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=%s&limit=%d", s, sembol, ar.binance, limit)
			if bitis > 0 {
				u += fmt.Sprintf("&endTime=%d", bitis)
			}
			kaynaklar = append(kaynaklar, u)
		}
	}
	var sonHata error
	for _, u := range kaynaklar {
		b, err := getJSON(u)
		if err != nil {
			sonHata = err
			var bh borsaHatasi
			if errors.As(err, &bh) {
				break // borsa "böyle coin yok" dediyse diğer sunucuları denemeye gerek yok
			}
			continue
		}
		var ham [][]any
		if err := json.Unmarshal(b, &ham); err != nil {
			sonHata = err
			continue
		}
		out := make([][]float64, 0, len(ham))
		for _, x := range ham {
			if len(x) < 7 {
				continue
			}
			out = append(out, []float64{sayi(x[0]), sayi(x[1]), sayi(x[2]), sayi(x[3]), sayi(x[4]), sayi(x[5]), sayi(x[6])})
		}
		sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
		host := u
		if pu, err := url.Parse(u); err == nil {
			host = pu.Host
		}
		return out, host, nil
	}
	return nil, "", sonHata
}

func mumlarIstegi(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	borsa := q.Get("borsa")
	if borsa != "mexc" {
		borsa = "binance"
	}
	sembol := strings.ToUpper(strings.TrimSpace(q.Get("sembol")))
	if !sembolKalip.MatchString(sembol) {
		hataYaz(w, 400, "Coin adı geçersiz. Örnek: BTCUSDT")
		return
	}
	aralik := q.Get("aralik")
	if _, ok := araliklar[aralik]; !ok {
		hataYaz(w, 400, "Mum süresi geçersiz")
		return
	}
	adet, _ := strconv.Atoi(q.Get("adet"))
	if adet < 10 {
		adet = 10
	}
	if adet > enFazlaMum {
		adet = enFazlaMum
	}
	// 0.8: bitis verilirse sadece bu andan (açılış zamanı, ms) ÖNCEKİ mumlar gelir
	bitis, _ := strconv.ParseInt(q.Get("bitis"), 10, 64)
	if bitis < 0 || borsa != "mexc" {
		bitis = 0
	}

	if testModu {
		ek := 0
		if bitis > 0 {
			ek = int(max(0, (time.Now().UnixMilli()-bitis)/araliklar[aralik].ms)) + 1
		}
		mm := testMumlari(sembol, aralik, adet+ek)
		if bitis > 0 {
			k := mm[:0]
			for _, x := range mm {
				if int64(x[0]) < bitis {
					k = append(k, x)
				}
			}
			mm = k
			if len(mm) > adet {
				mm = mm[len(mm)-adet:]
			}
		} else if len(mm) > adet {
			mm = mm[len(mm)-adet:]
		}
		jsonYaz(w, 200, map[string]any{"mumlar": mm, "acikSon": bitis == 0, "kaynak": "test verisi"})
		return
	}

	hepsi, kaynak, err := mumlariTopla(borsa, sembol, aralik, adet, bitis)
	if err != nil {
		ad := "Binance"
		if borsa == "mexc" {
			ad = "MEXC"
		}
		var bh borsaHatasi
		if errors.As(err, &bh) {
			hataYaz(w, 502, fmt.Sprintf("%s bu coini vermedi: %s. Coin adını kontrol et.", ad, bh.mesaj))
		} else {
			hataYaz(w, 502, fmt.Sprintf("%s'e bağlanılamadı (%v). İnternet bağlantını kontrol et.", ad, err))
		}
		return
	}
	// sırala ve tekrarları at
	sort.Slice(hepsi, func(i, j int) bool { return hepsi[i][0] < hepsi[j][0] })
	temiz := hepsi[:0]
	for _, x := range hepsi {
		if len(temiz) > 0 && temiz[len(temiz)-1][0] == x[0] {
			continue
		}
		temiz = append(temiz, x)
	}
	if len(temiz) > adet {
		temiz = temiz[len(temiz)-adet:]
	}
	acikSon := false
	// 0.8: "son mum kapandı mı?" MEXC'nin saatiyle (borsaFarki) ve 1 sn payla bakılır. 0.7'de bu bilgisayarın saati
	// MEXC'den ileriyse kapanmamış mum kapanmış sayılabiliyor, motor yarım mumla karar verebiliyordu.
	simdi := float64(time.Now().UnixMilli() + borsaFarki.Load() - 1000)
	out := make([][6]float64, len(temiz))
	for i, x := range temiz {
		out[i] = [6]float64{x[0], x[1], x[2], x[3], x[4], x[5]}
	}
	if len(temiz) > 0 && temiz[len(temiz)-1][6] > simdi {
		acikSon = true
	}
	jsonYaz(w, 200, map[string]any{"mumlar": out, "acikSon": acikSon, "kaynak": kaynak})
}

// Sayfa sayfa geriye giderek istenen sayıda mum toplar (sırasız olabilir, tekrar içerebilir).
// Hiç veri alınamazsa hata döner; bir kısmı alındıysa elde olanla yetinir.
func mumlariTopla(borsa, sembol, aralik string, adet int, bitis0 int64) ([][]float64, string, error) {
	ar := araliklar[aralik]
	mexc := borsa == "mexc"
	sayfaMax := 1000
	if mexc {
		sayfaMax = mexcSayfa
	}
	var hepsi [][]float64
	var kaynak string
	var enEski int64 // elimizdeki en eski mumun açılış zamanı (0 = henüz yok)
	var imlec int64  // sıradaki isteğin bitişi (MEXC'de hariç tutulur)
	if bitis0 > 0 && mexc {
		imlec, enEski = bitis0, bitis0 // sadece bu andan öncesi
	}
	bosPencere := 0
	for istekSayisi := 0; len(hepsi) < adet && istekSayisi < 80; istekSayisi++ {
		istek := adet - len(hepsi)
		if istek > sayfaMax {
			istek = sayfaMax
		}
		var bas, bitis int64
		if imlec > 0 {
			if mexc {
				// MEXC: [bas, bitis) penceresi, tam "istek" kadar mum sığar
				bas, bitis = imlec-int64(istek)*ar.ms, imlec
			} else {
				bitis = imlec - 1 // Binance: bitis dahil, en yeni "istek" mum gelir
			}
		}
		sayfa, k, err := mumSayfasi(borsa, sembol, aralik, istek, bas, bitis)
		if err != nil {
			if len(hepsi) > 0 {
				break // elimizde veri var, eskisini alamadık; olanla devam
			}
			return nil, "", err
		}
		kaynak = k
		if enEski > 0 {
			sayfa = zamanArasi(sayfa, 0, enEski) // sadece elimizdekinden eski mumlar
		}
		if len(sayfa) == 0 {
			if mexc && imlec > 0 && bosPencere < 1 {
				// Borsada uzun bir kesinti olmuş olabilir; bir pencere daha geriye bak
				bosPencere++
				imlec = bas
				continue
			}
			break // borsa daha eski veri vermiyor
		}
		bosPencere = 0
		if mexc && enEski > 0 {
			// Borsa pencerenin yeni tarafını kestiyse aradaki eksik mumları tamamla
			for dolgu := 0; dolgu < 10; dolgu++ {
				enYeni := int64(sayfa[len(sayfa)-1][0])
				if enYeni+ar.ms >= enEski {
					break
				}
				ek, _, err := mumSayfasi(borsa, sembol, aralik, sayfaMax, enYeni+1, enEski)
				if err != nil {
					break
				}
				ek = zamanArasi(ek, enYeni, enEski)
				if len(ek) == 0 {
					break // gerçekten boşluk var (borsa o sürede mum üretmemiş)
				}
				sayfa = append(sayfa, ek...)
			}
		}
		hepsi = append(sayfa, hepsi...)
		enEski = int64(sayfa[0][0])
		imlec = enEski
		if !mexc && len(sayfa) < istek {
			break // Binance kısa sayfa verdiyse daha eski veri yok
		}
	}
	return hepsi, kaynak, nil
}

// Açılış zamanı (alt, ust) aralığında olan mumları döndürür; alt 0 ise alt sınır yok.
func zamanArasi(mumlar [][]float64, alt, ust int64) [][]float64 {
	out := mumlar[:0:0]
	for _, x := range mumlar {
		t := int64(x[0])
		if (alt == 0 || t > alt) && t < ust {
			out = append(out, x)
		}
	}
	return out
}
