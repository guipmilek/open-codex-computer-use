//go:build windows

package main

import (
	"image"
	"math"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procGetWindow           = user32.NewProc("GetWindow")
	procIsWindow            = user32.NewProc("IsWindow")
	procWindowFromPoint     = user32.NewProc("WindowFromPoint")
	procMonitorFromPoint    = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")

	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
	WS_EX_LAYERED     = 0x00080000
	WS_EX_TRANSPARENT = 0x00000020
	WS_EX_NOACTIVATE  = 0x08000000
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_POPUP          = 0x80000000

	HWND_TOP       = 0
	HWND_TOPMOST   = ^uintptr(0) // -1
	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010
	SWP_SHOWWINDOW = 0x0040
	SW_SHOWNA      = 8
	SW_SHOW        = 5
	SW_HIDE        = 0

	ULW_ALPHA    = 0x00000002
	AC_SRC_OVER  = 0x00
	AC_SRC_ALPHA = 0x01

	WM_USER = 0x0400
	WM_QUIT = 0x0012

	GW_HWNDPREV = 3

	MONITOR_DEFAULTTONEAREST = 2
)

type POINT struct {
	X, Y int32
}

type SIZE struct {
	CX, CY int32
}

type RECT struct {
	Left, Top, Right, Bottom int32
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type BLENDFUNCTION struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

type MSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      POINT
}

type overlayCmd struct {
	fn   func()
	done chan struct{}
}

type overlayWindow struct {
	hwnd        uintptr
	memDC       uintptr
	memBitmap   uintptr
	oldBitmap   uintptr
	pixels      unsafe.Pointer
	cursorImage *image.NRGBA
	width       int32
	height      int32
	lastPos     POINT
	lastAlpha   byte

	cmdChan chan overlayCmd
}

var (
	overlayInstance *overlayWindow
	initOnce        sync.Once
)

func platformCreateOverlay() *overlayWindow {
	initOnce.Do(func() {
		overlayInstance = &overlayWindow{
			width:       126,
			height:      126,
			lastAlpha:   255,
			cursorImage: loadCursorImage(),
			cmdChan:     make(chan overlayCmd, 64),
		}

		ready := make(chan bool)
		go overlayInstance.runMessageLoop(ready)
		if !<-ready {
			overlayInstance = nil
		}
	})
	return overlayInstance
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	if msg == WM_QUIT {
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}

func (w *overlayWindow) runMessageLoop(ready chan bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("CursorOverlayClass")

	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInst,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	hwnd, _, _ := procCreateWindowExW.Call(
		WS_EX_LAYERED|WS_EX_TRANSPARENT|WS_EX_NOACTIVATE|WS_EX_TOOLWINDOW,
		uintptr(unsafe.Pointer(className)),
		0,
		WS_POPUP,
		0, 0, uintptr(w.width), uintptr(w.height),
		0, 0, hInst, 0,
	)

	if hwnd == 0 {
		ready <- false
		return
	}
	w.hwnd = hwnd

	screenDC, _, _ := procGetDC.Call(0)
	w.memDC, _, _ = procCreateCompatibleDC.Call(screenDC)
	procReleaseDC.Call(0, screenDC)

	bmi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
			BiWidth:       w.width,
			BiHeight:      -w.height,
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: 0,
		},
	}

	w.memBitmap, _, _ = procCreateDIBSection.Call(
		w.memDC,
		uintptr(unsafe.Pointer(&bmi)),
		0,
		uintptr(unsafe.Pointer(&w.pixels)),
		0, 0,
	)

	w.oldBitmap, _, _ = procSelectObject.Call(w.memDC, w.memBitmap)

	ready <- true

	var msg MSG
	for {
		for {
			select {
			case cmd := <-w.cmdChan:
				if cmd.fn != nil {
					cmd.fn()
				}
				if cmd.done != nil {
					close(cmd.done)
				}
			default:
				goto checkMessages
			}
		}
	checkMessages:
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	procSelectObject.Call(w.memDC, w.oldBitmap)
	procDeleteObject.Call(w.memBitmap)
	procDeleteDC.Call(w.memDC)
	procDestroyWindow.Call(w.hwnd)
}

// postCmdSync posts a command and synchronously blocks until the Win32 thread
// finishes executing it and committing the frame.
func (w *overlayWindow) postCmdSync(fn func()) {
	if w == nil || w.hwnd == 0 {
		return
	}
	done := make(chan struct{})
	w.cmdChan <- overlayCmd{fn: fn, done: done}
	procPostMessageW.Call(w.hwnd, WM_USER, 0, 0)
	<-done
}

func (w *overlayWindow) updateFrame(screenX, screenY float64, renderState VisualRenderState, clickProgress float64) {
	w.postCmdSync(func() {
		renderCursorToDIB(w.pixels, w.width, w.height, w.cursorImage, renderState, clickProgress)

		tipAnchorX, tipAnchorY := cursorTipAnchorX, cursorTipAnchorY
		left := int32(math.Round(screenX - tipAnchorX))
		top := int32(math.Round(screenY - tipAnchorY))
		w.lastPos = POINT{X: left, Y: top}

		ptDst := w.lastPos
		sizeDst := SIZE{CX: w.width, CY: w.height}
		ptSrc := POINT{X: 0, Y: 0}
		blend := BLENDFUNCTION{
			BlendOp:             AC_SRC_OVER,
			BlendFlags:          0,
			SourceConstantAlpha: w.lastAlpha,
			AlphaFormat:         AC_SRC_ALPHA,
		}

		procUpdateLayeredWindow.Call(
			w.hwnd,
			0,
			uintptr(unsafe.Pointer(&ptDst)),
			uintptr(unsafe.Pointer(&sizeDst)),
			w.memDC,
			uintptr(unsafe.Pointer(&ptSrc)),
			0,
			uintptr(unsafe.Pointer(&blend)),
			ULW_ALPHA,
		)
	})
}

// setZOrder places the overlay immediately above targetHWND in Win32 Z-order.
// In Win32 SetWindowPos semantics, inserting after `hWndInsertAfter` places
// the window BELOW `hWndInsertAfter`. Thus, to place `hwnd` immediately ABOVE
// `targetHWND`, `hWndInsertAfter` must be the predecessor of `targetHWND`
// (`GetWindow(targetHWND, GW_HWNDPREV)`).
func (w *overlayWindow) setZOrder(targetHWND uintptr) {
	w.postCmdSync(func() {
		var hwndInsertAfter uintptr = HWND_TOP
		if targetHWND != 0 {
			res, _, _ := procIsWindow.Call(targetHWND)
			if res != 0 {
				prev, _, _ := procGetWindow.Call(targetHWND, GW_HWNDPREV)
				if prev != 0 {
					hwndInsertAfter = prev
				} else {
					hwndInsertAfter = HWND_TOP
				}
			}
		}
		procSetWindowPos.Call(
			w.hwnd,
			hwndInsertAfter,
			0, 0, 0, 0,
			SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
		)
	})
}

// show shows the overlay using SWP_SHOWWINDOW and SWP_NOACTIVATE (never activates or steals focus).
func (w *overlayWindow) show() {
	w.postCmdSync(func() {
		procSetWindowPos.Call(
			w.hwnd,
			0,
			0, 0, 0, 0,
			SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE|SWP_SHOWWINDOW,
		)
	})
}

// hide hides the overlay immediately.
func (w *overlayWindow) hide() {
	w.postCmdSync(func() {
		procShowWindow.Call(w.hwnd, SW_HIDE)
		w.lastAlpha = 255
	})
}

// fadeOut animates alpha from 255 down to 0 over durationMs (120ms) with easing,
// then hides the window.
func (w *overlayWindow) fadeOut(durationMs int) {
	w.postCmdSync(func() {
		steps := 12
		stepDuration := time.Duration(durationMs/steps) * time.Millisecond

		ptDst := w.lastPos
		sizeDst := SIZE{CX: w.width, CY: w.height}
		ptSrc := POINT{X: 0, Y: 0}

		for i := steps; i >= 0; i-- {
			alpha := byte((i * 255) / steps)
			w.lastAlpha = alpha

			blend := BLENDFUNCTION{
				BlendOp:             AC_SRC_OVER,
				BlendFlags:          0,
				SourceConstantAlpha: alpha,
				AlphaFormat:         AC_SRC_ALPHA,
			}

			procUpdateLayeredWindow.Call(
				w.hwnd,
				0,
				uintptr(unsafe.Pointer(&ptDst)),
				uintptr(unsafe.Pointer(&sizeDst)),
				w.memDC,
				uintptr(unsafe.Pointer(&ptSrc)),
				0,
				uintptr(unsafe.Pointer(&blend)),
				ULW_ALPHA,
			)

			time.Sleep(stepDuration)
		}

		procShowWindow.Call(w.hwnd, SW_HIDE)
		w.lastAlpha = 255
	})
}

func (w *overlayWindow) destroy() {
	w.postCmdSync(func() {
		procPostQuitMessage.Call(0)
	})
}

// --- Win32 Monitor & Work Area Helpers ---

func platformMonitorWorkArea(pt Pt) Rect {
	winPt := POINT{X: int32(math.Round(pt.X)), Y: int32(math.Round(pt.Y))}
	hMon, _, _ := procMonitorFromPoint.Call(
		uintptr(winPt.X),
		uintptr(winPt.Y),
		MONITOR_DEFAULTTONEAREST,
	)

	if hMon == 0 {
		return Rect{X: 0, Y: 0, W: 1920, H: 1080}
	}

	var mi MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	res, _, _ := procGetMonitorInfoW.Call(hMon, uintptr(unsafe.Pointer(&mi)))
	if res == 0 {
		return Rect{X: 0, Y: 0, W: 1920, H: 1080}
	}

	return Rect{
		X: float64(mi.RcWork.Left),
		Y: float64(mi.RcWork.Top),
		W: float64(mi.RcWork.Right - mi.RcWork.Left),
		H: float64(mi.RcWork.Bottom - mi.RcWork.Top),
	}
}

func platformWindowIDAtPoint(pt Pt) uintptr {
	winPt := POINT{X: int32(math.Round(pt.X)), Y: int32(math.Round(pt.Y))}
	hwnd, _, _ := procWindowFromPoint.Call(uintptr(winPt.X), uintptr(winPt.Y))
	return hwnd
}
