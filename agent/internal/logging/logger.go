package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxLogBytes = 20 * 1024 * 1024
	maxLogFiles = 5
)

type Options struct {
	Console     bool
	Level       slog.Level
	FilePath    string
	ServiceName string
	Version     string
}

type Logger struct {
	*slog.Logger
	file io.Closer
	el   eventLogger
}

func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func New(opts Options) (*Logger, error) {
	if strings.TrimSpace(opts.FilePath) == "" {
		return nil, errLogFileRequired()
	}
	if err := os.MkdirAll(filepath.Dir(opts.FilePath), 0o755); err != nil {
		return nil, err
	}

	writer, err := newRotatingWriter(opts.FilePath, maxLogBytes, maxLogFiles)
	if err != nil {
		return nil, err
	}

	el := openEventLog(opts.ServiceName)
	writer.onFail = func(writeErr error) {
		if el != nil {
			el.Error(eventLogWriteFailed, "log file write failed: "+writeErr.Error())
		}
	}

	handlerOpts := &slog.HandlerOptions{
		Level: opts.Level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
				return slog.Time(slog.TimeKey, a.Value.Time().UTC())
			}
			return a
		},
	}

	var next slog.Handler = slog.NewJSONHandler(writer, handlerOpts)
	if opts.Console {
		next = &fanoutHandler{handlers: []slog.Handler{
			next,
			slog.NewTextHandler(os.Stdout, handlerOpts),
		}}
	}

	handler := &lifecycleHandler{next: next, el: el}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	logger := slog.New(handler).With(
		"service", opts.ServiceName,
		"version", opts.Version,
		"hostname", hostname,
		"instance_id", newInstanceID(),
	)

	return &Logger{Logger: logger, file: writer, el: el}, nil
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	var fileErr error
	if l.file != nil {
		fileErr = l.file.Close()
	}
	var elErr error
	if l.el != nil {
		elErr = l.el.Close()
	}
	return errors.Join(fileErr, elErr)
}

func newInstanceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

func errLogFileRequired() error {
	return errors.New("log_file is required")
}

type fanoutHandler struct {
	handlers []slog.Handler
}

func (f *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range f.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (f *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: next}
}

func (f *fanoutHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return &fanoutHandler{handlers: next}
}

type lifecycleHandler struct {
	next slog.Handler
	el   eventLogger
}

func (h *lifecycleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *lifecycleHandler) Handle(ctx context.Context, r slog.Record) error {
	err := h.next.Handle(ctx, r)
	if h.el == nil {
		return err
	}
	eid, ok := lifecycleEventIDs[r.Message]
	if !ok {
		return err
	}
	switch {
	case r.Level >= slog.LevelError:
		h.el.Error(eid, r.Message)
	case r.Level >= slog.LevelWarn:
		h.el.Warn(eid, r.Message)
	default:
		h.el.Info(eid, r.Message)
	}
	return err
}

func (h *lifecycleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &lifecycleHandler{next: h.next.WithAttrs(attrs), el: h.el}
}

func (h *lifecycleHandler) WithGroup(name string) slog.Handler {
	return &lifecycleHandler{next: h.next.WithGroup(name), el: h.el}
}
