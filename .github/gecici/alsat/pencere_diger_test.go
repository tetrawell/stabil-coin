//go:build !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// 0.4: bekçinin kapatacağı tarayıcı, profilin kilit dosyasından bulunur ve sadece komut satırında
// bu profil yazıyorsa kapatılır (Simülatöre ya da başka bir programa dokunulmaz).
func TestProfilTarayicisi(t *testing.T) {
	profil := t.TempDir()
	if profilTarayicisi(profil) != 0 {
		t.Fatal("kilit dosyası yokken 0 olmalı")
	}
	// bu profille çalışan sahte "tarayıcı"
	bizim := exec.Command("/bin/sh", "-c", "sleep 30", "tarayici", "--app=x", "--user-data-dir="+profil)
	if err := bizim.Start(); err != nil {
		t.Fatal(err)
	}
	defer bizim.Process.Kill()
	// başka bir profille çalışan program (dokunulmamalı)
	baska := exec.Command("/bin/sh", "-c", "sleep 30", "tarayici", "--user-data-dir="+filepath.Join(profil, "baska"))
	if err := baska.Start(); err != nil {
		t.Fatal(err)
	}
	defer baska.Process.Kill()
	time.Sleep(100 * time.Millisecond)

	kilit := filepath.Join(profil, "SingletonLock")
	_ = os.Symlink("storm-"+strconv.Itoa(bizim.Process.Pid), kilit)
	if pid := profilTarayicisi(profil); pid != bizim.Process.Pid {
		t.Fatalf("bizim tarayıcı bulunmalı: %d", pid)
	}
	_ = os.Remove(kilit)
	_ = os.Symlink("storm-"+strconv.Itoa(baska.Process.Pid), kilit)
	if pid := profilTarayicisi(profil); pid != 0 {
		t.Fatalf("başka profilin süreci asla seçilmemeli: %d", pid)
	}
	_ = os.Remove(kilit)
	_ = os.Symlink("storm-999999", kilit) // ölü süreç (eski kilit)
	if pid := profilTarayicisi(profil); pid != 0 {
		t.Fatalf("ölü süreç için 0 olmalı: %d", pid)
	}
	_ = os.Remove(kilit)
	_ = os.Symlink("bozuk", kilit)
	if profilTarayicisi(profil) != 0 {
		t.Fatal("bozuk kilit için 0 olmalı")
	}

	// surecBitti: çalışan süreç için false, öldürülünce true (zombi kalsa bile)
	if surecBitti(bizim.Process.Pid, 300*time.Millisecond) {
		t.Fatal("çalışan süreç bitmiş sayılmamalı")
	}
	_ = bizim.Process.Kill()
	if !surecBitti(bizim.Process.Pid, 3*time.Second) {
		t.Fatal("öldürülen süreç bitmiş sayılmalı")
	}
}

// 0.5: kilit dosyası işe yaramazsa ana tarayıcı komut satırından bulunur; komut satırı tek yazıya çevrilmiş
// olsa bile (Chromium bazen böyle yapar). Alt süreçler (--type=) ve başka profiller asla seçilmez.
func TestProfilSurecleri(t *testing.T) {
	profil := t.TempDir()
	baslat := func(argv0 string, args ...string) *exec.Cmd {
		// bash'in exec -a'sı süreç adını (argv[0]) istediğimiz yazı yapar: tek yazıya çevrilmiş komut satırı taklidi
		c := exec.Command("/bin/bash", append([]string{"-c", `exec -a "$0" sleep 30`, argv0}, args...)...)
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Process.Kill(); _, _ = c.Process.Wait() })
		return c
	}
	ayri := exec.Command("/bin/sh", "-c", "sleep 30", "chromium", "--app=x", "--user-data-dir="+profil)
	if err := ayri.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ayri.Process.Kill(); _, _ = ayri.Process.Wait() })
	tek := baslat("/usr/lib/chromium/chromium --app=http://127.0.0.1:47322/ --user-data-dir=" + profil + " --disable-gpu")
	alt := baslat("/usr/lib/chromium/chromium --type=renderer --user-data-dir=" + profil)
	baska := baslat("/usr/lib/chromium/chromium --user-data-dir=" + profil + "/baska")
	sim := baslat("/usr/lib/chromium/chromium --app=http://127.0.0.1:47321/")
	time.Sleep(200 * time.Millisecond)

	bulunan := map[int]bool{}
	for _, p := range profilSurecleri(profil) {
		bulunan[p] = true
	}
	if !bulunan[ayri.Process.Pid] || !bulunan[tek.Process.Pid] {
		t.Fatalf("iki ana tarayıcı da bulunmalı (ayrı ve tek yazı): %v", bulunan)
	}
	for ad, c := range map[string]*exec.Cmd{"alt süreç": alt, "başka profil": baska, "simülatör": sim} {
		if bulunan[c.Process.Pid] {
			t.Fatalf("%s asla seçilmemeli", ad)
		}
	}
	// kilit dosyası tek yazıya çevrilmiş komut satırını da tanır
	_ = os.Symlink("storm-"+strconv.Itoa(tek.Process.Pid), filepath.Join(profil, "SingletonLock"))
	if pid := profilTarayicisi(profil); pid != tek.Process.Pid {
		t.Fatalf("kilit dosyası tek yazılı komut satırını tanımalı: %d", pid)
	}
}
