// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

type ServerFlavor string

const (
	ServerFlavorV2 ServerFlavor = "v2"
	ServerFlavorV3 ServerFlavor = "v3"
)

type ClientConfig struct {
	Endpoint           string
	APIVersion         ServerFlavor
	AuthHeader         string // optional: literal "Header-Name: value"
	Token              string // optional: used to construct Authorization: Bearer <token>
	BasicAuth          *BasicAuthConfig
	OIDC               *OIDCConfig
	InsecureSkipVerify bool
	CABundlePath       string
	Timeout            time.Duration
}

type BasicAuthConfig struct {
	Username string
	Password string
}

type OIDCConfig struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
	ExtraParams  map[string]string
	AuthStyle    string // auto | in_header | in_params
}

type RegistryClient interface {
	Flavor() ServerFlavor

	GetArtifactMeta(ctx context.Context, groupID, artifactID string) (*ArtifactMetaResponse, *ResponseError)
	GetLatestArtifactContent(ctx context.Context, groupID, artifactID string) ([]byte, *ResponseError)

	CreateArtifact(ctx context.Context, groupID, artifactID, artifactType string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError)
	UpdateArtifactMeta(ctx context.Context, groupID, artifactID string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError)
	UpdateArtifactVersionMeta(ctx context.Context, groupID, artifactID, version string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError)

	VersionExists(ctx context.Context, groupID, artifactID, version string) (bool, *ResponseError)
	DeleteArtifactVersion(ctx context.Context, groupID, artifactID, version string) *ResponseError
	CreateArtifactVersion(ctx context.Context, groupID, artifactID string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError)

	DeleteArtifact(ctx context.Context, groupID, artifactID string, hardDelete bool) *ResponseError

	// Group operations (best-effort cleanup helpers).
	// Note: Apicurio groups are not automatically deleted when the last artifact is deleted.
	GroupHasAnyArtifacts(ctx context.Context, groupID string) (bool, *ResponseError)
	DeleteGroup(ctx context.Context, groupID string) *ResponseError

	GetGlobalRule(ctx context.Context, ruleType string) (string, *ResponseError)
	PutGlobalRule(ctx context.Context, ruleType, config string) *ResponseError
	DeleteGlobalRule(ctx context.Context, ruleType string) *ResponseError

	GetArtifactRule(ctx context.Context, groupID, artifactID, ruleType string) (string, *ResponseError)
	PutArtifactRule(ctx context.Context, groupID, artifactID, ruleType, config string) *ResponseError
	DeleteArtifactRule(ctx context.Context, groupID, artifactID, ruleType string) *ResponseError
}

type ResponseError struct {
	StatusCode int
	Body       string
	Err        error
}

func (e *ResponseError) Error() string {
	if e == nil {
		return ""
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("status %d: %v: %s", e.StatusCode, e.Err, e.Body)
	}
	return fmt.Sprintf("%v: %s", e.Err, e.Body)
}

func (e *ResponseError) IsNotFound() bool {
	return e != nil && e.StatusCode == http.StatusNotFound
}

type ArtifactMetaResponse struct {
	// Raw JSON is decoded client-side in a flexible way; we then normalize.
	Normalized NormalizedArtifactMeta
	Raw        map[string]any
}

type NormalizedArtifactMeta struct {
	GroupID        string
	ArtifactID     string
	Name           string
	Description    string
	Labels         []string
	GlobalID       *int64
	ContentID      *int64
	CreatedOn      *time.Time
	ModifiedOn     *time.Time
	LatestVersion  string
	CurrentVersion string
}

type ArtifactMetaUpdate struct {
	Name        *string
	Description *string
	Labels      []string // treated as a list; sent as map keys -> null (preferred) or array (fallback)
}

func New(ctx context.Context, cfg ClientConfig) (RegistryClient, diag.Diagnostics) {
	var diags diag.Diagnostics

	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if endpoint == "" {
		diags.AddError("Invalid endpoint", "endpoint must be set")
		return nil, diags
	}

	httpClient, d := newHTTPClient(cfg)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	// If explicitly configured, skip probing and select that flavor.
	if strings.TrimSpace(string(cfg.APIVersion)) != "" {
		switch cfg.APIVersion {
		case ServerFlavorV2:
			return NewV2(endpoint, httpClient, cfg), diags
		case ServerFlavorV3:
			return NewV3(endpoint, httpClient, cfg), diags
		default:
			diags.AddError("Invalid api_version", fmt.Sprintf("unsupported api_version %q (expected v2 or v3)", cfg.APIVersion))
			return nil, diags
		}
	}

	// Best-effort capability detection (prefer v3 when not specified).
	if ok, _ := probe(ctx, httpClient, endpoint+"/apis/registry/v3/system/info"); ok {
		return NewV3(endpoint, httpClient, cfg), diags
	}
	if ok, _ := probe(ctx, httpClient, endpoint+"/apis/registry/v2/system/info"); ok {
		return NewV2(endpoint, httpClient, cfg), diags
	}

	// Default to v3 semantics; calls will still return useful diagnostics if endpoints are missing.
	return NewV3(endpoint, httpClient, cfg), diags
}

func newHTTPClient(cfg ClientConfig) (*http.Client, diag.Diagnostics) {
	var diags diag.Diagnostics

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	tlsCfg := &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify} //nolint:gosec // user-configured

	if strings.TrimSpace(cfg.CABundlePath) != "" {
		pem, err := os.ReadFile(cfg.CABundlePath)
		if err != nil {
			diags.AddError("Failed to read ca_bundle_path", err.Error())
			return nil, diags
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if ok := pool.AppendCertsFromPEM(pem); !ok {
			diags.AddError("Invalid ca_bundle_path", "failed to parse any certificates from PEM")
			return nil, diags
		}
		tlsCfg.RootCAs = pool
	}

	baseTransport := &http.Transport{TLSClientConfig: tlsCfg}

	var tokenSource oauth2.TokenSource
	if cfg.OIDC != nil {
		ts, err := newOIDCTokenSource(context.Background(), baseTransport, timeout, *cfg.OIDC)
		if err != nil {
			diags.AddError("Invalid oidc", err.Error())
			return nil, diags
		}
		tokenSource = ts
	}

	transport := &authTransport{base: baseTransport, cfg: cfg, oidcTokenSource: tokenSource}
	return &http.Client{Timeout: timeout, Transport: transport}, diags
}

type authTransport struct {
	base            http.RoundTripper
	cfg             ClientConfig
	oidcTokenSource oauth2.TokenSource

	mu sync.Mutex
}

func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone to avoid mutating the original request.
	r := req.Clone(req.Context())
	if r.Header == nil {
		r.Header = make(http.Header)
	}

	// If caller already set Authorization (or custom header), respect it.
	applyAuth(r, t.cfg)

	if t.oidcTokenSource != nil && r.Header.Get("Authorization") == "" {
		// TokenSource is safe for concurrent use in practice, but keep our own mutex
		// in case the underlying source isn't.
		t.mu.Lock()
		tok, err := t.oidcTokenSource.Token()
		t.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if tok != nil && tok.AccessToken != "" {
			r.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		}
	}

	return t.base.RoundTrip(r)
}

func newOIDCTokenSource(ctx context.Context, baseTransport http.RoundTripper, timeout time.Duration, cfg OIDCConfig) (oauth2.TokenSource, error) {
	tokenURL := strings.TrimSpace(cfg.TokenURL)
	clientID := strings.TrimSpace(cfg.ClientID)
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if tokenURL == "" || clientID == "" || clientSecret == "" {
		return nil, errors.New("oidc.token_url, oidc.client_id, and oidc.client_secret must be set")
	}

	authStyle := strings.ToLower(strings.TrimSpace(cfg.AuthStyle))
	if authStyle == "" {
		authStyle = "auto"
	}
	var style oauth2.AuthStyle
	switch authStyle {
	case "auto":
		style = oauth2.AuthStyleAutoDetect
	case "in_header":
		style = oauth2.AuthStyleInHeader
	case "in_params":
		style = oauth2.AuthStyleInParams
	default:
		return nil, fmt.Errorf("unsupported oidc.auth_style %q (expected auto, in_header, or in_params)", cfg.AuthStyle)
	}

	endpointParams := url.Values{}
	for k, v := range cfg.ExtraParams {
		kk := strings.TrimSpace(k)
		vv := strings.TrimSpace(v)
		if kk == "" || vv == "" {
			continue
		}
		endpointParams.Set(kk, vv)
	}

	// Use an HTTP client with the same TLS settings (but without auth injection)
	// for token acquisition.
	hc := &http.Client{Transport: baseTransport, Timeout: timeout}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)

	cc := clientcredentials.Config{
		ClientID:       clientID,
		ClientSecret:   clientSecret,
		TokenURL:       tokenURL,
		Scopes:         cfg.Scopes,
		EndpointParams: endpointParams,
		AuthStyle:      style,
	}

	return oauth2.ReuseTokenSource(nil, cc.TokenSource(ctx)), nil
}

func probe(ctx context.Context, httpClient *http.Client, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func applyAuth(req *http.Request, cfg ClientConfig) {
	if h := strings.TrimSpace(cfg.AuthHeader); h != "" {
		// Expect: "Header-Name: value"
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if name != "" {
				req.Header.Set(name, val)
				return
			}
		}
		// If malformed, do nothing; provider will surface a config diagnostic.
	}
	if t := strings.TrimSpace(cfg.Token); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
		return
	}
	if cfg.BasicAuth != nil {
		u := strings.TrimSpace(cfg.BasicAuth.Username)
		p := cfg.BasicAuth.Password
		if u != "" && p != "" {
			req.SetBasicAuth(u, p)
			return
		}
	}
}

func ValidateAuthHeader(authHeader string) error {
	h := strings.TrimSpace(authHeader)
	if h == "" {
		return nil
	}
	parts := strings.SplitN(h, ":", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return errors.New("auth_header must be in the form 'Header-Name: value'")
	}
	return nil
}

func ReadBodyLimited(r io.Reader) (string, error) {
	const maxBodyBytes = 64 * 1024
	b, err := io.ReadAll(io.LimitReader(r, maxBodyBytes))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
