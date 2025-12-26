// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
)

// v3Client currently reuses v2 endpoint shapes as a best-effort fallback.
// Apicurio Registry v3 servers often still expose v2 Core API endpoints.
// If a server truly only exposes v3 endpoints, you can extend this client with
// the correct v3 paths without changing provider/resource code.
//
// For now, we implement v3 as:
// - Prefer /apis/registry/v3/* system probe
// - Use /apis/registry/v2/* semantics for CRUD (because the desired UX is v2 semantics)
// - If v2 endpoints are missing, operations will return clear HTTP errors.
//
// This keeps the abstraction layer in place while avoiding speculative v3 mapping.

type v3Client struct {
	delegate RegistryClient
}

func NewV3(endpoint string, httpClient *http.Client, cfg ClientConfig) RegistryClient {
	// Delegate to v2 semantics.
	return &v3Client{delegate: NewV2(endpoint, httpClient, cfg)}
}

func (c *v3Client) Flavor() ServerFlavor { return ServerFlavorV3 }

func (c *v3Client) GetArtifactMeta(ctx context.Context, groupID, artifactID string) (*ArtifactMetaResponse, *ResponseError) {
	return c.delegate.GetArtifactMeta(ctx, groupID, artifactID)
}

func (c *v3Client) GetLatestArtifactContent(ctx context.Context, groupID, artifactID string) ([]byte, *ResponseError) {
	return c.delegate.GetLatestArtifactContent(ctx, groupID, artifactID)
}

func (c *v3Client) CreateArtifact(ctx context.Context, groupID, artifactID, artifactType string, version *string, content []byte) (*ArtifactMetaResponse, *ResponseError) {
	return c.delegate.CreateArtifact(ctx, groupID, artifactID, artifactType, version, content)
}

func (c *v3Client) UpdateArtifactMeta(ctx context.Context, groupID, artifactID string, meta ArtifactMetaUpdate) (*ArtifactMetaResponse, *ResponseError) {
	return c.delegate.UpdateArtifactMeta(ctx, groupID, artifactID, meta)
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
