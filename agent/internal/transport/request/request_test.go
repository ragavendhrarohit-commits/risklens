package request

import "testing"

func TestConstructorsSetMethodAndPath(t *testing.T) {
	cases := []struct {
		req    Request
		method Method
	}{
		{GET("/a"), MethodGET},
		{POST("/a", nil), MethodPOST},
		{PUT("/a", nil), MethodPUT},
		{DELETE("/a"), MethodDELETE},
	}
	for _, tc := range cases {
		if tc.req.Method != tc.method {
			t.Errorf("method = %q, want %q", tc.req.Method, tc.method)
		}
		if tc.req.Path != "/a" {
			t.Errorf("path = %q", tc.req.Path)
		}
	}
}

func TestMethodValid(t *testing.T) {
	for _, m := range []Method{MethodGET, MethodPOST, MethodPUT, MethodDELETE} {
		if !m.Valid() {
			t.Errorf("%q should be valid", m)
		}
	}
	for _, m := range []Method{"PATCH", "HEAD", "", "get"} {
		if m.Valid() {
			t.Errorf("%q should be invalid", m)
		}
	}
}

func TestValidateRejectsUnsupportedMethod(t *testing.T) {
	if err := New("PATCH", "/a").Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRejectsEmptyPath(t *testing.T) {
	if err := GET("").Validate(); err == nil {
		t.Fatal("expected error for empty path")
	}
	if err := GET("   ").Validate(); err == nil {
		t.Fatal("expected error for blank path")
	}
}

func TestValidateRejectsBodyOnBodylessVerbs(t *testing.T) {
	if err := (Request{Method: MethodGET, Path: "/a", Body: "x"}).Validate(); err == nil {
		t.Fatal("GET with body should be rejected")
	}
	if err := (Request{Method: MethodDELETE, Path: "/a", Body: "x"}).Validate(); err == nil {
		t.Fatal("DELETE with body should be rejected")
	}
}

func TestValidateAcceptsBodyOnPostAndPut(t *testing.T) {
	if err := POST("/a", map[string]string{"k": "v"}).Validate(); err != nil {
		t.Errorf("POST with body: %v", err)
	}
	if err := PUT("/a", map[string]string{"k": "v"}).Validate(); err != nil {
		t.Errorf("PUT with body: %v", err)
	}
}

func TestWithHeaderDoesNotMutateOriginal(t *testing.T) {
	base := GET("/a").WithHeader("X-One", "1")
	derived := base.WithHeader("X-Two", "2")

	if _, exists := base.Headers["X-Two"]; exists {
		t.Fatal("WithHeader mutated the original request")
	}
	if derived.Headers["X-One"] != "1" || derived.Headers["X-Two"] != "2" {
		t.Fatalf("derived headers = %v", derived.Headers)
	}
}

func TestWithHeadersMergesAndCopies(t *testing.T) {
	base := GET("/a").WithHeader("X-One", "1")
	derived := base.WithHeaders(map[string]string{"X-Two": "2", "X-One": "override"})

	if base.Headers["X-One"] != "1" {
		t.Fatal("original was mutated")
	}
	if derived.Headers["X-One"] != "override" || derived.Headers["X-Two"] != "2" {
		t.Fatalf("derived headers = %v", derived.Headers)
	}
}

func TestWithHeadersIgnoresEmptyMap(t *testing.T) {
	base := GET("/a")
	if got := base.WithHeaders(nil); got.Headers != nil {
		t.Fatalf("headers = %v", got.Headers)
	}
}

func TestWithQueryDoesNotMutateOriginal(t *testing.T) {
	base := GET("/a").WithQuery("k", "1")
	derived := base.WithQuery("k", "2")

	if len(base.Query["k"]) != 1 {
		t.Fatalf("original query mutated: %v", base.Query)
	}
	if len(derived.Query["k"]) != 2 {
		t.Fatalf("derived query = %v", derived.Query)
	}
}

func TestWithBodyReturnsCopy(t *testing.T) {
	base := POST("/a", "one")
	derived := base.WithBody("two")
	if base.Body != "one" || derived.Body != "two" {
		t.Fatalf("base = %v, derived = %v", base.Body, derived.Body)
	}
}

func TestEncodedQueryIsEmptyWithoutParameters(t *testing.T) {
	if got := GET("/a").EncodedQuery(); got != "" {
		t.Fatalf("query = %q", got)
	}
}

func TestEncodedQuerySortsKeysForDeterminism(t *testing.T) {
	req := GET("/a").WithQuery("zeta", "1").WithQuery("alpha", "2")
	if got := req.EncodedQuery(); got != "alpha=2&zeta=1" {
		t.Fatalf("query = %q", got)
	}
}

func TestEncodedQueryRepeatsMultiValueKeys(t *testing.T) {
	req := GET("/a").WithQuery("tag", "x").WithQuery("tag", "y")
	if got := req.EncodedQuery(); got != "tag=x&tag=y" {
		t.Fatalf("query = %q", got)
	}
}

func TestEncodedQueryEscapesReservedCharacters(t *testing.T) {
	req := GET("/a").WithQuery("q", "a b&c=d/e")
	want := "q=a+b%26c%3Dd%2Fe"
	if got := req.EncodedQuery(); got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

func TestEncodedQueryPreservesUnreservedCharacters(t *testing.T) {
	req := GET("/a").WithQuery("q", "AZaz09-_.~")
	if got := req.EncodedQuery(); got != "q=AZaz09-_.~" {
		t.Fatalf("query = %q", got)
	}
}
