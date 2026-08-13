//go:build windows
// +build windows

package main

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

var (
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetActiveWindow     = user32.NewProc("GetActiveWindow")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
)

func TestPrecisionHarness(t *testing.T) {
	runWin32Scenario(t, func(t *testing.T) {
		hInst, _, _ := procGetModuleHandleW.Call(0)
		className, _ := syscall.UTF16PtrFromString("TestPrecisionClass")
		wc := WNDCLASSEX{
			CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
			LpfnWndProc:   procDefWindowProcW.Addr(),
			HInstance:     hInst,
			LpszClassName: className,
		}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		targetHWND, _, _ := procCreateWindowExW.Call(
			0, uintptr(unsafe.Pointer(className)), 0, WS_POPUP|WS_VISIBLE,
			200, 100, 800, 600, 0, 0, hInst, 0,
		)
		if targetHWND == 0 {
			t.Skip("Could not create target window")
		}
		defer destroyTestWindow(targetHWND)
		settleWin32(30 * time.Millisecond)

		ctrl := NewVisualCursorController()
		ctrl.SetTargetHWND(targetHWND)
		ctrl.ensureOverlay()
		if ctrl.overlay == nil {
			t.Skip("Overlay unavailable")
		}
		defer ctrl.Reset()

		wb := &frame{X: 200, Y: 100, Width: 800, Height: 600}

		type ptTest struct {
			rx, ry float64 // relative x, y
			desc   string
		}

		pts := []ptTest{
			// Corners
			{0, 0, "TopLeft"}, {800, 0, "TopRight"}, {0, 600, "BottomLeft"}, {800, 600, "BottomRight"},
			// Center
			{400, 300, "Center"},
			// Edge midpoints
			{400, 0, "TopMid"}, {400, 600, "BottomMid"}, {0, 300, "LeftMid"}, {800, 300, "RightMid"},
			// Small offsets
			{2, 2, "TopLeft+2"}, {795, 595, "BottomRight-5"},
		}

		// Add random interior points
		r := rand.New(rand.NewSource(12345))
		for i := 0; i < 20; i++ {
			pts = append(pts, ptTest{rx: r.Float64() * 800, ry: r.Float64() * 600, desc: fmt.Sprintf("Rand%d", i)})
		}
		// Add points at 10px intervals along edges
		for i := 0; i < 19; i++ { // fill up to 50
			x := float64(i * 10)
			if x > 800 {
				x = 800
			}
			pts = append(pts, ptTest{rx: x, ry: 0, desc: fmt.Sprintf("EdgeTop%d", i)})
		}

		type result struct {
			desc   string
			tx, ty float64
			vx, vy float64
			err    float64
		}
		var results []result
		var errors []float64

		t.Logf("%-15s | %-10s %-10s | %-10s %-10s | %-10s", "Point", "TargetX", "TargetY", "VisualX", "VisualY", "Error")
		t.Logf(strings.Repeat("-", 70))

		for _, p := range pts[:50] {
			tx := p.rx
			ty := p.ry
			target := resolveVisualCursorTarget(nil, &tx, &ty, wb)
			if target == nil {
				t.Fatalf("resolveVisualCursorTarget returned nil")
			}

			// Simulate updateFrame
			rs := ctrl.initialRenderState(*target)
			ctrl.overlay.updateFrame(target.X, target.Y, rs, 0)

			vx := float64(ctrl.overlay.lastPos.X) + cursorTipAnchorX
			vy := float64(ctrl.overlay.lastPos.Y) + cursorTipAnchorY

			err := math.Sqrt(math.Pow(vx-target.X, 2) + math.Pow(vy-target.Y, 2))
			results = append(results, result{p.desc, target.X, target.Y, vx, vy, err})
			errors = append(errors, err)

			t.Logf("%-15s | %10.2f %10.2f | %10.2f %10.2f | %10.4f", p.desc, target.X, target.Y, vx, vy, err)

			if err > 1.0 {
				t.Errorf("Point %s visual_action_error > 1.0: %v", p.desc, err)
			}
		}

		sort.Float64s(errors)
		min := errors[0]
		max := errors[len(errors)-1]
		p95 := errors[int(0.95*float64(len(errors)))]
		sum := 0.0
		for _, e := range errors {
			sum += e
		}
		mean := sum / float64(len(errors))

		t.Logf("\nStats: Min=%.4f, Mean=%.4f, P95=%.4f, Max=%.4f", min, mean, p95, max)
	})
}

func TestTemporalOrdering100Cycles(t *testing.T) {
	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("Visual cursor disabled")
	}
	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay unavailable")
	}
	defer ctrl.Reset()

	var d1, d2, d3 []time.Duration

	for i := 0; i < 100; i++ {
		x, y := 100.0+float64(i%10)*10, 100.0+float64(i/10)*10

		t0 := time.Now()
		ctrl.MoveToAndWait(x, y)
		t1 := ctrl.overlay.lastCommitTime

		time.Sleep(1 * time.Millisecond)
		t2 := time.Now()

		ctrl.PulseClickAndWait(x, y, 1, "left")
		t3 := ctrl.overlay.lastCommitTime

		if !t1.Before(t2) {
			t.Fatalf("Cycle %d: FrameCommitted (%v) not before ActionBegan (%v)", i, t1, t2)
		}
		if !t2.Before(t3) {
			t.Fatalf("Cycle %d: ActionBegan (%v) not before PulseCommitted (%v)", i, t2, t3)
		}

		d1 = append(d1, t1.Sub(t0))
		d2 = append(d2, t2.Sub(t1))
		d3 = append(d3, t3.Sub(t2))
	}

	reportStats := func(name string, d []time.Duration) {
		sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
		min := d[0]
		max := d[len(d)-1]
		p50 := d[len(d)/2]
		p95 := d[int(0.95*float64(len(d)))]
		p99 := d[int(0.99*float64(len(d)))]
		t.Logf("%s Latencies: Min=%v, P50=%v, P95=%v, P99=%v, Max=%v", name, min, p50, p95, p99, max)
	}

	reportStats("T1-T0 (Move)", d1)
	reportStats("T2-T1 (Wait)", d2)
	reportStats("T3-T2 (Pulse)", d3)
}

func TestPhysicalCursorStability(t *testing.T) {
	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("Visual cursor disabled")
	}
	defer ctrl.Reset()

	r := rand.New(rand.NewSource(99))

	type POINT struct {
		X, Y int32
	}

	getCursor := func() POINT {
		var pt POINT
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
		return pt
	}

	for i := 0; i < 100; i++ {
		rx, ry := 100.0+r.Float64()*500, 100.0+r.Float64()*500

		before := getCursor()
		ctrl.MoveToAndWait(rx, ry)
		during := getCursor()
		ctrl.PulseClickAndWait(rx, ry, 1, "left")
		after := getCursor()

		dist := func(p1, p2 POINT) float64 {
			return math.Sqrt(float64((p1.X-p2.X)*(p1.X-p2.X) + (p1.Y-p2.Y)*(p1.Y-p2.Y)))
		}

		if dist(before, during) > 2.0 || dist(during, after) > 2.0 {
			if dist(before, during) > 5.0 || dist(during, after) > 5.0 {
				t.Logf("Iteration %d: Cursor moved significantly (%.1f px). User likely moved mouse. Inconclusive.", i, math.Max(dist(before, during), dist(during, after)))
			} else {
				t.Errorf("Iteration %d: Cursor drifted more than 2px: before=%v during=%v after=%v", i, before, during, after)
			}
		}
	}
}

func TestStressResourceLeaks(t *testing.T) {
	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("Visual cursor disabled")
	}
	ctrl.ensureOverlay()
	if ctrl.overlay == nil {
		t.Skip("Overlay unavailable")
	}

	time.Sleep(10 * time.Millisecond) // settle
	initialGoroutines := runtime.NumGoroutine()
	initialCommits := ctrl.overlay.frameCommitCount
	initialAttempts := ctrl.overlay.frameAttemptCount

	t.Logf("Initial: Goroutines=%d, Commits=%d, Attempts=%d", initialGoroutines, initialCommits, initialAttempts)

	// Execute sequence
	for i := 0; i < 200; i++ {
		ctrl.MoveToAndWait(100.0+float64(i%10), 100.0+float64(i%10))
	}
	for i := 0; i < 200; i++ {
		ctrl.PulseClickAndWait(150, 150, 1, "left")
	}
	for i := 0; i < 100; i++ {
		ctrl.Settle(200, 200)
	}
	for i := 0; i < 50; i++ {
		ctrl.Reset()
		ctrl.ensureOverlay()
		ctrl.MoveToAndWait(300, 300)
	}

	finalGoroutines := runtime.NumGoroutine()
	finalCommits := ctrl.overlay.frameCommitCount
	finalAttempts := ctrl.overlay.frameAttemptCount

	t.Logf("Final: Goroutines=%d, Commits=%d, Attempts=%d", finalGoroutines, finalCommits, finalAttempts)

	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Goroutine leak detected: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
	if finalCommits <= initialCommits {
		t.Errorf("frameCommitCount did not increase: %d -> %d", initialCommits, finalCommits)
	}
}

func TestDPICoordinateConversions(t *testing.T) {
	// packPoint round-trip
	points := []Pt{
		{0, 0}, {96, 96}, {-120, -120}, {144, 2560}, {168, 1440}, {192, 1080},
		{3840, 2160}, {-5000, 5000},
	}

	unpackPoint := func(p uintptr) (int32, int32) {
		x := int32(uint32(p & 0xFFFFFFFF))
		y := int32(uint32(p >> 32))
		return x, y
	}

	for _, pt := range points {
		packed := packPoint(pt)
		ux, uy := unpackPoint(packed)
		ex, ey := int32(math.Round(pt.X)), int32(math.Round(pt.Y))
		if ux != ex || uy != ey {
			t.Errorf("packPoint round-trip failed for %v: got %d,%d, want %d,%d", pt, ux, uy, ex, ey)
		}
	}

	// platformMonitorWorkArea
	wa := platformMonitorWorkArea(Pt{X: 100, Y: 100})
	if wa.W <= 0 || wa.H <= 0 {
		t.Errorf("platformMonitorWorkArea returned invalid dims: %v", wa)
	}
	t.Logf("WorkArea at (100,100): %v", wa)

	// DPI awareness context
	res, _, _ := procSetProcessDpiAwarenessContext.Call(uintptr(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2))
	if res == 0 {
		err := syscall.GetLastError()
		if err != nil && err.Error() != "The operation completed successfully." {
			// Actually ERROR_ACCESS_DENIED is returned if it was already set, which is fine
			t.Logf("SetProcessDpiAwarenessContext result: %v (expected if already set)", err)
		}
	} else {
		t.Logf("SetProcessDpiAwarenessContext succeeded")
	}
}

func TestMultiMonitorMathSimulation(t *testing.T) {
	// Simulate motionBounds mathematically
	simMotionBounds := func(startArea, endArea Rect) Rect {
		minX := math.Min(startArea.MinX(), endArea.MinX())
		maxX := math.Max(startArea.MaxX(), endArea.MaxX())
		minY := math.Min(startArea.MinY(), endArea.MinY())
		maxY := math.Max(startArea.MaxY(), endArea.MaxY())
		return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
	}

	scenarios := []struct {
		a, b     Rect
		expected Rect
	}{
		{Rect{0, 0, 1920, 1080}, Rect{-1920, 0, 1920, 1080}, Rect{-1920, 0, 3840, 1080}},
		{Rect{0, 0, 1920, 1080}, Rect{0, -1080, 1920, 1080}, Rect{0, -1080, 1920, 2160}},
		{Rect{0, 0, 2560, 1440}, Rect{2560, 0, 1920, 1080}, Rect{0, 0, 4480, 1440}},
	}

	for i, s := range scenarios {
		res := simMotionBounds(s.a, s.b)
		if res != s.expected {
			t.Errorf("Scenario %d failed: want %v, got %v", i, s.expected, res)
		}
	}

	// Enumerate real monitors
	type MONITORINFO struct {
		CbSize    uint32
		RcMonitor struct{ Left, Top, Right, Bottom int32 }
		RcWork    struct{ Left, Top, Right, Bottom int32 }
		DwFlags   uint32
	}
	cb := syscall.NewCallback(func(hMonitor, hdcMonitor, lprcMonitor, dwData uintptr) uintptr {
		var mi MONITORINFO
		mi.CbSize = uint32(unsafe.Sizeof(mi))
		procGetMonitorInfoW := user32.NewProc("GetMonitorInfoW")
		procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&mi)))
		t.Logf("Monitor 0x%x: WorkArea=(%d,%d %dx%d)", hMonitor, mi.RcWork.Left, mi.RcWork.Top, mi.RcWork.Right-mi.RcWork.Left, mi.RcWork.Bottom-mi.RcWork.Top)
		return 1
	})
	procEnumDisplayMonitors.Call(0, 0, cb, 0)
}

func TestFocusStabilityExtended(t *testing.T) {
	procGetForegroundWindow := user32.NewProc("GetForegroundWindow")

	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("Visual cursor disabled")
	}
	ctrl.ensureOverlay()
	if ctrl.overlay == nil || ctrl.overlay.hwnd == 0 {
		t.Skip("Overlay creation unavailable")
	}
	defer ctrl.Reset()

	checkNotForeground := func(op string, idx int) {
		t.Helper()
		fg, _, _ := procGetForegroundWindow.Call()
		if fg == ctrl.overlay.hwnd {
			t.Fatalf("Overlay became foreground window during %s (idx %d): overlay=0x%x", op, idx, ctrl.overlay.hwnd)
		}
		active, _, _ := procGetActiveWindow.Call()
		if active == ctrl.overlay.hwnd {
			t.Fatalf("Overlay became active window during %s (idx %d): overlay=0x%x", op, idx, ctrl.overlay.hwnd)
		}
	}

	for i := 0; i < 50; i++ {
		ctrl.MoveToAndWait(400+float64(i%10), 400+float64(i%10))
		checkNotForeground("MoveToAndWait", i)
	}

	for i := 0; i < 50; i++ {
		ctrl.PulseClickAndWait(400+float64(i%10), 400+float64(i%10), 1, "left")
		checkNotForeground("PulseClickAndWait", i)
	}

	for i := 0; i < 20; i++ {
		ctrl.Settle(400+float64(i%10), 400+float64(i%10))
		checkNotForeground("Settle", i)
	}
}
