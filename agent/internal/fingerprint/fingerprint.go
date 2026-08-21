// Package fingerprint computes opaque identities of comparable snapshot content.
//
// It has no knowledge of collectors, transport, scheduling, or HTTP.
// Pipeline compares fingerprints only with Equal; it never hashes payloads.
package fingerprint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Fingerprint is an opaque value object. Callers compare with Equal and Empty.
// The hash algorithm is an implementation detail of Of and may change without
// changing this type or Pipeline’s public API.
type Fingerprint struct {
	Value string
}

// Empty reports whether no fingerprint was produced.
func (f Fingerprint) Empty() bool {
	return f.Value == ""
}

// Equal reports whether two fingerprints are identical.
func (f Fingerprint) Equal(other Fingerprint) bool {
	return f.Value == other.Value
}

func (f Fingerprint) String() string {
	return f.Value
}

// MarshalJSON stores the fingerprint as a JSON string so envelope format
// stays stable if hashing internals change.
func (f Fingerprint) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.Value)
}

// UnmarshalJSON reads a JSON string into Value.
func (f *Fingerprint) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		f.Value = ""
		return nil
	}
	return json.Unmarshal(data, &f.Value)
}

// Of returns a deterministic fingerprint of comparable envelope fields.
// CollectedAt, CollectError, and any pre-set fingerprint are not inputs.
// A nil payload yields an empty Fingerprint.
func Of(kind string, schemaVersion int, partial bool, payload any) Fingerprint {
	if payload == nil {
		return Fingerprint{}
	}
	normalized, err := normalize(payload)
	if err != nil {
		return Fingerprint{}
	}
	envelope := map[string]any{
		"kind":           kind,
		"partial":        partial,
		"payload":        normalized,
		"schema_version": schemaVersion,
	}
	var buf bytes.Buffer
	if err := writeCanonical(&buf, envelope); err != nil {
		return Fingerprint{}
	}
	sum := sha256.Sum256(buf.Bytes())
	return Fingerprint{Value: hex.EncodeToString(sum[:])}
}
