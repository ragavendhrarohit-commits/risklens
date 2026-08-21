package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type State int

const (
	StateStartPending State = iota
	StateRunning
	StateStopPending
	StateStopped
)

type Command int

const (
	CmdStop Command = iota
	CmdShutdown
	CmdInterrogate
)

type Request struct {
	Cmd     Command
	Current State
}

func executeLoop(application Application, logger *slog.Logger, timeout time.Duration, requests <-chan Request, report func(State, time.Duration)) uint32 {
	if logger == nil {
		logger = slog.Default()
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if application == nil {
		logger.Error("application is required", "component", "service")
		return 40
	}

	const startHint = 30 * time.Second
	if report != nil {
		report(StateStartPending, startHint)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				errc <- fmt.Errorf("application panic: %v", rec)
			}
		}()
		errc <- application.Run(runCtx)
	}()

	if report != nil {
		report(StateRunning, 0)
	}

	for {
		select {
		case err := <-errc:
			return mapAppError(logger, err)
		case req := <-requests:
			switch req.Cmd {
			case CmdInterrogate:
				if report != nil {
					report(req.Current, 0)
				}
			case CmdStop, CmdShutdown:
				if report != nil {
					report(StateStopPending, timeout)
				}
				cancel()
				timer := time.NewTimer(timeout + time.Second)
				select {
				case err := <-errc:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return mapAppError(logger, err)
				case <-timer.C:
					logger.Error("shutdown deadline exceeded", "component", "service")
					return 50
				}
			}
		}
	}
}

func mapAppError(logger *slog.Logger, err error) uint32 {
	if err == nil {
		return 0
	}
	if isShutdownTimeout(err) {
		logger.Error("shutdown deadline exceeded", "component", "service", "error", err)
		return 50
	}
	logger.Error("application exited", "component", "service", "error", err)
	return 40
}

func isShutdownTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), "shutdown deadline exceeded")
}
