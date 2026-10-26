//go:build windows

package main

// Kumbara çalışırken bilgisayarın uykuya geçmesini engeller (ekran kapanabilir, sistem uyumaz).
// SetThreadExecutionState çağıran iş parçacığına bağlıdır; bu yüzden tek bir sabit iş parçacığında çağrılır.

import (
	"runtime"
	"syscall"
)

var (
	kernel32Uyku   = syscall.NewLazyDLL("kernel32.dll")
	procUykuDurumu = kernel32Uyku.NewProc("SetThreadExecutionState")
	uykuKanali     = make(chan bool, 16)
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

func init() {
	go func() {
		runtime.LockOSThread()
		for acik := range uykuKanali {
			if acik {
				procUykuDurumu.Call(uintptr(esContinuous | esSystemRequired))
			} else {
				procUykuDurumu.Call(uintptr(esContinuous))
			}
		}
	}()
}

func uykuyuEngelle(acik bool) {
	select {
	case uykuKanali <- acik:
	default:
	}
}
