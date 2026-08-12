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

func renderCursorToDIB(pixels unsafe.Pointer, width, height int32, src *image.NRGBA, state VisualRenderState, clickProgress float64) {
	size := int(width * height * 4)
	bytesSlice := unsafe.Slice((*byte)(pixels), size)
	for i := 0; i < size; i++ {
		bytesSlice[i] = 0
	}

	scale := 1.0 + clickProgress*0.03
	angleRad := state.Rotation * math.Pi / 180.0
	cosA := math.Cos(angleRad)
	sinA := math.Sin(angleRad)

	cx := float64(width) / 2.0
	cy := float64(height) / 2.0

	for y := 0; y < int(height); y++ {
		for x := 0; x < int(width); x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy

			dx /= scale
			dy /= scale

			sx := dx*cosA + dy*sinA
			sy := -dx*sinA + dy*cosA

			sx -= state.CursorBodyOffDx
			sy -= state.CursorBodyOffDy
			sx -= state.FogOffDx
			sy -= state.FogOffDy

			srcX := int(sx + cx + 0.5)
			srcY := int(sy + cy + 0.5)

			if srcX >= 0 && srcX < 126 && srcY >= 0 && srcY < 126 {
				srcIdx := (srcY*126 + srcX) * 4
				r := uint32(src.Pix[srcIdx])
				g := uint32(src.Pix[srcIdx+1])
				b := uint32(src.Pix[srcIdx+2])
				a := uint32(src.Pix[srcIdx+3])

				if a > 0 {
					if state.FogOpacity > 0 {
						a = uint32(float64(a) * (1.0 - state.FogOpacity))
					}
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
