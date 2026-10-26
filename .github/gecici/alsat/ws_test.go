package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// ---------- deneme sunucusu (RFC 6455'in sunucu tarafı, sadece testler için) ----------

type denemeWS struct {
	ln       net.Listener
	kabulBoz bool // yanlış Sec-WebSocket-Accept gönder
	isle     func(c net.Conn, r *bufio.Reader)
}

func (d *denemeWS) adres() string { return "ws://" + d.ln.Addr().String() + "/ws" }

func denemeWSKur(t *testing.T, isle func(c net.Conn, r *bufio.Reader)) *denemeWS {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	d := &denemeWS{ln: ln, isle: isle}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				req, err := http.ReadRequest(r)
				if err != nil {
					return
				}
				anahtar := req.Header.Get("Sec-WebSocket-Key")
				kabul := wsKabul(anahtar)
				if d.kabulBoz {
					kabul = "yanlis"
				}
				if req.Header.Get("Upgrade") != "websocket" || req.Header.Get("Sec-WebSocket-Version") != "13" {
					io.WriteString(c, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
					return
				}
				io.WriteString(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: "+kabul+"\r\n\r\n")
				d.isle(c, r)
			}()
		}
	}()
	return d
}

// sunucudan istemciye (maskesiz) çerçeve
func sunucuYaz(c net.Conn, son bool, op byte, veri []byte) {
	b0 := op
	if son {
		b0 |= 0x80
	}
	bas := []byte{b0}
	n := len(veri)
	switch {
	case n < 126:
		bas = append(bas, byte(n))
	case n <= 0xFFFF:
		bas = append(bas, 126, byte(n>>8), byte(n))
	default:
		bas = append(bas, 127)
		bas = binary.BigEndian.AppendUint64(bas, uint64(n))
	}
	c.Write(append(bas, veri...))
}

// istemcinin (maskeli olması ZORUNLU) çerçevesini okur
func sunucuOku(r *bufio.Reader) (op byte, veri []byte, maskeli bool, err error) {
	var bas [2]byte
	if _, err = io.ReadFull(r, bas[:]); err != nil {
		return
	}
	op = bas[0] & 0x0F
	maskeli = bas[1]&0x80 != 0
	n := uint64(bas[1] & 0x7F)
	if n == 126 {
		var e [2]byte
		io.ReadFull(r, e[:])
		n = uint64(binary.BigEndian.Uint16(e[:]))
	} else if n == 127 {
		var e [8]byte
		io.ReadFull(r, e[:])
		n = binary.BigEndian.Uint64(e[:])
	}
	var m [4]byte
	if maskeli {
		io.ReadFull(r, m[:])
	}
	veri = make([]byte, n)
	if _, err = io.ReadFull(r, veri); err != nil {
		return
	}
	for i := range veri {
		veri[i] ^= m[i&3]
	}
	return
}

func TestWSElSikismaVeYanki(t *testing.T) {
	maskesiz := make(chan bool, 10)
	d := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) {
		for {
			op, veri, maskeli, err := sunucuOku(r)
			if err != nil {
				return
			}
			maskesiz <- !maskeli
			if op == wsKapat {
				sunucuYaz(c, true, wsKapat, veri)
				return
			}
			sunucuYaz(c, true, op, veri) // yankı
		}
	})
	w, err := wsBaglan(context.Background(), d.adres(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Kapat()
	if err := w.YazMetin(`{"method":"PING"}`); err != nil {
		t.Fatal(err)
	}
	op, veri, err := w.Oku(5 * time.Second)
	if err != nil || op != wsMetin || string(veri) != `{"method":"PING"}` {
		t.Fatalf("yankı: %v %d %q", err, op, veri)
	}
	// 126 ve 127 uzunluk biçimleri
	for _, n := range []int{125, 126, 300, 65535, 65536, 200_000} {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(i * 7)
		}
		if err := w.Yaz(wsIkili, b); err != nil {
			t.Fatal(err)
		}
		op, veri, err := w.Oku(5 * time.Second)
		if err != nil || op != wsIkili || len(veri) != n || string(veri) != string(b) {
			t.Fatalf("%d bayt: %v %d %d", n, err, op, len(veri))
		}
	}
	for i := 0; i < 7; i++ {
		if <-maskesiz {
			t.Fatal("istemci çerçevesi maskesiz gönderildi (RFC 6455'e aykırı)")
		}
	}
}

func TestWSParcaliMesajVePing(t *testing.T) {
	pongGeldi := make(chan string, 1)
	d := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) {
		sunucuYaz(c, false, wsIkili, []byte("bir-"))
		sunucuYaz(c, true, wsPing, []byte("zz")) // parçaların arasında ping
		sunucuYaz(c, false, wsDevam, []byte("iki-"))
		sunucuYaz(c, true, wsDevam, []byte("uc"))
		op, veri, _, err := sunucuOku(r)
		if err == nil && op == wsPong {
			pongGeldi <- string(veri)
		}
		sunucuYaz(c, true, wsMetin, []byte("sonraki"))
		time.Sleep(200 * time.Millisecond)
	})
	w, err := wsBaglan(context.Background(), d.adres(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Kapat()
	op, veri, err := w.Oku(5 * time.Second)
	if err != nil || op != wsIkili || string(veri) != "bir-iki-uc" {
		t.Fatalf("parçalı mesaj: %v %d %q", err, op, veri)
	}
	select {
	case p := <-pongGeldi:
		if p != "zz" {
			t.Fatalf("pong içeriği %q", p)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ping'e pong verilmedi")
	}
	op, veri, err = w.Oku(5 * time.Second)
	if err != nil || string(veri) != "sonraki" {
		t.Fatalf("sonraki mesaj: %v %q", err, veri)
	}
}

func TestWSKapanisVeHatalar(t *testing.T) {
	d := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) {
		sunucuYaz(c, true, wsKapat, []byte{0x03, 0xE9, 'g', 'i', 't'})
		sunucuOku(r)
	})
	w, err := wsBaglan(context.Background(), d.adres(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = w.Oku(5 * time.Second)
	var k wsKapandi
	if !errors.As(err, &k) || k.Kod != 1001 || k.Sebep != "git" {
		t.Fatalf("kapanış: %v", err)
	}
	w.Kapat()

	// yanlış kabul anahtarı → bağlanmamalı
	d2 := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) {})
	d2.kabulBoz = true
	if _, err := wsBaglan(context.Background(), d2.adres(), 5*time.Second); err == nil || !strings.Contains(err.Error(), "kabul") {
		t.Fatalf("yanlış kabul anahtarı kabul edildi: %v", err)
	}

	// sunucu susarsa okuma zaman aşımına düşer
	d3 := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) { time.Sleep(3 * time.Second) })
	w3, err := wsBaglan(context.Background(), d3.adres(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bas := time.Now()
	if _, _, err := w3.Oku(300 * time.Millisecond); err == nil || time.Since(bas) > 2*time.Second {
		t.Fatalf("zaman aşımı çalışmadı: %v", err)
	}
	w3.Kapat()

	// sıkıştırma bayrağı (RSV1) gelirse hata
	d4 := denemeWSKur(t, func(c net.Conn, r *bufio.Reader) { c.Write([]byte{0xC2, 0x01, 0x00}); time.Sleep(time.Second) })
	w4, err := wsBaglan(context.Background(), d4.adres(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := w4.Oku(2 * time.Second); err == nil {
		t.Fatal("RSV1 bayraklı çerçeve kabul edildi")
	}
	w4.Kapat()
}

// ---------- MEXC çözücüleri: gerçek kayıtlarla (26 Ekim 2026), beklenenler Python ile çözüldü ----------

func TestMexcSpotCozucu(t *testing.T) {
	b, err := os.ReadFile("test/akis_ornek.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Spot []struct {
			B   string `json:"b"`
			Bek struct {
				Kanal    string  `json:"kanal"`
				Sembol   string  `json:"sembol"`
				Zaman    int64   `json:"zaman"`
				Islemler [][]any `json:"islemler"`
				Defter   *struct {
					Satis [][2]float64 `json:"satis"`
					Alis  [][2]float64 `json:"alis"`
				} `json:"defter"`
			} `json:"bek"`
		} `json:"spot"`
		Vadeli []struct {
			T   string `json:"t"`
			Bek struct {
				Kanal    string    `json:"kanal"`
				Sembol   string    `json:"sembol"`
				Islemler [][]any   `json:"islemler"`
				Ticker   []float64 `json:"ticker"`
			} `json:"bek"`
		} `json:"vadeli"`
		Metin []string `json:"metin"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	islemSay, defterSay := 0, 0
	for i, x := range f.Spot {
		ham, _ := base64.StdEncoding.DecodeString(x.B)
		p, err := mexcCoz(ham)
		if err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if p.Kanal != x.Bek.Kanal || p.Sembol != x.Bek.Sembol || p.Zaman != x.Bek.Zaman {
			t.Fatalf("%d: üst bilgi %q %q %d ≠ %q %q %d", i, p.Kanal, p.Sembol, p.Zaman, x.Bek.Kanal, x.Bek.Sembol, x.Bek.Zaman)
		}
		if len(p.Islemler) != len(x.Bek.Islemler) {
			t.Fatalf("%d: %d işlem, beklenen %d", i, len(p.Islemler), len(x.Bek.Islemler))
		}
		for j, e := range x.Bek.Islemler {
			g := p.Islemler[j]
			if g.Z != int64(e[0].(float64)) || g.Fiyat != e[1].(float64) || g.Adet != e[2].(float64) || g.Alim != e[3].(bool) {
				t.Fatalf("%d/%d: %+v ≠ %v", i, j, g, e)
			}
			islemSay++
		}
		if x.Bek.Defter != nil {
			if p.Defter == nil || len(p.Defter.Alis) != len(x.Bek.Defter.Alis) || len(p.Defter.Satis) != len(x.Bek.Defter.Satis) {
				t.Fatalf("%d: defter çözülmedi", i)
			}
			for j := range p.Defter.Alis {
				if p.Defter.Alis[j] != x.Bek.Defter.Alis[j] || p.Defter.Satis[j] != x.Bek.Defter.Satis[j] {
					t.Fatalf("%d: defter basamağı %d", i, j)
				}
			}
			if p.Defter.Alis[0][0] >= p.Defter.Satis[0][0] {
				t.Fatalf("%d: alıcı satıcıdan pahalı (alış/satış karışmış)", i)
			}
			defterSay++
		} else if p.Defter != nil {
			t.Fatalf("%d: beklenmeyen defter", i)
		}
	}
	if islemSay < 20 || defterSay != 3 {
		t.Fatalf("az örnek: %d işlem %d defter", islemSay, defterSay)
	}
	// kırık mesaj panik değil hata versin
	ham, _ := base64.StdEncoding.DecodeString(f.Spot[0].B)
	for _, kes := range []int{1, 5, 20, len(ham) / 2, len(ham) - 1} {
		if _, err := mexcCoz(ham[:kes]); err == nil {
			t.Fatalf("%d bayta kesik mesaj hatasız çözüldü", kes)
		}
	}
	// kontrol mesajları
	for _, m := range f.Metin {
		if tamam, _ := spotKontrol([]byte(m)); !tamam {
			t.Fatalf("kontrol mesajı tanınmadı: %s", m)
		}
	}
	if tamam, _ := spotKontrol([]byte(`{"id":0,"code":30001,"msg":"Not Subscribed successfully! [spot@x]"}`)); tamam {
		t.Fatal("hata mesajı tamam sayıldı")
	}

	// vadeli
	vIslem, vTicker := 0, 0
	for i, x := range f.Vadeli {
		p, err := vadeliCoz([]byte(x.T))
		if err != nil {
			t.Fatalf("vadeli %d: %v", i, err)
		}
		if p.Kanal != x.Bek.Kanal {
			t.Fatalf("vadeli %d kanal %q", i, p.Kanal)
		}
		if len(p.Islemler) != len(x.Bek.Islemler) {
			t.Fatalf("vadeli %d: %d işlem ≠ %d", i, len(p.Islemler), len(x.Bek.Islemler))
		}
		for j, e := range x.Bek.Islemler {
			g := p.Islemler[j]
			if g.Z != int64(e[0].(float64)) || g.Fiyat != e[1].(float64) || g.Adet != e[2].(float64) || g.Alim != e[3].(bool) {
				t.Fatalf("vadeli %d/%d: %+v ≠ %v", i, j, g, e)
			}
			vIslem++
		}
		if x.Bek.Ticker != nil {
			if p.Ticker == nil || p.Ticker.Z != int64(x.Bek.Ticker[0]) || p.Ticker.Fiyat != x.Bek.Ticker[1] ||
				p.Ticker.AcikP != x.Bek.Ticker[2] || math.Abs(p.Ticker.Fon-x.Bek.Ticker[3]) > 1e-15 || p.Sembol != x.Bek.Sembol {
				t.Fatalf("vadeli ticker %d: %+v ≠ %v", i, p.Ticker, x.Bek.Ticker)
			}
			vTicker++
		}
	}
	if vIslem < 3 || vTicker != 6 {
		t.Fatalf("vadeli az örnek: %d işlem %d ticker", vIslem, vTicker)
	}
	// eski biçim: data tek nesne
	p, err := vadeliCoz([]byte(`{"channel":"push.deal","data":{"M":1,"O":1,"T":2,"p":95.5,"t":1700000000000,"v":3},"symbol":"X_USDT","ts":1}`))
	if err != nil || len(p.Islemler) != 1 || p.Islemler[0].Alim || p.Islemler[0].Adet != 3 {
		t.Fatalf("tek nesne biçimi: %v %+v", err, p)
	}
	if vadeliAdi("BTCUSDT") != "BTC_USDT" || vadeliAdi("USDT") != "" || vadeliAdi("BTCUSDC") != "" {
		t.Fatal("vadeli adı")
	}
	_ = fmt.Sprint()
}
