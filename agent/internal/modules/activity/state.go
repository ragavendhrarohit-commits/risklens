package activity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const stateFileName = "activity.json"

type stepResult struct {
	Next     persistedState
	Sessions []SessionItem
	Apps     []ActivityItem
	Changed  bool
}

func loadState(path string) (persistedState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return persistedState{}, nil
		}
		return persistedState{}, err
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return persistedState{}, err
	}
	return state, nil
}

func saveState(path string, state persistedState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func statePath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "RiskLens", stateFileName)
	}
	return filepath.Join(stateDir, stateFileName)
}

func sameSession(prev persistedState, live *LiveSession) bool {
	if live == nil || !prev.hasSession() {
		return false
	}
	return prev.WTSSessionID == live.WTSSessionID && prev.LogonAt.Equal(live.LogonAt.UTC())
}

func sameApp(prev persistedState, live *LiveApp) bool {
	if live == nil || !prev.hasApp() {
		return false
	}
	return strings.EqualFold(prev.ExecutableName, live.ExecutableName) &&
		prev.WindowTitle == live.WindowTitle
}

func closeSession(prev persistedState, now time.Time) (SessionItem, ActivityItem, bool) {
	item := SessionItem{
		SessionID:  prev.SessionID,
		Username:   prev.Username,
		AccountSID: prev.AccountSID,
		LogonAt:    prev.LogonAt,
		LogoffAt:   &now,
		Status:     StatusClosed,
	}
	var app ActivityItem
	hasApp := prev.hasApp()
	if hasApp {
		app = ActivityItem{
			ID:              prev.AppID,
			SessionID:       prev.SessionID,
			ApplicationName: prev.ApplicationName,
			ExecutableName:  prev.ExecutableName,
			WindowTitle:     prev.WindowTitle,
			StartedAt:       prev.StartedAt,
			StoppedAt:       &now,
			DurationSeconds: durationSeconds(prev.StartedAt, now),
		}
	}
	return item, app, hasApp
}

func openSessionItem(id string, live LiveSession) SessionItem {
	return SessionItem{
		SessionID:  id,
		Username:   live.Username,
		AccountSID: live.AccountSID,
		LogonAt:    live.LogonAt,
		LogoffAt:   nil,
		Status:     StatusActive,
	}
}

func openAppItem(sessionID string, live LiveApp, startedAt time.Time, now time.Time) ActivityItem {
	exe := strings.TrimSpace(live.ExecutableName)
	name := strings.TrimSpace(live.ApplicationName)
	if name == "" {
		name = applicationName(exe, live.WindowTitle)
	}
	return ActivityItem{
		ID:              activityID(sessionID, exe, startedAt),
		SessionID:       sessionID,
		ApplicationName: name,
		ExecutableName:  exe,
		WindowTitle:     strings.TrimSpace(live.WindowTitle),
		StartedAt:       startedAt,
		StoppedAt:       nil,
		DurationSeconds: durationSeconds(startedAt, now),
	}
}

func refreshOpenApp(prev persistedState, now time.Time) ActivityItem {
	return ActivityItem{
		ID:              prev.AppID,
		SessionID:       prev.SessionID,
		ApplicationName: prev.ApplicationName,
		ExecutableName:  prev.ExecutableName,
		WindowTitle:     prev.WindowTitle,
		StartedAt:       prev.StartedAt,
		StoppedAt:       nil,
		DurationSeconds: durationSeconds(prev.StartedAt, now),
	}
}

// step applies one poll to persisted state and returns upload rows.
func step(prev persistedState, obs Observation, agentID string, now time.Time) stepResult {
	now = now.UTC()
	live := obs.Session
	if live != nil {
		liveCopy := *live
		liveCopy.LogonAt = liveCopy.LogonAt.UTC()
		if liveCopy.LogonAt.IsZero() && sameWTS(prev, liveCopy.WTSSessionID) && !prev.LogonAt.IsZero() {
			liveCopy.LogonAt = prev.LogonAt
		}
		liveCopy.Username = strings.TrimSpace(liveCopy.Username)
		liveCopy.AccountSID = strings.TrimSpace(liveCopy.AccountSID)
		if liveCopy.LogonAt.IsZero() {
			return stepResult{Next: prev}
		}
		live = &liveCopy
	}

	if live == nil {
		if !prev.hasSession() {
			return stepResult{Next: prev}
		}
		if prev.alreadyClosedLocally(prev.SessionID) {
			return stepResult{Next: rememberClosed(prev, prev.SessionID, prev.LogonAt), Changed: true}
		}
		closed, app, hasApp := closeSession(prev, now)
		result := stepResult{
			Changed:  true,
			Sessions: []SessionItem{closed},
			Next:     rememberClosed(prev, prev.SessionID, prev.LogonAt),
		}
		if hasApp {
			result.Apps = []ActivityItem{app}
		}
		return result
	}

	sid := sessionID(agentID, live.WTSSessionID, live.LogonAt)
	if prev.alreadyClosedLocally(sid) && !prev.hasSession() {
		return stepResult{Next: prev}
	}
	if !prev.hasSession() && sid == prev.LastClosedSessionID {
		return stepResult{Next: prev}
	}
	next := persistedState{
		SessionID:           sid,
		WTSSessionID:        live.WTSSessionID,
		Username:            live.Username,
		AccountSID:          live.AccountSID,
		LogonAt:             live.LogonAt,
		LastClosedSessionID: prev.LastClosedSessionID,
		LastClosedLogonAt:   prev.LastClosedLogonAt,
		UploadedClosedIDs:   append([]string(nil), prev.UploadedClosedIDs...),
	}

	if prev.hasSession() && !sameSession(prev, live) {
		closedAt := previousCloseAt(prev, live, now)
		closed, app, hasApp := closeSession(prev, closedAt)
		next.LastClosedSessionID = prev.SessionID
		next.LastClosedLogonAt = prev.LogonAt
		result := stepResult{
			Changed:  true,
			Sessions: []SessionItem{closed, openSessionItem(sid, *live)},
			Next:     next,
		}
		if hasApp {
			result.Apps = append(result.Apps, app)
		}
		if obs.App != nil && strings.TrimSpace(obs.App.ExecutableName+obs.App.WindowTitle+obs.App.ApplicationName) != "" {
			started := now
			item := openAppItem(sid, *obs.App, started, now)
			result.Apps = append(result.Apps, item)
			result.Next.AppID = item.ID
			result.Next.ApplicationName = item.ApplicationName
			result.Next.ExecutableName = item.ExecutableName
			result.Next.WindowTitle = item.WindowTitle
			result.Next.StartedAt = started
		}
		return result
	}

	result := stepResult{
		Next:     next,
		Sessions: []SessionItem{openSessionItem(sid, *live)},
	}
	if prev.hasSession() && sameSession(prev, live) {
		result.Changed = false
	} else {
		result.Changed = true
	}

	if obs.App == nil || strings.TrimSpace(obs.App.ExecutableName+obs.App.WindowTitle+obs.App.ApplicationName) == "" {
		if prev.hasApp() && sameSession(prev, live) {
			_, app, hasApp := closeSession(prev, now)
			if hasApp {
				result.Apps = append(result.Apps, app)
				result.Changed = true
			}
		}
		return result
	}

	if sameSession(prev, live) && sameApp(prev, obs.App) {
		result.Next.AppID = prev.AppID
		result.Next.ApplicationName = prev.ApplicationName
		result.Next.ExecutableName = prev.ExecutableName
		result.Next.WindowTitle = prev.WindowTitle
		result.Next.StartedAt = prev.StartedAt
		result.Apps = append(result.Apps, refreshOpenApp(result.Next, now))
		return result
	}

	if sameSession(prev, live) && prev.hasApp() && !sameApp(prev, obs.App) {
		_, app, hasApp := closeSession(prev, now)
		if hasApp {
			result.Apps = append(result.Apps, app)
		}
		result.Changed = true
	} else if !prev.hasApp() || !sameSession(prev, live) {
		result.Changed = true
	}

	started := now
	item := openAppItem(sid, *obs.App, started, now)
	result.Apps = append(result.Apps, item)
	result.Next.AppID = item.ID
	result.Next.ApplicationName = item.ApplicationName
	result.Next.ExecutableName = item.ExecutableName
	result.Next.WindowTitle = item.WindowTitle
	result.Next.StartedAt = started
	return result
}

func sameWTS(prev persistedState, wts uint32) bool {
	return prev.hasSession() && prev.WTSSessionID == wts
}

func previousCloseAt(prev persistedState, live *LiveSession, now time.Time) time.Time {
	if live == nil || live.LogonAt.IsZero() {
		return now
	}
	at := live.LogonAt.UTC()
	if !prev.LogonAt.IsZero() && at.Before(prev.LogonAt) {
		return now
	}
	return at
}
