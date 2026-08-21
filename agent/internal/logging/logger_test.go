package logging

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreatesJSONLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	logger, err := New(Options{
		Level:       ParseLevel("info"),
		FilePath:    path,
		ServiceName: "RiskLensAgent",
		Version:     "test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	logger.Info("Service starting")
	logger.Info("Configuration loaded", "path", "test.yaml")
	logger.Info("Version")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var foundStarting bool
	for scanner.Scan() {
		line := scanner.Bytes()
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if rec["msg"] == "Service starting" {
			foundStarting = true
		}
		if rec["service"] != "RiskLensAgent" {
			t.Fatalf("service = %v", rec["service"])
		}
		if rec["version"] != "test" {
			t.Fatalf("version = %v", rec["version"])
		}
		if rec["hostname"] == nil || rec["hostname"] == "" {
			t.Fatal("hostname missing")
		}
		if rec["instance_id"] == nil || rec["instance_id"] == "" {
			t.Fatal("instance_id missing")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !foundStarting {
		t.Fatal("expected Service starting record")
	}
}

func TestLevelFilter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	logger, err := New(Options{
		Level:       ParseLevel("warn"),
		FilePath:    path,
		ServiceName: "RiskLensAgent",
		Version:     "test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	logger.Info("Service started")
	logger.Warn("disk low")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "Service started") {
		t.Fatal("info record should be filtered")
	}
	if !strings.Contains(text, "disk low") {
		t.Fatal("warn record missing")
	}
}

func TestCloseFlushes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	logger, err := New(Options{
		Level:       ParseLevel("info"),
		FilePath:    path,
		ServiceName: "RiskLensAgent",
		Version:     "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("Service stopped")
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("expected flushed log bytes")
	}
}

func TestWriteFailureDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := &rotatingWriter{
		path:     filepath.Join(blocker, "agent.log"),
		maxBytes: 1024,
		keep:     1,
	}
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("panic: %v", rec)
		}
	}()
	if _, err := w.Write([]byte(`{"msg":"x"}` + "\n")); err != nil {
		t.Fatalf("Write should swallow errors, got %v", err)
	}
}

func TestCloseDoesNotReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.log")
	w, err := newRotatingWriter(path, 1024, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("one\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("two\n")); err != os.ErrClosed {
		t.Fatalf("Write after Close = %v, want os.ErrClosed", err)
	}
}

func TestParseLevel(t *testing.T) {
	if ParseLevel("debug") != slog.LevelDebug {
		t.Fatalf("debug = %v", ParseLevel("debug"))
	}
	if ParseLevel("WARN") != slog.LevelWarn {
		t.Fatalf("warn = %v", ParseLevel("WARN"))
	}
	if ParseLevel("nope") != slog.LevelInfo {
		t.Fatalf("default = %v", ParseLevel("nope"))
	}
}

func TestNewRequiresLogFile(t *testing.T) {
	if _, err := New(Options{ServiceName: "RiskLensAgent"}); err == nil {
		t.Fatal("expected error")
	}
}
