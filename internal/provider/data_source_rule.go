// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

var _ datasource.DataSource = &ruleDataSource{}

func NewRuleDataSource() datasource.DataSource {
	return &ruleDataSource{}
}

type ruleDataSource struct {
	client client.RegistryClient
}

type ruleDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Scope      types.String `tfsdk:"scope"`
	RuleType   types.String `tfsdk:"rule_type"`
	GroupID    types.String `tfsdk:"group_id"`
	ArtifactID types.String `tfsdk:"artifact_id"`
	Config     types.String `tfsdk:"config"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func (d *ruleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rule"
}

func (d *ruleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads an Apicurio Registry rule (global or per-artifact).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal Terraform ID. Global: `global/<rule_type>`. Artifact: `<group_id>/<artifact_id>/<rule_type>`.",
				Computed:            true,
			},
			"scope": schema.StringAttribute{
				MarkdownDescription: "Rule scope: `global` or `artifact`. Defaults to `global`.",
				Optional:            true,
			},
			"rule_type": schema.StringAttribute{
				MarkdownDescription: "Rule type, e.g. `COMPATIBILITY`, `VALIDITY`.",
				Required:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Group ID (required when scope=artifact).",
				Optional:            true,
			},
			"artifact_id": schema.StringAttribute{
				MarkdownDescription: "Artifact ID (required when scope=artifact).",
				Optional:            true,
			},
			"config": schema.StringAttribute{
				MarkdownDescription: "Rule configuration value.",
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "True if the rule exists.",
				Computed:            true,
			},
		},
	}
}

func (d *ruleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ruleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ruleDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope := strings.ToLower(strings.TrimSpace(data.Scope.ValueString()))
	if data.Scope.IsNull() || data.Scope.IsUnknown() || scope == "" {
		scope = "global"
	}
	ruleType := strings.TrimSpace(data.RuleType.ValueString())
	groupID := strings.TrimSpace(data.GroupID.ValueString())
	artifactID := strings.TrimSpace(data.ArtifactID.ValueString())

	if ruleType == "" {
		resp.Diagnostics.AddError("Invalid configuration", "rule_type must be set")
		return
	}
	if err := validateRuleInputs(scope, groupID, artifactID); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	if scope == "artifact" {
		data.ID = types.StringValue(fmt.Sprintf("%s/%s/%s", groupID, artifactID, ruleType))
		cfg, err := d.client.GetArtifactRule(ctx, groupID, artifactID, ruleType)
		if err != nil {
			if err.IsNotFound() {
				data.Enabled = types.BoolValue(false)
				data.Config = types.StringNull()
				resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
				return
			}
			resp.Diagnostics.AddError("Read failed", formatClientError("unable to read artifact rule", err))
			return
		}
		data.Enabled = types.BoolValue(true)
		data.Config = types.StringValue(cfg)
	} else {
		data.ID = types.StringValue(fmt.Sprintf("global/%s", ruleType))
		data.GroupID = types.StringNull()
		data.ArtifactID = types.StringNull()
		cfg, err := d.client.GetGlobalRule(ctx, ruleType)
		if err != nil {
			if err.IsNotFound() {
				data.Enabled = types.BoolValue(false)
				data.Config = types.StringNull()
				resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
				return
			}
			resp.Diagnostics.AddError("Read failed", formatClientError("unable to read global rule", err))
			return
		}
		data.Enabled = types.BoolValue(true)
		data.Config = types.StringValue(cfg)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
