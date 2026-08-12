//go:build windows

package main

import (
	"image"
	"runtime"
	"sync"
	"syscall"
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

	HWND_TOPMOST   = ^uintptr(0) // -1
	SWP_NOSIZE     = 0x0001
	SWP_NOMOVE     = 0x0002
	SWP_NOACTIVATE = 0x0010
	SW_SHOW        = 5
	SW_HIDE        = 0

	ULW_ALPHA    = 0x00000002
	AC_SRC_OVER  = 0x00
	AC_SRC_ALPHA = 0x01

	WM_USER = 0x0400
	WM_QUIT = 0x0012
)

type POINT struct {
	X, Y int32
}

type SIZE struct {
	CX, CY int32
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

type overlayWindow struct {
	hwnd        uintptr
	memDC       uintptr
	memBitmap   uintptr
	oldBitmap   uintptr
	pixels      unsafe.Pointer
	cursorImage *image.NRGBA
	width       int32
	height      int32

	cmdChan chan func()
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
			cursorImage: loadCursorImage(),
			cmdChan:     make(chan func(), 64),
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
			case fn := <-w.cmdChan:
				fn()
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

func (w *overlayWindow) postCmd(fn func()) {
	if w == nil || w.hwnd == 0 {
		return
	}
	w.cmdChan <- fn
	procPostMessageW.Call(w.hwnd, WM_USER, 0, 0)
}

func (w *overlayWindow) updateFrame(screenX, screenY float64, renderState VisualRenderState, clickProgress float64) {
	w.postCmd(func() {
		renderCursorToDIB(w.pixels, w.width, w.height, w.cursorImage, renderState, clickProgress)

		tipAnchorX, tipAnchorY := 60.35, 70.3
		left := int32(screenX - tipAnchorX)
		top := int32(screenY - tipAnchorY)

		ptDst := POINT{X: left, Y: top}
		sizeDst := SIZE{CX: w.width, CY: w.height}
		ptSrc := POINT{X: 0, Y: 0}
		blend := BLENDFUNCTION{
			BlendOp:             AC_SRC_OVER,
			BlendFlags:          0,
			SourceConstantAlpha: 255,
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

func (w *overlayWindow) setZOrder(targetHWND uintptr) {
	w.postCmd(func() {
		hwndInsertAfter := HWND_TOPMOST
		if targetHWND != 0 {
			hwndInsertAfter = targetHWND
		}
		procSetWindowPos.Call(
			w.hwnd,
			hwndInsertAfter,
			0, 0, 0, 0,
			SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE,
		)
	})
}

func (w *overlayWindow) show() {
	w.postCmd(func() {
		procShowWindow.Call(w.hwnd, SW_SHOW)
	})
}

func (w *overlayWindow) hide() {
	w.postCmd(func() {
		procShowWindow.Call(w.hwnd, SW_HIDE)
	})
}

func (w *overlayWindow) fadeOut(durationMs int) {
	w.postCmd(func() {
		procShowWindow.Call(w.hwnd, SW_HIDE)
	})
}

func (w *overlayWindow) destroy() {
	w.postCmd(func() {
		procPostQuitMessage.Call(0)
	})
}
