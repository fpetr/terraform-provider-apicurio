package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRequestedVersionFromModel(t *testing.T) {
	{
		m := artifactResourceModel{Version: types.StringNull()}
		if got := requestedVersionFromModel(m); got != nil {
			t.Fatalf("expected nil for null version, got %q", *got)
		}
	}
	{
		m := artifactResourceModel{Version: types.StringUnknown()}
		if got := requestedVersionFromModel(m); got != nil {
			t.Fatalf("expected nil for unknown version, got %q", *got)
		}
	}
	{
		m := artifactResourceModel{Version: types.StringValue("   ")}
		if got := requestedVersionFromModel(m); got != nil {
			t.Fatalf("expected nil for empty/whitespace version, got %q", *got)
		}
	}
	{
		m := artifactResourceModel{Version: types.StringValue(" v2 ")}
		got := requestedVersionFromModel(m)
		if got == nil || *got != "v2" {
			t.Fatalf("expected trimmed 'v2', got %#v", got)
		}
	}
}

func TestRequestedVersionChanged(t *testing.T) {
	state := artifactResourceModel{Version: types.StringValue("v1")}
	if !requestedVersionChanged(state, ptr("v2")) {
		t.Fatalf("expected version change v1->v2")
	}
	if requestedVersionChanged(state, ptr(" v1 ")) {
		t.Fatalf("expected no change when only whitespace differs")
	}
	if requestedVersionChanged(state, nil) {
		t.Fatalf("expected no change when no requested version")
	}
}

func TestDecideRequestedVersionWrite_NoRequestedVersion(t *testing.T) {
	{
		deleteExisting, create, err := decideRequestedVersionWrite(false, nil, false, false, false)
		if deleteExisting || create || err != "" {
			t.Fatalf("expected no-op, got delete=%v create=%v err=%q", deleteExisting, create, err)
		}
	}
	{
		deleteExisting, create, err := decideRequestedVersionWrite(true, nil, false, false, false)
		if deleteExisting || !create || err != "" {
			t.Fatalf("expected create for contentChanged, got delete=%v create=%v err=%q", deleteExisting, create, err)
		}
	}
}

func TestDecideRequestedVersionWrite_VersionOnlyBumpCreatesWhenMissing(t *testing.T) {
	deleteExisting, create, err := decideRequestedVersionWrite(false, ptr("v2"), true, false, false)
	if deleteExisting || !create || err != "" {
		t.Fatalf("expected create when version changed and missing, got delete=%v create=%v err=%q", deleteExisting, create, err)
	}
}

func TestDecideRequestedVersionWrite_VersionChangeNoCreateWhenAlreadyExists(t *testing.T) {
	deleteExisting, create, err := decideRequestedVersionWrite(false, ptr("v2"), true, true, false)
	if deleteExisting || create || err != "" {
		t.Fatalf("expected no create when requested version already exists, got delete=%v create=%v err=%q", deleteExisting, create, err)
	}
}

func TestDecideRequestedVersionWrite_ContentChangedExistingVersion_OverwriteOffErrors(t *testing.T) {
	deleteExisting, create, err := decideRequestedVersionWrite(true, ptr("v1"), false, true, false)
	if deleteExisting || create || err == "" {
		t.Fatalf("expected error when rewriting existing version without overwrite, got delete=%v create=%v err=%q", deleteExisting, create, err)
	}
}

func TestDecideRequestedVersionWrite_ContentChangedExistingVersion_OverwriteOnDeletesAndCreates(t *testing.T) {
	deleteExisting, create, err := decideRequestedVersionWrite(true, ptr("v1"), false, true, true)
	if !deleteExisting || !create || err != "" {
		t.Fatalf("expected delete+create when overwrite enabled, got delete=%v create=%v err=%q", deleteExisting, create, err)
	}
}

func TestDecideRequestedVersionWrite_ContentChangedMissingVersion_Creates(t *testing.T) {
	deleteExisting, create, err := decideRequestedVersionWrite(true, ptr("v2"), false, false, false)
	if deleteExisting || !create || err != "" {
		t.Fatalf("expected create when contentChanged and version missing, got delete=%v create=%v err=%q", deleteExisting, create, err)
	}
}

func TestShouldAttemptVersionWrite(t *testing.T) {
	if shouldAttemptVersionWrite(false, nil, false) {
		t.Fatalf("expected false")
	}
	if !shouldAttemptVersionWrite(true, nil, false) {
		t.Fatalf("expected true for contentChanged")
	}
	if !shouldAttemptVersionWrite(false, ptr("v2"), true) {
		t.Fatalf("expected true for version-only bump")
	}
	if shouldAttemptVersionWrite(false, ptr("v2"), false) {
		t.Fatalf("expected false when nothing changed")
	}
}

func ptr(s string) *string { return &s }
