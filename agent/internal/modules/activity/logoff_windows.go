//go:build windows

package activity

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	ctrlLogoffEvent   = 5
	ctrlShutdownEvent = 6
)

var (
	modKernel32Logoff         = windows.NewLazySystemDLL("kernel32.dll")
	procSetConsoleCtrlHandler = modKernel32Logoff.NewProc("SetConsoleCtrlHandler")
	logoffHookOnce            sync.Once
	logoffModule              *Module
)

func (m *Module) installLogoffHook() {
	logoffModule = m
	logoffHookOnce.Do(func() {
		cb := windows.NewCallback(func(ctrlType uint32) uintptr {
			if ctrlType != ctrlLogoffEvent && ctrlType != ctrlShutdownEvent {
				return 0
			}
			mod := logoffModule
			if mod == nil {
				return 0
			}
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			reason := "console_ctrl_logoff"
			if ctrlType == ctrlShutdownEvent {
				reason = "console_ctrl_shutdown"
			}
			if mod.logger != nil {
				mod.logger.Info("Session notification received",
					"source", "console_ctrl",
					"event_type", ctrlType,
					"reason", reason,
				)
			}
			_ = mod.CloseSession(ctx, reason)
			return 1
		})
		r1, _, err := procSetConsoleCtrlHandler.Call(cb, 1)
		if r1 == 0 && m.logger != nil {
			m.logger.Warn("Activity logoff hook not installed", "error", err)
		}
	})
}
