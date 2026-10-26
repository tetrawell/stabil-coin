package main

// UZAKTAN STRATEJİ ALMA (0.8) — bilgisayardaki Kripto Simülatör'ün "Pi'ye gönder" butonu strateji dosyasını
// ev ağı üzerinden bu programa gönderir; dosya, simülatörün "Dışa aktar"ı gibi strateji klasörüne yazılır ve
// Al-Sat onu kendiliğinden görür. Kumbara BAŞLATMAZ: başlatmak her zaman kullanıcının işidir. (Çalışan bir kumbaranın
// dosyası gelirse, yeniden dışa aktarmadaki gibi eldeki işlem bitince yeni ayarla devam edilir.)
//
// Varsayılan KAPALI. Kullanıcı Al-Sat'ta "Bilgisayardan al"ı açınca rastgele bir eşleştirme kodu üretilir.
// Güvenlik:
//   - Ayrı kapı (47323). Asıl ekran sunucusu (47322) eskisi gibi sadece bu cihazdan erişilir.
//   - Her istek eşleştirme koduyla imzalı (HMAC-SHA256: yol + zaman + dosya adı + gövdenin özeti). Kod ağda gezmez.
//   - Zaman damgası en fazla 5 dk eski/ileri olabilir; aynı imza ikinci kez kabul edilmez (tekrar oynatma yok).
//   - Bir adresten 10 dk içinde 10 hatalı imza gelirse o adres 10 dk engellenir.
//   - Sadece strateji dosyası: en fazla 1 MB, adı COIN_SÜRE.json, içi stratejiler.go → dosyaOku kontrollerinden geçmeli
//     (MEXC verisi, süresi dolmamış, gelecekten değil). Başka hiçbir dosyaya, ayara ya da kumbaraya dokunamaz.
//   - Kod ve açık/kapalı durumu anahtar klasöründe (Pi: ~/.kriptoalsat/uzak.json, 0600), exe'nin yanında değil.

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	uzakPort       = 47323
	uzakSaatPayi   = 5 * time.Minute
	uzakEnFazla    = 1 << 20
	uzakHataSiniri = 10
	uzakHataSuresi = 10 * time.Minute
)

type uzakKaydi struct {
	Acik bool   `json:"acik"`
	Kod  string `json:"kod"`
}

type uzakSonGelen struct {
	Zaman  int64  `json:"zaman"`
	Dosya  string `json:"dosya"`
	Adres  string `json:"adres"`
	Durum  string `json:"durum"` // yeni | yenilendi | calisanKumbara
	Mesaj  string `json:"mesaj"`
	Basari bool   `json:"basari"`
}

var uzak = struct {
	mu       sync.Mutex
	kayit    uzakKaydi
	ln       net.Listener
	hata     string           // kapı açılamadıysa sebebi
	gorulen  map[string]int64 // imza → zaman (tekrar oynatmaya karşı)
	hatalar  map[string][]int64
	sonGelen []uzakSonGelen
	yuklendi bool
}{gorulen: map[string]int64{}, hatalar: map[string][]int64{}}

func uzakYolu() string { return filepath.Join(anahtarKlasoru(), "uzak.json") }

// Kod: 12 harf/rakam (karışabilecek 0/O, 1/I yok), ekranda 4'lü gruplar hâlinde. 60 bit rastgelelik.
const uzakAlfabe = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func uzakKodUret() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	var s strings.Builder
	for i, x := range b {
		if i > 0 && i%4 == 0 {
			s.WriteByte('-')
		}
		s.WriteByte(uzakAlfabe[int(x)%len(uzakAlfabe)])
	}
	return s.String()
}

// Kullanıcının yazdığı kodu karşılaştırılabilir hâle getirir (büyük harf, tire/boşluk yok)
func uzakKodSade(s string) string {
	s = strings.ToUpper(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// İmza: HMAC-SHA256(kod, yol \n zaman \n dosya \n sha256(gövde)) — iki program aynı şekilde hesaplar
func uzakImza(kod, yol, zaman, dosya string, govde []byte) string {
	oz := sha256.Sum256(govde)
	m := hmac.New(sha256.New, []byte(uzakKodSade(kod)))
	m.Write([]byte(yol + "\n" + zaman + "\n" + dosya + "\n" + hex.EncodeToString(oz[:])))
	return hex.EncodeToString(m.Sum(nil))
}

func uzakYukle() {
	uzak.mu.Lock()
	defer uzak.mu.Unlock()
	if uzak.yuklendi {
		return
	}
	uzak.yuklendi = true
	if b, err := os.ReadFile(uzakYolu()); err == nil {
		_ = json.Unmarshal(b, &uzak.kayit)
	}
	if len(uzakKodSade(uzak.kayit.Kod)) != 12 {
		uzak.kayit.Kod = ""
		uzak.kayit.Acik = false
	}
}

// uzak.mu kilitliyken çağrılır
func uzakKaydet() error {
	b, err := json.Marshal(uzak.kayit)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(anahtarKlasoru(), 0700); err != nil {
		return err
	}
	gecici := uzakYolu() + ".yeni"
	if err := os.WriteFile(gecici, b, 0600); err != nil {
		return err
	}
	return os.Rename(gecici, uzakYolu())
}

// Program açılınca: kullanıcı daha önce açtıysa kapıyı aç
func uzakBaslat() {
	uzakYukle()
	uzak.mu.Lock()
	acik := uzak.kayit.Acik
	uzak.mu.Unlock()
	if acik {
		uzakKapiAc()
	}
}

func uzakKapiAc() {
	uzak.mu.Lock()
	defer uzak.mu.Unlock()
	if uzak.ln != nil {
		return
	}
	port := uzakPort
	if v, err := strconv.Atoi(os.Getenv("ALSAT_UZAK_PORT")); err == nil && v > 0 { // testler için
		port = v
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		uzak.hata = "Kapı açılamadı: " + err.Error()
		log.Println("Uzaktan alma:", uzak.hata)
		return
	}
	uzak.ln, uzak.hata = ln, ""
	log.Printf("Uzaktan alma açık (kapı %d)", port)
	srv := &http.Server{Handler: uzakSunucu(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Println("Uzaktan alma durdu:", err)
		}
	}()
}

func uzakKapiKapat() {
	uzak.mu.Lock()
	defer uzak.mu.Unlock()
	if uzak.ln != nil {
		_ = uzak.ln.Close()
		uzak.ln = nil
		log.Println("Uzaktan alma kapatıldı")
	}
}

func uzakSunucu() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/kripto-alsat/dene", uzakDeneIstegi)
	mux.HandleFunc("/kripto-alsat/strateji", uzakStratejiIstegi)
	return mux
}

func istemciAdresi(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

// Hatalı imza sayacı: 10 dk'da 10 hata → o adres 10 dk engelli
func uzakEngelliMi(ip string, simdi int64) bool {
	uzak.mu.Lock()
	defer uzak.mu.Unlock()
	l := uzak.hatalar[ip]
	k := l[:0]
	for _, t := range l {
		if simdi-t < uzakHataSuresi.Milliseconds() {
			k = append(k, t)
		}
	}
	uzak.hatalar[ip] = k
	return len(k) >= uzakHataSiniri
}

func uzakHataEkle(ip string, simdi int64) {
	uzak.mu.Lock()
	uzak.hatalar[ip] = append(uzak.hatalar[ip], simdi)
	uzak.mu.Unlock()
}

// İsteğin imzasını denetler; gövdeyi döner
func uzakDogrula(w http.ResponseWriter, r *http.Request) ([]byte, string, bool) {
	simdi := time.Now().UnixMilli()
	ip := istemciAdresi(r)
	if r.Method != http.MethodPost {
		jsonYaz(w, 405, map[string]string{"hata": "Sadece POST"})
		return nil, "", false
	}
	// Simülatör tarayıcı değildir: tarayıcıdan (bir web sitesinden) gelen istek hiç değerlendirilmez ve hatalı deneme
	// sayılmaz (yoksa bilgisayarda açık bir site, kodu bilmeden bilgisayarın adresini 10 dk engelletebilirdi)
	if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
		jsonYaz(w, 403, map[string]string{"hata": "Tarayıcıdan gönderilemez"})
		return nil, "", false
	}
	if uzakEngelliMi(ip, simdi) {
		jsonYaz(w, 429, map[string]string{"hata": "Çok sayıda hatalı deneme; 10 dakika sonra tekrar dene"})
		return nil, "", false
	}
	uzak.mu.Lock()
	acik, kod := uzak.kayit.Acik, uzak.kayit.Kod
	uzak.mu.Unlock()
	if !acik || kod == "" {
		jsonYaz(w, 403, map[string]string{"hata": "Al-Sat'ta \"Bilgisayardan al\" kapalı"})
		return nil, "", false
	}
	govde, err := io.ReadAll(io.LimitReader(r.Body, uzakEnFazla+1))
	if err != nil {
		jsonYaz(w, 400, map[string]string{"hata": "İstek okunamadı"})
		return nil, "", false
	}
	if len(govde) > uzakEnFazla {
		jsonYaz(w, 413, map[string]string{"hata": "Dosya çok büyük"})
		return nil, "", false
	}
	zaman := r.Header.Get("X-KAS-Zaman")
	dosya := r.Header.Get("X-KAS-Dosya")
	imza := strings.ToLower(strings.TrimSpace(r.Header.Get("X-KAS-Imza")))
	z, err := strconv.ParseInt(zaman, 10, 64)
	if err != nil {
		uzakHataEkle(ip, simdi)
		jsonYaz(w, 401, map[string]string{"hata": "İmza eksik"})
		return nil, "", false
	}
	if d := simdi - z; d > uzakSaatPayi.Milliseconds() || d < -uzakSaatPayi.Milliseconds() {
		jsonYaz(w, 401, map[string]string{"hata": fmt.Sprintf("İki cihazın saati %d dakika farklı; bilgisayarın ve Pi'nin saatini kontrol et", absInt64(d)/60000)})
		return nil, "", false
	}
	beklenen := uzakImza(kod, r.URL.Path, zaman, dosya, govde)
	if !hmac.Equal([]byte(beklenen), []byte(imza)) {
		uzakHataEkle(ip, simdi)
		log.Printf("Uzaktan alma: %s adresinden hatalı kodla istek", ip)
		jsonYaz(w, 401, map[string]string{"hata": "Eşleştirme kodu yanlış. Al-Sat'taki kodu simülatöre yeniden yaz."})
		return nil, "", false
	}
	uzak.mu.Lock()
	for k, t := range uzak.gorulen {
		if simdi-t > 2*uzakSaatPayi.Milliseconds() {
			delete(uzak.gorulen, k)
		}
	}
	if _, vardir := uzak.gorulen[imza]; vardir {
		uzak.mu.Unlock()
		jsonYaz(w, 409, map[string]string{"hata": "Bu istek zaten alındı"})
		return nil, "", false
	}
	uzak.gorulen[imza] = simdi
	uzak.mu.Unlock()
	return govde, ip, true
}

func absInt64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func uzakDeneIstegi(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := uzakDogrula(w, r); !ok {
		return
	}
	klasor, _ := klasorBul()
	ad, _ := os.Hostname()
	jsonYaz(w, 200, map[string]any{"tamam": true, "program": "kripto-alsat", "surum": surumAdi, "cihaz": ad,
		"klasorVar": klasorVarMi(klasor), "klasor": filepath.Base(klasor)})
}

func uzakStratejiIstegi(w http.ResponseWriter, r *http.Request) {
	govde, ip, ok := uzakDogrula(w, r)
	if !ok {
		return
	}
	ad := r.Header.Get("X-KAS-Dosya")
	sonuc := uzakSonGelen{Zaman: time.Now().UnixMilli(), Dosya: ad, Adres: ip}
	kaydet := func(kod int, basari bool, durum, mesaj string) {
		sonuc.Basari, sonuc.Durum, sonuc.Mesaj = basari, durum, mesaj
		uzak.mu.Lock()
		uzak.sonGelen = append(uzak.sonGelen, sonuc)
		if len(uzak.sonGelen) > 10 {
			uzak.sonGelen = uzak.sonGelen[len(uzak.sonGelen)-10:]
		}
		uzak.mu.Unlock()
		if basari {
			jsonYaz(w, kod, map[string]any{"tamam": true, "dosya": ad, "durum": durum, "mesaj": mesaj})
		} else {
			jsonYaz(w, kod, map[string]any{"hata": mesaj})
		}
	}
	if m := stratejiAdiKalip.FindStringSubmatch(ad); m == nil {
		kaydet(400, false, "", "Dosya adı geçersiz (COIN_SÜRE.json olmalı)")
		return
	}
	klasor, _ := klasorBul()
	if !klasorVarMi(klasor) {
		kaydet(409, false, "", "Al-Sat'ın strateji klasörü bulunamadı; Pi'de Al-Sat'ta klasörü seç")
		return
	}
	govde = bytes.TrimPrefix(govde, []byte("\xef\xbb\xbf"))
	var guzel bytes.Buffer
	if err := json.Indent(&guzel, govde, "", "  "); err != nil {
		kaydet(400, false, "", "Dosya bozuk (JSON okunamadı)")
		return
	}
	// önce gizli bir geçici dosyaya yaz, kontrollerden geçerse adını değiştir (yarım dosya görünmesin)
	gecici := filepath.Join(klasor, "."+strings.TrimSuffix(ad, ".json")+".gelen")
	if err := os.WriteFile(gecici, guzel.Bytes(), 0644); err != nil {
		kaydet(500, false, "", "Dosya yazılamadı: "+err.Error())
		return
	}
	ayarMu.Lock()
	omur := omurTablosu(ayarlariOku())
	ayarMu.Unlock()
	d := dosyaOku(gecici, omur, simdiF())
	if d.Durum != "gecerli" {
		_ = os.Remove(gecici)
		neden := d.Hata
		if d.Durum == "suresiDoldu" {
			neden = "Dosyanın ömrü dolmuş"
		}
		kaydet(400, false, "", "Dosya kabul edilmedi: "+neden)
		return
	}
	// (dosyaOku geçici dosyanın adına bakıp hep "ad uyuşmuyor" notu düşer; asıl ad burada karşılaştırılır)
	if m := stratejiAdiKalip.FindStringSubmatch(ad); m == nil || m[1] != d.Sembol || m[2] != d.Aralik {
		_ = os.Remove(gecici)
		kaydet(400, false, "", "Dosya adı içindeki coin ve mum süresiyle uyuşmuyor")
		return
	}
	hedef := filepath.Join(klasor, ad)
	vardi := false
	if _, err := os.Stat(hedef); err == nil {
		vardi = true
	}
	if err := os.Rename(gecici, hedef); err != nil {
		_ = os.Remove(gecici)
		kaydet(500, false, "", "Dosya yerine konamadı: "+err.Error())
		return
	}
	durum, mesaj := "yeni", "Dosya Al-Sat'ın listesine eklendi. Kumbara açmak için Al-Sat'ta Başlat'a bas."
	durumMu.Lock()
	calisan := false
	for _, k := range D.Kumbaralar {
		if k.aktif() && k.Dosya == ad {
			calisan = true
		}
	}
	if calisan {
		durum, mesaj = "calisanKumbara", "Bu dosyayla çalışan bir kumbara var: elde coin varsa o işlem bitince yeni ayarla, aynı parayla devam edecek."
	} else if vardi {
		durum, mesaj = "yenilendi", "Al-Sat'taki aynı adlı dosya yenilendi. Kumbara açmak için Al-Sat'ta Başlat'a bas."
	}
	genelOlay("bilgi", fmt.Sprintf("Bilgisayardan strateji geldi: %s (%s)", ad, ip))
	degisti()
	durumMu.Unlock()
	kaydet(200, true, durum, mesaj)
}

// ---------- ekran uç noktası: /api/uzak (sadece bu cihazdaki pencere; guvenlik ara katmanı) ----------
// GET: durum. POST {acik?: bool, yeniKod?: bool}: aç/kapat, yeni kod üret.
func uzakIstegi(w http.ResponseWriter, r *http.Request) {
	uzakYukle()
	if r.Method == http.MethodPost {
		var q struct {
			Oturum  string `json:"oturum"`
			Acik    *bool  `json:"acik"`
			YeniKod bool   `json:"yeniKod"`
		}
		if jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
			hataYaz(w, 400, "İstek geçersiz")
			return
		}
		uzak.mu.Lock()
		if q.YeniKod || (q.Acik != nil && *q.Acik && uzak.kayit.Kod == "") {
			uzak.kayit.Kod = uzakKodUret()
			uzak.gorulen = map[string]int64{}
		}
		if q.Acik != nil {
			uzak.kayit.Acik = *q.Acik
		}
		acik := uzak.kayit.Acik
		err := uzakKaydet()
		uzak.mu.Unlock()
		if err != nil {
			hataYaz(w, 500, "Ayar kaydedilemedi: "+err.Error())
			return
		}
		if acik {
			uzakKapiAc()
		} else {
			uzakKapiKapat()
		}
		if q.YeniKod {
			log.Println("Uzaktan alma: yeni eşleştirme kodu üretildi")
		}
	}
	uzak.mu.Lock()
	cevap := map[string]any{
		"acik": uzak.kayit.Acik, "kod": uzak.kayit.Kod, "dinliyor": uzak.ln != nil, "hata": uzak.hata,
		"port": uzakPort, "sonGelen": append([]uzakSonGelen(nil), uzak.sonGelen...),
	}
	uzak.mu.Unlock()
	ad, _ := os.Hostname()
	cevap["cihaz"] = ad
	cevap["adresler"] = yerelAdresler()
	jsonYaz(w, 200, cevap)
}

// Bu cihazın ev ağındaki adresleri (simülatöre yazılacak)
func yerelAdresler() []string {
	var out []string
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		adrs, _ := i.Addrs()
		for _, a := range adrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
				out = append(out, n.IP.String())
			}
		}
	}
	sort.Strings(out)
	return out
}
