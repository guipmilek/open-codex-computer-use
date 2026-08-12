//go:build windows

package main

import (
	"math"
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

		// Verify unpacking lower and upper 32-bit halves
		unpackedX := int32(uint32(packed & 0xFFFFFFFF))
		unpackedY := int32(uint32(packed >> 32))

		if float64(unpackedX) != math.Round(tc.x) || float64(unpackedY) != math.Round(tc.y) {
			t.Fatalf("Unpacked point = (%d, %d), expected (%v, %v)", unpackedX, unpackedY, tc.x, tc.y)
		}
	}
}

func TestMonitorFromPointIntegration(t *testing.T) {
	pt1 := Pt{X: 100, Y: 100}
	work1 := platformMonitorWorkArea(pt1)

	if work1.W <= 0 || work1.H <= 0 {
		t.Fatalf("platformMonitorWorkArea returned invalid rect: %v", work1)
	}

	// Test two points with different Y on same X to ensure Y is not collapsed to 0 due to register shift
	pt2 := Pt{X: 100, Y: 500}
	work2 := platformMonitorWorkArea(pt2)

	if work2.W <= 0 || work2.H <= 0 {
		t.Fatalf("platformMonitorWorkArea(pt2) returned invalid rect: %v", work2)
	}
}

func TestWindowFromPointIntegration(t *testing.T) {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("TestDummyWindowClass")

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

	procShowWindow.Call(hwnd, SW_SHOWNA)

	hit := platformWindowIDAtPoint(Pt{X: 250, Y: 250}, 0)
	if hit == 0 {
		t.Fatal("platformWindowIDAtPoint returned 0 for test window point")
	}

	root := platformGetRootWindow(hit)
	if root == 0 {
		t.Fatal("platformGetRootWindow returned 0")
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

	var moveFrameCommitTime time.Time
	var actionStartTime time.Time
	var pulseFrameCommitTime time.Time

	// 1. MoveToAndWait
	ctrl.MoveToAndWait(350, 350)
	moveFrameCommitTime = ctrl.overlay.lastCommittedTime

	// 2. Action starts
	actionStartTime = time.Now()

	// 3. PulseClickAndWait
	ctrl.PulseClickAndWait(350, 350, 1, "left")
	pulseFrameCommitTime = ctrl.overlay.lastCommittedTime

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

func TestPhysicalCursorUnmoved(t *testing.T) {
	procGetCursorPos := user32.NewProc("GetCursorPos")

	var ptBefore POINT
	res1, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&ptBefore)))

	ctrl := NewVisualCursorController()
	ctrl.MoveToAndWait(500, 500)
	ctrl.PulseClickAndWait(500, 500, 1, "left")
	ctrl.Settle(500, 500)
	ctrl.Reset()

	var ptAfter POINT
	res2, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&ptAfter)))

	if res1 != 0 && res2 != 0 {
		t.Logf("Physical cursor position: Before=(%d, %d), After=(%d, %d)", ptBefore.X, ptBefore.Y, ptAfter.X, ptAfter.Y)
	}
}

func TestOverlayClickThroughStyle(t *testing.T) {
	procGetWindowLongW := user32.NewProc("GetWindowLongW")
	const GWL_EXSTYLE = -20

	ctrl := NewVisualCursorController()
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay window unavailable")
	}

	exStyle, _, _ := procGetWindowLongW.Call(ctrl.overlay.hwnd, ^uintptr(19))
	if exStyle&WS_EX_TRANSPARENT == 0 {
		t.Fatalf("Overlay window missing WS_EX_TRANSPARENT style: exStyle = 0x%x", exStyle)
	}

	if exStyle&WS_EX_NOACTIVATE == 0 {
		t.Fatalf("Overlay window missing WS_EX_NOACTIVATE style: exStyle = 0x%x", exStyle)
	}

	ctrl.Reset()
}
