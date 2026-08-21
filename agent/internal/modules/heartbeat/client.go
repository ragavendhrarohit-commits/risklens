package heartbeat

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
	endpointPrefix = "/api/v1/agents/"
	endpointSuffix = "/heartbeat"
	userAgent      = "RiskLensAgent"
)

var (
	ErrUnknownAgent      = errors.New("unknown agent")
	ErrValidation        = errors.New("request validation error")
	ErrServer            = errors.New("server error")
	ErrUnexpectedStatus  = errors.New("unexpected status")
	ErrMalformedResponse = errors.New("malformed response")
	ErrTransport         = errors.New("transport error")
)

// Result captures diagnostics from one heartbeat attempt.
type Result struct {
	HTTPStatusCode int
	ServerVersion  string
	ServerTime     time.Time
	ReceivedAt     time.Time
	RequestID      string
	ErrorCode      string
	ErrorMessage   string
}

// Client posts heartbeats through the shared Transport interface only.
type Client struct {
	transport txinterfaces.Transport
	logger    *slog.Logger
}

func NewClient(baseURL string, timeout time.Duration, agentVersion string, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	ua := userAgent
	if agentVersion != "" {
		ua = userAgent + "/" + agentVersion
	}
	transport, err := txclient.New(txclient.Config{
		BaseURL:   baseURL,
		Timeout:   timeout,
		UserAgent: ua,
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	return NewClientWithTransport(transport, logger), nil
}

func NewClientWithTransport(transport txinterfaces.Transport, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		transport: transport,
		logger:    logger.With("component", "heartbeat.client"),
	}
}

func endpointPath(agentID string) string {
	return endpointPrefix + agentID + endpointSuffix
}

// Send posts one heartbeat for the given agent.
func (c *Client) Send(ctx context.Context, req Request) (*Result, error) {
	result := &Result{}

	txReq, err := c.transport.BuildRequest(request.MethodPOST, endpointPath(req.AgentID), req)
	if err != nil {
		return result, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.transport.Send(ctx, txReq)
	if err != nil {
		return result, translateTransportError(err)
	}
	result.HTTPStatusCode = resp.StatusCode

	switch resp.StatusCode {
	case response.StatusOK, response.StatusAccepted, response.StatusCreated:
		var success SuccessResponse
		if err := c.transport.DecodeResponse(resp, &success); err != nil {
			return result, fmt.Errorf("%w: %v", ErrMalformedResponse, err)
		}
		if !success.Accepted {
			return result, fmt.Errorf("%w: server did not accept heartbeat", ErrMalformedResponse)
		}
		result.ServerVersion = success.ServerVersion
		result.ServerTime = success.ServerTime
		result.ReceivedAt = success.ReceivedAt
		return result, nil

	case response.StatusNotFound:
		parseErrorEnvelope(resp.Body, result)
		return result, ErrUnknownAgent

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

func translateTransportError(err error) error {
	switch {
	case errors.Is(err, txerrors.ErrTimeout):
		return fmt.Errorf("%w: timeout: %v", ErrTransport, err)
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
