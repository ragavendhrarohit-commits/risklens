//go:build windows

package service

import (
	"context"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

func IsWindowsService() (bool, error) {
	return svc.IsWindowsService()
}

func Run(name string, h *Handler, console bool) error {
	if h == nil || h.App == nil {
		return errors.New("handler is required")
	}
	if name == "" {
		return errors.New("service name is required")
	}
	if console {
		return debug.Run(name, h)
	}
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if isService {
		return svc.Run(name, h)
	}
	return debug.Run(name, h)
}

func (h *Handler) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	if h == nil || h.App == nil {
		return true, 40
	}

	requests := make(chan Request, 1)
	done := make(chan uint32, 1)

	go func() {
		done <- executeLoop(h.App, h.Logger, h.Timeout, requests, func(state State, hint time.Duration) {
			changes <- toStatus(state, hint)
		})
	}()

	for {
		select {
		case errno := <-done:
			h.setExitCode(errno)
			return errno != 0, errno
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.SessionChange:
				sessionID := uint32(c.EventData)
				if h.Logger != nil {
					h.Logger.Info("Session notification received",
						"source", "scm",
						"event_type", c.EventType,
						"wts_session_id", sessionID,
						"sink_attached", h.SessionChange != nil,
					)
				}
				if h.SessionChange != nil {
					eventType := c.EventType
					go h.SessionChange.OnWindowsSessionChange(eventType, sessionID)
				}
			case svc.PreShutdown, svc.Stop, svc.Shutdown:
				req := Request{Cmd: CmdStop}
				if c.Cmd == svc.Shutdown || c.Cmd == svc.PreShutdown {
					req.Cmd = CmdShutdown
					if h.SessionChange != nil {
						if h.Logger != nil {
							h.Logger.Info("Session notification received",
								"source", "scm",
								"cmd", c.Cmd,
								"reason", "shutdown",
							)
						}
						flushTimeout := 8 * time.Second
						if h.Timeout > 0 && h.Timeout < flushTimeout {
							flushTimeout = h.Timeout
						}
						ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
						h.SessionChange.OnWindowsShutdown(ctx)
						cancel()
					}
				}
				select {
				case requests <- req:
				case errno := <-done:
					h.setExitCode(errno)
					return errno != 0, errno
				}
			}
		}
	}
}

func toStatus(state State, hint time.Duration) svc.Status {
	status := svc.Status{WaitHint: uint32(hint.Milliseconds())}
	switch state {
	case StateStartPending:
		status.State = svc.StartPending
		status.CheckPoint = 1
	case StateRunning:
		status.State = svc.Running
		status.Accepts = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPreShutdown | svc.AcceptSessionChange
		status.ProcessId = uint32(os.Getpid())
	case StateStopPending:
		status.State = svc.StopPending
		status.CheckPoint = 1
	default:
		status.State = svc.StartPending
		status.CheckPoint = 1
	}
	return status
}
