package main

// KUMBARALAR — her strateji dosyası için ayrı çalışan alım-satım kasası.
//
// Kararları sayfadaki simülatör motoru verir ve her mum kapanışında buraya bildirir (/api/kumbara/karar).
// Burada gerçek durum (kumbaranın elinde coin var mı) motorun durumuyla eşleştirilir:
//   - motor bu mumda ALDI ve kumbarada coin yok → al (limit → 15 sn → piyasa). Mum, kumbara açıldıktan sonra
//     kapanmış ve karar taze olmalı (program kapalıyken kaçan alımın peşinden koşulmaz).
//   - motor artık coinde değil (ya da başka bir işlemde) ama kumbarada coin var → sat (kaçan satış dahil)
//   - motor daha önce almış ama kumbara almamış → bekle
// Ayrıca 2 saniyede bir fiyat izlenir: zarar durdur seviyesine değerse beklemeden piyasa emriyle satılır,
// hedef kâr seviyesine değerse normal limit kuralıyla satılır. Ömür dolunca yeni alım yapılmaz; elde coin varsa
// satış kuralı beklenir, en geç ömrün dörtte biri kadar sonra satılır.
//
// Sanal kumbaralar programın içindeki sanal cüzdandan, gerçek kumbaralar MEXC hesabındaki USDT'den para ayırır.
// Her kumbara sadece kendi parasını ve kendi aldığı coini kullanır. Durum kayitlar/durum.json'da saklanır.

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type kumbaraIslemi struct {
	No             int           `json:"no"`
	MotorMum       int64         `json:"motorMum"`       // motorun alış mumunun açılış zamanı
	MotorCikisMum  int64         `json:"motorCikisMum"`  // motorun satış mumu (biliniyorsa)
	BeklenenAlis   float64       `json:"beklenenAlis"`   // simülatörün alış fiyatı (mum kapanışı)
	BeklenenSatis  float64       `json:"beklenenSatis"`  // simülatörün satış fiyatı; 0 = bilinmiyor
	BeklenenMasraf float64       `json:"beklenenMasraf"` // simülatörün her yön için varsaydığı masraf (%)
	BeklenenYuzde  float64       `json:"beklenenYuzde"`
	BeklenenVar    bool          `json:"beklenenVar"`
	MotorSebep     string        `json:"motorSebep,omitempty"`
	Alis           *dolumSonucu  `json:"alis"`
	Satis          *dolumSonucu  `json:"satis,omitempty"`
	KismiSatis     []dolumSonucu `json:"kismiSatis,omitempty"` // 0.8: işlem kapanmadan önce yapılan kısmi satışlar
	SatisSebebi    string        `json:"satisSebebi,omitempty"`
	SatisAciklama  string        `json:"satisAciklama,omitempty"`
	Harcanan       float64       `json:"harcanan"`
	Gelen          float64       `json:"gelen"`
	Net            float64       `json:"net"`
	Yuzde          float64       `json:"yuzde"`
}

type kumbaraOlayi struct {
	Zaman  int64  `json:"zaman"`
	Tur    string `json:"tur"` // bilgi | al | sat | uyari | hata
	Mesaj  string `json:"mesaj"`
	Tekrar int    `json:"tekrar,omitempty"` // aynı olay art arda kaç kez daha oldu
}

type bekleyenEmir struct {
	Taraf     string  `json:"taraf"`
	Onek      string  `json:"onek"` // istemci emir kimliği öneki (L: limit, P: piyasa eklenir)
	Baslangic int64   `json:"baslangic"`
	MotorMum  int64   `json:"motorMum"`
	Beklenen  float64 `json:"beklenen"`
	Sebep     string  `json:"sebep"`
}

type devamPlani struct {
	Dosya       string          `json:"dosya"`
	Icerik      json.RawMessage `json:"icerik"`
	Olusturuldu int64           `json:"olusturuldu"`
	Bitis       int64           `json:"bitis"`
}

type kumbara struct {
	ID             string          `json:"id"`
	Dosya          string          `json:"dosya"`
	Sembol         string          `json:"sembol"`
	Aralik         string          `json:"aralik"`
	Mod            string          `json:"mod"` // sanal | gercek
	Icerik         json.RawMessage `json:"icerik"`
	Olusturuldu    int64           `json:"olusturuldu"`
	Bitis          int64           `json:"bitis"`
	EkBitis        int64           `json:"ekBitis"`
	Baslangic      int64           `json:"baslangic"`
	IlkMum         int64           `json:"ilkMum"`
	MumSayisi      int             `json:"mumSayisi"`
	BaslangicTutar float64         `json:"baslangicTutar"`
	Nakit          float64         `json:"nakit"`
	Adet           float64         `json:"adet"`
	Durum          string          `json:"durum"` // calisiyor | sureDoldu | durduruluyor | bitti
	BitisSebebi    string          `json:"bitisSebebi,omitempty"`
	BitisZamani    int64           `json:"bitisZamani,omitempty"`
	Iade           float64         `json:"iade,omitempty"`
	Islemler       []kumbaraIslemi `json:"islemler"`
	Olaylar        []kumbaraOlayi  `json:"olaylar"`
	SonKararMum    int64           `json:"sonKararMum"`
	SonKararZamani int64           `json:"sonKararZamani"`
	MotorAsama     string          `json:"motorAsama"`
	Stop           float64         `json:"stop"`
	Hedef          float64         `json:"hedef"`
	MotorGirisMum  int64           `json:"motorGirisMum"`
	Emirde         bool            `json:"emirde"`
	EmirAciklama   string          `json:"emirAciklama,omitempty"`
	Bekleyen       *bekleyenEmir   `json:"bekleyen,omitempty"`
	SatisTekrar    int64           `json:"satisTekrar,omitempty"`
	TekrarSebep    string          `json:"tekrarSebep,omitempty"`
	TekrarSayisi   int             `json:"tekrarSayisi,omitempty"`
	HemenSat       bool            `json:"hemenSat,omitempty"`
	Devam          *devamPlani     `json:"devam,omitempty"`
	OncekiID       string          `json:"oncekiId,omitempty"` // yeni ayarla devam edilen kumbara
	Sayac          int             `json:"sayac"`
	SonFiyat       float64         `json:"sonFiyat"`
	SonFiyatZamani int64           `json:"sonFiyatZamani"`
	Komisyon       float64         `json:"komisyon"`         // toplam ödenen (USDT karşılığı)
	Koruma         *koruma         `json:"koruma,omitempty"` // 0.7: kenara alma ve gölge takip (koruma.go)
	Akis           *akisKumbara    `json:"akis,omitempty"`   // 0.9: akış onayı kayıtları (akis_onay.go)
}

type durumDosyasi struct {
	Surum          int            `json:"surum"`
	SanalSerbest   float64        `json:"sanalSerbest"`
	SanalBaslangic float64        `json:"sanalBaslangic"`
	Kumbaralar     []*kumbara     `json:"kumbaralar"`
	GenelOlaylar   []kumbaraOlayi `json:"genelOlaylar"`
}

var (
	durumMu    sync.Mutex
	D          = yeniDurum()
	durumSurum atomic.Int64
	piyasa     piyasaKaynagi = yeniMexcPiyasa()
	sanal                    = yeniSanalBorsa(piyasa)
)

func yeniDurum() *durumDosyasi {
	return &durumDosyasi{Surum: 1, SanalSerbest: 1000, SanalBaslangic: 1000, Kumbaralar: []*kumbara{}, GenelOlaylar: []kumbaraOlayi{}}
}

func kayitKlasoru() string { return filepath.Join(veriKlasoru, "kayitlar") }

// ---------- kumbara yardımcıları ----------

func (k *kumbara) acikIslem() *kumbaraIslemi {
	if n := len(k.Islemler); n > 0 && k.Islemler[n-1].Satis == nil {
		return &k.Islemler[n-1]
	}
	return nil
}

func (k *kumbara) deger() float64 { return k.Nakit + k.Adet*k.SonFiyat }

// Simülatörün işlem başına varsaydığı masraf (%), dosyadaki stratejiden
func (k *kumbara) masraf() float64 {
	var ic struct {
		Strateji struct {
			Para struct {
				Masraf *float64 `json:"masraf"`
			} `json:"para"`
		} `json:"strateji"`
	}
	if json.Unmarshal(k.Icerik, &ic) == nil && ic.Strateji.Para.Masraf != nil && *ic.Strateji.Para.Masraf >= 0 {
		return *ic.Strateji.Para.Masraf
	}
	return 0.1
}

func (k *kumbara) aktif() bool { return k.Durum != "bitti" }

func (k *kumbara) ms() int64 {
	if a, ok := araliklar[k.Aralik]; ok {
		return a.ms
	}
	return 60_000
}

// Aynı olay art arda tekrarlanırsa (ör. bağlantı yokken her denemede aynı hata) yeni satır açılmaz, sayılır.
func olayEkle(k *kumbara, tur, mesaj string) {
	simdi := simdiF().UnixMilli()
	if n := len(k.Olaylar); n > 0 {
		s := &k.Olaylar[n-1]
		if s.Tur == tur && s.Mesaj == mesaj && simdi-s.Zaman < 15*60*1000 {
			s.Tekrar++
			s.Zaman = simdi
			return
		}
	}
	k.Olaylar = append(k.Olaylar, kumbaraOlayi{Zaman: simdi, Tur: tur, Mesaj: mesaj})
	if len(k.Olaylar) > 300 {
		k.Olaylar = append([]kumbaraOlayi(nil), k.Olaylar[len(k.Olaylar)-300:]...)
	}
	log.Printf("[%s %s %s] %s", k.Sembol, k.Aralik, k.Mod, mesaj)
}

func genelOlay(tur, mesaj string) {
	D.GenelOlaylar = append(D.GenelOlaylar, kumbaraOlayi{Zaman: simdiF().UnixMilli(), Tur: tur, Mesaj: mesaj})
	if len(D.GenelOlaylar) > 60 {
		D.GenelOlaylar = append([]kumbaraOlayi(nil), D.GenelOlaylar[len(D.GenelOlaylar)-60:]...)
	}
	log.Println(mesaj)
}

func kumbaraBul(id string) *kumbara {
	for _, k := range D.Kumbaralar {
		if k.ID == id {
			return k
		}
	}
	return nil
}

func aktifKumbaraVar() bool {
	for _, k := range D.Kumbaralar {
		if k.aktif() {
			return true
		}
	}
	return false
}

// durumMu kilitliyken çağrılır: kaydeder ve ekranı uyarır
func degisti() {
	durumSurum.Add(1)
	durumKaydet()
	uykuyuEngelle(aktifKumbaraVar())
}

// 0.8: durum.json okunamadıysa (bozuk) program onu KORUR: üstüne yazmaz, yeni kumbara açtırmaz, ekranda uyarır.
var durumHatasi string

func durumKaydet() {
	if durumHatasi != "" {
		return // bozuk dosyanın üstüne boş durum yazılmasın (elle kurtarılabilsin)
	}
	if err := os.MkdirAll(kayitKlasoru(), 0755); err != nil {
		log.Println("Kayıt klasörü açılamadı:", err)
		return
	}
	// biten kumbaraların en yeni 40'ı saklanır (işlemleri ayrıca islemler.csv'de)
	var aktifler, bitenler []*kumbara
	for _, k := range D.Kumbaralar {
		if k.aktif() {
			aktifler = append(aktifler, k)
		} else {
			bitenler = append(bitenler, k)
		}
	}
	if len(bitenler) > 40 {
		sort.Slice(bitenler, func(i, j int) bool { return bitenler[i].BitisZamani > bitenler[j].BitisZamani })
		bitenler = bitenler[:40]
		D.Kumbaralar = append(aktifler, bitenler...)
	}
	b, err := json.MarshalIndent(D, "", " ")
	if err != nil {
		log.Println("Durum yazılamadı:", err)
		return
	}
	yol := filepath.Join(kayitKlasoru(), "durum.json")
	// 0.8: önce diske tam yazılır (fsync; Pi'de elektrik kesilince yarım dosya kalmasın), eski dosya durum.json.onceki
	// olarak saklanır, sonra yenisi yerine konur. Açılışta durum.json bozuksa sırayla .yeni ve .onceki denenir.
	if err := diskeYaz(yol+".yeni", b); err != nil {
		log.Println("Durum yazılamadı:", err)
		return
	}
	if _, err := os.Stat(yol); err == nil {
		_ = os.Rename(yol, yol+".onceki")
	}
	// OneDrive ya da antivirüs dosyayı bir an kilitlemiş olabilir: birkaç kez dene
	var err2 error
	for deneme := 0; deneme < 5; deneme++ {
		if err2 = os.Rename(yol+".yeni", yol); err2 == nil {
			return
		}
		time.Sleep(time.Duration(40*(deneme+1)) * time.Millisecond)
	}
	log.Println("Durum yazılamadı:", err2)
}

// Dosyayı yazar ve diske kesin işlenmesini bekler (fsync)
func diskeYaz(yol string, b []byte) error {
	f, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func durumYukle() {
	durumMu.Lock()
	defer durumMu.Unlock()
	durumYukleIc()
}

// durumMu kilitliyken çağrılır (0.8: eski sürümden taşımada da kullanılır)
func durumYukleIc() {
	yol := filepath.Join(kayitKlasoru(), "durum.json")
	// 0.8: durum.json okunamazsa (bozuk ya da yarım) önce son yazılan .yeni, sonra bir önceki kayıt (.onceki) denenir.
	// Hiçbiri okunamazsa program boş durumla AÇILMAZ (0.7'de açılıp bozuk dosyanın üstüne yazıyordu: coin tutan
	// kumbaralar unutuluyordu): kayıt korunur, yeni kumbara açılmaz, ekranda uyarı çıkar.
	var d *durumDosyasi
	var hatalar []string
	vardi := false
	for _, aday := range []string{yol, yol + ".yeni", yol + ".onceki"} {
		b, err := os.ReadFile(aday)
		if err != nil {
			if !os.IsNotExist(err) {
				vardi = true
				hatalar = append(hatalar, filepath.Base(aday)+": "+err.Error())
			}
			continue
		}
		vardi = true
		x := yeniDurum()
		if err := json.Unmarshal(b, x); err != nil {
			hatalar = append(hatalar, filepath.Base(aday)+": "+err.Error())
			_ = os.WriteFile(filepath.Join(kayitKlasoru(), "durum_bozuk_"+time.Now().Format("20060102-150405")+"_"+filepath.Base(aday)), b, 0644)
			continue
		}
		d = x
		if aday != yol {
			log.Printf("durum.json okunamadı (%s); %s yüklendi", strings.Join(hatalar, "; "), filepath.Base(aday))
			defer func(ad string) {
				genelOlay("uyari", "Kayıt dosyası (durum.json) bozuktu; "+ad+" yüklendi. Kumbaraları kontrol et.")
			}(filepath.Base(aday))
		}
		break
	}
	if d == nil {
		if vardi {
			durumHatasi = "Kayıt dosyası (kayitlar/durum.json) okunamadı: " + strings.Join(hatalar, "; ") +
				". Program bu dosyanın üstüne yazmıyor ve yeni kumbara açmıyor. MEXC hesabını kontrol et; dosyayı onarınca programı yeniden aç."
			log.Println(durumHatasi)
		}
		return
	}
	if d.SanalBaslangic <= 0 {
		d.SanalBaslangic = 1000
	}
	if d.Kumbaralar == nil {
		d.Kumbaralar = []*kumbara{}
	}
	D = d
	for _, k := range D.Kumbaralar {
		if k.Islemler == nil {
			k.Islemler = []kumbaraIslemi{}
		}
		olaylariKisalt(k)  // 0.8
		korumaGeciciKur(k) // 0.7: eski sürümden kalan çalışan kumbara korumasız kalmasın (sayfa sonra asıl sınırları kurdurur)
		if kr := k.Koruma; kr != nil && kr.TavanTepe <= 0 {
			kr.TavanTepe = math.Max(kr.Tepe, kr.SonDeger)
		}
		if k.Olaylar == nil {
			k.Olaylar = []kumbaraOlayi{}
		}
		if !k.Emirde && k.Bekleyen == nil {
			continue
		}
		k.Emirde, k.EmirAciklama = false, ""
		if k.Mod == "sanal" {
			if k.Bekleyen != nil {
				olayEkle(k, "uyari", "Program kapanırken bir sanal emir yarım kaldı; yok sayıldı")
			}
			k.Bekleyen = nil
		} else if k.Bekleyen != nil {
			olayEkle(k, "uyari", "Program kapanırken bir gerçek emir yarım kalmıştı; MEXC'den son durumu sorulacak")
		}
	}
	uykuyuEngelle(aktifKumbaraVar())
}

// Kapanan işlemi Excel'de açılabilen dosyaya ekler (Türkçe Excel: ; ayraç, ondalık virgül)
func csvEkle(k *kumbara, t *kumbaraIslemi) {
	yol := filepath.Join(kayitKlasoru(), "islemler.csv")
	_ = os.MkdirAll(kayitKlasoru(), 0755)
	yeni := false
	if _, err := os.Stat(yol); err != nil {
		yeni = true
	}
	f, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if yeni {
		fmt.Fprint(f, "\xef\xbb\xbfalis_zamani;satis_zamani;coin;mum;mod;alis_beklenen;alis_gercek;alis_yolu;satis_beklenen;satis_gercek;satis_yolu;satis_sebebi;komisyon_usdt;harcanan_usdt;gelen_usdt;sonuc_usdt;sonuc_yuzde;beklenen_yuzde\r\n")
	}
	z := func(ms int64) string {
		if ms <= 0 {
			return ""
		}
		return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
	}
	s := func(x float64) string { return trSayi(x, 8) }
	kom := 0.0
	alisZ, alisF, alisY := "", "", ""
	if t.Alis != nil {
		kom += t.Alis.Komisyon
		alisZ, alisF, alisY = z(t.Alis.Bitis), s(t.Alis.Ortalama), t.Alis.Yol
	}
	satisZ, satisF, satisY := "", "", ""
	if t.Satis != nil {
		kom += t.Satis.Komisyon
		satisZ, satisF, satisY = z(t.Satis.Bitis), s(t.Satis.Ortalama), t.Satis.Yol
	}
	by := ""
	if t.BeklenenVar {
		by = trSayi(t.BeklenenYuzde, 4)
	}
	fmt.Fprintf(f, "%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s;%s\r\n",
		alisZ, satisZ, k.Sembol, k.Aralik, k.Mod, s(t.BeklenenAlis), alisF, alisY,
		s(t.BeklenenSatis), satisF, satisY, t.SatisSebebi, s(kom), s(t.Harcanan), s(t.Gelen), s(t.Net), trSayi(t.Yuzde, 4), by)
}

func rastgeleKimlik() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func oturumGecerli(o string) bool {
	a, _ := aktifOturum.Load().(string)
	return o != "" && o == a
}

// ---------- borsa seçimi ----------

func kumbaraBorsasi(k *kumbara) (borsa, error) {
	if k.Mod == "sanal" {
		return sanal, nil
	}
	G.mu.Lock()
	defer G.mu.Unlock()
	if !G.Bagli || G.borsa == nil {
		return nil, fmt.Errorf("MEXC bağlantısı yok")
	}
	return G.borsa, nil
}

func kumbaraKurali(k *kumbara) (sembolKurali, error) {
	kural, err := piyasa.Kurallar(k.Sembol)
	if err != nil {
		return kural, err
	}
	if k.Mod == "gercek" {
		G.mu.Lock()
		if kk, ok := G.komisyonlar[k.Sembol]; ok {
			kural.Maker, kural.Taker = kk[0], kk[1]
		}
		G.mu.Unlock()
	}
	return kural, nil
}

// ---------- metin yardımcıları ----------

func farkMetni(oran float64) string {
	y := oran * 100
	isaret := "+"
	if y < 0 {
		isaret = "−"
		y = -y
	}
	return isaret + "%" + trSayi(y, 2)
}

func yolAdi(yol string) string {
	switch yol {
	case "limit":
		return "limit emir doldu"
	case "limit+piyasa":
		return "limit + piyasa emri"
	case "piyasa":
		return "piyasa emri"
	case "toz":
		return "satılamayacak kadar küçük"
	case "birakildi":
		return "satılmadan bırakıldı"
	}
	return yol
}

var satisSebepAdi = map[string]string{
	"anlikZarar": "zarar durdur (anlık)",
	"anlikHedef": "hedef kâr (anlık)",
	"sureDoldu":  "ömür doldu",
	"elle":       "elle satıldı",
	"motor":      "motor çıktı",
}

// ---------- alış ve satış sonuçlarını kumbaraya işleme (durumMu kilitliyken) ----------

func alisIsle(k *kumbara, sonuc dolumSonucu, beklenen float64, motorMum int64, kural sembolKurali, onek string) {
	k.Nakit -= sonuc.NetTutar
	if k.Nakit < 0 {
		k.Nakit = 0
	}
	k.Adet += sonuc.NetMiktar
	k.Komisyon += sonuc.Komisyon
	if sonuc.Ortalama > 0 {
		k.SonFiyat = sonuc.Ortalama
	}
	t := kumbaraIslemi{No: len(k.Islemler) + 1, MotorMum: motorMum, BeklenenAlis: beklenen, BeklenenMasraf: k.masraf(), Alis: &sonuc, Harcanan: sonuc.NetTutar}
	k.Islemler = append(k.Islemler, t)
	fark := ""
	if beklenen > 0 && sonuc.Ortalama > 0 {
		fark = fmt.Sprintf(". Simülatör %s bekliyordu (fark %s)", fiyatMetni(beklenen, kural), farkMetni(sonuc.Ortalama/beklenen-1))
	}
	olayEkle(k, "al", fmt.Sprintf("%sALDI: %s %s, ortalama %s (%s), harcanan %s USDT, komisyon %s USDT%s", onek,
		trSayi(sonuc.NetMiktar, kural.MiktarOndalik), kural.Varlik, fiyatMetni(sonuc.Ortalama, kural), yolAdi(sonuc.Yol),
		trSayi(sonuc.NetTutar, 2), trSayi(sonuc.Komisyon, 4), fark))
}

// 0.8: işlemin daha önce yapılmış kısmi satışlarının toplam geliri (USDT)
func (t *kumbaraIslemi) kismiGelir() float64 {
	s := 0.0
	for _, x := range t.KismiSatis {
		s += x.NetTutar
	}
	return s
}

// Kısmi satışlar ve son satış tek kayıtta (işlemin toplam satışı)
func satisBirlestir(parcalar []dolumSonucu, kural sembolKurali) dolumSonucu {
	var hepsi []parca
	for _, x := range parcalar {
		hepsi = append(hepsi, x.Parcalar...)
	}
	d := dolumTopla("SELL", kural, hepsi)
	d.Baslangic, d.Bitis = parcalar[0].Baslangic, parcalar[len(parcalar)-1].Bitis
	if hepsi == nil {
		d.Parcalar = []parca{}
	}
	return d
}

// 0.8: Satış emrinin sonucu işlenir (durumMu kilitliyken). Satılamayan kısım MEXC'de satılabilecek büyüklükteyse
// işlem KAPANMAZ: satılan kısım kumbaraya işlenir, kalan için satış hemen (piyasa emriyle) tekrar denenir; bu arada
// zarar durdur ve ömür izlenmeye devam eder. 0.7'de satışın bir kısmı dolup kalanı için piyasa emri reddedilince işlem
// kapanıyor, kalan coin zarar durdursuz kalıyordu.
func satisSonucuIsle(k *kumbara, sonuc dolumSonucu, sebep, aciklama string, beklenen float64, kural sembolKurali, onek string) {
	t := k.acikIslem()
	kalan := k.Adet - sonuc.Miktar
	fiyat := sonuc.Ortalama
	if fiyat <= 0 {
		fiyat = k.SonFiyat
	}
	satilabilir := kalan > 0 && asagiYuvarla(kalan, kural.MiktarOndalik) >= kural.EnAzMiktar && (fiyat <= 0 || kalan*fiyat >= kural.EnAzTutarPiyasa)
	if t == nil || sonuc.Yol == "toz" || sonuc.Miktar <= 0 || !satilabilir {
		satisIsle(k, sonuc, sebep, aciklama, beklenen, kural, onek)
		return
	}
	k.Adet = kalan
	k.Nakit += sonuc.NetTutar
	k.Komisyon += sonuc.Komisyon
	if sonuc.Ortalama > 0 {
		k.SonFiyat = sonuc.Ortalama
	}
	t.KismiSatis = append(t.KismiSatis, sonuc)
	if t.BeklenenSatis == 0 && beklenen > 0 {
		t.BeklenenSatis = beklenen
	}
	k.SatisTekrar = simdiF().Add(3 * time.Second).UnixMilli()
	k.TekrarSebep = sebep
	olayEkle(k, "uyari", fmt.Sprintf("%sSatışın bir kısmı yapıldı: %s %s satıldı (ortalama %s, gelen %s USDT). Kalan %s %s için satış hemen tekrar deneniyor",
		onek, trSayi(sonuc.Miktar, kural.MiktarOndalik), kural.Varlik, fiyatMetni(sonuc.Ortalama, kural), trSayi(sonuc.NetTutar, 2), trSayi(kalan, kural.MiktarOndalik), kural.Varlik))
}

func satisIsle(k *kumbara, sonuc dolumSonucu, sebep, aciklama string, beklenen float64, kural sembolKurali, onek string) {
	t := k.acikIslem()
	k.Adet -= sonuc.Miktar
	if k.Adet < 1e-12 {
		k.Adet = 0
	}
	k.Nakit += sonuc.NetTutar
	k.Komisyon += sonuc.Komisyon
	if sonuc.Ortalama > 0 {
		k.SonFiyat = sonuc.Ortalama
	}
	k.HemenSat = false
	if t == nil {
		return
	}
	if len(t.KismiSatis) > 0 && sonuc.Yol != "toz" {
		b := satisBirlestir(append(append([]dolumSonucu(nil), t.KismiSatis...), sonuc), kural)
		t.Satis = &b
	} else {
		t.Satis = &sonuc
	}
	t.SatisSebebi = sebep
	t.SatisAciklama = aciklama
	t.Gelen = t.kismiGelir() + sonuc.NetTutar
	if t.BeklenenSatis == 0 && beklenen > 0 {
		t.BeklenenSatis = beklenen
	}
	beklenenHesapla(t)
	t.Net = t.Gelen - t.Harcanan
	if t.Harcanan > 0 {
		t.Yuzde = t.Net / t.Harcanan * 100
	}
	ad := satisSebepAdi[sebep]
	if ad == "" {
		ad = sebep
	}
	if sonuc.Yol == "toz" {
		olayEkle(k, "uyari", fmt.Sprintf("%sKalan %s %s MEXC'nin en az satış miktarının altında; satılamadı, işlem kapandı sayıldı (coin kumbarada duruyor, sonraki satışa eklenir)", onek, trSayi(k.Adet, kural.MiktarOndalik), kural.Varlik))
	} else {
		olayEkle(k, "sat", fmt.Sprintf("%sSATTI (%s): %s %s, ortalama %s (%s), gelen %s USDT, komisyon %s USDT. Sonuç %s USDT (%s)", onek, ad,
			trSayi(sonuc.Miktar, kural.MiktarOndalik), kural.Varlik, fiyatMetni(sonuc.Ortalama, kural), yolAdi(sonuc.Yol),
			trSayi(sonuc.NetTutar, 2), trSayi(sonuc.Komisyon, 4), trSayi(t.Net, 2), farkMetni(t.Yuzde/100)))
	}
	csvEkle(k, t)
	if kalan := k.Adet * k.SonFiyat; sonuc.Yol != "toz" && kalan >= 0.5 {
		olayEkle(k, "bilgi", fmt.Sprintf("Satılamayan küçük artık kaldı: %s %s (yaklaşık %s USDT)", trSayi(k.Adet, kural.MiktarOndalik), kural.Varlik, trSayi(kalan, 2)))
	}
}

// Simülatörün bu işlemden beklediği sonuç (her yönde kendi masraf varsayımıyla)
func beklenenHesapla(t *kumbaraIslemi) {
	if t.BeklenenAlis > 0 && t.BeklenenSatis > 0 {
		m := t.BeklenenMasraf / 100
		t.BeklenenYuzde = ((1-m)*(t.BeklenenSatis/t.BeklenenAlis)*(1-m) - 1) * 100
		t.BeklenenVar = true
	}
}

func emirOneki(k *kumbara) string {
	k.Sayac++
	return fmt.Sprintf("as%s%d", k.ID[:6], k.Sayac)
}

// ---------- alış ----------

func kumbaraAl(k *kumbara, beklenen float64, motorMum int64) {
	durumMu.Lock()
	if k.Emirde || k.Bekleyen != nil || k.acikIslem() != nil || k.Durum != "calisiyor" || (k.Koruma != nil && k.Koruma.Kenarda) {
		durumMu.Unlock()
		return
	}
	k.Emirde = true
	k.EmirAciklama = "Alış emri hazırlanıyor"
	onek := emirOneki(k)
	k.Bekleyen = &bekleyenEmir{Taraf: "BUY", Onek: onek, Baslangic: simdiF().UnixMilli(), MotorMum: motorMum, Beklenen: beklenen}
	butce := k.Nakit
	degisti()
	durumMu.Unlock()

	bitir := func(tur, hata string) {
		durumMu.Lock()
		k.Emirde, k.EmirAciklama, k.Bekleyen = false, "", nil
		if hata != "" {
			olayEkle(k, tur, "Alış yapılamadı: "+hata)
		}
		degisti()
		durumMu.Unlock()
	}
	b, err := kumbaraBorsasi(k)
	if err != nil {
		bitir("hata", err.Error())
		return
	}
	kural, err := kumbaraKurali(k)
	if err != nil {
		bitir("hata", "coin kuralları alınamadı: "+err.Error())
		return
	}
	if !kural.Acik || !kural.alinabilir() {
		bitir("hata", k.Sembol+" şu an MEXC'de alıma kapalı")
		return
	}
	if k.Mod == "gercek" {
		if bk, err := G.bakiyeYenile(); err == nil {
			if serbest := bk["USDT"].Serbest; serbest < butce {
				durumMu.Lock()
				olayEkle(k, "uyari", fmt.Sprintf("Hesaptaki serbest USDT (%s) kumbaradakinden (%s) az; olan kadar alınıyor", trSayi(serbest, 2), trSayi(butce, 2)))
				durumMu.Unlock()
				butce = serbest
			}
		}
	}
	if butce < kural.EnAzTutarPiyasa || butce < 1 {
		bitir("uyari", fmt.Sprintf("kumbaradaki para (%s USDT) MEXC'nin en az emir tutarının altında", trSayi(butce, 2)))
		return
	}
	olay := func(tur, mesaj string) {
		durumMu.Lock()
		olayEkle(k, tur, mesaj)
		k.EmirAciklama = mesaj
		durumSurum.Add(1)
		durumMu.Unlock()
	}
	sonuc, err := limitSonraPiyasa(b, piyasa, kural, "BUY", butce, 0, onek, olay)
	if err != nil {
		var bh belirsizHata
		if errors.As(err, &bh) && k.Mod == "gercek" {
			durumMu.Lock()
			k.Emirde, k.EmirAciklama = false, ""
			olayEkle(k, "uyari", "Emrin son durumu öğrenilemedi ("+err.Error()+"); MEXC'den tekrar sorulacak")
			degisti()
			durumMu.Unlock()
			return
		}
		bitir("hata", err.Error())
		return
	}
	if sonuc.Miktar <= 0 {
		bitir("hata", "emir dolmadı")
		return
	}
	durumMu.Lock()
	k.Emirde, k.EmirAciklama, k.Bekleyen = false, "", nil
	alisIsle(k, sonuc, beklenen, motorMum, kural, "")
	degisti()
	durumMu.Unlock()
}

// ---------- satış ----------

func kumbaraSat(k *kumbara, sebep, aciklama string, beklenen float64, acil bool) {
	durumMu.Lock()
	if k.Emirde || k.Bekleyen != nil || k.acikIslem() == nil {
		durumMu.Unlock()
		return
	}
	k.Emirde = true
	k.EmirAciklama = "Satış emri hazırlanıyor"
	onek := emirOneki(k)
	k.Bekleyen = &bekleyenEmir{Taraf: "SELL", Onek: onek, Baslangic: simdiF().UnixMilli(), Beklenen: beklenen, Sebep: sebep}
	miktar := k.Adet
	sonFiyat := k.SonFiyat
	degisti()
	durumMu.Unlock()

	hataylaBitir := func(hata string) {
		durumMu.Lock()
		k.Emirde, k.EmirAciklama, k.Bekleyen = false, "", nil
		k.TekrarSayisi++
		bekle := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second}[min(k.TekrarSayisi-1, 3)]
		olayEkle(k, "hata", fmt.Sprintf("Satış yapılamadı: %s. Tekrar denenecek", hata))
		k.SatisTekrar = simdiF().Add(bekle).UnixMilli()
		k.TekrarSebep = sebep
		degisti()
		durumMu.Unlock()
	}
	b, err := kumbaraBorsasi(k)
	if err != nil {
		hataylaBitir(err.Error())
		return
	}
	kural, err := kumbaraKurali(k)
	if err != nil {
		hataylaBitir("coin kuralları alınamadı: " + err.Error())
		return
	}
	if !kural.satilabilir() {
		hataylaBitir(k.Sembol + " şu an MEXC'de satışa kapalı")
		return
	}
	hesaptaYok := false
	if k.Mod == "gercek" {
		if bk, err := G.bakiyeYenile(); err == nil {
			if serbest := bk[kural.Varlik].Serbest; serbest < miktar*0.9999 {
				durumMu.Lock()
				olayEkle(k, "uyari", fmt.Sprintf("Hesaptaki serbest %s (%s) kumbaradakinden (%s) az (elle satılmış ya da başka bir emirde olabilir); olan satılıyor", kural.Varlik, trSayi(serbest, kural.MiktarOndalik), trSayi(miktar, kural.MiktarOndalik)))
				durumMu.Unlock()
				hesaptaYok = serbest < miktar*0.5
				miktar = serbest
			}
		}
	}
	// MEXC'nin en az miktarının altında kalan coin satılamaz: işlem kapandı sayılır, coin kumbarada kalır
	if fiyat := sonFiyat; asagiYuvarla(miktar, kural.MiktarOndalik) < kural.EnAzMiktar || (fiyat > 0 && miktar*fiyat < kural.EnAzTutarPiyasa) {
		durumMu.Lock()
		k.Emirde, k.EmirAciklama, k.Bekleyen = false, "", nil
		k.SatisTekrar, k.TekrarSebep, k.TekrarSayisi = 0, "", 0
		simdi := simdiF().UnixMilli()
		toz := dolumSonucu{Taraf: "SELL", Yol: "toz", Baslangic: simdi, Bitis: simdi, Parcalar: []parca{}}
		if hesaptaYok {
			olayEkle(k, "uyari", fmt.Sprintf("Kumbaranın aldığı %s hesapta yok (elle satılmış olabilir); işlem kapandı sayıldı", kural.Varlik))
			k.Adet = 0
		}
		satisIsle(k, toz, sebep, aciklama, beklenen, kural, "")
		if t := lastIslem(k); t != nil {
			// gerçekleşmemiş: kalan coinin bugünkü değeriyle tahmini sonuç (varsa önceki kısmi satışların geliri dahil)
			t.Gelen = t.kismiGelir() + k.Adet*fiyat
			t.Net = t.Gelen - t.Harcanan
			if t.Harcanan > 0 {
				t.Yuzde = t.Net / t.Harcanan * 100
			}
		}
		degisti()
		durumMu.Unlock()
		return
	}
	olay := func(tur, mesaj string) {
		durumMu.Lock()
		olayEkle(k, tur, mesaj)
		k.EmirAciklama = mesaj
		durumSurum.Add(1)
		durumMu.Unlock()
	}
	var sonuc dolumSonucu
	if acil {
		olay("uyari", "Beklemeden piyasa emriyle satılıyor: "+aciklama)
		sonuc, err = dogrudanPiyasa(b, piyasa, kural, "SELL", 0, miktar, onek, olay)
	} else {
		sonuc, err = limitSonraPiyasa(b, piyasa, kural, "SELL", 0, miktar, onek, olay)
	}
	if err != nil {
		var bh belirsizHata
		if errors.As(err, &bh) && k.Mod == "gercek" {
			durumMu.Lock()
			k.Emirde, k.EmirAciklama = false, ""
			olayEkle(k, "uyari", "Emrin son durumu öğrenilemedi ("+err.Error()+"); MEXC'den tekrar sorulacak")
			degisti()
			durumMu.Unlock()
			return
		}
		hataylaBitir(err.Error())
		return
	}
	if sonuc.Miktar <= 0 {
		hataylaBitir("emir dolmadı")
		return
	}
	durumMu.Lock()
	k.Emirde, k.EmirAciklama, k.Bekleyen = false, "", nil
	k.SatisTekrar, k.TekrarSebep, k.TekrarSayisi = 0, "", 0
	satisSonucuIsle(k, sonuc, sebep, aciklama, beklenen, kural, "")
	degisti()
	durumMu.Unlock()
}

func lastIslem(k *kumbara) *kumbaraIslemi {
	if n := len(k.Islemler); n > 0 {
		return &k.Islemler[n-1]
	}
	return nil
}

// ---------- karar eşleştirme ----------

type motorIslemi struct {
	G  int64    `json:"g"`  // alış mumu
	GF float64  `json:"gf"` // alış fiyatı
	C  *int64   `json:"c"`  // satış mumu
	CF *float64 `json:"cf"` // satış fiyatı
	K  string   `json:"k"`  // satış kuralı
}

type kararOlayi struct {
	Tip   string  `json:"tip"`
	Fiyat float64 `json:"fiyat"`
	Kural string  `json:"kural"`
}

type kararIstegi struct {
	Oturum         string        `json:"oturum"`
	ID             string        `json:"id"`
	Mum            int64         `json:"mum"` // son kapanan mumun açılış zamanı
	Asama          string        `json:"asama"`
	Olay           *kararOlayi   `json:"olay"`
	Stop           float64       `json:"stop"`
	Hedef          float64       `json:"hedef"`
	MotorGirisMum  int64         `json:"motorGirisMum"`
	MotorIslemleri []motorIslemi `json:"motorIslemleri"`
}

func (r *kararIstegi) motorIslemi(g int64) *motorIslemi {
	for i := range r.MotorIslemleri {
		if r.MotorIslemleri[i].G == g {
			return &r.MotorIslemleri[i]
		}
	}
	return nil
}

type eylem struct {
	tur, sebep, aciklama string
	beklenen             float64
	motorMum             int64
	acil                 bool
}

// Alım kararı ne kadar gecikmeyle uygulanabilir: mum süresinin %5'i, en az 30 sn
func tazelikSiniri(ms int64) int64 {
	return max(30_000, ms/20)
}

// durumMu kilitliyken çağrılır
func kararIsle(k *kumbara, r *kararIstegi) eylem {
	ms := k.ms()
	// kapanan işlemlerin simülatör tarafını doldur: anlık zarar durdurla, ömür sonunda ya da elle satılan
	// işlemde motorun kendi satışı sonradan gelir; beklenen sonuç motorun gerçek satış fiyatıyla hesaplanır.
	for i := range k.Islemler {
		t := &k.Islemler[i]
		if t.Satis == nil || t.MotorSebep != "" || t.MotorMum == 0 {
			continue
		}
		if mi := r.motorIslemi(t.MotorMum); mi != nil && mi.C != nil && mi.CF != nil {
			t.BeklenenSatis, t.MotorCikisMum, t.MotorSebep = *mi.CF, *mi.C, mi.K
			beklenenHesapla(t)
		}
	}
	akisSonucYaz(k, r) // 0.9: akış onayı kayıtlarının sonucu (motorun kapanan işlemlerinden)
	if r.Mum < k.SonKararMum {
		return eylem{}
	}
	yeni := r.Mum > k.SonKararMum
	k.SonKararMum = r.Mum
	k.SonKararZamani = simdiF().UnixMilli()
	k.MotorAsama = r.Asama
	if r.Asama == "coinde" {
		k.Stop, k.Hedef, k.MotorGirisMum = r.Stop, r.Hedef, r.MotorGirisMum
	} else {
		k.Stop, k.Hedef, k.MotorGirisMum = 0, 0, 0
	}
	korumaGolge(k, r) // 0.7: kenardaysa motorun işlemleri gölgede izlenir, gerekirse oyuna döner
	korumaDenetle(k)  // 0.7: yeni kapanan işlem varsa alarmlara bakılır (alımdan önce)
	if k.Emirde || k.Bekleyen != nil || !k.aktif() {
		return eylem{}
	}
	if t := k.acikIslem(); t != nil {
		cikti := r.Asama != "coinde" || (r.MotorGirisMum != 0 && t.MotorMum != 0 && r.MotorGirisMum != t.MotorMum)
		if !cikti || k.SatisTekrar > 0 {
			return eylem{}
		}
		e := eylem{tur: "sat", sebep: "motor", aciklama: "Simülatörün motoru artık coinde değil"}
		if r.Olay != nil && r.Olay.Tip == "sat" && (t.MotorMum == 0 || r.MotorGirisMum == 0 || r.Asama != "coinde") {
			e.sebep, e.beklenen, e.aciklama = r.Olay.Kural, r.Olay.Fiyat, "Simülatörün motoru sattı"
			t.MotorCikisMum, t.MotorSebep = r.Mum, r.Olay.Kural
		} else if mi := r.motorIslemi(t.MotorMum); mi != nil && mi.C != nil && mi.CF != nil {
			e.sebep, e.beklenen = mi.K, *mi.CF
			e.aciklama = "Simülatörün motoru daha önce satmıştı (kaçan satış)"
			t.MotorCikisMum, t.MotorSebep = *mi.C, mi.K
		}
		e.acil = e.sebep == "zararDurdur"
		return e
	}
	if !yeni || r.Olay == nil || r.Olay.Tip != "al" || k.Durum != "calisiyor" {
		return eylem{}
	}
	kapanis := r.Mum + ms - borsaFarki.Load() // mumun kapanışı, bu bilgisayarın saatiyle
	if kapanis <= k.Baslangic {
		return eylem{} // kumbara açılmadan önce kapanan mumun kararı: o işleme sonradan atlanmaz
	}
	if kr := k.Koruma; kr != nil {
		if kr.Kenarda {
			akisGolge(k, r) // 0.9: akış sistemi ne derdi (sadece kayıt)
			return eylem{}  // 0.7: kumbara kenarda; bu alım gölge işlem olarak izlenir
		}
		if kr.DonusMum > 0 && r.Mum <= kr.DonusMum {
			return eylem{} // oyuna dönülen mumun (ya da öncekinin) alımı: peşinden koşulmaz
		}
	}
	if gec := simdiF().UnixMilli() - kapanis; gec > tazelikSiniri(ms) {
		olayEkle(k, "bilgi", fmt.Sprintf("Simülatörün motoru %s kapanışında almıştı ama karar %s geç geldi; peşinden koşulmadı, sıradaki alım beklenecek", time.UnixMilli(kapanis).Format("15:04"), sureMetniTR(gec)))
		return eylem{}
	}
	// 0.9: akış onayı açıksa alım onaya kalır (akis_onay.go); kapalıysa eskisi gibi hemen alınır (gölge kaydedilir)
	return akisOnayKancasi(k, r, eylem{tur: "al", beklenen: r.Olay.Fiyat, motorMum: r.Mum})
}

func sureMetniTR(ms int64) string {
	sn := ms / 1000
	switch {
	case sn < 120:
		return fmt.Sprintf("%d sn", sn)
	case sn < 7200:
		return fmt.Sprintf("%d dk", sn/60)
	}
	return fmt.Sprintf("%d sa", sn/3600)
}

// Eylemi arka planda başlatır (testlerde yerine sahtesi konur)
var eylemiYap = func(k *kumbara, e eylem) {
	switch e.tur {
	case "al":
		go kumbaraAl(k, e.beklenen, e.motorMum)
	case "sat":
		go kumbaraSat(k, e.sebep, e.aciklama, e.beklenen, e.acil)
	}
}

// ---------- kumbara açma ve kapama ----------

type acmaIstegi struct {
	Dosya       string           `json:"dosya"`
	Icerik      json.RawMessage  `json:"-"`
	Sembol      string           `json:"-"`
	Aralik      string           `json:"-"`
	Olusturuldu int64            `json:"olusturuldu"`
	Bitis       int64            `json:"-"`
	Mod         string           `json:"mod"`
	Tutar       float64          `json:"tutar"`
	IlkMum      int64            `json:"ilkMum"`
	MumSayisi   int              `json:"mumSayisi"`
	Koruma      *korumaVerisi    `json:"koruma"`     // 0.7: ayarın geçmiş işlemleri (sayfada hesaplanır)
	Saglamlik   *saglamlikSonucu `json:"saglamlik"`  // 0.7: sağlamlık testi (sayfada hesaplanır)
	AkisKapali  bool             `json:"akisKapali"` // 0.9: bu kumbarada akış onayı kapalı
}

// durumMu kilitliyken çağrılır
func kumbaraOlustur(a acmaIstegi) *kumbara {
	simdi := simdiF().UnixMilli()
	omur := a.Bitis - a.Olusturuldu
	if omur < 0 {
		omur = 0
	}
	k := &kumbara{ID: rastgeleKimlik(), Dosya: a.Dosya, Sembol: a.Sembol, Aralik: a.Aralik, Mod: a.Mod, Icerik: a.Icerik,
		Olusturuldu: a.Olusturuldu, Bitis: a.Bitis, EkBitis: a.Bitis + omur/4, Baslangic: simdi, IlkMum: a.IlkMum, MumSayisi: a.MumSayisi,
		BaslangicTutar: a.Tutar, Nakit: a.Tutar, Durum: "calisiyor", Islemler: []kumbaraIslemi{}, Olaylar: []kumbaraOlayi{}}
	if a.AkisKapali {
		k.Akis = &akisKumbara{Kapali: true}
	}
	if a.Mod == "sanal" {
		D.SanalSerbest -= a.Tutar
		if D.SanalSerbest < 0 {
			D.SanalSerbest = 0
		}
	}
	modAd := map[string]string{"sanal": "sanal para", "gercek": "GERÇEK PARA (MEXC)"}[a.Mod]
	olayEkle(k, "bilgi", fmt.Sprintf("Kumbara açıldı: %s USDT, %s. Ömür %s'de bitiyor; elde coin kalırsa en geç %s'de satılır", trSayi(a.Tutar, 2), modAd,
		time.UnixMilli(a.Bitis).Format("02.01 15:04"), time.UnixMilli(k.EkBitis).Format("02.01 15:04")))
	D.Kumbaralar = append(D.Kumbaralar, k)
	return k
}

// durumMu kilitliyken çağrılır
func kumbaraBitir(k *kumbara, sebep string) {
	if !k.aktif() {
		return
	}
	k.Durum = "bitti"
	k.BitisSebebi = sebep
	k.BitisZamani = simdiF().UnixMilli()
	k.Stop, k.Hedef, k.HemenSat = 0, 0, false
	akisKumbaraBitti(k, "Kumbara kapandı") // 0.9
	if kr := k.Koruma; kr != nil {
		if n := len(kr.Donemler); n > 0 && kr.Donemler[n-1].Bit == 0 {
			kr.Donemler[n-1].Bit, kr.Donemler[n-1].Donus = k.BitisZamani, "kumbara kapandı"
		}
		kr.Gecmis = nil // biten kumbarada gerek yok (durum.json küçük kalsın)
	}
	defer olaylariKisalt(k) // 0.8: biten kumbaranın son 60 olayı saklanır (durum.json küçük kalsın)
	if k.Mod == "sanal" {
		k.Iade = k.Nakit + k.Adet*k.SonFiyat
		D.SanalSerbest += k.Iade
	} else {
		k.Iade = k.Nakit
	}
	sonuc := k.Iade - k.BaslangicTutar
	oran := 0.0
	if k.BaslangicTutar > 0 {
		oran = sonuc / k.BaslangicTutar
	}
	olayEkle(k, "bilgi", fmt.Sprintf("Kumbara kapandı (%s). Başlangıç %s, son %s USDT (%s)", sebep, trSayi(k.BaslangicTutar, 2), trSayi(k.Iade, 2), farkMetni(oran)))
	if p := k.Devam; p != nil {
		k.Devam = nil
		if simdiF().UnixMilli() >= p.Bitis {
			olayEkle(k, "uyari", "Yeni ayarın da süresi dolmuş; yeni kumbara açılmadı")
			return
		}
		tutar := math.Floor(k.Iade*100) / 100
		if tutar < 1 {
			olayEkle(k, "uyari", "Kumbarada devam edecek kadar para kalmadı; yeni kumbara açılmadı")
			return
		}
		var ic struct {
			Sembol string `json:"sembol"`
			Aralik string `json:"aralik"`
		}
		_ = json.Unmarshal(p.Icerik, &ic)
		y := kumbaraOlustur(acmaIstegi{Dosya: p.Dosya, Icerik: p.Icerik, Sembol: ic.Sembol, Aralik: ic.Aralik, Olusturuldu: p.Olusturuldu, Bitis: p.Bitis, Mod: k.Mod, Tutar: tutar,
			AkisKapali: k.Akis != nil && k.Akis.Kapali}) // 0.9: onayı kapalı kumbaranın devamında da kapalı
		y.OncekiID = k.ID
		olayEkle(y, "bilgi", "Simülatörde yeniden dışa aktarılan ayarla devam ediliyor (önceki kumbaranın parasıyla)")
		korumaGeciciKur(y) // 0.7: yeni ayarın geçmişi sayfada okununca asıl sınırlara geçilir
		if eski, yeni := k.Koruma, y.Koruma; eski != nil && yeni != nil && eski.TavanTepe > yeni.TavanTepe {
			// tavan kumbaranın (aynı paranın) toplam kaybını sınırlar: yeni ayarda da aynı tepeden sayılır
			yeni.TavanTepe = eski.TavanTepe
			olayEkle(y, "bilgi", "Tavan, önceki kumbaranın en yüksek değerinden ("+trSayi(eski.TavanTepe, 2)+" USDT) sayılmaya devam ediyor")
		}
		olayEkle(k, "bilgi", "Para yeni ayarla açılan kumbaraya geçti")
	}
}

// Biten kumbaranın sadece son 60 olayı saklanır (bütün işlemler ayrıca islemler.csv'de)
func olaylariKisalt(k *kumbara) {
	if !k.aktif() && len(k.Olaylar) > 60 {
		k.Olaylar = append([]kumbaraOlayi(nil), k.Olaylar[len(k.Olaylar)-60:]...)
	}
}

// ---------- izleyici: fiyat, zarar durdur, hedef, ömür ----------

var kurtarmaZamani = map[string]int64{} // durumMu ile korunur

func izleyiciDongusu() {
	for {
		uyu(2 * time.Second)
		izleyiciTur()
	}
}

func izleyiciTur() {
	durumMu.Lock()
	semboller := map[string]bool{}
	for _, k := range D.Kumbaralar {
		if k.aktif() {
			semboller[k.Sembol] = true
		}
	}
	durumMu.Unlock()
	fiyatlar := izleyiciFiyatlari(semboller) // 0.8: aynı anda, kısa zaman aşımıyla
	simdi := simdiF().UnixMilli()
	type yapilacak struct {
		k *kumbara
		e eylem
	}
	var liste []yapilacak
	var kurtarilacak []*kumbara
	durumMu.Lock()
	degisen := false
	for _, k := range D.Kumbaralar {
		if !k.aktif() {
			continue
		}
		f, fiyatVar := fiyatlar[k.Sembol]
		if fiyatVar {
			k.SonFiyat, k.SonFiyatZamani = f, simdi
		}
		if korumaDenetle(k) { // 0.7: yeni kapanan işlem varsa alarmlar
			degisen = true
		}
		if k.Durum == "calisiyor" && simdi >= k.Bitis {
			k.Durum = "sureDoldu"
			if k.acikIslem() != nil {
				olayEkle(k, "uyari", fmt.Sprintf("Ömür doldu: yeni alım yapılmayacak. Elde coin var; satış kuralı bekleniyor, en geç %s'de satılacak", time.UnixMilli(k.EkBitis).Format("02.01 15:04")))
			} else {
				olayEkle(k, "bilgi", "Ömür doldu: yeni alım yapılmayacak")
			}
			degisen = true
		}
		if k.Emirde {
			continue
		}
		if k.Bekleyen != nil {
			if k.Mod == "gercek" && simdi-kurtarmaZamani[k.ID] > 20_000 {
				kurtarmaZamani[k.ID] = simdi
				kurtarilacak = append(kurtarilacak, k)
			}
			continue
		}
		t := k.acikIslem()
		if t == nil {
			if k.Durum == "sureDoldu" || k.Durum == "durduruluyor" {
				neden := "ömür doldu"
				if k.Durum == "durduruluyor" {
					neden = "durduruldu"
					if k.Devam != nil {
						neden = "yeni ayara geçildi"
					}
				}
				kumbaraBitir(k, neden)
				degisen = true
			}
			continue
		}
		if k.SatisTekrar > 0 {
			if simdi >= k.SatisTekrar {
				k.SatisTekrar = 0
				liste = append(liste, yapilacak{k, eylem{tur: "sat", sebep: k.TekrarSebep, aciklama: "satış tekrar deneniyor", acil: true}})
			}
			continue
		}
		switch {
		case k.HemenSat:
			liste = append(liste, yapilacak{k, eylem{tur: "sat", sebep: "elle", aciklama: "şimdi sat ve durdur", acil: true}})
		case fiyatVar && k.Stop > 0 && f <= k.Stop:
			liste = append(liste, yapilacak{k, eylem{tur: "sat", sebep: "anlikZarar", aciklama: fmt.Sprintf("fiyat %s, zarar durdur seviyesi %s", trSayi(f, 8), trSayi(k.Stop, 8)), beklenen: k.Stop, acil: true}})
		case fiyatVar && k.Hedef > 0 && f >= k.Hedef:
			liste = append(liste, yapilacak{k, eylem{tur: "sat", sebep: "anlikHedef", aciklama: fmt.Sprintf("fiyat %s, hedef %s", trSayi(f, 8), trSayi(k.Hedef, 8)), beklenen: k.Hedef}})
		case simdi >= k.EkBitis:
			liste = append(liste, yapilacak{k, eylem{tur: "sat", sebep: "sureDoldu", aciklama: "ömür ve ek süre doldu"}})
		}
	}
	if degisen {
		degisti()
	} else if len(fiyatlar) > 0 {
		durumSurum.Add(1) // fiyatlar değişti; ekran yenilensin (diske yazmaya gerek yok)
	}
	durumMu.Unlock()
	for _, y := range liste {
		eylemiYap(y.k, y.e)
	}
	for _, k := range kurtarilacak {
		go kurtarF(k)
	}
}

var kurtarF = kurtar

// ---------- strateji klasörü: süresi dolanları silme, yeniden dışa aktarılanı izleme ----------

func klasorIzleyici() {
	sayac := 0
	for {
		uyu(10 * time.Second)
		yeniAyarKontrolu()
		sayac++
		if sayac%3 == 0 && os.Getenv("ALSAT_SILME_YOK") != "1" {
			dosyalariTemizle()
		}
	}
}

// Çalışan bir kumbaranın dosyası simülatörde yeniden dışa aktarıldıysa: eldeki işlem bitince yeni ayarla devam
func yeniAyarKontrolu() {
	klasor, _ := klasorBul()
	if !klasorVarMi(klasor) {
		return
	}
	ayarMu.Lock()
	omur := omurTablosu(ayarlariOku())
	ayarMu.Unlock()
	durumMu.Lock()
	type aday struct {
		id, dosya string
		olus      int64
	}
	var adaylar []aday
	for _, k := range D.Kumbaralar {
		if k.aktif() {
			adaylar = append(adaylar, aday{k.ID, k.Dosya, k.Olusturuldu})
		}
	}
	durumMu.Unlock()
	for _, a := range adaylar {
		d := dosyaOku(filepath.Join(klasor, a.dosya), omur, simdiF())
		if d.Durum != "gecerli" || d.Olusturuldu <= a.olus {
			continue
		}
		durumMu.Lock()
		k := kumbaraBul(a.id)
		if k != nil && k.aktif() && (k.Devam == nil || k.Devam.Olusturuldu != d.Olusturuldu) {
			k.Devam = &devamPlani{Dosya: k.Dosya, Icerik: d.Icerik, Olusturuldu: d.Olusturuldu, Bitis: d.Bitis}
			if k.Durum == "calisiyor" || k.Durum == "sureDoldu" {
				k.Durum = "durduruluyor"
			}
			if k.acikIslem() != nil || k.Emirde || k.Bekleyen != nil {
				olayEkle(k, "bilgi", "Dosya simülatörde yeniden dışa aktarıldı. Eldeki işlem bitince yeni ayarla, aynı parayla devam edilecek; bu arada yeni alım yapılmayacak")
			} else {
				olayEkle(k, "bilgi", "Dosya simülatörde yeniden dışa aktarıldı; yeni ayarla devam ediliyor")
				kumbaraBitir(k, "yeni ayara geçildi")
			}
			degisti()
		}
		durumMu.Unlock()
	}
}

func dosyalariTemizle() {
	klasor, _ := klasorBul()
	if !klasorVarMi(klasor) {
		return
	}
	ayarMu.Lock()
	omur := omurTablosu(ayarlariOku())
	ayarMu.Unlock()
	girdiler, err := os.ReadDir(klasor)
	if err != nil {
		return
	}
	for _, g := range girdiler {
		if g.IsDir() || !strings.EqualFold(filepath.Ext(g.Name()), ".json") {
			continue
		}
		yol := filepath.Join(klasor, g.Name())
		d := dosyaOku(yol, omur, simdiF())
		if d.Durum != "suresiDoldu" {
			continue
		}
		arsiv := filepath.Join(kayitKlasoru(), "arsiv")
		_ = os.MkdirAll(arsiv, 0755)
		hedef := filepath.Join(arsiv, strings.TrimSuffix(d.Ad, filepath.Ext(d.Ad))+"_"+time.UnixMilli(d.Olusturuldu).Format("20060102-150405")+".json")
		if err := os.WriteFile(hedef, d.Icerik, 0644); err != nil {
			log.Println("Arşive yazılamadı, dosya silinmedi:", err)
			continue
		}
		if err := os.Remove(yol); err != nil {
			log.Println("Süresi dolan dosya silinemedi:", err)
			continue
		}
		durumMu.Lock()
		genelOlay("bilgi", fmt.Sprintf("Süresi dolan strateji silindi: %s (kopyası kayitlar\\arsiv klasöründe)", d.Ad))
		degisti()
		durumMu.Unlock()
	}
}

// ---------- uç noktalar ----------

func jsonOku(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(v)
}

// Ekrana giden kumbara özeti: strateji içeriği (büyük) çıkarılır, olayların sonu verilir
func kumbaraOzeti(k *kumbara, olaySayisi int) map[string]any {
	kopya := *k
	kopya.Koruma = nil // koruma ayrıca (geçmiş işlemleri olmadan) eklenir
	kopya.Akis = nil   // 0.9: akış onayı ayrıca (son kayıtlar) eklenir
	m := map[string]any{}
	if b, err := json.Marshal(&kopya); err == nil {
		_ = json.Unmarshal(b, &m)
	} else {
		m = map[string]any{"id": k.ID, "dosya": k.Dosya, "sembol": k.Sembol, "aralik": k.Aralik, "mod": k.Mod, "durum": k.Durum, "hata": err.Error(),
			"islemler": []any{}, "olaylar": []any{}, "baslangicTutar": k.BaslangicTutar, "nakit": k.Nakit}
	}
	delete(m, "icerik")
	if d, ok := m["devam"].(map[string]any); ok {
		delete(d, "icerik")
	}
	if ol, ok := m["olaylar"].([]any); ok && len(ol) > olaySayisi {
		m["olaylar"] = ol[len(ol)-olaySayisi:]
	}
	m["deger"] = k.deger()
	if k.Koruma != nil {
		m["koruma"] = korumaOzeti(k) // geçmiş işlemler (büyük) gönderilmez
	}
	m["akis"] = akisOzeti(k) // 0.9
	return m
}

func durumIstegi(w http.ResponseWriter, r *http.Request) {
	var cevap map[string]any
	ayrilan := 0.0
	func() {
		durumMu.Lock()
		defer durumMu.Unlock() // bir hata olsa bile kilit açık kalmasın (izleyici ve zarar durdur buna bağlı)
		sanalKumbarada := 0.0
		liste := []map[string]any{}
		for _, k := range D.Kumbaralar {
			if k.aktif() {
				if k.Mod == "sanal" {
					sanalKumbarada += k.deger()
				} else {
					ayrilan += k.Nakit
				}
				liste = append(liste, kumbaraOzeti(k, 120))
			} else {
				liste = append(liste, kumbaraOzeti(k, 25))
			}
		}
		cevap = map[string]any{
			"surum": durumSurum.Load(), "simdi": simdiF().UnixMilli(), "test": testModu, "borsaFarki": borsaFarki.Load(),
			"sanal":        map[string]any{"serbest": D.SanalSerbest, "baslangic": D.SanalBaslangic, "kumbaralarda": sanalKumbarada, "toplam": D.SanalSerbest + sanalKumbarada},
			"kumbaralar":   liste,
			"genelOlaylar": D.GenelOlaylar,
			"durumHatasi":  durumHatasi, // 0.8
		}
	}()
	cevap["gercek"] = G.ozet(ayrilan)
	jsonYaz(w, 200, cevap)
}

// Bir kumbaranın tamamı (strateji içeriği ve bütün olaylar)
func kumbaraAyrintiIstegi(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	durumMu.Lock()
	defer durumMu.Unlock()
	k := kumbaraBul(id)
	if k == nil {
		hataYaz(w, 404, "Kumbara bulunamadı")
		return
	}
	jsonYaz(w, 200, k)
}

func kumbaraBaslatIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		acmaIstegi
		Oturum string `json:"oturum"`
		Onay   bool   `json:"onay"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil {
		hataYaz(w, 400, "İstek okunamadı")
		return
	}
	if !oturumGecerli(q.Oturum) {
		hataYaz(w, 409, "Bu pencere eskidi; programın son açılan penceresini kullan")
		return
	}
	if durumHatasi != "" {
		hataYaz(w, 409, durumHatasi)
		return
	}
	if q.Mod != "sanal" && q.Mod != "gercek" {
		hataYaz(w, 400, "Mod sanal ya da gerçek olmalı")
		return
	}
	if math.IsNaN(q.Tutar) || math.IsInf(q.Tutar, 0) || q.Tutar <= 0 {
		hataYaz(w, 400, "Tutar geçersiz")
		return
	}
	q.Tutar = math.Floor(q.Tutar*100) / 100
	if strings.ContainsAny(q.Dosya, `/\`) || q.Dosya == "" || q.Dosya == "." || q.Dosya == ".." {
		hataYaz(w, 400, "Dosya adı geçersiz")
		return
	}
	// dosyayı diskten yeniden oku: ekranda görülen dosya hâlâ geçerli mi?
	klasor, _ := klasorBul()
	ayarMu.Lock()
	omur := omurTablosu(ayarlariOku())
	ayarMu.Unlock()
	d := dosyaOku(filepath.Join(klasor, q.Dosya), omur, simdiF())
	switch {
	case d.Durum == "hatali":
		hataYaz(w, 400, "Dosya çalıştırılamaz: "+d.Hata)
		return
	case d.Durum == "suresiDoldu":
		hataYaz(w, 400, "Bu stratejinin ömrü dolmuş. Simülatörde yeniden dışa aktar.")
		return
	case d.Olusturuldu != q.Olusturuldu:
		hataYaz(w, 409, "Dosya bu arada değişti (yeniden dışa aktarılmış). Ekran yenilendi, tekrar dene.")
		return
	}
	kural, err := piyasa.Kurallar(d.Sembol)
	if err != nil {
		hataYaz(w, 502, "MEXC'den coin kuralları alınamadı: "+err.Error())
		return
	}
	if !kural.Acik || kural.TarafTuru > 1 {
		hataYaz(w, 400, d.Sembol+" şu an MEXC'de alım-satıma kapalı (ya da sadece tek yönlü işleme açık)")
		return
	}
	enAz := enAzKumbara(kural)
	if q.Tutar < enAz {
		hataYaz(w, 400, fmt.Sprintf("En az %s USDT ayırmalısın (MEXC'nin en az emir tutarı ve komisyon payı)", trSayi(enAz, 2)))
		return
	}
	if q.Mod == "gercek" {
		if !q.Onay {
			hataYaz(w, 400, "Gerçek parayla başlatmak için onay kutusunu işaretle")
			return
		}
		if msg := G.gercekBaslatmaKontrolu(d.Sembol, kural, q.Tutar); msg != "" {
			hataYaz(w, 400, msg)
			return
		}
	}
	// 0.7: koruma sınırları (torba) kilit dışında hesaplanır. Geçmiş gelmediyse ya da bozuksa geçici (yedek) sınırlar;
	// sayfa grafiği yükleyince asıl sınırları kurdurur. Kumbara hiçbir an korumasız kalmaz.
	kGecici := !q.Koruma.gecerli()
	var kGecmis []korumaIslemi
	var kSure int64
	kHas := ""
	if !kGecici {
		kGecmis, kSure, kHas = q.Koruma.Islemler, q.Koruma.Sure, q.Koruma.Hassasiyet
	}
	kH, kS, kKar, kTemiz := korumaSinirHesapla(d.Icerik, d.Aralik, d.Bitis-simdiF().UnixMilli(), kGecmis, kSure, kHas)
	durumMu.Lock()
	defer durumMu.Unlock()
	for _, k := range D.Kumbaralar {
		if k.aktif() && k.Dosya == q.Dosya {
			hataYaz(w, 409, "Bu dosya için zaten çalışan bir kumbara var")
			return
		}
	}
	if q.Mod == "sanal" && q.Tutar > D.SanalSerbest+1e-9 {
		hataYaz(w, 400, fmt.Sprintf("Sanal cüzdanda serbest %s USDT var", trSayi(D.SanalSerbest, 2)))
		return
	}
	if q.Mod == "gercek" {
		ayrilan := 0.0
		for _, k := range D.Kumbaralar {
			if k.aktif() && k.Mod == "gercek" {
				ayrilan += k.Nakit
			}
		}
		if serbest := G.usdtSerbest(); q.Tutar > serbest-ayrilan+1e-9 {
			hataYaz(w, 400, fmt.Sprintf("MEXC hesabında başka kumbaralara ayrılmamış %s USDT var", trSayi(math.Max(0, serbest-ayrilan), 2)))
			return
		}
	}
	q.Icerik, q.Sembol, q.Aralik, q.Bitis = d.Icerik, d.Sembol, d.Aralik, d.Bitis
	if q.IlkMum < 0 || q.MumSayisi < 0 || q.MumSayisi > enFazlaMum {
		q.IlkMum, q.MumSayisi = 0, 0
	}
	k := kumbaraOlustur(q.acmaIstegi)
	korumaKur(k, kH, kS, kKar, kTemiz, kGecici)
	k.Koruma.Saglamlik = q.Saglamlik.temizle()
	if !kGecici {
		go korumaVarsayilanKaydet(kH)
	}
	degisti()
	akisDegisti() // 0.9: bu coinin akış verisi hemen toplanmaya başlasın
	jsonYaz(w, 200, map[string]any{"id": k.ID})
}

// Kumbaraya en az ne kadar para ayrılmalı: MEXC'nin en az emir tutarının 1,5 katı, en az 5 USDT
func enAzKumbara(kural sembolKurali) float64 {
	return math.Max(5, math.Max(kural.EnAzTutar, kural.EnAzTutarPiyasa)*1.5)
}

// Sayfa, kumbaranın ilk mumunu (grafiğin başlangıcı) bildirir. Program yeniden açılınca aynı yerden yüklenir.
func kumbaraCapaIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum    string `json:"oturum"`
		ID        string `json:"id"`
		IlkMum    int64  `json:"ilkMum"`
		MumSayisi int    `json:"mumSayisi"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	k := kumbaraBul(q.ID)
	if k == nil {
		hataYaz(w, 404, "Kumbara bulunamadı")
		return
	}
	if k.IlkMum == 0 && q.IlkMum > 0 {
		k.IlkMum, k.MumSayisi = q.IlkMum, q.MumSayisi
		degisti()
	}
	jsonYaz(w, 200, map[string]any{"ilkMum": k.IlkMum, "mumSayisi": k.MumSayisi})
}

func kumbaraKararIstegi(w http.ResponseWriter, r *http.Request) {
	var q kararIstegi
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil {
		hataYaz(w, 400, "İstek okunamadı")
		return
	}
	if !oturumGecerli(q.Oturum) {
		jsonYaz(w, 409, map[string]string{"hata": "eski pencere"})
		return
	}
	durumMu.Lock()
	k := kumbaraBul(q.ID)
	if k == nil {
		durumMu.Unlock()
		jsonYaz(w, 404, map[string]string{"hata": "kumbara yok"})
		return
	}
	once := kumbaraImzasi(k)
	e := kararIsle(k, &q)
	if e.tur != "" || kumbaraImzasi(k) != once {
		degisti()
	} else {
		durumSurum.Add(1) // 0.8: değişen bir şey yoksa diske yazılmaz (ekranda gösterilen kumbara 5 sn'de bir bildirir; SD kart yıpranmasın)
	}
	durumMu.Unlock()
	eylemiYap(k, e)
	jsonYaz(w, 200, map[string]any{"eylem": e.tur})
}

// Kumbaranın diske yazılan hâlinin özeti (son karar zamanı hariç: o her bildirimde değişir ama saklanması gerekmez)
func kumbaraImzasi(k *kumbara) [32]byte {
	z := k.SonKararZamani
	k.SonKararZamani = 0
	b, _ := json.Marshal(k)
	k.SonKararZamani = z
	return sha256.Sum256(b)
}

func kumbaraDurdurIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string `json:"oturum"`
		ID     string `json:"id"`
		Nasil  string `json:"nasil"` // yeniAlimYok | simdiSat | satmadan
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
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
	if q.Nasil == "satmadan" {
		// Satış tekrar tekrar başarısız olduysa (ör. coin borsada işleme kapandı): kumbara satmadan kapanır,
		// coin hesapta kalır ve program ona artık dokunmaz.
		if k.Emirde || k.Bekleyen != nil {
			durumMu.Unlock()
			hataYaz(w, 409, "Şu an emir veriliyor; birkaç saniye sonra tekrar dene")
			return
		}
		satmadanKapat(k)
		degisti()
		durumMu.Unlock()
		jsonYaz(w, 200, map[string]bool{"tamam": true})
		return
	}
	k.Devam = nil
	k.Durum = "durduruluyor"
	var e eylem
	if q.Nasil == "simdiSat" {
		olayEkle(k, "uyari", "Elle: şimdi sat ve durdur")
		if k.acikIslem() != nil {
			k.HemenSat = true
			k.SatisTekrar = 0
			e = eylem{tur: "sat", sebep: "elle", aciklama: "şimdi sat ve durdur", acil: true}
		}
	} else {
		olayEkle(k, "bilgi", "Elle: yeni alım yapılmayacak. Elde coin varsa satış kuralıyla satılınca kumbara kapanacak")
	}
	if k.acikIslem() == nil && !k.Emirde && k.Bekleyen == nil {
		kumbaraBitir(k, "durduruldu")
	}
	degisti()
	durumMu.Unlock()
	eylemiYap(k, e)
	jsonYaz(w, 200, map[string]bool{"tamam": true})
}

// durumMu kilitliyken çağrılır
func satmadanKapat(k *kumbara) {
	k.Devam = nil
	if t := k.acikIslem(); t != nil {
		simdi := simdiF().UnixMilli()
		t.Satis = &dolumSonucu{Taraf: "SELL", Yol: "birakildi", Baslangic: simdi, Bitis: simdi, Parcalar: []parca{}}
		t.SatisSebebi = "elle"
		t.SatisAciklama = "satmadan kapatıldı"
		t.Gelen = t.kismiGelir() + k.Adet*k.SonFiyat // gerçekleşmemiş: bugünkü değeriyle (kısmi satışlar dahil)
		t.Net = t.Gelen - t.Harcanan
		if t.Harcanan > 0 {
			t.Yuzde = t.Net / t.Harcanan * 100
		}
		if k.Mod == "gercek" {
			olayEkle(k, "uyari", fmt.Sprintf("Elle: satmadan kapatıldı. %s %s MEXC hesabında duruyor; program ona artık dokunmaz", trSayi(k.Adet, 8), strings.TrimSuffix(k.Sembol, "USDT")))
		} else {
			olayEkle(k, "uyari", "Elle: satmadan kapatıldı (sanal coin bugünkü fiyatıyla cüzdana döner)")
		}
	}
	k.SatisTekrar, k.TekrarSebep, k.TekrarSayisi, k.HemenSat = 0, "", 0, false
	k.Durum = "durduruluyor"
	kumbaraBitir(k, "satmadan kapatıldı")
}

func hepsiniDurdurIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string `json:"oturum"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	durumMu.Lock()
	var satilacak []*kumbara
	for _, k := range D.Kumbaralar {
		if !k.aktif() {
			continue
		}
		k.Devam = nil
		k.Durum = "durduruluyor"
		olayEkle(k, "uyari", "Elle: hepsini sat ve durdur")
		if k.acikIslem() != nil {
			k.HemenSat = true
			k.SatisTekrar = 0
			satilacak = append(satilacak, k)
		} else if !k.Emirde && k.Bekleyen == nil {
			kumbaraBitir(k, "durduruldu")
		}
	}
	degisti()
	durumMu.Unlock()
	for _, k := range satilacak {
		go kumbaraSat(k, "elle", "hepsini sat ve durdur", 0, true)
	}
	jsonYaz(w, 200, map[string]any{"satilan": len(satilacak)})
}

func sanalSifirlaIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string  `json:"oturum"`
		Bakiye float64 `json:"bakiye"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	if math.IsNaN(q.Bakiye) || q.Bakiye < 10 || q.Bakiye > 1e7 {
		hataYaz(w, 400, "Bakiye 10 ile 10.000.000 arasında olmalı")
		return
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	for _, k := range D.Kumbaralar {
		if k.aktif() && k.Mod == "sanal" {
			hataYaz(w, 409, "Önce çalışan sanal kumbaraları durdur")
			return
		}
	}
	q.Bakiye = math.Floor(q.Bakiye*100) / 100
	D.SanalSerbest, D.SanalBaslangic = q.Bakiye, q.Bakiye
	genelOlay("bilgi", fmt.Sprintf("Sanal cüzdan sıfırlandı: %s USDT", trSayi(q.Bakiye, 2)))
	degisti()
	jsonYaz(w, 200, map[string]bool{"tamam": true})
}
