package provider

import "testing"

func TestCanonicalizeJSON_FormattingOnlyDiffsMatch(t *testing.T) {
	left := []byte("{\n  \"b\": 1,\n  \"a\": 2\n}\n")
	right := []byte("{\"a\":2,\"b\":1}")

	cl, err := canonicalizeJSON(left)
	if err != nil {
		t.Fatalf("expected left to parse as JSON, got error: %v", err)
	}
	cr, err := canonicalizeJSON(right)
	if err != nil {
		t.Fatalf("expected right to parse as JSON, got error: %v", err)
	}

	if string(cl) != string(cr) {
		t.Fatalf("expected canonical JSON to match, left=%s right=%s", string(cl), string(cr))
	}
}

func TestContentComparisonHash_ArrayOrderMatters(t *testing.T) {
	left := []byte("{\"fields\":[{\"name\":\"a\"},{\"name\":\"b\"}]}")
	right := []byte("{\"fields\":[{\"name\":\"b\"},{\"name\":\"a\"}]}")

	hl := contentComparisonHash("AVRO", left)
	hr := contentComparisonHash("AVRO", right)
	if hl == hr {
		t.Fatalf("expected different hashes when array element order changes")
	}
}

func TestContentComparisonHash_InvalidJSONFallsBackToRaw(t *testing.T) {
	b := []byte("not-json")
	got := contentComparisonHash("AVRO", b)
	want := sha256hex(b)
	if got != want {
		t.Fatalf("expected raw hash fallback, got %q want %q", got, want)
	}
}
