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

	if !resp.EffectiveDpiContextIsPMv2 {
		t.Fatalf("Effective DPI context is NOT Per-Monitor V2 (effectiveDpiContextIsPMv2=false)")
	}

	if resp.GetWindowRect == nil {
		t.Fatalf("getWindowRect is nil in dpi_diagnostics response")
	}

	rectW := resp.GetWindowRect.Right - resp.GetWindowRect.Left
	rectH := resp.GetWindowRect.Bottom - resp.GetWindowRect.Top

	if rectW <= 0 || rectH <= 0 {
		t.Fatalf("getWindowRect dimensions are not positive: width=%d, height=%d", rectW, rectH)
	}

	if resp.WindowBounds != nil {
		if resp.WindowBounds.Width <= 0 || resp.WindowBounds.Height <= 0 {
			t.Fatalf("windowBounds dimensions are not positive: %v", resp.WindowBounds)
		}
	}

	if resp.UIABoundingRectangle != nil {
		if resp.UIABoundingRectangle.Width <= 0 || resp.UIABoundingRectangle.Height <= 0 {
			t.Fatalf("uiaBoundingRectangle dimensions are not positive: %v", resp.UIABoundingRectangle)
		}
		diffW := math.Abs(float64(rectW) - resp.UIABoundingRectangle.Width)
		diffH := math.Abs(float64(rectH) - resp.UIABoundingRectangle.Height)
		if diffW > 20 || diffH > 20 {
			t.Fatalf("Inconsistency between GetWindowRect (%dx%d) and UIA BoundingRectangle (%.0fx%.0f)", rectW, rectH, resp.UIABoundingRectangle.Width, resp.UIABoundingRectangle.Height)
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
	runWin32Scenario(t, windowFromPointIntegrationScenario)
}

func windowFromPointIntegrationScenario(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestWinFromPtClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   procDefWindowProcW.Addr(),
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
	defer destroyTestWindow(hwnd)

	procSetWindowPos.Call(hwnd, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	settleWin32(30 * time.Millisecond)

	hit := platformWindowIDAtPoint(Pt{X: 400, Y: 400}, 0)
	root := platformGetRootWindow(hwnd)

	if hit != root {
		t.Fatalf("platformWindowIDAtPoint = 0x%x, expected root window 0x%x", hit, root)
	}
}

func TestPlatformWindowIDAtPointOverlappingGeometry(t *testing.T) {
	runWin32Scenario(t, platformWindowIDAtPointOverlappingGeometryScenario)
}

func platformWindowIDAtPointOverlappingGeometryScenario(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestPointAwareClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   procDefWindowProcW.Addr(),
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
	defer destroyTestWindow(targetHWND)

	// Unrelated window at (0, 0, 100, 100) -> [0, 100] (DOES NOT contain P(400, 400))
	unrelatedHWND, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		0, 0, 100, 100, 0, 0, hInst, 0,
	)
	if unrelatedHWND == 0 {
		t.Skip("Could not create unrelated window")
	}
	defer destroyTestWindow(unrelatedHWND)

	// Overlay window at (200, 200, 500, 500) -> [200, 700] (CONTAINS P(400, 400))
	overlayHWND, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		200, 200, 500, 500, 0, 0, hInst, 0,
	)
	if overlayHWND == 0 {
		t.Skip("Could not create overlay window")
	}
	defer destroyTestWindow(overlayHWND)

	// Set Z-order: overlay (top) > unrelated (middle) > target (bottom)
	procSetWindowPos.Call(targetHWND, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	procSetWindowPos.Call(unrelatedHWND, targetHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	procSetWindowPos.Call(overlayHWND, unrelatedHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	settleWin32(30 * time.Millisecond)

	P := Pt{X: 400, Y: 400}
	packedP := packPoint(P)

	// 1. Raw WindowFromPoint(P) must hit overlayHWND
	hitOverlay, _, _ := procWindowFromPoint.Call(packedP)
	t.Logf("raw hitOverlay=0x%x, overlayHWND=0x%x", hitOverlay, overlayHWND)
	if hitOverlay != overlayHWND {
		t.Fatalf("Raw WindowFromPoint(P) = 0x%x, expected overlay 0x%x", hitOverlay, overlayHWND)
	}

	// 2. Point-aware platformWindowIDAtPoint(P, overlayHWND) must skip unrelatedHWND (out of bounds) and return targetRoot!
	targetRoot := platformGetRootWindow(targetHWND)
	hitResolved := platformWindowIDAtPoint(P, overlayHWND)
	t.Logf("hitResolved=0x%x, targetRoot=0x%x", hitResolved, targetRoot)

	if hitResolved != targetRoot {
		t.Fatalf("platformWindowIDAtPoint(P, overlay) = 0x%x, expected targetRoot 0x%x", hitResolved, targetRoot)
	}
}

// destroyTestWindow hides and destroys a scenario-owned window. It must run on
// the OS thread that created the HWND, which runWin32Scenario guarantees.
func destroyTestWindow(hwnd uintptr) {
	if hwnd != 0 {
		procShowWindow.Call(hwnd, 0) // SW_HIDE
		procDestroyWindow.Call(hwnd)
	}
}

// zOrderIsAbove reports whether higherHWND sits somewhere above lowerHWND
// in the Z-order stack, walking upward via GW_HWNDPREV. Strict adjacency is
// NOT required: other windows may legitimately appear between the two. This
// lets tests assert the relative contract (e.g. "unrelated window is above
// the overlay") without depending on full desktop topology.
func zOrderIsAbove(higherHWND, lowerHWND uintptr) bool {
	const maxSteps = 4096
	hwnd := lowerHWND
	for i := 0; i < maxSteps && hwnd != 0; i++ {
		prev, _, _ := procGetWindow.Call(hwnd, GW_HWNDPREV)
		if prev == 0 {
			return false
		}
		if prev == higherHWND {
			return true
		}
		hwnd = prev
	}
	return false
}

func TestThreeWindowZOrderIntegration(t *testing.T) {
	runWin32Scenario(t, threeWindowZOrderIntegrationScenario)
}

func threeWindowZOrderIntegrationScenario(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestZOrderClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   procDefWindowProcW.Addr(),
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
	defer destroyTestWindow(targetHWND)

	unrelatedHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		300, 300, 400, 400, 0, 0, hInst, 0,
	)
	if unrelatedHWND == 0 {
		t.Skip("Could not create unrelated window")
	}
	defer destroyTestWindow(unrelatedHWND)

	// Show without activation to avoid stealing focus
	procShowWindow.Call(unrelatedHWND, 8) // SW_SHOWNA
	procShowWindow.Call(targetHWND, 8)    // SW_SHOWNA

	// Setup initial Z-order: unrelated (top) > target (bottom)
	procSetWindowPos.Call(unrelatedHWND, HWND_TOP, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	procSetWindowPos.Call(targetHWND, unrelatedHWND, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	settleWin32(30 * time.Millisecond)

	ctrl := NewVisualCursorController()
	ctrl.SetTargetHWND(targetHWND)
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay creation unavailable")
	}
	defer ctrl.Reset()

	targetRoot := platformGetRootWindow(targetHWND)
	unrelatedRoot := platformGetRootWindow(unrelatedHWND)

	// MoveToAndWait exercises ensureOverlay, refreshActiveOrderingIfNeeded, and initial placement
	ctrl.MoveToAndWait(200, 200)
	settleWin32(30 * time.Millisecond)

	// Assert BOTH Z-order relations:
	// 1. GetWindow(targetRoot, GW_HWNDPREV) == ctrl.overlay.hwnd (overlay directly above target)
	// 2. unrelatedRoot sits above ctrl.overlay.hwnd (relative contract, not strict adjacency)
	pTarget, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)

	if pTarget != ctrl.overlay.hwnd {
		t.Fatalf("GetWindow(target, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", pTarget, ctrl.overlay.hwnd)
	}
	if !zOrderIsAbove(unrelatedRoot, ctrl.overlay.hwnd) {
		t.Fatalf("unrelated root 0x%x is not above overlay 0x%x in the Z-order", unrelatedRoot, ctrl.overlay.hwnd)
	}

	// Repeat setZOrder(targetHWND) 3+ times via SetTargetHWND and re-verify BOTH relations
	for i := 0; i < 3; i++ {
		ctrl.SetTargetHWND(targetHWND)
		settleWin32(10 * time.Millisecond)

		pT, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)

		if pT != ctrl.overlay.hwnd {
			t.Fatalf("Iteration %d: GetWindow(target, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", i, pT, ctrl.overlay.hwnd)
		}
		if !zOrderIsAbove(unrelatedRoot, ctrl.overlay.hwnd) {
			t.Fatalf("Iteration %d: unrelated root 0x%x is not above overlay 0x%x in the Z-order", i, unrelatedRoot, ctrl.overlay.hwnd)
		}
	}

	// Test small movement (<= 2px): initial at (200, 200), small step to (201, 201)
	ctrl.MoveToAndWait(201, 201)
	settleWin32(30 * time.Millisecond)

	pTargetSmall, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)

	if pTargetSmall != ctrl.overlay.hwnd {
		t.Fatalf("After small move: GetWindow(target, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", pTargetSmall, ctrl.overlay.hwnd)
	}
	if !zOrderIsAbove(unrelatedRoot, ctrl.overlay.hwnd) {
		t.Fatalf("After small move: unrelated root 0x%x is not above overlay 0x%x in the Z-order", unrelatedRoot, ctrl.overlay.hwnd)
	}
}

func TestCandidateScoringWithVisibleOverlayOverNormalTarget(t *testing.T) {
	runWin32Scenario(t, candidateScoringWithVisibleOverlayOverNormalTargetScenario)
}

func candidateScoringWithVisibleOverlayOverNormalTargetScenario(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestNormalScoringClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   procDefWindowProcW.Addr(),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Target window (Normal Non-Topmost representative of Notepad/Explorer) at (1200, 200, 400, 400)
	targetHWND, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		1200, 200, 400, 400, 0, 0, hInst, 0,
	)
	if targetHWND == 0 {
		t.Skip("Could not create target window")
	}
	defer destroyTestWindow(targetHWND)

	// Raise the target to the top of the non-TOPMOST band. A plain HWND_TOP call
	// from a process that does not own the foreground window cannot reliably
	// cover a maximized foreground app, which would leave an unrelated window
	// between the point and the target. Promote-then-demote gives a
	// deterministic ordering while keeping the window genuinely non-TOPMOST.
	procSetWindowPos.Call(targetHWND, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE|SWP_SHOWWINDOW)
	procSetWindowPos.Call(targetHWND, HWND_NOTOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE)
	settleWin32(30 * time.Millisecond)

	procGetWindowLongW := user32.NewProc("GetWindowLongW")
	targetExStyle, _, _ := procGetWindowLongW.Call(targetHWND, ^uintptr(19)) // GWL_EXSTYLE = -20
	if targetExStyle&WS_EX_TOPMOST != 0 {
		t.Fatalf("Target window must stay non-TOPMOST for this scenario: exStyle = 0x%x", targetExStyle)
	}

	targetRoot := platformGetRootWindow(targetHWND)

	ctrl := NewVisualCursorController()
	ctrl.SetTargetHWND(targetHWND)
	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay creation unavailable")
	}
	defer ctrl.Reset()

	// Position overlay immediately above target at (1300, 300)
	ctrl.overlay.setZOrder(targetHWND)
	ctrl.overlay.updateFrame(1300, 300, ctrl.initialRenderState(Pt{X: 1300, Y: 300}), 0)
	ctrl.overlay.show()

	// Prove: point-aware platformWindowIDAtPoint(Pt{1300, 300}, ctrl.overlay.hwnd) returns targetRoot!
	hitBelow := platformWindowIDAtPoint(Pt{X: 1300, Y: 300}, ctrl.overlay.hwnd)
	if hitBelow != targetRoot {
		var tr RECT
		procGetWindowRect.Call(targetHWND, uintptr(unsafe.Pointer(&tr)))
		raw, _, _ := procWindowFromPoint.Call(packPoint(Pt{X: 1300, Y: 300}))
		prevOfTarget, _, _ := procGetWindow.Call(targetRoot, GW_HWNDPREV)
		t.Logf("DIAG target=0x%x root=0x%x visible=%v rect=%d,%d,%d,%d raw=0x%x rawRoot=0x%x prevOfTarget=0x%x overlay=0x%x",
			targetHWND, targetRoot, isWndVisible(targetHWND), tr.Left, tr.Top, tr.Right, tr.Bottom,
			raw, platformGetRootWindow(raw), prevOfTarget, ctrl.overlay.hwnd)
		t.Fatalf("platformWindowIDAtPoint below overlay = 0x%x, expected targetRoot 0x%x", hitBelow, targetRoot)
	}

	// Evaluate candidate trajectory and count target hits
	cand := ctrl.bestMotionCandidate(Pt{X: 1250, Y: 250}, Pt{X: 1350, Y: 350})
	pts := cand.Path.SampledConstraintPoints(10)
	targetHits := 0
	for _, pt := range pts {
		wnd := platformWindowIDAtPoint(pt, ctrl.overlay.hwnd)
		if wnd == targetRoot {
			targetHits++
		}
	}

	if targetHits == 0 {
		t.Fatalf("bestMotionCandidate targetHits = 0 when overlay is visible over normal target!")
	}
}

func TestCandidateScoringWithVisibleOverlayOverTopmostTarget(t *testing.T) {
	runWin32Scenario(t, candidateScoringWithVisibleOverlayOverTopmostTargetScenario)
}

func candidateScoringWithVisibleOverlayOverTopmostTargetScenario(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestTopmostScoringClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   procDefWindowProcW.Addr(),
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
	defer destroyTestWindow(targetHWND)

	procSetWindowPos.Call(targetHWND, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_SHOWWINDOW)
	settleWin32(30 * time.Millisecond)

	targetRoot := platformGetRootWindow(targetHWND)

	ctrl := NewVisualCursorController()
	ctrl.SetTargetHWND(targetHWND)
	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay creation unavailable")
	}
	defer ctrl.Reset()

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
		t.Fatalf("bestMotionCandidate targetHits = 0 when overlay is visible over topmost target!")
	}
}

func TestOverlayTransitionTopmostToNormalTarget(t *testing.T) {
	runWin32Scenario(t, overlayTransitionTopmostToNormalTargetScenario)
}

func overlayTransitionTopmostToNormalTargetScenario(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestTransitionClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   procDefWindowProcW.Addr(),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Target A = TOPMOST window at (300, 300, 400, 400)
	targetA, _, _ := procCreateWindowExW.Call(
		WS_EX_TOPMOST, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
		300, 300, 400, 400, 0, 0, hInst, 0,
	)
	if targetA == 0 {
		t.Skip("Could not create targetA window")
	}
	defer destroyTestWindow(targetA)

	// Target B = normal non-topmost window at (300, 300, 400, 400)
	targetB, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP,
		300, 300, 400, 400, 0, 0, hInst, 0,
	)
	if targetB == 0 {
		t.Skip("Could not create targetB window")
	}
	defer destroyTestWindow(targetB)

	procShowWindow.Call(targetB, 8) // SW_SHOWNA
	settleWin32(30 * time.Millisecond)

	ctrl := NewVisualCursorController()
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay creation unavailable")
	}
	defer ctrl.Reset()

	procGetWindowLongW := user32.NewProc("GetWindowLongW")

	// 1. Target A = TOPMOST
	ctrl.SetTargetHWND(targetA)
	ctrl.MoveToAndWait(350, 350)
	settleWin32(30 * time.Millisecond)

	exStyleA, _, _ := procGetWindowLongW.Call(ctrl.overlay.hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	if exStyleA&WS_EX_TOPMOST == 0 {
		t.Fatalf("Overlay missing WS_EX_TOPMOST style when target A is topmost: exStyle = 0x%x", exStyleA)
	}

	// 2. Target B = normal non-topmost
	ctrl.SetTargetHWND(targetB)
	ctrl.MoveToAndWait(360, 360)
	settleWin32(30 * time.Millisecond)

	exStyleB, _, _ := procGetWindowLongW.Call(ctrl.overlay.hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	if exStyleB&WS_EX_TOPMOST != 0 {
		t.Fatalf("Overlay STILL HAS WS_EX_TOPMOST style after transitioning to normal target B: exStyle = 0x%x", exStyleB)
	}

	targetBRoot := platformGetRootWindow(targetB)
	pB, _, _ := procGetWindow.Call(targetBRoot, GW_HWNDPREV)
	if pB != ctrl.overlay.hwnd {
		t.Fatalf("GetWindow(targetB, GW_HWNDPREV) = 0x%x, expected overlay 0x%x", pB, ctrl.overlay.hwnd)
	}
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
	defer ctrl.Reset()

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
	defer ctrl.Reset()

	exStyle, _, _ := procGetWindowLongW.Call(ctrl.overlay.hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	if exStyle&WS_EX_TRANSPARENT == 0 {
		t.Fatalf("Overlay window missing WS_EX_TRANSPARENT style: exStyle = 0x%x", exStyle)
	}

	if exStyle&WS_EX_NOACTIVATE == 0 {
		t.Fatalf("Overlay window missing WS_EX_NOACTIVATE style: exStyle = 0x%x", exStyle)
	}
}

// TestForegroundWindowStability proves that no overlay operation causes the
// overlay window to become the foreground window. The assertion is a positive
// proof (overlay.hwnd != GetForegroundWindow) at each operation, replacing the
// fragile before==during==after check that can fail when any other app on the
// desktop legitimately changes the foreground.
func TestForegroundWindowStability(t *testing.T) {
	procGetForegroundWindow := user32.NewProc("GetForegroundWindow")

	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("OPEN_COMPUTER_USE_VISUAL_CURSOR disabled")
	}
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay creation unavailable")
	}
	defer ctrl.Reset()

	checkNotForeground := func(op string) {
		t.Helper()
		fg, _, _ := procGetForegroundWindow.Call()
		if fg == ctrl.overlay.hwnd {
			t.Fatalf("Overlay became foreground window during %s: overlay=0x%x", op, ctrl.overlay.hwnd)
		}
	}

	ctrl.MoveToAndWait(400, 400)
	checkNotForeground("MoveToAndWait")

	ctrl.PulseClickAndWait(400, 400, 1, "left")
	checkNotForeground("PulseClickAndWait")

	ctrl.Settle(400, 400)
	checkNotForeground("Settle")

	ctrl.SetTargetHWND(0)
	checkNotForeground("SetTargetHWND")
}
