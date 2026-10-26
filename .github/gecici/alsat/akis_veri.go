package main

// AKIŞ VERİSİ (0.9) — "Akış onayı" için MEXC'den canlı veri toplar (hesap akis_hesap.go'da):
//   - spot: her işlemi kimin başlattığı (alıcı mı satıcı mı piyasa emriyle saldırdı) ve alış-satış defteri (en iyi
//     20 basamak), wss://wbs-api.mexc.com/ws (protobuf, pb.go)
//   - vadeli (USDT perpetual): işlemler, açık pozisyon (OI, holdVol) ve fonlama, wss://contract.mexc.com/edge (JSON)
// Hepsi 1 dakikalık "akış çubuklarına" (akisDakika) toplanır: saldırgan alım/satım hacmi, fiyat kovalarına göre
// alım/satım (footprint; kova = 1 baz puan, log ölçekte), büyük işlemler, vadeli alım/satım, OI, fonlama, defter eğilimi.
// Biten her dakika kayitlar/akis/YYYY-AA-GG.jsonl'ye bir satır olarak eklenir; program açılınca son 3 gün okunur.
// Program kapalıyken geçen süre için spot işlemler REST'ten (aggTrades, saniye çözünürlüğünde) geri doldurulur;
// vadeli işlem ve OI'nin geçmişi MEXC'de yok, o dakikalar "vadeli verisi yok" kalır.
//
// Hangi coinler: çalışan kumbaraların coinleri + ekranda bakılan coin (son bakıştan 15 dk sonra bırakılır).
// Bağlantılar coin listesi değişince kopmaz: aynı bağlantıda abone olunur / bırakılır (akisHat).
// Zamanlar MEXC saatiyle (akisSimdi = bilgisayar saati + borsaFarki).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	akisGun         = 3 // bellekte ve diskte tutulan gün
	akisBakisSure   = 15 * time.Minute
	akisSpotHatta   = 14 // bir spot bağlantısında en fazla coin (MEXC: bağlantı başına 30 abonelik; coin başına 2)
	akisVadeliHatta = 40
	akisGeriDakika  = 120         // program kapalıyken kaçan en fazla kaç dakika geri doldurulur
	akisKovaLog     = 0.000099995 // ln(1.0001): 1 baz puanlık kova
)

var (
	akisSpotAdres   = ortamVeya("ALSAT_MEXC_WS", "wss://wbs-api.mexc.com/ws")
	akisVadeliAdres = ortamVeya("ALSAT_MEXC_VADELI_WS", "wss://contract.mexc.com/edge")
	akisVadeliRest  = ortamVeya("ALSAT_MEXC_VADELI_ADRES", "https://contract.mexc.com")
)

func akisSimdi() int64 { return simdiF().UnixMilli() + borsaFarki.Load() }

// 1 dakikalık akış çubuğu
type akisDakika struct {
	Z     int64        `json:"z"`            // dakikanın başı (ms, MEXC saati)
	A     float64      `json:"a,omitempty"`  // ilk işlem fiyatı
	Y     float64      `json:"y,omitempty"`  // en yüksek işlem fiyatı
	D     float64      `json:"d,omitempty"`  // en düşük
	K     float64      `json:"k,omitempty"`  // son işlem fiyatı
	Al    float64      `json:"al,omitempty"` // saldırgan alım (coin)
	Sat   float64      `json:"st,omitempty"` // saldırgan satış (coin)
	N     int          `json:"n,omitempty"`  // işlem sayısı
	BAl   float64      `json:"ba,omitempty"` // büyük işlemlerdeki alım (coin)
	BSat  float64      `json:"bs,omitempty"`
	Iz    [][3]float64 `json:"iz,omitempty"` // footprint: [kova, alım, satım], kovaya göre sıralı
	VV    bool         `json:"vv,omitempty"` // vadeli verisi bu dakikanın tamamında toplandı
	VAl   float64      `json:"va,omitempty"` // vadeli saldırgan alım (coin)
	VSat  float64      `json:"vs,omitempty"`
	OI    float64      `json:"oi,omitempty"` // dakika sonunda açık pozisyon (coin); 0 = bilinmiyor
	OIA   float64      `json:"oa,omitempty"` // dakikadaki en düşük OI
	OIY   float64      `json:"oy,omitempty"` // en yüksek
	Fon   float64      `json:"f,omitempty"`  // fonlama oranı (0.0001 = %0,01)
	DE    float64      `json:"de,omitempty"` // defter eğilimi ortalaması: (alıcı − satıcı) / toplam, fiyatın ±%0,5'inde
	Eksik bool         `json:"e,omitempty"`  // spot akışı bu dakikanın bir kısmında yoktu
	Geri  bool         `json:"g,omitempty"`  // REST'ten geri dolduruldu (defter ve büyük işlem yok)
}

type buyukIslem struct {
	Z      int64   `json:"z"`
	Fiyat  float64 `json:"f"`
	Adet   float64 `json:"a"`
	Alim   bool    `json:"al"`
	Vadeli bool    `json:"v,omitempty"`
}

// süren (henüz kapanmamış) dakika
type acikDakika struct {
	d     *akisDakika
	iz    map[int32][2]float64
	deTop float64
	deSay int
}

type akisCoin struct {
	Sembol  string
	Vadeli  string  // BTC_USDT; "" = vadelisi yok
	Kontrat float64 // vadeli kontrat boyu (coin)

	dakikalar []*akisDakika // biten dakikalar, eskiden yeniye
	acik      []*acikDakika // süren dakikalar (en fazla birkaç; geç gelen veriye 3 sn pay), eskiden yeniye

	buyukler []buyukIslem
	ornek    []float64 // son işlemlerin USDT değerleri (büyük işlem sınırı için)
	ornekI   int
	esik     float64 // büyük işlem sınırı (USDT)
	esikZ    int64

	defter    *defterAnlik
	sonIslem  int64
	sonFiyat  float64
	spotCanli int64 // spot aboneliği bu andan beri canlı (0 = değil)
	vadCanli  int64
	sonOI     float64
	sonOIZ    int64
	sonFon    float64
	ilkKayit  int64 // bu istekte canlı toplamanın başladığı an
	geriDurum string
	hata      string // abonelik reddedildiyse
}

type akisToplayici struct {
	mu                    sync.Mutex
	coinler               map[string]*akisCoin
	bakis                 map[string]time.Time
	kontrat               map[string]float64 // vadeli sembol → kontrat boyu
	kontratZ              time.Time
	spotDurum             string
	vadDurum              string
	spotSinyal, vadSinyal chan struct{}
	disk                  *os.File
	diskGun               string
	yukluMu               bool
	sahte                 bool
}

var AK = yeniAkisToplayici()

func yeniAkisToplayici() *akisToplayici {
	return &akisToplayici{coinler: map[string]*akisCoin{}, bakis: map[string]time.Time{}, kontrat: map[string]float64{},
		spotSinyal: make(chan struct{}, 1), vadSinyal: make(chan struct{}, 1)}
}

func akisKova(fiyat float64) int32  { return int32(math.Floor(math.Log(fiyat) / akisKovaLog)) }
func akisKovaFiyat(k int32) float64 { return math.Exp(float64(k) * akisKovaLog) }

func akisKlasoru() string { return filepath.Join(kayitKlasoru(), "akis") }

func sinyal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

func akisDegisti() {
	sinyal(AK.spotSinyal)
	sinyal(AK.vadSinyal)
}

// akisIste: ekranda bu coine bakılıyor (toplamaya başla / sürdür).
func akisIste(sembol string) {
	if !sembolKalip.MatchString(sembol) {
		return
	}
	AK.mu.Lock()
	_, vardi := AK.bakis[sembol]
	AK.bakis[sembol] = time.Now()
	AK.mu.Unlock()
	if !vardi {
		akisDegisti()
	}
}

// ---------- okuma (hesap ve ekran için) ----------

type akisAnlikVeri struct {
	Sembol    string
	Vadeli    bool
	Dakikalar []akisDakika // sonuncusu süren dakika olabilir (Simdiki=true)
	Simdiki   bool
	Buyukler  []buyukIslem
	Defter    *defterAnlik
	SonFiyat  float64
	SonOI     float64
	SonFon    float64
	SpotCanli bool
	VadCanli  bool
	Durum     string
}

// akisAnlik: bir coinin bas'tan bugüne dakika çubuklarının kopyası, büyük işlemler, son defter ve durum.
func akisAnlik(sembol string, bas int64) akisAnlikVeri {
	AK.mu.Lock()
	defer AK.mu.Unlock()
	c := AK.coinler[sembol]
	v := akisAnlikVeri{Sembol: sembol}
	if c == nil {
		v.Durum = "Veri toplanmıyor"
		return v
	}
	v.Vadeli = c.Vadeli != ""
	i := sort.Search(len(c.dakikalar), func(i int) bool { return c.dakikalar[i].Z >= bas })
	v.Dakikalar = make([]akisDakika, 0, len(c.dakikalar)-i+1)
	for _, d := range c.dakikalar[i:] {
		v.Dakikalar = append(v.Dakikalar, *d)
	}
	for _, a := range c.acik {
		if a.d.Z < bas || (len(v.Dakikalar) > 0 && v.Dakikalar[len(v.Dakikalar)-1].Z >= a.d.Z) {
			continue
		}
		v.Dakikalar = append(v.Dakikalar, c.acikKopya(a))
		v.Simdiki = true
	}
	j := sort.Search(len(c.buyukler), func(i int) bool { return c.buyukler[i].Z >= bas })
	v.Buyukler = append([]buyukIslem(nil), c.buyukler[j:]...)
	if c.defter != nil {
		d := *c.defter
		v.Defter = &d
	}
	v.SonFiyat, v.SonOI, v.SonFon = c.sonFiyat, c.sonOI, c.sonFon
	v.SpotCanli, v.VadCanli = c.spotCanli > 0, c.vadCanli > 0
	switch {
	case c.hata != "":
		v.Durum = c.hata
	case c.spotCanli > 0:
		v.Durum = "Canlı"
	case AK.spotDurum != "":
		v.Durum = AK.spotDurum
	default:
		v.Durum = "Bağlanıyor"
	}
	if c.geriDurum != "" {
		v.Durum += " · " + c.geriDurum
	}
	return v
}

func izDiz(m map[int32][2]float64) [][3]float64 {
	if len(m) == 0 {
		return nil
	}
	out := make([][3]float64, 0, len(m))
	for k, v := range m {
		out = append(out, [3]float64{float64(k), v[0], v[1]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// ---------- işlem / defter / OI ekleme (AK.mu kilitliyken) ----------

// süren dakikanın o anki hâli (kopya)
func (c *akisCoin) acikKopya(a *acikDakika) akisDakika {
	k := *a.d
	k.Iz = izDiz(a.iz)
	if a.deSay > 0 {
		k.DE = a.deTop / float64(a.deSay)
	}
	k.Eksik = k.Eksik || c.spotCanli == 0 || c.spotCanli > k.Z
	k.VV = c.Vadeli != "" && c.vadCanli > 0 && c.vadCanli <= k.Z
	return k
}

// acikBul: z dakikasının süren çubuğu (yoksa açar). nil: o dakika çoktan kapandı (çok geç gelen veri).
func (c *akisCoin) acikBul(zaman int64) *acikDakika {
	z := zaman / 60_000 * 60_000
	if n := len(c.dakikalar); n > 0 && z <= c.dakikalar[n-1].Z {
		return nil
	}
	n := len(c.acik)
	i := sort.Search(n, func(i int) bool { return c.acik[i].d.Z >= z })
	if i < n && c.acik[i].d.Z == z {
		return c.acik[i]
	}
	a := &acikDakika{d: &akisDakika{Z: z}, iz: map[int32][2]float64{}}
	if c.sonOI > 0 && c.vadCanli > 0 {
		a.d.OI, a.d.OIA, a.d.OIY, a.d.Fon = c.sonOI, c.sonOI, c.sonOI, c.sonFon
	}
	c.acik = append(c.acik, nil)
	copy(c.acik[i+1:], c.acik[i:])
	c.acik[i] = a
	return a
}

// dakikaKapat: bitişinden 3 sn geçmiş süren dakikaları kapatır, listeye ekler; diske yazılacakları döndürür.
func (c *akisCoin) dakikaKapat(simdi int64) []*akisDakika {
	var yazilacak []*akisDakika
	for len(c.acik) > 0 && c.acik[0].d.Z+60_000+3_000 <= simdi {
		a := c.acik[0]
		c.acik = c.acik[1:]
		d := a.d
		k := c.acikKopya(a)
		*d = k
		if !d.VV {
			d.VAl, d.VSat, d.OI, d.OIA, d.OIY, d.Fon = 0, 0, 0, 0, 0, 0
		}
		if n := len(c.dakikalar); n == 0 || c.dakikalar[n-1].Z < d.Z {
			c.dakikalar = append(c.dakikalar, d)
			yazilacak = append(yazilacak, d)
		}
	}
	// 3 günden eskiyi at
	sinir := simdi - akisGun*86_400_000
	i := 0
	for i < len(c.dakikalar) && c.dakikalar[i].Z < sinir {
		i++
	}
	if i > 0 {
		c.dakikalar = append(c.dakikalar[:0:0], c.dakikalar[i:]...)
	}
	j := 0
	for j < len(c.buyukler) && c.buyukler[j].Z < sinir {
		j++
	}
	if j > 0 || len(c.buyukler) > 6000 {
		j = max(j, len(c.buyukler)-6000)
		c.buyukler = append(c.buyukler[:0:0], c.buyukler[j:]...)
	}
	return yazilacak
}

func (t *akisToplayici) islemEkle(c *akisCoin, is akisIslem, vadeli bool) {
	a := c.acikBul(is.Z)
	if a == nil {
		return // kapanmış dakikaya ait çok geç gelen işlem: kapanmış çubuğa dokunulmaz
	}
	d := a.d
	if vadeli {
		if is.Alim {
			d.VAl += is.Adet
		} else {
			d.VSat += is.Adet
		}
		return
	}
	if d.N == 0 {
		d.A, d.Y, d.D = is.Fiyat, is.Fiyat, is.Fiyat
	}
	d.Y = math.Max(d.Y, is.Fiyat)
	d.D = math.Min(d.D, is.Fiyat)
	if is.Z >= c.sonIslem {
		d.K = is.Fiyat
		c.sonIslem, c.sonFiyat = is.Z, is.Fiyat
	}
	d.N++
	k := akisKova(is.Fiyat)
	v := a.iz[k]
	if is.Alim {
		d.Al += is.Adet
		v[0] += is.Adet
	} else {
		d.Sat += is.Adet
		v[1] += is.Adet
	}
	a.iz[k] = v
	// büyük işlem: son işlemlerin en büyük %1'lik dilimi (en az ortancanın 10 katı)
	deger := is.Fiyat * is.Adet
	if len(c.ornek) < 4000 {
		c.ornek = append(c.ornek, deger)
	} else {
		c.ornek[c.ornekI%4000] = deger
		c.ornekI++
	}
	if (is.Z-c.esikZ > 30_000 || c.esik == 0) && len(c.ornek) >= 200 {
		s := append([]float64(nil), c.ornek...)
		sort.Float64s(s)
		c.esik = math.Max(s[len(s)*99/100], 10*s[len(s)/2])
		c.esikZ = is.Z
	}
	if c.esik > 0 && deger >= c.esik {
		if is.Alim {
			d.BAl += is.Adet
		} else {
			d.BSat += is.Adet
		}
		b := buyukIslem{Z: is.Z, Fiyat: is.Fiyat, Adet: is.Adet, Alim: is.Alim}
		n := len(c.buyukler)
		if n == 0 || c.buyukler[n-1].Z <= b.Z {
			c.buyukler = append(c.buyukler, b)
		} else {
			i := sort.Search(n, func(i int) bool { return c.buyukler[i].Z > b.Z })
			c.buyukler = append(c.buyukler, buyukIslem{})
			copy(c.buyukler[i+1:], c.buyukler[i:])
			c.buyukler[i] = b
		}
	}
}

func (t *akisToplayici) defterEkle(c *akisCoin, d *defterAnlik) {
	if len(d.Alis) == 0 || len(d.Satis) == 0 {
		return
	}
	c.defter = d
	a := c.acikBul(d.Z)
	if a == nil {
		return
	}
	orta := (d.Alis[0][0] + d.Satis[0][0]) / 2
	var al, sa float64
	for _, x := range d.Alis {
		if x[0] >= orta*0.995 {
			al += x[0] * x[1]
		}
	}
	for _, x := range d.Satis {
		if x[0] <= orta*1.005 {
			sa += x[0] * x[1]
		}
	}
	if al+sa > 0 {
		a.deTop += (al - sa) / (al + sa)
		a.deSay++
	}
}

func (t *akisToplayici) tickerEkle(c *akisCoin, tk *vadeliTicker) {
	if tk.AcikP <= 0 || c.Kontrat <= 0 {
		return
	}
	oi := tk.AcikP * c.Kontrat
	if tk.Z >= c.sonOIZ {
		c.sonOI, c.sonOIZ, c.sonFon = oi, tk.Z, tk.Fon
	}
	a := c.acikBul(tk.Z)
	if a == nil {
		return
	}
	d := a.d
	d.OI, d.Fon = oi, tk.Fon
	if d.OIA == 0 || oi < d.OIA {
		d.OIA = oi
	}
	if oi > d.OIY {
		d.OIY = oi
	}
	// sonraki (henüz açılmış) dakikalar bu değeri başlangıç olarak taşısın
	for _, x := range c.acik {
		if x.d.Z > d.Z && x.d.OI == 0 {
			x.d.OI, x.d.OIA, x.d.OIY, x.d.Fon = oi, oi, oi, tk.Fon
		}
	}
}

// ---------- disk ----------

type akisSatir struct {
	S string      `json:"s"`
	D *akisDakika `json:"d,omitempty"`
	B *buyukIslem `json:"b,omitempty"`
}

func akisGunAdi(z int64) string { return time.UnixMilli(z).UTC().Format("2006-01-02") }

// AK.mu kilitliyken çağrılır
func (t *akisToplayici) diskeYaz(sembol string, dk []*akisDakika) {
	if len(dk) == 0 || t.sahte && os.Getenv("ALSAT_AKIS_KAYIT") != "1" {
		return
	}
	c := t.coinler[sembol]
	for _, d := range dk {
		gun := akisGunAdi(d.Z)
		if t.disk == nil || t.diskGun != gun {
			if t.disk != nil {
				t.disk.Close()
				t.disk = nil
			}
			os.MkdirAll(akisKlasoru(), 0755)
			f, err := os.OpenFile(filepath.Join(akisKlasoru(), gun+".jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				log.Println("akış kaydı açılamadı:", err)
				return
			}
			t.disk, t.diskGun = f, gun
			t.eskiKayitlariSil()
		}
		b, _ := json.Marshal(akisSatir{S: sembol, D: d})
		t.disk.Write(append(b, '\n'))
		if c != nil && !d.Geri {
			for i := len(c.buyukler) - 1; i >= 0 && c.buyukler[i].Z >= d.Z; i-- {
				if c.buyukler[i].Z < d.Z+60_000 {
					b, _ := json.Marshal(akisSatir{S: sembol, B: &c.buyukler[i]})
					t.disk.Write(append(b, '\n'))
				}
			}
		}
	}
}

func (t *akisToplayici) eskiKayitlariSil() {
	girdiler, _ := os.ReadDir(akisKlasoru())
	sinir := time.Now().UTC().AddDate(0, 0, -(akisGun + 1)).Format("2006-01-02")
	for _, g := range girdiler {
		ad := g.Name()
		if strings.HasSuffix(ad, ".jsonl") && strings.TrimSuffix(ad, ".jsonl") < sinir {
			os.Remove(filepath.Join(akisKlasoru(), ad))
		}
	}
}

// akisYukle: açılışta son günlerin kaydını okur. Aynı dakika birden çok satırda varsa (ör. sonradan geri doldurulan)
// en son yazılan geçerlidir.
func akisYukle() {
	AK.mu.Lock()
	defer AK.mu.Unlock()
	if AK.yukluMu {
		return
	}
	AK.yukluMu = true
	sinir := akisSimdi() - akisGun*86_400_000
	dk := map[string]map[int64]*akisDakika{}
	bk := map[string]map[string]buyukIslem{}
	for g := akisGun + 1; g >= 0; g-- {
		ad := filepath.Join(akisKlasoru(), time.Now().UTC().AddDate(0, 0, -g).Format("2006-01-02")+".jsonl")
		f, err := os.Open(ad)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 8<<20)
		for sc.Scan() {
			var s akisSatir
			if json.Unmarshal(sc.Bytes(), &s) != nil || !sembolKalip.MatchString(s.S) {
				continue
			}
			if s.D != nil && s.D.Z >= sinir && s.D.Z%60_000 == 0 {
				if dk[s.S] == nil {
					dk[s.S] = map[int64]*akisDakika{}
				}
				dk[s.S][s.D.Z] = s.D
			}
			if s.B != nil && s.B.Z >= sinir {
				if bk[s.S] == nil {
					bk[s.S] = map[string]buyukIslem{}
				}
				bk[s.S][fmt.Sprintf("%d|%g|%g|%v", s.B.Z, s.B.Fiyat, s.B.Adet, s.B.Alim)] = *s.B
			}
		}
		f.Close()
	}
	for s, m := range dk {
		c := AK.coinler[s]
		if c == nil {
			c = &akisCoin{Sembol: s}
			AK.coinler[s] = c
		}
		for _, d := range m {
			c.dakikalar = append(c.dakikalar, d)
		}
		sort.Slice(c.dakikalar, func(i, j int) bool { return c.dakikalar[i].Z < c.dakikalar[j].Z })
		for _, b := range bk[s] {
			c.buyukler = append(c.buyukler, b)
		}
		sort.Slice(c.buyukler, func(i, j int) bool { return c.buyukler[i].Z < c.buyukler[j].Z })
	}
}

// ---------- hangi coinler ----------

// istenen coinler: çalışan kumbaralar + son 15 dk'da ekranda bakılanlar
func akisIstenenler() []string {
	set := map[string]bool{}
	durumMu.Lock()
	for _, k := range D.Kumbaralar {
		if k.aktif() {
			set[k.Sembol] = true
		}
	}
	durumMu.Unlock()
	AK.mu.Lock()
	for s, z := range AK.bakis {
		if time.Since(z) < akisBakisSure {
			set[s] = true
		} else {
			delete(AK.bakis, s)
		}
	}
	AK.mu.Unlock()
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func akisBaslat() {
	akisYukle()
	go akisDakikaDongusu()
	if testModu {
		AK.mu.Lock()
		AK.sahte = true
		AK.mu.Unlock()
		go akisSahteDongu()
		return
	}
	go akisHatYoneticisi(context.Background(), "spot", akisSpotHatta, akisIstenenler, AK.spotSinyal)
	go akisHatYoneticisi(context.Background(), "vadeli", akisVadeliHatta, akisVadeliIstenenler, AK.vadSinyal)
}

// her saniye: yeni istenen coinleri hazırla, sessiz coinlerin dakikasını kapat, geri doldurmayı başlat
func akisDakikaDongusu() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	son := map[string]bool{}
	for range t.C {
		son = akisDakikaTuru(son)
	}
}

func akisDakikaTuru(son map[string]bool) map[string]bool {
	simdi := akisSimdi()
	istenen := akisIstenenler()
	yeni := map[string]bool{}
	var baslayan []string
	AK.mu.Lock()
	for _, s := range istenen {
		yeni[s] = true
		c := AK.coinler[s]
		if c == nil {
			c = &akisCoin{Sembol: s}
			AK.coinler[s] = c
		}
		if !son[s] {
			// yeni istendi (ya da yeniden): canlı toplama bu andan başlar, öncesi geri doldurulur
			c.ilkKayit, c.acik = simdi, nil
			baslayan = append(baslayan, s)
		}
		c.acikBul(simdi) // işlem olmasa da her dakika bir çubuk olsun
		AK.diskeYaz(s, c.dakikaKapat(simdi))
	}
	if AK.disk != nil {
		AK.disk.Sync()
	}
	AK.mu.Unlock()
	if len(baslayan) > 0 || len(yeni) != len(son) {
		akisDegisti()
	}
	for _, s := range baslayan {
		go func(s string) {
			time.Sleep(3 * time.Second) // abonelik otursun; ilk canlı dakika belli olsun
			akisGeriDoldur(s)
		}(s)
	}
	return yeni
}

// ---------- websocket hatları ----------

// akisHat: tek websocket bağlantısı. İstenen küme değişince bağlantıyı koparmadan abone olur / bırakır.
type akisHat struct {
	tur     string // spot | vadeli
	mu      sync.Mutex
	istenen map[string]bool // spot: BTCUSDT, vadeli: BTC_USDT
	sinyal  chan struct{}
	iptal   context.CancelFunc
}

func (h *akisHat) ayarla(liste []string) {
	h.mu.Lock()
	h.istenen = map[string]bool{}
	for _, s := range liste {
		h.istenen[s] = true
	}
	h.mu.Unlock()
	sinyal(h.sinyal)
}

func (h *akisHat) istiyorMu(s string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.istenen[s]
}

func (h *akisHat) liste() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.istenen))
	for s := range h.istenen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// coinleri hatlara dağıtır; her coin bir hatta kalır (liste değişse de taşınmaz)
func akisHatYoneticisi(ana context.Context, tur string, kapasite int, istenenFn func() []string, sinyalC chan struct{}) {
	var hatlar []*akisHat
	atama := map[string]*akisHat{}
	icerik := map[*akisHat]map[string]bool{}
	for {
		liste := istenenFn()
		set := map[string]bool{}
		for _, s := range liste {
			set[s] = true
		}
		degisen := map[*akisHat]bool{}
		for s, h := range atama {
			if !set[s] {
				delete(atama, s)
				delete(icerik[h], s)
				degisen[h] = true
			}
		}
		for _, s := range liste {
			if atama[s] != nil {
				continue
			}
			var h *akisHat
			for _, x := range hatlar {
				if len(icerik[x]) < kapasite {
					h = x
					break
				}
			}
			if h == nil {
				ctx, iptal := context.WithCancel(ana)
				h = &akisHat{tur: tur, istenen: map[string]bool{}, sinyal: make(chan struct{}, 1), iptal: iptal}
				hatlar = append(hatlar, h)
				icerik[h] = map[string]bool{}
				go h.calis(ctx)
			}
			icerik[h][s] = true
			atama[s] = h
			degisen[h] = true
		}
		for h := range degisen {
			if len(icerik[h]) == 0 {
				h.iptal()
				delete(icerik, h)
				for i, x := range hatlar {
					if x == h {
						hatlar = append(hatlar[:i], hatlar[i+1:]...)
						break
					}
				}
				continue
			}
			var l []string
			for s := range icerik[h] {
				l = append(l, s)
			}
			h.ayarla(l)
		}
		select {
		case <-ana.Done():
			for _, h := range hatlar {
				h.iptal()
			}
			return
		case <-sinyalC:
			time.Sleep(300 * time.Millisecond) // art arda değişiklikleri topla
		case <-time.After(15 * time.Second):
		}
	}
}

func (h *akisHat) calis(ctx context.Context) {
	bekle := time.Second
	for ctx.Err() == nil {
		basla := time.Now()
		err := h.baglanti(ctx)
		h.canliSifirla(h.liste())
		if ctx.Err() != nil {
			return
		}
		AK.mu.Lock()
		if err != nil {
			if h.tur == "spot" {
				AK.spotDurum = "Bağlantı koptu, yeniden bağlanıyor"
			} else {
				AK.vadDurum = "Vadeli bağlantı koptu, yeniden bağlanıyor"
			}
		}
		AK.mu.Unlock()
		if err != nil {
			log.Printf("akış (%s): %v", h.tur, err)
		}
		if time.Since(basla) > 5*time.Minute {
			bekle = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(bekle):
		}
		bekle = min(bekle*2, time.Minute)
	}
}

func (h *akisHat) canliSifirla(liste []string) {
	AK.mu.Lock()
	defer AK.mu.Unlock()
	for _, s := range liste {
		for _, c := range AK.coinler {
			if h.tur == "spot" && c.Sembol == s {
				c.spotCanli = 0
			}
			if h.tur == "vadeli" && c.Vadeli == s {
				c.vadCanli = 0
			}
		}
	}
}

func (h *akisHat) mesajlar(s string, abone bool) []string {
	var out []string
	if h.tur == "spot" {
		yontem := "SUBSCRIPTION"
		if !abone {
			yontem = "UNSUBSCRIPTION"
		}
		b, _ := json.Marshal(map[string]any{"method": yontem, "params": []string{
			"spot@public.aggre.deals.v3.api.pb@100ms@" + s, "spot@public.limit.depth.v3.api.pb@" + s + "@20"}})
		return []string{string(b)}
	}
	on := "sub."
	if !abone {
		on = "unsub."
	}
	for _, y := range []string{"deal", "ticker"} {
		b, _ := json.Marshal(map[string]any{"method": on + y, "param": map[string]string{"symbol": s}})
		out = append(out, string(b))
	}
	return out
}

func (h *akisHat) baglanti(ctx context.Context) error {
	adres := akisSpotAdres
	if h.tur == "vadeli" {
		adres = akisVadeliAdres
	}
	w, err := wsBaglan(ctx, adres, 15*time.Second)
	if err != nil {
		return err
	}
	bitti := make(chan struct{})
	defer close(bitti)
	defer w.Kapat()
	go func() {
		select {
		case <-ctx.Done():
			w.conn.Close()
		case <-bitti:
		}
	}()
	abone := map[string]bool{}
	esitle := func() error {
		istenen := h.liste()
		set := map[string]bool{}
		for _, s := range istenen {
			set[s] = true
			if !abone[s] {
				for _, m := range h.mesajlar(s, true) {
					if err := w.YazMetin(m); err != nil {
						return err
					}
				}
				abone[s] = true
			}
		}
		for s := range abone {
			if !set[s] {
				for _, m := range h.mesajlar(s, false) {
					if err := w.YazMetin(m); err != nil {
						return err
					}
				}
				delete(abone, s)
				h.canliSifirla([]string{s})
			}
		}
		return nil
	}
	if err := esitle(); err != nil {
		return err
	}
	// yazıcı: abonelik değişiklikleri ve ping (okuma beklerken de çalışır)
	yazHata := make(chan error, 1)
	go func() {
		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		for {
			var err error
			select {
			case <-bitti:
				return
			case <-h.sinyal:
				err = esitle()
			case <-ping.C:
				if h.tur == "spot" {
					err = w.YazMetin(`{"method":"PING"}`)
				} else {
					err = w.YazMetin(`{"method":"ping"}`)
				}
			}
			if err != nil {
				yazHata <- err
				w.conn.Close()
				return
			}
		}
	}()
	acilis := time.Now()
	for {
		if time.Since(acilis) > 23*time.Hour {
			return fmt.Errorf("bağlantı 23 saati doldu, yenileniyor")
		}
		op, veri, err := w.Oku(50 * time.Second)
		if err != nil {
			select {
			case e := <-yazHata:
				return e
			default:
			}
			return err
		}
		if h.tur == "spot" {
			h.spotMesaj(op, veri)
		} else if op == wsMetin {
			if err := h.vadeliMesaj(veri); err != nil {
				return err
			}
		}
	}
}

func (h *akisHat) spotMesaj(op byte, veri []byte) {
	simdi := akisSimdi()
	if op == wsMetin {
		tamam, mesaj := spotKontrol(veri)
		if !tamam || strings.Contains(mesaj, "Not Subscribed") || strings.Contains(mesaj, "Blocked") {
			log.Printf("akış (spot): MEXC: %s", kisalt(mesaj, 200))
			AK.mu.Lock()
			for s, c := range AK.coinler {
				if strings.Contains(mesaj, "@"+s) {
					c.hata = "MEXC bu coinin canlı akışını vermedi"
				}
			}
			AK.mu.Unlock()
			return
		}
		if !strings.Contains(mesaj, "spot@") {
			return // PONG
		}
		AK.mu.Lock()
		for _, kanal := range strings.Split(mesaj, ",") {
			p := strings.Split(kanal, "@")
			for _, x := range p {
				if c := AK.coinler[x]; c != nil && h.istiyorMu(x) && c.spotCanli == 0 {
					c.spotCanli, c.hata = simdi, ""
				}
			}
		}
		AK.spotDurum = ""
		AK.mu.Unlock()
		return
	}
	p, err := mexcCoz(veri)
	if err != nil || p.Sembol == "" {
		return // tek bozuk mesaj bağlantıyı koparmasın
	}
	AK.mu.Lock()
	defer AK.mu.Unlock()
	c := AK.coinler[p.Sembol]
	if c == nil {
		return
	}
	for _, is := range p.Islemler {
		AK.islemEkle(c, is, false)
	}
	if p.Defter != nil {
		if p.Defter.Z == 0 {
			p.Defter.Z = simdi
		}
		AK.defterEkle(c, p.Defter)
	}
}

func (h *akisHat) vadeliMesaj(veri []byte) error {
	p, err := vadeliCoz(veri)
	if err != nil {
		return nil
	}
	switch p.Kanal {
	case "rs.error":
		log.Printf("akış (vadeli): MEXC: %s", kisalt(string(veri), 200))
		return nil
	case "push.deal", "push.ticker":
	default:
		return nil
	}
	if !h.istiyorMu(p.Sembol) {
		return nil
	}
	simdi := akisSimdi()
	AK.mu.Lock()
	defer AK.mu.Unlock()
	for _, c := range AK.coinler {
		if c.Vadeli == "" || c.Vadeli != p.Sembol {
			continue
		}
		if c.vadCanli == 0 {
			c.vadCanli = simdi
			AK.vadDurum = ""
		}
		for _, is := range p.Islemler {
			is.Adet *= c.Kontrat
			AK.islemEkle(c, is, true)
		}
		if p.Ticker != nil {
			AK.tickerEkle(c, p.Ticker)
		}
	}
	return nil
}

// MEXC'deki USDT vadeli kontratların kontrat boyları (12 saatte bir yenilenir).
func akisKontratlar() map[string]float64 {
	AK.mu.Lock()
	if time.Since(AK.kontratZ) < 12*time.Hour && len(AK.kontrat) > 0 {
		m := AK.kontrat
		AK.mu.Unlock()
		return m
	}
	AK.mu.Unlock()
	b, err := getJSON(akisVadeliRest + "/api/v1/contract/detail")
	if err != nil {
		return nil
	}
	m := kontratCoz(b)
	if len(m) == 0 {
		return nil
	}
	AK.mu.Lock()
	AK.kontrat, AK.kontratZ = m, time.Now()
	AK.mu.Unlock()
	return m
}

func kontratCoz(b []byte) map[string]float64 {
	var r struct {
		Data []struct {
			Symbol       string  `json:"symbol"`
			ContractSize float64 `json:"contractSize"`
			State        int     `json:"state"`
			QuoteCoin    string  `json:"quoteCoin"`
			SettleCoin   string  `json:"settleCoin"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	m := map[string]float64{}
	for _, x := range r.Data {
		if x.State == 0 && x.QuoteCoin == "USDT" && x.SettleCoin == "USDT" && x.ContractSize > 0 {
			m[x.Symbol] = x.ContractSize
		}
	}
	return m
}

// vadeli istenenler: istenen spot coinlerin MEXC'de vadelisi olanları (coin kaydına kontrat boyu yazılır)
func akisVadeliIstenenler() []string {
	k := akisKontratlar()
	istenen := akisIstenenler()
	var out []string
	AK.mu.Lock()
	defer AK.mu.Unlock()
	for _, s := range istenen {
		c := AK.coinler[s]
		if c == nil {
			continue
		}
		v := vadeliAdi(s)
		if boy := k[v]; boy > 0 {
			c.Vadeli, c.Kontrat = v, boy
			out = append(out, v)
		} else if k != nil {
			c.Vadeli, c.Kontrat = "", 0
		}
	}
	return out
}

// ---------- geri doldurma (REST aggTrades) ----------

var akisGeriKilit sync.Map // sembol → *sync.Mutex

// akisGeriSuresi: geri doldurulacak en uzun süre (dakika). Bu coinin kumbaralarının en uzun mum süresine göre: yapı, emir
// akışı ve CVD hesabı için yeterli mum olsun (1–5 dk: 2 saat; 15 dk: 5,5 saat; 1 saat ve üstü: 6 saat).
func akisGeriSuresi(sembol string) int {
	enUzun := int64(60_000)
	durumMu.Lock()
	for _, k := range D.Kumbaralar {
		if k.aktif() && k.Sembol == sembol {
			enUzun = max(enUzun, k.ms())
		}
	}
	durumMu.Unlock()
	return int(min(360, max(int64(akisGeriDakika), enUzun/60_000*22)))
}

func akisGeriDoldur(sembol string) {
	if testModu {
		akisSahteGeri(sembol) // sadece ALSAT_TEST=1: ekran denemesi için uydurma geçmiş (sahte_akis.go)
		return
	}
	m, _ := akisGeriKilit.LoadOrStore(sembol, &sync.Mutex{})
	mu := m.(*sync.Mutex)
	if !mu.TryLock() {
		return
	}
	defer mu.Unlock()
	geri := akisGeriSuresi(sembol)
	AK.mu.Lock()
	c := AK.coinler[sembol]
	if c == nil || c.ilkKayit == 0 {
		AK.mu.Unlock()
		return
	}
	// aralık: elimizdeki son tam dakikadan (yoksa 2 saat öncesinden) canlı toplamanın başladığı dakikaya
	bitis := c.ilkKayit / 60_000 * 60_000
	bas := bitis - int64(geri)*60_000
	for i := len(c.dakikalar) - 1; i >= 0; i-- {
		if d := c.dakikalar[i]; d.Z < bitis && !d.Eksik {
			bas = max(bas, d.Z+60_000)
			break
		}
	}
	AK.mu.Unlock()
	if bitis-bas < 2*60_000 {
		return
	}
	islemler, kapsam, err := akisAggTrades(sembol, bas, bitis-1, 60)
	AK.mu.Lock()
	defer AK.mu.Unlock()
	if err != nil {
		c.geriDurum = "geçmiş alınamadı"
		return
	}
	istenenBas := bas
	if kapsam > bas {
		// istek sınırına takıldı: getirilemeyen baştaki dakikalar "işlem yok" diye uydurulmaz, eksik kalır
		bas = (kapsam + 59_999) / 60_000 * 60_000
	}
	c.geriDurum = ""
	if bas < bitis {
		eklenen := akisGeriEkle(c, islemler, bas, bitis)
		AK.diskeYaz(sembol, eklenen)
	}
	if bas > istenenBas+5*60_000 {
		c.geriDurum = "kısa geçmiş"
	}
}

// akisGeriEkle: işlemleri dakikalara toplayıp [bas, bitis) aralığındaki eksik ya da hiç olmayan dakikaların yerine koyar.
// Tam (eksiksiz) dakikalara dokunmaz. Eklenen dakikaları döndürür (AK.mu kilitliyken).
func akisGeriEkle(c *akisCoin, islemler []akisIslem, bas, bitis int64) []*akisDakika {
	cubuk := map[int64]*akisDakika{}
	iz := map[int64]map[int32][2]float64{}
	for _, is := range islemler {
		z := is.Z / 60_000 * 60_000
		if z < bas || z >= bitis {
			continue
		}
		d := cubuk[z]
		if d == nil {
			d = &akisDakika{Z: z, A: is.Fiyat, Y: is.Fiyat, D: is.Fiyat, Geri: true}
			cubuk[z] = d
			iz[z] = map[int32][2]float64{}
		}
		d.Y, d.D, d.K = math.Max(d.Y, is.Fiyat), math.Min(d.D, is.Fiyat), is.Fiyat
		d.N++
		k := akisKova(is.Fiyat)
		v := iz[z][k]
		if is.Alim {
			d.Al += is.Adet
			v[0] += is.Adet
		} else {
			d.Sat += is.Adet
			v[1] += is.Adet
		}
		iz[z][k] = v
	}
	tam := map[int64]bool{}
	var kalan []*akisDakika
	for _, d := range c.dakikalar {
		if d.Z >= bas && d.Z < bitis && d.Eksik {
			continue // geri doldurulan çubukla değişecek
		}
		if d.Z >= bas && d.Z < bitis {
			tam[d.Z] = true
		}
		kalan = append(kalan, d)
	}
	var eklenen []*akisDakika
	for z := bas; z < bitis; z += 60_000 {
		if tam[z] {
			continue
		}
		d := cubuk[z]
		if d == nil {
			d = &akisDakika{Z: z, Geri: true} // işlem olmamış dakika
		} else {
			d.Iz = izDiz(iz[z])
		}
		eklenen = append(eklenen, d)
	}
	c.dakikalar = append(kalan, eklenen...)
	sort.Slice(c.dakikalar, func(i, j int) bool { return c.dakikalar[i].Z < c.dakikalar[j].Z })
	return eklenen
}

// akisAggTrades: [bas, bit] aralığındaki spot işlemler (eskiden yeniye). MEXC tek istekte en çok 1 saatlik aralık ve
// 1000 işlem verir (en yeniden eskiye); bitiş geriye kaydırılarak sayfalanır. Zamanlar saniye çözünürlüğünde gelir;
// aynı saniye yeniden istenince tekrar gelenler atılır.
// İkinci dönüş değeri: tam getirilen aralığın başı (istek sınırına takılmadıysa bas; takıldıysa daha geç bir an).
func akisAggTrades(sembol string, bas, bit int64, enFazlaIstek int) ([]akisIslem, int64, error) {
	var hepsi []akisIslem
	gorulen := map[string]bool{}
	son := bit
	kapsam := bit + 1
	for istek := 0; istek < enFazlaIstek && son >= bas; istek++ {
		ilk := max(bas, son-3_599_000)
		q := url.Values{"symbol": {sembol}, "startTime": {strconv.FormatInt(ilk, 10)}, "endTime": {strconv.FormatInt(son, 10)}, "limit": {"1000"}}
		b, err := piyasaGet("/api/v3/aggTrades", q)
		if err != nil {
			if len(hepsi) > 0 {
				break
			}
			return nil, 0, err
		}
		var ham []struct {
			P string `json:"p"`
			Q string `json:"q"`
			T int64  `json:"T"`
			M bool   `json:"m"`
		}
		if err := json.Unmarshal(b, &ham); err != nil {
			return nil, 0, err
		}
		enEski := son
		yeniVar := false
		for _, x := range ham {
			anahtar := strconv.FormatInt(x.T, 10) + "|" + x.P + "|" + x.Q + "|" + strconv.FormatBool(x.M)
			if x.T < enEski {
				enEski = x.T
			}
			if gorulen[anahtar] {
				continue
			}
			gorulen[anahtar] = true
			f, _ := strconv.ParseFloat(x.P, 64)
			a, _ := strconv.ParseFloat(x.Q, 64)
			if f > 0 && a > 0 && x.T >= bas && x.T <= bit {
				hepsi = append(hepsi, akisIslem{Z: x.T, Fiyat: f, Adet: a, Alim: !x.M})
				yeniVar = true
			}
		}
		switch {
		case len(ham) < 1000: // bu aralık tamam
			kapsam = ilk
			if ilk <= bas {
				son = bas - 1
			} else {
				son = ilk - 1
			}
		case !yeniVar: // aynı saniyede 1000'den fazla işlem: o saniyenin kalanı atlanır
			kapsam = enEski
			son = enEski - 1
		default: // en eski saniye yarım gelmiş olabilir: bir sonraki istekte yeniden istenir
			kapsam = min(kapsam, enEski+1000)
			son = enEski
		}
	}
	if son < bas {
		kapsam = bas
	}
	sort.SliceStable(hepsi, func(i, j int) bool { return hepsi[i].Z < hepsi[j].Z })
	return hepsi, kapsam, nil
}

// ---------- test modu: sahte akış ----------

// ALSAT_TEST=1: internet olmadan ekranın denenebilmesi için sahte mumların fiyat yolundan (sahte_piyasa.go)
// türetilen işlemler, defter ve açık pozisyon. Gerçek kullanımda hiç çalışmaz.
func akisSahteDongu() {
	rg := rand.New(rand.NewSource(7))
	oi := map[string]float64{}
	son := map[string]float64{}
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		istenen := akisIstenenler()
		simdi := akisSimdi()
		for _, s := range istenen {
			f := sahteFiyatAn(s, simdi)
			if f <= 0 {
				continue
			}
			yon := 0.0
			if son[s] > 0 {
				yon = math.Max(-0.3, math.Min(0.3, (f-son[s])/son[s]*3000))
			}
			son[s] = f
			AK.mu.Lock()
			c := AK.coinler[s]
			if c == nil {
				c = &akisCoin{Sembol: s}
				AK.coinler[s] = c
			}
			if c.spotCanli == 0 {
				c.spotCanli = simdi
				c.Vadeli, c.Kontrat, c.vadCanli = vadeliAdi(s), 1, simdi
			}
			for i := 0; i < 1+rg.Intn(4); i++ {
				adet := (20 + rg.ExpFloat64()*80) / f
				if rg.Float64() < 0.01 {
					adet *= 60
				}
				AK.islemEkle(c, akisIslem{Z: simdi, Fiyat: f * (1 + (rg.Float64()-0.5)*0.0004), Adet: adet, Alim: rg.Float64() < 0.5+yon}, false)
				AK.islemEkle(c, akisIslem{Z: simdi, Fiyat: f, Adet: adet * 3, Alim: rg.Float64() < 0.5+yon/2}, true)
			}
			d := &defterAnlik{Z: simdi}
			for i := 0; i < 20; i++ {
				d.Alis = append(d.Alis, [2]float64{f * (1 - 0.0003*float64(i+1)), (50 + rg.Float64()*300) / f})
				d.Satis = append(d.Satis, [2]float64{f * (1 + 0.0003*float64(i+1)), (50 + rg.Float64()*300) / f})
			}
			AK.defterEkle(c, d)
			if oi[s] == 0 {
				oi[s] = 1_000_000 / f
			}
			oi[s] *= 1 + (rg.Float64()-0.5)*0.0006 + yon*0.0003
			AK.tickerEkle(c, &vadeliTicker{Z: simdi, Fiyat: f, AcikP: oi[s], Fon: 0.0001})
			AK.mu.Unlock()
		}
	}
}
