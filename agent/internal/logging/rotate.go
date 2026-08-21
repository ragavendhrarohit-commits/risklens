package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const writeRetryInterval = 5 * time.Second

type rotatingWriter struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	keep     int
	file     *os.File
	size     int64
	lastFail time.Time
	closed   bool
	onFail   func(error)
}

func newRotatingWriter(path string, maxBytes int64, keep int) (*rotatingWriter, error) {
	w := &rotatingWriter{
		path:     path,
		maxBytes: maxBytes,
		keep:     keep,
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return 0, os.ErrClosed
	}

	var failErr error
	if w.file == nil {
		if !w.lastFail.IsZero() && time.Since(w.lastFail) < writeRetryInterval {
			w.mu.Unlock()
			return len(p), nil
		}
		if err := w.open(); err != nil {
			failErr = err
			w.markFailedLocked(err)
		}
	}

	if failErr == nil {
		if err := w.rotateIfNeeded(int64(len(p))); err != nil {
			failErr = err
			w.markFailedLocked(err)
		}
	}

	n := 0
	if failErr == nil {
		var err error
		n, err = w.file.Write(p)
		if err != nil {
			failErr = err
			w.markFailedLocked(err)
		} else {
			w.size += int64(n)
			w.lastFail = time.Time{}
		}
	}

	onFail := w.onFail
	w.mu.Unlock()

	if failErr != nil && onFail != nil {
		onFail(failErr)
		return len(p), nil
	}
	if failErr != nil {
		return len(p), nil
	}
	return n, nil
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *rotatingWriter) open() error {
	if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}

func (w *rotatingWriter) rotateIfNeeded(incoming int64) error {
	if w.file == nil || w.maxBytes <= 0 || w.size+incoming < w.maxBytes {
		return nil
	}
	if err := w.file.Close(); err != nil {
		w.file = nil
		return err
	}
	w.file = nil

	for i := w.keep - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		_ = os.Remove(dst)
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	rotated := w.path + ".1"
	_ = os.Remove(rotated)
	if err := os.Rename(w.path, rotated); err != nil && !os.IsNotExist(err) {
		return err
	}
	return w.open()
}

func (w *rotatingWriter) markFailedLocked(err error) {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	w.lastFail = time.Now()
}
