// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

var (
	_ resource.Resource                = &artifactResource{}
	_ resource.ResourceWithConfigure   = &artifactResource{}
	_ resource.ResourceWithImportState = &artifactResource{}
)

func NewArtifactResource() resource.Resource {
	return &artifactResource{}
}

type artifactResource struct {
	client client.RegistryClient
}

type artifactResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	GroupID               types.String `tfsdk:"group_id"`
	ArtifactID            types.String `tfsdk:"artifact_id"`
	ArtifactType          types.String `tfsdk:"artifact_type"`
	Content               types.String `tfsdk:"content"`
	ContentFile           types.String `tfsdk:"content_file"`
	Version               types.String `tfsdk:"version"`
	AllowOverwriteVersion types.Bool   `tfsdk:"allow_overwrite_version"`
	Labels                types.Set    `tfsdk:"labels"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	HardDelete            types.Bool   `tfsdk:"hard_delete"`

	GlobalID      types.Int64  `tfsdk:"global_id"`
	ContentID     types.Int64  `tfsdk:"content_id"`
	CreatedOn     types.String `tfsdk:"created_on"`
	ModifiedOn    types.String `tfsdk:"modified_on"`
	LatestVersion types.String `tfsdk:"latest_version"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
}

func (r *artifactResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_artifact"
}

func (r *artifactResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an Apicurio Registry artifact (group/artifactId/custom version/labels). Uses v3 endpoints when available, otherwise falls back to v2.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Internal Terraform ID in the form `<group_id>/<artifact_id>`.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				MarkdownDescription: "Artifact group ID.",
				Required:            true,
			},
			"artifact_id": schema.StringAttribute{
				MarkdownDescription: "Artifact ID.",
				Required:            true,
			},
			"artifact_type": schema.StringAttribute{
				MarkdownDescription: "Artifact type. One of: `AVRO`, `JSON`, `PROTOBUF`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("AVRO"),
				Validators: []validator.String{
					stringvalidator.OneOf("AVRO", "JSON", "PROTOBUF"),
				},
			},
			"content": schema.StringAttribute{
				MarkdownDescription: "Artifact content as a string.",
				Optional:            true,
				Sensitive:           true,
			},
			"content_file": schema.StringAttribute{
				MarkdownDescription: "Path to a file containing the artifact content.",
				Optional:            true,
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Custom version string (e.g. `v1`). If set, the provider creates/updates that specific version.",
				Optional:            true,
			},
			"allow_overwrite_version": schema.BoolAttribute{
				MarkdownDescription: "If true and `version` already exists, the provider will attempt to delete that version and recreate it with new content.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"labels": schema.SetAttribute{
				MarkdownDescription: "Labels to apply to the artifact. Apicurio UI shows them as keys with null values.",
				Optional:            true,
				ElementType:         types.StringType,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Artifact display name.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Artifact description.",
				Optional:            true,
			},
			"hard_delete": schema.BoolAttribute{
				MarkdownDescription: "If true, hard-delete the artifact. Default is soft delete.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"global_id": schema.Int64Attribute{
				MarkdownDescription: "Apicurio global ID (if exposed by the server).",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"content_id": schema.Int64Attribute{
				MarkdownDescription: "Apicurio content ID (if exposed by the server).",
				Computed:            true,
				PlanModifiers:       []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"created_on": schema.StringAttribute{
				MarkdownDescription: "Creation time (RFC3339) if exposed by the server.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"modified_on": schema.StringAttribute{
				MarkdownDescription: "Last modification time (RFC3339) if exposed by the server.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"latest_version": schema.StringAttribute{
				MarkdownDescription: "Latest version string as reported by the server (best-effort).",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"content_sha256": schema.StringAttribute{
				MarkdownDescription: "SHA256 of the latest content in the registry (best-effort).",
				Computed:            true,
			},
		},
	}
}

func (r *artifactResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.ExactlyOneOf(
			path.MatchRoot("content"),
			path.MatchRoot("content_file"),
		),
	}
}

func (r *artifactResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *artifactResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config artifactResourceModel
	var plan artifactResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	content, diags := resolveContentFromPlan(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Prevent accidental adoption; require import.
	if meta, err := r.client.GetArtifactMeta(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString()); err == nil && meta != nil {
		resp.Diagnostics.AddError(
			"Artifact already exists",
			"The artifact already exists in Apicurio. Import it using: terraform import apicurio_artifact.<name> <group_id>/<artifact_id>",
		)
		return
	} else if err != nil && !err.IsNotFound() {
		resp.Diagnostics.AddError("Read failed", formatClientError("unable to check existing artifact", err))
		return
	}

	var versionPtr *string
	if !plan.Version.IsNull() && !plan.Version.IsUnknown() {
		v := strings.TrimSpace(plan.Version.ValueString())
		if v != "" {
			versionPtr = &v
		}
	}

	_, cerr := r.client.CreateArtifact(
		ctx,
		plan.GroupID.ValueString(),
		plan.ArtifactID.ValueString(),
		plan.ArtifactType.ValueString(),
		versionPtr,
		content,
	)
	if cerr != nil {
		resp.Diagnostics.AddError("Create failed", formatClientError("unable to create artifact", cerr))
		return
	}

	// Apply metadata.
	var namePtr *string
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		n := strings.TrimSpace(config.Name.ValueString())
		if n != "" {
			namePtr = &n
		}
	}
	var descPtr *string
	if !config.Description.IsNull() && !config.Description.IsUnknown() {
		d := config.Description.ValueString()
		descPtr = &d
	}
	labels := setToStrings(ctx, config.Labels)
	metaUpdate := client.ArtifactMetaUpdate{
		Name:        namePtr,
		Description: descPtr,
		Labels:      labels,
	}
	_, merr := r.client.UpdateArtifactMeta(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), metaUpdate)
	if merr != nil {
		resp.Diagnostics.AddError("Metadata update failed", formatClientError("unable to update artifact metadata", merr))
		return
	}
	if versionPtr != nil {
		if _, verr := r.client.UpdateArtifactVersionMeta(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), *versionPtr, metaUpdate); verr != nil {
			resp.Diagnostics.AddError("Metadata update failed", formatClientError("unable to update artifact version metadata", verr))
			return
		}
	}

	// Refresh state (must return known values after apply).
	state, d := r.readIntoState(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ContentSHA256 = types.StringValue(sha256hex(content))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *artifactResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state artifactResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	meta, err := r.client.GetArtifactMeta(ctx, state.GroupID.ValueString(), state.ArtifactID.ValueString())
	if err != nil {
		if err.IsNotFound() {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Read failed", formatClientError("unable to read artifact metadata", err))
		return
	}

	// Preserve config-side fields; refresh computed/metadata.
	state = applyMetaToState(state, meta.Normalized)

	// Best-effort content hash.
	content, cerr := r.client.GetLatestArtifactContent(ctx, state.GroupID.ValueString(), state.ArtifactID.ValueString())
	if cerr != nil {
		resp.Diagnostics.AddWarning("Content read failed", formatClientError("unable to read artifact content to compute content_sha256", cerr))
	} else {
		if state.ContentSHA256.IsNull() || state.ContentSHA256.IsUnknown() || strings.TrimSpace(state.ContentSHA256.ValueString()) == "" {
			state.ContentSHA256 = types.StringValue(sha256hex(content))
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *artifactResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config artifactResourceModel
	var plan artifactResourceModel
	var state artifactResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	content, diags := resolveContentFromPlan(plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	newHash := sha256hex(content)
	oldHash := state.ContentSHA256.ValueString()
	contentChanged := oldHash == "" || newHash != oldHash

	var requestedVersion *string
	if !plan.Version.IsNull() && !plan.Version.IsUnknown() {
		v := strings.TrimSpace(plan.Version.ValueString())
		if v != "" {
			requestedVersion = &v
		}
	}

	metadataChanged := metadataDiffers(ctx, config, plan, state)

	if contentChanged {
		if requestedVersion != nil {
			exists, eerr := r.client.VersionExists(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), *requestedVersion)
			if eerr != nil {
				resp.Diagnostics.AddError("Version check failed", formatClientError("unable to check version existence", eerr))
				return
			}
			allowOverwrite := !plan.AllowOverwriteVersion.IsNull() && plan.AllowOverwriteVersion.ValueBool()
			if exists && !allowOverwrite {
				resp.Diagnostics.AddError(
					"Version already exists",
					"The requested version already exists in Apicurio. Set allow_overwrite_version=true to delete and recreate that version.",
				)
				return
			}
			if exists && allowOverwrite {
				if derr := r.client.DeleteArtifactVersion(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), *requestedVersion); derr != nil {
					resp.Diagnostics.AddError("Version delete failed", formatClientError("unable to delete existing version", derr))
					return
				}
			}
		}

		if _, uerr := r.client.CreateArtifactVersion(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), requestedVersion, content); uerr != nil {
			resp.Diagnostics.AddError("Content update failed", formatClientError("unable to create new artifact version", uerr))
			return
		}
	}

	if metadataChanged {
		var namePtr *string
		if !config.Name.IsNull() && !config.Name.IsUnknown() {
			n := strings.TrimSpace(config.Name.ValueString())
			if n != "" {
				namePtr = &n
			}
		}
		var descPtr *string
		if !config.Description.IsNull() && !config.Description.IsUnknown() {
			d := config.Description.ValueString()
			descPtr = &d
		}
		labels := setToStrings(ctx, config.Labels)
		metaUpdate := client.ArtifactMetaUpdate{
			Name:        namePtr,
			Description: descPtr,
			Labels:      labels,
		}
		if _, merr := r.client.UpdateArtifactMeta(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), metaUpdate); merr != nil {
			resp.Diagnostics.AddError("Metadata update failed", formatClientError("unable to update artifact metadata", merr))
			return
		}
		if requestedVersion != nil {
			if _, verr := r.client.UpdateArtifactVersionMeta(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), *requestedVersion, metaUpdate); verr != nil {
				resp.Diagnostics.AddError("Metadata update failed", formatClientError("unable to update artifact version metadata", verr))
				return
			}
		}
	}

	// Refresh state (must return known values after apply).
	newState, d := r.readIntoState(ctx, plan.GroupID.ValueString(), plan.ArtifactID.ValueString(), plan)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	newState.ContentSHA256 = types.StringValue(newHash)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newState)...)
}

func (r *artifactResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state artifactResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hardDelete := !state.HardDelete.IsNull() && state.HardDelete.ValueBool()
	if derr := r.client.DeleteArtifact(ctx, state.GroupID.ValueString(), state.ArtifactID.ValueString(), hardDelete); derr != nil {
		resp.Diagnostics.AddError("Delete failed", formatClientError("unable to delete artifact", derr))
		return
	}

	// Apicurio does not automatically delete groups when the last artifact is deleted.
	// When hard_delete=true, do a best-effort cleanup to delete the group if it is empty.
	if hardDelete {
		groupID := strings.TrimSpace(state.GroupID.ValueString())
		if groupID != "" {
			_, gerr := deleteGroupIfEmpty(ctx, r.client, groupID)
			if gerr != nil {
				resp.Diagnostics.AddWarning("Group cleanup failed", formatClientError("unable to clean up empty group (best-effort)", gerr))
			}
		}
	}
}

type groupCleanupClient interface {
	GroupHasAnyArtifacts(ctx context.Context, groupID string) (bool, *client.ResponseError)
	DeleteGroup(ctx context.Context, groupID string) *client.ResponseError
}

func deleteGroupIfEmpty(ctx context.Context, c groupCleanupClient, groupID string) (bool, *client.ResponseError) {
	hasAny, err := c.GroupHasAnyArtifacts(ctx, groupID)
	if err != nil {
		return false, err
	}
	if hasAny {
		return false, nil
	}
	if err := c.DeleteGroup(ctx, groupID); err != nil {
		return false, err
	}
	return true, nil
}

func (r *artifactResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		resp.Diagnostics.AddError("Unexpected import identifier", "Expected import identifier with format: <group_id>/<artifact_id>")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("artifact_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)

	// Defaults are not applied automatically during import.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("artifact_type"), "AVRO")...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("allow_overwrite_version"), false)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("hard_delete"), false)...)
}

func (r *artifactResource) readIntoState(ctx context.Context, groupID, artifactID string, plan artifactResourceModel) (artifactResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics

	meta, err := r.client.GetArtifactMeta(ctx, groupID, artifactID)
	if err != nil {
		diags.AddError("Read failed", formatClientError("unable to read artifact metadata", err))
		return artifactResourceModel{}, diags
	}

	state := plan
	state.ID = types.StringValue(groupID + "/" + artifactID)
	state = applyMetaToState(state, meta.Normalized)
	state.ContentSHA256 = types.StringValue(sha256hexFromPlan(plan))
	if state.ContentSHA256.ValueString() == "" {
		// Fallback to server content hash.
		content, cerr := r.client.GetLatestArtifactContent(ctx, groupID, artifactID)
		if cerr == nil {
			state.ContentSHA256 = types.StringValue(sha256hex(content))
		}
	}

	return state, diags
}

func applyMetaToState(state artifactResourceModel, n client.NormalizedArtifactMeta) artifactResourceModel {
	state.ID = types.StringValue(state.GroupID.ValueString() + "/" + state.ArtifactID.ValueString())

	if strings.TrimSpace(n.Name) != "" {
		state.Name = types.StringValue(n.Name)
	}
	if n.Description != "" {
		state.Description = types.StringValue(n.Description)
	}
	if n.Labels != nil && (state.Labels.IsNull() || state.Labels.IsUnknown()) {
		state.Labels = stringsToSet(n.Labels)
	}
	if n.GlobalID != nil && (state.GlobalID.IsNull() || state.GlobalID.IsUnknown()) {
		state.GlobalID = types.Int64Value(*n.GlobalID)
	}
	if n.ContentID != nil && (state.ContentID.IsNull() || state.ContentID.IsUnknown()) {
		state.ContentID = types.Int64Value(*n.ContentID)
	}
	if n.CreatedOn != nil && (state.CreatedOn.IsNull() || state.CreatedOn.IsUnknown() || strings.TrimSpace(state.CreatedOn.ValueString()) == "") {
		state.CreatedOn = types.StringValue(n.CreatedOn.UTC().Format(time.RFC3339))
	}
	if n.ModifiedOn != nil && (state.ModifiedOn.IsNull() || state.ModifiedOn.IsUnknown() || strings.TrimSpace(state.ModifiedOn.ValueString()) == "") {
		state.ModifiedOn = types.StringValue(n.ModifiedOn.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(n.LatestVersion) != "" && (state.LatestVersion.IsNull() || state.LatestVersion.IsUnknown() || strings.TrimSpace(state.LatestVersion.ValueString()) == "") {
		state.LatestVersion = types.StringValue(n.LatestVersion)
	}

	// If the server omits some computed fields (common across API flavors), ensure we don't
	// leave Unknown values in state after apply.
	if state.GlobalID.IsUnknown() {
		state.GlobalID = types.Int64Null()
	}
	if state.ContentID.IsUnknown() {
		state.ContentID = types.Int64Null()
	}
	if state.CreatedOn.IsUnknown() {
		state.CreatedOn = types.StringNull()
	}
	if state.ModifiedOn.IsUnknown() {
		state.ModifiedOn = types.StringNull()
	}
	if state.LatestVersion.IsUnknown() {
		state.LatestVersion = types.StringNull()
	}
	if state.Name.IsUnknown() {
		state.Name = types.StringNull()
	}
	return state
}

func resolveContentFromPlan(plan artifactResourceModel) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !plan.Content.IsNull() && !plan.Content.IsUnknown() {
		return []byte(plan.Content.ValueString()), diags
	}
	if !plan.ContentFile.IsNull() && !plan.ContentFile.IsUnknown() {
		p := strings.TrimSpace(plan.ContentFile.ValueString())
		b, err := os.ReadFile(p)
		if err != nil {
			diags.AddError("Failed to read content_file", err.Error())
			return nil, diags
		}
		return b, diags
	}

	diags.AddError("Invalid configuration", "Exactly one of content or content_file must be set")
	return nil, diags
}

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func sha256hexFromPlan(plan artifactResourceModel) string {
	b, diags := resolveContentFromPlan(plan)
	if diags.HasError() {
		return ""
	}
	return sha256hex(b)
}

func setToStrings(ctx context.Context, s types.Set) []string {
	if s.IsNull() || s.IsUnknown() {
		return nil
	}
	var out []string
	_ = s.ElementsAs(ctx, &out, false)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}

func stringsToSet(in []string) types.Set {
	vals := make([]attr.Value, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		vals = append(vals, types.StringValue(s))
	}
	set, _ := types.SetValue(types.StringType, vals)
	return set
}

func metadataDiffers(ctx context.Context, config, plan, state artifactResourceModel) bool {
	// Only compare name when explicitly configured. Otherwise the server-populated value in
	// state must not trigger updates.
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		pName := strings.TrimSpace(plan.Name.ValueString())
		sName := strings.TrimSpace(state.Name.ValueString())
		if pName != sName {
			return true
		}
	}
	if strings.TrimSpace(plan.Description.ValueString()) != strings.TrimSpace(state.Description.ValueString()) {
		return true
	}
	pLabels := normalizeStrings(setToStrings(ctx, plan.Labels))
	sLabels := normalizeStrings(setToStrings(ctx, state.Labels))
	return strings.Join(pLabels, "\n") != strings.Join(sLabels, "\n")
}

func normalizeStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func formatClientError(prefix string, err *client.ResponseError) string {
	if err == nil {
		return prefix
	}
	msg := prefix
	if err.StatusCode != 0 {
		msg += fmt.Sprintf(" (HTTP %d)", err.StatusCode)
	}
	if err.Err != nil {
		msg += ": " + err.Err.Error()
	}
	if strings.TrimSpace(err.Body) != "" {
		msg += "\nResponse body:\n" + err.Body
	}
	return msg
}
