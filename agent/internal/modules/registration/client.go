package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	txclient "risklens/agent/internal/transport/client"
	txerrors "risklens/agent/internal/transport/errors"
	txinterfaces "risklens/agent/internal/transport/interfaces"
	"risklens/agent/internal/transport/request"
	"risklens/agent/internal/transport/response"
)

const (
	// EndpointPath is fixed by the backend Sprint 2 API.
	EndpointPath = "/api/v1/agents/register"

	// userAgent identifies the agent to the backend.
	userAgent = "RiskLensAgent"
)

// Well-known error sentinels. Callers use errors.Is to branch. These describe
// registration outcomes, not transport mechanics: the transport's own error
// kinds are translated into these before leaving the package.
var (
	ErrDuplicate         = errors.New("agent already registered")
	ErrValidation        = errors.New("request validation error")
	ErrServer            = errors.New("server error")
	ErrUnexpectedStatus  = errors.New("unexpected status")
	ErrMalformedResponse = errors.New("malformed response")
	ErrTransport         = errors.New("transport error")
)

// Result is the parsed outcome of a single registration attempt. It is
// always populated (even on error) so the module can record diagnostics.
type Result struct {
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	RequestID      string
	ErrorCode      string
	ErrorMessage   string
}

// Client turns the registration API contract into transport calls. It owns
// no sockets, no timeouts, and no HTTP types: everything below the API
// contract belongs to the injected transport.
type Client struct {
	transport txinterfaces.Transport
	logger    *slog.Logger
}

// NewClient builds a registration client backed by the shared HTTP transport.
// It validates the backend URL and applies the configured timeout by handing
// both to the transport, and never dials the network.
func NewClient(baseURL string, timeout time.Duration, agentVersion string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}

	transport, err := txclient.New(txclient.Config{
		BaseURL:   baseURL,
		Timeout:   timeout,
		UserAgent: buildUserAgent(agentVersion),
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	return NewClientWithTransport(transport, logger), nil
}

// NewClientWithTransport wraps an existing transport. Used when the agent
// shares one transport across modules, and by tests that substitute a fake.
func NewClientWithTransport(transport txinterfaces.Transport, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		transport: transport,
		logger:    logger.With("component", "registration.client"),
	}
}

func buildUserAgent(agentVersion string) string {
	if agentVersion == "" {
		return userAgent
	}
	return userAgent + "/" + agentVersion
}

// Register performs one POST to /api/v1/agents/register. It returns a
// non-nil Result plus one of the sentinel errors above (or nil on 201).
// The caller is expected to translate the returned error into module state.
func (c *Client) Register(ctx context.Context, req Request) (*Result, error) {
	result := &Result{}

	txReq, err := c.transport.BuildRequest(request.MethodPOST, EndpointPath, req)
	if err != nil {
		return result, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.transport.Send(ctx, txReq)
	if err != nil {
		return result, translateTransportError(err)
	}

	result.HTTPStatusCode = resp.StatusCode

	switch resp.StatusCode {
	case response.StatusCreated:
		var success SuccessResponse
		if err := c.transport.DecodeResponse(resp, &success); err != nil {
			return result, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
		}
		if !success.Accepted || success.RegistrationStatus != "registered" {
			return result, fmt.Errorf("%w: server did not accept registration", ErrMalformedResponse)
		}
		result.ServerVersion = success.ServerVersion
		result.ServerTime = success.ServerTime
		return result, nil

	case response.StatusConflict:
		parseErrorEnvelope(resp.Body, result)
		return result, ErrDuplicate

	case response.StatusBadRequest, response.StatusUnprocessableEntity:
		parseErrorEnvelope(resp.Body, result)
		return result, fmt.Errorf("%w: %s", ErrValidation, safeMessage(result))

	default:
		parseErrorEnvelope(resp.Body, result)
		if resp.IsServerError() {
			return result, fmt.Errorf("%w: HTTP %d %s", ErrServer, resp.StatusCode, safeMessage(result))
		}
		return result, fmt.Errorf("%w: HTTP %d %s", ErrUnexpectedStatus, resp.StatusCode, safeMessage(result))
	}
}

// translateTransportError maps transport failure kinds onto this package's
// sentinels, so module code and its tests never reference transport errors.
// A reply that arrived but could not be used is "malformed"; anything that
// prevented a reply is "transport".
func translateTransportError(err error) error {
	switch {
	case errors.Is(err, txerrors.ErrInvalidResponse), errors.Is(err, txerrors.ErrDecode):
		return fmt.Errorf("%w: %v", ErrMalformedResponse, err)
	default:
		return fmt.Errorf("%w: %v", ErrTransport, err)
	}
}

func parseErrorEnvelope(body []byte, result *Result) {
	var envelope ErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return
	}
	result.ErrorCode = envelope.Error.Code
	result.ErrorMessage = envelope.Error.Message
	result.RequestID = envelope.Error.RequestID
}

func safeMessage(result *Result) string {
	if result.ErrorMessage != "" {
		return result.ErrorMessage
	}
	if result.ErrorCode != "" {
		return result.ErrorCode
	}
	return ""
}
