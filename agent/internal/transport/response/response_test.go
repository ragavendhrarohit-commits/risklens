package response

import "testing"

func sample(status int, body string) *Response {
	return &Response{
		StatusCode: status,
		Headers:    Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body:       []byte(body),
	}
}

func TestHeaderGetIsCaseInsensitive(t *testing.T) {
	h := Header{"X-Request-Id": {"rid-1"}}
	for _, key := range []string{"X-Request-Id", "x-request-id", "X-REQUEST-ID"} {
		if got := h.Get(key); got != "rid-1" {
			t.Errorf("Get(%q) = %q", key, got)
		}
	}
}

func TestHeaderGetReturnsEmptyForMissingKey(t *testing.T) {
	if got := (Header{}).Get("Absent"); got != "" {
		t.Fatalf("Get = %q", got)
	}
	if got := Header(nil).Get("Absent"); got != "" {
		t.Fatalf("Get on nil header = %q", got)
	}
}

func TestHeaderValuesReturnsAll(t *testing.T) {
	h := Header{"Set-Cookie": {"a=1", "b=2"}}
	if got := h.Values("set-cookie"); len(got) != 2 {
		t.Fatalf("Values = %v", got)
	}
	if got := h.Values("absent"); got != nil {
		t.Fatalf("Values for missing key = %v", got)
	}
}

func TestStatusClassHelpers(t *testing.T) {
	cases := []struct {
		status                        int
		success, clientErr, serverErr bool
	}{
		{200, true, false, false},
		{201, true, false, false},
		{204, true, false, false},
		{299, true, false, false},
		{400, false, true, false},
		{409, false, true, false},
		{422, false, true, false},
		{499, false, true, false},
		{500, false, false, true},
		{503, false, false, true},
		{100, false, false, false},
	}
	for _, tc := range cases {
		r := sample(tc.status, "{}")
		if r.IsSuccess() != tc.success {
			t.Errorf("status %d IsSuccess = %v", tc.status, r.IsSuccess())
		}
		if r.IsClientError() != tc.clientErr {
			t.Errorf("status %d IsClientError = %v", tc.status, r.IsClientError())
		}
		if r.IsServerError() != tc.serverErr {
			t.Errorf("status %d IsServerError = %v", tc.status, r.IsServerError())
		}
	}
}

func TestStatusHelpersAreNilSafe(t *testing.T) {
	var r *Response
	if r.IsSuccess() || r.IsClientError() || r.IsServerError() {
		t.Fatal("nil response must not report any status class")
	}
	if r.ContentType() != "" {
		t.Fatal("nil response must have no content type")
	}
}

func TestContentTypeStripsParameters(t *testing.T) {
	if got := sample(200, "{}").ContentType(); got != "application/json" {
		t.Fatalf("ContentType = %q", got)
	}
}

func TestContentTypeWithoutParameters(t *testing.T) {
	r := &Response{Headers: Header{"Content-Type": {"text/plain"}}}
	if got := r.ContentType(); got != "text/plain" {
		t.Fatalf("ContentType = %q", got)
	}
}

func TestDecodeJSONReadsBody(t *testing.T) {
	var target struct {
		Name string `json:"name"`
	}
	if err := sample(200, `{"name":"alpha"}`).DecodeJSON(&target); err != nil {
		t.Fatal(err)
	}
	if target.Name != "alpha" {
		t.Fatalf("name = %q", target.Name)
	}
}

func TestDecodeJSONWorksOnErrorStatuses(t *testing.T) {
	// Error envelopes must be readable even though the status is not 2xx.
	var target struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := sample(409, `{"error":{"code":"duplicate_agent"}}`).DecodeJSON(&target); err != nil {
		t.Fatal(err)
	}
	if target.Error.Code != "duplicate_agent" {
		t.Fatalf("code = %q", target.Error.Code)
	}
}

func TestDecodeJSONReportsMalformedBody(t *testing.T) {
	var target map[string]any
	if err := sample(200, "not json").DecodeJSON(&target); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestDecodeJSONOnNilResponse(t *testing.T) {
	var r *Response
	var target map[string]any
	if err := r.DecodeJSON(&target); err == nil {
		t.Fatal("expected an error for a nil response")
	}
}
