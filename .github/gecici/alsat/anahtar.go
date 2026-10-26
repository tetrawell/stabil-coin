package main

// API ANAHTARI SAKLAMA — anahtar sadece bu bilgisayarda durur.
// Windows'ta %LOCALAPPDATA%\KriptoAlSat\mexc_anahtar.dat dosyasında, Windows'un kendi şifrelemesiyle (DPAPI):
// başka bir bilgisayarda ya da başka bir Windows hesabında açılamaz. Masaüstü OneDrive'a yedeklendiği için
// anahtar exe'nin yanına KONMAZ. Gizli anahtar tarayıcı penceresine hiçbir zaman geri gönderilmez.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type anahtarKaydi struct {
	Erisim string `json:"erisim"`
	Gizli  string `json:"gizli"`
	Kayit  int64  `json:"kayit"`
}

func anahtarKlasoru() string {
	if v := os.Getenv("ALSAT_ANAHTAR_KLASORU"); v != "" { // testler için
		return v
	}
	if yer := os.Getenv("LOCALAPPDATA"); yer != "" {
		return filepath.Join(yer, "KriptoAlSat")
	}
	ev, _ := os.UserHomeDir()
	return filepath.Join(ev, ".kriptoalsat")
}

func anahtarYolu() string { return filepath.Join(anahtarKlasoru(), "mexc_anahtar.dat") }

func anahtarKaydet(a anahtarKaydi) error {
	a.Erisim, a.Gizli = strings.TrimSpace(a.Erisim), strings.TrimSpace(a.Gizli)
	if a.Erisim == "" || a.Gizli == "" {
		return errors.New("Access Key ve Secret Key boş olamaz")
	}
	a.Kayit = time.Now().UnixMilli()
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	sifreli, err := koru(b)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(anahtarKlasoru(), 0700); err != nil {
		return err
	}
	gecici := anahtarYolu() + ".yeni"
	if err := os.WriteFile(gecici, sifreli, 0600); err != nil {
		return err
	}
	return os.Rename(gecici, anahtarYolu())
}

func anahtarOku() (anahtarKaydi, error) {
	var a anahtarKaydi
	b, err := os.ReadFile(anahtarYolu())
	if err != nil {
		return a, err
	}
	acik, err := coz(b)
	if err != nil {
		return a, err
	}
	err = json.Unmarshal(acik, &a)
	return a, err
}

func anahtarSil() error {
	err := os.Remove(anahtarYolu())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Ekranda gösterilecek kısaltılmış hâli: mx0v…k3Fq
func anahtarMaskesi(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-4:]
}
