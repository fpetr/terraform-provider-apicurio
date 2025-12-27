package client

import (
	"encoding/json"
	"fmt"
	"strings"
)

func decodeMetaJSON(statusCode int, body string) (*ArtifactMetaResponse, *ResponseError) {
	if statusCode < 200 || statusCode >= 300 {
		return nil, &ResponseError{StatusCode: statusCode, Body: body, Err: fmt.Errorf("unexpected response")}
	}

	trim := strings.TrimSpace(body)
	if trim == "" {
		return &ArtifactMetaResponse{Raw: map[string]any{}, Normalized: NormalizedArtifactMeta{}}, nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(trim), &raw); err != nil {
		return nil, &ResponseError{StatusCode: statusCode, Body: trim, Err: fmt.Errorf("failed to decode JSON: %w", err)}
	}

	n := normalizeMeta(raw)
	return &ArtifactMetaResponse{Raw: raw, Normalized: n}, nil
}
