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
