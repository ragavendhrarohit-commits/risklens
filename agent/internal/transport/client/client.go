// Package client implements the transport interface over Go's standard
// library HTTP client.
//
// This is the only package in the agent permitted to import net/http. Every
// module reaches the network through transport/interfaces.Transport, which
// keeps the HTTP stack replaceable and keeps module tests free of sockets.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	transporterrors "risklens/agent/internal/transport/errors"
	"risklens/agent/internal/transport/interfaces"
	"risklens/agent/internal/transport/middleware"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

const (
	// DefaultTimeout applies when Config.Timeout is not positive.
	DefaultTimeout = 15 * time.Second

	// DefaultMaxResponseBytes bounds how much of a reply we will buffer.
	// A backend that returns more than this is treated as misbehaving
	// rather than being allowed to exhaust agent memory.
	DefaultMaxResponseBytes int64 = 1 << 20

	contentTypeJSON = "application/json"
)

// Config describes one transport instance. Only BaseURL is required.
type Config struct {
	// BaseURL is the scheme://host[:port][/prefix] every request is
	// resolved against.
	BaseURL string

	// Timeout bounds a whole request/response exchange.
	Timeout time.Duration

	// UserAgent is sent on every request when non-empty.
	UserAgent string

	// DefaultHeaders are applied when the request does not set them.
	DefaultHeaders map[string]string

	// MaxResponseBytes overrides DefaultMaxResponseBytes.
	MaxResponseBytes int64

	// Logger receives debug-level request/response lines.
	Logger *slog.Logger

	// Middleware wraps the send path, outermost first.
	Middleware []middleware.Middleware

	// HTTPClient injects a preconfigured client. When nil one is built from
	// Timeout. Supplied clients keep their own Timeout.
	HTTPClient *http.Client
}

// Client is an HTTP implementation of interfaces.Transport. It is safe for
// concurrent use and reuses connections across calls.
type Client struct {
	baseURL          string
	userAgent        string
	defaultHeaders   map[string]string
	maxResponseBytes int64
	httpClient       *http.Client
	logger           *slog.Logger
	send             middleware.RoundTripper
}

// New validates the configuration and returns a ready transport. It never
// performs I/O, so a construction failure always means misconfiguration
// rather than an unreachable backend.
func New(cfg Config) (*Client, error) {
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	maxBytes := cfg.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With("component", "transport.http")

	client := &Client{
		baseURL:          baseURL,
		userAgent:        strings.TrimSpace(cfg.UserAgent),
		defaultHeaders:   copyHeaders(cfg.DefaultHeaders),
		maxResponseBytes: maxBytes,
		httpClient:       httpClient,
		logger:           logger,
	}

	// Logging sits outermost so its duration covers the whole chain. Caller
	// middleware runs inside it, and the innermost link is the HTTP call.
	chain := append([]middleware.Middleware{middleware.Logging(client.logger)}, cfg.Middleware...)
	client.send = middleware.Chain(chain...)(client.do)
	return client, nil
}

// BaseURL returns the normalized base URL, useful for logging.
func (c *Client) BaseURL() string { return c.baseURL }

// BuildRequest assembles a request and validates it. No I/O occurs.
func (c *Client) BuildRequest(method request.Method, path string, body any) (request.Request, error) {
	req := request.Request{Method: method, Path: path, Body: body}
	if err := req.Validate(); err != nil {
		return request.Request{}, transporterrors.New(transporterrors.KindInvalidRequest, "build request", err)
	}
	return req, nil
}

// Send executes req through the middleware chain.
func (c *Client) Send(ctx context.Context, req request.Request) (*response.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := req.Validate(); err != nil {
		return nil, transporterrors.New(transporterrors.KindInvalidRequest, "send", err)
	}
	return c.send(ctx, req)
}

// DecodeResponse unmarshals a 2xx JSON body into target.
func (c *Client) DecodeResponse(resp *response.Response, target any) error {
	if resp == nil {
		return transporterrors.New(transporterrors.KindInvalidResponse, "decode response",
			stderrors.New("no response"))
	}
	if !resp.IsSuccess() {
		return transporterrors.WithStatus(transporterrors.KindUnexpectedStatus, "decode response",
			resp.StatusCode, nil)
	}
	if target == nil {
		return nil
	}
	if len(bytes.TrimSpace(resp.Body)) == 0 {
		return transporterrors.WithStatus(transporterrors.KindInvalidResponse, "decode response",
			resp.StatusCode, stderrors.New("empty body"))
	}
	if err := json.Unmarshal(resp.Body, target); err != nil {
		return transporterrors.WithStatus(transporterrors.KindDecode, "decode response",
			resp.StatusCode, err)
	}
	return nil
}

// CloseIdleConnections releases pooled sockets. Called on agent shutdown.
func (c *Client) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}

// do is the innermost RoundTripper: the only place net/http is executed.
func (c *Client) do(ctx context.Context, req request.Request) (*response.Response, error) {
	payload, err := encodeBody(req.Body)
	if err != nil {
		return nil, transporterrors.New(transporterrors.KindEncode, "encode request", err)
	}

	endpoint := c.baseURL + normalizePath(req.Path)
	if query := req.EncodedQuery(); query != "" {
		endpoint += "?" + query
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method.String(), endpoint, reader)
	if err != nil {
		return nil, transporterrors.New(transporterrors.KindInvalidRequest, "build http request", err)
	}

	httpReq.Header.Set("Accept", contentTypeJSON)
	if payload != nil {
		httpReq.Header.Set("Content-Type", contentTypeJSON)
	}
	if c.userAgent != "" {
		httpReq.Header.Set("User-Agent", c.userAgent)
	}
	for key, value := range c.defaultHeaders {
		httpReq.Header.Set(key, value)
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, classify(ctx, "send", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(httpResp.Body, c.maxResponseBytes))
		_ = httpResp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, c.maxResponseBytes+1))
	if err != nil {
		return nil, classify(ctx, "read response", err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return nil, transporterrors.WithStatus(transporterrors.KindInvalidResponse, "read response",
			httpResp.StatusCode, stderrors.New("response exceeds maximum size"))
	}

	return &response.Response{
		StatusCode: httpResp.StatusCode,
		Headers:    response.Header(httpResp.Header),
		Body:       body,
	}, nil
}

// classify maps a net/http failure onto a transport error kind. The context
// is consulted first because http.Client reports a cancelled context and an
// expired deadline through the same *url.Error shape.
func classify(ctx context.Context, op string, err error) error {
	switch {
	case stderrors.Is(err, context.Canceled) || ctx.Err() == context.Canceled:
		return transporterrors.New(transporterrors.KindCanceled, op, err)
	case stderrors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded:
		return transporterrors.New(transporterrors.KindTimeout, op, err)
	}

	var netErr net.Error
	if stderrors.As(err, &netErr) && netErr.Timeout() {
		return transporterrors.New(transporterrors.KindTimeout, op, err)
	}
	return transporterrors.New(transporterrors.KindConnection, op, err)
}

func encodeBody(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	if raw, ok := body.([]byte); ok {
		return raw, nil
	}
	return json.Marshal(body)
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", transporterrors.New(transporterrors.KindInvalidRequest, "base url",
			stderrors.New("base URL is required"))
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", transporterrors.New(transporterrors.KindInvalidRequest, "base url", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", transporterrors.New(transporterrors.KindInvalidRequest, "base url",
			stderrors.New(`scheme must be http or https, got "`+parsed.Scheme+`"`))
	}
	if parsed.Host == "" {
		return "", transporterrors.New(transporterrors.KindInvalidRequest, "base url",
			stderrors.New("host is required"))
	}
	return strings.TrimRight(trimmed, "/"), nil
}

func normalizePath(path string) string {
	if path == "" || strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func copyHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// Compile-time proof that the HTTP client satisfies the transport contract.
var _ interfaces.Transport = (*Client)(nil)
