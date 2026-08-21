//go:build !windows

package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func IsWindowsService() (bool, error) {
	return false, nil
}

func Run(name string, h *Handler, console bool) error {
	if h == nil || h.App == nil {
		return errors.New("handler is required")
	}
	if name == "" {
		return errors.New("service name is required")
	}

	requests := make(chan Request, 1)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		select {
		case requests <- Request{Cmd: CmdStop}:
		default:
		}
	}()

	errno := executeLoop(h.App, h.Logger, h.Timeout, requests, func(State, time.Duration) {})
	stop()
	h.setExitCode(errno)
	if errno != 0 {
		return fmt.Errorf("application exit code %d", errno)
	}
	return nil
}
