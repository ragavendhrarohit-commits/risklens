package activity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	idmodel "risklens/agent/internal/identity/model"
)

const (
	queueFileName = "session_upload_queue.json"
	maxQueueItems = 32
)

type queuedPayload struct {
	Kind         string              `json:"kind"`
	Sessions     *SessionUpload      `json:"sessions,omitempty"`
	Applications *ApplicationUpload  `json:"applications,omitempty"`
	EnqueuedAt   time.Time           `json:"enqueued_at"`
}

type uploadQueue struct {
	Items []queuedPayload `json:"items"`
}

func queuePath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "RiskLens", queueFileName)
	}
	return filepath.Join(stateDir, queueFileName)
}

func loadQueue(path string) uploadQueue {
	data, err := os.ReadFile(path)
	if err != nil {
		return uploadQueue{}
	}
	var q uploadQueue
	if err := json.Unmarshal(data, &q); err != nil {
		return uploadQueue{}
	}
	return q
}

func saveQueue(path string, q uploadQueue) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (q *uploadQueue) enqueue(item queuedPayload) {
	if item.Kind == "" {
		return
	}
	if item.Kind == "sessions" && item.Sessions != nil && len(item.Sessions.Sessions) == 0 {
		return
	}
	if item.Kind == "sessions" && item.Sessions != nil && !containsClosed(item.Sessions.Sessions) {
		for i := len(q.Items) - 1; i >= 0; i-- {
			existing := q.Items[i]
			if existing.Kind == "sessions" && existing.Sessions != nil && !containsClosed(existing.Sessions.Sessions) {
				q.Items[i] = item
				return
			}
		}
	}
	q.Items = append(q.Items, item)
	if len(q.Items) > maxQueueItems {
		keep := q.Items[len(q.Items)-maxQueueItems:]
		// Never drop a closed-session payload when trimming.
		var closed []queuedPayload
		dropped := q.Items[:len(q.Items)-maxQueueItems]
		for _, d := range dropped {
			if d.Kind == "sessions" && d.Sessions != nil && containsClosed(d.Sessions.Sessions) {
				closed = append(closed, d)
			}
		}
		q.Items = append(closed, keep...)
		if len(q.Items) > maxQueueItems {
			q.Items = q.Items[len(q.Items)-maxQueueItems:]
		}
	}
}

func containsClosed(items []SessionItem) bool {
	for _, item := range items {
		if item.Status == StatusClosed {
			return true
		}
	}
	return false
}

func (q *uploadQueue) dropUploadedClosed(state persistedState) {
	if len(q.Items) == 0 {
		return
	}
	kept := q.Items[:0]
	for _, item := range q.Items {
		if item.Kind == "sessions" && item.Sessions != nil {
			item.Sessions.Sessions = filterUnsentSessions(state, item.Sessions.Sessions)
			if len(item.Sessions.Sessions) == 0 {
				continue
			}
		}
		kept = append(kept, item)
	}
	q.Items = kept
}

func (q *uploadQueue) containsSessionID(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return false
	}
	for _, item := range q.Items {
		if item.Kind != "sessions" || item.Sessions == nil {
			continue
		}
		for _, session := range item.Sessions.Sessions {
			if session.SessionID == id {
				return true
			}
		}
	}
	return false
}

func (m *Module) persistQueue() {
	m.mu.RLock()
	q := m.queue
	path := m.queueFile
	m.mu.RUnlock()
	if err := saveQueue(path, q); err != nil && m.logger != nil {
		m.logger.Warn("Session upload queue write failed", "error", err)
	}
}

func (m *Module) enqueueUploads(id idmodel.Identity, now time.Time, sessions []SessionItem, apps []ActivityItem, fromIndex int) {
	m.mu.Lock()
	sessions = filterUnsentSessions(m.state, sessions)
	m.mu.Unlock()
	if fromIndex <= 0 && len(sessions) > 0 {
		payload := SessionUpload{
			AgentID:     id.AgentID,
			InstallID:   id.InstallID,
			CollectedAt: now,
			Sessions:    sessions,
		}
		m.mu.Lock()
		m.queue.enqueue(queuedPayload{Kind: "sessions", Sessions: &payload, EnqueuedAt: now})
		m.mu.Unlock()
	}
	if fromIndex <= 1 && len(apps) > 0 {
		payload := ApplicationUpload{
			AgentID:     id.AgentID,
			InstallID:   id.InstallID,
			CollectedAt: now,
			Activities:  apps,
		}
		m.mu.Lock()
		m.queue.enqueue(queuedPayload{Kind: "applications", Applications: &payload, EnqueuedAt: now})
		m.mu.Unlock()
	}
	m.persistQueue()
	if m.logger != nil {
		m.logger.Info("Queue persisted", "queue_len", m.QueueLen())
	}
}

func (m *Module) drainQueue(ctx context.Context) {
	if m.client == nil {
		return
	}
	for {
		m.mu.Lock()
		if len(m.queue.Items) == 0 {
			m.mu.Unlock()
			return
		}
		item := m.queue.Items[0]
		m.mu.Unlock()

		if ctx.Err() != nil {
			return
		}
		if item.Kind == "sessions" && item.Sessions != nil {
			m.mu.Lock()
			item.Sessions.Sessions = filterUnsentSessions(m.state, item.Sessions.Sessions)
			m.mu.Unlock()
			if len(item.Sessions.Sessions) == 0 {
				m.dequeueFront()
				continue
			}
		}
		var err error
		switch item.Kind {
		case "sessions":
			if item.Sessions == nil {
				err = nil
			} else {
				_, err = m.client.UploadSessions(ctx, *item.Sessions)
			}
		case "applications":
			if item.Applications == nil {
				err = nil
			} else {
				_, err = m.client.UploadApplications(ctx, *item.Applications)
			}
		}
		if err != nil {
			if m.logger != nil {
				m.logger.Warn("Queued activity upload retry",
					"kind", item.Kind,
					"retry", true,
					"error", err,
					"queue_len", m.QueueLen(),
				)
			}
			return
		}
		m.dequeueFront()
		if item.Kind == "sessions" && item.Sessions != nil {
			m.ackClosedSessionIDs(closedSessionIDs(item.Sessions.Sessions))
		}
		if m.logger != nil {
			m.logger.Info("Queued activity upload succeeded",
				"kind", item.Kind,
				"retry", m.QueueLen() > 0,
				"queue_len", m.QueueLen(),
			)
		}
	}
}

func (m *Module) dequeueFront() {
	m.mu.Lock()
	if len(m.queue.Items) > 0 {
		m.queue.Items = m.queue.Items[1:]
	}
	m.mu.Unlock()
	m.persistQueue()
}

func (m *Module) QueueLen() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.queue.Items)
}
