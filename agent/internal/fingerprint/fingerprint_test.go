package fingerprint

import (
	"testing"
	"time"
)

func TestIdenticalInputsProduceIdenticalFingerprints(t *testing.T) {
	payload := map[string]any{"hostname": "pc", "cores": 8}
	a := Of("device", 1, false, payload)
	b := Of("device", 1, false, payload)
	if a.Empty() || !a.Equal(b) {
		t.Fatalf("a=%s b=%s", a, b)
	}
}

func TestOfIsDeterministicAcrossManyCalls(t *testing.T) {
	payload := map[string]any{"hostname": "pc", "disks": []any{"C:", "D:"}}
	first := Of("device", 1, false, payload)
	for i := 0; i < 64; i++ {
		got := Of("device", 1, false, payload)
		if !got.Equal(first) {
			t.Fatalf("iteration %d: %s != %s", i, got, first)
		}
	}
}

func TestMapKeyOrderDoesNotAffectFingerprint(t *testing.T) {
	left := Of("device", 1, false, map[string]any{"z": 1, "a": 2, "m": map[string]any{"b": true, "a": false}})
	right := Of("device", 1, false, map[string]any{"a": 2, "m": map[string]any{"a": false, "b": true}, "z": 1})
	if !left.Equal(right) {
		t.Fatalf("map order changed hash: %s vs %s", left, right)
	}
}

func TestPayloadChangeProducesDifferentFingerprint(t *testing.T) {
	a := Of("device", 1, false, map[string]any{"hostname": "pc"})
	b := Of("device", 1, false, map[string]any{"hostname": "other"})
	if a.Equal(b) {
		t.Fatal("payload change must change fingerprint")
	}
}

func TestPartialAndSchemaAffectFingerprint(t *testing.T) {
	payload := map[string]any{"hostname": "pc"}
	base := Of("device", 1, false, payload)
	if base.Equal(Of("device", 1, true, payload)) {
		t.Fatal("partial must change fingerprint")
	}
	if base.Equal(Of("device", 2, false, payload)) {
		t.Fatal("schema version must change fingerprint")
	}
	if base.Equal(Of("users", 1, false, payload)) {
		t.Fatal("kind must change fingerprint")
	}
}

func TestTimestampsAreNotFingerprintInputs(t *testing.T) {
	payload := map[string]any{"hostname": "pc"}
	a := Of("device", 1, false, payload)
	time.Sleep(2 * time.Millisecond)
	b := Of("device", 1, false, payload)
	if !a.Equal(b) {
		t.Fatal("Of must not read the clock")
	}
}

func TestNilPayloadIsEmpty(t *testing.T) {
	if !Of("device", 1, false, nil).Empty() {
		t.Fatal("nil payload must be empty")
	}
}

func TestFingerprintJSONRoundTripAsString(t *testing.T) {
	fp := Of("device", 1, false, "payload")
	raw, err := fp.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var out Fingerprint
	if err := out.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if !out.Equal(fp) {
		t.Fatalf("json round trip = %s", out)
	}
}
