package main

import (
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

// VisualCursorController manages the software cursor overlay lifecycle.
// It is the single entry point used by the tool service to drive cursor animation.
// All public methods are synchronous: they block until the animation completes,
// matching the macOS behavior where moveVisualCursor blocks before the real action.
type VisualCursorController struct {
	mu                  sync.Mutex
	overlay             *overlayWindow
	displayedTip        *Pt // nil if cursor has never been shown or was reset
	restingTip          *Pt // where the cursor should idle
	visualDynamicsState *VisualDynamicsState
	currentForwardDx    float64 // current cursor forward heading
	currentForwardDy    float64
	idleTimer           *time.Timer
	hideTimer           *time.Timer
	idlePhase           float64
	idleStop            chan struct{}
	initialized         bool
	disabled            bool

	// The HWND of the target window for z-ordering.
	targetHWND uintptr
}

// NewVisualCursorController creates a new controller. It checks the
// OPEN_COMPUTER_USE_VISUAL_CURSOR environment variable and marks itself
// disabled if the user has explicitly turned it off.
func NewVisualCursorController() *VisualCursorController {
	return &VisualCursorController{
		disabled: !visualCursorEnabled(),
	}
}

// Tip anchor in the 126x126 canvas (matching macOS SoftwareCursorGlyphMetrics).
const (
	cursorTipAnchorX  = 60.35
	cursorTipAnchorY  = 70.3
	cursorWindowSize  = 126
	neutralHeading    = -(3 * math.Pi / 4) // -3π/4, matching macOS
	idleAmplitude     = 0.09
	idleTimeout       = 30 * time.Second
	fadeOutDurationMs = 120
)

// visualCursorEnabled checks the OPEN_COMPUTER_USE_VISUAL_CURSOR env var.
// Default is enabled (same as macOS).
func visualCursorEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("OPEN_COMPUTER_USE_VISUAL_CURSOR")))
	if raw == "" {
		return true
	}
	for _, disabled := range []string{"0", "false", "no", "off"} {
		if raw == disabled {
			return false
		}
	}
	return true
}

// defaultInitialTipPosition returns the fresh-start tip position,
// matching macOS: window origin (0,0) + tipAnchor.
func defaultInitialTipPosition() Pt {
	return Pt{X: cursorTipAnchorX, Y: cursorTipAnchorY}
}

// restingForwardVector returns the default cursor forward vector
// (the direction the cursor "faces" at rest).
func restingForwardVector() Vec2 {
	return Vec2{Dx: math.Cos(neutralHeading), Dy: math.Sin(neutralHeading)}
}

// currentForwardVector returns the cursor's current forward direction
// based on its last render rotation.
func (c *VisualCursorController) currentForwardVector() Vec2 {
	// If we have dynamics state, derive forward from the current rotation.
	if c.visualDynamicsState != nil {
		angle := neutralHeading + c.visualDynamicsState.Angle
		return Vec2{Dx: math.Cos(angle), Dy: math.Sin(angle)}
	}
	return restingForwardVector()
}

// ensureOverlay lazily creates the platform overlay window.
func (c *VisualCursorController) ensureOverlay() {
	if c.initialized {
		return
	}
	c.initialized = true
	c.overlay = platformCreateOverlay()
}

// MoveToAndWait animates the cursor from its current position to the target
// screen point. This call blocks until the animation is complete.
// It must be called BEFORE the actual click/set_value action.
func (c *VisualCursorController) MoveToAndWait(screenX, screenY float64) {
	if c.disabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureOverlay()
	if c.overlay == nil {
		return
	}

	c.stopIdleAnimation()
	c.cancelPendingHide()

	target := Pt{X: screenX, Y: screenY}
	isFreshStart := c.displayedTip == nil
	startPoint := defaultInitialTipPosition()
	if !isFreshStart {
		startPoint = *c.displayedTip
	}

	now := timeNow()
	if isFreshStart {
		vds := NewVisualDynamicsState(startPoint, now)
		c.visualDynamicsState = &vds
		rs := c.initialRenderState(startPoint)
		c.placeCursor(rs, 0)
	} else {
		c.seedVisualDynamicsIfNeeded(startPoint, now)
		rs := c.advanceVisualDynamics(startPoint, 0, now)
		c.placeCursor(rs, 0)
	}

	c.overlay.show()

	dist := math.Hypot(target.X-startPoint.X, target.Y-startPoint.Y)
	if dist > 2 {
		c.animateMove(startPoint, target)
	}

	// Final placement at exact target
	rs := c.advanceVisualDynamics(target, 0, timeNow())
	c.placeCursor(rs, 0)
}

// PulseClickAndWait plays the click pulse animation at the given screen point.
// This call blocks until the animation completes.
func (c *VisualCursorController) PulseClickAndWait(screenX, screenY float64, clickCount int, mouseButton string) {
	if c.disabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.overlay == nil {
		return
	}

	target := Pt{X: screenX, Y: screenY}
	now := timeNow()
	c.seedVisualDynamicsIfNeeded(target, now)
	c.restingTip = &target

	pulseBias := 1.0
	if mouseButton == "right" {
		pulseBias = 0.82
	}

	count := clickCount
	if count < 1 {
		count = 1
	}

	for pulse := 0; pulse < count; pulse++ {
		duration := 0.16
		startTime := timeNow()

		for {
			elapsed := timeNow() - startTime
			rawProgress := clampF(elapsed/duration, 0, 1)
			clickProgress := math.Sin(rawProgress*math.Pi) * pulseBias

			rs := c.advanceVisualDynamics(target, 0, timeNow())
			c.placeCursor(rs, clickProgress)

			if rawProgress >= 1 {
				break
			}
			sleepFrame()
		}

		if pulse < count-1 {
			pauseFor(0.05)
		}
	}

	// Final frame with no click effect
	rs := c.advanceVisualDynamics(target, 0, timeNow())
	c.placeCursor(rs, 0)

	c.startIdleAnimation()
	c.scheduleHide(idleTimeout)
}

// Settle places the cursor at the target without a click pulse (used for set_value).
func (c *VisualCursorController) Settle(screenX, screenY float64) {
	if c.disabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.overlay == nil {
		return
	}

	target := Pt{X: screenX, Y: screenY}
	c.restingTip = &target

	rs := c.advanceVisualDynamics(target, 0, timeNow())
	c.placeCursor(rs, 0)

	c.startIdleAnimation()
	c.scheduleHide(idleTimeout)
}

// Reset hides the cursor and clears all position state. Called on turn-ended.
func (c *VisualCursorController) Reset() {
	if c.disabled {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stopIdleAnimation()
	c.cancelPendingHide()
	c.displayedTip = nil
	c.restingTip = nil
	c.visualDynamicsState = nil
	c.targetHWND = 0
	if c.overlay != nil {
		c.overlay.hide()
	}
}

// SetTargetHWND sets the HWND of the target window for z-ordering.
func (c *VisualCursorController) SetTargetHWND(hwnd uintptr) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targetHWND = hwnd
	if c.overlay != nil {
		c.overlay.setZOrder(hwnd)
	}
}

// --- Internal animation helpers ---

func (c *VisualCursorController) animateMove(start, end Pt) {
	startForward := c.currentForwardVector()
	endForward := restingForwardVector()

	candidates := MakeHeadingDrivenCandidates(start, end, nil, startForward, endForward)
	candidate := ChooseHeadingDrivenBestCandidate(candidates)
	if candidate == nil {
		// Fallback: straight line
		path := NewMotionPathSimple(start, end)
		candidate = &MotionCandidate{
			Identifier:  "fallback",
			Kind:        "base",
			Path:        path,
			Measurement: path.Measure(nil, 0.01),
		}
	}

	path := candidate.Path
	duration := CalibratedTravelDuration()
	springTargetDuration := ComputeCloseEnoughTime(OfficialSpringConfig())
	startTime := timeNow()
	progress := 0.0
	springState := SpringState{}

	for {
		elapsed := timeNow() - startTime
		normalizedElapsed := clampF(elapsed/math.Max(duration, 0.001), 0, 1)
		springTime := normalizedElapsed * springTargetDuration
		progress, springState = AdvanceSpringTo(progress, 1, springState, OfficialSpringConfig(), springTime)

		sample, _ := path.Sample(progress)
		rs := c.advanceVisualDynamics(sample, 0, timeNow())
		c.placeCursor(rs, 0)

		if normalizedElapsed >= 1 || IsSpringCloseEnough(progress, 1, OfficialSpringConfig()) {
			break
		}
		sleepFrame()
	}
}

func (c *VisualCursorController) initialRenderState(tip Pt) VisualRenderState {
	return VisualRenderState{
		TipPosition:     tip,
		Rotation:        0,
		CursorBodyOffDx: 0,
		CursorBodyOffDy: 0,
		FogOffDx:        0,
		FogOffDy:        0,
		FogOpacity:      DefaultVisualDynamicsConfig().FogOpacityBase,
		FogScale:        1,
	}
}

func (c *VisualCursorController) seedVisualDynamicsIfNeeded(tip Pt, t float64) {
	if c.visualDynamicsState == nil {
		state := NewVisualDynamicsState(tip, t)
		c.visualDynamicsState = &state
	}
}

func (c *VisualCursorController) advanceVisualDynamics(targetTip Pt, idleAngleOffset, t float64) VisualRenderState {
	c.seedVisualDynamicsIfNeeded(targetTip, t)
	config := DefaultVisualDynamicsConfig()
	state, rs := AdvanceVisualDynamics(
		*c.visualDynamicsState,
		targetTip,
		t,
		idleAngleOffset,
		neutralHeading,
		1, // renderYAxisMultiplier: 1 on Windows (y-down, no flip needed)
		config,
	)
	c.visualDynamicsState = &state
	return rs
}

func (c *VisualCursorController) placeCursor(rs VisualRenderState, clickProgress float64) {
	if c.overlay == nil {
		return
	}
	c.overlay.updateFrame(rs.TipPosition.X, rs.TipPosition.Y, rs, clickProgress)
	tip := rs.TipPosition
	c.displayedTip = &tip
}

func (c *VisualCursorController) startIdleAnimation() {
	if c.restingTip == nil || c.overlay == nil {
		return
	}

	c.stopIdleAnimation()

	stop := make(chan struct{})
	c.idleStop = stop
	c.idlePhase = 0

	resting := *c.restingTip

	// Run idle animation in a goroutine. We don't hold the lock during sleep,
	// but we re-acquire it for each frame update.
	go func() {
		ticker := time.NewTicker(time.Second / 60)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				c.mu.Lock()
				if c.overlay == nil {
					c.mu.Unlock()
					return
				}
				c.idlePhase += 0.05
				angleOffset := math.Sin(c.idlePhase*0.8) * idleAmplitude
				rs := c.advanceVisualDynamics(resting, angleOffset, timeNow())
				c.placeCursor(rs, 0)
				c.mu.Unlock()
			}
		}
	}()
}

func (c *VisualCursorController) stopIdleAnimation() {
	if c.idleStop != nil {
		close(c.idleStop)
		c.idleStop = nil
	}
}

func (c *VisualCursorController) scheduleHide(delay time.Duration) {
	c.cancelPendingHide()
	c.hideTimer = time.AfterFunc(delay, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.hideOverlay()
	})
}

func (c *VisualCursorController) cancelPendingHide() {
	if c.hideTimer != nil {
		c.hideTimer.Stop()
		c.hideTimer = nil
	}
}

func (c *VisualCursorController) hideOverlay() {
	c.stopIdleAnimation()
	c.cancelPendingHide()
	if c.overlay != nil {
		c.overlay.fadeOut(fadeOutDurationMs)
	}
	c.displayedTip = nil
	c.restingTip = nil
	c.visualDynamicsState = nil
	c.targetHWND = 0
}

// --- Time helpers ---

func timeNow() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

func sleepFrame() {
	time.Sleep(time.Second / 120) // ~120 Hz
}

func pauseFor(seconds float64) {
	start := timeNow()
	for timeNow()-start < seconds {
		sleepFrame()
	}
}
