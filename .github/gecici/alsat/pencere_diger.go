//go:build !windows

package main

// RASPBERRY PI (Linux) penceresi: Windows'taki gibi Chromium'u uygulama penceresi olarak açar,
// ayrı profil ve "arka planda yavaşlatma" kapalı bayraklarla (pencere küçültülse de mum kapanışları kaçmasın).
//
// 0.6: ekran kartı yeniden açık (--disable-gpu kaldırıldı): saatlik bozulmanın sebebi ekran kartı değil, 0.5'te
// düzeltilen tutamak sızıntısıydı; ekran kartı kapalıyken işlemci yükü %50'ye çıkıyordu.
// 0.4: Pi'de pencere her saat ekran kartı tarafında bozuluyordu (Chromium kaydı: "TransferBuffer::Initialize()
// failed", "Invalid node channel message"). Bu yüzden pencere ekran kartı kullanılmadan çizilir (--disable-gpu;
// Pi'nin işlemcisi bu ekranı rahat çizer). Bekçi pencereyi yeniden açarken de bozuk pencereyi bırakıp yenisini
// eklemek yerine, SADECE Al-Sat'ın kendi profilindeki tarayıcıyı kapatıp temiz açar (pencereler birikmez).

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func tarayiciBul() string {
	for _, ad := range []string{"chromium-browser", "chromium", "google-chrome", "google-chrome-stable", "microsoft-edge"} {
		if yol, err := exec.LookPath(ad); err == nil {
			return yol
		}
	}
	return ""
}

func profilKlasoru() string {
	ev, _ := os.UserHomeDir()
	return filepath.Join(ev, ".config", "KriptoAlSat", "pencere")
}

func pencereAc(adres string) {
	profil := profilKlasoru()
	_ = os.MkdirAll(profil, 0700)
	if yol := tarayiciBul(); yol != "" {
		args := []string{"--app=" + adres, "--start-maximized", "--no-first-run", "--no-default-browser-check",
			"--disable-background-timer-throttling", "--disable-renderer-backgrounding",
			"--disable-backgrounding-occluded-windows", "--disable-features=IntensiveWakeUpThrottling",
			"--password-store=basic", "--user-data-dir=" + profil}
		cmd := exec.Command(yol, args...)
		// Chromium'un kendi mesajları ayrı dosyaya (pencere bir daha sorun çıkarırsa sebebi burada görünür)
		if f := chromiumKayitAc(); f != nil {
			cmd.Stdout, cmd.Stderr = f, f
			defer f.Close() // süreç başladıktan sonra kendi kopyasını tutar
		}
		if err := tarayiciBaslat(cmd); err == nil {
			log.Println("Pencere açıldı:", yol)
			return
		}
	}
	log.Println("Chromium bulunamadı, varsayılan tarayıcı açılıyor")
	if err := exec.Command("xdg-open", adres).Start(); err != nil {
		log.Println("Pencere açılamadı:", err, "— tarayıcıda şu adresi aç:", adres)
	}
}

// Bekçi pencereyi yeniden açarken çağırır: Al-Sat'ın kendi profilindeki tarayıcı hâlâ açıksa (bozuk pencereyle)
// önce onu kapatır, sonra temiz bir pencere açar. Başka hiçbir programa (Simülatör, kullanıcının Chromium'u) dokunmaz:
// kapatılacak süreç, profilin kilit dosyasından bulunur ve komut satırında bu profilin yolu olduğu doğrulanır.
func pencereyiYenidenAc(adres string) {
	profil := profilKlasoru()
	pidler := []int{}
	if pid := profilTarayicisi(profil); pid > 0 {
		pidler = append(pidler, pid)
	} else {
		// 0.5: kilit dosyasından bulunamazsa, komut satırında bu profil yazan ana tarayıcı süreçleri aranır
		pidler = profilSurecleri(profil)
	}
	if len(pidler) == 0 {
		log.Println("Eski tarayıcı bulunamadı; sadece yeni pencere açılıyor.")
	}
	for _, pid := range pidler {
		log.Printf("Bozuk pencerenin tarayıcısı kapatılıyor (süreç %d), temiz pencere açılacak.", pid)
		bilerekKapatma.Store(time.Now().UnixNano())
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	for _, pid := range pidler {
		if !surecBitti(pid, 8*time.Second) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			surecBitti(pid, 3*time.Second)
		}
	}
	pencereAc(adres)
}

// Komut satırında "--user-data-dir=<profil>" geçen ve alt süreç olmayan (--type= yok) süreçler.
// Chromium bazı sistemlerde komut satırını tek yazıya çevirir; bu yüzden ayraç olarak NUL ya da boşluk kabul edilir.
func profilSurecleri(profil string) []int {
	aranan := "--user-data-dir=" + profil
	girdiler, _ := os.ReadDir("/proc")
	var out []int
	for _, g := range girdiler {
		pid, err := strconv.Atoi(g.Name())
		if err != nil || pid <= 1 || pid == os.Getpid() {
			continue
		}
		ham, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(ham), "\x00", " ") + " "
		if strings.Contains(cmd, " --type=") || !strings.Contains(" "+cmd, " "+aranan+" ") {
			continue
		}
		out = append(out, pid)
	}
	return out
}

// Profilin "SingletonLock" bağlantısı "bilgisayar-süreç" gösterir. Süreç yaşıyor ve komut satırında
// --user-data-dir=<profil> varsa onun numarasını verir; yoksa 0.
func profilTarayicisi(profil string) int {
	hedef, err := os.Readlink(filepath.Join(profil, "SingletonLock"))
	if err != nil {
		return 0
	}
	i := strings.LastIndex(hedef, "-")
	if i < 0 {
		return 0
	}
	pid, err := strconv.Atoi(hedef[i+1:])
	if err != nil || pid <= 1 || pid == os.Getpid() {
		return 0
	}
	cmd, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return 0
	}
	// komut satırı NUL ile ayrılmış olabilir ya da (Chromium bazen) tek yazıya çevrilmiş olabilir
	yazi := " " + strings.ReplaceAll(string(cmd), "\x00", " ") + " "
	if strings.Contains(yazi, " --user-data-dir="+profil+" ") {
		return pid
	}
	return 0
}

func surecBitti(pid int, sure time.Duration) bool {
	son := time.Now().Add(sure)
	for time.Now().Before(son) {
		if syscall.Kill(pid, 0) != nil {
			return true
		}
		// bizim başlattığımız süreçse ölü (zombi) kalabilir: durumuna bak
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			if f := strings.Fields(string(b)); len(f) > 2 && f[2] == "Z" {
				return true
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

// chromium_kayit.txt (programın yanında). 5 MB'ı geçince bir öncekine taşınır.
func chromiumKayitAc() *os.File {
	yol := filepath.Join(veriKlasoru, "chromium_kayit.txt")
	if st, err := os.Stat(yol); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(yol, filepath.Join(veriKlasoru, "chromium_kayit_eski.txt"))
	}
	f, err := os.OpenFile(yol, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil
	}
	fmt.Fprintf(f, "\n===== %s pencere açılıyor =====\n", time.Now().Format("2006-01-02 15:04:05"))
	return f
}

// Klasör seçme penceresi (Raspberry Pi masaüstünde zenity varsa). Yoksa sayfa yolu elle sorar.
func klasorSecDiyalog(baslangic string) (string, bool, error) {
	yol, err := exec.LookPath("zenity")
	if err != nil {
		return "", false, errors.New("klasör seçme penceresi bulunamadı; yolu elle yaz")
	}
	args := []string{"--file-selection", "--directory", "--title=Kripto Simülatör'ün stratejiler klasörünü seç"}
	if baslangic != "" {
		args = append(args, "--filename="+strings.TrimRight(baslangic, "/")+"/")
	}
	out, err := exec.Command(yol, args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", false, nil // vazgeçildi
		}
		return "", false, err
	}
	s := strings.TrimSpace(string(out))
	return s, s != "", nil
}

// 0.8: program açılamadığında kullanıcıya küçük bir uyarı penceresi (zenity varsa; yoksa false)
func acilisHatasi(mesaj string) bool {
	yol, err := exec.LookPath("zenity")
	if err != nil || (os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "") {
		return false
	}
	_ = exec.Command(yol, "--error", "--title=Kripto Al-Sat", "--width=480", "--no-markup", "--text="+mesaj).Run()
	return true
}
