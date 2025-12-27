package provider

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/fpetr/terraform-provider-apicurio/internal/client"
)

func TestResolveContentFromPlan_Content(t *testing.T) {
	plan := artifactResourceModel{Content: types.StringValue("hello")}
	b, diags := resolveContentFromPlan(plan)
	if diags.HasError() {
		t.Fatalf("expected no diagnostics, got: %v", diags)
	}
	if string(b) != "hello" {
		t.Fatalf("expected content 'hello', got %q", string(b))
	}
}

func TestResolveContentFromPlan_ContentFileError(t *testing.T) {
	plan := artifactResourceModel{ContentFile: types.StringValue("/path/does/not/exist")}
	_, diags := resolveContentFromPlan(plan)
	if !diags.HasError() {
		t.Fatalf("expected diagnostics error")
	}
}

func TestSha256hexFromPlan_InvalidConfigReturnsEmpty(t *testing.T) {
	plan := artifactResourceModel{Content: types.StringNull(), ContentFile: types.StringNull()}
	if got := sha256hexFromPlan(plan); got != "" {
		t.Fatalf("expected empty hash on invalid config, got %q", got)
	}
}

func TestApplyMetaToState_UnknownsBecomeNull(t *testing.T) {
	state := artifactResourceModel{
		GroupID:       types.StringValue("g"),
		ArtifactID:    types.StringValue("a"),
		GlobalID:      types.Int64Unknown(),
		ContentID:     types.Int64Unknown(),
		CreatedOn:     types.StringUnknown(),
		ModifiedOn:    types.StringUnknown(),
		LatestVersion: types.StringUnknown(),
		Name:          types.StringUnknown(),
		Labels:        types.SetUnknown(types.StringType),
	}

	out := applyMetaToState(state, client.NormalizedArtifactMeta{})

	if !out.GlobalID.IsNull() {
		t.Fatalf("expected GlobalID to become null")
	}
	if !out.ContentID.IsNull() {
		t.Fatalf("expected ContentID to become null")
	}
	if !out.CreatedOn.IsNull() {
		t.Fatalf("expected CreatedOn to become null")
	}
	if !out.ModifiedOn.IsNull() {
		t.Fatalf("expected ModifiedOn to become null")
	}
	if !out.LatestVersion.IsNull() {
		t.Fatalf("expected LatestVersion to become null")
	}
	if !out.Name.IsNull() {
		t.Fatalf("expected Name to become null")
	}
	if !out.Labels.IsUnknown() {
		t.Fatalf("expected Labels to remain unknown")
	}
}

func TestApplyMetaToState_LabelsSetOnlyWhenUnset(t *testing.T) {
	state := artifactResourceModel{
		GroupID:    types.StringValue("g"),
		ArtifactID: types.StringValue("a"),
		Labels:     types.SetNull(types.StringType),
	}
	out := applyMetaToState(state, client.NormalizedArtifactMeta{Labels: []string{" l1 ", "l2"}})
	if out.Labels.IsNull() {
		t.Fatalf("expected labels to be set")
	}

	// If labels are already set in state, do not overwrite them.
	existing, _ := types.SetValueFrom(context.Background(), types.StringType, []string{"keep"})
	state2 := artifactResourceModel{GroupID: types.StringValue("g"), ArtifactID: types.StringValue("a"), Labels: existing}
	out2 := applyMetaToState(state2, client.NormalizedArtifactMeta{Labels: []string{"new"}})
	var got []string
	_ = out2.Labels.ElementsAs(context.Background(), &got, false)
	if len(got) != 1 || got[0] != "keep" {
		t.Fatalf("expected labels to remain ['keep'], got %v", got)
	}
}

func TestFormatClientError(t *testing.T) {
	if got := formatClientError("prefix", nil); got != "prefix" {
		t.Fatalf("unexpected message: %q", got)
	}

	err := &client.ResponseError{StatusCode: 500, Err: os.ErrNotExist, Body: " boom "}
	msg := formatClientError("unable", err)
	if !strings.Contains(msg, "unable (HTTP 500)") {
		t.Fatalf("expected status code in message, got: %q", msg)
	}
	if !strings.Contains(msg, os.ErrNotExist.Error()) {
		t.Fatalf("expected underlying error in message, got: %q", msg)
	}
	if !strings.Contains(msg, "Response body") {
		t.Fatalf("expected response body in message, got: %q", msg)
	}
}
