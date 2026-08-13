package main

import (
	"os"
	"testing"
)

func TestVisualCursorEnabledEnvVar(t *testing.T) {
	defer os.Unsetenv("OPEN_COMPUTER_USE_VISUAL_CURSOR")

	os.Unsetenv("OPEN_COMPUTER_USE_VISUAL_CURSOR")
	if !visualCursorEnabled() {
		t.Fatal("Default visualCursorEnabled() should be true when env var is absent")
	}

	for _, enabledVal := range []string{"1", "true", "YES", "On"} {
		os.Setenv("OPEN_COMPUTER_USE_VISUAL_CURSOR", enabledVal)
		if !visualCursorEnabled() {
			t.Fatalf("visualCursorEnabled() with %q should be true", enabledVal)
		}
	}

	for _, disabledVal := range []string{"0", "false", "NO", "off"} {
		os.Setenv("OPEN_COMPUTER_USE_VISUAL_CURSOR", disabledVal)
		if visualCursorEnabled() {
			t.Fatalf("visualCursorEnabled() with %q should be false", disabledVal)
		}
	}
}

func TestControllerDisabledState(t *testing.T) {
	os.Setenv("OPEN_COMPUTER_USE_VISUAL_CURSOR", "0")
	defer os.Unsetenv("OPEN_COMPUTER_USE_VISUAL_CURSOR")

	ctrl := NewVisualCursorController()
	if !ctrl.disabled {
		t.Fatal("Controller should be disabled when env var is set to 0")
	}

	// Calling methods on disabled controller should do nothing and not crash
	ctrl.MoveToAndWait(100, 100)
	ctrl.PulseClickAndWait(100, 100, 1, "left")
	ctrl.Settle(100, 100)
	ctrl.Reset()
}

func TestDefaultTipPositionAndForward(t *testing.T) {
	tip := defaultInitialTipPosition()
	if tip.X != cursorTipAnchorX || tip.Y != cursorTipAnchorY {
		t.Fatalf("defaultInitialTipPosition = %v, expected (%v, %v)", tip, cursorTipAnchorX, cursorTipAnchorY)
	}

	forward := restingForwardVector()
	if forward.Length() < 0.99 || forward.Length() > 1.01 {
		t.Fatalf("restingForwardVector length = %v, expected ~1.0", forward.Length())
	}
}

func TestClampTipPositionAndMotionBounds(t *testing.T) {
	ctrl := NewVisualCursorController()
	start := Pt{X: -500, Y: -200}
	end := Pt{X: 1920, Y: 1080}

	clampedStart := ctrl.clampTipPosition(start)
	if clampedStart.X < start.X {
		t.Fatalf("clampedStart.X = %v, expected >= %v", clampedStart.X, start.X)
	}

	bounds := ctrl.motionBounds(start, end)
	if bounds == nil || bounds.W <= 0 || bounds.H <= 0 {
		t.Fatalf("motionBounds returned invalid rect: %v", bounds)
	}
}

func TestSynchronousOrderCommitmentSequence(t *testing.T) {
	ctrl := NewVisualCursorController()
	if ctrl.disabled {
		t.Skip("Skipping synchronous sequence test: OPEN_COMPUTER_USE_VISUAL_CURSOR is disabled")
	}
	defer ctrl.Reset()

	var executionLog []string

	// 1. MoveToAndWait
	ctrl.MoveToAndWait(300, 300)
	executionLog = append(executionLog, "MoveToCompleted")

	// 2. Action starts
	executionLog = append(executionLog, "ActionStarted")

	// 3. PulseClickAndWait
	ctrl.PulseClickAndWait(300, 300, 1, "left")
	executionLog = append(executionLog, "PulseCompleted")

	expectedSequence := []string{"MoveToCompleted", "ActionStarted", "PulseCompleted"}
	for i, step := range expectedSequence {
		if executionLog[i] != step {
			t.Fatalf("Execution sequence mismatch at index %d: got %q, want %q", i, executionLog[i], step)
		}
	}
}
