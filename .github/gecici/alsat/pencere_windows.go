//go:build windows

package main

import (
	"bytes"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const pencereYok = 0x08000000 // CREATE_NO_WINDOW: PowerShell'in siyah penceresi görünmesin

// Chrome ya da Edge'i uygulama penceresi olarak açar.
// Simülatörden farkı: ayrı bir tarayıcı profili ve "arka planda yavaşlatma" kapalı.
// Böylece pencere küçültülse ya da arkada kalsa da bot mum kapanışlarını zamanında görür.
func pencereAc(adres string) {
	var profil string
	if yer := os.Getenv("LOCALAPPDATA"); yer != "" {
		profil = filepath.Join(yer, "KriptoAlSat", "pencere")
		_ = os.MkdirAll(profil, 0755)
	}
	adaylar := []string{
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LocalAppData"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
	}
	for _, yol := range adaylar {
		st, err := os.Stat(yol)
		if err != nil || st.IsDir() {
			continue
		}
		args := []string{"--app=" + adres, "--start-maximized", "--no-first-run", "--no-default-browser-check",
			"--disable-background-timer-throttling", "--disable-renderer-backgrounding",
			"--disable-backgrounding-occluded-windows", "--disable-features=CalculateNativeWinOcclusion,IntensiveWakeUpThrottling"}
		if profil != "" {
			args = append(args, "--user-data-dir="+profil)
		}
		if err := tarayiciBaslat(exec.Command(yol, args...)); err == nil {
			log.Println("Pencere açıldı:", yol)
			return
		}
	}
	log.Println("Chrome/Edge bulunamadı, varsayılan tarayıcı açılıyor")
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", adres).Start()
}

// Windows'ta pencere yeniden açılırken sadece yeni pencere açılır (0.3 ile aynı davranış).
func pencereyiYenidenAc(adres string) { pencereAc(adres) }

// Windows'un klasör seçme penceresi (PowerShell ile; ek program gerekmez).
func klasorSecDiyalog(baslangic string) (string, bool, error) {
	betik := `
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$sahip = New-Object System.Windows.Forms.Form
$sahip.TopMost = $true
$sahip.ShowInTaskbar = $false
$sahip.StartPosition = 'Manual'
$sahip.Location = New-Object System.Drawing.Point(-3000, -3000)
$sahip.Size = New-Object System.Drawing.Size(1, 1)
$sahip.Show()
$sahip.Activate()
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = 'Kripto Simülatör''ün stratejiler klasörünü seç'
$d.ShowNewFolderButton = $false
$d.SelectedPath = $env:KAS_BASLANGIC
if ($d.ShowDialog($sahip) -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $d.SelectedPath }
$sahip.Close()
`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-EncodedCommand", powershellKodla(betik))
	// 0.8: başlangıç klasörü betiğin içine yazılmaz, ortam değişkeniyle verilir: klasör adında ’ (akıllı tırnak)
	// olursa PowerShell onu da tırnak sayıyor, betik bozuluyordu
	cmd.Env = append(os.Environ(), "KAS_BASLANGIC="+baslangic)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: pencereYok}
	out, err := cmd.Output()
	if err != nil {
		return "", false, err
	}
	out = bytes.TrimPrefix(out, []byte("\xef\xbb\xbf"))
	yol := strings.TrimSpace(string(out))
	if i := strings.LastIndex(yol, "\n"); i >= 0 {
		yol = strings.TrimSpace(yol[i+1:])
	}
	return yol, yol != "", nil
}

// 0.8: program açılamadığında kullanıcıya uyarı penceresi
func acilisHatasi(mesaj string) bool {
	m, _ := syscall.UTF16PtrFromString(mesaj)
	b, _ := syscall.UTF16PtrFromString("Kripto Al-Sat")
	r, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(b)), 0x10|0x40000)
	return r != 0
}
