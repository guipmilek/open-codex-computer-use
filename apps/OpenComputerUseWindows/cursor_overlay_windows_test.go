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

const (
	WS_VISIBLE = 0x10000000
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

func TestPowerShellDPIDiagnosticsUsingEmbeddedScript(t *testing.T) {
	req := psRequest{
		Tool: "dpi_diagnostics",
		App:  "explorer.exe",
	}

	resp, err := runPowerShell(req)
	if err != nil {
		t.Fatalf("Failed to execute real runtime.ps1 with dpi_diagnostics: %v", err)
	}

	if !resp.OK {
		t.Fatalf("Real runtime.ps1 returned OK=false for dpi_diagnostics")
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
		WS_EX_TOPMOST,
		uintptr(unsafe.Pointer(className)),
		0,
		WS_POPUP|WS_VISIBLE,
		300, 300, 300, 300,
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		t.Skip("Could not create test window")
	}
	defer procDestroyWindow.Call(hwnd)

	procSetWindowPos.Call(hwnd, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	time.Sleep(30 * time.Millisecond)

	hit := platformWindowIDAtPoint(Pt{X: 400, Y: 400}, 0)
	root := platformGetRootWindow(hwnd)

	if hit != root {
		t.Fatalf("platformWindowIDAtPoint = 0x%x, expected root window 0x%x", hit, root)
	}
}

func TestPlatformWindowIDAtPointOverlappingGeometry(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestPointAwareClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Target window at (300, 300, 300, 300) -> [300, 600]
	targetHWND, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		300, 300, 300, 300, 0, 0, hInst, 0,
	)
	if targetHWND == 0 {
		t.Skip("Could not create target window")
	}
	defer procDestroyWindow.Call(targetHWND)

	// Unrelated window at (0, 0, 100, 100) -> [0, 100] (DOES NOT contain P(400, 400))
	unrelatedHWND, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		0, 0, 100, 100, 0, 0, hInst, 0,
	)
	if unrelatedHWND == 0 {
		t.Skip("Could not create unrelated window")
	}
	defer procDestroyWindow.Call(unrelatedHWND)

	// Overlay window at (200, 200, 500, 500) -> [200, 700] (CONTAINS P(400, 400))
	overlayHWND, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		200, 200, 500, 500, 0, 0, hInst, 0,
	)
	if overlayHWND == 0 {
		t.Skip("Could not create overlay window")
	}
	defer procDestroyWindow.Call(overlayHWND)

	// Set Z-order: overlay (top) > unrelated (middle) > target (bottom)
	procSetWindowPos.Call(targetHWND, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	procSetWindowPos.Call(unrelatedHWND, targetHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	procSetWindowPos.Call(overlayHWND, unrelatedHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	time.Sleep(30 * time.Millisecond)

	P := Pt{X: 400, Y: 400}
	packedP := packPoint(P)

	// 1. Raw WindowFromPoint(P) must hit overlayHWND
	hitOverlay, _, _ := procWindowFromPoint.Call(packedP)
	if hitOverlay != overlayHWND {
		t.Fatalf("Raw WindowFromPoint(P) = 0x%x, expected overlay 0x%x", hitOverlay, overlayHWND)
	}

	// 2. Point-aware platformWindowIDAtPoint(P, overlayHWND) must skip unrelatedHWND (out of bounds) and return targetRoot!
	targetRoot := platformGetRootWindow(targetHWND)
	hitResolved := platformWindowIDAtPoint(P, overlayHWND)

	if hitResolved != targetRoot {
		t.Fatalf("platformWindowIDAtPoint(P, overlay) = 0x%x, expected targetRoot 0x%x", hitResolved, targetRoot)
	}
}

func getPrevVisibleRootWindow(hwnd uintptr) uintptr {
	for hwnd != 0 {
		prev, _, _ := procGetWindow.Call(hwnd, GW_HWNDPREV)
		if prev == 0 {
			break
		}
		hwnd = prev
		if isWndVisible(hwnd) {
			return platformGetRootWindow(hwnd)
		}
	}
	return 0
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

	// Create non-topmost target and unrelated test windows
	targetHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		300, 300, 400, 400, 0, 0, hInst, 0,
	)
	if targetHWND == 0 {
		t.Skip("Could not create target window")
	}
	defer procDestroyWindow.Call(targetHWND)

	unrelatedHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		300, 300, 400, 400, 0, 0, hInst, 0,
	)
	if unrelatedHWND == 0 {
		t.Skip("Could not create unrelated window")
	}
	defer procDestroyWindow.Call(unrelatedHWND)

	// Show without activation to avoid stealing focus
	procShowWindow.Call(unrelatedHWND, 8) // SW_SHOWNA
	procShowWindow.Call(targetHWND, 8)    // SW_SHOWNA

	// Setup initial Z-order: unrelated (top) > target (bottom)
	procSetWindowPos.Call(unrelatedHWND, HWND_TOP, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	procSetWindowPos.Call(targetHWND, unrelatedHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	time.Sleep(30 * time.Millisecond)

	ctrl := NewVisualCursorController()
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay creation unavailable")
	}

	targetRoot := platformGetRootWindow(targetHWND)
	unrelatedRoot := platformGetRootWindow(unrelatedHWND)

	// Position overlay immediately above target
	ctrl.overlay.updateFrame(200, 200, ctrl.initialRenderState(Pt{X: 200, Y: 200}), 0)
	ctrl.overlay.setZOrder(targetHWND)
	ctrl.overlay.show()
	time.Sleep(30 * time.Millisecond)

	// Assert BOTH Z-order relations:
	// 1. GetWindow(targetRoot, GW_HWNDPREV) == ctrl.overlay.hwnd
	// 2. getPrevVisibleRootWindow(ctrl.overlay.hwnd) == unrelatedRoot
	pTarget, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)
	pOverlay := getPrevVisibleRootWindow(ctrl.overlay.hwnd)

	if pTarget != ctrl.overlay.hwnd {
		t.Fatalf("GetWindow(target, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", pTarget, ctrl.overlay.hwnd)
	}
	if pOverlay != unrelatedRoot {
		t.Fatalf("getPrevVisibleRootWindow(overlay) = 0x%x, expected unrelated 0x%x", pOverlay, unrelatedRoot)
	}

	// Repeat setZOrder(targetHWND) 3+ times and re-verify BOTH relations
	for i := 0; i < 3; i++ {
		ctrl.overlay.setZOrder(targetHWND)
		time.Sleep(10 * time.Millisecond)

		pT, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)
		pO := getPrevVisibleRootWindow(ctrl.overlay.hwnd)

		if pT != ctrl.overlay.hwnd {
			t.Fatalf("Iteration %d: GetWindow(target, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", i, pT, ctrl.overlay.hwnd)
		}
		if pO != unrelatedRoot {
			t.Fatalf("Iteration %d: getPrevVisibleRootWindow(overlay) = 0x%x, expected unrelated 0x%x", i, pO, unrelatedRoot)
		}
	}

	// Test small movement (<= 2px): initial at (200, 200), small step to (201, 201)
	ctrl.MoveToAndWait(200, 200)
	ctrl.MoveToAndWait(201, 201)
	time.Sleep(30 * time.Millisecond)

	pTargetSmall, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)
	pOverlaySmall := getPrevVisibleRootWindow(ctrl.overlay.hwnd)

	if pTargetSmall != ctrl.overlay.hwnd {
		t.Fatalf("After small move: GetWindow(target, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", pTargetSmall, ctrl.overlay.hwnd)
	}
	if pOverlaySmall != unrelatedRoot {
		t.Fatalf("After small move: getPrevVisibleRootWindow(overlay) = 0x%x, expected unrelated 0x%x", pOverlaySmall, unrelatedRoot)
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

	// Target window (WS_EX_TOPMOST) at (300, 300, 500, 500)
	targetHWND, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		300, 300, 500, 500, 0, 0, hInst, 0,
	)
	if targetHWND == 0 {
		t.Skip("Could not create target window")
	}
	defer procDestroyWindow.Call(targetHWND)

	procSetWindowPos.Call(targetHWND, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	time.Sleep(30 * time.Millisecond)

	targetRoot := platformGetRootWindow(targetHWND)

	ctrl := NewVisualCursorController()
	ctrl.SetTargetHWND(targetHWND)
	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay creation unavailable")
	}

	// Place overlay directly over target at (400, 400)
	ctrl.overlay.setZOrder(targetHWND)
	ctrl.overlay.updateFrame(400, 400, ctrl.initialRenderState(Pt{X: 400, Y: 400}), 0)
	ctrl.overlay.show()

	// Prove: point-aware platformWindowIDAtPoint(Pt{400, 400}, ctrl.overlay.hwnd) returns targetRoot!
	hitBelow := platformWindowIDAtPoint(Pt{X: 400, Y: 400}, ctrl.overlay.hwnd)
	if hitBelow != targetRoot {
		t.Fatalf("platformWindowIDAtPoint below overlay = 0x%x, expected targetRoot 0x%x", hitBelow, targetRoot)
	}

	// Evaluate candidate trajectory and count target hits
	cand := ctrl.bestMotionCandidate(Pt{X: 350, Y: 350}, Pt{X: 450, Y: 450})
	pts := cand.Path.SampledConstraintPoints(10)
	targetHits := 0
	for _, pt := range pts {
		wnd := platformWindowIDAtPoint(pt, ctrl.overlay.hwnd)
		if wnd == targetRoot {
			targetHits++
		}
	}

	if targetHits == 0 {
		t.Fatalf("bestMotionCandidate targetHits = 0 when overlay is visible over target!")
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

	files = append(files, "runtime.ps1")
	prohibitedAPIs := []string{"SetCursorPos", "SendInput", "mouse_event"}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue // Skip test files, audit production files only
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", file, err)
		}
		text := string(content)
		for _, prohibited := range prohibitedAPIs {
			if strings.Contains(text, prohibited) {
				t.Fatalf("Prohibited physical mouse input API %q found in production file %s!", prohibited, file)
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
