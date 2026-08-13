//go:build windows

package main

import (
	"runtime"
	"testing"
	"time"
	"unsafe"
)

// Win32 HWNDs have OS-thread affinity. CreateWindowExW binds the window to the
// message queue of the calling thread, and ShowWindow / SetWindowPos /
// DestroyWindow issued from any other thread are marshalled back to that owning
// thread. A Go test goroutine may migrate between OS threads at any point, and
// the runtime threads it lands on never run a Win32 message pump, so such a
// cross-thread call blocks forever: the test binary hangs, the leftover windows
// are marked "not responding", and the process is orphaned.
//
// runWin32Scenario is the single entry point for every scenario that owns its
// own HWNDs. It pins the goroutine for the whole scenario, so creation,
// SetWindowPos / ShowWindow, assertions and the deferred DestroyWindow all run
// on the same OS thread. Scenario bodies are unexported helpers so the testing
// package discovers exactly one Test function per scenario.
//
// The overlay window is deliberately exempt: platformCreateOverlay owns a
// dedicated locked thread with a real GetMessage loop, and postCmdSync
// marshals every overlay operation onto it. Tests that only drive the
// controller therefore never take ownership of an HWND themselves.
func runWin32Scenario(t *testing.T, scenario func(*testing.T)) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer pumpWin32Messages()
	scenario(t)
}

const pmRemove = 0x0001

var procPeekMessageW = user32.NewProc("PeekMessageW")

// pumpWin32Messages drains the calling thread's message queue so the windows it
// owns keep answering the system instead of going into the "not responding"
// ghost-window state while the scenario runs.
func pumpWin32Messages() {
	var msg MSG
	for {
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmRemove)
		if ret == 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// settleWin32 waits for d while keeping this thread's message queue pumped, so
// Z-order and visibility changes are applied without starving the windows the
// scenario owns.
func settleWin32(d time.Duration) {
	deadline := time.Now().Add(d)
	for {
		pumpWin32Messages()
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		if remaining > 5*time.Millisecond {
			remaining = 5 * time.Millisecond
		}
		time.Sleep(remaining)
	}
}
