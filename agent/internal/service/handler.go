package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Application is the process body started by the Windows service supervisor.
// The service package depends on this interface, not a concrete app type.
type Application interface {
	Run(ctx context.Context) error
}

// SessionChangeSink receives native Windows session-change and shutdown
// notifications. Implemented by the activity module. Optional.
type SessionChangeSink interface {
	OnWindowsSessionChange(eventType uint32, sessionID uint32)
	OnWindowsShutdown(ctx context.Context)
}

type Handler struct {
	App           Application
	Logger        *slog.Logger
	Timeout       time.Duration
	SessionChange SessionChangeSink

	lastErrno atomic.Uint32
}

func (h *Handler) ExitCode() uint32 {
	if h == nil {
		return 0
	}
	return h.lastErrno.Load()
}

func (h *Handler) setExitCode(code uint32) {
	if h == nil {
		return
	}
	h.lastErrno.Store(code)
}
