package main

import (
	"math"
	"testing"
)

func TestSpringPhysicsSimulation(t *testing.T) {
	config := OfficialSpringConfig()
	duration := ComputeCloseEnoughTime(config)

	if duration < 1.0 || duration > 2.0 {
		t.Fatalf("ComputeCloseEnoughTime() = %v, expected between 1.0s and 2.0s", duration)
	}

	progress, _ := AdvanceSpringTo(0, 1.0, SpringState{}, config, duration)

	if !IsSpringCloseEnough(progress, 1.0, config) {
		t.Fatalf("Progress after duration = %v, expected close enough to 1.0", progress)
	}
}

func TestHeadingDrivenCandidateGeneration(t *testing.T) {
	start := Pt{X: 100, Y: 100}
	end := Pt{X: 400, Y: 300}
	startForward := Vec2{Dx: 1, Dy: 0}
	endForward := Vec2{Dx: 0, Dy: 1}

	candidates := MakeHeadingDrivenCandidates(start, end, nil, startForward, endForward)
	if len(candidates) != 10 {
		t.Fatalf("MakeHeadingDrivenCandidates() count = %d, expected 10 descriptors", len(candidates))
	}

	best := ChooseHeadingDrivenBestCandidate(candidates)
	if best == nil {
		t.Fatal("ChooseHeadingDrivenBestCandidate() returned nil")
	}

	if best.Identifier == "" {
		t.Fatal("Best candidate missing identifier")
	}

	ptStart, _ := best.Path.Sample(0)
	if math.Hypot(ptStart.X-start.X, ptStart.Y-start.Y) > 0.001 {
		t.Fatalf("Path start = %v, expected %v", ptStart, start)
	}

	ptEnd, _ := best.Path.Sample(1.0)
	if math.Hypot(ptEnd.X-end.X, ptEnd.Y-end.Y) > 0.001 {
		t.Fatalf("Path end = %v, expected %v", ptEnd, end)
	}
}

func TestVisualDynamicsAdvance(t *testing.T) {
	cfg := DefaultVisualDynamicsConfig()
	start := Pt{X: 100, Y: 100}
	target := Pt{X: 200, Y: 150}

	state := NewVisualDynamicsState(start, 0)
	nextState, renderState := AdvanceVisualDynamics(state, target, 0.1, 0, neutralHeading, 1, cfg)

	if nextState.TipPosition == start {
		t.Fatal("TipPosition did not advance toward target")
	}

	if renderState.FogOpacity < cfg.FogOpacityBase {
		t.Fatalf("FogOpacity = %v, expected >= base %v", renderState.FogOpacity, cfg.FogOpacityBase)
	}
}

func TestAngleNormalization(t *testing.T) {
	for input, expected := range map[float64]float64{
		0:            0,
		math.Pi * 3:  math.Pi,
		-math.Pi * 3: -math.Pi,
	} {
		got := normalizeAngle(input)
		if math.Abs(got-expected) > 0.0001 {
			t.Errorf("normalizeAngle(%v) = %v, expected %v", input, got, expected)
		}
	}
}
