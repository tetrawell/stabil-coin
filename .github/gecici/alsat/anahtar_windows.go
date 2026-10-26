//go:build windows

package main

// Windows DPAPI (CryptProtectData / CryptUnprotectData): veri bu Windows hesabına bağlı şifrelenir.

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	crypt32         = syscall.NewLazyDLL("crypt32.dll")
	kernel32Anahtar = syscall.NewLazyDLL("kernel32.dll")
	procKoru        = crypt32.NewProc("CryptProtectData")
	procCoz         = crypt32.NewProc("CryptUnprotectData")
	procLocalFree   = kernel32Anahtar.NewProc("LocalFree")
)

type veriBlogu struct {
	boyut uint32
	veri  *byte
}

func blok(b []byte) *veriBlogu {
	if len(b) == 0 {
		return &veriBlogu{}
	}
	return &veriBlogu{boyut: uint32(len(b)), veri: &b[0]}
}

func (v *veriBlogu) kopya() []byte {
	if v.veri == nil || v.boyut == 0 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice(v.veri, v.boyut)...)
}

// Programa özel ek anahtar: başka programlar aynı dosyayı kolayca çözemesin
var dpapiEk = []byte("KriptoAlSat-MEXC-anahtari")

const cryptprotectUIForbidden = 0x1

func koru(veri []byte) ([]byte, error) {
	var cikti veriBlogu
	r, _, err := procKoru.Call(uintptr(unsafe.Pointer(blok(veri))), 0, uintptr(unsafe.Pointer(blok(dpapiEk))), 0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&cikti)))
	if r == 0 {
		return nil, errors.New("Windows şifrelemesi yapılamadı: " + err.Error())
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(cikti.veri)))
	return cikti.kopya(), nil
}

func coz(veri []byte) ([]byte, error) {
	var cikti veriBlogu
	r, _, err := procCoz.Call(uintptr(unsafe.Pointer(blok(veri))), 0, uintptr(unsafe.Pointer(blok(dpapiEk))), 0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&cikti)))
	if r == 0 {
		return nil, errors.New("anahtar çözülemedi (başka bir bilgisayar ya da Windows hesabı olabilir): " + err.Error())
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(cikti.veri)))
	return cikti.kopya(), nil
}
