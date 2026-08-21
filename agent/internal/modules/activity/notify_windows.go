//go:build windows

package activity

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmWTSSessionChange   = 0x02B1
	wmClose              = 0x0010
	wmDestroy            = 0x0002
	wmQueryEndSession    = 0x0011
	wmEndSession         = 0x0016
	notifyForAllSessions = 1
	wsPopup              = 0x80000000
	wsExNoActivate       = 0x08000000
	wsExToolWindow       = 0x00000080
)

var (
	modUser32Notify                      = windows.NewLazySystemDLL("user32.dll")
	modWtsNotify                         = windows.NewLazySystemDLL("wtsapi32.dll")
	modKernelNotify                      = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassExW                 = modUser32Notify.NewProc("RegisterClassExW")
	procCreateWindowExW                  = modUser32Notify.NewProc("CreateWindowExW")
	procDestroyWindow                    = modUser32Notify.NewProc("DestroyWindow")
	procGetMessageW                      = modUser32Notify.NewProc("GetMessageW")
	procTranslateMessage                 = modUser32Notify.NewProc("TranslateMessage")
	procDispatchMessageW                 = modUser32Notify.NewProc("DispatchMessageW")
	procDefWindowProcW                   = modUser32Notify.NewProc("DefWindowProcW")
	procPostMessageW                     = modUser32Notify.NewProc("PostMessageW")
	procGetModuleHandleW                 = modKernelNotify.NewProc("GetModuleHandleW")
	procWTSRegisterSessionNotification   = modWtsNotify.NewProc("WTSRegisterSessionNotification")
	procWTSUnRegisterSessionNotification = modWtsNotify.NewProc("WTSUnRegisterSessionNotification")
	procPostQuitMessage                  = modUser32Notify.NewProc("PostQuitMessage")
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

var notifyCallbackMu sync.Mutex
var notifyOwner *Module

func (m *Module) startSessionNotify() {
	if m == nil {
		return
	}
	m.notifyOnce.Do(func() {
		m.notifyDone = make(chan struct{})
		go m.sessionNotifyLoop()
	})
}

func (m *Module) stopSessionNotify() {
	if m == nil {
		return
	}
	hwnd := windows.Handle(m.notifyHwnd.Load())
	if hwnd != 0 {
		procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	}
	if m.notifyDone == nil {
		return
	}
	select {
	case <-m.notifyDone:
	case <-time.After(2 * time.Second):
	}
}

func (m *Module) sessionNotifyLoop() {
	defer close(m.notifyDone)

	className, err := windows.UTF16PtrFromString("RiskLensWTSNotify-" + strconv.Itoa(os.Getpid()))
	if err != nil {
		return
	}
	title, err := windows.UTF16PtrFromString("RiskLens Session Notify")
	if err != nil {
		return
	}

	instance, _, _ := procGetModuleHandleW.Call(0)
	wndProc := windows.NewCallback(sessionNotifyWndProc)

	notifyCallbackMu.Lock()
	notifyOwner = m
	notifyCallbackMu.Unlock()

	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   wndProc,
		Instance:  windows.Handle(instance),
		ClassName: className,
	}
	atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		if m.logger != nil {
			m.logger.Warn("Activity session notify window class not registered")
		}
		return
	}

	// HWND_MESSAGE windows do not receive WM_WTSSESSION_CHANGE or WM_ENDSESSION.
	// A hidden top-level popup is required for both console and service mode.
	hwnd, _, createErr := procCreateWindowExW.Call(
		wsExNoActivate|wsExToolWindow,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		0, 0, 0, 0,
		0,
		0,
		instance,
		0,
	)
	if hwnd == 0 {
		if m.logger != nil {
			m.logger.Warn("Activity session notify window not created", "error", createErr)
		}
		return
	}
	m.notifyHwnd.Store(uintptr(hwnd))

	r1, _, regErr := procWTSRegisterSessionNotification.Call(hwnd, notifyForAllSessions)
	if r1 == 0 {
		if m.logger != nil {
			m.logger.Warn("WTSRegisterSessionNotification failed", "error", regErr)
		}
	} else if m.logger != nil {
		m.logger.Info("WTSRegisterSessionNotification registered",
			"hwnd", hwnd,
			"notify", "all_sessions",
		)
	}

	var message msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&message)))
	}

	procWTSUnRegisterSessionNotification.Call(hwnd)
	procDestroyWindow.Call(hwnd)
	m.notifyHwnd.Store(0)
}

func sessionNotifyWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmWTSSessionChange:
		notifyCallbackMu.Lock()
		owner := notifyOwner
		notifyCallbackMu.Unlock()
		if owner != nil {
			owner.handleSessionChange("wts", uint32(wparam), uint32(lparam))
		}
		return 0
	case wmQueryEndSession:
		notifyCallbackMu.Lock()
		owner := notifyOwner
		notifyCallbackMu.Unlock()
		if owner != nil && owner.logger != nil {
			owner.logger.Info("Session notification received",
				"source", "end_session",
				"event", "query_end_session",
			)
		}
		return 1
	case wmEndSession:
		if wparam != 0 {
			notifyCallbackMu.Lock()
			owner := notifyOwner
			notifyCallbackMu.Unlock()
			if owner != nil {
				if owner.logger != nil {
					owner.logger.Info("Session notification received",
						"source", "end_session",
						"event", "end_session",
					)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				_ = owner.CloseSession(ctx, "end_session")
				cancel()
			}
		}
		return 0
	case wmClose:
		procWTSUnRegisterSessionNotification.Call(hwnd)
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	default:
		r1, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
		return r1
	}
}
