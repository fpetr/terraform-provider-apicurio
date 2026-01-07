// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

var _ datasource.DataSource = &artifactDataSource{}

func NewArtifactDataSource() datasource.DataSource {
	return &artifactDataSource{}
}

type artifactDataSource struct {
	client client.RegistryClient
}

type artifactDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	GroupID    types.String `tfsdk:"group_id"`
	ArtifactID types.String `tfsdk:"artifact_id"`

	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Labels      types.Set    `tfsdk:"labels"`

	GlobalID               types.Int64  `tfsdk:"global_id"`
	ContentID              types.Int64  `tfsdk:"content_id"`
	CreatedOn              types.String `tfsdk:"created_on"`
	ModifiedOn             types.String `tfsdk:"modified_on"`
	LatestVersion          types.String `tfsdk:"latest_version"`
	ContentSHA256          types.String `tfsdk:"content_sha256"`
	ContentCanonicalSHA256 types.String `tfsdk:"content_canonical_sha256"`
}

func (d *artifactDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_artifact"
}

func (d *artifactDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an Apicurio Registry artifact. Uses v3 endpoints when available, otherwise falls back to v2.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal Terraform ID in the form `<group_id>/<artifact_id>`.",
				Computed:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Artifact group ID.",
				Required:            true,
			},
			"artifact_id": schema.StringAttribute{
				MarkdownDescription: "Artifact ID.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Artifact display name.",
				Computed:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Artifact description.",
				Computed:            true,
			},
			"labels": schema.SetAttribute{
				MarkdownDescription: "Labels applied to the artifact.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"global_id": schema.Int64Attribute{
				MarkdownDescription: "Apicurio global ID (if exposed by the server).",
				Computed:            true,
			},
			"content_id": schema.Int64Attribute{
				MarkdownDescription: "Apicurio content ID (if exposed by the server).",
				Computed:            true,
			},
			"created_on": schema.StringAttribute{
				MarkdownDescription: "Creation time (RFC3339) if exposed by the server.",
				Computed:            true,
			},
			"modified_on": schema.StringAttribute{
				MarkdownDescription: "Last modification time (RFC3339) if exposed by the server.",
				Computed:            true,
			},
			"latest_version": schema.StringAttribute{
				MarkdownDescription: "Latest version string as reported by the server (best-effort).",
				Computed:            true,
			},
			"content_sha256": schema.StringAttribute{
				MarkdownDescription: "SHA256 of the latest content in the registry (best-effort).",
				Computed:            true,
			},
			"content_canonical_sha256": schema.StringAttribute{
				MarkdownDescription: "SHA256 of the latest content in the registry after JSON canonicalization (best-effort). For JSON-based artifact types (e.g. AVRO, JSON), this removes formatting-only differences (whitespace/indentation/object key ordering).",
				Computed:            true,
			},
		},
	}
}

func (d *artifactDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(client.RegistryClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected client.RegistryClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = c
}

func (d *artifactDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data artifactDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := strings.TrimSpace(data.GroupID.ValueString())
	artifactID := strings.TrimSpace(data.ArtifactID.ValueString())
	if groupID == "" || artifactID == "" {
		resp.Diagnostics.AddError("Invalid configuration", "group_id and artifact_id must be set")
		return
	}

	meta, err := d.client.GetArtifactMeta(ctx, groupID, artifactID)
	if err != nil {
		if err.IsNotFound() {
			resp.Diagnostics.AddError("Artifact not found", fmt.Sprintf("Artifact %q/%q was not found", groupID, artifactID))
			return
		}
		resp.Diagnostics.AddError("Read failed", formatClientError("unable to read artifact metadata", err))
		return
	}

	n := meta.Normalized
	data.ID = types.StringValue(groupID + "/" + artifactID)

	if strings.TrimSpace(n.Name) != "" {
		data.Name = types.StringValue(n.Name)
	}
	if n.Description != "" {
		data.Description = types.StringValue(n.Description)
	}
	if n.Labels != nil {
		data.Labels = stringsToSet(n.Labels)
	}
	if n.GlobalID != nil {
		data.GlobalID = types.Int64Value(*n.GlobalID)
	}
	if n.ContentID != nil {
		data.ContentID = types.Int64Value(*n.ContentID)
	}
	if n.CreatedOn != nil {
		data.CreatedOn = types.StringValue(n.CreatedOn.UTC().Format(time.RFC3339))
	}
	if n.ModifiedOn != nil {
		data.ModifiedOn = types.StringValue(n.ModifiedOn.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(n.LatestVersion) != "" {
		data.LatestVersion = types.StringValue(n.LatestVersion)
	}

	if content, cerr := d.client.GetLatestArtifactContent(ctx, groupID, artifactID); cerr == nil {
		data.ContentSHA256 = types.StringValue(sha256hex(content))
		if canon, err := canonicalizeJSON(content); err == nil {
			data.ContentCanonicalSHA256 = types.StringValue(sha256hex(canon))
		} else {
			data.ContentCanonicalSHA256 = types.StringValue(sha256hex(content))
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
