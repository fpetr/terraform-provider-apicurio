// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

var (
	_ resource.Resource                = &ruleResource{}
	_ resource.ResourceWithConfigure   = &ruleResource{}
	_ resource.ResourceWithImportState = &ruleResource{}
)

func NewRuleResource() resource.Resource {
	return &ruleResource{}
}

type ruleResource struct {
	client client.RegistryClient
}

type ruleResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Scope      types.String `tfsdk:"scope"`
	RuleType   types.String `tfsdk:"rule_type"`
	Config     types.String `tfsdk:"config"`
	GroupID    types.String `tfsdk:"group_id"`
	ArtifactID types.String `tfsdk:"artifact_id"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

func (r *ruleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rule"
}

func (r *ruleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Apicurio Registry rules (global or per-artifact).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal Terraform ID. Global: `global/<rule_type>`. Artifact: `<group_id>/<artifact_id>/<rule_type>`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scope": schema.StringAttribute{
				MarkdownDescription: "Rule scope: `global` or `artifact`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("global"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"rule_type": schema.StringAttribute{
				MarkdownDescription: "Rule type, e.g. `COMPATIBILITY`, `VALIDITY`.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"config": schema.StringAttribute{
				MarkdownDescription: "Rule configuration value, e.g. `BACKWARD`, `FULL`, `NONE`.",
				Required:            true,
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Group ID (required when scope=artifact).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"artifact_id": schema.StringAttribute{
				MarkdownDescription: "Artifact ID (required when scope=artifact).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "If false, the rule is deleted (disabled).",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *ruleResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.RequiredTogether(
			path.MatchRoot("group_id"),
			path.MatchRoot("artifact_id"),
		),
	}
}

func (r *ruleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(client.RegistryClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Configure Type",
			fmt.Sprintf("Expected client.RegistryClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = c
}

func (r *ruleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ruleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enabled := plan.Enabled.IsNull() || plan.Enabled.IsUnknown() || plan.Enabled.ValueBool()
	scope := strings.ToLower(strings.TrimSpace(plan.Scope.ValueString()))
	ruleType := strings.TrimSpace(plan.RuleType.ValueString())
	config := strings.TrimSpace(plan.Config.ValueString())
	groupID := strings.TrimSpace(plan.GroupID.ValueString())
	artifactID := strings.TrimSpace(plan.ArtifactID.ValueString())

	if err := validateRuleInputs(scope, groupID, artifactID); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	if scope == "artifact" {
		plan.ID = types.StringValue(fmt.Sprintf("%s/%s/%s", groupID, artifactID, ruleType))
	} else {
		plan.ID = types.StringValue(fmt.Sprintf("global/%s", ruleType))
		plan.GroupID = types.StringNull()
		plan.ArtifactID = types.StringNull()
	}

	if !enabled {
		// Disabled means ensure it doesn't exist, but keep Terraform state.
		if derr := r.deleteRule(ctx, scope, groupID, artifactID, ruleType); derr != nil && derr.StatusCode != 409 && !derr.IsNotFound() {
			resp.Diagnostics.AddError("Delete failed", formatClientError("unable to delete rule", derr))
			return
		}
		plan.Enabled = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	if scope == "artifact" {
		if err := r.client.PutArtifactRule(ctx, groupID, artifactID, ruleType, config); err != nil {
			resp.Diagnostics.AddError("Create failed", formatClientError("unable to set artifact rule", err))
			return
		}
	} else {
		if err := r.client.PutGlobalRule(ctx, ruleType, config); err != nil {
			resp.Diagnostics.AddError("Create failed", formatClientError("unable to set global rule", err))
			return
		}
	}

	plan.Enabled = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ruleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ruleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// When disabled, intentionally do not reconcile with remote state.
	if !state.Enabled.IsNull() && !state.Enabled.IsUnknown() && !state.Enabled.ValueBool() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	scope := strings.ToLower(strings.TrimSpace(state.Scope.ValueString()))
	ruleType := strings.TrimSpace(state.RuleType.ValueString())
	groupID := strings.TrimSpace(state.GroupID.ValueString())
	artifactID := strings.TrimSpace(state.ArtifactID.ValueString())

	if err := validateRuleInputs(scope, groupID, artifactID); err != nil {
		resp.Diagnostics.AddError("Invalid state", err.Error())
		return
	}

	if scope == "artifact" {
		cfg, err := r.client.GetArtifactRule(ctx, groupID, artifactID, ruleType)
		if err != nil {
			if err.IsNotFound() {
				if !state.Enabled.IsNull() && !state.Enabled.IsUnknown() && !state.Enabled.ValueBool() {
					// Disabled rule absent is expected; keep state.
					resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
					return
				}
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Read failed", formatClientError("unable to read artifact rule", err))
			return
		}
		state.Config = types.StringValue(cfg)
		state.Enabled = types.BoolValue(true)
		state.ID = types.StringValue(fmt.Sprintf("%s/%s/%s", groupID, artifactID, ruleType))
	} else {
		cfg, err := r.client.GetGlobalRule(ctx, ruleType)
		if err != nil {
			if err.IsNotFound() {
				if !state.Enabled.IsNull() && !state.Enabled.IsUnknown() && !state.Enabled.ValueBool() {
					resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
					return
				}
				resp.State.RemoveResource(ctx)
				return
			}
			resp.Diagnostics.AddError("Read failed", formatClientError("unable to read global rule", err))
			return
		}
		state.Config = types.StringValue(cfg)
		state.Enabled = types.BoolValue(true)
		state.ID = types.StringValue(fmt.Sprintf("global/%s", ruleType))
		state.GroupID = types.StringNull()
		state.ArtifactID = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ruleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ruleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	enabled := plan.Enabled.IsNull() || plan.Enabled.IsUnknown() || plan.Enabled.ValueBool()
	scope := strings.ToLower(strings.TrimSpace(plan.Scope.ValueString()))
	ruleType := strings.TrimSpace(plan.RuleType.ValueString())
	config := strings.TrimSpace(plan.Config.ValueString())
	groupID := strings.TrimSpace(plan.GroupID.ValueString())
	artifactID := strings.TrimSpace(plan.ArtifactID.ValueString())

	if err := validateRuleInputs(scope, groupID, artifactID); err != nil {
		resp.Diagnostics.AddError("Invalid configuration", err.Error())
		return
	}

	if scope == "artifact" {
		plan.ID = types.StringValue(fmt.Sprintf("%s/%s/%s", groupID, artifactID, ruleType))
	} else {
		plan.ID = types.StringValue(fmt.Sprintf("global/%s", ruleType))
		plan.GroupID = types.StringNull()
		plan.ArtifactID = types.StringNull()
	}

	if !enabled {
		if derr := r.deleteRule(ctx, scope, groupID, artifactID, ruleType); derr != nil && derr.StatusCode != 409 && !derr.IsNotFound() {
			resp.Diagnostics.AddError("Delete failed", formatClientError("unable to delete rule", derr))
			return
		}
		plan.Enabled = types.BoolValue(false)
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	if scope == "artifact" {
		if err := r.client.PutArtifactRule(ctx, groupID, artifactID, ruleType, config); err != nil {
			resp.Diagnostics.AddError("Update failed", formatClientError("unable to update artifact rule", err))
			return
		}
	} else {
		if err := r.client.PutGlobalRule(ctx, ruleType, config); err != nil {
			resp.Diagnostics.AddError("Update failed", formatClientError("unable to update global rule", err))
			return
		}
	}

	plan.Enabled = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ruleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ruleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scope := strings.ToLower(strings.TrimSpace(state.Scope.ValueString()))
	ruleType := strings.TrimSpace(state.RuleType.ValueString())
	groupID := strings.TrimSpace(state.GroupID.ValueString())
	artifactID := strings.TrimSpace(state.ArtifactID.ValueString())

	if err := validateRuleInputs(scope, groupID, artifactID); err != nil {
		resp.Diagnostics.AddError("Invalid state", err.Error())
		return
	}

	if derr := r.deleteRule(ctx, scope, groupID, artifactID, ruleType); derr != nil {
		if scope == "global" && derr.StatusCode == 409 {
			return
		}
		resp.Diagnostics.AddError("Delete failed", formatClientError("unable to delete rule", derr))
		return
	}
}

func (r *ruleResource) deleteRule(ctx context.Context, scope, groupID, artifactID, ruleType string) *client.ResponseError {
	if scope == "artifact" {
		return r.client.DeleteArtifactRule(ctx, groupID, artifactID, ruleType)
	}
	return r.client.DeleteGlobalRule(ctx, ruleType)
}

func (r *ruleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Supported import IDs:
	// - global/<rule_type>
	// - <group_id>/<artifact_id>/<rule_type>
	parts := strings.Split(req.ID, "/")
	if len(parts) == 2 && parts[0] == "global" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), "global")...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rule_type"), parts[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
		return
	}
	if len(parts) == 3 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), "artifact")...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("artifact_id"), parts[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("rule_type"), parts[2])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
		return
	}
	resp.Diagnostics.AddError("Unexpected import identifier", "Expected: global/<rule_type> OR <group_id>/<artifact_id>/<rule_type>")
}

func validateRuleInputs(scope, groupID, artifactID string) error {
	if scope == "" {
		scope = "global"
	}
	scope = strings.ToLower(scope)
	switch scope {
	case "global":
		if groupID != "" || artifactID != "" {
			return fmt.Errorf("group_id and artifact_id must not be set when scope=global")
		}
		return nil
	case "artifact":
		if strings.TrimSpace(groupID) == "" || strings.TrimSpace(artifactID) == "" {
			return fmt.Errorf("group_id and artifact_id are required when scope=artifact")
		}
		return nil
	default:
		return fmt.Errorf("scope must be 'global' or 'artifact'")
	}
}
