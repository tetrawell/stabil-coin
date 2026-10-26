// Kripto Al-Sat — Kripto Simülatör'ün dışa aktardığı stratejilerle MEXC spot'ta alıp satar.
// Çift tıklayınca: bilgisayarında küçük bir sunucu açar (sadece bu bilgisayardan erişilir),
// Chrome/Edge'i kendi uygulama penceresi olarak açar, pencere kapatılınca kendini kapatır.
//
// Sürüm 0.3: bekçi sabırlı. Kullanıcı pencereyi kapatırsa (ya da çalışan kumbara yoksa) program eskisi gibi kapanır.
// Kumbara çalışırken pencere kendiliğinden susarsa (donma, çökme) program açık kalır ve pencereyi yeniden açar.
// Ömür tablosu uzadı (1m 1 gün … 1d 1 ay).
//
// Sürüm 0.4 (sadece Raspberry Pi): pencere ekran kartısız çizilir; bekçi pencereyi yeniden açarken Al-Sat'ın
// kendi tarayıcısını kapatıp temiz açar. Bu kapatma "kullanıcı kapattı" sayılmaz. Windows sürümü 0.3'te kaldı.
//
// Sürüm 0.5 (Raspberry Pi): pencerenin saatte bir bozulmasının asıl sebebi bulundu ve düzeltildi: sayfa karar
// bildiriminin cevabını okumuyordu, Chromium her okunmayan cevap için bir tutamak açık bırakıyordu (bkz. uygulama.js).
//
// Sürüm 0.6 (Raspberry Pi): pencere yeniden ekran kartıyla çiziliyor (0.4'teki --disable-gpu kaldırıldı). Başka değişiklik yok.
//
// Sürüm 0.7 (Raspberry Pi; Windows'ta 0.4): KORUMA. Her kumbara, gidişatını ayarın kendi geçmişiyle karşılaştırır;
// "normal şanssızlık" sayılamayacak kadar kötü giderse kenara alınır (yeni alım yok), motor gölgede izlenir, gölgede
// art arda kârlı işlemler gelince oyuna döner. Başlat penceresinde ayrıca "sağlamlık testi". Ayrıntı: koruma.go.
//
// Sürüm 0.8 (Raspberry Pi): bilgisayardaki simülatörden ev ağı üzerinden strateji dosyası alma (uzak.go; varsayılan
// kapalı, eşleştirme koduyla imzalı). Mum sınırı 30.000 (simülatör 3.4 en fazla 20.000 mumla ayar bulabiliyor).
// Ayrıntı: CLAUDE.md → Sürümler.
//
// Sürüm 0.2: her strateji dosyası için ayrı "kumbara": sanal parayla (programın içindeki sanal cüzdan) ya da
// gerçek parayla (MEXC API anahtarı, sadece al-sat izni) alıp satar. Kararları simülatörün motoru verir (sayfada),
// emirleri bu program yürütür (limit → 15 sn → piyasa). Süresi dolan strateji dosyaları klasörden silinir.
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed web
var webDosyalari embed.FS

const (
	sabitPort = 47322 // Kripto Simülatör 47321'i kullanır; iki program aynı anda açık olabilir
	surumAdi  = "0.9"
)

var (
	exeKlasoru  string
	veriKlasoru string // ayarlar.json ve kayıt dosyası burada (normalde exe'nin yanı)
	testModu    = os.Getenv("ALSAT_TEST") == "1"
	baglantilar int64
	sonBaglanti atomic.Int64
	hicBaglandi atomic.Bool
	aktifOturum atomic.Value // string: en son açılan pencerenin kimliği; karar ve emir sadece ondan kabul edilir

	// Bekçi için: pencerenin kullanıcı tarafından mı kapatıldığı, yoksa kendiliğinden mi sustuğu
	kapananOturumlar sync.Map     // oturum kimliği → sayfanın "kapanıyorum" haberi (kapatma ya da yenileme)
	sonKopanOturum   atomic.Value // string: hattı en son kopan pencerenin oturumu
	tarayiciCanli    atomic.Int32 // açtığımız ve hâlâ çalışan tarayıcı süreçleri
	tarayiciKapandi  atomic.Bool  // açtığımız tarayıcı normal şekilde kapandı (kullanıcı pencereyi kapattı)
	bilerekKapatma   atomic.Int64 // 0.4: bekçinin tarayıcıyı yeniden başlatmak için kapattığı an (unix ns)
	yoksayilanOturum atomic.Value // 0.4: bekçi yeniden başlatırken kapanan pencerenin oturumu ("kapatıldı" sayılmaz)
	yenidenAcSuresi  = ortamSaniye("ALSAT_YENIDEN_AC_SN", 120*time.Second)
)

const (
	sessizlikSiniri  = 20 * time.Second // pencereden bu kadar haber gelmezse "sustu" sayılır
	yenidenAcAraligi = 10 * time.Minute // pencereyi yeniden açma denemeleri arası
)

func main() {
	exe, err := os.Executable()
	if err == nil {
		exeKlasoru = filepath.Dir(exe)
	} else {
		exeKlasoru = "."
	}
	veriKlasoru = ortamVeya("ALSAT_VERI_KLASORU", exeKlasoru)
	kayitAc()
	aktifOturum.Store("")
	sonKopanOturum.Store("")
	yoksayilanOturum.Store("")
	if testModu {
		piyasa = sahtePiyasa{}
		sanal = yeniSanalBorsa(piyasa)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", sabitPort))
	if err != nil {
		// Zaten açık mı? Öyleyse sadece pencereyi aç (eski pencere kendini durdurur).
		// 0.8: başka bir kapıda İKİNCİ bir Al-Sat açılmaz. 0.7'de açık olan program 2 sn içinde cevap vermezse
		// (ör. Pi meşgulken) rastgele bir kapıda ikinci bir program açılıyordu: aynı kumbaralar iki programda birden
		// alıp satabilirdi. Kapı tek kumbara sahibidir: kapıyı tutan program çalışır, diğeri kapanır.
		adres := fmt.Sprintf("http://127.0.0.1:%d/", sabitPort)
		for deneme := 0; deneme < 6; deneme++ {
			r, e := (&http.Client{Timeout: 3 * time.Second}).Get(adres + "api/ping")
			if e != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			b, _ := io.ReadAll(io.LimitReader(r.Body, 200))
			r.Body.Close()
			cevap := strings.TrimSpace(string(b))
			if cevap == "kripto-alsat "+surumAdi {
				log.Println("Program zaten açık; penceresi açılıyor")
				pencereAc(adres)
				return
			}
			if strings.HasPrefix(cevap, "kripto-alsat") {
				eski := strings.TrimSpace(strings.TrimPrefix(cevap, "kripto-alsat"))
				if eski == "" {
					eski = "eski sürüm"
				}
				mesaj := fmt.Sprintf("Kripto Al-Sat'ın başka bir sürümü (%s) açık. İki sürüm aynı anda çalışamaz.\n\n"+
					"Önce açık olanın penceresini kapat (kumbaraları çalışıyorsa önce OKU.txt'deki \"YENİ SÜRÜME GEÇERKEN\" "+
					"adımlarına bak), sonra %s'i yeniden aç.", eski, surumAdi)
				log.Println(strings.ReplaceAll(mesaj, "\n", " "))
				if !acilisHatasi(mesaj) {
					pencereAc(adres) // uyarı gösterilemediyse açık olanın penceresi gelsin (hiç tepki yok olmasın)
				}
				os.Exit(1)
			}
			break // kapıda başka bir program var
		}
		mesaj := fmt.Sprintf("Kripto Al-Sat açılamadı: %d numaralı kapı kullanımda ve cevap vermiyor.\n\n"+
			"Açık bir Kripto Al-Sat varsa birkaç dakika bekleyip yeniden dene. Olmazsa bilgisayarı yeniden başlat.", sabitPort)
		log.Println(strings.ReplaceAll(mesaj, "\n", " "), "(", err, ")")
		acilisHatasi(mesaj)
		os.Exit(1)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	adres := fmt.Sprintf("http://127.0.0.1:%d/", port)
	log.Printf("Başladı (sürüm %s): %s", surumAdi, adres)

	// Kumbaralar sadece asıl programda çalışır (ikinci kez açılan program yukarıda pencereyi açıp çıktı)
	korumaGenelYukle()
	akisAyarYukle() // 0.9: akış onayının ayarları
	durumYukle()
	akisBaslat()            // 0.9: canlı işlem/defter/açık pozisyon toplama (MEXC websocket)
	go akisBeklemeDongusu() // 0.9: onay bekleyen alımlar
	go izleyiciDongusu()
	go klasorIzleyici()
	go gercekDongusu()
	go borsaSaatiDongusu()
	uzakBaslat() // 0.8: bilgisayardan strateji alma (kullanıcı açtıysa)

	alt, _ := fs.Sub(webDosyalari, "web")
	mux := http.NewServeMux()
	dosyalar := http.FileServer(http.FS(alt))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		dosyalar.ServeHTTP(w, r)
	})
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "kripto-alsat "+surumAdi) }) // 0.8: sürüm de
	mux.HandleFunc("/api/mumlar", mumlarIstegi)
	mux.HandleFunc("/api/stratejiler", stratejilerIstegi)
	mux.HandleFunc("/api/ayarlar", ayarlarIstegi)
	mux.HandleFunc("/api/klasor-sec", klasorSecIstegi)
	mux.HandleFunc("/api/oturum", oturumIstegi)
	mux.HandleFunc("/api/kapaniyor", kapaniyorIstegi)
	mux.HandleFunc("/api/baglanti", baglantiIstegi)
	mux.HandleFunc("/api/durum", durumIstegi)
	mux.HandleFunc("/api/kumbara/baslat", kumbaraBaslatIstegi)
	mux.HandleFunc("/api/kumbara/ayrinti", kumbaraAyrintiIstegi)
	mux.HandleFunc("/api/kumbara/capa", kumbaraCapaIstegi)
	mux.HandleFunc("/api/kumbara/karar", kumbaraKararIstegi)
	mux.HandleFunc("/api/kumbara/durdur", kumbaraDurdurIstegi)
	mux.HandleFunc("/api/koruma/hesapla", korumaHesaplaIstegi)
	mux.HandleFunc("/api/kumbara/koruma", kumbaraKorumaIstegi)
	mux.HandleFunc("/api/kumbara/koruma-ayar", kumbaraKorumaAyarIstegi)
	mux.HandleFunc("/api/kumbara/koruma-don", kumbaraKorumaDonIstegi)
	mux.HandleFunc("/api/hepsini-durdur", hepsiniDurdurIstegi)
	mux.HandleFunc("/api/cuzdan/sifirla", sanalSifirlaIstegi)
	mux.HandleFunc("/api/anahtar", anahtarIstegi)
	mux.HandleFunc("/api/anahtar/sil", anahtarSilIstegi)
	mux.HandleFunc("/api/gercek/yenile", gercekYenileIstegi)
	mux.HandleFunc("/api/kural", kuralIstegi)
	mux.HandleFunc("/api/uzak", uzakIstegi)
	mux.HandleFunc("/api/tasima", tasimaIstegi)          // 0.8: eski sürümün kayıtlarını taşıma
	mux.HandleFunc("/api/akis/grafik", akisGrafikIstegi) // 0.9: akış onayı (akis_*.go)
	mux.HandleFunc("/api/akis/canli", akisCanliIstegi)
	mux.HandleFunc("/api/akis/footprint", akisFootprintIstegi)
	mux.HandleFunc("/api/akis/ayar", akisAyarIstegi)
	mux.HandleFunc("/api/akis/karne", akisKarneIstegi)
	mux.HandleFunc("/api/kumbara/akis", kumbaraAkisIstegi)

	if os.Getenv("ALSAT_PENCERE_YOK") != "1" {
		go pencereAc(adres)
	}
	if !testModu || os.Getenv("ALSAT_BEKCI") == "1" {
		go bekci(adres)
	}
	log.Fatal(http.Serve(ln, guvenlik(mux)))
}

// Sadece bu bilgisayardaki program penceresinden gelen isteklere izin verilir:
// başka bir web sitesi (kullanıcının tarayıcısında açık) programa istek gönderemesin, DNS hilesiyle de olmasın.
func guvenlik(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if !strings.HasPrefix(host, "127.0.0.1:") && !strings.HasPrefix(host, "localhost:") && host != "127.0.0.1" && host != "localhost" {
			http.Error(w, "yasak", http.StatusForbidden)
			return
		}
		// 0.8: başka bir site bu sayfayı kendi içine (görünmez çerçeve) alıp kullanıcıya fark ettirmeden tıklatamasın
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		sf := r.Header.Get("Sec-Fetch-Site")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+host {
				http.Error(w, "yasak", http.StatusForbidden)
				return
			}
			if sf != "" && sf != "same-origin" && sf != "none" {
				http.Error(w, "yasak", http.StatusForbidden)
				return
			}
		} else if strings.HasPrefix(r.URL.Path, "/api/") && sf != "" && sf != "same-origin" && sf != "none" {
			// 0.8: başka siteden gelen okuma isteği de yok (ör. canlı hattı açık tutmak, boşuna veri indirtmek)
			http.Error(w, "yasak", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Bir coinin MEXC kuralları (en az emir tutarı, komisyon); "başlat" penceresi için
func kuralIstegi(w http.ResponseWriter, r *http.Request) {
	sembol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sembol")))
	if !sembolKalip.MatchString(sembol) {
		hataYaz(w, 400, "Coin adı geçersiz")
		return
	}
	k, err := piyasa.Kurallar(sembol)
	if err != nil {
		hataYaz(w, 502, "MEXC'den coin kuralları alınamadı: "+err.Error())
		return
	}
	jsonYaz(w, 200, map[string]any{"kural": k, "enAz": enAzKumbara(k)})
}

// Kayıt dosyası: exe'nin yanında alsat_kayit.txt. Eskileri silinmez; 5 MB'ı geçince bir öncekine taşınır.
func kayitAc() {
	yol := filepath.Join(veriKlasoru, "alsat_kayit.txt")
	if st, err := os.Stat(yol); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(yol, filepath.Join(veriKlasoru, "alsat_kayit_eski.txt"))
	}
	if lf, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		log.SetOutput(lf)
	}
}

func ortamVeya(ad, varsayilan string) string {
	if v := os.Getenv(ad); v != "" {
		return v
	}
	return varsayilan
}

func ortamSaniye(ad string, varsayilan time.Duration) time.Duration {
	var sn float64
	if _, err := fmt.Sscan(os.Getenv(ad), &sn); err == nil && sn > 0 {
		return time.Duration(sn * float64(time.Second))
	}
	return varsayilan
}

// ---------- bekçi: pencere kapanınca programı kapatır ----------
//
// 0.2'de pencereden 20 sn haber gelmeyince program kapanıyordu. Raspberry Pi'de pencere bir an susunca
// (pencere aslında açıkken) program kapandı ve kumbaralar durdu. 0.3'te:
//   - kullanıcı pencereyi kapattıysa (sayfa "kapanıyorum" dedi ya da tarayıcı normal kapandı) → eskisi gibi kapan;
//   - çalışan kumbara yoksa → eskisi gibi kapan;
//   - kumbara çalışıyor ve pencere kendiliğinden sustuysa → kapanma; zarar durdur, hedef ve ömür izlenmeye devam
//     eder; 2 dk'da pencere geri gelmezse pencereyi yeniden aç (10 dk'da bir tekrar dener).

type bekciGozlem struct {
	Baglanti     int64         // açık canlı hat sayısı
	HicBaglandi  bool          // program açılalı hiç pencere bağlandı mı
	AcikGecen    time.Duration // program açılalı geçen süre
	Sessiz       time.Duration // son hat kopalı geçen süre
	Kapatildi    bool          // hattı kopan pencere "kapanıyorum" dedi
	TarayiciYok  bool          // açtığımız tarayıcı normal şekilde kapandı
	KumbaraVar   bool          // çalışan kumbara var
	SonAcmaGecen time.Duration // son yeniden açmadan beri (hiç açılmadıysa çok büyük)
}

const (
	bekciDevam = iota
	bekciCik
	bekciBekle
	bekciYenidenAc
)

func bekciKarari(g bekciGozlem) (int, string) {
	if g.Baglanti > 0 {
		return bekciDevam, ""
	}
	if !g.HicBaglandi {
		if g.KumbaraVar {
			// pencere hiç açılamadı ama kumbara var: kapanma, pencereyi açmayı yeniden dene
			if g.AcikGecen >= yenidenAcSuresi && g.SonAcmaGecen >= yenidenAcAraligi {
				return bekciYenidenAc, ""
			}
			return bekciBekle, ""
		}
		if g.AcikGecen > 3*time.Minute {
			return bekciCik, "pencere hiç açılmadı"
		}
		return bekciDevam, ""
	}
	if g.Kapatildi || g.TarayiciYok || !g.KumbaraVar {
		if g.Sessiz > sessizlikSiniri {
			switch {
			case g.Kapatildi:
				return bekciCik, "pencere kapatıldı"
			case g.TarayiciYok:
				return bekciCik, "tarayıcı kapatıldı"
			default:
				return bekciCik, "pencere sustu, çalışan kumbara yok"
			}
		}
		return bekciDevam, ""
	}
	if g.Sessiz <= sessizlikSiniri {
		return bekciDevam, ""
	}
	if g.Sessiz >= yenidenAcSuresi && g.SonAcmaGecen >= yenidenAcAraligi {
		return bekciYenidenAc, ""
	}
	return bekciBekle, ""
}

func kullaniciKapatti() bool {
	id, _ := sonKopanOturum.Load().(string)
	if id == "" {
		return false
	}
	// bekçi o pencerenin tarayıcısını kendisi kapattıysa, kapanırken gelen haber "kullanıcı kapattı" sayılmaz
	if y, _ := yoksayilanOturum.Load().(string); y == id {
		return false
	}
	_, ok := kapananOturumlar.Load(id)
	return ok
}

func sureMetni(d time.Duration) string {
	sn := int(d.Round(time.Second) / time.Second)
	switch {
	case sn < 120:
		return fmt.Sprintf("%d sn", sn)
	case sn < 7200:
		return fmt.Sprintf("%d dk", sn/60)
	default:
		return fmt.Sprintf("%d sa %d dk", sn/3600, sn%3600/60)
	}
}

func bekci(adres string) {
	baslangic := time.Now()
	sonAcma := time.Time{}
	beklemede := false
	for range time.Tick(3 * time.Second) {
		durumMu.Lock()
		kumbaraVar := aktifKumbaraVar()
		durumMu.Unlock()
		g := bekciGozlem{
			Baglanti:     atomic.LoadInt64(&baglantilar),
			HicBaglandi:  hicBaglandi.Load(),
			AcikGecen:    time.Since(baslangic),
			Sessiz:       time.Since(time.Unix(0, sonBaglanti.Load())),
			Kapatildi:    kullaniciKapatti(),
			TarayiciYok:  tarayiciKapandi.Load() && tarayiciCanli.Load() == 0,
			KumbaraVar:   kumbaraVar,
			SonAcmaGecen: time.Since(sonAcma),
		}
		if sonAcma.IsZero() {
			g.SonAcmaGecen = 1000 * time.Hour
		}
		karar, sebep := bekciKarari(g)
		switch karar {
		case bekciDevam:
			if g.Baglanti > 0 && beklemede {
				log.Println("Pencere geri geldi; her şey kaldığı yerden devam ediyor.")
				beklemede = false
				sonAcma = time.Time{} // bir sonraki susmada yine 2 dk sonra açılabilsin
			}
		case bekciCik:
			if sebep == "pencere hiç açılmadı" {
				log.Println("Pencere hiç açılmadı, çıkılıyor.")
			} else {
				log.Printf("Pencere kapandı, çıkılıyor (%s).", sebep)
			}
			os.Exit(0)
		case bekciBekle, bekciYenidenAc:
			if !beklemede {
				beklemede = true
				if g.HicBaglandi {
					log.Println("Pencereden haber gelmiyor ama pencere kapatılmadı; kumbara çalıştığı için program açık kalıyor, pencere bekleniyor.")
				} else {
					log.Println("Pencere açılamadı; kumbara çalıştığı için program açık kalıyor.")
				}
			}
			if karar == bekciYenidenAc {
				sonAcma = time.Now()
				if g.HicBaglandi {
					log.Printf("Pencere %s'dir sessiz; yeniden açılıyor.", sureMetni(g.Sessiz))
				} else {
					log.Println("Pencere yeniden açılmaya çalışılıyor.")
				}
				if os.Getenv("ALSAT_PENCERE_YOK") == "1" {
					log.Println("(test: pencere açma kapalı)")
				} else {
					id, _ := sonKopanOturum.Load().(string)
					yoksayilanOturum.Store(id)
					go pencereyiYenidenAc(adres)
				}
			}
		}
	}
}

// Açtığımız tarayıcı sürecini izler. Kullanıcı pencereyi kapatınca tarayıcı normal şekilde (hatasız) biter.
// Birkaç saniyede biten süreç, zaten açık olan tarayıcıya işi devredip çıkmıştır; sayılmaz.
// Hatayla biten (çöken) tarayıcı "kapatıldı" sayılmaz: bekçi pencereyi yeniden açar.
func tarayiciBaslat(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	tarayiciCanli.Add(1)
	tarayiciKapandi.Store(false)
	bas := time.Now()
	go func() {
		err := cmd.Wait()
		tarayiciCanli.Add(-1)
		if time.Since(bas) < 10*time.Second {
			return
		}
		if time.Since(time.Unix(0, bilerekKapatma.Load())) < 30*time.Second {
			log.Println("Tarayıcı, temiz pencere açmak için kapatıldı.")
			return
		}
		if err == nil {
			log.Println("Tarayıcı penceresi kapatıldı.")
			tarayiciKapandi.Store(true)
		} else {
			log.Println("Tarayıcı beklenmedik şekilde kapandı:", err)
		}
	}()
	return nil
}

// Sayfa kapanırken (kullanıcı pencereyi kapatınca ya da sayfayı yenileyince) haber verir.
// Bekçi buna bakarak "pencere kapatıldı" ile "pencere kendiliğinden sustu"yu ayırır.
func kapaniyorIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		hataYaz(w, 405, "Sadece POST")
		return
	}
	if id := r.URL.Query().Get("oturum"); id != "" && len(id) <= 64 {
		kapananOturumlar.Store(id, time.Now().UnixNano())
		log.Println("Pencere kapatılıyor ya da yenileniyor (oturum " + kisaKimlik(id) + ")")
	}
	w.WriteHeader(http.StatusNoContent)
}

func kisaKimlik(id string) string {
	if len(id) > 6 {
		return id[:6]
	}
	return id
}

// Her açılan pencere bir kimlik alır. En son açılan pencere "aktif" olur; eskisi kendini durdurur.
// Böylece program iki kez açılsa bile kararları tek pencere verir (2. adımdan sonra çift emir olmasın diye).
func oturumIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		hataYaz(w, 405, "Sadece POST")
		return
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	kimlik := hex.EncodeToString(b)
	aktifOturum.Store(kimlik)
	log.Println("Yeni pencere oturumu:", kimlik)
	jsonYaz(w, 200, map[string]any{"oturum": kimlik, "surum": surumAdi, "test": testModu})
}

// Pencere ile program arasındaki canlı hat (SSE). Her 2 saniyede bir "tik" gönderir:
// pencere küçültülse bile sayfa bu olaylarla uyanık kalır, mum kapanışını kaçırmaz.
// Hat koparsa bekçi karar verir (pencere kapatıldıysa programı kapatır, sustuysa bekler).
func baglantiIstegi(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "akış yok", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	kimlik := r.URL.Query().Get("oturum")
	atomic.AddInt64(&baglantilar, 1)
	hicBaglandi.Store(true)
	acilis := time.Now()
	log.Println("Pencere hattı açıldı (oturum " + kisaKimlik(kimlik) + ")")
	defer func() {
		sonKopanOturum.Store(kimlik)
		sonBaglanti.Store(time.Now().UnixNano())
		atomic.AddInt64(&baglantilar, -1)
		log.Printf("Pencere hattı kapandı (oturum %s, %s açık kaldı)", kisaKimlik(kimlik), sureMetni(time.Since(acilis)))
	}()
	fmt.Fprint(w, ": merhaba\n\n")
	fl.Flush()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-t.C:
			aktif := kimlik != "" && kimlik == aktifOturum.Load().(string)
			fmt.Fprintf(w, "data: {\"tip\":\"tik\",\"aktif\":%v,\"simdi\":%d,\"ds\":%d,\"bf\":%d}\n\n", aktif, time.Now().UnixMilli(), durumSurum.Load(), borsaFarki.Load())
			fl.Flush()
		}
	}
}

// ---------- JSON yardımcıları ----------

func jsonYaz(w http.ResponseWriter, kod int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(kod)
	_ = json.NewEncoder(w).Encode(v)
}

func hataYaz(w http.ResponseWriter, kod int, mesaj string) {
	log.Println("Hata:", mesaj)
	jsonYaz(w, kod, map[string]string{"hata": mesaj})
}
