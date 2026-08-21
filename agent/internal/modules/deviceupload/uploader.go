package deviceupload

import (
	"context"
	"errors"

	"risklens/agent/internal/modules/device"
	"risklens/agent/internal/pipeline"
)

var (
	ErrInvalidPayload      = errors.New("deviceupload: snapshot payload is not device.Info")
	ErrIdentityUnavailable = errors.New("deviceupload: identity not available")
	ErrEmptyInventory      = errors.New("deviceupload: device inventory empty")
)

// Uploader adapts pipeline.Snapshot to the existing Device Upload client.
// It does not HTTP, retry, schedule, or fingerprint.
type Uploader struct {
	identity IdentityProvider
	client   *Client
}

// NewUploader builds a pipeline.Uploader that delegates to Client.Upload.
func NewUploader(identity IdentityProvider, client *Client) (*Uploader, error) {
	if identity == nil {
		return nil, errors.New("identity provider is required")
	}
	if client == nil {
		return nil, errors.New("client is required")
	}
	return &Uploader{identity: identity, client: client}, nil
}

// Upload maps Snapshot.Payload to device.Info and sends one request through
// RequestFromInventory and Client.Upload.
func (u *Uploader) Upload(ctx context.Context, snap pipeline.Snapshot) error {
	info, err := payloadInfo(snap.Payload)
	if err != nil {
		return err
	}
	id, ok := u.identity.Current()
	if !ok {
		return ErrIdentityUnavailable
	}
	if info.IsEmpty() || firstNonEmpty(info.MachineName, info.Hostname) == "" {
		return ErrEmptyInventory
	}
	info = info.ForTransport(snap.CollectedAt)
	req := RequestFromInventory(id.AgentID, id.InstallID, info)
	_, err = u.client.Upload(ctx, req)
	return err
}

func payloadInfo(payload any) (device.Info, error) {
	switch v := payload.(type) {
	case device.Info:
		return v, nil
	case *device.Info:
		if v == nil {
			return device.Info{}, ErrInvalidPayload
		}
		return *v, nil
	default:
		return device.Info{}, ErrInvalidPayload
	}
}

var _ pipeline.Uploader = (*Uploader)(nil)
