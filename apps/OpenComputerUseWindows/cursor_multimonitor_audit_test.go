//go:build windows

package main

import (
	"math"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var procSetWindowPosLocal = user32.NewProc("SetWindowPos")
var procGetForegroundWindowLocal = user32.NewProc("GetForegroundWindow")
var procMonitorFromPointLocal = user32.NewProc("MonitorFromPoint")

func hasNegativeMonitor() bool {
	var hasNegative bool
	cb := syscall.NewCallback(func(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
		type MONITORINFO struct {
			CbSize    uint32
			RcMonitor struct{ Left, Top, Right, Bottom int32 }
			RcWork    struct{ Left, Top, Right, Bottom int32 }
			DwFlags   uint32
		}
		var mi MONITORINFO
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		user32.NewProc("GetMonitorInfoW").Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
		
		if mi.RcMonitor.Top < 0 || mi.RcMonitor.Left < 0 {
			hasNegative = true
		}
		return 1
	})
	user32.NewProc("EnumDisplayMonitors").Call(0, 0, cb, 0)
	return hasNegative
}

func monitorCount() int {
	var count int
	cb := syscall.NewCallback(func(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
		count++
		return 1
	})
	user32.NewProc("EnumDisplayMonitors").Call(0, 0, cb, 0)
	return count
}

func TestMultiMonitorRealFixture(t *testing.T) {
	if monitorCount() < 2 {
		t.Skip("Skipping multi-monitor real test: requires at least 2 physical monitors")
	}
	if !hasNegativeMonitor() {
		t.Skip("Skipping multi-monitor real test: requires a monitor with negative coordinates")
	}

	t.Log("Beginning Real Multi-Monitor Validation")

	ctrl := NewVisualCursorController()

	var pt struct{ X, Y int32 }
	var fgBefore, fgAfter uintptr

	runWin32Scenario(t, func(t *testing.T) {
		t.Log("--- DISPLAY7 TESTS ---")
		ptsD7 := []struct{ X, Y float64 }{
			{10, 10}, {2550, 10}, {1280, 540}, {10, 1070}, {2550, 1070}, {500, 500},
			{100, 200}, {800, 800}, {2000, 100}, {1500, 900},
		}

		for _, p := range ptsD7 {
			fgBefore, _, _ = procGetForegroundWindowLocal.Call()
			user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
			oldX, oldY := pt.X, pt.Y

			ctrl.MoveToAndWait(p.X, p.Y)
			ctrl.PulseClickAndWait(p.X, p.Y, 1, "left")

			visualX := float64(ctrl.overlay.lastPos.X) + cursorTipAnchorX
			visualY := float64(ctrl.overlay.lastPos.Y) + cursorTipAnchorY
			dist := math.Sqrt((visualX-p.X)*(visualX-p.X) + (visualY-p.Y)*(visualY-p.Y))

			mon, _, _ := procMonitorFromPointLocal.Call(uintptr(int32(p.X)), uintptr(int32(p.Y)), 2)

			t.Logf("D7 Target: (%.1f, %.1f), Visual: (%.1f, %.1f), Error: %.2fpx, Monitor: %d", p.X, p.Y, visualX, visualY, dist, mon)
			if dist > 1.0 {
				t.Errorf("Visual error > 1px for target %.1f, %.1f", p.X, p.Y)
			}

			user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
			if pt.X != oldX || pt.Y != oldY {
				t.Errorf("Physical cursor moved! Before: %d,%d, After: %d,%d", oldX, oldY, pt.X, pt.Y)
			}
			fgAfter, _, _ = procGetForegroundWindowLocal.Call()
			if fgBefore != fgAfter {
				t.Errorf("Foreground changed!")
			}
		}

		t.Log("--- DISPLAY6 TESTS (Negative Y) ---")
		ptsD6 := []struct{ X, Y float64 }{
			{10, -1070}, {2550, -1070}, {1280, -540}, {10, -10}, {2550, -10},
			{500, -500}, {100, -200}, {800, -800}, {2000, -100}, {1500, -900},
			{1280, -1050}, {1280, -900}, {1280, -100}, {1280, -50}, {1280, -1},
		}

		for _, p := range ptsD6 {
			ctrl.MoveToAndWait(p.X, p.Y)
			ctrl.PulseClickAndWait(p.X, p.Y, 1, "left")

			visualX := float64(ctrl.overlay.lastPos.X) + cursorTipAnchorX
			visualY := float64(ctrl.overlay.lastPos.Y) + cursorTipAnchorY
			dist := math.Sqrt((visualX-p.X)*(visualX-p.X) + (visualY-p.Y)*(visualY-p.Y))

			if dist > 1.0 {
				t.Errorf("Visual error > 1px for target %.1f, %.1f", p.X, p.Y)
			}
		}

		t.Log("--- CROSS-MONITOR TRANSITIONS ---")
		for i := 0; i < 20; i++ {
			ctrl.MoveToAndWait(1280, 200)  // D7
			ctrl.MoveToAndWait(1280, -200) // D6
		}

		t.Log("--- MULTI-MONITOR STRESS ---")
		for i := 0; i < 100; i++ {
			x := float64(100 + (i*20)%2000)
			y := float64(-500 + (i%2)*1000)
			ctrl.MoveToAndWait(x, y)
		}

		t.Log("--- BOUNDARY CROSSING TESTS ---")
		ptsSpan := []struct{ X, Y float64 }{
			{1280, -5}, {1280, 0}, {1280, 5},
		}
		for _, p := range ptsSpan {
			ctrl.MoveToAndWait(p.X, p.Y)
		}

		time.Sleep(200 * time.Millisecond)
	})
}
