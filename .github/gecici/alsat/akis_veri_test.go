package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// akisSifirla: her test temiz bir toplayıcıyla başlar.
func akisSifirla(t *testing.T) {
	t.Helper()
	eskiAK, eskiD, eskiKlasor := AK, D, veriKlasoru
	AK = yeniAkisToplayici()
	durumMu.Lock()
	D = yeniDurum()
	durumMu.Unlock()
	veriKlasoru = t.TempDir()
	t.Cleanup(func() {
		AK.mu.Lock()
		if AK.disk != nil {
			AK.disk.Close()
		}
		AK.mu.Unlock()
		AK, D, veriKlasoru = eskiAK, eskiD, eskiKlasor
	})
}

const dk = int64(60_000)

func TestAkisDakikaToplama(t *testing.T) {
	akisSifirla(t)
	c := &akisCoin{Sembol: "BTCUSDT", Vadeli: "BTC_USDT", Kontrat: 1, spotCanli: 1, vadCanli: 1}
	AK.coinler["BTCUSDT"] = c
	z0 := int64(1_800_000_000_000) / dk * dk
	AK.mu.Lock()
	AK.islemEkle(c, akisIslem{Z: z0 + 1000, Fiyat: 100, Adet: 2, Alim: true}, false)
	AK.islemEkle(c, akisIslem{Z: z0 + 2000, Fiyat: 101, Adet: 1, Alim: false}, false)
	AK.islemEkle(c, akisIslem{Z: z0 + 3000, Fiyat: 99, Adet: 3, Alim: false}, false)
	AK.islemEkle(c, akisIslem{Z: z0 + 4000, Fiyat: 100.004, Adet: 1, Alim: true}, false) // 100 ile aynı kova (0,4 baz puan)
	AK.islemEkle(c, akisIslem{Z: z0 + 5000, Fiyat: 100.5, Adet: 7, Alim: true}, true)    // vadeli: fiyat/footprint'e girmez
	// sonraki dakika
	AK.islemEkle(c, akisIslem{Z: z0 + dk + 10, Fiyat: 102, Adet: 1, Alim: true}, false)
	// pay süresi (3 sn) dolmadan önceki dakikaya geç gelen işlem kabul edilir
	AK.islemEkle(c, akisIslem{Z: z0 + 59_900, Fiyat: 98, Adet: 1, Alim: false}, false)
	yaz := c.dakikaKapat(z0 + dk + 2_999)
	if len(yaz) != 0 {
		t.Fatal("pay süresi dolmadan dakika kapandı")
	}
	yaz = c.dakikaKapat(z0 + dk + 3_000)
	AK.mu.Unlock()
	if len(yaz) != 1 || len(c.dakikalar) != 1 {
		t.Fatalf("dakika kapanmadı: %d %d", len(yaz), len(c.dakikalar))
	}
	d := c.dakikalar[0]
	if d.Z != z0 || d.A != 100 || d.Y != 101 || d.D != 98 || d.N != 5 || d.Al != 3 || d.Sat != 5 || d.VAl != 7 {
		t.Fatalf("çubuk yanlış: %+v", d)
	}
	if d.K != 100.004 {
		t.Fatalf("kapanış son (zamanca) işlem olmalı: %v", d.K)
	}
	// footprint: 100 ve 100.004 aynı kova, toplam alım 3
	k100 := float64(akisKova(100))
	var bulundu bool
	var top [2]float64
	for i, x := range d.Iz {
		if i > 0 && d.Iz[i-1][0] >= x[0] {
			t.Fatal("footprint kovaya göre sıralı değil")
		}
		top[0] += x[1]
		top[1] += x[2]
		if x[0] == k100 {
			bulundu = x[1] == 3 && x[2] == 0
		}
	}
	if !bulundu || top[0] != d.Al || top[1] != d.Sat {
		t.Fatalf("footprint: %v", d.Iz)
	}
	if f := akisKovaFiyat(akisKova(100)); f > 100 || f < 99.99 {
		t.Fatalf("kova fiyatı: %v", f)
	}
	// kapanmış dakikaya çok geç gelen işlem atılır
	AK.mu.Lock()
	AK.islemEkle(c, akisIslem{Z: z0 + 30_000, Fiyat: 97, Adet: 100, Alim: false}, false)
	AK.mu.Unlock()
	if c.dakikalar[0].Sat != 5 || c.dakikalar[0].D != 98 {
		t.Fatal("kapanmış çubuk değişti")
	}
	if len(c.acik) != 1 || c.acik[0].d.Z != z0+dk || c.acik[0].d.Al != 1 {
		t.Fatalf("süren dakika: %+v", c.acik)
	}
}

func TestAkisEksikVadeliVeOI(t *testing.T) {
	akisSifirla(t)
	z0 := int64(1_800_000_000_000) / dk * dk
	c := &akisCoin{Sembol: "DOSUSDT", Vadeli: "DOS_USDT", Kontrat: 1, spotCanli: z0 + 20_000, vadCanli: z0 - 1}
	AK.coinler["DOSUSDT"] = c
	AK.mu.Lock()
	AK.islemEkle(c, akisIslem{Z: z0 + 30_000, Fiyat: 0.26, Adet: 10, Alim: true}, false)
	AK.tickerEkle(c, &vadeliTicker{Z: z0 + 31_000, AcikP: 1000, Fon: 0.0001})
	AK.tickerEkle(c, &vadeliTicker{Z: z0 + 40_000, AcikP: 900, Fon: 0.0001})
	AK.tickerEkle(c, &vadeliTicker{Z: z0 + 50_000, AcikP: 950, Fon: 0.0002})
	c.dakikaKapat(z0 + dk + 3_000)
	c.acikBul(z0 + dk + 3_000)
	AK.tickerEkle(c, &vadeliTicker{Z: z0 + dk + 4_000, AcikP: 960, Fon: 0.0002})
	c.dakikaKapat(z0 + 2*dk + 3_000)
	AK.mu.Unlock()
	if len(c.dakikalar) != 2 {
		t.Fatalf("%d dakika", len(c.dakikalar))
	}
	d := c.dakikalar[0]
	if !d.Eksik {
		t.Fatal("spot aboneliği dakikanın ortasında başladı; dakika eksik sayılmalı")
	}
	if !d.VV || d.OI != 950 || d.OIA != 900 || d.OIY != 1000 || d.Fon != 0.0002 {
		t.Fatalf("OI: %+v", d)
	}
	d2 := c.dakikalar[1]
	if d2.Eksik || d2.OI != 960 || d2.OIA != 950 || d2.OIY != 960 {
		t.Fatalf("ikinci dakika (OI önceki dakikadan taşınır): %+v", d2)
	}
	// vadeli canlılığı dakikanın ortasında başladıysa vadeli verisi o dakika için yok sayılır
	c2 := &akisCoin{Sembol: "XUSDT", Vadeli: "X_USDT", Kontrat: 2, spotCanli: 1, vadCanli: z0 + 10_000}
	AK.mu.Lock()
	AK.islemEkle(c2, akisIslem{Z: z0 + 20_000, Fiyat: 5, Adet: 3, Alim: true}, true)
	AK.tickerEkle(c2, &vadeliTicker{Z: z0 + 20_000, AcikP: 50})
	c2.dakikaKapat(z0 + dk + 3_000)
	AK.mu.Unlock()
	if x := c2.dakikalar[0]; x.VV || x.VAl != 0 || x.OI != 0 {
		t.Fatalf("yarım vadeli dakika: %+v", x)
	}
	if c2.sonOI != 100 {
		t.Fatalf("kontrat boyu: son OI %v (50 kontrat × 2)", c2.sonOI)
	}
}

func TestAkisBuyukIslem(t *testing.T) {
	akisSifirla(t)
	c := &akisCoin{Sembol: "BTCUSDT", spotCanli: 1}
	z0 := int64(1_800_000_000_000) / dk * dk
	AK.mu.Lock()
	for i := 0; i < 300; i++ {
		AK.islemEkle(c, akisIslem{Z: z0 + int64(i), Fiyat: 100, Adet: 0.1 + float64(i%10)*0.01, Alim: i%2 == 0}, false)
	}
	AK.islemEkle(c, akisIslem{Z: z0 + 400, Fiyat: 100, Adet: 50, Alim: false}, false)
	AK.islemEkle(c, akisIslem{Z: z0 + 500, Fiyat: 100, Adet: 0.12, Alim: true}, false)
	AK.mu.Unlock()
	if len(c.buyukler) != 1 || c.buyukler[0].Adet != 50 || c.buyukler[0].Alim {
		t.Fatalf("büyük işlem: %+v (sınır %v)", c.buyukler, c.esik)
	}
	if c.acik[0].d.BSat != 50 || c.acik[0].d.BAl != 0 {
		t.Fatal("dakikadaki büyük satış")
	}
}

func TestAkisDiskVeYukleme(t *testing.T) {
	akisSifirla(t)
	simdi := akisSimdi() / dk * dk
	c := &akisCoin{Sembol: "BTCUSDT", spotCanli: 1}
	AK.coinler["BTCUSDT"] = c
	AK.mu.Lock()
	for i := int64(0); i < 5; i++ {
		AK.islemEkle(c, akisIslem{Z: simdi - (10-i)*dk + 5, Fiyat: 100 + float64(i), Adet: 1, Alim: true}, false)
	}
	AK.diskeYaz("BTCUSDT", c.dakikaKapat(simdi))
	// geri doldurma: kayıtta daha önce yazılmış, daha eski dakikalar (sıra dışı satır)
	eski := []*akisDakika{{Z: simdi - 20*dk, Al: 9, Geri: true}, {Z: simdi - 19*dk, Sat: 4, Geri: true}}
	AK.diskeYaz("BTCUSDT", eski)
	// aynı dakikanın yeniden yazılması: sonuncusu geçerli
	AK.diskeYaz("BTCUSDT", []*akisDakika{{Z: simdi - 19*dk, Sat: 5, Geri: true}})
	if AK.disk != nil {
		AK.disk.Close()
		AK.disk = nil
	}
	AK.mu.Unlock()
	// yeniden aç
	AK = yeniAkisToplayici()
	akisYukle()
	c2 := AK.coinler["BTCUSDT"]
	if c2 == nil || len(c2.dakikalar) != 7 {
		t.Fatalf("yüklenen: %+v", c2)
	}
	for i := 1; i < len(c2.dakikalar); i++ {
		if c2.dakikalar[i].Z <= c2.dakikalar[i-1].Z {
			t.Fatal("yüklenen dakikalar sıralı değil")
		}
	}
	if c2.dakikalar[0].Al != 9 || c2.dakikalar[1].Sat != 5 || c2.dakikalar[2].A != 100 {
		t.Fatalf("yüklenen değerler: %+v %+v %+v", c2.dakikalar[0], c2.dakikalar[1], c2.dakikalar[2])
	}
	// eski günlerin dosyası silinir
	os.WriteFile(filepath.Join(akisKlasoru(), "2020-01-01.jsonl"), []byte("{}\n"), 0644)
	AK.mu.Lock()
	AK.eskiKayitlariSil()
	AK.mu.Unlock()
	if _, err := os.Stat(filepath.Join(akisKlasoru(), "2020-01-01.jsonl")); err == nil {
		t.Fatal("eski kayıt silinmedi")
	}
}

func TestAkisGeriEkle(t *testing.T) {
	akisSifirla(t)
	z0 := int64(1_800_000_000_000) / dk * dk
	c := &akisCoin{Sembol: "BTCUSDT"}
	c.dakikalar = []*akisDakika{
		{Z: z0, Al: 1},                   // tam: dokunulmaz
		{Z: z0 + dk, Al: 1, Eksik: true}, // eksik: değişir
		{Z: z0 + 5*dk, Al: 1},            // aralık dışı
	}
	islemler := []akisIslem{
		{Z: z0 + 10, Fiyat: 10, Adet: 5, Alim: true},      // tam dakikaya: yok sayılır
		{Z: z0 + dk + 10, Fiyat: 10, Adet: 2, Alim: true}, // eksik dakikanın yerine
		{Z: z0 + dk + 20, Fiyat: 11, Adet: 3, Alim: false},
		{Z: z0 + 3*dk + 1, Fiyat: 12, Adet: 1, Alim: false}, // hiç olmayan dakika
	}
	eklenen := akisGeriEkle(c, islemler, z0, z0+4*dk)
	if len(eklenen) != 3 { // z0+1, z0+2 (işlemsiz), z0+3
		t.Fatalf("eklenen %d", len(eklenen))
	}
	if len(c.dakikalar) != 5 {
		t.Fatalf("toplam %d", len(c.dakikalar))
	}
	if c.dakikalar[0].Al != 1 || c.dakikalar[0].Geri {
		t.Fatal("tam dakika değişti")
	}
	d1 := c.dakikalar[1]
	if !d1.Geri || d1.Al != 2 || d1.Sat != 3 || d1.A != 10 || d1.K != 11 || d1.N != 2 || len(d1.Iz) != 2 {
		t.Fatalf("geri doldurulan: %+v", d1)
	}
	if !c.dakikalar[2].Geri || c.dakikalar[2].N != 0 {
		t.Fatal("işlemsiz dakika boş çubuk olmalı")
	}
	if c.dakikalar[4].Z != z0+5*dk {
		t.Fatal("aralık dışı dakika kayboldu")
	}
}

func TestAkisAggTradesSayfalama(t *testing.T) {
	akisSifirla(t)
	// sahte MEXC: saniye çözünürlüğünde, en yeniden eskiye, istekte en çok 1000 işlem ve 1 saat
	bas := int64(1_800_000_000_000) / 1000 * 1000
	// MEXC'nin alan sırası (a, f, l, p, q, T, m, M): "M" (isBestMatch) "m"den SONRA gelir. Sıralı olsun diye yapı.
	type aggIslem struct {
		A *int   `json:"a"`
		F *int   `json:"f"`
		L *int   `json:"l"`
		P string `json:"p"`
		Q string `json:"q"`
		T int64  `json:"T"`
		M bool   `json:"m"`
		B bool   `json:"M"`
	}
	var tum []aggIslem
	for s := int64(0); s < 1800; s++ { // 30 dk, her saniye 2 işlem
		for j := 0; j < 2; j++ {
			tum = append(tum, aggIslem{P: fmt.Sprintf("%.2f", 100+float64(s)/100), Q: fmt.Sprintf("%d.5", j+1), T: bas + s*1000, M: j == 1, B: true})
		}
	}
	for j := 0; j < 1500; j++ { // tek bir saniyede 1500 işlem (1000 sınırından fazla)
		tum = append(tum, aggIslem{P: "200.00", Q: fmt.Sprintf("0.%04d", j+1), T: bas + 900_000, M: false, B: true})
	}
	sort.SliceStable(tum, func(i, j int) bool { return tum[i].T > tum[j].T })
	var istek atomic.Int32
	sunucu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		istek.Add(1)
		st, _ := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
		en, _ := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		if en-st > 3_600_000 {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"code":-1,"msg":"range too large"}`)
			return
		}
		var out []aggIslem
		for _, x := range tum {
			if z := x.T; z >= st && z <= en {
				out = append(out, x)
				if len(out) == 1000 {
					break
				}
			}
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer sunucu.Close()
	eski := mexcAdresi
	mexcAdresi = sunucu.URL
	defer func() { mexcAdresi = eski }()
	islemler, kapsam, err := akisAggTrades("BTCUSDT", bas, bas+1800_000-1, 40)
	if err != nil {
		t.Fatal(err)
	}
	if kapsam != bas {
		t.Fatalf("bütün aralık getirilmeliydi: kapsam %d, bas %d", kapsam, bas)
	}
	// istek sınırına takılınca: getirilebilen kısmın başı bildirilir (öncesi "işlem yok" diye uydurulmasın)
	kisa, kapsam2, err := akisAggTrades("BTCUSDT", bas, bas+1800_000-1, 2)
	if err != nil || len(kisa) == 0 || kapsam2 <= bas || kapsam2 > kisa[0].Z+1000 {
		t.Fatalf("sınırlı istek: %d işlem, kapsam %d (ilk işlem %d), %v", len(kisa), kapsam2, kisa[0].Z, err)
	}
	beklenen := 3598 + 1000 // o saniyedeki 1502 işlemin (1500 + 2 olağan) en fazla 1000'i alınabilir; kalanı atlanır
	if len(islemler) != beklenen {
		t.Fatalf("%d işlem (beklenen %d), %d istek", len(islemler), beklenen, istek.Load())
	}
	gorulen := map[string]bool{}
	for i, x := range islemler {
		a := fmt.Sprint(x)
		if gorulen[a] {
			t.Fatal("tekrar eden işlem")
		}
		gorulen[a] = true
		if i > 0 && islemler[i-1].Z > x.Z {
			t.Fatal("eskiden yeniye sıralı değil")
		}
	}
	if islemler[0].Alim != true || islemler[1].Alim != false {
		t.Fatal("m (isBuyerMaker) yönü ters çevrilmeli: m=false → alıcı saldırdı")
	}
}

// ---------- canlı hatlar: sahte MEXC websocket sunucularıyla ----------

func pbMetinE(b []byte, no int, s string) []byte {
	b = binary.AppendUvarint(b, uint64(no<<3|2))
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}
func pbSayiE(b []byte, no int, v uint64) []byte {
	b = binary.AppendUvarint(b, uint64(no<<3))
	return binary.AppendUvarint(b, v)
}
func pbMesajE(b []byte, no int, m []byte) []byte {
	b = binary.AppendUvarint(b, uint64(no<<3|2))
	b = binary.AppendUvarint(b, uint64(len(m)))
	return append(b, m...)
}

func sahteSpotIslem(sembol string, z int64, fiyat, adet float64, alim bool) []byte {
	var oge []byte
	oge = pbMetinE(oge, 1, strconv.FormatFloat(fiyat, 'f', -1, 64))
	oge = pbMetinE(oge, 2, strconv.FormatFloat(adet, 'f', -1, 64))
	tip := uint64(2)
	if alim {
		tip = 1
	}
	oge = pbSayiE(oge, 3, tip)
	oge = pbSayiE(oge, 4, uint64(z))
	var govde []byte
	govde = pbMesajE(govde, 1, oge)
	govde = pbMetinE(govde, 2, "spot@public.aggre.deals.v3.api.pb@100ms")
	var w []byte
	w = pbMetinE(w, 1, "spot@public.aggre.deals.v3.api.pb@100ms@"+sembol)
	w = pbMetinE(w, 3, sembol)
	w = pbSayiE(w, 6, uint64(z))
	return pbMesajE(w, 314, govde)
}

type sahteMexcWS struct {
	mu       sync.Mutex
	baglanti int
	abone    map[string]bool // spot: kanal; vadeli: yöntem@sembol
	pingler  int
	ln       net.Listener
	vadeli   bool
}

func (s *sahteMexcWS) adres() string { return "ws://" + s.ln.Addr().String() + "/ws" }

func sahteMexcWSKur(t *testing.T, vadeli bool) *sahteMexcWS {
	s := &sahteMexcWS{abone: map[string]bool{}, vadeli: vadeli}
	d := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) {
		s.mu.Lock()
		s.baglanti++
		s.mu.Unlock()
		dur := make(chan struct{})
		var yazMu sync.Mutex
		yaz := func(op byte, b []byte) { yazMu.Lock(); sunucuYaz(c, true, op, b); yazMu.Unlock() }
		go func() { // abone olunan coinlere her 50 ms'de bir işlem gönder
			for i := 0; ; i++ {
				select {
				case <-dur:
					return
				case <-time.After(50 * time.Millisecond):
				}
				s.mu.Lock()
				var liste []string
				for k := range s.abone {
					liste = append(liste, k)
				}
				s.mu.Unlock()
				z := time.Now().UnixMilli()
				for _, k := range liste {
					if !vadeli && strings.Contains(k, "aggre.deals") {
						sem := k[strings.LastIndex(k, "@")+1:]
						yaz(wsIkili, sahteSpotIslem(sem, z, 100, 1, i%3 != 0))
					}
					if vadeli && strings.HasPrefix(k, "sub.deal@") {
						sem := strings.TrimPrefix(k, "sub.deal@")
						yaz(wsMetin, []byte(fmt.Sprintf(`{"channel":"push.deal","data":[{"p":100,"v":4,"T":1,"O":1,"M":2,"t":%d}],"symbol":"%s","ts":%d}`, z, sem, z)))
					}
					if vadeli && strings.HasPrefix(k, "sub.ticker@") {
						sem := strings.TrimPrefix(k, "sub.ticker@")
						yaz(wsMetin, []byte(fmt.Sprintf(`{"channel":"push.ticker","data":{"symbol":"%s","lastPrice":100,"holdVol":%d,"fundingRate":0.0001,"timestamp":%d},"symbol":"%s","ts":%d}`, sem, 1000+i, z, sem, z)))
					}
				}
			}
		}()
		defer close(dur)
		for {
			op, veri, _, err := sunucuOku(r)
			if err != nil || op == wsKapat {
				return
			}
			var m struct {
				Method string            `json:"method"`
				Params []string          `json:"params"`
				Param  map[string]string `json:"param"`
			}
			json.Unmarshal(veri, &m)
			s.mu.Lock()
			switch {
			case m.Method == "PING" || m.Method == "ping":
				s.pingler++
				if vadeli {
					s.mu.Unlock()
					yaz(wsMetin, []byte(`{"channel":"pong","data":1}`))
					continue
				}
				s.mu.Unlock()
				yaz(wsMetin, []byte(`{"id":0,"code":0,"msg":"PONG"}`))
				continue
			case m.Method == "SUBSCRIPTION":
				for _, p := range m.Params {
					s.abone[p] = true
				}
			case m.Method == "UNSUBSCRIPTION":
				for _, p := range m.Params {
					delete(s.abone, p)
				}
			case strings.HasPrefix(m.Method, "sub."):
				s.abone[m.Method+"@"+m.Param["symbol"]] = true
			case strings.HasPrefix(m.Method, "unsub."):
				delete(s.abone, "sub."+strings.TrimPrefix(m.Method, "unsub.")+"@"+m.Param["symbol"])
			}
			s.mu.Unlock()
			if vadeli {
				yaz(wsMetin, []byte(`{"channel":"rs.`+m.Method+`","data":"success","ts":1}`))
			} else {
				yaz(wsMetin, []byte(`{"id":0,"code":0,"msg":"`+strings.Join(m.Params, ",")+`"}`))
			}
		}
	})
	s.ln = d.ln
	return s
}

func (s *sahteMexcWS) abonelik() (map[string]bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]bool{}
	for k := range s.abone {
		m[k] = true
	}
	return m, s.baglanti
}

func bekleKosul(t *testing.T, sure time.Duration, ad string, f func() bool) {
	t.Helper()
	son := time.Now().Add(sure)
	for time.Now().Before(son) {
		if f() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("beklenen olmadı: %s", ad)
}

func TestAkisCanliHatlar(t *testing.T) {
	akisSifirla(t)
	spot := sahteMexcWSKur(t, false)
	vad := sahteMexcWSKur(t, true)
	kontratSunucu := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":true,"code":0,"data":[{"symbol":"BTC_USDT","contractSize":0.0001,"state":0,"quoteCoin":"USDT","settleCoin":"USDT"},{"symbol":"ETH_USDT","contractSize":0.01,"state":0,"quoteCoin":"USDT","settleCoin":"USDT"}]}`)
	}))
	defer kontratSunucu.Close()
	e1, e2, e3 := akisSpotAdres, akisVadeliAdres, akisVadeliRest
	akisSpotAdres, akisVadeliAdres, akisVadeliRest = spot.adres(), vad.adres(), kontratSunucu.URL
	defer func() { akisSpotAdres, akisVadeliAdres, akisVadeliRest = e1, e2, e3 }()

	ctx, iptal := context.WithCancel(context.Background())
	defer iptal()
	akisIste("BTCUSDT")
	akisIste("DOGEUSDT") // vadelisi yok (kontrat listesinde değil)
	son := akisDakikaTuru(map[string]bool{})
	go akisHatYoneticisi(ctx, "spot", 2, akisIstenenler, AK.spotSinyal)
	go akisHatYoneticisi(ctx, "vadeli", akisVadeliHatta, akisVadeliIstenenler, AK.vadSinyal)

	bekleKosul(t, 5*time.Second, "spot abonelikleri", func() bool {
		a, _ := spot.abonelik()
		return a["spot@public.aggre.deals.v3.api.pb@100ms@BTCUSDT"] && a["spot@public.limit.depth.v3.api.pb@DOGEUSDT@20"]
	})
	bekleKosul(t, 5*time.Second, "vadeli abonelik (sadece vadelisi olan)", func() bool {
		a, _ := vad.abonelik()
		return a["sub.deal@BTC_USDT"] && a["sub.ticker@BTC_USDT"] && len(a) == 2
	})
	bekleKosul(t, 5*time.Second, "işlemler ve OI geliyor", func() bool {
		v := akisAnlik("BTCUSDT", 0)
		if !v.SpotCanli || !v.VadCanli || len(v.Dakikalar) == 0 {
			return false
		}
		d := v.Dakikalar[len(v.Dakikalar)-1]
		return d.N > 3 && d.VAl > 0 && v.SonOI > 0
	})
	v := akisAnlik("BTCUSDT", 0)
	d := v.Dakikalar[len(v.Dakikalar)-1]
	if n := d.VAl / (4 * 0.0001); math.Abs(n-math.Round(n)) > 1e-6 {
		t.Fatalf("vadeli adet kontrat boyuyla çarpılmalı: %v", d.VAl)
	}
	if math.Abs(v.SonOI/0.0001-math.Round(v.SonOI/0.0001)) > 1e-6 {
		t.Fatalf("OI kontrat boyuyla çarpılmalı: %v", v.SonOI)
	}
	if akisAnlik("DOGEUSDT", 0).Vadeli {
		t.Fatal("DOGE'nin vadelisi yok sayılmalı")
	}
	_, ilkSpotBaglanti := spot.abonelik()
	_, ilkVadBaglanti := vad.abonelik()

	// liste değişir: DOGE bırakılır, ETH eklenir → bağlantı kopmadan abone olunur / bırakılır
	AK.mu.Lock()
	delete(AK.bakis, "DOGEUSDT")
	AK.mu.Unlock()
	akisIste("ETHUSDT")
	son = akisDakikaTuru(son)
	bekleKosul(t, 5*time.Second, "DOGE bırakıldı, ETH eklendi", func() bool {
		a, _ := spot.abonelik()
		return a["spot@public.aggre.deals.v3.api.pb@100ms@ETHUSDT"] && !a["spot@public.aggre.deals.v3.api.pb@100ms@DOGEUSDT"]
	})
	bekleKosul(t, 5*time.Second, "vadeli ETH", func() bool {
		a, _ := vad.abonelik()
		return a["sub.deal@ETH_USDT"] && a["sub.ticker@BTC_USDT"]
	})
	bekleKosul(t, 5*time.Second, "DOGE canlı değil", func() bool { return !akisAnlik("DOGEUSDT", 0).SpotCanli })
	bekleKosul(t, 5*time.Second, "ETH canlı", func() bool { return akisAnlik("ETHUSDT", 0).SpotCanli })
	_, b2 := spot.abonelik()
	_, v2 := vad.abonelik()
	// kapasite 2: BTC+DOGE ilk hatta; DOGE çıkınca ETH aynı hatta girer (yeni bağlantı açılmaz)
	if b2 != ilkSpotBaglanti || v2 != ilkVadBaglanti {
		t.Fatalf("liste değişince bağlantı yeniden kuruldu (spot %d→%d, vadeli %d→%d)", ilkSpotBaglanti, b2, ilkVadBaglanti, v2)
	}
	_ = son
}

// 26 Ekim canlı denemede yakalandı: abonelik onayı, dakika döngüsü coini kurmadan gelince coin hiç "canlı" sayılmıyordu
// (bütün dakikalar eksik, durum "Bağlanıyor"). Onay coin kurmadan da işlenmeli; onay kaçarsa ilk veri canlı yapmalı.
func TestAkisSpotOnayYarisi(t *testing.T) {
	akisSifirla(t)
	h := &akisHat{tur: "spot", istenen: map[string]bool{"BTCUSDT": true}, sinyal: make(chan struct{}, 1)}
	h.spotMesaj(wsMetin, []byte(`{"id":0,"code":0,"msg":"spot@public.aggre.deals.v3.api.pb@100ms@BTCUSDT,spot@public.limit.depth.v3.api.pb@BTCUSDT@20"}`))
	AK.mu.Lock()
	c := AK.coinler["BTCUSDT"]
	AK.mu.Unlock()
	if c == nil || c.spotCanli == 0 {
		t.Fatalf("coin kurulmadan gelen onay işlenmeli: %+v", c)
	}
	h.spotMesaj(wsMetin, []byte(`{"id":0,"code":0,"msg":"spot@public.aggre.deals.v3.api.pb@100ms@ETHUSDT"}`))
	AK.mu.Lock()
	_, var_ := AK.coinler["ETHUSDT"]
	AK.mu.Unlock()
	if var_ {
		t.Fatal("istenmeyen coinin onayı coin kurmamalı")
	}
	// onay hiç gelmediyse: ilk işlem coini kurar ve canlı yapar, işlem sayılır
	h2 := &akisHat{tur: "spot", istenen: map[string]bool{"DOGEUSDT": true}, sinyal: make(chan struct{}, 1)}
	z := akisSimdi()
	h2.spotMesaj(wsIkili, sahteSpotIslem("DOGEUSDT", z, 0.1, 50, true))
	AK.mu.Lock()
	d := AK.coinler["DOGEUSDT"]
	var al float64
	if d != nil {
		for _, a := range d.acik {
			al += a.d.Al
		}
	}
	AK.mu.Unlock()
	if d == nil || d.spotCanli == 0 || al != 50 {
		t.Fatalf("ilk veriyle canlı olmalı ve işlem sayılmalı: %+v al=%v", d, al)
	}
	// istenmeyen coinin verisi coin kurmaz
	h2.spotMesaj(wsIkili, sahteSpotIslem("XRPUSDT", z, 0.5, 5, true))
	AK.mu.Lock()
	_, var_ = AK.coinler["XRPUSDT"]
	AK.mu.Unlock()
	if var_ {
		t.Fatal("istenmeyen coinin verisi coin kurmamalı")
	}
}
