package provider

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func isJSONArtifactType(artifactType string) bool {
	switch strings.ToUpper(strings.TrimSpace(artifactType)) {
	case "AVRO", "JSON":
		return true
	default:
		return false
	}
}

func canonicalizeJSON(b []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()

	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}

	// Ensure there is no trailing data (including a second JSON value).
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}

	return json.Marshal(v)
}

// canonicalizeJSONIfApplicable attempts to canonicalize JSON for JSON-based artifact
// types (currently AVRO and JSON). If parsing fails or the artifact type is not
// JSON-based, it returns the original bytes and false.
func canonicalizeJSONIfApplicable(artifactType string, b []byte) ([]byte, bool) {
	if !isJSONArtifactType(artifactType) {
		return b, false
	}
	canon, err := canonicalizeJSON(b)
	if err != nil {
		return b, false
	}
	return canon, true
}

func contentComparisonHash(artifactType string, b []byte) string {
	canon, ok := canonicalizeJSONIfApplicable(artifactType, b)
	if ok {
		return sha256hex(canon)
	}
	return sha256hex(b)
}
