package main

import (
	"math"
	"testing"
)

func TestReferenceTransformInverseRotationAndPulse(t *testing.T) {
	width := 126.0
	height := 126.0
	cx := width / 2.0
	cy := height / 2.0

	// 1. Identity transform at center should map center to center
	stateZero := VisualRenderState{
		Rotation:        0,
		CursorBodyOffDx: 0,
		CursorBodyOffDy: 0,
	}
	sx, sy := computeReferenceTransformInverse(cx, cy, width, height, stateZero, 0)
	if math.Abs(sx-cx) > 0.001 || math.Abs(sy-cy) > 0.001 {
		t.Fatalf("Center mapping = (%v, %v), expected (%v, %v)", sx, sy, cx, cy)
	}

	// 2. Rotation in radians (e.g. math.Pi / 2 = 90 deg counter-clockwise)
	stateRot := VisualRenderState{
		Rotation:        math.Pi / 2,
		CursorBodyOffDx: 0,
		CursorBodyOffDy: 0,
	}
	// A point to the right of center (cx + 10, cy) unrotated should come from (cx, cy - 10)
	sxRot, syRot := computeReferenceTransformInverse(cx+10, cy, width, height, stateRot, 0)
	if math.Abs(sxRot-cx) > 0.001 || math.Abs(syRot-(cy-10)) > 0.001 {
		t.Fatalf("90 deg rotation mapping = (%v, %v), expected (%v, %v)", sxRot, syRot, cx, cy-10)
	}

	// 3. Pulse compression scale
	clickProgress := 1.0
	pulseCompression := clickProgress * 0.03
	scaleX := 1.0 - pulseCompression
	scaleY := 1.0 + (pulseCompression * 0.4)

	sxPulse, syPulse := computeReferenceTransformInverse(cx+10, cy+10, width, height, stateZero, clickProgress)
	expectedSx := (10 / scaleX) + cx
	expectedSy := (10 / scaleY) + cy
	if math.Abs(sxPulse-expectedSx) > 0.001 || math.Abs(syPulse-expectedSy) > 0.001 {
		t.Fatalf("Pulse compression mapping = (%v, %v), expected (%v, %v)", sxPulse, syPulse, expectedSx, expectedSy)
	}
}
