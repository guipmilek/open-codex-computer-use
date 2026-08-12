# Windows Cursor Flow (Visual Cursor Overlay) Implementation

## 原始诉求 / User Request

Study and implement the software cursor flow on Windows to match the macOS visual cursor overlay capabilities.

## 主要改动 / Key Changes

1. **`apps/OpenComputerUseWindows/cursor_motion.go`**:
   - 1:1 Go port of macOS `CursorMotionModel.swift`.
   - Includes full 10 heading-driven descriptors, cubic Bézier path sampling, spring VelocityVerlet simulation, candidates scoring, and visual dynamics lag/fog computation.

2. **`apps/OpenComputerUseWindows/cursor_target.go`**:
   - Unified target resolver `resolveVisualCursorTarget()` ensuring the visual cursor animates to the exact same screen point used by the PowerShell UIA/Win32 action.

3. **`apps/OpenComputerUseWindows/cursor_overlay.go`**:
   - Controller providing synchronous `MoveToAndWait`, `PulseClickAndWait`, `Settle`, and `Reset` API matching macOS overlay sequence.
   - `OPEN_COMPUTER_USE_VISUAL_CURSOR` environment variable gating (defaults to enabled).

4. **`apps/OpenComputerUseWindows/cursor_overlay_windows.go` & `cursor_renderer_windows.go`**:
   - Win32 `WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_NOACTIVATE | WS_EX_TOOLWINDOW` overlay window driven on a dedicated OS-locked thread (`runtime.LockOSThread()`).
   - `UpdateLayeredWindow` per-pixel alpha rendering with 32-bit DIB section, 126x126 canvas, and embedded official `official-software-cursor-window-252.png` glyph.
   - Graceful degradation for headless/SSH non-desktop environments.

5. **`apps/OpenComputerUseWindows/main.go`**:
   - Wired `click` and `set_value` actions to execute synchronous cursor movement before action execution and pulse/settle after action completion.
   - Wired `notifications/turn-ended` MCP notification to reset visual cursor state.

6. **Unit Tests & Documentation**:
   - Added unit tests in `cursor_motion_test.go`, `cursor_target_test.go`, and `cursor_overlay_test.go`.
   - Updated `docs/ARCHITECTURE.md` and `docs/exec-plans/active/20260422-windows-computer-use-runtime.md`.

## 受影响文件 / Key Affected Files

- `apps/OpenComputerUseWindows/cursor_motion.go`
- `apps/OpenComputerUseWindows/cursor_target.go`
- `apps/OpenComputerUseWindows/cursor_overlay.go`
- `apps/OpenComputerUseWindows/cursor_overlay_windows.go`
- `apps/OpenComputerUseWindows/cursor_renderer_windows.go`
- `apps/OpenComputerUseWindows/cursor_overlay_stub.go`
- `apps/OpenComputerUseWindows/main.go`
- `apps/OpenComputerUseWindows/official-software-cursor-window-252.png`
- `docs/ARCHITECTURE.md`
- `docs/exec-plans/active/20260422-windows-computer-use-runtime.md`
