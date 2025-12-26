// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// v3Client supports Apicurio Registry v3 endpoint shapes where they differ from v2.
// In particular, newer Apicurio v3 APIs manage artifact metadata at:
//   /apis/registry/v3/groups/{groupId}/artifacts/{artifactId}
// and version metadata at:
//   /apis/registry/v3/groups/{groupId}/artifacts/{artifactId}/versions/{versionExpression}
//
// Many deployments still expose v2 endpoints as well; to stay compatible (and keep
// acceptance tests working against v2-only servers), we fall back to the v2 delegate
// when v3 endpoints return 404.

type v3Client struct {
	endpoint   string
	httpClient *http.Client
	cfg        ClientConfig
	delegate   RegistryClient
}

func NewV3(endpoint string, httpClient *http.Client, cfg ClientConfig) RegistryClient {
	return &v3Client{
		endpoint:   endpoint,
		httpClient: httpClient,
		cfg:        cfg,
		delegate:   NewV2(endpoint, httpClient, cfg),
	}
}

func (c *v3Client) Flavor() ServerFlavor { return ServerFlavorV3 }

func (c *v3Client) base() string { return c.endpoint + "/apis/registry/v3" }

func (c *v3Client) GetArtifactMeta(ctx context.Context, groupID, artifactID string) (*ArtifactMetaResponse, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID)
	resp, err := c.doJSON(ctx, http.MethodGet, u, map[string]string{"Accept": "application/json"}, nil)
	if err != nil {
		if err.IsNotFound() {
			return c.delegate.GetArtifactMeta(ctx, groupID, artifactID)
		}
		return nil, err
	}
	return resp, nil
}

func (c *v3Client) GetLatestArtifactContent(ctx context.Context, groupID, artifactID string) ([]byte, *ResponseError) {
	return c.delegate.GetLatestArtifactContent(ctx, groupID, artifactID)
}

func (c *v3Client) CreateArtifact(ctx context.Context, groupID, artifactID, artifactType string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError) {
	return c.delegate.CreateArtifact(ctx, groupID, artifactID, artifactType, version, content)
}

func (c *v3Client) UpdateArtifactMeta(ctx context.Context, groupID, artifactID string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID)
	resp, err := c.updateArtifactMetaAt(ctx, u, meta)
	if err != nil {
		if err.IsNotFound() {
			return c.delegate.UpdateArtifactMeta(ctx, groupID, artifactID, meta)
		}
		return nil, err
	}
	return resp, nil
}

func (c *v3Client) UpdateArtifactVersionMeta(ctx context.Context, groupID, artifactID, version string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError) {
	// v3 manages version metadata at the version endpoint (no trailing /meta).
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/versions/" + url.PathEscape(version)
	resp, err := c.updateArtifactMetaAt(ctx, u, meta)
	if err != nil {
		if err.IsNotFound() {
			return c.delegate.UpdateArtifactVersionMeta(ctx, groupID, artifactID, version, meta)
		}
		return nil, err
	}
	return resp, nil
}

func (c *v3Client) updateArtifactMetaAt(ctx context.Context, u string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError) {
	basePayload := map[string]any{}
	if meta.Name != nil {
		basePayload["name"] = *meta.Name
	}
	if meta.Description != nil {
		basePayload["description"] = *meta.Description
	}

	var labels []string
	if meta.Labels != nil {
		labels = make([]string, 0, len(meta.Labels))
		seen := map[string]struct{}{}
		for _, l := range meta.Labels {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			if _, ok := seen[l]; ok {
				continue
			}
			seen[l] = struct{}{}
			labels = append(labels, l)
		}
	}

	headers := map[string]string{"Content-Type": "application/json", "Accept": "application/json"}

	// Prefer v3-style name/value label map.
	if meta.Labels != nil {
		payloadWithStringMapLabels := map[string]any{}
		for k, v := range basePayload {
			payloadWithStringMapLabels[k] = v
		}
		objStr := map[string]string{}
		for _, l := range labels {
			objStr[l] = "true"
		}
		payloadWithStringMapLabels["labels"] = objStr

		b, err := json.Marshal(payloadWithStringMapLabels)
		if err != nil {
			return nil, &ResponseError{Err: err}
		}
		if resp, rerr := c.doJSON(ctx, http.MethodPut, u, headers, bytes.NewReader(b)); rerr == nil {
			return resp, nil
		} else if rerr.StatusCode != http.StatusBadRequest {
			return nil, rerr
		}

		payloadWithNullMapLabels := map[string]any{}
		for k, v := range basePayload {
			payloadWithNullMapLabels[k] = v
		}
		objNull := map[string]any{}
		for _, l := range labels {
			objNull[l] = nil
		}
		payloadWithNullMapLabels["labels"] = objNull

		b, err = json.Marshal(payloadWithNullMapLabels)
		if err != nil {
			return nil, &ResponseError{Err: err}
		}
		if resp, rerr := c.doJSON(ctx, http.MethodPut, u, headers, bytes.NewReader(b)); rerr == nil {
			return resp, nil
		} else if rerr.StatusCode != http.StatusBadRequest {
			return nil, rerr
		}
	}

	payload := basePayload
	if meta.Labels != nil {
		payload = map[string]any{}
		for k, v := range basePayload {
			payload[k] = v
		}
		payload["labels"] = labels
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, &ResponseError{Err: err}
	}
	return c.doJSON(ctx, http.MethodPut, u, headers, bytes.NewReader(b))
}
func (c *v3Client) doRaw(ctx context.Context, method, urlStr string, headers map[string]string, body io.Reader) (*rawResp, *ResponseError) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, &ResponseError{Err: err}
	}
	applyAuth(req, c.cfg)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ResponseError{Err: err}
	}
	defer resp.Body.Close()

	b, _ := ReadBodyLimited(resp.Body)
	return &rawResp{StatusCode: resp.StatusCode, Body: b, Header: resp.Header}, nil
}

func (c *v3Client) doJSON(ctx context.Context, method, urlStr string, headers map[string]string, body io.Reader) (*ArtifactMetaResponse, *ResponseError) {
	resp, err := c.doRaw(ctx, method, urlStr, headers, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}

	trim := strings.TrimSpace(resp.Body)
	if trim == "" {
		return &ArtifactMetaResponse{Raw: map[string]any{}, Normalized: NormalizedArtifactMeta{}}, nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(trim), &raw); err != nil {
		return nil, &ResponseError{StatusCode: resp.StatusCode, Body: trim, Err: fmt.Errorf("failed to decode JSON: %w", err)}
	}

	n := normalizeMeta(raw)
	return &ArtifactMetaResponse{Raw: raw, Normalized: n}, nil
}

func (c *v3Client) VersionExists(ctx context.Context, groupID, artifactID, version string) (bool, *ResponseError) {
	return c.delegate.VersionExists(ctx, groupID, artifactID, version)
}

func (c *v3Client) DeleteArtifactVersion(ctx context.Context, groupID, artifactID, version string) *ResponseError {
	return c.delegate.DeleteArtifactVersion(ctx, groupID, artifactID, version)
}

func (c *v3Client) CreateArtifactVersion(ctx context.Context, groupID, artifactID string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError) {
	return c.delegate.CreateArtifactVersion(ctx, groupID, artifactID, version, content)
}

func (c *v3Client) DeleteArtifact(ctx context.Context, groupID, artifactID string, hardDelete bool) *ResponseError {
	return c.delegate.DeleteArtifact(ctx, groupID, artifactID, hardDelete)
}

func (c *v3Client) GetGlobalRule(ctx context.Context, ruleType string) (string, *ResponseError) {
	return c.delegate.GetGlobalRule(ctx, ruleType)
}

func (c *v3Client) PutGlobalRule(ctx context.Context, ruleType, config string) *ResponseError {
	return c.delegate.PutGlobalRule(ctx, ruleType, config)
}

func (c *v3Client) DeleteGlobalRule(ctx context.Context, ruleType string) *ResponseError {
	return c.delegate.DeleteGlobalRule(ctx, ruleType)
}

func (c *v3Client) GetArtifactRule(ctx context.Context, groupID, artifactID, ruleType string) (string, *ResponseError) {
	return c.delegate.GetArtifactRule(ctx, groupID, artifactID, ruleType)
}

func (c *v3Client) PutArtifactRule(ctx context.Context, groupID, artifactID, ruleType, config string) *ResponseError {
	return c.delegate.PutArtifactRule(ctx, groupID, artifactID, ruleType, config)
}

func (c *v3Client) DeleteArtifactRule(ctx context.Context, groupID, artifactID, ruleType string) *ResponseError {
	return c.delegate.DeleteArtifactRule(ctx, groupID, artifactID, ruleType)
}
