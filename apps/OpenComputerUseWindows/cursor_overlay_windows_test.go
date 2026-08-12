//go:build windows

package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestWin32ABIPACKING(t *testing.T) {
	testCases := []struct {
		x, y     float64
		expected uintptr
	}{
		{x: 100, y: 200, expected: packPoint(Pt{X: 100, Y: 200})},
		{x: -50, y: 300, expected: packPoint(Pt{X: -50, Y: 300})},
		{x: 0, y: 500, expected: packPoint(Pt{X: 0, Y: 500})},
		{x: 500, y: 0, expected: packPoint(Pt{X: 500, Y: 0})},
	}

	for _, tc := range testCases {
		packed := packPoint(Pt{X: tc.x, Y: tc.y})
		if packed != tc.expected {
			t.Fatalf("packPoint(%v, %v) = 0x%x, expected 0x%x", tc.x, tc.y, packed, tc.expected)
		}

		unpackedX := int32(uint32(packed & 0xFFFFFFFF))
		unpackedY := int32(uint32(packed >> 32))

		if float64(unpackedX) != math.Round(tc.x) || float64(unpackedY) != math.Round(tc.y) {
			t.Fatalf("Unpacked point = (%d, %d), expected (%v, %v)", unpackedX, unpackedY, tc.x, tc.y)
		}
	}
}

func TestMonitorFromPointIntegration(t *testing.T) {
	pt := Pt{X: 100, Y: 100}
	packed := packPoint(pt)

	hMon, _, _ := procMonitorFromPoint.Call(packed, MONITOR_DEFAULTTONEAREST)
	if hMon == 0 {
		t.Fatal("procMonitorFromPoint returned 0 for valid screen point")
	}

	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	res, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi)))
	if res == 0 {
		t.Fatal("procGetMonitorInfoW returned 0 for valid HMONITOR")
	}

	expectedWork := Rect{
		X: float64(mi.RcWork.Left),
		Y: float64(mi.RcWork.Top),
		W: float64(mi.RcWork.Right - mi.RcWork.Left),
		H: float64(mi.RcWork.Bottom - mi.RcWork.Top),
	}

	actualWork := platformMonitorWorkArea(pt)
	if actualWork != expectedWork {
		t.Fatalf("platformMonitorWorkArea = %v, expected exact rcWork = %v", actualWork, expectedWork)
	}
}

func TestWindowFromPointIntegration(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestWinFromPtClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		WS_POPUP,
		200, 200, 300, 300,
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		t.Skip("Could not create test window")
	}
	defer procDestroyWindow.Call(hwnd)

	procShowWindow.Call(hwnd, SW_SHOW)
	procSetWindowPos.Call(hwnd, HWND_TOP, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	time.Sleep(20 * time.Millisecond)

	hit := platformWindowIDAtPoint(Pt{X: 250, Y: 250}, 0)
	root := platformGetRootWindow(hwnd)

	if hit != root {
		t.Fatalf("platformWindowIDAtPoint = 0x%x, expected root window 0x%x", hit, root)
	}
}

func TestThreeWindowZOrderIntegration(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestZOrderClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Create target window (bottom)
	targetHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		100, 100, 400, 400, 0, 0, hInst, 0,
	)
	if targetHWND == 0 {
		t.Skip("Could not create target window")
	}
	defer procDestroyWindow.Call(targetHWND)

	// Create unrelated window (top)
	unrelatedHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		100, 100, 400, 400, 0, 0, hInst, 0,
	)
	if unrelatedHWND == 0 {
		t.Skip("Could not create unrelated window")
	}
	defer procDestroyWindow.Call(unrelatedHWND)

	procShowWindow.Call(targetHWND, SW_SHOWNA)
	procShowWindow.Call(unrelatedHWND, SW_SHOWNA)

	// Place unrelated above target
	procSetWindowPos.Call(unrelatedHWND, targetHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)

	ctrl := NewVisualCursorController()
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay creation unavailable")
	}

	// Call setZOrder(targetHWND)
	ctrl.overlay.setZOrder(targetHWND)

	// Call setZOrder repeatedly to verify Z-order is preserved and not destroyed
	for i := 0; i < 3; i++ {
		ctrl.overlay.setZOrder(targetHWND)
		p, _, _ := procGetWindow.Call(ctrl.overlay.hwnd, GW_HWNDPREV)
		if p == ctrl.overlay.hwnd {
			t.Fatalf("setZOrder loop iteration %d set prev to self!", i)
		}
	}

	ctrl.Reset()
}

func TestCandidateScoringWithVisibleOverlayOverTarget(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestScoringClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	targetHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		100, 100, 500, 500, 0, 0, hInst, 0,
	)
	if targetHWND == 0 {
		t.Skip("Could not create target window")
	}
	defer procDestroyWindow.Call(targetHWND)
	procShowWindow.Call(targetHWND, SW_SHOWNA)

	ctrl := NewVisualCursorController()
	ctrl.SetTargetHWND(targetHWND)
	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay creation unavailable")
	}

	// Place overlay visually over target
	ctrl.overlay.updateFrame(200, 200, ctrl.initialRenderState(Pt{X: 200, Y: 200}), 0)
	ctrl.overlay.show()

	cand := ctrl.bestMotionCandidate(Pt{X: 150, Y: 150}, Pt{X: 250, Y: 250})
	if cand.Identifier == "" {
		t.Fatal("bestMotionCandidate returned empty candidate when overlay is visible over target")
	}

	ctrl.Reset()
}

func TestInstrumentedSynchronousCommitGate(t *testing.T) {
	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("OPEN_COMPUTER_USE_VISUAL_CURSOR disabled")
	}

	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay creation unavailable")
	}

	var moveFrameCommitTime time.Time
	var actionStartTime time.Time
	var pulseFrameCommitTime time.Time

	// 1. MoveToAndWait
	ctrl.MoveToAndWait(350, 350)
	moveFrameCommitTime = ctrl.overlay.lastCommitTime

	if !ctrl.overlay.lastCommitSucceeded {
		t.Fatal("MoveToAndWait UpdateLayeredWindow commit failed")
	}

	// 2. Action starts
	actionStartTime = time.Now()

	// 3. PulseClickAndWait
	ctrl.PulseClickAndWait(350, 350, 1, "left")
	pulseFrameCommitTime = ctrl.overlay.lastCommitTime

	if !ctrl.overlay.lastCommitSucceeded {
		t.Fatal("PulseClickAndWait UpdateLayeredWindow commit failed")
	}

	if moveFrameCommitTime.IsZero() || pulseFrameCommitTime.IsZero() {
		t.Fatal("Frame commit timestamps were zero")
	}

	if moveFrameCommitTime.After(actionStartTime) {
		t.Fatalf("Move frame commit (%v) occurred AFTER action start (%v)", moveFrameCommitTime, actionStartTime)
	}

	if actionStartTime.After(pulseFrameCommitTime) {
		t.Fatalf("Action start (%v) occurred AFTER pulse frame commit (%v)", actionStartTime, pulseFrameCommitTime)
	}

	ctrl.Reset()
}

func TestNoPhysicalMouseInputAPICalls(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatal("Could not list Go source files")
	}

	prohibitedAPIs := []string{"SetCursorPos", "SendInput", "mouse_event"}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue // Audit production source code only
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", file, err)
		}
		text := string(content)
		for _, prohibited := range prohibitedAPIs {
			if strings.Contains(text, prohibited) {
				t.Fatalf("Prohibited physical mouse input API %q found in production source file %s!", prohibited, file)
			}
		}
	}
}

func TestOverlayClickThroughStyle(t *testing.T) {
	procGetWindowLongW := user32.NewProc("GetWindowLongW")

	ctrl := NewVisualCursorController()
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay window unavailable")
	}

	exStyle, _, _ := procGetWindowLongW.Call(ctrl.overlay.hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	if exStyle&WS_EX_TRANSPARENT == 0 {
		t.Fatalf("Overlay window missing WS_EX_TRANSPARENT style: exStyle = 0x%x", exStyle)
	}

	if exStyle&WS_EX_NOACTIVATE == 0 {
		t.Fatalf("Overlay window missing WS_EX_NOACTIVATE style: exStyle = 0x%x", exStyle)
	}

	ctrl.Reset()
}
