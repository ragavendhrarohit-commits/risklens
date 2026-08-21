package browseractivity

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const nativeStateEnv = "RISKLENS_BROWSER_STATE_DIR"

func IsNativeHostInvocation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	a := strings.TrimSpace(args[0])
	switch {
	case a == "--browser-native-host":
		return true
	case strings.HasPrefix(a, "chrome-extension://"):
		return true
	case strings.HasPrefix(a, "moz-extension://"):
		return true
	case strings.Contains(a, "@risklens"):
		return true
	default:
		return false
	}
}

func nativeStateDir() string {
	if v := strings.TrimSpace(os.Getenv(nativeStateEnv)); v != "" {
		return v
	}
	base := os.Getenv("PROGRAMDATA")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "RiskLens", "state")
}

// RunNativeHost reads length-prefixed JSON from stdin (Chrome/Edge/Firefox
// Native Messaging) and writes sanitized samples to the agent inbox.
// It never contacts the Platform API.
func RunNativeHost() int {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	stateDir := nativeStateDir()
	inbox := inboxPath(stateDir)
	in := bufio.NewReader(os.Stdin)
	for {
		raw, err := ReadNativeMessage(in)
		if errorsIsEOF(err) {
			return 0
		}
		if err != nil {
			logger.Error("native messaging read failed", "error", err)
			return 1
		}
		sample, dropped, serr := ParseAndSanitize(raw)
		if serr != nil && !dropped {
			logger.Warn("native message rejected", "error", serr)
			_ = WriteNativeMessage(os.Stdout, mustJSON(map[string]any{"ok": false}))
			continue
		}
		if dropped {
			_ = WriteNativeMessage(os.Stdout, mustJSON(map[string]any{"ok": true, "dropped": true}))
			continue
		}
		if err := AppendInbox(inbox, sample); err != nil {
			logger.Error("native inbox write failed", "error", err)
			_ = WriteNativeMessage(os.Stdout, mustJSON(map[string]any{"ok": false}))
			continue
		}
		_ = WriteNativeMessage(os.Stdout, mustJSON(map[string]any{"ok": true}))
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF || (err != nil && strings.Contains(err.Error(), "EOF"))
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"ok":false}`)
	}
	return data
}
