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
	"path"
	"strings"
	"time"
)

type v2Client struct {
	endpoint   string
	httpClient *http.Client
	cfg        ClientConfig
}

func NewV2(endpoint string, httpClient *http.Client, cfg ClientConfig) RegistryClient {
	return &v2Client{endpoint: endpoint, httpClient: httpClient, cfg: cfg}
}

func (c *v2Client) Flavor() ServerFlavor { return ServerFlavorV2 }

func (c *v2Client) base() string { return c.endpoint + "/apis/registry/v2" }

func (c *v2Client) GetArtifactMeta(ctx context.Context, groupID, artifactID string) (*ArtifactMetaResponse, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/meta"
	return c.doJSON(ctx, http.MethodGet, u, nil, nil)
}

func (c *v2Client) GetLatestArtifactContent(ctx context.Context, groupID, artifactID string) ([]byte, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, &ResponseError{Err: err}
	}
	applyAuth(req, c.cfg)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &ResponseError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := ReadBodyLimited(resp.Body)
		return nil, &ResponseError{StatusCode: resp.StatusCode, Body: body, Err: fmt.Errorf("unexpected response")}
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ResponseError{StatusCode: resp.StatusCode, Err: err}
	}
	return b, nil
}

func (c *v2Client) CreateArtifact(ctx context.Context, groupID, artifactID, artifactType string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts"
	headers := map[string]string{
		"X-Registry-ArtifactId":   artifactID,
		"X-Registry-ArtifactType": artifactType,
		"Content-Type":            "application/octet-stream",
	}
	if version != nil && strings.TrimSpace(*version) != "" {
		headers["X-Registry-Version"] = strings.TrimSpace(*version)
	}
	return c.doJSON(ctx, http.MethodPost, u, headers, bytes.NewReader(content))
}

func (c *v2Client) UpdateArtifactMeta(ctx context.Context, groupID, artifactID string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/meta"

	payload := map[string]any{}
	if meta.Name != nil {
		payload["name"] = *meta.Name
	}
	if meta.Description != nil {
		payload["description"] = *meta.Description
	}
	if meta.Labels != nil {
		labels := map[string]*string{}
		for _, l := range meta.Labels {
			l = strings.TrimSpace(l)
			if l == "" {
				continue
			}
			labels[l] = nil
		}
		payload["labels"] = labels
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return nil, &ResponseError{Err: err}
	}
	headers := map[string]string{"Content-Type": "application/json"}
	return c.doJSON(ctx, http.MethodPut, u, headers, bytes.NewReader(b))
}

func (c *v2Client) VersionExists(ctx context.Context, groupID, artifactID, version string) (bool, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/versions/" + url.PathEscape(version) + "/meta"
	resp, err := c.doRaw(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return false, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return true, nil
}

func (c *v2Client) DeleteArtifactVersion(ctx context.Context, groupID, artifactID, version string) *ResponseError {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/versions/" + url.PathEscape(version)
	resp, err := c.doRaw(ctx, http.MethodDelete, u, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return nil
}

func (c *v2Client) CreateArtifactVersion(ctx context.Context, groupID, artifactID string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/versions"
	headers := map[string]string{"Content-Type": "application/octet-stream"}
	if version != nil && strings.TrimSpace(*version) != "" {
		headers["X-Registry-Version"] = strings.TrimSpace(*version)
	}
	return c.doJSON(ctx, http.MethodPost, u, headers, bytes.NewReader(content))
}

func (c *v2Client) DeleteArtifact(ctx context.Context, groupID, artifactID string, hardDelete bool) *ResponseError {
	u, err := url.Parse(c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID))
	if err != nil {
		return &ResponseError{Err: err}
	}
	q := u.Query()
	if hardDelete {
		q.Set("hardDelete", "true")
	}
	u.RawQuery = q.Encode()

	resp, rerr := c.doRaw(ctx, http.MethodDelete, u.String(), nil, nil)
	if rerr != nil {
		return rerr
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return nil
}

func (c *v2Client) GetGlobalRule(ctx context.Context, ruleType string) (string, *ResponseError) {
	u := c.base() + "/rules/" + url.PathEscape(ruleType)
	resp, err := c.doRaw(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("not found")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return parseRuleConfig(resp.Body)
}

func (c *v2Client) PutGlobalRule(ctx context.Context, ruleType, config string) *ResponseError {
	u := c.base() + "/rules/" + url.PathEscape(ruleType)
	return c.putRuleConfig(ctx, u, config)
}

func (c *v2Client) DeleteGlobalRule(ctx context.Context, ruleType string) *ResponseError {
	u := c.base() + "/rules/" + url.PathEscape(ruleType)
	resp, err := c.doRaw(ctx, http.MethodDelete, u, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return nil
}

func (c *v2Client) GetArtifactRule(ctx context.Context, groupID, artifactID, ruleType string) (string, *ResponseError) {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/rules/" + url.PathEscape(ruleType)
	resp, err := c.doRaw(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		return "", &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("not found")}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return parseRuleConfig(resp.Body)
}

func (c *v2Client) PutArtifactRule(ctx context.Context, groupID, artifactID, ruleType, config string) *ResponseError {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/rules/" + url.PathEscape(ruleType)
	return c.putRuleConfig(ctx, u, config)
}

func (c *v2Client) DeleteArtifactRule(ctx context.Context, groupID, artifactID, ruleType string) *ResponseError {
	u := c.base() + "/groups/" + url.PathEscape(groupID) + "/artifacts/" + url.PathEscape(artifactID) + "/rules/" + url.PathEscape(ruleType)
	resp, err := c.doRaw(ctx, http.MethodDelete, u, nil, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return nil
}

func (c *v2Client) putRuleConfig(ctx context.Context, urlStr, config string) *ResponseError {
	payload := map[string]any{"config": config}
	b, err := json.Marshal(payload)
	if err != nil {
		return &ResponseError{Err: err}
	}
	headers := map[string]string{"Content-Type": "application/json"}
	resp, rerr := c.doRaw(ctx, http.MethodPut, urlStr, headers, bytes.NewReader(b))
	if rerr != nil {
		return rerr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}
	return nil
}

func parseRuleConfig(body string) (string, *ResponseError) {
	trim := strings.TrimSpace(body)
	if trim == "" {
		return "", nil
	}

	var obj struct {
		Config string `json:"config"`
	}
	if err := json.Unmarshal([]byte(trim), &obj); err == nil && strings.TrimSpace(obj.Config) != "" {
		return obj.Config, nil
	}

	var s string
	if err := json.Unmarshal([]byte(trim), &s); err == nil {
		return s, nil
	}

	// Best-effort: return raw body.
	return trim, nil
}

type rawResp struct {
	StatusCode int
	Body       string
	Header     http.Header
}

func (c *v2Client) doRaw(ctx context.Context, method, urlStr string, headers map[string]string, body io.Reader) (*rawResp, *ResponseError) {
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

func (c *v2Client) doJSON(ctx context.Context, method, urlStr string, headers map[string]string, body io.Reader) (*ArtifactMetaResponse, *ResponseError) {
	resp, err := c.doRaw(ctx, method, urlStr, headers, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ResponseError{StatusCode: resp.StatusCode, Body: resp.Body, Err: fmt.Errorf("unexpected response")}
	}

	// Some endpoints return empty body.
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

func normalizeMeta(raw map[string]any) NormalizedArtifactMeta {
	getStr := func(k string) string {
		if v, ok := raw[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	getInt64 := func(k string) *int64 {
		v, ok := raw[k]
		if !ok {
			return nil
		}
		switch t := v.(type) {
		case float64:
			i := int64(t)
			return &i
		case int64:
			i := t
			return &i
		case json.Number:
			if ii, err := t.Int64(); err == nil {
				return &ii
			}
		}
		return nil
	}
	parseTime := func(k string) *time.Time {
		// Apicurio commonly uses unix millis.
		v, ok := raw[k]
		if !ok {
			return nil
		}
		switch t := v.(type) {
		case float64:
			ms := int64(t)
			tm := time.Unix(0, ms*int64(time.Millisecond))
			return &tm
		case json.Number:
			if ms, err := t.Int64(); err == nil {
				tm := time.Unix(0, ms*int64(time.Millisecond))
				return &tm
			}
		case string:
			// best-effort RFC3339
			if tm, err := time.Parse(time.RFC3339, t); err == nil {
				return &tm
			}
		}
		return nil
	}
	labels := extractLabels(raw["labels"])

	return NormalizedArtifactMeta{
		GroupID:        firstNonEmpty(getStr("groupId"), getStr("group")),
		ArtifactID:     firstNonEmpty(getStr("artifactId"), getStr("id")),
		Name:           getStr("name"),
		Description:    getStr("description"),
		Labels:         labels,
		GlobalID:       firstNonNil(getInt64("globalId"), getInt64("globalID")),
		ContentID:      getInt64("contentId"),
		CreatedOn:      parseTime("createdOn"),
		ModifiedOn:     parseTime("modifiedOn"),
		LatestVersion:  firstNonEmpty(getStr("latestVersion"), getStr("version")),
		CurrentVersion: getStr("version"),
	}
}

func extractLabels(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case map[string]any:
		out := make([]string, 0, len(t))
		for k := range t {
			k = strings.TrimSpace(k)
			if k != "" {
				out = append(out, k)
			}
		}
		return out
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func firstNonNil(a, b *int64) *int64 {
	if a != nil {
		return a
	}
	return b
}

func joinURL(base string, elems ...string) string {
	u, _ := url.Parse(base)
	u.Path = path.Join(append([]string{u.Path}, elems...)...)
	return u.String()
}
