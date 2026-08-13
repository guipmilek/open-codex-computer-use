package main

import (
	"math"
	"testing"
)

// forwardReferenceTransform performs the exact forward transform matching Swift SoftwareCursorGlyphRenderer.drawReferenceImage:
//
//	T(origin) * R(rotation) * S(scaleX, scaleY) * T(-center)
func forwardReferenceTransform(srcX, srcY float64, width, height float64, state VisualRenderState, clickProgress float64) (float64, float64) {
	motionCompression := math.Min(math.Hypot(state.CursorBodyOffDx, state.CursorBodyOffDy)*0.008, 0.018)
	pulseCompression := clickProgress * 0.03
	scaleX := 1.0 - motionCompression - pulseCompression
	scaleY := 1.0 + (pulseCompression * 0.4)

	angleRad := state.Rotation
	cosA := math.Cos(angleRad)
	sinA := math.Sin(angleRad)

	cx := width / 2.0
	cy := height / 2.0

	// 1. Shift source point relative to source canvas center
	px := srcX - cx
	py := srcY - cy

	// 2. Scale
	scaledX := px * scaleX
	scaledY := py * scaleY

	// 3. Rotate R(angle)
	rotX := scaledX*cosA - scaledY*sinA
	rotY := scaledX*sinA + scaledY*cosA

	// 4. Translate to transform origin (center + cursorBodyOffset)
	originX := cx + state.CursorBodyOffDx
	originY := cy + state.CursorBodyOffDy

	return rotX + originX, rotY + originY
}

func TestGoldenInverseTransformMatchesForward(t *testing.T) {
	width := 126.0
	height := 126.0

	state := VisualRenderState{
		Rotation:        math.Pi / 4, // 45 degrees in radians
		CursorBodyOffDx: 10.0,
		CursorBodyOffDy: -5.0,
	}
	clickProgress := 1.0

	testPoints := []Pt{
		{X: 63, Y: 63},
		{X: 10, Y: 20},
		{X: 100, Y: 80},
		{X: 60.35, Y: 70.3},
	}

	for _, pt := range testPoints {
		// Forward transform: src -> dst
		dstX, dstY := forwardReferenceTransform(pt.X, pt.Y, width, height, state, clickProgress)

		// Inverse transform: dst -> src'
		computedSrcX, computedSrcY := computeReferenceTransformInverse(dstX, dstY, width, height, state, clickProgress)

		if math.Abs(computedSrcX-pt.X) > 0.0001 || math.Abs(computedSrcY-pt.Y) > 0.0001 {
			t.Fatalf("Inverse transform mismatch for point %v: got (%v, %v), want (%v, %v)",
				pt, computedSrcX, computedSrcY, pt.X, pt.Y)
		}
	}
}
