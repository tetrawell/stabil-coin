package main

// WEBSOCKET İSTEMCİSİ (0.9) — MEXC'nin canlı işlem ve açık pozisyon akışı için (akis_veri.go).
// Go'nun standart kütüphanesinde websocket yok; programın dış bağımlılığı olmasın diye RFC 6455'in istemci tarafının
// gereken kısmı burada: el sıkışma (Sec-WebSocket-Accept denetimi), maskeli yazma, parçalı mesajları birleştirme,
// ping'e otomatik pong, kapanış. Sıkıştırma (permessage-deflate) istenmez, MEXC de göndermez.
//
// Kullanım: wsBaglan → Yaz/YazMetin (aynı anda birden çok yerden çağrılabilir) → Oku (tek okuyucu) → Kapat.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	wsDevam  byte = 0x0
	wsMetin  byte = 0x1
	wsIkili  byte = 0x2
	wsKapat  byte = 0x8
	wsPing   byte = 0x9
	wsPong   byte = 0xA
	wsEnFazl      = 16 << 20 // tek mesajın en fazla boyu (16 MB)
)

type wsBaglanti struct {
	conn    net.Conn
	r       *bufio.Reader
	yazMu   sync.Mutex
	enFazla int
}

// wsKapandi: karşı taraf kapanış çerçevesi gönderdi.
type wsKapandi struct {
	Kod   int
	Sebep string
}

func (e wsKapandi) Error() string { return fmt.Sprintf("websocket kapandı (%d %s)", e.Kod, e.Sebep) }

// wsKabul: istemcinin anahtarına sunucunun vermesi gereken cevap.
func wsKabul(anahtar string) string {
	h := sha1.Sum([]byte(anahtar + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// wsBaglan: ws:// ya da wss:// adrese bağlanır ve el sıkışır.
func wsBaglan(ctx context.Context, adres string, zamanAsimi time.Duration) (*wsBaglanti, error) {
	u, err := url.Parse(adres)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("websocket adresi değil: %s", adres)
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "wss" {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}
	d := &net.Dialer{Timeout: zamanAsimi}
	var c net.Conn
	if u.Scheme == "wss" {
		td := &tls.Dialer{NetDialer: d, Config: &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12}}
		c, err = td.DialContext(ctx, "tcp", host)
	} else {
		c, err = d.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		return nil, err
	}
	basarili := false
	defer func() {
		if !basarili {
			c.Close()
		}
	}()
	c.SetDeadline(time.Now().Add(zamanAsimi))
	ham := make([]byte, 16)
	if _, err := rand.Read(ham); err != nil {
		return nil, err
	}
	anahtar := base64.StdEncoding.EncodeToString(ham)
	yol := u.RequestURI()
	istek := "GET " + yol + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + anahtar + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"User-Agent: KriptoAlSat/" + surumAdi + "\r\n\r\n"
	if _, err := io.WriteString(c, istek); err != nil {
		return nil, err
	}
	br := bufio.NewReaderSize(c, 64<<10)
	cevap, err := http.ReadResponse(br, &http.Request{Method: "GET", URL: u})
	if err != nil {
		return nil, fmt.Errorf("websocket el sıkışması: %w", err)
	}
	if cevap.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("websocket el sıkışması: sunucu %d döndü", cevap.StatusCode)
	}
	if !strings.EqualFold(cevap.Header.Get("Upgrade"), "websocket") || !strings.Contains(strings.ToLower(cevap.Header.Get("Connection")), "upgrade") {
		return nil, errors.New("websocket el sıkışması: sunucu bağlantıyı yükseltmedi")
	}
	if cevap.Header.Get("Sec-WebSocket-Accept") != wsKabul(anahtar) {
		return nil, errors.New("websocket el sıkışması: kabul anahtarı yanlış")
	}
	c.SetDeadline(time.Time{})
	basarili = true
	return &wsBaglanti{conn: c, r: br, enFazla: wsEnFazl}, nil
}

// Yaz: tek parça, maskeli bir çerçeve gönderir.
func (w *wsBaglanti) Yaz(op byte, veri []byte) error {
	w.yazMu.Lock()
	defer w.yazMu.Unlock()
	n := len(veri)
	bas := make([]byte, 0, 14+n)
	bas = append(bas, 0x80|op)
	switch {
	case n < 126:
		bas = append(bas, 0x80|byte(n))
	case n <= 0xFFFF:
		bas = append(bas, 0x80|126, byte(n>>8), byte(n))
	default:
		bas = append(bas, 0x80|127)
		bas = binary.BigEndian.AppendUint64(bas, uint64(n))
	}
	var maske [4]byte
	if _, err := rand.Read(maske[:]); err != nil {
		return err
	}
	bas = append(bas, maske[:]...)
	for i, b := range veri {
		bas = append(bas, b^maske[i&3])
	}
	w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := w.conn.Write(bas)
	return err
}

func (w *wsBaglanti) YazMetin(s string) error { return w.Yaz(wsMetin, []byte(s)) }

// Oku: bir sonraki veri mesajını (metin ya da ikili) döndürür. Ping'lere kendisi pong verir, parçalı mesajları
// birleştirir. zamanAsimi içinde hiçbir çerçeve gelmezse hata döner (bağlantı ölmüş sayılır).
func (w *wsBaglanti) Oku(zamanAsimi time.Duration) (byte, []byte, error) {
	var mesaj []byte
	var mesajOp byte
	parcali := false
	for {
		w.conn.SetReadDeadline(time.Now().Add(zamanAsimi))
		var bas [2]byte
		if _, err := io.ReadFull(w.r, bas[:]); err != nil {
			return 0, nil, err
		}
		son := bas[0]&0x80 != 0
		if bas[0]&0x70 != 0 {
			return 0, nil, errors.New("websocket: beklenmeyen çerçeve bayrağı (sıkıştırma istenmedi)")
		}
		op := bas[0] & 0x0F
		maskeli := bas[1]&0x80 != 0
		n := uint64(bas[1] & 0x7F)
		switch n {
		case 126:
			var ek [2]byte
			if _, err := io.ReadFull(w.r, ek[:]); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(ek[:]))
		case 127:
			var ek [8]byte
			if _, err := io.ReadFull(w.r, ek[:]); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(ek[:])
		}
		if n > uint64(w.enFazla) || (op >= 0x8 && n > 125) {
			return 0, nil, fmt.Errorf("websocket: çerçeve çok büyük (%d bayt)", n)
		}
		var maske [4]byte
		if maskeli {
			if _, err := io.ReadFull(w.r, maske[:]); err != nil {
				return 0, nil, err
			}
		}
		veri := make([]byte, n)
		if _, err := io.ReadFull(w.r, veri); err != nil {
			return 0, nil, err
		}
		if maskeli {
			for i := range veri {
				veri[i] ^= maske[i&3]
			}
		}
		switch op {
		case wsPing:
			if err := w.Yaz(wsPong, veri); err != nil {
				return 0, nil, err
			}
		case wsPong:
		case wsKapat:
			kod, sebep := 1005, ""
			if len(veri) >= 2 {
				kod = int(binary.BigEndian.Uint16(veri[:2]))
				sebep = string(veri[2:])
			}
			w.Yaz(wsKapat, veri[:min(len(veri), 2)]) // kapanışa karşılık (en iyi çaba)
			return 0, nil, wsKapandi{kod, sebep}
		case wsMetin, wsIkili:
			if parcali {
				return 0, nil, errors.New("websocket: parçalı mesajın ortasında yeni mesaj")
			}
			if son {
				return op, veri, nil
			}
			parcali, mesajOp, mesaj = true, op, veri
		case wsDevam:
			if !parcali {
				return 0, nil, errors.New("websocket: başı olmayan devam çerçevesi")
			}
			if len(mesaj)+len(veri) > w.enFazla {
				return 0, nil, errors.New("websocket: mesaj çok büyük")
			}
			mesaj = append(mesaj, veri...)
			if son {
				return mesajOp, mesaj, nil
			}
		default:
			return 0, nil, fmt.Errorf("websocket: bilinmeyen çerçeve türü %d", op)
		}
	}
}

// Kapat: kapanış çerçevesi gönderir (en iyi çaba) ve bağlantıyı kapatır.
func (w *wsBaglanti) Kapat() error {
	w.Yaz(wsKapat, []byte{0x03, 0xE8})
	return w.conn.Close()
}
