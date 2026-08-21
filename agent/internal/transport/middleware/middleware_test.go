package middleware

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

func okBase(seen *request.Request) RoundTripper {
	return func(_ context.Context, req request.Request) (*response.Response, error) {
		if seen != nil {
			*seen = req
		}
		return &response.Response{StatusCode: 200, Body: []byte(`{}`)}, nil
	}
}

func TestChainRunsOutermostFirst(t *testing.T) {
	var order []string

	record := func(name string) Middleware {
		return func(next RoundTripper) RoundTripper {
			return func(ctx context.Context, req request.Request) (*response.Response, error) {
				order = append(order, name+":before")
				resp, err := next(ctx, req)
				order = append(order, name+":after")
				return resp, err
			}
		}
	}

	chained := Chain(record("a"), record("b"), record("c"))(okBase(nil))
	if _, err := chained(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}

	want := "a:before,b:before,c:before,c:after,b:after,a:after"
	if got := strings.Join(order, ","); got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestChainWithNoMiddlewareReturnsBase(t *testing.T) {
	var seen request.Request
	chained := Chain()(okBase(&seen))
	if _, err := chained(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if seen.Path != "/x" {
		t.Fatalf("path = %q", seen.Path)
	}
}

func TestChainSkipsNilEntries(t *testing.T) {
	chained := Chain(nil, nil)(okBase(nil))
	if _, err := chained(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
}

func TestHeadersAddsDefaults(t *testing.T) {
	var seen request.Request
	wrapped := Headers(map[string]string{"X-Channel": "stable"})(okBase(&seen))

	if _, err := wrapped(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if seen.Headers["X-Channel"] != "stable" {
		t.Fatalf("headers = %v", seen.Headers)
	}
}

func TestHeadersDoesNotOverrideExplicitValue(t *testing.T) {
	var seen request.Request
	wrapped := Headers(map[string]string{"X-Channel": "stable"})(okBase(&seen))

	req := request.GET("/x").WithHeader("X-Channel", "beta")
	if _, err := wrapped(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if seen.Headers["X-Channel"] != "beta" {
		t.Fatalf("caller value should win, got %v", seen.Headers)
	}
}

func TestHeadersWithEmptyMapIsPassThrough(t *testing.T) {
	var seen request.Request
	wrapped := Headers(nil)(okBase(&seen))
	if _, err := wrapped(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if len(seen.Headers) != 0 {
		t.Fatalf("headers = %v", seen.Headers)
	}
}

func TestRequestIDStampsGeneratedValue(t *testing.T) {
	var seen request.Request
	wrapped := RequestID("X-Request-ID", func() string { return "rid-7" })(okBase(&seen))

	if _, err := wrapped(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if seen.Headers["X-Request-ID"] != "rid-7" {
		t.Fatalf("headers = %v", seen.Headers)
	}
}

func TestRequestIDKeepsCallerSuppliedValue(t *testing.T) {
	var seen request.Request
	wrapped := RequestID("X-Request-ID", func() string { return "generated" })(okBase(&seen))

	req := request.GET("/x").WithHeader("X-Request-ID", "caller")
	if _, err := wrapped(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if seen.Headers["X-Request-ID"] != "caller" {
		t.Fatalf("headers = %v", seen.Headers)
	}
}

func TestRequestIDSkipsEmptyGeneratedValue(t *testing.T) {
	var seen request.Request
	wrapped := RequestID("X-Request-ID", func() string { return "" })(okBase(&seen))

	if _, err := wrapped(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if _, exists := seen.Headers["X-Request-ID"]; exists {
		t.Fatalf("empty id should not be stamped, got %v", seen.Headers)
	}
}

func TestRequestIDWithNilGeneratorIsPassThrough(t *testing.T) {
	var seen request.Request
	wrapped := RequestID("X-Request-ID", nil)(okBase(&seen))
	if _, err := wrapped(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if len(seen.Headers) != 0 {
		t.Fatalf("headers = %v", seen.Headers)
	}
}

func TestLoggingRecordsStatus(t *testing.T) {
	var buffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	wrapped := Logging(logger)(okBase(nil))
	if _, err := wrapped(context.Background(), request.GET("/things")); err != nil {
		t.Fatal(err)
	}

	output := buffer.String()
	for _, want := range []string{"transport request", "transport response", "/things", "status=200"} {
		if !strings.Contains(output, want) {
			t.Errorf("log output missing %q:\n%s", want, output)
		}
	}
}

func TestLoggingRecordsFailureAndPropagatesError(t *testing.T) {
	var buffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	sentinel := errors.New("boom")
	failing := func(context.Context, request.Request) (*response.Response, error) {
		return nil, sentinel
	}

	wrapped := Logging(logger)(failing)
	_, err := wrapped(context.Background(), request.GET("/things"))
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(buffer.String(), "transport failure") {
		t.Errorf("log output missing failure line:\n%s", buffer.String())
	}
}

func TestLoggingDoesNotLogBodies(t *testing.T) {
	var buffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))

	wrapped := Logging(logger)(okBase(nil))
	secret := map[string]string{"install_id": "super-secret-install-id"}
	if _, err := wrapped(context.Background(), request.POST("/things", secret)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buffer.String(), "super-secret-install-id") {
		t.Errorf("request body leaked into logs:\n%s", buffer.String())
	}
}

func TestLoggingWithNilLoggerIsPassThrough(t *testing.T) {
	var seen request.Request
	wrapped := Logging(nil)(okBase(&seen))
	if _, err := wrapped(context.Background(), request.GET("/x")); err != nil {
		t.Fatal(err)
	}
	if seen.Path != "/x" {
		t.Fatalf("path = %q", seen.Path)
	}
}
