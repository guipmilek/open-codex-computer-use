package main

import (
	"testing"
)

func TestResolveVisualCursorTargetElement(t *testing.T) {
	wb := &frame{X: 100, Y: 200, Width: 800, Height: 600}
	el := &elementRecord{
		Index: 3,
		Frame: &frame{X: 50, Y: 40, Width: 100, Height: 60},
	}

	target := resolveVisualCursorTarget(el, nil, nil, wb)
	if target == nil {
		t.Fatal("resolveVisualCursorTarget returned nil for element")
	}

	// Target screen coordinate: windowX + elementX + width/2 = 100 + 50 + 50 = 200
	// windowY + elementY + height/2 = 200 + 40 + 30 = 270
	expectedX := 200.0
	expectedY := 270.0

	if target.X != expectedX || target.Y != expectedY {
		t.Fatalf("Target = (%v, %v), expected (%v, %v)", target.X, target.Y, expectedX, expectedY)
	}
}

func TestResolveVisualCursorTargetCoordinates(t *testing.T) {
	wb := &frame{X: 150, Y: 250, Width: 1000, Height: 800}
	x := 400.0
	y := 300.0

	target := resolveVisualCursorTarget(nil, &x, &y, wb)
	if target == nil {
		t.Fatal("resolveVisualCursorTarget returned nil for x/y")
	}

	// Screenshot coordinates are relative to windowBounds.origin
	expectedX := 150.0 + 400.0 // 550
	expectedY := 250.0 + 300.0 // 550

	if target.X != expectedX || target.Y != expectedY {
		t.Fatalf("Target = (%v, %v), expected (%v, %v)", target.X, target.Y, expectedX, expectedY)
	}
}

func TestTargetHWNDFromSnapshot(t *testing.T) {
	snapshot := &appSnapshot{
		Elements: []elementRecord{
			{Index: 0, NativeWindowHandle: 0x123456},
			{Index: 1, NativeWindowHandle: 0},
		},
	}

	hwnd := targetHWNDFromSnapshot(snapshot)
	if hwnd != 0x123456 {
		t.Fatalf("targetHWNDFromSnapshot = 0x%x, expected 0x123456", hwnd)
	}

	if targetHWNDFromSnapshot(nil) != 0 {
		t.Fatal("targetHWNDFromSnapshot(nil) expected 0")
	}
}
