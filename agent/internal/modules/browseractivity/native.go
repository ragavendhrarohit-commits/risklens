package browseractivity

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxNativeMessage = 1 << 20
	inboxFileName    = "browser_native_inbox.jsonl"
	stateFileName    = "browser_activity.json"
)

var errNativeTooLarge = errors.New("native message too large")

func inboxPath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "RiskLens", inboxFileName)
	}
	return filepath.Join(stateDir, inboxFileName)
}

func browserStatePath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "RiskLens", stateFileName)
	}
	return filepath.Join(stateDir, stateFileName)
}

func ReadNativeMessage(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	if length == 0 {
		return []byte{}, nil
	}
	if length > maxNativeMessage {
		return nil, errNativeTooLarge
	}
	buf := make([]byte, length)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func WriteNativeMessage(w io.Writer, payload []byte) error {
	if len(payload) > maxNativeMessage {
		return errNativeTooLarge
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func AppendInbox(path string, sample Sample) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(sample)
}

func ReadInbox(path string) ([]Sample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var samples []Sample
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var sample Sample
		if err := json.Unmarshal([]byte(line), &sample); err != nil {
			continue
		}
		if sample.At.IsZero() {
			sample.At = time.Now().UTC()
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

func ClearInbox(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadBrowserState(path string) (persistedState, error) {
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

func saveBrowserState(path string, state persistedState) error {
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

type activityFile struct {
	SessionID      string `json:"session_id"`
	ExecutableName string `json:"executable_name"`
}

func ReadForeground(activityJSONPath string) (Foreground, bool) {
	data, err := os.ReadFile(activityJSONPath)
	if err != nil {
		return Foreground{}, false
	}
	var snap activityFile
	if err := json.Unmarshal(data, &snap); err != nil {
		return Foreground{}, false
	}
	if strings.TrimSpace(snap.SessionID) == "" {
		return Foreground{}, false
	}
	return Foreground{
		SessionID:      strings.TrimSpace(snap.SessionID),
		ExecutableName: snap.ExecutableName,
	}, true
}

func activityJSONPath(stateDir string) string {
	if strings.TrimSpace(stateDir) == "" {
		return filepath.Join(os.TempDir(), "RiskLens", "activity.json")
	}
	return filepath.Join(stateDir, "activity.json")
}
