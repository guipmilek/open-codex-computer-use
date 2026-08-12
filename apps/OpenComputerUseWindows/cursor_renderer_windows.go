//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"math"
	"unsafe"
)

//go:embed official-software-cursor-window-252.png
var cursorImageData []byte

func loadCursorImage() *image.NRGBA {
	img, err := png.Decode(bytes.NewReader(cursorImageData))
	if err != nil {
		return image.NewNRGBA(image.Rect(0, 0, 126, 126))
	}

	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()

	if srcW == 126 && srcH == 126 {
		if nrgba, ok := img.(*image.NRGBA); ok {
			return nrgba
		}
		res := image.NewNRGBA(image.Rect(0, 0, 126, 126))
		for y := 0; y < 126; y++ {
			for x := 0; x < 126; x++ {
				res.Set(x, y, img.At(x, y))
			}
		}
		return res
	}

	res := image.NewNRGBA(image.Rect(0, 0, 126, 126))
	for y := 0; y < 126; y++ {
		for x := 0; x < 126; x++ {
			srcX := x * srcW / 126
			srcY := y * srcH / 126
			res.Set(x, y, img.At(srcBounds.Min.X+srcX, srcBounds.Min.Y+srcY))
		}
	}
	return res
}

// computeReferenceTransformInverse computes the inverse 2D affine mapping
// matching Swift SoftwareCursorGlyphRenderer.drawReferenceImage.
// Returns (srcX, srcY) in 126x126 space for target pixel (x, y).
func computeReferenceTransformInverse(x, y float64, width, height float64, state VisualRenderState, clickProgress float64) (float64, float64) {
	motionCompression := math.Min(math.Hypot(state.CursorBodyOffDx, state.CursorBodyOffDy)*0.008, 0.018)
	pulseCompression := clickProgress * 0.03
	scaleX := 1.0 - motionCompression - pulseCompression
	scaleY := 1.0 + (pulseCompression * 0.4)

	if scaleX <= 0 {
		scaleX = 0.001
	}
	if scaleY <= 0 {
		scaleY = 0.001
	}

	angleRad := state.Rotation // Directly in radians, matching Swift
	cosA := math.Cos(angleRad)
	sinA := math.Sin(angleRad)

	cx := width / 2.0
	cy := height / 2.0

	// Center of transform includes cursorBodyOffset
	originX := cx + state.CursorBodyOffDx
	originY := cy + state.CursorBodyOffDy

	// Shift target pixel relative to transform origin
	dx := x - originX
	dy := y - originY

	// Unscale
	dx /= scaleX
	dy /= scaleY

	// Unrotate (inverse rotation matrix)
	sx := dx*cosA + dy*sinA
	sy := -dx*sinA + dy*cosA

	// Back to canvas center
	return sx + cx, sy + cy
}

func renderCursorToDIB(pixels unsafe.Pointer, width, height int32, src *image.NRGBA, state VisualRenderState, clickProgress float64) {
	size := int(width * height * 4)
	bytesSlice := unsafe.Slice((*byte)(pixels), size)
	for i := 0; i < size; i++ {
		bytesSlice[i] = 0
	}

	w := float64(width)
	h := float64(height)

	for y := 0; y < int(height); y++ {
		for x := 0; x < int(width); x++ {
			sx, sy := computeReferenceTransformInverse(float64(x), float64(y), w, h, state, clickProgress)

			srcX := int(sx + 0.5)
			srcY := int(sy + 0.5)

			if srcX >= 0 && srcX < 126 && srcY >= 0 && srcY < 126 {
				srcIdx := (srcY*126 + srcX) * 4
				r := uint32(src.Pix[srcIdx])
				g := uint32(src.Pix[srcIdx+1])
				b := uint32(src.Pix[srcIdx+2])
				a := uint32(src.Pix[srcIdx+3])

				if a > 0 {
					// Premultiplied BGRA for Windows UpdateLayeredWindow
					r = (r * a) / 255
					g = (g * a) / 255
					b = (b * a) / 255

					dstIdx := (y*int(width) + x) * 4
					bytesSlice[dstIdx] = byte(b)
					bytesSlice[dstIdx+1] = byte(g)
					bytesSlice[dstIdx+2] = byte(r)
					bytesSlice[dstIdx+3] = byte(a)
				}
			}
		}
	}
}
