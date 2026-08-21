package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	transporterrors "risklens/agent/internal/transport/errors"
	"risklens/agent/internal/transport/middleware"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

type payload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newClient(t *testing.T, cfg Config) *Client {
	t.Helper()
	if cfg.Logger == nil {
		cfg.Logger = discardLogger()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 2 * time.Second
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func jsonServer(t *testing.T, status int, body string, capture func(*http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			capture(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------- construction

func TestNewRejectsEmptyBaseURL(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error for empty base URL")
	}
}

func TestNewRejectsNonHTTPScheme(t *testing.T) {
	_, err := New(Config{BaseURL: "ftp://example.com"})
	if !errors.Is(err, transporterrors.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestNewRejectsMissingHost(t *testing.T) {
	if _, err := New(Config{BaseURL: "http://"}); err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestNewAppliesDefaultTimeout(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://127.0.0.1:1", Timeout: -1})
	if c.httpClient.Timeout != DefaultTimeout {
		t.Fatalf("timeout = %s, want %s", c.httpClient.Timeout, DefaultTimeout)
	}
}

func TestNewTrimsTrailingSlashFromBaseURL(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com/api/"})
	if c.BaseURL() != "http://example.com/api" {
		t.Fatalf("base URL = %q", c.BaseURL())
	}
}

// ---------------------------------------------------------------- BuildRequest

func TestBuildRequestPopulatesFields(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com"})
	req, err := c.BuildRequest(request.MethodPOST, "/things", payload{Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != request.MethodPOST || req.Path != "/things" {
		t.Fatalf("req = %+v", req)
	}
	if req.Body == nil {
		t.Fatal("body not carried")
	}
}

func TestBuildRequestRejectsUnsupportedMethod(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com"})
	_, err := c.BuildRequest(request.Method("PATCH"), "/things", nil)
	if !errors.Is(err, transporterrors.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestBuildRequestRejectsEmptyPath(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com"})
	if _, err := c.BuildRequest(request.MethodGET, "  ", nil); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestBuildRequestRejectsBodyOnGET(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com"})
	if _, err := c.BuildRequest(request.MethodGET, "/things", payload{}); err == nil {
		t.Fatal("expected error for GET with body")
	}
}

// ---------------------------------------------------------------------- verbs

func TestGET(t *testing.T) {
	var gotMethod, gotPath string
	srv := jsonServer(t, http.StatusOK, `{"name":"alpha","count":3}`, func(r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, err := c.Send(context.Background(), request.GET("/things"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/things" {
		t.Errorf("path = %q", gotPath)
	}
	if resp.StatusCode != response.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}

	var decoded payload
	if err := c.DecodeResponse(resp, &decoded); err != nil {
		t.Fatalf("DecodeResponse: %v", err)
	}
	if decoded.Name != "alpha" || decoded.Count != 3 {
		t.Errorf("decoded = %+v", decoded)
	}
}

func TestPOSTEncodesJSONBody(t *testing.T) {
	var gotMethod, gotContentType string
	var gotBody payload
	srv := jsonServer(t, http.StatusCreated, `{"name":"created","count":1}`, func(r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, err := c.Send(context.Background(), request.POST("/things", payload{Name: "beta", Count: 7}))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q", gotContentType)
	}
	if gotBody.Name != "beta" || gotBody.Count != 7 {
		t.Errorf("server received %+v", gotBody)
	}
	if resp.StatusCode != response.StatusCreated {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

func TestPUT(t *testing.T) {
	var gotMethod string
	var gotBody payload
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	if _, err := c.Send(context.Background(), request.PUT("/things/1", payload{Name: "gamma"})); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q", gotMethod)
	}
	if gotBody.Name != "gamma" {
		t.Errorf("body = %+v", gotBody)
	}
}

func TestDELETE(t *testing.T) {
	var gotMethod string
	var hadBody bool
	srv := jsonServer(t, http.StatusNoContent, ``, func(r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		hadBody = len(body) > 0
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, err := c.Send(context.Background(), request.DELETE("/things/1"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q", gotMethod)
	}
	if hadBody {
		t.Error("DELETE should not send a body")
	}
	if resp.StatusCode != response.StatusNoContent {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

// ------------------------------------------------------------------- encoding

func TestSendAcceptsRawByteBody(t *testing.T) {
	var got string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = string(body)
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	req := request.POST("/raw", []byte(`{"pre":"encoded"}`))
	if _, err := c.Send(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got != `{"pre":"encoded"}` {
		t.Errorf("body = %q", got)
	}
}

func TestSendReportsEncodeFailure(t *testing.T) {
	srv := jsonServer(t, http.StatusOK, `{}`, nil)
	c := newClient(t, Config{BaseURL: srv.URL})

	// Channels cannot be marshalled to JSON.
	_, err := c.Send(context.Background(), request.POST("/things", make(chan int)))
	if !errors.Is(err, transporterrors.ErrEncode) {
		t.Fatalf("expected ErrEncode, got %v", err)
	}
}

// -------------------------------------------------------------------- headers

func TestSendSetsUserAgentAndAccept(t *testing.T) {
	var ua, accept string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		ua = r.Header.Get("User-Agent")
		accept = r.Header.Get("Accept")
	})

	c := newClient(t, Config{BaseURL: srv.URL, UserAgent: "RiskLensAgent/9.9.9"})
	if _, err := c.Send(context.Background(), request.GET("/things")); err != nil {
		t.Fatal(err)
	}
	if ua != "RiskLensAgent/9.9.9" {
		t.Errorf("user-agent = %q", ua)
	}
	if accept != "application/json" {
		t.Errorf("accept = %q", accept)
	}
}

func TestDefaultHeadersAreSent(t *testing.T) {
	var got string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		got = r.Header.Get("X-Agent-Channel")
	})

	c := newClient(t, Config{
		BaseURL:        srv.URL,
		DefaultHeaders: map[string]string{"X-Agent-Channel": "stable"},
	})
	if _, err := c.Send(context.Background(), request.GET("/things")); err != nil {
		t.Fatal(err)
	}
	if got != "stable" {
		t.Errorf("header = %q", got)
	}
}

func TestPerRequestHeaderOverridesDefault(t *testing.T) {
	var got string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		got = r.Header.Get("X-Agent-Channel")
	})

	c := newClient(t, Config{
		BaseURL:        srv.URL,
		DefaultHeaders: map[string]string{"X-Agent-Channel": "stable"},
	})
	req := request.GET("/things").WithHeader("X-Agent-Channel", "beta")
	if _, err := c.Send(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got != "beta" {
		t.Errorf("header = %q, want per-request override", got)
	}
}

func TestResponseHeadersAreExposed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "rid-42")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, err := c.Send(context.Background(), request.GET("/things"))
	if err != nil {
		t.Fatal(err)
	}
	// Lookup must be case-insensitive.
	if resp.Headers.Get("x-request-id") != "rid-42" {
		t.Errorf("header = %q", resp.Headers.Get("x-request-id"))
	}
}

// ---------------------------------------------------------------- url handling

func TestQueryParametersAreEncoded(t *testing.T) {
	var rawQuery string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		rawQuery = r.URL.RawQuery
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	req := request.GET("/things").WithQuery("state", "active").WithQuery("since", "2026-08-14 00:00")
	if _, err := c.Send(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	// Keys are emitted in sorted order for determinism.
	if rawQuery != "since=2026-08-14+00%3A00&state=active" {
		t.Errorf("query = %q", rawQuery)
	}
}

func TestBaseURLPrefixIsPreserved(t *testing.T) {
	var gotPath string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		gotPath = r.URL.Path
	})

	c := newClient(t, Config{BaseURL: srv.URL + "/api/v1"})
	if _, err := c.Send(context.Background(), request.GET("/agents")); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/agents" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestPathWithoutLeadingSlashIsNormalized(t *testing.T) {
	var gotPath string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		gotPath = r.URL.Path
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	if _, err := c.Send(context.Background(), request.GET("things")); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/things" {
		t.Errorf("path = %q", gotPath)
	}
}

// --------------------------------------------------------------------- status

func TestSendDoesNotErrorOnNon2xx(t *testing.T) {
	srv := jsonServer(t, http.StatusConflict, `{"error":{"code":"duplicate"}}`, nil)

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, err := c.Send(context.Background(), request.POST("/things", payload{}))
	if err != nil {
		t.Fatalf("Send should surface the response, got error %v", err)
	}
	if resp.StatusCode != response.StatusConflict {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !resp.IsClientError() {
		t.Error("expected IsClientError")
	}
}

func TestDecodeResponseRejectsNon2xx(t *testing.T) {
	srv := jsonServer(t, http.StatusInternalServerError, `{"error":{}}`, nil)

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, _ := c.Send(context.Background(), request.GET("/things"))

	var decoded payload
	err := c.DecodeResponse(resp, &decoded)
	if !errors.Is(err, transporterrors.ErrUnexpectedStatus) {
		t.Fatalf("expected ErrUnexpectedStatus, got %v", err)
	}
	if transporterrors.StatusOf(err) != 500 {
		t.Errorf("status on error = %d", transporterrors.StatusOf(err))
	}
}

func TestDecodeResponseRejectsMalformedJSON(t *testing.T) {
	srv := jsonServer(t, http.StatusOK, `this is not json`, nil)

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, _ := c.Send(context.Background(), request.GET("/things"))

	var decoded payload
	if err := c.DecodeResponse(resp, &decoded); !errors.Is(err, transporterrors.ErrDecode) {
		t.Fatalf("expected ErrDecode, got %v", err)
	}
}

func TestDecodeResponseRejectsEmptyBody(t *testing.T) {
	srv := jsonServer(t, http.StatusOK, ``, nil)

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, _ := c.Send(context.Background(), request.GET("/things"))

	var decoded payload
	if err := c.DecodeResponse(resp, &decoded); !errors.Is(err, transporterrors.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestDecodeResponseRejectsNilResponse(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com"})
	var decoded payload
	if err := c.DecodeResponse(nil, &decoded); !errors.Is(err, transporterrors.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestDecodeResponseAllowsNilTargetOnSuccess(t *testing.T) {
	srv := jsonServer(t, http.StatusNoContent, ``, nil)

	c := newClient(t, Config{BaseURL: srv.URL})
	resp, _ := c.Send(context.Background(), request.DELETE("/things/1"))
	if err := c.DecodeResponse(resp, nil); err != nil {
		t.Fatalf("expected nil error for discarded body, got %v", err)
	}
}

// --------------------------------------------------------------------- errors

func TestTimeoutProducesErrTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := newClient(t, Config{BaseURL: srv.URL, Timeout: 40 * time.Millisecond})
	_, err := c.Send(context.Background(), request.GET("/slow"))
	if !errors.Is(err, transporterrors.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if transporterrors.KindOf(err) != transporterrors.KindTimeout {
		t.Errorf("kind = %q", transporterrors.KindOf(err))
	}
}

func TestContextDeadlineProducesErrTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := newClient(t, Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	_, err := c.Send(ctx, request.GET("/slow"))
	if !errors.Is(err, transporterrors.ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}

func TestCanceledContextProducesErrCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c := newClient(t, Config{BaseURL: srv.URL, Timeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := c.Send(ctx, request.GET("/slow"))
	if !errors.Is(err, transporterrors.ErrCanceled) {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}
}

func TestAlreadyCanceledContextFailsBeforeIO(t *testing.T) {
	srv := jsonServer(t, http.StatusOK, `{}`, func(*http.Request) {
		t.Error("server should not be reached with a cancelled context")
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Send(ctx, request.GET("/things")); !errors.Is(err, transporterrors.ErrCanceled) {
		t.Fatalf("expected ErrCanceled, got %v", err)
	}
}

func TestConnectionFailureProducesErrConnection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	c := newClient(t, Config{BaseURL: closedURL, Timeout: 500 * time.Millisecond})
	_, err := c.Send(context.Background(), request.GET("/things"))
	if !errors.Is(err, transporterrors.ErrConnection) {
		t.Fatalf("expected ErrConnection, got %v", err)
	}
}

func TestOversizedResponseIsRejected(t *testing.T) {
	srv := jsonServer(t, http.StatusOK, strings.Repeat("x", 500), nil)

	c := newClient(t, Config{BaseURL: srv.URL, MaxResponseBytes: 64})
	_, err := c.Send(context.Background(), request.GET("/big"))
	if !errors.Is(err, transporterrors.ErrInvalidResponse) {
		t.Fatalf("expected ErrInvalidResponse, got %v", err)
	}
}

func TestResponseExactlyAtLimitIsAccepted(t *testing.T) {
	body := strings.Repeat("x", 64)
	srv := jsonServer(t, http.StatusOK, body, nil)

	c := newClient(t, Config{BaseURL: srv.URL, MaxResponseBytes: 64})
	resp, err := c.Send(context.Background(), request.GET("/edge"))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(resp.Body) != 64 {
		t.Errorf("body length = %d", len(resp.Body))
	}
}

func TestSendRejectsInvalidRequestBeforeIO(t *testing.T) {
	srv := jsonServer(t, http.StatusOK, `{}`, func(*http.Request) {
		t.Error("server should not be reached for an invalid request")
	})

	c := newClient(t, Config{BaseURL: srv.URL})
	_, err := c.Send(context.Background(), request.Request{Method: request.MethodGET, Path: ""})
	if !errors.Is(err, transporterrors.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}

// ----------------------------------------------------------------- middleware

func TestMiddlewareRunsAroundSend(t *testing.T) {
	var seenHeader string
	srv := jsonServer(t, http.StatusOK, `{}`, func(r *http.Request) {
		seenHeader = r.Header.Get("X-Correlation-Id")
	})

	c := newClient(t, Config{
		BaseURL: srv.URL,
		Middleware: []middleware.Middleware{
			middleware.RequestID("X-Correlation-Id", func() string { return "fixed-id" }),
		},
	})
	if _, err := c.Send(context.Background(), request.GET("/things")); err != nil {
		t.Fatal(err)
	}
	if seenHeader != "fixed-id" {
		t.Errorf("correlation header = %q", seenHeader)
	}
}

func TestClientSatisfiesTransportInterface(t *testing.T) {
	c := newClient(t, Config{BaseURL: "http://example.com"})
	// Compile-time assertion lives in client.go; this guards the concrete
	// methods staying reachable through the interface value.
	req, err := c.BuildRequest(request.MethodGET, "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != request.MethodGET {
		t.Fatalf("method = %q", req.Method)
	}
}
