//go:build windows

package main

import (
	"runtime"
	"testing"
)

// Win32 windows are owned by the OS thread that creates them. These wrappers
// keep the entire integration scenario (creation, Z-order operations, and
// deferred destruction) on one OS thread so the tests exercise Win32 rather
// than Go scheduler thread migration.
func runWin32TestOnLockedThread(t *testing.T, test func(*testing.T)) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	test(t)
}

func TestLockedPlatformWindowIDAtPointOverlappingGeometry(t *testing.T) {
	runWin32TestOnLockedThread(t, TestPlatformWindowIDAtPointOverlappingGeometry)
}

func TestLockedThreeWindowZOrderIntegration(t *testing.T) {
	runWin32TestOnLockedThread(t, TestThreeWindowZOrderIntegration)
}

func TestLockedCandidateScoringWithVisibleOverlayOverNormalTarget(t *testing.T) {
	runWin32TestOnLockedThread(t, TestCandidateScoringWithVisibleOverlayOverNormalTarget)
}

func TestLockedCandidateScoringWithVisibleOverlayOverTopmostTarget(t *testing.T) {
	runWin32TestOnLockedThread(t, TestCandidateScoringWithVisibleOverlayOverTopmostTarget)
}

func TestLockedOverlayTransitionTopmostToNormalTarget(t *testing.T) {
	runWin32TestOnLockedThread(t, TestOverlayTransitionTopmostToNormalTarget)
}
