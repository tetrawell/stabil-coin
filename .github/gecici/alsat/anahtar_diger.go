//go:build !windows

package main

// RASPBERRY PI (Linux) anahtar şifrelemesi. Windows'taki DPAPI'nin yerine:
// AES-256-GCM; şifreleme anahtarı bu cihazın kimliğinden (/etc/machine-id) ve ilk kayıtta üretilen
// rastgele bir tuzdan türetilir. Dosyalar sadece bu kullanıcının okuyabileceği izinlerle yazılır (0600).
// Dosya başka bir cihaza kopyalansa açılamaz.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var dpapiEk = []byte("KriptoAlSat-MEXC-anahtari")

func cihazKimligi() []byte {
	for _, yol := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(yol); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			return []byte(strings.TrimSpace(string(b)))
		}
	}
	ad, _ := os.Hostname()
	return []byte("makine:" + ad)
}

func sifreAnahtari(olustur bool) ([]byte, error) {
	yol := filepath.Join(anahtarKlasoru(), "tuz.dat")
	tuz, err := os.ReadFile(yol)
	if err != nil || len(tuz) != 32 {
		if !olustur {
			return nil, errors.New("anahtarın tuz dosyası bulunamadı")
		}
		tuz = make([]byte, 32)
		if _, err := rand.Read(tuz); err != nil {
			return nil, err
		}
		if err := os.MkdirAll(anahtarKlasoru(), 0700); err != nil {
			return nil, err
		}
		if err := os.WriteFile(yol, tuz, 0600); err != nil {
			return nil, err
		}
	}
	h := sha256.New()
	h.Write(dpapiEk)
	h.Write(cihazKimligi())
	h.Write(tuz)
	return h.Sum(nil), nil
}

func koru(veri []byte) ([]byte, error) {
	k, err := sifreAnahtari(true)
	if err != nil {
		return nil, err
	}
	blok, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blok)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, veri, nil), nil
}

func coz(veri []byte) ([]byte, error) {
	k, err := sifreAnahtari(false)
	if err != nil {
		return nil, err
	}
	blok, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(blok)
	if err != nil {
		return nil, err
	}
	if len(veri) < gcm.NonceSize() {
		return nil, errors.New("anahtar dosyası bozuk")
	}
	acik, err := gcm.Open(nil, veri[:gcm.NonceSize()], veri[gcm.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("anahtar çözülemedi (başka bir cihazdan kopyalanmış olabilir)")
	}
	return acik, nil
}
