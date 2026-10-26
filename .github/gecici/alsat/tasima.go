package main

// ESKİ SÜRÜMDEN TAŞIMA (0.8) — yeni sürüm kendi klasöründe boş başlar (her sürümün klasörü ayrı). Kullanıcı eski
// klasördeki "kayitlar" klasörünü ve ayarlar.json'u elle kopyalamayı unutursa çalışan kumbaralar (elde coin olanlar
// dahil) yeni sürümde görünmez. Bu yüzden: bu sürümde hiç kumbara yoksa ve yakındaki bir klasörde eski bir Al-Sat'ın
// kaydı varsa, sayfa bir kez sorar: "taşıyayım mı?". Evet denirse:
//   1. eski ayarlar.json ve kayitlar/ (durum.json, islemler.csv, arsiv/) bu sürümün yanına kopyalanır,
//   2. eski klasördeki durum.json'un adı durum.json.tasindi olur (eski sürüm yanlışlıkla yeniden açılırsa aynı
//      kumbaraları İKİNCİ KEZ çalıştırmasın: boş açılır), yanına TASINDI.txt konur,
//   3. kumbaralar bu sürümde kaldıkları yerden devam eder (programı yeniden açmışsın gibi).
// Eski sürüm açıksa taşınmaz (iki program aynı kumbarayı çalıştırmasın): aynı kapıyı kullandıkları için zaten ikisi
// birden açık olamaz; ayrıca eski durum.json son 1 dakikada yazıldıysa "önce eskisini kapat" denir.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type eskiKayit struct {
	Klasor  string `json:"klasor"`
	Ad      string `json:"ad"`
	Calisan int    `json:"calisan"` // çalışan kumbara
	Gercek  int    `json:"gercek"`  // gerçek parayla çalışan
	Coinde  int    `json:"coinde"`  // elinde coin olan
	Toplam  int    `json:"toplam"`
	Degisti int64  `json:"degisti"` // durum.json'un son yazıldığı an (ms)
}

// Aranacak yerler: bu klasörün kardeşleri ve ev klasörü, Masaüstü, İndirilenler (bir alt klasör derinliğinde)
var tasimaYerleri = func() []string {
	yerler := []string{filepath.Dir(exeKlasoru)}
	if ev, err := os.UserHomeDir(); err == nil && ev != "" {
		for _, ad := range []string{"", "Desktop", "Masaüstü", "Downloads", "İndirilenler"} {
			yerler = append(yerler, filepath.Join(ev, ad))
		}
	}
	return yerler
}

func eskiKayitlariBul() []eskiKayit {
	kendi, _ := filepath.Abs(exeKlasoru)
	gorulen := map[string]bool{kendi: true}
	var out []eskiKayit
	for _, yer := range tasimaYerleri() {
		girdiler, err := os.ReadDir(yer)
		if err != nil {
			continue
		}
		for i, g := range girdiler {
			if i > 300 {
				break
			}
			if !g.IsDir() {
				continue
			}
			k, _ := filepath.Abs(filepath.Join(yer, g.Name()))
			if gorulen[k] {
				continue
			}
			gorulen[k] = true
			yol := filepath.Join(k, "kayitlar", "durum.json")
			st, err := os.Stat(yol)
			if err != nil || st.Size() > 50<<20 {
				continue
			}
			b, err := os.ReadFile(yol)
			if err != nil {
				continue
			}
			var d struct {
				Kumbaralar []struct {
					Durum string  `json:"durum"`
					Mod   string  `json:"mod"`
					Adet  float64 `json:"adet"`
				} `json:"kumbaralar"`
			}
			if json.Unmarshal(b, &d) != nil {
				continue
			}
			e := eskiKayit{Klasor: k, Ad: g.Name(), Toplam: len(d.Kumbaralar), Degisti: st.ModTime().UnixMilli()}
			for _, x := range d.Kumbaralar {
				if x.Durum != "bitti" {
					e.Calisan++
					if x.Mod == "gercek" {
						e.Gercek++
					}
					if x.Adet > 0 {
						e.Coinde++
					}
				}
			}
			if e.Toplam > 0 {
				out = append(out, e)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Degisti > out[j].Degisti })
	return out
}

// durumMu kilitliyken: bu sürümde taşıma sorulabilir mi?
func tasimaUygun() bool {
	return durumHatasi == "" && len(D.Kumbaralar) == 0
}

var tasimaReddi bool // "şimdi değil" dendi (bu açılışta bir daha sorulmaz)

// GET: {teklif: eskiKayit|null}. POST {oturum, klasor}: taşır. POST {oturum, vazgec:true}: bu açılışta sorma.
func tasimaIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		durumMu.Lock()
		uygun := tasimaUygun() && !tasimaReddi
		durumMu.Unlock()
		var teklif *eskiKayit
		if uygun {
			if l := eskiKayitlariBul(); len(l) > 0 {
				teklif = &l[0]
			}
		}
		jsonYaz(w, 200, map[string]any{"teklif": teklif})
		return
	}
	var q struct {
		Oturum string `json:"oturum"`
		Klasor string `json:"klasor"`
		Vazgec bool   `json:"vazgec"`
	}
	if r.Method != http.MethodPost || jsonOku(r, &q) != nil || !oturumGecerli(q.Oturum) {
		hataYaz(w, 400, "İstek geçersiz")
		return
	}
	if q.Vazgec {
		durumMu.Lock()
		tasimaReddi = true
		durumMu.Unlock()
		jsonYaz(w, 200, map[string]bool{"tamam": true})
		return
	}
	// sadece bulunan klasörlerden biri taşınabilir (sayfa başka bir yol gönderemez)
	var secilen *eskiKayit
	for _, e := range eskiKayitlariBul() {
		if e.Klasor == q.Klasor {
			x := e
			secilen = &x
		}
	}
	if secilen == nil {
		hataYaz(w, 404, "Eski kayıt bulunamadı")
		return
	}
	if gecen := simdiF().UnixMilli() - secilen.Degisti; gecen >= 0 && gecen < 60_000 {
		hataYaz(w, 409, "Eski Al-Sat ("+secilen.Ad+") hâlâ açık görünüyor: kaydı az önce yazıldı. Onu kapat, 1 dakika bekle, sonra yeniden dene.")
		return
	}
	durumMu.Lock()
	defer durumMu.Unlock()
	if !tasimaUygun() {
		hataYaz(w, 409, "Bu sürümde artık kumbara var; taşıma yapılmadı")
		return
	}
	if err := kayitlariTasi(secilen.Klasor); err != nil {
		log.Println("Eski kayıtlar taşınamadı:", err)
		hataYaz(w, 500, "Taşınamadı: "+err.Error())
		return
	}
	korumaGenelYukle()
	durumYukleIc()
	genelOlay("bilgi", fmt.Sprintf("Eski sürümün kayıtları taşındı (%s): %d kumbara, %d çalışıyor. Eski klasördeki kayıt \"taşındı\" olarak işaretlendi.", secilen.Ad, secilen.Toplam, secilen.Calisan))
	degisti()
	log.Printf("Eski kayıtlar taşındı: %s (%d kumbara)", secilen.Klasor, secilen.Toplam)
	jsonYaz(w, 200, map[string]any{"tamam": true, "kumbara": len(D.Kumbaralar)})
}

// durumMu kilitliyken çağrılır
func kayitlariTasi(eski string) error {
	eskiKayitlar := filepath.Join(eski, "kayitlar")
	if _, err := os.Stat(filepath.Join(eskiKayitlar, "durum.json")); err != nil {
		return errors.New("eski durum.json okunamadı")
	}
	if err := os.MkdirAll(kayitKlasoru(), 0755); err != nil {
		return err
	}
	// 1) kayitlar/ (durum.json en son: yarıda kalırsa bu sürüm yine boş açılır, eski kayıt yerinde durur)
	if err := klasorKopyala(eskiKayitlar, kayitKlasoru(), map[string]bool{"durum.json": true, "durum.json.yeni": true, "durum.json.onceki": true}); err != nil {
		return err
	}
	// ayarlar: eski sürümdeki seçimler (klasör, görünüm, koruma, ömür) bu sürümdekilerin üstüne yazılır
	if b, err := os.ReadFile(filepath.Join(eski, "ayarlar.json")); err == nil {
		var em map[string]json.RawMessage
		if json.Unmarshal(b, &em) == nil && em != nil {
			ayarMu.Lock()
			m := ayarlariOku()
			for k, v := range em {
				if k == "klasor" && !klasorVarMi(ayarMetni(em, "klasor")) {
					continue // eski sürümün seçtiği klasör artık yok: bu sürümdeki kalsın
				}
				m[k] = v
			}
			err := ayarlariYaz(m)
			ayarMu.Unlock()
			if err != nil {
				return err
			}
		}
	}
	b, err := os.ReadFile(filepath.Join(eskiKayitlar, "durum.json"))
	if err != nil {
		return err
	}
	if err := diskeYaz(filepath.Join(kayitKlasoru(), "durum.json"), b); err != nil {
		return err
	}
	// 2) eski kaydı işaretle: eski sürüm yeniden açılırsa boş açılsın
	damga := time.Now().Format("20060102-150405")
	for _, ad := range []string{"durum.json", "durum.json.yeni", "durum.json.onceki"} {
		y := filepath.Join(eskiKayitlar, ad)
		if _, err := os.Stat(y); err == nil {
			if err := os.Rename(y, y+".tasindi-"+damga); err != nil && ad == "durum.json" {
				// eski kayıt işaretlenemedi: yeni kopyayı geri al (iki sürüm aynı kumbarayı çalıştırmasın)
				_ = os.Remove(filepath.Join(kayitKlasoru(), "durum.json"))
				return fmt.Errorf("eski klasördeki kayıt işaretlenemedi (%v); taşıma yapılmadı", err)
			}
		}
	}
	not := fmt.Sprintf("Bu klasördeki kumbaraların kaydı %s tarihinde Kripto Al-Sat %s'e taşındı:\n%s\n\n"+
		"Bu eski sürümü açarsan kumbarasız (boş) açılır; kumbaralar yeni sürümde devam ediyor.\n"+
		"Eski kayıt durum.json.tasindi-%s adıyla burada duruyor.\n", time.Now().Format("02.01.2006 15:04"), surumAdi, exeKlasoru, damga)
	_ = os.WriteFile(filepath.Join(eskiKayitlar, "TASINDI.txt"), []byte(not), 0644)
	return nil
}

// Klasörü kopyalar (alt klasörler dahil); hedefte aynı adlı dosya varsa üstüne yazmaz. atla: kökteki bu adlar kopyalanmaz.
func klasorKopyala(kaynak, hedef string, atla map[string]bool) error {
	return filepath.WalkDir(kaynak, func(yol string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		goreli, _ := filepath.Rel(kaynak, yol)
		if goreli == "." {
			return nil
		}
		if atla[goreli] || strings.Contains(goreli, ".tasindi-") {
			return nil
		}
		h := filepath.Join(hedef, goreli)
		if d.IsDir() {
			return os.MkdirAll(h, 0755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if _, err := os.Stat(h); err == nil {
			return nil
		}
		k, err := os.Open(yol)
		if err != nil {
			return err
		}
		defer k.Close()
		f, err := os.OpenFile(h, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, k); err != nil {
			f.Close()
			return err
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	})
}
