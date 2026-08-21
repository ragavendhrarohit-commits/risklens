package browseractivity

import (
	"strings"
	"time"
)

type stepResult struct {
	Next    persistedState
	Items   []BrowserItem
	Changed bool
	Dropped bool
}

func closeItem(prev persistedState, at time.Time) BrowserItem {
	return BrowserItem{
		ID:              prev.ID,
		SessionID:       prev.SessionID,
		ExecutableName:  prev.ExecutableName,
		Domain:          prev.Domain,
		URL:             prev.URL,
		Title:           prev.Title,
		Profile:         prev.Profile,
		WindowID:        prev.WindowID,
		TabID:           prev.TabID,
		StartedAt:       prev.StartedAt,
		StoppedAt:       &at,
		DurationSeconds: durationSeconds(prev.StartedAt, at),
	}
}

func openItem(sessionID string, sample Sample, startedAt, now time.Time) BrowserItem {
	return BrowserItem{
		ID:              browserActivityID(sessionID, sample.Executable, sample.Domain, sample.WindowID, sample.TabID, startedAt),
		SessionID:       sessionID,
		ExecutableName:  sample.Executable,
		Domain:          sample.Domain,
		URL:             sample.URL,
		Title:           sample.Title,
		Profile:         sample.Profile,
		WindowID:        sample.WindowID,
		TabID:           sample.TabID,
		StartedAt:       startedAt,
		StoppedAt:       nil,
		DurationSeconds: durationSeconds(startedAt, now),
	}
}

func refreshOpen(prev persistedState, now time.Time) BrowserItem {
	return BrowserItem{
		ID:              prev.ID,
		SessionID:       prev.SessionID,
		ExecutableName:  prev.ExecutableName,
		Domain:          prev.Domain,
		URL:             prev.URL,
		Title:           prev.Title,
		Profile:         prev.Profile,
		WindowID:        prev.WindowID,
		TabID:           prev.TabID,
		StartedAt:       prev.StartedAt,
		StoppedAt:       nil,
		DurationSeconds: durationSeconds(prev.StartedAt, now),
	}
}

// step applies one observation. Browser Activity never creates application
// seconds; it only opens and closes label rows.
func step(prev persistedState, fg Foreground, sample *Sample, now time.Time) stepResult {
	now = now.UTC()
	sessionID := strings.TrimSpace(fg.SessionID)

	if sample != nil {
		if !AllowSample(fg, *sample) {
			if prev.open() && !IsBrowserExecutable(fg.ExecutableName) {
				closed := closeItem(prev, now)
				return stepResult{Items: []BrowserItem{closed}, Changed: true, Dropped: true}
			}
			return stepResult{Next: prev, Dropped: true}
		}
		at := sample.At.UTC()
		if at.IsZero() {
			at = now
		}
		if sameLabel(prev, *sample, sessionID) {
			next := prev
			next.Title = sample.Title
			next.URL = sample.URL
			next.Profile = sample.Profile
			return stepResult{
				Next:    next,
				Items:   []BrowserItem{refreshOpen(next, now)},
				Changed: prev.Title != sample.Title,
			}
		}
		var items []BrowserItem
		if prev.open() {
			closeAt := at
			if closeAt.Before(prev.StartedAt) {
				closeAt = now
			}
			items = append(items, closeItem(prev, closeAt))
		}
		opened := openItem(sessionID, *sample, at, now)
		next := persistedState{
			ID:             opened.ID,
			SessionID:      sessionID,
			ExecutableName: opened.ExecutableName,
			Domain:         opened.Domain,
			URL:            opened.URL,
			Title:          opened.Title,
			Profile:        opened.Profile,
			WindowID:       opened.WindowID,
			TabID:          opened.TabID,
			StartedAt:      at,
		}
		items = append(items, opened)
		return stepResult{Next: next, Items: items, Changed: true}
	}

	if prev.open() && !IsBrowserExecutable(fg.ExecutableName) {
		return stepResult{Items: []BrowserItem{closeItem(prev, now)}, Changed: true}
	}
	if prev.open() && sessionID != "" && prev.SessionID == sessionID &&
		NormalizeExecutable(prev.ExecutableName) == NormalizeExecutable(fg.ExecutableName) {
		return stepResult{Next: prev, Items: []BrowserItem{refreshOpen(prev, now)}}
	}
	return stepResult{Next: prev}
}
