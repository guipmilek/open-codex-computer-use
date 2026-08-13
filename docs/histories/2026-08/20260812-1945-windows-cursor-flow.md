# Windows Cursor Flow (Visual Cursor Overlay) Implementation & Audit Fixes

## 原始诉求 / User Request

Study and implement the software cursor flow on Windows to match the macOS visual cursor overlay capabilities, addressing external audit findings for 1:1 renderer matrix transform, Win32 POINT ABI packing, GA_ROOT window normalization, SWP_NOZORDER preservation, Per-Monitor DPI V2 physical pixel coordinate contract, and easeInEaseOut fade animations.

## 主要改动 / Key Changes

1. **`apps/OpenComputerUseWindows/cursor_motion.go`**:
   - 1:1 Go port of macOS `CursorMotionModel.swift`.
   - Includes full 10 heading-driven descriptors, cubic Bézier path sampling, spring VelocityVerlet simulation, candidates scoring, and visual dynamics lag/fog computation.

2. **`apps/OpenComputerUseWindows/cursor_target.go`**:
   - Unified target resolver `resolveVisualCursorTarget()` ensuring the visual cursor animates to the exact same physical screen point used by the PowerShell UIA/Win32 action bridge.

3. **`apps/OpenComputerUseWindows/cursor_overlay.go`**:
   - Controller providing synchronous `MoveToAndWait`, `PulseClickAndWait`, `Settle`, and `Reset` API matching macOS overlay sequence.
   - `OPEN_COMPUTER_USE_VISUAL_CURSOR` environment variable gating (defaults to enabled).
   - Candidate trajectory selection filtered by constraint points hit-testing against `GA_ROOT` target window, excluding overlay's own HWND.
   - Work area clamping via `clampTipPosition` and multi-monitor union bounding via `motionBounds`.

4. **`apps/OpenComputerUseWindows/cursor_renderer_windows.go` & `cursor_renderer_test.go`**:
   - 1:1 inverse 2D affine matrix transform mapping `T(origin) * R(rotation) * S(scaleX, scaleY) * T(-center)` for anisotropic scaling + rotation in radians.
   - Motion and pulse compression scale factors without applying fog to the reference image.
   - Added golden test `TestGoldenInverseTransformMatchesForward` verifying forward vs inverse transform accuracy.

5. **`apps/OpenComputerUseWindows/cursor_overlay_windows.go` & `cursor_overlay_windows_test.go`**:
   - Win32 `WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW` overlay window driven on dedicated OS-locked thread (`runtime.LockOSThread()`).
   - Packed 64-bit `uintptr` `packPoint` helper for Win32 x64 ABI `POINT` struct value passing (`MonitorFromPoint` & `WindowFromPoint`).
   - Top-level target window normalization via `GetAncestor(hwnd, GA_ROOT)`.
   - `show()` uses `SWP_NOZORDER | SWP_NOACTIVATE | SWP_SHOWWINDOW` to preserve Z-order and prevent focus activation.
   - 120ms smoothstep easeInEaseOut curve fade out in `fadeOut(120)`.
   - Per-Monitor DPI Awareness (V2) process initialization matching physical screen pixel coordinate contract across 100%, 125%, 150%, 175%, and 200% DPI scales.
   - Integration tests in `cursor_overlay_windows_test.go` for packed POINT ABI, root window identification, instrumented synchronous frame commit gate (`FrameCommitted < ActionBegan < PulseBegan`), physical cursor stability, and click-through style.

6. **`apps/OpenComputerUseWindows/main.go`**:
   - Wired `click` and `set_value` actions to execute synchronous cursor movement before action execution and pulse/settle after action completion.
   - Wired `notifications/turn-ended` MCP notification to reset visual cursor state.

## 受影响文件 / Key Affected Files

- `apps/OpenComputerUseWindows/cursor_motion.go`
- `apps/OpenComputerUseWindows/cursor_target.go`
- `apps/OpenComputerUseWindows/cursor_overlay.go`
- `apps/OpenComputerUseWindows/cursor_overlay_windows.go`
- `apps/OpenComputerUseWindows/cursor_renderer_windows.go`
- `apps/OpenComputerUseWindows/cursor_overlay_stub.go`
- `apps/OpenComputerUseWindows/cursor_motion_test.go`
- `apps/OpenComputerUseWindows/cursor_target_test.go`
- `apps/OpenComputerUseWindows/cursor_overlay_test.go`
- `apps/OpenComputerUseWindows/cursor_renderer_test.go`

7. **Final Audit Blockers Resolved**:
   - **`platformWindowIDAtPoint` Point-Aware Z-Order Walk**: Implemented geometric point containment filtering via `GetWindowRect` during `GW_HWNDNEXT` iteration to skip out-of-bounds overlay/unrelated windows.
   - **Candidate Scoring Over Overlay**: Fixed candidate trajectory scoring when target window is below overlay by using point-aware `platformWindowIDAtPoint`, validated with `TestCandidateScoringWithVisibleOverlayOverTarget`.
   - **Three-Window Z-Order Integration**: Verified Z-order stack relations (`GetWindow(targetRoot, GW_HWNDPREV) == overlay` and `getPrevVisibleRootWindow(overlay) == unrelatedRoot`) across 3+ `setZOrder` calls and small movements ($\le 2\text{px}$).
   - **PowerShell DPI Diagnostics Output Suppression**: Added `$null =` output assignments for `Add-Type` and P/Invoke calls in `runtime.ps1` to prevent stdout JSON pollution; added real script diagnostic execution test `TestPowerShellDPIDiagnosticsUsingEmbeddedScript`.
   - **Multi-Toolchain & Cross-Arch Verification**: Re-verified clean compilation, test, and vet runs across Go 1.22 (`go1.22.12`), Go 1.26 (`go1.26.5`), and Windows ARM64 (`GOOS=windows GOARCH=arm64`).

8. **Phase 2 Audit & E2E Validation**:
   - **Screenshot Targeting**: `Capture-WindowPngBase64` rewritten in `runtime.ps1` to use native `PrintWindow(PW_RENDERFULLCONTENT)` instead of `CopyFromScreen` to strictly capture target window pixels (bypassing z-order occlusions).
   - **Performance Improvements**: Removed unused `fmt` import and moved `GetDesktopWindow` to global static variable in `cursor_overlay_windows.go`.
   - **E2E & Stress Validation Harness**: Added `cursor_audit_test.go` encompassing 15-phase audit for: sub-pixel precision rendering (<0.6px error margin over 50 positions), temporal strict ordering (Commit < Action < Pulse), physical cursor zero-drift validation, foreground/active-window anti-theft validation, and 200-cycle DIB memory stress test (no leaks/goroutine orphans).
