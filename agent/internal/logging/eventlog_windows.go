//go:build windows

package logging

import "golang.org/x/sys/windows/svc/eventlog"

type windowsEventLog struct {
	el *eventlog.Log
}

func openEventLog(source string) eventLogger {
	if source == "" {
		return nil
	}
	el, err := eventlog.Open(source)
	if err != nil {
		if instErr := eventlog.InstallAsEventCreate(source, eventlog.Error|eventlog.Warning|eventlog.Info); instErr != nil {
			return nil
		}
		el, err = eventlog.Open(source)
		if err != nil {
			return nil
		}
	}
	return &windowsEventLog{el: el}
}

func (w *windowsEventLog) Info(eid uint32, msg string) {
	_ = w.el.Info(eid, msg)
}

func (w *windowsEventLog) Warn(eid uint32, msg string) {
	_ = w.el.Warning(eid, msg)
}

func (w *windowsEventLog) Error(eid uint32, msg string) {
	_ = w.el.Error(eid, msg)
}

func (w *windowsEventLog) Close() error {
	return w.el.Close()
}
