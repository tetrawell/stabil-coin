package main

// KORUMA (0.7) — her kumbaranın kendi bekçisi.
//
// Simülatörün bulduğu ayar, hiç görmediği bir piyasaya (ör. 45 derece düşen bir trende) denk gelince art arda zarar
// edebilir: kullanıcı sanal parayla böyle bir seride %14 kaybetti. Koruma, kumbaranın gidişatını ayarın KENDİ
// GEÇMİŞİYLE karşılaştırır ve tek bir soru sorar: "Bu kötü gidişat, bu ayar için normal bir şanssızlık mı?"
// Değilse kumbara KENARA ALINIR: yeni alım yapılmaz, para kumbarada bekler. Motor çalışmaya devam eder; işlemleri
// GÖLGEDE (parasız) izlenir. Gölgede art arda yeterince kârlı işlem olunca kumbara kendiliğinden oyuna döner.
//
// Üç alarm (hangisi önce öterse). Alarmlar bir işlem KAPANDIĞINDA bakılır; kenara alındığı anda elde coin olmaz:
//   1. Art arda zarar.
//   2. Zirveden kayıp: kumbaranın (kapanan işlemlere göre) en yüksek değerinden düşüşü. Sınır, coinin kendi sınırı ile
//      kullanıcının TAVANI'ndan (bütün kumbaralar için tek sayı, ayarlar.json → koruma.tavan) küçük olanıdır.
//   3. Aşırı hızlanma: ayarın normalde 4 işlem yaptığı süre içinde, geçmişteki en yoğun dönemin f katı kadar işlem
//      yapıp toplamda zararda olmak.
//
// Sınırlar her kumbara için kumbara başlarken hesaplanır ve ömrü boyunca sabit kalır (kullanıcı elle değiştirebilir):
//   - "Torba" (bootstrap): ayarın geçmiş işlemlerinin sonuçları rastgele sırayla (geri koyarak) çekilir, kumbaranın
//     ömrü boyunca yapması beklenen işlem sayısı kadar uzunlukta binlerce olası gelecek üretilir. Hassasiyete göre
//     (sakin 99, dengeli 95, hassas 90) geleceklerin o kadarında ulaşılmayan seri ve düşüş, sınır olur.
//   - Geçmişte 20'den az işlem varsa torba güvenilmez: yedek kurallar (3 zarar; en büyük tek zararın ya da zarar
//     durdurun 2 katı kayıp; normalin 2 katı hız).
//   - Geri dönüş: gölgede art arda 2 kârlı işlem; işlemlerinin %65'inden fazlası kârlı olan ayarlarda 3.
// Geçmiş işlemler sayfada (simülatörün motoruyla) hesaplanıp gönderilir; kumbaranın açılışından önceki mumlar.

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
)

type korumaIslemi struct {
	G int64   `json:"g"` // alış mumunun açılışı (borsa saati, ms)
	C int64   `json:"c"` // satış mumunun açılışı
	Y float64 `json:"y"` // sonuç (%), simülatörün masraf varsayımıyla
}

type korumaSinirlari struct {
	Seri       int     `json:"seri"`       // bu kadar art arda zarar → kenara
	Kayip      float64 `json:"kayip"`      // coinin kendi zirveden kayıp sınırı (%)
	HizSayi    int     `json:"hizSayi"`    // HizPencere içinde bu kadar işlem + toplamda zarar → kenara (0 = hesaplanamadı)
	HizPencere int64   `json:"hizPencere"` // ms
	GeriDonus  int     `json:"geriDonus"`  // gölgede art arda kaç kârlı işlemde oyuna döner
}

type korumaKarakteri struct {
	Islem        int     `json:"islem"`
	Karli        int     `json:"karli"`
	Sure         int64   `json:"sure"`         // geçmiş verinin uzunluğu (ms)
	SaatteIslem  float64 `json:"saatteIslem"`  // normal hız
	EnUzunSeri   int     `json:"enUzunSeri"`   // geçmişte görülen en uzun zarar serisi
	EnBuyukDusus float64 `json:"enBuyukDusus"` // geçmişte görülen en büyük düşüş (%)
	EnBuyukZarar float64 `json:"enBuyukZarar"` // en büyük tek zarar (%, pozitif)
	EnYogun      int     `json:"enYogun"`      // hız penceresine sığan en çok işlem (geçmişte)
	Gelecek      int     `json:"gelecek"`      // torbadaki her gelecek kaç işlem
	Yedek        bool    `json:"yedek"`        // geçmiş az: yedek kurallar
	Stop         float64 `json:"stop,omitempty"`
}

type golgeIslem struct {
	G  int64   `json:"g"`
	C  int64   `json:"c"`
	GF float64 `json:"gf"`
	CF float64 `json:"cf"`
	Y  float64 `json:"y"`
}

type korumaDonemi struct {
	Bas   int64  `json:"bas"`
	Bit   int64  `json:"bit,omitempty"`
	Sebep string `json:"sebep"`
	Donus string `json:"donus,omitempty"`
}

// Sağlamlık testi (sayfada hesaplanır): ayarın, simülatörün ayarı bulduğu veriden ÖNCEKİ (görmediği) dönemdeki sonucu
type saglamlikSonucu struct {
	Gun   float64 `json:"gun"`
	Bot   float64 `json:"bot"`
	AlTut float64 `json:"alTut"`
	Islem int     `json:"islem"`
	Karli int     `json:"karli"`
	Dusus float64 `json:"dusus"`
	Yok   string  `json:"yok,omitempty"` // hesaplanamadıysa neden
}

type koruma struct {
	Hassasiyet  string           `json:"hassasiyet"`
	Oto         korumaSinirlari  `json:"oto"`   // hesaplanan
	Sinir       korumaSinirlari  `json:"sinir"` // geçerli (kullanıcı değiştirmiş olabilir)
	Elle        bool             `json:"elle,omitempty"`
	SeriKapali  bool             `json:"seriKapali,omitempty"`
	KayipKapali bool             `json:"kayipKapali,omitempty"` // coinin kendi kayıp sınırı kapalı (tavan yine geçerli)
	HizKapali   bool             `json:"hizKapali,omitempty"`
	Karakter    korumaKarakteri  `json:"karakter"`
	Gecmis      []korumaIslemi   `json:"gecmis"`
	Gecici      bool             `json:"gecici,omitempty"` // ayarın geçmişi henüz okunmadı: yedek sınırlar (sayfa okuyunca güncellenir)
	Baslangic   int64            `json:"baslangic"`        // alarmların saymaya başladığı an (kuruluş ya da oyuna dönüş)
	IlkNo       int              `json:"ilkNo"`            // kuruluşta var olan işlemler (açık olan dahil) alarmlarda sayılmaz
	Tepe        float64          `json:"tepe"`             // coinin kayıp sınırı için tepe (oyuna dönünce sıfırlanır)
	TavanTepe   float64          `json:"tavanTepe"`        // tavan için tepe (hiç sıfırlanmaz: tavan toplam kaybı sınırlar)
	SonDeger    float64          `json:"sonDeger"`         // son kapanan işlemden sonraki değer
	SonBakilan  int              `json:"sonBakilan"`
	DonusMum    int64            `json:"donusMum,omitempty"`  // oyuna dönülen mum: bu mumun ve öncekilerin alımı yapılmaz
	TavanNotu   bool             `json:"tavanNotu,omitempty"` // "tavana ulaşıldı, kendiliğinden dönmez" bu kenarda kalışta yazıldı
	Kenarda     bool             `json:"kenarda"`
	KenarSebep  string           `json:"kenarSebep,omitempty"`
	KenarZamani int64            `json:"kenarZamani,omitempty"`
	Golge       []golgeIslem     `json:"golge,omitempty"`
	Donemler    []korumaDonemi   `json:"donemler,omitempty"`
	Saglamlik   *saglamlikSonucu `json:"saglamlik,omitempty"`
}

// ---------- genel ayar: tavan ve yeni kumbaraların hassasiyeti (ayarlar.json → "koruma") ----------

type korumaGenelAyar struct {
	Tavan      float64 `json:"tavan"`      // 0 = tavan yok
	Hassasiyet string  `json:"hassasiyet"` // yeni kumbaralar için
}

var hassasiyetler = map[string]struct {
	q, hiz float64
	ad     string
}{
	"sakin":   {0.99, 2.0, "Sakin"},
	"dengeli": {0.95, 1.5, "Dengeli"},
	"hassas":  {0.90, 1.25, "Hassas"},
}

const (
	varsayilanTavan      = 10.0
	varsayilanHassasiyet = "dengeli"
	torbaGelecek         = 4000 // torbadan üretilen olası gelecek sayısı
	torbaEnAzIslem       = 20   // bundan az geçmiş işlemde yedek kurallar
)

var korumaGenel atomic.Value // korumaGenelAyar

func korumaAyariOku(m map[string]json.RawMessage) korumaGenelAyar {
	a := korumaGenelAyar{Tavan: varsayilanTavan, Hassasiyet: varsayilanHassasiyet}
	if v, ok := m["koruma"]; ok {
		var g struct {
			Tavan      *float64 `json:"tavan"`
			Hassasiyet string   `json:"hassasiyet"`
		}
		if json.Unmarshal(v, &g) == nil {
			if g.Tavan != nil && *g.Tavan >= 0 && *g.Tavan <= 90 && !math.IsNaN(*g.Tavan) {
				a.Tavan = *g.Tavan
			}
			if _, ok := hassasiyetler[g.Hassasiyet]; ok {
				a.Hassasiyet = g.Hassasiyet
			}
		}
	}
	return a
}

func korumaGenelYukle() {
	ayarMu.Lock()
	m := ayarlariOku()
	ayarMu.Unlock()
	korumaGenel.Store(korumaAyariOku(m))
}

func korumaGenelAl() korumaGenelAyar {
	if a, ok := korumaGenel.Load().(korumaGenelAyar); ok {
		return a
	}
	return korumaGenelAyar{Tavan: varsayilanTavan, Hassasiyet: varsayilanHassasiyet}
}

// Genel ayardan bir alanı değiştirip yazar (ayarMu'yu kendisi kilitler; durumMu kilitliyken ÇAĞRILMAZ)
func korumaGenelYaz(degistir func(*korumaGenelAyar)) error {
	ayarMu.Lock()
	defer ayarMu.Unlock()
	m := ayarlariOku()
	a := korumaAyariOku(m)
	degistir(&a)
	b, _ := json.Marshal(a)
	m["koruma"] = b
	if err := ayarlariYaz(m); err != nil {
		return err
	}
	korumaGenel.Store(a)
	return nil
}

// ---------- sınırların hesabı ----------

type korumaGirdisi struct {
	Islemler   []korumaIslemi
	Sure       int64 // geçmiş verinin uzunluğu (ms)
	AralikMs   int64
	OmurMs     int64 // kumbaranın ömrü
	Hassasiyet string
	Stop       float64 // dosyadaki zarar durdur (%), kapalıysa 0
	SimIslem   int     // simülatörün o günkü işlem sayısı (geçmiş boşsa hız için)
	SimSure    int64
}

// Geçmiş işlemleri temizler: bozuk sayılar atılır, satış zamanına göre sıralanır, en yeni 500 tanesi kalır
func korumaIslemleriTemizle(l []korumaIslemi) []korumaIslemi {
	out := make([]korumaIslemi, 0, len(l))
	for _, t := range l {
		if math.IsNaN(t.Y) || math.IsInf(t.Y, 0) || t.C < t.G || t.G <= 0 {
			continue
		}
		t.Y = math.Max(-99.9, math.Min(1000, t.Y))
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].C < out[j].C })
	if len(out) > 500 {
		out = append([]korumaIslemi(nil), out[len(out)-500:]...)
	}
	return out
}

// Bir sonuç dizisinde en uzun zarar serisi ve en büyük düşüş (%), bileşik
func seriVeDusus(y []float64) (int, float64) {
	seri, enSeri := 0, 0
	deger, tepe, dusus := 1.0, 1.0, 0.0
	for _, x := range y {
		if x < 0 {
			seri++
			enSeri = max(enSeri, seri)
		} else {
			seri = 0
		}
		deger *= 1 + x/100
		if deger > tepe {
			tepe = deger
		}
		if d := 1 - deger/tepe; d > dusus {
			dusus = d
		}
		if tepe > 1e100 { // taşmasın: ikisini birden küçült (düşüş oranı değişmez)
			deger, tepe = deger/tepe, 1
		}
	}
	return enSeri, dusus * 100
}

func yukariYuvarla(x float64, ondalik int) float64 {
	p := math.Pow(10, float64(ondalik))
	return math.Ceil(x*p-1e-9) / p
}

func korumaHesapla(g korumaGirdisi) (korumaSinirlari, korumaKarakteri) {
	h, ok := hassasiyetler[g.Hassasiyet]
	if !ok {
		h = hassasiyetler[varsayilanHassasiyet]
	}
	islemler := korumaIslemleriTemizle(g.Islemler)
	n := len(islemler)
	y := make([]float64, n)
	kar := korumaKarakteri{Islem: n, Sure: g.Sure, Stop: g.Stop}
	for i, t := range islemler {
		y[i] = t.Y
		if t.Y > 0 {
			kar.Karli++
		}
		if -t.Y > kar.EnBuyukZarar {
			kar.EnBuyukZarar = -t.Y
		}
	}
	kar.EnUzunSeri, kar.EnBuyukDusus = seriVeDusus(y)

	// normal hız (ms başına işlem)
	hiz := 0.0
	if n > 0 && g.Sure > 0 {
		hiz = float64(n) / float64(g.Sure)
	} else if g.SimIslem > 0 && g.SimSure > 0 {
		hiz = float64(g.SimIslem) / float64(g.SimSure)
	}
	kar.SaatteIslem = hiz * 3_600_000
	L := 10
	if hiz > 0 && g.OmurMs > 0 {
		L = int(math.Round(hiz * float64(g.OmurMs)))
	}
	L = max(10, min(300, L))
	kar.Gelecek = L

	var s korumaSinirlari
	if n >= torbaEnAzIslem {
		// torba: aynı geçmiş için hep aynı sayılar çıksın (tohum geçmişten)
		hs := fnv.New64a()
		for _, x := range y {
			fmt.Fprintf(hs, "%.6f;", x)
		}
		rg := rand.New(rand.NewSource(int64(hs.Sum64() & 0x7fffffffffffffff)))
		seriler := make([]int, torbaGelecek)
		dususler := make([]float64, torbaGelecek)
		for i := 0; i < torbaGelecek; i++ {
			seri, enSeri := 0, 0
			deger, tepe, dusus := 1.0, 1.0, 0.0
			for j := 0; j < L; j++ {
				x := y[rg.Intn(n)]
				if x < 0 {
					seri++
					if seri > enSeri {
						enSeri = seri
					}
				} else {
					seri = 0
				}
				deger *= 1 + x/100
				if deger > tepe {
					tepe = deger
				}
				if d := 1 - deger/tepe; d > dusus {
					dusus = d
				}
				if tepe > 1e100 {
					deger, tepe = deger/tepe, 1
				}
			}
			seriler[i], dususler[i] = enSeri, dusus*100
		}
		// seri: geleceklerin en fazla (1-q) kadarında ulaşılan en küçük sayı
		izin := int(math.Floor((1-h.q)*torbaGelecek + 1e-6))
		sort.Ints(seriler)
		esik := 1
		for esik <= L+1 {
			ulasan := torbaGelecek - sort.SearchInts(seriler, esik) // seri >= esik olan gelecek sayısı
			if ulasan <= izin {
				break
			}
			esik++
		}
		s.Seri = max(3, min(20, esik))
		sort.Float64s(dususler)
		idx := min(torbaGelecek-1, int(math.Ceil(h.q*torbaGelecek))-1)
		d := dususler[idx]
		// tek bir normal zarar (en büyük geçmiş zarar ya da dosyadaki zarar durdur) alarmı tek başına çalıştırmasın
		d = math.Max(d, math.Max(1.2*math.Max(kar.EnBuyukZarar, g.Stop), 1))
		s.Kayip = math.Min(90, yukariYuvarla(d, 1))
		if float64(kar.Karli)/float64(n) >= 0.65 {
			s.GeriDonus = 3
		} else {
			s.GeriDonus = 2
		}
	} else {
		kar.Yedek = true
		s.Seri = 3
		taban := math.Max(kar.EnBuyukZarar, g.Stop)
		if taban <= 0 {
			taban = 4
		}
		s.Kayip = math.Min(90, yukariYuvarla(2*taban, 1))
		s.GeriDonus = 2
	}

	// aşırı hızlanma: normalde 4 işlemlik süre
	if hiz > 0 {
		w := int64(4 / hiz)
		if g.AralikMs > 0 {
			w = max(w, 3*g.AralikMs)
		}
		s.HizPencere = w
		enYogun := 0
		bas := 0
		for i := range islemler {
			for islemler[i].C-islemler[bas].C > w {
				bas++
			}
			enYogun = max(enYogun, i-bas+1)
		}
		kar.EnYogun = enYogun
		if n >= torbaEnAzIslem {
			s.HizSayi = max(8, int(math.Ceil(h.hiz*float64(enYogun)-1e-9)))
		} else {
			s.HizSayi = 8
		}
	}
	return s, kar
}

// Strateji dosyasından korumanın ihtiyaç duyduğu bilgiler: zarar durdur (%) ve simülatörün o günkü sonucu
func korumaDosyaBilgisi(icerik json.RawMessage, aralikMs int64) (stop float64, simIslem int, simSure int64) {
	var ic struct {
		Strateji struct {
			Kurallar struct {
				ZararDurdur struct {
					Acik bool `json:"acik"`
					D    struct {
						Yuzde float64 `json:"yuzde"`
					} `json:"d"`
				} `json:"zararDurdur"`
			} `json:"kurallar"`
		} `json:"strateji"`
		GecmisSonuc struct {
			MumSayisi   float64 `json:"mumSayisi"`
			IslemSayisi float64 `json:"islemSayisi"`
		} `json:"gecmisSonuc"`
	}
	if json.Unmarshal(icerik, &ic) != nil {
		return
	}
	if z := ic.Strateji.Kurallar.ZararDurdur; z.Acik && z.D.Yuzde > 0 && z.D.Yuzde < 100 {
		stop = z.D.Yuzde
	}
	if ic.GecmisSonuc.IslemSayisi > 0 && ic.GecmisSonuc.MumSayisi > 0 {
		simIslem = int(ic.GecmisSonuc.IslemSayisi)
		simSure = int64(ic.GecmisSonuc.MumSayisi) * aralikMs
	}
	return
}

func korumaGirdisiYap(icerik json.RawMessage, aralik string, islemler []korumaIslemi, sure, omur int64, hassasiyet string) korumaGirdisi {
	ms := int64(60_000)
	if a, ok := araliklar[aralik]; ok {
		ms = a.ms
	}
	stop, simIslem, simSure := korumaDosyaBilgisi(icerik, ms)
	return korumaGirdisi{Islemler: islemler, Sure: max(0, sure), AralikMs: ms, OmurMs: max(0, omur), Hassasiyet: hassasiyet,
		Stop: stop, SimIslem: simIslem, SimSure: simSure}
}

// ---------- kumbaraya korumayı kurma ----------

func sonKapananNo(k *kumbara) int {
	for i := len(k.Islemler) - 1; i >= 0; i-- {
		if k.Islemler[i].Satis != nil {
			return k.Islemler[i].No
		}
	}
	return 0
}

// Sınırları hesaplar (durumMu GEREKMEZ; torba birkaç on milisaniye sürebilir, kilit dışında çağrılmalı).
// Hassasiyet geçersizse genel ayardaki kullanılır. Dönüş: hassasiyet, sınırlar, karakter, temizlenmiş geçmiş.
func korumaSinirHesapla(icerik json.RawMessage, aralik string, omur int64, islemler []korumaIslemi, sure int64, hassasiyet string) (string, korumaSinirlari, korumaKarakteri, []korumaIslemi) {
	if _, ok := hassasiyetler[hassasiyet]; !ok {
		hassasiyet = korumaGenelAl().Hassasiyet
	}
	temiz := korumaIslemleriTemizle(islemler)
	s, kar := korumaHesapla(korumaGirdisiYap(icerik, aralik, temiz, sure, omur, hassasiyet))
	return hassasiyet, s, kar, temiz
}

// durumMu kilitliyken: kumbaraya yeni koruma kurar. gecici: ayarın geçmişi henüz yok, yedek sınırlar
// (sayfa geçmişi gönderince korumaGuncelle ile asıl sınırlara geçilir; bu arada kumbara korumasız kalmaz).
func korumaKur(k *kumbara, h string, s korumaSinirlari, kar korumaKarakteri, gecmis []korumaIslemi, gecici bool) {
	deger := k.deger()
	if t := k.acikIslem(); t != nil {
		deger = k.Nakit + t.Harcanan // açık işlem varsa: alıştaki değer (maliyetiyle); kapanınca sonucu eklenir
	}
	ilk := 0
	if n := len(k.Islemler); n > 0 {
		ilk = k.Islemler[n-1].No
	}
	k.Koruma = &koruma{Hassasiyet: h, Oto: s, Sinir: s, Karakter: kar, Gecmis: gecmis, Gecici: gecici,
		Baslangic: simdiF().UnixMilli(), IlkNo: ilk, Tepe: deger, TavanTepe: deger, SonDeger: deger, SonBakilan: sonKapananNo(k)}
	if gecici {
		olayEkle(k, "bilgi", "Koruma geçici (yedek) sınırlarla kuruldu: "+korumaSinirMetni(k.Koruma, korumaGenelAl().Tavan)+". Ayarın geçmişi grafik yüklenince okunup sınırlar ona göre hesaplanacak")
	} else {
		olayEkle(k, "bilgi", "Koruma kuruldu ("+hassasiyetler[h].ad+"): "+korumaSinirMetni(k.Koruma, korumaGenelAl().Tavan))
	}
}

// durumMu kilitliyken: geçici korumayı ayarın geçmişinden hesaplanan sınırlara geçirir (durumu, tepeleri, kenarda
// olup olmadığını korur; kullanıcı sınırları elle değiştirdiyse onlara dokunmaz)
func korumaGuncelle(k *kumbara, h string, s korumaSinirlari, kar korumaKarakteri, gecmis []korumaIslemi) {
	kr := k.Koruma
	kr.Hassasiyet, kr.Oto, kr.Karakter, kr.Gecmis, kr.Gecici = h, s, kar, gecmis, false
	if !kr.Elle {
		kr.Sinir = s
	}
	olayEkle(k, "bilgi", "Koruma ayarın geçmişiyle güncellendi ("+hassasiyetler[h].ad+"): "+korumaSinirMetni(kr, korumaGenelAl().Tavan))
}

// durumMu kilitliyken: korumasız kumbaraya hemen geçici koruma (yedek sınırlar; torba yok, anında hesaplanır)
func korumaGeciciKur(k *kumbara) {
	if k.Koruma != nil || !k.aktif() {
		return
	}
	h, s, kar, _ := korumaSinirHesapla(k.Icerik, k.Aralik, k.Bitis-k.Baslangic, nil, 0, "")
	korumaKur(k, h, s, kar, nil, true)
}

func korumaSinirMetni(kr *koruma, tavan float64) string {
	var p []string
	if !kr.SeriKapali && kr.Sinir.Seri > 0 {
		p = append(p, fmt.Sprintf("art arda %d zararda", kr.Sinir.Seri))
	}
	if s, kaynak := kr.kayipSiniri(tavan); s > 0 {
		ek := ""
		if kaynak == "tavan" {
			ek = " (tavan)"
		}
		p = append(p, "%"+kisaSayi(s)+" kayıpta"+ek)
	}
	if !kr.HizKapali && kr.Sinir.HizSayi > 0 && kr.Sinir.HizPencere > 0 {
		p = append(p, fmt.Sprintf("%s içinde %d işlem ve zararda", sureUzunTR(kr.Sinir.HizPencere), kr.Sinir.HizSayi))
	}
	metin := "alarm yok"
	if len(p) > 0 {
		metin = strings.Join(p, ", ") + " kenara alınır"
	}
	metin += fmt.Sprintf("; gölgede art arda %d kârlı işlemde geri döner", kr.Sinir.GeriDonus)
	k := kr.Karakter
	if k.Yedek {
		metin += fmt.Sprintf(". Geçmişte %d işlem var (az): yedek sınırlar", k.Islem)
	} else {
		metin += fmt.Sprintf(". Geçmiş: %d işlem, %%%s kârlı", k.Islem, trSayi(float64(k.Karli)/float64(max(1, k.Islem))*100, 0))
	}
	return metin
}

// Tek ondalıklı sayı; ",0" yazılmaz ("8", "7,4")
func kisaSayi(x float64) string {
	return strings.TrimSuffix(trSayi(x, 1), ",0")
}

// Süre metni: "45 dk", "5,5 sa", "2,3 gün"
func sureUzunTR(ms int64) string {
	dk := float64(ms) / 60000
	switch {
	case dk < 90:
		return trSayi(math.Round(dk), 0) + " dk"
	case dk < 48*60:
		return kisaSayi(math.Round(dk/6)/10) + " sa"
	}
	return kisaSayi(math.Round(dk/144)/10) + " gün"
}

// Kayıp alarmının geçerli sınırı: coinin kendi sınırı (kapalı değilse) ile tavandan küçük olanı
func (kr *koruma) kayipSiniri(tavan float64) (float64, string) {
	s, kaynak := 0.0, ""
	if !kr.KayipKapali && kr.Sinir.Kayip > 0 {
		s, kaynak = kr.Sinir.Kayip, "coin"
	}
	if tavan > 0 && (s == 0 || tavan < s) {
		s, kaynak = tavan, "tavan"
	}
	return s, kaynak
}

// ---------- alarmlar ----------

type korumaOlcumu struct {
	Seri       int     `json:"seri"`
	Dusus      float64 `json:"dusus"`      // coinin sınırı için (oyuna dönünce sıfırlanan tepeden)
	TavanDusus float64 `json:"tavanDusus"` // tavan için (hiç sıfırlanmayan tepeden)
	HizIslem   int     `json:"hizIslem"`
	HizYuzde   float64 `json:"hizYuzde"`
	HizNet     float64 `json:"hizNet"`
}

// Toplam kayıp (tavan için, hiç sıfırlanmayan tepeden, %)
func (kr *koruma) tavanDususu() float64 {
	if kr.TavanTepe > 0 && kr.SonDeger < kr.TavanTepe {
		return (1 - kr.SonDeger/kr.TavanTepe) * 100
	}
	return 0
}

// Kumbara tavana ulaştı mı (tavan açıksa)
func (kr *koruma) tavanaUlasti(tavan float64) bool {
	return tavan > 0 && kr.tavanDususu() >= tavan-1e-9
}

// Bu işlem alarmlarda sayılır mı: kuruluştan sonra açılmış ve son başlangıçtan (kuruluş ya da oyuna dönüş) sonra kapanmış
func (kr *koruma) sayilir(t *kumbaraIslemi) bool {
	return t.Satis != nil && t.No > kr.IlkNo && t.Satis.Bitis >= kr.Baslangic
}

// Alarmların şimdiki değerleri: bitis anında biten pencere (hız için)
func korumaOlc(k *kumbara, bitis int64) korumaOlcumu {
	kr := k.Koruma
	var o korumaOlcumu
	for i := len(k.Islemler) - 1; i >= 0; i-- {
		t := &k.Islemler[i]
		if t.Satis == nil {
			continue
		}
		if !kr.sayilir(t) || t.Net >= 0 {
			break
		}
		o.Seri++
	}
	if kr.Tepe > 0 && kr.SonDeger < kr.Tepe {
		o.Dusus = (1 - kr.SonDeger/kr.Tepe) * 100
	}
	o.TavanDusus = kr.tavanDususu()
	if kr.Sinir.HizPencere > 0 {
		for i := range k.Islemler {
			t := &k.Islemler[i]
			if !kr.sayilir(t) || t.Satis.Bitis > bitis || t.Satis.Bitis <= bitis-kr.Sinir.HizPencere {
				continue
			}
			o.HizIslem++
			o.HizNet += t.Net
			o.HizYuzde += t.Yuzde
		}
	}
	return o
}

// Hangi alarm ötüyor? Boşsa hiçbiri.
func korumaSebebi(k *kumbara, bitis int64, tavan float64) string {
	kr := k.Koruma
	o := korumaOlc(k, bitis)
	if !kr.SeriKapali && kr.Sinir.Seri > 0 && o.Seri >= kr.Sinir.Seri {
		return fmt.Sprintf("art arda %d zarar (bu ayar için sınır %d)", o.Seri, kr.Sinir.Seri)
	}
	if tavan > 0 && o.TavanDusus >= tavan-1e-9 {
		return fmt.Sprintf("kumbara en yüksek değerinden %%%s düştü (senin tavanın %%%s)", kisaSayi(o.TavanDusus), kisaSayi(tavan))
	}
	if !kr.KayipKapali && kr.Sinir.Kayip > 0 && o.Dusus >= kr.Sinir.Kayip-1e-9 {
		return fmt.Sprintf("kumbara en yüksek değerinden %%%s düştü (bu ayarın kendi sınırı %%%s)", kisaSayi(o.Dusus), kisaSayi(kr.Sinir.Kayip))
	}
	if !kr.HizKapali && kr.Sinir.HizSayi > 0 && kr.Sinir.HizPencere > 0 && o.HizIslem >= kr.Sinir.HizSayi && o.HizNet < 0 {
		return fmt.Sprintf("son %s içinde %d işlem yaptı ve toplamda zararda (bu ayar için sınır %d işlem)", sureUzunTR(kr.Sinir.HizPencere), o.HizIslem, kr.Sinir.HizSayi)
	}
	return ""
}

// durumMu kilitliyken: yeni kapanan işlem varsa değeri ve tepeleri günceller, alarmlara bakar, gerekirse kenara alır.
// Kenara alma her zaman ZARARLI bir işlemin ardından olur (kârlı işlem kumbarayı kenara aldırmaz).
func korumaDenetle(k *kumbara) bool {
	kr := k.Koruma
	if kr == nil || kr.Kenarda || !k.aktif() || k.Emirde || k.Bekleyen != nil || k.acikIslem() != nil {
		return false
	}
	no := sonKapananNo(k)
	if no <= kr.SonBakilan {
		return false
	}
	// Değer zincirlenir: her işlemden sonra değer += işlemin net sonucu (kumbaranın işleme girmeyen parası değişmez).
	// Bakılmamış birden çok işlem varsa (normalde olmaz: her alımdan önce bakılır) aradaki tepeler de sayılır.
	var son *kumbaraIslemi
	deger := kr.SonDeger
	for i := range k.Islemler {
		t := &k.Islemler[i]
		if t.No <= kr.SonBakilan || t.Satis == nil {
			continue
		}
		son = t
		deger += t.Net
		kr.Tepe = math.Max(kr.Tepe, deger)
		kr.TavanTepe = math.Max(kr.TavanTepe, deger)
	}
	kr.SonBakilan = no
	kr.SonDeger = k.deger() // tam değer (kalan küçük artık dahil)
	kr.Tepe = math.Max(kr.Tepe, kr.SonDeger)
	kr.TavanTepe = math.Max(kr.TavanTepe, kr.SonDeger)
	if son == nil || !kr.sayilir(son) || son.Net >= 0 {
		return false
	}
	sebep := korumaSebebi(k, son.Satis.Bitis, korumaGenelAl().Tavan)
	if sebep == "" {
		return false
	}
	kenaraAl(k, sebep)
	return true
}

func kenaraAl(k *kumbara, sebep string) {
	kr := k.Koruma
	simdi := simdiF().UnixMilli()
	kr.Kenarda, kr.KenarSebep, kr.KenarZamani, kr.Golge, kr.TavanNotu = true, sebep, simdi, nil, false
	kr.Donemler = append(kr.Donemler, korumaDonemi{Bas: simdi, Sebep: sebep})
	if len(kr.Donemler) > 50 {
		kr.Donemler = append([]korumaDonemi(nil), kr.Donemler[len(kr.Donemler)-50:]...)
	}
	if kr.tavanaUlasti(korumaGenelAl().Tavan) {
		kr.TavanNotu = true
		olayEkle(k, "uyari", fmt.Sprintf("Koruma: %s. Kumbara kenara alındı: yeni alım yapılmayacak, para kumbarada bekliyor. Tavana ulaşıldığı için kendiliğinden oyuna DÖNMEZ (motor yine gölgede izlenir); istersen kumbara kartından \"Şimdi geri dön\" de", sebep))
		return
	}
	olayEkle(k, "uyari", fmt.Sprintf("Koruma: %s. Kumbara kenara alındı: yeni alım yapılmayacak, para kumbarada bekliyor. Motorun işlemleri gölgede (parasız) izleniyor; gölgede art arda %d kârlı işlem olunca kendiliğinden devam edecek", sebep, kr.Sinir.GeriDonus))
}

// mum: oyuna dönülen mumun açılışı (borsa saati); bu mumun ve öncekilerin alımı yapılmaz
func oyunaDon(k *kumbara, neden string, mum int64) {
	oyunaDonTavanla(k, neden, mum, false)
}

// tavaniSifirla: kullanıcı tavana ulaşmış kumbarayı elle döndürdü; tavan bugünkü değerden yeniden sayılır
func oyunaDonTavanla(k *kumbara, neden string, mum int64, tavaniSifirla bool) {
	kr := k.Koruma
	simdi := simdiF().UnixMilli()
	if n := len(kr.Donemler); n > 0 && kr.Donemler[n-1].Bit == 0 {
		kr.Donemler[n-1].Bit, kr.Donemler[n-1].Donus = simdi, neden
	}
	kr.Kenarda, kr.KenarSebep, kr.KenarZamani, kr.Golge, kr.TavanNotu = false, "", 0, nil, false
	kr.Baslangic, kr.DonusMum = simdi, mum
	kr.SonDeger = k.deger()
	kr.Tepe = kr.SonDeger // coinin kayıp sınırı yeniden sayar; tavanın tepesi (TavanTepe) aynı kalır
	ek := ""
	if tavaniSifirla {
		kr.TavanTepe = kr.SonDeger
		ek = "; tavan bugünkü değerden (" + trSayi(kr.SonDeger, 2) + " USDT) yeniden sayılacak"
	}
	kr.TavanTepe = math.Max(kr.TavanTepe, kr.SonDeger)
	kr.SonBakilan = sonKapananNo(k)
	olayEkle(k, "bilgi", "Koruma: "+neden+"; kumbara oyuna döndü. Alarmlar sıfırdan sayılıyor, motorun sıradaki alım kararı bekleniyor"+ek)
}

// durumMu kilitliyken (karar işlenirken): kenardaysa motorun işlemlerini gölge işlem olarak kaydeder,
// gölgede art arda yeterince kârlı işlem olduysa kumbarayı oyuna döndürür
func korumaGolge(k *kumbara, r *kararIstegi) {
	kr := k.Koruma
	if kr == nil || !kr.Kenarda || !k.aktif() {
		return
	}
	ms := k.ms()
	m := k.masraf() / 100
	fark := borsaFarki.Load()
	for _, mi := range r.MotorIslemleri {
		if mi.C == nil || mi.CF == nil || !(mi.GF > 0) || !(*mi.CF > 0) {
			continue
		}
		if mi.G+ms-fark <= kr.KenarZamani {
			continue // kenara alınmadan önce açılmış işlem
		}
		var vardi bool
		for _, g := range kr.Golge {
			if g.G == mi.G {
				vardi = true
				break
			}
		}
		if vardi {
			continue
		}
		y := ((1-m)*(*mi.CF/mi.GF)*(1-m) - 1) * 100
		if math.IsNaN(y) || math.IsInf(y, 0) {
			continue
		}
		kr.Golge = append(kr.Golge, golgeIslem{G: mi.G, C: *mi.C, GF: mi.GF, CF: *mi.CF, Y: y})
		sort.Slice(kr.Golge, func(i, j int) bool { return kr.Golge[i].G < kr.Golge[j].G })
		olayEkle(k, "bilgi", "Gölge işlem (parasız): "+farkMetni(y/100))
	}
	if len(kr.Golge) > 200 {
		kr.Golge = append([]golgeIslem(nil), kr.Golge[len(kr.Golge)-200:]...)
	}
	art := 0
	for i := len(kr.Golge) - 1; i >= 0 && kr.Golge[i].Y > 0; i-- {
		art++
	}
	if art >= max(1, kr.Sinir.GeriDonus) && k.Durum == "calisiyor" {
		if kr.tavanaUlasti(korumaGenelAl().Tavan) {
			// tavan toplam kaybı sınırlar: tavana ulaşmış kumbara kendiliğinden dönmez, kullanıcı karar verir
			if !kr.TavanNotu {
				kr.TavanNotu = true
				olayEkle(k, "uyari", fmt.Sprintf("Koruma: gölgede art arda %d kârlı işlem oldu ama kumbara tavana ulaştığı için kendiliğinden dönmüyor (toplam kayıp %%%s). İstersen kumbara kartından \"Şimdi geri dön\" de; tavan o zaman bugünkü değerden yeniden sayılır", art, kisaSayi(kr.tavanDususu())))
			}
			return
		}
		oyunaDon(k, fmt.Sprintf("gölgede art arda %d kârlı işlem", art), r.Mum)
	}
}

// ---------- ekrana giden özet ----------

func korumaOzeti(k *kumbara) map[string]any {
	kr := k.Koruma
	if kr == nil {
		return nil
	}
	kopya := *kr
	kopya.Gecmis = nil // büyük; ekrana gönderilmez
	m := map[string]any{}
	if b, err := json.Marshal(kopya); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	delete(m, "gecmis")
	tavan := korumaGenelAl().Tavan
	s, kaynak := kr.kayipSiniri(tavan)
	m["olcum"] = korumaOlc(k, simdiF().UnixMilli())
	m["tavan"] = tavan
	m["kayipEtkin"] = s
	m["kayipKaynak"] = kaynak
	m["hassasiyetAd"] = hassasiyetler[kr.Hassasiyet].ad
	return m
}

// ---------- uç noktalar ----------

type korumaVerisi struct {
	Islemler   []korumaIslemi `json:"islemler"`
	Sure       int64          `json:"sure"`
	Hassasiyet string         `json:"hassasiyet"`
}

func (v *korumaVerisi) gecerli() bool {
	return v != nil && v.Sure >= 0 && v.Sure < 50*365*86_400_000 && len(v.Islemler) <= 20000
}

// Başlat penceresi için: bu dosya ve bu geçmişle sınırlar ne olur (kaydetmez)
func korumaHesaplaIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		korumaVerisi
		Dosya string `json:"dosya"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !q.gecerli() {
		hataYaz(w, 400, "İstek okunamadı")
		return
	}
	if strings.ContainsAny(q.Dosya, `/\`) || q.Dosya == "" || q.Dosya == "." || q.Dosya == ".." {
		hataYaz(w, 400, "Dosya adı geçersiz")
		return
	}
	klasor, _ := klasorBul()
	ayarMu.Lock()
	omur := omurTablosu(ayarlariOku())
	ayarMu.Unlock()
	simdi := simdiF()
	d := dosyaOku(filepath.Join(klasor, q.Dosya), omur, simdi)
	if d.Durum == "hatali" {
		hataYaz(w, 400, "Dosya çalıştırılamaz: "+d.Hata)
		return
	}
	h, s, kar, _ := korumaSinirHesapla(d.Icerik, d.Aralik, d.Bitis-simdi.UnixMilli(), q.Islemler, q.Sure, q.Hassasiyet)
	onizleme := &koruma{Hassasiyet: h, Oto: s, Sinir: s, Karakter: kar}
	tavan := korumaGenelAl().Tavan
	ks, kaynak := onizleme.kayipSiniri(tavan)
	jsonYaz(w, 200, map[string]any{"hassasiyet": h, "sinir": s, "karakter": kar, "tavan": tavan, "kayipEtkin": ks, "kayipKaynak": kaynak})
}

// Korumasız ya da geçici korumalı kumbaraya (eski sürümden kalan, yeni ayarla devam eden, başlatırken grafiği
// yüklenmemiş olan) ayarın geçmişinden hesaplanan korumayı kurar
func kumbaraKorumaIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		korumaVerisi
		Oturum string `json:"oturum"`
		ID     string `json:"id"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) || !q.gecerli() {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	durumMu.Lock()
	k := kumbaraBul(q.ID)
	if k == nil || !k.aktif() {
		durumMu.Unlock()
		hataYaz(w, 404, "Çalışan kumbara bulunamadı")
		return
	}
	if k.Koruma != nil && !k.Koruma.Gecici {
		oz := korumaOzeti(k)
		durumMu.Unlock()
		jsonYaz(w, 200, map[string]any{"koruma": oz})
		return
	}
	icerik, aralik, omur := k.Icerik, k.Aralik, k.Bitis-k.Baslangic
	hs := ""
	if k.Koruma != nil {
		hs = k.Koruma.Hassasiyet
	}
	durumMu.Unlock()
	h, s, kar, temiz := korumaSinirHesapla(icerik, aralik, omur, q.Islemler, q.Sure, hs) // kilit dışında
	durumMu.Lock()
	defer durumMu.Unlock()
	if k = kumbaraBul(q.ID); k == nil || !k.aktif() {
		hataYaz(w, 404, "Çalışan kumbara bulunamadı")
		return
	}
	if k.Koruma != nil && k.Koruma.Gecici && k.Koruma.Hassasiyet != hs {
		hataYaz(w, 409, "Koruma bu arada değişti; sayfa birazdan yeniden dener")
		return
	}
	switch {
	case k.Koruma == nil:
		korumaKur(k, h, s, kar, temiz, false)
		degisti()
	case k.Koruma.Gecici:
		korumaGuncelle(k, h, s, kar, temiz)
		degisti()
	}
	jsonYaz(w, 200, map[string]any{"koruma": korumaOzeti(k)})
}

// Koruma ayarlarını değiştirir (onizle: sadece seçilen hassasiyetin otomatik sınırlarını hesaplar)
func kumbaraKorumaAyarIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum      string   `json:"oturum"`
		ID          string   `json:"id"`
		Onizle      bool     `json:"onizle"`
		Hassasiyet  string   `json:"hassasiyet"`
		Seri        int      `json:"seri"`
		Kayip       float64  `json:"kayip"`
		HizSayi     int      `json:"hizSayi"`
		GeriDonus   int      `json:"geriDonus"`
		SeriKapali  bool     `json:"seriKapali"`
		KayipKapali bool     `json:"kayipKapali"`
		HizKapali   bool     `json:"hizKapali"`
		Tavan       *float64 `json:"tavan"`
		GorulenOto  *struct {
			Seri      int     `json:"seri"`
			Kayip     float64 `json:"kayip"`
			HizSayi   int     `json:"hizSayi"`
			GeriDonus int     `json:"geriDonus"`
		} `json:"gorulenOto"` // pencerenin gösterdiği otomatik değerler
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	if _, ok := hassasiyetler[q.Hassasiyet]; !ok {
		hataYaz(w, 400, "Hassasiyet sakin, dengeli ya da hassas olmalı")
		return
	}
	durumMu.Lock()
	k := kumbaraBul(q.ID)
	if k == nil || !k.aktif() || k.Koruma == nil {
		durumMu.Unlock()
		hataYaz(w, 404, "Korumalı çalışan kumbara bulunamadı")
		return
	}
	icerik, aralik, omur := k.Icerik, k.Aralik, k.Bitis-k.Baslangic
	gecmis := append([]korumaIslemi(nil), k.Koruma.Gecmis...)
	sure := k.Koruma.Karakter.Sure
	durumMu.Unlock()
	_, oto, kar, _ := korumaSinirHesapla(icerik, aralik, omur, gecmis, sure, q.Hassasiyet) // kilit dışında
	if q.Onizle {
		jsonYaz(w, 200, map[string]any{"oto": oto, "karakter": kar})
		return
	}
	if g := q.GorulenOto; g != nil && (g.Seri != oto.Seri || math.Abs(g.Kayip-oto.Kayip) > 0.05 || g.HizSayi != oto.HizSayi || g.GeriDonus != oto.GeriDonus) {
		hataYaz(w, 409, "Koruma bu arada ayarın geçmişiyle güncellendi; pencereyi kapatıp yeniden aç")
		return
	}
	// kapalı alarmın kutusu boş ya da geçersizse otomatik değer kullanılır
	if q.SeriKapali && (q.Seri < 1 || q.Seri > 50) {
		q.Seri = oto.Seri
	}
	if q.KayipKapali && (math.IsNaN(q.Kayip) || q.Kayip < 0.5 || q.Kayip > 90) {
		q.Kayip = oto.Kayip
	}
	if (q.HizKapali || oto.HizPencere <= 0) && (q.HizSayi < 2 || q.HizSayi > 500) {
		q.HizSayi = oto.HizSayi
	}
	hata := ""
	switch {
	case q.Seri < 1 || q.Seri > 50:
		hata = "Art arda zarar 1 ile 50 arasında olmalı"
	case math.IsNaN(q.Kayip) || q.Kayip < 0.5 || q.Kayip > 90:
		hata = "Zirveden kayıp %0,5 ile %90 arasında olmalı"
	case oto.HizPencere > 0 && (q.HizSayi < 2 || q.HizSayi > 500):
		hata = "Aşırı hızlanmadaki işlem sayısı 2 ile 500 arasında olmalı"
	case q.GeriDonus < 1 || q.GeriDonus > 10:
		hata = "Geri dönüş için gereken kârlı gölge işlem 1 ile 10 arasında olmalı"
	case q.Tavan != nil && (math.IsNaN(*q.Tavan) || *q.Tavan < 0 || *q.Tavan > 90):
		hata = "Tavan %0 (tavan yok) ile %90 arasında olmalı"
	}
	if hata != "" {
		hataYaz(w, 400, hata)
		return
	}
	// önce tavan (genel ayar) yazılır; yazılamazsa kumbaraya da dokunulmaz
	if q.Tavan != nil {
		t := math.Round(*q.Tavan*10) / 10
		if err := korumaGenelYaz(func(a *korumaGenelAyar) { a.Tavan = t }); err != nil {
			hataYaz(w, 500, "Tavan kaydedilemedi: "+err.Error())
			return
		}
	}
	yeni := korumaSinirlari{Seri: q.Seri, Kayip: math.Round(q.Kayip*10) / 10, HizSayi: q.HizSayi, HizPencere: oto.HizPencere, GeriDonus: q.GeriDonus}
	if oto.HizPencere <= 0 {
		yeni.HizSayi = 0
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	k = kumbaraBul(q.ID)
	if k == nil || !k.aktif() || k.Koruma == nil {
		hataYaz(w, 404, "Korumalı çalışan kumbara bulunamadı")
		return
	}
	kr := k.Koruma
	kr.Hassasiyet, kr.Oto, kr.Karakter, kr.Sinir = q.Hassasiyet, oto, kar, yeni
	kr.Elle = yeni != oto
	kr.SeriKapali, kr.KayipKapali, kr.HizKapali = q.SeriKapali, q.KayipKapali, q.HizKapali
	olayEkle(k, "bilgi", "Elle: koruma ayarları değiştirildi ("+hassasiyetler[kr.Hassasiyet].ad+"). Şimdi: "+korumaSinirMetni(kr, korumaGenelAl().Tavan))
	degisti()
	jsonYaz(w, 200, map[string]bool{"tamam": true})
}

// Kenardaki kumbarayı elle oyuna döndürür
func kumbaraKorumaDonIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string `json:"oturum"`
		ID     string `json:"id"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	k := kumbaraBul(q.ID)
	if k == nil || !k.aktif() || k.Koruma == nil || !k.Koruma.Kenarda {
		hataYaz(w, 409, "Bu kumbara kenarda değil")
		return
	}
	oyunaDonTavanla(k, "elle geri döndürüldü", k.SonKararMum, k.Koruma.tavanaUlasti(korumaGenelAl().Tavan))
	degisti()
	jsonYaz(w, 200, map[string]bool{"tamam": true})
}

// Sağlamlık testinin sonucu sayfadan gelir: sayılar akla yatkın mı?
func (s *saglamlikSonucu) temizle() *saglamlikSonucu {
	if s == nil {
		return nil
	}
	sayilar := []float64{s.Gun, s.Bot, s.AlTut, s.Dusus}
	for _, x := range sayilar {
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e7 {
			return nil
		}
	}
	if s.Islem < 0 || s.Karli < 0 || s.Karli > s.Islem || s.Gun < 0 {
		return nil
	}
	if r := []rune(s.Yok); len(r) > 200 {
		s.Yok = string(r[:200])
	}
	return s
}

// Yeni kumbaranın hassasiyeti, sonrakiler için varsayılan olur
func korumaVarsayilanKaydet(h string) {
	if _, ok := hassasiyetler[h]; !ok || h == korumaGenelAl().Hassasiyet {
		return
	}
	_ = korumaGenelYaz(func(a *korumaGenelAyar) { a.Hassasiyet = h })
}
