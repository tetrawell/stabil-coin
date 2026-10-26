package main

// AKIŞ ONAYI — KUMBARALARA BAĞLANTI (0.9). Kullanıcının seçimi: "Mevcut kumbaralara onay versin". Simülatörün stratejisi
// "al" dediğinde kumbara, akış sistemi de onay verirse alır (tek kumbara, iki onay). Kullanıcının planı: simülatörde
// gevşek (çok sinyalli) strateji bulmak; akış onayı bu sinyalleri süzer.
//
//  1. kararIsle "al" diyecekken akisOnayKancasi çağrılır (durumMu kilitli). Onay uygulanıyorsa alım hemen yapılmaz:
//     bir kayıt açılır, hesap arka planda yapılır (akisOnayHesapla; ağdan mum gerekebilir, kilit tutulmaz).
//  2. Onay → alım her zamanki gibi yapılır (limit → 15 sn → piyasa; gerçek parada aynı emir yolu). Red → BeklemeMum > 0
//     ise o kadar mum boyunca 2 sn'de bir yeniden bakılır (akisBeklemeDongusu). Motor işlemden çıkarsa, süre dolarsa,
//     fiyat zarar durdura inerse ya da hedefe çıkarsa alınmaz; fiyat motorun fiyatından Kovalama'dan fazla yukarıdaysa beklenir.
//  3. Onay kapalıysa (genel ayar ya da bu kumbarada) ya da kumbara korumada kenardaysa alım eskisi gibi karar verilir;
//     sistemin ne diyeceği yine hesaplanıp "gölge" kaydedilir (karnede sayılır, alıma karışmaz).
//  4. Her kaydın sonucu motorun o işlemi kapanınca yazılır (kararIstegi.MotorIslemleri; simülatörün masrafıyla):
//     MotorSonuc = simülatörün kendi işlemi; Sonuc = süzgeçle (bekleyip onaylandıysa onay fiyatından girilmiş gibi).
//     Sonucu belli olan kayıtlar kayitlar/akis_karne.jsonl'ye de yazılır (kumbaralar silinse de karne kalsın).
// Onay sadece ALIMI süzer; satışlara, zarar durdura, korumaya, emir yürütücüye karışmaz.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type onayKaydi struct {
	MotorMum   int64       `json:"motorMum"`           // motorun alış mumu (açılış zamanı)
	Z          int64       `json:"z"`                  // sinyalin geldiği an (bu bilgisayarın saati)
	Fiyat      float64     `json:"fiyat"`              // motorun alış fiyatı
	Karar      string      `json:"karar"`              // hesaplaniyor | onay | red | bekliyor | bekleyip-onay | sure-doldu | motor-cikti | zarar-durdur | fiyat-kacti | iptal | hata | golge-onay | golge-red
	Uygulandi  bool        `json:"uygulandi"`          // onay alımı gerçekten süzdü mü (değilse gölge)
	Golge      string      `json:"golge,omitempty"`    // gölgeyse neden: "onay kapalı" | "kenarda"
	AyarZ      int64       `json:"ayarZ"`              // hesaplandığı ayarların değiştiği an (karnede "bu ayarlarla" süzgeci)
	Toplam     float64     `json:"toplam"`             // sinyal anındaki değerlendirme
	Esik       float64     `json:"esik"`               //
	Bloklar    [4]*float64 `json:"bloklar"`            // Yapı, Emir akışı, CVD, Açık pozisyon (nil: verisi yoktu)
	Engel      string      `json:"engel,omitempty"`    // eşikten bağımsız engel (veri eksik ya da yasak)
	Baglam     string      `json:"baglam,omitempty"`   //
	Ozet       string      `json:"ozet"`               //
	Not        string      `json:"not,omitempty"`      // sonradan olan (bekleme bitti, iptal sebebi)
	SonBakis   int64       `json:"sonBakis,omitempty"` //
	SonToplam  float64     `json:"sonToplam,omitempty"`
	OnayZ      int64       `json:"onayZ,omitempty"` // bekleyip onaylandıysa
	OnayFiyat  float64     `json:"onayFiyat,omitempty"`
	OnayOzet   string      `json:"onayOzet,omitempty"`
	Masraf     float64     `json:"masraf"`               // simülatörün her yön için masrafı (%)
	Cikis      float64     `json:"cikis,omitempty"`      // motorun satış fiyatı
	CikisMum   int64       `json:"cikisMum,omitempty"`   //
	MotorSonuc *float64    `json:"motorSonuc,omitempty"` // simülatör tek başına (%)
	Sonuc      *float64    `json:"sonuc,omitempty"`      // süzgeçle (%), bekleyip onaylandıysa onay fiyatından
}

type onayBekleme struct {
	MotorMum int64   `json:"motorMum"`
	Fiyat    float64 `json:"fiyat"`
	Bitis    int64   `json:"bitis"` // bu bilgisayarın saatiyle
}

type akisKumbara struct {
	Kapali   bool         `json:"kapali,omitempty"` // bu kumbarada onay kapalı (gölge kaydedilir)
	Bekleme  *onayBekleme `json:"bekleme,omitempty"`
	Kayitlar []onayKaydi  `json:"kayitlar,omitempty"`
}

const akisKayitSiniri = 300

// testlerde yerine sahtesi konur
var (
	akisDegerlendirF = akisDegerlendir
	akisUyku         = time.Sleep
	akisHesapBaslat  = func(k *kumbara, motorMum int64) { go akisOnayHesapla(k, motorMum) }
)

func (k *kumbara) akisUygulaniyor() bool {
	return akisAyarAl().Acik && (k.Akis == nil || !k.Akis.Kapali)
}

func (k *kumbara) onayKaydiBul(motorMum int64) *onayKaydi {
	if k.Akis == nil {
		return nil
	}
	for i := len(k.Akis.Kayitlar) - 1; i >= 0; i-- {
		if k.Akis.Kayitlar[i].MotorMum == motorMum {
			return &k.Akis.Kayitlar[i]
		}
	}
	return nil
}

// akisKayitAc: durumMu kilitliyken. Yeni sinyalin kaydını açar ve hesabı başlatır. Kayıt zaten varsa false.
func akisKayitAc(k *kumbara, r *kararIstegi, uygula bool, golge string) bool {
	if k.Akis == nil {
		k.Akis = &akisKumbara{}
	}
	if k.onayKaydiBul(r.Mum) != nil {
		return false
	}
	k.Akis.Kayitlar = append(k.Akis.Kayitlar, onayKaydi{MotorMum: r.Mum, Z: simdiF().UnixMilli(), Fiyat: r.Olay.Fiyat, Karar: "hesaplaniyor",
		Uygulandi: uygula, Golge: golge, AyarZ: akisAyarAl().Degisim, Masraf: k.masraf()})
	if n := len(k.Akis.Kayitlar); n > akisKayitSiniri {
		// en eskiler atılır (sonucu belli olanlar karne dosyasında da var)
		k.Akis.Kayitlar = append([]onayKaydi(nil), k.Akis.Kayitlar[n-akisKayitSiniri:]...)
	}
	akisHesapBaslat(k, r.Mum)
	return true
}

// akisOnayKancasi: durumMu kilitliyken, kararIsle "al" diyecekken. Onay uygulanıyorsa alımı bekletir (eylem yok).
func akisOnayKancasi(k *kumbara, r *kararIstegi, e eylem) eylem {
	if x := k.onayKaydiBul(r.Mum); x != nil {
		if x.Uygulandi {
			return eylem{} // aynı sinyal ikinci kez geldi: kararı akış verecek
		}
		return e
	}
	uygula := k.akisUygulaniyor()
	golge := ""
	if !uygula {
		golge = "onay kapalı"
	}
	akisKayitAc(k, r, uygula, golge)
	if uygula {
		return eylem{}
	}
	return e
}

// akisGolge: kumbara korumada kenardayken gelen taze alım sinyali için sadece gölge kaydı (durumMu kilitli).
func akisGolge(k *kumbara, r *kararIstegi) {
	kapanis := r.Mum + k.ms() - borsaFarki.Load()
	if simdiF().UnixMilli()-kapanis > tazelikSiniri(k.ms()) {
		return
	}
	akisKayitAc(k, r, false, "kenarda")
}

func bloklariAl(s akisSonuc) (out [4]*float64) {
	for i := range out {
		if i < len(s.Bloklar) && s.Bloklar[i].Var {
			p := s.Bloklar[i].Puan
			out[i] = &p
		}
	}
	return out
}

func kayitDoldur(x *onayKaydi, s akisSonuc) {
	x.Toplam, x.Esik, x.Ozet, x.Baglam = s.Toplam, s.Esik, s.Ozet, s.Baglam
	x.Bloklar = bloklariAl(s)
	x.Engel = s.Eksik
	if x.Engel == "" {
		x.Engel = s.Yasak
	}
}

// motor hâlâ bu alış işleminde mi?
func motorIslemde(k *kumbara, motorMum int64) bool {
	return k.MotorAsama == "coinde" && (k.MotorGirisMum == 0 || k.MotorGirisMum == motorMum)
}

// alım hâlâ yapılabilir mi? (değilse sebebi)
func akisAlinabilirMi(k *kumbara, motorMum int64) string {
	switch {
	case k.Durum != "calisiyor":
		return "kumbara yeni alım yapmıyor"
	case k.acikIslem() != nil || k.Emirde || k.Bekleyen != nil:
		return "kumbara başka bir işlemde"
	case k.Koruma != nil && k.Koruma.Kenarda:
		return "kumbara korumada kenarda"
	case !motorIslemde(k, motorMum):
		return "motor bu işlemden çıktı"
	}
	return ""
}

// akisMumIcinDegerlendir: motorun mumu (kapanmış) hesaba girene kadar birkaç kez dener (mum önbelleği bir an geride olabilir).
func akisMumIcinDegerlendir(sembol, aralik string, mum int64) (*akisHesap, error) {
	var h *akisHesap
	var err error
	for deneme := 0; deneme < 8; deneme++ {
		if deneme > 0 {
			akisUyku(time.Second)
		}
		h, err = akisDegerlendirF(sembol, aralik)
		if err == nil && (h.sonuc.Mum >= mum || h.sonuc.Mum == 0) {
			return h, nil
		}
	}
	if err == nil {
		err = fmt.Errorf("motorun mumu MEXC verisinde henüz yok")
	}
	return nil, err
}

// akisOnayHesapla: ilk değerlendirme (arka planda).
func akisOnayHesapla(k *kumbara, motorMum int64) {
	durumMu.Lock()
	sembol, aralik := k.Sembol, k.Aralik
	durumMu.Unlock()
	h, err := akisMumIcinDegerlendir(sembol, aralik, motorMum)
	durumMu.Lock()
	x := k.onayKaydiBul(motorMum)
	if x == nil || x.Karar != "hesaplaniyor" {
		durumMu.Unlock()
		return
	}
	var al *eylem
	switch {
	case err != nil:
		x.Ozet = "Akış hesabı yapılamadı: " + kisalt(err.Error(), 160)
		x.Karar = "hata"
		if !x.Uygulandi {
			x.Karar = "iptal" // gölgede hesap yoksa karneye girmez
		}
	case !x.Uygulandi:
		kayitDoldur(x, h.sonuc)
		x.Karar = map[bool]string{true: "golge-onay", false: "golge-red"}[h.sonuc.Onay]
	default:
		kayitDoldur(x, h.sonuc)
		if neden := akisAlinabilirMi(k, motorMum); neden != "" {
			x.Karar, x.Not = "iptal", "Alınmadı: "+neden
		} else if h.sonuc.Onay && akisFiyatUygun(x.Fiyat, h.sonuc.Fiyat) {
			x.Karar = "onay"
			olayEkle(k, "bilgi", "Akış onay verdi, alınıyor. "+h.sonuc.Ozet)
			al = &eylem{tur: "al", beklenen: x.Fiyat, motorMum: motorMum}
		} else {
			x.Karar = "red"
			if h.sonuc.Onay {
				x.Not = "Puan yetiyordu ama fiyat motorun fiyatından çok yukarıdaydı"
			}
		}
	}
	if x.Uygulandi && (x.Karar == "red" || x.Karar == "hata") {
		if b := akisAyarAl().BeklemeMum; b > 0 && akisAlinabilirMi(k, motorMum) == "" {
			kapanis := motorMum + k.ms() - borsaFarki.Load()
			k.Akis.Bekleme = &onayBekleme{MotorMum: motorMum, Fiyat: x.Fiyat, Bitis: kapanis + int64(b)*k.ms()}
			x.Karar = "bekliyor"
			olayEkle(k, "bilgi", fmt.Sprintf("Simülatör aldı ama akış onay vermedi; %d mum boyunca onay bekleniyor. %s", b, x.Ozet))
		} else {
			olayEkle(k, "bilgi", "Simülatör aldı ama akış onay vermedi; alım yapılmadı. "+x.Ozet)
		}
	}
	degisti()
	durumMu.Unlock()
	if al != nil {
		eylemiYap(k, *al)
	}
}

// fiyat motorun alış fiyatından Kovalama'dan fazla yukarıda değilse uygun
func akisFiyatUygun(motorFiyat, fiyat float64) bool {
	if fiyat <= 0 || motorFiyat <= 0 {
		return true
	}
	return fiyat <= motorFiyat*(1+akisAyarAl().Kovalama/100)
}

// durumMu kilitliyken: kumbara kapanırken (ya da onay kapatılınca) bekleyen onay biter
func akisKumbaraBitti(k *kumbara, neden string) {
	if k.Akis == nil || k.Akis.Bekleme == nil {
		return
	}
	if x := k.onayKaydiBul(k.Akis.Bekleme.MotorMum); x != nil && x.Karar == "bekliyor" {
		x.Karar, x.Not = "iptal", neden
	}
	k.Akis.Bekleme = nil
}

func akisBeklemeDongusu() {
	for {
		akisUyku(2 * time.Second)
		akisBeklemeTuru()
	}
}

var akisBeklemeSon sync.Map // kumbara kimliği → son hesap (ms)

// akisBeklemeTuru: bekleyen onayları yeniden hesaplar (kendi döngüsünde; izleyiciyi bekletmez).
func akisBeklemeTuru() {
	type aday struct {
		k              *kumbara
		motorMum       int64
		sembol, aralik string
	}
	var adaylar []aday
	simdi := simdiF().UnixMilli()
	durumMu.Lock()
	for _, k := range D.Kumbaralar {
		if !k.aktif() || k.Akis == nil || k.Akis.Bekleme == nil {
			continue
		}
		b := k.Akis.Bekleme
		x := k.onayKaydiBul(b.MotorMum)
		sonlandir := func(karar, neden string) {
			if x != nil && x.Karar == "bekliyor" {
				x.Karar, x.Not = karar, neden
			}
			k.Akis.Bekleme = nil
			olayEkle(k, "bilgi", "Akış onayı beklemesi bitti, alınmadı: "+neden)
			degisti()
		}
		switch {
		case !k.akisUygulaniyor():
			sonlandir("iptal", "onay kapatıldı")
		case !motorIslemde(k, b.MotorMum):
			sonlandir("motor-cikti", "simülatörün motoru işlemden çıktı")
		case k.Stop > 0 && k.SonFiyat > 0 && k.SonFiyat <= k.Stop:
			sonlandir("zarar-durdur", "fiyat zarar durdur seviyesine indi")
		case k.Hedef > 0 && k.SonFiyat >= k.Hedef:
			sonlandir("fiyat-kacti", "fiyat hedef seviyesine çıktı")
		case simdi >= b.Bitis:
			sonlandir("sure-doldu", "bekleme süresi doldu")
		case akisAlinabilirMi(k, b.MotorMum) != "":
			sonlandir("iptal", akisAlinabilirMi(k, b.MotorMum))
		default:
			adaylar = append(adaylar, aday{k, b.MotorMum, k.Sembol, k.Aralik})
		}
	}
	durumMu.Unlock()
	for _, a := range adaylar {
		if v, ok := akisBeklemeSon.Load(a.k.ID); ok && simdi-v.(int64) < 1900 {
			continue
		}
		akisBeklemeSon.Store(a.k.ID, simdi)
		h, err := akisDegerlendirF(a.sembol, a.aralik)
		if err != nil {
			continue
		}
		durumMu.Lock()
		k := a.k
		b := k.Akis.Bekleme
		if b == nil || b.MotorMum != a.motorMum || akisAlinabilirMi(k, a.motorMum) != "" || !k.akisUygulaniyor() {
			durumMu.Unlock()
			continue
		}
		x := k.onayKaydiBul(a.motorMum)
		if x != nil {
			x.SonBakis, x.SonToplam = simdiF().UnixMilli(), h.sonuc.Toplam
		}
		if !h.sonuc.Onay || !akisFiyatUygun(b.Fiyat, h.sonuc.Fiyat) {
			durumSurum.Add(1) // ekrandaki "son bakış" yenilensin (diske yazmaya gerek yok)
			durumMu.Unlock()
			continue
		}
		if x != nil {
			x.Karar, x.OnayZ, x.OnayFiyat, x.OnayOzet = "bekleyip-onay", simdiF().UnixMilli(), h.sonuc.Fiyat, h.sonuc.Ozet
		}
		k.Akis.Bekleme = nil
		olayEkle(k, "bilgi", "Akış onayı geldi (bekledikten sonra), alınıyor. "+h.sonuc.Ozet)
		degisti()
		durumMu.Unlock()
		eylemiYap(k, eylem{tur: "al", beklenen: b.Fiyat, motorMum: a.motorMum})
	}
}

// akisSonucYaz: durumMu kilitliyken (kararIsle'den): motorun kapanmış işlemleriyle kayıtların sonucunu doldurur.
func akisSonucYaz(k *kumbara, r *kararIstegi) {
	if k.Akis == nil {
		return
	}
	for i := range k.Akis.Kayitlar {
		x := &k.Akis.Kayitlar[i]
		if x.Sonuc != nil || x.Karar == "hesaplaniyor" || x.Karar == "bekliyor" {
			continue
		}
		mi := r.motorIslemi(x.MotorMum)
		if mi == nil || mi.C == nil || mi.CF == nil || *mi.CF <= 0 || x.Fiyat <= 0 {
			continue
		}
		sonuc := func(giris float64) *float64 {
			s := math.Round(((*mi.CF/giris-1)*100-2*x.Masraf)*1000) / 1000
			return &s
		}
		x.MotorSonuc = sonuc(x.Fiyat)
		x.Sonuc = x.MotorSonuc
		if x.Karar == "bekleyip-onay" && x.OnayFiyat > 0 {
			x.Sonuc = sonuc(x.OnayFiyat)
		}
		x.Cikis, x.CikisMum = *mi.CF, *mi.C
		if e, h := onayliMi(x.Karar); e || h {
			akisKarneEkle(karneKaydi{Kumbara: k.ID, Dosya: k.Dosya, Sembol: k.Sembol, Aralik: k.Aralik, Mod: k.Mod, Kayit: *x})
		}
	}
}

// ---------- onay karnesi ----------

type karneKaydi struct {
	Kumbara string    `json:"kumbara"`
	Dosya   string    `json:"dosya"`
	Sembol  string    `json:"sembol"`
	Aralik  string    `json:"aralik"`
	Mod     string    `json:"mod"`
	Kayit   onayKaydi `json:"kayit"`
}

var (
	akisKarneMu       sync.Mutex
	akisKarneListe    []karneKaydi
	akisKarneAnahtar  map[string]bool
	akisKarneYuklendi bool
)

const akisKarneSiniri = 5000

func akisKarneYolu() string { return filepath.Join(kayitKlasoru(), "akis_karne.jsonl") }

func karneAnahtari(x karneKaydi) string {
	return x.Kumbara + "|" + strconv.FormatInt(x.Kayit.MotorMum, 10)
}

// akisKarneKilitliYukle: akisKarneMu kilitliyken; dosyayı bir kez okur
func akisKarneKilitliYukle() {
	if akisKarneYuklendi {
		return
	}
	akisKarneYuklendi = true
	akisKarneAnahtar = map[string]bool{}
	f, err := os.Open(akisKarneYolu())
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		var x karneKaydi
		if json.Unmarshal(sc.Bytes(), &x) != nil || x.Kayit.Sonuc == nil {
			continue
		}
		a := karneAnahtari(x)
		if akisKarneAnahtar[a] {
			continue
		}
		akisKarneAnahtar[a] = true
		akisKarneListe = append(akisKarneListe, x)
	}
	if n := len(akisKarneListe); n > akisKarneSiniri {
		akisKarneListe = append([]karneKaydi(nil), akisKarneListe[n-akisKarneSiniri:]...)
	}
}

func akisKarneEkle(x karneKaydi) {
	akisKarneMu.Lock()
	defer akisKarneMu.Unlock()
	akisKarneKilitliYukle()
	a := karneAnahtari(x)
	if akisKarneAnahtar[a] {
		return
	}
	akisKarneAnahtar[a] = true
	akisKarneListe = append(akisKarneListe, x)
	if n := len(akisKarneListe); n > akisKarneSiniri+500 {
		akisKarneListe = append([]karneKaydi(nil), akisKarneListe[n-akisKarneSiniri:]...)
	}
	if durumHatasi != "" {
		return
	}
	if err := os.MkdirAll(kayitKlasoru(), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(akisKarneYolu(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Println("Akış karnesi yazılamadı:", err)
		return
	}
	b, _ := json.Marshal(x)
	f.Write(append(b, '\n'))
	f.Close()
}

type karneSatiri struct {
	N       int     `json:"n"`
	Kazanan int     `json:"kazanan"`
	Ort     float64 `json:"ort"` // ortalama sonuç (%)
	Top     float64 `json:"top"` // toplam (%)
}

func (s *karneSatiri) ekle(x float64) {
	s.N++
	if x > 0 {
		s.Kazanan++
	}
	s.Top += x
	s.Ort = s.Top / float64(s.N)
}

// onaylı mı reddedilmiş mi (ikisi de değilse karneye girmez)
func onayliMi(karar string) (evet, hayir bool) {
	switch karar {
	case "onay", "bekleyip-onay", "golge-onay":
		return true, false
	case "red", "sure-doldu", "motor-cikti", "zarar-durdur", "fiyat-kacti", "golge-red":
		return false, true
	}
	return false, false
}

type esikSatiri struct {
	Esik float64 `json:"esik"`
	karneSatiri
}

type akisKarne struct {
	Hepsi      karneSatiri       `json:"hepsi"`      // simülatör tek başına (bütün sinyaller)
	Onaylanan  karneSatiri       `json:"onaylanan"`  // süzgeçle
	Reddedilen karneSatiri       `json:"reddedilen"` // süzgecin kaçındığı
	Bekleyen   int               `json:"bekleyen"`   // sonucu henüz belli olmayan
	Esikler    []esikSatiri      `json:"esikler"`    // sinyal anındaki puana göre: eşik bu olsaydı
	Bloklar    [4][2]karneSatiri `json:"bloklar"`    // her blok: puan ≥ +10 iken / ≤ −10 iken simülatörün sonucu
	Hukum      string            `json:"hukum"`
	AyarZ      int64             `json:"ayarZ"`
	BuAyarla   bool              `json:"buAyarla"`
	Kayitlar   []karneKaydi      `json:"kayitlar"`
}

func akisKarneHesapla(kumbaraID string, buAyarla bool, enFazla int) akisKarne {
	kr := akisKarne{AyarZ: akisAyarAl().Degisim, BuAyarla: buAyarla, Kayitlar: []karneKaydi{}}
	birlesik := map[string]karneKaydi{}
	akisKarneMu.Lock()
	akisKarneKilitliYukle()
	for _, x := range akisKarneListe {
		birlesik[karneAnahtari(x)] = x
	}
	akisKarneMu.Unlock()
	durumMu.Lock()
	for _, k := range D.Kumbaralar {
		if k.Akis == nil {
			continue
		}
		for _, x := range k.Akis.Kayitlar {
			y := karneKaydi{Kumbara: k.ID, Dosya: k.Dosya, Sembol: k.Sembol, Aralik: k.Aralik, Mod: k.Mod, Kayit: x}
			birlesik[karneAnahtari(y)] = y // bellektekiler daha yeni
		}
	}
	durumMu.Unlock()
	hepsi := make([]karneKaydi, 0, len(birlesik))
	for _, x := range birlesik {
		if kumbaraID != "" && x.Kumbara != kumbaraID {
			continue
		}
		if buAyarla && x.Kayit.AyarZ != kr.AyarZ {
			continue
		}
		hepsi = append(hepsi, x)
	}
	sort.Slice(hepsi, func(i, j int) bool { return hepsi[i].Kayit.Z > hepsi[j].Kayit.Z })
	esikler := []float64{-40, -30, -20, -10, 0, 10, 20, 30, 40, 50, 60}
	kr.Esikler = make([]esikSatiri, len(esikler))
	for i, e := range esikler {
		kr.Esikler[i].Esik = e
	}
	for _, s := range hepsi {
		x := s.Kayit
		evet, hayir := onayliMi(x.Karar)
		if !evet && !hayir {
			if x.Karar == "bekliyor" || x.Karar == "hesaplaniyor" {
				kr.Bekleyen++
			}
			continue
		}
		if x.Sonuc == nil || x.MotorSonuc == nil {
			kr.Bekleyen++
			continue
		}
		kr.Hepsi.ekle(*x.MotorSonuc)
		if evet {
			kr.Onaylanan.ekle(*x.Sonuc)
		} else {
			kr.Reddedilen.ekle(*x.Sonuc)
		}
		if x.Engel == "" {
			for i := range kr.Esikler {
				if x.Toplam >= kr.Esikler[i].Esik {
					kr.Esikler[i].ekle(*x.MotorSonuc)
				}
			}
		}
		for b := range x.Bloklar {
			if p := x.Bloklar[b]; p != nil {
				if *p >= 10 {
					kr.Bloklar[b][0].ekle(*x.MotorSonuc)
				} else if *p <= -10 {
					kr.Bloklar[b][1].ekle(*x.MotorSonuc)
				}
			}
		}
	}
	for i := 0; i < len(hepsi) && i < enFazla; i++ {
		kr.Kayitlar = append(kr.Kayitlar, hepsi[i])
	}
	o, r := kr.Onaylanan, kr.Reddedilen
	oran := func(s karneSatiri) float64 {
		if s.N == 0 {
			return 0
		}
		return float64(s.Kazanan) / float64(s.N) * 100
	}
	switch {
	case o.N+r.N == 0:
		kr.Hukum = "Henüz sonucu belli olan sinyal yok. Simülatörün motoru bir alım yapıp o işlemi kapatınca burada görünür."
	case o.N < 20 || r.N < 20:
		kr.Hukum = fmt.Sprintf("Henüz az örnek (onaylanan %d, reddedilen %d). Şans payı büyük: karar vermek için iki tarafta da en az 20, tercihen 50 sinyal birikmeli.", o.N, r.N)
	case o.Ort > r.Ort && oran(o) >= oran(r):
		kr.Hukum = fmt.Sprintf("Süzgeç şimdilik işe yarıyor görünüyor: onaylananlar işlem başına ortalama %s puan daha iyi (kazanma %%%s ↔ %%%s).",
			trSayi(o.Ort-r.Ort, 2), trSayi(oran(o), 0), trSayi(oran(r), 0))
	case o.Ort > r.Ort:
		kr.Hukum = "Karışık: onaylananların ortalaması daha iyi ama kazanma oranı değil. Daha çok örnek birikmesini bekle."
	default:
		kr.Hukum = "Süzgeç şimdilik işe yaramıyor: onaylananlar reddedilenlerden iyi değil. Eşik ya da ağırlıkları değiştirmeyi dene."
	}
	return kr
}

// kumbara özeti için kısa hâli (durumMu kilitliyken)
func akisOzeti(k *kumbara) map[string]any {
	m := map[string]any{"uygulaniyor": k.akisUygulaniyor(), "genelAcik": akisAyarAl().Acik, "kapali": false}
	if k.Akis == nil {
		return m
	}
	m["kapali"] = k.Akis.Kapali
	if k.Akis.Bekleme != nil {
		m["bekleme"] = k.Akis.Bekleme
	}
	n := len(k.Akis.Kayitlar)
	m["kayitlar"] = k.Akis.Kayitlar[max(0, n-10):]
	var o, r karneSatiri
	for _, x := range k.Akis.Kayitlar {
		if x.Sonuc == nil {
			continue
		}
		if e, h := onayliMi(x.Karar); e {
			o.ekle(*x.Sonuc)
		} else if h {
			r.ekle(*x.Sonuc)
		}
	}
	m["onaylanan"], m["reddedilen"], m["toplamKayit"] = o, r, n
	return m
}

// ---------- uç noktalar ----------

func akisSorgu(w http.ResponseWriter, r *http.Request) (sembol, aralik string, ok bool) {
	q := r.URL.Query()
	sembol = strings.ToUpper(strings.TrimSpace(q.Get("sembol")))
	aralik = q.Get("aralik")
	if !sembolKalip.MatchString(sembol) {
		hataYaz(w, 400, "Coin adı geçersiz")
		return "", "", false
	}
	if _, var_ := araliklar[aralik]; !var_ {
		hataYaz(w, 400, "Mum süresi geçersiz")
		return "", "", false
	}
	return sembol, aralik, true
}

// GET /api/akis/grafik?sembol=&aralik=&kumbara= → grafikte çizilecek her şey + şu anki değerlendirme
func akisGrafikIstegi(w http.ResponseWriter, r *http.Request) {
	sembol, aralik, ok := akisSorgu(w, r)
	if !ok {
		return
	}
	akisIste(sembol)
	h, err := akisDegerlendirF(sembol, aralik)
	if err != nil {
		hataYaz(w, 502, "Akış hesabı yapılamadı: "+err.Error())
		return
	}
	g := akisGrafikHazirla(h, akisAyarAl())
	if id := r.URL.Query().Get("kumbara"); id != "" {
		durumMu.Lock()
		if k := kumbaraBul(id); k != nil && k.Akis != nil {
			g.Kararlar = append([]onayKaydi(nil), k.Akis.Kayitlar...)
		}
		durumMu.Unlock()
	}
	jsonYaz(w, 200, g)
}

// GET /api/akis/canli?sembol=&aralik= → şu anki değerlendirme (kart için, sık)
func akisCanliIstegi(w http.ResponseWriter, r *http.Request) {
	sembol, aralik, ok := akisSorgu(w, r)
	if !ok {
		return
	}
	akisIste(sembol)
	h, err := akisDegerlendirF(sembol, aralik)
	if err != nil {
		hataYaz(w, 502, "Akış hesabı yapılamadı: "+err.Error())
		return
	}
	jsonYaz(w, 200, h.sonuc)
}

// ayarlarda Acik dışında bir şey değişti mi?
func akisAyarFarkli(a, b akisAyar) bool {
	a.Acik, b.Acik, a.Degisim, b.Degisim = false, false, 0, 0
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) != string(y)
}

// GET/POST /api/akis/ayar
func akisAyarIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var q struct {
			Oturum     string   `json:"oturum"`
			Ayar       akisAyar `json:"ayar"`
			Varsayilan bool     `json:"varsayilan"`
		}
		if jsonOku(r, &q) != nil {
			hataYaz(w, 400, "İstek okunamadı")
			return
		}
		if !oturumGecerli(q.Oturum) {
			hataYaz(w, 409, "Bu pencere eskidi; programın son açılan penceresini kullan")
			return
		}
		eski := akisAyarAl()
		a := q.Ayar
		if q.Varsayilan {
			a = akisVarsayilan()
			a.Acik = eski.Acik
		}
		a = akisDuzelt(a)
		a.Degisim = eski.Degisim
		if akisAyarFarkli(a, eski) {
			a.Degisim = simdiF().UnixMilli()
		}
		if err := akisAyarYaz(a); err != nil {
			hataYaz(w, 500, "Ayar kaydedilemedi: "+err.Error())
			return
		}
		yeni := akisAyarAl()
		if eski.Acik != yeni.Acik || eski.Degisim != yeni.Degisim {
			mesaj := "Akış onayının ayarları değişti"
			if eski.Acik != yeni.Acik {
				mesaj = map[bool]string{true: "Akış onayı açıldı: kumbaraların alımları onay bekleyecek", false: "Akış onayı kapatıldı: alımlar simülatör gibi hemen yapılacak (sistemin kararı gölge olarak kaydedilir)"}[yeni.Acik]
			}
			durumMu.Lock()
			genelOlay("bilgi", mesaj)
			degisti()
			durumMu.Unlock()
		}
	}
	jsonYaz(w, 200, map[string]any{"ayar": akisAyarAl(), "varsayilan": akisVarsayilan()})
}

// POST /api/kumbara/akis {oturum, id, kapali}: bu kumbarada onayı aç/kapat
func kumbaraAkisIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string `json:"oturum"`
		ID     string `json:"id"`
		Kapali bool   `json:"kapali"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil {
		hataYaz(w, 400, "İstek okunamadı")
		return
	}
	if !oturumGecerli(q.Oturum) {
		hataYaz(w, 409, "Bu pencere eskidi; programın son açılan penceresini kullan")
		return
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	k := kumbaraBul(q.ID)
	if k == nil || !k.aktif() {
		hataYaz(w, 404, "Kumbara bulunamadı")
		return
	}
	if k.Akis == nil {
		k.Akis = &akisKumbara{}
	}
	if k.Akis.Kapali != q.Kapali {
		k.Akis.Kapali = q.Kapali
		if q.Kapali {
			olayEkle(k, "bilgi", "Akış onayı bu kumbarada kapatıldı: simülatörün alımları eskisi gibi hemen yapılacak (sistemin kararı gölge olarak kaydedilir)")
			akisKumbaraBitti(k, "Onay bu kumbarada kapatıldı")
		} else {
			olayEkle(k, "bilgi", "Akış onayı bu kumbarada açıldı: alımlar onay bekleyecek")
		}
		degisti()
	}
	jsonYaz(w, 200, map[string]any{"tamam": true})
}

// GET /api/akis/karne?kumbara=&buAyarla=1
func akisKarneIstegi(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	jsonYaz(w, 200, akisKarneHesapla(q.Get("kumbara"), q.Get("buAyarla") == "1", 300))
}
