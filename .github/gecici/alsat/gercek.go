package main

// GERÇEK PARA — MEXC hesabına bağlantı.
// Anahtar ekrandaki "MEXC bağlantısı" kutusuna girilir, buraya gelir; önce MEXC'ye sorulup izinleri denetlenir:
//   - para çekme (WITHDRAW) ya da transfer (TRANSFER) izni varsa anahtar KULLANILMAZ ve KAYDEDİLMEZ,
//   - gereken üç izin (hesabı görme, emirleri görme, alım-satım) yoksa yine kullanılmaz.
// Uygunsa bu bilgisayarda Windows şifrelemesiyle saklanır (anahtar.go). Gizli anahtar sayfaya geri gönderilmez.
// Program her açılışta kayıtlı anahtarla yeniden bağlanır ve izinleri yeniden denetler.

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type gercekDurum struct {
	mu          sync.Mutex
	Bagli       bool
	ist         *mexcIstemci
	borsa       *mexcBorsa
	maske       string
	izinler     []string
	fazlaIzin   []string
	kalanGun    int
	ipListesi   string
	hata        string
	kalici      bool // kayıtlı anahtar bu Windows hesabında açılamıyor: yeniden denemenin anlamı yok
	bakiye      map[string]bakiye
	bakiyeZaman int64
	komisyonlar map[string][2]float64
	denetZaman  int64
}

var G = &gercekDurum{komisyonlar: map[string][2]float64{}}

var izinAdlari = map[string]string{
	"SPOT_ACCOUNT_READ":      "Hesap bilgisini görme",
	"SPOT_ACCOUNT_WRITE":     "Hesap ayarlarını değiştirme",
	"SPOT_DEAL_READ":         "Emirleri görme",
	"SPOT_DEAL_WRITE":        "Alım-satım (emir verme)",
	"SPOT_WITHDRAW_READ":     "Para çekme bilgisini görme",
	"SPOT_WITHDRAW_WRITE":    "PARA ÇEKME",
	"SPOT_TRANSFER_READ":     "Transfer bilgisini görme",
	"SPOT_TRANSFER_WRITE":    "TRANSFER",
	"CONTRACT_ACCOUNT_READ":  "Vadeli hesap görme",
	"CONTRACT_ACCOUNT_WRITE": "Vadeli hesap değiştirme",
	"CONTRACT_DEAL_READ":     "Vadeli emir görme",
	"CONTRACT_DEAL_WRITE":    "Vadeli alım-satım",
}

func izinAdi(x string) string {
	if a, ok := izinAdlari[x]; ok {
		return a + " (" + x + ")"
	}
	return x
}

func izinListesi(l []string) string {
	var out []string
	for _, x := range l {
		out = append(out, izinAdi(x))
	}
	return strings.Join(out, ", ")
}

// Bağlantı denemesinin sonucu (ekrana gider)
type baglantiSonucu struct {
	Tamam    bool     `json:"tamam"`
	Hata     string   `json:"hata,omitempty"`
	Yasak    []string `json:"yasak,omitempty"`
	Eksik    []string `json:"eksik,omitempty"`
	Uyarilar []string `json:"uyarilar,omitempty"`
	Gecici   bool     `json:"gecici,omitempty"` // internet/MEXC sorunu: sonra yeniden denenebilir
	// 0.8: anahtar KESİN olarak kullanılamaz (silinmiş/dondurulmuş, süresi dolmuş, para çekme izni eklenmiş, izin
	// eksik, anahtar ya da gizli anahtar yanlış). Çalışan bağlantı sadece bunlarda kesilir; başka her sorunda
	// (MEXC'nin geçici hatası, saat farkı, tanınmayan cevap) bağlantı sürer, sonra yeniden denenir.
	Kesin bool `json:"kesin,omitempty"`
}

// Anahtarı MEXC'ye sorar ve denetler. Uygunsa bağlantıyı kurar.
func (g *gercekDurum) dene(erisim, gizli string) baglantiSonucu {
	if testModu {
		return baglantiSonucu{Hata: "Test modunda gerçek para kapalı"}
	}
	erisim, gizli = strings.TrimSpace(erisim), strings.TrimSpace(gizli)
	if erisim == "" || gizli == "" {
		return baglantiSonucu{Hata: "Access Key ve Secret Key ikisi de gerekli"}
	}
	if strings.ContainsAny(erisim+gizli, " \t\r\n") || len(erisim) > 128 || len(gizli) > 256 {
		return baglantiSonucu{Hata: "Anahtar biçimi hatalı (boşluk ya da fazladan karakter olmamalı)"}
	}
	ist := yeniMexcIstemci(erisim, gizli)
	if err := ist.saatEsitle(); err != nil {
		return baglantiSonucu{Hata: "MEXC'ye ulaşılamadı: " + err.Error(), Gecici: true}
	}
	ab, err := ist.anahtarBilgisi()
	if err != nil {
		var he mexcHata
		if errors.As(err, &he) {
			switch {
			case he.Kod == 10072 || he.Kod == 700002 || he.Kod == 602:
				return baglantiSonucu{Hata: "MEXC anahtarı kabul etmedi (anahtar silinmiş ya da Access/Secret Key yanlış): " + err.Error(), Kesin: true}
			case he.Kod == 700006:
				return baglantiSonucu{Hata: "MEXC bu cihazın IP adresine izin vermiyor (anahtarın IP listesi): " + err.Error(), Gecici: true}
			case he.HTTP >= 500 || he.HTTP == 429 || he.Kod == 700003 || he.Kod == 10073 || he.Kod == 0:
				// 0.8: saat farkı ve kodsuz cevaplar (ör. MEXC önündeki ağ geçidinin "403" sayfası) da geçici sayılır
				return baglantiSonucu{Hata: "MEXC şu an cevap vermiyor: " + err.Error(), Gecici: true}
			}
			return baglantiSonucu{Hata: "MEXC anahtarı kabul etmedi: " + err.Error()}
		}
		return baglantiSonucu{Hata: "MEXC'ye ulaşılamadı: " + err.Error(), Gecici: true}
	}
	if d := strings.ToUpper(ab.Durum); d != "" && d != "<NIL>" && d != "VALID" {
		return baglantiSonucu{Hata: "Bu anahtar MEXC'de geçerli değil (durum: " + ab.Durum + "). MEXC'de yeni anahtar oluştur.", Kesin: true}
	}
	if ab.KalanBiliniyor && ab.KalanGun == 0 {
		return baglantiSonucu{Hata: "Bu anahtarın süresi dolmuş. MEXC'de yeni anahtar oluştur (IP adresi bağlamazsan 90 gün geçerli).", Kesin: true}
	}
	if len(ab.Izinler) == 0 {
		return baglantiSonucu{Hata: "Anahtarın izinleri okunamadı. Güvenlik için kullanılmadı."}
	}
	yasak, eksik := izinDenetle(ab.Izinler)
	if len(yasak) > 0 {
		return baglantiSonucu{Yasak: yasak, Kesin: true, Hata: "Bu anahtarda para çekme ya da transfer izni var: " + izinListesi(yasak) +
			". Güvenliğin için KULLANILMADI ve KAYDEDİLMEDİ. MEXC'de bu anahtarı sil; sadece hesap bilgisini görme, emirleri görme ve alım-satım izinleriyle yeni anahtar oluştur."}
	}
	if len(eksik) > 0 {
		return baglantiSonucu{Eksik: eksik, Kesin: true, Hata: "Anahtarda gereken izin eksik: " + izinListesi(eksik) + ". MEXC'de anahtarın izinlerine ekle ya da yeni anahtar oluştur."}
	}
	bk, err := ist.bakiyeler()
	if err != nil {
		return baglantiSonucu{Hata: "Hesap bakiyesi okunamadı: " + err.Error(), Gecici: true}
	}
	var uyarilar, fazla []string
	gerekli := map[string]bool{"SPOT_ACCOUNT_READ": true, "SPOT_DEAL_READ": true, "SPOT_DEAL_WRITE": true}
	for _, x := range ab.Izinler {
		if !gerekli[x] {
			fazla = append(fazla, x)
		}
	}
	if len(fazla) > 0 {
		uyarilar = append(uyarilar, "Anahtarda bu programın kullanmadığı izinler de var: "+izinListesi(fazla)+". İstersen MEXC'de kaldırabilirsin.")
	}
	if ab.KalanGun > 0 && ab.KalanGun <= 7 {
		uyarilar = append(uyarilar, fmt.Sprintf("Anahtarın süresi %d gün sonra doluyor. MEXC'de yenilemen gerekecek.", ab.KalanGun))
	}
	g.mu.Lock()
	g.Bagli, g.ist, g.borsa = true, ist, &mexcBorsa{ist: ist, p: piyasa}
	g.maske, g.izinler, g.fazlaIzin, g.kalanGun, g.ipListesi = anahtarMaskesi(erisim), ab.Izinler, fazla, ab.KalanGun, ab.IPListesi
	g.hata, g.kalici = "", false
	g.bakiye, g.bakiyeZaman = bk, simdiF().UnixMilli()
	g.denetZaman = simdiF().UnixMilli()
	g.mu.Unlock()
	log.Printf("MEXC bağlantısı kuruldu (anahtar %s, izinler %s, kalan gün %d)", anahtarMaskesi(erisim), strings.Join(ab.Izinler, ","), ab.KalanGun)
	return baglantiSonucu{Tamam: true, Uyarilar: uyarilar}
}

func (g *gercekDurum) kes(sebep string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Bagli, g.ist, g.borsa = false, nil, nil
	g.bakiye, g.bakiyeZaman = nil, 0
	g.izinler, g.fazlaIzin = nil, nil
	g.hata = sebep
}

// Kayıtlı anahtarla bağlan (açılışta ve bağlantı koptuğunda)
func (g *gercekDurum) kayitliIleBaglan() {
	if testModu {
		return
	}
	a, err := anahtarOku()
	if err != nil {
		if !os.IsNotExist(err) {
			g.mu.Lock()
			g.hata, g.kalici = "Kayıtlı anahtar açılamadı ("+err.Error()+"). Anahtarı yeniden gir.", true
			g.mu.Unlock()
		}
		return
	}
	s := g.dene(a.Erisim, a.Gizli)
	if s.Tamam {
		return
	}
	g.mu.Lock()
	g.hata = s.Hata
	g.maske = anahtarMaskesi(a.Erisim)
	g.kalici = s.Kesin // 0.8: sadece kesin sebepte yeniden denemeyi bırak (0.7: geçici olmayan her hatada)
	g.mu.Unlock()
	log.Println("Kayıtlı anahtarla bağlanılamadı:", s.Hata)
}

// Arka planda: bağlantı yoksa kayıtlı anahtarla dakikada bir dener; varsa bakiyeyi tazeler, 6 saatte bir izinleri yeniden denetler.
func gercekDongusu() {
	G.kayitliIleBaglan()
	for {
		uyu(30 * time.Second)
		gercekTur()
	}
}

// Döngünün bir turu (testler doğrudan çağırır)
func gercekTur() {
	G.mu.Lock()
	bagli, kalici, denet := G.Bagli, G.kalici, G.denetZaman
	G.mu.Unlock()
	if !bagli {
		if !kalici {
			G.kayitliIleBaglan()
		}
		return
	}
	if simdiF().UnixMilli()-denet > 6*3600*1000 {
		// izinler değişmiş olabilir: ör. anahtara sonradan para çekme izni eklendiyse bağlantı kesilir
		sonraki := simdiF().UnixMilli()
		if a, err := anahtarOku(); err == nil {
			if s := G.dene(a.Erisim, a.Gizli); !s.Tamam && s.Kesin {
				G.kes(s.Hata)
				G.mu.Lock()
				G.kalici = true
				G.mu.Unlock()
				log.Println("MEXC bağlantısı kesildi:", s.Hata)
			} else if !s.Tamam {
				// 0.8: denetim yapılamadı (MEXC'nin geçici sorunu): bağlantı sürer, 30 dk sonra yeniden denenir.
				// 0.7'de saat farkı ya da tanınmayan bir cevap da bağlantıyı kalıcı kesiyordu: coin tutan kumbaralar
				// satamaz, zarar durdur çalışmaz hâle geliyordu.
				log.Println("6 saatlik anahtar denetimi yapılamadı (bağlantı sürüyor, 30 dk sonra yeniden):", s.Hata)
				sonraki = sonraki - 6*3600*1000 + 30*60*1000
			}
		}
		G.mu.Lock()
		G.denetZaman = sonraki
		G.mu.Unlock()
		return
	}
	_, _ = G.bakiyeYenile()
}

func (g *gercekDurum) istemci() *mexcIstemci {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.Bagli {
		return nil
	}
	return g.ist
}

func (g *gercekDurum) bakiyeYenile() (map[string]bakiye, error) {
	ist := g.istemci()
	if ist == nil {
		return nil, errors.New("MEXC bağlantısı yok")
	}
	bk, err := ist.bakiyeler()
	if err != nil {
		var he mexcHata
		if errors.As(err, &he) && (he.Kod == 700002 || he.Kod == 602 || he.Kod == 10072 || he.Kod == 700006 || he.Kod == 700007) {
			// anahtar silinmiş, değiştirilmiş ya da IP izni yok: bağlantıyı kes, ekranda söyle
			g.kes("MEXC anahtarı artık kabul etmiyor: " + err.Error())
			log.Println("MEXC bağlantısı kesildi:", err)
		}
		return nil, err
	}
	g.mu.Lock()
	g.bakiye, g.bakiyeZaman = bk, simdiF().UnixMilli()
	g.mu.Unlock()
	return bk, nil
}

func (g *gercekDurum) usdtSerbest() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.bakiye == nil {
		return 0
	}
	return g.bakiye["USDT"].Serbest
}

func (g *gercekDurum) ozet(ayrilan float64) map[string]any {
	_, err := os.Stat(anahtarYolu())
	g.mu.Lock()
	defer g.mu.Unlock()
	m := map[string]any{"kayitli": err == nil, "bagli": g.Bagli, "maske": g.maske, "izinler": g.izinler, "fazlaIzin": g.fazlaIzin,
		"kalanGun": g.kalanGun, "ip": g.ipListesi, "hata": g.hata, "ayrilan": ayrilan, "bakiyeZamani": g.bakiyeZaman, "test": testModu}
	if g.bakiye != nil {
		u := g.bakiye["USDT"]
		m["usdt"] = map[string]float64{"serbest": u.Serbest, "kilitli": u.Kilitli}
		var coinler []map[string]any
		for ad, b := range g.bakiye {
			if ad != "USDT" && b.Serbest+b.Kilitli > 0 {
				coinler = append(coinler, map[string]any{"ad": ad, "serbest": b.Serbest, "kilitli": b.Kilitli})
			}
		}
		sort.Slice(coinler, func(i, j int) bool { return coinler[i]["ad"].(string) < coinler[j]["ad"].(string) })
		m["coinler"] = coinler
	}
	return m
}

// Gerçek parayla başlatmadan önce: bağlantı, coin, komisyon ve MEXC'nin deneme emri (gerçek emir verilmez)
func (g *gercekDurum) gercekBaslatmaKontrolu(sembol string, kural sembolKurali, tutar float64) string {
	ist := g.istemci()
	if ist == nil {
		return "Gerçek parayla başlatmak için önce MEXC bağlantısını kur (üstteki \"MEXC\" düğmesi)"
	}
	if len(kural.Turler) > 0 {
		piyasaVar := false
		for _, t := range kural.Turler {
			if t == "MARKET" {
				piyasaVar = true
			}
		}
		if !piyasaVar {
			return sembol + " için MEXC'de piyasa emri kapalı; 15 saniye kuralı uygulanamaz"
		}
	}
	if _, err := g.bakiyeYenile(); err != nil {
		return "MEXC hesabı okunamadı: " + err.Error()
	}
	if mk, tk, err := ist.komisyon(sembol); err == nil && mk >= 0 && mk <= 0.01 && tk >= 0 && tk <= 0.01 {
		g.mu.Lock()
		g.komisyonlar[sembol] = [2]float64{mk, tk}
		g.mu.Unlock()
	}
	du, err := piyasa.EnIyiFiyat(sembol)
	if err != nil {
		return "Fiyat alınamadı: " + err.Error()
	}
	fiyat := limitFiyati("BUY", du, kural)
	miktar := asagiYuvarla(tutar/fiyat, kural.MiktarOndalik)
	if err := ist.denemeEmri(kural, fiyat, miktar); err != nil {
		msg := "MEXC deneme emrini kabul etmedi: " + err.Error()
		if liste, e2 := ist.apiSembolleri(); e2 == nil && len(liste) > 0 {
			listede := false
			for _, s := range liste {
				if s == sembol {
					listede = true
				}
			}
			if !listede {
				msg += ". " + sembol + " senin API işlem listende görünmüyor; MEXC'de API yönetimi sayfasında bu coini işlem çiftlerine eklemen gerekebilir."
			}
		}
		return msg
	}
	return ""
}

// ---------- program kapanırken yarım kalan gerçek emri kurtarma ----------

// Program bir emrin ortasında kapandıysa: MEXC'ye o emirlerin kimliğiyle sorulur, açık kalan iptal edilir,
// dolan kısım kumbaraya işlenir. Böylece hesap ile kumbara hep uyuşur.
func kurtar(k *kumbara) {
	durumMu.Lock()
	if k.Emirde || k.Bekleyen == nil || k.Mod != "gercek" {
		durumMu.Unlock()
		return
	}
	be := *k.Bekleyen
	k.Emirde = true
	k.EmirAciklama = "Yarım kalan emir MEXC'den soruluyor"
	degisti()
	durumMu.Unlock()

	birak := func(mesaj string, temizle bool) {
		durumMu.Lock()
		k.Emirde, k.EmirAciklama = false, ""
		if temizle {
			k.Bekleyen = nil
		}
		if mesaj != "" {
			olayEkle(k, "bilgi", mesaj)
		}
		degisti()
		durumMu.Unlock()
	}
	G.mu.Lock()
	mb := G.borsa
	G.mu.Unlock()
	if mb == nil {
		birak("", false) // bağlantı yok: sonra yeniden denenir
		return
	}
	kural, err := kumbaraKurali(k)
	if err != nil {
		birak("", false)
		return
	}
	var parcalar []parca
	for _, ek := range []string{"L", "P"} {
		d, err := mb.IstemciEmri(kural, be.Onek+ek)
		if err != nil {
			var he mexcHata
			if errors.As(err, &he) && (he.Kod == -2013 || he.Kod == -2011 || he.Kod == 22222 || he.HTTP == 404) {
				continue // böyle bir emir hiç verilmemiş
			}
			birak("", false) // MEXC'ye ulaşılamadı: sonra yeniden
			return
		}
		if !d.bitti() {
			if d2, err := mb.Iptal(kural, d.ID); err == nil {
				d = d2
			}
			if !d.bitti() {
				// 0.8: emir MEXC'de hâlâ açık (iptal kesinleşmedi): kalan kısmı sonradan dolabilir. Bekleyen silinmez,
				// sonra yeniden denenir. (0.7'de burada Bekleyen siliniyor, sonradan dolan kısım kumbaraya hiç girmiyordu.)
				birak("", false)
				return
			}
		}
		if d.Dolan > 0 {
			p := emrinDolumlari(mb, kural, d.ID, d, ek == "L")
			kaynak := map[string]string{"L": "limit", "P": "piyasa"}[ek]
			for i := range p {
				p[i].Kaynak = kaynak
			}
			parcalar = append(parcalar, p...)
		}
	}
	if len(parcalar) == 0 {
		birak("Yarım kalan emir MEXC'de hiç dolmamış; hesapta değişiklik yok", true)
		return
	}
	sonuc := dolumTopla(be.Taraf, kural, parcalar)
	sonuc.Baslangic, sonuc.Bitis = be.Baslangic, simdiF().UnixMilli()
	durumMu.Lock()
	k.Emirde, k.EmirAciklama, k.Bekleyen = false, "", nil
	if be.Taraf == "BUY" {
		alisIsle(k, sonuc, be.Beklenen, be.MotorMum, kural, "Yarım kalan alış emri MEXC'de dolmuş; kumbaraya işlendi. ")
	} else {
		satisSonucuIsle(k, sonuc, be.Sebep, "Yarım kalan satış emri", be.Beklenen, kural, "Yarım kalan satış emri MEXC'de dolmuş; kumbaraya işlendi. ")
	}
	degisti()
	durumMu.Unlock()
}

// ---------- uç noktalar ----------

func anahtarIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string `json:"oturum"`
		Erisim string `json:"erisim"`
		Gizli  string `json:"gizli"`
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
	gercekVar := false
	for _, k := range D.Kumbaralar {
		if k.aktif() && k.Mod == "gercek" {
			gercekVar = true
		}
	}
	durumMu.Unlock()
	// Bağlantı çalışırken anahtar değiştirilmez (kumbaraların coinleri başka hesaba düşmesin). Bağlantı koptuysa
	// (ör. anahtarın süresi doldu) yeni anahtar girilebilir; yoksa elde coin tutan kumbara hiç satamazdı.
	if gercekVar && G.istemci() != nil {
		hataYaz(w, 409, "Gerçek parayla çalışan kumbara varken çalışan anahtar değiştirilemez. Önce onları durdur.")
		return
	}
	s := G.dene(q.Erisim, q.Gizli)
	if !s.Tamam {
		log.Println("Anahtar kabul edilmedi:", s.Hata)
		jsonYaz(w, 400, s)
		return
	}
	if err := anahtarKaydet(anahtarKaydi{Erisim: q.Erisim, Gizli: q.Gizli}); err != nil {
		G.kes("Anahtar bu bilgisayara kaydedilemedi: " + err.Error())
		hataYaz(w, 500, "Anahtar bu bilgisayara kaydedilemedi: "+err.Error())
		return
	}
	durumMu.Lock()
	genelOlay("bilgi", "MEXC bağlantısı kuruldu, anahtar bu bilgisayarda şifreli saklandı")
	degisti()
	durumMu.Unlock()
	jsonYaz(w, 200, s)
}

func anahtarSilIstegi(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Oturum string `json:"oturum"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	for _, k := range D.Kumbaralar {
		if k.aktif() && k.Mod == "gercek" {
			hataYaz(w, 409, "Gerçek parayla çalışan kumbara varken anahtar silinemez. Önce onları durdur.")
			return
		}
	}
	if err := anahtarSil(); err != nil {
		hataYaz(w, 500, "Anahtar dosyası silinemedi: "+err.Error())
		return
	}
	G.kes("")
	G.mu.Lock()
	G.maske, G.kalici = "", false
	G.mu.Unlock()
	genelOlay("bilgi", "MEXC anahtarı bu bilgisayardan silindi")
	degisti()
	jsonYaz(w, 200, map[string]bool{"tamam": true})
}

func gercekYenileIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		hataYaz(w, 405, "Sadece POST")
		return
	}
	if G.istemci() == nil {
		G.mu.Lock()
		kalici := G.kalici
		G.mu.Unlock()
		if !kalici {
			G.kayitliIleBaglan()
		}
	} else if _, err := G.bakiyeYenile(); err != nil {
		hataYaz(w, 502, "Bakiye okunamadı: "+err.Error())
		return
	}
	durumSurum.Add(1)
	jsonYaz(w, 200, map[string]bool{"tamam": true})
}
