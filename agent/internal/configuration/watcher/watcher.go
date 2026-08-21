package watcher

import (
	"context"
	"os"
	"time"
)

const defaultInterval = time.Second

// FileWatcher polls a local file for size or modification-time changes.
// It uses os.Stat only — no Windows APIs and no network.
type FileWatcher struct {
	path     string
	interval time.Duration
	events   chan struct{}
}

func New(path string, interval time.Duration) *FileWatcher {
	if interval <= 0 {
		interval = defaultInterval
	}
	return &FileWatcher{
		path:     path,
		interval: interval,
		events:   make(chan struct{}, 1),
	}
}

func (w *FileWatcher) Events() <-chan struct{} {
	return w.events
}

func (w *FileWatcher) Run(ctx context.Context) error {
	if ctx == nil {
		return context.Canceled
	}
	prev, _ := stat(w.path)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cur, err := stat(w.path)
			if err != nil {
				continue
			}
			if changed(prev, cur) {
				prev = cur
				select {
				case w.events <- struct{}{}:
				default:
				}
			}
		}
	}
}

type fileSig struct {
	mod  time.Time
	size int64
	ok   bool
}

func stat(path string) (fileSig, error) {
	info, err := os.Stat(path)
	if err != nil {
		return fileSig{}, err
	}
	return fileSig{mod: info.ModTime(), size: info.Size(), ok: true}, nil
}

func changed(prev, cur fileSig) bool {
	if !prev.ok {
		return cur.ok
	}
	return !prev.mod.Equal(cur.mod) || prev.size != cur.size
}
