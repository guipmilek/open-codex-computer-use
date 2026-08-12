//go:build !windows

package main

type overlayWindow struct{}

func platformCreateOverlay() *overlayWindow { return nil }
func (w *overlayWindow) updateFrame(screenX, screenY float64, renderState VisualRenderState, clickProgress float64) {
}
func (w *overlayWindow) setZOrder(targetHWND uintptr) {}
func (w *overlayWindow) show()                        {}
func (w *overlayWindow) hide()                        {}
func (w *overlayWindow) fadeOut(durationMs int)       {}
func (w *overlayWindow) destroy()                     {}
