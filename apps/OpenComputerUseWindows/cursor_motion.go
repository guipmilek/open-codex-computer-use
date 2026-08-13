package main

import (
	"math"
)

// --- Utility Functions ---

func clampF(val, lo, hi float64) float64 {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

func normalizeAngle(angle float64) float64 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

func signedAngle(from, to Vec2) float64 {
	return math.Atan2(from.Dx*to.Dy-from.Dy*to.Dx, from.Dx*to.Dx+from.Dy*to.Dy)
}

func sampleCubic(p0, p1, p2, p3 Pt, t float64) Pt {
	omt := 1 - t
	omt2 := omt * omt
	t2 := t * t
	return Pt{
		X: omt2*omt*p0.X + 3*omt2*t*p1.X + 3*omt*t2*p2.X + t2*t*p3.X,
		Y: omt2*omt*p0.Y + 3*omt2*t*p1.Y + 3*omt*t2*p2.Y + t2*t*p3.Y,
	}
}

func sampleCubicTangent(p0, p1, p2, p3 Pt, t float64) Vec2 {
	omt := 1 - t
	return Vec2{
		Dx: 3*omt*omt*(p1.X-p0.X) + 6*omt*t*(p2.X-p1.X) + 3*t*t*(p3.X-p2.X),
		Dy: 3*omt*omt*(p1.Y-p0.Y) + 6*omt*t*(p2.Y-p1.Y) + 3*t*t*(p3.Y-p2.Y),
	}
}

// --- Basic Types ---

type Vec2 struct{ Dx, Dy float64 }

func (v Vec2) Length() float64       { return math.Hypot(v.Dx, v.Dy) }
func (v Vec2) Perpendicular() Vec2   { return Vec2{-v.Dy, v.Dx} }
func (v Vec2) Scaled(f float64) Vec2 { return Vec2{v.Dx * f, v.Dy * f} }
func (v Vec2) Add(o Vec2) Vec2       { return Vec2{v.Dx + o.Dx, v.Dy + o.Dy} }
func (v Vec2) Sub(o Vec2) Vec2       { return Vec2{v.Dx - o.Dx, v.Dy - o.Dy} }

func (v Vec2) Normalized() Vec2 {
	l := math.Max(v.Length(), 0.001)
	return Vec2{v.Dx / l, v.Dy / l}
}

type Pt struct{ X, Y float64 }

func (p Pt) AddVec(v Vec2) Pt { return Pt{p.X + v.Dx, p.Y + v.Dy} }
func (p Pt) SubVec(v Vec2) Pt { return Pt{p.X - v.Dx, p.Y - v.Dy} }
func (p Pt) SubPt(o Pt) Vec2  { return Vec2{p.X - o.X, p.Y - o.Y} }

type Rect struct{ X, Y, W, H float64 }

func (r Rect) MinX() float64 { return r.X }
func (r Rect) MaxX() float64 { return r.X + r.W }
func (r Rect) MinY() float64 { return r.Y }
func (r Rect) MaxY() float64 { return r.Y + r.H }
func (r Rect) Contains(pt Pt) bool {
	return pt.X >= r.MinX() && pt.X <= r.MaxX() && pt.Y >= r.MinY() && pt.Y <= r.MaxY()
}
func (r Rect) ContainsWithPadding(pt Pt, pad float64) bool {
	return pt.X >= r.MinX()-pad && pt.X <= r.MaxX()+pad &&
		pt.Y >= r.MinY()-pad && pt.Y <= r.MaxY()+pad
}

// --- Path & Measurement ---

type MotionSegment struct {
	End, Control1, Control2 Pt
}

type MotionPath struct {
	Start, End                       Pt
	StartControl, Arc, ArcIn, ArcOut *Pt
	EndControl                       *Pt
	Segments                         []MotionSegment
	CurveScale                       float64
}

// NewMotionPathSimple creates a straight-line single-segment path.
func NewMotionPathSimple(start, end Pt) MotionPath {
	c1 := Pt{X: start.X + (end.X-start.X)/3, Y: start.Y + (end.Y-start.Y)/3}
	c2 := Pt{X: start.X + 2*(end.X-start.X)/3, Y: start.Y + 2*(end.Y-start.Y)/3}
	return MotionPath{
		Start: start, End: end,
		StartControl: &c1, EndControl: &c2,
		Segments:   []MotionSegment{{End: end, Control1: c1, Control2: c2}},
		CurveScale: 0,
	}
}

func (mp *MotionPath) Sample(progress float64) (Pt, Vec2) {
	if len(mp.Segments) == 0 {
		return mp.Start, Vec2{1, 0}
	}
	clamped := clampF(progress, 0, 1)
	n := len(mp.Segments)
	var segIdx int
	var localT float64
	if clamped >= 1 {
		segIdx = n - 1
		localT = 1
	} else {
		scaled := clamped * float64(n)
		segIdx = int(scaled)
		if segIdx >= n {
			segIdx = n - 1
		}
		localT = scaled - float64(segIdx)
	}
	seg := mp.Segments[segIdx]
	segStart := mp.Start
	if segIdx > 0 {
		segStart = mp.Segments[segIdx-1].End
	}
	pt := sampleCubic(segStart, seg.Control1, seg.Control2, seg.End, localT)
	tan := sampleCubicTangent(segStart, seg.Control1, seg.Control2, seg.End, localT).Normalized()
	return pt, tan
}

func (mp *MotionPath) SampledConstraintPoints(samplesPerSegment int) []Pt {
	total := len(mp.Segments) * samplesPerSegment
	if total < 1 {
		total = 1
	}
	pts := make([]Pt, 0, total)
	for step := 1; step <= total; step++ {
		pt, _ := mp.Sample(float64(step) / float64(total))
		pts = append(pts, pt)
	}
	return pts
}

func (mp *MotionPath) Measure(bounds *Rect, minStepDist float64) MotionMeasurement {
	m := MotionMeasurement{StaysInBounds: bounds == nil || bounds.ContainsWithPadding(mp.Start, 20)}
	total := len(mp.Segments) * 24
	if total < 1 {
		total = 1
	}
	prev := mp.Start
	var prevAngle *float64
	for step := 1; step <= total; step++ {
		pt, _ := mp.Sample(float64(step) / float64(total))
		if bounds != nil && m.StaysInBounds {
			m.StaysInBounds = bounds.ContainsWithPadding(pt, 20)
		}
		delta := pt.SubPt(prev)
		stepLen := delta.Length()
		if stepLen > minStepDist {
			angle := math.Atan2(delta.Dy, delta.Dx)
			m.Length += stepLen
			if prevAngle != nil {
				ad := normalizeAngle(angle - *prevAngle)
				abs := math.Abs(ad)
				m.AngleChangeEnergy += ad * ad
				m.MaxAngleChange = math.Max(m.MaxAngleChange, abs)
				m.TotalTurn += abs
			}
			prevAngle = &angle
			prev = pt
		}
	}
	return m
}

type MotionMeasurement struct {
	Length, AngleChangeEnergy, MaxAngleChange, TotalTurn float64
	StaysInBounds                                        bool
}

type MotionCandidate struct {
	Identifier               string
	Kind                     string // "base" or "arched"
	Side                     int
	TableAScale, TableBScale *float64
	Path                     MotionPath
	Measurement              MotionMeasurement
	Score                    float64
}

// --- Spring ---

type SpringConfig struct {
	response, dampingFraction, dt, idleVelocityThreshold       float64
	stiffness, drag                                            float64
	closeEnoughProgressThreshold, closeEnoughDistanceThreshold float64
}

func newSpringConfig(response, damping, dt, idleVelThr float64) SpringConfig {
	rawStiff := math.Pow(2*math.Pi/response, 2)
	stiff := math.Min(rawStiff, idleVelThr)
	return SpringConfig{
		response: response, dampingFraction: damping, dt: dt,
		idleVelocityThreshold: idleVelThr, stiffness: stiff,
		drag:                         2 * damping * math.Sqrt(stiff),
		closeEnoughProgressThreshold: 1, closeEnoughDistanceThreshold: 0.01,
	}
}

func OfficialSpringConfig() SpringConfig {
	return newSpringConfig(1.4, 0.9, 1.0/240.0, 28800)
}

type SpringState struct{ Time, Velocity, Force float64 }

func AdvanceSpring(cur, target float64, s SpringState, c SpringConfig) (float64, SpringState) {
	half := c.dt * 0.5
	vHalf := s.Velocity + s.Force*half
	next := cur + vHalf*c.dt
	f := c.stiffness*(target-next) + (-c.drag)*vHalf
	v := vHalf + f*half
	return next, SpringState{Time: s.Time + c.dt, Velocity: v, Force: f}
}

func AdvanceSpringTo(cur, target float64, s SpringState, c SpringConfig, tgtTime float64) (float64, SpringState) {
	if s.Time > 0 && tgtTime-s.Time > 1 {
		s.Time = tgtTime - 1.0/60.0
	}
	for s.Time < tgtTime {
		cur, s = AdvanceSpring(cur, target, s, c)
	}
	return cur, s
}

func IsSpringCloseEnough(progress, target float64, c SpringConfig) bool {
	return progress >= c.closeEnoughProgressThreshold &&
		math.Abs(target-progress) <= c.closeEnoughDistanceThreshold
}

func ComputeCloseEnoughTime(c SpringConfig) float64 {
	cur := 0.0
	s := SpringState{}
	for step := 0; step < 4096; step++ {
		tgtTime := float64(step+1) * c.dt
		cur, s = AdvanceSpringTo(cur, 1, s, c, tgtTime)
		if IsSpringCloseEnough(cur, 1, c) {
			return s.Time
		}
	}
	return 1.43
}

func CalibratedTravelDuration() float64 {
	return ComputeCloseEnoughTime(OfficialSpringConfig())
}

// --- HeadingDrivenCursorMotionModel ---

const (
	hdDefaultStartHandle   = 0.29
	hdDefaultEndHandle     = 0.08
	hdDefaultArcSize       = 0.06
	hdDefaultArcFlow       = 0.64
	hdNormalizationEpsilon = 0.001
	hdMinStepDistance      = 0.01
)

type hdMotionDescriptor struct {
	id                   string
	family               string
	side                 int
	startReachScale      float64
	endReachScale        float64
	startLineWeight      float64
	endLineWeight        float64
	startHeadingWeight   float64
	endHeadingWeight     float64
	startNormalScale     float64
	endNormalScale       float64
	startGuideNormalBias float64
	endGuideNormalBias   float64
	startFlowWeight      float64
	endFlowWeight        float64
	flowShift            float64
	arcScale             float64
	scoreBias            float64
}

func (d hdMotionDescriptor) kind() string {
	if d.family == "direct" {
		return "base"
	}
	return "arched"
}

type hdMotionMetrics struct {
	start, end                             Pt
	dx, dy, distance                       float64
	direction, normal                      Vec2
	horizontalFactor, verticalFactor       float64
	diagonalFactor, closeFactor, farFactor float64
}

func newHDMetrics(start, end Pt) hdMotionMetrics {
	dx := end.X - start.X
	dy := end.Y - start.Y
	dist := math.Max(math.Hypot(dx, dy), 1)
	dir := hdNormVec(Vec2{dx, dy})
	norm := hdNormVec(Vec2{-dir.Dy, dir.Dx})
	hf := math.Abs(dx) / dist
	vf := math.Abs(dy) / dist
	return hdMotionMetrics{
		start: start, end: end, dx: dx, dy: dy, distance: dist,
		direction: dir, normal: norm,
		horizontalFactor: hf, verticalFactor: vf,
		diagonalFactor: math.Min(hf, vf) * 2,
		closeFactor:    clampF(1-dist/280, 0, 1),
		farFactor:      clampF((dist-180)/540, 0, 1),
	}
}

func hdNormVec(v Vec2) Vec2 {
	l := math.Max(v.Length(), hdNormalizationEpsilon)
	return Vec2{v.Dx / l, v.Dy / l}
}

func hdPreferredTurnSide(m hdMotionMetrics, sf, ef Vec2) int {
	sd := signedAngle(sf, m.direction)
	if math.Abs(sd) > 0.16 {
		if sd > 0 {
			return 1
		}
		return -1
	}
	ed := signedAngle(m.direction, ef)
	if math.Abs(ed) > 0.18 {
		if ed > 0 {
			return -1
		}
		return 1
	}
	if math.Abs(m.dy) > math.Abs(m.dx)*0.72 {
		if m.dy > 0 {
			return -1
		}
		return 1
	}
	if m.dx >= 0 {
		return 1
	}
	return -1
}

func hdDescriptors(m hdMotionMetrics, ps int) []hdMotionDescriptor {
	os := 0.82 + m.farFactor*0.26
	ts := 0.90 + m.farFactor*0.30
	bs := 0.74 + m.farFactor*0.24
	return []hdMotionDescriptor{
		{id: "direct-tight", family: "direct", side: 0, startReachScale: 0.90, endReachScale: 0.86, startLineWeight: 1.12, endLineWeight: 1.04, startHeadingWeight: 0.18, endHeadingWeight: 0.20, startNormalScale: 0.02, endNormalScale: 0.02, startGuideNormalBias: 0, endGuideNormalBias: 0, startFlowWeight: 0.02, endFlowWeight: 0.02, flowShift: -0.02, arcScale: 0.16, scoreBias: 18},
		{id: "direct-soft", family: "direct", side: 0, startReachScale: 0.98, endReachScale: 0.94, startLineWeight: 1.04, endLineWeight: 0.96, startHeadingWeight: 0.22, endHeadingWeight: 0.28, startNormalScale: 0.04, endNormalScale: 0.08, startGuideNormalBias: 0, endGuideNormalBias: 0.04, startFlowWeight: 0.04, endFlowWeight: 0.08, flowShift: 0.02, arcScale: 0.24, scoreBias: 24},
		{id: "turn-primary-tight", family: "turn", side: ps, startReachScale: 1.26, endReachScale: 1.30, startLineWeight: -0.24, endLineWeight: -0.04, startHeadingWeight: 1.50, endHeadingWeight: 1.18, startNormalScale: 0.46, endNormalScale: 0.08, startGuideNormalBias: 0.30, endGuideNormalBias: 0.16, startFlowWeight: -0.30, endFlowWeight: 0.20, flowShift: -0.08, arcScale: ts, scoreBias: 40},
		{id: "turn-primary-wide", family: "turn", side: ps, startReachScale: 1.30, endReachScale: 1.36, startLineWeight: -0.28, endLineWeight: -0.10, startHeadingWeight: 1.54, endHeadingWeight: 1.24, startNormalScale: 0.58, endNormalScale: 0.12, startGuideNormalBias: 0.34, endGuideNormalBias: 0.20, startFlowWeight: -0.34, endFlowWeight: 0.24, flowShift: 0.06, arcScale: ts * 1.06, scoreBias: 46},
		{id: "brake-primary-tight", family: "brake", side: ps, startReachScale: 0.92, endReachScale: 1.42, startLineWeight: 0.50, endLineWeight: -0.20, startHeadingWeight: 0.70, endHeadingWeight: 1.52, startNormalScale: 0.16, endNormalScale: 0.20, startGuideNormalBias: 0.10, endGuideNormalBias: 0.26, startFlowWeight: 0.10, endFlowWeight: 0.32, flowShift: -0.04, arcScale: bs, scoreBias: 44},
		{id: "brake-primary-wide", family: "brake", side: ps, startReachScale: 0.98, endReachScale: 1.50, startLineWeight: 0.44, endLineWeight: -0.26, startHeadingWeight: 0.74, endHeadingWeight: 1.62, startNormalScale: 0.22, endNormalScale: 0.26, startGuideNormalBias: 0.12, endGuideNormalBias: 0.32, startFlowWeight: 0.14, endFlowWeight: 0.38, flowShift: 0.04, arcScale: bs * 1.04, scoreBias: 50},
		{id: "orbit-primary-tight", family: "orbit", side: ps, startReachScale: 0.90, endReachScale: 0.98, startLineWeight: 0.72, endLineWeight: 0.76, startHeadingWeight: 0.30, endHeadingWeight: 0.22, startNormalScale: 0.90, endNormalScale: 0.82, startGuideNormalBias: 0.16, endGuideNormalBias: 0.06, startFlowWeight: 0.26, endFlowWeight: 0.12, flowShift: -0.06, arcScale: os, scoreBias: 54},
		{id: "orbit-primary-wide", family: "orbit", side: ps, startReachScale: 0.94, endReachScale: 1.02, startLineWeight: 0.68, endLineWeight: 0.82, startHeadingWeight: 0.28, endHeadingWeight: 0.22, startNormalScale: 1.02, endNormalScale: 0.94, startGuideNormalBias: 0.18, endGuideNormalBias: 0.08, startFlowWeight: 0.30, endFlowWeight: 0.16, flowShift: 0.06, arcScale: os * 1.06, scoreBias: 60},
		{id: "turn-secondary", family: "turn", side: -ps, startReachScale: 1.18, endReachScale: 1.26, startLineWeight: -0.18, endLineWeight: 0.02, startHeadingWeight: 1.32, endHeadingWeight: 1.08, startNormalScale: 0.34, endNormalScale: 0.06, startGuideNormalBias: 0.22, endGuideNormalBias: 0.14, startFlowWeight: -0.20, endFlowWeight: 0.14, flowShift: 0.02, arcScale: ts * 0.92, scoreBias: 88},
		{id: "brake-secondary", family: "brake", side: -ps, startReachScale: 0.90, endReachScale: 1.34, startLineWeight: 0.52, endLineWeight: -0.16, startHeadingWeight: 0.62, endHeadingWeight: 1.40, startNormalScale: 0.12, endNormalScale: 0.18, startGuideNormalBias: 0.08, endGuideNormalBias: 0.20, startFlowWeight: 0.10, endFlowWeight: 0.28, flowShift: -0.02, arcScale: bs * 0.92, scoreBias: 96},
	}
}

func hdResolvedGuide(line, forward, normal Vec2, sideSign, lineW, headingW, normalBias float64) Vec2 {
	return hdNormVec(line.Scaled(lineW).Add(forward.Scaled(headingW)).Add(normal.Scaled(normalBias * sideSign)))
}

func hdMakePath(start, end Pt, m hdMotionMetrics, d hdMotionDescriptor, sf, ef Vec2) MotionPath {
	dist := m.distance
	dir := m.direction
	norm := m.normal
	resolvedFlow := clampF(hdDefaultArcFlow+d.flowShift, 0, 1)
	flowBias := (resolvedFlow - 0.5) * dist * 0.18
	baseStartReach := dist * (0.10 + hdDefaultStartHandle*0.56)
	baseEndReach := dist * (0.11 + hdDefaultEndHandle*0.62)
	distLift := 0.68 + m.farFactor*0.56
	baseArcH := clampF(dist*(0.10+hdDefaultArcSize*0.92)*d.arcScale*distLift, 20, dist*0.96)

	ss := float64(d.side)
	arcVec := Vec2{norm.Dx * baseArcH * ss, norm.Dy * baseArcH * ss}

	sg := hdResolvedGuide(dir, sf, norm, ss, d.startLineWeight, d.startHeadingWeight, d.startGuideNormalBias)
	eg := hdResolvedGuide(dir, ef, norm, ss, d.endLineWeight, d.endHeadingWeight, d.endGuideNormalBias)

	startReach := math.Max(baseStartReach*d.startReachScale+flowBias*d.startFlowWeight, 12)
	endReach := math.Max(baseEndReach*d.endReachScale-flowBias*d.endFlowWeight, 12)

	c1Base := start.AddVec(sg.Scaled(startReach))
	c2Base := end.SubVec(eg.Scaled(endReach))
	c1 := c1Base.AddVec(arcVec.Scaled(d.startNormalScale))
	c2 := c2Base.AddVec(arcVec.Scaled(d.endNormalScale))

	return MotionPath{
		Start: start, End: end,
		StartControl: &c1, EndControl: &c2,
		Segments:   []MotionSegment{{End: end, Control1: c1, Control2: c2}},
		CurveScale: baseArcH * math.Max(math.Abs(d.startNormalScale), math.Max(math.Abs(d.endNormalScale), 0.12)),
	}
}

type hdScoringContext struct {
	metrics       hdMotionMetrics
	startForward  Vec2
	endForward    Vec2
	preferredSide int
}

func (c hdScoringContext) turnDemand() float64 {
	return clampF(math.Abs(signedAngle(c.startForward, c.metrics.direction))/math.Pi, 0, 1)
}

func (c hdScoringContext) arrivalDemand() float64 {
	return clampF(math.Abs(signedAngle(c.metrics.direction, c.endForward))/math.Pi, 0, 1)
}

func (c hdScoringContext) directness() float64 {
	return clampF(1-math.Max(c.turnDemand(), c.arrivalDemand()*0.82), 0, 1)
}

func hdScoreCandidate(meas MotionMeasurement, path MotionPath, d hdMotionDescriptor, ctx hdScoringContext) float64 {
	dist := math.Max(ctx.metrics.distance, 1)
	exLR := math.Max(meas.Length/dist-1, 0)
	stTan := hdNormVec(path.tangentAt(0.04))
	enTan := hdNormVec(path.tangentAt(0.96))
	stErr := math.Abs(signedAngle(ctx.startForward, stTan))
	enErr := math.Abs(signedAngle(enTan, ctx.endForward))

	s := d.scoreBias
	s += exLR * 180
	s += meas.AngleChangeEnergy * 90
	s += meas.MaxAngleChange * 85
	turnW := 10.0
	if d.side != 0 {
		turnW = 12
	}
	s += meas.TotalTurn * turnW
	s += stErr * 150
	s += enErr * 120

	if d.side == 0 {
		s += ctx.turnDemand() * 130
		s += ctx.arrivalDemand() * 30
	} else {
		s += ctx.directness() * 90
		if d.side != ctx.preferredSide {
			s += math.Max(ctx.turnDemand(), 0.45) * 200
		}
	}
	switch d.family {
	case "turn":
		s += (1 - ctx.turnDemand()) * 55
	case "brake":
		s += (1 - ctx.arrivalDemand()) * 40
	case "orbit":
		s += ctx.directness() * 70
	case "direct":
		s += math.Max(ctx.turnDemand()-0.12, 0) * 80
	}
	if !meas.StaysInBounds {
		s += 90
	}
	return s
}

func (mp *MotionPath) tangentAt(progress float64) Vec2 {
	_, t := mp.Sample(progress)
	return t
}

func MakeHeadingDrivenCandidates(start, end Pt, bounds *Rect, startForward, endForward Vec2) []MotionCandidate {
	m := newHDMetrics(start, end)
	sf := hdNormVec(startForward)
	ef := hdNormVec(endForward)
	ps := hdPreferredTurnSide(m, sf, ef)
	ctx := hdScoringContext{metrics: m, startForward: sf, endForward: ef, preferredSide: ps}

	descs := hdDescriptors(m, ps)
	candidates := make([]MotionCandidate, 0, len(descs))
	for _, d := range descs {
		path := hdMakePath(start, end, m, d, sf, ef)
		meas := path.Measure(bounds, hdMinStepDistance)
		score := hdScoreCandidate(meas, path, d, ctx)
		candidates = append(candidates, MotionCandidate{
			Identifier: d.id, Kind: d.kind(), Side: d.side,
			Path: path, Measurement: meas, Score: score,
		})
	}
	return candidates
}

func ChooseHeadingDrivenBestCandidate(candidates []MotionCandidate) *MotionCandidate {
	if len(candidates) == 0 {
		return nil
	}
	inBounds := make([]MotionCandidate, 0)
	for _, c := range candidates {
		if c.Measurement.StaysInBounds {
			inBounds = append(inBounds, c)
		}
	}
	pool := candidates
	if len(inBounds) > 0 {
		pool = inBounds
	}
	best := &pool[0]
	for i := 1; i < len(pool); i++ {
		c := &pool[i]
		if c.Score < best.Score || (c.Score == best.Score && c.Identifier < best.Identifier) {
			best = c
		}
	}
	// Return a copy so caller owns it
	cp := *best
	return &cp
}

// --- Visual Dynamics ---

type VisualDynamicsConfig struct {
	TipSpring, AngleSpring           SpringConfig
	HeadingVelocityFloor             float64
	AnimatedAngleOffsetMax           float64
	BodyOffsetScale, BodyOffsetMax   float64
	BodyLateralScale, BodyLateralMax float64
	FogOffsetScale, FogOffsetMax     float64
	FogOpacityBase                   float64
	FogOpacityVelocityScale          float64
	FogScaleVelocityScale            float64
	FogScaleMaxDelta                 float64
}

func DefaultVisualDynamicsConfig() VisualDynamicsConfig {
	return VisualDynamicsConfig{
		TipSpring:               newSpringConfig(0.18, 0.76, 1.0/240.0, 28800),
		AngleSpring:             newSpringConfig(0.24, 0.82, 1.0/240.0, 28800),
		HeadingVelocityFloor:    14,
		AnimatedAngleOffsetMax:  0.26,
		BodyOffsetScale:         0.0012,
		BodyOffsetMax:           2.4,
		BodyLateralScale:        0.06,
		BodyLateralMax:          1.4,
		FogOffsetScale:          0.0045,
		FogOffsetMax:            9,
		FogOpacityBase:          0.12,
		FogOpacityVelocityScale: 0.00006,
		FogScaleVelocityScale:   0.00012,
		FogScaleMaxDelta:        0.22,
	}
}

type VisualDynamicsState struct {
	Time                             float64
	TipPosition                      Pt
	TipVelocity, TipForce            Vec2
	Angle, AngleVelocity, AngleForce float64
}

func NewVisualDynamicsState(tip Pt, t float64) VisualDynamicsState {
	return VisualDynamicsState{Time: t, TipPosition: tip}
}

type VisualRenderState struct {
	TipPosition                      Pt
	Rotation                         float64 // radians, matching macOS
	CursorBodyOffDx, CursorBodyOffDy float64
	FogOffDx, FogOffDy               float64
	FogOpacity, FogScale             float64
}

func AdvanceVisualDynamics(
	state VisualDynamicsState, targetTip Pt,
	targetTime, idleAngleOffset, baseHeading, renderYMul float64,
	cfg VisualDynamicsConfig,
) (VisualDynamicsState, VisualRenderState) {
	if targetTime-state.Time > 1 {
		state.Time = targetTime - 1.0/60.0
	}

	dt := cfg.TipSpring.dt
	for state.Time < targetTime {
		half := dt * 0.5
		// Tip spring (VelocityVerlet)
		vhx := state.TipVelocity.Dx + state.TipForce.Dx*half
		vhy := state.TipVelocity.Dy + state.TipForce.Dy*half
		nx := state.TipPosition.X + vhx*dt
		ny := state.TipPosition.Y + vhy*dt
		fx := cfg.TipSpring.stiffness*(targetTip.X-nx) + (-cfg.TipSpring.drag)*vhx
		fy := cfg.TipSpring.stiffness*(targetTip.Y-ny) + (-cfg.TipSpring.drag)*vhy
		state.TipVelocity = Vec2{vhx + fx*half, vhy + fy*half}
		state.TipPosition = Pt{nx, ny}
		state.TipForce = Vec2{fx, fy}

		// Render velocity (apply y-axis multiplier)
		rv := Vec2{state.TipVelocity.Dx, state.TipVelocity.Dy * renderYMul}

		// Target angle
		speed := rv.Length()
		tgtAngle := 0.0
		if speed > cfg.HeadingVelocityFloor {
			tgtAngle = normalizeAngle(math.Atan2(rv.Dy, rv.Dx) - baseHeading)
		}

		// Angle spring (VelocityVerlet)
		aVH := state.AngleVelocity + state.AngleForce*half
		nAngle := normalizeAngle(state.Angle + aVH*dt)
		aErr := normalizeAngle(tgtAngle - nAngle)
		aF := aErr*cfg.AngleSpring.stiffness + (-cfg.AngleSpring.drag)*aVH
		state.AngleVelocity = aVH + aF*half
		state.Angle = normalizeAngle(nAngle)
		state.AngleForce = aF

		state.Time += dt
	}

	// Compute render state (faithful port of Swift renderState)
	rv := Vec2{state.TipVelocity.Dx, state.TipVelocity.Dy * renderYMul}
	speed := rv.Length()
	var dir Vec2
	if speed > 0.001 {
		dir = rv.Normalized()
	} else {
		dir = Vec2{math.Cos(baseHeading + idleAngleOffset), math.Sin(baseHeading + idleAngleOffset)}
	}
	bodyBackward := dir.Scaled(-math.Min(speed*cfg.BodyOffsetScale, cfg.BodyOffsetMax))
	latAmt := clampF(state.AngleVelocity*cfg.BodyLateralScale, -cfg.BodyLateralMax, cfg.BodyLateralMax)
	perp := dir.Perpendicular()
	bodyLateral := perp.Scaled(latAmt)
	bodyOff := bodyBackward.Add(bodyLateral)
	fogOff := dir.Scaled(-math.Min(speed*cfg.FogOffsetScale, cfg.FogOffsetMax)).Add(bodyLateral.Scaled(0.6))
	fogOp := math.Min(cfg.FogOpacityBase+speed*cfg.FogOpacityVelocityScale, 0.34)
	fogSc := 1 + math.Min(speed*cfg.FogScaleVelocityScale, cfg.FogScaleMaxDelta)

	clampedIdleOff := clampF(idleAngleOffset, -cfg.AnimatedAngleOffsetMax, cfg.AnimatedAngleOffsetMax)

	return state, VisualRenderState{
		TipPosition:     state.TipPosition,
		Rotation:        normalizeAngle(state.Angle + clampedIdleOff),
		CursorBodyOffDx: bodyOff.Dx,
		CursorBodyOffDy: bodyOff.Dy,
		FogOffDx:        fogOff.Dx,
		FogOffDy:        fogOff.Dy,
		FogOpacity:      fogOp,
		FogScale:        fogSc,
	}
}
