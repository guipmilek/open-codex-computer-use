//go:build windows

package main

import (
	"math"
	"testing"
	"time"
)

var procGetForegroundWindowEdge = user32.NewProc("GetForegroundWindow")
var procMonitorFromPointEdge = user32.NewProc("MonitorFromPoint")

func TestVisualCursorTipMatchesActionAtMonitorEdges(t *testing.T) {
	if monitorCount() < 2 {
		t.Skip("Skipping multi-monitor real test: requires at least 2 physical monitors")
	}
	if !hasNegativeMonitor() {
		t.Skip("Skipping multi-monitor real test: requires a monitor with negative coordinates")
	}

	t.Log("Beginning Real Edge Precision Validation")

	ctrl := NewVisualCursorController()

	runWin32Scenario(t, func(t *testing.T) {
		t.Log("--- DISPLAY7 EDGES TESTS ---")

		d7w := 2560.0
		d7h := 1080.0

		ptsD7 := []struct {
			desc string
			X, Y float64
		}{
			{"Center", 1280, 540},
			{"Top-Left 0px", 0, 0},
			{"Top-Right 0px", d7w - 1, 0},
			{"Bottom-Left 0px", 0, d7h - 1},
			{"Bottom-Right 0px", d7w - 1, d7h - 1},
			{"Top 5px", 1280, 5},
			{"Left 5px", 5, 540},
			{"Bottom 5px", 1280, d7h - 6},
			{"Right 5px", d7w - 6, 540},
			{"Outside-Bottom", 1280, 2000},
			{"Outside-Right", 3000, 540},
		}

		for _, p := range ptsD7 {
			targetX := p.X
			targetY := p.Y

			clampedTarget := ctrl.clampTipPosition(Pt{X: targetX, Y: targetY})

			left := int32(math.Round(clampedTarget.X - cursorTipAnchorX))
			top := int32(math.Round(clampedTarget.Y - cursorTipAnchorY))

			visualX := float64(left) + cursorTipAnchorX
			visualY := float64(top) + cursorTipAnchorY

			distOrig := math.Sqrt((visualX-targetX)*(visualX-targetX) + (visualY-targetY)*(visualY-targetY))

			if distOrig > 1.0 {
				t.Errorf("Visual error > 1px for target %.1f, %.1f", targetX, targetY)
			}
		}

		t.Log("--- DISPLAY6 EDGES TESTS (Negative Y) ---")

		d6w := 2560.0
		ptsD6 := []struct {
			desc string
			X, Y float64
		}{
			{"D6 Center", 1280, -540},
			{"D6 Top-Left 0px", 0, -1080},
			{"D6 Top-Right 0px", d6w - 1, -1080},
			{"D6 Bottom-Left 0px", 0, -1},
			{"D6 Bottom-Right 0px", d6w - 1, -1},
			{"D6 Top 5px", 1280, -1075},
			{"D6 Bottom 5px", 1280, -6},
			{"D6 Left 5px", 5, -540},
			{"D6 Right 5px", d6w - 6, -540},
			{"D6 Outside-Top", 1280, -2000},
			{"D6 Outside-Left", -500, -540},
		}

		for _, p := range ptsD6 {
			targetX := p.X
			targetY := p.Y

			clampedTarget := ctrl.clampTipPosition(Pt{X: targetX, Y: targetY})
			left := int32(math.Round(clampedTarget.X - cursorTipAnchorX))
			top := int32(math.Round(clampedTarget.Y - cursorTipAnchorY))
			visualX := float64(left) + cursorTipAnchorX
			visualY := float64(top) + cursorTipAnchorY

			distOrig := math.Sqrt((visualX-targetX)*(visualX-targetX) + (visualY-targetY)*(visualY-targetY))

			if distOrig > 1.0 {
				t.Errorf("Visual error > 1px for target %.1f, %.1f", targetX, targetY)
			}
		}

		time.Sleep(10 * time.Millisecond)
	})
}
