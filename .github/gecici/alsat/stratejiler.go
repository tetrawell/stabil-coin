package main

// STRATEJİLER — Kripto Simülatör'ün "Dışa aktar" ile yazdığı dosyaları okur.
// Dosya biçimi simülatörün kaynak/STRATEJI_FORMATI.md dosyasında anlatılır.
// Burada sadece temel kontroller yapılır (dosya sağlam mı, MEXC verisiyle mi, ömrü ne zaman bitiyor).
// Motorla ilgili kontroller (strateji sürümü, bilinmeyen kural) sayfada, motorun kendi listesiyle yapılır.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// Ömür tablosu (dakika). Kullanıcının 0.3'te istediği süreler (0.2'de "yaklaşık 100 mum" kuralıydı).
// Süre, dosyanın simülatörde dışa aktarıldığı andan ("olusturuldu") başlar. Ayarlardan değiştirilebilir.
var varsayilanOmur = map[string]int{
	"1m":  1440,  // 1 gün
	"5m":  4320,  // 3 gün
	"15m": 10080, // 1 hafta
	"1h":  20160, // 2 hafta
	"4h":  30240, // 3 hafta
	"1d":  43200, // 1 ay
}

var stratejiAdiKalip = regexp.MustCompile(`^([A-Z0-9]{2,30})_(1m|5m|15m|1h|4h|1d)\.json$`)

// 0.8: dosyadaki değerlerin türü denetlenir. Simülatörün yazdığı dosyada strateji sadece sayı, doğru/yanlış ve kısa
// seçim adları (ör. VWAP "gun") içerir; geçmiş sonuç sayı ve tarih. Başka bir şey (ör. sayfada çalışabilecek bir
// yazı) içeren dosya çalıştırılmaz: dosya artık ağdan da gelebiliyor.
var (
	secimKalip = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,23}$`)
	tarihKalip = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9:.]{5,18}(Z|[+-][0-9:]{4,5})$`)
)

func degerDenetle(ham json.RawMessage, metin *regexp.Regexp) error {
	if len(ham) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(ham))
	dec.UseNumber()
	derinlik, adet := 0, 0
	for {
		tk, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("okunamadı")
		}
		if adet++; adet > 20000 {
			return errors.New("çok fazla değer")
		}
		switch v := tk.(type) {
		case json.Delim:
			if v == '{' || v == '[' {
				if derinlik++; derinlik > 10 {
					return errors.New("çok derin")
				}
			} else {
				derinlik--
			}
		case json.Number:
			f, err := v.Float64()
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return errors.New("geçersiz sayı")
			}
		case bool, nil:
		case string:
			// nesne anahtarları da buraya düşer: onlar da kısa ad olmalı (kural ve ayar adları)
			if !secimKalip.MatchString(v) && (metin == nil || !metin.MatchString(v)) {
				return fmt.Errorf("beklenmeyen yazı %q", kisalt(v, 24))
			}
		}
	}
}

func kisalt(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ---------- ayarlar.json (exe'nin yanında; gizli bilgi içermez) ----------

var ayarMu sync.Mutex

func ayarYolu() string { return filepath.Join(veriKlasoru, "ayarlar.json") }

func ayarlariOku() map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	b, err := os.ReadFile(ayarYolu())
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	return m
}

func ayarlariYaz(m map[string]json.RawMessage) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	gecici := ayarYolu() + ".yeni"
	if err := os.WriteFile(gecici, b, 0644); err != nil {
		return err
	}
	return os.Rename(gecici, ayarYolu())
}

func ayarMetni(m map[string]json.RawMessage, ad string) string {
	var s string
	if v, ok := m[ad]; ok {
		_ = json.Unmarshal(v, &s)
	}
	return s
}

// Ömür tablosu: ayarlardaki değerler, eksik ya da geçersizse varsayılan.
func omurTablosu(m map[string]json.RawMessage) map[string]int {
	out := map[string]int{}
	for k, v := range varsayilanOmur {
		out[k] = v
	}
	if v, ok := m["omur"]; ok {
		var o map[string]float64
		if json.Unmarshal(v, &o) == nil {
			for k, dk := range o {
				if _, ok := varsayilanOmur[k]; ok && dk >= 5 && dk <= 525600 {
					out[k] = int(dk)
				}
			}
		}
	}
	return out
}

func klasorVarMi(yol string) bool {
	if yol == "" {
		return false
	}
	st, err := os.Stat(yol)
	return err == nil && st.IsDir()
}

// Strateji klasörünü bulur: önce ayarlardaki, yoksa olası yerler (simülatörün yanı, masaüstü).
// Bulduğunu ayarlara yazar ki hep aynı klasör izlensin.
func klasorBul() (yol, kaynak string) {
	ayarMu.Lock()
	defer ayarMu.Unlock()
	m := ayarlariOku()
	if y := ayarMetni(m, "klasor"); klasorVarMi(y) {
		return y, "ayar"
	} else if y != "" {
		return y, "yok" // kullanıcı seçmiş ama klasör artık yok: ona söyle, kendiliğinden başka yere geçme
	}
	if y := os.Getenv("ALSAT_KLASOR"); y != "" {
		return y, "ortam"
	}
	adaylar := []string{
		filepath.Join(exeKlasoru, "..", "KriptoSimulator", "stratejiler"),
		filepath.Join(exeKlasoru, "..", "stratejiler"),
	}
	ev := os.Getenv("USERPROFILE")
	if ev == "" {
		ev, _ = os.UserHomeDir()
	}
	if ev != "" {
		for _, masa := range []string{
			filepath.Join(ev, "OneDrive", "Masaüstü"), filepath.Join(ev, "OneDrive", "Desktop"),
			filepath.Join(ev, "Masaüstü"), filepath.Join(ev, "Desktop"),
		} {
			// En yeni simülatör klasörü önce (ör. KriptoSimulator_v3_3, KriptoSimulator_v3_2)
			var bulunan []string
			for _, kalip := range []string{"KriptoSimulator*/KriptoSimulator/stratejiler", "KriptoSimulator*/stratejiler"} {
				g, _ := filepath.Glob(filepath.Join(masa, kalip))
				bulunan = append(bulunan, g...)
			}
			sort.Slice(bulunan, func(i, j int) bool { return bulunan[i] > bulunan[j] })
			adaylar = append(adaylar, bulunan...)
		}
	}
	for _, a := range adaylar {
		if klasorVarMi(a) {
			temiz := filepath.Clean(a)
			if b, err := json.Marshal(temiz); err == nil {
				m["klasor"] = b
				if err := ayarlariYaz(m); err != nil {
					log.Println("Ayarlar yazılamadı:", err)
				}
			}
			log.Println("Strateji klasörü bulundu:", temiz)
			return temiz, "bulundu"
		}
	}
	return "", "yok"
}

func ayarlarIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		var gelen map[string]json.RawMessage
		if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&gelen); err != nil {
			hataYaz(w, 400, "Ayarlar okunamadı")
			return
		}
		ayarMu.Lock()
		defer ayarMu.Unlock()
		m := ayarlariOku()
		for k, v := range gelen {
			switch k {
			case "klasor":
				var y string
				if json.Unmarshal(v, &y) != nil || !klasorVarMi(strings.TrimSpace(y)) {
					hataYaz(w, 400, "Bu klasör bulunamadı: "+y)
					return
				}
				b, _ := json.Marshal(filepath.Clean(strings.TrimSpace(y)))
				m[k] = b
			case "omur":
				var o map[string]float64
				if json.Unmarshal(v, &o) != nil {
					hataYaz(w, 400, "Ömür tablosu okunamadı")
					return
				}
				m[k] = v
			case "koruma": // 0.7: tavan ve yeni kumbaraların hassasiyeti; gelen alanlar eskilerin üstüne yazılır
				var g struct {
					Tavan      *float64 `json:"tavan"`
					Hassasiyet *string  `json:"hassasiyet"`
				}
				if json.Unmarshal(v, &g) != nil {
					hataYaz(w, 400, "Koruma ayarı okunamadı")
					return
				}
				a := korumaAyariOku(m)
				if g.Tavan != nil && !math.IsNaN(*g.Tavan) && *g.Tavan >= 0 && *g.Tavan <= 90 {
					a.Tavan = *g.Tavan
				}
				if g.Hassasiyet != nil {
					if _, ok := hassasiyetler[*g.Hassasiyet]; ok {
						a.Hassasiyet = *g.Hassasiyet
					}
				}
				b, _ := json.Marshal(a)
				m[k] = b
			case "akis": // 0.9: akış onayının ayarları sadece /api/akis/ayar ile (denetlenerek) değişir
				continue
			default:
				if !json.Valid(v) {
					hataYaz(w, 400, "Ayar okunamadı: "+k)
					return
				}
				m[k] = v
			}
		}
		if err := ayarlariYaz(m); err != nil {
			hataYaz(w, 500, "Ayarlar kaydedilemedi: "+err.Error())
			return
		}
		if _, ok := gelen["koruma"]; ok {
			korumaGenel.Store(korumaAyariOku(m))
		}
		jsonYaz(w, 200, map[string]bool{"tamam": true})
		return
	}
	ayarMu.Lock()
	m := ayarlariOku()
	ayarMu.Unlock()
	omur, _ := json.Marshal(omurTablosu(m))
	m["omur"] = omur
	jsonYaz(w, 200, m)
}

// ---------- strateji dosyaları ----------

type stratejiDosyasi struct {
	Ad          string          `json:"ad"`
	Boyut       int64           `json:"boyut"`
	Degisti     int64           `json:"degisti"`
	Sembol      string          `json:"sembol,omitempty"`
	Aralik      string          `json:"aralik,omitempty"`
	Borsa       string          `json:"borsa,omitempty"`
	Olusturuldu int64           `json:"olusturuldu,omitempty"`
	Bitis       int64           `json:"bitis,omitempty"`
	KalanMs     int64           `json:"kalanMs"`
	Durum       string          `json:"durum"` // gecerli | suresiDoldu | hatali
	Hata        string          `json:"hata,omitempty"`
	Not         string          `json:"not,omitempty"`
	Icerik      json.RawMessage `json:"icerik,omitempty"`
}

// Bir dosyayı okuyup temel kontrolleri yapar.
func dosyaOku(yol string, omur map[string]int, simdi time.Time) stratejiDosyasi {
	d := stratejiDosyasi{Ad: filepath.Base(yol), Durum: "hatali"}
	st, err := os.Stat(yol)
	if err != nil {
		d.Hata = "Dosya okunamadı"
		return d
	}
	d.Boyut = st.Size()
	d.Degisti = st.ModTime().UnixMilli()
	if st.Size() > 1<<20 {
		d.Hata = "Dosya çok büyük; strateji dosyası değil"
		return d
	}
	b, err := os.ReadFile(yol)
	if err != nil {
		d.Hata = "Dosya okunamadı: " + err.Error()
		return d
	}
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	var ic struct {
		Tur         string          `json:"tur"`
		Olusturuldu string          `json:"olusturuldu"`
		Borsa       string          `json:"borsa"`
		Sembol      string          `json:"sembol"`
		Aralik      string          `json:"aralik"`
		Strateji    json.RawMessage `json:"strateji"`
		GecmisSonuc json.RawMessage `json:"gecmisSonuc"`
	}
	if err := json.Unmarshal(b, &ic); err != nil {
		d.Hata = "Dosya bozuk (JSON okunamadı)"
		return d
	}
	d.Sembol, d.Aralik, d.Borsa = ic.Sembol, ic.Aralik, ic.Borsa
	if ic.Tur != "kripto-simulator-strateji" {
		d.Hata = "Kripto Simülatör strateji dosyası değil"
		return d
	}
	if !sembolKalip.MatchString(ic.Sembol) {
		d.Hata = "Coin adı okunamadı"
		return d
	}
	if _, ok := araliklar[ic.Aralik]; !ok {
		d.Hata = "Mum süresi okunamadı"
		return d
	}
	var stj map[string]json.RawMessage
	if json.Unmarshal(ic.Strateji, &stj) != nil || stj == nil {
		d.Hata = "Dosyada strateji yok"
		return d
	}
	if err := degerDenetle(ic.Strateji, nil); err != nil {
		d.Hata = "Dosyanın strateji bölümünde beklenmeyen değer var (" + err.Error() + "); simülatörde yeniden dışa aktar"
		return d
	}
	if err := degerDenetle(ic.GecmisSonuc, tarihKalip); err != nil {
		d.Hata = "Dosyanın geçmiş sonuç bölümünde beklenmeyen değer var (" + err.Error() + "); simülatörde yeniden dışa aktar"
		return d
	}
	t, err := time.Parse(time.RFC3339Nano, ic.Olusturuldu)
	if err != nil {
		d.Hata = "Oluşturulma zamanı okunamadı"
		return d
	}
	d.Olusturuldu = t.UnixMilli()
	d.Bitis = t.Add(time.Duration(omur[ic.Aralik]) * time.Minute).UnixMilli()
	d.KalanMs = d.Bitis - simdi.UnixMilli()
	d.Icerik = json.RawMessage(b)
	if m := stratejiAdiKalip.FindStringSubmatch(d.Ad); m == nil || m[1] != ic.Sembol || m[2] != ic.Aralik {
		d.Not = "Dosya adı içindekiyle uyuşmuyor; içindeki coin ve mum süresi kullanılır."
	}
	switch {
	case ic.Borsa != "mexc":
		ad := ic.Borsa
		if ad == "binance" {
			ad = "Binance"
		}
		if ad == "" {
			ad = "bilinmeyen borsa"
		}
		d.Hata = ad + " verisiyle bulunmuş. MEXC'de işlem için simülatörde borsayı MEXC seçip ayarı yeniden bul ve dışa aktar."
	case t.After(simdi.Add(10 * time.Minute)):
		d.Hata = "Oluşturulma zamanı gelecekte görünüyor. Bilgisayarın saatini kontrol et."
	case d.KalanMs <= 0:
		d.Durum = "suresiDoldu"
	default:
		d.Durum = "gecerli"
	}
	return d
}

// Simülatörün motor dosyaları Al-Sat'takilerle aynı mı? (Simülatör güncellenip yeni kural eklenirse fark edilsin.)
func motorKarsilastir(klasor string) map[string]any {
	kok := filepath.Join(klasor, "..", "kaynak", "web")
	bulunan := 0
	farkli := []string{}
	for _, ad := range []string{"gostergeler.js", "kurallar.js", "motor.js"} {
		b, err := os.ReadFile(filepath.Join(kok, ad))
		if err != nil {
			continue
		}
		bulunan++
		gomulu, _ := webDosyalari.ReadFile("web/" + ad)
		duz := func(x []byte) []byte { return bytes.ReplaceAll(x, []byte("\r\n"), []byte("\n")) }
		if !bytes.Equal(duz(b), duz(gomulu)) {
			farkli = append(farkli, ad)
		}
	}
	durum := "ayni"
	if bulunan == 0 {
		durum = "bulunamadi"
	} else if len(farkli) > 0 {
		durum = "farkli"
	}
	return map[string]any{"durum": durum, "farkli": farkli, "yer": kok}
}

func stratejilerIstegi(w http.ResponseWriter, r *http.Request) {
	klasor, kaynak := klasorBul()
	ayarMu.Lock()
	omur := omurTablosu(ayarlariOku())
	ayarMu.Unlock()
	simdi := time.Now()
	cevap := map[string]any{
		"klasor": klasor, "klasorKaynagi": kaynak, "klasorVar": klasorVarMi(klasor),
		"simdi": simdi.UnixMilli(), "omur": omur, "dosyalar": []stratejiDosyasi{},
	}
	if !klasorVarMi(klasor) {
		jsonYaz(w, 200, cevap)
		return
	}
	girdiler, err := os.ReadDir(klasor)
	if err != nil {
		cevap["hata"] = "Klasör okunamadı: " + err.Error()
		jsonYaz(w, 200, cevap)
		return
	}
	dosyalar := []stratejiDosyasi{}
	for _, g := range girdiler {
		if g.IsDir() || !strings.EqualFold(filepath.Ext(g.Name()), ".json") {
			continue
		}
		dosyalar = append(dosyalar, dosyaOku(filepath.Join(klasor, g.Name()), omur, simdi))
	}
	sort.Slice(dosyalar, func(i, j int) bool { return dosyalar[i].Ad < dosyalar[j].Ad })
	cevap["dosyalar"] = dosyalar
	cevap["motor"] = motorKarsilastir(klasor)
	jsonYaz(w, 200, cevap)
}

// Windows'un klasör seçme penceresini açar; seçilen klasörü ayarlara yazar.
func klasorSecIstegi(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		hataYaz(w, 405, "Sadece POST")
		return
	}
	simdiki, _ := klasorBul()
	yol, secildi, err := klasorSecDiyalog(simdiki)
	if err != nil {
		hataYaz(w, 500, "Klasör seçme penceresi açılamadı: "+err.Error())
		return
	}
	if !secildi {
		jsonYaz(w, 200, map[string]any{"iptal": true})
		return
	}
	if !klasorVarMi(yol) {
		hataYaz(w, 400, "Bu klasör bulunamadı: "+yol)
		return
	}
	ayarMu.Lock()
	m := ayarlariOku()
	b, _ := json.Marshal(filepath.Clean(yol))
	m["klasor"] = b
	err = ayarlariYaz(m)
	ayarMu.Unlock()
	if err != nil {
		hataYaz(w, 500, "Ayarlar kaydedilemedi: "+err.Error())
		return
	}
	log.Println("Strateji klasörü seçildi:", yol)
	jsonYaz(w, 200, map[string]any{"klasor": filepath.Clean(yol)})
}

// PowerShell -EncodedCommand için: UTF-16LE + base64 (tırnak ve Türkçe harf sorunu olmasın diye)
func powershellKodla(betik string) string {
	u := utf16.Encode([]rune(betik))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i] = byte(v)
		b[2*i+1] = byte(v >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}
