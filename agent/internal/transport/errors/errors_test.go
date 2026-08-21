package errors

import (
	"context"
	stderrors "errors"
	"strings"
	"testing"
)

func TestErrorMatchesSentinelForItsKind(t *testing.T) {
	cases := []struct {
		kind     Kind
		sentinel error
	}{
		{KindTimeout, ErrTimeout},
		{KindCanceled, ErrCanceled},
		{KindConnection, ErrConnection},
		{KindInvalidRequest, ErrInvalidRequest},
		{KindEncode, ErrEncode},
		{KindDecode, ErrDecode},
		{KindInvalidResponse, ErrInvalidResponse},
		{KindUnexpectedStatus, ErrUnexpectedStatus},
	}
	for _, tc := range cases {
		err := New(tc.kind, "op", nil)
		if !stderrors.Is(err, tc.sentinel) {
			t.Errorf("kind %q does not match its sentinel", tc.kind)
		}
	}
}

func TestErrorDoesNotMatchOtherSentinels(t *testing.T) {
	err := New(KindTimeout, "send", nil)
	if stderrors.Is(err, ErrConnection) {
		t.Error("timeout must not match ErrConnection")
	}
	if stderrors.Is(err, ErrDecode) {
		t.Error("timeout must not match ErrDecode")
	}
}

func TestErrorUnwrapsToCause(t *testing.T) {
	err := New(KindTimeout, "send", context.DeadlineExceeded)
	if !stderrors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expected the cause to remain reachable")
	}
	if stderrors.Unwrap(err) != context.DeadlineExceeded {
		t.Fatal("Unwrap did not return the cause")
	}
}

func TestErrorMessageIncludesOpKindAndCause(t *testing.T) {
	err := New(KindConnection, "send", stderrors.New("connection refused"))
	message := err.Error()
	for _, want := range []string{"send", "connection", "connection refused"} {
		if !strings.Contains(message, want) {
			t.Errorf("message %q missing %q", message, want)
		}
	}
}

func TestErrorMessageIncludesStatusWhenPresent(t *testing.T) {
	err := WithStatus(KindUnexpectedStatus, "decode response", 503, nil)
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("message %q should mention the status", err.Error())
	}
}

func TestErrorMessageOmitsStatusWhenZero(t *testing.T) {
	err := New(KindTimeout, "send", nil)
	if strings.Contains(err.Error(), "HTTP 0") {
		t.Errorf("message %q should not mention a zero status", err.Error())
	}
}

func TestKindOfFindsWrappedTransportError(t *testing.T) {
	wrapped := stderrors.Join(stderrors.New("context"), New(KindDecode, "decode", nil))
	if KindOf(wrapped) != KindDecode {
		t.Fatalf("KindOf = %q", KindOf(wrapped))
	}
}

func TestKindOfReturnsEmptyForForeignError(t *testing.T) {
	if KindOf(stderrors.New("not ours")) != "" {
		t.Fatal("expected empty kind for a non-transport error")
	}
}

func TestStatusOfReportsAssociatedStatus(t *testing.T) {
	err := WithStatus(KindUnexpectedStatus, "decode", 409, nil)
	if StatusOf(err) != 409 {
		t.Fatalf("StatusOf = %d", StatusOf(err))
	}
	if StatusOf(stderrors.New("other")) != 0 {
		t.Fatal("expected 0 for a non-transport error")
	}
}
