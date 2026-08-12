package main

// resolveVisualCursorTarget computes the screen-space point that the visual
// cursor should animate toward. This must produce EXACTLY the same coordinate
// that the PowerShell bridge will use for the actual action, so the cursor
// and the click/set_value land on the same pixel.
//
// The Windows runtime uses two coordinate systems:
//   - element_index: element.Frame is window-relative, action point = windowBounds.origin + frame.center
//   - x/y: screenshot pixel coordinates, action point = windowBounds.origin + (x, y)
//
// The PowerShell bridge performs this same mapping in its click/drag handlers.
func resolveVisualCursorTarget(element *elementRecord, x, y *float64, windowBounds *frame) *Pt {
	if windowBounds == nil {
		return nil
	}

	// Element-targeted: use the center of the element's window-relative frame.
	if element != nil && element.Frame != nil {
		f := element.Frame
		screenX := windowBounds.X + f.X + f.Width/2
		screenY := windowBounds.Y + f.Y + f.Height/2
		return &Pt{X: screenX, Y: screenY}
	}

	// Coordinate-targeted: x/y are screenshot pixel coordinates.
	// The screenshot is captured at windowBounds, so the screen point is
	// windowBounds.origin + (x, y).
	if x != nil && y != nil {
		screenX := windowBounds.X + *x
		screenY := windowBounds.Y + *y
		return &Pt{X: screenX, Y: screenY}
	}

	return nil
}

// targetHWNDFromSnapshot extracts the HWND of the app's main window from the
// most recent snapshot, if available. The PowerShell bridge populates
// nativeWindowHandle on the root window element (element index 0).
func targetHWNDFromSnapshot(snapshot *appSnapshot) uintptr {
	if snapshot == nil {
		return 0
	}
	for _, el := range snapshot.Elements {
		if el.NativeWindowHandle != 0 {
			return uintptr(el.NativeWindowHandle)
		}
	}
	return 0
}
