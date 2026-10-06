//go:build windows

package hotkey

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

// user32 GUI calls (RegisterHotKey, GetMessage, ...) aren't covered by
// golang.org/x/sys/windows — that package wraps kernel32/ntdll/ole32-style
// APIs (it does have CoInitializeEx, which platform_windows.go in the main
// package uses) but not user32's window/message-loop surface. Declared
// directly via syscall, same as any Win32 API this project needs that
// isn't already wrapped somewhere.
var (
	user32                 = syscall.NewLazyDLL("user32.dll")
	procRegisterHotKey     = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procPostThreadMessageW = user32.NewProc("PostThreadMessageW")

	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wmHotkey = 0x0312
	wmQuit   = 0x0012

	// hotkeyID is the atom RegisterHotKey associates with this
	// registration. Only one hotkey is ever registered per process (one
	// Registration is live at a time — applyHotkey in app.go closes the
	// previous one before registering a new one), so a fixed ID is fine.
	hotkeyID = 1
)

// msg mirrors Win32's MSG struct — only the fields GetMessage needs to
// fill in and the message-loop cares about are named precisely; the rest
// just need to occupy the right space.
type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// Registration is one active global hotkey. Close releases it.
type Registration struct {
	threadID uint32
	done     chan struct{}
}

// Register binds combo to fire callback whenever it's pressed, system-wide,
// regardless of which window (if any) has focus. Win32 delivers WM_HOTKEY
// to the thread that registered it — registering with hwnd=0 posts it to
// the calling thread's own message queue, so this needs no window, window
// class, or WndProc, just a thread that owns a message loop. That thread
// is a dedicated goroutine pinned with LockOSThread for the registration's
// whole lifetime (message queues are thread-affine in Win32 — same
// reasoning as this package's sibling platform_windows.go:lockRenderThread
// in the main package).
func Register(combo Combo, callback func()) (*Registration, error) {
	errCh := make(chan error, 1)
	threadIDCh := make(chan uint32, 1)
	done := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		threadIDCh <- getCurrentThreadID()

		ret, _, callErr := procRegisterHotKey.Call(0, hotkeyID, uintptr(combo.Modifiers), uintptr(combo.Key))
		if ret == 0 {
			errCh <- fmt.Errorf("RegisterHotKey: %w", callErr)
			return
		}
		defer procUnregisterHotKey.Call(0, hotkeyID)
		errCh <- nil

		for {
			var m msg
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			// GetMessage returns 0 on WM_QUIT, or a huge value (cast from
			// -1) on error — either way, stop.
			if ret == 0 || int32(ret) == -1 {
				return
			}
			if m.message == wmHotkey && m.wParam == hotkeyID {
				callback()
			}
		}
	}()

	if err := <-errCh; err != nil {
		close(done) // the goroutine already returned without closing it
		return nil, err
	}

	return &Registration{threadID: <-threadIDCh, done: done}, nil
}

// Close stops the hotkey's message loop (which unregisters the hotkey on
// its way out) and waits for it to exit. Safe to call more than once.
func (r *Registration) Close() {
	select {
	case <-r.done:
		return // already stopped
	default:
	}
	procPostThreadMessageW.Call(uintptr(r.threadID), wmQuit, 0, 0)
	<-r.done
}

func getCurrentThreadID() uint32 {
	ret, _, _ := procGetCurrentThreadId.Call()
	return uint32(ret)
}
